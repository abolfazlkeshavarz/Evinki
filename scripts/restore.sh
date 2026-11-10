#!/usr/bin/env bash
# Replaces the database and uploads with the contents of a backup archive.
#   make restore FILE=backups/evinki-YYYYmmdd-HHMM.tar.gz
# Non-interactive (used by migrate-import): YES=1 skips the question, SKIP_SAFETY=1 skips the safety backup.
source "$(dirname "$0")/lib.sh"
FILE="${1:-}"; [[ -f "$FILE" ]] || die "usage: make restore FILE=backups/evinki-YYYYmmdd-HHMM.tar.gz"
if [[ "${YES:-0}" != "1" ]]; then
  warn "This REPLACES all current data with the backup. A safety backup is taken first."
  read -r -p "Type YES to continue: " a; [[ "$a" == "YES" ]] || die "cancelled"
fi
[[ "${SKIP_SAFETY:-0}" == "1" ]] || bash scripts/backup.sh
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
tar -xzf "$FILE" -C "$TMP"
[[ -f "$TMP/db.dump" ]] || die "'$FILE' is not an Evinki backup (no db.dump inside)."
$COMPOSE stop api web
$COMPOSE exec -T db psql -U evinki -d postgres -c "DROP DATABASE IF EXISTS evinki WITH (FORCE)" -c "CREATE DATABASE evinki"
$COMPOSE exec -T db pg_restore -U evinki -d evinki --no-owner < "$TMP/db.dump"
if [[ -f "$TMP/uploads.tar" ]]; then
  $COMPOSE run --rm --no-deps -T --entrypoint sh api -c 'rm -rf /data/uploads/* && tar -C /data -xf -' < "$TMP/uploads.tar"
fi
$COMPOSE start api web
bash scripts/health.sh && say "restored from $FILE"
