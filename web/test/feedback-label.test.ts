import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Feedback.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  './MaintenanceWindows': {}, '@core/console.ts': {}, '../../admin/console': {},
  '../../app/i18n': {},
  '../../ui': {}, './Server': {}, './Server.module.css': {},
}) as Promise<any>;

test('CD-38: known categories keep their labels; unknown taxonomy leaves show as-is', async () => {
  const {feedbackCategoryLabel} = await load();
  assert.equal(feedbackCategoryLabel('playback'), 'Playback');
  assert.equal(feedbackCategoryLabel('metadata'), 'Metadata');
  assert.equal(feedbackCategoryLabel('subtitles'), 'Subtitles');
  assert.equal(feedbackCategoryLabel('other'), 'Other');
  assert.equal(feedbackCategoryLabel('playback.stall'), 'playback.stall');
});
