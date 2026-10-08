/** Hosted `/claim` page: the code comes from the fragment and is scrubbed; lookup/approve/deny use lane A's shapes. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

async function helpers() {
  const h = hooks();
  return componentModule(new URL('../src/screens/auth/ClaimApproval.tsx', import.meta.url), {react: h.react, '@core/index.ts': {HttpLocalApi: class {}}, '../../bridge/account': {}, '../../app/i18n': {}, '../../app/errors': {}, '../../ui': {}, './AccountForm': {}, './AuthFrame': {}, './Auth.module.css': {default: {}}});
}
const code = 'x'.repeat(49);

test('claim page: reads code, server and return from the fragment and scrubs the address bar', async () => {
  const m = await helpers();
  let replaced = '';
  const got = m.takeClaimFragment({hash: `#code=${code}&server=Den&return=https%3A%2F%2Fden.example%3A32400`, pathname: '/claim', search: ''}, (url: string) => { replaced = url; });
  assert.deepEqual(got, {code, name: 'Den', returnOrigin: 'https://den.example:32400'});
  assert.equal(replaced, '/claim');
});

test('claim page: a malformed code is scrubbed and rejected; no fragment means no request', async () => {
  const m = await helpers();
  let replaced = '';
  assert.equal(m.takeClaimFragment({hash: '#code=short', pathname: '/claim', search: ''}, (url: string) => { replaced = url; }), null);
  assert.equal(replaced, '/claim');
  assert.equal(m.takeClaimFragment({hash: '', pathname: '/claim', search: ''}, () => { throw new Error('nothing to scrub'); }), null);
});

test('claim page: lookup, approve-code and deny post the code (and the owner profile when chosen)', async () => {
  const m = await helpers();
  const calls: any[] = [];
  const api = {request: async (path: string, method: string, body: unknown) => { calls.push([path, method, body]); return {}; }};
  await m.claimLookup(api, code);
  await m.claimApprove(api, code, 'prof_2');
  await m.claimApprove(api, code);
  await m.claimDeny(api, code);
  assert.deepEqual(calls, [
    ['/v1/server-claims/lookup', 'POST', {code}],
    ['/v1/server-claims/approve-code', 'POST', {code, ownerProfileId: 'prof_2'}],
    ['/v1/server-claims/approve-code', 'POST', {code}],
    ['/v1/server-claims/deny', 'POST', {code}],
  ]);
});
