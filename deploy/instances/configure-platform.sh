#!/usr/bin/env bash
# Post-apply for deploy/instances: Authorino CA trust,
# bind MaaS Gateway to this cluster (apps domain +
# default IngressController certificate), then dashboard MaaS UI flags.
set -euo pipefail
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bash "${SCRIPT_DIR}/configure-authorino-trust.sh"
bash "${SCRIPT_DIR}/configure-maas-gateway.sh"
bash "${SCRIPT_DIR}/configure-dashboard.sh"
echo "Platform instances configured (Authorino CA trust + MaaS Gateway + dashboard)."
