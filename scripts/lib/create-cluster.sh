#!/usr/bin/env bash
set -euo pipefail
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

set -a
# shellcheck disable=SC1091
source "${ENV_FILE}"
set +a

"${AZ}" aro hcp cluster create \
  --name "${CLUSTER_NAME}" \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --location "${LOCATION}" \
  --version "${CLUSTER_VERSION}" \
  --channel-group stable \
  --subnet-id "${SUBNET_ID}" \
  --vnet-integration-subnet-id "${VNET_INTEGRATION_SUBNET_ID}" \
  --nsg "${NSG_ID}" \
  --managed-resource-group-name "${MANAGED_RESOURCE_GROUP}" \
  --key-management-mode CustomerManaged \
  --etcd-encryption-type KMS \
  --kms-vault-name "${KEYVAULT_NAME}" \
  --vault-visibility Public \
  --kms-active-key "{name:${KMS_KEY_NAME},version:${KMS_KEY_VERSION}}" \
  --api-visibility Public \
  --ingress-visibility Public \
  --user-assigned-identities "{${SERVICE_MI_ID}:{},${CLUSTER_API_AZURE_MI_ID}:{},${CONTROL_PLANE_MI_ID}:{},${CLOUD_CONTROLLER_MANAGER_MI_ID}:{},${INGRESS_MI_ID}:{},${DISK_CSI_DRIVER_MI_ID}:{},${FILE_CSI_DRIVER_MI_ID}:{},${IMAGE_REGISTRY_MI_ID}:{},${CLOUD_NETWORK_CONFIG_MI_ID}:{},${KMS_MI_ID}:{}}" \
  --operators-authentication "{user-assigned-identities:{control-plane-operators:{cluster-api-azure:${CLUSTER_API_AZURE_MI_ID},control-plane:${CONTROL_PLANE_MI_ID},cloud-controller-manager:${CLOUD_CONTROLLER_MANAGER_MI_ID},ingress:${INGRESS_MI_ID},disk-csi-driver:${DISK_CSI_DRIVER_MI_ID},file-csi-driver:${FILE_CSI_DRIVER_MI_ID},image-registry:${IMAGE_REGISTRY_MI_ID},cloud-network-config:${CLOUD_NETWORK_CONFIG_MI_ID},kms:${KMS_MI_ID}},data-plane-operators:{disk-csi-driver:${DP_DISK_CSI_DRIVER_MI_ID},file-csi-driver:${DP_FILE_CSI_DRIVER_MI_ID},image-registry:${DP_IMAGE_REGISTRY_MI_ID}},service-managed-identity:${SERVICE_MI_ID}}}" \
  --query "{name:name,state:properties.provisioningState,api:properties.api.visibility,ingress:properties.ingress.type}" \
  -o json
