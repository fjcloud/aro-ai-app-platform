#!/usr/bin/env bash
# Authorino TLS between the MaaS Gateway and Authorino, per
# https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/3.5/html/govern_llm_access_with_models-as-a-service/deploy-and-manage-models-as-a-service
set -euo pipefail

# The Service appears before the operator creates the Deployment on a fresh lab.
# Wait for that object before configuring its CA environment.
WAIT_SECONDS=${AUTHORINO_WAIT_SECONDS:-300}
[[ "$WAIT_SECONDS" =~ ^[1-9][0-9]*$ ]] || { echo 'AUTHORINO_WAIT_SECONDS must be positive.' >&2; exit 1; }
deadline=$((SECONDS + WAIT_SECONDS))
until oc --request-timeout=15s get deployment/authorino -n kuadrant-system >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "Authorino Deployment was not created within $WAIT_SECONDS seconds; check the Kuadrant operator." >&2
    exit 1
  fi
  sleep 2
done

oc annotate service authorino-authorino-authorization \
  -n kuadrant-system \
  service.beta.openshift.io/serving-cert-secret-name=authorino-server-cert \
  --overwrite

oc patch authorino authorino -n kuadrant-system --type=merge --patch '{
  "spec": {
    "listener": {
      "tls": {
        "enabled": true,
        "certSecretRef": { "name": "authorino-server-cert" }
      }
    }
  }
}'

oc -n kuadrant-system set env deployment/authorino \
  SSL_CERT_FILE=/etc/ssl/certs/openshift-service-ca/service-ca-bundle.crt \
  REQUESTS_CA_BUNDLE=/etc/ssl/certs/openshift-service-ca/service-ca-bundle.crt

oc annotate gateway maas-default-gateway \
  -n openshift-ingress \
  security.opendatahub.io/authorino-tls-bootstrap=true \
  --overwrite

echo "Authorino TLS configured."
