import React from 'react';
import {Link} from '@tanstack/react-router';
import {Tabs as RTabs} from 'radix-ui';
import {cx} from './cx';
import s from './Tabs.module.css';

export type TabDef<V extends string> = {value: V; label: string; count?: number};

/** In-page tabs (state, not URL). Arrow keys move between them. */
export function Tabs<V extends string>({value, onChange, tabs, children, label, className}: {value: V; onChange: (v: V) => void; tabs: readonly TabDef<V>[]; children?: React.ReactNode; label: string; className?: string}) {
  return (
    <RTabs.Root value={value} onValueChange={v => onChange(v as V)} className={className}>
      <RTabs.List className={s.list} aria-label={label}>
        {tabs.map(t => <RTabs.Trigger key={t.value} value={t.value} className={s.tab}>{t.label}{t.count !== undefined ? <span className={s.count}>{t.count}</span> : null}</RTabs.Trigger>)}
      </RTabs.List>
      {children}
    </RTabs.Root>
  );
}
export function TabPanel({value, children}: {value: string; children: React.ReactNode}) {
  return <RTabs.Content value={value} className={s.panel}>{children}</RTabs.Content>;
}

/** Tabs that are links: each tab is its own URL (library pivots, saved views, account sections). */
export function LinkTabs({tabs, current, label, className}: {tabs: readonly {id: string; label: string; href: string; count?: number}[]; current: string; label: string; className?: string}) {
  const ref = React.useRef<HTMLElement>(null);
  // On narrow screens the tabs scroll; keep the current one in view.
  React.useEffect(() => {
    const nav = ref.current;
    const el = nav?.querySelector<HTMLElement>('[aria-current="page"]');
    if (nav && el && (el.offsetLeft < nav.scrollLeft || el.offsetLeft + el.offsetWidth > nav.scrollLeft + nav.clientWidth)) nav.scrollLeft = el.offsetLeft - 16;
  }, [current]);
  return (
    <nav ref={ref} className={cx(s.list, className)} aria-label={label}>
      {tabs.map(t => (
        <Link key={t.id} to={t.href as never} className={s.tab} aria-current={t.id === current ? 'page' : undefined} activeOptions={{exact: true, includeSearch: false}} data-status={t.id === current ? 'active' : undefined} activeProps={{}}>
          {t.label}{t.count !== undefined ? <span className={s.count}>{t.count}</span> : null}
        </Link>
      ))}
    </nav>
  );
}
