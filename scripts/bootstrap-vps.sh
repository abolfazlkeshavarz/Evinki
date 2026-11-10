#!/usr/bin/env bash
#
# Zero-to-deployed on a fresh Ubuntu/Debian VPS: installs Docker, writes .env with generated secrets and
# FREE ports, builds (or loads) the images, starts the stack, then configures the host's nginx + a
# Let's Encrypt certificate and a nightly backup.
#
# Runs happily next to OTHER projects on the same server: only this project's containers are touched, TLS
# reuses the host's single nginx/certbot (scripts/deploy-host-nginx.sh), ports are picked when free.
#
#   DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=me@evinki.com ZARINPAL_MERCHANT_ID=xxxx ./scripts/bootstrap-vps.sh
#
# Options (environment variables)
#   SKIP_SSL=1     do not request a certificate yet (run `make ssl` once DNS points here)
#   WEB_PORT=8082  pick the local port yourself
#   SEED_DEMO=1    load the sample catalogue
# Prebuilt images (make images-bundle -> scp -> make load-images) are detected and used instead of building.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
source scripts/lib.sh

need_root "$0" "$@"
REAL_USER="${SUDO_USER:-root}"
command -v apt-get >/dev/null 2>&1 || die "This script is written only for Ubuntu/Debian (apt)."

say "Installing base packages"
apt-get update
apt-get install -y --no-install-recommends ca-certificates curl gnupg make openssl git cron
bash scripts/install-docker.sh
if [[ "$REAL_USER" != "root" ]] && ! id -nG "$REAL_USER" | grep -qw docker; then
  say "Adding $REAL_USER to the docker group (log out/in to use docker without sudo)"
  usermod -aG docker "$REAL_USER"
fi

# ---------------------------------------------------------------- domain + email
if [[ ! -f .env ]]; then
  [[ -z "${DOMAIN:-}" && -t 0 ]] && read -r -p "Domain whose A record points to this server (e.g. shop.evinki.com): " DOMAIN
  [[ -z "${LETSENCRYPT_EMAIL:-}" && -t 0 ]] && read -r -p "Email for Let's Encrypt expiry warnings: " LETSENCRYPT_EMAIL
  : "${DOMAIN:?DOMAIN is required}" "${LETSENCRYPT_EMAIL:?LETSENCRYPT_EMAIL is required}"
  if [[ -d /etc/nginx || -x /usr/sbin/nginx ]] && [[ -t 0 && -z "${WEB_PORT:-}" ]]; then
    auto="$(find_free_port 8090)"
    echo "nginx is already on this server (another project is probably deployed)."
    read -r -p "Local port for Evinki [${auto}]: " reply; WEB_PORT="${reply:-$auto}"
  fi
  say "Creating .env with generated secrets"
  DOMAIN="$DOMAIN" LETSENCRYPT_EMAIL="$LETSENCRYPT_EMAIL" WEB_PORT="${WEB_PORT:-}" bash scripts/init-env.sh
  GENERATED_ENV=1
else
  say ".env already exists here - keeping its secrets"
  if [[ -n "${DOMAIN:-}" && "$(clean_domain "$DOMAIN")" != "$(env_get APP_DOMAIN)" ]]; then
    echo "" >&2
    echo "This folder already has a .env for '$(env_get APP_DOMAIN)', but you asked for '$DOMAIN'." >&2
    echo "To move a live installation use:  make change-domain DOMAIN=$DOMAIN" >&2
    echo "If .env is a leftover from a test:  docker compose down -v && rm .env   (only where the data is disposable)" >&2
    exit 1
  fi
  if [[ -n "${WEB_PORT:-}" && "$(env_get WEB_PORT)" != "$WEB_PORT" ]]; then set_env_value .env WEB_PORT "$WEB_PORT"; fi
  load_env .env
  # the stored port is taken by something that is not our own container?
  if [[ -z "$($COMPOSE ps -q web 2>/dev/null)" ]] && port_in_use "${WEB_PORT:-8090}"; then
    die "Port ${WEB_PORT:-8090} (from .env) is used by another program. Run again with e.g. WEB_PORT=$(find_free_port 8091)"
  fi
  if [[ -z "$($COMPOSE ps -q db 2>/dev/null)" ]] && port_in_use "${DB_HOST_PORT:-15432}"; then
    np="$(find_free_port 15432)"; warn "Database port ${DB_HOST_PORT:-15432} is taken by another program; using ${np}"
    set_env_value .env DB_HOST_PORT "$np"
  fi
fi
[[ -n "${ZARINPAL_MERCHANT_ID:-}" && "$(env_get ZARINPAL_MERCHANT_ID)" != "$ZARINPAL_MERCHANT_ID" ]] && set_env_value .env ZARINPAL_MERCHANT_ID "$ZARINPAL_MERCHANT_ID"
[[ "$REAL_USER" != "root" ]] && chown "$REAL_USER":"$REAL_USER" .env
chmod 600 .env
load_env .env
[[ -n "$(env_get ZARINPAL_MERCHANT_ID)" ]] || warn "No ZARINPAL_MERCHANT_ID: online payment will not work until you set it in .env and run 'make up'"

# ---------------------------------------------------------------- images + start
if images_present; then
  say "Prebuilt images found; skipping the build"
  $COMPOSE up -d --no-build
else
  say "Building the images (slow on a small server). If it runs out of memory, build on your own PC:"
  echo "    make images-bundle   ->   scp dist/evinki-images.tar.gz to this folder   ->   make load-images"
  $COMPOSE up -d --build
fi
wait_healthy api 120 || { $COMPOSE logs --tail=40 api; die "The API did not become healthy (see the log above)."; }
wait_healthy web 60 || warn "web is not healthy yet; check: make logs"
[[ "${SEED_DEMO:-0}" == "1" ]] && $COMPOSE run --rm --no-deps -T api evinkictl seed-demo

# ---------------------------------------------------------------- TLS + nginx + backups
if [[ "${SKIP_SSL:-0}" == "1" ]]; then
  say "SKIP_SSL=1: not requesting a certificate yet (run 'make ssl' once DNS points here)"
else
  say "Obtaining the SSL certificate and configuring host nginx"
  bash scripts/deploy-host-nginx.sh
fi

say "Scheduling the nightly backup (02:30)"
( crontab -l 2>/dev/null | grep -v 'evinki-backup' || true; echo "30 2 * * * cd ${ROOT} && ./scripts/backup.sh >> backups/backup.log 2>&1 # evinki-backup" ) | crontab -
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null; say "Firewall: opened 80 and 443"
fi

echo ""
echo "================================================================"
echo " Evinki is live at $(env_get PUBLIC_URL)"
echo "================================================================"
echo "  Admin login:  $(env_get BOOTSTRAP_ADMIN_USER) / $(env_get BOOTSTRAP_ADMIN_PASSWORD)   (change it at first login)"
echo "  Next: set SMS patterns in the admin panel (Settings -> SMS patterns), then 'make health'"
echo "        optional: make seed-demo | make backup-remote-setup REMOTE=user@host:/path | docs/MIGRATION.md"
