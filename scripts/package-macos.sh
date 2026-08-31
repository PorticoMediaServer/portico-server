#!/usr/bin/env bash
set -euo pipefail
export COPYFILE_DISABLE=1

reject_macos_metadata() {
  local root="$1"
  local found
  found="$(find "$root" -type f \( -name '._*' -o -name '.DS_Store' \) -print -quit)"
  if [[ -n "$found" ]]; then
    echo "macOS package contains forbidden metadata: $found" >&2
    exit 1
  fi
}

if [[ $# -ne 3 ]]; then echo "usage: $0 VERSION FFMPEG_ROOT BUILD_NUMBER" >&2; exit 2; fi
VERSION="$1"; FFMPEG_ROOT="$2"; BUILD_NUMBER="$3"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PAYLOAD="$ROOT/dist/stage-macos-arm64"
DMG_ROOT="$ROOT/dist/dmg-root"
APP="$DMG_ROOT/Portico Media Server.app"
"$ROOT/scripts/build-release-payload.sh" "$VERSION" darwin arm64 "$FFMPEG_ROOT" "$PAYLOAD" "$BUILD_NUMBER"
rm -rf "$DMG_ROOT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/bin" "$APP/Contents/Resources/web" "$APP/Contents/Resources/licenses" "$APP/Contents/Resources/launchd"
cp "$PAYLOAD/portico-media-server" "$APP/Contents/Resources/bin/portico-media-server"
CGO_ENABLED=1 go build -C "$ROOT" -trimpath \
  -ldflags="-s -w -X main.version=$VERSION -X main.buildNumber=$BUILD_NUMBER" \
  -o "$APP/Contents/Resources/bin/portico-desktop" ./cmd/portico-desktop
cp "$PAYLOAD/bin/ffmpeg" "$PAYLOAD/bin/ffprobe" "$APP/Contents/Resources/bin/"
cp -R "$PAYLOAD/web/." "$APP/Contents/Resources/web/"
cp -R "$PAYLOAD/licenses/." "$APP/Contents/Resources/licenses/"
cp "$PAYLOAD/LICENSE" "$PAYLOAD/THIRD-PARTY-NOTICES.md" "$APP/Contents/Resources/"
cp "$ROOT/packaging/macos/tv.getportico.server.service.plist" "$ROOT/packaging/macos/tv.getportico.server.companion.plist" "$APP/Contents/Resources/launchd/"
sed -e "s/__PORTICO_VERSION__/$VERSION/g" -e "s/__PORTICO_BUILD__/$BUILD_NUMBER/g" "$ROOT/packaging/macos/Info.plist" > "$APP/Contents/Info.plist"
sed "s/__PORTICO_VERSION__/$VERSION/g" "$ROOT/packaging/macos/launcher.sh" > "$APP/Contents/MacOS/Portico Media Server"
chmod +x "$APP/Contents/MacOS/Portico Media Server" "$APP/Contents/Resources/bin/portico-media-server" "$APP/Contents/Resources/bin/portico-desktop" "$APP/Contents/Resources/bin/ffmpeg" "$APP/Contents/Resources/bin/ffprobe"
ICONSET="$ROOT/dist/PorticoServer.iconset"
rm -rf "$ICONSET"
mkdir -p "$ICONSET"
for spec in "16:16x16" "32:16x16@2x" "32:32x32" "64:32x32@2x" "128:128x128" "256:128x128@2x" "256:256x256" "512:256x256@2x" "512:512x512" "1024:512x512@2x"; do
  pixels="${spec%%:*}"; name="${spec#*:}"
  sips -z "$pixels" "$pixels" "$PAYLOAD/web/brand/portico-app-icon-512.png" --out "$ICONSET/icon_${name}.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/PorticoServer.icns"
rm -rf "$ICONSET"
if [[ -n "${PORTICO_MACOS_SIGNING_IDENTITY:-}" ]]; then
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP/Contents/Resources/bin/portico-media-server"
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP/Contents/Resources/bin/portico-desktop"
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP/Contents/Resources/bin/ffmpeg"
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP/Contents/Resources/bin/ffprobe"
  codesign --force --options runtime --timestamp=none --sign "$PORTICO_MACOS_SIGNING_IDENTITY" "$APP"
  codesign --verify --deep --strict "$APP"
fi
ln -s /Applications "$DMG_ROOT/Applications"
reject_macos_metadata "$DMG_ROOT"
hdiutil create -quiet -volname "Portico Media Server" -srcfolder "$DMG_ROOT" -ov -format UDZO "$ROOT/dist/Portico-Media-Server-macOS-arm64.dmg"
