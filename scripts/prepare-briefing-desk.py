#!/usr/bin/env python3
"""Facilitator setup for the Signal Desk experiment on a prepared ARO lab."""
import base64
import importlib.util
import json
import urllib.error
import urllib.request
from pathlib import Path

spec = importlib.util.spec_from_file_location('decision', Path(__file__).with_name('prepare-decision-tetris.py'))
decision = importlib.util.module_from_spec(spec)
spec.loader.exec_module(decision)


def main():
    d = decision
    runtime = d.get('inferenceservice', 'd3-mini', d.NS)
    if not any(c['type'] == 'Ready' and c['status'] == 'True' for c in runtime.get('status', {}).get('conditions', [])):
        raise RuntimeError('The existing local d3-mini model must be Ready')
    d.get('service', 'lsd-genai-playground-service', d.APP)
    d.prepare_app('briefing-desk')
    domain = d.get('ingress.config.openshift.io', 'cluster')['spec']['domain']
    token = d.oc('create', 'token', 'decision-tetris', '-n', d.APP, '--duration=10m')
    body = {'name': 'signal-desk-' + str(__import__('time').time_ns()),
            'description': 'Signal Desk workshop runtime',
            'subscription': 'deepseekv4pro-cluster-voice', 'expiresIn': '24h'}
    request = urllib.request.Request('https://maas.' + domain + '/maas-api/v1/api-keys',
              data=json.dumps(body).encode(), headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            result = json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError('MaaS runtime key creation failed: HTTP ' + str(error.code)) from None
    key = result.get('key') or result.get('apiKey')
    if not key:
        raise RuntimeError('MaaS response did not contain a runtime key')
    d.apply({'apiVersion': 'v1', 'kind': 'Secret', 'metadata': {'name': 'briefing-desk-model', 'namespace': d.APP},
             'type': 'Opaque', 'data': {'api-key': base64.b64encode(key.encode()).decode()}})
    d.apply({'apiVersion': 'v1', 'kind': 'PersistentVolumeClaim', 'metadata': {'name': 'briefing-desk-data', 'namespace': d.APP},
             'spec': {'accessModes': ['ReadWriteOnce'], 'storageClassName': 'managed-csi',
                      'resources': {'requests': {'storage': '2Gi'}}}})
    print('Signal Desk prepared: local T4 reused, native agent verified, runtime key valid for 24 hours, persistent storage requested.')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, urllib.error.URLError) as error:
        raise SystemExit(str(error)) from None
