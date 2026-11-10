#!/usr/bin/env bash
#
# Moves this installation to a NEW main domain on the SAME server - one command instead of editing
# .env, nginx and certificates by hand.
#
#   make change-domain DOMAIN=newname.com
#   ./scripts/change-domain.sh newname.com [--drop-old]
#
# 1. backs up .env and rewrites APP_DOMAIN / PUBLIC_URL
# 2. restarts the app, gets a certificate + nginx entry for the new domain
# 3. keeps the OLD domain working as a redirect to the new one (--drop-old stops serving it)
#
# Before: the new domain's DNS A record must point to this server.
# Remember: if the payment gateway asks for the shop's domain in its panel, update it there too.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
source scripts/lib.sh

NEW="$(clean_domain "${1:-}")"
DROP=0; [[ "${2:-}" == "--drop-old" ]] && DROP=1
[[ -n "$NEW" ]] || die "Usage: change-domain.sh <new-domain> [--drop-old]"
valid_hostname "$NEW" || die "'$NEW' is not a valid domain name."
[[ -f .env ]] || die ".env not found - is this the installation folder?"
load_env .env
OLD="${APP_DOMAIN:-}"
[[ "$NEW" != "$OLD" ]] || { echo "Already using $NEW - nothing to do."; exit 0; }
getent hosts "$NEW" >/dev/null 2>&1 || warn "$NEW has no DNS record yet - the certificate step will fail until it points here."

backup=".env.bak-$(date +%Y%m%d-%H%M%S)"
cp .env "$backup"; say "Saved the previous settings to $backup"
apply_domain_env .env "$NEW"
load_env .env   # compose lets the environment override .env: reload so the restart sees the NEW domain
say "Restarting the app with the new address"
$COMPOSE up -d

say "Certificate and nginx for ${NEW}"
if ! bash scripts/deploy-host-nginx.sh; then
  warn "The certificate step failed. ${OLD:-The old domain} keeps working; previous settings are in $backup"
  warn "Fix DNS, then run: make ssl"
  exit 1
fi

if [[ -n "$OLD" && "$OLD" != "localhost" ]]; then
  if [[ "$DROP" == "1" ]]; then
    say "Dropping the old domain"; bash scripts/remove-domain.sh "$OLD" || true
  else
    say "Keeping $OLD as a redirect to $NEW"; bash scripts/add-domain.sh "$OLD" || warn "could not set up the redirect for $OLD"
  fi
fi
echo ""
echo "================================================================"
echo " Now live at https://${NEW}"
echo "================================================================"
echo "  Check: make deploy-health HOST=https://${NEW}"
