#!/usr/bin/env bash
# Imports a backup of one of the old systems. See docs/MIGRATION.md.
#   ./scripts/import-legacy.sh warehouse  -src sqlite:/legacy/warehouse.db -images /legacy/parts [-dry-run]
#   ./scripts/import-legacy.sh attendance -src sqlite:/legacy/attendance.db [-dry-run]
#   ./scripts/import-legacy.sh guarantee  -src "postgres://user:pass@host.docker.internal:5432/db" [-dry-run]
# Put backup files in ./legacy (mounted read-only at /legacy). Always rehearse with -dry-run first.
source "$(dirname "$0")/lib.sh"
KIND="${1:-}"; shift || true
[ -n "$KIND" ] || die "usage: import-legacy.sh <warehouse|guarantee|attendance> -src <db> [-dry-run]"
mkdir -p legacy
./scripts/backup.sh >/dev/null && say "safety backup taken"
$COMPOSE run --rm --no-deps -T -v "$ROOT/legacy:/legacy:ro" api evinkictl import "$KIND" "$@"
