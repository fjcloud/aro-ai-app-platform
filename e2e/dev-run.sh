#!/usr/bin/env bash
# Validate the app built by following Act 2 in a real Dev Space. Does not create a simulation pod.
set -euo pipefail
exec python3 "$(dirname "${BASH_SOURCE[0]}")/check.py" developer
