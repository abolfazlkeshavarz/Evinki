#!/usr/bin/env bash
# Build on a strong PC, ship to a small server (no building there). PLATFORM=linux/arm64 for ARM servers.
source "$(dirname "$0")/lib.sh"
mkdir -p dist
[ -f .env ] || ./scripts/gen-secrets.sh >/dev/null
DOCKER_DEFAULT_PLATFORM="${PLATFORM:-linux/amd64}" $COMPOSE build
docker save evinki-api:latest evinki-web:latest | gzip > dist/evinki-images.tar.gz
say "dist/evinki-images.tar.gz ($(du -h dist/evinki-images.tar.gz | cut -f1))"
say "copy it with the project folder to the server, then: make load-images && docker compose up -d --no-build"
