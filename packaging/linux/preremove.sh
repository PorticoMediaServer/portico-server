#!/bin/sh
set -eu
if command -v systemctl >/dev/null 2>&1; then
  systemctl stop portico-media-server.service || true
  systemctl disable portico-media-server.service || true
fi
