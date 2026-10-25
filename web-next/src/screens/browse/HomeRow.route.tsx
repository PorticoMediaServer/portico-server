import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useParams, useSearch} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {fetchHomeRow, type HomeRow} from '@core/home.ts';
import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';
import {acrossCondition, acrossPivot, groupOrderSort} from '@core/presentation/browse-groups.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {GridSkeleton} from '../../kit/states';
import type {AmbientArtSource} from '../../kit/artwork';
import {useShellWidth} from '../../shell/viewport';
import {screenLifetime, type ScreenContext} from '../detail/context';
import {ScreenProblem} from '../detail/ScreenProblem';
import {BrowseController} from './controller';
import type {LibraryBrowseBindings} from './Library';
import {HomeRowScreen} from './HomeRow';
import {useScreenAppearance} from './screen-preferences';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from './screen-context';
import {browseConjunction, parseHomeRowScreenSearch, type HomeRowScreenSearch} from './route-search';

type RowQuery = Readonly<{predicates: readonly BrowseNode[]; sorts: readonly BrowseSortSelection[]}>;
/** Best match is the row's own server order; the frame draws no other default. */
const bestMatch: readonly BrowseSortSelection[] = [{field: groupOrderSort, direction: 'asc'}];
const appliedSorts = (sorts: readonly BrowseSortSelection[]) => sorts.map(({field, direction, seed}) =>
  ({field, direction, ...(seed === undefined ? {} : {seed: seed})}));
/** The address and the controller are compared in one vocabulary: the page's own
 * scoped predicates and every applied sort, in the order the server receives them. */
function rowQuery(search: HomeRowScreenSearch): RowQuery {
  return {predicates: search.query ? browseConjunction(search.query) : [], sorts: appliedSorts(search.sort?.length ? search.sort : bestMatch)};
}
function queryKey(query: RowQuery): string {return JSON.stringify({predicates: query.predicates, sorts: query.sorts});}
function isBestMatch(sorts: readonly BrowseSortSelection[]): boolean {
  return sorts.length === 1 && sorts[0]?.field === groupOrderSort && sorts[0].direction === 'asc';
}

function LoadedHomeRow({captured, context, row}: {captured: ProtectedLibraryScreen; context: ScreenContext; row: HomeRow}) {
  const appearance = useScreenAppearance(captured.context)!, width = useShellWidth();
  // Frame 47's toolbar changes are this page's real state, so its own address carries
  // them: a reload, a share and Back from a title land on the same filtered grid. The
  // row's membership and the viewer's grant stay server-owned; only the toolbar's
  // scoped filters and order are read back from the route. A genuine navigation that
  // changes this address (browser Back/Forward or a link) is consumed into the
  // controller, never overwritten by the state the page held before it.
  const rawSearch = useSearch({from: '/home/rows/$rowId'});
  const routed = useMemo(() => rowQuery(parseHomeRowScreenSearch(rawSearch)), [rawSearch]);
  const routeKey = useMemo(() => queryKey(routed), [routed]);
  const [build, setBuild] = useState(routed);
  const settled = useRef(routeKey), written = useRef(new Set<string>()), lastRoute = useRef(routeKey);
  // The history action that produced the current address, so a genuine navigation is
  // never mistaken for this page's own replace echo.
  const arrival = useRef<Readonly<{href: string; type: string}>>();
  useEffect(() => router.history.subscribe(({location, action}) => {arrival.current = {href: location.href, type: action.type};}), []);
  const owner = useMemo(() => {
    const life = screenLifetime(context);
    const controller = new BrowseController(life.api, {libraryId: '*', pivot: acrossPivot('homeRow'),
      homeRowId: row.id, base: [acrossCondition('homeRow', row.id)], ...(build.predicates.length ? {predicates: build.predicates} : {}),
      sorts: build.sorts}, life.current);
    return {life, controller};
  }, [context, row.id, build]);
  useEffect(() => () => {owner.life.dispose();owner.controller.dispose();}, [owner]);
  const state = useSyncExternalStore(owner.controller.subscribe, owner.controller.getSnapshot, owner.controller.getSnapshot);
  const page = useSyncExternalStore(owner.controller.collection.subscribe, owner.controller.collection.getSnapshot, owner.controller.collection.getSnapshot);
  // The controller's real applied state, in the same vocabulary as the address.
  const controllerQuery = useMemo<RowQuery | undefined>(() => {
    if (state.phase !== 'ready') return undefined;
    try {
      const sorts = owner.controller.query().sort ?? [];
      return {predicates: state.predicates, sorts: appliedSorts(sorts.length ? sorts : bestMatch)};
    } catch {return undefined;}
  }, [state.phase, state.predicates, state.queryRevision, owner]);
  const controllerKey = controllerQuery && queryKey(controllerQuery);
  // Frame #seeall draws the page's own art as a three-image wash behind the header; the
  // sources are the titles actually on this page, so nothing is drawn before they arrive.
  const ambient = useMemo(() => {
    if (appearance.showBackdrops === false || !page.total) return undefined;
    const seen = new Set<string>(), sources: AmbientArtSource[] = [];
    for (const member of owner.controller.collection.residentItems()) {
      const url = member.entry.posterUrl;
      if (!url || seen.has(url)) continue;
      seen.add(url);
      const source = context.artwork.ambient(url, {group: 'home-row', order: sources.length});
      if (!source) continue;
      sources.push(source);
      if (sources.length === 3) break;
    }
    return sources;
  }, [owner, context.artwork, appearance.showBackdrops, page]);
  // The toolbar writes the page's own address, so Back from a title and a reload restore
  // the same scoped filters and order. One comparison settles both authorities: an
  // address that moved while the controller still held the previous one is a real
  // navigation, consumed into the controller; a controller that moved while the address
  // still held the previous one is the toolbar, published on the address. Our own
  // replace echo lands with both equal and does nothing, so it never rebuilds the page.
  useEffect(() => {
    if (!controllerQuery || !controllerKey || !owner.life.current() || !context.isCurrent()) return;
    const moved = routeKey !== lastRoute.current;
    if (moved) lastRoute.current = routeKey;
    if (routeKey === controllerKey) {settled.current = routeKey; return;}
    if (controllerKey !== settled.current) {
      // The controller moved after the last agreement: publish its real scoped state.
      // A stale own echo (an older replace arriving after a newer one) is republished
      // here rather than treated as a navigation.
      const navigated = moved && arrival.current?.href === router.state.location.href && arrival.current.type !== 'REPLACE';
      if (routeKey !== settled.current && (!written.current.has(routeKey) || navigated)) {setBuild(routed); return;}
      const next = {...(controllerQuery.predicates.length ? {query: controllerQuery.predicates.length === 1 ? controllerQuery.predicates[0] : {all: [...controllerQuery.predicates]}} : {}),
        ...(isBestMatch(controllerQuery.sorts) ? {} : {sort: controllerQuery.sorts})};
      if (written.current.size > 7) for (const key of written.current) {written.current.delete(key); break;}
      written.current.add(controllerKey);
      void router.navigate({to: '/home/rows/$rowId', params: {rowId: row.id}, search: next, replace: true, resetScroll: false})
        .catch(error => {if (owner.life.current() && context.isCurrent()) context.effects.reportError(error);});
      return;
    }
    // The address moved while the controller still held it: a genuine navigation
    // (browser Back/Forward or a link). Rebuild from the validated address instead of
    // writing the previous state over it.
    setBuild(routed);
  }, [routeKey, controllerKey, controllerQuery, owner, row.id, routed, context]);
  const [choice, setChoice] = useState<Readonly<{defaultView: 'grid' | 'list'; view: 'grid' | 'list'}>>();
  const [size, setSize] = useState<'small' | 'medium' | 'large'>('medium');
  const library = context.identity?.libraries.find(library => library.id === row.libraryId);
  const browse: LibraryBrowseBindings = {controller: owner.controller, library: {id: '*', name: row.title, kind: library?.kind ?? ''},
    pivot: owner.controller.source.pivot, artwork: context.artwork,
    appearance: {view: appearance.libraryView, cardWidth: (width < 600 ? 103 : 170) * (size === 'small' ? .8 : size === 'large' ? 1.2 : 1), backdrops: appearance.showBackdrops, cardSize: size},
    preferences: appearance, libraryViewOverride: choice?.defaultView === appearance.libraryView ? choice.view : undefined,
    shape: row.artworkShape === 'square' ? 'square' : 'poster',
    onAppearanceChange: change => {if (!context.isCurrent()) return;if (change.view) setChoice({defaultView: appearance.libraryView, view: change.view});if (change.cardSize) setSize(change.cardSize);},
    owner: context.identity?.viewer.role === 'owner', reducedMotion: appearance.reducedMotion,
    env: {t: defaultI18n.t, locale: defaultI18n.region.formatLocale}, reportError: context.effects.reportError,
    canSaveView: false, empty: undefined,
    // Frame #seeall draws one quick toggle and names the row's own order "Best match";
    // the scope chips take the viewer's library names ("TV Shows"), not the kind's generic name.
    sortLabel: defaultI18n.t('next.browse.bestMatch'), quickIds: ['unwatched'],
    scopeLabel: kind => context.identity?.libraries.find(library => library.kind === kind)?.name ?? kind,
    familyForEntry: entry => context.identity?.libraries.find(library => library.id === entry.libraryId)?.kind,
    scopeLibraries: context.identity?.libraries,
    selection: {actions: member => ({onOpen: () => context.effects.openEntry(member.entry),
      onPlay: member.entry.playback && member.entry.libraryId && member.entry.available !== false && !member.entry.missing ? () => {
        const entry = member.entry;
        void context.effects.play({libraryId: entry.libraryId!, itemId: entry.playback!.itemId, startSeconds: entry.playback!.startSeconds ?? 0}, owner.life.signal()).catch(context.effects.reportError);
      } : undefined})}};
  const play = captured.binding.playback && state.phase === 'ready' ? (shuffle: boolean) => {
    const playback = captured.binding.playback!, query = owner.controller.query(), selected = JSON.stringify(query), command = new AbortController();
    const current = () => {try {return owner.life.current() && JSON.stringify(owner.controller.query()) === selected;} catch {return false;}};
    const off = owner.controller.subscribe(() => {if (!current()) command.abort();});
    void playback.runtime.playFromQueue({query: {libraryId: '*', pivot: owner.controller.source.pivot, query: query.query, sort: query.sort}},
      {signal: owner.life.signal(command.signal), isCurrent: current}, {shuffle})
      .catch(context.effects.reportError).finally(off);
  } : undefined;
  const backHome = () => {
    if (!owner.life.current() || !context.isCurrent()) return;
    // The drawn "‹ Home" leaves the way the viewer came. The router's history index
    // counts only entries this app pushed, so a tab that opened See all first has
    // nothing to return to and gets an explicit Home instead of leaving for unrelated
    // browser history.
    if (router.history.canGoBack()) router.history.back();
    else void router.navigate({to: '/'}).catch(error => {if (owner.life.current() && context.isCurrent()) context.effects.reportError(error);});
  };
  return <HomeRowScreen row={row} browse={browse} ambient={ambient} onBack={backHome}
    onPlay={play ? () => play(false) : undefined} onShuffle={play ? () => play(true) : undefined}/>;
}
function BoundHomeRow({captured, rowId}: {captured: ProtectedLibraryScreen; rowId: string}) {
  const context = useLibraryScreenContext(captured), width = useShellWidth();
  const [row, setRow] = useState<HomeRow>(), [failure, setFailure] = useState<unknown>(), [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const life = screenLifetime(context);setFailure(undefined);
    void fetchHomeRow(life.api, rowId, {limit: 1}, life.signal()).then(row => {if (life.current()) setRow(row);})
      .catch(error => {if (life.current()) setFailure(error);});
    return life.dispose;
  }, [context, rowId, attempt]);
  return failure ? <ScreenProblem context={context} error={failure} retry={() => setAttempt(value => value + 1)}/>
    : row ? <LoadedHomeRow captured={captured} context={context} row={row}/>
      : <GridSkeleton elapsedMs={300} window={{width, height: window.innerHeight, scrollTop: 0}} itemCount={1}/>;
}
function HomeRowRouteScreen() {
  const captured = useProtectedWebScreenContext(), {rowId} = useParams({from: '/home/rows/$rowId'});
  return captured ? <BoundHomeRow key={rowId} captured={captured} rowId={rowId}/> : null;
}
export default defineWebScreen({id: 'homeRow', component: HomeRowRouteScreen, validateSearch: parseHomeRowScreenSearch});
