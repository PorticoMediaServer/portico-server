import React, {useEffect, useState, useSyncExternalStore} from 'react';
import {friendlyHost} from '@core/presentation/index.ts';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {Notice, StatusScreen} from '../ui';
import {useReachability} from '../app/freshness';
import s from './Shell.module.css';

/**
 * The shell's failure mode for a server it can't use. The shell always stays open (rail,
 * Settings, Account, switching profile or server).
 *
 * - A server that was working stops answering (the route is offline or being found again, the
 *   event feed is down, quiet reads keep failing): the page stays exactly as it is. What is on
 *   screen was read while the server answered and is still the best there is; reads are tried
 *   again in the background. Nothing is said for the first while. After a sustained failure one
 *   small line appears above the page ("Can't reach …"), and it goes away by itself with the
 *   first thing that succeeds. The page is never covered, emptied or started afresh: it is
 *   brought up to date in place when the server is back.
 * - The remembered server couldn't be reached at launch (`serverProblem`): "Can't reach" with
 *   Try again while Portico keeps retrying. Nothing has loaded yet, so there is no page to keep.
 *   When something else answers at its address, the status says so and also offers Use another
 *   server; the saved sign-in is never dropped for it. A sign-in that has ended never shows
 *   here: the app goes straight to the sign-in screen (`signInHint`, app/session.tsx).
 * - An address that now proves another identity while the app is open: the page is covered
 *   (and kept mounted underneath, inert), because nothing on it should be acted on against a
 *   server that may not be the person's.
 */
export function ConnectionOverlay({children}: {children: React.ReactNode}) {
  const {api, serverProblem, retryRestore, disconnectServer, servers, system, serverUrl} = useSession();
  const i18n = useI18n();
  const route = useSyncExternalStore(api.subscribeRoute, api.getRouteSnapshot);
  // Only an address answering as someone else covers the page; a server that is merely away does not.
  const unavailable = !serverProblem && !!route && route.phase === 'identity_mismatch';
  const away = useReachability() === 'unreachable' && !serverProblem && !unavailable;
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
  const unreachable = !!serverProblem;
  // Something else answers at the saved address. The sign-in is kept and Portico keeps trying;
  // only the person can decide that their server really was set up again.
  const replaced = serverProblem?.kind === 'replaced';
  const status = unreachable || shown ? (
    <div className={unreachable ? s.connectionStatus : s.connectionCover}>
      <StatusScreen
        title={i18n.t(replaced ? 'web.connection.replacedTitle' : 'launch.reconnectingTitle')}
        body={i18n.t(replaced ? 'web.connection.replacedBody' : 'web.connection.reconnectingBody', {server: name})}
        action={{label: i18n.t('action.tryAgain'), onClick: serverProblem ? retryRestore : () => void api.recoverRoute().catch(() => {})}}
        secondaryAction={replaced ? {label: i18n.t('web.restore.useAnother'), onClick: disconnectServer} : undefined}
      />
    </div>
  ) : null;
  // A server that could not be reached at launch has no page to keep: nothing has loaded yet.
  if (unreachable) return status;
  return (
    <>
      {status}
      {away && !shown ? <div className={s.connectionAway} role="status"><Notice tone="info" compact>{i18n.t('web.connection.cantReach', {server: name})}</Notice></div> : null}
      <div className={shown ? s.connectionHeld : undefined} inert={shown}>{children}</div>
    </>
  );
}
