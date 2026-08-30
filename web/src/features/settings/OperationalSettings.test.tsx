import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FixtureSettingsDataSource } from './FixtureSettingsDataSource';
import { DLNAOperations } from './IntegrationOperations';
import { LiveTVOperations } from './LiveTVOperations';
import { PlaybackOperations } from './PlaybackOperations';
import { PeopleOperations } from './PeopleOperations';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';

const owner: SettingsViewer = {
  id: 'fixture-owner',
  displayName: 'Portico Review',
  email: 'review@portico.local',
  role: 'owner',
  serverName: 'EhlerFlix Test',
  permissions: {
    manageServer: true,
    manageLibraries: true,
    manageDVR: true,
    viewDVR: true,
  },
};

describe('operational Settings surfaces', () => {
  it('tests a Live TV source before saving and reports DVR operations', async () => {
    const source = new FixtureSettingsDataSource();
    const create = vi.spyOn(source, 'createLiveTVSource');
    render(<LiveTVOperations source={source} viewer={owner} />);

    expect(await screen.findByText('HDHomeRun Living Room')).toBeInTheDocument();
    expect(await screen.findByText('No recording conflicts')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Add source' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Source name' }), { target: { value: 'Community IPTV' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'M3U playlist URL' }), { target: { value: 'https://media.example.test/channels.m3u' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Test & add source' }));

    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Community IPTV',
      m3uUrl: 'https://media.example.test/channels.m3u',
    }), true, expect.any(AbortSignal)));
    expect(await screen.findByText('Community IPTV')).toBeInTheDocument();
  });

  it('does not request source or DVR inventory without permission', async () => {
    const source = new FixtureSettingsDataSource();
    const sources = vi.spyOn(source, 'liveTVSources');
    const dvr = vi.spyOn(source, 'dvrStatus');
    render(<LiveTVOperations source={source} viewer={{ ...owner, role: 'user', permissions: { manageDVR: false, viewDVR: false } }} />);

    expect(screen.getByText('Your account cannot inspect or manage server source credentials.')).toBeInTheDocument();
    expect(screen.getByText('Your account cannot view DVR operations.')).toBeInTheDocument();
    await waitFor(() => {
      expect(sources).not.toHaveBeenCalled();
      expect(dvr).not.toHaveBeenCalled();
    });
  });

  it('renders real playback and DLNA operational truth', async () => {
    const source = new FixtureSettingsDataSource();
    const { rerender } = render(<PlaybackOperations source={source} viewer={owner} />);
    expect(await screen.findByText('FFmpeg')).toBeInTheDocument();
    expect(screen.getByText('Optimized versions')).toBeInTheDocument();

    rerender(<DLNAOperations source={source} viewer={owner} />);
    expect(await screen.findByText('DLNA runtime')).toBeInTheDocument();
    expect(screen.getByText('Local streaming')).toBeInTheDocument();
  });

  it('projects only calm invitation states and exposes retry after proven non-delivery', async () => {
    const source = new FixtureSettingsDataSource();
    const operations = await source.settingsOperations();
    operations.porticoInvites = [{
      id: 'invite-delivery-problem', serverId: 'fixture-server', invitedEmail: 'friend@example.test', deliveryMode: 'email', role: 'user', status: 'pending', emailDeliveryStatus: 'dead-letter', permissionTemplate: { permissions: {} }, resourceLimits: {}, allowSubordinateProfiles: true, createdByUserId: 'fixture-owner', createdAt: '2026-08-18T12:00:00Z', expiresAt: new Date(Date.now() + 86_400_000).toISOString(),
    }];
    const resend = vi.spyOn(source, 'resendPorticoMemberInvite');
    render(<PeopleOperations operations={operations} source={source} onChanged={() => undefined} />);

    expect(screen.getByText('Delivery problem')).toBeInTheDocument();
    expect(screen.queryByText(/dead-letter|queued|SMTP|outbox/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry email' }));
    await waitFor(() => expect(resend).toHaveBeenCalledWith('invite-delivery-problem', expect.any(AbortSignal)));
  });

  it('submits only canonical Hosted permission keys from the visible invitation form', async () => {
    const source = new FixtureSettingsDataSource();
    const operations = await source.settingsOperations();
    operations.users[0] = { ...operations.users[0]!, authOrigin: 'portico' };
    operations.capabilities.permissionCatalog = ['PlayMedia', 'ViewLiveTV', 'PlayLiveTV'];
    const create = vi.fn<SettingsDataSource['createPorticoMemberInvite']>().mockResolvedValue({ inviteUrl: 'https://web.getportico.tv/invite/example' });
    (source as unknown as SettingsDataSource).createPorticoMemberInvite = create;
    render(<PeopleOperations operations={operations} source={source as unknown as SettingsDataSource} onChanged={() => undefined} />);

    fireEvent.click(screen.getByRole('button', { name: 'Invite account' }));
    const dialog = screen.getByRole('dialog');
    const email = within(dialog).getByRole('textbox', { name: 'Portico Account email' });
    email.focus();
    fireEvent.input(email, { target: { value: 'justin+portico-qa-ios-20260827-2005@getportico.tv' } });
    expect(email).toHaveFocus();
    expect(email).toHaveValue('justin+portico-qa-ios-20260827-2005@getportico.tv');
    fireEvent.click(within(dialog).getByRole('radio', { name: 'Create a link to share yourself' }));
    expect(email).toHaveValue('justin+portico-qa-ios-20260827-2005@getportico.tv');
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /Play media/ }));
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /View Live TV/ }));
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /Play Live TV/ }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Create link' }));

    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      recipient: 'justin+portico-qa-ios-20260827-2005@getportico.tv',
      email: 'justin+portico-qa-ios-20260827-2005@getportico.tv',
      deliveryMode: 'link',
      permissionTemplate: { permissions: { playMedia: true, viewLiveTV: true, playLiveTV: true } },
    }), expect.any(AbortSignal)));
    expect(JSON.stringify(create.mock.calls[0]?.[0])).not.toMatch(/PlayMedia|ViewLiveTV|PlayLiveTV/);
    expect(create.mock.calls[0]?.[0]).not.toHaveProperty('role');
  });
});
