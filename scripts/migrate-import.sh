#!/usr/bin/env bash
#
# Sets up a NEW server from a migration bundle (make export on the old server) or from a plain backup
# archive (the nightly backups / the off-server copy), restoring the database and the uploads.
#
#   git clone https://github.com/abolfazlkeshavarz/Evinki.git /opt/evinki && cd /opt/evinki
#   ./scripts/migrate-import.sh evinki-migration-20261110-101500.tar.gz
#   ./scripts/migrate-import.sh bundle.tar.gz --domain newname.com        # the domain changes too
#   ./scripts/migrate-import.sh bundle.tar.gz --no-ssl                    # DNS not switched yet: no certificate now
#
# From a plain backup archive there is no .env, so tell it what to create:
#   DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=me@evinki.com ./scripts/migrate-import.sh evinki-2026....tar.gz
#
# Docker already installed and you only want the data restored (no nginx/TLS changes)?  add --skip-install
# Safe order for a zero-downtime move: run with --no-ssl, test the new server, then switch DNS and run `make ssl`.
set -euo pipefail
cd "$(dirname "$0")/.."
ORIG_ARGS=("$@")
FILE=""; NEW_DOMAIN=""; NO_SSL=0; SKIP_INSTALL=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) NEW_DOMAIN="$(tr 'A-Z' 'a-z' <<<"${2:?--domain needs a value}")"; shift 2 ;;
    --no-ssl) NO_SSL=1; shift ;;
    --skip-install) SKIP_INSTALL=1; NO_SSL=1; shift ;;
    -h|--help) sed -n '2,15p' "$0"; exit 0 ;;
    *) FILE="$1"; shift ;;
  esac
done
[[ -n "$FILE" && -f "$FILE" ]] || { echo "Usage: migrate-import.sh <bundle-or-backup.tar.gz> [--domain NEW] [--no-ssl] [--skip-install]" >&2; exit 1; }
FILE="$(cd "$(dirname "$FILE")" && pwd)/$(basename "$FILE")"
if [[ "$SKIP_INSTALL" == "0" && "$(id -u)" != "0" ]]; then echo "==> Root access is required; re-running with sudo"; exec sudo -E bash "$0" "${ORIG_ARGS[@]}"; fi
# shellcheck disable=SC1091
source scripts/lib.sh
WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

# ------------------------------------------------------------ what did we get?
LISTING="$(tar -tzf "$FILE" 2>/dev/null || true)"
if grep -qx 'backup.tar.gz' <<<"$LISTING"; then
  say "Migration bundle detected"
  tar -xzf "$FILE" -C "$WORK"; DATA="$WORK/backup.tar.gz"; ENV_SRC="$WORK/env"
  [[ -f "$WORK/manifest.txt" ]] && sed 's/^/    /' "$WORK/manifest.txt"
elif grep -qx 'db.dump' <<<"$LISTING"; then
  say "Plain backup archive detected"; DATA="$FILE"; ENV_SRC=""
else
  die "'$FILE' is neither a migration bundle nor an Evinki backup."
fi

# ------------------------------------------------------------ settings (.env)
if [[ -f .env ]]; then
  say ".env already exists here - keeping it"
elif [[ -n "$ENV_SRC" && -f "$ENV_SRC" ]]; then
  say "Using the settings from the bundle"; cp "$ENV_SRC" .env; chmod 600 .env
  # ports of the OLD server may be taken here: pick free ones
  set_env_value .env WEB_PORT "$(find_free_port "$(env_get WEB_PORT || echo 8090)")"
  set_env_value .env DB_HOST_PORT "$(find_free_port 15432)"
else
  : "${DOMAIN:?A plain backup has no settings: pass DOMAIN=... LETSENCRYPT_EMAIL=...}" "${LETSENCRYPT_EMAIL:?pass LETSENCRYPT_EMAIL=...}"
  say "Creating new settings"; DOMAIN="$DOMAIN" LETSENCRYPT_EMAIL="$LETSENCRYPT_EMAIL" bash scripts/init-env.sh
fi
load_env .env
OLD_DOMAIN="${APP_DOMAIN:-}"
if [[ -n "$NEW_DOMAIN" && "$NEW_DOMAIN" != "$OLD_DOMAIN" ]]; then
  valid_hostname "$NEW_DOMAIN" || die "'$NEW_DOMAIN' is not a valid domain."
  say "Switching the domain: ${OLD_DOMAIN:-none} -> $NEW_DOMAIN"
  apply_domain_env .env "$NEW_DOMAIN"; load_env .env
fi
: "${APP_DOMAIN:?APP_DOMAIN missing in .env}"

# ------------------------------------------------------------ start empty stack, then restore
export SKIP_SSL="$NO_SSL" DOMAIN="$APP_DOMAIN" LETSENCRYPT_EMAIL="${LETSENCRYPT_EMAIL:-}"
if [[ "$SKIP_INSTALL" == "1" ]]; then
  say "--skip-install: Docker is assumed to be installed; starting the stack"
  if images_present; then $COMPOSE up -d --no-build; else $COMPOSE up -d --build; fi
  wait_healthy api 120 || die "The API did not become healthy; check: make logs"
else
  say "Installing Docker and starting the stack"
  bash scripts/bootstrap-vps.sh
fi
say "Restoring the data"
YES=1 SKIP_SAFETY=1 bash scripts/restore.sh "$DATA"

# ------------------------------------------------------------ extra domains
pending=()
if [[ -f "$WORK/domains.txt" && -s "$WORK/domains.txt" ]]; then
  say "Extra domains found in the bundle:"
  server_ip="$(curl -fsS --max-time 10 https://api.ipify.org || true)"
  while IFS= read -r d; do
    [[ -n "$d" ]] || continue
    ip="$(getent hosts "$d" | awk '{print $1}' | head -1 || true)"
    if [[ "$NO_SSL" == "0" && -n "$server_ip" && "$ip" == "$server_ip" ]]; then bash scripts/add-domain.sh "$d" || pending+=("$d"); else pending+=("$d"); fi
  done < "$WORK/domains.txt"
fi

echo ""; bash scripts/health.sh || true
echo ""
echo "================================================================"
echo " Migration finished: $(env_get PUBLIC_URL)"
echo "================================================================"
echo "  Sign in with the same accounts as before."
[[ "$NO_SSL" == "1" ]] && echo "  HTTPS was skipped. Once DNS points to this server:  make ssl"
if ((${#pending[@]})); then echo "  Extra domains still to enable (after their DNS points here):"; for d in "${pending[@]}"; do echo "     make add-domain DOMAIN=$d"; done; fi
echo "  Then switch your DNS to this server and check:  make deploy-health HOST=$(env_get PUBLIC_URL)"
echo "  Finally delete the migration bundle - it contains secrets."
