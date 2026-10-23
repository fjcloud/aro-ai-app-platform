#!/usr/bin/env bash
# Validate an existing workshop environment without changing cloud or cluster resources.
set -euo pipefail
for tool in oc az git jq python3 curl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Missing $tool. Ask the facilitator to prepare the RHDP bastion." >&2; exit 1; }
done
az account show --output none
oc whoami >/dev/null 2>&1 || {
  echo 'Log in to the existing cluster using the RHDP access details or the console Copy login command.' >&2
  exit 1
}
[[ "$(oc auth can-i '*' '*' --all-namespaces)" == yes ]] || {
  echo 'The platform labs require cluster-admin access. Ask the facilitator for the assigned administrator login.' >&2
  exit 1
}
oc get nodes -o json | python3 -c '
import json,sys
nodes=json.load(sys.stdin).get("items",[])
workers=[n for n in nodes if "node-role.kubernetes.io/worker" in n.get("metadata",{}).get("labels",{})]
if not workers: sys.exit("No worker nodes found. Ask the facilitator to check the existing cluster.")
if any(not any(c.get("type")=="Ready" and c.get("status")=="True" for c in n.get("status",{}).get("conditions",[])) for n in workers):
    sys.exit("Workers are not all Ready. Wait or ask the facilitator before continuing.")
print("Existing cluster access verified; workers are Ready.")'
oc get clusterversion version -o json | python3 -c '
import json,re,sys
version=json.load(sys.stdin).get("status",{}).get("desired",{}).get("version","")
m=re.match(r"^(\d+)\.(\d+)\.",version)
if not m or tuple(map(int,m.groups())) < (4,21):
    sys.exit("This guide targets OpenShift 4.21 or later; the existing cluster reports "+(version or "an unknown version")+". Ask the facilitator for a compatible RHDP environment before Act 1.")
print("OpenShift version:",version)'
python3 "$(dirname "${BASH_SOURCE[0]}")/workshop-env.py"
echo 'Ready for Act 1. No cluster, identity, or billing resources were created.'
