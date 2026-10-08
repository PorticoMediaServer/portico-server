import {unreadableServerResponse} from './server-messages.ts';
/** Server-composed home rows. Clients render membership, order and policy as published; they never rank, hide or page a row themselves. */
import {unknownContentEntryKind, validateContentEntry, type ContentEntry} from './library-content.ts';

export type HomeArtworkShape = 'poster' | 'square' | 'landscape';
export type HomeRevision = Readonly<{catalog: number; viewer: number}>;
/** A server-authored label (CON-19): a catalogue message id, its parameters, and the US English
 * fallback for a code this client doesn't know. Name rows with `serverTextLabel`. */
export type ServerText = Readonly<{code: string; params?: Readonly<Record<string, string>>; fallback: string}>;
export type HomeRow = Readonly<{
  id: string; title: string; titleText?: ServerText; kind: string; artworkShape: HomeArtworkShape; explanation?: string; endpoint: string;
  /** `for_you` on the personal rows: the layout arranges and hides them together as the row `for_you`. */
  family?: string;
  libraryId?: string; privacySensitivity: string; policyState: string;
  relation?: string; provider?: string; evidenceId?: string;
  priority: number; cacheTtlSeconds: number;
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

const shapes = ['poster', 'square', 'landscape'];
const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const id = (v: unknown): v is string => text(v, 256) && (v as string).length > 0;
const whole = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;

function invalid(): never {
  throw Object.assign(new Error(unreadableServerResponse), {code: 'invalid_home', retryable: false});
}

/** A ServerText, or undefined when absent. Malformed text is refused like any other field. */
export function parseServerText(value: unknown): ServerText | undefined {
  if (value === undefined) return undefined;
  if (!object(value) || !text(value.code, 128) || !text(value.fallback, 1024)) invalid();
  if (value.params === undefined) return Object.freeze({code: value.code, fallback: value.fallback});
  if (!object(value.params) || Object.keys(value.params).length > 16) invalid();
  const params: Record<string, string> = {};
  for (const [name, param] of Object.entries(value.params)) {
    if (!id(name) || !text(param, 1024)) invalid();
    params[name] = param;
  }
  return Object.freeze({code: value.code, params: Object.freeze(params), fallback: value.fallback});
}

function revision(value: unknown): HomeRevision {
  if (!object(value) || !whole(value.catalog) || !whole(value.viewer)) invalid();
  return Object.freeze({catalog: value.catalog, viewer: value.viewer});
}

export function validateHomeRow(value: unknown): HomeRow {
  if (!object(value) || !id(value.id) || !text(value.title, 1024) || !id(value.kind) || !shapes.includes(value.artworkShape as string)) invalid();
  if (!id(value.endpoint) || !id(value.privacySensitivity) || !id(value.policyState)) invalid();
  if (!whole(value.priority) || !whole(value.cacheTtlSeconds) || !whole(value.total) || !whole(value.start) || !whole(value.limit)) invalid();
  for (const flag of ['required', 'hideable', 'reorderable', 'critical', 'cursorCapable', 'hasMore'] as const) {
    if (typeof value[flag] !== 'boolean') invalid();
  }
  if (value.required && value.hideable) invalid();
  if (!Array.isArray(value.entries) || value.entries.length > 200 || !text(value.nextCursor, 4096)) invalid();
  const entries = value.entries.filter(entry => !unknownContentEntryKind(entry)).map(entry => validateContentEntry(entry));
  if (new Set(entries.map(entry => entry.id)).size !== entries.length) invalid();
  if (entries.length > value.total) invalid();
  const row: Record<string, unknown> = {
    id: value.id, title: value.title, kind: value.kind, artworkShape: value.artworkShape, endpoint: value.endpoint,
    privacySensitivity: value.privacySensitivity, policyState: value.policyState, priority: value.priority,
    cacheTtlSeconds: value.cacheTtlSeconds, required: value.required, hideable: value.hideable,
    reorderable: value.reorderable, critical: value.critical, cursorCapable: value.cursorCapable,
    entries: Object.freeze(entries), total: value.total, start: value.start, limit: value.limit,
    hasMore: value.hasMore, nextCursor: value.nextCursor, revision: revision(value.revision),
  };
  const titleText = parseServerText(value.titleText);
  if (titleText) row.titleText = titleText;
  for (const field of ['explanation', 'libraryId', 'relation', 'provider', 'evidenceId', 'anchorId', 'family'] as const) {
    if (value[field] !== undefined) {
      if (!text(value[field], 1024)) invalid();
      row[field] = value[field];
    }
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

export function validateHomeLayout(value: unknown): HomeLayout {
  if (!object(value) || !whole(value.revision) || !Array.isArray(value.rowOrder) || !Array.isArray(value.hiddenRowIds)) invalid();
  if (value.rowOrder.length > 64 || value.hiddenRowIds.length > 64) invalid();
  const order = value.rowOrder.map(entry => {if (!id(entry)) invalid(); return entry;});
  const hidden = value.hiddenRowIds.map(entry => {if (!id(entry)) invalid(); return entry;});
  if (new Set(order).size !== order.length || new Set(hidden).size !== hidden.length) invalid();
  return Object.freeze({revision: value.revision, rowOrder: Object.freeze(order), hiddenRowIds: Object.freeze(hidden)});
}

export function validateHomeLayoutView(value: unknown): HomeLayoutView {
  const layout = validateHomeLayout(value);
  if (!object(value) || !Array.isArray(value.rows) || value.rows.length > 64) invalid();
  const rows = (value.rows as unknown[]).map(entry => {
    if (!object(entry) || !id(entry.id) || !text(entry.title, 1024) || !id(entry.kind) || !shapes.includes(entry.artworkShape as string)) invalid();
    for (const flag of ['required', 'hideable', 'reorderable', 'hidden'] as const) {
      if (typeof (entry as Record<string, unknown>)[flag] !== 'boolean') invalid();
    }
    const row = entry as Record<string, unknown>;
    if (row.required && row.hideable) invalid();
    if (row.hidden && (row.required || !row.hideable)) invalid();
    if (row.libraryId !== undefined && !id(row.libraryId)) invalid();
    const titleText = parseServerText(row.titleText);
    return Object.freeze({
      id: row.id as string, title: row.title as string, ...(titleText ? {titleText} : {}), kind: row.kind as string, artworkShape: row.artworkShape as HomeArtworkShape,
      ...(row.libraryId === undefined ? {} : {libraryId: row.libraryId as string}),
      required: row.required as boolean, hideable: row.hideable as boolean, reorderable: row.reorderable as boolean, hidden: row.hidden as boolean,
    });
  });
  if (new Set(rows.map(r => r.id)).size !== rows.length) invalid();
  return Object.freeze({...layout, rows: Object.freeze(rows)});
}

export function validateHomeDocument(value: unknown): HomeDocument {
  if (!object(value) || !id(value.serverId) || !id(value.viewerFence) || !Array.isArray(value.rows) || value.rows.length > 64) invalid();
  if (!text(value.generatedAt, 128) || !value.generatedAt) invalid();
  const rows = value.rows.map(row => validateHomeRow(row));
  if (new Set(rows.map(row => row.id)).size !== rows.length) invalid();
  const layout = validateHomeLayout(value.layout);
  // The server never publishes a row the viewer hid, nor an empty row that is not critical.
  for (const [index, row] of rows.entries()) {
    if (layout.hiddenRowIds.includes(row.id) || (row.entries.length === 0 && !(value.rows[index] as {entries: unknown[]}).entries.length && !row.critical)) invalid();
  }
  let hero: HomeHeroRef | undefined;
  if (value.hero !== undefined) {
    if (!object(value.hero) || !id(value.hero.rowId) || !id(value.hero.entryId)) invalid();
    // A hero whose entry this client cannot draw (a kind it does not know) is simply absent.
    if (rows.find(row => row.id === (value.hero as HomeHeroRef).rowId)?.entries.some(entry => entry.id === (value.hero as HomeHeroRef).entryId)) hero = Object.freeze({rowId: value.hero.rowId, entryId: value.hero.entryId});
  }
  return Object.freeze({serverId: value.serverId, viewerFence: value.viewerFence, rows: Object.freeze(rows), ...(hero ? {hero} : {}), layout, revision: revision(value.revision), generatedAt: value.generatedAt});
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
