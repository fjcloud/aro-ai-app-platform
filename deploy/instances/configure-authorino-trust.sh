#!/usr/bin/env bash
# Trust the OpenShift service CA for Authorino -> MaaS API HTTPS callbacks.
# Keep the gateway/Authorino listener protocol under Kuadrant's management.
set -euo pipefail
WAIT_SECONDS=${AUTHORINO_WAIT_SECONDS:-300}
[[ "$WAIT_SECONDS" =~ ^[1-9][0-9]*$ ]] || { echo 'AUTHORINO_WAIT_SECONDS must be positive.' >&2; exit 1; }
deadline=$((SECONDS + WAIT_SECONDS))
until oc --request-timeout=15s get deployment/authorino -n kuadrant-system >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "Authorino Deployment was not created; check Kuadrant and worker capacity." >&2
    exit 1
  fi
  sleep 2
done
python3 - <<'PY'
import json, subprocess
obj = json.loads(subprocess.check_output(['oc', 'get', 'authorino', 'authorino', '-n', 'kuadrant-system', '-o', 'json']))
volumes = obj.get('spec', {}).get('volumes', {})
items = [v for v in volumes.get('items', []) if v.get('name') != 'workshop-service-ca']
items.append({'name': 'workshop-service-ca', 'mountPath': '/etc/ssl/certs/openshift-service-ca',
              'configMaps': ['openshift-service-ca.crt'],
              'items': [{'key': 'service-ca.crt', 'path': 'service-ca-bundle.crt'}]})
subprocess.run(['oc', 'patch', 'authorino', 'authorino', '-n', 'kuadrant-system', '--type=merge',
                '-p', json.dumps({'spec': {'volumes': {**volumes, 'items': items}}})], check=True)
PY
oc -n kuadrant-system set env deployment/authorino \
  SSL_CERT_FILE=/etc/ssl/certs/openshift-service-ca/service-ca-bundle.crt \
  REQUESTS_CA_BUNDLE=/etc/ssl/certs/openshift-service-ca/service-ca-bundle.crt
oc rollout status deployment/authorino -n kuadrant-system --timeout=180s
echo 'Authorino service CA mounted; authentication and listener management remain with Kuadrant.'
