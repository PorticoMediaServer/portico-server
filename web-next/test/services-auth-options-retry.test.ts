import assert from 'node:assert/strict';
import {test} from 'node:test';
import {ApiError} from '@core/index.ts';
import {ServicesAuthFlow} from '../src/services/auth-flow.ts';
import type {ServicesAccount} from '../src/services/session.ts';

// FX133-B: absent-capability initialization failure and the explicit retry,
// authored UNRUN. Real ServicesAuthFlow; only the account port is a fixture.

const options = {password: true, recovery: true, registration: true, providers: [], verifiedContactRequired: true, accountSecurity: true};
function port(script: (attempt: number) => unknown) {
  let reads = 0, authenticated = 0;
  const value = {
    client: {options: async () => {reads++; const next = script(reads); if (next instanceof Error) throw next; return next;},
      transaction: async () => null, registration: async () => null, login: async () => ({}), mfa: async () => ({}), request: async () => ({})},
    service: {authenticateIfSignedOut: async (work: (signal: AbortSignal) => Promise<unknown>) => {authenticated++; return work(new AbortController().signal);}},
  } as unknown as ServicesAccount;
  return {value, reads: () => reads, authenticated: () => authenticated};
}

for (const mode of ['signin', 'create', 'forgot'] as const) test('a failed options read in '+mode+' is recoverable only by the explicit retry', async () => {
  const p = port(attempt => attempt === 1 ? new Error('service unavailable') : options);
  const flow = new ServicesAuthFlow(p.value, () => {}, mode);
  await flow.initialize();
  assert.deepEqual({options: flow.getSnapshot().options, failed: flow.getSnapshot().optionsFailed, busy: flow.getSnapshot().busy, mode: flow.getSnapshot().mode},
    {options: undefined, failed: true, busy: false, mode});
  for (let i = 0; i < 8; i++) await Promise.resolve();
  assert.equal(p.reads(), 1);                     // no automatic retry
  await flow.retryInitialization();
  assert.equal(p.reads(), 2);                     // exactly one additional request
  assert.equal(flow.getSnapshot().options?.password, true);
  assert.equal(flow.getSnapshot().optionsFailed, false);
  assert.equal(flow.getSnapshot().mode, mode);    // original mode preserved
  flow.dispose();
});

test('a concurrent initialize and retry share one flight and one request', async () => {
  const p = port(attempt => attempt === 1 ? new Error('service unavailable') : options);
  const flow = new ServicesAuthFlow(p.value, () => {});
  await flow.initialize();
  await Promise.all([flow.retryInitialization(), flow.initialize(), flow.retryInitialization()]);
  assert.equal(p.reads(), 2);
  assert.equal(flow.getSnapshot().options?.password, true);
  flow.dispose();
});

test('a 429 retry deadline suppresses the request without sleeping', async () => {
  const p = port(attempt => attempt === 1 ? new ApiError(429, 'rate_limited', 'rate_limited', true, {retryAfterSeconds: 5}) : options);
  const flow = new ServicesAuthFlow(p.value, () => {});
  await flow.initialize();
  assert.equal(flow.getSnapshot().optionsFailed, true);
  assert.equal(flow.getSnapshot().retryAt > Date.now(), true);
  await flow.retryInitialization();
  assert.equal(p.reads(), 1);                     // still the original deadline
  flow.dispose();
});

test('a disposed form retries nothing and a late original response cannot publish', async () => {
  let release!: (value: unknown) => void;
  const p = port(attempt => attempt === 1 ? new Promise(resolve => {release = resolve;}) as never : options);
  const flow = new ServicesAuthFlow(p.value, () => {});
  const pending = flow.initialize();
  flow.dispose();
  await flow.retryInitialization();
  assert.equal(p.reads(), 1);
  release(options);
  await pending.catch(() => {});
  assert.equal(flow.getSnapshot().options, undefined);
  assert.equal(flow.getSnapshot().optionsFailed, undefined);
  assert.equal(p.authenticated(), 0);
});
