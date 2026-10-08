import {BroadcastButton} from './MaintenanceWindows';
import {useState} from 'react';
import type {FeedbackReport, FeedbackStatus} from '@core/console.ts';
import {feedbackCategoryLabel, feedbackStatusLabel as statusLabel} from '@core/server-admin/panel-logic.ts';
export {feedbackCategoryLabel} from '@core/server-admin/panel-logic.ts';
import {sentence, useAction, useConsole, useRead, when} from '../../admin/console';
import {Badge, Button, Dialog, Freshness, ListRow, Notice, Select, StateView, Surface, Text, TextArea} from '../../ui';
import {SectionHeader} from './Server';
import {useI18n} from '../../app/i18n';
import s from './Server.module.css';

const statusTone = (v: FeedbackStatus): 'accent' | 'warning' | 'healthy' | 'neutral' => (v === 'open' ? 'accent' : v === 'in-progress' ? 'warning' : v === 'resolved' ? 'healthy' : 'neutral');
/** Viewer feedback: reports as an inbox, triaged in a dialog with a reply. */
export function FeedbackPanel() {
  const client = useConsole();
  const {t} = useI18n();
  const read = useRead(() => client.reports(true), [client]);
  const [selected, setSelected] = useState<FeedbackReport | null>(null);
  const items = read.data?.items ?? [];
  return (
    <>
      <SectionHeader title={t('settings.server.feedback')} lede={t('web.console.nav.feedbackLede')} actions={<div style={{display: 'flex', gap: 12, alignItems: 'center'}}><BroadcastButton /><Freshness at={read.at} onRefresh={read.reload} refreshing={read.loading} /></div>} />
      {read.error && !read.data ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {read.data && !items.length ? <StateView icon="mail" title={t('web.feedback.inboxEmpty')} body={t('web.feedback.inboxEmptyBody')} /> : null}
      {items.length ? <Surface padless>{items.map(r => <ListRow key={r.id} icon="mail" title={r.message.split('\n')[0]} subtitle={t('web.feedback.rowSubtitle', {category: feedbackCategoryLabel(r.category), when: when(r.createdAt), about: r.itemId ? 'yes' : 'no'})} meta={<Badge tone={statusTone(r.status)}>{statusLabel[r.status]}</Badge>} onClick={() => setSelected(r)} />)}</Surface> : null}
      <TriageDialog report={selected} onClose={() => setSelected(null)} onChanged={() => { setSelected(null); read.reload(); }} />
    </>
  );
}

function TriageDialog({report, onClose, onChanged}: {report: FeedbackReport | null; onClose: () => void; onChanged: () => void}) {
  const client = useConsole();
  const {t} = useI18n();
  const action = useAction();
  const [status, setStatus] = useState<FeedbackStatus>('in-progress');
  const [reply, setReply] = useState('');
  if (!report) return null;
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={feedbackCategoryLabel(report.category)} description={t('web.feedback.reportedMeta', {when: when(report.createdAt), status: statusLabel[report.status]})} width={560} actions={<><Button variant="ghost" label={t('action.close')} onClick={onClose} /><Button variant="primary" label={t('web.feedback.updateReport')} loading={action.busy} onClick={() => void action.run(() => client.triage(report.id, report.revision, status, reply), 'Updated.').then(ok => ok && onChanged())} /></>}>
      <Text as="p" variant="body" style={{whiteSpace: 'pre-wrap'}}>{report.message}</Text>
      {report.diagnostic ? <Text as="p" variant="caption" tone="tertiary">Sent from {sentence(report.diagnostic.platform)} while {report.diagnostic.state}, {report.diagnostic.online ? 'online' : 'offline'}.</Text> : null}
      <Select label={t('web.feedback.status')} value={status} options={(['open', 'in-progress', 'resolved', 'closed'] as FeedbackStatus[]).map(v => ({value: v, label: statusLabel[v]}))} onChange={e => setStatus(e.target.value as FeedbackStatus)} />
      <TextArea label={t('web.feedback.replyLabel')} optional value={reply} onChange={e => setReply(e.target.value)} maxLength={2000} help={t('web.feedback.replyHelp')} />
      {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
      <details className={s.details}><summary>{t('server.technicalDetails')}</summary><dl><dt>Report</dt><dd>{report.id}</dd><dt>Revision</dt><dd>{report.revision}</dd>{report.itemId ? <><dt>Item</dt><dd>{report.itemId}</dd></> : null}</dl></details> {/* lint-strings-allow: technical console readout (report id field labels) */}
    </Dialog>
  );
}
