import React, {forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import type {Slot, WindowedCollection} from '@core/collections/index.ts';
import {cx} from './cx';
import {settledPitch} from './settled-pitch';
import {useCardSize} from './Grid';
import s from './WindowedGrid.module.css';

export {createStaticCollection} from './static-collection';

/**
 * A grid (or one-column list) over a sparse `WindowedCollection` (the scale
 * rule: client cost follows what's on screen, never the collection's size).
 *
 * - Only the rows in view, plus `overscanRows` either side, are in the DOM.
 *   The container is sized from `total` × the measured row pitch, so the
 *   page's own scrollbar covers the whole collection.
 * - Column count comes from the rendered CSS grid, so the layout stays in CSS
 *   (`--card-min`, gaps) and responds to resizes.
 * - Each page of cells subscribes to its own page only; a page arriving
 *   re-renders its cells and nothing else.
 * - Unloaded slots render `renderPlaceholder`, which should match the card's
 *   final geometry.
 */
/** Tallest the window's own box gets; beyond it, scrolling is mapped (see `update`). */
const MAX_HEIGHT = 8_000_000;

export type WindowedGridHandle = {
  /** Scroll so this item's row is at the top of the viewport (below `stickyOffset`). */
  scrollToIndex(index: number): void;
};

type Props<T> = {
  collection: WindowedCollection<T>;
  renderItem: (item: T, index: number) => React.ReactNode;
  renderPlaceholder: (index: number) => React.ReactNode;
  /** A first guess at the row pitch (px) before a row has been measured. */
  estimateRowHeight: number;
  /** Minimum column width for the CSS grid (e.g. `clamp(120px, 11vw, 168px)`); omit for a one-column list. */
  columnMin?: string;
  overscanRows?: number;
  /** Space covered by sticky headers at the top of the viewport. */
  stickyOffset?: number;
  className?: string;
  /** Called with the first item index at the top of the viewport. */
  onFirstVisible?: (index: number) => void;
  label?: string;
};

function WindowedGridInner<T>({collection, renderItem, renderPlaceholder, estimateRowHeight, columnMin, overscanRows = 2, stickyOffset = 0, className, onFirstVisible, label}: Props<T>, ref: React.Ref<WindowedGridHandle>) {
  const snapshot = useSyncExternalStore(collection.subscribe, collection.getSnapshot);
  const container = useRef<HTMLDivElement>(null);
  const grid = useRef<HTMLDivElement>(null);
  const [layout, setLayout] = useState({cols: 1, pitch: estimateRowHeight, gap: 0});
  const [view, setView] = useState({first: 0, last: 3, offset: 0});
  // Before the first page arrives the total is unknown: show one page of placeholders.
  const total = snapshot.total ?? (snapshot.status === 'error' ? 0 : collection.pageSize);
  const totalRows = Math.ceil(total / layout.cols);
  const realHeight = totalRows > 0 ? totalRows * layout.pitch - layout.gap : 0;
  const compressed = realHeight > MAX_HEIGHT;
  const height = compressed ? MAX_HEIGHT : realHeight;
  // Browsers cap an element's height (Chrome ≈ 33.5 M px, Firefox ≈ 17.9 M px), and a
  // 10 M-item grid is far taller. Past MAX_HEIGHT the window maps scroll position to a
  // virtual position: wheel, keys and small scrolls move 1:1 from where you are, a
  // scrollbar drag or a jump maps proportionally, and the ends always line up.
  const virtual = useRef({top: 0, scrolled: 0, pitch: 0, cols: 1});

  const update = useCallback(() => {
    const el = container.current;
    if (!el) return;
    const top = el.getBoundingClientRect().top;
    const viewport = window.innerHeight;
    const pitch = layout.pitch || estimateRowHeight;
    const scrolled = stickyOffset - top;
    let v = scrolled;
    if (compressed) {
      const maxScrolled = Math.max(1, height - viewport);
      const maxVirtual = Math.max(1, realHeight - viewport);
      // A new layout (resize, columns) keeps the same first item on screen.
      let from = virtual.current.top;
      if (virtual.current.pitch && (virtual.current.pitch !== pitch || virtual.current.cols !== layout.cols)) {
        const anchor = Math.floor(from / virtual.current.pitch) * virtual.current.cols;
        from = Math.floor(anchor / layout.cols) * pitch;
      }
      const delta = scrolled - virtual.current.scrolled;
      if (scrolled <= 0) v = scrolled;
      else if (scrolled >= maxScrolled) v = maxVirtual + (scrolled - maxScrolled);
      else if (Math.abs(delta) < viewport * 3) v = Math.min(maxVirtual, Math.max(0, from + delta));
      else v = (scrolled / maxScrolled) * maxVirtual;
    }
    virtual.current = {top: v, scrolled, pitch, cols: layout.cols};
    const firstRow = Math.max(0, Math.floor(v / pitch) - overscanRows);
    const lastRow = Math.max(firstRow, Math.min(Math.max(0, totalRows - 1), Math.ceil((v + viewport - stickyOffset) / pitch) + overscanRows));
    const offset = firstRow * pitch - v + scrolled;
    setView(prev => (prev.first === firstRow && prev.last === lastRow && Math.abs(prev.offset - offset) < 0.5 ? prev : {first: firstRow, last: lastRow, offset}));
    const visibleFirst = Math.max(0, Math.floor(v / pitch)) * layout.cols;
    onFirstVisible?.(Math.min(visibleFirst, Math.max(0, total - 1)));
  }, [layout, estimateRowHeight, stickyOffset, overscanRows, totalRows, total, onFirstVisible, compressed, height, realHeight]);

  // Scroll and resize drive the window; one read per frame.
  // (A hidden document gets no animation frames, so it falls back to a timer.)
  useEffect(() => {
    let frame = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const run = () => { frame = 0; timer = undefined; update(); };
    const schedule = () => {
      if (frame || timer) return;
      if (document.visibilityState === 'hidden') timer = setTimeout(run, 16);
      else frame = requestAnimationFrame(run);
    };
    update();
    window.addEventListener('scroll', schedule, {passive: true});
    window.addEventListener('resize', schedule);
    return () => { cancelAnimationFrame(frame); clearTimeout(timer); window.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule); };
  }, [update]);

  const rows = view;
  // Declare interest: the store fetches, prefetches, aborts and evicts.
  const first = Math.min(rows.first * layout.cols, Math.max(0, total - 1));
  const last = Math.min(total - 1, (rows.last + 1) * layout.cols - 1);
  useEffect(() => { if (total > 0) collection.ensureRange(first, last); }, [collection, first, last, total, snapshot.generation]);

  // Measure columns and row pitch from what's rendered. Rows of different heights (a poster row
  // beside a square one) made the average depend on which rows were in the window and the window
  // on the average: a layout loop React stops with "Maximum update depth exceeded". So once measured
  // at a width the pitch only grows (`settledPitch`), and every row is held to it (`--row-min`),
  // which keeps positions exact and lets the loop settle.
  const measured = useRef<{width: number; pitch: number; columnMin?: string; cardSize: string} | undefined>(undefined);
  // A new card size changes the columns and the row height without changing the grid's width.
  const cardSize = useCardSize();
  useLayoutEffect(() => {
    const el = grid.current;
    if (!el) return;
    const measure = () => {
      const width = el.clientWidth;
      // A new width or a new presentation (grid ↔ list) starts the measurement over; the
      // settled pitch only ever grows, so a list would otherwise keep the grid's row height.
      if (measured.current && (measured.current.width !== width || measured.current.columnMin !== columnMin || measured.current.cardSize !== cardSize)) { measured.current = undefined; el.style.removeProperty('--row-min'); }
      const style = getComputedStyle(el);
      const cols = columnMin ? Math.max(1, style.gridTemplateColumns.split(' ').filter(Boolean).length) : 1;
      const gap = parseFloat(style.rowGap) || 0;
      const rendered = Math.ceil(el.childElementCount / cols);
      const height = el.getBoundingClientRect().height;
      let pitch = estimateRowHeight;
      if (rendered > 0 && height > 0) {
        pitch = settledPitch(measured.current?.pitch, (height + gap) / rendered);
        measured.current = {width, pitch, columnMin, cardSize};
        el.style.setProperty('--row-min', `${Math.max(0, pitch - gap)}px`);
      }
      setLayout(prev => (prev.cols === cols && Math.abs(prev.pitch - pitch) < 0.5 && prev.gap === gap ? prev : {cols, pitch, gap}));
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [columnMin, estimateRowHeight, first, last, cardSize]);

  useImperativeHandle(ref, () => ({
    scrollToIndex(index: number) {
      const el = container.current;
      if (!el) return;
      const row = Math.floor(Math.max(0, index) / layout.cols);
      const target = row * layout.pitch;
      let scrolled = target;
      if (compressed) {
        const viewport = window.innerHeight;
        const maxVirtual = Math.max(1, realHeight - viewport);
        scrolled = (Math.min(target, maxVirtual) / maxVirtual) * Math.max(1, height - viewport);
        virtual.current = {top: Math.min(target, maxVirtual), scrolled, pitch: layout.pitch, cols: layout.cols};
      }
      window.scrollTo({top: Math.max(0, el.getBoundingClientRect().top + window.scrollY + scrolled - stickyOffset)});
    },
  }), [layout, stickyOffset, compressed, realHeight, height]);

  const pages: {page: number; from: number; to: number}[] = [];
  if (total > 0) {
    for (let page = collection.pageOf(first); page <= collection.pageOf(last); page++) {
      const from = Math.max(first, page * collection.pageSize);
      const to = Math.min(last, (page + 1) * collection.pageSize - 1);
      if (from <= to) pages.push({page, from, to});
    }
  }
  return (
    <div ref={container} className={cx(s.window, className)} style={{height}} role={label ? 'region' : undefined} aria-label={label}>
      <div ref={grid} className={columnMin ? s.grid : s.list} style={{transform: `translateY(${compressed ? rows.offset : rows.first * layout.pitch}px)`, ...(columnMin ? ({'--card-min': columnMin} as React.CSSProperties) : {})}}>
        {pages.map(p => <PageCells key={`${snapshot.generation}:${p.page}`} collection={collection} page={p.page} from={p.from} to={p.to} renderItem={renderItem} renderPlaceholder={renderPlaceholder} />)}
      </div>
    </div>
  );
}

function PageCellsInner<T>({collection, page, from, to, renderItem, renderPlaceholder}: {collection: WindowedCollection<T>; page: number; from: number; to: number; renderItem: (item: T, index: number) => React.ReactNode; renderPlaceholder: (index: number) => React.ReactNode}) {
  // Subscribes to this page only.
  useSyncExternalStore(useCallback((cb: () => void) => collection.subscribePage(page, cb), [collection, page]), () => collection.getPageVersion(page));
  const cells: React.ReactNode[] = [];
  for (let index = from; index <= to; index++) {
    const slot: Slot<T> = collection.slotAt(index);
    cells.push(<React.Fragment key={slot.key}>{slot.kind === 'item' ? renderItem(slot.item, index) : renderPlaceholder(index)}</React.Fragment>);
  }
  return <>{cells}</>;
}
const PageCells = React.memo(PageCellsInner) as typeof PageCellsInner;

export const WindowedGrid = forwardRef(WindowedGridInner) as <T>(props: Props<T> & {ref?: React.Ref<WindowedGridHandle>}) => React.ReactElement;
