#!/usr/bin/env python3
"""Platform setup: Signal Desk identity, native Responses, MaaS and MLflow."""
import importlib.util
import json
from pathlib import Path
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('platform', ROOT / 'scripts/prepare-decision-model.py')
platform = importlib.util.module_from_spec(spec)
spec.loader.exec_module(platform)
oc, get, apply = platform.oc, platform.get, platform.apply
APP, BUILD, IDENTITY = platform.APP, platform.BUILD, 'signal-desk'


def request(url, token, body=None, workspace=None):
    headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'}
    if workspace:
        headers['X-MLFLOW-WORKSPACE'] = workspace
    req = urllib.request.Request(url, data=None if body is None else json.dumps(body).encode(), headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=45) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError('Platform API returned HTTP ' + str(error.code) + '; check readiness and permissions') from None


def binding(name, role, namespace=None, subject=IDENTITY, role_kind='ClusterRole'):
    meta = {'name': name}
    if namespace:
        meta['namespace'] = namespace
    apply({'apiVersion': 'rbac.authorization.k8s.io/v1',
           'kind': 'RoleBinding' if namespace else 'ClusterRoleBinding', 'metadata': meta,
           'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': role_kind, 'name': role},
           'subjects': [{'kind': 'ServiceAccount', 'name': subject, 'namespace': APP}]})


def main():
    ready = get('inferenceservice', 'd3-mini', 'decision-serving')
    if not any(c['type'] == 'Ready' and c['status'] == 'True' for c in ready.get('status', {}).get('conditions', [])):
        raise RuntimeError('Wait for d3-mini to be Ready before preparing the application')
    get('service', 'lsd-genai-playground-service', APP)
    domain = get('ingress.config.openshift.io', 'cluster')['spec']['domain']
    mlflow = get('mlflow', 'mlflow').get('status', {}).get('url', '').rstrip('/')
    if not mlflow.startswith('https://'):
        raise RuntimeError('Wait for the native MLflow instance to publish its HTTPS URL')
    apply({'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': {'name': IDENTITY, 'namespace': APP}})
    role = next((r for r in ('mlflow-operator-mlflow-integration', 'mlflow-integration')
                 if oc('get', 'clusterrole', r, '--ignore-not-found', '-o', 'name')), None)
    if not role:
        raise RuntimeError('MLflow integration role is unavailable')
    binding('signal-desk-mlflow', role, APP)
    for namespace in (APP, BUILD):
        binding('signal-desk-image-puller', 'system:image-puller', namespace)
    # No cluster-monitoring, SCC or workload-editing grant is needed by this app.
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'signal-desk-service-ca', 'namespace': APP,
           'annotations': {'service.beta.openshift.io/inject-cabundle': 'true'}}})
    oc('apply', '-f', str(ROOT / 'deploy/briefing/runtime-subscription.yaml'))
    oc('wait', 'maassubscription/deepseekv4pro-signal-desk', '-n', 'models-as-a-service', '--for=condition=Ready', '--timeout=180s')
    # The key API needs an actual OpenShift user token, not a certificate-only login.
    user_token = oc('whoami', '-t')
    if not user_token:
        raise RuntimeError('Use the OpenShift token login from Act 0 to mint the runtime key')
    key = request('https://maas.' + domain + '/maas-api/v1/api-keys', user_token,
                  {'name': 'signal-desk-runtime', 'description': 'Signal Desk agent only', 'subscription': 'deepseekv4pro-signal-desk', 'expiresIn': '24h'})
    value = key.get('key') or key.get('apiKey')
    if not value:
        raise RuntimeError('MaaS returned no runtime key')
    apply({'apiVersion': 'v1', 'kind': 'Secret', 'metadata': {'name': 'signal-desk-model', 'namespace': APP},
           'type': 'Opaque', 'stringData': {'api-key': value}})
    token = oc('create', 'token', IDENTITY, '-n', APP, '--duration=10m')
    found = request(mlflow + '/api/2.0/mlflow/experiments/search', token, {'filter': "name = 'signal-desk'", 'max_results': 10}, APP)
    experiment = next((str(e['experiment_id']) for e in found.get('experiments', []) if e.get('name') == 'signal-desk'), None)
    if experiment is None:
        experiment = str(request(mlflow + '/api/2.0/mlflow/experiments/create', token, {'name': 'signal-desk'}, APP)['experiment_id'])
    public_host = 'signal-desk-' + APP + '.' + domain
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'signal-desk-config', 'namespace': APP},
           'data': {'AGENT_BACKEND': 'rhoai', 'AGENT_URL': 'http://lsd-genai-playground-service.' + APP + '.svc.cluster.local:8321/v1',
                    'AGENT_MODEL': 'maas-vllm-inference-1/deepseekv4pro', 'AGENT_JOB_TIMEOUT': '10m',
                    'DECISION_URL': 'http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone',
                    'DECISION_MODEL': 'vllm-sr/d3-mini', 'NEWS_CORPUS': 'bundled', 'DECISION_TIMEOUT': '120s', 'MCP_PUBLIC_HOST': public_host,
                    'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT': mlflow + '/v1/traces', 'MLFLOW_EXPERIMENT_ID': experiment, 'MLFLOW_WORKSPACE': APP}})
    apply({'apiVersion': 'v1', 'kind': 'PersistentVolumeClaim', 'metadata': {'name': 'signal-desk-data', 'namespace': APP},
           'spec': {'accessModes': ['ReadWriteOnce'], 'storageClassName': 'managed-csi', 'resources': {'requests': {'storage': '2Gi'}}}})
    entry = json.dumps({'url': 'https://' + public_host + '/mcp', 'description': 'Read-only archived Techmeme news and ETF catalog. No prices, trades or external URLs.'})
    if oc('get', 'configmap', 'gen-ai-aa-mcp-servers', '-n', 'redhat-ods-applications', '--ignore-not-found', '-o', 'name'):
        oc('patch', 'configmap', 'gen-ai-aa-mcp-servers', '-n', 'redhat-ods-applications', '--type=merge', '-p', json.dumps({'data': {'Signal-Desk': entry}}))
    else:
        apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'gen-ai-aa-mcp-servers', 'namespace': 'redhat-ods-applications'}, 'data': {'Signal-Desk': entry}})
    print('Signal Desk prepared: native Responses, internal D3, public HTTPS app/MCP, storage and MLflow. No keys printed.')
    print('Runtime MaaS key expires after 24h; rerun preparation and restart the app to refresh it.')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, urllib.error.URLError) as error:
        raise SystemExit(str(error)) from None
