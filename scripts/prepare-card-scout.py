#!/usr/bin/env python3
"""Prepare native agent credentials and MLflow; manifests own the service."""
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('runtime_access', ROOT / 'scripts/prepare-briefing-desk.py')
runtime = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runtime)
oc, get, apply, request = runtime.oc, runtime.get, runtime.apply, runtime.request
APP = runtime.APP


def main():
    ready = get('inferenceservice', 'd3-mini', 'decision-serving')
    if not any(c['type'] == 'Ready' and c['status'] == 'True' for c in ready.get('status', {}).get('conditions', [])):
        raise RuntimeError('Wait for d3-mini before preparing Card Scout')
    get('service', 'lsd-genai-playground-service', APP)
    mlflow = get('mlflow', 'mlflow').get('status', {}).get('url', '').rstrip('/')
    if not mlflow.startswith('https://'):
        raise RuntimeError('Wait for MLflow to publish its URL')
    domain = get('ingress.config.openshift.io', 'cluster')['spec']['domain']
    role = next((r for r in ('mlflow-operator-mlflow-integration', 'mlflow-integration') if oc('get', 'clusterrole', r, '--ignore-not-found', '-o', 'name')), None)
    if not role:
        raise RuntimeError('MLflow integration role is unavailable')
    runtime.binding('signal-desk-mlflow', role, APP)
    oc('wait', 'maassubscription/deepseekv4pro-signal-desk', '-n', 'models-as-a-service', '--for=condition=Ready', '--timeout=180s')
    user_token = oc('whoami', '-t')
    key = request('https://maas.' + domain + '/maas-api/v1/api-keys', user_token, {'name': 'card-scout-runtime', 'description': 'Card Scout native agent', 'subscription': 'deepseekv4pro-signal-desk', 'expiresIn': '24h'})
    value = key.get('key') or key.get('apiKey')
    if not value:
        raise RuntimeError('MaaS returned no runtime key')
    apply({'apiVersion': 'v1', 'kind': 'Secret', 'metadata': {'name': 'signal-desk-model', 'namespace': APP}, 'type': 'Opaque', 'stringData': {'api-key': value}})
    token = oc('create', 'token', 'signal-desk', '-n', APP, '--duration=10m')
    found = request(mlflow + '/api/2.0/mlflow/experiments/search', token, {'filter': "name = 'card-scout'", 'max_results': 10}, APP)
    experiment = next((str(e['experiment_id']) for e in found.get('experiments', []) if e.get('name') == 'card-scout'), None)
    if experiment is None:
        experiment = str(request(mlflow + '/api/2.0/mlflow/experiments/create', token, {'name': 'card-scout'}, APP)['experiment_id'])
    service = 'http://card-scout-platform.' + APP + '.svc.cluster.local:8080'
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'signal-desk-config', 'namespace': APP}, 'data': {
        'CARD_SCOUT_URL': service, 'CARD_MCP_URL': service + '/mcp',
        'AGENT_URL': 'http://lsd-genai-playground-service.' + APP + '.svc.cluster.local:8321/v1',
        'AGENT_MODEL': 'maas-vllm-inference-1/deepseekv4pro',
        'DECISION_URL': 'http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone', 'DECISION_MODEL': 'vllm-sr/d3-mini',
        'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT': mlflow + '/v1/traces', 'MLFLOW_EXPERIMENT_ID': experiment, 'MLFLOW_WORKSPACE': APP}})
    entry = json.dumps({'url': 'https://card-scout-tools-' + APP + '.' + domain + '/mcp', 'description': 'Verify mixed-era catalogue cards and retrieve dated market reference prices.'})
    name = 'gen-ai-aa-mcp-servers'
    if oc('get', 'configmap', name, '-n', 'redhat-ods-applications', '--ignore-not-found', '-o', 'name'):
        oc('patch', 'configmap', name, '-n', 'redhat-ods-applications', '--type=merge', '-p', json.dumps({'data': {'Signal-Desk': None, 'Card-Scout': entry}}))
    else:
        apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': name, 'namespace': 'redhat-ods-applications'}, 'data': {'Card-Scout': entry}})
    print('Card Scout configured: native agent, scoped runtime key, fixed model endpoints and MLflow. No keys printed.')


if __name__ == '__main__':
    try:
        main()
    except RuntimeError as error:
        raise SystemExit(str(error)) from None
