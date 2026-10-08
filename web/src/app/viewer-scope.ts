import {useMemo} from 'react';
import {viewerScope} from '@core/presentation/index.ts';
import {useSession} from './session';

/**
 * The viewer and server the page is for, stable across access-token rotation (about every
 * 13 minutes): a new session object with the same viewer must not look like a new scope, or
 * every service keyed on it reloads.
 */
export function useViewerScope(): {viewerId: string; serverId: string} {
  const {api, session} = useSession();
  const {viewerId, serverId} = viewerScope(session, api);
  return useMemo(() => ({viewerId, serverId}), [viewerId, serverId]);
}
