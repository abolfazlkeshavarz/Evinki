#!/usr/bin/env bash
#
# Installs the update service on THIS server (run once, as root, in the Evinki folder):
#   sudo ./scripts/install-updater.sh        (or: sudo make install-updater)
#
# After that, administrators can upload an image bundle (made on a development computer with `make images-bundle`)
# in the admin panel under  سامانه > به‌روزرسانی  and the system updates itself, with a backup first and an automatic
# rollback if the new version does not start.
source "$(dirname "$0")/lib.sh"
[[ "$(id -u)" == "0" ]] || die "run as root:  sudo ./scripts/install-updater.sh"
command -v systemctl >/dev/null || die "systemd is required (or run 'bash scripts/updater.sh --daemon' under your own supervisor)"
chmod +x scripts/updater.sh

UNIT=/etc/systemd/system/evinki-updater.service
cat > "$UNIT" <<EOF
[Unit]
Description=Evinki platform updater (applies image bundles uploaded in the admin panel)
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=simple
WorkingDirectory=${ROOT}
ExecStart=/bin/bash ${ROOT}/scripts/updater.sh --daemon
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now evinki-updater.service
sleep 3
systemctl --no-pager --lines=5 status evinki-updater.service || true
echo ""
echo "Updater installed. Check it:  systemctl status evinki-updater   |   logs:  journalctl -u evinki-updater -f"
echo "If the host nginx was set up before this version, allow big uploads once:  make ssl   (re-renders the vhost)"
