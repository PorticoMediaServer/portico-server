import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

test('leaving a library disposes its list only after the list has had its turn to remember what was loaded', async () => {
  const cleanups: (() => void)[] = [];
  const h = hooks();
  const react = {...h.react, useEffect: (fn: () => void | (() => void)) => { const stop = fn(); if (typeof stop === 'function') cleanups.push(stop); }};
  let disposed = 0;
  const collection = {dispose: () => { disposed++; }, invalidate: () => {}, itemAt: () => (disposed ? undefined : {id: 'm1'})};
  const browse = await componentModule(new URL('../src/app/browse.ts', import.meta.url), {
    react: {...react, default: react},
    '@core/collections/index.ts': {createWindowedCollection: () => collection},
    './session': {useSession: () => ({api: {}, session: undefined}), sessionIdentity: () => 'ana'},
    './browse-page-source.ts': {browsePageSource: () => async () => ({items: [], total: 0})},
    './revalidating-cache.ts': {RevalidatingCache: class { use() { return () => {}; } }},
  }) as typeof import('../src/app/browse.ts');
  h.render(() => browse.useBrowseCollection({libraryId: 'movies', pivot: 'movies', predicates: [], enabled: true}));
  // React runs the owner's cleanup first, then the list's own (which reads the collection).
  for (const stop of cleanups) stop();
  assert.deepEqual(collection.itemAt(), {id: 'm1'}, 'the list can still read its window during its own cleanup');
  await new Promise(resolve => setTimeout(resolve, 5));
  assert.equal(disposed, 1, 'and the collection is disposed right after');
});
