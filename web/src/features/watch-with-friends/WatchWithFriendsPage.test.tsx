import type { WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { FixtureWatchWithFriendsSource } from './FixtureWatchWithFriendsSource';
import { WatchWithFriendsPage } from './WatchWithFriendsPage';
import type { WatchWithFriendsViewer } from './watchWithFriendsSource';

const timestamp = '2026-07-11T12:00:00.000Z';

const owner: WatchWithFriendsViewer = {
  profileId: 'owner',
  displayName: 'Portico Review',
  canUse: true,
};

function renderPage(page: ReactNode) {
  return render(<DataProvider source={new FixturePorticoDataSource()}>{page}</DataProvider>);
}

function groupFor({
  viewer = owner,
  ownerProfileId = viewer.profileId,
  ownerName = viewer.displayName,
  includeViewer = true,
}: {
  viewer?: WatchWithFriendsViewer;
  ownerProfileId?: string;
  ownerName?: string;
  includeViewer?: boolean;
} = {}): WatchWithFriendsGroup {
  return {
    id: 'watch-group-1',
    name: 'Friday screening',
    ownerProfileId,
    ownerName,
    mediaId: 'fargo',
    mediaTitle: 'Fargo · The Castle',
    state: 'paused',
    positionSeconds: 612,
    positionUpdatedAt: timestamp,
    serverTime: timestamp,
    playbackRate: 1,
    revision: 1,
    playbackRevision: 1,
    reconnectGeneration: 0,
    permissions: { isHost: viewer.profileId === ownerProfileId, canControl: viewer.profileId === ownerProfileId, canManageQueue: viewer.profileId === ownerProfileId },
    shuffleEnabled: false,
    repeatMode: 'none',
    command: {
      id: 'command-1',
      action: 'pause',
      mediaId: 'fargo',
      positionSeconds: 612,
      issuedAt: timestamp,
      issuedByProfileId: ownerProfileId,
    },
    members: [
      {
        profileId: ownerProfileId,
        displayName: ownerName,
        state: 'ready',
        positionSeconds: 612,
        joinedAt: timestamp,
        lastSeenAt: timestamp,
      },
      ...(includeViewer && viewer.profileId !== ownerProfileId
        ? [{
            profileId: viewer.profileId,
            displayName: viewer.displayName,
            state: 'joined' as const,
            positionSeconds: 612,
            joinedAt: timestamp,
            lastSeenAt: timestamp,
          }]
        : []),
    ],
    queue: [{
      mediaId: 'fargo',
      mediaTitle: 'Fargo · The Castle',
      sortOrder: 0,
      addedByProfileId: ownerProfileId,
      addedAt: timestamp,
    }],
    createdAt: timestamp,
    updatedAt: timestamp,
  };
}

describe('Watch With Friends workspace', () => {
  it('creates a truthful group and supports host playback, readiness, queue, and order controls', async () => {
    const source = new FixtureWatchWithFriendsSource({
      viewer: owner,
      mediaTitles: { fargo: 'Fargo · The Castle', rookie: 'The Rookie · Survive the Streets' },
      now: () => new Date(timestamp),
    });
    renderPage(<WatchWithFriendsPage source={source} viewer={owner} />);

    expect(await screen.findByText('No active groups')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Create group' }));
    const mediaInput = screen.getByRole('textbox', { name: 'Search media for the group' });
    fireEvent.change(mediaInput, { target: { value: 'Fargo' } });
    fireEvent.click(await screen.findByRole('option', { name: /Fargo/ }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Group name' }), { target: { value: 'Friday screening' } });
    const createForm = screen.getByRole('textbox', { name: 'Group name' }).closest('form');
    expect(createForm).not.toBeNull();
    fireEvent.click(within(createForm as HTMLFormElement).getByRole('button', { name: 'Create group' }));

    expect(await screen.findByRole('heading', { name: 'Friday screening' })).toBeInTheDocument();
    await screen.findByText('Live updates connected');
    fireEvent.click(screen.getByRole('button', { name: 'Ready' }));
    await waitFor(() => expect(source.snapshot('fixture-watch-group-1')?.members[0]?.state).toBe('ready'));
    fireEvent.click(screen.getByRole('button', { name: 'Play for group' }));
    await waitFor(() => expect(source.snapshot('fixture-watch-group-1')?.state).toBe('playing'));
    fireEvent.click(screen.getByRole('switch', { name: 'Enable shuffle' }));
    await waitFor(() => expect(source.snapshot('fixture-watch-group-1')?.shuffleEnabled).toBe(true));
    fireEvent.click(screen.getByRole('button', { name: 'All' }));
    await waitFor(() => expect(source.snapshot('fixture-watch-group-1')?.repeatMode).toBe('all'));

    const queueInput = screen.getByRole('textbox', { name: 'Search media to add to queue' });
    fireEvent.change(queueInput, { target: { value: 'Rookie' } });
    fireEvent.click(await screen.findByRole('option', { name: /The Rookie/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(await screen.findByText('The Rookie · Survive the Streets')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Move The Rookie · Survive the Streets up' }));

    await waitFor(() => {
      const snapshot = source.snapshot('fixture-watch-group-1');
      expect(snapshot?.queue.map((item) => item.mediaId)).toEqual(['rookie', 'fargo']);
    });

    fireEvent.click(screen.getByRole('button', { name: 'Remove The Rookie · Survive the Streets from queue' }));
    await waitFor(() => expect(source.snapshot('fixture-watch-group-1')?.queue.map((item) => item.mediaId)).toEqual(['fargo']));
  });

  it('requires membership, keeps playback host-only, and lets a participant leave', async () => {
    const viewer: WatchWithFriendsViewer = { profileId: 'guest', displayName: 'Guest Viewer', canUse: true };
    const source = new FixtureWatchWithFriendsSource({
      viewer,
      groups: [groupFor({ viewer, ownerProfileId: 'host', ownerName: 'Screening Host', includeViewer: false })],
      now: () => new Date(timestamp),
    });
    renderPage(<WatchWithFriendsPage source={source} viewer={viewer} />);

    expect(await screen.findByRole('button', { name: 'Join group' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Play for group' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Join group' }));
    expect(await screen.findByText('Playback is controlled by Screening Host')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Play for group' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'End group' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Ready' }));
    await waitFor(() => expect(source.snapshot('watch-group-1')?.members.find((member) => member.profileId === viewer.profileId)?.state).toBe('ready'));
    fireEvent.click(screen.getByRole('button', { name: 'Leave group' }));
    expect(await screen.findByRole('button', { name: 'Join group' })).toBeInTheDocument();
    expect(source.snapshot('watch-group-1')?.members.some((member) => member.profileId === viewer.profileId)).toBe(false);
  });

  it('requires confirmation before the owner ends a group', async () => {
    const source = new FixtureWatchWithFriendsSource({ viewer: owner, groups: [groupFor()], now: () => new Date(timestamp) });
    renderPage(<WatchWithFriendsPage source={source} viewer={owner} />);
    fireEvent.click(await screen.findByRole('button', { name: 'End group' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText(/live group updates will stop/i)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'End group' }));
    await waitFor(() => expect(source.snapshot('watch-group-1')).toBeUndefined());
    expect(await screen.findByText('No active groups')).toBeInTheDocument();
  });

  it('keeps a failed live connection visible until the member reconnects', async () => {
    const source = new FixtureWatchWithFriendsSource({ viewer: owner, groups: [groupFor()] });
    renderPage(<WatchWithFriendsPage source={source} viewer={owner} />);
    await screen.findByText('Live updates connected');

    act(() => source.setConnectionState('watch-group-1', 'failed', new Error('The connection timed out.')));
    expect(await screen.findByRole('alert')).toHaveTextContent('Live room updates stopped. Reconnect before continuing together.');
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(await screen.findByText('Live updates connected')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText('The connection timed out.')).not.toBeInTheDocument());
  });

  it('keeps failed creation visible and preserves the selected media for retry', async () => {
    const source = new FixtureWatchWithFriendsSource({ viewer: owner });
    vi.spyOn(source, 'createGroup').mockRejectedValue(new Error('That media item is not available to this account.'));
    renderPage(<WatchWithFriendsPage source={source} viewer={owner} />);
    await screen.findByText('No active groups');
    fireEvent.click(screen.getByRole('button', { name: 'Create group' }));
    const input = screen.getByRole('textbox', { name: 'Search media for the group' });
    fireEvent.change(input, { target: { value: 'Fargo' } });
    fireEvent.click(await screen.findByRole('option', { name: /Fargo/ }));
    const form = screen.getByRole('textbox', { name: 'Group name' }).closest('form');
    fireEvent.click(within(form as HTMLFormElement).getByRole('button', { name: 'Create group' }));
    expect(await screen.findByRole('alert')).toHaveTextContent("Portico couldn't update this group. Try again.");
    expect(screen.getByText('Fargo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Change' })).toBeInTheDocument();
  });

  it('does not load or imply group access for a viewer without permission', async () => {
    const viewer: WatchWithFriendsViewer = { profileId: 'restricted', displayName: 'Restricted', canUse: false };
    const source = new FixtureWatchWithFriendsSource({ viewer });
    const listGroups = vi.spyOn(source, 'listGroups');
    renderPage(<WatchWithFriendsPage source={source} viewer={viewer} />);
    expect(screen.getByRole('heading', { name: 'Watch With Friends is unavailable' })).toBeInTheDocument();
    expect(listGroups).not.toHaveBeenCalled();
  });
});
