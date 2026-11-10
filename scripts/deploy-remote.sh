#!/usr/bin/env bash
#
# Deploy to your server by building the images HERE (on your PC) and pushing them over SSH.
# The server never builds anything and does not need git.
#
#   make deploy-remote HOST=root@your-server                      # build, upload, backup, switch, health-check
#   make deploy-remote HOST=root@your-server DIR=/opt/evinki      # a different folder on the server
#   make rollback-remote HOST=root@your-server                    # go back to the previous version
#
# Options (environment variables)
#   HOST        required: user@server (SSH key login)
#   DIR         folder on the server (default /opt/evinki)
#   PLATFORM    server CPU architecture: linux/amd64 (default) or linux/arm64
#   SSH_PORT    default 22      SSH_KEY   path to a private key (optional)
#   SKIP_BUILD  1 = reuse the images already built on this PC
#
# First deployment on a brand-new server: uploads everything, installs Docker, then tells you the one command
# to finish (or runs it for you if you pass DOMAIN=... LETSENCRYPT_EMAIL=...).
# On the server, in order: back up (if running) -> keep the old images as "previous" -> load the new ones ->
# restart -> wait until healthy. A failed health check tells you how to roll back.
# Your .env, database, uploads and backups are never touched.
set -euo pipefail
cd "$(dirname "$0")/.."
export MSYS_NO_PATHCONV=1   # Git Bash on Windows must not rewrite /opt/... arguments

HOST="${HOST:-}"; DIR="${DIR:-/opt/evinki}"; PLATFORM="${PLATFORM:-linux/amd64}"
SSH_PORT="${SSH_PORT:-22}"; DOCKER="${DOCKER:-docker}"
ROLLBACK=0; [[ "${1:-}" == "--rollback" ]] && ROLLBACK=1
[[ -n "$HOST" ]] || { echo "HOST is required, e.g.  make deploy-remote HOST=root@203.0.113.10" >&2; exit 1; }
[[ "$HOST" =~ ^[A-Za-z0-9_.-]+@[A-Za-z0-9_.:-]+$ ]] || { echo "HOST must look like user@server" >&2; exit 1; }
[[ "$DIR" =~ ^/[A-Za-z0-9_./-]+$ ]] || { echo "DIR must be an absolute path such as /opt/evinki" >&2; exit 1; }
command -v ssh >/dev/null 2>&1 || { echo "ssh is required on this machine." >&2; exit 1; }

SSH_ARGS=(-p "$SSH_PORT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20)
[[ -n "${SSH_KEY:-}" ]] && SSH_ARGS+=(-i "$SSH_KEY")
remote() { ssh "${SSH_ARGS[@]}" "$HOST" "$@"; }

# The small support files (compose, Makefile, scripts, nginx template) are copied with Unix line endings: a
# Windows checkout has CRLF, and "bash\r: No such file or directory" would break every script.
stage_support_files() {
  local dest="$1"
  mkdir -p "$dest/deploy/nginx"
  cp docker-compose.yml Makefile .env.example "$dest/"
  cp -R scripts "$dest/"
  cp deploy/nginx/app.conf.template "$dest/deploy/nginx/"
  find "$dest" -type f \( -name '*.sh' -o -name 'Makefile' -o -name '*.yml' -o -name '*.template' -o -name '.env.example' \) -exec sed -i 's/\r$//' {} +
  chmod +x "$dest"/scripts/*.sh
}
if [[ -n "${STAGE_ONLY:-}" ]]; then stage_support_files "$STAGE_ONLY"; echo "staged into $STAGE_ONLY"; exit 0; fi

echo "==> Checking the SSH connection to ${HOST}"
remote "echo connected" >/dev/null 2>&1 || { echo "Cannot log in to ${HOST} with a key. Set up key login first:  ssh-copy-id ${HOST}   (or pass SSH_KEY=path)" >&2; exit 1; }

# ------------------------------------------------------------------------------------------ rollback
if [[ "$ROLLBACK" == "1" ]]; then
  echo "==> Rolling back to the previous images on ${HOST}"
  remote "bash -s" -- "$DIR" <<'REMOTE'
set -euo pipefail
cd "$1"
docker image inspect evinki-api:previous >/dev/null 2>&1 && docker image inspect evinki-web:previous >/dev/null 2>&1 || { echo "There is no previous version to go back to." >&2; exit 1; }
docker tag evinki-api:latest evinki-api:rolled-back-from 2>/dev/null || true
docker tag evinki-web:latest evinki-web:rolled-back-from 2>/dev/null || true
docker tag evinki-api:previous evinki-api:latest
docker tag evinki-web:previous evinki-web:latest
docker compose up -d --no-build --force-recreate api web </dev/null
for _ in $(seq 1 90); do
  [ "$(docker compose ps api --format '{{.Health}}' 2>/dev/null | head -1)" = "healthy" ] && { echo "api is healthy on the previous version"; exit 0; }
  sleep 1
done
echo "The api did not become healthy; check: docker compose logs --tail=50 api" >&2; exit 1
REMOTE
  echo "Done. Database changes are forward-only: if the newer version changed data, restore a backup (make restore FILE=backups/...)."
  exit 0
fi

# ------------------------------------------------------------------------------------------ build locally
REV="$(git rev-parse --short HEAD 2>/dev/null || echo manual)"
DIRTY=""; [[ -n "$(git status --porcelain 2>/dev/null)" ]] && DIRTY="-dirty"
TAG="${REV}${DIRTY}"
OUT="dist/deploy"; TAR="evinki-images-${TAG}.tar.gz"
mkdir -p "$OUT"
if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  [[ -z "$DIRTY" ]] || echo "NOTE: you have uncommitted changes; they are included in this build (version tag: ${TAG})."
  [[ -f .env ]] || bash scripts/init-env.sh >/dev/null   # compose needs some .env to read; it is NOT uploaded
  echo "==> Building the images on this machine (${PLATFORM}), version ${TAG}"
  DOCKER_DEFAULT_PLATFORM="$PLATFORM" docker compose build
  for i in api web; do
    got="$("$DOCKER" image inspect "evinki-$i:latest" --format '{{.Architecture}}')"
    [[ "$got" == "${PLATFORM##*/}" ]] || { echo "evinki-$i is ${got}, expected ${PLATFORM##*/}." >&2; exit 1; }
    "$DOCKER" tag "evinki-$i:latest" "evinki-$i:${TAG}"
  done
  echo "==> Packing the images"
  "$DOCKER" save evinki-api:latest evinki-web:latest "evinki-api:${TAG}" "evinki-web:${TAG}" | gzip -1 > "$OUT/$TAR"
fi
[[ -s "$OUT/$TAR" ]] || { echo "No image file at $OUT/$TAR (build first, or unset SKIP_BUILD)." >&2; exit 1; }
echo "    $(du -h "$OUT/$TAR" | cut -f1)  $OUT/$TAR"

# ------------------------------------------------------------------------------------------ upload
echo "==> Uploading to ${HOST}:${DIR}"
remote "mkdir -p '$DIR/incoming'"
STAGE="$(mktemp -d)"; trap 'rm -rf "$STAGE"' EXIT
stage_support_files "$STAGE"   # never .env, database, uploads or backups
tar -czf - -C "$STAGE" . | remote "cd '$DIR' && tar xzf -"
remote "cat > '$DIR/incoming/$TAR.part'" < "$OUT/$TAR"
LOCAL_SUM="$( (sha256sum "$OUT/$TAR" 2>/dev/null || shasum -a 256 "$OUT/$TAR") | cut -d' ' -f1)"
REMOTE_SUM="$(remote "sha256sum '$DIR/incoming/$TAR.part'" | cut -d' ' -f1)"
[[ "$LOCAL_SUM" == "$REMOTE_SUM" ]] || { echo "The uploaded file is damaged (checksum differs). Try again." >&2; exit 1; }
remote "mv '$DIR/incoming/$TAR.part' '$DIR/incoming/$TAR'"
echo "    checksum verified"

# ------------------------------------------------------------------------------------------ switch over
echo "==> Installing on the server"
STATUS="$(remote "bash -s" -- "$DIR" "$TAR" <<'REMOTE'
set -euo pipefail
cd "$1"; TAR="$2"
if ! command -v docker >/dev/null 2>&1; then echo "[server] installing Docker" >&2; bash scripts/install-docker.sh >&2; fi
# every docker command gets </dev/null: this script arrives on stdin and a command reading stdin would swallow the rest of it
if [ -f .env ] && [ -n "$(docker compose ps -q api 2>/dev/null </dev/null | head -1)" ]; then
  echo "[server] backing up before the switch" >&2
  bash scripts/backup.sh >&2 </dev/null || { echo "[server] BACKUP FAILED - not switching" >&2; echo "BACKUP_FAILED"; exit 0; }
fi
PREV=0
if docker image inspect evinki-api:latest >/dev/null 2>&1 && docker image inspect evinki-web:latest >/dev/null 2>&1; then
  docker tag evinki-api:latest evinki-api:previous; docker tag evinki-web:latest evinki-web:previous; PREV=1
fi
echo "[server] loading the images" >&2
gunzip -c "incoming/$TAR" | docker load >&2
rm -f "incoming/$TAR"
{ ls -1t incoming/*.tar.gz 2>/dev/null || true; } | tail -n +3 | xargs -r rm -f --   # keep the 2 newest uploads
if [ ! -f .env ]; then echo "NEEDS_SETUP"; exit 0; fi
docker compose up -d --no-build >&2 </dev/null
for _ in $(seq 1 120); do
  if [ "$(docker compose ps api --format '{{.Health}}' 2>/dev/null | head -1)" = "healthy" ] && [ "$(docker compose ps web --format '{{.Health}}' 2>/dev/null | head -1)" = "healthy" ]; then
    docker image prune -f >/dev/null 2>&1 || true; echo "HEALTHY PREV=$PREV"; exit 0
  fi
  sleep 1
done
echo "UNHEALTHY"
REMOTE
)" || { echo "The step on the server failed (see the messages above). Nothing was left half-done that a re-run cannot fix." >&2; exit 1; }

case "$STATUS" in
  *BACKUP_FAILED*)
    echo "The automatic backup before the switch failed, so nothing was changed (the images were NOT loaded)." >&2
    echo "Check the stack on the server ('docker compose logs --tail=50 api'), then run this again." >&2; exit 1 ;;
  *NEEDS_SETUP*)
    echo ""; echo "The server has no .env yet, so this is a first deployment. The images and scripts are in place."
    if [[ -n "${DOMAIN:-}" && -n "${LETSENCRYPT_EMAIL:-}" ]]; then
      echo "==> Finishing the setup (secrets, certificate, nginx, backups)"
      ssh -t "${SSH_ARGS[@]}" "$HOST" "cd '$DIR' && DOMAIN='$DOMAIN' LETSENCRYPT_EMAIL='$LETSENCRYPT_EMAIL' ZARINPAL_MERCHANT_ID='${ZARINPAL_MERCHANT_ID:-}' ./scripts/bootstrap-vps.sh"
    else
      echo "Finish it with ONE command on the server:"
      echo "    ssh ${HOST} \"cd ${DIR} && DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com ./scripts/bootstrap-vps.sh\""
      echo "(or re-run this command with DOMAIN=... LETSENCRYPT_EMAIL=... and it will do that for you)"
    fi
    exit 0 ;;
  *UNHEALTHY*)
    echo "The new version did not become healthy. Look:   ssh ${HOST} 'cd ${DIR} && docker compose logs --tail=60 api'" >&2
    echo "Go back to the previous version with:           make rollback-remote HOST=${HOST}" >&2; exit 1 ;;
  *HEALTHY*) ;;
  *) echo "Unexpected answer from the server: ${STATUS}" >&2; exit 1 ;;
esac
echo ""
echo "================================================================"
echo " Deployed ${TAG} to ${HOST}:${DIR}"
echo "================================================================"
case "$STATUS" in *PREV=1*) echo "  Previous version kept  ->  make rollback-remote HOST=${HOST}" ;; esac
echo "  Check the public site:  make deploy-health HOST=https://shop.evinki.com"
