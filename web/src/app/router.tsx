import {serverSettingsAddress} from './server-pages';
import React, {Suspense, lazy} from 'react';
import {createRootRoute, createRoute, createRouter, Outlet, redirect} from '@tanstack/react-router';
import {PreferencesProvider} from './preferences';
import {SessionProvider} from './session';
import {Gate} from '../screens/auth/Gate';
import {Shell} from '../shell/Shell';
import {PageSkeleton} from '../shell/ShellSkeleton';
import {DeviceApprovalScreen} from '../screens/auth/DeviceApproval';
import {HomeScreen} from '../screens/home/Home';
import {HomeRowPageScreen} from '../screens/home/HomeRowPage';

// Route-split the areas most viewers never open in a session, so the first
// paint does not pay for the server console, settings or the guide.
// While a chunk loads the content area keeps a page's shape (a title line and card rows), so a
// navigation never shows a blank frame between the click and the page.
const split = (load: () => Promise<React.ComponentType<any>>) => {
  const Lazy = lazy<React.ComponentType<any>>(async () => ({default: await load()}));
  return function Split() { return <Suspense fallback={<PageSkeleton />}><Lazy /></Suspense>; };
};
const LibraryScreen = split(() => import('../screens/library/Library').then(m => m.LibraryScreen));
const DetailScreen = split(() => import('../screens/detail/Detail').then(m => m.DetailScreen));
const ShowScreen = split(() => import('../screens/detail/Show').then(m => m.ShowScreen));
const EntityScreen = split(() => import('../screens/library/Entity').then(m => m.EntityScreen));
const SearchScreen = split(() => import('../screens/search/Search').then(m => m.SearchScreen));
const PersonScreen = split(() => import('../screens/people/Person').then(m => m.PersonScreen));
const SavedScreen = split(() => import('../screens/saved/Saved').then(m => m.SavedScreen));
const SavedResourceScreen = split(() => import('../screens/saved/Resource').then(m => m.SavedResourceScreen));
const ChannelsScreen = split(() => import('../screens/channels/Channels').then(m => m.ChannelsScreen));
const SettingsScreen = split(() => import('../screens/settings/Settings').then(m => m.SettingsScreen));
const SettingsSectionScreen = split(() => import('../screens/settings/Settings').then(m => m.SettingsSectionScreen));
const AccountWorkspace = split(() => import('../screens/account/Account').then(m => m.AccountWorkspace));
const TogetherScreen = split(() => import('../screens/together/Together').then(m => m.TogetherScreen));
const ResetPasswordScreen = split(() => import('../screens/auth/LinkRoutes').then(m => m.ResetPasswordScreen));
const VerifyEmailScreen = split(() => import('../screens/auth/LinkRoutes').then(m => m.VerifyEmailScreen));
const JoinScreen = split(() => import('../screens/auth/LinkRoutes').then(m => m.JoinScreen));
const TogetherJoinScreen = split(() => import('../screens/auth/LinkRoutes').then(m => m.TogetherJoinScreen));
const AccountReturnScreen = split(() => import('../screens/auth/AccountReturn').then(m => m.AccountReturnScreen));
const ClaimApprovalScreen = split(() => import('../screens/auth/ClaimApproval').then(m => m.ClaimApprovalScreen));
const DownloadsScreen = split(() => import('../screens/downloads/Downloads').then(m => m.DownloadsScreen));
const GalleryScreen = split(() => import('../screens/dev/Gallery').then(m => m.GalleryScreen));
const WindowedLab = split(() => import('../dev/WindowedLab').then(m => m.WindowedLab));
const ChannelsLab = split(() => import('../dev/ChannelsLab').then(m => m.ChannelsLab));
const BrowseLab = split(() => import('../dev/BrowseLab').then(m => m.BrowseLab));
const FeaturesGallery = split(() => import('../screens/dev/Features').then(m => m.FeaturesGallery));
import {InAppNotFound, NotFoundScreen, RouteErrorScreen} from '../screens/NotFound';

/**
 * Route table. URLs are the product's durable destinations: every library
 * pivot, detail page and settings section is linkable and survives reload.
 * Search text is deliberately not a route parameter.
 */
const root = createRootRoute({
  component: () => (
    <PreferencesProvider>
      <SessionProvider>
        <div className="atmosphere" aria-hidden />
        <Outlet />
      </SessionProvider>
    </PreferencesProvider>
  ),
  notFoundComponent: NotFoundScreen,
  errorComponent: RouteErrorScreen,
});

const app = createRoute({
  getParentRoute: () => root,
  id: 'app',
  notFoundComponent: InAppNotFound,
  component: () => (
    <Gate>
      <Shell />
    </Gate>
  ),
});

const str = (v: unknown) => (typeof v === 'string' && v.length <= 256 ? v : undefined);
const oneOf = <T extends string>(v: unknown, allowed: readonly T[]): T | undefined => (allowed.includes(v as T) ? (v as T) : undefined);

// The tabs of a library (client-core `library-tabs.ts`).
const libraryViews = ['discover', 'browse', 'aired', 'collections', 'releases', 'songs', 'genres', 'playlists', 'authors', 'series', 'unmatched'] as const;
export type LibraryView = (typeof libraryViews)[number];
export type LibrarySearch = {view?: LibraryView; sort?: string; direction?: 'asc' | 'desc'; category?: string; filters?: string; row?: string};
export const home = createRoute({getParentRoute: () => app, path: '/', component: HomeScreen});
/** A Home row's See all (Recommended, Trending now, a personal row): the row as a paged grid. */
export const homeRow = createRoute({getParentRoute: () => app, path: '/home/rows/$rowId', component: HomeRowPageScreen});
export const library = createRoute({
  getParentRoute: () => app,
  path: '/library/$libraryId',
  validateSearch: (raw: Record<string, unknown>): LibrarySearch => ({view: oneOf(raw.view, libraryViews), sort: str(raw.sort), direction: oneOf(raw.direction, ['asc', 'desc'] as const), category: str(raw.category), row: str(raw.row), filters: typeof raw.filters === 'string' && raw.filters.length <= 2048 ? raw.filters : Array.isArray(raw.filters) && JSON.stringify(raw.filters).length <= 2048 ? JSON.stringify(raw.filters) : undefined}),
  component: LibraryScreen,
});
export const entity = createRoute({
  getParentRoute: () => app,
  path: '/library/$libraryId/$view/$entityId',
  validateSearch: (raw: Record<string, unknown>): {title?: string} => ({title: str(raw.title)}),
  component: EntityScreen,
});
export const media = createRoute({
  getParentRoute: () => app,
  path: '/media/$itemId',
  validateSearch: (raw: Record<string, unknown>): {library?: string} => ({library: str(raw.library)}),
  component: DetailScreen,
});
export const show = createRoute({
  getParentRoute: () => app,
  path: '/show/$showId',
  validateSearch: (raw: Record<string, unknown>): {library?: string; season?: string; episode?: string; title?: string} => ({library: str(raw.library), season: str(raw.season), episode: str(raw.episode), title: str(raw.title)}),
  component: ShowScreen,
});
/** The episode panel over its show (Spec — Title Pages §3); a direct visit opens the show with the panel open. */
export const showEpisode = createRoute({
  getParentRoute: () => app,
  path: '/show/$showId/episode/$episodeId',
  validateSearch: (raw: Record<string, unknown>): {library?: string; season?: string; episode?: string; title?: string} => ({library: str(raw.library), season: str(raw.season), episode: str(raw.episode), title: str(raw.title)}),
  component: ShowScreen,
});
/** A search is linkable: the words and the type chip are in the address. */
export type SearchAddress = {q?: string; type?: string};
export const search = createRoute({getParentRoute: () => app, path: '/search', component: SearchScreen, validateSearch: (raw: Record<string, unknown>): SearchAddress => ({q: str(raw.q), type: oneOf(raw.type, ['movies', 'shows', 'episodes', 'people', 'artists', 'albums', 'songs', 'books', 'live-tv'] as const)})});
const unknown = createRoute({getParentRoute: () => app, path: '$', component: InAppNotFound});
export const person = createRoute({getParentRoute: () => app, path: '/person/$personId', component: PersonScreen});
export const saved = createRoute({
  getParentRoute: () => app,
  path: '/saved',
  validateSearch: (raw: Record<string, unknown>): {view?: string; sort?: string; direction?: 'asc' | 'desc'; filter?: 'all' | 'unwatched' | 'inProgress'; period?: '24h' | '7d' | '30d' | '90d' | 'all'} => ({view: str(raw.view), sort: str(raw.sort), direction: oneOf(raw.direction, ['asc', 'desc'] as const), filter: oneOf(raw.filter, ['all', 'unwatched', 'inProgress'] as const), period: oneOf(raw.period, ['24h', '7d', '30d', '90d', 'all'] as const)}),
  component: SavedScreen,
});
export const savedResource = createRoute({
  getParentRoute: () => app,
  path: '/saved/$kind/$resourceId',
  validateSearch: (raw: Record<string, unknown>): {title?: string} => ({title: str(raw.title)}),
  component: SavedResourceScreen,
});
/** Live TV became Channels (Spec — Channels and Guide §2): old links land on the matching entry. */
export const live = createRoute({
  getParentRoute: () => app,
  path: '/live',
  validateSearch: (raw: Record<string, unknown>): {view?: 'guide' | 'channels' | 'dvr'} => ({view: oneOf(raw.view, ['guide', 'channels', 'dvr'] as const)}),
  beforeLoad: ({search}) => {
    if (search.view === 'dvr') throw redirect({to: '/channels/$entry', params: {entry: 'recordings'}, replace: true});
    if (search.view === 'channels') throw redirect({to: '/channels/$entry', params: {entry: 'library'}, replace: true});
    throw redirect({to: '/channels', replace: true});
  },
});
const channelsSearch = (raw: Record<string, unknown>): {view?: 'guide' | 'list'; group?: string; favorites?: boolean} => ({
  view: oneOf(raw.view, ['guide', 'list'] as const),
  group: typeof raw.group === 'string' && raw.group.length <= 200 ? raw.group : undefined,
  favorites: raw.favorites === true || raw.favorites === 'true' ? true : undefined,
});
export const channels = createRoute({getParentRoute: () => app, path: '/channels', component: ChannelsScreen});
export const channelsEntry = createRoute({getParentRoute: () => app, path: '/channels/$entry', validateSearch: channelsSearch, component: ChannelsScreen});
export const settings = createRoute({getParentRoute: () => app, path: '/settings', component: SettingsScreen});
export const settingsSection = createRoute({
  getParentRoute: () => app,
  path: '/settings/$section',
  // `id` and `tab` address a thing inside a page (a library, an account) so it is linkable.
  validateSearch: (raw: Record<string, unknown>): {id?: string; tab?: string} => ({id: str(raw.id), tab: str(raw.tab)}),
  component: SettingsSectionScreen,
});
// The console's addresses from before the one Settings screen: each lands on the page that took the section over.
export const server = createRoute({getParentRoute: () => app, path: '/server', beforeLoad: () => { throw redirect({to: '/settings/$section', params: {section: 'server-dashboard'}, search: {}, replace: true}); }});
export const serverSection = createRoute({
  getParentRoute: () => app,
  path: '/server/$section',
  validateSearch: (raw: Record<string, unknown>): {id?: string; tab?: string} => ({id: str(raw.id), tab: str(raw.tab)}),
  beforeLoad: ({params, search}) => { throw redirect({to: '/settings/$section', params: {section: serverSettingsAddress(params.section)}, search, replace: true}); },
});
export const serverAlias = createRoute({getParentRoute: () => app, path: '/server/$section/$id', beforeLoad: ({params}) => { throw redirect({to: '/settings/$section', params: {section: serverSettingsAddress(params.section)}, search: {id: params.id}, replace: true}); }});

const device = createRoute({getParentRoute: () => root, path: '/device', component: DeviceApprovalScreen});
/** Portico Account site: approve connecting a server (opened from Server › General). */
const claim = createRoute({getParentRoute: () => root, path: '/claim', component: ClaimApprovalScreen});
// Link contract: email and share links (secrets in the fragment, scrubbed on arrival).
const reset = createRoute({getParentRoute: () => root, path: '/reset', component: ResetPasswordScreen});
const verifyEmail = createRoute({getParentRoute: () => root, path: '/verify-email', component: VerifyEmailScreen});
const join = createRoute({getParentRoute: () => root, path: '/join', component: JoinScreen});
const downloads = createRoute({getParentRoute: () => app, path: '/downloads', component: DownloadsScreen});
/** NEW-12: the Portico Account workspace lives under the app route (path stays
 * `/account`), so a server session shows it inside the product shell. With no
 * server session Gate exempts it back to its own light shell. */
const account = createRoute({getParentRoute: () => app, path: '/account', validateSearch: (raw: Record<string, unknown>): {tab?: 'sessions' | 'security' | 'servers' | 'notifications'; identityTransaction?: string} => ({tab: oneOf(raw.tab, ['sessions', 'security', 'servers', 'notifications'] as const), ...(typeof raw.identityTransaction === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(raw.identityTransaction) ? {identityTransaction: raw.identityTransaction} : {})}), component: AccountWorkspace});
const together = createRoute({getParentRoute: () => app, path: '/together', component: TogetherScreen});
const togetherJoin = createRoute({getParentRoute: () => root, path: '/together/join', component: TogetherJoinScreen});
// SEC-01: a Portico Account's return from an app's provider sign-in (continues into the app).
const accountReturn = createRoute({getParentRoute: () => root, path: '/app/account-return', component: AccountReturnScreen});
const gallery = createRoute({getParentRoute: () => root, path: '/gallery', component: GalleryScreen});
const features = createRoute({getParentRoute: () => root, path: '/gallery/features', component: FeaturesGallery});
const windowedLab = createRoute({getParentRoute: () => root, path: '/gallery/windowed', component: WindowedLab});
const channelsLab = createRoute({getParentRoute: () => root, path: '/gallery/channels', component: ChannelsLab});
const browseLab = createRoute({getParentRoute: () => root, path: '/gallery/browse', component: BrowseLab});

export const router = createRouter({
  routeTree: root.addChildren([app.addChildren([home, homeRow, library, entity, media, show, showEpisode, search, unknown, person, saved, savedResource, downloads, together, live, channels, channelsEntry, settings, settingsSection, server, serverSection, serverAlias, account]), device, claim, reset, verifyEmail, join, togetherJoin, accountReturn, ...(import.meta.env.DEV ? [gallery, features, windowedLab, browseLab, channelsLab] : [])]),
  defaultPreload: 'intent',
  scrollRestoration: true,
  defaultViewTransition: false,
  defaultErrorComponent: RouteErrorScreen,
});
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
