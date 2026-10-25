import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {createLibrarySelection} = await import('../src/screens/browse/selection.tsx');

const capabilities = {play: true, queue: true, playlist: true, myList: true, watched: true, collection: true, favorite: true, download: false, edit: true, refresh: true, delete: true};
const entry = {id: 'm1', kind: 'movie', title: 'Alpha', libraryId: 'a'};
const row = {entry, position: 0, key: 'key-0', capabilities};
const target = {mode: 'explicit' as const, context: {}, items: [{key: 'key-0', item: {kind: 'movie', id: 'm1', libraryId: 'a', capabilities}}], displayItems: [{key: 'key-0', item: {kind: 'movie', id: 'm1', libraryId: 'a', capabilities}}]};
const fake = (over: Record<string, unknown> = {}, host: Record<string, unknown> = {}) => {
  const calls: string[] = [];
  const snapshot = {
    phase: 'ready',
    rows: [row],
    resolving: false,
    playing: false,
    canPlay: true,
    selection: {active: true, context: {}, summary: {count: 1, types: [{kind: 'movie', count: 1}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
      selectedKeys: ['key-0'], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target},
    ...over,
  };
  const selection = {
    getSnapshot: () => snapshot,
    captureOperation: () => snapshot.selection.target ? {target: snapshot.selection.target} : null,
    isOperationCurrent: () => true,
    subscribe: () => () => {},
    toggle: (expected: unknown, range: boolean) => { calls.push(`toggle:${(expected as typeof row).key}:${range}`); return {ok: true}; },
    selectLoaded: () => { calls.push('selectLoaded'); return {ok: true}; },
    clear: () => { calls.push('clear'); },
    play: async () => { calls.push('play'); return {} as never; },
    registerWindow: async () => { calls.push('registerWindow'); return [row]; },
    start: () => () => {},
    dispose: () => {},
    ...host,
  };
  return {selection: selection as never, calls};
};

test('no bar and no checked circle before selection is active', () => {
  const {selection} = fake({selection: {active: false, summary: {count: 0, types: [], library: {kind: 'none'}, every: {}}, selectedKeys: [], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: null, target: null}});
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  assert.equal(renderToStaticMarkup(createElement('div', null, adapter.toolbar)), '<div></div>');
  const card = adapter.entry({membershipId: 'm1', entry} as never, 0);
  assert.equal(card?.active, false);
  assert.equal(card?.checked, false);
});

test('the bar counts the real selection and offers only what the adapter can perform', () => {
  const {selection, calls} = fake();
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, /1 selected/);
  assert.match(html, />Play</);
  // Whole-view Select all N is not offered until the descriptor cohort is wired here.
  assert.doesNotMatch(html, /Select all/);
  assert.doesNotMatch(html, />Queue</);
  assert.doesNotMatch(html, />Edit</);
  adapter.entry({membershipId: 'm1', entry} as never, 0)?.onToggle({range: true, source: 'pointer'});
  assert.deepEqual(calls, ['toggle:key-0:true']);
});

test('the window registers through the play adapter so ranges are real', () => {
  const {selection, calls} = fake();
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, />Done</);
  adapter.registerWindow(0, 40);
  assert.deepEqual(calls, ['registerWindow']);
});

test('a selection with nothing playable hides Play rather than pretending', () => {
  const {selection} = fake({selection: {active: true, summary: {count: 1, types: [{kind: 'movie', count: 1}], library: {kind: 'single', libraryId: 'a'},
    every: {...capabilities, play: false}}, selectedKeys: ['key-0'], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: null}});
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.doesNotMatch(html, />Play</);
  assert.match(html, /1 selected/);
});


test('whole-view Select all appears only with the server descriptor and a model that can hold it', () => {
  const whole = {libraryId: 'a', pivot: 'movies', filter: null, sort: [{field: 'title', direction: 'asc'}], catalogRevision: 'r1', total: 48000,
    types: [{kind: 'movie', count: 48000, capabilities: capabilities, libraries: [{libraryId: 'a', count: 48000}]}]};
  const withWhole = fake({}, {selectWhole: () => {}});
  const adapter = createLibrarySelection({playSelection: withWhole.selection, descriptor: () => whole as never, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, /Select all 48,000/);
  // Without a descriptor, or without the model capability, nothing is offered.
  const noDescriptor = createLibrarySelection({playSelection: withWhole.selection, reportError: () => {}});
  assert.doesNotMatch(renderToStaticMarkup(createElement('div', null, noDescriptor.toolbar)), /Select all/);
  const noModel = createLibrarySelection({playSelection: fake().selection, descriptor: () => whole as never, reportError: () => {}});
  assert.doesNotMatch(renderToStaticMarkup(createElement('div', null, noModel.toolbar)), /Select all/);
});

test('a card previews the range only when the play adapter says it would join it', () => {
  const previewing = fake({}, {preview: () => true});
  const adapter = createLibrarySelection({playSelection: previewing.selection, reportError: () => {}});
  assert.equal(adapter.entry({membershipId: 'm1', entry} as never, 0)?.preview, true);
  const quiet = fake({}, {preview: () => false});
  const noPreview = createLibrarySelection({playSelection: quiet.selection, reportError: () => {}});
  assert.equal(noPreview.entry({membershipId: 'm1', entry} as never, 0)?.preview, undefined);
});


test('Watched and My List appear only with real job ports, and the job carries the real target', async () => {
  const requests: {path: string; body: unknown}[] = [];
  const actions = {
    api: {request: async (path: string, _method?: string, body?: unknown) => {
      requests.push({path, body});
      return {jobId: 'job-1', command: 'personal-state', state: 'complete', total: 1, totalKnown: true, done: 1, failed: 0};
    }},
    events: {on: () => () => {}}, requestId: async () => 'operation-1', isCurrent: () => true, reportError: () => {},
  };
  const {selection} = fake();
  const adapter = createLibrarySelection({playSelection: selection, actions: actions as never, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, />Watched</);
  assert.match(html, />My List</);
  await adapter.personal({watched: true});
  assert.deepEqual(requests[0]?.body?.selector ?? {}, {items: {ids: ['m1']}});
  assert.equal((requests[0]?.body as {command: string}).command, 'personal-state');
  assert.deepEqual((requests[0]?.body as {args: unknown}).args, {watched: true});
  // Without the ports the bar offers no personal action at all.
  const bare = createLibrarySelection({playSelection: fake().selection, reportError: () => {}});
  assert.doesNotMatch(renderToStaticMarkup(createElement('div', null, bare.toolbar)), />Watched</);
});

test('a container pick never becomes a personal job', async () => {
  let requested = 0;
  const actions = {api: {request: async () => { requested++; return {}; }}, events: {on: () => () => {}}, requestId: async () => 'op', isCurrent: () => true, reportError: () => {}};
  const containerTarget = {...target, items: [{key: 'key-0', item: {kind: 'show', id: 's1', libraryId: 'a', capabilities}}]};
  const {selection} = fake({selection: {active: true, summary: {count: 1, types: [{kind: 'show', count: 1}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
    selectedKeys: ['key-0'], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: containerTarget}});
  const adapter = createLibrarySelection({playSelection: selection, actions: actions as never, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.doesNotMatch(html, />Watched</);
  await assert.rejects(adapter.personal({watched: true}), /container-personal-state-unsupported/);
  assert.equal(requested, 0);
});


test('Play is offered for explicit picks only until a whole view can be queued', () => {
  const whole = {...target, mode: 'query' as const, items: undefined, selector: {query: {libraryId: 'a', pivot: 'movies'}}, total: 48000, sourceTotal: 48000, refinement: {kind: null, excludedItems: []}, expected: {catalogRevision: 'r1'}};
  const {selection} = fake({selection: {active: true, summary: {count: 48000, types: [{kind: 'movie', count: 48000}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
    selectedKeys: [], version: 2, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: whole}});
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, /48,000 selected/);
  assert.doesNotMatch(html, />Play</);
});


test('Queue and the destination pickers take the explicit leaf ids in order', async () => {
  const calls: unknown[] = [];
  let queued: {items: readonly string[]; placement: string; signal: AbortSignal; isCurrent: () => boolean} | undefined;
  const enqueue = async (input: typeof queued) => { queued = input; };
  const destinations = {playlist: async (items: readonly string[], anchor: HTMLElement) => { calls.push(['playlist', items, anchor]); },
    collection: async (items: readonly string[], anchor: HTMLElement) => { calls.push(['collection', items, anchor]); }};
  const {selection} = fake();
  const adapter = createLibrarySelection({playSelection: selection, enqueue, destinations, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.match(html, />Add to queue</);
  assert.match(html, />Add to playlist</);
  const anchor = {} as HTMLElement;
  await adapter.queue('end');
  await adapter.addToPlaylist(anchor);
  await adapter.addToCollection(anchor);
  assert.deepEqual(queued?.items, ['m1']);
  assert.equal(queued?.placement, 'end');
  assert.ok(queued?.signal instanceof AbortSignal);
  assert.equal(typeof queued?.isCurrent, 'function');
  assert.deepEqual(calls, [['playlist', ['m1'], anchor], ['collection', ['m1'], anchor]]);
});

test('container and whole-view picks are offered no Queue or destination action', async () => {
  const enqueue = async () => { throw new Error('must not run'); };
  const destinations = {playlist: async () => { throw new Error('must not run'); }};
  const containerTarget = {...target, items: [{key: 'key-0', item: {kind: 'show', id: 's1', libraryId: 'a', capabilities}}],
    displayItems: [{key: 'key-0', item: {kind: 'show', id: 's1', libraryId: 'a', capabilities}}]};
  const container = fake({selection: {active: true, summary: {count: 1, types: [{kind: 'show', count: 1}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
    selectedKeys: ['key-0'], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: containerTarget}});
  const adapter = createLibrarySelection({playSelection: container.selection, enqueue, destinations, reportError: () => {}});
  const html = renderToStaticMarkup(createElement('div', null, adapter.toolbar));
  assert.doesNotMatch(html, />Add to queue</);
  assert.doesNotMatch(html, />Add to playlist</);
  await assert.rejects(adapter.queue('next'), /leaf-explicit-selection-required/);
  await assert.rejects(adapter.addToPlaylist({} as HTMLElement), /leaf-explicit-selection-required/);
});


test('a whole-view member paints checked from the model, not the explicit keys', () => {
  const whole = {...target, mode: 'query' as const, items: undefined, selector: {query: {libraryId: 'a', pivot: 'movies'}}, total: 48000, sourceTotal: 48000, refinement: {kind: null, excludedItems: []}, expected: {catalogRevision: 'r1'}};
  const {selection} = fake({selection: {active: true, summary: {count: 48000, types: [{kind: 'movie', count: 48000}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
    selectedKeys: [], version: 2, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: whole}},
    {selected: (row: {key: string}) => row.key === 'key-0'});
  const adapter = createLibrarySelection({playSelection: selection, reportError: () => {}});
  assert.equal(adapter.entry({membershipId: 'm1', entry} as never, 0)?.checked, true);
});


test('personal actions require a model operation lease', async () => {
  const actions = {api: {request: async () => { throw new Error('must not dispatch'); }}, events: {on: () => () => {}},
    requestId: async () => 'op', isCurrent: () => true, reportError: () => {}};
  for (const host of [{captureOperation: undefined}, {isOperationCurrent: undefined}]) {
    const adapter = createLibrarySelection({playSelection: fake({}, host).selection, actions: actions as never, reportError: () => {}});
    assert.doesNotMatch(renderToStaticMarkup(createElement('div', null, adapter.toolbar)), />Watched</);
    await assert.rejects(adapter.personal({watched: true}), /selection-actions-unavailable/);
  }
});

test('clearing a captured selection while preparing a personal job prevents submission', async () => {
  let current = true, requested = 0;
  const operation = {target};
  const host = {captureOperation: () => operation, isOperationCurrent: (captured: unknown) => captured === operation && current};
  const actions = {api: {request: async () => { requested++; throw new Error('must not dispatch'); }}, events: {on: () => () => {}},
    requestId: async () => { current = false; return 'op'; }, isCurrent: () => true, reportError: () => {}};
  const adapter = createLibrarySelection({playSelection: fake({}, host).selection, actions: actions as never, reportError: () => {}});
  await assert.rejects(adapter.personal({watched: true}), /cancelled/);
  assert.equal(requested, 0);
});
