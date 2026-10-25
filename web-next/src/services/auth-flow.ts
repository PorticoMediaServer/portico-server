import {AccountAttemptIds, MFARequiredError, type IdentityOptions, type IdentityProvider, type IdentityTransaction, type MFAChallenge, type Registration} from '@core/hosted-security.ts';
import {routeBody, routePath} from '../../../packages/contracts/src/hosted/routes.ts';
import {accountIdentity, currentAccountWork, sameAccount, type AccountIdentity, type ServicesAccount} from './session';

export type AuthMode = 'signin' | 'create' | 'verify' | 'forgot' | 'sent' | 'factor' | 'recovery' | 'provider' | 'provider-stopped' | 'ready';
export type AuthSnapshot = Readonly<{mode: AuthMode; options?: IdentityOptions; registration?: Registration; provider?: IdentityProvider; suggestedName?: string; verifiedEmail?: string; busy: boolean; retryAt: number; resends: number; error?: unknown}>;
/** Local form controller. The singleton CredentialSessionService owns every account session. */
export class ServicesAuthFlow {
  private state: AuthSnapshot;
  private listeners = new Set<() => void>();
  private operation?: AbortController;
  private initialized?: Promise<void>;
  private disposed = false;
  private challenge?: MFAChallenge;
  private transaction?: IdentityTransaction;
  private readyIdentity?: AccountIdentity;
  private ids = new AccountAttemptIds(async () => crypto.randomUUID());
  constructor(private readonly account: ServicesAccount, private readonly signedIn: (gesture?: boolean) => void,
    mode: 'signin' | 'create' | 'forgot' = 'signin', private readonly redirect = (url: string) => location.assign(url)) {
    this.state = {mode, busy: false, retryAt: 0, resends: 0};
  }
  getSnapshot = () => this.state;
  /** Presentation reads the canonical session only while its accepted family is current. */
  readyAccount() {
    if (this.state.mode !== 'ready' || !this.readyIdentity) return;
    const session = this.account.service.getSnapshot().session;
    if (!session?.familyId || !sameAccount(this.readyIdentity, accountIdentity(session))) return;
    return {name: session.account.displayName, username: session.account.username};
  }
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(value: Partial<AuthSnapshot>) { if (this.disposed) return; this.state = {...this.state, ...value}; for (const listener of this.listeners) listener(); }
  dispose() { this.disposed = true; this.operation?.abort(); this.listeners.clear(); this.challenge = undefined; this.transaction = undefined; this.ids.clear(); }
  go(mode: AuthMode) { if (this.operation) return; this.challenge = undefined; this.publish({mode, error: undefined, retryAt: 0}); }
  recovery() { if (!this.operation && this.challenge) this.publish({mode: this.state.mode === 'factor' ? 'recovery' : 'factor', error: undefined}); }
  startAgain() { if (!this.operation) { this.publish({registration: undefined, resends: 0}); this.go('create'); } }
  initialize() { return this.initialized ??= this.load(); }
  private async load() {
    await this.run(async signal => {
      if (this.state.mode !== 'signin') { this.publish({options: await this.account.client.options(signal)}); return; }
      const [options, registration, transaction] = await Promise.allSettled([
        this.account.client.options(signal), this.account.client.registration(undefined, signal), this.account.client.transaction(undefined, signal),
      ]);
      if (signal.aborted) return;
      if (options.status === 'fulfilled') this.publish({options: options.value}); else throw options.reason;
      if (transaction.status === 'fulfilled' && transaction.value?.mode === 'login') {
        const tx = transaction.value;
        if (tx.status === 'failed') {
          // The current browser-bound transaction is terminal. It proves no
          // email/account collision and cannot be completed or adopted.
          this.transaction = undefined; this.ids.clear('provider');
          this.publish({mode: 'provider-stopped', provider: tx.provider}); return;
        }
        if (tx.needsUsername && tx.contactVerified) {
          this.transaction = tx; this.publish({mode: 'provider', provider: tx.provider, suggestedName: tx.suggestedName, verifiedEmail: tx.verifiedEmail}); return;
        }
        if (tx.status === 'verified' && !tx.needsUsername) {
          await this.authenticate(inner => this.account.client.complete(tx.transactionId, {}, AbortSignal.any([inner, signal]))); return;
        }
      }
      if (registration.status === 'fulfilled' && registration.value && ['contact_pending', 'verified'].includes(registration.value.status))
        this.publish({mode: 'verify', registration: registration.value});
    });
  }
  private async run(work: (signal: AbortSignal) => Promise<void>) {
    if (this.disposed || this.operation || this.state.retryAt > Date.now()) return;
    const controller = new AbortController(); this.operation = controller; this.publish({busy: true, error: undefined});
    try { await work(controller.signal); }
    catch (error) {
      if (!controller.signal.aborted) {
        if (error instanceof MFARequiredError) { this.challenge = error.challenge; this.publish({mode: 'factor'}); }
        else { const seconds = (error as {retryAfterSeconds?: unknown})?.retryAfterSeconds; this.publish({error, retryAt: typeof seconds === 'number' && Number.isFinite(seconds) && seconds > 0 ? Date.now() + seconds * 1000 : 0}); }
      }
    } finally { if (this.operation === controller) this.operation = undefined; if (!controller.signal.aborted) this.publish({busy: false}); }
  }
  private async authenticate(work: Parameters<ServicesAccount['service']['authenticateIfSignedOut']>[0], gesture = false) {
    await this.account.service.authenticateIfSignedOut(work);
    this.ids.clear();
    if (!this.disposed) this.signedIn(gesture);
  }
  signIn(username: string, password: string, code: string) {
    return this.run(async signal => {
      if (this.challenge) { const challenge = this.challenge; await this.authenticate(inner => this.account.client.mfa(challenge, code.trim(), AbortSignal.any([inner, signal])), true); }
      else { const id = await this.ids.for('login', {username, password}); await this.authenticate(inner => this.account.client.login(username.trim(), password, id, AbortSignal.any([inner, signal])), true); }
    });
  }
  provider(provider: IdentityProvider) {
    return this.run(async signal => {
      if (this.state.mode === 'provider-stopped') this.ids.clear('provider');
      const value = await this.account.client.start(provider, {mode: 'login', requestId: await this.ids.for('provider', {provider})}, signal);
      if (value.authorizationUrl && !signal.aborted) this.redirect(value.authorizationUrl);
    });
  }
  cancelProvider() {
    return this.run(async signal => {
      await this.account.client.request(routePath('identity_cancel'), 'POST', undefined, signal);
      if (!signal.aborted) {
        this.transaction = undefined; this.ids.clear('provider');
        this.publish({mode: 'signin', provider: undefined, suggestedName: undefined, verifiedEmail: undefined});
      }
    });
  }
  create(input: {username: string; displayName: string; contactAddress: string; password: string}) {
    return this.run(async signal => {
      const value = await this.account.client.register({...input, username: input.username.trim().toLowerCase(), displayName: input.displayName.trim() || input.username.trim(), contactAddress: input.contactAddress.trim(), requestId: await this.ids.for('registration', input)}, signal);
      if (!signal.aborted) { this.ids.clear('registration'); this.publish({mode: 'verify', registration: value, resends: 0}); }
    });
  }
  verify(code: string) { return this.run(async signal => { const registration = this.state.registration; if (registration) await this.authenticate(inner => this.account.client.verifyRegistration(registration.registrationId, {code: code.trim()}, AbortSignal.any([signal, inner])), true); }); }
  resend() {
    return this.run(async signal => {
      const registration = this.state.registration; if (!registration || this.state.resends >= 5) return;
      const value = await this.account.client.resendRegistration(registration.registrationId, await this.ids.for('resend', {id: registration.registrationId, count: this.state.resends}), signal);
      if (!signal.aborted) this.publish({registration: value, resends: this.state.resends + 1});
    });
  }
  recover(email: string) {
    return this.run(async signal => {
      await this.account.client.request(routePath('account_recovery_request'), 'POST', routeBody('account_recovery_request', {username: '', contactAddress: email.trim(), requestId: await this.ids.for('recover', {email})}), signal);
      if (!signal.aborted) this.publish({mode: 'sent'});
    });
  }
  finishProvider(username: string, displayName: string) {
    return this.run(async signal => {
      const transaction = this.transaction; if (!transaction?.needsUsername || !transaction.contactVerified) return;
      const accepted = await this.account.service.authenticateIfSignedOut(inner => this.account.client.complete(transaction.transactionId,
        {username: username.trim(), displayName: displayName.trim(), ...(transaction.onboardingToken ? {onboardingToken: transaction.onboardingToken} : {})}, AbortSignal.any([inner, signal])));
      if (!signal.aborted) { this.readyIdentity = accountIdentity(accepted); this.transaction = undefined; this.publish({mode: 'ready'}); }
    });
  }
  continue() {
    return this.run(async () => {
      if (this.state.mode !== 'ready' || !this.readyIdentity) return;
      await currentAccountWork(this.account, async () => undefined, this.readyIdentity);
      if (!this.disposed) this.signedIn(true);
    });
  }
}
