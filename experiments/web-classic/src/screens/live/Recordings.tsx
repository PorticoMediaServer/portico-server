import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Recording, SeriesRule} from '@/data/types';
import {usePlayer} from '@/player/PlayerProvider';
import {useSession} from '@/app/session';
import {clock, day, plural} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, EmptyState, ErrorState, Meter, Skeleton, Surface, type Tone} from '@/ui/Bits';
import {Checkbox, Select, Switch} from '@/ui/Form';
import {ConfirmDialog, Dialog, Menu, useToast} from '@/ui/Overlay';
import {LinkTabs} from '@/ui/Tabs';
import {useTitle} from '@/ui/hooks';
import {useRouteSearch} from '../common';
import s from './Live.module.css';

type Tab = 'upcoming' | 'recorded' | 'series' | 'conflicts' | 'history' | 'storage';
const STATE: Record<Recording['state'], {label: string; tone: Tone}> = {
  scheduled: {label: 'Scheduled', tone: 'neutral'}, conflicted: {label: 'Conflict', tone: 'warn'}, recording: {label: 'Recording now', tone: 'bad'},
  completed: {label: 'Recorded', tone: 'good'}, incomplete: {label: 'Partly recorded', tone: 'warn'}, failed: {label: 'Didn’t record', tone: 'bad'}, cancelled: {label: 'Cancelled', tone: 'neutral'},
};

/** The DVR: what will record, what did, series rules, conflicts, history and recording space. */
export function Recordings() {
  const search = useRouteSearch<{tab?: string}>();
  const q = useQuery(c => c.live.recordings(), [], ['dvr', 'live']);
  const session = useSession();
  useTitle('Recordings');
  const tab = (['upcoming', 'recorded', 'series', 'conflicts', 'history', 'storage'].includes(search.tab ?? '') ? search.tab : 'upcoming') as Tab;
  if (session.profile?.limits?.dvr === false) return <EmptyState icon="lock" title="Recording isn’t available on this profile" body="The main profile can allow recording in Settings › Account › Profiles." />;
  const d = q.data;
  const tabs = [
    {id: 'upcoming', label: 'Upcoming', count: d?.upcoming.length}, {id: 'recorded', label: 'Recorded', count: d?.recorded.length}, {id: 'series', label: 'Series', count: d?.rules.length},
    ...(d?.conflicts.length ? [{id: 'conflicts', label: 'Conflicts', count: d.conflicts.length}] : []), {id: 'history', label: 'History'}, {id: 'storage', label: 'Space'},
  ];
  return (
    <div style={{padding: '28px 0 72px'}}>
      <div className={s.head}><div className={s.headText}><div className="t-label" style={{marginBottom: 6}}>Live TV</div><h1 className="t-title">Recordings</h1></div></div>
      <div style={{padding: '0 var(--gutter)', marginBottom: 20}}><LinkTabs label="Recordings" current={tab} tabs={tabs.map(t => ({...t, href: `/channels/recordings?tab=${t.id}`}))} /></div>
      {q.error ? <ErrorState onRetry={q.refresh} /> : !d ? <div className={s.recList} style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 8}}>{Array.from({length: 5}, (_, i) => <Skeleton key={i} height={72} />)}</div>
        : tab === 'upcoming' ? <RecList list={d.upcoming} empty={<EmptyState icon="recordings" title="Nothing scheduled" body="Record a program or a whole series from the guide." actions={[{label: 'Open the guide', to: '/channels'}]} />} />
        : tab === 'recorded' ? <RecList list={d.recorded} empty={<EmptyState icon="recordings" title="No recordings yet" body="Finished recordings appear here, ready to watch." />} />
        : tab === 'series' ? <Rules rules={d.rules} />
        : tab === 'conflicts' ? <RecList list={d.conflicts} conflicts empty={<EmptyState icon="check" title="No conflicts" body="Every scheduled recording has a tuner." />} />
        : tab === 'history' ? <RecList list={d.history} empty={<EmptyState icon="history" title="No history yet" />} />
        : <Storage storage={d.storage} />}
    </div>
  );
}

function RecList({list, empty, conflicts}: {list: Recording[]; empty: React.ReactNode; conflicts?: boolean}) {
  const client = useClient();
  const player = usePlayer();
  const toast = useToast();
  const [confirm, setConfirm] = useState<Recording>();
  const [conflict, setConflict] = useState<Recording>();
  if (!list.length) return <>{empty}</>;
  return (
    <div className={s.recList} role="list">
      {list.map(r => {
        const st = STATE[r.state];
        const done = r.state === 'completed' || r.state === 'incomplete';
        return (
          <div key={r.id} className={s.recRow} role="listitem">
            <Artwork src={r.image} title={r.title} shape="wide" />
            <div style={{minWidth: 0}}>
              <div className="t-strong clamp-1">{r.title}</div>
              <div className="t-caption c-3 clamp-1">{r.episodeTitle ? `${r.episodeTitle} · ` : ''}{r.channelName} · {day(r.start)}, {clock(r.start)}–{clock(r.end)}{r.size ? ` · ${r.size}` : ''}</div>
              {r.reason ? <div className="t-caption" style={{color: 'var(--amber)', marginTop: 2}}>{r.reason}</div> : null}
              <div className={s.recBadges}><Badge tone={st.tone} dot={r.state === 'recording'}>{st.label}</Badge>{r.seriesRuleId ? <Badge>Series</Badge> : null}{r.kept ? <Badge tone="accent">Kept</Badge> : null}</div>
            </div>
            <span style={{display: 'flex', gap: 4}}>
              {done || r.state === 'recording' ? <IconButton variant="secondary" icon="play" label={`Play ${r.title}`} onClick={() => player.play([{id: r.id, kind: 'recording', title: r.title, subtitle: r.channelName, image: r.image, backdrop: r.image, shape: 'wide', duration: (r.end - r.start) / 1000}], 0)} /> : null}
              {conflicts ? <Button size="sm" variant="primary" label="Resolve" onClick={() => setConflict(r)} /> : null}
              <Menu label={`More for ${r.title}`} items={[
                ...(done ? [{id: 'keep', label: r.kept ? 'Allow automatic deletion' : 'Keep until I delete it', icon: 'lock' as const, onSelect: async () => { await client.live.keepRecording(r.id, !r.kept); toast.show(r.kept ? 'This recording can be removed automatically' : 'Kept until you delete it', {icon: 'lock'}); }}] : []),
                ...(r.state === 'scheduled' || r.state === 'recording' || r.state === 'conflicted' ? [{id: 'cancel', label: r.state === 'recording' ? 'Stop recording' : 'Don’t record', icon: 'close' as const, onSelect: () => setConfirm(r)}] : []),
                ...(done || r.state === 'failed' ? [{id: 'delete', label: 'Delete recording', icon: 'trash' as const, danger: true, onSelect: () => setConfirm(r)}] : []),
              ]} trigger={<IconButton variant="ghost" icon="more" label={`More for ${r.title}`} />} />
            </span>
          </div>
        );
      })}
      {confirm ? (
        <ConfirmDialog open onOpenChange={o => !o && setConfirm(undefined)} title={confirm.state === 'completed' || confirm.state === 'incomplete' || confirm.state === 'failed' ? `Delete “${confirm.title}”?` : confirm.state === 'recording' ? `Stop recording “${confirm.title}”?` : `Don’t record “${confirm.title}”?`}
          body={confirm.state === 'recording' ? 'What’s recorded so far is kept.' : confirm.seriesRuleId && confirm.state === 'scheduled' ? 'Only this airing is skipped. The series keeps recording.' : confirm.state === 'completed' ? 'The recording is deleted from the server for everyone.' : undefined}
          action={confirm.state === 'recording' ? 'Stop' : confirm.state === 'scheduled' || confirm.state === 'conflicted' ? 'Don’t record' : 'Delete'} tone="danger"
          onConfirm={async () => { if (['completed', 'incomplete', 'failed'].includes(confirm.state)) await client.live.deleteRecording(confirm.id); else await client.live.cancelRecording(confirm.id); toast.show('Done', {icon: 'check'}); setConfirm(undefined); }} />
      ) : null}
      {conflict ? (
        <Dialog open onOpenChange={o => !o && setConflict(undefined)} title="Not enough tuners" description={`“${conflict.title}” overlaps other recordings, and every tuner is taken then.`} width={480} footer={<>
          <Button variant="ghost" label="Skip this one" onClick={async () => { await client.live.resolveConflict(conflict.id, 'skip'); setConflict(undefined); toast.show(`Won’t record “${conflict.title}”`, {icon: 'check'}); }} />
          <Button variant="primary" label="Record this instead" onClick={async () => { await client.live.resolveConflict(conflict.id, 'record'); setConflict(undefined); toast.show(`Recording “${conflict.title}”. The other overlapping recording was skipped.`, {icon: 'record'}); }} />
        </>}>
          <p className="c-2">{day(conflict.start)}, {clock(conflict.start)}–{clock(conflict.end)} on {conflict.channelName}. Recording this one means skipping the lowest-priority overlapping recording. Series priority is set under Series.</p>
        </Dialog>
      ) : null}
    </div>
  );
}

function Rules({rules}: {rules: SeriesRule[]}) {
  const [edit, setEdit] = useState<SeriesRule>();
  const [del, setDel] = useState<SeriesRule>();
  const [cancelUpcoming, setCancelUpcoming] = useState(true);
  const client = useClient();
  const toast = useToast();
  if (!rules.length) return <EmptyState icon="recordings" title="No series recordings" body="Choose Record series on any program in the guide." actions={[{label: 'Open the guide', to: '/channels'}]} />;
  return (
    <>
      <div className={s.ruleGrid} role="list">
        {[...rules].sort((a, b) => a.priority - b.priority).map(r => (
          <div key={r.id} className={s.rule} role="listitem">
            <Artwork src={r.image} title={r.title} shape="square" />
            <div style={{minWidth: 0}}>
              <div className="t-strong clamp-1">{r.title}</div>
              <div className="t-caption c-3 clamp-1">{r.channel === 'any' ? 'Any channel' : r.channelName} · {r.newOnly ? 'New episodes' : 'New and reruns'} · Keep {r.keep === 'all' ? 'all' : `latest ${r.keep}`}</div>
              <div className="t-caption c-2 clamp-1">{r.paused ? 'Paused' : `${plural(r.upcoming, 'upcoming')} · ${r.recorded} recorded`}</div>
            </div>
            <Menu label={`${r.title} options`} items={[
              {id: 'edit', label: 'Edit…', icon: 'edit', onSelect: () => setEdit(r)},
              {id: 'pause', label: r.paused ? 'Resume' : 'Pause', icon: r.paused ? 'play' : 'pause', onSelect: async () => { await client.live.updateRule(r.id, {paused: !r.paused}); toast.show(r.paused ? `Recording ${r.title} again` : `Paused ${r.title}`, {icon: 'check'}); }},
              {id: 'up', label: 'Raise priority', icon: 'chevronUp', disabled: r.priority === 1, onSelect: () => void client.live.updateRule(r.id, {priority: r.priority - 1})},
              {id: 'delete', label: 'Delete series recording…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDel(r)},
            ]} trigger={<IconButton variant="ghost" icon="more" label={`${r.title} options`} />} />
          </div>
        ))}
      </div>
      {edit ? <RuleDialog rule={edit} onClose={() => setEdit(undefined)} /> : null}
      {del ? <ConfirmDialog open onOpenChange={o => !o && setDel(undefined)} title={`Stop recording ${del.title}?`} body="Recordings already made are kept." action="Delete series recording" tone="danger" onConfirm={async () => { await client.live.deleteRule(del.id, cancelUpcoming); setDel(undefined); toast.show(`Stopped recording ${del.title}`, {icon: 'check'}); }}><div style={{marginTop: 12}}><Checkbox checked={cancelUpcoming} onChange={setCancelUpcoming} label={`Also cancel ${plural(del.upcoming, 'scheduled recording')}`} /></div></ConfirmDialog> : null}
    </>
  );
}

function RuleDialog({rule, onClose}: {rule: SeriesRule; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [r, setR] = useState(rule);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Series recording: ${rule.title}`} width={500} footer={<>
      <Button variant="ghost" label="Cancel" onClick={onClose} />
      <Button variant="primary" label="Save" onClick={async () => { await client.live.updateRule(rule.id, r); toast.show('Saved', {icon: 'check'}); onClose(); }} />
    </>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 16}}>
        <Select label="Channels" value={r.channel} onChange={v => setR({...r, channel: v})} options={[{value: 'this', label: `Only on ${rule.channelName}`}, {value: 'any', label: 'Any channel'}]} />
        <Checkbox checked={r.newOnly} onChange={v => setR({...r, newOnly: v})} label="New episodes only" />
        <Select label="Keep" value={String(r.keep)} onChange={v => setR({...r, keep: v === 'all' ? 'all' : Number(v)})} options={[{value: 'all', label: 'All episodes'}, ...[1, 3, 5, 10].map(n => ({value: String(n), label: `The latest ${n}`}))]} />
        <div style={{display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12}}>
          <Select label="Start early" value={String(r.startEarly)} onChange={v => setR({...r, startEarly: Number(v)})} options={[0, 1, 2, 5, 10].map(n => ({value: String(n), label: n ? `${n} min` : 'On time'}))} />
          <Select label="End late" value={String(r.endLate)} onChange={v => setR({...r, endLate: Number(v)})} options={[0, 1, 5, 10, 30].map(n => ({value: String(n), label: n ? `${n} min` : 'On time'}))} />
        </div>
        <label style={{display: 'flex', gap: 10, alignItems: 'center'}}><Switch checked={!r.paused} onChange={v => setR({...r, paused: !v})} label="Recording" /><span>Record new airings</span></label>
      </div>
    </Dialog>
  );
}

function Storage({storage}: {storage: {usedGB: number; freeGB: number; totalGB: number; hoursLeft: number; retentionDays: number; pendingDeleteGB: number; writable: boolean}}) {
  const session = useSession();
  const navigate = useNavigate();
  const used = storage.usedGB / storage.totalGB;
  return (
    <div className={s.storage}>
      <Surface>
        <div className="t-sub" style={{marginBottom: 4}}>{storage.freeGB.toLocaleString('en-US')} GB free of {storage.totalGB.toLocaleString('en-US')} GB</div>
        <p className="c-2 t-caption" style={{marginBottom: 14}}>Room for about {storage.hoursLeft} more hours of HD recordings.</p>
        <Meter value={used} tone={used > 0.9 ? 'bad' : used > 0.75 ? 'warn' : undefined} label="Recording space used" />
        <div className={s.legend} style={{marginTop: 12}}><span><i style={{background: 'var(--accent)'}} />Recordings {storage.usedGB} GB</span><span><i style={{background: 'var(--amber)'}} />Due for deletion {storage.pendingDeleteGB} GB</span><span><i style={{background: 'var(--track)'}} />Free</span></div>
      </Surface>
      <Surface>
        <div className="t-strong">Automatic cleanup</div>
        <p className="c-2 t-caption" style={{marginTop: 4}}>Watched recordings are removed after {storage.retentionDays} days unless you keep them. Series rules can keep only the latest episodes.</p>
      </Surface>
      {!storage.writable ? <Surface><div className="t-strong c-danger">The recording folder can’t be written to</div></Surface> : null}
      {session.isAdmin ? <div><Button variant="secondary" icon="settings" label="DVR settings" onClick={() => void navigate({to: '/server/dvr' as never})} /></div> : null}
    </div>
  );
}
