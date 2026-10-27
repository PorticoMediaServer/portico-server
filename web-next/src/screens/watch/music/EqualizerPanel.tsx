import {useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {DeviceEqualizer, type EqualizerPort} from '../../../kit/media';
import {sameFence, type PlaybackFence} from '../../../../../packages/player-core/src/index.ts';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../../platform/playback';

const t = defaultI18n.t;

/** One committed opening of the player's Equalizer panel. Its runtime, device port, queue
 * authority and run are captured once; a replaced output or another source never inherits it. */
type Lease = {
  runtime: WebShellPlaybackRuntime; port: EqualizerPort; authority: object; fence: PlaybackFence;
  run: number; hostCurrent: () => boolean; live: boolean;
};

/** The drawn More › Equalizer panel (owner layout/opening design). It edits the same
 * device-local record as Settings; a write already issued stays on the port it was sent to. */
export function WatchEqualizerPanel({runtime, snapshot, port, hostCurrent, close, openSettings}: Readonly<{
  runtime: WebShellPlaybackRuntime; snapshot: WebPlaybackRuntimeSnapshot; port: EqualizerPort;
  hostCurrent: () => boolean; close: () => void; openSettings: () => void;
}>) {
  const lease = useRef<Lease | undefined>(undefined), [ready, setReady] = useState(false), [error, setError] = useState<unknown>();
  const live = (held: Lease | undefined): held is Lease => {
    if (!held || !held.live || !held.hostCurrent()) return false;
    const value = held.runtime.getSnapshot();
    return !value.disposed && value.authority === held.authority && !!value.fence && sameFence(value.fence, held.fence)
      && value.playback.playbackRun === held.run && value.presentation?.kind === 'music'
      && !!value.presentation.fence && sameFence(value.presentation.fence, held.fence)
      && held.runtime.equalizerPort === held.port;
  };
  // Admission is layout-committed from the live runtime, never from a rendered copy or a later lookup.
  useLayoutEffect(() => {
    const value = runtime.getSnapshot();
    if (value.disposed || !value.fence || value.fence.kind !== 'queue' || value.presentation?.kind !== 'music'
      || !value.presentation.fence || !sameFence(value.presentation.fence, value.fence) || !hostCurrent()
      || runtime.equalizerPort !== port) {setReady(false); return;}
    const held: Lease = {runtime, port, authority: value.authority, fence: value.fence, run: value.playback.playbackRun, hostCurrent, live: true};
    lease.current = held;
    setReady(true);
    return () => {held.live = false; if (lease.current === held) lease.current = undefined;};
  }, [runtime, port, hostCurrent]);
  // The panel edits the original device port while this opening is live. A write already
  // handed to that port keeps its own promise; it is never rebound to a newer output.
  const guarded = useMemo<EqualizerPort>(() => ({
    getSnapshot: () => port.getSnapshot(), subscribe: listener => port.subscribe(listener), start: () => port.start(),
    change: async value => {if (!live(lease.current)) throw new DOMException('Equalizer opening retired', 'AbortError'); return port.change(value);},
    reset: async () => {if (!live(lease.current)) throw new DOMException('Equalizer opening retired', 'AbortError'); return port.reset();},
  }), [port]);
  const value = useSyncExternalStore(guarded.subscribe, guarded.getSnapshot, () => null);
  const report = (failure: unknown) => {if (live(lease.current)) setError(failure);};
  const back = () => {if (live(lease.current)) close();};
  const settings = () => {if (live(lease.current)) openSettings();};
  return <div data-watch-equalizer data-layout="panel">
    {ready && value ? <DeviceEqualizer port={guarded} layout="panel" reportError={report} onBack={back} onOpenSettings={settings} />
      : <div><p role="status">{t('next.states.loading')}</p><Button size="small" onClick={back}>{t('next.media.eq.backQueue')}</Button></div>}
    {error !== undefined && <p role="alert">{t('next.states.pageFailedBody')}</p>}
  </div>;
}
