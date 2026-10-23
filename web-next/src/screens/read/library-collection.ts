import {ReadingError, type ReadingClient} from '@portico/client-core';
import {createWindowedCollection} from '@core/collections/windowed.ts';

export type ReadingLibraryItem = Awaited<ReturnType<ReadingClient['list']>>['items'][number];
const pageSize = 60;

/** Cursor breadcrumbs contain no media models. The existing sparse collection
 * alone owns resident rows (six pages); scrolling never scans the library. */
export function createReadingCollection(client: ReadingClient, libraryId: string) {
  const cursors = new Map<number, string | undefined>([[0, undefined]]);
  const waiting = new Map<number, Set<() => void>>();
  let ended: number | undefined, frontier = 0, lowerBound = 0, disposed = false;
  const wake = () => {for (const listeners of waiting.values()) for (const listener of [...listeners]) listener();};
  const cursor = async (start: number, signal: AbortSignal) => {
    if (start > frontier + pageSize * 2) throw new ReadingError('invalid_cursor');
    if (!cursors.has(start) && (ended === undefined || start < ended)) await new Promise<void>((resolve, reject) => {
      const listeners = waiting.get(start) ?? new Set<() => void>(); waiting.set(start, listeners);
      const release = () => {listeners.delete(check); if (!listeners.size) waiting.delete(start); signal.removeEventListener('abort', check);};
      const check = () => {
        if (disposed || signal.aborted) {release(); reject(new ReadingError('cancelled'));}
        else if (cursors.has(start) || ended !== undefined && start >= ended) {release(); resolve();}
      };
      listeners.add(check); signal.addEventListener('abort', check, {once: true}); check();
    });
    if (disposed || signal.aborted) throw new ReadingError('cancelled');
    return cursors.get(start);
  };
  const collection = createWindowedCollection<ReadingLibraryItem>({pageSize, maxResidentPages: 6, maxConcurrent: 2, prefetchPages: 1,
    keyOf: item => item.id,
    async fetchPage(start, count, signal) {
      const next = await cursor(start, signal);
      if (ended !== undefined && start >= ended) return {items: [], total: ended};
      const response = await client.list({libraryId, cursor: next, limit: count}, signal);
      if (response.items.some(item => item.libraryId !== libraryId) || response.nextCursor && (response.items.length !== count || [...cursors.values()].includes(response.nextCursor)))
        throw new ReadingError('invalid_document');
      if (response.nextCursor) {
        cursors.set(start + count, response.nextCursor); frontier = Math.max(frontier, start + count);
        lowerBound = Math.max(lowerBound, start + response.items.length + 1);
      } else {ended = start + response.items.length;}
      wake();
      return {items: response.items, total: ended ?? lowerBound, totalIsLowerBound: ended === undefined};
    },
  });
  return Object.freeze({collection, dispose() {if (disposed) return; disposed = true; collection.dispose(); wake(); cursors.clear(); waiting.clear();}});
}
