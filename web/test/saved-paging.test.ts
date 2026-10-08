import test from 'node:test';
import assert from 'node:assert/strict';
import {SavedService} from '@core/saved.ts';
import {accumulatePage} from '../src/screens/shared/section-pages.ts';
import {addItemsToPlaylist} from '../src/screens/shared/playlist-add.ts';

const scope = {serverId: 'server', viewerId: '["local","account","profile"]'};
let ids = 0;
const requestId = async () => `00000000-0000-4000-8000-${String(++ids).padStart(12, '0')}`;
const media = (n: number) => ({id: `m${n}`, libraryId: 'library', kind: 'movie', title: `Movie ${n}`, available: true, playback: {itemId: `m${n}`, startSeconds: 0}});
const occurrence = (n: number) => ({id: `e${n}`, kind: 'playlist_entry', hidden: false, media: media(n)});
const card = (n: number) => ({id: `p${n}`, kind: 'playlist', title: `Playlist ${n}`, navigation: {view: 'playlist', entityId: `p${n}`}});
const navigation = [
  {id: 'watchlist', labelKey: 'saved.watchlist', view: 'watchlist'},
  {id: 'favorites', labelKey: 'saved.favorites', view: 'favorites'},
  {id: 'playlists', labelKey: 'saved.playlists', view: 'playlists'},
];
function projection(view: string, entries: unknown[], cursor: string, total: number) {
  return {
    scope: {serverId: 'server', libraryId: '', libraryKind: 'mixed', view, entityId: view === 'playlist' ? 'playlist' : '', viewerFence: 'fence'},
    revision: {catalog: 0, viewer: 0}, ...(view === 'playlist' ? {playlistRevision: 0} : {}),
    heading: {key: 'saved.title', fallback: 'Saved'}, navigation,
    query: {sort: view === 'playlist' ? 'position' : 'title', direction: 'asc', category: '', q: '', limit: 40, searchMode: 'none'},
    sorts: [], filters: [], actions: view === 'playlists' ? ['create_playlist'] : undefined,
    sections: [{id: 'entries', type: 'list', heading: {key: 'saved.entries', fallback: 'Entries'}, entries, totalCount: total, nextCursor: cursor}],
  };
}
const resource = (entryCount: number) => ({
  serverId: 'server', viewerFence: 'fence', id: 'playlist', name: 'Playlist', summary: '', revision: 0,
  role: 'owner', entryCount, actions: ['rename', 'summary', 'delete', 'share', 'add', 'remove', 'reorder'],
  limits: {maxEntries: 1000, maxShares: 100, maxReorderEntries: 1000}, shares: [],
});

test('PERF-S08: playlist paging reaches entry 1,000 through the service cursor', async () => {
  const requests: string[] = [];
  const api = {
    request: async (path: string) => {
      requests.push(path);
      if (path.startsWith('/v1/playlists/playlist/content')) {
        const cursor = new URLSearchParams(path.split('?')[1]).get('cursor');
        const page = cursor ? Number(cursor.slice(1)) : 0;
        const entries = Array.from({length: 40}, (_, i) => occurrence(page * 40 + i));
        return projection('playlist', entries, page < 24 ? `c${page + 1}` : '', 1000);
      }
      return resource(1000);
    },
  };
  const service = new SavedService({api, scope, requestId, timeoutMs: 500});
  await service.select({view: 'playlist', playlistId: 'playlist'});
  const store = {scope: '', pages: new Map<string, readonly {id: string}>()};
  let all: readonly {id: string}[] = [];
  for (;;) {
    const snap = service.getSnapshot();
    assert.equal(snap.phase, 'ready');
    const page = (snap.projection?.sections ?? []).flatMap(s => s.entries) as {id: string}[];
    all = accumulatePage(store, 'playlist:playlist', snap.pagination.cursor, page, e => e.id);
    const pageable = (snap.projection?.sections ?? []).find(s => s.nextCursor);
    if (!pageable) break;
    await service.next(pageable.id);
  }
  assert.equal(all.length, 1000);
  assert.equal(all[999]!.id, 'e999');
  assert.equal(requests.filter(p => p.includes('/content')).length, 25);
});

test('PERF-S08: the picker pages past the first 40 playlists', async () => {
  const api = {
    request: async (path: string) => {
      if (path.includes('cursor')) return projection('playlists', Array.from({length: 20}, (_, i) => card(40 + i)), '', 60);
      return projection('playlists', Array.from({length: 40}, (_, i) => card(i)), 'p2', 60);
    },
  };
  const service = new SavedService({api, scope, requestId, timeoutMs: 500});
  await service.select({view: 'playlists'});
  const store = {scope: '', pages: new Map<string, readonly {id: string}>()};
  const first = service.getSnapshot().projection!.sections.flatMap(s => s.entries) as {id: string}[];
  let all = accumulatePage(store, 'playlists', service.getSnapshot().pagination.cursor, first, e => e.id);
  assert.equal(all.length, 40);
  await service.next(service.getSnapshot().pagination.next[0]!.sectionId);
  const second = service.getSnapshot().projection!.sections.flatMap(s => s.entries) as {id: string}[];
  all = accumulatePage(store, 'playlists', service.getSnapshot().pagination.cursor, second, e => e.id);
  assert.equal(all.length, 60);
  assert.equal(all[40]!.id, 'p40');
});

test('PERF-S08: bulk add counts per-item outcomes and keeps sending past a failure', async () => {
  const posts: unknown[] = [];
  const api = {
    request: async (path: string, method?: string, body?: unknown) => {
      if ((method ?? 'GET') === 'GET') return path.includes('/content') ? projection('playlist', [occurrence(0)], '', 3) : resource(3);
      posts.push((body as {itemId?: string}).itemId);
      if ((body as {itemId?: string}).itemId === 'm1') throw Object.assign(new Error('gone'), {code: 'request_failed'});
      const b = body as {operationId: string};
      return {serverId: 'server', viewerFence: 'fence', operationId: b.operationId, playlistId: 'playlist', revision: 0, deleted: false};
    },
  };
  const service = new SavedService({api, scope, requestId, timeoutMs: 500});
  await service.select({view: 'playlist', playlistId: 'playlist'});
  const result = await addItemsToPlaylist(service, 'playlist', ['m0', 'm1', 'm2']);
  assert.deepEqual(result, {ok: 2, failed: ['m1']});
  // Q7: m1's failure rejects and leaves the queue, so m2 still sends; each
  // item is attempted once (no wedged retry/select-clear workaround).
  assert.ok(posts.includes('m0'));
  assert.ok(posts.includes('m2'));
  assert.equal(posts.filter(p => p === 'm1').length, 1);
});

test('Q7: a failed add followed by a successful add lands the second with exact counts', async () => {
  const posts: string[] = [];
  const api = {
    request: async (path: string, method?: string, body?: unknown) => {
      if ((method ?? 'GET') === 'GET') return path.includes('/content') ? projection('playlist', [occurrence(0)], '', 2) : resource(2);
      const itemId = (body as {itemId?: string}).itemId!;
      posts.push(itemId);
      if (itemId === 'bad') throw Object.assign(new Error('denied'), {code: 'request_failed'});
      const b = body as {operationId: string};
      return {serverId: 'server', viewerFence: 'fence', operationId: b.operationId, playlistId: 'playlist', revision: 0, deleted: false};
    },
  };
  const service = new SavedService({api, scope, requestId, timeoutMs: 500});
  await service.select({view: 'playlist', playlistId: 'playlist'});
  // Direct queue check: the failed command rejects and unblocks the next.
  await assert.rejects(service.mutate({action: 'add', itemId: 'bad'}), /denied/);
  assert.deepEqual(service.getSnapshot().pending, []);
  await service.mutate({action: 'add', itemId: 'good'});
  assert.deepEqual(posts, ['bad', 'good']);
  assert.equal(service.getSnapshot().lastReceipt!.playlistId, 'playlist');
  // Bulk helper counts the same run exactly.
  const bulk = new SavedService({api, scope, requestId, timeoutMs: 500});
  await bulk.select({view: 'playlist', playlistId: 'playlist'});
  assert.deepEqual(await addItemsToPlaylist(bulk, 'playlist', ['bad', 'good']), {ok: 1, failed: ['bad']});
});
