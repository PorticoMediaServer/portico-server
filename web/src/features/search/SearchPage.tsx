import { ArrowRight, Grid3X3, History, List, Search, Trash2, X } from '#portico-icons';
import { productMessage, type SearchGroupCapability } from '@porticomediaserver/client-core';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { IconButton, SecondaryButton } from '../../components/controls/Buttons';
import { SelectMenu } from '../../components/controls/SelectMenu';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { useAuthSession, useLibraries, usePorticoDataSource, useSearchContract } from '../../data/DataProvider';
import type { SearchDirection, SearchGroup, SearchHistoryItem, SearchPageInput, SearchSort } from '../../data/models';
import { useOptionalWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import { defaultWebDisplayPreferences, recordRecentSearch } from '../../preferences/webDisplayPreferences';
import { SectionHeading, SelectableMediaGrid } from '../catalog/CatalogSurface';
import { searchGroupFilterKinds, searchGroupMatchesSelection } from '../catalog/mediaPresentation';
import { RETAINED_RESULT_ITEM_BUDGET, retainedResultBudgetState } from '../library/retainedResultBudget';
import { SearchRequestPool } from './searchRequestPool';
import './search.css';

type SearchGroupState = {
  descriptor: SearchGroupCapability;
  status: 'loading' | 'success' | 'error';
  data?: SearchGroup;
  error?: Error;
  messageId?: SearchGroup['messageId'];
};

type SearchState =
  | { status: 'idle'; key?: undefined; groups?: undefined }
  | { status: 'loading' | 'success'; key: string; groups: SearchGroupState[] };

export function searchRequestIdentity(request: Pick<SearchPageInput, 'query' | 'entityKinds' | 'libraryIds' | 'sort' | 'direction'>) {
  return JSON.stringify({
    query: request.query.trim(),
    kinds: [...(request.entityKinds ?? [])].sort(),
    libraries: [...(request.libraryIds ?? [])].sort(),
    sort: request.sort ?? 'relevance',
    direction: request.direction ?? 'desc',
  });
}

function searchGroupFailure(messageId?: SearchGroup['messageId']) {
  return productMessage(messageId ?? 'search.group-unavailable');
}

function searchGroupArtworkShape(descriptor: SearchGroupCapability): 'square' | 'poster' | 'landscape' {
  const kinds = [descriptor.entityKind, ...descriptor.resultKinds];
  if (kinds.some((kind) => ['artist', 'album', 'track', 'person'].includes(kind))) return 'square';
  if (kinds.length > 0 && kinds.every((kind) => kind === 'episode')) return 'landscape';
  return 'poster';
}

export function SearchGroupSurface({ group, request, view, requestStatus, requestMessageId, onRetryRequest }: {
  group: SearchGroup;
  request: SearchPageInput;
  view: 'grid' | 'list';
  requestStatus?: 'loading' | 'error';
  requestMessageId?: SearchGroup['messageId'];
  onRetryRequest?: () => void;
}) {
  const source = usePorticoDataSource();
  const [current, setCurrent] = useState(group);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [errorMessageId, setErrorMessageId] = useState<SearchGroup['messageId']>();
  const generation = useRef(0);
  const continuation = useRef<AbortController | undefined>(undefined);
  const requestKey = `${searchRequestIdentity(request)}:${group.id}`;
  useEffect(() => {
    generation.current += 1;
    continuation.current?.abort();
    setCurrent(group);
    setLoading(false);
    setError(false);
    setErrorMessageId(undefined);
    return () => continuation.current?.abort();
  }, [group, requestKey]);
  const loadMore = async () => {
    if (!current.nextCursor || loading) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    setLoading(true);
    setError(false);
    setErrorMessageId(undefined);
    try {
      const response = await source.searchPage({ ...request, group: current.id, cursor: current.nextCursor }, controller.signal);
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      const nextGroup = response.groups.find((candidate) => candidate.id === current.id) ?? response.groups[0];
      if (!nextGroup) {
        setCurrent((value) => ({ ...value, hasMore: false, nextCursor: null }));
        return;
      }
      if (nextGroup.status === 'error') {
        setErrorMessageId(nextGroup.messageId);
        setError(true);
        return;
      }
      setCurrent((value) => {
        const known = new Set(value.items.map((item) => item.id));
        return { ...nextGroup, items: [...value.items, ...nextGroup.items.filter((item) => !known.has(item.id))] };
      });
    } catch {
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setError(true);
    } finally {
      if (activeGeneration === generation.current) setLoading(false);
    }
  };
  const failure = searchGroupFailure(errorMessageId);
  const requestFailure = searchGroupFailure(requestMessageId);
  const resultCount = productMessage('search.results-count', { count: `${current.items.length}${current.hasMore ? '+' : ''}` }).text;
  const loadMoreLabel = productMessage('action.load-more-group', { group: current.title.toLocaleLowerCase() }).text;
  return <section className="full-search-group" aria-busy={requestStatus === 'loading' || undefined} data-retained-result-count={current.items.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(current.items.length)}>
    <SectionHeading title={current.title} detail={resultCount} />
    <SelectableMediaGrid items={current.items} view={view} className={`full-search-results ${current.entityKind === 'person' ? 'person-search-results' : ''}`} playbackContext={{ type: 'search', id: current.id, title: request.query }} />
    {requestStatus === 'loading' && <span className="sr-only">{productMessage('search.loading').title}</span>}
    {requestStatus === 'error' && <div className="search-group-error" role="status"><ProductLanguageIcon presentation={requestFailure} /> <span>{requestFailure.body}</span><button type="button" onClick={onRetryRequest}>{requestFailure.actions[0]?.label}</button></div>}
    {error && <div className="search-group-error" role="status"><ProductLanguageIcon presentation={failure} /> <span>{failure.body}</span><button type="button" onClick={() => void loadMore()}>{failure.actions[0]?.label}</button></div>}
    {current.hasMore && current.nextCursor && <button type="button" className="search-more" disabled={loading} onClick={() => void loadMore()}>{loading ? productMessage('state.loading-more').title : loadMoreLabel}</button>}
  </section>;
}

function SearchGroupRequestStatus({ state, retry }: { state: SearchGroupState; retry: () => void }) {
  const failure = searchGroupFailure(state.messageId);
  const artworkShape = searchGroupArtworkShape(state.descriptor);
  return <section className={`full-search-group search-group-request-status is-${artworkShape}`} data-artwork-shape={artworkShape} aria-label={state.descriptor.title} aria-busy={state.status === 'loading' || undefined}>
    <SectionHeading title={state.descriptor.title} />
    {state.status === 'loading'
      ? <span className="sr-only">{productMessage('search.loading').title}</span>
      : <div className="search-group-error" role="status"><ProductLanguageIcon presentation={failure} /> <span>{failure.body}</span><button type="button" onClick={retry}>{failure.actions[0]?.label}</button></div>}
  </section>;
}

export function SearchPage() {
  const auth = useAuthSession();
  const source = usePorticoDataSource();
  const display = useOptionalWebDisplayPreferences();
  const displayPreferences = display?.preferences ?? defaultWebDisplayPreferences;
  const searchContract = useSearchContract();
  const [params, setParams] = useSearchParams();
  const committedQuery = params.get('q')?.trim() ?? '';
  const selectedKinds = useMemo(() => params.get('types')?.split(',').filter(Boolean) ?? [], [params]);
  const selectedLibrary = params.get('library') ?? '';
  const sortDescriptors = searchContract.status === 'success' ? searchContract.data.sorts : [];
  const defaultSortDescriptor = sortDescriptors.find((sort) => sort.id === 'relevance') ?? sortDescriptors[0];
  const selectedSortDescriptor = sortDescriptors.find((sort) => sort.id === params.get('sort')) ?? defaultSortDescriptor;
  const selectedSort = (selectedSortDescriptor?.id ?? 'relevance') as SearchSort;
  const requestedDirection = params.get('direction');
  const selectedDirection = (selectedSortDescriptor?.directions.includes(requestedDirection as SearchDirection)
    ? requestedDirection
    : selectedSortDescriptor?.defaultDirection ?? 'desc') as SearchDirection;
  const view = params.get('view') === 'list' ? 'list' : 'grid';
  const [draft, setDraft] = useState(committedQuery);
  const [state, setState] = useState<SearchState>({ status: 'idle' });
  const [serverHistory, setServerHistory] = useState<SearchHistoryItem[]>();
  const [historyError, setHistoryError] = useState(false);
  const lastRecordedQuery = useRef('');
  const searchGeneration = useRef(0);
  const retrySequence = useRef(0);
  const groupRequests = useRef(new Map<string, AbortController>());
  const requestPool = useRef(new SearchRequestPool(2));
  const libraries = useLibraries();
  useEffect(() => {
    if (committedQuery || !displayPreferences.rememberSearchHistory) return;
    const controller = new AbortController();
    setHistoryError(false);
    source.searchHistory(controller.signal).then(
      (items) => !controller.signal.aborted && setServerHistory(items),
      () => !controller.signal.aborted && setHistoryError(true),
    );
    return () => controller.abort();
  }, [committedQuery, displayPreferences.rememberSearchHistory, source]);
  useEffect(() => setDraft(committedQuery), [committedQuery]);
  const request = useMemo<SearchPageInput>(() => ({
    query: committedQuery,
    entityKinds: selectedKinds.length ? selectedKinds : undefined,
    libraryIds: selectedLibrary ? [selectedLibrary] : undefined,
    sort: selectedSort,
    direction: selectedDirection,
    limit: searchContract.status === 'success' ? searchContract.data.limits.fullDefaultGroupLimit : 50,
  }), [committedQuery, searchContract, selectedKinds.join(','), selectedLibrary, selectedSort, selectedDirection, selectedSortDescriptor]);
  const requestKey = searchRequestIdentity(request);
  const groupDescriptors = useMemo(() => {
    if (searchContract.status !== 'success') return [];
    const byID = new Map(searchContract.data.groups.map((group) => [group.id, group]));
    return searchContract.data.groupOrder
      .map((id) => byID.get(id))
      .filter((group): group is SearchGroupCapability => Boolean(group))
      .filter((group) => selectedSortDescriptor?.applicableGroups.includes(group.id) ?? true)
      .filter((group) => !selectedLibrary || group.supportsLibraryScope)
      .filter((group) => searchGroupMatchesSelection(group, selectedKinds));
  }, [searchContract, selectedKinds.join(','), selectedLibrary, selectedSortDescriptor]);
  const settleGroup = useCallback((activeGeneration: number, activeKey: string, groupID: string, outcome: { data: SearchGroup } | { error: Error; messageId?: SearchGroup['messageId'] }) => {
    if (activeGeneration !== searchGeneration.current) return;
    setState((current) => {
      if (current.status === 'idle' || current.key !== activeKey) return current;
      const groups = current.groups.map((group) => group.descriptor.id === groupID
        ? ('data' in outcome
          ? { ...group, status: 'success' as const, data: outcome.data, error: undefined }
          : { ...group, status: 'error' as const, error: outcome.error, messageId: outcome.messageId })
        : group);
      return { status: groups.some((group) => group.status === 'loading') ? 'loading' : 'success', key: activeKey, groups };
    });
  }, []);
  const loadGroup = useCallback(async (descriptor: SearchGroupCapability, activeGeneration: number, activeKey: string, recordHistory: boolean) => {
    groupRequests.current.get(descriptor.id)?.abort();
    const controller = new AbortController();
    groupRequests.current.set(descriptor.id, controller);
    try {
      const response = await source.searchPage({ ...request, group: descriptor.id, recordHistory }, controller.signal);
      if (controller.signal.aborted || activeGeneration !== searchGeneration.current) return;
      const data = response.groups.find((group) => group.id === descriptor.id) ?? {
        id: descriptor.id,
        title: descriptor.title,
        entityKind: descriptor.entityKind,
        status: 'success' as const,
        items: [],
        hasMore: false,
        nextCursor: null,
      };
      if (recordHistory) lastRecordedQuery.current = committedQuery;
      if (data.status === 'error') {
        settleGroup(activeGeneration, activeKey, descriptor.id, { error: new Error(data.errorCode ?? 'Search group unavailable.'), messageId: data.messageId });
        return;
      }
      settleGroup(activeGeneration, activeKey, descriptor.id, { data });
    } catch (reason: unknown) {
      if (controller.signal.aborted || activeGeneration !== searchGeneration.current) return;
      settleGroup(activeGeneration, activeKey, descriptor.id, { error: reason instanceof Error ? reason : new Error('Search is unavailable.'), messageId: 'search.group-unavailable' });
    } finally {
      if (groupRequests.current.get(descriptor.id) === controller) groupRequests.current.delete(descriptor.id);
    }
  }, [committedQuery, request, settleGroup, source]);
  useEffect(() => {
    if (!committedQuery || searchContract.status !== 'success') {
      searchGeneration.current += 1;
      requestPool.current.clear();
      groupRequests.current.forEach((controller) => controller.abort());
      groupRequests.current.clear();
      setState({ status: 'idle' });
      return;
    }
    const activeGeneration = searchGeneration.current + 1;
    searchGeneration.current = activeGeneration;
    requestPool.current.clear();
    groupRequests.current.forEach((controller) => controller.abort());
    groupRequests.current.clear();
    setState((current) => {
      const retained = current.status !== 'idle' && current.key === requestKey
        ? new Map(current.groups.map((group) => [group.descriptor.id, group.data]))
        : new Map<string, SearchGroup | undefined>();
      return {
        status: groupDescriptors.length ? 'loading' : 'success',
        key: requestKey,
        groups: groupDescriptors.map((descriptor) => ({ descriptor, status: 'loading', data: retained.get(descriptor.id) })),
      };
    });
    const shouldRecord = displayPreferences.rememberSearchHistory
      && lastRecordedQuery.current.toLocaleLowerCase() !== committedQuery.toLocaleLowerCase();
    groupDescriptors.forEach((descriptor, index) => {
      requestPool.current.enqueue(`${activeGeneration}:${descriptor.id}`, () => loadGroup(descriptor, activeGeneration, requestKey, shouldRecord && index === 0));
    });
    return () => {
      requestPool.current.clear();
      groupRequests.current.forEach((controller) => controller.abort());
      groupRequests.current.clear();
    };
  }, [committedQuery, displayPreferences.rememberSearchHistory, groupDescriptors, loadGroup, requestKey, searchContract.status]);

  const retryGroup = (descriptor: SearchGroupCapability) => {
    const activeGeneration = searchGeneration.current;
    setState((current) => {
      if (current.status === 'idle' || current.key !== requestKey) return current;
      const groups = current.groups.map((group) => group.descriptor.id === descriptor.id ? { ...group, status: 'loading' as const, error: undefined } : group);
      return { status: 'loading', key: current.key, groups };
    });
    retrySequence.current += 1;
    requestPool.current.enqueue(`${activeGeneration}:${descriptor.id}:retry:${retrySequence.current}`, () => loadGroup(descriptor, activeGeneration, requestKey, false));
  };

  const updateParams = (changes: Record<string, string | undefined>) => {
    const next = new URLSearchParams(params);
    for (const [key, value] of Object.entries(changes)) value ? next.set(key, value) : next.delete(key);
    setParams(next);
  };
  const submit = () => {
    const query = draft.trim();
    if (query && display?.preferences.rememberSearchHistory) {
      const recentSearches = recordRecentSearch(display.preferences.recentSearches, query);
      if (recentSearches.join('\n') !== display.preferences.recentSearches.join('\n')) void display.patch({ recentSearches }).catch(() => undefined);
    }
    updateParams({ q: query || undefined });
  };
  const toggleKinds = (kinds: readonly string[]) => {
    const selected = new Set(selectedKinds);
    const remove = kinds.every((kind) => selected.has(kind));
    for (const kind of kinds) remove ? selected.delete(kind) : selected.add(kind);
    const next = [...selected];
    updateParams({ types: next.length ? next.join(',') : undefined });
  };
  const changeSort = (id: string) => {
    const descriptor = sortDescriptors.find((sort) => sort.id === id);
    if (!descriptor) return;
    const sort = descriptor.id as SearchSort;
    const applicableKinds = new Set<string>((searchContract.status === 'success' ? searchContract.data.groups : []).filter((group) => descriptor.applicableGroups.includes(group.id)).flatMap((group) => [group.id, group.entityKind, ...group.resultKinds]));
    const nextKinds = sort === 'relevance' ? selectedKinds : selectedKinds.filter((kind) => applicableKinds.has(kind));
    updateParams({
      sort: sort === 'relevance' ? undefined : sort,
      direction: sort === 'relevance' ? undefined : sort === selectedSort ? selectedDirection : descriptor.defaultDirection,
      types: nextKinds.length ? nextKinds.join(',') : undefined,
    });
  };
  const changeDirection = (direction: string) => updateParams({ direction });
  const libraryOptions = libraries.status === 'success'
    ? [{ id: '', label: productMessage('search.all-libraries').text ?? '' }, ...libraries.data.map((library) => ({ id: library.id, label: library.name }))]
    : [{ id: '', label: productMessage('search.all-libraries').text ?? '' }];
  const groupStates = state.status === 'idle' ? [] : state.groups;
  const groups = groupStates.flatMap((group) => group.data ? [group.data] : []);
  const total = groups.reduce((count, group) => count + group.items.length, 0);
  const failedGroups = groupStates.filter((group) => group.status === 'error');
  const settledGroups = groupStates.filter((group) => group.status !== 'loading');
  const filtered = selectedKinds.length > 0 || Boolean(selectedLibrary) || selectedSort !== 'relevance';
  const entityKindFilter = searchContract.status === 'success' ? searchContract.data.filters.find((filter) => filter.id === 'entityKinds') : undefined;
  const allowedKinds = new Set(entityKindFilter?.allowedValues ?? []);
  const kindOptions = searchContract.status === 'success'
    ? searchContract.data.groups.flatMap((group) => {
      const values = searchGroupFilterKinds(group, allowedKinds);
      if (values.length === 0 && allowedKinds.has(group.id)) values.push(group.id);
      return values.length ? [{ id: group.id, label: group.title, values }] : [];
    }).filter((option, index, options) => options.findIndex((candidate) => candidate.id === option.id) === index)
    : [];
  const excludedSortGroups = searchContract.status === 'success' && selectedSortDescriptor
    ? searchContract.data.groups.filter((group) => !selectedSortDescriptor.applicableGroups.includes(group.id)).map((group) => group.title)
    : [];
  const recentSearches = serverHistory?.map((item) => item.query) ?? displayPreferences.recentSearches;
  const loadingMessage = productMessage('search.loading');
  const emptyMessage = productMessage('search.no-results');
  const filteredEmptyMessage = productMessage('search.filtered-empty');
  const historyFailure = productMessage('search.offline');
  const contractFailure = searchContract.status === 'error' ? productLanguageProblem(searchContract.error) : undefined;
  const pageMessage = productMessage('search.page');
  const clearHistory = async () => {
    setHistoryError(false);
    try {
      await source.clearSearchHistory(new AbortController().signal);
      setServerHistory([]);
      await display?.patch({ recentSearches: [] });
    } catch {
      setHistoryError(true);
    }
  };

  return <div className="standard-page full-search-page">
    <header className="full-search-header"><div><h1>{pageMessage.title}</h1><p>{pageMessage.body}</p></div></header>
    <form className="full-search-field" role="search" onSubmit={(event) => { event.preventDefault(); submit(); }}>
      <Search /><input type="search" autoFocus value={draft} onChange={(event) => setDraft(event.target.value)} placeholder={productMessage('search.input-placeholder', { serverName: auth.viewer?.serverName ?? productMessage('search.default-server-name').text ?? '' }).text} aria-label={productMessage('search.input-label').text} />
      {draft && <button type="button" aria-label={productMessage('action.clear-search').text} onClick={() => { setDraft(''); updateParams({ q: undefined }); }}><X /></button>}
      <kbd>{productMessage('search.submit-hint').text}</kbd>
    </form>
    {committedQuery && searchContract.status === 'success' && <div className="full-search-tools">
      <SelectMenu label={productMessage('search.control-library').text ?? ''} value={selectedLibrary} options={libraryOptions} onChange={(id) => updateParams({ library: id || undefined })} />
      {selectedSortDescriptor && <SelectMenu label={productMessage('search.control-sort').text ?? ''} value={selectedSortDescriptor.id} options={sortDescriptors.map((sort) => ({ id: sort.id, label: sort.label }))} onChange={changeSort} />}
      {selectedSortDescriptor && selectedSortDescriptor.directions.length > 1 && <SelectMenu label={productMessage('search.control-order').text ?? ''} value={selectedDirection} options={selectedSortDescriptor.directions.map((direction) => ({ id: direction, label: productMessage(direction === 'asc' ? 'search.order-ascending' : 'search.order-descending').text ?? direction }))} onChange={changeDirection} />}
      <div className="search-kind-filters" aria-label={productMessage('search.result-types-label').text}>
        {kindOptions.map((kind) => {
          const active = kind.values.every((value) => selectedKinds.includes(value));
          return <button type="button" key={kind.id} className={active ? 'active' : ''} aria-pressed={active} onClick={() => toggleKinds(kind.values)}>{kind.label}</button>;
        })}
      </div>
      <div className="search-view-switch" aria-label={productMessage('search.result-view-label').text}><IconButton label={productMessage('search.grid-view-label').text ?? ''} className={view === 'grid' ? 'selected' : ''} onClick={() => updateParams({ view: undefined })}><Grid3X3 /></IconButton><IconButton label={productMessage('search.list-view-label').text ?? ''} className={view === 'list' ? 'selected' : ''} onClick={() => updateParams({ view: 'list' })}><List /></IconButton></div>
    </div>}
    {committedQuery && excludedSortGroups.length > 0 && <p className="search-sort-scope">{productMessage('search.scope-hidden', { groups: excludedSortGroups.join(', '), verb: excludedSortGroups.length === 1 ? 'is' : 'are' }).text}</p>}
    {!committedQuery && displayPreferences.rememberSearchHistory && recentSearches.length > 0 && <section className="recent-searches" aria-labelledby="recent-searches-title">
      <header><div><History /><span><h2 id="recent-searches-title">{productMessage('search.recent-title').text}</h2><p>{productMessage('search.recent-body').text}</p></span></div><button type="button" disabled={display?.busy} onClick={() => void clearHistory()}><Trash2 /> {productMessage('action.clear-history').text}</button></header>
      <div>{recentSearches.map((query) => <button type="button" key={query.toLocaleLowerCase()} onClick={() => { setDraft(query); updateParams({ q: query }); }}><span>{query}</span><ArrowRight /></button>)}</div>
      {(display?.status === 'error' || historyError) && <p className="recent-search-error" role="status">{historyFailure.body}</p>}
    </section>}
    {!committedQuery && (!displayPreferences.rememberSearchHistory || recentSearches.length === 0) && <div className="library-state full-search-empty"><Search /><strong>{productMessage('search.start-title').text}</strong><p>{productMessage('search.start-body').text}</p></div>}
    {committedQuery && searchContract.status === 'loading' && <div className="search-contract-reservation" aria-busy="true"><span className="sr-only">{loadingMessage.title}</span></div>}
    {committedQuery && searchContract.status === 'error' && contractFailure && <div className="library-state error" role="alert"><ProductLanguageIcon presentation={contractFailure} /><strong>{contractFailure.title}</strong><p>{contractFailure.body}</p></div>}
    {committedQuery && searchContract.status === 'success' && state.status === 'loading' && settledGroups.length === 0 && <div className="search-groups search-groups-reservation" aria-busy="true">{groupStates.map((groupState) => <SearchGroupRequestStatus key={`${requestKey}:${groupState.descriptor.id}:reserved`} state={groupState} retry={() => retryGroup(groupState.descriptor)} />)}</div>}
    {committedQuery && searchContract.status === 'success' && state.status === 'success' && total === 0 && failedGroups.length === 0 && <div className="library-state"><ProductLanguageIcon presentation={filtered ? filteredEmptyMessage : emptyMessage} /><strong>{filtered ? filteredEmptyMessage.title : emptyMessage.title}</strong><p>{filtered ? filteredEmptyMessage.body : emptyMessage.body}</p>{filtered && <SecondaryButton onClick={() => updateParams({ types: undefined, library: undefined, sort: undefined, direction: undefined })}>{filteredEmptyMessage.actions[0]?.label}</SecondaryButton>}</div>}
    {searchContract.status === 'success' && (total > 0 || failedGroups.length > 0 || (state.status === 'loading' && settledGroups.length > 0)) && <div className="search-groups">{groupStates.map((groupState) => {
      if (groupState.data && groupState.data.items.length > 0) return <SearchGroupSurface key={`${requestKey}:${groupState.descriptor.id}`} group={groupState.data} request={request} view={view} requestStatus={groupState.status === 'success' ? undefined : groupState.status} requestMessageId={groupState.messageId} onRetryRequest={() => retryGroup(groupState.descriptor)} />;
      if (groupState.status === 'success') return null;
      return <SearchGroupRequestStatus key={`${requestKey}:${groupState.descriptor.id}`} state={groupState} retry={() => retryGroup(groupState.descriptor)} />;
    })}</div>}
  </div>;
}
