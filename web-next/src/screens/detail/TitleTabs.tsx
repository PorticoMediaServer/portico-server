import {useEffect, useRef, useState, type ReactNode} from 'react';
import {useShellWidth} from '../../shell/viewport';
import styles from './detail.module.css';
import {scrollLayout, useLayoutMotion} from '../../kit/layout/motion';
import {Button} from '../../kit/controls';
import {t} from './text';

/** Compact title/actions enter only after the real hero has scrolled out. */
export function TitleTabs({title, compact, children, actions, onBack}: {title: string; compact?: boolean; children: ReactNode; actions?: ReactNode; onBack?: () => void}) {
  const anchor = useRef<HTMLDivElement>(null), [stuck, setStuck] = useState(false), width = useShellWidth(), motion = useLayoutMotion(), scroll = useRef<(() => void) | undefined>(undefined);
  useEffect(() => () => scroll.current?.(), []);
  const phone = width < 1024, top = compact ? 0 : phone ? 44 : 76;
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
    <span className={styles.pinnedDesktopTitle}>{title}</span><span className={styles.pinnedRule} aria-hidden="true" />{phone && stuck && !compact && <div className={styles.pinnedPhone}>{onBack && <Button variant="icon" icon="i-left" iconOnly aria-label={t('next.states.back')} onClick={onBack} />}<span>{title}</span></div>}{children}{actions}
  </div></>;
}
