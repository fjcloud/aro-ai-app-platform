#!/usr/bin/env python3
"""Platform-only setup: read-only monitoring, runtime MaaS key, OAuth and MLflow."""
import argparse
import base64
import importlib.util
import json
from pathlib import Path
import secrets
import ssl
import subprocess
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('developer', ROOT / 'scripts/prepare-developer.py')
developer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(developer)
oc, apply, metadata = developer.oc, developer.apply, developer.metadata


def get(kind, name, namespace=None):
    return json.loads(oc('get', kind, name, *(['-n', namespace] if namespace else []), '-o', 'json'))


def binding(name, namespace, role, subject, cluster=False, role_kind="ClusterRole"):
    return {'apiVersion': 'rbac.authorization.k8s.io/v1',
            'kind': 'ClusterRoleBinding' if cluster else 'RoleBinding',
            'metadata': metadata(name, None if cluster else namespace),
            'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': role_kind, 'name': role},
            'subjects': [subject]}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, newurl):
        return None


def request_json(url, token, data=None, workspace=None):
    headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'}
    if workspace:
        headers['X-MLFLOW-WORKSPACE'] = workspace
    request = urllib.request.Request(url, headers=headers,
                                     data=json.dumps(data).encode() if data is not None else None)
    # Never disable TLS verification or print response/error bodies containing keys.
    try:
        opener = urllib.request.build_opener(NoRedirect, urllib.request.HTTPSHandler(context=ssl.create_default_context()))
        with opener.open(request, timeout=45) as response:
            return json.loads(response.read(256 * 1024))
    except urllib.error.HTTPError as error:
        from urllib.parse import urlsplit
        raise RuntimeError('Authenticated API HTTP ' + str(error.code) + ' at ' + urlsplit(url).path) from None
    except Exception:
        raise RuntimeError('Authenticated API request failed; check login, endpoint readiness and quota') from None


def ensure_experiment(uri, token, namespace):
    # Search in the namespace workspace, then create if it does not exist.
    result = request_json(uri + '/api/2.0/mlflow/experiments/search', token,
                          {'filter': "name = 'cluster-voice'", 'max_results': 10}, namespace)
    for experiment in result.get('experiments', []):
        if experiment.get('name') == 'cluster-voice':
            return str(experiment['experiment_id'])
    return str(request_json(uri + '/api/2.0/mlflow/experiments/create', token,
                            {'name': 'cluster-voice'}, namespace)['experiment_id'])


def prepare(mlflow_uri=None):
    if not mlflow_uri:
        mlflow_uri = get('mlflow', 'mlflow').get('status', {}).get('url', '').rstrip('/')
    if not mlflow_uri:
        raise RuntimeError('MLflow has no external URL yet; wait for the instance or supply --mlflow-url')
    from urllib.parse import urlsplit
    url = urlsplit(mlflow_uri)
    if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise RuntimeError('MLflow requires a verified HTTPS tracking URL')
    namespace = 'live-transcript-dev'
    # Namespaces and developer permissions remain the responsibility of the preceding step.
    get('namespace', namespace)
    oc('label', 'namespace', namespace, 'opendatahub.io/dashboard=true', '--overwrite')
    # This role grants only metrics API access, rather than general monitoring UI access.
    get('role', 'cluster-monitoring-metrics-api', 'openshift-monitoring')
    integration_role = None
    for candidate in ('mlflow-operator-mlflow-integration', 'mlflow-integration'):
        try:
            get('clusterrole', candidate)
            integration_role = candidate
            break
        except RuntimeError:
            continue
    if not integration_role:
        raise RuntimeError('MLflow integration role is unavailable; check the native operator')
    subject = {'kind': 'ServiceAccount', 'name': 'cluster-voice', 'namespace': namespace}
    sa = metadata('cluster-voice', namespace)
    sa['annotations'] = {
        'serviceaccounts.openshift.io/oauth-redirectreference.cluster-voice': json.dumps({
            'kind': 'OAuthRedirectReference', 'apiVersion': 'v1',
            'reference': {'kind': 'Route', 'name': 'live-transcript'}})}
    apply({'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': sa})
    apply(binding('cluster-voice-' + namespace, 'openshift-monitoring',
                  'cluster-monitoring-metrics-api', subject, role_kind='Role'))
    # OAuth proxy delegates bearer authentication for MCP, never passes tokens upstream.
    apply(binding('cluster-voice-auth-' + namespace, None, 'system:auth-delegator', subject, cluster=True))
    apply(binding('cluster-voice-mlflow', namespace, integration_role, subject))
    # Playground tokens authorize only this connector's Service, not namespace editing.
    viewer = {'kind': 'ServiceAccount', 'name': 'cluster-voice-playground', 'namespace': namespace}
    apply({'apiVersion': 'v1', 'kind': 'ServiceAccount',
           'metadata': metadata(viewer['name'], namespace)})
    apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role',
           'metadata': metadata('cluster-voice-playground', namespace),
           'rules': [{'apiGroups': [''], 'resources': ['services'],
                      'resourceNames': ['live-transcript'], 'verbs': ['get']}]})
    apply(binding('cluster-voice-playground', namespace, 'cluster-voice-playground', viewer, role_kind='Role'))
    apply(binding('cluster-voice-image-puller', 'live-transcript-build', 'system:image-puller', subject))
    apply(binding('cluster-voice-proxy-puller', namespace, 'system:image-puller', subject))
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap',
           'metadata': {**metadata('cluster-voice-service-ca', namespace),
                        'annotations': {'service.beta.openshift.io/inject-cabundle': 'true'}}})
    # Preserve cookie secret on repeat runs, so setup does not invalidate browser sessions.
    exists = subprocess.run(['oc', 'get', 'secret', 'cluster-voice-oauth', '-n', namespace],
                            capture_output=True, timeout=60)
    if exists.returncode:
        apply({'apiVersion': 'v1', 'kind': 'Secret', 'metadata': metadata('cluster-voice-oauth', namespace),
               'type': 'Opaque', 'stringData': {'cookie-secret': base64.b64encode(secrets.token_bytes(32)).decode()}})
    # Use the OAuth proxy from this cluster release, not a moving image tag.
    proxy_image = oc('adm', 'release', 'info', '--image-for=oauth-proxy')
    if '@sha256:' not in proxy_image:
        raise RuntimeError('Could not resolve the cluster OAuth proxy image digest')
    apply({'apiVersion': 'image.openshift.io/v1', 'kind': 'ImageStream',
           'metadata': metadata('cluster-voice-oauth', namespace),
           'spec': {'tags': [{'name': 'current', 'from': {'kind': 'DockerImage', 'name': proxy_image},
                             'referencePolicy': {'type': 'Local'}}]}})
    oc('import-image', 'cluster-voice-oauth:current', '-n', namespace, '--confirm')
    # This token is used only for setup calls; workloads read their rotating projected token.
    user_token = oc('whoami', '-t')
    if not user_token:
        raise RuntimeError('Log in with an OpenShift user token to mint the runtime MaaS key')
    domain = oc('get', 'ingress.config.openshift.io', 'cluster', '-o', 'jsonpath={.spec.domain}')
    maas = 'https://maas.' + domain
    oc('apply', '-f', str(ROOT / 'deploy/agentops/runtime-subscription.yaml'))
    oc('wait', 'maassubscription/deepseekv4pro-cluster-voice', '-n', 'models-as-a-service',
       '--for=condition=Ready', '--timeout=180s')
    key = request_json(maas + '/maas-api/v1/api-keys', user_token,
                       {'name': 'cluster-voice-runtime', 'description': 'Cluster Voice runtime only',
                        'subscription': 'deepseekv4pro-cluster-voice', 'expiresIn': '24h'})
    if not str(key.get('key', '')).startswith('sk-oai-') or not key.get('id'):
        raise RuntimeError('MaaS returned no runtime key')
    secret_meta = metadata('cluster-voice-model', namespace)
    secret_meta['annotations'] = {'workshop.fjcloud.io/maas-key-id': str(key['id'])}
    # No Dev Spaces injection labels: this key is for the deployed agent only.
    apply({'apiVersion': 'v1', 'kind': 'Secret', 'metadata': secret_meta, 'type': 'Opaque',
           'stringData': {'OPENAI_API_KEY': key['key']}})
    token = oc('create', 'token', 'cluster-voice', '-n', namespace, '--duration=10m')
    experiment = ensure_experiment(mlflow_uri, token, namespace)
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': metadata('cluster-voice-config', namespace),
           'data': {'AGENT_BASE_URL': maas + '/v1', 'AGENT_MODEL': 'deepseekv4pro',
                    'MCP_PUBLIC_HOST': 'live-transcript-' + namespace + '.' + domain,
                    'PROMETHEUS_URL': 'https://thanos-querier.openshift-monitoring.svc:9091',
                    'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT': mlflow_uri + '/v1/traces',
                    'MLFLOW_EXPERIMENT_ID': experiment, 'MLFLOW_WORKSPACE': namespace}})
    # Merge one catalog entry; retain any other MCP servers already configured.
    result = subprocess.run(['oc', 'get', 'configmap', 'gen-ai-aa-mcp-servers',
                             '-n', 'redhat-ods-applications', '-o', 'json'], capture_output=True, text=True, timeout=60)
    catalog = json.loads(result.stdout) if result.returncode == 0 else {
        'apiVersion': 'v1', 'kind': 'ConfigMap',
        'metadata': metadata('gen-ai-aa-mcp-servers', 'redhat-ods-applications')}
    entry = json.dumps({'url': 'https://live-transcript-' + namespace + '.' + domain + '/mcp',
                        'description': 'Read-only OpenShift health metrics. Tools: list_metrics, cluster_health, query_metric.'})
    # JSON merge patch avoids taking ownership of the rest of the shared catalog.
    if result.returncode == 0:
        oc('patch', 'configmap', 'gen-ai-aa-mcp-servers', '-n', 'redhat-ods-applications',
           '--type=merge', '-p', json.dumps({'data': {'OpenShift-Health': entry}}))
    else:
        catalog['data'] = {'OpenShift-Health': entry}
        apply(catalog)
    print('Runtime prepared: read-only metrics, OpenShift login, separate MaaS key and MLflow experiment.')
    print('Runtime key expires after 24h. Rerun this step and restart the app to refresh it.')
    print('MCP becomes reachable after the app is deployed. No credentials printed.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--mlflow-url', help='Optional HTTPS tracking URL; defaults to MLflow status.url')
    args = parser.parse_args()
    try:
        prepare(args.mlflow_url.rstrip('/') if args.mlflow_url else None)
    except Exception as error:
        raise SystemExit('Cluster Voice preparation failed: ' + str(error))
