import { useLayoutEffect, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router';
import { Search } from 'lucide-react';
import { MOBILE_TABS, isActive } from './nav';
import { useLibraries } from '../services/queries';
import { cls } from '../components/ui';
import { useRefraction } from '../lib/refraction';

export function TabBar() {
  const { pathname } = useLocation();
  const libs = useLibraries();
  const tabs = MOBILE_TABS.filter((t) => !t.library || !libs.data || libs.data.some((l) => l.id === t.library));
  const active = tabs.find((t) => isActive(t, pathname))?.id;
  const searchOn = pathname.startsWith('/search');
  const bar = useRef<HTMLElement>(null);
  const search = useRef<HTMLAnchorElement>(null);
  const [lens, setLens] = useState<{ x: number; w: number } | null>(null);
  useRefraction(bar, { scale: 60 });
  useRefraction(search, { scale: 50 });

  useLayoutEffect(() => {
    const el = bar.current;
    if (!el) return;
    const measure = () => {
      const a = el.querySelector<HTMLElement>(`[data-tab="${active}"]`);
      setLens(a ? { x: a.offsetLeft, w: a.offsetWidth } : null);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [active, tabs.length]);

  return (
    <div className="tabbar-wrap">
      <nav ref={bar} className="tabbar glass glass-chrome refract" aria-label="Tabs">
        {lens && <span className="tab-lens" style={{ width: lens.w, transform: `translateX(${lens.x}px)` }} aria-hidden="true" />}
        {tabs.map((t) => (
          <Link key={t.id} to={t.to} data-tab={t.id} className={cls('tab', active === t.id && 'on')} aria-current={active === t.id ? 'page' : undefined} viewTransition>
            <t.icon />
            <span>{t.label}</span>
          </Link>
        ))}
      </nav>
      <Link ref={search} to="/search" className={cls('tab-search glass glass-chrome refract', searchOn && 'on')} aria-label="Search" viewTransition>
        <Search />
      </Link>
    </div>
  );
}
