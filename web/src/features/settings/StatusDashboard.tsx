import type { Job, PlaybackSession, RemoteAccessStatus } from '@porticomediaserver/client-core';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  CircleOff,
  Cpu,
  Gauge,
  Globe2,
  HardDrive,
  MemoryStick,
  Network,
  Play,
  RefreshCw,
  ShieldCheck,
  Square,
  Wifi,
} from '#portico-icons';
import { useMemo, useState } from 'react';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { Link } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { InlineNotice, SettingsError, SettingsLoading } from './SettingsControls';
import { useAbortableMutation, useSettingsQuery } from './settingsHooks';
import type { SettingsDataSource, SettingsStatusSnapshot, SettingsViewer } from './settingsTypes';

const loadStatus = (source: SettingsDataSource, signal: AbortSignal) => source.settingsStatus(signal);

function bytes(value: number | undefined): string {
  if (value === undefined || !Number.isFinite(value)) return 'Unavailable';
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let size = value;
  let index = -1;
  do {
    size /= 1024;
    index += 1;
  } while (size >= 1024 && index < units.length - 1);
  return `${size >= 100 ? size.toFixed(0) : size >= 10 ? size.toFixed(1) : size.toFixed(2)} ${units[index]}`;
}

function duration(value: number): string {
  const seconds = Math.max(0, Math.floor(value));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}` : `${minutes}:${String(remainder).padStart(2, '0')}`;
}

function relativeTime(value: string): string {
  const elapsed = Date.now() - Date.parse(value);
  if (!Number.isFinite(elapsed) || elapsed < 0) return 'just now';
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 1) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

type TelemetryStatus = { status?: string; reason?: string } | undefined;
type TelemetryState = 'ok' | 'unavailable' | 'warming_up' | 'unsupported';
type TelemetryPresentation = { state: TelemetryState; label: string; reason?: string; ok: boolean };
type ActivityTelemetry = NonNullable<SettingsStatusSnapshot['activity']> & { cpuStatus?: TelemetryStatus; memoryStatus?: TelemetryStatus };
type GPUTelemetrySample = NonNullable<NonNullable<NonNullable<SettingsStatusSnapshot['dashboard']>['system']>['gpu']>[number] & { usageStatus?: TelemetryStatus; memoryStatus?: TelemetryStatus; encoderStatus?: TelemetryStatus; headroomStatus?: TelemetryStatus };
type DashboardSystemTelemetry = NonNullable<NonNullable<SettingsStatusSnapshot['dashboard']>['system']> & { gpu?: GPUTelemetrySample[] };

function telemetryState(status: TelemetryStatus): TelemetryState {
  const value = status?.status?.trim().toLowerCase();
  if (value === 'ok') return 'ok';
  if (value === 'warming_up' || value === 'warming-up') return 'warming_up';
  if (value === 'unsupported') return 'unsupported';
  return 'unavailable';
}

function telemetryStateLabel(state: TelemetryState): string {
  if (state === 'ok') return 'OK';
  if (state === 'warming_up') return 'Warming up';
  if (state === 'unsupported') return 'Unsupported';
  return 'Unavailable';
}

function telemetryReason(status: TelemetryStatus, fallback: string): string {
  const reason = status?.reason?.trim();
  return reason || fallback;
}

function presentTelemetryMetric(value: number | undefined, status: TelemetryStatus, format: (value: number) => string, fallbackReason: string): TelemetryPresentation {
  const state = telemetryState(status);
  if (state === 'ok' && value !== undefined && Number.isFinite(value)) return { state, label: format(value), ok: true };
  const displayState = state === 'ok' ? 'unavailable' : state;
  return { state: displayState, label: telemetryStateLabel(displayState), reason: telemetryReason(status, fallbackReason), ok: false };
}

function TelemetryReadout({ metric, okDetail }: { metric: TelemetryPresentation; okDetail: string }) {
  const detail = metric.ok ? okDetail : metric.reason;
  return <><strong data-telemetry-status={metric.state}>{metric.label}</strong><em title={detail}>{detail}</em></>;
}

function TelemetryStatusRow({ label, metric, icon: Icon, testId }: { label: string; metric: TelemetryPresentation; icon: typeof Cpu; testId: string }) {
  return <div data-testid={testId}><Icon /><span><strong>{label}</strong><small title={metric.ok ? metric.label : metric.reason}>{metric.ok ? metric.label : metric.reason}</small></span><b className={metric.ok ? 'healthy' : ''} data-telemetry-status={metric.state}>{metric.ok ? telemetryStateLabel(metric.state) : metric.label}</b></div>;
}

function remoteHealth(remote: RemoteAccessStatus | undefined): { label: string; detail: string; healthy: boolean } {
  if (!remote) return { label: 'Remote status unavailable', detail: 'Portico could not read direct-route health.', healthy: false };
  if (!remote.settings.enabled) return { label: 'Remote access is off', detail: 'Connections are limited to local routes.', healthy: false };
  const reachable = remote.connectivity.troubleshootingStatus === 'ok';
  if (reachable) return { label: 'Direct remote access is available', detail: remote.publicEndpoint.url, healthy: true };
  return { label: 'Direct route needs attention', detail: remote.settings.lastHeartbeatError || remote.settings.routerMappingError || 'Run a direct-route check for current results.', healthy: false };
}

function uniqueJobs(snapshot: SettingsStatusSnapshot): Job[] {
  const values = [...(snapshot.dashboard?.jobs ?? []), ...(snapshot.jobs ?? [])];
  return [...new Map(values.map((job) => [job.id, job])).values()].sort((left, right) => Date.parse(right.updatedAt) - Date.parse(left.updatedAt));
}

function ResourceLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const activity = snapshot.activity as ActivityTelemetry | undefined;
  const cpu = presentTelemetryMetric(activity?.cpuPercent, activity?.cpuStatus, (value) => `${Math.round(value)}%`, 'No current CPU sample.');
  const memory = presentTelemetryMetric(activity?.memoryUsedBytes, activity?.memoryStatus, (value) => `${bytes(value)} / ${bytes(activity?.memoryTotalBytes)}`, 'No current memory sample.');
  const memoryDetail = memory.ok && activity ? `${Math.round((activity.memoryUsedBytes / Math.max(activity.memoryTotalBytes, 1)) * 100)}% in use` : memory.reason ?? 'Memory telemetry is unavailable.';
  return <section className="portico-resource-ledger" aria-label="Server resources">
    <div data-testid="server-resource-cpu"><Cpu /><span><small>CPU</small><TelemetryReadout metric={cpu} okDetail={activity ? `${activity.activeTranscodes} active ${activity.activeTranscodes === 1 ? 'transcode' : 'transcodes'}` : 'No current sample.'} /></span></div>
    <div data-testid="server-resource-memory"><MemoryStick /><span><small>Memory</small><TelemetryReadout metric={memory} okDetail={memoryDetail} /></span></div>
    <div><HardDrive /><span><small>Portico data</small><strong>{bytes(snapshot.storage?.totalBytes)}</strong><em>{snapshot.storage ? `${snapshot.storage.categories.length} managed categories` : 'Storage report unavailable'}</em></span></div>
    <div><Network /><span><small>Outbound</small><strong>{activity ? `${activity.bandwidthMbps.toFixed(1)} Mbps` : 'Unavailable'}</strong><em>{activity ? `${activity.activeStreams} active ${activity.activeStreams === 1 ? 'stream' : 'streams'}` : 'No current sample'}</em></span></div>
  </section>;
}

function GPUTelemetry({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const system = snapshot.dashboard?.system as DashboardSystemTelemetry | undefined;
  const sample = system?.gpu?.[system.gpu.length - 1];
  const info = system?.gpuInfo;
  const fallbackStatus: TelemetryStatus = sample ? undefined : system === undefined ? { status: 'unavailable', reason: 'GPU telemetry did not respond.' } : info?.available === true ? { status: 'warming_up', reason: info.note || 'Waiting for the first GPU telemetry sample.' } : info?.available === false ? { status: 'unsupported', reason: info.note || 'GPU telemetry is not supported on this server.' } : { status: 'unavailable', reason: 'GPU telemetry did not report its availability.' };
  const usage = presentTelemetryMetric(sample?.usage, sample?.usageStatus ?? fallbackStatus, (value) => `${Math.round(value)}%`, 'GPU usage is unavailable.');
  const memory = presentTelemetryMetric(sample?.memory, sample?.memoryStatus ?? fallbackStatus, (value) => `${Math.round(value)}%`, 'GPU memory telemetry is unavailable.');
  const encoder = presentTelemetryMetric(sample?.encoder, sample?.encoderStatus ?? fallbackStatus, (value) => `${Math.round(value)}%`, 'GPU encoder telemetry is unavailable.');
  const headroom = presentTelemetryMetric(sample?.headroom, sample?.headroomStatus ?? fallbackStatus, (value) => `${Math.round(value)}%`, 'GPU headroom telemetry is unavailable.');
  const label = sample?.label || info?.device || info?.provider || 'GPU telemetry';
  return <section className="portico-status-section" aria-label="GPU telemetry">
    <header><div><h2>GPU telemetry</h2><p>{label}</p></div></header>
    <div className="portico-health-list">
      <TelemetryStatusRow label="Usage" metric={usage} icon={Cpu} testId="gpu-telemetry-usage" />
      <TelemetryStatusRow label="Memory" metric={memory} icon={MemoryStick} testId="gpu-telemetry-memory" />
      <TelemetryStatusRow label="Encoder" metric={encoder} icon={Gauge} testId="gpu-telemetry-encoder" />
      <TelemetryStatusRow label="Headroom" metric={headroom} icon={Gauge} testId="gpu-telemetry-headroom" />
    </div>
  </section>;
}

function StreamRow({ stream, expanded, busy, onExpand, onStop }: { stream: PlaybackSession; expanded: boolean; busy: boolean; onExpand: () => void; onStop: () => Promise<void> }) {
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState('');
  const runtime = stream.media.durationSeconds ?? 0;
  const artwork = stream.media.images?.thumb || stream.media.images?.backdrop || stream.media.images?.poster;
  const stop = async () => {
    setError('');
    try {
      await onStop();
      setConfirming(false);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'stop this stream' }));
    }
  };
  return <div className="portico-stream-entry">
    <button type="button" className={`portico-stream-row ${expanded ? 'active' : ''}`} aria-expanded={expanded} onClick={onExpand}>
      <span className="portico-stream-title"><span className="portico-stream-art">{artwork ? <img src={artwork} alt="" /> : <Play />}</span><span><strong>{stream.media.title}</strong><small>{duration(stream.positionSeconds)}{runtime > 0 ? ` / ${duration(runtime)}` : ''} · {stream.state}</small></span></span>
      <span><strong>{stream.user}</strong><small>{stream.device} · {stream.app}</small></span>
      <span><strong>{stream.decision || stream.transcode?.method || 'Playback'}</strong><small>{stream.videoTarget || stream.transcode?.quality || stream.videoDecision || 'Source quality'}</small></span>
      <span>{stream.bandwidthMbps.toFixed(1)} Mbps</span><ChevronDown />
    </button>
    {expanded && <div className="portico-stream-disclosure">
      <dl><div><dt>Connection</dt><dd>{stream.location} · {stream.clientIp || 'Address unavailable'}</dd></div><div><dt>Video</dt><dd>{[stream.videoSource, stream.videoDecision, stream.videoTarget].filter(Boolean).join(' → ') || 'Not reported'}</dd></div><div><dt>Audio</dt><dd>{[stream.audioSource, stream.audioDecision, stream.audioTarget].filter(Boolean).join(' → ') || 'Not reported'}</dd></div><div><dt>Started</dt><dd>{relativeTime(stream.startedAt)}</dd></div></dl>
      {error && <p className="portico-stream-error" role="alert">{error}</p>}
      <div>{confirming ? <><span>Stop playback on {stream.device}?</span><SecondaryButton disabled={busy} onClick={() => setConfirming(false)}>Cancel</SecondaryButton><button type="button" className="button secondary portico-destructive-button" disabled={busy} onClick={() => void stop()}><Square />{busy ? 'Stopping…' : 'Stop stream'}</button></> : <button type="button" className="button secondary portico-destructive-button" onClick={() => setConfirming(true)}><Square /> Stop stream</button>}</div>
    </div>}
  </div>;
}

function ActiveStreams({ snapshot, source, onChanged }: { snapshot: SettingsStatusSnapshot; source: SettingsDataSource; onChanged: () => void }) {
  const streams = snapshot.dashboard?.nowPlaying ?? [];
  const [expanded, setExpanded] = useState<string>(streams[0]?.id ?? '');
  const { busy, run } = useAbortableMutation();
  return <section className="portico-status-section">
    <header><div><h2>Now playing</h2><p>{streams.length} active {streams.length === 1 ? 'stream' : 'streams'} · {snapshot.activity?.activeTranscodes ?? snapshot.dashboard?.transcodes.length ?? 0} transcoding</p></div></header>
    {streams.length === 0 ? <div className="portico-status-empty"><CircleOff /><span><strong>No active streams</strong><p>Playback sessions will appear here while they are active.</p></span></div> : <div className="portico-stream-table"><div className="portico-stream-table-head"><span>Title</span><span>Member and device</span><span>Playback</span><span>Bandwidth</span><span /></div>{streams.map((stream) => <StreamRow key={stream.id} stream={stream} expanded={expanded === stream.id} busy={busy} onExpand={() => setExpanded((current) => current === stream.id ? '' : stream.id)} onStop={async () => { await run((signal) => source.stopPlayback(stream.id, signal)); onChanged(); }} />)}</div>}
  </section>;
}

function ConnectivityLedger({ remote }: { remote: RemoteAccessStatus | undefined }) {
  const settings = remote?.settings;
  const route = remote?.publicEndpoint.url;
  const routeHealthy = remote?.connectivity.troubleshootingStatus === 'ok';
  const hostedHealthy = remote?.connectivity.hostedServicesStatus === 'reachable';
  const certificateHealthy = settings?.certificateStatus === 'valid' || settings?.certificateStatus === 'ready';
  const mappingHealthy = settings?.routerMappingStatus === 'mapped' || settings?.routerMappingStatus === 'active';
  return <section className="portico-status-section">
    <header><div><h2>Connectivity</h2><p>Direct access</p></div></header>
    <div className="portico-health-list">
      <div><Globe2 /><span><strong>Public route</strong><small>{route || remote?.connectivity.troubleshootingHint || 'No public route is active'}</small></span><b className={routeHealthy ? 'healthy' : ''}>{routeHealthy ? 'Reachable' : remote?.connectivity.publicRouteStatus || 'Unavailable'}</b></div>
      <div><ShieldCheck /><span><strong>TLS certificate</strong><small>{settings?.certificateRenewalError || (settings?.certificateExpiresAt ? `Expires ${new Date(settings.certificateExpiresAt).toLocaleDateString()}` : 'No certificate expiry reported')}</small></span><b className={certificateHealthy ? 'healthy' : ''}>{settings?.certificateStatus || 'Unknown'}</b></div>
      <div><Wifi /><span><strong>Router mapping</strong><small>{settings?.routerMappingError || (settings ? `${settings.publicPortMode} · port ${settings.manualPublicPort || remote?.publicEndpoint.port || 'automatic'}` : 'Remote status unavailable')}</small></span><b className={mappingHealthy ? 'healthy' : ''}>{settings?.routerMappingStatus || 'Unknown'}</b></div>
      <div><Gauge /><span><strong>Hosted control plane</strong><small>{settings?.lastHeartbeatAt ? `Last contact ${relativeTime(settings.lastHeartbeatAt)}` : 'No recent hosted heartbeat'}</small></span><b className={hostedHealthy ? 'healthy' : ''}>{hostedHealthy ? 'Connected' : remote?.connectivity.hostedServicesStatus || 'Not connected'}</b></div>
    </div>
    <Link className="portico-settings-inline-link" to="/settings/connectivity">Open connectivity settings</Link>
  </section>;
}

function WorkLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const jobs = useMemo(() => uniqueJobs(snapshot).slice(0, 6), [snapshot]);
  return <section className="portico-status-section"><header><div><h2>Work</h2><p>Scheduled and recent</p></div></header>
    {jobs.length === 0 ? <div className="portico-status-empty compact"><Activity /><span><strong>No recent server work</strong><p>Scans, backups, and maintenance jobs will appear here.</p></span></div> : <div className="portico-work-list">{jobs.map((job) => <div key={job.id}><span className={`portico-job-state ${job.status}`}><Activity /></span><span><strong>{job.message || job.type.replaceAll('_', ' ')}</strong><small>{job.status} · {Math.round(job.progress)}% · {relativeTime(job.updatedAt)}</small></span></div>)}</div>}
  </section>;
}

function AlertsLedger({ snapshot }: { snapshot: SettingsStatusSnapshot }) {
  const alerts = snapshot.dashboard?.alerts ?? [];
  return <section className="portico-status-section"><header><div><h2>Needs attention</h2><p>{alerts.length ? `${alerts.length} ${alerts.length === 1 ? 'alert' : 'alerts'}` : 'No active alerts'}</p></div></header>
    {alerts.length > 0 ? <div className="portico-alert-list">{alerts.map((alert) => <div className={alert.level} key={alert.id}><AlertTriangle /><span><strong>{alert.title}</strong><small>{alert.message} · {relativeTime(alert.time)}</small></span></div>)}</div> : <div className="portico-status-empty compact healthy"><CheckCircle2 /><span><strong>No action is required</strong><p>Portico has not reported any active server alerts.</p></span></div>}
  </section>;
}

export function StatusDashboard({ source, viewer }: { source: SettingsDataSource; viewer: SettingsViewer }) {
  const [revision, setRevision] = useState(0);
  const [checkError, setCheckError] = useState('');
  const state = useSettingsQuery(loadStatus, source, revision);
  const checks = useAbortableMutation();
  if (state.status === 'loading') return <SettingsLoading label="Loading server status" />;
  if (state.status === 'error') return <SettingsError title="Server status is unavailable" message={reviewedProductErrorText(state.error, 'settings.load-failed', { sectionName: 'Server status' })} onRetry={() => setRevision((current) => current + 1)} />;
  const snapshot = state.data;
  const health = remoteHealth(snapshot.remoteAccess);
  return <div className="portico-status-dashboard">
    <div className={`portico-status-command ${health.healthy ? 'healthy' : 'warn'}`}><div>{health.healthy ? <CheckCircle2 /> : <AlertTriangle />}<span><strong>{snapshot.activity?.serverName || viewer.serverName} is online</strong><small>{health.label} · {health.detail}</small></span></div>{viewer.role !== 'user' && <SecondaryButton disabled={checks.busy} onClick={() => { setCheckError(''); void checks.run((signal) => source.runConnectivityCheck(signal)).then(() => setRevision((current) => current + 1), (reason) => setCheckError(reviewedProductErrorText(reason, 'settings.action-failed', { actionName: 'complete the connectivity check' }))); }}><RefreshCw className={checks.busy ? 'portico-settings-spinner' : ''} />{checks.busy ? 'Checking…' : 'Run checks'}</SecondaryButton>}</div>
    {checkError && <InlineNotice tone="error">{checkError}</InlineNotice>}
    {Object.keys(snapshot.failures ?? {}).length > 0 && <InlineNotice tone="warn">Some server status sources could not be refreshed. Available sections are shown with their most recent data.</InlineNotice>}
    <ResourceLedger snapshot={snapshot} />
    <GPUTelemetry snapshot={snapshot} />
    <ActiveStreams snapshot={snapshot} source={source} onChanged={() => setRevision((current) => current + 1)} />
    <div className="portico-status-columns"><ConnectivityLedger remote={snapshot.remoteAccess} /><WorkLedger snapshot={snapshot} /></div>
    <AlertsLedger snapshot={snapshot} />
  </div>;
}
