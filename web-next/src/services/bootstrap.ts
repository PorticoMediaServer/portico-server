import type {ServicesAccount} from './session';
import {restoreServicesLink, type LinkCapture, type ServicesLink, type ServicesLinkStore} from './intake';
import {hostedSignInStore, restoreHostedSignIn, type HostedSignIn, type SignInCapture} from './sign-in';

export type ServicesBootstrapSnapshot = Readonly<{phase: 'restoring' | 'ready' | 'error'; link?: ServicesLink; signIn?: HostedSignIn; invalidSignIn?: boolean; error?: unknown}>;
/** Account-only startup. The canonical cookie coordinator restores/renews its own family.
 * This root never constructs server bindings, loads a server or requests a library. */
export class ServicesBootstrap {
  private state: ServicesBootstrapSnapshot = {phase: 'restoring'};
  private readonly listeners = new Set<() => void>();
  private flight?: Promise<void>;
  private disposed = false;
  constructor(private readonly account: Pick<ServicesAccount, 'service'>,
    private readonly store: () => ServicesLinkStore, private readonly capture?: LinkCapture,
    private readonly signIn?: {origin: string; capture?: SignInCapture}) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(value: ServicesBootstrapSnapshot) { if (this.disposed) return; this.state = value; this.listeners.forEach(listener => listener()); }
  start = () => this.flight ??= this.restore().finally(() => { this.flight = undefined; });
  private async restore() {
    this.publish({phase: 'restoring'});
    try {
      // A captured token is already scrubbed. Persist it before any provider navigation.
      const link = await restoreServicesLink(this.store(), this.capture);
      let signIn: HostedSignIn | undefined;
      if (this.signIn) {
        try {signIn = await restoreHostedSignIn(hostedSignInStore(), this.signIn.origin, this.signIn.capture);}
        catch {this.publish({phase: 'ready', invalidSignIn: true}); return;}
      }
      if (this.disposed) return;
      await this.account.service.synchronize();
      if (this.disposed) return;
      if (this.account.service.getSnapshot().session) {
        try {await this.account.service.accessSession();}
        catch (error) {if (this.account.service.getSnapshot().session) throw error;}
      }
      if (!this.disposed) this.publish({phase: 'ready', link, signIn});
    } catch (error) { this.publish({phase: 'error', error}); }
  }
  dispose() { this.disposed = true; this.listeners.clear(); }
}
