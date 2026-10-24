import {BrowseQueryBuilder, browseCapabilitiesPath, browsePath, browseMembershipId, parseBrowseCapabilities, parseBrowseResult, browseContextFacetsPath, parseBrowseFacets, browseCountsPath, browseCountsRequest, parseBrowseCounts, parseBrowseNode,
  type BrowseCapabilities, type BrowseNode, type BrowseSortSelection} from '@core/browse.ts';
import {createWindowedCollection, type WindowedCollection} from '@core/collections/windowed.ts';
import type {ContentEntry, LibraryContentApi} from '@core/library-content.ts';
import {isStaleAnswer} from '@core/freshness-actions.ts';

export type BrowseMember = Readonly<{membershipId: string; entry: ContentEntry}>;
export type BrowseSource = Readonly<{libraryId: string; pivot: string; base?: readonly BrowseNode[]; predicates?: readonly BrowseNode[]; homeRowId?: string; sort?: BrowseSortSelection; sorts?: readonly BrowseSortSelection[]}>;
export type BrowseState = Readonly<{phase: 'loading' | 'ready' | 'error'; capabilities?: BrowseCapabilities; predicates: readonly BrowseNode[]; sort?: BrowseSortSelection; error?: unknown}>;
const closed = () => Object.assign(new Error('browse-scope-closed'), {name: 'AbortError'});

/** Private screen controller. The parent owns the authenticated API and exact viewer lifetime. */
export class BrowseController {
  readonly collection: WindowedCollection<BrowseMember>;
  private state: BrowseState = Object.freeze({phase: 'loading', predicates: []});
  private listeners = new Set<() => void>();
  private life = new AbortController();
  private epoch = 0;
  private sorts: readonly BrowseSortSelection[] = [];
  constructor(private api: LibraryContentApi, readonly source: BrowseSource, private isCurrent: () => boolean) {
    this.state = Object.freeze({phase: 'loading', predicates: Object.freeze([...(source.predicates ?? [])])});
    this.collection = createWindowedCollection({pageSize: 60, maxResidentPages: 6, maxConcurrent: 1, prefetchPages: 1,
      keyOf: (item: BrowseMember) => item.membershipId, fetchPage: async () => { throw closed(); }});
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private current() { return !this.life.signal.aborted && this.isCurrent(); }
  private publish(patch: Partial<BrowseState>) {
    if (!this.current()) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const listener of this.listeners) listener();
  }
  async load() {
    if (!this.current()) return;
    const epoch = ++this.epoch;
    this.publish({phase: 'loading', error: undefined});
    try {
      const caps = parseBrowseCapabilities(await this.api.request(browseCapabilitiesPath(this.source.libraryId, this.source.pivot), 'GET', undefined, this.life.signal));
      if (!this.current() || epoch !== this.epoch) return;
      const pivot = caps.resolvedPivot ?? caps.pivots.find(pivot => pivot.id === this.source.pivot);
      this.sorts = this.source.sorts ?? (this.source.sort ? [this.source.sort] : pivot?.defaultSort ?? []);
      const sort = this.sorts[0];
      if (!pivot?.browsable || !pivot.supportedViews.some(view => view === 'grid' || view === 'list') || !sort || caps.queryLimits.maximumLimit < 60) throw new Error('browse-capability-required');
      this.publish({phase: 'ready', capabilities: caps, sort});
      this.reset();
    } catch (error) { if (epoch === this.epoch) this.publish({phase: 'error', error}); }
  }
  setQuery(predicates: readonly BrowseNode[], sort: BrowseSortSelection = this.state.sort!) {
    if (!this.current() || !this.state.capabilities || !sort) return;
    this.builder(this.state.capabilities, predicates, sort).build();
    if (sort !== this.state.sort) this.sorts = [sort];
    this.publish({predicates: Object.freeze([...predicates]), sort});
    this.reset();
  }
  private builder(caps: BrowseCapabilities, predicates: readonly BrowseNode[], sort: BrowseSortSelection) {
    const builder = new BrowseQueryBuilder(caps, this.source.pivot);
    for (const node of [...(this.source.base ?? []), ...predicates]) builder.add(node);
    for (const selected of sort === this.state.sort && this.sorts.length ? this.sorts : [sort]) builder.sort(selected.field, selected.direction, selected.seed);
    if (this.source.homeRowId) builder.displayHomeRow(this.source.homeRowId);
    return builder.limit(60);
  }
  private reset() {
    const {capabilities: caps, predicates, sort} = this.state;
    if (!caps || !sort) return;
    const epoch = ++this.epoch;
    let revision: string | undefined;
    this.collection.invalidate(async (start, count, signal) => {
      if (!this.current() || epoch !== this.epoch) throw closed();
      const body = this.builder(caps, predicates, sort).limit(count).range({start, ...(revision ? {revision} : {})}).build();
      const page = parseBrowseResult(await this.api.request(browsePath(this.source.libraryId), 'POST', body, signal));
      if (!this.current() || epoch !== this.epoch || signal.aborted) throw closed();
      if (page.pivot !== this.source.pivot || page.pageInfo.start !== start || revision && revision !== page.pageInfo.revision) throw Object.assign(new Error('browse-page-revision-changed'), {retry: 'after_refresh'});
      revision = page.pageInfo.revision;
      return {items: page.entries.map((entry, index) => ({membershipId: browseMembershipId(page, index)!, entry})),
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
    if (this.state.phase === 'error') void this.load();
    else if (isStaleAnswer(this.collection.getSnapshot().error)) this.reset();
    else this.collection.retry();
  };
  dispose() { this.life.abort(); this.epoch++; this.collection.dispose(); this.listeners.clear(); }
}
