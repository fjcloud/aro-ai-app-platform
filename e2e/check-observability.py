#!/usr/bin/env python3
"""Read the workshop dashboard queries through OpenShift Thanos; print no credentials."""
import json
from pathlib import Path
import subprocess
import sys
import urllib.parse
import urllib.request


def oc(*args):
    return subprocess.check_output(['oc', *args], text=True, stderr=subprocess.DEVNULL).strip()


def main():
    dashboard = json.loads((Path(__file__).resolve().parents[1] / 'deploy/observability/workshop-ai-dashboard.json').read_text())
    host = oc('get', 'route', 'thanos-querier', '-n', 'openshift-monitoring', '-o', 'jsonpath={.spec.host}')
    token = oc('whoami', '-t')
    if not token:
        raise RuntimeError('Use a logged-in OpenShift user with monitoring access')
    report = []
    for panel in dashboard['panels']:
        query = panel['targets'][0]['expr']
        request = urllib.request.Request('https://' + host + '/api/v1/query?' + urllib.parse.urlencode({'query': query}),
                                         headers={'Authorization': 'Bearer ' + token})
        with urllib.request.urlopen(request, timeout=45) as response:
            data = json.load(response)
        results = data.get('data', {}).get('result', [])
        if data.get('status') != 'success' or not results:
            raise RuntimeError('No collected series: ' + panel['title'])
        if 'Scrape health' in panel['title'] and (
                {r['metric'].get('namespace') for r in results} != {'decision-serving', 'kuadrant-system'}
                or any(r['value'][1] != '1' for r in results)):
            raise RuntimeError('A workshop scrape target is down')
        report.append({'panel': panel['title'], 'query': query, 'series': len(results),
                       'samples': [{'labels': {k: v for k, v in r['metric'].items() if k in (
                           'model', 'subscription', 'namespace', 'modelName', 'gpu')},
                           'value': r['value'][1]} for r in results]})
    print(json.dumps({'checks': report, 'note': 'Latency can be NaN when no requests completed in the last five minutes.'}, indent=2))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Never include request headers, tokens or HTTP response bodies.
        sys.exit('FAIL: observability check (' + type(error).__name__ + '). Check login, model readiness, telemetry and scrape propagation.')
