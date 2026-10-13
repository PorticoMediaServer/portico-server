import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

test('leaving a library keeps its list for Back; the list is the viewer\'s pooled document, read again quietly with the filters on screen', async () => {
  const cleanups: (() => void)[] = [];
  const h = hooks();
  const react = {...h.react, useEffect: (fn: () => void | (() => void)) => { const stop = fn(); if (typeof stop === 'function') cleanups.push(stop); }};
  let disposed = 0, marked = 0;
  const quiet: unknown[] = [];
  const invalidated: unknown[] = [];
  const collection = {
    dispose: () => { disposed++; }, invalidate: (source: unknown) => { invalidated.push(source); }, itemAt: () => (disposed ? undefined : {id: 'm1'}),
    getSnapshot: () => ({total: 1}), subscribe: () => () => {}, revalidate: async (options: unknown) => { quiet.push(options); },
  };
  const pooled: {key: string; made: any}[] = [];
  const memo: Record<string, unknown> = {};
  const sources: unknown[] = [];
  const browse = await componentModule(new URL('../src/app/browse.ts', import.meta.url), {
    react: {...react, default: react},
    '@core/collections/index.ts': {createWindowedCollection: () => collection},
    '@core/freshness-hub.ts': {titlesWithPersonalState: (libraryId: string) => ['content:' + libraryId, 'personal:' + libraryId]},
    './freshness': {usePooled: (key: string, make: () => any) => { const made = pooled.find(p => p.key === key)?.made ?? make(); if (!pooled.some(p => p.key === key)) pooled.push({key, made}); return {service: made.service, document: {markCurrent: () => { marked++; }}, memo}; }},
    './session': {useSession: () => ({api: {}, session: undefined}), sessionIdentity: () => 'ana'},
    './browse-page-source.ts': {browsePageSource: (_api: unknown, input: unknown) => { sources.push(input); return async () => ({items: [], total: 0}); }},
    './revalidating-cache.ts': {RevalidatingCache: class { use() { return () => {}; } }},
  }) as typeof import('../src/app/browse.ts');
  const first = h.render(() => browse.useBrowseCollection({libraryId: 'movies', pivot: 'movies', predicates: [], enabled: true}));
  assert.equal(pooled[0]!.key, 'browse:movies:movies:60', 'one kept list per library and pivot');
  assert.deepEqual(pooled[0]!.made.document.dependencies, ['content:movies', 'personal:movies']);
  for (const stop of cleanups) stop();
  await new Promise(resolve => setTimeout(resolve, 5));
  assert.equal(disposed, 0, 'the list is not thrown away when the screen closes');
  assert.deepEqual(collection.itemAt(), {id: 'm1'});
  // Back: the same collection, and nothing is reloaded because the query is the one it shows.
  const again = h.render(() => browse.useBrowseCollection({libraryId: 'movies', pivot: 'movies', predicates: [], enabled: true}));
  assert.equal(again.collection, first.collection);
  assert.equal(invalidated.length, 0);
  // A quiet read asks for the filters and sort on screen now, and waits for the pause it is given.
  const settle = async () => {};
  await pooled[0]!.made.document.revalidate({settle});
  assert.equal(quiet.length, 1);
  assert.equal((quiet[0] as {hold: unknown}).hold, settle);
  assert.equal(typeof (quiet[0] as {fetchPage: unknown}).fetchPage, 'function');
  // A new filter is a new generation of the same list.
  h.render(() => browse.useBrowseCollection({libraryId: 'movies', pivot: 'movies', predicates: [{field: 'year', operator: 'eq', value: 1999}], enabled: true}));
  assert.equal(invalidated.length, 1);
  assert.ok(marked >= 2);
});
