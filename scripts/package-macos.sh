#!/usr/bin/env bash
# Turn the macOS payload (scripts/package-release.sh darwin arm64 …) into
# Portico Media Server.app inside a DMG. Runs on macOS.
#   scripts/package-macos.sh VERSION PAYLOAD_DIR OUT_DIR BUILD_NUMBER
set -euo pipefail
export COPYFILE_DISABLE=1
if [[ $# -ne 4 ]]; then echo "usage: $0 VERSION PAYLOAD_DIR OUT_DIR BUILD_NUMBER" >&2; exit 2; fi
VERSION="$1"; PAYLOAD="$2"; OUT="$3"; BUILD_NUMBER="$4"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DMG_ROOT="$OUT/dmg-root"
APP="$DMG_ROOT/Portico Media Server.app"
rm -rf "${DMG_ROOT:?}"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/server" "$APP/Contents/Resources/launchd"
cp -R "$PAYLOAD/." "$APP/Contents/Resources/server/"
cp "$ROOT/packaging/macos/tv.getportico.server.service.plist" "$APP/Contents/Resources/launchd/"
sed -e "s/__PORTICO_VERSION__/$VERSION/g" -e "s/__PORTICO_BUILD__/$BUILD_NUMBER/g" "$ROOT/packaging/macos/Info.plist" > "$APP/Contents/Info.plist"
cp "$ROOT/packaging/macos/launcher.sh" "$APP/Contents/MacOS/Portico Media Server"
chmod +x "$APP/Contents/MacOS/Portico Media Server" "$APP/Contents/Resources/server/portico-server" "$APP/Contents/Resources/server/third_party/ffmpeg/bin/"*
icon="$(find "$APP/Contents/Resources/server/web" -name 'portico-app-icon-512.png' -print -quit)"
if [[ -n "$icon" ]]; then
  iconset="$OUT/PorticoServer.iconset"
  rm -rf "${iconset:?}"; mkdir -p "$iconset"
  for spec in "16:16x16" "32:16x16@2x" "32:32x32" "64:32x32@2x" "128:128x128" "256:128x128@2x" "256:256x256" "512:256x256@2x" "512:512x512" "1024:512x512@2x"; do
    sips -z "${spec%%:*}" "${spec%%:*}" "$icon" --out "$iconset/icon_${spec#*:}.png" >/dev/null
  done
  iconutil -c icns "$iconset" -o "$APP/Contents/Resources/PorticoServer.icns"
  rm -rf "${iconset:?}"
fi
if [[ -n "${PORTICO_MACOS_SIGNING_IDENTITY:-}" ]]; then
  for binary in "$APP/Contents/Resources/server/portico-server" "$APP/Contents/Resources/server/third_party/ffmpeg/bin/"*; do
    codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$binary"
  done
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP"
  codesign --verify --deep --strict "$APP"
fi
ln -s /Applications "$DMG_ROOT/Applications"
if find "$DMG_ROOT" -type f \( -name '._*' -o -name '.DS_Store' \) | grep -q .; then echo "macOS package contains Finder metadata" >&2; exit 1; fi
hdiutil create -quiet -volname "Portico Media Server" -srcfolder "$DMG_ROOT" -ov -format UDZO "$OUT/Portico-Media-Server-macOS-arm64.dmg"
