import test from 'node:test';
import assert from 'node:assert/strict';
import { LibraryContentService, type ContentProjection, type ContentRoute, type LibraryContentApi } from '../src/library-content.ts';
const scope = { viewerId: JSON.stringify(['hosted','account','profile']), serverId: 'server' };
const route: ContentRoute = { libraryId: 'movies', view: 'browse' };
function entry(id = 'item') { return { id, kind: 'movie', title: `Server title ${id}`, navigation: { view: 'item', entityId: id } }; }
function projection(path: string, changes: Record<string, unknown> = {}): any {
  const u = new URL(path, 'https://server.test');
  return { heading: { key: 'library.title', fallback: 'Named Library' }, scope: { serverId: 'server', libraryId: decodeURIComponent(u.pathname.split('/')[3]), libraryKind: 'movie', view: u.searchParams.get('view'), entityId: u.searchParams.get('entityId') ?? '', viewerFence: 'fence' }, revision: { catalog: 3, viewer: 4 }, query: { sort: u.searchParams.get('sort') ?? 'title', direction: u.searchParams.get('direction') ?? 'asc', category: u.searchParams.get('category') ?? '', q: u.searchParams.get('q') ?? '', limit: Number(u.searchParams.get('limit')), searchMode: 'title_prefix' }, navigation: [{ id: 'discover', labelKey: 'library.discover', view: 'discover' }, { id: 'browse', labelKey: 'library.browse', view: 'browse' }], sorts: [{ id: 'title', labelKey: 'sort.title', directions: ['asc','desc'] }], filters: [{ id: 'category', labelKey: 'filter.category', options: [{ id: 'decade:1990', label: '1990s', count: 5 }] }], sections: [{ id: 'server-grid', type: 'grid', heading: { key: 'server.heading', fallback: 'Server-selected heading' }, entries: [entry()], totalCount: 1, nextCursor: '' }], ...changes };
}
function service(request: (path: string, signal?: AbortSignal) => Promise<unknown>, timeoutMs = 1000) {
  return new LibraryContentService({ api: { request: <T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => request(path, signal) as Promise<T> }, scope, timeoutMs });
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r; }); return { resolve, promise }; }

test('an unknown card kind drops only that Discover entry and retains the section total', async () => {
  const nav = service(async path => projection(path, {sections: [{id: 'personal', type: 'rail', heading: {key: 'personal', fallback: 'For you'}, entries: [entry('first'), {...entry('future'), kind: 'interactive'}, entry('last')], totalCount: 7, nextCursor: 'next'}]}));
  await nav.select({libraryId: 'movies', view: 'discover'});
  assert.equal(nav.getSnapshot().phase, 'ready');
  assert.deepEqual(nav.getSnapshot().sections[0].entries.map(card => card.id), ['first', 'last']);
  assert.equal(nav.getSnapshot().sections[0].totalCount, 7);
  assert.equal(nav.getSnapshot().sections[0].nextCursor, 'next');
  nav.dispose();
});

test('catalog entry retains every valid genre beyond the former preview size', async () => {
  const genres = Array.from({length: 25}, (_, i) => `Genre ${i}`);
  const nav = service(async path => { const result = projection(path); result.sections[0].entries[0].genres = genres; return result; });
  await nav.select(route);
  assert.deepEqual(nav.getSnapshot().projection!.sections[0].entries[0].genres, genres);
});

test('provider synopsis line breaks survive the library projection boundary', async () => {
  for (const overview of ['An office.\r\nA second paragraph.', 'A first line.\n\tAn indented line.', 'x'.repeat(16384)]) {
    const nav = service(async path => {
      const result = projection(path);
      result.sections[0].entries = [{ id: 'show', kind: 'show', title: 'The Office', overview, navigation: { view: 'show', entityId: 'show' } }];
      return result;
    });
    await nav.select(route);
    assert.equal(nav.getSnapshot().phase, 'ready');
    assert.equal(nav.getSnapshot().sections[0].entries[0].overview, overview);
    nav.dispose();
  }
});

test('synopsis support does not relax unsafe controls or structural fields', async () => {
  for (const fields of [{overview:'Unsafe\u0000text'}, {overview:'Unsafe\u000btext'}, {overview:'Unsafe\u007ftext'}, {overview:'x'.repeat(16385)}, {overview:42}, {overview:'Valid\r\ntext',title:'Invalid\nname'}, {overview:'Valid\ntext',posterUrl:'https://example.test/\nimage'}]) {
    const nav = service(async path => {
      const result = projection(path);
      Object.assign(result.sections[0].entries[0], fields);
      return result;
    });
    await nav.select(route);
    assert.equal(nav.getSnapshot().phase, 'error');
    assert.equal(nav.getSnapshot().error?.code, 'invalid_projection');
    assert.equal(nav.getSnapshot().sections.length, 0);
    nav.dispose();
  }
});

test('publishes exact server semantic rows, order, metadata and chapter playback intent', async () => {
  const nav = service(async path => projection(path, { sections: [{ id: 'server-chapters', type: 'list', heading: { key: 'book.chapters', fallback: 'Chapters' }, entries: [{ id: 'chapter-9', kind: 'chapter', title: 'First chosen by server', addedAt: null, playback: { itemId: 'file-2', startSeconds: 97.5 } }, { id: 'chapter-1', kind: 'chapter', title: 'Second chosen by server', playback: { itemId: 'file-1', startSeconds: 0 } }], totalCount: 2, nextCursor: '' }] }));
  await nav.select({ libraryId: 'books', view: 'book', entityId: 'book-1' });
  const state = nav.getSnapshot(); assert.equal(state.phase, 'ready');
  assert.deepEqual(state.sections[0].entries.map(e => e.id), ['chapter-9','chapter-1']);
  assert.equal(state.sections[0].heading.fallback, 'Chapters');
  assert.equal(state.sections[0].entries[0].addedAt, null);
  assert.deepEqual(state.sections[0].entries[0].playback, { itemId: 'file-2', startSeconds: 97.5 });
  assert.equal(state.projection!.filters[0].options[0].count, 5);
  assert.throws(() => { (state.sections[0].entries as any).reverse(); }, TypeError);
});

test('all typed domain routes request one canonical endpoint with encoded query values', async () => {
  const paths: string[] = [];
  const nav = service(async path => { paths.push(path); return projection(path); });
  for (const view of ['discover','browse','collections','categories','show','season','artist','album','book','collection'] as const) {
    await nav.select({ libraryId: 'library/a', view, ...(['show','season','artist','album','book','collection'].includes(view) ? { entityId: 'entity&x=1' } : {}) });
    assert.equal(nav.getSnapshot().phase, 'ready');
    assert.ok(paths.at(-1)!.startsWith('/v1/libraries/library%2Fa/content?'));
  }
  await nav.select({ ...route, sort: 'year', direction: 'desc', category: 'decade:1990', q: 'a&b? c' });
  const u = new URL(paths.at(-1)!, 'https://server.test');
  assert.equal(u.searchParams.get('q'), 'a&b? c'); assert.equal(u.searchParams.get('category'), 'decade:1990');
  assert.equal(u.searchParams.get('limit'), '40');
});

test('rapid library switch aborts and fences a late response even if transport ignores abort', async () => {
  const old = deferred<unknown>(); let oldPath = '', oldSignal: AbortSignal | undefined;
  const nav = service(async (path, signal) => { if (path.includes('/old/')) { oldPath = path; oldSignal = signal; return old.promise; } return projection(path); });
  const pending = nav.select({ libraryId: 'old', view: 'browse' });
  await nav.select(route); await pending;
  assert.equal(oldSignal?.aborted, true);
  old.resolve(projection(oldPath)); await Promise.resolve();
  assert.equal(nav.getSnapshot().projection!.scope.libraryId, 'movies');
});

test('same in-flight route is deduplicated without client entry membership changes', async () => {
  const result = deferred<unknown>(); let calls = 0, path = '';
  const nav = service(async p => { calls++; path = p; return result.promise; });
  const first = nav.select(route), second = nav.select(route);
  assert.equal(first, second); assert.equal(calls, 1);
  result.resolve(projection(path)); await first;
  assert.equal(nav.getSnapshot().phase, 'ready');
});

test('new viewer/server scope clears old projection and fences old scoped API', async () => {
  const result = deferred<unknown>(); let path = '';
  const nav = service(async p => { path = p; return result.promise; }); const pending = nav.select(route);
  nav.setScope({ viewerId: 'new-viewer', serverId: 'new-server' }, { request: async <T>(p: string) => { const data = projection(p); data.scope.serverId = 'new-server'; return data as T; } });
  assert.equal(nav.getSnapshot().phase, 'idle'); assert.equal(nav.getSnapshot().projection, null);
  await nav.select(route); result.resolve(projection(path)); await pending;
  assert.equal(nav.getSnapshot().projection!.scope.serverId, 'new-server');
  assert.equal(nav.getSnapshot().scope.viewerId, 'new-viewer');
});

test('one bounded page replaces previous page and continuation addresses selected section only', async () => {
  const paths: string[] = [];
  const nav = service(async p => {
    paths.push(p); const u = new URL(p, 'https://server.test'); const cursor = u.searchParams.get('cursor');
    const data = projection(p); data.sections[0].entries = [entry(cursor ?? 'first')]; data.sections[0].totalCount = 10; data.sections[0].nextCursor = cursor ? '' : 'signed-cursor'; return data;
  });
  await nav.select(route); await nav.next('server-grid');
  assert.equal(nav.getSnapshot().sections[0].entries.length, 1); assert.equal(nav.getSnapshot().sections[0].entries[0].id, 'signed-cursor');
  assert.equal(nav.getSnapshot().pagination.canPrevious, true);
  await nav.previous(); assert.equal(nav.getSnapshot().sections[0].entries[0].id, 'first');
  assert.equal(nav.getSnapshot().pagination.canPrevious, false);
  await nav.select({ libraryId: 'new', view: 'browse' });
  assert.equal(new URL(paths.at(-1)!, 'https://server.test').searchParams.has('cursor'), false);
});

test('stale continuation requires explicit first-page refresh and cannot retry stale cursor', async () => {
  let calls = 0; const paths: string[] = [];
  const nav = service(async path => {
    calls++; paths.push(path);
    if (new URL(path, 'https://server.test').searchParams.has('cursor')) throw Object.assign(new Error('changed'), { code: 'stale_continuation', status: 409, retryable: true });
    const data = projection(path); data.sections[0].nextCursor = 'old'; return data;
  });
  await nav.select(route); await nav.next('server-grid');
  assert.equal(nav.getSnapshot().phase, 'refresh-required'); assert.equal(nav.getSnapshot().projection, null);
  await nav.retry(); await nav.next('server-grid'); assert.equal(calls, 2);
  await nav.refresh(); assert.equal(calls, 3); assert.ok(!paths.at(-1)!.includes('cursor='));
  assert.equal(nav.getSnapshot().phase, 'ready');
});

test('Q5: an expired section cursor stays refresh-required (no range.start on /content)', async () => {
  // Spec: GET /v1/content and GET /v1/libraries/{id}/content accept only
  // cursor + limit; range.start exists only on POST browse. A section page
  // carrying stale_continuation or invalid_cursor therefore cannot re-anchor
  // by index — it stays an explicit first-page refresh (server need recorded
  // in M10 Results Q5).
  for (const code of ['stale_continuation', 'invalid_cursor']) {
    const paths: string[] = [];
    const nav = service(async path => {
      paths.push(path);
      if (new URL(path, 'https://server.test').searchParams.has('cursor')) throw Object.assign(new Error('expired'), {code, status: code === 'invalid_cursor' ? 400 : 409});
      const data = projection(path); data.sections[0].nextCursor = 'old'; return data;
    });
    await nav.select(route); await nav.next('server-grid');
    assert.equal(nav.getSnapshot().phase, 'refresh-required', code);
    assert.equal(nav.getSnapshot().error!.code, code);
    // No index re-anchor was attempted: the only continuation request carried
    // the opaque cursor, never a range/start offset.
    assert.ok(paths.at(-1)!.includes('cursor=old'));
    assert.ok(!paths.at(-1)!.includes('range') && !paths.at(-1)!.includes('start='));
    await nav.refresh();
    assert.ok(!paths.at(-1)!.includes('cursor='));
    assert.equal(nav.getSnapshot().phase, 'ready');
    nav.dispose();
  }
  // A first-page invalid_cursor (no continuation) remains a plain error.
  const first = service(async path => {
    if (!new URL(path, 'https://server.test').searchParams.has('cursor')) throw Object.assign(new Error('bad'), {code: 'invalid_cursor', status: 400});
    return projection(path);
  });
  await first.select(route);
  assert.equal(first.getSnapshot().phase, 'error');
  assert.equal(first.getSnapshot().error!.code, 'invalid_cursor');
  first.dispose();
});

test('client rejects successful continuation with mismatched revision or viewer fence', async () => {
  for (const mutate of [(d: any) => { d.revision.catalog++; }, (d: any) => { d.revision.viewer++; }, (d: any) => { d.scope.viewerFence = 'other-viewer'; }]) {
    const nav = service(async path => { const data = projection(path); data.sections[0].nextCursor = 'next'; if (path.includes('cursor=')) { mutate(data); data.sections[0].nextCursor = ''; } return data; });
    await nav.select(route); await nav.next('server-grid'); assert.equal(nav.getSnapshot().phase, 'refresh-required');
  }
});

test('rejects wrong scope, oversized rows, duplicate IDs, malformed metadata and wrong continuation section', async () => {
  for (const mutate of [
    (d: any) => { d.scope.libraryId = 'wrong'; }, (d: any) => { d.scope.serverId = 'wrong'; },
    (d: any) => { d.sections[0].entries = Array.from({ length: 101 }, (_, i) => entry(String(i))); d.sections[0].totalCount = 101; },
    (d: any) => { d.sections[0].entries = [entry(), entry()]; d.sections[0].totalCount = 2; },
    (d: any) => { d.sections = Array(5).fill(d.sections[0]); }, (d: any) => { d.navigation = null; },
    (d: any) => { d.revision.catalog = -1; }, (d: any) => { d.sections[0].entries[0].duration = Number.NaN; },
  ]) {
    const nav = service(async p => { const data = projection(p); mutate(data); return data; }); await nav.select(route);
    assert.equal(nav.getSnapshot().phase, 'error'); assert.equal(nav.getSnapshot().error!.code, 'invalid_projection'); assert.equal(nav.getSnapshot().sections.length, 0);
  }
  const nav = service(async p => { const d = projection(p); d.sections[0].nextCursor = 'next'; if (p.includes('cursor=')) d.sections[0].id = 'wrong-section'; return d; });
  await nav.select(route); await nav.next('server-grid'); assert.equal(nav.getSnapshot().error!.code, 'invalid_projection');
});

test('timeout and cancel settle locally even with permanently hung transport', async () => {
  const nav = service(() => new Promise(() => {}), 5); await nav.select(route);
  assert.equal(nav.getSnapshot().phase, 'error'); assert.equal(nav.getSnapshot().error!.code, 'timeout'); assert.equal(nav.getSnapshot().error!.retryable, true);
  const cancelled = service(() => new Promise(() => {}), 10000); const pending = cancelled.select(route); cancelled.cancel(); await pending;
  assert.equal(cancelled.getSnapshot().phase, 'idle'); assert.equal(cancelled.getSnapshot().projection, null);
});

test('permission failure clears rows immediately and remains an explicit server error', async () => {
  let deny = false;
  const nav = service(async p => { if (deny) throw Object.assign(new Error('Permission revoked'), { code: 'permission_denied', status: 403, retryable: false }); return projection(p); });
  await nav.select(route); deny = true; const pending = nav.refresh();
  assert.equal(nav.getSnapshot().phase, 'loading'); await pending;
  assert.equal(nav.getSnapshot().error!.code, 'permission_denied'); assert.equal(nav.getSnapshot().projection, null);
});

test('continuation history is bounded and cyclic cursors require refresh', async () => {
  let page = 0;
  const nav = service(async p => { const data = projection(p); data.sections[0].nextCursor = `page-${++page}`; return data; });
  await nav.select(route); for (let i = 0; i < 80; i++) await nav.next('server-grid');
  let backs = 0; while (nav.getSnapshot().pagination.canPrevious) { await nav.previous(); backs++; }
  assert.equal(backs, 63);
  const cycle = service(async p => { const d = projection(p); d.sections[0].nextCursor = 'same'; return d; });
  await cycle.select(route); await cycle.next('server-grid'); assert.equal(cycle.getSnapshot().phase, 'refresh-required');
});

test('invalid requests fail before transport and disposal fences late results', async () => {
  let calls = 0; const nav = service(async p => { calls++; return projection(p); });
  assert.throws(() => nav.select({ libraryId: 'movies', view: 'item' } as any));
  assert.throws(() => nav.select({ libraryId: 'movies', view: 'show' }));
  assert.throws(() => nav.select(route, { cursor: 'x'.repeat(4097) })); assert.equal(calls, 0);
  const late = deferred<unknown>(); let path = ''; const disposed = service(async p => { path = p; return late.promise; });
  const pending = disposed.select(route); const snapshot = disposed.getSnapshot(); disposed.dispose(); late.resolve(projection(path)); await pending;
  assert.equal(disposed.getSnapshot(), snapshot); assert.throws(() => disposed.refresh(), /disposed/);
});

function homeProjection(path: string, changes: Record<string, unknown> = {}): any {
  const data = projection(path);
  data.scope = { ...data.scope, libraryId: '', libraryKind: 'mixed', view: 'home', entityId: '' };
  data.query = { sort: 'server', direction: 'asc', category: '', q: '', limit: 12, searchMode: 'none' };
  data.navigation = []; data.sorts = []; data.filters = [];
  data.heading = { key: 'home.heading', fallback: 'Home' };
  data.sections = [{ id: 'continue_watching', type: 'rail', heading: { key: 'home.continue_watching', fallback: 'Continue Watching' }, entries: [{ ...entry('movie-b'), libraryId: 'library-b' }, { ...entry('movie-a'), libraryId: 'library-a' }], totalCount: 2, nextCursor: '' }];
  data.featured = { ...entry('chosen-movie'), libraryId: 'library-c', available: true, backdropUrl: '/v1/items/chosen-movie/artwork/backdrop', overview: 'The server-selected synopsis.', playback: { itemId: 'chosen-movie' } };
  return { ...data, ...changes };
}

test('Home uses global endpoint and preserves the cross-library row order', async () => {
  let requested = '';
  const nav = service(async path => { requested = path; return homeProjection(path); });
  await nav.select({ view: 'home' });
  assert.equal(requested, '/v1/content?view=home&limit=12');
  const data = nav.getSnapshot().projection!;
  assert.equal(nav.getSnapshot().phase, 'ready');
  assert.equal(data.scope.libraryId, ''); assert.equal(data.scope.libraryKind, 'mixed');
  assert.deepEqual(data.sections[0].entries.map(e => [e.id, e.libraryId]), [['movie-b','library-b'], ['movie-a','library-a']]);
  // The projection has no featured title any more: Home's hero is named by `/v1/home` (`home.ts`).
  assert.equal((data as {featured?: unknown}).featured, undefined);
});

test('Home accepts no featured media and an explicitly empty authorized catalog', async () => {
  const nav = service(async path => { const d = homeProjection(path); delete d.featured; d.sections = []; d.empty = { key: 'home.empty', fallback: 'No authorized content.' }; return d; });
  await nav.select({ view: 'home' });
  assert.equal(nav.getSnapshot().phase, 'ready'); assert.equal(nav.getSnapshot().projection!.featured, undefined);
  assert.deepEqual(nav.getSnapshot().sections, []); assert.equal(nav.getSnapshot().projection!.empty!.key, 'home.empty');
});

test('Home validates its entries\' source library and its global scope', async () => {
  for (const mutate of [
    (d: any) => { delete d.sections[0].entries[0].libraryId; },
    (d: any) => { d.scope.libraryId = 'not-global'; }, (d: any) => { d.scope.serverId = 'another-server'; },
  ]) {
    const nav = service(async path => { const d = homeProjection(path); mutate(d); return d; }); await nav.select({ view: 'home' });
    assert.equal(nav.getSnapshot().phase, 'error'); assert.equal(nav.getSnapshot().error!.code, 'invalid_projection');
    assert.equal(nav.getSnapshot().projection, null);
  }
});

test('global-to-library switching fences late Home hero and does not send global cursors to libraries', async () => {
  const late = deferred<unknown>(); let homePath = ''; const paths: string[] = [];
  const nav = service(async path => { paths.push(path); if (path.startsWith('/v1/content?')) { homePath = path; return late.promise; } return projection(path); });
  const home = nav.select({ view: 'home' }); await nav.select(route); await home;
  late.resolve(homeProjection(homePath)); await Promise.resolve();
  assert.equal(nav.getSnapshot().projection!.scope.libraryId, 'movies'); assert.equal(nav.getSnapshot().projection!.featured, undefined);
  assert.ok(!paths.at(-1)!.includes('cursor='));
});

test('Home scope replacement clears hero immediately and denies stale or unauthorized responses', async () => {
  const nav = service(async path => homeProjection(path)); await nav.select({ view: 'home' });
  nav.setScope({ ...scope, viewerId: 'other-profile' }, { request: async () => { throw Object.assign(new Error('Permission denied'), { status: 403, code: 'permission_denied' }); } });
  assert.equal(nav.getSnapshot().projection, null);
  await nav.select({ view: 'home' }); assert.equal(nav.getSnapshot().phase, 'error'); assert.equal(nav.getSnapshot().sections.length, 0);
  assert.throws(() => nav.select({ view: 'home', libraryId: 'library' } as any), /Home does not accept/);
  assert.throws(() => nav.select({ view: 'home', q: 'client-ranked' }), /Home does not accept/);
});


test('same-destination refresh retains bounded content until replacement; destination and permission changes clear it', async () => {
  const response = deferred<unknown>(); let count = 0;
  const nav = service(async path => ++count === 1 ? projection(path) : response.promise);
  await nav.select(route);
  const original = nav.getSnapshot().projection;
  const refresh = nav.refresh();
  assert.equal(nav.getSnapshot().phase, 'loading');
  assert.equal(nav.getSnapshot().projection, original);
  nav.setScope({...scope, viewerId:'other'}, {request:async () => {throw Object.assign(new Error('Denied'), {status:403});}});
  assert.equal(nav.getSnapshot().projection, null);
  response.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40'));
  await refresh;
  assert.equal(nav.getSnapshot().projection, null);
});

test('temporary refresh failure retains content but invalid and denied responses do not', async () => {
  for (const failure of [Object.assign(new Error('Unavailable'), {status:503}), new TypeError('Network failed'), Object.assign(new Error('Denied'), {status:403}), Object.assign(new Error('Invalid'), {code:'invalid_projection'})]) {
    let fail = false;
    const nav = service(async path => {if(fail)throw failure;return projection(path);});
    await nav.select(route); fail = true;
    await nav.refresh();
    assert.equal(nav.getSnapshot().phase,'error');
    assert.equal(nav.getSnapshot().sections.length, failure instanceof TypeError || 'status' in failure && failure.status===503 ? 1 : 0);
    nav.dispose();
  }
});

test('entity identity and art are independent of page rows and fenced to the requested library and entity',async()=>{
 const target:ContentRoute={libraryId:'music',view:'album',entityId:'release'};
 for(const change of [null,{id:'other'},{libraryId:'private'},{kind:'book'},{playback:{itemId:'track'}}]){
  const nav=service(async path=>projection(path,{entity:{id:'release',libraryId:'music',kind:'album',title:'Release',subtitle:'Artist',posterUrl:'/cover',backdropUrl:'/backdrop',count:205,...change},sections:[]}));
  await nav.select(target);
  if(change===null){assert.equal(nav.getSnapshot().phase,'ready');assert.equal(nav.getSnapshot().projection?.entity?.count,205);assert.equal(nav.getSnapshot().projection?.entity?.backdropUrl,'/backdrop');}
  else assert.equal(nav.getSnapshot().phase,'error');
  nav.dispose();
 }
});

test('M20: content sections report their start index, defaulting to 0', async () => {
  const opened = service(async path => { const data = projection(path); data.sections[0].start = 40; data.sections[0].totalCount = 100; return data; });
  await opened.select(route);
  assert.equal(opened.getSnapshot().phase, 'ready');
  assert.equal(opened.getSnapshot().sections[0].start, 40);
  assert.equal(opened.getSnapshot().sections[0].start ?? 0, 40);
  opened.dispose();
  // Sections without a start (older servers) still parse; readers see 0.
  const missing = service(async path => projection(path));
  await missing.select(route);
  assert.equal(missing.getSnapshot().phase, 'ready');
  assert.equal(missing.getSnapshot().sections[0].start, undefined);
  assert.equal(missing.getSnapshot().sections[0].start ?? 0, 0);
  missing.dispose();
  // An unknown start shape is dropped, never fatal.
  for (const start of ['40', -1, 1.5, NaN, {}]) {
    const odd = service(async path => { const data = projection(path); data.sections[0].start = start; return data; });
    await odd.select(route);
    assert.equal(odd.getSnapshot().phase, 'ready', JSON.stringify(start));
    assert.equal(odd.getSnapshot().sections[0].start, undefined, JSON.stringify(start));
    odd.dispose();
  }
});

test('M24: album artist link, release roles and category artworkPaths parse as optional, unknown values dropped', async () => {
  // Album entity with the new facts.
  const album = service(async path => projection(path, {
    entity: { id: 'album-1', libraryId: 'music', kind: 'album', title: 'Blue', subtitle: 'Joni Mitchell', year: 1971, duration: 2160, artist: { id: 'artist-1', name: 'Joni Mitchell' } },
    sections: [],
  }));
  await album.select({ libraryId: 'music', view: 'album', entityId: 'album-1' });
  assert.equal(album.getSnapshot().phase, 'ready');
  assert.deepEqual(album.getSnapshot().projection!.entity!.artist, { id: 'artist-1', name: 'Joni Mitchell' });
  assert.equal(album.getSnapshot().projection!.entity!.year, 1971);
  album.dispose();
  // Artist releases carry roles; category rows carry mosaic paths.
  const artist = service(async path => projection(path, {
    sections: [
      { id: 'songs', type: 'list', heading: { key: 'songs', fallback: 'Songs' }, entries: [{ id: 'song-1', kind: 'song', title: 'River', playback: { itemId: 'song-1' } }], totalCount: 1, nextCursor: '' },
      { id: 'popularTracks', type: 'list', heading: { key: 'popular', fallback: 'Popular tracks' }, entries: [{ id: 'song-1', kind: 'song', title: 'River', playback: { itemId: 'song-1' } }], totalCount: 1, nextCursor: '' },
      { id: 'releases', type: 'grid', heading: { key: 'releases', fallback: 'Releases & appearances' }, entries: [{ id: 'album-1', kind: 'album', title: 'Blue', role: 'album' }, { id: 'album-2', kind: 'album', title: 'Guest spot', role: 'appearance' }], totalCount: 2, nextCursor: '' },
    ],
  }));
  await artist.select({ libraryId: 'music', view: 'artist', entityId: 'artist-1' });
  assert.equal(artist.getSnapshot().phase, 'ready');
  const releases = artist.getSnapshot().sections.find(s => s.id === 'releases')!;
  assert.deepEqual(releases.entries.map(e => [e.id, e.role]), [['album-1', 'album'], ['album-2', 'appearance']]);
  artist.dispose();
  // be/pages: an artist entity with three genres, and release types (EP, single, compilation).
  const typed = service(async path => projection(path, {
    entity: { id: 'artist-1', libraryId: 'music', kind: 'artist', title: 'Radiohead', genres: ['Alternative', 'Rock', 'Electronic'] },
    sections: [{ id: 'releases', type: 'grid', heading: { key: 'releases', fallback: 'Releases' }, entries: [
      { id: 'album-1', kind: 'album', title: 'OK Computer', role: 'album', genres: ['Rock', 'Alternative', 'Art rock', 42] },
      { id: 'album-3', kind: 'album', title: 'Airbag', role: 'ep' }, { id: 'album-4', kind: 'album', title: 'Creep', role: 'single' },
      { id: 'album-5', kind: 'album', title: 'Best of', role: 'compilation' }, { id: 'album-6', kind: 'album', title: 'Odd', role: 'bootleg' }], totalCount: 5, nextCursor: '' }],
  }));
  await typed.select({ libraryId: 'music', view: 'artist', entityId: 'artist-1' });
  assert.equal(typed.getSnapshot().phase, 'ready');
  assert.deepEqual([...typed.getSnapshot().projection!.entity!.genres!], ['Alternative', 'Rock', 'Electronic']);
  const typedReleases = typed.getSnapshot().sections.find(s => s.id === 'releases')!;
  assert.deepEqual(typedReleases.entries.map(e => [e.id, e.role]), [['album-1', 'album'], ['album-3', 'ep'], ['album-4', 'single'], ['album-5', 'compilation'], ['album-6', undefined]]);
  assert.deepEqual([...typedReleases.entries[0].genres!], ['Rock', 'Alternative', 'Art rock']);
  typed.dispose();
  const categories = service(async path => projection(path, {
    sections: [{ id: 'categories', type: 'grid', heading: { key: 'categories', fallback: 'Categories' }, entries: [{ id: 'genre:Rock', kind: 'category', title: 'Rock', artworkPaths: ['/a', '/b', '/c', '/d'], navigation: { view: 'browse', category: 'genre:Rock' } }], totalCount: 1, nextCursor: '' }],
  }));
  await categories.select({ libraryId: 'movies', view: 'categories' });
  assert.equal(categories.getSnapshot().phase, 'ready');
  assert.deepEqual([...categories.getSnapshot().sections[0].entries[0].artworkPaths!], ['/a', '/b', '/c', '/d']);
  categories.dispose();
  // Unknown or malformed new values are dropped, never fatal (older/newer servers both parse).
  for (const patch of [
    { role: 'bootleg' },
    { role: 42 },
    { artist: { id: '', name: '' } },
    { artist: 'Joni' },
    { artworkPaths: 'not-an-array' },
    { artworkPaths: [42, null, ''] },
    { artworkPaths: ['/a', '/b', '/c', '/d', '/e', '/f'] },
  ]) {
    const odd = service(async path => {
      const data = projection(path);
      Object.assign(data.sections[0].entries[0], patch);
      return data;
    });
    await odd.select(route);
    assert.equal(odd.getSnapshot().phase, 'ready', JSON.stringify(patch));
    const row = odd.getSnapshot().sections[0].entries[0];
    if ('role' in patch) assert.equal(row.role, undefined, JSON.stringify(patch));
    if ('artist' in patch) assert.equal(row.artist, undefined, JSON.stringify(patch));
    if (JSON.stringify(patch).includes('artworkPaths')) {
      if (Array.isArray((patch as { artworkPaths?: unknown }).artworkPaths)) {
        // Non-string entries are dropped; more than 4 are truncated to 4.
        const kept = row.artworkPaths ?? [];
        assert.ok(kept.length <= 4, JSON.stringify(patch));
        assert.ok([...kept].every(p => typeof p === 'string' && p.length > 0), JSON.stringify(patch));
      } else {
        assert.equal(row.artworkPaths, undefined, JSON.stringify(patch));
      }
    }
    odd.dispose();
  }
});
