import type {SavedRoute} from '@core/saved.ts';

type Route = SavedRoute | Readonly<{view: 'downloads'}>;

/** The core normalizes query property order. A render must compare the actual
 * destination/query, independent of how the router constructed its object. */
export function savedRouteKey(route: Route): string {
  return JSON.stringify(Object.entries(route).filter(([, value]) => value !== undefined).sort(([a], [b]) => a.localeCompare(b)));
}

export function sameSavedRoute(actual: Route | null, expected: Route): boolean {
  return actual !== null && savedRouteKey(actual) === savedRouteKey(expected);
}
