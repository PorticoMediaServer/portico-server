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
import type {TakeAccountMove} from './route-registry-arrival';
export function App({binding, bootstrap, takeAccountReturn, takeAccountMove}: {binding?: ShellBindings; bootstrap?: ShellBootstrap; takeAccountReturn?: TakeAccountReturn; takeAccountMove?: TakeAccountMove} = {}) {
  return bootstrap ? <RestoredApp bootstrap={bootstrap} takeAccountReturn={takeAccountReturn} takeAccountMove={takeAccountMove} /> : <BoundApp binding={binding} takeAccountReturn={takeAccountReturn} takeAccountMove={takeAccountMove} />;
}
function RestoredApp({bootstrap, takeAccountReturn, takeAccountMove}: {bootstrap: ShellBootstrap; takeAccountReturn?: TakeAccountReturn; takeAccountMove?: TakeAccountMove}) {
  const snapshot = useSyncExternalStore(bootstrap.subscribe, bootstrap.getSnapshot, bootstrap.getSnapshot);
  useEffect(() => bindWebAccountEntry(router, bootstrap), [bootstrap]);
  return <ShellBootstrapState value={{snapshot, retry: bootstrap.start}}><BoundApp binding={snapshot.binding} context={snapshot.context} bootstrap={bootstrap} takeAccountReturn={takeAccountReturn} takeAccountMove={takeAccountMove} /></ShellBootstrapState>;
}
function BoundApp({binding, context, bootstrap, takeAccountReturn, takeAccountMove}: {binding?: ShellBindings; context?: ShellAssemblyContext; bootstrap?: ShellBootstrap; takeAccountReturn?: TakeAccountReturn; takeAccountMove?: TakeAccountMove}) {
  useEffect(() => bindShellRouteMotion(router), []);
  const MediaHost = webScreens.get('player')?.playbackHost ?? PlaybackHost;
  return <WebScreenProvider value={{binding, context, bootstrap, takeAccountReturn, takeAccountMove}}><IconSprite /><ShellBindingsProvider value={binding}><RouterProvider router={router} /></ShellBindingsProvider><MediaHost binding={binding?.playback} /></WebScreenProvider>;
}
