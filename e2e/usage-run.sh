#!/usr/bin/env bash
# Make one real Foundry call and check the usage object before the Act 3 cleanup.
set -euo pipefail
exec python3 "$(dirname "${BASH_SOURCE[0]}")/check.py" usage
