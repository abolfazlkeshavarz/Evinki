#!/usr/bin/env bash
#
# Run on your DEVELOPMENT computer: builds the images (or uses a bundle you already have) and uploads it to the
# running server through the same endpoint as the admin screen. The server's updater then backs up, loads the
# images, restarts and rolls back automatically if something is wrong.
#
#   make push-update URL=https://shop.evinki.com ADMIN=admin            (asks for the password)
#   make push-update URL=... ADMIN=... PASS=... FILE=dist/evinki-images.tar.gz   (skip the build)
#
# Options: PLATFORM=linux/arm64 for ARM servers.
source "$(dirname "$0")/lib.sh"
need curl
URL="${URL:-}"
URL="${URL%/}"
[[ -n "$URL" ]] || die "give the address of the server:  make push-update URL=https://shop.evinki.com ADMIN=admin"
USERN="${ADMIN_USER:-admin}"
PASS="${ADMIN_PASS:-}"
if [[ -z "$PASS" ]]; then read -r -s -p "Password of ${USERN} on ${URL}: " PASS; echo; fi

FILE="${FILE:-}"
if [[ -z "$FILE" ]]; then bash scripts/build-images.sh; FILE="dist/evinki-images.tar.gz"; fi
[[ -f "$FILE" ]] || die "bundle not found: $FILE"

say "Signing in to $URL"
LOGIN="$(curl -fsS -X POST "$URL/api/v1/auth/login" -H 'Content-Type: application/json' --data "{\"username\":\"${USERN}\",\"password\":\"${PASS//\"/\\\"}\"}")" || die "sign-in failed"
TOKEN="$(grep -o '"access_token":"[^"]*"' <<<"$LOGIN" | head -1 | cut -d'"' -f4)"
[[ -n "$TOKEN" ]] || die "sign-in failed (no token)"

SUM="$(sha256sum "$FILE" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "$FILE" | cut -d' ' -f1)"
say "Uploading $(basename "$FILE") ($(du -h "$FILE" | cut -f1)) ..."
curl -fS --progress-bar -X PUT "$URL/api/v1/admin/system/update" -H "Authorization: Bearer $TOKEN" \
  -H "X-Filename: $(basename "$FILE")" -H "X-Sha256: $SUM" -H 'Content-Type: application/octet-stream' --data-binary "@$FILE" -o /dev/null \
  || die "upload failed (is the update service installed on the server? sudo make install-updater)"

say "Uploaded. Following the update ..."
for _ in $(seq 1 120); do
  sleep 5
  S="$(curl -fsS "$URL/api/v1/admin/system/update" -H "Authorization: Bearer $TOKEN" 2>/dev/null || true)"
  [[ -n "$S" ]] || { echo "   (the application is restarting ...)"; continue; }
  STATE="$(grep -o '"state":"[^"]*"' <<<"$S" | head -1 | cut -d'"' -f4)"
  echo "   state: ${STATE:-?}"
  [[ "$STATE" == "done" ]] && { say "Updated."; exit 0; }
  [[ "$STATE" == "failed" ]] && die "the update did not go through (the previous version keeps running) - see the log in the admin panel: System > Update"
done
die "timed out waiting; check the admin panel (System > Update)"
