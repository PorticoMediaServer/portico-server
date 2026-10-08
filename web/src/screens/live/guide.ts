import {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {ChannelGuideService, channelOperationId, guideWindow, saveChannelPreference, type ChannelGuide, type ChannelKind, type GuideChannel, type GuideRoute, type GuideSnapshot} from '@core/channel-guide.ts';
import {DVRClient} from '@core/dvr.ts';
import {defaultI18n} from '@i18n';
import {useSession} from '../../app/session';
import {errorText} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';

const HOUR = 3600000;

export function localTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/** Formatters bound to the guide's timezone so the ruler matches the server viewport. */
export function useGuideFormat(timezone: string) {
  return useMemo(() => {
    let time: Intl.DateTimeFormat | null = null, weekday: Intl.DateTimeFormat | null = null, day: Intl.DateTimeFormat | null = null;
    try {
      time = new Intl.DateTimeFormat(undefined, {hour: 'numeric', minute: '2-digit', timeZone: timezone});
      weekday = new Intl.DateTimeFormat(undefined, {weekday: 'long', month: 'short', day: 'numeric', timeZone: timezone});
      day = new Intl.DateTimeFormat('en-CA', {year: 'numeric', month: '2-digit', day: '2-digit', timeZone: timezone});
    } catch {}
    const clock = (ms: number) => (time ? time.format(new Date(ms)) : new Date(ms).toISOString().slice(11, 16));
    const dayKey = (ms: number) => (day ? day.format(new Date(ms)) : new Date(ms).toISOString().slice(0, 10));
    const dayLabel = (ms: number) => {
      const key = dayKey(ms), now = Date.now();
      if (key === dayKey(now)) return defaultI18n.t('web.live.today');
      if (key === dayKey(now + 24 * HOUR)) return defaultI18n.t('web.live.tomorrow');
      if (key === dayKey(now - 24 * HOUR)) return defaultI18n.t('relative.yesterday');
      return weekday ? weekday.format(new Date(ms)) : key;
    };
    const range = (startMs: number, endMs: number) => `${clock(startMs)} – ${clock(endMs)}`;
    return {clock, dayLabel, range};
  }, [timezone]);
}


export type ChannelPreference = {favorite: boolean; hidden: boolean};
export type ChannelGuideBinding = {
  snapshot: GuideSnapshot; guide: ChannelGuide | null; route: GuideRoute; loading: boolean; refreshing: boolean;
  refresh: () => void; retry: () => void; next: () => void; previous: () => void; shift: (hours: number) => void; jumpToNow: () => void;
  setSearch: (v: string) => void; setFavorites: (v: boolean) => void; setIncludeHidden: (v: boolean) => void; setSource: (id: string) => void; setGroup: (group: string) => void; clearFilters: () => void; filtersActive: boolean;
  setPreference: (channel: GuideChannel, values: ChannelPreference) => void; pendingChannel: string | null; actionError: string; clearActionError: () => void;
};

/**
 * Binds one ChannelGuideService to the current viewer. The service owns
 * fetching, fences and paging; this hook owns the route (viewport + filters)
 * and channel preference writes.
 */
export function useChannelGuide(kind: ChannelKind): ChannelGuideBinding {
  const {api, session} = useSession();
  const scope = useViewerScope();
  const serverId = session?.viewer.serverId ?? '';
  const service = useMemo(() => new ChannelGuideService(api, serverId), [api, serverId, scope.viewerId]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => service.dispose(), [service]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const [route, setRoute] = useState<GuideRoute>(() => ({kind, ...guideWindow(), timezone: localTimezone(), search: '', sourceId: ''}));
  useEffect(() => {
    void service.load(route);
  }, [service, route]);
  const reload = useCallback((mode: 'refresh' | 'next' | 'previous') => {
    const current = service.getSnapshot().route ?? route;
    void service.load(current, mode);
  }, [service, route]);
  const patch = useCallback((values: Partial<GuideRoute>) => setRoute(prev => ({...prev, ...values})), []);
  const requests = useRef(new Map<string, string>());
  const [pendingChannel, setPendingChannel] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  const setPreference = useCallback((channel: GuideChannel, values: ChannelPreference) => {
    const key = JSON.stringify([channel.id, channel.preferenceRevision, values]);
    setActionError('');
    setPendingChannel(channel.id);
    void (async () => {
      try {
        let requestId = requests.current.get(key);
        if (!requestId) {
          requestId = await channelOperationId(() => Promise.resolve(crypto.randomUUID()));
          requests.current.set(key, requestId);
        }
        await saveChannelPreference(api, serverId, channel, requestId, values);
        requests.current.delete(key);
        const current = service.getSnapshot().route;
        if (current) await service.load(current, 'refresh');
      } catch (e) {
        setActionError(errorText(e, 'live', 'save'));
      } finally {
        setPendingChannel(prev => (prev === channel.id ? null : prev));
      }
    })();
  }, [api, serverId, service]);
  const filtersActive = !!route.search || route.favorites === true || route.includeHidden === true || !!route.sourceId || !!route.group;
  return {
    snapshot, guide: snapshot.guide, route,
    loading: snapshot.phase === 'loading' || (snapshot.phase === 'idle' && !snapshot.guide),
    refreshing: snapshot.phase === 'retained-refreshing',
    refresh: () => reload('refresh'), retry: () => void service.load(route), next: () => reload('next'), previous: () => reload('previous'),
    shift: hours => patch(guideWindow(new Date(Date.parse(route.start) + hours * HOUR))),
    jumpToNow: () => patch(guideWindow()),
    setSearch: v => patch({search: v}), setFavorites: v => patch({favorites: v || undefined}), setIncludeHidden: v => patch({includeHidden: v || undefined}), setSource: sourceId => patch({sourceId}), setGroup: group => patch({group: group || undefined}),
    clearFilters: () => patch({search: '', favorites: undefined, includeHidden: undefined, sourceId: '', group: undefined}),
    filtersActive, setPreference, pendingChannel, actionError, clearActionError: () => setActionError(''),
  };
}

/** 48 hex characters: the DVR's request-id grammar. */
function dvrRequestId(): Promise<string> {
  const bytes = crypto.getRandomValues(new Uint8Array(24));
  return Promise.resolve(Array.from(bytes, b => b.toString(16).padStart(2, '0')).join(''));
}

/** One DVR client per signed-in server, shared by the guide's Record actions and the DVR views
 * (FEAT-02 / CD-04: the DVR views used to send raw requests with the wrong revision field). */
export function useDVRClient(): DVRClient | null {
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const client = useMemo(() => (serverId ? new DVRClient(api, serverId, dvrRequestId) : null), [api, serverId]);
  useEffect(() => {
    if (!client) return;
    client.activate();
    return () => client.dispose();
  }, [client]);
  return client;
}
