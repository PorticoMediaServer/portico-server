#!/usr/bin/env python3
"""Activate a reviewed Hosted release on the existing primary systemd deployment.

Does not publish the marketing site, change DNS/provider accounts, reset the
database, or copy secrets into the release. Run release tests before invoking.
"""
import argparse
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("release", type=Path)
    parser.add_argument("--hosted-only", action="store_true", help="Update Hosted without replacing the hosted web release")
    # An ssh destination: the alias in ~/.ssh/config (root, the shared Portico VPS key).
    parser.add_argument("--host", default="portico-core-us-1")
    parser.add_argument("--operator-account-id", required=True, help="Portico Account that gets the in-app signing-key expiry warning")
    # Justin's Apple Team ID (public: it is in the association file itself). Pass "" to serve 404 instead.
    parser.add_argument("--apple-team-id", default="WB7BUK5Z7C", help="Apple Team ID for apple-app-site-association (empty: the file answers 404)")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", args.operator_account_id) or (args.apple_team_id and not re.fullmatch(r"[A-Z0-9]{10}", args.apple_team_id)):
        raise SystemExit("Invalid operator account or Apple Team ID")
    release = args.release.resolve()
    manifest = json.loads((release / "release.json").read_text())
    build = manifest["buildId"]
    if not re.fullmatch(r"portico-[A-Za-z0-9-]+", build) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.@_-]*", args.host):
        raise SystemExit("Invalid release or host")
    ssh = ["ssh", "-o", "BatchMode=yes", args.host]
    subprocess.run(ssh + ["install -d -m 0755 /opt/portico-astra-hosted/releases"], check=True)
    subprocess.run(["scp", "-q", "-o", "BatchMode=yes", str(release) + ".tar.gz",
                    args.host + ":/opt/portico-astra-hosted/releases/" + build + ".tar.gz"], check=True)
    program = r'''
from pathlib import Path
import hashlib,json,subprocess,tarfile,time,urllib.request
import shutil
releases=Path('/opt/portico-astra-hosted/releases');root=releases/BUILD;archive_path=releases/(BUILD+'.tar.gz')
# Unpack beside the live tree and swap directories: a redeploy of the running
# build must not write into executables that are in use.
staging=releases/(BUILD+'.staging')
shutil.rmtree(staging,ignore_errors=True);staging.mkdir(mode=0o755)
with tarfile.open(archive_path) as archive: archive.extractall(staging,filter='data')
archive_path.unlink()
manifest=json.loads((staging/'release.json').read_text())
for name,digest in manifest['files'].items():
    file=staging/name
    if not file.resolve().is_relative_to(staging.resolve()) or hashlib.sha256(file.read_bytes()).hexdigest()!=digest:
        raise RuntimeError('Release hash mismatch')
for name in ['hosted','web-host','portico-dns']: (staging/name).chmod(0o755)
replaced=releases/(BUILD+'.replaced')
if root.exists():
    shutil.rmtree(replaced,ignore_errors=True);root.rename(replaced)
staging.rename(root)
config='/etc/portico-astra-hosted/'
import re as _re0
PREVIOUS=[]
for _name in ('portico-astra-hosted','portico-astra-web'):
    _unit=Path('/etc/systemd/system/'+_name+'.service')
    if _unit.is_file(): PREVIOUS+=_re0.findall(r'/opt/portico-astra-hosted/releases/([^/ ]+)/',_unit.read_text())
# Root trust: signing.json with its certificate and sealing file (KEYS.md).
for name in ['signing.json','signing.json.certificate.json','signing.json.seal.json']:
    if not (Path(config)/name).is_file(): raise RuntimeError('Missing '+config+name+'; install the root-trust key files first')
args=['-listen','127.0.0.1:19410','-key-file',config+'signing.json','-operator-account-id',OPERATOR,
      '-database-url-file',config+'database-url',
      '-origins','https://web.getportico.tv,https://demo.getportico.tv,http://127.0.0.1:32500,http://localhost:32500,http://127.0.0.1:32503',
      '-device-verification-uri','https://web.getportico.tv/device',
      '-claim-audience','https://web.getportico.tv','-access-token-lifetime','15m',
      '-oidc-config-file',config+'oidc.json','-direct-dns-key-file',config+'dns-mac',
      '-smtp-config-file',config+'smtp.json','-acme-config-file',config+'acme.json',
      '-trusted-proxy-cidrs','127.0.0.1/32']
def unit(description,command,memory,environment=''):
    return '[Unit]\nDescription='+description+'\nAfter=network-online.target postgresql.service\nWants=network-online.target postgresql.service\n[Service]\n'+environment+'User=portico-astra-hosted\nGroup=portico-astra-hosted\nExecStart='+command+'\nRestart=on-failure\nRestartSec=3\nNoNewPrivileges=true\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nReadWritePaths=/etc/portico-astra-hosted\nMemoryMax='+memory+'\nTasksMax=128\n[Install]\nWantedBy=multi-user.target\n'
# The running units, kept so a release that fails to come up is switched back.
SAVED={n:Path('/etc/systemd/system/'+n+'.service').read_text() for n in ('portico-astra-hosted','portico-astra-web') if Path('/etc/systemd/system/'+n+'.service').is_file()}
def switch_back(reason):
    for n,text in SAVED.items(): Path('/etc/systemd/system/'+n+'.service').write_text(text)
    subprocess.run(['systemctl','daemon-reload'],check=False)
    if SAVED: subprocess.run(['systemctl','restart']+list(SAVED),check=False)
    raise RuntimeError(reason+'; switched back to the previous release (if this release migrated the database, the previous binary refuses it: roll forward with a fixed release)')
Path('/etc/systemd/system/portico-astra-hosted.service').write_text(unit('Portico Hosted',str(root/'hosted')+' '+' '.join(args),'384M'))
if not HOSTED_ONLY: Path('/etc/systemd/system/portico-astra-web.service').write_text(unit('Portico Hosted Web',str(root/'web-host')+' -root '+str(root/'web')+' -listen 127.0.0.1:19418','96M',('Environment=PORTICO_APPLE_TEAM_ID='+TEAM+'\n') if TEAM else ''))
subprocess.run(['systemctl','daemon-reload'],check=True)
subprocess.run(['systemctl','enable','portico-astra-hosted','portico-astra-web'],check=True)
subprocess.run(['systemctl','restart','portico-astra-hosted']+([] if HOSTED_ONLY else ['portico-astra-web']),check=True)
for attempt in range(30):
    try:
        with urllib.request.urlopen('http://127.0.0.1:19410/v1/system',timeout=3) as response: info=json.load(response)
        with urllib.request.urlopen('http://127.0.0.1:19410/ready',timeout=3) as response: ready=response.status==200
        if info['buildId']==BUILD and ready:break
    except Exception: pass
    time.sleep(1)
else: switch_back('Hosted did not become ready; inspect its private journal')
try:
    with urllib.request.urlopen('http://127.0.0.1:19418/portico-build.json',timeout=5) as response: web=json.load(response)
except Exception: switch_back('Hosted web did not answer')
if not HOSTED_ONLY and web['buildId']!=BUILD: switch_back('Hosted web build mismatch')
# Keep this release and the one it replaced; remove older ones.
import re as _re
active=set()
for name in ('portico-astra-hosted','portico-astra-web'):
    text=Path('/etc/systemd/system/'+name+'.service').read_text()
    active.update(_re.findall(r'/opt/portico-astra-hosted/releases/([^/ ]+)/',text))
keep=active|{BUILD}|set(PREVIOUS)
for old in Path('/opt/portico-astra-hosted/releases').iterdir():
    if old.name not in keep: subprocess.run(['rm','-rf','--',str(old)],check=True)
print(json.dumps({'buildId':BUILD,'hosted':info,'webBuildId':web['buildId'],'releasesKept':sorted(p.name for p in Path('/opt/portico-astra-hosted/releases').iterdir())}))
'''
    subprocess.run(ssh + ["python3 -"], input="BUILD=" + repr(build) + "\nHOSTED_ONLY=" + repr(args.hosted_only) + "\nOPERATOR=" + repr(args.operator_account_id) + "\nTEAM=" + repr(args.apple_team_id) + "\n" + program, text=True, check=True)


if __name__ == "__main__":
    main()
