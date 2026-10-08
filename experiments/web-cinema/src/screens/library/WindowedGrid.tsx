import React, {useEffect, useLayoutEffect, useRef, useState} from 'react';
import type {Item, Shape} from '../../data/types';
import {CardSkeleton, MediaCard} from '../shared/Cards';

/**
 * A card grid that renders only the rows near the viewport (the page scrolls; this listens to the
 * window). Rows are sized from the column width, so jumping to a letter can scroll straight to its
 * row before its page has loaded.
 */
export function WindowedGrid({total, at, ensure, shape, jumpTo, onJumped, context}: {
  total: number;
  at: (i: number) => Item | undefined;
  ensure: (from: number, to: number) => void;
  shape: Shape;
  jumpTo?: number | null;
  onJumped?: () => void;
  context?: 'card' | 'recommendation';
}) {
  const box = useRef<HTMLDivElement>(null);
  const [layout, setLayout] = useState({cols: 6, rowH: 320, top: 0, gapX: 18});
  const [range, setRange] = useState({first: 0, last: 3});
  const minW = shape === 'poster' ? posterMin() : shape === 'landscape' ? 280 : 170;
  const ratio = shape === 'poster' ? 1.5 : shape === 'landscape' ? 9 / 16 : 1;

  useLayoutEffect(() => {
    const measure = () => {
      const el = box.current;
      if (!el) return;
      const w = el.clientWidth;
      const phone = window.innerWidth < 768;
      const gapX = phone ? 10 : 18;
      const gapY = phone ? 20 : 28;
      const cols = phone ? (shape === 'poster' ? (w < 340 ? 2 : 3) : shape === 'landscape' ? 1 : 2) : Math.max(2, Math.floor((w + gapX) / (minW + gapX)));
      const colW = (w - gapX * (cols - 1)) / cols;
      const caption = 46;
      setLayout({cols, rowH: colW * ratio + caption + gapY, top: el.getBoundingClientRect().top + window.scrollY, gapX});
    };
    measure();
    const ro = new ResizeObserver(measure);
    if (box.current) ro.observe(box.current);
    return () => ro.disconnect();
  }, [shape, minW, ratio]);

  useEffect(() => {
    const onScroll = () => {
      const y = window.scrollY - layout.top;
      const first = Math.max(0, Math.floor(y / layout.rowH) - 2);
      const last = Math.ceil((y + window.innerHeight) / layout.rowH) + 2;
      setRange(r => (r.first === first && r.last === last ? r : {first, last}));
    };
    onScroll();
    window.addEventListener('scroll', onScroll, {passive: true});
    window.addEventListener('resize', onScroll);
    return () => { window.removeEventListener('scroll', onScroll); window.removeEventListener('resize', onScroll); };
  }, [layout]);

  const rows = Math.ceil(total / layout.cols);
  const first = Math.min(range.first, Math.max(0, rows - 1));
  const last = Math.min(range.last, rows - 1);
  useEffect(() => { ensure(first * layout.cols, (last + 1) * layout.cols - 1); }, [first, last, layout.cols, ensure]);

  useEffect(() => {
    if (jumpTo === null || jumpTo === undefined) return;
    const row = Math.floor(jumpTo / layout.cols);
    const topbar = window.innerWidth < 768 ? 64 : 16;
    window.scrollTo({top: layout.top + row * layout.rowH - topbar - 80, behavior: 'smooth'});
    onJumped?.();
  }, [jumpTo, layout, onJumped]);

  const cells: React.ReactNode[] = [];
  for (let i = first * layout.cols; i < Math.min(total, (last + 1) * layout.cols); i++) {
    const item = at(i);
    cells.push(item ? <MediaCard key={item.id} item={item} fill context={context} /> : <CardSkeleton key={`s${i}`} shape={shape} fill />);
  }
  return (
    <div ref={box} className="wgrid" style={{height: rows * layout.rowH}} aria-rowcount={rows}>
      <div className="wgrid-window" style={{transform: `translateY(${first * layout.rowH}px)`, gridTemplateColumns: `repeat(${layout.cols}, minmax(0, 1fr))`, columnGap: layout.gapX, gridAutoRows: layout.rowH}}>
        {cells}
      </div>
    </div>
  );
}

function posterMin() {
  const size = document.documentElement.dataset.posterSize;
  const vw = window.innerWidth;
  const base = Math.min(196, Math.max(132, vw * 0.104));
  return size === 'compact' ? base * 0.82 : size === 'large' ? base * 1.18 : base;
}
