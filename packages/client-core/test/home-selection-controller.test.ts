import assert from 'node:assert/strict';
import test from 'node:test';
import {HomeSelectionController, homeSelectionContext} from '../src/home-selection/index.ts';
import type {HomeDocument, HomeEntry, HomeRow, HomeActions} from '../src/home.ts';
import type {HomeService} from '../src/home-service.ts';
import type {HomeSelectionOptions, HomeSelectionPlaybackRequest} from '../src/home-selection/index.ts';
import {routePath} from '../../contracts/src/routes.ts';

const fence = 'a'.repeat(64), revision = 'b'.repeat(64);
let sequence = 0;
const requestId = async () => `00000000-0000-4000-8000-${String(++sequence).padStart(12, '0')}`;
const offers = (patch: Partial<HomeActions> = {}): HomeActions => ({play: true, queue: true, playlist: true, collection: true, myList: true, favorite: true, watched: true, played: false, rating: true, editMetadata: true, refreshMetadata: true, delete: true, download: true, details: true, selection: {play: true, queue: true, playlist: true, collection: true, myList: true, favorite: true, watched: true, download: true, edit: true, refresh: true, delete: true}, state: {watchlisted: false, favorite: false, watched: false, rating: 6, personalRevision: 0, containerRevision: 0}, ...patch});
function entry(id: string, kind: HomeEntry['kind'] = 'movie'): HomeEntry {return {id, libraryId: 'library', kind, title: `Title ${id}`, available: true, playback: {itemId: `play-${id}`}, homeActions: offers()};}
function row(entries: readonly HomeEntry[]): HomeRow {return {id: 'recent', viewerFence: fence, title: 'Recent', kind: 'media', artworkShape: 'poster', endpoint: '', privacySensitivity: '', policyState: '', priority: 0, required: false, hideable: true, reorderable: true, critical: false, cursorCapable: true, entries, total: entries.length, start: 0, limit: entries.length, hasMore: false, nextCursor: '', revision: {catalog: 1, viewer: 1}};}
function harness(entries = [entry('a'), entry('b')], request?: HomeSelectionOptions['api']['request']) {
  let document: HomeDocument = {serverId: 'server', viewerFence: fence, rows: [row(entries)], spotlight: [], layout: {revision: 1, rowOrder: ['recent'], hiddenRowIds: []}, revision: {catalog: 1, viewer: 1}, generatedAt: '2026-10-22T00:00:00Z'};
  const listeners = new Set<() => void>(), played: HomeSelectionPlaybackRequest[] = [], queued: HomeSelectionPlaybackRequest[] = [], errors: unknown[] = [];
  const header = {id: 'queue', revision: '1', total: 2, repeat: 'off' as const, segments: []};
  const options: HomeSelectionOptions = {
    api: {request: request ?? (async () => {throw new Error('Unexpected network request');})},
    home: {getSnapshot: () => ({phase: 'ready', document, paging: new Set()}), subscribe: (listener: () => void) => {listeners.add(listener); return () => listeners.delete(listener);}} as unknown as HomeService,
    events: {on: () => () => {}}, context: homeSelectionContext({serverId: 'server', authority: 'local', accountId: 'owner', profileId: 'profile', viewId: 'home'}, document), environment: {platform: 'desktop-web', isOwner: true, canEditMetadata: true}, isCurrent: () => true, requestId,
    playback: {play: async input => {played.push(input); return {queue: header};}, enqueue: async input => {queued.push(input); return {queue: header};}, playChannel: async () => {throw new Error('Unexpected channel playback');}},
    routes: {openEntry: () => {throw new Error('Unexpected navigation');}, openCollection: () => {throw new Error('Unexpected navigation');}, openDestination: () => {throw new Error('Unexpected navigation');}}, reportError: error => errors.push(error),
  };
  const controller = new HomeSelectionController(options);
  return {controller, options, entries, played, queued, errors, get document() {return document;}, replace(next: HomeDocument) {document = next; for (const listener of listeners) listener();}};
}
function pickAll(h: ReturnType<typeof harness>) {h.controller.registerWindow({id: 'window', row: h.document.rows[0], sectionOrder: 0}); h.controller.selectLoaded();}

test('Home Play follows display order, queue follows pick order, aggregate identity retains actual resume child', async () => {
  const a = entry('show', 'show'), b = entry('movie');
  const h = harness([a, b]), target = entry('episode', 'episode');
  h.replace({...h.document, rows: [{...h.document.rows[0], resumes: [{entryId: a.id, target}]}]});
  h.controller.registerWindow({id: 'window', row: h.document.rows[0], sectionOrder: 0});
  h.controller.toggle('window', h.controller.key({kind: b.kind, id: b.id, libraryId: 'library'}));
  h.controller.toggle('window', h.controller.key({kind: a.kind, id: a.id, libraryId: 'library'}));
  await h.controller.execute('play');
  assert.deepEqual(h.played[0].items.map(item => 'id' in item.identity ? item.identity.id : ''), ['show', 'movie']);
  const first = h.played[0].items[0]; assert.ok('entry' in first); assert.equal(first.entry, a); assert.equal(first.target, target.playback);
  await h.controller.execute('play-later');
  assert.deepEqual(h.queued[0].items.map(item => 'id' in item.identity ? item.identity.id : ''), ['movie', 'show']);
  assert.equal(h.errors.length, 0); h.controller.dispose();
});

test('foreign rows, reordered windows and independent spotlight impostors are refused', () => {
  const h = harness();
  assert.throws(() => h.controller.registerWindow({id: 'fake', row: {...h.document.rows[0]}, sectionOrder: 0}), /unoffered-home-row/);
  assert.throws(() => h.controller.registerWindow({id: 'reordered', row: h.document.rows[0], entries: [...h.entries].reverse(), sectionOrder: 0}), /home-window-order-mismatch/);
  const spotlight = {entry: h.entries[0], reason: {kind: 'featured', text: {code: 'featured', fallback: 'Featured'}}};
  h.replace({...h.document, spotlight: [spotlight]});
  assert.throws(() => h.controller.registerSpotlight({id: 'fake-hero', spotlight: {...spotlight}}), /unoffered-home-spotlight/);
  assert.equal(h.controller.registerSpotlight({id: 'hero', spotlight}).ok, true); h.controller.dispose();
});

test('spotlight My List saves the displayed show flag without a fabricated row or descendant job', async () => {
  const calls: {path: string; body: unknown}[] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, body) => {
    calls.push({path, body});
    return {revision: 1, watched: false, watermark: '', watchlisted: body !== undefined, favorite: false} as never;
  };
  const show = entry('show', 'show'), h = harness([show], api), spotlight = {entry: show, reason: {kind: 'featured', text: {code: 'featured', fallback: 'Featured'}}};
  h.replace({...h.document, spotlight: [spotlight]}); h.controller.registerSpotlight({id: 'hero', spotlight});
  h.controller.openMenu('hero', h.controller.key({kind: 'show', id: 'show', libraryId: 'library'})); await h.controller.executeMenu('watchlist');
  assert.equal(calls.length, 2); assert.match(calls[0].path, /containers\/show\/show\/personal-state$/);
  assert.deepEqual(calls[1].body, {expectedRevision: 0, watchlisted: true}); assert.equal(h.errors.length, 0); h.controller.dispose();
});

test('authority/revision change clears retained picks and blocks late playback completion', async () => {
  const h = harness(); let release!: (value: {queue: {id: string; revision: string; total: number; repeat: 'off'; segments: []}}) => void;
  h.options.playback.play = async () => new Promise(resolve => {release = resolve;});
  pickAll(h); const running = h.controller.execute('play');
  h.replace({...h.document, viewerFence: 'c'.repeat(64), revision: {catalog: 1, viewer: 2}});
  release({queue: {id: 'late', revision: '1', total: 2, repeat: 'off', segments: []}}); await running;
  assert.equal(h.controller.getSnapshot().selection.summary.count, 0); assert.deepEqual(h.controller.getSnapshot().displayMembers, []);
  assert.notEqual(h.controller.getSnapshot().action.phase, 'complete'); h.controller.dispose();
});

function preview(targets: readonly {kind: string; contentKind: string; id: string; libraryId: string}[]) {
  return {serverId: 'server', viewerFence: fence, contentKind: 'movie', kind: 'item', total: targets.length, schema: [{field: 'year', label: 'Year', group: 'numbers', type: 'integer', min: 0, max: 9999, bulk: true}], singleItemFields: [{field: 'year', label: 'Year', group: 'numbers', type: 'integer', min: 0, max: 9999, bulk: true}, {field: 'title', label: 'Title', group: 'general', type: 'text', bulk: false}], excludedFields: [{field: 'title', label: 'Title', group: 'general', type: 'text', bulk: false}], fields: {year: {state: 'shared', value: '2020', distinctCount: 1, values: [{value: '2020', count: targets.length}], entries: [], locked: 0, truncated: false}}, targets: targets.map(target => ({kind: target.kind, id: target.id, libraryId: target.libraryId, expectedRevision: revision}))};
}
test('metadata explicit retry previews only failed targets and retains every actual receipt', async () => {
  const previews: string[][] = [], edits: {operationId: string; targets: {kind: string; id: string}[]}[] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    const body = raw as {targets: {kind: string; contentKind: string; id: string; libraryId: string}[]; operationId: string};
    if (path === routePath('post_metadata_bulk_preview')) {previews.push(body.targets.map(target => target.id)); return preview(body.targets) as never;}
    if (path !== routePath('post_metadata_bulk')) throw new Error(`Unexpected route ${path}`);
    edits.push(body);
    const first = edits.length === 1;
    return {operationId: body.operationId, updated: 1, failed: first ? 1 : 0, results: body.targets.map(target => first && target.id === 'b' ? {kind: 'item', id: 'b', ok: false, code: 'metadata_conflict', message: 'Changed'} : {kind: 'item', id: target.id, ok: true, revision: 'd'.repeat(64)})} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.openEditor(); h.controller.setEditorDraft({fields: {year: {value: '2025'}}, lockEdited: true}); await h.controller.saveEditor();
  assert.equal(h.controller.getSnapshot().editor?.receipts.length, 1); await h.controller.retryEditor();
  assert.deepEqual(previews, [['a', 'b'], ['b']]); assert.deepEqual(edits.map(edit => edit.targets.map(target => target.id)), [['a', 'b'], ['b']]);
  assert.notEqual(edits[0].operationId, edits[1].operationId); assert.deepEqual(h.controller.getSnapshot().editor?.receipts.map(receipt => receipt.operationId), edits.map(edit => edit.operationId)); h.controller.dispose();
});

test('an ambiguous metadata response retries the original byte-identical operation', async () => {
  const bodies: unknown[] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    const body = raw as {targets: {kind: string; contentKind: string; id: string; libraryId: string}[]; operationId: string};
    if (path === routePath('post_metadata_bulk_preview')) return preview(body.targets) as never;
    bodies.push(raw); if (bodies.length === 1) throw new Error('Response lost');
    return {operationId: body.operationId, updated: body.targets.length, failed: 0, results: body.targets.map(target => ({kind: target.kind, id: target.id, ok: true, revision: 'd'.repeat(64)}))} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.openEditor(); h.controller.setEditorDraft({fields: {year: {value: '2025'}}}); await h.controller.saveEditor();
  assert.equal(h.controller.getSnapshot().editor?.phase, 'uncertain'); await h.controller.saveEditor();
  assert.deepEqual(bodies[1], bodies[0]); assert.equal(h.controller.getSnapshot().editor?.receipts.length, 1); h.controller.dispose();
});

test('picker search requests a new server window rather than filtering the displayed page', async () => {
  const requests: {query: string; cursor?: string}[] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    assert.equal(path, routePath('post_home_destination_picker')); const body = raw as {kind: string; query: string; cursor?: string}; requests.push(body);
    return {serverId: 'server', viewerFence: fence, kind: body.kind, query: body.query, revision: {catalog: 1, viewer: 1}, rows: [{id: body.query ? 'outside-first-page' : 'first-page', name: body.query || 'Initial', count: 5, revision: 2, canAdd: true, actions: ['add'], artwork: []}], nextCursor: ''} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.openPicker('playlist'); await h.controller.pickerSearch('Elsewhere');
  assert.deepEqual(requests.map(request => request.query), ['', 'Elsewhere']); assert.equal(h.controller.getSnapshot().picker?.rows[0].id, 'outside-first-page'); h.controller.dispose();
});

test('delete over 200 opens the real cap explanation without requesting or changing media', async () => {
  const h = harness(Array.from({length: 201}, (_, index) => entry(String(index)))); pickAll(h); await h.controller.openDelete();
  assert.equal(h.controller.getSnapshot().deletion?.phase, 'limit'); assert.equal(h.controller.getSnapshot().deletion?.preview, null); h.controller.dispose();
});

test('per-row editor retry rejects successful or duplicate target IDs before another read', async () => {
  let edits = 0, reads = 0;
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    const body = raw as {targets: {kind: string; contentKind: string; id: string; libraryId: string}[]; operationId: string};
    if (path === routePath('post_metadata_bulk_preview')) {reads++; return preview(body.targets) as never;}
    edits++; return {operationId: body.operationId, updated: 1, failed: 1, results: [{kind: 'item', id: 'a', ok: true, revision}, {kind: 'item', id: 'b', ok: false, code: 'metadata_conflict', message: 'Changed'}]} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.openEditor(); h.controller.setEditorDraft({fields: {year: {value: '2025'}}}); await h.controller.saveEditor();
  await assert.rejects(h.controller.retryEditor(h.controller.getSnapshot().editor!.draft, ['a']), /invalid-editor-retry-targets/);
  await assert.rejects(h.controller.retryEditor(h.controller.getSnapshot().editor!.draft, ['b', 'b']), /invalid-editor-retry-targets/);
  assert.equal(reads, 1); assert.equal(edits, 1); h.controller.dispose();
});

test('a failed retry preview preserves receipts, draft and only the same failed targets on retry', async () => {
  const reads: string[][] = [], writes: string[][] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    const body = raw as {targets: {kind: string; contentKind: string; id: string; libraryId: string}[]; operationId: string};
    if (path === routePath('post_metadata_bulk_preview')) {
      reads.push(body.targets.map(target => target.id));
      if (reads.length === 2) throw new Error('Preview connection lost');
      return preview(body.targets) as never;
    }
    writes.push(body.targets.map(target => target.id));
    const first = writes.length === 1;
    return {operationId: body.operationId, updated: 1, failed: first ? 1 : 0, results: body.targets.map(target => first && target.id === 'b' ? {kind: 'item', id: 'b', ok: false, code: 'metadata_conflict', message: 'Changed'} : {kind: 'item', id: target.id, ok: true, revision})} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.openEditor();
  h.controller.setEditorDraft({fields: {year: {value: '2024'}}}); await h.controller.saveEditor();
  const draft = {fields: {year: {value: '2025'}}}; await h.controller.retryEditor(draft, ['b']);
  assert.equal(h.controller.getSnapshot().editor?.phase, 'error'); assert.equal(h.controller.getSnapshot().editor?.receipts.length, 1);
  assert.deepEqual(h.controller.getSnapshot().editor?.draft, draft);
  await h.controller.reloadEditor();
  assert.deepEqual(reads, [['a', 'b'], ['b'], ['b']]); assert.deepEqual(writes, [['a', 'b'], ['b']]);
  assert.equal(h.controller.getSnapshot().editor?.receipts.length, 2); h.controller.dispose();
});

test('toolbar descendant grants never become an unsupported album title My List toggle', async () => {
  const bodies: unknown[] = [];
  const album = {...entry('album', 'album'), homeActions: offers({myList: false})};
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    assert.equal(path, routePath('create_job')); bodies.push(raw);
    return {jobId: 'real-job', command: 'personal-state', state: 'complete', total: 2, totalKnown: true, done: 2, failed: 0} as never;
  };
  const h = harness([album], api); pickAll(h);
  h.controller.openMenu('window', h.controller.key({kind: 'album', id: 'album', libraryId: 'library'}));
  await assert.rejects(h.controller.executeMenu('watchlist'), /menu-action-not-offered/);
  await h.controller.execute('my-list');
  assert.equal(bodies.length, 1); assert.deepEqual((bodies[0] as {selector: unknown}).selector, {container: {kind: 'album', id: 'album'}});
  assert.deepEqual((bodies[0] as {args: unknown}).args, {watchlist: true}); h.controller.dispose();
});

test('confirmed partial personal retry submits only real failed leaves and retains actual job ids', async () => {
  const bodies: {selector: unknown}[] = [];
  const api: HomeSelectionOptions['api']['request'] = async (path, _method, raw) => {
    if (path.endsWith('/first-job/failures')) return {items: [{itemId: 'b', code: 'personal_needs_resolution'}]} as never;
    assert.equal(path, routePath('create_job')); bodies.push(raw as {selector: unknown});
    const first = bodies.length === 1;
    return {jobId: first ? 'first-job' : 'retry-job', command: 'personal-state', state: first ? 'partial' : 'complete', total: first ? 2 : 1, totalKnown: true, done: 1, failed: first ? 1 : 0} as never;
  };
  const h = harness(undefined, api); pickAll(h); await h.controller.execute('my-list');
  assert.equal(h.controller.getSnapshot().action.phase, 'partial'); await h.controller.retryAction();
  assert.deepEqual(bodies[1].selector, {items: {ids: ['b']}}); assert.equal(h.controller.getSnapshot().action.phase, 'complete');
  assert.deepEqual(h.controller.getSnapshot().action.jobIds, ['first-job', 'retry-job']); h.controller.dispose();
});
