import {test} from 'node:test';
import assert from 'node:assert/strict';
import {playbackEvidence} from './page.mjs';

// Checkpoint-only classifier fixtures. Importing the helper never loads a browser.
const frame = {present: true, videoWidth: 640, videoHeight: 360, readyState: 4,
  paused: false, currentTime: 1, duration: 10, error: null};
const samples = patch => [frame, {...frame, currentTime: 2}].map(value => ({...value, ...patch}));

for (const [name, readings, reason] of [
  ['absent video', [{present: false}], /no <video>/],
  ['undecoded response', samples({videoWidth: 0, videoHeight: 0}), /no decoded frame/],
  ['unready video', samples({readyState: 1}), /HAVE_CURRENT_DATA/],
  ['paused video', samples({paused: true}), /stayed paused/],
  ['stalled clock', samples({currentTime: 1}), /did not advance/],
  ['media error', samples({error: {code: 3, message: 'fixture decode error'}}), /video element error 3/],
]) {
  test(`checkpoint playback refuses ${name}`, () => {
    const verdict = playbackEvidence(readings);
    assert.equal(verdict.ok, false);
    assert.match(verdict.reason, reason);
  });
}

test('decoded, ready, unpaused advancing video retains measured playback values', () => {
  const readings = samples({});
  const verdict = playbackEvidence(readings);
  assert.equal(verdict.ok, true);
  assert.equal(verdict.advanced, 1);
  assert.equal(verdict.width, 640);
  assert.equal(verdict.height, 360);
  assert.equal(verdict.duration, 10);
  assert.deepEqual(verdict.readings, readings);
});
