import {useLayoutEffect} from 'react';
import {createRoot} from 'react-dom/client';
import {RouterProvider, createMemoryHistory} from '@tanstack/react-router';
import type {HttpLocalApi, Library, Viewer} from '@core/index.ts';
import type {EventsClient} from '@core/playback-v1/events.ts';
import {router, rootRoute, searchRoute} from '../../router';
import {WebScreenProvider} from '../../route-registry-view';
import {ShellBindingsProvider} from '../../shell/bindings';
import {IconSprite} from '../../kit/Icon';

/**
 * FX-156 specimen: the real router, real ShellRoot and real Shell chrome with finite fixture data.
 * The destination fault is installed through the actual route's public options update and the exact
 * original options are restored on teardown. No production session, network or credentials exist here.
 */
type Fault = 'none' | 'render' | 'effect';
let fault: Fault = 'none';
const original = Object.freeze({search: searchRoute.options.component, root: rootRoute.options.component, validateSearch: searchRoute.options.validateSearch});
function FaultingDestination() {
  useLayoutEffect(() => { if (fault === 'effect') throw new Error('destination-effect-fault'); });
  if (fault === 'render') throw new Error('destination-render-fault');
  return <div data-destination-ok>ok</div>;
}
function installFault(value: Fault) {
  fault = value;
  searchRoute.update({component: (value === 'none' ? original.search : FaultingDestination)} as never);
}

declare global {
  interface Window { destinationRecovery?: {
    fault: (value: Fault) => void; scope: (value: string) => void; entry: () => void; navigate: (value: string) => Promise<void>;
    validate: (value: boolean) => void; rootFault: (value: boolean) => void; shellMarker: () => boolean; restore: () => void;
  }; disposeDestinationRecovery?: () => void; }
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
    reactRoot.render(<WebScreenProvider value={{binding} as never}>
      <IconSprite />
      <ShellBindingsProvider value={binding as never}><RouterProvider router={router} /></ShellBindingsProvider>
    </WebScreenProvider>);
  };
  const reactRoot = createRoot(element);
  router.update({history: createMemoryHistory({initialEntries: ['/search']})});
  render();
  window.destinationRecovery = {
    fault: value => installFault(value),
    scope: value => render(value),
    entry: () => { router.history.push('/search'); },
    navigate: async value => {
      const [path, query = ''] = value.split('?');
      const search = Object.fromEntries(new URLSearchParams(query));
      await router.navigate({to: path, search} as never);
    },
    validate: value => { searchRoute.update({validateSearch: (value ? () => { throw new Error('validation-fault'); } : original.validateSearch)} as never); },
    rootFault: value => { rootRoute.update({component: (value ? () => { throw new Error('shell-fault'); } : original.root)} as never); },
    shellMarker: () => Boolean(document.querySelector('header')),
    restore: () => { installFault('none'); searchRoute.update({validateSearch: original.validateSearch} as never); rootRoute.update({component: original.root} as never); },
  };
  (window as Window & {disposeDestinationRecovery?: () => void}).disposeDestinationRecovery = () => {
    window.destinationRecovery.restore();
    reactRoot.unmount();
  };
}
