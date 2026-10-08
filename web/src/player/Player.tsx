import {advancePendingChoice, takePendingTrackChoice, type PendingChoiceProgress} from './pending-choice';
import {Lyrics} from './Lyrics';
import {useCast} from '../app/cast';
import {PlayOnButton} from './PlayOn';
import {QueueList} from './QueueList';
import {useI18n} from '../app/i18n';
import {syncRate} from '../bridge/sync-rate';
import {FeedbackDialog} from '../screens/shared/FeedbackDialog';
import {trackName} from '@core/presentation/index.ts';
import React, {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {playbackUserMessage} from '@core/playback-messages.ts';
import {ownerStopOf} from './owner-stop';
import {preparedChoice, preparedReason} from '@core/prepared-media.ts';
import {listeningRates, type SleepChoice} from '@core/listening.ts';
import {subtitleReason} from '@core/subtitles.ts';
import {useWebPlaybackAdapter} from '../bridge/useWebPlaybackAdapter';
import {acceptsWebPlaybackEvent, webPlaybackResourceKey} from '../bridge/web-playback-binding';
import {useSystemListening} from '../bridge/useSystemListening';
import {formatClock} from '@core/presentation/index.ts';
import {currentAccessToken, useSession} from '../app/session';
import {Artwork, Backdrop, Button, ConfirmDialog, Dialog, Icon, IconButton, Menu, Popover, StateView, Text, cx, useCompact, useNarrow, type IconName} from '../ui';
import {isBitmapResource, textTrackForLanguage, bitmapWarningNeeded, ownerPlaybackSwitches} from '@core/presentation/index.ts';
import {formatBehindSec, isBehindLive, programmeRangeLabel} from '@core/presentation/index.ts';
import {dedupQualityRungs, convertQualityRungs, qualityNetworkFooter} from '@core/presentation/index.ts';
import {noteCastMeta} from '@core/presentation/index.ts';
import {channelTimeline} from '@core/playback/channel-timeline.ts';
import {defaultRecordingOptions} from '@core/dvr.ts';
import {currentProgramme, tunedChannelFor} from './live';
import {usePlayerDVR} from './record';
import {MiniGuide} from './MiniGuide';
import {usePlayer, type PlayerEngine} from './engine';
import {usePlayerAudioPlan, usePlayerChapters, usePlayerOffers, usePlayerSubtitles, useTrickplay} from './options';
import {SkipPrompt, type SkipMode} from './SkipPrompt';
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
  if (!player.active) return null;
  return (
    <>
      <FullPlayer engine={player} visible={player.expanded} />
      {!player.expanded && player.isAudio ? <MiniBar engine={player} /> : null}
    </>
  );
}

const rates = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2];
const noQueue = () => () => {};
const sleepChoices: {id: SleepChoice; label: string}[] = [{id: 'off', label: 'Off'}, {id: 15, label: '15 minutes'}, {id: 30, label: '30 minutes'}, {id: 45, label: '45 minutes'}, {id: 60, label: '1 hour'}, {id: 90, label: '90 minutes'}, {id: 'item', label: 'End of this item'}];

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
  const rateRef = useRef(rate);
  rateRef.current = rate;
  const item = identity.item ? {id: identity.item.id, libraryId: identity.item.libraryId} : identity.itemId && identity.libraryId ? {id: identity.itemId, libraryId: identity.libraryId} : undefined;
  const notify = useCallback((m: string) => setNotice(m), []);
  useWebPlaybackAdapter(service, api, video, rateRef, notify, resourceKey);
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
  const subtitles = usePlayerSubtitles(api, scope, service, video, resourceKey, isAudio);
  // Fetched with the session (one small read) so Chapters only appears when the title has them.
  const chapters = usePlayerChapters(api, scope, item, service, !isAudio);
  // FEAT-11: chapter starts and intro/credits on the seek bar.
  const chapterList = chapters.data?.status === 'available' ? chapters.data.chapters : undefined;
  const chapterMarks = useMemo(() => chapterList?.map(c => ({start: c.startSeconds, title: c.title})), [chapterList]);
  const markerList = offers.data?.markers?.markers;
  const segmentMarks = useMemo(() => markerList?.filter(m => m.kind === 'intro' || m.kind === 'credits' || m.kind === 'outro' || m.kind === 'recap').map(m => ({start: m.startSeconds, end: m.endSeconds})), [markerList]);
  const listening = useSyncExternalStore(queue?.listening.subscribe ?? (() => () => {}), () => queue?.listening.getSnapshot() ?? null);
  useSystemListening(service, state, video, identity.item ?? {id: identity.itemId ?? '', libraryId: identity.libraryId ?? '', title: identity.title, kind: identity.kind ?? '', duration: state.duration, progressSeconds: state.positionSeconds, available: true, posterUrl: identity.posterUrl}, queue, api, currentAccessToken);
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    el.volume = engine.volume;
    el.muted = engine.muted;
  }, [engine.volume, engine.muted, resourceKey]);
  useEffect(() => {
    if (!isAudio || !listening) return;
    setRate(listening.rate);
    if (video.current) video.current.playbackRate = listening.rate;
  }, [isAudio, listening?.rate, resourceKey]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setBuffering(false);
    setMediaPlaying(false);
    setBuffered(0);
    setOpenBox(null);
  }, [resourceKey]);
  /* A new video session starts at the profile's default speed. */
  useEffect(() => {
    if (isAudio || !state.session || state.channel) return;
    setRate(defaultSpeed);
    service.setPlaybackRate(defaultSpeed);
    if (video.current) video.current.playbackRate = defaultSpeed;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.session?.id, isAudio]);

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
  const currentResource = (el: HTMLVideoElement | null) => !service.getSnapshot().audioEffects.rendering && (!el?.dataset.porticoRoute || el.dataset.porticoRoute === 'ready') && acceptsWebPlaybackEvent(resourceKey, el, video.current, service.getSnapshot());
  const fact = (el: HTMLVideoElement | null = video.current) => {
    const now = service.getSnapshot();
    if (!now.channel && currentResource(el) && el && el.readyState >= 1 && !el.seeking && !now.pendingSeek) service.fact(now.intentId, el.currentTime, el.ended ? 'ended' : el.paused ? 'paused' : 'playing');
  };
  const updateBuffered = (el: HTMLVideoElement) => {
    try {
      const t = el.currentTime;
      for (let i = 0; i < el.buffered.length; i++) if (el.buffered.start(i) <= t && el.buffered.end(i) >= t) { setBuffered(el.buffered.end(i)); return; }
    } catch {}
  };
  /* Keyboard: the player owns shortcuts while expanded and no field has focus. */
  useEffect(() => {
    if (!visible) return;
    const key = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      const target = e.target as HTMLElement;
      if (target?.closest('input,select,textarea,[contenteditable=true],[role="menu"],[role="dialog"]')) return;
      reveal();
      switch (e.key) {
        case ' ': case 'k': e.preventDefault(); engine.toggle(); break;
        case 'ArrowLeft': case 'j': if (!target?.closest('[type=range]')) { e.preventDefault(); engine.seekBy(-skipBack); } break;
        case 'ArrowRight': case 'l': if (!target?.closest('[type=range]')) { e.preventDefault(); engine.seekBy(skipForward); } break;
        case 'ArrowUp': case 'PageUp': if (linear && canSurfChannels()) { e.preventDefault(); surf(-1); } break;
        case 'ArrowDown': case 'PageDown': if (linear && canSurfChannels()) { e.preventDefault(); surf(1); } break;
        case 'm': e.preventDefault(); engine.setMuted(!engine.muted); break;
        case 'f': e.preventDefault(); void toggleFullscreen(); break;
        case 'g': if (linear) { e.preventDefault(); setMiniGuide(true); } break;
        case '?': e.preventDefault(); setLegend(v => !v); break;
        case 'Escape': if (legend) { setLegend(false); } else if (openBox) { setOpenBox(null); } else if (document.fullscreenElement) { void document.exitFullscreen().catch(() => {}); } else if (isAudio) engine.collapse(); else close(); break;
      }
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, engine, openBox, isAudio, reveal, legend, skipBack, skipForward, state.linear]);
  async function toggleFullscreen() {
    try {
      if (document.fullscreenElement === stage.current) await document.exitFullscreen();
      else if (stage.current?.requestFullscreen) await stage.current.requestFullscreen();
    } catch {
      setNotice('Fullscreen is unavailable in this browser.');
    }
  }
  async function pip() {
    const el = video.current;
    try {
      if (document.pictureInPictureElement) await document.exitPictureInPicture();
      else if (el?.requestPictureInPicture) await el.requestPictureInPicture();
    } catch {
      setNotice('Picture in Picture could not start.');
    }
  }
  /** Watch Together from the player: the title goes to the Together page, which plays it once the group has started. */
  function watchTogether() {
    if (!state.itemId) return;
    setPendingTogether({itemId: state.itemId, title: identity.title});
    close();
    void navigate({to: '/together'});
  }
  function close() {
    fact();
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
    engine.leave();
  }
  // The error state card already says why playback stopped; the toast doesn't repeat it.
  const message = state.linearStalled ? null : engine.queueNotice || (state.error && state.phase !== 'error' ? playbackUserMessage(state.error) : state.seekError ? playbackUserMessage(state.seekError) : null) || (state.recovering ? 'Reconnecting…' : null) || notice || state.audioEffects.notice || (state.pendingSeek ? 'Seeking…' : state.phase === 'starting' ? 'Preparing playback…' : buffering && state.intent === 'playing' ? 'Buffering…' : isAudio ? listening?.message ?? undefined : undefined);
  const linear = state.linear;
  const ownerStop = ownerStopOf(state.ended);
  const ownerStopped = !!ownerStop;
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
  const currentSubtitle = subtitlePlan?.mode === 'track' && !subtitles.snapshot.suppressed ? (subtitlePlan.selected ? trackName(subtitlePlan.selected, i18n.locale) : '') || 'On' : 'Off';
  const offersData = offers.data;
  // The playing source's height, from its offers (PERF-24: the item isn't read again for it).
  const originalHeight = offersData?.sources.find(x => x.id === offersData.current?.sourceId)?.height || identity.item?.sources?.[0]?.height;
  /* MU4 COMPAT-01: 4K/HDR facts from the pre-play read, falling back to the playing source's
   * height already on the session (never a new request). */
  const cachedFacts = engine.sourceFacts(item?.id);
  const sourceFacts = {is4k: !!cachedFacts?.is4k || (originalHeight ?? 0) >= 2100, isHdr: !!cachedFacts?.isHdr};
  // ARCH-API-08: the owner's transcoding switch (from this play's offers) also turns off burned-in subtitles.
  const owner = ownerPlaybackSwitches(offersData);
  const ownerBlocks = (r: NonNullable<typeof subtitlePlan>['resources'][number]) => !owner.subtitleBurnIn && isBitmapResource(r);
  const chooseSubtitle = (resource: NonNullable<typeof subtitlePlan>['resources'][number]) => {
    const service = subtitles.service;
    if (!service || subtitles.snapshot.busy || !resource.enabled || ownerBlocks(resource)) return;
    // A language with both a text and a bitmap track plays the text track.
    if (isBitmapResource(resource)) {
      const text = textTrackForLanguage(subtitlePlan?.resources ?? [], resource.language);
      if (text && text.id !== resource.id) {
        void service.choose(text).catch(() => {});
        setNotice(i18n.t('player.subtitles.preferredText'));
        setOpenBox(null);
        return;
      }
      if (bitmapWarningNeeded(resource, sourceFacts, bitmapAcknowledged.current.has(resource.id))) {
        setBitmapPending({id: resource.id});
        return;
      }
    }
    void service.choose(resource).catch(() => {});
    setOpenBox(null);
  };
  const confirmBitmap = () => {
    const pending = bitmapPending ? subtitlePlan?.resources.find(r => r.id === bitmapPending.id) : undefined;
    setBitmapPending(null);
    if (!pending || !subtitles.service) return;
    bitmapAcknowledged.current.add(pending.id);
    void subtitles.service.choose(pending).catch(() => {});
    setOpenBox(null);
  };
  /* Pick before play: the title page's audio/subtitle choice, applied by the shared rule
   * (client-core `pending-track-choice`): audio first, then subtitles on the session left standing. */
  const pendingChoice = useRef<PendingChoiceProgress>({});
  useEffect(() => { pendingChoice.current = {choice: state.itemId ? takePendingTrackChoice(state.itemId) : undefined}; }, [state.itemId]);
  useEffect(() => {
    if (!state.session || pendingChoice.current.choice?.itemId !== state.itemId) return;
    pendingChoice.current = advancePendingChoice(pendingChoice.current, {
      sessionId: state.session.id,
      audio: {availability: audioPlan.availability, renditions: audioPlan.plan?.renditions},
      tracks: state.audioTracks,
      subtitles: subtitles.service && subtitlePlan ? {mode: subtitlePlan.mode, selectedId: subtitlePlan.selected?.id, busy: subtitles.snapshot.busy, resources: subtitlePlan.resources} : undefined,
    }, {
      selectRendition: id => { service.selectAudio(id); },
      selectTrack: stream => { void service.selectAudioTrack(stream); },
      chooseSubtitle: id => { const resource = id ? subtitlePlan?.resources.find(r => r.id === id) : null; if (resource !== undefined) void subtitles.service?.choose(resource).catch(() => {}); },
    });
  }, [state.session?.id, state.itemId, audioPlan.plan, audioPlan.availability, state.audioTracks, subtitlePlan, subtitles.service, subtitles.snapshot.busy, service]);
  const currentVersion = offersData?.current?.preparedVersionId ? offersData.preparedVersions.find(v => v.id === offersData.current?.preparedVersionId) : undefined;
  /* Ladder rungs come from the server's offers; choosing one restarts the session at the same position on that rung. */
  const [rung, setRung] = useState<string>('auto');
  useEffect(() => setRung('auto'), [item?.id]);
  const rungs = dedupQualityRungs(offersData?.sources);
  /* MU4 CON-26: one Quality model — Automatic first, then the rungs; the footer from the same
   * delivery policy as Apple ("Home network · up to 1080p" / "Away from home · up to 1080p"). */
  const convertRungs = convertQualityRungs(rungs);
  const qualityFooter = qualityNetworkFooter(offersData?.deliveryPolicy);
  const qualityFooterLine = qualityFooter.network && qualityFooter.height ? i18n.t('quality.networkLimit', {network: i18n.t(qualityFooter.network === 'home' ? 'network.home' : qualityFooter.network === 'away' ? 'network.away' : qualityFooter.network === 'wifi' ? 'network.wifi' : 'network.cellular'), height: qualityFooter.height}) : '';
  const chooseRung = (id: string) => {
    if (!item) return;
    setRung(id);
    engine.play(item.id, service.getSnapshot().pendingSeek?.positionSeconds ?? service.getSnapshot().positionSeconds, undefined, undefined, id === 'auto' ? undefined : id);
    setOpenBox(null);
  };
  const currentSource = offersData?.sources.find(x => x.id === offersData.current?.sourceId) ?? offersData?.sources[0];
  const infoSource = currentSource ? {container: currentSource.container, videoCodec: currentSource.videoCodec, audioCodec: currentSource.audioCodec, width: currentSource.width, height: currentSource.height} : undefined;
  const qualityLabelNow = offersData ? (currentVersion ? (currentVersion.facts.height ? `${currentVersion.facts.height}p` : 'Prepared') : rung !== 'auto' ? (rungs.find(q => q.id === rung)?.label ?? rung) : i18n.t('player.quality.automatic')) : '—';
  const sleepLabel = isAudio ? listening?.timer?.label ?? 'Off' : state.listeningDeadlines?.sleepAtMs ? 'On' : 'Off';
  const hidden = chromeHidden && canHide;
  const chooseVersion = (id: string) => {
    if (!offersData || !item) return;
    const version = id ? offersData.preparedVersions.find(v => v.id === id && v.selectable) : undefined;
    if (id && !version) return;
    engine.play(item.id, service.getSnapshot().pendingSeek?.positionSeconds ?? service.getSnapshot().positionSeconds, undefined, version ? preparedChoice({preparedOffersRevision: offersData.preparedOffersRevision}, version) : undefined);
    setOpenBox(null);
  };
  const setSpeed = (value: number) => {
    setRate(value);
    if (isAudio) void queue?.listening.setRate(value).catch(e => setNotice(errorText(e, 'playback', 'action')));
    else {
      service.setPlaybackRate(value);
      if (video.current) video.current.playbackRate = value;
    }
    setOpenBox(null);
  };
  const setSleep = (choice: SleepChoice) => {
    if (isAudio) void queue?.listening.setTimer(choice).catch(e => setNotice(errorText(e, 'playback', 'action')));
    else void service.setListeningDeadline('sleep', choice === 'off' ? null : typeof choice === 'number' ? choice : Math.max(1, Math.round((state.duration - state.positionSeconds) / 60))).catch(() => {});
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
  const optionRow = (label: string, checked: boolean, onSelect: () => void, meta?: string, disabled?: boolean) => (
    <button key={label + meta} type="button" role="menuitemradio" aria-checked={checked} className={s.option} onClick={onSelect} disabled={disabled}><span>{label}</span>{meta ? <span className={s.optionMeta}>{meta}</span> : checked ? <Icon name="check" size={16} /> : null}</button>
  );
  return (
    <div ref={stage} className={cx(s.stage, isAudio && s.audio, hidden && s.hidden)} hidden={!visible} aria-label={`Playing ${identity.title}`} role="region" onPointerMove={reveal} onPointerDown={reveal} tabIndex={-1}>
      {isAudio ? <Backdrop path={identity.backdropUrl ?? identity.posterUrl} height="100%" opacity={0.35} position="center" /> : null}
      <video
        key={resourceKey}
        ref={video}
        className={s.video}
        playsInline
        preload="metadata"
        onTimeUpdate={e => { fact(e.currentTarget); if ((bufferedTick.current = (bufferedTick.current + 1) % 8) === 0) updateBuffered(e.currentTarget); }}
        onProgress={e => updateBuffered(e.currentTarget)}
        onWaiting={e => { if (currentResource(e.currentTarget)) { setBuffering(true); setMediaPlaying(false); } }}
        onPlaying={e => { if (currentResource(e.currentTarget)) { setBuffering(false); setMediaPlaying(true); } }}
        onEmptied={e => { if (currentResource(e.currentTarget)) { setBuffering(false); setMediaPlaying(false); } }}
        onPause={e => { if (currentResource(e.currentTarget)) { setBuffering(false); setMediaPlaying(false); fact(e.currentTarget); } }}
        onEnded={e => fact(e.currentTarget)}
        onClick={() => (isAudio ? undefined : engine.toggle())}
      />
      {!isAudio ? <div className={s.tap} onClick={() => engine.toggle()} onDoubleClick={() => void toggleFullscreen()} aria-hidden /> : null}
      {(buffering || state.recovering) && state.intent === 'playing' && !state.error ? <div className={s.spinner} aria-hidden /> : null}
      {!isAudio ? <PostPlay engine={engine} onClose={close} /> : null}
      {!isAudio && !linear && state.phase === 'ready' ? <SkipPrompt markers={offers.data?.markers ?? null} position={state.positionSeconds} sessionId={state.session?.id} modes={skipModes} onSeek={engine.seekTo} onSkipped={(id, mode, at) => engine.service.markerSkipped(id, mode, at)} /> : null}
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
      {state.phase === 'error' && !ownerStopped ? (
        <div className={s.errorState}>
          <StateView icon="warning" title="Playback stopped" body={playbackUserMessage(state.error)} action={{label: 'Try again', onClick: engine.retry}} secondaryAction={{label: isAudio ? 'Close' : 'Back', onClick: close}} />
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
            <span className={s.subtitle}>{isAudio ? null : linear ? `On now: ${linear.programme?.title ?? 'Program information unavailable'}${linear.next ? ` · Next: ${linear.next.title}` : ''}` : identity.subtitle}</span>
            <GroupPill />
          </div>
          <div className={s.topActions}>
            {linear && !isAudio ? <LiveCaptions video={video.current} /> : null}
            {linear && canSurfChannels() ? <><IconButton name="chevronUp" label={i18n.t('web.player.channelUp')} variant="ghost" onClick={() => surf(-1)} /><IconButton name="chevronDown" label={i18n.t('web.player.channelDown')} variant="ghost" onClick={() => surf(1)} /></> : null}
            {!linear && state.itemId ? <PlayOnButton castAvailable={cast.state.available} video={video.current} onCast={() => { const itemId = state.itemId!, live = service.getSnapshot(), at = live.pendingSeek?.positionSeconds ?? live.positionSeconds; const meta = {title: identity.title, subtitle: identity.subtitle, kind: identity.kind ?? identity.item?.kind, artworkPath: identity.posterUrl ?? identity.item?.posterUrl}; noteCastMeta(itemId, meta); void cast.sender?.start(itemId, meta, at).then(ok => { if (ok) engine.leave(); }); }} /> : null}
            {!isAudio && document.pictureInPictureEnabled ? <IconButton name="pip" label="Picture in Picture" variant="ghost" onClick={() => void pip()} /> : null}
            {/* WEB-PLAYER-05: Report a problem is a flag in a More menu, not a warning sign in the bar.
                MU4 FEAT-07: linear sessions get Guide (mini guide) and Record here, never new top-bar buttons. */}
            <Menu label={i18n.t('player.more')} trigger={<IconButton name="more" label={i18n.t('player.more')} variant="ghost" />} items={[...(linear ? [{id: 'guide', label: i18n.t('player.live.guide'), icon: 'channels' as IconName}, ...(canRecordLive ? [{id: 'record', label: i18n.t('guide.record'), icon: 'dvr' as IconName}] : [])] : [...(!isAudio && !inRoom && state.itemId ? [{id: 'together', label: i18n.t('player.watchTogether'), icon: 'people' as IconName}] : []), {id: 'info', label: i18n.t('player.opt.info'), icon: 'info' as IconName}, {id: 'report', label: i18n.t('player.reportProblem'), icon: 'flag' as IconName}]), ...(!compact ? [{id: 'shortcuts', label: i18n.t('player.shortcuts'), icon: 'keyboard' as IconName}] : [])]} onSelect={id => (id === 'report' ? setReport(true) : id === 'guide' ? setMiniGuide(true) : id === 'record' ? recordLive() : id === 'info' ? setInfo(v => !v) : id === 'together' ? watchTogether() : setLegend(true))} />
            {!isAudio ? <IconButton name={document.fullscreenElement ? 'exitFullscreen' : 'fullscreen'} label="Fullscreen" variant="ghost" onClick={() => void toggleFullscreen()} /> : null}
            {isAudio ? <IconButton name="chevronDown" label="Minimize player" variant="ghost" onClick={engine.collapse} /> : null}
            <IconButton name="close" label="Close player" variant="ghost" onClick={close} />
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
              {engine.canPrevious && queueCount > 1 ? <button type="button" className={s.control} aria-label="Previous" disabled={!usable} onClick={engine.previous}><Icon name="previous" size={22} /></button> : null}
              {isMusic ? null : <button type="button" className={s.control} aria-label={`Back ${skipBack} seconds`} disabled={!usable} onClick={() => engine.seekBy(-skipBack)}><Icon name="back10" size={24} /><span className={s.seekBadge} aria-hidden>{skipBack}</span></button>}
              <button type="button" className={cx(s.control, s.play)} aria-label={hostControls ? i18n.t('web.together.pill.hostControls') : state.intent === 'playing' ? 'Pause' : 'Play'} title={hostControls ? i18n.t('web.together.pill.hostControls') : undefined} disabled={hostControls || (!state.session && state.phase !== 'starting' && state.phase !== 'error')} onClick={engine.toggle}><Icon name={state.phase === 'error' ? 'refresh' : state.intent === 'playing' ? 'pause' : 'play'} size={28} /></button>
              {isMusic ? null : <button type="button" className={s.control} aria-label={`Forward ${skipForward} seconds`} disabled={!usable} onClick={() => engine.seekBy(skipForward)}><Icon name="forward10" size={24} /><span className={s.seekBadge} aria-hidden>{skipForward}</span></button>}
              {engine.canNext ? <button type="button" className={s.control} aria-label="Next" disabled={!usable} onClick={engine.next}><Icon name="next" size={22} /></button> : null}
            </div>
            <div className={s.clusterRight}>
              {!compact && queue && isAudio ? <QueuePreference queue={queue} kind="repeat" onError={setNotice} /> : null}
              <div className={s.volume}>
                <button type="button" className={s.control} aria-label={engine.muted ? 'Unmute' : 'Mute'} onClick={() => engine.setMuted(!engine.muted)}><Icon name={engine.muted || engine.volume === 0 ? 'mute' : 'audio'} size={20} /></button>
                <input className={s.volumeRange} type="range" min={0} max={1} step={0.05} value={engine.muted ? 0 : engine.volume} aria-label="Volume" onChange={e => { engine.setVolume(Number(e.target.value)); engine.setMuted(false); }} />
              </div>
            </div>
          </div>
          {!linear && compact ? (
            <div className={s.options}>
              <button type="button" className={s.box} onClick={() => setSheet(true)} aria-label="Playback options"><Icon name="settings" size={16} />Options <strong>{[currentAudio?.label, !isAudio && currentSubtitle !== 'Off' ? `Subtitles ${currentSubtitle}` : null, rate === 1 ? null : `${rate}×`].filter(Boolean).join(' · ') || (isAudio ? 'Speed, sleep' : 'Audio, subtitles, quality')}</strong></button>
            </div>
          ) : null}
          {/* WEB-PLAYER-03: the row stays mounted while the chrome fades, so the first click lands on a box, not the video. */}
          {!linear ? (
            <div className={cx(s.options, compact && s.optionsSheet)} hidden={compact && !sheet}>
              {box('audio', 'audio', 'Audio', audioPlan.availability === 'loading' ? '…' : currentAudio?.label ?? (audioPlan.availability === 'available' ? 'Default' : 'Default'), (
                <>
                  {(!audioPlan.plan || audioPlan.availability==='unavailable') ? state.audioTracks?.tracks.map(t => optionRow(t.label,t.streamIndex===(state.audioTracks?.pending??state.audioTracks?.selected),()=>{void service.selectAudioTrack(t.streamIndex);setOpenBox(null);},undefined,state.audioTracks?.pending!==undefined)) : null}
                  {audioPlan.availability!=='unavailable' && audioPlan.plan?.renditions.map(r => optionRow(r.label, r.id === (audioPlan.pending?.renditionId ?? audioPlan.observedRenditionId), () => { service.selectAudio(r.id); setOpenBox(null); }, (r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id))?'Unavailable':undefined, !!audioPlan.pending||!!r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id)))}
                  {!audioPlan.plan?.renditions.length ? <Text variant="caption" tone="tertiary">{audioPlan.reason ?? 'This stream has one audio track.'}</Text> : null}
                  {audioPlan.failed && !audioPlan.failed.unavailable ? <Button size="sm" variant="outline" label="Try again" onClick={() => service.selectAudio(audioPlan.failed!.renditionId)} /> : null}
                  {state.session?.mode === 'hls' ? <Button size="sm" variant="ghost" icon="refresh" label="Refresh" onClick={refreshAudio} /> : null}
                </>
              ))}
              {!isAudio ? box('subtitles', 'subtitles', 'Subtitles', currentSubtitle, (
                <>
                  {subtitles.service && subtitlePlan ? (
                    <>
                      {optionRow('Off', subtitlePlan.mode === 'off', () => { void subtitles.service!.choose(null); setOpenBox(null); })}
                      {(subtitlePlan.resources ?? []).map(r => optionRow(trackName(r, i18n.locale) || r.title, !subtitles.snapshot.suppressed && subtitlePlan.selected?.id === r.id, () => chooseSubtitle(r), !r.enabled ? subtitleReason(r.reason) : ownerBlocks(r) ? i18n.t('player.ownerOff') : isBitmapResource(r) ? i18n.t('player.subtitles.bitmapNote') : r.origin === 'embedded' ? 'In file' : r.origin === 'sidecar' ? 'File' : r.origin === 'upload' ? 'Uploaded' : 'Provider', subtitles.snapshot.busy || !r.enabled || ownerBlocks(r)))}
                      {(subtitlePlan.discovered ?? []).filter(d => !(subtitlePlan.resources ?? []).some(r => r.discoveryId === d.id && r.enabled)).map(d => optionRow(trackName(d, i18n.locale) || d.title, false, () => { void subtitles.service!.import(d); setOpenBox(null); }, d.enabled ? 'Import' : subtitleReason(d.reason), subtitles.snapshot.busy || !d.enabled))}
                      {subtitles.snapshot.catalog?.provider.enabled ? (
                        subtitleSearch ? (
                          subtitles.snapshot.busy && !subtitles.snapshot.candidates.length ? <Text variant="caption" tone="tertiary">{i18n.t('player.opt.lookingForSubtitles')}</Text>
                            : subtitles.snapshot.candidates.length ? subtitles.snapshot.candidates.map(c => optionRow(`${c.title} · ${c.language}`, false, () => { void subtitles.service!.apply(c, 'personal', 'Downloaded by the viewer for personal viewing').then(() => setSubtitleSearch(false), () => {}); }, c.attribution, subtitles.snapshot.busy))
                            : <Text variant="caption" tone="tertiary">{i18n.t('player.opt.noSubtitlesFound')}</Text>
                        ) : optionRow(i18n.t('player.opt.findSubtitles'), false, () => { setSubtitleSearch(true); void subtitles.service!.search(i18n.locale.split('-')[0] ?? 'en').catch(() => {}); })
                      ) : null}
                      {subtitlePlan.selected ? <div style={{display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap'}}><Text variant="caption" tone="tertiary">Timing {(Number(subtitlePlan.offsetUs) / 1e6).toFixed(1)} s</Text><Button size="sm" variant="ghost" label="−0.5" onClick={() => void subtitles.service!.choose(subtitlePlan.selected!, String(Number(subtitlePlan.offsetUs) - 500000))} /><Button size="sm" variant="ghost" label="+0.5" onClick={() => void subtitles.service!.choose(subtitlePlan.selected!, String(Number(subtitlePlan.offsetUs) + 500000))} /></div> : null}
                    </>
                  ) : <Text variant="caption" tone="tertiary">{subtitles.snapshot.loading ? 'Loading subtitle choices…' : subtitles.snapshot.error ?? subtitles.rendererError ?? 'Subtitles are available once playback starts.'}</Text>}
                </>
              )) : null}
              {!isAudio ? box('quality', 'quality', 'Quality', qualityLabelNow, (
                <>
                  {/* WEB-PLAYER-03: consumer words; the delivery facts live under Playback info. */}
                  {optionRow(i18n.t('player.quality.automatic'), !currentVersion && rung === 'auto', () => chooseVersion(''), offersData?.current?.delivery === 'hls' ? i18n.t('player.quality.automaticHelp') : i18n.t('player.quality.originalHelp'))}
                  {offersData?.preparedVersions.map(v => optionRow(v.facts.height ? `${v.facts.height}p${v.facts.height >= 2100 ? ' · 4K' : ''}` : i18n.t('player.quality.prepared'), currentVersion?.id === v.id, () => chooseVersion(v.id), v.selectable ? undefined : preparedReason(v.reason), !v.selectable))}
                  {convertRungs.length ? <Text variant="label" tone="tertiary" style={{padding: '8px 12px 2px'}}>{i18n.t('player.quality.convert')}</Text> : null}
                  {convertRungs.map(q => optionRow(q.targetDisplayHeight ? `${q.targetDisplayHeight}p` : q.label, rung === q.id && !currentVersion, () => chooseRung(q.id), !owner.transcoding ? i18n.t('player.ownerOff') : q.enabled ? undefined : q.reason ?? 'Not available', !q.enabled || !owner.transcoding))}
                  {offersData?.current ? <Text variant="caption" tone="tertiary">{[qualityFooterLine, `${offersData.current.delivery === 'hls' ? (rung !== 'auto' ? i18n.t('player.quality.converting', {quality: rungs.find(q => q.id === rung)?.label ?? rung}) : i18n.t('player.quality.convertingAuto')) : i18n.t('player.quality.playingOriginal')}${offersData.transcodingEnabled ? '' : ' ' + i18n.t('player.quality.conversionOff')}`].filter(Boolean).join(' · ')}</Text> : null}
                  {state.audioEffects.rendering&&state.session?.audio?.downmixed?<Text variant="caption" tone="secondary">{i18n.t('player.opt.downmixed')}</Text>:null}
                </>
              )) : null}
              {isMusic ? null : box('speed', 'speed', 'Speed', rate === 1 ? 'Normal' : `${rate}×`, <div className={s.speedGrid}>{(isAudio ? [...listeningRates] : rates).map(v => optionRow(v === 1 ? 'Normal' : `${v}×`, v === rate, () => setSpeed(v)))}</div>)}
              {box('sleep', 'sleep', 'Sleep', sleepLabel, (
                <>
                  {sleepChoices.map(c => optionRow(c.label, isAudio ? (listening?.timer?.choice ?? 'off') === c.id : c.id === 'off' ? !state.listeningDeadlines?.sleepAtMs : false, () => setSleep(c.id)))}
                </>
              ))}
              {queueCount > 1 && queue ? box('queue', 'queue', i18n.t('player.queue'), String(queueCount - 1), <QueueList queue={queue} />) : null}
              {!isAudio && chapters.data?.status === 'available' && chapters.data.chapters.length ? box('chapters', 'chapters', 'Chapters', String(chapters.data.chapters.length), (
                <div className={s.chapters}>
                  {chapters.data?.status === 'available' ? chapters.data.chapters.map(ch => (
                    <button key={ch.id} type="button" role="menuitemradio" aria-checked={state.positionSeconds >= ch.startSeconds && state.positionSeconds < ch.endSeconds} className={s.option} onClick={() => { service.seek(ch.action.positionSeconds); setOpenBox(null); }}>
                      <span className={s.chapterRow}>{ch.thumbnailUrl ? <span className={s.chapterThumb}><Artwork path={ch.thumbnailUrl} shape="landscape" alt="" /></span> : null}<span>{ch.index}. {ch.title}</span></span>
                      <span className={s.optionMeta}>{formatClock(ch.startSeconds)}</span>
                    </button>
                  )) : <Text variant="caption" tone="tertiary">{chapters.state.phase === 'loading' ? 'Loading chapters…' : chapters.data?.status === 'none' ? 'This title has no chapters.' : 'Chapters are unavailable for this stream.'}</Text>}
                </div>
              )) : null}
            </div>
          ) : null}
        </div>
        {compact ? (
          <Dialog open={sheet} onOpenChange={setSheet} title="Playback options" description={identity.title}>
            <div className={s.sheetList}>
              {box('audio', 'audio', 'Audio', currentAudio?.label ?? 'Default', (
                <>
                  {(!audioPlan.plan || audioPlan.availability==='unavailable') ? state.audioTracks?.tracks.map(t => optionRow(t.label,t.streamIndex===(state.audioTracks?.pending??state.audioTracks?.selected),()=>{void service.selectAudioTrack(t.streamIndex);setOpenBox(null);},undefined,state.audioTracks?.pending!==undefined)) : null}
                  {audioPlan.availability!=='unavailable' && audioPlan.plan?.renditions.map(r => optionRow(r.label, r.id === (audioPlan.pending?.renditionId ?? audioPlan.observedRenditionId), () => { service.selectAudio(r.id); setOpenBox(null); }, (r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id))?'Unavailable':undefined, !!audioPlan.pending||!!r.unavailable||audioPlan.unavailableRenditionIds?.includes(r.id)))}
                  {!audioPlan.plan?.renditions.length ? <Text variant="caption" tone="tertiary">{audioPlan.reason ?? 'This stream has one audio track.'}</Text> : null}
                </>
              ))}
              {!isAudio ? box('subtitles', 'subtitles', 'Subtitles', currentSubtitle, subtitles.service && subtitlePlan ? (
                <>
                  {optionRow('Off', subtitlePlan.mode === 'off', () => { void subtitles.service!.choose(null); })}
                  {(subtitlePlan.resources ?? []).map(r => optionRow(trackName(r, i18n.locale) || r.title, !subtitles.snapshot.suppressed && subtitlePlan.selected?.id === r.id, () => chooseSubtitle(r), !r.enabled ? subtitleReason(r.reason) : ownerBlocks(r) ? i18n.t('player.ownerOff') : isBitmapResource(r) ? i18n.t('player.subtitles.bitmapNote') : undefined, subtitles.snapshot.busy || !r.enabled || ownerBlocks(r)))}
                </>
              ) : <Text variant="caption" tone="tertiary">Subtitles are available once playback starts.</Text>) : null}
              {!isAudio ? box('quality', 'quality', 'Quality', qualityLabelNow, (
                <>
                  {optionRow(i18n.t('player.quality.automatic'), !currentVersion, () => chooseVersion(''))}
                  {offersData?.preparedVersions.map(v => optionRow(v.facts.height ? `${v.facts.height}p` : i18n.t('player.quality.prepared'), currentVersion?.id === v.id, () => chooseVersion(v.id), v.selectable ? undefined : preparedReason(v.reason), !v.selectable))}
                  {state.audioEffects.rendering&&state.session?.audio?.downmixed?<Text variant="caption" tone="secondary">{i18n.t('player.opt.downmixed')}</Text>:null}
                </>
              )) : null}
              {isMusic ? null : box('speed', 'speed', 'Speed', rate === 1 ? 'Normal' : `${rate}×`, <div className={s.speedGrid}>{(isAudio ? [...listeningRates] : rates).map(v => optionRow(v === 1 ? 'Normal' : `${v}×`, v === rate, () => setSpeed(v)))}</div>)}
              {box('sleep', 'sleep', 'Sleep', sleepLabel, sleepChoices.map(c => optionRow(c.label, isAudio ? (listening?.timer?.choice ?? 'off') === c.id : c.id === 'off' ? !state.listeningDeadlines?.sleepAtMs : false, () => setSleep(c.id))))}
              {queueCount > 1 && queue ? box('queue', 'queue', i18n.t('player.queue'), String(queueCount - 1), <QueueList queue={queue} />) : null}
            </div>
          </Dialog>
        ) : null}
        <Dialog open={legend} onOpenChange={setLegend} title="Keyboard shortcuts" width={420}><ShortcutLegend skipBack={skipBack} skipForward={skipForward} /></Dialog>
        {/* MU4 FEAT-07: the mini guide over the video (previous/current/next, tune on select). */}
        {linear ? <MiniGuide open={miniGuide} onOpenChange={setMiniGuide} onTune={c => engine.tune(c)} /> : null}
        {/* MU4 COMPAT-01: bitmap subtitles on a 4K/HDR source confirm once per play, like Dolby Vision. */}
        <ConfirmDialog open={!!bitmapPending} onOpenChange={o => { if (!o) setBitmapPending(null); }} title={i18n.t('player.subtitles.burnWarning')} confirmLabel={i18n.t('action.continue')} cancelLabel={i18n.t('action.cancel')} destructive={false} onConfirm={confirmBitmap} />
        {info && visible ? <PlaybackInfoPanel api={api} sessionId={state.session?.id} v1={state.session?.protocol === 'v1'} delivery={offersData?.current?.delivery} chosenQuality={rung !== 'auto' && !currentVersion ? rungs.find(q => q.id === rung)?.label ?? rung : undefined} source={infoSource} video={video.current} audioOnly={isAudio} onClose={() => setInfo(false)} /> : null}
        {report ? <FeedbackDialog open onOpenChange={setReport} itemId={state.itemId} playbackSessionId={state.session?.id} title={identity.title} /> : null}
        {message ? (
          <div className={s.notice} role={state.error || state.seekError ? 'alert' : 'status'}>
            {message}
            {state.seekError && state.failedSeek ? <Button size="sm" variant="outline" label="Try again" onClick={() => state.failedSeek && service.seek(state.failedSeek.positionSeconds)} /> : null}
            {notice ? <IconButton name="close" label="Dismiss" variant="ghost" size="sm" onClick={() => setNotice('')} /> : null}
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
    <div className={s.mini} role="region" aria-label={`Now playing: ${identity.title}`}>
      {wide && state.duration > 0 ? (
        <input className={s.miniScrub} type="range" min={0} max={state.duration} step={1} value={Math.min(state.duration, state.positionSeconds)} aria-label="Position" aria-valuetext={`${formatClock(state.positionSeconds)} of ${formatClock(state.duration)}`} style={{'--p': `${progress * 100}%`} as React.CSSProperties} onChange={e => engine.seekTo(Number(e.target.value))} />
      ) : <div className={s.miniProgress} aria-hidden><i style={{width: `${progress * 100}%`}} /></div>}
      <div className={s.miniArt}><Artwork path={identity.posterUrl} shape="square" icon="music" alt="" /></div>
      <button type="button" className={s.miniCopy} onClick={engine.expand} aria-label="Open now playing">
        <span className={s.miniTitle}>{identity.title}</span>
        <span className={s.miniSub}>{ownerStopOf(state.ended) ? i18n.t('web.player.ownerStopped.short') : state.phase === 'error' ? 'Playback stopped' : identity.subtitle ?? (state.intent === 'playing' ? 'Playing' : 'Paused')}</span>
      </button>
      <div className={s.miniActions}>
        {wide && engine.queue ? <QueuePreference queue={engine.queue} kind="shuffle" onError={() => {}} /> : null}
        {engine.canPrevious ? <IconButton name="previous" label="Previous" variant="ghost" onClick={engine.previous} /> : null}
        <button type="button" className={s.miniPlay} aria-label={state.intent === 'playing' ? 'Pause' : 'Play'} onClick={engine.toggle}><Icon name={state.intent === 'playing' ? 'pause' : 'play'} size={18} /></button>
        {engine.canNext ? <IconButton name="next" label="Next" variant="ghost" onClick={engine.next} /> : null}
        {wide && engine.queue ? <QueuePreference queue={engine.queue} kind="repeat" onError={() => {}} /> : null}
        {engine.queue ? <MiniQueue queue={engine.queue} /> : null}
        {wide ? (
          <div className={s.volume}>
            <button type="button" className={s.control} aria-label={engine.muted ? 'Unmute' : 'Mute'} onClick={() => engine.setMuted(!engine.muted)}><Icon name={engine.muted || engine.volume === 0 ? 'mute' : 'audio'} size={18} /></button>
            <input className={cx(s.volumeRange, s.miniVolume)} type="range" min={0} max={1} step={0.05} value={engine.muted ? 0 : engine.volume} aria-label="Volume" onChange={e => { engine.setVolume(Number(e.target.value)); engine.setMuted(false); }} />
          </div>
        ) : null}
        <IconButton name="close" label="Stop" variant="ghost" onClick={engine.leave} />
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
