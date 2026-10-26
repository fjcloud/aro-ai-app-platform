#!/usr/bin/env bash
# Native MaaS usage telemetry; no additional monitoring stack or provider key.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
oc patch kuadrant kuadrant -n kuadrant-system --type=merge \
  -p '{"spec":{"observability":{"enable":true}}}'
oc wait maastenantconfig/default-tenant -n models-as-a-service \
  --for=condition=Ready --timeout=300s
oc patch maastenantconfig default-tenant -n models-as-a-service --type=merge -p '{
  "spec":{"telemetry":{"enabled":true,"metrics":{
    "captureOrganization":true,"captureModelUsage":true,"captureUser":false,"captureGroup":false
  }}}
}'
oc patch odhdashboardconfig odh-dashboard-config -n redhat-ods-applications --type=merge \
  -p '{"spec":{"dashboardConfig":{"observabilityDashboard":true}}}'
oc apply -k "$SCRIPT_DIR"
echo 'MaaS usage telemetry enabled; allow 2–3 minutes for configuration propagation and new samples.'
