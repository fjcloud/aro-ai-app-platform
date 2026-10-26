#!/usr/bin/env bash
# Post-apply for deploy/instances: Authorino TLS,
# bind MaaS Gateway to this cluster (apps domain +
# default IngressController certificate), recycle Kuadrant after
# Gateway API CRDs exist, then dashboard GPU taints + MaaS UI flags.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bash "${SCRIPT_DIR}/configure-authorino-tls.sh"
bash "${SCRIPT_DIR}/configure-maas-gateway.sh"
bash "${SCRIPT_DIR}/configure-kuadrant-gateway.sh"
bash "${SCRIPT_DIR}/configure-dashboard.sh"
echo "Platform instances configured (Authorino TLS + MaaS Gateway + Kuadrant + dashboard)."
