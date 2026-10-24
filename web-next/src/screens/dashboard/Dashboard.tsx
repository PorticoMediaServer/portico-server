import {useCallback, useEffect, useId, useRef, useState, type CSSProperties} from 'react';
import {jobName} from '@core/server-admin/console-catalogue.ts';
import type {MessageId} from '@i18n';
import {DASHBOARD_LOAD_RANGES, DASHBOARD_LOAD_SERIES, type DashboardStatus} from '@core/admin/dashboard.ts';
import type {TelemetryReading} from '@core/telemetry.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {ToastHost, type ToastData} from '../../kit/overlays';
import {Icon} from '../../kit/Icon';
import type {DashboardAlert, DashboardBindings, DashboardSessions} from './bindings';
import {fmtMbps, fmtPct} from './format';
import {loadTileValue, orderedSessions, sessionStopTitle} from './model';
import {useDashboardAccounts, useDashboardAuthority, useDashboardRead, type ReadState} from './use-read';
import {Range, ReadFeedback, Section, t} from './shared';
import {Chart, type ChartSeries} from './Chart';
import {SessionCard} from './SessionCard';
import {Analytics} from './Analytics';
import {useDashboardReport, type DashboardReportController} from './use-report';
import {readNowPlaying} from './pages';
import styles from './dashboard.module.css';

export type DashboardScreenProps = Readonly<{bindings: DashboardBindings; embedded?: boolean; initialSection?: 'streams' | 'activity' | 'updates'}>;
export function DashboardScreen(props: DashboardScreenProps) {
  const b = props.bindings;
  return <DashboardView key={JSON.stringify([b.scope.serverId, b.scope.viewerId, b.role])} {...props} />;
}
function DashboardView({bindings: b, embedded = false, initialSection}: DashboardScreenProps) {
  const current = useDashboardAuthority(b);
  const ref = useRef<HTMLDivElement>(null), allowed = current && b.isMainProfile() && (b.role === 'owner' || b.role === 'admin');
  const statusLoad = useCallback((signal: AbortSignal) => b.read.status(signal), [b]);
  const status = useDashboardRead(b, statusLoad, allowed && b.role === 'owner', 10000);
  const sessionsLoad = useCallback((signal: AbortSignal) => readNowPlaying(b, signal), [b]);
  const sessions = useDashboardRead(b, sessionsLoad, allowed, 10000);
  const liveLoad = useCallback((signal: AbortSignal) => b.read.telemetry('live', signal), [b]);
  const live = useDashboardRead(b, liveLoad, allowed && b.role === 'owner', 2000);
  const report = useDashboardReport(b, allowed && b.role === 'owner');
  const accounts = useDashboardAccounts(b, allowed && b.role === 'owner');
  useEffect(() => {if (initialSection) ref.current?.querySelector(`[data-dashboard-section="${initialSection}"]`)?.scrollIntoView({block: 'start'});}, [initialSection]);
  if (!allowed) return null;
  const failed = sessions.failed || b.role === 'owner' && (status.failed || live.failed || report.state.failed || accounts.failed);
  const retry = () => {sessions.refresh(); if (b.role === 'owner') {status.refresh(); live.refresh(); report.state.refresh(); accounts.refresh();}};
  return <div className={styles.dashboard} ref={ref}>{!embedded && <h2 className={styles.pageTitle}>{t('next.dashboard.title')}</h2>}
    {failed ? <ReadFeedback openHome={b.effects.openHome} state={{failed: true, loading: false, refresh: retry}} /> : <>
      {b.role === 'owner' && <Status bindings={b} status={status} />}
      <div data-dashboard-section="streams"><NowPlaying bindings={b} state={sessions} /></div>
      {b.role === 'owner' && <><Load bindings={b} report={report} live={live} /><div data-dashboard-section="activity"><Analytics bindings={b} report={report} accounts={accounts} /></div><Tasks status={status} /></>}
    </>}
  </div>;
}
type SessionsState = ReadState<DashboardSessions> & {refresh: () => void};
type LiveState = ReadState<TelemetryReading> & {refresh: () => void};
function NowPlaying({bindings: b, state}: {bindings: DashboardBindings; state: SessionsState}) {
  const [toast, showToast] = useState<ToastData | null>(null);
  const stack = useRef<HTMLDivElement>(null), removed = useRef<{id: string; index: number} | undefined>(undefined);
  const sessions = orderedSessions(state.value?.items ?? []), totals = state.value?.totals;
  useEffect(() => {
    const target = removed.current;
    if (!target || state.loading || state.failed || !state.value || state.value.items.some(s => s.id === target.id)) return;
    const buttons = stack.current?.querySelectorAll<HTMLButtonElement>('button[data-session-menu]');
    (buttons?.[Math.min(target.index, buttons.length - 1)] ?? stack.current)?.focus({preventScroll: true}); removed.current = undefined;
  }, [state.value, state.loading, state.failed]);
  return <Section title={t('next.dashboard.now')} extra={totals && totals.streams > 0 && <><span>{t('next.dashboard.streams', {count: totals.streams})}</span><small>{t('next.dashboard.locations', {home: totals.atHome, away: totals.away})}</small></>}>
    <ReadFeedback openHome={b.effects.openHome} state={state} />{state.value && <div className={styles.sessionStack} ref={stack} tabIndex={-1} aria-label={t('next.dashboard.now')}>{sessions.map((s, index) => <SessionCard key={s.id} session={s} bindings={b} refresh={id => {if (id) {removed.current = {id, index}; showToast({id, message: t('next.dashboard.stopped', {title: sessionStopTitle(s)}), icon: 'i-check', durationMs: 3200});} state.refresh();}} />)}
      {!sessions.length && <p className={styles.empty}>{t('next.dashboard.nobody')}</p>}
    </div>}
    <ToastHost toast={toast} onDismiss={() => showToast(null)} />
  </Section>;
}
const seriesNames: Record<string, MessageId> = {awayMbps: 'next.dashboard.away', homeMbps: 'next.dashboard.home', processCPU: 'next.dashboard.portico', cpu: 'next.dashboard.system', processMemory: 'next.dashboard.portico', memory: 'next.dashboard.system', hardwareConversions: 'next.dashboard.hardware', softwareConversions: 'next.dashboard.software'};
type LoadKind = keyof typeof DASHBOARD_LOAD_SERIES;
const loadKinds = Object.keys(DASHBOARD_LOAD_SERIES) as LoadKind[];
function loadSeries(reading: TelemetryReading | undefined, kind: LoadKind): ChartSeries[] {
  if (!reading) return [];
  return DASHBOARD_LOAD_SERIES[kind].metrics.filter(metric => reading.status[metric]?.status !== 'unavailable' && reading.series[metric]?.length).map(metric => ({key: metric, label: t(seriesNames[metric]), points: reading.series[metric]}));
}
function Load({bindings: b, report, live}: {bindings: DashboardBindings; report: DashboardReportController; live: LiveState}) {
  const [kind, setKind] = useState<LoadKind>('bandwidth'), id = useId(), range = report.query.window;
  const historical = {...report.state, value: report.state.value?.load};
  useEffect(() => {if (live.value && !loadSeries(live.value, kind).length) {const available = loadKinds.find(k => loadSeries(live.value, k).length); if (available) setKind(available);}}, [live.value, kind]);
  const detail = range === 'live' ? live : historical, series = loadSeries(detail.value, kind);
  const format = (value: number, selected = kind) => selected === 'bandwidth' ? fmtMbps(value) : selected === 'conversions' ? t(range === 'live' ? 'next.dashboard.running' : 'next.dashboard.averageCount', {value: range === 'live' ? String(Math.round(value)) : value.toFixed(1)}) : fmtPct(value);
  return <Section title={t('next.dashboard.load')}>
    <ReadFeedback height={118} openHome={b.effects.openHome} state={live} /><div className={styles.tiles} role="tablist" aria-label={t('next.dashboard.load')}>{loadKinds.map(k => {
      const available = loadSeries(live.value, k); if (!available.length) return null;
      const current = loadTileValue(live.value!, k);
      return <button type="button" className={styles.tile} key={k} id={`${id}-${k}`} role="tab" aria-selected={kind === k} aria-controls={`${id}-panel`} tabIndex={kind === k ? 0 : -1} onClick={() => setKind(k)} onKeyDown={event => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return; event.preventDefault();
        const keys = loadKinds.filter(key => loadSeries(live.value, key).length), next = keys[(keys.indexOf(k) + (event.key === 'ArrowLeft' ? keys.length - 1 : 1)) % keys.length]; setKind(next); document.getElementById(`${id}-${next}`)?.focus();
      }}><span>{t(`next.dashboard.${k}`)}</span>{current !== undefined && <strong>{k === 'conversions' ? String(Math.round(current)) : current.toFixed(1)}<small>{t(k === 'bandwidth' ? 'next.dashboard.unit.mbps' : k === 'conversions' ? 'next.dashboard.unit.running' : 'next.dashboard.unit.percent')}</small></strong>}<small>{available.map(s => <span key={s.key} data-series={s.key}><i />{k === 'bandwidth' ? `${s.label} ${s.points.at(-1)!.v.toFixed(1)}` : k === 'conversions' ? t(s.key === 'hardwareConversions' ? 'next.dashboard.hardwareCount' : 'next.dashboard.processorCount', {value: Math.round(s.points.at(-1)!.v)}) : s.key === 'cpu' || s.key === 'memory' ? t('next.dashboard.systemCurrent', {value: s.points.at(-1)!.v.toFixed(1)}) : t('next.dashboard.portico')}</span>)}</small><Chart interval={live.value!} kind={k} title={t(`next.dashboard.${k}`)} series={available} format={n => format(n, k)} step={k === 'conversions'} compact /></button>;
    })}</div>
    <div className={`${styles.card} ${styles.loadDetail}`} style={{'--load-index': loadKinds.indexOf(kind)} as CSSProperties} role="tabpanel" id={`${id}-panel`} aria-labelledby={`${id}-${kind}`}><div className={styles.heading}><h4>{t(`next.dashboard.${kind}`)}</h4><div className={styles.filters}><Range load value={range} values={DASHBOARD_LOAD_RANGES} onChange={value => report.change({window: value as typeof range})} /></div></div>
      <ReadFeedback height={250} openHome={b.effects.openHome} state={detail} />{detail.value && <Chart interval={detail.value} kind={kind} title={t(`next.dashboard.${kind}`)} series={series} format={format} formatAverage={value => kind === 'conversions' ? value.toFixed(1) : format(value)} step={kind === 'conversions' && range === 'live'} />}
    </div>
  </Section>;
}
const alertCopy: Record<string, MessageId> = {update_available: 'next.dashboard.updateAvailable', upload_budget_exhausted: 'next.dashboard.uploadAlert', state_permissions: 'next.dashboard.permissionsAlert', storage_nearly_full: 'next.dashboard.storageAlert', backup_failed: 'next.dashboard.backupAlert', library_unreachable: 'next.dashboard.sourceAlert', remote_unreachable: 'next.dashboard.sourceAlert', secure_renewal_failed: 'next.dashboard.certificateAlert'};
const alertDestinations: Record<string, {section: string; label: MessageId}> = {
  streaming: {section: 'streaming', label: 'next.dashboard.streaming'},
  networking: {section: 'remote', label: 'next.dashboard.remote'},
  storage: {section: 'storage', label: 'next.dashboard.storage'},
  backups: {section: 'storage', label: 'next.dashboard.storage'},
  'library-source': {section: 'libraries', label: 'next.dashboard.libraries'},
  'remote-source': {section: 'remote-sources', label: 'next.dashboard.remoteSources'},
};
type StatusState = ReadState<DashboardStatus> & {refresh: () => void};
function Status({bindings: b, status}: {bindings: DashboardBindings; status: StatusState}) {
  const alerts = status.value?.alerts.filter(alert => alertCopy[alert.code] && (alert.code !== 'update_available' || !!alert.params.latest) && (alert.code !== 'upload_budget_exhausted' || alert.count !== undefined)) ?? [];
  const [busy, setBusy] = useState<string | undefined>();
  const retry = async (alert: DashboardAlert) => {if (!b.effects.retryCertificate || busy) return; setBusy(alert.code); try {await b.effects.retryCertificate(); if (b.isCurrent()) status.refresh();} catch (error) {b.effects.reportError(error);} finally {setBusy(undefined);}};
  return <>{(status.loading || status.failed || !!alerts.length) && <div data-dashboard-section="updates"><ReadFeedback height={56} openHome={b.effects.openHome} state={status} />{!!alerts.length && <section className={styles.alerts} aria-label={t('next.dashboard.alerts')}>{alerts.map((alert, index) => {
    const update = alert.action.kind === 'update', destination = alertDestinations[alert.action.kind] ?? {section: 'troubleshooting', label: 'next.dashboard.troubleshooting' as const}, text = t(alertCopy[alert.code], {version: alert.params.latest ?? '', count: alert.count ?? 0});
    return <div className={styles.alert} key={`${alert.code}-${index}`} data-major={alert.major || undefined}><span className={styles.alertIcon} data-info={update || undefined}><Icon name={update ? 'i-arrow-up' : 'i-alert'} size={18} /></span><b>{text}</b>{alert.params.reason && <span>{alert.params.reason}</span>}{alert.params.subject && <span>{alert.params.subject}</span>}
      {alert.code === 'secure_renewal_failed' && b.effects.retryCertificate && <Button size="xs" variant="glass" disabled={!!busy} onClick={() => void retry(alert)}>{t('next.dashboard.retry')}</Button>}
      {!update && <Button size="xs" onClick={() => b.effects.openSettings(destination.section)}>{t(destination.label)}<Icon name="i-right" size={16} /></Button>}
    </div>;
  })}</section>}</div>}
  </>;
}

function Tasks({status}: {status: StatusState}) {
  const tasks = status.value?.tasks.items.filter(task => task.state === 'running' && defaultI18n.has(`server.job.${task.kind}`)) ?? [];
  if (!tasks.length && !status.value?.scans.length) return null;
  return <div className={styles.tasks}><Section title={t('next.dashboard.tasks')}><div className={`${styles.card} ${styles.taskCard}`}>{status.value?.scans.map(scan => <div className={styles.task} key={scan.libraryId}><span className={styles.taskIcon}><Icon name="i-library" size={18} /></span><div><b>{t('next.dashboard.scanning', {library: scan.libraryName})}</b>{scan.total !== undefined && <small>{t('next.dashboard.taskProgressFiles', {done: scan.analyzed, total: scan.total})}</small>}</div>{scan.total !== undefined && scan.total > 0 && <><progress aria-label={scan.libraryName} value={scan.analyzed} max={scan.total} /><span>{t('next.dashboard.percent', {value: Math.round(scan.analyzed / scan.total * 100)})}</span></>}</div>)}{tasks.map(task => <div className={styles.task} key={task.id}><span className={styles.taskIcon}><Icon name={task.kind === 'trickplay' ? 'i-image' : 'i-server'} size={18} /></span><div><b>{task.kind === 'trickplay' && task.libraryName ? t('next.dashboard.makingPreviews', {library: task.libraryName}) : <>{jobName(t, defaultI18n.has, task.kind)}{task.libraryName ? `: ${task.libraryName}` : ''}</>}</b>{task.total !== undefined && task.processed !== undefined && <small>{t(task.unit === 'episodes' ? 'next.dashboard.taskProgressEpisodes' : task.unit === 'files' ? 'next.dashboard.taskProgressFiles' : 'next.dashboard.taskProgress', {done: task.processed, total: task.total})}</small>}</div>{task.total !== undefined && task.total > 0 && task.processed !== undefined && <><progress aria-label={task.libraryName || t('next.dashboard.tasks')} value={task.processed} max={task.total} /><span>{t('next.dashboard.percent', {value: Math.round(task.processed / task.total * 100)})}</span></>}</div>)}</div></Section></div>;
}
