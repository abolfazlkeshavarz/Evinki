#!/usr/bin/env bash
#
# Shared helpers sourced by the other scripts. Not meant to be run directly.
if [[ "${LIB_NO_ERREXIT:-0}" == "1" ]]; then set -uo pipefail; else set -euo pipefail; fi
cd "$(dirname "${BASH_SOURCE[0]}")/.."
ROOT="$(pwd)"
COMPOSE="${COMPOSE:-docker compose}"
DOCKER="${DOCKER:-docker}"

say()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }

# env_get KEY - value of KEY in .env ("" when missing). Never executes the file.
env_get() { grep -E "^$1=" .env 2>/dev/null | head -1 | cut -d= -f2- | tr -d '\r' || true; }

# load_env <file> - exports KEY=VALUE lines WITHOUT `source` (which would run every line as a command).
load_env() {
  local file="${1:-.env}" line
  [[ -f "$file" ]] || { echo "Error: $file not found." >&2; return 1; }
  set -a
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    if [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      line="$(sed -E 's/[[:space:]]+#.*$//' <<<"$line")"
      export "$line"
    else
      echo "Warning: ignoring malformed line in $file: $line" >&2
    fi
  done < "$file"
  set +a
}

# set_env_value <file> <KEY> <value> - replace KEY=... (or append it)
set_env_value() {
  local file="$1" key="$2" value="$3"
  if grep -q "^${key}=" "$file"; then
    sed -i "s|^${key}=.*|${key}=${value//|/\\|}|" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$file"
  fi
}

# rand_secret <bytes> - URL/shell-safe random string
rand_secret() { openssl rand -base64 "${1:-32}" | tr -d '\n=+/'; }

# port_in_use <port> - true if anything on this host listens on it
port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then ss -Htln "( sport = :$port )" 2>/dev/null | grep -q .; return $?; fi
  (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null && { exec 3>&-; return 0; }
  return 1
}
# find_free_port <start> - first free port at or above <start>
find_free_port() { local p="${1:-8090}"; while port_in_use "$p"; do p=$((p + 1)); done; echo "$p"; }

valid_hostname() { [[ "$1" =~ ^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$ ]]; }
clean_domain() { tr 'A-Z' 'a-z' <<<"$1" | sed -E 's#^https?://##; s#/.*$##'; }

# need_root "$0" "$@" - re-run the script with sudo when not root
need_root() { [[ "$(id -u)" == "0" ]] || { echo "==> Root access is required; re-running with sudo"; exec sudo -E bash "$@"; }; }

# apply_domain_env <file> <domain> - point .env at a (new) primary domain
apply_domain_env() {
  set_env_value "$1" APP_DOMAIN "$2"
  set_env_value "$1" PUBLIC_URL "https://$2"
}

# add_extra_domain / remove_extra_domain - bookkeeping of alias domains (they redirect to the primary one)
extra_domains() { env_get EXTRA_DOMAINS | tr ',' '\n' | awk 'NF'; }
add_extra_domain() {
  local list; list="$( { extra_domains; echo "$1"; } | awk 'NF && !s[$0]++' | paste -sd, - )"
  set_env_value .env EXTRA_DOMAINS "$list"
}
remove_extra_domain() {
  local list; list="$(extra_domains | grep -vx "$1" | paste -sd, - || true)"
  set_env_value .env EXTRA_DOMAINS "$list"
}

# web_url - where the web container answers on this host
web_url() {
  local bind port; bind="$(env_get WEB_BIND)"; port="$(env_get WEB_PORT)"
  [[ "$bind" == "0.0.0.0" || -z "$bind" ]] && bind=127.0.0.1
  echo "http://${bind}:${port:-8090}"
}

# images_present - both application images exist locally
images_present() {
  "$DOCKER" image inspect evinki-api:latest >/dev/null 2>&1 && "$DOCKER" image inspect evinki-web:latest >/dev/null 2>&1
}

# wait_healthy <service> [seconds]
wait_healthy() {
  local svc="$1" n="${2:-120}" i
  for i in $(seq 1 "$n"); do
    [[ "$($COMPOSE ps "$svc" --format '{{.Health}}' 2>/dev/null | head -1)" == "healthy" ]] && return 0
    sleep 1
  done
  return 1
}
