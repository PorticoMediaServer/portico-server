#!/bin/bash
# Build on the personal Ubuntu host and deploy a committed server/web snapshot.
# Retain state, media, releases and backups; roll back a failed activation.
# Usage: scripts/deploy-demo.sh <server commit> [--stage-only|--activate-only]
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
server_repo=${PORTICO_SERVER_REPO:-"$here/../apps/portico-server"}
sha=$(git -C "$server_repo" rev-parse "${1:?server commit required}^{commit}")
short=${sha:0:8}
mode=${2:-}
case "$mode" in ''|--stage-only|--activate-only) ;; *) echo "Unknown mode: $mode" >&2; exit 2 ;; esac
release=portico-$short
stamp=$(date -u +%Y%m%dT%H%M%SZ)
work=$(mktemp -d -t portico-demo-deploy.XXXXXX)
trap 'rm -rf "${work:?}"' EXIT
if [ "$mode" != --activate-only ]; then
  git -C "$server_repo" archive "$sha" | tar -x -C "$work"
  root_key=$(cd "$here" && go run scripts/inspect-hosted-root.go deployment/hosted/root-public-key.pem)
  flags="-X portico.local/server/internal/hostedtrust.OfficialRoot=$root_key -X portico.local/server/internal/buildinfo.Version=0.1.0-demo+$short -X portico.local/server/internal/buildinfo.BuildID=$sha -X portico.local/server/internal/buildinfo.BuiltAt=$stamp"
  (cd "$work/server" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags release -trimpath -buildvcs=false -mod=readonly -ldflags "$flags" -o "$work/portico-server" ./cmd/server)
  (cd "$work" && npm ci --no-audit --no-fund > "$work/npm.log" 2>&1) || { tail -30 "$work/npm.log"; exit 1; }
  (cd "$work/web" && VITE_HOSTED_URL=https://web.getportico.tv npm run build -- --outDir "$work/out" --emptyOutDir > "$work/web.log" 2>&1) || { tail -30 "$work/web.log"; exit 1; }
  printf '{"server":"%s","commit":"%s","builtAt":"%s"}\n' "$short" "$sha" "$stamp" > "$work/out/portico-build.json"
  (cd "$work" && sha256sum portico-server > server.sha256)
  ssh portico-demo "test ! -e /opt/portico/demo-releases/$release && install -d -m 755 /opt/portico/demo-releases/$release.new-$stamp/web"
  scp -q "$work/portico-server" "$work/server.sha256" "portico-demo:/opt/portico/demo-releases/$release.new-$stamp/"
  tar -c -C "$work/out" . | ssh portico-demo "tar -x --no-same-owner -C /opt/portico/demo-releases/$release.new-$stamp/web"
  ssh portico-demo "set -e; cd /opt/portico/demo-releases/$release.new-$stamp; sha256sum -c server.sha256; chmod 755 portico-server; ./portico-server --version; mv /opt/portico/demo-releases/$release.new-$stamp /opt/portico/demo-releases/$release"
  artifacts="$server_repo/.scratch/demo-releases/$sha/$stamp"
  install -d -m 755 "$artifacts"
  install -m 755 "$work/portico-server" "$artifacts/portico-server"
  cp "$work/server.sha256" "$work/out/portico-build.json" "$artifacts/"
  echo "Local build receipt and binary: $artifacts"
fi
if [ "$mode" = --stage-only ]; then echo "STAGED $release"; exit 0; fi
ssh portico-demo bash -s -- "$release" "$stamp" "$sha" <<'REMOTE'
set -euo pipefail
umask 077
release=$1; stamp=$2; sha=$3
directory=/opt/portico/demo-releases
state=/var/lib/portico-demo-20261017
unit=/etc/systemd/system/portico-demo-current.service
environment=/etc/portico-demo-20261017.env
backup=/var/backups/portico/demo-$stamp
previous=$(dirname "$(sed -n 's/^ExecStart=\([^ ]*\).*/\1/p' "$unit")")
test "$previous" != "$directory/$release"
test -x "$directory/$release/portico-server"
test -f "$directory/$release/web/index.html"
python3 - "$directory/$release/web/portico-build.json" "$sha" <<'CHECK'
import json,sys
assert json.load(open(sys.argv[1]))['commit'] == sys.argv[2]
CHECK
install -d -m 700 "$backup"
required=$(du -sB1 "$state" | cut -f1)
available=$(df --output=avail -B1 "$state" | tail -1 | tr -d ' ')
test "$available" -gt "$((required + 1073741824))" || { echo 'Not enough space for the preserved state and rollback reserve.' >&2; exit 1; }
cp -a "$unit" "$backup/service"
cp -a "$environment" "$backup/environment"
printf '%s\n' "$previous" > "$backup/previous-release"
stopped=0; backed_up=0; committed=0
rollback() {
  result=$?
  trap - EXIT
  if [ "$stopped" = 1 ] && [ "$committed" = 0 ]; then
    systemctl stop portico-demo-current || true
    cp -a "$backup/service" "$unit"
    cp -a "$backup/environment" "$environment"
    if [ "$backed_up" = 1 ]; then
      mv "$state" "$state.failed-$stamp"
      cp -a "$backup/state" "$state"
    fi
    systemctl daemon-reload
    systemctl start portico-demo-current
    echo "ROLLED BACK to $previous; backup $backup" >&2
  fi
  exit "$result"
}
trap rollback EXIT
systemctl stop portico-demo-current
stopped=1
cp -a "$state" "$backup/state"
cmp "$state/server.sqlite" "$backup/state/server.sqlite"
python3 - "$backup/state/server.sqlite" "$backup/catalogue-before.json" <<'CHECK'
import json,sqlite3,sys
db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
assert db.execute('PRAGMA quick_check').fetchall()==[('ok',)]
counts={'entities':db.execute('SELECT COUNT(*) FROM catalog_entities').fetchone()[0],
        'availableFiles':db.execute('SELECT COUNT(*) FROM catalog_assets WHERE available=1').fetchone()[0]}
assert db.execute('SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1').fetchone()==(0,), 'Demo transcoding must be disabled before deployment'
with open(sys.argv[2],'w') as output:json.dump(counts,output)
print('Backup database verified; catalogue:',counts)
CHECK
backed_up=1
sed -i "s|^ExecStart=.*|ExecStart=$directory/$release/portico-server|; s|^WorkingDirectory=.*|WorkingDirectory=$directory/$release|" "$unit"
sed -i "s|^PORTICO_WEB_DIR=.*|PORTICO_WEB_DIR=$directory/$release/web|" "$environment"
systemctl daemon-reload
systemctl start portico-demo-current
ready=0
for ((attempt=0; attempt<120; attempt++)); do
  if curl -fsS http://127.0.0.1:32503/v1/readiness >/dev/null; then ready=1; break; fi
  sleep 1
done
test "$ready" = 1
curl -fsS http://127.0.0.1:32503/portico-build.json
python3 - "$state/server.sqlite" "$backup/catalogue-before.json" <<'CHECK'
import json,sqlite3,sys
db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
assert db.execute('PRAGMA quick_check').fetchall()==[('ok',)]
before=json.load(open(sys.argv[2]))
after={'entities':db.execute('SELECT COUNT(*) FROM catalog_entities').fetchone()[0],
       'availableFiles':db.execute('SELECT COUNT(*) FROM catalog_assets WHERE available=1').fetchone()[0]}
assert after['entities']>=before['entities'] and after['availableFiles']>=before['availableFiles'], (before,after)
assert db.execute('SELECT transcoding_enabled FROM playback_owner_policy WHERE singleton=1').fetchone()==(0,), 'Demo transcoding unexpectedly enabled'
print('Active database verified; preserved catalogue:',after)
CHECK
committed=1
printf '\nACTIVE %s; rollback backup %s\n' "$release" "$backup"
REMOTE
curl -fsS https://demo.getportico.tv/portico-build.json
curl -fsS https://demo.getportico.tv/v1/readiness
