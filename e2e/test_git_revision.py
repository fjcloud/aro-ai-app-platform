"""Run the actual Tekton clone script against a real local Git repository."""
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class ExactGitRevision(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / 'source repo'
        self.repo.mkdir()
        self.git('init', '-b', 'main')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'user.name', 'test')
        (self.repo / 'value').write_text('main')
        self.git('add', 'value')
        self.git('commit', '-m', 'main')
        self.sha = self.git('rev-parse', 'HEAD').strip()
        self.git('switch', '-c', 'feature')
        (self.repo / 'value').write_text('feature')
        self.git('commit', '-am', 'feature')
        self.git('switch', 'main')

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.repo), *args], stderr=subprocess.DEVNULL, text=True)

    def clone(self, revision):
        task = (ROOT / 'deploy/operators/tekton-tasks.yaml').read_text().split('\n---\n')[1]
        script = task.split('      script: |\n', 1)[1]
        script = '\n'.join(line[8:] for line in script.splitlines())
        params = {'workspaces.output.path': str(self.root / 'checkout'), 'params.SUBDIRECTORY': '',
                  'params.DELETE_EXISTING': 'true', 'params.SSL_VERIFY': 'true',
                  'params.DEPTH': '1', 'params.REVISION': revision, 'params.URL': self.repo.as_uri(),
                  'results.commit.path': str(self.root / 'commit'), 'results.url.path': str(self.root / 'url')}
        for key, value in params.items():
            script = script.replace('$(' + key + ')', value)
        return subprocess.run(['sh', '-c', script], capture_output=True, text=True)

    def test_branch_and_exact_sha(self):
        result = self.clone('feature')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'checkout/value').read_text(), 'feature')
        result = self.clone(self.sha)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'commit').read_text(), self.sha)
        self.assertEqual((self.root / 'checkout/value').read_text(), 'main')

    def test_missing_revision_fails_without_default_branch(self):
        result = self.clone('does-not-exist')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / 'commit').exists())
        self.assertFalse((self.root / 'checkout/value').exists())


if __name__ == '__main__':
    unittest.main()
