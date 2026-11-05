"""Prevent the prepared service from losing D3 access or leaking labels into inputs."""
import json
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

import yaml

ROOT = Path(__file__).resolve().parents[1]


class CardScoutPlatform(unittest.TestCase):
    def test_runtime_setup_matches_deployed_identity_subscription_and_mounts(self):
        spec = importlib.util.spec_from_file_location('card_scout_setup', ROOT / 'scripts/prepare-card-scout.py')
        setup = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(setup)
        access = list(yaml.safe_load_all((ROOT / 'deploy/card-scout/access/runtime-resources.yaml').read_text()))
        identity = next(r['metadata']['name'] for r in access if r['kind'] == 'ServiceAccount')
        namespace = next(r['metadata']['namespace'] for r in access if r['kind'] == 'ServiceAccount')
        subscription = yaml.safe_load((ROOT / 'deploy/card-scout/access/runtime-subscription.yaml').read_text())['metadata']['name']
        resources = list(yaml.safe_load_all((ROOT / 'deploy/card-scout/platform.yaml').read_text()))
        pod = next(r for r in resources if r['kind'] == 'Deployment')['spec']['template']['spec']
        self.assertEqual(pod['serviceAccountName'], identity)
        self.assertEqual(yaml.safe_load((ROOT / 'deploy/card-scout/kustomization.yaml').read_text())['namespace'], namespace)
        applied, requests = [], []

        def get(kind, name, ns=None):
            if kind == 'inferenceservice':
                return {'status': {'conditions': [{'type': 'Ready', 'status': 'True'}]}}
            if kind == 'service':
                self.assertEqual(ns, namespace)
                return {}
            if kind == 'mlflow':
                return {'status': {'url': 'https://mlflow.example.test'}}
            if kind == 'ingress.config.openshift.io':
                return {'spec': {'domain': 'apps.example.test'}}
            self.fail('Unexpected resource: ' + kind)

        def request(url, token, body=None, workspace=None):
            requests.append((url, body, workspace))
            if url.endswith('/api-keys'):
                self.assertEqual(body['subscription'], subscription)
                return {'key': 'fixture-key'}
            if url.endswith('/experiments/search'):
                return {'experiments': []}
            if url.endswith('/experiments/create'):
                return {'experiment_id': '7'}
            self.fail('Unexpected API: ' + url)

        with patch.object(setup, 'get', get), patch.object(setup, 'request', request), \
                patch.object(setup, 'oc', return_value='fixture-token') as oc, \
                patch.object(setup, 'apply', applied.append), \
                patch.object(setup.runtime, 'apply', applied.append):
            setup.main()
        oc.assert_any_call('create', 'token', identity, '-n', namespace, '--duration=10m')
        config = next(r for r in applied if r['kind'] == 'ConfigMap')
        secret = next(r for r in applied if r['kind'] == 'Secret')
        self.assertEqual(pod['containers'][0]['envFrom'][0]['configMapRef']['name'], config['metadata']['name'])
        self.assertIn(secret['metadata']['name'], [v['secret']['secretName'] for v in pod['volumes'] if 'secret' in v])
        self.assertEqual(config['data']['MLFLOW_WORKSPACE'], namespace)
        self.assertEqual(config['data']['CARD_SCOUT_URL'], 'http://card-scout-platform.' + namespace + '.svc.cluster.local:8080')
        self.assertTrue(all(r['metadata'].get('namespace') == namespace for r in applied))

    def test_d3_network_policy_matches_the_actual_caller(self):
        resources = list(yaml.safe_load_all((ROOT / 'deploy/card-scout/platform.yaml').read_text()))
        deployment = next(r for r in resources if r['kind'] == 'Deployment')
        labels = deployment['spec']['template']['metadata']['labels']
        policy = yaml.safe_load((ROOT / 'deploy/decision/networkpolicy.yaml').read_text())
        callers = [peer for rule in policy['spec']['ingress'] for peer in rule['from']
                   if peer.get('namespaceSelector', {}).get('matchLabels', {}).get('kubernetes.io/metadata.name') == 'card-scout-dev']
        self.assertTrue(any(all(labels.get(k) == v for k, v in peer.get('podSelector', {}).get('matchLabels', {}).items()) for peer in callers))
        self.assertTrue(all(peer.get('podSelector', {}).get('matchLabels') for peer in callers), 'Do not open model access to the entire namespace')

    def test_inputs_and_reference_labels_are_distinct(self):
        data = ROOT / 'examples/card-scout/data'
        inputs = json.loads((data / 'inputs.json').read_text())['items']
        references = json.loads((data / 'references.json').read_text())['items']
        self.assertEqual(len(inputs), 105)
        self.assertEqual({i['id'] for i in inputs}, {r['id'] for r in references})
        self.assertTrue(all(set(i) == {'id', 'image_url', 'image_sha256'} for i in inputs))
        self.assertEqual(len({i['image_sha256'] for i in inputs}), 105)
        self.assertEqual({r['set_id'] for r in references if r['card_kind'] == 'pokemon_tcg'}, {'base1'})

    def test_complete_numbered_english_set_and_bundled_bytes(self):
        import hashlib
        data = ROOT / 'examples/card-scout/data'
        references = [r for r in json.loads((data / 'references.json').read_text())['items'] if r['card_kind'] == 'pokemon_tcg']
        self.assertEqual(sorted(int(r['number']) for r in references), list(range(1, 103)))
        self.assertTrue(all(r['language'] == 'en' and r['print_edition'] == '1st Edition' for r in references))
        inputs = json.loads((data / 'inputs.json').read_text())['items']
        for item in inputs:
            digest = item['image_sha256']
            self.assertEqual(hashlib.sha256((data / 'images' / digest).read_bytes()).hexdigest(), digest)
        self.assertEqual({p.name for p in (data / 'images').iterdir()}, {i['image_sha256'] for i in inputs})
