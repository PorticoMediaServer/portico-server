/** Channel timeline callbacks, loading recovery and bounded paging. Leaf kit/browser APIs
 * are stubs; this checks screen wiring against explicit programme-source responses. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {type GuideChannelRow, type GuideProgram} from '@core/guide/index.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const MIN = 60_000, HOUR = 60 * MIN, DAY = 24 * HOUR;
/** 24 Oct 2026 18:00 UTC: a 3-hour tile and half-hour slot boundary, so ticks and blocks are exact. */
const T = Date.UTC(2026, 9, 24, 18, 0, 0);

const channel = (id: string, number: string, name: string, extra: Partial<GuideChannelRow> = {}): GuideChannelRow =>
  ({id, sourceId: 's1', kind: 'live', number, name, group: '', favorite: true, tuneAvailable: true, recordAvailable: true, guide: 'full', ...extra});
const channels = [channel('c0', '1', 'One'), channel('c1', '2', 'Two'), channel('c2', '3', 'Three')];
const programme = (id: string, channelId: string, title: string, start: number, end: number): GuideProgram => ({id, channelId, title, start, end});
const programs = [
  programme('p30', 'c0', 'Thirty', T - 30 * MIN, T),
  programme('p60', 'c0', 'Sixty A', T, T + 60 * MIN),
  programme('p60b', 'c1', 'Sixty B', T - 25 * MIN, T + 35 * MIN),
  programme('p90', 'c2', 'Ninety', T - 20 * MIN, T + 70 * MIN),
];

// Installed once for this file. The guide's fill-height effect schedules its 300 ms re-measure,
// which can fire after a test has ended; the stub stays inert for the whole file instead of
// being torn down under it. The test files run in their own process.
(function installDom() {
  const global = globalThis as any;
  global.HTMLElement = class {};
  global.ResizeObserver = class { observe() {} disconnect() {} };
  global.IntersectionObserver = class { observe() {} disconnect() {} };
  global.requestAnimationFrame = () => 0;
  global.window = {innerHeight: 800, addEventListener() {}, removeEventListener() {}};
})();

function guideFormat() {
  const dayKey = (ms: number) => new Intl.DateTimeFormat('en-CA', {year: 'numeric', month: '2-digit', day: '2-digit', timeZone: 'UTC'}).format(new Date(ms));
  const at = T;
  return {
    zone: 'UTC',
    clock: (ms: number) => defaultI18n.time(ms),
    dayLabel: (ms: number, now = at) => {
      const key = dayKey(ms);
      if (key === dayKey(now)) return t('guide.today');
      if (key === dayKey(now + DAY)) return t('guide.tomorrow');
      if (key === dayKey(now - DAY)) return t('guide.yesterday');
      return defaultI18n.date(ms, 'weekday');
    },
    range: (a: number, b: number) => t('guide.range', {start: defaultI18n.time(a), end: defaultI18n.time(b)}),
    t,
    duration: (seconds: number) => defaultI18n.duration(seconds, 'short'),
  };
}

async function mount(options: {channel?: GuideChannelRow; fail?: () => boolean; empty?: boolean} = {}) {
  const h = createRenderer();
  const calls = {open: [] as any[], watch: [] as GuideChannelRow[], close: 0, loads: [] as any[]};
  const selected = options.channel ?? channels[0];
  const nowStore = {get: () => T, getSnapshot: () => T, subscribe: () => () => {}, dispose() {}};
  const app = await componentModule(new URL('../src/screens/channels/Channels.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useParams: () => ({}), useSearch: () => ({}), useNavigate: () => () => Promise.resolve()},
    '../../app/channels': {
      ALL: 'all', LIBRARY: 'library', RECORDINGS: 'recordings',
      channelEntries: () => [], defaultEntry: () => 'all', hasReminder: () => false, readReminders: () => [],
      removeReminder: () => {}, sourceName: (list: any[], id: string) => list.find(source => source.id === id)?.name ?? '',
      setChannelSurf: () => {}, forgetGuideReads: () => {}, channelReason: () => '', serverWideOf: () => undefined,
      subscribeReminders: () => () => {}, toggleReminder: () => {},
      useChannelMemory: () => ({view: 'guide', sort: 'number', setView() {}, setSort() {}}),
      useChannelSources: () => ({sources: [{id: 's1', name: 'Living room', type: 'live', position: 0, recordAvailable: true}], loading: false, failed: false, refresh() {}}),
      channelGuideSource: () => ({}), localTimezone: () => 'UTC',
    },
    '../../app/session': {useSession: () => ({api: {}, session: {viewer: {serverId: 'server'}}, owner: false})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/errors': {ErrorState: 'ErrorState', errorText: () => ''},
    '../../player/PlayerContext': {usePlayerActions: () => ({playSequence() {}, play() {}})},
    '../live/guide': {useDVRClient: () => null},
    '../live/now-store': {createNowStore: () => nowStore},
    '../live/DVR': {DVRView: 'DVRView'},
    '../../app/freshness': {useQuietRefresh: () => {}},
    './Channels.module.css': {default: {}},
    '../live/dvr-groups': {dvrPaddings: {}, dvrLimits: {}},
    '../../ui': {
      Artwork: 'Artwork', Badge: 'Badge', Button: 'Button', Chip: 'Chip', Chips: 'Chips', Dialog: 'Dialog', Icon: 'Icon',
      IconButton: 'IconButton', Menu: 'Menu', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', Segmented: 'Segmented',
      Select: 'Select', Skeleton: 'Skeleton', StateView: 'StateView', Text: 'Text', Toast: 'Toast',
      cx: (...names: unknown[]) => names.filter(Boolean).join(' '),
      useArtworkUrl: () => ({url: ''}), useCompact: () => false, useHoverCapable: () => true,
    },
  });
  const range = {start: T - 3 * HOUR, end: T + 9 * HOUR};
  const loadPrograms = async (id: string, start: number, end: number, signal: AbortSignal) => {
    calls.loads.push({id, start, end, signal});
    if (options.fail?.()) throw new Error('unavailable');
    return options.empty ? [] : programs.filter(p => p.channelId === id && p.end > start && p.start < end);
  };
  const draw = () => app.ChannelTimeline({channel: selected, format: guideFormat(), range, loadPrograms,
    showSource: true, sources: [{id: 's1', name: 'Living room'}],
    onOpen: (selection: any) => calls.open.push(selection),
    onWatch: (row: GuideChannelRow) => calls.watch.push(row), onClose: () => { calls.close++; },
  });
  const settle = async () => {
    for (let i = 0; i < 12; i++) { await Promise.resolve(); h.render(draw); }
    return tree(h.render(draw));
  };
  h.render(draw);
  return {calls, settle, render: () => tree(h.render(draw))};
}

function freeze(context: any) {
  context.mock.timers.enable({apis: ['setInterval']});
  context.mock.method(Date, 'now', () => T);
}

test('timeline rows open their exact programme; Watch watches the channel and closes', async context => {
  freeze(context);
  const f = await mount();
  const rendered = await f.settle();
  const dialog = all(rendered, 'Dialog')[0];
  assert.equal(dialog.props.title, '1 · One');
  assert.equal(dialog.props.description, 'Living room');
  const current = rendered.find(node => node.type === 'button' && node.props?.['data-now'] === 'true');
  assert.ok(current);
  current.props.onClick();
  assert.equal(f.calls.open.at(-1).channel.id, 'c0');
  assert.equal(f.calls.open.at(-1).program.id, 'p60');
  const watch = all(rendered, 'Button').find(node => node.props.label === t('guide.watchLive'));
  assert.ok(watch);
  assert.equal(watch.props.disabled, false);
  watch.props.onClick();
  assert.deepEqual(f.calls.watch.map(row => row.id), ['c0']);
  assert.equal(f.calls.close, 1);
  dialog.props.onOpenChange(true);
  assert.equal(f.calls.close, 1);
  dialog.props.onOpenChange(false);
  assert.equal(f.calls.close, 2);
});

test('an unavailable channel disables the timeline Watch action', async context => {
  freeze(context);
  const f = await mount({channel: channel('c0', '1', 'One', {tuneAvailable: false})});
  const rendered = await f.settle();
  const watch = all(rendered, 'Button').find(node => node.props.label === t('guide.watchLive'));
  assert.ok(watch);
  assert.equal(watch.props.disabled, true);
  assert.deepEqual(f.calls.watch, []);
});

test('failed timeline reads report failure and Try again requests the same bounded block', async context => {
  freeze(context);
  let fail = true;
  const f = await mount({fail: () => fail});
  let rendered = await f.settle();
  const notice = all(rendered, 'Notice')[0];
  assert.ok(notice);
  assert.ok(textOf(notice).includes(t('guide.tileFailed')));
  assert.equal(notice.props.action.label, t('action.tryAgain'));
  assert.equal(f.calls.loads.length, 1);
  const first = f.calls.loads[0];
  assert.equal(first.id, 'c0');
  assert.ok(first.end > first.start && first.end - first.start <= 6 * HOUR);
  fail = false;
  notice.props.action.onClick();
  rendered = await f.settle();
  assert.equal(f.calls.loads.length, 2);
  assert.equal(f.calls.loads[1].start, first.start);
  assert.equal(f.calls.loads[1].end, first.end);
  assert.equal(all(rendered, 'Notice').length, 0);
  assert.ok(rendered.some(node => node.type === 'button' && node.props?.['data-now'] === 'true'));
});

test('an empty timeline finishes loading and says No guide', async context => {
  freeze(context);
  const f = await mount({empty: true});
  const rendered = await f.settle();
  assert.ok(rendered.some(node => node.type === 'Text' && textOf(node).includes(t('guide.noGuide'))));
  assert.equal(all(rendered, 'Skeleton').length, 0);
  assert.equal(all(rendered, 'Notice').length, 0);
  assert.equal(f.calls.loads.length, 1);
});
