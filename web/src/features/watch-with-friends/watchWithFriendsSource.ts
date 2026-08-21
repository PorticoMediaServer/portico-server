import type {
  PorticoClient,
  WatchWithFriendsCreateRequest,
  WatchWithFriendsGroup,
  WatchWithFriendsMemberStateRequest,
  WatchWithFriendsQueueOrderRequest,
  WatchWithFriendsQueueRequest,
  WatchWithFriendsSettingsRequest,
  WatchWithFriendsStateRequest,
} from '@portico/client-core';

export type WatchConnectionState = 'connecting' | 'live' | 'reconnecting' | 'failed';

export interface WatchGroupSubscription {
  onGroup: (group: WatchWithFriendsGroup) => void;
  onStatus: (status: WatchConnectionState) => void;
  onError: (error: Error) => void;
}

export interface WatchWithFriendsViewer {
  profileId: string;
  displayName: string;
  canUse: boolean;
  canManageServer?: boolean;
}

export interface WatchWithFriendsSource {
  listGroups(signal?: AbortSignal): Promise<WatchWithFriendsGroup[]>;
  group(id: string, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  createGroup(request: WatchWithFriendsCreateRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  joinGroup(id: string, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  leaveGroup(id: string, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  endGroup(id: string, expectedRevision: number, idempotencyKey: string, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  updateMemberState(id: string, request: WatchWithFriendsMemberStateRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  updatePlaybackState(id: string, request: WatchWithFriendsStateRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  updateSettings(id: string, request: WatchWithFriendsSettingsRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  addQueueItem(id: string, request: WatchWithFriendsQueueRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  reorderQueue(id: string, request: WatchWithFriendsQueueOrderRequest, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  removeQueueItem(id: string, mediaId: string, expectedRevision: number, idempotencyKey: string, signal?: AbortSignal): Promise<WatchWithFriendsGroup>;
  subscribe(id: string, subscription: WatchGroupSubscription): () => void;
}

export type WatchWithFriendsClient = Pick<PorticoClient,
  | 'watchWithFriendsGroups'
  | 'watchWithFriendsGroup'
  | 'createWatchWithFriendsGroup'
  | 'joinWatchWithFriendsGroup'
  | 'leaveWatchWithFriendsGroup'
  | 'endWatchWithFriendsGroup'
  | 'updateWatchWithFriendsMemberState'
  | 'updateWatchWithFriendsState'
  | 'updateWatchWithFriendsSettings'
  | 'addWatchWithFriendsQueueItem'
  | 'reorderWatchWithFriendsQueue'
  | 'removeWatchWithFriendsQueueItem'
  | 'watchWithFriendsGroupEventsUrl'
>;

export function groupIncludesViewer(group: WatchWithFriendsGroup, viewer: WatchWithFriendsViewer) {
  return group.members.some((member) => member.profileId === viewer.profileId);
}

export function viewerCanHost(group: WatchWithFriendsGroup, viewer: WatchWithFriendsViewer) {
  return Boolean(group.permissions?.isHost && group.ownerProfileId === viewer.profileId);
}
