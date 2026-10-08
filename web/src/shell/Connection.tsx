import React, {useEffect, useState, useSyncExternalStore} from 'react';
import {friendlyHost} from '@core/presentation/index.ts';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {StatusScreen} from '../ui';
import s from './Shell.module.css';

/**
 * The shell's failure mode for a server it can't use. The shell always stays open (rail,
 * Settings, Account, switching profile or server). A connection lost for more than a moment
 * fails the page as a whole: one centered "Reconnecting" status replaces the content area
 * (never a card-by-card wall of errors), and the page comes back by itself, reloading its
 * data, when the route is ready again.
 *
 * - The remembered server couldn't be reached at launch (`serverProblem`): "Can't reach" with
 *   Try again while Portico keeps retrying. A sign-in that has ended never shows here: the app
 *   goes straight to the sign-in screen (`signInHint`, app/session.tsx).
 * - A server that was working drops off (route offline, or an address answering with another
 *   identity, or a proxy answering 502/503/504 for it): "Reconnecting", after a moment so a
 *   blip never flashes one.
 */
export function ConnectionOverlay({children}: {children: React.ReactNode}) {
  const {api, serverProblem, retryRestore, servers, system, serverUrl} = useSession();
  const i18n = useI18n();
  const route = useSyncExternalStore(api.subscribeRoute, api.getRouteSnapshot);
  const unavailable = !serverProblem && !!route && (route.phase === 'offline' || route.phase === 'identity_mismatch' || route.phase === 'probing');
  const [shown, setShown] = useState(false);
  useEffect(() => {
    if (!unavailable) {
      setShown(false);
      return;
    }
    const id = setTimeout(() => setShown(true), 2500);
    return () => clearTimeout(id);
  }, [unavailable]);
  const name = system?.name ?? servers.find(x => x.id === serverProblem?.serverId)?.name ?? friendlyHost(serverProblem?.serverUrl ?? serverUrl) ?? i18n.t('web.settings.serverFallback');
  // A live route that proves a different identity may be an impostor at the address: that reads
  // as unreachable, never as an ended sign-in.
  if (serverProblem?.kind === 'unreachable' || shown) {
    const retry = serverProblem ? retryRestore : () => void api.recoverRoute().catch(() => {});
    return (
      <div className={s.connectionStatus}>
        <StatusScreen
          title={i18n.t('launch.reconnectingTitle')}
          body={i18n.t('web.connection.reconnectingBody', {server: name})}
          action={{label: i18n.t('action.tryAgain'), onClick: retry}}
        />
      </div>
    );
  }
  return <>{children}</>;
}
