import {ApiAccessError, createApiKey, getApiAccessOptions, listApiKeys, revokeApiKey, type ApiAccessOptions, type ApiAccessTransport, type ApiCredential, type ApiCredentialPage, type IssuedApiCredential, type NewApiCredential} from '@core/api-access.ts';
import {randomId} from '@core/random-id.ts';

export type ApiKeyDraft = Omit<NewApiCredential, 'operationId'>;
export type ApiKeysSnapshot = Readonly<{phase: 'idle'|'loading'|'ready'|'error'; options?: ApiAccessOptions; page?: ApiCredentialPage; busy: boolean; pendingCreate: boolean; error?: 'read'|'create'|'revoke'}>;
const empty: ApiKeysSnapshot = Object.freeze({phase: 'idle', busy: false, pendingCreate: false});
const terminal = (error: unknown) => {
  const {status, code} = (error && typeof error === 'object' ? error : {}) as {status?: number; code?: string};
  return status === 401 || status === 403 || ['unauthorized', 'forbidden', 'not_permitted'].includes(code || '');
};

/** Captured owner/server authority. Lists contain no secret; an issued secret is
 * returned only to the current caller and is never stored in a snapshot. */
export class SettingsApiKeysSource {
  private state = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private read?: AbortController;
  private intent?: NewApiCredential;
  constructor(private readonly api: ApiAccessTransport, private readonly isCurrent: () => boolean, private readonly reportError: (error: unknown) => void) {}
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  getSnapshot = (): ApiKeysSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<ApiKeysSnapshot>) {if (this.current()) {this.state = Object.freeze({...this.state, ...patch});for (const listener of this.listeners) listener();}}
  private failure(error: unknown, kind: 'read'|'create'|'revoke') {
    if (terminal(error)) {
      this.read?.abort();this.intent = undefined;this.lifetime.abort();this.state = empty;
      for (const listener of this.listeners) listener();
    } else this.publish({phase: 'error', error: kind});
    this.reportError(error);
  }
  load = async (cursor = ''): Promise<void> => {
    if (!this.current() || this.state.busy) return;
    this.read?.abort();const read = new AbortController();this.read = read;
    this.publish({phase: 'loading', error: undefined});
    try {
      const [page, options] = await Promise.all([listApiKeys(this.api, cursor, read.signal), getApiAccessOptions(this.api, read.signal)]);
      if (!this.current() || this.read !== read || read.signal.aborted) return;
      if (page.items.some(item => item.kind !== 'key')) throw new ApiAccessError('invalid_document');
      this.publish({phase: 'ready', page, options});
    } catch (error) {if (this.current() && this.read === read && !read.signal.aborted) this.failure(error, 'read');}
  };
  /** Start exactly the draft the dialog captured. An uncertain create is retried
   * through retryCreate(), never by substituting another draft under its ID. */
  create = async (draft: ApiKeyDraft): Promise<IssuedApiCredential|undefined> => {
    if (!this.current() || this.state.busy || this.intent || this.state.phase !== 'ready' || !this.state.options) return;
    const {scopes} = draft, offered = this.state.options.scopes;
    if (!draft.name.trim() || draft.name.length > 160 || !scopes.length || scopes.some(scope => !offered.includes(scope)) || (scopes.includes('admin') && scopes.length !== 1)) return;
    this.intent = Object.freeze({...draft, scopes: Object.freeze([...scopes]), operationId: randomId()});
    this.publish({pendingCreate: true});
    return this.performCreate();
  };
  retryCreate = async (): Promise<IssuedApiCredential|undefined> => {
    if (!this.current() || this.state.busy || !this.intent) return;
    return this.performCreate();
  };
  private async performCreate(): Promise<IssuedApiCredential|undefined> {
    const intent = this.intent;if (!intent || !this.current()) return;
    this.read?.abort();this.read = undefined;this.publish({busy: true, error: undefined});
    let issued: IssuedApiCredential|undefined;
    try {
      issued = await createApiKey(this.api, intent, this.lifetime.signal);
      if (!this.current()) return;
      this.intent = undefined;this.publish({pendingCreate: false, page: undefined});
    } catch (error) {if (this.current()) this.failure(error, 'create');}
    finally {this.publish({busy: false});}
    if (issued && this.current()) await this.load();
    return this.current() ? issued : undefined;
  }
  /** Only an actual listed, working key may enter its confirmation action. The
   * server rechecks authority; a lost revoke is read back without auto-replay. */
  revoke = async (credential: ApiCredential): Promise<void> => {
    if (!this.current() || this.state.busy || this.state.phase !== 'ready' || !this.state.page?.items.includes(credential) || credential.kind !== 'key' || !['active','awaiting_approval'].includes(credential.state)) return;
    this.read?.abort();this.read = undefined;this.publish({busy: true, error: undefined});
    try {
      await revokeApiKey(this.api, credential.id, this.lifetime.signal);
      if (this.current()) this.publish({page: undefined});
    } catch (error) {if (this.current()) this.failure(error, 'revoke');}
    finally {this.publish({busy: false});}
    if (this.current()) await this.load();
  };
  dispose = () => {this.read?.abort();this.intent = undefined;this.lifetime.abort();this.state = empty;this.listeners.clear();};
}
