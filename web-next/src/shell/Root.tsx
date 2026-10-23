import {useLayoutEffect, useMemo, useRef} from 'react';
import {Outlet, useRouterState} from '@tanstack/react-router';
import {FreshnessProvider} from '../platform/freshness';
import {useRouteFade} from '../platform/route-motion';
import {Shell} from './Shell';
import {useShellBindings} from './bindings';
import {finishRouteMotion, routeMotionOwnedByBrowser} from './route-motion';
import {BootstrapContent} from './BootstrapState';

function RouteContent() {
  const content = useRef<HTMLDivElement>(null);
  const routeKey = useRouterState({select: state => state.location.href});
  const target = useMemo(() => ({get current() { return routeMotionOwnedByBrowser() ? null : content.current; }}), []);
  useRouteFade(target, routeKey);
  useLayoutEffect(() => { finishRouteMotion(); }, [routeKey]);
  return <main id="portico-content" ref={content} data-route-content tabIndex={-1}><BootstrapContent><Outlet /></BootstrapContent></main>;
}
export function ShellRoot() {
  const binding = useShellBindings();
  const accountEntry = useRouterState({select: state => ['/sign-in', '/profiles', '/servers'].includes(state.location.pathname)});
  if (!binding || accountEntry) return <RouteContent />;
  const viewer = binding.shell.identity.viewer;
  return <FreshnessProvider key={binding.scopeKey} api={binding.api} events={binding.events} serverId={viewer.serverId} viewerId={JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}>
    <Shell {...binding.shell}><RouteContent /></Shell>
  </FreshnessProvider>;
}
