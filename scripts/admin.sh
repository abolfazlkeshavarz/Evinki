#!/usr/bin/env bash
#
# Runs the operator tool (evinkictl) inside the running stack:
#   scripts/admin.sh create-user -username ali -password 'Secret-123' -roles admin
#   scripts/admin.sh reset-password -username ali -password 'New-secret-1'
#   scripts/admin.sh stats | seed-demo | import warehouse -src ...
set -euo pipefail
cd "$(dirname "$0")/.."
DOCKER="${DOCKER:-docker}"
if "$DOCKER" compose ps -q api 2>/dev/null | grep -q .; then
  if [[ -t 0 ]]; then exec "$DOCKER" compose exec api evinkictl "$@"; fi
  exec "$DOCKER" compose exec -T api evinkictl "$@"
fi
exec "$DOCKER" compose run --rm --no-deps -T api evinkictl "$@"
