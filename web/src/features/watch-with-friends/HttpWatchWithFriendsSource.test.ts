import type { WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { HttpWatchWithFriendsSource } from './HttpWatchWithFriendsSource';
import type { WatchWithFriendsClient } from './watchWithFriendsSource';

const group: WatchWithFriendsGroup = {
  id: 'group-1',
  name: 'Friday screening',
  ownerProfileId: 'owner',
  ownerName: 'Owner',
  mediaId: 'fargo',
  mediaTitle: 'Fargo',
  state: 'paused',
  positionSeconds: 0,
  positionUpdatedAt: '2026-07-11T12:00:00.000Z',
  serverTime: '2026-07-11T12:00:00.000Z',
  playbackRate: 1,
  revision: 1,
  playbackRevision: 1,
  reconnectGeneration: 0,
  permissions: { isHost: true, canControl: true, canManageQueue: true },
  shuffleEnabled: false,
  repeatMode: 'none',
  command: { action: 'pause' },
  members: [],
  queue: [],
  createdAt: '2026-07-11T12:00:00.000Z',
  updatedAt: '2026-07-11T12:00:00.000Z',
};

function clientStub(): WatchWithFriendsClient {
  return {
    watchWithFriendsGroups: vi.fn().mockResolvedValue({ items: [group] }),
    watchWithFriendsGroup: vi.fn().mockResolvedValue(group),
    createWatchWithFriendsGroup: vi.fn().mockResolvedValue(group),
    joinWatchWithFriendsGroup: vi.fn().mockResolvedValue(group),
    leaveWatchWithFriendsGroup: vi.fn().mockResolvedValue(group),
    endWatchWithFriendsGroup: vi.fn().mockResolvedValue(group),
    updateWatchWithFriendsMemberState: vi.fn().mockResolvedValue(group),
    updateWatchWithFriendsState: vi.fn().mockResolvedValue(group),
    updateWatchWithFriendsSettings: vi.fn().mockResolvedValue(group),
    addWatchWithFriendsQueueItem: vi.fn().mockResolvedValue(group),
    reorderWatchWithFriendsQueue: vi.fn().mockResolvedValue(group),
    removeWatchWithFriendsQueueItem: vi.fn().mockResolvedValue(group),
    watchWithFriendsGroupEventsUrl: vi.fn().mockReturnValue('/api/watch-with-friends/groups/group-1/events'),
  };
}

class FakeEventStream {
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly close = vi.fn();
  private readonly listeners = new Map<string, EventListener[]>();

  addEventListener(type: string, listener: EventListener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  open() {
    this.onopen?.(new Event('open'));
  }

  fail() {
    this.onerror?.(new Event('error'));
  }

  sendGroup(value: WatchWithFriendsGroup) {
    const event = new MessageEvent('group', { data: JSON.stringify(value) });
    for (const listener of this.listeners.get('group') ?? []) listener(event);
  }
}

afterEach(() => vi.useRealTimers());

describe('HttpWatchWithFriendsSource', () => {
  it('delegates the feature-local contract to PorticoClient', async () => {
    const client = clientStub();
    const source = new HttpWatchWithFriendsSource(client);
    await expect(source.listGroups()).resolves.toEqual([group]);
    await source.createGroup({ mediaId: 'fargo', name: 'Friday screening' });
    await source.updatePlaybackState('group-1', { action: 'play', positionSeconds: 12, expectedRevision: 1, idempotencyKey: 'play-1' });
    await source.reorderQueue('group-1', { mediaIds: ['fargo', 'rookie'], expectedRevision: 1, idempotencyKey: 'reorder-1' });
    await source.removeQueueItem('group-1', 'rookie', 1, 'remove-1');
    await source.endGroup('group-1', 1, 'end-1');

    expect(client.watchWithFriendsGroups).toHaveBeenCalledOnce();
    expect(client.createWatchWithFriendsGroup).toHaveBeenCalledWith({ mediaId: 'fargo', name: 'Friday screening' });
    expect(client.updateWatchWithFriendsState).toHaveBeenCalledWith('group-1', { action: 'play', positionSeconds: 12, expectedRevision: 1, idempotencyKey: 'play-1' });
    expect(client.reorderWatchWithFriendsQueue).toHaveBeenCalledWith('group-1', { mediaIds: ['fargo', 'rookie'], expectedRevision: 1, idempotencyKey: 'reorder-1' });
    expect(client.removeWatchWithFriendsQueueItem).toHaveBeenCalledWith('group-1', 'rookie', 1, 'remove-1');
    expect(client.endWatchWithFriendsGroup).toHaveBeenCalledWith('group-1', 1, 'end-1');
  });

  it('publishes group events, reconnects once, and exposes terminal SSE failure', () => {
    vi.useFakeTimers();
    const client = clientStub();
    const first = new FakeEventStream();
    const second = new FakeEventStream();
    const createEventStream = vi.fn()
      .mockReturnValueOnce(first)
      .mockReturnValueOnce(second);
    const source = new HttpWatchWithFriendsSource(client, { createEventStream, reconnectDelaysMs: [50] });
    const statuses: string[] = [];
    const onGroup = vi.fn();
    const onError = vi.fn();

    const unsubscribe = source.subscribe('group-1', {
      onGroup,
      onStatus: (status) => statuses.push(status),
      onError,
    });
    expect(createEventStream).toHaveBeenCalledWith('/api/watch-with-friends/groups/group-1/events');
    first.open();
    first.sendGroup(group);
    expect(statuses).toEqual(['connecting', 'live']);
    expect(onGroup).toHaveBeenCalledWith(group);

    first.fail();
    expect(first.close).toHaveBeenCalledOnce();
    expect(statuses.at(-1)).toBe('reconnecting');
    vi.advanceTimersByTime(50);
    expect(createEventStream).toHaveBeenCalledTimes(2);
    second.fail();
    expect(statuses.at(-1)).toBe('failed');
    expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: 'Live group updates were interrupted.' }));

    unsubscribe();
    expect(second.close).toHaveBeenCalled();
  });

  it('stops retries and delivery when a subscription is disposed', () => {
    vi.useFakeTimers();
    const client = clientStub();
    const stream = new FakeEventStream();
    const createEventStream = vi.fn().mockReturnValue(stream);
    const source = new HttpWatchWithFriendsSource(client, { createEventStream, reconnectDelaysMs: [50] });
    const onGroup = vi.fn();
    const unsubscribe = source.subscribe('group-1', { onGroup, onStatus: vi.fn(), onError: vi.fn() });

    stream.fail();
    unsubscribe();
    vi.advanceTimersByTime(50);
    stream.sendGroup(group);
    expect(createEventStream).toHaveBeenCalledTimes(1);
    expect(onGroup).not.toHaveBeenCalled();
  });
});
