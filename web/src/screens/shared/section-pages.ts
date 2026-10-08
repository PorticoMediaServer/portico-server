import type {ContentSection} from '@core/library-content.ts';

/**
 * PERF-S02/S08: cursor-page accumulation for section "More" and saved lists
 * (the web port of Apple's `useAppendedPages` core). A page is stored under
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

export type AppendedSections = {routeKey: string; sections: readonly ContentSection[]};

const sectionSeen = new WeakMap<ContentSection, Set<string>>();

/**
 * PERF-S02 pure core: a full projection (first page) replaces every section;
 * a single-section "More" load appends its entries into the stored section and
 * keeps the page's other sections. Never reorders, never drops a section.
 */
export function mergeSectionPages(previous: AppendedSections | null, routeKey: string, pageCursor: string | null, incoming: readonly ContentSection[]): AppendedSections {
  if (!previous || previous.routeKey !== routeKey || pageCursor === null || incoming.length !== 1) {
    return {routeKey, sections: incoming};
  }
  const next = incoming[0]!;
  const index = previous.sections.findIndex(s => s.id === next.id);
  if (index < 0) return {routeKey, sections: incoming};
  const current = previous.sections[index]!;
  // The seen set follows the merged section, so a "More" page keys only its
  // own entries.
  const seen = sectionSeen.get(current) ?? new Set(current.entries.map(e => e.id));
  const added: ContentSection["entries"][number][] = [];
  for (const e of next.entries) if (!seen.has(e.id)) { seen.add(e.id); added.push(e); }
  const merged: ContentSection = {...current, entries: added.length ? current.entries.concat(added) : current.entries, totalCount: next.totalCount, nextCursor: next.nextCursor};
  sectionSeen.set(merged, seen);
  return {routeKey, sections: previous.sections.map((s, i) => (i === index ? merged : s))};
}

/** Past this many entries a section renders through the windowed grid/list (O(visible), never O(loaded)). */
export const SECTION_WINDOW_AT = 240;
