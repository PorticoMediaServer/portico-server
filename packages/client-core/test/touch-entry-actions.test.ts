import assert from 'node:assert/strict';
import {test} from 'node:test';
import {entryActions, touchEntryActions} from '../src/presentation/entry-actions.ts';

test('Q6 touch actions use the drawn order without queue or group actions', () => {
  const c = {playable: true, progressSeconds: 30, queue: true, group: true, watchlisted: false, watched: false, favorite: false, rating: null,
    addToPlaylist: true, addToCollection: true, download: 'media' as const, details: true, editMetadata: true, refreshMetadata: true, delete: true};
  assert.deepEqual(touchEntryActions(c).flatMap(group => group.actions.map(action => action.id)), ['play', 'watchlist', 'watched', 'favorite', 'rate', 'addToPlaylist', 'addToCollection', 'download', 'details', 'editMetadata', 'refreshMetadata', 'delete']);
  assert.equal(touchEntryActions(c).at(-1)?.title, 'next.selection.owner');
  assert.ok(entryActions(c).flatMap(group => group.actions).some(action => action.id === 'resume'));
  assert.ok(entryActions(c).flatMap(group => group.actions).some(action => action.id === 'addToQueue'));
});
test('touch projection invents no owner, personal, download or row-context authority', () => {
  assert.deepEqual(touchEntryActions({}), []);
  const groups = touchEntryActions({playable: true});
  assert.deepEqual(groups.flatMap(group => group.actions.map(action => action.id)), ['play']);
  const contextual = touchEntryActions({continueWatching: true, notInterested: true});
  assert.deepEqual(contextual.flatMap(group => group.actions.map(action => action.id)), ['removeFromContinueWatching', 'notInterested']);
});
