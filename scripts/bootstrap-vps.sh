#!/usr/bin/env bash
# One command on a fresh Ubuntu/Debian server:
#   DOMAIN=shop.example.com LETSENCRYPT_EMAIL=you@example.com ./scripts/bootstrap-vps.sh
# Installs Docker, creates .env, builds and starts the stack, puts nginx + a Let's Encrypt
# certificate in front, and prints the first administrator login.
#   LAN_ONLY=1  skip the domain/TLS (note: phone cameras only work over HTTPS)
#   ZARINPAL_MERCHANT_ID=...  set the payment gateway now (otherwise edit .env later)
source "$(dirname "$0")/lib.sh"
[ "$(id -u)" = 0 ] || die "run as root (sudo)"

if ! command -v docker >/dev/null; then
  say "installing Docker"
  curl -fsSL https://get.docker.com | sh
fi
need curl

./scripts/gen-secrets.sh
if [ -z "${LAN_ONLY:-}" ]; then
  : "${DOMAIN:?set DOMAIN=your.domain}" "${LETSENCRYPT_EMAIL:?set LETSENCRYPT_EMAIL=you@example.com}"
  sed -i "s|^PUBLIC_URL=.*|PUBLIC_URL=https://${DOMAIN}|" .env
else
  IP="$(hostname -I | awk '{print $1}')"
  sed -i "s|^PUBLIC_URL=.*|PUBLIC_URL=http://${IP}|; s|^HTTP_PORT=.*|HTTP_PORT=0.0.0.0:80|" .env
fi
if [ -n "${ZARINPAL_MERCHANT_ID:-}" ]; then
  sed -i "s|^ZARINPAL_MERCHANT_ID=.*|ZARINPAL_MERCHANT_ID=${ZARINPAL_MERCHANT_ID}|" .env
else
  warn "no ZARINPAL_MERCHANT_ID: online payment will not work until you set it in .env and run 'make up'"
fi

say "building and starting"
$COMPOSE up -d --build
./scripts/health.sh

if [ -z "${LAN_ONLY:-}" ]; then
  say "setting up nginx and TLS for ${DOMAIN}"
  apt-get update -y && apt-get install -y nginx certbot python3-certbot-nginx
  PORT="$(env_get HTTP_PORT)"; PORT="${PORT:-127.0.0.1:8090}"
  sed "s|__DOMAIN__|${DOMAIN}|g; s|__UPSTREAM__|${PORT}|g" deploy/nginx/host-vhost.conf.template > "/etc/nginx/sites-available/evinki"
  ln -sf /etc/nginx/sites-available/evinki /etc/nginx/sites-enabled/evinki
  nginx -t && systemctl reload nginx
  certbot --nginx -d "${DOMAIN}" -m "${LETSENCRYPT_EMAIL}" --agree-tos --no-eff-email --redirect -n
  # nightly backup at 02:30
  ( crontab -l 2>/dev/null | grep -v 'evinki-backup'; echo "30 2 * * * cd ${ROOT} && ./scripts/backup.sh >> backups/backup.log 2>&1 # evinki-backup" ) | crontab -
fi

say "done."
echo
echo "  Address : $(env_get PUBLIC_URL)"
echo "  Admin   : $(env_get BOOTSTRAP_ADMIN_USER)  /  $(env_get BOOTSTRAP_ADMIN_PASSWORD)   (you must change it at first login)"
echo "  Then    : make seed-demo   (optional sample catalogue),  see docs/MIGRATION.md to import the old systems"
