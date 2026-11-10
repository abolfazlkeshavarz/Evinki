#!/usr/bin/env bash
#
# RUN THIS ON THE NEW SERVER in the Evinki folder (e.g. /opt/evinki), with the file made by
# backup-attendance.sh on the old server.
#
#   ./scripts/legacy/import-attendance.sh attendance-migration-20261110-101500.tar.gz
#
# It: unpacks the file -> restores the old database into a scratch database -> shows the old counts ->
# rehearses the import (nothing saved) -> asks you to confirm -> imports -> keeps the old SECRET_KEY so the
# kiosk tablets keep working -> removes the scratch database -> restarts and checks the system.
# A safety backup of the new system is taken first. Re-running is safe (the importer skips what it already has).
#
# Options (environment variables)
#   YES=1       do not ask before the real import
#   DRY_ONLY=1  stop after the rehearsal
set -euo pipefail
source "$(dirname "$0")/../lib.sh"

FILE="${1:-}"
[[ -f "$FILE" ]] || die "usage: $0 attendance-migration-<date>.tar.gz   (the file made by backup-attendance.sh)"
[[ -f .env ]] || die ".env not found - run this in the Evinki installation folder."
# an exported variable from another project's .env would override this project's .env inside docker compose
for v in DB_PASSWORD JWT_SECRET POSTGRES_PASSWORD; do
  if [[ -n "${!v:-}" && "${!v}" != "$(env_get "$v")" && -n "$(env_get "$v")" ]]; then
    die "Your shell has a '$v' variable that differs from .env (left over from sourcing another project's .env). Log out and back in, then run this again."
  fi
done
$COMPOSE ps -q db | grep -q . || die "The Evinki stack is not running (make up)."

WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT
say "Unpacking $FILE"
tar -xzf "$FILE" -C "$WORK"
for f in db.sql.gz manifest.txt; do [[ -s "$WORK/$f" ]] || die "'$FILE' is not an attendance migration file (missing $f)."; done
sed 's/^/    /' "$WORK/manifest.txt"

say "Safety backup of the new system"
bash scripts/backup.sh >/dev/null

DBPW="$(env_get DB_PASSWORD)"
SCRATCH="legacy_attendance"
SRC="postgres://evinki:${DBPW}@db:5432/${SCRATCH}?sslmode=disable"
psql_db() { $COMPOSE exec -T db psql -U evinki -d "$1" -v ON_ERROR_STOP=1 -q "${@:2}"; }

say "Restoring the old database into a scratch database ($SCRATCH)"
psql_db postgres -c "DROP DATABASE IF EXISTS ${SCRATCH} WITH (FORCE)" -c "CREATE DATABASE ${SCRATCH}"
gunzip -c "$WORK/db.sql.gz" | $COMPOSE exec -T db psql -U evinki -d "$SCRATCH" -q -v ON_ERROR_STOP=0 >/dev/null 2>"$WORK/restore.log" || true
grep -i "error" "$WORK/restore.log" | head -3 || true
cleanup() { $COMPOSE exec -T db psql -U evinki -d postgres -q -c "DROP DATABASE IF EXISTS ${SCRATCH} WITH (FORCE)" >/dev/null 2>&1 || true; }
trap 'cleanup; rm -rf "$WORK"' EXIT

echo "Old system counts:"
psql_db "$SCRATCH" -At -c "SELECT 'employees: '||count(*) FROM employees" -c "SELECT 'attendance records: '||count(*) FROM attendance_records" \
  -c "SELECT 'devices: '||count(*) FROM devices" -c "SELECT 'leave requests: '||count(*) FROM leave_requests" -c "SELECT 'users: '||count(*) FROM users" | sed 's/^/    /' \
  || die "The restored database has no attendance tables; is this the right file?"

say "Rehearsal (nothing is saved)"
$COMPOSE run --rm --no-deps -T api evinkictl import attendance -src "$SRC" -dry-run
[[ "${DRY_ONLY:-0}" == "1" ]] && { say "DRY_ONLY=1: stopping here."; exit 0; }

if [[ "${YES:-0}" != "1" ]]; then
  read -r -p "The numbers look right? Type YES to import for real: " a
  [[ "$a" == "YES" ]] || die "cancelled (nothing was changed)"
fi

say "Importing"
$COMPOSE run --rm --no-deps -T api evinkictl import attendance -src "$SRC"

SECRET="$(tr -d '\r\n' < "$WORK/secret.txt" 2>/dev/null || true)"
if [[ -n "$SECRET" ]]; then
  say "Keeping the old SECRET_KEY so the kiosk tablets and readers keep working"
  set_env_value .env LEGACY_DEVICE_KEY_SECRET "$SECRET"
else
  warn "No SECRET_KEY in the file: register the tablets again in HR -> Devices."
fi

if [[ -s "$WORK/media.tar.gz" ]]; then
  mkdir -p legacy
  cp "$WORK/media.tar.gz" "legacy/attendance-media-$(date +%Y%m%d).tar.gz"
  say "Face photos kept for safekeeping in legacy/ (the recognition data itself is already in the database)"
fi

cleanup
say "Restarting the API"
$COMPOSE up -d --no-build
wait_healthy api 120 || warn "the API is not healthy yet; check: make logs"
echo ""
bash scripts/admin.sh stats | grep -E "users|employees|attendance_records" || true
echo ""
echo "================================================================"
echo " Attendance data imported."
echo "================================================================"
echo "  1. Admin panel -> HR -> Employees / Report: compare with the old numbers above."
echo "  2. Point each tablet / reader at the NEW address ($(env_get PUBLIC_URL)); their old device keys are accepted."
echo "  3. Employees keep their PINs; the old admin/manager/viewer users became staff users (passwords unchanged)."
echo "  4. Keep the old server read-only for a few weeks, then delete the migration file there and here."
