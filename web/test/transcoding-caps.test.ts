import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Transcoding.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/console.ts': {}, '@core/playback-settings.ts': {}, '../../admin/console': {},
  '../../app/session': {}, '../../app/viewer-scope': {}, '../../app/content': {},
  '../../app/i18n': {},
  '../../ui': {}, './Server': {}, './Server.module.css': {},
}) as Promise<any>;

test('CD-46: capacity caps are null for unlimited, otherwise 1–1000000', async () => {
  const {clampCap} = await load();
  assert.equal(clampCap(''), null);
  assert.equal(clampCap('   '), null);
  assert.equal(clampCap('1'), 1);
  assert.equal(clampCap('1000000'), 1000000);
  assert.equal(clampCap('0'), 1, 'zero is never sent');
  assert.equal(clampCap('-3'), 1);
  assert.equal(clampCap('1000001'), 1000000);
  assert.equal(clampCap('25.9'), 25);
});
