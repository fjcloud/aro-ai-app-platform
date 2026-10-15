#!/usr/bin/env bash
set -euo pipefail

# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/lib/request-kubeconfig.sh"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "No state/cluster.env. Run make deploy." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

request_admin_kubeconfig
echo "KUBECONFIG=${KUBECONFIG_FILE}"
echo "export KUBECONFIG=${KUBECONFIG_FILE}"
