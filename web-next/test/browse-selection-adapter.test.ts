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
const fake = (over: Record<string, unknown> = {}, host: Record<string, unknown> = {}) => {
  const calls: string[] = [];
  const snapshot = {
    phase: 'ready',
    rows: [row],
    resolving: false,
    playing: false,
    canPlay: true,
    selection: {active: true, context: {}, summary: {count: 1, types: [{kind: 'movie', count: 1}], library: {kind: 'single', libraryId: 'a'}, every: capabilities},
      selectedKeys: ['key-0'], version: 1, registeredWindowCount: 1, registeredItemCount: 1, lastSectionId: 'library', target: null},
    ...over,
  };
  const selection = {
    getSnapshot: () => snapshot,
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
