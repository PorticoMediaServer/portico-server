import {AccountMoveClient, type AccountMove, type AccountMoveConfirmation, type MoveClientOptions} from '@core/account-move.ts';
import {HttpLocalApi, type LocalSession} from '@core/index.ts';
import type {DirectSignIn} from '@core/profile-management.ts';
import type {SignInKind} from '@core/app-sessions.ts';
import {routePath} from '../../../../packages/contracts/src/routes.ts';
import {RouteError, routeOrigin} from '@core/route-identity.ts';
import {createBrowserOwnerClaim, type BrowserOwnerClaim} from '../../platform/owner-claim';
import {ownerSwitchContextAllowed} from '../../platform/secure-pending';

type Claim = Pick<BrowserOwnerClaim, 'prepare' | 'cancel' | 'cancelOperation'>;
type Snapshot = Readonly<{busy: boolean; preview?: AccountMove; confirmed: boolean}>;
const empty: Snapshot = Object.freeze({busy: false, confirmed: false});
export type OwnerSwitchOptions = Readonly<{
  serverId: string; hostedOrigin: string; isCurrent: () => boolean;
  retireLateGrant?: MoveClientOptions['retireLateGrant'];
  secureContextAllowed?: () => boolean; createClaim?: () => Claim;
}>;
const ended = () => new DOMException('The owner switch ended.', 'AbortError');

/** Initiation only. Return routing and the helper's already-admitted result belong to Entry. */
export class SettingsOwnerSwitch {
  private readonly moves: AccountMoveClient;
  private claim?: Claim;
  private state = empty;
  private readonly listeners = new Set<() => void>();
  private request?: AbortController;
  private epoch = 0;
  private closed = false;
  private delivered = false;
  private readonly origin: string;
  private readonly routes: ReturnType<HttpLocalApi['getRouteConnection']>;
  constructor(private readonly api: HttpLocalApi, private readonly options: OwnerSwitchOptions) {
    this.origin = api.baseUrl;this.routes = api.getRouteConnection();
    this.moves = new AccountMoveClient(api, {serverId: options.serverId, isCurrent: this.current, retireLateGrant: this.retire});
  }
  private retire = async (session: LocalSession) => {
    if (this.options.retireLateGrant) return this.options.retireLateGrant(session);
    const temporary = new HttpLocalApi(this.origin, session.accessToken);
    if (this.routes) temporary.useRoutes(this.routes);
    await temporary.request(routePath('delete_sessions_current'), 'DELETE', undefined, AbortSignal.timeout(5000)).catch(() => {});
  };
  private current = () => !this.closed && this.options.isCurrent();
  getSnapshot = () => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: Snapshot) {if (this.current()) {this.state = Object.freeze(state);for (const listener of this.listeners) listener();}}
  private guard() {
    if (!this.current()) throw ended();
    if (!(this.options.secureContextAllowed ?? ownerSwitchContextAllowed)()) throw new RouteError('secure_context_required', 'This continuation requires a secure browser context.');
    if (!this.options.hostedOrigin.startsWith('https:') || routeOrigin(this.options.hostedOrigin) !== this.options.hostedOrigin) throw new Error('Invalid owner handoff origin.');
  }
  private operation() {
    this.guard();
    if (this.state.busy) throw new Error('An owner switch request is already running.');
    // The real factory checks encryption, IDB and native locks before even a tab locator.
    this.claim ??= (this.options.createClaim ?? createBrowserOwnerClaim)();
    const epoch = ++this.epoch, request = this.request = new AbortController();
    this.publish({...this.state, busy: true});
    return {signal: request.signal, check: () => {this.guard();if (request.signal.aborted || epoch !== this.epoch) throw ended();},
      finish: () => {if (this.current() && epoch === this.epoch) this.publish({...this.state, busy: false});}};
  }
  /** The drawn Change group has both directions. The target is the viewer's choice,
   * never assumed: local and Portico moves use the same captured preview/confirm
   * controller and the same currentness guards. */
  async previewDirection(to: SignInKind, startOver: boolean): Promise<AccountMove> {
    if (this.state.preview || this.state.confirmed) throw new Error('Close the current owner switch first.');
    if (to !== 'local' && to !== 'portico') throw new Error('Invalid sign-in direction.');
    const operation = this.operation();
    try {
      const preview = await this.moves.preview(to, startOver, operation.signal);operation.check();
      if (preview.to !== to || preview.from === to || preview.stage !== 'preview' || preview.startOver !== startOver) throw new Error('The owner move changed.');
      this.publish({busy: true, confirmed: false, preview});return preview;
    } finally {operation.finish();}
  }
  async preview(startOver: boolean): Promise<AccountMove> { return this.previewDirection('portico', startOver); }
  /** A move to local ends in the server's own signed-in grant. This returns it to
   * the caller for the app's real admission path; it never stores or adopts a
   * session here, and it never skips the revision, PIN or remembered-server rules
   * that admission already applies. */
  async confirmLocal(confirmation: string): Promise<Extract<DirectSignIn, {kind: 'signed-in'}>> {
    const preview = this.state.preview;
    if (!preview || preview.to !== 'local' || this.state.confirmed) throw new Error('A fresh local move preview is required.');
    const operation = this.operation();
    try {
      const result: AccountMoveConfirmation = await this.moves.confirm({moveId: preview.id, expectedRevision: preview.revision, confirmation, username: '', password: ''}, operation.signal);
      operation.check();
      if (result.kind !== 'local' || result.signIn.kind !== 'signed-in') throw new Error('Unexpected owner move continuation.');
      if (result.signIn.snapshot.serverId !== preview.serverId) throw new Error('The owner move changed.');
      this.publish({busy: false, confirmed: false});
      return result.signIn;
    } finally {operation.finish();}
  }
  async confirmAndPrepare(confirmation: string): Promise<string> {
    const preview = this.state.preview;
    if (!preview || this.state.confirmed) throw new Error('A fresh owner move preview is required.');
    const operation = this.operation();
    try {
      const result = await this.moves.confirm({moveId: preview.id, expectedRevision: preview.revision, confirmation, username: '', password: ''}, operation.signal);
      if (result.kind !== 'portico') {
        if (result.signIn.kind === 'signed-in') await this.retire(result.signIn.session);
        throw new Error('Unexpected owner move continuation.');
      }
      operation.check();const move = result.move;
      if (move.id !== preview.id || move.revision !== preview.revision || move.from !== 'local' || move.to !== 'portico' || move.stage !== 'preview' || move.startOver !== preview.startOver) throw new Error('The owner move changed.');
      this.publish({busy: true, preview: move, confirmed: true});
      return await this.prepare(operation);
    } finally {operation.finish();}
  }
  async retryPreparation(): Promise<string> {
    if (!this.state.confirmed) throw new Error('The owner move has not been confirmed.');
    const operation = this.operation();
    try {return await this.prepare(operation);} finally {operation.finish();}
  }
  private async prepare(operation: ReturnType<SettingsOwnerSwitch['operation']>): Promise<string> {
    operation.check();const url = await this.claim!.prepare(this.api, this.options.hostedOrigin);
    try {operation.check();} catch (error) {await this.claim!.cancel();throw error;}
    this.delivered = true;return url;
  }
  async cancelPreview() {
    const preview = this.state.preview;
    if (!preview || this.state.confirmed) throw new Error('Only an unconfirmed move preview can be canceled.');
    const operation = this.operation();
    try {await this.moves.cancelPreview(preview, operation.signal);operation.check();await this.cancel();}
    finally {operation.finish();}
  }
  async cancelOperation() {
    const operation = this.operation();
    try {await this.claim!.cancelOperation(this.api);operation.check();await this.cancel();}
    finally {operation.finish();}
  }
  async cancel() {
    const epoch = ++this.epoch;this.request?.abort();this.moves.cancel();this.delivered = false;
    await this.claim?.cancel();if (this.current() && epoch === this.epoch) this.publish(empty);
  }
  dispose() {
    this.closed = true;++this.epoch;this.request?.abort();this.moves.dispose();this.state = empty;this.listeners.clear();
    // A full-document approval handoff must survive teardown in the original origin.
    if (!this.delivered) void this.claim?.cancel().catch(() => {});
  }
}
