import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

test('FEAT-07: live captions offer caption and subtitle tracks only', async () => {
  const {captionTracks} = await componentModule(new URL('../src/player/LiveCaptions.tsx', import.meta.url), {react: {default: {}, useEffect: () => {}, useState: (v: unknown) => [v, () => {}]}, '../ui': {}, '../app/i18n': {}}) as typeof import('../src/player/LiveCaptions.tsx');
  const tracks = [{kind: 'metadata'}, {kind: 'captions', label: 'CC1'}, {kind: 'subtitles', label: 'English'}, {kind: 'chapters'}] as unknown as TextTrack[];
  assert.deepEqual(captionTracks(tracks).map(t => t.label), ['CC1', 'English']);
  assert.deepEqual(captionTracks(undefined), []);
});
