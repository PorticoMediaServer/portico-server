import type { LiveTVSource, LiveTVSourceRequest } from '@porticomediaserver/client-core';
import {
  AlertTriangle,
  Antenna,
  CheckCircle2,
  Clock3,
  HardDrive,
  Pencil,
  Plus,
  RadioTower,
  RefreshCw,
  Trash2,
  X,
} from '#portico-icons';
import { useCallback, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import {
  ChoiceControl,
  InlineNotice,
  NumberControl,
  SettingsError,
  SettingsGroup,
  SettingsLoading,
  StringListControl,
  TextControl,
  ToggleControl,
} from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type { DVRStatusSnapshot, SettingsDataSource, SettingsViewer } from './settingsTypes';
import { requestError } from '../live-tv/liveFormat';

const sourceTypes = [
  { value: 'm3u', label: 'M3U playlist' },
  { value: 'xmltv', label: 'XMLTV guide only' },
  { value: 'xtream', label: 'Xtream Codes' },
  { value: 'hdhomerun', label: 'HDHomeRun' },
];

function bytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = Math.max(0, value);
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 10 || index === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

function timeLabel(value?: string): string {
  if (!value) return 'Never';
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString();
}

function can(viewer: SettingsViewer, permission: string, fallback: boolean): boolean {
  return viewer.permissions?.[permission] ?? fallback;
}

function SourceEditor({ source, dataSource, onDismiss, onSaved }: {
  source?: LiveTVSource;
  dataSource: SettingsDataSource;
  onDismiss: () => void;
  onSaved: (message: string) => void;
}) {
  const mutation = useAbortableMutation();
  const [name, setName] = useState(source?.name ?? '');
  const [type, setType] = useState<LiveTVSourceRequest['type']>(source?.type ?? 'm3u');
  const [enabled, setEnabled] = useState(source?.enabled ?? true);
  const [m3uUrl, setM3uUrl] = useState(source?.m3uUrl ?? '');
  const [m3uText, setM3uText] = useState('');
  const [epgUrl, setEpgUrl] = useState(source?.epgUrl ?? '');
  const [epgText, setEpgText] = useState('');
  const [xtreamBaseUrl, setXtreamBaseUrl] = useState(source?.xtreamBaseUrl ?? '');
  const [xtreamUsername, setXtreamUsername] = useState(source?.xtreamUsername ?? '');
  const [xtreamPassword, setXtreamPassword] = useState('');
  const [hdhomerunBaseUrl, setHdhomerunBaseUrl] = useState(source?.hdhomerunBaseUrl ?? '');
  const [streamBufferSeconds, setStreamBufferSeconds] = useState(source?.streamBufferSeconds ?? 18);
  const [maxRetrySeconds, setMaxRetrySeconds] = useState(source?.maxRetrySeconds ?? 45);
  const [refreshIntervalHours, setRefreshIntervalHours] = useState(source?.refreshIntervalHours ?? 12);
  const [overrideTunerCount, setOverrideTunerCount] = useState(source?.tunerCountMode === 'overridden');
  const [tunerCount, setTunerCount] = useState(source?.tunerCount ?? 1);
  const [filterRequireEpg, setFilterRequireEpg] = useState(source?.filterRequireEpg ?? false);
  const [filterCategories, setFilterCategories] = useState(source?.filterCategories ?? []);
  const [filterCountries, setFilterCountries] = useState(source?.filterCountries ?? []);
  const [keywordAllow, setKeywordAllow] = useState(source?.keywordAllow ?? []);
  const [keywordDeny, setKeywordDeny] = useState(source?.keywordDeny ?? []);
  const [error, setError] = useState('');

  const input = (): LiveTVSourceRequest => ({
    name: name.trim(),
    type,
    enabled,
    m3uUrl: m3uUrl.trim() || undefined,
    m3uText: m3uText.trim() || undefined,
    preserveM3uText: Boolean(source?.hasM3uText && !m3uText.trim()),
    epgUrl: epgUrl.trim() || undefined,
    epgText: epgText.trim() || undefined,
    preserveEpgText: Boolean(source?.hasEpgText && !epgText.trim()),
    xtreamBaseUrl: xtreamBaseUrl.trim() || undefined,
    xtreamUsername: xtreamUsername.trim() || undefined,
    xtreamPassword: xtreamPassword || undefined,
    preserveXtreamPassword: Boolean(source?.hasXtreamPassword && !xtreamPassword),
    hdhomerunBaseUrl: hdhomerunBaseUrl.trim() || undefined,
    streamBufferSeconds,
    maxRetrySeconds,
    refreshIntervalHours,
    tunerCount: overrideTunerCount ? tunerCount : source ? 0 : undefined,
    filterCategories,
    filterCountries,
    filterRequireEpg,
    keywordAllow,
    keywordDeny,
  });

  const save = async (testBeforeSaving: boolean) => {
    if (name.trim().length < 2) {
      setError('Enter a source name with at least two characters.');
      return;
    }
    setError('');
    try {
      if (source) {
        await mutation.run((signal) => dataSource.updateLiveTVSource(source.id, input(), signal));
        onSaved(`${name.trim()} saved.`);
      } else {
        await mutation.run((signal) => dataSource.createLiveTVSource(input(), testBeforeSaving, signal));
        onSaved(testBeforeSaving ? `${name.trim()} tested and added.` : `${name.trim()} added without an import test.`);
      }
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'save this source' }));
    }
  };

  return <ModalOverlay labelledBy="portico-live-source-editor-title" className="portico-settings-dialog portico-live-source-dialog" onDismiss={onDismiss}>
    <header><div><h2 id="portico-live-source-editor-title">{source ? `Edit ${source.name}` : 'Add Live TV source'}</h2><p>Connection, import, and guide policy</p></div><IconButton label="Close" onClick={onDismiss}><X /></IconButton></header>
    <div className="portico-settings-dialog-fields">
      <div className="portico-live-source-basics">
        <label><span>Name</span><TextControl label="Source name" value={name} onChange={setName} /></label>
        <div><span>Source type</span><ChoiceControl label="Source type" value={type} options={sourceTypes} onChange={(value) => setType(value as LiveTVSourceRequest['type'])} /></div>
        <label className="portico-live-source-enabled"><span>Enabled</span><ToggleControl label="Enable source" value={enabled} onChange={setEnabled} /></label>
      </div>

      {type === 'm3u' && <fieldset><legend>M3U playlist</legend>
        <label><span>Playlist URL</span><TextControl label="M3U playlist URL" value={m3uUrl} placeholder="https://provider.example/channels.m3u" onChange={setM3uUrl} /></label>
        <label><span>Playlist text <small>{source?.hasM3uText ? 'Leave empty to preserve the uploaded playlist' : 'Optional when a URL is supplied'}</small></span><TextControl label="M3U playlist text" value={m3uText} multiline onChange={setM3uText} /></label>
      </fieldset>}

      {type === 'xmltv' && <fieldset><legend>XMLTV guide</legend>
        <label><span>Guide URL</span><TextControl label="XMLTV guide URL" value={epgUrl} placeholder="https://provider.example/guide.xml" onChange={setEpgUrl} /></label>
        <label><span>Guide XML <small>{source?.hasEpgText ? 'Leave empty to preserve the uploaded guide' : 'Optional when a URL is supplied'}</small></span><TextControl label="XMLTV guide text" value={epgText} multiline onChange={setEpgText} /></label>
      </fieldset>}

      {type === 'xtream' && <fieldset><legend>Xtream account</legend>
        <label><span>Base URL</span><TextControl label="Xtream base URL" value={xtreamBaseUrl} placeholder="https://provider.example" onChange={setXtreamBaseUrl} /></label>
        <label><span>Username</span><TextControl label="Xtream username" value={xtreamUsername} onChange={setXtreamUsername} /></label>
        <label><span>Password <small>{source?.hasXtreamPassword ? 'Leave empty to keep the current password' : 'Required'}</small></span><TextControl label="Xtream password" type="password" value={xtreamPassword} onChange={setXtreamPassword} /></label>
      </fieldset>}

      {type === 'hdhomerun' && <fieldset><legend>HDHomeRun tuner</legend>
        <label><span>Device URL</span><TextControl label="HDHomeRun device URL" value={hdhomerunBaseUrl} placeholder="http://192.168.1.50" onChange={setHdhomerunBaseUrl} /></label>
      </fieldset>}

      {type !== 'xmltv' && <fieldset><legend>Guide data <small>Optional</small></legend>
        <label><span>XMLTV URL</span><TextControl label="EPG URL" value={epgUrl} placeholder="https://provider.example/guide.xml" onChange={setEpgUrl} /></label>
        <label><span>XMLTV text <small>{source?.hasEpgText ? 'Leave empty to preserve the uploaded guide' : 'Optional when a URL is supplied'}</small></span><TextControl label="EPG text" value={epgText} multiline onChange={setEpgText} /></label>
      </fieldset>}

      <fieldset><legend>Refresh and playback</legend><div className="portico-live-source-number-grid">
        <label><span>Refresh interval</span><NumberControl label="Guide refresh interval" value={refreshIntervalHours} min={1} max={168} unit="hours" onChange={(value) => setRefreshIntervalHours(value ?? 12)} /></label>
        <label><span>Stream buffer</span><NumberControl label="Stream buffer" value={streamBufferSeconds} min={5} max={120} unit="seconds" onChange={(value) => setStreamBufferSeconds(value ?? 18)} /></label>
        <label><span>Retry window</span><NumberControl label="Retry window" value={maxRetrySeconds} min={5} max={300} unit="seconds" onChange={(value) => setMaxRetrySeconds(value ?? 45)} /></label>
        <label><span>Tuner capacity</span><NumberControl label="Tuner capacity" value={tunerCount} min={1} max={64} unit={tunerCount === 1 ? 'tuner' : 'tuners'} disabled={!overrideTunerCount} onChange={(value) => setTunerCount(value ?? 1)} /></label>
      </div></fieldset>
      <label className="portico-live-source-check"><input type="checkbox" checked={overrideTunerCount} onChange={(event) => setOverrideTunerCount(event.target.checked)} /><span>Override detected tuner capacity{source ? ` (currently ${source.tunerCountMode === 'discovered' ? `detected as ${source.discoveredTunerCount ?? source.tunerCount}` : source.tunerCountMode === 'default' ? `using safe default ${source.tunerCount}` : `${source.tunerCount} configured`})` : ''}</span></label>

      <fieldset><legend>Channel filters</legend>
        <div className="portico-live-source-filter-grid">
          <label><span>Categories</span><StringListControl label="Filter categories" value={filterCategories} onChange={setFilterCategories} /></label>
          <label><span>Countries</span><StringListControl label="Filter countries" value={filterCountries} onChange={setFilterCountries} /></label>
          <label><span>Required keywords</span><StringListControl label="Allowed keywords" value={keywordAllow} onChange={setKeywordAllow} /></label>
          <label><span>Blocked keywords</span><StringListControl label="Denied keywords" value={keywordDeny} onChange={setKeywordDeny} /></label>
        </div>
        <label className="portico-live-source-check"><input type="checkbox" checked={filterRequireEpg} onChange={(event) => setFilterRequireEpg(event.target.checked)} /><span>Only import channels with matching guide data</span></label>
      </fieldset>
      {error && <p className="portico-settings-dialog-error" role="alert"><AlertTriangle />{error}</p>}
    </div>
    <footer>
      <SecondaryButton disabled={mutation.busy} onClick={onDismiss}>Cancel</SecondaryButton>
      {!source && <SecondaryButton disabled={mutation.busy} onClick={() => void save(false)}>Add without test</SecondaryButton>}
      <PrimaryButton disabled={mutation.busy} onClick={() => void save(!source)}>{mutation.busy ? 'Working…' : source ? 'Save source' : 'Test & add source'}</PrimaryButton>
    </footer>
  </ModalOverlay>;
}

function SourceInventory({ dataSource, canManage }: { dataSource: SettingsDataSource; canManage: boolean }) {
  const [revision, setRevision] = useState(0);
  const [editor, setEditor] = useState<LiveTVSource | 'new'>();
  const [confirmDelete, setConfirmDelete] = useState('');
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const load = useCallback((source: SettingsDataSource, signal: AbortSignal) => canManage ? source.liveTVSources(signal) : Promise.resolve([]), [canManage]);
  const query = useSettingsQuery(load, dataSource, revision);
  const reload = () => setRevision((current) => current + 1);

  const refresh = async (source: LiveTVSource) => {
    setFeedback(''); setError('');
    try {
      const updated = await mutation.run((signal) => dataSource.refreshLiveTVSource(source.id, signal));
      setFeedback(`${updated.name} refreshed ${updated.channelCount} channels and ${updated.programCount} guide entries.`);
      reload();
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: `refresh ${source.name}` }));
    }
  };

  const remove = async (source: LiveTVSource) => {
    setFeedback(''); setError('');
    try {
      await mutation.run((signal) => dataSource.deleteLiveTVSource(source.id, signal));
      setConfirmDelete('');
      setFeedback(`${source.name} removed.`);
      reload();
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: `remove ${source.name}` }));
    }
  };

  return <SettingsGroup title="Live TV sources" description="Provider connections, imported channel counts, guide cache, and source health." actions={canManage ? <PrimaryButton onClick={() => setEditor('new')}><Plus /> Add source</PrimaryButton> : undefined}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    {!canManage && <div className="portico-settings-readonly-note"><Antenna />Your account cannot inspect or manage server source credentials.</div>}
    {canManage && query.status === 'loading' && <SettingsLoading label="Loading Live TV sources" />}
    {canManage && query.status === 'error' && <SettingsError title="Live TV sources are unavailable" message={reviewedProductErrorText(query.error, 'live-tv.load-failed', { featureName: 'Live TV sources' })} onRetry={reload} />}
    {canManage && query.status === 'success' && (query.data.length === 0
      ? <div className="portico-settings-state"><Antenna /><strong>No Live TV sources</strong><p>Add an M3U, XMLTV, Xtream, or HDHomeRun source to begin importing channels.</p><PrimaryButton onClick={() => setEditor('new')}><Plus /> Add source</PrimaryButton></div>
      : <div className="portico-live-source-list">{query.data.map((source) => {
        const actions = new Set(source.actions ?? []);
        return <article key={source.id} className={source.lastError ? 'error' : ''}>
          <span className={`portico-live-source-icon ${source.enabled ? 'enabled' : ''}`}>{source.lastError ? <AlertTriangle /> : <Antenna />}</span>
          <span className="portico-live-source-copy"><strong>{source.name}</strong><small>{source.type.toLocaleUpperCase()} · {source.enabled ? 'enabled' : 'disabled'} · refreshed {timeLabel(source.lastRefreshedAt)}</small>{source.lastError && <em>{requestError({ code: 'live_tv_guide_unavailable' }, 'live-tv.guide-unavailable')}</em>}</span>
          <dl><div><dt>Channels</dt><dd>{source.channelCount}</dd></div><div><dt>Guide</dt><dd>{source.programCount}</dd></div><div><dt>Tuners</dt><dd>{source.tunerCount} <small>{source.tunerCountMode}</small></dd></div></dl>
          <div className="portico-live-source-actions">
            {actions.has('live.source.refresh') && <SecondaryButton disabled={mutation.busy} onClick={() => void refresh(source)}><RefreshCw className={mutation.busy ? 'portico-settings-spinner' : ''} /> Refresh</SecondaryButton>}
            {actions.has('live.source.edit') && <IconButton label={`Edit ${source.name}`} disabled={mutation.busy} onClick={() => setEditor(source)}><Pencil /></IconButton>}
            {actions.has('live.source.delete') && (confirmDelete === source.id
              ? <div className="portico-inline-confirm"><span>Delete {source.name}? Portico will remove this source and its guide cache; the upstream tuner or playlist is unchanged.</span><button type="button" onClick={() => setConfirmDelete('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void remove(source)}>Delete source</button></div>
              : <IconButton label={`Delete ${source.name}`} disabled={mutation.busy} onClick={() => setConfirmDelete(source.id)}><Trash2 /></IconButton>)}
          </div>
        </article>;
      })}</div>)}
    {editor && <SourceEditor source={editor === 'new' ? undefined : editor} dataSource={dataSource} onDismiss={() => setEditor(undefined)} onSaved={(message) => { setEditor(undefined); setFeedback(message); reload(); }} />}
  </SettingsGroup>;
}

function DVRStatusPanel({ dataSource, canView }: { dataSource: SettingsDataSource; canView: boolean }) {
  const [revision, setRevision] = useState(0);
  const load = useCallback((source: SettingsDataSource, signal: AbortSignal) => canView ? source.dvrStatus(undefined, signal) : Promise.resolve(null), [canView]);
  const query = useSettingsQuery<DVRStatusSnapshot | null>(load, dataSource, revision);
  if (!canView) return <SettingsGroup title="DVR operations" description="Guide, tuners, conflicts, and recording storage."><div className="portico-settings-readonly-note"><RadioTower />Your account cannot view DVR operations.</div></SettingsGroup>;
  if (query.status === 'loading') return <SettingsGroup title="DVR operations" description="Guide, tuners, conflicts, and recording storage."><SettingsLoading label="Loading DVR status" /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title="DVR operations" description="Guide, tuners, conflicts, and recording storage."><SettingsError title="DVR status is unavailable" message={reviewedProductErrorText(query.error, 'live-tv.load-failed', { featureName: 'DVR status' })} onRetry={() => setRevision((current) => current + 1)} /></SettingsGroup>;
  const status = query.data;
  if (!status) return null;
  if (!status.configured) return <SettingsGroup title="DVR operations" description="Guide, tuners, conflicts, and recording storage."><div className="portico-settings-state"><RadioTower /><strong>DVR is not configured</strong><p>Add a compatible Live TV source and recording storage before scheduling recordings.</p></div></SettingsGroup>;

  const busyTuners = status.tuners.filter((tuner) => tuner.state !== 'idle').length;
  const totalStorage = status.storage.usedBytes + status.storage.availableBytes;
  const usedPercent = totalStorage > 0 ? Math.round((status.storage.usedBytes / totalStorage) * 100) : 0;
  return <SettingsGroup title="DVR operations" description="Guide, tuners, conflicts, and recording storage." actions={<SecondaryButton onClick={() => setRevision((current) => current + 1)}><RefreshCw /> Refresh status</SecondaryButton>}>
    {!status.available && <InlineNotice tone="error">DVR is configured but currently unavailable.</InlineNotice>}
    <div className="portico-dvr-ledger">
      <div><Clock3 /><span><small>Guide</small><strong>{status.guide.state.replace('-', ' ')}</strong><em>{status.guide.messageId ? requestError({ messageId: status.guide.messageId }, 'live-tv.guide-unavailable') : `Updated ${timeLabel(status.guide.lastRefreshedAt)}`}</em></span></div>
      <div><RadioTower /><span><small>Tuners</small><strong>{busyTuners} busy · {status.tuners.length} total</strong><em>{status.tuners.filter((tuner) => tuner.state === 'offline').length} offline</em></span></div>
      <div><HardDrive /><span><small>Recording storage</small><strong>{bytes(status.storage.usedBytes)} used</strong><em>{usedPercent}% · {bytes(status.storage.availableBytes)} available</em></span></div>
      <div><CheckCircle2 /><span><small>Forecast</small><strong>{status.storage.forecastDays === undefined ? 'Unavailable' : `${status.storage.forecastDays} days`}</strong><em>{status.storage.state}</em></span></div>
    </div>
    <div className="portico-dvr-columns">
      <section><header><h3>Tuners</h3><span>{status.tuners.length}</span></header>{status.tuners.length === 0 ? <div className="portico-status-empty compact"><Antenna /><span><strong>No tuners reported</strong><p>The server did not return tuner inventory.</p></span></div> : <div className="portico-dvr-tuner-list">{status.tuners.map((tuner) => <div key={tuner.id}><span className={tuner.state}><RadioTower /></span><span><strong>{tuner.name}</strong><small>{tuner.state}{tuner.channelId ? ` · channel ${tuner.channelId}` : ''}</small></span></div>)}</div>}</section>
      <section><header><h3>Conflicts</h3><span>{status.conflicts.length}</span></header>{status.conflicts.length === 0 ? <div className="portico-status-empty compact healthy"><CheckCircle2 /><span><strong>No recording conflicts</strong><p>Current recording windows fit available tuners.</p></span></div> : <div className="portico-dvr-conflict-list">{status.conflicts.map((conflict) => <div key={conflict.id}><AlertTriangle /><span><strong>{requestError({ messageId: conflict.messageId, details: { capacity: conflict.capacity, demand: conflict.demand } }, 'dvr.conflict', { capacity: conflict.capacity, demand: conflict.demand })}</strong><small>{timeLabel(conflict.startsAt)}–{timeLabel(conflict.endsAt)} · {conflict.recordingIds.length} recordings</small></span></div>)}</div>}</section>
    </div>
  </SettingsGroup>;
}

export function LiveTVOperations({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const canManage = can(viewer, 'manageDVR', viewer.role !== 'user');
  const canView = can(viewer, 'viewDVR', canManage);
  return <div className="portico-settings-form"><SourceInventory dataSource={source} canManage={canManage} /><DVRStatusPanel dataSource={source} canView={canView} /></div>;
}
