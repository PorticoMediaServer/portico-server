/**
 * The Server pages' words for what the console reports (jobs, alerts, measurements): one set for
 * every client, so a job or an alert reads the same on the web and on a phone.
 */
import type {Alert, Job, Measurement} from '../console.ts';
import {formatBytes} from '../presentation/content.ts';

export const sentence = (v: string) => { const words = v.replace(/[_.-]+/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2').trim().toLowerCase(); return words.charAt(0).toUpperCase() + words.slice(1); };

export const jobLabels: Record<string, string> = {library_scan: 'Library scan', scan: 'Library scan', metadata_refresh: 'Metadata refresh', analysis: 'Media analysis', optimization: 'Prepare versions', backup: 'Backup', cleanup: 'Storage cleanup', guide_refresh: 'Guide refresh', recording: 'Recording', maintenance: 'Maintenance', prune: 'Retention pruning', subtitles: 'Subtitle work', trickplay: 'Preview thumbnails'};
export const jobLabel = (kind: string) => jobLabels[kind] ?? sentence(kind);
export const jobStateLabel: Record<Job['state'], string> = {queued: 'Queued', running: 'Running', paused: 'Paused', reconciling: 'Reconciling', 'cancellation-requested': 'Canceling', cancelled: 'Canceled', failed: 'Failed', succeeded: 'Finished'};
export const jobTone = (state: Job['state']): 'healthy' | 'warning' | 'danger' | 'neutral' | 'accent' => (state === 'running' ? 'accent' : state === 'succeeded' ? 'healthy' : state === 'failed' ? 'danger' : state === 'paused' || state === 'cancellation-requested' || state === 'reconciling' ? 'warning' : 'neutral');

export const alertLabels: Record<string, string> = {backup_failed: 'Backup failed', low_storage: 'Storage is running low', database_degraded: 'Database needs attention', job_failed_repeatedly: 'A task keeps failing', certificate_expiring: 'HTTPS certificate expires soon', remote_access_lost: 'Remote access is unavailable', recording_failed: 'A recording failed', update_required: 'Update needed', provider_invalid: 'A provider credential is no longer valid', prune_failed: 'Retention cleanup failed', 'upload-budget-exhausted': 'Remote streams are held at low quality by your upload speed', 'secure-connections-required-unavailable': 'Encrypted connections are required but unavailable'};
export const alertLabel = (a: Alert) => alertLabels[a.code] ?? sentence(a.code);
export const alertTone = (a: Alert): 'danger' | 'warning' | 'accent' => (a.severity === 'critical' ? 'danger' : a.severity === 'warning' ? 'warning' : 'accent');

export const factLabels: Record<string, string> = {uptime: 'Up for', processCPU: 'CPU (this process)', goHeap: 'Memory in use', goReserved: 'Memory reserved', goroutines: 'Concurrent tasks', activeRequests: 'Requests in flight', httpRequests: 'Requests served', httpBytesReceived: 'Data received', httpBytesSent: 'Data sent', databaseBytes: 'Database size', databaseWALBytes: 'Pending writes', readable: 'Database readable', readiness: 'Ready to serve', health: 'Health', stateVolumeAvailable: 'State volume', remoteAccess: 'Remote access', alerts: 'Alerts', openAlerts: 'Open alerts', capacityPolicy: 'Capacity policy', database: 'Database', storage: 'Storage', network: 'Network', requests: 'Requests', bytes: 'Bytes', seconds: 'Seconds', resources: 'Resources'};
export const factLabel = (key: string) => factLabels[key] ?? sentence(key);

export function formatFact(fact: Measurement['facts'][string]): string {
  if (fact.state && fact.state !== 'measured' && fact.state !== 'ok' && fact.state !== 'available') return fact.reason ? `${sentence(fact.state)} · ${fact.reason}` : sentence(fact.state);
  const v = fact.value;
  if (v === null || v === undefined) return fact.reason ?? 'Not available';
  if (typeof v === 'boolean') return v ? 'Yes' : 'No';
  if (typeof v === 'number') {
    if (fact.unit === 'bytes') return formatBytes(v);
    if (fact.unit === 'seconds') return duration(v);
    if (fact.unit === 'percent') return `${Math.round(v)}%`;
    if (fact.unit === 'ratio') return `${Math.round(v * 100)}%`;
    return v.toLocaleString() + (fact.unit ? ` ${fact.unit}` : '');
  }
  if (typeof v === 'string') return sentence(v);
  return JSON.stringify(v);
}
export function duration(seconds: number): string {
  const s = Math.round(seconds);
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${s}s`;
}
export function when(ms: number): string {
  if (!ms) return '—';
  const d = new Date(ms);
  const today = new Date();
  const sameDay = d.toDateString() === today.toDateString();
  return sameDay ? d.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'}) : d.toLocaleString([], {month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'});
}
