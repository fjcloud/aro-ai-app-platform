#!/usr/bin/env bash
# Kuadrant registers Istio reconcilers only if EnvoyFilter CRDs exist when the
# operator *process* starts. On OpenShift 4.21+ the Gateway API implementation
# (GatewayClass openshift-default → istiod) creates those CRDs after
# deploy/instances — the RHCL operator from Lab 1.2 is already running.
# Recycle the operator pod so AuthPolicy is not stuck MissingDependency.
#
# Do not oc rollout restart the Kuadrant operator Deployment: it is
# CSV-owned and OLM reverts the restart. Do not install OSSM 3.
set -euo pipefail

NS=kuadrant-system
DEPLOY=kuadrant-operator-controller-manager
GW_DEPLOY=maas-default-gateway-openshift-default
GW_NS=openshift-ingress

kuadrant_istio_ready() {
  oc get crd envoyfilters.networking.istio.io >/dev/null 2>&1 || return 1
  oc get envoyfilter -n "${GW_NS}" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null \
    | grep -q kuadrant
}

echo "Waiting for OpenShift Gateway API Istio CRDs (EnvoyFilter)..."
deadline=$((SECONDS + 300))
while (( SECONDS < deadline )); do
  if oc get crd envoyfilters.networking.istio.io >/dev/null 2>&1; then
    oc wait crd/envoyfilters.networking.istio.io \
      --for=condition=Established --timeout=60s >/dev/null
    break
  fi
  sleep 5
done

if ! oc get crd envoyfilters.networking.istio.io >/dev/null 2>&1; then
  echo "WARNING: envoyfilters.networking.istio.io CRD not found after 5m." >&2
  echo "Kuadrant AuthPolicy may stay MissingDependency until istiod is up." >&2
  exit 0
fi

if kuadrant_istio_ready; then
  echo "Kuadrant EnvoyFilters already present — operator does not need a recycle."
  exit 0
fi

echo "Recycling Kuadrant operator so it registers Istio reconcilers..."
oc delete pod -n "${NS}" -l app=kuadrant --wait=true
oc wait deployment/"${DEPLOY}" -n "${NS}" \
  --for=condition=Available --timeout=180s

if oc get deploy "${GW_DEPLOY}" -n "${GW_NS}" >/dev/null 2>&1; then
  oc rollout restart deployment/"${GW_DEPLOY}" -n "${GW_NS}"
  # Do not fail the whole platform configure if the dataplane is CPU-pending.
  oc rollout status deployment/"${GW_DEPLOY}" -n "${GW_NS}" --timeout=180s || \
    echo "WARNING: ${GW_DEPLOY} rollout not complete yet (often Insufficient cpu)." >&2
fi

echo "Kuadrant operator recycled after Gateway API CRDs."
