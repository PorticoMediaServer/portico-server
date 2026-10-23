import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('legacy reports keep one actual regional capture until refresh, viewer, API or zone changes', async () => {
  const h = hooks();
  let api = {}, scope = {viewerId: 'viewer', serverId: 'server'}, timeZone = 'America/Halifax';
  const m = await componentModule(new URL('../src/screens/server/report-context.ts', import.meta.url), {
    react: h.react, '../../app/session': {useSession: () => ({api})},
    '../../app/viewer-scope': {useViewerScope: () => scope}, '../../app/i18n': {useI18n: () => ({region: {timeZone}})},
  });
  const render = () => h.render(() => m.useReportContext());
  const first = render();
  assert.equal(first.context.timeZone, timeZone);
  assert.equal(Date.parse(first.context.until) % (15 * 60_000), 0);
  assert.equal(render().context, first.context);
  first.refresh();
  const refreshed = render().context;
  assert.notEqual(refreshed, first.context);
  assert.equal(render().context, refreshed);
  api = {};
  const changedAPI = render().context;
  assert.notEqual(changedAPI, refreshed);
  scope = {viewerId: 'other', serverId: 'server'};
  assert.notEqual(render().context, changedAPI);
  timeZone = 'Pacific/Auckland';
  assert.equal(render().context.timeZone, timeZone);
});

test('legacy telemetry and Today summary pass their original canonical context into the real call seam', async () => {
  const h = hooks();
  const context = Object.freeze({timeZone: 'America/Halifax', until: '2026-10-23T13:15:00.000Z'});
  const api = {}, calls: unknown[][] = [], keys: unknown[] = [];
  const client = {telemetry: (...args: unknown[]) => {calls.push(['telemetry', ...args]); return Promise.resolve();}};
  const m = await componentModule(new URL('../src/screens/server/OverviewCharts.tsx', import.meta.url), {
    react: {...h.react, useState: (initial: unknown) => [initial, () => {}]}, '../../app/session': {useSession: () => ({api})},
    '../../app/i18n': {useI18n: () => ({t: (key: string) => key})},
    './report-context': {useReportContext: () => ({context, refresh() {}})},
    '../../admin/console': {useConsole: () => client, useRead: (fn: () => Promise<unknown>, _deps: unknown[], options: {resetKey: unknown}) => {keys.push(options.resetKey); void fn(); return {loading: true};}},
    '@core/play-history.ts': {fetchPlaySummary: (...args: unknown[]) => {calls.push(['summary', ...args]); return Promise.resolve();}},
  });
  h.render(() => m.TelemetryPanel());
  h.render(() => m.ViewingPanel());
  assert.deepEqual(calls, [['telemetry', '1h', context], ['summary', api, {...context, period: 'today'}]]);
  assert.equal(calls[0][2], context);
  assert.deepEqual(keys, [context, context]);
});

test('legacy History retires old pages and cursors before a new report context can show them', async () => {
  const h = hooks();
  const effects: (() => void | (() => void))[] = [];
  const reads: {filters: unknown; cursor: string; signal: AbortSignal; resolve: (page: unknown) => void}[] = [];
  let context = Object.freeze({timeZone: 'America/Halifax', until: '2026-10-23T13:15:00.000Z'});
  const client = {}, api = {};
  const m = await componentModule(new URL('../src/screens/server/PlayHistory.tsx', import.meta.url), {
    react: {...h.react, useEffect: (effect: () => void) => effects.push(effect)},
    '../../app/session': {useSession: () => ({api})}, '../../app/libraries': {useLibrariesContext: () => ({items: []})},
    '../../app/i18n': {useI18n: () => ({t: (key: string) => key})},
    './report-context': {useReportContext: () => ({context, refresh() {}})},
    '../../admin/console': {useConsole: () => client, useRead: () => ({}), useQuietly() {}},
    '../../ui': {Surface: 'surface', Table: 'table', Button: 'button', Select: 'select', Freshness: 'freshness', Text: 'text'},
    '@core/play-history.ts': {fetchPlayHistory: (_api: unknown, filters: unknown, cursor: string, signal: AbortSignal) => new Promise(resolve => reads.push({filters, cursor, signal, resolve})), playHistoryUser: () => 'viewer', playHistoryType: () => 'Movie', playHistoryTitle: () => 'Title', playHistoryPlatform: () => ''},
  });
  const render = () => {effects.length = 0; return h.render(() => m.PlayHistoryPanel());};
  const nodes = (node: any): any[] => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props?.children)] : [];
  render();
  effects.at(-1)!();
  const original = reads[0];
  original.resolve({entries: [{id: 'play', libraryId: 'library'}], nextCursor: 'old-cursor', total: 1});
  await tick();
  const oldTree = render();
  const oldMore = nodes(oldTree).find(node => node.props?.label === 'server.plays.more');
  assert.ok(oldMore);
  context = Object.freeze({...context, timeZone: 'Pacific/Auckland'});
  const newTree = render();
  assert.equal(nodes(newTree).some(node => node.type === 'table'), false);
  assert.equal(nodes(newTree).some(node => node.props?.label === 'server.plays.more'), false);
  oldMore.props.onClick();
  assert.equal(reads.length, 1, 'retired cursor gesture cannot start another request');
  effects.at(-1)!();
  assert.equal(original.signal.aborted, true);
  assert.equal(reads[1].cursor, '');
  assert.deepEqual(reads[1].filters, {...context, period: 'all'});
});
