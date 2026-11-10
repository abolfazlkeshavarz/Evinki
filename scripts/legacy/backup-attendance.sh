#!/usr/bin/env bash
#
# RUN THIS ON THE OLD ATTENDANCE SERVER, inside the attendance project folder
# (the one with its docker-compose.yml and .env, e.g. /root/attendance-system).
#
#   bash backup-attendance.sh                 # from the project folder
#   bash /path/to/backup-attendance.sh /root/attendance-system
#
# It makes ONE file, attendance-migration-<date>.tar.gz, containing
#   db.sql.gz       the whole database (employees, face data, punches, leaves, tasks, devices)
#   media.tar.gz    face photos and snapshots
#   secret.txt      the old SECRET_KEY (keeps the kiosk tablets/readers working after the move)
#   manifest.txt    what is inside and when it was made
# The file contains secrets: copy it to the new server, then delete it from here.
# Nothing is changed in the old system. It is not self-contained with the rest of the repo on purpose,
# so you can copy just this one file to the old server.
set -euo pipefail

DIR="${1:-$(pwd)}"
cd "$DIR"
[[ -f docker-compose.yml && -f .env ]] || { echo "xx This is not the attendance project folder (no docker-compose.yml/.env in $DIR)." >&2; exit 1; }
command -v docker >/dev/null || { echo "xx docker is not installed." >&2; exit 1; }

# read values WITHOUT 'source' (sourcing would run every line as a command and leak variables into your shell)
envv() { grep -E "^$1=" .env | head -1 | cut -d= -f2- | tr -d '\r' | sed -E 's/^"(.*)"$/\1/'; }
PGUSER="$(envv POSTGRES_USER)"; PGUSER="${PGUSER:-attendance}"
PGDB="$(envv POSTGRES_DB)";     PGDB="${PGDB:-attendance}"
SECRET="$(envv SECRET_KEY)"
[[ -n "$SECRET" ]] || echo "!! SECRET_KEY not found in .env - tablets will need to be re-registered after the move." >&2

STAMP="$(date +%Y%m%d-%H%M%S)"
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT
OUT="$DIR/attendance-migration-$STAMP.tar.gz"

echo "==> Checking the containers"
docker compose ps -q db | grep -q . || { echo "xx The database container is not running (docker compose up -d)." >&2; exit 1; }

echo "==> Dumping the database ($PGDB)"
docker compose exec -T db pg_dump -U "$PGUSER" --no-owner --no-privileges "$PGDB" | gzip > "$WORK/db.sql.gz"
[[ $(stat -c %s "$WORK/db.sql.gz") -gt 2000 ]] || { echo "xx The dump is suspiciously small. Check: docker compose logs db" >&2; exit 1; }
# a dump must contain the main tables, otherwise the import would silently find nothing
for t in employees attendance_records; do
  gunzip -c "$WORK/db.sql.gz" | grep -q "CREATE TABLE public.$t " || { echo "xx Table $t is missing in the dump." >&2; exit 1; }
done
EMP="$(gunzip -c "$WORK/db.sql.gz" | awk '/^COPY public.employees /{f=1;next} f&&/^\\\./{exit} f{n++} END{print n+0}')"
PUN="$(gunzip -c "$WORK/db.sql.gz" | awk '/^COPY public.attendance_records /{f=1;next} f&&/^\\\./{exit} f{n++} END{print n+0}')"

echo "==> Copying the face photos and snapshots"
if docker compose ps -q backend | grep -q .; then
  docker compose exec -T backend tar czf - -C /app/app/static . > "$WORK/media.tar.gz" || { echo "!! could not read the media folder; continuing without it" >&2; : > "$WORK/media.tar.gz"; }
else
  echo "!! the backend container is not running; media not included" >&2; : > "$WORK/media.tar.gz"
fi

printf '%s' "$SECRET" > "$WORK/secret.txt"; chmod 600 "$WORK/secret.txt"
{
  echo "created: $(date -Iseconds)"
  echo "source: attendance-system"
  echo "database: $PGDB"
  echo "employees: $EMP"
  echo "attendance_records: $PUN"
  echo "media_bytes: $(stat -c %s "$WORK/media.tar.gz")"
} > "$WORK/manifest.txt"

tar -czf "$OUT" -C "$WORK" db.sql.gz media.tar.gz secret.txt manifest.txt
chmod 600 "$OUT"
echo ""
echo "Done: $OUT  ($(du -h "$OUT" | cut -f1))"
sed 's/^/   /' "$WORK/manifest.txt"
echo ""
echo "Next:"
echo "  scp $OUT root@NEW_SERVER:/opt/evinki/"
echo "  on the new server:   cd /opt/evinki && ./scripts/legacy/import-attendance.sh $(basename "$OUT")"
echo "  then delete $OUT from this server (it contains the secret key)."
