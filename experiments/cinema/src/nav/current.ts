import {useSyncExternalStore} from 'react';
import {navigationRef, type RouteName} from './routes';
import {back} from './back';

/** The visible route, for the rail, tab bar and chrome decisions. */
let current: {name: RouteName; params?: Record<string, unknown>; depth: number} = {name: 'Landing', depth: 1};
const listeners = new Set<() => void>();

export function updateCurrentRoute() {
  const r = navigationRef.getCurrentRoute();
  const depth = navigationRef.getRootState()?.routes.length ?? 1;
  if (!r) return;
  current = {name: r.name as RouteName, params: r.params as Record<string, unknown> | undefined, depth};
  listeners.forEach(l => l());
  back.sync();
}

export function useCurrentRoute() {
  return useSyncExternalStore(
    l => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}

export const currentRoute = () => current;

// Navigation back: pop the stack when there is somewhere to go.
back.register({
  priority: 10,
  active: () => navigationRef.isReady() && navigationRef.canGoBack(),
  handle: () => navigationRef.goBack(),
});
