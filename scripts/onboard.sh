#!/usr/bin/env bash
# Sign in to the existing ARO cluster and validate the workshop environment.
set -euo pipefail
for tool in oc az git jq python3 curl; do
  command -v "$tool" >/dev/null 2>&1 || { echo "Missing $tool. Ask the facilitator to prepare the RHDP bastion." >&2; exit 1; }
done
az account show --output none
python3 "$(dirname "${BASH_SOURCE[0]}")/workshop-env.py" --login
oc whoami -t >/dev/null 2>&1 || { echo 'ARO login did not establish a user session. Rerun make onboard.' >&2; exit 1; }
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
echo 'Ready for Act 1. No cluster, identity, or billing resources were created.'
