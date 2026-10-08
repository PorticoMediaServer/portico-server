import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import * as homeModule from '@core/home.ts';

async function customiseApp() {
  const h = hooks();
  const app = await componentModule(new URL('../src/screens/home/Customise.tsx', import.meta.url), {
    react: h.react,
    '@core/home.ts': homeModule,
    '@core/personal-saved.ts': {PersonalSavedService: class {}},
    '../../app/session': {useSession: () => ({api: {}})},
    '../../app/viewer-scope': {useViewerScope: () => ({})},
    '../../app/content': {useService: () => ({service: {}, snapshot: {resources: []}})},
    '../../app/detail': {requestId: async () => 'id'},
    '../../ui': {Button: 'Button', Dialog: 'Dialog', Icon: 'Icon', Loading: 'Loading', Notice: 'Notice', Switch: 'Switch', Text: 'Text', cx: (...parts: unknown[]) => parts.filter(Boolean).join(' ')},
    './Customise.module.css': {default: {}},
    '../../app/errors': {errorText: () => ''},
    '../../app/home': {homeRowTitle: (_id: string, title: string | undefined) => title ?? _id},
    '../../app/libraries': {useLibrariesContext: () => ({items: [], loading: false})},
    '../../app/i18n': {useI18n: () => defaultI18n},
  });
  return app;
}

const view: any = {
  revision: 3,
  rowOrder: ['continue', 'empty_row', 'trending_now'],
  hiddenRowIds: ['trending_now'],
  rows: [
    {id: 'continue', title: 'Continue Watching', kind: 'continue', artworkShape: 'landscape', required: true, hideable: false, reorderable: true, hidden: false},
    {id: 'empty_row', title: 'Empty Row', kind: 'recent', artworkShape: 'poster', libraryId: 'movies', required: false, hideable: true, reorderable: true, hidden: false},
    {id: 'trending_now', title: 'Trending Now', kind: 'community', artworkShape: 'poster', required: false, hideable: true, reorderable: true, hidden: true},
  ],
};

test('customise lists every layout row in order, even an empty one GET /v1/home would omit', async () => {
  const app = await customiseApp();
  const lists = app.customiseLists(view);
  assert.deepEqual(lists.order, ['continue', 'empty_row', 'trending_now']);
  assert.ok(lists.order.includes('empty_row'), 'an empty row stays listed');
  assert.deepEqual(lists.hidden, ['trending_now']);
  assert.equal(!lists.hidden.includes('empty_row'), true);
  // The hidden row's switch is off: checked = !hidden.has(id).
  assert.equal(!new Set(lists.hidden).has('trending_now'), false);
  assert.equal(!new Set(lists.hidden).has('empty_row'), true);
});

test('customise save fences on the loaded view revision and drops locked rows', async () => {
  const app = await customiseApp();
  const lists = app.customiseLists(view);
  const payload = app.customiseSavePayload(view, lists.order, lists.hidden);
  assert.equal(payload.expectedRevision, 3);
  assert.deepEqual(payload.rowOrder, ['continue', 'empty_row', 'trending_now']);
  assert.deepEqual(payload.hiddenRowIds, ['trending_now']);
  const locked = {...view, rows: [...view.rows, {id: 'locked', title: 'Locked', kind: 'continue', artworkShape: 'landscape', required: true, hideable: false, reorderable: false, hidden: false}]};
  const payload2 = app.customiseSavePayload(locked, [...lists.order, 'locked'], lists.hidden);
  assert.ok(!payload2.rowOrder.includes('locked'), 'non-reorderable rows are not sent');
  assert.equal(payload2.expectedRevision, 3);
});

test('P7: Picks for you is one entry, moved and hidden like any row; never a generator', async () => {
  const app = await customiseApp();
  const family = {...view, rows: [...view.rows,
    {id: 'for_you', title: 'Picks for you', titleText: {code: 'home.row.picksForYou', fallback: 'Picks for you'}, kind: 'family', artworkShape: 'poster', required: false, hideable: true, reorderable: true, hidden: true},
    {id: 'for_you:genre:drama', title: 'Drama for you', kind: 'recommendation', artworkShape: 'poster', required: false, hideable: true, reorderable: true, hidden: false},
  ]};
  const lists = app.customiseLists(family);
  assert.deepEqual(lists.order, ['continue', 'empty_row', 'trending_now', 'for_you']);
  assert.deepEqual(lists.hidden, ['trending_now', 'for_you']);
  const payload = app.customiseSavePayload(family, ['for_you', ...lists.order.filter((id: string) => id !== 'for_you')], lists.hidden);
  assert.equal(payload.rowOrder[0], 'for_you');
  assert.ok(!payload.rowOrder.some((id: string) => id.startsWith('for_you:')));
});

test('P6: a saved view is added last, once, and removed by dropping its id', async () => {
  const app = await customiseApp();
  const order = app.addViewRow(['continue', 'recommended'], 'v1');
  assert.deepEqual(order, ['continue', 'recommended', 'view:v1']);
  assert.deepEqual(app.addViewRow(order, 'v1'), order, 'not twice');
  const removed = app.removeViewRow(order, new Set(['view:v1', 'recommended']), 'view:v1');
  assert.deepEqual(removed.order, ['continue', 'recommended']);
  assert.deepEqual([...removed.hidden], ['recommended']);
  // An added view the layout doesn't list yet is still saved, in its place.
  const payload = app.customiseSavePayload(view, [...view.rowOrder, 'view:v2'], ['view:gone']);
  assert.deepEqual(payload.rowOrder, ['continue', 'empty_row', 'trending_now', 'view:v2']);
  assert.deepEqual(payload.hiddenRowIds, [], 'a removed view doesn’t linger as hidden');
});
