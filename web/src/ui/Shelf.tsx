import React, {useCallback, useEffect, useRef, useState} from 'react';
import {scaled} from './Grid';
import {uiI18n} from './i18n';
import {cx} from './cx';
import {Icon} from './Icon';
import {Button} from './Button';
import s from './Shelf.module.css';

export type ShelfDensity = 'poster' | 'square' | 'landscape' | 'person' | 'compact';
const widths: Record<ShelfDensity, string> = {poster: scaled('clamp(124px, 12.5vw, 176px)'), square: scaled('clamp(140px, 14vw, 200px)'), landscape: scaled('clamp(220px, 22vw, 320px)'), person: scaled('clamp(112px, 11vw, 150px)'), compact: scaled('clamp(100px, 10vw, 132px)')};

/**
 * Horizontal shelf. Header with optional trailing action, a snapping track,
 * and pointer-only paddles that appear on hover. Keyboard users tab through
 * cards and the track scrolls to keep focus visible.
 */
export function Shelf({title, count, action, density = 'poster', children, className, id, onEndReached}: {title?: string; count?: number; action?: {label: string; ariaLabel?: string; onClick: () => void}; density?: ShelfDensity; children: React.ReactNode; className?: string; id?: string; /** Called when the row's end is in view (again after more cards arrive), to load the next page. */ onEndReached?: () => void}) {
  const track = useRef<HTMLDivElement>(null);
  const [edge, setEdge] = useState({start: true, end: false});
  const update = useCallback(() => {
    const el = track.current;
    if (!el) return;
    const start = el.scrollLeft <= 4, end = el.scrollLeft + el.clientWidth >= el.scrollWidth - 4;
    // PERF-28: scroll events fire per frame; only a change at either end re-renders.
    setEdge(prev => (prev.start === start && prev.end === end ? prev : {start, end}));
  }, []);
  useEffect(() => {
    const el = track.current;
    if (!el) return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [update]);
  useEffect(update, [update, children]);
  const endReached = useRef(onEndReached);
  endReached.current = onEndReached;
  useEffect(() => { if (edge.end) endReached.current?.(); }, [edge.end, children]);
  // WEB-SYS-09: one Tab stop per row (the card focused last); arrows move along the row.
  const active = useRef(0);
  const primaries = () => [...(track.current?.children ?? [])].map(child => child.querySelector<HTMLElement>('button, a[href]')).filter((el): el is HTMLElement => !!el);
  useEffect(() => {
    const items = primaries();
    const current = Math.min(active.current, Math.max(0, items.length - 1));
    items.forEach((el, i) => { el.tabIndex = i === current ? 0 : -1; });
  });
  const onFocus = (e: React.FocusEvent<HTMLDivElement>) => {
    const items = primaries();
    const i = items.findIndex(el => el === e.target || el.contains(e.target as Node));
    if (i < 0) return;
    active.current = i;
    items.forEach((el, k) => { el.tabIndex = k === i ? 0 : -1; });
  };
  const onKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;
    const items = primaries();
    const at = items.findIndex(el => el === document.activeElement);
    if (at < 0) return;
    const to = e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : Math.max(0, Math.min(items.length - 1, at + (e.key === 'ArrowRight' ? 1 : -1)));
    e.preventDefault();
    items[to]!.focus();
    items[to]!.scrollIntoView({block: 'nearest', inline: 'nearest'});
  };
  const page = (dir: 1 | -1) => {
    const el = track.current;
    if (!el) return;
    el.scrollBy({left: dir * Math.max(200, el.clientWidth * 0.85), behavior: 'smooth'});
  };
  return (
    <section className={cx(s.shelf, className)} id={id} aria-label={title}>
      {title ? (
        <header className={s.head}>
          <div className={s.headLeft}>
            <h2 className={s.title}>{title}</h2>
            {count != null ? <span className={s.count}>{count}</span> : null}
          </div>
          {action ? <div className={s.headRight}><Button variant="link" size="sm" label={action.label} aria-label={action.ariaLabel} iconAfter="forward" onClick={action.onClick} /></div> : null}
        </header>
      ) : null}
      <div className={s.scroller}>
        <div className={cx(s.arrow, s.arrowLeft)}><button type="button" className={s.arrowButton} disabled={edge.start} onClick={() => page(-1)} aria-label={uiI18n().t('shelf.scrollBack')} tabIndex={-1}><Icon name="back" /></button></div>
        <div ref={track} className={s.track} style={{'--card-width': widths[density]} as React.CSSProperties} onScroll={update} onFocus={onFocus} onKeyDown={onKeyDown}>
          {children}
        </div>
        <div className={cx(s.arrow, s.arrowRight)}><button type="button" className={s.arrowButton} disabled={edge.end} onClick={() => page(1)} aria-label={uiI18n().t('shelf.scrollForward')} tabIndex={-1}><Icon name="forward" /></button></div>
      </div>
    </section>
  );
}
