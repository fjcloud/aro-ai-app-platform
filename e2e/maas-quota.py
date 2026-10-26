#!/usr/bin/env python3
"""Live quota test: temporary subscription and key, revoked/deleted in finally.

Run as an OpenShift user with permission to manage MaaS subscriptions/API keys.
Creates only deepseek-quota-validation in models-as-a-service; no provider edits.
"""
import base64
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

NAME = 'deepseek-quota-validation'
NS = 'models-as-a-service'


def oc(*args, **kwargs):
    return subprocess.check_output(['oc', *args], text=True, **kwargs).strip()


def main():
    token = oc('whoami', '-t')
    user = oc('whoami')
    secret = json.loads(oc('get', 'secret', 'ai-provider-openai-api-key', '-n', 'openshift-operators', '-o', 'json'))['data']
    endpoint = base64.b64decode(secret['OPENAI_BASE_URL']).decode().removesuffix('/v1')
    def request(method, path, data=None, key=token):
        return urllib.request.urlopen(urllib.request.Request(endpoint + path, method=method,
            data=None if data is None else json.dumps(data).encode(),
            headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key}), timeout=180)
    # Refuse to replace an unrelated existing object.
    exists = subprocess.run(['oc', 'get', 'maassubscription', NAME, '-n', NS, '--ignore-not-found', '-o', 'name'], capture_output=True, text=True, check=True)
    if exists.stdout.strip():
        raise RuntimeError('Validation subscription already exists; inspect it before retrying')
    subscription = {'apiVersion': 'maas.opendatahub.io/v1alpha1', 'kind': 'MaaSSubscription',
        'metadata': {'name': NAME, 'namespace': NS, 'labels': {'workshop.fjcloud.io/purpose': 'quota-validation'}},
        'spec': {'priority': 10, 'owner': {'users': [user], 'groups': []},
                 'modelRefs': [{'name': 'deepseekv4pro', 'namespace': 'llm-serving',
                                'tokenRateLimits': [{'limit': 1, 'window': '1h'}]}]}}
    key_id, created, report = None, False, {}
    try:
        oc('create', '-f', '-', input=json.dumps(subscription))
        created = True
        oc('wait', 'maassubscription/' + NAME, '-n', NS, '--for=condition=Ready', '--timeout=180s')
        with request('POST', '/maas-api/v1/api-keys', {'name': NAME, 'subscription': NAME, 'expiresIn': '10m'}) as r:
            minted = json.load(r)
        key_id = minted['id']
        codes = []
        for _ in range(4):
            try:
                with request('POST', '/v1/chat/completions', {'model': 'deepseekv4pro', 'messages': [
                    {'role': 'user', 'content': 'Say hello.'}], 'max_completion_tokens': 128}, minted['key']) as r:
                    json.load(r)
                    codes.append(r.status)
            except urllib.error.HTTPError as error:
                codes.append(error.code)
                if error.code != 429:
                    raise
                break
            time.sleep(1)
        assert 429 in codes, 'One-token allowance was not enforced'
        report = {'quota_limit': 1, 'quota_window': '1h', 'request_statuses': codes}
    finally:
        try:
            if key_id:
                with request('DELETE', '/maas-api/v1/api-keys/' + key_id) as r:
                    report['key_revoked'] = r.status in (200, 204)
        finally:
            if created:
                oc('delete', 'maassubscription', NAME, '-n', NS, '--wait=true', '--timeout=60s')
                report['temporary_subscription_deleted'] = True
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        sys.exit('Quota probe failed: HTTP ' + str(error.code))
    except Exception as error:
        sys.exit('Quota probe failed: ' + type(error).__name__)
