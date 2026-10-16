import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {feedbackCategoryName} from '../../packages/client-core/src/server-admin/console-catalogue.ts';

// The Feedback screen names a report's category through the shared catalogue word.
const label = (category: string) => feedbackCategoryName(defaultI18n.t, id => defaultI18n.has(id), category);

test('CD-38: known categories keep their labels; unknown taxonomy leaves show as-is', () => {
  assert.equal(label('playback'), 'Playback');
  assert.equal(label('metadata'), 'Metadata');
  assert.equal(label('subtitles'), 'Subtitles');
  assert.equal(label('other'), 'Other');
  assert.equal(label('playback.stall'), 'playback.stall');
  assert.equal(label(''), 'Other');
});
