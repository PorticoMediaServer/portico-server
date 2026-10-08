import React from 'react';
import {Link} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import type {Entry, TrackItem} from '@/data/types';
import {trackToEntry, useActions} from '@/app/actions';
import {usePlayer, usePlayerState} from '@/player/PlayerProvider';
import {formatDuration, formatTime, plural} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {ErrorState} from '@/ui/Bits';
import {Menu} from '@/ui/Overlay';
import {Icon} from '@/ui/Icon';
import {Shelf} from '@/ui/Shelf';
import {Section} from '@/ui/Page';
import {useTitle} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {DetailLoading} from '../common';
import {ActionRow, DetailFrame, Facts, MetaLine, Overview} from './parts';
import s from './Detail.module.css';

/** A track list: number (play on hover), title with a lyrics mark, album or artist, length, and each song's menu. */
export function TrackList({tracks, showAlbum, showArtist, discs, queueFrom}: {tracks: TrackItem[]; showAlbum?: boolean; showArtist?: boolean; discs?: boolean; queueFrom?: TrackItem[]}) {
  const player = usePlayer();
  const state = usePlayerState();
  const actions = useActions();
  const nowId = state.mode === 'music' ? state.queue[state.index]?.id : undefined;
  const all = queueFrom ?? tracks;
  const byDisc = discs && new Set(tracks.map(t => t.disc)).size > 1;
  let lastDisc = 0;
  return (
    <div className={cx(s.tracks, !showAlbum && !showArtist && s.tracksPlain)} role="table" aria-label="Songs">
      <div className={s.trackHead} role="row"><span role="columnheader" style={{textAlign: 'center'}}>#</span><span role="columnheader">Title</span>{showAlbum || showArtist ? <span role="columnheader" className={s.hideNarrow}>{showAlbum ? 'Album' : 'Artist'}</span> : null}<span role="columnheader" style={{textAlign: 'right'}}>Time</span><span /></div>
      {tracks.map((t, i) => {
        const disc = byDisc && t.disc !== lastDisc ? <div className={s.disc}><Icon name="music" size={14} />Disc {t.disc}</div> : null;
        lastDisc = t.disc;
        const e: Entry = trackToEntry(t);
        const now = nowId === t.id;
        return (
          <React.Fragment key={t.id}>
            {disc}
            <div className={cx(s.track, now && s.trackNow)} role="row" onDoubleClick={() => player.play(all.map(trackToEntry), all.indexOf(t), {mode: 'music'})}>
              <span className={s.trackNum} role="cell">
                <button type="button" className={s.trackNumBtn} aria-label={`Play ${t.title}`} onClick={() => player.play(all.map(trackToEntry), Math.max(0, all.indexOf(t)), {mode: 'music'})}>
                  <span className={s.trackNumLabel}>{now ? <Icon name="audio" size={16} /> : showAlbum || showArtist ? i + 1 : t.number}</span>
                  <span className={s.trackNumPlay}><Icon name="play" size={14} /></span>
                </button>
              </span>
              <span className={s.trackTitle} role="cell"><span>{t.title}</span>{t.hasLyrics ? <Icon name="lyrics" size={14} className={s.lyricsMark} label="Has lyrics" /> : null}</span>
              {showAlbum || showArtist ? <span className={cx(s.trackSub, s.hideNarrow)} role="cell">{showAlbum ? <Link to={`/media/${t.albumId}` as never}>{t.album}</Link> : <Link to={`/media/${t.artistId}` as never}>{t.artist}</Link>}</span> : null}
              <span className={s.trackTime} role="cell">{formatTime(t.duration)}</span>
              <span role="cell"><Menu label={`More for ${t.title}`} items={actions.items(e)} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`More for ${t.title}`} />} /></span>
            </div>
          </React.Fragment>
        );
      })}
    </div>
  );
}

export function AlbumDetail({id}: {id: string}) {
  const q = useQuery(c => c.catalog.album(id), [id], ['personal']);
  const player = usePlayer();
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  const queue = d.tracks.map(trackToEntry);
  return (
    <DetailFrame entry={d.entry} backdrop={d.artist.backdrop} shape="square"
      eyebrow={<>{d.type}<span aria-hidden>·</span><Link to={`/media/${d.artist.id}` as never}>{d.artist.title}</Link></>}
      sections={<>
        <section><TrackList tracks={d.tracks} discs /></section>
        {d.more.length ? <Shelf title={`More by ${d.artist.title}`} entries={d.more} shape="square" /> : null}
      </>}>
      <MetaLine items={[d.entry.year, d.genre, plural(d.tracks.length, 'song'), formatDuration(d.totalDuration)]} />
      <ActionRow entry={d.entry} personal={d.personal} watchlist={false} watchedToggle={false} onPlay={() => player.play(queue, 0, {mode: 'music'})} extra={<Button variant="secondary" size="lg" icon="shuffle" label="Shuffle" onClick={() => player.play([...queue].sort(() => Math.random() - 0.5), 0, {mode: 'music'})} />} />
      <Facts rows={[['Label', d.label], ['Released', String(d.entry.year ?? '')]]} />
    </DetailFrame>
  );
}

export function ArtistDetail({id}: {id: string}) {
  const q = useQuery(c => c.catalog.artist(id), [id], ['personal']);
  const actions = useActions();
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <DetailLoading />;
  const d = q.data;
  return (
    <DetailFrame entry={d.entry} shape="circle" eyebrow={<><Link to="/library/lib-music">Music</Link><span aria-hidden>·</span>{d.genre}</>}
      sections={<>
        <Section title="Popular">{null}</Section>
        <div style={{marginTop: -20}}><TrackList tracks={d.popular} showAlbum /></div>
        <Shelf title="Albums" entries={d.albums} shape="square" />
        {d.similar.length ? <Shelf title="Similar artists" entries={d.similar} shape="circle" /> : null}
      </>}>
      <MetaLine items={[d.genre, plural(d.albums.length, 'album')]} />
      <ActionRow entry={d.entry} personal={d.personal} watchlist={false} watchedToggle={false} onPlay={() => void actions.play(d.entry)} playLabel="Play" extra={<Button variant="secondary" size="lg" icon="shuffle" label="Shuffle" onClick={() => void actions.play(d.entry, {shuffle: true})} />} />
      <Overview text={d.bio} />
    </DetailFrame>
  );
}
