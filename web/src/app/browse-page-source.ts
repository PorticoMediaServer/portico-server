import type {FetchPage} from '@core/collections/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {parseBrowseResult, type BrowseNode, type BrowseRequestBody, type BrowseSortSelection} from '@core/browse.ts';
import {bucketIndexFor} from '@core/browse-buckets.ts';

export type Predicate = Extract<BrowseNode, {field: string}>;

function sortValueFor(entry: ContentEntry, sortField: string): unknown {
  if (sortField === 'title') return (entry as {title?: unknown}).title;
  if (sortField === 'year') return (entry as {year?: unknown}).year;
  if (sortField === 'duration' || sortField === 'durationSeconds') return (entry as {duration?: unknown}).duration;
  if (['added', 'lastPlayed', 'addedAt', 'lastPlayedAt', 'dateAdded'].includes(sortField)) return (entry as {addedAt?: unknown}).addedAt;
  return undefined;
}

/**
 * Q4: browse pages by offset (`range.start`) pinned to the first page's
 * `revision`, so a long scroll reads one consistent snapshot. An expired
 * pinned revision recovers in place: the failure clears the pin and reloads
 * once, instead of resetting to the top.
 *
 * W1 re-anchoring (decision A): no `anchorId` is sent for non-title sorts. On
 * a non-title sort, after a `stale_continuation` reload, the source
 * re-navigates through the fresh first page's `positionIndex` to the bucket
 * matching the sort value of the entry that was first visible before the
 * reload, jumping there with a plain `range.start`. No match keeps the same
 * index clamped to the new total. Pure (no React/session) so node:test can
 * exercise it without session.tsx.
 */
export function browsePageSource(api: {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>}, input: {libraryId: string; pivot: string; predicates: readonly Predicate[]; sort?: BrowseSortSelection}): FetchPage<ContentEntry> {
  const sortField = input.sort?.field ?? 'title';
  let revision: string | undefined;
  const firstEntries = new Map<number, ContentEntry>();
  const loadRange = async (start: number, count: number, signal: AbortSignal, rev: string | undefined) => {
    const body: BrowseRequestBody = {pivot: input.pivot, ...(input.predicates.length ? {query: {all: input.predicates}} : {}), ...(input.sort ? {sort: [input.sort]} : {}), limit: count, range: {start, ...(rev ? {revision: rev} : {})}};
    return parseBrowseResult(await api.request<unknown>(`/v1/libraries/${encodeURIComponent(input.libraryId)}/browse`, 'POST', body, signal));
  };
  const toPage = (result: ReturnType<typeof parseBrowseResult>, start: number) => {
    if (result.entries.length) {
      firstEntries.set(start, result.entries[0]!);
      while (firstEntries.size > 20) firstEntries.delete(firstEntries.keys().next().value!);
    }
    return {items: result.entries, total: result.pageInfo.total, positionIndex: Object.fromEntries(result.positionIndex.map(a => [a.key, a.index]))};
  };
  return async (start, count, signal) => {
    try {
      const result = await loadRange(start, count, signal, revision);
      revision ??= result.pageInfo.revision || undefined;
      return toPage(result, start);
    } catch (error) {
      const code = (error as {code?: unknown} | null)?.code;
      if (code === 'stale_continuation' && revision !== undefined && !signal.aborted) {
        const before = firstEntries.get(start);
        revision = undefined;
        if (sortField !== 'title' && before) {
          const freshFirst = await loadRange(0, count, signal, undefined);
          revision = freshFirst.pageInfo.revision || undefined;
          const freshTotal = freshFirst.pageInfo.total;
          const bucket = bucketIndexFor(freshFirst.positionIndex, sortField, sortValueFor(before, sortField));
          const target = bucket !== undefined ? bucket : Math.min(start, Math.max(0, freshTotal - 1));
          if (target === 0) {
            if (freshFirst.entries.length) firstEntries.set(0, freshFirst.entries[0]!);
            return toPage(freshFirst, 0);
          }
          const result = await loadRange(target, count, signal, revision);
          return toPage(result, target);
        }
        const result = await loadRange(start, count, signal, undefined);
        revision ??= result.pageInfo.revision || undefined;
        return toPage(result, start);
      }
      throw error;
    }
  };
}
