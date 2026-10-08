import {createWindowedCollection, type WindowedCollection} from '@core/collections/index.ts';

/**
 * PERF-S02/S04/S08: a `WindowedCollection` over items already in memory
 * (appended section pages, saved-list pages, a remembered list window), so a
 * long list renders only its visible window. `setItems` swaps to a new
 * generation serving the longer array; the grid keeps showing the old one
 * until the new first visible page answers (synchronously, from memory).
 *
 * With `base`/`total`, the memory holds a window of a larger list (a Back
 * restore): indices outside `[base, base + items.length)` read as
 * placeholders, and `total` sizes the scrollbar for the whole list.
 */
export function createStaticCollection<T>(options: {keyOf: (item: T) => string; pageSize?: number; maxResidentPages?: number; base?: number}): {collection: WindowedCollection<T>; setItems: (items: readonly T[], total?: number) => void} {
  const pageSize = Math.max(1, Math.floor(options.pageSize ?? 60));
  const base = Math.max(0, Math.floor(options.base ?? 0));
  let items: readonly T[] = [];
  let total: number | undefined;
  const fetch = async (start: number, count: number): Promise<{items: readonly T[]; total: number}> => {
    const size = total ?? items.length;
    // Items sit at absolute indices base, base+1, …: a page is only filled when
    // it starts at or after base, so an unaligned base never shifts an item to
    // another index (the page before it stays a placeholder until live data).
    if (start < base) return {items: [], total: size};
    const from = start - base;
    const to = Math.min(items.length, from + count);
    return {items: from < to ? items.slice(from, to) : [], total: size};
  };
  const collection = createWindowedCollection<T>({fetchPage: fetch, pageSize, maxResidentPages: Math.max(1, Math.floor(options.maxResidentPages ?? 5)), keyOf: options.keyOf});
  return {collection, setItems: (next, nextTotal) => { items = next; total = nextTotal; collection.invalidate(fetch); }};
}
