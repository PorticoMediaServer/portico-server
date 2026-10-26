import assert from 'node:assert/strict';
import test from 'node:test';
import {HostedAccountClient} from '../src/hosted-security.ts';

// ACC-LEAF-30: receipt lifetime and grant separation in the SDK, authored UNRUN.
// The transport is deferred so the descriptor can be mutated after dispatch; the
// client is the real one and nothing here authenticates a provider.

const origin = 'https://accounts.example.test';
const state = Buffer.from(new Uint8Array(32).fill(3)).toString('base64url');
const other = Buffer.from(new Uint8Array(32).fill(4)).toString('base64url');
const txA = 'oid_' + 'A'.repeat(43), txB = 'oid_' + 'B'.repeat(43);
const scope = {accountId: 'acct_A', familyId: 'family_A'};
const location = (tx: string, originValue = origin, stateValue = state) =>
  originValue + '/auth/portico-return#security=scr_' + 'C'.repeat(43) + '&state=' + stateValue + '&transaction=' + tx;
const start = () => ({transactionId: txA, registeredOrigin: origin, state, expiresAt: '2030-01-01T00:00:00Z',
  mode: 'reauthenticate' as const, provider: 'google' as const, intentId: 'sri_' + 'I'.repeat(43), context: {payload: 'p', signature: 's'}, authorizationUrl: 'https://accounts.google.com/x'});

function client() {
  const calls: {path: string; method?: string; body: unknown; token?: string}[] = [];
  let response: unknown;
  const transport = async (path: string, method: string, body: unknown, _signal?: AbortSignal, token?: string) => {
    calls.push({path, method, body, token});
    return response;
  };
  return {client: new HostedAccountClient(origin, 'browser', transport as never), calls,
    respond: (value: unknown) => {response = value;}};
}

test('finishSecurityReturn uses the captured transaction, origin and state after the caller mutates them', async () => {
  const f = client(), descriptor = start();
  f.respond({location: location(txA)});
  const running = f.client.finishSecurityReturn(descriptor);
  descriptor.transactionId = txB; descriptor.registeredOrigin = 'https://evil.example'; descriptor.state = other;
  assert.equal(await running, location(txA));
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0].path.includes(txA), true);
});

test('a successor or arbitrary-origin location is refused with one command', async () => {
  for (const value of [location(txB), location(txA, 'https://evil.example'), location(txA, origin, other)]) {
    const f = client(); f.respond({location: value});
    await assert.rejects(f.client.finishSecurityReturn(start()), value);
    assert.equal(f.calls.length, 1);
  }
});

test('completeSecurityReturn accepts only a scoped recentProof and refuses any grant field', async () => {
  const proof = {...scope, proof: 'p'.repeat(32), expiresAt: '2030-01-01T00:00:00Z'};
  const accepted = client(); accepted.respond({recentProof: proof});
  const out = await accepted.client.completeSecurityReturn(txA, {code: 'scr_' + 'C'.repeat(43), verifier: state, confirmation: {payload: 'p', signature: 's'}, factor: '123456'}, scope, 'token_A');
  assert.equal(out.proof, proof.proof);
  assert.equal(accepted.calls.length, 1);
  assert.equal(accepted.calls[0].token, 'token_A');
  for (const value of [{recentProof: {...proof, accountId: 'acct_B'}}, {recentProof: {...proof, familyId: 'family_B'}},
    {recentProof: proof, session: {accessToken: 'x'}}, {session: {accessToken: 'x'}}, {recentProof: proof, securityResult: {status: 'ok'}}, {recentProof: proof, grant: 'x'}]) {
    const f = client(); f.respond(value);
    await assert.rejects(f.client.completeSecurityReturn(txA, {code: 'scr_' + 'C'.repeat(43), verifier: state, confirmation: {payload: 'p', signature: 's'}, factor: '123456'}, scope, 'token_A'), JSON.stringify(value));
    assert.equal(f.calls.length, 1);
  }
});
