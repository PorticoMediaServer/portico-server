import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {type AppSessionStore, knownServerRoutes} from '@core/app-sessions.ts';
import {installationIdentity} from '@core/installation.ts';
import {type RegisteredSignIn, type ReturnPending, type ReturnPendingStore, prepareRegisteredSignIn, consumeSameTabReturn} from '@core/same-tab-return.ts';
import {assertFirstContactIdentity, routeBase64, routeBytes, routeOrigin, serverPin, sameServerPin, type ServerPin} from '@core/route-identity.ts';
import {connectPairedServer, connectPairedPorticoServer, rememberNativeSession, type ConnectionEnvironment} from '@core/server-connections.ts';
import {CodedError} from '@core/server-messages.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {secureBrowserPending} from './secure-pending';
import {browserAppSessions, browserConnectionEnvironment} from './server-connections';
import {type CapturedPorticoReturn, dispatchPorticoReturn} from './portico-return-dispatch';

type Pending = Readonly<{
  version: 1; kind: 'registered'; origin: string; hostedOrigin: string; pin: ServerPin;
  expectedRevision: number; attemptId: string; expiresAt: string; pending?: ReturnPending;
}>;
type Journal = Readonly<{
  assertAvailable: () => void;
  read: () => Promise<Pending | undefined>;
  change: (update: (old: Pending | undefined) => Pending | undefined) => Promise<void>;
}>;
type Ports = Readonly<{
  journal: Journal; sessions: Pick<AppSessionStore, 'read' | 'connectionStorage'>;
  environment: (serverId: string) => ConnectionEnvironment;
  installationId: () => string;
  pageOrigin: () => string;
  /** Only the steward's checked canonical builder may be supplied by production. */
  startURL: (value: RegisteredSignIn) => string;
  prepare?: typeof prepareRegisteredSignIn; consume?: typeof consumeSameTabReturn;
  connect?: typeof connectPairedServer; connectPortico?: typeof connectPairedPorticoServer; remember?: typeof rememberNativeSession;
  retire?: (api: HttpLocalApi, session: LocalSession) => Promise<void>;
}>;
const cancelled = () => new CodedError('cancelled', 'This server sign-in changed.');
const invalid = () => new CodedError('invalid_return', 'This sign-in is no longer available.');

/** One tab's private verifier and captured selection revision. The Hosted handoff
 * receives only RegisteredSignIn. This class never requests a Hosted credential. */
export class BrowserRegisteredSignIn {
  private epoch = 0;
  private controller?: AbortController;
  constructor(private readonly ports: Ports) {}
  private operation() {
    this.controller?.abort(); const epoch = ++this.epoch, controller = this.controller = new AbortController();
    return {signal: controller.signal, current: () => this.epoch === epoch && !controller.signal.aborted};
  }
  cancel() {++this.epoch; this.controller?.abort();}
  async prepare(api: HttpLocalApi, origin: string, hostedOrigin: string, installationId: string,
    move: Readonly<{moveTicket?: string; moveCode?: string}> = {}) {
    this.ports.journal.assertAvailable();
    if (routeOrigin(origin) !== origin || origin !== this.ports.pageOrigin() || !origin.startsWith('https://') || installationId !== this.ports.installationId()) throw invalid();
    const route = api.getRouteConnection();
    if (!route || route.getSnapshot().phase !== 'ready') throw invalid();
    const operation = this.operation(), state = await this.ports.sessions.read();
    if (!operation.current()) throw cancelled();
    assertFirstContactIdentity(origin, route.pin, {knownRoutes: knownServerRoutes(state)});
    const attemptId = routeBase64(crypto.getRandomValues(new Uint8Array(32)));
    // Supersede before network work. A second controller sharing this tab's
    // vault wins atomically, even if the earlier prepare answers much later.
    await this.ports.journal.change(() => {
      if (!operation.current()) throw cancelled();
      return {version: 1, kind: 'registered', origin, hostedOrigin, pin: route.pin,
        expectedRevision: state.revision, attemptId, expiresAt: ''};
    });
    const port: ReturnPendingStore = {
      read: async () => (await this.ports.journal.read())?.pending,
      change: async update => this.ports.journal.change(old => {
        if (!operation.current() || old?.attemptId !== attemptId) throw cancelled();
        const pending = update(old?.pending);
        if (!pending) return undefined;
        // prepareRegisteredSignIn provides the actual producer expiry after the
        // private write. Until then the record is deliberately not consumable.
        return {version: 1, kind: 'registered', origin, hostedOrigin, pin: route.pin,
          expectedRevision: state.revision, attemptId, expiresAt: '', pending};
      }),
    };
    try {
      const value = await (this.ports.prepare ?? prepareRegisteredSignIn)(api, port,
        {origin, hostedOrigin, installationId, ...move}, {signal: operation.signal, isCurrent: operation.current});
      const url = this.ports.startURL(value);
      if (!operation.current() || (await this.ports.sessions.read()).revision !== state.revision) throw cancelled();
      await this.ports.journal.change(old => {
        if (!operation.current() || !old || old.attemptId !== attemptId || old.pending?.state !== value.start.state) throw cancelled();
        return {...old, expiresAt: value.expiresAt};
      });
      if (!operation.current()) throw cancelled();
      return url;
    } catch (error) {
      await this.ports.journal.change(old => old?.attemptId === attemptId ? undefined : old);
      throw error;
    }
  }
  private async retire(api: HttpLocalApi, session: LocalSession) {
    if (this.ports.retire) return this.ports.retire(api, session);
    const temporary = new HttpLocalApi(api.baseUrl, session.accessToken), route = api.getRouteConnection();
    if (route) temporary.useRoutes(route);
    await temporary.request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});
  }
  /** capturePorticoReturn must already have removed the fragment synchronously.
   * Taking the journal is atomic; another tab/reload cannot reuse this attempt. */
  async finish(captured: CapturedPorticoReturn) {
    this.ports.journal.assertAvailable();
    const operation = this.operation(); let record: Pending | undefined;
    await this.ports.journal.change(old => {record = old; return undefined;});
    if (!record || record.version !== 1 || record.kind !== 'registered' || !record.pending) throw invalid();
    const saved = {...record, pending: record.pending}, url = new URL(captured.href), expires = Date.parse(saved.expiresAt), now = Date.now();
    if (!Number.isSafeInteger(saved.pending.createdAt) || saved.pending.createdAt < 0 || saved.pending.createdAt > now + 60_000 ||
      !Number.isFinite(expires) || expires <= now || expires > saved.pending.createdAt + 5 * 60_000 ||
      url.origin !== this.ports.pageOrigin() || url.origin !== saved.origin || saved.pending.registrationOrigin !== saved.origin || saved.pending.purpose !== 'sign-in' ||
      saved.pending.serverId !== saved.pin.serverId || saved.pending.installationId !== this.ports.installationId() ||
      !Number.isSafeInteger(saved.expectedRevision) || saved.expectedRevision < 0) throw invalid();
    const dispatch = dispatchPorticoReturn(captured, {kind: 'registered'});
    const actualPin = await serverPin(saved.pin.serverId, saved.pin.publicKey, saved.pin.fingerprint);
    if (!sameServerPin(actualPin, saved.pin)) throw invalid();
    const current = async () => {
      if (!operation.current() || (await this.ports.sessions.read()).revision !== saved.expectedRevision) throw cancelled();
    };
    await current();
    assertFirstContactIdentity(saved.origin, saved.pin, {knownRoutes: knownServerRoutes(await this.ports.sessions.read())});
    const environment = this.ports.environment(saved.pin.serverId);
    const connected = await (this.ports.connect ?? connectPairedServer)(saved.origin, saved.pin, environment, operation.signal);
    let grant: Awaited<ReturnType<typeof consumeSameTabReturn>> | undefined, adopted = false;
    let bound: Awaited<ReturnType<typeof connectPairedPorticoServer>> | undefined;
    try {
      await current();
      let pending: ReturnPending | undefined = saved.pending;
      const port: ReturnPendingStore = {read: async () => pending, change: async update => {pending = update(pending);}};
      grant = await (this.ports.consume ?? consumeSameTabReturn)(connected.api, port, {href: dispatch.href}, {replaceState: () => {}},
        {isCurrent: operation.current, retireLateGrant: session => this.retire(connected.api, session)}, operation.signal);
      if (grant.kind !== 'signed-in') throw invalid();
      await current();
      const accountId = grant.snapshot.account.hostedAccountId;
      if (!accountId || grant.snapshot.serverId !== saved.pin.serverId || grant.resetFileOwnerSession || grant.passwordChangeRequired) throw invalid();
      // The verified server grant supplies its admitted account id. The public
      // route-key reader needs no Hosted directory membership or account cookie.
      bound = await (this.ports.connectPortico ?? connectPairedPorticoServer)(saved.origin, saved.pin,
        {origin: saved.hostedOrigin, accountId, request: async () => {throw new CodedError('account_signed_out', 'Use Portico Account to refresh its directory.');}}, environment, operation.signal);
      await current();
      bound.api.setAccessToken(grant.session.accessToken);
      const admission = {...environment, storage: this.ports.sessions.connectionStorage(saved.pin.serverId,
        {signIn: 'portico', hostedAccount: bound.hosted, expectedRevision: saved.expectedRevision})};
      await (this.ports.remember ?? rememberNativeSession)(bound.api, grant.session, admission, bound.hosted, operation.current);
      if (!operation.current()) throw cancelled();
      adopted = true;
      return {api: bound.api, result: grant};
    } finally {
      if (!adopted && grant?.kind === 'signed-in') await this.retire(connected.api, grant.session);
      connected.connection.dispose();
      if (!adopted) bound?.connection.dispose();
    }
  }
}

/** The opaque tab locator contains no verifier, account, server or credential. */
export function createBrowserRegisteredSignIn(startURL: Ports['startURL']) {
  const guard = secureBrowserPending<Pending>('portico.registered-sign-in.guard'); guard.assertAvailable();
  const key = 'portico.registered-sign-in.tab.v1'; let tab = sessionStorage.getItem(key);
  if (tab !== null) routeBytes(tab, 32);
  else {tab = routeBase64(crypto.getRandomValues(new Uint8Array(32))); sessionStorage.setItem(key, tab);}
  const journal = secureBrowserPending<Pending>('portico.registered-sign-in.v1:' + tab);
  const installationId = () => {const current = installationIdentity(); if (!current) throw invalid(); return current.installationId;};
  const controller = new BrowserRegisteredSignIn({journal, sessions: browserAppSessions(), environment: browserConnectionEnvironment, startURL, installationId, pageOrigin: () => location.origin});
  return {controller, installationId};
}
