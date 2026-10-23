import {shellLanguage} from '../../shell/settings';
import {GroupSessionService, type GroupSessionSnapshot} from '@core/group-session.ts';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import type {GetItemsIdResponse} from '../../../../packages/contracts/src/types.ts';
import {parseMediaItem} from '@core/media-item.ts';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import type {WatchBindings} from '../watch';
import {router} from '../../router';

export type TogetherSnapshot = Readonly<{room: GroupSessionSnapshot; items: ReadonlyMap<string, GetItemsIdResponse>}>;
/** Inert construction. The persistent Watch parent starts/disposes it for one captured session. */
export function createTogetherBindings(context: ShellAssemblyContext, playback: WatchBindings) {
  const captured = context.session(), life = new AbortController();
  let i18n = shellLanguage(context.preferences?.getSnapshot('web') ?? context.facts.preferences);
  const isCurrent = () => context.isCurrent() && !life.signal.aborted && !playback.runtime.getSnapshot().disposed
    && context.session().sessionFamilyId === captured.sessionFamilyId
    && (['serverId', 'authority', 'accountId', 'profileId'] as const).every(key => context.session().viewer[key] === captured.viewer[key]);
  const guard = () => {if (!isCurrent()) throw new DOMException('Room authority retired', 'AbortError');};
  const api = {baseUrl: context.api.baseUrl, async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    guard();const active = signal ? AbortSignal.any([signal, life.signal]) : life.signal;
    const value = await context.api.request<T>(path, method, body, active);guard();active.throwIfAborted();return value;
  }};
  const service = new GroupSessionService({api, i18n, stream: async (path, lastEventId, signal) => {
    guard();const url = new URL(path, context.api.baseUrl);
    if (!path.startsWith('/v1/groups/') || url.origin !== new URL(context.api.baseUrl).origin) throw new Error('invalid_group_stream');
    const value = await context.api.routeFetch(url.href, {signal: AbortSignal.any([signal, life.signal]), headers: {Accept: 'text/event-stream', Authorization: `Bearer ${context.session().accessToken}`,
      ...(lastEventId ? {'Last-Event-ID': lastEventId} : {})}});
    guard();return value;
  }});
  const listeners = new Set<() => void>();
  let snapshot: TogetherSnapshot = {room: service.getSnapshot(), items: new Map()}, key = '', reading: AbortController | undefined, offGroup: (() => void) | undefined, offPreferences: (() => void) | undefined, started = false;
  const publish = (items = snapshot.items) => {snapshot = Object.freeze({room: service.getSnapshot(), items});for (const listener of [...listeners]) listener();};
  const off = service.subscribe(() => {
    publish();const room = snapshot.room, group = room.group, queue = room.queue ?? group?.queue;
    const next = group && (room.phase === 'live' || room.phase === 'reconnecting') ? `${group.id}:${queue?.revision ?? group.queueRevision}:${group.timeline.itemId}` : '';
    if (next === key) return;key = next;reading?.abort();reading = undefined;publish(new Map());
    if (!next || !group || !isCurrent()) return;
    const controller = new AbortController();reading = controller;
    const ids = [...new Set([group.timeline.itemId, ...(queue?.entries.filter(entry => !entry.unavailable).map(entry => entry.itemId) ?? [])].filter((id): id is string => !!id))];
    void (async () => {
      // Bounded visible room window, one actual entitled catalogue read at a time.
      for (const id of ids.slice(0, 50)) {
        if (!isCurrent() || controller.signal.aborted || key !== next) return;
        try {
          const raw = await api.request<GetItemsIdResponse>(encodedRoutePath('get_items_id', {id: encodeURIComponent(id)}), 'GET', undefined, controller.signal);
          if (parseMediaItem(raw).id !== id || controller.signal.aborted || key !== next) continue;
          const values = new Map(snapshot.items);values.set(id, raw);publish(values);
        } catch (error) {if (controller.signal.aborted || !isCurrent()) return;}
      }
    })();
  });
  return {
    service, runtime: playback.runtime, environment: playback.environment, artwork: playback.artwork, get t() {return i18n.t;}, get number() {return i18n.number;},
    profileName: context.facts.identity.profileName, profileArt: context.facts.identity.profileArt,
    openSettings: () => {guard();void router.navigate({to: '/settings'});},
    isCurrent, getSnapshot: () => snapshot, subscribe: (listener: () => void) => {listeners.add(listener);return () => {listeners.delete(listener);};},
    start: () => {guard();if (started) return;started = true;offGroup = playback.runtime.bindGroup(service, isCurrent, playback.reportError);
      offPreferences = context.preferences?.subscribe(() => {if (isCurrent()) {i18n = shellLanguage(context.preferences?.getSnapshot('web') ?? context.facts.preferences);service.setI18n(i18n);publish();}});
    },
    reportError: playback.reportError,
    dispose: () => {if (life.signal.aborted) return;life.abort();reading?.abort();offGroup?.();offPreferences?.();off();service.dispose();snapshot = {room: service.getSnapshot(), items: new Map()};for (const listener of [...listeners]) listener();listeners.clear();},
  };
}
export type TogetherBindings = ReturnType<typeof createTogetherBindings>;
