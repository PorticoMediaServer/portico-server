import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useLocation, useParams, useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {Library} from '@core/index.ts';
import {LibraryContentService, type ContentEntry, type ContentRoute, type ContentView} from '@core/library-content.ts';
import {libraryTabs, readingTabModel, type LibraryTab} from '@core/presentation/library-tabs.ts';
import {browseCapabilitiesPath, parseBrowseCapabilities, type BrowseCapabilities} from '@core/browse.ts';
import {metadataAgentPath, parseMetadataAgent} from '@core/server-admin/metadata-agent.ts';
import type {SavedDefinition} from '@core/personal-saved.ts';
import {randomId} from '@core/random-id.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {ShowScreen} from '../detail/Show';
import {AudiobookEntry} from '../detail/audiobooks/Entry';
import {createShowBindings} from '../detail/show-bindings';
import type {ScreenContext} from '../detail/context';
import {screenLifetime} from '../detail/context';
import {BrowseController} from './controller';
import {createLibraryPlaySelection} from './play-selection';
import {createLibrarySelection} from './selection';
import {useMusicMixes} from './music-mixes-hook';
import {LibraryIdentity, LibraryScreen, type LibraryBindings} from './Library';
import {NeedsAttention} from '../metadata/NeedsAttention';
import type {AttentionEntry} from '@core/metadata-attention.ts';
import {LibraryDiscoveryScreen} from './Discovery';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from './screen-context';
import {useScreenAppearance} from './screen-preferences';
import {browseConjunction, parseLibraryScreenSearch, type LibraryScreenSearch} from './route-search';
import {SaveView} from './SaveView';
import {FacetPage, ReadLibraryEntry, authorPredicate, type FacetRequest} from '../read';
import {isReadingLibrary} from './reading-library';

const t = defaultI18n.t;
function ReadingLibrary({captured, library, search}: {captured: ProtectedLibraryScreen; library: Library; search: LibraryScreenSearch}) {
  const context = useLibraryScreenContext(captured), appearance = useScreenAppearance(captured.context)!, width = useShellWidth();
  const life = useMemo(() => screenLifetime(context), [context]);
  const [capabilities, setCapabilities] = useState<BrowseCapabilities>();
  useEffect(() => () => life.dispose(), [life]);
  useEffect(() => {
    let active = true;
    void (async () => {
      const raw = await life.api.request(browseCapabilitiesPath(library.id, library.kind === 'comic' ? 'comics' : 'books'), 'GET');
      if (active && life.current()) setCapabilities(parseBrowseCapabilities(raw));
    })().catch(error => {if (active && life.current()) context.effects.reportError(error);});
    return () => {active = false;};
  }, [context, life, library.id, library.kind]);
  /** Root §3: the server's published pivots decide the reading tabs; older servers keep the authored pair. */
  const tabs = useMemo(() => readingTabModel(capabilities) ?? libraryTabs(library.kind), [capabilities, library.kind]);
  const selected = tabs.find(tab => tab.id === (search.view ?? 'discover')) ?? tabs[0]!;
  const gridPivot = tabs.find(tab => tab.id === 'browse')?.pivot;
  const navigate = (value: LibraryScreenSearch) => {if (context.isCurrent()) void router.navigate({to: '/library/$libraryId', params: {libraryId: library.id}, search: value}).catch(context.effects.reportError);};
  const header = <LibraryIdentity library={library} libraries={context.identity?.libraries ?? []} pivot={selected.id}
    tabs={tabs.map(tab => ({value: tab.id, label: t(tab.label)}))}
    appearance={{view: appearance.libraryView, cardWidth: width < 600 ? 108 : 190, cardSize: 'medium', backdrops: appearance.showBackdrops}}
    preferences={appearance} artwork={context.artwork} libraryMenu={undefined}
    onPivot={id => {const tab = tabs.find(tab => tab.id === id); if (!tab) return;
      if (tab.shows === 'facet') {navigate({view: tab.id === 'authors' ? 'authors' : 'series'}); return;}
      navigate({view: tab.id === 'discover' ? 'discover' : 'browse', ...(tab.pivot ? {pivot: tab.pivot} : {})});}}
    onLibrary={libraryId => {if (context.isCurrent()) void router.navigate({to: '/library/$libraryId', params: {libraryId}, search: {}}).catch(context.effects.reportError);}}
    onLibraries={() => {if (context.isCurrent()) void router.navigate({to: '/libraries'}).catch(context.effects.reportError);}}/>;
  const request = useMemo<FacetRequest>(() => (path, method, body, signal) => life.api.request(path, method, body, signal), [life]);
  if (selected.shows === 'facet') return <>{header}<FacetPage libraryId={library.id} field={selected.facetSource?.field === 'series' ? 'series' : 'author'}
    pivot={gridPivot ?? (library.kind === 'comic' ? 'comics' : 'books')} request={request}
    {...(selected.facetSource?.field === 'author' ? {onSelect: value => navigate({view: 'browse', ...(gridPivot ? {pivot: gridPivot} : {}), query: authorPredicate(value.value)})} : {})}/></>;
  if (search.view === 'browse' || search.query) return <BoundLibrary captured={captured} library={library} search={search} tabs={tabs}/>;
  return <ReadLibraryEntry libraryId={library.id} header={header} view="discover"/>;
}
function LibraryShow({context, libraryId, search}: {context: ScreenContext; libraryId: string; search: LibraryScreenSearch}) {
  const binding = useMemo(() => createShowBindings(context), [context]);
  useEffect(() => () => binding.dispose(), [binding]);
  return <ShowScreen binding={binding} target={{libraryId, ...(search.view === 'season' ? {seasonId: search.entityId} : {showId: search.entityId})}}/>;
}
function BoundLibrary({captured, library, search, tabs: offered}: {captured: ProtectedLibraryScreen; library: Library; search: LibraryScreenSearch; tabs?: readonly LibraryTab[]}) {
  const context = useLibraryScreenContext(captured), appearance = useScreenAppearance(captured.context)!, width = useShellWidth();
  // The offer needs this exact library's own online metadata agent. The fact is keyed to the
  // library and screen scope that produced it, so a stale read can never enable another library
  // or survive a scope change; unknown, reset and failed reads all withhold the tab.
  const [agentFact, setAgentFact] = useState<Readonly<{key: string; context: typeof context; online: boolean}>>();
  const agentKey = JSON.stringify([context.scope.serverId, context.scope.viewerId, library.id, library.kind]);
  // The captured ScreenContext itself is part of the fact: a new capture for the same viewer
  // must not reuse a previous true before its own read, including on early lifetime refusal.
  const onlineAttention = agentFact?.key === agentKey && agentFact.context === context && context.isCurrent() && agentFact.online === true;
  const tabs = offered ?? libraryTabs(library.kind, {owner: context.identity?.capabilities?.manageServer === true, onlineAttention}), requested = search.unmatched ? tabs.find(tab => tab.shows === 'unmatched') : tabs.find(tab => search.pivot ? tab.pivot === search.pivot : tab.id === (search.view ?? library.defaultView ?? 'discover')) ?? tabs[0];
  const view: ContentView = search.view ?? (search.pivot ? 'browse' : library.defaultView ?? 'discover');
  const musicMixes = useMusicMixes(captured, library.kind === 'music' && view === 'discover');
  const pivot = search.pivot ?? requested?.pivot;
  const browse = !!pivot && !search.entityId && !search.category && !search.q && view !== 'discover';
  const sourceKey = JSON.stringify({libraryId: library.id, pivot: pivot ?? '', predicates: search.query ? browseConjunction(search.query) : undefined,
    sorts: search.sort ?? (requested?.defaultSort ? [requested.defaultSort] : undefined)});
  const resources = useMemo(() => {
    const life = screenLifetime(context), source: ConstructorParameters<typeof BrowseController>[1] = JSON.parse(sourceKey);
    const controller = new BrowseController(life.api, source, life.current);
    const service = new LibraryContentService({api: life.api, scope: context.scope});
    return {life, controller, service};
  }, [context, sourceKey]);
  useEffect(() => () => {resources.life.dispose();resources.controller.dispose();resources.service.dispose();}, [resources]);
  const state = useSyncExternalStore(resources.controller.subscribe, resources.controller.getSnapshot, resources.controller.getSnapshot);
  useEffect(() => {
    if (!resources.life.current()) return;
    let live = true;
    const withhold = () => {if (live && resources.life.current()) setAgentFact({key: agentKey, context, online: false});};
    void resources.life.api.request(metadataAgentPath(library.id), 'GET').then(raw => {
      if (!live || !resources.life.current()) return;
      let agent;
      try {agent = parseMetadataAgent(raw);} catch {withhold(); return;}
      // A read that answers for another library or kind is not this library's fact.
      if (agent.libraryId !== library.id || agent.libraryKind !== library.kind) {withhold(); return;}
      setAgentFact({key: agentKey, context, online: agent.agent === 'online'});
    }, withhold).catch(withhold);
    return () => {live = false;};
  }, [resources, library.id, library.kind, agentKey, context]);
  const href = useLocation({select: location => location.href}), runtime = captured.binding.playback?.runtime;
  const runtimeRef = useRef(runtime);runtimeRef.current = runtime;
  const playSelection = useMemo(() => {
    const viewer = captured.context.session().viewer, queryRevision = state.queryRevision;
    if (viewer.authority !== 'local' && viewer.authority !== 'hosted') throw new DOMException('Screen authority unavailable', 'AbortError');
    return createLibraryPlaySelection({source: resources.controller, api: resources.life.api, scope: context.scope,
      principal: {serverId: viewer.serverId, authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId},
      requestId: async () => randomId(), signal: resources.life.signal(),
      isCurrent: () => resources.life.current() && resources.controller.getSnapshot().queryRevision === queryRevision
        && router.state.location.href === href && runtimeRef.current === runtime,
      // The bar's job executor is the same captured route lifetime; these are real
      // executors, and each title's own Detail actions still decide its grants.
      personal: {myList: true, favorite: true, watched: true},
      playback: runtime?.cataloguePlayback()});
  }, [captured.context, context, resources, runtime, href, state.queryRevision]);
  useEffect(() => {playSelection.start();return () => playSelection.dispose();}, [playSelection]);
  // The same captured route lifetime supplies the bar's real job ports. Queue and the
  // saved-list pickers stay absent until their own producers are wired, so the bar
  // cannot draw a control this screen cannot perform.
  const selection = useMemo(() => createLibrarySelection({
    playSelection,
    descriptor: () => resources.controller.wholeSelection(),
    actions: {api: resources.life.api, events: context.events, requestId: async () => randomId(),
      signal: resources.life.signal(), isCurrent: () => resources.life.current(), reportError: context.effects.reportError},
    reportError: context.effects.reportError,
  }), [context, playSelection, resources]);
  const [choice, setChoice] = useState<Readonly<{defaultView: 'grid' | 'list'; view: 'grid' | 'list'}>>();
  const [size, setSize] = useState<'small' | 'medium' | 'large'>('medium'), [saving, setSaving] = useState<SavedDefinition>();
  const navigate = (value: LibraryScreenSearch, id = library.id) => {if (context.isCurrent()) void router.navigate({to: '/library/$libraryId', params: {libraryId: id}, search: value}).catch(context.effects.reportError);};
  const actions = (entry: ContentEntry) => ({onOpen: () => {if (!browse || resources.controller.contains(entry)) context.effects.openEntry(entry);},
    onPlay: entry.playback && entry.libraryId && entry.available !== false && !entry.missing ? () => {
      if (browse && !resources.controller.contains(entry)) return;
      void context.effects.play({libraryId: entry.libraryId!, itemId: entry.playback!.itemId, startSeconds: entry.playback!.startSeconds ?? 0}, resources.life.signal(),
        () => resources.life.current() && (!browse || resources.controller.contains(entry))).catch(context.effects.reportError);
    } : undefined});
  // Frame #grid: seven fluid columns at 1440 on Medium. 170 is the Medium card size the
  // drawn column paints; the screen fills each column to the gutter.
  const appearanceBase = {view: appearance.libraryView, cardWidth: (width < 600 ? 112 : 170) * (size === 'small' ? .8 : size === 'large' ? 1.2 : 1), backdrops: appearance.showBackdrops, cardSize: size};
  const header = {library, libraries: context.identity?.libraries ?? [], pivot: requested?.id ?? 'discover', tabs: tabs.map(tab => ({value: tab.id, label: t(tab.label)})),
    appearance: appearanceBase, preferences: appearance, libraryViewOverride: choice?.defaultView === appearance.libraryView ? choice.view : undefined,
    artwork: context.artwork, libraryMenu: undefined,
    onLibrary: (id: string) => navigate({}, id), onLibraries: () => {void router.navigate({to: '/libraries'}).catch(context.effects.reportError);},
    onPivot: (id: string) => {const tab = tabs.find(tab => tab.id === id);if (!tab) return;
      if (tab.shows === 'unmatched') {navigate({unmatched: true});return;}
      if (tab.shows === 'playlists') {void router.navigate({to: '/saved', search: {view: 'playlists'}}).catch(context.effects.reportError);return;}
      navigate({view: tab.id === 'discover' ? 'discover' : 'browse', ...(tab.pivot ? {pivot: tab.pivot} : {}), ...(tab.defaultSort ? {sort: [tab.defaultSort]} : {})});}};
  const empty = <StateView title={t('library.emptyLibrary')} body={t('library.emptyLibraryBody')}/>;
  const save = () => {
    if (!resources.controller.canSaveView()) return;
    const query = resources.controller.query();
    setSaving({libraryId: library.id, pivot: resources.controller.source.pivot, query: query.query ?? null,
      sort: query.sort ?? [], presentation: header.libraryViewOverride ?? appearance.libraryView});
  };
  const bindings: LibraryBindings = {...header, controller: resources.controller,
    env: {t, locale: defaultI18n.region.formatLocale}, reducedMotion: appearance.reducedMotion, owner: context.identity?.viewer.role === 'owner',
    reportError: context.effects.reportError, empty, canSaveView: resources.controller.canSaveView(), onSaveView: save,
    selection: {toolbar: selection.toolbar, actions: (member, index) => ({...actions(member.entry), selection: selection.entry(member, index)}),
      registerWindow: selection.registerWindow, playSelection},
    onAppearanceChange: change => {if (!resources.life.current()) return;if (change.view) setChoice({defaultView: appearance.libraryView, view: change.view});if (change.cardSize) setSize(change.cardSize);}};
  const contentRoute: ContentRoute = {libraryId: library.id, view, ...(search.entityId ? {entityId: search.entityId} : {}), ...(search.category ? {category: search.category} : {}), ...(search.q ? {q: search.q} : {})};
  const unmatched = requested?.shows === 'unmatched';
  const editAttention = (entry: AttentionEntry, anchor: HTMLElement) => {
    if (!resources.life.current() || entry.libraryId !== library.id || entry.kind !== 'item') return;
    const kind = entry.itemKind;
    if (kind !== 'movie' && kind !== 'episode' && kind !== 'song' && kind !== 'audiobook_file') return;
    const edit = context.effects.editMetadata;
    if (edit) void edit({libraryId: library.id, itemId: entry.entityId, kind, title: entry.title}, anchor).catch(context.effects.reportError);
  };
  const openAttention = (entry: {kind: string; itemKind: string; entityId: string; parentId: string}) => {
    if (entry.kind === 'show' || entry.itemKind === 'episode') navigate({view: 'show', entityId: entry.kind === 'show' ? entry.entityId : entry.parentId});
    else if (entry.kind === 'album') navigate({view: 'album', entityId: entry.entityId});
    else if (entry.kind === 'song') navigate({view: 'album', entityId: entry.parentId});
    else if (entry.kind === 'book') navigate({view: 'book', entityId: entry.entityId});
    else if (entry.entityId) navigate({view: 'browse', entityId: entry.entityId});
  };
  if (library.kind === 'audiobook' && width < 1024 && (view === 'book' || view === 'author' || view === 'book_series') && search.entityId)
    return <AudiobookEntry context={context} target={{libraryId: library.id, view, entityId: search.entityId}}/>;
  return <>{unmatched ? <><LibraryIdentity {...header}/><NeedsAttention libraryId={library.id} libraryName={library.name} api={resources.life.api} onOpen={openAttention} onEdit={context.effects.editMetadata ? editAttention : undefined}/></>
    : view === 'show' || view === 'season' ? <LibraryShow context={context} libraryId={library.id} search={search}/>
    : browse ? <LibraryScreen {...bindings}/> : <LibraryDiscoveryScreen service={resources.service} route={contentRoute} musicMixes={musicMixes} header={<LibraryIdentity {...header}/>} empty={empty}
      appearance={appearanceBase} preferences={appearance} reducedMotion={appearance.reducedMotion} artwork={context.artwork} env={bindings.env}
      actions={(_section, entry) => actions(entry)} onSeeAll={section => {const target = section.seeAll;if (target) navigate({pivot: target.pivot, query: target.query, sort: target.sort});}}/>}
    {saving && <SaveView context={context} definition={saving} onClose={() => setSaving(undefined)}/>}</>;
}
function LibraryRouteScreen() {
  const captured = useProtectedWebScreenContext(), {libraryId} = useParams({from: '/library/$libraryId'});
  const search = parseLibraryScreenSearch(useSearch({from: '/library/$libraryId'}));
  const library = captured?.context.facts.identity.libraries.find(library => library.id === libraryId);
  return !captured ? null : library ? isReadingLibrary(library.kind) ? <ReadingLibrary key={libraryId} captured={captured} library={library} search={search}/>
    : <BoundLibrary key={libraryId} captured={captured} library={library} search={search}/>
    : <StateView title={t('next.states.notFound')} body={t('next.states.notFoundBody')}/>;
}
export default defineWebScreen({id: 'library', component: LibraryRouteScreen, validateSearch: parseLibraryScreenSearch});
