import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Libraries.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '../../admin/metadata-agent': {},
  './LibrarySettings': {}, './MetadataLookup': {}, './MetadataSource': {}, '../../app/i18n': {},
  '@tanstack/react-router': {}, '@core/library-management.ts': {}, '@core/library-inventory.ts': {},
  '../../admin/console': {}, '../../app/content': {}, '@core/presentation/index.ts': {},
  '../../app/session': {}, '../../app/viewer-scope': {}, '../../app/libraries': {}, '../../ui': {}, './Server': {},
}) as Promise<any>;

test('CD-08: a new folder submits the loaded configuration revision, never zero', async () => {
  const {addSourceSettings, canAddSource} = await load();
  // A second folder added at revision N submits N.
  assert.equal(addSourceSettings(7).expectedRevision, 7);
  assert.equal(addSourceSettings(1).expectedRevision, 1);
  // An unloaded configuration (or a zero revision) cannot submit.
  assert.equal(canAddSource(undefined), false);
  assert.equal(canAddSource(0), false);
  assert.equal(canAddSource(7), true);
});
