import React, {createContext, useContext, useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {currentI18n, useI18n} from './i18n';
import {GroupFollower, GroupSessionService, type GroupPlayer, type GroupSessionSnapshot} from '@core/index.ts';
import {syncRate} from '../bridge/sync-rate';
import {usePlayer} from '../player/engine';
import {useSession} from './session';

const Ctx = createContext<GroupSessionService | null>(null);

/** Watch Together for the signed-in viewer. One service for the whole app, so a room survives
 * moving between pages; while in a room, the local player follows the group's clock and the
 * viewer's play, pause and seek become the group's (when they are allowed to control it). */
export function TogetherProvider({children}: {children: React.ReactNode}) {
  const {api, session} = useSession();
  const engine = usePlayer();
  const token = session?.accessToken ?? '';
  const tokenRef = useRef(token);
  tokenRef.current = token;
  const viewerKey = session ? `${session.viewer.serverId}/${session.viewer.accountId}/${session.viewer.profileId}` : '';
  const service = useMemo(() => (viewerKey ? new GroupSessionService({
    api,
    stream: (path, lastEventId, signal) => api.routeFetch(api.baseUrl + path, {signal, headers: {Accept: 'text/event-stream', Authorization: 'Bearer ' + tokenRef.current, ...(lastEventId ? {'Last-Event-ID': lastEventId} : {})}}),
    i18n: currentI18n(),
  }) : null), [api, viewerKey]);
  useEffect(() => () => service?.dispose(), [service]);
  const i18n = useI18n();
  useEffect(() => { service?.setI18n(i18n); }, [service, i18n]);

  const live = useRef(engine);
  live.current = engine;
  const phase = useSyncExternalStore(service?.subscribe ?? noSubscribe, () => service?.getSnapshot().phase ?? 'idle');
  const inRoom = phase === 'live' || phase === 'reconnecting';
  useEffect(() => {
    if (!service || !inRoom) return;
    const player: GroupPlayer = {
      state: () => {
        // PERF-21: read the live snapshot; the engine's `state` is settled to whole seconds, so
        // drift measured against it is up to a second stale and triggers needless corrections.
        const s = live.current.service.getSnapshot();
        return {itemId: s.itemId ?? '', positionSeconds: s.pendingSeek?.positionSeconds ?? s.positionSeconds, playing: s.phase === 'ready' && s.intent === 'playing', buffering: !!s.recovering || s.phase === 'starting', ready: s.phase === 'ready'};
      },
      load: (itemId, positionSeconds) => {
        live.current.play(itemId, positionSeconds);
        // The group only supplies an id; the title and artwork come from the library.
        void api.item(itemId).then(item => { if (live.current.state.itemId === itemId) live.current.describe({itemId, title: item.title, posterUrl: item.posterUrl, backdropUrl: item.backdropUrl, kind: item.kind, libraryId: item.libraryId}); }, () => {});
      },
      // The follower speaks to the player directly: going through the engine would send its own
      // corrections back to the group as commands.
      play: () => live.current.service.resume(),
      pause: () => live.current.service.pause(),
      seek: seconds => live.current.service.seek(seconds),
      setRate: rate => syncRate.set(rate),
    };
    const follower = new GroupFollower(service, player);
    follower.start();
    live.current.setGroupControl({
      toggle: playing => {
        const group = service.getSnapshot().group;
        if (!group?.timeline.itemId || group.timeline.itemId !== live.current.state.itemId) return false;
        if (group.permissions.canControl) void service.transport(playing ? 'pause' : 'play');
        else service.notice('Only the host can play and pause for the group.');
        return true;
      },
      seek: seconds => {
        const group = service.getSnapshot().group;
        if (!group?.timeline.itemId || group.timeline.itemId !== live.current.state.itemId) return false;
        if (group.permissions.canControl) void service.transport('seek', {positionUs: String(Math.round(seconds * 1e6))});
        else service.notice('Only the host can move the group to another moment.');
        return true;
      },
    });
    return () => { follower.stop(); live.current.setGroupControl(null); };
  }, [service, inRoom, api]);
  return <Ctx.Provider value={service}>{children}</Ctx.Provider>;
}

/** Development only: a room driven by a hand-made service, with no player attached. */
export const TogetherFixture = Ctx.Provider;

const idle: GroupSessionSnapshot = Object.freeze<GroupSessionSnapshot>({directory: [], directoryPhase: 'idle', phase: 'idle', busy: false, clockOffsetMs: 0});
const noSubscribe = () => () => {};
export function useTogether(): {service: GroupSessionService | null; state: GroupSessionSnapshot} {
  const service = useContext(Ctx);
  const state = useSyncExternalStore(service?.subscribe ?? noSubscribe, service?.getSnapshot ?? (() => idle));
  return {service, state};
}
/** Whether the viewer is in a room right now, without re-rendering on every clock tick. */
export function useInRoom(): boolean {
  const service = useContext(Ctx);
  return useSyncExternalStore(service?.subscribe ?? noSubscribe, () => { const p = service?.getSnapshot().phase; return p === 'live' || p === 'reconnecting'; });
}
/** What a title's menu needs: whether there is a room, and whether this viewer may add to it. */
export function useGroupActions(): {service: GroupSessionService | null; inRoom: boolean; canQueue: boolean} {
  const service = useContext(Ctx);
  const inRoom = useInRoom();
  const canQueue = useSyncExternalStore(service?.subscribe ?? noSubscribe, () => !!service?.getSnapshot().group?.permissions.canManageQueue);
  return {service, inRoom, canQueue};
}

/**
 * A title waiting to be played in the group the viewer is about to start
 * (WEB-TOGETHER-01: a title's "Watch Together" goes to /together with it
 * queued). In memory only; the Together screen takes it once.
 */
let pendingTogether: {itemId: string; title: string} | undefined;
export function setPendingTogether(item: {itemId: string; title: string}) { pendingTogether = item; }
export function takePendingTogether(): {itemId: string; title: string} | undefined { const item = pendingTogether; pendingTogether = undefined; return item; }
