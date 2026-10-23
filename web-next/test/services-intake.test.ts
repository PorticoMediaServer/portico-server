import assert from 'node:assert/strict';
import {test} from 'node:test';
import {captureServicesLink, forgetServicesLink, restoreServicesLink, type ServicesLink, type ServicesLinkStore} from '../src/services/intake.ts';

function vault(initial?: ServicesLink) {
  let value = initial;
  const store: ServicesLinkStore = {read: async () => value, change: async update => { value = update(value); }};
  return {store, value: () => value};
}

test('device and mail fragments are removed before any asynchronous continuation work', () => {
  for (const [pathname, hash, value] of [['/device', '#code=7kqm4txp', '7KQM-4TXP'], ['/reset', '#token=secret_123', 'secret_123'], ['/verify-email', '#token=secret_123', 'secret_123']]) {
    let scrubbed = false;
    const result = captureServicesLink({pathname, hash, search: ''}, {state: {entry: 1}, replaceState: (_state, _title, url) => { scrubbed = true; assert.equal(url, pathname); }}, 1000);
    assert.equal(scrubbed, true); assert.equal(result?.link?.value, value);
  }
});

test('ambiguous new links clear previous continuation rather than restoring it', async () => {
  const saved: ServicesLink = {version: 1, kind: 'device', value: '7KQM-4TXP', createdAt: 1000};
  for (const hash of ['#code=7KQM-4TXP&code=7KQM-4TXP', '#code=7KQM-4TXP&return=secret', '#code=ILLEGAL0', '#token=secret']) {
    const v = vault(saved);
    const capture = captureServicesLink({pathname: '/device', search: '', hash}, {state: null, replaceState() {}}, 1001);
    assert.equal(await restoreServicesLink(v.store, capture, 1001), undefined); assert.equal(v.value(), undefined);
  }
});

test('a protected continuation survives reload only within its drawn lifetime', async () => {
  const saved: ServicesLink = {version: 1, kind: 'device', value: '7KQM-4TXP', createdAt: 1000};
  const v = vault(saved);
  assert.deepEqual(await restoreServicesLink(v.store, undefined, 600999), saved);
  assert.equal(await restoreServicesLink(v.store, undefined, 601000), undefined);
  const future = vault({...saved, createdAt: 2000});
  assert.equal(await restoreServicesLink(future.store, undefined, 1000), undefined);
});

test('finishing an older page cannot clear a newer link opened in another tab', async () => {
  const old: ServicesLink = {version: 1, kind: 'reset', value: 'first', createdAt: 1000};
  const next = {...old, value: 'second', createdAt: 1001};
  const v = vault(next); await forgetServicesLink(v.store, old); assert.deepEqual(v.value(), next);
  await forgetServicesLink(v.store, next); assert.equal(v.value(), undefined);
});
