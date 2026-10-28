#!/usr/bin/env python3
"""Claim the fresh test server through normal authenticated claim APIs. No secrets printed."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import time
import urllib.request
import urllib.error


def private(path):
    st = path.lstat()
    if path.is_symlink() or not path.is_file() or st.st_uid != os.getuid() or st.st_mode & 0o777 != 0o600:
        raise SystemExit('Private input must be an operator-owned regular 0600 file')
    return json.loads(path.read_text())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fixture-source', type=Path, required=True)
    parser.add_argument('--credentials', type=Path, required=True)
    parser.add_argument('--hosted-session', type=Path, required=True)
    parser.add_argument('--hosted-api', default='http://127.0.0.1:49410')
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--execute', action='store_true')
    args = parser.parse_args()
    if not args.execute:
        print('PLAN: sign in local owner, prepare bound claim, approve with fresh Hosted session, continue until installed+acknowledged; no network')
        return
    if 'CI' in os.environ or 'GITHUB_ACTIONS' in os.environ:
        parser.error('Claim setup is prohibited in CI')
    hosted = private(args.hosted_session)
    if args.hosted_api not in ('http://127.0.0.1:49410', hosted['base']):
        parser.error('Hosted API must be the private tunnel or its captured HTTPS origin')
    spec = importlib.util.spec_from_file_location('fixture_seed', args.fixture_source / 'fixtures/test-library/seed_state.py')
    seed = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(seed)
    cfg = private(args.credentials)
    client = seed.Client('http://127.0.0.1:49300')
    signed, client.token = seed.sign_in(client, cfg['owner']['username'], cfg['owner']['password'])
    primary = next(p for p in signed['profiles'] if p.get('primary'))
    client.token = seed.select_profile(client, primary['id'])
    session = hosted['session']
    account = session['account']['id']
    def central(path, body):
        req = urllib.request.Request(args.hosted_api + path, json.dumps(body, separators=(',', ':')).encode(),
              {'Content-Type': 'application/json', 'Accept': 'application/json', 'Authorization': 'Bearer ' + session['accessToken']}, method='POST')
        try:
            response = urllib.request.urlopen(req, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = json.loads(response.read())
            if response.status != 200:
                code = data.get('error', {}).get('code', 'unknown')
                raise SystemExit('Hosted claim approval refused: HTTP %d %s' % (response.status, code))
            return data
    state = client.read('/v1/networking/claim')
    server_id = state['identity']['serverId']
    if 'prepare' in state['actions']:
        state = client.call('POST', '/v1/networking/claim/prepare', {'accountId': account, 'expected': state['identity']})
    if state.get('accountId') != account:
        raise SystemExit('Existing claim names another account; never replace it implicitly')
    if state.get('approvalRequired'):
        request = state['approvalRequest']
        fields = ['operationId', 'serverId', 'publicKey', 'localGeneration', 'name', 'expectedRevision',
                  'purpose', 'currentOwnerAccountId', 'previousOwnerRole', 'returnRegistrationId']
        envelope = central('/v1/server-claims/approve', {key: request[key] for key in fields})
        state = client.call('POST', '/v1/networking/claim/approve', {'expected': state['operation'],
                            'approvalEnvelope': json.dumps(envelope, separators=(',', ':'))})
    for _ in range(30):
        if state['state'] == 'installed' and state.get('installationAcknowledged'):
            break
        if 'continue' not in state['actions']:
            raise SystemExit('Claim cannot continue in its actual state')
        try:
            state = client.call('POST', '/v1/networking/claim/continue', {'expected': state['operation']})
        except seed.Refusal as error:
            if error.code != 'claim_unavailable':
                raise
            time.sleep(2)
            state = client.read('/v1/networking/claim')
        time.sleep(1)
    check = client.read('/v1/networking/claim')
    passed = check['identity']['serverId'] == server_id and check.get('accountId') == account and check['state'] == 'installed' and check.get('installationAcknowledged') is True
    args.report.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(args.report, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, 'w') as out:
        json.dump({'installed': passed, 'state': check['state'], 'installationAcknowledged': check.get('installationAcknowledged'),
                   'serverId': server_id, 'verifiedBy': hosted.get('verifiedBy', 'existing-normal-session')}, out, indent=2)
    print('Claim installed and acknowledged:', passed)
    if not passed:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
