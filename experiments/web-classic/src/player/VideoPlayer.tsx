import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Chapter, Entry, Marker, Track} from '@/data/types';
import {useActions} from '@/app/actions';
import {usePrefs} from '@/app/prefs';
import {formatTime, clock} from '@/app/format';
import {Artwork} from '@/ui/Bits';
import {Button} from '@/ui/Button';
import {Slider} from '@/ui/Form';
import {Icon, type IconName} from '@/ui/Icon';
import {useToast} from '@/ui/Overlay';
import {cx} from '@/ui/cx';
import {usePlayer, usePlayerState} from './PlayerProvider';
import {Captions, captionAt, Timeline} from './Timeline';
import {GroupPill, MiniGuide, OptionsPanel, PlayOnDialog, QueuePanel} from './Panels';
import {useDemo} from '@/app/demo';
import s from './Player.module.css';

type Panel = 'options' | 'queue' | 'guide' | undefined;
const WHY: Record<string, string> = {
  tunersBusy: 'Every tuner is busy recording or being watched. Try again when one frees up, or stop a recording under Recordings.',
  sourceUnavailable: 'The channel’s source isn’t responding. Try again in a minute.',
  storage: 'The server has no room to buffer live TV right now. The owner can free up space under Manage server › Storage.',
  stalled: 'The channel took too long to get ready. Try again.',
  startFailed: 'The server couldn’t start this channel. Try again, or pick another channel.',
};

function Ctl({icon, label, onClick, big, pressed, badge, className}: {icon: IconName; label: string; onClick: () => void; big?: boolean; pressed?: boolean; badge?: string; className?: string}) {
  return <button type="button" className={cx(s.ctl, big && s.ctlBig, className)} onClick={onClick} aria-label={label} title={label} aria-pressed={pressed}><Icon name={icon} size={big ? 34 : 24} />{badge ? <span className={s.ctlLabel}>{badge}</span> : null}</button>;
}

/**
 * The video and live player. Playback is simulated (a still with slow
 * motion and a real clock); every control, panel and state is real.
 * Keyboard: Space/K play, ←/→ or J/L skip, ↑/↓ volume, M mute, F full
 * screen, C subtitles, Shift+N next, G mini guide (live), R record (live), Esc close.
 */
export function VideoPlayer() {
  const st = usePlayerState();
  const player = usePlayer();
  const client = useClient();
  const actions = useActions();
  const toast = useToast();
  const navigate = useNavigate();
  const {prefs} = usePrefs();
  const entry = st.queue[st.index];
  const live = st.mode === 'live';
  const root = useRef<HTMLDivElement>(null);
  const [chrome, setChrome] = useState(true);
  const [panel, setPanel] = useState<Panel>();
  const [playOn, setPlayOn] = useState(false);
  useDemo(['player-options', 'player-queue', 'player-playon', 'player-miniguide'], name => { if (name === 'player-playon') setPlayOn(true); else setPanel(name === 'player-options' ? 'options' : name === 'player-queue' ? 'queue' : 'guide'); });
  const [volume, setVolume] = useState(80);
  const [muted, setMuted] = useState(false);
  const [captionStyle, setCaptionStyle] = useState({size: 'medium', background: 'box'});
  const [problem, setProblem] = useState<string>();
  const [attempt, setAttempt] = useState(0);
  const [dismissedPost, setDismissedPost] = useState<string>();
  const [inRow, setInRow] = useState(0);
  const [liveCaptions, setLiveCaptions] = useState(false);
  const hideTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const skipBack = Number(client.prefs.get('playback.skipBackSeconds') ?? 10);
  const skipFwd = Number(client.prefs.get('playback.skipForwardSeconds') ?? 30);
  const autoplay = client.prefs.get('playback.autoplayNext') !== false;
  const countdown = Number(client.prefs.get('playback.upNextCountdownSeconds') ?? 10);

  const detail = useQuery(async c => {
    if (!entry || live) return undefined;
    if (entry.kind === 'episode') { const d = await c.catalog.episode(entry.id); return {chapters: [] as Chapter[], markers: d.markers, audio: [] as Track[], subtitles: [] as Track[], related: [] as Entry[], still: d.entry.image}; }
    if (entry.kind === 'movie' && !entry.id.match(/-(trailer|bts|scene)$/)) { const d = await c.catalog.movie(entry.id); return {chapters: d.chapters, markers: [{kind: 'credits' as const, start: (d.entry.duration ?? 0) - 240, end: d.entry.duration ?? 0}] as Marker[], audio: d.audio, subtitles: d.subtitles, related: d.related, still: undefined}; }
    return undefined;
  }, [entry?.id, live]);
  const chapters = detail.data?.chapters ?? [];
  const markers = detail.data?.markers ?? [];
  const audioTracks = detail.data?.audio.length ? detail.data.audio : [{id: 'a1', label: 'English', language: 'en', codec: 'AAC', channels: 'Stereo', default: true}];
  const subtitleTracks = detail.data?.subtitles.length ? detail.data.subtitles : [{id: 's1', label: 'English', language: 'en', codec: 'SRT'}, {id: 's2', label: 'English (SDH)', language: 'en', codec: 'SRT'}];

  const poke = useCallback(() => {
    setChrome(true);
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setChrome(false), 3500);
  }, []);
  useEffect(() => { poke(); return () => { if (hideTimer.current) clearTimeout(hideTimer.current); }; }, [poke, entry?.id]);
  useEffect(() => { if (!st.playing) { setChrome(true); if (hideTimer.current) clearTimeout(hideTimer.current); } else poke(); }, [st.playing, poke]);
  useEffect(() => { root.current?.focus(); }, []);

  // Live: ask the server to start the channel, and say plainly why when it can't.
  useEffect(() => {
    if (!live || !st.liveChannelId) { setProblem(undefined); return; }
    let alive = true;
    setProblem(undefined);
    void client.live.start(st.liveChannelId).then(r => {
      if (!alive) return;
      if (r.ok) { if (attempt > 0) player.set({buffering: false, playing: true}); return; }
      setProblem(r.reason);
      player.set({playing: false, buffering: false});
    });
    return () => { alive = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [live, st.liveChannelId, attempt]);

  // Skip intro and recap automatically when the preference says so.
  const marker = markers.find(m => st.position >= m.start && st.position < m.end - 2);
  useEffect(() => {
    if (!marker) return;
    const pref = client.prefs.get(marker.kind === 'intro' ? 'playback.introSkip' : marker.kind === 'recap' ? 'playback.recapSkip' : 'playback.creditsSkip');
    if (pref === 'auto' && marker.kind !== 'credits') { player.seek(marker.end); toast.show(marker.kind === 'intro' ? 'Skipped the intro' : 'Skipped the recap', {icon: 'skipForward'}); }
  }, [marker?.kind, marker?.start]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { if (st.ended && entry?.kind === 'episode') setInRow(n => n + 1); }, [st.ended]); // eslint-disable-line react-hooks/exhaustive-deps

  const close = useCallback(() => { if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}); player.close(); }, [player]);
  const toggleFullscreen = useCallback(() => { if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}); else void root.current?.requestFullscreen?.().catch(() => {}); }, []);
  const surf = useCallback(async (d: number) => {
    const list = await client.live.channels();
    const i = list.findIndex(c => c.id === st.liveChannelId);
    const ch = list[(i + d + list.length) % list.length]!;
    const now = client.live.nowPlaying(ch.id);
    player.playLive(ch.id, {id: ch.id, kind: 'channel', title: now?.title ?? ch.name, subtitle: `${ch.number} · ${ch.name}`, image: now?.image, shape: 'wide'});
  }, [client, player, st.liveChannelId]);
  const record = useCallback(async () => {
    if (!st.liveChannelId) return;
    const now = client.live.nowPlaying(st.liveChannelId);
    if (!now) return;
    await client.live.record(now.id);
    toast.show(`Recording “${now.title}” until ${clock(now.end)}`, {icon: 'record'});
  }, [client, st.liveChannelId, toast]);

  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (panel || playOn || e.metaKey || e.ctrlKey || e.altKey) return;
      const t = e.target as HTMLElement;
      if (/^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName) || t.closest('[role=dialog]:not([data-player])')) return;
      poke();
      const k = e.key;
      if (k === ' ' || k === 'k') { if (t.tagName === 'BUTTON' && k === ' ') return; e.preventDefault(); player.toggle(); }
      else if ((k === 'ArrowLeft' || k === 'j') && !t.closest('[role=slider]')) { e.preventDefault(); if (!live) player.skip(-skipBack); }
      else if ((k === 'ArrowRight' || k === 'l') && !t.closest('[role=slider]')) { e.preventDefault(); if (!live) player.skip(skipFwd); }
      else if (k === 'ArrowUp' && !live) { e.preventDefault(); setVolume(v => Math.min(100, v + 10)); setMuted(false); }
      else if (k === 'ArrowDown' && !live) { e.preventDefault(); setVolume(v => Math.max(0, v - 10)); }
      else if ((k === 'PageUp' || k === 'ArrowUp') && live) { e.preventDefault(); void surf(-1); }
      else if ((k === 'PageDown' || k === 'ArrowDown') && live) { e.preventDefault(); void surf(1); }
      else if (k === 'm') setMuted(v => !v);
      else if (k === 'f') toggleFullscreen();
      else if (k === 'c') { if (live) setLiveCaptions(v => !v); else player.set({subtitles: st.subtitles === 'off' ? subtitleTracks[0]!.id : 'off'}); }
      else if (k === 'g' && live) setPanel('guide');
      else if (k === 'r' && live) void record();
      else if (k === 'N' && e.shiftKey) player.next();
      else if (k === 'P' && e.shiftKey) player.previous();
      else if (k === 'Escape') { if (document.fullscreenElement) void document.exitFullscreen().catch(() => {}); else close(); }
    };
    window.addEventListener('keydown', on);
    return () => window.removeEventListener('keydown', on);
  }, [panel, playOn, poke, player, live, skipBack, skipFwd, surf, toggleFullscreen, st.subtitles, subtitleTracks, record, close]);

  const next = st.queue[st.index + 1];
  const thumb = useMemo(() => {
    if (chapters.length) return (t: number) => [...chapters].reverse().find(c => c.start <= t)?.image ?? entry?.backdrop;
    return () => detail.data?.still ?? entry?.backdrop ?? entry?.image;
  }, [chapters, entry, detail.data?.still]);
  if (!entry) return null;

  if (live && problem) {
    const channel = entry.subtitle?.split(' · ').pop() ?? 'This channel';
    return (
      <div className={s.video} data-player>
        <div className={s.problem} role="alert">
          <Icon name="live" size={44} style={{color: 'var(--amber)'}} />
          <h2>{channel} can’t start right now</h2>
          <p>{WHY[problem] ?? WHY.startFailed}</p>
          <div style={{display: 'flex', gap: 10, marginTop: 8, flexWrap: 'wrap', justifyContent: 'center'}}>
            <Button variant="primary" icon="refresh" label="Try again" autoFocus onClick={() => { player.set({buffering: true, playing: true}); setAttempt(a => a + 1); }} />
            <Button variant="secondary" icon="guide" label="Back to the guide" onClick={() => { close(); void navigate({to: '/channels' as never}); }} />
          </div>
        </div>
      </div>
    );
  }

  const title = entry.kind === 'episode' ? (entry.caption?.split(' · ').find(p => !/^(S\d|E\d|Special)/.test(p)) ?? entry.subtitle ?? entry.title) : entry.title;
  const sub = entry.kind === 'episode' ? `${(entry.caption ?? '').split(' · ').filter(p => /^(S\d|E\d)/.test(p)).join(' ')} · ${entry.title}` : live ? entry.subtitle : [entry.year, entry.rating].filter(Boolean).join(' · ');
  const picture = live ? entry.image : entry.backdrop ?? entry.image;
  const liveNow = live && st.liveChannelId ? client.live.nowPlaying(st.liveChannelId) : undefined;
  const credits = !live && !!next && entry.kind === 'episode' && (st.ended || (marker?.kind === 'credits'));
  const showUpNext = credits && dismissedPost !== entry.id && inRow < 3;
  const stillWatching = st.ended && inRow >= 3 && entry.kind === 'episode';
  const finished = st.ended && !next && !live;
  const subsOn = !live && st.subtitles !== 'off';
  return (
    <div ref={root} tabIndex={-1} className={cx(s.video, !chrome && st.playing && !panel && s.hide, !st.playing && s.paused)} data-player role="dialog" aria-modal="true" aria-label={`Player: ${title}`} onMouseMove={poke} onClick={e => { if (e.target === e.currentTarget || (e.target as HTMLElement).dataset.picture) { if (chrome) player.toggle(); poke(); } }}>
      <div data-picture className={cx(s.picture, st.playing && !prefs.reduceMotion && s.pictureMotion)} style={{backgroundImage: picture ? `url(${picture})` : undefined}} />
      {st.buffering ? <div className={s.spinnerWrap} role="status"><span className={s.spin} />{live ? 'Tuning…' : 'Getting ready…'}</div> : null}
      {st.cast ? <div className={s.castBanner}><Icon name={st.cast.kind === 'cast' ? 'cast' : st.cast.kind === 'airplay' ? 'airplay' : 'tvDevice'} size={40} /><div className="t-sub">Playing on {st.cast.name}</div><div className="t-caption c-2">This window is now the remote. Use Play on to stop or continue here.</div></div> : null}
      {(subsOn || (live && liveCaptions)) && !finished ? <Captions text={live ? `[Live] ${captionAt(st.position)}` : captionAt(st.position + st.subtitleOffset)} bottom={chrome ? 150 : 56} style={captionStyle} /> : null}

      <div className={s.chrome}>
        <div className={s.top}>
          <Ctl icon="back" label="Close player (Esc)" onClick={close} />
          <div className={s.topTitle}><h2>{live && liveNow ? liveNow.title : title}</h2><p>{live && liveNow ? `${entry.subtitle} · ${clock(liveNow.start)}–${clock(liveNow.end)}` : sub}</p></div>
          <GroupPill />
          {st.cast ? null : <Ctl icon="cast" label="Play on another device" onClick={() => setPlayOn(true)} />}
        </div>
        <div className={s.center} hidden={finished || stillWatching}>
          {!live ? <Ctl icon="skipBack" label={`Back ${skipBack} seconds`} onClick={() => player.skip(-skipBack)} big /> : null}
          <button type="button" className={s.bigPlay} onClick={() => player.toggle()} aria-label={st.playing ? 'Pause' : 'Play'}><Icon name={st.playing ? 'pause' : 'play'} size={40} /></button>
          {!live ? <Ctl icon="skipForward" label={`Forward ${skipFwd} seconds`} onClick={() => player.skip(skipFwd)} big /> : null}
        </div>
        <div className={s.bottom}>
          {live ? (
            <Timeline position={liveNow ? (Date.now() - liveNow.start) / 1000 : 0} duration={liveNow ? (liveNow.end - liveNow.start) / 1000 : 1} onSeek={() => toast.show('This channel can be paused, but not rewound past when you tuned in.', {icon: 'info'})} label="Program progress" />
          ) : <Timeline position={st.position} duration={st.duration} onSeek={t => player.seek(t)} chapters={chapters} markers={markers} thumb={thumb} buffered={6} />}
          <div className={s.controls}>
            <Ctl icon={st.playing ? 'pause' : 'play'} label={st.playing ? 'Pause (Space)' : 'Play (Space)'} onClick={() => player.toggle()} />
            {!live ? <><Ctl icon="skipBack" label={`Back ${skipBack} seconds`} onClick={() => player.skip(-skipBack)} className={s.hideNarrow} /><Ctl icon="skipForward" label={`Forward ${skipFwd} seconds`} onClick={() => player.skip(skipFwd)} className={s.hideNarrow} /></> : null}
            {next && !live ? <Ctl icon="next" label={`Next: ${next.title} (Shift+N)`} onClick={() => player.next()} /> : null}
            <Ctl icon={muted || volume === 0 ? 'minus' : 'audio'} label={muted ? 'Unmute (M)' : 'Mute (M)'} onClick={() => setMuted(!muted)} />
            <div className={s.volume}><Slider label="Volume" value={muted ? 0 : volume} onChange={v => { setVolume(v); setMuted(v === 0); }} /></div>
            {live ? <span className={cx(s.liveBadge, !st.playing && s.behind)}>{st.playing ? 'LIVE' : 'PAUSED'}</span> : <span className={s.time}>{formatTime(st.position)} / {formatTime(st.duration)}</span>}
            <span className={s.grow} />
            {live ? <><Ctl icon="chevronUp" label="Channel up (Page Up)" onClick={() => void surf(-1)} className={s.hideNarrow} /><Ctl icon="chevronDown" label="Channel down (Page Down)" onClick={() => void surf(1)} className={s.hideNarrow} /><Ctl icon="record" label="Record this program (R)" onClick={() => void record()} /><Ctl icon="captions" label={liveCaptions ? 'Turn off live captions (C)' : 'Turn on live captions (C)'} pressed={liveCaptions} onClick={() => setLiveCaptions(!liveCaptions)} /><Ctl icon="guide" label="Mini guide (G)" onClick={() => setPanel('guide')} /></> : null}
            {!live ? <Ctl icon="subtitles" label="Audio and subtitles" pressed={subsOn} onClick={() => setPanel('options')} /> : null}
            {!live && st.queue.length > 1 ? <Ctl icon="queue" label="Up next" onClick={() => setPanel('queue')} /> : null}
            <Ctl icon="settings" label="Quality and speed" onClick={() => setPanel('options')} className={s.hideNarrow} />
            {!live ? <Ctl icon="flag" label="Report a problem" onClick={() => actions.dialog({kind: 'feedback', entry})} className={s.hideNarrow} /> : null}
            <Ctl icon="expand" label="Full screen (F)" onClick={toggleFullscreen} />
          </div>
        </div>
      </div>

      {marker && marker.kind !== 'credits' && !st.buffering ? <div className={s.skip}><button type="button" className={s.skipBtn} onClick={() => player.seek(marker.end)}>{marker.kind === 'intro' ? 'Skip intro' : 'Skip recap'}<Icon name="skipForward" size={18} /></button></div> : null}
      {marker?.kind === 'credits' && !next && !live && !st.ended ? <div className={s.skip}><button type="button" className={s.skipBtn} onClick={() => player.seek(st.duration)}>Skip credits<Icon name="skipForward" size={18} /></button></div> : null}
      {showUpNext && next && !stillWatching ? <UpNext next={next} seconds={countdown} autoplay={autoplay && (st.ended || marker?.kind === 'credits')} onPlay={() => player.next()} onDismiss={() => setDismissedPost(entry.id)} /> : null}
      {stillWatching ? (
        <div className={s.post} role="dialog" aria-label="Are you still watching?">
          <div className="t-sub">Are you still watching?</div>
          <p className="t-caption c-2">You’ve watched {inRow} episodes in a row. Playback paused so you don’t miss anything.</p>
          <div className={s.postButtons}><Button variant="primary" label="Keep watching" autoFocus onClick={() => { setInRow(0); player.next(); }} /><Button variant="secondary" label="I’m done" onClick={close} /></div>
        </div>
      ) : null}
      {finished ? (
        <div className={s.finished} role="dialog" aria-label="Finished">
          <div className={s.finishedHead}>
            <div style={{flex: 1, minWidth: 260}}><div className="t-label">Finished</div><h2 className="t-title">{title}</h2></div>
            <Button variant="secondary" icon="refresh" label="Watch again" onClick={() => player.seek(0)} />
            <Button variant="secondary" icon="star" label="Rate" onClick={() => actions.dialog({kind: 'rate', entry})} />
            <Button variant="primary" label="Close" onClick={close} autoFocus />
          </div>
          {detail.data?.related.length ? (
            <div style={{padding: '0 var(--gutter)'}}>
              <div className="t-sub" style={{marginBottom: 12}}>More like this</div>
              <div style={{display: 'flex', gap: 14, overflow: 'hidden'}}>
                {detail.data.related.slice(0, 8).map(r => <button key={r.id} type="button" className={s.finishedCard} onClick={() => player.play([r], 0)} aria-label={`Play ${r.title}`}><Artwork src={r.image} title={r.title} tint={r.tint} /><span className="t-caption t-strong clamp-1">{r.title}</span><span className="t-caption c-3">{r.year}</span></button>)}
              </div>
            </div>
          ) : null}
        </div>
      ) : null}

      {panel === 'options' ? <OptionsPanel onClose={() => setPanel(undefined)} audio={audioTracks} subtitles={subtitleTracks} captionStyle={captionStyle} onCaptionStyle={setCaptionStyle} /> : null}
      {panel === 'queue' ? <QueuePanel onClose={() => setPanel(undefined)} /> : null}
      {panel === 'guide' ? <MiniGuide onClose={() => setPanel(undefined)} /> : null}
      <PlayOnDialog open={playOn} onOpenChange={setPlayOn} />
    </div>
  );
}

function UpNext({next, seconds, autoplay, onPlay, onDismiss}: {next: Entry; seconds: number; autoplay: boolean; onPlay: () => void; onDismiss: () => void}) {
  const [left, setLeft] = useState(seconds);
  useEffect(() => {
    if (!autoplay) return;
    if (left <= 0) { onPlay(); return; }
    const t = setTimeout(() => setLeft(l => l - 1), 1000);
    return () => clearTimeout(t);
  }, [left, autoplay, onPlay]);
  return (
    <div className={s.post} role="dialog" aria-label="Up next">
      <div className="t-label">Up next</div>
      <div className={s.postNext}>
        <div style={{position: 'relative', borderRadius: 8, overflow: 'hidden'}}><Artwork src={next.image} title={next.title} shape="wide" />{autoplay ? <div className={s.postRing} aria-live="polite">{left}</div> : null}</div>
        <div style={{minWidth: 0}}><div className="t-strong clamp-2">{next.title}</div><div className="t-caption c-3">{next.caption?.split(' · ').filter(p => /^(S\d|E\d)/.test(p)).join(' ') ?? next.subtitle}</div></div>
      </div>
      <div className={s.postButtons}><Button variant="primary" icon="play" label={autoplay ? `Play now` : 'Play next episode'} onClick={onPlay} autoFocus /><Button variant="secondary" label="Not now" onClick={onDismiss} /></div>
    </div>
  );
}
