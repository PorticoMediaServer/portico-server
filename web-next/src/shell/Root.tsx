import {useLayoutEffect, useMemo, useRef} from 'react';
import {Outlet, useRouter, useRouterState} from '@tanstack/react-router';
import {useRouteFade, type RouteFadeCommits} from '../platform/route-motion';
import {Shell} from './Shell';
import {GlobalShortcuts} from './GlobalShortcuts';
import {useShellBindings} from './bindings';
import {finishRouteMotion, routeMotionOwnedByBrowser} from './route-motion';
import {webScreens, WebRetainedScreenHosts, useWebAction} from '../route-registry-view';
import {webScreenIdForRoute} from '../route-registry';
import {BootstrapContent} from './BootstrapState';
import {DestinationErrorBoundary, type DestinationRecoveryWitness} from './DestinationError';
import {useShellWidth} from './viewport';
import {useServerAway} from './ConnectionAway';

function RouteContent() {
  const content = useRef<HTMLDivElement>(null);
  const router = useRouter();
  const owner = useShellBindings();
  const routeKey = useRouterState({select: state => {
    const entry = state.location.state.__TSR_key;
    return typeof entry === 'string' && entry.length ? JSON.stringify([state.location.href, entry]) : undefined;
  }});
  const commits = useMemo<RouteFadeCommits>(() => ({subscribe: listener => router.subscribe('onRendered', event => {
    const entry = event.toLocation.state.__TSR_key;
    if (typeof entry === 'string' && entry.length) listener(JSON.stringify([event.toLocation.href, entry]));
  })}), [router, owner]);
  // The recovery witness is selected from the actual location object, independently of the
  // optional motion route key above: a missing history entry must not collapse every href, and a
  // new location object with the same href is still a new recovery lifetime.
  const location = useRouterState({select: state => state.location});
  const recovery = useMemo<DestinationRecoveryWitness>(() => {
    const entry = location.state.__TSR_key;
    return Object.freeze({location, href: location.href, entry: typeof entry === 'string' && entry.length ? entry : undefined, scopeKey: owner?.scopeKey});
  }, [location, owner?.scopeKey]);
  const target = useMemo(() => ({get current() { return routeMotionOwnedByBrowser() ? null : content.current; }}), []);
  useRouteFade(target, routeKey, commits);
  useLayoutEffect(() => { finishRouteMotion(); }, [routeKey]);
  // The destination boundary wraps only the outlet: the top bar, rail and phone navigation stay
  // mounted through a destination failure, while a ShellRoot failure stays a router/app-level failure
  // rather than becoming a destination recovery.
  // Its recovery lifetime is the witness above (actual location, href, entry, owner scope).
  return <main id="portico-content" ref={content} data-route-content data-shell-page tabIndex={-1}><BootstrapContent><DestinationErrorBoundary witness={recovery}><Outlet /></DestinationErrorBoundary></BootstrapContent></main>;
}
export function ShellRoot() {
  const binding = useShellBindings();
  const onFeedback = useWebAction('feedback');
  const routeId = useRouterState({select: state => state.matches.at(-1)?.routeId ?? ''});
  const screenId = webScreenIdForRoute(routeId);
  const viewer = binding?.shell.identity.viewer, phone = useShellWidth() < 600;
  const serverAway = useServerAway(binding?.api);
  const content = !binding || !viewer || screenId && (webScreens.chrome(screenId) !== 'shell' || phone && webScreens.phoneChrome(screenId) === 'standalone') ? <RouteContent />
    : <Shell {...binding.shell} serverAway={serverAway} onFeedback={onFeedback}><RouteContent /></Shell>;
  return <>{content}{binding?.playback && <GlobalShortcuts key={binding.scopeKey} playback={binding.playback}/>}<WebRetainedScreenHosts /></>;
}
