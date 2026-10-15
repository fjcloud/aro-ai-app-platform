#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

GPU_NAME="${GPU_NAME:-gpu}"
GPU_VM_SIZE="${GPU_VM_SIZE:-Standard_NC24ads_A100_v4}"
GPU_REPLICAS="${GPU_REPLICAS:-1}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "No state/cluster.env. Run make deploy." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

if "${AZ}" aro hcp cluster nodepool show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name "${GPU_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  echo "Node pool ${GPU_NAME} already exists"
  exit 0
fi

echo "Creating node pool ${GPU_NAME} (${GPU_REPLICAS} x ${GPU_VM_SIZE})"
echo "This VM family needs Standard NCADS_A100_v4 Family vCPU quota in ${LOCATION} (24 vCPUs for one node)."
"${AZ}" aro hcp cluster nodepool create \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name "${GPU_NAME}" \
  --location "${LOCATION}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --subnet-id "${SUBNET_ID}" \
  --vm-size "${GPU_VM_SIZE}" \
  --replicas "${GPU_REPLICAS}" \
  --version "${NP_VERSION}" \
  --channel-group stable \
  --taints "[{key:nvidia.com/gpu,value:present,effect:NoSchedule}]" \
  --query "{name:name,state:properties.provisioningState,replicas:properties.replicas,vmSize:properties.platform.vmSize}" \
  --output json
