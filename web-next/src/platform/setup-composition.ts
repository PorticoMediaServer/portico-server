import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {parseLocalSession} from '@core/local-session.ts';
import {parseDirectSnapshot} from '@core/profile-management.ts';
import {initializeServer, type PendingAccountSetup, type SetupDraft, type SetupDraftStore} from '@core/setup-onboarding.ts';
import type {ServerSignIn} from '@core/sign-in.ts';
import {sameServerPin, type ServerPin} from '@core/route-identity.ts';
import {rememberNativeSession} from '@core/server-connections.ts';
import {installationIdentity} from '@core/installation.ts';
import {CodedError} from '@core/server-messages.ts';
import {decodePostLibrariesResponse} from '../../../packages/contracts/src/decoders.ts';
import {routeBody, routePath, encodedRoutePath} from '../../../packages/contracts/src/routes.ts';
import {browserAppSessions, browserConnectionEnvironment, restoreBrowserConnection} from './server-connections';
import {createBrowserSetupClaim, type BrowserSetupClaim} from './setup-claim';
import {secureBrowserPending} from './secure-pending';

export type SetupTarget = Readonly<{server: ServerSignIn; api: HttpLocalApi; pin: ServerPin; draft: SetupDraftStore; dispose: () => void; admittedOwner?: true}>;
const changed = () => new CodedError('cancelled', 'The pending setup or selected server changed.');

/** The screen owns form state; this lifetime owns real credential adoption and
 * the initialized owner's API. No setup bearer may create a library. */
export class BrowserSetupComposition {
  private active = true;
  private readonly abort = new AbortController();
  private readonly captures = new WeakMap<SetupTarget, {revision: number; installationId: string}>();
  private owner?: {api: HttpLocalApi; session: LocalSession};
  private claim?: BrowserSetupClaim;
  private handoffDelivered = false;
  constructor(private readonly hostedOrigin: string, private readonly navigate: (url: string) => void = url => location.assign(url)) {}
  private current() {if (!this.active || this.abort.signal.aborted) throw changed();}
  private install() {const value = installationIdentity(); if (!value) throw changed(); return value.installationId;}
  private async assert(server: SetupTarget) {
    this.current(); const capture = this.captures.get(server);
    if (!capture || capture.installationId !== this.install() || !sameServerPin(server.pin, server.api.getRouteConnection()?.pin ?? {serverId: '', publicKey: '', fingerprint: ''})) throw changed();
    const selected = await browserAppSessions().read(); this.current();
    if (selected.revision !== capture.revision) throw changed();
    return capture;
  }
  /** Capture before the first pairing await. An unrelated selection cannot be
   * overwritten by a late setup response, including a completed-server resume. */
  connect = async (connect: (signal: AbortSignal) => Promise<SetupTarget>, signal: AbortSignal): Promise<SetupTarget> => {
    this.current(); const state = await browserAppSessions().read(), install = this.install(); this.current();
    const server = await connect(AbortSignal.any([signal, this.abort.signal]));
    try {
      this.current(); const current = await browserAppSessions().read(); this.current();
      if (current.revision !== state.revision || install !== this.install()) throw changed();
      if (server.server.setupRequired) {
        this.captures.set(server, {revision: current.revision, installationId: install}); return server;
      }
      // A callback resumes auth3 only through a genuine saved owner session.
      const selected = current.servers.find(value => value.connection.pin.serverId === current.selectedServerId);
      if (!selected || !sameServerPin(selected.connection.pin, server.pin) || selected.connection.session.viewer.role !== 'owner') throw changed();
      const restored = await restoreBrowserConnection(selected.connection.session); this.current();
      if (!restored) throw changed();
      try {
        const after = await browserAppSessions().read(); this.current();
        const live = after.servers.find(value => value.connection.pin.serverId === after.selectedServerId);
        if (!live || !sameServerPin(live.connection.pin, server.pin) || live.connection.session.sessionFamilyId !== restored.session.sessionFamilyId || restored.session.viewer.role !== 'owner') throw changed();
        await this.ownerSnapshot(restored.api, restored.session); this.current();
        const target: SetupTarget = {...server, api: restored.api, admittedOwner: true, dispose: () => {server.dispose(); restored.api.getRouteConnection()?.dispose();}};
        this.captures.set(target, {revision: after.revision, installationId: install});
        this.owner = restored; return target;
      } catch (error) {restored.api.getRouteConnection()?.dispose(); throw error;}
    } catch (error) {server.dispose(); throw error;}
  };
  private async ownerSnapshot(api: HttpLocalApi, session: LocalSession) {
    if (session.viewer.role !== 'owner') throw changed();
    const snapshot = parseDirectSnapshot(await api.request(routePath('get_direct_account'), 'GET', undefined, this.abort.signal),
      {authority: 'local', serverId: session.viewer.serverId, accountId: session.viewer.accountId}, true);
    if (snapshot.account.role !== 'owner' || snapshot.account.disabled || !snapshot.capabilities.manageServer || snapshot.account.primaryProfileId !== session.viewer.profileId) throw changed();
    return snapshot;
  }
  acceptLocal = async (server: SetupTarget, value: LocalSession): Promise<void> => {
    const capture = await this.assert(server), session = parseLocalSession(value, {serverId: server.pin.serverId, authority: 'local'});
    if (session.viewer.role !== 'owner' || Date.parse(session.expiresAt) <= Date.now()) throw changed();
    const draft = await server.draft.read(); await this.assert(server);
    if (!draft || draft.authMode !== 'local' || draft.serverId !== server.pin.serverId) throw changed();
    const sessions = browserAppSessions(), environment = browserConnectionEnvironment(server.pin.serverId);
    let adopted = false;
    try {
      await rememberNativeSession(server.api, session, {...environment, storage: sessions.connectionStorage(server.pin.serverId,
        {signIn: 'local', expectedRevision: capture.revision})}, undefined, () => this.active && this.install() === capture.installationId);
      this.current();
      const selected = await sessions.selected(); this.current();
      if (!selected || selected.signIn !== 'local' || !sameServerPin(selected.connection.pin, server.pin) || selected.connection.session.sessionFamilyId !== session.sessionFamilyId) throw changed();
      await this.ownerSnapshot(server.api, session); this.current();
      await server.draft.change(old => {
        this.current(); if (old?.requestId !== draft.requestId || old.setupToken !== draft.setupToken || old.authMode !== 'local') throw changed(); return undefined;
      });
      this.owner = {api: server.api, session}; adopted = true;
    } finally {
      if (!adopted) {
        await sessions.connectionStorage(server.pin.serverId).change(old => old?.session.sessionFamilyId === session.sessionFamilyId ? undefined : old).catch(() => {});
        const retire = new HttpLocalApi(server.api.baseUrl, session.accessToken), route = server.api.getRouteConnection(); if (route) retire.useRoutes(route);
        await retire.request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});
      }
    }
  };
  continueHosted = async (server: SetupTarget, input: Readonly<{name: string}>): Promise<void> => {
    // Require secure/encrypted atomic storage before acquiring or saving the secret.
    const claim = this.claim ??= createBrowserSetupClaim();
    const draft = secureBrowserPending<SetupDraft>('portico.initialize.' + server.pin.serverId); draft.assertAvailable();
    await this.assert(server);
    const old = await draft.read(); await this.assert(server);
    const issue = old ? {serverId: old.serverId, setupToken: old.setupToken} : await server.api.request<{serverId: string; setupToken: string}>(routePath('start_browser_setup'), 'POST', {}, this.abort.signal);
    await this.assert(server);
    if (issue.serverId !== server.pin.serverId || typeof issue.setupToken !== 'string' || !issue.setupToken) throw changed();
    const pending = await initializeServer(server.api, draft, {setupToken: issue.setupToken, name: input.name, authMode: 'hosted', interactive: true}, this.abort.signal);
    await this.assert(server);
    if (!('claimRequired' in pending) || pending.requestId !== (await draft.read())?.requestId) throw changed();
    await this.assert(server);
    const url = await claim.prepare(server.api, this.hostedOrigin); await this.assert(server);
    this.handoffDelivered = true; this.navigate(url);
  };
  resumeHosted = async (server: SetupTarget, pending: PendingAccountSetup): Promise<void> => {
    const claim = this.claim ??= createBrowserSetupClaim();
    await this.assert(server);
    const draft = await server.draft.read(); await this.assert(server);
    if (!draft || pending.serverId !== server.pin.serverId || pending.requestId !== draft.requestId || draft.authMode !== 'hosted') throw changed();
    const url = await claim.prepare(server.api, this.hostedOrigin); await this.assert(server);
    this.handoffDelivered = true; this.navigate(url);
  };
  addLibrary = async (input: Readonly<{kind: string; name: string; path: string}>): Promise<void> => {
    this.current(); const owner = this.owner; if (!owner) throw changed();
    const assert = async () => {
      this.current(); const selected = await browserAppSessions().selected(); this.current();
      if (!selected || selected.connection.session.sessionFamilyId !== owner.session.sessionFamilyId || !sameServerPin(selected.connection.pin, owner.api.getRouteConnection()!.pin) || selected.connection.session.viewer.role !== 'owner') throw changed();
      owner.api.setAccessToken(selected.connection.session.accessToken);
    };
    await assert(); await this.ownerSnapshot(owner.api, owner.session); await assert();
    const library = decodePostLibrariesResponse(await owner.api.request(routePath('post_libraries'), 'POST', routeBody('post_libraries', {kind: input.kind, name: input.name, path: input.path}), this.abort.signal));
    await assert();
    await owner.api.request(encodedRoutePath('post_libraries_id_scans', {id: encodeURIComponent(library.id)}), 'POST', {}, this.abort.signal);
    await assert();
  };
  dispose = () => {
    if (!this.active) return; this.active = false; this.abort.abort(); this.owner = undefined;
    if (!this.handoffDelivered) void this.claim?.cancel().catch(() => {});
  };
}
