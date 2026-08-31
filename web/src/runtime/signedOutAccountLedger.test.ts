import { TrustedServerPublicationBlockedError } from '@porticomediaserver/client-core';
import { afterEach, describe, expect, it } from 'vitest';
import { createHostedConnectionVault, type HostedConnectionVaultStorage } from './hostedConnectionVault';
import {
  clearAccountAfterVerifiedCleanup,
  GLOBAL_SIGN_OUT_FENCE_ID,
  markAccountSignedOut,
  protectHostedConnectionVault,
  resetSignedOutAccountQuarantineForTests,
  SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX,
  signedOutAccountQuarantine,
  SignedOutAccountRestoreBlockedError,
} from './signedOutAccountLedger';

function vaultStorage(): HostedConnectionVaultStorage {
  const stores = new Map<string, Map<string, unknown>>();
  const store = (name: string) => {
    const current = stores.get(name) ?? new Map<string, unknown>();
    stores.set(name, current);
    return current;
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

afterEach(() => {
  window.localStorage.clear();
  resetSignedOutAccountQuarantineForTests();
});

describe('signed-out account cleanup ledger', () => {
  it('publishes independent versioned account tombstones without one sign-out overwriting another', () => {
    expect(markAccountSignedOut('account-1')).toBe(true);
    expect(markAccountSignedOut('account-2')).toBe(true);
    expect(JSON.parse(window.localStorage.getItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}account-1`)!)).toEqual({ version: 1, accountId: 'account-1' });
    expect(JSON.parse(window.localStorage.getItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}account-2`)!)).toEqual({ version: 1, accountId: 'account-2' });
    expect(clearAccountAfterVerifiedCleanup('account-1')).toBe(true);
    expect([...signedOutAccountQuarantine().accountIds]).toEqual(['account-2']);
  });

  it('keeps both accounts when two stale tab views publish different sign-outs', () => {
    const values = new Map<string, string>();
    const tab = () => ({
      get length() { return values.size; },
      key: (index: number) => [...values.keys()][index] ?? null,
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => { values.set(key, value); },
      removeItem: (key: string) => { values.delete(key); },
    });
    const firstTab = tab();
    const secondTab = tab();

    expect(markAccountSignedOut('account-1', firstTab)).toBe(true);
    expect(markAccountSignedOut('account-2', secondTab)).toBe(true);
    resetSignedOutAccountQuarantineForTests();

    expect([...signedOutAccountQuarantine(tab()).accountIds].sort()).toEqual(['account-1', 'account-2']);
  });

  it('denies all restore when the durable ledger is corrupt or cannot be verified after writing', () => {
    window.localStorage.setItem(`${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}account-1`, '{not-json');
    expect(signedOutAccountQuarantine().trustedForRestore).toBe(false);

    resetSignedOutAccountQuarantineForTests();
    window.localStorage.clear();
    const values = new Map<string, string>();
    const lyingStorage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (_key: string, _value: string) => undefined,
      removeItem: (key: string) => { values.delete(key); },
    };
    expect(markAccountSignedOut('account-1', lyingStorage)).toBe(false);
    expect(signedOutAccountQuarantine(lyingStorage).trustedForRestore).toBe(false);
  });

  it('enforces a durable vault cleanup barrier after a fresh in-process quarantine state', async () => {
    const raw = createHostedConnectionVault(vaultStorage());
    await raw.markCleanupPending('account-1');
    resetSignedOutAccountQuarantineForTests();
    const guarded = protectHostedConnectionVault(raw);

    await expect(guarded.list('account-1')).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
    await expect(guarded.load('account-1', 'server-1')).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
  });

  it('uses the reserved unknown-account fence to deny every account restore', async () => {
    const raw = createHostedConnectionVault(vaultStorage());
    await raw.rememberAccount({accountId: 'account-1', displayName: 'Justin', email: 'justin@example.test'});
    expect(markAccountSignedOut(GLOBAL_SIGN_OUT_FENCE_ID)).toBe(true);
    resetSignedOutAccountQuarantineForTests();
    const guarded = protectHostedConnectionVault(raw);

    await expect(guarded.activeAccount()).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
    await expect(guarded.list('account-2')).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
  });

  it('rejects credential publication when another tab adds a cleanup barrier during the save', async () => {
    const base = createHostedConnectionVault(vaultStorage());
    const raw = {
      ...base,
      save: async (record: Parameters<typeof base.save>[0]) => {
        await base.save(record);
        await base.markCleanupPending(record.accountId);
      },
    };
    const guarded = protectHostedConnectionVault(raw);
    await expect(guarded.save({
      schemaVersion: 3,
      accountId: 'account-1',
      serverId: 'server-1',
      profileId: 'profile-1',
      serverName: 'Home',
      serverPublicKey: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA',
      serverPublicKeyFingerprint: 'sha256:home',
      currentRoute: { url: 'https://home.direct.getportico.tv', type: 'public_direct', verifiedAt: '2026-07-16T00:00:00.000Z' },
      session: { serverId: 'server-1', apiBaseUrl: 'https://home.direct.getportico.tv', accessToken: 'candidate', refreshToken: 'candidate-refresh', serverPublicKey: 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA' },
      lastSuccessfulConnectionAt: '2026-07-16T00:00:00.000Z',
      mutationVersion: 1,
    })).rejects.toBeInstanceOf(TrustedServerPublicationBlockedError);
    await expect(guarded.assertPublicationAllowed?.('account-1')).rejects.toBeInstanceOf(TrustedServerPublicationBlockedError);
    await expect(guarded.load('account-1', 'server-1')).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
  });

  it('does not swallow a tombstone published during Hosted account persistence', async () => {
    const base = createHostedConnectionVault(vaultStorage());
    const raw = {
      ...base,
      rememberAccount: async (account: Parameters<typeof base.rememberAccount>[0]) => {
        await base.rememberAccount(account);
        markAccountSignedOut(account.accountId);
      },
    };
    const guarded = protectHostedConnectionVault(raw);
    await expect(guarded.rememberAccount({
      accountId: 'account-1',
      displayName: 'Justin',
      email: 'justin@example.test',
    })).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
    await expect(guarded.activeAccount()).rejects.toBeInstanceOf(SignedOutAccountRestoreBlockedError);
  });
});
