#!/usr/bin/env bash
# Install all operators, then the platform instances. No model or provider keys.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
oc apply -k deploy/operators
bash scripts/wait-for-platform.sh
oc apply -f deploy/operators/tekton-tasks.yaml
# The database must be available before the MaaS component starts.
oc apply -f deploy/instances/maas-postgres.yaml
oc rollout status deployment/postgres -n redhat-ai-gateway-infra --timeout=300s
oc apply -k deploy/instances
# Configure TLS before waiting for dependent MaaS resources.
deadline=$((SECONDS + 600))
until oc get service/authorino-authorino-authorization -n kuadrant-system >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "Authorino Service was not created; check Kuadrant and worker capacity." >&2
    exit 1
  fi
  sleep 5
done
bash deploy/instances/configure-platform.sh
oc wait dsci/default-dsci --for=condition=Ready --timeout=1200s
oc wait dsc/default-dsc --for=condition=Ready --timeout=1200s
oc wait crd/mlflows.mlflow.opendatahub.io --for=condition=Established --timeout=300s
# Keep existing storage settings if the facilitator already has an MLflow instance.
if ! oc get mlflow/mlflow >/dev/null 2>&1; then
  oc apply -f deploy/agentops/mlflow.yaml
fi
oc wait maastenantconfig/default-tenant -n models-as-a-service --for=condition=Ready --timeout=600s
bash deploy/observability/configure.sh
# Keep the mounted service CA after the final Kuadrant reconciliation.
bash deploy/instances/configure-authorino-trust.sh
oc rollout status deployment/payload-processing -n openshift-ingress --timeout=600s
oc rollout status deployment/payload-pre-processing -n openshift-ingress --timeout=600s
oc wait gateway/maas-default-gateway -n openshift-ingress --for=condition=Programmed --timeout=600s
oc wait checluster/devspaces -n openshift-operators --for=jsonpath='{.status.chePhase}'=Active --timeout=900s
printf '\nPlatform ready. Continue with the Foundry deployment.\n'
