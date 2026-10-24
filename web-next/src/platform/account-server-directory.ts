import {decodeServersListResponse} from '../../../packages/contracts/src/hosted/decoders.ts';
import {routePath} from '../../../packages/contracts/src/hosted/routes.ts';
import type {browserAccount} from './account';
import {currentAccountWork, type AccountIdentity} from './account-work';

type AccountPort = Pick<ReturnType<typeof browserAccount>, 'api' | 'service'>;
/** Inert binding over the caller's existing browserAccount singleton. No UI,
 * session/store construction, cache, guessed origin or Server admission. */
export function createAccountServerDirectory(account: AccountPort, scope: AccountIdentity) {
  const captured = Object.freeze({accountId: scope.accountId, familyId: scope.familyId});
  return {
    scope: captured,
    read: async (signal?: AbortSignal) => {
      signal?.throwIfAborted();
      const result = await currentAccountWork(account, async session => {
        signal?.throwIfAborted();
        const raw = await account.api.send(routePath('servers_list'), 'GET', undefined, signal, session.accessToken);
        signal?.throwIfAborted();
        return decodeServersListResponse(raw);
      }, captured);
      signal?.throwIfAborted();
      return result;
    },
  };
}

export type AccountServerDirectory = ReturnType<typeof createAccountServerDirectory>;
