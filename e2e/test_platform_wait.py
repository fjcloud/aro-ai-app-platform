"""Operator readiness failures must prevent instance or database installation."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
FAKE_OC = '''#!/usr/bin/env python3
import json,os,sys
with open(os.environ['CALL_LOG'],'a') as f:f.write(json.dumps(sys.argv[1:])+'\\n')
a=[x for x in sys.argv[1:] if not x.startswith('--request-timeout=')]
if a[:3]==['apply','-k','deploy/operators']:sys.exit(0)
if a[0]=='get' and a[1].startswith('deployment/'):
    sys.exit(1 if os.environ['MODE']=='missing' else 0)
if a[0]=='get' and a[1]=='subscriptions':sys.exit(0)
if a[0]=='wait':sys.exit(1)
sys.exit('Unexpected mutation after a failed readiness gate')
'''


class PlatformWait(unittest.TestCase):
    def test_failed_operator_gate_does_not_apply_instances_or_database(self):
        for mode in ('missing', 'unavailable'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                tmp=Path(directory)
                cli=tmp/'oc';cli.write_text(FAKE_OC);cli.chmod(0o755)
                log=tmp/'calls.jsonl'
                env=dict(os.environ, PATH=str(tmp)+os.pathsep+os.environ['PATH'], MODE=mode,
                         CALL_LOG=str(log), OPERATOR_WAIT_SECONDS='1')
                result=subprocess.run(['bash',str(ROOT/'scripts/install-platform.sh')],env=env,
                                      text=True,capture_output=True,timeout=12)
                self.assertNotEqual(result.returncode,0)
                calls=[json.loads(x) for x in log.read_text().splitlines()]
                self.assertEqual([a for a in calls if a[0]=='apply'],[['apply','-k','deploy/operators']])
                self.assertFalse(any('scale' in a or 'delete' in a for a in calls))
                if mode=='missing':self.assertIn('was not created within 1 seconds',result.stderr)


if __name__ == '__main__':
    unittest.main()
