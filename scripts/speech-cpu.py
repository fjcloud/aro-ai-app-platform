#!/usr/bin/env python3
"""Plan one extra CPU worker for the speech workshop on classic ARO."""
import argparse
import copy
import json
import subprocess
import sys


def read(*args):
    return json.loads(subprocess.check_output(args, text=True))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--zone', required=True, help='Azure availability zone, matching an existing Dev Spaces PVC if any')
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    subscription = read('az', 'account', 'show', '-o', 'json')['id'].lower()
    nodes = read('oc', 'get', 'nodes', '-o', 'json')['items']
    if any('/subscriptions/' + subscription + '/' not in n['spec'].get('providerID', '').lower() for n in nodes):
        sys.exit('Azure and OpenShift must select the same subscription.')
    items = read('oc', 'get', 'machinesets', '-n', 'openshift-machine-api', '-o', 'json')['items']
    existing = [m for m in items if m['metadata'].get('labels', {}).get('workshop.fjcloud.io/speech-capacity') == 'true']
    if existing:
        print('Speech CPU worker already configured: ' + existing[0]['metadata']['name'])
        return
    candidates = [m for m in items if m['spec']['template']['spec']['providerSpec']['value'].get('zone') == args.zone
                  and m['spec']['template']['spec']['providerSpec']['value'].get('vmSize', '').startswith('Standard_D')]
    if not candidates:
        sys.exit('No classic Azure CPU MachineSet found in that zone.')
    machine = copy.deepcopy(sorted(candidates, key=lambda m: m['metadata']['name'])[0])
    provider = machine['spec']['template']['spec']['providerSpec']['value']
    size = 'Standard_D8s_v3'
    skus = read('az', 'vm', 'list-skus', '-l', provider['location'], '--resource-type', 'virtualMachines', '--all', '-o', 'json')
    sku = next(s for s in skus if s['name'] == size)
    if args.zone not in sku.get('locationInfo', [{}])[0].get('zones', []):
        sys.exit('CPU VM size does not list that availability zone.')
    if any(r['type'] == 'Location' or args.zone in r.get('restrictionInfo', {}).get('zones', []) for r in sku.get('restrictions', [])):
        sys.exit('CPU VM size is unavailable in that zone.')
    quotas = read('az', 'vm', 'list-usage', '-l', provider['location'], '-o', 'json')
    for family in [sku['family'], 'cores']:
        quota = next((q for q in quotas if q['name']['value'].lower() == family.lower()), None)
        if not quota or quota['limit'] - quota['currentValue'] < 8:
            sys.exit('Need 8 unused vCPUs of quota for ' + family)
    infra = machine['spec']['selector']['matchLabels']['machine.openshift.io/cluster-api-cluster']
    name = f"{infra}-speech-cpu-{provider['location']}{args.zone}"
    machine['metadata'] = {'name': name, 'namespace': 'openshift-machine-api', 'labels': {'workshop.fjcloud.io/speech-capacity': 'true'}}
    machine.pop('status', None)
    label = 'machine.openshift.io/cluster-api-machineset'
    machine['spec']['replicas'] = 1
    machine['spec']['selector']['matchLabels'][label] = name
    machine['spec']['template']['metadata']['labels'][label] = name
    provider['vmSize'] = size
    manifest = json.dumps(machine, indent=2) + '\n'
    if args.apply:
        subprocess.run(['oc', 'create', '-f', '-'], input=manifest, text=True, check=True)
    else:
        print(manifest, end='')


if __name__ == '__main__':
    main()
