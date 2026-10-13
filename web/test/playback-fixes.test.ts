import test from 'node:test';
import assert from 'node:assert/strict';
import {parsePlaybackFixes} from '../src/bridge/playback-fixes.ts';

test('only a stored true turns a playback fix on', () => {
  assert.deepEqual(parsePlaybackFixes(undefined), {forceStereo: false, disablePassthrough: false, disableHdr: false});
  assert.deepEqual(parsePlaybackFixes({forceStereo: true, disableHdr: 'yes', disablePassthrough: 1, extra: true}), {forceStereo: true, disablePassthrough: false, disableHdr: false});
  assert.deepEqual(parsePlaybackFixes('nonsense'), {forceStereo: false, disablePassthrough: false, disableHdr: false});
});
