import {useState} from 'react';
import type {Job} from '@core/console.ts';
import {jobLabel, jobStateLabel, jobTone, sentence, useAction, useConsole, useRead, when} from '../../admin/console';
import {Badge, Button, Freshness, Notice, StateView, Surface, Table, tableCell, Text} from '../../ui';
import s from './Server.module.css';
import {useI18n} from '../../app/i18n';
import {dependsOn} from '@core/freshness-hub.ts';

const TASK_EVENTS = Object.freeze([dependsOn.event('operation.updated'), dependsOn.event('library.scan.updated'), dependsOn.event('job.updated')]);

/** Background work: scans, analysis and the rest, with progress and what can be done about each. */
export function JobsPanel({compact}: {compact?: boolean}) {
  const i18n = useI18n();
  const {t} = i18n;
  const client = useConsole();
  const [cursor, setCursor] = useState('');
  // Tasks follow the server as it works: a scan's progress and a task's change of state arrive on the event feed.
  const read = useRead(() => client.jobs(cursor), [client, cursor], {every: 20000, on: TASK_EVENTS});
  const action = useAction();
  const items = read.data?.items ?? [];
  const command = (job: Job, a: 'cancel' | 'retry' | 'pause' | 'resume') => void action.run(() => client.jobCommand(job, a)).then(ok => ok && read.reload());
  return (
    <Surface padless>
      <div className={s.panelHead} style={{padding: '16px 16px 0'}}>
        <span className={s.panelTitle}>{t('web.activity.tasks')}</span>
        <Freshness at={read.at} staleAfterMs={60_000} onRefresh={read.reload} refreshing={read.loading} />
      </div>
      {read.error && !read.data ? <div style={{padding: 16}}><Notice tone="error" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {read.data && !items.length ? <StateView inline icon="jobs" title={t('web.activity.noTasks')} body={t('web.activity.noTasksBody')} /> : null}
      {items.length ? (
        <Table columns={[{label: t('web.activity.taskColumn')}, {label: t('web.activity.stateColumn')}, {label: t('web.activity.progressColumn'), num: true}, {label: compact ? t('web.activity.whenColumn') : t('web.activity.startedColumn')}, {label: '', actions: true}]} className="" >
          {items.map(job => (
            <tr key={job.id}>
              <td><Text variant="bodyStrong">{jobLabel(job.kind)}</Text>{job.resource ? <Text as="div" variant="caption" tone="tertiary">{job.resource}</Text> : null}{job.errorCode ? <Text as="div" variant="caption" tone="danger">{t('web.console.jobFailed')}</Text> : null}<details className={s.details}><summary>{t('server.technicalDetails')}</summary><dl><dt>{t('web.activity.taskColumn')}</dt><dd>{job.id}</dd><dt>{t('web.activity.detail.phase')}</dt><dd>{job.phase}</dd><dt>{t('web.activity.detail.attempt')}</dt><dd>{job.attempt}</dd><dt>{t('web.activity.detail.queue')}</dt><dd>{job.lane}</dd><dt>{t('web.activity.detail.trigger')}</dt><dd>{job.trigger}</dd></dl></details></td>
              <td><Badge tone={jobTone(job.state)} dot={job.state === 'running'} live={job.state === 'running'}>{jobStateLabel[job.state]}</Badge></td>
              <td className={tableCell.num}>{job.processed != null ? i18n.number(job.processed) : '—'}</td>
              <td>{when(i18n, job.createdAt)}</td>
              <td className={tableCell.actions}>
                {job.actions.includes('pause') ? <Button size="sm" variant="ghost" label={t('action.pause')} disabled={action.busy} onClick={() => command(job, 'pause')} /> : null}
                {job.actions.includes('resume') ? <Button size="sm" variant="ghost" label={t('entry.resume')} disabled={action.busy} onClick={() => command(job, 'resume')} /> : null}
                {job.actions.includes('retry') ? <Button size="sm" variant="ghost" label={t('action.tryAgain')} disabled={action.busy} onClick={() => command(job, 'retry')} /> : null}
                {job.actions.includes('cancel') ? <Button size="sm" variant="ghost" label={t('action.cancel')} disabled={action.busy} onClick={() => command(job, 'cancel')} /> : null}
              </td>
            </tr>
          ))}
        </Table>
      ) : null}
      {read.data?.nextCursor || cursor ? <div style={{display: 'flex', gap: 8, padding: 12}}><Button size="sm" variant="ghost" label={t('web.activity.latest')} disabled={!cursor} onClick={() => setCursor('')} /><Button size="sm" variant="ghost" label={t('web.hostedAccount.older')} disabled={!read.data?.nextCursor} onClick={() => setCursor(read.data!.nextCursor)} /></div> : null}
      {action.error ? <div style={{padding: '0 16px 16px'}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
    </Surface>
  );
}
