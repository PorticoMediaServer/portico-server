import {BrowseQueryBuilder, browsePath, parseBrowseResult, type BrowseCapabilities, type BrowseNode, type BrowseResult, type BrowseSortSelection, type BrowseWholeSelection} from './browse.ts';
import type {FetchPage} from './collections/windowed.ts';
import type {ContentEntry} from './library-content.ts';
import {isStaleAnswer} from './freshness-actions.ts';

/** One applied Browse query: what every page, count, facet read and saved view is built from.
 * `nodes` holds the fixed source filter followed by the viewer's own predicates. */
export type BrowseSessionQuery = Readonly<{
  libraryId: string;
  pivot: string;
  capabilities: BrowseCapabilities;
  nodes: readonly BrowseNode[];
  sorts: readonly BrowseSortSelection[];
  homeRowId?: string;
}>;

/** The canonical builder for a query; callers add the limit and range they need. */
export function browseSessionBuilder(query: BrowseSessionQuery): BrowseQueryBuilder {
  const builder = new BrowseQueryBuilder(query.capabilities, query.pivot);
  for (const node of query.nodes) builder.add(node);
  for (const sort of query.sorts) builder.sort(sort.field, sort.direction, sort.seed);
  if (query.homeRowId) builder.displayHomeRow(query.homeRowId);
  return builder;
}

/** A browse page is one library's scope; entity entries (show, album, artist) can omit their
 * library even though every destination needs it. Fill only what the server left absent. */
export function withLibraryScope(page: BrowseResult, libraryId: string): BrowseResult {
  if (!libraryId) return page;
  let changed = false;
  const entries = page.entries.map(entry => {
    if (entry.libraryId) return entry;
    changed = true;
    return Object.freeze({...entry, libraryId});
  });
  return changed ? Object.freeze({...page, entries: Object.freeze(entries)}) : page;
}

/** The session was retired (a new query, a closed scope); its late answers are not anyone's. */
export const browseSessionClosed = (): Error => Object.assign(new Error('browse-scope-closed'), {name: 'AbortError'});
/** The view moved on under this query: refresh it, then page again. */
export const browseRevisionChanged = (): Error => Object.assign(new Error('browse-page-revision-changed'), {code: 'stale_continuation', retry: 'after_refresh'});
/** An answer that says this view is gone for this viewer, not a transient failure. */
const ended = (error: unknown): boolean => isStaleAnswer(error)
  || typeof error === 'object' && error !== null && 'status' in error && typeof error.status === 'number' && [401, 403, 404].includes(error.status);

export type BrowseSessionOptions<T> = Readonly<{
  query: BrowseSessionQuery;
  request: (path: string, method: string, body: unknown, signal: AbortSignal) => Promise<unknown>;
  /** The owning screen is still open for this viewer. */
  current: () => boolean;
  /** What the collection holds for each entry (the entry itself, or a membership row). */
  item: (entry: ContentEntry, page: BrowseResult, index: number) => T;
  /** The first page's opaque revision arrived; every later page must carry the same one. */
  onRevision?: (revision: string) => void;
  /** The server's whole-view descriptor, from this library's first page only. */
  onWhole?: (whole: BrowseWholeSelection) => void;
  onPage?: (page: BrowseResult) => void;
  /** The view changed or ended under this query. Later pages refuse until a new session. */
  onStale?: (error: unknown) => void;
}>;

/** One applied query's page source, shared by web Browse and the native library.
 * Each query gets its own session: a new filter, sort or opening retires the old one,
 * so a late page, revision or descriptor can never land on the query that replaced it. */
export class BrowseSession<T> {
  readonly query: BrowseSessionQuery;
  private readonly options: BrowseSessionOptions<T>;
  private pinned?: string;
  private whole?: BrowseWholeSelection;
  private ended = false;
  private retired = false;
  private entries = new WeakSet<ContentEntry>();
  constructor(options: BrowseSessionOptions<T>) {
    this.options = options;
    this.query = options.query;
  }
  private live(): boolean {return !this.retired && !this.ended && this.options.current();}
  /** The page token every page of this query carries, once the first one has replied. */
  get revision(): string | undefined {return this.live() ? this.pinned : undefined;}
  get stale(): boolean {return this.ended;}
  /** The exact whole-view descriptor while this query's pages are current. */
  wholeSelection(): BrowseWholeSelection | undefined {return this.live() && this.pinned ? this.whole : undefined;}
  /** The entry arrived in one of this query's pages, and the query is still current. */
  owns(entry: ContentEntry): boolean {return this.live() && this.entries.has(entry);}
  retire(): void {this.retired = true; this.entries = new WeakSet(); this.whole = undefined;}
  /** Give up the held descriptor (a retry is unsure of it); only a later first page reinstalls it. */
  forgetWholeSelection(): void {this.whole = undefined;}
  private end(error: unknown): void {
    if (this.retired || this.ended) return;
    this.ended = true; this.entries = new WeakSet(); this.whole = undefined;
    this.options.onStale?.(error);
  }
  fetchPage: FetchPage<T> = async (start, count, signal) => {
    if (this.retired || !this.options.current()) throw browseSessionClosed();
    if (this.ended) throw browseRevisionChanged();
    const body = browseSessionBuilder(this.query).limit(count).range({start, ...(this.pinned ? {revision: this.pinned} : {})}).build();
    let raw: unknown;
    try {raw = await this.options.request(browsePath(this.query.libraryId), 'POST', body, signal);}
    catch (error) {
      if (!this.retired && this.options.current() && !signal.aborted && ended(error)) this.end(error);
      throw error;
    }
    if (this.retired || !this.options.current() || signal.aborted) throw browseSessionClosed();
    const page = withLibraryScope(parseBrowseResult(raw), this.query.libraryId);
    if (page.pivot !== this.query.pivot || page.pageInfo.start !== start || page.entries.length > count
      || !page.pageInfo.revision || this.pinned !== undefined && page.pageInfo.revision !== this.pinned) {
      const error = browseRevisionChanged();
      this.end(error);
      throw error;
    }
    const first = this.pinned === undefined;
    this.pinned = page.pageInfo.revision;
    for (const entry of page.entries) this.entries.add(entry);
    // The descriptor is first-page-only and source-bound: a later page never installs or
    // replaces it, and one for another library is not this view even with a matching revision.
    if (start === 0 && page.wholeSelection !== undefined && page.wholeSelection.libraryId === this.query.libraryId) {
      this.whole = page.wholeSelection;
      this.options.onWhole?.(page.wholeSelection);
    }
    if (first) this.options.onRevision?.(this.pinned);
    this.options.onPage?.(page);
    if (this.retired || !this.options.current()) throw browseSessionClosed();
    return {items: page.entries.map((entry, index) => this.options.item(entry, page, index)),
      total: page.pageInfo.total, totalIsLowerBound: page.pageInfo.totalIsLowerBound,
      positionIndex: Object.fromEntries(page.positionIndex.map(anchor => [anchor.key, anchor.index]))};
  };
}
