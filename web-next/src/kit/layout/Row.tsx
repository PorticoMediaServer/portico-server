import {useCallback, useEffect, useId, useRef, useState, type CSSProperties, type HTMLAttributes, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../Icon';
import {layoutType, platformType, type LayoutPlatform} from './Lockup';
import {anchoredOffset} from './geometry';
import {cancelLayoutScroll, scrollLayout, useLayoutMotion} from './motion';
import styles from './layout.module.css';

export type RowProps = Omit<HTMLAttributes<HTMLElement>, 'title'> & {heading: ReactNode; subtitle?: ReactNode; onSeeAll?: () => void; seeAllHref?: string; platform?: LayoutPlatform};
export function Row({heading, subtitle, onSeeAll, seeAllHref, platform = 'desktop', children, className, ...props}: RowProps) {
  const id = useId(), type = platformType(platform);
  return <section {...props} className={[styles.row, className].filter(Boolean).join(' ')} data-platform={platform} aria-labelledby={id}>
    <header className={styles.rowHead}><h2 id={id} style={layoutType(type.rowHeading)}>{heading}</h2>
      {subtitle && <span className={styles.rowSubtitle} style={layoutType(type.rowSubtitle)}>{subtitle}</span>}
      {seeAllHref ? <a className={styles.seeAll} href={seeAllHref} onClick={onSeeAll} style={layoutType(type.seeAll)}>{defaultI18n.t('next.layout.seeAll')}<Icon name="i-right" size={16} /></a>
        : onSeeAll && <button type="button" className={styles.seeAll} onClick={onSeeAll} style={layoutType(type.seeAll)}>{defaultI18n.t('next.layout.seeAll')}<Icon name="i-right" size={16} /></button>}
    </header>{children}
  </section>;
}

export type ShelfItemContext = Readonly<{index: number; focused: boolean; key: string}>;
export type ShelfProps<T> = Omit<RowProps, 'children'> & {
  /** Only the bounded row window returned by the server; never a whole-library array. */
  items: readonly T[]; getKey: (item: T) => string; renderItem: (item: T, context: ShelfItemContext) => ReactNode;
  onItemFocus?: (item: T, index: number) => void; onVisibleRange?: (first: number, last: number) => void;
  itemWidth?: number; reducedMotion?: boolean;
};

export function Shelf<T>({items, getKey, renderItem, onItemFocus, onVisibleRange, itemWidth, reducedMotion = false, platform = 'desktop', ...props}: ShelfProps<T>) {
  const track = useRef<HTMLDivElement>(null), [focusedKey, setFocusedKey] = useState<string | null>(null), [edges, setEdges] = useState({left: false, right: false});
  const motion = useLayoutMotion(reducedMotion);
  const measure = useCallback(() => {
    const element = track.current; if (!element) return;
    setEdges({left: element.scrollLeft > 1, right: element.scrollLeft + element.clientWidth < element.scrollWidth - 1});
    let first = -1, last = -1;
    const bounds = element.getBoundingClientRect();
    [...element.children].forEach((child, index) => { const rect = child.getBoundingClientRect(); if (rect.right > bounds.left && rect.left < bounds.right) { if (first < 0) first = index; last = index; } });
    onVisibleRange?.(first, last);
  }, [onVisibleRange]);
  useEffect(() => {
    const element = track.current; if (!element) return;
    const observer = new ResizeObserver(measure); observer.observe(element); measure();
    return () => { observer.disconnect(); cancelLayoutScroll(element); };
  }, [measure, items]);
  const step = (direction: number) => { const element = track.current; if (element) scrollLayout(element, Math.max(0, Math.min(element.scrollWidth - element.clientWidth, element.scrollLeft + direction * (element.clientWidth - (platform === 'tv' ? 296 : platform === 'desktop' ? 128 : 40)))), 0, motion.reduced); };
  return <Row {...props} platform={platform}>
    <div ref={track} className={styles.track} onScroll={measure} onKeyDown={event => {
      if ((event.target as HTMLElement).closest('input,textarea,select,[contenteditable="true"]')) return;
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
      const element = track.current, item = (event.target as HTMLElement).closest<HTMLElement>('[data-shelf-index]');
      if (!element || !item) return;
      const index = Number(item.dataset.shelfIndex), next = index + (event.key === 'ArrowRight' ? 1 : -1), target = element.children[next] as HTMLElement | undefined;
      const focusable = target?.querySelector<HTMLElement>('[data-layout-primary],button:not(:disabled),a[href],[tabindex="0"]');
      if (!focusable) return;
      event.preventDefault(); focusable.focus({preventScroll: true});
      const inset = platform === 'tv' ? 200 : platform === 'desktop' ? 64 : 20;
      scrollLayout(element, anchoredOffset(target.offsetLeft, target.offsetLeft + target.offsetWidth, element.clientWidth, element.scrollLeft, inset), 0, motion.reduced, event.repeat);
    }} onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setFocusedKey(null); }}>
      {items.map((item, index) => { const key = getKey(item); return <div key={key} className={styles.shelfItem} style={itemWidth ? {width: itemWidth} : undefined} data-shelf-index={index} data-item-id={key}
        onFocusCapture={() => { setFocusedKey(key); onItemFocus?.(item, index); }}>
        {renderItem(item, {index, key, focused: focusedKey === key})}
      </div>; })}
    </div>
    {platform === 'desktop' && <><button type="button" className={`${styles.arrow} ${styles.arrowLeft}`} aria-label={defaultI18n.t('next.layout.previousItems')} disabled={!edges.left} onClick={() => step(-1)}><Icon name="i-left" size={28} /></button>
      <button type="button" className={`${styles.arrow} ${styles.arrowRight}`} aria-label={defaultI18n.t('next.layout.nextItems')} disabled={!edges.right} onClick={() => step(1)}><Icon name="i-right" size={28} /></button></>}
  </Row>;
}

export type FocusAnchorProps = HTMLAttributes<HTMLDivElement> & {reducedMotion?: boolean; inset?: number};
/** Browser counterpart of the native worklet: observe real DOM focus and keep its slot safe. */
export function FocusAnchor({reducedMotion = false, inset = 0, onFocusCapture, children, ...props}: FocusAnchorProps) {
  const motion = useLayoutMotion(reducedMotion), ref = useRef<HTMLDivElement>(null);
  useEffect(() => { const element = ref.current; return () => { if (element) cancelLayoutScroll(element); }; }, []);
  return <div {...props} ref={ref} onFocusCapture={event => {
    const element = event.currentTarget, target = event.target as HTMLElement;
    const outer = element.getBoundingClientRect(), inner = target.getBoundingClientRect();
    scrollLayout(element, anchoredOffset(inner.left - outer.left + element.scrollLeft, inner.right - outer.left + element.scrollLeft, element.clientWidth, element.scrollLeft, inset),
      anchoredOffset(inner.top - outer.top + element.scrollTop, inner.bottom - outer.top + element.scrollTop, element.clientHeight, element.scrollTop, inset), motion.reduced);
    onFocusCapture?.(event);
  }}>{children}</div>;
}

export type FlowSection = Readonly<{id: string; heading: ReactNode; count?: ReactNode; cap: number; itemWidth: number; itemCount: number; fullWidth?: boolean; grow?: boolean; content: ReactNode; onSeeAll?: () => void}>;
export function FlowSections({sections, className, ...props}: Omit<HTMLAttributes<HTMLDivElement>, 'children'> & {sections: readonly FlowSection[]}) {
  return <div {...props} className={[styles.flow, className].filter(Boolean).join(' ')}>{sections.map(section => <section key={section.id} className={styles.flowSection}
    style={{width: section.fullWidth ? '100%' : Math.min(section.cap, section.itemCount) * section.itemWidth + Math.max(0, Math.min(section.cap, section.itemCount) - 1) * 16, flexGrow: section.grow ? 1 : 0} as CSSProperties}>
    <header className={styles.flowHead}><h2>{section.heading}</h2>{section.count !== undefined && section.count !== null && <span>{section.count}</span>}{section.onSeeAll && <button type="button" onClick={section.onSeeAll}>{defaultI18n.t('next.layout.seeAll')}<Icon name="i-right" size={16} /></button>}</header>
    {section.content}
  </section>)}</div>;
}
