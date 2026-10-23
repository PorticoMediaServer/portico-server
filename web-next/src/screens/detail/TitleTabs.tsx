import {useEffect, useRef, useState, type ReactNode} from 'react';
import {useShellWidth} from '../../shell/viewport';
import styles from './detail.module.css';
import {scrollLayout, useLayoutMotion} from '../../kit/layout/motion';

/** Compact title/actions enter only after the real hero has scrolled out. */
export function TitleTabs({title, compact, children, actions}: {title: string; compact?: boolean; children: ReactNode; actions?: ReactNode}) {
  const anchor = useRef<HTMLDivElement>(null), [stuck, setStuck] = useState(false), width = useShellWidth(), motion = useLayoutMotion(), scroll = useRef<(() => void) | undefined>(undefined);
  useEffect(() => () => scroll.current?.(), []);
  const top = compact ? 0 : width >= 1024 ? 76 : 56;
  useEffect(() => {
    if (!anchor.current) return;
    const observer = new IntersectionObserver(entries => { const entry = entries[0]; if (entry) setStuck(!entry.isIntersecting && entry.boundingClientRect.top < top); }, {rootMargin: `-${top}px 0px 0px 0px`});
    observer.observe(anchor.current); return () => observer.disconnect();
  }, [top]);
  return <><div ref={anchor} className={styles.tabAnchor} aria-hidden="true" /><div className={styles.sticky} data-stuck={stuck && !compact || undefined} onClickCapture={event => {
    const tab = (event.target as Element).closest('[role=tab]');
    if (tab?.getAttribute('aria-selected') !== 'true') return;
    let scroller = anchor.current?.parentElement;
    while (scroller && scroller !== document.documentElement && !(/auto|scroll/.test(getComputedStyle(scroller).overflowY) && scroller.scrollHeight > scroller.clientHeight)) scroller = scroller.parentElement;
    scroller ??= document.scrollingElement as HTMLElement;
    scroll.current?.(); scroll.current = scrollLayout(scroller, scroller.scrollLeft, 0, motion.reduced);
  }}>
    <span>{title}</span>{children}{actions}
  </div></>;
}
