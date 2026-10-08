import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const noop = () => ({});
const stub = () =>
  new Proxy(
    {
      react: {default: {}, useCallback: (f: unknown) => f, useEffect: noop, useMemo: noop, useState: (v: unknown) => [v, noop]},
      '@i18n': {defaultI18n: {t: (k: string) => k}},
    },
    {get: (t: any, k: string) => t[k] ?? new Proxy({}, {get: () => noop})},
  );
const serversModule = () => componentModule(new URL('../src/screens/account/Servers.tsx', import.meta.url), stub()) as Promise<typeof import('../src/screens/account/Servers.tsx')>;

test('WEB-AUTH-02: servers parse defensively and malformed rows never fail the screen', async () => {
  const {parseAccountServers} = await serversModule();
  assert.equal(parseAccountServers({items: Array.from({length: 551}, (_, i) => ({id: `server-${i}`}))}).at(-1)?.id, 'server-550');
  assert.deepEqual(parseAccountServers(null), []);
  assert.deepEqual(parseAccountServers({items: 'nope'}), []);
  assert.deepEqual(parseAccountServers({items: [{id: 'a', name: 'Home', ownedByMe: false, presence: {online: true, lastSeenAt: '2026-09-20T10:00:00Z'}}, {id: 'b'}, {name: 'nameless'}, 'junk', {id: 'c', name: 'Cabin', ownedByMe: true, lastSeenAt: 'not-a-date'}]}), [
    {id: 'a', name: 'Home', ownedByMe: false, online: true, lastSeenAt: '2026-09-20T10:00:00Z'},
    {id: 'b', lastSeenAt: null},
    {id: 'c', name: 'Cabin', ownedByMe: true, lastSeenAt: null},
  ]);
});

test('WEB-AUTH-02: leaving a server posts to its leave route with the id encoded', async () => {
  const {leaveAccountServer} = await serversModule();
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  await leaveAccountServer(async (path: string, method = 'GET', body?: unknown) => {
    calls.push({path, method, body});
    return undefined;
  }, 'srv 1/id');
  assert.deepEqual(calls, [{path: '/v1/servers/srv%201%2Fid/leave', method: 'POST', body: {}}]);
});
