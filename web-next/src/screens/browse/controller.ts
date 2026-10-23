import {BrowseQueryBuilder, browseCapabilitiesPath, browsePath, browseMembershipId, parseBrowseCapabilities, parseBrowseResult, browseContextFacetsPath, parseBrowseFacets, browseCountsPath, browseCountsRequest, parseBrowseCounts, parseBrowseNode,
  type BrowseCapabilities, type BrowseNode, type BrowseSortSelection} from '@core/browse.ts';
import {createWindowedCollection, type WindowedCollection} from '@core/collections/windowed.ts';
import type {ContentEntry, LibraryContentApi} from '@core/library-content.ts';
import {isStaleAnswer} from '@core/freshness-actions.ts';

export type BrowseMember = Readonly<{membershipId: string; entry: ContentEntry}>;
export type BrowseSource = Readonly<{libraryId: string; pivot: string; base?: readonly BrowseNode[]; homeRowId?: string; sort?: BrowseSortSelection}>;
export type BrowseState = Readonly<{phase: 'loading' | 'ready' | 'error'; capabilities?: BrowseCapabilities; predicates: readonly BrowseNode[]; sort?: BrowseSortSelection; error?: unknown}>;
const closed = () => Object.assign(new Error('browse-scope-closed'), {name: 'AbortError'});

/** Private screen controller. The parent owns the authenticated API and exact viewer lifetime. */
export class BrowseController {
  readonly collection: WindowedCollection<BrowseMember>;
  private state: BrowseState = Object.freeze({phase: 'loading', predicates: []});
  private listeners = new Set<() => void>();
  private life = new AbortController();
  private epoch = 0;
  constructor(private api: LibraryContentApi, readonly source: BrowseSource, private isCurrent: () => boolean) {
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
      const sort = this.source.sort ?? pivot?.defaultSort[0];
      if (!pivot?.browsable || !sort || caps.queryLimits.maximumLimit < 60) throw new Error('browse-capability-required');
      this.publish({phase: 'ready', capabilities: caps, sort});
      this.reset();
    } catch (error) { if (epoch === this.epoch) this.publish({phase: 'error', error}); }
  }
  setQuery(predicates: readonly BrowseNode[], sort: BrowseSortSelection = this.state.sort!) {
    if (!this.current() || !this.state.capabilities || !sort) return;
    this.builder(this.state.capabilities, predicates, sort).build();
    this.publish({predicates: Object.freeze([...predicates]), sort});
    this.reset();
  }
  private builder(caps: BrowseCapabilities, predicates: readonly BrowseNode[], sort: BrowseSortSelection) {
    const builder = new BrowseQueryBuilder(caps, this.source.pivot);
    for (const node of [...(this.source.base ?? []), ...predicates]) builder.add(node);
    builder.sort(sort.field, sort.direction, sort.seed);
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
    const {capabilities: caps, sort} = this.state;
    if (!caps || !sort || !this.current()) throw closed();
    const epoch = this.epoch, body = this.builder(caps, predicates, sort).build();
    const raw = await this.api.request(browseCountsPath(this.source.libraryId), 'POST', browseCountsRequest(this.source.pivot, [{id: 'draft', query: body.query ?? null}]), signal);
    if (!this.current() || epoch !== this.epoch || signal.aborted) throw closed();
    return parseBrowseCounts(raw, ['draft'])[0];
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
