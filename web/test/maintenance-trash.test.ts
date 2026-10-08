import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/Maintenance.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  './Backups': {}, './MaintenanceWindows': {}, './AlertThresholds': {},
  '@core/administration.ts': {}, '../../admin/console': {}, '../../admin/cursor-read': {},
  './ListPager': {}, './Server': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
}) as Promise<any>;

const page = (items: number, heldCount: number) => ({items: Array.from({length: items}, (_, n) => n), heldCount});

test('CD-42: due runs only when the loaded page is the whole held listing', async () => {
  const {isFullTrashList} = await load();
  assert.equal(isFullTrashList(page(100, 100), false, false), true, 'all held titles on one page');
  assert.equal(isFullTrashList(page(0, 0), false, false), true, 'empty trash is trivially whole');
  assert.equal(isFullTrashList(page(100, 250), false, true), false, 'a next page means more to load');
  assert.equal(isFullTrashList(page(50, 250), true, false), false, 'a last partial page is not the whole list');
  assert.equal(isFullTrashList(page(100, 101), true, false), false, 'paged history is not the whole list');
  assert.equal(isFullTrashList(undefined, false, false), false, 'nothing loaded yet');
});
