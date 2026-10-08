import { useRef } from 'react';
import { Link } from 'react-router';
import { Play, Shuffle } from 'lucide-react';
import { useWindowVirtualizer } from '@tanstack/react-virtual';
import { useAlbums, useArtists, useLibraries, useMusicHome, usePlaylists, useSongs } from '../../services/queries';
import { api } from '../../services/client';
import { usePlayer } from '../../state/player';
import { PageHeader } from '../../components/PageHeader';
import { MediaCard, Shelf, ShelfSkeleton } from '../../components/media';
import { Button, ErrorState, Segmented, Skel } from '../../components/ui';
import { useViewParam } from '../library/Movies';
import { Mosaic, TrackRow } from './common';
import { tracks as allTracks } from '../../data/fixtures/index';
import { plural, useIsMobile } from '../../lib/hooks';

const VIEWS = [
  { value: 'home', label: 'Listen Now' },
  { value: 'artists', label: 'Artists' },
  { value: 'albums', label: 'Albums' },
  { value: 'songs', label: 'Songs' },
  { value: 'playlists', label: 'Playlists' },
];

export function MusicPage() {
  const [view, setView] = useViewParam('home');
  const mobile = useIsMobile();
  const lib = useLibraries().data?.find((l) => l.id === 'lib-music');
  const p = usePlayer.getState();
  const shuffleAll = async () => {
    const list = await api.getTracks(allTracks.map((t) => t.id));
    p.playTracks(list, 0, { shuffle: true });
  };
  return (
    <div className="page">
      <PageHeader
        title="Music"
        subtitle={lib ? `${lib.countLabel} · Lossless where available` : undefined}
        actions={
          <>
            <Segmented value={view} onChange={setView} options={VIEWS} label="View" />
            <Button variant="glass" icon={<Shuffle />} onClick={shuffleAll}>
              Shuffle All
            </Button>
          </>
        }
      >
        <div className="m-seg music-seg no-scrollbar">
          <Segmented value={view} onChange={setView} options={mobile ? VIEWS.map((v) => ({ ...v, label: v.value === 'home' ? 'Now' : v.label })) : VIEWS} label="View" />
        </div>
      </PageHeader>
      <div className="page-body">
        {view === 'home' && <MusicHome />}
        {view === 'artists' && <Artists />}
        {view === 'albums' && <Albums />}
        {view === 'songs' && <Songs />}
        {view === 'playlists' && <Playlists />}
      </div>
    </div>
  );
}

function MusicHome() {
  const q = useMusicHome();
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data)
    return (
      <>
        <ShelfSkeleton style="square" />
        <ShelfSkeleton style="square" />
      </>
    );
  const top = q.data.albums[0];
  return (
    <>
      <div className="pad">
        <div className="music-feature" style={{ ['--tint' as string]: top.palette.base, ['--glow' as string]: top.palette.accent }}>
          <img className="mf-bg" src={top.art.square} alt="" />
          <img className="mf-art" src={top.art.square} alt="" />
          <div className="mf-text">
            <span className="t-eyebrow">New in your library</span>
            <h2 className="mf-title">{top.title}</h2>
            <Link to={`/artist/${top.artistId}`} className="mf-artist">
              {top.artistName}
            </Link>
            <p className="mf-review clamp-3">{top.review ?? `${top.format} · ${top.year} · ${top.genre}`}</p>
            <div className="mf-actions">
              <Button variant="primary" icon={<Play fill="currentColor" />} onClick={() => usePlayer.getState().playTracks(top.tracks, 0)}>
                Play
              </Button>
              <Button variant="glass" to={`/album/${top.id}`}>
                View Album
              </Button>
            </div>
          </div>
        </div>
      </div>
      {q.data.shelves.map((s) => (
        <Shelf key={s.id} shelf={s} />
      ))}
    </>
  );
}

function Artists() {
  const q = useArtists();
  return (
    <div className="pad square-grid circles">
      {q.data
        ? q.data.map((a) => <MediaCard key={a.id} shape="circle" item={{ id: a.id, kind: 'artist', title: a.title, subtitle: a.genres.join(', '), image: a.art.square!, href: `/artist/${a.id}`, palette: a.palette }} />)
        : Array.from({ length: 12 }, (_, i) => (
            <div key={i}>
              <Skel r={999} style={{ aspectRatio: '1' }} />
            </div>
          ))}
    </div>
  );
}

function Albums() {
  const q = useAlbums('artist');
  return (
    <div className="pad square-grid">
      {q.data
        ? q.data.map((a) => <MediaCard key={a.id} shape="square" item={{ id: a.id, kind: 'album', title: a.title, subtitle: `${a.artistName} · ${a.year}`, image: a.art.square!, href: `/album/${a.id}`, palette: a.palette }} />)
        : Array.from({ length: 12 }, (_, i) => (
            <div key={i}>
              <Skel style={{ aspectRatio: '1' }} />
            </div>
          ))}
    </div>
  );
}

function Songs() {
  const q = useSongs(0, 5000, 'title');
  const ref = useRef<HTMLDivElement>(null);
  const list = q.data?.items ?? [];
  const v = useWindowVirtualizer({ count: list.length, estimateSize: () => 56, overscan: 10, scrollMargin: ref.current?.offsetTop ?? 0 });
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  return (
    <div className="pad">
      <div className="songs-head">
        <span className="tnum">{q.data ? `${q.data.total} songs` : ' '}</span>
        <Button size="sm" variant="plain" icon={<Shuffle />} disabled={!list.length} onClick={() => usePlayer.getState().playTracks(list, 0, { shuffle: true })}>
          Shuffle
        </Button>
      </div>
      <div className="tlist-head with-album">
        <span>Title</span>
        <span>Album</span>
        <span>Time</span>
      </div>
      <div ref={ref} style={{ height: v.getTotalSize(), position: 'relative' }}>
        {!q.data &&
          Array.from({ length: 12 }, (_, i) => (
            <div key={i} className="trow">
              <Skel w={40} h={40} r={6} />
              <Skel w="40%" h={12} />
            </div>
          ))}
        {v.getVirtualItems().map((r) => (
          <div key={r.key} style={{ position: 'absolute', top: 0, left: 0, right: 0, transform: `translateY(${r.start - v.options.scrollMargin}px)` }}>
            <TrackRow t={list[r.index]} index={r.index} list={list} showArt showAlbum />
          </div>
        ))}
      </div>
    </div>
  );
}

function Playlists() {
  const q = usePlaylists();
  return (
    <div className="pad square-grid">
      {(q.data ?? []).map((p) => (
        <Link key={p.id} to={`/playlist/${p.id}`} className="card card-square pl-card" viewTransition>
          <span className="art card-art pl-art">
            <Mosaic images={p.trackIds.map((id) => allTracks.find((t) => t.id === id)?.art.square ?? '')} />
          </span>
          <span className="card-text">
            <span className="card-title ellipsis">{p.title}</span>
            <span className="card-sub ellipsis">{p.smart ? 'Smart playlist' : plural(p.trackIds.length, 'song')}</span>
          </span>
        </Link>
      ))}
    </div>
  );
}
