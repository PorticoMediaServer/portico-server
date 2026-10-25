import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import {directSignIn, completeDirectSignInChallenge, type DirectSignIn, type DirectSnapshot} from '@core/profile-management.ts';
import {IdentityClient} from '@core/identity-client.ts';
import {porticoSignIn, type HostedRequestApi} from '@core/portico-servers.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import type {SignInKind} from '@core/app-sessions.ts';

type Grant = Extract<DirectSignIn, {kind: 'signed-in'}>;
type RecoveryState = Readonly<{phase: 'password' | 'portico' | 'challenge' | 'pin' | 'done'; busy?: boolean; error?: unknown}>;
/** A temporary sign-in proves one PIN reset. It never replaces the selected
 * server/viewer, enters the app session store or escapes this controller. */
export class BrowserProfileRecovery {
  private state: RecoveryState;
  private readonly listeners = new Set<() => void>();
  private epoch = 0;
  private controller?: AbortController;
  private grant?: Grant;
  private challenge?: Extract<DirectSignIn, {kind: 'challenge'}>;
  private disposed = false;
  constructor(private readonly api: HttpLocalApi, readonly snapshot: DirectSnapshot, readonly profileId: string, kind: SignInKind,
    private readonly authorityCurrent: () => boolean, private readonly temporaryAPI?: (session: LocalSession) => HttpLocalApi,
    private readonly openRecovered?: (pin: string, current: () => boolean) => Promise<void>) {
    if (!snapshot.profiles.some(profile => profile.id === profileId)) throw new Error('This profile is no longer available.');
    this.state = Object.freeze({phase: snapshot.canManage ? 'pin' : kind === 'local' ? 'password' : 'portico'});
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {this.listeners.add(listener); return () => {this.listeners.delete(listener);};};
  private publish(value: RecoveryState) {if (this.disposed) return; this.state = Object.freeze(value); for (const listener of this.listeners) listener();}
  private operation() {
    this.controller?.abort(); const epoch = ++this.epoch, controller = this.controller = new AbortController();
    return {signal: controller.signal, current: () => !this.disposed && this.epoch === epoch && !controller.signal.aborted && this.authorityCurrent()};
  }
  private temporary(session: LocalSession) {
    if (this.temporaryAPI) return this.temporaryAPI(session);
    const api = new HttpLocalApi(this.api.baseUrl, session.accessToken), route = this.api.getRouteConnection();
    if (route) api.useRoutes(route); return api;
  }
  private async retire(grant: Grant) {await this.temporary(grant.session).request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});}
  private async accept(result: DirectSignIn, current: () => boolean) {
    if (result.kind === 'challenge') {if (current()) {this.challenge = result; this.publish({phase: 'challenge'});} return;}
    if (!current() || result.snapshot.serverId !== this.snapshot.serverId || result.snapshot.account.id !== this.snapshot.account.id ||
      !result.snapshot.profiles.some(profile => profile.id === this.profileId) || !result.snapshot.capabilities.manageAccount || result.passwordChangeRequired || result.resetFileOwnerSession) {
      await this.retire(result); throw new Error('Sign in to the account that owns this profile.');
    }
    this.grant = result; this.publish({phase: 'pin'});
  }
  async password(password: string) {
    if (this.disposed || !this.authorityCurrent() || this.state.phase !== 'password' || !password || this.state.busy) return;
    const operation = this.operation(); this.publish({phase: 'password', busy: true});
    try {await this.accept(await directSignIn(this.api, this.snapshot.account.username, password, operation.signal), operation.current);}
    catch (error) {if (operation.current()) this.publish({phase: 'password', error});}
  }
  async portico(hosted: HostedRequestApi, origin: string, accountCurrent: () => boolean) {
    if (this.disposed || !this.authorityCurrent() || this.state.phase !== 'portico' || this.state.busy) return;
    const operation = this.operation(); this.publish({phase: 'portico', busy: true});
    try {await this.accept(await porticoSignIn(hosted, origin, this.api, this.snapshot.serverId, {signal: operation.signal}), () => operation.current() && accountCurrent());}
    catch (error) {if (operation.current()) this.publish({phase: 'portico', error});}
  }
  async code(code: string) {
    const challenge = this.challenge; if (this.disposed || !this.authorityCurrent() || this.state.phase !== 'challenge' || !challenge || this.state.busy) return;
    const operation = this.operation(); this.publish({phase: 'challenge', busy: true});
    try {await this.accept(await completeDirectSignInChallenge(this.api, challenge.token, code, operation.signal), operation.current);}
    catch (error) {if (operation.current()) this.publish({phase: 'challenge', error});}
  }
  async reset(pin: string) {
    if (this.disposed || !this.authorityCurrent() || this.state.phase !== 'pin' || !/^\d{4}$/.test(pin) || this.state.busy) return;
    const operation = this.operation(), grant = this.grant;
    this.publish({phase: 'pin', busy: true}); let completed = false;
    try {
      if (!operation.current()) return;
      const api = grant ? this.temporary(grant.session) : this.api;
      await new IdentityClient({request: (path, method, body) => api.request(path, method, body, operation.signal)}).resetPIN(this.profileId, pin);
      completed = true;
      if (operation.current()) this.publish({phase: 'done'});
    } catch (error) {if (operation.current()) this.publish({phase: 'pin', error});}
    finally {if (grant && (completed || !operation.current())) {this.grant = undefined; await this.retire(grant);}}
  }
  async openProfile(pin: string, current: () => boolean) {
    if (this.disposed || this.state.phase !== 'done' || !/^\d{4}$/.test(pin) || !this.authorityCurrent() || !current() || !this.openRecovered) return;
    await this.openRecovered(pin, current);
  }

  dispose() {
    if (this.disposed) return; this.disposed = true; ++this.epoch; this.controller?.abort(); this.challenge = undefined;
    const grant = this.grant; this.grant = undefined; if (grant) void this.retire(grant); this.listeners.clear();
  }
}
