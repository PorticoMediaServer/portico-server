import {
	AlertTriangle,
  BookOpen,
  Bookmark,
	ArrowDown,
	ArrowUp,
  ChevronDown,
  ChevronRight,
  Film,
  Home,
  Image as ImageIcon,
  LibraryBig,
  LogOut,
  Menu,
  MoreHorizontal,
  Music,
	PanelLeftClose,
	PanelLeftOpen,
	PinOff,
  Radio,
  Search,
  Server,
  ServerOff,
  TerminalSquare,
  Tv,
  Users,
  UsersRound,
	UserRound,
  X,
} from '#portico-icons';
import { productMessage, type ProductMessageId } from '@portico/client-core';
import { type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Link, NavLink, useLocation, useNavigate, useNavigationType } from 'react-router-dom';
import { IconButton } from '../components/controls/Buttons';
import { SemanticProductIcon, productText } from '../components/ProductLanguage';
import { AnchoredOverlay } from '../components/overlay/OverlayPortal';
import { useAuthSession, useLibraries, useSearchContract } from '../data/DataProvider';
import { canManageServer as viewerCanManageServer } from '../data/authority';
import type { LibrarySummary, Viewer } from '../data/models';
import { QuickSearchPanel } from '../features/search/QuickSearchPanel';
import { FeedbackDialog } from '../features/feedback/FeedbackDialog';
import { ProfileSwitcherDialog } from '../features/profiles/ProfileSwitcherDialog';
import { useNotifications } from '../features/notifications/NotificationProvider';
import { useOptionalWebDisplayPreferences } from '../preferences/WebDisplayPreferencesProvider';
import { defaultWebDisplayPreferences, recordRecentSearch } from '../preferences/webDisplayPreferences';
import { useOptionalRuntime } from '../runtime/RuntimeContext';
import { usePersistentFrame } from '../runtime/RuntimeProductFrame';
import { useRouteLifecycle } from '../runtime/useRouteLifecycle';

const navigation = [
  ['/', 'navigation.home', Home],
  ['/libraries', 'navigation.libraries', LibraryBig],
  ['/live', 'navigation.live-tv', Radio],
  ['/search', 'navigation.search', Search],
  ['/saved', 'navigation.saved', Bookmark],
] as const satisfies ReadonlyArray<readonly [string, ProductMessageId, typeof Home]>;

const mobileNavigationQuery = '(max-width: 900px)';
const LOCAL_PROFILE_SELECTION_KEY = 'portico.local-profile-selection-required.v1';
const SIDEBAR_WIDTH_KEY = 'portico.sidebar-width.v1';
const SIDEBAR_WIDTH_DEFAULT = 278;
const SIDEBAR_WIDTH_MIN = 140;
const SIDEBAR_WIDTH_MAX = 348;

function clampSidebarWidth(value: number) {
  return Math.max(SIDEBAR_WIDTH_MIN, Math.min(SIDEBAR_WIDTH_MAX, Math.round(value)));
}

function retainedSidebarWidth() {
  try {
    const value = Number(localStorage.getItem(SIDEBAR_WIDTH_KEY));
    return Number.isFinite(value) && value > 0 ? clampSidebarWidth(value) : SIDEBAR_WIDTH_DEFAULT;
  } catch {
    return SIDEBAR_WIDTH_DEFAULT;
  }
}

function persistSidebarWidth(value: number) {
  try { localStorage.setItem(SIDEBAR_WIDTH_KEY, String(clampSidebarWidth(value))); }
  catch { /* Browser storage is optional; resizing still works for this page lifetime. */ }
}

function retainedLocalProfileSelection() {
  try { return sessionStorage.getItem(LOCAL_PROFILE_SELECTION_KEY) === 'true'; }
  catch { return false; }
}

function mobileNavigationLayout() {
  return globalThis.matchMedia?.(mobileNavigationQuery).matches ?? false;
}

function drawerFocusableElements(drawer: HTMLElement) {
  return Array.from(drawer.querySelectorAll<HTMLElement>(
    'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
  )).filter((element) => !element.hasAttribute('hidden') && element.getAttribute('aria-hidden') !== 'true' && !element.closest('[inert]'));
}

function LibraryIcon({ library }: { library: LibrarySummary }) {
  if (library.kind === 'music') return <Music aria-hidden="true" />;
  if (library.kind === 'movies') return <Film aria-hidden="true" />;
  if (library.kind === 'audiobooks') return <BookOpen aria-hidden="true" />;
  if (library.kind === 'recorded-tv') return <Radio aria-hidden="true" />;
  return <Tv aria-hidden="true" />;
}

function PinnedLibraryNavItem({ library, active, first, last, onChange }: {
  library: LibrarySummary;
  active: boolean;
  first: boolean;
  last: boolean;
  onChange: (action: 'up' | 'down' | 'unpin') => void;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const destination = `/library/${encodeURIComponent(library.id)}`;
  return <div className={`library-pin-row ${active ? 'active' : ''}`}>
    <Link to={destination} className="nav-item library-pin"><LibraryIcon library={library} /><span>{library.name}</span></Link>
    <button ref={trigger} type="button" className="library-pin-more" aria-label={productMessage('navigation.library-options', { title: library.name }).text} aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((value) => !value)}><MoreHorizontal /></button>
    {open && <AnchoredOverlay anchorRef={trigger} placement="bottom-start" className="library-pin-menu" role="menu" onDismiss={() => setOpen(false)}>
      <button role="menuitem" disabled={first} onClick={() => { onChange('up'); setOpen(false); }}><ArrowUp /> {productMessage('navigation.move-up').text}</button>
      <button role="menuitem" disabled={last} onClick={() => { onChange('down'); setOpen(false); }}><ArrowDown /> {productMessage('navigation.move-down').text}</button>
      <button role="menuitem" onClick={() => { onChange('unpin'); setOpen(false); }}><PinOff /> {productMessage('navigation.unpin').text}</button>
    </AnchoredOverlay>}
  </div>;
}

export function AppShell({ children, viewer, player }: { children: ReactNode; viewer: Viewer; player?: ReactNode }) {
  const auth = useAuthSession();
  const runtime = useOptionalRuntime();
  const persistentFrame = usePersistentFrame();
  const display = useOptionalWebDisplayPreferences();
  const notifications = useNotifications();
  const displayPreferences = display?.preferences ?? defaultWebDisplayPreferences;
  const [mobileLayout, setMobileLayout] = useState(mobileNavigationLayout);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [quickQuery, setQuickQuery] = useState('');
  const [debouncedQuickQuery, setDebouncedQuickQuery] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [activeSearchOptionId, setActiveSearchOptionId] = useState<string>();
  const [profileOpen, setProfileOpen] = useState(false);
  const [profileSwitcherOpen, setProfileSwitcherOpen] = useState(retainedLocalProfileSelection);
  const [feedbackOpen, setFeedbackOpen] = useState(false);
	const [sidebarWidth, setSidebarWidth] = useState(retainedSidebarWidth);
	const [sidebarResizing, setSidebarResizing] = useState(false);
	const [connectionWarningHeight, setConnectionWarningHeight] = useState(52);
  const location = useLocation();
  const navigationType = useNavigationType();
  const navigate = useNavigate();
  const searchAnchor = useRef<HTMLFormElement>(null);
  const searchInput = useRef<HTMLInputElement>(null);
  const suppressNextSearchOpen = useRef(false);
  const standaloneSidebar = useRef<HTMLElement>(null);
  const sidebar = persistentFrame?.sidebar ?? standaloneSidebar;
  const mobileTrigger = useRef<HTMLButtonElement>(null);
  const restoreMobileFocus = useRef(false);
  const profileTrigger = useRef<HTMLButtonElement>(null);
  const sidebarWidthRef = useRef(sidebarWidth);
  const sidebarResizeStart = useRef<{ pointerId: number; clientX: number; width: number } | undefined>(undefined);
  const standaloneMainContent = useRef<HTMLElement>(null);
  const mainContent = persistentFrame?.main ?? standaloneMainContent;
  const connectionWarningRef = useRef<HTMLDivElement>(null);
  const libraries = useLibraries();
  const searchContract = useSearchContract();
  const displayName = viewer.user?.displayName || 'Portico user';
  const initial = displayName.trim().slice(0, 1).toLocaleUpperCase() || 'P';
  const storedSidebarOrder = viewer.user?.preferences?.sidebarOrder ?? [];
  const pinnedLibraryIds = display
    ? displayPreferences.pinnedLibraryIds
    : storedSidebarOrder.flatMap((entry) => entry.startsWith('library:') ? [entry.slice('library:'.length)] : []);
  const pinnedLibraries = libraries.status === 'success' ? pinnedLibraryIds.flatMap((id) => {
    const library = libraries.data.find((candidate) => candidate.id === id);
    return library ? [library] : [];
  }) : [];
  const canManageServer = viewerCanManageServer(viewer.user);
  const canWatchWithFriends = canManageServer || viewer.user?.permissions?.watchWithFriends === true;
  const canUseLiveTV = canManageServer || ['viewLiveTV', 'playLiveTV', 'viewDVR', 'scheduleDVR', 'manageDVR'].some((permission) => viewer.user?.permissions?.[permission] === true);
  const availableNavigation = navigation.filter(([to]) => to !== '/live' || canUseLiveTV);
  const avatar = viewer.user?.profileImageUrl ? <img src={viewer.user.profileImageUrl} alt="" /> : initial;
	const routeTransitioning = useRouteLifecycle(location, navigationType, mainContent);
	const connectionWarning = runtime?.connectionWarning ? productMessage(runtime.connectionWarning) : undefined;
	const openMobileNavigation = useCallback(() => {
		restoreMobileFocus.current = true;
		setSearchOpen(false);
		setProfileOpen(false);
		setMobileOpen(true);
	}, []);
	const closeMobileNavigation = useCallback((restoreFocus = true) => {
		restoreMobileFocus.current = restoreFocus;
		setMobileOpen(false);
	}, []);
  const submitSearch = () => {
    const query = quickQuery.trim();
    if (query && display?.preferences.rememberSearchHistory) {
      const recentSearches = recordRecentSearch(display.preferences.recentSearches, query);
      if (recentSearches.join('\n') !== display.preferences.recentSearches.join('\n')) void display.patch({ recentSearches }).catch(() => undefined);
    }
    navigate(query ? `/search?q=${encodeURIComponent(query)}` : '/search');
    setSearchOpen(false);
  };

  useEffect(() => {
    closeMobileNavigation(false);
    setSearchOpen(false);
    setProfileOpen(false);
  }, [closeMobileNavigation, location.pathname]);

  useEffect(() => {
    const query = window.matchMedia(mobileNavigationQuery);
    const update = () => {
      setMobileLayout(query.matches);
      if (!query.matches) closeMobileNavigation(false);
    };
    update();
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, [closeMobileNavigation]);

  useLayoutEffect(() => {
    if (!mobileLayout) return;
    if (mobileOpen) {
      const frame = window.requestAnimationFrame(() => drawerFocusableElements(sidebar.current!)[0]?.focus());
      return () => window.cancelAnimationFrame(frame);
    }
    if (!restoreMobileFocus.current) return;
    restoreMobileFocus.current = false;
    const frame = window.requestAnimationFrame(() => mobileTrigger.current?.focus({ preventScroll: true }));
    return () => window.cancelAnimationFrame(frame);
  }, [mobileLayout, mobileOpen]);

  useEffect(() => {
    if (!mobileLayout || !mobileOpen) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const containFocus = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeMobileNavigation();
        return;
      }
      if (event.key !== 'Tab' || !sidebar.current) return;
      const items = drawerFocusableElements(sidebar.current);
      if (items.length === 0) {
        event.preventDefault();
        sidebar.current.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && (document.activeElement === first || !sidebar.current.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener('keydown', containFocus);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener('keydown', containFocus);
    };
  }, [closeMobileNavigation, mobileLayout, mobileOpen]);

  useEffect(() => {
    const root = document.documentElement;
    if (displayPreferences.reduceMotion) root.dataset.porticoReduceMotion = 'true';
    else delete root.dataset.porticoReduceMotion;
    return () => { delete root.dataset.porticoReduceMotion; };
  }, [displayPreferences.reduceMotion]);

  useLayoutEffect(() => {
    if (!connectionWarningRef.current) return;
    const warning = connectionWarningRef.current;
    const update = () => setConnectionWarningHeight(Math.ceil(warning.getBoundingClientRect().height));
    update();
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(update);
    observer?.observe(warning);
    window.addEventListener('resize', update);
    return () => {
      observer?.disconnect();
      window.removeEventListener('resize', update);
    };
  }, [connectionWarning]);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuickQuery(quickQuery.trim()), 180);
    setActiveSearchOptionId(undefined);
    return () => window.clearTimeout(timer);
  }, [quickQuery]);

  const moveSearchSelection = (direction: 1 | -1) => {
    const options = Array.from(document.querySelectorAll<HTMLElement>('#global-search-results [data-search-result]'));
    if (!options.length) return;
    const current = options.findIndex((option) => option.id === activeSearchOptionId);
    const next = current < 0 ? (direction > 0 ? 0 : options.length - 1) : (current + direction + options.length) % options.length;
    setActiveSearchOptionId(options[next].id);
    options[next].scrollIntoView({ block: 'nearest' });
  };

  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        searchInput.current?.focus();
        setSearchOpen(true);
      }
      if (event.key === 'Escape') {
        if (searchOpen) {
          event.preventDefault();
          suppressNextSearchOpen.current = true;
          setSearchOpen(false);
          window.requestAnimationFrame(() => searchInput.current?.focus());
        }
        if (profileOpen) {
          event.preventDefault();
          setProfileOpen(false);
          window.requestAnimationFrame(() => profileTrigger.current?.focus());
        }
      }
    };
    window.addEventListener('keydown', shortcut);
    return () => window.removeEventListener('keydown', shortcut);
  }, [profileOpen, searchOpen]);

  const updateSidebarWidth = useCallback((value: number, persist = false) => {
    const next = clampSidebarWidth(value);
    sidebarWidthRef.current = next;
    setSidebarWidth(next);
    if (persist) persistSidebarWidth(next);
  }, []);

  const startSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    sidebarResizeStart.current = { pointerId: event.pointerId, clientX: event.clientX, width: sidebarWidthRef.current };
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setSidebarResizing(true);
  };

  const moveSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = sidebarResizeStart.current;
    if (!start || start.pointerId !== event.pointerId) return;
    updateSidebarWidth(start.width + event.clientX - start.clientX);
  };

  const finishSidebarResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    const start = sidebarResizeStart.current;
    if (!start || start.pointerId !== event.pointerId) return;
    sidebarResizeStart.current = undefined;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture?.(event.pointerId);
    persistSidebarWidth(sidebarWidthRef.current);
    setSidebarResizing(false);
  };

  const resizeSidebarWithKeyboard = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    const step = event.shiftKey ? 24 : 8;
    let next: number | undefined;
    if (event.key === 'ArrowLeft') next = sidebarWidthRef.current - step;
    else if (event.key === 'ArrowRight') next = sidebarWidthRef.current + step;
    else if (event.key === 'Home') next = SIDEBAR_WIDTH_MIN;
    else if (event.key === 'End') next = SIDEBAR_WIDTH_MAX;
    if (next === undefined) return;
    event.preventDefault();
    updateSidebarWidth(next, true);
  };

  const drawerActive = mobileLayout && mobileOpen;
  const pageInactive = drawerActive || undefined;
  const sidebarCollapsed = displayPreferences.sidebarCollapsed && !mobileLayout;
  const shellClassName = `shell ${connectionWarning ? 'has-connection-warning' : ''} ${sidebarCollapsed ? 'sidebar-collapsed' : ''} ${sidebarResizing ? 'sidebar-resizing' : ''}`.trim();
  const shellStyle = useMemo(() => ({ '--sidebar-expanded-width': `${sidebarWidth}px`, '--catalog-card-scale': displayPreferences.cardSizePercent / 100, '--context-chrome-height': connectionWarning ? `${connectionWarningHeight}px` : '0px' } as CSSProperties), [connectionWarning, connectionWarningHeight, displayPreferences.cardSizePercent, sidebarWidth]);
  const sidebarClassName = `sidebar ${mobileOpen ? 'open' : ''}`;

  useLayoutEffect(() => {
    if (!persistentFrame) return;
    persistentFrame.setPresentation({
      shellClassName,
      shellStyle,
      reduceMotion: displayPreferences.reduceMotion,
      sidebarClassName,
      sidebarInert: mobileLayout && !mobileOpen ? true : undefined,
      sidebarHidden: mobileLayout && !mobileOpen ? true : undefined,
      sidebarLabel: mobileLayout ? productMessage('navigation.primary-label').text : undefined,
      sidebarModal: drawerActive || undefined,
      sidebarRole: drawerActive ? 'dialog' : undefined,
      sidebarTabIndex: drawerActive ? -1 : undefined,
      pageInert: pageInactive,
      pageHidden: drawerActive || undefined,
    });
    if (persistentFrame.main.current) persistentFrame.main.current.className = `main route-surface ${routeTransitioning ? 'route-entering' : ''}`;
    if (persistentFrame.mobileTabs.current) {
      persistentFrame.mobileTabs.current.setAttribute('aria-label', productMessage('navigation.primary-label').text ?? '');
      persistentFrame.mobileTabs.current.style.setProperty('--mobile-tab-count', String(availableNavigation.length));
    }
  }, [availableNavigation.length, displayPreferences.reduceMotion, drawerActive, mobileLayout, mobileOpen, pageInactive, persistentFrame, routeTransitioning, shellClassName, shellStyle, sidebarClassName]);

  const sidebarContent = <>
      <div className="brand-row"><img src="/brand/portico-wordmark-white.svg" className="brand brand-wordmark" alt="Portico" /><img src="/brand/portico-symbol-white.svg" className="brand brand-symbol" alt="Portico" /><IconButton label={productMessage('navigation.close').text ?? ''} className="nav-close" onClick={() => closeMobileNavigation()}><X /></IconButton></div>
      <nav className="primary-nav" aria-label={productMessage('navigation.primary-label').text}>{availableNavigation.map(([to, messageId, Icon]) => <NavLink key={to} to={to} end={to === '/'} className={({ isActive }) => isActive ? 'nav-item active' : 'nav-item'}><Icon aria-hidden="true" /><span>{productMessage(messageId).text}</span></NavLink>)}</nav>
      {pinnedLibraries.length > 0 && <><div className="sidebar-rule" /><div className="nav-caption"><span>{productMessage('navigation.pinned-libraries').text}</span><Link to="/libraries">{productMessage('navigation.manage').text}</Link></div><nav className="library-nav" aria-label={productMessage('navigation.pinned-libraries').text}>{pinnedLibraries.map((library, index) => {
        const destination = `/library/${encodeURIComponent(library.id)}`;
        return <PinnedLibraryNavItem key={library.id} library={library} active={location.pathname === destination} first={index === 0} last={index === pinnedLibraries.length - 1} onChange={(action) => {
          const next = [...pinnedLibraryIds];
          const currentIndex = next.indexOf(library.id);
          if (currentIndex < 0) return;
          if (action === 'unpin') next.splice(currentIndex, 1);
          else {
            const targetIndex = action === 'up' ? currentIndex - 1 : currentIndex + 1;
            if (targetIndex < 0 || targetIndex >= next.length) return;
            [next[currentIndex], next[targetIndex]] = [next[targetIndex], next[currentIndex]];
          }
          void display?.patch({ pinnedLibraryIds: next }).catch(() => undefined);
        }} />;
      })}</nav></>}
      <div className="sidebar-spacer" />
      {!mobileLayout && <button type="button" className="sidebar-collapse" aria-label={productText(sidebarCollapsed ? 'navigation.expand' : 'navigation.collapse')} title={productText(sidebarCollapsed ? 'navigation.expand' : 'navigation.collapse')} onClick={() => void display?.patch({ sidebarCollapsed: !sidebarCollapsed }).catch(() => undefined)}>{sidebarCollapsed ? <PanelLeftOpen /> : <PanelLeftClose />}<span>{productText(sidebarCollapsed ? 'navigation.expand' : 'navigation.collapse')}</span></button>}
      {!mobileLayout && !sidebarCollapsed && <div className="sidebar-resize-handle" role="separator" aria-label="Resize navigation" aria-orientation="vertical" aria-valuemin={SIDEBAR_WIDTH_MIN} aria-valuemax={SIDEBAR_WIDTH_MAX} aria-valuenow={sidebarWidth} tabIndex={0} title="Resize navigation" onDoubleClick={() => updateSidebarWidth(SIDEBAR_WIDTH_DEFAULT, true)} onKeyDown={resizeSidebarWithKeyboard} onPointerDown={startSidebarResize} onPointerMove={moveSidebarResize} onPointerUp={finishSidebarResize} onPointerCancel={finishSidebarResize} />}
  </>;
  const topbarContent = <>
        <IconButton ref={mobileTrigger} label={productMessage('navigation.open').text ?? ''} className="menu-button" onClick={openMobileNavigation}><Menu /></IconButton>
        <div className="topbar-search">
          <form ref={searchAnchor} className={`global-search ${searchOpen ? 'active' : ''}`} role="search" onSubmit={(event) => { event.preventDefault(); submitSearch(); }}>
            <button className="quick-search-submit" type="submit" aria-label={productMessage('search.open-full').text}><Search /></button>
            <input ref={searchInput} value={quickQuery} maxLength={searchContract.status === 'success' ? searchContract.data.limits.maximumQueryLength : undefined} onFocus={() => {
              if (suppressNextSearchOpen.current) {
                suppressNextSearchOpen.current = false;
                return;
              }
              setSearchOpen(true);
            }} onChange={(event) => { setQuickQuery(event.target.value); setSearchOpen(true); }} onKeyDown={(event) => {
              if (event.key === 'Enter' && activeSearchOptionId) {
                event.preventDefault();
                document.getElementById(activeSearchOptionId)?.click();
              } else if (event.key === 'Enter') {
                event.preventDefault();
                submitSearch();
              } else if (event.key === 'ArrowDown' && searchOpen) {
                event.preventDefault();
                moveSearchSelection(1);
              } else if (event.key === 'ArrowUp' && searchOpen) {
                event.preventDefault();
                moveSearchSelection(-1);
              }
            }} placeholder={productMessage('search.input-placeholder', { serverName: viewer.serverName }).text} aria-label={productMessage('search.quick-input-label').text} role="combobox" aria-haspopup="dialog" aria-expanded={searchOpen} aria-controls="global-search-results" aria-activedescendant={activeSearchOptionId} />
            {quickQuery && <button className="quick-search-clear" type="button" aria-label={productMessage('action.clear-search').text} onClick={() => { setQuickQuery(''); searchInput.current?.focus(); }}><X /></button>}
          </form>
          {searchOpen && <AnchoredOverlay id="global-search-results" role="dialog" anchorRef={searchAnchor} returnFocusRef={searchInput} matchAnchorWidth autoFocusComposite={false} ariaLabel="Search suggestions" className="quick-search-panel" onDismiss={(reason) => {
            if (reason === 'escape') suppressNextSearchOpen.current = true;
            setSearchOpen(false);
          }}>
            <QuickSearchPanel query={debouncedQuickQuery} serverName={viewer.serverName} activeOptionId={activeSearchOptionId} onActiveOptionChange={setActiveSearchOptionId} onSelect={() => setSearchOpen(false)} onViewAll={submitSearch} />
          </AnchoredOverlay>}
        </div>
        <div className="profile-menu-root">
          <button ref={profileTrigger} className={`profile-button ${profileOpen ? 'selected' : ''}`} aria-label={`Open profile menu for ${displayName}${notifications.unreadCount ? `. ${productText('notification.unread-label', { count: notifications.unreadCount })}` : ''}`} aria-expanded={profileOpen} aria-haspopup="menu" onClick={() => setProfileOpen((value) => !value)} onKeyDown={(event) => {
            if (event.key === 'ArrowDown' && !profileOpen) {
              event.preventDefault();
              setProfileOpen(true);
            }
          }}><span className="profile-avatar-shell"><span className="avatar">{avatar}</span>{notifications.unreadCount > 0 && <span className="notification-count-badge" aria-hidden="true">{notifications.unreadCount > 99 ? '99+' : notifications.unreadCount}</span>}</span><span><strong>{displayName}</strong><small>{viewer.serverName}</small></span><ChevronDown /></button>
          {profileOpen && <AnchoredOverlay anchorRef={profileTrigger} placement="bottom-end" className="profile-menu" role="menu" onDismiss={() => setProfileOpen(false)}>
            <div className="profile-menu-current"><span className="avatar">{avatar}</span><span><strong>{displayName}</strong><small>{viewer.user?.role || 'user'} · {viewer.serverName}</small></span></div>
			<Link role="menuitem" to="/notifications" onClick={() => setProfileOpen(false)}><SemanticProductIcon id="status.notification" /> {productText('notification.title')}{notifications.unreadCount > 0 && <span className="profile-menu-count">{notifications.unreadCount > 99 ? '99+' : notifications.unreadCount}</span>}</Link>
			<button role="menuitem" onClick={() => {
				setProfileOpen(false);
				if (runtime?.config.mode === 'hosted') void runtime.beginProfileSelection();
				else {
          try { sessionStorage.setItem(LOCAL_PROFILE_SELECTION_KEY, 'true'); } catch { /* The dedicated state still applies for this page lifetime. */ }
          setProfileSwitcherOpen(true);
        }
			}}><UserRound /> Switch Profile <ChevronRight /></button>
            <Link role="menuitem" to="/settings/account"><Users /> Account settings <ChevronRight /></Link>
            <Link role="menuitem" to="/settings/profiles"><UserRound /> Profiles <ChevronRight /></Link>
            <Link role="menuitem" to="/settings/appearance"><ImageIcon /> Appearance <ChevronRight /></Link>
            <button role="menuitem" onClick={() => { setProfileOpen(false); setFeedbackOpen(true); }}><SemanticProductIcon id="action.message" /> {productText('action.send-message')}</button>
            {canWatchWithFriends && <Link role="menuitem" to="/watch-with-friends"><UsersRound /> Watch With Friends <ChevronRight /></Link>}
            {canManageServer && <Link role="menuitem" to="/settings/status"><Server /> Server settings <ChevronRight /></Link>}
            {canManageServer && <Link role="menuitem" to="/settings/diagnostics"><TerminalSquare /> Server console <ChevronRight /></Link>}
            {runtime?.config.mode === 'hosted' ? <>
              <button role="menuitem" onClick={() => { setProfileOpen(false); runtime.disconnectServer(); }}><ServerOff /> Choose another server</button>
              <button role="menuitem" onClick={() => { setProfileOpen(false); void runtime.hostedLogout(); }}><LogOut /> Sign out of Portico Account</button>
			</> : <>
				<button role="menuitem" onClick={() => { setProfileOpen(false); void auth.logout(); }}><LogOut /> Sign out of current session</button>
			</>}
          </AnchoredOverlay>}
        </div>
  </>;
  const warningContent = connectionWarning ? <div ref={connectionWarningRef} className="connection-durability-warning" role="status" aria-live="polite" data-semantic-icon={connectionWarning.icon}>
        <AlertTriangle aria-hidden="true" />
        <span><strong>{connectionWarning.title}</strong><small>{connectionWarning.body}</small></span>
  </div> : null;
  const mobileTabsContent = availableNavigation.map(([to, messageId, Icon]) => <NavLink key={to} to={to} end={to === '/'} className={({ isActive }) => isActive ? 'mobile-tab active' : 'mobile-tab'}><Icon /><span>{productMessage(messageId).text}</span></NavLink>);
  const localProfileSelectionRequired = profileSwitcherOpen && runtime?.config.mode !== 'hosted';
  const finishLocalProfileSelection = () => {
    try { sessionStorage.removeItem(LOCAL_PROFILE_SELECTION_KEY); } catch { /* Nothing else is required. */ }
    setProfileSwitcherOpen(false);
  };
  const auxiliaryContent = <>{feedbackOpen && <FeedbackDialog kind="general" onDismiss={() => setFeedbackOpen(false)} />}{profileSwitcherOpen && !localProfileSelectionRequired && <ProfileSwitcherDialog onDismiss={() => setProfileSwitcherOpen(false)} />}</>;

  useLayoutEffect(() => {
    if (!persistentFrame) return;
    persistentFrame.setConnectedContentActive(!localProfileSelectionRequired);
    return () => persistentFrame.setConnectedContentActive(false);
  }, [localProfileSelectionRequired, persistentFrame]);

  if (localProfileSelectionRequired) {
    return <ProfileSwitcherDialog
      required
      onDismiss={finishLocalProfileSelection}
      onSignOut={() => {
        finishLocalProfileSelection();
        void auth.logout();
      }}
    />;
  }

  if (persistentFrame?.sidebar.current && persistentFrame.topbar.current && persistentFrame.warning.current && persistentFrame.main.current && persistentFrame.player.current && persistentFrame.backdrop.current && persistentFrame.mobileTabs.current) {
    return <>
      {createPortal(sidebarContent, persistentFrame.sidebar.current)}
      {createPortal(topbarContent, persistentFrame.topbar.current)}
      {createPortal(warningContent, persistentFrame.warning.current)}
      {createPortal(children, persistentFrame.main.current)}
      {createPortal(player, persistentFrame.player.current)}
      {createPortal(drawerActive ? <div className="mobile-sidebar-backdrop" aria-hidden="true" onPointerDown={() => closeMobileNavigation()} /> : null, persistentFrame.backdrop.current)}
      {createPortal(mobileTabsContent, persistentFrame.mobileTabs.current)}
      {auxiliaryContent}
    </>;
  }

  return <div className={shellClassName} data-reduce-motion={displayPreferences.reduceMotion ? 'true' : undefined} style={shellStyle}>
    <a className="skip-link" href="#main-content" inert={pageInactive} aria-hidden={drawerActive || undefined}>{productMessage('navigation.skip-content').text}</a>
    <aside ref={sidebar} className={sidebarClassName} inert={mobileLayout && !mobileOpen ? true : undefined} aria-hidden={mobileLayout && !mobileOpen ? true : undefined} aria-label={mobileLayout ? productMessage('navigation.primary-label').text : undefined} aria-modal={drawerActive || undefined} role={drawerActive ? 'dialog' : undefined} tabIndex={drawerActive ? -1 : undefined}>{sidebarContent}</aside>
    {drawerActive && <div className="mobile-sidebar-backdrop" aria-hidden="true" onPointerDown={() => closeMobileNavigation()} />}
    <div className="app-frame" inert={pageInactive} aria-hidden={drawerActive || undefined}>
      <header className="topbar">{topbarContent}</header>
      {warningContent}
      <main ref={mainContent} id="main-content" className={`main route-surface ${routeTransitioning ? 'route-entering' : ''}`} tabIndex={-1}>{children}</main>
    </div>
    <div className="shell-player-host" inert={pageInactive} aria-hidden={drawerActive || undefined}>{player}</div>
    {auxiliaryContent}
    <nav className="mobile-tabs" inert={pageInactive} aria-hidden={drawerActive || undefined} aria-label={productMessage('navigation.primary-label').text} style={{ '--mobile-tab-count': availableNavigation.length } as CSSProperties}>{mobileTabsContent}</nav>
  </div>;
}
