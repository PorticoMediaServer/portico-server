import React, {useEffect, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery, useTopic} from '@/data/DataProvider';
import type {Track} from '@/data/types';
import {formatTime, clock} from '@/app/format';
import {Artwork, Avatar, Badge, Spinner} from '@/ui/Bits';
import {Button, IconButton} from '@/ui/Button';
import {Dialog} from '@/ui/Overlay';
import {Icon, type IconName} from '@/ui/Icon';
import {usePlayer, usePlayerState} from './PlayerProvider';
import s from './Player.module.css';

export function SidePanel({title, onClose, children}: {title: string; onClose: () => void; children: React.ReactNode}) {
  const ref = React.useRef<HTMLElement>(null);
  useEffect(() => { ref.current?.focus({preventScroll: true}); }, []);
  useEffect(() => {
    const on = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.stopPropagation(); onClose(); } };
    window.addEventListener('keydown', on, true);
    return () => window.removeEventListener('keydown', on, true);
  }, [onClose]);
  return (
    <aside ref={ref} tabIndex={-1} className={s.side} role="dialog" aria-label={title} onKeyDown={e => e.stopPropagation()} style={{outline: 'none'}}>
      <div className={s.sideHead}><h2>{title}</h2><IconButton variant="ghost" icon="close" label="Close" onClick={onClose} /></div>
      <div className={s.sideBody}>{children}</div>
    </aside>
  );
}

function Opt({checked, label, sub, onClick}: {checked: boolean; label: string; sub?: string; onClick: () => void}) {
  return (
    <button type="button" role="menuitemradio" aria-checked={checked} className={s.opt} onClick={onClick}>
      <span className={s.optCheck}>{checked ? <Icon name="check" size={16} /> : null}</span>
      <span className={s.optText}>{label}{sub ? <span className={s.optSub}>{sub}</span> : null}</span>
    </button>
  );
}
function Group({label, children}: {label: string; children: React.ReactNode}) {
  return <div className={s.optGroup} role="group" aria-label={label}><div className={s.optLabel}>{label}</div>{children}</div>;
}

const QUALITIES = [
  {id: 'auto', label: 'Automatic', sub: 'Best for this connection'},
  {id: 'original', label: 'Original', sub: 'Plays the file as it is'},
  {id: '1080p', label: '1080p · 12 Mbps'}, {id: '720p', label: '720p · 4 Mbps'}, {id: '480p', label: '480p · 1.5 Mbps'},
];
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];

/** Audio, subtitles (with size, background and timing), quality and speed. Audiobooks get speed and the sleep timer. */
export function OptionsPanel({onClose, audio, subtitles, captionStyle, onCaptionStyle}: {onClose: () => void; audio: Track[]; subtitles: Track[]; captionStyle: {size: string; background: string}; onCaptionStyle: (v: {size: string; background: string}) => void}) {
  const st = usePlayerState();
  const player = usePlayer();
  const book = st.mode === 'book';
  const music = st.mode === 'music';
  return (
    <SidePanel title={book || music ? 'Playback' : 'Audio, subtitles and quality'} onClose={onClose}>
      {!book && !music && audio.length ? <Group label="Audio">{audio.map(t => <Opt key={t.id} checked={st.audio === t.id} label={t.label} sub={[t.codec, t.channels].filter(Boolean).join(' · ')} onClick={() => player.set({audio: t.id})} />)}</Group> : null}
      {!book && !music ? (
        <Group label="Subtitles">
          <Opt checked={st.subtitles === 'off'} label="Off" onClick={() => player.set({subtitles: 'off'})} />
          {subtitles.map(t => <Opt key={t.id} checked={st.subtitles === t.id} label={t.label} sub={t.origin === 'provider' ? `${t.codec} · downloaded` : t.codec} onClick={() => player.set({subtitles: t.id})} />)}
          {st.subtitles !== 'off' ? (
            <>
              <div className={s.optLabel} style={{marginTop: 10}}>Timing</div>
              <div className={s.stepper}>
                <IconButton size="sm" variant="secondary" icon="minus" label="Show subtitles earlier" onClick={() => player.set({subtitleOffset: st.subtitleOffset - 0.25})} />
                <span>{st.subtitleOffset === 0 ? 'In sync' : `${st.subtitleOffset > 0 ? '+' : ''}${st.subtitleOffset.toFixed(2)} s`}</span>
                <IconButton size="sm" variant="secondary" icon="plus" label="Show subtitles later" onClick={() => player.set({subtitleOffset: st.subtitleOffset + 0.25})} />
              </div>
              <div className={s.optLabel} style={{marginTop: 10}}>Size</div>
              {['small', 'medium', 'large', 'huge'].map(v => <Opt key={v} checked={captionStyle.size === v} label={v[0]!.toUpperCase() + v.slice(1)} onClick={() => onCaptionStyle({...captionStyle, size: v})} />)}
              <div className={s.optLabel} style={{marginTop: 10}}>Background</div>
              {[['box', 'Dark box'], ['none', 'Outline only']].map(([v, l]) => <Opt key={v} checked={captionStyle.background === v} label={l!} onClick={() => onCaptionStyle({...captionStyle, background: v!})} />)}
            </>
          ) : null}
        </Group>
      ) : null}
      {!book && !music ? <Group label="Quality">{QUALITIES.map(q => <Opt key={q.id} checked={st.quality === q.id} label={q.label} sub={q.sub} onClick={() => player.set({quality: q.id})} />)}</Group> : null}
      {st.mode !== 'live' ? <Group label="Speed">{SPEEDS.map(v => <Opt key={v} checked={st.speed === v} label={v === 1 ? 'Normal' : `${v}×`} onClick={() => player.set({speed: v})} />)}</Group> : null}
      {book || music ? (
        <Group label="Sleep timer">
          <Opt checked={!st.sleepLabel} label="Off" onClick={() => player.sleep(undefined)} />
          {[15, 30, 45, 60].map(m => <Opt key={m} checked={st.sleepLabel === `${m} min`} label={`${m} minutes`} onClick={() => player.sleep(m)} />)}
          <Opt checked={st.sleepLabel === 'End of chapter'} label={book ? 'End of chapter' : 'End of song'} onClick={() => player.sleep('end')} />
          {st.sleepEndsAt ? <p className="t-caption c-3" style={{padding: '6px 8px'}}>Stops at {clock(st.sleepEndsAt)}.</p> : null}
        </Group>
      ) : null}
    </SidePanel>
  );
}

/** Up next: what's playing, then the queue with reorder and remove. */
export function QueuePanel({onClose, embedded}: {onClose?: () => void; embedded?: boolean}) {
  const st = usePlayerState();
  const player = usePlayer();
  const body = (
    <div role="list" aria-label="Queue">
      {st.queue.map((e, i) => i < st.index ? null : (
        <div key={`${e.id}-${i}`} role="listitem" className={`${s.qRow} ${i === st.index ? s.qNow : ''}`}>
          <button type="button" onClick={() => player.jump(i)} aria-label={`Play ${e.title}`}><Artwork src={e.kind === 'episode' ? e.image : e.image} title={e.title} shape={e.kind === 'episode' || e.kind === 'movie' ? 'wide' : 'square'} /></button>
          <div className={s.qText}><b>{e.title}</b><span>{i === st.index ? 'Now playing' : [e.subtitle, e.caption?.includes('·') ? e.caption : null].filter(Boolean).join(' · ') || formatTime(e.duration ?? 0)}</span></div>
          {i > st.index ? (
            <div className={s.qActions}>
              <IconButton size="sm" variant="ghost" icon="chevronUp" label={`Move ${e.title} up`} disabled={i === st.index + 1} onClick={() => player.move(i, -1)} />
              <IconButton size="sm" variant="ghost" icon="chevronDown" label={`Move ${e.title} down`} disabled={i === st.queue.length - 1} onClick={() => player.move(i, 1)} />
              <IconButton size="sm" variant="ghost" icon="close" label={`Remove ${e.title}`} onClick={() => player.remove(i)} />
            </div>
          ) : <Badge tone="accent">Playing</Badge>}
        </div>
      ))}
      {st.queue.length - st.index <= 1 ? <p className="t-caption c-3" style={{padding: 10}}>Nothing else is queued. Use Play next or Add to queue on any title’s menu.</p> : null}
    </div>
  );
  if (embedded) return body;
  return <SidePanel title="Up next" onClose={onClose!}>{body}</SidePanel>;
}

const DEVICES: {kind: 'portico' | 'cast'; name: string; sub: string; icon: IconName}[] = [
  {kind: 'portico', name: 'Living Room Apple TV', sub: 'Portico · on this network', icon: 'tvDevice'},
  {kind: 'portico', name: 'Bedroom SHIELD', sub: 'Portico · on this network', icon: 'tvDevice'},
  {kind: 'cast', name: 'Kitchen Nest Hub', sub: 'Google Cast', icon: 'cast'},
  {kind: 'cast', name: 'Office Chromecast', sub: 'Google Cast', icon: 'cast'},
];

/** Play on: another Portico device, Google Cast or AirPlay, with the hand-over states and Continue here. */
export function PlayOnDialog({open, onOpenChange}: {open: boolean; onOpenChange: (o: boolean) => void}) {
  const st = usePlayerState();
  const player = usePlayer();
  const [scanning, setScanning] = useState(true);
  const [phase, setPhase] = useState<{name: string; step: 'connecting' | 'approve'}>();
  const [stopped, setStopped] = useState<{name: string; at: number}>();
  useEffect(() => { if (!open) return; setScanning(true); const t = setTimeout(() => setScanning(false), 1100); return () => clearTimeout(t); }, [open]);
  const choose = (kind: 'portico' | 'cast' | 'airplay', name: string) => {
    setPhase({name, step: 'connecting'});
    setTimeout(() => setPhase(kind === 'portico' ? {name, step: 'approve'} : undefined), 1100);
    setTimeout(() => { setPhase(undefined); player.set({cast: {kind, name}}); onOpenChange(false); }, kind === 'portico' ? 2600 : 1300);
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Play on" description="Send what’s playing to another screen or speaker. It carries on here until the other device takes over." width={460}>
      {st.cast ? (
        <div style={{display: 'flex', gap: 12, alignItems: 'center', padding: 12, borderRadius: 12, background: 'var(--accent-soft)', marginBottom: 12}}>
          <Icon name={st.cast.kind === 'cast' ? 'cast' : st.cast.kind === 'airplay' ? 'airplay' : 'tvDevice'} size={22} />
          <span style={{flex: 1}}><span className="t-strong" style={{display: 'block'}}>Playing on {st.cast.name}</span><span className="t-caption c-2">Control it here or with the other device.</span></span>
          <Button size="sm" variant="secondary" label="Stop" onClick={() => { setStopped({name: st.cast!.name, at: st.position}); player.set({cast: undefined, playing: false}); }} />
        </div>
      ) : null}
      {stopped && !st.cast ? (
        <div style={{display: 'flex', gap: 12, alignItems: 'center', padding: 12, borderRadius: 12, background: 'var(--surface-1)', marginBottom: 12}}>
          <span style={{flex: 1}} className="t-caption c-2">Stopped playing on {stopped.name}.</span>
          <Button size="sm" variant="primary" label={`Continue here from ${formatTime(stopped.at)}`} onClick={() => { player.seek(stopped.at); player.setPlaying(true); setStopped(undefined); onOpenChange(false); }} />
        </div>
      ) : null}
      {phase ? <div style={{display: 'flex', gap: 12, alignItems: 'center', padding: '8px 4px 14px'}} role="status"><Spinner size={22} /><span><span className="t-strong" style={{display: 'block'}}>{phase.step === 'approve' ? `Accept on ${phase.name} to continue` : `Connecting to ${phase.name}…`}</span><span className="t-caption c-3">Playback carries on here meanwhile.</span></span></div> : null}
      <div style={{display: 'grid', gap: 4, margin: '0 -8px'}}>
        <button type="button" className={s.opt} onClick={() => choose('airplay', 'Living Room HomePod')}><Icon name="airplay" size={20} /><span className={s.optText}>AirPlay<span className={s.optSub}>Choose a TV or speaker in the next step</span></span></button>
        <div className={s.optLabel} style={{marginTop: 8}}>On this network</div>
        {scanning ? <div style={{display: 'flex', gap: 10, alignItems: 'center', padding: 8}}><Spinner size={18} /><span className="t-caption c-3">Looking for devices…</span></div> : DEVICES.map(d => (
          <button key={d.name} type="button" className={s.opt} aria-current={st.cast?.name === d.name} onClick={() => choose(d.kind, d.name)}>
            <Icon name={d.icon} size={20} /><span className={s.optText}>{d.name}<span className={s.optSub}>{d.sub}</span></span>{st.cast?.name === d.name ? <Badge tone="accent">Playing</Badge> : null}
          </button>
        ))}
      </div>
    </Dialog>
  );
}

/** The live mini guide: channels with what's on now, to switch without leaving the player. */
export function MiniGuide({onClose}: {onClose: () => void}) {
  const st = usePlayerState();
  const player = usePlayer();
  const client = useClient();
  const channels = useQuery(c => c.live.channels(), []);
  return (
    <SidePanel title="Mini guide" onClose={onClose}>
      {!channels.data ? <Spinner /> : channels.data.map(ch => {
        const now = client.live.nowPlaying(ch.id);
        return (
          <button key={ch.id} type="button" className={s.guideRow} aria-current={st.liveChannelId === ch.id} onClick={() => { player.playLive(ch.id, {id: ch.id, kind: 'channel', title: now?.title ?? ch.name, subtitle: `${ch.number} · ${ch.name}`, image: now?.image, shape: 'wide'}); onClose(); }}>
            {ch.logo ? <img src={ch.logo} alt="" className={s.guideLogo} /> : <span className="t-strong">{ch.number}</span>}
            <span style={{minWidth: 0}}><span className="t-strong clamp-1" style={{display: 'block'}}>{ch.number} · {ch.name}</span><span className="t-caption c-3 clamp-1" style={{display: 'block'}}>{now ? `${now.title} · until ${clock(now.end)}` : 'Off air'}</span></span>
          </button>
        );
      })}
    </SidePanel>
  );
}

/** Watch Together pill: who's here and whether everyone is in step. */
export function GroupPill() {
  const client = useClient();
  useTopic(['together']);
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const g = client.together.current();
  if (!g) return null;
  const waiting = g.members.find(x => x.status !== 'inStep');
  return (
    <>
      <button type="button" className={s.pill} onClick={() => setOpen(true)} aria-label={`${g.name}: ${g.members.length} watching. ${waiting ? `Waiting for ${waiting.name}` : 'Everyone is in step'}`}>
        <span className={s.pillFaces}>{g.members.slice(0, 3).map(m => <Avatar key={m.id} name={m.name} art={m.art} size={28} />)}</span>
        <span className={waiting ? s.pillWait : undefined}>{waiting ? `Waiting for ${waiting.name}…` : 'In step'}</span>
      </button>
      <Dialog open={open} onOpenChange={setOpen} title={g.name} description={`${g.members.length} watching · ${g.control === 'host-only' ? 'The host controls playback' : 'Everyone can control playback'}`} width={420} footer={<Button variant="secondary" label="Open Watch Together" onClick={() => { setOpen(false); void navigate({to: '/together' as never}); }} />}>
        {g.members.map(m => (
          <div key={m.id} style={{display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0'}}>
            <Avatar name={m.name} art={m.art} size={34} />
            <span style={{flex: 1}}><span className="t-strong">{m.name}{m.you ? ' (you)' : ''}</span><span className="t-caption c-3" style={{display: 'block'}}>{m.host ? 'Host · ' : ''}{({inStep: 'In step', buffering: 'Loading…', catchingUp: 'Catching up', away: 'Away'} as const)[m.status]}</span></span>
          </div>
        ))}
      </Dialog>
    </>
  );
}
