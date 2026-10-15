#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"

if [[ "${CONFIRM:-}" != "yes" ]]; then
  echo "Refusing to delete. Re-run: make delete CONFIRM=yes" >&2
  exit 1
fi

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Missing ${ENV_FILE}. Nothing to delete." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

echo "Deleting ARO HCP cluster ${CLUSTER_NAME} in ${CUSTOMER_RG_NAME}"

# The service rejects deletion of the last node pool. Cluster deletion removes it.
if "${AZ}" aro hcp cluster show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --name "${CLUSTER_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  echo "Deleting cluster ${CLUSTER_NAME}"
  "${AZ}" aro hcp cluster delete \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --name "${CLUSTER_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --yes
fi

if [[ "$("${AZ}" group exists --name "${MANAGED_RESOURCE_GROUP}" --subscription "${SUBSCRIPTION_ID}" --output tsv)" == "true" ]]; then
  echo "Deleting managed resource group ${MANAGED_RESOURCE_GROUP}"
  "${AZ}" group delete \
    --name "${MANAGED_RESOURCE_GROUP}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --yes
fi

if [[ "${KEEP_RESOURCE_GROUP:-}" == "yes" ]]; then
  echo "Keeping resource group ${CUSTOMER_RG_NAME}"
  if "${AZ}" keyvault show \
    --name "${KEYVAULT_NAME}" \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --query name --output tsv >/dev/null 2>&1; then
    echo "Deleting Key Vault ${KEYVAULT_NAME}"
    "${AZ}" keyvault delete \
      --name "${KEYVAULT_NAME}" \
      --resource-group "${CUSTOMER_RG_NAME}" \
      --subscription "${SUBSCRIPTION_ID}"
  fi
elif [[ "$("${AZ}" group exists --name "${CUSTOMER_RG_NAME}" --subscription "${SUBSCRIPTION_ID}" --output tsv)" == "true" ]]; then
  echo "Deleting customer resource group ${CUSTOMER_RG_NAME}"
  "${AZ}" group delete \
    --name "${CUSTOMER_RG_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --yes
fi

if "${AZ}" keyvault list-deleted \
  --subscription "${SUBSCRIPTION_ID}" \
  --query "[?name=='${KEYVAULT_NAME}'].name" \
  --output tsv | grep -qx "${KEYVAULT_NAME}"; then
  echo "Purging soft-deleted Key Vault ${KEYVAULT_NAME}"
  "${AZ}" keyvault purge \
    --name "${KEYVAULT_NAME}" \
    --location "${LOCATION}" \
    --subscription "${SUBSCRIPTION_ID}" \
    || echo "Key Vault ${KEYVAULT_NAME} could not be purged. Delete it later from the soft-delete list." >&2
fi

delete_app() {
  local app_id="$1"
  if [[ -n "${app_id}" ]] && "${AZ}" ad app show --id "${app_id}" --query appId --output tsv >/dev/null 2>&1; then
    echo "Deleting Entra application ${app_id}"
    "${AZ}" ad app delete --id "${app_id}"
  fi
}

delete_app "${CONSOLE_APP_ID:-}"
delete_app "${CLI_APP_ID:-}"

rm -f "${ENV_FILE}"
echo "DELETE_DONE"
