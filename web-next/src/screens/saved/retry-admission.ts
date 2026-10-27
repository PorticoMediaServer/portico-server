import type {SavedRoute, SavedService} from '@core/saved.ts';
import type {SavedRuntime} from './runtime.ts';
import {sameSavedRoute} from './route.ts';

/** One exact retry lifetime: the original runtime, normalized route and reader
 * identities. The ticket is render identity only; the layout-owned admission is
 * what authorizes a command. */
export type SavedRetryTicket = Readonly<{
  runtime: SavedRuntime;
  route: SavedRoute;
  routeKey: string;
  posters: SavedService;
  squares: SavedService;
  saved: SavedService;
  reportError: (error: unknown) => void;
}>;
export type SavedRetryAdmission = {ticket: SavedRetryTicket; live: boolean; pending: symbol | undefined};

export function createSavedRetryAdmission(ticket: SavedRetryTicket): SavedRetryAdmission {
  return {ticket, live: true, pending: undefined};
}

/** Dead and detached before any abort/select callback can reenter. */
export function retireSavedRetryAdmission(value: SavedRetryAdmission): void {
  value.live = false;
  value.pending = undefined;
}

/** The original route/readers must still own the current runtime, not merely the
 * parent's global operations flag. */
export function savedRetryCurrent(value: SavedRetryAdmission): boolean {
  if (!value.live || !value.ticket.runtime.operations.current) return false;
  const route = value.ticket.route;
  if (route.view === 'watchlist' || route.view === 'favorites')
    return sameSavedRoute(value.ticket.posters.getSnapshot().route, {...route, shape: 'posters'})
      && sameSavedRoute(value.ticket.squares.getSnapshot().route, {...route, shape: 'squares'});
  return sameSavedRoute(value.ticket.saved.getSnapshot().route, route);
}
