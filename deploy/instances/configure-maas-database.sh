#!/usr/bin/env bash
# Generate/reuse workshop database credentials; ordinary resources stay in YAML.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
NS=redhat-ai-gateway-infra
oc apply -f "${SCRIPT_DIR}/maas-namespace.yaml"
if oc get deployment postgres -n "$NS" -o json 2>/dev/null | \
  jq -e '.spec.template.spec.volumes[] | select(.name=="data") | has("emptyDir")' >/dev/null; then
  echo 'Existing ephemeral database detected. Keep the original lab checkout; this setup does not migrate its data.' >&2
  exit 1
fi
if oc get secret postgres-creds -n "$NS" >/dev/null 2>&1; then
  DB_PASSWORD=$(oc get secret postgres-creds -n "$NS" -o jsonpath='{.data.POSTGRESQL_PASSWORD}' | base64 -d)
else
  DB_PASSWORD=$(openssl rand -hex 24)
fi
[[ -n "$DB_PASSWORD" ]] || { echo 'Database password is empty.' >&2; exit 1; }
oc create secret generic postgres-creds -n "$NS" \
  --from-literal=POSTGRESQL_USER=maas --from-literal=POSTGRESQL_PASSWORD="$DB_PASSWORD" \
  --from-literal=POSTGRESQL_DATABASE=maas --dry-run=client -o yaml | oc apply -f -
oc create secret generic maas-db-config -n "$NS" \
  --from-literal=DB_CONNECTION_URL="postgresql://maas:${DB_PASSWORD}@postgres.${NS}.svc.cluster.local:5432/maas?sslmode=disable" \
  --dry-run=client -o yaml | oc apply -f -
unset DB_PASSWORD
echo 'MaaS database credentials prepared; no password printed.'
