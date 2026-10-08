import { Suspense, useEffect, useRef } from 'react';
import { Navigate, Outlet, useLocation, useNavigate, useNavigationType } from 'react-router';
import { WifiOff } from 'lucide-react';
import { useSession } from '../state/session';
import { usePlayer } from '../state/player';
import { useUi } from '../state/ui';
import { useIsMobile } from '../lib/hooks';
import { TopBar } from './TopBar';
import { TabBar } from './TabBar';
import { CommandPalette } from './CommandPalette';
import { NotificationsSheet } from './Notifications';
import { CastSheet } from './CastSheet';
import { ShortcutsSheet } from './Shortcuts';
import { AudioEngine } from '../features/audio/AudioEngine';
import { MiniPlayer } from '../features/audio/MiniPlayer';
import { NowPlaying } from '../features/audio/NowPlaying';
import { Toaster } from '../components/overlay';
import { PageFallback } from '../components/Fallbacks';

/** Redirects into the sign-in → server → profile flow until a session exists. */
export function RequireSession({ children }: { children: React.ReactNode }) {
  const { signedIn, serverId, profileId } = useSession();
  const loc = useLocation();
  if (!signedIn) return <Navigate to="/welcome" replace state={{ from: loc.pathname }} />;
  if (!serverId) return <Navigate to="/servers" replace />;
  if (!profileId) return <Navigate to="/profiles" replace />;
  return <>{children}</>;
}

/** Applies the user's transparency / motion overrides to the document. */
export function PreferencesBridge() {
  const transparency = useSession((s) => s.transparency);
  const motion = useSession((s) => s.motion);
  useEffect(() => {
    const root = document.documentElement;
    if (transparency === 'system') root.removeAttribute('data-transparency');
    else root.setAttribute('data-transparency', transparency);
    if (motion === 'system') root.removeAttribute('data-motion');
    else root.setAttribute('data-motion', motion);
  }, [transparency, motion]);
  return null;
}

/** Marks navigation direction so phones can play iOS push/pop transitions. */
function NavDirection() {
  const type = useNavigationType();
  const { pathname } = useLocation();
  const prev = useRef(pathname);
  useEffect(() => {
    const depth = (p: string) => p.split('/').filter(Boolean).length;
    const dir = type === 'POP' ? 'pop' : depth(pathname) > depth(prev.current) || /^\/(movie|show|episode|album|artist|playlist|audiobook|person|collection|home-video)\//.test(pathname) ? 'push' : 'fade';
    document.documentElement.dataset.nav = dir;
    prev.current = pathname;
  }, [pathname, type]);
  return null;
}

function ScrollToTop() {
  const { pathname } = useLocation();
  const type = useNavigationType();
  useEffect(() => {
    if (type !== 'POP') window.scrollTo({ top: 0, behavior: 'instant' as ScrollBehavior });
  }, [pathname, type]);
  return null;
}

function GlobalKeys() {
  const navigate = useNavigate();
  const openPalette = useUi((s) => s.openPalette);
  const setShortcuts = useUi((s) => s.setShortcuts);
  useEffect(() => {
    let g = 0;
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        openPalette();
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === '/') {
        e.preventDefault();
        openPalette();
        return;
      }
      if (e.key === '?') {
        setShortcuts(true);
        return;
      }
      if (e.key === ' ' && usePlayer.getState().mode && !document.querySelector('.sheet-root')) {
        e.preventDefault();
        usePlayer.getState().toggle();
        return;
      }
      if (e.key === 'g') {
        g = Date.now();
        return;
      }
      if (Date.now() - g < 900) {
        const map: Record<string, string> = { h: '/', m: '/movies', t: '/tv', a: '/anime', u: '/music', l: '/live', b: '/audiobooks', p: '/photos', w: '/watchlist', d: '/downloads', s: '/settings', c: '/console' };
        if (map[e.key]) {
          e.preventDefault();
          navigate(map[e.key], { viewTransition: true });
        }
        g = 0;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [navigate, openPalette, setShortcuts]);
  return null;
}

/**
 * Review hooks: ?open=palette|notifications|cast|shortcuts|mini|nowplaying|lyrics|queue|book
 * opens that overlay on load, so every state is linkable from the screen index.
 */
function OpenBridge() {
  const done = useRef(false);
  useEffect(() => {
    if (done.current) return;
    done.current = true;
    const open = new URLSearchParams(window.location.search).get('open');
    if (!open) return;
    const ui = useUi.getState();
    const p = usePlayer.getState();
    const music = async (panel?: 'lyrics' | 'queue', full = true) => {
      const { api } = await import('../services/client');
      const { album } = await api.getAlbum('northern-static');
      await p.playTracks(album.tracks, 0);
      usePlayer.setState({ position: 97 });
      if (panel) usePlayer.setState({ panel });
      if (full) p.openNowPlaying(true);
    };
    if (open === 'palette') ui.openPalette('mer');
    else if (open === 'notifications') ui.setNotifications(true);
    else if (open === 'cast') ui.setCast(true);
    else if (open === 'shortcuts') ui.setShortcuts(true);
    else if (open === 'mini') music(undefined, false);
    else if (open === 'nowplaying') music();
    else if (open === 'lyrics') music('lyrics');
    else if (open === 'queue') music('queue');
    else if (open === 'book')
      import('../services/client').then(async ({ api }) => {
        const { book } = await api.getAudiobook('pride-and-prejudice');
        await p.playBook(book, 3 * 3600 + 14 * 60);
        p.openNowPlaying(true);
      });
  }, []);
  return null;
}

const OFFLINE_OK = ['/downloads', '/settings', '/watch/'];

function OfflineBanner() {
  const offline = useUi((s) => s.offline);
  const setOffline = useUi((s) => s.setOffline);
  if (!offline) return null;
  return (
    <div className="offline-banner glass glass-thick" role="status">
      <WifiOff />
      <span>
        <b>Offline mode.</b> Only downloads are available.
      </span>
      <button className="linkish" onClick={() => setOffline(false)}>
        Go online
      </button>
    </div>
  );
}

function OfflineGate({ children }: { children: React.ReactNode }) {
  const offline = useUi((s) => s.offline);
  const { pathname } = useLocation();
  if (!offline || OFFLINE_OK.some((p) => pathname.startsWith(p))) return <>{children}</>;
  return <Navigate to="/downloads" replace />;
}

export function Shell() {
  const mobile = useIsMobile();
  const mode = usePlayer((s) => s.mode);
  return (
    <RequireSession>
      <div className={mode ? 'app has-mini' : 'app'}>
        <div className="canvas" aria-hidden="true" />
        {!mobile && <TopBar />}
        <main id="main">
          <OfflineBanner />
          <Suspense fallback={<PageFallback />}>
            <OfflineGate>
              <Outlet />
            </OfflineGate>
          </Suspense>
        </main>
        <MiniPlayer />
        {mobile && <TabBar />}
        <NowPlaying />
        <CommandPalette />
        <NotificationsSheet />
        <CastSheet />
        <ShortcutsSheet />
        <AudioEngine />
        <Toaster />
        <GlobalKeys />
        <OpenBridge />
        <NavDirection />
        <ScrollToTop />
      </div>
    </RequireSession>
  );
}

/** Bare layout for full-screen flows (sign-in, profiles, setup, player). */
export function Bare() {
  return (
    <div className="app bare">
      <div className="canvas" aria-hidden="true" />
      <Suspense fallback={<PageFallback bare />}>
        <Outlet />
      </Suspense>
      <Toaster />
      <NavDirection />
    </div>
  );
}
