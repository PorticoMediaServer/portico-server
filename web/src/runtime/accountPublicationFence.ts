import { TrustedServerPublicationBlockedError } from '@porticomediaserver/client-core';

const LOCK_PREFIX = 'portico-hosted-account-publication-v1:';
const AMBIENT_COOKIE_MUTATION_LOCK = 'portico-ambient-cookie-mutation-v1';
const CHANNEL_NAME = 'portico-hosted-account-fence-v1';

type AccountFenceMessage = {version: 1; accountId: string};

function lockManager(): LockManager | undefined {
  if (typeof navigator === 'undefined') return undefined;
  return navigator.locks;
}

/**
 * Serializes the final authorization check/runtime publication boundary with
 * sign-out marker publication across every same-origin browsing context.
 * Unsupported browsers fail closed instead of pretending a process mutex is a
 * cross-tab security primitive.
 */
export async function withAccountPublicationLock<T>(accountId: string, operation: () => Promise<T> | T): Promise<T> {
  const locks = lockManager();
  if (!locks?.request) {
    throw new TrustedServerPublicationBlockedError(new Error('This browser cannot provide the required cross-tab account publication lock.'));
  }
  return locks.request(`${LOCK_PREFIX}${encodeURIComponent(accountId)}`, {mode: 'exclusive'}, operation);
}

/** Acquires a stable, de-duplicated set of account locks without lock-order cycles. */
export async function withAccountPublicationLocks<T>(accountIds: Iterable<string>, operation: () => Promise<T> | T): Promise<T> {
  const ordered = [...new Set(accountIds)].sort();
  const acquire = (index: number): Promise<T> => index >= ordered.length
    ? Promise.resolve().then(operation)
    : withAccountPublicationLock(ordered[index], () => acquire(index + 1));
  return acquire(0);
}

/**
 * Serializes every bundled HttpOnly-cookie mutation across all same-origin
 * browsing contexts. The cookie is one browser-global authority, so this lock
 * must not be partitioned by DataProvider instance, account, or profile.
 */
export async function withAmbientCookieMutationLock<T>(operation: () => Promise<T> | T): Promise<T> {
  const locks = lockManager();
  if (!locks?.request) {
    throw new TrustedServerPublicationBlockedError(new Error('This browser cannot provide the required cross-tab ambient-cookie mutation lock.'));
  }
  return locks.request(AMBIENT_COOKIE_MUTATION_LOCK, {mode: 'exclusive'}, operation);
}

/** Best-effort prompt; durable tombstones/barriers remain the restore authority. */
export function broadcastAccountFence(accountId: string): void {
  if (typeof BroadcastChannel === 'undefined') return;
  try {
    const channel = new BroadcastChannel(CHANNEL_NAME);
    channel.postMessage({version: 1, accountId} satisfies AccountFenceMessage);
    channel.close();
  } catch {
    // Storage events and per-action durable checks remain active.
  }
}

export function subscribeAccountFences(listener: (accountId: string) => void): () => void {
  if (typeof BroadcastChannel === 'undefined') return () => undefined;
  try {
    const channel = new BroadcastChannel(CHANNEL_NAME);
    channel.addEventListener('message', (event: MessageEvent<unknown>) => {
      const value = event.data as Partial<AccountFenceMessage> | null;
      if (value?.version === 1 && typeof value.accountId === 'string' && value.accountId) listener(value.accountId);
    });
    return () => channel.close();
  } catch {
    return () => undefined;
  }
}
