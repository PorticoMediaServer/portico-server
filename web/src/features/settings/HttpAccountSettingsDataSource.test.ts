import type { HostedServicesClient, PorticoAccountUser, PorticoClient, User, UserPatchRequest } from '@porticomediaserver/client-core';
import { describe, expect, it, vi } from 'vitest';
import { HttpSettingsDataSource } from './HttpSettingsDataSource';

const hostedUser: PorticoAccountUser = {
  id: 'portico-user',
  username: 'justinehler',
  email: 'justin@example.test',
  profileImageUrl: '/api/account/me/image',
  createdAt: '2026-07-11T01:00:00Z',
  preferences: {
    locale: 'en-CA',
    timeZone: 'America/Halifax',
    dateFormat: 'medium',
    hourCycle: 'auto',
    audioLanguage: 'en',
    subtitleLanguage: 'en',
    sidebarOrder: [],
    musicPlayback: { autoplayDefault: true, crossfadeSeconds: 0, gapless: true, normalizationMode: 'off', repeatDefault: 'none', shuffleDefault: false },
    playbackProgress: { playedThresholdPercent: 95, startedThresholdPercent: 5 },
    privacy: { includeInWatchWithFriends: true, pauseWatchHistory: false, showActivityToMembers: true },
  },
};

describe('HttpSettingsDataSource account contracts', () => {
  it('updates the hosted source of truth and synchronizes the connected server profile', async () => {
    const request = vi.fn().mockResolvedValue({ displayName: hostedUser.username, email: hostedUser.email, profileImageUrl: 'https://api.getportico.tv/api/account/me/image' });
    const hostedRequest = vi.fn().mockResolvedValue({ user: hostedUser });
    const main = { request } as unknown as PorticoClient;
    const hosted = { request: hostedRequest, hostedApiUrl: (path: string) => `https://api.getportico.tv${path}` } as unknown as HostedServicesClient;
    const source = new HttpSettingsDataSource(main, hosted);
    const signal = new AbortController().signal;

    const result = await source.updateAccountIdentity('portico', { displayName: 'justinehler', email: 'justin@example.test' }, signal);

    expect(hostedRequest).toHaveBeenCalledWith('/api/account/me', expect.objectContaining({ method: 'PATCH', body: { username: 'justinehler', email: 'justin@example.test' }, signal }));
    expect(request).toHaveBeenCalledWith('/api/account/profile', expect.objectContaining({ method: 'PATCH', body: expect.objectContaining({ profileImageUrl: 'https://api.getportico.tv/api/account/me/image' }), signal }));
    expect(result.profileImageUrl).toBe('https://api.getportico.tv/api/account/me/image');
  });

  it('preserves hosted success and returns a warning when server profile synchronization fails', async () => {
    const main = { request: vi.fn().mockRejectedValue(new Error('Server is offline.')) } as unknown as PorticoClient;
    const hosted = { request: vi.fn().mockResolvedValue({ user: hostedUser }), hostedApiUrl: (path: string) => `https://api.getportico.tv${path}` } as unknown as HostedServicesClient;
    const source = new HttpSettingsDataSource(main, hosted);

    const result = await source.updateAccountIdentity('portico', { displayName: hostedUser.username!, email: hostedUser.email }, new AbortController().signal);
    expect(result.serverSyncWarning).toBe('Your Portico Account was updated, but this server could not receive the latest profile yet. Try reconnecting later.');
  });

  it('supports account-only updates while the selected media server is unavailable', async () => {
    const request = vi.fn().mockRejectedValue(new Error('Server is offline.'));
    const hostedRequest = vi.fn().mockResolvedValue({ user: hostedUser });
    const source = new HttpSettingsDataSource(
      { request } as unknown as PorticoClient,
      { request: hostedRequest, hostedApiUrl: (path: string) => `https://api.getportico.tv${path}` } as unknown as HostedServicesClient,
      { syncHostedIdentityToServer: false },
    );

    const result = await source.updateAccountIdentity('portico', { displayName: hostedUser.username!, email: hostedUser.email }, new AbortController().signal);

    expect(hostedRequest).toHaveBeenCalled();
    expect(request).not.toHaveBeenCalled();
    expect(result.serverSyncWarning).toBeUndefined();
  });

  it('uses the authoritative hosted password and MFA routes', async () => {
    const hostedRequest = vi.fn()
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ enabled: false, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: 0 })
      .mockResolvedValueOnce({ secret: 'SETUPKEY', otpauthUrl: 'otpauth://totp/Portico:test?secret=SETUPKEY', enrollmentToken: 'short-lived-token' })
      .mockResolvedValueOnce({ enabled: true, recoveryCodes: ['one', 'two'] })
      .mockResolvedValueOnce({ ok: true });
    const source = new HttpSettingsDataSource({} as PorticoClient, { request: hostedRequest } as unknown as HostedServicesClient);
    const signal = new AbortController().signal;

    await source.changePorticoPassword({ currentPassword: 'current', newPassword: 'a-long-new-password' }, signal);
    expect(await source.porticoMFAStatus(signal)).toEqual({ enabled: false, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: 0 });
    expect(await source.startPorticoMFA('current', signal)).toEqual({ enrollmentToken: 'short-lived-token', secret: 'SETUPKEY', otpauthUrl: 'otpauth://totp/Portico:test?secret=SETUPKEY' });
    expect(await source.enablePorticoMFA({ code: '123456', enrollmentToken: 'short-lived-token' }, signal)).toEqual({ enabled: true, recoveryCodes: ['one', 'two'] });
    await source.disablePorticoMFA({ password: 'current', code: '654321' }, signal);

    expect(hostedRequest.mock.calls.map(([path]) => path)).toEqual([
      '/api/account/me/password',
      '/api/auth/mfa/status',
      '/api/auth/mfa/setup',
      '/api/auth/mfa/enable',
      '/api/auth/mfa/disable',
    ]);
    expect(hostedRequest).toHaveBeenNthCalledWith(3, '/api/auth/mfa/setup', expect.objectContaining({ body: { password: 'current' }, method: 'POST', signal }));
    expect(hostedRequest).toHaveBeenNthCalledWith(4, '/api/auth/mfa/enable', expect.objectContaining({ body: { code: '123456', enrollmentToken: 'short-lived-token' }, method: 'POST', signal }));
  });

  it('uses the password-confirmed Cloud account deletion contract', async () => {
    const deleteAccount = vi.fn().mockResolvedValue({ ok: true, deletedAt: '2026-07-16T20:00:00Z' });
    const source = new HttpSettingsDataSource({} as PorticoClient, { deleteAccount } as unknown as HostedServicesClient);
    const signal = new AbortController().signal;

    await source.deletePorticoAccount({ password: 'current-password', mfaCode: '123456' }, signal);

    expect(deleteAccount).toHaveBeenCalledWith({ password: 'current-password', mfaCode: '123456' }, { signal });
  });

  it('uses the main-server password contract for local credentials', async () => {
    const changeAccountPassword = vi.fn().mockResolvedValue({ ok: true });
    const source = new HttpSettingsDataSource({ changeAccountPassword } as unknown as PorticoClient, { request: vi.fn() } as unknown as HostedServicesClient);
    const signal = new AbortController().signal;

    await source.changeLocalPassword({ currentPassword: 'current-password', newPassword: 'new-password-123' }, signal);

    expect(changeAccountPassword).toHaveBeenCalledWith({ currentPassword: 'current-password', newPassword: 'new-password-123' }, { signal });
  });

  it('lists and revokes account-wide Cloud devices for Portico Account settings', async () => {
    const devices = vi.fn().mockResolvedValue({
      items: [
        { id: 'device-active', userId: 'portico-user', name: 'Living Room TV', platform: 'tvOS', lastSeenAt: '2026-07-16T20:00:00Z' },
        { id: 'device-revoked', userId: 'portico-user', name: 'Old Phone', platform: 'iOS', lastSeenAt: '2026-07-01T20:00:00Z', revokedAt: '2026-07-02T20:00:00Z' },
      ],
      pageInfo: { nextCursor: null, hasMore: false, total: 2 },
    });
    const revokeDevice = vi.fn().mockResolvedValue({ ok: true });
    const source = new HttpSettingsDataSource({} as PorticoClient, { devices, revokeDevice } as unknown as HostedServicesClient);
    const signal = new AbortController().signal;

    await expect(source.signedInDevices('portico', signal)).resolves.toEqual([
      expect.objectContaining({ id: 'device-active', authority: 'portico', name: 'Living Room TV', canRevoke: true }),
    ]);
    await source.revokeSignedInDevice('portico', 'device-active', signal);

    expect(devices).toHaveBeenCalledWith({ limit: 100, count: 'exact' });
    expect(revokeDevice).toHaveBeenCalledWith('device-active');
  });

  it('preserves server-local session management for Local Auth settings', async () => {
    const accountSessions = vi.fn().mockResolvedValue({ items: [{
      id: 'local-session', app: 'Portico Web', authProvider: 'local', canRevoke: true, current: false,
      deviceName: 'Kitchen Mac', platform: 'macOS', trusted: true, createdAt: '2026-07-10T20:00:00Z',
      lastSeenAt: '2026-07-16T20:00:00Z', expiresAt: '2026-08-10T20:00:00Z',
    }] });
    const request = vi.fn().mockResolvedValue({ ok: true });
    const source = new HttpSettingsDataSource({ accountSessions, request } as unknown as PorticoClient, {} as HostedServicesClient);
    const signal = new AbortController().signal;

    await expect(source.signedInDevices('local', signal)).resolves.toEqual([
      expect.objectContaining({ id: 'local-session', authority: 'local', name: 'Kitchen Mac', current: false }),
    ]);
    await source.revokeSignedInDevice('local', 'local-session', signal);

    expect(accountSessions).toHaveBeenCalledWith({ signal });
    expect(request).toHaveBeenCalledWith('/api/account/sessions/local-session', { method: 'DELETE', signal });
  });

  it('routes Portico Account access changes through Hosted authority and then synchronizes local policy', async () => {
    const request = vi.fn()
      .mockResolvedValueOnce({ remoteAccess: {} })
      .mockResolvedValueOnce({ ok: true });
    const remoteAccessStatus = vi.fn().mockResolvedValue({ settings: { serverId: 'server-1' } });
    const hostedRequest = vi.fn().mockResolvedValue({ ok: true });
    const source = new HttpSettingsDataSource(
      { request, remoteAccessStatus } as unknown as PorticoClient,
      { request: hostedRequest } as unknown as HostedServicesClient,
    );
    const member = {
      id: 'local-profile',
      porticoMembershipId: 'membership-1',
      authOrigin: 'portico',
      displayName: 'Remote Member',
    } as User;
    const input = {
      displayName: 'Remote Member',
      username: '',
      email: 'member@example.test',
      libraryIds: ['movies'],
      permissions: { download: true },
      maxContentRating: 'PG-13',
    } as UserPatchRequest;
    const signal = new AbortController().signal;

    await source.updateUser(member, input, signal);
    await source.deleteUser(member, signal);

    expect(hostedRequest).toHaveBeenNthCalledWith(1, '/api/account/servers/server-1/members/membership-1', expect.objectContaining({
      method: 'PATCH',
      body: {
        permissionTemplate: { libraryIds: ['movies'], permissions: { download: true }, maxContentRating: 'PG-13' },
      },
      signal,
    }));
    expect(hostedRequest).toHaveBeenNthCalledWith(2, '/api/account/servers/server-1/members/membership-1', expect.objectContaining({ method: 'DELETE', signal }));
    expect(request).toHaveBeenNthCalledWith(1, '/api/remote-access/policy-sync', { method: 'POST', signal });
    expect(request).toHaveBeenNthCalledWith(2, '/api/remote-access/policy-sync', { method: 'POST', signal });
  });

  it('creates Portico Account invitations directly through Hosted authority', async () => {
    const request = vi.fn().mockResolvedValue({ id: 'invite-1' });
    const remoteAccessStatus = vi.fn().mockResolvedValue({ settings: { serverId: 'server-1' } });
    const hostedRequest = vi.fn().mockResolvedValue({ id: 'invite-1' });
    const source = new HttpSettingsDataSource(
      { request, remoteAccessStatus } as unknown as PorticoClient,
      { request: hostedRequest } as unknown as HostedServicesClient,
    );
    const signal = new AbortController().signal;
    const input = {
      recipient: 'member@example.test',
      email: 'member@example.test',
      role: 'user' as const,
      permissionTemplate: { libraryIds: ['movies'], permissions: { download: true } },
	  deliveryMode: 'email' as const,
    };

    await source.createPorticoMemberInvite(input, signal);

    expect(hostedRequest).toHaveBeenCalledWith('/api/account/servers/server-1/invites', expect.objectContaining({ method: 'POST', body: input, signal }));
    expect(request).not.toHaveBeenCalled();
  });

  it('keeps This Server account mutations on the local user contract', async () => {
    const request = vi.fn()
      .mockResolvedValueOnce({ id: 'local-user' })
      .mockResolvedValueOnce({ ok: true });
    const source = new HttpSettingsDataSource({ request } as unknown as PorticoClient, {} as HostedServicesClient);
    const user = { id: 'local-user', authOrigin: 'local' } as User;
    const input = { displayName: 'Local User', username: 'local', email: '', libraryIds: [], permissions: {} } as UserPatchRequest;
    const signal = new AbortController().signal;

    await source.updateUser(user, input, signal);
    await source.deleteUser(user, signal);

    expect(request).toHaveBeenNthCalledWith(1, '/api/users/local-user', expect.objectContaining({ method: 'PATCH', body: input, signal }));
    expect(request).toHaveBeenNthCalledWith(2, '/api/users/local-user', expect.objectContaining({ method: 'DELETE', signal }));
  });
});
