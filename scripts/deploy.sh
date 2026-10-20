#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
PULL_SECRET_FILE="${PULL_SECRET_FILE:-${HOME}/pull_secret.json}"
NP_VM_SIZE="${NP_VM_SIZE:-Standard_D4s_v3}"
NP_REPLICAS="${NP_REPLICAS:-2}"

# Fail before provisioning when shell credentials or console prerequisites are missing.
bash "${ROOT}/scripts/onboard.sh"

if ! "${AZ}" account show --query id --output tsv >/dev/null 2>&1; then
  echo "Not logged in to Azure. Run az login against the subscription that should own the cluster." >&2
  exit 1
fi

if ! "${AZ}" aro hcp cluster create -h 2>&1 | grep -- '--operators-authentication' >/dev/null; then
  echo "This az aro hcp command is the published aro extension. It does not accept --operators-authentication." >&2
  echo "Refresh the workshop page and press Start so the terminal uses the aro-hcp extension." >&2
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

if [[ "${LOCATION}" != "eastus2" ]]; then
  echo "This workshop deploys only in East US 2 (eastus2). state/cluster.env has LOCATION=${LOCATION}." >&2
  echo "Remove state/cluster.env and run the prerequisite step again." >&2
  exit 1
fi

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

pool_show() {
  "${AZ}" aro hcp cluster nodepool show \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --cluster-name "${CLUSTER_NAME}" \
    --name "${NP_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    "$@"
}

create_pool() {
  echo "Creating node pool ${NP_NAME} (${NP_REPLICAS} x ${NP_VM_SIZE}, 4 vCPUs each)"
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
    --query "{name:name,state:properties.provisioningState,replicas:properties.replicas,vmSize:properties.platform.vmSize}" \
    --output json
}

if ! pool_show --query name --output tsv >/dev/null 2>&1; then
  create_pool
else
  current_size="$(pool_show --query "properties.platform.vmSize" --output tsv)"
  current_replicas="$(pool_show --query "properties.replicas" --output tsv)"
  if [[ "${current_size}" != "${NP_VM_SIZE}" ]]; then
    echo "Replacing node pool ${NP_NAME} (${current_replicas} x ${current_size}) with ${NP_REPLICAS} x ${NP_VM_SIZE}"
    "${AZ}" aro hcp cluster nodepool delete \
      --resource-group "${CUSTOMER_RG_NAME}" \
      --cluster-name "${CLUSTER_NAME}" \
      --name "${NP_NAME}" \
      --subscription "${SUBSCRIPTION_ID}" \
      --yes
    create_pool
  elif [[ "${current_replicas}" != "${NP_REPLICAS}" ]]; then
    echo "Scaling node pool ${NP_NAME} from ${current_replicas} to ${NP_REPLICAS} x ${NP_VM_SIZE}"
    "${AZ}" aro hcp cluster nodepool update \
      --resource-group "${CUSTOMER_RG_NAME}" \
      --cluster-name "${CLUSTER_NAME}" \
      --name "${NP_NAME}" \
      --subscription "${SUBSCRIPTION_ID}" \
      --replicas "${NP_REPLICAS}" \
      --query "{name:name,state:properties.provisioningState,replicas:properties.replicas,vmSize:properties.platform.vmSize}" \
      --output json
  else
    echo "Node pool ${NP_NAME} already has ${NP_REPLICAS} x ${NP_VM_SIZE}"
  fi
fi

"${ROOT}/scripts/lib/configure.sh"
"${ROOT}/scripts/status.sh"
