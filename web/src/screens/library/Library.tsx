import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {compositePatch} from '@core/presentation/settings-structure.ts';
import {useServerPreferences} from '../../app/server-preferences';
import type {LibraryContentSnapshot} from '@core/library-content.ts';
import type {BrowseSortSelection} from '@core/browse.ts';
import {parseBrowseResult} from '@core/browse.ts';
import {BROWSE_PAGE, decodePredicates, encodePredicates, seeAllPredicates, useBrowseCapabilities, useBrowseCollection, useShuffled, type Predicate} from '../../app/browse';
import {listWindowKey} from '../../app/list-position';
import {BrowseBar} from './BrowseFilters';
import {EngineResults} from './EngineResults';
import {GenreCard, LibraryPlaylists} from './LibraryViews';
import {PersonalSavedService} from '@core/personal-saved.ts';
import {categoryDestination, libraryTab, libraryTabForPivot, libraryTabSort, libraryTabs, viewerScope, type LibraryTab} from '@core/presentation/index.ts';
import {SaveViewDialog} from './SaveView';
import b from './BrowseFilters.module.css';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {viewSequence} from '../../app/container-play';
import type {ContentRoute, ContentSeeAll, ContentView} from '@core/library-content.ts';
import {useContent} from '../../app/content';
import {browseCountLabel, seeAllTarget} from '../../app/content';
import {useLibrariesContext, libraryKindLabel} from '../../app/libraries';
import {useSession} from '../../app/session';
import {useViewerScope} from '../../app/viewer-scope';
import {useLibraryScan} from '../../app/library-scan';
import {usePreferences} from '../../app/preferences';
import {useEmptyLibraryCopy} from '../shared/NoLibraries';
import {Button, Badge, Chip, Inset, Menu, Notice, Page, PageHeader, Row, Segmented, StateView, Tabs, type MenuItem} from '../../ui';
import {useOpenEntry} from '../../app/open';
import {LoadingGrid, LoadingShelves, SectionView, sectionHeading} from '../shared/Sections';
import {useSelectionQueryTarget, type SelectionTarget} from '../../app/selection';
import {useSelectionRefresh} from '../../app/selection';
import {iconFor} from '@core/presentation/index.ts';
import {libraryOpening, rememberedTab} from '@core/library-memory.ts';
import {useLibraryMemory} from '../../app/library-memory';

import type {LibraryView as Tab} from '../../app/router';
import {ErrorNotice, ErrorState, changedError, errorText} from '../../app/errors';
import {isRecommendationRow} from '@core/recommendation-feedback.ts';
import {useNotInterested} from '../../app/not-interested-notice';
import {withoutHidden} from '../../app/not-interested';
import {useI18n, type I18n} from '../../app/i18n';
import {useQuietRefresh} from '../../app/quiet-refresh';

const SORT_NAMES = new Set(['title', 'added', 'year', 'duration', 'last_played', 'rating', 'episode', 'track', 'artist', 'album', 'author', 'release', 'random']);
const sortLabel = (t: I18n['t'], key: string): string => {
  const id = key.replace(/^sort\./, '');
  // A sort this build has no name for shows the server's own key, tidied.
  return SORT_NAMES.has(id) ? t('lib.sortField', {field: id}) : id.charAt(0).toUpperCase() + id.slice(1).replace(/_/g, ' ');
};
/** The tab that reads a pivot in a library of this kind (a row's See all names a pivot). */
export const tabForPivot = (pivot: string, kind: string): Tab | undefined => libraryTabForPivot(kind, pivot)?.id as Tab | undefined;

/**
 * One library. Its tabs are authored in client-core (`library-tabs.ts`): Discover first and the
 * default, then the kind's own. The tab, sort and filters live in the URL so every view is
 * linkable; the server owns the sorts and filters a tab offers.
 */
export function LibraryScreen() {
  const i18n = useI18n();
  const {t} = i18n;
  const emptyCopy = useEmptyLibraryCopy();
  const {libraryId} = useParams({from: '/app/library/$libraryId'});
  const address = useSearch({from: '/app/library/$libraryId'});
  const navigate = useNavigate();
  const {items: libraries} = useLibrariesContext();
  const library = libraries.find(l => l.id === libraryId);
  const {api, session, owner} = useSession();
  // A library reopens the way this profile left it: the tab, and that tab's sort and filters.
  // An address that names any of them (a link, a row's See all) is followed as it is; only a plain
  // visit is answered from memory, and the address is then brought in line so it can be shared.
  const memory = useLibraryMemory();
  const opening = useMemo(() => {
    if (!library) return undefined;
    const named = {tab: address.view, sort: address.sort ? {field: address.sort, direction: address.direction ?? 'asc'} : undefined, filters: address.filters ? decodePredicates(address.filters) : undefined, other: !!(address.category || address.row)};
    const found = libraryOpening(named, memory.recall(libraryId), libraryTabs(library.kind, {owner, unmatched: true}).map(x => x.id));
    return found.remembered ? found : undefined;
  }, [library, libraryId, memory, owner, address.view, address.sort, address.direction, address.filters, address.category, address.row]);
  // A category link ("genre:Drama", "decade:1990", "series:Dune") is the tab that lists those titles with
  // the category as one ordinary filter, so it has every sort and filter the library has. The page
  // shows that at once and the address is brought in line with it.
  const categoryTarget = useMemo(() => (library && address.category && !address.row ? categoryDestination(library.kind, address.category) : undefined), [library, address.category, address.row]);
  const search = useMemo<typeof address>(() => (opening ? {view: opening.tab as typeof address.view, ...(opening.sort ? {sort: opening.sort.field, direction: opening.sort.direction} : {}), ...(opening.filters?.length ? {filters: encodePredicates(opening.filters as readonly Predicate[])} : {})} : categoryTarget ? {view: categoryTarget.tab as typeof address.view, filters: encodePredicates([categoryTarget.predicate as Predicate])} : address), [opening, categoryTarget, address]);
  useEffect(() => {
    if (opening || categoryTarget) void navigate({to: '/library/$libraryId', params: {libraryId}, search, replace: true});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [opening, categoryTarget, libraryId]);
  // Whether the filters on screen were put there from memory rather than by a link or by hand just now.
  const filtersFromMemory = useRef(false);
  if (opening) filtersFromMemory.current = true;
  useEffect(() => { filtersFromMemory.current = !!opening; /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [libraryId]);
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
  const cardSizeItems: MenuItem[] = (['small', 'medium', 'large'] as const).map(id => ({id, label: t(`settings.cardSize.${id}`), selected: id === cardSize}));
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
  // Every tab with a pivot reads the browse engine, groups (genres, series) included: one toolbar, one
  // windowed list however many there are. Playlists are the viewer's own and draw themselves.
  const custom = active.shows === 'playlists';
  const pivot = library && !custom ? active.pivot : undefined;
  const engine = !!pivot && !search.category && !search.row;
  // A tab of groups: each card opens another tab filtered to it.
  const groups = !!active.opens;
  // Grid or list is a choice only on a grid of titles: Discover is rows, Songs is always a table, groups are cards.
  const viewChoice = engine && active.shows === 'titles' && !groups;
  const content = useMemo<ContentRoute>(() => (search.row || custom ? {libraryId, view: 'discover'} : ({libraryId, view: tab as ContentView, ...(search.category ? {category: search.category} : {}), ...(search.sort ? {sort: search.sort, direction: search.direction} : {})})), [libraryId, tab, custom, search.row, search.category, search.sort, search.direction]);
  /* The authored content route still serves Discover, categories and the tab strip; browsable pivots read the engine. */
  const {snapshot, retry, refresh, next} = useContent(engine ? {libraryId, view: 'discover'} : content);
  // Read again quietly when the server says the page changed; the notice is for one that stays refused.
  const changedStuck = useQuietRefresh(!engine && !custom && snapshot.phase === 'refresh-required', refresh);
  const projection = snapshot.projection;
  // PERF-S02: section "More" appends inside its own section; the page's other
  // sections stay rendered.
  // Every page read so far, per section, kept with the document: Back returns to the same list, and quiet updates land in place.
  const appendedSections = snapshot.appended;
  const {capabilities, error: capabilityError} = useBrowseCapabilities(engine ? libraryId : undefined, pivot);
  // A remembered filter the library no longer offers is not sent: a view remembered before a
  // server change opens without it instead of failing every time the library is opened.
  const chosenPredicates = useMemo(() => {
    const asked = decodePredicates(search.filters);
    return capabilities && filtersFromMemory.current ? asked.filter(p => capabilities.fields.some(f => f.id === p.field && f.operators.includes(p.operator))) : asked;
  }, [search.filters, capabilities]);
  // A tab's own conditions (Unmatched) come before the viewer's and are not shown as chips.
  const predicates = useMemo(() => (active.fixedFilter ? [...(active.fixedFilter as readonly Predicate[]), ...chosenPredicates] : chosenPredicates), [active.fixedFilter, chosenPredicates]);
  // The order the tab opens in (Recently aired: the latest episode first) is only where it starts: any
  // sort the server offers can be chosen, and the choice is remembered for the tab.
  const tabSort = useMemo<BrowseSortSelection>(() => libraryTabSort(active, capabilities?.sorts, capabilities?.resolvedPivot?.defaultSort[0]), [active, capabilities]);
  const chosenSort = useMemo<BrowseSortSelection>(() => {
    const chosen = search.sort ? capabilities?.sorts.find(x => x.id === search.sort && !x.requiresGroup) : undefined;
    if (search.sort && chosen) return {field: search.sort, direction: search.direction ?? chosen.defaultDirection};
    return tabSort;
  }, [search.sort, search.direction, capabilities, tabSort]);
  // Random is one shuffle for as long as the app is open (paging and Back read the same one); choosing it again draws another.
  const sort = useShuffled(chosenSort);
  const browse = useBrowseCollection({libraryId, pivot, predicates, sort, enabled: engine && !!capabilities, pageSize: capabilities ? Math.min(BROWSE_PAGE, capabilities.queryLimits.maximumLimit) : undefined});
  const browseTotal = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().total);
  const browseTotalIsLowerBound = useSyncExternalStore(browse.collection?.subscribe ?? noSubscribe, () => browse.collection?.getSnapshot().totalIsLowerBound ?? false);
  useSelectionRefresh(engine ? browse.refresh : refresh);
  // PERF-S09: the engine grid names its whole set (browse query + total) so
  // Select-all with nothing deselected bulk-acts in one fenced job.
  // Registering sends nothing; the revision resolves lazily at submit with one
  // tiny browse request, cached per query (a moved set falls back to items).
  const revisionCache = useRef<{key: string; promise: Promise<string | undefined>} | null>(null);
  const browseTargetKey = engine && pivot && !groups ? `browse:${libraryId}:${pivot}:${search.filters ?? ''}:${JSON.stringify(chosenSort)}` : '';
  const browseTarget = useMemo<SelectionTarget | undefined>(() => {
    if (!browseTargetKey || !pivot || browseTotal === undefined) return undefined;
    const key = browseTargetKey;
    const resolveRevision = () => {
      const hit = revisionCache.current;
      if (hit && hit.key === key) return hit.promise;
      const promise = api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/browse`, 'POST', {pivot, ...(predicates.length ? {query: {all: predicates}} : {}), sort: [chosenSort], limit: 1, range: {start: 0}}).then(
        raw => { try { return parseBrowseResult(raw).pageInfo.revision || undefined; } catch { return undefined; } },
        () => undefined,
      );
      revisionCache.current = {key, promise};
      return promise;
    };
    return {selector: {query: {libraryId, pivot, ...(predicates.length ? {filter: {all: predicates}} : {}), sort: [chosenSort]}}, total: browseTotal, resolveRevision};
  }, [browseTargetKey, pivot, libraryId, predicates, chosenSort, browseTotal, api]);
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
      () => setScanNotice({tone: 'success', message: t('web.library.scanStarted')}),
      (e: unknown) => setScanNotice({tone: 'error', message: errorText(e, 'library', 'load')}),
    ).finally(() => setScanBusy(false));
  };
  const ownerMenu: MenuItem[] = [
    {id: 'scan', label: t('web.library.scan'), icon: 'refresh'},
    {id: 'manage', label: t('web.library.manage'), icon: 'settings'},
  ];
  const tabs = useMemo(() => libraryTabs(kind, tabOptions).map(x => ({id: x.id, label: t(x.label)})), [kind, tabOptions, t]);
  const sorts = projection?.sorts ?? [];
  const setSearch = (patch: Partial<typeof search>) => void navigate({to: '/library/$libraryId', params: {libraryId}, search: {...search, ...patch}, replace: true});
  // A sort chosen from the toolbar or a column header is this tab's from now on.
  const chooseSort = (next: BrowseSortSelection) => { memory.rememberTab(libraryId, tab); memory.rememberSort(libraryId, tab, next); setSearch({sort: next.field, direction: next.direction}); };
  // A group's card opens the tab that lists its titles, filtered to it: the same destination its category link has.
  const openGroup = (value: string) => { const to = active.opens ? categoryDestination(kind, `${active.opens.field}:${value}`) : undefined; if (to) void navigate({to: '/library/$libraryId', params: {libraryId}, search: {view: to.tab as Tab, filters: encodePredicates([to.predicate as Predicate])}}); };
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
  const libraryItems: MenuItem[] = libraries.map(l => ({id: l.id, label: l.name, icon: iconFor(l.kind), meta: sharedKind(l.kind) && l.name.toLowerCase() !== libraryKindLabel(l.kind, t).toLowerCase() ? libraryKindLabel(l.kind, t) : undefined, selected: l.id === libraryId}));
  return (
    <Page>
      <PageHeader
        eyebrow={library ? t('web.libraryKind.library', {kind: library.kind}) : undefined}
        title={library?.name ?? projection?.heading.fallback ?? t('web.library.titleFallback')}
        provisional={!library?.name && !projection?.heading.fallback}
        subtitle={scan.scanning ? <Badge tone="accent" dot live>{t('web.library.scanning', {count: i18n.number(scan.found)})}</Badge> : undefined}
        titleAdornment={libraries.length > 1 ? <Menu label={t('web.library.switchLibrary')} align="start" trigger={<Button variant="ghost" icon="chevronDown" aria-label={t('web.library.switchLibrary')} size="sm" />} items={libraryItems} onSelect={id => void navigate({to: '/library/$libraryId', params: {libraryId: id}, search: {}})} /> : undefined}
        actions={<>{viewChoice ? <Segmented label={t('action.view')} size="sm" options={[{id: 'grid', icon: 'grid', label: t('web.prefs.grid')}, {id: 'list', icon: 'list', label: t('web.prefs.list')}]} value={view} onChange={setView} /> : null}{viewChoice && view === 'grid' ? <Menu label={t('settings.row.cardSize')} trigger={<Button variant="ghost" icon="sliders" aria-label={t('settings.row.cardSize')} title={t('settings.row.cardSize')} size="sm" />} items={cardSizeItems} onSelect={setCardSize} /> : null}{owner ? <Menu label={t('title.moreActions')} trigger={<Button variant="ghost" icon="more" aria-label={t('title.moreActions')} title={t('title.moreActions')} size="sm" />} items={ownerMenu} onSelect={id => { if (id === 'scan') scanLibrary(); else if (id === 'manage') void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: {id: libraryId}}); }} /> : null}</>}
      />
      {scanNotice ? <Inset><Notice tone={scanNotice.tone}>{scanNotice.message}</Notice></Inset> : null}
      <Tabs items={tabs} value={tab} onChange={id => {
        // Choosing a tab is remembered, and the tab comes back with the sort and filters it was last left with.
        memory.rememberTab(libraryId, id);
        const kept = rememberedTab(memory.recall(libraryId), id);
        filtersFromMemory.current = !!kept?.filters?.length;
        setSearch({view: id as Tab, category: undefined, row: undefined, sort: kept?.sort?.field, direction: kept?.sort?.direction, filters: kept?.filters?.length ? encodePredicates(kept.filters as readonly Predicate[]) : undefined});
      }} size="large" label={t('web.library.viewsLabel')} />
      {search.row ? (
        <RowView libraryId={libraryId} rowId={search.row} snapshot={snapshot} changedStuck={changedStuck} retry={retry} refresh={refresh} next={next} onBack={() => setSearch({row: undefined})} list={view === 'list'} square={square} />
      ) : (
      <>
      {engine && capabilities ? (
        <Inset>
          <BrowseBar libraryId={libraryId} libraryKind={library?.kind ?? ''} pivot={pivot} capabilities={capabilities} predicates={chosenPredicates} onChange={next => { filtersFromMemory.current = false; memory.rememberTab(libraryId, tab); memory.rememberFilters(libraryId, tab, next); setSearch({filters: encodePredicates(next)}); }} sort={sort} onSort={chooseSort} defaultSort={tabSort} total={browseTotal} totalIsLowerBound={browseTotalIsLowerBound} onSave={active.fixedFilter || groups ? undefined : () => setSavingView(true)} />
          <SaveViewDialog open={savingView} onClose={() => setSavingView(false)} onSave={async name => { const service = new PersonalSavedService(api, viewerScope(session, api), () => Promise.resolve(crypto.randomUUID())); try { await service.select({view: 'views'}); await service.mutate({action: 'create', kind: 'view', name, definition: {libraryId, pivot: pivot!, query: predicates.length ? {all: predicates} : null, sort: [chosenSort], presentation: view}}); } finally { service.dispose?.(); } }} />
        </Inset>
      ) : null}
      {active.shows === 'playlists' ? <LibraryPlaylists sort={search.sort ? {field: search.sort, direction: search.direction ?? 'asc'} : undefined} onSort={chooseSort} /> : null}
      {engine && capabilityError ? <Inset><Notice tone="error" action={{label: t('action.tryAgain'), onClick: () => window.location.reload()}}>{capabilityError}</Notice></Inset> : null}
      {engine && browse.collection ? <EngineResults sequence={viewSequence({libraryId, pivot, predicates, sort})} unmatched={tab === 'unmatched'} collection={browse.collection} onRefresh={browse.refresh} filtered={chosenPredicates.length > 0} onClearFilters={() => setSearch({filters: undefined})} pivot={pivot} libraryId={libraryId} libraryKind={library?.kind ?? ''} list={(view === 'list' && viewChoice) || active.shows === 'tracks'} tracks={active.shows === 'tracks'} sort={sort} sorts={capabilities?.sorts ?? []} onSort={chooseSort} sortField={sort.field} square={square} scanning={scan.scanning} card={active.shows === 'genres' ? entry => <GenreCard libraryId={libraryId} entry={entry} onOpen={() => openGroup(entry.title)} /> : undefined} empty={active.shows === 'genres' ? <StateView icon="category" title={t('library.genres.empty')} body={t('library.genres.emptyBody')} /> : undefined} listKey={listWindowKey({serverId: scope.serverId, viewerId: scope.viewerId, libraryId, pivot, query: JSON.stringify([search.filters ?? null, sort])})} /> : engine && !capabilityError ? <LoadingGrid density={square ? 'square' : 'poster'} /> : null}
      {!engine && !custom && tab === 'browse' && sorts.length ? (
        <Inset>
          <Row>
            {/* Authored (category and row) browses: the same Sort control as the engine toolbar. */}
            {(() => {
              const activeId = projection?.query.sort ?? sorts[0]?.id ?? 'title';
              const direction = projection?.query.direction === 'desc' ? 'desc' : 'asc';
              const active = sorts.find(srt => srt.id === activeId);
              return <Menu label={t('web.filters.sort')} align="start" trigger={<Chip label={t('lib.sortChip', {field: active ? sortLabel(t, active.labelKey) : '', direction: t('web.filters.direction', {kind: /title|name/.test(activeId ?? '') ? 'alpha' : /added|date|year/.test(activeId ?? '') ? 'date' : 'other', dir: direction})})} icon="sort" />} items={sorts.map(srt => ({id: srt.id, label: sortLabel(t, srt.labelKey), selected: srt.id === activeId, icon: srt.id === activeId ? (direction === 'desc' ? 'chevronDown' as const : 'chevronUp' as const) : undefined}))} onSelect={id => setSearch({sort: id, direction: id === activeId && direction === 'asc' ? 'desc' : 'asc'})} />;
            })()}
            {search.category ? <Chip label={t('web.library.categoryChip', {category: categoryName(search.category)})} pressed icon="close" onClick={() => setSearch({category: undefined})} /> : null}
          </Row>
        </Inset>
      ) : null}
      {!engine && !custom && snapshot.phase === 'loading' && !snapshot.sections.length ? (tab === 'browse' ? <LoadingGrid density={square ? 'square' : 'poster'} /> : <LoadingShelves density={square ? 'square' : 'poster'} />) : null}
      {!engine && !custom && snapshot.phase === 'error' && !snapshot.sections.length ? <ErrorState error={snapshot.error} context="library" retry={retry} refresh={refresh} /> : null}
      {changedStuck ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="library" refresh={refresh} /></Inset> : null}
      {!engine && !custom ? (appendedSections.map(raw => {
        // P5: Discover's recommendation rows offer Not interested and drop a card turned down.
        const recommendation = tab === 'discover' && isRecommendationRow(raw);
        const section = recommendation ? withoutHidden(raw, notInterested.hidden) : raw;
        return <SectionView key={section.id} origin={recommendation ? 'recommendation' : undefined} section={view === 'list' && section.type === 'grid' ? {...section, type: 'list'} : section} libraryId={libraryId} hideTitle={tab === 'browse' && appendedSections.length === 1} loadingMore={snapshot.phase === 'loading'} onMore={section.nextCursor ? () => next(section.id) : undefined} onSeeAll={section.type === 'rail' && (section.totalCount > section.entries.length || section.seeAll) ? () => goSeeAll(section) : undefined} />;
      })) : null}
      {!engine && !custom && snapshot.phase === 'ready' && !snapshot.sections.length ? <StateView icon={iconFor(library?.kind ?? '')} title={scan.scanning ? t('web.library.scanningTitle') : (projection?.empty?.fallback ?? t('library.emptyLibrary'))} body={scan.scanning ? t('web.library.scanningBody') : emptyCopy.body} action={scan.scanning ? undefined : emptyCopy.action} /> : null}
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
function RowView({libraryId, rowId, snapshot, changedStuck, retry, refresh, next, onBack, list, square}: {libraryId: string; rowId: string; snapshot: LibraryContentSnapshot; changedStuck: boolean; retry: () => void; refresh: () => void; next: (sectionId: string) => void; onBack: () => void; list: boolean; square: boolean}) {
  const i18n = useI18n();
  const {t} = i18n;
  const notInterested = useNotInterested();
  const found = snapshot.appended.find(s => s.id === rowId) ?? snapshot.sections.find(s => s.id === rowId);
  const recommendation = !!found && isRecommendationRow(found);
  const section = found && recommendation ? withoutHidden(found, notInterested.hidden) : found;
  if (snapshot.phase === 'loading' && !snapshot.sections.length) {
    return (
      <>
        <PageHeader title=" " onBack={onBack} backLabel={t('action.back')} />
        <LoadingGrid density={square ? 'square' : 'poster'} />
      </>
    );
  }
  if (snapshot.phase === 'error' && !snapshot.sections.length) {
    return (
      <>
        <PageHeader title=" " onBack={onBack} backLabel={t('action.back')} />
        <ErrorState error={snapshot.error} context="library" retry={retry} refresh={refresh} />
      </>
    );
  }
  if (!section && snapshot.phase === 'ready') {
    return <StateView icon="library" title={t('route.notFoundTitle')} body={t('route.notFoundBody')} action={{label: t('action.back'), onClick: onBack}} />;
  }
  if (!section) return null;
  const presented = section.type === 'rail' ? {...section, type: 'grid' as const} : (list && section.type === 'grid' ? {...section, type: 'list' as const} : section);
  return (
    <>
      <PageHeader title={sectionHeading(i18n, section.heading)} onBack={onBack} backLabel={t('action.back')} />
      {changedStuck ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="library" refresh={refresh} /></Inset> : null}
      <SectionView section={presented} libraryId={libraryId} hideTitle origin={recommendation ? 'recommendation' : undefined} onMore={section.nextCursor ? () => next(section.id) : undefined} />
    </>
  );
}

const noSubscribe = () => () => {};
