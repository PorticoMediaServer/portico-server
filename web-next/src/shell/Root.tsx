import {useLayoutEffect, useMemo, useRef} from 'react';
import {Outlet, useRouterState} from '@tanstack/react-router';
import {FreshnessProvider} from '../platform/freshness';
import {useRouteFade} from '../platform/route-motion';
import {Shell} from './Shell';
import {useShellBindings} from './bindings';
import {finishRouteMotion, routeMotionOwnedByBrowser} from './route-motion';
import {BootstrapContent} from './BootstrapState';
import {DestinationErrorBoundary} from './DestinationError';

function RouteContent() {
  const content = useRef<HTMLDivElement>(null);
  const routeKey = useRouterState({select: state => state.location.href});
  const target = useMemo(() => ({get current() { return routeMotionOwnedByBrowser() ? null : content.current; }}), []);
  useRouteFade(target, routeKey);
  useLayoutEffect(() => { finishRouteMotion(); }, [routeKey]);
  // The destination boundary wraps only the outlet, so the shell (top bar, rail, phone nav) stays
  // mounted through a destination failure and a ShellRoot failure still reaches the app last resort.
  return <main id="portico-content" ref={content} data-route-content tabIndex={-1}><BootstrapContent><DestinationErrorBoundary routeKey={routeKey}><Outlet /></DestinationErrorBoundary></BootstrapContent></main>;
}
export function ShellRoot() {
  const binding = useShellBindings();
  if (!binding) return <RouteContent />;
  const viewer = binding.shell.identity.viewer;
  return <FreshnessProvider key={binding.scopeKey} api={binding.api} events={binding.events} serverId={viewer.serverId} viewerId={JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}>
    <Shell {...binding.shell}><RouteContent /></Shell>
  </FreshnessProvider>;
}
