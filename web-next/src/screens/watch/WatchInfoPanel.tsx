import {useEffect, useMemo, useRef, useState, type RefObject} from 'react';
import type {I18n} from '@i18n';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../platform/playback';
import type {QueuePlaybackFence} from '../../../../packages/player-core/src/index.ts';
import {PlaybackInfo, type PlaybackInfoProjection} from '../../kit/media';
import {playbackInfoCopy, watchPlaybackInfo} from './info-copy';

/** The drawn Playback info panel. It opens from More (frame 1), reads the current
 * committed comparison afresh once, and updates live when the route or track changes.
 * The media view stays mounted and playing underneath; no control is drawn over it. */
export function WatchInfoPanel({runtime, snapshot, fence, title, deviceName, i18n, layout, phoneTop, returnFocusRef, close, reportError}: {
  runtime: WebShellPlaybackRuntime; snapshot: WebPlaybackRuntimeSnapshot; fence: QueuePlaybackFence;
  title: string; deviceName?: string; i18n: I18n; layout: 'desktop' | 'phone'; phoneTop: number;
  returnFocusRef: RefObject<HTMLButtonElement | null>; close: () => void; reportError: (error: unknown) => void;
}) {
  const copy = useMemo(() => playbackInfoCopy(i18n), [i18n]);
  // The committed session already carries the server's own comparison; the fresh
  // read only refreshes it, so the panel never waits on a request to draw.
  const committed = snapshot.session;
  const [projection, setProjection] = useState<PlaybackInfoProjection | undefined>(() => committed?.playbackInfo
    ? watchPlaybackInfo(committed.playbackInfo, committed.observedPlayback, i18n, title, deviceName) : undefined);
  // A changed fence object must not re-read on every observation tick: only the
  // committed session's own revision and presentation generation move the facts.
  const live = useRef(fence); live.current = fence;
  const key = `${fence.kind}:${fence.sessionId}:${fence.generation}:${committed ? `${committed.revision}:${committed.presentation.generation}` : ''}`;
  useEffect(() => {
    const expected = live.current;
    if (expected.kind !== 'queue') return;
    const controller = new AbortController();
    let current = true;
    void runtime.readPlaybackInfo(expected, controller.signal).then(read => {
      if (!current || !read || !read.isCurrent() || !read.playbackInfo) return;
      const next = watchPlaybackInfo(read.playbackInfo, read.observedPlayback, i18n, title, deviceName);
      if (current && next) setProjection(next);
    }).catch(error => {
      if (!current || (error instanceof DOMException && error.name === 'AbortError')) return;
      reportError(error);
    });
    return () => { current = false; controller.abort(); };
  }, [runtime, key, title, deviceName, i18n, reportError]);
  if (!projection) return null;
  return <PlaybackInfo projection={projection} copy={copy} layout={layout} open onOpenChange={open => { if (!open) close(); }}
    returnFocusRef={returnFocusRef} phoneTop={phoneTop} />;
}
