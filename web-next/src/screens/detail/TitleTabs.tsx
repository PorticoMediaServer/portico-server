import {useEffect, useRef, useState, type ReactNode} from 'react';
import {useShellWidth} from '../../shell/viewport';
import styles from './detail.module.css';

/** Compact title/actions enter only after the real hero has scrolled out. */
export function TitleTabs({title, compact, children, actions}: {title: string; compact?: boolean; children: ReactNode; actions?: ReactNode}) {
  const anchor = useRef<HTMLDivElement>(null), [stuck, setStuck] = useState(false), width = useShellWidth();
  const top = compact ? 0 : width >= 1024 ? 76 : 56;
  useEffect(() => {
    if (!anchor.current) return;
    const observer = new IntersectionObserver(entries => { const entry = entries[0]; if (entry) setStuck(!entry.isIntersecting && entry.boundingClientRect.top < top); }, {rootMargin: `-${top}px 0px 0px 0px`});
    observer.observe(anchor.current); return () => observer.disconnect();
  }, [top]);
  return <><div ref={anchor} className={styles.tabAnchor} aria-hidden="true" /><div className={styles.sticky} data-stuck={stuck && !compact || undefined}>
    <span>{title}</span>{children}{actions}
  </div></>;
}
