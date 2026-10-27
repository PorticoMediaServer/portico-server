/**
 * FX229 / BLK-PLAY-016: the host, More menu and Equalizer openings each have their own published
 * receipt, minted fresh in layout from immutable rendered facts.
 *
 * Real fences and the actual admission helpers are used; DeviceEqualizer/Menu are stub hosts. The
 * harness runs hooks eagerly and has no React StrictMode or unmount replay; a cleanup/setup replay
 * is modelled by a fresh layout setup with the same facts, not by relaunching the whole player.
 * Source only: unrun here.
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
  const runtime = {getSnapshot: () => state, equalizerPort: port};
  let host = true, scopeAllowed = true;
  const isCurrent = () => host, scope = {current: () => scopeAllowed};
  const calls = {close: 0, settings: 0};
  const facts = (value: any = state) => ({runtime: runtime as any, port: port as any, authority: value.authority,
    fenceAt: fenceKey(value.fence), presentationAt: fenceKey(value.presentation.fence), run: value.playback.playbackRun, isCurrent, scope});
  const receipts: {host: any; surface: any; publishedHost: {current: any}; published: {current: any}} = {host: null, surface: null, publishedHost: {current: null}, published: {current: null}};
  const setup = (value: any = state, reuseFacts?: any) => {
    const hostFacts = reuseFacts ?? facts(value);
    const hostReceipt = {facts: hostFacts, live: true};
    const surface = {host: hostFacts, live: true};
    receipts.host = hostReceipt; receipts.surface = surface;
    receipts.publishedHost.current = hostReceipt; receipts.published.current = surface;
    const paint = () => tree(h.render(() => panel.WatchEqualizerPanel({receipt: surface, rendered: surface, published: receipts.published,
      host: hostFacts, hostReceipt, publishedHost: receipts.publishedHost, runtime: runtime as any, port: port as any,
      isCurrent, scope, close: () => {calls.close++;}, openSettings: () => {calls.settings++;}})));
    paint();
    return {hostFacts, hostReceipt, surface, tree: paint()};
  };
  return {calls, changes, facts, held, receipts, scope, setHost: (value: boolean) => {host = value;}, setScope: (value: boolean) => {scopeAllowed = value;}, setup, state};
}

test('a live EQ opening writes the original port; ordinary rerender keeps it', async () => {
  const {changes, setup} = await mountPanel();
  const first = setup();
  const eq = one(first.tree, 'DeviceEqualizer').props;
  await eq.port.change(equalizer(false));
  assert.equal(changes.length, 1);
  const again = setup(undefined as any, first.hostFacts);
  assert.equal(one(again.tree, 'DeviceEqualizer').props.port, eq.port);
});

test('a fresh setup with the same facts retires the old EQ callbacks', async () => {
  const {calls, changes, setup} = await mountPanel();
  const first = setup();
  const oldEq = one(first.tree, 'DeviceEqualizer').props;
  const second = setup(undefined as any, first.hostFacts);
  assert.notEqual(second.surface, first.surface);
  oldEq.onBack();
  await assert.rejects(oldEq.port.change(equalizer(false)), /retired/i);
  assert.equal(calls.close, 0);
  const successor = one(second.tree, 'DeviceEqualizer').props;
  await successor.port.change(equalizer(false));
  assert.equal(changes.length, 1, 'the successor receipt works; the old one did not write');
});

test('a same-key host replacement withholds the old panel', async () => {
  const {facts, setup} = await mountPanel();
  const first = setup();
  const oldEq = one(first.tree, 'DeviceEqualizer').props;
  const replaced = setup(undefined as any, facts({...stateLike(first), authority: {}}));
  assert.notEqual(replaced.hostFacts, first.hostFacts);
  await assert.rejects(oldEq.port.change(equalizer(true)), /retired/i);
});

const stateLike = (value: any) => ({authority: value.hostFacts.authority, fence: fence(), playback: {playbackRun: 7}, presentation: {kind: 'music', fence: fence()}});

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
  return {show, setExpanded: (value: boolean) => {expanded = value;}, snapshot};
}

const panelHost = (v: any[]) => all(v, 'WatchEqualizerPanel')[0];
const moreButton = (v: any[]) => all(v, 'button').find(node => node.props['aria-label'] === 'player.more');

test('the actual saved More dismissal cannot clear a successor menu', async () => {
  const {setExpanded, show} = await nowPlaying();
  let v = show();
  const firstMenu = one(v, 'Menu').props;
  firstMenu.onOpenChange(true);
  v = show();
  assert.equal(one(v, 'Menu').props.open, true);
  setExpanded(false);
  show();
  setExpanded(true);
  v = show();
  moreButton(v)!.props.onClick();
  v = show();
  assert.equal(one(v, 'Menu').props.open, true, 'the successor More menu is open');
  firstMenu.onOpenChange(false);
  v = show();
  assert.equal(one(v, 'Menu').props.open, true, 'the old dismissal cannot close the successor');
  firstMenu.items[0].onSelect();
  v = show();
  assert.equal(panelHost(v), undefined, 'the old selection cannot open the successor EQ');
});

test('the actual saved EQ Back cannot close a reopened EQ', async () => {
  const {show} = await nowPlaying();
  let v = show();
  moreButton(v)!.props.onClick();
  v = show();
  one(v, 'Menu').props.items[0].onSelect();
  v = show();
  const firstPanel = panelHost(v);
  assert.ok(firstPanel);
  firstPanel.props.close();
  v = show();
  assert.equal(panelHost(v), undefined);

  moreButton(v)!.props.onClick();
  v = show();
  one(v, 'Menu').props.items[0].onSelect();
  v = show();
  const successor = panelHost(v);
  assert.ok(successor);
  firstPanel.props.close();
  v = show();
  assert.ok(panelHost(v), 'the saved Back cannot close the successor EQ');
  successor.props.close();
  v = show();
  assert.equal(panelHost(v), undefined);
});

test('an ordinary position update keeps the actual EQ opening live', async () => {
  const {show, snapshot} = await nowPlaying();
  let v = show();
  moreButton(v)!.props.onClick();
  v = show();
  one(v, 'Menu').props.items[0].onSelect();
  const opened = show();
  const position = {...snapshot, presentation: {...snapshot.presentation, positionSeconds: 42}};
  const after = show(position);
  assert.ok(panelHost(after));
  assert.equal(panelHost(after), panelHost(opened));
});
