import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import {ClaimError} from '../../packages/client-core/src/claim-web.ts';

test('Hosted pending setup is never passed to local viewer admission', async () => {
  const h = hooks();
  const m = await componentModule(new URL('../src/screens/auth/ServerSetup.tsx', import.meta.url), {react: h.react, '../../app/claim-connect': {ClaimError}});
  const pending = Object.freeze({serverId: 'server', requestId: 'operation', authMode: 'hosted', claimRequired: true, revision: '1'});
  let admitted = false;
  await assert.rejects(m.acceptSetupSession({}, pending, async () => {admitted = true;}), (error: unknown) => error instanceof ClaimError && error.code === 'claim_unavailable');
  assert.equal(admitted, false);
  assert.equal('accessToken' in pending, false);
});

test('local setup forwards the original issued session and API without synthesizing authority', async () => {
  const h = hooks();
  const m = await componentModule(new URL('../src/screens/auth/ServerSetup.tsx', import.meta.url), {react: h.react});
  const api = {}, issued = Object.freeze({accessToken: 'unit-only-issued-token'});
  let observed: unknown[] = [];
  await m.acceptSetupSession(api, issued, async (...args: unknown[]) => {observed = args;});
  assert.equal(observed[0], api);
  assert.equal(observed[1], issued);
});
