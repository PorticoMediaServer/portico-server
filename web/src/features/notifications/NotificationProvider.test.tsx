import type { NotificationPage, ViewerNotification } from '@porticomediaserver/client-core';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { type ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { NotificationProvider, useNotifications } from './NotificationProvider';

type PendingRead = {
  audience: 'profile' | 'account-admin';
  cursor?: string;
  signal: AbortSignal;
  resolve: (page: NotificationPage) => void;
};

const recipient: NotificationPage['recipient'] = {
  authority: 'local',
  accountId: 'fixture-owner',
  serverId: 'fixture-server',
  profileId: 'fixture-owner-profile',
  audience: 'profile',
};

function notification(id: string): ViewerNotification {
  return {
    id,
    recipient,
    kind: 'server.message',
    severity: 'informational',
    messageId: 'notification.fallback-title',
    iconId: 'status.notification',
    interpolation: {},
    actions: [],
    content: { title: id, body: `${id} body` },
    createdAt: '2026-07-17T12:00:00.000Z',
    readAt: null,
    archivedAt: null,
  };
}

function page(ids: string[], nextCursor: string | null = null): NotificationPage {
  return {
    recipient,
    items: ids.map(notification),
    unreadCount: ids.length,
    revision: 1,
    pageInfo: { hasMore: nextCursor !== null, nextCursor },
  };
}

function Probe() {
  const notifications = useNotifications();
  return <div>
    <button type="button" onClick={() => void notifications.refresh()}>Refresh</button>
    <button type="button" onClick={() => void notifications.loadMore('profile')}>Load more</button>
    <output>{notifications.profile.page?.items.map((item) => item.id).join(',') ?? notifications.profile.status}</output>
  </div>;
}

async function renderControlled(children: ReactNode) {
  const source = new FixturePorticoDataSource();
  const directory = await source.accountProfiles(new AbortController().signal);
  vi.spyOn(source, 'accountProfiles').mockResolvedValue({
    ...directory,
    profiles: directory.profiles.map((profile) => ({ ...profile, isAccountAdmin: false })),
  });
  const pending: PendingRead[] = [];
  vi.spyOn(source, 'viewerNotifications').mockImplementation((audience, cursor, signal) => new Promise((resolve) => {
    pending.push({ audience, cursor, signal, resolve });
  }));
  render(<DataProvider source={source}><NotificationProvider>{children}</NotificationProvider></DataProvider>);
  await waitFor(() => expect(pending.filter((request) => request.audience === 'profile')).toHaveLength(1));
  return { pending };
}

describe('NotificationProvider request ordering', () => {
  it('loads personal notifications when account-profile capability discovery is unavailable', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'accountProfiles').mockRejectedValue(new Error('profile directory unavailable'));
    const pending: PendingRead[] = [];
    vi.spyOn(source, 'viewerNotifications').mockImplementation((audience, cursor, signal) => new Promise((resolve) => {
      pending.push({ audience, cursor, signal, resolve });
    }));

    render(<DataProvider source={source}><NotificationProvider><Probe /></NotificationProvider></DataProvider>);
    await waitFor(() => expect(pending).toHaveLength(1));
    expect(pending[0].audience).toBe('profile');

    act(() => pending[0].resolve(page(['personal'])));
    expect(await screen.findByText('personal')).toBeInTheDocument();
  });

  it('aborts and fences an older refresh so it cannot replace newer notification state', async () => {
    const { pending } = await renderControlled(<Probe />);
    const initial = pending[0];

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(pending).toHaveLength(2));
    const newer = pending[1];
    expect(initial.signal.aborted).toBe(true);

    act(() => newer.resolve(page(['newer'])));
    expect(await screen.findByText('newer')).toBeInTheDocument();
    act(() => initial.resolve(page(['older'])));
    await act(async () => Promise.resolve());
    expect(screen.getByText('newer')).toBeInTheDocument();
    expect(screen.queryByText('older')).not.toBeInTheDocument();
  });

  it('does not append a superseded cursor page after a fresh first page wins', async () => {
    const { pending } = await renderControlled(<Probe />);
    act(() => pending[0].resolve(page(['first'], 'cursor-a')));
    expect(await screen.findByText('first')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    await waitFor(() => expect(pending).toHaveLength(2));
    const append = pending[1];
    expect(append.cursor).toBe('cursor-a');

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(pending).toHaveLength(3));
    const refresh = pending[2];
    expect(append.signal.aborted).toBe(true);
    act(() => refresh.resolve(page(['fresh'], 'cursor-b')));
    expect(await screen.findByText('fresh')).toBeInTheDocument();

    act(() => append.resolve(page(['stale-append'])));
    await act(async () => Promise.resolve());
    expect(screen.getByText('fresh')).toBeInTheDocument();
    expect(screen.queryByText(/stale-append/)).not.toBeInTheDocument();
  });
});
