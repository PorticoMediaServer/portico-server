import {createRootRoute, createRoute, createRouter, lazyRouteComponent} from '@tanstack/react-router';
import {gallerySearch} from './platform/gallery-search';
import {ShellRoot} from './shell/Root';
import {useShellBindings} from './shell/bindings';
import {HomeScreen} from './screens/home/Home';
import {librarySearch, searchQuery} from './shell/navigation';

function BoundHome() { const binding = useShellBindings(); return binding ? <HomeScreen {...binding.home} /> : null; }

export const rootRoute = createRootRoute({component: ShellRoot, notFoundComponent: () => null});
// Screen owners supply components in task 1.6 and later phases. These are structural outlets.
export const homeRoute = createRoute({getParentRoute: () => rootRoute, path: '/', component: BoundHome});
export const searchRoute = createRoute({getParentRoute: () => rootRoute, path: '/search', validateSearch: searchQuery});
export const librariesRoute = createRoute({getParentRoute: () => rootRoute, path: '/libraries'});
export const libraryRoute = createRoute({getParentRoute: () => rootRoute, path: '/library/$libraryId', validateSearch: librarySearch});
export const liveRoute = createRoute({getParentRoute: () => rootRoute, path: '/live', validateSearch: (raw: Record<string, unknown>): {sourceId?: string; group?: string} => ({...(typeof raw.sourceId === 'string' && raw.sourceId.length <= 256 ? {sourceId: raw.sourceId} : {}), ...(typeof raw.group === 'string' && raw.group.length <= 128 ? {group: raw.group} : {})})});
export const savedRoute = createRoute({getParentRoute: () => rootRoute, path: '/saved', validateSearch: (raw: Record<string, unknown>): {resourceId?: string} => typeof raw.resourceId === 'string' && raw.resourceId.length <= 256 ? {resourceId: raw.resourceId} : {}});
export const itemRoute = createRoute({getParentRoute: () => rootRoute, path: '/item/$itemId'});
export const homeRowRoute = createRoute({getParentRoute: () => rootRoute, path: '/home/rows/$rowId'});
export const playerRoute = createRoute({getParentRoute: () => rootRoute, path: '/player'});
export const settingsRoute = createRoute({getParentRoute: () => rootRoute, path: '/settings'});
export const dashboardRoute = createRoute({getParentRoute: () => rootRoute, path: '/dashboard', validateSearch: (raw: Record<string, unknown>): {section?: 'streams' | 'activity' | 'updates'} => ['streams', 'activity', 'updates'].includes(String(raw.section)) ? {section: raw.section as 'streams' | 'activity' | 'updates'} : {}});
export const notificationsRoute = createRoute({getParentRoute: () => rootRoute, path: '/notifications'});
export const profilesRoute = createRoute({getParentRoute: () => rootRoute, path: '/profiles', validateSearch: (raw: Record<string, unknown>): {profileId?: string; add?: boolean} => ({...(typeof raw.profileId === 'string' && raw.profileId.length <= 128 ? {profileId: raw.profileId} : {}), ...(raw.add === true ? {add: true} : {})})});
export const serversRoute = createRoute({getParentRoute: () => rootRoute, path: '/servers'});
export const accountRoute = createRoute({getParentRoute: () => rootRoute, path: '/account'});
export const signInRoute = createRoute({getParentRoute: () => rootRoute, path: '/sign-in'});
export const togetherRoute = createRoute({getParentRoute: () => rootRoute, path: '/watch-together'});
export const feedbackRoute = createRoute({getParentRoute: () => rootRoute, path: '/feedback'});
export const helpRoute = createRoute({getParentRoute: () => rootRoute, path: '/help'});
const destinations = [homeRoute, searchRoute, librariesRoute, libraryRoute, liveRoute, savedRoute, itemRoute, homeRowRoute, playerRoute, settingsRoute, dashboardRoute, notificationsRoute, profilesRoute, serversRoute, accountRoute, signInRoute, togetherRoute, feedbackRoute, helpRoute];
const devRoutes = import.meta.env.DEV ? [createRoute({
  getParentRoute: () => rootRoute, path: '/dev/kit', validateSearch: gallerySearch,
  component: lazyRouteComponent(() => import('./dev/KitGallery'), 'KitGallery'),
})] : [];
export const routeTree = rootRoute.addChildren([...destinations, ...devRoutes]);
export const router = createRouter({routeTree, defaultViewTransition: true});
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
