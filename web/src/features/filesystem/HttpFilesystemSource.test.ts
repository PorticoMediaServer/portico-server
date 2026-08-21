import type { FilesystemBrowseResponse } from '@portico/client-core';
import { describe, expect, it, vi } from 'vitest';
import { HttpFilesystemSource } from './HttpFilesystemSource';
import type { FilesystemClient } from './filesystemSource';

const response: FilesystemBrowseResponse = {
  path: '/srv/media',
  parent: '/srv',
  roots: [{ name: 'Media', path: '/srv/media' }],
  entries: [],
};

describe('HttpFilesystemSource', () => {
  it('delegates browsing and directory creation to PorticoClient', async () => {
    const client: FilesystemClient = {
      browseFilesystem: vi.fn().mockResolvedValue(response),
      createFilesystemDirectory: vi.fn().mockResolvedValue({ ...response, path: '/srv/media/TV' }),
    };
    const source = new HttpFilesystemSource(client);
    await expect(source.browse('/srv/media')).resolves.toEqual(response);
    await source.createDirectory('/srv/media/TV');
    expect(client.browseFilesystem).toHaveBeenCalledWith('/srv/media');
    expect(client.createFilesystemDirectory).toHaveBeenCalledWith({ path: '/srv/media/TV' });
  });

  it('does not start a client request for a signal that is already aborted', async () => {
    const client: FilesystemClient = {
      browseFilesystem: vi.fn().mockResolvedValue(response),
      createFilesystemDirectory: vi.fn().mockResolvedValue(response),
    };
    const source = new HttpFilesystemSource(client);
    const controller = new AbortController();
    controller.abort();
    await expect(source.browse('/srv/media', controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(client.browseFilesystem).not.toHaveBeenCalled();
  });
});
