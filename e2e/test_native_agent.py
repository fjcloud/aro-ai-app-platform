"""A missing Responses backend must stop before credentials are created."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('native_agent', ROOT / 'scripts/prepare-briefing-desk.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class NativeAgentPrerequisites(unittest.TestCase):
    def test_missing_backend_reports_cli_step_without_creating_credentials(self):
        def get(kind, name, namespace=None):
            if kind == 'inferenceservice':
                return {'status': {'conditions': [{'type': 'Ready', 'status': 'True'}]}}
            if kind == 'service':
                self.assertEqual((name, namespace), ('lsd-genai-playground-service', 'signal-desk-dev'))
                raise RuntimeError('OpenShift command failed: get service')
            self.fail('Preparation continued after a missing backend')

        with patch.object(module, 'get', get), patch.object(module, 'oc') as oc, \
                patch.object(module, 'apply') as apply, patch.object(module, 'request') as request:
            with self.assertRaisesRegex(RuntimeError, 'missing in signal-desk-dev.*CLI deployment in Lab 1.8'):
                module.main()
            oc.assert_not_called()
            apply.assert_not_called()
            request.assert_not_called()
