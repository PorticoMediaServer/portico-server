import React, {createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import type {ChannelApi} from '@core/channel-guide.ts';
import {legacyChannelSources, legacyGuideSource, type ChannelSource, type GuideChannelRow, type GuideDataSource} from '@core/guide/index.ts';
import {sessionIdentity, useSession} from './session';
import {currentI18n} from './i18n';
import {onServerChanged} from './server-changes';
import {dedupedGuideApi, forgetGuideReads} from './guide-reads';
import {ChannelSourcesStore} from '@core/guide/source-reads.ts';
import {readRailCache, writeRailCache} from './rail-cache';
export {forgetGuideReads} from './guide-reads';

/**
 * Channels (Spec — Channels and Guide §2, §7): the viewer's channel sources for the rail and the
 * Channels screens, a guide data source per entry over today's `/v1/guide` (the §8.1 stopgap), and
 * the per-source view memory.
 *
 * The data comes from the shared `@core/guide` adapter.
 */

export const ALL = 'all';
export const RECORDINGS = 'recordings';
export const LIBRARY = 'library';
export type ChannelView = 'guide' | 'list';
export type ChannelSort = 'number' | 'name';

export function localTimezone(): string {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'; } catch { return 'UTC'; }
}

export type GuideQuery = Readonly<{entry: string; timezone: string; group?: string; favorites?: boolean; includeHidden?: boolean; sort: ChannelSort}>;

/**
 * The guide data source behind a Channels entry: the shared stopgap adapter over `/v1/guide`
 * (`@core/guide` `legacyGuideSource`; its rows carry the native channel and programme for tuning,
 * recording and favorites via `legacyChannel` / `legacyProgramme`).
 */
export function channelGuideSource(api: ChannelApi, serverId: string, q: GuideQuery): GuideDataSource {
  return legacyGuideSource(dedupedGuideApi(api), serverId, {
    kind: q.entry === ALL ? 'all' : q.entry === LIBRARY ? 'library-channel' : 'live-source',
    sourceId: q.entry === ALL || q.entry === LIBRARY ? '' : q.entry,
    timezone: q.timezone, sort: q.sort,
    // Only an explicit filter is sent (the server treats favorites=false as "not favorites").
    ...(q.favorites ? {favorites: true} : {}), ...(q.includeHidden ? {includeHidden: true} : {}), ...(q.group ? {group: q.group} : {}),
  });
}

// ── Sources for the rail and the screens ─────────────────────────

type SourcesState = {sources: readonly ChannelSource[]; loading: boolean; failed: boolean; refresh: () => void};
const SourcesContext = createContext<SourcesState>({sources: [], loading: false, failed: false, refresh: () => {}});
export const useChannelSources = () => useContext(SourcesContext);

/** One read per signed-in viewer (and on demand). Channels is hidden while there are no sources (§2.0). */
const sourceStores = new WeakMap<ChannelApi, Map<string, ChannelSourcesStore>>();
function sourcesStore(api: ChannelApi, who: string, serverId: string): ChannelSourcesStore {
  let stores = sourceStores.get(api);
  if (!stores) sourceStores.set(api, (stores = new Map()));
  const key = `${who}|${serverId}`;
  let store = stores.get(key);
  if (!store) {
    store = new ChannelSourcesStore(api, (current, signal) => legacyChannelSources(dedupedGuideApi(current), serverId, localTimezone(), currentI18n().t('channels.custom.name'), Date.now(), signal), Date.now, readRailCache(who)?.sources ?? []);
    stores.set(key, store);
    while (stores.size > 4) { const oldest = stores.keys().next().value!; stores.get(oldest)?.dispose(); stores.delete(oldest); }
  }
  return store;
}
const emptySources = Object.freeze({sources: [] as readonly ChannelSource[], loading: false, failed: false});
const noSourcesSubscribe = () => () => {};
const getEmptySources = () => emptySources;

export function ChannelSourcesProvider({children}: {children: React.ReactNode}) {
  const {api, session} = useSession();
  const who = sessionIdentity(session);
  const serverId = session?.viewer.serverId ?? '';
  const store = useMemo(() => who && serverId ? sourcesStore(api, who, serverId) : null, [api, who, serverId]);
  const state = useSyncExternalStore(store?.subscribe ?? noSourcesSubscribe, store?.get ?? getEmptySources);
  useEffect(() => { if (who && !state.loading && !state.failed) writeRailCache(who, {sources: state.sources}); }, [who, state]);
  const refresh = useCallback(() => store?.load(), [store]);
  useEffect(() => onServerChanged(() => { forgetGuideReads(); refresh(); }), [refresh]);
  const value = useMemo(() => ({...state, refresh}), [state, refresh]);
  return <SourcesContext.Provider value={value}>{children}</SourcesContext.Provider>;
}

/** The rail and phone picker entries (§2.0): All channels with 2+ sources, each source, Recordings when a live source records. */
/** Glyphs: live sources a neutral channels glyph (never the source type); Library Channels its own mark (placeholder `library` until `libraryChannel` is drawn). */
export type ChannelEntry = {id: string; name: string; icon: 'channels' | 'library' | 'grid' | 'dvr'; library: boolean};
export function channelEntries(sources: readonly ChannelSource[]): ChannelEntry[] {
  const t = currentI18n().t;
  if (!sources.length) return [];
  const out: ChannelEntry[] = [];
  if (sources.length >= 2) out.push({id: ALL, name: t('channels.all'), icon: 'grid', library: false});
  for (const s of sources) out.push({id: s.id, name: s.type === 'library' ? t('channels.custom.name') : s.name, icon: s.type === 'library' ? 'library' : 'channels', library: s.type === 'library'});
  if (sources.some(s => s.recordAvailable)) out.push({id: RECORDINGS, name: t('channels.recordings'), icon: 'dvr', library: false});
  return out;
}
/** Channels opens on All channels when there are 2+ sources, else the only source (Justin, 23 Sep). */
export const defaultEntry = (sources: readonly ChannelSource[]) => (sources.length >= 2 ? ALL : sources[0]?.id ?? ALL);

// ── Per-source memory (device-local until the preference registry has `channels.view.*`) ──

const MEMORY_KEY = 'portico.channels.v1';
type Memory = {views: Record<string, ChannelView>; sorts: Record<string, ChannelSort>};
function readMemory(viewer: string): Memory {
  try {
    const all = JSON.parse(localStorage.getItem(MEMORY_KEY) ?? '{}') as Record<string, Memory>;
    const m = all[viewer];
    return {views: m?.views ?? {}, sorts: m?.sorts ?? {}};
  } catch { return {views: {}, sorts: {}}; }
}
function writeMemory(viewer: string, m: Memory) {
  try {
    const all = JSON.parse(localStorage.getItem(MEMORY_KEY) ?? '{}') as Record<string, Memory>;
    all[viewer] = {views: Object.fromEntries(Object.entries(m.views).slice(-50)), sorts: Object.fromEntries(Object.entries(m.sorts).slice(-50))};
    const keys = Object.keys(all);
    for (const k of keys.slice(0, Math.max(0, keys.length - 10))) delete all[k];
    localStorage.setItem(MEMORY_KEY, JSON.stringify(all));
  } catch {}
}
export function useChannelMemory(entry: string) {
  const {session} = useSession();
  const viewer = sessionIdentity(session).split('/').slice(0, 3).join('/');
  const [memory, setMemory] = useState(() => readMemory(viewer));
  useEffect(() => setMemory(readMemory(viewer)), [viewer]);
  const update = useCallback((patch: (m: Memory) => Memory) => setMemory(prev => { const next = patch(prev); writeMemory(viewer, next); return next; }), [viewer]);
  return {
    view: memory.views[entry] as ChannelView | undefined,
    sort: memory.sorts[entry] ?? 'number',
    setView: (v: ChannelView) => update(m => ({...m, views: {...m.views, [entry]: v}})),
    setSort: (s: ChannelSort) => update(m => ({...m, sorts: {...m.sorts, [entry]: s}})),
  };
}

// ── Reminders (§3.5 "Remind me": an in-app toast while a Portico tab is open) ──

export {readReminders, hasReminder, toggleReminder, removeReminder, subscribeReminders} from './channel-reminders';
export type {Reminder} from './channel-reminders';

/** The owner's name for a source (Library Channels by its one catalogue name), for All channels row labels. */
export function sourceName(sources: readonly ChannelSource[], id: string | undefined): string {
  if (id === LIBRARY) return currentI18n().t('channels.custom.name');
  return sources.find(s => s.id === id)?.name ?? '';
}

/**
 * FEAT-07: channel up/down in the player. Watching from the guide records where the channel
 * sits; the player steps to the next watchable channel above or below, among the rows the guide
 * has loaded (it never loads the whole lineup).
 */
type ChannelSurf = {row: number; channelAt: (row: number) => GuideChannelRow | undefined};
let surf: ChannelSurf | undefined;
export function setChannelSurf(next: ChannelSurf | undefined) { surf = next; }
export function canSurfChannels(): boolean { return !!surf; }
/** The rows around the watched channel (the nearest watchable one each side), without moving. */
export function surfNeighbours(): {previous?: GuideChannelRow; current?: GuideChannelRow; next?: GuideChannelRow} | undefined {
  const at = surf;
  if (!at) return undefined;
  const find = (delta: 1 | -1) => {
    for (let row = at.row + delta, steps = 0; row >= 0 && steps < 50; row += delta, steps++) {
      const channel = at.channelAt(row);
      if (!channel) return undefined;
      if (channel.tuneAvailable) return channel;
    }
    return undefined;
  };
  return {previous: find(-1), current: at.channelAt(at.row), next: find(1)};
}
export function surfChannel(delta: 1 | -1): GuideChannelRow | undefined {
  if (!surf) return undefined;
  for (let row = surf.row + delta, steps = 0; row >= 0 && steps < 50; row += delta, steps++) {
    const channel = surf.channelAt(row);
    if (!channel) return undefined;
    if (channel.tuneAvailable) { surf = {...surf, row}; return channel; }
  }
  return undefined;
}

/**
 * FEAT-01 (kept from F-web2's Live TV screen): viewer language for why one channel can't be
 * watched or recorded, never the core's engineering sentence. Reasons that belong to the whole
 * server (platform, video tools, delivery) are explained once by `ServerWideNotice`.
 */
export type ServerWide = 'platform' | 'tools' | 'delivery';
export function serverWideReason(reason: string | undefined): ServerWide | null {
  switch (reason) {
    case 'decoder_confinement_unavailable': return 'platform';
    case 'ffmpeg_not_configured': case 'ffprobe_not_configured': case 'decoder_dependencies_unavailable': return 'tools';
    case 'delivery-unavailable': case 'delivery_unavailable': case 'channel_runtime_unavailable': return 'delivery';
    default: return null;
  }
}
/** The server-wide reason when every loaded channel carries one (O(loaded rows)), else null. */
export function serverWideOf(channels: readonly GuideChannelRow[]): ServerWide | null {
  if (!channels.length || channels.some(c => c.tuneAvailable)) return null;
  const reasons = new Set(channels.map(c => serverWideReason(c.tuneUnavailableReason)));
  if (reasons.has(null)) return null;
  return reasons.has('platform') ? 'platform' : reasons.has('tools') ? 'tools' : 'delivery';
}
export function channelReason(reason: string | undefined): string {
  const t = currentI18n().t;
  switch (reason ?? '') {
    case '': return '';
    case 'source-disabled': return t('web.live.reason.sourceOff');
    case 'source-unavailable': return t('web.live.reason.sourceDown');
    case 'capacity-unavailable': return t('web.live.reason.tunersBusy');
    case 'permission-denied': return t('web.live.reason.permission');
    case 'schedule_preparing': return t('web.live.reason.preparing');
    case 'no-schedule': return t('web.live.reason.noSchedule');
    case 'recording-unavailable': return t('web.live.reason.cantRecord');
    case 'not-recordable': return t('web.live.reason.notRecordable');
    default: return t('web.live.reason.cantPlay');
  }
}
