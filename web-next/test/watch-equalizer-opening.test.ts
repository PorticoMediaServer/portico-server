/**
 * FX229 / BLK-PLAY-016: the player's Equalizer opening and the Now playing More menu are each
 * bound to one published host opening.
 *
 * Real playback fences and the actual panel/admission helpers are used; DeviceEqualizer and Menu
 * are stub hosts. The harness runs hooks eagerly and has no React StrictMode or unmount replay;
 * host replay is approximated by collapsing and reopening the expanded host, which is the same
 * layout cleanup and mint path. Source only: unrun here.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {fenceKey} from '../src/shell/playback-model';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tick, tree} from '../../web/test/helpers/interaction-harness.mjs';

const scopeToken = {}, authority = {};
const fence = (itemId = 'song-1') => ({kind: 'queue' as const, playback: {scope: scopeToken, run: 7}, sessionId: 'session', itemId, sourceId: 'source', entryId: 'entry', generation: 1});
const equalizer = (enabled = true) => ({enabled, preampDb: 0, bandsDb: Array.from({length: 10}, () => 0)});

async function panelModule() {
  const h = createRenderer();
  const media = {DeviceEqualizer: 'DeviceEqualizer', ListeningNotice: 'ListeningNotice', SignalLine: 'SignalLine', SignalPath: 'SignalPath'};
  const panel = await componentModule(new URL('../src/screens/watch/music/EqualizerPanel.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key}},
    '../../../kit/controls': {Button: 'Button'},
    '../../../kit/media': media,
    '../../../shell/playback-model': {fenceKey},
  });
  return {h, media, panel};
}

async function mountPanel() {
  const {h, media, panel} = await panelModule();
  const changes: any[] = [];
  const held: {value: any; pending: Promise<void> | undefined} = {value: equalizer(), pending: undefined};
  const port = {getSnapshot: () => held.value, subscribe: () => () => {}, start: () => () => {},
    change: async (value: any) => {changes.push(value); await held.pending;}, reset: async () => {changes.push('reset');}};
  const originalFence = fence();
  const state: any = {disposed: false, authority, fence: originalFence, playback: {playbackRun: 7}, presentation: {kind: 'music', fence: originalFence}, equalizerPort: port};
  const runtime = {getSnapshot: () => state, equalizerPort: port};
  let host = true, scopeAllowed = true;
  const isCurrent = () => host;
  const scope = {current: () => scopeAllowed};
  const published: {current: any} = {current: null};
  const calls = {close: 0, settings: 0};
  const show = (value: any = state, reuse?: any) => {
    const computed = reuse ?? {runtime, port, authority: value.authority, fenceAt: fenceKey(value.fence), presentationAt: fenceKey(value.presentation.fence),
      run: value.playback.playbackRun, isCurrent, scope, live: true};
    published.current = computed;
    const paint = () => tree(h.render(() => panel.WatchEqualizerPanel({opening: computed, rendered: computed, published, runtime: runtime as any,
      port: port as any, isCurrent, scope, close: () => {calls.close++;}, openSettings: () => {calls.settings++;}})));
    paint();
    return {computed, tree: paint()};
  };
  return {calls, changes, held, isCurrent, published, scope, setHost: (value: boolean) => {host = value;}, setScope: (value: boolean) => {scopeAllowed = value;}, show, state};
}

test('a live opening writes the original port; an ordinary rerender keeps the receipt', async () => {
  const {changes, show} = await mountPanel();
  const first = show();
  const eq = one(first.tree, 'DeviceEqualizer').props;
  await eq.port.change(equalizer(false));
  assert.equal(changes.length, 1);
  const again = show(undefined as any, first.computed);
  assert.equal(one(again.tree, 'DeviceEqualizer').props.port, eq.port);
});

test('a same-key replacement withholds the old panel and its callbacks', async () => {
  const {calls, changes, published, show, state} = await mountPanel();
  const first = show();
  const oldEq = one(first.tree, 'DeviceEqualizer').props;
  const replacement = {...first.computed, authority: {}, live: true};
  const second = show(state, replacement);
  assert.notEqual(second.computed, first.computed);
  assert.equal(published.current, replacement);
  await assert.rejects(oldEq.port.change(equalizer(true)), /retired/i);
  oldEq.onBack();
  oldEq.onOpenSettings();
  oldEq.reportError(new Error('late'));
  await tick();
  assert.equal(changes.length, 0);
  assert.equal(calls.close, 0);
  assert.equal(calls.settings, 0);
});

test('the original host predicate is rechecked before a write', async () => {
  const {changes, scope, show} = await mountPanel();
  const v = show();
  const eq = one(v.tree, 'DeviceEqualizer').props;
  let checks = 0;
  (scope as any).current = () => ++checks === 1;
  await assert.rejects(eq.port.change(equalizer(false)), /retired/i);
  assert.equal(changes.length, 0, 'a host that flipped during the predicate stops the write');
});

test('an absent device record offers Back while it loads', async () => {
  const {held, show} = await mountPanel();
  held.value = null;
  const v = show();
  assert.equal(all(v.tree, 'DeviceEqualizer').length, 0);
  const back = all(v.tree, 'Button').find(node => (Array.isArray(node.props.children) ? node.props.children.join('') : node.props.children) === 'next.media.eq.backQueue');
  assert.ok(back);
});

async function nowPlaying() {
  const {h, media, panel} = await panelModule();
  const held: {value: any; pending: Promise<void> | undefined} = {value: equalizer(), pending: undefined};
  const port = {getSnapshot: () => held.value, subscribe: () => () => {}, start: () => () => {}, change: async () => {}, reset: async () => {}};
  const queueFence = fence();
  const snapshot: any = {disposed: false, authority, fence: queueFence, playback: {playbackRun: 7, intent: 'playing', phase: 'playing'},
    presentation: {kind: 'music', fence: queueFence, positionSeconds: 0, durationSeconds: 180, playing: true, controls: ['play', 'pause', 'seek', 'next', 'previous'], subject: {}, posterUrl: ''},
    dock: {local: {kind: 'music', expanded: true, fence: queueFence}, remote: null}, queue: {}, sequence: 1};
  const runtime = {getSnapshot: () => snapshot, equalizerPort: port};
  const app = await componentModule(new URL('../src/screens/watch/music/NowPlaying.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key}},
    '@core/presentation/kit-model/index.ts': {playbackIdentity: () => ({title: 'Song', subtitle: [{text: 'Artist'}]})},
    '../../../kit/artwork': {Artwork: 'Artwork'},
    '../../../kit/Icon': {Icon: 'Icon'},
    '../../../kit/controls': {Slider: 'Slider'},
    '../../../kit/overlays': {Menu: 'Menu'},
    '../../../kit/media': media,
    '../model': {dispatchWatch: async () => {}, watchClock: () => '0:00', watchSeek: () => null, watchSeekBounds: () => null},
    '../queue-neighbours': {watchQueueNeighbours: () => ({})},
    './signal': {watchMusicSignal: () => undefined},
    './audio-facts': {readWatchMusicAudioFacts: () => ({})},
    '../../../shell/playback-model': {fenceKey},
    '../../../router': {router: {navigate: async () => {}}},
    './EqualizerPanel': {WatchEqualizerPanel: 'WatchEqualizerPanel', equalizerAdmit: panel.equalizerAdmit},
  });
  const binding = {runtime: runtime as any, artwork: {foreground: () => undefined}, environment: {locale: 'en'}, skips: {back: 10, forward: 30}, reportError: () => {}};
  let expanded = true;
  const show = (value: any = snapshot) => {
    const paint = () => tree(h.render(() => app.WatchMusicNowPlaying({binding: binding as any, snapshot: value as any, expanded, onCollapse: () => {}, phone: false})));
    paint();
    return paint();
  };
  return {app, show, setExpanded: (value: boolean) => {expanded = value;}, snapshot};
}

const equalizerVisible = (v: any[]) => v.some(node => node.type === 'WatchEqualizerPanel');

test('the actual saved More menu refuses a close/reopen of the same playback', async () => {
  const {setExpanded, show} = await nowPlaying();
  let v = show();
  const menu = one(v, 'Menu').props;
  const savedSelect = menu.items[0].onSelect, savedChange = menu.onOpenChange;
  savedSelect();
  v = show();
  assert.ok(equalizerVisible(v), 'the menu opens the side panel');
  setExpanded(false);
  show();
  setExpanded(true);
  v = show();
  assert.ok(!equalizerVisible(v), 'the reopened host starts on Up next');
  savedSelect();
  savedChange(false);
  savedChange(true);
  v = show();
  assert.ok(!equalizerVisible(v), 'the old menu cannot revive the Equalizer');
  assert.equal(one(v, 'Menu').props.open, false, 'an old dismissal cannot clear the successor menu');
});

test('an ordinary position update keeps the actual menu callback live', async () => {
  const {show, snapshot} = await nowPlaying();
  const v = show();
  one(v, 'Menu').props.items[0].onSelect();
  const position = {...snapshot, presentation: {...snapshot.presentation, positionSeconds: 42}};
  const after = show(position);
  assert.ok(equalizerVisible(after), 'the same opening keeps the panel across an ordinary position render');
});
