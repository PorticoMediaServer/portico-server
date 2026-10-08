import React, {Suspense, lazy, useEffect, useState} from 'react';
import {Link, Outlet, useLocation, useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../data/hooks';
import type {Library, LibraryKind} from '../data/types';
import {useSession} from '../app/session';
import {usePrefs} from '../app/prefs';
import {usePlayer} from '../app/player';
import {Avatar, BrandSymbol, Wordmark} from '../ui/Media';
import {Icon, type IconName} from '../ui/Icon';
import {Dialog, Menu, Tooltip} from '../ui/Overlay';
import {cx} from '../ui/cx';
import {useMediaQuery} from '../ui/useMediaQuery';
import {ConnectionBanner} from './ConnectionBanner';
import {ActionsHost} from '../screens/shared/actions';
import {MiniPlayer} from '../player/MiniPlayer';
import {DemoHost} from '../app/demo';
import {useTogether} from '../app/together';
import {SelectionBar} from '../screens/shared/SelectionBar';
import {ReminderWatcher} from '../app/Reminders';

const PlayerHost = lazy(() => import('../player/PlayerHost').then(m => ({default: m.PlayerHost})));
const ServerSwitcher = lazy(() => import('./ServerSwitcher').then(m => ({default: m.ServerSwitcher})));
const ProfileSwitcher = lazy(() => import('./ServerSwitcher').then(m => ({default: m.ProfileSwitcher})));

export const libraryIcon: Record<LibraryKind, IconName> = {movies: 'film', shows: 'tv', anime: 'sparkles', music: 'music', audiobooks: 'bookOpen', photos: 'photos', homevideos: 'video'};

export function useLibraries() {
  const services = useServices();
  return useQuery('libraries', () => services.catalog.libraries());
}

/** "/" anywhere outside a text field opens Search with the field focused. */
function useSearchShortcut() {
  const navigate = useNavigate();
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey || e.defaultPrevented) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || t.closest('[role=dialog],[role=menu],.player'))) return;
      e.preventDefault();
      const focus = () => document.querySelector<HTMLInputElement>('input[type=search]')?.focus();
      if (location.pathname === '/search') focus();
      else void navigate({to: '/search'}).then(() => requestAnimationFrame(focus));
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [navigate]);
}

/**
 * The authenticated frame. Desktop and laptop: a left rail that pushes the page (expanded with
 * labels, or collapsed to icons with tooltips). Tablet and phone widths: a top bar and a bottom tab
 * bar. The page's backdrop art runs full-bleed behind the translucent rail.
 */
export function Shell() {
  const compact = useMediaQuery('(max-width: 767px)');
  const tablet = useMediaQuery('(max-width: 1199px)');
  const {prefs, update} = usePrefs();
  const [tabletOpen, setTabletOpen] = useState(false);
  const collapsed = tablet ? !tabletOpen : prefs.railCollapsed;
  const player = usePlayer(s => ({active: s.active, kind: s.kind, minimized: s.minimized}), true);
  const mini = player.active && (player.kind === 'audio' || player.kind === 'book' || player.minimized);
  useSearchShortcut();
  const loc = useLocation();
  useEffect(() => {
    if (tablet) setTabletOpen(false);
  }, [loc.pathname, tablet]);
  return (
    <div className={cx('shell', compact ? 'is-compact' : collapsed ? 'is-collapsed' : 'is-expanded', mini && 'has-mini')}>
      <a className="skip-link" href="#main">Skip to content</a>
      {!compact ? <Rail collapsed={collapsed} onToggle={() => (tablet ? setTabletOpen(o => !o) : update({railCollapsed: !prefs.railCollapsed}))} /> : null}
      <div className="shell-main">
        {compact ? <TopBar /> : null}
        <ConnectionBanner />
        <main id="main" className="shell-content" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
      {compact ? <BottomBar /> : null}
      {mini ? <MiniPlayer /> : null}
      <Suspense fallback={null}><PlayerHost /></Suspense>
      <SelectionBar />
      <ActionsHost />
      <DemoHost />
      <ReminderWatcher />
    </div>
  );
}

function RailLink({to, params, icon, label, collapsed, exact, badge, count, active, search}: {to: string; params?: Record<string, string>; icon: IconName; label: string; collapsed: boolean; exact?: boolean; badge?: React.ReactNode; count?: number; active?: boolean; search?: Record<string, unknown>}) {
  const link = (
    <Link
      to={to as never}
      params={params as never}
      search={(search ?? {}) as never}
      className="rail-item"
      activeOptions={{exact: !!exact || active !== undefined, includeSearch: false}}
      {...(active !== undefined ? {'data-status': active ? 'active' : undefined, 'aria-current': active ? ('page' as const) : undefined} : {})}
      aria-label={collapsed ? (count ? `${label}, ${count} unread` : label) : undefined}
    >
      <span className="rail-icon"><Icon name={icon} size={20} /></span>
      <span className="rail-label">{label}</span>
      {badge}
      {count ? <span className="rail-count" aria-hidden>{count > 99 ? '99+' : count}</span> : null}
    </Link>
  );
  return collapsed ? <Tooltip label={label} side="right">{link}</Tooltip> : link;
}

function useUnread() {
  const services = useServices();
  const q = useQuery('notifications', () => services.account.notifications());
  return (q.data ?? []).filter(n => !n.read && !n.archived).length;
}

function Rail({collapsed, onToggle}: {collapsed: boolean; onToggle: () => void}) {
  const session = useSession();
  const libraries = useLibraries();
  const services = useServices();
  const channels = useQuery('live.channels', () => services.live.channels());
  const unread = useUnread();
  const together = useTogether();
  const pathname = useLocation().pathname;
  const sources = [...new Set((channels.data ?? []).filter(c => !c.library).map(c => c.source))];
  const kids = session.profile?.kids;
  const liveAllowed = !kids && session.profile?.limits.liveTv !== false;
  return (
    <nav className="rail" aria-label="Primary">
      <div className="rail-top">
        <Link to="/" className="rail-brand" aria-label="Portico home">
          {collapsed ? <BrandSymbol size={30} /> : <Wordmark height={20} />}
        </Link>
      </div>
      <div className="rail-scroll">
        <div className="rail-group">
          <RailLink to="/" icon="home" label="Home" exact collapsed={collapsed} />
          <RailLink to="/search" icon="search" label="Search" collapsed={collapsed} />
          <RailLink to="/saved" icon="bookmark" label="Saved" collapsed={collapsed} />
          {session.profile?.limits.downloads !== false ? <RailLink to="/downloads" icon="download" label="Downloads" collapsed={collapsed} /> : null}
          {session.profile?.limits.together !== false ? <RailLink to="/together" icon="people" label="Watch Together" collapsed={collapsed} badge={together.inGroup ? <span className="rail-live" aria-label="In a group">{collapsed ? '' : 'Live'}</span> : undefined} /> : null}
        </div>
        <div className="rail-heading">{collapsed ? <span className="rail-rule" /> : 'Libraries'}</div>
        <div className="rail-group">
          {(libraries.data ?? []).map((l: Library) => (
            <RailLink key={l.id} to="/library/$libraryId" params={{libraryId: l.id}} icon={libraryIcon[l.kind]} label={l.name} collapsed={collapsed} badge={l.scanning ? <span className="rail-scan" title="Scanning for new files" aria-label="Scanning"><span /></span> : undefined} />
          ))}
          {libraries.loading && !libraries.data ? [0, 1, 2, 3].map(i => <span key={i} className="rail-item is-skeleton"><span className="skeleton" style={{width: 20, height: 20, borderRadius: 6}} />{collapsed ? null : <span className="skeleton" style={{width: 90, height: 12}} />}</span>) : null}
        </div>
        {liveAllowed && sources.length ? (
          <>
            <div className="rail-heading">{collapsed ? <span className="rail-rule" /> : 'Channels'}</div>
            <div className="rail-group">
              <RailLink to="/channels/$entry" params={{entry: 'all'}} icon="live" label="All channels" collapsed={collapsed} active={pathname === '/channels' || pathname === '/channels/all'} />
              {sources.map(src => <RailLink key={src} to="/channels/$entry" params={{entry: src.toLowerCase().replace(/[^a-z0-9]+/g, '-')}} icon={src.includes('Antenna') ? 'tv' : 'globe'} label={src} collapsed={collapsed} />)}
              <RailLink to="/channels/$entry" params={{entry: 'library'}} icon="channels" label="Library Channels" collapsed={collapsed} />
              {session.profile?.limits.dvr !== false ? <RailLink to="/channels/$entry" params={{entry: 'recordings'}} icon="record" label="Recordings" collapsed={collapsed} /> : null}
            </div>
          </>
        ) : null}
      </div>
      <div className="rail-foot">
        {session.manager ? <RailLink to="/server/$section" params={{section: 'overview'}} icon="server" label="Manage server" collapsed={collapsed} active={pathname.startsWith('/server')} /> : null}
        <RailLink to="/settings/$section" params={{section: 'notifications'}} icon="bell" label="Notifications" collapsed={collapsed} count={unread} />
        <RailLink to="/settings" icon="settings" label="Settings" collapsed={collapsed} active={pathname.startsWith('/settings') && !pathname.startsWith('/settings/notifications')} />
        <ProfileMenu collapsed={collapsed} />
        <button type="button" className="rail-toggle" onClick={onToggle} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-expanded={!collapsed}>
          <Icon name={collapsed ? 'expand' : 'collapse'} size={18} />
          {collapsed ? null : <span>Collapse</span>}
        </button>
      </div>
    </nav>
  );
}

/** Says who and where you are, and switches profile or server, account, feedback, help, sign out. */
export function ProfileMenu({collapsed, compact}: {collapsed?: boolean; compact?: boolean}) {
  const session = useSession();
  const navigate = useNavigate();
  const [switching, setSwitching] = useState<'profile' | 'server' | null>(null);
  const [feedback, setFeedback] = useState(false);
  const name = session.profile?.name ?? 'Profile';
  const where = [session.server?.name, session.via === 'account' ? 'justin' : 'Server account'].filter(Boolean).join(' · ');
  const trigger = compact ? (
    <button type="button" className="topbar-avatar" aria-label={`${name}: profile and account`}><Avatar name={name} color={session.profile?.color} size={30} /></button>
  ) : (
    <button type="button" className="rail-item rail-profile" aria-label={collapsed ? `${name}: profile and account` : undefined}>
      <span className="rail-icon"><Avatar name={name} color={session.profile?.color} size={26} /></span>
      <span className="rail-label">{name}<span className="rail-sub">{session.server?.name}</span></span>
      {collapsed ? null : <Icon name="chevronUp" size={16} className="rail-caret" />}
    </button>
  );
  return (
    <>
      <Menu
        label="Profile and account"
        align={compact ? 'end' : 'start'}
        side={compact ? 'bottom' : 'top'}
        header={<><Avatar name={name} color={session.profile?.color} size={42} /><span className="menu-who"><strong>{name}</strong><span>{where}</span></span></>}
        trigger={trigger}
        items={[
          {id: 'switch', label: 'Switch profile', icon: 'people', onSelect: () => setSwitching('profile')},
          {id: 'server', label: 'Switch server', icon: 'server', onSelect: () => setSwitching('server')},
          {id: 'account', label: 'Account', icon: 'account', onSelect: () => void navigate({to: '/account'})},
          ...(compact ? [{id: 'settings', label: 'Settings', icon: 'settings' as IconName, onSelect: () => void navigate({to: '/settings'})}] : []),
          {id: 'feedback', label: 'Send feedback', icon: 'message', separatorBefore: true, onSelect: () => setFeedback(true)},
          {id: 'help', label: 'Help', icon: 'help', hint: '↗', onSelect: () => window.open('https://getportico.tv/help', '_blank', 'noopener')},
          {id: 'signout', label: 'Sign out', icon: 'signOut', separatorBefore: true, onSelect: () => { session.signOut(); void navigate({to: '/signin'}); }},
        ]}
      />
      {switching === 'server' ? <Suspense fallback={null}><ServerSwitcher onClose={() => setSwitching(null)} /></Suspense> : null}
      {switching === 'profile' ? <Suspense fallback={null}><ProfileSwitcher onClose={() => setSwitching(null)} /></Suspense> : null}
      {feedback ? <FeedbackLazy onClose={() => setFeedback(false)} /> : null}
    </>
  );
}

const FeedbackDialog = lazy(() => import('../screens/shared/FeedbackDialog').then(m => ({default: m.FeedbackDialog})));
function FeedbackLazy({onClose}: {onClose: () => void}) {
  return <Suspense fallback={null}><FeedbackDialog open onOpenChange={o => !o && onClose()} /></Suspense>;
}

function TopBar() {
  const unread = useUnread();
  const navigate = useNavigate();
  const session = useSession();
  return (
    <header className="topbar">
      <Link to="/" aria-label="Portico home" className="topbar-brand"><Wordmark height={17} /></Link>
      <div className="topbar-actions">
        {session.profile?.limits.downloads !== false ? <button type="button" className="topbar-btn" aria-label="Downloads" onClick={() => void navigate({to: '/downloads'})}><Icon name="download" size={21} /></button> : null}
        <button type="button" className="topbar-btn" aria-label={unread ? `Notifications, ${unread} unread` : 'Notifications'} onClick={() => void navigate({to: '/settings/$section', params: {section: 'notifications'}})}>
          <Icon name="bell" size={21} />{unread ? <span className="topbar-dot" aria-hidden /> : null}
        </button>
        <ProfileMenu compact />
      </div>
    </header>
  );
}

function BottomBar() {
  const libraries = useLibraries();
  const pathname = useLocation().pathname;
  const navigate = useNavigate();
  const [picker, setPicker] = useState(false);
  const session = useSession();
  const live = !session.profile?.kids && session.profile?.limits.liveTv !== false;
  const tab = (to: string, icon: IconName, label: string, active: boolean, onClick?: () => void) => (
    onClick ? (
      <button type="button" className={cx('tabbar-item', active && 'is-on')} aria-current={active ? 'page' : undefined} onClick={onClick}><Icon name={icon} size={22} /><span>{label}</span></button>
    ) : (
      <Link to={to as never} className={cx('tabbar-item', active && 'is-on')} aria-current={active ? 'page' : undefined}><Icon name={icon} size={22} /><span>{label}</span></Link>
    )
  );
  return (
    <>
      <nav className="tabbar" aria-label="Primary" style={{gridTemplateColumns: `repeat(${live ? 5 : 4}, 1fr)`}}>
        {tab('/', 'home', 'Home', pathname === '/')}
        {tab('', 'library', 'Library', pathname.startsWith('/library') || pathname.startsWith('/media') || pathname.startsWith('/show'), () => setPicker(true))}
        {live ? tab('/channels', 'live', 'Channels', pathname.startsWith('/channels')) : null}
        {tab('/saved', 'bookmark', 'Saved', pathname.startsWith('/saved'))}
        {tab('/search', 'search', 'Search', pathname.startsWith('/search'))}
      </nav>
      <Dialog open={picker} onOpenChange={setPicker} title="Libraries">
        <div className="library-picker">
          {(libraries.data ?? []).map(l => (
            <button key={l.id} type="button" className={cx('library-pick', pathname === `/library/${l.id}` && 'is-on')} onClick={() => { setPicker(false); void navigate({to: '/library/$libraryId', params: {libraryId: l.id}}); }}>
              <span className="row-icon"><Icon name={libraryIcon[l.kind]} size={20} /></span>
              <span className="library-pick-text"><strong>{l.name}</strong><span>{l.count.toLocaleString('en-US')} {l.kind === 'music' ? 'artists' : l.kind === 'photos' ? 'albums' : 'titles'}{l.scanning ? ' · Scanning' : ''}</span></span>
              <Icon name="chevronRight" size={18} className="row-chevron" />
            </button>
          ))}
          <button type="button" className="library-pick" onClick={() => { setPicker(false); void navigate({to: '/together'}); }}>
            <span className="row-icon"><Icon name="people" size={20} /></span>
            <span className="library-pick-text"><strong>Watch Together</strong><span>Watch in step with friends</span></span>
            <Icon name="chevronRight" size={18} className="row-chevron" />
          </button>
          {session.manager ? (
            <button type="button" className="library-pick" onClick={() => { setPicker(false); void navigate({to: '/server/$section', params: {section: 'overview'}}); }}>
              <span className="row-icon"><Icon name="server" size={20} /></span>
              <span className="library-pick-text"><strong>Manage server</strong><span>{session.server?.name}</span></span>
              <Icon name="chevronRight" size={18} className="row-chevron" />
            </button>
          ) : null}
        </div>
      </Dialog>
    </>
  );
}
