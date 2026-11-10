#!/usr/bin/env bash
# Shared helpers for the operator scripts.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
ROOT="$(pwd)"
COMPOSE="${COMPOSE:-docker compose}"
say()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }
env_get() { grep -E "^$1=" .env 2>/dev/null | head -1 | cut -d= -f2- || true; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"; }
rand() { head -c "$1" /dev/urandom | base64 | tr -d '/+=\n' | head -c "$2"; }
