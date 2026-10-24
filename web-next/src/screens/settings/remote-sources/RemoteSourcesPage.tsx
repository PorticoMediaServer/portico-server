import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {
  RemoteSourceService, sourceReadPreset, sourceReadPresetName,
  type DAVConnectionResult, type RemoteSource, type SourceReadPreset, type SourceReadSettings, type SourceReadValues,
} from '@core/remote-sources.ts';
import {davConfiguration, stableStorageOperationId, type WebDavDraft} from '@core/server-admin/panel-logic.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {Button, RichSelect, Switch, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsGroup, SettingNumber} from '../../../kit/settings';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import {Icon} from '../../../kit/Icon';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;
const emptyDraft: WebDavDraft = {name: '', root: '', username: '', password: '', insecureLocal: false};
const presetKeys: readonly SourceReadPreset[] = ['list', 'once', 'everything', 'ask'];
const presetLabels: Readonly<Record<SourceReadPreset, MessageId>> = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
};
const presetHints: Readonly<Record<SourceReadPreset, MessageId>> = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.hint',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.hint',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.hint',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.hint',
};
const presetLabel = (name: SourceReadPreset): MessageId => presetLabels[name];
const presetHint = (name: SourceReadPreset): MessageId => presetHints[name];
const presetOptions = presetKeys.map(name => ({value: name, label: t(presetLabel(name)), description: t(presetHint(name))}));

function stateId(source: RemoteSource): MessageId {
  switch (source.errorCode) {
    case 'offline': return 'next.remoteSources.state.unreachable';
    case 'credentials_required': return 'next.remoteSources.state.credentials';
    case 'rate_limited': case 'quota_exceeded': return 'next.remoteSources.state.waiting';
    case 'range_unsupported': return 'next.remoteSources.state.limited';
    case 'invalid_configuration': case 'budget_exceeded': return 'next.remoteSources.state.attention';
    case 'cancelled': return 'next.remoteSources.state.stopped';
    case 'source_changed': case 'cursor_expired': return 'next.remoteSources.state.checking';
  }
  if (source.state === 'ready' || source.state === 'healthy') return 'next.remoteSources.state.ready';
  if (source.state === 'checking' || source.state === 'pending') return 'next.remoteSources.state.checking';
  if (source.state === 'unreachable' || source.state === 'degraded') return 'next.remoteSources.state.unreachable';
  if (source.state === 'credentials_required' || source.state === 'unauthorized') return 'next.remoteSources.state.credentials';
  return 'next.remoteSources.state.problem';
}
function stateTone(source: RemoteSource): 'healthy' | 'warning' | 'danger' | 'neutral' {
  if (source.errorCode === 'offline' || source.errorCode === 'credentials_required' || source.errorCode === 'binary_changed') return 'danger';
  if (source.errorCode === 'rate_limited' || source.errorCode === 'quota_exceeded' || source.errorCode === 'range_unsupported') return 'warning';
  if (source.errorCode) return 'danger';
  return source.state === 'ready' || source.state === 'healthy' ? 'healthy' : 'neutral';
}
function problemId(source: RemoteSource): MessageId | undefined {
  const id = source.errorCode && `next.remoteSources.problem.${source.errorCode}`;
  return id && defaultI18n.has(id) ? id as MessageId : undefined;
}
function SourceStatus({source}: {source: RemoteSource}) {
  return <span className={styles.badge} data-tone={stateTone(source)}>{t(stateId(source))}</span>;
}
export function remoteErrorText(error: unknown): string {
  const code = error && typeof error === 'object' && typeof (error as {code?: unknown}).code === 'string' ? (error as {code: string}).code : '';
  switch (code) {
    case 'remote_credentials_required': case 'source_credentials_required': return t('next.remoteSources.problem.credentials_required');
    case 'remote_invalid_configuration': case 'invalid_request': return t('next.remoteSources.error.invalid');
    case 'remote_range_unsupported': return t('next.remoteSources.problem.range_unsupported');
    case 'remote_rate_limited': return t('next.remoteSources.problem.rate_limited');
    case 'revision_mismatch': case 'storage_command_conflict': return t('next.remoteSources.error.changed');
    case 'idempotency_key_reused': return t('next.remoteSources.error.reused');
    case 'request_in_progress': return t('next.remoteSources.error.busy');
    case 'unauthorized': return t('next.remoteSources.error.signIn');
    case 'forbidden': case 'not_permitted': return t('next.remoteSources.error.forbidden');
    case 'refresh_failed': return t('next.remoteSources.error.refreshFailed');
    default: return t('next.remoteSources.error.generic');
  }
}
function refreshFailure(error: unknown): boolean {
  return !!error && typeof error === 'object' && (error as {code?: string}).code === 'refresh_failed';
}

export type RemoteSourcesPageProps = Readonly<{
  api: LibraryContentApi; serverId: string; viewerId: string; isCurrent: () => boolean;
}>;

/** Server › Remote sources. The drawn page over the source list, the WebDAV add
 * flow and the source's reading profile; only routes the server actually has. */
export function RemoteSourcesPage({api, serverId, viewerId, isCurrent}: RemoteSourcesPageProps) {
  const [ids] = useState(stableStorageOperationId);
  const service = useMemo(() => new RemoteSourceService(api, {serverId, viewerId}, ids.next), [api, serverId, viewerId, ids]);
  const [adding, setAdding] = useState(false);
  const [managing, setManaging] = useState<RemoteSource>();
  useEffect(() => {
    void service.load();
    return () => service.dispose();
  }, [service]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const sources = snapshot.sources.filter(source => source.kind === 'webdav');
  const [notice, setNotice] = useState('');
  const refresh = () => {if (!isCurrent()) return;setNotice('');void service.load();};
  const recover = () => {if (!isCurrent()) return;setNotice('');void service.recover().catch(error => {if (isCurrent()) setNotice(remoteErrorText(error));});};
  return <div className={styles.page}>
    <SettingsGroup title={t('next.settingsSpec.server.remote-sources.groups.0.title')}
      actions={<Button size="compact" variant="primary" icon="i-plus" onClick={() => setAdding(true)}>{t('next.settingsSpec.server.remote-sources.groups.0.rows.0.control.addLabel')}</Button>}>
      {snapshot.error && <p className={styles.error} role="alert">{snapshot.error}{snapshot.ambiguous && <Button size="xs" variant="text" onClick={recover}>{t('next.remoteSources.checkResult')}</Button>}</p>}
      {notice && <p className={styles.error} role="alert">{notice}</p>}
      {snapshot.loading && !sources.length && <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
      {!snapshot.loading && !sources.length && !snapshot.error && <div className={styles.empty}><b>{t('next.remoteSources.empty')}</b><p>{t('next.remoteSources.emptyHelp')}</p></div>}
      {sources.map(source => <SettingsRow key={source.id} label={<><Icon name="i-globe" size={18} /> {source.name}</>}
        help={<>{t('next.remoteSources.kindWebdav')}{source.origin ? ` · ${source.origin}` : ''}{problemId(source) ? ` · ${t(problemId(source)!)}` : ''}</>}>
        <span className={styles.rowActions}><SourceStatus source={source}/><Button size="small" variant="glass" onClick={() => setManaging(source)}>{t('next.settingsSpec.server.remote-sources.groups.0.rows.0.control.itemActions.0.label')}</Button></span>
      </SettingsRow>)}
      {!snapshot.loading && <p className={styles.note}>{t('next.remoteSources.foldersBlocked')}</p>}
    </SettingsGroup>
    <AddSourceDialog service={service} open={adding} isCurrent={isCurrent} onClose={() => setAdding(false)} onCreated={source => {setAdding(false);setManaging(source);}}/>
    {managing && <ManageSourceDialog service={service} source={managing} isCurrent={isCurrent} onClose={() => setManaging(undefined)} onRemoved={() => setManaging(undefined)}/>}
    {snapshot.loading && sources.length > 0 && <Button size="xs" variant="text" onClick={refresh}>{t('next.remoteSources.refresh')}</Button>}
  </div>;
}

function AddSourceDialog({service, open, isCurrent, onClose, onCreated}: Readonly<{
  service: RemoteSourceService; open: boolean; isCurrent: () => boolean; onClose: () => void; onCreated: (source: RemoteSource) => void;
}>) {
  const [step, setStep] = useState<'connection'|'reading'>('connection');
  const [draft, setDraft] = useState<WebDavDraft>(emptyDraft);
  const [result, setResult] = useState<DAVConnectionResult>();
  const [error, setError] = useState('');
  const [created, setCreated] = useState<RemoteSource>();
  const nextSourceId = useRef('');
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('once'));
  const [busy, setBusy] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(false);
  const testAttempt = useRef(0);
  const reset = () => {setStep('connection');setDraft(emptyDraft);setResult(undefined);setError('');setCreated(undefined);setProfile(undefined);setValues(sourceReadPreset('once'));setBusy(false);setLoadingProfile(false);testAttempt.current++;};
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {if (!open) reset();}, [open]);
  const test = async () => {
    if (!isCurrent()) return;
    const attempt = ++testAttempt.current, captured = {...draft};
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testConnection({name: captured.name.trim(), root: captured.root.trim(), username: captured.username, password: captured.password, insecureLocal: captured.insecureLocal});
      if (isCurrent() && attempt === testAttempt.current) setResult(next);}
    catch (value) {if (isCurrent() && attempt === testAttempt.current) setError(remoteErrorText(value));}
    finally {if (isCurrent() && attempt === testAttempt.current) setBusy(false);}
  };
  const create = async () => {
    if (!isCurrent() || !result) return;
    setError('');setBusy(true);
    try {
      await service.configure(null, davConfiguration(null, draft));
      const state = service.getSnapshot();
      if (state.error || !state.receipt) {if (isCurrent()) setError(state.error || 'The source was not confirmed.');return;}
      const source = state.receipt.source;setCreated(source);nextSourceId.current = source.id;setStep('reading');setLoadingProfile(true);setDraft(current => ({...current, password: ''}));
      try {const next = await service.readProfile(source.id, String(source.generation));if (isCurrent()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (isCurrent()) setError(remoteErrorText(value));}
      finally {if (isCurrent()) setLoadingProfile(false);}
    } finally {if (isCurrent()) setBusy(false);}
  };
  const saveReading = async () => {
    if (!isCurrent() || !profile) return;
    setError('');setBusy(true);
    try {
      const next = await service.saveProfile(profile, values);
      if (!isCurrent()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      setProfile(next);
      const saved = created ?? service.getSnapshot().sources.find(source => source.id === next.sourceId);
      if (saved) onCreated(saved); else onClose();
    } catch (value) {
      if (!isCurrent()) return;
      const saved = created ?? service.getSnapshot().sources.find(source => source.id === nextSourceId.current);
      if (refreshFailure(value) && saved) onCreated(saved); else setError(remoteErrorText(value));
    } finally {if (isCurrent()) setBusy(false);}
  };
  const close = () => {if (busy) return;setDraft(emptyDraft);setResult(undefined);setError('');onClose();};
  const footer = step === 'connection'
    ? <><Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button><Button size="compact" variant="primary" disabled={busy || !result} onClick={create}>{t('next.remoteSources.saveAndContinue')}</Button></>
    : <><Button size="compact" variant="glass" disabled={busy} onClick={() => setStep('connection')}>{t('next.remoteSources.back')}</Button><Button size="compact" variant="primary" disabled={busy || loadingProfile || !profile} onClick={saveReading}>{t('next.remoteSources.save')}</Button></>;
  return <Dialog open={open} onOpenChange={value => {if (!value) close();}} title={t('next.settingsSpec.server.remote-sources.panels.newSource.newTitle')} width={720}
    description={step === 'connection' ? t('next.remoteSources.step', {step: 1}) : t('next.remoteSources.step', {step: 2})} footer={footer}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {step === 'connection' ? <>
      <div className={styles.kind}><Icon name="i-globe" size={22} /><span><b>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label')}</b><small>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.hint')}</small></span></div>
      <p className={styles.note}>{t('next.remoteSources.kindsBlocked')}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.0.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'}} onChange={event => updateDraft({root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.1.control.placeholder')} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.2.control.placeholder')} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
      </div>
    </> : <>
      <p className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.label')}</p>
      <RichSelect aria-label={t('next.remoteSources.readingMode')} value={sourceReadPresetName(values) ?? 'custom'} options={[...presetOptions, ...(sourceReadPresetName(values) ? [] : [{value: 'custom', label: t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.4.label'), disabled: true}])]}
        onValueChange={value => {if (value !== 'custom') setValues(sourceReadPreset(value as SourceReadPreset));}}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={busy} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>
      <p className={styles.note}>{t('next.remoteSources.foldersBlocked')}</p>
    </>}
  </Dialog>;
}

function ManageSourceDialog({service, source, isCurrent, onClose, onRemoved}: Readonly<{
  service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean; onClose: () => void; onRemoved: () => void;
}>) {
  const [draft, setDraft] = useState<WebDavDraft>({name: source.name, root: '', username: '', password: '', insecureLocal: source.insecureLocal});
  const [result, setResult] = useState<DAVConnectionResult>();
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [typed, setTyped] = useState('');
  const [confirming, setConfirming] = useState(false);
  const testAttempt = useRef(0);
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {
    let active = true;
    service.readProfile(source.id, String(source.generation)).then(next => {
      if (!active || !isCurrent()) return;
      setProfile(next);
      setValues(next.saved ? next.values : sourceReadPreset('list'));
    }).catch(value => {if (active && isCurrent()) setError(remoteErrorText(value));});
    return () => {active = false;};
  }, [service, source.id, isCurrent]);
  const test = async () => {
    if (!isCurrent()) return;
    const attempt = ++testAttempt.current, captured = {...draft};
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testConnection({name: captured.name.trim(), root: captured.root.trim(), username: captured.username, password: captured.password, insecureLocal: captured.insecureLocal});
      if (isCurrent() && attempt === testAttempt.current) setResult(next);}
    catch (value) {if (isCurrent() && attempt === testAttempt.current) setError(remoteErrorText(value));}
    finally {if (isCurrent() && attempt === testAttempt.current) setBusy(false);}
  };
  const saveConnection = async () => {
    if (!isCurrent()) return;
    setError('');setBusy(true);
    try {
      await service.configure(source.id, davConfiguration(source, draft), source);
      if (!isCurrent()) return;
      const state = service.getSnapshot();
      if (state.error) {setError(state.error);return;}
      close();
    } finally {if (isCurrent()) setBusy(false);}
  };
  const saveReading = async () => {
    if (!isCurrent() || !profile) return;
    setError('');setBusy(true);
    try {const next = await service.saveProfile(profile, values);if (!isCurrent()) return;if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}setProfile(next);}
    catch (value) {if (!isCurrent()) return;if (refreshFailure(value)) close(); else setError(remoteErrorText(value));}
    finally {if (isCurrent()) setBusy(false);}
  };
  const disconnect = async () => {
    if (!isCurrent() || typed.trim() !== source.name) return;
    const current = service.getSnapshot().sources.find(value => value.id === source.id && value.generation === source.generation);
    if (!current) {setError(remoteErrorText({code: 'revision_mismatch'}));return;}
    setError('');setBusy(true);
    try {await service.remove(current);const state = service.getSnapshot();if (!isCurrent()) return;if (state.error) {setError(state.error);return;}onRemoved();}
    finally {if (isCurrent()) setBusy(false);}
  };
  const close = () => {if (busy) return;setDraft(current => ({...current, password: ''}));setTyped('');onClose();};
  return <Dialog open onOpenChange={value => {if (!value) close();}} title={source.name} width={760} footer={<Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.group}><h3>{t('next.remoteSources.connection')}</h3>
      <p className={styles.note}><SourceStatus source={source}/> {source.origin ?? ''}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={source.origin ?? t('next.settingsSpec.server.remote-sources.panels.source.tabs.0.groups.0.rows.1.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'}} onChange={event => updateDraft({root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
      </div>
      <Button size="compact" variant="primary" disabled={busy || !draft.name.trim() || (!result && !draft.root.trim())} onClick={() => void saveConnection()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>
      <RichSelect aria-label={t('next.remoteSources.readingMode')} value={sourceReadPresetName(values) ?? 'custom'} options={[...presetOptions, ...(sourceReadPresetName(values) ? [] : [{value: 'custom', label: t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.4.label'), disabled: true}])]}
        onValueChange={value => {if (value !== 'custom') setValues(sourceReadPreset(value as SourceReadPreset));}}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={busy} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>
      <Button size="compact" variant="primary" disabled={busy || !profile} onClick={() => void saveReading()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.danger}><h3>{t('next.remoteSources.disconnect')}</h3><p>{t('next.remoteSources.disconnectBody')}</p>
      {!confirming ? <Button size="compact" variant="text" onClick={() => setConfirming(true)}>{t('next.remoteSources.disconnect')}</Button> : <>
        <p>{t('next.remoteSources.disconnectConfirm')}</p>
        <TextField label={t('next.remoteSources.name')} value={typed} disabled={busy} onChange={event => setTyped(event.target.value)}/>
        <div className={styles.actions}><Button size="compact" variant="glass" disabled={busy} onClick={() => {setConfirming(false);setTyped('');}}>{t('next.remoteSources.cancel')}</Button>
          <Button size="compact" variant="primary" disabled={busy || typed.trim() !== source.name} onClick={() => void disconnect()}>{t('next.remoteSources.disconnect')}</Button></div>
      </>}
    </section>
  </Dialog>;
}
