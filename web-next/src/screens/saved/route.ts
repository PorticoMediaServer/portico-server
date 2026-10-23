import type {SavedRoute} from '@core/saved.ts';
import type {SavedDownloadsScope} from './downloads-port.ts';

type Route = SavedRoute | Readonly<{view: 'downloads'}>;

/** The core normalizes query property order. A render must compare the actual
 * destination/query, independent of how the router constructed its object. */
export function savedRouteKey(route: Route): string {
  return JSON.stringify(Object.entries(route).filter(([, value]) => value !== undefined).sort(([a], [b]) => a.localeCompare(b)));
}

export function sameSavedRoute(actual: Route | null, expected: Route): boolean {
  return actual !== null && savedRouteKey(actual) === savedRouteKey(expected);
}

/** Runtime-only identity for UI state, never a native/persisted storage key. */
export function savedViewKey(scope: SavedDownloadsScope, route: Route): string {
  return JSON.stringify([scope.serverId, scope.authority, scope.accountId, scope.profileId, scope.sessionFamilyId, savedRouteKey(route)]);
}
