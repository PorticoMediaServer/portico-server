import {useLayoutEffect, useMemo, useRef} from 'react';
import {Outlet, useRouterState} from '@tanstack/react-router';
import {FreshnessProvider} from '../platform/freshness';
import {useRouteFade} from '../platform/route-motion';
import {Shell} from './Shell';
import {useShellBindings} from './bindings';
import {finishRouteMotion, routeMotionOwnedByBrowser} from './route-motion';
import {webScreens, WebRetainedScreenHosts} from '../route-registry-view';
import {webScreenIdForRoute} from '../route-registry';
import {BootstrapContent} from './BootstrapState';

function RouteContent() {
  const content = useRef<HTMLDivElement>(null);
  const routeKey = useRouterState({select: state => state.location.href});
  const target = useMemo(() => ({get current() { return routeMotionOwnedByBrowser() ? null : content.current; }}), []);
  useRouteFade(target, routeKey);
  useLayoutEffect(() => { finishRouteMotion(); }, [routeKey]);
  return <main id="portico-content" ref={content} data-route-content data-shell-page tabIndex={-1}><BootstrapContent><Outlet /></BootstrapContent></main>;
}
export function ShellRoot() {
  const binding = useShellBindings();
  const routeId = useRouterState({select: state => state.matches.at(-1)?.routeId ?? ''});
  const screenId = webScreenIdForRoute(routeId);
  const viewer = binding?.shell.identity.viewer;
  const content = !binding || !viewer || screenId && webScreens.chrome(screenId) !== 'shell' ? <RouteContent />
    : <FreshnessProvider key={binding.scopeKey} api={binding.api} events={binding.events} serverId={viewer.serverId} viewerId={JSON.stringify([viewer.authority, viewer.accountId, viewer.profileId])}>
      <Shell {...binding.shell}><RouteContent /></Shell>
    </FreshnessProvider>;
  return <>{content}<WebRetainedScreenHosts /></>;
}
