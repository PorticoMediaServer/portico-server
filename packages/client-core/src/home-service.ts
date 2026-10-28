/**
 * Home as one document every app shows the same way: read once, kept while the app runs, and
 * brought up to date in place.
 *
 * - `load()` is the first read and the explicit refresh. Rows already on screen stay there while
 *   it runs; an error with rows on screen keeps the rows.
 * - `revalidate()` is the quiet read the freshness hub asks for when a library, the profile's
 *   watched state or a playback position moved. The answer is merged into what is shown: a row
 *   that did not change is the same object (its cards do not re-render), and the order of rows
 *   follows the server. Membership-fenced rows replace their pages and cursor together; rows
 *   whose server supports reanchoring may retain their bounded tail.
 * - A row's next page that the server refuses as out of date is not the person's problem: the
 *   whole document is read again quietly and the row continues from there.
 * - Saving the layout carries the revision that was read; when the layout moved elsewhere first,
 *   the conflict is surfaced for an explicit Reload layout before saving again.
 *
 * No React and no timers of its own beyond the read deadline: tested under Node.
 */
import {appendHomeRowPage, fetchHome, fetchHomeLayout, fetchHomeRow, hasFencedHomeMembership, hasHomeRowContent, resetHomeLayout, saveHomeLayout, type HomeApi, type HomeDocument, type HomeLayout, type HomeLayoutView} from './home.ts';
import {isStaleAnswer, mergeRevalidatedList} from './freshness-actions.ts';

/** One original layout editor opening; it never becomes the next editor. */
export type HomeLayoutWriteLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;
const layoutRetired = () => Object.assign(new Error('Home layout opening retired'), {name: 'AbortError'});
export type HomeSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'ready' | 'error';
  document?: HomeDocument;
  /** The last failed read. Present it through the app's error words; never show its message. */
  error?: unknown;
  /** Rows whose next page is being read. */
  paging: ReadonlySet<string>;
}>;

/** A receipt belongs only to this call's row and cursor, never to a quiet refresh. */
export type HomePageOutcome = Readonly<{rowId: string; requestedCursor: string | undefined} & (
  /** Raw entry.id of the first new retained card; null for a page without new cards (including Live rows). */
  | {status: 'applied'; applied: true; firstItemId: string | null}
  | {status: 'failed'; applied: false}
  | {status: 'refused'; applied: false; reason: 'not_ready' | 'row_missing' | 'no_cursor' | 'already_paging' | 'permission_denied'}
  /** The server rejected the continuation and the recovery read failed. */
  | {status: 'stale'; applied: false}
  /** Recovery read succeeded; its entries are not a receipt for the rejected page. */
  | {status: 'revalidated'; applied: false}
  | {status: 'ignored'; applied: false; reason: 'disposed' | 'superseded' | 'aborted' | 'row_changed' | 'viewer_changed' | 'no_progress'}
)>;

const none: ReadonlySet<string> = Object.freeze(new Set<string>());
const sameJson = (a: unknown, b: unknown) => a === b || JSON.stringify(a) === JSON.stringify(b);
const permissionDenied=(error:unknown):boolean=>typeof error==='object'&&error!==null&&('status' in error&&[401,403,404].includes(Number(error.status))||'code' in error&&['unauthorized','forbidden','not_found'].includes(String(error.code)));

/** `fresh` merged into `shown`: only reanchorable rows may keep a bounded tail. Returns `shown` when nothing changed. */
export function mergeHomeDocument(shown: HomeDocument, fresh: HomeDocument): HomeDocument {
  if (shown.serverId !== fresh.serverId || shown.viewerFence !== fresh.viewerFence) return fresh;
  // Layout owns row order and hidden membership, so an older layout cannot replace either.
  if (fresh.layout.revision < shown.layout.revision) return shown;
  const before = new Map(shown.rows.map(row => [row.id, row]));
  let changed = shown.rows.length !== fresh.rows.length;
  const rows = fresh.rows.flatMap((row, index) => {
    if (row.deferred && row.id === 'for_you') {
      const previous = shown.rows.filter(old => old.family === 'for_you');
      if (previous.length) { changed = true; return previous; }
    }
    const old = before.get(row.id);
    if (!old) { changed = true; return [row]; }
    if (row.deferred) { if (shown.rows[index] !== old) changed = true; return [old]; }
    if(row.live||old.live){
      if(sameJson(old,row)){if(shown.rows[index]!==old)changed=true;return [old];}
      changed=true;return [row];
    }
    // No membership fingerprint is exposed to the client. Even overlapping first
    // pages cannot prove that a retained tail or its signed cursor is still valid.
    const refreshed = mergeRevalidatedList(old.entries, row.entries, e => e.id, {same: sameJson, complete: hasFencedHomeMembership(row.id) || !row.nextCursor});
    const entries = refreshed;
    const keptTail = entries.length > row.entries.length;
    const {entries: _a, revision: _b, nextCursor: _c, hasMore: _d, ...oldRest} = old;
    const {entries: _e, revision: _f, nextCursor: _g, hasMore: _h, ...freshRest} = row;
    const nextCursor = keptTail ? old.nextCursor : row.nextCursor;
    if (entries === old.entries && nextCursor === old.nextCursor && sameJson(oldRest, freshRest)) {
      if (shown.rows[index] !== old) changed = true;
      return [old];
    }
    changed = true;
    return [Object.freeze({...row, entries, nextCursor, hasMore: keptTail ? old.hasMore : row.hasMore})];
  });
  if (!changed && sameJson(shown.layout, fresh.layout) && sameJson(shown.spotlight, fresh.spotlight)) return shown;
  return Object.freeze({...fresh, rows: Object.freeze(rows)});
}

export class HomeService {
  private state: HomeSnapshot = Object.freeze({phase: 'idle', paging: none});
  private listeners = new Set<() => void>();
  private generation = 0;
  /** Quiet reads are numbered too: a settled read older than the newest accepted quiet read is stale. */
  private quiet = 0;
  private reading?: {controller: AbortController; promise: Promise<void>};
  private pages = new Map<string, AbortController>();
  private disposed = false;
  private hydrationGeneration = 0;
  private hydrations = new Set<AbortController>();
  private readonly api: () => HomeApi;
  private readonly timeoutMs: number;
  private readonly initialLimit: number | undefined;

  /** `api` may be a function so a renewed sign-in is picked up without a new service. */
  constructor(api: HomeApi | (() => HomeApi), options: {timeoutMs?: number; initialLimit?: number} = {}) {
    if (options.initialLimit !== undefined && (!Number.isSafeInteger(options.initialLimit) || options.initialLimit < 1 || options.initialLimit > 60)) throw new RangeError('Home initialLimit must be an integer from 1 to 60');
    this.api = typeof api === 'function' ? api : () => api;
    this.timeoutMs = options.timeoutMs ?? 15_000;
    this.initialLimit = options.initialLimit;
  }

  getSnapshot = (): HomeSnapshot => this.state;
  subscribe = (listener: () => void): (() => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(patch: Partial<HomeSnapshot>): void {
    if (this.disposed) return;
    this.state = Object.freeze({...this.state, ...patch});
    const listeners = [...this.listeners];
    for (const listener of listeners) listener();
  }

  private hydrateRows(fresh: HomeDocument, quiet?: number): void {
    const shown = this.state.document;
    if (shown && shown.serverId === fresh.serverId && shown.viewerFence === fresh.viewerFence && fresh.layout.revision < shown.layout.revision) return;
    const epoch = ++this.hydrationGeneration;
    for (const controller of this.hydrations) controller.abort();
    this.hydrations.clear();
    const queue = fresh.rows.filter(row => row.deferred);
    const mine = this.generation;
    const consume = async () => {
      while (queue.length && !this.disposed && epoch === this.hydrationGeneration && (quiet === undefined || quiet === this.quiet)) {
        const descriptor = queue.shift()!;
        const controller = new AbortController();
        this.hydrations.add(controller);
        let abort = () => {};
        let timer: ReturnType<typeof setTimeout> | undefined;
        try {
          const row = await Promise.race([
            fetchHomeRow(this.api(), descriptor.id, {limit: this.initialLimit ?? (descriptor.limit || undefined)}, controller.signal),
            new Promise<never>((_, reject) => {
              abort = () => reject(Object.assign(new Error('home_row_cancelled'), {name: 'AbortError'}));
              controller.signal.addEventListener('abort', abort, {once: true});
              timer = setTimeout(() => controller.abort(), this.timeoutMs);
            }),
          ]);
          const shown = this.state.document;
          if (this.disposed || mine !== this.generation || (quiet !== undefined && quiet !== this.quiet) || epoch !== this.hydrationGeneration || controller.signal.aborted || !shown || shown.viewerFence !== fresh.viewerFence || shown.serverId !== fresh.serverId || row.viewerFence !== fresh.viewerFence || row.children?.some(child => child.viewerFence !== fresh.viewerFence)) continue;
          const replacements = row.children ?? [row];
          const rows = shown.rows.flatMap(old => old.id === descriptor.id || descriptor.id === 'for_you' && old.family === 'for_you' ? [] : [old]);
          const at = shown.rows.findIndex(old => old.id === descriptor.id || descriptor.id === 'for_you' && old.family === 'for_you');
          rows.splice(Math.max(0, at), 0, ...replacements.filter(next => hasHomeRowContent(next) || next.critical));
          this.publish({document: mergeHomeDocument(shown, Object.freeze({...shown, rows: Object.freeze(rows)})), error: undefined});
        } catch (error) {
          if(permissionDenied(error)&&!this.disposed&&mine===this.generation&&(quiet===undefined||quiet===this.quiet)&&epoch===this.hydrationGeneration&&!controller.signal.aborted)this.removeRow(descriptor.id);
          // A failed shelf keeps its previous cards. Other shelves continue;
          // the next quiet Home refresh retries the deferred endpoint.
        } finally { if (timer) clearTimeout(timer); controller.signal.removeEventListener('abort', abort); this.hydrations.delete(controller); }
      }
    };
    void consume();
    void consume();
  }

  private removeRow(id:string):void {
    const document=this.state.document;
    if(document)this.publish({document:Object.freeze({...document,rows:Object.freeze(document.rows.filter(row=>row.id!==id))})});
  }

  private async read(signal: AbortSignal): Promise<HomeDocument> {
    let timer: ReturnType<typeof setTimeout> | undefined, abort = () => {};
    const controller = new AbortController();
    const stop = () => controller.abort();
    signal.addEventListener('abort', stop, {once: true});
    try {
      return await Promise.race([
        fetchHome(this.api(), this.initialLimit, controller.signal),
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
    // An explicit load/clear invalidates every outstanding quiet read.
    this.quiet++;
    for (const page of this.pages.values()) page.abort();
    this.pages.clear();
    this.publish({phase: 'loading', error: undefined, paging: none, ...(options.clear ? {document: undefined} : {})});
    const promise = (async () => {
      try {
        const document = await this.read(controller.signal);
        if (this.disposed || mine !== this.generation) return;
        this.publish({phase: 'ready', document: this.state.document ? mergeHomeDocument(this.state.document, document) : document, error: undefined});
        this.hydrateRows(document);
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
    const mine = this.generation, ticket = ++this.quiet, controller = new AbortController(), stop = () => controller.abort();
    const current = () => !this.disposed && mine === this.generation && ticket === this.quiet && !controller.signal.aborted;
    context?.signal?.addEventListener('abort', stop, {once: true});
    try {
      const fresh = await this.read(controller.signal);
      // A response that was already superseded must not invoke another caller's
      // settle callback or wait on its obsolete completion.
      if (!current()) return;
      await context?.settle?.();
      if (!current() || !this.state.document || this.state.phase !== 'ready') return;
      const merged = mergeHomeDocument(this.state.document, fresh);
      if (merged !== this.state.document) this.publish({document: merged});
      // A read superseded after its settle may not start or own deferred shelves.
      if (current()) this.hydrateRows(fresh, ticket);
    } catch (error) {
      // A late failure from a superseded read is not the current document's failure.
      if (!current()) return;
      throw error;
    } finally { context?.signal?.removeEventListener('abort', stop); }
  }

  /** The next page of one row. Only an applied outcome may fulfil that exact cursor's wait. */
  async more(rowId: string): Promise<HomePageOutcome> {
    const shown = this.state.document;
    const row = shown?.rows.find(r => r.id === rowId);
    const request = {rowId, requestedCursor: row?.nextCursor};
    if (this.disposed) return {...request, status: 'ignored', applied: false, reason: 'disposed'};
    if (this.state.phase !== 'ready') return {...request, status: 'refused', applied: false, reason: 'not_ready'};
    if (!shown || !row) return {...request, status: 'refused', applied: false, reason: 'row_missing'};
    if (!row.nextCursor) return {...request, status: 'refused', applied: false, reason: 'no_cursor'};
    if (this.pages.has(rowId)) return {...request, status: 'refused', applied: false, reason: 'already_paging'};
    const mine = this.generation, controller = new AbortController(), cursor = row.nextCursor;
    const ignored = (reason: Extract<HomePageOutcome, {status: 'ignored'}>['reason']): HomePageOutcome => ({...request, status: 'ignored', applied: false, reason});
    const cancelled = (): HomePageOutcome | undefined => this.disposed ? ignored('disposed') : mine !== this.generation ? ignored('superseded') : controller.signal.aborted ? ignored('aborted') : undefined;
    const opposed = (): HomePageOutcome | undefined => {
      const stopped = cancelled();
      if (stopped) return stopped;
      const current = this.state.document;
      if (!current || current.serverId !== shown.serverId || current.viewerFence !== shown.viewerFence) return ignored('viewer_changed');
      // Even an equal cursor cannot authorize a page against a replaced membership or Live observation.
      if (current.rows.find(r => r.id === rowId) !== row) return ignored('row_changed');
    };
    this.pages.set(rowId, controller);
    this.publish({paging: new Set(this.state.paging).add(rowId)});
    const done = () => {
      if (this.pages.get(rowId) !== controller) return;
      this.pages.delete(rowId);
      if (this.disposed || mine !== this.generation) return;
      const paging = new Set(this.state.paging); paging.delete(rowId);
      this.publish({paging: paging.size ? paging : none});
    };
    let abort = () => {};
    try {
      const stopped = opposed();
      if (stopped) return stopped;
      // Settle on cancellation even when the transport ignores its signal.
      const cancellation = new Promise<never>((_, reject) => {
        abort = () => reject(Object.assign(new Error('home_page_cancelled'), {name: 'AbortError'}));
        controller.signal.addEventListener('abort', abort, {once: true});
      });
      const page = await Promise.race([fetchHomeRow(this.api(), rowId, {cursor,limit:row.limit||undefined,group:row.live?.group||undefined,sourceId:row.seeAll?.type==='guide'?row.seeAll.sourceId:undefined}, controller.signal), cancellation]);
      const outdated = opposed();
      if (outdated) return outdated;
      if (page.viewerFence !== shown.viewerFence) return ignored('viewer_changed');
      const next = appendHomeRowPage(row, page);
      const seen = new Set(row.entries.map(entry => entry.id));
      const firstItemId = next.entries.find(entry => !seen.has(entry.id))?.id ?? null;
      if (firstItemId === null && next.nextCursor === cursor && (next.live?.channels.length ?? 0) === (row.live?.channels.length ?? 0)) return ignored('no_progress');
      const document = this.state.document!;
      this.publish({document: Object.freeze({...document, rows: Object.freeze(document.rows.map(r => r.id === rowId ? next : r))})});
      done();
      const superseded = cancelled();
      if (superseded) return superseded;
      if (this.state.document?.rows.find(r => r.id === rowId) !== next) return ignored('row_changed');
      return {...request, status: 'applied', applied: true, firstItemId};
    } catch (error) {
      const outdated = opposed();
      if (outdated) return outdated;
      done();
      const replaced = opposed();
      if (replaced) return replaced;
      if(permissionDenied(error)){this.removeRow(rowId);return {...request, status: 'refused', applied: false, reason: 'permission_denied'};}
      // Out of date, not broken: bring the whole of Home up to date in place and let the row continue from there.
      if (isStaleAnswer(error)) {
        const document = this.state.document;
        if (hasFencedHomeMembership(rowId) && document && document.rows.find(r => r.id === rowId)?.nextCursor === cursor) {
          this.publish({document: Object.freeze({...document, rows: Object.freeze(document.rows.map(r => r.id === rowId ? Object.freeze({...r, nextCursor: ''}) : r))})});
        }
        try {
          await this.revalidate();
          return cancelled() ?? {...request, status: 'revalidated', applied: false};
        } catch {
          return cancelled() ?? {...request, status: 'stale', applied: false};
        }
      }
      // Any other failed page leaves the row as it is; reaching its end again asks again.
      return {...request, status: 'failed', applied: false};
    } finally {
      controller.signal.removeEventListener('abort', abort);
      done();
    }
  }

  loadLayout = (signal?: AbortSignal): Promise<HomeLayoutView> => fetchHomeLayout(this.api(), signal);

  private assertLayoutCurrent(lifetime?: HomeLayoutWriteLifetime): void {
    let live = !this.disposed && !lifetime?.signal.aborted;
    try {if (lifetime && !lifetime.isCurrent()) live = false;} catch {live = false;}
    if (!live) throw layoutRetired();
  }
  /** A conflicting save is surfaced for Reload layout; it never overwrites another device silently. */
  async saveLayout(rowOrder: readonly string[], hiddenRowIds: readonly string[], idempotencyKey: () => string, expectedRevision?: number, lifetime?: HomeLayoutWriteLifetime): Promise<HomeLayout> {
    this.assertLayoutCurrent(lifetime);
    const revision = expectedRevision ?? this.state.document?.layout.revision;
    if (revision === undefined) throw new Error('Home is not loaded.');
    try {
      const saved = await saveHomeLayout(this.api(), {expectedRevision: revision, rowOrder, hiddenRowIds, idempotencyKey: idempotencyKey()}, lifetime?.signal);
      this.assertLayoutCurrent(lifetime);
      await this.afterLayout(lifetime);this.assertLayoutCurrent(lifetime);return saved;
    } catch (error) {this.assertLayoutCurrent(lifetime);throw error;}
  }

  async resetLayout(idempotencyKey: () => string, expectedRevision?: number, lifetime?: HomeLayoutWriteLifetime): Promise<void> {
    this.assertLayoutCurrent(lifetime);
    try {
      await resetHomeLayout(this.api(), idempotencyKey(), lifetime?.signal, expectedRevision);
      this.assertLayoutCurrent(lifetime);
      await this.afterLayout(lifetime);this.assertLayoutCurrent(lifetime);
    } catch (error) {this.assertLayoutCurrent(lifetime);throw error;}
  }

  /** The rows follow a new arrangement at once, without a loading state when Home is already showing. */
  private async afterLayout(lifetime?: HomeLayoutWriteLifetime): Promise<void> {
    this.quiet++;
    if (lifetime) {
      this.assertLayoutCurrent(lifetime);
      if (this.state.phase === 'ready' && this.state.document) {
        const mine = this.generation;
        try {
          const fresh = await this.read(lifetime.signal);this.assertLayoutCurrent(lifetime);
          if (mine === this.generation && this.state.document) {
            this.quiet++;
            this.publish({document: mergeHomeDocument(this.state.document, fresh)});this.hydrateRows(fresh);
          }
        } catch (error) {this.assertLayoutCurrent(lifetime);throw error;}
      }
      // A supplied opening never falls through to load(), whose lifecycle is
      // the persistent Home owner rather than this editor's captured lifetime.
      this.assertLayoutCurrent(lifetime);return;
    }
    if (this.state.phase === 'ready' && this.state.document) {
      const mine = this.generation;
      try {
        const fresh = await this.read(new AbortController().signal);
        if (!this.disposed && mine === this.generation && this.state.document) {
          this.quiet++;
          this.publish({document: mergeHomeDocument(this.state.document, fresh)});this.hydrateRows(fresh);
        }
        return;
      } catch { /* Existing callers keep their ordinary Home load fallback. */ }
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
    this.quiet++;
    this.hydrationGeneration++;
    for (const controller of this.hydrations) controller.abort();
    this.hydrations.clear();
    this.generation++;
    this.reading?.controller.abort();
    for (const page of this.pages.values()) page.abort();
    this.pages.clear();
    this.listeners.clear();
  }
}
