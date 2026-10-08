import {unreadableServerResponse} from './server-messages.ts';
import {parseListeningJourney,type ListeningJourney} from './listening.ts';
import {parseBrowseNode,type BrowseNode,type BrowseSortSelection} from './browse.ts';
/** Transport/state for server-authored semantic projections. No client ranking or grouping. */
export type ContentView = 'discover' | 'browse' | 'collections' | 'categories' | 'show' | 'season' | 'artist' | 'album' | 'book' | 'collection' | 'releases' | 'songs' | 'authors' | 'series' | 'author' | 'book_series' | 'disc';
export type ContentSurface = ContentView | 'home';
export type ContentRoute = Readonly<{ sort?: string; direction?: 'asc' | 'desc'; category?: string; q?: string } & ({ view: 'home'; libraryId?: never; entityId?: never } | { libraryId: string; view: Exclude<ContentView, 'home'>; entityId?: string })>;
export type ContentScope = Readonly<{ viewerId: string; serverId: string }>;
export interface LibraryContentApi { request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> }
export type ContentEntryKind = 'movie' | 'show' | 'season' | 'episode' | 'artist' | 'album' | 'song' | 'book' | 'audiobook_file' | 'chapter' | 'collection' | 'category' | 'author' | 'book_series' | 'disc' | 'extra';
/** `params` fill a parameterised heading (a Discover personal row: "{genre} for you"); name it with `serverTextLabel`. */
export type ContentHeading = Readonly<{ key: string; fallback: string; params?: Readonly<Record<string, string>> }>;
export type ContentEntry = Readonly<{
  id: string; libraryId?: string; kind: ContentEntryKind; title: string; subtitle?: string; posterUrl?: string; backdropUrl?: string; /** An episode's own still, never inherited from the season or show (absent when it has none). */ stillUrl?: string; overview?: string;
  available?: boolean; watched?:boolean;trackNumber?:number;episodeNumber?:number;seasonNumber?:number; duration?: number; progressSeconds?: number; addedAt?: string | null; count?: number;
  year?: number; contentRating?: string; genres?: readonly string[]; watchlisted?: boolean;
  /** The album artist; album entities only (M24, behind presence). Absent on older servers. */
  artist?: Readonly<{ id: string; name: string }>;
  /** A book's author; on book entries, and on an audiobook file (its book's author). */
  author?: string;
  /** How many times the viewer has played a song to the end; song entries only, absent when never played. */
  plays?: number;
  /** A song's album; song entries only. */
  album?: Readonly<{ id: string; name: string }>;
  /** The release's relationship to the artist; artist releases only. Unknown values are dropped. */
  role?: 'album' | 'ep' | 'single' | 'compilation' | 'appearance';
  /** Up to 4 poster paths for a 2x2 mosaic; category rows only. Absent when there is nothing to show. */
  artworkPaths?: readonly string[];
  navigation?: Readonly<{ view: ContentView | 'item'; entityId?: string; category?: string }>;
  playback?: Readonly<{ itemId: string; startSeconds?: number }>;
  /** An episode's air date (YYYY-MM-DD): show workspace pages, and episode rows of a browse page. */
  airDate?: string;
  /** A show's most recently aired episode, on a Recently aired page: the card is the show's, its caption the episode's. */
  latestEpisode?: Readonly<{airDate: string; seasonNumber?: number; episodeNumber: number; title?: string; count: number}>;
  /** An episode's number across the show's seasons, when the show is set to absolute numbering: it reads "Episode 1043". */
  absoluteNumber?: number;
  /** Community rating (0–10) and the best file's picture class; browse pages only (the list view's columns). */
  rating?: number;
  resolution?: '4k' | '1080p' | '720p' | 'sd';
  /** Where a provider biography came from ("Wikipedia") and its article; artist entities only. */
  overviewSource?: string;
  overviewSourceUrl?: string;
  /** The artist's country code and active years (no end year: still active); artist entities only. */
  country?: string;
  activeBeginYear?: number;
  activeEndYear?: number;
  /** The album's record label and release-group type (Album, EP, Single, Compilation); album entities only. */
  label?: string;
  albumType?: string;
}>;
/** A Discover personal section's See all (Recommendations P6): the library's browse with exactly this pivot, query and sort (For you). */
export type ContentSeeAll = Readonly<{ pivot: string; query?: BrowseNode; sort: readonly BrowseSortSelection[] }>;
export type ContentSection = Readonly<{ id: string; type: 'rail' | 'grid' | 'list'; heading: ContentHeading; entries: readonly ContentEntry[]; totalCount: number; nextCursor: string; /** First entry's index in the whole section (M20): the requested `start`, a listening continuation's resume offset, else 0. Optional so older projections still parse. */ start?: number; seeAll?: ContentSeeAll }>;
export type ContentFilterOption = Readonly<{ id: string; label: string; count: number }>;
export type ContentFilter = Readonly<{ id: string; labelKey: string; options: readonly ContentFilterOption[] }>;
export type ContentProjection = Readonly<{
  heading: ContentHeading;
  entity?: ContentEntry;
  listening?: ListeningJourney;
  scope: Readonly<{ serverId: string; libraryId: string; libraryKind: string; view: ContentSurface; entityId: string; viewerFence: string }>;
  revision: Readonly<{ catalog: number; viewer: number }>;
  navigation: readonly Readonly<{ id: string; labelKey: string; view: ContentView }>[];
  query: Readonly<{ sort: string; direction: string; category: string; q: string; limit: number; searchMode: 'none' | 'title_prefix' }>;
  sorts: readonly Readonly<{ id: string; labelKey: string; directions: readonly ('asc' | 'desc')[] }>[];
  filters: readonly ContentFilter[];
  sections: readonly ContentSection[];
  empty?: ContentHeading;
}>;
export type LibraryContentSnapshot = Readonly<{
  generation: number; scope: ContentScope; route: ContentRoute | null;
  phase: 'idle' | 'loading' | 'ready' | 'error' | 'refresh-required';
  projection: ContentProjection | null;
  sections: readonly ContentSection[];
  pagination: Readonly<{ cursor: string | null; canPrevious: boolean; next: readonly Readonly<{ sectionId: string; cursor: string }>[] }>;
  error: Readonly<{ code: string; message: string; retryable: boolean }> | null;
}>;
const views: readonly string[] = ['home','discover','browse','collections','categories','show','season','artist','album','book','collection','releases','songs','authors','series','author','book_series','disc'];
const entityViews = new Set(['show','season','artist','album','book','collection','author','book_series','disc']);
const entryKinds: readonly string[] = ['movie','show','season','episode','artist','album','song','book','audiobook_file','chapter','collection','category','author','book_series','disc','extra'];
/** Unknown string kinds are future server cards; only their entries are omitted. */
export const unknownContentEntryKind = (v: unknown): boolean => record(v) && typeof v.kind === 'string' && !entryKinds.includes(v.kind);
/** A malformed See all query drops only that destination, never the section. */
const optional = <K extends string, V>(key: K, value: V | undefined): Partial<Record<K, V>> => (value === undefined ? {} : { [key]: value } as Record<K, V>);
const record = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const string = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const synopsis = (v: unknown): v is string => typeof v === 'string' && v.length <= 16384 && !/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const id = (v: unknown): v is string => string(v, 256) && v.length > 0;
const count = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;
const finite = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v) && v >= 0;
class ContentFailure extends Error {
  code: string; retryable: boolean;
  constructor(code: string, message: string, retryable = false) { super(message); this.code = code; this.retryable = retryable; }
}
function invalid(): never { throw new ContentFailure('invalid_projection', unreadableServerResponse); }
function boundedArray(v: unknown, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) invalid(); return v; }
function unique<T>(rows: T[], getID: (row: T) => string): T[] {
  if (new Set(rows.map(getID)).size !== rows.length) invalid();
  return rows;
}
function heading(v: unknown): ContentHeading {
  if (!record(v) || !id(v.key) || !string(v.fallback, 512)) invalid();
  if (v.params === undefined) return Object.freeze({ key: v.key, fallback: v.fallback });
  if (!record(v.params) || Object.keys(v.params).length > 16) invalid();
  const params: Record<string, string> = {};
  for (const [name, value] of Object.entries(v.params)) {
    if (!id(name) || !string(value, 1024)) invalid();
    params[name] = value as string;
  }
  return Object.freeze({ key: v.key, fallback: v.fallback, params: Object.freeze(params) });
}
function seeAll(v: unknown): ContentSeeAll | undefined {
  if (!record(v) || !id(v.pivot) || !Array.isArray(v.sort) || v.sort.length < 1 || v.sort.length > 3) invalid();
  const sort = (v.sort as unknown[]).map(s => {
    if (!record(s) || !id(s.field) || (s.direction !== 'asc' && s.direction !== 'desc')) invalid();
    return Object.freeze({ field: s.field, direction: s.direction as 'asc' | 'desc' });
  });
  let query: BrowseNode | undefined;
  if (v.query !== undefined && v.query !== null) { try { query = parseBrowseNode(v.query); } catch { return undefined; } }
  return Object.freeze({ pivot: v.pivot, ...(query ? { query } : {}), sort: Object.freeze(sort) });
}
function entry(v: unknown): ContentEntry {
  if (!record(v) || !id(v.id) || !entryKinds.includes(v.kind as string) || !string(v.title, 2048)) invalid();
  const row: { -readonly [K in keyof ContentEntry]: ContentEntry[K] } = { id: v.id, kind: v.kind as ContentEntryKind, title: v.title };
  if (v.libraryId !== undefined) { if (!id(v.libraryId)) invalid(); row.libraryId = v.libraryId; }
  if (v.overview !== undefined) { if (!synopsis(v.overview)) invalid(); row.overview = v.overview; }
  for (const field of ['subtitle', 'posterUrl', 'backdropUrl', 'stillUrl'] as const) {
    if (v[field] !== undefined) { if (!string(v[field], 4096)) invalid(); row[field] = v[field] as string; }
  }
  if (v.addedAt !== undefined) { if (v.addedAt !== null && !string(v.addedAt, 128)) invalid(); row.addedAt = v.addedAt as string | null; }
  if (v.count !== undefined) { if (!count(v.count)) invalid(); row.count = v.count; }
  if(v.watched!==undefined){if(typeof v.watched!=='boolean')invalid();row.watched=v.watched;}
  for(const field of ['trackNumber','episodeNumber','seasonNumber'] as const){if(v[field]!==undefined){if(!count(v[field]))invalid();row[field]=v[field] as number;}}
  if (v.available !== undefined) { if (typeof v.available !== 'boolean') invalid(); row.available = v.available; }
  for (const field of ['duration', 'progressSeconds'] as const) {
    if (v[field] !== undefined) { if (!finite(v[field])) invalid(); row[field] = v[field] as number; }
  }
  if (v.year !== undefined) { if (typeof v.year !== 'number' || !Number.isSafeInteger(v.year) || v.year <= 0) invalid(); row.year = v.year as number; }
  if (v.contentRating !== undefined) { if (!string(v.contentRating, 64)) invalid(); row.contentRating = v.contentRating as string; }
  // Genres degrade (§4a): be/pages sends up to three for artists and more for albums; the
  // readable ones are kept (at most 8), anything else is dropped, never fatal.
  if (Array.isArray(v.genres)) {
    const genres = (v.genres as unknown[]).filter(g => string(g, 128) && (g as string).length > 0) as string[];
    if (genres.length) row.genres = Object.freeze(genres);
  }
  if (v.watchlisted !== undefined) { if (typeof v.watchlisted !== 'boolean') invalid(); row.watchlisted = v.watchlisted as boolean; }
  // M24 (behind presence): album artist link, release role and category mosaic
  // paths. All three are optional; malformed or unknown values are dropped,
  // never fatal, so older and newer servers both parse.
  if (v.artist !== undefined) {
    const a = v.artist;
    if (record(a) && id(a.id) && string(a.name, 2048) && (a.name as string).length > 0) {
      row.artist = Object.freeze({ id: a.id as string, name: a.name as string });
    }
  }
  if (string(v.author, 512) && (v.author as string).length > 0) row.author = v.author as string;
  if (v.plays !== undefined) { if (typeof v.plays !== 'number' || !Number.isSafeInteger(v.plays) || v.plays < 1) invalid(); row.plays = v.plays; }
  if (v.album !== undefined) {
    const a = v.album;
    if (record(a) && id(a.id) && string(a.name, 2048) && (a.name as string).length > 0) {
      row.album = Object.freeze({ id: a.id as string, name: a.name as string });
    }
  }
  if (v.role !== undefined) {
    if (v.role === 'album' || v.role === 'ep' || v.role === 'single' || v.role === 'compilation' || v.role === 'appearance') row.role = v.role;
  }
  if (v.artworkPaths !== undefined) {
    if (Array.isArray(v.artworkPaths)) {
      const paths = (v.artworkPaths as unknown[]).filter(p => string(p, 4096) && (p as string).length > 0).slice(0, 4) as string[];
      if (paths.length) row.artworkPaths = Object.freeze(paths);
    }
  }
  // Page-content facts (Spec — Page Content by Media Type): all optional; a malformed value is dropped, never fatal.
  if (string(v.airDate, 10) && /^\d{4}-\d{2}-\d{2}$/.test(v.airDate as string)) row.airDate = v.airDate as string;
  if (typeof v.latestEpisode === 'object' && v.latestEpisode !== null) {
    const l = v.latestEpisode as Record<string, unknown>;
    if (typeof l.airDate === 'string' && /^\d{4}-\d{2}-\d{2}/.test(l.airDate) && Number.isSafeInteger(l.episodeNumber) && Number.isSafeInteger(l.count) && (l.count as number) > 0) {
      row.latestEpisode = Object.freeze({airDate: l.airDate.slice(0, 10), episodeNumber: l.episodeNumber as number, count: l.count as number, ...(Number.isSafeInteger(l.seasonNumber) ? {seasonNumber: l.seasonNumber as number} : {}), ...(typeof l.title === 'string' && l.title.length <= 2048 && l.title ? {title: l.title} : {})});
    }
  }
  if (typeof v.absoluteNumber === 'number' && Number.isSafeInteger(v.absoluteNumber) && v.absoluteNumber > 0) row.absoluteNumber = v.absoluteNumber;
  if (typeof v.rating === 'number' && Number.isFinite(v.rating) && v.rating > 0 && v.rating <= 10) row.rating = v.rating;
  if (v.resolution === '4k' || v.resolution === '1080p' || v.resolution === '720p' || v.resolution === 'sd') row.resolution = v.resolution;
  if (string(v.overviewSource, 128) && (v.overviewSource as string).length > 0) row.overviewSource = v.overviewSource as string;
  if (string(v.overviewSourceUrl, 4096) && /^https:\/\//.test(v.overviewSourceUrl as string)) row.overviewSourceUrl = v.overviewSourceUrl as string;
  if (string(v.country, 8) && (v.country as string).length > 0) row.country = v.country as string;
  if (typeof v.activeBeginYear === 'number' && Number.isSafeInteger(v.activeBeginYear) && v.activeBeginYear > 0) row.activeBeginYear = v.activeBeginYear;
  if (typeof v.activeEndYear === 'number' && Number.isSafeInteger(v.activeEndYear) && v.activeEndYear > 0) row.activeEndYear = v.activeEndYear;
  if (string(v.label, 256) && (v.label as string).length > 0) row.label = v.label as string;
  if (string(v.albumType, 64) && (v.albumType as string).length > 0) row.albumType = v.albumType as string;
  if (v.navigation !== undefined) {
    const n = v.navigation;
    if (!record(n) || !(views.includes(n.view as string) || n.view === 'item') || (n.entityId !== undefined && !string(n.entityId, 256)) || (n.category !== undefined && !string(n.category, 256))) invalid();
    row.navigation = Object.freeze({ view: n.view as ContentView | 'item', ...(n.entityId === undefined ? {} : { entityId: n.entityId as string }), ...(n.category === undefined ? {} : { category: n.category as string }) });
  }
  if (v.playback !== undefined) {
    const p = v.playback;
    if (!record(p) || !id(p.itemId) || (p.startSeconds !== undefined && !finite(p.startSeconds))) invalid();
    row.playback = Object.freeze({ itemId: p.itemId, ...(p.startSeconds === undefined ? {} : { startSeconds: p.startSeconds as number }) });
  }
  return Object.freeze(row);
}
function route(input: ContentRoute): ContentRoute {
  if (record(input) && input.view === 'home') {
    if (input.libraryId !== undefined || input.entityId !== undefined || input.sort !== undefined || input.direction !== undefined || input.category !== undefined || input.q !== undefined) throw new Error('Home does not accept library query overrides.');
    return Object.freeze({ view: 'home' });
  }
  if (!record(input) || !id(input.libraryId) || !views.includes(input.view) || (entityViews.has(input.view) && !id(input.entityId))) throw new Error('Invalid library content route.');
  for (const field of ['entityId', 'sort', 'category', 'q'] as const) if (input[field] !== undefined && !string(input[field], field === 'q' ? 512 : 256)) throw new Error('Invalid library content query.');
  if (input.direction !== undefined && input.direction !== 'asc' && input.direction !== 'desc') throw new Error('Invalid library sort direction.');
  return Object.freeze({ libraryId: input.libraryId, view: input.view, ...(input.entityId === undefined ? {} : { entityId: input.entityId }), ...(input.sort === undefined ? {} : { sort: input.sort }), ...(input.direction === undefined ? {} : { direction: input.direction }), ...(input.category === undefined ? {} : { category: input.category }), ...(input.q === undefined ? {} : { q: input.q }) });
}
function scope(input: ContentScope): ContentScope {
  if (!record(input) || !id(input.serverId) || !string(input.viewerId, 1024) || !input.viewerId) throw new Error('A bound server and viewer scope is required.');
  return Object.freeze({ serverId: input.serverId, viewerId: input.viewerId });
}
function projection(raw: unknown, expected: ContentRoute, bound: ContentScope, pageSize: number): ContentProjection {
  if (!record(raw) || !record(raw.scope) || !record(raw.revision) || !record(raw.query)) invalid();
  const s = raw.scope, r = raw.revision, q = raw.query;
  if (s.serverId !== bound.serverId || s.libraryId !== (expected.libraryId ?? '') || s.view !== expected.view || s.entityId !== (expected.entityId ?? '') || !id(s.viewerFence) || !id(s.libraryKind)) invalid();
  if (!count(r.catalog) || !count(r.viewer)) invalid();
  if (q.searchMode !== 'none' && q.searchMode !== 'title_prefix') invalid();
  if (!string(q.sort) || !string(q.direction) || !string(q.category) || !string(q.q) || !count(q.limit) || q.limit < 1 || q.limit > (expected.view === 'home' ? 12 : pageSize)) invalid();
  for (const field of ['sort','direction','category','q'] as const) if (expected[field] !== undefined && expected[field] !== q[field]) invalid();
  if (expected.view === 'home' && (s.libraryKind !== 'mixed' || q.sort !== 'server' || q.direction !== 'asc' || q.category !== '' || q.q !== '' || q.searchMode !== 'none')) invalid();
  const navigation = unique(boundedArray(raw.navigation, 16).map(n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey) || !views.includes(n.view as string)) invalid();
    return Object.freeze({ id: n.id, labelKey: n.labelKey, view: n.view as ContentView });
  }), n => n.id);
  const sorts = unique(boundedArray(raw.sorts, 32).map(n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey)) invalid();
    const directions = boundedArray(n.directions, 2);
    if (!directions.every(v => v === 'asc' || v === 'desc') || new Set(directions).size !== directions.length) invalid();
    return Object.freeze({ id: n.id, labelKey: n.labelKey, directions: Object.freeze(directions as ('asc' | 'desc')[]) });
  }), n => n.id);
  const filters = unique(boundedArray(raw.filters, 16).map(n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey)) invalid();
    const options = unique(boundedArray(n.options, 100).map(o => {
      if (!record(o) || !id(o.id) || !string(o.label) || !count(o.count)) invalid();
      return Object.freeze({ id: o.id, label: o.label, count: o.count });
    }), o => o.id);
    return Object.freeze({ id: n.id, labelKey: n.labelKey, options: Object.freeze(options) });
  }), n => n.id);
  // Discover carries Continue, Recommended, the personal rows, Trending and Recently added.
  const sections = unique(boundedArray(raw.sections, 24).map(n => {
    if (!record(n) || !id(n.id) || !['rail','grid','list'].includes(n.type as string) || !count(n.totalCount) || !string(n.nextCursor, 4096)) invalid();
    const entries = unique(boundedArray(n.entries, n.type === 'rail' ? 12 : 100).filter(v => !unknownContentEntryKind(v)).map(entry), e => e.id);
    if (n.totalCount < entries.length) invalid();
    for (const row of entries) {
      if (expected.view === 'home' ? !row.libraryId : row.libraryId !== undefined && row.libraryId !== expected.libraryId) invalid();
    }
    return Object.freeze({ id: n.id, type: n.type as ContentSection['type'], heading: heading(n.heading), entries: Object.freeze(entries), totalCount: n.totalCount, nextCursor: n.nextCursor, ...(typeof n.start === 'number' && Number.isSafeInteger(n.start) && (n.start as number) >= 0 ? { start: n.start as number } : {}), ...(n.seeAll === undefined || n.seeAll === null ? {} : optional('seeAll', seeAll(n.seeAll))) });
  }), n => n.id);
  let entity: ContentEntry | undefined;
  if(raw.entity!==undefined && !unknownContentEntryKind(raw.entity)){entity=entry(raw.entity);if(!['artist','album','book','disc'].includes(expected.view)||entity.kind!==expected.view||entity.id!==expected.entityId||entity.libraryId!==expected.libraryId||entity.playback!==undefined)invalid();}
  return Object.freeze({ ...(entity?{entity}:{}), ...(raw.listening===undefined?{}:{listening:parseListeningJourney(raw.listening,s.libraryId as string)}), heading: heading(raw.heading), scope: Object.freeze({ serverId: s.serverId as string, libraryId: s.libraryId as string, libraryKind: s.libraryKind as string, view: s.view as ContentSurface, entityId: s.entityId as string, viewerFence: s.viewerFence as string }), revision: Object.freeze({ catalog: r.catalog, viewer: r.viewer }), query: Object.freeze({ sort: q.sort, direction: q.direction, category: q.category, q: q.q, limit: q.limit, searchMode: q.searchMode }), navigation: Object.freeze(navigation), sorts: Object.freeze(sorts), filters: Object.freeze(filters), sections: Object.freeze(sections), ...(raw.empty === undefined ? {} : { empty: heading(raw.empty) }) });
}
type Page = { cursor: string | null; sectionId?: string };
type Operation = { route: ContentRoute; page: Page; history: Page[]; fence: ContentProjection | null };
export class LibraryContentService {
  private api: LibraryContentApi;
  private bound: ContentScope;
  private pageSize: number;
  private timeoutMs: number;
  private listeners = new Set<() => void>();
  private generation = 0;
  private controller?: AbortController;
  private pending?: Promise<void>;
  private activeKey = '';
  private operation?: Operation;
  private history: Page[] = [];
  private current: Page = { cursor: null };
  private disposed = false;
  private snapshot: LibraryContentSnapshot;
  constructor(options: { api: LibraryContentApi; scope: ContentScope; pageSize?: number; timeoutMs?: number }) {
    this.api = options.api; this.bound = scope(options.scope); this.pageSize = options.pageSize ?? 40; this.timeoutMs = options.timeoutMs ?? 15_000;
    if (!Number.isInteger(this.pageSize) || this.pageSize < 1 || this.pageSize > 100 || !Number.isFinite(this.timeoutMs) || this.timeoutMs <= 0 || this.timeoutMs > 120_000) throw new Error('Invalid content request bounds.');
    this.snapshot = this.empty();
  }
  private empty(): LibraryContentSnapshot { return Object.freeze({ generation: this.generation, scope: this.bound, route: null, phase: 'idle', projection: null, sections: Object.freeze([]), pagination: Object.freeze({ cursor: null, canPrevious: false, next: Object.freeze([]) }), error: null }); }
  getSnapshot = (): LibraryContentSnapshot => this.snapshot;
  subscribe = (listener: () => void): (() => void) => { if (this.disposed) return () => {}; this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(patch: Partial<LibraryContentSnapshot>): void { if (this.disposed) return; this.snapshot = Object.freeze({ ...this.snapshot, ...patch }); for (const listener of this.listeners) listener(); }
  private assertActive(): void { if (this.disposed) throw new Error('Content service is disposed.'); }
  select(input: ContentRoute, options: { cursor?: string | null } = {}): Promise<void> {
    this.assertActive();
    const selected = route(input.q === undefined ? input : { ...input, q: input.q.trim() }), cursor = options.cursor ?? null;
    if (cursor !== null && (!string(cursor, 4096) || !cursor)) throw new Error('Invalid content cursor.');
    return this.load({ route: selected, page: { cursor }, history: [], fence: null });
  }
  /** Explicit refresh always restarts the first page, including after stale_continuation. */
  refresh(): Promise<void> { this.assertActive(); return this.snapshot.route ? this.select(this.snapshot.route) : Promise.resolve(); }
  retry(): Promise<void> { this.assertActive(); if (this.snapshot.phase === 'refresh-required') return Promise.resolve(); return this.operation ? this.load(this.operation) : Promise.resolve(); }
  next(sectionId: string): Promise<void> {
    this.assertActive();
    if (this.snapshot.phase !== 'ready' || !this.snapshot.projection || !this.snapshot.route) return Promise.resolve();
    const section = this.snapshot.sections.find(s => s.id === sectionId);
    if (!section?.nextCursor) return Promise.resolve();
    return this.load({ route: this.snapshot.route, page: { cursor: section.nextCursor, sectionId }, history: [...this.history, this.current].slice(-63), fence: this.snapshot.projection });
  }
  previous(): Promise<void> {
    this.assertActive();
    if (this.snapshot.phase !== 'ready' || !this.snapshot.route || !this.history.length) return Promise.resolve();
    return this.load({ route: this.snapshot.route, page: this.history[this.history.length - 1], history: this.history.slice(0, -1), fence: this.snapshot.projection });
  }
  /** A new scoped API must be bound to the new viewer/server; tokens never enter this service. */
  setScope(next: ContentScope, api: LibraryContentApi): void {
    this.assertActive(); const checked = scope(next); this.controller?.abort(); this.generation++; this.bound = checked; this.api = api;
    this.operation = undefined; this.history = []; this.current = { cursor: null }; this.activeKey = ''; this.pending = undefined;
    this.publish(this.empty());
  }
  cancel(): void { this.assertActive(); this.controller?.abort(); this.generation++; this.pending = undefined; this.activeKey = ''; this.publish({ ...this.empty(), route: this.snapshot.route }); }
  dispose(): void { this.controller?.abort(); this.disposed = true; this.generation++; this.listeners.clear(); }
  private load(op: Operation): Promise<void> {
    const key = JSON.stringify([op.route, op.page]);
    if (this.snapshot.phase === 'loading' && this.activeKey === key && this.pending) return this.pending;
    this.controller?.abort(); const controller = new AbortController(); this.controller = controller;
    const generation = ++this.generation, api = this.api, bound = this.bound;
    this.activeKey = key; this.operation = op;
    // Retain a bounded, already-authorized page only within this exact destination.
    // setScope/cancel clear synchronously; a different library/entity/view never inherits it.
    const previous = this.snapshot.route;
    const retain = previous?.view === op.route.view && previous?.libraryId === op.route.libraryId && previous?.entityId === op.route.entityId;
    this.publish({ generation, route: op.route, phase: 'loading', projection: retain ? this.snapshot.projection : null, sections: retain ? this.snapshot.sections : Object.freeze([]), error: null, pagination: retain ? this.snapshot.pagination : Object.freeze({ cursor: op.page.cursor, canPrevious: false, next: Object.freeze([]) }) });
    const task = this.fetch(op, api, bound, controller, generation);
    this.pending = task; return task;
  }
  private async fetch(op: Operation, api: LibraryContentApi, bound: ContentScope, controller: AbortController, generation: number): Promise<void> {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let onAbort: (() => void) | undefined;
    const query = new URLSearchParams({ view: op.route.view, limit: String(op.route.view === 'home' ? 12 : this.pageSize) });
    for (const field of ['entityId','sort','direction','category','q'] as const) if (op.route[field] !== undefined) query.set(field, op.route[field]!);
    if (op.page.cursor) query.set('cursor', op.page.cursor);
    const path = (op.route.view === 'home' ? '/v1/content?' : '/v1/libraries/' + encodeURIComponent(op.route.libraryId) + '/content?') + query;
    try {
      const cancelled = new Promise<never>((_, reject) => { onAbort = () => reject(new ContentFailure('cancelled', 'Library request cancelled.')); controller.signal.addEventListener('abort', onAbort, { once: true }); });
      const raw = await Promise.race([api.request<unknown>(path, 'GET', undefined, controller.signal), cancelled, new Promise<never>((_, reject) => { timer = setTimeout(() => { reject(new ContentFailure('timeout', 'The server took too long to respond. Try again.', true)); controller.abort(); }, this.timeoutMs); })]);
      if (this.disposed || generation !== this.generation) return;
      const result = projection(raw, op.route, bound, this.pageSize);
      if (op.page.cursor && op.fence && (result.scope.viewerFence !== op.fence.scope.viewerFence || result.revision.catalog !== op.fence.revision.catalog || result.revision.viewer !== op.fence.revision.viewer)) throw new ContentFailure('stale_continuation', 'The library changed. Refresh to continue.', true);
      if (op.page.sectionId && (result.sections.length !== 1 || result.sections[0].id !== op.page.sectionId)) invalid();
      for (const section of result.sections) if (section.nextCursor && (section.nextCursor === op.page.cursor || op.history.some(p => p.cursor === section.nextCursor))) throw new ContentFailure('stale_continuation', 'The library continuation repeated. Refresh to continue.', true);
      this.history = op.history; this.current = op.page;
      this.publish({ phase: 'ready', projection: result, sections: result.sections, pagination: Object.freeze({ cursor: op.page.cursor, canPrevious: this.history.length > 0, next: Object.freeze(result.sections.filter(s => s.nextCursor).map(s => Object.freeze({ sectionId: s.id, cursor: s.nextCursor }))) }) });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const e = error as { code?: unknown; retryable?: unknown; message?: unknown; status?: unknown } | null;
      // Q5: checked `server/api/openapi.yaml` — `/v1/content` and
      // `/v1/libraries/{id}/content` accept only `cursor` + `limit` (no
      // `range.start`; that exists only on POST browse). A stale or expired
      // section continuation therefore cannot re-anchor by index client-side;
      // it stays an explicit first-page refresh. `invalid_cursor` on a
      // continuation joins `stale_continuation` there; on a first page it
      // remains a plain error.
      const stale = e?.code === 'stale_continuation' || (!!op.page.cursor && e?.code === 'invalid_cursor');
      const code = string(e?.code) && e?.code ? e.code : 'request_failed';
      const transient = !stale && code !== 'invalid_projection' && e?.status !== 401 && e?.status !== 403 && (e?.retryable === true || code === 'timeout' || error instanceof TypeError || typeof e?.status === 'number' && (e.status >= 500 || e.status === 408 || e.status === 429));
      this.publish({ phase: stale ? 'refresh-required' : 'error', projection: transient ? this.snapshot.projection : null, sections: transient ? this.snapshot.sections : Object.freeze([]), error: Object.freeze({ code, message: stale ? 'The library changed. Refresh to continue.' : error instanceof Error ? error.message : 'Could not load this library.', retryable: stale || transient }), pagination: Object.freeze({ cursor: op.page.cursor, canPrevious: false, next: Object.freeze([]) }) });
    } finally {
      if (timer) clearTimeout(timer);
      if (onAbort) controller.signal.removeEventListener('abort', onAbort);
      if (generation === this.generation) { this.pending = undefined; this.activeKey = ''; }
    }
  }
}

/** Reuse the canonical strict semantic DTO validation in composed server workspaces. */
export { projection as validateContentProjection };

/** Shared strict media-entry validation for server-authored cross-library search. */
export { entry as validateContentEntry };
