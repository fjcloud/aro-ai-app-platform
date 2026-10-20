#!/usr/bin/env bash
# Configure the Red Hat pull secret and Entra ID console login.
# API and ingress are public, so oc runs on this machine.
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/request-kubeconfig.sh"
PULL_SECRET_FILE="${PULL_SECRET_FILE:-${HOME}/pull_secret.json}"
WORKDIR="$(mktemp -d)"

cleanup() {
  local status=$?
  rm -rf "${WORKDIR}"
  exit "${status}"
}
trap cleanup EXIT

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

if [[ "${AUTH_CONFIGURED:-}" == "yes" ]]; then
  if [[ ! -s "${KUBECONFIG_FILE}" ]]; then
    echo "Refreshing admin kubeconfig"
    request_admin_kubeconfig
  fi
  echo "Pull secret and console auth are already configured"
  echo "KUBECONFIG=${KUBECONFIG_FILE}"
  trap - EXIT
  rm -rf "${WORKDIR}"
  exit 0
fi

if [[ ! -f "${PULL_SECRET_FILE}" ]]; then
  echo "Pull secret file not found: ${PULL_SECRET_FILE}" >&2
  exit 1
fi

if ! command -v oc >/dev/null 2>&1; then
  echo "oc is not on PATH. Install the OpenShift CLI before continuing." >&2
  exit 1
fi

python3 - "${PULL_SECRET_FILE}" "${WORKDIR}/pull.json" <<'PY'
import json, sys
src, dest = sys.argv[1], sys.argv[2]
auths = json.load(open(src)).get("auths", {})
keep = {}
required = ("registry.redhat.io", "registry.connect.redhat.com", "cloud.openshift.com")
for name in required + ("quay.io",):
    if name in auths:
        keep[name] = auths[name]
missing = [name for name in required if name not in keep]
if missing:
    sys.exit("pull secret is missing: " + ", ".join(missing))
json.dump({"auths": keep}, open(dest, "w"))
PY

TENANT_ID="$("${AZ}" account show --query tenantId --output tsv)"
ADMIN_USER="${CLUSTER_ADMIN_USER:-${WORKSHOP_EMAIL:-}}"
if [[ -z "${ADMIN_USER}" ]]; then
  if [[ "$("${AZ}" account show --query user.type --output tsv)" == "servicePrincipal" ]]; then
    echo 'Set CLUSTER_ADMIN_USER to an existing Entra user in the lab tenant; a service principal cannot sign in to the console.' >&2
    exit 1
  fi
  ADMIN_USER="$("${AZ}" account show --query user.name --output tsv)"
fi

echo "Waiting for cluster ${CLUSTER_NAME} to finish provisioning"
state=""
for _ in $(seq 1 40); do
  state="$("${AZ}" aro hcp cluster show \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --name "${CLUSTER_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --query properties.provisioningState --output tsv || true)"
  echo "cluster=${state:-unknown}"
  if [[ "${state}" == "Succeeded" ]]; then
    break
  fi
  sleep 15
done
if [[ "${state}" != "Succeeded" ]]; then
  echo "Cluster ${CLUSTER_NAME} is not Succeeded (${state:-unknown})." >&2
  exit 1
fi

CONSOLE_URL="$("${AZ}" aro hcp cluster show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --name "${CLUSTER_NAME}" \
  --subscription "${SUBSCRIPTION_ID}" \
  --query properties.console.url --output tsv)"
CONSOLE_URL="${CONSOLE_URL%/}"

upsert_env() {
  python3 - "${ENV_FILE}" "$1" "$2" <<'PY'
import sys
from pathlib import Path
path, key, value = sys.argv[1:]
file = Path(path)
lines = file.read_text().splitlines() if file.exists() else []
out, found = [], False
for line in lines:
    if line.startswith(key + "="):
        out.append(f"{key}={value}")
        found = True
    else:
        out.append(line)
if not found:
    out.append(f"{key}={value}")
file.write_text("\n".join(out) + "\n")
PY
}

create_app() {
  local display_name="$1"
  local kind="$2"
  local app_id
  app_id="$("${AZ}" ad app list --display-name "${display_name}" --query "[0].appId" --output tsv)"
  if [[ -z "${app_id}" || "${app_id}" == "null" ]]; then
    if [[ "${kind}" == "console" ]]; then
      app_id="$("${AZ}" ad app create \
        --display-name "${display_name}" \
        --sign-in-audience AzureADMyOrg \
        --web-redirect-uris "${CONSOLE_URL}/auth/callback" \
        --enable-id-token-issuance true \
        --query appId --output tsv)"
    else
      app_id="$("${AZ}" ad app create \
        --display-name "${display_name}" \
        --sign-in-audience AzureADMyOrg \
        --is-fallback-public-client true \
        --public-client-redirect-uris "http://localhost:8080" "http://127.0.0.1:8080" \
        --query appId --output tsv)"
    fi
  fi
  "${AZ}" ad sp create --id "${app_id}" --output none >/dev/null 2>&1 || true
  "${AZ}" rest --method PATCH \
    --uri "https://graph.microsoft.com/v1.0/applications(appId='${app_id}')" \
    --headers "Content-Type=application/json" \
    --body '{"api":{"requestedAccessTokenVersion":2},"optionalClaims":{"idToken":[{"name":"email","essential":false},{"name":"preferred_username","essential":false},{"name":"upn","essential":false}]}}' \
    --output none >/dev/null
  printf '%s' "${app_id}"
}

echo "Creating Entra applications"
CONSOLE_APP_ID="$(create_app "${CLUSTER_NAME}-console" console)"
CLI_APP_ID="$(create_app "${CLUSTER_NAME}-cli" cli)"
upsert_env CONSOLE_APP_ID "${CONSOLE_APP_ID}"
upsert_env CLI_APP_ID "${CLI_APP_ID}"
upsert_env CLUSTER_ADMIN_USER "${ADMIN_USER}"
upsert_env CONSOLE_URL "${CONSOLE_URL}"

CLIENT_SECRET="$("${AZ}" ad app credential reset \
  --id "${CONSOLE_APP_ID}" \
  --display-name console \
  --years 1 \
  --query password --output tsv)"
printf '%s' "${CLIENT_SECRET}" > "${WORKDIR}/client-secret"
chmod 600 "${WORKDIR}/client-secret"

if ! "${AZ}" aro hcp cluster external-auth show \
  --resource-group "${CUSTOMER_RG_NAME}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name entra \
  --subscription "${SUBSCRIPTION_ID}" \
  --query name --output tsv >/dev/null 2>&1; then
  echo "Creating external auth entra"
  python3 - "${WORKDIR}/audiences.json" "${WORKDIR}/clients.json" "${CONSOLE_APP_ID}" "${CLI_APP_ID}" <<'PY'
import json, sys
audiences, clients, console_id, cli_id = sys.argv[1:]
json.dump([console_id, cli_id], open(audiences, "w"))
json.dump([
    {
        "client-id": console_id,
        "type": "Confidential",
        "component": {"name": "console", "auth-client-namespace": "openshift-console"},
        "extra-scopes": ["email", "profile"],
    },
    {
        "client-id": cli_id,
        "type": "Public",
        "component": {"name": "cli", "auth-client-namespace": "openshift-console"},
        "extra-scopes": ["email", "profile"],
    },
], open(clients, "w"))
PY
  "${AZ}" aro hcp cluster external-auth create \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --cluster-name "${CLUSTER_NAME}" \
    --name entra \
    --subscription "${SUBSCRIPTION_ID}" \
    --issuer-url "https://login.microsoftonline.com/${TENANT_ID}/v2.0" \
    --issuer-audience "@${WORKDIR}/audiences.json" \
    --username-claim preferred_username \
    --username-prefix-policy NoPrefix \
    --clients "@${WORKDIR}/clients.json" \
    --query name --output tsv
fi

echo "Requesting admin kubeconfig"
request_admin_kubeconfig
export KUBECONFIG="${KUBECONFIG_FILE}"

echo "Applying pull secret and console auth"
oc wait --for=condition=Ready nodes --all --timeout=900s
pull_b64="$(python3 -c 'import base64, pathlib, sys; print(base64.b64encode(pathlib.Path(sys.argv[1]).read_bytes()).decode())' "${WORKDIR}/pull.json")"
oc apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: additional-pull-secret
  namespace: kube-system
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: ${pull_b64}
EOF

regs=""
for i in $(seq 1 30); do
  regs="$(oc get secret global-pull-secret -n kube-system -o jsonpath='{.data.\.dockerconfigjson}' 2>/dev/null | base64 -d | python3 -c 'import json,sys
try:
    print(",".join(sorted(json.load(sys.stdin).get("auths",{}))))
except Exception:
    print("")' || true)"
  echo "registries=${regs:-none} try=${i}"
  case "${regs}" in
    *cloud.openshift.com*registry.redhat.io*) break ;;
  esac
  sleep 10
done
case "${regs}" in
  *cloud.openshift.com*registry.redhat.io*) ;;
  *)
    echo "Pull secret did not merge into global-pull-secret." >&2
    exit 1
    ;;
esac

oc create secret generic entra-console-openshift-console \
  -n openshift-config \
  --from-file=clientSecret="${WORKDIR}/client-secret" \
  --dry-run=client -o yaml | oc apply -f -
oc adm policy add-cluster-role-to-user cluster-admin "${ADMIN_USER}"

available=""
degraded=""
for i in $(seq 1 40); do
  available="$(oc get co console -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || true)"
  degraded="$(oc get co console -o jsonpath='{.status.conditions[?(@.type=="Degraded")].status}' 2>/dev/null || true)"
  echo "available=${available} degraded=${degraded} try=${i}"
  if [[ "${available}" == "True" && "${degraded}" == "False" ]]; then
    break
  fi
  sleep 15
done

code=""
for _ in $(seq 1 20); do
  code="$(curl -sk -o /dev/null -w '%{http_code}' --max-time 30 "${CONSOLE_URL}" || true)"
  echo "console_http=${code}"
  case "${code}" in
    200|302|303) break ;;
  esac
  sleep 15
done

if [[ "${available}" != "True" || "${degraded}" != "False" ]]; then
  echo "Console operator is not Available." >&2
  exit 1
fi
case "${code}" in
  200|302|303) ;;
  *)
    echo "Console ${CONSOLE_URL} did not respond (HTTP ${code:-none})." >&2
    exit 1
    ;;
esac

upsert_env AUTH_CONFIGURED yes
echo "CONFIGURE_DONE user=${ADMIN_USER} console=${CONSOLE_URL}"
echo "KUBECONFIG=${KUBECONFIG_FILE}"
