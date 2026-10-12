#!/bin/sh
set -eu
if ! getent group portico >/dev/null 2>&1; then groupadd --system portico; fi
if ! id portico >/dev/null 2>&1; then useradd --system --gid portico --home-dir /var/lib/portico-media-server --shell /usr/sbin/nologin portico; fi
install -d -o portico -g portico -m 0700 /var/lib/portico-media-server

# Debian runs this as "configure <previous version>", with no previous version
# on a first install. RPM passes how many copies of the package are installed:
# 1 on a first install, 2 or more on an upgrade.
case "${1:-}" in
  configure) if [ -n "${2:-}" ]; then mode=upgrade; else mode=install; fi ;;
  abort-remove) mode=install ;;
  abort-upgrade | abort-deconfigure) mode=upgrade ;;
  1 | '') mode=install ;;
  *) mode=upgrade ;;
esac

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  if [ "$mode" = install ]; then
    systemctl enable portico-media-server.service || true
  fi
  # /run/systemd/system exists only while systemd is the running init, so an
  # image build or a chroot installs the service without trying to start it.
  if [ -d /run/systemd/system ]; then
    if [ "$mode" = install ]; then
      systemctl start portico-media-server.service || true
    else
      # An upgrade keeps what the owner had: a running server moves onto the
      # new build, and one they stopped or disabled stays that way.
      systemctl try-restart portico-media-server.service || true
    fi
  fi
fi
