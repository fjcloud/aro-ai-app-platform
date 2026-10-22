#!/usr/bin/env bash
# Validate the already-installed Foundry-backed platform. Does not provision resources.
set -euo pipefail
exec python3 "$(dirname "${BASH_SOURCE[0]}")/check.py" platform
