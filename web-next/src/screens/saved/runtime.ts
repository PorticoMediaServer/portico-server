import {SavedService, type SavedApi, type SavedRestore, type SavedRoute} from '@core/saved.ts';
import {PersonalSavedService, isPersonalSavedRoute} from '@core/personal-saved.ts';
import {DownloadsService} from '@core/downloads-service.ts';
import {randomId} from '@core/random-id.ts';
import type {SavedDownloadsScope} from './downloads-port.ts';
import {SavedOperations} from './operations.ts';

export type SavedScreenRoute = SavedRoute | Readonly<{view: 'downloads'}>;

/** One captured parent lifetime, using only the published server services. Native
 * files/rules/artwork are installed separately by Settings; this creates no store. */
export function createSavedRuntime(input: Readonly<{api: SavedApi; scope: SavedDownloadsScope; isCurrent: () => boolean; deviceId?: string}>) {
  if (!['local', 'hosted'].includes(input.scope.authority) || [input.scope.serverId, input.scope.accountId, input.scope.profileId, input.scope.sessionFamilyId].some(value => typeof value !== 'string' || !value || /[\u0000-\u001f\u007f]/.test(value))) throw new Error('Saved requires a complete captured viewer scope.');
  const scope = Object.freeze({...input.scope}), operations = new SavedOperations(input.isCurrent), controllers = new Set<AbortController>();
  const abort = () => new DOMException('Saved authority is no longer current.', 'AbortError');
  const api: SavedApi = {
    async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
      if (!operations.current || signal?.aborted) throw abort();
      const controller = new AbortController(), stop = () => controller.abort();
      signal?.addEventListener('abort', stop, {once: true}); controllers.add(controller);
      try {
        if (!operations.current || signal?.aborted) throw abort();
        const result = await input.api.request<T>(path, method, body, controller.signal);
        if (!operations.current || controller.signal.aborted) throw abort();
        return result;
      } finally { signal?.removeEventListener('abort', stop); controllers.delete(controller); }
    },
  };
  const serverScope = Object.freeze({serverId: scope.serverId, viewerId: JSON.stringify([scope.authority, scope.accountId, scope.profileId, scope.sessionFamilyId])});
  const makeSaved = () => new SavedService({api, scope: serverScope, requestId: async () => randomId()});
  const saved = makeSaved(), posters = makeSaved(), squares = makeSaved(), personal = new PersonalSavedService(api, serverScope, async () => randomId()), downloads = new DownloadsService(api);
  if (input.deviceId) downloads.followDevice(input.deviceId);
  return {
    scope, operations, saved, posters, squares, personal, downloads,
    async select(route: SavedScreenRoute, restore?: SavedRestore): Promise<void> {
      if (!operations.current || route.view === 'downloads') return;
      if (isPersonalSavedRoute(route)) { await personal.select(route, restore); return; }
      if (route.view === 'watchlist' || route.view === 'favorites') {
        await Promise.all([posters.select({...route, shape: 'posters'}, restore), squares.select({...route, shape: 'squares'}, restore)]);
      } else await saved.select(route, restore);
    },
    dispose(): void {
      operations.dispose(); for (const controller of controllers) controller.abort(); controllers.clear();
      saved.dispose(); posters.dispose(); squares.dispose(); personal.dispose(); downloads.dispose();
    },
  };
}
export type SavedRuntime = ReturnType<typeof createSavedRuntime>;
