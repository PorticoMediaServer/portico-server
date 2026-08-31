import type {
  PlaybackCommand,
  WatchWithFriendsGroup,
  WatchWithFriendsMember,
  WatchWithFriendsQueueItem,
} from '@porticomediaserver/client-core';
import type {
  WatchConnectionState,
  WatchGroupSubscription,
  WatchWithFriendsSource,
  WatchWithFriendsViewer,
} from './watchWithFriendsSource';
import { secureRandomUUID } from '../../runtime/secureRandomUUID';

export interface FixtureWatchWithFriendsOptions {
  viewer: WatchWithFriendsViewer;
  groups?: WatchWithFriendsGroup[];
  mediaTitles?: Record<string, string>;
  now?: () => Date;
}

function cloneGroup(group: WatchWithFriendsGroup): WatchWithFriendsGroup {
  return {
    ...group,
    command: { ...group.command },
    members: group.members.map((member) => ({ ...member })),
    queue: group.queue.map((item) => ({ ...item })),
  };
}

function cancelled(signal?: AbortSignal) {
  if (signal?.aborted) throw new DOMException('The request was cancelled.', 'AbortError');
}

export class FixtureWatchWithFriendsSource implements WatchWithFriendsSource {
  private readonly groups = new Map<string, WatchWithFriendsGroup>();
  private readonly subscriptions = new Map<string, Set<WatchGroupSubscription>>();
  private readonly mediaTitles: Record<string, string>;
  private readonly now: () => Date;
  private groupSequence = 0;

  constructor(private readonly options: FixtureWatchWithFriendsOptions) {
    this.mediaTitles = { ...(options.mediaTitles ?? {}) };
    this.now = options.now ?? (() => new Date());
    for (const group of options.groups ?? []) this.groups.set(group.id, cloneGroup(group));
  }

  async listGroups(signal?: AbortSignal) {
    cancelled(signal);
    return [...this.groups.values()].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)).map(cloneGroup);
  }

  async group(id: string, signal?: AbortSignal) {
    cancelled(signal);
    return cloneGroup(this.requireGroup(id));
  }

  async createGroup(request: Parameters<WatchWithFriendsSource['createGroup']>[0], signal?: AbortSignal) {
    cancelled(signal);
    const mediaId = request.mediaId.trim();
    if (!mediaId) throw new Error('Choose a media item before creating a group.');
    const timestamp = this.timestamp();
    const mediaTitle = this.mediaTitles[mediaId] ?? mediaId;
    let id = '';
    do id = `fixture-watch-group-${++this.groupSequence}`;
    while (this.groups.has(id));
    const member: WatchWithFriendsMember = {
      profileId: this.options.viewer.profileId,
      displayName: this.options.viewer.displayName,
      state: 'joined',
      positionSeconds: 0,
      joinedAt: timestamp,
      lastSeenAt: timestamp,
    };
    const queueItem: WatchWithFriendsQueueItem = {
      entryId: `fixture-watch-entry-${secureRandomUUID()}`,
      mediaId,
      mediaTitle,
      sortOrder: 0,
      addedByProfileId: this.options.viewer.profileId,
      addedAt: timestamp,
      unavailable: false,
    };
    const group: WatchWithFriendsGroup = {
      id,
      name: request.name?.trim() || mediaTitle,
      ownerProfileId: this.options.viewer.profileId,
      ownerName: this.options.viewer.displayName,
      mediaId,
      mediaTitle,
      currentEntryId: queueItem.entryId,
      state: 'paused',
      positionSeconds: 0,
      positionUpdatedAt: timestamp,
      serverTime: timestamp,
      playbackRate: 1,
      revision: 1,
      playbackRevision: 1,
      reconnectGeneration: 0,
      shuffleEnabled: false,
      repeatMode: 'none',
      permissions: { isHost: true, canControl: true, canManageQueue: true },
      command: this.command('pause', mediaId, 0),
      members: [member],
      queue: [queueItem],
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    this.groups.set(id, group);
    this.emit(group);
    return cloneGroup(group);
  }

  async joinGroup(id: string, signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    if (!group.members.some((member) => member.profileId === this.options.viewer.profileId)) {
      const timestamp = this.timestamp();
      group.members.push({
        profileId: this.options.viewer.profileId,
        displayName: this.options.viewer.displayName,
        state: 'joined',
        positionSeconds: group.positionSeconds,
        joinedAt: timestamp,
        lastSeenAt: timestamp,
      });
      this.touch(group);
    }
    return this.publish(group);
  }

  async leaveGroup(id: string, signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    group.members = group.members.filter((member) => member.profileId !== this.options.viewer.profileId);
    this.touch(group);
    return this.publish(group);
  }

  async endGroup(id: string, _expectedRevision: number, _idempotencyKey: string, signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    group.state = 'stopped';
    this.touch(group);
    const ended = cloneGroup(group);
    this.groups.delete(id);
    this.emit(group);
    return ended;
  }

  async updateMemberState(id: string, request: Parameters<WatchWithFriendsSource['updateMemberState']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    const member = group.members.find((candidate) => candidate.profileId === this.options.viewer.profileId);
    if (!member) throw new Error('Join this group before updating readiness.');
    member.state = request.state;
    member.positionSeconds = Math.max(0, request.positionSeconds ?? group.positionSeconds);
    member.lastSeenAt = this.timestamp();
    this.touch(group);
    return this.publish(group);
  }

  async updatePlaybackState(id: string, request: Parameters<WatchWithFriendsSource['updatePlaybackState']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    let mediaId = group.mediaId;
    let mediaTitle = group.mediaTitle;
    let action: PlaybackCommand['action'] = request.action;
    let position = Math.max(0, request.positionSeconds ?? group.positionSeconds);
    if (request.action === 'next' || request.action === 'previous') {
      const currentIndex = group.queue.findIndex((item) => item.entryId === group.currentEntryId);
      const offset = request.action === 'next' ? 1 : -1;
      let targetIndex = currentIndex + offset;
      if (group.repeatMode === 'all') targetIndex = (targetIndex + group.queue.length) % group.queue.length;
      const target = group.queue[targetIndex];
      if (!target) throw new Error(`No ${request.action} queue item is available.`);
      mediaId = target.mediaId;
      mediaTitle = target.mediaTitle;
      group.currentEntryId = target.entryId;
      action = 'load';
      position = 0;
    } else if (request.action === 'load') {
      const requestedEntryId = request.entryId?.trim() ?? '';
      mediaId = request.mediaId?.trim() ?? '';
      let target = requestedEntryId ? group.queue.find((item) => item.entryId === requestedEntryId) : undefined;
      if (!target && mediaId) {
        target = {
          entryId: `fixture-watch-entry-${secureRandomUUID()}`, mediaId,
          mediaTitle: this.mediaTitles[mediaId] ?? mediaId, sortOrder: group.queue.length,
          addedByProfileId: this.options.viewer.profileId, addedAt: this.timestamp(), unavailable: false,
        };
        group.queue.push(target);
      }
      if (!target) throw new Error('The selected queue occurrence is not available.');
      mediaId = target.mediaId;
      mediaTitle = target.mediaTitle;
      group.currentEntryId = target.entryId;
      position = 0;
    }
    group.mediaId = mediaId;
    group.mediaTitle = mediaTitle;
    group.positionSeconds = position;
    group.state = request.action === 'stop'
      ? 'stopped'
      : request.action === 'pause' || request.action === 'seek'
        ? 'paused'
        : 'playing';
    group.command = this.command(action, mediaId, position);
    this.touch(group);
    return this.publish(group);
  }

  async updateSettings(id: string, request: Parameters<WatchWithFriendsSource['updateSettings']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    if (request.shuffleEnabled !== undefined) group.shuffleEnabled = request.shuffleEnabled;
    if (request.repeatMode !== undefined) group.repeatMode = request.repeatMode;
    this.touch(group);
    return this.publish(group);
  }

  async addQueueItem(id: string, request: Parameters<WatchWithFriendsSource['addQueueItem']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    const mediaId = request.mediaId.trim();
    if (!mediaId) throw new Error('Enter a media ID to add it to the queue.');
    group.queue.push({
        entryId: `fixture-watch-entry-${secureRandomUUID()}`,
        mediaId,
        mediaTitle: this.mediaTitles[mediaId] ?? mediaId,
        sortOrder: group.queue.length,
        addedByProfileId: this.options.viewer.profileId,
        addedAt: this.timestamp(),
        unavailable: false,
      });
    this.touch(group);
    return this.publish(group);
  }

  async reorderQueue(id: string, request: Parameters<WatchWithFriendsSource['reorderQueue']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    const from = group.queue.findIndex((item) => item.entryId === request.entryId);
    if (from < 0) throw new Error('Queue occurrence was not found.');
    const [entry] = group.queue.splice(from, 1);
    let destination = group.queue.findIndex((item) => item.entryId === request.destinationEntryId);
    if (destination < 0) throw new Error('Queue destination was not found.');
    if (request.placement === 'after') destination += 1;
    group.queue.splice(destination, 0, entry);
    group.queue = group.queue.map((item, sortOrder) => ({ ...item, sortOrder }));
    this.touch(group);
    return this.publish(group);
  }

  async removeQueueItem(id: string, entryId: string, _expectedRevision: number, _idempotencyKey: string, signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    if (group.currentEntryId === entryId) throw new Error('The currently playing item cannot be removed.');
    const next = group.queue.filter((item) => item.entryId !== entryId);
    if (next.length === group.queue.length) throw new Error('Queue item was not found.');
    group.queue = next.map((item, sortOrder) => ({ ...item, sortOrder }));
    this.touch(group);
    return this.publish(group);
  }

  subscribe(id: string, subscription: WatchGroupSubscription) {
    const subscriptions = this.subscriptions.get(id) ?? new Set<WatchGroupSubscription>();
    subscriptions.add(subscription);
    this.subscriptions.set(id, subscriptions);
    queueMicrotask(() => {
      if (!subscriptions.has(subscription)) return;
      subscription.onStatus('live');
      const group = this.groups.get(id);
      if (group) subscription.onGroup(cloneGroup(group));
    });
    return () => {
      subscriptions.delete(subscription);
      if (subscriptions.size === 0) this.subscriptions.delete(id);
    };
  }

  setConnectionState(id: string, state: WatchConnectionState, error?: Error) {
    for (const subscription of this.subscriptions.get(id) ?? []) {
      subscription.onStatus(state);
      if (error) subscription.onError(error);
    }
  }

  snapshot(id: string) {
    const group = this.groups.get(id);
    return group ? cloneGroup(group) : undefined;
  }

  private requireGroup(id: string) {
    const group = this.groups.get(id);
    if (!group) throw new Error('Watch With Friends group was not found.');
    return group;
  }

  private timestamp() {
    return this.now().toISOString();
  }

  private touch(group: WatchWithFriendsGroup) {
    group.updatedAt = this.timestamp();
    group.serverTime = group.updatedAt;
    group.revision += 1;
  }

  private command(action: PlaybackCommand['action'], mediaId: string, positionSeconds: number): PlaybackCommand {
    return {
      id: `fixture-command-${this.timestamp()}`,
      action,
      mediaId,
      positionSeconds,
      issuedAt: this.timestamp(),
      issuedByProfileId: this.options.viewer.profileId,
    };
  }

  private publish(group: WatchWithFriendsGroup) {
    this.emit(group);
    return cloneGroup(group);
  }

  private emit(group: WatchWithFriendsGroup) {
    for (const subscription of this.subscriptions.get(group.id) ?? []) subscription.onGroup(cloneGroup(group));
  }
}
