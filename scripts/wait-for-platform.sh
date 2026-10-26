#!/usr/bin/env bash
# Bounded, read-only operator checks. Do not apply instances after a failed check.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TIMEOUT=${OPERATOR_WAIT_SECONDS:-600}
[[ "$TIMEOUT" =~ ^[1-9][0-9]*$ ]] || { echo 'OPERATOR_WAIT_SECONDS must be a positive integer.' >&2; exit 1; }

wait_deploy() {
  local ns=$1 dep=$2 deadline=$((SECONDS + TIMEOUT))
  echo "Waiting for $dep in $ns..."
  until oc --request-timeout=15s get deployment/"$dep" -n "$ns" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      echo "Operator Deployment $ns/$dep was not created within $TIMEOUT seconds. Check OLM subscription conditions and worker capacity before continuing." >&2
      oc --request-timeout=15s get subscriptions -n "$ns" >&2 || true
      exit 1
    fi
    sleep 5
  done
  oc wait deployment/"$dep" -n "$ns" \
    --for=condition=Available --timeout="${TIMEOUT}s"
}

wait_deploy openshift-operators devspaces-operator
wait_deploy openshift-nfd nfd-controller-manager
wait_deploy nvidia-gpu-operator gpu-operator
wait_deploy redhat-ods-operator rhods-operator
wait_deploy kuadrant-system kuadrant-operator-controller-manager
wait_deploy openshift-lws-operator openshift-lws-operator
wait_deploy cert-manager cert-manager
wait_deploy openshift-pipelines tekton-pipelines-controller
wait_deploy openshift-gitops openshift-gitops-server
wait_deploy openshift-cluster-observability-operator observability-operator
wait_deploy openshift-opentelemetry-operator opentelemetry-operator-controller-manager
