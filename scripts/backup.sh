#!/usr/bin/env bash
# One archive with a consistent database dump and the uploaded files. Keeps the newest 14.
source "$(dirname "$0")/lib.sh"
mkdir -p backups
TS="$(date +%Y%m%d-%H%M)"; TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
say "dumping database"
$COMPOSE exec -T db pg_dump -U evinki -Fc evinki > "$TMP/db.dump"
say "copying uploads"
if $COMPOSE exec -T api tar -C /data -cf - uploads > "$TMP/uploads.tar" 2>/dev/null; then
  ( cd "$TMP" && tar -czf "$ROOT/backups/evinki-$TS.tar.gz" db.dump uploads.tar )
else
  warn "no uploads to copy"
  ( cd "$TMP" && tar -czf "$ROOT/backups/evinki-$TS.tar.gz" db.dump )
fi
chmod 600 "backups/evinki-$TS.tar.gz"
ls -1t backups/evinki-*.tar.gz | tail -n +15 | xargs -r rm -f
say "backup written: backups/evinki-$TS.tar.gz ($(du -h "backups/evinki-$TS.tar.gz" | cut -f1))"
