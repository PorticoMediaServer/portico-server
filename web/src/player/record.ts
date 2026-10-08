import {useEffect, useMemo} from 'react';
import {DVRClient} from '@core/dvr.ts';
import {useSession} from '../app/session';
import {useViewerScope} from '../app/viewer-scope';

/** 48 hex characters: the DVR's request-id grammar (mirrors the guide's client). */
function dvrRequestId(): Promise<string> {
  const bytes = crypto.getRandomValues(new Uint8Array(24));
  return Promise.resolve(Array.from(bytes, b => b.toString(16).padStart(2, '0')).join(''));
}

/**
 * MU4 FEAT-07: the in-player Record action schedules through the shared
 * `DVRClient`, like the guide sheet does. One client per signed-in server.
 */
export function usePlayerDVR(): DVRClient | null {
  const {api} = useSession();
  const scope = useViewerScope();
  const serverId = scope.serverId;
  const client = useMemo(() => (serverId ? new DVRClient(api, serverId, dvrRequestId) : null), [api, serverId]);
  useEffect(() => {
    if (!client) return;
    client.activate();
    return () => client.dispose();
  }, [client]);
  return client;
}
