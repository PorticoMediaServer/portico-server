import test from 'node:test';
import assert from 'node:assert/strict';
import {parseOptions} from '../src/playback-v1/types.ts';

const options = (audio: object[]) => parseOptions({itemId: 'song', kind: 'song', versions: [{id: 'version', audio}]});
test('Options retains existing measured source audio fields without deriving lossless or object audio', () => {
  const track = options([{id: 'a1', codec: 'wavpack', sampleRate: 96000, bitDepth: 24, variant: 'hybrid', objectAudio: 'dtsx', dtsx: true}]).versions[0]!.audio[0]!;
  assert.equal(track.sampleRate, 96000);assert.equal(track.bitDepth, 24);assert.equal(track.variant, 'hybrid');assert.equal(track.objectAudio, 'dtsx');assert.equal(track.dtsx, true);
  assert.equal(track.atmos, false);assert.equal(Object.hasOwn(track, 'lossless'), false);
  const unknown = options([{id: 'a1', codec: 'flac', sampleRate: -1, bitDepth: NaN}]).versions[0]!.audio[0]!;
  assert.equal(unknown.sampleRate, undefined);assert.equal(unknown.bitDepth, undefined);assert.equal(unknown.objectAudio, undefined);assert.equal(unknown.dtsx, undefined);
});
