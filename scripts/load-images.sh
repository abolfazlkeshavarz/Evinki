#!/usr/bin/env bash
source "$(dirname "$0")/lib.sh"
[ -f dist/evinki-images.tar.gz ] || die "dist/evinki-images.tar.gz not found"
gunzip -c dist/evinki-images.tar.gz | docker load
say "images loaded; start with: docker compose up -d --no-build"
