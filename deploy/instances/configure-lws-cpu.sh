#!/usr/bin/env bash
# LeaderWorkerSet defaults to 2 manager replicas at 1000m CPU each. On a
# 3-worker ARO HCP cluster that packs the nodes so Dev Spaces (500m) cannot
# schedule. Qwen3.8 is an LLMInferenceService Deployment — keep the LWS CRD,
# keep one manager, stop the operator from restoring replicas=2.
set -euo pipefail

NS=openshift-lws-operator
echo "Waiting for LeaderWorkerSet manager..."
for _ in $(seq 1 60); do
  if oc get deploy/lws-controller-manager -n "$NS" >/dev/null 2>&1; then
    break
  fi
  sleep 5
done

if oc get leaderworkersetoperator/cluster -n "$NS" >/dev/null 2>&1; then
  oc patch leaderworkersetoperator/cluster -n "$NS" --type=merge \
    -p '{"spec":{"managementState":"Unmanaged"}}' >/dev/null || true
fi

# The operator Deployment restores replicas=2 while it is running.
if oc get deploy/openshift-lws-operator -n "$NS" >/dev/null 2>&1; then
  oc scale deploy/openshift-lws-operator -n "$NS" --replicas=0 >/dev/null
fi
oc scale deploy/lws-controller-manager -n "$NS" --replicas=1 >/dev/null
echo "LeaderWorkerSet manager scaled to 1 (CPU headroom for Dev Spaces)."
