import {createSavedCatalogueSelection} from './catalogue-selection';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useSearch} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useShellWidth} from '../../shell/viewport';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {useScreenAppearance} from '../browse/screen-preferences';
import {SavedScreen, type SavedEffects} from './Saved';
import {SavedHost, useRetainedSavedOwner} from './SavedHost';
import type {RetainedSavedOwner} from './retained-owner';
import {parseSavedScreenRoute, savedRouteKey} from './route';

const networkSnapshot = () => navigator.onLine;
const networkSubscribe = (changed: () => void) => {
  window.addEventListener('online', changed);window.addEventListener('offline', changed);
  return () => {window.removeEventListener('online', changed);window.removeEventListener('offline', changed);};
};
function BoundSaved({captured, owner}: {captured: ProtectedLibraryScreen; owner: RetainedSavedOwner}) {
  const context = useLibraryScreenContext(captured), appearance = useScreenAppearance(captured.context)!;
  const search = useSearch({from: '/saved'});
  const route = useMemo(() => parseSavedScreenRoute(search), [search]);
  const selectedRoute = useRef(route), playback = useRef(captured.binding.playback);
  selectedRoute.current = route;playback.current = captured.binding.playback;
  const hasPlayback = !!captured.binding.playback;
  const online = useSyncExternalStore(networkSubscribe, networkSnapshot, () => true), width = useShellWidth();
  const [choice, setChoice] = useState<Readonly<{defaultView: 'grid' | 'list'; view: 'grid' | 'list'}>>();
  const runtime = owner.runtime, native = useSyncExternalStore(owner.subscribe, owner.getSnapshot, owner.getSnapshot);
  const routeKey = savedRouteKey(route);
  const selection = useMemo(() => {
    const viewer = captured.context.session().viewer;
    if (viewer.authority !== 'local' && viewer.authority !== 'hosted') throw new DOMException('Saved authority unavailable', 'AbortError');
    return createSavedCatalogueSelection(context, runtime, route, {serverId: viewer.serverId, authority: viewer.authority,
      accountId: viewer.accountId, profileId: viewer.profileId}, () => captured.context.viewerFence, () => savedRouteKey(selectedRoute.current) === routeKey, captured.binding.playback?.runtime);
  }, [context, runtime, route, routeKey, captured.context, captured.binding.playback?.runtime]);
  useEffect(() => {selection.start();return () => selection.dispose();}, [selection]);
  const effects: SavedEffects = useMemo(() => ({
    openEntry: context.effects.openEntry,
    openReadingGroup(card) {
      if (!context.isCurrent() || !runtime.operations.current || selectedRoute.current.view !== 'watchlist') return;
      void router.navigate({to: '/library/$libraryId', params: {libraryId: card.libraryId}, search: {view: 'series', entityId: card.readingGroup.seriesId,
        ...(card.navigation.pivot ? {pivot: card.navigation.pivot} : {})}}).catch(context.effects.reportError);
    },
    async playEntry(entry: ContentEntry, signal: AbortSignal) {
      if (!entry.libraryId || !entry.playback || entry.available === false || entry.missing) throw new DOMException('Saved playback changed', 'AbortError');
      await context.effects.play({libraryId: entry.libraryId, itemId: entry.playback.itemId,
        startSeconds: entry.playback.startSeconds ?? 0}, signal);
    },
    selectRoute(value) {if (context.isCurrent()) void router.navigate({to: '/saved', search: value}).catch(context.effects.reportError);},
    openItem(itemId) {if (context.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId}}).catch(context.effects.reportError);},
    async saveFile(preparation, signal) {
      signal.throwIfAborted();
      if (!context.isCurrent()) throw new DOMException('Saved authority changed', 'AbortError');
      const grant = await runtime.downloads.grant(preparation);
      signal.throwIfAborted();
      if (!context.isCurrent()) throw new DOMException('Saved authority changed', 'AbortError');
      window.location.assign(context.api.mediaUrl(grant.url));
    },
    reportError: context.effects.reportError,
    ...(native.saveReadyPreparation ? {saveReadyPreparation: native.saveReadyPreparation} : {}),
    ...(hasPlayback ? {async playResource(target: {kind: 'playlist' | 'collection' | 'view'; id: string; shuffle: boolean}, signal: AbortSignal) {
      const key = savedRouteKey(selectedRoute.current), capturedPlayback = playback.current;
      const current = () => context.isCurrent() && runtime.operations.current && savedRouteKey(selectedRoute.current) === key;
      if (!current() || !capturedPlayback) throw new DOMException('Saved playback changed', 'AbortError');
      const resource = target.kind === 'playlist' ? runtime.saved.getSnapshot().playlist : runtime.personal.getSnapshot().resource;
      if (resource?.id !== target.id || target.kind !== 'playlist' && runtime.personal.getSnapshot().resource?.kind !== target.kind) throw new DOMException('Saved resource changed', 'AbortError');
      await capturedPlayback.runtime.playFromQueue(target.kind === 'view' ? {saved: {id: target.id}} : {container: {kind: target.kind, id: target.id}},
        {signal, isCurrent: current}, {shuffle: target.shuffle});
    }} : {}),
  }), [context, runtime, hasPlayback, native.saveReadyPreparation]);
  return <SavedScreen catalogueSelection={selection} runtime={runtime} localDownloads={native.localDownloads} route={route} online={online} downloadsOffered artwork={context.artwork}
    preparationArtwork={preparation => {
      // Only already-authorized bounded Home facts are reused. There is no
      // guessed item image URL or per-row catalogue scan for missing artwork.
      const find = (entries: readonly ContentEntry[]) => entries.find(entry => entry.id === preparation.itemId && entry.libraryId === preparation.libraryId);
      const document = captured.context.home.getSnapshot().document;
      const entry = document && (find(document.spotlight.map(slide => slide.entry)) ?? document.rows.flatMap(row => row.entries).find(entry => entry.id === preparation.itemId && entry.libraryId === preparation.libraryId));
      return context.artwork.foreground(entry?.posterUrl, 120, preparation.title.kind === 'book' || preparation.title.kind === 'album' || preparation.title.kind === 'song' ? 1 : 2 / 3);
    }}
    presentation={{view: choice?.defaultView === appearance.libraryView ? choice.view : appearance.libraryView,
      cardWidth: (width < 600 ? 112 : 173.5) * appearance.cardSizePercent / 100, backdrops: appearance.showBackdrops, reducedMotion: appearance.reducedMotion,
      setView: view => {if (context.isCurrent()) setChoice({defaultView: appearance.libraryView, view});}}}
    effects={effects}/>;
}
function SavedRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <SavedRouteOwner captured={captured}/> : null;
}
function SavedRouteOwner({captured}: {captured: ProtectedLibraryScreen}) {
  const owner = useRetainedSavedOwner(captured.context);
  return owner ? <BoundSaved captured={captured} owner={owner}/> : null;
}
export default defineWebScreen({id: 'saved', component: SavedRouteScreen, retainedHost: SavedHost, validateSearch: parseSavedScreenRoute});
