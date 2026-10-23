import test from 'node:test';
import assert from 'node:assert/strict';
import type {ScreenContext} from '../src/screens/detail/context.ts';
import {createLiveBindings} from '../src/screens/live/bindings.ts';
import {parseLiveScreenSearch} from '../src/screens/live/route-search.ts';
import {screenEntryDestination} from '../src/screens/browse/navigation.ts';
const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('Live accepts an actual channel identity without inventing its source or provenance', () => {
  assert.deepEqual(parseLiveScreenSearch({channelId: 'opaque/channel', token: 'unowned'}), {channelId: 'opaque/channel'});
  assert.deepEqual(parseLiveScreenSearch({sourceId: 'actual-source', kind: 'library-channel', view: 'guide', group: 'Drama'}),
    {sourceId: 'actual-source', kind: 'library-channel', view: 'guide', group: 'Drama'});
  for (const raw of [{view: 'invented'}, {kind: 'library'}, {favorites: 'true'}, {channelId: ['opaque']}, {sourceId: 'x\n'}]) assert.throws(() => parseLiveScreenSearch(raw));
});
test('Search navigation preserves only its canonical channel tuple and keeps Person separate', () => {
  assert.deepEqual(screenEntryDestination({id: 'channel/a', kind: 'channel', title: 'Actual channel',
    navigation: {view: 'channel', entityId: 'channel/a', channelId: 'channel/a', sourceId: 'actual-source', provenance: 'live-source'}}),
  {to: '/live', search: {sourceId: 'actual-source', channelId: 'channel/a', kind: 'live-source'}});
  assert.deepEqual(screenEntryDestination({id: 'legacy', kind: 'channel', title: 'Actual legacy channel', navigation: {view: 'channel', entityId: 'legacy'}}),
    {to: '/live', search: {channelId: 'legacy'}});
  assert.deepEqual(screenEntryDestination({id: 'person/a', kind: 'person', title: 'Actual person', navigation: {view: 'person', entityId: 'person/a'}}),
    {to: '/person/$personId', params: {personId: 'person/a'}});
});
test('a selected channel is one named, authorized guide read and never a guessed source lookup', async () => {
  const requests: URL[] = [];let wrongSource = false;
  const context = {scope: {serverId: 'server', viewerId: 'viewer'}, isCurrent: () => true,
    api: {async request(path: string) {
      const url = new URL(path, 'https://example.test');requests.push(url);
      const source = {id: wrongSource ? 'other-source' : 'actual-source', name: 'Actual source', provenance: 'live-source', generation: 'generation',
        publishedAt: new Date().toISOString(), availableStart: '', availableEnd: '', refreshState: 'healthy'};
      return {protocolVersion: '1.0', serverId: 'server', guide: {viewerFence: 'actual-fence', state: 'no-guide-data',
        start: url.searchParams.get('start'), end: url.searchParams.get('end'), timezone: 'UTC', observedAt: new Date().toISOString(), nextCursor: '', sources: [source],
        channels: [{id: 'actual-channel', sourceId: source.id, provenance: 'live-source', generation: 'generation', name: 'Actual channel', number: '1', group: '',
          programmes: [], tuneAvailable: true, recordAvailable: false, favorite: false, hidden: false, preferenceRevision: 0}]}};
    }}} as unknown as ScreenContext;
  const owner = createLiveBindings(context, {kind: 'all', sourceId: '', timezone: 'UTC'});
  try {
    const target = {sourceId: 'actual-source', channelId: 'actual-channel', provenance: 'live-source' as const};
    assert.equal((await owner.selectedChannel(target))?.sourceId, 'actual-source');
    assert.equal(requests[0].pathname, '/v1/guide');
    assert.equal(requests[0].searchParams.get('channels'), target.channelId);
    assert.equal(requests[0].searchParams.get('sourceId'), target.sourceId);
    assert.equal(requests[0].searchParams.get('limit'), '1');
    wrongSource = true;assert.equal(await owner.selectedChannel(target), undefined);
    assert.equal(requests.length, 2);
  } finally {owner.dispose();}
});
test('Live construction is inert and an authority refusal retires the bounded guide before further HTTP', async () => {
  let reads = 0;
  const denied = Object.assign(new Error('denied'), {status: 403});
  const context = {scope: {serverId: 'server', viewerId: 'viewer'}, isCurrent: () => true,
    api: {async request() {reads++;throw denied;}}} as unknown as ScreenContext;
  const owner = createLiveBindings(context, {kind: 'all', sourceId: '', timezone: 'UTC'});
  assert.equal(reads, 0);
  await assert.rejects(owner.sources(), error => error === denied);
  assert.equal(owner.life.current(), false);
  assert.equal(owner.failure.getSnapshot(), denied);
  assert.deepEqual(owner.store.resident(), {channelPages: 0, tiles: 0, programs: 0, inFlight: 0, queued: 0});
  owner.store.setViewport({firstRow: 0, lastRow: 12, start: 0, end: 60_000});await tick();
  await assert.rejects(owner.sources(), {name: 'AbortError'});
  assert.equal(reads, 1);
  owner.dispose();assert.equal(owner.failure.getSnapshot(), undefined);
});
