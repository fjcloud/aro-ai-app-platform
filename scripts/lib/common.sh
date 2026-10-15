# Shared paths for the deploy scripts. Source this after `set -euo pipefail`.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AZ="${AZ:-az}"
ENV_FILE="${ROOT}/state/cluster.env"
