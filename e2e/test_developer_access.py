"""Developer preparation must never grant cluster administration or SCC use."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('prepare_developer', ROOT/'scripts/prepare-developer.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class DeveloperAccess(unittest.TestCase):
    def prepare(self, token='private-scoped-token'):
        manifests, calls = [], []
        def oc(*args):
            calls.append(args)
            if args[:2] == ('whoami', '--show-server'): return 'https://api.lab.example:6443'
            if args[0] == 'get': return '1000000000/10000'
            if args[0] == 'annotate': return ''
            if args[0] == 'config': return 'public-ca-data'
            if args[:2] == ('create', 'token'): return token
            self.fail('Unexpected command: ' + repr(args))
        output = io.StringIO()
        with patch.object(module, 'oc', oc), patch.object(module, 'apply', manifests.append), contextlib.redirect_stdout(output):
            module.prepare('test-app')
        return manifests, calls, output.getvalue()

    def test_only_two_namespaces_are_prepared(self):
        manifests, _, _ = self.prepare()
        self.assertEqual([m['metadata']['name'] for m in manifests if m['kind']=='Namespace'], ['test-app-build','test-app-dev'])
        self.assertNotIn('annotations', manifests[0]['metadata'])
        self.assertIn(('annotate', 'namespace', 'test-app-build', 'openshift.io/sa.scc.supplemental-groups=1000/10000', '--overwrite'), self.prepare()[1])
        self.assertNotIn('annotations',manifests[1]['metadata'])

    def test_no_scc_or_cluster_admin_grants(self):
        manifests, calls, _ = self.prepare()
        self.assertFalse(any(m['kind'] in ('ClusterRole','ClusterRoleBinding','SecurityContextConstraints') for m in manifests))
        for m in manifests:
            if m['kind']=='RoleBinding':
                self.assertIn(m['metadata']['namespace'],['test-app-build','test-app-dev'])
                self.assertIn(m['roleRef']['name'],['edit','workshop-delivery','system:image-puller'])
            if m['kind']=='Role':
                self.assertEqual({g for r in m['rules'] for g in r['apiGroups']},{'tekton.dev','argoproj.io'})
        self.assertFalse(any('add-scc-to-user' in c for c in calls))

    def test_scoped_token_only_in_workspace_secret(self):
        manifests, calls, output = self.prepare()
        secrets = [m for m in manifests if m['kind']=='Secret']
        self.assertEqual(len(secrets),1)
        config=json.loads(secrets[0]['stringData']['config'])
        self.assertEqual(config['users'][0]['user'],{'token':'private-scoped-token'})
        self.assertEqual(config['contexts'][0]['context']['namespace'],'test-app-dev')
        self.assertNotIn('private-scoped-token',output)
        self.assertIn(('create','token','workshop-developer','-n','test-app-dev','--duration=24h'),calls)

    def test_missing_security_allocation_stops_before_credentials(self):
        with patch.object(module, 'apply'), patch.object(module, 'oc', return_value='') as oc, patch.object(module.time, 'sleep'), self.assertRaises(RuntimeError):
            module.prepare('test-app')
        self.assertFalse(any(c.args[0] in ('annotate', 'create') for c in oc.call_args_list))

    def test_rejects_reserved_or_invalid_app_names_before_changes(self):
        for name in ('openshift-system','kube-system','a/b','UPPER','default-dev;bad','a'*41):
            with self.subTest(name=name),patch.object(module,'apply') as apply,patch.object(module,'oc') as oc,self.assertRaises(ValueError):
                module.prepare(name)
            apply.assert_not_called();oc.assert_not_called()
