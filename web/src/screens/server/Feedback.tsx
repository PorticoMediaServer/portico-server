import {BroadcastButton} from './MaintenanceWindows';
import {feedbackCategoryName, feedbackStatusName} from '@core/server-admin/console-catalogue.ts';
import {useState} from 'react';
import type {FeedbackReport, FeedbackStatus} from '@core/console.ts';
export {feedbackCategoryLabel} from '@core/server-admin/panel-logic.ts';
import {QUIET_READ_EVERY_MS, useAction, useConsole, useRead, when} from '../../admin/console';
import {devicePlatform} from '@core/server-admin/people-words.ts';
import {Badge, Button, Dialog, Freshness, ListRow, Notice, Select, StateView, Surface, Text, TextArea} from '../../ui';
import {SectionHeader} from './Server';
import {useI18n} from '../../app/i18n';
import s from './Server.module.css';

const statusTone = (v: FeedbackStatus): 'accent' | 'warning' | 'healthy' | 'neutral' => (v === 'open' ? 'accent' : v === 'in-progress' ? 'warning' : v === 'resolved' ? 'healthy' : 'neutral');
/** Viewer feedback: reports as an inbox, triaged in a dialog with a reply. */
export function FeedbackPanel() {
  const client = useConsole();
  const i18n = useI18n();
  const {t} = i18n;
  // A report a viewer sends shows here by itself.
  const read = useRead(() => client.reports(true), [client], {every: QUIET_READ_EVERY_MS});
  const has = (id: string) => i18n.has(id);
  const [selected, setSelected] = useState<FeedbackReport | null>(null);
  const items = read.data?.items ?? [];
  return (
    <>
      <SectionHeader title={t('settings.server.feedback')} lede={t('web.console.nav.feedbackLede')} actions={<div style={{display: 'flex', gap: 12, alignItems: 'center'}}><BroadcastButton /><Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} /></div>} />
      {read.error && !read.data ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {read.data && !items.length ? <StateView icon="mail" title={t('web.feedback.inboxEmpty')} body={t('web.feedback.inboxEmptyBody')} /> : null}
      {items.length ? <Surface padless>{items.map(r => <ListRow key={r.id} icon="mail" title={r.message.split('\n')[0]} subtitle={t('web.feedback.rowSubtitle', {category: feedbackCategoryName(t, has, r.category), when: when(i18n, r.createdAt), about: r.itemId ? 'yes' : 'no'})} meta={<Badge tone={statusTone(r.status)}>{feedbackStatusName(t, has, r.status)}</Badge>} onClick={() => setSelected(r)} />)}</Surface> : null}
      <TriageDialog report={selected} onClose={() => setSelected(null)} onChanged={() => { setSelected(null); read.reload(); }} />
    </>
  );
}

function TriageDialog({report, onClose, onChanged}: {report: FeedbackReport | null; onClose: () => void; onChanged: () => void}) {
  const client = useConsole();
  const i18n = useI18n();
  const {t} = i18n;
  const action = useAction();
  const [status, setStatus] = useState<FeedbackStatus>('in-progress');
  const [reply, setReply] = useState('');
  const has = (id: string) => i18n.has(id);
  if (!report) return null;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={feedbackCategoryName(t, has, report.category)} description={t('web.feedback.reportedMeta', {when: when(i18n, report.createdAt), status: feedbackStatusName(t, has, report.status)})} width={560} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="primary" label={t('web.feedback.updateReport')} loading={action.busy} onClick={() => void action.run(() => client.triage(report.id, report.revision, status, reply), t('server.feedback.updated')).then(ok => ok && onChanged())} /></>}>
      <Text as="p" variant="body" style={{whiteSpace: 'pre-wrap'}}>{report.message}</Text>
      {report.diagnostic ? <Text as="p" variant="caption" tone="tertiary">{t('server.feedback.sentFrom', {platform: devicePlatform(t, report.diagnostic.platform) || report.diagnostic.platform, state: report.diagnostic.state, online: report.diagnostic.online ? 'yes' : 'no'})}</Text> : null}
      <Select label={t('web.feedback.status')} value={status} options={(['open', 'in-progress', 'resolved', 'closed'] as FeedbackStatus[]).map(v => ({value: v, label: feedbackStatusName(t, has, v)}))} onChange={e => setStatus(e.target.value as FeedbackStatus)} />
      <TextArea label={t('web.feedback.replyLabel')} optional value={reply} onChange={e => setReply(e.target.value)} maxLength={2000} help={t('web.feedback.replyHelp')} />
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
      <details className={s.details}><summary>{t('server.technicalDetails')}</summary><dl><dt>{t('server.feedback.field.report')}</dt><dd>{report.id}</dd><dt>{t('server.feedback.field.revision')}</dt><dd>{report.revision}</dd>{report.itemId ? <><dt>{t('server.feedback.field.item')}</dt><dd>{report.itemId}</dd></> : null}</dl></details>
    </Dialog>
  );
}
