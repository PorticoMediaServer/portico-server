#!/usr/bin/env python3
"""Run the four real demo browser checks through an ephemeral private stdin channel."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import signal
import stat
import subprocess
import sys
import time


def stdin_ready(text):
    return 'PRIVATE_STDIN_READY' in text.splitlines()


def browser_ready(text):
    return 'BROWSER_ADMITTED' in text.splitlines()


def load_deployment():
    spec = importlib.util.spec_from_file_location('review_deploy', Path(__file__).with_name('deploy-review.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ReviewActivation:
    def __init__(self, commit, cycle, attempt):
        self.commit, self.cycle, self.attempt, self.started = commit, cycle, attempt, False
        self.backup = cycle + ('-' + attempt if attempt else '')

    def start(self):
        if self.started:
            raise RuntimeError('review activation already started')
        deployment = load_deployment()
        deployment.remote(deployment.ACTIVATE, '/opt/portico/demo-releases/' + self.cycle,
                          '/var/lib/portico-' + self.cycle, '/etc/portico-' + self.cycle + '.env',
                          self.commit, self.backup)
        self.started = True

    def rollback(self):
        if self.started:
            deployment = load_deployment()
            deployment.remote(deployment.ROLLBACK, self.backup)


def read_credentials(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd) as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.getuid():
            raise ValueError('private review credentials require an owned regular file with mode0600')
        document = stream.read()
    state = json.loads(document if document.lstrip().startswith('{') else document.split('```json\n', 1)[1].split('\n```', 1)[0])
    return {'username': state['username'], 'password': state['password']}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('commit')
    p.add_argument('--repo', required=True, type=Path)
    p.add_argument('--runner', required=True, type=Path)
    p.add_argument('--credentials', required=True, type=Path)
    p.add_argument('--output', required=True, type=Path)
    p.add_argument('--host', choices=['netcup', 'contabo'], default='netcup')
    p.add_argument('--activate-cycle')
    p.add_argument('--attempt')
    a = p.parse_args()
    if not re.fullmatch('[0-9a-f]{40}', a.commit):
        p.error('exact MAIN source cutoff required')
    if a.activate_cycle and not re.fullmatch('review-[a-z0-9-]{1,64}', a.activate_cycle):
        p.error('safe review activation cycle required')
    if a.attempt and (not a.activate_cycle or not re.fullmatch('[a-z0-9-]{1,24}', a.attempt)):
        p.error('safe activation retry name required')
    subprocess.run(['git', '-C', str(a.repo), 'merge-base', '--is-ancestor', a.commit, 'origin/main'], check=True)
    read_credentials(a.credentials)  # Refuse unsafe custody before starting a browser.
    tools_repo = Path(__file__).resolve().parents[1]
    tools_commit = subprocess.check_output(['git', '-C', str(tools_repo), 'rev-parse', 'HEAD'], text=True).strip()
    script = subprocess.check_output(['git', '-C', str(tools_repo), 'show', tools_commit + ':scripts/review-smoke.mjs'], text=True)
    command = ['bash', '-c', 'set -eu; (cd "$PORTICO_RUNNER_SECONDARY_SOURCE/scripts" && python3 -m unittest -v test_deploy_review.py && node --check review-smoke.mjs && node --test test_review_smoke_privacy.mjs); export PORTICO_REVIEW_ORIGIN="$2" PORTICO_REVIEW_COMMIT="$3" PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH="$4" PORTICO_REVIEW_AWAIT_ACTIVATION="$5"; npm install --prefix "$TMPDIR/browser" --no-save --package-lock=false --no-audit --no-fund playwright@1.58.2 >/dev/null; node --input-type=module -e "$1"', 'release-smoke', script, 'https://demo.getportico.tv', a.commit, '/usr/bin/chromium' if a.host == 'netcup' else '/snap/bin/chromium', '1' if a.activate_cycle else '']
    args = ['python3', str(a.runner), '--host', a.host, '--browser', '--repo', str(a.repo), '--commit', a.commit, '--secondary-repo', str(tools_repo), '--secondary-commit', tools_commit, '--lane', 'release-signin-smoke', '--cwd', '.', '--output', str(a.output), '--collect', 'final-page.png', '--collect', 'failure-dom.log', '--'] + command
    if (a.output / 'runner.log').exists():
        raise FileExistsError('retain existing browser evidence; use a new cycle output')
    child = subprocess.Popen(args, stdin=subprocess.PIPE, start_new_session=True)
    sent = False
    passed = False
    activation = ReviewActivation(a.commit, a.activate_cycle, a.attempt) if a.activate_cycle else None
    deadline = time.monotonic() + 300
    try:
        while child.poll() is None:
            log = a.output / 'runner.log'
            text = log.read_text() if log.exists() else ''
            if activation and not activation.started and browser_ready(text):
                activation.start()
                child.stdin.write(b'PUBLIC_ACTIVATED\n')
                child.stdin.flush()
                print('Admitted browser ready; public review activation complete.', flush=True)
            if not sent and stdin_ready(text):
                payload = json.dumps(read_credentials(a.credentials)).encode() + b'\n'
                child.stdin.write(payload)
                child.stdin.close()
                payload = b''
                sent = True
            if time.monotonic() > deadline:
                raise TimeoutError('bounded browser smoke did not complete')
            time.sleep(.25)
        code = child.wait()
        text = (a.output / 'runner.log').read_text() if (a.output / 'runner.log').exists() else ''
        for line in text.splitlines():
            if line.startswith('PASS ') or line.startswith('{"result"') or line.startswith('{"diagnosis"'):
                print(line)
        if code == 0 and not any(line.startswith('{"result":"FOUR_SMOKES_PASS"') for line in text.splitlines()):
            code = 1
        passed = code == 0
        return code
    finally:
        if child.poll() is None:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=30)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
        if child.stdin and not child.stdin.closed:
            child.stdin.close()
        if activation and activation.started and not passed:
            activation.rollback()


if __name__ == '__main__':
    raise SystemExit(main())
