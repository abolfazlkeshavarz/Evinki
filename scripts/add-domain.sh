#!/usr/bin/env bash
#
# Makes an EXTRA domain (e.g. www.shop.evinki.com, or evinki-shop.ir) open the shop: gets a free
# Let's Encrypt certificate for it and a vhost that redirects (301) to the main domain, so there is
# one address in Google, in payment callbacks and in SMS links.
#
#   make add-domain DOMAIN=www.shop.evinki.com
#
# Before: the extra domain's DNS A record must point to THIS server.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(id -u)" != "0" ]]; then echo "==> Root access is required; re-running with sudo"; exec sudo -E bash "$0" "$@"; fi
# shellcheck disable=SC1091
source scripts/lib.sh
load_env .env || exit 1

EXTRA="$(clean_domain "${1:-}")"
[[ -n "$EXTRA" ]] || die "Usage: add-domain.sh <domain>"
valid_hostname "$EXTRA" || die "'$EXTRA' is not a valid domain name."
MAIN="${APP_DOMAIN:-}"
[[ "$EXTRA" != "$MAIN" ]] || die "$EXTRA is already the main domain."
: "${LETSENCRYPT_EMAIL:?Set LETSENCRYPT_EMAIL in .env}"
APP_HTTP_PORT="${WEB_PORT:-8090}"
VHOST_PATH="/etc/nginx/sites-available/${EXTRA}.conf"
VHOST_LINK="/etc/nginx/sites-enabled/${EXTRA}.conf"
CERT_PATH="/etc/letsencrypt/live/${EXTRA}"
WEBROOT="/var/www/certbot"
STAGING_FLAG=(); [[ "${LETSENCRYPT_STAGING:-false}" == "true" ]] && STAGING_FLAG=(--staging)

say "Installing nginx/certbot if needed"
apt-get update && apt-get install -y --no-install-recommends nginx certbot curl
systemctl enable --now nginx
mkdir -p "$WEBROOT"

say "Checking that ${EXTRA} points to this server"
server_ip="$(curl -fsS --max-time 10 https://api.ipify.org || true)"
domain_ip="$(getent hosts "$EXTRA" | awk '{print $1}' | head -1 || true)"
[[ -n "$domain_ip" ]] || die "${EXTRA} has no DNS record yet. Add an A record -> ${server_ip:-the IP of this server}, wait a few minutes, run again."
if [[ -n "$server_ip" && "$server_ip" != "$domain_ip" ]]; then
  warn "${EXTRA} resolves to ${domain_ip}, this server is ${server_ip}."
  if [[ -t 0 ]]; then read -r -p "Continue anyway? [y/N] " r; [[ "$r" =~ ^[Yy]$ ]] || exit 1; else exit 1; fi
fi

if [[ ! -d "$CERT_PATH" ]]; then
  say "Temporary HTTP vhost for the certificate challenge"
  cat > "$VHOST_PATH" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${EXTRA};
    location ^~ /.well-known/acme-challenge/ { root ${WEBROOT}; default_type "text/plain"; }
    location / { return 301 https://${MAIN}\$request_uri; }
}
EOF
  ln -sf "$VHOST_PATH" "$VHOST_LINK"
  nginx -t && systemctl reload nginx
  certbot certonly --webroot -w "$WEBROOT" "${STAGING_FLAG[@]}" --email "$LETSENCRYPT_EMAIL" -d "$EXTRA" \
    --agree-tos --no-eff-email --non-interactive
fi

say "Writing the redirect vhost"
cat > "$VHOST_PATH" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name ${EXTRA};
    location ^~ /.well-known/acme-challenge/ { root ${WEBROOT}; default_type "text/plain"; }
    location / { return 301 https://${MAIN}\$request_uri; }
}
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ${EXTRA};
    ssl_certificate     /etc/letsencrypt/live/${EXTRA}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${EXTRA}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    return 301 https://${MAIN}\$request_uri;
}
EOF
ln -sf "$VHOST_PATH" "$VHOST_LINK"
nginx -t && systemctl reload nginx
add_extra_domain "$EXTRA"
echo ""
echo "Done: https://${EXTRA} now redirects to https://${MAIN} (certificate renews automatically)."
