import {useLayoutEffect, useRef, type Dispatch, type SetStateAction} from 'react';
import {sameFence} from '../../../../packages/player-core/src/index.ts';
import {createWatchBodyBindings, useCapturedScreenContext, type CapturedScreenContextInput, type ScreenContext, type WatchBodyBindings, type WatchBodyTarget} from '../detail';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../platform/playback';

export type RetainedWatchBody = Readonly<{runtime: WebShellPlaybackRuntime; context: ScreenContext; key: string; binding: WatchBodyBindings; dispose(): void}>;
export function watchBodyTarget(snapshot: WebPlaybackRuntimeSnapshot | undefined): WatchBodyTarget | undefined {
  const fence = snapshot?.fence, presentation = snapshot?.presentation, session = snapshot?.session, offers = snapshot?.offers;
  if (snapshot?.disposed || fence?.kind !== 'queue' || presentation?.subject.kind !== 'video' || !sameFence(presentation.fence, fence)
    || session?.id !== fence.sessionId || session.itemId !== fence.itemId || session.presentation.generation !== fence.generation
    || !offers || offers.scope.serverId !== snapshot.authority.serverId || offers.scope.serverId !== fence.playback.scope.serverId
    || offers.scope.itemId !== fence.itemId || offers.current?.sessionId !== fence.sessionId
    || offers.current.generation !== fence.generation || offers.current.sourceId !== fence.sourceId) return undefined;
  const item = presentation.subject.item, kind = item.kind;
  if (item.id !== fence.itemId || kind !== 'movie' && kind !== 'episode' && kind !== 'extra') return undefined;
  return Object.freeze({libraryId: offers.scope.libraryId, itemId: item.id, kind, ...(item.show?.id ? {showId: item.show.id} : {})});
}
export const watchBodyKey = (target: WatchBodyTarget) => JSON.stringify([target.libraryId, target.itemId, target.kind]);

/** The factory and its real view state survive temporary source/offer gaps and every layout.
 * Only a verified new title or captured authority retires them. This owns no media or route. */
export function WatchBodyOwner({captured, runtime, target, onReady}: {
  captured: CapturedScreenContextInput; runtime: WebShellPlaybackRuntime; target: WatchBodyTarget | undefined;
  onReady: Dispatch<SetStateAction<RetainedWatchBody | undefined>>;
}) {
  const context = useCapturedScreenContext(captured), current = useRef<RetainedWatchBody | undefined>(undefined);
  useLayoutEffect(() => {
    const before = current.current;
    if (before && (before.runtime !== runtime || before.context !== context || !context.isCurrent())) {
      before.dispose();current.current = undefined;onReady(value => value === before ? undefined : value);
    }
    if (!target || !context.isCurrent() || runtime.getSnapshot().disposed) return;
    const key = watchBodyKey(target), held = current.current;
    if (held?.key === key) return;
    held?.dispose();
    const life = new AbortController(), binding = createWatchBodyBindings(context, target);
    const value: RetainedWatchBody = {runtime, context, key, binding, dispose: () => {life.abort();binding.dispose();}};
    current.current = value;onReady(value);
    void binding.load(life.signal).catch(error => {if (!life.signal.aborted && context.isCurrent()) context.effects.reportError(error);});
  }, [context, runtime, target, onReady]);
  useLayoutEffect(() => () => {
    const value = current.current;current.current = undefined;value?.dispose();
    onReady(before => before === value ? undefined : before);
  }, [onReady]);
  return null;
}
