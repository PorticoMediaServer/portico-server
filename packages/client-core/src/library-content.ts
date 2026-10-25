import type {RouteResponse} from "../../contracts/src/routes.ts";
import {encodedRoutePath} from "../../contracts/src/routes.ts";
import {wireDurations,wireDisplayDurations} from './wire-units.ts';
import {unreadableServerResponse} from './server-messages.ts';
import {listeningData,parseBookProgress,type BookProgress,parseListeningJourney,type ListeningJourney} from './listening.ts';
import {parseBrowseNode,type BrowseNode,type BrowseSortSelection} from './browse.ts';
import {firstOfEach,keepReadable,noteSkipped,oneOfOr} from './tolerant.ts';
import {mergeRevalidatedList} from './freshness-actions.ts';
/** Transport/state for server-authored semantic projections. No client ranking or grouping. */
export type ContentView = 'discover' | 'browse' | 'collections' | 'categories' | 'show' | 'season' | 'artist' | 'album' | 'book' | 'collection' | 'releases' | 'songs' | 'authors' | 'series' | 'author' | 'book_series' | 'disc';
export type ContentSurface = ContentView;
export type ContentRoute = Readonly<{ sort?: string; direction?: 'asc' | 'desc'; category?: string; q?: string } & { libraryId: string; view: ContentView; entityId?: string }>;
export type ContentScope = Readonly<{ viewerId: string; serverId: string }>;
export interface LibraryContentApi { request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> }
export type ContentEntryKind = 'ebook' | 'comic' | 'movie' | 'show' | 'season' | 'episode' | 'artist' | 'album' | 'song' | 'book' | 'audiobook_file' | 'chapter' | 'collection' | 'category' | 'author' | 'book_series' | 'disc' | 'extra' | 'playlist';
/** `params` fill a parameterised heading (a Discover personal row: "{genre} for you"); name it with `serverTextLabel`. */
export type ContentHeading = Readonly<{ key: string; fallback: string; params?: Readonly<Record<string, string>> }>;
export type ContentEntry = Readonly<{
  publicationType?: 'book' | 'magazine' | 'newspaper';
  dateBased?:boolean;special?:boolean;new?:boolean; show?:Readonly<{id:string;name:string}>;
  favorite?:boolean;releaseDate?:string;artistType?:'Group'|'Person';albumCount?:number;displayTitle?:string;narrators?:readonly string[];series?:readonly Readonly<{name:string;position?:string}>[];publisher?:string;language?:string;edition?:string;publishedYear?:number;bookProgress?:BookProgress;
  id: string; libraryId?: string; kind: ContentEntryKind; title: string; subtitle?: string; posterUrl?: string; backdropUrl?: string; /** An episode's own still, never inherited from the season or show (absent when it has none). */ stillUrl?: string; overview?: string;
  /** The letter a browse page files the entry under (its sort title's, articles already set aside): a key of the page's `positionIndex`. */
  sortLetter?: string;
  missing?:boolean; available?: boolean; watched?:boolean;discNumber?:number;trackNumber?:number;episodeNumber?:number;seasonNumber?:number; duration?: number; progressSeconds?: number; addedAt?: string | null; count?: number;
  /** A container's watched tally, when the server publishes one. Absent is not zero. */
  watchedCount?: number; unwatchedCount?: number;
  year?: number; contentRating?: string; genres?: readonly string[]; watchlisted?: boolean;
  /** The album artist; album entities only (M24, behind presence). Absent on older servers. */
  artist?: Readonly<{ id: string; name: string }>;
  /** Accepted Reading author or a book/audiobook part's semantic author, never parsed from subtitle. */
  author?: string;
  /** How many times the viewer has played a song to the end; song entries only, absent when never played. */
  plays?: number;
  /** A song's album; song entries only. */
  album?: Readonly<{ id: string; name: string }>;
  /** The release's relationship to the artist; artist releases only. Unknown values are dropped. */
  role?: 'album' | 'ep' | 'single' | 'compilation' | 'appearance';
  /** Up to 4 poster paths for a 2x2 mosaic; category rows only. Absent when there is nothing to show. */
  /** `count` is a lower bound: a grouping under a broad filter counts each group up to 10,000 ("10,000+"). */
  countIsLowerBound?: boolean;
  removed?:boolean;
  artworkPaths?: readonly string[];
  yearMin?:number; yearMax?:number; origin?:'automatic'|'owner';
  navigation?: Readonly<{ view: ContentView | 'item' | 'playlist'; entityId?: string; category?: string }>;
  playback?: Readonly<{ itemId: string; startSeconds?: number }>;
  /** An episode's air date (YYYY-MM-DD): show workspace pages, and episode rows of a browse page. */
  airDate?: string;
  /** A show's most recently aired episode, on a Recently aired page: the card is the show's, its caption the episode's. */
  latestEpisode?: Readonly<{airDate: string; seasonNumber?: number; episodeNumber: number; title?: string; count: number}>;
  /** An episode's number across the show's seasons, when the show is set to absolute numbering: it reads "Episode 1043". */
  absoluteNumber?: number;
  /** How many seasons a show has, on a Recently Added shelf when the whole show (several seasons) was the addition: "6 seasons". */
  seasonCount?: number;
  /** Community rating (0–10) and the best file's picture class; browse pages only (the list view's columns). */
  rating?: number;
 userRating?: number;
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
  /**
   * The first page as the server has it now, read again quietly while the screen was open
   * (`revalidate`). `serial` changes with each such read. A screen that has appended later pages
   * merges this into what it shows (`mergeRevalidatedList`) instead of starting over.
   */
  head?: Readonly<{ serial: number; sections: readonly ContentSection[] }>;
  /**
   * Everything read so far, as one list per section: the first page, each later page of a
   * section appended to it (duplicates dropped), and quiet reads merged in place. A screen that
   * shows "More" inside a section renders this; `sections` is only the page last read.
   */
  appended: readonly ContentSection[];
}>;
const views: readonly string[] = ['discover','browse','collections','categories','show','season','artist','album','book','collection','releases','songs','authors','series','author','book_series','disc'];
const entityViews = new Set(['show','season','artist','album','book','collection','author','book_series','disc']);
const entryKinds: readonly string[] = ['ebook','comic','movie','show','season','episode','artist','album','song','book','audiobook_file','chapter','collection','category','author','book_series','disc','extra','playlist'];
/** Unknown string kinds are future server cards; only their entries are omitted. */
export const unknownContentEntryKind = (v: unknown): boolean => record(v) && typeof v.kind === 'string' && !entryKinds.includes(v.kind);
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
/** A part of a projection that could not be read was left out; an absent or null value is not counted. */
function left(part: string, value: unknown): void { if (value !== undefined && value !== null) noteSkipped('content', part); }
/** A heading keeps its words when its parameters cannot be read; the fallback then names it. */
function heading(v: unknown): ContentHeading {
  if (!record(v) || !id(v.key) || !string(v.fallback, 512)) invalid();
  if (v.params === undefined || v.params === null) return Object.freeze({ key: v.key, fallback: v.fallback });
  if (!record(v.params) || Object.keys(v.params).length > 16) { left('heading.params', v.params); return Object.freeze({ key: v.key, fallback: v.fallback }); }
  const params: Record<string, string> = {};
  for (const [name, value] of Object.entries(v.params)) {
    if (!id(name) || !string(value, 1024)) { left('heading.params', value); continue; }
    params[name] = value as string;
  }
  return Object.freeze({ key: v.key, fallback: v.fallback, params: Object.freeze(params) });
}
/** A heading that cannot be read is blank under `key`, so its section or page is still shown. */
function headingOr(v: unknown, key: string): ContentHeading {
  try { return heading(v); } catch { left('heading', v); return Object.freeze({ key, fallback: '' }); }
}
/** A See all destination that cannot be read is left out, never its section or row. */
function seeAll(v: unknown): ContentSeeAll | undefined {
  if (v === undefined || v === null) return undefined;
  try {
    if (!record(v) || !id(v.pivot) || !Array.isArray(v.sort) || v.sort.length < 1 || v.sort.length > 3) invalid();
    const sort = (v.sort as unknown[]).map(s => {
      if (!record(s) || !id(s.field) || (s.direction !== 'asc' && s.direction !== 'desc')) invalid();
      return Object.freeze({ field: s.field, direction: s.direction as 'asc' | 'desc' });
    });
    const query = v.query === undefined || v.query === null ? undefined : parseBrowseNode(v.query);
    return Object.freeze({ pivot: v.pivot, ...(query ? { query } : {}), sort: Object.freeze(sort) });
  } catch { noteSkipped('content', 'seeAll'); return undefined; }
}
/**
 * One entry. Its id, kind, library and destination are exact: an entry whose kind or destination
 * this app does not know is refused, and the caller leaves out that entry alone. Every other
 * field is optional and is left out when it cannot be read.
 */
function entry(v: unknown): ContentEntry {
 v=wireDisplayDurations(v,{duration:"durationMs",progressSeconds:"progressMs"},left);
  if (!record(v) || !id(v.id) || !entryKinds.includes(v.kind as string) || !string(v.title ?? '', 2048)) invalid();
  if ((v.kind==='ebook'||v.kind==='comic') && (v.playback!=null||v.durationMs!=null||v.duration!=null)) invalid();
  const row: { -readonly [K in keyof ContentEntry]: ContentEntry[K] } = { id: v.id, kind: v.kind as ContentEntryKind, title: (v.title ?? '') as string };
  if ((v.kind === 'ebook' || v.kind === 'comic') && (v.publicationType === 'book' || v.publicationType === 'magazine' || v.publicationType === 'newspaper')) row.publicationType = v.publicationType;
  if (v.libraryId !== undefined) { if (!id(v.libraryId)) invalid(); row.libraryId = v.libraryId; }
  if (synopsis(v.overview)) row.overview = v.overview; else left('entry.overview', v.overview);
  for (const field of ['subtitle', 'posterUrl', 'backdropUrl', 'stillUrl'] as const) {
    if (string(v[field], 4096)) row[field] = v[field] as string; else left('entry.' + field, v[field]);
  }
  if (v.addedAt === null || string(v.addedAt, 128)) row.addedAt = v.addedAt as string | null; else left('entry.addedAt', v.addedAt);
  if (string(v.sortLetter, 8) && v.sortLetter) row.sortLetter = v.sortLetter as string; else left('entry.sortLetter', v.sortLetter);
  if (count(v.count)) row.count = v.count; else left('entry.count', v.count);
  if (count(v.totalAtLeast) && row.count !== undefined) row.countIsLowerBound = true; else left('entry.totalAtLeast', v.totalAtLeast);
  // A container's tallies stay exact nonnegative counts; absence stays absent, never zero.
  for (const field of ['watchedCount', 'unwatchedCount'] as const) {
    if (count(v[field])) row[field] = v[field] as number; else left('entry.' + field, v[field]);
  }
  for(const flag of ['dateBased','special'] as const)if(typeof v[flag]==='boolean')row[flag]=v[flag] as boolean;
  if(typeof v.watched==='boolean')row.watched=v.watched;else left('entry.watched',v.watched);
  if(typeof v.new==='boolean')row.new=v.new;else left('entry.new',v.new);
  if(record(v.show)&&id(v.show.id)&&string(v.show.name,2048))row.show=Object.freeze({id:v.show.id,name:v.show.name});else left('entry.show',v.show);
  for(const field of ['discNumber','trackNumber','episodeNumber','seasonNumber'] as const){if(count(v[field]))row[field]=v[field] as number;else left('entry.'+field,v[field]);}
  if (typeof v.missing === 'boolean') row.missing=v.missing;
  if (typeof v.available === 'boolean') row.available = v.available; else left('entry.available', v.available);
  for (const field of ['duration', 'progressSeconds'] as const) {
    if (finite(v[field])) row[field] = v[field] as number; else left('entry.' + field, v[field]);
  }
  if(v.userRating!==undefined){if(typeof v.userRating=== "number"&&Number.isInteger(v.userRating)&&v.userRating>=0&&v.userRating<=10)row.userRating=v.userRating;else left("entry.userRating",v.userRating);}
  // A year of 0 is how a server says it has none.
  if (typeof v.year === 'number' && Number.isSafeInteger(v.year) && v.year > 0) row.year = v.year; else if (v.year !== 0) left('entry.year', v.year);
  if (string(v.contentRating, 64)) row.contentRating = v.contentRating as string; else left('entry.contentRating', v.contentRating);
  // Genres degrade (§4a): be/pages sends up to three for artists and more for albums; the
  // readable ones are kept (at most 8), anything else is dropped, never fatal.
  if (Array.isArray(v.genres)) {
    const genres = (v.genres as unknown[]).filter(g => string(g, 128) && (g as string).length > 0) as string[];
    if (genres.length) row.genres = Object.freeze(genres);
  }
  if(typeof v.favorite==='boolean')row.favorite=v.favorite;else left('entry.favorite',v.favorite);
  if(typeof v.albumCount==='number'&&count(v.albumCount))row.albumCount=v.albumCount;else left('entry.albumCount',v.albumCount);
  for(const key of ['releaseDate','displayTitle','publisher','language','edition'] as const){if(string(v[key],2048)&&v[key])row[key]=v[key] as string;else left('entry.'+key,v[key]);}
  if(v.artistType==='Group'||v.artistType==='Person')row.artistType=v.artistType;else left('entry.artistType',v.artistType);
  if(count(v.publishedYear)&&v.publishedYear>0&&v.publishedYear<=9999)row.publishedYear=v.publishedYear;else left('entry.publishedYear',v.publishedYear);
  if(Array.isArray(v.narrators))row.narrators=Object.freeze(v.narrators.filter((n):n is string=>string(n,1024)&&!!n).slice(0,128));
  if(Array.isArray(v.series)){row.series=Object.freeze(v.series.filter(n=>record(n)&&string(n.name,1024)&&!!n.name&&(n.position===undefined||string(n.position,128))).slice(0,64).map(n=>Object.freeze({name:n.name as string,...(n.position?{position:n.position as string}:{})})));}
  if(v.bookProgress!==undefined){try{row.bookProgress=parseBookProgress(v.bookProgress);if(row.kind==='book'&&row.bookProgress.bookId!==row.id||row.kind==='audiobook_file'&&row.bookProgress.itemId!==row.id)throw new Error('Wrong book identity');}catch{delete row.bookProgress;left('entry.bookProgress',v.bookProgress);}}
  if (typeof v.watchlisted === 'boolean') row.watchlisted = v.watchlisted; else left('entry.watchlisted', v.watchlisted);
  // M24 (behind presence): album artist link, release role and category mosaic
  // paths. All three are optional; malformed or unknown values are dropped,
  // never fatal, so older and newer servers both parse.
  if (v.artist !== undefined) {
    const a = v.artist;
    if (record(a) && id(a.id) && string(a.name, 2048) && (a.name as string).length > 0) {
      row.artist = Object.freeze({ id: a.id as string, name: a.name as string });
    }
  }
  if (string(v.author, v.kind === 'ebook' || v.kind === 'comic' ? 4096 : 512) && (v.author as string).length > 0) row.author = v.author as string;
  if (typeof v.plays === 'number' && Number.isSafeInteger(v.plays) && v.plays >= 1) row.plays = v.plays; else if (v.plays !== 0) left('entry.plays', v.plays);
  if (v.album !== undefined) {
    const a = v.album;
    if (record(a) && id(a.id) && string(a.name, 2048) && (a.name as string).length > 0) {
      row.album = Object.freeze({ id: a.id as string, name: a.name as string });
    }
  }
  if (v.role !== undefined) {
    if (v.role === 'album' || v.role === 'ep' || v.role === 'single' || v.role === 'compilation' || v.role === 'appearance') row.role = v.role;
  }
  for(const field of ['yearMin','yearMax'] as const)if(typeof v[field]==='number'&&Number.isInteger(v[field])&&(v[field] as number)>0)row[field]=v[field] as number;
  if(v.origin==='automatic'||v.origin==='owner')row.origin=v.origin;
  if(v.removed===true)row.removed=true;
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
  if (typeof v.seasonCount === 'number' && Number.isSafeInteger(v.seasonCount) && v.seasonCount > 0) row.seasonCount = v.seasonCount;
  if (typeof v.rating === 'number' && Number.isFinite(v.rating) && v.rating > 0 && v.rating <= 10) row.rating = v.rating;
  if (v.resolution === '4k' || v.resolution === '1080p' || v.resolution === '720p' || v.resolution === 'sd') row.resolution = v.resolution;
  if (string(v.overviewSource, 128) && (v.overviewSource as string).length > 0) row.overviewSource = v.overviewSource as string;
  if (string(v.overviewSourceUrl, 4096) && /^https:\/\//.test(v.overviewSourceUrl as string)) row.overviewSourceUrl = v.overviewSourceUrl as string;
  if (string(v.country, 8) && (v.country as string).length > 0) row.country = v.country as string;
  if (typeof v.activeBeginYear === 'number' && Number.isSafeInteger(v.activeBeginYear) && v.activeBeginYear > 0) row.activeBeginYear = v.activeBeginYear;
  if (typeof v.activeEndYear === 'number' && Number.isSafeInteger(v.activeEndYear) && v.activeEndYear > 0) row.activeEndYear = v.activeEndYear;
  if (string(v.label, 256) && (v.label as string).length > 0) row.label = v.label as string;
  if (string(v.albumType, 64) && (v.albumType as string).length > 0) row.albumType = v.albumType as string;
  // Where the entry leads is exact: an entry that cannot say it is not shown.
  if (v.navigation !== undefined && v.navigation !== null) {
    const n = v.navigation;
    if (!record(n) || !(views.includes(n.view as string) || n.view === 'item' || n.view === 'playlist') || (n.entityId !== undefined && !string(n.entityId, 256)) || (n.category !== undefined && !string(n.category, 256))) invalid();
    row.navigation = Object.freeze({ view: n.view as ContentView | 'item' | 'playlist', ...(n.entityId === undefined ? {} : { entityId: n.entityId as string }), ...(n.category === undefined ? {} : { category: n.category as string }) });
  }
  // Playback that cannot be read is left out when the entry still opens something; otherwise the entry is refused.
  if (v.playback !== undefined && v.playback !== null) {
    const p = wireDisplayDurations(v.playback,{startSeconds:"startMs"},left);
    if (record(p) && id(p.itemId)) row.playback = Object.freeze({ itemId: p.itemId, ...(finite(p.startSeconds) ? { startSeconds: p.startSeconds } : {}) });
    else if (row.navigation) left('entry.playback', p);
    else invalid();
  }
  return Object.freeze(row);
}
function route(input: ContentRoute): ContentRoute {
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
  // A search mode this app does not know reads as no search.
  const searchMode = oneOfOr('content', 'query.searchMode', q.searchMode, ['none', 'title_prefix'] as const, 'none');
  if (!string(q.sort) || !string(q.direction) || !string(q.category) || !string(q.q) || !count(q.limit) || q.limit < 1 || q.limit > pageSize) invalid();
  for (const field of ['sort','direction','category','q'] as const) if (expected[field] !== undefined && expected[field] !== q[field]) invalid();
  // The lists below keep what can be read: a tab, sort, filter, option, section or entry this app
  // cannot read is left out alone, and a repeated id keeps its first appearance.
  const navigation = firstOfEach('content', 'navigation', keepReadable('content', 'navigation', boundedArray(raw.navigation ?? [], 16), n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey) || !views.includes(n.view as string)) invalid();
    return Object.freeze({ id: n.id, labelKey: n.labelKey, view: n.view as ContentView });
  }), n => n.id);
  const sorts = firstOfEach('content', 'sorts', keepReadable('content', 'sorts', boundedArray(raw.sorts ?? [], 32), n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey)) invalid();
    const directions = [...new Set(boundedArray(n.directions, 2).filter(v => v === 'asc' || v === 'desc'))];
    return Object.freeze({ id: n.id, labelKey: n.labelKey, directions: Object.freeze(directions as ('asc' | 'desc')[]) });
  }), n => n.id);
  const filters = firstOfEach('content', 'filters', keepReadable('content', 'filters', boundedArray(raw.filters ?? [], 16), n => {
    if (!record(n) || !id(n.id) || !id(n.labelKey)) invalid();
    const options = firstOfEach('content', 'filter.options', keepReadable('content', 'filter.options', boundedArray(n.options, 100), o => {
      if (!record(o) || !id(o.id) || !string(o.label ?? '')) invalid();
      return Object.freeze({ id: o.id, label: (o.label ?? '') as string, count: count(o.count) ? o.count : 0 });
    }), o => o.id);
    return Object.freeze({ id: n.id, labelKey: n.labelKey, options: Object.freeze(options) });
  }), n => n.id);
  // Discover carries Continue, Recommended, the personal rows, Trending and Recently added.
  const sections = firstOfEach('content', 'sections', keepReadable('content', 'sections', boundedArray(raw.sections, 24), n => {
    if (!record(n) || !id(n.id) || !string(n.nextCursor ?? '', 4096)) invalid();
    // A layout this app does not know is drawn as a grid, which holds any number of entries.
    const type = oneOfOr('content', 'section.type', n.type, ['rail', 'grid', 'list'] as const, 'grid');
    const entries = firstOfEach('content', 'entries', keepReadable('content', 'entries', boundedArray(n.entries, type === 'rail' ? 12 : 100), v => {
      const row = entry(v);
      if (row.libraryId !== undefined && row.libraryId !== expected.libraryId) invalid();
      return row;
    }), e => e.id);
    // The count is at least the entries kept.
    return Object.freeze({ id: n.id, type, heading: headingOr(n.heading, n.id), entries: Object.freeze(entries), totalCount: Math.max(count(n.totalCount) ? n.totalCount : 0, entries.length), nextCursor: (n.nextCursor ?? '') as string, ...(typeof n.start === 'number' && Number.isSafeInteger(n.start) && (n.start as number) >= 0 ? { start: n.start as number } : {}), ...optional('seeAll', seeAll(n.seeAll)) });
  }), n => n.id);
  // A page none of whose sections could be read is refused rather than shown as empty.
  if (!sections.length && (raw.sections as unknown[]).length) invalid();
  let entity: ContentEntry | undefined;
  // The page's own entity is exact (it is what was asked for); it never plays, so playback on it is ignored.
  if(raw.entity!==undefined && raw.entity!==null && !unknownContentEntryKind(raw.entity)){entity=entry(raw.entity);if(!['artist','album','book','disc','author','book_series'].includes(expected.view)||entity.kind!==expected.view||entity.id!==expected.entityId||entity.libraryId!==expected.libraryId)invalid();if(entity.playback!==undefined){const{playback:_ignored,...rest}=entity;entity=Object.freeze(rest);}}
  // The listening actions and the empty-state words are optional: unreadable ones are left out.
  let listening: ListeningJourney | undefined;
  if (raw.listening !== undefined && raw.listening !== null) { try { listening = parseListeningJourney(raw.listening, s.libraryId as string); } catch { noteSkipped('content', 'listening'); } }
  let empty: ContentHeading | undefined;
  if (raw.empty !== undefined && raw.empty !== null) { try { empty = heading(raw.empty); } catch { noteSkipped('content', 'empty'); } }
  return Object.freeze({ ...(entity?{entity}:{}), ...(listening?{listening}:{}), heading: headingOr(raw.heading, s.view as string), scope: Object.freeze({ serverId: s.serverId as string, libraryId: s.libraryId as string, libraryKind: s.libraryKind as string, view: s.view as ContentSurface, entityId: s.entityId as string, viewerFence: s.viewerFence as string }), revision: Object.freeze({ catalog: r.catalog, viewer: r.viewer }), query: Object.freeze({ sort: q.sort, direction: q.direction, category: q.category, q: q.q, limit: q.limit, searchMode }), navigation: Object.freeze(navigation), sorts: Object.freeze(sorts), filters: Object.freeze(filters), sections: Object.freeze(sections), ...(empty ? { empty } : {}) });
}
type Page = { cursor: string | null; sectionId?: string };
const seenEntries = new WeakMap<ContentSection, Set<string>>();
const sameEntry = (a: ContentEntry, b: ContentEntry) => a === b || JSON.stringify(a) === JSON.stringify(b);
/** One more page of one section, appended to that section; the other sections stay as they are. */
function appendSectionPage(previous: readonly ContentSection[], page: readonly ContentSection[]): readonly ContentSection[] {
  if (page.length !== 1 || !previous.length) return page;
  const next = page[0], index = previous.findIndex(s => s.id === next.id);
  if (index < 0) return page;
  const current = previous[index];
  // The seen set follows the merged section, so a page keys only its own entries.
  const seen = seenEntries.get(current) ?? new Set(current.entries.map(e => e.id));
  const added: ContentEntry[] = [];
  for (const e of next.entries) if (!seen.has(e.id)) { seen.add(e.id); added.push(e); }
  const merged: ContentSection = Object.freeze({ ...current, entries: added.length ? Object.freeze(current.entries.concat(added)) : current.entries, totalCount: next.totalCount, nextCursor: next.nextCursor });
  seenEntries.set(merged, seen);
  return Object.freeze(previous.map((s, i) => (i === index ? merged : s)));
}
/** A fresh first page merged into what is shown: sections follow the server, unchanged ones keep their identity, paged ones keep their tail. */
function mergeHead(previous: readonly ContentSection[], fresh: readonly ContentSection[]): readonly ContentSection[] {
  const before = new Map(previous.map(s => [s.id, s]));
  let changed = previous.length !== fresh.length;
  const sections = fresh.map((section, index) => {
    const old = before.get(section.id);
    if (!old) { changed = true; return section; }
    const entries = mergeRevalidatedList(old.entries, section.entries, e => e.id, { same: sameEntry, complete: !section.nextCursor });
    const keptTail = entries.length > section.entries.length;
    const nextCursor = keptTail ? old.nextCursor : section.nextCursor;
    const { entries: _a, nextCursor: _b, ...oldRest } = old, { entries: _c, nextCursor: _d, ...freshRest } = section;
    if (entries === old.entries && nextCursor === old.nextCursor && JSON.stringify(oldRest) === JSON.stringify(freshRest)) { if (previous[index] !== old) changed = true; return old; }
    changed = true;
    return Object.freeze({ ...section, entries: entries === old.entries ? entries : Object.freeze([...entries]), nextCursor }) as ContentSection;
  });
  return changed ? Object.freeze(sections) : previous;
}
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
  private quiet?: AbortController;
  private headSerial = 0;
  private snapshot: LibraryContentSnapshot;
  constructor(options: { api: LibraryContentApi; scope: ContentScope; pageSize?: number; timeoutMs?: number }) {
    this.api = options.api; this.bound = scope(options.scope); this.pageSize = options.pageSize ?? 40; this.timeoutMs = options.timeoutMs ?? 15_000;
    if (!Number.isInteger(this.pageSize) || this.pageSize < 1 || this.pageSize > 100 || !Number.isFinite(this.timeoutMs) || this.timeoutMs <= 0 || this.timeoutMs > 120_000) throw new Error('Invalid content request bounds.');
    this.snapshot = this.empty();
  }
  private empty(): LibraryContentSnapshot { return Object.freeze({ generation: this.generation, scope: this.bound, route: null, phase: 'idle', projection: null, sections: Object.freeze([]), appended: Object.freeze([]), pagination: Object.freeze({ cursor: null, canPrevious: false, next: Object.freeze([]) }), error: null }); }
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
  retry(): Promise<void> { this.assertActive(); if (this.snapshot.phase === 'refresh-required') return this.refresh(); return this.operation ? this.load(this.operation) : Promise.resolve(); }
  /**
   * Read the first page again without touching `phase`, and show it in place. What is on screen
   * stays until the answer is in; a failed read rejects and changes nothing. A service that has
   * moved on to a later page keeps that page and publishes the fresh first page as `head`.
   */
  async revalidate(context?: { signal?: AbortSignal; settle?: () => Promise<void> }): Promise<void> {
    this.assertActive();
    const route = this.snapshot.route;
    if (!route || this.snapshot.phase !== 'ready' || !this.snapshot.projection) return;
    this.quiet?.abort(); const controller = new AbortController(); this.quiet = controller;
    const stop = () => controller.abort();
    context?.signal?.addEventListener('abort', stop, { once: true });
    const generation = this.generation, api = this.api, bound = this.bound;
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const query = new URLSearchParams({ view: route.view, limit: String(this.pageSize) });
      for (const field of ['entityId','sort','direction','category','q'] as const) if (route[field] !== undefined) query.set(field, route[field]!);
      const raw = await Promise.race([
        api.request<RouteResponse<"get_libraries_id_content">>(encodedRoutePath("get_libraries_id_content", {"id": encodeURIComponent(route.libraryId)}) + "?" + (query), 'GET', undefined, controller.signal),
        new Promise<never>((_, reject) => { timer = setTimeout(() => { reject(new ContentFailure('timeout', 'The server took too long to respond. Try again.', true)); controller.abort(); }, this.timeoutMs); }),
      ]);
      if (this.disposed || generation !== this.generation || controller.signal.aborted) return;
      const result = projection(raw, route, bound, this.pageSize);
      await context?.settle?.();
      if (this.disposed || generation !== this.generation || controller.signal.aborted || this.snapshot.phase !== 'ready') return;
      // Nothing changed on the first page: nothing is published, so nothing re-renders.
      if (this.current.cursor === null && JSON.stringify(result) === JSON.stringify(this.snapshot.projection)) return;
      const head = Object.freeze({ serial: ++this.headSerial, sections: result.sections });
      const appended = mergeHead(this.snapshot.appended, result.sections);
      // A section that did not change is the same object as before, so a list built on it is left alone.
      const before = new Map(this.snapshot.sections.map(s => [s.id, s]));
      const sections = Object.freeze(result.sections.map(s => { const old = before.get(s.id); return old && JSON.stringify(old) === JSON.stringify(s) ? old : s; }));
      if (this.current.cursor === null) this.publish({ projection: result, sections, appended, head, pagination: Object.freeze({ cursor: null, canPrevious: false, next: Object.freeze(result.sections.filter(s => s.nextCursor).map(s => Object.freeze({ sectionId: s.id, cursor: s.nextCursor }))) }) });
      else this.publish({ head, appended });
    } catch (error) {
      if (this.disposed || generation !== this.generation || controller.signal.aborted) return;
      const e = error as { status?: unknown; code?: unknown } | null;
      // The server answered that this is gone or not allowed: that is the screen's state now, not a failed refresh.
      if (e?.status === 401 || e?.status === 403 || e?.status === 404) { await this.refresh(); return; }
      throw error;
    } finally {
      if (timer) clearTimeout(timer);
      context?.signal?.removeEventListener('abort', stop);
      if (this.quiet === controller) this.quiet = undefined;
    }
  }
  next(sectionId: string): Promise<void> {
    this.assertActive();
    if (this.snapshot.phase !== 'ready' || !this.snapshot.projection || !this.snapshot.route) return Promise.resolve();
    // The section as shown (every page of it read so far), so More works on any section of the page, not only the one paged last.
    const section = this.snapshot.appended.find(s => s.id === sectionId) ?? this.snapshot.sections.find(s => s.id === sectionId);
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
  dispose(): void { this.controller?.abort(); this.quiet?.abort(); this.disposed = true; this.generation++; this.listeners.clear(); }
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
    this.publish({ generation, route: op.route, phase: 'loading', projection: retain ? this.snapshot.projection : null, sections: retain ? this.snapshot.sections : Object.freeze([]), appended: retain ? this.snapshot.appended : Object.freeze([]), error: null, pagination: retain ? this.snapshot.pagination : Object.freeze({ cursor: op.page.cursor, canPrevious: false, next: Object.freeze([]) }) });
    const task = this.fetch(op, api, bound, controller, generation);
    this.pending = task; return task;
  }
  private async fetch(op: Operation, api: LibraryContentApi, bound: ContentScope, controller: AbortController, generation: number): Promise<void> {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let onAbort: (() => void) | undefined;
    const query = new URLSearchParams({ view: op.route.view, limit: String(this.pageSize) });
    for (const field of ['entityId','sort','direction','category','q'] as const) if (op.route[field] !== undefined) query.set(field, op.route[field]!);
    if (op.page.cursor) query.set('cursor', op.page.cursor);
    const path = encodedRoutePath("get_libraries_id_content", {"id": encodeURIComponent(op.route.libraryId)}) + "?" + (query);
    try {
      const cancelled = new Promise<never>((_, reject) => { onAbort = () => reject(new ContentFailure('cancelled', 'Library request cancelled.')); controller.signal.addEventListener('abort', onAbort, { once: true }); });
      const raw = await Promise.race([api.request<unknown>(path, 'GET', undefined, controller.signal), cancelled, new Promise<never>((_, reject) => { timer = setTimeout(() => { reject(new ContentFailure('timeout', 'The server took too long to respond. Try again.', true)); controller.abort(); }, this.timeoutMs); })]);
      if (this.disposed || generation !== this.generation) return;
      const result = projection(raw, op.route, bound, this.pageSize);
      // A continuation read after the library moved is still a valid page (the server re-anchors it); only another viewer's answer is refused.
      if (op.page.cursor && op.fence && result.scope.viewerFence !== op.fence.scope.viewerFence) throw new ContentFailure('stale_continuation', 'The library changed. Refresh to continue.', true);
      if (op.page.sectionId && (result.sections.length !== 1 || result.sections[0].id !== op.page.sectionId)) invalid();
      // A cursor that comes back again would page for ever: that section has reached its end.
      const repeats = (c: string) => c === op.page.cursor || op.history.some(p => p.cursor === c);
      const sections = result.sections.some(s2 => s2.nextCursor && repeats(s2.nextCursor)) ? Object.freeze(result.sections.map(s2 => (s2.nextCursor && repeats(s2.nextCursor) ? Object.freeze({ ...s2, nextCursor: '' }) : s2))) : result.sections;
      this.history = op.history; this.current = op.page;
      this.publish({ phase: 'ready', projection: result, sections, appended: op.page.cursor === null ? sections : appendSectionPage(this.snapshot.appended, sections), pagination: Object.freeze({ cursor: op.page.cursor, canPrevious: this.history.length > 0, next: Object.freeze(sections.filter(s => s.nextCursor).map(s => Object.freeze({ sectionId: s.id, cursor: s.nextCursor }))) }) });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const e = error as { code?: unknown; retryable?: unknown; message?: unknown; status?: unknown } | null;
      // Q5: checked `server/api/portico.openapi.json` — `/v1/content` and
      // `/v1/libraries/{id}/content` accept only `cursor` + `limit` (no
      // `range.start`; that exists only on POST browse). A stale or expired
      // section continuation therefore cannot re-anchor by index client-side;
      // it stays an explicit first-page refresh. `invalid_cursor` on a
      // continuation joins `stale_continuation` there; on a first page it
      // remains a plain error.
      const stale = e?.code === 'stale_continuation' || (!!op.page.cursor && e?.code === 'invalid_cursor');
      // A continuation the server no longer honours is not the person's problem: start the list again from its first page, keeping what is shown until it arrives.
      if (stale && op.page.cursor) { await this.load({ route: op.route, page: { cursor: null }, history: [], fence: null }); return; }
      const code = string(e?.code) && e?.code ? e.code : 'request_failed';
      const transient = !stale && code !== 'invalid_projection' && e?.status !== 401 && e?.status !== 403 && (e?.retryable === true || code === 'timeout' || error instanceof TypeError || typeof e?.status === 'number' && (e.status >= 500 || e.status === 408 || e.status === 429));
      this.publish({ phase: stale ? 'refresh-required' : 'error', projection: transient ? this.snapshot.projection : null, sections: transient ? this.snapshot.sections : Object.freeze([]), appended: transient ? this.snapshot.appended : Object.freeze([]), error: Object.freeze({ code, message: stale ? 'The library changed. Refresh to continue.' : error instanceof Error ? error.message : 'Could not load this library.', retryable: stale || transient }), pagination: Object.freeze({ cursor: op.page.cursor, canPrevious: false, next: Object.freeze([]) }) });
    } finally {
      if (timer) clearTimeout(timer);
      if (onAbort) controller.signal.removeEventListener('abort', onAbort);
      if (generation === this.generation) { this.pending = undefined; this.activeKey = ''; }
    }
  }
}

/** Reuse the canonical strict semantic DTO validation in composed server workspaces. */
export { projection as validateContentProjection };

/** Shared media-entry validation for server-authored cross-library search. It throws for an entry that cannot be shown; callers leave out that entry alone. */
export { entry as validateContentEntry };

/** A See all destination (`{pivot, query?, sort}`), or undefined when it is absent or cannot be read. Never throws. */
export { seeAll as parseContentSeeAll };

/** A bounded server ranking, read lazily when its title/artist page asks for it. */
export async function readListeningSimilar(api:LibraryContentApi,scope:import('./queues.ts').QueueScope,itemId:string,options:{limit?:number;by?:'sound';signal?:AbortSignal}={}){
 const q=new URLSearchParams();if(options.limit!==undefined)q.set('limit',String(options.limit));if(options.by!==undefined){if(options.by!=='sound')invalid();q.set('by',options.by);}
 const raw=listeningData(await api.request(encodedRoutePath('get_items_id_similar',{id:encodeURIComponent(itemId)})+(q.size?'?'+q:''),'GET',undefined,options.signal),scope);
 if(!record(raw)||raw.itemId!==itemId||!id(raw.libraryId)||!record(raw.revision)||!count(raw.revision.catalog)||!count(raw.revision.viewer)||typeof raw.bySound!=='boolean'||!Array.isArray(raw.entries)||raw.entries.length>24)invalid();
 const entries=raw.entries.map(entry);if(entries.some(e=>!['artist','album','song','book'].includes(e.kind)||raw.bySound&&e.kind==='book'||e.playback!==undefined))invalid();
 return Object.freeze({itemId,libraryId:raw.libraryId,bySound:raw.bySound,revision:Object.freeze({catalog:raw.revision.catalog,viewer:raw.revision.viewer}),entries:Object.freeze(entries)});
}
