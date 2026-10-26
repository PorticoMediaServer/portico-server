import {useCallback, useLayoutEffect, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {
  RemoteSourceService, cloudTokenDraft, driveOAuthDraft, driveServiceAccountDraft, s3SourceDraft, sourceReadPreset, sourceReadPresetName,
  type CloudTokenFields, type DAVConnectionResult, type DriveKeyFields, type RcloneConfigView, type RcloneProgram, type RcloneTestResult, type RemoteFolder, type RemoteSource, type SavedSourceTest, type S3SourceFields, type RemoteSourceStatus, type RemoteContactErrorCode, type RemoteScanMode, type RemoteScanPhase, type RemoteSourceHealth, type RemoteScanStateCode, type SourceAdvanced, type SourceReadPreset, type SourceReadSettings, type SourceReadValues,
} from '@core/remote-sources.ts';
import {LibraryManagementService, type ManagedLibrary} from '@core/library-management.ts';
import {readLibraryInventoryConfig, saveLibraryFolder} from '@core/server-admin/libraries.ts';
import {davConfiguration, stableStorageOperationId, type WebDavDraft} from '@core/server-admin/panel-logic.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {Button, RichSelect, Switch, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {usePhone} from '../../../kit/overlays/shared';
import {SettingsGroup, SettingNumber} from '../../../kit/settings';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import {Icon, type IconName} from '../../../kit/Icon';
import styles from './remote-sources.module.css';
import {RemoteFolderPicker} from './RemoteFolderPicker';
import {ReadingChoices, presetHint} from './ReadingChoices';
import {SourceAdvancedRows} from './SourceAdvancedRows';
import {ProgramPicker} from './ProgramPicker';
import {SourceReadingDialog} from './SourceReadingDialog';
import {useOwnedResource} from './resources';
import {AnalysisConsentDraft, ProfileAdmission, type HeldProfile} from './profile-admission';

const t = defaultI18n.t;

/** Read values, including the retired whole-file compatibility wire; the independent analysis consent is its own draft. */
function readValuesWithoutConsent(values: SourceReadValues): SourceReadValues {
  const {allowAdvancedMediaAnalysis, ...read} = values;
  return allowAdvancedMediaAnalysis === undefined ? values : read;
}
/** A save body carries consent only when this server actually published one. */
function readValuesWithConsent(values: SourceReadValues, consent: boolean | undefined, edited: boolean | undefined): SourceReadValues {
  const read = readValuesWithoutConsent(values);
  return !edited || consent === undefined ? read : {...read, allowAdvancedMediaAnalysis: consent};
}
/** One source-specific analysis control; an older server without the fact draws no control,
 * never an off/false checkbox it cannot have recorded. */
function AnalysisConsent({consent, disabled, onChange}: {consent: boolean | undefined; disabled: boolean; onChange: (value: boolean) => void}) {
  if (consent === undefined) return null;
  return <label className={styles.switchRow}>
    <span>{t('next.remoteSources.allowAdvancedMediaAnalysis')}<small className={styles.note}>{t('next.remoteSources.allowAdvancedMediaAnalysisWarning')}</small></span>
    <Switch aria-label={t('next.remoteSources.allowAdvancedMediaAnalysis')} checked={consent} disabled={disabled} onCheckedChange={onChange}/>
  </label>;
}

/** Keep the read profile's captured legacy whole-file compatibility value while a choice or an
 * unsaved default replaces only the drawn read values. Consent is never part of this. */
function withLegacyWholeFileReads(values: SourceReadValues, legacy: boolean): SourceReadValues {
  return values.wholeFileReads === legacy ? values : {...values, wholeFileReads: legacy};
}
/** A new exact source/service/currentness tuple creates a fresh layout resource.
 * No ref is written during render and no retired profile is rearmed. */
function useProfileAdmission({service, source, isCurrent}: {service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean}) {
  const owned = useOwnedResource(() => new ProfileAdmission(source, () => isCurrent() && service.getSnapshot().sources.some(row =>
    row.id === source.id && row.generation === source.generation)), value => value.retire(), [service, source, isCurrent]);
  const entry = owned?.value;
  const [published, publish] = useState<HeldProfile>();
  const held = entry && entry.current() && published === entry.snapshot() ? published : undefined;
  const current = () => !!entry && entry.current();
  const setHeld = (update: HeldProfile | undefined | ((value: HeldProfile | undefined) => HeldProfile | undefined)) => {
    if (!entry || !entry.holds(held)) return;
    const next = typeof update === 'function' ? update(held) : update;
    if (entry.replace(held, next)) publish(entry.snapshot());
  };
  return {admitted: !!entry && entry.current(), token: entry, held, setHeld, current,
    holds: (record: HeldProfile) => !!entry && entry.holds(record), opening: () => !!entry && entry.opening(), retire: () => entry?.retire(),
    begin: (record: HeldProfile) => entry?.begin(record), owns: (flight: object) => !!entry && entry.owns(flight),
    finish: (flight: object) => !!entry && entry.finish(flight),
    settle: (flight: object, expected: HeldProfile, next: HeldProfile) => {
      if (entry?.owns(flight) && entry.replace(expected, next, flight)) publish(entry.snapshot());
    }};
}
const emptyDraft: WebDavDraft = {name: '', root: '', username: '', password: '', insecureLocal: false};
type SourceKind = 'webdav' | 'rclone' | 'config' | 's3' | 'drive' | 'cloud';
type RcloneDraft = {name: string; remote: string; config: string};
type Tested = Readonly<{revision: number; result: RcloneTestResult}>;
const emptyRclone: RcloneDraft = {name: '', remote: '', config: ''};
const emptyS3: S3SourceFields = {endpoint: '', region: '', accessKey: '', secretKey: '', bucket: '', folder: ''};
const emptyDrive: DriveKeyFields = {keyJson: '', location: 'folder', locationId: ''};
const emptyCloud: CloudTokenFields = {provider: 'onedrive', token: '', folder: ''};
/** Step 1 draws three choices: WebDAV, a new rclone remote, or one already in
 * the account's own configuration (remote-sources.html#1, #8a, #14a). */
const whatOptions: readonly {family: SourceFamily; label: MessageId; hint: MessageId; tag?: MessageId}[] = [
  {family: 'webdav', label: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label', hint: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.hint', tag: 'next.remoteSources.kindCommon'},
  {family: 'rclone', label: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.1.label', hint: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.1.hint', tag: 'next.remoteSources.kindIncluded'},
  {family: 'config', label: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.2.label', hint: 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.2.hint', tag: 'next.remoteSources.kindIncluded'},
];
/** Step 2's kinds, grouped by how each is filled in, and only the kinds
 * Portico can build a form for, plus pasting (remote-sources.html#3d). */
const rcloneKindGroups: readonly {title: MessageId; kinds: readonly {value: SourceKind; label: MessageId; icon: IconName}[]}[] = [
  {title: 'next.remoteSources.groupDetails', kinds: [{value: 's3', label: 'next.remoteSources.connectS3', icon: 'i-storage'}]},
  {title: 'next.remoteSources.groupCloud', kinds: [{value: 'drive', label: 'next.remoteSources.connectDrive', icon: 'i-google'}, {value: 'cloud', label: 'next.remoteSources.connectCloud', icon: 'i-storage'}]},
  {title: 'next.remoteSources.groupAnything', kinds: [{value: 'rclone', label: 'next.remoteSources.connectRclone', icon: 'i-storage'}]},
];
const presetLabels: Readonly<Record<SourceReadPreset, MessageId>> = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
};
const presetLabel = (name: SourceReadPreset): MessageId => presetLabels[name];

/** The drawn words for a source's own active scan (remote status scan, run facts). */
const scanStateWord = (state: RemoteScanStateCode): MessageId => state === 'queued' ? 'next.remoteSources.scanQueued'
  : state === 'paused' ? 'next.remoteSources.scanPaused' : 'next.remoteSources.scanRunning';
const scanModeWord = (mode: RemoteScanMode | undefined): MessageId | undefined => mode === 'first-import' ? 'next.remoteSources.scanModeFirst'
  : mode === 'incremental' ? 'next.remoteSources.scanModeIncremental' : mode === 'integrity' ? 'next.remoteSources.scanModeIntegrity'
    : mode === 'full' ? 'next.remoteSources.scanModeFull' : undefined;
/** The drawn words for the all-agree folder health; absent stays absent. */
/** The closed words for a real provider failure; never prose from the server. */
const contactWord = (code: RemoteContactErrorCode): MessageId => code === 'offline' ? 'next.remoteSources.contactOffline'
  : code === 'credentials_required' ? 'next.remoteSources.contactCredentials' : code === 'rate_limited' ? 'next.remoteSources.contactRateLimited'
    : 'next.remoteSources.contactQuota';
const healthWord = (health: RemoteSourceHealth): MessageId => health === 'ready' ? 'next.remoteSources.healthReady'
  : health === 'healthy' ? 'next.remoteSources.healthHealthy' : health === 'degraded' ? 'next.remoteSources.healthDegraded'
  : health === 'inventory_running' ? 'next.remoteSources.healthInventoryRunning' : health === 'cancelled' ? 'next.remoteSources.healthCancelled'
    : health === 'offline' ? 'next.remoteSources.healthOffline' : health === 'root_changed' ? 'next.remoteSources.healthRootChanged'
      : 'next.remoteSources.healthRemoving';
const scanPhaseWord = (phase: RemoteScanPhase | undefined): MessageId | undefined => phase === 'inventory' ? 'next.remoteSources.scanPhaseInventory'
  : phase === 'legacy_inventory' ? 'next.remoteSources.scanPhaseLegacyInventory' : phase === 'sidecars' ? 'next.remoteSources.scanPhaseSidecars'
  : phase === 'basic_pending' ? 'next.remoteSources.scanPhaseBasicPending' : phase === 'basic_running' ? 'next.remoteSources.scanPhaseBasicRunning'
    : phase === 'analysis_enqueue' ? 'next.remoteSources.scanPhaseAnalysis' : phase === 'reconciling' ? 'next.remoteSources.scanPhaseReconciling'
      : phase === 'verifying' ? 'next.remoteSources.scanPhaseVerifying' : phase === 'extras' ? 'next.remoteSources.scanPhaseExtras' : undefined;

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
    case 'remote_administration_unavailable': case 'administration_unavailable': return t('next.remoteSources.problem.administration_unavailable');
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

/** The wizard step's Program row: what the server's own rclone is, drawn from
 * the same published status the source page reads, with the frame's Check again
 * repeating that read without leaving the step (remote-sources 2/2c). */
function ProgramRow({program, disabled, onCheck}: {program?: RcloneProgram; disabled: boolean; onCheck?: () => void}) {
  return <div className={styles.programRow}>
    <span>{t('next.remoteSources.program')}</span>
    {program && <span role="status">{program.available ? (program.version ? t('next.remoteSources.bundledVersion', {version: program.version}) : t('next.remoteSources.bundledProgram')) : t('next.remoteSources.bundledUnavailable')}</span>}
    {onCheck && <Button size="xs" variant="glass" disabled={disabled} onClick={onCheck}>{t('next.remoteSources.checkAgain')}</Button>}
  </div>;
}

export type RemoteSourcesPageProps = Readonly<{
  api: LibraryContentApi; serverId: string; viewerId: string; isCurrent: () => boolean; reportError: (error: unknown) => void;
  openSection?: (section: string) => void;
}>;

/** Server › Remote sources. The drawn page over the source list, the WebDAV add
 * flow and the source's reading profile; only routes the server actually has. */
export function RemoteSourcesPage(props: RemoteSourcesPageProps) {
  // The page's reader belongs to this exact API/scope: it is created in a
  // layout pass and retired by the same pass, so a replay or a scope change
  // gets a fresh reader rather than a disposed one.
  const owned = useOwnedResource(
    () => new RemoteSourceService(props.api, {serverId: props.serverId, viewerId: props.viewerId}, stableStorageOperationId().next, props.isCurrent),
    value => value.dispose(),
    [props.api, props.serverId, props.viewerId, props.isCurrent],
    value => {void value.load();});
  if (!owned) return <div className={styles.page}><p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p></div>;
  // The body belongs to that exact reader: a successor scope starts clean.
  return <RemoteSourcesLoaded key={owned.id} {...props} service={owned.value}/>;
}

function RemoteSourcesLoaded({api, serverId, viewerId, isCurrent, reportError, openSection, service}: RemoteSourcesPageProps & {service: RemoteSourceService}) {
  const [adding, setAdding] = useState(false);
  const [managing, setManaging] = useState<RemoteSource>();
  const [reading, setReading] = useState<RemoteSource>();
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
      {!!sources.length && <p className={styles.sectionLabel}>{t('next.settingsSpec.server.remote-sources.groups.0.rows.0.label')}</p>}
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
    {reading && <SourceReadingDialog key={`${reading.id}:${reading.generation}`} service={service} source={reading} isCurrent={isCurrent} reportError={reportError} onClose={() => setReading(undefined)} errorText={remoteErrorText} isAuthFailure={authFailure}/>}
    {snapshot.loading && sources.length > 0 && <Button size="xs" variant="text" onClick={refresh}>{t('next.remoteSources.refresh')}</Button>}
  </div>;
}

type WizardStep = 'what' | 'remote' | 'reading' | 'folders' | 'done';
type SourceFamily = 'webdav' | 'rclone' | 'config';
type ReadingOpening = Readonly<{guard: () => boolean; sourceId: string; sourceGeneration: number; profileGeneration: string; profileRevision: number}>;
/** The saved test's admission: the exact dialog owner, service, source row and
 *  ready status object it was opened against. It is published through state and
 *  retired as a whole when any of those is replaced. */
type SavedTestLease = Readonly<{owner: object; service: RemoteSourceService; source: RemoteSource; status: RemoteSourceStatus; generation: string}>;
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
/** Google's own console, opened by the owner; Portico never navigates there itself. */
const DRIVE_CONSOLE = 'https://console.cloud.google.com';

type AddSourceProps = Readonly<{
  service: RemoteSourceService; api: LibraryContentApi; serverId: string; viewerId: string; open: boolean;
  isCurrent: () => boolean; reportError: (error: unknown) => void; onClose: () => void; onCreated: (source: RemoteSource) => void;
}>;

/** The wizard's library reader belongs to this opening: created in a layout
 * pass from the page's reader and scope, retired with it, and the body keyed to
 * that exact resource so a replay or replacement starts clean. */
export function AddSourceDialog(props: AddSourceProps) {
  const owned = useOwnedResource(
    () => new LibraryManagementService({api: props.api, scope: {serverId: props.serverId, viewerId: props.viewerId}}),
    value => value.dispose(),
    [props.api, props.serverId, props.viewerId, props.service]);
  if (!owned) return null;
  return <AddSourceLoaded key={owned.id} {...props} libraries={owned.value}/>;
}

function AddSourceLoaded({service, api, serverId, viewerId, open, isCurrent, reportError, onClose, onCreated, libraries}: AddSourceProps & Readonly<{libraries: LibraryManagementService}>) {
  const [step, setStep] = useState<WizardStep>('what');
  const [family, setFamily] = useState<SourceFamily>('webdav');
  const [sourceKind, setSourceKind] = useState<SourceKind>('webdav');
  const [draft, setDraft] = useState<WebDavDraft>(emptyDraft);
  const [rclone, setRclone] = useState<RcloneDraft>(emptyRclone);
  const [program, setProgram] = useState<RcloneProgram>();
  const [providers, setProviders] = useState<readonly string[]>([]);
  const [configView, setConfigView] = useState<RcloneConfigView>();
  const [configRemote, setConfigRemote] = useState('');
  const [configName, setConfigName] = useState('');
  const [s3, setS3] = useState<S3SourceFields>(emptyS3);
  const [s3Name, setS3Name] = useState('');
  const [drive, setDrive] = useState<DriveKeyFields>(emptyDrive);
  const [driveName, setDriveName] = useState('');
  const [driveMode, setDriveMode] = useState<'service'|'oauth'>('service');
  const [oauthClientId, setOauthClientId] = useState('');
  const [oauthClientSecret, setOauthClientSecret] = useState('');
  const [oauthToken, setOauthToken] = useState('');
  const [oauthState, setOauthState] = useState('');
  const [oauthPhase, setOauthPhase] = useState<'idle'|'starting'|'waiting'|'ready'|'expired'|'denied'|'error'>('idle');
  const [oauthAddress, setOauthAddress] = useState('');
  const [oauthError, setOauthError] = useState('');
  const oauthLease = useRef<{state:string}|null>(null);
  const oauthGeneration = useRef(0);
  const [cloud, setCloud] = useState<CloudTokenFields>(emptyCloud);
  const [cloudName, setCloudName] = useState('');
  // A test belongs to the exact rendered tuple; the revision changes with it.
  const [s3Revision, setS3Revision] = useState(0);
  const [driveRevision, setDriveRevision] = useState(0);
  const [s3Tested, setS3Tested] = useState<Tested>();
  const [driveTested, setDriveTested] = useState<Tested>();
  const [cloudRevision, setCloudRevision] = useState(0);
  const [cloudTested, setCloudTested] = useState<Tested>();
  // The refs are the invalidation truth and move before the state is published,
  // so a retained handler cannot use the tuple a finished render still shows.
  const s3RevisionRef = useRef(0), driveRevisionRef = useRef(0), cloudRevisionRef = useRef(0);
  const invalidateS3 = () => {const next = ++s3RevisionRef.current;setS3Revision(next);return next;};
  const invalidateDrive = () => {const next = ++driveRevisionRef.current;setDriveRevision(next);return next;};
  const invalidateCloud = () => {const next = ++cloudRevisionRef.current;setCloudRevision(next);return next;};
  const keyFile = useRef<HTMLInputElement>(null);
  const driveResult = driveServiceAccountDraft(drive);
  // A signed-in OAuth client is tested with its own token, not the key draft:
  // the drawn flow dims Test until the sign-in is done, then lets it run.
  const driveTestReady = driveMode === 'oauth' ? oauthPhase === 'ready' && !!oauthToken && !!drive.locationId.trim() : driveResult.ok;
  const cloudResult = cloudTokenDraft(cloud);
  const [result, setResult] = useState<DAVConnectionResult>();
  const [rcloneResult, setRcloneResult] = useState<RcloneTestResult>();
  const [error, setError] = useState('');
  const [created, setCreated] = useState<RemoteSource>();
  const nextSourceId = useRef('');
  const [profile, setProfile] = useState<SourceReadSettings>();
  const [values, setValues] = useState<SourceReadValues>(sourceReadPreset('once'));
  const [, showConsent] = useState(0);
  // The drawn mobile-web step (remote-sources.html#14b) shows the choice in force
  // and opens the four choices from Change; the wide layout keeps them inline.
  // A confirm belongs to the opening it was rendered in: the wizard's token, its
  // reading step, and the exact source generation and saved profile revision.
  const phone = usePhone();
  const [pendingReading, setPendingReading] = useState<SourceReadValues>(sourceReadPreset('once'));
  // The opening is its own object, never a flag: a handler that belongs to a
  // retired opening cannot confirm, edit or close the one that replaced it.
  const [readingOpening, setReadingOpening] = useState<ReadingOpening | null>(null);
  const readingOpeningRef = useRef<ReadingOpening | null>(null);
  const readingLive = useRef<{step: WizardStep; source?: RemoteSource; profile?: SourceReadSettings}>({step: 'what'});
  // The committed step and form token of this wizard opening.
  const wizardLive = useRef<{step: WizardStep; token: number}>({step: 'what', token: -1});
  const [busy, setBusy] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(false);
  // The folders step: chosen folders, the library they join, and what the done step reports.
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
  const initialAdvanced: SourceAdvanced = {scanIntervalSeconds: 21600, watchForChanges: false, scanOnStart: false, fullRecheckDays: 0, cacheBytes: 0, cacheFloorBytes: 0, seekPreviews: true, chapterImages: true, waveforms: true, loudness: true, checksums: true, introsMode: 'inherit', doubleCheckFolders: false, dirCacheSeconds: 0, readBufferBytes: 0, connectTimeoutSeconds: 0, idleTimeoutSeconds: 0, retries: 0, lowLevelRetries: 0};
  const [advancedValues, setAdvancedValues] = useState<SourceAdvanced>(initialAdvanced);
  const [advancedTouched, setAdvancedTouched] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [leaving, setLeaving] = useState<'' | 'saved' | 'unsaved'>('');
  const [leaveChoice, setLeaveChoice] = useState<'keep' | 'remove'>('keep');
  const [leaveRemove, setLeaveRemove] = useState(false);
  const [leaveError, setLeaveError] = useState('');
  const testAttempt = useRef(0), active = useRef(false), pending = useRef(false), lifetime = useRef(0);
  const [formToken, setFormToken] = useState(0);
  // A saved handler belongs to its rendered form, even if invoked after reopen.
  const attemptGuard = () => () => active.current && lifetime.current === formToken && isCurrent();
  // This render's own guard. A retained handler carries the guard of the
  // render that drew it, never one acquired later.
  const renderGuard = attemptGuard();
  const consentOwned = useOwnedResource(() => profile ? new AnalysisConsentDraft(profile, renderGuard) : undefined,
    value => value?.retire(), [service, profile, isCurrent, formToken]);
  const consentDraft = consentOwned?.value;

  /** A refused owner retires this form before Settings hears about it. */
  const refuse = (error: unknown) => {active.current=false;pending.current=false;lifetime.current++;reportError(error);};
  const reset = () => {
    setStep('what'); setFamily('webdav'); setSourceKind('webdav'); setDraft(emptyDraft); setRclone(emptyRclone); setProgram(undefined); setProviders([]); setConfigView(undefined); setConfigRemote(''); setConfigName(''); setS3(emptyS3); setDrive(emptyDrive); setS3Name(''); setDriveName(''); setS3Tested(undefined); setDriveTested(undefined); setCloud(emptyCloud); setCloudName(''); setCloudTested(undefined); setDriveMode('service'); setOauthClientId(''); setOauthClientSecret(''); setOauthToken(''); setOauthState(''); setOauthPhase('idle'); setOauthAddress(''); setOauthError(''); oauthLease.current = null; oauthGeneration.current++; invalidateS3(); invalidateDrive(); invalidateCloud();
    setResult(undefined); setRcloneResult(undefined); setError(''); setCreated(undefined);
    setProfile(undefined); setValues(sourceReadPreset('once')); setBusy(false); setLoadingProfile(false);
    setReadingOpening(null); setPendingReading(sourceReadPreset('once')); readingOpeningRef.current = null;
    setChosen([]); setPicking(false); setAttachMode('new'); setLibraryName(''); setKind('movie'); setExistingId('');
    setFinishBusy(false); setFinishError(''); setSummary({folders: 0}); setLeaving(''); setLeaveRemove(false); setLeaveError(''); setLeaveChoice('keep');
    setAdvancedValues(initialAdvanced);
    setAdvancedTouched(false); setAdvancedOpen(false);
    testAttempt.current++; lifetime.current++; setFormToken(lifetime.current); pending.current = false;
  };
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  const updateRclone = (patch: Partial<RcloneDraft>) => {testAttempt.current++;setRcloneResult(undefined);setError('');setRclone(current => ({...current, ...patch}));};
  useEffect(() => {active.current=open;if(open)reset();else{retireOAuthCurrentRef.current();lifetime.current++;pending.current=false;readingOpeningRef.current=null;setReadingOpening(null);setDraft(emptyDraft);setRclone(emptyRclone);setResult(undefined);setRcloneResult(undefined);setError('');}}, [open]);
  useEffect(() => () => {active.current=false;lifetime.current++;retireOAuthCurrentRef.current();}, []);
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
    testAttempt.current++;retireOAuth(oauthRenderTicket);setResult(undefined);setRcloneResult(undefined);setS3Tested(undefined);setDriveTested(undefined);setCloudTested(undefined);invalidateS3();invalidateDrive();invalidateCloud();setError('');setSourceKind(next);
    if (next === 'rclone' && !program) void loadProgram();
    if (next === 'config' && !configView) void loadConfig();
  };
  /** A step move belongs to the render that drew the control, in the opening it
   * was drawn for: a retained Continue cannot move a successor wizard. */
  const wizardAction = (guard: () => boolean, expected: WizardStep, token: number) =>
    guard() && wizardLive.current.step === expected && wizardLive.current.token === token;
  const continueFromWhat = (guard: () => boolean, token: number) => {
    if (!wizardAction(guard, 'what', token)) return;
    setStep('remote');
  };
  const goBack = (from: WizardStep, to: WizardStep, guard: () => boolean, token: number) => {
    if (!wizardAction(guard, from, token)) return;
    setStep(to);
  };
  const chooseFamily = (next: SourceFamily, guard: () => boolean, token: number) => {
    if (!wizardAction(guard, 'what', token) || next === family) return;
    setFamily(next);
    chooseSourceKind(next === 'webdav' ? 'webdav' : next === 'config' ? 'config' : 's3');
  };
  const chooseRemoteKind = (next: SourceKind, guard: () => boolean, token: number) => {
    if (!wizardAction(guard, 'remote', token)) return;
    chooseSourceKind(next);
  };
  const driveReason = (reason: 'json'|'oauth'|'type'|'email'|'key'|'location'|'client'|'token'): MessageId => reason === 'client' || reason === 'token' ? 'next.remoteSources.driveOAuthInvalid'
    : reason === 'json' ? 'next.remoteSources.driveKeyNotJson'
    : reason === 'oauth' ? 'next.remoteSources.driveKeyOAuth'
      : reason === 'type' ? 'next.remoteSources.driveKeyWrongType'
        : reason === 'email' ? 'next.remoteSources.driveKeyEmail'
          : reason === 'key' ? 'next.remoteSources.driveKeyUnreadable'
            : 'next.remoteSources.driveLocationInvalid';
  // One rendered sign-in ticket owns Start/Paste/Status/Cancel and every phase.
  const oauthTicket = useRef<object | null>(null);
  const oauthPollPromise = useRef<Promise<void> | null>(null);
  const oauthApproved = useRef<{clientId: string; clientSecret: string} | null>(null);
  const oauthClientRef = useRef({id: '', secret: ''});
  const oauthRenderTicket = oauthTicket.current;
  const oauthRenderLease = oauthLease.current;
  const oauthRenderClient = {id: oauthClientId, secret: oauthClientSecret};
  oauthClientRef.current = oauthRenderClient;
  const retireOAuth = (ticket: object | null): boolean => {
    // Only the exact opening may retire itself; it cannot retire a successor.
    if (ticket !== oauthTicket.current) return false;
    oauthGeneration.current++;
    oauthTicket.current = null;
    const lease = oauthLease.current;
    oauthLease.current = null;
    oauthApproved.current = null;
    if (ticket && lease) void service.driveOAuthCancel(lease.state);
    setOauthState('');setOauthToken('');setOauthClientSecret('');setOauthPhase('idle');setOauthAddress('');setOauthError('');
    return true;
  };
  /** Every OAuth transport carries this opening's exact ticket, lease and draft revision. */
  const oauthGuardFor = (ticket: object, revision: number, lease?: {state: string}) => () => {
    const guard = attemptGuard();
    return guard() && oauthTicket.current === ticket && driveRevisionRef.current === revision && (!lease || oauthLease.current === lease);
  };
  const retireOAuthCurrentRef = useRef<() => void>(() => {});
  retireOAuthCurrentRef.current = () => retireOAuth(oauthTicket.current);
  const updateOAuthClient = (patch: {clientId?: string; clientSecret?: string}) => {
    // A new client is a new authorization: cancel the old lease at once.
    retireOAuth(oauthRenderTicket);
    invalidateDrive();setDriveTested(undefined);setRcloneResult(undefined);setError('');
    if (patch.clientId !== undefined) setOauthClientId(patch.clientId);
    if (patch.clientSecret !== undefined) setOauthClientSecret(patch.clientSecret);
  };
  const updatePasteAddress = (value: string) => {setOauthAddress(value);};
  const pollOAuth = async (state: string, ticket: object, guard: () => boolean, revision: number, generation: number) => {
    for (let attempt = 0; attempt < 150; attempt++) {
      await new Promise(resolve => setTimeout(resolve, 2000));
      if (!guard() || oauthTicket.current !== ticket || oauthGeneration.current !== generation || driveRevisionRef.current !== revision) return;
      try {
        const status = await service.driveOAuthStatus(state, oauthGuardFor(ticket, revision, oauthLease.current ?? undefined));
        if (!guard() || oauthTicket.current !== ticket || oauthGeneration.current !== generation || driveRevisionRef.current !== revision) return;
        if (status.phase === 'pending') continue;
        if (status.phase === 'ready') {setOauthToken(status.token);setDriveTested(undefined);setOauthPhase('ready');return;}
        setOauthPhase(status.phase === 'denied' ? 'denied' : 'expired');return;
      } catch { if (guard() && oauthTicket.current === ticket) setOauthPhase('error'); return; }
    }
    if (oauthTicket.current === ticket) setOauthPhase('expired');
  };
  const startOAuth = async () => {
    const guard = attemptGuard();
    const frozen = oauthRenderClient, revision = driveRevision;
    if (pending.current || !guard()) return;
    if (oauthTicket.current !== oauthRenderTicket) return;
    if (oauthClientRef.current.id !== frozen.id || oauthClientRef.current.secret !== frozen.secret || revision !== driveRevisionRef.current) return;
    if (!frozen.id.trim() || !frozen.secret) {setOauthError(t('next.remoteSources.driveOAuthInvalid'));return;}
    if (!retireOAuth(oauthRenderTicket)) return;
    const ticket = {};oauthTicket.current = ticket;
    pending.current = true;setOauthError('');setOauthPhase('starting');
    try {
      const startedGuard = oauthGuardFor(ticket, revision);
      const started = await service.startDriveOAuth(frozen.id.trim(), frozen.secret, startedGuard);
      if (!startedGuard()) {void service.driveOAuthCancel(started.state);return;}
      // The opening's client tuple stays private for its Ready/Test/Save; the visible secret field stays empty.
      oauthLease.current = {state: started.state};oauthApproved.current = {clientId: frozen.id, clientSecret: frozen.secret};
      setOauthState(started.state);setOauthPhase('waiting');
      window.open(started.authorizationUrl, '_blank', 'noopener,noreferrer');
      // One status owner: await the retiring flight's settlement, then start this ticket's.
      const prior = oauthPollPromise.current;
      const run = (async () => {
        if (prior) await prior;
        if (!guard() || oauthTicket.current !== ticket) return;
        await pollOAuth(started.state, ticket, startedGuard, revision, ++oauthGeneration.current);
      })();
      oauthPollPromise.current = run;
      void run.finally(() => {if (oauthPollPromise.current === run) oauthPollPromise.current = null;});
    } catch (value) {
      if (!guard() || oauthTicket.current !== ticket) return;
      if (authFailure(value)) {refuse(value);return;}
      setOauthPhase('error');setOauthError(t('next.remoteSources.driveOAuthInvalid'));
    } finally {if (guard()) pending.current = false;}
  };
  const pasteOAuth = async () => {
    const guard = attemptGuard();
    const ticket = oauthRenderTicket, lease = oauthRenderLease;
    if (pending.current || !guard() || !ticket || !lease || !oauthAddress.trim()) return;
    if (oauthTicket.current !== ticket || oauthLease.current !== lease) return;
    const pasteGuard = oauthGuardFor(ticket, driveRevision, lease);
    pending.current = true;setOauthError('');
    try {
      await service.driveOAuthPaste(lease.state, oauthAddress, pasteGuard);
      if (!guard() || oauthTicket.current !== ticket || oauthLease.current !== lease) return;
      // The pasted address is spent; the running poll is the only status owner.
      setOauthAddress('');
    } catch (value) {
      if (!guard() || oauthTicket.current !== ticket) return;
      if (authFailure(value)) {refuse(value);return;}
      setOauthError(t('next.remoteSources.driveOAuthInvalid'));
    } finally {if (guard()) pending.current = false;}
  };
  const updateDrive = (patch: Partial<DriveKeyFields>) => {testAttempt.current++;setRcloneResult(undefined);setDriveTested(undefined);invalidateDrive();setError('');setDrive(current => ({...current, ...patch}));};
  const readKeyFile = async (file: File | undefined) => {
    const guard = attemptGuard();
    const opening = driveRevisionRef.current;
    if (!file || !guard()) return;
    if (file.size > 64 * 1024) {setError(t('next.remoteSources.driveKeyTooLarge'));return;}
    let body = '';
    try {body = await file.text();}
    catch {if (guard() && opening === driveRevisionRef.current) setError(t('next.remoteSources.driveKeyUnreadable'));return;}
    // The key applies only to the form that opened the file and did not change since.
    if (!guard() || opening !== driveRevisionRef.current) return;
    updateDrive({keyJson: body});
  };
  const copyEmail = (email: string) => {const clipboard = navigator.clipboard;if (!clipboard) return;void clipboard.writeText(email).catch(() => undefined);};
  const testDrive = async () => {
    const guard = attemptGuard();
    const revision = driveRevision, fields = {...drive};
    if (pending.current || !guard() || revision !== driveRevisionRef.current) return;
    const result = driveMode === 'oauth'
      ? driveOAuthDraft({clientId: oauthApproved.current?.clientId ?? '', clientSecret: oauthApproved.current?.clientSecret ?? '', token: oauthToken, location: fields.location, locationId: fields.locationId})
      : driveServiceAccountDraft(fields);
    if (!result.ok) {setError(t(driveMode === 'oauth' ? 'next.remoteSources.driveOAuthInvalid' : driveReason(result.reason)));return;}
    pending.current = true;
    const attempt = ++testAttempt.current, built = result.draft;
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: built.remote, config: built.config}, guard);
      if (guard() && attempt === testAttempt.current && revision === driveRevisionRef.current) {setRcloneResult(next);setDriveTested({revision, result: next});}}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const createDrive = async () => {
    const guard = attemptGuard();
    const revision = driveRevision, fields = {...drive}, name = driveName.trim(), tested = driveTested;
    if (pending.current || !guard() || !tested || tested.revision !== revision || revision !== driveRevisionRef.current || !name) return;
    const result = driveMode === 'oauth'
      ? driveOAuthDraft({clientId: oauthApproved.current?.clientId ?? '', clientSecret: oauthApproved.current?.clientSecret ?? '', token: oauthToken, location: fields.location, locationId: fields.locationId})
      : driveServiceAccountDraft(fields);
    if (!result.ok) {setError(t(driveMode === 'oauth' ? 'next.remoteSources.driveOAuthInvalid' : driveReason(result.reason)));return;}
    pending.current = true;setError('');setBusy(true);
    const built = result.draft;
    // The submitted tuple is captured; the key leaves the form and this witness is spent.
    setDrive(current => ({...current, keyJson: ''}));setDriveTested(undefined);invalidateDrive();
    if (driveMode === 'oauth') {setOauthClientSecret('');setOauthToken('');retireOAuth(oauthRenderTicket);}
    try {
      const receipt = await service.configureRclone(null, {name, remote: built.remote, config: built.config}, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const cloudReason = (reason: 'json'|'token'|'client'|'google'|'folder'): MessageId => reason === 'json' ? 'next.remoteSources.cloudTokenNotJson'
    : reason === 'token' ? 'next.remoteSources.cloudTokenMissing'
      : reason === 'client' ? 'next.remoteSources.cloudTokenClient'
        : reason === 'google' ? 'next.remoteSources.cloudTokenGoogle'
          : 'next.remoteSources.cloudFolderInvalid';
  const updateCloud = (patch: Partial<CloudTokenFields>) => {testAttempt.current++;setRcloneResult(undefined);setCloudTested(undefined);invalidateCloud();setError('');setCloud(current => ({...current, ...patch}));};
  const testCloud = async () => {
    const guard = attemptGuard();
    const revision = cloudRevision, fields = {...cloud};
    if (pending.current || !guard() || revision !== cloudRevisionRef.current) return;
    const result = cloudTokenDraft(fields);
    if (!result.ok) {setError(t(cloudReason(result.reason)));return;}
    pending.current = true;
    const attempt = ++testAttempt.current, built = result.draft;
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: built.remote, config: built.config}, guard);
      if (guard() && attempt === testAttempt.current && revision === cloudRevisionRef.current) {setRcloneResult(next);setCloudTested({revision, result: next});}}
    catch (value) {
      if (!guard() || attempt !== testAttempt.current) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    }
    finally {if (guard() && attempt === testAttempt.current) {pending.current=false;setBusy(false);}}
  };
  const createCloud = async () => {
    const guard = attemptGuard();
    const revision = cloudRevision, fields = {...cloud}, name = cloudName.trim(), tested = cloudTested;
    if (pending.current || !guard() || !tested || tested.revision !== revision || revision !== cloudRevisionRef.current || !name) return;
    const result = cloudTokenDraft(fields);
    if (!result.ok) {setError(t(cloudReason(result.reason)));return;}
    pending.current = true;setError('');setBusy(true);
    const built = result.draft;
    // The submitted tuple is captured; the token leaves the form and this witness is spent.
    setCloud(current => ({...current, token: ''}));setCloudTested(undefined);invalidateCloud();
    try {
      const receipt = await service.configureRclone(null, {name, remote: built.remote, config: built.config}, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const updateS3 = (patch: Partial<S3SourceFields>) => {testAttempt.current++;setRcloneResult(undefined);setS3Tested(undefined);invalidateS3();setError('');setS3(current => ({...current, ...patch}));};
  const testS3 = async () => {
    const guard = attemptGuard();
    const revision = s3Revision, fields = {...s3};
    if (pending.current || !guard() || revision !== s3RevisionRef.current) return;
    const built = s3SourceDraft(fields);
    if (!built) {setError(t('next.remoteSources.s3Invalid'));return;}
    pending.current = true;
    const attempt = ++testAttempt.current;
    setError('');setRcloneResult(undefined);setBusy(true);
    try {const next = await service.testRclone({remote: built.remote, config: built.config}, guard);
      if (guard() && attempt === testAttempt.current && revision === s3RevisionRef.current) {setRcloneResult(next);setS3Tested({revision, result: next});}}
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
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
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
      try {const next = await service.readProfile(source.id, String(source.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const createS3 = async () => {
    const guard = attemptGuard();
    const revision = s3Revision, fields = {...s3}, name = s3Name.trim(), tested = s3Tested;
    if (pending.current || !guard() || !tested || tested.revision !== revision || revision !== s3RevisionRef.current || !name) return;
    const built = s3SourceDraft(fields);
    if (!built) {setError(t('next.remoteSources.s3Invalid'));return;}
    pending.current = true;setError('');setBusy(true);
    // The submitted tuple is captured; the secret leaves the form and this witness is spent.
    setS3(current => ({...current, secretKey: ''}));setS3Tested(undefined);invalidateS3();
    try {
      const receipt = await service.configureRclone(null, {name, remote: built.remote, config: built.config}, undefined, guard);
      if (!guard()) return;
      const state = service.getSnapshot();
      if (!receipt) {
        if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}
        setError(state.error || t('next.remoteSources.unconfirmed'));return;
      }
      nextSourceId.current = receipt.sourceId;setStep('reading');setLoadingProfile(true);
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
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
      try {const next = await service.readProfile(receipt.sourceId, String(receipt.generation), guard);if (guard()) {setProfile(next);setValues(next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('once'), next.values.wholeFileReads));}}
      catch (value) {if (guard()) setError(remoteErrorText(value));}
      finally {if (guard()) setLoadingProfile(false);}
    } finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const saveReading = async () => {
    const original = attemptGuard();
    const guard = () => original() && !!consentDraft?.current();
    if (pending.current || !guard() || !profile) return;
    pending.current = true;
    setError('');setBusy(true);
    let confirmed: SourceReadSettings | undefined;
    try {
      const next = await service.saveProfile(profile, consentDraft!.values(values), guard);
      if (!guard()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      confirmed = next;
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
    } finally {if (guard()) {if (confirmed) setProfile(confirmed);pending.current=false;setBusy(false);}}
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
      if (step === 'what') force = true;
      else if (step !== 'remote') {setLeaving('unsaved');setLeaveError('');return;}
      else if (!draft.name.trim() && !draft.root.trim() && !draft.username && !draft.password) force = true;
      else {setLeaving('unsaved');setLeaveError('');return;}
    }
    if (busy || pending.current) return;
    retireOAuthCurrentRef.current();
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
  // The drawn wizard has five steps: what, the remote, how Portico treats it,
  // folders, done (remote-sources.html#1, #8 to #12).
  const stepNumber = step === 'what' ? 1 : step === 'remote' ? 2 : step === 'reading' ? 3 : step === 'folders' ? 4 : 5;
  const stepLabel = step === 'done' ? undefined : t('next.remoteSources.stepOf', {step: stepNumber});
  const familyLabel = family === 'webdav' ? 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label'
    : family === 'rclone' ? 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.1.label'
      : 'next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.2.label';
  const stepTitle = step === 'what' ? t('next.remoteSources.whatToConnect') : step === 'remote' ? t(familyLabel)
    : step === 'reading' ? t('next.remoteSources.howTreated') : step === 'folders' ? t('next.remoteSources.foldersSection') : '';
  const foldersFooter = <>
    <Button size="compact" variant="glass" disabled={finishBusy} onClick={() => {setFinishError('');setStep('reading');}}>{t('next.remoteSources.back')}</Button>
    <Button size="compact" variant="primary" loading={finishBusy} disabled={finishBusy || (!!chosen.length && attachMode === 'new' && !libraryName.trim())} onClick={() => void finish()}>{t('next.remoteSources.finish')}</Button>
  </>;
  const doneFooter = <>
    <Button size="compact" variant="glass" onClick={() => close(true)}>{t('next.remoteSources.done')}</Button>
    <Button size="compact" variant="glass" onClick={reset}>{t('next.remoteSources.addAnother')}</Button>
    {sourceNow && <Button size="compact" variant="primary" onClick={() => {active.current=false;onCreated(sourceNow);}}>{t('next.remoteSources.openSource', {name: sourceNow.name})}</Button>}
  </>;
  const canContinue = sourceKind === 'webdav' ? !!result
    : sourceKind === 'rclone' ? !!rcloneResult && !!rclone.name.trim()
      : sourceKind === 's3' ? !!s3Tested && s3Tested.revision === s3Revision && !!s3Name.trim()
        : sourceKind === 'drive' ? !!driveTested && driveTested.revision === driveRevision && !!driveName.trim() && (driveMode === 'oauth' ? oauthPhase === 'ready' && !!oauthToken : driveResult.ok)
          : sourceKind === 'cloud' ? !!cloudTested && cloudTested.revision === cloudRevision && !!cloudName.trim() && cloudResult.ok
          : !!configRemote && !!configView?.remotes.some(remote => remote.name === configRemote && remote.supported);
  useLayoutEffect(() => {wizardLive.current = {step, token: formToken}; readingOpeningRef.current = readingOpening; readingLive.current = {step, source: sourceNow, profile};});
  // The dialog cannot outlive a phone layout: a wider window retires its opening.
  useLayoutEffect(() => {if (phone || !readingOpening) return; readingOpeningRef.current = null; setReadingOpening(null);}, [phone, readingOpening]);
  // Retire the opening as soon as the wizard closes, and on unmount, in a layout
  // pass: a retained choice edit must not write in the passive-cleanup gap.
  useLayoutEffect(() => {if (open) return; readingOpeningRef.current = null; setReadingOpening(null);}, [open]);
  useLayoutEffect(() => () => {readingOpeningRef.current = null;}, []);
  /** Retires the opening only when it is still the one this handler belongs to. */
  const retireReading = (opening: ReadingOpening) => {
    if (readingOpeningRef.current !== opening) return;
    readingOpeningRef.current = null;
    setReadingOpening(null);
  };
  const openReadingChange = () => {
    if (!profile || !sourceNow || readingOpeningRef.current) return;
    const opening: ReadingOpening = {guard: attemptGuard(), sourceId: sourceNow.id, sourceGeneration: sourceNow.generation, profileGeneration: profile.generation, profileRevision: profile.revision};
    readingOpeningRef.current = opening;
    setPendingReading(values);
    setReadingOpening(opening);
  };
  /** Only the exact opening this handler was rendered with may replace the choice. */
  const useReadingOpening = (opening: ReadingOpening) => {
    if (readingOpeningRef.current !== opening) return;
    const live = readingLive.current;
    readingOpeningRef.current = null;
    setReadingOpening(null);
    if (!opening.guard() || live.step !== 'reading') return;
    if (!live.source || !live.profile || live.source.id !== opening.sourceId || live.source.generation !== opening.sourceGeneration) return;
    if (live.profile.generation !== opening.profileGeneration || live.profile.revision !== opening.profileRevision) return;
    setValues(withLegacyWholeFileReads(pendingReading, values.wholeFileReads));
  };
  const readingChangeDialog = phone && readingOpening ? <Dialog open width={560} title={t('next.remoteSources.changeTitle')} description={t('next.remoteSources.changeBody')}
    onOpenChange={value => {if (!value) retireReading(readingOpening);}}
    footer={<><Button size="compact" variant="glass" onClick={() => retireReading(readingOpening)}>{t('next.remoteSources.cancel')}</Button>
      <Button size="compact" variant="primary" onClick={() => useReadingOpening(readingOpening)}>{t('next.remoteSources.useChoice')}</Button></>}>
    <ReadingChoices values={pendingReading} origin={result?.origin ?? sourceNow?.origin} rangeSupport={result?.rangeSupport ?? sourceNow?.rangeSupport ?? 'unknown'} disabled={false}
      onChange={next => {if (readingOpeningRef.current === readingOpening) setPendingReading(next);}}/>
  </Dialog> : null;
  const footer = step === 'what'
    ? <><Button size="compact" variant="glass" onClick={() => close()}>{t('next.remoteSources.cancel')}</Button><Button size="compact" variant="primary" onClick={() => continueFromWhat(renderGuard, formToken)}>{t('next.remoteSources.continue')}</Button></>
    : step === 'remote'
      ? <><Button size="compact" variant="glass" disabled={busy} onClick={() => goBack('remote', 'what', renderGuard, formToken)}>{t('next.remoteSources.back')}</Button><Button size="compact" variant="primary" disabled={busy || !canContinue} onClick={() => void (sourceKind === 'webdav' ? create() : sourceKind === 'rclone' ? createRclone() : sourceKind === 's3' ? createS3() : sourceKind === 'drive' ? createDrive() : sourceKind === 'cloud' ? createCloud() : createFromConfig())}>{t('next.remoteSources.saveAndContinue')}</Button></>
      : step === 'reading'
        ? <><Button size="compact" variant="glass" disabled={busy} onClick={() => goBack('reading', 'remote', renderGuard, formToken)}>{t('next.remoteSources.back')}</Button><Button size="compact" variant="primary" disabled={busy || loadingProfile || !profile} onClick={saveReading}>{t('next.remoteSources.save')}</Button></>
        : step === 'folders' ? foldersFooter : doneFooter;
  return <Dialog open={open} onOpenChange={value => {if (!value) close();}} title={t('next.settingsSpec.server.remote-sources.panels.newSource.newTitle')} width={760}
    description={step === 'done' ? undefined : stepTitle} footer={footer}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    {finishError && <p className={styles.error} role="alert">{finishError}</p>}
    {step !== 'done' && <div className={styles.wizardHead}>
      <div className={styles.wizardSteps} aria-hidden="true">{[1, 2, 3, 4, 5].map(value => <i key={value} data-on={value <= stepNumber || undefined}/>)}</div>
      <div className={styles.wizardStep}><span>{stepLabel}</span><b>{stepTitle}</b></div>
    </div>}
    {step === 'what' && <div className={styles.whatCards} role="radiogroup" aria-label={t('next.remoteSources.whatToConnect')}>
      {whatOptions.map(option => <button key={option.family} type="button" role="radio" aria-checked={family === option.family} className={styles.whatCard}
        data-picked={family === option.family || undefined} disabled={busy} onClick={() => chooseFamily(option.family, renderGuard, formToken)}>
        <span className={styles.choiceRadio} aria-hidden="true"/>
        <span className={styles.whatBody}><b>{t(option.label)}{option.tag && <span className={styles.choiceTag} data-tone={option.tag === 'next.remoteSources.kindCommon' ? 'good' : undefined}>{t(option.tag)}</span>}</b>
          <span className={styles.choiceHint}>{t(option.hint)}</span></span>
      </button>)}
    </div>}
    {step === 'remote' && <>
      {family === 'rclone' && <div className={styles.remoteKinds}>
        {rcloneKindGroups.map(group => <div key={group.title} className={styles.remoteGroup}>
          <p className={styles.advTitle}>{t(group.title)}</p>
          <div className={styles.remoteKindList} role="radiogroup" aria-label={t('next.remoteSources.remoteKind')}>
            {group.kinds.map(kind => <button key={kind.value} type="button" role="radio" aria-checked={sourceKind === kind.value} className={styles.remoteKind}
              data-picked={sourceKind === kind.value || undefined} disabled={busy} onClick={() => chooseRemoteKind(kind.value, renderGuard, formToken)}>
              <span className={styles.choiceRadio} aria-hidden="true"/><Icon name={kind.icon} size={20}/>{t(kind.label)}
            </button>)}
          </div>
        </div>)}
      </div>}
      {sourceKind === 'webdav' ? <>
        <div className={styles.kind}><Icon name="i-globe" size={22} /><span><b>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.label')}</b><small>{t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.0.rows.0.control.options.0.hint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
        <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.0.control.placeholder')} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateDraft({root: event.target.value})}/>
        <TextField label={t('next.remoteSources.username')} value={draft.username} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.1.control.placeholder')} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
        <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} placeholder={t('next.settingsSpec.server.remote-sources.panels.newSource.tabs.0.groups.1.rows.2.control.placeholder')} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
        <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
        </div>
      </> : sourceKind === 'cloud' ? <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectCloud')}</b><small>{t('next.remoteSources.connectCloudHint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={cloudName} maxLength={200} disabled={busy} onChange={event => setCloudName(event.target.value)}/>
        <div className={styles.lab2}>{t('next.remoteSources.cloudProvider')}</div>
        <div className={styles.segment} role="group">
          <Button size="small" variant={cloud.provider === 'onedrive' ? 'primary' : 'glass'} aria-pressed={cloud.provider === 'onedrive'} disabled={busy} onClick={() => updateCloud({provider: 'onedrive'})}>{t('next.remoteSources.cloudOneDrive')}</Button>
          <Button size="small" variant={cloud.provider === 'dropbox' ? 'primary' : 'glass'} aria-pressed={cloud.provider === 'dropbox'} disabled={busy} onClick={() => updateCloud({provider: 'dropbox'})}>{t('next.remoteSources.cloudDropbox')}</Button>
        </div>
        <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.cloudToken')}</span>
          <textarea className={styles.config} aria-label={t('next.remoteSources.cloudToken')} value={cloud.token} placeholder={t('next.remoteSources.cloudTokenPlaceholder')} maxLength={8192}
            disabled={busy} spellCheck={false} rows={5} onChange={event => updateCloud({token: event.target.value})}/>
          <small className={styles.note}>{t('next.remoteSources.cloudTokenHelp')}</small>
        </label>
        {!cloud.token.trim() ? <p className={styles.note}>{t('next.remoteSources.cloudTokenEmpty')}</p>
          : !cloudResult.ok ? <p className={styles.error} role="alert">{t(cloudReason(cloudResult.reason))}</p> : null}
        <TextField label={t('next.remoteSources.cloudFolder')} description={t('next.remoteSources.cloudFolderHelp')} value={cloud.folder} maxLength={1024} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateCloud({folder: event.target.value})}/>
        <p className={styles.note}>{t('next.remoteSources.cloudChecks')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !cloudResult.ok} onClick={() => void testCloud()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </> : sourceKind === 'drive' ? <>
        <div className={styles.kind}><Icon name="i-google" size={22} /><span><b>{t('next.remoteSources.connectDrive')}</b><small>{t('next.remoteSources.connectDriveHint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={driveName} maxLength={200} disabled={busy} onChange={event => setDriveName(event.target.value)}/>
        <div className={styles.lab2}>{t('next.remoteSources.driveSignIn')}</div>
        <div className={styles.kindTiles} role="radiogroup" aria-label={t('next.remoteSources.driveSignIn')}>
          <button type="button" role="radio" aria-checked={driveMode === 'service'} className={styles.kindTile} data-picked={driveMode === 'service' || undefined} disabled={busy}
            onClick={() => {retireOAuth(oauthRenderTicket);invalidateDrive();setDriveTested(undefined);setDriveMode('service');}}><Icon name="i-key" size={22}/>{t('next.remoteSources.driveServiceAccount')}</button>
          <button type="button" role="radio" aria-checked={driveMode === 'oauth'} className={styles.kindTile} data-picked={driveMode === 'oauth' || undefined} disabled={busy}
            onClick={() => {retireOAuth(oauthRenderTicket);invalidateDrive();setDriveTested(undefined);setDriveMode('oauth');}}><Icon name="i-google" size={22}/>{t('next.remoteSources.driveOAuth')}</button>
        </div>
        {driveMode === 'service' ? <>
          <p className={styles.note}>{t('next.remoteSources.driveServiceAccountHint')}</p>
          <a className={styles.consoleLink} href={DRIVE_CONSOLE} target="_blank" rel="noopener noreferrer">{t('next.remoteSources.driveConsole')}</a>
        </> : <>
          <p className={styles.note}>{t('next.remoteSources.driveOAuthHint')}</p>
          <a className={styles.consoleLink} href={DRIVE_CONSOLE} target="_blank" rel="noopener noreferrer">{t('next.remoteSources.driveConsole')}</a>
          <TextField label={t('next.remoteSources.driveClientId')} value={oauthClientId} maxLength={512} disabled={busy}
            autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateOAuthClient({clientId: event.target.value})}/>
          <TextField label={t('next.remoteSources.driveClientSecret')} type="password" value={oauthClientSecret} maxLength={2048} disabled={busy} onChange={event => updateOAuthClient({clientSecret: event.target.value})}/>
          <div className={styles.testRow}>
            <Button size="compact" variant="glass" disabled={busy || !oauthClientId.trim() || !oauthClientSecret} onClick={() => void startOAuth()}>{t('next.remoteSources.driveOAuthSignIn')}</Button>
            {oauthPhase === 'waiting' && <span className={styles.note} role="status">{t('next.remoteSources.driveOAuthWaiting')}</span>}
            {oauthPhase === 'ready' && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{t('next.remoteSources.driveOAuthReady')}</span>}
            {oauthPhase === 'expired' && <span className={styles.note} role="status">{t('next.remoteSources.driveOAuthExpired')}</span>}
          </div>
          {oauthPhase === 'denied' && <p className={styles.error} role="alert">{t('next.remoteSources.driveOAuthDenied')}</p>}
          <p className={styles.note}>{t('next.remoteSources.driveOAuthTestingNote')}</p>
          {oauthPhase === 'waiting' && <>
            <TextField label={t('next.remoteSources.driveOAuthPaste')} value={oauthAddress} maxLength={8192} disabled={busy}
              autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updatePasteAddress(event.target.value)}/>
            <Button size="compact" variant="glass" disabled={busy || !oauthAddress.trim()} onClick={() => void pasteOAuth()}>{t('next.remoteSources.driveOAuthPasteAction')}</Button>
          </>}
          {oauthError && <p className={styles.error} role="alert">{oauthError}</p>}
        </>}
        {driveMode === 'service' && <>
        <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.driveKey')}</span>
          <textarea className={styles.config} aria-label={t('next.remoteSources.driveKey')} value={drive.keyJson} placeholder={t('next.remoteSources.driveKeyPlaceholder')} maxLength={65536}
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
          : !driveResult.ok ? <div className={styles.testRow}><p className={styles.error} role="alert">{t(driveReason(driveResult.reason))}</p>
            {driveResult.reason === 'oauth' && <Button size="xs" variant="glass" disabled={busy}
              onClick={() => {retireOAuth(oauthRenderTicket);invalidateDrive();setDriveTested(undefined);setDriveMode('oauth');}}>{t('next.remoteSources.driveOAuth')}</Button>}</div>
            : <div className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/><span>{driveResult.draft.email}</span>
                <Button size="xs" variant="text" onClick={() => copyEmail(driveResult.draft.email)}>{t('next.remoteSources.driveCopy')}</Button></div>}
        {driveResult.ok && <p className={styles.note}>{t('next.remoteSources.driveEmailHeading')} {t('next.remoteSources.driveEmailHelp')}</p>}
        </>}

        <div className={styles.lab2}>{t('next.remoteSources.driveLocation')}</div>
        <div className={styles.segment} role="group">
          <Button size="small" variant={drive.location === 'folder' ? 'primary' : 'glass'} aria-pressed={drive.location === 'folder'} disabled={busy} onClick={() => updateDrive({location: 'folder'})}>{t('next.remoteSources.driveFolder')}</Button>
          <Button size="small" variant={drive.location === 'drive' ? 'primary' : 'glass'} aria-pressed={drive.location === 'drive'} disabled={busy} onClick={() => updateDrive({location: 'drive'})}>{t('next.remoteSources.driveDrive')}</Button>
        </div>
        <TextField label={t(drive.location === 'drive' ? 'next.remoteSources.driveDriveId' : 'next.remoteSources.driveFolderId')}
          description={t(drive.location === 'drive' ? 'next.remoteSources.driveDriveHelp' : 'next.remoteSources.driveFolderHelp')}
          value={drive.locationId} maxLength={200} disabled={busy} autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateDrive({locationId: event.target.value})}/>
        <p className={styles.note}>{t('next.remoteSources.driveCannotSee')}</p>
        <p className={styles.note}>{t('next.remoteSources.driveChecks')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !driveTestReady} onClick={() => void testDrive()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </> : sourceKind === 's3' ? <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectS3')}</b><small>{t('next.remoteSources.connectS3Hint')}</small></span></div>
        <TextField label={t('next.remoteSources.name')} value={s3Name} maxLength={200} disabled={busy} onChange={event => setS3Name(event.target.value)}/>
        <TextField label={t('next.remoteSources.s3Endpoint')} description={t('next.remoteSources.s3EndpointHelp')} value={s3.endpoint} placeholder={t('next.remoteSources.s3EndpointPlaceholder')} maxLength={2048} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateS3({endpoint: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Region')} description={t('next.remoteSources.s3RegionHelp')} value={s3.region} maxLength={32} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateS3({region: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3AccessKey')} value={s3.accessKey} maxLength={256} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateS3({accessKey: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3SecretKey')} type="password" value={s3.secretKey} maxLength={1024} disabled={busy} onChange={event => updateS3({secretKey: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Bucket')} description={t('next.remoteSources.s3BucketHelp')} value={s3.bucket} maxLength={63} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateS3({bucket: event.target.value})}/>
        <TextField label={t('next.remoteSources.s3Folder')} description={t('next.remoteSources.s3FolderHelp')} value={s3.folder} maxLength={1024} disabled={busy}
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateS3({folder: event.target.value})}/>
        <p className={styles.note}>{t('next.remoteSources.s3Checks')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !s3.endpoint.trim() || !s3.accessKey.trim() || !s3.secretKey || !s3.bucket.trim()} onClick={() => void testS3()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </> : sourceKind === 'config' ? <>
        <div className={styles.kind}><Icon name="i-storage" size={22} /><span><b>{t('next.remoteSources.connectFromConfig')}</b><small>{t('next.remoteSources.connectFromConfigHint')}</small></span></div>
        <ProgramRow program={program} disabled={busy} onCheck={() => void loadProgram()}/>
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
          autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateRclone({remote: event.target.value})}/>
        <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.configuration')}</span>
          <textarea className={styles.config} aria-label={t('next.remoteSources.configuration')} value={rclone.config} placeholder={t('next.remoteSources.configurationPlaceholder')} maxLength={65536}
            disabled={busy} spellCheck={false} rows={8} onChange={event => updateRclone({config: event.target.value})}/>
          <small className={styles.note}>{t('next.remoteSources.configurationHelp')}</small>
        </label>
        <ProgramRow program={program} disabled={busy} onCheck={() => void loadProgram()}/>
        {providers.length > 0 && <p className={styles.note}>{t('next.remoteSources.providers', {providers: providers.join(', ')})}</p>}
        <p className={styles.note}>{t('next.remoteSources.rcloneKindsNote')}</p>
        <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !rclone.remote.trim() || !rclone.config.trim()} onClick={() => void testRclone()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
          {rcloneResult && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{rcloneResult.version ? t('next.remoteSources.testedRclone', {version: rcloneResult.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
        </div>
      </>}
    </>}
    {step === 'reading' && <>
      <p className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.label')}</p>
      {phone
        ? <div className={styles.readingSummary}>
          <span><b>{t(presetName ? presetLabel(presetName) : 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.4.label')}</b>
            {presetName && <small>{t(presetHint(presetName))}</small>}</span>
          <Button size="xs" variant="glass" disabled={busy || loadingProfile || !profile || !sourceNow} onClick={openReadingChange}>{t('next.remoteSources.change')}</Button>
        </div>
        : advancedOpen
          // Frame 4b folds the four choices into one line while Advanced is
          // open, so the page does not double in length. It states only the
          // current profile in the same words the phone uses; it makes no claim
          // that any Advanced value belongs to that choice.
          ? <div className={styles.readingSummary}>
            <span><b>{t(presetName ? presetLabel(presetName) : 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.4.label')}</b>
              {presetName && <small>{t(presetHint(presetName))}</small>}</span>
          </div>
          : <ReadingChoices values={values} origin={result?.origin ?? sourceNow?.origin} rangeSupport={result?.rangeSupport ?? sourceNow?.rangeSupport ?? 'unknown'} disabled={busy} onChange={next => setValues(withLegacyWholeFileReads(next, values.wholeFileReads))}/>}
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null) setValues({...values, requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={values.bytesPerSecond ? values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => setValues({...values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      <AnalysisConsent consent={consentDraft?.current() ? consentDraft.value() : undefined} disabled={busy} onChange={value => {if (!pending.current && consentDraft?.change(value)) showConsent(before => before + 1);}}/>
      <button type="button" className={styles.advRow} aria-expanded={advancedOpen} onClick={() => setAdvancedOpen(value => !value)}>
        <span><b>{t('next.remoteSources.advanced')}</b><small>{t('next.remoteSources.advancedHelp')}</small></span>
        <Icon name="i-down" size={18}/>
      </button>
      {advancedOpen && <SourceAdvancedRows values={advancedValues} cacheSupported transportSupported={!!sourceNow?.mountId} cacheApplied cacheError={undefined} observedInterval={0} disabled={busy}
        onChange={next => {setAdvancedTouched(true);setAdvancedValues(next);}}/>}
    </>}
    {step === 'folders' && <>
      <p className={styles.note}>{t('next.remoteSources.foldersOnlyHint')}</p>
      <div className={styles.lab2}>{t('next.remoteSources.foldersHeader', {name: sourceNow?.name ?? ''})}</div>
      <ul className={styles.pickedList}>
        {chosen.map(folder => <li key={folder.relative} className={styles.pickedRow}>
          <Icon name="i-folder" size={20}/><span className={styles.pickedPath}>{folder.relative ? `/${folder.relative}` : '/'}</span>
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
    {readingChangeDialog}
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


type ManageSourceProps = Readonly<{
  service: RemoteSourceService; source: RemoteSource; api: LibraryContentApi; serverId: string; viewerId: string;
  openSection?: (section: string) => void;
  isCurrent: () => boolean; reportError: (error: unknown) => void; onClose: () => void; onRemoved: () => void;
}>;

/** The dialog's library reader belongs to this opening: created in a layout
 * pass, retired with it, and the body is keyed to that exact resource so a
 * successor opening cannot inherit its state. */
function ManageSourceDialog(props: ManageSourceProps) {
  const owned = useOwnedResource(
    () => new LibraryManagementService({api: props.api, scope: {serverId: props.serverId, viewerId: props.viewerId}}),
    value => value.dispose(),
    // The opening is the service, the source row object and the scope: the
    // parent keys by source id, so a replacement row under the same id must
    // still be a new resource rather than the previous opened one.
    [props.api, props.serverId, props.viewerId, props.service, props.source]);
  if (!owned) return null;
  return <ManageSourceLoaded key={owned.id} {...props} libraries={owned.value} openingId={owned.id}/>;
}

function ManageSourceLoaded({service, source, api, serverId, viewerId, openSection, isCurrent, reportError, onClose, onRemoved, libraries, openingId}: ManageSourceProps & Readonly<{libraries: LibraryManagementService; openingId: number}>) {
  const [draft, setDraft] = useState<WebDavDraft>({name: source.name, root: '', username: '', password: '', insecureLocal: source.insecureLocal});
  const [result, setResult] = useState<DAVConnectionResult>();
  const admission = useProfileAdmission({service, source, isCurrent});
  const held = admission.held;
  const flight = useRef<object | undefined>(undefined);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useLayoutEffect(() => {setBusy(false);}, [admission.token]);
  const [typed, setTyped] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [status, setStatus] = useState<RemoteSourceStatus>();
  // The saved test is admitted by a layout-published lease: the exact dialog
  // owner, service, source row and ready status object the render was made
  // for. A replaced source or status retires it whole.
  const [savedTestLease, setSavedTestLease] = useState<SavedTestLease | null>(null);
  const [savedTest, setSavedTest] = useState<Readonly<{result: SavedSourceTest; generation: string}> | null>(null);
  const [savedTesting, setSavedTesting] = useState(false);
  const savedTestOwner = useRef<object | null>(null);
  const savedTestLeaseRef = useRef<SavedTestLease | null>(null);
  const savedTestRunning = useRef<SavedTestLease | null>(null);
  useLayoutEffect(() => {
    const owner = {};
    savedTestOwner.current = owner;
    const ready = status;
    const lease = ready && ready.sourceId === source.id && ready.generation
      ? Object.freeze({owner, service, source, status: ready, generation: ready.generation}) : null;
    savedTestLeaseRef.current = lease;
    setSavedTestLease(lease);
    // A new lease is a new admission: nothing from the old one stays displayed.
    setSavedTesting(false);
    setSavedTest(null);
    return () => {
      if (savedTestOwner.current !== owner) return;
      savedTestOwner.current = null;
      savedTestLeaseRef.current = null;
      if (savedTestRunning.current?.owner === owner) savedTestRunning.current = null;
    };
  }, [service, source, status]);
  const [changing, setChanging] = useState(false);
  const [advanced, setAdvanced] = useState<{settings?: Awaited<ReturnType<RemoteSourceService['readAdvanced']>>; values: SourceAdvanced}>();
  const [advancedBusy, setAdvancedBusy] = useState(false);
  const [advancedNote, setAdvancedNote] = useState('');
  const [pickingProgram, setPickingProgram] = useState(false);
  const [programNote, setProgramNote] = useState('');
  const [pendingValues, setPendingValues] = useState<SourceReadValues>(sourceReadPreset('list'));
  const librarySnapshot = useSyncExternalStore(libraries.subscribe, libraries.getSnapshot, libraries.getSnapshot);
  const [attachTo, setAttachTo] = useState('');
  const [picking, setPicking] = useState(false);
  const [attachBusy, setAttachBusy] = useState(false);
  const [attachNote, setAttachNote] = useState('');
  const testAttempt = useRef(0), active = useRef(true), pending = useRef(false), lifetime = useRef(0);
  const attemptGuard = () => { const captured = lifetime.current; return () => active.current && lifetime.current === captured && isCurrent(); };
  // The dialog's admission is layout-owned: a replayed mount admits again, and
  // the cleanup retires this exact render synchronously.
  useLayoutEffect(() => {
    active.current = true;
    return () => {active.current=false;lifetime.current++;};
  }, [openingId]);
  const updateDraft = (patch: Partial<WebDavDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {
    // The guard belongs to this rendered dialog, not to a later one.
    const captured = lifetime.current;
    const guard = () => active.current && lifetime.current === captured && isCurrent();
    const exact = admission.token;
    if (!admission.admitted || !exact) return;
    service.readProfile(source.id, String(source.generation), () => admission.current() && guard()).then(next => {
      if (!admission.current() || !guard() || next.sourceId !== source.id || next.generation !== String(source.generation)) return;
      admission.setHeld({token: exact, profile: next, consent: next.values.allowAdvancedMediaAnalysis,
        values: next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('list'), next.values.wholeFileReads)});
    }).catch(value => {if (admission.current() && guard()) setError(remoteErrorText(value));});
    service.readStatus(source.id, guard).then(next => {if (guard()) setStatus(next);}).catch(() => {});
    service.readAdvanced(source.id, guard).then(next => {if (guard()) setAdvanced({settings: next, values: next.values});}).catch(() => {});
    void libraries.loadLibraries();
  }, [service, source.id, source.generation, isCurrent, libraries, admission.admitted, admission.token]);
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
    const guard = attemptGuard(), exact = admission.token, snapshot = held;
    if (pending.current || !guard() || !exact || !snapshot || !admission.holds(snapshot) || flight.current) return;
    const claim = admission.begin(snapshot);if (!claim) return;
    pending.current = true;flight.current = claim;
    const current = () => admission.owns(claim) && admission.holds(snapshot) && guard();
    setError('');setBusy(true);
    try {
      const next = await service.saveProfile(snapshot.profile, readValuesWithConsent(snapshot.values, snapshot.consent, snapshot.consentDirty), current);
      if (!current()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      admission.settle(claim, snapshot, {token: exact, profile: next, consent: next.values.allowAdvancedMediaAnalysis, values: readValuesWithoutConsent(next.values)});
    }
    catch (value) {if (!current()) return;if (authFailure(value)) {active.current=false;lifetime.current++;reportError(value);return;}if (refreshFailure(value)) done(); else setError(remoteErrorText(value));}
    finally {if (flight.current === claim) {flight.current = undefined;pending.current=false;if (admission.finish(claim) && guard()) setBusy(false);}}
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
  const done = () => {
    admission.retire();active.current=false;pending.current=false;lifetime.current++;
    // Retire the test admission with the close gesture, before the unmount
    // layout runs: a retained Test cannot be admitted in between.
    savedTestOwner.current=null;
    savedTestLeaseRef.current=null;
    savedTestRunning.current=null;
    setDraft(current => ({...current, password: ''}));setTyped('');onClose();
  };
  const close = () => {if (busy || pending.current) return;done();};
  /** A saved-source test is its own command; its answer names the effective
   * observation generation, which must still be the one this page reports. */
  /** Runs only the lease the pressed button was rendered with; never a newer one. */
  const testSavedSource = async (lease: SavedTestLease) => {
    // The lease itself is the single-flight claim: a second press while this
    // one runs finds it taken.
    if (savedTestLeaseRef.current !== lease || savedTestRunning.current || pending.current || busy) return;
    const owns = () => savedTestLeaseRef.current === lease && savedTestOwner.current === lease.owner
      && savedTestRunning.current === lease && isCurrent()
      && status === lease.status && service.getSnapshot().sources.indexOf(lease.source) >= 0;
    savedTestRunning.current = lease;
    if (!owns()) { savedTestRunning.current = null; return; }
    setSavedTesting(true);setSavedTest(null);setError('');
    try {
      const result = await service.testSaved(lease.source.id, owns);
      if (!owns() || result.sourceId !== lease.source.id || result.generation !== lease.generation) return;
      setSavedTest({result, generation: lease.generation});
    } catch (value) {
      if (!owns()) return;
      // A failure in Portico's own local helper is not a provider answer:
      // the server reports administration_unavailable, with no contact fact.
      const code = value && typeof value === 'object' ? (value as {code?: unknown}).code : undefined;
      setError(code === 'administration_unavailable' ? t('next.remoteSources.savedTestLocalFailure') : remoteErrorText(value));
    }
    finally {
      if (savedTestRunning.current === lease) {
        savedTestRunning.current = null;
        if (savedTestLeaseRef.current === lease && isCurrent()) setSavedTesting(false);
      }
    }
  };
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
            ? status.libraries.map(use => `${use.name} · ${use.folders}${use.lastCompleteAt ? ` · ${t('next.remoteSources.libraryComplete', {when: checkedWhen(use.lastCompleteAt)})}` : ''}`).join(', ') + (status.librariesTruncated ? ' …' : '')
            : t('next.remoteSources.noLibraryUses')}</dd></div>
        {!!status?.scanIntervalSeconds && <div><dt>{t('next.remoteSources.scanning')}</dt>
          <dd>{t('next.remoteSources.scansEveryHours', {hours: Math.round(status.scanIntervalSeconds / 3600) || 1})}</dd></div>}
        <div><dt>{t('next.remoteSources.savedTest')}</dt><dd><span className={styles.rowActions}>
          <Button size="xs" variant="glass" loading={savedTesting} disabled={busy || savedTesting || !savedTestLease}
            onClick={() => {if (savedTestLease) void testSavedSource(savedTestLease);}}>{t('next.remoteSources.test')}</Button>
          {savedTest && savedTestLease?.status === status && savedTest.generation === status?.generation && <span className={styles.testCard} data-tone={savedTest.result.reachable ? 'good' : 'warning'}>{savedTest.result.reachable
            ? t(savedTest.result.rangeSupport === 'unsupported' ? 'next.remoteSources.savedTestLimited' : 'next.remoteSources.savedTestReachable')
            : t(contactWord(savedTest.result.errorCode!))}</span>}
        </span></dd></div>
        {status?.lastConnectedAt && <div><dt>{t('next.remoteSources.lastConnected')}</dt><dd>{checkedWhen(status.lastConnectedAt)}</dd></div>}
        {status?.lastErrorAt && status.lastErrorCode && <div><dt>{t('next.remoteSources.lastError')}</dt><dd>{checkedWhen(status.lastErrorAt)} · {t(contactWord(status.lastErrorCode))}</dd></div>}
        {status?.health && <div><dt>{t('next.remoteSources.health')}</dt><dd>{t(healthWord(status.health))}</dd></div>}
        {status?.lastCompleteAt && <div><dt>{t('next.remoteSources.lastComplete')}</dt><dd>{checkedWhen(status.lastCompleteAt)}</dd></div>}
        {status?.scan && <>
          <div><dt>{t('next.remoteSources.scanState')}</dt><dd>{[scanStateWord(status.scan.state), scanModeWord(status.scan.mode), scanPhaseWord(status.scan.phase)]
            .filter((word): word is MessageId => Boolean(word)).map(word => t(word)).join(' · ')}</dd></div>
          <div><dt>{t('next.remoteSources.scanFiles')}</dt><dd>{t('next.remoteSources.scanProcessedOf', {processed: status.scan.processed, discovered: status.scan.discovered})}</dd></div>
          <div><dt>{t('next.remoteSources.scanAnalyzed')}</dt><dd>{status.scan.analyzed}</dd></div>
          {status.scan.warnings > 0 && <div><dt>{t('next.remoteSources.scanWarnings')}</dt><dd>{status.scan.warnings}</dd></div>}
          {status.scan.queuedAt && <div><dt>{t('next.remoteSources.scanQueuedAt')}</dt><dd>{checkedWhen(status.scan.queuedAt)}</dd></div>}
          {status.scan.completedAt && <div><dt>{t('next.remoteSources.scanListed')}</dt><dd>{checkedWhen(status.scan.completedAt)}</dd></div>}
          {status.scan.finishedAt && <div><dt>{t('next.remoteSources.scanFinished')}</dt><dd>{checkedWhen(status.scan.finishedAt)}</dd></div>}
        </>}
      </dl>
    </section>
    <section className={styles.group} id="source-connection"><h3>{t('next.remoteSources.connection')}</h3>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.address')} value={draft.root} placeholder={source.origin ?? t('next.settingsSpec.server.remote-sources.panels.source.tabs.0.groups.0.rows.1.control.placeholder')} disabled={busy}
        autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateDraft({root: event.target.value})}/>
      <TextField label={t('next.remoteSources.username')} value={draft.username} disabled={busy} autoCapitalize="off" spellCheck={false} onChange={event => updateDraft({username: event.target.value})}/>
      <TextField label={t('next.remoteSources.password')} type="password" value={draft.password} disabled={busy} onChange={event => updateDraft({password: event.target.value})}/>
      <label className={styles.switchRow}><span>{t('next.remoteSources.insecureLocal')}</span><Switch aria-label={t('next.remoteSources.insecureLocal')} checked={draft.insecureLocal} disabled={busy} onCheckedChange={value => updateDraft({insecureLocal: value})}/></label>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.name.trim() || !draft.root.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone={result.rangeSupport === 'unsupported' ? 'warning' : 'good'}><Icon name="i-check" size={18}/>{t(result.rangeSupport === 'unsupported' ? 'next.remoteSources.testedLimited' : 'next.remoteSources.tested', {origin: result.origin})}</span>}
      </div>
      <Button size="compact" variant="primary" disabled={busy || !draft.name.trim() || (!result && !draft.root.trim())} onClick={() => void saveConnection()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>
      {!held ? <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p> : <>
      <div className={styles.choiceSummary}>
        <span>{t('next.remoteSources.howTreated')}</span>
        <b>{t(presetLabel(sourceReadPresetName(held.values) ?? 'once'))}</b>
        <Button size="xs" variant="glass" disabled={busy} onClick={() => {setPendingValues(held.values);setChanging(true);}}>{t('next.remoteSources.change')}</Button>
      </div>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={held.values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null && held) admission.setHeld(current => current && current.token === held.token ? {...current, values: {...current.values, connections: value}} : current);}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={held.values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null && held) admission.setHeld(current => current && current.token === held.token ? {...current, values: {...current.values, requestsPerSecond: value}} : current);}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={held.values.bytesPerSecond ? held.values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => {if (held) admission.setHeld(current => current && current.token === held.token ? {...current, values: {...current.values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000}} : current);}}/>
      </div>
      <AnalysisConsent consent={held.consent} disabled={busy} onChange={value => {if (held) admission.setHeld(current => current && current.token === held.token ? {...current, consent: value, consentDirty: true} : current);}}/>
      <Button size="compact" variant="primary" disabled={busy} onClick={() => void saveReading()}>{t('next.remoteSources.save')}</Button>
      </>}
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.advanced')}</h3>
      <p className={styles.note}>{t('next.remoteSources.advancedHelp')}</p>
      {advanced
        ? <><SourceAdvancedRows values={advanced.values} cacheSupported={advanced.settings?.cacheSupported ?? false} transportSupported={advanced.settings?.cacheSupported ?? false}
          cacheApplied={advanced.settings?.cacheApplied ?? true} cacheError={advanced.settings?.cacheError} cacheUsedBytes={advanced.settings?.cacheUsedBytes} observedInterval={advanced.settings?.scanIntervalObserved ?? 0}
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
    <Dialog open={changing && !!held} width={560} title={t('next.remoteSources.changeTitle')} description={t('next.remoteSources.changeBody')}
      onOpenChange={setChanging}
      footer={<><Button size="compact" variant="glass" onClick={() => setChanging(false)}>{t('next.remoteSources.cancel')}</Button>
        <Button size="compact" variant="primary" onClick={() => {if (held) admission.setHeld(current => current && current.token === held.token ? {...current, values: withLegacyWholeFileReads(pendingValues, current.values.wholeFileReads)} : current);setChanging(false);}}>{t('next.remoteSources.useChoice')}</Button></>}>
      <ReadingChoices values={pendingValues} origin={source.origin} rangeSupport={source.rangeSupport} disabled={false} onChange={setPendingValues}/>
    </Dialog>
    {picking && <RemoteFolderPicker service={service} sourceId={source.id} sourceName={source.name} open={picking}
      onOpenChange={setPicking} onChoose={folders => {setPicking(false);void attach(folders);}}/>}
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
  const admission = useProfileAdmission({service, source, isCurrent});
  const profileHeld = admission.held;
  const profileFlight = useRef<object | undefined>(undefined);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  useLayoutEffect(() => {setBusy(false);}, [admission.token]);
  const [typed, setTyped] = useState('');
  const [confirming, setConfirming] = useState(false);
  const testAttempt = useRef(0), pending = useRef(false);
  const ownedLease = useOwnedResource(() => ({live: true, service, sourceId: source.id, generation: source.generation, isCurrent, reportError, onClose, onRemoved}),
    value => {value.live = false;}, [service, source, isCurrent]);
  const lease = ownedLease?.value;
  const held = (captured: typeof lease) => () => {try {return !!captured && captured.live && captured.isCurrent();} catch {return false;}};
  const retire = () => {admission.retire();if (lease) lease.live = false;pending.current = false;};
  const refuse = (error: unknown) => {if (!held(lease)()) return;retire();lease?.reportError(error);};
  const stillListed = (captured: typeof lease) => !!captured && held(captured)() && service.getSnapshot().sources.some(row => row.id === captured.sourceId && row.generation === captured.generation);
  const updateDraft = (patch: Partial<RcloneDraft>) => {testAttempt.current++;setResult(undefined);setError('');setDraft(current => ({...current, ...patch}));};
  useEffect(() => {
    // The guard stays with this lease, not with a later source or service.
    const captured = lease;
    const guard = held(captured);
    const exact = admission.token;
    if (!admission.admitted || !exact) return;
    service.readProfile(source.id, String(source.generation), () => admission.current() && guard()).then(next => {
      if (!admission.current() || !guard() || next.sourceId !== source.id || next.generation !== String(source.generation)) return;
      admission.setHeld({token: exact, profile: next, consent: next.values.allowAdvancedMediaAnalysis,
        values: next.saved ? readValuesWithoutConsent(next.values) : withLegacyWholeFileReads(sourceReadPreset('list'), next.values.wholeFileReads)});
    }).catch(value => {
      if (!guard() || !admission.current()) return;
      if (authFailure(value)) {refuse(value);return;}
      setError(remoteErrorText(value));
    });
    service.rcloneProgram().then(next => {if (guard()) setProgram(next);}).catch(value => {
      if (guard() && authFailure(value)) {refuse(value);}
    });
  }, [service, source.id, source.generation, isCurrent, lease, admission.admitted, admission.token]);
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
    const guard = held(lease), exact = admission.token, snapshot = profileHeld;
    if (pending.current || !guard() || !exact || !snapshot || !admission.holds(snapshot) || profileFlight.current) return;
    const claim = admission.begin(snapshot);if (!claim) return;
    pending.current = true;profileFlight.current = claim;
    const current = () => guard() && admission.owns(claim) && admission.holds(snapshot);
    setError('');setBusy(true);
    try {
      const next = await service.saveProfile(snapshot.profile, readValuesWithConsent(snapshot.values, snapshot.consent, snapshot.consentDirty), current);
      if (!current()) return;
      if (service.getSnapshot().error) {setError(service.getSnapshot().error);return;}
      admission.settle(claim, snapshot, {token: exact, profile: next, consent: next.values.allowAdvancedMediaAnalysis, values: readValuesWithoutConsent(next.values)});
    }
    catch (value) {if (!current()) return;if (authFailure(value)) {refuse(value);return;}if (refreshFailure(value)) done(); else setError(remoteErrorText(value));}
    finally {if (profileFlight.current === claim) {profileFlight.current = undefined;pending.current=false;if (admission.finish(claim) && guard()) setBusy(false);}}
  };
  const disconnect = async () => {
    const guard = held(lease);
    if (pending.current || !guard() || !lease || typed.trim() !== source.name) return;
    const current = service.getSnapshot().sources.find(value => value.id === lease.sourceId && value.generation === lease.generation);
    if (!current) {setError(remoteErrorText({code: 'revision_mismatch'}));return;}
    pending.current = true;setError('');setBusy(true);
    try {const receipt = await service.removeRclone(current, guard);if (!guard()) return;const state = service.getSnapshot();
      if (!receipt) {if (authFailure({code: state.errorCode})) {refuse({code: state.errorCode});return;}setError(state.error || t('next.remoteSources.unconfirmed'));return;}
      retire();lease?.onRemoved();}
    finally {if (guard()) {pending.current=false;setBusy(false);}}
  };
  const done = () => {retire();setDraft(current => ({...current, config: ''}));setTyped('');lease?.onClose();};
  const close = () => {if (busy || pending.current) return;done();};
  if (!lease || !held(lease)()) return null;
  return <Dialog open onOpenChange={value => {if (!value) close();}} title={source.name} width={760} footer={<Button size="compact" variant="glass" disabled={busy} onClick={close}>{t('next.remoteSources.cancel')}</Button>}>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.group} id="source-connection"><h3>{t('next.remoteSources.connection')}</h3>
      <p className={styles.note}><SourceStatus source={source}/> {t('next.remoteSources.kindRclone')}</p>
      <TextField label={t('next.remoteSources.name')} value={draft.name} maxLength={100} disabled={busy} onChange={event => updateDraft({name: event.target.value})}/>
      <TextField label={t('next.remoteSources.remoteName')} description={t('next.remoteSources.remoteNameHelp')} value={draft.remote} placeholder={t('next.remoteSources.remoteNamePlaceholder')} maxLength={200} disabled={busy}
        autoCapitalize="off" spellCheck={false} className={styles.monoInput} onChange={event => updateDraft({remote: event.target.value})}/>
      <label className={styles.configField}><span className={styles.configLabel}>{t('next.remoteSources.configuration')}</span>
        <textarea className={styles.config} aria-label={t('next.remoteSources.configuration')} value={draft.config} placeholder={t('next.remoteSources.replaceConfiguration')} maxLength={65536}
          disabled={busy} spellCheck={false} rows={8} onChange={event => updateDraft({config: event.target.value})}/>
        <small className={styles.note}>{t('next.remoteSources.configurationHelp')}</small>
      </label>
      <ProgramRow program={program} disabled={busy}/>
      <div className={styles.testRow}><Button size="compact" variant="glass" disabled={busy || !draft.remote.trim() || !draft.config.trim()} onClick={() => void test()}>{busy ? t('next.remoteSources.testing') : t('next.remoteSources.test')}</Button>
        {result && <span className={styles.testCard} data-tone="good"><Icon name="i-check" size={18}/>{result.version ? t('next.remoteSources.testedRclone', {version: result.version}) : t('next.remoteSources.testedRcloneUnknown')}</span>}
      </div>
      <Button size="compact" variant="primary" disabled={busy || !draft.name.trim() || !draft.remote.trim() || !draft.config.trim()} onClick={() => void saveConnection()}>{t('next.remoteSources.save')}</Button>
    </section>
    <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>
      {!profileHeld ? <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p> : <>
      <ReadingChoices values={profileHeld.values} origin={source.origin} rangeSupport={source.rangeSupport} disabled={busy} onChange={next => admission.setHeld(current => current && current.token === profileHeld.token ? {...current, values: withLegacyWholeFileReads(next, current.values.wholeFileReads)} : current)}/>
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={profileHeld.values.connections} min={1} max={32} unit={t('next.remoteSources.atOnce')} disabled={busy} onCommit={value => {if (value !== null && profileHeld) admission.setHeld(current => current && current.token === profileHeld.token ? {...current, values: {...current.values, connections: value}} : current);}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={profileHeld.values.requestsPerSecond} min={1} max={50} unit={t('next.remoteSources.aSecond')} disabled={busy} onCommit={value => {if (value !== null && profileHeld) admission.setHeld(current => current && current.token === profileHeld.token ? {...current, values: {...current.values, requestsPerSecond: value}} : current);}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={profileHeld.values.bytesPerSecond ? profileHeld.values.bytesPerSecond / 1_000_000 : null} min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={busy} onCommit={value => {if (profileHeld) admission.setHeld(current => current && current.token === profileHeld.token ? {...current, values: {...current.values, bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000}} : current);}}/>
      </div>
      <AnalysisConsent consent={profileHeld.consent} disabled={busy} onChange={value => {if (profileHeld) admission.setHeld(current => current && current.token === profileHeld.token ? {...current, consent: value, consentDirty: true} : current);}}/>
      <Button size="compact" variant="primary" disabled={busy} onClick={() => void saveReading()}>{t('next.remoteSources.save')}</Button>
      </>}
    </section>

  </Dialog>;
}
