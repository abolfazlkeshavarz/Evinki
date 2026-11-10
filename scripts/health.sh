#!/usr/bin/env bash
# Health check for Evinki.
#
#   bash scripts/health.sh                          # the local docker stack (containers, data, public side)
#   API_URL=https://shop.evinki.com REMOTE=1 bash scripts/health.sh   # only the public side of a deployment
#   make deploy-health HOST=https://shop.evinki.com
# Exits non-zero if anything required is down, so it works as a post-deploy gate.
set -uo pipefail
export LIB_NO_ERREXIT=1
source "$(dirname "$0")/lib.sh"
[[ -f .env ]] && load_env .env 2>/dev/null
API_URL="${1:-${API_URL:-$(web_url)}}"
REMOTE="${REMOTE:-}"
GREEN=$'\033[0;32m'; RED=$'\033[0;31m'; YELLOW=$'\033[1;33m'; NC=$'\033[0m'
failures=0; warnings=0
ok()   { echo "  ${GREEN}[OK]${NC}   $1"; }
bad()  { echo "  ${RED}[FAIL]${NC} $1"; failures=$((failures + 1)); }
wrn()  { echo "  ${YELLOW}[WARN]${NC} $1"; warnings=$((warnings + 1)); }
code_of() { local c; c=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "$1" 2>/dev/null) || c=000; echo "${c:-000}"; }

check_public() {
  echo ""; echo "Public side ($API_URL)"
  local code
  code=$(code_of "$API_URL/healthz");            [[ "$code" == "200" ]] && ok "GET /healthz -> 200" || { bad "GET /healthz -> $code"; return; }
  code=$(code_of "$API_URL/api/v1/version");     [[ "$code" == "200" ]] && ok "API answers" || bad "GET /api/v1/version -> $code"
  code=$(code_of "$API_URL/api/v1/shop/categories"); [[ "$code" == "200" ]] && ok "database reachable through the API" || bad "GET /api/v1/shop/categories -> $code"
  code=$(code_of "$API_URL/");                   [[ "$code" == "200" ]] && ok "web app served" || bad "GET / -> $code"
  code=$(code_of "$API_URL/admin/orders");       [[ "$code" == "200" ]] && ok "single-page routes fall back to the app" || wrn "GET /admin/orders -> $code"
  code=$(code_of "$API_URL/api/v1/admin/users"); [[ "$code" == "401" ]] && ok "admin API requires sign-in" || bad "GET /api/v1/admin/users anonymous -> $code (expected 401)"
}

echo ""; echo "Evinki health check"; echo "==================="
if [[ -n "$REMOTE" ]]; then
  check_public
else
  echo ""; echo "Containers"
  if ! command -v "$DOCKER" >/dev/null 2>&1; then wrn "docker not found; skipping container checks"; else
    for s in db api web; do
      cid="$($COMPOSE ps -q "$s" 2>/dev/null | head -1)"
      if [[ -z "$cid" ]]; then bad "$s is not running (try: make up)"; continue; fi
      state="$("$DOCKER" inspect -f '{{.State.Status}}' "$cid" 2>/dev/null)"
      health="$("$DOCKER" inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$cid" 2>/dev/null)"
      if [[ "$state" == "running" && ( "$health" == "healthy" || "$health" == "none" ) ]]; then ok "$s ($state${health:+, $health})"
      elif [[ "$state" == "running" ]]; then wrn "$s running but health=$health"
      else bad "$s is $state"; fi
    done
    echo ""; echo "Data"
    n="$($COMPOSE exec -T db psql -U evinki -d evinki -Atc 'SELECT COUNT(*) FROM schema_migrations' 2>/dev/null | tr -d '\r' || true)"
    [[ -n "$n" ]] && ok "database migrations applied: ${n}" || bad "cannot read the database"
    u="$($COMPOSE exec -T db psql -U evinki -d evinki -Atc 'SELECT COUNT(*) FROM users WHERE deleted_at IS NULL' 2>/dev/null | tr -d '\r' || true)"
    [[ -n "$u" ]] && ok "users: ${u}"
    echo ""; echo "Operations"
    LAST=$(ls -1t backups/evinki-*.tar.gz 2>/dev/null | head -1 || true)
    if [[ -z "$LAST" ]]; then wrn "no backup exists yet: run 'make backup'"
    elif [[ -n "$(find "$LAST" -mtime +2 2>/dev/null)" ]]; then wrn "latest backup is older than 2 days: $LAST"
    else ok "latest backup: $LAST"; fi
    [[ -n "$(env_get BACKUP_REMOTE)" ]] && ok "off-server backups: $(env_get BACKUP_REMOTE)" || wrn "no off-server backup copy: losing this server would lose the backups too (make backup-remote-setup REMOTE=user@host:/path)"
    [[ -n "$(env_get ZARINPAL_MERCHANT_ID)" || "$(env_get PAYMENT_GATEWAY)" == "mock" ]] || wrn "no ZARINPAL_MERCHANT_ID: online payment is off"
    [[ "$(env_get SMS_ENABLED)" == "true" ]] || wrn "SMS is off: customers cannot receive login codes"
  fi
  check_public
  pub="$(env_get PUBLIC_URL)"
  if [[ "$pub" == https://* ]]; then echo ""; echo "Internet"; code=$(code_of "$pub/healthz"); [[ "$code" == "200" ]] && ok "$pub reachable over HTTPS" || wrn "$pub -> $code (DNS/TLS not ready? make ssl)"; fi
fi
echo ""; echo "==================="
if (( failures > 0 )); then echo "${RED}${failures} failed${NC}, ${warnings} warning(s)"; exit 1; fi
echo "${GREEN}All required checks passed${NC}, ${warnings} warning(s)"
