import type {HttpLocalApi} from '@core/index.ts';
import {ConsoleClient, type ConsoleScope} from '@core/console.ts';
import {DashboardDataClient, type DashboardHistory, type DashboardPeople, type DashboardStatus, type DashboardTopKind} from '@core/admin/dashboard.ts';
import {parseDashboardSessions} from '@core/admin/activity-summary/parser.ts';
import type {DashboardSessionsPage} from '@core/admin/activity-summary/types.ts';
import {parseAdminSession, type AdminSession} from '@core/playback-v1/types.ts';
import {call} from '@core/playback-v1/http.ts';
import {routePath} from '../../../../packages/contracts/src/routes.ts';
import {AdminSessionsClient} from '@core/playback-v1/admin-sessions.ts';
import {localV1Http} from '@core/playback-v1/local-http.ts';
import {playbackControlsService} from '@core/playback-controls.ts';
import {sendTransport} from '@core/server-admin/now-playing.ts';
import {fetchPlayHistory, type PlayHistoryApi, type PlayHistoryFilters, type PlayHistoryPage} from '@core/play-history.ts';
import type {TelemetryReading, TelemetryWindow} from '@core/telemetry.ts';

export type DashboardEvents = Readonly<{on(type: string, listener: () => void): () => void; onResync?(listener: () => void): () => void}>;
export type DashboardAlert = DashboardStatus['alerts'][number];
export type DashboardSession = AdminSession & Pick<DashboardSessionsPage['items'][number], 'channel' | 'programme'>;
export type DashboardSessions = Omit<Awaited<ReturnType<AdminSessionsClient['list']>>, 'items'> & {items: readonly DashboardSession[]};
export type DashboardBindings = Readonly<{
  scope: ConsoleScope; role: 'owner' | 'admin' | 'member'; isCurrent: () => boolean; isMainProfile: () => boolean;
  events?: DashboardEvents;
  subscribeAuthority?: (listener: () => void) => () => void;
  read: Readonly<{
    sessions: (cursor?: string, signal?: AbortSignal) => Promise<DashboardSessions>;
    status: (signal?: AbortSignal) => Promise<DashboardStatus>;
    telemetry: (range: TelemetryWindow, signal?: AbortSignal) => Promise<TelemetryReading>;
    people: (filters: PlayHistoryFilters, cursor?: string, signal?: AbortSignal) => Promise<DashboardPeople>;
    history: (filters: PlayHistoryFilters, kind: DashboardTopKind, signal?: AbortSignal) => Promise<DashboardHistory>;
    plays: (filters: PlayHistoryFilters, cursor?: string, signal?: AbortSignal) => Promise<PlayHistoryPage>;
  }>;
  effects: Readonly<{
    transport: (id: string, command: 'pause' | 'resume') => Promise<void>;
    stop: (id: string, message?: string) => Promise<void>;
    openHistory: () => void; openSettings: (section: string) => void;
    /** Supplied by the action owner; mutation must retain its existing revision/idempotency guard. */
    alert?: (alert: DashboardAlert) => Promise<void>;
    reportError: (error: unknown) => void;
  }>;
  artwork: (itemId: string, kind: 'poster' | 'backdrop' | 'square', revision?: string) => string | undefined;
  dispose: () => void;
}>;
export type DashboardBindingOptions = Pick<DashboardBindings, 'scope' | 'role' | 'isCurrent' | 'isMainProfile' | 'events' | 'artwork' | 'subscribeAuthority'> & {
  api: Pick<HttpLocalApi, 'request' | 'requestRaw'>;
  effects: Pick<DashboardBindings['effects'], 'openHistory' | 'openSettings' | 'reportError' | 'alert'>;
};

/** Captures one selected-server/main-profile lifetime. All asynchronous reads and writes recheck it. */
export function createDashboardBindings(options: DashboardBindingOptions): DashboardBindings {
  const life = new AbortController(), listeners = new Set<() => void>();
  const close = () => {if (!life.signal.aborted) {life.abort(); for (const listener of [...listeners]) listener();}};
  const current = () => !life.signal.aborted && options.isCurrent() && options.isMainProfile() && (options.role === 'owner' || options.role === 'admin');
  const assert = (owner = false) => {if (!current()) {close(); throw new DOMException('Dashboard authority changed', 'AbortError');} if (owner && options.role !== 'owner') throw new DOMException('Dashboard authority changed', 'AbortError');};
  const offAuthority = options.subscribeAuthority?.(() => {if (!current()) close();});
  const api: PlayHistoryApi = {request: async <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
    assert(); try {const result = await options.api.request<T>(path, method, body, signal ? AbortSignal.any([signal, life.signal]) : life.signal); assert(); return result;} catch (error) {if (error && typeof error === 'object' && 'status' in error && (error.status === 401 || error.status === 403)) close(); throw error;}
  }};
  const http = localV1Http({requestRaw: async (...args: Parameters<HttpLocalApi['requestRaw']>) => {
    assert(); const [path, method, init] = args;
    const result = await options.api.requestRaw(path, method, {...init, signal: init?.signal ? AbortSignal.any([init.signal, life.signal]) : life.signal}); if (result.status === 401 || result.status === 403) close(); assert(); return result;
  }});
  const data = new DashboardDataClient(api), sessions = new AdminSessionsClient(http), consoleClient = new ConsoleClient(api, options.scope);
  const controls = playbackControlsService(api.request.bind(api));
  let fence: string | undefined;
  let observed: string | undefined;
  const ownerRead = async <T>(work: () => Promise<T>) => {assert(true); const value = await work(); assert(true); return value;};
  return {
    scope: {...options.scope}, role: options.role, isCurrent: current, isMainProfile: options.isMainProfile, events: options.events, subscribeAuthority: listener => {listeners.add(listener); return () => {listeners.delete(listener);};}, artwork: options.artwork,
    read: {
      sessions: async (cursor, signal) => {
        assert(); const query = new URLSearchParams({limit: '10'}); if (cursor) query.set('cursor', cursor);
        const raw = parseDashboardSessions((await call(http, {method: 'GET', path: routePath('list_admin_sessions') + '?' + query, signal})).body); assert();
        if (raw.scope.serverId !== options.scope.serverId || fence !== undefined && fence !== raw.scope.viewerFence || observed !== undefined && Date.parse(raw.observedAt) < Date.parse(observed)) {close(); throw new DOMException('Dashboard authority changed', 'AbortError');}
        fence = raw.scope.viewerFence; observed = raw.observedAt;
        return {items: raw.items.map(item => ({...parseAdminSession(item), channel: item.channel, programme: item.programme})), nextCursor: raw.page.nextCursor ?? '', scope: raw.scope, totals: raw.totals};
      },
      status: signal => ownerRead(() => data.status(signal)),
      telemetry: (range, signal) => ownerRead(async () => {if (signal?.aborted) throw signal.reason; const value = await consoleClient.telemetry(range); if (signal?.aborted) throw signal.reason; return value;}),
      people: (filters, cursor, signal) => ownerRead(() => data.people(filters, cursor, 100, signal)),
      history: (filters, kind, signal) => ownerRead(() => data.history(filters, kind, signal)),
      plays: (filters, cursor, signal) => ownerRead(() => fetchPlayHistory(api, filters, cursor, signal)),
    },
    effects: {...options.effects,
      transport: async (id, command) => {assert(); await sendTransport(controls, id, command); assert();},
      stop: async (id, message) => {assert(true); await sessions.terminate(id, message); assert(true);},
      alert: options.effects.alert ? alert => ownerRead(() => options.effects.alert!(alert)) : undefined,
    },
    dispose: () => {close(); offAuthority?.(); consoleClient.dispose(); listeners.clear();},
  };
}
