import React, {Suspense, lazy} from 'react';
import {useLocation} from '@tanstack/react-router';
import {useSession} from '../../app/session';
import {DeviceGate} from '../../app/device';
import {RestoreScreen} from './Restore';
import {SignInScreen} from './SignIn';
import {ServerlessShell} from '../../shell/ServerlessShell';
import {ShellSkeleton} from '../../shell/ShellSkeleton';
import {lastRailCache} from '../../app/rail-cache';

// NEW-12 (M28 Q1): the account workspace renders its own light shell and
// needs no shell providers. Lazy, so the account screens stay out of the
// startup graph (PERF-25); the route-level split in app/router.tsx stays the
// one the product shell uses.
const AccountWorkspace = lazy(() => import('../account/Account').then(m => ({default: m.AccountWorkspace})));

/** Renders the authenticated tree only once a verified viewer exists. */
export function Gate({children}: {children: React.ReactNode}) {
  const session = useSession();
  const {pathname} = useLocation();
  // A stored session restores behind the shell's own shape, not a card: the rail as the viewer
  // last saw it and a page skeleton. The card stays for a reconnect that is taking a while
  // (it carries the retry and "use another server" actions) and for a browser with no history.
  if (session.phase === 'restoring') return !session.restoreMessage && lastRailCache() ? <ShellSkeleton /> : <RestoreScreen />;
  // NEW-12: `/account` lives under the app route, so a server session shows
  // it inside the product shell (below). With no server session Gate would
  // otherwise land it in ServerlessShell/SignInScreen, so it renders its own
  // light shell instead (which signs in right there when signed out).
  if (pathname === '/account' && session.phase === 'signedOut') {
    return <Suspense fallback={null}><AccountWorkspace /></Suspense>;
  }
  // INT gate 2: a Portico Account without an open server lands in the shell, never on a page of
  // its own. Only a direct sign-in that ended (hint `local`) opens the direct form, for that server.
  if (session.phase === 'signedOut' && session.hosted && session.signInHint?.authority !== 'local') return <ServerlessShell />;
  if (session.phase === 'signedOut') return <SignInScreen />;
  // A browser waiting for the owner's approval, or refused, is told so inside the shell's frame.
  return <DeviceGate blocked={access => <ServerlessShell device={access} />}>{children}</DeviceGate>;
}
