import {defaultI18n} from '@i18n';
import {remoteAccessForm} from '@core/server-admin/server-forms.ts';
import type {RemoteConfiguration} from '@core/remote-access.ts';
import type {SettingsRow, SettingsValue} from '@core/presentation/settings-spec.ts';
import type {SettingPresentation} from '../../../kit/settings';
import type {SettingsPageSnapshot} from '../Settings';

type Form = ReturnType<typeof remoteAccessForm>;
type Api = Parameters<typeof remoteAccessForm>[0]['api'];
const rowIds = new Set(['remote.mapping', 'remote.mappingProtocols', 'remote.gateway', 'remote.publicPort', 'remote.ipv6']);
const cancelled = () => new DOMException('Remote Settings build retired', 'AbortError');

/** One committed owner build, one original API and one revision-bearing form.
 * This private Web adapter does not invent generic configuration bindings. */
export class RemoteFormSource {
  private readonly form: Form;
  private readonly life = new AbortController();
  private disposed = false;
  private pending?: Promise<void>;
  private pageBase?: SettingsPageSnapshot;
  private pageForm?: ReturnType<Form['getSnapshot']>;
  private page?: SettingsPageSnapshot;
  constructor(api: Api, private readonly isCurrent: () => boolean, private readonly owner: boolean) {
    const request: Api['request'] = async <T>(...args: Parameters<Api['request']>): Promise<T> => {
      const [path, method, body, signal] = args;
      this.assert();
      const deadline = new AbortController();
      const timer = setTimeout(() => deadline.abort(new DOMException('Remote Settings timed out', 'TimeoutError')), 15_000);
      const bounded = AbortSignal.any([this.life.signal, deadline.signal, ...(signal ? [signal] : [])]);
      let stop: (() => void) | undefined;
      try {
        const aborted = new Promise<never>((_resolve, reject) => {
          stop = () => reject(bounded.reason ?? cancelled());
          bounded.addEventListener('abort', stop, {once: true});
          if (bounded.aborted) stop();
        });
        const value = await Promise.race([api.request<T>(path, method, body, bounded), aborted]);
        this.assert();if (bounded.aborted) throw bounded.reason;
        return value;
      } finally {clearTimeout(timer);if (stop) bounded.removeEventListener('abort', stop);}
    };
    this.form = remoteAccessForm({api: {request}} as Parameters<typeof remoteAccessForm>[0]);
  }
  current = () => {
    if (this.disposed || !this.owner || this.life.signal.aborted) return false;
    const accepted = this.isCurrent();
    return accepted && !this.disposed && this.owner && !this.life.signal.aborted;
  };
  private assert() {if (!this.current()) {this.dispose();throw cancelled();}}
  has = (rowId: string) => rowIds.has(rowId);
  getSnapshot = () => this.form.getSnapshot();
  subscribe = (listener: () => void) => this.form.subscribe(listener);
  load = (): Promise<void> => {
    if (!this.current()) return Promise.resolve();
    if (this.pending) return this.pending;
    // Reserve before DraftForm publishes loading to synchronous subscribers.
    const pending = Promise.resolve().then(() => {if (!this.current()) return;return this.form.load();}).finally(() => {if (this.pending === pending) this.pending = undefined;});
    this.pending = pending;return pending;
  };
  rowState(row: SettingsRow): SettingPresentation {
    if (!this.current() || !this.has(row.id)) return {present: false};
    const state = this.getSnapshot(), draft = state.draft;
    if (!draft) return {present: false};
    const value = this.value(row.id, draft), saved = state.saved && this.value(row.id, state.saved);
    return {present: true, value, disabled: state.loading || state.saving, saving: state.saving,
      changed: JSON.stringify(value) !== JSON.stringify(saved), defaultValue: saved,
      ...(row.id === 'remote.ipv6' && value === undefined ? {readOnly: true, displayValue: defaultI18n.t('next.settings.notSet')} : {})};
  }
  private value(id: string, config: RemoteConfiguration): SettingsValue | undefined {
    switch (id) {
      case 'remote.mapping': return config.mapping ? 'auto' : 'off';
      case 'remote.mappingProtocols': return {pcp: config.pcp ? 'try' : 'skip', natpmp: config.natpmp ? 'try' : 'skip', upnp: config.upnp ? 'try' : 'skip'};
      case 'remote.gateway': return config.gateway;
      case 'remote.publicPort': return config.publicPort;
      case 'remote.ipv6': return config.ipv6Open;
    }
  }
  change(rowId: string, value: SettingsValue) {
    if (!this.current() || !this.has(rowId)) return;
    const {draft, loading, saving} = this.getSnapshot();
    if (!draft || loading || saving) return;
    switch (rowId) {
      case 'remote.mapping': if (value === 'auto' || value === 'off') this.form.set({mapping: value === 'auto'});break;
      case 'remote.mappingProtocols': {
        if (!draft.mapping || !value || typeof value !== 'object' || Array.isArray(value)) break;
        const matrix = value as Readonly<Record<string, SettingsValue>>;
        if (Object.keys(matrix).length !== 3 || !['pcp', 'natpmp', 'upnp'].every(key => matrix[key] === 'try' || matrix[key] === 'skip')) break;
        this.form.set({pcp: matrix.pcp === 'try', natpmp: matrix.natpmp === 'try', upnp: matrix.upnp === 'try'});break;
      }
      case 'remote.gateway': if (draft.mapping && typeof value === 'string' && value.length <= 64) this.form.set({gateway: value});break;
      case 'remote.publicPort': if (typeof value === 'number' && Number.isInteger(value) && value >= 1024 && value <= 65535) this.form.set({publicPort: value});break;
      case 'remote.ipv6': if (draft.ipv6Open !== undefined && typeof value === 'boolean') this.form.set({ipv6Open: value});break;
    }
  }
  reset(rowId: string) {
    if (!this.current() || !this.has(rowId)) return;
    const saved = this.getSnapshot().saved, value = saved && this.value(rowId, saved);
    if (value !== undefined) this.change(rowId, value);
  }
  save = async (expected = this.getSnapshot()) => {this.assert();if (expected !== this.getSnapshot()) return;await this.form.save();};
  discard = (expected = this.getSnapshot()) => {if (this.current() && expected === this.getSnapshot()) this.form.discard();};
  pageSnapshot(base: SettingsPageSnapshot): SettingsPageSnapshot {
    const form = this.getSnapshot();
    if (this.page && this.pageBase === base && this.pageForm === form) return this.page;
    this.pageBase = base;this.pageForm = form;
    return this.page = Object.freeze({...base, loading: base.loading || form.loading, error: base.error ?? form.loadError,
      saveState: form.saving ? 'saving' : base.saveState});
  }
  dispose() {if (this.disposed) return;this.disposed = true;this.life.abort();this.form.dispose();}
}
