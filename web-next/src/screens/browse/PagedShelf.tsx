import {useCallback, useMemo, useRef} from 'react';
import {Shelf, type ShelfProps} from '../../kit/layout';
import {Skeleton} from '../../kit/states';
import styles from './browse.module.css';

type Slot<T> = Readonly<{kind: 'item'; value: T}> | Readonly<{kind: 'pending'}>;
export type PagedShelfProps<T> = ShelfProps<T> & Readonly<{
  hasMore: boolean;
  onMore: () => void;
  /** A source cursor, never an item identity. Changes only after a real page. */
  continuation: string;
  cardWidth: number;
  artworkRatio: number;
}>;
/** A bounded server row. The sole trailing placeholder is never a selectable entity. */
export function PagedShelf<T>({items, getKey, renderItem, onItemFocus, onVisibleRange, hasMore, onMore, continuation, cardWidth, artworkRatio, ...props}: PagedShelfProps<T>) {
  const requested = useRef<string | undefined>(undefined);
  const slots = useMemo<readonly Slot<T>[]>(() => [...items.map(value => ({kind: 'item' as const, value})), ...(hasMore ? [{kind: 'pending' as const}] : [])], [items, hasMore]);
  const visibleRange = useCallback((first: number, last: number) => {
    const realLast = Math.min(last, items.length - 1);
    onVisibleRange?.(first < items.length ? first : -1, realLast);
    if (!hasMore || last < Math.max(0, items.length - 10)) { requested.current = undefined; return; }
    if (requested.current === continuation) return;
    requested.current = continuation;
    onMore();
  }, [onVisibleRange, hasMore, items.length, continuation, onMore]);
  return <Shelf {...props} items={slots} itemWidth={cardWidth} getKey={slot => slot.kind === 'item' ? JSON.stringify(['entry', getKey(slot.value)]) : 'pending-page'}
    onVisibleRange={visibleRange} onItemFocus={(slot, index) => { if (slot.kind === 'item') onItemFocus?.(slot.value, index); }}
    renderItem={(slot, context) => slot.kind === 'item' ? renderItem(slot.value, {...context, key: getKey(slot.value)}) :
      <div className={styles.pendingCard} data-row-pending aria-busy="true"><Skeleton width={cardWidth} height={cardWidth / artworkRatio}/></div>} />;
}
