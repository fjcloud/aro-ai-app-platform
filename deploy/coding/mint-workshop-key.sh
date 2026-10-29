#!/usr/bin/env bash
# Mint a MaaS API key bound to deepseekv4pro-workshop and store it as OPENAI_API_KEY
# for Dev Spaces (OpenCode). Requires cluster-admin (or any user
# in a group listed on MaaSSubscription/deepseekv4pro-workshop).
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=../cluster-env.sh
source "${SCRIPT_DIR}/../cluster-env.sh"

SECRET_NS="${SECRET_NS:-openshift-operators}"
SECRET_NAME="${SECRET_NAME:-ai-provider-openai-api-key}"
MAAS_SUBSCRIPTION="${MAAS_SUBSCRIPTION:-deepseekv4pro-workshop}"
KEY_NAME="${KEY_NAME:-workshop-opencode}"

if ! TOKEN="$(oc whoami -t 2>/dev/null)" || [[ -z "$TOKEN" ]]; then
  echo 'Run make onboard to sign in to ARO, then rerun this script.' >&2
  exit 1
fi

MAAS_HOST="${MAAS_HOST:-$(workshop_maas_host)}"
MAAS_API_URL="https://${MAAS_HOST}"
OPENAI_BASE_URL="${MAAS_API_URL}/v1"

echo "Minting MaaS key '${KEY_NAME}' on ${MAAS_API_URL} (subscription=${MAAS_SUBSCRIPTION})"

mint_key() {
  curl -sS -w '\n%{http_code}' -X POST "${MAAS_API_URL}/maas-api/v1/api-keys" \
    --max-time 60 -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"name\": \"${KEY_NAME}\", \"description\": \"Dev Spaces OpenCode workshop key\", \"subscription\": \"${MAAS_SUBSCRIPTION}\", \"expiresIn\": \"24h\"}"
}

RESP="$(mint_key)"
HTTP_CODE="$(echo "${RESP}" | tail -n1)"
BODY="$(echo "${RESP}" | sed '$d')"

# 503 / WASM AUTH_FAILURE: Kuadrant started before EnvoyFilter CRDs existed.
if [[ "${HTTP_CODE}" == "503" || "${HTTP_CODE}" == "000" ]]; then
  echo "Mint HTTP ${HTTP_CODE} — recycling Kuadrant after Gateway API CRDs..." >&2
  bash "${SCRIPT_DIR}/../instances/configure-kuadrant-gateway.sh"
  sleep 8
  RESP="$(mint_key)"
  HTTP_CODE="$(echo "${RESP}" | tail -n1)"
  BODY="$(echo "${RESP}" | sed '$d')"
fi

if [[ "${HTTP_CODE}" != "200" && "${HTTP_CODE}" != "201" ]]; then
  echo "Failed to mint API key (HTTP ${HTTP_CODE}):" >&2
  exit 1
fi

KEY_ID="$(echo "${BODY}" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("id", ""))')"
API_KEY="$(echo "${BODY}" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("key") or "")')"
if [[ -z "${API_KEY}" || "${API_KEY}" != sk-oai-* || ! "${KEY_ID}" =~ ^[[:alnum:]_-]+$ ]]; then
  echo "Mint response did not include a valid MaaS key and key ID." >&2
  exit 1
fi

oc create secret generic "${SECRET_NAME}" \
  -n "${SECRET_NS}" \
  --from-literal=OPENAI_API_KEY="${API_KEY}" \
  --from-literal=OPENAI_BASE_URL="${OPENAI_BASE_URL}" \
  --dry-run=client -o yaml | oc apply -f -

# Restore Che injector labels after apply
oc label secret "${SECRET_NAME}" -n "${SECRET_NS}" \
  app.kubernetes.io/part-of=che.eclipse.org \
  app.kubernetes.io/component=workspaces-config \
  controller.devfile.io/mount-to-devworkspace=true \
  controller.devfile.io/watch-secret=true \
  che.eclipse.org/ai-provider-id=opencodeai-opencode \
  --overwrite >/dev/null
oc annotate secret "${SECRET_NAME}" -n "${SECRET_NS}" \
  workshop.fjcloud.io/maas-key-id="${KEY_ID}" \
  controller.devfile.io/mount-as=env \
  controller.devfile.io/mount-on-start=true \
  --overwrite >/dev/null

echo "Stored key in secret/${SECRET_NAME} (${SECRET_NS})"
echo "OPENAI_BASE_URL=${OPENAI_BASE_URL}"
echo "Smoke: curl -sS ${MAAS_API_URL}/v1/models -H 'Authorization: Bearer \$OPENAI_API_KEY'"
