import {advancePendingChoice, takePendingTrackChoice, type PendingChoiceProgress} from './pending-choice';
import {Lyrics} from './Lyrics';
import {useCast} from '../app/cast';
import {PlayOnButton} from './PlayOn';
import {QueueList} from './QueueList';
import {useI18n} from '../app/i18n';
import {syncRate} from '../bridge/sync-rate';
import {FeedbackDialog} from '../screens/shared/FeedbackDialog';
import {subtitleTrackNames, trackName} from '@core/presentation/index.ts';
import React, {useCallback, useEffect, useEffectEvent, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {playbackUserMessage} from '@core/playback-messages.ts';
import {playerSessionChoices} from '@core/player-session-choices.ts';
import {usePlaybackChoice} from './playback-choice';
import {playbackFailure} from './failure';
import {SubtitleSurface} from './subtitle-surface';
import {SubtitleOverlay} from './subtitles/overlay';
import {webPlaybackResourceKey} from '../playback-v1/web-playback';
import {fixedQuality, offeredQualities, qualityChoiceOf, qualityRequest, type FixedQuality, type QualityChoice} from '@core/playback-v1/choices.ts';
import {ownerStopOf} from './owner-stop';
import {TUNER_RESERVED, tunerHandover, tunerHandoverLines} from '@core/live-handover.ts';
import {shortcutKey} from './shortcut-key';
import {audiobookRates, type SleepChoice} from '@core/listening.ts';
import {subtitleReason} from '@core/subtitles.ts';
import {useSystemListening} from '../bridge/useSystemListening';
import {formatClock} from '@core/presentation/index.ts';
import {currentAccessToken, useSession} from '../app/session';
import {Artwork, Backdrop, Button, ConfirmDialog, Dialog, Icon, IconButton, Menu, Popover, StateView, Text, cx, useCompact, useNarrow, type IconName} from '../ui';
import {isBitmapResource, textTrackForLanguage, bitmapWarningNeeded, ownerPlaybackSwitches} from '@core/presentation/index.ts';
import {formatBehindSec, isBehindLive, programmeRangeLabel} from '@core/presentation/index.ts';
import {qualityNetworkFooter} from '@core/presentation/index.ts';
import {noteCastMeta} from '@core/presentation/index.ts';
import {channelTimeline} from '@core/playback/channel-timeline.ts';
import {defaultRecordingOptions} from '@core/dvr.ts';
import {currentProgramme, tunedChannelFor} from './live';
import {usePlayerDVR} from './record';
import {MiniGuide} from './MiniGuide';
import {usePlayer, type PlayerEngine} from './engine';
import {usePlayerAudioPlan, usePlayerChapters, usePlayerOffers, usePlayerSubtitles, useTrickplay} from './options';
import {SkipPrompt, type SkipMode} from './SkipPrompt';
import {UpNextCredits, useUpNextDuringCredits} from './UpNextCredits';
import {useServerPreferences} from '../app/server-preferences';
import {LiveTimeline} from './Timeline';
import {PostPlay, ShortcutLegend} from './PostPlay';
import s from './Player.module.css';
import {GroupPill, useHostControlsPlayback} from './GroupPill';
import {PlaybackInfoPanel} from './PlaybackInfo';
import {setPendingTogether, useInRoom} from '../app/together';
import {LiveCaptions} from './LiveCaptions';
import {channelProblemId} from './channel-problem';
import {useNavigate} from '@tanstack/react-router';
import {canSurfChannels, surfChannel} from '../app/channels';
import {legacyChannel} from '@core/guide/index.ts';
import {useViewerScope} from '../app/viewer-scope';
import {errorText} from '../app/errors';

/**
 * Player surface mounted once in the shell. Full-screen video (player-canvas)
 * or audio Now Playing when expanded; the audio mini bar when collapsed.
 * Video is full-screen only; leaving it stops playback.
 */
export function PlayerSurface() {
  const player = usePlayer();
  const compact = useCompact();
  useEffect(() => {
    const reserve = player.active && !player.expanded && player.isAudio ? (compact ? 56 : 64) : 0;
    document.documentElement.style.setProperty('--player-reserve', `${reserve}px`);
    document.body.style.overflow = player.active && player.expanded ? 'hidden' : '';
    return () => {
      document.documentElement.style.removeProperty('--player-reserve');
      document.body.style.overflow = '';
    };
  }, [player.active, player.expanded, player.isAudio, compact]);
  const i18n = useI18n();
  if (!player.active) return null;
  return (
    <PlayerBoundary title={i18n.t('web.player.crashed.title')} body={i18n.t('web.player.crashed.body')} retry={i18n.t('action.tryAgain')} close={i18n.t('action.close')} onClose={player.leave}>
      <FullPlayer engine={player} visible={player.expanded} />
      {!player.expanded && player.isAudio ? <MiniBar engine={player} /> : null}
    </PlayerBoundary>
  );
}

/**
 * A fault while drawing the player (a menu, an option panel) stops at the player: the rest of the
 * app stays up, and Try again mounts the player afresh. The playback path remembers the
 * presentation and its position, so the title picks up where it was.
 */
class PlayerBoundary extends React.Component<{title: string; body: string; retry: string; close: string; onClose: () => void; children: React.ReactNode}, {failed: boolean}> {
  state = {failed: false};
  static getDerivedStateFromError() { return {failed: true}; }
  componentDidCatch(error: unknown) { console.error('[player] render failed', error); }
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div className={s.stage} role="alert">
        <div className={s.errorState}>
          <StateView icon="warning" title={this.props.title} body={this.props.body} action={{label: this.props.retry, onClick: () => this.setState({failed: false})}} secondaryAction={{label: this.props.close, onClick: () => { this.setState({failed: false}); this.props.onClose(); }}} />
        </div>
      </div>
    );
  }
}

const rates = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];
/** Picture in Picture: the browser's own window, with the video alone. Subtitles burned into the
 * picture stay in it; drawn ones are not shown there and come back when the picture does. */
const pipAvailable = typeof document !== 'undefined' && !!document.pictureInPictureEnabled;

const noQueue = () => () => {};
const sleepChoices: readonly SleepChoice[] = ['off', 15, 30, 45, 60, 90, 'item'];
/** One film frame, for stepping while paused (the file's own rate is not known to the page). */
const FRAME_SECONDS = 1 / 24;

function FullPlayer({engine, visible}: {engine: PlayerEngine; visible: boolean}) {
  const {service, state, identity, isAudio, queue} = engine;
  // Music has no 10/30-second skip and no Speed; audiobooks keep both.
  const isMusic = identity.kind === 'song';
  const hostControls = useHostControlsPlayback();
  const navigate = useNavigate();
  const inRoom = useInRoom();
  // Playback information (More): stays up over the picture until closed or the title changes.
  const [info, setInfo] = useState(false);
  useEffect(() => setInfo(false), [identity.itemId]);
  // Subtitle search asks the server's provider; the results list in the subtitle menu.
  const [subtitleSearch, setSubtitleSearch] = useState(false);
  useEffect(() => setSubtitleSearch(false), [identity.itemId]);
  const i18n = useI18n();
  // Entries from the current one on (1 = only what's playing): drives the Up next box.
  const queueCount = useSyncExternalStore(queue?.workspace.service.subscribe ?? noQueue, () => {
    const q = queue?.workspace.service.getSnapshot().queue;
    if (!q) return 0;
    const live = q.entries.filter(e => !e.removed);
    const at = live.findIndex(e => e.id === q.currentEntryId);
    return live.length - Math.max(0, at);
  });
  const cast = useCast();
  const {api} = useSession();
  const scope = useViewerScope();
  const compact = useCompact();
  // WEB-PLAYER-01: publish the bottom chrome's height so Now Playing's identity never runs under it.
  const bottom = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = bottom.current, host = stage.current;
    if (!el || !host || typeof ResizeObserver === 'undefined') return;
    const set = () => host.style.setProperty('--np-chrome-height', `${Math.round(el.getBoundingClientRect().height)}px`);
    set();
    const observer = new ResizeObserver(set);
    observer.observe(el);
    return () => observer.disconnect();
  }, [isAudio]);
  const video = useRef<HTMLVideoElement>(null);
  useEffect(() => { syncRate.attach(video.current); return () => syncRate.attach(null); }, []);
  /* One element for the life of the player. The playback path mounts its port on it, and the
     subtitle surface is the one place a subtitle track is attached to it. */
  /* Fit: the whole picture (the default), or filling the screen with the edges cropped, for this playback. */
  const [fill, setFill] = usePlaybackChoice('fill', state.playbackRun, false);
  const toggleFill = () => setFill(now => !now);
  const [surface, setSurface] = useState<SubtitleSurface | null>(null);
  /* Styled and image subtitles are drawn on a canvas over the picture; the surface passes the
     stream's place on the element's clock on to it. */
  const subtitleCanvas = useRef<HTMLCanvasElement>(null);
  const [overlay, setOverlay] = useState<SubtitleOverlay | null>(null);
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    const drawn = subtitleCanvas.current && SubtitleOverlay.supported() ? new SubtitleOverlay(el, subtitleCanvas.current) : null;
    const made = SubtitleSurface.supported(el) ? new SubtitleSurface(el, {companion: drawn ?? undefined}) : null;
    setSurface(made);
    setOverlay(drawn);
    const unmount = engine.playback.mount(el, made ?? drawn ?? undefined, () => (document.pictureInPictureElement === el ? 'pip' : undefined));
    return () => { unmount(); made?.dispose(); drawn?.dispose(); setSurface(null); setOverlay(null); };
  }, [engine.playback]);
  const stage = useRef<HTMLDivElement>(null);
  const resourceKey = webPlaybackResourceKey(state);
  const [notice, setNotice] = useState('');
  const [buffering, setBuffering] = useState(false);
  const [mediaPlaying, setMediaPlaying] = useState(false);
  const [buffered, setBuffered] = useState(0);
  const [chromeHidden, setChromeHidden] = useState(false);
  const [openBox, setOpenBox] = useState<string | null>(null);
  // WEB-PLAYER-03: an option panel never stays open over the post-play card.
  useEffect(() => { if (state.phase === 'ended') setOpenBox(null); }, [state.phase]);
  // Buffered ranges change slowly; reading them on every timeupdate is a
  // seekable scan plus a render at 4 Hz for nothing.
  const bufferedTick = useRef(0);
  const [legend, setLegend] = useState(false);
  const [report, setReport] = useState(false);
  const [rate, setRate] = useState(1);
  const item = identity.item ? {id: identity.item.id, libraryId: identity.item.libraryId} : identity.itemId && identity.libraryId ? {id: identity.itemId, libraryId: identity.libraryId} : undefined;
  useEffect(() => { engine.playback.setRate(rate); }, [engine.playback, rate]);
  const offers = usePlayerOffers(api, scope, item, state.session?.id, state.session?.generation);
  const refreshAudio = usePlayerAudioPlan(offers, item, service, state.session?.id, state.session?.generation);
  const prefs = useServerPreferences();
  const subtitleSize=prefs.value<string>('playback.subtitleSize','medium');
  const subtitleBackground=prefs.value<string>('playback.subtitleBackground','translucent');
  useEffect(()=>{const el=video.current;if(!el)return;el.style.setProperty('--subtitle-size',({small:'80%',medium:'100%',large:'125%','extra-large':'150%'} as Record<string,string>)[subtitleSize]??'100%');el.style.setProperty('--subtitle-background',({none:'transparent',translucent:'var(--subtitle-translucent)',opaque:'black'} as Record<string,string>)[subtitleBackground]??'var(--subtitle-translucent)');},[resourceKey,subtitleSize,subtitleBackground]);
  const skipBack = prefs.value<number>('playback.skipBackSeconds', 10);
  const skipForward = prefs.value<number>('playback.skipForwardSeconds', 30);
  const defaultSpeed = prefs.value<number>('playback.defaultSpeed', 1);
  const skipModes: Record<string, SkipMode> = {intro: prefs.value('playback.introSkip', 'ask'), credits: prefs.value('playback.creditsSkip', 'ask'), recap: prefs.value('playback.recapSkip', 'ask'), outro: prefs.value('playback.creditsSkip', 'ask'), commercial: 'ask'};
  const trickplay = useTrickplay(api, scope, item, offers.data?.current?.sourceId, !isAudio && !state.channel);
  /* An iPhone's full-screen player shows the video and its text track only: a drawn subtitle is
     handed to it as text while it lasts (see usePlayerSubtitles). Picture in Picture hands nothing
     over and starts no conversion. */
  const [systemFullscreen, setSystemFullscreen] = useState(false);
  const subtitles = usePlayerSubtitles(api, scope, service, surface, resourceKey, isAudio, overlay, systemFullscreen);
  /* Forced only (an image track's forced lines alone) is a choice for the track playing now: it starts
     off and holds for this playback. Settings' "Show subtitles: Forced only" picks forced tracks themselves. */
  const [forcedOnly, setForcedOnly] = usePlaybackChoice('forcedOnly', state.playbackRun, false);
  useEffect(() => { overlay?.setForcedOnly(forcedOnly); }, [overlay, forcedOnly]);
  const toggleForcedOnly = () => setForcedOnly(now => !now);
  /* Drawn subtitles keep the look they were made with. The viewer's caption size nudges their
     overall size, and they can be moved up or down the picture for this playback. */
  const [subtitleRaise, setSubtitleRaise] = usePlaybackChoice('subtitleRaise', state.playbackRun, 0);
  const moveSubtitles = (step: number | null) => {
    const next = step === null ? 0 : Math.max(-10, Math.min(30, subtitleRaise + step));
    setSubtitleRaise(() => next);
    setNotice(step === null ? i18n.t('player.subtitles.positionBack') : i18n.t('player.subtitles.positionNotice', {direction: i18n.t(step > 0 ? 'player.subtitles.positionUp' : 'player.subtitles.positionDown')}));
  };
  useEffect(() => { overlay?.adjust({scale: ({small: 0.9, medium: 1, large: 1.12, 'extra-large': 1.25} as Record<string, number>)[subtitleSize] ?? 1, raise: subtitleRaise / 100}); }, [overlay, subtitleSize, subtitleRaise]);
  const drawFallbacks = subtitles.drawFallbacks;
  useEffect(() => { if (drawFallbacks) setNotice(i18n.t('player.subtitles.drawFailed')); }, [drawFallbacks]); // eslint-disable-line react-hooks/exhaustive-deps
  // Fetched with the session (one small read) so Chapters only appears when the title has them.
  const chapters = usePlayerChapters(api, scope, item, service, !isAudio || identity.kind === 'audiobook_file', identity.kind === 'audiobook_file' ? state.positionSeconds : undefined);
  // FEAT-11: chapter starts and intro/credits on the seek bar.
  const chapterList = chapters.data?.status === 'available' ? chapters.data.chapters : undefined;
  const chapterMarks = useMemo(() => chapterList?.map(c => ({start: c.startSeconds, title: c.title})), [chapterList]);
  const markerList = offers.data?.markers?.markers;
  /* While the closing credits run, the next title is offered in a corner; its card stands in for the "Skip credits" button. */
  const upNext = useUpNextDuringCredits(engine, markerList, skipModes.credits ?? 'ask', !isAudio && !state.channel);
  const promptModes = upNext.offer?.marker ? {...skipModes, credits: 'off' as SkipMode, outro: 'off' as SkipMode} : skipModes;
  const segmentMarks = useMemo(() => markerList?.filter(m => m.kind === 'intro' || m.kind === 'credits' || m.kind === 'outro' || m.kind === 'recap').map(m => ({start: m.startSeconds, end: m.endSeconds})), [markerList]);
  const listening = useSyncExternalStore(queue?.listening.subscribe ?? (() => () => {}), () => queue?.listening.getSnapshot() ?? null);
  useSystemListening(service, state, video, identity.item ?? {id: identity.itemId ?? '', libraryId: identity.libraryId ?? '', title: identity.title, kind: identity.kind ?? '', duration: state.duration, progressSeconds: state.positionSeconds, available: true, posterUrl: identity.posterUrl}, queue, api, currentAccessToken, identity.subtitle, chapterList);
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    // Loudness is not linear: squaring the slider's value spreads the audible change along it.
    el.volume = engine.volume * engine.volume;
    el.muted = engine.muted;
  }, [engine.volume, engine.muted, resourceKey]);
  useEffect(() => {
    if (!isAudio || !listening) return;
    setRate(listening.rate);
  }, [isAudio, listening?.rate, resourceKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setBuffering(false);
    setMediaPlaying(false);
    setBuffered(0);
    setOpenBox(null);
  }, [resourceKey]);
  /* A new video playback starts at the profile's default speed; a speed chosen in the player holds
     through the queue (the next episode) and goes when the viewer starts something new. */
  const videoSpeed = playerSessionChoices.get('speed', state.playbackRun, defaultSpeed);
  useEffect(() => {
    if (isAudio || !state.session || state.channel) return;
    setRate(videoSpeed);
    service.setPlaybackRate(videoSpeed);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.session?.id, state.playbackRun, isAudio]);

  const usable = !!state.session && state.phase !== 'error';
  const canHide = visible && !isAudio && mediaPlaying && state.phase === 'ready' && state.intent === 'playing' && !buffering && !state.pendingSeek && !state.error && !notice && !openBox;
  const hideAllowed = useRef(canHide);
  hideAllowed.current = canHide;
  const hideTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const reveal = useCallback(() => {
    setChromeHidden(false);
    clearTimeout(hideTimer.current);
    if (hideAllowed.current) hideTimer.current = setTimeout(() => hideAllowed.current && setChromeHidden(true), 2800);
  }, []);
  useEffect(() => {
    reveal();
    return () => clearTimeout(hideTimer.current);
  }, [canHide, state.intentId, reveal]);
  // What the element is doing, for the spinner and for hiding the controls. Facts about playback
  // itself reach the service through the playback path, never from here.
  const showsMedia = () => !service.getSnapshot().audioEffects.rendering && !!service.getSnapshot().session && service.getSnapshot().phase !== 'error';
  const updateBuffered = (el: HTMLVideoElement) => {
    try {
      const t = el.currentTime;
      for (let i = 0; i < el.buffered.length; i++) if (el.buffered.start(i) <= t && el.buffered.end(i) >= t) { setBuffered(el.buffered.end(i)); return; }
    } catch {}
  };
  /* Brief feedback for a keyboard or wheel action (volume, speed, subtitle timing): it goes by itself. */
  const flashTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const flash = useCallback((text: string) => {
    setNotice(text);
    clearTimeout(flashTimer.current);
    flashTimer.current = setTimeout(() => setNotice(now => (now === text ? '' : now)), 1800);
  }, []);
  useEffect(() => () => clearTimeout(flashTimer.current), []);
  const changeVolume = (delta: number) => {
    const next = Math.max(0, Math.min(1, Math.round(((engine.muted ? 0 : engine.volume) + delta) * 20) / 20));
    engine.setVolume(next);
    engine.setMuted(false);
    flash(i18n.t('web.player.volume.changed', {percent: Math.round(next * 100)}));
  };
  const stepSpeed = (direction: 1 | -1) => {
    const list: readonly number[] = isAudio ? audiobookRates : rates;
    const at = list.indexOf(rate) < 0 ? list.indexOf(1) : list.indexOf(rate);
    const next = list[Math.max(0, Math.min(list.length - 1, at + direction))]!;
    if (next === rate) return;
    setSpeed(next);
    flash(i18n.t('web.player.speed.changed', {speed: next}));
  };
  /** One frame at a time while paused. The element is moved directly: the step is far inside the
   * tolerance a seek is confirmed with, and the position follows from the element's own report. */
  const stepFrame = (direction: 1 | -1) => {
    const el = video.current;
    if (!el || !el.paused || el.readyState < 2 || service.getSnapshot().audioEffects.rendering) return;
    el.currentTime = Math.max(0, el.currentTime + direction * FRAME_SECONDS);
  };
  const stepChapter = (direction: 1 | -1) => {
    const list = chapterList;
    if (!list?.length) return;
    const now = service.getSnapshot(), at = now.pendingSeek?.positionSeconds ?? now.positionSeconds;
    // Back goes to the start of this chapter first, then to the one before it.
    const to = direction > 0 ? list.find(c => c.startSeconds > at + 1) : [...list].reverse().find(c => c.startSeconds < at - 3);
    if (to) engine.seekTo(to.startSeconds); else if (direction < 0) engine.seekTo(0);
  };
  const lastSubtitle = useRef<string | undefined>(undefined);
  const toggleSubtitles = () => {
    const plan = subtitles.snapshot.plan;
    if (!subtitles.service || !plan || subtitles.snapshot.busy) return;
    if (plan.mode === 'track' && !subtitles.snapshot.suppressed) { lastSubtitle.current = plan.selected?.id; void subtitles.turnOff().catch(() => subtitleChangeFailed()); return; }
    const pick = plan.resources.find(r => r.id === lastSubtitle.current && r.enabled) ?? plan.resources.find(r => r.enabled && !burnsIn(r)) ?? plan.resources.find(r => r.enabled);
    if (pick) chooseSubtitle(pick); else flash(i18n.t('web.player.subtitles.none'));
  };
  const nudgeSubtitles = (delta: number) => {
    const next = subtitles.nudge(delta);
    if (next !== undefined) flash(i18n.t('web.player.subtitles.timing', {seconds: next.toFixed(1)}));
  };
  /* Keyboard: the player owns shortcuts while expanded and no text field, menu or dialog has focus. */
  useEffect(() => {
    if (!visible) return;
    const key = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      const target = e.target as HTMLElement | null;
      const field = (target?.closest?.('input,select,textarea,[contenteditable=true]') ?? null) as HTMLElement | null;
      // A slider keeps focus after it is clicked. Only the keys that move it are its own; the
      // rest of the shortcuts go on working.
      const slider = field instanceof HTMLInputElement && field.type === 'range';
      if ((field && !slider) || target?.closest?.('[role="menu"],[role="dialog"]')) return;
      const pressed = shortcutKey(e);
      if (pressed === undefined) return;
      // Space or Enter on a focused button presses that button; it must not toggle playback as well.
      if ((pressed === ' ' || pressed === 'Enter') && target?.closest?.('button,a,summary,[role="button"],[role="menuitemradio"]')) return;
      if (slider && ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End', 'PageUp', 'PageDown'].includes(pressed)) return;
      const live = !!state.linear, now = service.getSnapshot();
      const take = () => { e.preventDefault(); reveal(); };
      switch (pressed) {
        case ' ': case 'k': take(); engine.toggle(); break;
        case 'ArrowLeft': case 'j': take(); engine.seekBy(-skipBack); break;
        case 'ArrowRight': case 'l': take(); engine.seekBy(skipForward); break;
        case 'ArrowUp': case 'ArrowDown':
          take();
          if (live && canSurfChannels()) surf(pressed === 'ArrowUp' ? -1 : 1); else changeVolume(pressed === 'ArrowUp' ? 0.05 : -0.05);
          break;
        case 'PageUp': case 'PageDown':
          if (live) { if (canSurfChannels()) { take(); surf(pressed === 'PageUp' ? -1 : 1); } }
          else if (chapterList?.length) { take(); stepChapter(pressed === 'PageDown' ? 1 : -1); }
          break;
        case 'm': take(); engine.setMuted(!engine.muted); break;
        case 'f': if (!isAudio) { take(); void toggleFullscreen(); } break;
        case 'i': if (!isAudio && pipAvailable) { take(); void pip(); } break;
        case 'c': if (!isAudio && !live) { take(); toggleSubtitles(); } break;
        case 'z': if (!isAudio) { take(); toggleFill(); } break;
        case 'g': if (live) { take(); setMiniGuide(true); } else if (!isAudio) { take(); nudgeSubtitles(-0.1); } break;
        case 'h': if (!isAudio && !live) { take(); nudgeSubtitles(0.1); } break;
        case ',': case '.': if (!isAudio && !live && now.intent === 'paused') { take(); stepFrame(pressed === '.' ? 1 : -1); } break;
        case '<': case '>': if (!isMusic && !live) { take(); stepSpeed(pressed === '>' ? 1 : -1); } break;
        case 'n': case 'p': if (e.shiftKey && !live) { take(); if (pressed === 'n') engine.next(); else engine.previous(); } break;
        case 'Home': if (!live && now.session) { take(); engine.seekTo(0); } break;
        case 'End': if (!live && now.duration > 0) { take(); engine.seekTo(Math.max(0, now.duration - 5)); } break;
        case '0': case '1': case '2': case '3': case '4': case '5': case '6': case '7': case '8': case '9':
          if (!live && now.duration > 0) { take(); engine.seekTo((now.duration * Number(pressed)) / 10); }
          break;
        case '?': take(); setLegend(v => !v); break;
        case 'Escape': if (legend) { setLegend(false); } else if (openBox) { setOpenBox(null); } else if (document.fullscreenElement) { void document.exitFullscreen().catch(() => {}); } else if (isAudio) engine.collapse(); else close(); break;
      }
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  });
  /* Fullscreen. Where the page cannot make its own player full screen (an iPhone), the system's
     full-screen video player is used instead: its own controls, with the same subtitle lines. */
  type AppleVideo = HTMLVideoElement & {webkitEnterFullscreen?: () => void; webkitExitFullscreen?: () => void; webkitDisplayingFullscreen?: boolean};
  const [fullscreen, setFullscreen] = useState(false);
  useEffect(() => {
    const el = video.current as AppleVideo | null;
    const read = () => {
      const on = !!document.fullscreenElement || !!el?.webkitDisplayingFullscreen;
      setFullscreen(on);
      // The system's own full-screen player (an iPhone) shows the video and its text track only.
      setSystemFullscreen(!document.fullscreenElement && !!el?.webkitDisplayingFullscreen);
      // A phone or tablet held upright turns to landscape for the film, where the browser allows.
      const orientation = screen.orientation as (ScreenOrientation & {lock?: (o: string) => Promise<void>}) | undefined;
      try { if (on && matchMedia('(pointer: coarse)').matches) void orientation?.lock?.('landscape').catch(() => {}); else orientation?.unlock?.(); } catch { /* not lockable here */ }
    };
    document.addEventListener('fullscreenchange', read);
    el?.addEventListener('webkitbeginfullscreen', read);
    el?.addEventListener('webkitendfullscreen', read);
    return () => { document.removeEventListener('fullscreenchange', read); el?.removeEventListener('webkitbeginfullscreen', read); el?.removeEventListener('webkitendfullscreen', read); };
  }, []);
  async function toggleFullscreen() {
    const el = video.current as AppleVideo | null;
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else if (stage.current?.requestFullscreen) await stage.current.requestFullscreen();
      else if (el?.webkitDisplayingFullscreen) el.webkitExitFullscreen?.();
      else if (el?.webkitEnterFullscreen) el.webkitEnterFullscreen();
      else setNotice(i18n.t('web.player.fullscreenUnavailable'));
    } catch {
      setNotice(i18n.t('web.player.fullscreenUnavailable'));
    }
  }
  /* Picture in Picture: the browser's own video window. */
  async function pip() {
    const el = video.current;
    try {
      if (document.pictureInPictureElement) { await document.exitPictureInPicture(); return; }
      if (el?.requestPictureInPicture) await el.requestPictureInPicture();
    } catch {
      setNotice(i18n.t('web.player.pipFailed'));
    }
  }
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    /* Safari moves the picture from its own controls too (an iPhone's full-screen player, its
       Picture in Picture button, the swipe home) and says so as a presentation mode. */
    const mode = () => {
      const now = (el as HTMLVideoElement & {webkitPresentationMode?: string}).webkitPresentationMode;
      if (now !== undefined) setSystemFullscreen(now === 'fullscreen' && !document.fullscreenElement);
    };
    el.addEventListener('webkitpresentationmodechanged', mode);
    return () => el.removeEventListener('webkitpresentationmodechanged', mode);
  }, []);
  /* The subtitle chosen here goes to the television with the title. It cannot draw styled or image
     subtitles itself, so the server burns those into its picture (text stays text). Before the
     subtitle list has loaded nothing is said, and the profile's usual choice applies. */
  const castSubtitles = () => {
    const plan = subtitles.snapshot.plan;
    if (!plan) return undefined;
    return {subtitleTrackId: plan.mode === 'track' && !subtitles.snapshot.suppressed && plan.selected ? plan.selected.id : null};
  };
  /** Watch Together from the player: the title goes to the Together page, which plays it once the group has started. */
  function watchTogether() {
    if (!state.itemId) return;
    setPendingTogether({itemId: state.itemId, title: identity.title});
    close();
    void navigate({to: '/together'});
  }
  function close() {
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
    engine.leave();
  }
  // The error state card already says why playback stopped; the toast doesn't repeat it.
  const message = state.linearStalled ? null : engine.queueNotice || (state.error && state.phase !== 'error' ? playbackUserMessage(state.error) : state.seekError ? playbackUserMessage(state.seekError) : null) || (state.recovering ? i18n.t('web.player.reconnecting') : null) || notice || state.audioEffects.notice || (state.pendingSeek ? 'Seeking…' : state.phase === 'starting' ? 'Preparing playback…' : buffering && state.intent === 'playing' ? 'Buffering…' : isAudio ? listening?.message ?? undefined : undefined);
  const linear = state.linear;
  const failed = playbackFailure(state.error);
  const ownerStop = ownerStopOf(state.ended);
  const ownerStopped = !!ownerStop;
  // B-11.27: a recording is taking this channel's tuner (the owner's "Stop watching" setting): the
  // time it goes, then a countdown; at the moment the service stops the channel itself.
  const tunerDeadline = linear?.media.reservationRequiredAtMs;
  const [clock, setClock] = useState(() => Date.now());
  useEffect(() => { if (tunerDeadline === undefined) return; setClock(Date.now()); const timer = setInterval(() => setClock(Date.now()), 1000); return () => clearInterval(timer); }, [tunerDeadline]);
  const handover = state.phase !== 'error' ? tunerHandover(tunerDeadline, clock) : undefined;
  const handoverLines = handover ? tunerHandoverLines(handover, i18n.t, ms => i18n.time(ms)) : undefined;
  const tunerReserved = state.ended?.reason === TUNER_RESERVED;
  // FEAT-07: channel up/down from the guide's lineup.
  const surf = (delta: 1 | -1) => { const next = surfChannel(delta); const raw = next ? legacyChannel(next) : undefined; if (raw) engine.tune(raw); };
  const audioPlan = state.audio;
  // The audio in use: the plan's rendition, or (a server without an audio plan) the track picked from the file's own list.
  const currentAudio = audioPlan.plan?.renditions.find(r => r.id === (audioPlan.pending?.renditionId ?? audioPlan.observedRenditionId))
    ?? (!audioPlan.plan || audioPlan.availability === 'unavailable' ? state.audioTracks?.tracks.find(t => t.streamIndex === (state.audioTracks?.pending ?? state.audioTracks?.selected)) : undefined);
  const subtitlePlan = subtitles.snapshot.plan;
  // PERF-24: the subtitle catalogue is re-read only while subtitles are on or their menu is open.
  const watchSubtitles = subtitlePlan?.mode === 'track' || openBox === 'subtitles';
  useEffect(() => { subtitles.service?.setPolling(watchSubtitles); }, [subtitles.service, watchSubtitles]);
  // The pill names the language ("French", "English · SDH"), not the file's stock track title.
  // Every track reads differently in the menu: two unnamed files in one language gain their format (the shared rule).
  const subtitleNames = useMemo(() => subtitleTrackNames(subtitlePlan?.resources ?? [], i18n.t, i18n.locale), [subtitlePlan?.resources, i18n]);
  const subtitleName = (r: {id: string; title: string; language?: string}) => subtitleNames.get(r.id) ?? (trackName(r, i18n.locale) || r.title);
  const subtitlesOn = subtitlePlan?.mode === 'track' && !subtitles.snapshot.suppressed;
  const currentSubtitle = subtitlesOn ? (subtitlePlan?.selected ? subtitleName(subtitlePlan.selected) : '') || i18n.t('player.opt.on') : i18n.t('player.opt.off');
  // A choice the playing session refused is said, and the subtitles stay as they were.
  const subtitleChangeFailed = () => setNotice(i18n.t('player.subtitles.changeFailed'));
  const offersData = offers.data;
  // The playing source's height, from its offers (PERF-24: the item isn't read again for it).
  const originalHeight = offersData?.sources.find(x => x.id === offersData.current?.sourceId)?.height || identity.item?.sources?.[0]?.height;
  /* MU4 COMPAT-01: 4K/HDR facts from the pre-play read, falling back to the playing source's
   * height already on the session (never a new request). */
  const cachedFacts = engine.sourceFacts(item?.id);
  const sourceFacts = {is4k: !!cachedFacts?.is4k || (originalHeight ?? 0) >= 2100, isHdr: !!cachedFacts?.isHdr};
  // ARCH-API-08: the owner's transcoding switch (from this play's offers) also turns off burned-in subtitles.
  const owner = ownerPlaybackSwitches(offersData);
  /* An image track this browser draws itself is like any text track: nothing is converted, so
     there is nothing to warn about, nothing for the owner's switch to forbid, and no reason to
     prefer another track over the one the viewer picked. */
  const burnsIn = (r: NonNullable<typeof subtitlePlan>['resources'][number]) => isBitmapResource(r) && !subtitles.draws(r);
  const ownerBlocks = (r: NonNullable<typeof subtitlePlan>['resources'][number]) => !owner.subtitleBurnIn && burnsIn(r);
  const chooseSubtitle = (resource: NonNullable<typeof subtitlePlan>['resources'][number]) => {
    const service = subtitles.service;
    if (!service || subtitles.snapshot.busy || !resource.enabled || ownerBlocks(resource)) return;
    // A language with both a text and a bitmap track plays the text track, when the bitmap one would be burned in.
    if (burnsIn(resource)) {
      const text = textTrackForLanguage(subtitlePlan?.resources ?? [], resource.language);
      if (text && text.id !== resource.id) {
        void subtitles.choose(text).catch(subtitleChangeFailed);
        setNotice(i18n.t('player.subtitles.preferredText'));
        setOpenBox(null);
        return;
      }
      if (bitmapWarningNeeded(resource, sourceFacts, bitmapAcknowledged.current.has(resource.id))) {
        setBitmapPending({id: resource.id});
        return;
      }
    }
    void subtitles.choose(resource).catch(subtitleChangeFailed);
    setOpenBox(null);
  };
  const confirmBitmap = () => {
    const pending = bitmapPending ? subtitlePlan?.resources.find(r => r.id === bitmapPending.id) : undefined;
    setBitmapPending(null);
    if (!pending || !subtitles.service) return;
    bitmapAcknowledged.current.add(pending.id);
    void subtitles.choose(pending).catch(subtitleChangeFailed);
    setOpenBox(null);
  };
  /* Pick before play: the title page's audio/subtitle choice, applied by the shared rule
   * (client-core `pending-track-choice`): audio first, then subtitles on the session left standing. */
  const pendingChoice = useRef<PendingChoiceProgress>({});
  useEffect(() => { pendingChoice.current = {choice: state.itemId ? takePendingTrackChoice(state.itemId) : undefined}; }, [state.itemId]);
  // Advances on the changes listed below; the subtitle client and the failure notice are read as they are when it does.
  const advanceChoice = useEffectEvent(() => {
    if (!state.session || pendingChoice.current.choice?.itemId !== state.itemId) return;
    pendingChoice.current = advancePendingChoice(pendingChoice.current, {
      sessionId: state.session.id,
      audio: {availability: audioPlan.availability, renditions: audioPlan.plan?.renditions},
      tracks: state.audioTracks,
      subtitles: subtitles.service && subtitlePlan ? {mode: subtitlePlan.mode, selectedId: subtitlePlan.selected?.id, busy: subtitles.snapshot.busy, resources: subtitlePlan.resources} : undefined,
    }, {
      selectRendition: id => { service.selectAudio(id); },
      selectTrack: stream => { void service.selectAudioTrack(stream); },
      chooseSubtitle: id => { const resource = id ? subtitlePlan?.resources.find(r => r.id === id) : null; if (resource === null) void subtitles.turnOff().catch(subtitleChangeFailed); else if (resource) void subtitles.choose(resource).catch(subtitleChangeFailed); },
    });
  });
  useEffect(() => { advanceChoice(); }, [state.session?.id, state.itemId, audioPlan.plan, audioPlan.availability, state.audioTracks, subtitlePlan, subtitles.service, subtitles.snapshot.busy, service]);
  /* Quality. Original is the default; Automatic starts at Original and moves down, and back up,
     as the connection requires; then the fixed qualities below the source. A choice re-plans the
     playing session where it is (the queue is untouched) and holds for this viewing; the saved
     default (Settings › Quality, one for home and one for away) applies to every new play. */
  const presentation = engine.playback.session(state.session?.id)?.presentation;
  const converting = [presentation?.decision.video, presentation?.decision.audio, presentation?.decision.subtitles].some(d => d?.action === 'transcode' || d?.action === 'burn');
  // What the original needs from the connection: known while the original itself is playing.
  const originalRate = useRef<{itemId?: string; kbps?: number}>({});
  if (originalRate.current.itemId !== item?.id) originalRate.current = {itemId: item?.id};
  if (presentation && !converting && presentation.bitrateKbps) originalRate.current.kbps = presentation.bitrateKbps;
  const originalKbps = originalRate.current.kbps;
  const offeredSteps = useMemo(() => offeredQualities({height: originalHeight, bitrateKbps: originalKbps}), [originalHeight, originalKbps]);
  const qualityFooter = qualityNetworkFooter(offersData?.deliveryPolicy);
  const qualityFooterLine = qualityFooter.network && qualityFooter.height ? i18n.t('quality.networkLimit', {network: i18n.t(qualityFooter.network === 'home' ? 'network.home' : 'network.away'), height: qualityFooter.height}) : '';
  // A viewing that named no quality was started by the server with the viewer's saved default for
  // where this device is (Settings › Quality: one for home, one for away): the quality in force on
  // the session says which. A saved limit at or above this source does not limit it: that is
  // Original for this title. Until there is a session, nothing is assumed.
  const inForce = qualityChoiceOf(presentation?.quality);
  const savedQuality: QualityChoice = inForce && (!fixedQuality(inForce) || offeredSteps.some(q => q.id === inForce)) ? inForce : 'original';
  const qualityChoice: QualityChoice = engine.qualityChoice ?? savedQuality;
  const automaticStep = useSyncExternalStore(engine.playback.subscribeAutomatic, () => engine.playback.automaticAt);
  const stepLabel = (q: FixedQuality) => (q.id === '2160' ? i18n.t('quality.4k') : i18n.t('quality.height', {height: q.height}));
  const automaticWanted = qualityChoice === 'automatic' && !isAudio && !state.linear && !!state.session && owner.transcoding;
  useEffect(() => {
    engine.playback.setAutomatic(automaticWanted ? {steps: offeredSteps, ...(originalKbps ? {originalKbps} : {})} : null);
  }, [engine.playback, automaticWanted, offeredSteps, originalKbps, state.itemId]);
  useEffect(() => () => engine.playback.setAutomatic(null), [engine.playback]);
  useEffect(() => engine.playback.onNotice(kind => {
    if (kind === 'autoplayBlocked') { setNotice(i18n.t('web.player.pressPlay')); return; }
    const at = engine.playback.automaticAt;
    flash(i18n.t(kind === 'qualityLowered' ? 'web.player.quality.lowered' : 'web.player.quality.raised', {quality: at ? stepLabel(at) : i18n.t('player.quality.original')}));
  }), [engine.playback, i18n, flash]); // eslint-disable-line react-hooks/exhaustive-deps
  const chooseQuality = (choice: QualityChoice) => {
    setOpenBox(null);
    if (choice === qualityChoice) return;
    engine.setQualityChoice(choice);
    engine.playback.forgetAutomaticStep();
    void service.changePlayback({quality: qualityRequest(choice)}).catch(() => setNotice(i18n.t('web.player.quality.changeFailed')));
  };
  const chosenFixed = fixedQuality(qualityChoice);
  const currentSource = offersData?.sources.find(x => x.id === offersData.current?.sourceId) ?? offersData?.sources[0];
  const infoSource = currentSource ? {container: currentSource.container, videoCodec: currentSource.videoCodec, audioCodec: currentSource.audioCodec, width: currentSource.width, height: currentSource.height} : undefined;
  const qualityLabelNow = !state.session ? '—' : qualityChoice === 'automatic' ? (automaticStep ? i18n.t('web.player.quality.automaticAt', {quality: stepLabel(automaticStep)}) : i18n.t('player.quality.automatic')) : chosenFixed ? stepLabel(chosenFixed) : i18n.t('player.quality.original');
  const deliveredAs = chosenFixed ?? (qualityChoice === 'automatic' ? automaticStep : undefined);
  const qualityContent = () => (
    <>
      {optionRow(i18n.t('player.quality.original'), qualityChoice === 'original', () => chooseQuality('original'), qualityChoice === 'original' ? undefined : i18n.t('player.quality.originalHelp'))}
      {optionRow(i18n.t('player.quality.automatic'), qualityChoice === 'automatic', () => chooseQuality('automatic'), !owner.transcoding ? i18n.t('player.ownerOff') : qualityChoice === 'automatic' ? undefined : i18n.t('web.player.quality.automaticHelp'), !owner.transcoding)}
      {offeredSteps.map(q => optionRow(stepLabel(q), qualityChoice === q.id, () => chooseQuality(q.id), !owner.transcoding ? i18n.t('player.ownerOff') : qualityChoice === q.id ? undefined : i18n.t('web.player.quality.rate', {mbps: q.videoKbps >= 1000 ? Math.round(q.videoKbps / 100) / 10 : q.videoKbps / 1000}), !owner.transcoding))}
      {presentation ? <Text variant="caption" tone="tertiary">{[qualityFooterLine, converting ? (deliveredAs ? i18n.t('player.quality.converting', {quality: stepLabel(deliveredAs)}) : i18n.t('web.player.quality.convertingForDevice')) : i18n.t('player.quality.playingOriginal'), offersData && !offersData.transcodingEnabled ? i18n.t('player.quality.conversionOff') : ''].filter(Boolean).join(' · ')}</Text> : null}
      {state.audioEffects.rendering && state.session?.audio?.downmixed ? <Text variant="caption" tone="secondary">{i18n.t('player.opt.downmixed')}</Text> : null}
    </>
  );
  const [videoSleep, setVideoSleep] = useState<SleepChoice>('off');
  const sleepAt = state.listeningDeadlines?.sleepAtMs;
  useEffect(() => { if (!sleepAt) setVideoSleep('off'); }, [sleepAt]);
  const sleepChoiceLabel = (c: SleepChoice) => (c === 'off' ? i18n.t('player.opt.off') : c === 'item' ? i18n.t('web.player.sleep.endOfItem') : i18n.t('player.opt.minutes', {count: c}));
  const sleepLabel = isAudio ? listening?.timer?.label ?? i18n.t('player.opt.off') : sleepAt ? i18n.t('player.opt.minutes', {count: Math.max(1, Math.ceil((sleepAt - Date.now()) / 60000))}) : i18n.t('player.opt.off');
  const hidden = chromeHidden && canHide;
  const setSpeed = (value: number) => {
    setRate(value);
    if (isAudio) void queue?.listening.setRate(value).catch(e => setNotice(errorText(e, 'playback', 'action')));
    else { playerSessionChoices.set('speed', state.playbackRun, value); service.setPlaybackRate(value); }
    setOpenBox(null);
  };
  const setSleep = (choice: SleepChoice) => {
    if (isAudio) void queue?.listening.setTimer(choice).catch(e => setNotice(errorText(e, 'playback', 'action')));
    else {
      // "End of this title" is what is left of it at the speed it is playing.
      setVideoSleep(choice);
      void service.setListeningDeadline('sleep', choice === 'off' ? null : typeof choice === 'number' ? choice : Math.max(1, Math.round((state.duration - state.positionSeconds) / Math.max(0.25, rate) / 60))).catch(() => setVideoSleep('off'));
    }
    setOpenBox(null);
  };
  const [sheet, setSheet] = useState(false);
  /* MU4 FEAT-07: the mini guide overlay and the in-player Record action (More menu, never a new top-bar button). */
  const [miniGuide, setMiniGuide] = useState(false);
  const dvr = usePlayerDVR();
  const tunedChannel = tunedChannelFor(state.channel);
  const liveProgramme = tunedChannel ? currentProgramme(tunedChannel) : null;
  const canRecordLive = !!linear && !!dvr && !!tunedChannel && tunedChannel.recordAvailable && !!liveProgramme;
  const recordLive = () => {
    if (!dvr || !tunedChannel || !liveProgramme) { setNotice(i18n.t('guide.cantRecord')); return; }
    void dvr.schedule({channel: tunedChannel, programme: liveProgramme, series: false}, defaultRecordingOptions)
      .then(() => setNotice(i18n.t('guide.recordedShortcut', {title: liveProgramme.title})))
      .catch(e => setNotice(errorText(e, 'live', 'save')));
  };
  /* MU4 FEAT-07: the live label reads the programme's wall-clock times, plus how far behind live. */
  const timeBare = (ms: number) => {
    try {
      const parts = new Intl.DateTimeFormat(i18n.locale, {hour: 'numeric', minute: '2-digit'}).formatToParts(new Date(ms));
      return parts.filter(p => p.type !== 'dayPeriod').map(p => p.value).join('').replace(/\s+/g, ' ').trim() || i18n.time(ms);
    } catch { return i18n.time(ms); }
  };
  const liveBehindSec = linear ? channelTimeline(linear, state.positionSeconds).behind : 0;
  const liveProgrammeLabel = linear?.programme ? programmeRangeLabel(linear.programme.startMs, linear.programme.endMs, (ms, meridiem) => (meridiem ? i18n.time(ms) : timeBare(ms))) : undefined;
  const liveBehindLabel = isBehindLive(liveBehindSec) ? i18n.t('player.live.behindLive', {duration: formatBehindSec(liveBehindSec)}) : undefined;
  /* MU4 COMPAT-01: a bitmap track on a 4K/HDR source asks once per play (like the Dolby warning).
   * Cancel keeps the current track: nothing is chosen until Continue. */
  const [bitmapPending, setBitmapPending] = useState<{id: string} | null>(null);
  const bitmapAcknowledged = useRef(new Set<string>());
  useEffect(() => { bitmapAcknowledged.current.clear(); setBitmapPending(null); }, [resourceKey]);
  /* The picture. A mouse click plays or pauses, after a moment in which a second click makes it a
     double-click (fullscreen) instead, so a double-click never blips a pause. A finger does not
     pause: a tap shows the controls, or hides them when they are showing; a double tap on the left
     or right third skips back or forward, and in the middle goes fullscreen. */
  const pointerKind = useRef<string>('mouse');
  const hiddenAtDown = useRef(false);
  const clickTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const lastTap = useRef(0);
  useEffect(() => () => clearTimeout(clickTimer.current), []);
  const tap = (e: React.MouseEvent<HTMLDivElement>) => {
    const kind = (e.nativeEvent as PointerEvent).pointerType || pointerKind.current;
    if (kind === 'mouse') {
      clearTimeout(clickTimer.current);
      if (e.detail > 1) return;
      clickTimer.current = setTimeout(() => engine.toggle(), 220);
      return;
    }
    const now = Date.now(), rect = e.currentTarget.getBoundingClientRect(), x = rect.width > 0 ? (e.clientX - rect.left) / rect.width : 0.5;
    if (now - lastTap.current < 320) {
      lastTap.current = 0;
      if (x < 0.35 && !linear) engine.seekBy(-skipBack);
      else if (x > 0.65 && !linear) engine.seekBy(skipForward);
      else void toggleFullscreen();
      reveal();
      return;
    }
    lastTap.current = now;
    // The press itself has already brought hidden controls back; a tap while they show puts them away.
    if (!hiddenAtDown.current && hideAllowed.current) { clearTimeout(hideTimer.current); setChromeHidden(true); }
  };
  const box = (id: string, icon: IconName, label: string, value: string, content: React.ReactNode, disabled = false) =>
    compact ? (
      <details key={id} className={s.sheetSection} open={openBox === id} onToggle={e => setOpenBox((e.currentTarget as HTMLDetailsElement).open ? id : openBox === id ? null : openBox)}>
        <summary className={s.sheetSummary}><Icon name={icon} size={18} /><span>{label}</span><strong>{value}</strong><Icon name="chevronDown" size={16} /></summary>
        <div className={s.panel} role="group" aria-label={label}>{content}</div>
      </details>
    ) : (
      <Popover key={id} open={openBox === id} onOpenChange={o => setOpenBox(o ? id : null)} side="top" align="center" trigger={<button type="button" className={s.box} disabled={disabled} aria-label={`${label}: ${value}`}><Icon name={icon} size={16} />{label} <strong>{value}</strong></button>}>
        <div className={s.panel} role="group" aria-label={label}>
          <span className={s.panelTitle}>{label}</span>
          {content}
        </div>
      </Popover>
    );
  /* An image subtitle with some lines marked forced (a disc's full track with foreign dialogue
     flagged): the viewer may show those lines alone. Kept on this device. */
  const forcedOnlyRow = (
    <button key="forced-only" type="button" role="menuitemcheckbox" aria-checked={forcedOnly} className={s.option} title={i18n.t('player.subtitles.forcedOnlyHelp')} onClick={toggleForcedOnly}><span>{i18n.t('player.subtitles.forcedOnly')}</span>{forcedOnly ? <Icon name="check" size={16} /> : null}</button>
  );
  const optionRow = (label: string, checked: boolean, onSelect: () => void, meta?: string, disabled?: boolean) => (
    <button key={label + meta} type="button" role="menuitemradio" aria-checked={checked} className={s.option} onClick={onSelect} disabled={disabled}><span>{label}</span>{meta ? <span className={s.optionMeta}>{meta}</span> : checked ? <Icon name="check" size={16} /> : null}</button>
  );
  return (
    <div ref={stage} className={cx(s.stage, isAudio && s.audio, hidden && s.hidden)} hidden={!visible} aria-label={i18n.t('player.stageLabel', {title: identity.title})} role="region" onPointerMove={e => { if (e.pointerType === 'mouse') reveal(); }} onPointerDown={e => { pointerKind.current = e.pointerType || 'mouse'; hiddenAtDown.current = chromeHidden && canHide; reveal(); }} tabIndex={-1}>
      {isAudio ? <Backdrop path={identity.backdropUrl ?? identity.posterUrl} height="100%" opacity={0.35} position="center" /> : null}
      <video
        ref={video}
        className={cx(s.video, fill && !isAudio && s.fill)}
        playsInline
        preload="metadata"
        onTimeUpdate={e => { if ((bufferedTick.current = (bufferedTick.current + 1) % 8) === 0) updateBuffered(e.currentTarget); }}
        onProgress={e => updateBuffered(e.currentTarget)}
        onWaiting={() => { if (showsMedia()) { setBuffering(true); setMediaPlaying(false); } }}
        onPlaying={() => { if (showsMedia()) { setBuffering(false); setMediaPlaying(true); } }}
        onEmptied={() => { setBuffering(false); setMediaPlaying(false); }}
        onPause={() => { setBuffering(false); setMediaPlaying(false); }}
      />
      {/* Always in the page, like the video element: the overlay is made once for the player's life. Hidden unless a drawn subtitle is on. */}
      <canvas ref={subtitleCanvas} className={s.subtitleCanvas} aria-hidden />
      {!isAudio ? <div className={s.tap} onClick={tap} onDoubleClick={() => { if (pointerKind.current === 'mouse') { clearTimeout(clickTimer.current); void toggleFullscreen(); } }} onWheel={e => { if (!openBox && e.deltaY !== 0) changeVolume(e.deltaY < 0 ? 0.05 : -0.05); }} aria-hidden /> : null}
      {(buffering || state.recovering) && state.intent === 'playing' && !state.error ? <div className={s.spinner} aria-hidden /> : null}
      {!isAudio ? <PostPlay engine={engine} onClose={close} /> : null}
      {!isAudio && !linear && state.phase === 'ready' ? <SkipPrompt markers={offers.data?.markers ?? null} position={state.positionSeconds} sessionId={state.session?.id} modes={promptModes} onSeek={engine.seekTo} onSkipped={(id, mode, at) => engine.service.markerSkipped(id, mode, at)} /> : null}
      {upNext.offer && upNext.next ? <UpNextCredits engine={engine} offer={upNext.offer} next={upNext.next} postPlay={upNext.postPlay} rate={rate} onDismiss={upNext.dismiss} /> : null}
      {linear && state.linearStalled && state.phase !== 'error' ? (
        // A channel start that can't proceed says why, instead of "Preparing playback…" forever.
        <div className={s.errorState}>
          <StateView icon="warning" title={i18n.t('web.channelProblem.title', {channel: linear.name})} body={i18n.t(channelProblemId(linear.media.errorCode) as never)} action={{label: i18n.t('action.tryAgain'), onClick: engine.retry}} secondaryAction={{label: i18n.t('web.channelProblem.backToGuide'), onClick: () => { close(); void navigate({to: '/channels'}); }}} />
        </div>
      ) : null}
      {ownerStopped ? (
        // The server owner stopped this playback (v1 or v2): say so, and what they wrote; nothing retries.
        <div className={s.errorState} role="alert">
          <StateView icon="info" title={i18n.t('web.player.ownerStopped')} body={ownerStop?.message ? i18n.t('web.player.ownerStopped.message', {message: ownerStop.message}) : undefined} action={{label: i18n.t(isAudio ? 'action.close' : 'action.back'), onClick: close}} />
        </div>
      ) : null}
      {tunerReserved ? (
        // A recording took the tuner: said once, with the way back to the guide; nothing retries.
        <div className={s.errorState} role="alert">
          <StateView icon="info" title={i18n.t('live.handover.stoppedTitle')} body={i18n.t('live.handover.stopped')} action={{label: i18n.t('web.channelProblem.backToGuide'), onClick: () => { close(); void navigate({to: '/channels'}); }}} secondaryAction={{label: i18n.t('action.close'), onClick: close}} />
        </div>
      ) : null}
      {handoverLines ? (
        <div className={s.handover} role={handover?.kind === 'countdown' ? 'alert' : 'status'} aria-live={handover?.kind === 'countdown' ? 'assertive' : 'polite'}>
          <strong>{handoverLines[0]}</strong>
          <span>{handoverLines[1]}</span>
        </div>
      ) : null}
      {state.phase === 'error' && !ownerStopped && !tunerReserved ? (
        // Two honest cases: something another try can fix, and a title that cannot be played here at all.
        <div className={s.errorState} role="alert">
          {failed.permanent
            ? <StateView icon="info" title={i18n.t(failed.title)} body={failed.body ? i18n.t(failed.body) : failed.text} action={{label: i18n.t(isAudio ? 'action.close' : 'action.back'), onClick: close}} secondaryAction={{label: i18n.t('web.player.error.report'), onClick: () => setReport(true)}} />
            : <StateView icon="warning" title={i18n.t(failed.title)} body={failed.body ? i18n.t(failed.body) : failed.text} action={{label: i18n.t('action.tryAgain'), onClick: engine.retry}} secondaryAction={{label: i18n.t(isAudio ? 'action.close' : 'action.back'), onClick: close}} />}
        </div>
      ) : null}
      {isAudio ? (
        <div className={s.nowPlaying}>
          {/* The cover is centred with the title under it, once; lyrics sit beside it. */}
          <div className={s.npMain}>
            <div className={s.cover}><Artwork path={identity.posterUrl} shape="square" icon={isMusic ? 'music' : 'book'} alt="" /></div>
            <div className={s.npIdentity}>
              <h1 className={s.npTitle}>{identity.title}</h1>
              {identity.subtitle ? <span className={s.npSub}>{identity.subtitle}</span> : null}
            </div>
          </div>
          {isMusic&&item&&prefs.value<boolean>('playback.showSyncedLyrics',true)?<Lyrics className={s.npLyrics} api={api} scope={scope} service={service} libraryId={item.libraryId}/>:null}
        </div>
      ) : null}
      <div className={cx(s.chrome, hidden && s.faded)} aria-hidden={hidden}>
        <header className={s.top}>
          <div className={s.identity}>
            {/* Now Playing says the title once, under the cover. */}
            {isAudio ? null : <span className={s.title}>{linear ? linear.name : identity.title}</span>}
            <span className={s.subtitle}>{isAudio ? null : linear ? [i18n.t('player.live.onNow', {title: linear.programme?.title ?? i18n.t('player.live.noProgramme')}), linear.next ? i18n.t('player.live.next', {title: linear.next.title}) : null].filter(Boolean).join(' · ') : identity.subtitle}</span>
            <GroupPill />
          </div>
          <div className={s.topActions}>
            {linear && !isAudio ? <LiveCaptions video={video.current} /> : null}
            {linear && canSurfChannels() ? <><IconButton name="chevronUp" label={i18n.t('web.player.channelUp')} variant="ghost" onClick={() => surf(-1)} /><IconButton name="chevronDown" label={i18n.t('web.player.channelDown')} variant="ghost" onClick={() => surf(1)} /></> : null}
            {!linear && state.itemId ? <PlayOnButton castAvailable={cast.state.available} video={video.current} onCast={() => { const itemId = state.itemId!, live = service.getSnapshot(), at = live.pendingSeek?.positionSeconds ?? live.positionSeconds; const meta = {title: identity.title, subtitle: identity.subtitle, kind: identity.kind ?? identity.item?.kind, artworkPath: identity.posterUrl ?? identity.item?.posterUrl}; noteCastMeta(itemId, meta); void cast.sender?.start(itemId, meta, at, castSubtitles()).then(ok => { if (ok) engine.leave(); }); }} /> : null}
            {!isAudio && pipAvailable ? <IconButton name="pip" label={i18n.t('web.player.pip')} variant="ghost" onClick={() => void pip()} /> : null}
            {/* WEB-PLAYER-05: Report a problem is a flag in a More menu, not a warning sign in the bar.
                MU4 FEAT-07: linear sessions get Guide (mini guide) and Record here, never new top-bar buttons. */}
            <Menu label={i18n.t('player.more')} trigger={<IconButton name="more" label={i18n.t('player.more')} variant="ghost" />} items={[...(linear ? [{id: 'guide', label: i18n.t('player.live.guide'), icon: 'channels' as IconName}, ...(canRecordLive ? [{id: 'record', label: i18n.t('guide.record'), icon: 'dvr' as IconName}] : [])] : [...(!isAudio && !inRoom && state.itemId ? [{id: 'together', label: i18n.t('player.watchTogether'), icon: 'people' as IconName}] : []), ...(!isAudio ? [{id: 'fit', label: i18n.t(fill ? 'web.player.fit.contain' : 'web.player.fit.fill'), icon: 'fullscreen' as IconName}] : []), {id: 'info', label: i18n.t('player.opt.info'), icon: 'info' as IconName}, {id: 'report', label: i18n.t('player.reportProblem'), icon: 'flag' as IconName}]), ...(!compact ? [{id: 'shortcuts', label: i18n.t('player.shortcuts'), icon: 'keyboard' as IconName}] : [])]} onSelect={id => (id === 'report' ? setReport(true) : id === 'guide' ? setMiniGuide(true) : id === 'record' ? recordLive() : id === 'info' ? setInfo(v => !v) : id === 'together' ? watchTogether() : id === 'fit' ? toggleFill() : setLegend(true))} />
            {!isAudio ? <IconButton name={fullscreen ? 'exitFullscreen' : 'fullscreen'} label={i18n.t(fullscreen ? 'web.player.exitFullscreen' : 'web.player.fullscreen')} variant="ghost" onClick={() => void toggleFullscreen()} /> : null}
            {isAudio ? <IconButton name="chevronDown" label={i18n.t('player.minimize')} variant="ghost" onClick={engine.collapse} /> : null}
            <IconButton name="close" label={i18n.t('player.close')} variant="ghost" onClick={close} />
          </div>
        </header>
        <div className={s.bottom} ref={bottom}>
          {linear ? (
            <LiveTimeline service={service} pending={false} duration={state.duration} disabled={!usable} onSeek={engine.seekTo} live onLive={() => service.goLive()} programmeLabel={liveProgrammeLabel} behindLabel={liveBehindLabel} />
          ) : (
            <LiveTimeline service={service} duration={state.duration} buffered={buffered} disabled={!usable} onSeek={engine.seekTo} trickplay={trickplay} step={skipForward} chapters={chapterMarks} segments={segmentMarks} />
          )}
          <div className={s.transport}>
            <div className={cx(s.cluster, s.clusterLeft)}>
              {!compact && queue && isAudio ? <QueuePreference queue={queue} kind="shuffle" onError={setNotice} /> : null}
            </div>
            <div className={s.cluster}>
              {engine.canPrevious && queueCount > 1 ? <button type="button" className={s.control} aria-label={i18n.t('player.previous')} disabled={!usable} onClick={engine.previous}><Icon name="previous" size={22} /></button> : null}
              {isMusic ? null : <button type="button" className={s.control} aria-label={i18n.t('player.skipBackSeconds', {seconds: skipBack})} disabled={!usable} onClick={() => engine.seekBy(-skipBack)}><Icon name="back10" size={24} /><span className={s.seekBadge} aria-hidden>{skipBack}</span></button>}
              <button type="button" className={cx(s.control, s.play)} aria-label={hostControls ? i18n.t('web.together.pill.hostControls') : i18n.t(state.intent === 'playing' ? 'action.pause' : 'action.play')} title={hostControls ? i18n.t('web.together.pill.hostControls') : undefined} disabled={hostControls || (!state.session && state.phase !== 'starting' && state.phase !== 'error')} onClick={engine.toggle}><Icon name={state.phase === 'error' ? 'refresh' : state.intent === 'playing' ? 'pause' : 'play'} size={28} /></button>
              {isMusic ? null : <button type="button" className={s.control} aria-label={i18n.t('player.skipForwardSeconds', {seconds: skipForward})} disabled={!usable} onClick={() => engine.seekBy(skipForward)}><Icon name="forward10" size={24} /><span className={s.seekBadge} aria-hidden>{skipForward}</span></button>}
              {engine.canNext ? <button type="button" className={s.control} aria-label={i18n.t('player.next')} disabled={!usable} onClick={engine.next}><Icon name="next" size={22} /></button> : null}
            </div>
            <div className={s.clusterRight}>
              {!compact && queue && isAudio ? <QueuePreference queue={queue} kind="repeat" onError={setNotice} /> : null}
              <div className={s.volume}>
                <button type="button" className={s.control} aria-label={i18n.t(engine.muted ? 'player.unmute' : 'player.mute')} onClick={() => engine.setMuted(!engine.muted)}><Icon name={engine.muted || engine.volume === 0 ? 'mute' : 'audio'} size={20} /></button>
                <input className={s.volumeRange} type="range" min={0} max={1} step={0.05} value={engine.muted ? 0 : engine.volume} aria-label={i18n.t('player.volume')} onChange={e => { engine.setVolume(Number(e.target.value)); engine.setMuted(false); }} />
              </div>
            </div>
          </div>
          {!linear && compact ? (
            <div className={s.options}>
              <button type="button" className={s.box} onClick={() => setSheet(true)} aria-label={i18n.t('player.opt.menu')}><Icon name="settings" size={16} />{i18n.t('player.opt.short')} <strong>{[currentAudio?.label, !isAudio && subtitlesOn ? i18n.t('player.opt.subtitlesNamed', {name: currentSubtitle}) : null, rate === 1 ? null : `${rate}×`].filter(Boolean).join(' · ') || i18n.t(isAudio ? 'player.opt.summaryListening' : 'player.opt.summaryWatching')}</strong></button>
            </div>
          ) : null}
          {/* WEB-PLAYER-03: the row stays mounted while the chrome fades, so the first click lands on a box, not the video. */}
          {!linear ? (
            <div className={cx(s.options, compact && s.optionsSheet)} hidden={compact && !sheet}>
              {box('audio', 'audio', i18n.t('player.opt.audio'), audioPlan.availability === 'loading' ? '…' : currentAudio?.label ?? i18n.t('player.opt.defaultTrack'), (
                <>
                  {(!audioPlan.plan || audioPlan.availability==='unavailable') ? state.audioTracks?.tracks.map(t => optionRow(t.label,t.streamIndex===(state.audioTracks?.pending??state.audioTracks?.selected),()=>{void service.selectAudioTrack(t.streamIndex).catch(()=>setNotice(i18n.t('web.player.audio.changeFailed')));setOpenBox(null);},undefined,state.audioTracks?.pending!==undefined)) : null}
                  {audioPlan.availability!=='unavailable' && audioPlan.plan?.renditions.map(r => optionRow(r.label, r.id === (audioPlan.pending?.renditionId ?? audioPlan.observedRenditionId), () => { service.selectAudio(r.id); setOpenBox(null); }, (r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id))?i18n.t('player.opt.unavailable'):undefined, !!audioPlan.pending||!!r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id)))}
                  {!audioPlan.plan?.renditions.length ? <Text variant="caption" tone="tertiary">{audioPlan.reason ?? i18n.t('player.opt.oneAudioTrack')}</Text> : null}
                  {audioPlan.failed && !audioPlan.failed.unavailable ? <Button size="sm" variant="outline" label={i18n.t('action.tryAgain')} onClick={() => service.selectAudio(audioPlan.failed!.renditionId)} /> : null}
                  {state.session?.mode === 'hls' ? <Button size="sm" variant="ghost" icon="refresh" label={i18n.t('action.refresh')} onClick={refreshAudio} /> : null}
                </>
              ))}
              {!isAudio ? box('subtitles', 'subtitles', i18n.t('player.opt.subtitles'), currentSubtitle, (
                <>
                  {subtitles.service && subtitlePlan ? (
                    <>
                      {optionRow(i18n.t('player.opt.off'), subtitlePlan.mode === 'off', () => { void subtitles.turnOff().catch(subtitleChangeFailed); setOpenBox(null); })}
                      {(subtitlePlan.resources ?? []).map(r => optionRow(subtitleName(r), !subtitles.snapshot.suppressed && subtitlePlan.selected?.id === r.id, () => chooseSubtitle(r), !r.enabled ? subtitleReason(r.reason) : ownerBlocks(r) ? i18n.t('player.ownerOff') : burnsIn(r) ? i18n.t('player.subtitles.bitmapNote') : i18n.t('player.opt.origin', {origin: r.origin}), subtitles.snapshot.busy || !r.enabled || ownerBlocks(r)))}
                      {(subtitlePlan.discovered ?? []).filter(d => !(subtitlePlan.resources ?? []).some(r => r.discoveryId === d.id && r.enabled)).map(d => optionRow(trackName(d, i18n.locale) || d.title, false, () => { void subtitles.add(d).catch(subtitleChangeFailed); setOpenBox(null); }, d.enabled ? i18n.t('player.opt.add') : subtitleReason(d.reason), subtitles.snapshot.busy || !d.enabled))}
                      {subtitles.snapshot.catalog?.provider.enabled ? (
                        subtitleSearch ? (
                          subtitles.snapshot.busy && !subtitles.snapshot.candidates.length ? <Text variant="caption" tone="tertiary">{i18n.t('player.opt.lookingForSubtitles')}</Text>
                            : subtitles.snapshot.candidates.length ? subtitles.snapshot.candidates.map(c => optionRow(`${c.title} · ${c.language}`, false, () => { void subtitles.service!.apply(c, 'personal', 'Downloaded by the viewer for personal viewing').then(() => setSubtitleSearch(false), () => {}); }, c.attribution, subtitles.snapshot.busy))
                            : <Text variant="caption" tone="tertiary">{i18n.t('player.opt.noSubtitlesFound')}</Text>
                        ) : optionRow(i18n.t('player.opt.findSubtitles'), false, () => { setSubtitleSearch(true); void subtitles.service!.search().catch(() => {}); })
                      ) : null}
                      {subtitlePlan.selected ? <div style={{display: 'flex', gap: 4, alignItems: 'center', flexWrap: 'wrap'}} title={i18n.t('player.opt.timingHelp')}><Text variant="caption" tone="tertiary">{i18n.t('player.opt.timing')} {i18n.t('player.opt.offsetSeconds', {seconds: subtitles.offsetSeconds.toFixed(1)})}</Text>{[-0.5, -0.1, 0.1, 0.5].map(step => <Button key={step} size="sm" variant="ghost" label={(step > 0 ? '+' : '−') + Math.abs(step)} onClick={() => { subtitles.nudge(step); }} />)}</div> : null}
                      {subtitles.forcedChoice ? forcedOnlyRow : null}
                      {subtitles.drawnKind ? <div style={{display: 'flex', gap: 4, alignItems: 'center', flexWrap: 'wrap'}} title={i18n.t('player.subtitles.positionHelp')}><Text variant="caption" tone="tertiary">{subtitles.drawing ? i18n.t('player.subtitles.loadingDrawn') : i18n.t('player.subtitles.position')}</Text><Button size="sm" variant="ghost" label={i18n.t('player.subtitles.moveUp')} onClick={() => moveSubtitles(2)} /><Button size="sm" variant="ghost" label={i18n.t('player.subtitles.moveDown')} onClick={() => moveSubtitles(-2)} />{subtitleRaise !== 0 ? <Button size="sm" variant="ghost" label={i18n.t('player.subtitles.positionReset')} onClick={() => moveSubtitles(null)} /> : null}</div> : null}
                    </>
                  ) : <Text variant="caption" tone="tertiary">{subtitles.snapshot.loading ? i18n.t('player.opt.loadingSubtitles') : subtitles.snapshot.error ?? subtitles.rendererError ?? i18n.t('player.opt.subtitlesLater')}</Text>}
                </>
              )) : null}
              {!isAudio ? box('quality', 'quality', i18n.t('player.opt.quality'), qualityLabelNow, qualityContent()) : null}
              {isMusic ? null : box('speed', 'speed', i18n.t('player.opt.speed'), rate === 1 ? i18n.t('player.opt.normalSpeed') : `${rate}×`, <div className={s.speedGrid}>{(isAudio ? [...audiobookRates] : rates).map(v => optionRow(v === 1 ? i18n.t('player.opt.normalSpeed') : `${v}×`, v === rate, () => setSpeed(v)))}</div>)}
              {box('sleep', 'sleep', i18n.t('player.opt.sleep'), sleepLabel, (
                <>
                  {sleepChoices.map(c => optionRow(sleepChoiceLabel(c), (isAudio ? listening?.timer?.choice ?? 'off' : sleepAt ? videoSleep : 'off') === c, () => setSleep(c)))}
                </>
              ))}
              {queueCount > 1 && queue ? box('queue', 'queue', i18n.t('player.queue'), String(queueCount - 1), <QueueList queue={queue} />) : null}
              {!isAudio && chapters.data?.status === 'available' && chapters.data.chapters.length ? box('chapters', 'chapters', i18n.t('player.opt.chapters'), String(chapters.data.chapters.length), (
                <div className={s.chapters}>
                  {chapters.data?.status === 'available' ? chapters.data.chapters.map(ch => (
                    <button key={ch.id} type="button" role="menuitemradio" aria-checked={state.positionSeconds >= ch.startSeconds && state.positionSeconds < ch.endSeconds} className={s.option} onClick={() => { service.seek(ch.action.positionSeconds); setOpenBox(null); }}>
                      <span className={s.chapterRow}>{ch.thumbnailUrl ? <span className={s.chapterThumb}><Artwork path={ch.thumbnailUrl} shape="landscape" alt="" /></span> : null}<span>{ch.index}. {ch.title}</span></span>
                      <span className={s.optionMeta}>{formatClock(ch.startSeconds)}</span>
                    </button>
                  )) : <Text variant="caption" tone="tertiary">{i18n.t(chapters.state.phase === 'loading' ? 'player.opt.loadingChapters' : chapters.data?.status === 'none' ? 'player.opt.noChapters' : 'player.opt.chaptersUnavailable')}</Text>}
                </div>
              )) : null}
            </div>
          ) : null}
        </div>
        {compact ? (
          <Dialog open={sheet} onOpenChange={setSheet} title={i18n.t('player.opt.menu')} description={identity.title}>
            <div className={s.sheetList}>
              {box('audio', 'audio', i18n.t('player.opt.audio'), currentAudio?.label ?? i18n.t('player.opt.defaultTrack'), (
                <>
                  {(!audioPlan.plan || audioPlan.availability==='unavailable') ? state.audioTracks?.tracks.map(t => optionRow(t.label,t.streamIndex===(state.audioTracks?.pending??state.audioTracks?.selected),()=>{void service.selectAudioTrack(t.streamIndex).catch(()=>setNotice(i18n.t('web.player.audio.changeFailed')));setOpenBox(null);},undefined,state.audioTracks?.pending!==undefined)) : null}
                  {audioPlan.availability!=='unavailable' && audioPlan.plan?.renditions.map(r => optionRow(r.label, r.id === (audioPlan.pending?.renditionId ?? audioPlan.observedRenditionId), () => { service.selectAudio(r.id); setOpenBox(null); }, (r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id))?i18n.t('player.opt.unavailable'):undefined, !!audioPlan.pending||!!r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id)))}
                  {!audioPlan.plan?.renditions.length ? <Text variant="caption" tone="tertiary">{audioPlan.reason ?? i18n.t('player.opt.oneAudioTrack')}</Text> : null}
                </>
              ))}
              {!isAudio ? box('subtitles', 'subtitles', i18n.t('player.opt.subtitles'), currentSubtitle, subtitles.service && subtitlePlan ? (
                <>
                  {optionRow(i18n.t('player.opt.off'), subtitlePlan.mode === 'off', () => { void subtitles.turnOff().catch(subtitleChangeFailed); })}
                  {(subtitlePlan.resources ?? []).map(r => optionRow(subtitleName(r), !subtitles.snapshot.suppressed && subtitlePlan.selected?.id === r.id, () => chooseSubtitle(r), !r.enabled ? subtitleReason(r.reason) : ownerBlocks(r) ? i18n.t('player.ownerOff') : burnsIn(r) ? i18n.t('player.subtitles.bitmapNote') : undefined, subtitles.snapshot.busy || !r.enabled || ownerBlocks(r)))}
                  {subtitles.forcedChoice ? forcedOnlyRow : null}
                </>
              ) : <Text variant="caption" tone="tertiary">{i18n.t('player.opt.subtitlesLater')}</Text>) : null}
              {!isAudio ? box('quality', 'quality', i18n.t('player.opt.quality'), qualityLabelNow, qualityContent()) : null}
              {isMusic ? null : box('speed', 'speed', i18n.t('player.opt.speed'), rate === 1 ? i18n.t('player.opt.normalSpeed') : `${rate}×`, <div className={s.speedGrid}>{(isAudio ? [...audiobookRates] : rates).map(v => optionRow(v === 1 ? i18n.t('player.opt.normalSpeed') : `${v}×`, v === rate, () => setSpeed(v)))}</div>)}
              {box('sleep', 'sleep', i18n.t('player.opt.sleep'), sleepLabel, sleepChoices.map(c => optionRow(sleepChoiceLabel(c), (isAudio ? listening?.timer?.choice ?? 'off' : sleepAt ? videoSleep : 'off') === c, () => setSleep(c))))}
              {queueCount > 1 && queue ? box('queue', 'queue', i18n.t('player.queue'), String(queueCount - 1), <QueueList queue={queue} />) : null}
            </div>
          </Dialog>
        ) : null}
        <Dialog open={legend} onOpenChange={setLegend} title={i18n.t('player.shortcuts')} width={420}><ShortcutLegend skipBack={skipBack} skipForward={skipForward} live={!!linear} audio={isAudio} /></Dialog>
        {/* MU4 FEAT-07: the mini guide over the video (previous/current/next, tune on select). */}
        {linear ? <MiniGuide open={miniGuide} onOpenChange={setMiniGuide} onTune={c => engine.tune(c)} /> : null}
        {/* MU4 COMPAT-01: bitmap subtitles on a 4K/HDR source confirm once per play, like Dolby Vision. */}
        <ConfirmDialog open={!!bitmapPending} onOpenChange={o => { if (!o) setBitmapPending(null); }} title={i18n.t('player.subtitles.burnWarning')} confirmLabel={i18n.t('action.continue')} cancelLabel={i18n.t('action.cancel')} destructive={false} onConfirm={confirmBitmap} />
        {info && visible ? <PlaybackInfoPanel api={api} sessionId={state.session?.id} generation={state.session?.generation} v1={state.session?.protocol === 'v1'} delivery={offersData?.current?.delivery} chosenQuality={deliveredAs ? stepLabel(deliveredAs) : undefined} source={infoSource} video={video.current} audioOnly={isAudio} onClose={() => setInfo(false)} /> : null}
        {report ? <FeedbackDialog open onOpenChange={setReport} itemId={state.itemId} playbackSessionId={state.session?.id} title={identity.title} /> : null}
        {message ? (
          <div className={s.notice} role={state.error || state.seekError ? 'alert' : 'status'}>
            {message}
            {state.seekError && state.failedSeek ? <Button size="sm" variant="outline" label={i18n.t('action.tryAgain')} onClick={() => state.failedSeek && service.seek(state.failedSeek.positionSeconds)} /> : null}
            {notice ? <IconButton name="close" label={i18n.t('action.dismiss')} variant="ghost" size="sm" onClick={() => setNotice('')} /> : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function MiniBar({engine}: {engine: PlayerEngine}) {
  const {state, identity} = engine;
  const i18n = useI18n();
  const progress = state.duration > 0 ? Math.min(1, state.positionSeconds / state.duration) : 0;
  // WEB-PLAYER-05: on a desktop-width window the bar can seek and set the volume; handhelds keep the compact bar.
  const wide = !useNarrow();
  return (
    <div className={s.mini} role="region" aria-label={i18n.t('player.nowPlayingTitle', {title: identity.title})}>
      {wide && state.duration > 0 ? (
        <input className={s.miniScrub} type="range" min={0} max={state.duration} step={1} value={Math.min(state.duration, state.positionSeconds)} aria-label={i18n.t('player.position')} aria-valuetext={i18n.t('player.clockOf', {position: formatClock(state.positionSeconds), duration: formatClock(state.duration)})} style={{'--p': `${progress * 100}%`} as React.CSSProperties} onChange={e => engine.seekTo(Number(e.target.value))} />
      ) : <div className={s.miniProgress} aria-hidden><i style={{width: `${progress * 100}%`}} /></div>}
      <div className={s.miniArt}><Artwork path={identity.posterUrl} shape="square" icon="music" alt="" /></div>
      <button type="button" className={s.miniCopy} onClick={engine.expand} aria-label={i18n.t('player.openNowPlaying')}>
        <span className={s.miniTitle}>{identity.title}</span>
        <span className={s.miniSub}>{ownerStopOf(state.ended) ? i18n.t('web.player.ownerStopped.short') : state.phase === 'error' ? i18n.t(playbackFailure(state.error).title) : identity.subtitle ?? i18n.t(state.intent === 'playing' ? 'player.miniPlaying' : 'player.miniPaused')}</span>
      </button>
      <div className={s.miniActions}>
        {wide && engine.queue ? <QueuePreference queue={engine.queue} kind="shuffle" onError={() => {}} /> : null}
        {engine.canPrevious ? <IconButton name="previous" label={i18n.t('player.previous')} variant="ghost" onClick={engine.previous} /> : null}
        <button type="button" className={s.miniPlay} aria-label={i18n.t(state.intent === 'playing' ? 'action.pause' : 'action.play')} onClick={engine.toggle}><Icon name={state.intent === 'playing' ? 'pause' : 'play'} size={18} /></button>
        {engine.canNext ? <IconButton name="next" label={i18n.t('player.next')} variant="ghost" onClick={engine.next} /> : null}
        {wide && engine.queue ? <QueuePreference queue={engine.queue} kind="repeat" onError={() => {}} /> : null}
        {engine.queue ? <MiniQueue queue={engine.queue} /> : null}
        {wide ? (
          <div className={s.volume}>
            <button type="button" className={s.control} aria-label={i18n.t(engine.muted ? 'player.unmute' : 'player.mute')} onClick={() => engine.setMuted(!engine.muted)}><Icon name={engine.muted || engine.volume === 0 ? 'mute' : 'audio'} size={18} /></button>
            <input className={cx(s.volumeRange, s.miniVolume)} type="range" min={0} max={1} step={0.05} value={engine.muted ? 0 : engine.volume} aria-label={i18n.t('player.volume')} onChange={e => { engine.setVolume(Number(e.target.value)); engine.setMuted(false); }} />
          </div>
        ) : null}
        <IconButton name="close" label={i18n.t('action.stop')} variant="ghost" onClick={engine.leave} />
      </div>
    </div>
  );
}

/** Shuffle and repeat are durable queue preferences; the workspace service owns them. */
function QueuePreference({queue, kind, onError}: {queue: NonNullable<PlayerEngine['queue']>; kind: 'shuffle' | 'repeat'; onError: (m: string) => void}) {
  const workspace = queue.workspace.service;
  const state = useSyncExternalStore(workspace.subscribe, workspace.getSnapshot);
  const q = state.queue;
  const repeat = q?.repeat ?? 'off';
  const nextRepeat = repeat === 'off' ? 'all' : repeat === 'all' ? 'one' : 'off';
  const active = kind === 'shuffle' ? !!q?.shuffled : repeat !== 'off';
  const label = kind === 'shuffle' ? 'Shuffle' : `Repeat: ${repeat === 'off' ? 'off' : repeat === 'one' ? 'one' : 'all'}`;
  return (
    <button type="button" className={cx(s.control, active && s.on)} aria-label={label} aria-pressed={active} disabled={!q || state.phase !== 'ready'} onClick={() => {
      if (!q) return;
      void workspace.mutate(kind === 'shuffle' ? {action: 'shuffle', shuffled: !q.shuffled} : {action: 'repeat', repeat: nextRepeat}).catch((e: unknown) => onError(errorText(e, 'playback', 'save')));
    }}>
      <Icon name={kind} size={20} />
      {kind === 'repeat' && repeat === 'one' ? <span style={{position: 'absolute', fontSize: 9, fontWeight: 700, marginTop: 2}}>1</span> : null}
    </button>
  );
}

/** WEB-PLAYER-02: the queue from the mini-bar, in a popover (the bar keeps its layout). */
function MiniQueue({queue}: {queue: NonNullable<PlayerEngine['queue']>}) {
  const i18n = useI18n();
  const [open, setOpen] = useState(false);
  const upNext = useSyncExternalStore(queue.workspace.service.subscribe, () => {
    const q = queue.workspace.service.getSnapshot().queue;
    if (!q) return 0;
    const live = q.entries.filter(e => !e.removed);
    return live.length - Math.max(0, live.findIndex(e => e.id === q.currentEntryId)) - 1;
  });
  if (upNext < 1) return null;
  return (
    <Popover open={open} onOpenChange={setOpen} side="top" align="end" trigger={<IconButton name="queue" label={`${i18n.t('player.queue')} (${upNext})`} variant="ghost" />}>
      <div className={s.panel} role="group" aria-label={i18n.t('player.queue')}><QueueList queue={queue} /></div>
    </Popover>
  );
}
