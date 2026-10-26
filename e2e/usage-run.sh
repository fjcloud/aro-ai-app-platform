#!/usr/bin/env bash
# Make one real Foundry call and check the usage object for Act 3 observability.
set -euo pipefail
exec python3 "$(dirname "${BASH_SOURCE[0]}")/check.py" usage
