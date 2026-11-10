#!/usr/bin/env bash
#
# Stops serving an extra domain from this server: removes its nginx entry and, after confirmation,
# its certificate.   make remove-domain DOMAIN=www.shop.evinki.com
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(id -u)" != "0" ]]; then echo "==> Root access is required; re-running with sudo"; exec sudo -E bash "$0" "$@"; fi
# shellcheck disable=SC1091
source scripts/lib.sh
load_env .env 2>/dev/null || true

EXTRA="$(clean_domain "${1:-}")"
valid_hostname "$EXTRA" || die "Usage: remove-domain.sh <domain>"
[[ "$EXTRA" != "${APP_DOMAIN:-}" ]] || die "Refusing: $EXTRA is the main domain. Use make change-domain first."

if [[ -e "/etc/nginx/sites-enabled/${EXTRA}.conf" || -e "/etc/nginx/sites-available/${EXTRA}.conf" ]]; then
  rm -f "/etc/nginx/sites-enabled/${EXTRA}.conf" "/etc/nginx/sites-available/${EXTRA}.conf"
  nginx -t && systemctl reload nginx
  say "Removed the nginx entry for ${EXTRA}."
else
  say "No nginx entry found for ${EXTRA}."
fi
if [[ -d "/etc/letsencrypt/live/${EXTRA}" ]]; then
  read -r -p "Also delete the certificate for ${EXTRA}? [y/N] " reply
  if [[ "$reply" =~ ^[Yy]$ ]]; then certbot delete --cert-name "${EXTRA}" --non-interactive; fi
fi
[[ -f .env ]] && remove_extra_domain "$EXTRA"
echo "Done."
