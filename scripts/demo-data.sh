#!/usr/bin/env bash
#
# Loads a sample catalogue (FZ530, accessories, services, shifts, the standard survey).
# Safe to re-run. Use on a demo/staging server or at the start of a real one.   make demo
exec bash "$(dirname "$0")/admin.sh" seed-demo
