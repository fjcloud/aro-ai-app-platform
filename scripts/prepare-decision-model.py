#!/usr/bin/env python3
"""Deploy the prebuilt, pinned d3-mini runtime on the workshop's existing T4."""
import argparse
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
NS, BUILD, APP = 'decision-serving', 'signal-desk-build', 'signal-desk-dev'
IMAGE = (ROOT / 'deploy/decision/runtime-image.txt').read_text().strip()
MODEL_REVISION = 'a475367de0d8133a3c1a2a0dc12303d89cfa2ab4'


def oc(*args, data=None):
    result = subprocess.run(['oc', *args], input=None if data is None else json.dumps(data),
                            capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('OpenShift command failed: ' + ' '.join(args[:3]))
    return result.stdout.strip()


def get(kind, name, namespace=None):
    return json.loads(oc('get', kind, name, *(['-n', namespace] if namespace else []), '-o', 'json'))


def apply(obj):
    oc('apply', '--server-side', '-f', '-', data=obj)


def prepare(reuse_runtime=False):
    oc('apply', '-k', str(ROOT / 'deploy/decision'))
    oc('wait', 'inferenceservice/d3-mini', '-n', NS, '--for=condition=Ready', '--timeout=1800s')
    print('d3-mini is ready on one local T4. No runtime build was needed.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--reuse-runtime', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    try:
        prepare(args.reuse_runtime)
    except RuntimeError as error:
        raise SystemExit(str(error)) from None
