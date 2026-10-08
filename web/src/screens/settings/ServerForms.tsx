import React, {createContext, useContext, useEffect, useMemo, useSyncExternalStore} from 'react';
import {FormSet, type AnyDraftForm, type DraftForm, type FormState} from '@core/server-admin/draft-form.ts';
import {createServerForm, formValue, withFormValue, type ServerFormDeps, type ServerFormId} from '@core/server-admin/server-forms.ts';
import {serverChoices, serverRowShown, type ServerSettingRow} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {problem, useConsole} from '../../admin/console';
import {operationId} from '../server/operation-ids';
import {Button, Input, Notice, Select, SettingsRow, Switch, TextArea} from '../../ui';
import s from './Settings.module.css';

/**
 * A Server page's forms (client-core `server-admin`): one `FormSet` per page, so the page has one
 * Save and one Discard. A form is created the first time a row or panel asks for it and is read
 * at once; everything on the page that edits the same document shares the one draft.
 */
type AnyForm = DraftForm<any, any>;
type PageForms = Readonly<{set: FormSet; deps: ServerFormDeps; form: (id: ServerFormId) => AnyForm}>;
const Ctx = createContext<PageForms | null>(null);

export function ServerFormsProvider({children}: {children: React.ReactNode}) {
  const {api, system, session} = useSession();
  const client = useConsole();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const value = useMemo<PageForms>(() => {
    const set = new FormSet();
    const deps: ServerFormDeps = {api, console: client, serverId, operationId};
    const made = new Map<ServerFormId, AnyForm>();
    return {
      set, deps,
      form: id => {
        let form = made.get(id);
        if (!form) {
          form = createServerForm(id, deps);
          made.set(id, form);
          set.add(id, form as AnyDraftForm);
          void form.load();
        }
        return form;
      },
    };
  }, [api, client, serverId]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

function usePageForms(): PageForms {
  const forms = useContext(Ctx);
  if (!forms) throw new Error('Server forms are used inside a Server page.');
  return forms;
}

/** One of the page's forms and its state. */
export function useServerForm<T extends object = any, S = any>(id: ServerFormId): {form: DraftForm<T, S>; state: FormState<T, S>} {
  const form = usePageForms().form(id) as DraftForm<T, S>;
  const state = useSyncExternalStore(form.subscribe, form.getSnapshot);
  return {form, state};
}

/** Whether none of these forms exists on this server (features that are not set up): their rows group is left out. */
export function useFormsAbsent(ids: readonly ServerFormId[]): boolean {
  const forms = usePageForms();
  let absent = ids.length > 0;
  // The ids of a section are authored and never change between renders, so the hook count is stable.
  // eslint-disable-next-line react-hooks/rules-of-hooks
  for (const id of ids) { const form = forms.form(id); if (!useSyncExternalStore(form.subscribe, form.getSnapshot).unavailable) absent = false; }
  return absent;
}

export function useServerDeps(): ServerFormDeps {
  return usePageForms().deps;
}

type InlineState = {dirty: boolean; save: () => Promise<void>; discard: () => void; problem?: string};

/** A panel's own draft as one of the page's forms. */
class InlineForm implements AnyDraftForm {
  current: InlineState;
  private saving = false;
  private saveError: unknown;
  private listeners = new Set<() => void>();
  private state: FormState<object, unknown>;
  constructor(initial: InlineState) { this.current = initial; this.state = this.compute(); }
  private compute(): FormState<object, unknown> { return Object.freeze({loading: false, unavailable: false, dirty: this.current.dirty, saving: this.saving, saveError: this.saveError, problem: this.current.dirty ? this.current.problem : undefined}); }
  private publish() { this.state = this.compute(); for (const fn of this.listeners) fn(); }
  update(next: InlineState) { const changed = next.dirty !== this.current.dirty || next.problem !== this.current.problem; this.current = next; if (changed) { if (!next.dirty) this.saveError = undefined; this.publish(); } }
  getSnapshot = () => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  load = async () => {};
  discard = () => { this.saveError = undefined; this.current.discard(); this.publish(); };
  save = async () => {
    if (!this.current.dirty || this.saving) return;
    this.saving = true; this.saveError = undefined; this.publish();
    try { await this.current.save(); } catch (error) { this.saveError = error; throw error; } finally { this.saving = false; this.publish(); }
  };
  dispose = () => { this.listeners.clear(); };
}

/**
 * A panel that keeps its own draft (a form that is not one of the shared documents) joins the
 * page's one Save: it says whether it is dirty and how to save and discard, and shows no
 * buttons of its own.
 */
export function useInlineForm(id: string, form: InlineState): {saving: boolean; error?: unknown} {
  const {set} = usePageForms();
  const adapter = React.useRef<InlineForm | null>(null);
  if (!adapter.current) adapter.current = new InlineForm(form);
  useEffect(() => { adapter.current!.update(form); });
  useEffect(() => set.add('inline:' + id, adapter.current!), [set, id]);
  const state = useSyncExternalStore(adapter.current.subscribe, adapter.current.getSnapshot);
  return {saving: state.saving, error: state.saveError};
}

/** A catalogue id the forms use for a problem, or the server's own words. */
function problemText(t: (id: MessageId) => string, has: (id: string) => boolean, text: string | undefined): string | undefined {
  return text && has(text) ? t(text as MessageId) : text;
}

/** One server setting, drawn from its authored row. */
export function ServerSettingRowView({row, anchor}: {row: ServerSettingRow; anchor?: string}) {
  const i18n = useI18n();
  const {t} = i18n;
  const {form, state} = useServerForm(row.form);
  if (state.draft === undefined) return null;
  const read = (key: string) => formValue(state.draft, key);
  if (!serverRowShown(row, read)) return null;
  const value = read(row.key);
  const write = (next: unknown) => form.set((draft: object) => withFormValue(draft, row.key, next));
  const label = t(row.label);
  const restart = row.form === 'runtime' && (state.status as {restartFields?: readonly string[]} | undefined)?.restartFields?.includes(row.key) ? t('settings.save.afterRestart') : undefined;
  const common = {id: anchor, label, help: row.help ? t(row.help) : undefined, state: restart};
  const control = row.control;
  switch (control.type) {
    case 'switch': return <SettingsRow {...common} control={<Switch checked={value === true} onCheckedChange={write} label={label} />} />;
    case 'choice': {
      const known = control.choices.some(c => c.value === value);
      const options = [...(known ? [] : [{value: String(value), label: String(value)}]), ...control.choices.map(c => ({value: String(c.value), label: t(c.label, c.labelValues as Record<string, string | number> | undefined)}))];
      return <SettingsRow {...common} control={<Select hideLabel label={label} options={options} value={String(value)} onChange={e => { const picked = control.choices.find(c => String(c.value) === e.target.value); if (picked) write(picked.value); }} />} />;
    }
    case 'choiceFrom': {
      const options = serverChoices(control.source, state.status, t, typeof value === 'string' ? value : undefined);
      return <SettingsRow {...common} control={<Select hideLabel label={label} options={[...options]} value={String(value ?? '')} onChange={e => write(e.target.value)} />} />;
    }
    case 'number': {
      const scale = control.scale ?? 1;
      const empty = control.empty && (value === control.empty.stores || value === null);
      const shown = empty ? '' : String(Math.round(Number(value) / scale));
      const parse = (text: string) => {
        if (text.trim() === '' && control.empty) return write(control.empty.stores);
        const n = Math.floor(Number(text));
        if (!Number.isFinite(n)) return;
        write(Math.min(control.max, Math.max(control.min, n)) * scale);
      };
      return <SettingsRow {...common} control={<span className={s.numberField}><Input hideLabel label={label} type="number" inputMode="numeric" min={control.min} max={control.max} placeholder={control.empty ? t(control.empty.placeholder) : undefined} value={shown} onChange={e => parse(e.target.value)} />{control.unit ? <span className={s.unit}>{t(control.unit)}</span> : null}</span>} />;
    }
    case 'text': return <SettingsRow {...common} stack={control.mono} full={control.mono} control={<Input hideLabel label={label} mono={control.mono} placeholder={control.example} maxLength={control.maxLength} value={typeof value === 'string' ? value : ''} onChange={e => write(e.target.value)} />} />;
    case 'lines': {
      const lines = Array.isArray(value) ? (value as readonly string[]) : [];
      return <SettingsRow {...common} stack full control={<LinesField label={label} example={control.example} value={lines} onChange={write} />} />;
    }
  }
}

/** A list edited one entry per line; the draft holds the trimmed, non-empty lines. */
function LinesField({label, example, value, onChange}: {label: string; example?: string; value: readonly string[]; onChange: (next: readonly string[]) => void}) {
  // What is typed is kept as typed (a trailing newline must survive); the draft gets the cleaned list.
  const [text, setText] = React.useState(value.join('\n'));
  const clean = (v: string) => v.split('\n').map(x => x.trim()).filter(Boolean);
  useEffect(() => {
    if (clean(text).join('\n') !== value.join('\n')) setText(value.join('\n'));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);
  return <TextArea hideLabel label={label} rows={Math.min(8, Math.max(2, value.length + 1))} placeholder={example} value={text} className={s.lines} onChange={e => { setText(e.target.value); onChange(clean(e.target.value)); }} />;
}

/** What a form could not load, above its rows; nothing when it loaded or the feature is absent. */
export function FormLoadNotice({id}: {id: ServerFormId}) {
  const {t} = useI18n();
  const {form, state} = useServerForm(id);
  if (!state.loadError) return null;
  return <Notice tone={state.saved ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: () => void form.load()}}>{problem(state.loadError)}</Notice>;
}

/** The page's one Save / Discard, stuck to the bottom while anything on the page is unsaved. */
export function SaveBar() {
  const i18n = useI18n();
  const {t} = i18n;
  const {set} = usePageForms();
  const state = useSyncExternalStore(set.subscribe, set.getSnapshot);
  const [saved, setSaved] = React.useState(false);
  useEffect(() => {
    if (!saved) return;
    const timer = setTimeout(() => setSaved(false), 2500);
    return () => clearTimeout(timer);
  }, [saved]);
  // Leaving the site with unsaved changes asks first.
  useEffect(() => {
    if (!state.dirty) return;
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [state.dirty]);
  const blocked = problemText(t, id => i18n.has(id), state.problem);
  const failed = state.saveError !== undefined ? problem(state.saveError, 'action') || t('settings.save.failed') : undefined;
  if (!state.dirty && !saved && !failed) return null;
  return (
    <div className={s.saveBar} role="region" aria-label={t('action.save')}>
      <span className={s.saveNote} role="status" data-tone={blocked || failed ? 'danger' : undefined}>{blocked ?? failed ?? (state.dirty ? t('settings.save.unsaved') : t('settings.save.saved'))}</span>
      {state.dirty ? <Button variant="ghost" label={t('action.discard')} disabled={state.saving} onClick={() => set.discard()} /> : null}
      {state.dirty ? <Button variant="primary" label={t('action.save')} disabled={!!blocked} loading={state.saving} onClick={() => void set.save().then(ok => setSaved(ok))} /> : null}
    </div>
  );
}

/** Whether the page has unsaved changes (a tab switch or page link inside Settings asks before leaving). */
export function usePageDirty(): boolean {
  const {set} = usePageForms();
  return useSyncExternalStore(set.subscribe, () => set.getSnapshot().dirty);
}
