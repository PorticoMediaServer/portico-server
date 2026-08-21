import { TrustedServerPublicationBlockedError } from '@porticomediaserver/client-core';
import type { HostedConnectionVault } from './hostedConnectionVault';

const TOMBSTONE_VERSION = 1 as const;
const MAX_ACCOUNT_ID_LENGTH = 512;

// Account tombstones are cleanup barriers, not browser-installation identity.
// They survive sign-out until exact verified cleanup and must never be used as
// a reason to silently clear or regenerate the installation binding.
/** Legacy aggregate key retained only for fail-closed migration. */
export const SIGNED_OUT_ACCOUNT_LEDGER_KEY = 'portico.hosted.signed-out-accounts.v1';
export const SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX = 'portico.hosted.signed-out-account.v1:';
export const LEGACY_SIGNED_OUT_ACCOUNT_KEY = 'portico.hosted.explicitly-signed-out-account';
/** Local-only reserved ID used when sign-out precedes authoritative account discovery. */
export const GLOBAL_SIGN_OUT_FENCE_ID = '__portico_global_sign_out_fence__';

type LegacySignedOutAccountLedger = {
  version: typeof TOMBSTONE_VERSION;
  accountIds: string[];
};

type SignedOutAccountTombstone = {
  version: typeof TOMBSTONE_VERSION;
  accountId: string;
};

export type SignedOutAccountQuarantine = {
  accountIds: ReadonlySet<string>;
  trustedForRestore: boolean;
};

type LedgerStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> & Partial<Pick<Storage, 'key' | 'length'>>;

const volatileQuarantine = new Set<string>();
let restoreTrustLost = false;

function browserStorage(): LedgerStorage | undefined {
  if (typeof window === 'undefined') return undefined;
  try {
    return window.localStorage;
  } catch {
    restoreTrustLost = true;
    return undefined;
  }
}

function validAccountId(value: unknown): value is string {
  return typeof value === 'string' && value.trim() === value && value.length > 0 && value.length <= MAX_ACCOUNT_ID_LENGTH;
}

function tombstoneKey(accountId: string): string {
  return `${SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX}${encodeURIComponent(accountId)}`;
}

function parseLegacyLedger(value: string): Set<string> {
  const parsed = JSON.parse(value) as Partial<LegacySignedOutAccountLedger> | null;
  if (!parsed || parsed.version !== TOMBSTONE_VERSION || !Array.isArray(parsed.accountIds)) {
    throw new Error('The signed-out account ledger has an unsupported shape.');
  }
  if (!parsed.accountIds.every(validAccountId)) {
    throw new Error('The signed-out account ledger contains invalid account identifiers.');
  }
  return new Set(parsed.accountIds);
}

function serializeTombstone(accountId: string): string {
  return JSON.stringify({ version: TOMBSTONE_VERSION, accountId } satisfies SignedOutAccountTombstone);
}

function parseTombstone(value: string, expectedAccountId?: string): string {
  const parsed = JSON.parse(value) as Partial<SignedOutAccountTombstone> | null;
  if (!parsed || parsed.version !== TOMBSTONE_VERSION || !validAccountId(parsed.accountId)) {
    throw new Error('A signed-out account tombstone has an unsupported shape.');
  }
  if (expectedAccountId !== undefined && parsed.accountId !== expectedAccountId) {
    throw new Error('A signed-out account tombstone does not match its storage key.');
  }
  return parsed.accountId;
}

function writeAndVerifyTombstone(storage: LedgerStorage, accountId: string): void {
  const key = tombstoneKey(accountId);
  storage.setItem(key, serializeTombstone(accountId));
  const confirmed = storage.getItem(key);
  if (confirmed === null || parseTombstone(confirmed, accountId) !== accountId) {
    throw new Error('The signed-out account tombstone could not be verified after writing.');
  }
}

function removeAndVerify(storage: LedgerStorage, key: string): void {
  storage.removeItem(key);
  if (storage.getItem(key) !== null) throw new Error('A signed-out account marker could not be removed.');
}

/** Migrates old aggregate/scalar markers without using an aggregate for new writes. */
function migrateLegacyMarkers(storage: LedgerStorage): void {
  const aggregate = storage.getItem(SIGNED_OUT_ACCOUNT_LEDGER_KEY);
  const legacyAccountId = storage.getItem(LEGACY_SIGNED_OUT_ACCOUNT_KEY);
  const accountIds = aggregate === null ? new Set<string>() : parseLegacyLedger(aggregate);
  if (legacyAccountId !== null) {
    if (!validAccountId(legacyAccountId)) throw new Error('The legacy signed-out account marker is invalid.');
    accountIds.add(legacyAccountId);
  }
  for (const accountId of accountIds) writeAndVerifyTombstone(storage, accountId);
  if (aggregate !== null) removeAndVerify(storage, SIGNED_OUT_ACCOUNT_LEDGER_KEY);
  if (legacyAccountId !== null) removeAndVerify(storage, LEGACY_SIGNED_OUT_ACCOUNT_KEY);
}

function prepareStorage(storage: LedgerStorage | undefined): storage is LedgerStorage {
  if (!storage) {
    restoreTrustLost = true;
    return false;
  }
  try {
    migrateLegacyMarkers(storage);
    return !restoreTrustLost;
  } catch {
    restoreTrustLost = true;
    return false;
  }
}

function readExactTombstone(storage: LedgerStorage, accountId: string): boolean {
  const raw = storage.getItem(tombstoneKey(accountId));
  if (raw === null) return false;
  parseTombstone(raw, accountId);
  volatileQuarantine.add(accountId);
  return true;
}

function enumerateTombstones(storage: LedgerStorage): Set<string> {
  if (typeof storage.key !== 'function' || typeof storage.length !== 'number') {
    throw new Error('Browser storage cannot enumerate signed-out account tombstones.');
  }
  const accountIds = new Set<string>();
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index);
    if (!key?.startsWith(SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX)) continue;
    const raw = storage.getItem(key);
    if (raw === null) throw new Error('A signed-out account tombstone disappeared while it was being verified.');
    const accountId = parseTombstone(raw);
    if (key !== tombstoneKey(accountId)) throw new Error('A signed-out account tombstone uses the wrong storage key.');
    accountIds.add(accountId);
    volatileQuarantine.add(accountId);
  }
  return accountIds;
}

/**
 * Returns the complete known quarantine and whether browser storage is safe to
 * use as a restore authority. Corrupt or unavailable storage is fail-closed.
 */
export function signedOutAccountQuarantine(storage: LedgerStorage | undefined = browserStorage()): SignedOutAccountQuarantine {
  let persisted: Set<string> | undefined;
  if (prepareStorage(storage)) {
    try {
      persisted = enumerateTombstones(storage);
    } catch {
      restoreTrustLost = true;
    }
  }
  const accountIds = new Set(volatileQuarantine);
  if (persisted) for (const accountId of persisted) accountIds.add(accountId);
  return { accountIds, trustedForRestore: persisted !== undefined && !restoreTrustLost };
}

/** Re-checks one exact account without relying on an earlier cross-tab snapshot. */
export function signedOutAccountRestoreStatus(accountId: string, storage: LedgerStorage | undefined = browserStorage()): { quarantined: boolean; trustedForRestore: boolean } {
  if (!validAccountId(accountId) || !prepareStorage(storage)) return { quarantined: true, trustedForRestore: false };
  try {
    const quarantined = volatileQuarantine.has(GLOBAL_SIGN_OUT_FENCE_ID)
      || volatileQuarantine.has(accountId)
      || readExactTombstone(storage, GLOBAL_SIGN_OUT_FENCE_ID)
      || readExactTombstone(storage, accountId);
    return { quarantined, trustedForRestore: !restoreTrustLost };
  } catch {
    restoreTrustLost = true;
    return { quarantined: true, trustedForRestore: false };
  }
}

/**
 * Consumes the tombstone payload carried by a cross-tab storage event. The
 * event itself is authoritative even if the writing tab already completed
 * cleanup and removed the key before this tab's queued event is delivered.
 * Malformed set events fence every account and permanently discard restore
 * trust for this process.
 */
export function accountFenceFromTombstoneEvent(key: string | null, newValue: string | null): string | undefined {
  if (!key?.startsWith(SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX) || newValue === null) return undefined;
  try {
    const accountId = parseTombstone(newValue);
    if (key !== tombstoneKey(accountId)) throw new Error('A signed-out account tombstone event uses the wrong storage key.');
    volatileQuarantine.add(accountId);
    return accountId;
  } catch {
    restoreTrustLost = true;
    volatileQuarantine.add(GLOBAL_SIGN_OUT_FENCE_ID);
    return GLOBAL_SIGN_OUT_FENCE_ID;
  }
}

/**
 * Publishes an independent per-account marker. Two tabs signing out different
 * accounts never share a read/modify/write record and therefore cannot erase
 * one another's tombstones.
 */
export function markAccountSignedOut(accountId: string, storage: LedgerStorage | undefined = browserStorage()): boolean {
  if (!validAccountId(accountId)) {
    restoreTrustLost = true;
    return false;
  }
  volatileQuarantine.add(accountId);
  const prepared = prepareStorage(storage);
  if (!storage) return false;
  try {
    writeAndVerifyTombstone(storage, accountId);
    return prepared && !restoreTrustLost;
  } catch {
    restoreTrustLost = true;
    return false;
  }
}

/** Removes only this account's marker after its stale vault records are gone. */
export function clearAccountAfterVerifiedCleanup(accountId: string, storage: LedgerStorage | undefined = browserStorage()): boolean {
  if (!validAccountId(accountId) || !prepareStorage(storage) || !storage || restoreTrustLost) return false;
  try {
    removeAndVerify(storage, tombstoneKey(accountId));
    volatileQuarantine.delete(accountId);
    return true;
  } catch {
    restoreTrustLost = true;
    return false;
  }
}

export class SignedOutAccountRestoreBlockedError extends Error {
  readonly cause: unknown;

  constructor(cause?: unknown) {
    super('Saved sign-in data cannot be trusted until browser storage is available and cleanup is verified.');
    this.name = 'SignedOutAccountRestoreBlockedError';
    this.cause = cause;
  }
}

function assertRestoreAllowed(accountId?: string): SignedOutAccountQuarantine {
  if (accountId !== undefined) {
    const status = signedOutAccountRestoreStatus(accountId);
    if (!status.trustedForRestore || status.quarantined) throw new SignedOutAccountRestoreBlockedError();
  }
  const quarantine = signedOutAccountQuarantine();
  if (!quarantine.trustedForRestore || quarantine.accountIds.has(GLOBAL_SIGN_OUT_FENCE_ID)) throw new SignedOutAccountRestoreBlockedError();
  return quarantine;
}

async function assertVaultRestoreAllowed(vault: HostedConnectionVault, accountId: string): Promise<void> {
  assertRestoreAllowed(accountId);
  let cleanupPending: string[];
  try {
    cleanupPending = await vault.cleanupPendingAccounts();
  } catch (reason) {
    throw new SignedOutAccountRestoreBlockedError(reason);
  }
  // A tombstone may have been published by another tab while IndexedDB was read.
  assertRestoreAllowed(accountId);
  if (cleanupPending.includes(GLOBAL_SIGN_OUT_FENCE_ID) || cleanupPending.includes(accountId)) throw new SignedOutAccountRestoreBlockedError();
}

/**
 * Applies the quarantine to every credential restore/publication entrypoint.
 * Destructive operations remain available so explicit sign-out can retry all
 * cleanup even while restore trust is unavailable.
 */
export function protectHostedConnectionVault(vault: HostedConnectionVault): HostedConnectionVault {
  return {
    persistencePolicy: vault.persistencePolicy,
    durability: () => vault.durability(),
    installationId: () => vault.installationId(),
    activeAccount: async () => {
      assertRestoreAllowed();
      let cleanupPending: string[];
      try {
        cleanupPending = await vault.cleanupPendingAccounts();
      } catch (reason) {
        throw new SignedOutAccountRestoreBlockedError(reason);
      }
      assertRestoreAllowed();
      if (cleanupPending.includes(GLOBAL_SIGN_OUT_FENCE_ID)) throw new SignedOutAccountRestoreBlockedError();
      const account = await vault.activeAccount();
      if (account) {
        await assertVaultRestoreAllowed(vault, account.accountId);
        if (cleanupPending.includes(account.accountId)) throw new SignedOutAccountRestoreBlockedError();
      }
      assertRestoreAllowed();
      return account;
    },
    knownAccountIds: () => vault.knownAccountIds(),
    rememberAccount: async (account) => {
      await assertVaultRestoreAllowed(vault, account.accountId);
      await vault.rememberAccount(account);
      await assertVaultRestoreAllowed(vault, account.accountId);
    },
    forgetAccount: (accountId) => vault.forgetAccount(accountId),
    markCleanupPending: (accountId) => vault.markCleanupPending(accountId),
    cleanupPendingAccounts: () => vault.cleanupPendingAccounts(),
    clearCleanupPending: (accountId) => vault.clearCleanupPending(accountId),
    load: async (accountId, serverId) => {
      await assertVaultRestoreAllowed(vault, accountId);
      const record = await vault.load(accountId, serverId);
      await assertVaultRestoreAllowed(vault, accountId);
      return record;
    },
    save: async (record) => {
      try {
        await assertVaultRestoreAllowed(vault, record.accountId);
      } catch (reason) {
        throw new TrustedServerPublicationBlockedError(reason);
      }
      await vault.save(record);
      try {
        await assertVaultRestoreAllowed(vault, record.accountId);
      } catch (reason) {
        throw new TrustedServerPublicationBlockedError(reason);
      }
    },
    compareAndSwap: async (expectedVersion, record) => {
      try {
        await assertVaultRestoreAllowed(vault, record.accountId);
      } catch (reason) {
        throw new TrustedServerPublicationBlockedError(reason);
      }
      const saved = await vault.compareAndSwap(expectedVersion, record);
      try {
        await assertVaultRestoreAllowed(vault, record.accountId);
      } catch (reason) {
        throw new TrustedServerPublicationBlockedError(reason);
      }
      return saved;
    },
    removeWithTombstone: (tombstone) => vault.removeWithTombstone(tombstone),
    loadRemovalTombstone: (accountId, serverId) => vault.loadRemovalTombstone(accountId, serverId),
    remove: (accountId, serverId) => vault.remove(accountId, serverId),
    clearAccount: (accountId) => vault.clearAccount(accountId),
    list: async (accountId) => {
      await assertVaultRestoreAllowed(vault, accountId);
      const records = await vault.list(accountId);
      await assertVaultRestoreAllowed(vault, accountId);
      return records;
    },
    automaticProfileTrust: async (scope, profileId) => {
      await assertVaultRestoreAllowed(vault, scope.accountId);
      const trust = await vault.automaticProfileTrust(scope, profileId);
      await assertVaultRestoreAllowed(vault, scope.accountId);
      return trust;
    },
    saveAutomaticProfileTrust: async (trust) => {
      await assertVaultRestoreAllowed(vault, trust.accountId);
      await vault.saveAutomaticProfileTrust(trust);
      await assertVaultRestoreAllowed(vault, trust.accountId);
    },
    clearAutomaticProfileTrust: (scope, profileId) => vault.clearAutomaticProfileTrust(scope, profileId),
    profileLaunchPreference: async (scope) => {
      await assertVaultRestoreAllowed(vault, scope.accountId);
      const preference = await vault.profileLaunchPreference(scope);
      await assertVaultRestoreAllowed(vault, scope.accountId);
      return preference;
    },
    saveProfileLaunchPreference: async (scope, preference) => {
      await assertVaultRestoreAllowed(vault, scope.accountId);
      await vault.saveProfileLaunchPreference(scope, preference);
      await assertVaultRestoreAllowed(vault, scope.accountId);
    },
    assertPublicationAllowed: async (accountId) => {
      try {
        await assertVaultRestoreAllowed(vault, accountId);
      } catch (reason) {
        throw new TrustedServerPublicationBlockedError(reason);
      }
    },
  };
}

export function resetSignedOutAccountQuarantineForTests(): void {
  volatileQuarantine.clear();
  restoreTrustLost = false;
}
