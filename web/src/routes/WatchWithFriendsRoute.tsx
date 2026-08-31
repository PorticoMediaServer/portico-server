import { StatusWarningIcon } from '#portico-icons';
import { useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useAuthSession, usePorticoDataSource, useViewerRuntime } from '../data/DataProvider';
import { HttpPorticoDataSource } from '../data/httpSource';
import type { Viewer } from '../data/models';
import { canManageServer } from '../data/authority';
import { HttpWatchWithFriendsSource, WatchWithFriendsPage, type WatchWithFriendsSource, type WatchWithFriendsViewer } from '../features/watch-with-friends';

function scopedWatchSource(source: WatchWithFriendsSource, runtime: ReturnType<typeof useViewerRuntime>, auth: ReturnType<typeof useAuthSession>): WatchWithFriendsSource {
  const run = <T,>(name: string, args: unknown[], operation: (signal: AbortSignal) => Promise<T>) => runtime.run(`watch-with-friends.${name}`, args, operation);
  return {
    listGroups: () => run('listGroups', [], (signal) => source.listGroups(signal)),
    group: (id) => run('group', [id], (signal) => source.group(id, signal)),
    createGroup: (request) => run('createGroup', [request], (signal) => source.createGroup(request, signal)),
    joinGroup: (id) => run('joinGroup', [id], (signal) => source.joinGroup(id, signal)),
    leaveGroup: (id) => run('leaveGroup', [id], (signal) => source.leaveGroup(id, signal)),
    endGroup: (id, expectedRevision, idempotencyKey) => run('endGroup', [id, expectedRevision, idempotencyKey], (signal) => source.endGroup(id, expectedRevision, idempotencyKey, signal)),
    updateMemberState: (id, request) => run('updateMemberState', [id, request], (signal) => source.updateMemberState(id, request, signal)),
    updatePlaybackState: (id, request) => run('updatePlaybackState', [id, request], (signal) => source.updatePlaybackState(id, request, signal)),
    updateSettings: (id, request) => run('updateSettings', [id, request], (signal) => source.updateSettings(id, request, signal)),
    addQueueItem: (id, request) => run('addQueueItem', [id, request], (signal) => source.addQueueItem(id, request, signal)),
    reorderQueue: (id, request) => run('reorderQueue', [id, request], (signal) => source.reorderQueue(id, request, signal)),
    removeQueueItem: (id, entryId, expectedRevision, idempotencyKey) => run('removeQueueItem', [id, entryId, expectedRevision, idempotencyKey], (signal) => source.removeQueueItem(id, entryId, expectedRevision, idempotencyKey, signal)),
    subscribe: (id, subscription) => {
      const close = source.subscribe(id, subscription);
      const unregister = auth.registerRuntimeTeardown('realtime', close);
      return () => {
        unregister();
        close();
      };
    },
  };
}

function providesDevelopmentWatchWithFriends(value: unknown): value is { watchWithFriendsSource: (viewer: WatchWithFriendsViewer) => WatchWithFriendsSource } {
  if (!import.meta.env.DEV || !value || typeof value !== 'object') return false;
  return typeof (value as { watchWithFriendsSource?: unknown }).watchWithFriendsSource === 'function';
}

export function WatchWithFriendsRoute({ viewer }: { viewer: Viewer }) {
  const dataSource = usePorticoDataSource();
  const runtime = useViewerRuntime();
  const auth = useAuthSession();
  const [parameters] = useSearchParams();
  const user = viewer.user;
  const watchViewer = useMemo<WatchWithFriendsViewer | undefined>(() => user ? {
    profileId: viewer.viewerScope?.profileId ?? '',
    displayName: user.displayName,
    canUse: user.permissions?.watchWithFriends === true || canManageServer(user),
    canManageServer: canManageServer(user),
  } : undefined, [user, viewer.viewerScope?.profileId]);
  const watchSource = useMemo<WatchWithFriendsSource | undefined>(() => {
    if (!watchViewer) return undefined;
    if (dataSource instanceof HttpPorticoDataSource) return scopedWatchSource(new HttpWatchWithFriendsSource(dataSource.porticoClient()), runtime, auth);
    if (providesDevelopmentWatchWithFriends(dataSource)) return scopedWatchSource(dataSource.watchWithFriendsSource(watchViewer), runtime, auth);
    return undefined;
  }, [auth.registerRuntimeTeardown, dataSource, runtime, watchViewer]);

  if (!watchViewer || !watchSource) {
    return <div className="standard-page"><div className="library-state error"><StatusWarningIcon /><strong>Watch With Friends isn’t available</strong><p>Reconnect to a compatible Portico server and try again.</p></div></div>;
  }
  return <WatchWithFriendsPage
    source={watchSource}
    viewer={watchViewer}
    initialGroupId={parameters.get('group') ?? ''}
    initialMediaId={parameters.get('media') ?? ''}
  />;
}
