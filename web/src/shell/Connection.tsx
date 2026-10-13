import React, {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {useRouterState} from '@tanstack/react-router';
import {friendlyHost} from '@core/presentation/index.ts';
import {useSession} from '../app/session';
import {useI18n} from '../app/i18n';
import {StatusScreen} from '../ui';
import s from './Shell.module.css';

/**
 * The shell's failure mode for a server it can't use. The shell always stays open (rail,
 * Settings, Account, switching profile or server). A connection lost for more than a moment
 * fails the page as a whole: one centered "Reconnecting" status covers the content area
 * (never a card-by-card wall of errors), and the page comes back by itself when the route is
 * ready again.
 *
 * While a working server is away the page stays mounted underneath, hidden and inert, so what
 * the person had typed or changed is still there afterwards. A page nobody had typed into or
 * changed is started afresh when the server returns, so it reloads its data; a page with edits
 * is given back exactly as it was (see `pageAfterOutage`).
 *
 * - The remembered server couldn't be reached at launch (`serverProblem`): "Can't reach" with
 *   Try again while Portico keeps retrying. A sign-in that has ended never shows here: the app
 *   goes straight to the sign-in screen (`signInHint`, app/session.tsx).
 * - A server that was working drops off (route offline, or an address answering with another
 *   identity, or a proxy answering 502/503/504 for it): "Reconnecting", after a moment so a
 *   blip never flashes one.
 */
/**
 * What happens to the covered page when the server is back: `keep` it as it was when the person
 * had edited something on it (a settings form, a half-typed search), otherwise `reload` it so it
 * shows fresh data rather than whatever failed to load while the server was away.
 */
export function pageAfterOutage(page: {edited: boolean}): 'keep' | 'reload' {
  return page.edited ? 'keep' : 'reload';
}

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
  // Typing or changing a control anywhere on the page marks it edited until the address changes.
  const pathname = useRouterState({select: state => state.location.pathname});
  const edited = useRef(false);
  useEffect(() => { edited.current = false; }, [pathname]);
  const [fresh, setFresh] = useState(0);
  const covered = useRef(false);
  useEffect(() => {
    if (shown) { covered.current = true; return; }
    if (!covered.current) return;
    covered.current = false;
    if (pageAfterOutage({edited: edited.current}) === 'reload') setFresh(n => n + 1);
  }, [shown]);
  const name = system?.name ?? servers.find(x => x.id === serverProblem?.serverId)?.name ?? friendlyHost(serverProblem?.serverUrl ?? serverUrl) ?? i18n.t('web.settings.serverFallback');
  // A live route that proves a different identity may be an impostor at the address: that reads
  // as unreachable, never as an ended sign-in.
  const unreachable = serverProblem?.kind === 'unreachable';
  const status = unreachable || shown ? (
    <div className={unreachable ? s.connectionStatus : s.connectionCover}>
      <StatusScreen
        title={i18n.t('launch.reconnectingTitle')}
        body={i18n.t('web.connection.reconnectingBody', {server: name})}
        action={{label: i18n.t('action.tryAgain'), onClick: serverProblem ? retryRestore : () => void api.recoverRoute().catch(() => {})}}
      />
    </div>
  ) : null;
  // A server that could not be reached at launch has no page to keep: nothing has loaded yet.
  if (unreachable) return status;
  return (
    <>
      {status}
      <div className={shown ? s.connectionHeld : undefined} inert={shown} onInputCapture={() => { edited.current = true; }} onChangeCapture={() => { edited.current = true; }}>
        <React.Fragment key={fresh}>{children}</React.Fragment>
      </div>
    </>
  );
}
