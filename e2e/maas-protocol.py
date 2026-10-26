#!/usr/bin/env python3
"""Billable live protocol probes. No mutations; credentials remain in memory."""
import base64
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request


def get(kind, name, namespace):
    return json.loads(subprocess.check_output(['oc', 'get', kind, name, '-n', namespace, '-o', 'json'], text=True))


def main():
    secret = get('secret', 'ai-provider-openai-api-key', 'openshift-operators')['data']
    endpoint = base64.b64decode(secret['OPENAI_BASE_URL']).decode().rstrip('/')
    key = base64.b64decode(secret['OPENAI_API_KEY']).decode()
    def request(path, payload=None, credential=key):
        headers = {'Content-Type': 'application/json'}
        if credential is not None:
            headers['Authorization'] = 'Bearer ' + credential
        return urllib.request.urlopen(urllib.request.Request(endpoint + path,
            data=None if payload is None else json.dumps(payload).encode(), headers=headers), timeout=180)
    report = {}
    with request('/models') as response:
        models = json.load(response)
    assert any(m['id'] == 'deepseekv4pro' for m in models['data']), 'Model missing from MaaS catalog'
    report['catalog'] = 'deepseekv4pro'
    short = {'model': 'deepseekv4pro', 'messages': [{'role': 'user', 'content': 'Say hello.'}], 'max_completion_tokens': 128}
    for name, credential in [('missing_key', None), ('invalid_key', 'sk-oai-invalid-validation-key')]:
        try:
            with request('/chat/completions', short, credential):
                raise AssertionError('Invalid authentication accepted')
        except urllib.error.HTTPError as error:
            assert error.code in (401, 403), 'Unexpected authentication response'
            report[name] = error.code
    tool = {'type': 'function', 'function': {'name': 'workshop_probe', 'description': 'Record the workshop validation marker.',
            'parameters': {'type': 'object', 'properties': {'marker': {'type': 'string'}}, 'required': ['marker'], 'additionalProperties': False}}}
    body = {**short, 'messages': [{'role': 'user', 'content': 'Call workshop_probe with marker ARO_MAAS_OK.'}],
            'tools': [tool], 'tool_choice': {'type': 'function', 'function': {'name': 'workshop_probe'}}, 'max_completion_tokens': 512}
    with request('/chat/completions', body) as response:
        result = json.load(response)
    calls = result['choices'][0]['message']['tool_calls']
    assert calls[0]['function']['name'] == 'workshop_probe'
    assert json.loads(calls[0]['function']['arguments'])['marker'] == 'ARO_MAAS_OK'
    report['tool_call'] = 'workshop_probe(ARO_MAAS_OK)'
    report['tool_usage'] = result['usage']
    body = {**short, 'stream': True, 'stream_options': {'include_usage': True}, 'messages': [
        {'role': 'system', 'content': 'Reply with exactly ARO_STREAM_OK. Context: ' + 'workshop context. ' * 4096},
        {'role': 'user', 'content': 'Confirm the marker.'}]}
    report['request_bytes'] = len(json.dumps(body).encode())
    start = time.monotonic()
    chunks, text, done, usage = 0, '', False, {}
    with request('/chat/completions', body) as response:
        assert 'text/event-stream' in response.headers.get('Content-Type', ''), 'Not an SSE response'
        for raw in response:
            line = raw.decode().strip()
            if not line.startswith('data: '):
                continue
            if line[6:] == '[DONE]':
                done = True
                break
            event = json.loads(line[6:])
            chunks += 1
            for choice in event.get('choices', []):
                text += choice.get('delta', {}).get('content') or ''
            if event.get('usage'):
                usage = event['usage']
    assert done and chunks > 1 and 'ARO_STREAM_OK' in text, 'Incomplete streamed response'
    assert usage.get('prompt_tokens', 0) > 0 and usage.get('completion_tokens', 0) > 0, 'Missing streamed token usage'
    report.update(streaming_chunks=chunks, streaming_done=done, streaming_usage=usage,
                  streaming_seconds=round(time.monotonic() - start, 2))
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        sys.exit('Protocol probe failed: HTTP ' + str(error.code))
    except Exception as error:
        sys.exit('Protocol probe failed: ' + type(error).__name__)
