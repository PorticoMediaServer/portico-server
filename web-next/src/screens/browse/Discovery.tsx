import {useEffect, useRef, useSyncExternalStore, type ReactNode} from 'react';
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
import {PagedShelf} from './PagedShelf';
import type {BrowsePresentation} from './Library';
import {browsePresentation, type ScreenAppearance} from './appearance';
import type {MusicMixesBinding} from './music-mixes-binding';
import styles from './recent-tiles.module.css';

export type DiscoveryBindings = Readonly<{
  service: LibraryContentService; route: ContentRoute; artwork: ShellArtwork; env: TextEnvironment; appearance: BrowsePresentation; reducedMotion: boolean;
  preferences?: ScreenAppearance;
  musicMixes?: MusicMixesBinding;
  header: ReactNode; empty: ReactNode; selectionToolbar?: ReactNode;
  actions: (section: ContentSection, entry: ContentEntry, index: number) => BrowseItemActions;
  registerWindow?: (section: ContentSection, first: number, last: number) => void;
  onSeeAll: (section: ContentSection) => void;
  /** The album the viewer's own playback is playing now, from the shell runtime. */
  nowPlayingAlbumId?: string;
}>;
/** Sections and cursors are the authorized library projection, never a client-composed library scan. */
export function LibraryDiscoveryScreen({service, route, header, empty, artwork, env, appearance: base, preferences, reducedMotion, actions, registerWindow, selectionToolbar, onSeeAll, nowPlayingAlbumId}: DiscoveryBindings) {
  const appearance = browsePresentation(base, preferences);
  // The frame's library waits a quiet 300ms before its grid shapes, then keeps the real
  // elapsed clock so only a genuinely slow answer moves on at eight seconds.
  const elapsed = useLoadingClock();
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot), width = useShellWidth();
  const routeKey = JSON.stringify(route);
  const selected = useRef<Readonly<{service: LibraryContentService; key: string}> | undefined>(undefined);
  useEffect(() => {
    if (selected.current?.service === service && selected.current.key === routeKey) return;
    selected.current = {service, key: routeKey};void service.select(route);
  }, [service, route, routeKey]);
  if (!snapshot.projection && snapshot.phase === 'error') return <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => {void service.retry();}}}/>;
  return <>{header}{selectionToolbar}{!snapshot.projection ? <HomeSkeleton elapsedMs={elapsed} window={{width, height: window.innerHeight, scrollTop: window.scrollY}}/> :
    snapshot.appended.length === 0 ? empty : snapshot.appended.map(section => {
      // Phone-web frame 7: the Music library's Recently played row is the app's quick grid,
      // and the album the runtime is actually playing carries the level mark.
      if (width < 600 && snapshot.projection!.scope.libraryKind === 'music' && section.id === 'recently_played')
        return <RecentTiles key={section.id} section={section} actions={actions} artwork={artwork} nowPlayingAlbumId={nowPlayingAlbumId}/>;
      const square = snapshot.projection!.scope.libraryKind === 'music' || snapshot.projection!.scope.libraryKind === 'audiobook';
      const cardWidth = appearance.cardWidth, musicGenres = snapshot.projection!.scope.libraryKind === 'music' && section.id === 'genres';
      const shape = musicGenres ? cardWidth / 152 : square ? 1 : 2 / 3;
      return <PagedShelf key={section.id} heading={serverTextLabel(defaultI18n, {code: section.heading.key, fallback: section.heading.fallback, params: section.heading.params})}
        platform={width >= 1024 ? 'desktop' : 'phone-web'} reducedMotion={preferences?.reducedMotion ?? reducedMotion} items={section.entries} getKey={entry => entry.id} cardWidth={cardWidth} artworkRatio={shape}
        hasMore={!!section.nextCursor} continuation={section.nextCursor} onMore={() => {void service.next(section.id);}} onSeeAll={section.seeAll ? () => onSeeAll(section) : undefined}
        onVisibleRange={registerWindow ? (first, last) => registerWindow(section, first, last) : undefined} renderItem={(entry, context) => <BrowseEntryCard entry={entry} artwork={artwork} env={env} width={cardWidth}
          group={section.id} order={context.index} visible family={snapshot.projection!.scope.libraryKind} backdrops={appearance.backdrops} actions={actions(section, entry, context.index)}/>}/>;
    })}</>;
}
function RecentTiles({section, actions, artwork, nowPlayingAlbumId}: {section: ContentSection; actions: DiscoveryBindings['actions']; artwork: ShellArtwork; nowPlayingAlbumId?: string}) {
  const heading = serverTextLabel(defaultI18n, {code: section.heading.key, fallback: section.heading.fallback, params: section.heading.params});
  return <section className={styles.section} aria-label={heading}>
    <header className={styles.heading}><h2>{heading}</h2></header>
    <div className={styles.grid} role="list">
      {section.entries.map((entry, index) => {
        const action = actions(section, entry, index), art = artwork.foreground(entry.posterUrl, 104, 1), playing = entry.id === nowPlayingAlbumId;
        return <div key={entry.id} role="listitem" className={styles.item}>
          <button type="button" className={styles.tile} data-playing={playing || undefined} onClick={action.onOpen}>
            <span className={styles.art}>{art && <Artwork source={art} alt="" />}</span>
            <span className={styles.copy}><span className={styles.title}>{playing && <span className={styles.levels} role="img" aria-label={defaultI18n.t('next.browse.playing')}><i/><i/><i/></span>}{entry.displayTitle ?? entry.title}</span>
              <small className={styles.artist}>{entry.artist?.name}</small></span>
          </button>
        </div>;
      })}
    </div>
  </section>;
}
