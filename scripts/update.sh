#!/usr/bin/env bash
# Safe update on a server that has the git checkout: backup, pull (fast-forward only), rebuild, restart,
# verify. Rolls the code back if unhealthy. (From your PC you can also use: make deploy-remote HOST=...)
source "$(dirname "$0")/lib.sh"
need git
PREV="$(git rev-parse HEAD)"
bash scripts/backup.sh
git pull --ff-only
$COMPOSE up -d --build
if ! bash scripts/health.sh; then
  warn "new version unhealthy: going back to $PREV"
  git checkout "$PREV" && $COMPOSE up -d --build && bash scripts/health.sh
  die "update failed and was rolled back (migrations are additive; data is untouched)"
fi
say "updated to $(git rev-parse --short HEAD)"
