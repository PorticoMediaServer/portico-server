import {StrictMode, useLayoutEffect, useSyncExternalStore} from 'react';
import {createRoot} from 'react-dom/client';
import {RouterProvider, createMemoryHistory} from '@tanstack/react-router';
import type {HttpLocalApi, Library, Viewer} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import {router, rootRoute, searchRoute} from '../../router';
import {WebScreenProvider} from '../../route-registry-view';
import {ShellBindingsProvider} from '../../shell/bindings';
import {IconSprite} from '../../kit/Icon';
import {LastResortBoundary} from '../../shell/Errors';
import {DestinationErrorBoundary, type DestinationRecoveryWitness} from '../../shell/DestinationError';

/**
 * FX-156 specimen: the real router, real ShellRoot and real Shell chrome with finite fixture data.
 * The destination fault is installed through the actual route's public options update and the exact
 * original options are restored on teardown. No production session, network or credentials exist here.
 */
type Fault = 'none' | 'render' | 'effect';
let fault: Fault = 'none';
const faultListeners = new Set<() => void>();
const setFault = (value: Fault) => { fault = value; for (const listener of faultListeners) listener(); };
const subscribeFault = (listener: () => void) => { faultListeners.add(listener); return () => {faultListeners.delete(listener);}; };
const faultSnapshot = () => fault;
const original = Object.freeze({search: searchRoute.options.component, root: rootRoute.options.component, validateSearch: searchRoute.options.validateSearch, history: router.history});
/** The deterministic fixture destination is mounted from the start; only its fault state changes. */
function FixtureDestination() {
  const value = useSyncExternalStore(subscribeFault, faultSnapshot, faultSnapshot);
  useLayoutEffect(() => { if (value === 'effect') throw new Error('destination-effect-fault'); }, [value]);
  if (value === 'render') throw new Error('destination-render-fault');
  return <div data-destination-ok>{value}</div>;
}

/* Component-level witness case: the real outlet boundary with typed witnesses, separate from the
   production-shell proof above. It proves only the boundary's own identity rule. */
let witnessFault = true;
function WitnessThrower() { if (witnessFault) throw new Error('witness-fault'); return <div data-witness-ok>ok</div>; }
function mountWitness(element: HTMLElement, witness: DestinationRecoveryWitness) {
  const root = createRoot(element);
  const render = (value: DestinationRecoveryWitness) => root.render(<StrictMode><DestinationErrorBoundary witness={value}><WitnessThrower/></DestinationErrorBoundary></StrictMode>);
  render(witness);
  return {render, dispose: () => root.unmount()};
}

declare global {
  interface Window { destinationRecovery?: {
    fault: (value: Fault) => void; scope: (value: string) => void; entry: () => void; navigate: (value: string) => Promise<void>;
    validate: (value: boolean) => void; rootFault: (value: boolean) => void; shellMarker: () => boolean; restore: () => void;
    witness: () => void;
    moveHistory: (href: string) => void;
  };
  destinationRecoveryWitness?: {
    same: () => void; freshEntry: () => void; scope: () => void; missingKey: () => void; sameHrefNewObject: () => void;
    release: () => void; rearm: () => void; dispose: () => void;
  };
  disposeDestinationRecovery?: () => void; }
}

const root = typeof document === 'undefined' ? null : document.getElementById('root');
if (root) mount(root);

function mount(element: HTMLElement) {
  const viewer: Viewer = {serverId: 'fixture-server', authority: 'local', accountId: 'fixture-account', profileId: 'fixture-profile', role: 'owner'};
  const libraries: readonly Library[] = [{id: 'movies', name: 'Movies', kind: 'movie'}];
  const identity = {viewer, username: 'fixture', profileName: 'Fixture', serverName: 'Fixture server', primaryProfile: true,
    profiles: [], canManageProfiles: false, accountFacts: [], libraries, liveAvailable: false, unread: 0, showNotificationDot: false};
  const api = {
    getRouteSnapshot: () => ({phase: 'ready'}), subscribeRoute: () => () => {},
  } as unknown as HttpLocalApi;
  const events = {on: () => () => {}, onResync: () => () => {}, onAny: () => () => {}, getStatus: () => ({connected: true}), subscribeStatus: () => () => {}} as unknown as EventsClient;
  const bindingFor = (scopeKey: string) => ({api, events, scopeKey,
    shell: {identity, OwnerActivity: () => null, dockVisible: false, onSignOut: () => {}}, home: {} as never});
  let scopeKey = 'fixture-scope';
  const render = (value = scopeKey) => {
    scopeKey = value;
    const binding = bindingFor(value);
    reactRoot.render(<StrictMode><LastResortBoundary>
      <WebScreenProvider value={{binding} as never}>
        <IconSprite />
        <ShellBindingsProvider value={binding as never}><RouterProvider router={router} /></ShellBindingsProvider>
      </WebScreenProvider>
    </LastResortBoundary></StrictMode>);
  };
  const reactRoot = createRoot(element);
  const originalHistory = original.history;
  searchRoute.update({component: FixtureDestination});
  router.update({history: createMemoryHistory({initialEntries: ['/search']})});
  render();
  window.destinationRecovery = {
    fault: value => setFault(value),
    scope: value => render(value),
    entry: () => { router.history.push('/search'); },
    navigate: async value => {
      const [path, query = ''] = value.split('?');
      const search = Object.fromEntries(new URLSearchParams(query));
      await router.navigate({to: path, search} as never);
    },
    // Test-only instrumentation: validateSearch is not part of the supported updatable options.
    validate: value => { searchRoute.update({validateSearch: (value ? () => { throw new Error('validation-fault'); } : original.validateSearch)} as never); },
    rootFault: value => { rootRoute.update({component: (value ? () => { throw new Error('shell-fault'); } : original.root)} as never); },
    shellMarker: () => Boolean(document.querySelector('header')),
    restore: () => { setFault('none'); searchRoute.update({component: original.search}); searchRoute.update({validateSearch: original.validateSearch} as never); rootRoute.update({component: original.root}); router.update({history: originalHistory}); },
    moveHistory: href => { router.history.push(href); },
    witness: () => {
      const host = document.createElement('div'); document.body.append(host);
      const base = {location: {}, href: '/witness', entry: 'entry-1', scopeKey: 'scope-1'};
      const mount = mountWitness(host, base);
      witnessFault = true;
      const dispose = mount.dispose;
      window.destinationRecoveryWitness = {
        same: () => mount.render(base),
        freshEntry: () => mount.render({location: {}, href: '/witness', entry: 'entry-2', scopeKey: 'scope-1'}),
        scope: () => mount.render({location: {}, href: '/witness', entry: 'entry-2', scopeKey: 'scope-2'}),
        missingKey: () => mount.render({location: {}, href: '/witness?x=1', scopeKey: 'scope-2'}),
        sameHrefNewObject: () => mount.render({location: {}, href: '/witness', entry: 'entry-1', scopeKey: 'scope-1'}),
        release: () => { witnessFault = false; },
        rearm: () => { witnessFault = true; mount.render({location: {}, href: '/witness', entry: 'entry-1', scopeKey: 'scope-1'}); },
        dispose: () => {dispose(); host.remove(); delete window.destinationRecoveryWitness;},
      };
    },
  };
  (window as Window & {disposeDestinationRecovery?: () => void}).disposeDestinationRecovery = () => {
    window.destinationRecovery?.restore();
    reactRoot.unmount();
  };
}
