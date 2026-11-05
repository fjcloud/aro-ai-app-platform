#!/usr/bin/env python3
"""Shared OpenShift, MaaS and MLflow access for Card Scout."""
import importlib.util
import json
from pathlib import Path
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('platform', ROOT / 'scripts/prepare-decision-model.py')
platform = importlib.util.module_from_spec(spec)
spec.loader.exec_module(platform)
oc, get, apply = platform.oc, platform.get, platform.apply
APP, BUILD, IDENTITY = platform.APP, platform.BUILD, 'card-scout'


def request(url, token, body=None, workspace=None):
    headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'}
    if workspace:
        headers['X-MLFLOW-WORKSPACE'] = workspace
    req = urllib.request.Request(url, data=None if body is None else json.dumps(body).encode(), headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=45) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        raise RuntimeError('Platform API returned HTTP ' + str(error.code) + '; check readiness and permissions') from None


def binding(name, role, namespace=None, subject=IDENTITY, role_kind='ClusterRole'):
    meta = {'name': name}
    if namespace:
        meta['namespace'] = namespace
    apply({'apiVersion': 'rbac.authorization.k8s.io/v1',
           'kind': 'RoleBinding' if namespace else 'ClusterRoleBinding', 'metadata': meta,
           'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': role_kind, 'name': role},
           'subjects': [{'kind': 'ServiceAccount', 'name': subject, 'namespace': APP}]})

