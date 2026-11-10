#!/usr/bin/env bash
# Checks a server before the first deployment.
source "$(dirname "$0")/lib.sh"
ok=1
chk() { if eval "$2" >/dev/null 2>&1; then say "$1"; else warn "MISSING: $1"; ok=0; fi; }
chk "docker installed" "command -v docker"
chk "docker compose plugin" "docker compose version"
chk ".env present" "test -f .env"
if [ -f .env ]; then
  chk "JWT_SECRET is long and random" "[ \$(env_get JWT_SECRET | wc -c) -gt 40 ] && ! env_get JWT_SECRET | grep -qi change-me"
  chk "DB_PASSWORD changed" "! env_get DB_PASSWORD | grep -qi change-me"
  chk "PUBLIC_URL is https" "env_get PUBLIC_URL | grep -q '^https://'"
  chk "payment gateway is not mock" "[ \"\$(env_get PAYMENT_GATEWAY)\" != mock ]"
  chk "zarinpal merchant id set" "[ -n \"\$(env_get ZARINPAL_MERCHANT_ID)\" ]"
fi
chk "at least 2 GB free disk" "[ \$(df -Pk . | awk 'NR==2{print \$4}') -gt 2000000 ]"
if [ $ok = 1 ]; then say "ready to deploy"; else die "fix the items above first"; fi
