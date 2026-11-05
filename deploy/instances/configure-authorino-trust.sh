#!/usr/bin/env bash
# Trust the OpenShift service CA for Authorino -> MaaS API HTTPS callbacks.
# Use the service-CA-signed listener expected by the MaaS gateway.
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
oc annotate service authorino-authorino-authorization -n kuadrant-system \
  service.beta.openshift.io/serving-cert-secret-name=authorino-server-cert --overwrite
oc patch authorino authorino -n kuadrant-system --type=merge \
  -p '{"spec":{"listener":{"tls":{"enabled":true,"certSecretRef":{"name":"authorino-server-cert"}}}}}'
oc annotate gateway maas-default-gateway -n openshift-ingress \
  security.opendatahub.io/authorino-tls-bootstrap=true --overwrite
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
echo 'Authorino service CA mounted; TLS verification enabled for internal callbacks.'
