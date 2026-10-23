import {createRootRoute, createRoute, createRouter, lazyRouteComponent} from '@tanstack/react-router';
import {WebScreenOutlet, webScreens} from './route-registry-view';
import type {WebScreenId, WebScreenSearchValidator} from './route-registry';
import {gallerySearch} from './platform/gallery-search';
import {ShellRoot} from './shell/Root';
import {useShellBindings} from './shell/bindings';
import {HomeScreen} from './screens/home/Home';
import {librarySearch, searchQuery} from './shell/navigation';

function BoundHome() { const binding = useShellBindings(); return binding ? <HomeScreen {...binding.home} /> : null; }

export const rootRoute = createRootRoute({component: ShellRoot, notFoundComponent: () => null});
// Each screen owns its component/registration; the shared router owns only stable URLs.
const screenSearch = (id: WebScreenId, fallback?: WebScreenSearchValidator) => (raw: Record<string, unknown>) => webScreens.validateSearch(id, raw, fallback);
const screen = (id: WebScreenId) => function RegisteredScreen() {return <WebScreenOutlet id={id} />;};
export const homeRoute = createRoute({getParentRoute: () => rootRoute, path: '/', component: () => <WebScreenOutlet id="home" fallback={<BoundHome />} />, validateSearch: screenSearch('home')});
export const searchRoute = createRoute({getParentRoute: () => rootRoute, component: screen('search'), path: '/search', validateSearch: screenSearch('search', searchQuery)});
export const librariesRoute = createRoute({getParentRoute: () => rootRoute, component: screen('libraries'), path: '/libraries', validateSearch: screenSearch('libraries')});
export const libraryRoute = createRoute({getParentRoute: () => rootRoute, component: screen('library'), path: '/library/$libraryId', validateSearch: screenSearch('library', librarySearch)});
export const liveRoute = createRoute({getParentRoute: () => rootRoute, component: screen('live'), path: '/live', validateSearch: screenSearch('live', (raw: Record<string, unknown>): {sourceId?: string; group?: string} => ({...(typeof raw.sourceId === 'string' && raw.sourceId.length <= 256 ? {sourceId: raw.sourceId} : {}), ...(typeof raw.group === 'string' && raw.group.length <= 128 ? {group: raw.group} : {})}))});
export const savedRoute = createRoute({getParentRoute: () => rootRoute, component: screen('saved'), path: '/saved', validateSearch: screenSearch('saved', (raw: Record<string, unknown>): {resourceId?: string} => typeof raw.resourceId === 'string' && raw.resourceId.length <= 256 ? {resourceId: raw.resourceId} : {})});
export const itemRoute = createRoute({getParentRoute: () => rootRoute, component: screen('item'), path: '/item/$itemId', validateSearch: screenSearch('item')});
export const homeRowRoute = createRoute({getParentRoute: () => rootRoute, component: screen('homeRow'), path: '/home/rows/$rowId', validateSearch: screenSearch('homeRow')});
export const playerRoute = createRoute({getParentRoute: () => rootRoute, component: screen('player'), path: '/player', validateSearch: screenSearch('player')});
export const settingsRoute = createRoute({getParentRoute: () => rootRoute, component: screen('settings'), path: '/settings', validateSearch: screenSearch('settings')});
export const dashboardRoute = createRoute({getParentRoute: () => rootRoute, component: screen('dashboard'), path: '/dashboard', validateSearch: screenSearch('dashboard', (raw: Record<string, unknown>): {section?: 'streams' | 'activity' | 'updates'} => ['streams', 'activity', 'updates'].includes(String(raw.section)) ? {section: raw.section as 'streams' | 'activity' | 'updates'} : {})});
export const notificationsRoute = createRoute({getParentRoute: () => rootRoute, component: screen('notifications'), path: '/notifications', validateSearch: screenSearch('notifications')});
export const profilesRoute = createRoute({getParentRoute: () => rootRoute, component: screen('profiles'), path: '/profiles', validateSearch: screenSearch('profiles', (raw: Record<string, unknown>): {profileId?: string; add?: boolean} => ({...(typeof raw.profileId === 'string' && raw.profileId.length <= 128 ? {profileId: raw.profileId} : {}), ...(raw.add === true ? {add: true} : {})}))});
export const serversRoute = createRoute({getParentRoute: () => rootRoute, component: screen('servers'), path: '/servers', validateSearch: screenSearch('servers')});
export const accountRoute = createRoute({getParentRoute: () => rootRoute, component: screen('account'), path: '/account', validateSearch: screenSearch('account')});
export const signInRoute = createRoute({getParentRoute: () => rootRoute, component: screen('signIn'), path: '/sign-in', validateSearch: screenSearch('signIn')});
export const togetherRoute = createRoute({getParentRoute: () => rootRoute, component: screen('together'), path: '/watch-together', validateSearch: screenSearch('together')});
export const feedbackRoute = createRoute({getParentRoute: () => rootRoute, component: screen('feedback'), path: '/feedback', validateSearch: screenSearch('feedback')});
export const helpRoute = createRoute({getParentRoute: () => rootRoute, component: screen('help'), path: '/help', validateSearch: screenSearch('help')});
export const personRoute = createRoute({getParentRoute: () => rootRoute, component: screen('person'), path: '/person/$personId', validateSearch: screenSearch('person')});
export const readRoute = createRoute({getParentRoute: () => rootRoute, component: screen('read'), path: '/read/$itemId', validateSearch: screenSearch('read')});
const destinations = [homeRoute, searchRoute, librariesRoute, libraryRoute, liveRoute, savedRoute, itemRoute, homeRowRoute, playerRoute, settingsRoute, dashboardRoute, notificationsRoute, profilesRoute, serversRoute, accountRoute, signInRoute, togetherRoute, feedbackRoute, helpRoute, personRoute, readRoute];
const devRoutes = import.meta.env.DEV ? [createRoute({
  getParentRoute: () => rootRoute, path: '/dev/kit', validateSearch: gallerySearch,
  component: lazyRouteComponent(() => import('./dev/KitGallery'), 'KitGallery'),
})] : [];
export const routeTree = rootRoute.addChildren([...destinations, ...devRoutes]);
export const router = createRouter({routeTree, defaultViewTransition: true});
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
