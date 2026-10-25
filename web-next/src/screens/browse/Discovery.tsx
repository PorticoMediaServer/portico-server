import {useEffect, useMemo, useRef, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentService, ContentEntry, ContentRoute, ContentSection} from '@core/library-content.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import type {TextEnvironment} from '@portico/client-core/kit-model';
import type {ShellArtwork} from '../../shell/artwork';
import {HomeSkeleton, StateView} from '../../kit/states';
import {useLoadingClock} from '../detail/Loading';
import {Artwork} from '../../kit/artwork';
import {useShellWidth} from '../../shell/viewport';
import {BrowseEntryCard, type BrowseItemActions} from './EntryCard';
import {CatalogueSelectionEntry} from './document-selection';
import {useLibraryCardMenu} from './use-card-menu';
import {useDiscoverySelection, discoveryEntries, discoverySections, type DiscoverySelectionInput} from './discovery-selection';
import {PagedShelf} from './PagedShelf';
import type {BrowsePresentation} from './Library';
import {browsePresentation, type ScreenAppearance} from './appearance';
import type {MusicMixesBinding} from './music-mixes-binding';
import {MusicMixesRow} from './MusicMixesRow';
import styles from './recent-tiles.module.css';

export type DiscoveryBindings = Readonly<{
  service: LibraryContentService; route: ContentRoute; artwork: ShellArtwork; env: TextEnvironment; appearance: BrowsePresentation; reducedMotion: boolean;
  preferences?: ScreenAppearance;
  musicMixes?: MusicMixesBinding;
  mixesTarget?: boolean;
  reportError: (error: unknown) => void;
  header: ReactNode; empty: ReactNode; selectionToolbar?: ReactNode;
  /** The pivot's own explicit selection authority; absent leaves its cards unselectable. */
  selectionInput?: Omit<DiscoverySelectionInput, 'service' | 'route'>;
  actions: (section: ContentSection, entry: ContentEntry, index: number) => BrowseItemActions;
  registerWindow?: (section: ContentSection, first: number, last: number) => void;
  onSeeAll: (section: ContentSection) => void;
  /** The album the viewer's own playback is playing now, from the shell runtime. */
  nowPlayingAlbumId?: string;
}>;
/** Sections and cursors are the authorized library projection, never a client-composed library scan. */
export function LibraryDiscoveryScreen({service, route, header, empty, artwork, env, appearance: base, preferences, reducedMotion, actions, registerWindow, selectionToolbar, selectionInput, onSeeAll, nowPlayingAlbumId, musicMixes, mixesTarget, reportError}: DiscoveryBindings) {
  const appearance = browsePresentation(base, preferences);
  // The frame's library waits a quiet 300ms before its grid shapes, then keeps the real
  // elapsed clock so only a genuinely slow answer moves on at eight seconds.
  const elapsed = useLoadingClock();
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot), width = useShellWidth();
  // The pivot's selection document is built once for this committed screen; the
  // shelves only ever offer their admitted, mounted rows.
  const selection = useDiscoverySelection(selectionInput ? {...selectionInput, service,
    route: {libraryId: route.libraryId, view: route.view, entityId: route.entityId}} : undefined);
  const routeKey = JSON.stringify(route);
  const selected = useRef<Readonly<{service: LibraryContentService; key: string}> | undefined>(undefined);
  useEffect(() => {
    if (selected.current?.service === service && selected.current.key === routeKey) return;
    selected.current = {service, key: routeKey};void service.select(route);
  }, [service, route, routeKey]);
  // The menu's admission port is stable for this service and route: it reads the
  // service's own current generation and exact admitted rows, so an opening keeps
  // its identity across renders and never survives a replaced projection.
  const admission = useMemo(() => ({
    getSnapshot: () => ({queryRevision: service.getSnapshot().generation}),
    contains: (entry: ContentEntry) => {
      const current = service.getSnapshot(), projection = current.projection;
      if (!projection || current.phase !== 'ready') return false;
      const scope = projection.scope;
      if (scope.libraryId !== route.libraryId || (scope.view ?? '') !== (route.view ?? '') || (scope.entityId ?? '') !== (route.entityId ?? '')) return false;
      return discoveryEntries(current).some(candidate => candidate === entry);
    },
  }), [service, route.libraryId, route.view ?? '', route.entityId ?? '']);
  const {wrap: wrapCard, menu: cardMenu} = useLibraryCardMenu({admission, queryRevision: snapshot.generation});
  // The library frame is already known while its sections fail to load; keep it and put the
  // failure and its retry in place of the sections.
  if (!snapshot.projection && snapshot.phase === 'error') return <>{header}<StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => {void service.retry();}}}/></>;
  // The real render order over the merged shelves: the music continuation first,
  // then the rest. A card's registration position is its offset in that order, so
  // base and appended sections never share a starting position.
  const merged = discoverySections(snapshot);
  const recentSection = snapshot.projection?.scope.libraryKind === 'music' ? merged.find(section => section.id === 'recently_played') : undefined;
  const ordered = recentSection ? [recentSection, ...merged.filter(section => section !== recentSection)] : merged;
  const orderBase = new Map<string, number>(); let offset = 0;
  for (const section of ordered) {orderBase.set(section.id, offset); offset += section.entries.length;}
  const renderSection = (section: ContentSection) => {
      // Music's authored quick grid precedes Mixes on both web widths. Its entries
      // and scope come unchanged from this library's actual Discover projection.
      if (snapshot.projection!.scope.libraryKind === 'music' && section.id === 'recently_played')
        return <RecentTiles key={section.id} section={section} actions={actions} artwork={artwork} nowPlayingAlbumId={nowPlayingAlbumId}/>;
      const square = snapshot.projection!.scope.libraryKind === 'music' || snapshot.projection!.scope.libraryKind === 'audiobook';
      const cardWidth = appearance.cardWidth, musicGenres = snapshot.projection!.scope.libraryKind === 'music' && section.id === 'genres';
      const shape = musicGenres ? cardWidth / 152 : square ? 1 : 2 / 3;
      return <PagedShelf key={section.id} heading={serverTextLabel(defaultI18n, {code: section.heading.key, fallback: section.heading.fallback, params: section.heading.params})}
        platform={width >= 1024 ? 'desktop' : 'phone-web'} reducedMotion={preferences?.reducedMotion ?? reducedMotion} items={section.entries} getKey={entry => entry.id} cardWidth={cardWidth} artworkRatio={shape}
        hasMore={!!section.nextCursor} continuation={section.nextCursor} onMore={() => {void service.next(section.id);}} onSeeAll={section.seeAll ? () => onSeeAll(section) : undefined}
        onVisibleRange={registerWindow ? (first, last) => registerWindow(section, first, last) : undefined} renderItem={(entry, context) => <CatalogueSelectionEntry selection={selection} entry={entry} position={(orderBase.get(section.id) ?? 0) + context.index}>{cardSelection => <BrowseEntryCard entry={entry} artwork={artwork} env={env} width={cardWidth}
          group={section.id} order={context.index} visible family={snapshot.projection!.scope.libraryKind} backdrops={appearance.backdrops} actions={wrapCard({...actions(section, entry, context.index), ...(cardSelection ? {selection: cardSelection} : {})}, entry)}/>}</CatalogueSelectionEntry>}/>;
    };
  const music = snapshot.projection?.scope.libraryKind === 'music';
  const recent = music ? merged.find(section => section.id === 'recently_played') : undefined;
  return <>{header}{selection?.toolbar ?? selectionToolbar}{!snapshot.projection ? <HomeSkeleton elapsedMs={elapsed} window={{width, height: window.innerHeight, scrollTop: window.scrollY}}/> : <div className={styles.rows}>
    {recent && renderSection(recent)}
    {music && musicMixes && <MusicMixesRow binding={musicMixes} artwork={artwork} target={mixesTarget} empty={snapshot.appended.length === 0 ? empty : undefined} reducedMotion={preferences?.reducedMotion ?? reducedMotion} reportError={reportError}/>}
    {snapshot.appended.length === 0 ? !musicMixes && empty : snapshot.appended.filter(section => section !== recent).map(renderSection)}
  </div>}{cardMenu}</>;
}
function RecentTiles({section, actions, artwork, nowPlayingAlbumId}: {section: ContentSection; actions: DiscoveryBindings['actions']; artwork: ShellArtwork; nowPlayingAlbumId?: string}) {
  const artSize = (useShellWidth() >= 1024 ? 76 : 52) * devicePixelRatio;
  const heading = serverTextLabel(defaultI18n, {code: section.heading.key, fallback: section.heading.fallback, params: section.heading.params});
  return <section className={styles.section} aria-label={heading}>
    <header className={styles.heading}><h2>{heading}</h2></header>
    <div className={styles.grid} role="list">
      {section.entries.map((entry, index) => {
        const action = actions(section, entry, index), art = artwork.foreground(entry.posterUrl, artSize, 1), playing = entry.id === nowPlayingAlbumId;
        return <div key={entry.id} role="listitem" className={styles.item}>
          <button type="button" className={styles.tile} data-playing={playing || undefined} onClick={action.onOpen}>
            <span className={styles.art}>{art && <Artwork source={art} alt="" />}</span>
            <span className={styles.copy}><span className={styles.title}>{playing && <span className={styles.levels} role="img" aria-label={defaultI18n.t('next.browse.playing')}><i/><i/><i/></span>}{entry.kind === 'radio' ? defaultI18n.t('music.radioNamed', {name: entry.displayTitle ?? entry.title}) : entry.displayTitle ?? entry.title}</span>
              <small className={styles.artist}>{entry.artist?.name ?? (entry.kind === 'radio' ? defaultI18n.t('music.radio') : undefined)}</small></span>
          </button>
        </div>;
      })}
    </div>
  </section>;
}
