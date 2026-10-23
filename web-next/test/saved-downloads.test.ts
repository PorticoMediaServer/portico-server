import test from 'node:test';
import assert from 'node:assert/strict';
import {SavedOperations} from '../src/screens/saved/operations.ts';
import {sameSavedDownloadsScope, type SavedDownloadsScope, type SavedLocalDownload} from '../src/screens/saved/downloads-port.ts';
import {diskFractions, localDownloadGroups, measuredPercent} from '../src/screens/saved/downloads-model.ts';
import {createSavedRuntime} from '../src/screens/saved/runtime.ts';
import {sameSavedRoute} from '../src/screens/saved/route.ts';

const scope: SavedDownloadsScope = {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', sessionFamilyId: 'family'};
test('every captured identity boundary distinguishes the binding', () => {
  assert.equal(sameSavedDownloadsScope(scope, {...scope}), true);
  for (const field of ['serverId', 'authority', 'accountId', 'profileId', 'sessionFamilyId'] as const) {
    assert.equal(sameSavedDownloadsScope(scope, {...scope, [field]: field === 'authority' ? 'hosted' : 'other'}), false, field);
  }
});
test('authority closing while an operation completes suppresses all UI effects', async () => {
  let current = true, complete!: (value: number) => void, callbacks = 0;
  const operations = new SavedOperations(() => current);
  const pending = operations.run(() => new Promise<number>(resolve => { complete = resolve; }), () => callbacks++);
  current = false; complete(1); await pending;
  assert.equal(callbacks, 0);
});
test('parent disposal aborts pending native/API work and cannot reopen for another family', async () => {
  let signal: AbortSignal | undefined, reject!: (error: unknown) => void, callbacks = 0;
  const operations = new SavedOperations(() => true);
  const pending = operations.run(value => { signal = value; return new Promise((_, fail) => { reject = fail; }); }, () => callbacks++);
  operations.dispose(); assert.equal(signal?.aborted, true);
  reject(new Error('late native result')); await pending;
  await operations.run(async () => { callbacks++; }); assert.equal(callbacks, 0);
});
test('current native failures stay actionable; late failures are silent', async () => {
  const operations = new SavedOperations(() => true), failure = new Error('native write refused');
  await assert.rejects(operations.run(async () => { throw failure; }), error => error === failure);
  operations.dispose(); await operations.run(async () => { throw failure; });
});
test('route effect cleanup aborts the old callback even when the current view opens again', async () => {
  let active = true, complete!: () => void, callbacks = 0, signal: AbortSignal | undefined;
  const operations = new SavedOperations(() => active);
  const pending = operations.run(value => { signal = value; return new Promise<void>(resolve => { complete = resolve; }); }, () => callbacks++);
  active = false; operations.abortPending(); active = true; complete(); await pending;
  assert.equal(signal?.aborted, true); assert.equal(callbacks, 0);
  await operations.run(async () => {}, () => callbacks++);
  assert.equal(callbacks, 1); operations.dispose();
});
test('Saved route guards compare the complete query independently of property order', () => {
  const route = {view: 'watchlist', filter: 'unwatched', sort: 'title', direction: 'asc', shape: 'posters'} as const;
  assert.equal(sameSavedRoute({view: 'watchlist', shape: 'posters', sort: 'title', direction: 'asc', filter: 'unwatched'}, route), true);
  for (const other of [{...route, filter: 'all'} as const, {...route, direction: 'desc'} as const, {...route, shape: 'squares'} as const, {...route, sort: 'added'} as const]) assert.equal(sameSavedRoute(other, route), false);
  assert.equal(sameSavedRoute({view: 'playlist', playlistId: 'old'}, {view: 'playlist', playlistId: 'new'}), false);
  assert.equal(sameSavedRoute({view: 'history', period: '30d'}, {view: 'history', period: 'all'}), false);
  assert.equal(sameSavedRoute({view: 'resource', resourceId: 'a'}, {view: 'resource', resourceId: 'b'}), false);
  assert.equal(sameSavedRoute(null, route), false);
});
test('storage bar uses actual OS total/free and all native Portico bytes', () => {
  assert.deepEqual(diskFractions({totalBytes: 1000, freeBytes: 400, usedBytes: 70}), {portico: 0.07, other: 0.53});
  for (const storage of [{totalBytes: 0, freeBytes: 0, usedBytes: 0}, {totalBytes: 100, freeBytes: 80, usedBytes: 30}, {totalBytes: 100, freeBytes: -1, usedBytes: 0}]) assert.equal(diskFractions(storage), undefined);
  assert.equal(measuredPercent(null), undefined); assert.equal(measuredPercent(NaN), undefined); assert.equal(measuredPercent(101), undefined); assert.equal(measuredPercent(0), 0);
});
test('local group identity survives duplicate show names; missing-server bytes remain visible', () => {
  const item = (id: string, parentId: string, onServer = true): SavedLocalDownload => ({id, revision: 1, preparationId: id, itemId: id, quality: 'original', state: 'ready', held: false, bytes: 100, bytesDone: 100, percent: 100, offlineAccess: 'valid', onServer, title: {kind: 'episode', title: id, year: 2026, durationSeconds: 1, parentId, parentTitle: 'Same show name', groupId: '', groupTitle: '', groupNumber: 1, number: 1, artist: '', onServer}});
  const groups = localDownloadGroups([item('a', 'show-a'), item('b', 'show-b'), item('c', 'show-a'), item('d', 'show-a', false)]);
  assert.deepEqual(groups.map(group => [group.id, group.items.map(row => row.id)]), [['show:show-a', ['a', 'c']], ['show:show-b', ['b']], ['server', ['d']]]);
  assert.equal('count' in groups[0], false);
});
test('the actual Saved runtime refuses incomplete authority captures before requesting data', () => {
  for (const key of ['serverId', 'accountId', 'profileId', 'sessionFamilyId'] as const) {
    assert.throws(() => createSavedRuntime({scope: {...scope, [key]: ''}, api: {request: async () => { throw new Error('unexpected API call'); }}, isCurrent: () => true}), /complete captured/);
  }
});
test('no server service begins a read after the parent authority closes', async () => {
  let calls = 0;
  const runtime = createSavedRuntime({scope, api: {request: async () => { calls++; throw new Error('unexpected API call'); }}, isCurrent: () => false});
  await runtime.select({view: 'watchlist'});
  assert.equal(calls, 0); runtime.dispose();
});
test('parent disposal aborts both real shape-service transports and drops delayed responses', async () => {
  const signals: AbortSignal[] = [];
  const runtime = createSavedRuntime({scope, isCurrent: () => true, api: {request: (_path, _method, _body, signal) => {
    assert.ok(signal); signals.push(signal);
    return new Promise((_resolve, reject) => signal.addEventListener('abort', () => reject(new DOMException('Cancelled', 'AbortError')), {once: true}));
  }}});
  const pending = runtime.select({view: 'watchlist'});
  assert.equal(signals.length, 2); runtime.dispose(); await pending;
  assert.ok(signals.every(signal => signal.aborted));
  assert.equal(runtime.posters.getSnapshot().projection, null); assert.equal(runtime.squares.getSnapshot().projection, null);
});
