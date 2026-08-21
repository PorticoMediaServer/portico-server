#!/bin/sh
set -eu
if ! getent group portico >/dev/null 2>&1; then groupadd --system portico; fi
if ! id portico >/dev/null 2>&1; then useradd --system --gid portico --home-dir /var/lib/portico-media-server --shell /usr/sbin/nologin portico; fi
install -d -o portico -g portico -m 0750 /var/lib/portico-media-server
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  systemctl enable portico-media-server.service || true
fi
