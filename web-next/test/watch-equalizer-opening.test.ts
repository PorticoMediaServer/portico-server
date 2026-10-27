/**
 * FX229 / BLK-PLAY-016: the player's Equalizer opening edits this device's existing port.
 *
 * The actual panel is mounted through the interaction harness with its DeviceEqualizer host
 * stubbed; the runtime, port and opening guards are real. Covers writes while live, an issued
 * write staying on its original port, saved callbacks after a same-port/run replacement, an
 * ordinary position rerender keeping the admission, host-predicate reentry, a fresh admission
 * after the host is replayed, and stale Back/Settings/report refusals.
 *
 * The harness runs hooks eagerly and has no React StrictMode or unmount replay; the Strict and
 * replay cases are approximated by flipping the host off and on, which is the same layout cleanup
 * and mint path. Source only: unrun here.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import * as playerCore from '../../packages/player-core/src/index.ts';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tick, tree} from '../../web/test/helpers/interaction-harness.mjs';

const equalizer = (enabled = true) => ({enabled, preampDb: 0, bandsDb: Array.from({length: 10}, () => 0)});
const fence = (itemId: string) => ({kind: 'queue', itemId, queueId: 'q1', entryId: itemId + '-e', playback: 4});
const defer = <T,>() => { let resolve!: (value: T) => void, reject!: (error?: unknown) => void; const promise = new Promise<T>((yes, no) => {resolve = yes; reject = no;}); return {promise, resolve, reject}; };

async function mount() {
  const h = createRenderer();
  const changes: any[] = [], reports: unknown[] = [];
  let resets = 0;
  const held: {value: any; pending: Promise<void> | undefined} = {value: equalizer(), pending: undefined};
  const port = {
    getSnapshot: () => held.value, subscribe: () => () => {}, start: () => () => {},
    change: async (value: any) => {changes.push(value); await held.pending;},
    reset: async () => {resets++;},
  };
  const authority = {}, original = fence('song-1');
  const state: any = {disposed: false, authority, fence: original, playback: {playbackRun: 7}, presentation: {kind: 'music', fence: original}, equalizerPort: port, position: 0};
  let reads: any[] = [];
  const runtime = {getSnapshot: () => reads.shift() ?? state, equalizerPort: port};
  const app = await componentModule(new URL('../src/screens/watch/music/EqualizerPanel.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key}},
    '../../../kit/controls': {Button: 'Button'},
    '../../../kit/media': {DeviceEqualizer: 'DeviceEqualizer'},
    '../../../../../packages/player-core/src/index.ts': playerCore,
  });
  const calls = {close: 0, settings: 0};
  const show = (options: {snapshot?: any; active?: boolean; scope?: {current(): boolean}} = {}) => {
    const current = options.snapshot ?? state;
    const panel = () => tree(h.render(() => app.WatchEqualizerPanel({runtime: runtime as any, snapshot: current, port: port as any, active: options.active ?? true,
      ...(options.scope ? {scope: options.scope} : {}), close: () => {calls.close++;}, openSettings: () => {calls.settings++;}})));
    panel();
    return panel();
  };
  return {app, calls, changes, equalizerHostCurrent: app.equalizerHostCurrent, h, held, port, reports, resets: () => resets, run: () => state, runtime, setRun: (run: number) => {state.playback.playbackRun = run;}, show, state};
}

test('a live opening edits the original device port through the drawn panel', async () => {
  const {changes, show} = await mount();
  const v = show();
  const eq = one(v, 'DeviceEqualizer').props;
  assert.equal(eq.layout, 'panel');
  await eq.port.change(equalizer(false));
  assert.equal(changes.length, 1, 'the live opening writes the existing port');
  assert.equal(one(show(), 'DeviceEqualizer').props.port, eq.port, 'ordinary rerenders keep the same guarded port');
});

test('an ordinary position rerender does not mint a new admission', async () => {
  const {show, state} = await mount();
  const before = one(show(), 'DeviceEqualizer').props.port;
  const position = {...state, position: 31, presentation: {...state.presentation}};
  const after = one(show({snapshot: position}), 'DeviceEqualizer').props.port;
  assert.equal(after, before, 'the same fence/run/scope keeps one published admission');
});

test('a write already issued keeps its original port after the run is replaced', async () => {
  const {changes, held, setRun, show} = await mount();
  const guarded = one(show(), 'DeviceEqualizer').props.port;
  const pending = defer<void>();
  held.pending = pending.promise;
  const write = guarded.change(equalizer(false));
  setRun(8);
  show();
  pending.resolve();
  await write;
  assert.equal(changes.length, 1, 'the in-flight write stayed on the port it was sent to');
  await assert.rejects(guarded.change(equalizer(true)), /retired/i, 'a new write on the retired opening is refused');
  assert.equal(changes.length, 1);
});

test('saved Back, Settings, report and writes from a same-port run replacement are refused', async () => {
  const {calls, show, setRun} = await mount();
  const v = show();
  const eq = one(v, 'DeviceEqualizer').props;
  setRun(9);
  show();
  eq.onBack();
  eq.onOpenSettings();
  eq.reportError(new Error('late'));
  await assert.rejects(eq.port.change(equalizer(true)), /retired/i);
  await tick();
  assert.equal(calls.close, 0, 'the retired opening cannot close the successor');
  assert.equal(calls.settings, 0, 'the retired opening cannot navigate Settings');
  const later = show();
  assert.equal(all(later, 'p').filter(node => JSON.stringify(node.props).includes('next.states.pageFailedBody')).length, 0, 'a late error publishes no copy');
});

test('the host predicate is rechecked before a write', async () => {
  const {changes, runtime, show, state} = await mount();
  const v = show();
  const guarded = one(v, 'DeviceEqualizer').props.port;
  const broken = {...state, playback: {playbackRun: 10}};
  const reads = [state, broken];
  (runtime as any).getSnapshot = () => reads.shift() ?? broken;
  await assert.rejects(guarded.change(equalizer(false)), /retired/i, 'a host that changed during the predicate stops the write');
  assert.equal(changes.length, 0);
});

test('replaying the host mints a fresh admission and refuses the old one', async () => {
  const {show} = await mount();
  const first = one(show(), 'DeviceEqualizer').props;
  show({active: false});
  const successor = one(show(), 'DeviceEqualizer').props;
  assert.notEqual(successor.port, first.port, 'a fresh opening gets its own guarded port');
  first.onBack();
  await assert.rejects(first.port.change(equalizer(false)), /retired/i, 'the old opening never adopts the successor');
  await successor.port.change(equalizer(false));
});

test('an absent device record offers Back while it loads', async () => {
  const {held, show} = await mount();
  held.value = null;
  const v = show();
  assert.equal(all(v, 'DeviceEqualizer').length, 0);
  const back = all(v, 'Button').find(node => (Array.isArray(node.props.children) ? node.props.children.join('') : node.props.children) === 'next.media.eq.backQueue');
  assert.ok(back, 'the loading panel keeps an actionable Back');
});

test('the saved More host check is tied to its rendered source', async () => {
  const {equalizerHostCurrent, run, runtime, setRun, state} = await mount();
  const old = {...state};
  assert.equal(equalizerHostCurrent(runtime, state.equalizerPort, old, true), true);
  setRun(11);
  assert.equal(equalizerHostCurrent(runtime, state.equalizerPort, old, true), false, 'a saved More callback refuses a successor run');
  let allow = true;
  const scope = {current: () => allow};
  assert.equal(equalizerHostCurrent(runtime, state.equalizerPort, run(), true, scope), true);
  allow = false;
  assert.equal(equalizerHostCurrent(runtime, state.equalizerPort, run(), true, scope), false, 'the original host predicate still gates the saved callback');
});
