import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router';
import {
  AlertTriangle,
  Cast,
  ChevronLeft,
  CircleDot,
  ListOrdered,
  ListVideo,
  Maximize,
  MessageSquareText,
  Minimize,
  Pause,
  PictureInPicture2,
  Play,
  Radio,
  RotateCcw,
  RotateCw,
  Settings2,
  SkipForward,
  Users,
  Volume2,
  VolumeX,
  X,
} from 'lucide-react';
import { api } from '../../services/client';
import { usePlayer } from '../../state/player';
import { useUi } from '../../state/ui';
import { CastSheet } from '../../app/CastSheet';
import { castLabel } from '../audio/MiniPlayer';
import { Scrubber } from '../../components/Scrubber';
import { IconButton, cls } from '../../components/ui';
import { toast, Toaster } from '../../components/overlay';
import { fmtClock, useIsMobile } from '../../lib/hooks';
import { frameStyle } from '../../lib/scrub';
import { cueAt, resolveMeta } from './meta';
import type { PlayableSource } from '../../services/api';

type Panel = null | 'tracks' | 'settings' | 'chapters' | 'episodes' | 'together';

export function VideoPlayer() {
  const { id = '' } = useParams();
  const [params] = useSearchParams();
  const extra = params.get('extra');
  const startParam = params.get('t');
  const navigate = useNavigate();
  const meta = useMemo(() => resolveMeta(id, extra), [id, extra]);
  const video = useRef<HTMLVideoElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const [src, setSrc] = useState<PlayableSource | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [playing, setPlaying] = useState(true);
  const [pos, setPos] = useState(0);
  const [waiting, setWaiting] = useState(true);
  const [volume, setVolume] = useState(0.9);
  const [muted, setMuted] = useState(false);
  const [rate, setRate] = useState(1);
  const [quality, setQuality] = useState('Original');
  const [audioId, setAudioId] = useState<string>('');
  const [subId, setSubId] = useState<string>('off');
  const [subSize, setSubSize] = useState<'S' | 'M' | 'L'>('M');
  const [panel, setPanel] = useState<Panel>((params.get('panel') as Panel) ?? null);
  const [chrome, setChrome] = useState(true);
  const [stats, setStats] = useState(false);
  const [full, setFull] = useState(false);
  const [upNextDismissed, setUpNextDismissed] = useState(false);
  const [flash, setFlash] = useState<null | 'play' | 'pause' | 'back' | 'fwd'>(null);
  const castTo = usePlayer((s) => s.castTo);
  const setCastOpen = useUi((s) => s.setCast);
  const mobile = useIsMobile();
  const last = useRef(0);
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const lastSave = useRef(0);
  const posRef = useRef(0);
  posRef.current = pos;

  const duration = src?.duration ?? 0;
  const isLive = meta?.kind === 'channel';

  // Pause any music when a video starts.
  useEffect(() => {
    usePlayer.getState().pause();
  }, []);

  useEffect(() => {
    if (!meta) return;
    setErr(null);
    setSrc(null);
    setWaiting(true);
    setUpNextDismissed(false);
    const def = meta.audio.find((a) => a.isDefault) ?? meta.audio[0];
    setAudioId(def?.id ?? '');
    const forced = meta.subtitles.find((s) => s.forced);
    setSubId(forced?.id ?? 'off');
    const load = extra
      ? Promise.resolve<PlayableSource>({ itemId: id, kind: 'video', url: '/media/sintel-trailer.mp4', mediaDuration: 52.2, duration: 52.2, startAt: 0 })
      : api.getPlayable(id);
    load
      .then((s) => {
        setSrc(s);
        const start = startParam !== null ? Number(startParam) : isLive ? Math.max(0, (Date.now() - (meta.live?.start ?? Date.now())) / 1000) : s.startAt;
        setPos(start);
        if (start > 5 && !startParam && !isLive) toast(`Resuming from ${fmtClock(start)}`, <RotateCcw />, { label: 'Start Over', run: () => seek(0) });
      })
      .catch((e) => setErr(e instanceof Error ? e.message : 'Playback failed'));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, extra]);

  // map the virtual position onto the stream
  const syncVideo = useCallback((t: number) => {
    const v = video.current;
    if (!v || !src) return;
    const d = v.duration || src.mediaDuration || 1;
    const target = src.mediaDuration && Math.abs((src.mediaDuration ?? 0) - src.duration) < 1 ? Math.min(t, d - 0.05) : t % d;
    v.currentTime = target;
    last.current = target;
  }, [src]);

  const seek = useCallback(
    (t: number) => {
      const clamped = Math.max(0, Math.min(t, (isLive ? (Date.now() - (meta?.live?.start ?? 0)) / 1000 : duration) - 0.5));
      setPos(clamped);
      syncVideo(clamped);
    },
    [duration, syncVideo, isLive, meta],
  );

  useEffect(() => {
    const v = video.current;
    if (!v || !src) return;
    const onMeta = () => syncVideo(posRef.current);
    v.addEventListener('loadedmetadata', onMeta);
    return () => v.removeEventListener('loadedmetadata', onMeta);
  }, [src, syncVideo]);

  useEffect(() => {
    const v = video.current;
    if (!v) return;
    v.volume = volume;
    v.muted = muted || !!castTo;
    v.playbackRate = rate;
  }, [volume, muted, rate, castTo]);

  useEffect(() => {
    const v = video.current;
    if (!v || !src) return;
    if (playing) v.play().catch(() => setPlaying(false));
    else v.pause();
  }, [playing, src]);

  const onTime = () => {
    const v = video.current;
    if (!v || !src) return;
    let d = v.currentTime - last.current;
    if (d < -0.5) d += v.duration || 0;
    last.current = v.currentTime;
    if (d <= 0 || d > 3) return;
    const next = posRef.current + d;
    if (!isLive && next >= duration - 0.3) {
      setPos(duration);
      setPlaying(false);
      finish();
      return;
    }
    setPos(next);
    if (!isLive && Date.now() - lastSave.current > 8000 && meta && meta.kind !== 'extra') {
      lastSave.current = Date.now();
      api.saveProgress(meta.id, next, duration);
    }
  };

  const finish = () => {
    if (!meta) return;
    if (meta.kind !== 'extra' && meta.kind !== 'channel') api.saveProgress(meta.id, duration, duration);
    if (meta.next && !upNextDismissed) navigate(`/watch/${meta.next.id}`, { replace: true });
  };

  const close = useCallback(() => {
    if (meta && src && !isLive && meta.kind !== 'extra') api.saveProgress(meta.id, posRef.current, src.duration);
    if (document.fullscreenElement) document.exitFullscreen().catch(() => undefined);
    if (window.history.length > 1) navigate(-1);
    else navigate(meta?.backHref ?? '/');
  }, [meta, src, isLive, navigate]);

  const poke = useCallback(() => {
    setChrome(true);
    clearTimeout(idle.current);
    idle.current = setTimeout(() => {
      if (!panel) setChrome(false);
    }, 3200);
  }, [panel]);

  useEffect(() => {
    poke();
    return () => clearTimeout(idle.current);
  }, [poke]);

  const toggle = useCallback(() => {
    setPlaying((p) => {
      setFlash(p ? 'pause' : 'play');
      return !p;
    });
    setTimeout(() => setFlash(null), 500);
  }, []);

  const toggleFull = useCallback(() => {
    if (!document.fullscreenElement) root.current?.requestFullscreen?.().catch(() => undefined);
    else document.exitFullscreen().catch(() => undefined);
  }, []);

  useEffect(() => {
    const onFs = () => setFull(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', onFs);
    return () => document.removeEventListener('fullscreenchange', onFs);
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).tagName === 'INPUT' || e.metaKey || e.ctrlKey) return;
      poke();
      switch (e.key) {
        case ' ':
        case 'k':
          e.preventDefault();
          toggle();
          break;
        case 'ArrowLeft':
        case 'j':
          seek(posRef.current - 10);
          setFlash('back');
          setTimeout(() => setFlash(null), 500);
          break;
        case 'ArrowRight':
        case 'l':
          seek(posRef.current + 10);
          setFlash('fwd');
          setTimeout(() => setFlash(null), 500);
          break;
        case 'ArrowUp':
          e.preventDefault();
          setVolume((v) => Math.min(1, v + 0.1));
          break;
        case 'ArrowDown':
          e.preventDefault();
          setVolume((v) => Math.max(0, v - 0.1));
          break;
        case 'f':
          toggleFull();
          break;
        case 'm':
          setMuted((m) => !m);
          break;
        case 'c': {
          const first = meta?.subtitles.find((s) => !s.forced);
          setSubId((s) => (s === 'off' && first ? first.id : 'off'));
          break;
        }
        case 'n':
          if (meta?.next) navigate(`/watch/${meta.next.id}`, { replace: true });
          break;
        case 'i':
          setStats((s) => !s);
          break;
        case 'Escape':
          if (panel) setPanel(null);
          else if (!document.fullscreenElement) close();
          break;
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [poke, toggle, seek, toggleFull, meta, panel, close, navigate]);

  if (!meta)
    return (
      <div className="vp error-only">
        <div className="vp-error">
          <AlertTriangle />
          <h2>This title can’t be played</h2>
          <p>It may have been removed from Harbor.</p>
          <Link to="/" className="btn btn-primary">
            Go Home
          </Link>
        </div>
      </div>
    );

  const sub = meta.subtitles.find((s) => s.id === subId);
  const cue = sub && pos > 3 ? cueAt(sub.language, pos) : null;
  const showSkipIntro = !!meta.introEnd && pos > 4 && pos < meta.introEnd;
  const showUpNext = !!meta.next && !!meta.creditsStart && pos >= meta.creditsStart && !upNextDismissed;
  const liveEdge = isLive ? (Date.now() - (meta.live?.start ?? 0)) / 1000 : 0;
  const behindLive = isLive && liveEdge - pos > 20;
  const progLen = isLive ? ((meta.live?.end ?? 0) - (meta.live?.start ?? 0)) / 1000 : duration;
  const audio = meta.audio.find((a) => a.id === audioId);
  const transcoding = quality !== 'Original' || (audio?.codec === 'TrueHD' && mobile);

  return (
    <div
      ref={root}
      className={cls('vp', chrome || !playing || panel ? 'chrome' : 'idle', mobile && 'is-mobile', panel && 'with-panel')}
      onPointerMove={poke}
      onClick={(e) => {
        if ((e.target as HTMLElement).closest('.vp-ui, .vp-panel, .vp-upnext, .vp-skip')) return;
        if (mobile) setChrome((c) => !c);
        else toggle();
      }}
      onDoubleClick={(e) => {
        if (mobile) {
          const x = e.clientX / window.innerWidth;
          if (x < 0.35) seek(pos - 10);
          else if (x > 0.65) seek(pos + 10);
        } else toggleFull();
      }}
    >
      {src && (
        <video
          ref={video}
          className={cls('vp-video', castTo && 'hidden')}
          src={src.url}
          poster={meta.image}
          loop={Math.abs((src.mediaDuration ?? 0) - src.duration) > 1}
          playsInline
          autoPlay
          preload="auto"
          onTimeUpdate={onTime}
          onWaiting={() => setWaiting(true)}
          onPlaying={() => setWaiting(false)}
          onCanPlay={() => setWaiting(false)}
          onError={() => setErr('Harbor couldn’t start this stream. The file may be unreadable or the server is busy.')}
        />
      )}

      {castTo && (
        <div className="vp-cast">
          {meta.image && <img src={meta.image} alt="" className="vp-cast-bg" />}
          <div className="vp-cast-card glass glass-thick">
            <Cast />
            <div className="vp-cast-title">Playing on {castLabel(castTo)}</div>
            <div className="t-sub">Use these controls or the remote for {castLabel(castTo)}.</div>
            <button className="btn btn-plain btn-sm" onClick={() => usePlayer.getState().setCast(null)}>
              Play on this browser instead
            </button>
          </div>
        </div>
      )}

      {cue && <div className={cls('vp-cue', `size-${subSize}`, chrome && 'lifted')}>{cue}</div>}

      {(waiting || !src) && !err && (
        <div className="vp-loading">
          {meta.image && !src && <img src={meta.image} alt="" />}
          <span className="spinner" style={{ width: 42, height: 42, borderWidth: 3 }} />
        </div>
      )}

      {flash && (
        <div className={cls('vp-flash', flash)}>
          {flash === 'play' ? <Play fill="currentColor" /> : flash === 'pause' ? <Pause fill="currentColor" /> : flash === 'back' ? <RotateCcw /> : <RotateCw />}
          {(flash === 'back' || flash === 'fwd') && <span>10</span>}
        </div>
      )}

      {err && (
        <div className="vp-error" role="alert">
          <AlertTriangle />
          <h2>Playback stopped</h2>
          <p>{err}</p>
          <div className="actions">
            <button className="btn btn-primary" onClick={() => window.location.reload()}>
              Try Again
            </button>
            <button className="btn btn-plain" onClick={close}>
              Close
            </button>
          </div>
        </div>
      )}

      {/* Top bar */}
      <div className="vp-ui vp-top">
        <IconButton label="Close player" variant="clear" onClick={close}>
          {mobile ? <X /> : <ChevronLeft />}
        </IconButton>
        <div className="vp-titles">
          {meta.logo && !mobile && meta.kind === 'episode' ? <img src={meta.logo} alt="" className="vp-logo" /> : null}
          <div className="vp-title ellipsis">{meta.title}</div>
          <div className="vp-sub ellipsis">{meta.subtitle}</div>
        </div>
        <div className="vp-top-right">
          {isLive && (
            <span className="badge live">
              <Radio className="inline-ic" style={{ width: 12, height: 12 }} />
              LIVE
            </span>
          )}
          {!mobile && (
            <IconButton label="Watch together" variant="clear" onClick={() => setPanel(panel === 'together' ? null : 'together')} active={panel === 'together'}>
              <Users />
            </IconButton>
          )}
          <IconButton label="Play on another device" variant="clear" active={!!castTo} onClick={() => setCastOpen(true)}>
            <Cast />
          </IconButton>
          {!mobile && 'pictureInPictureEnabled' in document && (
            <IconButton label="Picture in picture" variant="clear" onClick={() => video.current?.requestPictureInPicture?.().catch(() => undefined)}>
              <PictureInPicture2 />
            </IconButton>
          )}
        </div>
      </div>

      {/* Center transport on phones */}
      {mobile && (
        <div className="vp-ui vp-center">
          <button className="vp-round" onClick={() => seek(pos - 10)} aria-label="Back 10 seconds">
            <RotateCcw />
            <span>10</span>
          </button>
          <button className="vp-round big" onClick={toggle} aria-label={playing ? 'Pause' : 'Play'}>
            {playing ? <Pause fill="currentColor" /> : <Play fill="currentColor" />}
          </button>
          <button className="vp-round" onClick={() => seek(pos + 10)} aria-label="Forward 10 seconds">
            <RotateCw />
            <span>10</span>
          </button>
        </div>
      )}

      {showSkipIntro && (
        <button className="vp-skip glass glass-clear" onClick={() => seek(meta.introEnd!)}>
          Skip Intro
          <SkipForward />
        </button>
      )}

      {showUpNext && meta.next && (
        <div className="vp-upnext glass glass-thick">
          <img src={meta.next.image} alt="" />
          <div className="vp-upnext-main">
            <span className="t-eyebrow">Up Next · {Math.max(0, Math.ceil(duration - pos))}s</span>
            <span className="vp-upnext-title ellipsis">{meta.next.title}</span>
            <span className="t-sub">{meta.next.subtitle}</span>
            <div className="vp-upnext-actions">
              <button className="btn btn-primary btn-sm" onClick={() => navigate(`/watch/${meta.next!.id}`, { replace: true })}>
                <Play fill="currentColor" /> Play Now
              </button>
              <button className="btn btn-plain btn-sm" onClick={() => setUpNextDismissed(true)}>
                Watch Credits
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Bottom controls */}
      <div className="vp-ui vp-bottom glass glass-clear">
        <Scrubber
          value={pos}
          max={isLive ? Math.max(progLen, pos + 1) : duration || 1}
          onSeek={seek}
          marks={meta.chapters.map((c) => c.start)}
          buffered={isLive ? liveEdge : Math.min(duration, pos + 180)}
          preview={(t) => (src ? <span className="vp-thumb" style={frameStyle(meta.stream, t, 200)} /> : null)}
          showTimes={false}
          label="Seek"
        />
        <div className="vp-row">
          {!mobile && (
            <>
              <IconButton label={playing ? 'Pause (space)' : 'Play (space)'} variant="plain" onClick={toggle} className="vp-btn">
                {playing ? <Pause fill="currentColor" /> : <Play fill="currentColor" />}
              </IconButton>
              <IconButton label="Back 10 seconds" variant="plain" onClick={() => seek(pos - 10)} className="vp-btn">
                <RotateCcw />
              </IconButton>
              <IconButton label="Forward 10 seconds" variant="plain" onClick={() => seek(pos + 10)} className="vp-btn">
                <RotateCw />
              </IconButton>
              <div className="vp-vol">
                <IconButton label={muted ? 'Unmute (m)' : 'Mute (m)'} variant="plain" onClick={() => setMuted(!muted)} className="vp-btn">
                  {muted || volume === 0 ? <VolumeX /> : <Volume2 />}
                </IconButton>
                <input type="range" className="range" min={0} max={1} step={0.02} value={muted ? 0 : volume} style={{ ['--pct' as string]: `${(muted ? 0 : volume) * 100}%` }} onChange={(e) => { setVolume(Number(e.target.value)); setMuted(false); }} aria-label="Volume" />
              </div>
            </>
          )}
          <div className="vp-time tnum">
            {isLive ? (
              behindLive ? (
                <button className="vp-golive" onClick={() => seek(liveEdge)}>
                  <CircleDot /> {fmtClock(liveEdge - pos)} behind · Go Live
                </button>
              ) : (
                <span className="vp-live-label">
                  <i /> {meta.live?.programTitle} · ends {new Date(meta.live?.end ?? 0).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}
                </span>
              )
            ) : (
              <>
                {fmtClock(pos)} <span className="vp-dim">/ {fmtClock(duration)}</span>
                {!mobile && duration > 0 && <span className="vp-dim vp-ends"> · Ends at {new Date(Date.now() + ((duration - pos) / rate) * 1000).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}</span>}
              </>
            )}
          </div>
          <div className="vp-right">
            {meta.season && (
              <IconButton label="Episodes" variant="plain" className="vp-btn" active={panel === 'episodes'} onClick={() => setPanel(panel === 'episodes' ? null : 'episodes')}>
                <ListVideo />
              </IconButton>
            )}
            {meta.chapters.length > 1 && (
              <IconButton label="Chapters" variant="plain" className="vp-btn" active={panel === 'chapters'} onClick={() => setPanel(panel === 'chapters' ? null : 'chapters')}>
                <ListOrdered />
              </IconButton>
            )}
            <IconButton label="Audio & subtitles (c)" variant="plain" className="vp-btn" active={panel === 'tracks' || subId !== 'off'} onClick={() => setPanel(panel === 'tracks' ? null : 'tracks')}>
              <MessageSquareText />
            </IconButton>
            <IconButton label="Settings" variant="plain" className="vp-btn" active={panel === 'settings'} onClick={() => setPanel(panel === 'settings' ? null : 'settings')}>
              <Settings2 />
            </IconButton>
            {meta.next && !mobile && (
              <IconButton label="Next episode (n)" variant="plain" className="vp-btn" onClick={() => navigate(`/watch/${meta.next!.id}`, { replace: true })}>
                <SkipForward fill="currentColor" />
              </IconButton>
            )}
            {!mobile && (
              <IconButton label="Full screen (f)" variant="plain" className="vp-btn" onClick={toggleFull}>
                {full ? <Minimize /> : <Maximize />}
              </IconButton>
            )}
          </div>
        </div>
      </div>

      {panel && (
        <div className={cls('vp-panel glass glass-thick', `p-${panel}`)} role="dialog" aria-label={panel}>
          <button className="icon-btn sm plain vp-panel-close" aria-label="Close" onClick={() => setPanel(null)}>
            <X />
          </button>
          {panel === 'tracks' && (
            <div className="vp-cols">
              <div>
                <h3>Audio</h3>
                {meta.audio.map((a) => (
                  <button key={a.id} className={cls('vp-opt', a.id === audioId && 'on')} onClick={() => { setAudioId(a.id); toast(`Audio: ${a.label}`); }}>
                    <span>{a.label}</span>
                    <span className="vp-opt-sub">{a.channels}</span>
                  </button>
                ))}
              </div>
              <div>
                <h3>Subtitles</h3>
                <button className={cls('vp-opt', subId === 'off' && 'on')} onClick={() => setSubId('off')}>
                  <span>Off</span>
                </button>
                {meta.subtitles.map((s) => (
                  <button key={s.id} className={cls('vp-opt', s.id === subId && 'on')} onClick={() => setSubId(s.id)}>
                    <span>{s.label}</span>
                    <span className="vp-opt-sub">{s.format}</span>
                  </button>
                ))}
                {meta.subtitles.length === 0 && <p className="t-sub" style={{ padding: '6px 10px' }}>No subtitles in this file. Portico can search OpenSubtitles when an admin enables it.</p>}
                <div className="vp-subsize">
                  <span className="t-sub">Size</span>
                  {(['S', 'M', 'L'] as const).map((s) => (
                    <button key={s} className={cls('chip', subSize === s && 'on')} onClick={() => setSubSize(s)}>
                      {s === 'S' ? 'Small' : s === 'M' ? 'Medium' : 'Large'}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}
          {panel === 'settings' && (
            <div className="vp-cols">
              <div>
                <h3>Quality</h3>
                {[
                  ['Original', `${meta.version.label} · ${(meta.version.bitrate / 1000).toFixed(0)} Mbps`],
                  ['Auto', 'Adjusts to your connection'],
                  ['1080p', '10 Mbps · converted on Harbor'],
                  ['720p', '4 Mbps'],
                  ['480p', '2 Mbps · data saver'],
                ].map(([q, d]) => (
                  <button key={q} className={cls('vp-opt', quality === q && 'on')} onClick={() => { setQuality(q); if (q !== 'Original' && q !== 'Auto') toast(`Switching to ${q}. Harbor will convert this stream.`, <Settings2 />); }}>
                    <span>{q}</span>
                    <span className="vp-opt-sub">{d}</span>
                  </button>
                ))}
              </div>
              <div>
                <h3>Speed</h3>
                <div className="vp-speeds">
                  {[0.5, 0.75, 1, 1.25, 1.5, 2].map((r) => (
                    <button key={r} className={cls('chip', rate === r && 'on')} onClick={() => setRate(r)}>
                      {r}×
                    </button>
                  ))}
                </div>
                <h3 style={{ marginTop: 18 }}>Playback info</h3>
                <button className={cls('vp-opt', stats && 'on')} onClick={() => setStats(!stats)}>
                  <span>Show stream details</span>
                  <span className="vp-opt-sub">i</span>
                </button>
              </div>
            </div>
          )}
          {panel === 'chapters' && (
            <div className="vp-list">
              <h3>Chapters</h3>
              <div className="vp-chapters">
                {meta.chapters.map((c, i) => {
                  const on = pos >= c.start && (i === meta.chapters.length - 1 || pos < meta.chapters[i + 1].start);
                  return (
                    <button key={i} className={cls('vp-chapter', on && 'on')} onClick={() => seek(c.start)}>
                      <span className="vp-chapter-frame" style={frameStyle(meta.stream, c.start * 1.37 + i * 11, 168)} />
                      <span className="ellipsis">{c.title}</span>
                      <span className="vp-opt-sub tnum">{fmtClock(c.start)}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
          {panel === 'episodes' && meta.season && (
            <div className="vp-list">
              <h3>Season {meta.season.number}</h3>
              <div className="vp-eps">
                {meta.season.episodes.map((e) => (
                  <button key={e.id} className={cls('vp-ep', e.id === meta.id && 'on')} onClick={() => navigate(`/watch/${e.id}`, { replace: true })}>
                    <img src={e.image} alt="" />
                    <span className="vp-ep-main">
                      <span className="vp-opt-sub">Episode {e.number} · {e.runtime} min</span>
                      <span className="ellipsis">{e.title}</span>
                    </span>
                    {e.id === meta.id && <span className="badge new">Playing</span>}
                  </button>
                ))}
              </div>
            </div>
          )}
          {panel === 'together' && (
            <div className="vp-list">
              <h3>Watch Together</h3>
              <p className="t-sub" style={{ marginBottom: 14 }}>Play, pause and seek stay in sync for everyone. Each person streams from Harbor at the quality their connection allows.</p>
              {[
                ['Daniel', '#f0a02a', 'Living Room · ready'],
                ['Grandma Ingrid', '#c9a86a', 'Göteborg · invited'],
                ['Sam Carter', '#8a5cf0', 'Montréal · available'],
              ].map(([n, c, s]) => (
                <div key={n} className="vp-person">
                  <span className="avatar" style={{ ['--s' as string]: '36px', ['--c' as string]: c }}>{n[0]}</span>
                  <span className="vp-ep-main">
                    <span>{n}</span>
                    <span className="vp-opt-sub">{s}</span>
                  </span>
                  <button className="btn btn-tinted btn-sm" onClick={() => toast(`Invited ${n} to watch ${meta.title}`, <Users />)}>
                    Invite
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {stats && (
        <div className="vp-stats">
          <div>
            <b>Decision</b> {transcoding ? 'Transcode (hardware, Quick Sync)' : isLive ? 'Direct Stream (remux)' : 'Direct Play'}
          </div>
          <div>
            <b>Video</b> {meta.version.video}
            {transcoding ? ` → ${quality === 'Auto' ? '1080p' : quality} H.264` : ''}
          </div>
          <div>
            <b>Audio</b> {audio?.label ?? meta.version.audio}
          </div>
          <div>
            <b>Container</b> {meta.version.container}
            {isLive ? ' → HLS' : ''}
          </div>
          <div>
            <b>Bitrate</b> {transcoding ? '9.8 Mbps' : `${(meta.version.bitrate / 1000).toFixed(1)} Mbps`} · <b>Buffer</b> {isLive ? '6.0' : '118.4'} s
          </div>
          <div>
            <b>Server</b> Harbor · LAN · 3 ms · <b>Dropped</b> 0 frames
          </div>
          <button className="icon-btn sm plain" aria-label="Hide stream details" onClick={() => setStats(false)}>
            <X />
          </button>
        </div>
      )}
      <CastSheet />
      <Toaster />
    </div>
  );
}
