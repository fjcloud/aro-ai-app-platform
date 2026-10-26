"""Regression checks for provider endpoints and credential isolation, without Azure."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('coding_provider', ROOT/'scripts/configure-coding-provider.py')
provider = importlib.util.module_from_spec(spec)
spec.loader.exec_module(provider)


class CodingProvider(unittest.TestCase):
    def test_azure_endpoint_and_header_auth(self):
        p = provider.provider_manifest('https://workshop.cognitiveservices.azure.com/')
        self.assertEqual(p['spec']['endpoint'], 'workshop.cognitiveservices.azure.com')
        self.assertEqual(p['spec']['provider'], 'azure')
        self.assertEqual(p['spec']['auth'], {'type': 'apikey', 'secretRef': {'name': 'foundry-provider-key'}})

    def test_rejects_non_account_or_untrusted_endpoints(self):
        for endpoint in ('http://workshop.cognitiveservices.azure.com', 'https://evil.example',
                         'https://workshop.cognitiveservices.azure.com.evil.example',
                         'https://user:pass@workshop.cognitiveservices.azure.com',
                         'https://workshop.cognitiveservices.azure.com:444',
                         'https://workshop.cognitiveservices.azure.com/openai/v1',
                         'https://workshop.cognitiveservices.azure.com/?key=secret',
                         'https://workshop.cognitiveservices.azure.com/#fragment'):
            with self.subTest(endpoint=endpoint), self.assertRaises(ValueError):
                provider.provider_manifest(endpoint)

    def setup_calls(self, key_result=None, fail_secret=False):
        calls = []
        def run(args, **kwargs):
            calls.append((args, kwargs))
            if args[0] == 'az':
                return key_result or subprocess.CompletedProcess(args, 0, 'private-provider-key\n', '')
            if fail_secret and json.loads(kwargs.get('input', '{}')).get('kind') == 'Secret':
                return subprocess.CompletedProcess(args, 1, '', 'private-provider-key')
            return subprocess.CompletedProcess(args, 0, '', '')
        self.env = {'ENDPOINT': 'https://workshop.cognitiveservices.azure.com/', 'SUBSCRIPTION': 'lab',
                    'RESOURCEGROUP': 'assigned', 'FOUNDRY_NAME': 'workshop', 'API': '2025-06-01'}
        return calls, run

    def test_provider_secret_is_watched_and_never_injected_into_devspaces(self):
        calls, run = self.setup_calls()
        stdout = io.StringIO()
        with patch.dict(os.environ, self.env), patch.object(provider.subprocess, 'run', run), contextlib.redirect_stdout(stdout):
            provider.main()
        manifests = [json.loads(k['input']) for a, k in calls if 'input' in k]
        secret = next(m for m in manifests if m['kind'] == 'Secret')
        self.assertEqual(secret['metadata']['namespace'], 'llm-serving')
        self.assertEqual(secret['metadata']['labels'], {'inference.llm-d.ai/ipp-managed': 'true'})
        self.assertEqual(secret['stringData'], {'api-key': 'private-provider-key'})
        self.assertNotIn('private-provider-key', stdout.getvalue())
        self.assertFalse(any(m['metadata']['namespace'] == 'openshift-operators' for m in manifests))

    def test_missing_key_stops_before_cluster_mutation(self):
        for result in (subprocess.CompletedProcess([], 0, '', ''), subprocess.CompletedProcess([], 1, '', 'private-provider-key')):
            calls, run = self.setup_calls(result)
            with patch.dict(os.environ, self.env), patch.object(provider.subprocess, 'run', run), self.assertRaises(RuntimeError) as error:
                provider.main()
            self.assertNotIn('private-provider-key', str(error.exception))
            self.assertEqual(len(calls), 1)

    def test_bad_endpoint_stops_before_azure_or_cluster_calls(self):
        calls, run = self.setup_calls()
        self.env['ENDPOINT'] = 'https://evil.example'
        with patch.dict(os.environ, self.env), patch.object(provider.subprocess, 'run', run), self.assertRaises(ValueError):
            provider.main()
        self.assertEqual(calls, [])

    def test_failed_secret_apply_does_not_publish_provider_or_expose_key(self):
        calls, run = self.setup_calls(fail_secret=True)
        with patch.dict(os.environ, self.env), patch.object(provider.subprocess, 'run', run), self.assertRaises(RuntimeError) as error:
            provider.main()
        self.assertNotIn('private-provider-key', str(error.exception))
        self.assertEqual(len(calls), 3)


if __name__ == '__main__':
    unittest.main()
