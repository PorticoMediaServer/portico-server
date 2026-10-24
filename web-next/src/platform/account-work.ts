import {ApiError, type HostedSession} from '@core/index.ts';
import type {browserAccount} from './account';

export type AccountIdentity = Readonly<{accountId: string; familyId: string}>;
export function accountIdentity(session: Pick<HostedSession, 'account' | 'familyId'>): AccountIdentity {
  if (!session.familyId) throw new ApiError(401, 'account_signed_out', 'account_signed_out', false);
  return {accountId: session.account.id, familyId: session.familyId};
}
export function sameAccount(a: AccountIdentity, b: AccountIdentity) {
  return a.accountId === b.accountId && a.familyId === b.familyId;
}

type AccountPort = Pick<ReturnType<typeof browserAccount>, 'service'>;
/** Shared browser account/family boundary. Cookie renewal and cross-tab
 * synchronization remain exclusively in the canonical CredentialSessionService. */
export async function currentAccountWork<T>(account: AccountPort,
  work: (session: Omit<HostedSession, 'refreshToken'>) => Promise<T>, expected?: AccountIdentity): Promise<T> {
  await account.service.synchronize();
  const session = await account.service.accessSession();
  const identity = accountIdentity(session);
  if (expected && !sameAccount(identity, expected)) throw accountChanged();
  const result = await work(session);
  await account.service.synchronize();
  const current = account.service.getSnapshot().session;
  if (!current || !sameAccount(identity, accountIdentity(current))) throw accountChanged();
  return result;
}

function accountChanged() { return new ApiError(409, 'account_changed', 'account_changed', false); }
