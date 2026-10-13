#!/usr/bin/env python3
"""Isolated Hosted/server acceptance. No production credentials or providers.

Requires Go, PostgreSQL 17/18 binaries, psql/createdb, and Python cryptography.
Run from any directory with --server-tree pointing at the read-only server tree
and --scratch at the programme's scratch/e2e-hosted directory. Each run retains
private logs/state there and stops only processes/cluster it starts. Ports are
allocated dynamically, all listeners are numeric loopback. No installation.
"""
import argparse
import base64
import hashlib
import http.client
import http.cookies
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import sqlite3
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.parse
import uuid

from cryptography.hazmat.primitives import serialization, hashes
from cryptography.hazmat.primitives.asymmetric import x25519
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF


def b64(raw):
    return base64.urlsafe_b64encode(raw).decode().rstrip('=')


def unb64(raw):
    return base64.urlsafe_b64decode(raw + '=' * (-len(raw) % 4))


def random_id():
    return b64(secrets.token_bytes(32))


def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


class Client:
    def __init__(self, origin, ca=None):
        u = urllib.parse.urlsplit(origin)
        assert u.hostname == '127.0.0.1' and u.scheme in ('http', 'https')
        self.origin, self.u, self.ca = origin, u, ca
        self.cookies = {}

    def request(self, method, path, body=None, token=None, browser=False,
                installation=None, expected=200, cookies=True, headers=None):
        assert path.startswith('/') and not path.startswith('//')
        h = {'Content-Type': 'application/json'}
        if token:
            h['Authorization'] = 'Bearer ' + token
        if browser:
            h['Origin'] = self.origin
            if cookies:
                h['Cookie'] = '; '.join(k + '=' + v for k, v in self.cookies.items())
                for k, v in self.cookies.items():
                    if k.endswith('_csrf'):
                        h['X-CSRF-Token'] = v
        if installation:
            h['X-Portico-Installation-Id'] = installation
        h.update(headers or {})
        connection = (http.client.HTTPSConnection(self.u.hostname, self.u.port,
                      context=ssl.create_default_context(cafile=str(self.ca)), timeout=20)
                      if self.u.scheme == 'https' else
                      http.client.HTTPConnection(self.u.hostname, self.u.port, timeout=20))
        try:
            data = None if body is None else json.dumps(body, separators=(',', ':')).encode()
            connection.request(method, path, data, h)
            response = connection.getresponse()
            raw = response.read(2 << 20)
            rh = dict(response.getheaders())
            for k, v in response.getheaders():
                if k.lower() == 'set-cookie' and cookies:
                    parsed = http.cookies.SimpleCookie(v)
                    self.cookies.update({k: m.value for k, m in parsed.items()})
            try:
                out = json.loads(raw) if raw.strip() else None
            except ValueError:
                out = raw.decode()
            if expected is not None and response.status != expected:
                # Never print successful token/envelope bodies or cookie headers.
                error = out.get('error', {}) if isinstance(out, dict) else {}
                raise AssertionError(f'{method} {path.split("?")[0]}: {response.status}, expected {expected}; {error}')
            return out, rh, response.status
        finally:
            connection.close()


class Run:
    def __init__(self, args):
        self.args = args
        self.module = Path(__file__).resolve().parents[2]
        self.scratch = Path(args.scratch).resolve()
        self.scratch.mkdir(parents=True, exist_ok=True, mode=0o700)
        self.dir = Path(tempfile.mkdtemp(prefix='run-', dir=self.scratch))
        self.env = {k: v for k, v in os.environ.items() if not k.startswith('PORTICO_')}
        self.env['GOFLAGS'] = '-p=2'
        self.processes, self.files, self.results = [], [], []
        self.pgstarted = False
        self.socketdir = Path(tempfile.mkdtemp(prefix='portico-e2e-socket-'))
        self.pg = Path(args.pg_bin)
        self.pgport = port()
        self.api = Client('http://127.0.0.1:' + str(port()))
        self.tls = Client('https://127.0.0.1:' + str(port()), self.dir / 'tls/cert.pem')
        self.server = Client('http://127.0.0.1:' + str(port()))
        self.dbname = 'portico_hosted_e2e_test'
        self.password = random_id() + '-Aa1!'
        self.write('credentials.json', {'password': self.password})
        # A refusing proxy prevents ambient HTTP provider configuration from making
        # an outbound request. Clients in this driver never consult proxy env vars.
        class Refuse(http.server.BaseHTTPRequestHandler):
            def do_CONNECT(inner):
                self.results.append({'unexpectedEgress': inner.path})
                inner.send_error(502)
            do_GET = do_POST = do_CONNECT
            def log_message(*unused):
                pass
        self.proxy = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Refuse)
        threading.Thread(target=self.proxy.serve_forever, daemon=True).start()
        proxy = 'http://127.0.0.1:' + str(self.proxy.server_port)
        self.env.update(HTTP_PROXY=proxy, HTTPS_PROXY=proxy, NO_PROXY='127.0.0.1,::1,localhost')

    def write(self, name, value):
        p = self.dir / name
        p.write_text(json.dumps(value, indent=2) + '\n')
        p.chmod(0o600)

    def command(self, args, cwd=None, input=None):
        result = subprocess.run([str(x) for x in args], cwd=cwd or self.module,
                                env=self.env, capture_output=True, text=True, input=input)
        with (self.dir / 'commands.log').open('a') as f:
            # Some commands take the generated password: log output only, not argv.
            f.write(result.stdout + result.stderr)
        if result.returncode:
            raise RuntimeError(f'{Path(str(args[0])).name} failed; see private commands.log')
        return result.stdout.strip()

    def start(self, args, name, env=None):
        log = (self.dir / (name + '.log')).open('ab')
        self.files.append(log)
        p = subprocess.Popen([str(x) for x in args], cwd=self.module,
                             env=env or self.env, stdout=log, stderr=log)
        self.processes.append(p)
        return p

    def stop(self, p):
        if p.poll() is None:
            p.terminate()
            try:
                p.wait(timeout=15)
            except subprocess.TimeoutExpired:
                p.kill()
                p.wait(timeout=5)

    def wait(self, client, path, seconds=40):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            try:
                client.request('GET', path)
                return
            except (OSError, AssertionError, http.client.HTTPException):
                time.sleep(.2)
        raise RuntimeError(f'listener did not become ready: {path}; see private logs')

    def passed(self, name, **details):
        self.results.append({'flow': name, 'status': 'passed', **details})
        self.write('results.json', self.results)
        print('PASS ' + name, flush=True)

    def sql(self, statement):
        return self.command(['psql', '-X', '-h', self.socketdir, '-p', self.pgport,
                             '-d', self.dbname, '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-c', statement])

    def sqlite(self, statement, params=()):
        with sqlite3.connect(self.dir / 'server-e2e/server.sqlite', timeout=10) as db:
            return db.execute(statement, params).fetchall()

    def hosted(self, keyset):
        env = dict(self.env, PORTICO_DEV_LOOPBACK_ROUTE='1')
        return self.start([self.scratch / 'hosted', '-listen', self.api.u.netloc,
            '-metrics-listen', '127.0.0.1:' + str(port()),
            '-database-url-file', self.dir / 'database-url',
            '-root-public-key-file', self.dir / 'root-public.pem',
            '-key-file', self.dir / keyset / 'signing.json', '-origins', self.api.origin,
            '-oidc-config-file', self.dir / 'account-origin.json',
            '-claim-audience', self.tls.origin, '-tv-issuer', self.api.origin,
            '-device-verification-uri', self.api.origin + '/device', '-allow-loopback-enrollment'],
            'hosted-' + keyset, env)

    def challenge(self, installation):
        nonce = random_id()
        out, _, _ = self.server.request('POST', '/v1/direct/portico-challenge',
                    {'nonce': nonce}, installation=installation)
        # Verify the actual server challenge signature against its claimed key.
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
        public = unb64(self.approval['publicKey'])
        Ed25519PublicKey.from_public_bytes(public).verify(unb64(out['proof']['signature']),
                                                         unb64(out['proof']['payload']))
        proof = json.loads(unb64(out['proof']['payload']))
        assert proof['challenge'] == out['challenge']
        assert proof['nonce'] == nonce and proof['installationId'] == installation
        assert proof['serverId'] == self.server_id and proof['publicKey'] == self.approval['publicKey']
        return out['challenge']

    def sign_in(self, hosted_token, installation):
        challenge = self.challenge(installation)
        signed, _, _ = self.api.request('POST', '/v1/servers/' + self.server_id + '/identity',
                                      {'challenge': challenge}, token=hosted_token)
        answer, _, _ = self.server.request('POST', '/v1/direct/sign-in',
                    {'porticoIdentity': signed}, installation=installation)
        assert answer.get('session'), 'server requires a factor/profile before admitting the account'
        token = answer['session']['accessToken']
        self.server.request('GET', '/v1/me', token=token)
        return answer, signed

    def exercise(self):
        if not self.args.skip_build:
            for name, target in [('hosted', './cmd/hosted'), ('localtls', './dev/localtls'),
                                 ('e2eaccounts', './dev/e2eaccounts'), ('e2ewake', './dev/e2ehosted/wake')]:
                self.command(['go', 'build', '-tags', 'devtrust', '-o', self.scratch / name, target])
            self.command(['go', 'build', '-tags', 'devtrust', '-o', self.scratch / 'server', './cmd/server'],
                         Path(self.args.server_tree) / 'server')
        self.command([self.pg / 'initdb', '-D', self.dir / 'postgres', '--auth-local=trust', '--auth-host=reject'])
        self.command([self.pg / 'pg_ctl', '-D', self.dir / 'postgres', '-l', self.dir / 'postgres.log',
                      '-o', f"-k {self.socketdir} -p {self.pgport} -c listen_addresses=''", '-w', 'start'])
        self.pgstarted = True
        self.command(['createdb', '-h', self.socketdir, '-p', self.pgport, self.dbname])
        p = self.dir / 'database-url'
        p.write_text(f'postgres:///{self.dbname}?host={self.socketdir}&port={self.pgport}&sslmode=disable\n')
        p.chmod(0o600)
        self.command([self.scratch / 'hosted', 'keys', 'setup', '--private-out', self.dir / 'root-private.pem',
                      '--public-out', self.dir / 'root-public.pem', '--staging', self.dir / 'key-a'])
        self.write('account-origin.json', {'webOrigin': self.api.origin})
        self.hp = self.hosted('key-a')
        self.wait(self.api, '/ready')
        assert self.sql('SELECT count(*) FROM accounts') == '0'
        self.passed('Hosted starts on an empty database', schema=self.sql('SELECT version FROM hosted_schema_contract WHERE singleton'))
        self.command([self.scratch / 'e2eaccounts', '-database-url-file', p,
                      '-password', self.password, 'alice'])
        self.account, _, _ = self.api.request('POST', '/v1/sessions',
                         {'username': 'alice', 'password': self.password, 'sessionMode': 'native', 'requestId': random_id()}, headers={'X-Portico-Continuation': random_id()})
        self.account_token = self.account['accessToken']
        assert self.sql('SELECT count(*) FROM accounts') == '1'
        self.passed('Test account creation and password sign-in')
        (self.dir / 'tls').mkdir()
        self.lp = self.start([self.scratch / 'localtls', '-dir', self.dir / 'tls',
                             '-listen', self.tls.u.netloc, '-upstream', self.api.origin], 'localtls')
        self.wait(self.tls, '/health')
        public = serialization.load_pem_public_key((self.dir / 'root-public.pem').read_bytes()).public_bytes(
            serialization.Encoding.Raw, serialization.PublicFormat.Raw)
        root_id = hashlib.sha256(public).digest()[:16].hex()
        env = dict(self.env, PORTICO_STATE_DIR=str(self.dir / 'server-e2e'),
                   PORTICO_BIND=self.server.u.netloc, PORTICO_DISCOVERY='0',
                   PORTICO_HOSTED_ORIGIN=self.tls.origin, PORTICO_HOSTED_PUBLIC_KEY=b64(public),
                   PORTICO_HOSTED_KEY_ID=root_id, PORTICO_DEV_HOSTED_CA_FILE=str(self.dir / 'tls/cert.pem'))
        self.sp = self.start([self.scratch / 'server'], 'server', env)
        self.wait(self.server, '/v1/system')
        direct, _, _ = self.server.request('POST', '/v1/dev/e2e/sign-in', {'username': 'e2e-owner'})
        self.direct = direct['session']['accessToken']
        # Let the unclaimed server's background loops become quiet before claim.
        time.sleep(1)
        state, _, _ = self.server.request('GET', '/v1/networking/claim', token=self.direct)
        state, _, _ = self.server.request('POST', '/v1/networking/claim/prepare',
                         {'accountId': self.account['account']['id'], 'expected': state['identity']}, token=self.direct)
        raw_approval = state['approvalRequest']
        self.approval = {k: raw_approval[k] for k in ['operationId', 'serverId', 'publicKey', 'localGeneration', 'name']}
        self.approval['ownerProfileId'] = self.account['profiles'][0]['id']
        self.approval['expectedRevision'] = raw_approval['expectedRevision']
        signed, _, _ = self.api.request('POST', '/v1/server-claims/approve', self.approval, token=self.account_token)
        state, _, _ = self.server.request('POST', '/v1/networking/claim/approve',
                    {'expected': state['operation'], 'approvalEnvelope': json.dumps(signed, separators=(',', ':'))}, token=self.direct)
        for _ in range(12):
            if state['state'] == 'installed' and state['installationAcknowledged']:
                break
            state, _, _ = self.server.request('POST', '/v1/networking/claim/continue',
                                             {'expected': state['operation']}, token=self.direct)
        assert state['state'] == 'installed' and state['installationAcknowledged']
        self.server_id = state['identity']['serverId']
        linked = lambda: self.sqlite('SELECT count(*) FROM account_portico_links WHERE hosted_account_id=?',
                                     (self.account['account']['id'],))[0][0] == 1
        try:
            self.until(linked, seconds=3)
        except AssertionError:
            installation = random_id()
            assertion, _, _ = self.api.request('POST', '/v1/servers/' + self.server_id + '/identity',
                                 {'challenge': self.challenge(installation)}, token=self.account_token)
            refused, _, status = self.server.request('POST', '/v1/direct/sign-in',
                                {'porticoIdentity': assertion}, installation=installation, expected=None)
            if status == 403 and refused.get('error', {}).get('code') == 'access_refused':
                self.results.append({'flow': 'Server claim owner admission', 'status': 'gap',
                    'needed': 'Wake Hosted control after claim installation/acknowledgment so the owner link and first check-in are processed promptly',
                    'signInStatusBeforeWake': status})
                self.write('results.json', self.results)
                print('GAP: installed claim owner is refused until Hosted control is woken', flush=True)
            else:
                assert status == 200, 'unexpected initial claim admission refusal'
            # Continue remaining acceptance using the injected signed-wake seam.
            self.wake('key-a')
            self.until(linked)
        self.write('sessions.json', {'direct': direct, 'hosted': self.account, 'claim': state})
        self.passed('Server claim is installed and acknowledged')
        listed, _, _ = self.api.request('GET', '/v1/servers', token=self.account_token)
        assert any(v['id'] == self.server_id for v in listed['items'])
        self.passed('Directory lists the claimed server')
        self.session_a, self.assertion_a = self.sign_in(self.account_token, random_id())
        self.passed('Hosted identity assertion admits a local server session')
        self.third_party()
        self.tv()
        self.outage()
        self.rotate()
        self.rate_limits()
        assert not any('unexpectedEgress' in v for v in self.results), 'an external HTTP request was attempted'
        self.passed('No provider HTTP requests attempted')

    def third_party(self):
        self.api.request('POST', '/v1/sessions', {'username': 'alice', 'password': self.password,
                         'sessionMode': 'browser', 'requestId': random_id()}, browser=True)
        installation, verifier, state = random_id(), random_id(), random_id()
        callback = {}
        class Return(http.server.BaseHTTPRequestHandler):
            def do_GET(inner):
                callback.update(urllib.parse.parse_qs(urllib.parse.urlsplit(inner.path).query))
                inner.send_response(200)
                inner.end_headers()
                inner.wfile.write(b'Signed in.')
            def log_message(*unused):
                pass
        listener = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Return)
        threading.Thread(target=listener.serve_forever, daemon=True).start()
        try:
            redirect = f'http://127.0.0.1:{listener.server_port}/return?existing=kept'
            q = dict(serverId=self.server_id, challenge=self.challenge(installation),
                     redirectUri=redirect, state=state, appName='Local integration client',
                     codeChallenge=b64(hashlib.sha256(verifier.encode()).digest()))
            page, headers, _ = self.api.request('GET', '/auth/apps/authorize?' + urllib.parse.urlencode(q))
            assert '<section id="consent" hidden>' in page and "frame-ancestors 'none'" in headers['Content-Security-Policy']
            # Drive the exact page endpoints with its canonical cookie and CSRF.
            self.api.request('POST', '/v1/app-authorizations', q, browser=True,
                             expected=403, headers={'X-CSRF-Token': ''})
            review, _, _ = self.api.request('POST', '/v1/app-authorizations', q, browser=True, expected=201)
            assert review['redirectUri'] == redirect and review['accountId'] == self.account['account']['id']
            result, _, _ = self.api.request('POST', '/v1/app-authorizations/' + review['id'] + '/decision',
                                 {'accountId': review['accountId'], 'approve': True}, browser=True)
            u = urllib.parse.urlsplit(result['location'])
            assert f'{u.scheme}://{u.netloc}{u.path}' == redirect.split('?')[0]
            Client(f'{u.scheme}://{u.netloc}').request('GET', u.path + '?' + u.query)
            assert callback['state'] == [state] and callback['existing'] == ['kept'] and 'error' not in callback
            exchange = dict(code=callback['code'][0], verifier=verifier, redirectUri=redirect)
            self.api.request('POST', '/v1/app-authorizations/token', dict(exchange, verifier=random_id()), expected=400)
            grant, _, _ = self.api.request('POST', '/v1/app-authorizations/token', exchange)
            assert set(grant) == {'assertion'}
            self.api.request('POST', '/v1/app-authorizations/token', exchange, expected=400)
            answer, _, _ = self.server.request('POST', '/v1/direct/sign-in',
                      {'porticoIdentity': grant['assertion']}, installation=installation)
            self.server.request('GET', '/v1/me', token=answer['session']['accessToken'])
            self.app_session = answer
            self.passed('Third-party consent, loopback callback, PKCE and server admission',
                        csrf='refused', wrongVerifier='refused', replay='refused')
        finally:
            listener.shutdown()
            listener.server_close()

    def tv(self):
        key = x25519.X25519PrivateKey.generate()
        public = b64(key.public_key().public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw))
        out, _, _ = self.api.request('POST', '/v1/device-authorizations',
                       dict(sessionMode='native', protocol='tv-x25519-v1', publicKey=public,
                            requestId=str(uuid.uuid4()), deviceName='Integration TV', platform='tvOS', appVersion='1'), expected=201)
        _, early_headers, _ = self.api.request('POST', '/v1/tv-authorizations/token', {'deviceCode': out['deviceCode']}, expected=429)
        review, _, _ = self.api.request('POST', '/v1/device-authorizations/review',
                                     {'userCode': out['userCode']}, token=self.account_token)
        self.api.request('POST', '/v1/device-authorizations/decision',
                       {'requestId': review['requestId'], 'userCode': out['userCode'], 'decision': 'approve'},
                       token=self.account_token, expected=204)
        time.sleep(max(out['interval'], int(early_headers.get('Retry-After', '0'))))
        grant, _, _ = self.api.request('POST', '/v1/tv-authorizations/token', {'deviceCode': out['deviceCode']})
        assert grant['issuer'] == self.api.origin and grant['recipientKey'] == public
        assert grant['requestId'] == review['clientRequestId']
        shared = key.exchange(x25519.X25519PublicKey.from_public_bytes(unb64(grant['publicKey'])))
        info = '\0'.join(['portico.tv-grant.v1', grant['issuer'], grant['requestId'], public, grant['publicKey']]).encode()
        secret = HKDF(algorithm=hashes.SHA256(), length=32, salt=unb64(grant['salt']), info=info).derive(shared)
        plain = AESGCM(secret).decrypt(unb64(grant['nonce']), unb64(grant['ciphertext']), info)
        pair = json.loads(plain)
        assert pair['account']['id'] == self.account['account']['id']
        self.tv_server_session, _ = self.sign_in(pair['accessToken'], random_id())
        self.tv_session = pair
        self.passed('TV code review, approval, encrypted grant and server sign-in')

    def outage(self):
        self.stop(self.hp)
        self.stop(self.lp)
        self.server.request('GET', '/v1/me', token=self.session_a['session']['accessToken'])
        self.server.request('GET', '/v1/me', token=self.direct)
        self.server.request('GET', '/v1/system')
        self.sql('UPDATE servers SET policy_revision=policy_revision+1')
        self.wake('key-a')
        self.until(lambda: self.sqlite('SELECT attempts FROM networking_control_schedule')[0][0] > 0)
        before = self.sqlite('SELECT attempts,retry_at FROM networking_control_schedule')[0]
        assert before[1] > int(time.time()*1000)
        self.wake('key-a')
        self.wake('key-a')
        time.sleep(.3)
        assert self.sqlite('SELECT attempts,retry_at FROM networking_control_schedule')[0] == before
        client_result = self.command(['node', '--experimental-strip-types', self.module / 'dev/e2ehosted/outage.mjs'],
                     input=json.dumps({'hosted': self.api.origin, 'server': self.server.origin,
                                       'serverTree': self.args.server_tree, 'token': self.session_a['session']['accessToken']}))
        client_result = json.loads(client_result)
        self.passed('Hosted outage preserves admitted and direct sessions; shared client gate blocks automatic requests')
        if client_result['devicePollBypassesGate']:
            self.results.append({'flow': 'Client device activation outage gate', 'status': 'gap',
                                 'bug': 'B-22.26', 'networkRequestsWhileGateOpen': client_result['devicePollRequests'],
                                 'needed': 'HttpDeviceAuthorizationApi.poll must use hostedGate(baseUrl).run(automatic) and the scheduler must honor its deadline'})
            self.write('results.json', self.results)
            print('GAP B-22.26: device activation adapter bypasses the shared outage gate', flush=True)
        else:
            self.passed('Client device activation adapter honors shared outage gate')
        self.lp = self.start([self.scratch / 'localtls', '-dir', self.dir / 'tls',
                             '-listen', self.tls.u.netloc, '-upstream', self.api.origin], 'localtls-restart')
        self.hp = self.hosted('key-a')
        self.wait(self.api, '/ready')
        self.wait(self.tls, '/health')

    def wake(self, keyset):
        self.command([self.scratch / 'e2ewake', '-database-url-file', self.dir / 'database-url',
                      '-root-public-key-file', self.dir / 'root-public.pem',
                      '-key-file', self.dir / keyset / 'signing.json',
                      '-server-origin', self.server.origin, '-server-id', self.server_id])

    def until(self, check, seconds=30):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if check():
                return
            time.sleep(.1)
        raise AssertionError('runtime readback did not converge before its deadline')

    def rotate(self):
        self.stop(self.hp)
        self.command([self.scratch / 'hosted', 'keys', 'renew', '--root', self.dir / 'root-private.pem',
                      '--current-certificate', self.dir / 'key-a/signing.json.certificate.json',
                      '--staging', self.dir / 'key-b'])
        shutil.copyfile(self.dir / 'key-a/signing.json.seal.json', self.dir / 'key-b/signing.json.seal.json')
        (self.dir / 'key-b/signing.json.seal.json').chmod(0o600)
        self.hp = self.hosted('key-b')
        self.wait(self.api, '/ready')
        self.until(lambda: self.sql('SELECT pending FROM hosted_signing_state WHERE singleton') == 'f')
        self.wake('key-b')
        self.until(lambda: self.sqlite('SELECT revision FROM hosted_trust_state WHERE singleton=1')[0][0] == 2)
        self.server.request('GET', '/v1/me', token=self.session_a['session']['accessToken'])
        session_b, _ = self.sign_in(self.account_token, random_id())
        self.passed('Signing rotation reaches server; previous and new sessions work')
        self.stop(self.hp)
        (self.dir / 'key-c').mkdir()
        a_id = json.loads((self.dir / 'key-a/signing.json.certificate.json').read_text())['keyId']
        self.command([self.scratch / 'hosted', 'keys', 'rotate', '--out', self.dir / 'key-c/signing.json',
                      '--public-out', self.dir / 'key-c/signing-public.pem',
                      '--seal-from', self.dir / 'key-a/signing.json.seal.json'])
        self.command([self.scratch / 'hosted', 'keys', 'certify', '--root', self.dir / 'root-private.pem',
                      '--public', self.dir / 'key-c/signing-public.pem', '--out', self.dir / 'key-c/signing.json.certificate.json',
                      '--revision', '3', '--revoked', a_id, '--days', '30'])
        self.hp = self.hosted('key-c')
        self.wait(self.api, '/ready')
        self.until(lambda: self.sql('SELECT pending FROM hosted_signing_state WHERE singleton') == 'f')
        revision = int(self.sql('SELECT policy_revision FROM servers'))
        # Still-valid B wakes the server. Only the actual HTTP heartbeat
        # carries C's revocation of A, proving check-in delivery separately.
        self.wake('key-b')
        self.until(lambda: self.sqlite('SELECT revision FROM hosted_trust_state WHERE singleton=1')[0][0] == 3)
        assert self.sqlite('SELECT count(*) FROM hosted_signing_revocations WHERE key_id=?', (a_id,))[0][0] == 1
        self.server.request('GET', '/v1/me', token=self.session_a['session']['accessToken'], expected=401)
        self.server.request('GET', '/v1/me', token=self.app_session['session']['accessToken'], expected=401)
        self.server.request('GET', '/v1/me', token=self.tv_server_session['session']['accessToken'], expected=401)
        self.server.request('GET', '/v1/me', token=session_b['session']['accessToken'])
        self.server.request('GET', '/v1/me', token=self.direct)
        self.until(lambda: int(self.sql('SELECT acked_revision FROM wake_state')) >= revision)
        self.sign_in(self.account_token, random_id())
        self.passed('Check-in revocation ends A sessions; B, direct and C sessions work',
                    wakeRevisionAcknowledged=revision, transport='injected loopback worker; real heartbeat')

    def rate_limits(self):
        for path, body in [('/v1/server-claims/nonce', {}),
                           ('/v1/sessions', {'sessionMode': 'invalid'}),
                           ('/v1/registrations', {'sessionMode': 'invalid'}),
                           ('/v1/device-authorizations/token', {'deviceCode': 'unknown'}),
                           ('/v1/app-authorizations/token', {'code': 'unknown', 'verifier': random_id(),
                                                          'redirectUri': 'http://127.0.0.1:12345/return'})]:
            seen = False
            for _ in range(500):
                _, h, status = self.api.request('POST', path, body, expected=None)
                if status == 429:
                    assert int(h['Retry-After']) > 0
                    seen = True
                    break
            assert seen, path + ' did not rate-limit'
            self.passed('Anonymous rate limit ' + path)

    def cleanup(self):
        for p in reversed(self.processes):
            self.stop(p)
        if self.pgstarted:
            self.command([self.pg / 'pg_ctl', '-D', self.dir / 'postgres', '-m', 'fast', '-w', 'stop'])
        for f in self.files:
            f.close()
        self.proxy.shutdown()
        self.proxy.server_close()
        shutil.rmtree(self.socketdir)
        self.write('results.json', self.results)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--server-tree', required=True)
    p.add_argument('--scratch', required=True)
    p.add_argument('--pg-bin', required=True)
    p.add_argument('--skip-build', action='store_true', help='reuse binaries already built in scratch')
    args = p.parse_args()
    run = Run(args)
    print('Private evidence: ' + str(run.dir), flush=True)
    try:
        run.exercise()
    except Exception as error:
        run.results.append({'status': 'failed', 'error': str(error)})
        raise
    finally:
        run.cleanup()


if __name__ == '__main__':
    main()
