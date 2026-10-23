import {ApiError, type HostedSession} from '@core/index.ts';
import {HostedAccountClient} from '@core/hosted-security.ts';
import {browserAccount} from '../platform/account';

/** The Services root shares the browser's one cookie coordinator with the server app. */
export function servicesAccount() {
  const account = browserAccount();
  const client = new HostedAccountClient(account.api.origin, 'browser',
    (path, method, body, signal, token) => account.api.send(path, method, body, signal, token));
  return {...account, client};
}

export type ServicesAccount = ReturnType<typeof servicesAccount>;
export type AccountIdentity = Readonly<{accountId: string; familyId: string}>;
export function accountIdentity(session: Pick<HostedSession, 'account' | 'familyId'>): AccountIdentity {
  if (!session.familyId) throw new ApiError(401, 'account_signed_out', 'account_signed_out', false);
  return {accountId: session.account.id, familyId: session.familyId};
}
export function sameAccount(a: AccountIdentity, b: AccountIdentity) {
  return a.accountId === b.accountId && a.familyId === b.familyId;
}

type AccountPort = Pick<ServicesAccount, 'service'>;
/** Bind review and mutation results to the account AND family that initiated them.
 * Cookie renewal and cross-tab synchronization remain exclusively in client-core. */
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
