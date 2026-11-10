"""Read-only platform configuration must not obstruct a fresh developer home."""
import json
from pathlib import Path, PurePosixPath
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[1]


class WorkspaceConfig(unittest.TestCase):
    def resources(self):
        return {r['metadata']['name']: r for r in yaml.safe_load_all(
            (ROOT / 'deploy/instances/opencode-workspace-config.yaml').read_text())}

    def test_model_configuration_mount_does_not_create_parents_under_writable_home(self):
        model = self.resources()['opencode-workspace-config']
        mount = PurePosixPath(model['metadata']['annotations']['controller.devfile.io/mount-path'])
        self.assertTrue(mount.is_absolute())
        self.assertNotEqual(mount, PurePosixPath('/home/user'))
        self.assertNotIn(PurePosixPath('/home/user'), mount.parents)
        self.assertNotIn(PurePosixPath('/tmp/opencode-home'), mount.parents)
        self.assertEqual(model['metadata']['annotations']['controller.devfile.io/mount-as'], 'subpath')

    def test_opencode_selects_the_mounted_file_and_keeps_credentials_in_environment(self):
        resources = self.resources()
        env = resources['opencode-xdg-env']['data']
        model = resources['opencode-workspace-config']
        mount = PurePosixPath(model['metadata']['annotations']['controller.devfile.io/mount-path'])
        config = PurePosixPath(env['OPENCODE_CONFIG'])
        self.assertEqual(config.parent, mount)
        self.assertIn(config.name, model['data'])
        payload = json.loads(model['data'][config.name])
        self.assertEqual(payload['model'], 'deepseek/deepseekv4pro')
        self.assertEqual(payload['provider']['deepseek']['options']['apiKey'], '{env:OPENAI_API_KEY}')
        self.assertEqual(env['XDG_DATA_HOME'], '/home/user/.local/share')
        self.assertEqual(env['XDG_STATE_HOME'], '/home/user/.local/state')


if __name__ == '__main__':
    unittest.main()
