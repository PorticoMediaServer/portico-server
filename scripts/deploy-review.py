#!/usr/bin/env python3
"""Stage/activate a remotely built main + web-next review with fresh state."""
import argparse
import hashlib
from pathlib import Path
import re
import shlex
import subprocess
import tarfile


def remote(script, *args):
    command = "bash -s -- " + " ".join(shlex.quote(str(arg)) for arg in args)
    subprocess.run(["ssh", "-o", "BatchMode=yes", "portico-demo", command],
                   input=script, text=True, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("commit")
    parser.add_argument("artifacts", type=Path)
    parser.add_argument("--cycle", required=True)
    parser.add_argument("--stage-only", action="store_true")
    parser.add_argument("--activate-only", action="store_true")
    parser.add_argument("--attempt", help="safe retry name; keep the same state and preserve earlier rollback backups")
    parser.add_argument("--runtime-root", help="installed qualified full Linux bundle for this fresh stage")
    parser.add_argument('--browser-repo', type=Path)
    parser.add_argument('--runner', type=Path)
    parser.add_argument('--private-review', type=Path)
    parser.add_argument('--smoke-output', type=Path)
    parser.add_argument('--smoke-host', choices=['netcup', 'contabo'], default='netcup')
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.commit):
        parser.error("a full published MAIN commit is required")
    if not re.fullmatch(r"review-[a-z0-9-]{1,64}", args.cycle):
        parser.error("cycle must be a safe review- name")
    if args.stage_only == args.activate_only:
        parser.error("choose --stage-only, then --activate-only after public setup")
    if args.runtime_root and (not args.stage_only or not re.fullmatch(r"/opt/portico/media-runtimes/[A-Za-z0-9-]{1,64}/bundle", args.runtime_root)):
        parser.error("runtime root must be a safe installed bundle used only with --stage-only")
    if args.activate_only and not all((args.browser_repo, args.runner, args.private_review, args.smoke_output)):
        parser.error('activation requires the real browser smoke repo, runner, private review file and output')
    if args.attempt and (not args.activate_only or not re.fullmatch(r'[a-z0-9-]{1,24}', args.attempt)):
        parser.error('attempt names apply only to activation retries')
    backup_cycle = args.cycle + ('-' + args.attempt if args.attempt else '')
    directory = "/opt/portico/demo-releases/" + args.cycle
    state = "/var/lib/portico-" + args.cycle
    environment = "/etc/portico-" + args.cycle + ".env"
    if args.stage_only:
        binary, web = args.artifacts / "portico-server", args.artifacts / "web.tar"
        if not binary.is_file() or not web.is_file():
            parser.error("remotely built portico-server and web.tar are required")
        with tarfile.open(web) as archive:
            members = archive.getmembers()
            if not members or not any(x.name in ("index.html", "./index.html") for x in members):
                parser.error("web-next index.html is absent")
            for member in members:
                if (Path(member.name).is_absolute() or ".." in Path(member.name).parts
                        or not (member.isdir() or member.isfile())):
                    parser.error("unsafe built web member")
        with binary.open("rb") as stream:
            digest = hashlib.file_digest(stream, "sha256").hexdigest()
        remote("""set -euo pipefail
test ! -e "$1"; test ! -e "$2"
test "$(systemctl is-active portico-review-stage || true)" != active
install -d -m755 "$1/web"
install -d -o portico -g portico -m700 "$2" "$2/cache-home"
""", directory, state)
        subprocess.run(["scp", "-q", str(binary), "portico-demo:" + directory + "/portico-server"], check=True)
        with web.open("rb") as stream:
            subprocess.run(["ssh", "-o", "BatchMode=yes", "portico-demo",
                            "tar -xf - --no-same-owner -C " + shlex.quote(directory + "/web")], stdin=stream, check=True)
        remote(STAGE, directory, state, environment, args.commit, digest, args.runtime_root or "")
    else:
        command = ['python3', str(Path(__file__).with_name('smoke-review.py')), args.commit,
                   '--repo', str(args.browser_repo), '--runner', str(args.runner),
                   '--credentials', str(args.private_review), '--output', str(args.smoke_output),
                   '--host', args.smoke_host, '--activate-cycle', args.cycle]
        if args.attempt:
            command += ['--attempt', args.attempt]
        subprocess.run(command, check=True)


STAGE = r'''set -euo pipefail
directory=$1; state=$2; environment=$3; commit=$4; expected=$5
printf '%s  %s\n' "$expected" "$directory/portico-server" | sha256sum -c -
chmod 755 "$directory/portico-server"
python3 - "$directory/web/portico-build.json" "$commit" <<'PY'
import json,sys
with open(sys.argv[1],'w') as out:json.dump({'commit':sys.argv[2],'client':'web-next','purpose':'review'},out)
PY
umask 077
if [ -n "${6:-}" ]; then
 runtime=$6
 test -f "$runtime/toolchain-manifest.json"; test -f "$runtime/corresponding-source.tar.xz"
 test -f "$runtime/NOTICE.md"; test -f "$runtime/LICENSES/portico-disc.txt"
 test -x "$runtime/bin/portico-disc"
 ffmpeg=$runtime/bin/ffmpeg; ffprobe=$runtime/bin/ffprobe
else
 ffmpeg=$(sed -n 's/^PORTICO_FFMPEG=//p' /etc/portico-demo-20261017.env)
 ffprobe=$(sed -n 's/^PORTICO_FFPROBE=//p' /etc/portico-demo-20261017.env)
fi
test -x "$ffmpeg"; test -x "$ffprobe"
install -d -o portico -g portico -m700 /var/cache/portico
cat > "$environment" <<ENV
PORTICO_DATA_DIR=$state
PORTICO_SERVICE=1
PORTICO_BIND=127.0.0.1:32504
PORTICO_WEB_DIR=$directory/web
PORTICO_FFMPEG=$ffmpeg
PORTICO_FFPROBE=$ffprobe
PORTICO_DISCOVERY=0
XDG_CACHE_HOME=$state/cache-home
GOMAXPROCS=1
GOMEMLIMIT=256MiB
ENV
cat > /etc/systemd/system/portico-review-stage.service <<UNIT
[Unit]
Description=Portico fresh review staging
After=network-online.target
[Service]
User=portico
Group=portico
EnvironmentFile=$environment
ExecStart=$directory/portico-server
WorkingDirectory=$directory
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$state /var/cache/portico
MemoryMax=512M
TasksMax=256
UNIT
systemctl daemon-reload
systemctl start portico-review-stage
for attempt in $(seq 1 60); do
 if curl -fsS http://127.0.0.1:32504/health/ready >/dev/null 2>&1; then
  printf 'Fresh review staged on loopback32504; complete public setup before activation.\n'
  exit 0
 fi
 sleep 1
done
systemctl stop portico-review-stage
exit 1
'''

ROLLBACK = r'''set -euo pipefail
backup=/var/backups/portico/$1
test -f "$backup/service"
systemctl stop portico-demo-current
cp -a "$backup/service" /etc/systemd/system/portico-demo-current.service
systemctl daemon-reload
systemctl start portico-demo-current
printf 'Browser smoke incomplete; previous review service and database restored.\n'
'''

BACKUP_ENVIRONMENTS = r'''python3 - "$backup" <<'ENV_PY'
from pathlib import Path
import re,shlex,shutil,subprocess,sys
backup=Path(sys.argv[1])
value=subprocess.check_output(['systemctl','show','portico-demo-current','--property=EnvironmentFiles','--value'],text=True)
tokens=shlex.split(value)
if not tokens or len(tokens)%2:
 raise ValueError('cannot identify outgoing service environment files')
for filename,status in zip(tokens[::2],tokens[1::2]):
 if not re.fullmatch(r'\(ignore_errors=(?:yes|no)\)',status):
  raise ValueError('unsupported service environment description')
 source=Path(filename)
 if not source.is_absolute() or '..' in source.parts:
  raise ValueError('absolute outgoing environment path required')
 if status=='(ignore_errors=yes)' and not source.exists():continue
 target=backup/'environments'/source.relative_to('/')
 target.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
 shutil.copy2(source,target)
ENV_PY
'''


ACTIVATE = r'''set -euo pipefail
directory=$1; state=$2; environment=$3; commit=$4; cycle=$5
test -x "$directory/portico-server"; test -f "$state/Database/server.sqlite"
curl -fsS http://127.0.0.1:32504/v1/system | python3 -c 'import json,sys;s=json.load(sys.stdin);assert s["signIn"]=="local" and not s["setupRequired"]'
backup=/var/backups/portico/$cycle
test ! -e "$backup"
install -d -m700 "$backup"
cp -a /etc/systemd/system/portico-demo-current.service "$backup/service"
''' + BACKUP_ENVIRONMENTS + r'''committed=0
rollback() {
 result=$?; trap - EXIT
 if [ "$committed" = 0 ]; then
  systemctl stop portico-demo-current || true
  cp -a "$backup/service" /etc/systemd/system/portico-demo-current.service
  systemctl daemon-reload; systemctl start portico-demo-current
  echo 'Review activation failed; old service/state restored without opening new DB.' >&2
 fi
 exit "$result"
}
trap rollback EXIT
systemctl stop portico-review-stage
systemctl stop portico-demo-current
sed -i 's/127.0.0.1:32504/127.0.0.1:32503/' "$environment"
sed 's|^Description=.*|Description=Portico current review server|' /etc/systemd/system/portico-review-stage.service > /etc/systemd/system/portico-demo-current.service
systemctl daemon-reload
systemctl start portico-demo-current
for attempt in $(seq 1 60); do
 if curl -fsS http://127.0.0.1:32503/health/ready >/dev/null 2>&1; then
  curl -fsS http://127.0.0.1:32503/portico-build.json | python3 -c 'import json,sys;assert json.load(sys.stdin)["commit"]==sys.argv[1]' "$commit"
  committed=1
  rm /etc/systemd/system/portico-review-stage.service
  systemctl daemon-reload
  printf 'Review active; previous database and service backup preserved.\n'
  exit 0
 fi
 sleep 1
done
exit 1
'''


if __name__ == "__main__":
    main()
