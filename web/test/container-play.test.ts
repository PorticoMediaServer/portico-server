/**
 * Container plays on Playback v1: which Play and Shuffle go to the server as one `playSelector`
 * request, and which keep sending the loaded entries (episode groups, no container). Popular
 * tracks (a top-5 list) always play their own five as an item list, so pages pass no container.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {containerPlay, discSelectorId, groupSequence, occurrenceSequence, savedSequence, sequenceContainer, viewSequence} from '../src/app/container-play.ts';

test('a show, a season, an album, an artist, a disc, a collection, a playlist and a book are containers', () => {
  for (const kind of ['show', 'season', 'album', 'artist', 'collection', 'playlist', 'book']) assert.deepEqual(sequenceContainer(kind, 'c1'), {kind, id: 'c1'}, kind);
  assert.deepEqual(sequenceContainer('season', 's1', {shuffle: true}), {kind: 'season', id: 's1', shuffle: true});
  assert.deepEqual(sequenceContainer('album', 'a1', {shuffle: true}), {kind: 'album', id: 'a1', shuffle: true});
  assert.deepEqual(sequenceContainer('artist', 'ar1', {shuffle: true}), {kind: 'artist', id: 'ar1', shuffle: true});
});

test('a disc page maps its catalog id to the v1 disc selector', () => {
  assert.deepEqual(sequenceContainer('disc', 'albumA:disc:2'), {kind: 'disc', id: 'albumA:2'});
  assert.deepEqual(sequenceContainer('disc', 'albumA:disc:2', {shuffle: true}), {kind: 'disc', id: 'albumA:2', shuffle: true});
  assert.equal(sequenceContainer('disc', 'albumA'), undefined);
  assert.equal(sequenceContainer('disc', 'albumA:2'), undefined);
  assert.equal(sequenceContainer('disc', ':disc:2'), undefined);
});

test('an episode group that isn\'t a season, an unknown kind or a missing id falls back to the entry list', () => {
  assert.equal(sequenceContainer('season', 's1', {group: true}), undefined);
  for (const kind of ['movie', 'episode', 'song', undefined]) assert.equal(sequenceContainer(kind, 'x'), undefined, String(kind));
  assert.equal(sequenceContainer('show', undefined), undefined);
});

test('v1 Play: the container from the pressed entry, at its resume point', () => {
  assert.deepEqual(containerPlay({kind: 'season', id: 's1'}, 'ep4', 612), {selector: {container: {kind: 'season', id: 's1'}}, options: {anchor: {itemId: 'ep4'}, startSeconds: 612}});
  assert.deepEqual(containerPlay({kind: 'collection', id: 'c1'}, 'm1', 0), {selector: {container: {kind: 'collection', id: 'c1'}}, options: {anchor: {itemId: 'm1'}}});
  assert.deepEqual(containerPlay({kind: 'album', id: 'a1'}, 'track5', 0), {selector: {container: {kind: 'album', id: 'a1'}}, options: {anchor: {itemId: 'track5'}}});
  assert.deepEqual(containerPlay({kind: 'artist', id: 'ar1'}, 'track2', 0), {selector: {container: {kind: 'artist', id: 'ar1'}}, options: {anchor: {itemId: 'track2'}}});
  assert.deepEqual(containerPlay({kind: 'disc', id: 'a1:2'}, 'track7', 0), {selector: {container: {kind: 'disc', id: 'a1:2'}}, options: {anchor: {itemId: 'track7'}}});
});

test('v1 Shuffle: the server shuffles the whole container and picks the first entry', () => {
  assert.deepEqual(containerPlay({kind: 'playlist', id: 'p1', shuffle: true}, 'x', 0), {selector: {container: {kind: 'playlist', id: 'p1'}}, options: {shuffle: true}});
  assert.deepEqual(containerPlay({kind: 'artist', id: 'ar1', shuffle: true}, 'x', 0), {selector: {container: {kind: 'artist', id: 'ar1'}}, options: {shuffle: true}});
  assert.deepEqual(containerPlay({kind: 'disc', id: 'a1:2', shuffle: true}, 'x', 0), {selector: {container: {kind: 'disc', id: 'a1:2'}}, options: {shuffle: true}});
});

test('disc page ids map to the selector the server accepts', () => {
  assert.equal(discSelectorId('albumA:disc:2'), 'albumA:2');
  assert.equal(discSelectorId('albumA:disc:0'), 'albumA:0');
  assert.equal(discSelectorId('albumA'), undefined);
  assert.equal(discSelectorId('albumA:2'), undefined);
});

test('no container: the loaded entries play', () => {
  assert.equal(containerPlay(undefined, 'ep1', 0), null);
});

test('a playlist row plays that occurrence: the entry, with its title for when that copy is gone', () => {
  assert.deepEqual(containerPlay(occurrenceSequence('p1', 'entry-2'), 'song', 0), {selector: {container: {kind: 'playlist', id: 'p1'}}, options: {anchor: {entryId: 'entry-2', itemId: 'song'}}});
});

test('a saved view or personal collection plays whole from the server by its id', () => {
  assert.deepEqual(containerPlay(savedSequence('view1'), 'm1', 0), {selector: {saved: {id: 'view1'}}, options: {anchor: {itemId: 'm1'}}});
  assert.deepEqual(containerPlay(savedSequence('view1', {shuffle: true}), 'm1', 0), {selector: {saved: {id: 'view1'}}, options: {shuffle: true}});
});

test('a sorted or filtered list plays as the same query the list is read with', () => {
  const sort = {field: 'year', direction: 'desc'} as const;
  const view = viewSequence({libraryId: 'lib', pivot: 'songs', base: [{field: 'memberOf', operator: 'equals', value: 'album1'}], predicates: [{field: 'year', operator: 'at-least', value: 2000}], sort});
  assert.deepEqual(containerPlay(view, 'song9', 0), {selector: {query: {libraryId: 'lib', pivot: 'songs', query: {all: [{field: 'memberOf', operator: 'equals', value: 'album1'}, {field: 'year', operator: 'at-least', value: 2000}]}, sort: [sort]}}, options: {anchor: {itemId: 'song9'}}});
  assert.deepEqual(viewSequence({libraryId: 'lib', pivot: 'movies'}), {kind: 'query', query: {libraryId: 'lib', pivot: 'movies'}});
  assert.equal(viewSequence({libraryId: 'lib'}), undefined);
  const shuffled = viewSequence({libraryId: 'lib', pivot: 'movies', sort: {field: 'random', direction: 'asc', seed: 7}}, {shuffle: true});
  assert.deepEqual(containerPlay(shuffled, 'x', 0)?.selector, {query: {libraryId: 'lib', pivot: 'movies', sort: [{field: 'random', direction: 'asc', seed: 7}]}});
});

test('a group plays its query only while it shows a sorted or filtered list', () => {
  const spec = {libraryId: 'lib', pivot: 'episodes', base: [{field: 'memberOf', operator: 'equals', value: 's1'}]};
  const sort = {field: 'title', direction: 'asc'} as const;
  assert.equal(groupSequence({active: false, spec, predicates: [], sort}), undefined);
  assert.deepEqual(groupSequence({active: true, spec, predicates: [], sort}, {shuffle: true}), {kind: 'query', query: {libraryId: 'lib', pivot: 'episodes', query: {all: spec.base}, sort: [sort]}, shuffle: true});
});

test('a list across libraries plays as the server query it is, in its own order, a Home row too', async () => {
  const {groupSequence, viewSequence, episodeGroupSequence, homeRowSequence} = await import('../src/app/container-play.ts');
  const spec = {libraryId: '*', pivot: 'items', base: [{field: 'list', operator: 'equals' as const, value: 'p1'}]};
  assert.deepEqual(groupSequence({active: true, spec, predicates: [{field: 'year', operator: 'at-least', value: 2000}], sort: {field: 'groupOrder', direction: 'asc'}}), {kind: 'query', query: {libraryId: '*', pivot: 'items', query: {all: [{field: 'list', operator: 'equals', value: 'p1'}, {field: 'year', operator: 'at-least', value: 2000}]}, sort: [{field: 'groupOrder', direction: 'asc'}]}});
  assert.deepEqual(viewSequence({libraryId: '*', pivot: 'titles', base: [{field: 'homeRow', operator: 'equals', value: 'recommended'}]}), {kind: 'query', query: {libraryId: '*', pivot: 'titles', query: {all: [{field: 'homeRow', operator: 'equals', value: 'recommended'}]}}});
  // A Home row of songs plays those songs, whole, in the row's order; an episode group the show's own.
  assert.deepEqual(homeRowSequence('recent_music', ['song']), {kind: 'query', query: {libraryId: '*', pivot: 'items', query: {all: [{field: 'homeRow', operator: 'equals', value: 'recent_music'}]}, sort: [{field: 'groupOrder', direction: 'asc'}]}});
  assert.deepEqual(episodeGroupSequence('tv', 's1', 'range:100', {shuffle: true}), {kind: 'query', query: {libraryId: 'tv', pivot: 'episodes', query: {all: [{field: 'episodeGroup', operator: 'equals', value: 's1/range:100'}]}, sort: [{field: 'groupOrder', direction: 'asc'}]}, shuffle: true});
});
