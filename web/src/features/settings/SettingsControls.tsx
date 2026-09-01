import { StatusWarningIcon, ActionConfirmIcon, AccountSecurityIcon, ActionRefreshIcon, ActionResetIcon, ActionCloseIcon } from '#portico-icons';
import { createContext, type ReactNode, useCallback, useContext, useEffect, useId, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { PasswordInput } from '../../components/controls/PasswordInput';
import { SelectMenu } from '../../components/controls/SelectMenu';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productText } from '../../components/ProductLanguage';
import { isSettingsNavigationDirty, isSettingsNavigationSensitive, setSettingsNavigationDirty, subscribeSettingsBlockedNavigation, type SettingsBlockedNavigation } from './settingsNavigationGuard';

export function SettingsGroup({ title, description, children, actions }: { title: string; description?: string; children: ReactNode; actions?: ReactNode }) {
  return <section className="portico-settings-group">
    <header><div><h2>{title}</h2>{description && <p>{description}</p>}</div>{actions && <div className="portico-settings-group-actions">{actions}</div>}</header>
    <div className="portico-settings-group-body">{children}</div>
  </section>;
}

type SettingAssociation = { labelledBy: string; describedBy: string };
const SettingAssociationContext = createContext<SettingAssociation | undefined>(undefined);

export function SettingRow({ label, description, children, indicator }: { label: string; description: string; children: ReactNode; indicator?: ReactNode }) {
  const id = useId();
  const labelledBy = `${id}-label`;
  const describedBy = `${id}-description`;
  return <div className="portico-setting-row">
    <div className="portico-setting-copy"><div><strong id={labelledBy}>{label}</strong>{indicator}</div><p id={describedBy}>{description}</p></div>
    <SettingAssociationContext.Provider value={{ labelledBy, describedBy }}><div className="portico-setting-control">{children}</div></SettingAssociationContext.Provider>
  </div>;
}

export function ToggleControl({ label, value, onChange, disabled = false }: { label: string; value: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  const association = useContext(SettingAssociationContext);
  return <button type="button" className={`portico-setting-toggle ${value ? 'on' : ''}`} role="switch" aria-label={association ? undefined : label} aria-labelledby={association?.labelledBy} aria-describedby={association?.describedBy} aria-checked={value} disabled={disabled} onClick={() => onChange(!value)}><span /></button>;
}

export function TextControl({ label, value, onChange, disabled = false, placeholder, type = 'text', multiline = false }: { label: string; value: string; onChange: (value: string) => void; disabled?: boolean; placeholder?: string; type?: 'text' | 'email' | 'password'; multiline?: boolean }) {
  const id = useId();
  const association = useContext(SettingAssociationContext);
  const accessibility = { 'aria-label': association ? undefined : label, 'aria-labelledby': association?.labelledBy, 'aria-describedby': association?.describedBy };
  if (multiline) return <textarea id={id} {...accessibility} value={value} disabled={disabled} placeholder={placeholder} rows={3} onChange={(event) => onChange(event.target.value)} />;
  if (type === 'password') return <PasswordInput id={id} {...accessibility} value={value} disabled={disabled} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />;
  return <input id={id} {...accessibility} value={value} disabled={disabled} placeholder={placeholder} type={type} onChange={(event) => onChange(event.target.value)} />;
}

export function NumberControl({ label, value, onChange, disabled = false, min, max, step = 1, unit, fieldKey, error }: { label: string; value: number | undefined; onChange: (value: number | undefined) => void; disabled?: boolean; min?: number; max?: number; step?: number; unit?: string; fieldKey?: string; error?: string }) {
  const association = useContext(SettingAssociationContext);
  const errorId = useId();
  const describedBy = [association?.describedBy, error ? errorId : undefined].filter(Boolean).join(' ') || undefined;
  return <label className="portico-setting-number"><input aria-label={association ? undefined : label} aria-labelledby={association?.labelledBy} aria-describedby={describedBy} aria-invalid={error ? true : undefined} data-settings-field={fieldKey} type="number" value={value ?? ''} disabled={disabled} min={min} max={max} step={step} onChange={(event) => onChange(event.target.value === '' ? undefined : event.target.valueAsNumber)} />{unit && <span>{unit}</span>}{error && <span id={errorId} className="portico-setting-field-error">{error}</span>}</label>;
}

export function ChoiceControl({ label, value, options, onChange, disabled = false }: { label: string; value: string; options: Array<{ value: string; label: string }>; onChange: (value: string) => void; disabled?: boolean }) {
  const association = useContext(SettingAssociationContext);
  const selected = options.find((option) => option.value === value) ?? options[0];
  if (disabled) return <span className="portico-setting-readonly">{selected?.label ?? value}</span>;
  return <SelectMenu label={label} labelledBy={association?.labelledBy} describedBy={association?.describedBy} value={selected?.value ?? value} options={options.map((option) => ({ id: option.value, label: option.label }))} onChange={onChange} />;
}

export function StringListControl({ label, value, onChange, disabled = false, placeholder }: { label: string; value: string[] | undefined; onChange: (value: string[]) => void; disabled?: boolean; placeholder?: string }) {
  return <TextControl label={label} value={(value ?? []).join(', ')} disabled={disabled} placeholder={placeholder} onChange={(next) => onChange(next.split(',').map((entry) => entry.trim()).filter(Boolean))} />;
}

export type SecretChange = { remove?: boolean; replacement?: string } | undefined;

export function SecretControl({ label, present, value, onChange, disabled = false }: { label: string; present: boolean; value: SecretChange; onChange: (value: SecretChange) => void; disabled?: boolean }) {
  const [editing, setEditing] = useState(Boolean(value?.replacement));
  const replacement = value?.replacement ?? '';
  if (editing) return <div className="portico-secret-editor"><TextControl label={label} type="password" value={replacement} disabled={disabled} placeholder="Enter replacement" onChange={(next) => onChange(next ? { replacement: next } : undefined)} /><button type="button" aria-label={`Cancel ${label} replacement`} disabled={disabled} onClick={() => { setEditing(false); onChange(undefined); }}><ActionCloseIcon /></button></div>;
  return <div className="portico-secret-state"><span className={present && !value?.remove ? 'present' : ''}><AccountSecurityIcon />{value?.remove ? 'Will be removed' : present ? 'Configured' : 'Not configured'}</span><SecondaryButton disabled={disabled} onClick={() => { setEditing(true); onChange(undefined); }}>{present ? 'Replace' : 'Add'}</SecondaryButton>{present && !value?.remove && <button type="button" className="portico-text-button danger" disabled={disabled} onClick={() => onChange({ remove: true })}>Remove</button>}{value?.remove && <button type="button" className="portico-text-button" disabled={disabled} onClick={() => onChange(undefined)}>Undo</button>}</div>;
}

export function ReadOnlyValue({ children, tone }: { children: ReactNode; tone?: 'healthy' | 'warn' | 'danger' }) {
  return <span className={`portico-setting-readonly ${tone ?? ''}`}>{children}</span>;
}

type SaveRegistration = { dirty: boolean; busy: boolean; feedback?: string; error?: string; onSave: () => void | Promise<void>; onReset: () => void };
type SaveCoordinator = {
  register: (id: string, read: () => SaveRegistration) => () => void;
  invalidate: () => void;
  requestNavigation: (destination: string) => void;
};
const SaveCoordinatorContext = createContext<SaveCoordinator | null>(null);

export function useSettingsNavigationGuard() {
  return useContext(SaveCoordinatorContext)?.requestNavigation;
}

export function SettingsSaveCoordinator({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const registrations = useRef(new Map<string, () => SaveRegistration>());
  const [, setRevision] = useState(0);
  const [showSaved, setShowSaved] = useState(false);
  const [coordinatorSaving, setCoordinatorSaving] = useState(false);
  const [pendingDestination, setPendingDestination] = useState<string>();
  const [blockedNavigation, setBlockedNavigation] = useState<SettingsBlockedNavigation>();
  const invalidate = useCallback(() => setRevision((current) => current + 1), []);
  const register = useCallback((id: string, read: () => SaveRegistration) => {
    registrations.current.set(id, read);
    invalidate();
    return () => {
      registrations.current.delete(id);
      invalidate();
    };
  }, [invalidate]);
  const entries = [...registrations.current.values()].map((read) => read());
  const dirty = entries.some((entry) => entry.dirty);
  const busy = coordinatorSaving || entries.some((entry) => entry.busy);
  const errors = entries.map((entry) => entry.error).filter(Boolean);
  const sensitiveNavigation = isSettingsNavigationSensitive();
  const saveAll = useCallback(async (): Promise<boolean> => {
    const targets = [...registrations.current.entries()]
      .map(([id, read]) => ({ id, read, entry: read() }))
      .filter(({ entry }) => entry.dirty);
    if (!targets.length) return true;
    setShowSaved(false);
    setCoordinatorSaving(true);
    const results = await Promise.allSettled(targets.map(({ entry }) => Promise.resolve().then(() => entry.onSave())));
    await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()));
    const currentErrors = targets.map(({ id, read }) => registrations.current.has(id) ? read().error : undefined).filter(Boolean);
    if (results.every((result) => result.status === 'fulfilled') && currentErrors.length === 0) setShowSaved(true);
    setCoordinatorSaving(false);
    return results.every((result) => result.status === 'fulfilled') && currentErrors.length === 0
      && targets.every(({ id, read }) => !registrations.current.has(id) || !read().dirty);
  }, []);
  const requestNavigation = useCallback((destination: string) => {
    const hasDirtyRegistration = [...registrations.current.values()].some((read) => read().dirty);
    if (hasDirtyRegistration) setPendingDestination(destination);
    else navigate(destination);
  }, [navigate]);
  const coordinator = useMemo(() => ({ register, invalidate, requestNavigation }), [invalidate, register, requestNavigation]);
  const stayOnCurrentSettings = useCallback(() => {
    blockedNavigation?.reset();
    setBlockedNavigation(undefined);
    setPendingDestination(undefined);
  }, [blockedNavigation]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if (!isSettingsNavigationDirty()) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, []);
  useEffect(() => {
    setSettingsNavigationDirty(dirty);
    return () => setSettingsNavigationDirty(false);
  }, [dirty]);
  useEffect(() => subscribeSettingsBlockedNavigation(setBlockedNavigation), []);
  useEffect(() => {
    if (!showSaved) return;
    const timer = window.setTimeout(() => setShowSaved(false), 2800);
    return () => window.clearTimeout(timer);
  }, [showSaved]);
  return <SaveCoordinatorContext.Provider value={coordinator}>
    {children}
    {entries.length > 0 && <div className="portico-settings-save-bar portico-settings-page-save">
      <div aria-live="polite" className={errors.length ? 'error' : ''}>{errors.length ? <><StatusWarningIcon />{errors[0]}</> : dirty ? 'Unsaved changes' : null}</div>
      {dirty && <SecondaryButton disabled={busy} onClick={() => entries.forEach((entry) => entry.dirty && entry.onReset())}><ActionResetIcon /> Reset</SecondaryButton>}
      <PrimaryButton disabled={!dirty || busy} onClick={() => void saveAll()}>{busy ? <><ActionRefreshIcon className="portico-settings-spinner" /> Saving…</> : <><ActionConfirmIcon /> Save changes</>}</PrimaryButton>
    </div>}
    {showSaved && errors.length === 0 && <div className="portico-settings-saved-toast" role="status" aria-live="polite"><ActionConfirmIcon /> Settings Saved</div>}
    {(pendingDestination || blockedNavigation) && <ModalOverlay className="portico-settings-dialog portico-settings-unsaved-dialog" labelledBy="settings-unsaved-title" describedBy="settings-unsaved-description" onDismiss={stayOnCurrentSettings}>
        <div><StatusWarningIcon /><h2 id="settings-unsaved-title">{sensitiveNavigation ? 'Save your API key' : 'Unsaved settings'}</h2><p id="settings-unsaved-description">{sensitiveNavigation ? 'This API key is shown only once. Copy it or confirm that you saved it before leaving this page.' : 'Save or discard your changes before opening another settings section.'}</p></div>
        <footer>
          <SecondaryButton disabled={busy} onClick={stayOnCurrentSettings}>Stay</SecondaryButton>
          {!sensitiveNavigation && <><SecondaryButton disabled={busy} onClick={() => { entries.forEach((entry) => entry.dirty && entry.onReset()); const destination = pendingDestination; const blocked = blockedNavigation; setBlockedNavigation(undefined); setPendingDestination(undefined); if (blocked) blocked.proceed(); else if (destination) navigate(destination); }}>Discard</SecondaryButton>
          <PrimaryButton disabled={busy} onClick={() => void saveAll().then((saved) => { if (!saved) return; const destination = pendingDestination; const blocked = blockedNavigation; setBlockedNavigation(undefined); setPendingDestination(undefined); if (blocked) blocked.proceed(); else if (destination) navigate(destination); })}>{busy ? 'Saving…' : 'Save and continue'}</PrimaryButton></>}
        </footer>
    </ModalOverlay>}
  </SaveCoordinatorContext.Provider>;
}

export function SaveBar(props: { dirty: boolean; busy: boolean; feedback?: string; error?: string; onSave: () => void | Promise<void>; onReset: () => void }) {
  const { dirty, busy, feedback, error, onSave, onReset } = props;
  const coordinator = useContext(SaveCoordinatorContext);
  const id = useId();
  const current = useRef<SaveRegistration>(props);
  current.current = props;
  useEffect(() => coordinator?.register(id, () => current.current), [coordinator, id]);
  useEffect(() => coordinator?.invalidate(), [busy, coordinator, dirty, error, feedback]);
  const [showSaved, setShowSaved] = useState(false);
  useEffect(() => {
    if (!feedback) return;
    setShowSaved(true);
    const timer = window.setTimeout(() => setShowSaved(false), 2800);
    return () => window.clearTimeout(timer);
  }, [feedback]);
  if (coordinator) return error ? <div className="portico-settings-inline-save-error" role="alert"><StatusWarningIcon />{error}</div> : null;
  return <><div className="portico-settings-save-bar">
    <div aria-live="polite" className={error ? 'error' : ''}>{error ? <><StatusWarningIcon />{error}</> : dirty ? 'Unsaved changes' : null}</div>
    {dirty && <SecondaryButton disabled={busy} onClick={onReset}><ActionResetIcon /> Reset</SecondaryButton>}
    <PrimaryButton disabled={!dirty || busy} onClick={onSave}>{busy ? <><ActionRefreshIcon className="portico-settings-spinner" /> Saving…</> : <><ActionConfirmIcon /> Save changes</>}</PrimaryButton>
  </div>{showSaved && !error && <div className="portico-settings-saved-toast" role="status" aria-live="polite"><ActionConfirmIcon /> Settings Saved</div>}</>;
}

export function SettingsLoading({ label = 'Loading settings' }: { label?: string }) {
  return <div className="portico-settings-state" aria-live="polite" aria-busy="true"><ActionRefreshIcon className="portico-settings-spinner" /><strong>{label}</strong></div>;
}

export function SettingsError({ title, message, onRetry }: { title: string; message: string; onRetry?: () => void }) {
  return <div className="portico-settings-state error" role="alert"><StatusWarningIcon /><strong>{title}</strong><p>{message}</p>{onRetry && <SecondaryButton onClick={onRetry}><ActionRefreshIcon /> {productText('action.retry')}</SecondaryButton>}</div>;
}

export function InlineNotice({ children, tone = 'info', action }: { children: ReactNode; tone?: 'info' | 'success' | 'warn' | 'error'; action?: ReactNode }) {
  return <div className={`portico-settings-notice ${tone}`} role={tone === 'error' ? 'alert' : 'status'} aria-live={tone === 'error' ? 'assertive' : 'polite'}><span>{children}</span>{action}</div>;
}
