import {ApiError} from '@core/index.ts';
import {browserAccount} from './account';

export type AccountScope = Readonly<{accountId: string; familyId: string}>;

/** Calls to the Portico Account service on behalf of the page that is open. Every call is
 * something the person just asked for, so these go straight out; and every answer is dropped
 * if the account signed in to this browser changed while it was in flight. */
export function accountApi(scope: AccountScope) {
  const central = browserAccount();
  const changed = () => new ApiError(409, 'account_changed', 'Your Portico Account session changed. Reopen this page.', false);
  return {
    async request<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
      const session = await central.service.accessSession();
      if (signal?.aborted || session.account.id !== scope.accountId || session.familyId !== scope.familyId) throw changed();
      const response = await fetch(central.api.origin + path, {method, signal, headers: {Authorization: 'Bearer ' + session.accessToken, ...(body === undefined ? {} : {'Content-Type': 'application/json'})}, ...(body === undefined ? {} : {body: JSON.stringify(body)})});
      const live = central.service.getSnapshot().session;
      if (signal?.aborted || live?.account.id !== scope.accountId || live.familyId !== scope.familyId) throw changed();
      if (!response.ok) { const value = await response.json().catch(() => null); throw new ApiError(response.status, value?.error?.code ?? 'request_failed', value?.error?.message ?? 'The account service could not do that.', value?.error?.retryable ?? response.status >= 500); }
      return (response.status === 204 || response.status === 202 && !response.headers.get('content-type')?.includes('json') ? undefined : await response.json()) as T;
    },
    async token(): Promise<string> { return (await central.service.accessSession()).accessToken; },
  };
}
