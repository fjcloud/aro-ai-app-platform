#!/usr/bin/env python3
"""Live read-only acceptance: real app, MaaS tool calling and monitoring evidence.

Makes billable model calls. Requires an authenticated OpenShift user token.
This does not prove microphone inference, Playground UI or MLflow UI visibility.
"""
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request


def oc(*args):
    result = subprocess.run(['oc', *args], capture_output=True, text=True, timeout=45)
    if result.returncode:
        raise RuntimeError('OpenShift prerequisite failed')
    return result.stdout.strip()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, msg, headers, newurl):
        return None


def main():
    host = oc('get', 'route', 'live-transcript', '-n', 'live-transcript-dev', '-o', 'jsonpath={.spec.host}')
    token = oc('whoami', '-t')
    if not host or not token:
        raise RuntimeError('App route and an OpenShift user token are required')
    base = 'https://' + host
    opener = urllib.request.build_opener(NoRedirect)
    # Credentials must never be forwarded through a redirect or appear in output.
    request = urllib.request.Request(base + '/api/chat', data=b'{"question":"health?"}',
                                     headers={'Content-Type':'application/json'})
    try:
        opener.open(request, timeout=15)
    except urllib.error.HTTPError as error:
        if error.code not in (302, 303, 401, 403):
            raise RuntimeError('Unexpected unauthenticated app response') from None
    else:
        raise RuntimeError('Cluster health endpoint is publicly accessible')
    report = []
    tests = [
        ('Are the OpenShift nodes ready? Query the current measurements.', 'nodes_ready', False),
        ('Use query_metric to show cpu_percent over 1h, with units and timestamps.', 'cpu_percent', True),
        ('Use query_metric to show gpu_memory_gib for Whisper. Explain missing data.', 'gpu_memory_gib', False),
    ]
    for question, metric, trend in tests:
        request = urllib.request.Request(base + '/api/chat', data=json.dumps({'question':question}).encode(),
                                         headers={'Authorization':'Bearer ' + token, 'Content-Type':'application/json'})
        start = time.monotonic()
        with opener.open(request, timeout=110) as response:
            result = json.load(response)
        evidence = [e for e in result.get('evidence',[]) if e.get('metric')==metric]
        if not result.get('answer') or not evidence or not re.fullmatch(r'[a-f0-9]{32}', result.get('trace_id','')):
            raise RuntimeError('Answer is missing tool evidence or trace ID')
        if any(e['status']=='error' for e in evidence):
            raise RuntimeError('Monitoring query failed for ' + metric)
        if metric!='gpu_memory_gib' and not any(e['status']=='ok' for e in evidence):
            raise RuntimeError('Required monitoring series missing for ' + metric)
        if trend and not any(e.get('data',{}).get('resultType')=='matrix' for e in evidence):
            raise RuntimeError('CPU trend did not execute a range query')
        report.append({'metric':metric,'statuses':[e['status'] for e in evidence],
                       'seconds':round(time.monotonic()-start,2),'trace_id':result['trace_id']})
    print(json.dumps({'passed':True,'scope':'LIVE app + MaaS + monitoring; validate microphone, MCP Playground and MLflow UI separately',
                      'checks':report},indent=2))


if __name__=='__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        sys.exit('FAIL: Cluster Voice HTTP ' + str(error.code) + ' (response body withheld)')
    except Exception as error:
        sys.exit('FAIL: Cluster Voice (' + type(error).__name__ + '). Check app readiness, authentication, runtime key and metrics access.')
