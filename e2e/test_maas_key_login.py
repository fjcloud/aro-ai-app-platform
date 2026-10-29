"""Exercise the real key script with certificate-only and token-based oc sessions."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class MaaSKeyLogin(unittest.TestCase):
    def run_script(self, mode, script='deploy/coding/mint-workshop-key.sh'):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            log = folder / 'commands.jsonl'
            common = '''#!/usr/bin/env python3
import json, os, sys
with open(os.environ['COMMAND_LOG'], 'a') as stream:
    stream.write(json.dumps([os.path.basename(sys.argv[0]), *sys.argv[1:]]) + '\\n')
'''
            oc = common + '''
args = sys.argv[1:]
if args == ['whoami']:
    print('system:admin')
elif args == ['whoami', '-t']:
    mode = os.environ['TOKEN_MODE']
    if mode == 'certificate':
        print('error: no token is currently in use for this session', file=sys.stderr)
        sys.exit(1)
    if mode == 'token':
        print('private-user-token')
elif args[:3] == ['create', 'secret', 'generic']:
    print('{"kind":"Secret"}')
elif args[:1] == ['apply']:
    sys.stdin.read()
elif args[:1] not in (['label'], ['annotate']):
    sys.exit('Unexpected oc command')
'''
            curl = common + '''
print('{"id":"test-key-id","key":"sk-oai-test-only"}')
print('201')
'''
            for name, source in [('oc', oc), ('curl', curl), ('az', common)]:
                path = folder / name
                path.write_text(source)
                path.chmod(0o700)
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'],
                       COMMAND_LOG=str(log), TOKEN_MODE=mode)
            env.pop('MAAS_HOST', None)
            if mode == 'token':
                env['MAAS_HOST'] = 'maas.apps.example.test'
            result = subprocess.run(['bash', str(ROOT/script)],
                                    env=env, capture_output=True, text=True, timeout=15)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            return result, calls

    def test_certificate_login_stops_before_discovery_or_minting(self):
        result, calls = self.run_script('certificate')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Run make onboard', result.stderr)
        self.assertNotIn('error: no token is currently in use', result.stderr)
        self.assertEqual(calls, [['oc', 'whoami', '-t']])

    def test_empty_token_also_stops_before_any_side_effect(self):
        result, calls = self.run_script('empty')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Run make onboard', result.stderr)
        self.assertEqual(calls, [['oc', 'whoami', '-t']])

    def test_user_token_mints_and_stores_the_key_without_printing_credentials(self):
        result, calls = self.run_script('token')
        self.assertEqual(result.returncode, 0, result.stderr)
        mint = next(call for call in calls if call[0] == 'curl')
        self.assertIn('Authorization: Bearer private-user-token', mint)
        create = next(call for call in calls if call[:4] == ['oc', 'create', 'secret', 'generic'])
        self.assertIn('--from-literal=OPENAI_API_KEY=sk-oai-test-only', create)
        self.assertTrue(any(call[:3] == ['oc', 'apply', '-f'] for call in calls))
        for secret in ['private-user-token', 'sk-oai-test-only']:
            self.assertNotIn(secret, result.stdout + result.stderr)
