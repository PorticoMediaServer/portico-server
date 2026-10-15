/**
 * PERF-S06 page wiring (web): the album, artist and disc pages send one v1 container selector
 * for Play, Shuffle and play-from-a-track — never a client-shuffled page — and the playlist
 * page does the same with the playlist container. Popular tracks (a top-5 list) play their own
 * five as an item list. The engine turns a container into exactly one `playSelector` request
 * (`player/engine.tsx`: container → `playSelector`, anchored at the pressed entry for Play,
 * `shuffle: true` for Shuffle).
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');

test('the entity page plays and shuffles through the container helper', () => {
  const entity = src('screens/library/Entity.tsx');
  assert.match(entity, /import \{groupSequence, sequenceContainer\} from '\.\.\/\.\.\/app\/container-play';/);
  assert.match(entity, /const containerOf = \(shuffle: boolean\) => sequenceContainer\(entity\?\.kind, entity\?\.id, \{shuffle\}\);/);
  assert.match(entity, /player\.playSequence\(loaded, 0, 0, containerOf\(true\)\);/);
  assert.match(entity, /player\.playSequence\(queue, 0, queue\[0\]!\.playback!\.startSeconds \?\? 0, container\);/);
});

test('a sorted or filtered song list plays the whole list from the server, from the song pressed', () => {
  const entity = src('screens/library/Entity.tsx');
  assert.match(entity, /player\.playSequence\(\[pressed\], 0, 0, groupSequence\(songGroup\)\)/);
  assert.doesNotMatch(entity, /index \+ 200/);
});

test('the entity page never shuffles the loaded entries on the client', () => {
  const entity = src('screens/library/Entity.tsx');
  assert.doesNotMatch(entity, /Math\.random/);
});

test('popular tracks play their own five as an item list, not the artist container', () => {
  const entity = src('screens/library/Entity.tsx');
  assert.match(entity, /playTrack\(popularTracks\.entries, index, undefined\)/);
});

test('the playlist page plays, shuffles and plays-from-a-row through the playlist container', () => {
  const resource = src('screens/saved/Resource.tsx');
  assert.match(resource, /playOwn\(media\.map\(e => e\.media\), sequenceContainer\('playlist', resourceId\)\)/);
  assert.match(resource, /sequenceContainer\('playlist', resourceId, \{shuffle: true\}\)/);
  // A row plays its own occurrence, so the copy pressed plays when a title is in the playlist twice.
  assert.match(resource, /player\.playSequence\(media\.map\(x => x\.media\), media\.indexOf\(e\), e\.media\.playback\?\.startSeconds \?\? 0, occurrenceSequence\(resourceId, e\.id\)\)/);
  assert.doesNotMatch(resource, /player\.play\(first\.media\.playback/);
});

test('a sorted or filtered playlist, saved view or collection plays the list shown, and a collection or view plays whole by its id', () => {
  const resource = src('screens/saved/Resource.tsx');
  assert.match(resource, /groupSequence\(group, \{shuffle\}\)/);
  assert.match(resource, /playOwn\(ownPlayable, savedSequence\(resourceId\)\)/);
  assert.match(resource, /playOwn\(ownPlayable, savedSequence\(resourceId, \{shuffle: true\}\)\)/);
});

test('a removal offers the server\'s Undo on the playlist and the collection', () => {
  const resource = src('screens/saved/Resource.tsx');
  assert.match(resource, /service\.mutate\(\{action: 'restore', undoToken: r\.token\}\)/);
  assert.match(resource, /service\.mutate\(\{action: 'entries', undoToken: r\.token\}\)/);
});
