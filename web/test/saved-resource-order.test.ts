import test from 'node:test';
import assert from 'node:assert/strict';
import type {SavedOrder, SavedService, SavedSnapshot} from '@core/saved.ts';
import {commitPreparedResourceOrder, preparePlaylistReorder, resourceOrderMove} from '../src/screens/saved/resource-order.ts';

test('the final reorder admission and mutation are atomic after asynchronous preparation', async () => {
  let revision = 7;
  const writes: number[] = [];
  await assert.rejects(commitPreparedResourceOrder({current: () => revision === 7, prepare: async () => {
    assert.equal(revision, 7);
    queueMicrotask(() => {revision = 8;});
  }}, async () => {writes.push(revision);}), error => (error as {code?: string}).code === 'revision_mismatch');
  assert.deepEqual(writes, [], 'a newer revision cannot authorize the old page permutation');
  revision = 7;
  await commitPreparedResourceOrder({prepare: async () => {}, current: () => {
    queueMicrotask(() => {revision = 8;});
    return revision === 7;
  }}, async () => {writes.push(revision);});
  assert.deepEqual(writes, [7], 'mutation captures the admitted revision without another yield');
});

function fixture(ids: readonly string[] = ['a', 'b'], count = 300, history: readonly (string | null)[] = []) {
  let snapshot: SavedSnapshot = {
    generation: 1, scope: {serverId: 'server', viewerId: 'viewer'}, route: {view: 'playlist', playlistId: 'playlist'}, phase: 'ready',
    playlist: {serverId: 'server', viewerFence: 'fence', id: 'playlist', name: 'Playlist', summary: '', revision: 7, pinned: false, pinRevision: 1,
      role: 'owner', entryCount: count, actions: ['reorder'], limits: {maxShares: 10, maxReorderEntries: 200}},
    projection: {playlistRevision: 7, heading: {key: 'saved.title', fallback: 'Saved'},
      scope: {serverId: 'server', libraryId: '', libraryKind: 'mixed', view: 'playlist', entityId: 'playlist', viewerFence: 'fence'},
      revision: {catalog: 1, viewer: 1}, navigation: [], query: {sort: 'position', direction: 'asc', category: '', q: '', limit: 40, searchMode: 'none'}, sorts: [], filters: [],
      sections: [{id: 'entries', type: 'list', heading: {key: 'saved.title', fallback: 'Saved'}, totalCount: count, nextCursor: count > ids.length ? 'content-next' : '',
        entries: ids.map(id => ({id, kind: 'playlist_entry', hidden: true}))}]},
    pagination: {cursor: history.length ? 'content-current' : null, history, canPrevious: !!history.length, next: []},
    error: null, pending: [], mutationError: null, lastReceipt: null, candidates: null, candidatesLoading: false, candidatesError: null,
    order: null, orderLoading: false, orderError: null,
  };
  let reads = 0, moreReads = 0;
  let load = async () => {}, more = async () => {};
  const service: Pick<SavedService, 'getSnapshot' | 'loadOrder' | 'loadMoreOrder'> = {
    getSnapshot: () => snapshot,
    loadOrder: async () => {reads++;snapshot = {...snapshot, order: null, orderError: null};await load();},
    loadMoreOrder: async () => {moreReads++;snapshot = {...snapshot, orderError: null};await more();},
  };
  const order = (entryIds: readonly string[], nextCursor = '', patch: Partial<SavedOrder> = {}) => {
    snapshot = {...snapshot, order: {serverId: 'server', viewerFence: 'fence', playlistId: 'playlist', revision: 7, entryIds, nextCursor, ...patch}};
  };
  return {service, order, get: () => snapshot, patch: (patch: Partial<SavedSnapshot>) => {snapshot = {...snapshot, ...patch};},
    onLoad: (work: () => Promise<void>) => {load = work;}, onMore: (work: () => Promise<void>) => {more = work;},
    reads: () => ({reads, moreReads})};
}

test('Reorder preserves occurrence identity and the supplied order window', () => {
  const ids = Object.freeze(['outside-left', 'a', 'hidden', 'b', 'outside-right']);
  assert.deepEqual(resourceOrderMove(ids, 'a', 'b'), {action: 'reorder', entryIds: ['hidden', 'b', 'a']});
  assert.deepEqual(resourceOrderMove(ids, 'b', 'a'), {action: 'reorder', entryIds: ['b', 'a', 'hidden']});
  assert.deepEqual(ids, ['outside-left', 'a', 'hidden', 'b', 'outside-right']);
  // Duplicate media can have distinct occurrence ids and must both survive.
  assert.deepEqual(resourceOrderMove(['movie-first', 'movie-second'], 'movie-first', 'movie-second'),
    {action: 'reorder', entryIds: ['movie-second', 'movie-first']});
});

test('Reorder refuses unknown, duplicate, unchanged or oversized windows', () => {
  assert.equal(resourceOrderMove(['a', 'b'], 'absent', 'b'), undefined);
  assert.equal(resourceOrderMove(['a', 'b'], 'a', 'absent'), undefined);
  assert.equal(resourceOrderMove(['a', 'a', 'b'], 'a', 'b'), undefined);
  assert.equal(resourceOrderMove(['a', 'b'], 'a', 'a'), undefined);
  const ids = Array.from({length: 201}, (_, i) => String(i));
  assert.equal(resourceOrderMove(ids, '0', '200'), undefined);
  assert.equal(resourceOrderMove(ids, '0', '199')?.entryIds.length, 200);
});

test('A complete current playlist projection needs no extra order read', async () => {
  const value = fixture(['a', 'b'], 2);
  await preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true);
  assert.deepEqual(value.reads(), {reads: 0, moreReads: 0});
});

test('A matching existing order witness needs no extra read', async () => {
  const value = fixture();value.order(['a', 'b'], 'unread-tail');
  await preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true);
  assert.deepEqual(value.reads(), {reads: 0, moreReads: 0});
});

test('A partial page reads bounded order ids, includes the anchor, and stops when verified', async () => {
  const value = fixture(['c', 'd'], 500, [null, 'previous', 'previous-2']);
  value.onLoad(async () => {value.order(['a', 'b'], 'next-1');});
  value.onMore(async () => {value.order(['a', 'b', 'c', 'd'], 'unread-tail');});
  await preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['d'], afterEntryId: 'c'}, () => true);
  assert.deepEqual(value.reads(), {reads: 1, moreReads: 1});
});

test('A failed prior read can be retried and a foreign witness cannot authorize a move', async () => {
  const value = fixture();
  value.order(['a', 'b'], '', {viewerFence: 'old-viewer'});
  value.patch({orderError: {code: 'network', message: 'Previous read failed', retryable: true}});
  value.onLoad(async () => {value.order(['a', 'b']);});
  await preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true);
  assert.deepEqual(value.reads(), {reads: 1, moreReads: 0});
});

test('Revision, viewer, permission and route retirement during order reads all refuse the move', async () => {
  for (const patch of [{revision: 8}, {viewerFence: 'other-viewer'}, {id: 'other-playlist'}, {actions: []}]) {
    const value = fixture();value.onLoad(async () => {value.order(['a', 'b']);value.patch({playlist: {...value.get().playlist!, ...patch}});});
    await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true), /playlist changed/i);
    assert.deepEqual(value.reads(), {reads: 1, moreReads: 0});
  }
  const value = fixture();let live = true;
  value.onLoad(async () => {value.order(['a', 'b']);live = false;});
  await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => live), /playlist changed/i);
});

test('Failed order reads are reported before another request or mutation', async () => {
  const value = fixture();const failure = {code: 'network', message: 'Order read failed', retryable: true};
  value.onLoad(async () => {value.patch({orderError: failure});});
  await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true), error => error === failure);
  assert.deepEqual(value.reads(), {reads: 1, moreReads: 0});
});

test('Missing occurrences, cursor cycles and unexpectedly long continuations stop within the page budget', async () => {
  for (const kind of ['end', 'cycle', 'long'] as const) {
    const value = fixture();let page = 0;
    value.onLoad(async () => {value.order(['other'], kind === 'end' ? '' : 'cursor-0');});
    value.onMore(async () => {value.order(['other'], kind === 'cycle' ? 'cursor-0' : `cursor-${++page}`);});
    await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true), /could not be verified/i);
    assert.ok(value.reads().reads + value.reads().moreReads <= 2);
  }
});

test('An unavailable resource or retired command cannot start an order read', async () => {
  const value = fixture();
  await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => false), /no longer available/i);
  value.patch({playlist: {...value.get().playlist!, actions: []}});
  await assert.rejects(preparePlaylistReorder(value.service, {action: 'reorder', entryIds: ['b', 'a']}, () => true), /no longer available/i);
  assert.deepEqual(value.reads(), {reads: 0, moreReads: 0});
});

test('Global destinations use the prepend sentinel and only a proven complete tail', async () => {
  const {resourceOrderComplete, resourceOrderEdge} = await import('../src/screens/saved/resource-order.ts');
  const ids = ['first', 'hidden', 'duplicate-media-occurrence', 'last'];
  assert.equal(resourceOrderComplete(ids, 4, true, ['', null]), true);
  for (const [count, start, cursors] of [[5, true, ['']], [4, false, ['']], [4, true, ['unread']]] as const)
    assert.equal(resourceOrderComplete(ids, count, start, cursors), false);
  assert.equal(resourceOrderComplete(['a', 'a'], 2, true, []), false);
  assert.deepEqual(resourceOrderEdge(ids, 'duplicate-media-occurrence', 'top', false), {action: 'reorder', entryIds: ['duplicate-media-occurrence'], afterEntryId: ''});
  assert.deepEqual(resourceOrderEdge(ids, 'first', 'bottom', true), {action: 'reorder', entryIds: ['first'], afterEntryId: 'last'});
  assert.equal(resourceOrderEdge(ids, 'first', 'bottom', false), undefined);
  assert.equal(resourceOrderEdge(ids, 'first', 'top', true), undefined);
  assert.equal(resourceOrderEdge(ids, 'last', 'bottom', true), undefined);
  assert.equal(resourceOrderEdge(ids, 'missing', 'top', false), undefined);
});

test('Edge moves retain the asynchronous witness and final revision guards', async () => {
  for (const afterEntryId of ['', 'b']) {
    const value = fixture();let live = true, writes = 0;
    value.onLoad(async () => {value.order(['a', 'b']);queueMicrotask(() => {live = false;});});
    const intent = {action: 'reorder' as const, entryIds: ['a'], afterEntryId};
    await assert.rejects(commitPreparedResourceOrder({current: () => live,
      prepare: () => preparePlaylistReorder(value.service, intent, () => live)}, async () => {writes++;}));
    assert.equal(writes, 0);
  }
});

test('Move results report only proven global positions and retain hidden slots', async () => {
  const {resourceOrderEdge, resourceOrderPosition} = await import('../src/screens/saved/resource-order.ts');
  const ids = ['a', 'hidden', 'b', 'tail'];
  assert.equal(resourceOrderPosition(resourceOrderMove(ids, 'a', 'b')!, ids, 'a', true), 3);
  assert.equal(resourceOrderPosition(resourceOrderMove(ids, 'a', 'b')!, ids, 'a', false), undefined);
  assert.equal(resourceOrderPosition(resourceOrderEdge(ids, 'b', 'top', false)!, ids, 'b', false), 1);
  assert.equal(resourceOrderPosition(resourceOrderEdge(ids, 'a', 'bottom', true)!, ids, 'a', true), 4);
  let result: number | undefined;
  await assert.rejects(commitPreparedResourceOrder({prepare: async () => {}, current: () => true}, async () => {throw new Error('refused');})
    .then(() => {result = resourceOrderPosition(resourceOrderMove(ids, 'a', 'b')!, ids, 'a', true);}));
  assert.equal(result, undefined, 'a refused move has no successful result to announce');
});
