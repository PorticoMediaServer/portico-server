import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {componentModule} from './helpers/component-harness.mjs';
import {EventsClient} from '@core/playback-v1/events.ts';
import * as inventory from '@core/library-inventory.ts';

/** MU2: scan state comes from the event feed, not polling. A tiny driver with
 * real effects (the shared harness ignores them), mirroring the Apple
 * hook-driver contract: one render, then effects, then manual re-renders. */
function driver() {
  const slots: any[] = [];
  let cursor = 0;
  const react: any = {
    createContext: (value: unknown) => ({value, Provider: 'provider'}),
    createElement: (type: unknown, props: any, ...children: unknown[]) => ({type, props: {...props, children}}),
    useRef: (initial: unknown) => {
      const n = cursor++;
      if (!(n in slots)) slots[n] = {current: initial};
      return slots[n];
    },
    useState: (initial: any) => {
      const n = cursor++;
      if (!(n in slots)) slots[n] = typeof initial === 'function' ? initial() : initial;
      const set = (v: any) => {
        slots[n] = typeof v === 'function' ? v(slots[n]) : v;
      };
      return [slots[n], set];
    },
    useMemo: (fn: () => any, deps: readonly unknown[]) => {
      const n = cursor++;
      const prev = slots[n];
      const same = prev && deps && prev.deps && deps.length === prev.deps.length && deps.every((v, i) => Object.is(v, prev.deps[i]));
      if (!prev || !same) slots[n] = {deps, value: fn()};
      return slots[n].value;
    },
    useCallback: (fn: any, deps: readonly unknown[]) => react.useMemo(() => fn, deps),
    useEffect: (create: () => unknown, deps: readonly unknown[] | undefined) => {
      const n = cursor++;
      const prev = slots[n];
      const same = prev && deps && prev.deps && deps.length === prev.deps.length && deps.every((v, i) => Object.is(v, prev.deps[i]));
      if (!prev || !same) slots[n] = {deps, create, destroy: prev?.destroy, pending: true};
    },
    useSyncExternalStore: (subscribe: (cb: () => void) => () => void, get: () => any) => {
      const n = cursor++;
      let rec = slots[n];
      if (!rec) {
        rec = {unsub: null, last: null};
        slots[n] = rec;
      }
      if (rec.last !== subscribe) {
        try {
          rec.unsub?.();
        } catch {}
        rec.unsub = subscribe(() => {});
        rec.last = subscribe;
      }
      return get();
    },
    useContext: (context: any) => context.value,
  };
  react.default = react;
  const runEffects = () => {
    for (const slot of slots) {
      if (slot && slot.pending) {
        slot.pending = false;
        const create = slot.create;
        slot.create = undefined;
        try {
          slot.destroy?.();
        } catch {}
        slot.destroy = undefined;
        const out = create();
        if (typeof out === 'function') slot.destroy = out;
      }
    }
  };
  return {
    react,
    render: (fn: () => any) => {
      cursor = 0;
      const out = fn();
      runEffects();
      return out;
    },
    unmount: () => {
      for (const slot of slots) {
        try {
          slot?.destroy?.();
        } catch {}
        try {
          slot?.unsub?.();
        } catch {}
      }
    },
  };
}

const flush = async () => {
  for (let i = 0; i < 100; i++) {
    await Promise.resolve();
    await new Promise<void>(done => setImmediate(done));
  }
};

const source = (status: string, discovered: number) => ({id: `s-${status}-${discovered}`, health: 'healthy', lastCompleteAt: '', jobId: 'job', status, phase: 'inventory', discovered, analyzed: 0, warnings: 0, pauseReason: ''});
const scanOf = (s: {sources: {status: string; discovered: number}[]} | null | undefined) => {
  const active = (s?.sources ?? []).filter(x => x.status === 'queued' || x.status === 'running');
  return active.length ? {scanning: true, found: active.reduce((n, x) => n + x.discovered, 0)} : {scanning: false, found: 0};
};

async function load(feed: EventsClient | null, byLibrary: Record<string, unknown[]>, requests: string[][]) {
  const d = driver();
  const api = {request: async () => ({})} as never;
  const mod = (await componentModule(new URL('../src/app/library-scan.ts', import.meta.url), {
    react: d.react,
    '@core/library-management.ts': {
      fetchInventoryStatus: async (_reader: unknown, _server: string, libraryId: string) => {
        requests.push([libraryId]);
        return {sources: byLibrary[libraryId] ?? []};
      },
      scanProgress: scanOf,
    },
    '@core/library-inventory.ts': {LibraryScanTracker: inventory.LibraryScanTracker, parseLibraryScanEvent: inventory.parseLibraryScanEvent},
    '@core/presentation/index.ts': {compatContentApi: (a: unknown) => a},
    '@core/index.ts': {},
    '@core/playback-v1/events.ts': {},
    './device-events': {useDeviceEvents: () => feed},
  })) as any;
  return {d, mod, api};
}

const scanEvent = (id: string, libraryId: string, revision: string, state: string, found: number) => ({id, type: 'library.scan.updated', resource: {kind: 'library', id: libraryId}, revision, data: {state, found}});

/** MU2: the hooks read once, follow the feed, re-read on resync, and never poll. */
test('MU2: initial read once, events update, resync re-reads, no timers', async () => {
  const feed = new EventsClient({http: {request: async () => { throw new Error('no network'); }} as never});
  const requests: string[][] = [];
  const byLibrary: Record<string, unknown[]> = {lib1: [source('running', 100)], lib2: [source('complete', 5)]};
  const {d, mod, api} = await load(feed, byLibrary, requests);
  let single: any = 'unset';
  let side: any = 'unset';
  const render = () =>
    d.render(() => {
      single = mod.useLibraryScan(api, 'server', 'lib1');
      side = mod.useSidebarScans(api, 'server', ['lib1', 'lib2']);
    });
  render();
  await flush();
  render();
  // Bounded initial reads: one per visible library for coming on screen, plus
  // one wiring resync each — never a burst, even though both hooks watch lib1.
  const count = (id: string) => requests.filter(r => r[0] === id).length;
  assert.ok(count('lib1') >= 1 && count('lib1') <= 2, `lib1 read ${count('lib1')}x`);
  assert.ok(count('lib2') >= 1 && count('lib2') <= 2, `lib2 read ${count('lib2')}x`);
  assert.ok(requests.length <= 4, `${requests.length} initial requests`);
  const settled = requests.length;
  assert.deepEqual(single, {scanning: true, found: 100, finished: 0});
  assert.equal(side.get('lib1'), 100);
  assert.equal(side.has('lib2'), false);
  // A progress event updates both hooks with no new request.
  feed.deliver(scanEvent('e1', 'lib1', '20', 'progress', 999) as never);
  render();
  assert.deepEqual(single, {scanning: true, found: 999, finished: 0});
  assert.equal(side.get('lib1'), 999);
  assert.equal(requests.length, settled, 'no request per event');
  // A finished event clears the badge and counts once.
  feed.deliver(scanEvent('e2', 'lib1', '21', 'finished', 999) as never);
  render();
  assert.deepEqual(single, {scanning: false, found: 0, finished: 1});
  assert.equal(side.has('lib1'), false);
  assert.equal(requests.length, settled, 'no request per event');
  // stream.resync re-reads every watched library once.
  feed.deliver({id: 'r1', type: 'stream.resync'} as never);
  await flush();
  render();
  assert.equal(requests.length, settled + 2, 'one re-read per watched library');
  assert.deepEqual(requests.slice(settled).map(r => r[0]).sort(), ['lib1', 'lib2']);
  assert.deepEqual(single, {scanning: true, found: 100, finished: 1});
  // No timers left: nothing further happens on its own.
  const src = await readFile(new URL('../src/app/library-scan.ts', import.meta.url), 'utf8');
  assert.ok(!src.includes('setInterval'), 'no setInterval');
  assert.ok(!src.includes('setTimeout'), 'no setTimeout');
  assert.ok(!src.includes('new EventsClient'), 'no second event client');
  await flush();
  render();
  assert.equal(requests.length, settled + 2, 'no polling after 60s of fake time');
  d.unmount();
});

/** MU2: with no feed yet the initial read still shows the state. */
test('MU2: the initial read shows the state before the feed exists', async () => {
  const requests: string[][] = [];
  const {d, mod, api} = await load(null, {lib1: [source('queued', 7)]}, requests);
  let single: any = 'unset';
  const render = () => d.render(() => {
    single = mod.useLibraryScan(api, 'server', 'lib1');
  });
  render();
  await flush();
  render();
  assert.deepEqual(requests, [['lib1']]);
  assert.deepEqual(single, {scanning: true, found: 7, finished: 0});
  d.unmount();
});

/** M29 quiet start is kept: missing scope reads quiet, never a crash. */
test('M29: the scan hooks start quiet and tolerate missing scope', async () => {
  const feed = new EventsClient({http: {request: async () => { throw new Error('no network'); }} as never});
  const requests: string[][] = [];
  const {d, mod, api} = await load(feed, {}, requests);
  let out: any;
  d.render(() => {
    out = mod.useLibraryScan(api, undefined, 'library');
  });
  assert.deepEqual(out, {scanning: false, found: 0, finished: 0});
  d.render(() => {
    out = mod.useLibraryScan(api, 'server', undefined);
  });
  assert.deepEqual(out, {scanning: false, found: 0, finished: 0});
  d.render(() => {
    out = mod.useSidebarScans(api, 'server', []);
  });
  assert.equal((out as ReadonlyMap<string, number>).size, 0);
  d.unmount();
  assert.deepEqual(requests, []);
});
