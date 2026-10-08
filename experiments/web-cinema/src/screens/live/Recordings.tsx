import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Channel, Recording, RecordingRule} from '../../data/types';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {Page, PageHeader, Panel, Tabs} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Select, Stepper, Switch} from '../../ui/Controls';
import {Badge, Notice, Progress, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {ConfirmDialog, Dialog, DialogClose, Menu} from '../../ui/Overlay';
import {Checkbox} from '../../ui/Controls';
import {cx} from '../../ui/cx';
import {plural, timeOfDay} from '../shared/items';
import {programmeArt} from './Channels';

const TABS = [{value: 'upcoming', label: 'Upcoming'}, {value: 'recorded', label: 'Recorded'}, {value: 'rules', label: 'Series rules'}, {value: 'history', label: 'History'}, {value: 'storage', label: 'Storage'}] as const;
const STATE: Record<Recording['state'], {label: string; tone: 'neutral' | 'accent' | 'good' | 'warn' | 'bad' | 'live'}> = {
  scheduled: {label: 'Scheduled', tone: 'accent'}, conflicted: {label: 'Conflict', tone: 'warn'}, recording: {label: 'Recording', tone: 'live'}, completed: {label: 'Recorded', tone: 'good'},
  incomplete: {label: 'Partly recorded', tone: 'warn'}, failed: {label: 'Didn’t record', tone: 'bad'}, cancelled: {label: 'Canceled', tone: 'neutral'},
};

/** Recordings (the DVR): what's coming, what's recorded, series rules, what happened, and the space. */
export function Recordings({tab: initial}: {tab?: string}) {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const session = useSession();
  const tab = (TABS.some(t => t.value === initial) ? initial : 'upcoming') as (typeof TABS)[number]['value'];
  const recs = useQuery(`live.recordings.${version}`, () => services.live.recordings());
  const rules = useQuery(`live.rules.${version}`, () => services.live.rules());
  const channels = useQuery('live.channels', () => services.live.channels());
  const storage = useQuery(`live.storage.${version}`, () => services.live.storage());
  const ch = (id: string) => channels.data?.find(c => c.id === id);
  const all = recs.data ?? [];
  const upcoming = all.filter(r => r.state === 'scheduled' || r.state === 'conflicted' || r.state === 'recording').sort((a, b) => a.start - b.start);
  const recorded = all.filter(r => r.state === 'completed' || r.state === 'incomplete').sort((a, b) => b.start - a.start);
  const history = all.filter(r => r.state === 'failed' || r.state === 'cancelled' || r.state === 'incomplete').sort((a, b) => b.start - a.start);
  const conflicts = upcoming.filter(r => r.state === 'conflicted');
  if (session.profile?.limits.dvr === false) return <Page title="Recordings"><StateView icon="lock" title="Recording is off for this profile" body="The main profile can turn it on in Settings › Profiles." /></Page>;
  return (
    <Page title="Recordings">
      <PageHeader eyebrow="DVR" title="Recordings" lede={storage.data ? `${storage.data.usedGB.toLocaleString('en-US')} GB used · about ${storage.data.hoursLeft.toLocaleString('en-US')} hours of space left` : undefined} actions={<Button variant="secondary" icon="grid" label="Open the guide" onClick={() => void navigate({to: '/channels/$entry', params: {entry: 'all'}})} />} />
      <Tabs label="Recordings" value={tab} onChange={v => void navigate({to: '/channels/$entry', params: {entry: 'recordings'}, search: {tab: v}, replace: true})} tabs={TABS.map(t => ({...t, count: t.value === 'upcoming' ? upcoming.length : t.value === 'recorded' ? recorded.length : t.value === 'rules' ? rules.data?.length : undefined}))} />
      {!recs.data ? <div className="stack">{[0, 1, 2, 3].map(i => <Skeleton key={i} h={84} />)}</div> : null}
      {recs.data && tab === 'upcoming' ? (
        <>
          {conflicts.length ? <Notice tone="warn" title={conflicts.length === 1 ? 'One recording can’t record' : `${conflicts.length} recordings can’t record`} className="mb">Every tuner is busy at those times. Choose what to skip, or record it instead of another.</Notice> : null}
          {upcoming.length ? <RecList list={upcoming} ch={ch} /> : <StateView icon="record" title="Nothing scheduled" body="Choose Record on anything in the guide, or set up a series." action={{label: 'Open the guide', onClick: () => void navigate({to: '/channels/$entry', params: {entry: 'all'}})}} />}
        </>
      ) : null}
      {recs.data && tab === 'recorded' ? (recorded.length ? <RecordedGrid list={recorded} ch={ch} /> : <StateView icon="film" title="No recordings yet" body="Finished recordings appear here, ready to watch." />) : null}
      {tab === 'rules' ? (rules.data?.length ? <Rules rules={rules.data} ch={ch} /> : rules.data ? <StateView icon="repeat" title="No series rules" body="In the guide, choose Record series on a show to record every new episode." /> : null) : null}
      {recs.data && tab === 'history' ? (history.length ? <RecList list={history} ch={ch} history /> : <StateView icon="history" title="Nothing to report" body="Recordings that were canceled or didn’t finish are listed here with the reason." />) : null}
      {tab === 'storage' && storage.data ? (
        <div className="grid-2">
          <Panel title="Recording space">
            <div className="storage-big"><strong>{storage.data.usedGB.toLocaleString('en-US')} GB</strong><span>of {storage.data.totalGB.toLocaleString('en-US')} GB</span></div>
            <Progress value={storage.data.usedGB / storage.data.totalGB} label="Recording space used" />
            <p className="faint small" style={{marginTop: 10}}>About {storage.data.hoursLeft.toLocaleString('en-US')} hours left at typical HD quality. {storage.data.pendingDeleteGB} GB is waiting to be removed.</p>
          </Panel>
          <Panel title="Keeping recordings">
            <p className="muted">Recordings are kept for {storage.data.retentionDays} days unless you keep them, or a series rule keeps a set number. When space runs low the oldest watched recordings go first.</p>
            {session.manager ? <Button variant="secondary" icon="settings" label="DVR settings" onClick={() => void navigate({to: '/server/$section', params: {section: 'live-tv'}, search: {tab: 'dvr'}})} /> : null}
          </Panel>
        </div>
      ) : null}
    </Page>
  );
}

function RecList({list, ch, history}: {list: Recording[]; ch: (id: string) => Channel | undefined; history?: boolean}) {
  const services = useServices();
  const [cancel, setCancel] = useState<Recording | null>(null);
  const day = (t: number) => {
    const d = new Date(t);
    const today = new Date();
    if (d.toDateString() === today.toDateString()) return 'Today';
    if (d.toDateString() === new Date(Date.now() + 86400000).toDateString()) return 'Tomorrow';
    if (d.toDateString() === new Date(Date.now() - 86400000).toDateString()) return 'Yesterday';
    return d.toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
  };
  return (
    <ul className="rec-list">
      {list.map(r => {
        const c = ch(r.channelId);
        const s = STATE[r.state];
        return (
          <li key={r.id} className={cx('rec-row', r.state === 'conflicted' && 'is-conflict')}>
            <Art src={programmeArt(r)} tint={c?.tint} ratio="16/9" className="rec-art" />
            <div className="rec-text">
              <strong>{r.title}{r.episodeTitle ? <span className="faint"> · {r.episodeTitle}</span> : null}</strong>
              <span className="faint small">{day(r.start)}, {timeOfDay(r.start)} – {timeOfDay(r.end)} · {c ? `${c.number} ${c.name}` : 'Channel'}{r.ruleId ? ' · Series' : ''}</span>
              {r.reason ? <span className={cx('small', r.state === 'failed' || r.state === 'conflicted' ? 'rec-reason is-warn' : 'rec-reason')}>{r.reason}</span> : null}
            </div>
            <Badge tone={s.tone} icon={r.state === 'recording' ? 'record' : undefined}>{s.label}</Badge>
            <div className="rec-actions">
              {r.state === 'conflicted' ? (
                <>
                  <Button size="sm" variant="secondary" label="Skip this" onClick={() => void services.live.resolveConflict(r.id, 'skip').then(() => toast('Skipped. The other recordings keep their tuners.'))} />
                  <Button size="sm" variant="primary" label="Record instead" onClick={() => void services.live.resolveConflict(r.id, 'record').then(() => toast('Recording this instead; the lowest-priority recording was skipped.'))} />
                </>
              ) : r.state === 'recording' ? <Button size="sm" variant="secondary" icon="play" label="Watch" onClick={() => player.playLive(r.channelId)} /> : null}
              {!history && r.state !== 'conflicted' ? <Button size="sm" variant="ghost" label={r.state === 'recording' ? 'Stop' : 'Cancel'} onClick={() => setCancel(r)} /> : null}
            </div>
          </li>
        );
      })}
      <ConfirmDialog open={!!cancel} onOpenChange={o => !o && setCancel(null)} title={cancel?.state === 'recording' ? 'Stop this recording?' : 'Cancel this recording?'} body={cancel?.state === 'recording' ? 'What’s been recorded so far is kept.' : cancel?.ruleId ? 'Only this airing is canceled; the series rule keeps recording new ones.' : 'It won’t be recorded.'} confirmLabel={cancel?.state === 'recording' ? 'Stop recording' : 'Cancel recording'} danger onConfirm={async () => { if (cancel) await services.live.cancelRecording(cancel.id); toast('Recording canceled'); }} />
    </ul>
  );
}

function RecordedGrid({list, ch}: {list: Recording[]; ch: (id: string) => Channel | undefined}) {
  const services = useServices();
  const [del, setDel] = useState<Recording | null>(null);
  return (
    <div className="rec-grid">
      <div className="rec-cards">
        {list.map(r => (
          <div key={r.id} className="rec-card">
            <button type="button" className="rec-card-main" onClick={() => player.playLive(r.channelId)} aria-label={`Play ${r.title}${r.episodeTitle ? `: ${r.episodeTitle}` : ''}`}>
              <span className="rec-card-art"><Art src={programmeArt(r)} tint={ch(r.channelId)?.tint} ratio="16/9" /><span className="extra-play"><Icon name="play" size={22} fill="currentColor" /></span>{r.watched ? <span className="card-watched"><Icon name="check" size={13} stroke={3} /></span> : null}{r.kept ? <span className="card-tag">Kept</span> : null}</span>
              <span className="card-caption"><span className="card-title">{r.title}</span><span className="card-sub">{r.episodeTitle ? `${r.episodeTitle} · ` : ''}{new Date(r.start).toLocaleDateString('en-US', {month: 'short', day: 'numeric'})} · {r.sizeGB} GB{r.state === 'incomplete' ? ' · Partly recorded' : ''}</span></span>
            </button>
            <Menu label="Recording options" trigger={<button type="button" className="track-more is-shown rec-more" aria-label={`Options for ${r.title}`}><Icon name="more" size={18} /></button>} items={[
              {id: 'keep', label: r.kept ? 'Don’t keep' : 'Keep until I delete it', icon: 'pin', onSelect: () => void services.live.keepRecording(r.id, !r.kept).then(() => toast(r.kept ? 'It can be removed when space runs low' : 'Kept until you delete it'))},
              {id: 'delete', label: 'Delete recording…', icon: 'trash', danger: true, separatorBefore: true, onSelect: () => setDel(r)},
            ]} />
          </div>
        ))}
      </div>
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title="Delete this recording?" body="The file is removed from the server. This can’t be undone." confirmLabel="Delete" onConfirm={async () => { if (del) await services.live.deleteRecording(del.id); toast('Recording deleted'); }} />
    </div>
  );
}

function Rules({rules, ch}: {rules: RecordingRule[]; ch: (id: string) => Channel | undefined}) {
  const services = useServices();
  const [edit, setEdit] = useState<RecordingRule | null>(null);
  const [del, setDel] = useState<RecordingRule | null>(null);
  const [cancelUpcoming, setCancelUpcoming] = useState(true);
  return (
    <div className="rules">
      {rules.map(r => (
        <div key={r.id} className={cx('rule-row', r.paused && 'is-paused')}>
          <Art src={programmeArt(r)} tint={ch(r.channelId ?? '')?.tint} ratio="16/9" className="rec-art" />
          <div className="rec-text">
            <strong>{r.title}</strong>
            <span className="faint small">{r.newOnly ? 'New episodes' : 'All episodes'} · {r.channelId ? ch(r.channelId)?.name ?? 'This channel' : 'Any channel'} · Keep {r.keep === 'all' ? 'all' : `latest ${r.keep}`}{r.startEarly || r.endLate ? ` · ${r.startEarly ? `${r.startEarly} min early` : ''}${r.startEarly && r.endLate ? ', ' : ''}${r.endLate ? `${r.endLate} min late` : ''}` : ''}</span>
            <span className="small">{r.paused ? 'Paused' : `${plural(r.scheduledCount, 'airing')} scheduled`} · Priority {r.priority}</span>
          </div>
          {r.paused ? <Badge>Paused</Badge> : <Badge tone="accent">On</Badge>}
          <div className="rec-actions">
            <Button size="sm" variant="secondary" label="Edit" onClick={() => setEdit(r)} />
            <Button size="sm" variant="ghost" label={r.paused ? 'Resume' : 'Pause'} onClick={() => void services.live.updateRule(r.id, {paused: !r.paused}).then(() => toast(r.paused ? `Recording ${r.title} again` : `${r.title} paused`))} />
            <Button size="sm" variant="ghost" icon="trash" aria-label={`Delete ${r.title} rule`} onClick={() => setDel(r)} />
          </div>
        </div>
      ))}
      {edit ? <RuleEditor rule={edit} onClose={() => setEdit(null)} /> : null}
      <ConfirmDialog open={!!del} onOpenChange={o => !o && setDel(null)} danger title={`Delete the ${del?.title} rule?`} body="No new airings will be scheduled. Recordings you already have stay." confirmLabel="Delete rule" onConfirm={async () => { if (del) await services.live.deleteRule(del.id, cancelUpcoming); toast('Series rule deleted'); }}>
        <div style={{marginTop: 12}}><Checkbox checked={cancelUpcoming} onChange={setCancelUpcoming} label="Also cancel its upcoming recordings" /></div>
      </ConfirmDialog>
    </div>
  );
}

function RuleEditor({rule, onClose}: {rule: RecordingRule; onClose: () => void}) {
  const services = useServices();
  const [draft, setDraft] = useState(rule);
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Edit ${rule.title}`} size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Save" onClick={() => void services.live.updateRule(rule.id, draft).then(() => { toast('Series rule saved'); onClose(); })} /></>}>
      <div className="stack">
        <label className="row" style={{padding: 0, border: 0}}><span className="row-text"><span className="row-label">New episodes only</span><span className="row-help">Skip reruns</span></span><Switch checked={draft.newOnly} onChange={v => setDraft({...draft, newOnly: v})} label="New episodes only" /></label>
        <div className="field"><span className="field-label">Keep</span><Select value={String(draft.keep)} onChange={v => setDraft({...draft, keep: v === 'all' ? 'all' : Number(v)})} options={[{value: 'all', label: 'All episodes'}, {value: '3', label: 'Latest 3'}, {value: '5', label: 'Latest 5'}, {value: '10', label: 'Latest 10'}]} /></div>
        <div className="grid-2">
          <div className="field"><span className="field-label">Start early</span><Stepper label="Start early" value={draft.startEarly} onChange={v => setDraft({...draft, startEarly: v})} max={30} suffix="min" /></div>
          <div className="field"><span className="field-label">End late</span><Stepper label="End late" value={draft.endLate} onChange={v => setDraft({...draft, endLate: v})} max={120} suffix="min" /></div>
        </div>
        <div className="field"><span className="field-label">Priority when tuners are busy</span><Stepper label="Priority" value={draft.priority} onChange={v => setDraft({...draft, priority: v})} min={1} max={10} /></div>
      </div>
    </Dialog>
  );
}
