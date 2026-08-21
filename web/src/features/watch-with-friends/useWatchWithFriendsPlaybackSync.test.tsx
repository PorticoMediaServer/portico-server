import type { WatchWithFriendsGroup } from '@portico/client-core';
import { renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { WatchWithFriendsSource, WatchWithFriendsViewer } from './watchWithFriendsSource';
import { useWatchWithFriendsPlaybackSync } from './useWatchWithFriendsPlaybackSync';

const playback = vi.hoisted(() => ({
  current: undefined as undefined | {
    playback?: { media: { id: string }; sessionId: string };
    mediaRef: { current: HTMLVideoElement | null };
    applyExternalCommand: ReturnType<typeof vi.fn>;
  },
}));

vi.mock('../player/PlayerSurface', () => ({
  useOptionalPlaybackSession: () => playback.current,
}));

const viewer: WatchWithFriendsViewer = {
  profileId: 'owner',
  displayName: 'Portico Review',
  canUse: true,
};

function group(command: WatchWithFriendsGroup['command'], state: WatchWithFriendsGroup['state'] = 'paused'): WatchWithFriendsGroup {
  const timestamp = '2026-07-13T12:00:00Z';
  return {
    id: 'group-1',
    name: 'Friday screening',
    ownerProfileId: viewer.profileId,
    ownerName: viewer.displayName,
    mediaId: 'fargo',
    mediaTitle: 'Fargo',
    state,
    positionSeconds: 42,
    positionUpdatedAt: timestamp,
    serverTime: timestamp,
    playbackRate: 1,
    revision: 1,
    playbackRevision: 1,
    reconnectGeneration: 0,
    permissions: { isHost: true, canControl: true, canManageQueue: true },
    shuffleEnabled: false,
    repeatMode: 'none',
    command,
    members: [{ profileId: viewer.profileId, displayName: viewer.displayName, joinedAt: timestamp, lastSeenAt: timestamp }],
    queue: [],
    createdAt: timestamp,
    updatedAt: timestamp,
  };
}

function source() {
  return {
    group: vi.fn(),
    updateMemberState: vi.fn().mockResolvedValue(undefined),
    updatePlaybackState: vi.fn().mockResolvedValue(undefined),
  } as unknown as WatchWithFriendsSource;
}

describe('Watch With Friends playback synchronization', () => {
  beforeEach(() => {
    playback.current = {
      mediaRef: { current: null },
      applyExternalCommand: vi.fn().mockResolvedValue(undefined),
    };
  });

  it('applies each new group command once to the real player bridge', async () => {
    const dataSource = source();
    const initial = group({ id: 'command-1', action: 'pause', mediaId: 'fargo', positionSeconds: 42 });
    const { rerender } = renderHook(({ value }) => useWatchWithFriendsPlaybackSync({ group: value, source: dataSource, viewer }), {
      initialProps: { value: initial },
    });

    await waitFor(() => expect(playback.current?.applyExternalCommand).toHaveBeenCalledWith(expect.objectContaining({ action: 'load', mediaId: 'fargo', positionSeconds: 42 })));
    expect(playback.current?.applyExternalCommand).toHaveBeenCalledWith(expect.objectContaining({ action: 'pause', positionSeconds: 42 }));
    rerender({ value: initial });
    expect(playback.current?.applyExternalCommand).toHaveBeenCalledTimes(2);

    playback.current = {
      playback: { media: { id: 'fargo' }, sessionId: 'session-1' },
      mediaRef: { current: null },
      applyExternalCommand: playback.current?.applyExternalCommand ?? vi.fn().mockResolvedValue(undefined),
    };
    const playing = {
      ...group({ id: 'command-2', action: 'play', mediaId: 'fargo', positionSeconds: 45 }, 'playing'),
      revision: 2,
      playbackRevision: 2,
    };
    rerender({ value: playing });
    await waitFor(() => expect(playback.current?.applyExternalCommand).toHaveBeenCalledWith(expect.objectContaining({ action: 'play' })));
  });

  it('closes playback when the synchronized group stops', async () => {
    const stopped = group({ id: 'command-stop', action: 'stop', message: 'The host ended this group.' }, 'stopped');
    renderHook(() => useWatchWithFriendsPlaybackSync({ group: stopped, source: source(), viewer }));

    await waitFor(() => expect(playback.current?.applyExternalCommand).toHaveBeenCalledWith(expect.objectContaining({
      action: 'stop',
      message: 'The host ended this group.',
    })));
  });

  it('publishes host transport changes after remote synchronization settles', async () => {
    const media = document.createElement('video');
    Object.defineProperty(media, 'paused', { configurable: true, value: false });
    Object.defineProperty(media, 'currentTime', { configurable: true, writable: true, value: 64 });
    playback.current = {
      playback: { media: { id: 'fargo' }, sessionId: 'session-1' },
      mediaRef: { current: media },
      applyExternalCommand: vi.fn().mockResolvedValue(undefined),
    };
    const dataSource = source();
    const updatePlaybackState = vi.mocked(dataSource.updatePlaybackState);
    renderHook(() => useWatchWithFriendsPlaybackSync({
      group: group({ id: 'command-1', action: 'play', mediaId: 'fargo', positionSeconds: 64 }, 'playing'),
      source: dataSource,
      viewer,
    }));

    await new Promise((resolve) => window.setTimeout(resolve, 400));
    media.dispatchEvent(new Event('play'));

    await waitFor(() => expect(updatePlaybackState).toHaveBeenCalledWith('group-1', expect.objectContaining({
      action: 'play',
      positionSeconds: 64,
      expectedRevision: 1,
      playbackRate: 1,
    })));
  });
});
