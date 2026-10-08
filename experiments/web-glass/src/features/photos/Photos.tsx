import { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { Link, useSearchParams } from 'react-router';
import { Aperture, Calendar, Camera, ChevronLeft, ChevronRight, Download, Heart, Images, Info, MapPin, Pause, Play, Share2, X } from 'lucide-react';
import type { Photo } from '../../data/types';
import { useMemories, usePhotoAlbums, usePhotos } from '../../services/queries';
import { PageHeader } from '../../components/PageHeader';
import { Button, EmptyState, ErrorState, IconButton, Segmented, Skel, cls } from '../../components/ui';
import { toast } from '../../components/overlay';
import { useLockBody } from '../../lib/hooks';

export function PhotosPage() {
  const [params, setParams] = useSearchParams();
  const view = params.get('view') ?? 'library';
  const albumId = params.get('album');
  const memoryId = params.get('memory');
  const photoId = params.get('photo');
  const all = usePhotos();
  const album = usePhotos(albumId ?? undefined);
  const albums = usePhotoAlbums();
  const memories = useMemories();
  const [grid, setGrid] = useState<'months' | 'all'>('months');

  const set = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) v === null ? next.delete(k) : next.set(k, v);
    setParams(next);
  };
  const current = albumId ? album.data : all.data;
  const memory = memories.data?.find((m) => m.id === memoryId);
  const memoryPhotos = memory ? (all.data ?? []).filter((p) => memory.photoIds.includes(p.id)) : [];
  const viewerList = memory ? memoryPhotos : current ?? [];
  const openAlbum = albums.data?.find((a) => a.id === albumId);

  return (
    <div className="page photos-page">
      <PageHeader
        title={openAlbum ? openAlbum.title : 'Photos'}
        subtitle={openAlbum ? openAlbum.subtitle : all.data ? `${all.data.length} photos · ${albums.data?.length ?? 0} albums · Backed up from 3 devices` : undefined}
        back={openAlbum ? '/photos?view=albums' : undefined}
        actions={
          !openAlbum && (
            <Segmented
              value={view}
              onChange={(v) => set({ view: v === 'library' ? null : v, album: null })}
              options={[
                { value: 'library', label: 'Library' },
                { value: 'albums', label: 'Albums' },
                { value: 'memories', label: 'Memories' },
              ]}
            />
          )
        }
      >
        {!openAlbum && (
          <div className="m-seg">
            <Segmented
              block
              value={view}
              onChange={(v) => set({ view: v === 'library' ? null : v, album: null })}
              options={[
                { value: 'library', label: 'Library' },
                { value: 'albums', label: 'Albums' },
                { value: 'memories', label: 'Memories' },
              ]}
            />
          </div>
        )}
      </PageHeader>
      <div className="page-body pad">
        {all.isError && <ErrorState error={all.error} retry={() => all.refetch()} />}
        {(view === 'library' || openAlbum) && (
          <>
            {!openAlbum && (
              <div className="photo-tools">
                <Segmented size="sm" value={grid} onChange={setGrid} options={[{ value: 'months', label: 'Months' }, { value: 'all', label: 'All Photos' }]} />
                <Button size="sm" variant="plain" icon={<Play />} disabled={!all.data} onClick={() => all.data && set({ photo: all.data[0].id, slideshow: '1' })}>
                  Slideshow
                </Button>
              </div>
            )}
            {!current ? (
              <div className="photo-grid">
                {Array.from({ length: 24 }, (_, i) => (
                  <Skel key={i} style={{ aspectRatio: '1' }} r={2} />
                ))}
              </div>
            ) : current.length === 0 ? (
              <EmptyState icon={<Images />} title="No photos here yet" />
            ) : grid === 'months' && !openAlbum ? (
              <MonthGroups photos={current} onOpen={(id) => set({ photo: id })} />
            ) : (
              <div className="photo-grid">
                {current.map((p) => (
                  <Thumb key={p.id} p={p} onOpen={() => set({ photo: p.id })} />
                ))}
              </div>
            )}
          </>
        )}
        {view === 'albums' && !openAlbum && (
          <div className="album-grid">
            {(albums.data ?? []).map((a) => (
              <Link key={a.id} to={`/photos?view=albums&album=${a.id}`} className="palbum">
                <span className="palbum-img">
                  <img src={a.cover} alt="" loading="lazy" />
                  {a.kind === 'smart' && (
                    <span className="palbum-badge">
                      <Heart fill="currentColor" />
                    </span>
                  )}
                </span>
                <span className="card-title">{a.title}</span>
                <span className="card-sub">{a.subtitle}</span>
              </Link>
            ))}
          </div>
        )}
        {view === 'memories' && !openAlbum && (
          <div className="memory-grid">
            {(memories.data ?? []).map((m, i) => (
              <button key={m.id} className={cls('memory', i === 0 && 'big')} onClick={() => set({ memory: m.id, photo: m.photoIds[0], slideshow: '1' })}>
                <img src={m.cover} alt="" loading="lazy" />
                <span className="memory-shade" />
                <span className="memory-text">
                  <span className="memory-title">{m.title}</span>
                  <span className="memory-sub">{m.subtitle} · {m.photoIds.length === 1 ? '1 photo' : `${m.photoIds.length} photos`}</span>
                </span>
                <span className="memory-play glass glass-clear">
                  <Play fill="currentColor" />
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
      {photoId && viewerList.length > 0 && (
        <Lightbox
          photos={viewerList}
          id={photoId}
          slideshow={params.get('slideshow') === '1'}
          title={memory?.title ?? openAlbum?.title}
          onChange={(id) => set({ photo: id })}
          onClose={() => set({ photo: null, slideshow: null, memory: null })}
        />
      )}
    </div>
  );
}

function Thumb({ p, onOpen }: { p: Photo; onOpen: () => void }) {
  return (
    <button className="thumb" onClick={onOpen} aria-label={`${p.place ?? 'Photo'}, ${new Date(p.takenAt).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}`}>
      <img src={p.thumb} alt="" loading="lazy" decoding="async" style={{ background: p.palette.base }} />
      {p.favorite && <Heart className="thumb-fav" fill="currentColor" />}
    </button>
  );
}

function MonthGroups({ photos, onOpen }: { photos: Photo[]; onOpen: (id: string) => void }) {
  const groups = useMemo(() => {
    const m = new Map<string, Photo[]>();
    for (const p of photos) {
      const d = new Date(p.takenAt);
      const k = `${d.getFullYear()}-${d.getMonth()}`;
      if (!m.has(k)) m.set(k, []);
      m.get(k)!.push(p);
    }
    return [...m.values()];
  }, [photos]);
  return (
    <div className="months">
      {groups.map((g) => {
        const d = new Date(g[0].takenAt);
        const places = Array.from(new Set(g.map((p) => p.place))).slice(0, 2).join(' & ');
        return (
          <section key={g[0].id} className="month">
            <header className="month-head">
              <h2>
                {d.toLocaleDateString('en-US', { month: 'long' })} <span>{d.getFullYear()}</span>
              </h2>
              <span className="t-sub">
                {places} · {g.length} {g.length === 1 ? 'photo' : 'photos'}
              </span>
            </header>
            <div className="photo-grid">
              {g.map((p) => (
                <Thumb key={p.id} p={p} onOpen={() => onOpen(p.id)} />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function Lightbox({ photos, id, slideshow, title, onChange, onClose }: { photos: Photo[]; id: string; slideshow: boolean; title?: string; onChange: (id: string) => void; onClose: () => void }) {
  const i = Math.max(0, photos.findIndex((p) => p.id === id));
  const p = photos[i];
  const [info, setInfo] = useState(false);
  const [playing, setPlaying] = useState(slideshow);
  const [fav, setFav] = useState(!!p?.favorite);
  useLockBody(true);
  useEffect(() => setFav(!!p?.favorite), [p]);
  const go = useCallback((d: number) => onChange(photos[(i + d + photos.length) % photos.length].id), [i, photos, onChange]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight') go(1);
      if (e.key === 'ArrowLeft') go(-1);
      if (e.key === 'Escape') onClose();
      if (e.key === ' ') {
        e.preventDefault();
        setPlaying((x) => !x);
      }
      if (e.key === 'i') setInfo((x) => !x);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [go, onClose]);
  useEffect(() => {
    if (!playing) return;
    const t = setTimeout(() => go(1), 4200);
    return () => clearTimeout(t);
  }, [playing, go, i]);
  // swipe on touch
  const [x0, setX0] = useState<number | null>(null);
  if (!p) return null;
  return createPortal(
    <div
      className={cls('lightbox', playing && 'playing', info && 'with-info')}
      role="dialog"
      aria-label="Photo viewer"
      onTouchStart={(e) => setX0(e.touches[0].clientX)}
      onTouchEnd={(e) => {
        if (x0 === null) return;
        const dx = e.changedTouches[0].clientX - x0;
        if (Math.abs(dx) > 50) go(dx < 0 ? 1 : -1);
        setX0(null);
      }}
    >
      <img className="lb-bg" src={p.thumb} alt="" aria-hidden="true" />
      <div className="lb-stage">
        <img key={p.id} className={cls('lb-img', playing && 'kb')} src={p.src} alt={p.place ?? ''} />
      </div>
      <div className="lb-top">
        <IconButton label="Close" variant="clear" onClick={onClose}>
          <X />
        </IconButton>
        <div className="lb-title">
          <span>{title ?? p.place}</span>
          <span className="t-sub tnum">
            {new Date(p.takenAt).toLocaleDateString('en-US', { weekday: 'short', month: 'long', day: 'numeric', year: 'numeric' })} · {i + 1} of {photos.length}
          </span>
        </div>
        <div className="lb-actions">
          <IconButton label={playing ? 'Pause slideshow' : 'Play slideshow'} variant="clear" onClick={() => setPlaying(!playing)}>
            {playing ? <Pause /> : <Play />}
          </IconButton>
          <IconButton label={fav ? 'Unfavorite' : 'Favorite'} variant="clear" active={fav} onClick={() => setFav(!fav)}>
            <Heart fill={fav ? 'currentColor' : 'none'} />
          </IconButton>
          <IconButton label="Share" variant="clear" onClick={() => toast('Shared link created · expires in 7 days', <Share2 />)}>
            <Share2 />
          </IconButton>
          <IconButton label="Download original" variant="clear" onClick={() => toast('Downloading original (HEIC, 4.2 MB)', <Download />)}>
            <Download />
          </IconButton>
          <IconButton label="Info" variant="clear" active={info} onClick={() => setInfo(!info)}>
            <Info />
          </IconButton>
        </div>
      </div>
      <button className="lb-nav prev glass glass-clear" onClick={() => go(-1)} aria-label="Previous photo">
        <ChevronLeft />
      </button>
      <button className="lb-nav next glass glass-clear" onClick={() => go(1)} aria-label="Next photo">
        <ChevronRight />
      </button>
      {playing && (
        <div className="lb-progress" key={p.id}>
          <i />
        </div>
      )}
      {info && (
        <aside className="lb-info glass glass-thick">
          <h3>{p.title}</h3>
          <div className="lb-row">
            <Calendar /> {new Date(p.takenAt).toLocaleString('en-US', { dateStyle: 'full', timeStyle: 'short' })}
          </div>
          <div className="lb-row">
            <MapPin /> {p.place}
          </div>
          <div className="lb-row">
            <Camera /> {p.camera}
          </div>
          <div className="lb-row">
            <Aperture /> {p.lens}
          </div>
          <div className="lb-exif tnum">{p.exposure}</div>
          <div className="lb-exif">
            {p.width} × {p.height} · {(p.width * p.height / 1e6).toFixed(1)} MP
          </div>
        </aside>
      )}
      <div className="lb-strip no-scrollbar">
        {photos.slice(Math.max(0, i - 12), i + 13).map((x) => (
          <button key={x.id} className={cls('lb-strip-item', x.id === p.id && 'on')} onClick={() => onChange(x.id)} aria-label="Show photo">
            <img src={x.thumb} alt="" loading="lazy" />
          </button>
        ))}
      </div>
    </div>,
    document.body,
  );
}
