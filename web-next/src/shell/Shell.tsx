import {useEffect, useRef, useState, type ComponentType, type ReactNode} from 'react';
import {Link, useNavigate, useRouterState} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import type {Library, Viewer} from '@core/index.ts';
import {AccountMenu, ProfileAvatar, type ProfileIdentity, type ProfileTone} from '../kit/shell';
import type {FactPart} from '@core/presentation/kit-model/index.ts';
import {Icon} from '../kit/Icon';
import {Menu} from '../kit/overlays';
import {desktopLibraries, isLibraryLocation, rememberLibrary, rememberedLibrary, showOwnerActivity} from './model';
import {useShellScrolled, useShellWidth} from './viewport';
import styles from './shell.module.css';
import wordmark from './wordmark.svg';

export type ShellIdentity = Readonly<{
  viewer: Viewer; username?: string; profileName: string; profileArt?: ProfileTone;
  primaryProfile?: boolean; profiles: readonly ProfileIdentity[]; canManageProfiles: boolean; accountFacts: readonly FactPart[];
  libraries: readonly Library[]; liveAvailable: boolean; unread: number; showNotificationDot: boolean;
}>;
export type ShellProps = Readonly<{
  identity: ShellIdentity;
  /** Mount owns its reads. This component is never constructed for non-owner or phone. */
  OwnerActivity: ComponentType;
  dockVisible: boolean;
  onSignOut: () => void;
  overlays?: ReactNode;
  phoneSelection?: ReactNode;
  children: ReactNode;
}>;
const t = defaultI18n.t;
function libraryMemory() { try { return window.localStorage; } catch { return undefined; } }

export function Shell({identity, OwnerActivity, dockVisible, onSignOut, phoneSelection, overlays, children}: ShellProps) {
  const width = useShellWidth(), desktop = width >= 1024, scrolled = useShellScrolled();
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
  const account = <AccountMenu open={accountOpen} onOpenChange={setAccountOpen} current={current} primary={identity.primaryProfile === true}
    notificationCount={desktop ? undefined : identity.unread} notificationDot={!desktop && identity.showNotificationDot && identity.unread > 0}
    profiles={identity.profiles} accountFacts={identity.accountFacts} actions={{
      manageProfiles: () => { void navigate({to: '/profiles'}); },
      ...(identity.canManageProfiles ? {addProfile: () => { void navigate({to: '/profiles', search: {add: true}}); }} : {}),
      switchProfile: profile => { void navigate({to: '/profiles', search: {profileId: profile.id}}); },
      switchServer: () => { void navigate({to: '/servers'}); }, settings: () => { void navigate({to: '/settings'}); },
      ...(!desktop ? {notifications: () => { void navigate({to: '/notifications'}); }} : {}),
      watchTogether: () => { void navigate({to: '/watch-together'}); }, feedback: () => { void navigate({to: '/feedback'}); },
      help: () => { void navigate({to: '/help'}); }, signOut: onSignOut,
    }} trigger={<ProfileAvatar className={styles.avatar} name={identity.profileName} initial={current.initial} art={identity.profileArt} size={desktop ? 34 : 30}
      notificationDot={!desktop && identity.showNotificationDot && identity.unread > 0} aria-label={t('next.home.shell.profileMenu', {name: identity.profileName})} />} />;
  return <div className={styles.shell} data-desktop={desktop} data-has-dock={dockVisible}>
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
      </nav> : <>
        {home && <header className={styles.phoneHeader}><Link className={styles.brand} to="/" aria-label={t('next.platform.appName')}><img src={wordmark} alt="" /></Link>{account}</header>}
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
