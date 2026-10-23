#!/usr/bin/env python3
"""Build exact published server MAIN and web-next on the temporary Netcup runner."""
import argparse
import base64
from pathlib import Path
import re
import subprocess


def public_root(path):
    pem = path.read_text()
    der = base64.b64decode(''.join(x for x in pem.splitlines() if not x.startswith('---')), validate=True)
    if len(der) != 44 or der[:12].hex() != '302a300506032b6570032100':
        raise ValueError('official Ed25519 public PEM required')
    return base64.urlsafe_b64encode(der[12:]).decode().rstrip('=')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('commit')
    p.add_argument('--repo', required=True, type=Path)
    p.add_argument('--output', required=True, type=Path)
    p.add_argument('--artifacts', required=True, type=Path)
    p.add_argument('--runner', required=True, type=Path)
    a = p.parse_args()
    if not re.fullmatch('[0-9a-f]{40}', a.commit):
        p.error('full published MAIN commit required')
    subprocess.run(['git', '-C', str(a.repo), 'merge-base', '--is-ancestor', a.commit, 'origin/main'], check=True)
    node = subprocess.check_output(['git', '-C', str(a.repo), 'show', a.commit + ':.node-version'], text=True).strip()
    go_mod = subprocess.check_output(['git', '-C', str(a.repo), 'show', a.commit + ':server/go.mod'], text=True)
    go = re.search(r'^go (\d+\.\d+\.\d+)$', go_mod, re.M).group(1)
    if not re.fullmatch(r'\d+\.\d+\.\d+', node):
        p.error('exact repository Node version required')
    root = public_root(Path(__file__).resolve().parents[1] / 'deployment/hosted/root-public-key.pem')
    flags = ('-X portico.local/server/internal/hostedtrust.OfficialRoot=' + root
             + ' -X portico.local/server/internal/buildinfo.Version=0.1.0-review+' + a.commit[:8]
             + ' -X portico.local/server/internal/buildinfo.BuildID=' + a.commit)
    a.artifacts.mkdir(parents=True, exist_ok=True)
    common = ['python3', str(a.runner), '--host', 'netcup', '--repo', str(a.repo), '--commit', a.commit]
    build = ['bash', '-c', 'set -euo pipefail; export GOMAXPROCS=2 GOMEMLIMIT=1400MiB GOTOOLCHAIN="$1"; '
             'go version; go build -p=2 -tags release -trimpath -buildvcs=false -mod=readonly '
             '-ldflags "$2" -o "$PORTICO_RUNNER_OUTPUT/portico-server" ./cmd/server', 'review', 'go' + go, flags]
    subprocess.run(common + ['--lane', 'release-server', '--cwd', 'server', '--output', str(a.output / 'server'),
                             '--artifact', 'portico-server', '--artifact-output', str(a.artifacts), '--'] + build, check=True)
    remote_artifact = '/home/runner/work/artifacts/release-r40/' + a.commit + '-web'
    web_script = """set -euo pipefail
export NODE_OPTIONS=--max-old-space-size=1536
npm exec --yes --package="node@$1" -- bash -c 'set -euo pipefail; node --version; npm ci --no-audit --no-fund; npm run build --workspace portico-web-next -- --outDir "$PORTICO_RUNNER_OUTPUT/web" --emptyOutDir'
test ! -e "$2"
mkdir -p "$2"
tar -cf "$2/web.tar" -C "$PORTICO_RUNNER_OUTPUT/web" .
"""
    web = ['bash', '-c', web_script, 'review', node, remote_artifact]
    subprocess.run(common + ['--lane', 'release-web', '--cwd', '.', '--output', str(a.output / 'web'), '--'] + web, check=True)
    target = a.artifacts / 'web.tar'
    if target.exists():
        raise FileExistsError('existing web artifact retained; choose a new artifact directory')
    try:
        with target.open('xb') as out:
            subprocess.run(['ssh', '-o', 'BatchMode=yes', 'portico-runner-g12', 'cat ' + remote_artifact + '/web.tar'], stdout=out, check=True)
        if not 0 < target.stat().st_size < 100 * 1024**2:
            raise ValueError('unexpected built web package size')
    finally:
        subprocess.run(['ssh', '-o', 'BatchMode=yes', 'portico-runner-g12',
                        'rm -f ' + remote_artifact + '/web.tar; rmdir ' + remote_artifact], check=True)
    print('Built review artifacts:', a.artifacts)


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode)
