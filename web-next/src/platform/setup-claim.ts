import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {type AppSessionStore, knownServerRoutes} from '@core/app-sessions.ts';
import {parseClaimStatus, type ClaimExpected} from '@core/claim-onboarding.ts';
import {installationIdentity} from '@core/installation.ts';
import {prepareSetupClaimHandoff, type SetupClaimHandoff} from '@core/setup-claim-client.ts';
import {verifySetupClaimStart} from '@core/setup-claim-start.ts';
import {resumeServerSetup, type SetupDraft, type SetupDraftStore} from '@core/setup-onboarding.ts';
import {parseDirectSignIn, type DirectSignIn} from '@core/profile-management.ts';
import {assertFirstContactIdentity, routeBase64, routeBytes, routeOrigin, sameServerPin, type ServerPin} from '@core/route-identity.ts';
import {connectPairedServer, connectPairedPorticoServer, rememberNativeSession, type ConnectionEnvironment} from '@core/server-connections.ts';
import {CodedError} from '@core/server-messages.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {dispatchPorticoReturn, type CapturedPorticoReturn} from './portico-return-dispatch';
import {secureBrowserPending} from './secure-pending';
import {BrowserProtectedStore, browserAppSessions, browserConnectionEnvironment} from './server-connections';

/** Reuse the original setup draft namespace. Passwords are never stored here. */
export const browserSetupDraft = (serverId: string): SetupDraftStore => new BrowserProtectedStore<SetupDraft>('portico.initialize.' + serverId);
type Pending = Readonly<{
  version: 1; kind: 'setup'; attemptId: string; origin: string; hostedOrigin: string; pin: ServerPin;
  expectedRevision: number; setupRequestId: string; setupToken: string; setupRevision: string;
  installationId: string; state: string; verifier: string; codeChallenge: string; createdAt: number; expiresAt: string;
  expected?: ClaimExpected; handoff?: SetupClaimHandoff;
}>;
type Journal = Readonly<{assertAvailable: () => void; read: () => Promise<Pending | undefined>;
  change: (update: (old: Pending | undefined) => Pending | undefined) => Promise<void>}>;
type Ports = Readonly<{
  journal: Journal; draft: (serverId: string) => SetupDraftStore; sessions: Pick<AppSessionStore, 'read' | 'connectionStorage'>;
  environment: (serverId: string) => ConnectionEnvironment; installationId: () => string; pageOrigin: () => string;
  now?: () => number; prepare?: typeof prepareSetupClaimHandoff;
  connect?: typeof connectPairedServer; connectPortico?: typeof connectPairedPorticoServer; remember?: typeof rememberNativeSession;
  retire?: (api: HttpLocalApi, session: LocalSession) => Promise<void>;
}>;
type Completion = Readonly<{api: HttpLocalApi; result: Extract<DirectSignIn, {kind: 'signed-in'}>}>;
const invalid = () => new CodedError('invalid_return', 'This setup continuation is no longer available.');
const cancelled = () => new CodedError('cancelled', 'The pending setup or server selection changed.');
const same = (a: Pending | undefined, b: Pending) => !!a && JSON.stringify(a) === JSON.stringify(b);
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

/** Initial setup has no Local owner or authorization family. Its only authority
 * is the original protected setup request/token and Server-supplied revision. */
export class BrowserSetupClaim {
  private epoch = 0;
  private controller?: AbortController;
  private attemptId?: string;
  private busy = false;
  constructor(private readonly ports: Ports) {}
  private now() {return (this.ports.now ?? Date.now)();}
  private operation() {
    if (this.busy) throw new CodedError('move_request_in_progress', 'Wait for this setup request to finish.');
    this.busy = true;
    this.controller?.abort(); const epoch = ++this.epoch, controller = this.controller = new AbortController();
    return {signal: controller.signal, current: () => epoch === this.epoch && !controller.signal.aborted};
  }
  private async current(record: Pending, operation: ReturnType<BrowserSetupClaim['operation']>, journal = true) {
    this.ports.journal.assertAvailable();
    if (journal && !same(await this.ports.journal.read(), record)) throw cancelled();
    const draft = await this.ports.draft(record.pin.serverId).read(), selection = await this.ports.sessions.read();
    if (!operation.current() || record.version !== 1 || record.kind !== 'setup' || record.origin !== this.ports.pageOrigin() ||
      record.installationId !== this.ports.installationId() || selection.revision !== record.expectedRevision ||
      !draft || draft.version !== 1 || draft.authMode !== 'hosted' || draft.serverId !== record.pin.serverId || draft.requestId !== record.setupRequestId || draft.setupToken !== record.setupToken ||
      !Number.isSafeInteger(record.createdAt) || record.createdAt < 0 || record.createdAt > this.now() + 60_000 || !Number.isFinite(Date.parse(record.expiresAt)) ||
      Date.parse(record.expiresAt) <= this.now() || Date.parse(record.expiresAt) > record.createdAt + 300_000) throw cancelled();
    return draft;
  }
  /** Retire only this browser attempt. It cannot undo an installed claim. */
  async cancel() {
    ++this.epoch; this.controller?.abort();
    const id = this.attemptId; this.attemptId = undefined;
    if (id) await this.ports.journal.change(old => old?.attemptId === id ? undefined : old);
  }
  async dispose() {await this.cancel();}
  async prepare(api: HttpLocalApi, hostedOrigin: string): Promise<string> {
    this.ports.journal.assertAvailable();
    const origin = this.ports.pageOrigin(), route = api.getRouteConnection();
    if (api.baseUrl !== origin || routeOrigin(origin) !== origin || !route || route.getSnapshot().phase !== 'ready' ||
      routeOrigin(hostedOrigin) !== hostedOrigin || !hostedOrigin.startsWith('https://')) throw invalid();
    const operation = this.operation();
    try {
      const draft = await this.ports.draft(route.pin.serverId).read(), selection = await this.ports.sessions.read();
      if (!operation.current() || !draft || draft.version !== 1 || draft.serverId !== route.pin.serverId || draft.authMode !== 'hosted' || !draft.setupToken) throw invalid();
      assertFirstContactIdentity(origin, route.pin, {knownRoutes: knownServerRoutes(selection)});
      const captured = new HttpLocalApi(origin, draft.setupToken); captured.useRoutes(route);
      const resumed = await resumeServerSetup(captured, this.ports.draft(route.pin.serverId), operation.signal);
      if (!('claimRequired' in resumed) || resumed.requestId !== draft.requestId) throw invalid();
      let pending = await this.ports.journal.read();
      if (!pending) {
        const verifier = routeBase64(crypto.getRandomValues(new Uint8Array(32))), createdAt = this.now();
        const codeChallenge = routeBase64(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))));
        pending = {version: 1, kind: 'setup', attemptId: routeBase64(crypto.getRandomValues(new Uint8Array(32))), origin, hostedOrigin, pin: {...route.pin},
          expectedRevision: selection.revision, setupRequestId: draft.requestId, setupToken: draft.setupToken, setupRevision: resumed.revision,
          installationId: this.ports.installationId(), state: routeBase64(crypto.getRandomValues(new Uint8Array(32))), verifier, codeChallenge,
          createdAt, expiresAt: new Date(createdAt + 300_000).toISOString()};
        const fresh = pending;
        await this.ports.journal.change(old => {if (!operation.current() || old) throw cancelled(); return fresh;});
      }
      if (pending.origin !== origin || pending.hostedOrigin !== hostedOrigin || !sameServerPin(pending.pin, route.pin) || pending.setupRequestId !== resumed.requestId || pending.setupRevision !== resumed.revision) throw invalid();
      this.attemptId = pending.attemptId;
      await this.current(pending, operation);
      if (!pending.expected) {
        let status = parseClaimStatus(await captured.request(routePath('get_networking_claim'), 'GET', undefined, operation.signal), pending.pin);
        await this.current(pending, operation);
        if (status.actions.includes('prepare')) {
          status = parseClaimStatus(await captured.request(routePath('post_networking_claim_prepare'), 'POST', {purpose: 'setup', expected: status.identity}, operation.signal), pending.pin);
          await this.current(pending, operation);
        }
        if (status.state !== 'prepared' || !status.operation || status.approvalRequest?.purpose !== 'setup' || status.approvalRequest.currentOwnerAccountId !== '' ||
          status.approvalRequest.previousOwnerRole !== '' || status.approvalRequest.returnRegistrationId !== '') throw invalid();
        const old = pending, updated = {...pending, expected: {...status.operation}};
        await this.ports.journal.change(current => {if (!operation.current() || !same(current, old)) throw cancelled(); return updated;});
        pending = updated;
      }
      const binding = {pin: pending.pin, origin, hostedOrigin, operationId: pending.expected!.operationId, localGeneration: pending.expected!.localGeneration,
        setupRequestId: pending.setupRequestId, setupRevision: pending.setupRevision, state: pending.state, codeChallenge: pending.codeChallenge, installationId: pending.installationId};
      return await (this.ports.prepare ?? prepareSetupClaimHandoff)(captured, pending.expected!, binding, {
        assertPendingCurrent: async () => {await this.current(pending!, operation);},
        bindPending: async handoff => {
          await this.current(pending!, operation);
          const old = pending!, updated = {...old, handoff, expiresAt: new Date(Math.min(Date.parse(old.expiresAt), Date.parse(handoff.expiresAt))).toISOString()};
          await this.ports.journal.change(current => {if (!operation.current() || !same(current, old)) throw cancelled(); return updated;});
          pending = updated;
        },
      }, {signal: operation.signal, now: () => this.now()});
    } finally {this.busy = false;}
  }
  async pendingReturn() {
    this.ports.journal.assertAvailable(); const record = await this.ports.journal.read();
    if (!record || record.kind !== 'setup' || !record.handoff) return;
    // A read-only probe must not create or consume an attempt.
    const epoch = this.epoch;
    try {await this.current(record, {signal: new AbortController().signal, current: () => epoch === this.epoch});} catch {
      await this.ports.journal.change(old => same(old, record) ? undefined : old); return;
    }
    this.attemptId = record.attemptId;
    return {kind: 'setup' as const, transactionId: record.handoff.transactionId};
  }
  private async retire(api: HttpLocalApi, session: LocalSession) {
    if (this.ports.retire) return this.ports.retire(api, session);
    const temporary = new HttpLocalApi(api.baseUrl, session.accessToken), routes = api.getRouteConnection();
    if (routes) temporary.useRoutes(routes);
    await temporary.request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});
  }
  async consume(captured: CapturedPorticoReturn) {return this.finish(captured);}
  /** A valid opaque callback consumes the protected attempt before network I/O.
   * A lost completion is terminal; ordinary sign-in recovers the installed owner. */
  async finish(captured: CapturedPorticoReturn): Promise<Completion> {
    this.ports.journal.assertAvailable(); const operation = this.operation();
    let connected: Awaited<ReturnType<typeof connectPairedServer>> | undefined, bound: Awaited<ReturnType<typeof connectPairedPorticoServer>> | undefined;
    let grant: DirectSignIn | undefined, adopted = false, admissionServer: string | undefined;
    try {
      const record = await this.ports.journal.read();
      if (record?.version !== 1 || record.kind !== 'setup' || !record.handoff || !record.expected) throw invalid();
      const handoff = record.handoff;
      const callback = dispatchPorticoReturn(captured, {kind: 'setup', transactionId: handoff.transactionId});
      if (new URL(callback.href).origin !== record.origin || record.origin !== this.ports.pageOrigin()) throw invalid();
      await this.ports.journal.change(old => {if (!operation.current() || !same(old, record)) throw cancelled(); return undefined;});
      this.attemptId = undefined;
      const current = () => this.current(record, operation, false);
      await current();
      const expected = handoff.binding;
      if (expected.origin !== record.origin || expected.hostedOrigin !== record.hostedOrigin || !sameServerPin(expected.pin, record.pin) || expected.state !== record.state ||
        expected.installationId !== record.installationId || expected.codeChallenge !== record.codeChallenge || expected.operationId !== record.expected.operationId ||
        expected.setupRequestId !== record.setupRequestId || expected.setupRevision !== record.setupRevision || expected.localGeneration !== record.expected.localGeneration ||
        JSON.stringify(handoff.expected) !== JSON.stringify(record.expected) || Date.parse(record.expiresAt) > Date.parse(handoff.expiresAt)) throw invalid();
      const proof = await verifySetupClaimStart(handoff.claim, handoff.start, expected, this.now());
      const digest = routeBase64(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(record.verifier))));
      routeBytes(record.verifier, 32);
      if (digest !== proof.start.codeChallenge || handoff.expiresAt !== proof.start.expiresAt) throw invalid();
      await current();
      const environment = this.ports.environment(record.pin.serverId);
      connected = await (this.ports.connect ?? connectPairedServer)(record.origin, record.pin, environment, operation.signal);
      await current(); connected.api.setAccessToken(record.setupToken);
      const raw = await connected.api.request<unknown>(routePath('get_networking_claim'), 'GET', undefined, operation.signal);
      await current();
      const status = parseClaimStatus(raw, record.pin), setup = object(raw) ? raw.setupContinuation : undefined;
      if (status.state !== 'installed' || status.identity.localGeneration !== record.expected.localGeneration || status.operation?.operationId !== record.expected.operationId ||
        status.operation.localGeneration !== record.expected.localGeneration || BigInt(status.operation.revision) < BigInt(record.expected.revision) || !object(setup) || setup.phase !== 'installed' || setup.transactionId !== handoff.transactionId ||
        Object.keys(setup).sort().join(',') !== 'expiresAt,phase,returnRegistration,transactionId' || typeof setup.expiresAt !== 'string' || Date.parse(setup.expiresAt) !== Date.parse(handoff.expiresAt) ||
        !object(setup.returnRegistration) || Object.keys(setup.returnRegistration).sort().join(',') !== 'id,origin' || typeof setup.returnRegistration.id !== 'string' ||
        !/^ret_[A-Za-z0-9_-]{43}$/.test(setup.returnRegistration.id) || typeof setup.returnRegistration.origin !== 'string' ||
        !setup.returnRegistration.origin.startsWith('https://') || routeOrigin(setup.returnRegistration.origin) !== setup.returnRegistration.origin) throw invalid();
      grant = parseDirectSignIn(await connected.api.request(routePath('post_auth_portico_return_exchange'), 'POST', {transactionId: handoff.transactionId, state: record.state,
        verifier: record.verifier, registrationId: setup.returnRegistration.id, installationId: record.installationId, serverChallenge: proof.start.serverChallenge, purpose: 'setup'}, operation.signal));
      await current();
      if (grant.kind !== 'signed-in' || grant.snapshot.serverId !== record.pin.serverId || grant.snapshot.account.role !== 'owner' || grant.snapshot.account.disabled ||
        !grant.snapshot.account.hostedAccountId || grant.session.viewer.profileId !== grant.snapshot.account.primaryProfileId || grant.resetFileOwnerSession || grant.passwordChangeRequired || grant.snapshot.twoStepSetupRequired) throw invalid();
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
        entry.connection.session.viewer.accountId !== grant.snapshot.account.id || entry.connection.hosted?.origin !== record.hostedOrigin || entry.connection.hosted.accountId !== accountId) throw cancelled();
      await this.ports.draft(record.pin.serverId).change(old => {
        if (!admissionCurrent() || !old || old.requestId !== record.setupRequestId || old.setupToken !== record.setupToken || old.authMode !== 'hosted') throw cancelled();
        return undefined;
      });
      adopted = true; return {api: bound.api, result: grant};
    } finally {
      this.busy = false;
      try {
        if (!adopted && grant?.kind === 'signed-in') {
          const family = grant.session.sessionFamilyId;
          if (admissionServer) await this.ports.sessions.connectionStorage(admissionServer).change(old => old?.session.sessionFamilyId === family ? undefined : old).catch(() => {});
          if (connected) await this.retire(connected.api, grant.session);
        }
      } finally {connected?.connection.dispose(); if (!adopted) bound?.connection.dispose();}
    }
  }
}

export function createBrowserSetupClaim() {
  const guard = secureBrowserPending<Pending>('portico.setup-claim.guard'); guard.assertAvailable();
  const key = 'portico.setup-claim.tab.v1'; let tab = sessionStorage.getItem(key);
  if (tab !== null) routeBytes(tab, 32);
  else {tab = routeBase64(crypto.getRandomValues(new Uint8Array(32))); sessionStorage.setItem(key, tab);}
  return new BrowserSetupClaim({journal: secureBrowserPending<Pending>('portico.setup-claim.v1:' + tab),
    draft: id => secureBrowserPending<SetupDraft>('portico.initialize.' + id), sessions: browserAppSessions(), environment: browserConnectionEnvironment,
    installationId: () => {const value = installationIdentity(); if (!value) throw invalid(); return value.installationId;}, pageOrigin: () => location.origin});
}
