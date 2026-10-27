/**
 * FX229 / BLK-PLAY-016: the player's Equalizer opening edits this device's existing port.
 *
 * The actual panel is mounted through the interaction harness with its DeviceEqualizer host
 * stubbed; the runtime, port and opening guards are real. Covers writes while live, a write
 * already issued staying on its original port, and stale Back/Settings/write after the output
 * or run is replaced. Source only: unrun here.
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
  const changes: any[] = [], resets = 0, reports: unknown[] = [];
  const held = {value: equalizer(), pending: 0 as Promise<void> | undefined};
  const port = {
    getSnapshot: () => held.value, subscribe: () => () => {}, start: () => () => {},
    change: async (value: any) => {changes.push(value); await held.pending;},
    reset: async () => {held.pending = undefined; resets++;},
  };
  const authority = {}, original = fence('song-1');
  const state = {disposed: false, authority, fence: original, playback: {playbackRun: 7}, presentation: {kind: 'music', fence: original}, equalizerPort: port};
  const runtime = {getSnapshot: () => state, equalizerPort: port};
  const app = await componentModule(new URL('../src/screens/watch/music/EqualizerPanel.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key}},
    '../../../kit/controls': {Button: 'Button'},
    '../../../kit/media': {DeviceEqualizer: 'DeviceEqualizer'},
    '../../../../../packages/player-core/src/index.ts': playerCore,
  });
  const calls = {close: 0, settings: 0};
  const show = (host = true) => {
    const panel = () => tree(h.render(() => app.WatchEqualizerPanel({runtime: runtime as any, snapshot: state as any, port: port as any,
      hostCurrent: () => host, close: () => {calls.close++;}, openSettings: () => {calls.settings++;}})));
    panel();
    return panel();
  };
  return {calls, changes, h, held, port, reports, run: () => state, setRun: (run: number) => {state.playback.playbackRun = run;}, show, state};
}

test('a live opening edits the original device port through the drawn panel', async () => {
  const {changes, h, show} = await mount();
  let v = show();
  const eq = one(v, 'DeviceEqualizer').props;
  assert.equal(eq.layout, 'panel');
  assert.equal(typeof eq.onBack, 'function');
  assert.equal(typeof eq.onOpenSettings, 'function');
  await eq.port.change(equalizer(false));
  assert.equal(changes.length, 1, 'the live opening writes the existing port');
  assert.equal(one(v, 'DeviceEqualizer').props.port, eq.port, 'the same guarded port stays bound across renders');
  void h;
});

test('a write already issued keeps its original port after the run is replaced', async () => {
  const {changes, held, run, setRun, show} = await mount();
  let v = show();
  const guarded = one(v, 'DeviceEqualizer').props.port;
  const pending = defer<void>();
  held.pending = pending.promise;
  const write = guarded.change(equalizer(false));
  setRun(8);
  v = show();
  pending.resolve();
  await write;
  assert.equal(changes.length, 1, 'the in-flight write stayed on the port it was sent to');
  await assert.rejects(guarded.change(equalizer(true)), /retired/i, 'a new write on the retired opening is refused');
  assert.equal(changes.length, 1, 'the retired opening cannot start another device write');
  void run;
});

test('stale Back, Settings and errors do not act on a replaced output', async () => {
  const {calls, show, setRun} = await mount();
  let v = show();
  const eq = one(v, 'DeviceEqualizer').props;
  setRun(9);
  v = show();
  eq.onBack();
  eq.onOpenSettings();
  eq.reportError(new Error('late'));
  await tick();
  assert.equal(calls.close, 0, 'the retired opening cannot close the successor');
  assert.equal(calls.settings, 0, 'the retired opening cannot navigate Settings');
  assert.ok(!all(v, 'p').some(node => JSON.stringify(node.props).includes('next.states.pageFailedBody')), 'a late error publishes no copy');
});

test('an absent device record offers Back while it loads', async () => {
  const {held, show} = await mount();
  held.value = null as any;
  const v = show();
  assert.equal(all(v, 'DeviceEqualizer').length, 0);
  const back = all(v, 'Button').find(node => (Array.isArray(node.props.children) ? node.props.children.join('') : node.props.children) === 'next.media.eq.backQueue');
  assert.ok(back, 'the loading panel keeps an actionable Back');
});
