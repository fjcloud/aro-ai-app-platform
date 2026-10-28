#!/usr/bin/env python3
"""Legacy decision-game setup; model serving is shared with Signal Desk."""
import argparse
import base64
import importlib.util
import json
from pathlib import Path
import urllib.error
import urllib.request

spec = importlib.util.spec_from_file_location('decision_model', Path(__file__).with_name('prepare-decision-model.py'))
model = importlib.util.module_from_spec(spec)
spec.loader.exec_module(model)
oc, get, apply, wait_job = model.oc, model.get, model.apply, model.wait_job
NS, BUILD, APP = model.NS, model.BUILD, model.APP

def prepare_app(experiment_name='decision-tetris'):
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
        experiment = request('/api/2.0/mlflow/experiments/get-by-name?experiment_name=' + experiment_name)['experiment']['experiment_id']
    except urllib.error.HTTPError as err:
        if err.code != 404:
            raise RuntimeError('MLflow experiment lookup failed: HTTP ' + str(err.code)) from None
        experiment = request('/api/2.0/mlflow/experiments/create', {'name': experiment_name})['experiment_id']
    apply({'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': {'name': 'decision-tetris-config', 'namespace': APP},
           'data': {'DECISION_URL': 'http://d3-mini.decision-serving.svc.cluster.local:8080/v1/systemone',
                    'DECISION_MODEL': 'vllm-sr/d3-mini', 'OTEL_EXPORTER_OTLP_TRACES_ENDPOINT': uri + '/v1/traces',
                    'MLFLOW_EXPERIMENT_ID': str(experiment), 'MLFLOW_WORKSPACE': APP}})



def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reuse-runtime', action='store_true')
    parser.add_argument('--game', choices=['decision-tetris', 'orbit-checkpoint', 'sky-control'], default='decision-tetris')
    args = parser.parse_args()
    model.prepare(args.reuse_runtime)
    prepare_app(args.game)


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, urllib.error.URLError) as error:
        raise SystemExit(str(error)) from None
