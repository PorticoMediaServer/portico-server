import { describe, expect, it, vi } from 'vitest';
import { ApiError, type HostedServicesClient, type PorticoClient } from '@porticomediaserver/client-core';
import { HttpSettingsDataSource } from './HttpSettingsDataSource';

describe('HttpSettingsDataSource remote unclaim', () => {
  it('finishes local unclaim when Hosted already committed deletion', async () => {
    const remoteAccessStatus = vi.fn().mockResolvedValue({ settings: { serverId: 'srv-deleted' } });
    const request = vi.fn().mockResolvedValue({ settings: { serverId: '' } });
    const hostedRequest = vi.fn().mockRejectedValue(new ApiError(404, 'server_not_found', 'Server not found.'));
    const source = new HttpSettingsDataSource(
      { remoteAccessStatus, request } as unknown as PorticoClient,
      { request: hostedRequest } as unknown as HostedServicesClient,
    );

    const result = await source.unclaimRemoteAccess(new AbortController().signal);

    expect(hostedRequest).toHaveBeenCalledWith('/api/account/servers/srv-deleted', expect.objectContaining({ method: 'DELETE' }));
    expect(request).toHaveBeenCalledWith('/api/remote-access/unclaim', expect.objectContaining({ method: 'POST' }));
    expect(result.settings.serverId).toBe('');
  });

  it('does not clear the local claim for a non-terminal Hosted failure', async () => {
    const remoteAccessStatus = vi.fn().mockResolvedValue({ settings: { serverId: 'srv-live' } });
    const request = vi.fn();
    const failure = new ApiError(503, 'hosted_unavailable', 'Hosted unavailable.');
    const source = new HttpSettingsDataSource(
      { remoteAccessStatus, request } as unknown as PorticoClient,
      { request: vi.fn().mockRejectedValue(failure) } as unknown as HostedServicesClient,
    );

    await expect(source.unclaimRemoteAccess(new AbortController().signal)).rejects.toBe(failure);
    expect(request).not.toHaveBeenCalled();
  });
});

describe('HttpSettingsDataSource operational partial state', () => {
  it('keeps healthy panels when one operational read fails', async () => {
    const internalMessage = 'pq: relation account_secrets does not exist';
    const libraries = vi.fn().mockRejectedValue(new Error(internalMessage));
    const source = new HttpSettingsDataSource({ libraries } as unknown as PorticoClient);

    const snapshot = await source.settingsOperations('media', new AbortController().signal);

    expect(snapshot.libraries).toEqual([]);
    expect(snapshot.failures?.libraries).toBeTruthy();
    expect(snapshot.failures?.libraries).not.toContain(internalMessage);
    expect(snapshot.failures?.libraries).toBe("Portico couldn't load this information. Try again.");
    expect(snapshot.failures?.release).toBeUndefined();
    expect(libraries).toHaveBeenCalled();
  });

  it('projects arbitrary status failures through reviewed product language', async () => {
    const internalMessage = 'dial tcp 10.0.0.8:5432: connection refused';
    const failure = vi.fn().mockRejectedValue(new Error(internalMessage));
    const source = new HttpSettingsDataSource({
      dashboardActivity: failure,
      dashboard: vi.fn().mockResolvedValue({}),
      systemStorage: vi.fn().mockResolvedValue({}),
      remoteAccessStatus: vi.fn().mockResolvedValue({}),
      activity: vi.fn().mockResolvedValue({ items: [] }),
    } as unknown as PorticoClient);

    const snapshot = await source.settingsStatus(new AbortController().signal);

    expect(snapshot.failures?.activity).toBe("Portico couldn't load this information. Try again.");
    expect(snapshot.failures?.activity).not.toContain(internalMessage);
  });

  it('does not expose Hosted invitation diagnostics in People settings', async () => {
    const internalMessage = 'upstream redis password=super-secret';
    const client = {
      libraries: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      users: vi.fn().mockResolvedValue({ items: [{ id: 'owner', role: 'owner', authOrigin: 'portico' }], total: 1 }),
      devices: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      apiKeys: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      accountSessions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      remoteAccessStatus: vi.fn().mockResolvedValue({ settings: { serverId: 'server-1' } }),
      request: vi.fn().mockResolvedValue({ permissionCatalog: [] }),
    };
    const hosted = { request: vi.fn().mockRejectedValue(new Error(internalMessage)) };
    const source = new HttpSettingsDataSource(
      client as unknown as PorticoClient,
      hosted as unknown as HostedServicesClient,
    );

    const snapshot = await source.settingsOperations('people', new AbortController().signal);

    expect(snapshot.failures?.porticoInvites).toBe("Portico couldn't load this information. Try again.");
    expect(snapshot.failures?.porticoInvites).not.toContain(internalMessage);
  });

  it('loads the libraries and permission catalog required by People editors', async () => {
    const client = {
      libraries: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      users: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      devices: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      apiKeys: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      accountSessions: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      request: vi.fn().mockResolvedValue({ permissionCatalog: [] }),
    };
    const source = new HttpSettingsDataSource(client as unknown as PorticoClient);

    const snapshot = await source.settingsOperations('people', new AbortController().signal);

    expect(client.libraries).toHaveBeenCalled();
    expect(client.request).toHaveBeenCalledWith('/api/system/capabilities', expect.objectContaining({ signal: expect.any(AbortSignal) }));
    expect(snapshot.failures).toBeUndefined();
  });
});
