import type {DetailApi, DetailScope} from './detail.ts';
import {MediaSelectionService} from './media-selection.ts';
import type {SegmentInput} from './playback-v1/queue.ts';
import type {QueueHeader} from './playback-v1/types.ts';
import type {V1QueuePlayer} from './playback-v1/v1-queue-player.ts';
import type {SelectionItem} from './selection-model/types.ts';
import {checkedItem} from './selection-model/validation.ts';

/** The caller captures its own source continuity and principal/route authority.
 * Search/Saved fences and opaque Browse revisions remain in their own domains. */
export type CatalogueSelectionLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;
const unavailable = () => Object.assign(new Error('catalogue-selection-unavailable'), {code: 'catalogue-selection-unavailable'});
function requireCurrent(lifetime: CatalogueSelectionLifetime): void {
  if (lifetime.signal.aborted || !lifetime.isCurrent()) throw Object.assign(new Error('cancelled'), {name: 'AbortError', code: 'cancelled'});
}

/** Loaded leaf personal actions use the existing revision/idempotency owner.
 * Retain this binding for retries; aggregates stay unavailable, never expanded
 * into a different parent My List/Favorite intent. Construction performs no I/O. */
export function createCataloguePersonalSelection(options: Readonly<{
  api: DetailApi; scope: DetailScope; requestId(): Promise<string>;
}> & CatalogueSelectionLifetime): Readonly<{service: MediaSelectionService; dispose(): void}> {
  const {api: capturedApi, requestId, signal, isCurrent} = options;
  const scope = Object.freeze({...options.scope});
  let disposed = false;
  const lifetime = {signal, isCurrent: () => !disposed && isCurrent()};
  requireCurrent(lifetime);
  const api: DetailApi = {request: async<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> => {
    requireCurrent(lifetime);
    const value = await capturedApi.request<T>(path, method, body, signal);
    requireCurrent(lifetime);
    return value;
  }};
  const service = new MediaSelectionService({api, scope, requestId: async () => {
    requireCurrent(lifetime);
    const id = await requestId();
    requireCurrent(lifetime);
    return id;
  }});
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    signal.removeEventListener('abort', dispose);
    service.dispose();
  };
  signal.addEventListener('abort', dispose, {once: true});
  return Object.freeze({service, dispose});
}

/** Input is the bounded explicit selection, already in display order for Play
 * or pick order for queue actions. Membership expands only on the server. */
export function catalogueSelectionSegments(items: readonly SelectionItem[], capability: 'play' | 'queue' = 'play'): readonly SegmentInput[] {
  if (!items.length || !['play', 'queue'].includes(capability)) throw unavailable();
  const segments: SegmentInput[] = [], seen = new Set<string>();
  for (const raw of items) {
    const item = checkedItem(raw);
    if (!item.capabilities[capability] || item.kind === 'person' || item.kind === 'channel') throw unavailable();
    const key = JSON.stringify([item.libraryId, item.kind, item.id]);
    if (seen.has(key)) throw unavailable();
    seen.add(key);
    if (['show', 'season', 'album', 'artist', 'book', 'collection', 'playlist'].includes(item.kind)) {
      segments.push(Object.freeze({source: Object.freeze({container: Object.freeze({kind: item.kind, id: item.id})})}));
    } else if (['movie', 'episode', 'song', 'audiobook_file', 'extra'].includes(item.kind)) {
      const previous = segments.at(-1);
      if (previous && 'items' in previous.source && previous.source.items.ids.length < 500) {
        segments[segments.length - 1] = Object.freeze({source: Object.freeze({items: Object.freeze({ids: Object.freeze([...previous.source.items.ids, item.id])})})});
      } else segments.push(Object.freeze({source: Object.freeze({items: Object.freeze({ids: Object.freeze([item.id])})})}));
    } else throw unavailable();
  }
  return Object.freeze(segments);
}

/** The real player retains this original lifetime through queue POST/adoption.
 * A QueueHeader is the receipt; no visible-page queue or fake success. */
export async function playCatalogueSelection(player: Pick<V1QueuePlayer, 'playSegments'>, items: readonly SelectionItem[], lifetime: CatalogueSelectionLifetime): Promise<QueueHeader> {
  const captured = Object.freeze({signal: lifetime.signal, isCurrent: lifetime.isCurrent});
  requireCurrent(captured);
  const segments = catalogueSelectionSegments(items);
  const result = await player.playSegments(segments, captured);
  requireCurrent(captured);
  return result;
}
