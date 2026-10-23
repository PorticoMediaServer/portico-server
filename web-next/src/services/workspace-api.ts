import {ApiError} from '@core/index.ts';
import {AccountNotificationsClient} from '@core/account-notifications.ts';
import {AccountSessionReviewService} from '@core/account-sessions.ts';
import {decodeAccountDevicesListResponse, decodeServersListResponse} from '../../../packages/contracts/src/hosted/decoders.ts';
import {routePath} from '../../../packages/contracts/src/hosted/routes.ts';
import {currentAccountWork, type AccountIdentity, type ServicesAccount} from './session';

/** Every read/mutation is bound to the account and family that opened this workspace. */
export function workspaceApi(account: ServicesAccount, identity: AccountIdentity) {
  const request = <T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal) =>
    currentAccountWork(account, session => account.api.send<T>(path, method, body, signal, session.accessToken), identity);
  return {
    request,
    notifications: new AccountNotificationsClient(request),
    sessions: () => new AccountSessionReviewService({scope: identity, api: {request}}),
    security: (signal?: AbortSignal) => currentAccountWork(account, session => account.client.state(identity, session.accessToken, signal), identity),
    devices: async (signal?: AbortSignal) => {
      const result = decodeAccountDevicesListResponse(await request(routePath('account_devices_list'), 'GET', undefined, signal));
      if (result.accountId !== identity.accountId || result.currentFamilyId !== identity.familyId)
        throw new ApiError(409, 'account_changed', 'account_changed', false);
      return result;
    },
    servers: async (signal?: AbortSignal) => decodeServersListResponse(await request(routePath('servers_list'), 'GET', undefined, signal)),
  };
}

export type WorkspaceApi = ReturnType<typeof workspaceApi>;
export type WorkspaceDevices = Awaited<ReturnType<WorkspaceApi['devices']>>;
export type WorkspaceServers = Awaited<ReturnType<WorkspaceApi['servers']>>;
