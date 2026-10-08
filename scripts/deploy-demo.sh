#!/bin/bash
# Deploy a portico-server commit to the demo (demo.getportico.tv).
#
# The server is a release build on the G12 runner (the Hosted root compiled in); the web
# app is built on this Mac from the same commit. Both install on the demo as
# /opt/portico/demo-releases/portico-<sha>, then the unit portico-demo-current switches to it
# and the script waits for readiness. The state directory is copied aside first
# (<state>.bak-pre-<sha>); with --fresh it is also moved aside and the server starts empty
# (the owner then redoes setup and the claim, and re-adds the libraries). The previous
# release is kept for rollback; older ones are removed.
#
# Usage: scripts/deploy-demo.sh <portico-server commit> [--stage-only|--fresh]
# PORTICO_SERVER_REPO points at the portico-server checkout (default: ../apps/portico-server).
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
server_repo=${PORTICO_SERVER_REPO:-"$here/../apps/portico-server"}
SV=$(git -C "$server_repo" rev-parse --short=8 "${1:?portico-server commit}")
MODE=${2:-}
REL=portico-$SV
FRESH=""; [ "$MODE" = "--fresh" ] && FRESH=1
ROOT_KEY=$(cd "$here" && go run scripts/inspect-hosted-root.go deployment/hosted/root-public-key.pem)
work=$(mktemp -d -t portico-demo-deploy.XXXXXX)
trap 'rm -rf "${work:?}"' EXIT

echo "== server $SV: release build on the G12"
ssh portico-runner-g12 "rm -rf ~/build/${SV:?} && mkdir -p ~/build/${SV:?}"
git -C "$server_repo" archive "$SV" server apikit | ssh portico-runner-g12 "tar -x -C ~/build/${SV:?}"
ssh portico-runner-g12 "source /etc/profile; cd ~/build/${SV:?}/server && go build -tags release -trimpath -ldflags '-X portico.local/server/internal/hostedtrust.OfficialRoot=$ROOT_KEY' -o ~/build/${SV:?}/portico-server ./cmd/server && sha256sum ~/build/${SV:?}/portico-server"

echo "== web $SV: build on this Mac"
git -C "$server_repo" archive "$SV" | tar -x -C "$work"
(cd "$work" && npm ci --ignore-scripts --no-audit --no-fund > "$work/npm.log" 2>&1) || { tail -30 "$work/npm.log"; exit 1; }
(cd "$work/web" && VITE_HOSTED_URL=https://web.getportico.tv nice -n 10 npx vite build --outDir "$work/out" --emptyOutDir > "$work/web.log" 2>&1) || { tail -30 "$work/web.log"; exit 1; }
printf '{"server":"%s","builtAt":"%s"}\n' "$SV" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$work/out/portico-build.json"

echo "== upload $REL"
ssh portico-demo "rm -rf /opt/portico/demo-releases/${REL:?}.new && mkdir -p /opt/portico/demo-releases/${REL:?}.new"
ssh portico-runner-g12 "cat ~/build/${SV:?}/portico-server" | ssh portico-demo "cat > /opt/portico/demo-releases/${REL:?}.new/portico-server && chmod 755 /opt/portico/demo-releases/${REL:?}.new/portico-server"
(COPYFILE_DISABLE=1 tar --no-xattrs -c -C "$work/out" .) | ssh portico-demo "mkdir -p /opt/portico/demo-releases/${REL:?}.new/web && tar -x --no-same-owner -C /opt/portico/demo-releases/${REL:?}.new/web"
ssh portico-demo "set -e; D=/opt/portico/demo-releases; U=/etc/systemd/system/portico-demo-current.service
CUR=\$(basename \$(dirname \$(grep -o '^ExecStart=[^ ]*' \$U | cut -d= -f2)))
[ \"\$CUR\" = ${REL:?} ] && { echo 'refusing to replace the running release'; exit 1; }
rm -rf \$D/${REL:?}; mv \$D/${REL:?}.new \$D/${REL:?}; \$D/${REL:?}/portico-server --version"
ssh portico-runner-g12 "rm -rf ~/build/${SV:?}"
if [ "$MODE" = "--stage-only" ]; then echo "STAGED $REL (not switched)"; exit 0; fi

echo "== switch"
ssh portico-demo "set -e
D=/opt/portico/demo-releases; ST=/var/lib/portico-demo-20260923; U=/etc/systemd/system/portico-demo-current.service; E=/etc/portico-demo-20260919.env
OLD=\$(basename \$(dirname \$(grep -o '^ExecStart=[^ ]*' \$U | cut -d= -f2)))
echo previous \$OLD
systemctl stop portico-demo-current
for b in \$ST.bak-pre-*; do [ -e \"\$b\" ] && rm -rf \"\${b:?}\"; done
cp -a \$ST \$ST.bak-pre-${SV:?}
if [ -n \"${FRESH}\" ]; then mv \$ST \$ST.pre-${SV:?}; install -d -m 700 -o \$(stat -c %U \$ST.pre-${SV:?}) -g \$(stat -c %G \$ST.pre-${SV:?}) \$ST; echo fresh state; fi
sed -i \"s|^ExecStart=.*|ExecStart=\$D/${REL:?}/portico-server|; s|^WorkingDirectory=.*|WorkingDirectory=\$D/${REL:?}|\" \$U
sed -i \"s|^PORTICO_WEB_DIR=.*|PORTICO_WEB_DIR=\$D/${REL:?}/web|\" \$E
systemctl daemon-reload; systemctl start portico-demo-current
ok=0; for i in \$(seq 1 240); do c=\$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:32503/v1/readiness || true); [ \"\$c\" = 200 ] && { ok=1; echo ready after \$i s; break; }; sleep 1; done
[ \$ok = 1 ] || { journalctl -u portico-demo-current -n 40 --no-pager; echo DEPLOY-NOT-READY; exit 1; }
for r in \$D/portico-*; do n=\$(basename \$r); [ \"\$n\" = ${REL:?} ] || [ \"\$n\" = \"\$OLD\" ] || rm -rf \"\${r:?}\"; done
ls \$D; ls -d \$ST.bak-*"
curl -s https://demo.getportico.tv/portico-build.json; echo
curl -s -o /dev/null -w 'readiness %{http_code}\n' https://demo.getportico.tv/v1/readiness
curl -s -o /dev/null -w 'index %{http_code}\n' https://demo.getportico.tv/
echo DEPLOY-DONE "$SV"
