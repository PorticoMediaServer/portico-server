import {useRef} from 'react';
import {useRouteFade} from './platform/route-motion';
import {createRootRoute, createRoute, createRouter, Outlet, lazyRouteComponent, useRouterState} from '@tanstack/react-router';
import {gallerySearch} from './platform/gallery-search';

function RouteOutlet() {
  const target = useRef<HTMLDivElement>(null);
  const routeKey = useRouterState({select: state => state.location.pathname});
  useRouteFade(target, routeKey);
  return <div ref={target} data-route-content><Outlet /></div>;
}

export const rootRoute = createRootRoute({component: RouteOutlet, notFoundComponent: () => null});
// Screen owners supply components in task 1.6 and later phases. These are structural outlets.
export const homeRoute = createRoute({getParentRoute: () => rootRoute, path: '/'});
export const searchRoute = createRoute({getParentRoute: () => rootRoute, path: '/search', validateSearch: (raw: Record<string, unknown>) => ({q: typeof raw.q === 'string' ? raw.q : ''})});
export const librariesRoute = createRoute({getParentRoute: () => rootRoute, path: '/libraries'});
export const libraryRoute = createRoute({getParentRoute: () => rootRoute, path: '/library/$libraryId'});
export const liveRoute = createRoute({getParentRoute: () => rootRoute, path: '/live'});
export const savedRoute = createRoute({getParentRoute: () => rootRoute, path: '/saved'});
export const settingsRoute = createRoute({getParentRoute: () => rootRoute, path: '/settings'});
export const dashboardRoute = createRoute({getParentRoute: () => rootRoute, path: '/dashboard'});
export const notificationsRoute = createRoute({getParentRoute: () => rootRoute, path: '/notifications'});
export const profilesRoute = createRoute({getParentRoute: () => rootRoute, path: '/profiles'});
export const serversRoute = createRoute({getParentRoute: () => rootRoute, path: '/servers'});
export const accountRoute = createRoute({getParentRoute: () => rootRoute, path: '/account'});
export const signInRoute = createRoute({getParentRoute: () => rootRoute, path: '/sign-in'});
const destinations = [homeRoute, searchRoute, librariesRoute, libraryRoute, liveRoute, savedRoute, settingsRoute, dashboardRoute, notificationsRoute, profilesRoute, serversRoute, accountRoute, signInRoute];
const devRoutes = import.meta.env.DEV ? [createRoute({
  getParentRoute: () => rootRoute, path: '/dev/kit', validateSearch: gallerySearch,
  component: lazyRouteComponent(() => import('./dev/KitGallery'), 'KitGallery'),
})] : [];
export const routeTree = rootRoute.addChildren([...destinations, ...devRoutes]);
export const router = createRouter({routeTree, defaultViewTransition: true});
declare module '@tanstack/react-router' { interface Register { router: typeof router } }
