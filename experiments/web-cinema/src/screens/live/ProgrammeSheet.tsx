import {useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Channel, Programme} from '../../data/types';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {Dialog} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {ChoiceCards, Field, Select, Stepper} from '../../ui/Controls';
import {Badge, Notice, Progress, toast} from '../../ui/Feedback';
import {Art} from '../../ui/Media';
import {Facts} from '../detail/parts';
import {timeOfDay} from '../shared/items';
import {programmeArt} from './Channels';

/** A program in the guide: watch, record once or as a series, remind, and the channel's options. */
export function ProgrammeSheet({programme: p, channel, onClose}: {programme: Programme; channel?: Channel; onClose: () => void}) {
  const services = useServices();
  const version = useDataVersion();
  const navigate = useNavigate();
  const session = useSession();
  const recs = useQuery(`live.recordings.${version}`, () => services.live.recordings());
  const reminders = useQuery(`live.reminders.${version}`, () => services.live.reminders());
  const [series, setSeries] = useState(false);
  const [newOnly, setNewOnly] = useState<'new' | 'all'>('new');
  const [where, setWhere] = useState<'this' | 'any'>('this');
  const [keep, setKeep] = useState('all');
  const [early, setEarly] = useState(0);
  const [late, setLate] = useState(2);
  const now = Date.now();
  const live = p.start <= now && p.end > now;
  const past = p.end <= now;
  const rec = recs.data?.find(r => r.programmeId === p.id && r.state !== 'cancelled');
  const reminded = reminders.data?.includes(p.id);
  const dvr = session.profile?.limits.dvr !== false;
  const day = new Date(p.start).toDateString() === new Date().toDateString() ? 'Today' : new Date(p.start).toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={p.title} kind="sheet" size="md" hideTitle className="prog-sheet">
      <div className="prog-hero">
        <Art src={programmeArt(p)} tint={channel?.tint} ratio="16/9" eager />
        <div className="ep-hero-scrim" />
        {channel ? <span className="prog-logo"><Art src={channel.logo} tint={channel.tint} /></span> : null}
      </div>
      <div className="ep-body">
        <div className="eyebrow">{channel ? `${channel.number} · ${channel.name}` : 'Live TV'}{live ? ' · On now' : ''}</div>
        <h2 className="display ep-title">{p.title}</h2>
        {p.episodeTitle ? <p className="prog-ep">{p.episodeTitle}</p> : null}
        <Facts items={[`${day}, ${timeOfDay(p.start)} – ${timeOfDay(p.end)}`, `${Math.round((p.end - p.start) / 60000)} min`, p.genre, p.rating]} badges={<>{p.isNew ? <Badge tone="accent">New</Badge> : null}{rec ? <Badge tone="bad" icon="record">{rec.state === 'recording' ? 'Recording' : rec.state === 'conflicted' ? 'Conflict' : 'Will record'}</Badge> : null}</>} />
        {live ? <Progress value={(now - p.start) / (p.end - p.start)} tone="bad" label="Progress" /> : null}
        <p className="ep-syn">{p.synopsis || 'No description.'}</p>
        {rec?.state === 'conflicted' ? <Notice tone="warn" title="Tuner conflict">Other recordings are using every tuner at this time. <button type="button" className="link" onClick={() => void navigate({to: '/channels/$entry', params: {entry: 'recordings'}, search: {tab: 'upcoming'}})}>See the conflict</button></Notice> : null}
        <div className="action-bar">
          <div className="action-primary">
            {live ? <Button variant="primary" size="lg" icon="play" label="Watch live" onClick={() => { onClose(); player.playLive(p.channelId); }} /> : past ? <Button variant="primary" size="lg" icon="play" label="Watch channel" onClick={() => { onClose(); player.playLive(p.channelId); }} /> : (
              <Button variant={reminded ? 'secondary' : 'primary'} size="lg" icon={reminded ? 'bellOff' : 'bell'} label={reminded ? 'Stop reminding' : 'Remind me'} onClick={() => void services.live.toggleReminder(p.id).then(on => toast(on ? 'You’ll get a reminder 2 minutes before it starts, while Portico is open.' : 'Reminder removed', {icon: 'bell'}))} />
            )}
          </div>
          {dvr && !past ? (
            <div className="action-secondary">
              {rec ? <Button variant="danger" icon="close" label={rec.state === 'recording' ? 'Stop recording' : 'Cancel recording'} onClick={() => void services.live.cancelRecording(rec.id).then(() => toast('Recording canceled'))} /> : <Button variant="glass" icon="record" label="Record" onClick={() => void services.live.record(p.id, {series: false}).then(() => toast(`Recording “${p.title}”`, {icon: 'record', action: {label: 'View', onClick: () => void navigate({to: '/channels/$entry', params: {entry: 'recordings'}})}}))} />}
              {p.series ? <Button variant="glass" icon="repeat" label="Record series…" onClick={() => setSeries(s => !s)} /> : null}
            </div>
          ) : null}
        </div>
        {series ? (
          <div className="series-form panel is-padded">
            <h3 className="panel-title">Record series</h3>
            <ChoiceCards label="Which episodes" value={newOnly} onChange={setNewOnly} options={[{value: 'new', title: 'New episodes only', body: 'Skips reruns.'}, {value: 'all', title: 'All episodes', body: 'Every airing, new or not.'}]} />
            <ChoiceCards label="Channel" value={where} onChange={setWhere} options={[{value: 'this', title: 'This channel', body: channel?.name}, {value: 'any', title: 'Any channel', body: 'Wherever it airs.'}]} />
            <div className="grid-3" style={{alignItems: 'end'}}>
              <Field label="Keep"><Select value={keep} onChange={setKeep} options={[{value: 'all', label: 'All episodes'}, {value: '3', label: 'Latest 3'}, {value: '5', label: 'Latest 5'}, {value: '10', label: 'Latest 10'}]} /></Field>
              <div className="field"><span className="field-label">Start early</span><Stepper label="Start early" value={early} onChange={setEarly} max={30} suffix="min" /></div>
              <div className="field"><span className="field-label">End late</span><Stepper label="End late" value={late} onChange={setLate} max={120} step={1} suffix="min" /></div>
            </div>
            <div className="hstack" style={{justifyContent: 'flex-end'}}>
              <Button variant="ghost" label="Cancel" onClick={() => setSeries(false)} />
              <Button variant="primary" icon="record" label="Record series" onClick={() => void services.live.record(p.id, {series: true, newOnly: newOnly === 'new', keep: keep === 'all' ? 'all' : Number(keep), startEarly: early, endLate: late}).then(() => { setSeries(false); toast('Series recording set up. New airings are added as the guide updates.', {icon: 'record', action: {label: 'View', onClick: () => void navigate({to: '/channels/$entry', params: {entry: 'recordings'}, search: {tab: 'rules'}})}}); })} />
            </div>
          </div>
        ) : null}
        {channel ? (
          <div className="prog-more">
            <Button variant="ghost" icon="live" label={`More for ${channel.name}`} onClick={() => { onClose(); void navigate({to: '/channels/$entry', params: {entry: 'all'}, search: {view: 'list', group: channel.group}}); }} />
            <Button variant="ghost" icon="star" label={channel.favorite ? 'Remove from Favorites' : 'Add to Favorites'} onClick={() => void services.live.setChannel(channel.id, {favorite: !channel.favorite}).then(() => toast(channel.favorite ? 'Removed from Favorites' : 'Added to Favorites'))} />
            <Button variant="ghost" icon="eyeOff" label="Hide channel" onClick={() => void services.live.setChannel(channel.id, {hidden: true}).then(() => { toast(`${channel.name} is hidden`); onClose(); })} />
          </div>
        ) : null}
      </div>
    </Dialog>
  );
}
