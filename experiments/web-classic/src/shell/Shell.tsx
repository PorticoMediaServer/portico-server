import React, {lazy, Suspense, useEffect, useState} from 'react';
import {Link, Outlet, useLocation, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery, useTopic} from '@/data/DataProvider';
import {useSession} from '@/app/session';
import {usePrefs} from '@/app/prefs';
import {libraryIcon, libraryKindLabel} from '@/app/libraryMeta';
import {useActions} from '@/app/actions';
import {usePlayerState} from '@/player/PlayerProvider';
import {Avatar, Kbd} from '@/ui/Bits';
import {Dialog, Menu, Tooltip, type MenuItem} from '@/ui/Overlay';
import {Icon, type IconName} from '@/ui/Icon';
import {useMediaQuery, usePhone} from '@/ui/hooks';
import {cx} from '@/ui/cx';
import {ProfileSwitcher, ServerSwitcher} from './Switchers';
import {ConnectionBanner} from './ConnectionBanner';
import {ShortcutsDialog} from './Shortcuts';
import {SelectionBar} from './SelectionBar';
import {DemoRunner} from './DemoRunner';
import {ReminderWatcher} from './ReminderWatcher';
import {useDemo} from '@/app/demo';
import {SOURCES} from '@/app/liveSources';
import s from './Shell.module.css';

const PlayerHost = lazy(() => import('@/player/PlayerHost'));


/**
 * The signed-in frame. A persistent sidebar with the libraries on laptops and
 * desktops (collapsible to icons; collapsed by default below 1280 px, where
 * expanding pushes the page), a top bar plus a bottom tab bar on phones. The
 * player, mini player and selection bar dock above the content.
 */
export function Shell() {
  const phone = usePhone();
  const path = useLocation().pathname;
  // Manage server has its own section list, so the sidebar steps back to icons there unless the screen is very wide.
  const roomy = useMediaQuery('(min-width: 1600px)');
  const narrow = useMediaQuery('(max-width: 1279px)') || (path.startsWith('/server') && !roomy);
  const {prefs, update} = usePrefs();
  const [narrowOpen, setNarrowOpen] = useState(false);
  useEffect(() => { setNarrowOpen(false); }, [narrow]);
  const collapsed = narrow ? !narrowOpen : prefs.sidebar === 'collapsed';
  const player = usePlayerState();
  const dock = player.mode === 'music' || player.mode === 'book' ? !player.expanded : false;
  useGlobalShortcuts();
  const [shortcuts, setShortcuts] = useState(false);
  useDemo('shortcuts', () => setShortcuts(true));
  useEffect(() => {
    const on = (e: KeyboardEvent) => { if (e.key === '?' && !isTyping(e)) { e.preventDefault(); setShortcuts(v => !v); } };
    window.addEventListener('keydown', on);
    return () => window.removeEventListener('keydown', on);
  }, []);
  return (
    <div className={cx(s.frame, collapsed && s.collapsed)} style={{'--dock-h': dock ? 'var(--miniplayer-h)' : '0px'} as React.CSSProperties}>
      <a href="#main" className="skip-link">Skip to content</a>
      {phone ? <TopBar onShortcuts={() => setShortcuts(true)} /> : <Sidebar collapsed={collapsed} onToggle={() => (narrow ? setNarrowOpen(!narrowOpen) : update({sidebar: collapsed ? 'expanded' : 'collapsed'}))} onShortcuts={() => setShortcuts(true)} />}
      <div className={s.main}>
        <ConnectionBanner />
        <main id="main" className={s.content} tabIndex={-1}>
          <Outlet />
        </main>
      </div>
      {phone ? <BottomBar /> : null}
      <SelectionBar />
      <Suspense fallback={null}>{player.mode !== 'none' ? <PlayerHost /> : null}</Suspense>
      <ShortcutsDialog open={shortcuts} onOpenChange={setShortcuts} />
      <DemoRunner />
      <ReminderWatcher />
    </div>
  );
}

export function isTyping(e: KeyboardEvent): boolean {
  const t = e.target as HTMLElement | null;
  return !!t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
}

/** "/" opens Search from anywhere; "g" then a letter jumps to a destination (g h Home, g s Saved, g l Live TV, g t Settings). */
function useGlobalShortcuts() {
  const navigate = useNavigate();
  useEffect(() => {
    let g = 0;
    const on = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e) || e.defaultPrevented) return;
      if ((e.target as HTMLElement).closest?.('[role=dialog],[role=menu]')) return;
      if (e.key === '/') { e.preventDefault(); void navigate({to: '/search' as never}).then(() => requestAnimationFrame(() => document.querySelector<HTMLInputElement>('input[type=search]')?.focus())); return; }
      if (e.key === 'g') { g = Date.now(); return; }
      if (Date.now() - g < 1200) {
        const to = ({h: '/', s: '/saved', l: '/channels', t: '/settings', d: '/downloads', w: '/together', m: '/server/overview'} as Record<string, string>)[e.key];
        if (to) { e.preventDefault(); void navigate({to: to as never}); }
        g = 0;
      }
    };
    window.addEventListener('keydown', on);
    return () => window.removeEventListener('keydown', on);
  }, [navigate]);
}

function useLibraries() {
  return useQuery(c => c.libraries.list(), [], ['libraries', 'session']);
}

function NavItem({to, icon, label, collapsed, count, soft, exact, active, extra}: {to: string; icon: IconName; label: string; collapsed: boolean; count?: number; soft?: boolean; exact?: boolean; active?: boolean; extra?: React.ReactNode}) {
  const path = useLocation().pathname;
  const on = active ?? (exact ? path === to : path === to || path.startsWith(`${to}/`));
  const link = (
    <Link to={to as never} className={cx(s.item, on && s.on)} activeOptions={{exact: true, includeSearch: false}} activeProps={{}} aria-current={on ? 'page' : undefined} aria-label={collapsed ? (count ? `${label}, ${count} new` : label) : undefined}>
      <Icon name={icon} size={20} />
      <span className={s.label}>{label}</span>
      {extra}
      {count ? <span className={cx(s.count, soft && s.countSoft)} aria-hidden>{count > 99 ? '99+' : count}</span> : null}
    </Link>
  );
  return collapsed ? <Tooltip label={label} side="right">{link}</Tooltip> : link;
}

function Sidebar({collapsed, onToggle, onShortcuts}: {collapsed: boolean; onToggle: () => void; onShortcuts: () => void}) {
  const session = useSession();
  const libs = useLibraries();
  const client = useClient();
  useTopic(['notifications', 'together']);
  useTopic(['prefs']);
  const unread = client.prefs.get('notifications.badges') === false ? 0 : client.notifications.unread();
  const inGroup = client.together.current();
  const navigate = useNavigate();
  const path = useLocation().pathname;
  const limits = session.profile?.limits;
  return (
    <nav className={s.rail} aria-label="Primary">
      <Link to="/" className={s.brand} aria-label="Portico home">
        <img src="/brand/portico-symbol-dark.svg" alt="" className={s.symbol} />
        <img src="/brand/portico-wordmark-dark.svg" alt="" className={s.wordmark} />
      </Link>
      <button type="button" className={s.searchBtn} onClick={() => void navigate({to: '/search' as never})} aria-label="Search (press /)">
        <Icon name="search" size={18} /><span>Search</span><Kbd>/</Kbd>
      </button>
      <div className={s.scroll}>
        {inGroup ? <Link to="/together" className={s.groupPill}><Icon name="together" size={18} /><span>In {inGroup.name}</span></Link> : null}
        <NavItem to="/" icon="home" label="Home" collapsed={collapsed} exact />
        <NavItem to="/saved" icon="saved" label="Saved" collapsed={collapsed} />
        {limits?.downloads === false ? null : <NavItem to="/downloads" icon="download" label="Downloads" collapsed={collapsed} />}
        {limits?.together === false ? null : <NavItem to="/together" icon="together" label="Watch Together" collapsed={collapsed} />}
        <div className={s.heading}>Libraries</div>
        {libs.data ? libs.data.map(l => (
          <NavItem key={l.id} to={`/library/${l.id}`} icon={libraryIcon(l.kind)} label={l.name} collapsed={collapsed} extra={l.scanning ? <span className={s.scan} title={`Scanning, ${l.scanning.found} files found`}>{l.scanning.found}</span> : null} />
        )) : Array.from({length: 6}, (_, i) => <div key={i} className="skeleton" style={{height: 30, margin: '4px 6px', borderRadius: 8}} />)}
        {limits?.liveTv === false ? null : (
          <>
            <Link to="/channels" className={cx(s.heading, s.headingLink)}>Live TV</Link>
            {SOURCES.map(src => <NavItem key={src.id} to={`/channels/${src.id}`} icon={src.icon} label={src.name} collapsed={collapsed} active={path === `/channels/${src.id}` || (src.id === 'all' && path === '/channels')} />)}
            {limits?.dvr === false ? null : <NavItem to="/channels/recordings" icon="recordings" label="Recordings" collapsed={collapsed} />}
          </>
        )}
      </div>
      <div className={s.foot}>
        {session.isAdmin ? <NavItem to="/server/overview" icon="console" label="Manage server" collapsed={collapsed} active={path.startsWith('/server')} /> : null}
        <NavItem to="/settings/notifications" icon="bell" label="Notifications" collapsed={collapsed} count={unread} />
        <NavItem to="/settings" icon="settings" label="Settings" collapsed={collapsed} active={path.startsWith('/settings') && !path.startsWith('/settings/notifications')} />
        <ProfileMenu collapsed={collapsed} onShortcuts={onShortcuts} />
      </div>
      <button type="button" className={s.toggle} onClick={onToggle} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}><Icon name={collapsed ? 'forward' : 'back'} size={14} /></button>
    </nav>
  );
}

/** Who and where you are, with switch profile, switch server, account, feedback, help and sign out. */
function ProfileMenu({collapsed, phone, onShortcuts}: {collapsed?: boolean; phone?: boolean; onShortcuts: () => void}) {
  const session = useSession();
  const navigate = useNavigate();
  const actions = useActions();
  const [sheet, setSheet] = useState<'profile' | 'server'>();
  const [menuOpen, setMenuOpen] = useState(false);
  useDemo(['profile-menu', 'profile-switcher', 'server-switcher'], name => (name === 'profile-menu' ? setMenuOpen(true) : setSheet(name === 'profile-switcher' ? 'profile' : 'server')));
  const p = session.profile;
  const where = `${session.server?.name ?? ''} · ${session.session?.account.username ?? ''}`;
  const items: MenuItem[] = [
    {id: 'profile', label: 'Switch profile…', icon: 'switch', onSelect: () => setSheet('profile')},
    {id: 'server', label: 'Switch server…', icon: 'server', onSelect: () => setSheet('server')},
    {id: 'account', label: 'Portico Account', icon: 'user', onSelect: () => void navigate({to: '/account' as never})},
    ...(phone ? [
      {id: 'settings', label: 'Settings', icon: 'settings' as IconName, onSelect: () => void navigate({to: '/settings' as never})},
      ...(session.isAdmin ? [{id: 'admin', label: 'Manage server', icon: 'console' as IconName, onSelect: () => void navigate({to: '/server/overview' as never})}] : []),
      {id: 'downloads', label: 'Downloads', icon: 'download' as IconName, onSelect: () => void navigate({to: '/downloads' as never})},
      {id: 'together', label: 'Watch Together', icon: 'together' as IconName, onSelect: () => void navigate({to: '/together' as never})},
    ] : []),
    {id: 'feedback', label: 'Send feedback…', icon: 'flag', separatorBefore: true, onSelect: () => actions.dialog({kind: 'feedback'})},
    ...(phone ? [] : [{id: 'keys', label: 'Keyboard shortcuts', icon: 'keyboard' as IconName, hint: '?', onSelect: onShortcuts}]),
    {id: 'help', label: 'Help and support', icon: 'external', onSelect: () => window.open('https://getportico.tv/help', '_blank', 'noopener')},
    {id: 'signOut', label: 'Sign out', icon: 'signOut', separatorBefore: true, onSelect: () => void session.signOut().then(() => navigate({to: '/welcome' as never}))},
  ];
  const header = p ? <><Avatar name={p.name} art={p.art} size={40} kids={p.kids} /><span style={{minWidth: 0}}><span className="t-strong" style={{display: 'block'}}>{p.name}</span><span className="t-caption c-3 clamp-1" style={{display: 'block'}}>{where}</span></span></> : null;
  const trigger = phone
    ? <button type="button" className={s.iconBtn} aria-label={`${p?.name ?? 'Profile'}: profile and settings`}><Avatar name={p?.name ?? '?'} art={p?.art} size={28} /></button>
    : (
      <button type="button" className={s.profileBtn} aria-label={`${p?.name ?? 'Profile'}, ${where}. Profile menu`}>
        <Avatar name={p?.name ?? '?'} art={p?.art} size={32} kids={p?.kids} />
        <span className={s.profileText}><span className={s.profileName}>{p?.name}</span><span className={s.profileWhere}>{session.server?.name}</span></span>
        <Icon name="chevronUp" size={16} className={cx(s.profileChevron, 'c-3')} />
      </button>
    );
  return (
    <>
      <Menu label="Profile" open={menuOpen} onOpenChange={setMenuOpen} trigger={collapsed ? <span style={{display: 'block'}}>{trigger}</span> : trigger} items={items} header={header} align={phone ? 'end' : 'start'} side={phone ? 'bottom' : 'top'} />
      <ProfileSwitcher open={sheet === 'profile'} onClose={() => setSheet(undefined)} />
      <ServerSwitcher open={sheet === 'server'} onClose={() => setSheet(undefined)} />
    </>
  );
}

function TopBar({onShortcuts}: {onShortcuts: () => void}) {
  const client = useClient();
  useTopic(['notifications', 'prefs']);
  const unread = client.prefs.get('notifications.badges') === false ? 0 : client.notifications.unread();
  return (
    <header className={s.topbar}>
      <Link to="/" className={s.topbarBrand} aria-label="Portico home">
        <img src="/brand/portico-symbol-dark.svg" alt="" className={s.symbol} />
        <img src="/brand/portico-wordmark-dark.svg" alt="" className={s.wordmark} />
      </Link>
      <Link to="/settings/notifications" className={s.iconBtn} aria-label={unread ? `Notifications, ${unread} unread` : 'Notifications'}><Icon name="bell" size={21} />{unread ? <span className={s.dot} /> : null}</Link>
      <ProfileMenu phone onShortcuts={onShortcuts} />
    </header>
  );
}

function BottomBar() {
  const path = useLocation().pathname;
  const [drawer, setDrawer] = useState(false);
  const libs = useLibraries();
  const session = useSession();
  const inLibrary = path.startsWith('/library') || path.startsWith('/media') || path.startsWith('/show');
  const tab = (to: string, icon: IconName, label: string, active: boolean) => (
    <Link to={to as never} className={s.tab} data-status={active ? 'active' : undefined} aria-current={active ? 'page' : undefined}><Icon name={icon} size={22} />{label}</Link>
  );
  return (
    <>
      <nav className={s.bottombar} aria-label="Primary">
        {tab('/', 'home', 'Home', path === '/')}
        <button type="button" className={s.tab} data-status={inLibrary ? 'active' : undefined} onClick={() => setDrawer(true)} aria-haspopup="dialog"><Icon name="gridView" size={22} />Libraries</button>
        {session.profile?.limits?.liveTv === false ? tab('/together', 'together', 'Together', path.startsWith('/together')) : tab('/channels', 'live', 'Live TV', path.startsWith('/channels'))}
        {tab('/saved', 'saved', 'Saved', path.startsWith('/saved'))}
        {tab('/search', 'search', 'Search', path.startsWith('/search'))}
      </nav>
      <Dialog open={drawer} onOpenChange={setDrawer} title="Libraries">
        <div className={s.drawerList}>
          {libs.data?.map(l => (
            <Link key={l.id} to={`/library/${l.id}` as never} className={s.drawerItem} onClick={() => setDrawer(false)} aria-current={path.startsWith(`/library/${l.id}`) ? 'page' : undefined}>
              <span className={s.drawerIcon}><Icon name={libraryIcon(l.kind)} size={19} /></span>
              <span>{l.name}</span>
              <span className="t-caption c-3">{l.name.toLowerCase() === libraryKindLabel(l.kind).toLowerCase() ? '' : libraryKindLabel(l.kind)}</span>
            </Link>
          ))}
          <div className={s.drawerHeading}>Live TV</div>
          {SOURCES.map(src => (
            <Link key={src.id} to={`/channels/${src.id}` as never} className={s.drawerItem} onClick={() => setDrawer(false)}>
              <span className={s.drawerIcon}><Icon name={src.icon} size={19} /></span><span>{src.name}</span>
            </Link>
          ))}
          <Link to={'/channels/recordings' as never} className={s.drawerItem} onClick={() => setDrawer(false)}><span className={s.drawerIcon}><Icon name="recordings" size={19} /></span><span>Recordings</span></Link>
        </div>
      </Dialog>
    </>
  );
}
