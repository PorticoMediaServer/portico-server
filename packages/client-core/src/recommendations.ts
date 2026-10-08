import {unreadableServerResponse} from './server-messages.ts';
/** Viewer suggestions and per-item related rows. Reasons, ranking and row shape are server-authored. */
import {validateContentEntry, type ContentEntry} from './library-content.ts';
import {validateHomeRow, type HomeApi, type HomeRevision, type HomeRow} from './home.ts';

export type Suggestion = Readonly<{entry: ContentEntry; reason: string; source: string; score: number}>;
export type Suggestions = Readonly<{items: readonly Suggestion[]; total: number; revision: HomeRevision; generatedAt: string}>;
export type ItemRecommendations = Readonly<{itemId: string; rows: readonly HomeRow[]; revision: HomeRevision}>;

const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length > 0 && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const whole = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0;

function invalid(): never {
  throw Object.assign(new Error(unreadableServerResponse), {code: 'invalid_recommendations', retryable: false});
}

function revision(value: unknown): HomeRevision {
  if (!object(value) || !whole(value.catalog) || !whole(value.viewer)) invalid();
  return Object.freeze({catalog: value.catalog, viewer: value.viewer});
}

export function validateSuggestions(value: unknown): Suggestions {
  if (!object(value) || !Array.isArray(value.items) || value.items.length > 100 || !whole(value.total) || !text(value.generatedAt, 128)) invalid();
  const items = value.items.map(raw => {
    if (!object(raw) || !text(raw.reason, 1024) || !text(raw.source, 128)) invalid();
    if (typeof raw.score !== 'number' || !Number.isFinite(raw.score) || raw.score <= 0 || raw.score > 1) invalid();
    return Object.freeze({entry: validateContentEntry(raw.entry), reason: raw.reason, source: raw.source, score: raw.score});
  });
  if (new Set(items.map(item => item.entry.id)).size !== items.length || items.length !== value.total) invalid();
  return Object.freeze({items: Object.freeze(items), total: value.total, revision: revision(value.revision), generatedAt: value.generatedAt});
}

export function validateItemRecommendations(value: unknown, itemId: string): ItemRecommendations {
  if (!object(value) || value.itemId !== itemId || !Array.isArray(value.rows) || value.rows.length > 12) invalid();
  const rows = value.rows.map(row => validateHomeRow(row));
  if (new Set(rows.map(row => row.id)).size !== rows.length) invalid();
  for (const row of rows) {
    // Related rows are always explained by a relation and never echo their own item.
    if (!row.relation || row.entries.length === 0 || row.entries.some(entry => entry.id === itemId)) invalid();
  }
  return Object.freeze({itemId, rows: Object.freeze(rows), revision: revision(value.revision)});
}

/**
 * A title page's recommendation rows (Recommendations P4): every row the endpoint returned, in
 * the server's order (by `priority`): the engine's rows first (More like X, Starring, From its
 * creator, Viewers also watched), then the director, the genre and person rows and the
 * collections. Nothing is filtered or reordered here; an empty row has nothing to show. Name each
 * with its `titleText`.
 */
export function titleRecommendationRows(recommendations: Readonly<{rows: readonly HomeRow[]}> | null | undefined): readonly HomeRow[] {
  return (recommendations?.rows ?? []).filter(row => row.entries.length > 0);
}

/**
 * A movie's "More like this" row from its recommendations: the library-local
 * similar-titles row (relation `more_like`, formerly `because_you_watched`) under the given
 * heading, else the server's first row under its own title; undefined when
 * there is nothing to show.
 */
export function moreLikeThisRow(recommendations: ItemRecommendations | null | undefined, heading: string): {id: string; title: string; entries: readonly ContentEntry[]} | undefined {
  const rows = recommendations?.rows ?? [];
  const similar = rows.find(row => row.relation === 'more_like' || row.relation === 'because_you_watched');
  const row = similar ?? rows[0];
  if (!row || !row.entries.length) return undefined;
  return {id: row.id, title: similar ? heading : row.title, entries: row.entries};
}

export type ShowRecommendations = Readonly<{showId: string; rows: readonly HomeRow[]; revision: HomeRevision}>;

/** A show page's rows (`GET /v1/shows/{id}/recommendations`): More like the show, From its creator… */
export function validateShowRecommendations(value: unknown, showId: string): ShowRecommendations {
  if (!object(value) || value.showId !== showId || !Array.isArray(value.rows) || value.rows.length > 12) invalid();
  const rows = value.rows.map(row => validateHomeRow(row));
  if (new Set(rows.map(row => row.id)).size !== rows.length) invalid();
  for (const row of rows) {
    if (!row.relation || row.entries.length === 0 || row.entries.some(entry => entry.id === showId)) invalid();
  }
  return Object.freeze({showId, rows: Object.freeze(rows), revision: revision(value.revision)});
}

export async function fetchShowRecommendations(api: HomeApi, showId: string, limit?: number, signal?: AbortSignal): Promise<ShowRecommendations> {
  const path = '/v1/shows/' + encodeURIComponent(showId) + '/recommendations' + (limit === undefined ? '' : '?limit=' + limit);
  return validateShowRecommendations(await api.request<unknown>(path, 'GET', undefined, signal), showId);
}

export async function fetchSuggestions(api: HomeApi, limit?: number, signal?: AbortSignal): Promise<Suggestions> {
  return validateSuggestions(await api.request<unknown>('/v1/suggestions' + (limit === undefined ? '' : '?limit=' + limit), 'GET', undefined, signal));
}

export async function fetchItemRecommendations(api: HomeApi, itemId: string, limit?: number, signal?: AbortSignal): Promise<ItemRecommendations> {
  const path = '/v1/items/' + encodeURIComponent(itemId) + '/recommendations' + (limit === undefined ? '' : '?limit=' + limit);
  return validateItemRecommendations(await api.request<unknown>(path, 'GET', undefined, signal), itemId);
}
