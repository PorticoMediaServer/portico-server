import React, {Suspense, lazy} from 'react';
import {Link, Navigate, useNavigate, useParams} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Album, Artist, Audiobook, Item, Movie, PhotoAlbum, TrackItem} from '../../data/types';
import {usePlayer} from '../../app/player';
import {Page, useTitle} from '../../ui/Layout';
import {Art} from '../../ui/Media';
import {Button, IconButton} from '../../ui/Button';
import {Badge, ErrorState, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Menu} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {ActionBar, CastShelf, Credits, DetailsSection, ExtrasShelf, Facts, PlaybackChoices, ReadMore, RelatedShelf, Stage, TitleSkeleton} from './parts';
import {Shelf} from '../shared/Shelf';
import {MediaCard} from '../shared/Cards';
import {bestResolution, formatClock, formatRemaining, formatRuntime, plural} from '../shared/items';
import {startPlayback} from '../shared/playback';
import {actions, useEntryMenu} from '../shared/actions';
import {player} from '../../app/player';

const PhotoAlbumView = lazy(() => import('./Photos').then(m => ({default: m.PhotoAlbumView})));

/** Every non-show title opens here: films, home videos, albums, artists, audiobooks, photo albums. */
export function MediaScreen() {
  const {itemId} = useParams({from: '/app/media/$itemId'});
  const services = useServices();
  useDataVersion();
  const q = useQuery(`item.${itemId}`, () => services.catalog.item(itemId));
  const item = q.data;
  useTitle(item?.title);
  if (q.error) return <Page><ErrorState error={q.error} onRetry={q.reload} what="this title" /></Page>;
  if (!item) return q.loading ? <Page flush><TitleSkeleton /></Page> : <Page><StateView icon="search" title="This title isn’t on the server any more" body="It may have been removed, or the link is from another server." action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  switch (item.kind) {
    case 'movie': case 'homevideo': return <MovieView movie={item as Movie} />;
    case 'album': return <AlbumView album={item as Album} />;
    case 'artist': return <ArtistView artist={item as Artist} />;
    case 'audiobook': return <AudiobookView book={item as Audiobook} />;
    case 'photoAlbum': return <Suspense fallback={<Page flush><TitleSkeleton /></Page>}><PhotoAlbumView album={item as PhotoAlbum} /></Suspense>;
    case 'episode': return <Navigate to="/show/$showId/episode/$episodeId" params={{showId: item.showId, episodeId: item.id}} replace />;
    case 'show': return <Navigate to="/show/$showId" params={{showId: item.id}} replace />;
    case 'track': return <Navigate to="/media/$itemId" params={{itemId: (item as TrackItem).albumId}} replace />;
    default: return <Page><StateView title="Portico can’t show this kind of item yet" /></Page>;
  }
}

function MovieView({movie}: {movie: Movie}) {
  const services = useServices();
  const st = services.user.state(movie.id);
  const home = movie.kind === 'homevideo';
  const res = bestResolution(movie);
  const v0 = movie.versions[0];
  const lib = home ? 'Home Videos' : 'Film';
  const director = movie.credits.find(c => c.job === 'Director');
  return (
    <Page flush className="title-page">
      <Stage art={movie.art.backdrop ?? movie.art.poster} tint={movie.tint}>
        <div className="eyebrow stage-eyebrow">{lib}{movie.genres.length ? ` · ${movie.genres.slice(0, 2).join(' · ')}` : ''}</div>
        <h1 className="display stage-title">{movie.title}</h1>
        <Facts
          items={[home && movie.date ? new Date(movie.date).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}) : movie.year, movie.contentRating !== 'NR' && movie.contentRating, formatRuntime(movie.runtime), home ? movie.place : null]}
          badges={<>
            {res ? <Badge tone="solid">{res}</Badge> : null}
            {v0?.hdr ? <Badge>{v0.hdr}</Badge> : null}
            {v0?.audio.includes('Atmos') ? <Badge>Atmos</Badge> : v0?.audio.includes('5.1') ? <Badge>5.1</Badge> : null}
            {movie.criticScore ? <span className="score" title="Critics"><Icon name="star" size={14} fill="currentColor" />{movie.criticScore}%</span> : null}
            {movie.audienceScore ? <span className="score is-aud" title="Audience"><Icon name="people" size={14} />{movie.audienceScore}%</span> : null}
          </>}
        />
        {movie.tagline ? <p className="stage-tagline">{movie.tagline}</p> : null}
        <ReadMore text={movie.synopsis} className="stage-syn" />
        <ActionBar item={movie} trailer={!home && movie.extras.some(x => x.kind === 'Trailer')} remaining={movie.runtime * 60 - st.position} />
        <PlaybackChoices versions={movie.versions} audio={movie.audioTracks} subs={movie.subtitleTracks} itemId={movie.id} />
        {!home ? <Credits credits={movie.credits} /> : null}
      </Stage>
      <div className="title-sections">
        {!home ? <ExtrasShelf extras={movie.extras} parentId={movie.id} /> : null}
        {movie.chapters.length ? <ChaptersShelf movie={movie} /> : null}
        {!home ? <CastShelf credits={movie.credits} /> : null}
        <RelatedShelf id={movie.id} title={home ? 'More home videos' : 'More like this'} />
        <div className="page-pad">
          <DetailsSection item={movie} about={[
            ...(home ? [] : [['Genres', movie.genres.join(', ')] as [string, string], ['Studio', movie.studio ?? '—'] as [string, string], ['Country', movie.country ?? '—'] as [string, string], ['Language', movie.language ?? 'English'] as [string, string]]),
            ...(director ? [['Director', <Link to="/person/$personId" params={{personId: director.personId}} className="link">{director.name}</Link>] as [string, React.ReactNode]] : []),
            ['Added', new Date(movie.addedAt).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'})],
            ...(movie.license ? [['License', movie.license] as [string, string]] : []),
            ...(st.lastPlayedAt ? [['Last watched', new Date(st.lastPlayedAt).toLocaleDateString('en-US', {month: 'long', day: 'numeric'})] as [string, string]] : []),
          ]} />
        </div>
      </div>
    </Page>
  );
}

function ChaptersShelf({movie}: {movie: Movie}) {
  return (
    <Shelf id="chapters" title="Chapters">
      {movie.chapters.map((c, i) => (
        <div key={i} className="card-cell cell-landscape cell-chapter">
          <button type="button" className="card card-landscape" onClick={() => player.play(movie.id, {kind: 'video', from: c.start})} aria-label={`Play from chapter ${i + 1}: ${c.title}, ${formatClock(c.start)}`}>
            <span className="card-frame"><Art src={c.art ?? movie.art.backdrop} tint={movie.tint} /><span className="extra-time">{formatClock(c.start)}</span></span>
            <span className="card-caption"><span className="card-title">{i + 1}. {c.title}</span></span>
          </button>
        </div>
      ))}
    </Shelf>
  );
}

// ---- Music

function AlbumView({album}: {album: Album}) {
  const services = useServices();
  useDataVersion();
  const tracks = useQuery(`tracks.${album.id}`, () => services.catalog.items(album.trackIds));
  const artist = useQuery(`item.${album.artistId}`, () => services.catalog.item(album.artistId));
  const more = useQuery(`albums.by.${album.artistId}`, async () => {
    const a = await services.catalog.item(album.artistId);
    return a?.kind === 'artist' ? services.catalog.items(a.albumIds.filter(id => id !== album.id)) : [];
  });
  const now = usePlayer(s => s.itemId);
  const st = services.user.state(album.id);
  const menu = useEntryMenu(album, 'detail');
  const list = (tracks.data ?? []) as TrackItem[];
  const discs = [...new Set(list.map(t => t.disc))];
  return (
    <Page flush className="title-page music-page">
      <Stage art={album.art.square} tint={album.tint} blur className="stage-music">
        <div className="music-head">
          <Art src={album.art.square} tint={album.tint} className="music-cover" ratio="1/1" eager />
          <div className="music-info">
            <div className="eyebrow stage-eyebrow">{album.albumType}</div>
            <h1 className="display stage-title is-music">{album.title}</h1>
            <p className="music-by"><Link to="/media/$itemId" params={{itemId: album.artistId}} className="credit-link">{album.artistName}</Link></p>
            <Facts items={[album.year, album.genres[0], plural(album.trackIds.length, 'song'), formatRuntime(album.duration / 60), album.label]} />
            <div className="action-bar">
              <div className="action-primary">
                <Button variant="primary" size="lg" icon="play" label="Play" onClick={() => void startPlayback(services, album)} />
                <Button variant="glass" size="lg" icon="shuffle" label="Shuffle" onClick={() => void startPlayback(services, album, {shuffle: true})} />
              </div>
              <div className="action-secondary">
                <IconButton icon="heart" label={st.favorite ? 'Remove from Favorites' : 'Add to Favorites'} active={st.favorite} onClick={() => void services.user.set([album.id], {favorite: !st.favorite}).then(() => toast(st.favorite ? 'Removed from Favorites' : 'Added to Favorites'))} />
                <IconButton icon="listPlus" label="Add to queue" onClick={() => { player.enqueue(album.trackIds); toast(`Added “${album.title}” to the queue`, {icon: 'queue'}); }} />
                <Menu label="More actions" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="More actions"><Icon name="more" size={19} /></button>} items={menu} />
              </div>
            </div>
          </div>
        </div>
      </Stage>
      <div className="page-pad">
        <div className="tracklist" role="table" aria-label={`${album.title} songs`}>
          {discs.map(d => (
            <React.Fragment key={d}>
              {discs.length > 1 ? <div className="tracklist-disc eyebrow" role="row"><span role="cell">Disc {d}</span></div> : null}
              {list.filter(t => t.disc === d).map(t => <TrackRow key={t.id} track={t} playing={now === t.id} onPlay={() => player.play(album.trackIds, {kind: 'audio', index: album.trackIds.indexOf(t.id)})} />)}
            </React.Fragment>
          ))}
          {tracks.loading && !list.length ? Array.from({length: 8}, (_, i) => <div key={i} className="track-row is-skeleton"><span className="skeleton" style={{height: 14, width: '40%'}} /></div>) : null}
        </div>
        <p className="faint small" style={{marginTop: 16}}>{album.year} · {album.label}</p>
      </div>
      {more.data?.length ? <Shelf id="more-by" title={`More by`} subject={artist.data?.title ?? album.artistName}>{more.data.map(a => <MediaCard key={a.id} item={a} />)}</Shelf> : null}
      <RelatedShelf id={album.id} title="You might also like" />
    </Page>
  );
}

export function TrackRow({track, playing, onPlay, index, showArt, showAlbum, trailing}: {track: TrackItem; playing?: boolean; onPlay: () => void; index?: number; showArt?: boolean; showAlbum?: boolean; trailing?: React.ReactNode}) {
  const services = useServices();
  const navigate = useNavigate();
  const st = services.user.state(track.id);
  const menu: Parameters<typeof Menu>[0]['items'] = [
    {id: 'next', label: 'Play next', icon: 'next', onSelect: () => { player.enqueue([track.id], true); toast(`“${track.title}” plays next`, {icon: 'queue'}); }},
    {id: 'queue', label: 'Add to queue', icon: 'listPlus', onSelect: () => { player.enqueue([track.id]); toast(`Added “${track.title}” to the queue`, {icon: 'queue'}); }},
    {id: 'playlist', label: 'Add to playlist…', icon: 'playlist', onSelect: () => actions.addToPlaylist([track.id])},
    {id: 'fav', label: st.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: 'heart', separatorBefore: true, onSelect: () => void services.user.set([track.id], {favorite: !st.favorite})},
    {id: 'album', label: 'Go to album', icon: 'disc', onSelect: () => void navigate({to: '/media/$itemId', params: {itemId: track.albumId}})},
    {id: 'artist', label: 'Go to artist', icon: 'mic', onSelect: () => void navigate({to: '/media/$itemId', params: {itemId: track.artistId}})},
    {id: 'report', label: 'Report a problem…', icon: 'flag', separatorBefore: true, onSelect: () => actions.feedback(track.id)},
  ];
  return (
    <div className={cx('track-row', playing && 'is-playing')} role="row">
      <button type="button" className="track-main" onClick={onPlay} aria-label={`Play ${track.title}`}>
        <span className="track-num" role="cell">{playing ? <span className="eq" aria-label="Playing"><i /><i /><i /></span> : <><span className="track-n">{index ?? track.number}</span><Icon name="play" size={15} fill="currentColor" className="track-play" /></>}</span>
        {showArt ? <Art src={track.art.square} tint={track.tint} className="track-art" ratio="1/1" /> : null}
        <span className="track-text" role="cell"><span className="track-title">{track.title}</span>{showAlbum ? <span className="track-sub">{track.artistName} · {track.albumTitle}</span> : null}</span>
      </button>
      {track.hasLyrics ? <span className="track-lyrics" title="Lyrics available" role="cell"><Icon name="mic" size={14} /><span className="sr-only">Lyrics</span></span> : <span role="cell" />}
      {st.favorite ? <Icon name="heart" size={14} fill="currentColor" className="track-fav" /> : null}
      <span className="track-time tabular" role="cell">{formatClock(track.duration)}</span>
      {trailing}
      <Menu label={`More for ${track.title}`} trigger={<button type="button" className="track-more" aria-label={`More for ${track.title}`}><Icon name="more" size={18} /></button>} items={menu} />
    </div>
  );
}

function ArtistView({artist}: {artist: Artist}) {
  const services = useServices();
  const albums = useQuery(`albums.${artist.id}`, () => services.catalog.items(artist.albumIds));
  const list = (albums.data ?? []) as Album[];
  const top = useQuery(list.length ? `top.${artist.id}` : null, () => services.catalog.items(list.flatMap(a => a.trackIds.slice(0, 2)).slice(0, 6)));
  const now = usePlayer(s => s.itemId);
  const st = services.user.state(artist.id);
  const menu = useEntryMenu(artist, 'detail');
  const main = list.filter(a => a.albumType === 'Album');
  const other = list.filter(a => a.albumType !== 'Album');
  return (
    <Page flush className="title-page artist-page">
      <Stage art={artist.art.square} tint={artist.tint} blur className="stage-music">
        <div className="music-head is-artist">
          <Art src={artist.art.square} tint={artist.tint} className="artist-photo" ratio="1/1" eager fallbackIcon="mic" />
          <div className="music-info">
            <div className="eyebrow stage-eyebrow">{artist.band ? 'Band' : 'Artist'} · {artist.genres.slice(0, 2).join(' · ')}</div>
            <h1 className="display stage-title is-music">{artist.title}</h1>
            <Facts items={[artist.origin, artist.band ? `Formed ${artist.formed}` : `Active since ${artist.formed}`, plural(artist.albumIds.length, 'release')]} />
            <ReadMore text={artist.bio} lines={2} className="stage-syn" />
            <div className="action-bar">
              <div className="action-primary">
                <Button variant="primary" size="lg" icon="play" label="Play" onClick={() => void startPlayback(services, artist)} />
                <Button variant="glass" size="lg" icon="shuffle" label="Shuffle" onClick={() => void startPlayback(services, artist, {shuffle: true})} />
              </div>
              <div className="action-secondary">
                <IconButton icon="heart" label={st.favorite ? 'Remove from Favorites' : 'Add to Favorites'} active={st.favorite} onClick={() => void services.user.set([artist.id], {favorite: !st.favorite})} />
                <Menu label="More actions" trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="More actions"><Icon name="more" size={19} /></button>} items={menu} />
              </div>
            </div>
          </div>
        </div>
      </Stage>
      <div className="page-pad">
        {top.data?.length ? (
          <section className="section">
            <h2 className="shelf-title" style={{marginBottom: 12}}>Top songs</h2>
            <div className="tracklist is-cols" role="table" aria-label="Top songs">
              {(top.data as TrackItem[]).map((t, i) => <TrackRow key={t.id} track={t} index={i + 1} showArt playing={now === t.id} onPlay={() => player.play(top.data!.map(x => x.id), {kind: 'audio', index: i})} />)}
            </div>
          </section>
        ) : null}
      </div>
      {main.length ? <Shelf id="albums" title="Albums">{main.map(a => <MediaCard key={a.id} item={a} subtitle={String(a.year)} />)}</Shelf> : null}
      {other.length ? <Shelf id="singles" title="Singles & EPs">{other.map(a => <MediaCard key={a.id} item={a} subtitle={`${a.albumType} · ${a.year}`} />)}</Shelf> : null}
      <RelatedShelf id={artist.id} title="Similar artists" />
    </Page>
  );
}

// ---- Audiobooks

function AudiobookView({book}: {book: Audiobook}) {
  const services = useServices();
  useDataVersion();
  const st = services.user.state(book.id);
  const series = useQuery(book.series ? `entity.series.${book.series.name}` : null, () => services.catalog.entity(book.libraryId, 'series', `series:${book.series!.name}`));
  const byAuthor = useQuery(`entity.author.${book.author}`, () => services.catalog.entity(book.libraryId, 'author', `author:${book.author}`));
  const seriesItems = useQuery(series.data ? `items.series.${book.id}` : null, () => services.catalog.items(series.data!.itemIds.filter(id => id !== book.id)));
  const authorItems = useQuery(byAuthor.data ? `items.author.${book.id}` : null, () => services.catalog.items(byAuthor.data!.itemIds.filter(id => id !== book.id)));
  const nowId = usePlayer(s => s.itemId);
  const position = usePlayer(s => (s.itemId === book.id ? s.position : null));
  const pos = position ?? st.position;
  const current = [...book.chapters].reverse().find(c => c.start <= pos) ?? book.chapters[0];
  const currentIndex = current ? book.chapters.indexOf(current) : 0;
  const inProgress = st.progress > 0 && !st.watched;
  return (
    <Page flush className="title-page book-page">
      <Stage art={book.art.poster} tint={book.tint} blur className="stage-music">
        <div className="music-head is-book">
          <Art src={book.art.poster} tint={book.tint} className="book-cover" ratio="2/3" eager fallbackIcon="bookOpen" />
          <div className="music-info">
            <div className="eyebrow stage-eyebrow">Audiobook{book.series ? ` · ${book.series.name}, book ${book.series.number}` : ''}</div>
            <h1 className="display stage-title is-music">{book.title}</h1>
            <p className="music-by">By <Link to="/library/$libraryId/$view/$entityId" params={{libraryId: book.libraryId, view: 'author', entityId: `author:${book.author}`}} className="credit-link">{book.author}</Link> · Read by {book.narrator}</p>
            <Facts items={[book.year, formatRuntime(book.duration / 60), plural(book.chapters.length, 'chapter'), book.genres[0]]} badges={book.publicDomain ? <Badge>Public domain</Badge> : null} />
            <ReadMore text={book.synopsis} lines={3} className="stage-syn" />
            <ActionBar item={book} playLabel={inProgress ? `Resume chapter ${currentIndex + 1}` : 'Listen'} remaining={book.duration - st.position} />
            {inProgress ? <p className="resume-line"><span className="resume-bar"><span style={{width: `${st.progress * 100}%`}} /></span>{formatRemaining(book.duration - st.position)} in the book</p> : null}
          </div>
        </div>
      </Stage>
      <div className="page-pad">
        <section className="section">
          <div className="section-head"><h2 className="shelf-title">Chapters</h2><span className="faint small">{formatRuntime(book.duration / 60)} in all</span></div>
          <ol className="chapter-list">
            {book.chapters.map((c, i) => {
              const done = pos >= c.start + c.duration;
              const here = i === currentIndex && (inProgress || nowId === book.id);
              return (
                <li key={i}>
                  <button type="button" className={cx('chapter-row', here && 'is-current', done && 'is-done')} onClick={() => player.play(book.id, {kind: 'book', from: c.start})} aria-label={`Play chapter ${i + 1}: ${c.title}`}>
                    <span className="chapter-n tabular">{here ? <Icon name="play" size={14} fill="currentColor" /> : done ? <Icon name="check" size={14} /> : i + 1}</span>
                    <span className="chapter-t">{c.title}{here ? <span className="chapter-here">{nowId === book.id ? 'Playing' : 'You’re here'}</span> : null}</span>
                    <span className="chapter-d tabular">{formatClock(c.duration)}</span>
                  </button>
                </li>
              );
            })}
          </ol>
        </section>
      </div>
      {seriesItems.data?.length ? <Shelf id="series" title="More in" subject={book.series?.name}>{seriesItems.data.map(i => <MediaCard key={i.id} item={i} />)}</Shelf> : null}
      {authorItems.data?.length ? <Shelf id="author" title="More by" subject={book.author}>{authorItems.data.map(i => <MediaCard key={i.id} item={i} />)}</Shelf> : null}
      <div className="page-pad">
        <section className="details">
          <h2 className="shelf-title">About the author</h2>
          <p className="muted" style={{maxWidth: 720, marginTop: 10}}>{book.authorBio || `${book.author} has ${plural((byAuthor.data?.itemIds.length ?? 1), 'book')} in this library.`}</p>
        </section>
      </div>
    </Page>
  );
}

export type {Item};
