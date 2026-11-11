#!/usr/bin/env bash
#
# Loads the bundle made by scripts/build-images.sh. Run this ON THE SERVER.
#   make load-images                       (looks in ./ and ./dist)
#   make load-images FILE=path/to/bundle.tar.gz
source "$(dirname "$0")/lib.sh"
BUNDLE="${1:-}"
if [[ -z "$BUNDLE" ]]; then
  for c in evinki-images.tar.gz dist/evinki-images.tar.gz; do [[ -f "$c" ]] && { BUNDLE="$c"; break; }; done
fi
[[ -n "$BUNDLE" && -f "$BUNDLE" ]] || die "image bundle not found. Build it on your own machine (make images-bundle) and scp it here."

say "Loading images from ${BUNDLE}"
LOADED="$(gunzip -c "$BUNDLE" | "$DOCKER" load 2>&1)" || { echo "$LOADED" >&2; die "docker could not read ${BUNDLE} - is it the file made by 'make images-bundle'?"; }
echo "$LOADED"
# a garbage file must never count as an update just because older images with the same names exist
for img in evinki-api evinki-web; do
  grep -q "Loaded image: ${img}:" <<<"$LOADED" || die "the bundle does not contain ${img}. Build it with: make images-bundle"
done

host_arch="$("$DOCKER" info --format '{{.Architecture}}' 2>/dev/null || uname -m)"
case "$host_arch" in x86_64|amd64) host_arch=amd64 ;; aarch64|arm64) host_arch=arm64 ;; esac
for img in evinki-api:latest evinki-web:latest; do
  got="$("$DOCKER" image inspect "$img" --format '{{.Architecture}}' 2>/dev/null || echo missing)"
  [[ "$got" != "missing" ]] || die "${img} is not in the bundle."
  [[ "$got" == "$host_arch" ]] || die "${img} is ${got} but this server is ${host_arch}; rebuild with PLATFORM=linux/${host_arch}."
  echo "    ${img}: ${got} - matches this server"
done
echo ""
echo "Images loaded. Start without building:  docker compose up -d --no-build   (or: make up-prebuilt)"
