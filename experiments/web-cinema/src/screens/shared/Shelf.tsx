import React, {useEffect, useRef, useState} from 'react';
import {Link} from '@tanstack/react-router';
import {Icon} from '../../ui/Icon';
import {cx} from '../../ui/cx';

/**
 * A horizontal row of cards. Paging arrows appear on hover at the edges; the arrow keys move focus
 * card to card; the first card lines up with the page's text edge.
 */
export function Shelf({title, subject, eyebrow, seeAll, children, className, id, count, variant}: {
  title: React.ReactNode;
  subject?: React.ReactNode;
  eyebrow?: React.ReactNode;
  seeAll?: {to?: string; params?: Record<string, string>; search?: Record<string, unknown>; label?: string; onClick?: () => void};
  children: React.ReactNode;
  className?: string;
  id?: string;
  count?: number;
  variant?: 'ranked';
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState({start: true, end: false});
  const update = () => {
    const el = scroller.current;
    if (!el) return;
    setEdges({start: el.scrollLeft < 8, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 8});
  };
  useEffect(() => {
    update();
    const el = scroller.current;
    if (!el) return;
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [children]);
  const page = (dir: 1 | -1) => {
    const el = scroller.current;
    if (!el) return;
    el.scrollBy({left: dir * (el.clientWidth - 120), behavior: 'smooth'});
  };
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    const cards = Array.from(scroller.current?.querySelectorAll<HTMLElement>(':scope > .card-cell > .card') ?? []);
    const i = cards.indexOf(document.activeElement as HTMLElement);
    if (i < 0) return;
    const next = cards[e.key === 'ArrowRight' ? i + 1 : i - 1];
    if (next) {
      e.preventDefault();
      next.focus();
      next.scrollIntoView({block: 'nearest', inline: 'nearest', behavior: 'smooth'});
    }
  };
  const headingId = id ? `${id}-h` : undefined;
  return (
    <section className={cx('shelf', variant && `shelf-${variant}`, className)} aria-labelledby={headingId}>
      <div className="shelf-head">
        <h2 className="shelf-title" id={headingId}>
          {eyebrow ? <span className="shelf-eyebrow">{eyebrow}</span> : null}
          <span>{title}{subject ? <> <em className="shelf-subject">{subject}</em></> : null}</span>
        </h2>
        {seeAll?.onClick ? (
          <button type="button" className="shelf-all" onClick={seeAll.onClick}>{seeAll.label ?? 'See all'}<Icon name="chevronRight" size={16} /></button>
        ) : seeAll?.to ? (
          <Link to={seeAll.to as never} params={seeAll.params as never} search={(seeAll.search ?? {}) as never} className="shelf-all">
            {seeAll.label ?? 'See all'}{count ? <span className="shelf-count">{count}</span> : null}<Icon name="chevronRight" size={16} />
          </Link>
        ) : null}
      </div>
      <div className="shelf-body">
        <button type="button" className="shelf-arrow is-prev" onClick={() => page(-1)} hidden={edges.start} aria-label="Scroll back" tabIndex={-1}><Icon name="chevronLeft" size={26} /></button>
        <div className="shelf-scroller" ref={scroller} onScroll={update} onKeyDown={onKey}>
          {React.Children.map(children, c => (c ? <>{c}</> : null))}
        </div>
        <button type="button" className="shelf-arrow is-next" onClick={() => page(1)} hidden={edges.end} aria-label="Scroll forward" tabIndex={-1}><Icon name="chevronRight" size={26} /></button>
      </div>
    </section>
  );
}
