#!/usr/bin/env bash
# Configure supported dashboard flags; scheduling stays with the operators.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
oc wait --for=create odhdashboardconfig/odh-dashboard-config \
  -n redhat-ods-applications --timeout=300s
oc patch odhdashboardconfig odh-dashboard-config -n redhat-ods-applications \
  --type=merge --patch-file="${SCRIPT_DIR}/odh-dashboard-config.yaml"
echo 'MaaS and Gen AI studio dashboard flags applied.'
