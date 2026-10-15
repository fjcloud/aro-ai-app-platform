#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "No state/cluster.env. Run make deploy." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

echo "==== CLUSTER ===="
"${AZ}" aro hcp cluster show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --name "${CLUSTER_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query "{name:name,state:properties.provisioningState,api:properties.api.visibility,ingress:properties.ingress.type,version:properties.version.id,console:properties.console.url}" \
  --output json

echo "==== NODE POOL ===="
"${AZ}" aro hcp cluster nodepool show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name "${NP_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query "{name:name,state:properties.provisioningState,replicas:properties.replicas,vmSize:properties.platform.vmSize,version:properties.version.id}" \
  --output json

echo "==== GPU NODE POOL ===="
if "${AZ}" aro hcp cluster nodepool show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name "${GPU_NAME:-gpu}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  "${AZ}" aro hcp cluster nodepool show \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --cluster-name "${CLUSTER_NAME}" \
    --name "${GPU_NAME:-gpu}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --query "{name:name,state:properties.provisioningState,replicas:properties.replicas,vmSize:properties.platform.vmSize,version:properties.version.id}" \
    --output json
else
  echo "No gpu node pool yet. Run make gpu."
fi

if [[ -f "${ROOT}/state/kubeconfig" ]]; then
  echo "KUBECONFIG=${ROOT}/state/kubeconfig"
fi
if [[ -n "${CLUSTER_ADMIN_USER:-}" ]]; then
  echo "Console login: ${CLUSTER_ADMIN_USER}"
fi
