import {unreadableServerResponse} from './server-messages.ts';
/** Server-composed home rows. Clients render membership, order and policy as published; they never rank, hide or page a row themselves. */
import {validateContentEntry, type ContentEntry} from './library-content.ts';
import {parseBrowseNode, type BrowseNode, type BrowseSortSelection} from './browse.ts';
import {firstOfEach, keepReadable, noteSkipped, oneOfOr} from './tolerant.ts';

export type HomeArtworkShape = 'poster' | 'square' | 'landscape';
export type HomeRevision = Readonly<{catalog: number; viewer: number}>;
/** A server-authored label (CON-19): a catalogue message id, its parameters, and the US English
 * fallback for a code this client doesn't know. Name rows with `serverTextLabel`. */
export type ServerText = Readonly<{code: string; params?: Readonly<Record<string, string>>; fallback: string}>;
export type HomeSeeAll = Readonly<
 {type:'library';libraryId:string;pivot:string;query?:BrowseNode;sort:readonly BrowseSortSelection[]} |
 {type:'row';rowId:string} | {type:'saved_view';resourceId:string}
>;
export type HomeRow = Readonly<{
  id: string; title: string; titleText?: ServerText; kind: string; artworkShape: HomeArtworkShape; explanation?: string; endpoint: string;
  /** `for_you` on the personal rows: the layout arranges and hides them together as the row `for_you`. */
  family?: string;
  seeAll?: HomeSeeAll;
  libraryId?: string; privacySensitivity: string; policyState: string;
  relation?: string; provider?: string; evidenceId?: string;
  priority: number;
  /** How long the server would let the row be reused. No client reads it; absent when the server does not send it. */
  cacheTtlSeconds?: number;
  required: boolean; hideable: boolean; reorderable: boolean; critical: boolean; cursorCapable: boolean;
  entries: readonly ContentEntry[]; total: number; start: number; limit: number; hasMore: boolean;
  nextCursor: string; anchorId?: string; revision: HomeRevision;
}>;
export type HomeLayout = Readonly<{revision: number; rowOrder: readonly string[]; hiddenRowIds: readonly string[]}>;
export type HomeLayoutRowView = Readonly<{id: string; title: string; titleText?: ServerText; kind: string; artworkShape: HomeArtworkShape; libraryId?: string; required: boolean; hideable: boolean; reorderable: boolean; hidden: boolean}>;
export type HomeLayoutView = Readonly<HomeLayout & {rows: readonly HomeLayoutRowView[]}>;
/** The entry Home opens with: the first entry of one of its rows (the server chooses which). */
export type HomeHeroRef = Readonly<{rowId: string; entryId: string}>;
export type HomeDocument = Readonly<{
  serverId: string; viewerFence: string; rows: readonly HomeRow[]; hero?: HomeHeroRef; layout: HomeLayout; revision: HomeRevision; generatedAt: string;
}>;
export type HomeRowPage = Readonly<{limit?: number; cursor?: string; start?: number; revision?: string; anchorId?: string}>;
export type HomeLayoutIntent = Readonly<{expectedRevision: number; rowOrder: readonly string[]; hiddenRowIds: readonly string[]; idempotencyKey?: string}>;
export interface HomeApi {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}

const shapes = ['poster', 'square', 'landscape'] as const;
const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const id = (v: unknown): v is string => text(v, 256) && (v as string).length > 0;
const whole = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;

function invalid(): never {
  throw Object.assign(new Error(unreadableServerResponse), {code: 'invalid_home', retryable: false});
}

/**
 * A ServerText, or undefined when absent. Text whose code or fallback cannot be read is refused;
 * parameters that cannot be read are left out, and the fallback then names the row.
 */
export function parseServerText(value: unknown): ServerText | undefined {
  if (value === undefined || value === null) return undefined;
  if (!object(value) || !text(value.code, 128) || !text(value.fallback, 1024)) invalid();
  if (value.params === undefined || value.params === null) return Object.freeze({code: value.code, fallback: value.fallback});
  if (!object(value.params) || Object.keys(value.params).length > 16) { noteSkipped('home', 'titleText.params'); return Object.freeze({code: value.code, fallback: value.fallback}); }
  const params: Record<string, string> = {};
  for (const [name, param] of Object.entries(value.params)) {
    if (!id(name) || !text(param, 1024)) { noteSkipped('home', 'titleText.params'); continue; }
    params[name] = param;
  }
  return Object.freeze({code: value.code, params: Object.freeze(params), fallback: value.fallback});
}

/** A row's title text, left out (the plain title then names the row) when it cannot be read. */
function titleTextOf(value: unknown): ServerText | undefined {
  try { return parseServerText(value); } catch { noteSkipped('home', 'titleText'); return undefined; }
}
/** Text a row is drawn with: absent reads as empty, and text over its bound refuses the row. */
function words(value: unknown, max: number): string {
  if (value === undefined || value === null) return '';
  if (!text(value, max)) invalid();
  return value;
}

function revision(value: unknown): HomeRevision {
  if (!object(value) || !whole(value.catalog) || !whole(value.viewer)) invalid();
  return Object.freeze({catalog: value.catalog, viewer: value.viewer});
}

export function validateHomeSeeAll(value:unknown):HomeSeeAll {
 if(!object(value))invalid();
 if(value.type==='row' && id(value.rowId))return Object.freeze({type:'row',rowId:value.rowId});
 if(value.type==='saved_view' && id(value.resourceId))return Object.freeze({type:'saved_view',resourceId:value.resourceId});
 if(value.type!=='library'||!id(value.libraryId)||!id(value.pivot)||!Array.isArray(value.sort)||value.sort.length<1||value.sort.length>3)invalid();
 const sort=value.sort.map(s=>{if(!object(s)||!id(s.field)||(s.direction!=='asc'&&s.direction!=='desc'))invalid();return Object.freeze({field:s.field,direction:s.direction}) as BrowseSortSelection;});
 return Object.freeze({type:'library',libraryId:value.libraryId,pivot:value.pivot,sort:Object.freeze(sort),...(value.query===undefined?{}:{query:parseBrowseNode(value.query)})});
}

/**
 * One Home row. Its id, cursor, revision and the bound on its entries are exact. Everything it is
 * only drawn with has a neutral reading: an artwork shape this app does not know is a poster, a
 * missing flag is off, a missing count is 0, and an entry that cannot be read is left out alone.
 */
export function validateHomeRow(value: unknown): HomeRow {
  if (!object(value) || !id(value.id)) invalid();
  if (!Array.isArray(value.entries) || value.entries.length > 200 || !text(value.nextCursor ?? '', 4096)) invalid();
  const entries = firstOfEach('home', 'entries', keepReadable('home', 'entries', value.entries, entry => validateContentEntry(entry)), entry => entry.id);
  const flag = (name: string) => value[name] === true;
  const amount = (name: string) => (whole(value[name]) ? value[name] as number : 0);
  const required = flag('required');
  const row: Record<string, unknown> = {
    id: value.id, title: words(value.title, 1024), kind: words(value.kind, 256),
    artworkShape: oneOfOr('home', 'row.artworkShape', value.artworkShape, shapes, 'poster'), endpoint: words(value.endpoint, 256),
    privacySensitivity: words(value.privacySensitivity, 256), policyState: words(value.policyState, 256), priority: amount('priority'),
    // A required row is never hideable, whatever the two flags say together.
    required, hideable: flag('hideable') && !required,
    reorderable: flag('reorderable'), critical: flag('critical'), cursorCapable: flag('cursorCapable'),
    // The total is at least the entries kept.
    entries: Object.freeze(entries), total: Math.max(amount('total'), entries.length), start: amount('start'), limit: amount('limit'),
    hasMore: flag('hasMore'), nextCursor: value.nextCursor ?? '', revision: revision(value.revision),
  };
  if (whole(value.cacheTtlSeconds)) row.cacheTtlSeconds = value.cacheTtlSeconds;
  const titleText = titleTextOf(value.titleText);
  if (titleText) row.titleText = titleText;
  for (const field of ['explanation', 'libraryId', 'relation', 'provider', 'evidenceId', 'anchorId', 'family'] as const) {
    if (text(value[field], 1024)) row[field] = value[field];
    else if (value[field] !== undefined && value[field] !== null) noteSkipped('home', 'row.' + field);
  }
  if (value.seeAll !== undefined && value.seeAll !== null) {
    try { row.seeAll = validateHomeSeeAll(value.seeAll); } catch { noteSkipped('home', 'row.seeAll'); }
  }
  return Object.freeze(row) as HomeRow;
}

/** A row as a content section heading: its title code and params when it has them (`sectionHeading`,
 * `serverTextLabel` name it), its id and title otherwise. */
export function homeRowHeading(row: Pick<HomeRow, 'id' | 'title' | 'titleText'>): Readonly<{key: string; fallback: string; params?: Readonly<Record<string, string>>}> {
  const code = row.titleText?.code;
  if (!code) return {key: row.id, fallback: row.title};
  return row.titleText?.params ? {key: code, fallback: row.title, params: row.titleText.params} : {key: code, fallback: row.title};
}

/**
 * The personal rows (Recommendations P2): (`for_you:<generator>[:<key>]`) are one
 * family in the layout, arranged and hidden together as the row `for_you`. Customize Home lists the
 * family, never a generator.
 */
export const PERSONAL_FAMILY = 'for_you';
export const isPersonalRowId = (rowId: string) => rowId.startsWith(PERSONAL_FAMILY + ':');

/**
 * Saved views on Home (Recommendations P6): the layout order may name `view:<savedResourceId>`;
 * Home then shows that view as a row (kind `saved_view`), under its name, in its own sort. A
 * viewer adds one by appending its id to the order and removes it by dropping the id; a view they
 * can no longer read is simply absent.
 */
export const HOME_VIEW_PREFIX = 'view:';
export const homeViewRowId = (resourceId: string) => HOME_VIEW_PREFIX + resourceId;
/** The saved view a Home row shows, or undefined for any other row. */
export const homeViewResourceId = (rowId: string): string | undefined => (rowId.startsWith(HOME_VIEW_PREFIX) && rowId.length > HOME_VIEW_PREFIX.length ? rowId.slice(HOME_VIEW_PREFIX.length) : undefined);

/** The saved layout. Its revision and list bounds are exact; an entry that is not an id, or a repeated id, is left out. */
export function validateHomeLayout(value: unknown): HomeLayout {
  if (!object(value) || !whole(value.revision) || !Array.isArray(value.rowOrder) || !Array.isArray(value.hiddenRowIds)) invalid();
  if (value.rowOrder.length > 64 || value.hiddenRowIds.length > 64) invalid();
  const ids = (part: string, list: unknown[]) => firstOfEach('home', part, keepReadable('home', part, list, entry => (id(entry) ? entry : undefined)), entry => entry);
  return Object.freeze({revision: value.revision, rowOrder: Object.freeze(ids('layout.rowOrder', value.rowOrder)), hiddenRowIds: Object.freeze(ids('layout.hiddenRowIds', value.hiddenRowIds))});
}

export function validateHomeLayoutView(value: unknown): HomeLayoutView {
  const layout = validateHomeLayout(value);
  if (!object(value) || !Array.isArray(value.rows) || value.rows.length > 64) invalid();
  // A row that cannot be read is left out of Customize Home alone; contradictory flags take the reading that keeps a row visible.
  const rows = firstOfEach('home', 'layout.rows', keepReadable('home', 'layout.rows', value.rows as unknown[], entry => {
    if (!object(entry) || !id(entry.id)) invalid();
    const row = entry as Record<string, unknown>;
    const required = row.required === true, hideable = row.hideable === true && !required;
    const titleText = titleTextOf(row.titleText);
    return Object.freeze({
      id: row.id as string, title: words(row.title, 1024), ...(titleText ? {titleText} : {}), kind: words(row.kind, 256), artworkShape: oneOfOr('home', 'layout.artworkShape', row.artworkShape, shapes, 'poster'),
      ...(id(row.libraryId) ? {libraryId: row.libraryId} : {}),
      required, hideable, reorderable: row.reorderable === true, hidden: row.hidden === true && hideable,
    });
  }), row => row.id);
  return Object.freeze({...layout, rows: Object.freeze(rows)});
}

/**
 * Home. The server, the viewer fence, the layout, the revision and the bound on rows are exact.
 * A row that cannot be read is left out alone, a repeated row keeps its first appearance, and a
 * row the viewer hid or an empty row that is not critical is not shown.
 */
export function validateHomeDocument(value: unknown): HomeDocument {
  if (!object(value) || !id(value.serverId) || !id(value.viewerFence) || !Array.isArray(value.rows) || value.rows.length > 64) invalid();
  const layout = validateHomeLayout(value.layout);
  const readable = keepReadable('home', 'rows', value.rows, raw => {
    const row = validateHomeRow(raw);
    // An entry of a kind this app does not know leaves its row in place; a row the server sent empty is kept only when critical.
    return {row, shown: !layout.hiddenRowIds.includes(row.id) && ((raw as {entries: unknown[]}).entries.length > 0 || row.critical)};
  });
  // A Home none of whose rows could be read is refused rather than shown as empty.
  if (!readable.length && value.rows.length) invalid();
  const rows = firstOfEach('home', 'rows', readable.filter(read => read.shown).map(read => read.row), row => row.id);
  let hero: HomeHeroRef | undefined;
  // A hero that cannot be read, or whose entry this client cannot draw (a kind it does not know), is simply absent.
  if (object(value.hero) && id(value.hero.rowId) && id(value.hero.entryId)) {
    if (rows.find(row => row.id === (value.hero as HomeHeroRef).rowId)?.entries.some(entry => entry.id === (value.hero as HomeHeroRef).entryId)) hero = Object.freeze({rowId: value.hero.rowId, entryId: value.hero.entryId});
  } else if (value.hero !== undefined && value.hero !== null) noteSkipped('home', 'hero');
  return Object.freeze({serverId: value.serverId, viewerFence: value.viewerFence, rows: Object.freeze(rows), ...(hero ? {hero} : {}), layout, revision: revision(value.revision), generatedAt: words(value.generatedAt, 128)});
}

function pageQuery(page: HomeRowPage): string {
  const query = new URLSearchParams();
  if (page.limit !== undefined) query.set('limit', String(page.limit));
  if (page.cursor) query.set('cursor', page.cursor);
  if (page.start !== undefined) query.set('start', String(page.start));
  if (page.revision) query.set('revision', page.revision);
  if (page.anchorId) query.set('anchorId', page.anchorId);
  const text = query.toString();
  return text ? '?' + text : '';
}

export async function fetchHome(api: HomeApi, limit?: number, signal?: AbortSignal): Promise<HomeDocument> {
  return validateHomeDocument(await api.request<unknown>('/v1/home' + (limit === undefined ? '' : '?limit=' + limit), 'GET', undefined, signal));
}

export async function fetchHomeLayout(api: HomeApi, signal?: AbortSignal): Promise<HomeLayoutView> {
  return validateHomeLayoutView(await api.request<unknown>('/v1/home/layout', 'GET', undefined, signal));
}

/**
 * PERF-S14: Home is a summary, so a row paged sideways keeps at most this many entries on the device
 * (never O(row)). Past it the row stops offering more; the row's own destination (See all: the
 * library, Saved…) is the full, windowed view.
 */
export const HOME_ROW_MAX_ENTRIES = 100;

/** A row after one more page: new entries appended (duplicates dropped), capped at `max`. */
export function appendHomeRowPage(row: HomeRow, page: HomeRow, max = HOME_ROW_MAX_ENTRIES): HomeRow {
  const seen = new Set(row.entries.map(e => e.id));
  const entries = [...row.entries, ...page.entries.filter(e => !seen.has(e.id))].slice(0, max);
  const capped = entries.length >= max;
  return Object.freeze({...row, entries, nextCursor: capped ? '' : page.nextCursor, hasMore: page.hasMore, total: page.total});
}

export async function fetchHomeRow(api: HomeApi, rowId: string, page: HomeRowPage = {}, signal?: AbortSignal): Promise<HomeRow> {
  if (page.cursor && (page.start !== undefined || page.anchorId)) throw new Error('A home row pages by cursor or by anchored range, not both.');
  const row = validateHomeRow(await api.request<unknown>('/v1/home/rows/' + encodeURIComponent(rowId) + pageQuery(page), 'GET', undefined, signal));
  if (row.id !== rowId) invalid();
  return row;
}

export async function saveHomeLayout(api: HomeApi, intent: HomeLayoutIntent, signal?: AbortSignal): Promise<HomeLayout> {
  return validateHomeLayout(await api.request<unknown>('/v1/home/layout', 'PUT', {
    expectedRevision: intent.expectedRevision, idempotencyKey: intent.idempotencyKey ?? '',
    rowOrder: [...intent.rowOrder], hiddenRowIds: [...intent.hiddenRowIds],
  }, signal));
}

export async function resetHomeLayout(api: HomeApi, idempotencyKey = '', signal?: AbortSignal): Promise<HomeLayout> {
  return validateHomeLayout(await api.request<unknown>('/v1/home/layout/reset', 'POST', {idempotencyKey}, signal));
}
