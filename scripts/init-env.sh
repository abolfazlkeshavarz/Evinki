#!/usr/bin/env bash
#
# Creates .env from .env.example with generated secrets and FREE local ports, so a new install never
# ships a placeholder password and never collides with another project on the same server.
#
#   make env
#   DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=me@evinki.com make env
#   DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=me@evinki.com ZARINPAL_MERCHANT_ID=xxxx make env
#   DEV=1 make env                       # local testing: http, mock payment, no domain
#
# Never overwrites an existing .env (delete it yourself if you really want a new one).
source "$(dirname "$0")/lib.sh"

if [[ -f .env ]]; then say ".env already exists - leaving it untouched"; exit 0; fi
cp .env.example .env

ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(rand_secret 18)}"
set_env_value .env DB_PASSWORD "$(rand_secret 32)"
set_env_value .env JWT_SECRET "$(rand_secret 64)"
set_env_value .env BOOTSTRAP_ADMIN_PASSWORD "$ADMIN_PASSWORD"
[[ -n "${ADMIN_USERNAME:-}" ]] && set_env_value .env BOOTSTRAP_ADMIN_USER "$ADMIN_USERNAME"

# ports: reuse what the caller asked for, otherwise the first free one (another project may own 8090/15432)
WEB_PORT="${WEB_PORT:-$(find_free_port 8090)}"
DB_HOST_PORT="${DB_HOST_PORT:-$(find_free_port 15432)}"
set_env_value .env WEB_PORT "$WEB_PORT"
set_env_value .env DB_HOST_PORT "$DB_HOST_PORT"
set_env_value .env WEB_BIND "${WEB_BIND:-127.0.0.1}"

if [[ "${DEV:-0}" == "1" ]]; then
  set_env_value .env APP_ENV development
  set_env_value .env PAYMENT_GATEWAY mock
  set_env_value .env APP_DOMAIN localhost
  set_env_value .env PUBLIC_URL "http://127.0.0.1:${WEB_PORT}"
else
  if [[ -n "${DOMAIN:-}" ]]; then
    DOMAIN="$(clean_domain "$DOMAIN")"
    valid_hostname "$DOMAIN" || die "'$DOMAIN' is not a valid domain name"
    apply_domain_env .env "$DOMAIN"
  fi
  [[ -n "${LETSENCRYPT_EMAIL:-}" ]] && set_env_value .env LETSENCRYPT_EMAIL "$LETSENCRYPT_EMAIL"
  [[ -n "${ZARINPAL_MERCHANT_ID:-}" ]] && set_env_value .env ZARINPAL_MERCHANT_ID "$ZARINPAL_MERCHANT_ID"
  [[ "${ZARINPAL_SANDBOX:-}" == "true" ]] && set_env_value .env ZARINPAL_SANDBOX true
fi
if [[ -n "${SMS_USERNAME:-}" && -n "${SMS_PASSWORD:-}" ]]; then
  set_env_value .env SMS_ENABLED true; set_env_value .env SMS_USERNAME "$SMS_USERNAME"; set_env_value .env SMS_PASSWORD "$SMS_PASSWORD"
fi
[[ -n "${LEGACY_DEVICE_KEY_SECRET:-}" ]] && set_env_value .env LEGACY_DEVICE_KEY_SECRET "$LEGACY_DEVICE_KEY_SECRET"

chmod 600 .env 2>/dev/null || true
say "Created .env with generated secrets (web port ${WEB_PORT}, database port ${DB_HOST_PORT})."
echo "  Admin login:  $(env_get BOOTSTRAP_ADMIN_USER) / ${ADMIN_PASSWORD}   (also stored in .env; change it at first login)"
[[ "${DEV:-0}" == "1" ]] || [[ -n "${DOMAIN:-}" ]] || echo "  Edit APP_DOMAIN / PUBLIC_URL / ZARINPAL_MERCHANT_ID / SMS_* in .env before going live."
