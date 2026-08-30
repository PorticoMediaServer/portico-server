import { NavigationDisclosureIcon, ActionRefreshIcon, NavigationSearchIcon } from '#portico-icons';
import { orderSearchGroups, productMessage, resolveSearchResultSemantic, type ProductContract } from '@porticomediaserver/client-core';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ProductLanguageIcon } from '../../components/states/ProductLanguageState';
import { usePorticoDataSource, useProductContract } from '../../data/DataProvider';
import type { SearchGroup, SearchPageResult } from '../../data/models';
import { MediaArtwork, mediaDetailPath } from '../catalog/CatalogSurface';
import { mediaPresentation, searchGroupFilterKinds } from '../catalog/mediaPresentation';
import './search.css';

type QuickSearchState =
  | { status: 'idle'; data?: undefined; error?: undefined }
  | { status: 'loading'; data?: SearchPageResult; error?: undefined }
  | { status: 'success'; data: SearchPageResult; error?: undefined }
  | { status: 'error'; data?: SearchPageResult; error: Error };

function orderedGroups(contract: ProductContract, groups: SearchGroup[]) {
  return orderSearchGroups(
    contract,
    groups.filter((group) => group.status === 'error' || group.items.length > 0) as never,
    contract.search.limits.quickMaximumGroups,
  ) as unknown as SearchGroup[];
}

function searchGroupFailure(messageId?: SearchGroup['messageId']) {
  return productMessage(messageId ?? 'search.group-unavailable');
}

export function useQuickSearchGroups(query: string, contract: ProductContract | undefined, reloadKey = 0): QuickSearchState {
  const source = usePorticoDataSource();
  const normalized = query.trim();
  const [state, setState] = useState<QuickSearchState>({ status: normalized ? 'loading' : 'idle' });
  useEffect(() => {
    if (!normalized) {
      setState({ status: 'idle' });
      return;
    }
    if (!contract) {
      setState({ status: 'loading' });
      return;
    }
    const controller = new AbortController();
    setState((current) => ({ status: 'loading', data: current.data?.query === normalized ? current.data : undefined }));
    source.searchPage({ query: normalized, limit: contract.search.limits.quickInitialGroupLimit }, controller.signal).then((data) => {
      if (!controller.signal.aborted) setState({ status: 'success', data: { ...data, groups: orderedGroups(contract, data.groups) } });
    }, (reason: unknown) => {
      if (!controller.signal.aborted) setState((current) => ({ status: 'error', data: current.data?.query === normalized ? current.data : undefined, error: reason instanceof Error ? reason : new Error('Search is unavailable.') }));
    });
    return () => controller.abort();
  }, [contract, normalized, reloadKey, source]);
  return state;
}

function searchOptionId(groupId: string, itemId: string) {
  return `quick-search-option-${encodeURIComponent(groupId)}-${encodeURIComponent(itemId)}`;
}

function QuickSearchGroup({ contract, group, query, activeOptionId, onActiveOptionChange, onSelect, onRetry }: { contract: ProductContract; group: SearchGroup; query: string; activeOptionId?: string; onActiveOptionChange?: (id: string) => void; onSelect: () => void; onRetry: () => void }) {
  const source = usePorticoDataSource();
  const [current, setCurrent] = useState(group);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [errorMessageId, setErrorMessageId] = useState<SearchGroup['messageId']>();
  const generation = useRef(0);
  const continuation = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    generation.current += 1;
    continuation.current?.abort();
    setCurrent(group);
    setLoading(false);
    setError(false);
    setErrorMessageId(undefined);
    return () => continuation.current?.abort();
  }, [group, query]);
  const maximumItems = contract.search.limits.quickMaximumItemsPerGroup;
  const pageLimit = contract.search.limits.quickInitialGroupLimit;
  const groupCapability = contract.search.groups.find((candidate) => candidate.id === group.id);
  const filterKinds = groupCapability ? searchGroupFilterKinds(groupCapability) : [group.entityKind];
  const fullSearch = `/search?q=${encodeURIComponent(query)}&types=${encodeURIComponent(filterKinds.join(','))}`;
  const loadMore = async () => {
    if (!current.nextCursor || current.items.length >= maximumItems) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    setLoading(true);
    setError(false);
    setErrorMessageId(undefined);
    try {
      const response = await source.searchPage({ query, group: current.id, cursor: current.nextCursor, limit: pageLimit }, controller.signal);
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      const next = response.groups.find((candidate) => candidate.id === current.id) ?? response.groups[0];
      if (!next) {
        setCurrent((value) => ({ ...value, hasMore: false, nextCursor: null }));
        return;
      }
      if (next.status === 'error') {
        setErrorMessageId(next.messageId);
        setError(true);
        return;
      }
      setCurrent((value) => {
        const known = new Set(value.items.map((item) => item.id));
        const merged = [...value.items, ...next.items.filter((item) => !known.has(item.id))].slice(0, maximumItems);
        return { ...next, items: merged, hasMore: next.hasMore || merged.length >= maximumItems };
      });
    } catch {
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setError(true);
    } finally {
      if (activeGeneration === generation.current) setLoading(false);
    }
  };

  const failure = searchGroupFailure(errorMessageId);
  const groupFailure = searchGroupFailure(current.messageId);
  const loadMoreLabel = productMessage('action.load-more-group', { group: current.title.toLocaleLowerCase() }).text;
  return <section className="quick-group" aria-labelledby={`quick-group-${current.id}`}>
    <div className="quick-group-heading"><strong id={`quick-group-${current.id}`}>{current.title}</strong><Link to={fullSearch} onClick={onSelect}>{productMessage('action.view-all').text} <NavigationDisclosureIcon /></Link></div>
    <div className={`quick-group-items ${current.entityKind === 'person' ? 'people' : ''}`}>{current.items.map((item) => {
      const artworkRole = resolveSearchResultSemantic(contract, item.entityKind)?.artworkRole;
      const artworkRatio = contract.artworkRoles.find((role) => role.id === artworkRole)?.aspectRatio;
      const square = artworkRatio != null && Math.abs(artworkRatio - 1) < 0.08;
      const optionId = searchOptionId(current.id, item.id);
      return <Link id={optionId} data-search-result data-active={activeOptionId === optionId ? 'true' : undefined} tabIndex={-1} aria-label={productMessage('action.open-item', { title: item.title }).text} to={mediaDetailPath(item) ?? `/media/${item.id}`} key={item.id} onMouseEnter={() => onActiveOptionChange?.(optionId)} onFocus={() => onActiveOptionChange?.(optionId)} onClick={onSelect}>
      <MediaArtwork className={`quick-result-art ${square ? 'square' : ''}`} item={item} />
      <span><strong>{item.title}</strong><small>{item.subtitle || [mediaPresentation(item).label, item.year || undefined].filter(Boolean).join(' · ')}</small></span><NavigationDisclosureIcon />
    </Link>;
    })}</div>
    {current.status === 'error' && <div className="quick-group-error" role="status"><ProductLanguageIcon presentation={groupFailure} /><span>{groupFailure.body}</span><button type="button" onClick={onRetry}>{groupFailure.actions[0]?.label}</button></div>}
    {error && <div className="quick-group-error" role="status"><ProductLanguageIcon presentation={failure} /><span>{failure.body}</span><button type="button" onClick={() => void loadMore()}>{failure.actions[0]?.label}</button></div>}
    {current.status === 'success' && current.hasMore && current.nextCursor && current.items.length < maximumItems && <button className="quick-group-more" type="button" disabled={loading} onClick={() => void loadMore()}>{loading ? <><ActionRefreshIcon className="state-spinner" /> {productMessage('state.loading-more').title}</> : loadMoreLabel}</button>}
  </section>;
}

export function QuickSearchPanel({ query, serverName, activeOptionId, onActiveOptionChange, onSelect, onViewAll }: { query: string; serverName: string; activeOptionId?: string; onActiveOptionChange?: (id: string) => void; onSelect: () => void; onViewAll: () => void }) {
  const [reloadKey, setReloadKey] = useState(0);
  const productContract = useProductContract(reloadKey);
  const state = useQuickSearchGroups(query, productContract.status === 'success' ? productContract.data : undefined, reloadKey);
  const groups = useMemo(() => state.data?.groups ?? [], [state]);
  const loadingMessage = productMessage('search.loading');
  const emptyMessage = productMessage('search.no-results');
  const failure = productMessage('search.offline');
  return <>
    <div className="quick-search-heading"><strong>{productMessage(query.trim() ? 'search.quick-results' : 'search.quick-browse').text}</strong><span>{productMessage('search.quick-full-hint').text}</span></div>
    <div className="quick-search-results grouped">
      {!query.trim() && <div className="quick-search-empty">{productMessage('search.quick-start').text}</div>}
      {query.trim() && (productContract.status === 'loading' || state.status === 'loading') && !state.data && <div className="quick-search-empty"><ProductLanguageIcon presentation={loadingMessage} /> {loadingMessage.title}</div>}
      {(productContract.status === 'error' || state.status === 'error') && <div className="quick-search-empty quick-search-failed"><ProductLanguageIcon presentation={failure} /><span>{failure.body}</span><button type="button" onClick={() => setReloadKey((value) => value + 1)}>{failure.actions[0]?.label}</button></div>}
      {productContract.status === 'success' && state.status === 'success' && groups.length === 0 && <div className="quick-search-empty"><ProductLanguageIcon presentation={emptyMessage} /> {productMessage('search.quick-empty', { serverName }).text}</div>}
      {productContract.status === 'success' && groups.map((group) => <QuickSearchGroup key={`${query.trim()}:${group.id}`} contract={productContract.data} group={group} query={query.trim()} activeOptionId={activeOptionId} onActiveOptionChange={onActiveOptionChange} onSelect={onSelect} onRetry={() => setReloadKey((value) => value + 1)} />)}
    </div>
    <button className="quick-search-all" type="button" onClick={onViewAll}><NavigationSearchIcon /> {productMessage(query.trim() ? 'search.view-all-query' : 'search.view-all', { query: query.trim() }).text}</button>
  </>;
}
