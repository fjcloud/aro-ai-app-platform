#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

az() {
  command "${AZ}" "$@"
}

LOCATION="${LOCATION:-westeurope}"
SUBSCRIPTION_ID="$(az account show --query id --output tsv)"
CLUSTER_NAME="${CLUSTER_NAME:-aro-ai-weu}"
if [[ -n "${CUSTOMER_RG_NAME:-}" ]]; then
  KEEP_RESOURCE_GROUP=yes
else
  CUSTOMER_RG_NAME="${CLUSTER_NAME}-rg"
  KEEP_RESOURCE_GROUP=no
fi
CUSTOMER_NSG="${CLUSTER_NAME}-nsg"
CUSTOMER_VNET_NAME="${CLUSTER_NAME}-vnet"
CUSTOMER_VNET_SUBNET1="workers"
CUSTOMER_VNET_INTEGRATION_SUBNET_NAME="vnet-integration"
MANAGED_RESOURCE_GROUP="${CLUSTER_NAME}-managed-rg"
NP_NAME="workers"
CLUSTER_VERSION="${CLUSTER_VERSION:-4.22}"
NP_VERSION="${NP_VERSION:-4.22.15}"
KEYVAULT_NAME="kv-${CLUSTER_NAME}-$(openssl rand -hex 3)"
KMS_KEY_NAME="etcd-kms-key"

assign_role() {
  local pid="$1" role="$2" scope="$3"
  local attempt
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
    if az role assignment create \
      --assignee-object-id "${pid}" \
      --assignee-principal-type ServicePrincipal \
      --role "${role}" \
      --scope "${scope}" \
      --only-show-errors \
      --output none; then
      return 0
    fi
    echo "retry ${attempt}: role ${role} on ${scope}" >&2
    sleep 15
  done
  echo "failed role ${role} on ${scope}" >&2
  return 1
}

echo "subscription ${SUBSCRIPTION_ID} location ${LOCATION} keyvault ${KEYVAULT_NAME}"

az group create \
  --name "${CUSTOMER_RG_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --location "${LOCATION}" \
  --output none

if ! az network nsg show --name "${CUSTOMER_NSG}" --resource-group "${CUSTOMER_RG_NAME}" --output none 2>/dev/null; then
  az network nsg create \
    --name "${CUSTOMER_NSG}" \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --location "${LOCATION}" \
    --output none
fi

az network vnet create \
  --name "${CUSTOMER_VNET_NAME}" \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --location "${LOCATION}" \
  --address-prefixes 10.0.0.0/16 \
  --subnet-name "${CUSTOMER_VNET_SUBNET1}" \
  --subnet-prefixes 10.0.0.0/24 \
  --nsg "${CUSTOMER_NSG}" \
  --output none

az network vnet subnet create \
  --name "${CUSTOMER_VNET_INTEGRATION_SUBNET_NAME}" \
  --vnet-name "${CUSTOMER_VNET_NAME}" \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --address-prefixes 10.0.1.0/24 \
  --network-security-group "${CUSTOMER_NSG}" \
  --delegations Microsoft.RedHatOpenShift/hcpOpenShiftClusters \
  --output none

NSG_ID=$(az network nsg show --name "${CUSTOMER_NSG}" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv)
SUBNET_ID=$(az network vnet subnet show --name "${CUSTOMER_VNET_SUBNET1}" --vnet-name "${CUSTOMER_VNET_NAME}" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv)
VNET_ID=$(az network vnet show --name "${CUSTOMER_VNET_NAME}" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv)
VNET_INTEGRATION_SUBNET_ID=$(az network vnet subnet show --name "${CUSTOMER_VNET_INTEGRATION_SUBNET_NAME}" --vnet-name "${CUSTOMER_VNET_NAME}" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv)

for name in \
  "${CLUSTER_NAME}-cp-cluster-api-azure" \
  "${CLUSTER_NAME}-cp-control-plane" \
  "${CLUSTER_NAME}-cp-cloud-controller-manager" \
  "${CLUSTER_NAME}-cp-ingress" \
  "${CLUSTER_NAME}-cp-disk-csi-driver" \
  "${CLUSTER_NAME}-cp-file-csi-driver" \
  "${CLUSTER_NAME}-cp-image-registry" \
  "${CLUSTER_NAME}-cp-cloud-network-config" \
  "${CLUSTER_NAME}-cp-kms" \
  "${CLUSTER_NAME}-dp-disk-csi-driver" \
  "${CLUSTER_NAME}-dp-file-csi-driver" \
  "${CLUSTER_NAME}-dp-image-registry" \
  "${CLUSTER_NAME}-service-managed-identity"
do
  az identity create --name "${name}" --resource-group "${CUSTOMER_RG_NAME}" --location "${LOCATION}" --output none
done

id_of() { az identity show --name "$1" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv; }
pid_of() { az identity show --name "$1" --resource-group "${CUSTOMER_RG_NAME}" --query principalId --output tsv; }

CLUSTER_API_AZURE_MI_ID=$(id_of "${CLUSTER_NAME}-cp-cluster-api-azure")
CLUSTER_API_AZURE_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-cluster-api-azure")
CONTROL_PLANE_MI_ID=$(id_of "${CLUSTER_NAME}-cp-control-plane")
CONTROL_PLANE_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-control-plane")
CLOUD_CONTROLLER_MANAGER_MI_ID=$(id_of "${CLUSTER_NAME}-cp-cloud-controller-manager")
CLOUD_CONTROLLER_MANAGER_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-cloud-controller-manager")
INGRESS_MI_ID=$(id_of "${CLUSTER_NAME}-cp-ingress")
INGRESS_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-ingress")
DISK_CSI_DRIVER_MI_ID=$(id_of "${CLUSTER_NAME}-cp-disk-csi-driver")
FILE_CSI_DRIVER_MI_ID=$(id_of "${CLUSTER_NAME}-cp-file-csi-driver")
FILE_CSI_DRIVER_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-file-csi-driver")
IMAGE_REGISTRY_MI_ID=$(id_of "${CLUSTER_NAME}-cp-image-registry")
IMAGE_REGISTRY_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-image-registry")
CLOUD_NETWORK_CONFIG_MI_ID=$(id_of "${CLUSTER_NAME}-cp-cloud-network-config")
CLOUD_NETWORK_CONFIG_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-cloud-network-config")
KMS_MI_ID=$(id_of "${CLUSTER_NAME}-cp-kms")
KMS_MI_PID=$(pid_of "${CLUSTER_NAME}-cp-kms")
DP_DISK_CSI_DRIVER_MI_ID=$(id_of "${CLUSTER_NAME}-dp-disk-csi-driver")
DP_FILE_CSI_DRIVER_MI_ID=$(id_of "${CLUSTER_NAME}-dp-file-csi-driver")
DP_FILE_CSI_DRIVER_MI_PID=$(pid_of "${CLUSTER_NAME}-dp-file-csi-driver")
DP_IMAGE_REGISTRY_MI_ID=$(id_of "${CLUSTER_NAME}-dp-image-registry")
DP_IMAGE_REGISTRY_MI_PID=$(pid_of "${CLUSTER_NAME}-dp-image-registry")
SERVICE_MI_ID=$(id_of "${CLUSTER_NAME}-service-managed-identity")
SERVICE_MI_PID=$(pid_of "${CLUSTER_NAME}-service-managed-identity")

READER_ROLE="acdd72a7-3385-48ef-bd42-f606fba81ae7"
HCP_CLUSTER_API_PROVIDER_ROLE="88366f10-ed47-4cc0-9fab-c8a06148393e"
HCP_CONTROL_PLANE_OPERATOR_ROLE="fc0c873f-45e9-4d0d-a7d1-585aab30c6ed"
CLOUD_CONTROLLER_MANAGER_ROLE="a1f96423-95ce-4224-ab27-4e3dc72facd4"
INGRESS_OPERATOR_ROLE="0336e1d3-7a87-462b-b6db-342b63f7802c"
FILE_STORAGE_OPERATOR_ROLE="0d7aedc0-15fd-4a67-a412-efad370c947e"
NETWORK_OPERATOR_ROLE="be7a6435-15ae-4171-8f30-4a343eff9e8f"
IMAGE_REGISTRY_OPERATOR_ROLE="8b32b316-c2f5-4ddf-b05b-83dacd2d08b5"
FEDERATED_CREDENTIAL_ROLE="ef318e2a-8334-4a05-9e4a-295a196c6a6e"
HCP_SERVICE_MI_ROLE="c0ff367d-66d8-445e-917c-583feb0ef0d4"
KEY_VAULT_CRYPTO_USER_ROLE="12338af0-0e69-4776-bea7-57ae8d297424"

assign_role "${CLUSTER_API_AZURE_MI_PID}" "${HCP_CLUSTER_API_PROVIDER_ROLE}" "${SUBNET_ID}"
assign_role "${CLUSTER_API_AZURE_MI_PID}" "${HCP_CLUSTER_API_PROVIDER_ROLE}" "${VNET_ID}"
assign_role "${CONTROL_PLANE_MI_PID}" "${HCP_CONTROL_PLANE_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${CONTROL_PLANE_MI_PID}" "${HCP_CONTROL_PLANE_OPERATOR_ROLE}" "${NSG_ID}"
assign_role "${CLOUD_CONTROLLER_MANAGER_MI_PID}" "${CLOUD_CONTROLLER_MANAGER_ROLE}" "${SUBNET_ID}"
assign_role "${CLOUD_CONTROLLER_MANAGER_MI_PID}" "${CLOUD_CONTROLLER_MANAGER_ROLE}" "${NSG_ID}"
assign_role "${CLOUD_CONTROLLER_MANAGER_MI_PID}" "${CLOUD_CONTROLLER_MANAGER_ROLE}" "${VNET_ID}"
assign_role "${INGRESS_MI_PID}" "${INGRESS_OPERATOR_ROLE}" "${SUBNET_ID}"
assign_role "${INGRESS_MI_PID}" "${INGRESS_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${SUBNET_ID}"
assign_role "${FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${NSG_ID}"
assign_role "${FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${IMAGE_REGISTRY_MI_PID}" "${IMAGE_REGISTRY_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${CLOUD_NETWORK_CONFIG_MI_PID}" "${NETWORK_OPERATOR_ROLE}" "${SUBNET_ID}"
assign_role "${CLOUD_NETWORK_CONFIG_MI_PID}" "${NETWORK_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${SERVICE_MI_PID}" "${HCP_SERVICE_MI_ROLE}" "${VNET_ID}"
assign_role "${SERVICE_MI_PID}" "${HCP_SERVICE_MI_ROLE}" "${NSG_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${CLUSTER_API_AZURE_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${CONTROL_PLANE_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${CLOUD_CONTROLLER_MANAGER_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${INGRESS_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${DISK_CSI_DRIVER_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${FILE_CSI_DRIVER_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${IMAGE_REGISTRY_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${CLOUD_NETWORK_CONFIG_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${READER_ROLE}" "${KMS_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${FEDERATED_CREDENTIAL_ROLE}" "${DP_DISK_CSI_DRIVER_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${FEDERATED_CREDENTIAL_ROLE}" "${DP_FILE_CSI_DRIVER_MI_ID}"
assign_role "${SERVICE_MI_PID}" "${FEDERATED_CREDENTIAL_ROLE}" "${DP_IMAGE_REGISTRY_MI_ID}"
assign_role "${DP_FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${SUBNET_ID}"
assign_role "${DP_FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${NSG_ID}"
assign_role "${DP_FILE_CSI_DRIVER_MI_PID}" "${FILE_STORAGE_OPERATOR_ROLE}" "${VNET_ID}"
assign_role "${DP_IMAGE_REGISTRY_MI_PID}" "${IMAGE_REGISTRY_OPERATOR_ROLE}" "${VNET_ID}"

az keyvault create \
  --name "${KEYVAULT_NAME}" \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --location "${LOCATION}" \
  --enable-rbac-authorization true \
  --output none

KEYVAULT_ID=$(az keyvault show --name "${KEYVAULT_NAME}" --resource-group "${CUSTOMER_RG_NAME}" --query id --output tsv)
USER_NAME="$(az account show --query user.name -o tsv)"

for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
  if az role assignment create \
    --role "Key Vault Crypto Officer" \
    --assignee "${USER_NAME}" \
    --scope "${KEYVAULT_ID}" \
    --only-show-errors \
    --output none; then
    break
  fi
  echo "retry ${attempt}: Key Vault Crypto Officer" >&2
  sleep 15
  if [[ "${attempt}" -eq 12 ]]; then
    exit 1
  fi
done

for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
  if az keyvault key create \
    --vault-name "${KEYVAULT_NAME}" \
    --name "${KMS_KEY_NAME}" \
    --kty RSA \
    --size 2048 \
    --output none; then
    break
  fi
  echo "retry ${attempt}: create KMS key" >&2
  sleep 20
  if [[ "${attempt}" -eq 12 ]]; then
    exit 1
  fi
done

KMS_KEY_VERSION=$(az keyvault key show \
  --vault-name "${KEYVAULT_NAME}" \
  --name "${KMS_KEY_NAME}" \
  --query "key.kid" --output tsv | awk -F/ '{print $NF}')

assign_role "${KMS_MI_PID}" "${KEY_VAULT_CRYPTO_USER_ROLE}" "${KEYVAULT_ID}"

mkdir -p "$(dirname "${ENV_FILE}")"
cat > "${ENV_FILE}" <<EOF
LOCATION=${LOCATION}
SUBSCRIPTION_ID=${SUBSCRIPTION_ID}
CUSTOMER_RG_NAME=${CUSTOMER_RG_NAME}
KEEP_RESOURCE_GROUP=${KEEP_RESOURCE_GROUP}
CUSTOMER_NSG=${CUSTOMER_NSG}
CUSTOMER_VNET_NAME=${CUSTOMER_VNET_NAME}
CUSTOMER_VNET_SUBNET1=${CUSTOMER_VNET_SUBNET1}
CUSTOMER_VNET_INTEGRATION_SUBNET_NAME=${CUSTOMER_VNET_INTEGRATION_SUBNET_NAME}
CLUSTER_NAME=${CLUSTER_NAME}
MANAGED_RESOURCE_GROUP=${MANAGED_RESOURCE_GROUP}
NP_NAME=${NP_NAME}
CLUSTER_VERSION=${CLUSTER_VERSION}
NP_VERSION=${NP_VERSION}
KEYVAULT_NAME=${KEYVAULT_NAME}
KMS_KEY_NAME=${KMS_KEY_NAME}
KMS_KEY_VERSION=${KMS_KEY_VERSION}
NSG_ID=${NSG_ID}
SUBNET_ID=${SUBNET_ID}
VNET_ID=${VNET_ID}
VNET_INTEGRATION_SUBNET_ID=${VNET_INTEGRATION_SUBNET_ID}
CLUSTER_API_AZURE_MI_ID=${CLUSTER_API_AZURE_MI_ID}
CONTROL_PLANE_MI_ID=${CONTROL_PLANE_MI_ID}
CLOUD_CONTROLLER_MANAGER_MI_ID=${CLOUD_CONTROLLER_MANAGER_MI_ID}
INGRESS_MI_ID=${INGRESS_MI_ID}
DISK_CSI_DRIVER_MI_ID=${DISK_CSI_DRIVER_MI_ID}
FILE_CSI_DRIVER_MI_ID=${FILE_CSI_DRIVER_MI_ID}
IMAGE_REGISTRY_MI_ID=${IMAGE_REGISTRY_MI_ID}
CLOUD_NETWORK_CONFIG_MI_ID=${CLOUD_NETWORK_CONFIG_MI_ID}
KMS_MI_ID=${KMS_MI_ID}
DP_DISK_CSI_DRIVER_MI_ID=${DP_DISK_CSI_DRIVER_MI_ID}
DP_FILE_CSI_DRIVER_MI_ID=${DP_FILE_CSI_DRIVER_MI_ID}
DP_IMAGE_REGISTRY_MI_ID=${DP_IMAGE_REGISTRY_MI_ID}
SERVICE_MI_ID=${SERVICE_MI_ID}
EOF

echo "PREP_DONE keyvault=${KEYVAULT_NAME} key_version=${KMS_KEY_VERSION}"
