import React, {useCallback, useEffect, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery} from '@/data/DataProvider';
import type {PhotoItem} from '@/data/types';
import {plural} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {ErrorState} from '@/ui/Bits';
import {BackLink} from '@/ui/Page';
import {Icon} from '@/ui/Icon';
import {useTitle} from '@/ui/hooks';
import {PageLoading, useRouteSearch} from '../common';
import s from './Detail.module.css';

const taken = (iso: string) => { const d = new Date(iso); return Number.isNaN(d.getTime()) ? iso : `${d.toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'})} at ${d.toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}`; };

/** A photo album: the grid, and a full-screen viewer with a slideshow (arrow keys, Space, I for info, Esc). */
export function PhotoAlbumDetail({id}: {id: string}) {
  const q = useQuery(c => c.catalog.photoAlbum(id), [id]);
  const search = useRouteSearch<{photo?: string; slideshow?: string}>();
  const navigate = useNavigate();
  useTitle(q.data?.entry.title);
  if (q.error && !q.data) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <PageLoading />;
  const d = q.data;
  const open = (photo?: string, slideshow?: boolean) => void navigate({to: `/media/${id}` as never, search: {photo, slideshow: slideshow ? '1' : undefined} as never, replace: !!search.photo});
  const index = search.photo ? d.photos.findIndex(p => p.id === search.photo) : search.slideshow ? 0 : -1;
  const years = [...new Set(d.photos.map(p => p.taken.slice(-4)))];
  return (
    <div style={{padding: '28px 0 72px'}}>
      <header style={{padding: '0 var(--gutter)', marginBottom: 20, display: 'flex', alignItems: 'flex-end', gap: 16, flexWrap: 'wrap'}}>
        <div style={{flex: '1 1 300px'}}>
          <BackLink fallback="/library/lib-photos" label="Photos" />
          <h1 className="t-title">{d.entry.title}</h1>
          <p className="c-2" style={{marginTop: 6}}>{plural(d.photos.length, 'photo')} · {years.join('–')}</p>
        </div>
        <Button variant="primary" icon="play" label="Slideshow" onClick={() => open(d.photos[0]?.id, true)} />
      </header>
      <div className={s.photoGrid} role="group" aria-label="Photos">
        {d.photos.map(p => <button key={p.id} type="button" className={s.photoBtn} onClick={() => open(p.id)} aria-label={`${p.title}, ${taken(p.taken)}`}><img src={p.thumb} alt="" loading="lazy" /></button>)}
      </div>
      {index >= 0 ? <PhotoViewer photos={d.photos} start={index} slideshow={!!search.slideshow} title={d.entry.title} onChange={p => open(p.id, !!search.slideshow)} onClose={() => void navigate({to: `/media/${id}` as never, search: {} as never})} /> : null}
    </div>
  );
}

function PhotoViewer({photos, start, slideshow, title, onChange, onClose}: {photos: PhotoItem[]; start: number; slideshow: boolean; title: string; onChange: (p: PhotoItem) => void; onClose: () => void}) {
  const [i, setI] = useState(start);
  const [playing, setPlaying] = useState(slideshow);
  const [info, setInfo] = useState(false);
  const p = photos[i]!;
  const rootRef = React.useRef<HTMLDivElement>(null);
  useEffect(() => { rootRef.current?.focus({preventScroll: true}); }, []);
  const go = useCallback((d: number) => setI(x => (x + d + photos.length) % photos.length), [photos.length]);
  useEffect(() => { setI(start); }, [start]);
  useEffect(() => { if (photos[i] && photos[i]!.id !== photos[start]?.id) onChange(photos[i]!); }, [i]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (!playing) return; const t = setTimeout(() => go(1), 4500); return () => clearTimeout(t); }, [playing, i, go]);
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.key === 'ArrowRight') go(1); else if (e.key === 'ArrowLeft') go(-1);
      else if (e.key === ' ') { e.preventDefault(); setPlaying(v => !v); } else if (e.key === 'i') setInfo(v => !v);
      else if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', on);
    return () => window.removeEventListener('keydown', on);
  }, [go, onClose]);
  return (
    <div ref={rootRef} tabIndex={-1} className={s.viewer} role="dialog" aria-modal="true" aria-label={`${title}: ${p.title}`}>
      <div className={s.viewerStage}>
        <div className={s.viewerBar}>
          <IconButton variant="ghost" icon="close" label="Close (Esc)" onClick={onClose} />
          <div className={s.viewerTitle}><div className="t-strong">{p.title}</div><div className="t-caption c-2">{i + 1} of {photos.length} · {taken(p.taken)}</div></div>
          <Button variant="ghost" icon={playing ? 'pause' : 'play'} label={playing ? 'Pause slideshow' : 'Slideshow'} onClick={() => setPlaying(!playing)} />
          <IconButton variant="ghost" icon="info" label="Photo info (I)" pressed={info} onClick={() => setInfo(!info)} />
        </div>
        <button type="button" className={s.viewerNav} style={{left: 16}} onClick={() => go(-1)} aria-label="Previous photo"><Icon name="back" size={22} /></button>
        <img key={p.id} src={p.image} alt={p.title} />
        <button type="button" className={s.viewerNav} style={{right: 16}} onClick={() => go(1)} aria-label="Next photo"><Icon name="forward" size={22} /></button>
        {info ? (
          <div className={s.viewerInfo}>
            <div className="t-strong">{p.title}</div>
            <div className="c-2">Taken {taken(p.taken)}</div>
            <div className="c-2">{p.camera}</div>
            <div className="c-3">{p.portrait ? 'Portrait' : 'Landscape'} · in {title}</div>
          </div>
        ) : null}
      </div>
      <div className={s.filmstrip} aria-label="Photos in this album">
        {photos.map((x, n) => <button key={x.id} type="button" aria-current={n === i} aria-label={x.title} onClick={() => setI(n)} tabIndex={-1}><img src={x.thumb} alt="" loading="lazy" /></button>)}
      </div>
    </div>
  );
}
