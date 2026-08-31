import type { Job, Library, RemoteStorageAnalysisMode, RemoteStorageSource, RemoteStorageSourceRequest } from '@porticomediaserver/client-core';
import { StatusWarningIcon, StatusSuccessIcon, ActionPrepareDownloadIcon, DeviceStorageIcon, ActionAddIcon, ActionRefreshIcon, ActionDeleteIcon, ActionCloseIcon } from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productProblemText } from '../../components/ProductLanguage';
import { isAbsoluteFilesystemPath } from '../filesystem';
import { InlineNotice } from './SettingsControls';
import { useAbortableMutation } from './settingsHooks';
import type { SettingsDataSource } from './settingsTypes';

type RemoteKind = RemoteStorageSourceRequest['kind'];

const analysisModeOptions: Array<{ value: RemoteStorageAnalysisMode; label: string; description: string }> = [
  { value: 'basic', label: 'Basic', description: 'Recommended for rclone and WebDAV. Adds technical facts and representative thumbnails with bounded reads.' },
  { value: 'file_list_only', label: 'File List Only', description: 'Reads no media content during scans. Technical stream data and thumbnails are deferred.' },
  { value: 'complete', label: 'Complete', description: 'Deep whole-file compute, including sonic analysis, loudness, and intro/credit detection; highest cloud traffic.' },
  { value: 'custom', label: 'Custom', description: 'Advanced. Uses exactly the enabled Low, Moderate, and High disk-I/O operations in this library’s Custom analysis settings.' },
];

function analysisModeLabel(mode?: RemoteStorageAnalysisMode) {
  return analysisModeOptions.find((option) => option.value === (mode ?? 'basic'))?.label ?? 'Basic';
}

const completeStorageWarning = 'Complete can temporarily stage an entire remote file for deep analysis, use significantly more server storage for generated files, and create substantially more remote bandwidth and provider operations.';
const customStorageWarning = 'Custom is advanced. Enabled High disk-I/O operations can perform sustained/full-file reads, stage remote objects, and use significantly more network bandwidth and generated storage.';

function formatRemoteCount(value: number) {
  return value.toLocaleString();
}

function formatRemoteTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function remoteLocation(source: RemoteStorageSource) {
  if (source.kind === 'webdav') return [source.endpoint, source.root].filter(Boolean).join(' · ');
  return source.root ? `rclone root: ${source.root}` : 'rclone remote root';
}

function remoteStateLabel(source: RemoteStorageSource) {
  if (source.health === 'healthy') return 'Healthy';
  if (source.health === 'unknown') return 'Not checked';
  return source.health.charAt(0).toUpperCase() + source.health.slice(1);
}

function RemoteStorageEditor({
  library,
  source,
  onDismiss,
  onCreated,
}: {
  library: Library;
  source: SettingsDataSource;
  onDismiss: () => void;
  onCreated: (created: RemoteStorageSource) => void;
}) {
  const [kind, setKind] = useState<RemoteKind>('webdav');
  const [name, setName] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [root, setRoot] = useState('');
  const [analysisMode, setAnalysisMode] = useState<RemoteStorageAnalysisMode>('basic');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [rcloneBinaryPath, setRcloneBinaryPath] = useState('');
  const [rcloneRemoteName, setRcloneRemoteName] = useState('');
  const [rcloneConfig, setRcloneConfig] = useState('');
  const [error, setError] = useState('');
  const nameInput = useRef<HTMLInputElement>(null);
  const mutation = useAbortableMutation();

  const clearSecrets = () => {
    setUsername('');
    setPassword('');
    setRcloneConfig('');
  };

  const submit = async () => {
    const cleanName = name.trim();
    if (!cleanName) {
      setError('Enter a source name.');
      return;
    }
    const input: RemoteStorageSourceRequest = { kind, name: cleanName, root: root.trim(), analysisMode };
    if (kind === 'webdav') {
      let parsed: URL;
      try {
        parsed = new URL(endpoint.trim());
      } catch {
        setError('Enter a valid WebDAV endpoint.');
        return;
      }
      if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) {
        setError('Use an HTTP or HTTPS WebDAV endpoint without credentials in the URL.');
        return;
      }
      input.endpoint = parsed.toString();
      input.username = username;
      input.password = password;
    } else {
      if (!isAbsoluteFilesystemPath(rcloneBinaryPath.trim())) {
        setError('Enter the absolute server path to the rclone executable.');
        return;
      }
      if (!rcloneRemoteName.trim()) {
        setError('Enter the remote name used in the rclone config.');
        return;
      }
      if (!rcloneConfig.trim()) {
        setError('Paste the rclone config for this remote.');
        return;
      }
      input.rcloneBinaryPath = rcloneBinaryPath.trim();
      input.rcloneRemoteName = rcloneRemoteName.trim();
      input.rcloneConfig = rcloneConfig;
    }
    setError('');
    try {
      const created = await mutation.run((signal) => source.createRemoteStorageSource(library.id, input, signal));
      clearSecrets();
      onCreated(created);
    } catch (reason) {
      clearSecrets();
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `add this ${kind === 'webdav' ? 'WebDAV' : 'rclone'} source` }));
    }
  };

  const dismiss = mutation.busy ? () => undefined : () => {
    clearSecrets();
    onDismiss();
  };

  return <ModalOverlay labelledBy="portico-remote-storage-editor-title" className="portico-settings-dialog portico-remote-storage-dialog" initialFocusRef={nameInput} onDismiss={dismiss}>
    <header>
      <div><h2 id="portico-remote-storage-editor-title">Add remote storage</h2><p>{library.name}</p></div>
      <IconButton label="Close" disabled={mutation.busy} onClick={dismiss}><ActionCloseIcon /></IconButton>
    </header>
    <div className="portico-settings-dialog-fields">
      <fieldset className="portico-remote-kind">
        <legend>Connection type</legend>
        <label><input type="radio" name="remote-kind" value="webdav" checked={kind === 'webdav'} disabled={mutation.busy} onChange={() => { setKind('webdav'); setError(''); }} /><span><strong>WebDAV</strong><small>Connect directly to a WebDAV endpoint.</small></span></label>
        <label><input type="radio" name="remote-kind" value="rclone" checked={kind === 'rclone'} disabled={mutation.busy} onChange={() => { setKind('rclone'); setError(''); }} /><span><strong>rclone</strong><small>Use a validated rclone binary and an isolated config.</small></span></label>
      </fieldset>
      <label><span>Name</span><input ref={nameInput} aria-label="Remote source name" value={name} disabled={mutation.busy} maxLength={120} autoComplete="off" onChange={(event) => setName(event.target.value)} placeholder={kind === 'webdav' ? 'Cloud Movies' : 'Archive remote'} /></label>
      {kind === 'webdav' ? <>
        <label><span>Endpoint</span><input aria-label="WebDAV endpoint" type="url" value={endpoint} disabled={mutation.busy} autoComplete="url" spellCheck={false} onChange={(event) => setEndpoint(event.target.value)} placeholder="https://dav.example.com/media" /></label>
        <label><span>Root <small>Optional path within the endpoint</small></span><input aria-label="WebDAV root" value={root} disabled={mutation.busy} autoComplete="off" spellCheck={false} onChange={(event) => setRoot(event.target.value)} placeholder="Movies" /></label>
        <div className="portico-remote-credential-fields">
          <label><span>Username <small>Optional</small></span><input aria-label="WebDAV username" value={username} disabled={mutation.busy} autoComplete="username" onChange={(event) => setUsername(event.target.value)} /></label>
          <label><span>Password <small>Stored encrypted</small></span><input aria-label="WebDAV password" type="password" value={password} disabled={mutation.busy} autoComplete="new-password" onChange={(event) => setPassword(event.target.value)} /></label>
        </div>
      </> : <>
        <label><span>rclone executable</span><input aria-label="rclone binary path" value={rcloneBinaryPath} disabled={mutation.busy} autoComplete="off" spellCheck={false} onChange={(event) => setRcloneBinaryPath(event.target.value)} placeholder="/usr/local/bin/rclone" /></label>
        <div className="portico-remote-credential-fields">
          <label><span>Remote name</span><input aria-label="rclone remote name" value={rcloneRemoteName} disabled={mutation.busy} autoComplete="off" spellCheck={false} onChange={(event) => setRcloneRemoteName(event.target.value)} placeholder="media" /></label>
          <label><span>Root <small>Optional</small></span><input aria-label="rclone root" value={root} disabled={mutation.busy} autoComplete="off" spellCheck={false} onChange={(event) => setRoot(event.target.value)} placeholder="Movies" /></label>
        </div>
        <label><span>rclone config <small>Write-only secret</small></span><textarea aria-label="rclone config" value={rcloneConfig} disabled={mutation.busy} autoComplete="off" spellCheck={false} rows={7} onChange={(event) => setRcloneConfig(event.target.value)} placeholder={'[media]\ntype = webdav\nurl = https://…'} /></label>
      </>}
      <label className="portico-remote-analysis-mode">
        <span>Scan depth</span>
        <select aria-label="Remote scan depth" value={analysisMode} disabled={mutation.busy} onChange={(event) => setAnalysisMode(event.target.value as RemoteStorageAnalysisMode)}>
          {analysisModeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
        <small>{analysisModeOptions.find((option) => option.value === analysisMode)?.description}</small>
      </label>
      {analysisMode === 'complete' && <InlineNotice tone="warn">{completeStorageWarning}</InlineNotice>}
      {analysisMode === 'custom' && <InlineNotice tone="warn">{customStorageWarning}</InlineNotice>}
      <p className="portico-remote-secret-note">Credentials and rclone configuration are sent once. Portico will not show them again.</p>
      {error && <p className="portico-settings-dialog-error" role="alert"><StatusWarningIcon />{error}</p>}
    </div>
    <footer>
      <SecondaryButton disabled={mutation.busy} onClick={dismiss}>Cancel</SecondaryButton>
      <PrimaryButton disabled={mutation.busy} onClick={() => void submit()}>{mutation.busy ? 'Adding source…' : 'Add source'}</PrimaryButton>
    </footer>
  </ModalOverlay>;
}

export function RemoteStorageSettings({ library, source, onScanQueued }: { library: Library; source: SettingsDataSource; onScanQueued: (job: Job) => void }) {
  const [items, setItems] = useState<RemoteStorageSource[]>([]);
  const [loading, setLoading] = useState(true);
  const [editorOpen, setEditorOpen] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState('');
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();

  const load = useCallback(async (signal: AbortSignal) => {
    setLoading(true);
    try {
      const remoteSources = await source.remoteStorageSources(library.id, signal);
      if (!signal.aborted) setItems(remoteSources);
    } catch (reason) {
      if (!signal.aborted) setError(productProblemText(reason, 'settings.action-failed', { actionName: `load remote storage for ${library.name}` }));
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [library.id, library.name, source]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const inventory = async (item: RemoteStorageSource) => {
    setFeedback('');
    setError('');
    try {
      const queued = await mutation.run((signal) => source.inventoryRemoteStorageSource(library.id, item.id, signal));
      setFeedback(`Inventory scan queued for ${item.name}.`);
      onScanQueued(queued);
      const controller = new AbortController();
      await load(controller.signal);
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `scan ${item.name}` }));
    }
  };

  const remove = async (item: RemoteStorageSource) => {
    setFeedback('');
    setError('');
    try {
      await mutation.run((signal) => source.deleteRemoteStorageSource(library.id, item.id, signal));
      setItems((current) => current.filter((candidate) => candidate.id !== item.id));
      setConfirmRemove('');
      setFeedback(`${item.name} removed. Remote files were not changed.`);
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `remove ${item.name}` }));
    }
  };

  const updateAnalysisMode = async (item: RemoteStorageSource, analysisMode: RemoteStorageAnalysisMode) => {
    setFeedback('');
    setError('');
    try {
      const updated = await mutation.run((signal) => source.updateRemoteStorageSourceAnalysisMode(library.id, item.id, analysisMode, signal));
      setItems((current) => current.map((candidate) => candidate.id === item.id ? updated : candidate));
      setFeedback(`${item.name} scan depth updated to ${analysisModeLabel(analysisMode).toLocaleLowerCase()}.`);
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `update scan depth for ${item.name}` }));
    }
  };

  return <section className="portico-remote-storage" aria-label={`${library.name} remote storage`}>
    <header>
      <span><strong>Remote storage</strong><small>Managed WebDAV and rclone sources are inventoried without walking a mounted filesystem.</small></span>
      <SecondaryButton disabled={mutation.busy} onClick={() => setEditorOpen(true)}><ActionAddIcon /> Add source</SecondaryButton>
    </header>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    {loading ? <p className="portico-remote-storage-state"><ActionRefreshIcon className="portico-settings-spinner" /> Loading remote sources…</p>
      : items.length === 0 ? <p className="portico-remote-storage-state"><ActionPrepareDownloadIcon /> No managed remote sources.</p>
        : <div className="portico-remote-storage-list">{items.map((item) => <article key={item.id}>
          <div className="portico-remote-storage-identity">
            <DeviceStorageIcon />
            <span><strong>{item.name}</strong><small>{item.kind === 'webdav' ? 'WebDAV' : 'rclone'} · {remoteLocation(item)}</small></span>
          </div>
          <dl>
            <div><dt>Health</dt><dd className={item.health === 'healthy' ? 'healthy' : item.health === 'unknown' ? '' : 'warning'}>{item.health === 'healthy' && <StatusSuccessIcon />}{remoteStateLabel(item)}</dd></div>
            <div><dt>Inventory</dt><dd>{item.inventoryStatus}</dd></div>
            <div className="portico-remote-storage-mode"><dt>Scan depth</dt><dd><select aria-label={`Scan depth for ${item.name}`} value={item.analysisMode ?? 'basic'} disabled={mutation.busy} onChange={(event) => void updateAnalysisMode(item, event.target.value as RemoteStorageAnalysisMode)}>{analysisModeOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></dd></div>
            <div><dt>Objects</dt><dd>{formatRemoteCount(item.objects)}{item.missingObjects > 0 ? ` · ${formatRemoteCount(item.missingObjects)} missing` : ''}</dd></div>
            <div><dt>Updated</dt><dd>{formatRemoteTime(item.updatedAt)}</dd></div>
          </dl>
          {item.analysisMode === 'complete' && <InlineNotice tone="warn">{completeStorageWarning}</InlineNotice>}
          {item.analysisMode === 'custom' && <InlineNotice tone="warn">{customStorageWarning}</InlineNotice>}
          <div className="portico-remote-storage-actions">
            <SecondaryButton disabled={mutation.busy} onClick={() => void inventory(item)}><ActionRefreshIcon /> Scan</SecondaryButton>
            {confirmRemove === item.id
              ? <div className="portico-inline-confirm"><span>Remove {item.name}? Portico will delete its saved connection, not remote files.</span><button type="button" onClick={() => setConfirmRemove('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void remove(item)}>Remove source</button></div>
              : <IconButton label={`Remove ${item.name}`} disabled={mutation.busy} onClick={() => setConfirmRemove(item.id)}><ActionDeleteIcon /></IconButton>}
          </div>
        </article>)}</div>}
    {editorOpen && <RemoteStorageEditor
      library={library}
      source={source}
      onDismiss={() => setEditorOpen(false)}
      onCreated={(created) => {
        setItems((current) => [...current, created]);
        setEditorOpen(false);
        setFeedback(`${created.name} added. Run a scan to inventory the remote.`);
        setError('');
      }}
    />}
  </section>;
}
