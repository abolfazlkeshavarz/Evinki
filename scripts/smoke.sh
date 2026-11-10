#!/usr/bin/env bash
# Runs the end-to-end check inside the api image against the live stack.
source "$(dirname "$0")/lib.sh"
USERN="$(env_get BOOTSTRAP_ADMIN_USER)"; PASS="${SMOKE_PASSWORD:-$(env_get BOOTSTRAP_ADMIN_PASSWORD)}"
[ -n "$PASS" ] || die "set SMOKE_PASSWORD=<admin password> (BOOTSTRAP_ADMIN_PASSWORD is empty in .env)"
[ "$(env_get PAYMENT_GATEWAY)" = "mock" ] || warn "PAYMENT_GATEWAY is not 'mock': the checkout steps will fail (expected in production)"
$COMPOSE run --rm --no-deps -T api smoke -url http://web -user "${USERN:-admin}" -pass "$PASS"
