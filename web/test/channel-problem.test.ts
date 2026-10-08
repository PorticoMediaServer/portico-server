import test from 'node:test';
import assert from 'node:assert/strict';
import {channelProblemId} from '../src/player/channel-problem.ts';
import {enUS} from '../../packages/i18n/src/catalog/index.ts';

test('every recoverable channel code maps to a catalogued plain message; unknown codes get the general line', () => {
  const cases: [string | undefined, string][] = [
    ['channel_start_failed', 'web.channelProblem.startFailed'],
    ['source_unavailable', 'web.channelProblem.sourceUnavailable'],
    ['timeshift_storage_unavailable', 'web.channelProblem.storage'],
    ['channel_preparation_stalled', 'web.channelProblem.stalled'],
    ['something_new', 'web.channelProblem.general'],
    [undefined, 'web.channelProblem.general'],
  ];
  for (const [code, id] of cases) {
    assert.equal(channelProblemId(code), id);
    const text = (enUS as Record<string, string>)[id];
    assert.ok(text, `${id} is catalogued`);
    if (code) assert.ok(!text.includes(code), 'never shows the code');
  }
});
