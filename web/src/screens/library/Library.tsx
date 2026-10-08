import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {compositePatch} from '@core/presentation/settings-structure.ts';
import {useServerPreferences} from '../../app/server-preferences';
import type {ContentEntry, ContentSection, LibraryContentSnapshot} from '@core/library-content.ts';
import type {WindowedCollection} from '@core/collections/index.ts';
import type {BrowseSortCapability, BrowseSortSelection} from '@core/browse.ts';
import {parseBrowseResult} from '@core/browse.ts';
import {BROWSE_PAGE, decodePredicates, encodePredicates, seeAllPredicates, useBrowseCapabilities, useBrowseCollection, type Predicate} from '../../app/browse';
import {listWindowKey, recallListWindow, saveListWindow} from '../../app/list-position';
import {AlphabetRail, BrowseBar} from './BrowseFilters';
import {GenresGrid, LibraryPlaylists, TitlesTable} from './LibraryViews';
import {PersonalSavedService} from '@core/personal-saved.ts';
import {libraryTab, libraryTabForPivot, libraryTabs, viewerScope, type LibraryTab} from '@core/presentation/index.ts';
import {SaveViewDialog} from './SaveView';
import b from './BrowseFilters.module.css';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {ContentRoute, ContentSeeAll, ContentView} from '@core/library-content.ts';
import {useContent} from '../../app/content';
import {currentI18n} from '../../app/i18n';
import {browseCountLabel, libraryKindSingular, seeAllTarget} from '../../app/content';
import {useLibrariesContext, libraryKindLabel} from '../../app/libraries';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {useLibraryScan} from '../../app/library-scan';
import {usePreferences} from '../../app/preferences';
import {useEmptyLibraryCopy} from '../shared/NoLibraries';
import {Button, Badge, Chip, Chips, Inset, Menu, Notice, Page, PageHeader, Row, Section, Segmented, StateView, Tabs, WindowedGrid, createStaticCollection, gridColumnMin, type MenuItem, type WindowedGridHandle} from '../../ui';
import {Artwork, Grid, Text} from '../../ui';
import {useOpenEntry} from '../../app/open';
import {EntryCard, EntryListRow, LoadingGrid, LoadingShelves, SectionView, sectionHeading, useAppendedSections} from '../shared/Sections';
import {Skeleton} from '../../ui';
import {useSelectionActions, useSelectionActive, useSelectionSection} from '../../app/selection';
import {useSelectionQueryTarget, type SelectionTarget} from '../../app/selection';
import {useSelectionRefresh} from '../../app/selection';
import {iconFor} from '@core/presentation/index.ts';

import type {LibraryView as Tab} from '../../app/router';
import {ErrorNotice, ErrorState, changedError, errorText} from '../../app/errors';
import {isRecommendationRow} from '@core/recommendation-feedback.ts';
import {useNotInterested} from '../../app/not-interested-notice';
import {withoutHidden} from '../../app/not-interested';

const sortLabel = (key: string): string => {
  const id = key.replace(/^sort\./, '');
  const names: Record<string, string> = {title: 'Title', added: 'Date added', year: 'Year', duration: 'Duration', last_played: 'Last played', rating: 'Rating', episode: 'Episode', track: 'Track', artist: 'Artist', album: 'Album', author: 'Author', release: 'Release date', random: 'Random'}; // lint-strings-allow: sort-key label map (server sort keys; only Title matches the lint's prop list)
  return names[id] ?? id.charAt(0).toUpperCase() + id.slice(1).replace(/_/g, ' ');
};
/** The tab that reads a pivot in a library of this kind (a row's See all names a pivot). */
export const tabForPivot = (pivot: string, kind: string): Tab | undefined => libraryTabForPivot(kind, pivot)?.id as Tab | undefined;

/**
 * One library. Its tabs are authored in client-core (`library-tabs.ts`): Discover first and the
 * default, then the kind's own. The tab, sort and filters live in the URL so every view is
 * linkable; the server owns the sorts and filters a tab offers.
 */
const fallbackSort: BrowseSortSelection = {field: 'title', direction: 'asc'};

export function LibraryScreen() {
  const emptyCopy = useEmptyLibraryCopy();
  const {libraryId} = useParams({from: '/app/library/$libraryId'});
  const search = useSearch({from: '/app/library/$libraryId'});
  const navigate = useNavigate();
  const {items: libraries} = useLibrariesContext();
  const library = libraries.find(l => l.id === libraryId);
  const {api, session, owner} = useSession();
  const scope = useViewerScope();
  // ONB-04: the header and empty state reflect the running scan, if any.
  const scan = useLibraryScan(api, scope.serverId, libraryId);
  const {preferences, update} = usePreferences();
  const openEntry = useOpenEntry();
  // P5: a Discover recommendation marked Not interested leaves its row at once; Undo puts it back.
  const notInterested = useNotInterested();
  // WEB-LIB-04: each library keeps its own grid or list choice.
  const view = preferences.libraryViews[libraryId] ?? preferences.libraryView;
  const setView = (v: 'grid' | 'list') => update({libraryViews: {...preferences.libraryViews, [libraryId]: v}});
  // Card size is the Appearance setting; the toolbar is a second place to set it, where the cards are.
  const serverPreferences = useServerPreferences();
  const cardSize = preferences.posterSize === 'compact' ? 'small' : preferences.posterSize === 'large' ? 'large' : 'medium';
  const cardSizeItems: MenuItem[] = (['small', 'medium', 'large'] as const).map(id => ({id, label: currentI18n().t(`settings.cardSize.${id}`), selected: id === cardSize}));
  const setCardSize = (id: string) => {
    // At once on this device; the profile's setting follows, and is the one Appearance shows.
    update({posterSize: id === 'small' ? 'compact' : id === 'large' ? 'large' : 'regular'});
    void serverPreferences.set(compositePatch('cardSize', id)).catch(() => {});
  };
  const [savingView, setSavingView] = useState(false);
  // A full-page row view (?row=<id>) always reads the Discover projection.
  const kind = library?.kind ?? '';
  const tabOptions = useMemo(() => ({owner, unmatched: true}), [owner]);
  const active: LibraryTab = libraryTab(kind, search.row ? 'discover' : search.view, tabOptions);
  const tab = active.id as Tab;
  // Genre cards and playlists draw themselves; every other tab with a pivot reads the browse engine.
  const custom = active.shows === 'genres' || active.shows === 'playlists';
  const pivot = library && !custom ? active.pivot : undefined;
  const engine = !!pivot && !search.category && !search.row;
  // Grid or list is a choice only on a browsable grid: Discover is rows, Songs is always a table, Genres and Playlists draw themselves.
  const viewChoice = engine && active.shows !== 'tracks';
  const content = useMemo<ContentRoute>(() => (search.row || custom ? {libraryId, view: 'discover'} : ({libraryId, view: tab as ContentView, ...(search.category ? {category: search.category} : {}), ...(search.sort ? {sort: search.sort, direction: search.direction} : {})})), [libraryId, tab, custom, search.row, search.category, search.sort, search.direction]);
  /* The authored content route still serves Discover, categories and the tab strip; browsable pivots read the engine. */
  const {snapshot, retry, refresh, next} = useContent(engine ? {libraryId, view: 'discover'} : content);
  const projection = snapshot.projection;
  // PERF-S02: section "More" appends inside its own section; the page's other
  // sections stay rendered.
  const appendedSections = useAppendedSections(JSON.stringify(engine ? {libraryId, view: 'discover'} : content), snapshot.pagination.cursor, snapshot.sections);
  const {capabilities, error: capabilityError} = useBrowseCapabilities(engine ? libraryId : undefined, pivot);
  const chosenPredicates = useMemo(() => decodePredicates(search.filters), [search.filters]);
  // A tab's own conditions (Unmatched) come before the viewer's and are not shown as chips.
  const predicates = useMemo(() => (active.fixedFilter ? [...(active.fixedFilter as readonly Predicate[]), ...chosenPredicates] : chosenPredicates), [active.fixedFilter, chosenPredicates]);
  const sort = useMemo<BrowseSortSelection>(() => {
    if (active.fixedSort) return active.fixedSort;
    const chosen = search.sort ? capabilities?.sorts.find(x => x.id === search.sort) : undefined;
    if (search.sort && chosen) return {field: search.sort, direction: search.direction ?? chosen.defaultDirection};
    return capabilities?.resolvedPivot?.defaultSort[0] ?? fallbackSort;
  }, [search.sort, search.direction, capabilities, active.fixedSort]);
  const browse = useBrowseCollection({libraryId, pivot, predicates, sort, enabled: engine && !!capabilities, pageSize: capabilities ? Math.min(BROWSE_PAGE, capabilities.queryLimits.maximumLimit) : undefined});
  const browseTotal = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().total);
  useSelectionRefresh(engine ? browse.refresh : refresh);
  // PERF-S09: the engine grid names its whole set (browse query + total) so
  // Select-all with nothing deselected bulk-acts in one fenced job.
  // Registering sends nothing; the revision resolves lazily at submit with one
  // tiny browse request, cached per query (a moved set falls back to items).
  const revisionCache = useRef<{key: string; promise: Promise<string | undefined>} | null>(null);
  const browseTargetKey = engine && pivot ? `browse:${libraryId}:${pivot}:${search.filters ?? ''}:${JSON.stringify(sort)}` : '';
  const browseTarget = useMemo<SelectionTarget | undefined>(() => {
    if (!browseTargetKey || !pivot || browseTotal === undefined) return undefined;
    const key = browseTargetKey;
    const resolveRevision = () => {
      const hit = revisionCache.current;
      if (hit && hit.key === key) return hit.promise;
      const promise = api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/browse`, 'POST', {pivot, ...(predicates.length ? {query: {all: predicates}} : {}), sort: [sort], limit: 1, range: {start: 0}}).then(
        raw => { try { return parseBrowseResult(raw).pageInfo.revision || undefined; } catch { return undefined; } },
        () => undefined,
      );
      revisionCache.current = {key, promise};
      return promise;
    };
    return {selector: {query: {libraryId, pivot, ...(predicates.length ? {filter: {all: predicates}} : {}), sort: [sort]}}, total: browseTotal, resolveRevision};
  }, [browseTargetKey, pivot, libraryId, predicates, sort, browseTotal, api]);
  useSelectionQueryTarget(browseTargetKey, browseTarget);
  // MU2: when a scan finishes, refresh once so the titles that arrived appear.
  const finishedSeen = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (finishedSeen.current === undefined) {
      finishedSeen.current = scan.finished;
      return;
    }
    if (scan.finished === finishedSeen.current) return;
    finishedSeen.current = scan.finished;
    if (engine) browse.refresh();
    else refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scan.finished]);
  const [switcher, setSwitcher] = useState(false);
  useEffect(() => setSwitcher(false), [libraryId]);
  // WEB-LIB-07: owner library actions live in a ⋯ menu (scan calls what
  // Server › Libraries uses: POST /v1/libraries/{id}/scans). No consumer
  // refresh button; no Shuffle — `query` selectors answer `unsupported_selector`
  // in the first Playback v1 release (see Questions), so no dead button.
  const [scanBusy, setScanBusy] = useState(false);
  const [scanNotice, setScanNotice] = useState<{tone: 'success' | 'error'; message: string} | null>(null);
  useEffect(() => setScanNotice(null), [libraryId]);
  const scanLibrary = () => {
    if (scanBusy) return;
    setScanBusy(true);
    setScanNotice(null);
    void api.request(`/v1/libraries/${encodeURIComponent(libraryId)}/scans`, 'POST').then(
      () => setScanNotice({tone: 'success', message: currentI18n().t('web.library.scanStarted')}),
      (e: unknown) => setScanNotice({tone: 'error', message: errorText(e, 'library', 'load')}),
    ).finally(() => setScanBusy(false));
  };
  const ownerMenu: MenuItem[] = [
    {id: 'scan', label: currentI18n().t('web.library.scan'), icon: 'refresh'},
    {id: 'manage', label: currentI18n().t('web.library.manage'), icon: 'settings'},
  ];
  const tabs = useMemo(() => libraryTabs(kind, tabOptions).map(x => ({id: x.id, label: currentI18n().t(x.label)})), [kind, tabOptions]);
  const sorts = projection?.sorts ?? [];
  const setSearch = (patch: Partial<typeof search>) => void navigate({to: '/library/$libraryId', params: {libraryId}, search: {...search, ...patch}, replace: true});
  // WEB-LIB-05: See all lands on a clean, linkable destination (no stale sort/filter/row).
  const goSeeAll = (section: {id: string; heading: {key: string}; seeAll?: ContentSeeAll}) => {
    // P6: a personal section names its own browse (pivot, conditions, For you); a query the
    // filter bar can't hold keeps the row's own full page.
    const own = section.seeAll;
    const tab = own ? tabForPivot(own.pivot, library?.kind ?? '') : undefined;
    const conditions = own ? seeAllPredicates(own.query) : undefined;
    if (own && tab && conditions) {
      const first = own.sort[0];
      void navigate({to: '/library/$libraryId', params: {libraryId}, search: {view: tab, ...(first ? {sort: first.field, direction: first.direction} : {}), ...(conditions.length ? {filters: encodePredicates(conditions)} : {})}});
      return;
    }
    const target = seeAllTarget({id: section.id, headingKey: section.heading.key});
    void navigate({to: '/library/$libraryId', params: {libraryId}, search: target.view === 'browse' ? {view: 'browse', ...(target.sort ? {sort: target.sort, direction: target.direction} : {})} : {view: 'discover', row: target.row}});
  };
  const square = library?.kind === 'music' || library?.kind === 'audiobook';
  // CON-05: the kind only when it says something the name doesn't (two libraries share a kind).
  const sharedKind = (kind: string) => libraries.filter(x => x.kind === kind).length > 1;
  const libraryItems: MenuItem[] = libraries.map(l => ({id: l.id, label: l.name, icon: iconFor(l.kind), meta: sharedKind(l.kind) && l.name.toLowerCase() !== libraryKindLabel(l.kind).toLowerCase() ? libraryKindLabel(l.kind) : undefined, selected: l.id === libraryId}));
  return (
    <Page>
      <PageHeader
        eyebrow={library ? currentI18n().t('web.library.kind', {kind: libraryKindSingular(library.kind)}) : undefined}
        title={library?.name ?? projection?.heading.fallback ?? currentI18n().t('web.library.titleFallback')}
        subtitle={scan.scanning ? <Badge tone="accent" dot live>{currentI18n().t('web.library.scanning', {count: currentI18n().number(scan.found)})}</Badge> : undefined}
        titleAdornment={libraries.length > 1 ? <Menu label={currentI18n().t('web.library.switchLibrary')} align="start" trigger={<Button variant="ghost" icon="chevronDown" aria-label={currentI18n().t('web.library.switchLibrary')} size="sm" />} items={libraryItems} onSelect={id => void navigate({to: '/library/$libraryId', params: {libraryId: id}, search: {}})} /> : undefined}
        actions={<>{viewChoice ? <Segmented label={currentI18n().t('action.view')} size="sm" options={[{id: 'grid', icon: 'grid', label: currentI18n().t('web.prefs.grid')}, {id: 'list', icon: 'list', label: currentI18n().t('web.prefs.list')}]} value={view} onChange={setView} /> : null}{viewChoice && view === 'grid' ? <Menu label={currentI18n().t('settings.row.cardSize')} trigger={<Button variant="ghost" icon="sliders" aria-label={currentI18n().t('settings.row.cardSize')} title={currentI18n().t('settings.row.cardSize')} size="sm" />} items={cardSizeItems} onSelect={setCardSize} /> : null}{owner ? <Menu label={currentI18n().t('title.moreActions')} trigger={<Button variant="ghost" icon="more" aria-label={currentI18n().t('title.moreActions')} title={currentI18n().t('title.moreActions')} size="sm" />} items={ownerMenu} onSelect={id => { if (id === 'scan') scanLibrary(); else if (id === 'manage') void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: {id: libraryId}}); }} /> : null}</>}
      />
      {scanNotice ? <Inset><Notice tone={scanNotice.tone}>{scanNotice.message}</Notice></Inset> : null}
      <Tabs items={tabs} value={tab} onChange={id => setSearch({view: id as Tab, category: undefined, sort: undefined, direction: undefined, row: undefined})} size="large" label={currentI18n().t('web.library.viewsLabel')} />
      {search.row ? (
        <RowView libraryId={libraryId} rowId={search.row} snapshot={snapshot} retry={retry} refresh={refresh} next={next} onBack={() => setSearch({row: undefined})} list={view === 'list'} square={square} />
      ) : (
      <>
      {engine && capabilities ? (
        <Inset>
          <BrowseBar libraryId={libraryId} libraryKind={library?.kind ?? ''} pivot={pivot} capabilities={capabilities} predicates={chosenPredicates} onChange={next => setSearch({filters: encodePredicates(next)})} sort={sort} onSort={next => setSearch({sort: next.field, direction: next.direction})} fixedSort={!!active.fixedSort} total={browseTotal} onSave={active.fixedSort || active.fixedFilter ? undefined : () => setSavingView(true)} />
          <SaveViewDialog open={savingView} onClose={() => setSavingView(false)} onSave={async name => { const service = new PersonalSavedService(api, viewerScope(session, api), () => Promise.resolve(crypto.randomUUID())); try { await service.select({view: 'views'}); await service.mutate({action: 'create', kind: 'view', name, definition: {libraryId, pivot: pivot!, query: predicates.length ? {all: predicates} : null, sort: [sort], presentation: view}}); } finally { service.dispose?.(); } }} />
        </Inset>
      ) : null}
      {active.shows === 'genres' && active.pivot && active.opens ? <GenresGrid libraryId={libraryId} pivot={active.pivot} onOpen={genre => void navigate({to: '/library/$libraryId', params: {libraryId}, search: {view: active.opens!.tab as Tab, filters: encodePredicates([{field: active.opens!.field, operator: 'in', value: [genre]}])}})} /> : null}
      {active.shows === 'playlists' ? <LibraryPlaylists /> : null}
      {engine && capabilityError ? <Inset><Notice tone="error" action={{label: currentI18n().t('action.tryAgain'), onClick: () => window.location.reload()}}>{capabilityError}</Notice></Inset> : null}
      {engine && browse.collection ? <EngineResults unmatched={tab === 'unmatched'} collection={browse.collection} onRefresh={browse.refresh} filtered={chosenPredicates.length > 0} onClearFilters={() => setSearch({filters: undefined})} pivot={pivot} libraryId={libraryId} libraryKind={library?.kind ?? ''} list={view === 'list' || active.shows === 'tracks'} tracks={active.shows === 'tracks'} sort={sort} sorts={capabilities?.sorts ?? []} onSort={active.fixedSort ? undefined : next => setSearch({sort: next.field, direction: next.direction})} sortField={sort.field} square={square} scanning={scan.scanning} listKey={listWindowKey({serverId: scope.serverId, viewerId: scope.viewerId, libraryId, pivot, query: JSON.stringify([search.filters ?? null, sort])})} /> : engine && !capabilityError ? <LoadingGrid density={square ? 'square' : 'poster'} /> : null}
      {!engine && !custom && tab === 'browse' && sorts.length ? (
        <Inset>
          <Row>
            {/* Authored (category and row) browses: the same Sort control as the engine toolbar. */}
            {(() => {
              const activeId = projection?.query.sort ?? sorts[0]?.id ?? 'title';
              const direction = projection?.query.direction === 'desc' ? 'desc' : 'asc';
              const active = sorts.find(srt => srt.id === activeId);
              return <Menu label={currentI18n().t('web.filters.sort')} align="start" trigger={<Chip label={`${active ? sortLabel(active.labelKey) : ''} · ${currentI18n().t('web.filters.direction', {kind: /title|name/.test(activeId ?? '') ? 'alpha' : /added|date|year/.test(activeId ?? '') ? 'date' : 'other', dir: direction})}`} icon="sort" />} items={sorts.map(srt => ({id: srt.id, label: sortLabel(srt.labelKey), selected: srt.id === activeId, icon: srt.id === activeId ? (direction === 'desc' ? 'chevronDown' as const : 'chevronUp' as const) : undefined}))} onSelect={id => setSearch({sort: id, direction: id === activeId && direction === 'asc' ? 'desc' : 'asc'})} />;
            })()}
            {search.category ? <Chip label={currentI18n().t('web.library.categoryChip', {category: categoryName(search.category)})} pressed icon="close" onClick={() => setSearch({category: undefined})} /> : null}
          </Row>
        </Inset>
      ) : null}
      {!engine && !custom && snapshot.phase === 'loading' && !snapshot.sections.length ? (tab === 'browse' ? <LoadingGrid density={square ? 'square' : 'poster'} /> : <LoadingShelves density={square ? 'square' : 'poster'} />) : null}
      {!engine && !custom && snapshot.phase === 'error' && !snapshot.sections.length ? <ErrorState error={snapshot.error} context="library" retry={retry} refresh={refresh} /> : null}
      {!engine && !custom && snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="library" refresh={refresh} /></Inset> : null}
      {!engine && !custom ? (appendedSections.map(raw => {
        // P5: Discover's recommendation rows offer Not interested and drop a card turned down.
        const recommendation = tab === 'discover' && isRecommendationRow(raw);
        const section = recommendation ? withoutHidden(raw, notInterested.hidden) : raw;
        return <SectionView key={section.id} origin={recommendation ? 'recommendation' : undefined} section={view === 'list' && section.type === 'grid' ? {...section, type: 'list'} : section} libraryId={libraryId} hideTitle={tab === 'browse' && appendedSections.length === 1} loadingMore={snapshot.phase === 'loading'} onMore={section.nextCursor ? () => next(section.id) : undefined} onSeeAll={section.type === 'rail' && (section.totalCount > section.entries.length || section.seeAll) ? () => goSeeAll(section) : undefined} />;
      })) : null}
      {!engine && !custom && snapshot.phase === 'ready' && !snapshot.sections.length ? <StateView icon={iconFor(library?.kind ?? '')} title={scan.scanning ? currentI18n().t('web.library.scanningTitle') : (projection?.empty?.fallback ?? currentI18n().t('library.emptyLibrary'))} body={scan.scanning ? currentI18n().t('web.library.scanningBody') : emptyCopy.body} action={scan.scanning ? undefined : emptyCopy.action} /> : null}
      </>
      )}
    </Page>
  );
}

/** A category id ("decade:2020", "genre:Drama", "studio:Pixar") as the chip reads it: "2020s", "Drama", "Pixar". */
function categoryName(id: string): string {
  const at = id.indexOf(':');
  if (at < 0) return id;
  const kind = id.slice(0, at), value = id.slice(at + 1);
  return kind === 'decade' && /^\d{4}$/.test(value) ? `${value}s` : value;
}

/**
 * WEB-LIB-05: one Discover row as a full page. It reads the same Discover
 * projection (the row pages through its own cursor, exactly like the inline
 * rail), rendered as a grid with the row's title and a way back.
 */
function RowView({libraryId, rowId, snapshot, retry, refresh, next, onBack, list, square}: {libraryId: string; rowId: string; snapshot: LibraryContentSnapshot; retry: () => void; refresh: () => void; next: (sectionId: string) => void; onBack: () => void; list: boolean; square: boolean}) {
  const notInterested = useNotInterested();
  const found = snapshot.sections.find(s => s.id === rowId);
  const recommendation = !!found && isRecommendationRow(found);
  const section = found && recommendation ? withoutHidden(found, notInterested.hidden) : found;
  if (snapshot.phase === 'loading' && !snapshot.sections.length) {
    return (
      <>
        <PageHeader title=" " onBack={onBack} backLabel={currentI18n().t('action.back')} />
        <LoadingGrid density={square ? 'square' : 'poster'} />
      </>
    );
  }
  if (snapshot.phase === 'error' && !snapshot.sections.length) {
    return (
      <>
        <PageHeader title=" " onBack={onBack} backLabel={currentI18n().t('action.back')} />
        <ErrorState error={snapshot.error} context="library" retry={retry} refresh={refresh} />
      </>
    );
  }
  if (!section && snapshot.phase === 'ready') {
    return <StateView icon="library" title={currentI18n().t('route.notFoundTitle')} body={currentI18n().t('route.notFoundBody')} action={{label: currentI18n().t('action.back'), onClick: onBack}} />;
  }
  if (!section) return null;
  const presented = section.type === 'rail' ? {...section, type: 'grid' as const} : (list && section.type === 'grid' ? {...section, type: 'list' as const} : section);
  return (
    <>
      <PageHeader title={sectionHeading(section.heading)} onBack={onBack} backLabel={currentI18n().t('action.back')} />
      {snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="library" refresh={refresh} /></Inset> : null}
      <SectionView section={presented} libraryId={libraryId} hideTitle origin={recommendation ? 'recommendation' : undefined} onMore={section.nextCursor ? () => next(section.id) : undefined} />
    </>
  );
}

const noSubscribe = () => () => {};
const EMPTY: readonly ContentEntry[] = [];

/**
 * Browse results over a windowed collection: only the rows on screen (plus a
 * little overscan) are in the DOM and in memory, whatever the library's size.
 * The position rail jumps with the server's `positionIndex` (any sort that has
 * one; the rail stays hidden where the server sent no anchors).
 *
 * PERF-S04: Back restores from the position memory (`listKey`) with zero
 * requests before paint — the remembered window renders from memory at its
 * scroll offset while the live collection revalidates behind it.
 */
function EngineResults({collection, onRefresh, libraryId, libraryKind, list, tracks, unmatched, sort, sorts, onSort, sortField, square, filtered, onClearFilters, pivot, listKey, scanning}: {tracks?: boolean; /** The owner's Unmatched tab: empty is good news. */ unmatched?: boolean; sort: BrowseSortSelection; sorts: readonly BrowseSortCapability[]; /** Absent: the tab's order is fixed. */ onSort?: (next: BrowseSortSelection) => void; collection: WindowedCollection<ContentEntry>; onRefresh: () => void; libraryId: string; libraryKind: string; list: boolean; sortField: string; square: boolean; filtered: boolean; onClearFilters?: () => void; pivot?: string; listKey: string; scanning?: boolean}) {
  const emptyCopy = useEmptyLibraryCopy();
  const remembered = useMemo(() => recallListWindow<ContentEntry>(listKey), [listKey]);
  const staticStore = useMemo(() => (remembered ? createStaticCollection<ContentEntry>({keyOf: e => e.id, base: remembered.first}) : null), [remembered]);
  useEffect(() => { staticStore?.setItems(remembered?.items ?? [], remembered?.total); }, [staticStore, remembered]);
  useEffect(() => () => staticStore?.collection.dispose(), [staticStore]);
  const [showLive, setShowLive] = useState(!remembered);
  const active = showLive || !staticStore ? collection : staticStore.collection;
  const snapshot = useSyncExternalStore(active.subscribe, active === collection ? collection.getSnapshot : staticStore!.collection.getSnapshot);
  // The live collection revalidates behind the remembered window; its changes
  // re-render so the swap below fires (the screen subscribes to `active` only).
  const liveSnap = useSyncExternalStore(collection.subscribe, collection.getSnapshot);
  const grid = useRef<WindowedGridHandle>(null);
  const actions = useSelectionActions();
  const selecting = useSelectionActive();
  const selectable = !!actions && !!selecting;
  const [range, setRange] = useState({first: remembered?.first ?? 0, count: 0});
  const rangeRef = useRef(range);
  rangeRef.current = range;
  const [current, setCurrent] = useState<string>();
  // A different list in the same mount starts from its own memory, not the old screen.
  const lastKey = useRef(listKey);
  if (lastKey.current !== listKey) {
    lastKey.current = listKey;
    setShowLive(!remembered);
    setRange({first: remembered?.first ?? 0, count: 0});
    setCurrent(undefined);
  }
  const anchors = useMemo(() => Object.entries(snapshot.positionIndex ?? remembered?.positionIndex ?? {}).map(([key, index]) => ({key, index})).sort((a, b) => a.index - b.index), [snapshot.positionIndex, remembered]);
  const onFirstVisible = useCallback((index: number) => {
    let letter: string | undefined;
    for (const a of anchors) { if (a.index <= index) letter = a.key; else break; }
    setCurrent(letter);
    setRange(prev => (prev.first === index ? prev : {first: index, count: 60}));
    // While memory serves the screen, the live collection pages behind it.
    if (!showLive) collection.ensureRange(Math.max(0, index - 60), index + 120);
  }, [anchors, showLive, collection]);
  // The live collection starts at the remembered position, not the top.
  useEffect(() => {
    if (remembered && !showLive) collection.ensureRange(Math.max(0, remembered.first - 60), remembered.first + 120);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, showLive]);
  // Swap to the live collection once it holds the remembered position.
  useEffect(() => {
    if (showLive || !remembered) return;
    // Also when the live list turned out shorter than the remembered position,
    // or failed: the server's current answer replaces memory either way.
    if (liveSnap.total !== undefined && (collection.itemAt(remembered.first) !== undefined || liveSnap.total <= remembered.first) || liveSnap.error !== undefined) {
      setShowLive(true);
      staticStore?.collection.dispose();
    }
  }, [showLive, remembered, liveSnap, collection, staticStore]);
  // Back lands at the remembered scroll offset (the static grid is sized from
  // the remembered total, so the offset maps to the same rows).
  useLayoutEffect(() => {
    if (remembered) window.scrollTo(0, remembered.scrollY);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // Leaving the list remembers its loaded window and scroll offset.
  useEffect(() => () => {
    const snap = collection.getSnapshot();
    // Remember from the start of the page (60): the restored list then has no
    // partial page above the first visible item.
    const visibleFirst = rangeRef.current.first;
    const aligned = visibleFirst - (visibleFirst % 60);
    const first = collection.itemAt(aligned) !== undefined ? aligned : visibleFirst;
    const items: ContentEntry[] = [];
    for (let i = first; i < first + 180 && i < (snap.total ?? Number.MAX_SAFE_INTEGER); i++) {
      const item = collection.itemAt(i);
      if (!item) break;
      items.push(item);
    }
    saveListWindow(listKey, {first, items, total: snap.total ?? first + items.length, positionIndex: snap.positionIndex, scrollY: window.scrollY});
  }, [collection, listKey]);
  // Select all covers what's on screen (a million-item library can't be held).
  const visible = useMemo(() => {
    const out: ContentEntry[] = [];
    for (let i = range.first; i < range.first + range.count; i++) { const item = active.itemAt(i); if (item) out.push(item); }
    return out.length ? out : EMPTY;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, range, snapshot]);
  useSelectionSection('browse', visible);
  const siblings = useCallback(() => visible, [visible]);
  const density = square ? 'square' : 'poster';
  // Before the first page the grid itself shows placeholders (and asks for that page): never gate it on `total`.
  if (snapshot.status === 'error' && !snapshot.total) return <ErrorState error={snapshot.error} context="library" retry={() => active.retry()} refresh={onRefresh} />;
  // WEB-LIB-02: "no match" only when a filter is doing the excluding.
  if (snapshot.total === 0) return filtered
    ? <StateView icon="filter" title={currentI18n().t('library.noMatch')} body={currentI18n().t('library.noMatchBody')} action={onClearFilters ? {label: currentI18n().t('library.clearFilters'), onClick: onClearFilters} : undefined} />
    : scanning
      ? <StateView icon={iconFor(libraryKind)} title={currentI18n().t('web.library.scanningTitle')} body={currentI18n().t('web.library.scanningBody')} />
      : pivot === 'collections'
        ? <StateView icon="collection" title={currentI18n().t('library.emptyCollections')} body={currentI18n().t('library.emptyCollectionsBody')} />
        : unmatched
          ? <StateView icon="check" title={currentI18n().t('library.emptyUnmatched')} body={currentI18n().t('library.emptyUnmatchedBody')} />
        : pivot === 'series'
          ? <StateView icon="collection" title={currentI18n().t('library.emptySeries')} body={currentI18n().t('library.emptySeriesBody')} />
          : <StateView icon={iconFor(libraryKind)} title={currentI18n().t('library.emptyLibrary')} body={emptyCopy.body} action={emptyCopy.action} />;
  const shape = square ? 'square' : 'poster';
  // M20: the rail follows anchor presence for default browsing (behind
  // presence for older servers), not the sort. An ad hoc query (a filter or
  // search) has no anchors and never shows the rail.
  // For you ranks by taste: it has no letters to jump to.
  // The letter rail is for a title order only: other orders have nothing to jump to by letter.
  const rail = anchors.length >= 4 && !filtered && sortField === 'title';
  return (
    <div className={b.layout} style={{paddingRight: rail ? 0 : undefined}}>
      <div>
        {snapshot.status === 'error' ? <Inset><ErrorNotice error={snapshot.error} context="library" retry={() => active.retry()} refresh={onRefresh} /></Inset> : null}
        {list ? (
          <TitlesTable ref={grid} collection={active} libraryId={libraryId} tracks={tracks} sort={sort} sorts={sorts} onSort={onSort} onFirstVisible={onFirstVisible} />
        ) : (
          <WindowedGrid
            ref={grid}
            collection={active}
            columnMin={gridColumnMin(density)}
            estimateRowHeight={square ? 240 : 300}
            label={currentI18n().t('web.library.titlesLabel')}
            onFirstVisible={onFirstVisible}
            renderItem={entry => <EntryCard entry={entry} siblings={siblings} libraryId={libraryId} selectable={selectable} />}
            renderPlaceholder={() => <div aria-hidden><Skeleton style={{aspectRatio: shape === 'square' ? '1 / 1' : '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>}
          />
        )}
      </div>
      {rail ? <AlphabetRail index={anchors} current={current} sortField={sortField} onJump={key => { const index = active.indexOfLetter(key) ?? anchors.find(a => a.key === key)?.index; if (index !== undefined) grid.current?.scrollToIndex(index); }} /> : null}
    </div>
  );
}
