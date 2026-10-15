#!/usr/bin/env bash
# Bind the MaaS Gateway to this cluster's apps domain and wildcard cert.
# Cluster id and DNS change on every cluster — do not hardcode them.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=../cluster-env.sh
source "${SCRIPT_DIR}/../cluster-env.sh"

HOST="$(workshop_maas_host)"
# If the Gateway still has the placeholder, compute from the Ingress domain.
if [[ "${HOST}" == "maas.apps.cluster.example.com" ]]; then
  HOST="maas.$(workshop_apps_domain)"
fi
CERT="$(workshop_maas_cert)"
BASE="https://${HOST}/llm-serving/qwen3/v1"
SECRET_NS="${SECRET_NS:-openshift-operators}"
SECRET_NAME="${SECRET_NAME:-ai-provider-openai-api-key}"

echo "MaaS Gateway host: ${HOST}"
echo "TLS secret:        ${CERT}"

if ! oc get secret "${CERT}" -n openshift-ingress >/dev/null 2>&1; then
  echo "WARNING: secret/${CERT} not found in openshift-ingress" >&2
fi

oc patch gateway maas-default-gateway -n openshift-ingress --type=json -p "[
  {\"op\":\"replace\",\"path\":\"/spec/listeners/0/hostname\",\"value\":\"${HOST}\"},
  {\"op\":\"replace\",\"path\":\"/spec/listeners/0/tls/certificateRefs/0/name\",\"value\":\"${CERT}\"}
]"

# OpenCode reads {env:OPENAI_BASE_URL} / ${OPENAI_BASE_URL}.
# Keep the Dev Spaces secret in sync if mint already created it.
if oc get secret "${SECRET_NAME}" -n "${SECRET_NS}" >/dev/null 2>&1; then
  oc patch secret "${SECRET_NAME}" -n "${SECRET_NS}" --type=merge \
    -p "{\"stringData\":{\"OPENAI_BASE_URL\":\"${BASE}\"}}" >/dev/null
  echo "Updated OPENAI_BASE_URL on secret/${SECRET_NAME}"
fi

echo "MaaS Gateway bound to this cluster."
