import {useState} from 'react';
import {deleteClientReport, listClientReports, readClientReport, type ClientReport} from '@core/client-reports.ts';
import {problem, useAction, useRead} from '../../admin/console';
import {readCatalogueFailures} from '@core/catalogue-failures.ts';
import {dependsOn} from '@core/freshness-hub.ts';

// A rebuild that finishes, or fails again, moves this list.
const SET_ASIDE_MOVES = [dependsOn.content()];
import {useCursorList} from '../../admin/cursor-read';
import {ListPager} from './ListPager';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, ConfirmDialog, Dialog, Loading, Menu, Notice, SettingsEmpty, SettingsFooter, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Text} from '../../ui';

const CLIENT_REPORT_MOVES = [dependsOn.list('clientReports')];

/** "iPhone · phone · 1.4.2": what sent a report, in the words the report carries. */
export const reportSender = (report: Pick<ClientReport, 'platform' | 'deviceClass' | 'appVersion'>): string => [report.platform, report.deviceClass, report.appVersion].filter(Boolean).join(' · ');

/** Saves a report the way the server's own download does: one JSON file with what sent it and the log. */
function save(report: ClientReport) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2)], {type: 'application/json'}));
  const a = document.createElement('a');
  a.href = url;
  a.download = `portico-client-report-${report.receivedAt.slice(0, 19).replace(/[:T]/g, '-')}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * Troubleshooting › Client reports: the logs an app sent after something went wrong, newest
 * first. Each can be read here, downloaded, or deleted. Nothing is sent anywhere else.
 */
export function ClientReportsPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api, system, session} = useSession();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const read = useCursorList(async cursor => { const page = await listClientReports(api, serverId, cursor); return {items: [...page.items], nextCursor: page.nextCursor ?? ''}; }, [api, serverId], {on: CLIENT_REPORT_MOVES});
  const action = useAction();
  const [open, setOpen] = useState<{report: ClientReport; body?: string; error?: string}>();
  const [removing, setRemoving] = useState<ClientReport>();
  const load = (report: ClientReport, then: (full: ClientReport) => void) => readClientReport(api, serverId, report.id).then(then);
  const show = (report: ClientReport) => { setOpen({report}); load(report, full => setOpen(o => (o?.report.id === report.id ? {report: full, body: full.body ?? ''} : o))).catch(e => setOpen(o => (o?.report.id === report.id ? {report, error: problem(e)} : o))); };
  return (
    <SettingsGroup title={t('web.diagnostics.clientReports')} description={t('server.reports.lede')}>
      {read.error ? <SettingsNote><Notice tone={read.data ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {action.error && !removing ? <SettingsNote><Notice tone="error" compact>{action.error}</Notice></SettingsNote> : null}
      {!read.data && read.loading ? <SettingsLoading thing={t('server.thing.reports')} /> : null}
      {read.data?.map(report => (
        <SettingsRow key={report.id} icon="logs" label={reportSender(report) || t('server.reports.unknownSender')} help={`${i18n.dateTime(report.receivedAt)} · ${i18n.bytes(report.bytes)}`}
          control={<div style={{display: 'flex', gap: 8, alignItems: 'center'}}>
            <Button size="sm" variant="secondary" label={t('server.reports.read')} onClick={() => show(report)} />
            <Menu label={reportSender(report)} trigger={<Button size="sm" variant="ghost" icon="more" aria-label={t('server.row.actions', {name: reportSender(report) || t('server.reports.unknownSender')})} />} items={[{id: 'download', label: t('web.diagnostics.clientReportDownload'), icon: 'download'}, {id: 'delete', label: t('web.diagnostics.clientReportDelete'), icon: 'trash', destructive: true, separatorBefore: true}]}
              onSelect={id => { action.clear(); if (id === 'delete') setRemoving(report); else void action.run(() => load(report, save)); }} />
          </div>} />
      ))}
      {read.data && !read.data.length ? <SettingsEmpty title={t('server.reports.none')} /> : null}
      <ListPager pages={read} />
      <Dialog open={!!open} onOpenChange={o => !o && setOpen(undefined)} title={t('server.reports.title')} description={open ? `${reportSender(open.report)} · ${i18n.dateTime(open.report.receivedAt)}` : undefined} width={720}
        actions={<><Button variant="ghost" icon="download" label={t('web.diagnostics.clientReportDownload')} disabled={open?.body === undefined} onClick={() => open && save(open.report)} /><Button variant="primary" label={t('action.done')} onClick={() => setOpen(undefined)} /></>}>
        {open?.error ? <Notice tone="error" compact>{open.error}</Notice> : null}
        {open && open.body === undefined && !open.error ? <Loading label={t('server.reports.title')} /> : null}
        {open?.body !== undefined ? <pre style={{margin: 0, maxHeight: 440, overflow: 'auto', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontFamily: 'var(--font-mono)', fontSize: '0.75rem', lineHeight: 1.5}}>{open.body || t('server.reports.empty')}</pre> : null}
      </Dialog>
      <ConfirmDialog open={!!removing} onOpenChange={o => !o && setRemoving(undefined)} title={t('server.reports.deleteTitle')} body={t('server.reports.deleteBody')} confirmLabel={t('action.delete')} busy={action.busy} error={action.error || undefined}
        onConfirm={() => void action.run(async () => { if (!removing) return; await deleteClientReport(api, removing.id); setRemoving(undefined); read.reload(); })} />
    </SettingsGroup>
  );
}

/**
 * Troubleshooting › Titles set aside: titles a catalogue rebuild could not finish, so that
 * everything else could. Each says why, in the server's own words, and how often it has been
 * tried; the server tries again by itself. Nothing is shown while there are none.
 */
export function SetAsideTitlesPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const read = useRead(() => readCatalogueFailures(api), [api], {on: SET_ASIDE_MOVES});
  const list = read.data;
  if (!list?.failures.length) return null;
  return (
    <SettingsGroup title={t('server.setAside.title')} description={t('server.setAside.lede')}>
      {read.error ? <SettingsNote><Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {list.failures.map((f, n) => (
        <SettingsRow key={`${f.domain}:${f.entityId ?? n}`} icon="warning" label={f.title || t('server.setAside.untitled')} stack
          help={t('server.setAside.tried', {count: f.attempts, when: i18n.dateTime(f.failedAt)})}
          control={<Text variant="mono" tone="tertiary" style={{fontSize: '0.75rem', overflowWrap: 'anywhere'}}>{f.reason}</Text>} />
      ))}
      <SettingsFooter>
        <Button size="sm" variant="ghost" icon="refresh" label={t('server.setAside.check')} loading={read.loading} onClick={read.reload} />
        {list.more ? <Text variant="caption" tone="tertiary">{t('server.setAside.more')}</Text> : null}
      </SettingsFooter>
    </SettingsGroup>
  );
}
