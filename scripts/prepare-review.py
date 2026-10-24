#!/usr/bin/env python3
"""Prepare or resume a fresh review stage; publish private notes only after its browser smoke."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import tempfile
import urllib.error
import urllib.request
import uuid


def validate_smoke(directory, commit):
    record = json.loads((directory / 'runner-receipt.json').read_text())
    if record.get('exitCode') != 0 or record.get('commit') != commit:
        raise RuntimeError('matching browser job did not pass')
    results = []
    for line in (directory / 'runner.log').read_text().splitlines():
        try:
            value = json.loads(line)
        except ValueError:
            continue
        if isinstance(value, dict) and value.get('result'):
            results.append(value)
    if len(results) != 1 or results[0].get('result') != 'FOUR_SMOKES_PASS' or results[0].get('commit') != commit:
        raise RuntimeError('four matching actual browser checks are required')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('commit')
    p.add_argument('--cycle', required=True)
    p.add_argument('--private-review', required=True, type=Path)
    p.add_argument('--smoke-output', required=True, type=Path)
    p.add_argument('--resume', action='store_true')
    p.add_argument('--conversion-mode', choices=('all', 'sound', 'never'), default='never',
                   help='explicit demo playback policy; campaign M0 uses all for its bounded transcode smoke')
    a = p.parse_args()
    if not re.fullmatch('[0-9a-f]{40}', a.commit) or not re.fullmatch('review-[a-z0-9-]{1,64}', a.cycle):
        p.error('exact review cutoff and safe cycle required')
    spec = importlib.util.spec_from_file_location('review_smoke', Path(__file__).with_name('smoke-review.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    credentials = module.read_credentials(a.private_review)
    oldtext = a.private_review.read_text()
    old = json.loads(oldtext.split('```json\n', 1)[1].split('\n```', 1)[0])
    state = dict(cycle=a.cycle, origin='https://demo.getportico.tv', serverCommit=a.commit, webCommit=a.commit,
                 **credentials, installationId=str(uuid.uuid4()), requestId=str(uuid.uuid4()))
    for key in ('apk', 'apkNativeCommit', 'apkBytes'):
        if key in old:
            state[key] = old[key]
    base, token = 'http://127.0.0.1:48141', ''
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def call(method, path, body=None, status=200, origin=False):
        headers = {'Content-Type': 'application/json', 'X-Portico-Installation-Id': state['installationId']}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        if origin:
            headers['Origin'] = base
        request = urllib.request.Request(base + path, json.dumps(body).encode() if body is not None else None, headers, method=method)
        try:
            response = opener.open(request, timeout=45)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw, code = response.read(), response.code
        if code != status:
            safe_path = re.sub(r'/profiles/[^/]+/select$', '/profiles/:id/select', path)
            raise RuntimeError(f'{method} {safe_path}: HTTP {code}')
        return json.loads(raw) if raw else None

    assert call('GET', '/portico-build.json')['commit'] == a.commit
    system = call('GET', '/v1/system')
    if a.resume:
        assert not system['setupRequired'] and system['signIn'] == 'local'
        signed = call('POST', '/v1/direct/sign-in', credentials)
        session = signed['session']
        token = session['accessToken']
        direct = call('GET', '/v1/direct')
        if session['viewer']['role'] == 'account':
            primary = next(profile for profile in direct['profiles'] if profile['primary'])
            assert not primary['pinRequired']
            session = call('POST', '/v1/direct/profiles/' + primary['id'] + '/select', {'installationId': state['installationId'], 'pin': ''})['session']
            token = session['accessToken']
        assert session['viewer']['role'] == 'owner'
        state['profiles'] = direct['profiles']
    else:
        assert system['setupRequired'] and system['signIn'] == 'portico'
        setup = call('POST', '/v1/setup/browser', {}, origin=True)['setupToken']
        owner = call('POST', '/v1/setup', dict(setupToken=setup, requestId=state['requestId'], name='Portico demo', authMode='local', interactive=True, **credentials), status=201)
        session = owner.get('session', owner)
        token = session['accessToken']
        ready = call('GET', '/v1/setup/state')
        call('POST', '/v1/setup/finish', {'revision': ready['revision']})
        state['profiles'] = call('POST', '/v1/direct/profiles', {'name': 'Review', 'art': 'mint'}, status=201)['profiles']
    state['session'] = session
    document = call('GET', '/v1/admin/console/settings')['data']
    values = document['requested']
    changed = document
    if values['conversionMode'] != a.conversion_mode or document['effective']['conversionMode'] != a.conversion_mode:
        values['conversionMode'] = a.conversion_mode
        changed = call('PATCH', '/v1/admin/console/settings', {'expectedRevision': document['revision'], 'idempotencyKey': str(uuid.uuid4()), 'values': values})['data']
    assert changed['requested']['conversionMode'] == changed['effective']['conversionMode'] == a.conversion_mode
    state['conversionMode'] = a.conversion_mode
    system = call('GET', '/v1/system')
    assert not system['setupRequired'] and system['signIn'] == 'local'
    state['serverId'] = system['id']
    print(f'Stage prepared: two profiles and effective conversionMode={a.conversion_mode}. Private notes unchanged.', flush=True)
    if input().strip() != 'publish-four-browser-passes':
        raise SystemExit('Private review publication cancelled; state preserved.')
    validate_smoke(a.smoke_output, a.commit)
    current = a.private_review.read_text()
    before, rest = current.split('```json\n', 1)
    _, after = rest.split('\n```', 1)
    output = before + '```json\n' + json.dumps(state, indent=2) + '\n```' + after + '\n\n## Previous runtime preserved before review cutover\n\n```json\n' + json.dumps(old, indent=2) + '\n```\n'
    fd, name = tempfile.mkstemp(prefix='.review-', dir=a.private_review.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'w') as stream:
            stream.write(output)
        os.replace(name, a.private_review)
    finally:
        if os.path.exists(name):
            os.unlink(name)
    print('Private review notes updated after four matching actual browser checks; mode0600.', flush=True)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('Review preparation failed:', str(error) if isinstance(error, RuntimeError) else type(error).__name__)
        raise SystemExit(1)
