#!/usr/bin/env bash
#
# Host-side updater. It applies an image bundle that an administrator uploaded in the admin panel
# (System -> Update) - the API container itself never touches Docker.
#
#   scripts/updater.sh --daemon   run forever (installed as a systemd service by `make install-updater`)
#   scripts/updater.sh --once     apply a pending request now, then exit
#
# For every request it: backs up -> loads the images -> restarts the stack -> checks health ->
# (on failure) puts the previous images back. Progress is written to status.json / last.log inside the
# "updates" Docker volume, which the admin screen shows.
set -uo pipefail
export LIB_NO_ERREXIT=1
source "$(dirname "$0")/lib.sh"

MODE="${1:---daemon}"
PROJECT="$(basename "$ROOT" | tr 'A-Z' 'a-z' | tr -cd 'a-z0-9_-')"
VOLUME="${UPDATES_VOLUME:-evinki_updates}"
DIR="${UPDATES_DIR:-$($DOCKER volume inspect "$VOLUME" --format '{{.Mountpoint}}' 2>/dev/null || true)}"   # UPDATES_DIR: a folder instead of the volume (tests)
[[ -n "$DIR" && -d "$DIR" ]] || die "Docker volume '$VOLUME' not found. Start the stack first (make up), then run this."
LOG="$DIR/last.log"
now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

json_get() { grep -o "\"$2\":\"[^\"]*\"" "$1" 2>/dev/null | head -1 | cut -d'"' -f4; }
status() { # id state message
  printf '{"id":"%s","state":"%s","message":"%s","at":"%s"}\n' "$1" "$2" "$3" "$(now)" > "$DIR/status.json.tmp" && mv "$DIR/status.json.tmp" "$DIR/status.json"
  echo "[$(now)] $2: $3" >> "$LOG"
}
heartbeat() {
  printf '{"seen_at":"%s","host":"%s","arch":"%s","dir":"%s"}\n' "$(now)" "$(hostname)" "$(uname -m)" "$ROOT" > "$DIR/updater.json.tmp" && mv "$DIR/updater.json.tmp" "$DIR/updater.json"
}
history() { # id ok message file
  printf '{"id":"%s","ok":%s,"message":"%s","file":"%s","finished_at":"%s"}\n' "$1" "$2" "$3" "$4" "$(now)" >> "$DIR/history.jsonl"
}

wait_healthy_stack() {
  local i
  for i in 1 2 3 4 5 6 7 8; do
    sleep 8
    if bash scripts/health.sh >>"$LOG" 2>&1; then return 0; fi
  done
  return 1
}

rollback() {
  local id="$1" reason="$2"
  status "$id" rolling_back "$reason - restoring the previous version"
  for i in api web; do $DOCKER image inspect "evinki-$i:previous" >/dev/null 2>&1 && $DOCKER tag "evinki-$i:previous" "evinki-$i:latest"; done
  $COMPOSE up -d --no-build >>"$LOG" 2>&1
  if wait_healthy_stack; then
    status "$id" failed "$reason. The previous version was restored and is running."
    history "$id" false "$reason (rolled back)" "$FILE"
  else
    status "$id" failed "$reason. ROLLBACK ALSO FAILED - check: docker compose logs api"
    history "$id" false "$reason (rollback failed)" "$FILE"
  fi
}

apply() {
  local id file
  id="$(json_get "$DIR/request.json" id)"
  file="$(json_get "$DIR/request.json" file)"
  FILE="$file"
  : > "$LOG"
  [[ -n "$id" && -f "$DIR/$file" ]] || { status "${id:-unknown}" failed "the uploaded file is missing"; return; }

  status "$id" backing_up "taking a backup of the database and uploads"
  if ! bash scripts/backup.sh >>"$LOG" 2>&1; then
    status "$id" failed "the backup failed, so nothing was changed (see the log)"; history "$id" false "backup failed" "$file"; return
  fi

  status "$id" loading "loading the images (this can take a minute)"
  for i in api web; do $DOCKER image inspect "evinki-$i:latest" >/dev/null 2>&1 && $DOCKER tag "evinki-$i:latest" "evinki-$i:previous"; done
  if ! bash scripts/load-images.sh "$DIR/$file" >>"$LOG" 2>&1; then
    # nothing was started yet, so the running system is untouched: no restart needed
    for i in api web; do $DOCKER image inspect "evinki-$i:previous" >/dev/null 2>&1 && $DOCKER tag "evinki-$i:previous" "evinki-$i:latest"; done
    status "$id" failed "the bundle could not be loaded (wrong file, or built for another CPU architecture). The running system was not changed."
    history "$id" false "bundle could not be loaded" "$file"; rm -f "$DIR/$file"; return
  fi

  status "$id" restarting "restarting the application"
  if ! $COMPOSE up -d --no-build >>"$LOG" 2>&1; then rollback "$id" "restart failed"; return; fi

  status "$id" verifying "checking that the new version is healthy"
  if ! wait_healthy_stack; then rollback "$id" "the new version did not become healthy"; return; fi

  status "$id" done "updated successfully"
  history "$id" true "updated successfully" "$file"
  # keep disk usage low: the bundle is no longer needed; old images are pruned
  rm -f "$DIR/$file"
  $DOCKER image prune -f >>"$LOG" 2>&1 || true
}

pending() {
  [[ -f "$DIR/request.json" ]] || return 1
  local rid sid state
  rid="$(json_get "$DIR/request.json" id)"
  sid="$(json_get "$DIR/status.json" id)"
  state="$(json_get "$DIR/status.json" state)"
  [[ "$rid" != "$sid" ]] && return 0       # a request nobody has started
  [[ "$state" == "queued" ]]               # the screen wrote "queued"
}

# a previous run was killed half way (power loss): do not leave the screen on "loading" forever
recover() {
  local sid state
  sid="$(json_get "$DIR/status.json" id)"; state="$(json_get "$DIR/status.json" state)"
  case "$state" in backing_up|loading|restarting|verifying|rolling_back)
    status "$sid" failed "the update was interrupted (server restarted). Check the system and try again." ;;
  esac
}

if command -v flock >/dev/null 2>&1; then
  exec 9>"/tmp/evinki-updater-$PROJECT.lock"
  flock -n 9 || die "another updater is already running"
fi
recover
if [[ "$MODE" == "--once" ]]; then
  heartbeat; pending && apply; exit 0
fi
say "Evinki updater watching $DIR"
while true; do
  heartbeat
  if pending; then apply; fi
  sleep 5
done
