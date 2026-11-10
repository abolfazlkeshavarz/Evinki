#!/usr/bin/env bash
#
# Builds the images HERE, on your own machine, and packs them into one tarball to carry to the server -
# so a small VPS (1 core / 1 GB) never has to run npm/go builds, which is slow at best and OOM-killed at worst.
#
#   make images-bundle
#
# Options (environment variables)
#   PLATFORM=linux/arm64     target architecture if the server is not x86-64 (default linux/amd64)
#   VERSION=1.2.0            extra tag besides latest
#   OUT=path/to/file.tar.gz  where to write the bundle (default dist/evinki-images.tar.gz)
source "$(dirname "$0")/lib.sh"
PLATFORM="${PLATFORM:-linux/amd64}"
VERSION="${VERSION:-}"
OUT="${OUT:-dist/evinki-images.tar.gz}"
[[ -f .env ]] || bash scripts/init-env.sh >/dev/null || true

say "Building the images for ${PLATFORM}"
DOCKER_DEFAULT_PLATFORM="$PLATFORM" $COMPOSE build
IMAGES=(evinki-api:latest evinki-web:latest)
if [[ -n "$VERSION" ]]; then
  for i in api web; do "$DOCKER" tag "evinki-$i:latest" "evinki-$i:$VERSION"; IMAGES+=("evinki-$i:$VERSION"); done
fi

say "Verifying the images really are ${PLATFORM}"
want="${PLATFORM##*/}"   # a wrong architecture loads fine and then dies with "exec format error"
for img in evinki-api:latest evinki-web:latest; do
  got="$("$DOCKER" image inspect "$img" --format '{{.Architecture}}')"
  [[ "$got" == "$want" ]] || die "${img} is ${got}, expected ${want}. Check your Docker buildx setup."
  echo "    ${img}: ${got}"
done

say "Packing into ${OUT}"
mkdir -p "$(dirname "$OUT")"
"$DOCKER" save "${IMAGES[@]}" | gzip -1 > "$OUT"
echo ""
echo "Built $(du -h "$OUT" | cut -f1) -> ${OUT}"
echo "Next:"
echo "  scp ${OUT} USER@SERVER:/opt/evinki/dist/"
echo "  ssh USER@SERVER 'cd /opt/evinki && make load-images && docker compose up -d --no-build'"
echo "(or from here in one step, with rollback:  make deploy-remote HOST=root@SERVER)"
