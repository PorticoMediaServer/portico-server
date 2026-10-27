/**
 * FX229 / BLK-PLAY-016: the host, More menu and Equalizer openings each have their own published
 * receipt, minted fresh in layout from immutable rendered facts.
 *
 * Panel cases use a manual receipt factory with the actual panel/admission helpers; the parent
 * cases mount the actual NowPlaying with a stub More/EQ host. The harness runs hooks eagerly and
 * has no React StrictMode or unmount replay; cleanup→setup is modelled as a fresh receipt pair
 * with the same immutable facts, not as relaunching the player. Source only: unrun here.
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
    react: h.react, '@i18n': {defaultI18n: {t: (key: string) => key}}, '../../../kit/controls': {Button: 'Button'},
    '../../../kit/media': media, '../../../shell/playback-model': {fenceKey},
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
  let getter: (() => any) | undefined;
  const runtime = {getSnapshot: () => getter ? getter() : state, equalizerPort: port};
  let host = true, scopeAllowed = true;
  const isCurrent = () => host, scope = {current: () => scopeAllowed};
  const calls = {close: 0, settings: 0};
  const facts = (value: any = state) => ({runtime: runtime as any, port: port as any, authority: value.authority,
    fenceAt: fenceKey(value.fence), presentationAt: fenceKey(value.presentation.fence), run: value.playback.playbackRun, isCurrent, scope});
  const publishedHost: {current: any} = {current: null}, publishedSurface: {current: any} = {current: null};
  const setup = (value: any = state, reuse?: {host?: any; surface?: any}) => {
    const hostFacts = reuse?.host?.facts ?? facts(value);
    const hostReceipt = reuse?.host ?? {facts: hostFacts, live: true};
    const surface = reuse?.surface ?? {host: hostReceipt, live: true};
    if (!reuse) {publishedHost.current = hostReceipt; publishedSurface.current = surface;}
    const paint = () => tree(h.render(() => panel.WatchEqualizerPanel({receipt: surface, rendered: surface, published: publishedSurface,
      host: hostFacts, hostReceipt, publishedHost, runtime: runtime as any, port: port as any, isCurrent, scope,
      close: () => {calls.close++;}, openSettings: () => {calls.settings++;}})));
    paint();
    return {hostFacts, hostReceipt, surface, tree: paint()};
  };
  return {calls, changes, facts, getter: (fn: () => any) => {getter = fn;}, held, publishedHost, publishedSurface, scope, setHost: (value: boolean) => {host = value;}, setScope: (value: boolean) => {scopeAllowed = value;}, setup, state};
}

test('a live EQ opening writes; an ordinary rerender keeps the same receipts', async () => {
  const {changes, setup} = await mountPanel();
  const first = setup();
  const eq = one(first.tree, 'DeviceEqualizer').props;
  await eq.port.change(equalizer(false));
  assert.equal(changes.length, 1);
  const again = setup(undefined as any, {host: first.hostReceipt, surface: first.surface});
  assert.equal(one(again.tree, 'DeviceEqualizer').props.port, eq.port);
});

test('a same-facts cleanup/setup replay makes the old EQ actions dead and the new live', async () => {
  const {calls, changes, setup} = await mountPanel();
  const first = setup();
  const oldEq = one(first.tree, 'DeviceEqualizer').props;
  const second = setup(undefined as any, undefined);
  assert.notEqual(second.surface, first.surface);
  oldEq.onBack();
  oldEq.onOpenSettings();
  oldEq.reportError(new Error('late'));
  await assert.rejects(oldEq.port.change(equalizer(false)), /retired/i);
  assert.equal(calls.close, 0);
  assert.equal(calls.settings, 0);
  await one(second.tree, 'DeviceEqualizer').props.port.change(equalizer(false));
  assert.equal(changes.length, 1, 'only the successor receipt wrote');
});

test('a getSnapshot that swaps the published receipt mid-check cannot slip a write through', async () => {
  const {changes, publishedHost, setup, state} = await mountPanel();
  const first = setup();
  const eq = one(first.tree, 'DeviceEqualizer').props;
  const successor = {facts: first.hostFacts, live: true};
  let calls = 0;
  // The last external getSnapshot retires A and publishes B while still returning A's facts.
  const getter = () => {if (++calls === 2) {first.hostReceipt.live = false; publishedHost.current = successor;} return state;};
  const original = first.hostFacts.runtime.getSnapshot;
  (first.hostFacts.runtime as any).getSnapshot = getter;
  await assert.rejects(eq.port.change(equalizer(false)), /retired/i);
  (first.hostFacts.runtime as any).getSnapshot = original;
  assert.equal(changes.length, 0);
});

test('the original predicates are rechecked before a write', async () => {
  const {changes, scope, setup} = await mountPanel();
  const v = setup();
  const eq = one(v.tree, 'DeviceEqualizer').props;
  let checks = 0;
  (scope as any).current = () => ++checks === 1;
  await assert.rejects(eq.port.change(equalizer(false)), /retired/i);
  assert.equal(changes.length, 0);
});

test('an absent device record offers Back while it loads', async () => {
  const {held, setup} = await mountPanel();
  held.value = null;
  const v = setup();
  assert.equal(all(v.tree, 'DeviceEqualizer').length, 0);
  assert.ok(all(v.tree, 'Button').some(node => (Array.isArray(node.props.children) ? node.props.children.join('') : node.props.children) === 'next.media.eq.backQueue'));
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
    react: h.react, '@i18n': {defaultI18n: {t: (key: string) => key}},
    '@core/presentation/kit-model/index.ts': {playbackIdentity: () => ({title: 'Song', subtitle: [{text: 'Artist'}]})},
    '../../../kit/artwork': {Artwork: 'Artwork'}, '../../../kit/Icon': {Icon: 'Icon'}, '../../../kit/controls': {Slider: 'Slider'},
    '../../../kit/overlays': {Menu: 'Menu'}, '../../../kit/media': media,
    '../model': {dispatchWatch: async () => {}, watchClock: () => '0:00', watchSeek: () => null, watchSeekBounds: () => null},
    '../queue-neighbours': {watchQueueNeighbours: () => ({})}, './signal': {watchMusicSignal: () => undefined},
    './audio-facts': {readWatchMusicAudioFacts: () => ({})}, '../../../shell/playback-model': {fenceKey},
    '../../../router': {router: {navigate: async () => {}}},
    './EqualizerPanel': {WatchEqualizerPanel: 'WatchEqualizerPanel', hostAdmit: panel.hostAdmit, surfaceAdmit: panel.surfaceAdmit},
  });
  const binding = {runtime: runtime as any, artwork: {foreground: () => undefined}, environment: {locale: 'en'}, skips: {back: 10, forward: 30}, reportError: () => {}};
  let expanded = true;
  const show = (value: any = snapshot) => {
    const paint = () => tree(h.render(() => app.WatchMusicNowPlaying({binding: binding as any, snapshot: value as any, expanded, onCollapse: () => {}, phone: false})));
    paint();
    return paint();
  };
  return {show, snapshot};
}

const panelHost = (v: any[]) => all(v, 'WatchEqualizerPanel')[0];
const moreButton = (v: any[]) => all(v, 'button').find(node => node.props['aria-label'] === 'player.more');
const menu = (v: any[]) => one(v, 'Menu').props;
const openMenu = (show: any) => {let v = show(); moreButton(v)!.props.onClick(); return show();};
const selectEq = (show: any) => {let v = openMenu(show); menu(v).items[0].onSelect(); return show();};

test('More A dismissed then B opened on the same host: saved A cannot close B', async () => {
  const {show} = await nowPlaying();
  const a = openMenu(show);
  const menuA = menu(a);
  menuA.onOpenChange(false);
  const b = openMenu(show);
  assert.equal(menu(b).open, true, 'B is open on the same expanded host');
  menuA.onOpenChange(false);
  assert.equal(menu(show()).open, true, 'saved A dismissal cannot close B');
  menuA.items[0].onSelect();
  assert.equal(panelHost(show()), undefined, 'saved A selection cannot open EQ');
});

test('EQ A Back then EQ B on the same host: saved A cannot close B', async () => {
  const {show} = await nowPlaying();
  const a = selectEq(show);
  const panelA = panelHost(a);
  assert.ok(panelA);
  panelA.props.close();
  const b = selectEq(show);
  const panelB = panelHost(b);
  assert.ok(panelB);
  panelA.props.close();
  assert.ok(panelHost(show()), 'saved A Back cannot close B');
  panelB.props.close();
  assert.equal(panelHost(show()), undefined);
});

test('same-tick old dismiss after a successor opens is refused', async () => {
  const {show} = await nowPlaying();
  const a = openMenu(show);
  const menuA = menu(a);
  menuA.onOpenChange(false);
  const b = openMenu(show);
  menuA.onOpenChange(false);
  assert.equal(one(b, 'Menu').props.open, true);
});

test('an ordinary position update keeps the actual EQ receipt', async () => {
  const {show, snapshot} = await nowPlaying();
  const opened = selectEq(show);
  const position = {...snapshot, presentation: {...snapshot.presentation, positionSeconds: 42}};
  const after = show(position);
  assert.ok(panelHost(after));
  assert.equal(panelHost(after).props.receipt, panelHost(opened).props.receipt);
  assert.equal(panelHost(after).props.hostReceipt, panelHost(opened).props.hostReceipt);
});
