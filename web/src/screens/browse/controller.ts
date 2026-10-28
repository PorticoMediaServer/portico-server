import {BrowseQueryBuilder, browseCapabilitiesPath, browsePath, browseMembershipId, parseBrowseCapabilities, parseBrowseResult, browseContextFacetsPath, parseBrowseFacets, browseCountsPath, browseCountsRequest, parseBrowseCounts, parseBrowseNode, type BrowseResult,
  type BrowseCapabilities, type BrowseNode, type BrowseSortSelection, type BrowseWholeSelection} from '@core/browse.ts';
import {createWindowedCollection, type WindowedCollection} from '@core/collections/windowed.ts';
import type {ContentEntry, LibraryContentApi} from '@core/library-content.ts';
import {isStaleAnswer} from '@core/freshness-actions.ts';

export type BrowseMember = Readonly<{membershipId: string; entry: ContentEntry}>;
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

export type BrowseSource = Readonly<{libraryId: string; pivot: string; base?: readonly BrowseNode[]; predicates?: readonly BrowseNode[]; homeRowId?: string; sort?: BrowseSortSelection; sorts?: readonly BrowseSortSelection[]}>;
export type BrowseState = Readonly<{phase: 'loading' | 'ready' | 'error'; capabilities?: BrowseCapabilities; predicates: readonly BrowseNode[]; sort?: BrowseSortSelection; error?: unknown;
  /** Actual opaque page token, absent until this applied query has replied. */
  revision?: string; queryRevision: number}>;
const closed = () => Object.assign(new Error('browse-scope-closed'), {name: 'AbortError'});
const changed = () => Object.assign(new Error('browse-page-revision-changed'), {retry: 'after_refresh'});
function captureNode(input: BrowseNode): BrowseNode {
  const freeze = (node: BrowseNode): BrowseNode => {
    if ('field' in node) return Object.freeze({...node, value: Array.isArray(node.value) ? Object.freeze([...node.value]) : node.value});
    if ('all' in node) return Object.freeze({all: Object.freeze(node.all.map(freeze))});
    if ('any' in node) return Object.freeze({any: Object.freeze(node.any.map(freeze))});
    return Object.freeze({not: freeze(node.not)});
  };
  return freeze(parseBrowseNode(input));
}

/** Private screen controller. The parent owns the authenticated API and exact viewer lifetime. */
export class BrowseController {
  readonly collection: WindowedCollection<BrowseMember>;
  readonly source: BrowseSource;
  private state: BrowseState = Object.freeze({phase: 'loading', predicates: [], queryRevision: 0});
  private issued = new WeakSet<BrowseState>();
  private listeners = new Set<() => void>();
  private life = new AbortController();
  private epoch = 0;
  private sorts: readonly BrowseSortSelection[] = [];
  private members = new WeakMap<ContentEntry, number>();
  private whole?: BrowseWholeSelection;
  constructor(private api: LibraryContentApi, source: BrowseSource, private isCurrent: () => boolean) {
    this.source = Object.freeze({...source,
      ...(source.base ? {base: Object.freeze(source.base.map(captureNode))} : {}),
      ...(source.predicates ? {predicates: Object.freeze(source.predicates.map(captureNode))} : {}),
      ...(source.sort ? {sort: Object.freeze({...source.sort})} : {}),
      ...(source.sorts ? {sorts: Object.freeze(source.sorts.map(sort => Object.freeze({...sort})))} : {})});
    this.state = Object.freeze({phase: 'loading', predicates: this.source.predicates ?? Object.freeze([]), queryRevision: 0});
    this.collection = createWindowedCollection({pageSize: 60, maxResidentPages: 6, maxConcurrent: 1, prefetchPages: 1,
      keyOf: (item: BrowseMember) => item.membershipId, fetchPage: async () => { throw closed(); }});
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private current() { return !this.life.signal.aborted && this.isCurrent(); }
  /** Borrowed row/menu actions require the same current resident object.
   * Explicit selection has its own retained picks and operation lifetime. */
  contains(entry: ContentEntry): boolean {
    return this.current() && this.state.phase === 'ready' && !!this.state.revision && this.members.get(entry) === this.epoch
      && this.collection.residentItems().some(member => member.entry === entry);
  }
  /** The server's exact whole-view descriptor for the page this controller holds.
   * It is present only while that same page revision is the current one; any
   * invalidate, retry or revision change drops it before a new one can arrive. */
  wholeSelection(): BrowseWholeSelection | undefined {
    return this.current() && this.state.phase === 'ready' && !!this.state.revision ? this.whole : undefined;
  }
  private publish(patch: Partial<BrowseState>) {
    if (!this.current()) return;
    this.state = Object.freeze({...this.state, ...patch});
    this.issued.add(this.state);
    for (const listener of this.listeners) listener();
  }
  async load() {
    if (!this.current()) return;
    const epoch = ++this.epoch;
    this.whole = undefined;
    this.publish({phase: 'loading', error: undefined, revision: undefined});
    try {
      const caps = parseBrowseCapabilities(await this.api.request(browseCapabilitiesPath(this.source.libraryId, this.source.pivot), 'GET', undefined, this.life.signal));
      if (!this.current() || epoch !== this.epoch) return;
      const pivot = caps.resolvedPivot ?? caps.pivots.find(pivot => pivot.id === this.source.pivot);
      this.sorts = this.source.sorts ?? (this.source.sort ? [this.source.sort] : pivot?.defaultSort ?? []);
      const sort = this.sorts[0];
      if (!pivot?.browsable || !pivot.supportedViews.some(view => view === 'grid' || view === 'list') || !sort || caps.queryLimits.maximumLimit < 60) throw new Error('browse-capability-required');
      this.publish({phase: 'ready', capabilities: caps, sort});
      if (!this.current() || epoch !== this.epoch) return;
      this.reset();
    } catch (error) { if (epoch === this.epoch) this.publish({phase: 'error', error}); }
  }
  acceptsQuery(original: BrowseState): boolean {
    return this.current() && this.issued.has(original) && original.phase === 'ready' && this.state.phase === 'ready'
      && this.state.queryRevision === original.queryRevision && this.state.capabilities === original.capabilities
      && this.state.predicates === original.predicates && this.state.sort === original.sort;
  }
  setQuery(predicates: readonly BrowseNode[], sort?: BrowseSortSelection, original: BrowseState = this.state): boolean {
    // A rendered control or delayed draft belongs to its original applied query.
    // Page/count publication may advance without replacing that query; another
    // query, capabilities or opening must never lend the callback its state.
    const current = () => this.acceptsQuery(original);
    if (!current() || !original.capabilities) return false;
    const chosen = sort ?? original.sort;
    if (!chosen) return false;
    const captured = Object.freeze(predicates.map(captureNode)), selected = chosen === original.sort ? chosen : Object.freeze({...chosen});
    this.builder(original.capabilities, captured, selected).build();
    // Capturing/validating caller-owned input cannot re-enter and then overwrite
    // a query accepted in the meantime.
    if (!current()) return false;
    if (selected !== original.sort) this.sorts = [selected];
    this.reset({predicates: captured, sort: selected});
    return true;
  }
  private builder(caps: BrowseCapabilities, predicates: readonly BrowseNode[], sort: BrowseSortSelection) {
    const builder = new BrowseQueryBuilder(caps, this.source.pivot);
    for (const node of [...(this.source.base ?? []), ...predicates]) builder.add(node);
    for (const selected of sort === this.state.sort && this.sorts.length ? this.sorts : [sort]) builder.sort(selected.field, selected.direction, selected.seed);
    if (this.source.homeRowId) builder.displayHomeRow(this.source.homeRowId);
    return builder.limit(60);
  }
  private reset(next: Partial<Pick<BrowseState, 'predicates' | 'sort'>> = {}) {
    const {capabilities: caps, predicates, sort} = {...this.state, ...next};
    if (!caps || !sort) return;
    const epoch = ++this.epoch;
    this.members = new WeakMap();
    this.whole = undefined;
    this.publish({...next, revision: undefined, queryRevision: this.state.queryRevision + 1});
    if (!this.current() || epoch !== this.epoch) return;
    let revision: string | undefined, stale = false;
    this.collection.invalidate(async (start, count, signal) => {
      if (!this.current() || epoch !== this.epoch) throw closed();
      if (stale) throw changed();
      const body = this.builder(caps, predicates, sort).limit(count).range({start, ...(revision ? {revision} : {})}).build();
      const raw = await this.api.request(browsePath(this.source.libraryId), 'POST', body, signal).catch(error => {
        if (this.current() && epoch === this.epoch && !signal.aborted && (isStaleAnswer(error)
          || typeof error === 'object' && error !== null && 'status' in error && typeof error.status === 'number' && [401, 403, 404].includes(error.status))) {
          stale = true;this.members = new WeakMap();this.whole = undefined;this.publish({revision: undefined});
        }
        throw error;
      });
      const page = withLibraryScope(parseBrowseResult(raw), this.source.libraryId);
      if (!this.current() || epoch !== this.epoch || signal.aborted) throw closed();
      if (page.pivot !== this.source.pivot || page.pageInfo.start !== start || revision && revision !== page.pageInfo.revision) {
        stale = true;this.members = new WeakMap();this.whole = undefined;this.publish({revision: undefined});
        throw changed();
      }
      revision = page.pageInfo.revision;
      // The approved descriptor is first-page-only and source-bound. Later pages
      // never install or replace it, and a descriptor for another library is not
      // this controller's view even when its revision string matches.
      if (start === 0 && page.wholeSelection !== undefined && page.wholeSelection.libraryId === this.source.libraryId) this.whole = page.wholeSelection;
      this.publish({revision});
      return {items: page.entries.map((entry, index) => {this.members.set(entry, epoch);return Object.freeze({membershipId: browseMembershipId(page, index)!, entry});}),
        total: page.pageInfo.total, totalIsLowerBound: page.pageInfo.totalIsLowerBound,
        positionIndex: Object.fromEntries(page.positionIndex.map(anchor => [anchor.key, anchor.index]))};
    });
    this.collection.ensureRange(0, 0);
  }
  async facets(field: string, query: string, predicates: readonly BrowseNode[], signal: AbortSignal) {
    const {capabilities: caps, sort} = this.state;
    if (!caps || !sort || !this.current()) throw closed();
    const epoch = this.epoch, body = this.builder(caps, predicates, sort).build();
    const raw = await this.api.request(browseContextFacetsPath(this.source.libraryId, field, this.source.pivot, body.query, query, 200), 'GET', undefined, signal);
    if (!this.current() || epoch !== this.epoch || signal.aborted) throw closed();
    return parseBrowseFacets(raw, field);
  }
  async count(predicates: readonly BrowseNode[], signal: AbortSignal) {
    return (await this.counts([{id: 'draft', predicates}], signal))[0];
  }
  async counts(groups: readonly Readonly<{id: string; predicates: readonly BrowseNode[]}>[], signal: AbortSignal) {
    const {capabilities: caps, sort} = this.state;
    if (!caps || !sort || !this.current()) throw closed();
    const epoch = this.epoch;
    const queries = groups.map(group => ({id: group.id, query: this.builder(caps, group.predicates, sort).build().query ?? null}));
    const raw = await this.api.request(browseCountsPath(this.source.libraryId), 'POST', browseCountsRequest(this.source.pivot, queries), signal);
    if (!this.current() || epoch !== this.epoch || signal.aborted) throw closed();
    return parseBrowseCounts(raw, groups.map(group => group.id));
  }
  /** The same server query used by pages, counts, saved views and whole-group Play. */
  query() {
    const {capabilities, predicates, sort} = this.state;
    if (!this.current() || !capabilities || !sort) throw closed();
    return this.builder(capabilities, predicates, sort).build();
  }
  /** Fixed source membership is the default, not a user-applied filter. */
  canSaveView() {
    const {capabilities, predicates, sort, phase} = this.state;
    if (!this.current() || phase !== 'ready' || !capabilities || !sort) return false;
    const pivot = capabilities.resolvedPivot ?? capabilities.pivots.find(value => value.id === this.source.pivot);
    if (!pivot) return false;
    return predicates.length > 0 || this.sorts.length !== pivot.defaultSort.length || this.sorts.some((value, index) => {
      const base = pivot.defaultSort[index];
      return !base || value.field !== base.field || value.direction !== base.direction || value.seed !== base.seed;
    });
  }
  /** UI controls preserve compound predicates and validate every generated node. */
  parseNode = (value: unknown) => parseBrowseNode(value);
  retry = () => {
    this.whole = undefined;
    if (this.state.phase === 'error') void this.load();
    else if (isStaleAnswer(this.collection.getSnapshot().error)) this.reset();
    else this.collection.retry();
  };
  dispose() { this.life.abort(); this.epoch++; this.whole = undefined; this.collection.dispose(); this.listeners.clear(); }
}
