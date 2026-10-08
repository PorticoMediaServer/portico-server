import test from 'node:test';
import assert from 'node:assert/strict';
import {DOLBY_VISION_APPROXIMATE_REASON, dolbyVisionWarningNeeded} from '../src/player/dolby-vision.ts';

test('flags the new pre-play decision reason', () => {
  assert.equal(DOLBY_VISION_APPROXIMATE_REASON, 'dolby_vision_colors_approximate');
  assert.equal(dolbyVisionWarningNeeded({decision: {video: {action: 'transcode', reasons: ['dolby_vision_colors_approximate']}}}), true);
  assert.equal(dolbyVisionWarningNeeded({decision: {video: {action: 'copy', reasons: []}}}), false);
});

test('flags the plan streams fallback and session presentation', () => {
  assert.equal(dolbyVisionWarningNeeded({plan: {versionId: 'v1', mode: 'stream', streams: [{id: 'v0', action: 'transcode', reasons: ['dolby_vision_colors_approximate']}]}}), true);
  assert.equal(dolbyVisionWarningNeeded({plan: {versionId: 'v1', mode: 'stream', streams: [{id: 'v0', action: 'copy', reasons: []}]}}), false);
  assert.equal(dolbyVisionWarningNeeded({presentation: {decision: {video: {action: 'transcode', reasons: ['dolby_vision_colors_approximate']}}}}), true);
});

test('older servers without the reason read as no warning', () => {
  assert.equal(dolbyVisionWarningNeeded({}), false);
  assert.equal(dolbyVisionWarningNeeded(null), false);
  assert.equal(dolbyVisionWarningNeeded({decision: {audio: {reasons: ['dolby_vision_colors_approximate']}}}), false);
  assert.equal(dolbyVisionWarningNeeded({plan: {streams: [{id: 'a1', action: 'copy', reasons: ['dolby_vision_colors_approximate']}]}}), true);
});

test('audio-only kinds skip the pre-play check; unknown and video kinds are checked', async () => {
  const {mayCarryVideo} = await import('../src/player/dolby-vision.ts');
  assert.equal(mayCarryVideo('track'), false);
  assert.equal(mayCarryVideo('audiobook'), false);
  assert.equal(mayCarryVideo('movie'), true);
  assert.equal(mayCarryVideo('episode'), true);
  assert.equal(mayCarryVideo(undefined), true);
});
