import type { WatchWithFriendsGroup } from '@porticomediaserver/client-core';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { GroupWorkspace, type GroupWorkspaceActions } from './GroupWorkspace';
import type { WatchWithFriendsViewer } from './watchWithFriendsSource';

const viewer: WatchWithFriendsViewer = { profileId: 'owner', displayName: 'Portico Review', canUse: true };

const group: WatchWithFriendsGroup = {
  id: 'group-1',
  name: 'Friday screening',
  ownerProfileId: viewer.profileId,
  ownerName: viewer.displayName,
  mediaId: 'fargo',
  mediaTitle: 'Fargo',
  currentEntryId: 'entry-fargo',
  state: 'paused',
  positionSeconds: 0,
  positionUpdatedAt: '2026-07-17T12:00:00.000Z',
  serverTime: '2026-07-17T12:00:00.000Z',
  playbackRate: 1,
  revision: 1,
  playbackRevision: 1,
  reconnectGeneration: 0,
  permissions: { isHost: true, canControl: true, canManageQueue: true },
  shuffleEnabled: false,
  repeatMode: 'none',
  command: {
    id: 'command-1',
    action: 'pause',
    mediaId: 'fargo',
    positionSeconds: 0,
    issuedAt: '2026-07-17T12:00:00.000Z',
    issuedByProfileId: viewer.profileId,
  },
  members: [{
    profileId: viewer.profileId,
    displayName: viewer.displayName,
    state: 'ready',
    positionSeconds: 0,
    joinedAt: '2026-07-17T12:00:00.000Z',
    lastSeenAt: '2026-07-17T12:00:00.000Z',
  }],
  queue: [{
    entryId: 'entry-fargo',
    mediaId: 'fargo',
    mediaTitle: 'Fargo',
    sortOrder: 0,
    addedByProfileId: viewer.profileId,
    addedAt: '2026-07-17T12:00:00.000Z',
    unavailable: false,
  }],
  createdAt: '2026-07-17T12:00:00.000Z',
  updatedAt: '2026-07-17T12:00:00.000Z',
};

const actions: GroupWorkspaceActions = {
  join: vi.fn(async () => true),
  leave: vi.fn(async () => true),
  end: vi.fn(async () => true),
  retryEvents: vi.fn(),
  refresh: vi.fn(async () => true),
  updateMember: vi.fn(async () => true),
  updatePlayback: vi.fn(async () => true),
  updateSettings: vi.fn(async () => true),
  addQueueItem: vi.fn(async () => true),
  reorderQueue: vi.fn(async () => true),
  removeQueueItem: vi.fn(async () => true),
};

function Workspace({ busy = '' }: { busy?: string }) {
  return <DataProvider source={new FixturePorticoDataSource()}><GroupWorkspace group={group} viewer={viewer} connection="live" connectionError="" busy={busy} actions={actions} /></DataProvider>;
}

describe('GroupWorkspace end confirmation', () => {
  it('uses the shared modal lifecycle and prevents dismissal while ending', async () => {
    const rendered = render(<Workspace />);
    const trigger = screen.getByRole('button', { name: 'End group' });
    trigger.focus();
    fireEvent.click(trigger);

    expect(screen.getByRole('dialog', { name: 'End “Friday screening”?' })).toBeInTheDocument();
    expect(document.body.style.overflow).toBe('hidden');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus());

    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(document.body.style.overflow).toBe('');

    fireEvent.click(trigger);
    const backdrop = screen.getByRole('dialog').parentElement;
    expect(backdrop).not.toBeNull();
    fireEvent.pointerDown(backdrop as HTMLElement);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());

    fireEvent.click(trigger);
    rendered.rerender(<Workspace busy="end:group-1" />);
    fireEvent.keyDown(window, { key: 'Escape' });
    fireEvent.pointerDown(screen.getByRole('dialog').parentElement as HTMLElement);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  });
});
