#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
PULL_SECRET_FILE="${PULL_SECRET_FILE:-${HOME}/pull_secret.json}"
NP_VM_SIZE="${NP_VM_SIZE:-Standard_D8s_v3}"
NP_REPLICAS="${NP_REPLICAS:-3}"

if ! "${AZ}" account show --query id --output tsv >/dev/null 2>&1; then
  echo "Not logged in to Azure. Run az login against the subscription that should own the cluster." >&2
  exit 1
fi

if ! "${AZ}" aro hcp cluster --help >/dev/null 2>&1; then
  echo "The ARO extension is missing. Install it with: az extension add --name aro" >&2
  exit 1
fi

if [[ ! -f "${PULL_SECRET_FILE}" ]]; then
  echo "Pull secret file not found: ${PULL_SECRET_FILE}" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Preparing network, identities, and Key Vault"
  "${ROOT}/scripts/lib/prepare.sh"
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

if ! "${AZ}" aro hcp cluster show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --name "${CLUSTER_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  echo "Creating cluster ${CLUSTER_NAME}"
  "${ROOT}/scripts/lib/create-cluster.sh"
else
  echo "Cluster ${CLUSTER_NAME} already exists"
fi

if ! "${AZ}" aro hcp cluster nodepool show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name "${NP_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  echo "Creating node pool ${NP_NAME} (${NP_REPLICAS} x ${NP_VM_SIZE})"
  "${AZ}" aro hcp cluster nodepool create \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --cluster-name "${CLUSTER_NAME}" \
    --name "${NP_NAME}" \
    --location "${LOCATION}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --subnet-id "${SUBNET_ID}" \
    --vm-size "${NP_VM_SIZE}" \
    --replicas "${NP_REPLICAS}" \
    --version "${NP_VERSION}" \
    --channel-group stable \
    --query "{name:name,state:properties.provisioningState,replicas:properties.replicas}" \
    --output json
else
  echo "Node pool ${NP_NAME} already exists"
fi

"${ROOT}/scripts/lib/configure.sh"
"${ROOT}/scripts/status.sh"
