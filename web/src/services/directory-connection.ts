import {connectHostedServer, inspectDirectServer} from '@core/server-connections.ts';
import {ServerSignInProbe, parseServerSignIn} from '@core/sign-in.ts';
import {RouteError, routeOrigin} from '@core/route-identity.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {browserConnectionEnvironment} from '../platform/server-connections';
import type {ServicesAccount} from './session';
import {servicesInvitationAccount} from './invitation-account';
import type {HostedSession} from '@core/index.ts';
import type {DirectoryItem} from './directory-flow';
import {DirectoryAccountUnavailable, accountDirectoryFailure} from './directory-errors';
import {routePath as hostedRoutePath} from '../../../packages/contracts/src/hosted/routes.ts';

export function servicesDirectoryConnection(account: ServicesAccount, session: Omit<HostedSession,'refreshToken'>) {
  const source = servicesInvitationAccount(account, session);
  const check = (signal: AbortSignal) => {signal.throwIfAborted(); if (!source.isCurrent()) throw new RouteError('cancelled','The Portico Account changed.');};
  const accountRequest = async <T,>(path: string, signal: AbortSignal): Promise<T> => {
    try {return await source.request<T>(path, 'GET', undefined, signal);}
    catch (error) {throw accountDirectoryFailure(error);}
  };
  return async (server: DirectoryItem, signal: AbortSignal) => {
    check(signal);
    // Fetch the current signed route document for this exact directory pin;
    // a long-open chooser must not reuse its earlier inline envelope.
    const selected={id:server.id,name:server.name,baseUrl:server.baseUrl,publicKey:server.publicKey,policyRevision:server.policyRevision};
    const environment = browserConnectionEnvironment(server.id), transport = environment.fetcher ?? globalThis.fetch;
    const fetcher: typeof fetch = async (input, init) => {
      // Identify the account key endpoint before I/O. All server requests retain
      // their own failures; only this fixed Hosted path can open connect 2d.
      if (String(input) !== source.origin + hostedRoutePath('keys_get')) {
        try {return await transport(input, init);}
        catch (error) {if (error instanceof TypeError) throw new RouteError('route_unavailable', 'The server did not answer.'); throw error;}
      }
      let response: Response;
      try {response = await transport(input, init);} catch (error) {throw accountDirectoryFailure(error);}
      if (response.status === 429 || response.status >= 500) {
        const value = response.headers.get('Retry-After'), seconds = value ? (/^\d+$/.test(value) ? Number(value) : Math.max(0, Math.ceil((Date.parse(value) - Date.now()) / 1000))) : undefined;
        void response.body?.cancel().catch(() => {});
        throw new DirectoryAccountUnavailable(undefined, seconds);
      }
      return response;
    };
    const connected = await connectHostedServer(selected, {origin: source.origin, accountId: source.accountId, request: accountRequest}, {...environment, fetcher}, signal);
    try {
      check(signal);
      const system = parseServerSignIn(await connected.api.request(routePath('get_system'),'GET',undefined,signal)); check(signal);
      if (system.id !== server.id || system.signIn !== 'portico' || system.setupRequired || connected.connection.pin.publicKey !== server.publicKey) throw new RouteError('identity_mismatch','The selected server identity or sign-in changed.');
      const origin = await connected.connection.resolve(signal); check(signal);
      return {origin, dispose: () => connected.connection.dispose()};
    } catch (error) {connected.connection.dispose(); throw error;}
  };
}

/** Typed local address entry still proves the actual server before handoff. */
export async function servicesLocalOrigin(address: string, signal: AbortSignal) {
  const probe = new ServerSignInProbe(), abort = () => probe.cancel();
  signal.addEventListener('abort',abort,{once:true});
  try {
    signal.throwIfAborted(); const system = await probe.inspect(address); signal.throwIfAborted();
    const state = probe.getSnapshot();
    if (!system || !state.origin) throw state.error ?? new RouteError('route_unavailable','The server did not answer.');
    const origin = routeOrigin(state.origin), pin = await inspectDirectServer(origin,browserConnectionEnvironment(system.id),signal); signal.throwIfAborted();
    if (pin.serverId !== system.id) throw new RouteError('identity_mismatch','The server identity changed.');
    return origin;
  } finally {signal.removeEventListener('abort',abort); probe.dispose();}
}
