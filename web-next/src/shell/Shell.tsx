import {useEffect, useRef, useState, type ComponentType, type ReactNode} from 'react';
import {Link, useNavigate, useRouterState} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {Library, Viewer} from '@core/index.ts';
import {AccountMenu, ProfileAvatar, ProfileAvatarFace, type ProfileIdentity, type ProfileTone} from '../kit/shell';
import type {FactPart} from '@core/presentation/kit-model/index.ts';
import {Icon} from '../kit/Icon';
import {Menu, MenuItem} from '../kit/overlays';
import {desktopLibraries, isLibraryLocation, rememberLibrary, rememberedLibrary, showOwnerActivity} from './model';
import {useShellScrolled} from './viewport';
import {useSizeClass, useCoarsePointer, useWindowScrollAnchor} from '../kit/theme';
import styles from './shell.module.css';
import wordmark from './wordmark.svg';
import type {OwnerActivityProps} from './Activity';
import {LibraryNavigation} from './LibraryNavigation';

export type ShellIdentity = Readonly<{
  viewer: Viewer; username?: string; profileName: string; profileArt?: ProfileTone; serverName: string;
  primaryProfile?: boolean; profiles: readonly ProfileIdentity[]; canManageProfiles: boolean; accountFacts: readonly FactPart[];
  libraries: readonly Library[]; liveAvailable: boolean; unread: number; showNotificationDot: boolean;
}>;
export type ShellProps = Readonly<{
  identity: ShellIdentity;
  /** Mount owns its reads. Compact and non-owner shells do not mount it. */
  OwnerActivity: ComponentType<OwnerActivityProps>;
  dockVisible: boolean;
  onSignOut: () => void;
  overlays?: ReactNode;
  phoneSelection?: ReactNode;
  children: ReactNode;
}>;
const t = defaultI18n.t;
function libraryMemory() { try { return window.localStorage; } catch { return undefined; } }
const collapseMemoryKey = 'portico.next.sidebar-collapsed';

export function Shell({identity, OwnerActivity, dockVisible, onSignOut, phoneSelection, overlays, children}: ShellProps) {
  useWindowScrollAnchor();
  const sizeClass = useSizeClass(), coarsePointer = useCoarsePointer();
  const desktop = sizeClass === 'expanded' && !coarsePointer, compact = sizeClass === 'compact', scrolled = useShellScrolled();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return window.localStorage.getItem(collapseMemoryKey) === 'true'; } catch { return false; }
  });
  const toggleSidebar = () => setSidebarCollapsed(value => {
    try { window.localStorage.setItem(collapseMemoryKey, String(!value)); } catch { /* The live choice still works. */ }
    return !value;
  });
  const navigation = compact ? 'tabs' : desktop ? 'topbar' : sizeClass === 'medium' || sidebarCollapsed ? 'rail' : 'sidebar';
  const pathname = useRouterState({select: state => state.location.pathname});
  const query = useRouterState({select: state => state.location.search});
  const lastSearch = useRef('');
  if (pathname === '/search' && typeof query.q === 'string') lastSearch.current = query.q;
  const navigate = useNavigate();
  const home = pathname === '/', [accountOpen, setAccountOpen] = useState(false), [moreOpen, setMoreOpen] = useState(false);
  const {visible, overflow} = desktopLibraries(identity.libraries);
  const {viewer} = identity;
  useEffect(() => {
    if (!pathname.startsWith('/library/')) return;
    let id: string;
    try { id = decodeURIComponent(pathname.slice('/library/'.length).split('/')[0]); } catch { return; }
    rememberLibrary(libraryMemory(), viewer, identity.libraries, id);
  }, [pathname, viewer, identity.libraries]);
  useEffect(() => { setAccountOpen(false); setMoreOpen(false); }, [pathname]);
  const openLibraries = () => {
    const id = rememberedLibrary(libraryMemory(), viewer, identity.libraries);
    if (id) void navigate({to: '/library/$libraryId', params: {libraryId: id}});
    else void navigate({to: '/libraries'});
  };
  const current = {id: viewer.profileId, username: identity.username, name: identity.profileName, initial: Array.from(identity.profileName.trim())[0] ?? '', art: identity.profileArt};
  const dashboard = () => { void navigate({to: '/dashboard', search: viewer.role === 'owner' ? {} : {section: 'streams'}}); };
  const serverAction = !desktop && (viewer.role === 'owner' || viewer.role === 'admin' || viewer.role === 'administrator')
    ? <MenuItem className={styles.dashboardCard} onSelect={dashboard}>
      <span className={styles.dashboardIcon}><Icon name="i-pulse" size={22} /></span>
      <span className={styles.dashboardText}><small>{t('next.shell.server')}</small><b>{t('next.shell.dashboard')}</b></span>
      <Icon name="i-right" size={18} />
    </MenuItem> : undefined;
  const account = <AccountMenu open={accountOpen} onOpenChange={setAccountOpen} current={current} primary={identity.primaryProfile === true}
    notificationCount={desktop ? undefined : identity.unread} notificationDot={!desktop && identity.showNotificationDot && identity.unread > 0}
    profiles={identity.profiles} accountFacts={identity.accountFacts} serverAction={serverAction} actions={{
      manageProfiles: () => { void navigate({to: '/profiles'}); },
      ...(identity.canManageProfiles ? {addProfile: () => { void navigate({to: '/profiles', search: {add: true}}); }} : {}),
      switchProfile: profile => { void navigate({to: '/profiles', search: {profileId: profile.id}}); },
      switchServer: () => { void navigate({to: '/servers'}); }, settings: () => { void navigate({to: '/settings'}); },
      ...(!desktop ? {notifications: () => { void navigate({to: '/notifications'}); }} : {}),
      watchTogether: () => { void navigate({to: '/watch-together'}); }, feedback: () => { void navigate({to: '/feedback'}); },
      help: () => { void navigate({to: '/help'}); }, signOut: onSignOut,
    }} trigger={navigation === 'sidebar' ? <button type="button" className={styles.profileRow} aria-label={[t('web.account.switchProfileOrServer'), identity.username || t('next.shell.usernameUnavailable'), identity.serverName].join(', ')}>
      <ProfileAvatarFace initial={current.initial} art={identity.profileArt} size={34} />
      <span className={styles.profileText}><b>{identity.username || t('next.shell.usernameUnavailable')}</b><small>{identity.serverName}</small></span><Icon name="i-updown" size={18} />
    </button> : <ProfileAvatar className={styles.avatar} name={identity.profileName} initial={current.initial} art={identity.profileArt} size={desktop ? 34 : 30}
      notificationDot={compact && identity.showNotificationDot && identity.unread > 0} aria-label={t('next.home.shell.profileMenu', {name: identity.profileName})} />} />;
  return <div className={styles.shell} data-desktop={desktop} data-size-class={sizeClass} data-pointer={coarsePointer ? 'coarse' : 'fine'} data-navigation={navigation} data-has-dock={dockVisible}>
    <a className={styles.skip} href="#portico-content">{t('next.home.shell.skip')}</a>
    <div className={styles.navigation} data-shell-navigation>
      {desktop ? <nav className={styles.topnav} data-solid={scrolled || !home} aria-label={t('next.home.shell.main')}>
        <Link className={styles.brand} to="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></Link>
        <ul className={styles.destinations}>
          <li><Link to="/" activeProps={{'aria-current': 'page'}} activeOptions={{exact: true}}>{t('next.home.shell.home')}</Link></li>
          {visible.map(library => <li key={library.id}><Link to="/library/$libraryId" params={{libraryId: library.id}} activeProps={{'aria-current': 'page'}}>{library.name}</Link></li>)}
          {overflow.length > 0 && <li><Menu title={t('next.home.shell.libraries')} open={moreOpen} onOpenChange={setMoreOpen}
            trigger={<button type="button" className={styles.more} data-active={overflow.some(library => pathname === '/library/' + encodeURIComponent(library.id))}>{t('next.home.shell.more')}<Icon name="i-down" size={14} /></button>}
            items={overflow.map(library => ({id: library.id, label: library.name, selected: pathname === '/library/' + encodeURIComponent(library.id), onSelect: () => { void navigate({to: '/library/$libraryId', params: {libraryId: library.id}}); }}))} /></li>}
          {identity.liveAvailable && <li><Link to="/live" activeProps={{'aria-current': 'page'}}>{t('next.home.shell.live')}</Link></li>}
        </ul>
        <div className={styles.tools}>
          <Link className={styles.icon} to="/search" search={{q: lastSearch.current}} aria-label={t('next.home.shell.search')} activeProps={{'aria-current': 'page'}}><Icon name="i-search" size={22} /></Link>
          <Link className={styles.icon} to="/saved" aria-label={t('next.home.shell.saved')} activeProps={{'aria-current': 'page'}}><Icon name="i-bookmark" size={22} /></Link>
          <Link className={styles.icon} to="/notifications" aria-label={identity.unread > 0 ? t('next.home.shell.unread', {count: identity.unread}) : t('next.home.shell.notifications')} activeProps={{'aria-current': 'page'}}><Icon name="i-bell" size={22} />{identity.showNotificationDot && identity.unread > 0 && <span className={styles.dot} />}</Link>
          {showOwnerActivity(viewer.role, desktop) && <OwnerActivity />}
          {account}
        </div>
      </nav> : !compact ? <nav className={styles.rail} aria-label={t('next.home.shell.main')}>
        {sizeClass === 'expanded' && <div className={styles.railBrand}>
          {navigation === 'sidebar' && <Link className={styles.brand} to="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></Link>}
          <button type="button" className={styles.collapse} aria-label={t(sidebarCollapsed ? 'web.shell.expand' : 'web.shell.collapse')} aria-expanded={!sidebarCollapsed} onClick={toggleSidebar}><Icon name="i-panels" size={22} /></button>
        </div>}
        <div className={styles.railPrimary}>
          <Link className={styles.railItem} to="/search" search={{q: lastSearch.current}} activeProps={{'aria-current': 'page'}}><Icon name="i-search" size={22} /><span data-nav-label>{t('next.home.shell.search')}</span></Link>
          <Link className={styles.railItem} to="/" activeOptions={{exact: true}} activeProps={{'aria-current': 'page'}}><Icon name="i-home" size={22} /><span data-nav-label>{t('next.home.shell.home')}</span></Link>
        </div>
        <LibraryNavigation libraries={identity.libraries} sidebar={navigation === 'sidebar'} />
        <div className={styles.railFixed}>
          <div className={styles.railDivider} />
          {identity.liveAvailable && <Link className={styles.railItem} to="/live" activeProps={{'aria-current': 'page'}}><Icon name="i-live" size={22} /><span data-nav-label>{t('next.home.shell.live')}</span></Link>}
          <Link className={styles.railItem} to="/saved" activeProps={{'aria-current': 'page'}}><Icon name="i-bookmark" size={22} /><span data-nav-label>{t('next.home.shell.saved')}</span></Link>
        </div>
        <div className={styles.railBottom}>
          <Link className={styles.railItem} to="/notifications" data-icon-only={navigation === 'rail' || undefined} aria-label={identity.unread > 0 ? t('next.home.shell.unread', {count: identity.unread}) : t('next.home.shell.notifications')} activeProps={{'aria-current': 'page'}}><Icon name="i-bell" size={22} />{navigation === 'sidebar' && <span data-nav-label>{t('next.home.shell.notifications')}</span>}{identity.showNotificationDot && identity.unread > 0 && <span data-nav-count aria-hidden="true">{identity.unread}</span>}</Link>
          {viewer.role === 'owner' && <OwnerActivity presentation={navigation === 'sidebar' ? 'sidebar' : 'rail'} className={styles.railItem} />}
          <Link className={styles.railItem} to="/settings" activeProps={{'aria-current': 'page'}}><Icon name="i-settings" size={22} /><span data-nav-label>{t('next.shell.settings')}</span></Link>
          {account}
        </div>
      </nav> : <>
        {home && <header className={styles.phoneHeader}><Link className={styles.brand} to="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></Link><div className={styles.phoneTools}>
          {viewer.role === 'owner' && <button type="button" className={styles.phoneActivity} aria-label={t('next.shell.activity')} onClick={dashboard}><Icon name="i-activity" size={22} /></button>}
          {account}
        </div></header>}
        {phoneSelection ?? <nav className={styles.tabbar} aria-label={t('next.home.shell.main')}>
          <Link to="/" activeOptions={{exact: true}} activeProps={{'aria-current': 'page'}}><Icon name="i-home" size={22} /><span>{t('next.home.shell.home')}</span></Link>
          <Link to="/search" search={{q: lastSearch.current}} activeProps={{'aria-current': 'page'}}><Icon name="i-search" size={22} /><span>{t('next.home.shell.search')}</span></Link>
          <button type="button" aria-current={isLibraryLocation(pathname) ? 'page' : undefined} onClick={openLibraries}><Icon name="i-grid" size={22} /><span>{t('next.home.shell.libraries')}</span></button>
          {identity.liveAvailable && <Link to="/live" activeProps={{'aria-current': 'page'}}><Icon name="i-tv" size={22} /><span>{t('next.home.shell.live')}</span></Link>}
          <Link to="/saved" activeProps={{'aria-current': 'page'}}><Icon name="i-bookmark" size={22} /><span>{t('next.home.shell.saved')}</span></Link>
        </nav>}
      </>}
    </div>
    {children}
    {overlays}
  </div>;
}
