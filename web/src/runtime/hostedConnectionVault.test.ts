import type { TrustedServerConnectionRecord } from '@porticomediaserver/client-core';
import { describe, expect, it } from 'vitest';
import {
  createBrowserHostedConnectionVault,
  createBrowserHostedConnectionVaultWithDurableMetadata,
  createHostedConnectionVault,
  HOSTED_BROWSER_SESSION_TTL_MS,
  type HostedConnectionVaultStorage,
} from './hostedConnectionVault';

function memoryStorage(): HostedConnectionVaultStorage {
  const stores = new Map<string, Map<string, unknown>>();
  const store = (name: string) => {
    const value = stores.get(name) ?? new Map<string, unknown>();
    stores.set(name, value);
    return value;
  };
  const key = (value: IDBValidKey) => JSON.stringify(value);
  return {
    get: async <T,>(name: string, id: IDBValidKey) => store(name).get(key(id)) as T | undefined,
    put: async <T,>(name: string, value: T) => {
      const record = value as { key?: IDBValidKey; accountId?: IDBValidKey };
      store(name).set(key(record.key ?? record.accountId ?? ''), value);
    },
    delete: async (name, id) => { store(name).delete(key(id)); },
    getAll: async <T,>(name: string) => [...store(name).values()] as T[],
    compareAndSwapConnection: async (id, expectedVersion, value) => {
      const current = store('connections').get(key(id)) as { metadata?: { mutationVersion?: number }; mutationVersion?: number } | undefined;
      if ((current?.metadata?.mutationVersion ?? current?.mutationVersion ?? 0) !== expectedVersion) return false;
      store('connections').set(key(id), value);
      return true;
    },
    replaceConnectionWithTombstone: async (id, value) => { store('connections').set(key(id), value); },
  };
}

function connection(accountId: string, serverId: string, connectedAt: string): TrustedServerConnectionRecord {
  return {
    schemaVersion: 3,
    accountId,
    serverId,
    profileId: 'profile-1',
    serverName: serverId === 'server-1' ? 'Family Media' : 'Cinema',
    serverPublicKey: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
    serverPublicKeyFingerprint: `sha256:${serverId}`,
    currentRoute: { url: `https://${serverId}.direct.getportico.tv`, type: 'public_direct', verifiedAt: connectedAt },
    session: {
      serverId,
      serverName: serverId === 'server-1' ? 'Family Media' : 'Cinema',
      apiBaseUrl: `https://${serverId}.direct.getportico.tv`,
      accessToken: `access-${serverId}`,
      refreshToken: `refresh-${serverId}`,
      serverPublicKey: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
      serverPublicKeyFingerprint: `sha256:${serverId}`,
    },
    lastSuccessfulConnectionAt: connectedAt,
    mutationVersion: 1,
  };
}

describe('Hosted Web trusted connection vault', () => {
  it('reports tab-lifetime storage honestly when IndexedDB is unavailable', () => {
    expect(createBrowserHostedConnectionVault(undefined).durability()).toBe('memory-only');
  });

  it('rejects a browser composition backed by an adapter that may persist reusable credentials', () => {
    const unsafe = createHostedConnectionVault(memoryStorage());
    expect(unsafe.persistencePolicy).toBe('saved-session');
    expect(() => createBrowserHostedConnectionVaultWithDurableMetadata(unsafe)).toThrow(/must reject reusable server credentials/i);
  });
  it('keeps account identity separate from multiple account-scoped server sessions', async () => {
    const vault = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'));
    await vault.rememberAccount({ accountId: 'account-1', displayName: 'Justin', email: 'justin@example.test' });
    await vault.save(connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z'));
    await vault.save(connection('account-1', 'server-2', '2026-07-14T11:30:00.000Z'));
    await vault.save(connection('account-2', 'server-private', '2026-07-14T11:45:00.000Z'));

    expect(await vault.activeAccount()).toMatchObject({ accountId: 'account-1', displayName: 'Justin' });
    expect((await vault.list('account-1')).map((record) => record.serverId)).toEqual(['server-2', 'server-1']);
    expect(await vault.load('account-1', 'server-private')).toBeUndefined();
  });

  it('uses mutation versions for compare-and-swap without overwriting a concurrent value', async () => {
    const vault = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'));
    const first = { ...connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z'), mutationVersion: 1 };
    const stale = { ...connection('account-1', 'server-1', '2026-07-14T11:01:00.000Z'), mutationVersion: 2 };

    await expect(vault.compareAndSwap(0, first)).resolves.toBe(true);
    await expect(vault.compareAndSwap(0, stale)).resolves.toBe(false);
    await expect(vault.load('account-1', 'server-1')).resolves.toEqual(first);
  });

  it('atomically replaces a connection with a durable removal tombstone', async () => {
    const vault = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'));
    const record = { ...connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z'), mutationVersion: 1 };
    const tombstone = {
      schemaVersion: 1 as const,
      accountId: 'account-1',
      serverId: 'server-1',
      mutationVersion: 2,
      removedAt: '2026-07-14T12:00:00.000Z',
    };
    await vault.save(record);

    await vault.removeWithTombstone(tombstone);

    await expect(vault.load('account-1', 'server-1')).resolves.toBeUndefined();
    await expect(vault.list('account-1')).resolves.toEqual([]);
    await expect(vault.loadRemovalTombstone('account-1', 'server-1')).resolves.toEqual(tombstone);
    await expect(vault.compareAndSwap(1, { ...record, mutationVersion: 2 })).resolves.toBe(false);
  });

  it('removes only the explicitly signed-out account and leaves other account records intact', async () => {
    const vault = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'));
    await vault.rememberAccount({ accountId: 'account-1', displayName: 'Justin', email: 'justin@example.test' });
    await vault.save(connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z'));
    await vault.save(connection('account-2', 'server-2', '2026-07-14T11:00:00.000Z'));

    await vault.forgetAccount('account-1');
    expect(await vault.activeAccount()).toBeUndefined();
    expect(await vault.list('account-1')).toEqual([]);
    expect(await vault.load('account-2', 'server-2')).toBeDefined();
  });

  it('expires inactive browser state after six months without treating a network failure as revocation', async () => {
    const storage = memoryStorage();
    let now = new Date('2026-01-01T00:00:00.000Z');
    const vault = createHostedConnectionVault(storage, () => now);
    await vault.rememberAccount({ accountId: 'account-1', displayName: 'Justin', email: 'justin@example.test' });
    await vault.save(connection('account-1', 'server-1', now.toISOString()));

    now = new Date(now.getTime() + HOSTED_BROWSER_SESSION_TTL_MS - 1);
    expect(await vault.activeAccount()).toBeDefined();
    expect(await vault.load('account-1', 'server-1')).toBeDefined();

    now = new Date(now.getTime() + 2);
    expect(await vault.activeAccount()).toBeUndefined();
    expect(await vault.load('account-1', 'server-1')).toBeUndefined();
  });

  it('keeps reusable credentials in process memory only and cannot restore them in a fresh browser vault after deletion failure', async () => {
    const durableStorage = memoryStorage();
    const durable = createHostedConnectionVault(durableStorage, () => new Date('2026-07-14T12:00:00.000Z'), { persistCredentials: false });
    expect(durable.durability()).toBe('durable');
    expect(durable.persistencePolicy).toBe('reauthorize-on-start');
    const failingDeletion = { ...durable, forgetAccount: async () => { throw new Error('durable deletion failed'); } };
    const firstProcess = createBrowserHostedConnectionVaultWithDurableMetadata(failingDeletion);
    expect(firstProcess.durability()).toBe('durable');
    expect(firstProcess.persistencePolicy).toBe('reauthorize-on-start');
    const record = connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z');
    await firstProcess.save(record);
    expect(await firstProcess.load('account-1', 'server-1')).toEqual(record);
    await expect(firstProcess.forgetAccount('account-1')).rejects.toThrow('browser metadata');

    const freshProcess = createBrowserHostedConnectionVaultWithDurableMetadata(
      createHostedConnectionVault(durableStorage, () => new Date('2026-07-14T12:01:00.000Z'), { persistCredentials: false }),
    );
    expect(await freshProcess.load('account-1', 'server-1')).toBeUndefined();
    expect(await freshProcess.list('account-1')).toEqual([]);
    expect(JSON.stringify(await durableStorage.getAll('connections'))).not.toContain('refresh-server-1');
    expect(JSON.stringify(await durableStorage.getAll('connections'))).not.toContain('access-server-1');
  });

  it('keeps a verified process-memory credential live when auxiliary metadata hits quota', async () => {
    const metadata = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'), { persistCredentials: false });
    const failingMetadata = { ...metadata, save: async () => { throw new DOMException('Storage full', 'QuotaExceededError'); } };
    const vault = createBrowserHostedConnectionVaultWithDurableMetadata(failingMetadata);
    const record = connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z');

    await expect(vault.save(record)).resolves.toBeUndefined();
    expect(await vault.load('account-1', 'server-1')).toEqual(record);
    expect(vault.durability()).toBe('memory-only');
  });

  it('retains the account-scoped cleanup barrier when account records are forgotten', async () => {
    const vault = createHostedConnectionVault(memoryStorage());
    await vault.markCleanupPending('account-1');
    await vault.forgetAccount('account-1');
    expect(await vault.cleanupPendingAccounts()).toEqual(['account-1']);
    await vault.clearCleanupPending('account-1');
    expect(await vault.cleanupPendingAccounts()).toEqual([]);
  });

  it('retains independent cleanup barriers when cleanup fails for multiple accounts', async () => {
    const vault = createHostedConnectionVault(memoryStorage());
    await vault.markCleanupPending('account-1');
    await vault.markCleanupPending('account-2');
    const cleanup = async (accountId: string) => {
      await vault.forgetAccount(accountId);
      throw new Error(`cleanup verification failed for ${accountId}`);
    };

    const results = await Promise.allSettled([cleanup('account-1'), cleanup('account-2')]);
    expect(results.every((result) => result.status === 'rejected')).toBe(true);
    expect((await vault.cleanupPendingAccounts()).sort()).toEqual(['account-1', 'account-2']);
  });

  it('scopes automatic profile trust to the account, server, and profile rather than installation metadata', async () => {
    const vault = createHostedConnectionVault(memoryStorage(), () => new Date('2026-07-14T12:00:00.000Z'));
    await vault.saveAutomaticProfileTrust({
      version: 'v1',
      purpose: 'automatic-profile-selection',
      token: 'profile-trust-token',
      authority: 'hosted',
      accountId: 'account-1',
      serverId: 'server-1',
      profileId: 'profile-1',
      pinRevision: 2,
      installationId: 'legacy-installation-metadata',
      expiresAt: '2026-07-15T12:00:00.000Z',
    });

    await expect(vault.automaticProfileTrust({
      authority: 'hosted',
      accountId: 'account-1',
      serverId: 'server-1',
    }, 'profile-1')).resolves.toMatchObject({ token: 'profile-trust-token' });
  });

  it('attempts account, active metadata, and every discovered connection deletion before reporting failure', async () => {
    const base = memoryStorage();
    const deletions: { store: string; key: IDBValidKey }[] = [];
    const storage: HostedConnectionVaultStorage = {
      ...base,
      delete: async (store, key) => {
        deletions.push({ store, key });
        if (store === 'accounts') throw new Error('account delete failed');
        await base.delete(store, key);
      },
    };
    const vault = createHostedConnectionVault(storage);
    await vault.rememberAccount({ accountId: 'account-1', displayName: 'Justin', email: 'justin@example.test' });
    await vault.save(connection('account-1', 'server-1', '2026-07-14T11:00:00.000Z'));
    await vault.save(connection('account-1', 'server-2', '2026-07-14T11:01:00.000Z'));

    await expect(vault.forgetAccount('account-1')).rejects.toBeInstanceOf(AggregateError);
    expect(deletions).toEqual(expect.arrayContaining([
      { store: 'accounts', key: 'account-1' },
      { store: 'metadata', key: 'active-account' },
      { store: 'connections', key: 'account-1\u0000server-1' },
      { store: 'connections', key: 'account-1\u0000server-2' },
    ]));
  });
});
