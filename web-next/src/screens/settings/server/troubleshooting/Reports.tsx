import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {clientReportDownloadPath, type ClientReport} from '@core/client-reports.ts';
import type {EvidenceRecord} from '@core/console.ts';
import type {CatalogueFailure} from '@core/freshness.ts';
import {sentence, when} from '@core/server-admin/console-words.ts';
import {Button} from '../../../../kit/controls';
import {Icon} from '../../../../kit/Icon';
import {Dialog} from '../../../../kit/overlays/Dialog';
import {Menu} from '../../../../kit/overlays/Popover';
import {SettingsAction} from '../../../../kit/settings';
import type {RecordLane, SettingsReportsSource} from './reports-source';
import styles from './troubleshooting.module.css';

const t = defaultI18n.t;

export type ReportsPanelProps = Readonly<{
  source: SettingsReportsSource;
  subscribe: (listener: () => void) => () => void;
  isCurrent: () => boolean;
  /** The selected server's own origin, so a downloaded report comes from the server that holds it. */
  downloadBase: string;
  reportError: (error: unknown) => void;
}>;

const LANES: readonly Readonly<{id: RecordLane; label: MessageId}>[] = Object.freeze([
  {id: 'runtime', label: 'web.logs.laneServer'},
  {id: 'client', label: 'web.logs.laneClient'},
  {id: 'audit', label: 'web.logs.laneAudit'},
]);
const LEVELS: Readonly<Record<string, MessageId>> = Object.freeze({info: 'next.settings.levelInfo', warn: 'next.settings.levelWarn', error: 'next.settings.levelError', debug: 'next.settings.levelDebug'});
const levelLabel = (severity: string | undefined) => t(LEVELS[severity ?? ''] ?? 'next.settings.levelInfo');
const senderIcon = (deviceClass: string): 'i-phone' | 'i-tv' | 'i-laptop' | 'i-monitor' =>
  /phone|tablet|mobile/i.test(deviceClass) ? 'i-phone' : /tv|console/i.test(deviceClass) ? 'i-tv' : /laptop/i.test(deviceClass) ? 'i-laptop' : 'i-monitor';
const sender = (report: ClientReport) => [report.platform, report.deviceClass, report.appVersion].filter(Boolean).join(' ') || t('server.reports.unknownSender');

/** The drawn Reports tab: client reports, the titles a rebuild set aside, and the server's records by lane. */
export function ReportsPanel(props: ReportsPanelProps) {
  const state = useSyncExternalStore(props.subscribe, props.source.getSnapshot, props.source.getSnapshot);
  const [reading, setReading] = useState<ClientReport>(), [confirm, setConfirm] = useState<ClientReport>();
  useEffect(() => {void props.source.load();}, [props.source]);
  if (state.phase === 'error') return <p className={styles.failure} role="alert"><span>{t('next.states.pageFailedBody')}</span><button type="button" onClick={() => {void props.source.retry();}}>{t('next.shell.tryAgain')}</button></p>;
  if (state.phase === 'idle' || state.phase === 'loading' && !state.reports.length) return <p role="status">{t('next.settings.loading')}</p>;
  return <div className={styles.reports}>
    <section className={styles.panel}>
      <div className={styles.panelHead}><h3>{t('web.diagnostics.clientReports')}</h3></div>
      <p className={styles.lede}>{t('server.reports.lede')}</p>
      {state.reports.length ? <ul className={styles.rows}>
        {state.reports.map(report => <ReportRow key={report.id} report={report} downloadBase={props.downloadBase} disabled={state.phase === 'loading'} onRead={() => setReading(report)} onDelete={() => setConfirm(report)} />)}
      </ul> : <p className={styles.none}>{t('server.reports.none')}</p>}
      {state.next && <div className={styles.pager}><Button variant="glass" size="small" onClick={() => {void props.source.more();}}>{t('server.showMore')}</Button></div>}
    </section>
    {state.setAside && state.setAside.failures.length > 0 && <SetAside failures={state.setAside.failures} more={state.setAside.more} onCheck={() => {void props.source.checkSetAside();}} />}
    {state.records && <Records records={state.records.items} lane={state.records.lane} next={state.records.next} onLane={lane => {void props.source.selectLane(lane);}}
      onOlder={() => {void props.source.moreRecords();}} onLatest={() => {void props.source.latestRecords();}} />}
    {reading && <ReadReport report={reading} source={props.source} downloadBase={props.downloadBase} onClose={() => setReading(undefined)} reportError={props.reportError} />}
    {confirm && <Dialog open onOpenChange={open => {if (!open) setConfirm(undefined);}} title={t('server.reports.deleteTitle')}
      footer={<><Button size="small" onClick={() => setConfirm(undefined)}>{t('next.settings.cancel')}</Button><SettingsAction size="small" appearance="danger"
        onClick={() => {const report = confirm;setConfirm(undefined);void props.source.remove(report.id).catch(props.reportError);}}>{t('web.diagnostics.clientReportDelete')}</SettingsAction></>}>
      <p>{t('server.reports.deleteBody')}</p>
    </Dialog>}
  </div>;
}

function ReportRow({report, downloadBase, disabled, onRead, onDelete}: Readonly<{report: ClientReport; downloadBase: string; disabled: boolean; onRead: () => void; onDelete: () => void}>) {
  const [open, setOpen] = useState(false), anchor = useRef<HTMLButtonElement>(null);
  const title = sender(report), whenText = defaultI18n.dateTime(report.receivedAt), size = defaultI18n.bytes(report.bytes);
  return <li className={styles.reportRow}>
    <Icon name={senderIcon(report.deviceClass)} size={20} />
    <div className={styles.reportText}><b>{title}</b><small>{whenText} {size}</small></div>
    <Button variant="glass" size="small" disabled={disabled} onClick={onRead}>{t('server.reports.read')}</Button>
    <Button ref={anchor} size="small" variant="icon" icon="i-more" iconOnly aria-label={t('next.home.moreFor', {title})} disabled={disabled} onClick={() => setOpen(value => !value)} />
    <Menu open={open && !disabled} onOpenChange={setOpen} anchorRef={anchor} title={title} returnFocusRef={anchor} items={[
      {id: 'download', icon: 'i-download', label: t('web.diagnostics.clientReportDownload'), onSelect: () => {window.location.assign(`${downloadBase}${clientReportDownloadPath(report.id)}`);}},
      {id: 'delete', icon: 'i-trash', destructive: true, label: t('web.diagnostics.clientReportDelete'), onSelect: onDelete},
    ]} />
  </li>;
}

/** One report's own log, read fresh for the dialog that is open now. */
function ReadReport({report, source, downloadBase, onClose, reportError}: Readonly<{report: ClientReport; source: SettingsReportsSource; downloadBase: string; onClose: () => void; reportError: (error: unknown) => void}>) {
  const [body, setBody] = useState<string>(), [failed, setFailed] = useState(false), done = useRef<HTMLButtonElement>(null);
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void source.read(report.id, controller.signal).then(value => {if (!controller.signal.aborted) {setBody(value.body ?? '');setLoaded(true);}})
      .catch(error => {if (!controller.signal.aborted) {setFailed(true);reportError(error);}});
    return () => controller.abort();
  }, [report.id, source, reportError]);
  const shown = body ?? '';
  const text = failed ? t('next.states.pageFailedBody') : !loaded ? t('next.settings.loading') : shown.length ? shown : t('server.reports.empty');
  return <Dialog open onOpenChange={open => {if (!open) onClose();}} size="lg" title={t('server.reports.title')} initialFocusRef={done} returnFocusRef={done}
    footer={<><a className={styles.download} href={`${downloadBase}${clientReportDownloadPath(report.id)}`} download>{t('web.diagnostics.clientReportDownload')}</a><Button ref={done} size="small" onClick={onClose}>{t('next.settings.done')}</Button></>}>
    <p className={styles.reportMeta}>{sender(report)} · {defaultI18n.dateTime(report.receivedAt)} · {defaultI18n.bytes(report.bytes)}</p>
    <pre className={styles.log}>{text}</pre>
  </Dialog>;
}

/** Titles a catalogue rebuild could not finish; the server tries them again by itself. */
function SetAside({failures, more, onCheck}: Readonly<{failures: readonly CatalogueFailure[]; more: boolean; onCheck: () => void}>) {
  return <section className={styles.panel}>
    <div className={styles.panelHead}><h3>{t('server.setAside.title')} <span className={styles.tag}>{t('next.settings.administratorsSee')}</span></h3></div>
    <p className={styles.lede}>{t('server.setAside.lede')}</p>
    <ul className={styles.rows}>{failures.map((failure, index) => <li key={`${failure.domain}-${failure.entityId ?? index}`} className={styles.setAsideRow}>
      <Icon name="i-alert" size={20} />
      <div className={styles.reportText}><b>{failure.title || t('server.setAside.untitled')}</b>
        <small>{t('server.setAside.tried', {count: failure.attempts, when: defaultI18n.dateTime(failure.failedAt)})}</small></div>
      <p className={styles.reason}>{failure.reason}</p>
    </li>)}</ul>
    {more && <p className={styles.note}>{t('server.setAside.more')}</p>}
    <div><Button variant="glass" size="small" onClick={onCheck}>{t('server.setAside.check')}</Button></div>
  </section>;
}

/** What the server noted, by lane, a page at a time. */
function Records({records, lane, next, onLane, onOlder, onLatest}: Readonly<{records: readonly EvidenceRecord[]; lane: RecordLane; next?: string; onLane: (lane: RecordLane) => void; onOlder: () => void; onLatest: () => void}>) {
  return <section className={styles.panel}>
    <div className={styles.panelHead}><h3>{t('settings.server.records')}</h3></div>
    <p className={styles.lede}>{t('next.settings.recordsHelp')}</p>
    <div className={styles.lanes} role="tablist">{LANES.map(entry => <button key={entry.id} type="button" role="tab" aria-selected={entry.id === lane}
      className={styles.lane} onClick={() => onLane(entry.id)}>{t(entry.label)}</button>)}</div>
    {records.length ? <ul className={styles.rows}>{records.map(record => <li key={record.sequence} className={styles.recordRow}>
      <span className={styles.level} data-level={record.severity ?? 'info'}>{levelLabel(record.severity)}</span>
      <div className={styles.reportText}><b>{sentence(record.code ?? record.action ?? '—')}{record.component ? <span className={styles.component}> {sentence(record.component)}</span> : null}</b>
        <small>{[when(record.at), ...Object.entries(record.fields ?? {}).map(([key, value]) => `${sentence(key)} ${value}`)].filter(Boolean).join(' · ')}</small></div>
      <span className={styles.sequence}>#{record.sequence}</span>
    </li>)}</ul> : <p className={styles.none}>{t('settings.server.records.none')}</p>}
    <div className={styles.pager}><Button variant="glass" size="small" onClick={onLatest}>{t('next.settings.recordsLatest')}</Button>
      <Button variant="glass" size="small" disabled={!next} onClick={onOlder}>{t('next.settings.recordsOlder')}</Button></div>
  </section>;
}
