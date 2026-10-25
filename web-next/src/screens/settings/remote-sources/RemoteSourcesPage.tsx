import {useCallback, useLayoutEffect, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {
  RemoteSourceService, driveServiceAccountDraft, s3SourceDraft, sourceReadPreset, sourceReadPresetName,
  type DAVConnectionResult, type DriveKeyFields, type RcloneConfigView, type RcloneProgram, type RcloneTestResult, type RemoteFolder, type RemoteSource, type S3SourceFields, type RemoteSourceStatus, type SourceAdvanced, type SourceReadPreset, type SourceReadSettings, type SourceReadValues,
} from '@core/remote-sources.ts';
import {LibraryManagementService, type ManagedLibrary} from '@core/library-management.ts';
import {readLibraryInventoryConfig, saveLibraryFolder} from '@core/server-admin/libraries.ts';
import {davConfiguration, stableStorageOperationId, type WebDavDraft} from '@core/server-admin/panel-logic.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {Button, RichSelect, Switch, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsGroup, SettingNumber} from '../../../kit/settings';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import {Icon, type IconName} from '../../../kit/Icon';
import styles from './remote-sources.module.css';
import {RemoteFolderPicker} from './RemoteFolderPicker';
import {ReadingChoices} from './ReadingChoices';
import {SourceAdvancedRows} from './SourceAdvancedRows';
import {ProgramPicker} from './ProgramPicker';

const t = defaultI18n.t;
const emptyDraft: WebDavDraft = {name: '', root: '', username: '', password: '', insecureLocal: false};
type SourceKind = 'webdav' | 'rclone' | 'config' | 's3' | 'drive';
type RcloneDraft = {name: string; remote: string; config: string};
const emptyRclone: RcloneDraft = {name: '', remote: '', config: ''};
const emptyS3: S3SourceFields = {name: '', endpoint: '', region: '', accessKey: '', secretKey: '', bucket: '', folder: ''};
const emptyDrive: DriveKeyFields = {name: '', keyJson: '', location: 'folder', locationId: ''};
const kindOptions = [
  {value: 'webdav', label: t('next.remoteSources.connectWebdav')},
  {value: 'rclone', label: t('next.remoteSources.connectRclone')},
  {value: 'config', label: t('next.remoteSources.connectFromConfig')},
  {value: 's3', label: t('next.remoteSources.connectS3')},
  {value: 'drive', label: t('next.remoteSources.connectDrive')},
];
const presetLabels: Readonly<Record<SourceReadPreset, MessageId>> = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
};
const presetLabel = (name: SourceReadPreset): MessageId => presetLabels[name];

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
    case 'source_folders_unsupported': return t('next.remoteSources.foldersUnsupported');
    default: return t('next.remoteSources.error.generic');
  }
}
function authFailure(error: unknown): boolean {
  const code = error && typeof error === 'object' ? (error as {code?: string}).code : '';
  return code === 'unauthorized' || code === 'forbidden' || code === 'not_permitted' || code === 'remote_credentials_required' || code === 'source_credentials_required';
}
function refreshFailure(error: unknown): boolean {
  return !!error && typeof error === 'object' && (error as {code?: string}).code === 'refresh_failed';
}
/** The server's own check time, in the owner's local words. */
function checkedWhen(value: string): string {
  const when = new Date(value);
  if (Number.isNaN(when.getTime())) return value;
  try { return new Intl.DateTimeFormat(undefined, {dateStyle: 'medium', timeStyle: 'short'}).format(when); }
  catch { return when.toISOString(); }
}
/** A folder attachment failure names the one thing the owner can act on: the
 * folders are saved on the source either way, so this never reads as data loss. */
function attachFailure(error: unknown): string {
  const code = error && typeof error === 'object' && typeof (error as {code?: unknown}).code === 'string' ? (error as {code: string}).code : '';
  if (code === 'source_already_configured') return t('next.remoteSources.folderAlreadyAdded');
  if (code === 'library_configuration_conflict' || code === 'revision_mismatch') return t('next.remoteSources.error.changed');
  if (code === 'unauthorized' || code === 'forbidden' || code === 'not_permitted') return t('next.remoteSources.error.forbidden');
  return t('next.remoteSources.attachFailed');
}

function ProgramNote({program}: {program?: RcloneProgram}) {
  if (!program) return <p className={styles.note}>{t('next.remoteSources.bundledProgram')}</p>;
  if (!program.available) return <p className={styles.notice} role="status">{t('next.remoteSources.bundledUnavailable')}</p>;
  return <p className={styles.note}>{program.version ? t('next.remoteSources.bundledVersion', {version: program.version}) : t('next.remoteSources.bundledProgram')}</p>;
}

export type RemoteSourcesPageProps = Readonly<{
  api: LibraryContentApi; serverId: string; viewerId: string; isCurrent: () => boolean; reportError: (error: unknown) => void;
  openSection?: (section: string) => void;
}>;

/** Server › Remote sources. The drawn page over the source list, the WebDAV add
 * flow and the source's reading profile; only routes the server actually has. */
export function RemoteSourcesPage({api, serverId, viewerId, isCurrent, reportError, openSection}: RemoteSourcesPageProps) {
  const [ids] = useState(stableStorageOperationId);
  const service = useMemo(() => new RemoteSourceService(api, {serverId, viewerId}, ids.next, isCurrent), [api, serverId, viewerId, ids, isCurrent]);
  const [adding, setAdding] = useState(false);
  const [managing, setManaging] = useState<RemoteSource>();
  const [reading, setReading] = useState<RemoteSource>();
  useEffect(() => {
    void service.load();
    return () => service.dispose();
  }, [service]);
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const sources = snapshot.sources.filter(source => source.kind === 'webdav' || source.kind === 'rclone');
  const [notice, setNotice] = useState('');
  const refresh = () => {if (!isCurrent()) return;setNotice('');void service.load();};
  const recover = () => {if (!isCurrent()) return;setNotice('');void service.recover().catch(error => {if (isCurrent()) setNotice(remoteErrorText(error));});};
  const needing = sources.filter(source => stateTone(source) === 'danger');
  const attention = needing[0];
  const attentionMore = needing.length > 1 ? t('next.remoteSources.problemMore', {count: needing.length - 1}) : '';
  return <div className={styles.page}>
    {attention && <div className={styles.pageNotice} role="status">
      <Icon name="i-warn" size={20}/>
      <span><b>{t('next.remoteSources.problemHeading')}</b> {attention.name}: {problemId(attention) ? t(problemId(attention)!) : t('next.remoteSources.problem.offline')}{attentionMore}
        <Button size="xs" variant="text" onClick={() => setManaging(attention)}>{t('next.remoteSources.openSource', {name: attention.name})}</Button></span>
    </div>}
    <SettingsGroup title={t('next.settingsSpec.server.remote-sources.groups.0.title')}
      actions={<Button size="compact" variant="primary" icon="i-plus" onClick={() => setAdding(true)}>{t('next.settingsSpec.server.remote-sources.groups.0.rows.0.control.addLabel')}</Button>}>
      {snapshot.error && <p className={styles.error} role="alert">{snapshot.error}{snapshot.ambiguous && <Button size="xs" variant="text" onClick={recover}>{t('next.remoteSources.checkResult')}</Button>}</p>}
      {notice && <p className={styles.error} role="alert">{notice}</p>}
      {snapshot.loading && !sources.length && <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
      {!snapshot.loading && !sources.length && !snapshot.error && <div className={styles.empty}><b>{t('next.remoteSources.empty')}</b><p>{t('next.remoteSources.emptyHelp')}</p></div>}
      {sources.map(source => <SettingsRow key={source.id} label={<><Icon name="i-globe" size={18} /> {source.name}</>}
        help={<>{t(source.kind === 'rclone' ? 'web.storage.rcloneRemote' : 'next.remoteSources.kindWebdav')}{source.origin ? ` · ${source.origin}` : ''}{problemId(source) ? ` · ${t(problemId(source)!)}` : ''}</>}>
        <span className={styles.rowActions}><SourceStatus source={source}/><Button size="small" variant="glass" onClick={() => setManaging(source)}>{t('next.settingsSpec.server.remote-sources.groups.0.rows.0.control.itemActions.0.label')}</Button>{source.kind === 'rclone' && <Button size="small" variant="glass" onClick={() => setReading(source)}>{t('next.remoteSources.reading')}</Button>}</span>
      </SettingsRow>)}
    </SettingsGroup>
    <AddSourceDialog service={service} api={api} serverId={serverId} viewerId={viewerId} open={adding} isCurrent={isCurrent} reportError={reportError} onClose={() => setAdding(false)} onCreated={source => {setAdding(false);setManaging(source);}}/>
    {managing && (managing.kind === 'rclone'
      ? <ManageRcloneDialog key={managing.id} service={service} source={managing} isCurrent={isCurrent} reportError={reportError} onClose={() => setManaging(undefined)} onRemoved={() => setManaging(undefined)}/>
      : <ManageSourceDialog key={managing.id} service={service} source={managing} api={api} serverId={serverId} viewerId={viewerId} isCurrent={isCurrent} reportError={reportError} onClose={() => setManaging(undefined)} onRemoved={() => setManaging(undefined)}/>)}
    {reading && <SourceReadingDialog key={`${reading.id}:${reading.generation}`} service={service} source={reading} isCurrent={isCurrent} reportError={reportError} onClose={() => setReading(undefined)}/>}
    {snapshot.loading && sources.length > 0 && <Button size="xs" variant="text" onClick={refresh}>{t('next.remoteSources.refresh')}</Button>}
  </div>;
}

type WizardStep = 'connection' | 'reading' | 'folders' | 'done';
type AttachMode = 'new' | 'existing';
type LibraryKind = ManagedLibrary['kind'];

/** The seven drawn kinds, named and hinted from the catalogue (owner-libraries.html#8). */
const libraryKinds: readonly {id: LibraryKind; icon: IconName; label: MessageId; hint: MessageId}[] = [
  {id: 'movie', icon: 'i-film', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.0.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.0.hint'},
  {id: 'tv', icon: 'i-tvlib', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.1.hint'},
  {id: 'anime', icon: 'i-tvlib', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.2.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.2.hint'},
  {id: 'music', icon: 'i-music', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.3.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.3.hint'},
  {id: 'audiobook', icon: 'i-headph', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.4.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.4.hint'},
  {id: 'book', icon: 'i-sbook', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.5.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.5.hint'},
  {id: 'comic', icon: 'i-page', label: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.6.label', hint: 'next.settingsSpec.server.libraries.panels.newLibrary.tabs.0.groups.0.rows.1.control.options.6.hint'},
];
const SIX_HOURS = 21_600;

export function AddSourceDialog({service, api, serverId, viewerId, open, isCurrent, reportError, onClose, onCreated}: Readonly<{
  service: RemoteSourceService; api: LibraryContentApi; serverId: string; viewerId: string; open: boolean;
  isCurrent: () => boolean; reportError: (error: unknown) => void; onClose: () => void; onCreated: (source: RemoteSource) => void;
}>) {
  const [step, setStep] = useState<WizardStep>('connection');
  const [sourceKind, setSourceKind] = useState<SourceKind>('webdav');
  const [draft, setDraft] = useState<WebDavDraft>(emptyDraft);
  const [rclone, setRclone] = useState<RcloneDraft>(emptyRclone);
  const [program, setProgram] = useState<RcloneProgram>();
  const [providers, setProviders] = useState<readonly string[]>([]);
  const [configView, setConfigView] = useState<RcloneConfigView>();
  const [configRemote, setConfigRemote] = useState('');
  const [configName, setConfigName] = useState('');
  const [s3, setS3] = useState<S3SourceFields>(emptyS3);
  const [drive, setDrive] = useState<DriveKeyFields>(emptyDrive);
  const keyFile = useRef<HTMLInputElement>(null);
  const driveResult = driveServiceAccountDraft(drive);
  const [result, setResult] = useState<DAVConnectionResult>();
  const [rcloneResult, setRcloneResult] = useState<RcloneTestResult>();
  const [error, setError] = useState('');
  const [created, setCreated] = useState<RemoteSource>();
  const nextSourceId = useRef('');
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('once'));
  const [busy, setBusy] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(false);
  // The folders step: chosen folders, the library they join, and what the done step reports.
  const libraries = useMemo(() => new LibraryManagementService({api, scope: {serverId, viewerId}}), [api, serverId, viewerId]);
  useEffect(() => () => libraries.dispose(), [libraries]);
  const librarySnapshot = useSyncExternalStore(libraries.subscribe, libraries.getSnapshot, libraries.getSnapshot);
  const [chosen, setChosen] = useState<readonly RemoteFolder[]>([]);
  const [picking, setPicking] = useState(false);
  const [attachMode, setAttachMode] = useState<AttachMode>('new');
  const [libraryName, setLibraryName] = useState('');
  const [kind, setKind] = useState<LibraryKind>('movie');
  const [existingId, setExistingId] = useState('');
  const [finishBusy, setFinishBusy] = useState(false);
  const [finishError, setFinishError] = useState('');
  const [summary, setSummary] = useState<Readonly<{library?: string; folders: number}>>({folders: 0});
  const [advancedValues, setAdvancedValues] = useState<SourceAdvanced>({scanIntervalSeconds: 21600, watchForChanges: false, scanOnStart: false, fullRecheckDays: 0, cacheBytes: 0, cacheFloorBytes: 0, seekPreviews: true, chapterImages: true, waveforms: true, loudness: true, checksums: true});
  const [advancedTouched, setAdvancedTouched] = useState(false);
  const [leaving, setLeaving] = useState<'' | 'saved' | 'unsaved'>('');
  const [leaveChoice, setLeaveChoice] = useState<'keep' | 'remove'>('keep');
  const [leaveRemove, setLeaveRemove] = useState(false);
  const [leaveError, setLeaveError] = useState('');
  const testAttempt = useRef(0), active = useRef(false), pending = useRef(false), lifetime = useRef(0);
  const [formToken, setFormToken] = useState(0);
  // A saved handler belongs to its rendered form, even if invoked after reopen.
  const attemptGuard = () => () => active.current && lifetime.current === formToken && isCurrent();
  /** A refused owner retires this form before Settings hears about it. */
  const refuse = (error: unknown) => {active.current=false;pending.current=false;lifetime.current++;reportError(error);};
  const reset = () => {
    setStep('connection'); setSourceKind('webdav'); setDraft(emptyDraft); setRclone(emptyRclone); setProgram(undefined); setProviders([]); setConfigView(undefined); setConfigRemote(''); setConfigName(''); setS3(emptyS3); setDrive(emptyDrive);
    setResult(undefined); setRcloneResult(undefined); setError(''); setCreated(undefined);
    setProfile(undefined); setValues(sourceReadPreset('once')); setBusy(false); setLoadingProfile(false);
    setChosen([]); setPicking(false); setAttachMode('new'); setLibraryName(''); setKind('movie'); setExistingId('');
    setFinishBusy(false); setFinishError(''); setSummary({folders: 0}); setLeaving(''); setLeaveRemove(false); setLeaveError(''); setLeaveChoice('keep');
    setAdvancedValues({scanIntervalSeconds: 21600, watchForChanges: false, scanOnStart: false, fullRecheckDays: 0, cacheBytes: 0, cacheFloorBytes: 0, seekPreviews: true, chapterImages: true, waveforms: true, loudness: true, checksums: true});
    setAdvancedTouched(false);
    testAttempt.current++; lifetime.current++; setFormToken(lifetime.current); pending.current = false;
  };
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  const updateRclone = (patch: Partial<RcloneDraft>) => {testAttempt.current++;setRcloneResult(undefined);setError('');setRclone(current => ({...current, ...patch}));};
  useEffect(() => {active.current=open;if(open)reset();else{lifetime.current++;pending.current=false;setDraft(emptyDraft);setRclone(emptyRclone);setResult(undefined);setRcloneResult(undefined);setError('');}}, [open]);
  useEffect(() => () => {active.current=false;lifetime.current++;}, []);
  const test = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    const attempt = ++testAttempt.current, captured = {...draft};
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testConnection({name: captured.name.trim(), root: captured.root.trim(), username: captured.username, password: captured.password, insecureLocal: captured.insecureLocal}, guard);
      if (guard() && attempt === testAttempt.current) setResult(next);}
    catch (value) {if (guard() && attempt === testAttempt.current) setError(remoteErrorText(value));}
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const loadProgram = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    try {
      const [status, names] = await Promise.all([service.rcloneProgram(), service.rcloneProviders()]);
      if (guard()) {setProgram(status);setProviders(names);}
    } catch (value) {
      // The generic note stays for an ordinary failure; a refusal retires the form.
      if (guard() && authFailure(value)) refuse(value);
    }
    finally {if (guard()) pending.current = false;}
  };
  const loadConfig = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    try {
      const [status, view] = await Promise.all([service.rcloneProgram(), service.rcloneConfig()]);
      if (!guard()) return;
      setProgram(status);setConfigView(view);
      setConfigRemote(current => current || (view.remotes.find(remote => remote.supported)?.name ?? ''));
    } catch (value) {
      if (!guard()) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard()) pending.current = false;}
  };
  const chooseSourceKind = (next: SourceKind) => {
    if (next === sourceKind) return;
    testAttempt.current++;setResult(undefined);setRcloneResult(undefined);setError('');setSourceKind(next);
    if (next === 'rclone' && !program) void loadProgram();
    if (next === 'config' && !configView) void loadConfig();
  };
  const driveReason = (reason: 'json'|'oauth'|'type'|'email'|'key'|'location'): MessageId => reason === 'json' ? 'next.remoteSources.driveKeyNotJson'
    : reason === 'oauth' ? 'next.remoteSources.driveKeyOAuth'
      : reason === 'type' ? 'next.remoteSources.driveKeyWrongType'
        : reason === 'email' ? 'next.remoteSources.driveKeyEmail'
          : reason === 'key' ? 'next.remoteSources.driveKeyUnreadable'
            : 'next.remoteSources.driveLocationInvalid';
  const updateDrive = (patch: Partial<DriveKeyFields>) => {testAttempt.current++;setRcloneResult(undefined);setError('');setDrive(current => ({...current, ...patch}));};
  const readKeyFile = async (file: File | undefined) => {
    if (!file) return;
    if (file.size > 64 * 1024) {setError(t('next.remoteSources.driveKeyTooLarge'));return;}
    try {const body = await file.text();updateDrive({keyJson: body});}
    catch {setError(t('next.remoteSources.driveKeyUnreadable'));}
  };
  const copyEmail = (email: string) => {const clipboard = navigator.clipboard;if (!clipboard) return;void clipboard.writeText(email).catch(() => undefined);};
  const testDrive = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    if (!driveResult.ok) {setError(t(driveReason(driveResult.reason)));return;}
    pending.current = true;
    const attempt = ++testAttempt.current, built = driveResult.draft;
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: built.remote, config: built.config}, guard);
      if (guard() && attempt === testAttempt.current) setRcloneResult(next);}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const createDrive = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !rcloneResult || !drive.name.trim() || !driveResult.ok) return;
    pending.current = true;setError('');setBusy(true);
    const built = driveResult.draft;
    // The key leaves the form as the save starts; only the request holds it now.
    setDrive(current => ({...current, keyJson: ''}));
    try {
      const receipt = await service.configureRclone(null, {name: drive.name.trim(), remote: built.remote, config: built.config}, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const updateS3 = (patch: Partial<S3SourceFields>) => {testAttempt.current++;setRcloneResult(undefined);setError('');setS3(current => ({...current, ...patch}));};
  const testS3 = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    const built = s3SourceDraft(s3);
    if (!built) {setError(t('next.remoteSources.s3Invalid'));return;}
    pending.current = true;
    const attempt = ++testAttempt.current;
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: built.remote, config: built.config}, guard);
      if (guard() && attempt === testAttempt.current) setRcloneResult(next);}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const testRclone = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    const attempt = ++testAttempt.current, captured = {...rclone};
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: captured.remote.trim(), config: captured.config}, guard);
      if (guard() && attempt === testAttempt.current) setRcloneResult(next);}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const createRclone = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !rcloneResult) return;
    pending.current = true;
    setError('');setBusy(true);
    const outgoing = {name: rclone.name.trim(), remote: rclone.remote.trim(), config: rclone.config};
    // The configuration leaves the form as the mutation starts; only the request holds it now.
    setRclone(current => ({...current, config: ''}));
    try {
      const receipt = await service.configureRclone(null, outgoing, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const create = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !result) return;
    pending.current = true;
    setError('');setBusy(true);
    try {
      await service.configure(null, davConfiguration(null, draft), undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (state.error || !state.receipt) {if (guard()) setError(state.error || t('next.remoteSources.unconfirmed'));return;}
      const source = state.receipt.source;setCreated(source);nextSourceId.current = source.id;setStep('reading');setLoadingProfile(true);setDraft(current => ({...current, password: ''}));
      try {const next = await service.readProfile(source.id, String(source.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const createS3 = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !rcloneResult || !s3.name.trim()) return;
    const built = s3SourceDraft(s3);
    if (!built) {setError(t('next.remoteSources.s3Invalid'));return;}
    pending.current = true;setError('');setBusy(true);
    // The secret leaves the fields as the save starts; only the request holds it now.
    setS3(current => ({...current, secretKey: ''}));
    try {
      const receipt = await service.configureRclone(null, {name: s3.name.trim(), remote: built.remote, config: built.config}, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const createFromConfig = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !configRemote) return;
    const chosen = configView?.remotes.find(remote => remote.name === configRemote && remote.supported);
    if (!chosen) {setError(t('next.remoteSources.configUnsupportedKind'));return;}
    pending.current = true;setError('');setBusy(true);
    try {
      const receipt = await service.configureRcloneFromConfig({name: configName.trim() || chosen.name, remote: chosen.name}, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? next.values : sourceReadPreset('once'));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const saveReading = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !profile) return;
    pending.current = true;
    setError('');setBusy(true);
    try {
      const next = await service.saveProfile(profile, values, guard);
      if (!guard()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      setProfile(next);
      const savedSource = created ?? service.getSnapshot().sources.find(source => source.id === next.sourceId);
      if (advancedTouched && savedSource) {
        const current = await service.readAdvanced(savedSource.id, guard);
        if (!guard()) return;
        const advancedSaved = await service.saveAdvanced(current, advancedValues, guard);
        if (!guard()) return;
        setAdvancedValues(advancedSaved.values);
      }
      setStep('folders');
      setLibraryName(current => current || (created ?? service.getSnapshot().sources.find(source => source.id === next.sourceId))?.name || '');
      void libraries.loadLibraries();
    } catch (value) {
      if (!guard()) return;
      const saved = created ?? service.getSnapshot().sources.find(source => source.id === nextSourceId.current);
      if (authFailure(value)) { active.current=false; reportError(value); return; }
      if (refreshFailure(value) && saved) {active.current=false;setCreated(saved);setStep('folders');void libraries.loadLibraries();} else setError(remoteErrorText(value));
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  /** Attaches the chosen folders to a new or existing library; the source is
   * already saved, so a failure here leaves the source in place. */
  const finish = async () => {
    if (finishBusy || pending.current) return;
    setFinishBusy(true);setFinishError('');
    try {
      if (!chosen.length) {setSummary({folders: 0});setStep('done');return;}
      if (attachMode === 'new') {
        const name = libraryName.trim() || created?.name || '';
        if (!name) {setFinishError(t('next.remoteSources.attachFailed'));return;}
        await libraries.createLibrary({name, kind, folders: chosen.map(folder => folder.path)});
        if (libraries.getSnapshot().mutation.phase !== 'complete') {setFinishError(t('next.remoteSources.attachFailed'));return;}
        setSummary({library: name, folders: chosen.length});
      } else {
        const target = librarySnapshot.directory.data?.items.find(library => library.id === existingId);
        if (!target) {setFinishError(t('next.remoteSources.existingLibraryMissing'));return;}
        for (const folder of chosen) {
          const config = await readLibraryInventoryConfig(api, {serverId, viewerId}, target.id);
          await saveLibraryFolder(api, target.id, undefined, {name: folder.name, path: folder.path, classification: 'local', followSymlinks: false, intervalSeconds: SIX_HOURS, expectedRevision: config.revision, acceptReplacement: false});
        }
        setSummary({library: target.name, folders: chosen.length});
      }
      setStep('done');
    } catch (value) {
      if (isCurrent()) setFinishError(attachFailure(value));
    } finally {if (isCurrent()) setFinishBusy(false);}
  };
  /** Leaving before the end is a real choice once the source is saved, and the
   * dialog says exactly what is kept and what is removed. */
  const close = (force = false) => {
    if (finishBusy) return;
    if (!force && step !== 'done') {
      const saved = created ?? service.getSnapshot().sources.find(source => source.id === nextSourceId.current);
      if (saved) {setLeaving('saved');setLeaveRemove(false);setLeaveError('');setLeaveChoice('keep');return;}
      if (step !== 'connection') {setLeaving('unsaved');setLeaveError('');return;}
      if (!draft.name.trim() && !draft.root.trim() && !draft.username && !draft.password) {force = true;}
      else {setLeaving('unsaved');setLeaveError('');return;}
    }
    if (busy || pending.current) return;
    active.current=false;pending.current=false;lifetime.current++;setDraft(emptyDraft);setResult(undefined);setError('');setChosen([]);setLeaving('');onClose();
  };
  const removeAndLeave = async () => {
    const saved = created ?? service.getSnapshot().sources.find(source => source.id === nextSourceId.current);
    if (!saved || leaveRemove) return;
    setLeaveRemove(true);setLeaveError('');
    try {await service.remove(saved, isCurrent);const state = service.getSnapshot();if (state.error) {setLeaveError(state.error);return;}close(true);}
    catch (value) {setLeaveError(remoteErrorText(value));}
    finally {setLeaveRemove(false);}
  };
  useEffect(() => {if (attachMode === 'existing' && !existingId && librarySnapshot.directory.data?.items.length) setExistingId(librarySnapshot.directory.data.items[0].id);}, [attachMode, existingId, librarySnapshot.directory.data]);
  const sourceNow = created ?? (step === 'folders' || step === 'done' ? service.getSnapshot().sources.find(source => source.id === nextSourceId.current) : undefined);
  const presetName = sourceReadPresetName(values);
  const stepLabel = step === 'done' ? undefined : t('next.remoteSources.stepOf', {step: step === 'connection' ? 1 : step === 'reading' ? 2 : 3});
  const foldersFooter = <>
    <Button size="compact" variant="glass" disabled={finishBusy} onClick={() => {setFinishError('');setStep('reading');}}>{t('next.remoteSources.back')}</Button>
    <Button size="compact" variant="primary" loading={finishBusy} disabled={finishBusy || (!!chosen.length && attachMode === 'new' && !libraryName.trim())} onClick={() => void finish()}>{t('next.remoteSources.finish')}</Button>
  </>;
  const doneFooter = <>
    <Button size="compact" variant="glass" onClick={() => close()}>{t('next.remoteSources.done')}</Button>
    <Button size="compact" variant="glass" onClick={reset}>{t('next.remoteSources.addAnother')}</Button>
    {sourceNow && <Button size="compact" variant="primary" onClick={() => {active.current=false;onCreated(sourceNow);}}>{t('next.remoteSources.openSource', {name: sourceNow.name})}</Button>}
  </>;
  const canContinue = sourceKind === 'webdav' ? !!result
    : sourceKind === 'rclone' ? !!rcloneResult && !!rclone.name.trim()
      : sourceKind === 's3' ? !!rcloneResult && !!s3.name.trim()
        : sourceKind === 'drive' ? !!rcloneResult && !!drive.name.trim() && driveResult.ok
          : !!configRemote && !!configView?.remotes.some(remote => remote.name === configRemote && remote.supported);
  const footer = step === 'connection'
    ? <><Button size="compact" variant="glass" disabled={busy} onClick={() => close()}>{t('next.remoteSources.cancel')}</Button><Button size="compact" variant="primary" disabled={busy || !canContinue} onClick={() => void (sourceKind === 'webdav' ? create() : sourceKind === 'rclone' ? createRclone() : sourceKind === 's3' ? createS3() : sourceKind === 'drive' ? createDrive() : createFromConfig())}>{t('next.remoteSources.saveAndContinue')}</Button></>
    : step === 'reading'
      ? <><Button size="compact" variant="glass" disabled={busy} onClick={() => setStep('connection')}>{t('next.remoteSources.back')}</Button><Button size="compact" variant="primary" disabled={busy || loadingProfile || !profile} onClick={saveReading}>{t('next.remoteSources.save')}</Button></>
      : step === 'folders' ? foldersFooter : doneFooter;
  return <Dialog open={open} onOpenChange={value => {if (!value) close();}} title={t('next.settingsSpec.server.remote-sources.panels.newSource.newTitle')} width={760}
    description={stepLabel} footer={footer}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {finishError && <p className={styles.error} role="alert">{finishError}</p>}
    {step === 'connection' && <>
      <div className={styles.kindTiles} role="radiogroup" aria-label={t('next.remoteSources.whatToConnect')}>
        {kindOptions.map(option => <button key={option.value} type="button" role="radio" aria-checked={sourceKind === option.value} className={styles.kindTile}
          data-picked={sourceKind === option.value || undefined} disabled={busy} onClick={() => chooseSourceKind(option.value as SourceKind)}>
          <Icon name={option.value === 'webdav' ? 'i-globe' : option.value === 'drive' ? 'i-google' : 'i-storage'} size={22}/>{option.label}</button>)}
      </div>
      {sourceKind === 'webdav' ? <>
        <div className={styles.kind}><Icon name="i-globe" size={22} /><span><b>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label')}</b><small>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.hint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
        <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.0.control.placeholder')} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateDraft({root: event.target.value})}/>
        <TextField label={t('next.remoteSources.username')} value={draft.username} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.1.control.placeholder')} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
        <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.2.control.placeholder')} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
        <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
        </div>
      </> : sourceKind === 'drive' ? <>
        <div className={styles.kind}><Icon name="i-google" size={22} /><span><b>{t('next.remoteSources.connectDrive')}</b><small>{t('next.remoteSources.connectDriveHint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={drive.name} maxLength={200} disabled={busy} onChange={event => updateDrive({name: event.target.value})}/>
        <div className={styles.lab2}>{t('next.remoteSources.driveSignIn')}</div>
        <div className={styles.kindTiles} role="radiogroup" aria-label={t('next.remoteSources.driveSignIn')}>
          <button type="button" role="radio" aria-checked={true} className={styles.kindTile} data-picked><Icon name="i-key" size={22}/>{t('next.remoteSources.driveServiceAccount')}</button>
          <button type="button" role="radio" aria-checked={false} className={styles.kindTile} disabled title={t('next.remoteSources.driveOAuthUnavailable')}><Icon name="i-google" size={22}/>{t('next.remoteSources.driveOAuth')}</button>
        </div>
        <p className={styles.note}>{t('next.remoteSources.driveServiceAccountHint')}</p>
        <p className={styles.note}>{t('next.remoteSources.driveOAuthUnavailable')}</p>
        <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.driveKey')}</span>
          <textarea className={styles.config} aria-label={t('next.remoteSources.driveKey')} value={drive.keyJson} placeholder={'{"type": "service_account", …}'} maxLength={65536}
            disabled={busy} spellCheck={false} rows={7} onChange={event => updateDrive({keyJson: event.target.value})}/>
          <small className={styles.note}>{t('next.remoteSources.driveKeyHelp')}</small>
        </label>
        <div className={styles.testRow}>
          <Button size="compact" variant="glass" disabled={busy} onClick={() => keyFile.current?.click()}>{t('next.remoteSources.driveUpload')}</Button>
          <Button size="compact" variant="text" disabled={busy || !drive.keyJson} onClick={() => updateDrive({keyJson: ''})}>{t('next.remoteSources.driveClear')}</Button>
          <input ref={keyFile} type="file" accept="application/json,.json" hidden aria-label={t('next.remoteSources.driveUpload')}
            onChange={event => {const file = event.target.files?.[0];void readKeyFile(file);event.target.value = '';}}/>
        </div>
        {!drive.keyJson.trim() ? <p className={styles.note}>{t('next.remoteSources.driveKeyEmptyHint')}</p>
          : !driveResult.ok ? <p className={styles.error} role="alert">{t(driveReason(driveResult.reason))}</p>
            : <div className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/><span>{driveResult.draft.email}</span>
                <Button size="xs" variant="text" onClick={() => copyEmail(driveResult.draft.email)}>{t('next.remoteSources.driveCopy')}</Button></div>}
        {driveResult.ok && <p className={styles.note}>{t('next.remoteSources.driveEmailHeading')} {t('next.remoteSources.driveEmailHelp')}</p>}
        <div className={styles.lab2}>{t('next.remoteSources.driveLocation')}</div>
        <div className={styles.segment} role="group">
          <Button size="small" variant={drive.location === 'folder' ? 'primary' : 'glass'} aria-pressed={drive.location === 'folder'} disabled={busy} onClick={() => updateDrive({location: 'folder'})}>{t('next.remoteSources.driveFolder')}</Button>
          <Button size="small" variant={drive.location === 'drive' ? 'primary' : 'glass'} aria-pressed={drive.location === 'drive'} disabled={busy} onClick={() => updateDrive({location: 'drive'})}>{t('next.remoteSources.driveDrive')}</Button>
        </div>
        <TextField label={t(drive.location === 'drive' ? 'next.remoteSources.driveDriveId' : 'next.remoteSources.driveFolderId')}
          description={t(drive.location === 'drive' ? 'next.remoteSources.driveDriveHelp' : 'next.remoteSources.driveFolderHelp')}
          value={drive.locationId} maxLength={200} disabled={busy} autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateDrive({locationId: event.target.value})}/>
        <p className={styles.note}>{t('next.remoteSources.driveCannotSee')}</p>
        <p className={styles.note}>{t('next.remoteSources.driveChecks')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !driveResult.ok} onClick={() => void testDrive()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </> : sourceKind === 's3' ? <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectS3')}</b><small>{t('next.remoteSources.connectS3Hint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={s3.name} maxLength={200} disabled={busy} onChange={event => updateS3({name: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Endpoint')} description={t('next.remoteSources.s3EndpointHelp')} value={s3.endpoint} placeholder={t('next.remoteSources.s3EndpointPlaceholder')} maxLength={2048} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateS3({endpoint: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Region')} description={t('next.remoteSources.s3RegionHelp')} value={s3.region} maxLength={32} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateS3({region: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3AccessKey')} value={s3.accessKey} maxLength={256} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateS3({accessKey: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3SecretKey')} type="password" value={s3.secretKey} maxLength={1024} disabled={busy} onChange={event => updateS3({secretKey: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Bucket')} description={t('next.remoteSources.s3BucketHelp')} value={s3.bucket} maxLength={63} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateS3({bucket: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Folder')} description={t('next.remoteSources.s3FolderHelp')} value={s3.folder} maxLength={1024} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateS3({folder: event.target.value})}/>
        <p className={styles.note}>{t('next.remoteSources.s3Checks')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !s3.endpoint.trim() || !s3.accessKey.trim() || !s3.secretKey || !s3.bucket.trim()} onClick={() => void testS3()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </> : sourceKind === 'config' ? <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectFromConfig')}</b><small>{t('next.remoteSources.connectFromConfigHint')}</small></span></div>
        <ProgramNote program={program}/>
        {!configView ? <p className={styles.note} role="status">{t('next.remoteSources.loading')}</p>
          : !configView.remotes.length ? <p className={styles.note} role="status">{t(configView.path ? 'next.remoteSources.configEmpty' : 'next.remoteSources.configAbsent')}</p>
            : <>
              {configView.path && <p className={styles.note}>{t('next.remoteSources.configPath', {path: configView.path})}</p>}
              <RichSelect aria-label={t('next.remoteSources.configRemotes')} value={configRemote} disabled={busy}
                options={configView.remotes.map(remote => ({value: remote.name, label: t('next.remoteSources.configRemoteLabel', {name: remote.name, kind: remote.kind || t('next.remoteSources.configUnknownKind')}),
                  description: remote.supported ? undefined : t(remote.reason === 'encrypted' ? 'next.remoteSources.configEncrypted' : remote.reason === 'wrapper' ? 'next.remoteSources.configWrapper' : remote.reason === 'duplicate' ? 'next.remoteSources.configDuplicate' : remote.reason === 'ambiguous' ? 'next.remoteSources.configAmbiguous' : 'next.remoteSources.configUnsupportedKind'),
                  disabled: !remote.supported}))}
                onValueChange={value => {setConfigRemote(value);setConfigName(current => current || value);}}/>
              <TextField label={t('next.remoteSources.name')} value={configName} maxLength={200} disabled={busy}
                placeholder={configRemote} onChange={event => setConfigName(event.target.value)}/>
              {configView.truncated && <p className={styles.note} role="status">{t('next.remoteSources.configTruncated')}</p>}
              <p className={styles.note}>{t('next.remoteSources.configChecksOnSave')}</p>
            </>}
      </> : <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectRclone')}</b><small>{t('next.remoteSources.connectRcloneHint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={rclone.name} maxLength={100} disabled={busy} onChange={event => updateRclone({name: event.target.value})}/>
        <TextField label={t('next.remoteSources.remoteName')} description={t('next.remoteSources.remoteNameHelp')} value={rclone.remote} placeholder={t('next.remoteSources.remoteNamePlaceholder')} maxLength={200} disabled={busy}
          autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateRclone({remote: event.target.value})}/>
        <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.configuration')}</span>
          <textarea className={styles.config} aria-label={t('next.remoteSources.configuration')} value={rclone.config} placeholder={t('next.remoteSources.configurationPlaceholder')} maxLength={65536}
            disabled={busy} spellCheck={false} rows={8} onChange={event => updateRclone({config: event.target.value})}/>
          <small className={styles.note}>{t('next.remoteSources.configurationHelp')}</small>
        </label>
        <ProgramNote program={program}/>
        {providers.length > 0 && <p className={styles.note}>{t('next.remoteSources.providers', {providers: providers.join(', ')})}</p>}
        <p className={styles.note}>{t('next.remoteSources.rcloneKindsNote')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !rclone.remote.trim() || !rclone.config.trim()} onClick={() => void testRclone()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </>}
    </>}
    {step === 'reading' && <>
      <p className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.label')}</p>
      <ReadingChoices values={values} origin={result?.origin ?? sourceNow?.origin} rangeSupport={result?.rangeSupport ?? sourceNow?.rangeSupport ?? 'unknown'} disabled={busy} onChange={setValues}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={busy} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>
      <details className={styles.advDisclosure}>
        <summary>{t('next.remoteSources.advanced')}</summary>
        <SourceAdvancedRows values={advancedValues} cacheSupported transportSupported={!!sourceNow?.mountId} cacheApplied cacheError={undefined} cacheUsedBytes={0} observedInterval={0} disabled={busy}
          onChange={next => {setAdvancedTouched(true);setAdvancedValues(next);}}/>
      </details>
    </>}
    {step === 'folders' && <>
      <p className={styles.note}>{t('next.remoteSources.foldersOnlyHint')}</p>
      <div className={styles.lab2}>{t('next.remoteSources.foldersHeader', {name: sourceNow?.name ?? ''})}</div>
      <ul className={styles.pickedList}>
        {chosen.map(folder => <li key={folder.relative} className={styles.pickedRow}>
          <Icon name="i-folder" size={20}/><span className={styles.pickedPath}>{folder.relative ? `/${folder.relative}` : '/'}<small>{t('next.remoteSources.checksEverySixHours')}</small></span>
          <Button size="xs" variant="text" onClick={() => setChosen(chosen.filter(value => value.relative !== folder.relative))} aria-label={t('next.remoteSources.removeFolder', {name: folder.name})}><Icon name="i-x" size={16}/></Button>
        </li>)}
        <li className={styles.pickedRow} data-empty={!chosen.length || undefined}>
          {!chosen.length && <span className={styles.pickedPath}>{t('next.remoteSources.noFoldersChosen')}</span>}
          <Button size="small" variant="glass" icon="i-folder" onClick={() => setPicking(true)}>{t('next.remoteSources.chooseFolders')}</Button>
        </li>
      </ul>
      <div className={styles.lab2}>{t('next.remoteSources.addThemTo')}</div>
      <div className={styles.segment} role="group">
        <Button size="small" variant={attachMode === 'new' ? 'primary' : 'glass'} aria-pressed={attachMode === 'new'} onClick={() => setAttachMode('new')}>{t('next.remoteSources.newLibrary')}</Button>
        <Button size="small" variant={attachMode === 'existing' ? 'primary' : 'glass'} aria-pressed={attachMode === 'existing'} onClick={() => setAttachMode('existing')}>{t('next.remoteSources.existingLibrary')}</Button>
      </div>
      {attachMode === 'new' ? <>
        <TextField label={t('next.remoteSources.libraryName')} value={libraryName} maxLength={100} disabled={finishBusy} onChange={event => setLibraryName(event.currentTarget.value)}/>
        <div className={styles.kindTiles} role="radiogroup" aria-label={t('next.remoteSources.libraryKind')}>
          {libraryKinds.map(value => <button key={value.id} type="button" role="radio" aria-checked={kind === value.id} className={styles.kindTile}
            data-picked={kind === value.id || undefined} onClick={() => setKind(value.id)}><Icon name={value.icon} size={22}/>{t(value.label)}</button>)}
        </div>
        <p className={styles.note}>{t(libraryKinds.find(value => value.id === kind)!.hint)}</p>
      </> : (librarySnapshot.directory.data?.items.length
        ? <RichSelect aria-label={t('next.remoteSources.existingLibrary')} value={existingId} disabled={finishBusy}
          options={librarySnapshot.directory.data.items.map(library => ({value: library.id, label: library.name, description: t(libraryKinds.find(value => value.id === library.kind)?.label ?? libraryKinds[0].label)}))}
          onValueChange={setExistingId}/>
        : <p className={styles.note} role="status">{librarySnapshot.directory.phase === 'loading' ? t('next.remoteSources.loading') : t('next.remoteSources.existingLibraryMissing')}</p>)}
      <div className={styles.switchRow}><span>{t('next.remoteSources.scanWhenFinished')}<small className={styles.note}>{t('next.remoteSources.scanWhenFinishedHelp')}</small></span>
        <span className={styles.staticValue}>{t('next.remoteSources.on')}</span></div>
    </>}
    {step === 'done' && <div className={styles.doneList}>
      <div className={styles.doneHead}><Icon name="i-okc" size={34}/><h3>{t('next.remoteSources.doneTitle', {name: sourceNow?.name ?? ''})}</h3></div>
      <p className={styles.note}>{summary.library ? t('next.remoteSources.doneLede', {name: summary.library}) : t('next.remoteSources.summaryNoLibraryHelp')}</p>
      <ul className={styles.doneRows}>
        <li><b>{t('next.remoteSources.summaryConnected')}</b><small>{t('next.remoteSources.summaryConnectedHelp')}</small></li>
        <li><b>{t('next.remoteSources.summaryLimits', {connections: values.connections, requests: values.requestsPerSecond})}</b>
          <small>{values.bytesPerSecond ? t('next.remoteSources.downloadSpeed') + ': ' + (values.bytesPerSecond / 1_000_000) + ' ' + t('next.remoteSources.megabytesPerSecond') + '. ' : t('next.remoteSources.noLimit') + '. '}{t('next.remoteSources.summaryLimitsHelp', {name: sourceNow?.name ?? ''})}</small></li>
        <li><b>{t('next.remoteSources.summaryTreatment', {preset: presetName ? t(presetLabel(presetName)) : t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.4.label')})}</b>
          <small>{t('next.remoteSources.summaryTreatmentHelp', {name: sourceNow?.name ?? ''})}</small></li>
        {summary.library
          ? <li><b>{t('next.remoteSources.summaryLibrary', {library: summary.library})}</b>
            <small>{t(summary.folders === 1 ? 'next.remoteSources.summaryLibraryOne' : 'next.remoteSources.summaryLibraryMany', {count: summary.folders, source: sourceNow?.name ?? ''})}</small></li>
          : <li><b>{t('next.remoteSources.summaryNoLibrary')}</b><small>{t('next.remoteSources.summaryNoLibraryHelp')}</small></li>}
        {summary.library && <li><b>{t('next.remoteSources.summaryScan')}</b><small>{t('next.remoteSources.summaryScanHelp')}</small></li>}
      </ul>
    </div>}
    {picking && sourceNow && <RemoteFolderPicker service={service} sourceId={sourceNow.id} sourceName={sourceNow.name} open={picking}
      onOpenChange={setPicking} onChoose={folders => setChosen(current => [...current, ...folders.filter(folder => !current.some(value => value.relative === folder.relative))])}/>}
    <Dialog open={leaving !== ''} role="alertdialog" width={520} title={t('next.remoteSources.leaveTitle')}
      onOpenChange={value => {if (!value && !leaveRemove) setLeaving('');}}
      description={leaving === 'saved' ? t('next.remoteSources.leaveSavedBody', {name: sourceNow?.name ?? ''}) : t('next.remoteSources.leaveUnsavedBody')}
      footer={<>
        <Button size="compact" variant="glass" disabled={leaveRemove} onClick={() => setLeaving('')}>{t('next.remoteSources.stayHere')}</Button>
        <Button size="compact" variant="primary" loading={leaveRemove} disabled={leaveRemove}
          onClick={() => {if (leaving === 'saved' && leaveChoice === 'remove') void removeAndLeave(); else close(true);}}>{t('next.remoteSources.leave')}</Button>
      </>}>
      {leaving === 'saved' && <div className={styles.leaveRows} role="radiogroup" aria-label={t('next.remoteSources.leaveTitle')}>
        <button type="button" role="radio" aria-checked={leaveChoice === 'keep'} className={styles.leaveRow}
          data-picked={leaveChoice === 'keep' || undefined} onClick={() => setLeaveChoice('keep')}>
          <span className={styles.choiceRadio} aria-hidden="true"/><span><b>{t('next.remoteSources.keepIt')}</b>
            <small>{t('next.remoteSources.keepItHelp', {name: sourceNow?.name ?? ''})}</small></span>
        </button>
        <button type="button" role="radio" aria-checked={leaveChoice === 'remove'} className={styles.leaveRow}
          data-picked={leaveChoice === 'remove' || undefined} onClick={() => setLeaveChoice('remove')}>
          <span className={styles.choiceRadio} aria-hidden="true"/><span><b>{t('next.remoteSources.removeIt')}</b>
            <small>{t('next.remoteSources.removeItHelp', {name: sourceNow?.name ?? ''})}</small></span>
        </button>
      </div>}
      {leaveError && <p className={styles.error} role="alert">{leaveError}</p>}
    </Dialog>
  </Dialog>;
}


function ManageSourceDialog({service, source, api, serverId, viewerId, openSection, isCurrent, reportError, onClose, onRemoved}: Readonly<{
  service: RemoteSourceService; source: RemoteSource; api: LibraryContentApi; serverId: string; viewerId: string;
  openSection?: (section: string) => void;
  isCurrent: () => boolean; reportError: (error: unknown) => void; onClose: () => void; onRemoved: () => void;
}>) {
  const [draft, setDraft] = useState<WebDavDraft>({name: source.name, root: '', username: '', password: '', insecureLocal: source.insecureLocal});
  const [result, setResult] = useState<DAVConnectionResult>();
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [typed, setTyped] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [status, setStatus] = useState<RemoteSourceStatus>();
  const [changing, setChanging] = useState(false);
  const [advanced, setAdvanced] = useState<{settings?: Awaited<ReturnType<RemoteSourceService['readAdvanced']>>; values: SourceAdvanced}>();
  const [advancedBusy, setAdvancedBusy] = useState(false);
  const [advancedNote, setAdvancedNote] = useState('');
  const [pickingProgram, setPickingProgram] = useState(false);
  const [programNote, setProgramNote] = useState('');
  const [pendingValues, setPendingValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const libraries = useMemo(() => new LibraryManagementService({api, scope: {serverId, viewerId}}), [api, serverId, viewerId]);
  const librarySnapshot = useSyncExternalStore(libraries.subscribe, libraries.getSnapshot, libraries.getSnapshot);
  const [attachTo, setAttachTo] = useState('');
  const [picking, setPicking] = useState(false);
  const [attachBusy, setAttachBusy] = useState(false);
  const [attachNote, setAttachNote] = useState('');
  const testAttempt = useRef(0), active = useRef(true), pending = useRef(false), lifetime = useRef(0);
  const attemptGuard = () => { const captured = lifetime.current; return () => active.current && lifetime.current === captured && isCurrent(); };
  useEffect(() => () => {active.current=false;lifetime.current++;}, []);
  useEffect(() => () => libraries.dispose(), [libraries]);
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {
    // The guard belongs to this rendered dialog, not to a later one.
    const captured = lifetime.current;
    const guard = () => active.current && lifetime.current === captured && isCurrent();
    service.readProfile(source.id, String(source.generation), guard).then(next => {
      if (!guard()) return;
      setProfile(next);
      setValues(next.saved ? next.values : sourceReadPreset('list'));
    }).catch(value => {if (guard()) setError(remoteErrorText(value));});
    service.readStatus(source.id, guard).then(next => {if (guard()) setStatus(next);}).catch(() => {});
    service.readAdvanced(source.id, guard).then(next => {if (guard()) setAdvanced({settings: next, values: next.values});}).catch(() => {});
    void libraries.loadLibraries();
  }, [service, source.id, source.generation, isCurrent, libraries]);
  useEffect(() => {if (!attachTo && status?.libraries.length) setAttachTo(status.libraries[0].libraryId);}, [attachTo, status]);
  useEffect(() => {if (!attachTo && librarySnapshot.directory.data?.items.length) setAttachTo(librarySnapshot.directory.data.items[0].id);}, [attachTo, librarySnapshot.directory.data]);
  const changeProgram = async (choice: Readonly<{useIncluded?: boolean; executable?: string}>) => {
    if (!status?.programRevision) {setProgramNote(t('next.remoteSources.programUnavailable'));return;}
    setProgramNote('');
    try {
      await service.setProgram(source.id, status.programRevision, choice, attemptGuard());
      const next = await service.readStatus(source.id, attemptGuard());
      if (active.current) {setStatus(next);setProgramNote(t('next.remoteSources.programChanged'));}
    } catch (value) {if (active.current) setProgramNote(attachFailure(value));}
  };
  const saveAdvanced = async () => {
    if (!advanced || advancedBusy) return;
    setAdvancedBusy(true);setAdvancedNote('');
    try {
      const saved = advanced.settings ? await service.saveAdvanced(advanced.settings, advanced.values, attemptGuard()) : undefined;
      if (saved && active.current) {setAdvanced({settings: saved, values: saved.values});setAdvancedNote(t('next.remoteSources.saved'));}
    } catch (value) {if (active.current) setAdvancedNote(attachFailure(value));}
    finally {if (active.current) setAdvancedBusy(false);}
  };
  const attach = async (folders: readonly RemoteFolder[]) => {
    if (!attachTo || attachBusy) return;
    setAttachBusy(true); setAttachNote('');
    try {
      for (const folder of folders) {
        const config = await readLibraryInventoryConfig(api, {serverId, viewerId}, attachTo);
        await saveLibraryFolder(api, attachTo, undefined, {name: folder.name, path: folder.path, classification: 'local', followSymlinks: false, intervalSeconds: SIX_HOURS, expectedRevision: config.revision, acceptReplacement: false});
      }
      const next = await service.readStatus(source.id, attemptGuard());
      if (active.current) setStatus(next);
    } catch (value) {if (active.current) setAttachNote(attachFailure(value));}
    finally {if (active.current) setAttachBusy(false);}
  };
  const test = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    const attempt = ++testAttempt.current, captured = {...draft};
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testConnection({name: captured.name.trim(), root: captured.root.trim(), username: captured.username, password: captured.password, insecureLocal: captured.insecureLocal}, guard);
      if (guard() && attempt === testAttempt.current) setResult(next);}
    catch (value) {if (guard() && attempt === testAttempt.current) setError(remoteErrorText(value));}
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const saveConnection = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard()) return;
    pending.current = true;
    setError('');setBusy(true);
    try {
      await service.configure(source.id, davConfiguration(source, draft), source, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (state.error) {setError(state.error);return;}
      done();
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const saveReading = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || !profile) return;
    pending.current = true;
    setError('');setBusy(true);
    try {const next = await service.saveProfile(profile, values, guard);if (!guard()) return;if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}setProfile(next);}
    catch (value) {if (!guard()) return;if (authFailure(value)) {active.current=false;lifetime.current++;reportError(value);return;}if (refreshFailure(value)) done(); else setError(remoteErrorText(value));}
    finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const disconnect = async () => {
    const guard = attemptGuard();
    if (pending.current || !guard() || typed.trim() !== source.name) return;
    const current = service.getSnapshot().sources.find(value => value.id === source.id && value.generation === source.generation);
    if (!current) {setError(remoteErrorText({code: 'revision_mismatch'}));return;}
    pending.current = true;setError('');setBusy(true);
    try {await service.remove(current, guard);const state = service.getSnapshot();if (!guard()) return;if (state.error) {setError(state.error);return;}active.current=false;lifetime.current++;onRemoved();}
    finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const done = () => {active.current=false;pending.current=false;lifetime.current++;setDraft(current => ({...current, password: ''}));setTyped('');onClose();};
  const close = () => {if (busy || pending.current) return;done();};
  return <Dialog open onOpenChange={value => {if (!value) close();}} title={source.name} width={760} footer={<Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.statusBox} aria-label={t('next.remoteSources.status')}>
      <div className={styles.statusHead}><SourceStatus source={source}/>{source.origin && <span className={styles.statusOrigin}>{source.origin}</span>}</div>
      {problemId(source) && <p className={styles.statusNotice}>{t(problemId(source)!)}</p>}
      <dl className={styles.statusRows}>
        <div><dt>{t('next.remoteSources.rightNow')}</dt><dd>{!status ? '—'
          : status.connectionsInUse === 0 ? t('next.remoteSources.nothingReading')
            : status.playbackReads > 0 ? t('next.remoteSources.readingNow', {watching: status.playbackReads, connections: status.connectionsInUse})
              : t('next.remoteSources.connectionsNow', {connections: status.connectionsInUse})}</dd></div>
        <div><dt>{t('next.remoteSources.lastChecked')}</dt><dd>{!status ? '—' : status.lastCheckedAt
          ? t('next.remoteSources.checkedAt', {when: checkedWhen(status.lastCheckedAt)})
          : t('next.remoteSources.neverChecked')}</dd></div>
        <div><dt>{t('next.remoteSources.usedBy')}</dt><dd>{!status ? '—'
          : status.libraries.length
            ? status.libraries.map(use => `${use.name} · ${use.folders}`).join(', ') + (status.librariesTruncated ? ' …' : '')
            : t('next.remoteSources.noLibraryUses')}</dd></div>
        {!!status?.scanIntervalSeconds && <div><dt>{t('next.remoteSources.scanning')}</dt>
          <dd>{t('next.remoteSources.scansEveryHours', {hours: Math.round(status.scanIntervalSeconds / 3600) || 1})}</dd></div>}
      </dl>
    </section>
    <section className={styles.group} id="source-connection"><h3>{t('next.remoteSources.connection')}</h3>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={source.origin ?? t('next.settingsSpec.server.remote-sources.panels.source.tabs.0.groups.0.rows.1.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateDraft({root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
      </div>
      <Button size="compact" variant="primary" disabled={busy || !draft.name.trim() || (!result && !draft.root.trim())} onClick={() => void saveConnection()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>
      <div className={styles.choiceSummary}>
        <span>{t('next.remoteSources.howTreated')}</span>
        <b>{t(presetLabel(sourceReadPresetName(values) ?? 'once'))}</b>
        <Button size="xs" variant="glass" disabled={busy} onClick={() => {setPendingValues(values);setChanging(true);}}>{t('next.remoteSources.change')}</Button>
      </div>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={busy} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>
      <Button size="compact" variant="primary" disabled={busy || !profile} onClick={() => void saveReading()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.advanced')}</h3>
      <p className={styles.note}>{t('next.remoteSources.advancedHelp')}</p>
      {advanced
        ? <><SourceAdvancedRows values={advanced.values} cacheSupported={advanced.settings?.cacheSupported ?? false} transportSupported={advanced.settings?.cacheSupported ?? false}
          cacheApplied={advanced.settings?.cacheApplied ?? true} cacheError={advanced.settings?.cacheError} cacheUsedBytes={advanced.settings?.cacheUsedBytes ?? 0} observedInterval={advanced.settings?.scanIntervalObserved ?? 0}
          disabled={advancedBusy} onChange={values => setAdvanced(current => current && {...current, values})}/>
          <Button size="compact" variant="primary" disabled={advancedBusy} onClick={() => void saveAdvanced()}>{t('next.remoteSources.save')}</Button></>
        : <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
      {advancedNote && <p className={styles.note} role="status">{advancedNote}</p>}
      {status?.program && <div className={styles.programRows}>
        <p className={styles.advTitle}>{t('next.remoteSources.connection')}</p>
        <label className={styles.switchRow}><span>{t('next.remoteSources.customProgram')}<small className={styles.note}>{t('next.remoteSources.customProgramHelp')}</small></span>
          <Switch aria-label={t('next.remoteSources.customProgram')} checked={!status.programIncluded} disabled={!status.programRevision}
            onCheckedChange={value => {if (!value) void changeProgram({useIncluded: true}); else setPickingProgram(true);}}/></label>
        <div className={styles.programRow}><span>{t('next.remoteSources.program')}</span>
          <span className={styles.mono}>{status.program}{status.programVersion ? ` · ${status.programVersion}` : ''}</span>
          <Button size="xs" variant="glass" onClick={() => setPickingProgram(true)}>{t('next.remoteSources.chooseProgram')}</Button></div>
        {status.remote && <div className={styles.programRow}><span>{t('next.remoteSources.remote')}</span>
          <span className={styles.mono}>{status.remote}</span>
          <Button size="xs" variant="glass" onClick={() => document.getElementById('source-connection')?.scrollIntoView({block: 'start'})}>{t('next.remoteSources.change')}</Button></div>}
        {status.backgroundSpeed && <div className={styles.programRow}><span>{t('next.remoteSources.backgroundSpeed')}</span>
          <span>{status.backgroundSpeed}</span>
          <Button size="xs" variant="glass" disabled={!openSection} onClick={() => openSection?.('storage')}>{t('next.remoteSources.changeInSchedule')}</Button></div>}
        {programNote && <p className={styles.note} role="status">{programNote}</p>}
      </div>}
      {pickingProgram && <ProgramPicker open={pickingProgram} api={api} startAt={status?.program ?? ''} onOpenChange={setPickingProgram} onChoose={path => void changeProgram({executable: path})}/>}
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.foldersSection')}</h3>
      <p className={styles.note}>{t('next.remoteSources.foldersNote')}</p>
      {status?.program && <dl className={styles.statusRows}>
        <div><dt>{t('next.remoteSources.program')}</dt><dd>{status.program}{status.programVersion ? ` · ${status.programVersion}` : ''}</dd></div>
        {status.remote && <div><dt>{t('next.remoteSources.remote')}</dt><dd className={styles.mono}>{status.remote}</dd></div>}
        {status.backgroundSpeed && <div><dt>{t('next.remoteSources.backgroundSpeed')}</dt><dd>{status.backgroundSpeed} · {t('next.remoteSources.backgroundSpeedHelp')}</dd></div>}
      </dl>}
      {!!status?.libraries.length && <ul className={styles.usedList}>{status.libraries.map(use => <li key={use.libraryId}>
        <Icon name="i-library" size={18}/><span>{use.name}<small>{t('next.remoteSources.foldersInLibrary', {count: use.folders})}</small></span></li>)}</ul>}
      {librarySnapshot.directory.data?.items.length
        ? <div className={styles.attachRow}>
          <RichSelect aria-label={t('next.remoteSources.attachTo')} value={attachTo} disabled={attachBusy}
            options={librarySnapshot.directory.data.items.map(library => ({value: library.id, label: library.name, description: t(libraryKinds.find(value => value.id === library.kind)?.label ?? libraryKinds[0].label)}))}
            onValueChange={setAttachTo}/>
          <Button size="small" variant="glass" icon="i-folder" disabled={attachBusy || !attachTo} onClick={() => setPicking(true)}>{t('next.remoteSources.chooseFolders')}</Button>
        </div>
        : <p className={styles.note}>{t('next.remoteSources.noLibraryUses')}</p>}
      {attachNote && <p className={styles.error} role="alert">{attachNote}</p>}
    </section>
    <section className={styles.danger}><h3>{t('next.remoteSources.disconnect')}</h3><p>{t('next.remoteSources.disconnectBody')}</p>
      {!confirming ? <Button size="compact" variant="text" onClick={() => setConfirming(true)}>{t('next.remoteSources.disconnect')}</Button> : <>
        <p>{t('next.remoteSources.disconnectConfirm')}</p>
        <TextField label={t('next.remoteSources.name')} value={typed} disabled={busy} onChange={event => setTyped(event.target.value)}/>
        <div className={styles.actions}><Button size="compact" variant="glass" disabled={busy} onClick={() => {setConfirming(false);setTyped('');}}>{t('next.remoteSources.cancel')}</Button>
          <Button size="compact" variant="primary" disabled={busy || typed.trim() !== source.name} onClick={() => void disconnect()}>{t('next.remoteSources.disconnect')}</Button></div>
      </>}
    </section>
    <Dialog open={changing} width={560} title={t('next.remoteSources.changeTitle')} description={t('next.remoteSources.changeBody')}
      onOpenChange={setChanging}
      footer={<><Button size="compact" variant="glass" onClick={() => setChanging(false)}>{t('next.remoteSources.cancel')}</Button>
        <Button size="compact" variant="primary" onClick={() => {setValues(pendingValues);setChanging(false);}}>{t('next.remoteSources.useChoice')}</Button></>}>
      <ReadingChoices values={pendingValues} origin={source.origin} rangeSupport={source.rangeSupport} disabled={false} onChange={setPendingValues}/>
    </Dialog>
    {picking && <RemoteFolderPicker service={service} sourceId={source.id} sourceName={source.name} open={picking}
      onOpenChange={setPicking} onChoose={folders => {setPicking(false);void attach(folders);}}/>}    <section className={styles.danger}><h3>{t('next.remoteSources.disconnect')}</h3><p>{t('next.remoteSources.disconnectBody')}</p>
      {!confirming ? <Button size="compact" variant="text" onClick={() => setConfirming(true)}>{t('next.remoteSources.disconnect')}</Button> : <>
        <p>{t('next.remoteSources.disconnectConfirm')}</p>
        <TextField label={t('next.remoteSources.name')} value={typed} disabled={busy} onChange={event => setTyped(event.target.value)}/>
        <div className={styles.actions}><Button size="compact" variant="glass" disabled={busy} onClick={() => {setConfirming(false);setTyped('');}}>{t('next.remoteSources.cancel')}</Button>
          <Button size="compact" variant="primary" disabled={busy || typed.trim() !== source.name} onClick={() => void disconnect()}>{t('next.remoteSources.disconnect')}</Button></div>
      </>}
    </section>
  </Dialog>;
}

/** An imported source is replaced whole: the server needs its configuration
 * again and never reads the stored one back to the page. */
function ManageRcloneDialog({service, source, isCurrent, reportError, onClose, onRemoved}: Readonly<{
  service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean; reportError: (error: unknown) => void; onClose: () => void; onRemoved: () => void;
}>) {
  const [draft, setDraft] = useState<RcloneDraft>({name: source.name, remote: '', config: ''});
  const [program, setProgram] = useState<RcloneProgram>();
  const [result, setResult] = useState<RcloneTestResult>();
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [typed, setTyped] = useState('');
  const [confirming, setConfirming] = useState(false);
  const testAttempt = useRef(0), pending = useRef(false), retired = useRef(false);
  // A lease binds every handler and read to this exact rendered source and service.
  const leaseSeq = useRef(0);
  const lease = useMemo(() => ({token: ++leaseSeq.current, service, sourceId: source.id, generation: source.generation}), [service, source.id, source.generation]);
  const leaseRef = useRef(lease); leaseRef.current = lease;
  const reportRef = useRef(reportError); reportRef.current = reportError;
  const held = (captured: typeof lease) => () => !retired.current && leaseRef.current === captured && isCurrent();
  const retire = () => {retired.current = true;pending.current = false;};
  const refuse = (error: unknown) => {retire();reportRef.current(error);};
  /** A command may only start against the exact row the owner is looking at. */
  const stillListed = (captured: typeof lease) => service.getSnapshot().sources.some(row => row.id === captured.sourceId && row.generation === captured.generation);
  useEffect(() => () => {retired.current = true;}, []);
  const updateDraft = (patch: Partial<RcloneDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {
    // The guard stays with this lease, not with a later source or service.
    const captured = lease;
    const guard = () => !retired.current && leaseRef.current === captured && isCurrent();
    service.readProfile(source.id, String(source.generation), guard).then(next => {
      if (!guard()) return;
      setProfile(next);
      setValues(next.saved ? next.values : sourceReadPreset('list'));
    }).catch(value => {
      if (!guard()) return;
      if (authFailure(value)) {retired.current=true;pending.current=false;reportRef.current(value);return;}
      setError(remoteErrorText(value));
    });
    service.rcloneProgram().then(next => {if (guard()) setProgram(next);}).catch(value => {
      if (guard() && authFailure(value)) {retired.current=true;pending.current=false;reportRef.current(value);}
    });
  }, [service, source.id, source.generation, isCurrent, lease]);
  const test = async () => {
    const guard = held(lease);
    if (pending.current || !guard()) return;
    pending.current = true;
    const attempt = ++testAttempt.current, captured = {...draft};
    setError('');setResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: captured.remote.trim(), config: captured.config}, guard);
      if (guard() && attempt === testAttempt.current) setResult(next);}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const saveConnection = async () => {
    const guard = held(lease);
    if (pending.current || !guard()) return;
    if (!stillListed(lease)) {setError(remoteErrorText({code: 'revision_mismatch'}));return;}
    pending.current = true;
    setError('');setBusy(true);
    const outgoing = {name: draft.name.trim(), remote: draft.remote.trim(), config: draft.config};
    // The configuration leaves the form as the mutation starts; only the request holds it now.
    setDraft(current => ({...current, config: ''}));
    try {
      const receipt = await service.configureRclone(source.id, outgoing, source, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      done();
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const saveReading = async () => {
    const guard = held(lease);
    if (pending.current || !guard() || !profile) return;
    pending.current = true;
    setError('');setBusy(true);
    try {const next = await service.saveProfile(profile, values, guard);if (!guard()) return;if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}setProfile(next);}
    catch (value) {if (!guard()) return;if (authFailure(value)) {refuse(value);return;}if (refreshFailure(value)) done(); else setError(remoteErrorText(value));}
    finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const disconnect = async () => {
    const guard = held(lease);
    if (pending.current || !guard() || typed.trim() !== source.name) return;
    const current = service.getSnapshot().sources.find(value => value.id === lease.sourceId && value.generation === lease.generation);
    if (!current) {setError(remoteErrorText({code: 'revision_mismatch'}));return;}
    pending.current = true;setError('');setBusy(true);
    try {const receipt = await service.removeRclone(current, guard);if (!guard()) return;const state = service.getSnapshot();
      if (!receipt) {if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}setError(state.error || t('next.remoteSources.unconfirmed'));return;}
      retire();onRemoved();}
    finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const done = () => {retire();setDraft(current => ({...current, config: ''}));setTyped('');onClose();};
  const close = () => {if (busy || pending.current) return;done();};
  return <Dialog open onOpenChange={value => {if (!value) close();}} title={source.name} width={760} footer={<Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.group} id="source-connection"><h3>{t('next.remoteSources.connection')}</h3>
      <p className={styles.note}><SourceStatus source={source}/> {t('next.remoteSources.kindRclone')}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.remoteName')} description={t('next.remoteSources.remoteNameHelp')} value={draft.remote} placeholder={t('next.remoteSources.remoteNamePlaceholder')} maxLength={200} disabled={busy}
        autoCapitalize="off" spellCheck={false} style={{fontFamily: 'var(--font-mono)'}} onChange={event => updateDraft({remote: event.target.value})}/>
      <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.configuration')}</span>
        <textarea className={styles.config} aria-label={t('next.remoteSources.configuration')} value={draft.config} placeholder={t('next.remoteSources.replaceConfiguration')} maxLength={65536}
          disabled={busy} spellCheck={false} rows={8} onChange={event => updateDraft({config: event.target.value})}/>
        <small className={styles.note}>{t('next.remoteSources.configurationHelp')}</small>
      </label>
      <ProgramNote program={program}/>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.remote.trim() || !draft.config.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{result.version ? t('next.remoteSources.testedRclone', {version: result.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
      </div>
      <Button size="compact" variant="primary" disabled={busy || !draft.name.trim() || !draft.remote.trim() || !draft.config.trim()} onClick={() => void saveConnection()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>
      <ReadingChoices values={values} origin={source.origin} rangeSupport={source.rangeSupport} disabled={busy} onChange={setValues}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={busy} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>
      <Button size="compact" variant="primary" disabled={busy || !profile} onClick={() => void saveReading()}>{t('next.remoteSources.save')}</Button>
    </section>

  </Dialog>;
}

/** Reading applies to the exact listed source; no credential or connection mutation. */
function SourceReadingDialog({service, source, isCurrent, reportError, onClose}: Readonly<{
  service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean;
  reportError: (error: unknown) => void; onClose: () => void;
}>) {
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const scope = useMemo(() => ({service, source, isCurrent}), [service, source, isCurrent]);
  const token = useRef<object | null>(scope), flight = useRef(false);
  const state = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const listed = useCallback(() => service.getSnapshot().sources.some(row => row === source), [service, source]);
  const current = useCallback((captured: object | null) => captured !== null && token.current === captured && isCurrent() && listed(), [isCurrent, listed]);
  const close = () => {token.current = null;onClose();};
  useLayoutEffect(() => {
    const captured = scope;token.current = captured;
    return () => {if (token.current === captured) token.current = null;};
  }, [scope]);
  useEffect(() => {
    const captured = scope;
    if (!current(captured)) return;
    void service.readProfile(source.id, String(source.generation), () => current(captured)).then(next => {
      if (!current(captured)) return;
      setProfile(next);setValues(next.values);
    }).catch(value => {
      if (!current(captured)) return;
      if (authFailure(value)) {token.current = null;reportError(value);} else setError(remoteErrorText(value));
    });
  }, [service, source, scope, current, reportError]);
  const save = async () => {
    const captured = scope;
    if (!current(captured) || flight.current || !profile || state.busy || state.ambiguous) return;
    const original = profile, draft = Object.freeze({...values});
    flight.current = true;setBusy(true);setError('');
    try {
      await service.saveProfile(original, draft, () => current(captured));
      if (current(captured)) close();
    } catch (value) {
      if (!current(captured)) return;
      if (authFailure(value)) {token.current = null;reportError(value);}
      else if (refreshFailure(value)) close();
      else setError(remoteErrorText(value));
    } finally {flight.current = false;if (current(captured)) setBusy(false);}
  };
  const disabled = busy || state.busy || state.ambiguous || !profile || !listed();
  return <Dialog open title={source.name} description={t('next.remoteSources.reading')} onOpenChange={open => {if (!open) close();}}
    footer={<><Button size="compact" variant="glass" onClick={close}>{t('action.cancel')}</Button><Button size="compact" variant="primary" disabled={disabled} onClick={() => void save()}>{t('next.remoteSources.save')}</Button></>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {!profile && !error && <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
    <fieldset disabled={disabled} className={styles.readingFields}>
      <ReadingChoices values={values} origin={source.origin} rangeSupport={source.rangeSupport} disabled={disabled} onChange={setValues}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={disabled} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={disabled} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={disabled} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.help')}</small></span><Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.1.label')} checked={values.wholeFileReads} disabled={disabled} onCheckedChange={value => setValues({...values, wholeFileReads: value})}/></label>

    </fieldset>
  </Dialog>;
}
