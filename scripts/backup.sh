#!/usr/bin/env bash
# One archive with a consistent database dump and the uploaded files. Keeps the newest 14 locally and,
# when BACKUP_REMOTE is set (make backup-remote-setup), copies the archive to a second server too.
source "$(dirname "$0")/lib.sh"
mkdir -p backups
TS="$(date +%Y%m%d-%H%M)"; TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
OUTFILE="backups/evinki-$TS.tar.gz"
say "dumping database"
$COMPOSE exec -T db pg_dump -U evinki -Fc evinki > "$TMP/db.dump"
say "copying uploads"
if $COMPOSE exec -T api tar -C /data -cf - uploads > "$TMP/uploads.tar" 2>/dev/null; then
  ( cd "$TMP" && tar -czf "$ROOT/$OUTFILE" db.dump uploads.tar )
else
  warn "no uploads to copy"
  ( cd "$TMP" && tar -czf "$ROOT/$OUTFILE" db.dump )
fi
chmod 600 "$OUTFILE"
ls -1t backups/evinki-*.tar.gz | tail -n +15 | xargs -r rm -f
say "backup written: $OUTFILE ($(du -h "$OUTFILE" | cut -f1))"

REMOTE="$(env_get BACKUP_REMOTE)"
if [[ -n "$REMOTE" ]]; then
  KEY="$HOME/.ssh/evinki_backup"; PORT="$(env_get BACKUP_REMOTE_PORT)"
  if scp -q -i "$KEY" -P "${PORT:-22}" -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 "$OUTFILE" "${REMOTE%/}/" 2>/dev/null; then
    say "copied to ${REMOTE}"
  else
    warn "could not copy the backup to ${REMOTE} (check: make backup-remote-setup REMOTE=${REMOTE})"
  fi
fi
