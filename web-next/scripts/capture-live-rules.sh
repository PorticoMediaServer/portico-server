#!/usr/bin/env bash
# Capture the Live TV > Recordings > Series rules gallery state and its design frame at 1440x950.
# Usage: scripts/capture-live-rules.sh <design-ui-v1-root> <output-dir> [chromium]
# The design root is only read and served; nothing is written inside it.
set -euo pipefail
design=${1:?usage: capture-live-rules.sh <design-ui-v1-root> <output-dir> [chromium]}
out=${2:?usage: capture-live-rules.sh <design-ui-v1-root> <output-dir> [chromium]}
chrome=${3:-}
[ -n "$chrome" ] || chrome=$(command -v chromium || command -v chromium-browser || command -v google-chrome)
site="$design/design/ui-v1"
[ -f "$site/desktop-live.html" ] || { echo "desktop-live.html not found under $site" >&2; exit 1; }
here=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/portico-rules-capture.XXXXXX")
vite_pid=
design_pid=
out_pid=
cleanup() {
  [ -n "$vite_pid" ] && kill -- -"$vite_pid" 2>/dev/null || true
  [ -n "$design_pid" ] && kill "$design_pid" 2>/dev/null || true
  [ -n "$out_pid" ] && kill "$out_pid" 2>/dev/null || true
  wait 2>/dev/null || true
  rm -rf "$tmp"
  rm -f "$out/_live-rules-capture.html" "$out/_capture-manrope.woff2"
}
trap cleanup EXIT

port=$((44000 + RANDOM % 1000))
design_port=$((45000 + RANDOM % 1000))
out_port=$((46000 + RANDOM % 1000))
cp "$here/src/kit/fonts/manrope-var-latin.woff2" "$out/_capture-manrope.woff2"
python3 - "$site" "$out/_live-rules-capture.html" "$out/_capture-manrope.woff2" "$design_port" "$out_port" <<'PY'
import pathlib, re, sys
site, target, font, design_port, out_port = sys.argv[1:]
html = (pathlib.Path(site) / 'desktop-live.html').read_text()
html = re.sub(r'\s*<link rel="preconnect"[^>]*>', '', html)
html = re.sub(r'\s*<link href="https://fonts.googleapis.com[^>]*>', '', html)
html = html.replace('<head>', f'<head><base href="http://127.0.0.1:{design_port}/">', 1)
style = f'''<style>
@font-face {{ font-family: 'Manrope'; src: url('http://127.0.0.1:{out_port}/_capture-manrope.woff2') format('woff2'); font-style: normal; font-weight: 400 800; font-display: block; }}
html, body {{ margin: 0 !important; padding: 0 !important; background: #071018 !important; }}
.frame {{ border-radius: 0 !important; }}
</style>'''
script = '''<script>
(() => {
  document.documentElement.classList.add('standalone');
  const wrap = document.querySelector('.wrap');
  for (const child of [...wrap.children]) {
    if (!(child.classList.contains('frame') && child.querySelector('#live-rules'))) child.remove();
  }
  wrap.style.padding = '0';
  wrap.style.gap = '0';
  const screen = document.querySelector('#live-rules');
  screen.style.setProperty('--z', '1');
  screen.style.height = '950px';
})();
</script>'''
html = html.replace('</head>', style + '</head>')
html = html.replace('</body>', script + '</body>')
pathlib.Path(target).write_text(html)
PY

shot() {
  local url=$1 file=$2 profile=$3
  "$chrome" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --hide-scrollbars \
    --force-prefers-reduced-motion --force-device-scale-factor=1 --window-size=1440,950 --virtual-time-budget=15000 \
    --user-data-dir="$tmp/$profile" --screenshot="$file" "$url" > "$out/$profile.log" 2>&1
  [ -s "$file" ] || { echo "capture failed: $file" >&2; exit 1; }
}

# The gallery state, straight from the Vite dev server.
(cd "$here" && exec setsid npx vite --host 127.0.0.1 --port "$port" --strictPort > "$out/vite.log" 2>&1) &
vite_pid=$!
for _ in $(seq 1 60); do
  python3 -c "import socket; s=socket.create_connection(('127.0.0.1',$port),1); s.close()" 2>/dev/null && break
  sleep 2
done
shot "http://127.0.0.1:$port/dev/kit?family=live&case=series-rules&capture=true" "$out/live-rules-gallery.png" gallery
kill -- -"$vite_pid" 2>/dev/null || true
wait "$vite_pid" 2>/dev/null || true
vite_pid=

# The design frame, served read-only, with the gallery's font and a 1440x950 viewport.
python3 -m http.server "$design_port" --bind 127.0.0.1 --directory "$site" > "$out/frame-design-server.log" 2>&1 &
design_pid=$!
python3 -m http.server "$out_port" --bind 127.0.0.1 --directory "$out" > "$out/frame-output-server.log" 2>&1 &
out_pid=$!
for _ in $(seq 1 30); do
  python3 -c "import socket; s=socket.create_connection(('127.0.0.1',$design_port),1); s.close(); s=socket.create_connection(('127.0.0.1',$out_port),1); s.close()" 2>/dev/null && break
  sleep 1
done
shot "http://127.0.0.1:$out_port/_live-rules-capture.html" "$out/live-rules-frame.png" frame
printf 'gallery %s bytes, frame %s bytes\n' "$(stat -c %s "$out/live-rules-gallery.png")" "$(stat -c %s "$out/live-rules-frame.png")"
