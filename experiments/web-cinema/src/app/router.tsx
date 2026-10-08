import React, {Suspense, lazy} from 'react';
import {createRootRoute, createRoute, createRouter, Outlet, redirect} from '@tanstack/react-router';
import {ServicesProvider} from '../data/hooks';
import {createFixtureServices} from '../data/fixture/backend';
import {setDurationResolver} from './player';
import {SessionProvider} from './session';
import {Gate} from './Gate';
import {Shell} from '../shell/Shell';
import {Toaster} from '../ui/Feedback';
import {TooltipProvider} from '../ui/Overlay';
import {NotFound, RouteError} from '../screens/NotFound';

/** The data layer: swap `createFixtureServices()` for real Portico services to ship. */
export const services = createFixtureServices();
setDurationResolver(id => services.catalog.durationOf(id));

const split = <T extends string>(load: () => Promise<Record<T, unknown>>, name: T) => {
  const Lazy = lazy(async () => ({default: (await load())[name] as React.ComponentType}));
  return function Split() {
    return <Suspense fallback={<div className="route-loading" aria-hidden />}><Lazy /></Suspense>;
  };
};

const Home = split(() => import('../screens/home/Home'), 'HomeScreen');
const HomeRow = split(() => import('../screens/home/HomeRow'), 'HomeRowScreen');
const Library = split(() => import('../screens/library/Library'), 'LibraryScreen');
const Entity = split(() => import('../screens/library/Entity'), 'EntityScreen');
const Media = split(() => import('../screens/detail/Media'), 'MediaScreen');
const Show = split(() => import('../screens/detail/Show'), 'ShowScreen');
const Person = split(() => import('../screens/person/Person'), 'PersonScreen');
const Search = split(() => import('../screens/search/Search'), 'SearchScreen');
const Saved = split(() => import('../screens/saved/Saved'), 'SavedScreen');
const SavedResource = split(() => import('../screens/saved/Resource'), 'ResourceScreen');
const Downloads = split(() => import('../screens/downloads/Downloads'), 'DownloadsScreen');
const Together = split(() => import('../screens/together/Together'), 'TogetherScreen');
const Channels = split(() => import('../screens/live/Channels'), 'ChannelsScreen');
const Settings = split(() => import('../screens/settings/Settings'), 'SettingsScreen');
const SettingsSection = split(() => import('../screens/settings/Settings'), 'SettingsSectionScreen');
const Account = split(() => import('../screens/account/Account'), 'AccountScreen');
const Server = split(() => import('../screens/server/Server'), 'ServerScreen');
const Screens = split(() => import('../screens/dev/Screens'), 'ScreensIndex');
const Auth = split(() => import('../screens/auth/Auth'), 'AuthScreen');
const Links = split(() => import('../screens/auth/Links'), 'LinkScreen');

const root = createRootRoute({
  component: () => (
    <ServicesProvider services={services}>
      <SessionProvider>
        <TooltipProvider>
          <div className="atmosphere" aria-hidden />
          <Outlet />
          <Toaster />
        </TooltipProvider>
      </SessionProvider>
    </ServicesProvider>
  ),
  notFoundComponent: NotFound,
  errorComponent: RouteError,
});

const app = createRoute({getParentRoute: () => root, id: 'app', component: () => <Gate><Shell /></Gate>, notFoundComponent: NotFound});

const str = (v: unknown) => (typeof v === 'string' && v.length <= 2048 ? v : undefined);
const oneOf = <T extends string>(v: unknown, allowed: readonly T[]): T | undefined => (allowed.includes(v as T) ? (v as T) : undefined);

export type LibrarySearch = {view?: string; sort?: string; direction?: 'asc' | 'desc'; filters?: string; letter?: string; layout?: 'grid' | 'list'};
const home = createRoute({getParentRoute: () => app, path: '/', component: Home, validateSearch: (r: Record<string, unknown>): {customize?: boolean} => ({customize: r.customize === true || r.customize === 'true' || r.customize === 1 ? true : undefined})});
const homeRow = createRoute({getParentRoute: () => app, path: '/home/rows/$rowId', component: HomeRow});
const library = createRoute({
  getParentRoute: () => app,
  path: '/library/$libraryId',
  validateSearch: (r: Record<string, unknown>): LibrarySearch => ({view: str(r.view), sort: str(r.sort), direction: oneOf(r.direction, ['asc', 'desc'] as const), filters: typeof r.filters === 'object' ? JSON.stringify(r.filters) : str(r.filters), letter: str(r.letter), layout: oneOf(r.layout, ['grid', 'list'] as const)}),
  component: Library,
});
const entity = createRoute({getParentRoute: () => app, path: '/library/$libraryId/$view/$entityId', component: Entity});
const media = createRoute({getParentRoute: () => app, path: '/media/$itemId', validateSearch: (r: Record<string, unknown>): {photo?: string; slideshow?: boolean} => ({photo: str(r.photo), slideshow: r.slideshow === true || r.slideshow === 'true' ? true : undefined}), component: Media});
export type ShowSearch = {season?: string};
const show = createRoute({getParentRoute: () => app, path: '/show/$showId', validateSearch: (r: Record<string, unknown>): ShowSearch => ({season: str(r.season)}), component: Show});
const showEpisode = createRoute({getParentRoute: () => app, path: '/show/$showId/episode/$episodeId', validateSearch: (r: Record<string, unknown>): ShowSearch => ({season: str(r.season)}), component: Show});
const person = createRoute({getParentRoute: () => app, path: '/person/$personId', component: Person});
const search = createRoute({getParentRoute: () => app, path: '/search', validateSearch: (r: Record<string, unknown>): {q?: string; kind?: string} => ({q: str(r.q), kind: str(r.kind)}), component: Search});
export type SavedSearch = {view?: string; sort?: string; direction?: 'asc' | 'desc'; filter?: 'all' | 'unwatched' | 'inProgress'; period?: '24h' | '7d' | '30d' | '90d' | 'all'};
const saved = createRoute({
  getParentRoute: () => app,
  path: '/saved',
  validateSearch: (r: Record<string, unknown>): SavedSearch => ({view: str(r.view), sort: str(r.sort), direction: oneOf(r.direction, ['asc', 'desc'] as const), filter: oneOf(r.filter, ['all', 'unwatched', 'inProgress'] as const), period: oneOf(r.period, ['24h', '7d', '30d', '90d', 'all'] as const)}),
  component: Saved,
});
const savedResource = createRoute({getParentRoute: () => app, path: '/saved/$kind/$resourceId', component: SavedResource});
const downloads = createRoute({getParentRoute: () => app, path: '/downloads', validateSearch: (r: Record<string, unknown>): {tab?: string} => ({tab: str(r.tab)}), component: Downloads});
const togetherRoute = createRoute({getParentRoute: () => app, path: '/together', validateSearch: (r: Record<string, unknown>): {group?: string} => ({group: str(r.group)}), component: Together});
/** Old Live TV links land on the matching Channels entry. */
const live = createRoute({
  getParentRoute: () => app,
  path: '/live',
  validateSearch: (r: Record<string, unknown>): {view?: string} => ({view: str(r.view)}),
  beforeLoad: ({search}) => {
    if (search.view === 'dvr') throw redirect({to: '/channels/$entry', params: {entry: 'recordings'}, replace: true});
    throw redirect({to: '/channels/$entry', params: {entry: 'all'}, replace: true});
  },
});
export type ChannelsSearch = {view?: 'guide' | 'list'; group?: string; favorites?: boolean; tab?: string; day?: string};
const channelsSearch = (r: Record<string, unknown>): ChannelsSearch => ({view: oneOf(r.view, ['guide', 'list'] as const), group: str(r.group), favorites: r.favorites === true || r.favorites === 'true' ? true : undefined, tab: str(r.tab), day: str(r.day)});
const channels = createRoute({getParentRoute: () => app, path: '/channels', beforeLoad: () => { throw redirect({to: '/channels/$entry', params: {entry: 'all'}, replace: true}); }});
const channelsEntry = createRoute({getParentRoute: () => app, path: '/channels/$entry', validateSearch: channelsSearch, component: Channels});
const settings = createRoute({getParentRoute: () => app, path: '/settings', component: Settings});
const settingsSection = createRoute({getParentRoute: () => app, path: '/settings/$section', component: SettingsSection});
const account = createRoute({getParentRoute: () => app, path: '/account', validateSearch: (r: Record<string, unknown>): {tab?: string} => ({tab: str(r.tab)}), component: Account});
const server = createRoute({getParentRoute: () => app, path: '/server', beforeLoad: () => { throw redirect({to: '/server/$section', params: {section: 'overview'}}); }});
const serverSection = createRoute({getParentRoute: () => app, path: '/server/$section', validateSearch: (r: Record<string, unknown>): {id?: string; tab?: string} => ({id: str(r.id), tab: str(r.tab)}), component: Server});

const unknown = createRoute({getParentRoute: () => app, path: '$', component: NotFound});

// Outside the app frame: getting in, and links from email or another device.
export type AuthSearch = {step?: string; error?: string; invite?: string; code?: string};
const authSearch = (r: Record<string, unknown>): AuthSearch => ({step: str(r.step), error: str(r.error), invite: str(r.invite), code: str(r.code)});
const auth = createRoute({getParentRoute: () => root, path: '/welcome/$screen', validateSearch: authSearch, component: Auth});
const signin = createRoute({getParentRoute: () => root, path: '/signin', beforeLoad: () => { throw redirect({to: '/welcome/$screen', params: {screen: 'signin'}}); }});
const profiles = createRoute({getParentRoute: () => root, path: '/profiles', beforeLoad: () => { throw redirect({to: '/welcome/$screen', params: {screen: 'profiles'}}); }});
const linkRoutes = ['device', 'claim', 'reset', 'verify-email', 'join', 'together/join', 'app/account-return'].map(path =>
  createRoute({getParentRoute: () => root, path: `/${path}`, validateSearch: authSearch, component: Links}),
);
const screens = createRoute({getParentRoute: () => root, path: '/dev/screens', component: Screens});

export const router = createRouter({
  routeTree: root.addChildren([
    app.addChildren([home, homeRow, library, entity, media, show, showEpisode, person, search, saved, savedResource, downloads, togetherRoute, live, channels, channelsEntry, settings, settingsSection, account, server, serverSection, unknown]),
    auth, signin, profiles, ...linkRoutes, screens,
  ]),
  defaultPreload: 'intent',
  scrollRestoration: true,
  defaultErrorComponent: RouteError,
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
