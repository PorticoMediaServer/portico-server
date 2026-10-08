import React, {useEffect, useRef, useState} from 'react';
import {useQuery, useServices} from '../data/hooks';
import type {Channel, Episode, Item, Movie} from '../data/types';
import {player, usePlayer, type PlayerState} from '../app/player';
import {Art} from '../ui/Media';
import {Icon, type IconName} from '../ui/Icon';
import {Spinner, toast} from '../ui/Feedback';
import {Stepper} from '../ui/Controls';
import {cx} from '../ui/cx';
import {episodeCode, formatClock, timeOfDay} from '../screens/shared/items';

export type PanelId = 'options' | 'audio' | 'chapters' | 'queue' | 'playon' | 'guide' | 'shortcuts' | 'info' | 'sleep';

/** The player's side panel: one place for options, tracks, chapters, the queue, Play on and the guide. */
export function PlayerPanel({id, onClose, onPick, item, channel}: {id: PanelId; onClose: () => void; onPick: (p: PanelId) => void; item: Movie | Episode | null; channel?: Channel}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { ref.current?.querySelector<HTMLElement>('button, [tabindex="0"]')?.focus(); }, [id]);
  const titles: Record<PanelId, string> = {options: 'Playback options', audio: 'Audio & subtitles', chapters: 'Chapters', queue: 'Up next', playon: 'Play on', guide: 'Guide', shortcuts: 'Keyboard shortcuts', info: 'Playback info', sleep: 'Sleep timer'};
  return (
    <aside ref={ref} className="player-panel" aria-label={titles[id]} onClick={e => e.stopPropagation()}>
      <div className="pp-head">
        {id === 'info' || id === 'sleep' ? <button type="button" className="p-btn" onClick={() => onPick('options')} aria-label="Back"><Icon name="chevronLeft" size={20} /></button> : null}
        <h2 className="pp-title">{titles[id]}</h2>
        <button type="button" className="p-btn" onClick={onClose} aria-label="Close panel"><Icon name="close" size={20} /></button>
      </div>
      <div className="pp-body">
        {id === 'options' ? <Options onPick={onPick} item={item} /> : null}
        {id === 'audio' ? <Tracks item={item} /> : null}
        {id === 'chapters' ? <Chapters item={item} /> : null}
        {id === 'queue' ? <Queue /> : null}
        {id === 'playon' ? <PlayOn /> : null}
        {id === 'guide' ? <MiniGuide current={channel} /> : null}
        {id === 'shortcuts' ? <Shortcuts /> : null}
        {id === 'info' ? <Info item={item} /> : null}
        {id === 'sleep' ? <Sleep onDone={onClose} /> : null}
      </div>
    </aside>
  );
}

function Opt({label, hint, on, onClick, icon, trailing}: {label: React.ReactNode; hint?: React.ReactNode; on?: boolean; onClick: () => void; icon?: IconName; trailing?: React.ReactNode}) {
  return (
    <button type="button" className={cx('pp-opt', on && 'is-on')} onClick={onClick} aria-pressed={on ?? false}>
      {icon ? <Icon name={icon} size={18} /> : null}
      <span className="pp-opt-text"><strong>{label}</strong>{hint ? <span>{hint}</span> : null}</span>
      {trailing ?? (on ? <Icon name="check" size={18} className="pp-check" /> : null)}
    </button>
  );
}

const QUALITY: {id: PlayerState['quality']; label: string; hint: string}[] = [
  {id: 'auto', label: 'Automatic', hint: 'Best quality your connection allows'},
  {id: 'original', label: 'Original', hint: 'Plays the file as it is'},
  {id: '1080p', label: 'Convert to 1080p', hint: 'About 8 Mb/s'},
  {id: '720p', label: 'Convert to 720p', hint: 'About 4 Mb/s'},
  {id: '480p', label: 'Convert to 480p', hint: 'About 1.5 Mb/s · data saver'},
];
const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];

function Options({onPick, item}: {onPick: (p: PanelId) => void; item: Movie | Episode | null}) {
  const s = usePlayer();
  return (
    <div className="pp-groups">
      <section>
        <h3 className="pp-h">Quality</h3>
        <div className="pp-list">
          {QUALITY.map(q => <Opt key={q.id} label={q.label} hint={q.hint} on={s.quality === q.id} onClick={() => { player.set({quality: q.id}); toast(q.id === 'original' ? 'Playing the original file' : q.id === 'auto' ? 'Quality set to Automatic' : `Converting to ${q.id}`); }} />)}
        </div>
        <p className="pp-note"><Icon name="wifi" size={14} />Connection: home network · up to 4K</p>
      </section>
      <section>
        <h3 className="pp-h">Speed</h3>
        <div className="pp-speeds" role="radiogroup" aria-label="Speed">
          {SPEEDS.map(r => <button key={r} type="button" role="radio" aria-checked={s.rate === r} className={cx('pp-speed', s.rate === r && 'is-on')} onClick={() => player.set({rate: r})}>{r === 1 ? 'Normal' : `${r}×`}</button>)}
        </div>
      </section>
      <section className="pp-list">
        <Opt icon="timer" label="Sleep timer" hint={s.sleep.mode === 'off' ? 'Off' : s.sleep.label} onClick={() => onPick('sleep')} trailing={<Icon name="chevronRight" size={18} />} />
        <Opt icon="info" label="Playback info" hint={s.quality === 'original' || s.quality === 'auto' ? 'Playing the file as it is' : 'Converted for this device'} onClick={() => onPick('info')} trailing={<Icon name="chevronRight" size={18} />} />
        <Opt icon="keyboard" label="Keyboard shortcuts" onClick={() => onPick('shortcuts')} trailing={<Icon name="chevronRight" size={18} />} />
        {item ? <Opt icon="flag" label="Report a problem" onClick={() => import('../screens/shared/actions').then(m => m.actions.feedback(item.id, 'player'))} trailing={<Icon name="chevronRight" size={18} />} /> : null}
      </section>
    </div>
  );
}

function Tracks({item}: {item: Movie | Episode | null}) {
  const s = usePlayer();
  const [finding, setFinding] = useState<'idle' | 'looking' | 'found'>('idle');
  const audio = item?.audioTracks ?? [];
  const subs = item?.subtitleTracks ?? [];
  const origin = (o?: string) => (o === 'embedded' ? 'In the file' : o === 'sidecar' ? 'Beside the file' : o === 'provider' ? 'Downloaded' : 'Uploaded');
  return (
    <div className="pp-groups">
      <section>
        <h3 className="pp-h">Audio</h3>
        <div className="pp-list">
          {audio.length ? audio.map((a, i) => <Opt key={a.id} label={a.label} hint={a.codec} on={(s.audioTrack ?? audio[0]?.id) === a.id || (!s.audioTrack && i === 0)} onClick={() => player.set({audioTrack: a.id})} />) : <p className="pp-note">One audio track</p>}
        </div>
      </section>
      <section>
        <h3 className="pp-h">Subtitles</h3>
        <div className="pp-list">
          <Opt label="Off" on={!s.subtitle} onClick={() => player.set({subtitle: null})} />
          {subs.map(x => <Opt key={x.id} label={x.label} hint={`${x.forced ? 'Forced · ' : ''}${origin(x.origin)}`} on={s.subtitle === x.id} onClick={() => player.set({subtitle: x.id})} />)}
          {finding === 'found' ? <Opt label="English · Hearing impaired" hint="Downloaded · OpenSubtitles" on={s.subtitle === 'found-en'} onClick={() => player.set({subtitle: 'found-en'})} /> : null}
        </div>
        <button type="button" className="pp-link" onClick={() => { setFinding('looking'); setTimeout(() => setFinding('found'), 1200); }} disabled={finding !== 'idle'}>
          {finding === 'looking' ? <><Spinner size={14} />Looking for subtitles…</> : finding === 'found' ? <><Icon name="check" size={15} />Found 1 more in English</> : <><Icon name="search" size={15} />Find subtitles…</>}
        </button>
      </section>
      {s.subtitle ? (
        <section>
          <h3 className="pp-h">Timing</h3>
          <div className="pp-timing">
            <span className="pp-note">Move subtitles earlier or later</span>
            <Stepper label="Subtitle timing" value={s.subtitleOffset} min={-10} max={10} step={0.5} suffix="s" onChange={v => player.set({subtitleOffset: v})} />
          </div>
        </section>
      ) : null}
    </div>
  );
}

function Chapters({item}: {item: Movie | Episode | null}) {
  const s = usePlayer();
  const list = item?.chapters ?? [];
  const current = Math.max(0, list.findLastIndex(c => c.start <= s.position));
  return (
    <ol className="pp-chapters">
      {list.map((c, i) => (
        <li key={i}>
          <button type="button" className={cx('pp-chapter', i === current && 'is-on')} onClick={() => player.seek(c.start)}>
            <Art src={c.art ?? item?.art.backdrop} tint={item?.tint} ratio="16/9" />
            <span className="pp-opt-text"><strong>{i + 1}. {c.title}</strong><span>{formatClock(c.start)}</span></span>
          </button>
        </li>
      ))}
    </ol>
  );
}

function Queue() {
  const services = useServices();
  const s = usePlayer();
  const items = useQuery(`queue.${s.queue.join(',')}`, () => services.catalog.items(s.queue)).data ?? [];
  const byId = new Map(items.map(i => [i.id, i]));
  return (
    <div className="pp-groups">
      <div className="pp-row-tools">
        <button type="button" className={cx('chip', s.shuffle && 'is-on')} onClick={() => player.set({shuffle: !s.shuffle})}><Icon name="shuffle" size={15} />{s.shuffle ? 'Shuffle on' : 'Shuffle off'}</button>
        <button type="button" className={cx('chip', s.repeat !== 'off' && 'is-on')} onClick={() => player.set({repeat: s.repeat === 'off' ? 'all' : s.repeat === 'all' ? 'one' : 'off'})}><Icon name={s.repeat === 'one' ? 'repeatOne' : 'repeat'} size={15} />{s.repeat === 'off' ? 'Repeat off' : s.repeat === 'all' ? 'Repeat all' : 'Repeat this title'}</button>
      </div>
      <QueueList queue={s.queue} index={s.index} byId={byId} />
    </div>
  );
}

export function QueueList({queue, index, byId, compact}: {queue: string[]; index: number; byId: Map<string, Item>; compact?: boolean}) {
  if (queue.length <= 1) return <p className="pp-note">Nothing is queued.</p>;
  return (
    <ol className={cx('queue', compact && 'is-compact')}>
      {queue.map((id, i) => {
        const it = byId.get(id);
        const ep = it?.kind === 'episode' ? (it as Episode) : null;
        return (
          <li key={id + i} className={cx('queue-row', i === index && 'is-now', i < index && 'is-past')}>
            <button type="button" className="queue-main" onClick={() => player.jumpTo(i)} aria-label={i === index ? `Now playing: ${it?.title}` : `Play now: ${it?.title}`}>
              <Art src={it?.art.square ?? it?.art.still ?? it?.art.poster} tint={it?.tint} ratio={it?.art.square ? '1/1' : ep ? '16/9' : '2/3'} className="queue-art" />
              <span className="pp-opt-text"><strong>{it?.title ?? '…'}</strong><span>{i === index ? 'Now playing' : ep ? episodeCode(ep) : it && 'artistName' in it ? (it as {artistName: string}).artistName : ''}</span></span>
            </button>
            {i !== index ? (
              <span className="queue-tools">
                <button type="button" className="p-btn is-sm" onClick={() => player.move(i, i - 1)} disabled={i === 0} aria-label={`Move ${it?.title} earlier`}><Icon name="chevronUp" size={16} /></button>
                <button type="button" className="p-btn is-sm" onClick={() => player.move(i, i + 1)} disabled={i === queue.length - 1} aria-label={`Move ${it?.title} later`}><Icon name="chevronDown" size={16} /></button>
                <button type="button" className="p-btn is-sm" onClick={() => player.removeAt(i)} aria-label={`Remove ${it?.title}`}><Icon name="close" size={16} /></button>
              </span>
            ) : <span className="eq" aria-hidden><i /><i /><i /></span>}
          </li>
        );
      })}
    </ol>
  );
}

const DEVICES = [
  {id: 'tv', name: 'Living Room Apple TV', kind: 'portico' as const, icon: 'tv' as IconName, sub: 'Portico · on this network'},
  {id: 'airplay', name: 'Bedroom HomePod', kind: 'airplay' as const, icon: 'airplay' as IconName, sub: 'AirPlay'},
  {id: 'cast', name: 'Kitchen display', kind: 'cast' as const, icon: 'cast' as IconName, sub: 'Google Cast'},
  {id: 'ipad', name: 'Ava’s iPad', kind: 'portico' as const, icon: 'tablet' as IconName, sub: 'Portico · asks Ava to allow'},
];

function PlayOn() {
  const s = usePlayer();
  const [looking, setLooking] = useState(true);
  useEffect(() => { const t = setTimeout(() => setLooking(false), 900); return () => clearTimeout(t); }, []);
  const pick = (d: (typeof DEVICES)[number]) => {
    player.set({playOn: {device: d.name, kind: d.kind, state: d.id === 'ipad' ? 'waiting' : 'connecting'}});
    setTimeout(() => {
      const cur = player.get().playOn;
      if (!cur || cur.device !== d.name) return;
      if (d.id === 'cast') player.set({playOn: {...cur, state: 'failed'}});
      else if (d.id !== 'ipad') { player.set({playOn: {...cur, state: 'playing'}}); toast(`Playing on ${d.name}`, {icon: 'cast'}); }
    }, 1600);
  };
  return (
    <div className="pp-groups">
      <p className="pp-note">{looking ? <><Spinner size={14} />Looking for screens nearby…</> : 'Screens on this network'}</p>
      <div className="pp-list">
        <Opt icon="monitor" label="This browser" hint="Play here" on={!s.playOn} onClick={() => player.set({playOn: null})} />
        {!looking ? DEVICES.map(d => {
          const cur = s.playOn?.device === d.name ? s.playOn : null;
          return <Opt key={d.id} icon={d.icon} label={d.name} on={cur?.state === 'playing'} onClick={() => pick(d)} hint={cur ? (cur.state === 'connecting' ? 'Connecting…' : cur.state === 'waiting' ? 'Waiting for Ava to allow it on the iPad' : cur.state === 'failed' ? 'Couldn’t start. Check the display is on, then try again.' : 'Playing') : d.sub} trailing={cur?.state === 'connecting' || cur?.state === 'waiting' ? <Spinner size={16} /> : cur?.state === 'failed' ? <Icon name="alert" size={18} className="pp-warn" /> : undefined} />;
        }) : null}
      </div>
      <p className="pp-note">Other Portico apps signed in to this server appear here. AirPlay and Google Cast need the display on the same network.</p>
    </div>
  );
}

function MiniGuide({current}: {current?: Channel}) {
  const services = useServices();
  const channels = useQuery('live.channels', () => services.live.channels()).data ?? [];
  const now = Date.now();
  const guide = useQuery(`guide.now.${Math.floor(now / 600000)}`, () => services.live.guide(now - 3 * 3600_000, now + 3 * 3600_000)).data ?? [];
  const ref = useRef<HTMLOListElement>(null);
  useEffect(() => { ref.current?.querySelector('.is-on')?.scrollIntoView({block: 'center'}); }, [channels.length]);
  return (
    <ol className="mini-guide" ref={ref}>
      {channels.filter(c => !c.hidden).map(c => {
        const on = guide.find(p => p.channelId === c.id && p.start <= now && p.end > now);
        const next = guide.find(p => p.channelId === c.id && on && p.start >= on.end);
        const pct = on ? (now - on.start) / (on.end - on.start) : 0;
        return (
          <li key={c.id}>
            <button type="button" className={cx('mg-row', c.id === current?.id && 'is-on')} onClick={() => player.playLive(c.id)}>
              <span className="mg-logo"><Art src={c.logo} tint={c.tint} ratio="16/9" /></span>
              <span className="pp-opt-text">
                <strong><span className="mg-num">{c.number}</span>{on?.title ?? 'No information'}</strong>
                <span>{next ? `Next: ${next.title} at ${timeOfDay(next.start)}` : c.name}</span>
                {on ? <span className="mg-bar"><span style={{width: `${pct * 100}%`}} /></span> : null}
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}

function Shortcuts() {
  const keys: [string, string][] = [['Space or K', 'Play or pause'], ['← or J', 'Back 10 seconds'], ['→ or L', 'Forward 10 seconds'], ['↑ ↓', 'Volume'], ['M', 'Mute'], ['F', 'Full screen'], ['C', 'Subtitles or captions'], ['N', 'Next episode'], ['Page Up / Down', 'Change channel (live)'], ['Esc', 'Close a panel, then minimize'], ['?', 'This list']];
  return <dl className="shortcuts">{keys.map(([k, v]) => <div key={k}><dt>{k.split(' or ').map((x, i) => <React.Fragment key={x}>{i ? <span className="faint"> or </span> : null}<span className="kbd">{x}</span></React.Fragment>)}</dt><dd>{v}</dd></div>)}</dl>;
}

function Info({item}: {item: Movie | Episode | null}) {
  const s = usePlayer();
  const v = item?.versions.find(x => x.id === s.versionId) ?? item?.versions[0];
  const converting = s.quality !== 'auto' && s.quality !== 'original';
  return (
    <dl className="kv pp-info">
      <div className="kv-row"><dt>Delivery</dt><dd>{converting ? `Converted to ${s.quality}` : 'Playing the file as it is'}</dd></div>
      <div className="kv-row"><dt>Video</dt><dd>{converting ? `H.264 · ${s.quality} · converted` : v?.video ?? '—'}</dd></div>
      <div className="kv-row"><dt>Audio</dt><dd>{v?.audio ?? '—'}{v?.audio.includes('Atmos') ? ' · mixed to stereo for this device' : ''}</dd></div>
      <div className="kv-row"><dt>Subtitles</dt><dd>{s.subtitle ? (converting ? 'Text · shown by the browser' : 'Text · shown by the browser') : 'Off'}</dd></div>
      <div className="kv-row"><dt>Container</dt><dd>{v?.container ?? '—'}</dd></div>
      {converting ? <div className="kv-row"><dt>Converted by</dt><dd>Graphics hardware (VideoToolbox)</dd></div> : null}
      {v?.hdr ? <div className="kv-row"><dt>HDR</dt><dd>{v.hdr}{converting ? ' · converted for this display' : ''}</dd></div> : null}
      <div className="kv-row"><dt>Connection</dt><dd>Home network · 212 Mb/s</dd></div>
      <div className="kv-row"><dt>Buffer</dt><dd>38 s ahead</dd></div>
    </dl>
  );
}

export function Sleep({onDone}: {onDone: () => void}) {
  const s = usePlayer();
  const pick = (mode: 'off' | 'minutes' | 'endOfItem' | 'endOfChapter', m?: number) => { player.setSleep(mode, m); toast(mode === 'off' ? 'Sleep timer off' : `Sleep timer: ${mode === 'minutes' ? `${m} minutes` : mode === 'endOfChapter' ? 'end of chapter' : 'end of this title'}`, {icon: 'timer'}); onDone(); };
  return (
    <div className="pp-list">
      <Opt label="Off" on={s.sleep.mode === 'off'} onClick={() => pick('off')} />
      {[15, 30, 45, 60, 90].map(m => <Opt key={m} label={`${m} minutes`} on={s.sleep.mode === 'minutes' && s.sleep.label === `${m} min`} onClick={() => pick('minutes', m)} />)}
      {s.kind === 'book' ? <Opt label="End of chapter" on={s.sleep.mode === 'endOfChapter'} onClick={() => pick('endOfChapter')} /> : null}
      <Opt label={s.kind === 'book' ? 'End of the book' : s.kind === 'audio' ? 'End of this song' : 'End of this title'} on={s.sleep.mode === 'endOfItem'} onClick={() => pick('endOfItem')} />
    </div>
  );
}

