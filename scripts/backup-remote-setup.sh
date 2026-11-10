#!/usr/bin/env bash
#
# Copies every backup to a SECOND server too, so losing this server loses nothing and a new server can be
# restored from the off-server copy.
#
#   make backup-remote-setup REMOTE=backupuser@backup.example.com:/backups/evinki
#   (PORT_SSH=2222 for a non-standard ssh port; YES=1 to skip the pause)
#
# The remote side only needs SSH and a folder. This script
#   1. stores REMOTE in .env
#   2. creates an SSH key on THIS server (~/.ssh/evinki_backup) and prints its public half
#   3. waits while you add that key to the backup server, then tests the connection and sends a first copy
# Restore later with:  ./scripts/migrate-import.sh <the copied evinki-....tar.gz>   (or make restore FILE=...)
source "$(dirname "$0")/lib.sh"
[[ -f .env ]] || die ".env not found - run this in the installation folder."
REMOTE="${1:-$(env_get BACKUP_REMOTE)}"
[[ "$REMOTE" =~ ^[A-Za-z0-9_.-]+@[A-Za-z0-9_.-]+:/[A-Za-z0-9_./-]*$ ]] || die "Usage: backup-remote-setup.sh user@host:/absolute/folder   (got: '${REMOTE}')"
set_env_value .env BACKUP_REMOTE "$REMOTE"
[[ -n "${PORT_SSH:-}" ]] && set_env_value .env BACKUP_REMOTE_PORT "$PORT_SSH"
PORT="$(env_get BACKUP_REMOTE_PORT)"; PORT="${PORT:-22}"
say "Saved BACKUP_REMOTE=$REMOTE"

KEY="$HOME/.ssh/evinki_backup"
mkdir -p "$HOME/.ssh"; chmod 700 "$HOME/.ssh"
[[ -f "$KEY" ]] || ssh-keygen -q -t ed25519 -N "" -C "evinki-backup@$(hostname)" -f "$KEY"
pub="$(cat "$KEY.pub")"
host="${REMOTE%%:*}"
echo ""
echo "On the BACKUP server, add this line to  ~/.ssh/authorized_keys  of ${host%%@*}:"
echo ""
echo "  $pub"
echo ""
echo "  (one command on the backup server:  echo '$pub' >> ~/.ssh/authorized_keys )"
echo ""
[[ "${YES:-0}" == "1" ]] || read -r -p "Press Enter once the key is added to test the connection... " _

say "Testing the connection"
dir="${REMOTE#*:}"
ssh -i "$KEY" -p "$PORT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=20 "$host" "mkdir -p '$dir' && echo ok" \
  || die "The connection failed. Check the key, the user and the folder, then run this again."
say "Sending the newest backup as a first copy"
newest="$(ls -1t backups/evinki-*.tar.gz 2>/dev/null | head -1 || true)"
if [[ -n "$newest" ]]; then scp -q -i "$KEY" -P "$PORT" "$newest" "${REMOTE%/}/" && echo "    sent $newest"; else warn "no local backup yet - the next one (make backup) will be copied"; fi
echo ""
echo "Done. Every backup from now on is copied to ${REMOTE} automatically."
