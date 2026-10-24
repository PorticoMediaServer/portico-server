import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {type AppSessionRecord, type AppSessionStore, knownServerRoutes} from '@core/app-sessions.ts';
import {parseClaimStatus, type ClaimExpected} from '@core/claim-onboarding.ts';
import {installationIdentity} from '@core/installation.ts';
import {prepareOwnerClaimHandoff, type OwnerClaimHandoff} from '@core/owner-claim-client.ts';
import {verifyOwnerClaimStart} from '@core/owner-claim-start.ts';
import {parseDirectSignIn, type DirectSignIn} from '@core/profile-management.ts';
import {assertFirstContactIdentity, routeBase64, routeBytes, routeOrigin, sameServerPin, type ServerPin} from '@core/route-identity.ts';
import {connectPairedServer, connectPairedPorticoServer, rememberNativeSession, type ConnectionEnvironment} from '@core/server-connections.ts';
import {CodedError} from '@core/server-messages.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {dispatchPorticoReturn, type CapturedPorticoReturn} from './portico-return-dispatch';
import {secureBrowserPending} from './secure-pending';
import {browserAppSessions, browserConnectionEnvironment} from './server-connections';

type Pending = Readonly<{
  version: 1; kind: 'owner'; attemptId: string; origin: string; hostedOrigin: string; pin: ServerPin;
  expectedRevision: number; familyId: string; tokenGeneration: string; accountId: string; profileId: string;
  installationId: string; state: string; verifier: string; codeChallenge: string; createdAt: number; expiresAt: string;
  expected?: ClaimExpected; handoff?: OwnerClaimHandoff;
}>;
type Journal = Readonly<{assertAvailable: () => void; read: () => Promise<Pending | undefined>;
  change: (update: (old: Pending | undefined) => Pending | undefined) => Promise<void>}>;
type Ports = Readonly<{
  journal: Journal; sessions: Pick<AppSessionStore, 'read' | 'connectionStorage'>;
  environment: (serverId: string) => ConnectionEnvironment; installationId: () => string; pageOrigin: () => string;
  now?: () => number; prepare?: typeof prepareOwnerClaimHandoff;
  connect?: typeof connectPairedServer; connectPortico?: typeof connectPairedPorticoServer; remember?: typeof rememberNativeSession;
  retire?: (api: HttpLocalApi, session: LocalSession) => Promise<void>;
}>;
type Completion = Readonly<{api: HttpLocalApi; result: Extract<DirectSignIn, {kind: 'signed-in'}>}>;
const invalid = () => new CodedError('invalid_return', 'This owner continuation is no longer available.');
const cancelled = () => new CodedError('cancelled', 'The selected owner or server changed.');
const same = (a: Pending | undefined, b: Pending) => !!a && JSON.stringify(a) === JSON.stringify(b);
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

/** Separate owner KIND and encrypted journal. This never restores its narrowed
 * family with /me, renews it, or substitutes an ordinary member sign-in. The
 * caller confirms the existing move first; Server guards own its lifecycle. */
export class BrowserOwnerClaim {
  private epoch = 0;
  private controller?: AbortController;
  private attemptId?: string;
  private busy = false;
  constructor(private readonly ports: Ports) {}
  private now() {return (this.ports.now ?? Date.now)();}
  private operation() {
    if (this.busy) throw new CodedError('move_request_in_progress', 'Wait for this account move request to finish.');
    this.busy = true;
    this.controller?.abort(); const epoch = ++this.epoch, controller = this.controller = new AbortController();
    return {signal: controller.signal, current: () => epoch === this.epoch && !controller.signal.aborted};
  }
  private selected(state: AppSessionRecord, pin: ServerPin) {
    const selected = state.servers.find(entry => entry.connection.pin.serverId === state.selectedServerId);
    if (!selected || selected.signIn !== 'local' || selected.connection.hosted || !sameServerPin(selected.connection.pin, pin) ||
      selected.connection.session.viewer.authority !== 'local' || selected.connection.session.viewer.role !== 'owner') throw invalid();
    return selected.connection;
  }
  private async current(record: Pending, operation: ReturnType<BrowserOwnerClaim['operation']>, journal = true) {
    this.ports.journal.assertAvailable();
    if (journal && !same(await this.ports.journal.read(), record)) throw cancelled();
    const state = await this.ports.sessions.read();
    if (!operation.current()) throw cancelled();
    return this.captured(record, state);
  }
  private captured(record: Pending, state: AppSessionRecord) {
    const selected = this.selected(state, record.pin), session = selected.session;
    if (record.version !== 1 || record.kind !== 'owner' || record.origin !== this.ports.pageOrigin() ||
      record.installationId !== this.ports.installationId() || state.revision !== record.expectedRevision || session.sessionFamilyId !== record.familyId ||
      session.tokenGeneration !== record.tokenGeneration || session.viewer.accountId !== record.accountId || session.viewer.profileId !== record.profileId ||
      !Number.isSafeInteger(record.createdAt) || record.createdAt < 0 || record.createdAt > this.now() + 60_000 || !Number.isFinite(Date.parse(record.expiresAt)) ||
      Date.parse(record.expiresAt) <= this.now() || Date.parse(record.expiresAt) > record.createdAt + 300_000 || Date.parse(session.expiresAt) <= this.now()) throw cancelled();
    return selected;
  }
  /** Forget this browser attempt only. Server move cancellation remains the
   * existing explicit AccountMoveClient action; installed claims never roll back. */
  async cancel() {
    ++this.epoch; this.controller?.abort();
    const id = this.attemptId; this.attemptId = undefined;
    if (id) await this.ports.journal.change(old => old?.attemptId === id ? undefined : old);
  }
  async dispose() {await this.cancel();}
  /** Same-origin private preparation only, after the actual move confirmation.
   * Ambiguous network failures retain the exact verifier/proofs for that attempt. */
  async prepare(api: HttpLocalApi, hostedOrigin: string): Promise<string> {
    this.ports.journal.assertAvailable();
    const origin = this.ports.pageOrigin(), route = api.getRouteConnection();
    if (api.baseUrl !== origin || routeOrigin(origin) !== origin || !route || route.getSnapshot().phase !== 'ready' ||
      routeOrigin(hostedOrigin) !== hostedOrigin || !hostedOrigin.startsWith('https://')) throw invalid();
    const operation = this.operation();
    try {
      const state = await this.ports.sessions.read(), selected = this.selected(state, route.pin);
      if (!operation.current()) throw cancelled();
      assertFirstContactIdentity(origin, route.pin, {knownRoutes: knownServerRoutes(state)});
      let pending = await this.ports.journal.read();
      if (!pending) {
        const verifier = routeBase64(crypto.getRandomValues(new Uint8Array(32))), createdAt = this.now();
        const codeChallenge = routeBase64(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))));
        pending = {version: 1, kind: 'owner', attemptId: routeBase64(crypto.getRandomValues(new Uint8Array(32))), origin, hostedOrigin, pin: {...route.pin},
          expectedRevision: state.revision, familyId: selected.session.sessionFamilyId, tokenGeneration: selected.session.tokenGeneration,
          accountId: selected.session.viewer.accountId, profileId: selected.session.viewer.profileId, installationId: this.ports.installationId(),
          state: routeBase64(crypto.getRandomValues(new Uint8Array(32))), verifier, codeChallenge, createdAt, expiresAt: new Date(createdAt + 300_000).toISOString()};
        const fresh = pending;
        await this.ports.journal.change(old => {if (!operation.current() || old) throw cancelled(); return fresh;});
      }
      if (pending.origin !== origin || pending.hostedOrigin !== hostedOrigin || !sameServerPin(pending.pin, route.pin)) throw invalid();
      this.attemptId = pending.attemptId;
      await this.current(pending, operation);
      if (!pending.expected) {
        let status = parseClaimStatus(await api.request(routePath('get_networking_claim'), 'GET', undefined, operation.signal), pending.pin);
        await this.current(pending, operation);
        if (status.actions.includes('prepare')) {
          status = parseClaimStatus(await api.request(routePath('post_networking_claim_prepare'), 'POST', {purpose: 'switch', expected: status.identity}, operation.signal), pending.pin);
          await this.current(pending, operation);
        }
        if (status.state !== 'prepared' || !status.operation || status.approvalRequest?.purpose !== 'switch' || status.approvalRequest.currentOwnerAccountId !== '' ||
          status.approvalRequest.previousOwnerRole !== '' || status.approvalRequest.returnRegistrationId !== '') throw invalid();
        const old = pending, updated = {...pending, expected: {...status.operation}};
        await this.ports.journal.change(current => {if (!operation.current() || !same(current, old)) throw cancelled(); return updated;});
        pending = updated;
      }
      const binding = {pin: pending.pin, origin, hostedOrigin, operationId: pending.expected!.operationId, localGeneration: pending.expected!.localGeneration,
        state: pending.state, codeChallenge: pending.codeChallenge, installationId: pending.installationId};
      return await (this.ports.prepare ?? prepareOwnerClaimHandoff)(api, pending.expected!, binding, {
        assertPendingCurrent: async () => {await this.current(pending!, operation);},
        bindPending: async handoff => {
          await this.current(pending!, operation);
          // The Server challenge begins after our private journal was written.
          // Keep the earlier client deadline; never extend it to the later
          // signed expiry or change the producer's exact signed bytes.
          const old = pending!, updated = {...old, handoff, expiresAt: new Date(Math.min(Date.parse(old.expiresAt), Date.parse(handoff.expiresAt))).toISOString()};
          await this.ports.journal.change(current => {if (!operation.current() || !same(current, old)) throw cancelled(); return updated;});
          pending = updated;
        },
      }, {signal: operation.signal, now: () => this.now()});
    } finally {this.busy = false;}
  }
  /** Only protected KIND selects this consumer. The URL cannot select owner
   * authority and supplies no state, code, registration or credentials. */
  async pendingReturn() {
    this.ports.journal.assertAvailable(); const record = await this.ports.journal.read();
    if (!record || record.kind !== 'owner' || !record.handoff) return;
    const state = await this.ports.sessions.read();
    try {this.captured(record, state);} catch {
      await this.ports.journal.change(old => same(old, record) ? undefined : old); return;
    }
    this.attemptId = record.attemptId;
    return {kind: 'owner' as const, transactionId: record.handoff.transactionId};
  }
  private async retire(api: HttpLocalApi, session: LocalSession) {
    if (this.ports.retire) return this.ports.retire(api, session);
    const temporary = new HttpLocalApi(api.baseUrl, session.accessToken), routes = api.getRouteConnection();
    if (routes) temporary.useRoutes(routes);
    await temporary.request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});
  }
  /** One-shot exchange: lost response is terminal. Recovery is fresh ordinary
   * admission to the already-bound owner, never another claim or owner grant. */
  async finish(captured: CapturedPorticoReturn): Promise<Completion> {
    this.ports.journal.assertAvailable(); const operation = this.operation();
    let connected: Awaited<ReturnType<typeof connectPairedServer>> | undefined, bound: Awaited<ReturnType<typeof connectPairedPorticoServer>> | undefined;
    let grant: DirectSignIn | undefined, adopted = false, admissionServer: string | undefined;
    try {
      const record = await this.ports.journal.read();
      if (record?.version !== 1 || record.kind !== 'owner' || !record.handoff || !record.expected) throw invalid();
      const handoff = record.handoff, pending = record;
      const callback = dispatchPorticoReturn(captured, {kind: 'owner', transactionId: handoff.transactionId});
      if (new URL(callback.href).origin !== record.origin || record.origin !== this.ports.pageOrigin()) throw invalid();
      // A valid owner-only callback consumes this attempt before any proof,
      // network or admission await. Malformed/mixed callbacks above preserve
      // the legitimate pending; a successor can never be consumed here.
      await this.ports.journal.change(old => {if (!operation.current() || !same(old, pending)) throw cancelled(); return undefined;});
      this.attemptId = undefined;
      const current = () => this.current(record, operation, false);
      const selected = await current();
      const expected = handoff.binding;
      if (expected.origin !== record.origin || expected.hostedOrigin !== record.hostedOrigin || !sameServerPin(expected.pin, record.pin) || expected.state !== record.state ||
        expected.installationId !== record.installationId || expected.codeChallenge !== record.codeChallenge || expected.operationId !== record.expected.operationId ||
        expected.localGeneration !== record.expected.localGeneration || JSON.stringify(handoff.expected) !== JSON.stringify(record.expected) || Date.parse(record.expiresAt) > Date.parse(handoff.expiresAt)) throw invalid();
      const proof = await verifyOwnerClaimStart(handoff.claim, handoff.start, expected, this.now());
      const digest = routeBase64(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(record.verifier))));
      routeBytes(record.verifier, 32);
      if (digest !== proof.start.codeChallenge || handoff.expiresAt !== proof.start.expiresAt) throw invalid();
      await current();
      const environment = this.ports.environment(record.pin.serverId);
      connected = await (this.ports.connect ?? connectPairedServer)(record.origin, record.pin, environment, operation.signal);
      await current(); connected.api.setAccessToken(selected.session.accessToken);
      const raw = await connected.api.request<unknown>(routePath('get_networking_claim'), 'GET', undefined, operation.signal);
      await current();
      const status = parseClaimStatus(raw, record.pin), owner = object(raw) ? raw.ownerContinuation : undefined;
      if (status.state !== 'installed' || status.identity.localGeneration !== record.expected.localGeneration || status.operation?.operationId !== record.expected.operationId ||
        status.operation.localGeneration !== record.expected.localGeneration || BigInt(status.operation.revision) < BigInt(record.expected.revision) || !object(owner) || owner.phase !== 'installed' || owner.transactionId !== handoff.transactionId ||
        Object.keys(owner).sort().join(',') !== 'expiresAt,phase,returnRegistration,transactionId' || typeof owner.expiresAt !== 'string' || Date.parse(owner.expiresAt) !== Date.parse(handoff.expiresAt) ||
        !object(owner.returnRegistration) || Object.keys(owner.returnRegistration).sort().join(',') !== 'id,origin' || typeof owner.returnRegistration.id !== 'string' ||
        !/^ret_[A-Za-z0-9_-]{43}$/.test(owner.returnRegistration.id) || typeof owner.returnRegistration.origin !== 'string' ||
        !owner.returnRegistration.origin.startsWith('https://') || routeOrigin(owner.returnRegistration.origin) !== owner.returnRegistration.origin) throw invalid();
      grant = parseDirectSignIn(await connected.api.request(routePath('post_auth_portico_return_exchange'), 'POST', {transactionId: handoff.transactionId, state: record.state,
        verifier: record.verifier, registrationId: owner.returnRegistration.id, installationId: record.installationId, serverChallenge: proof.start.serverChallenge, purpose: 'switch'}, operation.signal));
      await current();
      if (grant.kind !== 'signed-in' || grant.snapshot.serverId !== record.pin.serverId || grant.snapshot.account.id !== record.accountId || grant.snapshot.account.primaryProfileId !== record.profileId ||
        grant.snapshot.account.role !== 'owner' || grant.snapshot.account.disabled || !grant.snapshot.account.hostedAccountId || grant.resetFileOwnerSession || grant.passwordChangeRequired ||
        grant.snapshot.twoStepSetupRequired || grant.session.sessionFamilyId === record.familyId) throw invalid();
      const accountId = grant.snapshot.account.hostedAccountId;
      bound = await (this.ports.connectPortico ?? connectPairedPorticoServer)(record.origin, record.pin,
        {origin: record.hostedOrigin, accountId, request: async () => {throw invalid();}}, environment, operation.signal);
      await current(); bound.api.setAccessToken(grant.session.accessToken);
      const admissionCurrent = () => operation.current() && this.now() < Date.parse(record.expiresAt) && record.origin === this.ports.pageOrigin() && record.installationId === this.ports.installationId();
      admissionServer = record.pin.serverId;
      await (this.ports.remember ?? rememberNativeSession)(bound.api, grant.session,
        {...environment, storage: this.ports.sessions.connectionStorage(record.pin.serverId, {signIn: 'portico', hostedAccount: bound.hosted, expectedRevision: record.expectedRevision})}, bound.hosted, admissionCurrent);
      const stored = await this.ports.sessions.read(), entry = stored.servers.find(value => value.connection.pin.serverId === stored.selectedServerId);
      if (!admissionCurrent() || !entry || entry.signIn !== 'portico' || !sameServerPin(entry.connection.pin, record.pin) || entry.connection.session.sessionFamilyId !== grant.session.sessionFamilyId ||
        entry.connection.session.viewer.accountId !== record.accountId || entry.connection.hosted?.origin !== record.hostedOrigin || entry.connection.hosted.accountId !== accountId) throw cancelled();
      adopted = true; return {api: bound.api, result: grant};
    } finally {
      this.busy = false;
      try {
        if (!adopted && grant?.kind === 'signed-in') {
          const family = grant.session.sessionFamilyId;
          if (admissionServer) await this.ports.sessions.connectionStorage(admissionServer).change(old => old?.session.sessionFamilyId === family ? undefined : old).catch(() => {});
          if (connected) await this.retire(connected.api, grant.session);
        }
      }
      finally {connected?.connection.dispose(); if (!adopted) bound?.connection.dispose();}
    }
  }
}

/** No tab locator, DB or private pending is touched before secure prerequisites. */
export function createBrowserOwnerClaim() {
  const guard = secureBrowserPending<Pending>('portico.owner-claim.guard'); guard.assertAvailable();
  const key = 'portico.owner-claim.tab.v1'; let tab = sessionStorage.getItem(key);
  if (tab !== null) routeBytes(tab, 32);
  else {tab = routeBase64(crypto.getRandomValues(new Uint8Array(32))); sessionStorage.setItem(key, tab);}
  return new BrowserOwnerClaim({journal: secureBrowserPending<Pending>('portico.owner-claim.v1:' + tab), sessions: browserAppSessions(), environment: browserConnectionEnvironment,
    installationId: () => {const value = installationIdentity(); if (!value) throw invalid(); return value.installationId;}, pageOrigin: () => location.origin});
}
