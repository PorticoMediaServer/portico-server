import {useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {Artwork} from '../../kit/artwork';
import type {ShellArtwork} from '../../shell/artwork';
import {savedCatalogueEntry} from './catalogue-selection';
import {sameSavedRoute} from './route';
import {Icon} from '../../kit/Icon';
import type {SavedRuntime, SavedScreenRoute} from './runtime.ts';
import styles from './saved.module.css';

const t = defaultI18n.t;
/** phone-web frame 6: Saved is a list of rows on a phone, each opening its own
 * pushed page. Counts are the summary's published fields only; no row is offered
 * that the runtime cannot open, and there is no selection or write here. */
export function SavedPhoneDirectory({runtime, downloadsOffered, artwork, select}: {
  runtime: SavedRuntime;
  downloadsOffered: boolean;
  artwork: ShellArtwork;
  select(route: SavedScreenRoute): void;
}) {
  const summary = useSyncExternalStore(runtime.summary.subscribe, runtime.summary.getSnapshot, runtime.summary.getSnapshot);
  const posters = useSyncExternalStore(runtime.posters.subscribe, runtime.posters.getSnapshot, runtime.posters.getSnapshot);
  const squares = useSyncExternalStore(runtime.squares.subscribe, runtime.squares.getSnapshot, runtime.squares.getSnapshot);
  const value = summary.value;
  // The drawn My List rail: at most three covers, only from the already-admitted
  // canonical My List projections. No read is made for it and none is invented.
  const watchlist = [posters, squares].find(state => state.phase === 'ready' && !!state.projection
    && sameSavedRoute(state.route, {...state.route!, view: 'watchlist'}));
  const preview = watchlist?.projection?.sections.flatMap(section => section.entries)
    .flatMap(entry => {const media = savedCatalogueEntry(entry); return media?.posterUrl ? [media] : [];}).slice(0, 3) ?? [];
  const count = (shapes: Readonly<{posters: number; squares: number}> | undefined) => shapes === undefined ? undefined : shapes.posters + shapes.squares;
  const rows: readonly Readonly<{id: string; label: string; count?: number; icon: Parameters<typeof Icon>[0]['name']; route: SavedScreenRoute}>[] = [
    {id: 'watchlist', label: t('saved.tab.watchlist'), count: count(value?.myList), icon: 'i-bookmark', route: {view: 'watchlist'}},
    {id: 'favorites', label: t('saved.tab.favorites'), count: count(value?.favorites), icon: 'i-heart', route: {view: 'favorites'}},
    {id: 'playlists', label: t('saved.tab.playlists'), count: value?.playlists, icon: 'i-queue', route: {view: 'playlists'}},
    {id: 'collections', label: t('saved.tab.collections'), count: value?.collections, icon: 'i-stack', route: {view: 'collections'}},
    {id: 'views', label: t('saved.tab.savedViews'), count: value?.savedViews, icon: 'i-filter', route: {view: 'views'}},
    {id: 'history', label: t('saved.tab.history'), icon: 'i-clock', route: {view: 'history'}},
    ...(downloadsOffered ? [{id: 'downloads', label: t('downloads.title'), icon: 'i-dl' as const, route: {view: 'downloads'} as SavedScreenRoute}] : []),
  ];
  return <div className={styles.phoneDirectory}>
    <header className={styles.header}><h1>{t('saved.title')}</h1></header>
    {summary.error && !value ? <p className={styles.directoryHelp} role="alert">{t('saved.loadFailed')}</p> : null}
    {preview.length > 0 && <div className={styles.phonePreview}>
      <div className={styles.phonePreviewHead}><b>{t('saved.tab.watchlist')}</b>{count(value?.myList) !== undefined && <span>{defaultI18n.number(count(value!.myList)!)}</span>}<Icon name="i-right" size={16} /></div>
      <div className={styles.phonePreviewRail}>{preview.map(media => <span key={media.id} className={styles.phonePreviewCard}>{artwork.foreground(media.posterUrl, 96 * devicePixelRatio, 2 / 3) && <Artwork source={artwork.foreground(media.posterUrl, 96 * devicePixelRatio, 2 / 3)} alt="" />}</span>)}</div>
    </div>}
    <div className={styles.phoneDirectoryRows} role="list">
      {rows.map(row => <div key={row.id} role="listitem">
        <Button variant="text" size="small" className={styles.phoneDirectoryRow} onClick={() => select(row.route)}>
          <span className={styles.phoneDirectoryIcon}><Icon name={row.icon} size={18} /></span>
          <span className={styles.phoneDirectoryLabel}>{row.label}</span>
          {row.count !== undefined && <span className={styles.phoneDirectoryCount}>{defaultI18n.number(row.count)}</span>}
          <Icon name="i-right" size={18} className={styles.phoneDirectoryChevron} />
        </Button>
      </div>)}
    </div>
  </div>;
}
