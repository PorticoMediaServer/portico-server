import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { ViewerMessagesSettings } from './ViewerMessagesSettings';

const recipients = {
  profiles: [{
    authority: 'local' as const,
    audience: 'profile' as const,
    accountId: 'local-account',
    profileId: 'local-child',
    accountName: 'Rivera household',
    profileName: 'Kids',
  }],
  accountAdmins: [{
    authority: 'hosted' as const,
    audience: 'account-admin' as const,
    accountId: 'hosted-account',
    accountName: 'Sam Rivera',
  }],
};

function feedback(id: string, status: 'new' | 'read', message: string) {
  return {
    id,
    reporter: { authority: 'local' as const, accountId: 'local-account', accountName: `${id} account` },
    kind: 'playback' as const,
    category: 'wont-play',
    status,
    message,
    diagnostics: { deviceClass: 'web' as const, platform: 'macOS', appVersion: '0.1.0-test', occurredAt: '2026-08-06T12:00:00.000Z' },
    duplicateCount: 0,
    submittedAt: '2026-08-06T12:00:00.000Z',
    updatedAt: '2026-08-06T12:00:00.000Z',
    revision: 1,
  };
}

function feedbackPage(items: ReturnType<typeof feedback>[]) {
  return {
    items,
    pageInfo: { hasMore: false, nextCursor: null },
    statusCounts: { new: 1, read: 1, resolved: 0, dismissed: 0 },
  };
}

describe('ViewerMessagesSettings recipient selector', () => {
  it('shows a normalized failure and retries the dedicated recipient directory', async () => {
    const source = new FixturePorticoDataSource();
    const loadRecipients = vi.spyOn(source, 'ownerViewerNotificationRecipients')
      .mockRejectedValueOnce(new Error('raw database path must not be shown'))
      .mockResolvedValueOnce(recipients);

    render(<ViewerMessagesSettings source={source} />);

    const failure = await screen.findByRole('alert');
    expect(within(failure).getByRole('button', { name: 'Try again' })).toBeInTheDocument();
    expect(screen.queryByText(/raw database path/i)).not.toBeInTheDocument();
    fireEvent.click(within(failure).getByRole('button', { name: 'Try again' }));

    expect(await screen.findByRole('option', { name: 'Kids · Rivera household' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Audience'), { target: { value: 'account-admin' } });
    expect(screen.getByRole('option', { name: 'Sam Rivera · Portico Account' })).toBeInTheDocument();
    expect(loadRecipients).toHaveBeenCalledTimes(2);
  });

  it('sends one generated-union recipient identity without mixed fields', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'ownerViewerNotificationRecipients').mockResolvedValue(recipients);
    const send = vi.spyOn(source, 'createOwnerViewerNotice').mockResolvedValue();

    render(<ViewerMessagesSettings source={source} />);
    const profile = await screen.findByLabelText('Profile');
    fireEvent.change(profile, { target: { value: 'local-child' } });
    fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'Library maintenance tonight.' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send notice' }));

    await waitFor(() => expect(send).toHaveBeenCalledWith({
      audience: 'profile',
      profileId: 'local-child',
      message: 'Library maintenance tonight.',
      severity: 'informational',
    }, expect.any(AbortSignal)));
  });

  it('reconciles the selected feedback detail when its status filter changes', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'ownerViewerNotificationRecipients').mockResolvedValue(recipients);
    vi.spyOn(source, 'ownerViewerFeedback').mockImplementation(async (status) => (
      status === 'read'
        ? feedbackPage([feedback('read-viewer', 'read', 'Read-filter detail')])
        : feedbackPage([feedback('new-viewer', 'new', 'New-filter detail')])
    ));

    render(<ViewerMessagesSettings source={source} />);

    expect(await screen.findByText('New-filter detail')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: /read/i }));

    expect(await screen.findByText('Read-filter detail')).toBeInTheDocument();
    expect(screen.queryByText('New-filter detail')).not.toBeInTheDocument();
  });
});
