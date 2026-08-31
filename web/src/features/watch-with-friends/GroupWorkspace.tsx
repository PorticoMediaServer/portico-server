import type {
  WatchWithFriendsGroup,
  WatchWithFriendsMemberStateRequest,
  WatchWithFriendsSettingsRequest,
  WatchWithFriendsStateRequest,
} from '@porticomediaserver/client-core';
import { NavigationMoveDownIcon, NavigationMoveUpIcon, PlaybackPreviousIcon, PlaybackNextIcon, StatusSuccessIcon, PlaybackQueueIcon, StatusLoadingIcon, PlaybackPauseIcon, PlaybackPlayIcon, ActionAddIcon, ActionRefreshIcon, ActionResetIcon, PlaybackShuffleIcon, ActionDeleteIcon, AccountVerifiedIcon, AccountWatchTogetherIcon, DeviceWifiIcon, DeviceOfflineIcon } from '#portico-icons';
import { useState, type FormEvent } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { MediaSearchPicker } from './MediaSearchPicker';
import { connectionLabel, initials, relativeSeen, watchClock, watchStateLabel } from './watchFormat';
import {
  groupIncludesViewer,
  viewerCanHost,
  type WatchConnectionState,
  type WatchWithFriendsViewer,
} from './watchWithFriendsSource';

type WatchWithFriendsPlaybackDraft = Omit<WatchWithFriendsStateRequest, 'expectedRevision' | 'idempotencyKey'>;
type WatchWithFriendsSettingsDraft = Omit<WatchWithFriendsSettingsRequest, 'expectedRevision' | 'idempotencyKey'>;

export interface GroupWorkspaceActions {
  join: () => Promise<boolean>;
  leave: () => Promise<boolean>;
  end: () => Promise<boolean>;
  retryEvents: () => void;
  refresh: () => Promise<boolean>;
  updateMember: (request: WatchWithFriendsMemberStateRequest) => Promise<boolean>;
  updatePlayback: (request: WatchWithFriendsPlaybackDraft) => Promise<boolean>;
  updateSettings: (request: WatchWithFriendsSettingsDraft) => Promise<boolean>;
  addQueueItem: (mediaId: string) => Promise<boolean>;
  reorderQueue: (entryId: string, destinationEntryId: string, placement: 'before' | 'after') => Promise<boolean>;
  removeQueueItem: (entryId: string) => Promise<boolean>;
}

function ConnectionState({ state }: { state: WatchConnectionState | 'idle' }) {
  const Icon = state === 'live' ? DeviceWifiIcon : state === 'failed' ? DeviceOfflineIcon : ActionRefreshIcon;
  return <span className={`watch-connection ${state}`}><Icon className={state === 'connecting' || state === 'reconnecting' ? 'watch-spin' : ''} />{connectionLabel(state)}</span>;
}

function GroupTransport({ group, canHost, busy, onCommand }: {
  group: WatchWithFriendsGroup;
  canHost: boolean;
  busy: boolean;
  onCommand: (request: WatchWithFriendsPlaybackDraft) => Promise<boolean>;
}) {
  const currentIndex = group.queue.findIndex((item) => item.mediaId === group.mediaId);
  const hasPrevious = currentIndex > 0 || (group.repeatMode === 'all' && group.queue.length > 1);
  const hasNext = currentIndex >= 0 && (currentIndex < group.queue.length - 1 || (group.repeatMode === 'all' && group.queue.length > 1));
  const toggleAction = group.state === 'playing' ? 'pause' : 'play';
  return <section className="watch-sync-rail" aria-label="Group playback controls">
    <div className="watch-now-playing"><span><PlaybackPlayIcon /></span><div><strong>{group.mediaTitle}</strong><span>{group.state === 'playing' ? 'Playing for the group' : 'Paused for the group'}</span></div></div>
    {canHost
      ? <div className="watch-transport-controls">
        <IconButton label="Previous queue item" disabled={busy || !hasPrevious} onClick={() => void onCommand({ action: 'previous' })}><PlaybackPreviousIcon /></IconButton>
        <IconButton label="Back 10 seconds" disabled={busy} onClick={() => void onCommand({ action: 'seek', positionSeconds: Math.max(0, group.positionSeconds - 10) })}><PlaybackPreviousIcon /></IconButton>
        <IconButton label={toggleAction === 'play' ? 'Play for group' : 'Pause for group'} className="watch-primary-transport" disabled={busy} onClick={() => void onCommand({ action: toggleAction, positionSeconds: group.positionSeconds })}>{toggleAction === 'play' ? <PlaybackPlayIcon /> : <PlaybackPauseIcon />}</IconButton>
        <IconButton label="Forward 30 seconds" disabled={busy} onClick={() => void onCommand({ action: 'seek', positionSeconds: group.positionSeconds + 30 })}><PlaybackNextIcon /></IconButton>
        <IconButton label="Next queue item" disabled={busy || !hasNext} onClick={() => void onCommand({ action: 'next' })}><PlaybackNextIcon /></IconButton>
      </div>
      : <div className="watch-transport-owner"><AccountWatchTogetherIcon /><span>Playback is controlled by {group.ownerName}</span></div>}
    <time>{watchClock(group.positionSeconds)}</time>
  </section>;
}

function MemberRoster({ group, viewer, busy, onUpdate }: {
  group: WatchWithFriendsGroup;
  viewer: WatchWithFriendsViewer;
  busy: boolean;
  onUpdate: (request: WatchWithFriendsMemberStateRequest) => Promise<boolean>;
}) {
  const ownMember = group.members.find((member) => member.profileId === viewer.profileId);
  const ready = group.members.filter((member) => member.state === 'ready' || member.state === 'playing' || member.state === 'paused').length;
  return <section className="watch-members watch-panel">
    <header><div><h3>People</h3><span>{ready} of {group.members.length} ready</span></div>{ownMember && <div className="watch-readiness-controls"><SecondaryButton selected={ownMember.state === 'ready'} disabled={busy} onClick={() => void onUpdate({ state: 'ready', positionSeconds: group.positionSeconds })}><StatusSuccessIcon /> Ready</SecondaryButton><SecondaryButton selected={ownMember.state === 'buffering'} disabled={busy} onClick={() => void onUpdate({ state: 'buffering', positionSeconds: group.positionSeconds })}><ActionResetIcon /> Buffering</SecondaryButton></div>}</header>
    <div className="watch-member-list">{group.members.map((member) => <article key={member.profileId}>
      <span className="watch-member-avatar">{initials(member.displayName)}</span>
      <div><strong>{member.displayName}{member.profileId === viewer.profileId ? ' (you)' : ''}</strong><span>{member.profileId === group.ownerProfileId ? 'Host' : relativeSeen(member.lastSeenAt)}</span></div>
      <span className={`watch-member-state ${member.state ?? 'joined'}`}>{watchStateLabel(member.state)}</span>
    </article>)}</div>
  </section>;
}

function QueueWorkspace({ group, canHost, busy, onAdd, onReorder, onRemove, onPlayNow }: {
  group: WatchWithFriendsGroup;
  canHost: boolean;
  busy: boolean;
  onAdd: (mediaId: string) => Promise<boolean>;
  onReorder: (entryId: string, destinationEntryId: string, placement: 'before' | 'after') => Promise<boolean>;
  onRemove: (entryId: string) => Promise<boolean>;
  onPlayNow: (entryId: string) => Promise<boolean>;
}) {
  const [mediaId, setMediaId] = useState('');
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!mediaId.trim()) return;
    void onAdd(mediaId.trim()).then((succeeded) => {
      if (succeeded) setMediaId('');
    });
  };
  const move = (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction;
    if (nextIndex < 0 || nextIndex >= group.queue.length) return;
    const item = group.queue[index];
    const destination = group.queue[nextIndex];
    if (item && destination) void onReorder(item.entryId, destination.entryId, direction < 0 ? 'before' : 'after');
  };
  return <section className="watch-queue watch-panel">
    <header><div><h3>Queue</h3><span>{group.queue.length} item{group.queue.length === 1 ? '' : 's'}</span></div></header>
    {canHost && <form className="watch-queue-add" onSubmit={submit}>
      <MediaSearchPicker
        value={mediaId}
        onChange={setMediaId}
        label="Add to queue"
        placeholder="Search your server"
        inputLabel="Search media to add to queue"
        compact
        disabled={busy}
      />
      <button type="submit" disabled={busy || !mediaId.trim()}><ActionAddIcon /> Add</button>
    </form>}
    {group.queue.length === 0
      ? <div className="watch-queue-empty"><PlaybackQueueIcon /><strong>The queue is empty</strong></div>
      : <ol>{group.queue.map((item, index) => {
        const current = item.entryId === group.currentEntryId;
        return <li className={current ? 'current' : ''} key={item.entryId}>
          <span className="watch-queue-number">{index + 1}</span>
          <div><strong>{item.mediaTitle}</strong><span>{current ? 'Now playing' : 'Up next'}</span></div>
          {canHost && <div className="watch-queue-actions">
            {!current && <button type="button" disabled={busy || item.unavailable} onClick={() => void onPlayNow(item.entryId)} aria-label={`Play ${item.mediaTitle} now`}><PlaybackPlayIcon /></button>}
            <button type="button" disabled={busy || index === 0} onClick={() => move(index, -1)} aria-label={`Move ${item.mediaTitle} up`}><NavigationMoveUpIcon /></button>
            <button type="button" disabled={busy || index === group.queue.length - 1} onClick={() => move(index, 1)} aria-label={`Move ${item.mediaTitle} down`}><NavigationMoveDownIcon /></button>
            <button type="button" disabled={busy || current} onClick={() => void onRemove(item.entryId)} aria-label={`Remove ${item.mediaTitle} from queue`}><ActionDeleteIcon /></button>
          </div>}
        </li>;
      })}</ol>}
  </section>;
}

function GroupSettings({ group, canHost, busy, onUpdate }: {
  group: WatchWithFriendsGroup;
  canHost: boolean;
  busy: boolean;
  onUpdate: (request: WatchWithFriendsSettingsDraft) => Promise<boolean>;
}) {
  return <section className="watch-group-settings watch-panel">
    <header><div><h3>Playback order</h3><span>{canHost ? 'Applies to everyone in the group' : `Managed by ${group.ownerName}`}</span></div></header>
    <div className="watch-setting-row"><div><PlaybackShuffleIcon /><span><strong>Shuffle</strong><span>{group.shuffleEnabled ? 'Enabled' : 'Disabled'}</span></span></div>{canHost && <button type="button" role="switch" aria-label={group.shuffleEnabled ? 'Disable shuffle' : 'Enable shuffle'} aria-checked={group.shuffleEnabled} disabled={busy} onClick={() => void onUpdate({ shuffleEnabled: !group.shuffleEnabled })}><span /></button>}</div>
    <div className="watch-setting-row repeat"><div><ActionRefreshIcon /><span><strong>Repeat</strong><span>{group.repeatMode === 'none' ? 'Off' : group.repeatMode === 'one' ? 'Current item' : 'Entire queue'}</span></span></div>{canHost && <div className="watch-repeat-options">{(['none', 'one', 'all'] as const).map((mode) => <button type="button" className={group.repeatMode === mode ? 'selected' : ''} disabled={busy} key={mode} onClick={() => void onUpdate({ repeatMode: mode })}>{mode === 'none' ? 'Off' : mode === 'one' ? 'One' : 'All'}</button>)}</div>}</div>
  </section>;
}

export function GroupWorkspace({
  group,
  viewer,
  connection,
  connectionError,
  busy,
  actions,
}: {
  group: WatchWithFriendsGroup;
  viewer: WatchWithFriendsViewer;
  connection: WatchConnectionState | 'idle';
  connectionError: string;
  busy: string;
  actions: GroupWorkspaceActions;
}) {
  const [confirmEnd, setConfirmEnd] = useState(false);
  const joined = groupIncludesViewer(group, viewer);
  const canHost = viewerCanHost(group, viewer);
  const isBusy = busy !== '';

  if (!joined) return <section className="watch-group-preview">
    <AccountWatchTogetherIcon />
    <h2>{group.name}</h2>
    <p>{group.ownerName} is watching {group.mediaTitle} with {group.members.length} member{group.members.length === 1 ? '' : 's'}.</p>
    <PrimaryButton disabled={busy === `join:${group.id}`} onClick={() => void actions.join()}>{busy === `join:${group.id}` ? <StatusLoadingIcon className="watch-spin" /> : <AccountVerifiedIcon />} Join group</PrimaryButton>
  </section>;

  return <div className="watch-active-workspace">
    <header className="watch-group-header">
      <div><span>{group.ownerName} hosts</span><h2>{group.name}</h2><p>{group.mediaTitle}</p></div>
      <div className="watch-group-header-actions"><ConnectionState state={connection} /><IconButton label="Refresh group" disabled={isBusy} onClick={() => void actions.refresh()}><ActionRefreshIcon /></IconButton>{canHost ? <SecondaryButton disabled={isBusy} onClick={() => setConfirmEnd(true)}>End group</SecondaryButton> : <SecondaryButton disabled={isBusy} onClick={() => void actions.leave()}>Leave group</SecondaryButton>}</div>
    </header>
    {connection === 'failed' && <div className="watch-alert connection" role="alert"><DeviceOfflineIcon /><span><strong>Live updates stopped</strong><span>{connectionError || 'Portico could not reconnect to this group.'}</span></span><SecondaryButton onClick={actions.retryEvents}>Reconnect</SecondaryButton></div>}
    <GroupTransport group={group} canHost={canHost} busy={isBusy} onCommand={actions.updatePlayback} />
    <div className="watch-workspace-columns">
      <div><MemberRoster group={group} viewer={viewer} busy={isBusy} onUpdate={actions.updateMember} /><GroupSettings group={group} canHost={canHost} busy={isBusy} onUpdate={actions.updateSettings} /></div>
      <QueueWorkspace group={group} canHost={canHost} busy={isBusy} onAdd={actions.addQueueItem} onReorder={actions.reorderQueue} onRemove={actions.removeQueueItem} onPlayNow={(entryId) => actions.updatePlayback({ action: 'load', entryId, positionSeconds: 0 })} />
    </div>
    {confirmEnd && <ModalOverlay className="watch-end-dialog" labelledBy="watch-end-title" onDismiss={() => { if (!isBusy) setConfirmEnd(false); }}><h2 id="watch-end-title">End “{group.name}”?</h2><p>Playback synchronization and live group updates will stop for every member.</p><div><SecondaryButton disabled={isBusy} onClick={() => setConfirmEnd(false)}>Cancel</SecondaryButton><button className="watch-destructive" type="button" disabled={isBusy} onClick={() => void actions.end().then((succeeded) => { if (succeeded) setConfirmEnd(false); })}>{isBusy ? 'Ending…' : 'End group'}</button></div></ModalOverlay>}
  </div>;
}
