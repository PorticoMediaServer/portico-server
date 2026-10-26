import type {DashboardStatus} from '@core/admin/dashboard.ts';
import type {DashboardSessions} from './bindings';

type Quiet = 'quiet' | 'not-quiet' | 'unknown';
export type DashboardQuietInput = Readonly<{
  status?: DashboardStatus; statusLoading: boolean; statusFailed: boolean;
  sessions?: DashboardSessions; sessionsLoading: boolean; sessionsFailed: boolean;
}>;
export type DashboardQuietState = Readonly<{alerts: Quiet; tasks: Quiet; streams: Quiet; grouped: boolean}>;

/** Only the original ready, unfiltered documents can establish an empty section. */
export function desktopDashboardQuietState(input: DashboardQuietInput): DashboardQuietState {
  const status = !input.statusLoading && !input.statusFailed ? input.status : undefined;
  const sessions = !input.sessionsLoading && !input.sessionsFailed ? input.sessions : undefined;
  const alerts: Quiet = !status ? 'unknown' : status.alerts.length === 0 ? 'quiet' : 'not-quiet';
  const tasks: Quiet = !status ? 'unknown' : status.tasks.items.length === 0 && status.tasks.count === 0 && status.scans.length === 0 ? 'quiet' : 'not-quiet';
  const streams: Quiet = !sessions ? 'unknown' : sessions.totals.streams === 0 && sessions.items.length === 0 ? 'quiet' : 'not-quiet';
  return {alerts, tasks, streams, grouped: alerts === 'quiet' && tasks === 'quiet' && streams === 'quiet'};
}
