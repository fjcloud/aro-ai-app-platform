"""Exercise the platform setup with controlled API replies; never use live credentials."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('cluster_voice', ROOT/'scripts/prepare-cluster-voice.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ClusterVoicePreparation(unittest.TestCase):
    def run_prepare(self, existing_catalog=False, native_role=True):
        manifests, calls = [], []
        def oc(*args):
            calls.append(args)
            if args[:2] == ('get','mlflow'):
                return json.dumps({'status': {'url':'https://ai.lab.example/mlflow'}})
            if args[:2] == ('get','namespace') or args[:2] in (('get','clusterrole'),('get','role')):
                if args[:3] == ('get','clusterrole','mlflow-operator-mlflow-integration') and not native_role:
                    raise RuntimeError('Role not found')
                return '{}'
            if args[:3] == ('adm','release','info'):
                return 'quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:'+'a'*64
            if args[:2] == ('whoami','-t'):return 'private-user-token'
            if args[:2] == ('get','ingress.config.openshift.io'):return 'apps.lab.example'
            if args[:2] == ('create','token'):return 'private-runtime-token'
            if args[0] in ('apply','patch','label','import-image','wait'):return ''
            raise AssertionError(args)
        def run(args, **kwargs):
            if args[1:3] == ['get','secret']:return SimpleNamespace(returncode=1)
            if args[1:3] == ['get','configmap']:
                return SimpleNamespace(returncode=0 if existing_catalog else 1,
                                       stdout=json.dumps({'data': {'Other': '{}'}}))
            raise AssertionError(args)
        def request(url, token, data=None, workspace=None):
            if 'api-keys' in url:
                self.assertTrue(any(c[:2]==('wait','maassubscription/deepseekv4pro-cluster-voice') for c in calls))
                self.assertEqual(data['subscription'],'deepseekv4pro-cluster-voice')
                self.assertEqual(token,'private-user-token')
                return {'key':'sk-oai-private-runtime-key','id':'key-id'}
            self.assertEqual(token,'private-runtime-token')
            self.assertEqual(workspace,'live-transcript-dev')
            self.assertTrue(url.startswith('https://ai.lab.example/mlflow/api/'))
            return {'experiments':[{'name':'cluster-voice','experiment_id':'17'}]}
        output=io.StringIO()
        with patch.object(module,'oc',oc),patch.object(module,'apply',manifests.append),patch.object(module.subprocess,'run',run),patch.object(module,'request_json',request),contextlib.redirect_stdout(output):
            module.prepare()
        return manifests,calls,output.getvalue()

    def test_minimal_read_only_metrics_and_separate_runtime_secret(self):
        manifests,_,output=self.run_prepare()
        bindings=[m for m in manifests if m['kind'].endswith('RoleBinding')]
        monitoring=next(m for m in bindings if m['roleRef']['name']=='cluster-monitoring-metrics-api')
        self.assertEqual(monitoring['metadata']['namespace'],'openshift-monitoring')
        self.assertEqual(monitoring['roleRef']['kind'],'Role')
        self.assertEqual(monitoring['subjects'][0]['name'],'cluster-voice')
        self.assertEqual([m['roleRef']['name'] for m in bindings if m['kind']=='ClusterRoleBinding'],['system:auth-delegator'])
        self.assertFalse(any(m['roleRef']['name'] in ('edit','admin','cluster-admin','privileged') for m in bindings))
        key=next(m for m in manifests if m['kind']=='Secret' and m['metadata']['name']=='cluster-voice-model')
        self.assertEqual(key['metadata']['namespace'],'live-transcript-dev')
        self.assertNotIn('labels',key['metadata'])
        self.assertNotIn('sk-oai-private-runtime-key',output)
        self.assertNotIn('private-user-token',output)
        config=next(m for m in manifests if m['kind']=='ConfigMap' and m['metadata']['name']=='cluster-voice-config')
        self.assertEqual(config['data']['MLFLOW_EXPERIMENT_ID'],'17')
        self.assertEqual(config['data']['OTEL_EXPORTER_OTLP_TRACES_ENDPOINT'],'https://ai.lab.example/mlflow/v1/traces')

    def test_catalog_patch_does_not_replace_other_entries(self):
        manifests,calls,_=self.run_prepare(existing_catalog=True)
        self.assertFalse(any(m['metadata']['name']=='gen-ai-aa-mcp-servers' for m in manifests))
        call=next(c for c in calls if c[:2]==('patch','configmap'))
        patch_data=json.loads(call[-1])
        self.assertEqual(list(patch_data['data']),['OpenShift-Health'])
        self.assertTrue(json.loads(patch_data['data']['OpenShift-Health'])['url'].endswith('/mcp'))

    def test_rejects_unverified_or_credential_bearing_tracking_url(self):
        for url in ('http://bad.example','https://user:password@bad.example','https://bad.example?key=secret'):
            with patch.object(module,'oc') as oc,self.assertRaises(RuntimeError):module.prepare(url)
            oc.assert_not_called()

    def test_native_and_standalone_mlflow_role_names(self):
        for native in (True,False):
            with self.subTest(native=native):
                manifests,_,_=self.run_prepare(native_role=native)
                binding=next(m for m in manifests if m['metadata']['name']=='cluster-voice-mlflow')
                self.assertEqual(binding['roleRef']['name'], 'mlflow-operator-mlflow-integration' if native else 'mlflow-integration')

    def test_playground_identity_cannot_edit_namespace_or_read_secrets(self):
        manifests,_,_=self.run_prepare()
        role=next(m for m in manifests if m['kind']=='Role' and m['metadata']['name']=='cluster-voice-playground')
        self.assertEqual(role['rules'],[{'apiGroups':[''],'resources':['services'],'resourceNames':['live-transcript'],'verbs':['get']}])
        binding=next(m for m in manifests if m['kind']=='RoleBinding' and m['metadata']['name']=='cluster-voice-playground')
        self.assertEqual(binding['subjects'][0]['name'],'cluster-voice-playground')
