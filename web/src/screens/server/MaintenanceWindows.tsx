import {useState} from 'react';
import type {Schedule} from '@core/console.ts';
import type {LogSettings} from '@core/server-administration.ts';
import {scheduledJobs, type LogsDraft, type MaintenanceDocument, type MaintenanceSettings, type MaintenanceWindow} from '@core/server-admin/server-forms.ts';
import {TASK_LABELS, broadcastBody, broadcastProblem, clockMinutes, clockText, retentionRows, utf8Length} from '@core/server-admin/panel-logic.ts';
export {backgroundPriorityOrDefault, broadcastBody, broadcastBodyBytes, broadcastProblem, broadcastTitleBytes, maintenanceSaveSettings, utf8Length, type BackgroundTaskPriority, type BroadcastProblem} from '@core/server-admin/panel-logic.ts';
import {jobLabel, useAction, useConsole, useRead, when} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {Button, Dialog, Input, Notice, Select, SettingsGroup, SettingsRow, Status, Switch, Text, TextArea} from '../../ui';
import {FormLoadNotice, useServerForm} from '../settings/ServerForms';
import page from '../settings/Settings.module.css';

const clock = clockText, minutes = clockMinutes;

function useTaskLabel() {
  const {t} = useI18n();
  return (task: string) => (TASK_LABELS[task] ? t(TASK_LABELS[task]!) : jobLabel(task));
}

/**
 * Schedule › Maintenance windows: when the server does its heavier background work and what
 * runs in each window. Edits go to the page's maintenance form, saved with the page.
 */
export function WindowsPanel() {
  const {t} = useI18n();
  const task = useTaskLabel();
  const {form, state} = useServerForm<MaintenanceSettings, MaintenanceDocument>('maintenance');
  const draft = state.draft, doc = state.status;
  const change = (index: number, next: Partial<MaintenanceWindow>) => form.set(d => ({...d, windows: d.windows.map((w, i) => (i === index ? {...w, ...next} : w))}));
  return (
    <SettingsGroup title={t('settings.server.windows')} description={t('settings.server.windowsLede')}>
      <FormLoadNotice id="maintenance" />
      {draft && doc ? draft.windows.map((w, i) => (
        <SettingsRow key={w.id} label={<Input hideLabel label={t('web.maintenance.windowName')} value={w.name} onChange={e => change(i, {name: e.target.value})} style={{width: 200}} />} stack
          control={<div style={{display: 'flex', flexDirection: 'column', gap: 12, width: '100%'}}>
            <div style={{display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center'}}>
              <Switch checked={w.enabled} onCheckedChange={v => change(i, {enabled: v})} label={t('web.dvr.on')} />
              <Select hideLabel label={t('web.maintenance.days')} value={w.cadence} onChange={e => { const c = doc.cadences.find(x => x.id === e.target.value); change(i, {cadence: e.target.value, days: c ? [...c.days] : w.days}); }} options={doc.cadences.map(c => ({value: c.id, label: c.name}))} />
              <Text variant="caption" tone="tertiary">{t('server.windows.from')}</Text>
              <Input hideLabel label={t('web.maintenance.startsAt')} type="time" value={clock(w.startMinute)} onChange={e => change(i, {startMinute: minutes(e.target.value)})} style={{width: 120}} />
              <Text variant="caption" tone="tertiary">{t('server.windows.for')}</Text>
              <Input hideLabel label={t('web.maintenance.hours')} type="number" min={1} max={24} value={Math.round(w.durationMinutes / 60)} onChange={e => change(i, {durationMinutes: Math.min(24, Math.max(1, Number(e.target.value) || 1)) * 60})} style={{width: 70}} />
              <Text variant="caption" tone="tertiary">{t('server.windows.hours')}</Text>
              {draft.windows.length > 1 ? <Button size="sm" variant="ghost" icon="trash" aria-label={t('web.maintenance.removeWindow')} onClick={() => form.set(d => ({...d, windows: d.windows.filter((_, n) => n !== i)}))} /> : null}
            </div>
            <div style={{display: 'flex', gap: 12, flexWrap: 'wrap'}}>{doc.tasks.map(id => <label key={id} style={{display: 'flex', gap: 8, alignItems: 'center', fontSize: 13}}><Switch checked={w.tasks.includes(id)} onCheckedChange={v => change(i, {tasks: v ? [...w.tasks, id] : w.tasks.filter(x => x !== id)})} label={task(id)} />{task(id)}</label>)}</div>
          </div>} />
      )) : null}
    </SettingsGroup>
  );
}

/**
 * Schedule › Scheduled jobs: every job the server runs on a schedule (backups included), the
 * windows it runs in, when it last ran and when the next window opens.
 */
export function ScheduledJobsPanel() {
  const {t} = useI18n();
  const task = useTaskLabel();
  const client = useConsole();
  const {state} = useServerForm<MaintenanceSettings, MaintenanceDocument>('maintenance');
  // The last run of each kind, from the newest page of finished work.
  const jobs = useRead(() => client.jobs(''), [client], {every: 60000});
  const schedules = useRead<Schedule[]>(() => client.schedules(), [client]);
  const doc = state.status, settings = state.draft;
  const last = (kind: string) => jobs.data?.items.find(j => j.kind === kind || j.kind === kind.replace(/-/g, '_'));
  const rows = doc && settings ? scheduledJobs(doc, settings, new Date()) : [];
  return (
    <SettingsGroup title={t('settings.server.jobs')}>
      <FormLoadNotice id="maintenance" />
      {rows.map(job => {
        const ran = last(job.task);
        return (
          <SettingsRow key={job.task} icon="jobs" label={task(job.task)}
            help={job.windows.length ? t('settings.server.jobs.in', {windows: job.windows.join(', ')}) : t('settings.server.jobs.never')}
            control={<div className={page.statusFacts}>{ran ? <span>{t('settings.server.jobs.last', {when: when(ran.createdAt)})}</span> : null}{job.nextAt ? <Status tone="neutral">{t('settings.server.jobs.next', {when: when(job.nextAt.getTime())})}</Status> : null}</div>} />
        );
      })}
      {/* Schedules an owner or an integration added by hand, each with its own window. */}
      {schedules.data?.map(x => <ScheduleRow key={x.id} schedule={x} onSaved={schedules.reload} />)}
    </SettingsGroup>
  );
}

const zones = (() => {
  try { return (Intl as unknown as {supportedValuesOf?: (k: string) => string[]}).supportedValuesOf?.('timeZone') ?? []; } catch { return []; }
})();

/** An extra schedule: edited in a dialog, since it is a separate record with its own revision. */
function ScheduleRow({schedule, onSaved}: {schedule: Schedule; onSaved: () => void}) {
  const client = useConsole();
  const {t} = useI18n();
  const action = useAction();
  const [draft, setDraft] = useState<Schedule>();
  const options = zones.length ? zones.map(z => ({value: z, label: z})) : [{value: schedule.timezone, label: schedule.timezone}];
  return (
    <>
      <SettingsRow icon="jobs" label={jobLabel(schedule.kind)} help={[schedule.resource, schedule.enabled ? t('server.schedule.daily', {time: clock(schedule.startMinute)}) : t('server.schedule.off'), schedule.lastSlot ? t('settings.server.jobs.last', {when: schedule.lastSlot}) : ''].filter(Boolean).join(' · ')}
        control={<Button size="sm" variant="ghost" label={t('web.selection.edit')} onClick={() => { action.clear(); setDraft(schedule); }} />} />
      <Dialog open={!!draft} onOpenChange={o => !o && setDraft(undefined)} title={jobLabel(schedule.kind)} width={480}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setDraft(undefined)} /><Button variant="primary" label={t('action.save')} loading={action.busy} disabled={!draft || JSON.stringify(draft) === JSON.stringify(schedule)} onClick={() => draft && void action.run(() => client.saveSchedule(draft, schedule.revision)).then(ok => { if (ok) { setDraft(undefined); onSaved(); } })} /></>}>
        {draft ? (
          <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
            {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
            <SettingsRow label={t('web.schedules.enabled')} help={t('web.schedules.enabledHelp')} control={<Switch checked={draft.enabled} onCheckedChange={v => setDraft({...draft, enabled: v})} label={t('web.schedules.enabled')} />} />
            <Input label={t('web.schedules.windowStarts')} help={t('web.schedules.windowStartsHelp')} type="time" value={clock(draft.startMinute)} onChange={e => setDraft({...draft, startMinute: minutes(e.target.value)})} />
            <Input label={t('web.schedules.windowLength')} help={t('web.schedules.windowLengthHelp')} type="number" min={15} max={1440} step={15} inputMode="numeric" value={draft.windowMinutes} onChange={e => setDraft({...draft, windowMinutes: Math.max(15, Number(e.target.value) || 15)})} />
            <Select label={t('web.schedules.timeZone')} value={draft.timezone} options={options} onChange={e => setDraft({...draft, timezone: e.target.value})} />
            <SettingsRow label={t('web.schedules.catchUpRow')} help={t('web.schedules.catchUpHelp')} control={<Switch checked={draft.catchUp} onCheckedChange={v => setDraft({...draft, catchUp: v})} label={t('web.schedules.catchUp')} />} />
          </div>
        ) : null}
      </Dialog>
    </>
  );
}

/**
 * Storage & backups › What Portico keeps: the rows whose keys the server names at run time
 * (how long each kind of generated file stays, and the message logs by category). The fixed
 * rows above them are authored in the structure; all of them save with the page.
 */
export function RetentionRows() {
  const {t} = useI18n();
  const maintenance = useServerForm<MaintenanceSettings, MaintenanceDocument>('maintenance');
  const logs = useServerForm<LogsDraft, LogSettings>('logs');
  const days = t('settings.unit.days');
  const draft = maintenance.state.draft, doc = maintenance.state.status;
  return (
    <>
      {draft && doc ? retentionRows(Object.keys(draft.retention), key => doc.storageCategories.find(c => c.retentionKey === key)?.name, t).map(({key, label}) => (
        <SettingsRow key={key} label={label}
          control={<span className={page.numberField}><Input hideLabel label={label} type="number" min={0} max={doc.retentionMaxima[key] ?? 3650} value={draft.retention[key]} onChange={e => maintenance.form.set(d => ({...d, retention: {...d.retention, [key]: Math.min(doc.retentionMaxima[key] ?? 3650, Math.max(0, Number(e.target.value) || 0))}}))} /><span className={page.unit}>{days}</span></span>} />
      )) : null}
      {logs.state.draft?.retention.map(r => (
        <SettingsRow key={r.category} label={t('settings.server.retention.logs', {category: t(`server.logCategory.${r.category}` as MessageId)})}
          control={<span className={page.numberField}><Input hideLabel label={t('settings.server.retention.logs', {category: r.category})} type="number" min={1} max={365} value={r.days} onChange={e => logs.form.set(d => ({retention: d.retention.map(x => (x.category === r.category ? {...x, days: Math.min(365, Math.max(1, Number(e.target.value) || 1))} : x))}))} /><span className={page.unit}>{days}</span></span>} />
      ))}
    </>
  );
}

/** A message from the owner to everyone on the server. */
export function BroadcastButton() {
  const {api} = useSession();
  const {t} = useI18n();
  const action = useAction();
  const [open, setOpen] = useState(false);
  const [operation, setOperation] = useState('');
  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');
  const [audience, setAudience] = useState('profile');
  const [severity, setSeverity] = useState('info');
  const problem = broadcastProblem(title, body);
  const send = () => void action.run(async () => { await api.request('/v1/admin/notifications/broadcast', 'POST', broadcastBody(operation, {audience, severity, title, body})); setOpen(false); setTitle(''); setBody(''); setOperation(''); }, 'Message sent.');
  return (
    <>
      <Button size="sm" variant="secondary" icon="mail" label={t('server.messageEveryone')} onClick={() => { action.clear(); setOperation(crypto.randomUUID()); setOpen(true); }} />
      {action.notice ? <Text variant="caption" tone="tertiary">{action.notice}</Text> : null}
      <Dialog open={open} onOpenChange={setOpen} title={t('server.messageEveryone')} description={t('web.broadcast.lede')} width={500}
        actions={<><Button variant="ghost" label={t('action.cancel')} onClick={() => setOpen(false)} /><Button variant="primary" label={t('feedback.send')} disabled={!title.trim() || !!problem} loading={action.busy} onClick={send} /></>}>
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {action.error ? <Notice tone="error" compact>{action.error}</Notice> : null}
          {problem ? <Notice tone="error" compact>{problem === 'title-too-long' ? t('web.broadcast.titleTooLong', {count: utf8Length(title.trim())}) : t('web.broadcast.bodyTooLong', {count: utf8Length(body.trim())})}</Notice> : null}
          <Input label={t('web.broadcast.title')} maxLength={120} autoFocus value={title} onChange={e => setTitle(e.target.value)} placeholder={t('web.broadcast.titleExample')} />
          <TextArea label={t('web.broadcast.message')} optional rows={4} maxLength={2000} value={body} onChange={e => setBody(e.target.value)} />
          <div style={{display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12}}>
            <Select label={t('web.broadcast.audience')} value={audience} onChange={e => setAudience(e.target.value)} options={[{value: 'profile', label: t('web.together.everyone')}, {value: 'account-admin', label: t('web.broadcast.adminOnly')}]} />
            <Select label={t('web.broadcast.importance')} value={severity} onChange={e => setSeverity(e.target.value)} options={[{value: 'info', label: t('web.broadcast.info')}, {value: 'warning', label: t('web.broadcast.notice')}, {value: 'critical', label: t('web.broadcast.urgent')}]} />
          </div>
        </div>
      </Dialog>
    </>
  );
}
