#!/usr/bin/env python3
"""Facilitator-only experimental setup on an already prepared ARO workshop."""
import argparse
import base64
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
NS, BUILD, APP = 'decision-serving', 'live-transcript-build', 'live-transcript-dev'
IMAGE = 'image-registry.openshift-image-registry.svc:5000/' + BUILD + '/decision-runtime:latest'
MODEL_REVISION = 'a475367de0d8133a3c1a2a0dc12303d89cfa2ab4'


def oc(*args, data=None):
    result = subprocess.run(['oc', *args], input=None if data is None else json.dumps(data),
                            capture_output=True, text=True)
    if result.returncode:
        # Never include API bodies or Secret data in diagnostics.
        raise RuntimeError('OpenShift command failed: ' + ' '.join(args[:3]))
    return result.stdout.strip()


def get(kind, name, namespace=None):
    return json.loads(oc('get', kind, name, *(['-n', namespace] if namespace else []), '-o', 'json'))


def apply(obj):
    oc('apply', '--server-side', '-f', '-', data=obj)


def wait_job(kind, name, namespace, seconds=1800):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        obj = get(kind, name, namespace)
        conditions = obj.get('status', {}).get('conditions', [])
        for c in conditions:
            if c['type'] in ('Succeeded', 'Complete') and c['status'] == 'True':
                return obj
            if (c['type'] == 'Succeeded' and c['status'] == 'False') or (c['type'] == 'Failed' and c['status'] == 'True'):
                raise RuntimeError(name + ' failed; inspect its pod logs')
        time.sleep(5)
    raise RuntimeError(name + ' did not finish before the timeout')


def build_runtime():
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    if subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip():
        raise RuntimeError('Commit and push this workshop checkout before building its runtime')
    active = json.loads(oc('get', 'pipelineruns', '-n', BUILD, '-o', 'json'))
    if any(c['status'] == 'Unknown' for p in active['items'] for c in p.get('status', {}).get('conditions', []) if c['type'] == 'Succeeded'):
        raise RuntimeError('Wait for existing pipeline runs; the source PVC is shared')
    auths = json.loads(base64.b64decode(get('secret', 'pull-secret', 'openshift-config')['data']['.dockerconfigjson']))['auths']
    if 'registry.redhat.io' not in auths:
        raise RuntimeError('The cluster pull secret has no Red Hat Registry authentication')
    token = oc('create', 'token', 'pipeline', '-n', BUILD, '--duration=2h')
    registry = {'auths': {'registry.redhat.io': auths['registry.redhat.io'],
                'image-registry.openshift-image-registry.svc:5000': {
                    'auth': base64.b64encode(('serviceaccount:' + token).encode()).decode()}}}
    secret = 'decision-runtime-build-registry'
    obj = {'apiVersion': 'v1', 'kind': 'Secret', 'metadata': {'name': secret, 'namespace': BUILD},
           'type': 'kubernetes.io/dockerconfigjson', 'data': {
               '.dockerconfigjson': base64.b64encode(json.dumps(registry).encode()).decode()}}
    # Refuse to overwrite credentials belonging to another run.
    oc('create', '-f', '-', data=obj)
    try:
        task = get('task', 'workshop-buildah', 'openshift-pipelines')['spec']
        task['volumes'] = [{'name': 'registry-auth', 'secret': {'secretName': secret}}]
        for step in task['steps']:
            step.setdefault('env', []).append({'name': 'REGISTRY_AUTH_FILE', 'value': '/var/run/registry-auth/.dockerconfigjson'})
            step.setdefault('volumeMounts', []).append({'name': 'registry-auth', 'mountPath': '/var/run/registry-auth', 'readOnly': True})
        resolver = {'resolver': 'cluster', 'params': [{'name': 'kind', 'value': 'task'},
                    {'name': 'name', 'value': 'workshop-git-clone'}, {'name': 'namespace', 'value': 'openshift-pipelines'}]}
        run = {'apiVersion': 'tekton.dev/v1', 'kind': 'PipelineRun', 'metadata': {
            'generateName': 'build-decision-runtime-', 'namespace': BUILD}, 'spec': {
            'taskRunTemplate': {'serviceAccountName': 'pipeline', 'podTemplate': {
                'hostUsers': False, 'securityContext': {'runAsUser': 0, 'runAsGroup': 1000, 'fsGroup': 1000}}},
            'pipelineSpec': {'workspaces': [{'name': 'source'}], 'tasks': [
                {'name': 'clone', 'taskRef': resolver, 'params': [
                    {'name': 'URL', 'value': 'https://github.com/fjcloud/aro-ai-app-platform.git'},
                    {'name': 'REVISION', 'value': revision}], 'workspaces': [{'name': 'output', 'workspace': 'source'}]},
                {'name': 'build', 'runAfter': ['clone'], 'taskSpec': task, 'params': [
                    {'name': 'IMAGE', 'value': IMAGE},
                    {'name': 'DOCKERFILE', 'value': './examples/decision-tetris/runtime/Dockerfile'},
                    {'name': 'CONTEXT', 'value': './examples/decision-tetris/runtime'}],
                 'workspaces': [{'name': 'source', 'workspace': 'source'}]}]},
            'workspaces': [{'name': 'source', 'persistentVolumeClaim': {'claimName': 'build-ws'}}]}}
        created = json.loads(oc('create', '-f', '-', '-o', 'json', data=run))
        name = created['metadata']['name']
        print('Building custom runtime with Tekton:', name, flush=True)
        wait_job('pipelinerun', name, BUILD)
    finally:
        oc('delete', 'secret', secret, '-n', BUILD, '--ignore-not-found')


def prepare_app():
    def binding(name, namespace, role):
        obj = {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding' if namespace else 'ClusterRoleBinding',
               'metadata': {'name': name}, 'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'ClusterRole', 'name': role},
               'subjects': [{'kind': 'ServiceAccount', 'name': 'decision-tetris', 'namespace': APP}]}
        if namespace:
            obj['metadata']['namespace'] = namespace
        apply(obj)
    apply({'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': {'name': 'decision-tetris', 'namespace': APP,
           'annotations': {'serviceaccounts.openshift.io/oauth-redirectreference.primary': json.dumps({
               'kind': 'OAuthRedirectReference', 'apiVersion': 'v1', 'reference': {'kind': 'Route', 'name': 'live-transcript'}})}}})
    binding('decision-tetris-auth-' + APP, None, 'system:auth-delegator')
    role = next((r for r in ('mlflow-operator-mlflow-integration', 'mlflow-integration')
                 if oc('get', 'clusterrole', r, '--ignore-not-found', '-o', 'name')), None)
    if not role:
        raise RuntimeError('Install native MLflow before preparing Decision Tetris')
    binding('decision-tetris-mlflow', APP, role)
    for ns in (APP, BUILD):
        binding('decision-tetris-image-puller', ns, 'system:image-puller')
    domain = get('ingress.config.openshift.io', 'cluster')['spec']['domain']
    uri = 'https://rh-ai.' + domain + '/mlflow'
    token = oc('create', 'token', 'decision-tetris', '-n', APP, '--duration=10m')
    def request(path, body=None):
        req = urllib.request.Request(uri + path, data=None if body is None else json.dumps(body).encode(),
                headers={'Authorization': 'Bearer ' + token, 'X-MLFLOW-WORKSPACE': APP, 'Content-Type': 'application/json'})
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)
    try:
        experiment = request('/api/2.0/mlflow/experiments/get-by-name?experiment_name=decision-tetris')['experiment']['experiment_id']
    except urllib.error.HTTPError as err:
        if err.code != 404:
            raise RuntimeError('MLflow experiment lookup failed: HTTP ' + str(err.code)) from None
        experiment = request('/api/2.0/mlflow/experiments/create', {'name': 'decision-tetris'})['experiment_id']
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'decision-tetris-config', 'namespace': APP},
           'data': {'DECISION_URL': 'http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone',
                    'DECISION_MODEL': 'vllm-sr/d3-mini', 'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT': uri + '/v1/traces',
                    'MLFLOW_EXPERIMENT_ID': str(experiment), 'MLFLOW_WORKSPACE': APP}})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reuse-runtime', action='store_true', help='reuse the already built runtime image')
    args = parser.parse_args()
    oc('get', 'namespace', APP)
    oc('get', 'task', 'workshop-buildah', '-n', 'openshift-pipelines')
    oc('apply', '-f', str(ROOT / 'deploy/operators/tekton-tasks.yaml'))
    apply({'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': NS, 'labels': {'opendatahub.io/dashboard': 'true'}}})
    apply({'apiVersion': 'v1', 'kind': 'PersistentVolumeClaim', 'metadata': {'name': 'decision-cache', 'namespace': NS},
           'spec': {'accessModes': ['ReadWriteOnce'], 'storageClassName': 'managed-csi', 'resources': {'requests': {'storage': '20Gi'}}}})
    if not args.reuse_runtime:
        build_runtime()
    oc('get', 'imagestreamtag', 'decision-runtime:latest', '-n', BUILD)
    apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding',
           'metadata': {'name': 'decision-runtime-puller', 'namespace': BUILD},
           'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'ClusterRole', 'name': 'system:image-puller'},
           'subjects': [{'kind': 'ServiceAccount', 'name': 'default', 'namespace': NS}]})
    job = {'apiVersion': 'batch/v1', 'kind': 'Job', 'metadata': {'generateName': 'd3-mini-download-', 'namespace': NS},
           'spec': {'backoffLimit': 1, 'activeDeadlineSeconds': 1800, 'template': {'spec': {
               'restartPolicy': 'Never', 'automountServiceAccountToken': False,
               'nodeSelector': {'node.kubernetes.io/instance-type': 'Standard_NC8as_T4_v3'},
               'tolerations': [{'key': 'nvidia.com/gpu', 'operator': 'Exists', 'effect': 'NoSchedule'}],
               'containers': [{'name': 'download', 'image': IMAGE,
                   'command': ['python3', '-c', "from huggingface_hub import snapshot_download; snapshot_download('vllm-sr/d3-mini',revision='" + MODEL_REVISION + "',cache_dir='/cache/hub',max_workers=4)"],
                   'env': [{'name': 'HF_HUB_OFFLINE', 'value': '0'}, {'name': 'TRANSFORMERS_OFFLINE', 'value': '0'}, {'name': 'HOME', 'value': '/tmp'}],
                   'securityContext': {'runAsNonRoot': True, 'allowPrivilegeEscalation': False, 'capabilities': {'drop': ['ALL']}},
                   'resources': {'requests': {'cpu': '500m', 'memory': '1Gi'}, 'limits': {'cpu': '2', 'memory': '4Gi'}},
                   'volumeMounts': [{'name': 'cache', 'mountPath': '/cache'}]}],
               'volumes': [{'name': 'cache', 'persistentVolumeClaim': {'claimName': 'decision-cache'}}]}}}}
    name = json.loads(oc('create', '-f', '-', '-o', 'json', data=job))['metadata']['name']
    print('Caching the pinned model:', name, flush=True)
    wait_job('job', name, NS)
    # This assigned workshop has one GPU. Keep the speech model and its cache, stopped.
    if oc('get', 'inferenceservice', 'whisper', '-n', 'speech-serving', '--ignore-not-found', '-o', 'name'):
        oc('annotate', 'inferenceservice', 'whisper', '-n', 'speech-serving', 'serving.kserve.io/stop=true', '--overwrite')
    oc('apply', '--server-side', '-k', str(ROOT / 'deploy/decision'))
    # Freeze the custom runtime to the image actually built, not a mutable tag.
    digest = get('imagestreamtag', 'decision-runtime:latest', BUILD)['image']['metadata']['name']
    oc('patch', 'servingruntime', 'd3-native', '-n', NS, '--type=json', '-p', json.dumps([
        {'op': 'replace', 'path': '/spec/containers/0/image', 'value': IMAGE.rsplit(':', 1)[0] + '@' + digest}]))
    prepare_app()
    oc('wait', 'inferenceservice/d3-mini', '-n', NS, '--for=condition=Ready', '--timeout=900s')
    print('d3-mini is ready on the local T4. Copy the decision-tetris reference into the personal app, then build and ship it.')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, urllib.error.URLError) as error:
        raise SystemExit(str(error)) from None
