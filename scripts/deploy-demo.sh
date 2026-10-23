#!/bin/bash
# Deploy remotely built review artifacts into fresh state.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec python3 "$here/deploy-review.py" "$@"
