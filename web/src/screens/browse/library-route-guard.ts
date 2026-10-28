import {useLayoutEffect, useMemo} from 'react';
import {parseMediaItem} from '@core/media-item.ts';
import type {ShellAssemblyContext} from '../../shell/bootstrap';
import {forwardFacts} from '../../shell/fact-source';
import {createLibraryRouteGuard, type LibraryRouteGuard} from '../../shell/library-route-proof';
import {encodedRoutePath} from '../../../../packages/contracts/src/routes.ts';
import {router} from '../../router';

/** The original committed route/receiver is the key. Display publication is not. */
export function useLibraryRouteGuard(context: ShellAssemblyContext | undefined, opening: object,
  routeCurrent: () => boolean, target: Readonly<{libraryId?: string; itemId?: string}>, reportError: (error: unknown) => void,
  canReplace: () => boolean = routeCurrent): LibraryRouteGuard | undefined {
  const {libraryId, itemId} = target;
  const guard = useMemo(() => {
    if (!context?.libraryRouteProof || context.libraryRouteSignal) return;
    const original = context.session(), family = original.sessionFamilyId, device = original.deviceId, viewer = Object.freeze({...original.viewer});
    const authority = () => {
      if (!context.isCurrent()) return false;
      const now = context.session();
      return now.sessionFamilyId === family && now.deviceId === device
        && (['serverId', 'authority', 'accountId', 'profileId', 'role'] as const).every(key => now.viewer[key] === viewer[key]) && context.isCurrent();
    };
    return createLibraryRouteGuard({source: context.libraryRouteProof, opening, libraryId,
      isCurrent: () => authority() && routeCurrent(), canReplace: () => authority() && canReplace(),
      ...(itemId ? {readParent: async (signal: AbortSignal) => {
        signal.throwIfAborted();if (!authority() || !routeCurrent()) throw new DOMException('Item route retired', 'AbortError');
        const raw = await context.api.request(encodedRoutePath('get_items_id', {id: encodeURIComponent(itemId)}), 'GET', undefined, signal);
        signal.throwIfAborted();if (!authority() || !routeCurrent()) throw new DOMException('Item route retired', 'AbortError');
        const item = parseMediaItem(raw);
        if (item.id !== itemId || !item.libraryId) throw new Error('Invalid route parent document');
        return item.libraryId;
      }} : {}),
      replace: destination => router.navigate(destination), reportError,
    });
    // The opening owns these original route/error callbacks; later render closures
    // must not replace its authority or reconstruct its readers on a rename.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [context, opening, libraryId, itemId]);
  useLayoutEffect(() => {guard?.start();return () => guard?.dispose();}, [guard]);
  return guard;
}

export function useGuardedLibraryContext(context: ShellAssemblyContext | undefined, guard: LibraryRouteGuard | undefined) {
  return useMemo(() => context && guard ? forwardFacts(context, {isCurrent: guard.isCurrent, libraryRouteSignal: guard.signal}) : context, [context, guard]);
}
