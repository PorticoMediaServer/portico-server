import {test} from 'node:test';
import assert from 'node:assert/strict';
import {discoveredRoutes, gatewayFailure} from '../src/route-connection.ts';

const pin = {serverId: 'srv-1', publicKey: 'A'.repeat(43), fingerprint: 'fp-1'};
const now = Date.now();
const good = {
  baseUrl: 'http://192.168.1.8:32500',
  serverId: 'srv-1',
  fingerprint: 'fp-1',
  port: 32500,
  path: '/',
  expiresAt: now + 60000,
};

test('a single good record yields one LAN candidate', () => {
  assert.deepEqual(discoveredRoutes([good], pin, now), [
    {baseUrl: 'http://192.168.1.8:32500', class: 'lan', quality: 'probe_required', generation: '1'},
  ]);
});

test('more than 64 records yields nothing, however good', () => {
  const many = Array.from({length: 65}, () => ({...good}));
  assert.deepEqual(discoveredRoutes(many, pin, now), []);
});

test('a conflicting group contributes nothing', () => {
  // Another serverId under the same fingerprint poisons the group.
  assert.deepEqual(discoveredRoutes([good, {...good, serverId: 'other'}], pin, now), []);
  // A bad path poisons the group too.
  assert.deepEqual(discoveredRoutes([good, {...good, path: '/other'}], pin, now), []);
  // A bad port poisons the group.
  assert.deepEqual(discoveredRoutes([good, {...good, port: 0}], pin, now), []);
  assert.deepEqual(discoveredRoutes([good, {...good, port: 70000}], pin, now), []);
  // Two distinct port|path combos poison the group (otherwise plausible records included).
  assert.deepEqual(
    discoveredRoutes([good, {...good, baseUrl: 'http://192.168.1.8:32501', port: 32501}], pin, now),
    [],
  );
  // Records under other fingerprints are ignored, not poisoning.
  assert.equal(discoveredRoutes([good, {...good, fingerprint: 'other', serverId: 'other'}], pin, now).length, 1);
});

test('expiry is bounded: finite, future, within two minutes', () => {
  for (const bad of [NaN, Infinity, now, now - 1, now + 120001]) {
    assert.deepEqual(discoveredRoutes([{...good, expiresAt: bad}], pin, now), [], String(bad));
  }
  assert.equal(discoveredRoutes([{...good, expiresAt: now + 1}], pin, now).length, 1);
  assert.equal(discoveredRoutes([{...good, expiresAt: now + 120000}], pin, now).length, 1);
});

test('only private hosts with matching ports survive; duplicates collapse; max eight', () => {
  assert.deepEqual(discoveredRoutes([{...good, baseUrl: 'https://public.example:32500'}], pin, now), []);
  assert.deepEqual(discoveredRoutes([{...good, baseUrl: 'http://192.168.1.8:9999'}], pin, now), []);
  assert.deepEqual(discoveredRoutes([good, {...good}], pin, now).length, 1);
  const eight = Array.from({length: 10}, (_, i) => ({
    ...good,
    baseUrl: `http://192.168.1.${10 + i}:32500`,
    port: 32500,
  }));
  // Same port+path across distinct hosts does not poison (one combo), but output caps at 8.
  // To keep one combo, vary only the host octets while keeping port/path identical:
  // the group check uses port|path only, so 10 records share one combo and survive to the cap.
  const out = discoveredRoutes(eight, pin, now);
  assert.equal(out.length, 8);
});

test('a proxy error page for a down server is a lost route; Portico answers are not', () => {
  const res = (status: number, type?: string) => new Response(status === 204 ? null : 'x', {status, headers: type ? {'content-type': type} : {}});
  assert.equal(gatewayFailure(res(502)), true);
  assert.equal(gatewayFailure(res(503, 'text/html')), true);
  assert.equal(gatewayFailure(res(504, 'text/plain; charset=utf-8')), true);
  // Portico's own 503s (visibility_building, unavailable features) are JSON answers.
  assert.equal(gatewayFailure(res(503, 'application/json')), false);
  assert.equal(gatewayFailure(res(500)), false);
  assert.equal(gatewayFailure(res(404, 'text/html')), false);
  assert.equal(gatewayFailure(res(200, 'application/json')), false);
});
