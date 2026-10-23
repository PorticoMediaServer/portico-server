import {RouterProvider} from '@tanstack/react-router';
import {useEffect, useSyncExternalStore} from 'react';
import {IconSprite} from './kit/Icon';
import {WebScreenProvider, webScreens} from './route-registry-view';
import type {ShellAssemblyContext} from './shell/bootstrap';
import {router} from './router';
import {ShellBindingsProvider, type ShellBindings} from './shell/bindings';
import {bindShellRouteMotion} from './shell/route-motion';
import {ShellBootstrapState} from './shell/BootstrapState';
import type {ShellBootstrap} from './shell/bootstrap';
import {PlaybackHost} from './shell/Playback';
export function App({binding, bootstrap}: {binding?: ShellBindings; bootstrap?: ShellBootstrap} = {}) {
  return bootstrap ? <RestoredApp bootstrap={bootstrap} /> : <BoundApp binding={binding} />;
}
function RestoredApp({bootstrap}: {bootstrap: ShellBootstrap}) {
  const snapshot = useSyncExternalStore(bootstrap.subscribe, bootstrap.getSnapshot, bootstrap.getSnapshot);
  useEffect(() => { void bootstrap.start(); return () => bootstrap.dispose(); }, [bootstrap]);
  return <ShellBootstrapState value={{snapshot, retry: bootstrap.start}}><BoundApp binding={snapshot.binding} context={snapshot.context} bootstrap={bootstrap} /></ShellBootstrapState>;
}
function BoundApp({binding, context, bootstrap}: {binding?: ShellBindings; context?: ShellAssemblyContext; bootstrap?: ShellBootstrap}) {
  useEffect(() => bindShellRouteMotion(router), []);
  const MediaHost = webScreens.get('player')?.playbackHost ?? PlaybackHost;
  return <WebScreenProvider value={{binding, context, bootstrap}}><IconSprite /><ShellBindingsProvider value={binding}><RouterProvider router={router} /></ShellBindingsProvider><MediaHost binding={binding?.playback} /></WebScreenProvider>;
}
