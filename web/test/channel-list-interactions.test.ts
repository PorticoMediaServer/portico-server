/** Channel-list wiring against the real GuideWindowStore. Browser measurement and leaf kit
 * controls are stubbed; these checks establish callbacks and bounded rows, not browser layout. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {GuideWindowStore, type GuideChannelRow, type GuideProgram} from '@core/guide/index.ts';
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

async function mount(options: {channels?: GuideChannelRow[]} = {}) {
  const h = createRenderer({element: {
    clientWidth: 1220,
    getBoundingClientRect: () => ({top: 64}),
    addEventListener() {},
    removeEventListener() {},
  }});
  const rows = options.channels ?? channels;
  const calls = {watch: [] as GuideChannelRow[], record: [] as unknown[][], open: [] as any[], channel: [] as GuideChannelRow[], menu: [] as unknown[][]};
  let programCalls = 0;
  const source = {
    channels: async (from: number, limit: number) => ({items: rows.slice(from, from + limit), total: rows.length}),
    programs: async (ids: readonly string[], start: number, end: number) => {
      programCalls++;
      if (options.fail?.()) throw Object.assign(new Error('unavailable'), {status: 503});
      const out: Record<string, readonly GuideProgram[]> = {};
      for (const id of ids) out[id] = programs.filter(p => p.channelId === id && p.end > start && p.start < end);
      return out;
    },
  };
  const store = new GuideWindowStore({source, retryMs: []});
  const nowStore = {get: () => Date.now(), getSnapshot: () => Date.now(), subscribe: () => () => {}, dispose() {}};
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
  const shared = {
    store, format: guideFormat(), sources: [{id: 's1', name: 'Living room', type: 'live', position: 0, recordAvailable: true} as any], entry: 'all',
    onOpen: (selection: any) => calls.open.push(selection),
    onWatch: (value: GuideChannelRow) => calls.watch.push(value),
    onRecord: (value: GuideChannelRow, program: GuideProgram) => calls.record.push([value, program]),
    channelMenu: () => [{id: 'favorite', label: t('channels.removeFavorite')}],
    onChannelMenu: (value: GuideChannelRow, id: string) => calls.menu.push([value, id]),
    onChannel: (value: GuideChannelRow) => calls.channel.push(value),
    loadPrograms: async () => [],
    viewport: {current: {firstRow: 0, lastRow: 0}},
  };
  const draw = () => app.ChannelList(shared);
  h.render(draw);
  // Let the page and tile promises settle and each state update reach a render.
  const settle = async (rounds = 12) => {
    for (let i = 0; i < rounds; i++) { await Promise.resolve(); h.render(draw); }
    return tree(h.render(draw));
  };
  return {h, store, calls, draw, render: () => tree(h.render(draw)), settle, programCalls: () => programCalls};
}

const rows = (rendered: any[]) => rendered.filter(node => node.props?.role === 'listitem');
const button = (rendered: any[], label: string) => {
  const found = rendered.find(node => node.type === 'button' && String(node.props?.['aria-label']).includes(label));
  assert.ok(found, `row button for ${label}`);
  return found;
};

test('channel list rows open their channel, watch it and dispatch their own menu action', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    const rendered = await f.settle();
    assert.equal(rows(rendered).length, 3);
    button(rendered, 'One').props.onClick();
    assert.deepEqual(f.calls.channel.map(row => row.id), ['c0']);
    const watch = all(rendered, 'IconButton').find(node => node.props.label === t('channels.watchChannel', {channel: 'Two'}));
    assert.ok(watch);
    watch.props.onClick();
    assert.deepEqual(f.calls.watch.map(row => row.id), ['c1']);
    const menu = all(rendered, 'Menu').find(node => node.props.label === t('channels.moreFor', {channel: 'Three'}));
    assert.ok(menu);
    menu.props.onSelect('favorite');
    assert.deepEqual(f.calls.menu.map(([row, id]: any) => [row.id, id]), [['c2', 'favorite']]);
  } finally { Date.now = clock; }
});

test('an unavailable channel retains its details and menu but has no Watch control', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount({channels: [channel('c0', '1', 'One', {tuneAvailable: false})]});
    const rendered = await f.settle();
    assert.equal(all(rendered, 'IconButton').filter(node => node.props.name === 'play').length, 0);
    button(rendered, 'One').props.onClick();
    assert.equal(f.calls.channel.at(-1)?.id, 'c0');
    assert.equal(all(rendered, 'Menu').length, 1);
    assert.equal(rows(rendered)[0].props.style.opacity, 0.6);
  } finally { Date.now = clock; }
});

test('a channel without guide data says No guide while keeping its channel actions', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount({channels: [channel('c0', '1', 'One', {guide: 'none'})]});
    const rendered = await f.settle();
    assert.ok(button(rendered, 'One').props['aria-label'].includes(t('guide.noGuide')));
    assert.ok(textOf(rows(rendered)[0]).includes(t('guide.noGuide')));
    assert.equal(all(rendered, 'Skeleton').length, 0);
    assert.equal(all(rendered, 'IconButton').filter(node => node.props.name === 'play').length, 1);
  } finally { Date.now = clock; }
});

test('scrolling the channel list moves the bounded store viewport and visible channel actions', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const channels = Array.from({length: 100}, (_, i) => channel(`c${i}`, String(i + 1), `Channel ${i + 1}`, {guide: 'none'}));
    const f = await mount({channels});
    let rendered = await f.settle();
    assert.ok(rows(rendered).length < 20, 'only a viewport and overscan render');
    const scroller = rendered.find(node => node.props?.role === 'list');
    assert.ok(scroller);
    scroller.props.onScroll({currentTarget: {scrollTop: 50 * 88}});
    rendered = await f.settle();
    assert.ok(rows(rendered).length < 20);
    assert.equal(rendered.some(node => node.type === 'button' && String(node.props?.['aria-label']).startsWith('1, ')), false);
    button(rendered, 'Channel 51').props.onClick();
    assert.equal(f.calls.channel.at(-1)?.id, 'c50');
  } finally { Date.now = clock; }
});
