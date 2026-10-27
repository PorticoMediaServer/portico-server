import {useMemo, useRef, useState, useSyncExternalStore, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {DeviceEqualizer, type EqualizerPort} from '../../../kit/media';
import {sameFence} from '../../../../../packages/player-core/src/index.ts';
import {fenceKey} from '../../../shell/playback-model';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../../platform/playback';

const t = defaultI18n.t;

/** One published host opening, minted in layout from the rendered playback facts. */
export type WatchEqualizerOpening = {
  runtime: WebShellPlaybackRuntime; port: EqualizerPort; authority: object;
  fenceAt: string; presentationAt: string; run: number;
  isCurrent?: () => boolean; scope?: Readonly<{current(): boolean}>;
  live: boolean;
};

/** The live source still being the admitted one. Position/preference updates never replace it. */
export const equalizerFactsCurrent = (held: WatchEqualizerOpening): boolean => {
  if (!held.live) return false;
  const value = held.runtime.getSnapshot();
  return !value.disposed && value.authority === held.authority && !!value.fence && fenceKey(value.fence) === held.fenceAt
    && value.playback.playbackRun === held.run && value.presentation?.kind === 'music'
    && !!value.presentation.fence && fenceKey(value.presentation.fence) === held.presentationAt
    && held.runtime.equalizerPort === held.port;
};

/** The original host (isCurrent + scope) and published receipt, checked before and after the
 * facts read so a predicate that reenters or retires cannot slip a write through. */
export const equalizerAdmit = (held: WatchEqualizerOpening | null, published: RefObject<WatchEqualizerOpening | null>, rendered: WatchEqualizerOpening | null): held is WatchEqualizerOpening => {
  if (!held || !rendered || held !== rendered || published.current !== held || !held.live) return false;
  if (held.isCurrent && !held.isCurrent()) return false;
  if (held.scope && !held.scope.current()) return false;
  if (!equalizerFactsCurrent(held)) return false;
  if (published.current !== held) return false;
  if (held.isCurrent && !held.isCurrent()) return false;
  if (held.scope && !held.scope.current()) return false;
  return equalizerFactsCurrent(held);
};

/** The drawn More › Equalizer panel (music-plus frame 12): the side panel swaps Up next for the
 * same device record Settings edits. The parent owns the published opening; this view only shows
 * it while the render facts are that opening and every action closes over it. */
export function WatchEqualizerPanel({opening, rendered, published, runtime, port, isCurrent, scope, close, openSettings}: Readonly<{
  opening: WatchEqualizerOpening | null; rendered: WatchEqualizerOpening | null;
  published: RefObject<WatchEqualizerOpening | null>;
  runtime: WebShellPlaybackRuntime; port: EqualizerPort;
  isCurrent?: () => boolean; scope?: Readonly<{current(): boolean}>;
  close: () => void; openSettings: () => void;
}>) {
  const [error, setError] = useState<unknown>();
  const matches = (held: WatchEqualizerOpening | null) => !!held && held.runtime === runtime && held.port === port
    && held.isCurrent === isCurrent && held.scope === scope;
  const shown = matches(opening) && opening === rendered && published.current === opening ? opening : null;
  const guarded = useMemo<EqualizerPort>(() => ({
    getSnapshot: () => port.getSnapshot(), subscribe: listener => port.subscribe(listener), start: () => port.start(),
    change: async value => {
      if (!equalizerAdmit(opening, published, rendered)) throw new DOMException('Equalizer opening retired', 'AbortError');
      return port.change(value);
    },
    reset: async () => {
      if (!equalizerAdmit(opening, published, rendered)) throw new DOMException('Equalizer opening retired', 'AbortError');
      return port.reset();
    },
  }), [opening, rendered, published, port]);
  const value = useSyncExternalStore(guarded.subscribe, guarded.getSnapshot, () => null);
  const back = () => {if (equalizerAdmit(opening, published, rendered)) close();};
  const settings = () => {if (equalizerAdmit(opening, published, rendered)) openSettings();};
  const report = (failure: unknown) => {if (equalizerAdmit(opening, published, rendered)) setError(failure);};
  return <div data-watch-equalizer data-layout="panel">
    {shown && value ? <DeviceEqualizer port={guarded} layout="panel" reportError={report} onBack={back} onOpenSettings={settings} />
      : shown ? <div><p role="status">{t('next.states.loading')}</p><Button size="small" onClick={back}>{t('next.media.eq.backQueue')}</Button></div> : null}
    {error !== undefined && shown && <p role="alert">{t('next.states.pageFailedBody')}</p>}
  </div>;
}
