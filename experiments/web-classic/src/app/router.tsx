import React, {lazy, Suspense} from 'react';
import {createRootRoute, createRoute, createRouter, Navigate, Outlet, redirect} from '@tanstack/react-router';
import {DataProvider} from '@/data/DataProvider';
import {TooltipProvider, ToastProvider} from '@/ui/Overlay';
import {PlayerProvider} from '@/player/PlayerProvider';
import {Shell} from '@/shell/Shell';
import {PageLoading} from '@/screens/common';
import {Splash} from './Boot';
import {PrefsProvider} from './prefs';
import {SessionProvider, useSession} from './session';
import {SelectionProvider} from './selection';
import {ActionsProvider} from './actions';
import {DemoProvider} from './demo';

// Each area is its own chunk, so the first paint only pays for the shell and Home.
const split = (load: () => Promise<{default: React.ComponentType}>) => {
  const L = lazy(load);
  return function Split() { return <Suspense fallback={<PageLoading />}><L /></Suspense>; };
};
const Home = split(() => import('@/screens/home/Home'));
const HomeRow = split(() => import('@/screens/home/HomeRow'));
const Library = split(() => import('@/screens/library/Library'));
const Entity = split(() => import('@/screens/library/Entity'));
const Media = split(() => import('@/screens/detail/Media'));
const Show = split(() => import('@/screens/detail/Show'));
const Person = split(() => import('@/screens/detail/Person'));
const Search = split(() => import('@/screens/search/Search'));
const Saved = split(() => import('@/screens/saved/Saved'));
const Resource = split(() => import('@/screens/saved/Resource'));
const Channels = split(() => import('@/screens/live/Channels'));
const Downloads = split(() => import('@/screens/downloads/Downloads'));
const Together = split(() => import('@/screens/together/Together'));
const Settings = split(() => import('@/screens/settings/Settings'));
const Account = split(() => import('@/screens/account/Account'));
const Server = split(() => import('@/screens/server/Server'));
const NotFound = split(() => import('@/screens/NotFound'));
const Auth = split(() => import('@/screens/auth/Auth'));
const ScreenIndex = split(() => import('@/screens/dev/ScreenIndex'));

const root = createRootRoute({
  component: () => (
    <PrefsProvider>
      <div className="atmosphere" aria-hidden />
      <DataProvider fallback={<Splash />}>
        <SessionProvider>
          <DemoProvider>
          <TooltipProvider delayDuration={400}>
            <ToastProvider>
              <Outlet />
            </ToastProvider>
          </TooltipProvider>
          </DemoProvider>
        </SessionProvider>
      </DataProvider>
    </PrefsProvider>
  ),
  notFoundComponent: () => <NotFound />,
});

/** Signed-in area: the shell with the player, actions and selection available everywhere inside it. */
function AppGate() {
  const session = useSession();
  if (session.phase === 'booting') return <Splash />;
  // Signing out clears the session a moment before the phase changes; don't render the shell without one.
  if (session.phase === 'signedIn' && !session.session) return <Splash />;
  if (session.phase === 'signedOut') return <Navigate to={'/welcome' as never} search={{next: location.pathname + location.search} as never} replace />;
  return (
    <PlayerProvider>
      <SelectionProvider>
        <ActionsProvider>
          <Shell />
        </ActionsProvider>
      </SelectionProvider>
    </PlayerProvider>
  );
}
const app = createRoute({getParentRoute: () => root, id: 'app', component: AppGate, notFoundComponent: () => <NotFound />});

type Q = Record<string, unknown>;
const str = (v: unknown) => (typeof v === 'string' && v.length <= 512 ? v : typeof v === 'number' ? String(v) : undefined);
const passSearch = (raw: Q): Record<string, string | undefined> => Object.fromEntries(Object.entries(raw).map(([k, v]) => [k, str(v)]));

const r = (path: string, component: () => React.ReactNode, parent: 'app' | 'root' = 'app') => createRoute({getParentRoute: () => (parent === 'app' ? app : root), path, component, validateSearch: passSearch});

const appRoutes = [
  r('/', Home),
  r('/home/rows/$rowId', HomeRow),
  r('/library/$libraryId', Library),
  r('/library/$libraryId/$view/$entityId', Entity),
  r('/media/$itemId', Media),
  r('/show/$showId', Show),
  r('/show/$showId/episode/$episodeId', Show),
  r('/person/$personId', Person),
  r('/search', Search),
  r('/saved', Saved),
  r('/saved/$kind/$resourceId', Resource),
  r('/channels', Channels),
  r('/channels/$entry', Channels),
  createRoute({getParentRoute: () => app, path: '/live', beforeLoad: ({search}) => { const v = (search as Q).view; throw redirect({to: (v === 'dvr' ? '/channels/recordings' : '/channels') as never, replace: true}); }}),
  r('/downloads', Downloads),
  r('/together', Together),
  r('/settings', Settings),
  r('/settings/$section', Settings),
  r('/account', Account),
  createRoute({getParentRoute: () => app, path: '/server', beforeLoad: () => { throw redirect({to: '/server/overview' as never, replace: true}); }}),
  r('/server/$section', Server),
  r('/server/$section/$id', Server),
];

/** Signed-out and link routes: sign-in, invitations, device approval, email links, first run. */
const authPaths = ['/welcome', '/sign-in', '/sign-in/$step', '/join', '/device', '/activate', '/claim', '/setup', '/restore', '/verify-email', '/reset', '/together/join', '/app/account-return', '/unavailable', '/offline'];
const authRoutes = authPaths.map(p => r(p, Auth, 'root'));

/** Search params are plain strings in the URL (no JSON quoting), so links stay readable: ?view=browse&day=1. */
const parseSearch = (search: string) => Object.fromEntries(new URLSearchParams(search.startsWith('?') ? search.slice(1) : search));
const stringifySearch = (search: Record<string, unknown>) => {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(search)) if (v !== undefined && v !== null && v !== '') params.set(k, typeof v === 'string' ? v : JSON.stringify(v));
  const out = params.toString();
  return out ? `?${out}` : '';
};

export const router = createRouter({
  parseSearch,
  stringifySearch,
  routeTree: root.addChildren([app.addChildren(appRoutes), ...authRoutes, r('/dev/screens', ScreenIndex, 'root')]),
  defaultPreload: 'intent',
  scrollRestoration: true,
  defaultPendingMinMs: 0,
});
