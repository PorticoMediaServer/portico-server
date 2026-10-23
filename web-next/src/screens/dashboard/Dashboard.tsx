import {useCallback, useEffect, useId, useRef, useState, type CSSProperties} from 'react';
import {jobName} from '@core/server-admin/console-catalogue.ts';
import type {MessageId} from '@i18n';
import {DASHBOARD_LOAD_RANGES, DASHBOARD_LOAD_SERIES, type DashboardStatus} from '@core/admin/dashboard.ts';
import type {TelemetryReading, TelemetryWindow} from '@core/telemetry.ts';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import {Icon} from '../../kit/Icon';
import type {DashboardAlert, DashboardBindings} from './bindings';
import {fmtMbps, fmtPct} from './format';
import {orderedSessions} from './model';
import {useDashboardAuthority, useDashboardRead, type ReadState} from './use-read';
import {Range, ReadFeedback, Section, t} from './shared';
import {Chart, type ChartSeries} from './Chart';
import {SessionCard} from './SessionCard';
import {Analytics} from './Analytics';
import styles from './dashboard.module.css';

export type DashboardScreenProps = Readonly<{bindings: DashboardBindings; embedded?: boolean; initialSection?: 'streams' | 'activity' | 'updates'}>;
export function DashboardScreen({bindings: b, embedded = false, initialSection}: DashboardScreenProps) {
  const current = useDashboardAuthority(b);
  const ref = useRef<HTMLDivElement>(null), allowed = current && b.isMainProfile() && (b.role === 'owner' || b.role === 'admin');
  const statusLoad = useCallback((signal: AbortSignal) => b.read.status(signal), [b]);
  const status = useDashboardRead(b, statusLoad, allowed && b.role === 'owner', 10000);
  useEffect(() => {if (initialSection) ref.current?.querySelector(`[data-dashboard-section="${initialSection}"]`)?.scrollIntoView({block: 'start'});}, [initialSection]);
  if (!allowed) return <p role="status">{t('next.dashboard.restricted')}</p>;
  return <div className={styles.dashboard} ref={ref}>{!embedded && <h2 className={styles.pageTitle}>{t('next.dashboard.title')}</h2>}
    {b.role === 'owner' && <Status bindings={b} status={status} />}
    <div data-dashboard-section="streams"><NowPlaying bindings={b} /></div>
    {b.role === 'owner' && <><Load bindings={b} /><div data-dashboard-section="activity"><Analytics bindings={b} /></div><Tasks status={status} /></>}
  </div>;
}
function NowPlaying({bindings: b}: {bindings: DashboardBindings}) {
  const [cursor, setCursor] = useState(''), [expanded, setExpanded] = useState(false);
  const stack = useRef<HTMLDivElement>(null), removed = useRef<{id: string; index: number} | undefined>(undefined);
  const load = useCallback((signal: AbortSignal) => b.read.sessions(cursor, signal), [b, cursor]);
  const state = useDashboardRead(b, load, true, 10000), sessions = orderedSessions(state.value?.items ?? []), totals = state.value?.totals;
  useEffect(() => {
    const target = removed.current;
    if (!target || state.loading || state.failed || !state.value || state.value.items.some(s => s.id === target.id)) return;
    const buttons = stack.current?.querySelectorAll<HTMLButtonElement>('button[data-session-menu]');
    (buttons?.[Math.min(target.index, buttons.length - 1)] ?? stack.current)?.focus({preventScroll: true}); removed.current = undefined;
  }, [state.value, state.loading, state.failed]);
  return <Section title={t('next.dashboard.now')} extra={totals && <><span>{t('next.dashboard.streams', {count: totals.streams})}</span><small>{t('next.dashboard.locations', {home: totals.atHome, away: totals.away})}</small></>}>
    <ReadFeedback state={state} />{state.value && <div className={styles.sessionStack} ref={stack} tabIndex={-1} aria-label={t('next.dashboard.now')}>{(expanded ? sessions : sessions.slice(0, 8)).map((s, index) => <SessionCard key={s.id} session={s} bindings={b} refresh={id => {if (id) removed.current = {id, index}; state.refresh();}} />)}
      {!sessions.length && <p className={styles.empty}>{t('next.dashboard.nobody')}</p>}
      {sessions.length > 8 && <Button size="xs" variant="text" onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>{expanded ? t('next.dashboard.fewer') : t('next.dashboard.showAll', {count: totals?.streams ?? sessions.length})}</Button>}
      {expanded && state.value.nextCursor && <Button size="xs" onClick={() => setCursor(state.value!.nextCursor!)}>{t('next.dashboard.more')}</Button>}
      {cursor && <Button size="xs" onClick={() => setCursor('')}>{t('next.dashboard.now')}</Button>}
    </div>}
  </Section>;
}
const seriesNames: Record<string, MessageId> = {awayMbps: 'next.dashboard.away', homeMbps: 'next.dashboard.home', processCPU: 'next.dashboard.portico', cpu: 'next.dashboard.system', processMemory: 'next.dashboard.portico', memory: 'next.dashboard.system', hardwareConversions: 'next.dashboard.hardware', softwareConversions: 'next.dashboard.software'};
type LoadKind = keyof typeof DASHBOARD_LOAD_SERIES;
const loadKinds = Object.keys(DASHBOARD_LOAD_SERIES) as LoadKind[];
function loadSeries(reading: TelemetryReading | undefined, kind: LoadKind): ChartSeries[] {
  if (!reading) return [];
  return DASHBOARD_LOAD_SERIES[kind].metrics.filter(metric => reading.status[metric]?.status !== 'unavailable' && reading.series[metric]?.length).map(metric => ({key: metric, label: t(seriesNames[metric]), points: reading.series[metric]}));
}
function Load({bindings: b}: {bindings: DashboardBindings}) {
  const [kind, setKind] = useState<LoadKind>('bandwidth'), [range, setRange] = useState<TelemetryWindow>('live'), id = useId();
  const liveLoad = useCallback((signal: AbortSignal) => b.read.telemetry('live', signal), [b]);
  const historicalLoad = useCallback((signal: AbortSignal) => b.read.telemetry(range, signal), [b, range]);
  const live = useDashboardRead(b, liveLoad, true, 2000), historical = useDashboardRead(b, historicalLoad, range !== 'live', 60000);
  useEffect(() => {if (live.value && !loadSeries(live.value, kind).length) {const available = loadKinds.find(k => loadSeries(live.value, k).length); if (available) setKind(available);}}, [live.value, kind]);
  const detail = range === 'live' ? live : historical, series = loadSeries(detail.value, kind);
  const format = (value: number, selected = kind) => selected === 'bandwidth' ? fmtMbps(value) : selected === 'conversions' ? t(range === 'live' ? 'next.dashboard.running' : 'next.dashboard.averageCount', {value: range === 'live' ? String(Math.round(value)) : value.toFixed(1)}) : fmtPct(value);
  return <Section title={t('next.dashboard.load')}>
    <ReadFeedback state={live} /><div className={styles.tiles} role="tablist" aria-label={t('next.dashboard.load')}>{loadKinds.map(k => {
      const available = loadSeries(live.value, k); if (!available.length) return null;
      const latest = available.map(s => s.points.at(-1)!.v), current = k === 'bandwidth' || k === 'conversions' ? latest.reduce((a, b) => a + b, 0) : latest[0];
      return <button type="button" className={styles.tile} key={k} id={`${id}-${k}`} role="tab" aria-selected={kind === k} aria-controls={`${id}-panel`} tabIndex={kind === k ? 0 : -1} onClick={() => setKind(k)} onKeyDown={event => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return; event.preventDefault();
        const keys = loadKinds.filter(key => loadSeries(live.value, key).length), next = keys[(keys.indexOf(k) + (event.key === 'ArrowLeft' ? keys.length - 1 : 1)) % keys.length]; setKind(next); document.getElementById(`${id}-${next}`)?.focus();
      }}><span>{t(`next.dashboard.${k}`)}</span><strong>{k === 'conversions' ? String(Math.round(current)) : current.toFixed(1)}<small>{t(k === 'bandwidth' ? 'next.dashboard.unit.mbps' : k === 'conversions' ? 'next.dashboard.unit.running' : 'next.dashboard.unit.percent')}</small></strong><small>{available.map(s => <span key={s.key} data-series={s.key}><i />{k === 'bandwidth' ? `${s.label} ${s.points.at(-1)!.v.toFixed(1)}` : k === 'conversions' ? t(s.key === 'hardwareConversions' ? 'next.dashboard.hardwareCount' : 'next.dashboard.processorCount', {value: Math.round(s.points.at(-1)!.v)}) : s.key === 'cpu' || s.key === 'memory' ? t('next.dashboard.systemCurrent', {value: s.points.at(-1)!.v.toFixed(1)}) : t('next.dashboard.portico')}</span>)}</small><Chart kind={k} title={t(`next.dashboard.${k}`)} series={available} format={n => format(n, k)} step={k === 'conversions'} compact /></button>;
    })}</div>
    <div className={`${styles.card} ${styles.loadDetail}`} style={{'--load-index': loadKinds.indexOf(kind)} as CSSProperties} role="tabpanel" id={`${id}-panel`} aria-labelledby={`${id}-${kind}`}><div className={styles.heading}><h4>{t(`next.dashboard.${kind}`)}</h4><div className={styles.filters}><Range value={range} values={DASHBOARD_LOAD_RANGES} onChange={value => setRange(value as TelemetryWindow)} /></div></div>
      <ReadFeedback state={detail} /><Chart kind={kind} title={t(`next.dashboard.${kind}`)} series={series} format={format} formatAverage={value => kind === 'conversions' ? value.toFixed(1) : format(value)} step={kind === 'conversions' && range === 'live'} />
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
  const [confirm, setConfirm] = useState<DashboardAlert | undefined>(), [busy, setBusy] = useState(false), [updating, setUpdating] = useState(false);
  const run = async (alert: DashboardAlert) => {if (!b.effects.alert || busy) return; setBusy(true); try {await b.effects.alert(alert); if (b.isCurrent()) {setConfirm(undefined); if (alert.action.kind === 'update') setUpdating(true); status.refresh();}} catch (error) {b.effects.reportError(error);} finally {setBusy(false);}};
  return <>{(status.loading || status.failed || !!status.value?.alerts.length) && <div data-dashboard-section="updates"><ReadFeedback state={status} />{!!status.value?.alerts.length && <section className={styles.alerts} aria-label={t('next.dashboard.alerts')}>{status.value.alerts.map((alert, index) => {
    const update = alert.action.kind === 'update', destination = alertDestinations[alert.action.kind] ?? {section: 'troubleshooting', label: 'next.dashboard.troubleshooting' as const}, text = update && updating ? t('next.dashboard.updating') : t(alertCopy[alert.code] ?? 'next.dashboard.serverAlert', {version: alert.params.latest ?? '', count: alert.count ?? 0});
    return <div className={styles.alert} key={`${alert.code}-${index}`} data-major={alert.major || undefined}><span className={styles.alertIcon} data-info={update || undefined}><Icon name={update && updating ? 'i-refresh' : update ? 'i-arrow-up' : 'i-alert'} size={18} /></span><b>{text}</b>{alert.params.reason && <span>{alert.params.reason}</span>}{alert.params.subject && <span>{alert.params.subject}</span>}
      {b.effects.alert && (update || alert.code === 'secure_renewal_failed') && !(update && updating) && <Button size="xs" variant={update ? 'primary' : 'glass'} disabled={busy} onClick={() => update ? setConfirm(alert) : void run(alert)}>{update ? t('next.dashboard.update') : t('next.dashboard.retry')}</Button>}
      {!update && <Button size="xs" onClick={() => b.effects.openSettings(destination.section)}>{t(destination.label)}<Icon name="i-right" size={16} /></Button>}
    </div>;
  })}</section>}</div>}

    <Dialog open={!!confirm} onOpenChange={open => {if (!open && !busy) setConfirm(undefined);}} title={t('next.dashboard.updateTitle')} description={t('next.dashboard.updateDetail')} footer={<><Button size="compact" onClick={() => setConfirm(undefined)} disabled={busy}>{t('next.dashboard.cancel')}</Button><Button size="compact" variant="primary" loading={busy} onClick={() => {if (confirm) void run(confirm);}}>{t('next.dashboard.update')}</Button></>}>{null}</Dialog>
  </>;
}

function Tasks({status}: {status: StatusState}) {
  const tasks = status.value?.tasks.items.filter(task => task.state === 'running') ?? [];
  return <div className={styles.tasks}><Section title={t('next.dashboard.tasks')}><div className={styles.card}>{status.value?.scans.map(scan => <div className={styles.task} key={scan.libraryId}><div><b>{t('next.dashboard.scanning', {library: scan.libraryName})}</b>{scan.total !== undefined && <small>{t('next.dashboard.taskProgress', {done: scan.analyzed, total: scan.total})}</small>}</div>{scan.total !== undefined && scan.total > 0 && <><progress aria-label={scan.libraryName} value={scan.analyzed} max={scan.total} /><span>{t('next.dashboard.percent', {value: Math.round(scan.analyzed / scan.total * 100)})}</span></>}</div>)}{tasks.map(task => <div className={styles.task} key={task.id}><div><b>{jobName(t, defaultI18n.has, task.kind)}{task.libraryName ? `: ${task.libraryName}` : ''}</b>{task.total !== undefined && task.processed !== undefined && <small>{t('next.dashboard.taskProgress', {done: task.processed, total: task.total})}</small>}</div>{task.total !== undefined && task.total > 0 && task.processed !== undefined && <><progress aria-label={task.libraryName || t('next.dashboard.tasks')} value={task.processed} max={task.total} /><span>{t('next.dashboard.percent', {value: Math.round(task.processed / task.total * 100)})}</span></>}</div>)}{status.value && !tasks.length && !status.value.scans.length && <p className={styles.empty}>{t('next.dashboard.noTasks')}</p>}</div></Section></div>;
}
