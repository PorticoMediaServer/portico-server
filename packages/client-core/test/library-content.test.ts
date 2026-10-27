import {wireFixture} from './wire-fixture.ts';
import test from 'node:test';
import assert from 'node:assert/strict';
import { LibraryContentService, type ContentProjection, type ContentRoute, type LibraryContentApi } from '../src/library-content.ts';
import { resetSkippedParts, skippedParts } from '../src/tolerant.ts';
const scope = { viewerId: JSON.stringify(['hosted','account','profile']), serverId: 'server' };
const route: ContentRoute = { libraryId: 'movies', view: 'browse' };
function entry(id = 'item') { return { id, kind: 'movie', title: `Server title ${id}`, navigation: { view: 'item', entityId: id } }; }
function projection(path: string, changes: Record<string, unknown> = {}): any {
  const u = new URL(path, 'https://server.test');
  return { heading: { key: 'library.title', fallback: 'Named Library' }, scope: { serverId: 'server', libraryId: decodeURIComponent(u.pathname.split('/')[3]), libraryKind: 'movie', view: u.searchParams.get('view'), entityId: u.searchParams.get('entityId') ?? '', viewerFence: 'fence' }, revision: { catalog: 3, viewer: 4 }, query: { sort: u.searchParams.get('sort') ?? 'title', direction: u.searchParams.get('direction') ?? 'asc', category: u.searchParams.get('category') ?? '', q: u.searchParams.get('q') ?? '', limit: Number(u.searchParams.get('limit')), searchMode: 'title_prefix' }, navigation: [{ id: 'discover', labelKey: 'library.discover', view: 'discover' }, { id: 'browse', labelKey: 'library.browse', view: 'browse' }], sorts: [{ id: 'title', labelKey: 'sort.title', directions: ['asc','desc'] }], filters: [{ id: 'category', labelKey: 'filter.category', options: [{ id: 'decade:1990', label: '1990s', count: 5 }] }], sections: [{ id: 'server-grid', type: 'grid', heading: { key: 'server.heading', fallback: 'Server-selected heading' }, entries: [entry()], totalCount: 1, nextCursor: '' }], ...changes };
}
function service(request: (path: string, signal?: AbortSignal) => Promise<unknown>, timeoutMs = 1000) {
  return new LibraryContentService({ api: { request: <T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => request(path, signal).then(v=>wireFixture(v,{'duration': 'durationMs', 'progressSeconds': 'progressMs', 'startSeconds': 'startMs', 'positionSeconds': 'positionMs'})) as Promise<T> }, scope, timeoutMs });
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

test('synopsis support does not relax unsafe controls: unsafe text is never shown, and only its own part is left out', async () => {
  const read = async (fields: Record<string, unknown>) => {
    const nav = service(async path => {
      const result = projection(path);
      result.sections[0].entries = [{...entry('unsafe'), ...fields}, entry('safe')];
      result.sections[0].totalCount = 2;
      return result;
    });
    await nav.select(route);
    assert.equal(nav.getSnapshot().phase, 'ready');
    const entries = nav.getSnapshot().sections[0].entries;
    nav.dispose();
    return entries;
  };
  // An optional field that cannot be read is left out and its entry is kept.
  for (const fields of [{overview:'Unsafe\u0000text'}, {overview:'Unsafe\u000btext'}, {overview:'Unsafe\u007ftext'}, {overview:'x'.repeat(16385)}, {overview:42}]) {
    const entries = await read(fields);
    assert.deepEqual(entries.map(e => e.id), ['unsafe', 'safe']);
    assert.equal(entries[0].overview, undefined);
  }
  const art = await read({overview:'Valid\ntext',posterUrl:'https://example.test/\nimage'});
  assert.deepEqual([art[0].overview, art[0].posterUrl], ['Valid\ntext', undefined]);
  // A title with a control character is never shown: that entry alone is left out.
  assert.deepEqual((await read({overview:'Valid\r\ntext',title:'Invalid\nname'})).map(e => e.id), ['safe']);
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

test('a continuation the server no longer honours restarts the list from its first page without a refresh state', async () => {
  for (const code of ['stale_continuation', 'invalid_cursor']) {
    const paths: string[] = []; const phases: string[] = [];
    const nav = service(async path => {
      paths.push(path);
      if (new URL(path, 'https://server.test').searchParams.has('cursor')) throw Object.assign(new Error('changed'), { code, status: code === 'invalid_cursor' ? 400 : 409, retryable: true });
      const data = projection(path); data.sections[0].nextCursor = 'old'; return data;
    });
    await nav.select(route);
    nav.subscribe(() => phases.push(nav.getSnapshot().phase + ':' + nav.getSnapshot().sections.length));
    await nav.next('server-grid');
    // The person never sees "the library changed": what was shown stays until the first page is back.
    assert.equal(nav.getSnapshot().phase, 'ready', code); assert.equal(nav.getSnapshot().error, null);
    assert.ok(!phases.some(p => p.startsWith('refresh-required') || p.startsWith('error')), phases.join());
    assert.ok(phases.every(p => !p.endsWith(':0')), 'the list was never emptied');
    assert.equal(paths.length, 3); assert.ok(paths[1].includes('cursor=old')); assert.ok(!paths[2].includes('cursor='));
    assert.equal(nav.getSnapshot().pagination.cursor, null);
  }
});

test('a continuation read after the library moved is still a page; only another viewer\'s answer restarts the list', async () => {
  for (const mutate of [(d: any) => { d.revision.catalog++; }, (d: any) => { d.revision.viewer++; }]) {
    const nav = service(async path => { const data = projection(path); data.sections[0].nextCursor = 'next'; if (path.includes('cursor=')) { mutate(data); data.sections[0].nextCursor = ''; } return data; });
    await nav.select(route); await nav.next('server-grid');
    assert.equal(nav.getSnapshot().phase, 'ready'); assert.equal(nav.getSnapshot().pagination.cursor, 'next');
  }
  let calls = 0;
  const nav = service(async path => { calls++; const data = projection(path); data.sections[0].nextCursor = 'next'; if (path.includes('cursor=')) data.scope.viewerFence = 'other-viewer'; return data; });
  await nav.select(route); await nav.next('server-grid');
  assert.equal(nav.getSnapshot().phase, 'ready'); assert.equal(nav.getSnapshot().pagination.cursor, null); assert.equal(calls, 3);
});

test('rejects wrong scope, oversized lists, a wrong revision and a wrong continuation section', async () => {
  for (const mutate of [
    (d: any) => { d.scope.libraryId = 'wrong'; }, (d: any) => { d.scope.serverId = 'wrong'; }, (d: any) => { d.scope.viewerFence = ''; },
    (d: any) => { d.sections[0].entries = Array.from({ length: 101 }, (_, i) => entry(String(i))); d.sections[0].totalCount = 101; },
    (d: any) => { d.sections = Array.from({ length: 25 }, (_, i) => ({ ...d.sections[0], id: 's' + i })); }, (d: any) => { d.sections = null; },
    (d: any) => { d.navigation = Array.from({ length: 17 }, (_, i) => ({ id: 'n' + i, labelKey: 'k', view: 'browse' })); },
    (d: any) => { d.revision.catalog = -1; }, (d: any) => { d.query.limit = 1000; }, (d: any) => { d.query.sort = 'other'; },
  ]) {
    const nav = service(async p => { const data = projection(p); mutate(data); return data; }); await nav.select({ ...route, sort: 'title' });
    assert.equal(nav.getSnapshot().phase, 'error'); assert.equal(nav.getSnapshot().error!.code, 'invalid_projection'); assert.equal(nav.getSnapshot().sections.length, 0);
  }
  const nav = service(async p => { const d = projection(p); d.sections[0].nextCursor = 'next'; if (p.includes('cursor=')) d.sections[0].id = 'wrong-section'; return d; });
  await nav.select(route); await nav.next('server-grid'); assert.equal(nav.getSnapshot().error!.code, 'invalid_projection');
});

test('one unreadable row of a projection is left out and the rest of the page is kept', async () => {
  const read = async (mutate: (d: any) => void) => {
    const nav = service(async p => { const data = projection(p); mutate(data); return data; }); await nav.select(route);
    assert.equal(nav.getSnapshot().phase, 'ready'); const result = nav.getSnapshot().projection!; nav.dispose(); return result;
  };
  resetSkippedParts();
  // Entries: a broken one, a repeated id, one of another library and one whose destination this app does not know.
  const rows = await read(d => { d.sections[0].entries = [entry('a'), entry('a'), { id: 'broken' }, { ...entry('b'), libraryId: 'other' }, { ...entry('c'), navigation: { view: 'hologram' } }, { ...entry('d'), duration: Number.NaN, year: 0 }, entry('e')]; d.sections[0].totalCount = 2; });
  assert.deepEqual(rows.sections[0].entries.map(e => e.id), ['a', 'd', 'e']);
  assert.equal(rows.sections[0].entries[1].duration, undefined);
  assert.equal(rows.sections[0].totalCount, 3, 'the count is at least the entries kept');
  assert.deepEqual(skippedParts().filter(part => part.part === 'entries'), [{ document: 'content', part: 'entries', count: 4 }]);
  // A page none of whose sections can be read is refused rather than shown as empty.
  const none = service(async p => { const d = projection(p); d.sections = [{ ...d.sections[0], id: '' }, 'nope']; return d; }); await none.select(route);
  assert.equal(none.getSnapshot().error?.code, 'invalid_projection'); none.dispose();
  // Sections: a repeated id keeps the first, a broken one is left out, and a layout this app does not know is a grid.
  const sections = await read(d => { const s = d.sections[0]; d.sections = [s, { ...s, heading: { key: 'x', fallback: 'Second' } }, { ...s, id: '' }, { ...s, id: 'cursor', nextCursor: 7 }, { ...s, id: 'mosaic', type: 'mosaic', heading: 7 }, 'nope']; });
  assert.deepEqual(sections.sections.map(s => [s.id, s.type, s.heading.fallback]), [['server-grid', 'grid', 'Server-selected heading'], ['mosaic', 'grid', '']]);
  // Tabs, sorts, filters and their options.
  const menus = await read(d => {
    d.navigation = [...d.navigation, { id: 'future', labelKey: 'library.future', view: 'hologram' }, { id: 'browse', labelKey: 'again', view: 'browse' }, null];
    d.sorts = [{ id: 'title', labelKey: 'sort.title', directions: ['asc', 'sideways'] }, { id: 'bad' }, { id: 'long', labelKey: 'x', directions: ['asc', 'desc', 'asc'] }, { id: 'title', labelKey: 'again', directions: ['desc'] }];
    d.filters = [{ id: 'category', labelKey: 'filter.category', options: [{ id: 'a', label: 'A', count: 1 }, { id: 'a', label: 'Again', count: 2 }, { id: 'b' }, { label: 'no id' }] }, { id: 'broken', labelKey: 'x', options: 'nope' }];
  });
  assert.deepEqual(menus.navigation.map(n => n.id), ['discover', 'browse']);
  assert.deepEqual(menus.sorts, [{ id: 'title', labelKey: 'sort.title', directions: ['asc'] }]);
  assert.deepEqual(menus.filters, [{ id: 'category', labelKey: 'filter.category', options: [{ id: 'a', label: 'A', count: 1 }, { id: 'b', label: '', count: 0 }] }]);
  // Lists the server leaves out read as empty.
  const bare = await read(d => { d.navigation = null; delete d.sorts; delete d.filters; });
  assert.deepEqual([bare.navigation.length, bare.sorts.length, bare.filters.length, bare.sections.length], [0, 0, 0, 1]);
});

test('an unknown search mode, an unreadable heading and unknown fields do not spoil the page', async () => {
  const nav = service(async p => { const d = projection(p); d.query.searchMode = 'semantic'; d.heading = { key: '' }; d.empty = 'nothing'; d.listening = { target: 7 }; d.future = { a: 1 }; d.sections[0].future = true; d.sections[0].entries[0].future = [1]; d.sections[0].seeAll = { pivot: '' }; return d; });
  await nav.select(route);
  assert.equal(nav.getSnapshot().phase, 'ready');
  const result = nav.getSnapshot().projection!;
  assert.equal(result.query.searchMode, 'none');
  assert.deepEqual(result.heading, { key: 'browse', fallback: '' });
  assert.equal(result.empty, undefined);
  assert.equal(result.listening, undefined);
  assert.equal(result.sections[0].seeAll, undefined);
  assert.equal('future' in result || 'future' in result.sections[0] || 'future' in result.sections[0].entries[0], false);
  nav.dispose();
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

test('continuation history is bounded and a cyclic cursor ends the section', async () => {
  let page = 0;
  const nav = service(async p => { const data = projection(p); data.sections[0].nextCursor = `page-${++page}`; return data; });
  await nav.select(route); for (let i = 0; i < 80; i++) await nav.next('server-grid');
  let backs = 0; while (nav.getSnapshot().pagination.canPrevious) { await nav.previous(); backs++; }
  assert.equal(backs, 63);
  // A cursor that comes back again is the end of that section, never a loop and never a refresh state.
  const cycle = service(async p => { const d = projection(p); d.sections[0].nextCursor = 'same'; return d; });
  await cycle.select(route); await cycle.next('server-grid'); assert.equal(cycle.getSnapshot().phase, 'ready');
  assert.equal(cycle.getSnapshot().sections[0].nextCursor, ''); assert.deepEqual(cycle.getSnapshot().pagination.next, []);
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

test('Home is read through the composed Home API, not the library content service',()=>{
 const nav=service(async path=>projection(path));
 assert.throws(()=>nav.select({view:'home'} as any),/Invalid library content route/);
 assert.throws(()=>nav.select({libraryId:'movies',view:'home'} as any),/Invalid library content route/);
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
  if(change===null||'playback' in change){assert.equal(nav.getSnapshot().phase,'ready');assert.equal(nav.getSnapshot().projection?.entity?.count,205);assert.equal(nav.getSnapshot().projection?.entity?.backdropUrl,'/backdrop');assert.equal(nav.getSnapshot().projection?.entity?.playback,undefined,'the page\'s own entity never plays');}
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

test('a quiet read replaces the first page in place with no loading state, and leaves a failed read invisible', async () => {
  let ids = ['one', 'two'], fail = false;
  const nav = service(async path => { if (fail) throw Object.assign(new Error('down'), { status: 503 }); const data = projection(path); data.sections[0].entries = ids.map(entry); data.sections[0].totalCount = ids.length; return data; });
  await nav.select(route);
  const phases: string[] = []; nav.subscribe(() => phases.push(nav.getSnapshot().phase));
  ids = ['new', 'one', 'two'];
  await nav.revalidate();
  assert.deepEqual(nav.getSnapshot().sections[0].entries.map(e => e.id), ['new', 'one', 'two']);
  assert.deepEqual(phases, ['ready'], 'never loading, never empty');
  assert.equal(nav.getSnapshot().head!.serial, 1);
  const shown = nav.getSnapshot();
  fail = true;
  await assert.rejects(nav.revalidate(), { status: 503 });
  assert.equal(nav.getSnapshot(), shown);
  // The person may be mid-press: the answer waits for the pause it is given.
  fail = false; ids = ['later'];
  let release!: () => void; const pause = new Promise<void>(r => { release = r; });
  const quiet = nav.revalidate({ settle: () => pause });
  await new Promise(r => setTimeout(r, 5));
  assert.equal(nav.getSnapshot(), shown);
  release(); await quiet;
  assert.deepEqual(nav.getSnapshot().sections[0].entries.map(e => e.id), ['later']);
});

test('a quiet read while on a later page keeps that page and publishes the fresh first page as head', async () => {
  const nav = service(async path => { const data = projection(path); const later = path.includes('cursor='); data.sections[0].entries = [entry(later ? 'page-two' : 'page-one')]; data.sections[0].nextCursor = later ? '' : 'next'; return data; });
  await nav.select(route); await nav.next('server-grid');
  assert.equal(nav.getSnapshot().sections[0].entries[0].id, 'page-two');
  await nav.revalidate();
  assert.equal(nav.getSnapshot().sections[0].entries[0].id, 'page-two', 'the page the service is on is untouched');
  assert.equal(nav.getSnapshot().pagination.cursor, 'next');
  assert.equal(nav.getSnapshot().head!.sections[0].entries[0].id, 'page-one');
  // A title that is gone, or access that was removed, becomes the screen's own state through an ordinary load.
  const gone = service(async path => { if (path.includes('gone=')) throw Object.assign(new Error('gone'), { status: 404, code: 'not_found' }); return projection(path); });
  await gone.select(route);
  (gone as any).api = { request: async () => { throw Object.assign(new Error('gone'), { status: 404, code: 'not_found', retryable: false }); } };
  await gone.revalidate();
  assert.equal(gone.getSnapshot().phase, 'error'); assert.equal(gone.getSnapshot().error!.code, 'not_found');
});

test('everything read so far is one list per section: later pages append, More works on any section, and a quiet read merges in place', async () => {
  const shelf = (id: string, ids: string[], nextCursor = '') => ({ id, type: 'rail', heading: { key: id, fallback: id }, entries: ids.map(entry), totalCount: 9, nextCursor });
  let first = [shelf('a', ['a1', 'a2'], 'a-next'), shelf('b', ['b1', 'b2'], 'b-next')];
  const nav = service(async path => {
    const cursor = new URL(path, 'https://server.test').searchParams.get('cursor');
    if (cursor === 'a-next') return projection(path, { sections: [shelf('a', ['a3', 'a2'], '')] });
    if (cursor === 'b-next') return projection(path, { sections: [shelf('b', ['b3'], '')] });
    return projection(path, { sections: first });
  });
  await nav.select({ libraryId: 'movies', view: 'discover' });
  await nav.next('a');
  const ids = () => nav.getSnapshot().appended.map(s => s.entries.map(e => e.id).join(','));
  assert.deepEqual(ids(), ['a1,a2,a3', 'b1,b2'], 'the page is appended to its own section, a repeated entry is dropped, the other section stays');
  // The service is on a page of section a; More on section b still works.
  await nav.next('b');
  assert.deepEqual(ids(), ['a1,a2,a3', 'b1,b2,b3']);
  const untouched = nav.getSnapshot().appended[1];
  // A new title arrives at the front of section a while the screen is open.
  first = [shelf('a', ['new', 'a1'], 'a-next-2'), shelf('b', ['b1', 'b2'], 'b-next')];
  await nav.revalidate();
  assert.deepEqual(ids(), ['new,a1,a2,a3', 'b1,b2,b3'], 'the fresh first page leads; what was scrolled into stays');
  assert.equal(nav.getSnapshot().appended[1], untouched, 'a section that did not change is the same object');
  assert.equal(nav.getSnapshot().appended[0].nextCursor, '', 'the section continues from where the person was (its end)');
});


test('author and series heroes are exact entities in their own library',async()=>{
 for(const view of ['author','book_series'] as const){
  const target:ContentRoute={libraryId:'books',view,entityId:'group'};
  for(const change of [null,{id:'other'},{libraryId:'private'},{kind:'book'}]){
   const nav=service(async path=>projection(path,{entity:{id:'group',libraryId:'books',kind:view,title:'Writer',...change},sections:[]}));
   await nav.select(target);
   assert.equal(nav.getSnapshot().phase,change===null?'ready':'error');
   if(change===null)assert.equal(nav.getSnapshot().projection?.entity?.title,'Writer');
   nav.dispose();
  }
 }
});

test('refresh-required policy keeps committed rows for an explicit current retry',async()=>{
 let calls=0; const ctx={signal:new AbortController().signal,isCurrent:()=>true};
 const service=new LibraryContentService({scope,continuationRecovery:'refresh-required',api:{request:async (path:string)=>{calls++; if(new URL(path,'https://x').searchParams.get('cursor'))throw Object.assign(new Error('stale'),{code:'stale_continuation'}); return projection(path,{sections:[{...projection(path).sections[0],nextCursor:'next'}]});}}});
 await service.select(route,{context:ctx});
 assert.equal(calls,1);
 await service.next('server-grid',ctx);
 assert.equal(calls,2,'the refused continuation is not retried as page one');
 assert.equal(service.getSnapshot().phase,'refresh-required');
 assert.equal(service.getSnapshot().error?.code,'stale_continuation');
 assert.equal(service.getSnapshot().sections[0]!.entries[0]!.id,'item','committed rows stay visible');
 assert.deepEqual(service.getSnapshot().pagination.next,[]);
 await service.retry(ctx);
 assert.equal(calls,3,'explicit current retry starts page one');
 assert.equal(service.getSnapshot().phase,'ready');
 service.dispose();
});

test('an original-opening abort retires only its flight and restores the committed baseline',async()=>{
 const initial=deferred<unknown>(); const late=deferred<unknown>(); let calls=0, signal:AbortSignal|undefined;
 const service=new LibraryContentService({scope,api:{request:async (path:string,_m?:string,_b?:unknown,s?:AbortSignal)=>{calls++;signal=s;return calls===1?initial.promise:late.promise;}}});
 const first=service.select(route); initial.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],nextCursor:'next'}]})); await first;
 const controller=new AbortController(); const ctx={signal:controller.signal,isCurrent:()=>true};
 const pending=service.next('server-grid',ctx); await new Promise(r=>setImmediate(r));
 controller.abort();
 await pending;
 assert.equal(signal?.aborted,true,'the HTTP controller is aborted');
 assert.equal(service.getSnapshot().phase,'ready');
 assert.equal(service.getSnapshot().sections[0]!.entries[0]!.id,'item');
 assert.equal(service.getSnapshot().error,null);
 late.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40&cursor=next'));
 await new Promise(r=>setImmediate(r));
 assert.equal(service.getSnapshot().sections[0]!.entries[0]!.id,'item','a late success cannot publish');
 service.dispose();
});

test('same key with a new opening context is isolated; duplicate same opening coalesces',async()=>{
 const reads:ReturnType<typeof deferred<unknown>>[]=[]; let calls=0;
 const service=new LibraryContentService({scope,api:{request:async ()=>{calls++;const d=deferred<unknown>();reads.push(d);return d.promise;}}});
 const c1={signal:new AbortController().signal,isCurrent:()=>true},c2={signal:new AbortController().signal,isCurrent:()=>true};
 const a=service.select(route,{context:c1}), b=service.select(route,{context:c1});
 assert.strictEqual(a,b,'the same opening and key coalesce');
 assert.equal(calls,1);
 const c=service.select(route,{context:c2});
 assert.notStrictEqual(c,a,'a new opening does not inherit the old promise');
 assert.equal(calls,2);
 reads[0]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],entries:[entry('old')]}]}));
 reads[1]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],entries:[entry('new')]}]}));
 await Promise.allSettled([a,c]);
 assert.equal(service.getSnapshot().sections[0]!.entries[0]!.id,'new');
 service.dispose();
});

test('a synchronous loading subscriber can coalesce the same opening without recursion',async()=>{
 let calls=0, nested:Promise<void>|undefined;
 const ctx={signal:new AbortController().signal,isCurrent:()=>true};
 const nav=new LibraryContentService({scope,api:{request:async(path:string)=>{calls++;return projection(path,{sections:[{...projection(path).sections[0],nextCursor:'next'}]});}}});
 nav.subscribe(()=>{if(nav.getSnapshot().phase==='loading'&&!nested)nested=nav.select(route,{context:ctx});});
 await nav.select(route,{context:ctx});
 if(nested)await nested;
 assert.equal(calls,1,'the nested same-opening call joins the established flight');
 assert.equal(nav.getSnapshot().phase,'ready');
 nav.dispose();
});

test('isCurrent-only retirement after a successful response restores the committed baseline',async()=>{
 const response=deferred<unknown>(); let current=true,calls=0;
 const nav=new LibraryContentService({scope,api:{request:async(path:string)=>{calls++;return calls===1?projection(path,{sections:[{...projection(path).sections[0],nextCursor:'next'}]}):response.promise;}}});
 const ctx={signal:new AbortController().signal,isCurrent:()=>current};
 await nav.select(route,{context:ctx});
 const pending=nav.next('server-grid',ctx);
 current=false;
 response.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40&cursor=next',{sections:[{...projection('/x').sections[0],entries:[entry('late')],nextCursor:''}]}));
 await pending;
 assert.equal(nav.getSnapshot().phase,'ready','the retired flight restores rather than stranding loading');
 assert.equal(nav.getSnapshot().sections[0]!.entries[0]!.id,'item');
 nav.dispose();
});

test('aborting the predecessor after a successor opening installs cannot erase it',async()=>{
 const reads:ReturnType<typeof deferred<unknown>>[]=[]; let calls=0;
 const nav=new LibraryContentService({scope,api:{request:async()=>{calls++;const d=deferred<unknown>();reads.push(d);return d.promise;}}});
 const first=nav.select(route,{context:{signal:new AbortController().signal,isCurrent:()=>true}});
 reads[0]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40'));
 await first;
 const c1=new AbortController(), c2=new AbortController();
 const older=nav.select(route,{context:{signal:c1.signal,isCurrent:()=>true}});
 const newer=nav.select(route,{context:{signal:c2.signal,isCurrent:()=>true}});
 assert.equal(calls,3);
 c1.abort();
 reads[2]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],entries:[entry('successor')]}]}));
 await Promise.allSettled([older,newer]);
 assert.equal(nav.getSnapshot().phase,'ready');
 assert.equal(nav.getSnapshot().sections[0]!.entries[0]!.id,'successor');
 nav.dispose();
});

test('a reentrant currentness predicate cannot overwrite the successor it installs',async()=>{
 const reads:ReturnType<typeof deferred<unknown>>[]=[]; let reenter=false,successor:Promise<void>|undefined;
 const nav=new LibraryContentService({scope,api:{request:async()=>{const d=deferred<unknown>();reads.push(d);return d.promise;}}});
 const ctx={signal:new AbortController().signal,isCurrent:()=>{if(reenter){reenter=false;successor=nav.select({libraryId:'other',view:'browse'});return false;}return true;}};
 const first=nav.select({libraryId:'movies',view:'browse'},{context:ctx});
 reads[0]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],nextCursor:'next'}]}));
 await first;
 const pending=nav.next('server-grid',ctx);
 reenter=true;
 reads[1]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40&cursor=next'));
 await pending;
 assert.ok(successor,'the predicate installed a successor');
 reads[2]!.resolve(projection('/v1/libraries/other/content?view=browse&limit=40'));
 await successor;
 assert.equal(nav.getSnapshot().route?.libraryId,'other','the successor remains current');
 assert.equal(nav.getSnapshot().phase,'ready');
 nav.dispose();
});

test('an abort listener installing a successor during cleanup is not rolled back',async()=>{
 const reads:ReturnType<typeof deferred<unknown>>[]=[];let successor:Promise<void>|undefined;
 const nav=new LibraryContentService({scope,api:{request:async()=>{const d=deferred<unknown>();reads.push(d);return d.promise;}}});
 const first=nav.select(route,{context:{signal:new AbortController().signal,isCurrent:()=>true}});
 reads[0]!.resolve(projection('/v1/libraries/movies/content?view=browse&limit=40',{sections:[{...projection('/x').sections[0],nextCursor:'next'}]}));
 await first;
 const controller=new AbortController();
 const ctx={signal:controller.signal,isCurrent:()=>true};
 // Registered before the flight's own listener, so it runs first during abort.
 controller.signal.addEventListener('abort',()=>{successor=nav.select({libraryId:'other',view:'browse'});},{once:true});
 const pending=nav.next('server-grid',ctx);
 await new Promise(r=>setImmediate(r));
 controller.abort();
 assert.ok(successor,'the abort listener installed a successor');
 reads[2]!.resolve(projection('/v1/libraries/other/content?view=browse&limit=40'));
 await successor;
 await pending;
 assert.equal(nav.getSnapshot().route?.libraryId,'other');
 assert.equal(nav.getSnapshot().phase,'ready');
 nav.dispose();
});
