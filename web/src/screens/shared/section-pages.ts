
/**
 * PERF-S02/S08: cursor-page accumulation for saved lists (the web port of Apple's
 * `useAppendedPages` core). A library section's pages are kept by the content service itself
 * (`LibraryContentSnapshot.appended`), with the document, so they survive Back. A page is stored under
 * its cursor, so a page that changes in place replaces its own segment; a new
 * scope (tab, sort, route) or a first page (`cursor === null`) starts over.
 * Entries dedupe by key. Pure, so the unit tests drive it without React.
 */
export type PageStore<T> = {scope: string; pages: Map<string, readonly T[]>; out?: readonly T[]; seen?: Set<string>};

export function accumulatePage<T>(store: PageStore<T>, scope: string, cursor: string | null, page: readonly T[], keyOf: (item: T) => string): readonly T[] {
  if (store.scope !== scope || cursor === null) {
    store.scope = scope;
    store.pages = new Map();
    store.out = undefined;
  }
  const key = cursor ?? '';
  // A page seen for the first time appends: only its own entries are keyed,
  // so paging through N entries costs O(N), not O(N²). A page that
  // changed in place (a cursor already held) rebuilds from every page.
  if (!store.pages.has(key) && store.out && store.seen) {
    store.pages.set(key, page);
    const added: T[] = [];
    for (const item of page) {
      const k = keyOf(item);
      if (store.seen.has(k)) continue;
      store.seen.add(k);
      added.push(item);
    }
    store.out = added.length ? store.out.concat(added) : store.out;
    return store.out;
  }
  store.pages.set(key, page);
  const seen = new Set<string>();
  const out: T[] = [];
  for (const items of store.pages.values()) {
    for (const item of items) {
      const k = keyOf(item);
      if (seen.has(k)) continue;
      seen.add(k);
      out.push(item);
    }
  }
  store.out = out;
  store.seen = seen;
  return out;
}

/** Past this many entries a section renders through the windowed grid/list (O(visible), never O(loaded)). */
export const SECTION_WINDOW_AT = 240;
