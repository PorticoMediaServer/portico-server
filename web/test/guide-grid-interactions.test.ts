/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the merged guide grid's drawn geometry and its controls.
 *
 * The real `GuideGrid` and its rows, headers, ruler and cells run here against a real
 * `GuideWindowStore`; only the host element, the browser sensors, the now-store tick and the
 * leaf kit controls are stubbed. Host elements are inert: the assertions are about the props
 * the screen would give a browser (positions, focus stop, callbacks), not React scheduling,
 * layout or painting. Visual capture stays with Visual QA.
 */
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
const COLUMN_PX = 240, LANE_PX = 998, PAGE_PX = 8; // 30-minute drawn column: 8 px a minute.

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

async function mount(options: {channels?: GuideChannelRow[]; fail?: () => boolean; range?: {start: number; end: number}} = {}) {
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
  const draw = () => app.GuideGrid({...shared, range: options.range ?? {start: T - 3 * HOUR, end: T + 6 * DAY}});
  h.render(draw);
  // Let the page and tile promises settle and each state update reach a render.
  const settle = async (rounds = 12) => {
    for (let i = 0; i < rounds; i++) { await Promise.resolve(); h.render(draw); }
    return tree(h.render(draw));
  };
  return {h, store, calls, draw, render: () => tree(h.render(draw)), settle, programCalls: () => programCalls};
}

const role = (rendered: any[], value: string) => rendered.filter(node => node.props?.role === value);
const cells = (rendered: any[]) => role(rendered, 'gridcell');
const cell = (rendered: any[], title: string) => {
  const found = cells(rendered).filter(node => String(node.props['aria-label']).includes(title));
  assert.equal(found.length, 1, `one cell titled ${title}`);
  return found[0];
};
const findText = (node: any, wanted: string) => {
  if (!node || typeof node !== 'object' || node.type === undefined) return false;
  return textOf(node).includes(wanted);
};
const onlyText = (node: any) => textOf(node.props?.children).trim();
const nowTab = (rendered: any[]) => rendered.find(node => node.type === 'span' && onlyText(node) === t('guide.now') && typeof node.props?.style?.left === 'number');
function press(rendered: any[], key: string, init: Record<string, unknown> = {}, inCell = true) {
  const grid = rendered.find(node => node.props?.role === 'grid' && typeof node.props?.onKeyDown === 'function');
  assert.ok(grid, 'the grid handles keys');
  const target = new (globalThis as any).HTMLElement();
  target.closest = (selector: string) => (inCell && selector === '[role="gridcell"]' ? {dataset: {}} : null);
  let prevented = false;
  grid.props.onKeyDown({key, target, preventDefault: () => { prevented = true; }, ...init});
  return prevented;
}

test('the drawn guide scale places rows, 30/60/90-minute cells and the 2px gap without overflow', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    const rendered = await f.settle();
    const rows = role(rendered, 'row').filter(node => node.props?.style?.top !== undefined);
    assert.deepEqual(rows.map(node => node.props.style.top).sort((a: number, b: number) => a - b), [0, 72, 144]);
    assert.equal(cell(rendered, 'Thirty').props.style.width, COLUMN_PX - 2);
    assert.equal(cell(rendered, 'Sixty A').props.style.width, 2 * COLUMN_PX - 2);
    assert.equal(cell(rendered, 'Sixty B').props.style.width, 2 * COLUMN_PX - 2);
    assert.equal(cell(rendered, 'Ninety').props.style.width, 3 * COLUMN_PX - 2);
    // A minute is eight pixels; the view starts 32 minutes before now (now at 25% of the span).
    assert.equal(cell(rendered, 'Thirty').props.style.left, 2 * PAGE_PX);
    const thirty = cell(rendered, 'Thirty').props.style, sixty = cell(rendered, 'Sixty A').props.style;
    assert.equal(sixty.left - (thirty.left + thirty.width), 2, 'the drawn 2px gap between adjacent programmes');
    for (const node of cells(rendered)) {
      assert.ok(Number.isFinite(node.props.style.left) && Number.isFinite(node.props.style.width));
      assert.ok(node.props.style.left >= 0 && node.props.style.left + node.props.style.width <= LANE_PX, 'a cell stays inside the lane');
    }
    assert.ok(findText({type: 'div', props: {children: rendered}}, t('guide.keyboardHint')), 'the drawn keyboard hint');
  } finally { Date.now = clock; }
});

test('the ruler ticks and the now line use the same time scale, offset by the channel column', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    const rendered = await f.settle();
    const tab = nowTab(rendered);
    assert.ok(tab, 'now is inside the drawn span');
    assert.ok(tab.props.style.left > 0 && tab.props.style.left < LANE_PX);
    const line = rendered.find(node => node.props?.['aria-hidden'] === true && typeof node.props?.style?.left === 'number' && node.props.style.left > 220);
    assert.ok(line, 'the now line is drawn');
    assert.equal(line.props.style.left - tab.props.style.left, 220, 'the channel column offsets the lane');
    const tick = rendered.find(node => onlyText(node) === defaultI18n.time(T + 30 * MIN) && typeof node.props?.style?.left === 'number');
    assert.ok(tick, 'the next half-hour tick is labelled');
    assert.equal(tick.props.style.left, tab.props.style.left + COLUMN_PX, 'one 30-minute column is 240px');
  } finally { Date.now = clock; }
});

test('arrow keys move the one focus stop between rows and cells, and put browser focus there', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    let rendered = await f.settle();
    const focused = () => cells(rendered).filter(node => node.props['data-focused'] === 'true');
    assert.equal(focused().length, 1);
    assert.ok(String(focused()[0].props['aria-label']).includes('Sixty A'));
    assert.equal(cells(rendered).filter(node => node.props.tabIndex === 0).length, 1, 'one tab stop');

    assert.equal(press(rendered, 'ArrowDown'), true);
    rendered = f.render();
    assert.ok(String(focused()[0].props['aria-label']).includes('Sixty B'));
    assert.equal(f.h.focused.at(-1), 'first-result', 'the moved cell receives browser focus');

    press(rendered, 'ArrowUp'); rendered = f.render();
    assert.ok(String(focused()[0].props['aria-label']).includes('Sixty A'));
    press(rendered, 'ArrowLeft'); rendered = f.render();
    assert.ok(String(focused()[0].props['aria-label']).includes('Thirty'));
    press(rendered, 'ArrowRight'); rendered = f.render();
    assert.ok(String(focused()[0].props['aria-label']).includes('Sixty A'));

    // Keys outside a grid cell (the channel header, a menu) are not the grid's.
    assert.equal(press(rendered, 'ArrowDown', {}, false), false);
    rendered = f.render();
    assert.ok(String(focused()[0].props['aria-label']).includes('Sixty A'));
  } finally { Date.now = clock; }
});

test('Home, End and PageDown clamp to the drawn channel rows', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    let rendered = await f.settle();
    press(rendered, 'End'); rendered = f.render();
    assert.ok(String(cells(rendered).find(node => node.props['data-focused'] === 'true').props['aria-label']).includes('Ninety'));
    press(rendered, 'Home'); rendered = f.render();
    assert.ok(String(cells(rendered).find(node => node.props['data-focused'] === 'true').props['aria-label']).includes('Sixty A'));
    press(rendered, 'PageDown'); rendered = f.render();
    assert.ok(String(cells(rendered).find(node => node.props['data-focused'] === 'true').props['aria-label']).includes('Ninety'), 'a screenful clamps at the last row');
    press(rendered, 'PageUp'); rendered = f.render();
    assert.ok(String(cells(rendered).find(node => node.props['data-focused'] === 'true').props['aria-label']).includes('Sixty A'));
  } finally { Date.now = clock; }
});

test('Enter opens the focused programme, W watches its row and R records that programme', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    let rendered = await f.settle();
    press(rendered, 'Enter');
    assert.equal(f.calls.open.at(-1).channel.id, 'c0');
    assert.equal(f.calls.open.at(-1).program.id, 'p60');
    press(rendered, ' ');
    assert.equal(f.calls.open.length, 2, 'Space opens like Enter');
    press(rendered, 'w');
    assert.deepEqual(f.calls.watch.map(row => row.id), ['c0']);
    press(rendered, 'r');
    assert.deepEqual(f.calls.record.map(([row, program]: any) => [row.id, program.id]), [['c0', 'p60']]);
    press(rendered, 'w', {ctrlKey: true}); press(rendered, 'r', {metaKey: true});
    assert.deepEqual(f.calls.watch.map(row => row.id), ['c0'], 'a modifier does not watch');
    assert.equal(f.calls.record.length, 1, 'a modifier does not record');

    press(rendered, 'ArrowDown'); rendered = f.render();
    press(rendered, 'w'); press(rendered, 'r');
    assert.deepEqual(f.calls.watch.map(row => row.id), ['c0', 'c1']);
    assert.equal(f.calls.record.at(-1)[1].id, 'p60b');
  } finally { Date.now = clock; }
});

test('a day chip jumps to prime time and the Now button and N come back to now', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    let rendered = await f.settle();
    const chips = () => all(rendered, 'Chip');
    assert.equal(chips().find((chip: any) => chip.props.label === t('guide.today')).props.pressed, true);
    chips().find((chip: any) => chip.props.label === t('guide.tomorrow')).props.onClick();
    rendered = f.render();
    assert.equal(chips().find((chip: any) => chip.props.label === t('guide.today')).props.pressed, false);
    assert.equal(chips().find((chip: any) => chip.props.label === t('guide.tomorrow')).props.pressed, true);
    const dayMenu = all(rendered, 'Menu').find((menu: any) => menu.props.label === t('guide.day'));
    assert.equal(dayMenu.props.items.find((item: any) => item.selected).label, t('guide.tomorrow'));
    assert.equal(nowTab(rendered), undefined, 'tomorrow is outside the drawn now');
    const nowButton = () => all(rendered, 'Button').find((button: any) => button.props.label === t('guide.now'));
    assert.equal(nowButton().props['aria-label'], t('guide.nowEarlier'));
    nowButton().props.onClick();
    rendered = f.render();
    assert.ok(nowTab(rendered));
    assert.equal(chips().find((chip: any) => chip.props.label === t('guide.today')).props.pressed, true);
    assert.equal(nowButton().props['aria-label'], t('guide.now'));

    const later = () => rendered.find(node => node.type === 'button' && node.props?.['aria-label'] === t('guide.later'));
    later().props.onClick(); rendered = f.render();
    later().props.onClick(); rendered = f.render();
    assert.equal(nowTab(rendered), undefined, 'four hours later is outside the drawn now');
    press(rendered, 'n');
    rendered = f.render();
    assert.ok(nowTab(rendered), 'N jumps the view back to now');
  } finally { Date.now = clock; }
});

test('the two-hour ruler edges move the view and disappear at the published range ends', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount({range: {start: T - HOUR, end: T + 6 * DAY}});
    let rendered = await f.settle();
    const edge = (label: string) => rendered.find(node => node.type === 'button' && node.props?.['aria-label'] === label);
    assert.ok(edge(t('guide.earlier')));
    assert.ok(edge(t('guide.later')));
    edge(t('guide.earlier')).props.onClick();
    rendered = f.render();
    assert.equal(edge(t('guide.earlier')), undefined, 'no earlier than the range start');
    assert.ok(edge(t('guide.later')));
    edge(t('guide.later')).props.onClick();
    rendered = f.render();
    assert.ok(edge(t('guide.earlier')), 'two hours from the start is inside the range');
  } finally { Date.now = clock; }
});

test('a failed part of the guide says so and Try again fetches it again', async () => {
  const clock = Date.now; Date.now = () => T;
  let failing = true;
  try {
    const f = await mount({fail: () => failing});
    let rendered = await f.settle();
    assert.equal(cells(rendered).length, 0);
    assert.ok(findText({type: 'div', props: {children: rendered}}, t('guide.tileFailed')));
    const retries = all(rendered, 'Button').filter((button: any) => button.props.label === t('action.tryAgain'));
    assert.equal(retries.length, 3);
    const before = f.programCalls();
    failing = false;
    retries[0].props.onClick();
    rendered = await f.settle();
    assert.ok(f.programCalls() > before, 'Try again asks the source again');
    assert.equal(all(rendered, 'Button').filter((button: any) => button.props.label === t('action.tryAgain')).length, 0);
    assert.ok(String(cell(rendered, 'Sixty A').props['aria-label']).includes('Sixty A'));
  } finally { Date.now = clock; }
});

test('a channel with no guide draws one full-lane No information cell and offers no record', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount({channels: [channel('c0', '1', 'One', {guide: 'none'})]});
    const rendered = await f.settle();
    assert.equal(cells(rendered).length, 1);
    const blank = cells(rendered)[0];
    assert.ok(String(blank.props['aria-label']).includes(t('guide.noInformation')));
    assert.equal(blank.props.style.left, 0);
    assert.equal(blank.props.style.width, LANE_PX - 2);
    press(rendered, 'Enter');
    assert.equal(f.calls.open.at(-1).channel.id, 'c0');
    assert.equal(f.calls.open.at(-1).program, undefined);
    press(rendered, 'r');
    assert.deepEqual(f.calls.record, []);
  } finally { Date.now = clock; }
});

test('the drawn channel header, channel menu and cells call their own actions', async () => {
  const clock = Date.now; Date.now = () => T;
  try {
    const f = await mount();
    const rendered = await f.settle();
    const header = rendered.find(node => node.type === 'button' && String(node.props?.['aria-label']).startsWith(t('channels.channel', {number: '1'})));
    assert.ok(header, 'the channel name is a button');
    header.props.onClick();
    assert.equal(f.calls.channel.at(-1).id, 'c0');
    const menu = all(rendered, 'Menu').find((node: any) => node.props.label === t('channels.moreFor', {channel: 'One'}));
    assert.ok(menu, 'the row has its more menu');
    menu.props.onSelect('favorite');
    assert.deepEqual(f.calls.menu.at(-1).map((value: any) => value.id ?? value), ['c0', 'favorite']);

    cell(rendered, 'Sixty A').props.onClick();
    assert.equal(f.calls.open.at(-1).program.id, 'p60');
    const sixty = cell(rendered, 'Sixty A').props.style;
    const gap = cells(rendered).find(node => String(node.props['aria-label']).includes(', One') && String(node.props['aria-label']).includes(t('guide.noInformation')) && node.props.style.left > sixty.left + sixty.width);
    assert.ok(gap, 'the hole after the drawn programme');
    gap.props.onClick();
    assert.equal(f.calls.open.at(-1).channel.id, 'c0');
    assert.equal(f.calls.open.at(-1).program, undefined);
  } finally { Date.now = clock; }
});
