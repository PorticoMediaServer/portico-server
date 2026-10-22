import {test} from 'node:test';
import assert from 'node:assert/strict';
import {lintNextGlossary} from '../scripts/lint-next-glossary.mjs';

test('approved state copy is verbatim while unrelated or modified glossary violations still fail', () => {
  assert.deepEqual(lintNextGlossary({
    'next.overlays.reloadHelp': 'Reload to use the new version.',
    'next.overlays.reload': 'Reload',
    'next.states.reload': 'Reload',
    'next.states.playbackInfo': 'Playback info',
  }), []);
  for (const messages of [
    {'next.unrelated': 'Reload'},
    {'next.states.reload': 'Reload everything'},
    {'next.states.playbackInfo': 'Playback info about this'},
    {'next.states.reload': 'Portico Cloud'},
  ]) assert.ok(lintNextGlossary(messages).length > 0);
});
