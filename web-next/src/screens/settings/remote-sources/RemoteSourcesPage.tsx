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
function errorText(error: unknown): string {
  return error instanceof Error ? error.message : '';
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
  const refresh = () => {setNotice('');void service.load();};
  const recover = () => {setNotice('');void service.recover().catch(error => {if (isCurrent()) setNotice(errorText(error));});};
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
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('once'));
  const [busy, setBusy] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(false);
  const reset = () => {setStep('connection');setDraft(emptyDraft);setResult(undefined);setError('');setCreated(undefined);setProfile(undefined);setValues(sourceReadPreset('once'));setBusy(false);setLoadingProfile(false);};
  useEffect(() => {if (open) reset();}, [open]);
  const test = async () => {
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testConnection({name: draft.name.trim(), root: draft.root.trim(), username: draft.username, password: draft.password, insecureLocal: draft.insecureLocal});if (isCurrent()) setResult(next);}
    catch (value) {if (isCurrent()) setError(errorText(value));}
    finally {if (isCurrent()) setBusy(false);}
  };
  const create = async () => {
    if (!result) return;
    setError('');setBusy(true);
    try {
      await service.configure(null, davConfiguration(null, draft));
      const state = service.getSnapshot();
      if (state.error || !state.receipt) {if (isCurrent()) setError(state.error || 'The source was not confirmed.');return;}
      const source = state.receipt.source;setCreated(source);setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(source.id);if (isCurrent()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (isCurrent()) setError(errorText(value));}
      finally {if (isCurrent()) setLoadingProfile(false);}
    } finally {if (isCurrent()) setBusy(false);}
  };
  const saveReading = async () => {
    if (!profile) return;
    setError('');setBusy(true);
    try {
      const next = await service.saveProfile(profile, values);
      if (!isCurrent()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      setProfile(next);
      const saved = created ?? service.getSnapshot().sources.find(source => source.id === next.sourceId);
      if (saved) onCreated(saved); else onClose();
    } catch (value) {if (isCurrent()) setError(errorText(value));}
    finally {if (isCurrent()) setBusy(false);}
  };
  const close = () => {if (!busy) onClose();};
  const footer = step === 'connection'
    ? <><Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button><Button size="compact" variant="primary" disabled={busy || !result} onClick={create}>{t('next.remoteSources.saveAndContinue')}</Button></>
    : <><Button size="compact" variant="glass" disabled={busy} onClick={() => setStep('connection')}>{t('next.remoteSources.back')}</Button><Button size="compact" variant="primary" disabled={busy || loadingProfile || !profile} onClick={saveReading}>{t('next.remoteSources.save')}</Button></>;
  return <Dialog open={open} onOpenChange={value => {if (!value) close();}} title={t('next.settingsSpec.server.remote-sources.panels.newSource.newTitle')} width={720}
    description={step === 'connection' ? t('next.remoteSources.step', {step: 1}) : t('next.remoteSources.step', {step: 2})} footer={footer}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {step === 'connection' ? <>
      <div className={styles.kind}><Icon name="i-globe" size={22} /><span><b>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label')}</b><small>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.hint')}</small></span></div>
      <p className={styles.note}>{t('next.remoteSources.kindsBlocked')}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => setDraft({...draft, name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.0.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'}} onChange={event => setDraft({...draft, root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.1.control.placeholder')} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => setDraft({...draft, username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.2.control.placeholder')} disabled={busy} onChange={event => setDraft({...draft, password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => setDraft({...draft, insecureLocal: value})}/></label>
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
  useEffect(() => {
    let active = true;
    service.readProfile(source.id).then(next => {
      if (!active || !isCurrent()) return;
      setProfile(next);
      setValues(next.saved ? next.values : sourceReadPreset('list'));
    }).catch(value => {if (active && isCurrent()) setError(errorText(value));});
    return () => {active = false;};
  }, [service, source.id, isCurrent]);
  const test = async () => {setError('');setResult(undefined);setBusy(true);try {const next = await service.testConnection({name: draft.name.trim(), root: draft.root.trim(), username: draft.username, password: draft.password, insecureLocal: draft.insecureLocal});if (isCurrent()) setResult(next);}catch (value) {if (isCurrent()) setError(errorText(value));}finally {if (isCurrent()) setBusy(false);}};
  const saveConnection = async () => {
    setError('');setBusy(true);
    try {
      await service.configure(source.id, davConfiguration(source, draft), source);
      const state = service.getSnapshot();
      if (state.error) {if (isCurrent()) setError(state.error);return;}
      if (isCurrent()) onClose();
    } finally {if (isCurrent()) setBusy(false);}
  };
  const saveReading = async () => {
    if (!profile) return;
    setError('');setBusy(true);
    try {const next = await service.saveProfile(profile, values);if (!isCurrent()) return;if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}setProfile(next);}
    catch (value) {if (isCurrent()) setError(errorText(value));}
    finally {if (isCurrent()) setBusy(false);}
  };
  const disconnect = async () => {
    setError('');setBusy(true);
    try {await service.remove(source);const state = service.getSnapshot();if (state.error) {if (isCurrent()) setError(state.error);return;}if (isCurrent()) onRemoved();}
    finally {if (isCurrent()) setBusy(false);}
  };
  return <Dialog open onOpenChange={value => {if (!value && !busy) onClose();}} title={source.name} width={760} footer={<Button size="compact" variant="glass" disabled={busy} onClick={onClose}>{t('next.remoteSources.cancel')}</Button>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.group}><h3>{t('next.remoteSources.connection')}</h3>
      <p className={styles.note}><SourceStatus source={source}/> {source.origin ?? ''}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => setDraft({...draft, name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={source.origin ?? t('next.settingsSpec.server.remote-sources.panels.source.tabs.0.groups.0.rows.1.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'}} onChange={event => setDraft({...draft, root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => setDraft({...draft, username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} disabled={busy} onChange={event => setDraft({...draft, password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => setDraft({...draft, insecureLocal: value})}/></label>
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
