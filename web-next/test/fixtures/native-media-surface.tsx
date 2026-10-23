// Real PlaybackHost/MediaSurface and CSS; synthetic runtime boundary, no native or HDR claim.
import {createRoot} from 'react-dom/client';
import {createMemoryHistory} from '@tanstack/react-router';
import {PlaybackDock, type QueuePlaybackFence} from '../../../packages/player-core/src/index.ts';
import {defaultI18n} from '@i18n';
import {PlaybackHost, type ShellPlaybackBinding} from '../../src/shell/Playback';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../src/platform/playback';
import {router} from '../../src/router';
import {IconSprite} from '../../src/kit/Icon';
import {Dialog} from '../../src/kit/overlays';
import {useState} from 'react';
import '../../src/kit/tokens.css';
import '../../src/kit/fonts.css';
import '../../src/bootstrap.css';

const scope = {serverId: 'synthetic-server', authority: 'local' as const, accountId: 'synthetic-account', profileId: 'synthetic-profile', controllerId: 'fixture', controllerEpoch: 'fixture', commandLaneId: 'fixture'};
const fence: QueuePlaybackFence = {kind: 'queue', playback: {scope, run: 1}, sessionId: 'fixture-session', itemId: 'fixture-item', sourceId: 'fixture-source', entryId: 'fixture-entry', generation: 1};
const dock = new PlaybackDock(scope), listeners = new Set<() => void>();
const state = {expanded: false, native: false, finished: false, hidden: false, audio: false, remote: false};
const proof = {mounts: 0, unmounts: 0, errors: [] as string[]};
let snapshot: WebPlaybackRuntimeSnapshot;
const refresh = () => {
  dock.reset(scope);
  const playback = {kind: state.audio ? 'music' as const : 'video' as const, fence, positionSeconds: 30, playing: !state.finished, expanded: state.expanded, queue: null};
  if (!state.hidden) {
    dock.start(playback);
    if (state.remote) dock.beginRemote({id: 'synthetic-remote', kind: 'cast', sequence: '1', phase: 'playing', playback, positionSeconds: 30}, 0);
  }
  // Only this explicit test boundary supplies snapshots. The production getter is checked separately.
  snapshot = {dock: dock.getSnapshot(), fence: state.hidden ? undefined : fence, nativeVideoSurfaceActive: state.native,
    presentation: state.hidden ? undefined : {fence, subject: {kind: state.audio ? 'music' : 'video', item: {id: 'fixture-item', kind: state.audio ? 'song' : 'movie', title: 'Synthetic media surface'}}, kind: state.audio ? 'music' : 'video', durationSeconds: 120, positionSeconds: 30, playing: !state.finished, expanded: state.expanded, finished: state.finished, controls: ['play', 'pause', 'seek'], volume: 1, muted: false, rate: 1, policy: null},
    presentations: [], sequence: 1, disposed: false} as unknown as WebPlaybackRuntimeSnapshot;
  for (const listener of listeners) listener();
};
refresh();
const runtime = {dock, subscribe: (listener: () => void) => {listeners.add(listener); return () => listeners.delete(listener);}, getSnapshot: () => snapshot,
  mount: (element: HTMLVideoElement) => {proof.mounts++; element.dataset.fixtureVideo = 'persistent'; return () => {proof.unmounts++;};},
  onAction: async (action: {kind: string}) => {if (action.kind === 'expand' || action.kind === 'collapse') {state.expanded = action.kind === 'expand'; refresh();}},
  collapse: () => {state.expanded = false; refresh();},
} as unknown as WebShellPlaybackRuntime;
const binding: ShellPlaybackBinding = {runtime, artwork: {foreground: () => undefined, ambient: () => null} as unknown as ShellPlaybackBinding['artwork'], environment: {t: defaultI18n.t, locale: 'en-US', timeZone: 'UTC'}, skips: {back: 10, forward: 30}, reportError: error => proof.errors.push(String(error))};
router.update({history: createMemoryHistory({initialEntries: ['/']})});
await router.load();
function Fixture() {
  const [dialog, setDialog] = useState(false);
  return <><IconSprite /><main id="portico-content" data-route-content style={{minHeight: '100vh', padding: 32, background: 'var(--color-ink)'}}><h1>Native media surface regression</h1><p>Synthetic runtime only. No native decoder or output proof.</p></main>
    <aside data-fixture-actions style={{position: 'fixed', right: 16, top: 16, zIndex: 100, display: 'flex', gap: 8, flexWrap: 'wrap', maxWidth: 700}}>
      {(['native', 'expanded', 'hidden', 'finished', 'audio', 'remote'] as const).map(key => <button type="button" key={key} data-fixture-toggle={key} onClick={() => {state[key] = !state[key]; refresh();}}>{key}</button>)}
      <button type="button" onClick={() => setDialog(true)}>Dialog</button>
    </aside>
    <PlaybackHost binding={binding} /><Dialog open={dialog} onOpenChange={setDialog} title="Synthetic overlay"><button type="button" onClick={() => setDialog(false)}>Close test dialog</button></Dialog>
  </>;
}
Object.assign(window, {nativeSurfaceFixture: {state, proof, set: (values: Partial<typeof state>) => {Object.assign(state, values); refresh();}}});
createRoot(document.getElementById('root')!).render(<Fixture />);
