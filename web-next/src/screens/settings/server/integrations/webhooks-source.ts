import {ApiAccessError, createWebhook, deleteWebhook, listWebhooks, listWebhookDeliveries, sendWebhookTest, updateWebhook, type ApiAccessTransport, type IssuedWebhook, type NewWebhook, type Webhook, type WebhookChange, type WebhookDeliveryPage, type WebhookList} from '@core/api-access.ts';

type Failure = 'read'|'write'|'deliveries';
export type WebhooksSnapshot = Readonly<{phase: 'idle'|'loading'|'ready'|'error'; list?: WebhookList; selected?: Webhook; deliveries?: WebhookDeliveryPage; busy: boolean; uncertain: boolean; error?: Failure}>;
const empty: WebhooksSnapshot = Object.freeze({phase: 'idle', busy: false, uncertain: false});
const terminal = (error: unknown) => {
  const {status, code} = (error && typeof error === 'object' ? error : {}) as {status?: number; code?: string};
  return status === 401 || status === 403 || ['unauthorized','forbidden','not_permitted'].includes(code ?? '');
};
const eventsKey = (events: readonly string[]) => [...new Set(events)].sort().join(',');
const matches = (hook: Webhook, input: WebhookChange) => (input.name === undefined || hook.name === input.name)
  && (input.url === undefined || hook.url === input.url) && (input.enabled === undefined || hook.enabled === input.enabled)
  && (input.events === undefined || eventsKey(hook.events) === eventsKey(input.events));

/** One owner/server lifetime. No webhook write is replayed after an uncertain
 * response: these endpoints do not promise operation-id deduplication. */
export class SettingsWebhooksSource {
  private state = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private read?: AbortController;
  private detail?: AbortController;
  private mutation?: AbortController;
  constructor(private readonly api: ApiAccessTransport, private readonly isCurrent: () => boolean, private readonly reportError: (error: unknown) => void) {}
  private current = () => !this.lifetime.signal.aborted && this.isCurrent();
  private ownsRead = (read: AbortController) => this.current() && this.read === read && !read.signal.aborted && !this.mutation;
  private ownsWrite = (write: AbortController) => this.current() && this.mutation === write && !write.signal.aborted;
  private transport(owns: () => boolean): ApiAccessTransport {return {request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => owns() ? this.api.request<T>(path, method, body, signal) : Promise.reject(new DOMException('The webhook scope ended.', 'AbortError'))};}
  getSnapshot = (): WebhooksSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<WebhooksSnapshot>) {if (this.current()) {this.state = Object.freeze({...this.state, ...patch});for (const listener of this.listeners) listener();}}
  private failure(error: unknown, kind: Failure) {
    if (terminal(error)) {this.read?.abort();this.detail?.abort();this.mutation?.abort();this.mutation = undefined;this.lifetime.abort();this.state = empty;for (const listener of this.listeners) listener();this.reportError(error);}
    else this.publish({phase: 'error', error: kind});
  }
  load = async (): Promise<void> => {
    if (!this.current() || this.mutation) return;
    this.read?.abort();this.detail?.abort();const read = new AbortController();this.read = read;
    this.publish({phase: 'loading', error: undefined, selected: undefined, deliveries: undefined});
    if (!this.ownsRead(read)) return;
    try {const list = await listWebhooks(this.transport(() => this.ownsRead(read)), read.signal);if (this.ownsRead(read)) this.publish({phase: 'ready', list});}
    catch (error) {if (this.ownsRead(read)) this.failure(error, 'read');}
    finally {if (this.read === read) this.read = undefined;}
  };
  /** Explicitly reconcile the list after an uncertain write. This only reads;
   * a later write requires a separate user action against this new list. */
  reconcile = async () => {await this.load();if (this.current() && this.state.phase === 'ready') this.publish({uncertain: false});};
  select = async (hook: Webhook, cursor = ''): Promise<void> => {
    if (!this.current() || this.mutation || !this.state.list?.items.includes(hook)) return;
    this.detail?.abort();const detail = new AbortController();this.detail = detail;
    const owns = () => this.current() && this.detail === detail && !detail.signal.aborted && this.state.selected === hook && this.state.list?.items.includes(hook) === true && !this.mutation;
    this.publish({selected: hook, deliveries: undefined, error: undefined});if (!owns()) return;
    try {const deliveries = await listWebhookDeliveries(this.transport(owns), hook.id, {cursor: cursor || undefined, limit: 50}, detail.signal);if (owns()) this.publish({deliveries});}
    catch (error) {if (owns()) this.failure(error, 'deliveries');}
  };
  close = () => {this.detail?.abort();this.detail = undefined;this.publish({selected: undefined, deliveries: undefined});};
  private claim(hook?: Webhook): AbortController | undefined {
    if (!this.current() || this.mutation || this.state.uncertain || this.state.phase !== 'ready' || !this.state.list || hook && !this.state.list.items.includes(hook)) return;
    this.read?.abort();this.detail?.abort();const write = new AbortController();this.mutation = write;
    this.publish({busy: true, error: undefined});return write;
  }
  private async write<T>(operation: (api: ApiAccessTransport, signal: AbortSignal) => Promise<T>, hook?: Webhook): Promise<T | undefined> {
    const write = this.claim(hook);if (!write) return;
    let result: T | undefined;
    try {if (!this.ownsWrite(write)) return;result = await operation(this.transport(() => this.ownsWrite(write)), write.signal);if (!this.ownsWrite(write)) return;}
    catch (error) {if (this.ownsWrite(write)) {this.publish({uncertain: true});this.failure(error, 'write');}}
    finally {if (this.mutation === write) {this.mutation = undefined;this.publish({busy: false});}}
    if (result !== undefined && this.current()) await this.load();
    return this.current() ? result : undefined;
  }
  create = async (input: NewWebhook): Promise<IssuedWebhook | undefined> => {
    const name = input.name.trim(), url = input.url.trim(), events = Object.freeze([...new Set(input.events)]);
    if (!name || name.length > 160 || !events.length || events.some(event => !this.state.list?.events.includes(event)) || this.state.list && this.state.list.items.length >= this.state.list.maximum) return;
    try {const parsed = new URL(url);if (!['http:','https:'].includes(parsed.protocol) || parsed.username || parsed.password) return;} catch {return;}
    const intent = Object.freeze({...input, name, url, events});
    return this.write(async (api, signal) => {const issued = await createWebhook(api, intent, signal);if (!matches(issued.webhook, intent)) throw new ApiAccessError('invalid_document');return issued;});
  };
  update = async (hook: Webhook, change: WebhookChange): Promise<IssuedWebhook | undefined> => {
    const intent = Object.freeze({...change, ...(change.events ? {events: Object.freeze([...change.events])} : {})});
    return this.write(async (api, signal) => {const issued = await updateWebhook(api, hook.id, hook.revision, intent, signal);if (issued.webhook.id !== hook.id || !matches(issued.webhook, intent)) throw new ApiAccessError('invalid_document');return issued;}, hook);
  };
  remove = (hook: Webhook) => this.write(async (api, signal) => {await deleteWebhook(api, hook.id, signal);return true;}, hook);
  test = (hook: Webhook) => this.write((api, signal) => sendWebhookTest(api, hook.id, signal), hook);
  dispose = () => {this.read?.abort();this.detail?.abort();this.mutation?.abort();this.mutation = undefined;this.lifetime.abort();this.state = empty;this.listeners.clear();};
}
