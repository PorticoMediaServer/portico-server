/**
 * Actions and lists that survive the world moving under them.
 *
 * The server guards a change with the revision the client last read, and a page of a list with
 * the continuation it handed out. When something else moved in between (another device, a scan,
 * the person's own previous press), the server answers "that is out of date". That answer is not
 * the person's problem: the client reads the thing again and, when what they asked for still
 * makes sense, does it. Only when it no longer does (the thing they acted on is gone, or now
 * says something that contradicts the action) is anything said, in plain words.
 */

const STALE_CODES: ReadonlySet<string> = new Set([
  'stale_continuation', 'invalid_cursor', 'revision_conflict', 'resource_conflict', 'conflict',
  'personal_state_conflict', 'playlist_conflict', 'collection_conflict', 'console_conflict', 'administration_conflict',
  'show_settings_conflict', 'listening_preferences_conflict', 'library_configuration_conflict', 'metadata_conflict',
  'channel_revision_conflict', 'operation_conflict', 'precondition_failed', 'stale_playback_offer', 'stale_chapter_projection',
]);

/** The server (or a client check) said what the action was based on is out of date. */
export function isStaleAnswer(error: unknown): boolean {
  const e = error as {code?: unknown; status?: unknown; retry?: unknown} | null;
  if (!e || typeof e !== 'object') return false;
  if (typeof e.code === 'string' && STALE_CODES.has(e.code)) return true;
  if (e.retry === 'after_refresh') return true;
  return e.status === 412;
}

export type RetryWhenStale = Readonly<{
  /** Read the thing again so the next attempt carries what the server has now. */
  reread(): Promise<unknown>;
  /** After the re-read: does the action still make sense? `false` gives up; `'done'` means the new state already is what was asked for. */
  stillApplies?(): boolean | 'done';
  /** Silent attempts after the first (default 2). */
  attempts?: number;
  isStale?(error: unknown): boolean;
}>;

/** Returned (never thrown) when the re-read showed the action is already done. */
export const ALREADY_DONE = Symbol('already done');

/**
 * Runs `attempt`. An out-of-date answer is met with a re-read and another attempt, silently; any
 * other failure, a re-read that fails, or an action that no longer applies ends with the error
 * the caller shows. `attempt` is given how many silent retries came before it.
 */
export async function retryWhenStale<T>(attempt: (retries: number) => Promise<T>, options: RetryWhenStale): Promise<T | typeof ALREADY_DONE> {
  const limit = Math.max(0, options.attempts ?? 2), stale = options.isStale ?? isStaleAnswer;
  for (let retries = 0; ; retries++) {
    try {
      return await attempt(retries);
    } catch (error) {
      if (retries >= limit || !stale(error)) throw error;
      // A re-read that fails leaves the person with the first answer: the action itself did not go through.
      try { await options.reread(); } catch { throw error; }
      const applies = options.stillApplies ? options.stillApplies() : true;
      if (applies === 'done') return ALREADY_DONE;
      if (!applies) throw error;
    }
  }
}

/**
 * A list that was read page by page, after its first page has been read again.
 *
 * `fresh` is the first page as the server has it now; `shown` is everything on screen, first
 * page and the pages appended after it. The answer leads with the fresh page and keeps the tail
 * the person had already scrolled into, so nothing below them disappears and the list does not
 * jump. Entries that used to sit inside the first page's reach and are no longer in it have been
 * removed or moved, and are dropped. Entries keep their identity (`keyOf`), so a row that is
 * unchanged is the same row. When nothing changed at all, `shown` itself is returned.
 */
export function mergeRevalidatedList<T>(shown: readonly T[], fresh: readonly T[], keyOf: (item: T) => string, options: {same?: (a: T, b: T) => boolean; /** The fresh page is the whole list (it has no next page): nothing beyond it is kept. */ complete?: boolean} = {}): readonly T[] {
  if (!shown.length) return fresh;
  const same = options.same ?? Object.is;
  const freshKeys = new Set<string>();
  for (const item of fresh) freshKeys.add(keyOf(item));
  // How far into what is shown the fresh page reaches: the last shown entry that is also in the fresh page.
  let reach = -1;
  for (let i = shown.length - 1; i >= 0; i--) if (freshKeys.has(keyOf(shown[i]!))) { reach = i; break; }
  // Nothing in common: the list is a different list now (another sort, everything replaced).
  if (reach < 0) return fresh;
  const before = new Map<string, T>();
  for (let i = 0; i <= reach; i++) before.set(keyOf(shown[i]!), shown[i]!);
  const out: T[] = [];
  let changed = false;
  for (const item of fresh) {
    const old = before.get(keyOf(item));
    const kept = old !== undefined && same(old, item) ? old : item;
    if (kept !== shown[out.length]) changed = true;
    out.push(kept);
  }
  if (!options.complete) for (let i = reach + 1; i < shown.length; i++) {
    const item = shown[i]!;
    if (freshKeys.has(keyOf(item))) { changed = true; continue; }
    if (item !== shown[out.length]) changed = true;
    out.push(item);
  }
  return changed || out.length !== shown.length ? out : shown;
}

/** Entries present in `next` and not in `previous`: the cards that fade in. Bounded, so a first load or a new sort animates nothing. */
export function arrivals<T>(previous: readonly T[] | undefined, next: readonly T[], keyOf: (item: T) => string, limit = 12): ReadonlySet<string> {
  const none: ReadonlySet<string> = new Set();
  if (!previous?.length || !next.length) return none;
  const had = new Set<string>();
  for (const item of previous) had.add(keyOf(item));
  const added = new Set<string>();
  for (const item of next) { const key = keyOf(item); if (!had.has(key)) { added.add(key); if (added.size > limit) return none; } }
  return added;
}
