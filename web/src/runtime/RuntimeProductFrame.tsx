import { Bookmark, ChevronDown, Home, LibraryBig, LogOut, Menu, Radio, Search, Server, Settings, UserRound } from '#portico-icons';
import { productMessage } from '@porticomediaserver/client-core';
import { createContext, type CSSProperties, lazy, type ReactNode, type RefObject, Suspense, useCallback, useContext, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { AnchoredOverlay } from '../components/overlay/OverlayPortal';
import { runtimeFrameServerName } from './runtimeFramePolicy';
import { useRuntime } from './RuntimeContext';

const HostedAccountSettingsDialog = lazy(() => import('./HostedAccountSettingsDialog').then((module) => ({
  default: module.HostedAccountSettingsDialog,
})));

const destinations = [
  ['navigation.home', Home],
  ['navigation.libraries', LibraryBig],
  ['navigation.live-tv', Radio],
  ['navigation.search', Search],
  ['navigation.saved', Bookmark],
] as const;

export type PersistentFramePresentation = {
  shellClassName: string;
  shellStyle?: CSSProperties;
  reduceMotion?: boolean;
  sidebarClassName: string;
  sidebarInert?: boolean;
  sidebarHidden?: boolean;
  sidebarLabel?: string;
  sidebarModal?: boolean;
  sidebarRole?: 'dialog';
  sidebarTabIndex?: number;
  pageInert?: boolean;
  pageHidden?: boolean;
};

type PersistentFrameContextValue = {
  sidebar: RefObject<HTMLElement | null>;
  topbar: RefObject<HTMLElement | null>;
  warning: RefObject<HTMLDivElement | null>;
  main: RefObject<HTMLElement | null>;
  player: RefObject<HTMLDivElement | null>;
  backdrop: RefObject<HTMLDivElement | null>;
  mobileTabs: RefObject<HTMLElement | null>;
  setPresentation: (value: PersistentFramePresentation) => void;
  setConnectedContentActive: (value: boolean) => void;
};

const PersistentFrameContext = createContext<PersistentFrameContextValue | null>(null);

export function usePersistentFrame() {
  return useContext(PersistentFrameContext);
}

const defaultPresentation: PersistentFramePresentation = {
  shellClassName: 'shell',
  sidebarClassName: 'sidebar',
};

/**
 * The one product-frame DOM instance for the signed-in lifecycle. Connected
 * services portal their controls into these stable regions instead of
 * replacing the sidebar, topbar, route surface, or player host.
 */
export function RuntimeProductFrame({ children, connected = false }: { children: ReactNode; connected?: boolean }) {
  const runtime = useRuntime();
  const sidebar = useRef<HTMLElement>(null);
  const topbar = useRef<HTMLElement>(null);
  const warning = useRef<HTMLDivElement>(null);
  const main = useRef<HTMLElement>(null);
  const player = useRef<HTMLDivElement>(null);
  const backdrop = useRef<HTMLDivElement>(null);
  const mobileTabs = useRef<HTMLElement>(null);
  const [presentation, setPresentation] = useState(defaultPresentation);
  const [connectedContentActive, setConnectedContentActive] = useState(false);
  const [profileMenuOpen, setProfileMenuOpen] = useState(false);
  const [accountSettingsOpen, setAccountSettingsOpen] = useState(false);
  const profileButton = useRef<HTMLButtonElement>(null);
  const context = useMemo<PersistentFrameContextValue>(() => ({ sidebar, topbar, warning, main, player, backdrop, mobileTabs, setPresentation, setConnectedContentActive }), []);
  useLayoutEffect(() => {
    if (connected) return;
    setPresentation(defaultPresentation);
    setConnectedContentActive(false);
  }, [connected]);
  const serverName = runtimeFrameServerName(runtime.state);
  const displayName = runtime.restoredPresentation?.displayName;
  const accountLabel = displayName || 'Portico Account';
  const initial = displayName?.trim().slice(0, 1).toLocaleUpperCase();
  const connectedPresentation = connected && connectedContentActive;
  const accountControlsAvailable = !connectedPresentation && runtime.config.mode === 'hosted';
  const activePresentation = connectedPresentation ? presentation : defaultPresentation;
  const pageInactive = activePresentation.pageInert || undefined;
  const dismissProfileMenu = useCallback(() => setProfileMenuOpen(false), []);
  const openAccountSettings = () => {
    setProfileMenuOpen(false);
    setAccountSettingsOpen(true);
  };
  return <PersistentFrameContext.Provider value={context}>
    <div className={connectedPresentation ? activePresentation.shellClassName : 'shell runtime-product-frame'} data-runtime-state={connectedPresentation ? undefined : runtime.state.id} data-reduce-motion={connectedPresentation && activePresentation.reduceMotion ? 'true' : undefined} style={connectedPresentation ? activePresentation.shellStyle : undefined}>
      <a className="skip-link" href="#main-content" inert={!connectedPresentation || pageInactive} aria-hidden={!connectedPresentation || activePresentation.pageHidden || undefined}>{productMessage('navigation.skip-content').text}</a>
      <aside ref={sidebar} className={connectedPresentation ? activePresentation.sidebarClassName : 'sidebar'} inert={connectedPresentation ? activePresentation.sidebarInert : true} aria-hidden={connectedPresentation ? activePresentation.sidebarHidden : true} aria-label={connectedPresentation ? activePresentation.sidebarLabel : productMessage('navigation.primary-label').text} aria-modal={connectedPresentation ? activePresentation.sidebarModal : undefined} role={connectedPresentation ? activePresentation.sidebarRole : undefined} tabIndex={connectedPresentation ? activePresentation.sidebarTabIndex : undefined}>
        {!connectedContentActive && <>
          <div className="brand-row"><img src="/brand/portico-wordmark-white.svg" className="brand" alt="Portico" /></div>
          <nav className="primary-nav" aria-label={productMessage('navigation.primary-label').text} aria-disabled="true">
            {destinations.map(([messageId, Icon], index) => <span key={messageId} className={`nav-item ${index === 0 ? 'active' : ''}`} aria-disabled="true"><Icon aria-hidden="true" /><span>{productMessage(messageId).text}</span></span>)}
          </nav>
          <div className="sidebar-spacer" />
          {serverName && <div className="server-card" aria-disabled="true"><Server aria-hidden="true" /><span><strong>{serverName}</strong><small>Connecting</small></span><span className="health-dot pending" aria-label="Server connection pending" /></div>}
          <span className="nav-item compact" aria-disabled="true"><Settings aria-hidden="true" /><span>Settings</span></span>
        </>}
      </aside>
      <div ref={backdrop} className="persistent-frame-backdrop-host" />
      <div className="app-frame" inert={pageInactive} aria-hidden={activePresentation.pageHidden || undefined}>
        <header ref={topbar} className="topbar">
          {!connectedContentActive && <>
            <button type="button" className="icon-button menu-button" disabled aria-label={productMessage('navigation.open').text}><Menu /></button>
            <div className="topbar-search"><div className="global-search" aria-disabled="true"><Search aria-hidden="true" /><span>{serverName ? productMessage('search.input-placeholder', { serverName }).text : 'Search Portico'}</span></div></div>
            <div className="toolbar-spacer" />
            <button
              ref={profileButton}
              type="button"
              className="profile-button"
              disabled={!accountControlsAvailable}
              aria-label={accountControlsAvailable ? `Open account menu for ${accountLabel}` : 'Account menu unavailable'}
              aria-haspopup={accountControlsAvailable ? 'menu' : undefined}
              aria-expanded={accountControlsAvailable ? profileMenuOpen : undefined}
              onClick={() => accountControlsAvailable && setProfileMenuOpen((open) => !open)}
            >
              <span className="avatar">{initial || <UserRound aria-hidden="true" />}</span>
              <span><strong>{accountLabel}</strong><small>{serverName || 'No server selected'}</small></span>
              {accountControlsAvailable && <ChevronDown aria-hidden="true" />}
            </button>
            {profileMenuOpen && accountControlsAvailable && <AnchoredOverlay
              anchorRef={profileButton}
              className="profile-menu runtime-profile-menu"
              id="runtime-profile-menu"
              onDismiss={dismissProfileMenu}
              placement="bottom-end"
              role="menu"
            >
              <div className="profile-menu-current">
                <span className="avatar">{initial || <UserRound aria-hidden="true" />}</span>
                <span><strong>{accountLabel}</strong><small>Portico Account</small></span>
              </div>
              <button type="button" role="menuitem" onClick={openAccountSettings}><Settings /> Account settings</button>
              <button type="button" role="menuitem" onClick={() => { setProfileMenuOpen(false); void runtime.reselectServer(); }}><Server /> Choose another server</button>
              <button type="button" role="menuitem" onClick={() => { setProfileMenuOpen(false); void runtime.hostedLogout(); }}><LogOut /> Sign out</button>
            </AnchoredOverlay>}
          </>}
        </header>
        <div ref={warning} className="persistent-frame-warning-host" />
        <main ref={main} id="main-content" className="main route-surface" tabIndex={-1}>{children}</main>
      </div>
      <div ref={player} className="shell-player-host" inert={pageInactive} aria-hidden={activePresentation.pageHidden || undefined} />
      <nav ref={mobileTabs} className="mobile-tabs" inert={pageInactive} aria-hidden={activePresentation.pageHidden || undefined} />
      {accountSettingsOpen && <Suspense fallback={null}><HostedAccountSettingsDialog onDismiss={() => setAccountSettingsOpen(false)} /></Suspense>}
    </div>
  </PersistentFrameContext.Provider>;
}
