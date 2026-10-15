import {useMemo} from 'react';
import {accountApi, type AccountScope} from '../../bridge/account-api';

/**
 * The account a page acts for, as one object that changes only when the account or its sign-in
 * family does. Pages receive a fresh scope object on each render; keying work on this one keeps
 * clients and requests from being rebuilt for nothing, without leaving the hooks' dependencies
 * incomplete.
 */
export function useAccountScope(scope: AccountScope): AccountScope {
  const {accountId, familyId} = scope;
  return useMemo(() => ({accountId, familyId}), [accountId, familyId]);
}

/** Calls to the Portico Account service for this account, rebuilt only when the account changes. */
export function useAccountApi(scope: AccountScope): ReturnType<typeof accountApi> {
  const stable = useAccountScope(scope);
  return useMemo(() => accountApi(stable), [stable]);
}
