import {useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {t} from './text';
import type {FactPart} from '@core/presentation/kit-model';
import {LibraryContentService, readListeningSimilar, type ContentEntry, type ContentProjection} from '@core/library-content.ts';
import {albumMusicFormatBadges, formatSignalText} from '@core/music-signal.ts';
import {defaultI18n} from '@i18n';
import {clockText} from '@core/presentation/kit-model/text.ts';
import {AmbientArt, Artwork} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Lockup} from '../../kit/layout';
import {TitleFactsLine} from '../../kit/title/TitleParts';
import {StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {runScreenCommand, type ScreenContext} from './context';
import {TitleRow} from './Detail';
import styles from './album.module.css';

export type AlbumActions = (entry: ContentEntry) => Readonly<{onOpen: () => void; onPlay?: () => void}>;
export type AlbumScreenProps = Readonly<{context: ScreenContext; libraryId: string; albumId: string; actions: AlbumActions; onBack?: () => void}>;
type Snapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; projection?: ContentProjection; songs: readonly ContentEntry[]}>;
const noSongs: readonly ContentEntry[] = Object.freeze([]);

/** desktop-listen's album page on its canonical library route (`?view=album&entityId=`): the album's
 * own content read supplies the cover, artist and real song list, and only those songs' measured
 * facts become the album's format badges. Per-song play uses the existing item effect; the album
 * container's Play/Shuffle waits for the queue-source seam named in the handoff. */
export function AlbumScreen({context, libraryId, albumId, actions, onBack}: AlbumScreenProps) {
  const phone = useShellWidth() < 1024;
  const [snapshot, setSnapshot] = useState<Snapshot>({phase: 'idle', songs: noSongs});
  const [similar, setSimilar] = useState<Readonly<{albumId: string; bySound: boolean; entries: readonly ContentEntry[]}>>();
  // One lease per album page: its own service and captured scope; retiring it cancels the read.
  const service = useMemo(() => new LibraryContentService({api: context.api, scope: {serverId: context.scope.serverId, viewerId: context.scope.viewerId}}), [context]);
  // One admission lifetime for this page's Play/Shuffle; retiring it cancels the start.
  const life = useMemo(() => new AbortController(), [service]);
  useEffect(() => () => life.abort(), [life]);
  useEffect(() => {
    const request = new AbortController();
    setSnapshot({phase: 'loading', songs: noSongs}); setSimilar(undefined);
    void service.select({libraryId, view: 'album', entityId: albumId}).then(() => {
      if (request.signal.aborted || !context.isCurrent()) return;
      const value = service.getSnapshot(), projection = value.projection, entity = projection?.entity;
      if (value.phase !== 'ready' || !projection || !entity || entity.id !== albumId || projection.scope.libraryId !== libraryId) {
        setSnapshot({phase: value.phase === 'ready' ? 'error' : value.phase, songs: noSongs});
        return;
      }
      setSnapshot({phase: 'ready', projection, songs: projection.sections.flatMap(section => section.entries).filter(entry => entry.kind === 'song')});
    }, error => { if (!request.signal.aborted && context.isCurrent()) { setSnapshot({phase: 'error', songs: noSongs}); context.effects.reportError(error); } });
    return () => { request.abort(); service.cancel(); };
  }, [service, libraryId, albumId, context]);
  useEffect(() => {
    if (snapshot.phase !== 'ready') return;
    const request = new AbortController(), viewer = context.identity?.viewer;
    if (!viewer || (viewer.authority !== 'local' && viewer.authority !== 'hosted')) return;
    void readListeningSimilar(context.api, {serverId: context.scope.serverId, authority: viewer.authority, accountId: viewer.accountId, profileId: viewer.profileId,
      controllerId: 'v1', controllerEpoch: 'v1', commandLaneId: 'v1'}, albumId, {limit: 12, by: 'sound', signal: request.signal})
      .then(value => { if (!request.signal.aborted && context.isCurrent() && value.itemId === albumId && value.libraryId === libraryId) setSimilar({albumId, bySound: value.bySound, entries: value.entries}); }, () => undefined);
    return () => request.abort();
  }, [snapshot.phase, context, libraryId, albumId]);
  useEffect(() => () => service.dispose(), [service]);
  const projection = snapshot.projection, album = projection?.entity, playAlbum = context.effects.playAlbum;
  // The caller's own witness: this admitted album projection, still the one this page drew.
  const callerCurrent = (captured: ContentProjection) => () => context.isCurrent() && service.getSnapshot().projection === captured;
  const badges = useMemo(() => albumMusicFormatBadges(snapshot.songs.flatMap(song => song.audioFacts ? [song.audioFacts] : [])), [snapshot]);
  if (snapshot.phase === 'idle' || snapshot.phase === 'loading') return <p className={styles.loading} role="status">{t('music.loading')}</p>;
  if (snapshot.phase === 'error' || !projection || !album) return <StateView title={t('title.nothingHere')} />;
  const parts: FactPart[] = [
    ...(album.year && album.year > 0 ? [{id: 'year', text: defaultI18n.number(album.year, {useGrouping: false})}] : []),
    {id: 'songs', text: t('lib.count', {count: snapshot.songs.length, pivot: 'songs'})},
  ];
  return <div className={styles.page}>
    <div className={styles.bar}>
      {onBack && <Button variant="icon" icon="i-left" iconOnly aria-label={t('next.states.back')} onClick={() => runScreenCommand(context, () => onBack())} />}
      <span className={styles.barTitle}>{album.title}</span>
    </div>
    <header className={styles.hero}>
      <span className={styles.art}><AmbientArt className={styles.ambient} treatment="phone" source={context.artwork.ambient(album.posterUrl)} foreground={context.artwork.foreground(album.posterUrl, 760, 1)} /><Artwork source={context.artwork.foreground(album.posterUrl, 760, 1)} alt="" /></span>
      <div className={styles.copy}>
        <Lockup title={album.title} platform={phone ? 'phone-web' : 'desktop'} />
        {album.artist?.name && <p className={styles.artist}>{album.artist.name}</p>}
        <TitleFactsLine parts={parts} />
        {badges.length > 0 && <p className={styles.badges}>{badges.map(badge => {
          const text = typeof badge === 'string' ? badge : formatSignalText(badge, t);
          return <span key={text} className={styles.badge}>{text}</span>;
        })}</p>}
        {playAlbum && <div className={styles.actions}>
          <Button variant="primary" icon="i-play" onClick={() => runScreenCommand(context, () => playAlbum(albumId, false, life.signal, callerCurrent(projection)))}>{t('next.controls.play')}</Button>
          <Button variant="glass" icon="i-shuffle" onClick={() => runScreenCommand(context, () => playAlbum(albumId, true, life.signal, callerCurrent(projection)))}>{t('title.shuffle')}</Button>
        </div>}
      </div>
    </header>
    <ol className={styles.songs} aria-label={t('music.allSongs')}>
      {snapshot.songs.map((song, index) => {
        const act = actions(song), title = song.displayTitle ?? song.title;
        return <li key={song.id} className={styles.song}>
          <span className={styles.index}>{defaultI18n.number(index + 1)}</span>
          <button type="button" className={styles.songTitle} disabled={!act.onPlay} aria-label={t('next.browse.play', {title})} onClick={() => act.onPlay?.()}>{title}</button>
          <span className={styles.length}>{clockText(song.duration)}</span>
        </li>;
      })}
    </ol>
    {similar?.albumId === albumId && similar.entries.length > 0 && <TitleRow heading={t('music.moreLikeThis')}
      subtitle={t(similar.bySound ? 'music.moreLikeThisBySound' : 'music.moreLikeThisHint')} entries={similar.entries} binding={{context}} />}
  </div>;
}
