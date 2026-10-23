import {useEffect, useState, useSyncExternalStore} from 'react';
import {useMatch} from '@tanstack/react-router';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createRetainedSavedOwner, type RetainedSavedOwner} from './retained-owner';

const owners = new WeakMap<ShellAssemblyContext, RetainedSavedOwner>(), listeners = new Set<() => void>();
const changed = () => {for (const listener of [...listeners]) listener();};
const subscribe = (listener: () => void) => {listeners.add(listener);return () => {listeners.delete(listener);};};
export function useRetainedSavedOwner(context: ShellAssemblyContext): RetainedSavedOwner | undefined {
  return useSyncExternalStore(subscribe, () => context.isCurrent() ? owners.get(context) : undefined, () => undefined);
}
function CapturedSavedOwner({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured);
  useEffect(() => {
    const session = captured.context.session(), viewer = session.viewer;
    if (viewer.authority !== 'local' && viewer.authority !== 'hosted' || !context.isCurrent()) return;
    const owner = createRetainedSavedOwner({api: captured.context.api, scope: {serverId: viewer.serverId, authority: viewer.authority,
      accountId: viewer.accountId, profileId: viewer.profileId, sessionFamilyId: session.sessionFamilyId}, deviceId: session.deviceId,
      isCurrent: context.isCurrent, reportError: context.effects.reportError});
    if (owners.has(captured.context)) {owner.dispose();throw new Error('saved_owner_already_registered');}
    owners.set(captured.context, owner);changed();owner.start();
    return () => {if (owners.get(captured.context) === owner) owners.delete(captured.context);owner.dispose();changed();};
  }, [captured.context, context]);
  return null;
}
/** The registry already retains this protected host across route changes.
 * Its first real Saved visit activates it; no import or unrelated route does I/O. */
export function SavedHost() {
  const captured = useProtectedWebScreenContext(), match = useMatch({from: '/saved', shouldThrow: false});
  const [visited, setVisited] = useState(false);
  useEffect(() => {if (match) setVisited(true);}, [match]);
  return captured && visited ? <CapturedSavedOwner captured={captured}/> : null;
}
