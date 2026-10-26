import {useCallback, useEffect, useImperativeHandle, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type CSSProperties, type ReactNode, type Ref} from 'react';
import type {WindowedCollection} from '@core/collections/windowed';
import {gridColumns, gridWindow} from './geometry';
import {cancelLayoutScroll, scrollLayout, useLayoutMotion} from './motion';
import {RemovalFocusOwner} from './removal-focus-owner';
import {GridRemovalFocus} from './GridRemovalFocus';
import styles from './layout.module.css';

export type WindowedGridHandle = {scrollToIndex: (index: number, options?: {focus?: boolean}) => void};
export type GridItemContext = Readonly<{index: number; key: string; focused: boolean}>;
export type WindowedGridProps<T> = {
  collection: WindowedCollection<T>; renderItem: (item: T, context: GridItemContext) => ReactNode; renderPlaceholder: (index: number) => ReactNode;
  label: string; height: number | string; cardWidth: number; rowHeight: number; gap?: number; rowGap?: number; overscanRows?: number; stickyOffset?: number;
  /** Position once after this collection total and viewport are observed. */
  initialIndex?: number;
  onVisibleRange?: (first: number, last: number) => void; onItemFocus?: (item: T, index: number) => void; reducedMotion?: boolean;
  ref?: Ref<WindowedGridHandle>; className?: string; style?: CSSProperties;
};

// Keep even million-item spans below the smallest supported browser's box limit.
const MAX_SCROLL_HEIGHT = 8_000_000;
export function WindowedGrid<T>({collection, renderItem, renderPlaceholder, label, height, cardWidth, rowHeight, gap = 20, rowGap = 30, overscanRows = 1, stickyOffset = 0,
  initialIndex, onVisibleRange, onItemFocus, reducedMotion = false, ref, className, style}: WindowedGridProps<T>) {
  const initialPosition = useRef<WindowedCollection<T> | undefined>(undefined);
  const root = useRef<HTMLDivElement>(null), pendingFocus = useRef<number | null>(null), [viewport, setViewport] = useState({width: 0, height: 0, top: 0});
  const [focused, setFocused] = useState<string | null>(null), motion = useLayoutMotion(reducedMotion);
  const focusOwner = useMemo(() => new RemovalFocusOwner(), [collection]);
  useLayoutEffect(() => {pendingFocus.current = null;setFocused(null);}, [focusOwner]);
  const snapshot = useSyncExternalStore(collection.subscribe, collection.getSnapshot, collection.getSnapshot);
  const total = snapshot.total ?? collection.pageSize, columns = gridColumns(viewport.width, cardWidth, gap), pitch = rowHeight + rowGap;
  const logicalHeight = Math.max(0, Math.ceil(total / columns) * pitch - rowGap), scrollHeight = Math.min(MAX_SCROLL_HEIGHT, logicalHeight);
  const usableHeight = Math.max(1, viewport.height - stickyOffset), maxPhysical = Math.max(1, scrollHeight - usableHeight), maxLogical = Math.max(1, logicalHeight - usableHeight);
  const logicalTop = viewport.top * maxLogical / maxPhysical;
  const range = gridWindow(total, columns, pitch, logicalTop, usableHeight, overscanRows);
  const firstPage = collection.pageOf(range.renderFirst), lastPage = collection.pageOf(Math.max(range.renderFirst, range.renderLast));
  const subscribePages = useCallback((changed: () => void) => {
    const unsubscribers: (() => void)[] = [];
    for (let page = firstPage; page <= lastPage; page++) unsubscribers.push(collection.subscribePage(page, changed));
    return () => { for (const unsubscribe of unsubscribers) unsubscribe(); };
  }, [collection, firstPage, lastPage]);
  const pageVersion = useCallback(() => { let version = ''; for (let page = firstPage; page <= lastPage; page++) version += `${collection.getPageVersion(page)};`; return version; }, [collection, firstPage, lastPage]);
  const pages = useSyncExternalStore(subscribePages, pageVersion, pageVersion);
  useEffect(() => {
    const element = root.current; if (!element) return;
    const read = () => setViewport({width: element.clientWidth, height: element.clientHeight, top: element.scrollTop});
    const observer = new ResizeObserver(read); observer.observe(element); read();
    return () => { observer.disconnect(); cancelLayoutScroll(element); };
  }, []);
  useEffect(() => {
    if (range.renderLast >= range.renderFirst) collection.ensureRange(range.renderFirst, range.renderLast);
    onVisibleRange?.(range.first, range.last);
  }, [collection, range.renderFirst, range.renderLast, range.first, range.last, onVisibleRange]);
  const jump = useCallback((index: number, focus = false, repeat = false) => {
    const element = root.current; if (!element || index < 0 || index >= total) return;
    if (focus) { pendingFocus.current = index; element.focus({preventScroll: true}); }
    const rowTop = Math.floor(index / columns) * pitch, rowBottom = rowTop + rowHeight;
    const target = focus && rowTop >= logicalTop && rowBottom <= logicalTop + usableHeight ? logicalTop : focus && rowBottom > logicalTop + usableHeight ? rowBottom - usableHeight : rowTop;
    // Alphabet/End jumps go straight to their sparse window instead of fetching every crossed page.
    const instant = motion.reduced || !focus || Math.abs(target - logicalTop) > usableHeight * 2;
    scrollLayout(element, 0, Math.min(maxPhysical, Math.max(0, target * maxPhysical / maxLogical)), instant, repeat);
    if (focus) setViewport(previous => ({...previous}));
  }, [total, columns, pitch, rowHeight, logicalTop, usableHeight, maxPhysical, maxLogical, motion.reduced]);
  useLayoutEffect(() => {
    if (initialPosition.current === collection || initialIndex === undefined || !Number.isSafeInteger(initialIndex)
      || initialIndex < 0 || snapshot.total === undefined || initialIndex >= snapshot.total || viewport.width <= 0 || viewport.height <= 0) return;
    initialPosition.current = collection; jump(initialIndex);
  }, [collection, initialIndex, snapshot.total, viewport.width, viewport.height, jump]);
  useImperativeHandle(ref, () => ({scrollToIndex: (index, options) => jump(index, options?.focus)}), [jump]);
  useLayoutEffect(() => {
    const index = pendingFocus.current; if (index === null) return;
    const slot = root.current?.querySelector<HTMLElement>(`[data-grid-index="${index}"]`);
    if (!slot?.dataset.itemId) return;
    const target = slot.querySelector<HTMLElement>('[data-layout-primary],button:not(:disabled),a[href],[tabindex="0"]') ?? slot;
    target.focus({preventScroll: true}); pendingFocus.current = null;
  }, [range.renderFirst, range.renderLast, pages, viewport]);
  const rendered: ReactNode[] = [], keys = new Set<string>();
  for (let index = range.renderFirst; index <= range.renderLast; index++) {
    const slot = collection.slotAt(index), row = Math.floor(index / columns), column = index % columns;
    if (slot.kind === 'item') keys.add(slot.key);
    // An unloaded slot's coordinate key never stands in for an entity ID or selection ID.
    rendered.push(<div key={slot.key} className={styles.gridItem} role="listitem" aria-posinset={index + 1} aria-setsize={snapshot.total ?? -1}
      tabIndex={-1} data-grid-index={index} data-item-id={slot.kind === 'item' ? slot.key : undefined} data-layout-item={slot.kind === 'item' ? slot.key : undefined}
      style={{width: cardWidth, height: rowHeight, left: column * (cardWidth + gap), top: row * pitch - logicalTop + viewport.top + stickyOffset}}
      onFocusCapture={() => { if (slot.kind === 'item') { setFocused(slot.key); onItemFocus?.(slot.item, index); } }}>
      {slot.kind === 'item' ? renderItem(slot.item, {index, key: slot.key, focused: slot.key === focused}) : renderPlaceholder(index)}
    </div>);
  }
  return <GridRemovalFocus owner={focusOwner} root={root} generation={snapshot.generation} accepted={snapshot.status === 'ready' && !snapshot.refreshing && snapshot.total !== undefined} keys={keys} viewport={viewport}>
    <div ref={root} tabIndex={-1} className={[styles.grid, className].filter(Boolean).join(' ')} style={{height, ...style}} role="list" aria-label={label} aria-busy={snapshot.status === 'loading'}
    onScroll={event => { const element = event.currentTarget; setViewport({width: element.clientWidth, height: element.clientHeight, top: element.scrollTop}); }}
    onBlurCapture={event => { const next = event.relatedTarget as HTMLElement | null; if (!next || !event.currentTarget.contains(next) || !next.closest('[data-item-id]')) setFocused(null); }}
    onKeyDown={event => {
      if ((event.target as HTMLElement).closest('input,textarea,select,[contenteditable="true"]')) return;
      const slot = (event.target as HTMLElement).closest<HTMLElement>('[data-grid-index]'); if (!slot) return;
      const index = Number(slot.dataset.gridIndex);
      const delta = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowDown' ? columns : event.key === 'ArrowUp' ? -columns : 0;
      const target = event.key === 'Home' ? 0 : event.key === 'End' ? total - 1 : index + delta;
      if (!delta && event.key !== 'Home' && event.key !== 'End') return;
      if (target < 0 || target >= total) return;
      event.preventDefault(); jump(target, true, event.repeat);
    }}><div key={focusOwner.lifetime} className={styles.gridExtent} style={{height: scrollHeight + stickyOffset, minWidth: columns * cardWidth + (columns - 1) * gap}}>{rendered}</div></div>
  </GridRemovalFocus>;
}
