import type {
  PlaybackCommand,
  WatchWithFriendsGroup,
  WatchWithFriendsMember,
  WatchWithFriendsQueueItem,
} from '@portico/client-core';
import type {
  WatchConnectionState,
  WatchGroupSubscription,
  WatchWithFriendsSource,
  WatchWithFriendsViewer,
} from './watchWithFriendsSource';

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
      mediaId,
      mediaTitle,
      sortOrder: 0,
      addedByProfileId: this.options.viewer.profileId,
      addedAt: timestamp,
    };
    const group: WatchWithFriendsGroup = {
      id,
      name: request.name?.trim() || mediaTitle,
      ownerProfileId: this.options.viewer.profileId,
      ownerName: this.options.viewer.displayName,
      mediaId,
      mediaTitle,
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
      const currentIndex = group.queue.findIndex((item) => item.mediaId === group.mediaId);
      const offset = request.action === 'next' ? 1 : -1;
      let targetIndex = currentIndex + offset;
      if (group.repeatMode === 'all') targetIndex = (targetIndex + group.queue.length) % group.queue.length;
      const target = group.queue[targetIndex];
      if (!target) throw new Error(`No ${request.action} queue item is available.`);
      mediaId = target.mediaId;
      mediaTitle = target.mediaTitle;
      action = 'load';
      position = 0;
    } else if (request.action === 'load') {
      mediaId = request.mediaId?.trim() ?? '';
      const target = group.queue.find((item) => item.mediaId === mediaId);
      if (!target) throw new Error('The selected media item is not in this queue.');
      mediaTitle = target.mediaTitle;
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
    if (!group.queue.some((item) => item.mediaId === mediaId)) {
      group.queue.push({
        mediaId,
        mediaTitle: this.mediaTitles[mediaId] ?? mediaId,
        sortOrder: group.queue.length,
        addedByProfileId: this.options.viewer.profileId,
        addedAt: this.timestamp(),
      });
      this.touch(group);
    }
    return this.publish(group);
  }

  async reorderQueue(id: string, request: Parameters<WatchWithFriendsSource['reorderQueue']>[1], signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    if (request.mediaIds.length !== group.queue.length || new Set(request.mediaIds).size !== group.queue.length) {
      throw new Error('Queue order must contain every queued item once.');
    }
    const byId = new Map(group.queue.map((item) => [item.mediaId, item]));
    group.queue = request.mediaIds.map((mediaId, sortOrder) => {
      const item = byId.get(mediaId);
      if (!item) throw new Error('Queue order contains an unknown media item.');
      return { ...item, sortOrder };
    });
    this.touch(group);
    return this.publish(group);
  }

  async removeQueueItem(id: string, mediaId: string, _expectedRevision: number, _idempotencyKey: string, signal?: AbortSignal) {
    cancelled(signal);
    const group = this.requireGroup(id);
    if (group.mediaId === mediaId) throw new Error('The currently playing item cannot be removed.');
    const next = group.queue.filter((item) => item.mediaId !== mediaId);
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
