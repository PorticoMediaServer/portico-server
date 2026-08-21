import {
  availableFields,
  availableSorts,
  countConditions,
  encodeExpression,
  encodeSorts,
  expressionToFilter,
  queryChips,
  removeExpressionAtPath,
  resolveBrowseWorkspaceQuery,
  sortLabel,
  productMessage,
  type BrowseLibraryRequest,
  type LibraryBrowseCapabilities,
  type SavedView,
} from '@porticomediaserver/client-core';
import {
  ArrowDown,
  Check,
  Grid3X3,
  LayoutList,
  RefreshCw,
  Rows3,
  Save,
  Shapes,
  Table2,
  X,
} from '#portico-icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { usePorticoQuery } from '../../data/DataProvider';
import { optionalMotionBehavior } from '../../runtime/motion';
import { markWebTiming, measureWebTiming } from '../../runtime/performance';
import {
  AdvancedButton,
  AdvancedLibraryDialog,
  CapabilityMenu,
  LibraryControlGroup,
  SaveLibraryViewDialog,
  SortCapabilityMenu,
} from './LibraryControls';
import { LibraryResults } from './LibraryResults';
import type {
  BrowseExpression,
  LibraryPivotPage,
  LibraryPresentation,
  LibraryWorkspaceLibrary,
  LibraryWorkspaceSource,
} from './libraryTypes';
import './library.css';

type PageState = {
  status: 'idle' | 'loading' | 'success' | 'error';
  data?: LibraryPivotPage;
  error?: Error;
  key?: string;
};

const alphabet = ['All', '#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];

function mergeUnique<T extends { id: string }>(left: T[], right: T[]) {
  const seen = new Set(left.map((item) => item.id));
  return [...left, ...right.filter((item) => !seen.has(item.id))];
}

function mergePages(current: LibraryPivotPage, next: LibraryPivotPage): LibraryPivotPage {
  const sections = current.sections || next.sections
    ? [...(current.sections ?? [])].map((section) => {
        const incoming = next.sections?.find((candidate) => candidate.id === section.id);
        return incoming ? { ...section, items: mergeUnique(section.items, incoming.items) } : section;
      })
    : undefined;
  next.sections?.forEach((section) => {
    if (!sections?.some((candidate) => candidate.id === section.id)) sections?.push(section);
  });
  return {
    ...next,
    items: mergeUnique(current.items, next.items),
    sections,
    facets: mergeUnique(current.facets ?? [], next.facets ?? []),
    total: next.total ?? current.total,
  };
}

function loadedResultCount(page: LibraryPivotPage) {
  const identities = new Set(page.items.map((item) => `item:${item.id}`));
  page.sections?.forEach((section) => section.items.forEach((item) => identities.add(`item:${item.id}`)));
  page.facets?.forEach((facet) => identities.add(`facet:${facet.id}`));
  return identities.size;
}

function normalizedInitial(title: string) {
  const withoutArticle = title.trim().replace(/^(a|an|the)\s+/i, '');
  return withoutArticle.normalize('NFKD').replaceAll(/[\u0300-\u036f]/g, '').charAt(0).toLocaleUpperCase();
}

function findAlphabetTarget(page: LibraryPivotPage, letter: string) {
  const items = page.sections?.length ? page.sections.flatMap((section) => section.items) : page.items;
  if (letter === 'All') return items[0];
  if (letter === '#') return items.find((item) => !/[A-Z]/.test(normalizedInitial(item.sortTitle || item.title)));
  return items.find((item) => normalizedInitial(item.sortTitle || item.title) >= letter);
}

function viewIcon(view: LibraryPresentation) {
  if (view === 'shelves') return <Rows3 />;
  if (view === 'list') return <LayoutList />;
  if (view === 'table') return <Table2 />;
  if (view === 'facets') return <Shapes />;
  return <Grid3X3 />;
}

function viewLabel(view: LibraryPresentation) {
  const ids = {
    grid: 'library.view-grid',
    'compact-grid': 'library.view-compact-grid',
    shelves: 'library.view-shelves',
    list: 'library.view-list',
    table: 'library.view-table',
    facets: 'library.view-facets',
  } as const;
  return productMessage(ids[view]).text ?? '';
}

function pivotUrl(pivotId: string) {
  const parameters = new URLSearchParams();
  parameters.set('pivot', pivotId);
  return `?${parameters.toString()}`;
}

type LibraryArtworkShape = 'square' | 'poster' | 'landscape';

function libraryArtworkShape(library: LibraryWorkspaceLibrary, pivot?: LibraryBrowseCapabilities['pivots'][number]): LibraryArtworkShape {
  if (library.kind === 'music' || pivot?.entityKinds.every((kind) => ['artist', 'album', 'track'].includes(kind))) return 'square';
  if (pivot?.entityKinds.length && pivot.entityKinds.every((kind) => kind === 'episode')) return 'landscape';
  return 'poster';
}

type QuickFilter = { id: string; label: string; expression?: BrowseExpression };

function quickFilters(fields: ReturnType<typeof availableFields>, pivotKinds: string[]): QuickFilter[] {
  const filters: QuickFilter[] = [{ id: 'all', label: productMessage('library.quick-all').text ?? '' }];
  const field = (id: string) => fields.find((candidate) => candidate.id === id);
  const predicate = (id: string, value: string | boolean): BrowseExpression | undefined => {
    const capability = field(id);
    const operator = capability?.operators.includes('equals')
      ? 'equals'
      : capability?.operators.includes('is') ? 'is' : undefined;
    return operator ? { field: id, operator, value } : undefined;
  };
  const add = (id: string, label: string, expression: BrowseExpression | undefined) => {
    if (expression) filters.push({ id, label, expression });
  };
  const audioOnly = pivotKinds.length > 0 && pivotKinds.every((kind) => ['artist', 'album', 'track'].includes(kind));
  add('unplayed', productMessage(audioOnly ? 'library.quick-unplayed' : 'library.quick-unwatched').text ?? '', predicate('playState', 'unplayed'));
  add('in-progress', productMessage('library.quick-in-progress').text ?? '', predicate('playState', 'in-progress'));
  add('played', productMessage(audioOnly ? 'library.quick-played' : 'library.quick-watched').text ?? '', predicate('playState', 'played'));
  add('favorite', productMessage('library.quick-favorites').text ?? '', predicate('favorite', true));
  add('watchlisted', productMessage('library.quick-watchlist').text ?? '', predicate('watchlisted', true));
  add('available', productMessage('library.quick-available').text ?? '', predicate('availability', 'available'));
  add('missing', productMessage('library.quick-missing').text ?? '', predicate('availability', 'unavailable'));
  return filters;
}

function expressionKey(expression: BrowseExpression | undefined) {
  return expression ? JSON.stringify(expression) : '';
}

export function LibraryWorkspaceFrame({ library }: { library: LibraryWorkspaceLibrary }) {
  const count = productMessage(library.itemCount === 1 ? 'media.item-count-single' : 'media.item-count', { count: library.itemCount }).text;
  const artworkShape = libraryArtworkShape(library);
  return <div className={`standard-page library-workspace-page library-workspace-frame is-${artworkShape}`} aria-busy="true" data-artwork-shape={artworkShape}>
    <header className="library-workspace-header" aria-hidden="true">
      <div><p className="route-context">{productMessage('library.route-context', { libraryName: library.name }).text}</p><h1>{library.name}</h1><p>{count}</p></div>
    </header>
    <div className="library-pivots library-pivots-reserved" aria-hidden="true" />
    <div className="library-workspace-toolbar library-workspace-toolbar-reserved" aria-hidden="true" />
    <div className={`library-results-reservation is-${artworkShape}`} aria-hidden="true" />
  </div>;
}

export function LibraryWorkspacePage({
  library,
  source,
}: {
  library: LibraryWorkspaceLibrary;
  source: LibraryWorkspaceSource;
}) {
  const [parameters, setParameters] = useSearchParams();
  const [page, setPage] = useState<PageState>({ status: 'idle' });
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [saveOpen, setSaveOpen] = useState(false);
  const [savedView, setSavedView] = useState<SavedView>();
  const [reloadRevision, setReloadRevision] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [continuationError, setContinuationError] = useState<{ kind: 'load-more' } | { kind: 'seek'; letter: string }>();
  const [seekingLetter, setSeekingLetter] = useState('');
  const [activeLetter, setActiveLetter] = useState('All');
  const activeSeek = useRef('');
  const requestGeneration = useRef(0);
  const continuationController = useRef<AbortController | undefined>(undefined);
  const seekController = useRef<AbortController | undefined>(undefined);

  useEffect(() => {
    if (!page.data) return;
    markWebTiming('library-useful-content');
    measureWebTiming('runtime-to-library-useful-content', 'first-frame', 'library-useful-content');
  }, [page.data]);

  const capabilityLoad = useCallback(async (_ignored: unknown, signal: AbortSignal) => {
		const data = await source.libraryBrowseCapabilities(library.id, signal);
		if (data.library.id !== library.id) throw new Error('The server returned capabilities for a different library.');
		return data;
	}, [library.id, source]);
	const capabilities = usePorticoQuery<LibraryBrowseCapabilities>(
		`library-capabilities:${library.id}`,
		capabilityLoad,
		['libraries', 'library-items'],
		reloadRevision,
	);

  const requestedPivotId = parameters.get('pivot') ?? '';
  const rawExpression = parameters.get('filters');
  const rawSorts = parameters.get('sort');
  const rawPresentation = parameters.get('view');
  const capabilityData = capabilities.status === 'success' ? capabilities.data : undefined;
  const resolvedQuery = useMemo(() => capabilityData
    ? resolveBrowseWorkspaceQuery({
        pivot: requestedPivotId,
        filters: rawExpression,
        sort: rawSorts,
        view: rawPresentation,
      }, capabilityData)
    : undefined, [capabilityData, rawExpression, rawPresentation, rawSorts, requestedPivotId]);
  const pivot = resolvedQuery?.pivot;
  const expression = resolvedQuery?.expression;
  const expressionInvalid = resolvedQuery?.expressionInvalid ?? false;
  const sorts = resolvedQuery?.sorts ?? [];
  const presentation = resolvedQuery?.presentation ?? 'grid';
  const fields = capabilityData && pivot ? availableFields(capabilityData, pivot) : [];
  const chips = useMemo(() => queryChips(expression, fields), [expression, fields]);
  const queryConditionCount = useMemo(() => expression ? countConditions(expressionToFilter(expression, fields)) : 0, [expression, fields]);
  const sortCapabilities = capabilityData && pivot ? availableSorts(capabilityData, pivot) : [];
  const primarySort = sorts[0];
  const quickFilterOptions = useMemo(() => pivot ? quickFilters(fields, [...pivot.entityKinds]) : [], [fields, pivot]);
  const activeQuickFilter = quickFilterOptions.find((option) => expressionKey(option.expression) === expressionKey(expression));

  const updateParameters = useCallback((updates: Record<string, string | undefined>, replace = false) => {
    const next = new URLSearchParams(parameters);
    Object.entries(updates).forEach(([key, value]) => value ? next.set(key, value) : next.delete(key));
    setParameters(next, { replace });
  }, [parameters, setParameters]);

  useEffect(() => {
    if (!pivot || requestedPivotId === pivot.id) return;
    updateParameters({ pivot: pivot.id }, true);
  }, [pivot, requestedPivotId, updateParameters]);

  const limit = capabilityData
    ? Math.min(capabilityData.queryLimits.maximumLimit, Math.max(capabilityData.queryLimits.defaultLimit, 60))
    : 60;
  const request = useMemo<BrowseLibraryRequest | undefined>(() => pivot ? {
    pivot: pivot.id,
    query: expression,
    sort: sorts,
    presentation: { fields: pivot.presentationFields },
    limit,
  } : undefined, [expression, limit, pivot, sorts]);
  const requestKey = request ? `${library.id}:${JSON.stringify(request)}` : '';

  const fetchPage = useCallback((cursor: string | undefined, signal: AbortSignal, seekPrefix = activeSeek.current) => {
    if (!pivot || !request) return Promise.reject(new Error('This library does not have an active view.'));
    return source.libraryPivot({
      libraryId: library.id,
      libraryKind: library.kind,
      pivot,
      request: { ...request, cursor, ...(seekPrefix ? { seek: { prefix: seekPrefix } } : {}) },
    }, signal);
  }, [library.id, library.kind, pivot, request, source]);
	const initialPageLoad = useCallback((_ignored: unknown, signal: AbortSignal) => fetchPage(undefined, signal, ''), [fetchPage]);
	const initialPage = usePorticoQuery<LibraryPivotPage>(
		`library-pivot:${requestKey}`,
		initialPageLoad,
		['libraries', 'library-items', 'media', 'metadata', 'playback-progress', 'media-state'],
		reloadRevision,
		{ enabled: Boolean(request && pivot && !expressionInvalid) },
	);

  useEffect(() => {
    if (!request || !pivot || expressionInvalid) return;
    requestGeneration.current += 1;
    continuationController.current?.abort();
    seekController.current?.abort();
    setActiveLetter('All');
    activeSeek.current = '';
    setLoadingMore(false);
    setSeekingLetter('');
    setContinuationError(undefined);
    return () => {
      continuationController.current?.abort();
      seekController.current?.abort();
    };
  }, [expressionInvalid, pivot, request, requestKey]);

	useEffect(() => {
		if (!request || !pivot || expressionInvalid) return;
		if (initialPage.status === 'success') {
			setPage((current) => current.key === requestKey && current.data && current.data.nextCursor !== initialPage.data.nextCursor
				? { status: 'success', data: mergePages(initialPage.data, current.data), key: requestKey }
				: { status: 'success', data: initialPage.data, key: requestKey });
			return;
		}
		if (initialPage.status === 'error') {
			setPage((current) => ({ status: 'error', data: current.key === requestKey ? current.data : undefined, error: initialPage.error, key: requestKey }));
			return;
		}
		setPage((current) => ({
			status: 'loading',
			// A refresh of the same logical query keeps its useful result mounted.
			// A different filter, sort, pivot, or presentation reserves geometry
			// instead of briefly showing media from the previous request.
			data: current.key === requestKey ? current.data : undefined,
			key: requestKey,
		}));
	}, [expressionInvalid, initialPage.data, initialPage.error, initialPage.status, pivot, request, requestKey]);

  const appendNextPage = useCallback(async (current: LibraryPivotPage, signal: AbortSignal) => {
    if (!current.hasMore || !current.nextCursor) return current;
    const next = await fetchPage(current.nextCursor, signal);
    return mergePages(current, next);
  }, [fetchPage]);

  const loadMore = async () => {
    if (!page.data?.hasMore || loadingMore) return;
    continuationController.current?.abort();
    const controller = new AbortController();
    continuationController.current = controller;
    const generation = requestGeneration.current;
    const key = requestKey;
    const currentPage = page.data;
    setLoadingMore(true);
    setContinuationError(undefined);
    try {
      const merged = await appendNextPage(currentPage, controller.signal);
      if (controller.signal.aborted || generation !== requestGeneration.current) return;
      setPage((current) => current.key === key && current.data
        ? { status: 'success', data: mergePages(current.data, merged), key }
        : current);
    } catch {
      if (controller.signal.aborted || generation !== requestGeneration.current) return;
      setContinuationError({ kind: 'load-more' });
    } finally {
      if (generation === requestGeneration.current) setLoadingMore(false);
    }
  };

  const scrollToTarget = (id: string) => {
    window.requestAnimationFrame(() => window.requestAnimationFrame(() => {
      document.getElementById(`library-item-${id}`)?.scrollIntoView({ behavior: optionalMotionBehavior(), block: 'center' });
    }));
  };

  const seekAlphabet = async (letter: string) => {
    if (!page.data || seekingLetter) return;
    continuationController.current?.abort();
    seekController.current?.abort();
    requestGeneration.current += 1;
    const generation = requestGeneration.current;
    setActiveLetter(letter);
    setSeekingLetter(letter);
    setLoadingMore(false);
    setContinuationError(undefined);
    const controller = new AbortController();
    seekController.current = controller;
    try {
      const prefix = letter === 'All' ? '' : letter;
      const current = await fetchPage(undefined, controller.signal, prefix);
      if (controller.signal.aborted || generation !== requestGeneration.current) return;
      activeSeek.current = prefix;
      setPage({ status: 'success', data: current, key: requestKey });
      const target = findAlphabetTarget(current, letter);
      if (target) scrollToTarget(target.id);
    } catch {
      if (controller.signal.aborted || generation !== requestGeneration.current) return;
      setContinuationError({ kind: 'seek', letter });
    } finally {
      if (generation === requestGeneration.current) setSeekingLetter('');
    }
  };

  const alphabetical = Boolean(
    pivot?.browseSupported
    && primarySort
    && ['title', 'sortTitle'].includes(primarySort.field)
    && primarySort.direction === 'asc'
    && ['grid', 'compact-grid', 'list', 'table'].includes(presentation),
  );

  const applyQuery = (nextExpression: BrowseExpression | undefined, nextSorts = sorts, nextPivotId?: string) => {
    updateParameters({
      pivot: nextPivotId ?? pivot?.id,
      filters: encodeExpression(nextExpression),
      sort: encodeSorts(nextSorts),
    });
    setSavedView(undefined);
  };

  const applyFacet = (nextExpression: BrowseExpression, targetPivotId?: string) => {
    const browsePivot = capabilityData
      ? capabilityData.pivots.find((candidate) => candidate.id === targetPivotId && candidate.browseSupported)
        ?? capabilityData.pivots.find((candidate) => candidate.browseSupported)
      : undefined;
    if (browsePivot) {
      updateParameters({
        pivot: browsePivot.id,
        filters: encodeExpression(nextExpression),
        sort: undefined,
        view: undefined,
      });
    }
  };

  const loadingMessage = productMessage('library.loading');
  const emptyMessage = productMessage('library.empty');
  const filteredEmptyMessage = productMessage('library.filtered-empty');
  const invalidFilterMessage = productMessage('library.filter-invalid');
  const loadingMoreMessage = productMessage('state.loading-more');
  const loadMoreLabel = productMessage('action.load-more-group', { group: productMessage('library.results-label').text }).text;

  if (capabilities.status === 'loading' && !capabilities.data) {
    return <LibraryWorkspaceFrame library={library} />;
  }

  if (capabilities.status === 'error' && !capabilities.data) {
    const failure = productLanguageProblem(capabilities.error, 'library.load-failed');
    return <div className="standard-page library-workspace-page"><div className="library-workspace-state error" role="alert"><ProductLanguageIcon presentation={failure} /><strong>{failure.title}</strong><p>{failure.body}</p><SecondaryButton onClick={() => setReloadRevision((current) => current + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton></div></div>;
  }

  if (!pivot || !capabilityData) {
    const noPivotsMessage = productMessage('library.no-pivots', { libraryName: library.name });
    return <div className="standard-page library-workspace-page"><div className="library-workspace-state"><ProductLanguageIcon presentation={noPivotsMessage} /><strong>{noPivotsMessage.title}</strong><p>{noPivotsMessage.body}</p></div></div>;
  }

  const loadedCount = page.data ? loadedResultCount(page.data) : 0;
  const resultCount = page.data?.total ?? loadedCount;
  const pageFailure = page.status === 'error' ? productLanguageProblem(page.error, 'library.load-failed') : undefined;
  const continuationFailure = productMessage('problem.request-failed');
  const resultCountLabel = page.data
    ? productMessage(page.data.total == null
      ? resultCount === 1 ? 'library.result-loaded-single' : 'library.results-loaded'
      : resultCount === 1 ? 'library.result-count-single' : 'library.results-count', { count: resultCount }).text
    : productMessage(library.itemCount === 1 ? 'media.item-count-single' : 'media.item-count', { count: library.itemCount }).text;
  const isDiscoverPivot = pivot.id === 'discover';
  const artworkShape = libraryArtworkShape(library, pivot);
  return <div className={`standard-page library-workspace-page ${alphabetical ? 'has-alpha-rail' : ''}`}>
    <header className="library-workspace-header">
      <div><p className="route-context">{productMessage('library.route-context', { libraryName: library.name }).text}</p><h1>{library.name}</h1><p>{resultCountLabel}</p></div>
      {capabilityData.actions.includes('manageLibrary') && <Link className="button secondary" to={`/settings/media?library=${encodeURIComponent(library.id)}`}>{productMessage('action.library-settings').text}</Link>}
    </header>

    <nav className="library-pivots" aria-label={productMessage('library.views-label', { libraryName: library.name }).text}>
      {capabilityData.pivots.map((candidate) => <Link key={candidate.id} className={candidate.id === pivot.id ? 'active' : ''} aria-current={candidate.id === pivot.id ? 'page' : undefined} to={pivotUrl(candidate.id)}>{candidate.label}</Link>)}
    </nav>

    {!isDiscoverPivot && <div className="library-workspace-toolbar">
      {pivot.browseSupported && primarySort && <>
        <CapabilityMenu
          label={productMessage('library.filter-label').text ?? ''}
          value={activeQuickFilter?.id ?? 'custom'}
          options={activeQuickFilter ? quickFilterOptions : [{ id: 'custom', label: productMessage('library.custom-filters').text ?? '' }, ...quickFilterOptions]}
          onChange={(id) => {
            if (id === 'custom') return;
            applyQuery(quickFilterOptions.find((option) => option.id === id)?.expression);
          }}
        />
        <SortCapabilityMenu
          value={primarySort}
          options={sortCapabilities}
          onChange={(nextSort) => updateParameters({ sort: encodeSorts([nextSort, ...sorts.filter((sort) => sort.field !== nextSort.field)]) })}
        />
        <AdvancedButton count={queryConditionCount} onClick={() => setAdvancedOpen(true)} />
        <SecondaryButton onClick={() => setSaveOpen(true)}><Save /> {productMessage('action.save-view').text}</SecondaryButton>
      </>}
      <span className="library-toolbar-spacer" />
      <LibraryControlGroup>
        {(pivot.supportedViews as LibraryPresentation[]).map((view) => <IconButton
          key={view}
          label={productMessage('library.view-label', { view: viewLabel(view) }).text ?? ''}
          className={presentation === view ? 'selected' : ''}
          onClick={() => updateParameters({ view })}
        >{viewIcon(view)}</IconButton>)}
      </LibraryControlGroup>
    </div>}

    {chips.length > 0 && <div className="library-query-chips" aria-label={productMessage('library.applied-filters-label').text}>
      {chips.map((chip) => <button key={chip.key} type="button" onClick={() => expression && applyQuery(removeExpressionAtPath(expression, chip.path))}>{chip.label}<X /></button>)}
      <button type="button" className="clear-all" onClick={() => applyQuery(undefined)}>{productMessage('action.clear-filters').text}</button>
    </div>}

    {!isDiscoverPivot && <div className="library-results-summary">
      <span>{resultCountLabel}</span>
      {page.data && pivot.browseSupported && primarySort && <span>{sortLabel(primarySort, sortCapabilities)} · {productMessage(primarySort.direction === 'asc' ? 'search.order-ascending' : 'search.order-descending').text}</span>}
    </div>}

    {expressionInvalid && <div className="library-inline-error" role="alert"><ProductLanguageIcon presentation={invalidFilterMessage} /><span><strong>{invalidFilterMessage.title}</strong><p>{invalidFilterMessage.body}</p></span><SecondaryButton onClick={() => updateParameters({ filters: undefined })}>{invalidFilterMessage.actions[0]?.label}</SecondaryButton></div>}
    {page.status === 'loading' && !page.data && !expressionInvalid && <div className={`library-results-reservation is-${artworkShape}`} data-artwork-shape={artworkShape} aria-busy="true"><span className="sr-only">{loadingMessage.title}. {loadingMessage.body}</span></div>}
    {pageFailure && <div className="library-inline-error" role="alert"><ProductLanguageIcon presentation={pageFailure} /><span><strong>{pageFailure.title}</strong><p>{pageFailure.body}</p></span><SecondaryButton onClick={() => setReloadRevision((current) => current + 1)}><RefreshCw /> {pageFailure.actions[0]?.label}</SecondaryButton></div>}
    {continuationError && page.data && <div className="library-inline-error" role="alert"><ProductLanguageIcon presentation={continuationFailure} /><span><strong>{continuationFailure.title}</strong><p>{continuationFailure.body}</p></span><SecondaryButton onClick={() => void (continuationError.kind === 'seek' ? seekAlphabet(continuationError.letter) : loadMore())}><RefreshCw /> {continuationFailure.actions[0]?.label}</SecondaryButton></div>}
    {!expressionInvalid && page.data && loadedCount === 0 && <div className="library-workspace-state"><ProductLanguageIcon presentation={expression ? filteredEmptyMessage : emptyMessage} /><strong>{expression ? filteredEmptyMessage.title : emptyMessage.title}</strong><p>{expression ? filteredEmptyMessage.body : emptyMessage.body}</p>{expression && <SecondaryButton onClick={() => applyQuery(undefined)}>{filteredEmptyMessage.actions[0]?.label}</SecondaryButton>}</div>}
    {!expressionInvalid && page.data && loadedCount > 0 && <LibraryResults
      library={library}
      pivot={pivot}
      page={page.data}
      presentation={presentation}
      onApplyFacet={applyFacet}
      onChanged={() => setReloadRevision((current) => current + 1)}
    />}
    {!expressionInvalid && page.data?.hasMore && <div className="library-load-more"><PrimaryButton disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? <RefreshCw className="state-spinner" /> : <ArrowDown />} {loadingMore ? loadingMoreMessage.title : loadMoreLabel}</PrimaryButton></div>}

    {!expressionInvalid && alphabetical && page.data && loadedCount > 0 && <nav className="library-alpha-rail" aria-label={productMessage('library.alpha-label').text}>
      {alphabet.map((letter) => <button
        key={letter}
        type="button"
        className={activeLetter === letter ? 'active' : ''}
        disabled={Boolean(seekingLetter)}
        aria-label={productMessage(letter === 'All' ? 'library.jump-beginning' : 'library.jump-letter', { letter }).text}
        onClick={() => void seekAlphabet(letter)}
      >{seekingLetter === letter ? '·' : letter}</button>)}
    </nav>}

    {advancedOpen && <AdvancedLibraryDialog
      capabilities={capabilityData}
      library={library}
      source={source}
      pivot={pivot}
      expression={expression}
      sorts={sorts}
      onApply={applyQuery}
      onDismiss={() => setAdvancedOpen(false)}
    />}
    {saveOpen && <SaveLibraryViewDialog
      library={library}
      source={source}
      pivot={pivot}
      expression={expression}
      sorts={sorts}
      presentationFields={pivot.presentationFields}
      onSaved={setSavedView}
      onDismiss={() => setSaveOpen(false)}
    />}
    {savedView && <div className="library-saved-notice" role="status"><Check /> <span>{productMessage('library.saved-view-notice', { title: savedView.title }).text}</span><IconButton label={productMessage('library.dismiss-saved-view').text ?? ''} onClick={() => setSavedView(undefined)}><X /></IconButton></div>}
  </div>;
}
