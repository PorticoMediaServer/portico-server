import test from 'node:test';
import assert from 'node:assert/strict';
import {validateHomeDocument, type HomeApi} from '../src/home.ts';
import {HomeService, mergeHomeDocument} from '../src/home-service.ts';

function entry(id: string, extra: Record<string, unknown> = {}) {
  return {id, kind: 'movie', title: `Title ${id}`, libraryId: 'movies', available: true, navigation: {view: 'item', entityId: id}, playback: {itemId: id}, ...extra};
}
function row(id: string, ids: string[], changes: Record<string, unknown> = {}): any {
  return {
    id, title: id, kind: 'recent', artworkShape: 'poster', endpoint: '/v1/home/rows/' + id, libraryId: 'movies', viewerFence: 'fence', privacySensitivity: 'catalog', policyState: 'available', priority: 100,
    required: false, hideable: true, reorderable: true, critical: false, cursorCapable: true,
    entries: ids.map(i => entry(i)), total: 50, start: 0, limit: ids.length, hasMore: true, nextCursor: 'first.' + id, revision: {catalog: 3, viewer: 1}, ...changes,
  };
}
function document(rows: any[], changes: Record<string, unknown> = {}): any {
  return {serverId: 'server', viewerFence: 'fence', rows, layout: {revision: 2, rowOrder: rows.map(r => r.id), hiddenRowIds: []}, revision: {catalog: 3, viewer: 1}, generatedAt: '2026-09-16T12:00:00.000Z', ...changes};
}
const api = (handler: (path: string, method?: string, body?: any) => unknown): HomeApi => ({request: async <T>(path: string, method?: string, body?: unknown) => handler(path, method, body) as T});
const ids = (service: HomeService, rowId: string) => service.getSnapshot().document!.rows.find(r => r.id === rowId)!.entries.map(e => e.id).join(',');

test('a quiet read updates Home in place: unchanged rows are the same objects, a new card arrives, nothing is blanked', async () => {
  let current = document([row('continue', ['a', 'b']), row('recent', ['m1', 'm2'])]);
  const service = new HomeService(api(() => JSON.parse(JSON.stringify(current))));
  await service.load();
  const before = service.getSnapshot().document!;
  const phases: string[] = [];
  service.subscribe(() => phases.push(service.getSnapshot().phase + ':' + (service.getSnapshot().document ? 'shown' : 'blank')));
  // Nothing moved: the very same document, and no listener is told anything.
  await service.revalidate();
  assert.equal(service.getSnapshot().document, before);
  assert.deepEqual(phases, []);
  // A new film was added, and the server stamps a new revision on every row.
  current = document([row('continue', ['a', 'b'], {revision: {catalog: 4, viewer: 1}}), row('recent', ['new', 'm1'], {revision: {catalog: 4, viewer: 1}})], {revision: {catalog: 4, viewer: 1}});
  await service.revalidate();
  const after = service.getSnapshot().document!;
  assert.equal(after.rows[0], before.rows[0], 'the untouched row keeps its identity, so its cards do not re-render');
  assert.equal(ids(service, 'recent'), 'new,m1,m2', 'the new card pushes the row along; nothing the person was looking at vanishes');
  assert.equal(after.rows[1]!.entries[1], before.rows[1]!.entries[0], 'an unchanged card is the same object');
  assert.deepEqual(phases, ['ready:shown']);
});

test('a row the person paged sideways keeps its tail when the first page is read again', async () => {
  let current = document([row('recent', ['m1', 'm2'])]);
  const service = new HomeService(api(path => {
    if (path.startsWith('/v1/home/rows/recent')) return row('recent', ['m3', 'm4'], {nextCursor: 'second.recent', start: 2});
    return JSON.parse(JSON.stringify(current));
  }));
  await service.load();
  await service.more('recent');
  assert.equal(ids(service, 'recent'), 'm1,m2,m3,m4');
  current = document([row('recent', ['new', 'm1'])]);
  await service.revalidate();
  assert.equal(ids(service, 'recent'), 'new,m1,m2,m3,m4', 'm2 fell off the first page but is still in the row');
  assert.equal(service.getSnapshot().document!.rows[0]!.nextCursor, 'second.recent', 'the row continues from where the person was');
  // The row shrank to a complete, short list (the rest was watched elsewhere): the tail goes.
  current = document([row('recent', ['m1'], {nextCursor: '', hasMore: false, total: 1})]);
  await service.revalidate();
  assert.equal(ids(service, 'recent'), 'm1');
});

test('a failed quiet read changes nothing and rejects; a failed first load is an error with a retry', async () => {
  let fail = false;
  const service = new HomeService(api(() => { if (fail) throw Object.assign(new Error('down'), {status: 503}); return document([row('recent', ['m1'])]); }));
  await service.load();
  const shown = service.getSnapshot();
  fail = true;
  await assert.rejects(service.revalidate(), {status: 503});
  assert.equal(service.getSnapshot(), shown, 'what is on screen is untouched');
  // An explicit refresh that fails keeps the rows too.
  await service.load();
  assert.equal(service.getSnapshot().phase, 'error');
  assert.equal(service.getSnapshot().document, shown.document);
  fail = false;
  await service.retry();
  assert.equal(service.getSnapshot().phase, 'ready');
});

test('a row page the server refuses as out of date brings Home up to date quietly instead of failing', async () => {
  let current = document([row('recent', ['m1', 'm2'])]);
  let rowReads = 0;
  const service = new HomeService(api(path => {
    if (path.startsWith('/v1/home/rows/')) { rowReads++; throw Object.assign(new Error('stale'), {code: 'stale_continuation', status: 409}); }
    return JSON.parse(JSON.stringify(current));
  }));
  await service.load();
  current = document([row('recent', ['new', 'm1'], {nextCursor: 'fresh.recent'})]);
  await service.more('recent');
  assert.equal(rowReads, 1);
  assert.equal(service.getSnapshot().phase, 'ready');
  assert.equal(service.getSnapshot().error, undefined);
  assert.equal(service.getSnapshot().paging.size, 0);
  assert.equal(ids(service, 'recent'), 'new,m1,m2');
});

test('saving the layout after it moved elsewhere reads it again and sends the same arrangement', async () => {
  let revision = 2;
  const writes: any[] = [];
  const service = new HomeService(api((path, method, body) => {
    if (path === '/v1/home/layout' && method === 'PUT') {
      writes.push(body);
      if (body.expectedRevision !== revision) throw Object.assign(new Error('moved'), {code: 'revision_conflict', status: 409});
      revision++;
      return {revision, rowOrder: body.rowOrder, hiddenRowIds: body.hiddenRowIds};
    }
    if (path === '/v1/home/layout') return {revision, rowOrder: ['a', 'b'], hiddenRowIds: [], rows: []};
    return document([row('a', ['m1']), row('b', ['m2'])], {layout: {revision, rowOrder: ['a', 'b'], hiddenRowIds: []}});
  }));
  await service.load();
  revision = 7; // another device rearranged Home
  let key = 0;
  const saved = await service.saveLayout(['b', 'a'], [], () => 'key-' + (++key));
  assert.equal(saved.revision, 8);
  assert.deepEqual(writes.map(w => w.expectedRevision), [2, 7]);
  assert.deepEqual(writes[1].rowOrder, ['b', 'a']);
  assert.equal(service.getSnapshot().phase, 'ready');
});

test('removing a card is immediate and can be put back', async () => {
  const service = new HomeService(api(() => document([row('continue', ['a', 'b']), row('recent', ['a', 'm2'])])));
  await service.load();
  const undo = service.withoutEntry('a');
  assert.deepEqual([ids(service, 'continue'), ids(service, 'recent')], ['b', 'm2']);
  undo();
  assert.deepEqual([ids(service, 'continue'), ids(service, 'recent')], ['a,b', 'a,m2']);
});

test('another viewer or server is never merged into what is shown', () => {
  const shown = validateHomeDocument(document([row('recent', ['m1'])]));
  const other = validateHomeDocument(document([row('recent', ['x'])], {viewerFence: 'someone-else'}));
  assert.equal(mergeHomeDocument(shown, other), other);
});

test('Home publishes fast shelves while another shelf is still loading', async () => {
  let finishSlow!: (value: unknown) => void;
  const slow = new Promise(resolve => { finishSlow = resolve; });
  const service = new HomeService(api(path => {
    if (path === '/v1/home') return document([row('slow', [], {deferred: true, limit: 12}), row('fast', [], {deferred: true, limit: 12}), row('tail', [], {deferred: true, limit: 12})]);
    if (path.includes('/rows/slow')) return slow;
    if (path.includes('/rows/fast')) return row('fast', ['fast-card']);
    if (path.includes('/rows/tail')) return row('tail', ['tail-card']);
    throw new Error(path);
  }));
  await service.load();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(ids(service, 'fast'), 'fast-card');
  assert.equal(ids(service, 'tail'), 'tail-card');
  assert.equal(ids(service, 'slow'), '');
  assert.equal(service.getSnapshot().phase, 'ready');
  finishSlow(row('slow', ['slow-card']));
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(ids(service, 'slow'), 'slow-card');
  service.dispose();
});

test('a deferred quiet refresh keeps previous cards and rejects a row from another viewer', async () => {
  let deferred = false;
  const service = new HomeService(api(path => {
    if (path === '/v1/home') return document([row('recent', deferred ? [] : ['old'], deferred ? {deferred: true, limit: 12} : {})]);
    return row('recent', ['private'], {viewerFence: 'another-viewer'});
  }));
  await service.load();
  const before = service.getSnapshot().document!.rows[0];
  deferred = true;
  await service.revalidate();
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(service.getSnapshot().document!.rows[0], before);
  assert.equal(ids(service, 'recent'), 'old');
  assert.equal(service.getSnapshot().error, undefined);
  service.dispose();
});

test('the server expands the personal family in its published position', async () => {
  const service = new HomeService(api(path => {
    if (path === '/v1/home') return document([row('first', [], {deferred: true, limit: 12}), row('for_you', [], {deferred: true, kind: 'family', limit: 12}), row('last', [], {deferred: true, limit: 12})]);
    if (path.includes('/rows/for_you')) return row('for_you', [], {children: [row('for_you:one', ['one'], {family: 'for_you'}), row('for_you:two', ['two'], {family: 'for_you'})]});
    if (path.includes('/rows/first')) return row('first', ['first']);
    return row('last', ['last']);
  }));
  await service.load();
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(service.getSnapshot().document!.rows.map(row => row.id), ['first', 'for_you:one', 'for_you:two', 'last']);
  service.dispose();
});

test('a row timeout releases its queue slot even if its API ignores cancellation', async () => {
  const never = new Promise(() => {});
  const service = new HomeService(api(path => {
    if (path === '/v1/home') return document([row('one', [], {deferred: true, limit: 12}), row('two', [], {deferred: true, limit: 12}), row('ready', [], {deferred: true, limit: 12})]);
    if (path.includes('/rows/ready')) return row('ready', ['card']);
    return never;
  }), {timeoutMs: 20});
  await service.load();
  await new Promise(resolve => setTimeout(resolve, 50));
  assert.equal(ids(service, 'ready'), 'card');
  assert.equal(service.getSnapshot().error, undefined);
  service.dispose();
});
