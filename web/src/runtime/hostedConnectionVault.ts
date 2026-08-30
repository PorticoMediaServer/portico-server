import {
	normalizeAccountServerInstallationPreferences,
	parseAutomaticProfileTrust,
	preferenceStorageKeys,
	TrustedServerDurabilityUncertainError,
	type AccountServerInstallationPreferences,
	type AutomaticProfileTrust,
	type PreferenceScopeIdentity,
	type TrustedServerConnectionAdapter,
	type TrustedServerConnectionRecord,
	type TrustedServerRemovalTombstone,
} from '@porticomediaserver/client-core';
import { secureRandomUUID } from './secureRandomUUID';

const DATABASE_NAME = 'portico-hosted-web-canonical-v1';
const DATABASE_VERSION = 1;
const ACCOUNTS_STORE = 'accounts';
const CONNECTIONS_STORE = 'connections';
const METADATA_STORE = 'metadata';
const PROFILE_TRUSTS_STORE = 'profile-trusts';
const ACTIVE_ACCOUNT_KEY = 'active-account';
const INSTALLATION_ID_KEY = 'installation-id';
const CLEANUP_PENDING_PREFIX = 'cleanup-pending:';

export const HOSTED_BROWSER_SESSION_TTL_MS = 180 * 24 * 60 * 60 * 1000;

export type HostedAccountSnapshot = {
  accountId: string;
  displayName: string;
  email: string;
  lastUsedAt: string;
  expiresAt: string;
};

type StoredConnection = {
  key: string;
  accountId: string;
  serverId: string;
  expiresAt: string;
  metadata: Omit<TrustedServerConnectionRecord, 'session'>;
  /** Present only in process-memory vaults. Durable browser storage omits it. */
  record?: TrustedServerConnectionRecord;
};

type StoredConnectionTombstone = TrustedServerRemovalTombstone & {
  key: string;
  kind: 'removal-tombstone';
};

type StoredConnectionEntry = StoredConnection | StoredConnectionTombstone;

type StoredMetadata = {
  key: string;
  value: unknown;
};

type StoredProfileTrust = {
  key: string;
  trust: AutomaticProfileTrust;
};

type ProfileTrustScope = Pick<PreferenceScopeIdentity, 'authority' | 'accountId' | 'serverId'>;

export interface HostedConnectionVaultStorage {
  get<T>(storeName: string, key: IDBValidKey): Promise<T | undefined>;
  put<T>(storeName: string, value: T): Promise<void>;
  delete(storeName: string, key: IDBValidKey): Promise<void>;
  getAll<T>(storeName: string): Promise<T[]>;
  /** Production storage implements these operations in one read/write transaction. */
  compareAndSwapConnection(key: string, expectedVersion: number, value: StoredConnection): Promise<boolean>;
  replaceConnectionWithTombstone(key: string, value: StoredConnectionTombstone): Promise<void>;
}

export interface HostedConnectionVault extends TrustedServerConnectionAdapter {
  readonly persistencePolicy: 'saved-session' | 'reauthorize-on-start';
  durability(): 'durable' | 'memory-only';
  installationId(): Promise<string>;
  activeAccount(): Promise<HostedAccountSnapshot | undefined>;
  /** Enumerates every account referenced by durable account or connection metadata. */
  knownAccountIds(): Promise<string[]>;
  rememberAccount(account: Pick<HostedAccountSnapshot, 'accountId' | 'displayName' | 'email'>): Promise<void>;
  forgetAccount(accountId: string): Promise<void>;
  markCleanupPending(accountId: string): Promise<void>;
  cleanupPendingAccounts(): Promise<string[]>;
  clearCleanupPending(accountId: string): Promise<void>;
	automaticProfileTrust(scope: ProfileTrustScope, profileId: string): Promise<AutomaticProfileTrust | undefined>;
	saveAutomaticProfileTrust(trust: AutomaticProfileTrust): Promise<void>;
	clearAutomaticProfileTrust(scope: ProfileTrustScope, profileId: string): Promise<void>;
	profileLaunchPreference(scope: PreferenceScopeIdentity): Promise<AccountServerInstallationPreferences | undefined>;
	saveProfileLaunchPreference(scope: PreferenceScopeIdentity, preference: AccountServerInstallationPreferences): Promise<void>;
  /** Re-checks the account authorization fence at a publication boundary. */
  assertPublicationAllowed?(accountId: string): Promise<void>;
}

function connectionKey(accountId: string, serverId: string): string {
  return `${accountId}\u0000${serverId}`;
}

function cleanupPendingKey(accountId: string): string {
  return `${CLEANUP_PENDING_PREFIX}${accountId}`;
}

function profileTrustKey(scope: ProfileTrustScope, profileId: string): string {
	return `${scope.authority}\u0000${scope.accountId}\u0000${scope.serverId}\u0000${profileId}`;
}

function expiration(now: Date): string {
  return new Date(now.getTime() + HOSTED_BROWSER_SESSION_TTL_MS).toISOString();
}

function expired(value: { expiresAt: string }, now: Date): boolean {
  const expiresAt = Date.parse(value.expiresAt);
  return !Number.isFinite(expiresAt) || expiresAt <= now.getTime();
}

function connectionMetadata(record: TrustedServerConnectionRecord): Omit<TrustedServerConnectionRecord, 'session'> {
  const { session: _session, ...metadata } = record;
  return metadata;
}

function isConnectionTombstone(value: StoredConnectionEntry): value is StoredConnectionTombstone {
  return 'kind' in value && value.kind === 'removal-tombstone';
}

/**
 * Builds the Hosted Web credential vault without coupling runtime logic to a
 * particular browser database. The production adapter uses origin-scoped
 * IndexedDB: server credentials never enter localStorage, URLs, or cookies
 * sent to user-operated direct server hosts.
 */
export function createHostedConnectionVault(
  storage: HostedConnectionVaultStorage,
  now: () => Date = () => new Date(),
  options: { persistCredentials?: boolean } = {},
): HostedConnectionVault {
  const persistCredentials = options.persistCredentials ?? true;
  const remove = async (accountId: string, serverId: string) => {
    await storage.delete(CONNECTIONS_STORE, connectionKey(accountId, serverId));
  };

  const load = async (accountId: string, serverId: string) => {
    const stored = await storage.get<StoredConnectionEntry>(CONNECTIONS_STORE, connectionKey(accountId, serverId));
    if (!stored || isConnectionTombstone(stored)) return undefined;
    if (expired(stored, now())) {
      await remove(accountId, serverId);
      return undefined;
    }
    if (!persistCredentials) return undefined;
    return stored.record;
  };

  const list = async (accountId: string) => {
    const records = await storage.getAll<StoredConnectionEntry>(CONNECTIONS_STORE);
    const current = now();
    const matches: TrustedServerConnectionRecord[] = [];
    await Promise.all(records.map(async (stored) => {
      if (isConnectionTombstone(stored)) return;
      if (stored.accountId !== accountId) return;
      if (expired(stored, current)) {
        await remove(stored.accountId, stored.serverId);
        return;
      }
      if (!persistCredentials) return;
      if (stored.record) matches.push(stored.record);
    }));
    return matches.sort((left, right) => Date.parse(right.lastSuccessfulConnectionAt) - Date.parse(left.lastSuccessfulConnectionAt));
  };

  return {
    persistencePolicy: persistCredentials ? 'saved-session' : 'reauthorize-on-start',
    ready: async () => {},
    durability: () => 'durable',
    load,
    save: async (record) => {
      await storage.put<StoredConnection>(CONNECTIONS_STORE, {
        key: connectionKey(record.accountId, record.serverId),
        accountId: record.accountId,
        serverId: record.serverId,
        expiresAt: expiration(now()),
        metadata: connectionMetadata(record),
        record: persistCredentials ? record : undefined,
      });
    },
    compareAndSwap: async (expectedVersion, record) => {
      const key = connectionKey(record.accountId, record.serverId);
      const value: StoredConnection = {
        key,
        accountId: record.accountId,
        serverId: record.serverId,
        expiresAt: expiration(now()),
        metadata: connectionMetadata(record),
        record: persistCredentials ? record : undefined,
      };
      return storage.compareAndSwapConnection(key, expectedVersion, value);
    },
    removeWithTombstone: async (tombstone) => {
      const key = connectionKey(tombstone.accountId, tombstone.serverId);
      const value: StoredConnectionTombstone = { ...tombstone, key, kind: 'removal-tombstone' };
      await storage.replaceConnectionWithTombstone(key, value);
    },
    loadRemovalTombstone: async (accountId, serverId) => {
      const stored = await storage.get<StoredConnectionEntry>(CONNECTIONS_STORE, connectionKey(accountId, serverId));
      if (!stored || !isConnectionTombstone(stored)) return undefined;
      const { key: _key, kind: _kind, ...tombstone } = stored;
      return tombstone;
    },
    remove,
    clearAccount: async (accountId) => {
      const records = await storage.getAll<StoredConnectionEntry>(CONNECTIONS_STORE);
      await Promise.all(records.filter((record) => record.accountId === accountId).map((record) => remove(record.accountId, record.serverId)));
    },
    list,
    installationId: async () => {
      const existing = await storage.get<StoredMetadata>(METADATA_STORE, INSTALLATION_ID_KEY);
      if (typeof existing?.value === 'string' && existing.value) return existing.value;
      const value = `web-${secureRandomUUID()}`;
      await storage.put<StoredMetadata>(METADATA_STORE, { key: INSTALLATION_ID_KEY, value });
      return value;
    },
    activeAccount: async () => {
      const metadata = await storage.get<StoredMetadata>(METADATA_STORE, ACTIVE_ACCOUNT_KEY);
      if (!metadata || typeof metadata.value !== 'string') return undefined;
      const account = await storage.get<HostedAccountSnapshot>(ACCOUNTS_STORE, metadata.value);
      if (!account) return undefined;
      if (expired(account, now())) {
        await storage.delete(ACCOUNTS_STORE, account.accountId);
        await storage.delete(METADATA_STORE, ACTIVE_ACCOUNT_KEY);
        return undefined;
      }
      return account;
    },
    knownAccountIds: async () => {
      const [accounts, records, active] = await Promise.all([
        storage.getAll<HostedAccountSnapshot>(ACCOUNTS_STORE),
        storage.getAll<StoredConnectionEntry>(CONNECTIONS_STORE),
        storage.get<StoredMetadata>(METADATA_STORE, ACTIVE_ACCOUNT_KEY),
      ]);
      const ids = new Set<string>();
      for (const account of accounts) if (typeof account.accountId === 'string' && account.accountId) ids.add(account.accountId);
      for (const record of records) if (typeof record.accountId === 'string' && record.accountId) ids.add(record.accountId);
      if (typeof active?.value === 'string' && active.value) ids.add(active.value);
      return [...ids].sort();
    },
    rememberAccount: async (account) => {
      const timestamp = now();
      await storage.put<HostedAccountSnapshot>(ACCOUNTS_STORE, {
        ...account,
        lastUsedAt: timestamp.toISOString(),
        expiresAt: expiration(timestamp),
      });
      await storage.put<StoredMetadata>(METADATA_STORE, { key: ACTIVE_ACCOUNT_KEY, value: account.accountId });
    },
    markCleanupPending: async (accountId) => {
      await storage.put<StoredMetadata>(METADATA_STORE, { key: cleanupPendingKey(accountId), value: accountId });
      const confirmed = await storage.get<StoredMetadata>(METADATA_STORE, cleanupPendingKey(accountId));
      if (confirmed?.value !== accountId) throw new Error('Portico could not verify the saved sign-out cleanup barrier.');
    },
    cleanupPendingAccounts: async () => {
      const metadata = await storage.getAll<StoredMetadata>(METADATA_STORE);
      return metadata.flatMap((record) => {
        if (!record.key.startsWith(CLEANUP_PENDING_PREFIX)) return [];
        const accountId = record.key.slice(CLEANUP_PENDING_PREFIX.length);
        if (!accountId || record.value !== accountId) throw new Error('Portico found an invalid saved sign-out cleanup barrier.');
        return [accountId];
      });
    },
    clearCleanupPending: async (accountId) => {
      await storage.delete(METADATA_STORE, cleanupPendingKey(accountId));
      if (await storage.get<StoredMetadata>(METADATA_STORE, cleanupPendingKey(accountId))) {
        throw new Error('Portico could not verify removal of the saved sign-out cleanup barrier.');
      }
    },
    forgetAccount: async (accountId) => {
      // Discovery failures must not prevent independent account and metadata
      // deletion attempts. Every connection we can enumerate is also attempted,
      // and the caller receives one aggregate failure only after all work settles.
      const [metadataResult, recordsResult] = await Promise.allSettled([
        storage.get<StoredMetadata>(METADATA_STORE, ACTIVE_ACCOUNT_KEY),
        storage.getAll<StoredConnectionEntry>(CONNECTIONS_STORE),
      ]);
      const deletions: Promise<void>[] = [storage.delete(ACCOUNTS_STORE, accountId)];
      if (metadataResult.status === 'fulfilled' && metadataResult.value?.value === accountId) {
        deletions.push(storage.delete(METADATA_STORE, ACTIVE_ACCOUNT_KEY));
      }
      if (recordsResult.status === 'fulfilled') {
        deletions.push(...recordsResult.value
          .filter((record) => record.accountId === accountId)
          .map((record) => storage.delete(CONNECTIONS_STORE, connectionKey(record.accountId, record.serverId))));
      }
      const deletionResults = await Promise.allSettled(deletions);
      const verificationResults = await Promise.allSettled([
        storage.get<HostedAccountSnapshot>(ACCOUNTS_STORE, accountId),
        storage.get<StoredMetadata>(METADATA_STORE, ACTIVE_ACCOUNT_KEY),
        storage.getAll<StoredConnectionEntry>(CONNECTIONS_STORE),
      ]);
      const failures: unknown[] = [metadataResult, recordsResult, ...deletionResults, ...verificationResults]
        .filter((result): result is PromiseRejectedResult => result.status === 'rejected')
        .map((result) => result.reason);
      if (verificationResults[0].status === 'fulfilled' && verificationResults[0].value) {
        failures.push(new Error('The signed-out account record is still present.'));
      }
      if (verificationResults[1].status === 'fulfilled' && verificationResults[1].value?.value === accountId) {
        failures.push(new Error('The signed-out account is still active in browser storage.'));
      }
      if (verificationResults[2].status === 'fulfilled' && verificationResults[2].value.some((record) => record.accountId === accountId)) {
        failures.push(new Error('Saved server credentials for the signed-out account are still present.'));
      }
      if (failures.length > 0) throw new AggregateError(failures, 'Portico could not remove every saved credential for this account.');
    },
	automaticProfileTrust: async (scope, profileId) => {
		const key = profileTrustKey(scope, profileId);
		const stored = await storage.get<StoredProfileTrust>(PROFILE_TRUSTS_STORE, key);
		if (!stored) return undefined;
		try {
			const trust = parseAutomaticProfileTrust(stored.trust);
			if (expired(trust, now())) {
				await storage.delete(PROFILE_TRUSTS_STORE, key);
				return undefined;
			}
			return trust;
		} catch {
			await storage.delete(PROFILE_TRUSTS_STORE, key);
			return undefined;
		}
	},
	saveAutomaticProfileTrust: async (value) => {
		const trust = parseAutomaticProfileTrust(value);
		await storage.put<StoredProfileTrust>(PROFILE_TRUSTS_STORE, { key: profileTrustKey(trust, trust.profileId), trust });
	},
	clearAutomaticProfileTrust: (scope, profileId) => storage.delete(PROFILE_TRUSTS_STORE, profileTrustKey(scope, profileId)),
	profileLaunchPreference: async (scope) => {
		const key = preferenceStorageKeys(scope).accountServerInstallation;
		const stored = await storage.get<StoredMetadata>(METADATA_STORE, key);
		if (stored?.value === undefined) return undefined;
		try { return normalizeAccountServerInstallationPreferences(stored.value, scope.deviceClass); }
		catch { await storage.delete(METADATA_STORE, key); return undefined; }
	},
	saveProfileLaunchPreference: async (scope, value) => {
		const key = preferenceStorageKeys(scope).accountServerInstallation;
		const preference = normalizeAccountServerInstallationPreferences(value, scope.deviceClass);
		await storage.put<StoredMetadata>(METADATA_STORE, { key, value: preference });
	},
  };
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    request.addEventListener('success', () => resolve(request.result), { once: true });
    request.addEventListener('error', () => reject(request.error ?? new Error('Portico browser storage failed.')), { once: true });
  });
}

function transactionComplete(transaction: IDBTransaction): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    transaction.addEventListener('complete', () => resolve(), { once: true });
    transaction.addEventListener('abort', () => reject(transaction.error ?? new Error('Portico browser storage was interrupted.')), { once: true });
    transaction.addEventListener('error', () => reject(transaction.error ?? new Error('Portico browser storage failed.')), { once: true });
  });
}

export function createIndexedDBHostedConnectionVault(factory: IDBFactory = indexedDB): HostedConnectionVault {
  let databasePromise: Promise<IDBDatabase> | undefined;
  const database = () => {
    if (!databasePromise) {
      databasePromise = new Promise<IDBDatabase>((resolve, reject) => {
        const request = factory.open(DATABASE_NAME, DATABASE_VERSION);
        request.addEventListener('upgradeneeded', () => {
          const db = request.result;
          if (!db.objectStoreNames.contains(ACCOUNTS_STORE)) db.createObjectStore(ACCOUNTS_STORE, { keyPath: 'accountId' });
          if (!db.objectStoreNames.contains(CONNECTIONS_STORE)) db.createObjectStore(CONNECTIONS_STORE, { keyPath: 'key' });
          if (!db.objectStoreNames.contains(METADATA_STORE)) db.createObjectStore(METADATA_STORE, { keyPath: 'key' });
          if (!db.objectStoreNames.contains(PROFILE_TRUSTS_STORE)) db.createObjectStore(PROFILE_TRUSTS_STORE, { keyPath: 'key' });
        });
        request.addEventListener('success', () => resolve(request.result), { once: true });
        request.addEventListener('error', () => reject(request.error ?? new Error('Portico browser storage could not open.')), { once: true });
        request.addEventListener('blocked', () => reject(new Error('Portico browser storage is blocked by another tab.')), { once: true });
      });
    }
    return databasePromise;
  };

  const storage: HostedConnectionVaultStorage = {
    get: async <T,>(storeName: string, key: IDBValidKey) => {
      const db = await database();
      return requestResult(db.transaction(storeName, 'readonly').objectStore(storeName).get(key)) as Promise<T | undefined>;
    },
    put: async <T,>(storeName: string, value: T) => {
      const db = await database();
      const transaction = db.transaction(storeName, 'readwrite');
      transaction.objectStore(storeName).put(value);
      await transactionComplete(transaction);
    },
    delete: async (storeName, key) => {
      const db = await database();
      const transaction = db.transaction(storeName, 'readwrite');
      transaction.objectStore(storeName).delete(key);
      await transactionComplete(transaction);
    },
    getAll: async <T,>(storeName: string) => {
      const db = await database();
      return requestResult(db.transaction(storeName, 'readonly').objectStore(storeName).getAll()) as Promise<T[]>;
    },
    compareAndSwapConnection: async (key, expectedVersion, value) => {
      const db = await database();
      const transaction = db.transaction(CONNECTIONS_STORE, 'readwrite');
      const objectStore = transaction.objectStore(CONNECTIONS_STORE);
      const request = objectStore.get(key) as IDBRequest<StoredConnectionEntry | undefined>;
      const matched = await new Promise<boolean>((resolve, reject) => {
        request.addEventListener('success', () => {
          const current = request.result;
          const currentVersion = current
            ? isConnectionTombstone(current) ? current.mutationVersion : current.metadata.mutationVersion
            : 0;
          if (currentVersion !== expectedVersion) {
            resolve(false);
            return;
          }
          objectStore.put(value);
          resolve(true);
        }, { once: true });
        request.addEventListener('error', () => reject(request.error ?? new Error('Portico browser storage failed.')), { once: true });
      });
      await transactionComplete(transaction);
      return matched;
    },
    replaceConnectionWithTombstone: async (_key, value) => {
      const db = await database();
      const transaction = db.transaction(CONNECTIONS_STORE, 'readwrite');
      transaction.objectStore(CONNECTIONS_STORE).put(value);
      await transactionComplete(transaction);
    },
  };
  return createHostedConnectionVault(storage, () => new Date(), { persistCredentials: false });
}

function createMemoryVaultStorage(): HostedConnectionVaultStorage {
  const stores = new Map<string, Map<string, unknown>>();
  const store = (name: string) => {
    const current = stores.get(name) ?? new Map<string, unknown>();
    stores.set(name, current);
    return current;
  };
  const keyFor = (key: IDBValidKey) => JSON.stringify(key);
  return {
    get: async <T,>(storeName: string, key: IDBValidKey) => store(storeName).get(keyFor(key)) as T | undefined,
    put: async <T,>(storeName: string, value: T) => {
      const candidate = value as { key?: IDBValidKey; accountId?: IDBValidKey };
      const key = candidate.key ?? candidate.accountId;
      if (key === undefined) throw new Error('Portico browser storage received a record without a key.');
      store(storeName).set(keyFor(key), value);
    },
    delete: async (storeName, key) => { store(storeName).delete(keyFor(key)); },
    getAll: async <T,>(storeName: string) => [...store(storeName).values()] as T[],
    compareAndSwapConnection: async (key, expectedVersion, value) => {
      const current = store(CONNECTIONS_STORE).get(keyFor(key)) as StoredConnectionEntry | undefined;
      const currentVersion = current
        ? isConnectionTombstone(current) ? current.mutationVersion : current.metadata.mutationVersion
        : 0;
      if (currentVersion !== expectedVersion) return false;
      store(CONNECTIONS_STORE).set(keyFor(key), value);
      return true;
    },
    replaceConnectionWithTombstone: async (key, value) => { store(CONNECTIONS_STORE).set(keyFor(key), value); },
  };
}

/**
 * Uses IndexedDB when available and degrades to a tab-lifetime memory vault in
 * hardened/private contexts. Persistence failure never blocks a live direct
 * connection, but it also never falls back to localStorage for raw tokens.
 */
export function createBrowserHostedConnectionVault(factory: IDBFactory | undefined = globalThis.indexedDB): HostedConnectionVault {
  if (!factory) {
    const memory = createHostedConnectionVault(createMemoryVaultStorage());
    return { ...memory, persistencePolicy: 'reauthorize-on-start', durability: () => 'memory-only' };
  }
  return createBrowserHostedConnectionVaultWithDurableMetadata(createIndexedDBHostedConnectionVault(factory));
}

/** Testable browser composition boundary: each call owns fresh credential memory. */
export function createBrowserHostedConnectionVaultWithDurableMetadata(indexed: HostedConnectionVault): HostedConnectionVault {
  if (indexed.persistencePolicy !== 'reauthorize-on-start') {
    throw new Error('Browser durable storage must reject reusable server credentials and require reauthorization on start.');
  }
  const memory = createHostedConnectionVault(createMemoryVaultStorage());
  let durableMetadata = true;
  const runRead = async <T,>(primary: () => Promise<T>, fallback: () => Promise<T>) => {
    if (!durableMetadata) return fallback();
    try {
      return await primary();
    } catch {
      durableMetadata = false;
      return fallback();
    }
  };
  const runWrite = async (primary: () => Promise<void>, fallback: () => Promise<void>) => {
    try {
      await primary();
      durableMetadata = true;
    } catch (reason) {
      durableMetadata = false;
      await fallback();
      throw reason;
    }
  };
  const runWriteBoth = async (primary: () => Promise<void>, processMemory: () => Promise<void>) => {
    const results = await Promise.allSettled([primary(), processMemory()]);
    durableMetadata = results[0].status === 'fulfilled' && results[1].status === 'fulfilled';
    const failures = results.flatMap((result) => result.status === 'rejected' ? [result.reason] : []);
    if (failures.length > 0) throw new AggregateError(failures, 'Portico could not update both browser metadata and process-memory credentials.');
  };
  const saveCredentialWithAuxiliaryMetadata = async (record: TrustedServerConnectionRecord) => {
    const [metadataResult, credentialResult] = await Promise.allSettled([indexed.save(record), memory.save(record)]);
    durableMetadata = metadataResult.status === 'fulfilled';
    if (credentialResult.status === 'rejected') {
      throw new TrustedServerDurabilityUncertainError(
        credentialResult.reason,
        metadataResult.status === 'rejected' ? [metadataResult.reason] : [],
      );
    }
    // IndexedDB holds only non-authenticating route metadata. Its failure does
    // not invalidate the verified process-memory credential; adapter durability
    // already reports memory-only to Client Core.
  };
  return {
    persistencePolicy: 'reauthorize-on-start',
    ready: async () => { await indexed.ready(); },
    // Healthy restart metadata is durable even though the explicit policy
    // requires Hosted reauthorization to mint a new server credential family.
    durability: () => durableMetadata ? 'durable' : 'memory-only',
    installationId: () => runRead(() => indexed.installationId(), () => memory.installationId()),
    // Reusable server credential families are process-memory only. Durable
    // storage contains pinned route/server metadata but is never a restore
    // authority after a browser restart.
    list: async (accountId) => {
      const active = await memory.list(accountId);
      if (active.length > 0) return active;
      return [];
    },
    load: async (accountId, serverId) => {
      const active = await memory.load(accountId, serverId);
      if (active) return active;
      return undefined;
    },
    save: saveCredentialWithAuxiliaryMetadata,
    compareAndSwap: async (expectedVersion, record) => {
      const metadataSaved = await indexed.compareAndSwap(expectedVersion, record);
      if (!metadataSaved) return false;
      durableMetadata = true;
      try {
        const credentialSaved = await memory.compareAndSwap(expectedVersion, record);
        if (!credentialSaved) {
          throw new TrustedServerDurabilityUncertainError(new Error('The process-memory credential changed concurrently.'));
        }
        return true;
      } catch (reason) {
        throw reason instanceof TrustedServerDurabilityUncertainError
          ? reason
          : new TrustedServerDurabilityUncertainError(reason);
      }
    },
    removeWithTombstone: (tombstone) => runWriteBoth(
      () => indexed.removeWithTombstone(tombstone),
      () => memory.removeWithTombstone(tombstone),
    ),
    loadRemovalTombstone: async (accountId, serverId) => {
      const [durable, active] = await Promise.all([
        indexed.loadRemovalTombstone(accountId, serverId),
        memory.loadRemovalTombstone(accountId, serverId),
      ]);
      if (!durable) return active;
      if (!active) return durable;
      return durable.mutationVersion >= active.mutationVersion ? durable : active;
    },
    remove: (accountId, serverId) => runWriteBoth(() => indexed.remove(accountId, serverId), () => memory.remove(accountId, serverId)),
    clearAccount: (accountId) => runWriteBoth(() => indexed.clearAccount(accountId), () => memory.clearAccount(accountId)),
    activeAccount: () => runRead(() => indexed.activeAccount(), () => memory.activeAccount()),
    // Global sign-out cleanup must enumerate the durable authority. Never hide
    // an IndexedDB enumeration failure behind process memory.
    knownAccountIds: async () => {
      const [durable, active] = await Promise.all([indexed.knownAccountIds(), memory.knownAccountIds()]);
      return [...new Set([...durable, ...active])].sort();
    },
    rememberAccount: (account) => runWriteBoth(() => indexed.rememberAccount(account), () => memory.rememberAccount(account)),
    forgetAccount: (accountId) => runWriteBoth(() => indexed.forgetAccount(accountId), () => memory.forgetAccount(accountId)),
    markCleanupPending: (accountId) => runWrite(() => indexed.markCleanupPending(accountId), () => memory.markCleanupPending(accountId)),
    // A durable cleanup barrier is an authorization fence, not a cache hint.
    // Never hide an IndexedDB read failure behind an empty memory fallback.
    cleanupPendingAccounts: () => indexed.cleanupPendingAccounts(),
    clearCleanupPending: (accountId) => runWrite(() => indexed.clearCleanupPending(accountId), () => memory.clearCleanupPending(accountId)),
	automaticProfileTrust: (scope, profileId) => runRead(() => indexed.automaticProfileTrust(scope, profileId), () => memory.automaticProfileTrust(scope, profileId)),
	saveAutomaticProfileTrust: (trust) => runWrite(() => indexed.saveAutomaticProfileTrust(trust), () => memory.saveAutomaticProfileTrust(trust)),
	clearAutomaticProfileTrust: (scope, profileId) => runWrite(() => indexed.clearAutomaticProfileTrust(scope, profileId), () => memory.clearAutomaticProfileTrust(scope, profileId)),
	profileLaunchPreference: (scope) => runRead(() => indexed.profileLaunchPreference(scope), () => memory.profileLaunchPreference(scope)),
	saveProfileLaunchPreference: (scope, preference) => runWrite(() => indexed.saveProfileLaunchPreference(scope, preference), () => memory.saveProfileLaunchPreference(scope, preference)),
  };
}
