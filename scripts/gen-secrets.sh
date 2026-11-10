#!/usr/bin/env bash
# Creates .env from .env.example with random secrets. Never overwrites an existing .env.
source "$(dirname "$0")/lib.sh"
[ -f .env ] && { say ".env already exists, left untouched"; exit 0; }
cp .env.example .env
sed -i "s|^DB_PASSWORD=.*|DB_PASSWORD=$(rand 48 32)|" .env
sed -i "s|^JWT_SECRET=.*|JWT_SECRET=$(rand 96 64)|" .env
sed -i "s|^BOOTSTRAP_ADMIN_PASSWORD=.*|BOOTSTRAP_ADMIN_PASSWORD=$(rand 24 16)|" .env
chmod 600 .env
say ".env created with random secrets."
say "Edit PUBLIC_URL (and the payment/SMS settings) before going live."
say "First administrator: user '$(env_get BOOTSTRAP_ADMIN_USER)', password in .env (BOOTSTRAP_ADMIN_PASSWORD). Change it at first login."
