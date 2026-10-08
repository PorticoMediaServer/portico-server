import { useLayoutEffect, useRef, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { Bell, Cast, ChevronDown, Search } from 'lucide-react';
import { MORE, PERSONAL, PRIMARY, sectionFor, type NavItem } from './nav';
import { useLibraries, useNotifications } from '../services/queries';
import { useUi } from '../state/ui';
import { usePlayer } from '../state/player';
import { IconButton, Kbd, cls } from '../components/ui';
import { Menu, useMenu } from '../components/overlay';
import { useRefraction, useSheen } from '../lib/refraction';
import { useIsDesktop, useScrolled } from '../lib/hooks';
import { ProfileButton } from './ProfileMenu';

export function useVisibleNav() {
  const libs = useLibraries();
  const ids = new Set((libs.data ?? []).map((l) => l.id));
  const ok = (i: NavItem) => !i.library || !libs.data || ids.has(i.library);
  return { primary: PRIMARY.filter(ok), more: MORE.filter(ok), personal: PERSONAL };
}

export function TopBar() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const anime = useUi((s) => s.animeHint);
  const openPalette = useUi((s) => s.openPalette);
  const setNotifications = useUi((s) => s.setNotifications);
  const setCast = useUi((s) => s.setCast);
  const castTo = usePlayer((s) => s.castTo);
  const { primary, more, personal } = useVisibleNav();
  const notes = useNotifications();
  const unread = (notes.data ?? []).filter((n) => !n.read).length;
  const scrolled = useScrolled(12);
  const section = sectionFor(pathname, anime);
  const moreMenu = useMenu();
  const desktop = useIsDesktop();

  const capsule = useRef<HTMLElement>(null);
  const [lens, setLens] = useState<{ x: number; w: number } | null>(null);
  useRefraction(capsule, { scale: 64 });
  useSheen(capsule);

  const inMore = [...more, ...personal, ...(desktop ? [] : primary.filter((i) => i.id === 'anime'))].find((i) => i.id === section);
  const activeId = inMore ? 'more' : section;

  useLayoutEffect(() => {
    const el = capsule.current;
    if (!el) return;
    const measure = () => {
      const a = el.querySelector<HTMLElement>(`[data-nav="${activeId}"]`);
      setLens(a ? { x: a.offsetLeft, w: a.offsetWidth } : null);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [activeId, primary.length]);

  return (
    <header className={cls('topbar', scrolled && 'scrolled')}>
      <div className="topbar-scrim" aria-hidden="true" />
      <Link to="/" className="brand" aria-label="Portico home" viewTransition>
        <img src="/brand/portico-wordmark-mono-white.svg" alt="Portico" />
      </Link>
      <nav ref={capsule} className="nav-capsule glass glass-chrome refract" aria-label="Primary">
        {lens && <span className="nav-lens" style={{ width: lens.w, transform: `translateX(${lens.x}px)` }} aria-hidden="true" />}
        {primary.map((item) => (
          <Link
            key={item.id}
            to={item.to}
            data-nav={item.id}
            className={cls('nav-item', item.id === 'anime' && 'hide-tablet', activeId === item.id && 'on')}
            aria-current={activeId === item.id ? 'page' : undefined}
            viewTransition
          >
            {item.label}
          </Link>
        ))}
        <button data-nav="more" className={cls('nav-item', activeId === 'more' && 'on')} onClick={moreMenu.open} aria-haspopup="menu" aria-expanded={moreMenu.isOpen}>
          {inMore ? inMore.label : 'More'}
          <ChevronDown className="nav-caret" />
        </button>
      </nav>
      <Menu
        anchor={moreMenu.anchor}
        open={moreMenu.isOpen}
        onClose={moreMenu.close}
        align="center"
        width={250}
        title="More"
        items={[
          ...(desktop ? [] : primary.filter((i) => i.id === 'anime').map((i) => ({ label: i.label, icon: <i.icon />, checked: section === i.id, onSelect: () => navigate(i.to, { viewTransition: true }) }))),
          { heading: 'Libraries' },
          ...more.map((i) => ({ label: i.label, icon: <i.icon />, checked: section === i.id, onSelect: () => navigate(i.to, { viewTransition: true }) })),
          { separator: true, label: '' },
          { heading: 'Yours' },
          ...personal.map((i) => ({ label: i.label, icon: <i.icon />, checked: section === i.id, onSelect: () => navigate(i.to, { viewTransition: true }) })),
        ]}
      />
      <div className="topbar-actions">
        <button className="search-pill glass" onClick={() => openPalette()} aria-label="Search (Command K)">
          <Search />
          <span className="label">Search</span>
          <span className="keys">
            <Kbd>⌘</Kbd>
            <Kbd>K</Kbd>
          </span>
        </button>
        <IconButton label={castTo ? 'Casting' : 'Cast to a device'} onClick={() => setCast(true)} active={!!castTo}>
          <Cast />
        </IconButton>
        <IconButton label={`Notifications${unread ? `, ${unread} unread` : ''}`} onClick={() => setNotifications(true)} count={unread}>
          <Bell />
        </IconButton>
        <ProfileButton />
      </div>
    </header>
  );
}
