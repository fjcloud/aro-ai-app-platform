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

class AuthorinoBootstrap(unittest.TestCase):
    def test_service_can_exist_before_deployment(self):
        with tempfile.TemporaryDirectory() as directory:
            tmp=Path(directory);log=tmp/'calls.jsonl';counter=tmp/'counter'
            cli=tmp/'oc';cli.write_text('''#!/usr/bin/env python3
import json,os,pathlib,sys
args=[a for a in sys.argv[1:] if not a.startswith('--request-timeout=')]
with open(os.environ['CALL_LOG'],'a') as f:f.write(json.dumps(args)+'\\n')
if args[:2]==['get','deployment/authorino']:
 p=pathlib.Path(os.environ['COUNTER']);n=int(p.read_text())+1 if p.exists() else 1;p.write_text(str(n));sys.exit(0 if n>=2 else 1)
if args[:2]==['get','authorino']:
 print(json.dumps({'spec':{'volumes':{'items':[{'name':'existing','mountPath':'/existing','configMaps':['existing']}]}}}));sys.exit(0)
if not pathlib.Path(os.environ['COUNTER']).exists() or int(pathlib.Path(os.environ['COUNTER']).read_text())<2:sys.exit('Configured before Deployment existed')
''');cli.chmod(0o755)
            sleep=tmp/'sleep';sleep.write_text('#!/bin/sh\nexit 0\n');sleep.chmod(0o755)
            env=dict(os.environ,PATH=str(tmp)+os.pathsep+os.environ['PATH'],CALL_LOG=str(log),COUNTER=str(counter))
            result=subprocess.run(['bash',str(ROOT/'deploy/instances/configure-authorino-trust.sh')],env=env,text=True,capture_output=True,timeout=10)
            self.assertEqual(result.returncode,0,result.stderr)
            calls=[json.loads(x) for x in log.read_text().splitlines()]
            self.assertEqual(calls[:2],[['get','deployment/authorino','-n','kuadrant-system']]*2)
            self.assertTrue(any('set' in c and 'env' in c for c in calls))
            patch=next(json.loads(c[c.index('-p')+1]) for c in calls if c[:2]==['patch','authorino'])
            volumes=patch['spec']['volumes']['items']
            self.assertEqual({v['name'] for v in volumes},{'existing','workshop-service-ca'})
            self.assertNotIn('listener',patch['spec'])


if __name__ == '__main__':
    unittest.main()
