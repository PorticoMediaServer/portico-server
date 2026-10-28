import test from 'node:test';
import assert from 'node:assert/strict';
import {parseServerSignIn, ServerSignInProbe, signInLabelKey, serverSignInSettingsURL} from '../src/sign-in.ts';
const system = (signIn = 'local') => ({id: 'server', name: 'Study', signIn, move:null,version: '1', setupRequired: false, hostedConfigured: true, hostedAttached: false, apiLevel: 1});
test('Change this on the web opens Sign-in on the exact chosen server without secrets or alternate authorities',()=>{
  assert.equal(serverSignInSettingsURL('http://192.168.1.5:32500'),'http://192.168.1.5:32500/settings?scope=server&section=signin');
  for(const address of ['https://user:pass@server.test','https://server.test/?code=secret','https://server.test/#secret','javascript:alert(1)','https://server.test/path'])assert.throws(()=>serverSignInSettingsURL(address));
});
test('explicit kind alone controls credential route and correct label, never old attached flag', async () => {
  for (const signIn of ['local', 'portico'] as const) {
    const calls: unknown[] = [];
    const probe = new ServerSignInProbe(async (url, init) => {calls.push([url, init]); return Response.json({...system(signIn), hostedAttached: signIn === 'local', apiLevel: 1});});
    await probe.inspect('http://192.168.1.5:8096');
    assert.equal(probe.getSnapshot().destination, signIn === 'local' ? 'local-account' : 'portico-account');
    assert.equal(signInLabelKey(signIn), 'accounts.session.' + signIn);
    assert.equal(calls.length, 1); const [url, init] = calls[0] as [string, RequestInit];
    assert.equal(url, 'http://192.168.1.5:8096/v1/system'); assert.equal(init.credentials, 'omit'); assert.equal(init.redirect, 'error'); assert.equal(new Headers(init.headers).has('Authorization'), false);
    probe.dispose();
  }
});
test('missing/unknown/malformed kind never offers credentials; setup remains separate', async () => {
  for (const signIn of [undefined, '', 'both', 'hosted']) assert.throws(() => parseServerSignIn({...system(), signIn}), {code: 'invalid_system'});
  const probe = new ServerSignInProbe(async () => Response.json({...system(), setupRequired: true}));
  await probe.inspect('https://server.test'); assert.equal(probe.getSnapshot().destination, 'setup'); probe.dispose();
});
test('cancelled/offline/timed-out/late probes cannot reopen an old password form', async () => {
  let answer!: (v: Response) => void;
  const probe = new ServerSignInProbe(() => new Promise(resolve => {answer = resolve;}), 10);
  const pending = probe.inspect('https://old.test'); probe.cancel(); answer(Response.json(system())); await pending;
  assert.equal(probe.getSnapshot().phase, 'idle');
  await probe.inspect('https://offline.test'); assert.equal(probe.getSnapshot().phase, 'error');
  answer(Response.json(system('portico'))); await new Promise(resolve => setTimeout(resolve, 1));
  assert.equal(probe.getSnapshot().phase, 'error'); probe.dispose();
});

test('default browser fetch is not invoked with the probe as its receiver', async () => {
  const original = globalThis.fetch;
  const calls: unknown[][] = [];
  globalThis.fetch = async function(this: unknown, input, init) {
    // The browser's native fetch accepts a value call and rejects an arbitrary
    // instance receiver. Node fetch alone would hide this production defect.
    if (this !== undefined && this !== globalThis) throw new TypeError('Illegal invocation');
    calls.push([this, input, init]);
    return Response.json(system());
  };
  const probe = new ServerSignInProbe();
  try {
    assert.equal((await probe.inspect('https://server.test'))?.signIn, 'local');
    assert.equal(probe.getSnapshot().phase, 'ready');
    assert.equal(calls.length, 1);
    assert.equal(calls[0][1], 'https://server.test/v1/system');
    const init = calls[0][2] as RequestInit;
    assert.equal(init.credentials, 'omit');
    assert.equal(init.redirect, 'error');
    assert.equal(new Headers(init.headers).has('Authorization'), false);
  } finally {probe.dispose(); globalThis.fetch = original;}
});
test('injected value and explicitly bound fetch transports preserve their invocation contract', async () => {
  const receivers: unknown[] = [];
  const transport: typeof fetch = async function(this: unknown) {
    receivers.push(this);return Response.json(system('portico'));
  };
  const owner = {name: 'injected transport'};
  for (const [fetcher, expected] of [[transport, undefined], [transport.bind(owner), owner]] as const) {
    const probe = new ServerSignInProbe(fetcher);
    try {
      assert.equal((await probe.inspect('https://server.test'))?.signIn, 'portico');
      assert.equal(receivers.at(-1), expected);
    } finally {probe.dispose();}
  }
});

test('a server of another API level is refused before sign-in (P2-66 G-2)', async () => {
  const {parseServerSignIn, assertSameApiLevel, API_LEVEL_MISMATCH} = await import('../src/sign-in.ts');
  const {API_LEVEL} = await import('../../contracts/src/api-level.ts');
  const raw = {id: 'server', name: 'Study', version: '1', signIn: 'portico', move: null, setupRequired: false, hostedConfigured: true, hostedAttached: true};
  for (const apiLevel of [undefined, 0, -1, 1.5, '1', null]) assert.throws(() => parseServerSignIn({...raw, apiLevel}), /did not report how to sign in/);
  assert.doesNotThrow(() => assertSameApiLevel(parseServerSignIn({...raw, apiLevel: API_LEVEL})));
  for (const apiLevel of [API_LEVEL - 1 || 2, API_LEVEL + 1]) assert.throws(() => assertSameApiLevel(parseServerSignIn({...raw, apiLevel})), (error: {code?: string}) => error.code === API_LEVEL_MISMATCH);
});

test('the move window comes from the server and is counted in whole days on this calendar (P2-66)', async () => {
  const {parseServerSignIn, moveOfferDays} = await import('../src/sign-in.ts');
  const raw = {id: 'server', name: 'Study', version: '1', signIn: 'portico', move: 'to_portico', setupRequired: false, hostedConfigured: true, hostedAttached: true, apiLevel: 1};
  assert.equal(parseServerSignIn({...raw, moveEndsAt: '2026-11-20T10:00:00Z'}).moveEndsAt, '2026-11-20T10:00:00Z');
  assert.equal(parseServerSignIn({...raw, moveEndsAt: null}).moveEndsAt, null);
  assert.equal(parseServerSignIn(raw).moveEndsAt, null);
  for (const moveEndsAt of ['not a date', 7, {}]) assert.throws(() => parseServerSignIn({...raw, moveEndsAt}), /move window/);
  assert.throws(() => parseServerSignIn({...raw, move: null, moveEndsAt: '2026-11-20T10:00:00Z'}), /move window/, 'only a move to Portico Accounts has an old-sign-in window');
  const now = new Date(2026, 9, 28, 21, 0);
  assert.equal(moveOfferDays(new Date(2026, 9, 28, 23, 0).toISOString(), now), 0, 'ends today');
  assert.equal(moveOfferDays(new Date(2026, 9, 29, 9, 0).toISOString(), now), 1, 'ends tomorrow: within the next day');
  assert.equal(moveOfferDays(new Date(2026, 10, 27, 9, 0).toISOString(), now), 30);
  assert.equal(moveOfferDays(new Date(2026, 9, 28, 20, 0).toISOString(), now), undefined, 'already over');
  assert.equal(moveOfferDays(null, now), undefined);
});
