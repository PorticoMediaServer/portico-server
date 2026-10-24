import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {useParams} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {fetchHomeRow, type HomeRow} from '@core/home.ts';
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

function LoadedHomeRow({captured, context, row}: {captured: ProtectedLibraryScreen; context: ScreenContext; row: HomeRow}) {
  const appearance = useScreenAppearance(captured.context)!, width = useShellWidth();
  const owner = useMemo(() => {
    const life = screenLifetime(context);
    const controller = new BrowseController(life.api, {libraryId: '*', pivot: acrossPivot('homeRow'),
      homeRowId: row.id, base: [acrossCondition('homeRow', row.id)], sort: {field: groupOrderSort, direction: 'asc'}}, life.current);
    return {life, controller};
  }, [context, row.id]);
  useEffect(() => () => {owner.life.dispose();owner.controller.dispose();}, [owner]);
  const state = useSyncExternalStore(owner.controller.subscribe, owner.controller.getSnapshot, owner.controller.getSnapshot);
  const page = useSyncExternalStore(owner.controller.collection.subscribe, owner.controller.collection.getSnapshot, owner.controller.collection.getSnapshot);
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
  const [choice, setChoice] = useState<Readonly<{defaultView: 'grid' | 'list'; view: 'grid' | 'list'}>>();
  const [size, setSize] = useState<'small' | 'medium' | 'large'>('medium');
  const library = context.identity?.libraries.find(library => library.id === row.libraryId);
  const browse: LibraryBrowseBindings = {controller: owner.controller, library: {id: '*', name: row.title, kind: library?.kind ?? ''},
    pivot: owner.controller.source.pivot, artwork: context.artwork,
    appearance: {view: appearance.libraryView, cardWidth: (width < 600 ? 112 : 170) * (size === 'small' ? .8 : size === 'large' ? 1.2 : 1), backdrops: appearance.showBackdrops, cardSize: size},
    preferences: appearance, libraryViewOverride: choice?.defaultView === appearance.libraryView ? choice.view : undefined,
    shape: row.artworkShape === 'square' ? 'square' : 'poster',
    onAppearanceChange: change => {if (!context.isCurrent()) return;if (change.view) setChoice({defaultView: appearance.libraryView, view: change.view});if (change.cardSize) setSize(change.cardSize);},
    owner: context.identity?.viewer.role === 'owner', reducedMotion: appearance.reducedMotion,
    env: {t: defaultI18n.t, locale: defaultI18n.region.formatLocale}, reportError: context.effects.reportError,
    canSaveView: false, empty: undefined,
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
  return <HomeRowScreen row={row} browse={browse} ambient={ambient} onBack={() => {void router.navigate({to: '/'}).catch(context.effects.reportError);}}
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
export default defineWebScreen({id: 'homeRow', component: HomeRowRouteScreen});
