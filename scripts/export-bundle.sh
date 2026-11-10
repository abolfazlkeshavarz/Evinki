#!/usr/bin/env bash
#
# Packs EVERYTHING needed to move to a new server into one file: a fresh backup (database + uploads) plus this
# installation's settings (.env) and the list of extra domains.
#
#   make export
#   ./scripts/export-bundle.sh [output-file]
#
# The bundle contains secrets (.env) - treat it like a password and delete it after the move.
# On the NEW server:   ./scripts/migrate-import.sh evinki-migration-....tar.gz
source "$(dirname "$0")/lib.sh"
[[ -f .env ]] || die ".env not found - run this in the installation folder."
load_env .env
$COMPOSE ps -q api | grep -q . || die "The stack is not running (make up first)."
TS="$(date +%Y%m%d-%H%M%S)"
OUT="${1:-dist/evinki-migration-${TS}.tar.gz}"
mkdir -p "$(dirname "$OUT")"
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

say "Taking a fresh backup (database and uploads)"
bash scripts/backup.sh >/dev/null
newest="$(ls -1t backups/evinki-*.tar.gz | head -1)"
[[ -n "$newest" ]] || die "Backup failed."
cp "$newest" "$WORK/backup.tar.gz"

say "Adding settings and the list of extra domains"
cp .env "$WORK/env"
extra_domains > "$WORK/domains.txt" || true
{
  echo "created: $(date -Iseconds)"
  echo "domain: ${APP_DOMAIN:-}"
  echo "version: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  echo "extra_domains: $(wc -l < "$WORK/domains.txt" | tr -d ' ')"
} > "$WORK/manifest.txt"
tar -czf "$OUT" -C "$WORK" backup.tar.gz env domains.txt manifest.txt
chmod 600 "$OUT"
echo ""
echo "Bundle ready: $OUT  ($(du -h "$OUT" | cut -f1))"
sed 's/^/  /' "$WORK/manifest.txt"
echo ""
echo "Next:"
echo "  1. scp $OUT USER@NEW_SERVER:/opt/evinki/"
echo "  2. On the new server (fresh Ubuntu/Debian):"
echo "       git clone https://github.com/abolfazlkeshavarz/Evinki.git /opt/evinki && cd /opt/evinki"
echo "       ./scripts/migrate-import.sh $(basename "$OUT")        # add  --domain NEW.com  if the domain changes"
echo "  3. Switch DNS to the new server once it answers, then delete the bundle (it holds secrets)."
