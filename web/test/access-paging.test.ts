import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const loadAccess = async () => {
  const cursors = await loadCursors();
  return componentModule(new URL('../src/screens/server/Access.tsx', import.meta.url), {
    react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
    '@core/server-administration.ts': {}, '../../admin/console': {}, '../../admin/cursor-read': cursors,
    './ListPager': {}, './Server': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
  }) as Promise<any>;
};

// Pure helpers loaded as the real code with their imports stubbed.
const loadCursors = () => componentModule(new URL('../src/admin/cursor-read.ts', import.meta.url), {
  react: hooks().react, './console': {},
}) as Promise<any>;
const loadAdmin = () => componentModule(new URL('../../packages/client-core/src/server-administration.ts', import.meta.url), {
  react: {default: {}},
  './server-messages.ts': {unreadableServerResponse: 'Unreadable server response.'},
}) as Promise<any>;

test('CD-07: every access list pages at limit=100 and follows nextCursor', async () => {
  const {accessLists, accessListPath} = await loadAccess();
  for (const list of Object.keys(accessLists)) {
    const first = accessListPath(list);
    assert.match(first, /limit=100/, `${list} stays on the server page limit`);
    assert.doesNotMatch(first, /limit=200/, `${list} never requests 200 rows`);
    assert.doesNotMatch(first, /cursor=/, `${list} starts uncursored`);
    assert.match(accessListPath(list, 'c:42'), /cursor=c%3A42/, `${list} threads the cursor`);
  }
});

test('CD-07: 101+ devices stay reachable across pages without limit=200', async () => {
  const {accessListPath} = await loadAccess();
  const {nextCursorOf} = await loadCursors();
  const {parseAccessDevicePage} = await loadAdmin();
  const device = (n: number) => ({id: `d${n}`, accountId: 'a1', trust: 'approved', firstSeenAt: '2026-09-01T00:00:00Z', lastSeenAt: '2026-09-02T00:00:00Z', revision: 1});
  // First page: 100 devices and a cursor onward.
  const raw1 = {limit: 100, items: Array.from({length: 100}, (_, n) => device(n)), nextCursor: 'c:100', approvalRequired: false};
  assert.match(accessListPath('devices'), /limit=100/);
  const page1 = parseAccessDevicePage(raw1);
  assert.equal(page1.items.length, 100);
  const cursor = nextCursorOf(raw1);
  assert.equal(cursor, 'c:100');
  // Second page reached through that cursor: the last device.
  assert.match(accessListPath('devices', cursor), /cursor=c%3A100/);
  const raw2 = {limit: 100, items: [device(100)], nextCursor: '', approvalRequired: false};
  const page2 = parseAccessDevicePage(raw2);
  assert.equal(page2.items.length, 1);
  assert.equal(nextCursorOf(raw2), '');
  assert.equal(page1.items.length + page2.items.length, 101);
});
