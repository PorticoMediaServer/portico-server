import test from 'node:test';
import assert from 'node:assert/strict';

const store = new Map<string, string>();
(globalThis as any).localStorage = {getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => { store.set(k, v); }, removeItem: (k: string) => { store.delete(k); }};
const {cachedServerList, serverListStorage, storeServerList} = await import('../src/app/server-list-cache.ts');

test('the kept server list drops signed routes (they go stale) and is per account', () => {
  storeServerList('acc_1', [{id: 'srv_1', name: 'Home', baseUrl: 'https://home.example', routes: {payload: 'x', signature: 'y'}} as any]);
  const kept = cachedServerList('acc_1')!;
  assert.equal(kept.length, 1);
  assert.equal('routes' in kept[0]!, false);
  assert.equal(cachedServerList('acc_2'), undefined);
  storeServerList('acc_1', null);
  assert.equal(cachedServerList('acc_1'), undefined);
});

test('the version record round-trips and a malformed one reads as none', () => {
  const s = serverListStorage('acc_1');
  s.save({version: '7', watch: 'w.mac', checkedAt: 1, nextCheckAt: 2});
  assert.deepEqual(s.load(), {version: '7', watch: 'w.mac', checkedAt: 1, nextCheckAt: 2});
  store.set('portico.serverList.version.acc_1', '{"version":7}');
  assert.equal(s.load(), null);
  s.save(null);
  assert.equal(s.load(), null);
});

test('the cached chooser retains servers beyond row 500', () => {
  const servers = Array.from({length: 551}, (_, i) => ({id: `srv_${i}`, name: `Server ${i}`, baseUrl: 'https://server.example'}));
  storeServerList('many', servers as any);
  assert.equal(cachedServerList('many')?.length, 551);
  assert.equal(cachedServerList('many')?.[550]?.id, 'srv_550');
});
