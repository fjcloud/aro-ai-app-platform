# Write an admin kubeconfig to state/kubeconfig. Source after common.sh and cluster.env.
KUBECONFIG_FILE="${ROOT}/state/kubeconfig"

request_admin_kubeconfig() {
  local raw="${KUBECONFIG_FILE}.raw"
  mkdir -p "$(dirname "${KUBECONFIG_FILE}")"
  "${AZ}" aro hcp cluster request-credential --admin \
    --name "${CLUSTER_NAME}" \
    --resource-group "${CUSTOMER_RG_NAME}" \
    --subscription "${SUBSCRIPTION_ID}" \
    --file "${raw}" \
    --overwrite-existing \
    --output none
  python3 - "${raw}" "${KUBECONFIG_FILE}" <<'PY'
import pathlib, sys
raw, dest = sys.argv[1:]
text = pathlib.Path(raw).read_text()
start = text.find("apiVersion:")
if start < 0:
    sys.exit("kubeconfig has no apiVersion")
path = pathlib.Path(dest)
path.write_text(text[start:])
path.chmod(0o600)
pathlib.Path(raw).unlink(missing_ok=True)
PY
}
