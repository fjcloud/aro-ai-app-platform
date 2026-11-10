"""Live API validation must inspect the developer actually using the workspace."""
import contextlib
import importlib.util
import io
import os
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('model_api_checks', Path(__file__).with_name('check.py'))
checks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checks)


class ModelApiIdentity(unittest.TestCase):
    def test_rejects_administrator_and_retired_service_account_before_calling_models(self):
        for identity in ('workshop-admin', 'system:serviceaccount:card-scout-dev:workshop-developer'):
            with self.subTest(identity=identity), patch.dict(os.environ, {'E2E_NS': 'dev-devspaces', 'DEV_POD': 'workspace-pod'}), \
                    patch.object(checks, 'oc', return_value=identity), patch.object(checks, 'resource') as resource, \
                    contextlib.redirect_stdout(io.StringIO()), self.assertRaisesRegex(AssertionError, 'native dev user'):
                checks.model_apis()
            resource.assert_not_called()

    def test_forbidden_model_access_is_checked_from_the_workspace_without_impersonation(self):
        def oc(*args):
            return 'dev' if args[-1] == 'oc whoami' else '{"items":[]}'

        ready = {'status': {'conditions': [{'type': 'Ready', 'status': 'True'}]}}
        with patch.dict(os.environ, {'E2E_NS': 'dev-devspaces', 'DEV_POD': 'workspace-pod'}), \
                patch.object(checks, 'oc', side_effect=oc), patch.object(checks, 'resource', return_value=ready), \
                patch.object(checks.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, 'yes\n')) as run, \
                contextlib.redirect_stdout(io.StringIO()), self.assertRaisesRegex(AssertionError, 'cannot get secrets'):
            checks.model_apis()
        command = run.call_args.args[0]
        self.assertEqual(command[:6], ['oc', 'exec', 'workspace-pod', '-n', 'dev-devspaces', '-c'])
        self.assertEqual(command[-1], 'oc auth can-i get secrets -n decision-serving')
        self.assertFalse(any(arg.startswith('--as') for arg in command))


if __name__ == '__main__':
    unittest.main()
