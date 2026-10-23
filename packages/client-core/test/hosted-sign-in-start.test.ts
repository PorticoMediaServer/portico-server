import assert from 'node:assert/strict';
import {test} from 'node:test';
import {buildHostedSignInStartURL, parseHostedSignInStartURL} from '../src/hosted-sign-in-start.ts';
import {routeBase64} from '../src/route-identity.ts';
import type {RegisteredSignIn} from '../src/same-tab-return.ts';

const now = Date.UTC(2030, 0, 1), hosted = 'https://web.example.test';
const token = (byte: number) => routeBase64(new Uint8Array(32).fill(byte));
const value = (): RegisteredSignIn => ({start: {serverId: 'srv_' + token(1), purpose: 'sign-in', claimOperationId: 'installed_operation-1',
  registrationId: 'ret_' + token(2), state: token(3), codeChallenge: token(4), serverChallenge: token(5), installationId: '10000000-0000-4000-8000-000000000001'},
  registration: {id: 'ret_' + token(2), origin: 'https://server.example.test'}, hostedOrigin: hosted, expiresAt: new Date(now + 60_000).toISOString()});
const encode = (json: string) => hosted + '/#signIn=v1.' + routeBase64(new TextEncoder().encode(json));

test('the genuine installed operation survives a canonical ordinary HTTPS-root handoff', () => {
  const input = value(), href = buildHostedSignInStartURL(input, now), parsed = parseHostedSignInStartURL(href, hosted, now);
  assert.deepEqual(parsed, input); assert.equal(parsed?.start.claimOperationId, 'installed_operation-1');
  assert(Object.isFrozen(parsed) && Object.isFrozen(parsed.start) && Object.isFrozen(parsed.registration));
  assert.equal(parseHostedSignInStartURL(href, 'https://other.example.test', now), undefined);
});
test('the builder rejects every protected or unknown field at all three object boundaries', () => {
  for (const key of ['verifier', 'moveTicket', 'moveCode', 'credentials', 'ownerTransactionId', 'pinReset', 'expectedRevision']) {
    for (const part of ['root', 'start', 'registration']) {
      const input = value() as unknown as Record<string, any>;
      (part === 'root' ? input : input[part])[key] = 'must-never-travel';
      assert.throws(() => buildHostedSignInStartURL(input as unknown as RegisteredSignIn, now));
    }
  }
  const input = value(); let called = false;
  Object.defineProperty(input.start, 'state', {enumerable: true, get() { called = true; return token(3); }});
  assert.throws(() => buildHostedSignInStartURL(input, now)); assert.equal(called, false);
});
test('ordinary purpose, actual operation, registration binding, token encodings and original lifetime are mandatory', () => {
  const changes = [
    (v: any) => { v.start.purpose = 'switch'; }, (v: any) => { v.start.claimOperationId = ''; },
    (v: any) => { v.registration.id = 'ret_' + token(9); }, (v: any) => { v.start.state += '='; },
    (v: any) => { v.start.codeChallenge = 'a'.repeat(42); }, (v: any) => { v.start.installationId = 'short'; },
    (v: any) => { v.hostedOrigin = 'http://127.0.0.1:19410'; }, (v: any) => { v.registration.origin += '/'; },
    (v: any) => { v.registration.origin = 'https://user:pass@server.example.test'; },
    (v: any) => { v.expiresAt = new Date(now).toISOString(); }, (v: any) => { v.expiresAt = new Date(now + 300_001).toISOString(); },
  ];
  for (const change of changes) { const input = value(); change(input); assert.throws(() => buildHostedSignInStartURL(input, now)); }
  const href = buildHostedSignInStartURL(value(), now);
  assert.equal(parseHostedSignInStartURL(href, hosted, now + 60_000), undefined);
});
test('the parser rejects duplicate/reordered/extra JSON, noncanonical base64, UTF8 and URL spellings', () => {
  const json = JSON.stringify(value()), href = encode(json);
  const invalid = [
    encode(' ' + json), encode(json.replace('"start":', '"unexpected":0,"start":')),
    encode(json.replace('"purpose":"sign-in"', '"purpose":"switch","purpose":"sign-in"')),
    encode(JSON.stringify({...value(), start: {...value().start, verifier: 'never'}})),
    encode(JSON.stringify({expiresAt: value().expiresAt, hostedOrigin: hosted, registration: value().registration, start: value().start})),
    href + '=', href + '&other=1', href.replace('/#', '/account#'), href.replace('/#', '/?query=1#'),
    href.replace('https://web.', 'https://WEB.'), hosted + '/#signIn=v1.' + routeBase64(new Uint8Array([0xc3, 0x28])),
    hosted + '/#signIn=v1.' + 'A'.repeat(8192),
  ];
  for (const candidate of invalid) assert.equal(parseHostedSignInStartURL(candidate, hosted, now), undefined);
});
