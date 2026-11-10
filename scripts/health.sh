#!/usr/bin/env bash
# Checks the stack end to end through the web container, like a browser would.
source "$(dirname "$0")/lib.sh"
PORT="$(env_get HTTP_PORT)"; PORT="${PORT:-127.0.0.1:8090}"
URL="${1:-http://${PORT}}"
for i in $(seq 1 30); do
  if curl -fsS "$URL/healthz" >/dev/null 2>&1 && curl -fsS "$URL/api/v1/version" >/dev/null 2>&1; then
    say "healthy: $URL"
    $COMPOSE ps 2>/dev/null || true
    LAST=$(ls -1t backups/*.tar.gz 2>/dev/null | head -1 || true)
    if [ -z "$LAST" ]; then warn "no backup exists yet: run 'make backup'"
    elif [ -n "$(find "$LAST" -mtime +2 2>/dev/null)" ]; then warn "latest backup is older than 2 days: $LAST"; fi
    exit 0
  fi
  sleep 2
done
die "not healthy after 60 s; see: make logs"
