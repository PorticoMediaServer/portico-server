import test from 'node:test';
import assert from 'node:assert/strict';
import {watchMusicSignal} from './signal.ts';
import type {BrowserAudioFacts} from '../../../platform/playback/audio/output';

/** A minimal accepted render recipe: pinned and current agree, so outsidePlan is false. */
const recipe = {id: 'plan-1', mode: 'decode', codec: 'flac', sampleRate: 96000, channels: 2, bitDepth: 24};
const snapshot = (deliveredSampleRate: number) => ({
  session: {id: 'session-1', kind: 'audio', observedPlayback: {audio: undefined}, playbackInfo: {
    route: 'direct', losses: [], delivered: {audio: {codec: 'flac', bitDepth: 24, sampleRate: deliveredSampleRate, channels: 2}},
    source: {audio: {codec: 'flac', bitDepth: 24, sampleRate: 96000, channels: 2, layout: 'stereo', lossless: true}},
  }, presentation: {audio: recipe}},
  playback: {audioEffects: {rendering: true, eqApplied: false, levelingApplied: false, settings: {normalization: 'off'}}, session: {id: 'session-1', audio: recipe}, musicFallback: undefined},
} as never);
const facts = (over: Partial<BrowserAudioFacts> = {}): BrowserAudioFacts => ({
  engine: 'web-audio', bitPerfect: false, sourceRate: 96000, contextRate: 48000, resampledToContext: true,
  deviceRate: null, browserMayResample: true, sourceChannels: 2, outputChannels: 2, eqApplied: false, levelingApplied: false, ...over,
} as BrowserAudioFacts);

test('96k into a 48k context is reported as browser resampling at the context rate', () => {
  const model = watchMusicSignal(snapshot(96000), null, facts());
  assert.equal(model?.output.sampleRate, 48000);
  assert.equal(model?.output.resampledBy, 'browser');
  assert.notEqual(model?.output.bitPerfect, true);
});

test('a server conversion to 48k into a 48k context is not browser resampling', () => {
  const model = watchMusicSignal(snapshot(48000), null, facts({sourceRate: 48000, contextRate: 48000, resampledToContext: false}));
  assert.equal(model?.output.sampleRate, 48000);
  assert.equal(model?.output.resampledBy, undefined);
});

test('facts for another recipe, or internally inconsistent facts, are refused', () => {
  assert.equal(watchMusicSignal(snapshot(96000), null, facts({sourceChannels: 6}))?.output.sampleRate, undefined);
  assert.equal(watchMusicSignal(snapshot(96000), null, facts({sourceRate: 44100}))?.output.sampleRate, undefined);
  assert.equal(watchMusicSignal(snapshot(96000), null, facts({resampledToContext: false}))?.output.sampleRate, undefined);
  assert.equal(watchMusicSignal(snapshot(96000), null, facts({contextRate: 0}))?.output.sampleRate, undefined);
  assert.equal(watchMusicSignal(snapshot(96000), null, facts({deviceRate: 48000 as never}))?.output.sampleRate, undefined);
});

test('absent facts keep the committed comparison, and no layout or bit-perfect is invented', () => {
  const model = watchMusicSignal(snapshot(96000), null);
  assert.equal(model?.output.sampleRate, undefined);
  const local = watchMusicSignal(snapshot(96000), null, facts());
  assert.equal(local?.output.decodedLayout, undefined);
  assert.equal(local?.output.passthrough, undefined);
  assert.equal(local?.output.bitDepth, undefined);
  assert.notEqual(local?.output.bitPerfect, true);
});
