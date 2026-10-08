import React, {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import type {Entry, Shape} from '@/data/types';
import type {Origin} from '@/app/actions';
import {CardSkeleton, MediaCard} from './Card';
import {useRoving} from './roving';
import s from './Windowed.module.css';

export type PageSource = {key: string; total: number; fetch: (offset: number, limit: number) => Promise<Entry[]>};

const RATIO: Record<Shape, number> = {poster: 1.5, wide: 0.5625, square: 1, circle: 1};
const TEXT_H = 46;
const ROW_GAP = 24;

/** Window scroll position and viewport height, updated on scroll and resize. */
function useViewport() {
  const [v, setV] = useState(() => ({y: typeof window === 'undefined' ? 0 : window.scrollY, h: typeof window === 'undefined' ? 800 : window.innerHeight}));
  useEffect(() => {
    let raf = 0;
    const on = () => { cancelAnimationFrame(raf); raf = requestAnimationFrame(() => setV({y: window.scrollY, h: window.innerHeight})); };
    window.addEventListener('scroll', on, {passive: true});
    window.addEventListener('resize', on);
    return () => { cancelAnimationFrame(raf); window.removeEventListener('scroll', on); window.removeEventListener('resize', on); };
  }, []);
  return v;
}

/** Pages of entries fetched on demand and cached per query. */
function usePages(source: PageSource, pageSize: number) {
  const [pages, setPages] = useState<Map<number, Entry[]>>(new Map());
  const pending = useRef(new Set<number>());
  const keyRef = useRef(source.key);
  useEffect(() => { keyRef.current = source.key; pending.current.clear(); setPages(new Map()); }, [source.key]);
  const ensure = useCallback((first: number, last: number) => {
    for (let p = Math.floor(first / pageSize); p <= Math.floor(last / pageSize); p++) {
      if (pages.has(p) || pending.current.has(p) || p * pageSize >= source.total) continue;
      pending.current.add(p);
      const key = source.key;
      void source.fetch(p * pageSize, pageSize).then(list => {
        if (keyRef.current !== key) return;
        pending.current.delete(p);
        setPages(prev => new Map(prev).set(p, list));
      });
    }
  }, [pages, pageSize, source]);
  const at = useCallback((i: number) => pages.get(Math.floor(i / pageSize))?.[i % pageSize], [pages, pageSize]);
  return {ensure, at};
}

/**
 * A virtualised card grid over a paged source. Only the rows in (and just
 * around) the viewport are rendered or fetched, so a library of millions
 * costs the same as a library of hundreds. `jumpTo` scrolls to an index
 * (the alphabet rail).
 */
export function WindowedGrid({source, shape, origin, pageSize = 60, jumpRef}: {source: PageSource; shape: Shape; origin?: Origin; pageSize?: number; jumpRef?: React.MutableRefObject<((index: number) => void) | null>}) {
  const root = useRef<HTMLDivElement>(null);
  const probe = useRef<HTMLDivElement>(null);
  const [geo, setGeo] = useState({width: 0, col: 160, top: 0});
  const vp = useViewport();
  const {ensure, at} = usePages(source, pageSize);
  const roving = useRoving(root, 'grid', [source.key, geo.width]);
  useLayoutEffect(() => {
    const measure = () => {
      if (!root.current || !probe.current) return;
      const cs = getComputedStyle(root.current);
      const inner = root.current.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
      setGeo({width: inner, col: probe.current.getBoundingClientRect().width || 160, top: root.current.getBoundingClientRect().top + window.scrollY});
    };
    measure();
    const ro = new ResizeObserver(measure);
    if (root.current) ro.observe(root.current);
    return () => ro.disconnect();
  }, []);
  const gap = geo.width < 600 ? 12 : 16;
  const cols = Math.max(2, Math.floor((geo.width + gap) / (geo.col + gap)));
  const cardW = (geo.width - gap * (cols - 1)) / cols;
  const rowH = cardW * RATIO[shape] + (shape === 'circle' ? TEXT_H : TEXT_H) + ROW_GAP;
  const rows = Math.ceil(source.total / cols);
  const firstRow = Math.max(0, Math.floor((vp.y - geo.top) / rowH) - 2);
  const lastRow = Math.min(rows - 1, Math.ceil((vp.y - geo.top + vp.h) / rowH) + 2);
  useEffect(() => { if (geo.width && rows) ensure(firstRow * cols, Math.min(source.total - 1, (lastRow + 1) * cols - 1)); }, [ensure, firstRow, lastRow, cols, rows, source.total, geo.width]);
  useEffect(() => {
    if (!jumpRef) return;
    jumpRef.current = (index: number) => {
      const top = (root.current?.getBoundingClientRect().top ?? 0) + window.scrollY;
      window.scrollTo({top: top + Math.floor(index / cols) * rowH - 120, behavior: 'smooth'});
    };
  }, [jumpRef, cols, rowH]);
  const rendered = useMemo(() => {
    if (!geo.width) return null;
    const out: React.ReactNode[] = [];
    for (let r = firstRow; r <= lastRow; r++) {
      const cells: React.ReactNode[] = [];
      for (let c = 0; c < cols; c++) {
        const i = r * cols + c;
        if (i >= source.total) break;
        const e = at(i);
        cells.push(e ? <div role="listitem" key={e.id} style={{minWidth: 0}}><MediaCard entry={e} shape={shape} grid origin={origin} /></div> : <CardSkeleton key={`s${i}`} shape={shape} grid />);
      }
      out.push(<div key={r} className={s.row} role="presentation" style={{top: r * rowH, gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`}}>{cells}</div>);
    }
    return out;
  }, [geo.width, firstRow, lastRow, cols, source.total, at, shape, origin, rowH]);
  return (
    <div className={s.root} ref={root} {...roving} role="list" aria-label="Titles" aria-rowcount={rows}>
      <div className={s.probe} ref={probe} style={{width: `var(--${shape}-w)`}} />
      <div className={s.canvas} style={{height: rows * rowH}}>{rendered}</div>
    </div>
  );
}

/** A virtualised list of fixed-height rows over a paged source (library list view, songs). */
export function WindowedList({source, rowHeight, renderRow, pageSize = 80, header}: {source: PageSource; rowHeight: number; renderRow: (e: Entry | undefined, index: number) => React.ReactNode; pageSize?: number; header?: React.ReactNode}) {
  const root = useRef<HTMLDivElement>(null);
  const [top, setTop] = useState(0);
  const vp = useViewport();
  const {ensure, at} = usePages(source, pageSize);
  useLayoutEffect(() => { if (root.current) setTop(root.current.getBoundingClientRect().top + window.scrollY); }, [source.key]);
  const first = Math.max(0, Math.floor((vp.y - top) / rowHeight) - 6);
  const last = Math.min(source.total - 1, Math.ceil((vp.y - top + vp.h) / rowHeight) + 6);
  useEffect(() => { if (source.total) ensure(first, last); }, [ensure, first, last, source.total]);
  const rows: React.ReactNode[] = [];
  for (let i = first; i <= last; i++) rows.push(<div key={i} className={s.listRow} style={{top: i * rowHeight, height: rowHeight}}>{renderRow(at(i), i)}</div>);
  return (
    <div>
      {header}
      <div className={s.list} ref={root} style={{height: source.total * rowHeight}} role="list">{rows}</div>
    </div>
  );
}
