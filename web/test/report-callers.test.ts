import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import {FreshnessHub} from '../../packages/client-core/src/freshness-hub.ts';
import {ConsoleClient} from '../../packages/client-core/src/console.ts';
import {METRIC_NAMES} from '../../packages/client-core/src/telemetry.ts';

const tick = () => new Promise(resolve => setImmediate(resolve));
const deferred = () => {
  let resolve: (value: any) => void;
  const promise = new Promise<any>(done => { resolve = done; });
  return {promise, resolve: (value: any) => resolve(value)};
};
function atTime(at: number, fn: () => void) {
  const RealDate = globalThis.Date;
  globalThis.Date = class extends RealDate {
    constructor(value?: any) { super(value === undefined ? at : value); }
    static now() { return at; }
  } as typeof Date;
  try { fn(); } finally { globalThis.Date = RealDate; }
}
function engine() {
  const h = hooks();
  const effects: {deps: readonly unknown[]; cleanup?: () => void}[] = [];
  let index = 0, pending: (() => void)[] = [];
  h.react.useCallback = (fn: any, deps: unknown[]) => h.react.useMemo(() => fn, deps);
  h.react.useEffect = (fn: () => undefined | (() => void), deps: unknown[]) => {
    const n = index++;
    if (effects[n] && deps.every((value, i) => effects[n].deps[i] === value)) return;
    pending.push(() => {
      effects[n]?.cleanup?.();
      effects[n] = {deps, cleanup: fn() ?? undefined};
    });
  };
  return {
    react: h.react,
    render(fn: () => any) {
      index = 0;
      const tree = h.render(fn);
      for (const effect of pending.splice(0)) effect();
      return tree;
    },
    dispose() { for (const effect of effects) effect?.cleanup?.(); },
  };
}
const nodes = (tree: any, type: any): any[] => !tree || typeof tree !== 'object' ? [] :
  [...(tree.type === type ? [tree] : []), ...(tree.props?.children ?? []).flat(Infinity).flatMap((child: any) => nodes(child, type))];
const one = (tree: any, type: any) => { const found = nodes(tree, type); assert.equal(found.length, 1, type); return found[0].props; };
const ids = (tree: any) => nodes(tree, 'tr').map(row => row.props.key);
function interval(request: any) {
  const q = request.query, until = q.get('until'), period = q.get('period') ?? q.get('window');
  const count = ({today: 1, '7d': 7, '30d': 30, '24h': 24} as Record<string, number>)[period] ?? 1;
  const end = Date.parse(until), width = 900000;
  const buckets = Array.from({length: count}, (_, i) => ({start: new Date(end - (count - i) * width).toISOString(), end: new Date(end - (count - i - 1) * width).toISOString(), partial: false}));
  return {period, timeZone: q.get('timeZone'), from: buckets[0].start, until, buckets};
}
function history(request: any, id: string, nextCursor = '') {
  return {...interval(request), entries: [{id, user: 'owner', accountId: 'owner', kind: 'movie', title: id, libraryId: 'library', startedAt: '2026-10-02T09:08:00Z', positionMs: 0, durationMs: 1000, completed: false}], nextCursor, total: 9};
}
function telemetry(request: any, liveOffset = 0) {
  const range = interval(request);
  if (liveOffset) {
    range.until = new Date(Date.parse(range.until) + liveOffset).toISOString();
    range.buckets[0].end = range.until;
  }
  return {...range, window: request.query.get('window'), series: Object.fromEntries(METRIC_NAMES.map(name => [name, []])), status: Object.fromEntries(METRIC_NAMES.map(name => [name, {status: 'unavailable', value: 0}])), observedAt: Date.parse(range.until)};
}

async function fixture(panel: string) {
  const outer = engine();
  let active = outer, inner: ReturnType<typeof engine> | undefined, key: any, childType: any;
  const react = new Proxy({}, {get: (_, name: string) => name === 'default' ? react : (...args: any[]) => (active.react as any)[name](...args)});
  const requests: any[] = [];
  const makeApi = () => ({request(path: string, method: string, _body?: unknown, signal?: AbortSignal) {
    const wait = deferred(), url = new URL(path, 'https://server.example');
    requests.push({path: url.pathname, query: url.searchParams, method, signal, ...wait});
    return wait.promise;
  }});
  let api = makeApi(), viewer = {viewerId: 'owner', serverId: 'server'}, zone: string | undefined = 'America/Halifax';
  const timers: any[] = [];
  let now = 0;
  const hub = new FreshnessHub({now: () => now, random: () => 0, setTimer: (fn, ms) => { const t = {fn, at: now + ms}; timers.push(t); return t; }, clearTimer: t => { const i = timers.indexOf(t); if (i >= 0) timers.splice(i, 1); }});
  let quiet: (() => Promise<unknown>) | undefined;
  const client = {
    accountOptions: async () => [],
    telemetry(window: any, context: any) {
      return ConsoleClient.prototype.telemetry.call({call: (path: string, method: string, body: unknown, parse: (raw: unknown) => unknown) => api.request(path, method, body).then(parse)} as any, window, context);
    },
  };
  const i18n = () => ({region: {timeZone: zone}, t: (id: string) => id});
  const common: any = {
    react, '../../app/session': {useSession: () => ({api})},
    '../../app/i18n': {useI18n: i18n},
    '../../app/viewer-scope': {useViewerScope: () => viewer},
    '../../app/libraries': {useLibrariesContext: () => ({items: []})},
    '../../app/errors': {present: () => ({body: 'error'})},
    '../../ui': Object.fromEntries(['Button', 'Freshness', 'Notice', 'Select', 'Surface', 'Table', 'Text', 'Segmented'].map(name => [name, name])),
  };
  const report = await componentModule(new URL('../src/screens/server/report-context.ts', import.meta.url), common);
  const consoleHooks = await componentModule(new URL('../src/admin/console.ts', import.meta.url), {...common, '../app/freshness': {useFreshness: () => ({hub})}});
  common['./report-context'] = report;
  common['../../admin/console'] = {useConsole: () => client, useRead: consoleHooks.useRead, useQuietly: (fn: () => Promise<unknown>) => { quiet = fn; }};
  const module = await componentModule(new URL('../src/screens/server/' + (panel === 'PlayHistoryPanel' ? 'PlayHistory' : 'OverviewCharts') + '.tsx', import.meta.url), common);
  const render = () => {
    active = outer;
    const child = outer.render(() => module[panel]());
    if (!inner || key !== child.props.key || childType !== child.type) {
      inner?.dispose();
      inner = engine();
      key = child.props.key; childType = child.type;
    }
    active = inner;
    return inner.render(() => child.type(child.props));
  };
  return {
    requests, render, quiet: () => quiet!(),
    changeScope(kind: string) { if (kind === 'api') api = makeApi(); if (kind === 'viewer') viewer = {...viewer, viewerId: 'other'}; if (kind === 'zone') zone = 'Asia/Tokyo'; if (kind === 'fallback') zone = undefined; },
    async advance(ms: number) {
      const end = now + ms;
      for (;;) {
        await tick();
        const due = timers.filter(t => t.at <= end).sort((a, b) => a.at - b.at)[0];
        if (!due) break;
        timers.splice(timers.indexOf(due), 1); now = due.at; due.fn();
      }
      now = end; await tick();
    },
    dispose() { inner?.dispose(); outer.dispose(); hub.dispose(); },
  };
}

test('legacy viewing defaults to today, retains the calendar context on period changes, and refreshes before reloading', async () => {
  const f = await fixture('ViewingPanel');
  try {
    let tree = f.render();
    const first = f.requests[0];
    assert.equal(first.path, '/v1/admin/play-history/summary');
    assert.equal(first.query.get('period'), 'today');
    assert.equal(first.query.get('timeZone'), 'America/Halifax');
    one(tree, 'Segmented').onChange('7d');
    tree = f.render();
    const next = f.requests[1];
    assert.equal(next.query.get('period'), '7d');
    assert.equal(next.query.get('until'), first.query.get('until'));
    atTime(Date.parse(first.query.get('until')) + 3600000, () => one(tree, 'Freshness').onRefresh());
    assert.equal(f.requests[2].query.get('until'), new Date(Date.parse(first.query.get('until')) + 3600000).toISOString());
    assert.equal(f.requests[2].query.get('period'), '7d');
    one(tree, 'Segmented').onChange('today');
    f.render();
    assert.equal(f.requests[3].query.get('until'), f.requests[2].query.get('until'), 'the refreshed context survives another period change');
  } finally { f.dispose(); }
});

test('telemetry preserves historical until through polling, uses the canonical rolling 10m contract, and disposes the old reader', async () => {
  const f = await fixture('TelemetryPanel');
  try {
    let tree = f.render();
    const first = f.requests[0];
    first.resolve(telemetry(first)); await tick(); tree = f.render();
    await f.advance(61000);
    assert.equal(f.requests[1].query.get('until'), first.query.get('until'));
    const stale = f.requests[1];
    one(tree, 'Segmented').onChange('10m'); tree = f.render();
    const live = f.requests[2];
    assert.equal(live.query.get('window'), '10m');
    assert.equal(live.query.get('until'), first.query.get('until'));
    live.resolve(telemetry(live, 1234)); await tick(); tree = f.render();
    assert.equal(one(tree, 'Freshness').refreshing, false, 'rolling server until is accepted by the real canonical parser');
    stale.resolve(telemetry(stale)); await tick(); tree = f.render();
    assert.equal(one(tree, 'Segmented').value, '10m');
    await f.advance(16000);
    assert.equal(f.requests.at(-1).query.get('window'), '10m');
  } finally { f.dispose(); }
});

test('telemetry Refresh refuses late foreground and quiet replies using the actual QuietRead generation', async () => {
  const f = await fixture('TelemetryPanel');
  const answer = (request: any, value: number) => {
    const raw = telemetry(request);
    raw.status.cpu = {status: 'available', value};
    request.resolve(raw);
  };
  try {
    let tree = f.render();
    const first = f.requests[0];
    atTime(Date.parse(first.query.get('until')) + 3600000, () => one(tree, 'Freshness').onRefresh());
    const refreshed = f.requests[1];
    assert.equal(Date.parse(refreshed.query.get('until')) - Date.parse(first.query.get('until')), 3600000);
    answer(refreshed, 22); await tick(); tree = f.render();
    answer(first, 11); await tick(); tree = f.render();
    assert.match(JSON.stringify(tree), /22%/);
    assert.doesNotMatch(JSON.stringify(tree), /11%/);
    await f.advance(61000);
    const quiet = f.requests[2];
    atTime(Date.parse(refreshed.query.get('until')) + 3600000, () => one(tree, 'Freshness').onRefresh());
    const newer = f.requests[3];
    answer(newer, 44); await tick(); tree = f.render();
    answer(quiet, 33); await tick(); tree = f.render();
    assert.match(JSON.stringify(tree), /44%/);
    assert.doesNotMatch(JSON.stringify(tree), /33%/);
  } finally { f.dispose(); }
});

test('history keeps context through filters, paging and quiet reads; refresh resets cursor and refuses late foreground/quiet replies', async () => {
  const f = await fixture('PlayHistoryPanel');
  try {
    let tree = f.render(), first = f.requests[0];
    assert.equal(first.query.get('timeZone'), 'America/Halifax');
    assert.equal(first.query.get('limit'), '100');
    first.resolve(history(first, 'first', 'cursor-one')); await tick(); tree = f.render();
    const oldMore = one(tree, 'Button').onClick;
    oldMore(); tree = f.render();
    const second = f.requests[1];
    assert.equal(second.query.get('cursor'), 'cursor-one');
    assert.equal(second.query.get('until'), first.query.get('until'));
    second.resolve(history(second, 'second', 'cursor-two')); await tick(); tree = f.render();
    assert.deepEqual(ids(tree), ['first', 'second']);
    const quiet = f.quiet(), quietRequest = f.requests[2];
    assert.equal(quietRequest.query.get('until'), first.query.get('until'));
    const currentMore = one(tree, 'Button').onClick;
    atTime(Date.parse(first.query.get('until')) + 3600000, () => one(tree, 'Freshness').onRefresh());
    currentMore();
    assert.equal(f.requests.length, 3, 'old cursor cannot start while replacement filters await rendering');
    tree = f.render();
    const refreshed = f.requests[3];
    assert.equal(refreshed.query.has('cursor'), false);
    assert.equal(Date.parse(refreshed.query.get('until')) - Date.parse(first.query.get('until')), 3600000);
    quietRequest.resolve(history(quietRequest, 'stale-quiet')); await quiet; await tick();
    refreshed.resolve(history(refreshed, 'fresh', 'new-cursor')); await tick(); tree = f.render();
    assert.deepEqual(ids(tree), ['fresh']);
    one(tree, 'Button').onClick(); tree = f.render();
    const late = f.requests[4];
    const period = nodes(tree, 'Select').find(node => node.props.label === 'server.plays.filterPeriod')!.props;
    period.onChange({target: {value: 'today'}}); tree = f.render();
    const filtered = f.requests[5];
    assert.equal(late.signal.aborted, true);
    assert.equal(filtered.query.get('period'), 'today');
    assert.equal(filtered.query.get('until'), refreshed.query.get('until'));
    assert.equal(filtered.query.has('cursor'), false);
    filtered.resolve(history(filtered, 'today')); await tick();
    late.resolve(history(late, 'stale-foreground')); await tick(); tree = f.render();
    assert.deepEqual(ids(tree), ['today']);
  } finally { f.dispose(); }
});

for (const panel of ['PlayHistoryPanel', 'TelemetryPanel', 'ViewingPanel']) {
  test(panel + ' starts a fresh scoped reader for API/viewer/zone changes and uses the runtime zone fallback', async () => {
    const f = await fixture(panel);
    try {
      f.render();
      for (const kind of ['api', 'viewer', 'zone', 'fallback']) {
        const old = f.requests.at(-1);
        f.changeScope(kind); let tree = f.render();
        const current = f.requests.at(-1);
        assert.notEqual(current, old);
        assert.equal(current.query.has('cursor'), false);
        assert.equal(current.query.get('timeZone'), kind === 'zone' ? 'Asia/Tokyo' : kind === 'fallback' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'America/Halifax');
        if (panel === 'PlayHistoryPanel') {
          current.resolve(history(current, 'current'));
          await tick(); tree = f.render();
          old.resolve(history(old, 'wrong-scope')); await tick(); tree = f.render();
          assert.deepEqual(ids(tree), ['current']);
        } else {
          old.resolve(panel === 'TelemetryPanel' ? telemetry(old) : {...interval(old), plays: 100, people: 1, watchedMs: 1, converted: 0, mostPlayed: [], mostActive: []});
          await tick(); tree = f.render();
          assert.equal(one(tree, 'Freshness').refreshing, true, 'an old scope reply cannot fill the new reader');
        }
      }
    } finally { f.dispose(); }
  });
}

