#!/usr/bin/env bash
# Pre-flight checks to run BEFORE deploying.   make deploy-check
# Catches the mistakes that are cheap to find now and expensive in production: committed secrets,
# weak or default values, ports already in use, wrong line endings, failing checks.
set -uo pipefail
export LIB_NO_ERREXIT=1
source "$(dirname "$0")/lib.sh"
GREEN=$'\033[0;32m'; RED=$'\033[0;31m'; YELLOW=$'\033[1;33m'; NC=$'\033[0m'
errors=0; warnings=0
ok()   { echo "  ${GREEN}[OK]${NC}    $1"; }
bad()  { echo "  ${RED}[BLOCK]${NC} $1"; errors=$((errors + 1)); }
wrn()  { echo "  ${YELLOW}[WARN]${NC}  $1"; warnings=$((warnings + 1)); }

echo ""; echo "Evinki deploy pre-flight"; echo "========================"
echo ""; echo "1. Secrets and settings"
if git ls-files --error-unmatch .env >/dev/null 2>&1; then bad ".env is tracked by git - it holds credentials (git rm --cached .env)"; else ok ".env is not tracked by git"; fi
if [[ ! -f .env ]]; then
  wrn ".env not found - create it with: make env   (DOMAIN=... LETSENCRYPT_EMAIL=... make env)"
else
  load_env .env 2>/dev/null
  [[ ${#JWT_SECRET} -ge 40 && "$JWT_SECRET" != change-me* ]] && ok "JWT_SECRET is ${#JWT_SECRET} chars" || bad "JWT_SECRET must be a random value of 40+ characters (make env generates one)"
  [[ -n "${DB_PASSWORD:-}" && "$DB_PASSWORD" != change-me* ]] && ok "DB_PASSWORD is set" || bad "DB_PASSWORD is empty or still the example value"
  if [[ "${APP_ENV:-production}" == "production" ]]; then
    [[ "${PUBLIC_URL:-}" == https://* ]] && ok "PUBLIC_URL=${PUBLIC_URL}" || bad "PUBLIC_URL must be https://your.domain in production"
    [[ "${PAYMENT_GATEWAY:-}" != "mock" ]] && ok "payment gateway: ${PAYMENT_GATEWAY:-zarinpal}" || bad "PAYMENT_GATEWAY=mock is not allowed in production"
    [[ -n "${ZARINPAL_MERCHANT_ID:-}" ]] && ok "Zarinpal merchant id set" || wrn "ZARINPAL_MERCHANT_ID is empty: online payment will not work"
    [[ "${SMS_ENABLED:-false}" == "true" && -n "${SMS_USERNAME:-}" ]] && ok "SMS enabled" || wrn "SMS is off: customers cannot sign in with a phone code in production"
    [[ -n "${LETSENCRYPT_EMAIL:-}" ]] && ok "LETSENCRYPT_EMAIL set" || wrn "LETSENCRYPT_EMAIL is empty (needed by make ssl)"
    [[ -n "${APP_DOMAIN:-}" && "${APP_DOMAIN}" != "localhost" && "${APP_DOMAIN}" != *example.com ]] && ok "APP_DOMAIN=${APP_DOMAIN}" || bad "APP_DOMAIN is not set to your real domain"
  else
    wrn "APP_ENV=${APP_ENV} (development): fine for testing, not for the public"
  fi
  if [[ -z "$($COMPOSE ps -q web 2>/dev/null)" ]]; then
    port_in_use "${WEB_PORT:-8090}" && bad "WEB_PORT ${WEB_PORT:-8090} is already used by another program (pick one: WEB_PORT=$(find_free_port 8091) make up)" || ok "local web port ${WEB_PORT:-8090} is free"
    port_in_use "${DB_HOST_PORT:-15432}" && bad "DB_HOST_PORT ${DB_HOST_PORT:-15432} is already used (set DB_HOST_PORT=$(find_free_port 15433) in .env)" || ok "local database port ${DB_HOST_PORT:-15432} is free"
  fi
  [[ -n "${BACKUP_REMOTE:-}" ]] && ok "off-server backups: ${BACKUP_REMOTE}" || wrn "no off-server backup copy (make backup-remote-setup REMOTE=user@host:/path)"
fi

echo ""; echo "2. Server"
command -v docker >/dev/null 2>&1 && ok "docker installed" || bad "docker is not installed (scripts/install-docker.sh)"
docker compose version >/dev/null 2>&1 && ok "docker compose plugin" || bad "docker compose plugin missing"
[[ $(df -Pk . | awk 'NR==2{print $4}') -gt 2000000 ]] && ok "at least 2 GB free disk" || bad "less than 2 GB free disk"
if command -v docker >/dev/null 2>&1 && [[ -f .env ]]; then $COMPOSE config -q >/dev/null 2>&1 && ok "docker-compose.yml is valid" || bad "docker compose config reported a problem (is .env complete?)"; fi
if [[ -n "${APP_DOMAIN:-}" && "${APP_DOMAIN}" != "localhost" ]]; then
  dip="$(getent hosts "$APP_DOMAIN" | awk '{print $1}' | head -1 || true)"; sip="$(curl -fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)"
  if [[ -z "$dip" ]]; then wrn "${APP_DOMAIN} has no DNS record yet (needed for make ssl)"; elif [[ -n "$sip" && "$dip" != "$sip" ]]; then wrn "${APP_DOMAIN} points to ${dip}, this server is ${sip}"; else ok "${APP_DOMAIN} points to this server"; fi
fi

echo ""; echo "3. Code"
for f in scripts/*.sh; do bash -n "$f" 2>/dev/null || bad "syntax error in $f"; done; ok "shell scripts parse"
if git rev-parse --git-dir >/dev/null 2>&1; then
  crlf="$(git ls-files --eol scripts frontend/nginx.conf docker-compose.yml 2>/dev/null | awk '$1=="i/crlf"{print $NF}' | tr '\n' ' ')"
  [[ -z "${crlf// }" ]] && ok "committed with Unix line endings" || bad "files committed with Windows line endings (breaks on Linux): ${crlf}"
  tracked_bad="$(git ls-files 2>/dev/null | grep -E '(^|/)(\.env|node_modules/|dist/|uploads/|backups/)|\.log$' | grep -v '\.env\.example' | head -5)"
  [[ -z "$tracked_bad" ]] && ok "no env files, build output or uploads are tracked" || { bad "files that should never be committed are tracked:"; sed 's/^/           /' <<<"$tracked_bad"; }
  n="$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')"; [[ "$n" == "0" ]] || wrn "$n uncommitted change(s) - the server uses what you push"
fi
if command -v go >/dev/null 2>&1 && [[ -d backend ]]; then (cd backend && go vet ./... >/dev/null 2>&1) && ok "go vet passes" || bad "go vet fails"; (cd backend && go test ./... >/dev/null 2>&1) && ok "go tests pass" || bad "go tests fail"; fi
if command -v npm >/dev/null 2>&1 && [[ -d frontend/node_modules ]]; then (cd frontend && npx tsc --noEmit >/dev/null 2>&1) && ok "typescript check passes" || bad "typescript check fails"; fi

echo ""; echo "========================"
if (( errors > 0 )); then echo "${RED}${errors} blocker(s)${NC}, ${warnings} warning(s) - do not deploy yet"; exit 1; fi
echo "${GREEN}No blockers${NC}, ${warnings} warning(s)"
echo ""
echo "Next, on the server:  DOMAIN=... LETSENCRYPT_EMAIL=... ./scripts/bootstrap-vps.sh"
echo "Small VPS:            make images-bundle -> scp -> make load-images -> ./scripts/bootstrap-vps.sh"
echo "From your PC:         make deploy-remote HOST=root@SERVER"
