import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../data/hooks';
import type {Episode, Item, Movie, Programme} from '../data/types';
import {artKeys} from '../data/art';
import {player, usePlayer} from '../app/player';
import {usePrefs} from '../app/prefs';
import {useTogether} from '../app/together';
import {Art} from '../ui/Media';
import {Icon, type IconName} from '../ui/Icon';
import {Tooltip} from '../ui/Overlay';
import {Spinner, toast} from '../ui/Feedback';
import {Slider} from '../ui/Controls';
import {cx} from '../ui/cx';
import {episodeCode, formatClock} from '../screens/shared/items';
import {actions} from '../screens/shared/actions';
import {Timeline} from './Timeline';
import {PlayerPanel, type PanelId} from './Panels';
import {PostPlay} from './PostPlay';
import {GroupPill} from './GroupPill';

const CAPTIONS = [
  'I told you we’d be late.',
  'Then we’d better hurry.',
  'Is anyone else here?',
  'Keep your voice down.',
  'We don’t have much time.',
  'You were right about the door.',
  'Where does this road go?',
  'Nobody’s been this way in years.',
  'Hold on. Listen.',
  'That’s the signal.',
];

/**
 * The video player. There's no video in the fixture build, so the picture is the title's own
 * frames (chapter stills and backdrop) crossfading with a slow drift; everything around it is the
 * real player: timeline with chapter thumbnails, intro and credits, options, queue, Play on,
 * post-play and the live-TV controls.
 */
export function VideoPlayer() {
  const services = useServices();
  const navigate = useNavigate();
  const s = usePlayer();
  const {prefs, update} = usePrefs();
  const live = s.kind === 'live';
  const item = useQuery(s.itemId ? `item.${s.itemId}` : null, () => services.catalog.item(s.itemId!)).data;
  const show = useQuery(item?.kind === 'episode' ? `item.${item.showId}` : null, () => services.catalog.item((item as Episode).showId)).data;
  const channels = useQuery(live ? 'live.channels' : null, () => services.live.channels()).data ?? [];
  const channel = channels.find(c => c.id === s.channelId);
  const now = Date.now();
  const guide = useQuery(live ? `guide.now.${Math.floor(now / 600000)}` : null, () => services.live.guide(now - 3 * 3600_000, now + 3 * 3600_000)).data ?? [];
  const programme = guide.find(p => p.channelId === s.channelId && p.start <= now && p.end > now);
  const [chrome, setChrome] = useState(true);
  const [panel, setPanel] = useState<PanelId | null>(() => takePending() as PanelId | null);
  const idle = useRef<ReturnType<typeof setTimeout>>(undefined);
  const root = useRef<HTMLDivElement>(null);
  const together = useTogether();

  const wake = useCallback(() => {
    setChrome(true);
    clearTimeout(idle.current);
    idle.current = setTimeout(() => { if (player.get().playing && !document.querySelector('.player-panel')) setChrome(false); }, 3200);
  }, []);
  useEffect(() => { wake(); return () => clearTimeout(idle.current); }, [wake]);
  useEffect(() => { if (!s.playing) setChrome(true); }, [s.playing]);
  useEffect(() => {
    const on = (e: Event) => setPanel((e as CustomEvent<PanelId>).detail);
    window.addEventListener('portico:panel', on);
    return () => window.removeEventListener('portico:panel', on);
  }, []);
  useEffect(() => {
    document.body.style.overflow = 'hidden';
    root.current?.focus();
    return () => { document.body.style.overflow = ''; };
  }, []);

  const close = () => { if (document.fullscreenElement) void document.exitFullscreen(); player.stop(); };
  const minimize = () => { if (document.fullscreenElement) void document.exitFullscreen(); player.minimize(); };
  const fullscreen = () => { if (document.fullscreenElement) void document.exitFullscreen(); else void root.current?.requestFullscreen?.().catch(() => undefined); };
  const toggle = (p: PanelId) => setPanel(cur => (cur === p ? null : p));
  const zap = (dir: 1 | -1) => {
    const list = channels.filter(c => !c.hidden);
    const i = list.findIndex(c => c.id === s.channelId);
    const next = list[(i + dir + list.length) % list.length];
    if (next) player.playLive(next.id);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
      if (t.closest('[role=menu],[role=listbox]')) return;
      wake();
      switch (e.key) {
        case ' ': case 'k': if (t.tagName === 'BUTTON' && e.key === ' ') return; e.preventDefault(); player.toggle(); break;
        case 'ArrowLeft': case 'j': if (t.closest('.timeline, .slider')) return; e.preventDefault(); player.skip(-10); break;
        case 'ArrowRight': case 'l': if (t.closest('.timeline, .slider')) return; e.preventDefault(); player.skip(10); break;
        case 'ArrowUp': if (t.closest('.player-panel, .slider')) return; e.preventDefault(); update({volume: Math.min(1, prefs.volume + 0.1), muted: false}); break;
        case 'ArrowDown': if (t.closest('.player-panel, .slider')) return; e.preventDefault(); update({volume: Math.max(0, prefs.volume - 0.1)}); break;
        case 'm': update({muted: !prefs.muted}); break;
        case 'f': fullscreen(); break;
        case 'c': player.set(live ? {captions: !s.captions} : {subtitle: s.subtitle ? null : (item && 'subtitleTracks' in item ? (item as Movie).subtitleTracks[0]?.id ?? null : null)}); break;
        case 'n': if (!live) player.next(); break;
        case '?': toggle('shortcuts'); break;
        case 'Escape':
          if (panel) { setPanel(null); e.preventDefault(); }
          else if (!document.fullscreenElement) { minimize(); e.preventDefault(); }
          break;
        case 'PageUp': if (live) zap(-1); break;
        case 'PageDown': if (live) zap(1); break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const video = item && (item.kind === 'movie' || item.kind === 'homevideo' || item.kind === 'episode') ? (item as Movie | Episode) : null;
  const ep = item?.kind === 'episode' ? (item as Episode) : null;
  const frames = useMemo(() => framesFor(video, live ? programme?.title ?? channel?.id : undefined), [video, channel, live, programme?.title]);
  const chapterIndex = video ? Math.max(0, video.chapters.findLastIndex(c => c.start <= s.position)) : 0;
  const frame = frames.length ? frames[(live ? Math.floor(s.position / 12) : chapterIndex) % frames.length] : undefined;
  const intro = ep?.introEnd && s.position >= 4 && s.position < ep.introEnd;
  const credits = ep?.creditsStart && s.position >= ep.creditsStart && s.position < s.duration - 1;
  const ended = !live && s.duration > 0 && s.position >= s.duration - 1 && !s.playing;
  const hasNext = s.index < s.queue.length - 1;
  const caption = (s.subtitle || (live && s.captions)) && s.playing ? CAPTIONS[Math.floor((s.position + s.subtitleOffset) / 6) % CAPTIONS.length] : null;
  const title = live ? programme?.title ?? channel?.name ?? 'Live TV' : ep ? show?.title ?? '' : item?.title ?? '';
  const subtitle = live ? (channel ? `${channel.number} · ${channel.name}${programme?.episodeTitle ? ` · ${programme.episodeTitle}` : ''}` : '') : ep ? `${episodeCode(ep)} · ${ep.title}` : item?.kind === 'movie' ? String(item.year ?? '') : '';

  return (
    <div
      ref={root}
      className={cx('player', chrome ? 'show-chrome' : 'hide-chrome', panel && 'has-panel', live && 'is-live')}
      role="dialog"
      aria-modal="true"
      aria-label={`Player: ${title}`}
      tabIndex={-1}
      onMouseMove={wake}
      onClick={wake}
      style={{'--tint-c': item?.tint ?? channel?.tint ?? '#409fd4'} as React.CSSProperties}
    >
      <div className="player-picture" onClick={e => { if (e.target === e.currentTarget || (e.target as HTMLElement).closest('.player-frame')) { if (chrome) player.toggle(); } }} onDoubleClick={fullscreen}>
        {frames.map((f, i) => (
          <div key={f + i} className={cx('player-frame', f === frame && 'is-on', s.playing && 'is-moving')}>
            <Art src={f} tint={item?.tint ?? channel?.tint} eager={f === frame} />
          </div>
        ))}
        {!frames.length ? <div className="player-frame is-on player-noframe">{channel ? <Art src={channel.logo} tint={channel.tint} /> : null}</div> : null}
        <div className="player-vignette" />
        {caption ? <p className="player-caption" aria-live="off">{caption}</p> : null}
        {s.buffering ? <div className="player-buffer"><Spinner size={46} label="Loading" /></div> : null}
        {!s.playing && !s.buffering && !ended && !s.ownerStopped ? <button type="button" className="player-bigplay" onClick={() => player.toggle()} aria-label="Play"><Icon name="play" size={40} fill="currentColor" /></button> : null}
      </div>

      <div className="player-top">
        <div className="player-top-left">
          <Tooltip label="Close player"><button type="button" className="p-btn" onClick={close} aria-label="Close player"><Icon name="close" size={24} /></button></Tooltip>
          <div className="player-titles">
            {live ? <span className="live-badge"><span />Live</span> : null}
            <strong className="player-title">{title}</strong>
            {subtitle ? <span className="player-sub">{subtitle}</span> : null}
          </div>
        </div>
        <div className="player-top-right">
          {together.inGroup ? <GroupPill /> : null}
          <Tooltip label="Report a problem"><button type="button" className="p-btn" onClick={() => actions.feedback(s.itemId ?? undefined, 'player')} aria-label="Report a problem"><Icon name="flag" size={20} /></button></Tooltip>
          <Tooltip label={s.playOn ? `Playing on ${s.playOn.device}` : 'Play on another screen'}><button type="button" className={cx('p-btn', s.playOn && 'is-on')} onClick={() => toggle('playon')} aria-label="Play on"><Icon name="cast" size={20} /></button></Tooltip>
        </div>
      </div>

      {s.playOn?.state === 'playing' ? <div className="player-remote"><Icon name="cast" size={34} /><strong>Playing on {s.playOn.device}</strong><span>This screen is now a remote. Use the controls below.</span><button type="button" className="btn btn-glass btn-sm" onClick={() => player.set({playOn: null})}><span className="btn-label">Play here instead</span></button></div> : null}

      {intro ? <button type="button" className="skip-prompt" onClick={() => player.seek(ep!.introEnd!)}>Skip intro<Icon name="next" size={18} /></button> : null}
      {credits && hasNext ? <button type="button" className="skip-prompt" onClick={() => player.next()}>Next episode<Icon name="next" size={18} /></button> : null}
      {credits && !hasNext && !ep ? <button type="button" className="skip-prompt" onClick={() => player.seek(s.duration - 1)}>Skip credits<Icon name="next" size={18} /></button> : null}

      <div className="player-bottom">
        {live ? (
          <LiveBar programme={programme} behind={s.liveBehind} onJump={() => player.jumpToLive()} />
        ) : (
          <Timeline item={video} position={s.position} duration={s.duration} onSeek={t => player.seek(t)} />
        )}
        <div className="player-controls">
          <div className="pc-left">
            <PBtn icon={s.playing ? 'pause' : 'play'} label={s.playing ? 'Pause (space)' : 'Play (space)'} onClick={() => player.toggle()} big />
            <PBtn icon="rewind" label="Back 10 seconds (J)" onClick={() => player.skip(-10)} />
            <PBtn icon="fastForward" label={live ? 'Forward 10 seconds' : 'Forward 10 seconds (L)'} onClick={() => player.skip(10)} disabled={live && s.liveBehind === 0} />
            {live ? (
              <>
                <PBtn icon="chevronUp" label="Previous channel" onClick={() => zap(-1)} />
                <PBtn icon="chevronDown" label="Next channel" onClick={() => zap(1)} />
              </>
            ) : hasNext ? <PBtn icon="next" label="Next (N)" onClick={() => player.next()} /> : null}
            <div className="pc-volume">
              <PBtn icon={prefs.muted || prefs.volume === 0 ? 'mute' : 'volume'} label={prefs.muted ? 'Unmute (M)' : 'Mute (M)'} onClick={() => update({muted: !prefs.muted})} />
              <Slider className="pc-vol-slider" label="Volume" value={prefs.muted ? 0 : Math.round(prefs.volume * 100)} onChange={v => update({volume: v / 100, muted: false})} />
            </div>
            {!live ? <span className="pc-time tabular">{formatClock(s.position)} <span>/ {formatClock(s.duration)}</span></span> : null}
          </div>
          <div className="pc-right">
            {live ? (
              <>
                <PBtn icon="record" label="Record" onClick={() => programme && void services.live.record(programme.id, {series: false}).then(() => toast(`Recording “${programme.title}”`, {icon: 'record'}))} />
                <PBtn icon="captions" label={s.captions ? 'Turn off captions (C)' : 'Turn on captions (C)'} active={s.captions} onClick={() => player.set({captions: !s.captions})} />
                <PBtn icon="grid" label="Guide" active={panel === 'guide'} onClick={() => toggle('guide')} />
              </>
            ) : (
              <>
                {video?.chapters.length ? <PBtn icon="list" label="Chapters" active={panel === 'chapters'} onClick={() => toggle('chapters')} /> : null}
                {s.queue.length > 1 ? <PBtn icon="queue" label="Up next" active={panel === 'queue'} onClick={() => toggle('queue')} /> : null}
                <PBtn icon="captions" label="Audio and subtitles" active={panel === 'audio' || !!s.subtitle} onClick={() => toggle('audio')} />
              </>
            )}
            <PBtn icon="settings" label="Playback options" active={panel === 'options'} onClick={() => toggle('options')} />
            <PBtn icon="pip" label="Picture in picture" onClick={() => { player.set({pip: true}); minimize(); }} />
            <PBtn icon="minimize" label="Minimize (Esc)" onClick={minimize} />
            <PBtn icon="fullscreen" label="Full screen (F)" onClick={fullscreen} />
          </div>
        </div>
      </div>

      {panel ? <PlayerPanel id={panel} onClose={() => setPanel(null)} onPick={setPanel} item={video} channel={channel} /> : null}
      {ended && video ? <PostPlay item={video} onClose={close} onBrowse={to => { close(); void navigate({to: to as never}); }} /> : null}
      {s.ownerStopped ? <Problem icon="stop" title="The server owner stopped this playback" body={s.ownerStopped ? `Message from the server owner: “${s.ownerStopped}”` : undefined} actions={[{label: 'Close', onClick: close}]} /> : null}
      {s.problem === 'reconnecting' ? <Problem icon="refresh" spin title="Reconnecting to your server…" body="Playback continues from here as soon as the connection is back." actions={[{label: 'Close', onClick: close}]} /> : null}
      {s.problem === 'unplayable' ? <Problem icon="alert" title="This title can’t play right now" body="The file isn’t available on the server. It may be on a drive that’s disconnected." actions={[{label: 'Try again', onClick: () => player.set({problem: null})}, {label: 'Report a problem', onClick: () => actions.feedback(s.itemId ?? undefined, 'player')}]} /> : null}
      {s.problem === 'dolbyVision' ? <Problem icon="monitor" title="This display can’t show Dolby Vision" body="Colors may look wrong. Portico can play the HDR10 version or convert it for this screen." actions={[{label: 'Play anyway', onClick: () => player.set({problem: null})}, {label: 'Convert for this screen', onClick: () => { player.set({problem: null, quality: '1080p'}); toast('Converting for this screen'); }}]} /> : null}
      {s.problem === 'channel' ? <Problem icon="wifiOff" title="This channel isn’t coming through" body="The tuner lost the signal. Try another channel, or try again in a moment." actions={[{label: 'Try again', onClick: () => player.set({problem: null})}, {label: 'Open the guide', onClick: () => { player.set({problem: null}); setPanel('guide'); }}]} /> : null}
    </div>
  );
}

/** A panel a review link asked for (see app/demo.tsx). */
export function takePending(): string | null {
  const w = window as unknown as {porticoPanel?: string};
  const p = w.porticoPanel ?? null;
  delete w.porticoPanel;
  return p;
}

function PBtn({icon, label, onClick, active, big, disabled}: {icon: IconName; label: string; onClick: () => void; active?: boolean; big?: boolean; disabled?: boolean}) {
  return (
    <Tooltip label={label}>
      <button type="button" className={cx('p-btn', big && 'is-big', active && 'is-on')} onClick={onClick} aria-label={label.replace(/ \(.+\)$/, '')} disabled={disabled}>
        <Icon name={icon} size={big ? 28 : 22} fill={icon === 'play' || icon === 'pause' ? 'currentColor' : undefined} />
      </button>
    </Tooltip>
  );
}

function Problem({icon, title, body, actions: list, spin}: {icon: IconName; title: string; body?: string; actions: {label: string; onClick: () => void}[]; spin?: boolean}) {
  return (
    <div className="player-problem" role="alertdialog" aria-label={title}>
      <span className={cx('state-icon', spin && 'is-spin')}><Icon name={icon} size={28} /></span>
      <h2 className="state-title">{title}</h2>
      {body ? <p className="state-body">{body}</p> : null}
      <div className="state-actions">{list.map((a, i) => <button key={a.label} type="button" className={cx('btn btn-md', i === 0 ? 'btn-primary' : 'btn-glass')} onClick={a.onClick}><span className="btn-label">{a.label}</span></button>)}</div>
    </div>
  );
}

function LiveBar({programme, behind, onJump}: {programme?: Programme; behind: number; onJump: () => void}) {
  const now = Date.now();
  const start = programme?.start ?? now - 1800_000;
  const end = programme?.end ?? now + 1800_000;
  const at = (now - behind * 1000 - start) / (end - start);
  const edge = (now - start) / (end - start);
  const t = (ms: number) => new Date(ms).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
  return (
    <div className="livebar">
      <span className="livebar-time">{t(start)}</span>
      <div className="livebar-track" role="progressbar" aria-label="Live position" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(at * 100)}>
        <span className="livebar-edge" style={{width: `${edge * 100}%`}} />
        <span className="livebar-at" style={{left: `${Math.min(edge, at) * 100}%`}} />
      </div>
      <span className="livebar-time">{t(end)}</span>
      {behind > 0 ? <button type="button" className="live-jump" onClick={onJump}>−{formatClock(behind)} · Jump to live</button> : <span className="live-now"><span />Live</span>}
    </div>
  );
}

/** Live channels have no stills in the fixtures: pick a few stable backdrops per programme. */
const backdrops = [...artKeys].filter(k => k.startsWith('backdrop/'));
function framesFor(item: Movie | Episode | null, liveKey?: string): string[] {
  if (liveKey) {
    let h = 0;
    for (let i = 0; i < liveKey.length; i++) h = (h * 31 + liveKey.charCodeAt(i)) | 0;
    return [0, 1, 2].map(n => backdrops[Math.abs(h + n * 7919) % backdrops.length]!);
  }
  if (!item) return [];
  const list = item.chapters.map(c => c.art).filter((x): x is string => !!x);
  const backdrop = item.kind === 'episode' ? item.art.still : item.art.backdrop ?? item.art.still;
  const all = [...new Set([backdrop, ...list].filter((x): x is string => !!x))];
  return all.length ? all : [item.art.poster].filter((x): x is string => !!x);
}

export type {Item};
