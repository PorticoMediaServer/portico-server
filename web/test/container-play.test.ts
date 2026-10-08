/**
 * Container plays on Playback v1: which Play and Shuffle go to the server as one `playSelector`
 * request, and which keep sending the loaded entries (episode groups, no container). Popular
 * tracks (a top-5 list) always play their own five as an item list, so pages pass no container.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {containerPlay, discSelectorId, sequenceContainer} from '../src/app/container-play.ts';

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
