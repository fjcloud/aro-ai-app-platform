#!/usr/bin/env bash
# Check prerequisites without creating a cluster, identities or resource groups.
set -euo pipefail
CLUSTER_ADMIN_USER="${CLUSTER_ADMIN_USER:-${WORKSHOP_EMAIL:-}}"
if [[ -s "${HOME}/.rhcs.env" ]]; then
  set -a
  source "${HOME}/.rhcs.env"
  set +a
fi
for tool in az oc git make python3; do
  command -v "$tool" >/dev/null || { echo "Missing $tool. Start a new Cloud Terminal." >&2; exit 1; }
done
az account show >/dev/null
az aro hcp cluster create -h 2>&1 | grep -- '--operators-authentication' >/dev/null || {
  echo 'The aro-hcp extension is missing. Start a new Cloud Terminal; do not install the published aro extension.' >&2
  exit 1
}
if [[ -n "${RESOURCEGROUP:-}" ]]; then
  az group show --name "$RESOURCEGROUP" --output none
  echo "Using the assigned resource group: $RESOURCEGROUP"
fi
pull_secret="${PULL_SECRET_FILE:-${HOME}/pull_secret.json}"
if [[ ! -s "$pull_secret" ]]; then
  if command -v workshop-tools >/dev/null 2>&1 && [[ -n "${RHCS_CLIENT_ID:-}" && -n "${RHCS_CLIENT_SECRET:-}" ]]; then
    workshop-tools pull-secret "$pull_secret"
  else
    echo 'Save your Red Hat pull secret to ~/pull_secret.json: https://console.redhat.com/openshift/install/pull-secret' >&2
    exit 1
  fi
fi
python3 - "$pull_secret" <<'PY'
import json, sys
try:
    with open(sys.argv[1]) as stream:
        auths = json.load(stream).get("auths", {})
except (OSError, ValueError):
    sys.exit("Pull secret is unreadable or invalid JSON.")
missing = [r for r in ("registry.redhat.io", "registry.connect.redhat.com", "cloud.openshift.com") if r not in auths]
if missing:
    sys.exit("Pull secret is missing: " + ", ".join(missing))
PY
if [[ "$(az account show --query user.type -o tsv)" == "servicePrincipal" && -z "${CLUSTER_ADMIN_USER:-}" ]]; then
  echo 'The terminal uses a service principal. Set CLUSTER_ADMIN_USER to the Entra user email that will sign in to the cluster console.' >&2
  echo 'The principal must also be allowed to create Entra applications; subscription Contributor alone does not grant this permission.' >&2
  exit 1
fi
if [[ -n "${CLUSTER_ADMIN_USER:-}" ]]; then
  az ad user show --id "$CLUSTER_ADMIN_USER" --query userPrincipalName --output tsv >/dev/null || {
    echo 'The console administrator must exist in the lab Entra tenant. Ask the facilitator to supply a tenant user or invite the workshop email.' >&2
    exit 1
  }
fi
echo 'Azure identity, ARO extension, OpenShift CLI and pull secret are ready. No cluster resources were created.'
