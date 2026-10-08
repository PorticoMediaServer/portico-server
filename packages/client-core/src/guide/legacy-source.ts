/**
 * The stopgap `GuideDataSource` over today's `GET /v1/guide` (Spec — Channels and Guide §8.1,
 * decided 22 Sep: build the screens against it for guides of 100 channels or fewer). Web and Apple
 * share this one adapter.
 *
 * Today's cursors are signed per time window and kind, and there is no total, so:
 * - `channels(from, limit)` walks every cursor page of one window once and serves offsets from that list;
 * - `programs(ids, start, end)` walks every page of that window once (shared by all channel pages
 *   asking for the same block) and picks the requested channels.
 *
 * Each row and program carries its `/v1/guide` object as an opaque `native` handle, because tuning,
 * recording and favorites still take those types: read it with `legacyChannel` / `legacyProgramme`.
 *
 * Programme facts (subtitle, episode, categories, rating, year, starRating, flags, image) are
 * mapped behind presence: older servers send none of them, and malformed values were already
 * dropped by `parseChannelGuide`, never fatal.
 */
import {parseChannelGuide, type ChannelApi, type ChannelGuide, type GuideChannel, type GuideProgramme, type GuideRoute} from '../channel-guide.ts';
import {HOUR_MS, floorTo} from './time.ts';
import type {ChannelSource, GuideChannelRow, GuideDataSource, GuideProgram} from './types.ts';

export type LegacyGuideOptions = Readonly<{
  /** `all`: live sources, then Library Channels (All channels, §2.0). */
  kind: 'live-source' | 'library-channel' | 'all';
  /** '' for every source of the kind. */
  sourceId: string;
  timezone: string;
  group?: string;
  favorites?: boolean;
  /** Owners: include hidden channels (to unhide them). */
  includeHidden?: boolean;
  /**
   * `server` keeps the server's order. `number`: sources in the owner's order, each by channel
   * number; `name` interleaves sources by name (§2.2).
   */
  sort?: 'server' | 'number' | 'name';
  /** Above this, the adapter still works but warns: it walks every page per block. */
  maxChannels?: number;
  now?: () => number;
  warn?: (message: string) => void;
}>;

const iso = (ms: number) => new Date(ms).toISOString().replace('.000Z', 'Z');

type GuideSelection = Readonly<{channels?: readonly string[]; rowsOnly?: boolean}>;

function query(route: GuideRoute, cursor: string, select: GuideSelection = {}): string {
  const q = new URLSearchParams({kind: route.kind, start: route.start, end: route.end, timezone: route.timezone, search: route.search, sourceId: route.sourceId, limit: select.channels ? '50' : '30', cursor});
  if (select.channels) q.set('channels', select.channels.join(','));
  if (select.rowsOnly) q.set('programmes', 'none');
  if (route.favorites !== undefined) q.set('favorites', String(route.favorites));
  if (route.includeHidden) q.set('includeHidden', 'true');
  if (route.group) q.set('group', route.group);
  return '/v1/guide?' + q.toString();
}

/** Every page of one window, in order. */
async function walk(api: ChannelApi, serverId: string, route: GuideRoute, signal?: AbortSignal, select: GuideSelection = {}): Promise<ChannelGuide[]> {
  const pages: ChannelGuide[] = [];
  let cursor = '';
  for (let i = 0; i < 200; i++) {
    const page = parseChannelGuide(await api.request<unknown>(query(route, cursor, select), 'GET', undefined, signal), serverId, route);
    pages.push(page);
    if (!page.nextCursor) break;
    cursor = page.nextCursor;
  }
  return pages;
}

// The handles this adapter attached, so the accessors never trust an object they didn't make.
const nativeChannels = new WeakSet<object>();
const nativeProgrammes = new WeakSet<object>();

/** The `/v1/guide` channel behind a row this adapter made (for tuning, recording and favorites). */
export function legacyChannel(row: GuideChannelRow): GuideChannel | undefined {
  const n = row.native;
  return n !== null && typeof n === 'object' && nativeChannels.has(n) ? n as GuideChannel : undefined;
}

/** The `/v1/guide` programme behind a program this adapter made (for recording). */
export function legacyProgramme(p: GuideProgram): GuideProgramme | undefined {
  const n = p.native;
  return n !== null && typeof n === 'object' && nativeProgrammes.has(n) ? n as GuideProgramme : undefined;
}

export function legacyChannelRow(c: GuideChannel): GuideChannelRow {
  nativeChannels.add(c);
  return {
    id: c.id, sourceId: c.provenance === 'library-channel' ? 'library' : c.sourceId, kind: c.provenance === 'library-channel' ? 'library' : 'live',
    number: c.number, name: c.name, group: c.group, ...(c.logoPath ? {logoUrl: c.logoPath} : {}), favorite: c.favorite,
    tuneAvailable: c.tuneAvailable, ...(c.tuneUnavailableReason ? {tuneUnavailableReason: c.tuneUnavailableReason} : {}),
    recordAvailable: c.recordAvailable, guide: 'full', native: c,
  };
}

export function legacyProgram(p: GuideProgramme): GuideProgram {
  nativeProgrammes.add(p);
  return {
    id: p.id, channelId: p.channelId, title: p.title, start: Date.parse(p.start), end: Date.parse(p.end),
    ...(p.seriesId ? {seriesId: p.seriesId} : {}),
    ...(p.subtitle ? {subtitle: p.subtitle} : {}),
    ...(p.episode ? {episode: p.episode} : {}),
    ...(p.categories !== undefined ? {categories: p.categories} : {}),
    ...(p.rating ? {rating: p.rating.value} : {}),
    ...(p.year !== undefined ? {year: p.year} : {}),
    ...(p.starRating ? {starRating: p.starRating} : {}),
    ...(p.image ? {image: p.image} : {}),
    flags: {live: p.flags?.live === true, new: p.flags?.new === true || p.newEvidence === 'new', premiere: p.flags?.premiere === true, repeat: p.flags?.repeat === true || p.newEvidence === 'repeat'},
    ...(p.recordingId ? {recording: {id: p.recordingId, state: p.recordingState ?? ''}} : {}),
    ...(p.description ? {description: p.description} : {}),
    native: p,
  };
}

const byNumber = (a: GuideChannelRow, b: GuideChannelRow) => {
  const na = parseFloat(a.number), nb = parseFloat(b.number);
  if (Number.isFinite(na) && Number.isFinite(nb) && na !== nb) return na - nb;
  if (Number.isFinite(na) !== Number.isFinite(nb)) return Number.isFinite(na) ? -1 : 1;
  return a.name.localeCompare(b.name);
};

/** Rows in the requested order, each channel once. */
export function orderLegacyRows(rows: readonly GuideChannelRow[], sort: 'server' | 'number' | 'name'): GuideChannelRow[] {
  const seen = new Set<string>();
  const unique = rows.filter(r => !seen.has(r.id) && !!seen.add(r.id));
  if (sort === 'server') return unique;
  if (sort === 'name') return unique.sort((a, b) => a.name.localeCompare(b.name) || byNumber(a, b));
  const order = new Map<string, number>();
  unique.forEach(r => { if (!order.has(r.sourceId ?? '')) order.set(r.sourceId ?? '', order.size); });
  return unique.sort((a, b) => (order.get(a.sourceId ?? '')! - order.get(b.sourceId ?? '')!) || byNumber(a, b));
}

export function legacyGuideSource(api: ChannelApi, serverId: string, o: LegacyGuideOptions): GuideDataSource {
  const now = o.now ?? Date.now;
  const kinds = o.kind === 'all' ? (['live-source', 'library-channel'] as const) : [o.kind];
  const route = (kind: 'live-source' | 'library-channel', start: number, end: number): GuideRoute => ({
    kind, start: iso(start), end: iso(end), timezone: o.timezone, search: '', sourceId: o.sourceId,
    ...(o.favorites !== undefined ? {favorites: o.favorites} : {}), ...(o.includeHidden ? {includeHidden: true} : {}), ...(o.group ? {group: o.group} : {}),
  });
  let list: Promise<GuideChannelRow[]> | undefined;
  const windows = new Map<string, Promise<ChannelGuide[]>>();
  // Library Channels are few and owner-made: their programmes come from one walk per window.
  const libraryKinds = kinds.filter(k => k === 'library-channel');
  const window = (start: number, end: number) => {
    const key = `${start}:${end}`;
    let pages = windows.get(key);
    if (!pages) {
      // One walk per window for every channel page that asks; not tied to one caller's abort.
      pages = Promise.allSettled(libraryKinds.map(k => walk(api, serverId, route(k, start, end)))).then(results => {
        if (results.length && results.every(r => r.status === 'rejected')) throw (results[0] as PromiseRejectedResult).reason;
        return results.flatMap(r => (r.status === 'fulfilled' ? r.value : []));
      });
      windows.set(key, pages);
      pages.catch(() => windows.delete(key));
      while (windows.size > 8) windows.delete(windows.keys().next().value!);
    }
    return pages;
  };
  return {
    async channels(from, limit, signal) {
      if (!list) {
        // The channel list is the same for every time window: live channels are listed once
        // without programmes; in All channels a kind that fails is left out unless every kind fails.
        const start = floorTo(now(), HOUR_MS);
        list = Promise.allSettled(kinds.map(k => walk(api, serverId, route(k, start, start + 3 * HOUR_MS), undefined, k === 'live-source' ? {rowsOnly: true} : {}))).then(results => {
          if (results.every(r => r.status === 'rejected')) throw (results[0] as PromiseRejectedResult).reason;
          return orderLegacyRows(results.flatMap(r => (r.status === 'fulfilled' ? r.value : [])).flatMap(p => p.channels.map(legacyChannelRow)), o.sort ?? 'server');
        });
        list.catch(() => { list = undefined; });
      }
      const all = await list;
      if (signal.aborted) throw new Error('aborted');
      return {items: all.slice(from, from + limit), total: all.length};
    },
    async programs(channelIds, start, end, signal) {
      const out: Record<string, GuideProgram[]> = {};
      for (const id of channelIds) out[id] = [];
      const rows = new Map((list ? await list : []).map(r => [r.id, r]));
      // Live channels on screen are asked for by id, 50 at a time, for this window only.
      const live = channelIds.filter(id => rows.get(id)?.kind !== 'library');
      const library = channelIds.filter(id => rows.get(id)?.kind === 'library');
      const reads: Promise<ChannelGuide[]>[] = [];
      if (kinds.includes('live-source')) for (let i = 0; i < live.length; i += 50) reads.push(walk(api, serverId, route('live-source', start, end), signal, {channels: live.slice(i, i + 50)}));
      if (library.length) reads.push(window(start, end));
      const wanted = new Set(channelIds);
      for (const pages of await Promise.all(reads)) for (const page of pages) for (const c of page.channels) if (wanted.has(c.id)) out[c.id] = c.programmes.map(legacyProgram);
      if (signal.aborted) throw new Error('aborted');
      return out;
    },
  };
}

/** The refresh states in which a source's guide is out of date; any other state is fine. */
const staleRefreshStates: ReadonlySet<string> = new Set(['degraded', 'credentials-required']);

/** A live source's guide state from its `/v1/guide` refresh state. */
export function legacyGuideState(refreshState: string): 'ready' | 'refreshing' | 'stale' {
  return refreshState === 'refreshing' ? 'refreshing' : staleRefreshStates.has(refreshState) ? 'stale' : 'ready';
}

/**
 * The Channels entries from today's API: each live source by name, plus "Library Channels" when
 * the library-channel guide has any channel. `libraryName` is the catalogue's name for it.
 */
export async function legacyChannelSources(api: ChannelApi, serverId: string, timezone: string, libraryName: string, now = Date.now(), signal?: AbortSignal): Promise<ChannelSource[]> {
  const start = floorTo(now, HOUR_MS), end = start + 3 * HOUR_MS;
  const base = {start: iso(start), end: iso(end), timezone, search: '', sourceId: ''};
  const [livePages, library] = await Promise.all([
    walk(api, serverId, {...base, kind: 'live-source'}, signal),
    api.request<unknown>(query({...base, kind: 'library-channel'}, ''), 'GET', undefined, signal).then(raw => parseChannelGuide(raw, serverId, {...base, kind: 'library-channel'}), () => undefined),
  ]);
  const recordable = new Set(livePages.flatMap(p => p.channels).filter(c => c.recordAvailable).map(c => c.sourceId));
  const liveDays = livePages.map(p => p.days).filter((d): d is number => d !== undefined);
  const liveGuideDays = liveDays.length ? Math.max(...liveDays) : undefined;
  const out: ChannelSource[] = (livePages[0]?.sources ?? []).map((s, i) => ({
    id: s.id, name: s.name, type: 'live', position: i, recordAvailable: recordable.has(s.id),
    guideState: legacyGuideState(s.refreshState),
    ...(s.availableStart ? {availableStart: Date.parse(s.availableStart)} : {}), ...(s.availableEnd ? {availableEnd: Date.parse(s.availableEnd)} : {}),
    ...(liveGuideDays !== undefined ? {guideDays: liveGuideDays} : {}),
  }));
  if (library && library.channels.length) out.push({id: 'library', name: libraryName, type: 'library', position: out.length, recordAvailable: false, guideState: 'ready', ...(library.days !== undefined ? {guideDays: library.days} : {})});
  return out;
}

