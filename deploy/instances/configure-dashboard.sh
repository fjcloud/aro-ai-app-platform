#!/usr/bin/env bash
# After DSC dashboard+ogx are Managed:
# 1) allow UI pods to tolerate the GPU taint if needed
# 2) enable MaaS / Gen AI studio flags on OdhDashboardConfig
set -euo pipefail

NS=redhat-ods-applications
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

DEPLOYMENTS=(
  rhods-dashboard
  dashboard-redirect
  agent-ops-ui
  gen-ai-ui
  maas-ui
  ogx-k8s-operator-controller-manager
)

PATCH='{"spec":{"template":{"spec":{"tolerations":[{"key":"nvidia.com/gpu","operator":"Exists","effect":"NoSchedule"}]}}}}'

patch_known_deploys() {
  for d in "${DEPLOYMENTS[@]}"; do
    if oc get deploy "$d" -n "$NS" >/dev/null 2>&1; then
      oc patch deploy "$d" -n "$NS" --type=strategic --patch "$PATCH" >/dev/null
      echo "  patched $d"
    fi
  done
}

echo "Waiting for dashboard Deployments..."
for _ in $(seq 1 36); do
  if oc get deploy rhods-dashboard -n "$NS" >/dev/null 2>&1; then
    break
  fi
  sleep 5
done

patch_known_deploys

# OGX UIs (maas-ui, gen-ai-ui) appear a few seconds after the dashboard operator.
echo "Waiting for MaaS UI Deployments..."
for _ in $(seq 1 24); do
  if oc get deploy maas-ui -n "$NS" >/dev/null 2>&1; then
    break
  fi
  sleep 5
done
patch_known_deploys

echo "Waiting for OdhDashboardConfig..."
for _ in $(seq 1 36); do
  if oc get odhdashboardconfig odh-dashboard-config -n "$NS" >/dev/null 2>&1; then
    break
  fi
  sleep 5
done

# Merge only supported flags. oc apply on the full CR trips CEL rules for
# deprecated keys and notebookNamespace immutability.
oc patch odhdashboardconfig odh-dashboard-config -n "$NS" --type=merge --patch-file="${SCRIPT_DIR}/odh-dashboard-config.yaml"

echo "Dashboard optional GPU-node tolerations and MaaS UI flags applied."
