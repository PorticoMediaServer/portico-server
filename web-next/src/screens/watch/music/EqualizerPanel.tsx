import {useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {DeviceEqualizer, type EqualizerPort} from '../../../kit/media';
import {sameFence, type PlaybackFence} from '../../../../../packages/player-core/src/index.ts';
import {fenceKey} from '../../../shell/playback-model';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../../platform/playback';

const t = defaultI18n.t;

/** The immutable facts of one rendered host opening. They come from this render's snapshot,
 * never from a later runtime read; the published admission and its callbacks close over them. */
type Facts = Readonly<{
  runtime: WebShellPlaybackRuntime; port: EqualizerPort; authority: object; fence: PlaybackFence;
  run: number; active: boolean; scope?: Readonly<{current(): boolean}>;
}>;
type Admission = Facts & {key: string; live: boolean};
type HostScope = Readonly<{current(): boolean}>;
const keyOf = (value: Facts) => `${fenceKey(value.fence)}|${value.run}|${value.active ? 1 : 0}`;
/** The exact host opening a More entry or panel callback was drawn for. */
export const equalizerHostCurrent = (runtime: WebShellPlaybackRuntime, port: EqualizerPort, snapshot: WebPlaybackRuntimeSnapshot, active: boolean, scope?: HostScope): boolean => {
  if (!active || (scope && !scope.current())) return false;
  const value = runtime.getSnapshot();
  return !value.disposed && value.authority === snapshot.authority && !!value.fence && !!snapshot.fence && sameFence(value.fence, snapshot.fence)
    && value.playback.playbackRun === snapshot.playback.playbackRun && value.presentation?.kind === 'music'
    && !!value.presentation.fence && sameFence(value.presentation.fence, value.fence) && runtime.equalizerPort === port;
};
const factsOf = (runtime: WebShellPlaybackRuntime, port: EqualizerPort, snapshot: WebPlaybackRuntimeSnapshot, active: boolean, scope?: HostScope): Facts | undefined => {
  if (snapshot.disposed || !snapshot.fence || snapshot.fence.kind !== 'queue' || snapshot.presentation?.kind !== 'music'
    || !snapshot.presentation.fence || !sameFence(snapshot.presentation.fence, snapshot.fence) || runtime.equalizerPort !== port) return undefined;
  return {runtime, port, authority: snapshot.authority, fence: snapshot.fence, run: snapshot.playback.playbackRun, active, ...(scope ? {scope} : {})};
};

/** The drawn More › Equalizer panel (music-plus frame 12): the side panel swaps Up next for the
 * same device record Settings edits. One committed opening owns its writes, Back and Settings;
 * ordinary position renders never mint a new admission. */
export function WatchEqualizerPanel({runtime, snapshot, port, active, scope, close, openSettings}: Readonly<{
  runtime: WebShellPlaybackRuntime; snapshot: WebPlaybackRuntimeSnapshot; port: EqualizerPort;
  active: boolean; scope?: HostScope; close: () => void; openSettings: () => void;
}>) {
  const [admission, setAdmission] = useState<Admission | null>(null), [error, setError] = useState<unknown>();
  const published = useRef<Admission | null>(null);
  // The rendered facts of this commit: a changed source/run/scope mints a new admission;
  // an ordinary position render keeps the same key and leaves the admission alone.
  const source = useRef(snapshot); source.current = snapshot;
  const facts = factsOf(runtime, port, snapshot, active, scope);
  const key = facts ? keyOf(facts) : '';
  // Setup mints the one admission for these rendered facts and publishes it. Its cleanup retires
  // exactly that receipt. A changed source/run/scope withholds the panel until the next commit.
  useLayoutEffect(() => {
    const rendered = factsOf(runtime, port, source.current, active, scope);
    if (!rendered) {published.current = null; setAdmission(null); return;}
    const minted: Admission = {...rendered, key, live: true};
    published.current = minted;
    setAdmission(minted);
    setError(undefined);
    return () => {minted.live = false; if (published.current === minted) published.current = null;};
  }, [runtime, port, active, key, scope, snapshot.authority]);
  const shown = admission && published.current === admission && admission.live && keyOf(admission) === key ? admission : null;
  const liveFacts = (held: Admission) => {
    if (!held.live) return false;
    const value = held.runtime.getSnapshot();
    return !value.disposed && value.authority === held.authority && !!value.fence && sameFence(value.fence, held.fence)
      && value.playback.playbackRun === held.run && value.presentation?.kind === 'music'
      && !!value.presentation.fence && sameFence(value.presentation.fence, held.fence)
      && held.runtime.equalizerPort === held.port;
  };
  // Exact identity + the original host predicate, then the same facts again before acting.
  const admit = (held: Admission) => liveFacts(held) && held.active && (!held.scope || held.scope.current()) && liveFacts(held);
  const guarded = useMemo<EqualizerPort>(() => ({
    getSnapshot: () => port.getSnapshot(), subscribe: listener => port.subscribe(listener), start: () => port.start(),
    change: async value => {
      const held = shown;
      if (!held || !admit(held)) throw new DOMException('Equalizer opening retired', 'AbortError');
      return held.port.change(value);
    },
    reset: async () => {
      const held = shown;
      if (!held || !admit(held)) throw new DOMException('Equalizer opening retired', 'AbortError');
      return held.port.reset();
    },
  }), [shown, port]);
  const value = useSyncExternalStore(guarded.subscribe, guarded.getSnapshot, () => null);
  const back = () => {const held = shown; if (held && admit(held)) close();};
  const settings = () => {const held = shown; if (held && admit(held)) openSettings();};
  const report = (failure: unknown) => {const held = shown; if (held && admit(held)) setError(failure);};
  return <div data-watch-equalizer data-layout="panel">
    {shown && value ? <DeviceEqualizer port={guarded} layout="panel" reportError={report} onBack={back} onOpenSettings={settings} />
      : shown ? <div><p role="status">{t('next.states.loading')}</p><Button size="small" onClick={back}>{t('next.media.eq.backQueue')}</Button></div> : null}
    {error !== undefined && shown && <p role="alert">{t('next.states.pageFailedBody')}</p>}
  </div>;
}
