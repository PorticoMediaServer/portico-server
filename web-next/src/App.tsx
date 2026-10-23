import {RouterProvider} from '@tanstack/react-router';
import {useEffect, useSyncExternalStore} from 'react';
import {IconSprite} from './kit/Icon';
import {bindWebAccountEntry} from './route-registry-lifecycle';
import {WebScreenProvider, webScreens} from './route-registry-view';
import type {ShellAssemblyContext} from './shell/bootstrap';
import {router} from './router';
import {ShellBindingsProvider, type ShellBindings} from './shell/bindings';
import {bindShellRouteMotion} from './shell/route-motion';
import {ShellBootstrapState} from './shell/BootstrapState';
import type {ShellBootstrap} from './shell/bootstrap';
import {PlaybackHost} from './shell/Playback';
import type {TakeAccountReturn} from './route-registry-return';
export function App({binding, bootstrap, takeAccountReturn}: {binding?: ShellBindings; bootstrap?: ShellBootstrap; takeAccountReturn?: TakeAccountReturn} = {}) {
  return bootstrap ? <RestoredApp bootstrap={bootstrap} takeAccountReturn={takeAccountReturn} /> : <BoundApp binding={binding} takeAccountReturn={takeAccountReturn} />;
}
function RestoredApp({bootstrap, takeAccountReturn}: {bootstrap: ShellBootstrap; takeAccountReturn?: TakeAccountReturn}) {
  const snapshot = useSyncExternalStore(bootstrap.subscribe, bootstrap.getSnapshot, bootstrap.getSnapshot);
  useEffect(() => bindWebAccountEntry(router, bootstrap), [bootstrap]);
  return <ShellBootstrapState value={{snapshot, retry: bootstrap.start}}><BoundApp binding={snapshot.binding} context={snapshot.context} bootstrap={bootstrap} takeAccountReturn={takeAccountReturn} /></ShellBootstrapState>;
}
function BoundApp({binding, context, bootstrap, takeAccountReturn}: {binding?: ShellBindings; context?: ShellAssemblyContext; bootstrap?: ShellBootstrap; takeAccountReturn?: TakeAccountReturn}) {
  useEffect(() => bindShellRouteMotion(router), []);
  const MediaHost = webScreens.get('player')?.playbackHost ?? PlaybackHost;
  return <WebScreenProvider value={{binding, context, bootstrap, takeAccountReturn}}><IconSprite /><ShellBindingsProvider value={binding}><RouterProvider router={router} /></ShellBindingsProvider><MediaHost binding={binding?.playback} /></WebScreenProvider>;
}
