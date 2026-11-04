"""Prevent the prepared service from losing D3 access or leaking labels into inputs."""
import json
from pathlib import Path
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]


class CardScoutPlatform(unittest.TestCase):
    def test_d3_network_policy_matches_the_actual_caller(self):
        resources = list(yaml.safe_load_all((ROOT / 'deploy/card-scout/platform.yaml').read_text()))
        deployment = next(r for r in resources if r['kind'] == 'Deployment')
        labels = deployment['spec']['template']['metadata']['labels']
        policy = yaml.safe_load((ROOT / 'deploy/decision/networkpolicy.yaml').read_text())
        callers = [peer for rule in policy['spec']['ingress'] for peer in rule['from']
                   if peer.get('namespaceSelector', {}).get('matchLabels', {}).get('kubernetes.io/metadata.name') == 'signal-desk-dev']
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
