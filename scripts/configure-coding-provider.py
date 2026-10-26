#!/usr/bin/env python3
"""Keep the Foundry account key in the model namespace, outside Dev Spaces."""
import json
import os
import subprocess
from urllib.parse import urlsplit


def provider_manifest(endpoint):
    url = urlsplit(endpoint)
    if (url.scheme != 'https' or not url.hostname
            or not url.hostname.endswith('.cognitiveservices.azure.com')
            or url.username or url.password or url.port
            or url.path not in ('', '/') or url.query or url.fragment):
        raise ValueError('Expected the HTTPS Foundry account endpoint, without an API path')
    return {
        'apiVersion': 'inference.opendatahub.io/v1alpha1', 'kind': 'ExternalProvider',
        'metadata': {'name': 'foundry', 'namespace': 'llm-serving'},
        'spec': {'provider': 'azure', 'endpoint': url.hostname,
                 'auth': {'type': 'apikey', 'secretRef': {'name': 'foundry-provider-key'}}},
    }


def main():
    provider = provider_manifest(os.environ['ENDPOINT'])
    account = ('https://management.azure.com/subscriptions/' + os.environ['SUBSCRIPTION']
               + '/resourceGroups/' + os.environ['RESOURCEGROUP']
               + '/providers/Microsoft.CognitiveServices/accounts/' + os.environ['FOUNDRY_NAME'])
    # Capture credentials; never print az/oc output containing a key on failure.
    key_result = subprocess.run(['az', 'rest', '--method', 'post', '--url',
                                 account + '/listKeys?api-version=' + os.environ['API'],
                                 '--query', 'key1', '-o', 'tsv'], capture_output=True, text=True, timeout=60)
    if key_result.returncode or not key_result.stdout.strip():
        raise RuntimeError('Cannot read the Foundry key; check Azure permissions and state/foundry.env')
    secret = {'apiVersion': 'v1', 'kind': 'Secret', 'type': 'Opaque',
              'metadata': {'name': 'foundry-provider-key', 'namespace': 'llm-serving',
                           'labels': {'inference.llm-d.ai/ipp-managed': 'true'}},
              'stringData': {'api-key': key_result.stdout.strip()}}
    subprocess.run(['oc', 'apply', '-f', 'deploy/coding/namespace.yaml'], check=True)
    for manifest in (secret, provider):
        result = subprocess.run(['oc', 'apply', '-f', '-'], input=json.dumps(manifest),
                                capture_output=True, text=True, timeout=60)
        if result.returncode:
            raise RuntimeError('Cannot apply the provider Secret or ExternalProvider; check platform readiness')
    print('Foundry provider configured in llm-serving; no Azure key injected into workspaces.')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # No subprocess bodies, environment values or credentials in diagnostics.
        raise SystemExit('Provider setup failed: ' + type(error).__name__ + '. Check state/foundry.env, Azure access and platform readiness.')
