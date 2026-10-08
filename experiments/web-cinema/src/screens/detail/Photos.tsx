import {useEffect, useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Photo, PhotoAlbum} from '../../data/types';
import {Page} from '../../ui/Layout';
import {Art} from '../../ui/Media';
import {Button, IconButton} from '../../ui/Button';
import {Icon} from '../../ui/Icon';
import {Skeleton, toast} from '../../ui/Feedback';
import {cx} from '../../ui/cx';
import {reducedMotion} from '../../app/prefs';
import {Facts, Stage} from './parts';
import {actions} from '../shared/actions';

const SLIDE_MS = 5000;

/** A photo album: a justified grid, a full-screen viewer with arrow keys, and a slideshow. */
export function PhotoAlbumView({album}: {album: PhotoAlbum}) {
  const services = useServices();
  const navigate = useNavigate();
  const search = useSearch({from: '/app/media/$itemId'});
  const photos = useQuery(`photos.${album.id}`, () => services.catalog.items(album.photoIds));
  const list = (photos.data ?? []) as Photo[];
  const open = (id: string | undefined, slideshow = false) => void navigate({to: '/media/$itemId', params: {itemId: album.id}, search: id ? {photo: id, slideshow: slideshow || undefined} : {}, replace: !!search.photo});
  const range = album.dateStart === album.dateEnd ? fmt(album.dateStart) : `${fmt(album.dateStart)} – ${fmt(album.dateEnd)}`;
  return (
    <Page flush className="title-page photos-page">
      <Stage art={album.art.backdrop} tint={album.tint} className="stage-short">
        <div className="eyebrow stage-eyebrow">Photo album</div>
        <h1 className="display stage-title">{album.title}</h1>
        <Facts items={[range, album.place, `${album.photoIds.length} photos`]} />
        <div className="action-bar">
          <div className="action-primary">
            <Button variant="primary" size="lg" icon="play" label="Slideshow" onClick={() => open(album.photoIds[0], true)} />
          </div>
          <div className="action-secondary">
            <IconButton icon="share" label="Copy link" onClick={() => { void navigator.clipboard?.writeText(location.href).catch(() => undefined); toast('Link copied'); }} />
            <IconButton icon="download" label="Download album" onClick={() => actions.download([album.id])} />
          </div>
        </div>
      </Stage>
      <div className="page-pad">
        <div className="photo-grid">
          {list.length ? list.map(p => (
            <button key={p.id} type="button" className={cx('photo-cell', p.height > p.width && 'is-tall')} onClick={() => open(p.id)} aria-label={`Open photo: ${p.caption || p.title}`}>
              <Art src={p.art.thumb} tint={p.tint} fallbackIcon="image" />
            </button>
          )) : Array.from({length: 12}, (_, i) => <Skeleton key={i} className="photo-cell" />)}
        </div>
      </div>
      {search.photo ? <PhotoViewer photos={list} id={search.photo} slideshow={!!search.slideshow} onChange={(id, s) => open(id, s)} onClose={() => open(undefined)} album={album} /> : null}
    </Page>
  );
}

const fmt = (d: string) => new Date(d).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'});

function PhotoViewer({photos, id, slideshow, onChange, onClose, album}: {photos: Photo[]; id: string; slideshow: boolean; onChange: (id: string, slideshow: boolean) => void; onClose: () => void; album: PhotoAlbum}) {
  const i = Math.max(0, photos.findIndex(p => p.id === id));
  const photo = photos[i];
  const [info, setInfo] = useState(false);
  const [chrome, setChrome] = useState(true);
  const go = (d: number) => photos.length && onChange(photos[(i + d + photos.length) % photos.length]!.id, slideshow);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight') go(1);
      else if (e.key === 'ArrowLeft') go(-1);
      else if (e.key === 'Escape') onClose();
      else if (e.key === ' ') { e.preventDefault(); onChange(id, !slideshow); }
      else if (e.key === 'i') setInfo(v => !v);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
  useEffect(() => {
    if (!slideshow || reducedMotion()) return;
    const t = setTimeout(() => go(1), SLIDE_MS);
    return () => clearTimeout(t);
  }, [slideshow, id]);
  useEffect(() => {
    if (!slideshow) { setChrome(true); return; }
    const t = setTimeout(() => setChrome(false), 1800);
    return () => clearTimeout(t);
  }, [slideshow, id]);
  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  }, []);
  if (!photo) return null;
  return (
    <div className={cx('viewer', !chrome && 'is-quiet', slideshow && 'is-slideshow')} role="dialog" aria-modal="true" aria-label={`Photo ${i + 1} of ${photos.length}`} onMouseMove={() => setChrome(true)}>
      <div className="viewer-stage">
        <Art key={photo.id} src={photo.art.backdrop} tint={photo.tint} className={cx('viewer-img', slideshow && 'is-kb')} eager fallbackIcon="image" />
      </div>
      <div className="viewer-top">
        <div className="viewer-title"><strong>{album.title}</strong><span>{i + 1} of {photos.length}</span></div>
        <div className="viewer-tools">
          <IconButton icon={slideshow ? 'pause' : 'play'} label={slideshow ? 'Pause slideshow' : 'Play slideshow'} onClick={() => onChange(id, !slideshow)} />
          <IconButton icon="info" label="Photo info" active={info} onClick={() => setInfo(v => !v)} />
          <IconButton icon="download" label="Download photo" onClick={() => toast('Downloading the original photo')} />
          <IconButton icon="close" label="Close" onClick={onClose} />
        </div>
      </div>
      <button type="button" className="viewer-nav is-prev" onClick={() => go(-1)} aria-label="Previous photo"><Icon name="chevronLeft" size={30} /></button>
      <button type="button" className="viewer-nav is-next" onClick={() => go(1)} aria-label="Next photo"><Icon name="chevronRight" size={30} /></button>
      {photo.caption ? <p className="viewer-caption">{photo.caption}</p> : null}
      {slideshow ? <div className="viewer-progress" key={photo.id}><span style={{animationDuration: `${SLIDE_MS}ms`}} /></div> : null}
      {info ? (
        <aside className="viewer-info" aria-label="Photo information">
          <h2 className="panel-title">Photo info</h2>
          <dl className="kv">
            <div className="kv-row"><dt>Taken</dt><dd>{new Date(photo.taken).toLocaleString('en-US', {dateStyle: 'long', timeStyle: 'short'})}</dd></div>
            <div className="kv-row"><dt>Place</dt><dd>{photo.place || '—'}</dd></div>
            <div className="kv-row"><dt>Size</dt><dd>{photo.width} × {photo.height}</dd></div>
            <div className="kv-row"><dt>Camera</dt><dd>iPhone 15 Pro · 24 mm · ƒ/1.78</dd></div>
            <div className="kv-row"><dt>Album</dt><dd>{album.title}</dd></div>
          </dl>
        </aside>
      ) : null}
    </div>
  );
}
