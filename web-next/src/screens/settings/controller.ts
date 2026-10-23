import {ConsoleClient} from '@core/console.ts';
import {PreferencesReader} from '@core/preferences-reader.ts';
import {validatePreferencePatch, type PreferencePatch, type PreferenceScope, type PreferenceSnapshot} from '@core/preferences.ts';
import {loadSettingsConfiguration, searchConfiguration, type SettingsConfiguration} from '@core/settings-configuration.ts';
import {applySettingsBindingPatch, changedFromDefault, exactPresetValue, presetPatch, readSettingsBinding, settingsBinding, settingsBindingPatch, type SettingsBindingForm, type SettingsBindingSources, type SettingsBindingValue} from '@core/presentation/settings-bindings.ts';
import {settingsRowIndex, type SettingsRow, type SettingsSection, type SettingsValue} from '@core/presentation/settings-spec.ts';
import {runtimeForm, connectivityForm, maintenanceForm, alertsForm, logsForm, type ServerFormDeps} from '@core/server-admin/server-forms.ts';
import type {DraftForm} from '@core/server-admin/draft-form.ts';
import {randomId} from '@core/random-id.ts';
import {defaultI18n} from '@i18n';
import type {HttpLocalApi} from '@core/index.ts';
import type {SettingPresentation} from '../../kit/settings';
import type {SettingsContext, SettingsPageSnapshot} from './Settings';
import {settingsServerHeader} from './server-header';
import {configurationFieldEligible, settingsRowEligible, type SettingsEligibility} from './eligibility';
import {settingsRowVisible, settingsSectionVisible, type SettingsEnvironment} from '@core/presentation/settings-visibility.ts';

type DocumentForm = Readonly<{
  load: () => Promise<void>; dispose: () => void;
  value: () => Readonly<Record<string, unknown>> | undefined;
  error: () => unknown;
  patch: (values: Readonly<Record<string, unknown>>) => Promise<void>;
}>;
function documentForm<T extends object, S>(form: DraftForm<T, S>): DocumentForm {
  return {
    load: form.load.bind(form), dispose: form.dispose.bind(form),
    value: () => form.getSnapshot().saved as Readonly<Record<string, unknown>> | undefined,
    error: () => form.getSnapshot().loadError,
    patch: async values => {
      const state = form.getSnapshot();
      if (!state.saved || state.saving) throw new Error('settings_document_unavailable');
      form.set(draft => applySettingsBindingPatch(draft as Record<string, unknown>, {form: 'runtime', values}) as T);
      await form.save();
    },
  };
}
/** Supplied by the checked Reading facade adapter; the UI owns no row/key map. */
export type ReadingSettingsAdapter = Readonly<{
  read: (rowId: string, snapshot: PreferenceSnapshot) => SettingsBindingValue;
  patch: (rowId: string, value: SettingsValue | null, snapshot: PreferenceSnapshot) => PreferencePatch;
}>;
export type SettingsControllerOptions = Readonly<{
  api: Pick<HttpLocalApi, 'request'>; serverId: string; viewerId: string;
  role: 'owner' | 'admin' | 'member'; mainProfile: boolean;
  capabilities: Readonly<{manageServer: boolean; managePeople: boolean; manageSignIn: boolean}>;
  preferences: PreferenceSnapshot; isCurrent: () => boolean; reportError: (error: unknown) => void;
  eligibility: SettingsEligibility; reading: ReadingSettingsAdapter; environment: SettingsEnvironment;
}>;

/** One selected session owns every document and pending write. No constructor fetches. */
export class SettingsController {
  private readonly options: SettingsControllerOptions;
  private readonly console: ConsoleClient;
  private readonly preferencesReader: PreferencesReader;
  private readonly forms: Partial<Record<SettingsBindingForm, DocumentForm>> = {};
  private preferences: PreferenceSnapshot;
  private configuration?: SettingsConfiguration;
  private snapshot: SettingsPageSnapshot = Object.freeze({loading: true});
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private readonly rowErrors = new Map<string, unknown>();
  private closed = false;
  private loaded = false;
  private loading?: Promise<void>;
  private writing = false;
  private header: ReturnType<typeof settingsServerHeader> = {facts: []};
  constructor(options: SettingsControllerOptions) {
    this.options = options; this.preferences = options.preferences;
    const scope = {serverId: options.serverId, viewerId: options.viewerId};
    this.console = new ConsoleClient(options.api, scope);
    this.preferencesReader = new PreferencesReader(options.api, scope);
    if (options.role === 'owner' && options.mainProfile && options.capabilities.manageServer) {
      const deps: ServerFormDeps = {api: options.api, console: this.console, serverId: options.serverId, operationId: randomId};
      this.forms.runtime = documentForm(runtimeForm(deps));
      this.forms.connectivity = documentForm(connectivityForm(deps));
      this.forms.maintenance = documentForm(maintenanceForm(deps));
      this.forms.alerts = documentForm(alertsForm(deps));
      this.forms.logs = documentForm(logsForm(deps));
    }
  }
  private current = () => !this.closed && this.options.isCurrent();
  getSnapshot = () => this.snapshot;
  serverHealth = () => this.current() ? this.header.health : undefined;
  serverFacts = () => this.current() ? this.header.facts : [];
  subscribe = (listener: () => void) => {this.listeners.add(listener); return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<SettingsPageSnapshot>) {
    if (!this.current()) return;
    this.snapshot = Object.freeze({...this.snapshot, ...patch});
    for (const listener of this.listeners) listener();
  }
  sectionVisible = (section: SettingsSection) => {
    if (!settingsSectionVisible(section, this.options.environment)) return false;
    if (section.scope === 'account') return true;
    if (!this.options.mainProfile) return false;
    if (section.id === 'people') return this.options.capabilities.managePeople;
    if (section.id === 'signin') return this.options.capabilities.manageSignIn;
    return this.options.capabilities.manageServer && (this.options.role === 'owner' || section.id === 'dashboard' || section.id === 'libraries');
  };
  eligible = (row: SettingsRow, context?: SettingsContext): boolean => {
    const location = settingsRowIndex.get(row.id);
    return !!location && this.current() && settingsRowVisible(row, {...this.options.environment, role: this.options.role}) && settingsRowEligible(row, context ?? {section: location.section, panelId: location.panelId, tabId: location.tabId}, this.options.eligibility);
  };
  private sources(): SettingsBindingSources {
    return {configuration: this.configuration, preferences: this.preferences,
      forms: Object.fromEntries(Object.entries(this.forms).map(([name, form]) => [name, form.value()]))};
  }
  private read(row: SettingsRow, context?: SettingsContext): SettingsBindingValue | undefined {
    if (!this.eligible(row, context)) return undefined;
    const location = settingsRowIndex.get(row.id)!;
    if (location.section.scope === 'account' && location.section.id === 'reading') return this.options.reading.read(row.id, this.preferences);
    const options = row.control.options;
    if (options?.some(option => option.sets)) {
      const ids = [...new Set(options.flatMap(option => Object.keys(option.sets ?? {})))];
      const values = new Map(ids.map(id => [id, settingsRowIndex.get(id)?.row]).map(([id, row]) => [id, row ? this.read(row as SettingsRow, context) : undefined]));
      if (ids.some(id => !values.get(id))) return undefined;
      const defaultsKnown = ids.every(id => values.get(id)!.default !== undefined);
      return {current: exactPresetValue(options, id => values.get(id)?.current), effective: exactPresetValue(options, id => values.get(id)?.effective),
        default: defaultsKnown ? exactPresetValue(options, id => values.get(id)?.default) : undefined, source: [...new Set(ids.map(id => values.get(id)!.source))].join(','), configOnly: false};
    }
    const binding = settingsBinding(row.id);
    return binding && readSettingsBinding(binding, this.sources(), row.id);
  }
  rowState = (row: SettingsRow, context?: SettingsContext): SettingPresentation => {
    const location = settingsRowIndex.get(row.id);
    if (!location || !this.sectionVisible(location.section)) return {present: false};
    const value = this.read(row, context);
    if (!value || value.configOnly) return {present: false};
    const current = value.current as SettingsValue;
    const option = row.control.options?.find(option => option.value === current);
    return {present: true, value: current, defaultValue: value.default as SettingsValue | undefined,
      changed: value.default !== undefined && changedFromDefault(value.effective, value.default),
      displayValue: option ? defaultI18n.t(option.label) : typeof current === 'boolean' ? defaultI18n.t(current ? 'next.settings.on' : 'next.settings.off') : typeof current === 'object' ? JSON.stringify(current) : String(current),
      disabled: this.writing || !this.current(), saving: this.writing,
      error: this.rowErrors.has(row.id) ? defaultI18n.t('next.states.pageFailedBody') : undefined};
  };
  configurationMatches = (query: string) => this.configuration && this.options.role === 'owner' && this.options.mainProfile ? searchConfiguration(this.configuration, query).filter(field => configurationFieldEligible(field, this.eligible)).map(field => ({...field, label: settingsRowIndex.has(field.id) ? defaultI18n.t(settingsRowIndex.get(field.id)!.row.label) : field.key, path: this.configuration!.path})) : [];
  load = (): Promise<void> => {
    if (!this.current() || this.loaded) return Promise.resolve();
    if (this.loading) return this.loading;
    this.publish({loading: true, error: undefined});
    const task = (async () => {
      try {
        if (this.forms.runtime) {
          const [health, storage] = await Promise.allSettled([this.console.measurement('health'), this.console.measurement('storage')]);
          if (!this.current()) return;
          this.header = settingsServerHeader(health.status === 'fulfilled' ? health.value : undefined, storage.status === 'fulfilled' ? storage.value : undefined);
          this.publish({});
        }
        await Promise.all(Object.values(this.forms).map(async form => {await form.load(); if (form.error()) throw form.error();}));
        if (this.forms.runtime) this.configuration = await loadSettingsConfiguration(this.options.api, this.options.serverId, this.lifetime.signal);
        if (!this.current()) return;
        this.loaded = true; this.publish({loading: false});
      } catch (error) {this.publish({loading: false, error});}
      finally {this.loading = undefined;}
    })();
    this.loading = task; return task;
  };
  private preferenceScope(row: SettingsRow, key: string): PreferenceScope {
    const field = this.preferences.registry.fields.find(field => field.key === key);
    if (!field) throw new Error('settings_preference_unavailable');
    const scope = row.scope === 'device' ? 'profile-device-class' : 'profile-server';
    if (!field.scopes.includes(scope)) {
      if (field.scopes.length === 1) return field.scopes[0];
      throw new Error('settings_preference_scope_unavailable');
    }
    return scope;
  }
  private write = async (row: SettingsRow, value: SettingsValue, reset: boolean, context?: SettingsContext) => {
    if (this.writing || !this.current() || !this.rowState(row, context).present) return;
    this.writing = true; this.rowErrors.delete(row.id); this.publish({saveState: 'saving'});
    try {
      const sets = row.control.options?.some(option => option.sets) ? presetPatch(row.control.options, value, id => {const row = settingsRowIndex.get(id)?.row; return row && this.read(row, context)?.current;}) : {[row.id]: value};
      const patches = new Map<SettingsBindingForm, Record<string, unknown>>(), preferencePatches = new Map<PreferenceScope, Record<string, unknown>>();
      const sources = this.sources();
      for (const [id, next] of Object.entries(sets)) {
        const target = settingsRowIndex.get(id)?.row, binding = settingsBinding(id);
        if (!target || !this.eligible(target, context)) throw new Error('settings_binding_unavailable');
        const location = settingsRowIndex.get(id)!;
        if (location.section.scope === 'account' && location.section.id === 'reading') {
          const patch = this.options.reading.patch(id, reset ? null : next, this.preferences);
          for (const [key, value] of Object.entries(patch)) {
            const scope = this.preferenceScope(target, key);
            preferencePatches.set(scope, {...preferencePatches.get(scope), [key]: value});
          }
          continue;
        }
        if (!binding) throw new Error('settings_binding_unavailable');
        const patch = settingsBindingPatch(binding, next, sources);
        if (patch.form === 'preferences') {
          for (const [key, value] of Object.entries(patch.values)) {
            const scope = this.preferenceScope(target, key);
            preferencePatches.set(scope, {...preferencePatches.get(scope), [key]: reset ? null : value});
          }
        } else patches.set(patch.form, {...patches.get(patch.form), ...patch.values});
      }
      for (const [scope, values] of preferencePatches) {
        const document = this.preferences.documents.find(document => document.scope === scope);
        if (!document) throw new Error('settings_preference_document_unavailable');
        const patch = validatePreferencePatch(this.preferences.registry, scope, values as PreferencePatch);
        this.preferences = await this.preferencesReader.applyPreferences(scope, this.preferences.deviceClass, document.revision, patch);
        if (!this.current()) return;
      }
      for (const [id, patch] of patches) {
        const form = this.forms[id]; if (!form) throw new Error('settings_document_unavailable');
        await form.patch(patch); if (!this.current()) return;
      }
      if (patches.size) this.configuration = await loadSettingsConfiguration(this.options.api, this.options.serverId, this.lifetime.signal);
      this.publish({saveState: 'saved'});
    } catch (error) {
      if (this.current()) {this.rowErrors.set(row.id, error); this.publish({saveState: undefined}); this.options.reportError(error);}
    } finally {this.writing = false; this.publish({});}
  };
  change = (row: SettingsRow, value: SettingsValue, context?: SettingsContext) => {void this.write(row, value, false, context);};
  reset = (row: SettingsRow, context?: SettingsContext) => {const value = this.read(row, context)?.default; if (value !== undefined) void this.write(row, value as SettingsValue, true, context);};
  dispose = () => {
    this.closed = true; this.lifetime.abort(); this.console.dispose(); this.preferencesReader.dispose();
    for (const form of Object.values(this.forms)) form.dispose();
    this.configuration = undefined; this.header = {facts: []}; this.rowErrors.clear(); this.listeners.clear();
  };
}
