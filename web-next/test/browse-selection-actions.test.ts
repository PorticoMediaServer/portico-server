import test from 'node:test';
import assert from 'node:assert/strict';
import {createSelectionActions, personalTargetIsLeaf, SelectionActionError} from '../src/screens/browse/selection-actions.ts';

const item = (kind: string, id: string) => ({kind, id, libraryId: 'a', capabilities: {play: true, queue: true, playlist: true, myList: true, watched: true, collection: true, favorite: true, download: false, edit: true, refresh: true, delete: true}});
const explicit = (kinds: readonly string[]) => ({mode: 'explicit', context: {}, items: kinds.map((kind, index) => ({key: `${kind}:${index}`, item: item(kind, `id${index}`)})), displayItems: []});
const query = {mode: 'query', context: {}, queryId: 'q', selector: {query: {libraryId: 'a', pivot: 'movies'}}, total: 10, sourceTotal: 10, refinement: {kind: null, excludedItems: []}, summary: {}, expected: {catalogRevision: 'r1'}};

test('only leaf picks and the server leaf views may become a personal job', () => {
  assert.equal(personalTargetIsLeaf(explicit(['movie', 'episode', 'song', 'audiobook_file'])), true);
  assert.equal(personalTargetIsLeaf(explicit(['movie', 'show'])), false);
  assert.equal(personalTargetIsLeaf(explicit(['show'])), false);
  assert.equal(personalTargetIsLeaf(explicit(['album', 'artist', 'book', 'collection'])), false);
  assert.equal(personalTargetIsLeaf(explicit([])), false);
  // A whole view is already restricted to leaf pivots by the server descriptor.
  assert.equal(personalTargetIsLeaf(query), true);
});

test('one job command carries exactly one personal fact', async () => {
  const actions = createSelectionActions({api: {request: async () => { throw new Error('no request expected'); }}, events: {on: () => () => {}},
    requestId: async () => 'op', isCurrent: () => true, reportError: () => {}});
  const signal = new AbortController().signal;
  await assert.rejects(actions.personal({target: explicit(['show'])} as never, {watched: true}, signal), (error: unknown) => error instanceof SelectionActionError && error.code === 'container-personal-state-unsupported');
  await assert.rejects(actions.personal({target: explicit(['movie'])} as never, {}, signal), (error: unknown) => error instanceof SelectionActionError && error.code === 'empty-command');
  await assert.rejects(actions.personal({target: explicit(['movie'])} as never, {watched: true, favorite: true}, signal), (error: unknown) => error instanceof SelectionActionError && error.code === 'mixed-personal-command');
  await assert.rejects(actions.personal({target: explicit(['movie'])} as never, {watched: true}, AbortSignal.abort()), (error: unknown) => error instanceof SelectionActionError && error.code === 'cancelled');
});
