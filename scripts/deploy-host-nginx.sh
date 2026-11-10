#!/usr/bin/env bash
#
# Makes the HOST's nginx the reverse proxy + TLS terminator for Evinki and obtains its Let's Encrypt
# certificate (HTTP-01, auto-renewing).
#
# Why host-level: the web container serves plain HTTP on a loopback-only port (WEB_BIND/WEB_PORT in .env).
# One server can host several projects on their own (sub)domains, but only one process can own port 443,
# so nginx lives on the host and each project gets its own vhost file. This script only writes the file
# named after THIS project's domain - it never edits another project's.
#
#   make ssl            (DNS A record of APP_DOMAIN must already point to this server)
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(id -u)" != "0" ]]; then echo "==> Root access is required to configure nginx/certbot; re-running with sudo"; exec sudo -E bash "$0" "$@"; fi
command -v apt-get >/dev/null 2>&1 || { echo "Ubuntu/Debian (apt) only." >&2; exit 1; }

# shellcheck disable=SC1091
source scripts/lib.sh
load_env .env || exit 1
DOMAIN="${APP_DOMAIN:-}"
[[ -n "$DOMAIN" && "$DOMAIN" != "localhost" ]] || die "Set APP_DOMAIN in .env (e.g. shop.evinki.com)"
: "${LETSENCRYPT_EMAIL:?Set LETSENCRYPT_EMAIL in .env}"
APP_HTTP_PORT="${WEB_PORT:-8090}"
SERVER_NAMES="$DOMAIN"
CERT_NAME="$DOMAIN"
VHOST_PATH="/etc/nginx/sites-available/${DOMAIN}.conf"
VHOST_LINK="/etc/nginx/sites-enabled/${DOMAIN}.conf"
CERT_PATH="/etc/letsencrypt/live/${CERT_NAME}"
WEBROOT="/var/www/certbot"
STAGING_FLAG=()
if [[ "${LETSENCRYPT_STAGING:-false}" == "true" ]]; then
  warn "Let's Encrypt staging mode: the certificate will NOT be trusted by browsers."
  STAGING_FLAG=(--staging)
fi

say "Installing nginx/certbot if needed (existing installs are left alone)"
apt-get update
apt-get install -y --no-install-recommends nginx certbot gettext-base curl
[[ -d /etc/nginx/sites-enabled ]] || die "Expected the Debian/Ubuntu nginx sites-enabled layout."
systemctl enable --now nginx
mkdir -p "$WEBROOT"

say "Installing the shared certbot renewal hook"
mkdir -p /etc/letsencrypt/renewal-hooks/deploy
printf '#!/bin/sh\nnginx -t && systemctl reload nginx\n' > /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh
chmod +x /etc/letsencrypt/renewal-hooks/deploy/reload-nginx.sh

say "Checking that ${DOMAIN} points to this server"
server_ip="$(curl -fsS --max-time 10 https://api.ipify.org || true)"
domain_ip="$(getent hosts "${DOMAIN}" | awk '{print $1}' | head -1 || true)"
if [[ -z "$domain_ip" ]]; then
  die "${DOMAIN} has no DNS record yet. Add an A record -> ${server_ip:-the IP of this server}, wait a few minutes, then run: make ssl"
fi
if [[ -n "$server_ip" && "$server_ip" != "$domain_ip" ]]; then
  warn "${DOMAIN} resolves to ${domain_ip}, this server is ${server_ip}."
  if [[ -t 0 ]]; then read -r -p "Continue anyway? [y/N] " reply; [[ "$reply" =~ ^[Yy]$ ]] || exit 1; else exit 1; fi
fi

if [[ -d "$CERT_PATH" ]]; then
  say "Certificate for ${DOMAIN} already exists - re-rendering the vhost only"
else
  say "Writing a temporary HTTP-only vhost for the certificate challenge"
  cat > "$VHOST_PATH" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${SERVER_NAMES};
    location ^~ /.well-known/acme-challenge/ { root ${WEBROOT}; default_type "text/plain"; }
    location / { proxy_pass http://127.0.0.1:${APP_HTTP_PORT}; proxy_set_header Host \$host; }
}
EOF
  ln -sf "$VHOST_PATH" "$VHOST_LINK"
  nginx -t && systemctl reload nginx
  say "Requesting the certificate (HTTP-01)"
  certbot certonly --webroot -w "$WEBROOT" "${STAGING_FLAG[@]}" --email "${LETSENCRYPT_EMAIL}" \
    -d "${DOMAIN}" --agree-tos --no-eff-email --non-interactive
fi

say "Writing the HTTPS vhost from deploy/nginx/app.conf.template"
SERVER_NAMES="$SERVER_NAMES" CERT_NAME="$CERT_NAME" APP_HTTP_PORT="$APP_HTTP_PORT" \
  envsubst '${SERVER_NAMES} ${CERT_NAME} ${APP_HTTP_PORT}' < deploy/nginx/app.conf.template > "$VHOST_PATH"
ln -sf "$VHOST_PATH" "$VHOST_LINK"
nginx -t
systemctl reload nginx

echo ""
echo "Done: https://${DOMAIN} -> nginx -> 127.0.0.1:${APP_HTTP_PORT} (the Evinki web container)."
echo "Renewal is automatic through certbot's systemd timer."
