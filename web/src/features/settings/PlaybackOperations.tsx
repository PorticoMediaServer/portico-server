import type { SystemStorageReport, TranscodeCapacityReport } from '@porticomediaserver/client-core';
import { AlertTriangle, CheckCircle2, Cpu, Gauge, HardDrive, RefreshCw, ServerCog } from '#portico-icons';
import { useCallback, useState } from 'react';
import { SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { InlineNotice, SettingsError, SettingsGroup, SettingsLoading } from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource, SettingsViewer } from './settingsTypes';

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

function RuntimeCapacity({ source }: { source: SettingsDataSource }) {
  const [revision, setRevision] = useState(0);
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.transcodeCapacity(signal), []);
  const query = useSettingsQuery<TranscodeCapacityReport>(load, source, revision);
  if (query.status === 'loading') return <SettingsGroup title="Playback runtime" description="Transcode capacity, hardware support, and required runtime dependencies."><SettingsLoading label="Loading playback runtime" /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title="Playback runtime" description="Transcode capacity, hardware support, and required runtime dependencies."><SettingsError title="Playback runtime is unavailable" message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: 'Playback runtime' })} onRetry={() => setRevision((current) => current + 1)} /></SettingsGroup>;
  const capacity = query.data;
  return <SettingsGroup title="Playback runtime" description="Transcode capacity, hardware support, and required runtime dependencies." actions={<SecondaryButton onClick={() => setRevision((current) => current + 1)}><RefreshCw /> Refresh</SecondaryButton>}>
    {capacity.warnings.length > 0 && <InlineNotice tone="warn">{capacity.warnings.join(' ')}</InlineNotice>}
    <div className="portico-playback-ledger">
      <div><Gauge /><span><small>Sessions</small><strong>{capacity.activeSessions} active · {capacity.availableSlots} available</strong><em>{capacity.maxConcurrentSessions} maximum</em></span></div>
      <div><Cpu /><span><small>Hardware</small><strong>{capacity.hardwareSupportLevel.replace('_', ' ')}</strong><em>{capacity.hardwareEncoderAvailable ? capacity.hardwareEncoder || 'Encoder available' : 'Software encoding'}</em></span></div>
      <div><ServerCog /><span><small>HDR tone mapping</small><strong>{capacity.hdrToneMappingStatus}</strong><em>{capacity.hdrToneMappingDetail}</em></span></div>
      <div><HardDrive /><span><small>Temporary storage</small><strong>{capacity.temporaryDirectoryReady ? 'Ready' : 'Unavailable'}</strong><em>{capacity.temporaryDirectory}</em></span></div>
    </div>
    <div className="portico-runtime-dependencies">
      {[capacity.ffmpeg, capacity.ffprobe].map((dependency) => <div key={dependency.name}><span className={dependency.available ? 'healthy' : 'danger'}>{dependency.available ? <CheckCircle2 /> : <AlertTriangle />}</span><span><strong>{dependency.name}</strong><small>{dependency.available ? dependency.versionLine || dependency.resolvedPath || 'Available' : dependency.error || `${dependency.configuredPath} was not found`}</small></span></div>)}
    </div>
  </SettingsGroup>;
}

function OptimizedStorage({ source }: { source: SettingsDataSource }) {
  const [revision, setRevision] = useState(0);
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const mutation = useAbortableMutation();
  const load = useCallback((next: SettingsDataSource, signal: AbortSignal) => next.systemStorage(signal), []);
  const query = useSettingsQuery<SystemStorageReport>(load, source, revision);
  const cleanup = async () => {
    setFeedback(''); setError('');
    try {
      const result = await mutation.run((signal) => source.cleanupStorage(signal));
      const removed = Object.values(result.removed).reduce((total, count) => total + count, 0);
      setFeedback(removed > 0 ? `${removed} managed files removed using current retention policy.` : 'Cleanup completed; no managed files were eligible.');
      setRevision((current) => current + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'run managed storage cleanup' }));
    }
  };
  if (query.status === 'loading') return <SettingsGroup title="Optimized version storage" description="Server-wide usage for generated compatible copies."><SettingsLoading label="Loading optimized storage" /></SettingsGroup>;
  if (query.status === 'error') return <SettingsGroup title="Optimized version storage" description="Server-wide usage for generated compatible copies."><SettingsError title="Optimized storage is unavailable" message={reviewedProductErrorText(query.error, 'settings.load-failed', { sectionName: 'Optimized storage' })} onRetry={() => setRevision((current) => current + 1)} /></SettingsGroup>;
  const optimized = query.data.categories.find((category) => category.key === 'optimized');
  if (!optimized) return <SettingsGroup title="Optimized version storage" description="Server-wide usage for generated compatible copies."><div className="portico-settings-state"><HardDrive /><strong>No optimized storage category</strong><p>This server did not report a managed optimized-version path.</p></div></SettingsGroup>;
  return <SettingsGroup title="Optimized version storage" description="Server-wide usage for generated compatible copies." actions={optimized.cleanupSupported ? <SecondaryButton disabled={mutation.busy} onClick={() => void cleanup()}><RefreshCw className={mutation.busy ? 'portico-settings-spinner' : ''} /> {mutation.busy ? 'Cleaning…' : 'Run managed cleanup'}</SecondaryButton> : undefined}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    <div className="portico-optimized-storage-row">
      <HardDrive />
      <span><strong>{optimized.label}</strong><small>{optimized.available ? `${bytes(optimized.sizeBytes)} · ${optimized.fileCount} ${optimized.fileCount === 1 ? 'file' : 'files'}` : optimized.error || 'Unavailable'}</small></span>
      <dl><div><dt>Path state</dt><dd>{optimized.writable ? 'Writable' : 'Read only'}</dd></div><div><dt>Cleanup</dt><dd>{optimized.cleanupSupported ? 'Supported' : 'Unavailable'}</dd></div><div><dt>Managed total</dt><dd>{bytes(query.data.totalBytes)}</dd></div></dl>
    </div>
    {optimized.cleanupSupported && <p className="portico-operations-footnote">Managed cleanup applies the server’s configured retention policy across every cleanup-supported cache.</p>}
  </SettingsGroup>;
}

export function PlaybackOperations({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const canManage = viewer.permissions?.manageServer ?? viewer.role !== 'user';
  if (!canManage) return <SettingsGroup title="Playback operations" description="Runtime capacity and managed optimized storage."><div className="portico-settings-readonly-note"><ServerCog />Your account can view playback preferences but cannot inspect server runtime details.</div></SettingsGroup>;
  return <div className="portico-settings-form"><RuntimeCapacity source={source} /><OptimizedStorage source={source} /></div>;
}
