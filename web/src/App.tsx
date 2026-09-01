import { NavigationLibraryIcon } from '#portico-icons';
import { lazy, Suspense, useEffect, useState } from 'react';
import { Link, Route, Routes, useLocation } from 'react-router-dom';
import { AppShell } from './app/AppShell';
import { useAuthSession } from './data/DataProvider';
import { clearArtworkFailureCache } from './data/artworkFailureCache';
import type { Viewer } from './data/models';
import { AccountChooserSurface, AuthFailureSurface, AuthLoadingSurface, LocalProfileSelectionSurface, SetupSurface, SignInSurface } from './features/auth/AuthSurface';
import { PlaybackSessionProvider, PlayerDock, usePlaybackSession, WatchPage } from './features/player/PlayerSurface';
import { WebDisplayPreferencesProvider } from './preferences/WebDisplayPreferencesProvider';
import { NotificationProvider } from './features/notifications/NotificationProvider';
import { RouteErrorBoundary } from './runtime/RouteErrorBoundary';
import { VIEWER_TRANSITION_EVENT } from './components/overlay/OverlayPortal';
import { reviewedProductErrorText, productText } from './components/ProductLanguage';
import { SettingsNavigationBlocker } from './features/settings/SettingsNavigationBlocker';

const loadHomePage = () => import('./features/home/HomePage').then((module) => ({ default: module.HomePage }));
const loadLibrariesHubPage = () => import('./features/libraries/LibrariesHubPage').then((module) => ({ default: module.LibrariesHubPage }));
const loadLibraryWorkspaceRoute = () => import('./routes/LibraryWorkspaceRoute').then((module) => ({ default: module.LibraryWorkspaceRoute }));
const loadDetailPage = () => import('./features/detail/DetailPage').then((module) => ({ default: module.DetailPage }));
const loadPersonDetailPage = () => import('./features/detail/PersonDetailPage').then((module) => ({ default: module.PersonDetailPage }));
const loadLiveTVPage = () => import('./features/live-tv/LiveTVPage').then((module) => ({ default: module.LiveTVPage }));
const loadSettingsRoute = () => import('./routes/SettingsRoute').then((module) => ({ default: module.SettingsRoute }));
const loadSearchPage = () => import('./features/search/SearchPage').then((module) => ({ default: module.SearchPage }));
const loadSavedWorkspace = () => import('./features/saved/SavedWorkspace');
const loadSavedPage = () => loadSavedWorkspace().then((module) => ({ default: module.SavedPage }));
const loadSavedResourcePage = () => loadSavedWorkspace().then((module) => ({ default: module.SavedResourcePage }));
const loadWatchWithFriendsRoute = () => import('./routes/WatchWithFriendsRoute').then((module) => ({ default: module.WatchWithFriendsRoute }));
const loadNotificationsPage = () => import('./features/notifications/NotificationsPage').then((module) => ({ default: module.NotificationsPage }));

const HomePage = lazy(loadHomePage);
const LibrariesHubPage = lazy(loadLibrariesHubPage);
const LibraryWorkspaceRoute = lazy(loadLibraryWorkspaceRoute);
const DetailPage = lazy(loadDetailPage);
const PersonDetailPage = lazy(loadPersonDetailPage);
const LiveTVPage = lazy(loadLiveTVPage);
const SettingsRoute = lazy(loadSettingsRoute);
const SearchPage = lazy(loadSearchPage);
const SavedPage = lazy(loadSavedPage);
const SavedResourcePage = lazy(loadSavedResourcePage);
const WatchWithFriendsRoute = lazy(loadWatchWithFriendsRoute);
const NotificationsPage = lazy(loadNotificationsPage);

type PreloadConnectionHints = {
  saveData?: boolean;
  effectiveType?: string;
  downlink?: number;
};

type PreloadNavigatorHints = {
  onLine?: boolean;
  connection?: PreloadConnectionHints;
};

/** Route preloading is a convenience and must yield to user/network policy. */
export function shouldPreloadProductRoutes(navigatorHints: PreloadNavigatorHints = typeof navigator === 'undefined' ? {} : navigator as PreloadNavigatorHints): boolean {
  if (navigatorHints.onLine === false) return false;
  const connection = navigatorHints.connection;
  if (connection?.saveData) return false;
  if (connection?.effectiveType === 'slow-2g' || connection?.effectiveType === '2g') return false;
  if (typeof connection?.downlink === 'number' && connection.downlink < 1.5) return false;
  return true;
}

const preloadProductRoutes = () => Promise.allSettled([
  // Keep this speculative set to the shell and the most likely next browsing
  // destinations. Detail, playback, saved, settings, and social routes remain
  // lazy until navigation asks for them.
  loadHomePage(),
  loadLibrariesHubPage(),
  loadLibraryWorkspaceRoute(),
  loadSearchPage(),
  loadNotificationsPage(),
]);

export function scheduleProductRoutePreload(task: () => void): () => void {
  let cancelled = false;
  let idleHandle: number | undefined;
  let fallbackHandle: number | undefined;
  const idleWindow = window as Window & {
    requestIdleCallback?: (callback: IdleRequestCallback, options?: IdleRequestOptions) => number;
    cancelIdleCallback?: (handle: number) => void;
  };
  const run = (deadline?: IdleDeadline) => {
    if (cancelled || !shouldPreloadProductRoutes()) return;
    // A timeout-driven idle callback is not evidence of useful idle time. Do
    // not turn speculative imports into foreground work on a busy device.
    if (deadline && (deadline.didTimeout || deadline.timeRemaining() < 50)) return;
    task();
  };
  if (typeof idleWindow.requestIdleCallback === 'function') {
    idleHandle = idleWindow.requestIdleCallback(run, { timeout: 3_000 });
  } else {
    // Older browsers have no idle scheduler; defer one turn and re-check the
    // connection policy rather than eagerly importing during first paint.
    fallbackHandle = window.setTimeout(() => run(), 1_000);
  }
  return () => {
    cancelled = true;
    if (idleHandle !== undefined) idleWindow.cancelIdleCallback?.(idleHandle);
    if (fallbackHandle !== undefined) window.clearTimeout(fallbackHandle);
  };
}

function RouteLoading() {
  const { pathname } = useLocation();
  return <div className="standard-page route-content-reservation" data-route-kind={pathname.split('/').filter(Boolean)[0] || 'home'} aria-busy="true" />;
}

function NotFoundPage() {
  return <div className="standard-page"><div className="library-state error" role="status">
    <NavigationLibraryIcon />
    <strong>This page isn’t available</strong>
    <p>The address may be old, or this server no longer exposes that part of Portico.</p>
    <div className="route-recovery-links"><Link className="button primary" to="/">Open Home</Link><Link className="button secondary" to="/libraries">Browse libraries</Link></div>
  </div></div>;
}

function AppRoutes({ viewer }: { viewer: Viewer }) {
  return <Suspense fallback={<RouteLoading />}><Routes>
    <Route path="/" element={<HomePage />} />
    <Route path="/libraries" element={<LibrariesHubPage />} />
    <Route path="/library/:libraryId" element={<LibraryWorkspaceRoute />} />
    <Route path="/media/:id" element={<DetailPage />} />
    <Route path="/person/:id" element={<PersonDetailPage />} />
    <Route path="/live" element={<LiveTVPage />} />
    <Route path="/settings" element={<SettingsRoute viewer={viewer} />} />
    <Route path="/settings/:section" element={<SettingsRoute viewer={viewer} />} />
    <Route path="/search" element={<SearchPage />} />
    <Route path="/notifications" element={<NotificationsPage />} />
    <Route path="/saved" element={<SavedPage />} />
    <Route path="/saved/:kind/:id" element={<SavedResourcePage />} />
    <Route path="/watch-with-friends" element={<WatchWithFriendsRoute viewer={viewer} />} />
    <Route path="/watch/:id" element={<WatchPage />} />
    <Route path="*" element={<NotFoundPage />} />
  </Routes></Suspense>;
}

function AccountSessionTeardown() {
	const auth = useAuthSession();
	const playback = usePlaybackSession();
	useEffect(() => auth.registerSessionTeardown(playback.close), [auth.registerSessionTeardown, playback.close]);
	useEffect(() => auth.registerRuntimeTeardown('overlay', () => {
		window.dispatchEvent(new Event(VIEWER_TRANSITION_EVENT));
	}), [auth.registerRuntimeTeardown]);
	useEffect(() => auth.registerRuntimeTeardown('focus', () => {
		if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
	}), [auth.registerRuntimeTeardown]);
	useEffect(() => auth.registerRuntimeTeardown('artwork', () => {
		clearArtworkFailureCache();
		for (const image of document.querySelectorAll<HTMLImageElement>('#root img, #portico-overlays img')) image.removeAttribute('src');
		for (const element of document.querySelectorAll<HTMLElement>('#root [style*="background-image"], #portico-overlays [style*="background-image"]')) element.style.backgroundImage = 'none';
	}), [auth.registerRuntimeTeardown]);
	return null;
}

function ProductApp({ viewer }: { viewer: Viewer }) {
  const location = useLocation();
	const [blockingRouteFailure, setBlockingRouteFailure] = useState(false);
	useEffect(() => {
		return scheduleProductRoutePreload(() => { void preloadProductRoutes(); });
	}, []);
	return <WebDisplayPreferencesProvider><NotificationProvider><PlaybackSessionProvider>
	<AccountSessionTeardown />
	<SettingsNavigationBlocker />
    <AppShell viewer={viewer} player={<PlayerDock />} blockingRouteFailure={blockingRouteFailure}>
      <RouteErrorBoundary routeKey={`${location.pathname}${location.search}`} onBlockingStateChange={setBlockingRouteFailure}>
        <AppRoutes viewer={viewer} />
      </RouteErrorBoundary>
    </AppShell>
	</PlaybackSessionProvider></NotificationProvider></WebDisplayPreferencesProvider>;
}

export function App() {
  const auth = useAuthSession();
  if (auth.status === 'loading') return <AuthLoadingSurface />;
  if (auth.status === 'error') return <AuthFailureSurface message={reviewedProductErrorText(auth.error, 'auth.server-session-load-failed')} />;
  if (!auth.viewer) return <AuthFailureSurface message={productText('auth.server-session-load-failed')} />;
  if (auth.viewer.setupRequired) return <SetupSurface serverName={auth.viewer.serverName} />;
	if (auth.localProfileLogin) return <LocalProfileSelectionSurface serverName={auth.viewer.serverName} />;
	if (auth.addingAccount) return <SignInSurface serverName={auth.viewer.serverName} addingAccount />;
	if (!auth.viewer.authenticated) {
		if (auth.browserAccounts.status === 'loading' || (auth.busy && auth.browserAccounts.data.automaticSignIn && auth.browserAccounts.data.accounts.length > 0)) {
			return <AuthLoadingSurface title="Opening your account" detail="Checking accounts remembered on this browser…" />;
		}
		if (auth.browserAccounts.data.accounts.length > 0) return <AccountChooserSurface serverName={auth.viewer.serverName} />;
		// Remembered-account discovery is a convenience, not an authentication
		// dependency. If it is unavailable, keep manual sign-in usable instead of
		// replacing the entire auth surface with an optional lookup failure.
		return <SignInSurface serverName={auth.viewer.serverName} />;
	}
	return <ProductApp key={auth.viewerScopeKey || `unverified:${auth.sessionRevision}`} viewer={auth.viewer} />;
}
