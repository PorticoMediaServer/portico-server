import {unreadableServerResponse} from './server-messages.ts';
/** Server-owned Saved projections and playlist commands. No ranking, navigation or queue owner. */
import type { ContentEntry, ContentHeading, ContentProjection } from './library-content.ts';
export type SavedScope = Readonly<{ serverId: string; viewerId: string }>;
export interface SavedApi { request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> }
export type SavedListFilter = 'all' | 'unwatched' | 'inProgress';
export type SavedListSort = 'title' | 'added' | 'updated' | 'year' | 'duration' | 'progress';
export type HistoryPeriod = '24h' | '7d' | '30d' | '90d' | 'all';
export const savedListFilters: readonly SavedListFilter[] = ['all', 'unwatched', 'inProgress'];
export const savedListSorts: readonly SavedListSort[] = ['title', 'added', 'updated', 'year', 'duration', 'progress'];
export const historyPeriods: readonly HistoryPeriod[] = ['24h', '7d', '30d', '90d', 'all'];
export type SavedRoute = Readonly<{ view: 'watchlist' | 'favorites' | 'playlists' | 'collections' | 'views' | 'history' | 'resource'; resourceId?:string; sort?: string; direction?: 'asc' | 'desc'; filter?: SavedListFilter; period?: HistoryPeriod } | { view: 'playlist'; playlistId: string }>;
/** Filters and sorts are named server policies. The client picks a published name; it never
 * evaluates the predicate or re-sorts the page it was given. */
export function savedListPath(view: 'watchlist' | 'favorites', options: Readonly<{ filter?: SavedListFilter; sort?: SavedListSort; direction?: 'asc' | 'desc'; limit?: number; cursor?: string | null }> = {}): string {
  const limit = options.limit ?? 40;
  if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('Saved lists page 1 to 100 rows.');
  if (options.filter !== undefined && !savedListFilters.includes(options.filter)) throw new Error('Unknown saved list filter.');
  if (options.sort !== undefined && !savedListSorts.includes(options.sort)) throw new Error('Unknown saved list sort.');
  if (options.direction !== undefined && options.direction !== 'asc' && options.direction !== 'desc') throw new Error('Unknown sort direction.');
  const params = new URLSearchParams({ view, limit: String(limit) });
  if (options.filter) params.set('filter', options.filter);
  if (options.sort) params.set('sort', options.sort);
  if (options.direction) params.set('direction', options.direction);
  if (options.cursor) params.set('cursor', options.cursor);
  return '/v1/content?' + params;
}
export function personalHistoryPath(options: Readonly<{ period?: HistoryPeriod; libraryId?: string; limit?: number; cursor?: string | null }> = {}): string {
  const limit = options.limit ?? 40;
  if (!Number.isInteger(limit) || limit < 1 || limit > 100) throw new Error('History pages 1 to 100 rows.');
  if (options.period !== undefined && !historyPeriods.includes(options.period)) throw new Error('Unknown history period.');
  const params = new URLSearchParams({ limit: String(limit) });
  if (options.period) params.set('period', options.period);
  if (options.libraryId) params.set('libraryId', options.libraryId);
  if (options.cursor) params.set('cursor', options.cursor);
  return '/v1/personal-history?' + params;
}
export type PlaylistOccurrence = Readonly<{ id: string; kind: 'playlist_entry'; hidden: true } | { id: string; kind: 'playlist_entry'; hidden: false; media: ContentEntry }>;
export type PlaylistCard = Readonly<{ id: string; kind: 'playlist'; title: string; subtitle?: string; count?: number; navigation: Readonly<{ view: 'playlist'; entityId: string }> }>;
export type SavedEntry = ContentEntry | PlaylistOccurrence | PlaylistCard;
export type SavedProjection = Readonly<Omit<ContentProjection, 'scope' | 'sections' | 'navigation'> & {
 scope: Readonly<{ serverId: string; libraryId: ''; libraryKind: 'mixed'; view: SavedRoute['view']; entityId: string; viewerFence: string }>;
 playlistRevision?: number;
 actions?: readonly string[];
 navigation: readonly Readonly<{ id: string; labelKey: string; view: 'watchlist' | 'favorites' | 'playlists' }>[];
 sections: readonly Readonly<{ id: string; type: 'rail' | 'grid' | 'list'; heading: ContentHeading; entries: readonly SavedEntry[]; totalCount: number; nextCursor: string }>[];
}>;
export type SavedRestore = Readonly<{ cursor?: string | null; history?: readonly (string | null)[]; scope?: SavedScope }>;
export type SavedActor = Readonly<{ authority: 'local' | 'hosted'; accountId: string; profileId: string }>;
export type PlaylistResource = Readonly<{ serverId: string; viewerFence: string; id: string; name: string; summary: string; revision: number; pinned:boolean;pinRevision:number;role: 'owner' | 'editor' | 'viewer'; entryCount: number; actions: readonly string[]; limits: Readonly<{ maxEntries?: number; maxShares: number; maxReorderEntries: number }>; shares?: readonly Readonly<SavedActor & { role: 'viewer' | 'editor'; displayName: string }>[] }>;
export type SavedReceipt = Readonly<{ serverId: string; viewerFence: string; operationId: string; playlistId: string; revision: number; entryId?: string; deleted: boolean }>;
export type SavedIntent = Readonly<
 | { action: 'create'; name: string; summary?: string;itemIds?:readonly string[] }
 | { action: 'rename'; name?: string; summary?: string }
 | { action: 'add'; itemId: string }
 | { action: 'remove'; entryId: string }
 /** Windowed reorder: 1–200 distinct occurrence ids. Omit
  * `afterEntryId` to permute just the selected occurrences in their existing
  * slots; supply an occurrence id to move them after it, or `""` to prepend.
  * The anchor cannot be among the moved ids. */
 | { action: 'reorder'; entryIds: readonly string[]; afterEntryId?: string }
 | { action: 'delete' }
 | { action:'pin';pinned:boolean }
 | { action: 'share'; actor: SavedActor; role: 'viewer' | 'editor' }
 | { action: 'unshare'; actor: SavedActor }
>;
/** One window of the playlist's entry order. Page with `cursor`/`limit`
 * (1–100, default 100); stop when `nextCursor` is absent. A cursor binds
 * profile, playlist, viewer fence, page size and revision: on
 * `revision_mismatch`, restart paging. */
export type SavedOrder = Readonly<{ serverId: string; viewerFence: string; playlistId: string; revision: number; entryIds: readonly string[]; nextCursor: string }>;
export type SavedCandidates = Readonly<{ serverId: string; viewerFence: string; playlistId: string; revision: number; candidates: readonly Readonly<SavedActor & { id: string; displayName: string }>[]; nextCursor: string }>;
export type SavedError = Readonly<{ code: string; message: string; retryable: boolean }>;
export type SavedSnapshot = Readonly<{ generation: number; scope: SavedScope; route: SavedRoute | null; phase: 'idle' | 'loading' | 'ready' | 'error' | 'refresh-required'; projection: SavedProjection | null; playlist: PlaylistResource | null; error: SavedError | null; pagination: Readonly<{ cursor: string | null; history: readonly (string | null)[]; canPrevious: boolean; next: readonly Readonly<{ sectionId: string; cursor: string }>[] }>; pending: readonly Readonly<{ intent: SavedIntent; phase: 'queued' | 'sending' | 'retry-required' | 'conflict' }>[]; mutationError: SavedError | null; lastReceipt: SavedReceipt | null; candidates: SavedCandidates | null; candidatesLoading: boolean; candidatesError: SavedError | null; order: SavedOrder | null; orderLoading: boolean; orderError: SavedError | null }>;
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const prose = (v: unknown, max: number, multiline = false): v is string => typeof v === 'string' && v.length <= max * 2 && [...v].length <= max && !(multiline ? /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/ : /[\x00-\x1f\x7f]/).test(v);
const id = (v: unknown): v is string => text(v, 256) && v.length > 0;
const int = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;
const number = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v >= 0;
class Failure extends Error { code: string; retryable: boolean; constructor(code: string, message: string, retryable = false) { super(message); this.code = code; this.retryable = retryable; } }
function invalid(): never { throw new Failure('invalid_saved', unreadableServerResponse); }
function list(v: unknown, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) invalid(); return v; }
function unique<T>(rows: T[], key: (row: T) => string): T[] { if (new Set(rows.map(key)).size !== rows.length) invalid(); return rows; }
function freeze<T>(v: T): T { if (v && typeof v === 'object') { for (const child of Object.values(v)) freeze(child); Object.freeze(v); } return v; }
function heading(v: unknown): ContentHeading { if (!record(v) || !id(v.key) || !text(v.fallback)) invalid(); return { key: v.key, fallback: v.fallback }; }
function actor(v: unknown): SavedActor { if (!record(v) || !['local','hosted'].includes(v.authority as string) || !id(v.accountId) || !id(v.profileId)) invalid(); return { authority: v.authority as SavedActor['authority'], accountId: v.accountId, profileId: v.profileId }; }
function scope(v: SavedScope): SavedScope { if (!id(v.serverId) || !text(v.viewerId, 1024) || !v.viewerId) throw new Error('A complete Saved viewer/server scope is required.'); return freeze({ serverId: v.serverId, viewerId: v.viewerId }); }
function route(v: SavedRoute): SavedRoute {
 if (!record(v) || !['watchlist','favorites','playlists','playlist','collections','views','history','resource'].includes(v.view)) throw new Error('Invalid Saved route.');
 if (v.view === 'playlist') { if (!id(v.playlistId)) throw new Error('Playlist ID required.'); return freeze({ view: v.view, playlistId: v.playlistId }); }
 if(v.view==='resource'&&!id(v.resourceId))throw new Error('Saved resource ID required.');
 if (v.sort !== undefined && !id(v.sort) || v.direction !== undefined && !['asc','desc'].includes(v.direction)) throw new Error('Invalid Saved query.');
 // CD-17: retain and validate filter when normalizing watchlist/favorites routes.
 // Only accept all/unwatched/inProgress for that view; reject filter for other views.
 if (v.filter !== undefined) {
  if (v.view !== 'watchlist' && v.view !== 'favorites') throw new Error('Invalid Saved query.');
  if (!savedListFilters.includes(v.filter)) throw new Error('Unknown saved list filter.');
 }
 if (v.period !== undefined && !historyPeriods.includes(v.period)) throw new Error('Unknown history period.');
 if (v.period !== undefined && v.view !== 'history') throw new Error('Invalid Saved query.');
 return freeze({ view: v.view,...(v.resourceId?{resourceId:v.resourceId}:{}), ...(v.sort === undefined ? {} : { sort: v.sort }), ...(v.direction === undefined ? {} : { direction: v.direction }), ...(v.filter === undefined ? {} : { filter: v.filter }), ...(v.period === undefined ? {} : { period: v.period }) });
}
function media(v: unknown): ContentEntry {
 if (!record(v) || !id(v.id) || !id(v.libraryId) || !['movie','show','season','episode','artist','album','song','book','audiobook_file','chapter','collection','category','extra','author','book_series','disc'].includes(v.kind as string) || !text(v.title, 2048)) invalid();
 const out: Record<string, unknown> = { id: v.id, libraryId: v.libraryId, kind: v.kind, title: v.title };
 for (const key of ['subtitle','posterUrl','backdropUrl','overview']) if (v[key] !== undefined) { if (key === 'overview' ? !prose(v[key], 16384, true) : !text(v[key], 4096)) invalid(); out[key] = v[key]; }
 for (const key of ['duration','progressSeconds']) if (v[key] !== undefined) { if (!number(v[key])) invalid(); out[key] = v[key]; }
 if (v.available !== undefined) { if (typeof v.available !== 'boolean') invalid(); out.available = v.available; }
 if (v.count !== undefined) { if (!int(v.count)) invalid(); out.count = v.count; }
 if (v.addedAt !== undefined) { if (v.addedAt !== null && !text(v.addedAt, 128)) invalid(); out.addedAt = v.addedAt; }
 if (v.navigation !== undefined) { const n = v.navigation; if (!record(n) || !['item','show','season','artist','album','book','collection','browse','categories','author','book_series','disc'].includes(n.view as string) || n.entityId !== undefined && !id(n.entityId) || n.category !== undefined && !text(n.category, 256)) invalid(); out.navigation = { view: n.view, ...(n.entityId === undefined ? {} : { entityId: n.entityId }), ...(n.category === undefined ? {} : { category: n.category }) }; }
 if (v.playback !== undefined) { const p = v.playback; if (!record(p) || !id(p.itemId) || p.startSeconds !== undefined && !number(p.startSeconds) || v.available === false) invalid(); out.playback = { itemId: p.itemId, ...(p.startSeconds === undefined ? {} : { startSeconds: p.startSeconds }) }; }
 return out as ContentEntry;
}
function entry(v: unknown, view: SavedRoute['view']): SavedEntry {
 if (!record(v)) invalid();
 if (view === 'playlist') {
  if (!id(v.id) || v.kind !== 'playlist_entry' || typeof v.hidden !== 'boolean' || Object.keys(v).some(k => !['id','kind','hidden','media'].includes(k))) invalid();
  if (v.hidden) { if (v.media !== undefined) invalid(); return { id: v.id, kind: 'playlist_entry', hidden: true }; }
  return { id: v.id, kind: 'playlist_entry', hidden: false, media: media(v.media) };
 }
 if (view === 'playlists') {
  if (!id(v.id) || v.kind !== 'playlist' || !prose(v.title, 200) || !record(v.navigation) || v.navigation.view !== 'playlist' || v.navigation.entityId !== v.id || v.playback !== undefined) invalid();
  if (v.subtitle !== undefined && !prose(v.subtitle, 4000, true) || v.count !== undefined && !int(v.count)) invalid();
  return { id: v.id, kind: 'playlist', title: v.title, navigation: { view: 'playlist', entityId: v.id }, ...(v.subtitle === undefined ? {} : { subtitle: v.subtitle as string }), ...(v.count === undefined ? {} : { count: v.count as number }) };
 }
 return media(v);
}
function projection(v: unknown, bound: SavedScope, selected: SavedRoute): SavedProjection {
 if (!record(v) || !record(v.scope) || !record(v.revision) || !record(v.query)) invalid();
 const s = v.scope, r = v.revision, q = v.query;
 // Watchlist and Favourites echo their list filter as the query category and publish the filter's counts.
 const listView = selected.view === 'watchlist' || selected.view === 'favorites';
 if (s.serverId !== bound.serverId || s.libraryId !== '' || s.libraryKind !== 'mixed' || s.view !== selected.view || s.entityId !== (selected.view === 'playlist' ? selected.playlistId : '') || !id(s.viewerFence) || !int(r.catalog) || !int(r.viewer)) invalid();
 if (!id(q.sort) || !['asc','desc'].includes(q.direction as string) || q.q !== '' || q.limit !== 40 || q.searchMode !== 'none') invalid();
 // CD-17: preserve validated category; verify the echo matches the selected filter (default all).
 if (listView) {
  if (!savedListFilters.includes(q.category as SavedListFilter)) invalid();
  const expected = (selected as { filter?: SavedListFilter }).filter ?? 'all';
  if (q.category !== expected) invalid();
 } else if (q.category !== '') invalid();
 if (selected.view === 'playlist' ? q.sort !== 'position' || q.direction !== 'asc' || !int(v.playlistRevision) : selected.sort !== undefined && q.sort !== selected.sort || selected.direction !== undefined && q.direction !== selected.direction) invalid();
 const sections = unique(list(v.sections, 4).map(row => { if (!record(row) || !id(row.id) || !['rail','grid','list'].includes(row.type as string) || !int(row.totalCount) || !text(row.nextCursor, 4096)) invalid(); const entries = unique(list(row.entries, row.type === 'rail' ? 12 : 100).map(value => entry(value, selected.view)), value => value.id); if (row.totalCount < entries.length) invalid(); return { id: row.id, type: row.type as 'rail'|'grid'|'list', heading: heading(row.heading), entries, totalCount: row.totalCount, nextCursor: row.nextCursor }; }), row => row.id);
 const navigation = unique(list(v.navigation, 3).map(n => { if (!record(n) || !id(n.id) || !id(n.labelKey) || !['watchlist','favorites','playlists'].includes(n.view as string)) invalid(); return { id: n.id, labelKey: n.labelKey, view: n.view as 'watchlist'|'favorites'|'playlists' }; }), n => n.id);
 const sorts = unique(list(v.sorts, 16).map(row => { if (!record(row) || !id(row.id) || !id(row.labelKey)) invalid(); const directions = unique(list(row.directions, 2).map(d => { if (d !== 'asc' && d !== 'desc') invalid(); return d; }), d => d); return { id: row.id, labelKey: row.labelKey, directions }; }), row => row.id);
 // CD-17: preserve validated filter definitions/counts; do not erase server-authored filter state.
 const filters = unique(list(v.filters, listView ? 4 : 0).map(f => { if (!record(f) || !id(f.id) || !id(f.labelKey)) invalid(); const options = unique(list(f.options, 8).map(o => { if (!record(o) || !id(o.id) || !prose(o.label, 120) || !int(o.count)) invalid(); return Object.freeze({ id: o.id as string, label: o.label as string, count: o.count as number }); }), k => (k as { id: string }).id); return Object.freeze({ id: f.id as string, labelKey: f.labelKey as string, options: Object.freeze(options) }); }), k => (k as { id: string }).id);
 const actions = v.actions === undefined ? undefined : unique(list(v.actions, 8).map(a => { if (selected.view !== 'playlists' || a !== 'create_playlist') invalid(); return a; }), a => a);
 if (selected.view === 'playlists' && actions === undefined) invalid();
 return freeze({ scope: { serverId: bound.serverId, libraryId: '', libraryKind: 'mixed', view: selected.view, entityId: s.entityId as string, viewerFence: s.viewerFence }, revision: { catalog: r.catalog, viewer: r.viewer }, heading: heading(v.heading), query: { sort: q.sort, direction: q.direction as string, category: q.category as string, q: '', limit: 40, searchMode: 'none' }, navigation, sorts, filters, sections, ...(actions === undefined ? {} : { actions }), ...(selected.view === 'playlist' ? { playlistRevision: v.playlistRevision as number } : {}), ...(v.empty === undefined ? {} : { empty: heading(v.empty) }) });
}
function resource(v: unknown, bound: SavedScope, playlistId: string): PlaylistResource {
 if (!record(v) || v.serverId !== bound.serverId || !id(v.viewerFence) || v.id !== playlistId || !prose(v.name, 200) || !prose(v.summary, 4000, true) || !int(v.revision) || !['owner','editor','viewer'].includes(v.role as string) || !int(v.entryCount) || !record(v.limits)) invalid();
 // Stored playlists have no entry cap (`limits.maxEntries` is
 // omitted); synchronous writes still bound each request (`maxReorderEntries` 200).
 const limits = v.limits;
 if (limits.maxEntries !== undefined && (!int(limits.maxEntries))) invalid();
 for (const k of ['maxShares','maxReorderEntries']) if (!int(limits[k]) || (limits[k] as number) > (k === 'maxShares' ? 100 : 1000)) invalid();
 const actions = unique(list(v.actions, 16).map(a => { if (!['rename','summary','delete','share','add','remove','reorder','pin'].includes(a as string)) invalid(); return a as string; }), a => a);
 if (v.role === 'viewer' && actions.some(a=>a!=='pin') || v.role === 'editor' && actions.some(a => !['add','remove','reorder','pin'].includes(a))) invalid();
 const shares = v.shares === undefined ? undefined : unique(list(v.shares, 100).map(row => { if (!record(row) || !['viewer','editor'].includes(row.role as string) || !text(row.displayName)) invalid(); return { ...actor(row), role: row.role as 'viewer'|'editor', displayName: row.displayName }; }), row => JSON.stringify([row.authority,row.accountId,row.profileId]));
 if (shares !== undefined && v.role !== 'owner' || limits.maxEntries !== undefined && v.entryCount > (limits.maxEntries as number)) invalid();
 if(v.pinned!==undefined&&typeof v.pinned!=='boolean'||v.pinRevision!==undefined&&!int(v.pinRevision))invalid();
 return freeze({ pinned:v.pinned===true,pinRevision:(v.pinRevision as number)??0,serverId: bound.serverId, viewerFence: v.viewerFence, id: playlistId, name: v.name, summary: v.summary, revision: v.revision, role: v.role as PlaylistResource['role'], entryCount: v.entryCount, actions, limits: { ...(limits.maxEntries === undefined ? {} : {maxEntries: limits.maxEntries as number}), maxShares: limits.maxShares as number, maxReorderEntries: limits.maxReorderEntries as number }, ...(shares === undefined ? {} : { shares }) });
}
function errorInfo(error: unknown): SavedError { const e = error as { code?: unknown; retryable?: unknown }; return freeze({ code: id(e?.code) ? e.code : 'request_failed', message: error instanceof Error ? error.message : 'Saved request failed.', retryable: e?.retryable === true || !id(e?.code) }); }
function failedError(e: SavedError): Error { return Object.assign(new Error(e.message), e); }
type Command = { intent: SavedIntent; phase: 'queued'|'sending'|'retry-required'|'conflict'; operationId?: string; orderRevision?: number; body?: Record<string, unknown>; promise: Promise<void>; resolve: () => void; reject: (error: unknown) => void };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
function intent(v: SavedIntent): SavedIntent {
 if (!record(v)) throw new Error('Invalid playlist command.');
 switch (v.action) {
  case 'create': if(v.itemIds!==undefined&&(!Array.isArray(v.itemIds)||v.itemIds.length>200||!v.itemIds.every(id)))break; if (!prose(v.name, 200) || !v.name.trim() || v.summary !== undefined && !prose(v.summary, 4000, true)) break; return freeze({ action: v.action, name: v.name,...(v.itemIds?{itemIds:[...v.itemIds]}:{}), ...(v.summary === undefined ? {} : { summary: v.summary }) });
  case 'rename': if (v.name === undefined && v.summary === undefined || v.name !== undefined && (!prose(v.name, 200) || !v.name.trim()) || v.summary !== undefined && !prose(v.summary, 4000, true)) break; return freeze({ action: v.action, ...(v.name === undefined ? {} : { name: v.name }), ...(v.summary === undefined ? {} : { summary: v.summary }) });
  case 'add': if (id(v.itemId)) return freeze({ action: v.action, itemId: v.itemId }); break;
  case 'remove': if (id(v.entryId)) return freeze({ action: v.action, entryId: v.entryId }); break;
  // Windowed reorder: 1–200 distinct occurrence ids; `afterEntryId` may be an
  // occurrence id, `""` (prepend) or omitted (in-place permute). The anchor
  // cannot be among the moved ids.
  case 'reorder': if (Array.isArray(v.entryIds) && v.entryIds.length >= 1 && v.entryIds.length <= 200 && v.entryIds.every(id) && new Set(v.entryIds).size === v.entryIds.length && (v.afterEntryId === undefined || typeof v.afterEntryId === 'string' && v.afterEntryId.length <= 256 && !/[\x00-\x1f\x7f]/.test(v.afterEntryId)) && (v.afterEntryId === undefined || v.afterEntryId === '' || !v.entryIds.includes(v.afterEntryId))) return freeze({ action: v.action, entryIds: [...v.entryIds], ...(v.afterEntryId === undefined ? {} : {afterEntryId: v.afterEntryId}) }); break;
  case 'delete': return freeze({ action: v.action });
  case 'pin':if(typeof v.pinned==='boolean')return freeze({action:'pin',pinned:v.pinned});break;
  case 'share': if (v.role === 'editor' || v.role === 'viewer') return freeze({ action: v.action, actor: actor(v.actor), role: v.role }); break;
  case 'unshare': return freeze({ action: v.action, actor: actor(v.actor) });
 }
 throw new Error('Invalid playlist command.');
}
export class SavedService {
 private api: SavedApi; private bound: SavedScope; private requestId: () => Promise<string>; private timeoutMs: number;
 private generation = 0; private readGeneration = 0; private candidateGeneration = 0; private orderGeneration = 0; private controllers = new Set<AbortController>();
 private readController?: AbortController; private candidateController?: AbortController; private orderController?: AbortController; private disposed = false; private running = false;
  private commands: Command[] = []; private listeners = new Set<() => void>(); private state: SavedSnapshot; private history: (string|null)[] = []; private readPromise?: Promise<void>; private requestKey = ''; private retryCursor: string|null = null; private retryMove?: 'next'|'previous';
  /** Q7: the last failed command, kept for explicit retryMutation with the same
   * idempotency key. Failed commands are removed from `commands` so later
   * commands proceed; this slot preserves the operation for a deliberate retry. */
  private lastFailure?: {intent: SavedIntent; operationId?: string; body?: Record<string, unknown>; orderRevision?: number; kind: 'retry-required'|'conflict'};
 constructor(options: { api: SavedApi; scope: SavedScope; requestId: () => Promise<string>; timeoutMs?: number }) {
  this.api = options.api; this.bound = scope(options.scope); this.requestId = options.requestId; this.timeoutMs = options.timeoutMs ?? 15000;
  if (!number(this.timeoutMs) || this.timeoutMs < 1 || this.timeoutMs > 120000) throw new Error('Invalid Saved deadline.');
  this.state = this.empty();
 }
 private empty(): SavedSnapshot { return freeze({ generation: this.generation, scope: this.bound, route: null, phase: 'idle', projection: null, playlist: null, error: null, pagination: { cursor: null, history: [], canPrevious: false, next: [] }, pending: [], mutationError: null, lastReceipt: null, candidates: null, candidatesLoading: false, candidatesError: null, order: null, orderLoading: false, orderError: null }); }
 getSnapshot = (): SavedSnapshot => this.state;
 subscribe = (listener: () => void): (() => void) => { if (this.disposed) return () => {}; this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
 private publish(patch: Partial<SavedSnapshot> = {}): void { if (this.disposed) return; this.state = freeze({ ...this.state, ...patch, pending: this.commands.map(c => ({ intent: c.intent, phase: c.phase })) }); for (const listener of this.listeners) listener(); }
  private active(): void { if (this.disposed) throw new Error('Saved service is disposed.'); }
  private fence(): void { this.generation++; this.readGeneration++; this.candidateGeneration++; this.orderGeneration++; for (const c of this.controllers) c.abort(); for (const c of this.commands) c.resolve(); this.commands = []; this.lastFailure = undefined; this.history = []; this.running = false; this.readPromise = undefined; this.requestKey = ''; }
 select(input: SavedRoute, restore: SavedRestore = {}): Promise<void> {
  this.active(); const selected = route(input), cursor = restore.cursor ?? null, history = restore.history ?? [];
  if (!Array.isArray(history) || history.length > 64 || history.some(value => value !== null && (!text(value, 4096) || !value)) || cursor !== null && (!text(cursor, 4096) || !cursor)) throw new Error('Invalid bounded Saved cursor restoration.');
  if ((cursor !== null || history.length > 0) && (!restore.scope || restore.scope.serverId !== this.bound.serverId || restore.scope.viewerId !== this.bound.viewerId)) throw new Error('Saved cursor restoration requires the matching viewer/server scope.');
  if (cursor === null && history.length > 0) throw new Error('First-page restoration cannot have previous cursors.');
  if (this.state.phase === 'loading' && JSON.stringify(selected) === JSON.stringify(this.state.route) && cursor === this.retryCursor && JSON.stringify(history) === JSON.stringify(this.history) && this.readPromise) return this.readPromise;
  this.fence(); this.history = [...history]; this.publish({ ...this.empty(), route: selected, pagination: { cursor, history: [...history], canPrevious: history.length > 0, next: [] } }); return this.read(cursor);
 }
 refresh(): Promise<void> { this.active(); this.history = []; return this.read(null); }
 retry(): Promise<void> { this.active(); return this.state.phase === 'refresh-required' ? this.refresh() : this.read(this.retryCursor, this.retryMove); }
 next(sectionId: string): Promise<void> { this.active(); if (this.state.phase !== 'ready') return Promise.resolve(); const next = this.state.pagination.next.find(n => n.sectionId === sectionId); if (!next) return Promise.resolve(); return this.read(next.cursor, 'next'); }
 previous(): Promise<void> { this.active(); if (this.state.phase !== 'ready' || !this.history.length) return Promise.resolve(); return this.read(this.history.at(-1)!, 'previous'); }
 /** Position for the connected collection/view/history adapter, owned by existing navigation frames. */
 externalPosition(cursor:string|null,history:readonly (string|null)[]):void {
  this.active();if(!this.state.route||!['collections','views','history','resource'].includes(this.state.route.view))return;
  if(cursor!==null&&!text(cursor,4096)||history.length>64||history.some(c=>c!==null&&!text(c,4096)))throw new Error('Invalid Saved position.');
  if(cursor===this.state.pagination.cursor&&JSON.stringify(history)===JSON.stringify(this.state.pagination.history))return;
  this.publish({pagination:{cursor,history:[...history],canPrevious:history.length>0,next:[]}});
 }
 setScope(next: SavedScope, api: SavedApi): void { this.active(); const checked = scope(next); this.fence(); this.api = api; this.bound = checked; this.publish(this.empty()); }
 cancel(): void { this.active(); this.fence(); this.publish(this.empty()); }
 dispose(): void { this.fence(); this.disposed = true; this.listeners.clear(); }
 private read(cursor: string|null, move?: 'next'|'previous'): Promise<void> {
  const selected = this.state.route; if (!selected) return Promise.resolve();
  if(['collections','views','history','resource'].includes(selected.view)){this.publish({phase:'ready'});return Promise.resolve();}
  const key = JSON.stringify([selected,cursor]); if (this.state.phase === 'loading' && key === this.requestKey && this.readPromise) return this.readPromise;
  this.orderController?.abort(); this.orderGeneration++; this.publish({ order: null, orderLoading: false, orderError: null });
  this.readController?.abort(); const controller = new AbortController(); this.readController = controller;
  const generation = this.generation, read = ++this.readGeneration, api = this.api, bound = this.bound, previous = this.state.projection, oldCursor = this.state.pagination.cursor;
  this.requestKey = key; this.retryCursor = cursor; this.retryMove = move; this.publish({ phase: 'loading', error: null });
  const task = (async () => {
   try {
    const params = new URLSearchParams({ limit: '40' }); if (cursor) params.set('cursor', cursor);
    let path: string;
    if (selected.view === 'playlist') path = '/v1/playlists/' + encodeURIComponent(selected.playlistId) + '/content';
    else { path = '/v1/content'; params.set('view', selected.view); if (selected.sort) params.set('sort', selected.sort); if (selected.direction) params.set('direction', selected.direction); if ('filter' in selected && selected.filter) { if (!savedListFilters.includes(selected.filter)) throw new Error('Unknown saved list filter.'); params.set('filter', selected.filter); } }
    const [raw, rawResource] = await this.deadline(Promise.all([api.request<unknown>(path + '?' + params, 'GET', undefined, controller.signal), selected.view === 'playlist' ? api.request<unknown>('/v1/playlists/' + encodeURIComponent(selected.playlistId), 'GET', undefined, controller.signal) : Promise.resolve(null)]), controller);
    if (this.disposed || generation !== this.generation || read !== this.readGeneration) return;
    const parsed = projection(raw, bound, selected), playlist = selected.view === 'playlist' ? resource(rawResource, bound, selected.playlistId) : null;
    if (playlist && this.state.lastReceipt?.playlistId === playlist.id && playlist.revision < this.state.lastReceipt.revision) throw new Failure('stale_continuation', 'The server returned a playlist older than the confirmed change. Refresh again.', true);
    if (playlist && (playlist.viewerFence !== parsed.scope.viewerFence || playlist.revision !== parsed.playlistRevision)) throw new Failure('stale_continuation', 'The playlist changed while loading. Refresh its current order.', true);
    if (cursor && previous && (JSON.stringify(parsed.revision) !== JSON.stringify(previous.revision) || parsed.scope.viewerFence !== previous.scope.viewerFence)) throw new Failure('stale_continuation', 'Saved results changed. Refresh the first page.', true);
    if (move === 'next') { this.history.push(oldCursor); if (this.history.length > 64) this.history.shift(); } else if (move === 'previous') this.history.pop();
    this.publish({ phase: 'ready', projection: parsed, playlist, pagination: { cursor, history: [...this.history], canPrevious: this.history.length > 0, next: parsed.sections.filter(s => s.nextCursor).map(s => ({ sectionId: s.id, cursor: s.nextCursor })) } });
   } catch (error) {
    if (this.disposed || generation !== this.generation || read !== this.readGeneration) return;
    const e = errorInfo(error), denied = ['unauthorized','forbidden','not_found','playlist_forbidden','playlist_not_found','policy_expired'].includes(e.code);
    this.publish({ phase: e.code === 'stale_continuation' || cursor !== null && e.code === 'invalid_cursor' ? 'refresh-required' : 'error', error: e, ...(denied ? { projection: null, playlist: null, candidates: null } : {}) });
   }
  })(); this.readPromise = task; return task;
 }
 private allowed(command: SavedIntent): void {
  if (command.action === 'create') { if (this.state.route?.view !== 'playlists' || this.state.phase !== 'ready' || !this.state.projection?.actions?.includes('create_playlist')) throw new Error('Open Playlists before creating a playlist.'); return; }
  const p = this.state.playlist;
  if (!p || this.state.phase !== 'ready') throw new Error('Load the current playlist before changing it.');
  const required = command.action === 'unshare' ? 'share' : command.action === 'rename' && command.name === undefined ? 'summary' : command.action;
  if (!p.actions.includes(required) || command.action === 'rename' && command.summary !== undefined && !p.actions.includes('summary')) throw new Error('This playlist action is not available.');
  if (command.action === 'add' && p.limits.maxEntries !== undefined && p.entryCount >= p.limits.maxEntries) throw new Error('This playlist is at its entry limit.');
  if (command.action === 'reorder') {
   const order = this.state.order;
   const sections = this.state.projection?.sections ?? [];
   const fullPage = sections.length === 1 && !sections[0].nextCursor && sections[0].entries.length === p.entryCount && this.state.projection?.playlistRevision === p.revision;
   const verified = order && order.revision === p.revision && order.viewerFence === p.viewerFence && order.playlistId === p.id ? order.entryIds : fullPage ? sections[0].entries.map(e => e.id) : null;
   // Windowed reorder needs only the moved occurrences plus the anchor to be
   // known at the current revision — never the whole multi-thousand list.
   if (!verified || !command.entryIds.length || command.entryIds.length > Math.min(200, p.limits.maxReorderEntries) || command.entryIds.some(entryId => !verified.includes(entryId))) throw new Error('Load the current playlist order before moving these entries.');
   if (command.afterEntryId !== undefined && command.afterEntryId !== '' && !verified.includes(command.afterEntryId)) throw new Error('Load the current playlist order before moving these entries.');
  }
 }
  mutate(input: SavedIntent): Promise<void> {
   this.active(); const checked = intent(input); this.allowed(checked); if (this.commands.length >= 8) throw new Error('Wait for pending playlist changes.');
   let resolve!: () => void; let reject!: (error: unknown) => void; const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej; });
   // Two deliberate Add calls are distinct occurrences. Only retryMutation reuses an operation —
   // except Q7's ambiguous resume: when the last failure was retry-required with
   // the identical intent, a fresh mutate reuses its idempotency key once so an
   // ambiguous (possibly-confirmed) add is never duplicated. Deliberate
   // same-item duplicates after a definite failure still need a new key; see
   // Questions (playlist-selection owns that distinction).
   // Q7: a failed command rejects to its caller and leaves the queue, so later
   // commands proceed; the operation is kept in lastFailure for explicit retry.
   promise.catch(() => {});
   const reuse = this.lastFailure?.kind === 'retry-required' && JSON.stringify(this.lastFailure.intent) === JSON.stringify(checked) ? this.lastFailure : undefined;
   if (reuse) this.lastFailure = undefined;
   const orderRevision = reuse?.orderRevision !== undefined ? reuse.orderRevision : checked.action === 'reorder' ? this.state.playlist!.revision : undefined;
   this.commands.push({ intent: checked, ...(orderRevision !== undefined ? {orderRevision} : {}), ...(reuse?.operationId ? {operationId: reuse.operationId} : {}), ...(reuse?.body ? {body: {...reuse.body}} : {}), phase: 'queued', promise, resolve, reject }); this.publish(); void this.pump(); return promise;
  }
  retryMutation(): Promise<void> {
   this.active();
   const c = this.commands[0];
   if (c && (this.running || c.phase === 'queued' || c.phase === 'sending')) return c.promise;
   if (c && (c.phase === 'retry-required' || c.phase === 'conflict')) {
    if (c.phase === 'conflict') { this.allowed(c.intent); c.operationId = undefined; c.body = undefined; if (c.intent.action === 'reorder') c.orderRevision = this.state.playlist!.revision; }
    let resolve!: () => void; let reject!: (error: unknown) => void; c.promise = new Promise<void>((res, rej) => { resolve = res; reject = rej; }); c.promise.catch(() => {}); c.resolve = resolve; c.reject = reject; c.phase = 'queued'; this.publish({ mutationError: null }); void this.pump(); return c.promise;
   }
   // Q7: failed commands are removed from the queue; retry the last failure
   // with its original idempotency key (retry-required) or a fresh one
   // (conflict, after re-checking the current revision).
   const failed = this.lastFailure;
   if (!failed) return Promise.resolve();
   if (failed.kind === 'conflict') this.allowed(failed.intent);
   let resolve!: () => void; let reject!: (error: unknown) => void; const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej; }); promise.catch(() => {});
   const orderRevision = failed.kind === 'conflict' && failed.intent.action === 'reorder' ? this.state.playlist!.revision : failed.orderRevision;
   this.commands.unshift({intent: failed.intent, ...(orderRevision !== undefined ? {orderRevision} : {}), ...(failed.kind === 'conflict' ? {} : {...(failed.operationId ? {operationId: failed.operationId} : {}), ...(failed.body ? {body: {...failed.body}} : {})}), phase: 'queued', promise, resolve, reject});
   this.lastFailure = undefined;
   this.publish({mutationError: null}); void this.pump(); return promise;
  }
 private async pump(): Promise<void> {
  if (this.running || this.disposed || this.commands[0]?.phase !== 'queued') return;
  const c = this.commands[0], generation = this.generation, api = this.api, selected = this.state.route, fence = this.state.projection?.scope.viewerFence;
  const controller = new AbortController(); this.running = true; c.phase = 'sending'; this.publish({ mutationError: null });
  try {
   if (!c.operationId) { this.allowed(c.intent); if (c.intent.action === 'reorder' && c.orderRevision !== this.state.playlist?.revision) throw new Failure('playlist_conflict', 'The playlist changed after this order was chosen. Reload and review before retrying.', true); const value = await this.deadline(this.requestId(), controller); if (!uuid.test(value)) throw new Failure('invalid_request_id', 'A UUIDv4 operation ID is required.'); if (generation !== this.generation || this.disposed) return; c.operationId = value; c.body = { operationId: value, ...(c.intent.action === 'create' ? {} : { expectedRevision: c.intent.action==='pin'?this.state.playlist!.pinRevision:c.orderRevision ?? this.state.playlist!.revision }) }; }
   if (!selected || !fence) throw new Failure('saved_required', 'Reload Saved before retrying.', true);
   const value = c.intent; let path = '/v1/playlists', method = 'POST'; const body = { ...c.body };
   if (value.action !== 'create') { if (selected.view !== 'playlist') throw new Error('Playlist is not selected.'); path += '/' + encodeURIComponent(selected.playlistId); }
   switch (value.action) {
    case 'pin': path='/v1/saved-pins/playlist/'+encodeURIComponent(this.state.playlist!.id);method='PUT';body.pinned=value.pinned;break;
    case 'create': case 'rename': if(value.action==='create'&&value.itemIds)body.itemIds=value.itemIds;if (value.action === 'rename') method = 'PATCH'; if (value.name !== undefined) body.name = value.name; if (value.summary !== undefined) body.summary = value.summary; break;
    case 'add': path += '/entries'; body.itemId = value.itemId; break;
    case 'remove': path += '/entries/' + encodeURIComponent(value.entryId); method = 'DELETE'; break;
    case 'reorder': path += '/order'; method = 'PUT'; body.entryIds = value.entryIds; if (value.afterEntryId !== undefined) body.afterEntryId = value.afterEntryId; break;
    case 'delete': method = 'DELETE'; break;
    case 'share': case 'unshare': path += '/shares'; method = value.action === 'share' ? 'PUT' : 'DELETE'; Object.assign(body, value.actor); if (value.action === 'share') body.role = value.role; break;
   }
   const raw = await this.deadline(api.request<unknown>(path, method, body, controller.signal), controller);
   if (this.disposed || generation !== this.generation) return;
   if (!record(raw) || raw.serverId !== this.bound.serverId || raw.viewerFence !== fence || raw.operationId !== c.operationId || !id(raw.playlistId) || selected.view === 'playlist' && raw.playlistId !== selected.playlistId || !int(raw.revision) || typeof raw.deleted !== 'boolean' || raw.entryId !== undefined && !id(raw.entryId)) invalid();
   const receipt: SavedReceipt = freeze({ serverId: this.bound.serverId, viewerFence: fence, operationId: c.operationId, playlistId: raw.playlistId, revision: raw.revision, deleted: raw.deleted, ...(raw.entryId === undefined ? {} : { entryId: raw.entryId as string }) });
   this.candidateController?.abort(); this.candidateGeneration++; this.orderController?.abort(); this.orderGeneration++;
   this.commands.shift(); this.publish({ lastReceipt: receipt, mutationError: null, candidates: null, candidatesLoading: false, order: null, orderLoading: false, orderError: null });
   if (receipt.deleted && selected.view === 'playlist') { for (const pending of this.commands) pending.resolve(); this.commands = []; this.lastFailure = undefined; this.readController?.abort(); this.readGeneration++; this.publish({ phase: 'ready', projection: null, playlist: null, pagination: { cursor: null, history: [], canPrevious: false, next: [] } }); c.resolve(); }
   else { await this.refresh(); c.resolve(); }
   } catch (error) {
    if (this.disposed || generation !== this.generation) return;
    const e = errorInfo(error);
    const kind = ['playlist_conflict','revision_conflict','resource_conflict'].includes(e.code) ? 'conflict' : 'retry-required';
    // Q7: report the failure to its caller and remove it from the queue so
    // later commands proceed. Conflict still refreshes to the current revision.
    // The operation is kept for explicit retryMutation (same idempotency key
    // for retry-required, fresh key for conflict). Settle after the conflict
    // refresh so a sequential caller sees a ready service.
    this.commands.shift();
    this.lastFailure = {intent: c.intent, ...(c.operationId ? {operationId: c.operationId} : {}), ...(c.body ? {body: {...c.body}} : {}), ...(c.orderRevision !== undefined ? {orderRevision: c.orderRevision} : {}), kind};
    this.publish({ mutationError: e }); if (kind === 'conflict') await this.refresh();
    c.reject(failedError(e));
   } finally { if (generation === this.generation) { this.running = false; if (this.commands[0]?.phase === 'queued') void this.pump(); } }
 }
 /** Windowed order read: one page of opaque occurrence ids
  * (1–100, default 100) with the revision and the next cursor. Restart from
  * the first page when the revision moves. */
 async loadOrder(cursor?: string, limit?: number): Promise<void> {
  this.active(); const selected = this.state.route, p = this.state.playlist;
  if (selected?.view !== 'playlist' || this.state.phase !== 'ready' || !p?.actions.includes('reorder')) throw new Error('Full order requires a current editable playlist.');
  if (cursor !== undefined && (!text(cursor, 4096) || !cursor)) throw new Error('Invalid playlist order continuation.');
  if (limit !== undefined && (!Number.isInteger(limit) || limit < 1 || limit > 100)) throw new Error('Playlist order pages read 1 to 100 occurrences.');
  this.orderController?.abort(); const controller = new AbortController(); this.orderController = controller;
  const generation = this.generation, read = ++this.orderGeneration;
  this.publish({ order: cursor ? this.state.order : null, orderLoading: true, orderError: null });
  try {
   const query = new URLSearchParams({limit: String(limit ?? 100)});
   if (cursor) query.set('cursor', cursor);
   const raw = await this.deadline(this.api.request<unknown>('/v1/playlists/' + encodeURIComponent(p.id) + '/order?' + query, 'GET', undefined, controller.signal), controller);
   if (this.disposed || generation !== this.generation || read !== this.orderGeneration) return;
   if (!record(raw) || Object.keys(raw).some(k => !['serverId','viewerFence','playlistId','revision','entryIds','nextCursor'].includes(k)) || raw.serverId !== this.bound.serverId || raw.viewerFence !== p.viewerFence || raw.playlistId !== p.id || !int(raw.revision)) invalid();
   const current = this.state.playlist;
   if (!current || current.revision !== raw.revision || current.viewerFence !== raw.viewerFence || !current.actions.includes('reorder')) throw new Failure('stale_continuation', 'The playlist changed. Refresh it before loading its order.', true);
   if (raw.nextCursor !== undefined && !text(raw.nextCursor, 4096)) invalid();
   const entryIds = unique(list(raw.entryIds, limit ?? 100).map(value => { if (!id(value)) invalid(); return value; }), value => value);
   this.publish({ orderLoading: false, order: freeze({ serverId: this.bound.serverId, viewerFence: p.viewerFence, playlistId: p.id, revision: raw.revision, entryIds, nextCursor: String(raw.nextCursor ?? '') }) });
  } catch (error) { if (!this.disposed && generation === this.generation && read === this.orderGeneration) this.publish({ order: cursor ? this.state.order : null, orderLoading: false, orderError: errorInfo(error) }); }
 }
 /** Reads the next order window and appends it (deduplicated by occurrence id). A no-op without a cursor. */
 async loadMoreOrder(): Promise<void> {
  this.active();
  const window = this.state.order;
  if (!window?.nextCursor || this.state.orderLoading) return;
  this.orderController?.abort(); const controller = new AbortController(); this.orderController = controller;
  const generation = this.generation, read = ++this.orderGeneration, previous = window;
  this.publish({ orderLoading: true, orderError: null });
  try {
   const raw = await this.deadline(this.api.request<unknown>('/v1/playlists/' + encodeURIComponent(previous.playlistId) + '/order?limit=100&cursor=' + encodeURIComponent(previous.nextCursor), 'GET', undefined, controller.signal), controller);
   if (this.disposed || generation !== this.generation || read !== this.orderGeneration) return;
   if (!record(raw) || raw.serverId !== this.bound.serverId || raw.viewerFence !== previous.viewerFence || raw.playlistId !== previous.playlistId || !int(raw.revision)) invalid();
   if (raw.revision !== previous.revision) throw new Failure('stale_continuation', 'The playlist changed. Restart its order from the first page.', true);
   if (raw.nextCursor !== undefined && !text(raw.nextCursor, 4096)) invalid();
   const seen = new Set(previous.entryIds);
   const entryIds = [...previous.entryIds, ...unique(list(raw.entryIds, 100).map(value => { if (!id(value)) invalid(); return value; }), value => value).filter(entryId => !seen.has(entryId))];
   this.publish({ orderLoading: false, order: freeze({ ...previous, entryIds: Object.freeze(entryIds), nextCursor: String(raw.nextCursor ?? '') }) });
  } catch (error) { if (!this.disposed && generation === this.generation && read === this.orderGeneration) this.publish({ orderLoading: false, orderError: errorInfo(error) }); }
 }
 async loadCandidates(cursor?: string): Promise<void> {
  this.active(); const selected = this.state.route, p = this.state.playlist;
  if (selected?.view !== 'playlist' || !p?.actions.includes('share')) throw new Error('Share candidates require an owner playlist.');
  if (cursor !== undefined && (!text(cursor, 4096) || cursor !== this.state.candidates?.nextCursor)) throw new Error('Invalid candidate continuation.');
  this.candidateController?.abort(); const controller = new AbortController(); this.candidateController = controller;
  const generation = this.generation, read = ++this.candidateGeneration;
  this.publish({ candidatesLoading: true, candidatesError: null });
  try {
   const query = new URLSearchParams({ limit: '40' }); if (cursor) query.set('cursor', cursor);
   const raw = await this.deadline(this.api.request<unknown>('/v1/playlists/' + encodeURIComponent(selected.playlistId) + '/share-candidates?' + query, 'GET', undefined, controller.signal), controller);
   if (this.disposed || generation !== this.generation || read !== this.candidateGeneration) return;
   if (!record(raw) || raw.serverId !== this.bound.serverId || raw.viewerFence !== p.viewerFence || raw.playlistId !== p.id || !int(raw.revision) || !text(raw.nextCursor, 4096)) invalid();
   if (raw.revision !== this.state.playlist?.revision) throw new Failure('stale_continuation', 'The playlist changed. Reload its share candidates.', true);
   const candidates = unique(list(raw.candidates, 40).map(row => { if (!record(row) || !id(row.id) || !text(row.displayName)) invalid(); return { ...actor(row), id: row.id, displayName: row.displayName }; }), row => row.id);
   this.publish({ candidatesLoading: false, candidates: freeze({ serverId: this.bound.serverId, viewerFence: p.viewerFence, playlistId: p.id, revision: raw.revision, candidates, nextCursor: raw.nextCursor }) });
  } catch (error) { if (!this.disposed && generation === this.generation && read === this.candidateGeneration) this.publish({ candidatesLoading: false, candidatesError: errorInfo(error), candidates: null }); }
 }
 private async deadline<T>(operation: Promise<T>, controller: AbortController): Promise<T> {
  this.controllers.add(controller); let timer: ReturnType<typeof setTimeout>|undefined, listener: (() => void)|undefined;
  try { return await Promise.race([operation, new Promise<never>((_, reject) => { listener = () => reject(new Failure('cancelled','Saved request cancelled.')); if (controller.signal.aborted) listener(); else controller.signal.addEventListener('abort', listener, { once: true }); timer = setTimeout(() => { reject(new Failure('timeout','Saved request timed out. Retry the same operation.',true)); controller.abort(); }, this.timeoutMs); })]); }
  finally { if (timer) clearTimeout(timer); if (listener) controller.signal.removeEventListener('abort', listener); this.controllers.delete(controller); }
 }
}
