import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Link} from '@tanstack/react-router';
import type {Entry, Shape} from '@/data/types';
import type {Origin} from '@/app/actions';
import {CardSkeleton, MediaCard} from './Card';
import {Icon} from './Icon';
import {useRoving} from './roving';
import s from './Shelf.module.css';

/**
 * A titled horizontal row of cards (Plex-style hub). Scroll buttons appear
 * with a pointer; the row is one Tab stop and the arrow keys move along it.
 */
export function Shelf({title, entries, shape, href, reason, origin, total, eager, onOpen, children, headExtra}: {title: string; entries?: Entry[]; shape?: Shape; href?: string; reason?: string; origin?: Origin; total?: number; eager?: boolean; onOpen?: (e: Entry) => void; children?: React.ReactNode; headExtra?: React.ReactNode}) {
  const track = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({start: true, end: false});
  const roving = useRoving(track, 'row', [entries?.length]);
  const update = useCallback(() => {
    const el = track.current;
    if (!el) return;
    setEdges({start: el.scrollLeft < 8, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 8});
  }, []);
  useEffect(() => { update(); }, [update, entries?.length]);
  const page = (dir: 1 | -1) => {
    const el = track.current;
    if (!el) return;
    el.scrollBy({left: dir * (el.clientWidth - 120), behavior: 'smooth'});
  };
  const id = React.useId();
  const loading = !entries && !children;
  const sh = shape ?? entries?.[0]?.shape ?? 'poster';
  return (
    <section className={s.shelf} aria-labelledby={id}>
      <div className={s.head}>
        <h2 className={s.title} id={id}>{href ? <Link to={href as never} className={s.titleLink}>{title}<Icon name="forward" size={16} /></Link> : title}</h2>
        {reason ? <span className={s.reason}>{reason}</span> : null}
        <span className={s.spacer} />
        {headExtra}
        {href ? <Link to={href as never} className={s.seeAll}>See all{total && total > (entries?.length ?? 0) ? ` ${total}` : ''}</Link> : null}
        <div className={s.nav}>
          <button type="button" className={s.navBtn} onClick={() => page(-1)} disabled={edges.start} aria-label={`Scroll ${title} back`} tabIndex={-1}><Icon name="back" size={16} /></button>
          <button type="button" className={s.navBtn} onClick={() => page(1)} disabled={edges.end} aria-label={`Scroll ${title} forward`} tabIndex={-1}><Icon name="forward" size={16} /></button>
        </div>
      </div>
      <div className={s.track} ref={track} onScroll={update} role="list" aria-label={title} {...roving}>
        {loading ? Array.from({length: 8}, (_, i) => <CardSkeleton key={i} shape={sh} />) : children ?? entries!.map((e, i) => <div role="listitem" key={e.id} style={{display: 'contents'}}><MediaCard entry={e} shape={shape} origin={origin ?? (e.recommendation ? 'recommendation' : undefined)} eager={eager && i < 8} onOpen={onOpen} /></div>)}
      </div>
    </section>
  );
}

/** A responsive grid of cards; columns follow the card size preference. */
export function CardGrid({entries, shape = 'poster', origin, loading, skeletons = 18, onOpen, subtitle}: {entries?: Entry[]; shape?: Shape; origin?: Origin; loading?: boolean; skeletons?: number; onOpen?: (e: Entry) => void; subtitle?: (e: Entry) => string | undefined}) {
  const ref = useRef<HTMLDivElement>(null);
  const roving = useRoving(ref, 'grid', [entries?.length]);
  return (
    <div className={s.grid} ref={ref} style={{'--col-w': `var(--${shape}-w)`} as React.CSSProperties} role="list" {...roving}>
      {loading && !entries ? Array.from({length: skeletons}, (_, i) => <CardSkeleton key={i} shape={shape} grid />) : entries?.map(e => <div role="listitem" key={e.id} style={{display: 'contents'}}><MediaCard entry={e} shape={shape} grid origin={origin} onOpen={onOpen} subtitle={subtitle?.(e)} /></div>)}
    </div>
  );
}
