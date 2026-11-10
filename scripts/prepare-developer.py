#!/usr/bin/env python3
"""Prepare app namespaces and permissions for the native Dev Spaces user identity."""
import argparse
import json
import re
import subprocess
import time


def oc(*args):
    result = subprocess.run(['oc', *args], capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError('OpenShift prerequisite call failed: ' + args[0])
    return result.stdout.strip()


def apply(manifest):
    result = subprocess.run(['oc', 'apply', '-f', '-'], input=json.dumps(manifest),
                            capture_output=True, text=True, timeout=60)
    if result.returncode:
        # The input can contain the scoped token; do not echo subprocess bodies.
        raise RuntimeError('Could not apply ' + manifest['kind'] + '/' + manifest['metadata']['name'])


def metadata(name, namespace=None):
    return {'name': name, **({'namespace': namespace} if namespace else {})}


def prepare(app):
    if not re.fullmatch(r'[a-z][a-z0-9-]{0,38}[a-z0-9]', app) or app.startswith(('openshift', 'kube-')):
        raise ValueError('Use a lowercase workshop app name, 2–40 characters')
    build, dev = app + '-build', app + '-dev'
    for ns in (build, dev):
        m = metadata(ns)
        m['labels'] = {'workshop.fjcloud.io/app': app}
        if ns == dev:
            m['labels']['opendatahub.io/dashboard'] = 'true'
        apply({'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': m})
    # Let OpenShift allocate UID/MCS first. Pre-setting supplemental-groups on a
    # new namespace makes the allocator skip its other security annotations.
    for ns in (build, dev):
        for attempt in range(60):
            if oc('get', 'namespace', ns, '-o',
                  'jsonpath={.metadata.annotations.openshift\\.io/sa\\.scc\\.uid-range}'):
                break
            time.sleep(1)
        else:
            raise RuntimeError('Namespace security allocation did not complete: ' + ns)
    # FSGroup must fit in the container's 65536-ID Linux user namespace.
    oc('annotate', 'namespace', build, 'openshift.io/sa.scc.supplemental-groups=1000/10000', '--overwrite')
    apply({'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': metadata('pipeline', build)})
    # Dev Spaces injects the OpenShift browser user's token for oc/kubectl.
    # Keep this user aligned with Act 0 and deploy/developer/dev-user-access.yaml.
    subject = {'kind': 'User', 'apiGroup': 'rbac.authorization.k8s.io', 'name': 'dev'}
    for ns in (build, dev):
        apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding',
               'metadata': metadata('workshop-developer-edit', ns), 'subjects': [subject],
               'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'ClusterRole', 'name': 'edit'}})
        # Explicit custom-resource permissions; never grant RBAC or SCC mutation.
        apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role',
               'metadata': metadata('workshop-delivery', ns), 'rules': [
                   {'apiGroups': ['argoproj.io'], 'resources': ['argocds', 'applications', 'appprojects'],
                    'verbs': ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete']},
                   {'apiGroups': ['tekton.dev'], 'resources': ['pipelines', 'pipelineruns', 'taskruns', 'tasks'],
                    'verbs': ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete']} ]})
        apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding',
               'metadata': metadata('workshop-delivery', ns), 'subjects': [subject],
               'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'Role', 'name': 'workshop-delivery'}})
    apply({'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding',
           'metadata': metadata('application-image-puller', build),
           'subjects': [{'kind': 'ServiceAccount', 'name': 'default', 'namespace': dev}],
           'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'ClusterRole', 'name': 'system:image-puller'}})
    # The Pipelines operator provisions pipeline's standard pipelines-scc and image-builder binding.
    # Do not grant privileged, anyuid, cluster-admin, or a custom SCC here.
    # Retire the previous shared CLI credentials and their KUBECONFIG override.
    # Remove both the global source and any copy in this workshop user's project.
    # An absent workspace project is normal before the first workspace launch.
    for ns in ('openshift-operators', 'dev-devspaces'):
        oc('delete', 'secret/workshop-developer-access', 'configmap/workshop-developer-env',
           '-n', ns, '--ignore-not-found')
    oc('delete', 'serviceaccount/workshop-developer', '-n', dev, '--ignore-not-found')
    print('Developer permissions prepared for dev in ' + build + ' and ' + dev + '.')
    print('Start or restart Dev Spaces; oc uses the automatically injected user credentials.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--app-name', default='card-scout')
    args = parser.parse_args()
    try:
        prepare(args.app_name)
    except Exception as error:
        raise SystemExit('Developer preparation failed: ' + str(error))
