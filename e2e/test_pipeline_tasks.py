"""Task adaptation must preserve vendor scripts, images and security settings."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("tasks", ROOT / "scripts/install-pipeline-tasks.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PipelineTaskAdaptation(unittest.TestCase):
    def test_preserves_vendor_execution_and_input(self):
        task = {"apiVersion": "tekton.dev/v1", "metadata": {"name": "buildah"},
                "spec": {"workspaces": [{"name": "source"}, {"name": "ssl"}],
                         "params": [{"name": "CONTEXT", "default": "."}],
                         "steps": [{"name": "build", "workingDir": "$(workspaces.source.path)",
                                    "image": "registry.redhat.io/rhel9/buildah@sha256:example",
                                    "script": "vendor build and push", "securityContext": {"runAsNonRoot": True}}]}}
        import copy
        original = copy.deepcopy(task)
        output = module.adapt(task, "source")
        self.assertEqual(task, original)
        actual = output["spec"]["steps"][0]
        for key in ("image", "script", "securityContext"):
            self.assertEqual(actual[key], task["spec"]["steps"][0][key])
        self.assertEqual(output["spec"]["params"], task["spec"]["params"])
        self.assertEqual(output["spec"]["workspaces"][1], {"name": "ssl"})
        self.assertEqual(output["metadata"]["name"], "workshop-buildah")
