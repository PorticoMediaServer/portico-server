/**
 * PERF-S04: in-memory position memory for visited lists. The loaded window
 * (items around the viewport) and the scroll offset of the last few visited
 * lists are kept keyed by route plus query; Back restores from memory with
 * zero requests before paint, while the live collection revalidates behind
 * it. Pure (no React), so the unit tests drive it directly.
 */
export type ListWindow<T> = {
  /** Index of `items[0]` in the list. */
  first: number;
  /** Dense items from `first` (truncated at the first hole). */
  items: readonly T[];
  /** Whole-list total, for scrollbar sizing. */
  total: number;
  /** Letter/bucket anchors, for the alphabet rail while memory serves. */
  positionIndex?: Readonly<Record<string, number>>;
  scrollY: number;
  at: number;
};

/** How many visited lists keep their window (audit: "the last N (e.g. 5)"). */
export const MAX_REMEMBERED_LISTS = 5;

const windows = new Map<string, ListWindow<unknown>>();
/** The viewer (server plus profile) the remembered windows belong to. */
let currentScope = '';

/**
 * Stable key for one browsable list: whose it is (server and viewer), the route
 * and its query. Another viewer's key starts a new scope and forgets every
 * window: remembered entries are that viewer's titles and must never render
 * for someone else, even for a frame.
 */
export function listWindowKey(input: {serverId: string; viewerId: string; libraryId: string; pivot?: string; query: string}): string {
  const scope = JSON.stringify([input.serverId, input.viewerId]);
  if (scope !== currentScope) {
    currentScope = scope;
    windows.clear();
  }
  return JSON.stringify([scope, input.libraryId, input.pivot ?? '', input.query]);
}

const scopeOf = (key: string): string => {
  try { return (JSON.parse(key) as unknown[])[0] as string; } catch { return ''; }
};

export function saveListWindow<T>(key: string, window: Omit<ListWindow<T>, 'at'>): void {
  if (!window.items.length) return;
  // A screen of the previous viewer saving as it unmounts must not refill the memory.
  if (scopeOf(key) !== currentScope) return;
  windows.delete(key);
  windows.set(key, {...window, at: Date.now()} as ListWindow<unknown>);
  while (windows.size > MAX_REMEMBERED_LISTS) windows.delete(windows.keys().next().value!);
}

export function recallListWindow<T>(key: string): ListWindow<T> | undefined {
  if (scopeOf(key) !== currentScope) return undefined;
  const hit = windows.get(key) as ListWindow<T> | undefined;
  if (!hit) return undefined;
  // A recall is a visit: it refreshes recency.
  windows.delete(key);
  windows.set(key, hit);
  return hit;
}

export function clearListWindows(): void {
  currentScope = '';
  windows.clear();
}
