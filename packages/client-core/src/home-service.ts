/**
 * Home as one document every app shows the same way: read once, kept while the app runs, and
 * brought up to date in place.
 *
 * - `load()` is the first read and the explicit refresh. Rows already on screen stay there while
 *   it runs; an error with rows on screen keeps the rows.
 * - `revalidate()` is the quiet read the freshness hub asks for when a library, the profile's
 *   watched state or a playback position moved. The answer is merged into what is shown: a row
 *   that did not change is the same object (its cards do not re-render), a row that did keeps the
 *   pages the person had already scrolled into, and the order of rows follows the server.
 * - A row's next page that the server refuses as out of date is not the person's problem: the
 *   whole document is read again quietly and the row continues from there.
 * - Saving the layout carries the revision that was read; when the layout moved elsewhere first,
 *   it is read again and the same arrangement is sent with the new revision.
 *
 * No React and no timers of its own beyond the read deadline: tested under Node.
 */
import {appendHomeRowPage, fetchHome, fetchHomeLayout, fetchHomeRow, resetHomeLayout, saveHomeLayout, type HomeApi, type HomeDocument, type HomeLayout, type HomeLayoutView, type HomeRow} from './home.ts';
import {ALREADY_DONE, isStaleAnswer, mergeRevalidatedList, retryWhenStale} from './freshness-actions.ts';

export type HomeSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'ready' | 'error';
  document?: HomeDocument;
  /** The last failed read. Present it through the app's error words; never show its message. */
  error?: unknown;
  /** Rows whose next page is being read. */
  paging: ReadonlySet<string>;
}>;

const none: ReadonlySet<string> = Object.freeze(new Set<string>());
const sameJson = (a: unknown, b: unknown) => a === b || JSON.stringify(a) === JSON.stringify(b);

/** `fresh` merged into `shown`: unchanged rows keep their identity, paged rows keep their tail. Returns `shown` when nothing changed. */
export function mergeHomeDocument(shown: HomeDocument, fresh: HomeDocument): HomeDocument {
  if (shown.serverId !== fresh.serverId || shown.viewerFence !== fresh.viewerFence) return fresh;
  const before = new Map(shown.rows.map(row => [row.id, row]));
  let changed = shown.rows.length !== fresh.rows.length;
  const rows = fresh.rows.map((row, index) => {
    const old = before.get(row.id);
    if (!old) { changed = true; return row; }
    const entries = mergeRevalidatedList(old.entries, row.entries, e => e.id, {same: sameJson, complete: !row.nextCursor});
    const keptTail = entries.length > row.entries.length;
    const {entries: _a, revision: _b, nextCursor: _c, hasMore: _d, ...oldRest} = old;
    const {entries: _e, revision: _f, nextCursor: _g, hasMore: _h, ...freshRest} = row;
    const nextCursor = keptTail ? old.nextCursor : row.nextCursor;
    if (entries === old.entries && nextCursor === old.nextCursor && sameJson(oldRest, freshRest)) {
      if (shown.rows[index] !== old) changed = true;
      return old;
    }
    changed = true;
    return Object.freeze({...row, entries, nextCursor, hasMore: keptTail ? old.hasMore : row.hasMore});
  });
  if (!changed && sameJson(shown.layout, fresh.layout) && sameJson(shown.hero, fresh.hero)) return shown;
  return Object.freeze({...fresh, rows: Object.freeze(rows)});
}

export class HomeService {
  private state: HomeSnapshot = Object.freeze({phase: 'idle', paging: none});
  private listeners = new Set<() => void>();
  private generation = 0;
  private reading?: {controller: AbortController; promise: Promise<void>};
  private pages = new Map<string, AbortController>();
  private disposed = false;
  private readonly api: () => HomeApi;
  private readonly timeoutMs: number;

  /** `api` may be a function so a renewed sign-in is picked up without a new service. */
  constructor(api: HomeApi | (() => HomeApi), options: {timeoutMs?: number} = {}) {
    this.api = typeof api === 'function' ? api : () => api;
    this.timeoutMs = options.timeoutMs ?? 15_000;
  }

  getSnapshot = (): HomeSnapshot => this.state;
  subscribe = (listener: () => void): (() => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(patch: Partial<HomeSnapshot>): void {
    if (this.disposed) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const listener of [...this.listeners]) listener();
  }

  private async read(signal: AbortSignal): Promise<HomeDocument> {
    let timer: ReturnType<typeof setTimeout> | undefined, abort = () => {};
    const controller = new AbortController();
    const stop = () => controller.abort();
    signal.addEventListener('abort', stop, {once: true});
    try {
      return await Promise.race([
        fetchHome(this.api(), undefined, controller.signal),
        new Promise<never>((_, reject) => {
          abort = () => reject(Object.assign(new Error('home_read_cancelled'), {name: 'AbortError'}));
          controller.signal.addEventListener('abort', abort, {once: true});
          timer = setTimeout(() => { reject(Object.assign(new Error('home_read_timeout'), {code: 'request_timeout', retryable: true})); controller.abort(); }, this.timeoutMs);
        }),
      ]);
    } finally {
      if (timer) clearTimeout(timer);
      controller.signal.removeEventListener('abort', abort);
      signal.removeEventListener('abort', stop);
    }
  }

  /** The first read, or an explicit refresh. `clear` drops what is shown first (another profile's taste was reset). Concurrent calls share one read. */
  load(options: {clear?: boolean} = {}): Promise<void> {
    if (this.disposed) return Promise.resolve();
    if (this.reading && !options.clear) return this.reading.promise;
    this.reading?.controller.abort();
    const mine = ++this.generation, controller = new AbortController();
    for (const page of this.pages.values()) page.abort();
    this.pages.clear();
    this.publish({phase: 'loading', error: undefined, paging: none, ...(options.clear ? {document: undefined} : {})});
    const promise = (async () => {
      try {
        const document = await this.read(controller.signal);
        if (this.disposed || mine !== this.generation) return;
        this.publish({phase: 'ready', document, error: undefined});
      } catch (error) {
        if (this.disposed || mine !== this.generation || controller.signal.aborted) return;
        this.publish({phase: 'error', error});
      } finally {
        if (this.reading?.controller === controller) this.reading = undefined;
      }
    })();
    this.reading = {controller, promise};
    return promise;
  }

  retry = (): Promise<void> => this.load();

  /** The quiet read: merge the server's current Home into what is shown. Rejects when the read failed; nothing changes then. */
  async revalidate(context?: {signal?: AbortSignal; settle?: () => Promise<void>}): Promise<void> {
    if (this.disposed || this.state.phase !== 'ready' || !this.state.document) return;
    const mine = this.generation, controller = new AbortController(), stop = () => controller.abort();
    context?.signal?.addEventListener('abort', stop, {once: true});
    try {
      const fresh = await this.read(controller.signal);
      await context?.settle?.();
      if (this.disposed || mine !== this.generation || controller.signal.aborted || !this.state.document || this.state.phase !== 'ready') return;
      const merged = mergeHomeDocument(this.state.document, fresh);
      if (merged !== this.state.document) this.publish({document: merged});
    } catch (error) {
      if (this.disposed || mine !== this.generation || controller.signal.aborted) return;
      throw error;
    } finally { context?.signal?.removeEventListener('abort', stop); }
  }

  /** The next page of one row. */
  async more(rowId: string): Promise<void> {
    const row = this.state.document?.rows.find(r => r.id === rowId);
    if (this.disposed || !row?.nextCursor || this.pages.has(rowId) || this.state.phase !== 'ready') return;
    const mine = this.generation, controller = new AbortController(), cursor = row.nextCursor;
    this.pages.set(rowId, controller);
    this.publish({paging: new Set(this.state.paging).add(rowId)});
    const done = () => {
      if (this.pages.get(rowId) === controller) this.pages.delete(rowId);
      if (this.disposed || mine !== this.generation) return;
      const paging = new Set(this.state.paging); paging.delete(rowId);
      this.publish({paging: paging.size ? paging : none});
    };
    try {
      const page = await fetchHomeRow(this.api(), rowId, {cursor}, controller.signal);
      if (this.disposed || mine !== this.generation || controller.signal.aborted) return;
      const document = this.state.document;
      // The row moved on while this page was read (a quiet read replaced it): this page belongs to the older row.
      if (document && document.rows.find(r => r.id === rowId)?.nextCursor === cursor) this.publish({document: Object.freeze({...document, rows: Object.freeze(document.rows.map(r => (r.id === rowId ? appendHomeRowPage(r, page) : r)))})});
      done();
    } catch (error) {
      if (this.disposed || mine !== this.generation || controller.signal.aborted) return;
      done();
      // Out of date, not broken: bring the whole of Home up to date in place and let the row continue from there.
      if (isStaleAnswer(error)) { await this.revalidate().catch(() => {}); return; }
      // Any other failed page leaves the row as it is; reaching its end again asks again.
    }
  }

  loadLayout = (signal?: AbortSignal): Promise<HomeLayoutView> => fetchHomeLayout(this.api(), signal);

  /** Saves the arrangement. A layout that moved elsewhere first is read again and the same arrangement sent once more. */
  async saveLayout(rowOrder: readonly string[], hiddenRowIds: readonly string[], idempotencyKey: () => string, expectedRevision?: number): Promise<HomeLayout> {
    let revision = expectedRevision ?? this.state.document?.layout.revision;
    if (revision === undefined) throw new Error('Home is not loaded.');
    const saved = await retryWhenStale(() => saveHomeLayout(this.api(), {expectedRevision: revision!, rowOrder, hiddenRowIds, idempotencyKey: idempotencyKey()}), {
      reread: async () => { revision = (await fetchHomeLayout(this.api())).revision; },
    });
    if (saved === ALREADY_DONE) throw new Error('Home is not loaded.');
    await this.afterLayout();
    return saved;
  }

  async resetLayout(idempotencyKey: () => string): Promise<void> {
    await resetHomeLayout(this.api(), idempotencyKey());
    await this.afterLayout();
  }

  /** The rows follow a new arrangement at once, without a loading state when Home is already showing. */
  private async afterLayout(): Promise<void> {
    if (this.state.phase === 'ready' && this.state.document) {
      const mine = this.generation;
      try {
        const fresh = await this.read(new AbortController().signal);
        if (!this.disposed && mine === this.generation && this.state.document) this.publish({document: mergeHomeDocument(this.state.document, fresh)});
        return;
      } catch { /* fall through to an ordinary load, which reports its own failure */ }
    }
    await this.load();
  }

  /** Take an entry out of every row at once (an optimistic removal the screen has just asked the server for); returns the way to put the rows back. */
  withoutEntry(entryId: string): () => void {
    const before = this.state.document;
    if (!before) return () => {};
    let touched = false;
    const rows = before.rows.map(row => {
      if (!row.entries.some(e => e.id === entryId)) return row;
      touched = true;
      return Object.freeze({...row, entries: Object.freeze(row.entries.filter(e => e.id !== entryId)), total: Math.max(0, row.total - 1)});
    });
    if (!touched) return () => {};
    const after = Object.freeze({...before, rows: Object.freeze(rows)});
    this.publish({document: after});
    return () => { if (this.state.document === after) this.publish({document: before}); else void this.revalidate().catch(() => {}); };
  }

  dispose(): void {
    this.disposed = true;
    this.generation++;
    this.reading?.controller.abort();
    for (const page of this.pages.values()) page.abort();
    this.pages.clear();
    this.listeners.clear();
  }
}
