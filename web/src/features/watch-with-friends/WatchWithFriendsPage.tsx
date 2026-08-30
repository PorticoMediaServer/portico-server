import type { WatchWithFriendsCreateRequest, WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import { StatusWarningIcon, StatusLoadingIcon, ActionAddIcon, ActionRefreshIcon, AccountWatchTogetherIcon, ActionCloseIcon } from '#portico-icons';
import { useCallback, useEffect, useState } from 'react';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { secureRandomUUID } from '../../runtime/secureRandomUUID';
import { CreateGroupForm, GroupDirectory } from './GroupDirectory';
import { GroupWorkspace, type GroupWorkspaceActions } from './GroupWorkspace';
import {
  groupIncludesViewer,
  type WatchConnectionState,
  type WatchWithFriendsSource,
  type WatchWithFriendsViewer,
} from './watchWithFriendsSource';
import { useWatchWithFriendsPlaybackSync } from './useWatchWithFriendsPlaybackSync';
import './watch-with-friends.css';

function sortGroups(groups: WatchWithFriendsGroup[]) {
  return [...groups].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

export interface WatchWithFriendsPageProps {
  source: WatchWithFriendsSource;
  viewer: WatchWithFriendsViewer;
  initialGroupId?: string;
  initialMediaId?: string;
}

export function WatchWithFriendsPage({ source, viewer, initialGroupId = '', initialMediaId = '' }: WatchWithFriendsPageProps) {
  const [groups, setGroups] = useState<WatchWithFriendsGroup[]>([]);
  const [selectedId, setSelectedId] = useState(initialGroupId);
  const [loading, setLoading] = useState(viewer.canUse);
  const [loadError, setLoadError] = useState('');
  const [operationError, setOperationError] = useState('');
  const [busy, setBusy] = useState('');
  const [creating, setCreating] = useState(Boolean(initialMediaId));
  const [connection, setConnection] = useState<WatchConnectionState | 'idle'>('idle');
  const [connectionError, setConnectionError] = useState('');
  const [eventRevision, setEventRevision] = useState(0);
  const selectedGroup = groups.find((group) => group.id === selectedId);
  const joined = selectedGroup ? groupIncludesViewer(selectedGroup, viewer) : false;
  const { applyGroupUpdate } = useWatchWithFriendsPlaybackSync({ group: joined ? selectedGroup : undefined, source, viewer });

  const chooseGroup = useCallback((items: WatchWithFriendsGroup[], currentId: string) => {
    if (currentId && items.some((group) => group.id === currentId)) return currentId;
    if (initialGroupId && items.some((group) => group.id === initialGroupId)) return initialGroupId;
    return items.find((group) => groupIncludesViewer(group, viewer))?.id ?? items[0]?.id ?? '';
  }, [initialGroupId, viewer]);

  const replaceGroups = useCallback((items: WatchWithFriendsGroup[]) => {
    const sorted = sortGroups(items);
    setGroups(sorted);
    setSelectedId((current) => chooseGroup(sorted, current));
  }, [chooseGroup]);

  const refreshGroups = useCallback(async (signal?: AbortSignal) => {
    const items = await source.listGroups(signal);
    replaceGroups(items);
  }, [replaceGroups, source]);

  useEffect(() => {
    if (!viewer.canUse) return;
    const controller = new AbortController();
    setLoading(true);
    setLoadError('');
    refreshGroups(controller.signal).catch((reason) => {
      if (!controller.signal.aborted) setLoadError(reviewedProductErrorText(reason, 'watch-with-friends.load-failed'));
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [refreshGroups, viewer.canUse]);

  const applyGroup = useCallback((group: WatchWithFriendsGroup) => {
    applyGroupUpdate(group);
    if (group.state === 'stopped') {
      setGroups((current) => current.filter((item) => item.id !== group.id));
      return;
    }
    setGroups((current) => sortGroups(current.some((item) => item.id === group.id)
      ? current.map((item) => item.id === group.id ? group : item)
      : [group, ...current]));
  }, [applyGroupUpdate]);

  useEffect(() => {
    if (!selectedId || groups.some((group) => group.id === selectedId)) return;
    setSelectedId(chooseGroup(groups, ''));
  }, [chooseGroup, groups, selectedId]);

  useEffect(() => {
    if (!selectedGroup || !joined) {
      setConnection('idle');
      setConnectionError('');
      return;
    }
    setConnection('connecting');
    setConnectionError('');
    return source.subscribe(selectedGroup.id, {
      onGroup: applyGroup,
      onStatus: (status) => {
        setConnection(status);
        if (status === 'live') setConnectionError('');
      },
      onError: (error) => setConnectionError(reviewedProductErrorText(error, 'watch-with-friends.connection-failed')),
    });
  }, [applyGroup, eventRevision, joined, selectedGroup?.id, source]);

  const runGroupMutation = async (key: string, operation: () => Promise<WatchWithFriendsGroup>) => {
    setBusy(key);
    setOperationError('');
    try {
      const group = await operation();
      applyGroup(group);
      setSelectedId(group.id);
      return group;
    } catch (reason) {
      setOperationError(reviewedProductErrorText(reason, 'watch-with-friends.action-failed', { actionName: 'update this group' }));
      return undefined;
    } finally {
      setBusy('');
    }
  };

  const createGroup = async (request: WatchWithFriendsCreateRequest) => {
    const group = await runGroupMutation('create', () => source.createGroup(request));
    if (group) setCreating(false);
  };

  const joinGroup = async (group: WatchWithFriendsGroup) => {
    setSelectedId(group.id);
    return Boolean(await runGroupMutation(`join:${group.id}`, () => source.joinGroup(group.id)));
  };

  const activeActions: GroupWorkspaceActions | undefined = selectedGroup ? {
    join: () => joinGroup(selectedGroup),
    leave: async () => {
      setBusy('leave');
      setOperationError('');
      try {
        await source.leaveGroup(selectedGroup.id);
        await refreshGroups();
        return true;
      } catch (reason) {
        setOperationError(reviewedProductErrorText(reason, 'watch-with-friends.action-failed', { actionName: 'leave this group' }));
        return false;
      } finally {
        setBusy('');
      }
    },
    end: async () => {
      setBusy('end');
      setOperationError('');
      try {
        await source.endGroup(selectedGroup.id, selectedGroup.revision, secureRandomUUID());
        setGroups((current) => current.filter((group) => group.id !== selectedGroup.id));
        setSelectedId('');
        await refreshGroups();
        return true;
      } catch (reason) {
        setOperationError(reviewedProductErrorText(reason, 'watch-with-friends.action-failed', { actionName: 'end this group' }));
        return false;
      } finally {
        setBusy('');
      }
    },
    retryEvents: () => {
      setConnectionError('');
      setEventRevision((current) => current + 1);
    },
    refresh: async () => {
      return Boolean(await runGroupMutation('refresh', () => source.group(selectedGroup.id)));
    },
    updateMember: async (request) => {
      return Boolean(await runGroupMutation('member', () => source.updateMemberState(selectedGroup.id, request)));
    },
    updatePlayback: async (request) => {
      return Boolean(await runGroupMutation('playback', () => source.updatePlaybackState(selectedGroup.id, {
        ...request,
        expectedRevision: selectedGroup.revision,
        idempotencyKey: secureRandomUUID(),
      })));
    },
    updateSettings: async (request) => {
      return Boolean(await runGroupMutation('settings', () => source.updateSettings(selectedGroup.id, { ...request, expectedRevision: selectedGroup.revision, idempotencyKey: secureRandomUUID() })));
    },
    addQueueItem: async (mediaId) => {
      return Boolean(await runGroupMutation('queue:add', () => source.addQueueItem(selectedGroup.id, { mediaId, expectedRevision: selectedGroup.revision, idempotencyKey: secureRandomUUID() })));
    },
    reorderQueue: async (mediaIds) => {
      return Boolean(await runGroupMutation('queue:order', () => source.reorderQueue(selectedGroup.id, { mediaIds, expectedRevision: selectedGroup.revision, idempotencyKey: secureRandomUUID() })));
    },
    removeQueueItem: async (mediaId) => {
      return Boolean(await runGroupMutation(`queue:remove:${mediaId}`, () => source.removeQueueItem(selectedGroup.id, mediaId, selectedGroup.revision, secureRandomUUID())));
    },
  } : undefined;

  if (!viewer.canUse) return <div className="standard-page watch-page"><section className="watch-access-state" role="alert"><StatusWarningIcon /><h1>Watch With Friends is unavailable</h1><p>This account does not have permission to create or join synchronized playback groups.</p></section></div>;

  return <div className="standard-page watch-page">
    <header className="page-header watch-page-header"><div><h1>Watch With Friends</h1><p>Synchronized playback, readiness, and a shared queue.</p></div><PrimaryButton onClick={() => setCreating((current) => !current)}>{creating ? <ActionCloseIcon /> : <ActionAddIcon />} {creating ? 'Close' : 'Create group'}</PrimaryButton></header>
    {creating && <CreateGroupForm initialMediaId={initialMediaId} busy={busy === 'create'} onCancel={() => setCreating(false)} onCreate={createGroup} />}
    {loadError && <div className="watch-load-error" role="alert"><StatusWarningIcon /><span><strong>Groups could not be loaded</strong><span>{loadError}</span></span><SecondaryButton onClick={() => { setLoading(true); setLoadError(''); refreshGroups().catch((reason) => setLoadError(reviewedProductErrorText(reason, 'watch-with-friends.load-failed'))).finally(() => setLoading(false)); }}><ActionRefreshIcon /> Try again</SecondaryButton></div>}
    {operationError && <div className="watch-alert watch-operation-error" role="alert"><StatusWarningIcon /><span>{operationError}</span></div>}
    {loading
      ? <section className="watch-loading" aria-busy="true"><StatusLoadingIcon className="watch-spin" /><strong>Loading groups</strong></section>
      : !loadError && <div className="watch-layout">
        <GroupDirectory groups={groups} selectedId={selectedId} viewer={viewer} busy={busy} onSelect={(group) => { setSelectedId(group.id); setOperationError(''); }} onJoin={joinGroup} onRefresh={() => { void refreshGroups().catch((reason) => setLoadError(reviewedProductErrorText(reason, 'watch-with-friends.load-failed'))); }} />
        <main className="watch-workspace">
          {selectedGroup && activeActions
            ? <GroupWorkspace group={selectedGroup} viewer={viewer} connection={connection} connectionError={connectionError} busy={busy} actions={activeActions} />
            : <section className="watch-no-selection"><AccountWatchTogetherIcon /><h2>{groups.length ? 'Choose a group' : 'No group selected'}</h2><p>{groups.length ? 'Open an active group to see playback and readiness.' : 'Create a group from a media item when you are ready to watch.'}</p></section>}
        </main>
      </div>}
  </div>;
}

export const WatchWithFriendsRoutePage = WatchWithFriendsPage;
