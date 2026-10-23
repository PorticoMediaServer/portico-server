import test from 'node:test';
import assert from 'node:assert/strict';
import type {ScreenContext} from '../src/screens/detail/context.ts';
import {createLiveBindings} from '../src/screens/live/bindings.ts';
import {parseLiveScreenSearch} from '../src/screens/live/route-search.ts';
const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('Live accepts an actual channel identity without inventing its source or provenance', () => {
  assert.deepEqual(parseLiveScreenSearch({channelId: 'opaque/channel', token: 'unowned'}), {channelId: 'opaque/channel'});
  assert.deepEqual(parseLiveScreenSearch({sourceId: 'actual-source', kind: 'library-channel', view: 'guide', group: 'Drama'}),
    {sourceId: 'actual-source', kind: 'library-channel', view: 'guide', group: 'Drama'});
  for (const raw of [{view: 'invented'}, {kind: 'library'}, {favorites: 'true'}, {channelId: ['opaque']}, {sourceId: 'x\n'}]) assert.throws(() => parseLiveScreenSearch(raw));
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
