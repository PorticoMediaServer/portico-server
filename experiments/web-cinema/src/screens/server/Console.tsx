import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {FeedbackRow, LogLine, PlaybackSession} from '../../data/admin';
import {Panel, Row, KeyValue} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Checkbox, Field, Input, Segmented, Select, Stepper, Switch, TextArea} from '../../ui/Controls';
import {Badge, Notice, Progress, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {Dialog, DialogClose} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {relativeTime} from '../shared/items';
import {AdminSkeleton, Bars, HealthDot, SectionHead, Spark, Stat, StatusBadge, UnsavedBar, useAdmin, useAdminForm} from './common';
import type {Sec} from './Server';

export function ConsoleSection({id, sec}: {id: string; sec: Sec}) {
  switch (id) {
    case 'now-playing': return <NowPlaying sec={sec} />;
    case 'activity': return <Activity sec={sec} />;
    case 'logs': return <Logs sec={sec} />;
    case 'diagnostics': return <Diagnostics sec={sec} />;
    case 'feedback': return <Feedback sec={sec} />;
    case 'alerts': return <Alerts sec={sec} />;
    default: return <Overview sec={sec} />;
  }
}

function Overview({sec}: {sec: Sec}) {
  const services = useServices();
  const navigate = useNavigate();
  const q = useAdmin('overview');
  const playing = useAdmin('nowPlaying');
  const o = q.data;
  if (!o) return <AdminSkeleton />;
  const open = o.attention.filter(a => !a.acknowledged);
  const days = Array.from({length: 30}, (_, i) => new Date(Date.now() - (29 - i) * 86400000).toLocaleDateString('en-US', {month: 'short', day: 'numeric'}));
  return (
    <div className="adm-sections">
      <SectionHead eyebrow={`Portico ${o.version} · up ${o.uptimeDays} days`} title={o.serverName} lede={sec.lede} actions={<Button variant="secondary" icon="refresh" label="Refresh" onClick={() => { q.reload(); toast('Updated just now'); }} />} />
      <div className="stat-row">
        <Stat label="Watching now" icon="play" value={playing.data?.length ?? '—'} sub={`${playing.data?.filter(p => p.delivery.startsWith('Converting')).length ?? 0} converting`} />
        <Stat label="Processor" icon="cpu" value={`${Math.round(o.telemetry.cpu.at(-1) ?? 0)}%`} sub="Average over 5 minutes" />
        <Stat label="Media storage" icon="drive" value={`${Math.round((o.storage[0]!.usedTB / o.storage[0]!.totalTB) * 100)}%`} sub={`${(o.storage[0]!.totalTB - o.storage[0]!.usedTB).toFixed(1)} TB free`} tone="warn" />
        <Stat label="Needs you" icon="alert" value={open.length} sub={open.length ? 'See below' : 'All clear'} tone={open.some(a => a.severity === 'urgent') ? 'bad' : undefined} />
      </div>
      {open.length ? (
        <Panel title="Needs your attention" padded={false}>
          {open.map(a => (
            <div key={a.id} className={cx('attn', `is-${a.severity}`)}>
              <span className="attn-icon"><Icon name={a.severity === 'urgent' ? 'alert' : a.severity === 'warn' ? 'alert' : 'info'} size={18} /></span>
              <div className="attn-text"><strong>{a.title}</strong><span>{a.body}</span></div>
              <div className="rec-actions">
                <Button size="sm" variant="secondary" label="Open" onClick={() => void navigate({to: '/server/$section', params: {section: a.section}})} />
                <Button size="sm" variant="ghost" label="Dismiss" onClick={() => void services.admin.run('acknowledge', {id: a.id}).then(() => toast('Dismissed; it comes back if things change'))} />
              </div>
            </div>
          ))}
        </Panel>
      ) : <Notice tone="good" title="Nothing needs you">The server is healthy.</Notice>}
      <Panel title="Health" padded={false}>
        <div className="health-grid">
          {o.health.map(h => <div key={h.label} className="health-item"><HealthDot state={h.state} /><div><strong>{h.label}</strong><span>{h.detail}</span></div></div>)}
        </div>
      </Panel>
      <Panel title="Last 24 hours" subtitle="Sampled every 30 minutes.">
        <div className="spark-grid">
          <Spark label="Processor" values={o.telemetry.cpu} />
          <Spark label="Memory" values={o.telemetry.memory} color="#8a5fd0" />
          <Spark label="Upload" values={o.telemetry.netOut} unit=" Mb/s" color="#3fae78" />
          <Spark label="Disk reads" values={o.telemetry.diskRead} unit=" MB/s" color="#e8b04c" />
          <Spark label="Graphics" values={o.telemetry.gpu} color="#d4668f" />
        </div>
      </Panel>
      <div className="grid-2">
        <Panel title="Viewing, last 30 days" subtitle={`${o.telemetry.viewingHours.reduce((a, b) => a + b, 0)} hours in all`}>
          <Bars values={o.telemetry.viewingHours} labels={days} label="Hours watched per day" />
        </Panel>
        <Panel title="Storage">
          <div className="stack" style={{gap: 14}}>
            {o.storage.map(s => (
              <div key={s.label}>
                <div className="storage-line"><strong>{s.label}</strong><span className="faint">{s.usedTB} of {s.totalTB} TB</span></div>
                <Progress value={s.usedTB / s.totalTB} tone={s.usedTB / s.totalTB > 0.9 ? 'bad' : s.usedTB / s.totalTB > 0.75 ? 'warn' : 'accent'} label={`${s.label} used`} />
              </div>
            ))}
          </div>
        </Panel>
      </div>
      <Panel title="Recently played" padded={false} actions={<Button size="sm" variant="ghost" label="Activity" onClick={() => void navigate({to: '/server/$section', params: {section: 'activity'}})} />}>
        {o.recent.map((r, i) => <Row key={i} icon="play" label={r.title} help={`${r.profile} · ${r.device} · ${relativeTime(r.at)}`} />)}
      </Panel>
    </div>
  );
}

function NowPlaying({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('nowPlaying');
  const [stop, setStop] = useState<PlaybackSession | null>(null);
  const [msg, setMsg] = useState('');
  const items = useQuery(q.data ? `admin.np.items.${q.data.map(s => s.itemId).join()}` : null, () => services.catalog.items(q.data!.map(s => s.itemId)));
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  if (!q.data) return <AdminSkeleton />;
  const total = q.data.reduce((a, s) => a + s.rateMbps, 0);
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <div className="stat-row">
        <Stat label="Sessions" icon="play" value={q.data.length} />
        <Stat label="Converting" icon="cpu" value={q.data.filter(s => s.delivery.startsWith('Converting')).length} sub="of 4 allowed at once" />
        <Stat label="Bandwidth" icon="network" value={`${total.toFixed(1)} Mb/s`} sub="Upload in use" />
        <Stat label="Away from home" icon="globe" value={q.data.filter(s => s.location === 'Remote').length} />
      </div>
      {!q.data.length ? <StateView icon="play" title="Nobody’s watching right now" body="Sessions appear here as soon as someone presses Play." /> : (
        <div className="np-cards">
          {q.data.map(s => {
            const it = byId.get(s.itemId);
            return (
              <div key={s.id} className="np-card">
                <div className="np-card-art">{it ? <Art src={it.art.backdrop ?? it.art.still ?? it.art.poster} tint={it.tint} ratio="16/9" /> : null}<StatusBadge state={s.state} /></div>
                <div className="np-card-body">
                  <strong>{s.title}</strong>
                  <span className="faint small">{s.subtitle}</span>
                  <Progress value={s.progress} thin label="Progress" />
                  <KeyValue items={[['Who', `${s.profile} (${s.account})`], ['Device', `${s.device} · ${s.platform}`], ['Delivery', <>{s.delivery}{s.hardware ? <span className="faint"> · {s.hardware}</span> : null}</>], ['Quality', s.quality], ['Network', `${s.location} · ${s.rateMbps ? `${s.rateMbps} Mb/s` : 'idle'}`], ['Started', relativeTime(s.startedAt)]]} />
                  <Button size="sm" variant="danger" icon="stop" label="Stop playback…" onClick={() => setStop(s)} />
                </div>
              </div>
            );
          })}
        </div>
      )}
      <Dialog open={!!stop} onOpenChange={o => !o && setStop(null)} title={`Stop ${stop?.profile}’s playback?`} description={`“${stop?.title}” on ${stop?.device} stops right away.`} size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="danger" label="Stop playback" onClick={() => stop && void services.admin.run('stopSession', {id: stop.id, message: msg}).then(() => { toast(`Stopped ${stop.profile}’s playback`); setStop(null); setMsg(''); })} /></>}>
        <Field label="Message to show them" optional help="They see this on their screen."><TextArea value={msg} onChange={e => setMsg(e.target.value)} placeholder="Restarting the server for an update. Back in five minutes." rows={3} /></Field>
      </Dialog>
    </div>
  );
}

function Activity({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('activity');
  const np = useAdmin('nowPlaying');
  const ov = useAdmin('overview');
  const [filter, setFilter] = useState<'all' | 'running' | 'failed'>('all');
  if (!q.data) return <AdminSkeleton />;
  const tasks = q.data.tasks.filter(t => filter === 'all' || (filter === 'running' ? t.state === 'running' || t.state === 'queued' : t.state === 'failed'));
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel title="Playing now" padded={false}>
        {(np.data ?? []).map(s => <Row key={s.id} icon="play" label={`${s.title}${s.subtitle ? ` · ${s.subtitle}` : ''}`} help={`${s.profile} · ${s.device} · ${s.delivery}`}><StatusBadge state={s.state} /></Row>)}
        {np.data && !np.data.length ? <Row label="Nothing is playing" /> : null}
      </Panel>
      <Panel title="Background tasks" padded={false} actions={<Segmented size="sm" label="Show" value={filter} onChange={setFilter} options={[{value: 'all', label: 'All'}, {value: 'running', label: 'Running'}, {value: 'failed', label: 'Failed'}]} />}>
        {tasks.map(t => (
          <div key={t.id} className="task-row">
            <div className="task-text"><strong>{t.name}</strong><span className="faint small">{t.detail} · {t.state === 'queued' ? `starts ${new Date(t.startedAt).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}` : relativeTime(t.startedAt)}</span>{t.state === 'running' ? <Progress value={t.progress} thin label={`${t.name} progress`} /> : null}</div>
            <StatusBadge state={t.state} />
            {t.state === 'running' || t.state === 'queued' ? <Button size="sm" variant="ghost" label="Cancel" onClick={() => void services.admin.set('activity', {tasks: q.data!.tasks.map(x => (x.id === t.id ? {...x, state: 'cancelled'} : x))}).then(() => toast('Task canceled'))} /> : t.state === 'failed' ? <Button size="sm" variant="secondary" label="Retry" onClick={() => void services.admin.set('activity', {tasks: q.data!.tasks.map(x => (x.id === t.id ? {...x, state: 'running', progress: 0.02} : x))}).then(() => toast('Retrying'))} /> : null}
          </div>
        ))}
        {!tasks.length ? <Row label="Nothing here" /> : null}
      </Panel>
      <Panel title="Recently played" padded={false}>{(ov.data?.recent ?? []).map((r, i) => <Row key={i} icon="history" label={r.title} help={`${r.profile} · ${r.device} · ${relativeTime(r.at)}`} />)}</Panel>
    </div>
  );
}

function Logs({sec}: {sec: Sec}) {
  const form = useAdminForm('logs');
  const [q, setQ] = useState('');
  const [exporting, setExporting] = useState(false);
  const l = form.data;
  if (!l) return <AdminSkeleton />;
  const order = ['debug', 'info', 'warn', 'error'];
  const lines = l.lines.filter((x: LogLine) => order.indexOf(x.level) >= order.indexOf(l.level) && (!q || `${x.component} ${x.message}`.toLowerCase().includes(q.toLowerCase())));
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="secondary" icon="download" label="Support export…" onClick={() => setExporting(true)} />} />
      <Panel padded={false}>
        <div className="log-tools">
          <Segmented size="sm" label="Level" value={l.level} onChange={v => form.set({level: v})} options={[{value: 'debug', label: 'Debug'}, {value: 'info', label: 'Info'}, {value: 'warn', label: 'Warnings'}, {value: 'error', label: 'Errors'}]} />
          <Input value={q} onChange={e => setQ(e.target.value)} placeholder="Filter messages" aria-label="Filter messages" className="log-find" />
          <label className="hstack small"><Switch checked={l.following} onChange={v => form.set({following: v})} label="Follow new messages" />Follow</label>
        </div>
        <div className="log-view" role="log" aria-label="Server log">
          {lines.map(x => <div key={x.id} className={cx('log-line', `is-${x.level}`)}><span className="log-time tabular">{new Date(x.at).toLocaleTimeString('en-US', {hour12: false})}</span><span className="log-level">{x.level}</span><span className="log-comp">{x.component}</span><span className="log-msg">{x.message}</span></div>)}
          {!lines.length ? <p className="faint" style={{padding: 16}}>No messages at this level{q ? ` matching “${q}”` : ''}.</p> : null}
        </div>
      </Panel>
      <div className="grid-2">
        <Panel title="Detailed capture" subtitle="Record debug messages for one part of the server for a while, then return to normal.">
          <div className="stack">
            <Field label="Part of the server"><Select value={l.capture.component} onChange={v => form.set({capture: {...l.capture, component: v}})} options={['playback', 'transcoder', 'scanner', 'livetv', 'auth', 'network'].map(v => ({value: v, label: v}))} /></Field>
            <div className="field"><span className="field-label">For</span><Stepper label="Minutes" value={l.capture.minutes} onChange={v => form.set({capture: {...l.capture, minutes: v}})} min={5} max={120} step={5} suffix="min" /></div>
            <Button variant="secondary" icon="record" label="Start capture" onClick={() => toast(`Capturing ${l.capture.component} for ${l.capture.minutes} minutes`)} />
          </div>
        </Panel>
        <Panel title="Keep records for">
          <div className="stack">
            <div className="field"><span className="field-label">Log messages</span><Stepper label="Log retention" value={l.retention.records} onChange={v => form.set({retention: {...l.retention, records: v}})} min={7} max={365} suffix="days" /></div>
            <div className="field"><span className="field-label">Notifications</span><Stepper label="Notification retention" value={l.retention.notifications} onChange={v => form.set({retention: {...l.retention, notifications: v}})} min={7} max={365} suffix="days" /></div>
            <div className="field"><span className="field-label">Finished tasks</span><Stepper label="Task retention" value={l.retention.tasks} onChange={v => form.set({retention: {...l.retention, tasks: v}})} min={7} max={365} suffix="days" /></div>
          </div>
        </Panel>
      </div>
      <UnsavedBar form={form} />
      <Dialog open={exporting} onOpenChange={setExporting} title="Support export" description="A zip of logs and settings to send to support. Passwords and keys are never included." size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" icon="download" label="Make export" onClick={() => { setExporting(false); toast('portico-support-2026-09-27.zip is ready', {icon: 'download'}); }} /></>}>
        <div className="stack">
          <Field label="Covering"><Select value={l.export.window} onChange={v => form.set({export: {...l.export, window: v}})} options={['1 hour', '24 hours', '7 days'].map(v => ({value: v, label: `The last ${v}`}))} /></Field>
          <Checkbox checked={l.export.server} onChange={v => form.set({export: {...l.export, server: v}})} label="Server logs" />
          <Checkbox checked={l.export.client} onChange={v => form.set({export: {...l.export, client: v}})} label="App logs sent by devices" />
          <Checkbox checked={l.export.audit} onChange={v => form.set({export: {...l.export, audit: v}})} label="Security audit log" />
          <Checkbox checked={l.export.snapshot} onChange={v => form.set({export: {...l.export, snapshot: v}})} label="Settings snapshot" />
        </div>
      </Dialog>
    </div>
  );
}

function Diagnostics({sec}: {sec: Sec}) {
  const q = useAdmin('diagnostics');
  const [running, setRunning] = useState(false);
  if (!q.data) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Button variant="secondary" icon="refresh" label="Run checks again" busy={running} onClick={() => { setRunning(true); setTimeout(() => { setRunning(false); toast('All checks ran'); }, 1400); }} />} />
      <Panel title="Connectivity and health checks" padded={false}>
        {q.data.checks.map(c => <Row key={c.label} label={<span className="hstack"><HealthDot state={c.state} />{c.label}</span>} help={c.detail} />)}
      </Panel>
      <Panel title="What this server can do" padded={false}>
        {q.data.capabilities.map(c => (
          <Row key={c.id} label={c.label} help={<>{c.reason ?? (c.state === 'available' ? 'Available' : '')}{c.technical ? <details className="tech"><summary>Technical details</summary><code>{c.technical}</code></details> : null}</>}>
            <Badge tone={c.state === 'available' ? 'good' : c.state === 'unavailable' ? 'warn' : 'neutral'}>{c.state === 'available' ? 'Available' : c.state === 'unavailable' ? 'Unavailable' : 'Not checked'}</Badge>
          </Row>
        ))}
      </Panel>
    </div>
  );
}

function Feedback({sec}: {sec: Sec}) {
  const services = useServices();
  const q = useAdmin('feedback');
  const [open, setOpen] = useState<FeedbackRow | null>(null);
  const [reply, setReply] = useState('');
  const [status, setStatus] = useState<FeedbackRow['status']>('new');
  const [filter, setFilter] = useState<'open' | 'all'>('open');
  if (!q.data) return <AdminSkeleton />;
  const list = q.data.filter(f => filter === 'all' || f.status === 'new' || f.status === 'looking');
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} actions={<Segmented size="sm" label="Show" value={filter} onChange={setFilter} options={[{value: 'open', label: 'Open'}, {value: 'all', label: 'All'}]} />} />
      {!list.length ? <StateView icon="message" title="No open reports" body="When someone chooses Report a problem, it lands here." /> : (
        <Panel padded={false}>
          {list.map(f => (
            <button key={f.id} type="button" className="fb-row" onClick={() => { setOpen(f); setReply(f.reply); setStatus(f.status); }}>
              <span className="note-icon"><Icon name="flag" size={16} /></span>
              <span className="fb-text"><strong>{f.category}{f.title ? ` · ${f.title}` : ''}</strong><span className="clamp-2">{f.message}</span><span className="faint small">{f.from} · {relativeTime(f.at)}{f.reply ? ' · Replied' : ''}</span></span>
              <StatusBadge state={f.status} />
            </button>
          ))}
        </Panel>
      )}
      <Dialog open={!!open} onOpenChange={o => !o && setOpen(null)} title={open ? `${open.category}${open.title ? ` · ${open.title}` : ''}` : ''} description={open ? `From ${open.from}, ${relativeTime(open.at).toLowerCase()}` : undefined} size="md" footer={<><DialogClose asChild><Button variant="ghost" label="Close" /></DialogClose><Button variant="primary" label={reply && reply !== open?.reply ? 'Save and send reply' : 'Save'} onClick={() => open && void services.admin.set('feedback', q.data!.map(f => (f.id === open.id ? {...f, reply, status} : f))).then(() => { toast(reply && reply !== open.reply ? `Reply sent to ${open.from.split(' ')[0]}` : 'Saved'); setOpen(null); })} /></>}>
        {open ? (
          <div className="stack">
            <blockquote className="fb-quote">{open.message}</blockquote>
            <div className="field"><span className="field-label">Status</span><Segmented size="sm" label="Status" value={status} onChange={setStatus} options={[{value: 'new', label: 'New'}, {value: 'looking', label: 'Looking into it'}, {value: 'fixed', label: 'Fixed'}, {value: 'wontFix', label: 'Won’t fix'}]} /></div>
            <Field label="Reply" optional help="They get it as a notification."><TextArea value={reply} onChange={e => setReply(e.target.value)} rows={3} /></Field>
            <p className="faint small">Playback diagnostics were attached: Direct play · 1080p · Safari 19 · 212 Mb/s.</p>
          </div>
        ) : null}
      </Dialog>
    </div>
  );
}

function Alerts({sec}: {sec: Sec}) {
  const form = useAdminForm('alerts');
  const a = form.data;
  if (!a) return <AdminSkeleton />;
  return (
    <div className="adm-sections">
      <SectionHead title={sec.label} lede={sec.lede} />
      <Panel title="Storage" padded={false}>
        <Row label="Warn when free space falls below" help="Shown on Overview and sent to admins."><Stepper label="Warning threshold" value={a.storageWarning} onChange={v => form.set({storageWarning: v})} min={5} max={40} suffix="%" /></Row>
        <Row label="Urgent when free space falls below" help="Recordings and conversions may start failing."><Stepper label="Critical threshold" value={a.storageCritical} onChange={v => form.set({storageCritical: v})} min={1} max={20} suffix="%" /></Row>
      </Panel>
      <Panel title="Certificate and connection" padded={false}>
        <Row label="Warn before the certificate expires" help="Portico renews it on its own; this is for when it can’t."><Stepper label="Certificate warning" value={a.certificateDays} onChange={v => form.set({certificateDays: v})} min={3} max={60} suffix="days" /></Row>
        <Row label="Tell me when the server is unreachable for" help="Sent through your Portico Account."><Stepper label="Offline alert" value={a.offlineMinutes} onChange={v => form.set({offlineMinutes: v})} min={5} max={240} step={5} suffix="min" /></Row>
        <Row label="Tell me when a library scan fails"><Switch checked={a.failedScans} onChange={v => form.set({failedScans: v})} label="Failed scans" /></Row>
      </Panel>
      <UnsavedBar form={form} />
    </div>
  );
}

