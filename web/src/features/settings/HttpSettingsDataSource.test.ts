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
    const libraries = vi.fn().mockRejectedValue(new ApiError(503, 'libraries_unavailable', 'Libraries unavailable.'));
    const source = new HttpSettingsDataSource({ libraries } as unknown as PorticoClient);

    const snapshot = await source.settingsOperations('media', new AbortController().signal);

    expect(snapshot.libraries).toEqual([]);
    expect(snapshot.failures?.libraries).toBeTruthy();
    expect(snapshot.failures?.release).toBeUndefined();
    expect(libraries).toHaveBeenCalled();
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
