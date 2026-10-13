#!/bin/sh
# Run with sudo. Server data and the service account are deliberately retained.
set -eu
launchctl bootout system/tv.getportico.server.service 2>/dev/null || true
rm -f /Library/LaunchDaemons/tv.getportico.server.service.plist
rm -rf '/Applications/Portico Media Server.app'
pkgutil --forget tv.getportico.server >/dev/null
