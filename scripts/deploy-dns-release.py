#!/usr/bin/env python3
"""Install a release's portico-dns on one DNS node and restart the native unit.

The node must already be provisioned (scripts/provision-dns-node.sh): dnsdist,
BIND and the portico-dns systemd unit are in place, and the feed secret is on
the host. The release goes to /opt/portico-dns/releases/<build>; the
/opt/portico-dns/current link moves only after the binary reports the right
build, and moves back if the authenticated feed does not synchronize. The
current and the previous release are kept; older ones are removed.

  scripts/deploy-dns-release.py <release-dir> --node ns2   # then ns1
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]
# ssh aliases (~/.ssh/config, root, the shared Portico VPS key) and public addresses.
NODES = {"ns1": ("portico-dns-primary-1", "174.138.183.45"),
         "ns2": ("portico-dns-de-1", "188.68.32.46")}

PROGRAM = r'''
from pathlib import Path
import hashlib,os,subprocess,time
releases=Path('/opt/portico-dns/releases');root=releases/BUILD;binary=root/'portico-dns';upload=root/'portico-dns.upload'
# Uploaded beside the binary and renamed over it: a redeploy of the running
# build cannot overwrite the executable in place.
if hashlib.sha256(upload.read_bytes()).hexdigest()!=DIGEST:raise RuntimeError('Upload hash mismatch')
upload.chmod(0o755);os.replace(upload,binary)
if subprocess.run([str(binary),'version'],capture_output=True,text=True,check=True).stdout.strip()!=BUILD:
    raise RuntimeError('Binary reports a different build')
secret=Path('/etc/portico-dns/feed-secret')
if (root/'feed-secret').exists():
    secret.write_bytes((root/'feed-secret').read_bytes());(root/'feed-secret').unlink()
if not secret.is_file() or secret.stat().st_size==0:
    raise RuntimeError('No feed secret on this host; pass --feed-secret-file')
subprocess.run(['chown','portico-dns:portico-dns',str(secret)],check=True);secret.chmod(0o600)
link=Path('/opt/portico-dns/current')
previous=os.readlink(link) if link.is_symlink() else None
def point(target):
    temporary=link.with_name('current.new')
    if temporary.is_symlink():temporary.unlink()
    temporary.symlink_to(target);os.replace(temporary,link)
def ready():
    # The first authenticated feed sync after a restart can take over a minute.
    for attempt in range(180):
        if subprocess.run(['runuser','-u','portico-dns','--',str(link/'portico-dns'),'dns-readiness'],capture_output=True).returncode==0:
            return subprocess.run(['runuser','-u','portico-dns','--',str(link/'portico-dns'),'dns-healthcheck'],capture_output=True).returncode==0
        time.sleep(1)
    return False
point('releases/'+BUILD)
subprocess.run(['systemctl','restart','portico-dns'],check=True)
if not ready():
    if previous and previous!='releases/'+BUILD:
        point(previous);subprocess.run(['systemctl','restart','portico-dns'],check=False)
    raise RuntimeError('DNS address feed did not authenticate and synchronize; the previous release is back')
keep={BUILD,(previous or '').rsplit('/',1)[-1]}
for old in releases.iterdir():
    if old.name not in keep:subprocess.run(['rm','-rf','--',str(old)],check=True)
print(BUILD)
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("release", type=Path)
    parser.add_argument("--node", required=True, choices=sorted(NODES))
    # Optional: without it the secret already on the node (/etc/portico-dns/feed-secret)
    # is reused, so it never leaves the host.
    parser.add_argument("--feed-secret-file", type=Path)
    args = parser.parse_args()
    alias, public_ip = NODES[args.node]
    build = json.loads((args.release / "release.json").read_text())["buildId"]
    if not re.fullmatch(r"portico-[A-Za-z0-9-]+", build):
        raise SystemExit("Invalid release")
    if args.feed_secret_file is not None and args.feed_secret_file.stat().st_mode & 0o077:
        raise SystemExit("Feed secret must be private")
    binary = args.release / "portico-dns"
    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
    ssh = ["ssh", "-o", "BatchMode=yes", alias]
    remote = "/opt/portico-dns/releases/" + build
    subprocess.run(ssh + ["install -d -m 0755 " + remote], check=True)
    uploads = [(binary, "portico-dns.upload")]
    if args.feed_secret_file is not None:
        uploads.append((args.feed_secret_file, "feed-secret"))
    for source, name in uploads:
        subprocess.run(["scp", "-q", "-o", "BatchMode=yes", str(source), alias + ":" + remote + "/" + name], check=True)
    subprocess.run(ssh + ["python3 -"], input="BUILD=" + repr(build) + "\nDIGEST=" + repr(digest) + "\n" + PROGRAM,
                   text=True, check=True)
    # Public UDP and TCP from here, through dnsdist, as a resolver would ask.
    subprocess.run(["python3", str(ROOT / "hosted-services/dns/deploy/healthcheck.py"), public_ip], check=True)
    print(json.dumps({"node": args.node, "host": alias, "buildId": build, "feedOrigin": "https://web.getportico.tv",
                      "authenticatedFeedSync": "passed", "publicUDPAndTCP": "passed"}))


if __name__ == "__main__":
    main()
