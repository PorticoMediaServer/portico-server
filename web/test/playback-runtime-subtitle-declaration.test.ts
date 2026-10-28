import test from 'node:test';
import assert from 'node:assert/strict';
import {declareSubtitleRendering, subtitleRenderingKinds} from '../src/platform/playback/subtitle-declaration.ts';

test('runtime declares its actual text surface without advertising absent styled or image renderers', () => {
  assert.deepEqual(subtitleRenderingKinds(), ['text']);
  const before = {engines: {native: {subtitleRendering: ['ass', 'pgs'], audio: ['aac']}, hls: {subtitleRendering: ['dvb']}}};
  assert.deepEqual(declareSubtitleRendering(before), {engines: {native: {subtitleRendering: ['text'], audio: ['aac']}, hls: {subtitleRendering: ['text']}}});
  assert.deepEqual(before.engines.native.subtitleRendering, ['ass', 'pgs'], 'the caller capability document is not mutated');
});

test('runtime legacy capability shape also removes styled and bitmap drawing claims', () => {
  assert.deepEqual(declareSubtitleRendering({subtitles: {text: ['webvtt'], styled: ['ass'], bitmap: ['pgs']}}), {subtitles: {text: ['webvtt'], styled: [], bitmap: []}});
});
