import { describe, expect, it, vi } from 'vitest';
import type { HostedServicesClient, PorticoClient } from '@porticomediaserver/client-core';
import { HttpPorticoDataSource } from '../data/httpSource';
import { settingsDataSourceFor } from './SettingsRoute';

describe('SettingsRoute datasource ownership', () => {
  it('does not serialize the Hosted invitation read behind unrelated People panels', async () => {
    let releaseLibraries!: () => void;
    const libraries = new Promise<{ items: never[]; total: number }>((resolve) => {
      releaseLibraries = () => resolve({ items: [], total: 0 });
    });
    const hostedRequest = vi.fn().mockResolvedValue({ items: [] });
    const client = {
      libraries: vi.fn().mockReturnValue(libraries),
      users: vi.fn().mockResolvedValue({
        items: [{ id: 'owner', role: 'owner', authOrigin: 'portico' }],
        total: 1,
      }),
      devices: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      apiKeys: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      accountSessions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      remoteAccessStatus: vi.fn().mockResolvedValue({ settings: { serverId: 'server-runtime' } }),
      request: vi.fn().mockResolvedValue({ permissionCatalog: [] }),
    } as unknown as PorticoClient;
    const source = new HttpPorticoDataSource(client, {
      hostedClient: { request: hostedRequest } as unknown as HostedServicesClient,
      hostedServerId: 'server-runtime',
      connectionVault: {} as never,
      switchHostedProfile: vi.fn(),
    });
    expect(source.authoritativeHostedServerId).toBe('server-runtime');
    expect(typeof source.authoritativeHostedServerId).toBe('string');
    expect(source.authoritativeHostedClient).toBeDefined();
    const settings = settingsDataSourceFor({ source });

    const pending = settings!.settingsOperations('people', new AbortController().signal);
    await vi.waitFor(() => expect(hostedRequest).toHaveBeenCalledTimes(1));
    expect(client.remoteAccessStatus).not.toHaveBeenCalled();
    expect(hostedRequest).toHaveBeenCalledWith(
      '/api/account/servers/server-runtime/invites?limit=100',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
    releaseLibraries();
    await expect(pending).resolves.toMatchObject({ porticoInvites: [] });
  });

  it('rejects an asynchronous Hosted server identity at real datasource construction', () => {
    expect(() => new HttpPorticoDataSource({} as PorticoClient, {
      hostedClient: {} as HostedServicesClient,
      hostedServerId: Promise.resolve('server-runtime') as unknown as string,
      connectionVault: {} as never,
      switchHostedProfile: vi.fn(),
    })).toThrow('Portico Hosted server identity has an invalid runtime shape.');
  });

  it('rejects an asynchronous Hosted client at real datasource construction', () => {
    expect(() => new HttpPorticoDataSource({} as PorticoClient, {
      hostedClient: Promise.resolve({}) as unknown as HostedServicesClient,
      hostedServerId: 'server-runtime',
      connectionVault: {} as never,
      switchHostedProfile: vi.fn(),
    })).toThrow('Portico Hosted client has an invalid runtime shape.');
  });

  it('uses the runtime Hosted client for invitation list and create requests', async () => {
    const serverId = 'server-runtime';
    const signal = new AbortController().signal;
    const hostedRequest = vi.fn().mockImplementation(async (path: string) => {
      if (path.endsWith('/invites?limit=100')) return { items: [] };
      return { inviteUrl: 'https://web.getportico.tv/invite/test' };
    });
    const client = {
      libraries: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      users: vi.fn().mockResolvedValue({
        items: [{ id: 'owner', role: 'owner', authOrigin: 'portico' }],
        total: 1,
      }),
      devices: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      apiKeys: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      accountSessions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      remoteAccessStatus: vi.fn().mockResolvedValue({ settings: { serverId } }),
      request: vi.fn().mockResolvedValue({ permissionCatalog: [] }),
    } as unknown as PorticoClient;
    const hosted = { request: hostedRequest } as unknown as HostedServicesClient;
    const source = new HttpPorticoDataSource(client, {
      hostedClient: hosted,
      hostedServerId: serverId,
      connectionVault: {} as never,
      switchHostedProfile: vi.fn(),
    });

    const settings = settingsDataSourceFor({ source });
    expect(settings).toBeDefined();
    await settings!.settingsOperations('people', signal);
    await settings!.createPorticoMemberInvite({
      recipient: 'person@example.com',
      email: 'person@example.com',
      deliveryMode: 'link',
      permissionTemplate: {
        permissions: { playMedia: true, viewLiveTV: true, playLiveTV: true },
      },
    }, signal);

    expect(hostedRequest).toHaveBeenNthCalledWith(
      1,
      `/api/account/servers/${serverId}/invites?limit=100`,
      { signal },
    );
    expect(client.remoteAccessStatus).not.toHaveBeenCalled();
    expect(hostedRequest).toHaveBeenNthCalledWith(
      2,
      `/api/account/servers/${serverId}/invites`,
      expect.objectContaining({
        method: 'POST',
        signal,
        body: expect.objectContaining({
          recipient: 'person@example.com',
          email: 'person@example.com',
          deliveryMode: 'link',
          permissionTemplate: {
            permissions: { playMedia: true, viewLiveTV: true, playLiveTV: true },
          },
        }),
      }),
    );
    expect(hostedRequest.mock.calls[1]?.[1]?.body).not.toHaveProperty('role');
  });
});
