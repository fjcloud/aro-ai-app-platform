"""The browser persona must stay separate from platform administration."""
from pathlib import Path
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[1]


class BrowserDeveloperAccess(unittest.TestCase):
    def resources(self):
        return list(yaml.safe_load_all((ROOT / 'deploy/developer/dev-user-access.yaml').read_text()))

    def test_workspace_is_discoverable_and_owned_by_the_dev_persona(self):
        namespaces = [r for r in self.resources() if r['kind'] == 'Namespace']
        self.assertEqual(len(namespaces), 1)
        meta = namespaces[0]['metadata']
        self.assertEqual(meta['name'], 'dev-devspaces')
        self.assertEqual(meta['annotations'], {'che.eclipse.org/username': 'dev'})
        self.assertEqual(meta['labels']['app.kubernetes.io/part-of'], 'che.eclipse.org')
        self.assertEqual(meta['labels']['app.kubernetes.io/component'], 'workspaces-namespace')

    def test_browser_grants_are_namespaced_and_do_not_grant_cluster_admin_or_scc(self):
        allowed = {'dev-devspaces': 'admin', 'card-scout-dev': 'edit', 'card-scout-build': 'edit'}
        bindings = [r for r in self.resources() if r['kind'] == 'RoleBinding']
        self.assertEqual({r['metadata']['namespace'] for r in bindings}, set(allowed))
        self.assertTrue(all(r['kind'] in ('Namespace', 'RoleBinding') for r in self.resources()))
        for r in bindings:
            self.assertEqual(r['roleRef']['name'], allowed[r['metadata']['namespace']])
            self.assertEqual(r['subjects'], [{'kind': 'User', 'apiGroup': 'rbac.authorization.k8s.io', 'name': 'dev'}])
            self.assertNotIn('scc', r['roleRef']['name'])


if __name__ == '__main__':
    unittest.main()
