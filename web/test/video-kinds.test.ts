import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {mayCarryVideo} from '../src/player/video-kinds.ts';

test('audio-only kinds have no picture to read about; unknown and video kinds may', () => {
  assert.equal(mayCarryVideo('track'), false);
  assert.equal(mayCarryVideo('song'), false);
  assert.equal(mayCarryVideo('audiobook'), false);
  assert.equal(mayCarryVideo('movie'), true);
  assert.equal(mayCarryVideo('episode'), true);
  assert.equal(mayCarryVideo(undefined), true);
});

test('nothing is asked of the server before a title starts', () => {
  const engine = readFileSync(new URL('../src/player/engine.tsx', import.meta.url), 'utf8');
  const play = engine.slice(engine.indexOf('const play = useCallback('), engine.indexOf('const startSequenceInner'));
  // The start comes first and is not chained to a read; the source facts are read beside it.
  assert.match(play, /startSingle\(itemId, startSeconds, entry, prepared, quality, choices\);\s+noteSourceFacts\(itemId, entry\?\.kind\);/);
  assert.doesNotMatch(play, /\.then\(/);
  assert.doesNotMatch(engine, /dolbyPending|checkDolbyFor|dolby_vision_colors_approximate/);
});
