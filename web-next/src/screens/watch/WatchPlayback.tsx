import {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {playbackIdentity} from '@core/presentation/kit-model/index.ts';
import {itemSubject} from '../../platform/playback/subject';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../../platform/playback';
import type {PlayOnSnapshot} from '../../platform/playback/play-on';
import {CastPlayback} from '../../kit/player';
import {DockHost, DockNotice, MediaSurface, type DockBinding, type DockNoticeData, type DockPresentation} from '../../kit/dock';
import {WatchScreen} from './WatchScreen';
import {WatchSkipPrompt} from './SkipPrompt';
import {WatchUndoPrompt} from './UndoPrompt';
import {WatchUpNextPrompt} from './UpNextPrompt';
import type {WatchBindings, WatchSurfaces, WatchGeometry} from './types';
import {createWatchPlayOn, type WatchPlaybackContext, type WatchPlayOnBinding} from './play-on-binding';
import {createWindowChromePublisher, type WindowChromePublisher} from '../../platform/window-chrome';
import {router} from '../../router';
import {settingsPlatform} from '../../platform/settings-platform';
import {feedbackAvailable, subscribeFeedback, openFeedback} from '../account/feedback';
import {dispatchWatchFeedback, watchFeedbackTarget} from './feedback';
import {fenceKey, localVideoSurfaceActive, playbackPresentation, PlaybackPresentations, retainedPlaybackNotice} from '../../shell/playback-model';
import {PlaybackMediaOwner} from '../../shell/playback-media-owner';
import {useShellWidth} from '../../shell/viewport';
import styles from './playback-host.module.css';

export type WatchPlaybackProps = Readonly<{binding?: WatchBindings; context?: WatchPlaybackContext; surfaces?: WatchSurfaces; geometry?: WatchGeometry}>;
const noSubscription = () => () => {};
const noSnapshot = () => undefined;
const noPresentations: readonly DockPresentation[] = Object.freeze([]);
const subscribeFullscreen = (changed: () => void) => {document.addEventListener('fullscreenchange', changed);return () => document.removeEventListener('fullscreenchange', changed);};
const fullscreenSnapshot = () => !!document.fullscreenElement;
const noFullscreen = () => false;

/** This parent and its one video stay mounted through routes AND authority handovers. */
export function WatchPlayback({binding, context, surfaces, geometry}: WatchPlaybackProps) {
  const runtime = binding?.runtime;
  const snapshot = useSyncExternalStore<WebPlaybackRuntimeSnapshot | undefined>(runtime?.subscribe ?? noSubscription, runtime?.getSnapshot ?? noSnapshot, runtime?.getSnapshot ?? noSnapshot);
  const canReport = useSyncExternalStore(subscribeFeedback, feedbackAvailable, () => false);
  const feedbackTarget = binding && snapshot ? watchFeedbackTarget(binding, snapshot) : undefined;
  const width = useShellWidth(), phone = width < 1024;
  const host = useRef<HTMLDivElement>(null), video = useRef<HTMLVideoElement>(null), dockSlot = useRef<HTMLSpanElement>(null), expandedSlot = useRef<HTMLDivElement>(null);
  const castAnchor = useRef<HTMLElement | null>(null);
  const [castOwner, setCastOwner] = useState<{runtime: WebShellPlaybackRuntime; context: WatchPlaybackContext; value: WatchPlayOnBinding} | null>(null);
  useEffect(() => {
    // Desktop's native media owner has no browser-video Cast/AirPlay transport. Its real
    // platform is installed before the unchanged web entry, never inferred from the UA.
    if (!binding || !context || !video.current || !context.isCurrent() || settingsPlatform()?.environment.platform === 'desktop') return;
    const value = createWatchPlayOn(context, binding, video.current, import.meta.env.VITE_CAST_APPLICATION_ID);
    setCastOwner({runtime: binding.runtime, context, value});
    return value.dispose;
  }, [binding, context]);
  const cast = castOwner && castOwner.runtime === runtime && castOwner.context === context ? castOwner.value : undefined;
  const remoteState = useSyncExternalStore<PlayOnSnapshot | undefined>(cast?.controller.subscribe ?? noSubscription, cast?.controller.getSnapshot ?? noSnapshot, noSnapshot);
  const castAvailable = !!cast && (remoteState?.airPlayAvailable || remoteState?.cast.available && snapshot?.fence?.kind === 'queue');
  const remotePlaying = remoteState?.remote?.phase === 'playing';
  const media = useMemo(() => new PlaybackMediaOwner<HTMLVideoElement>(), []);
  const cache = useMemo(() => new PlaybackPresentations(runtime), [runtime]);
  const presentations = useMemo(() => snapshot && binding ? cache.update(snapshot, playbackPresentation(snapshot, binding.environment, binding.artwork, binding.skips)).map(value =>
    castAvailable && snapshot.fence && fenceKey(value.fence) === fenceKey(snapshot.fence) ? {...value, controls: [...value.controls, 'play-on' as const]} : value) : noPresentations, [snapshot, binding, cache, castAvailable]);
  const dockBinding: DockBinding | undefined = useMemo(() => runtime && binding ? {
    getSnapshot: () => runtime.dock.getSnapshot(), onAction: action => {
      if (action.kind === 'play-on' && cast && castAvailable) {
        const live = runtime.getSnapshot();
        if (!live.disposed && live.fence && fenceKey(live.fence) === fenceKey(action.fence)) {
          castAnchor.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          cast.controller.openPlayOn();
        }
      } else void runtime.onAction(action).catch(binding.reportError);
    },
  } : undefined, [runtime, binding, cast, castAvailable]);
  const expanded = !!snapshot?.dock.local?.expanded;
  const activeVideo = localVideoSurfaceActive(snapshot) && !remotePlaying;
  const fullscreen = useSyncExternalStore(subscribeFullscreen, fullscreenSnapshot, noFullscreen);
  const [pipActive, setPipActive] = useState(false);
  const pipAvailable = typeof document !== 'undefined' && document.pictureInPictureEnabled;
  useEffect(() => {
    const element = video.current;
    if (!element || !pipAvailable) return;
    const entered = () => setPipActive(document.pictureInPictureElement === element), left = () => setPipActive(false);
    element.addEventListener('enterpictureinpicture', entered);
    element.addEventListener('leavepictureinpicture', left);
    return () => {element.removeEventListener('enterpictureinpicture', entered);element.removeEventListener('leavepictureinpicture', left);};
  }, [pipAvailable, runtime]);
  const togglePip = async () => {
    const element = video.current;
    if (!element) return;
    if (document.pictureInPictureElement === element) await document.exitPictureInPicture();
    else if (document.pictureInPictureElement) throw new DOMException('Another picture source is active', 'InvalidStateError');
    else await element.requestPictureInPicture();
  };
  const [controlsVisible, setControlsVisible] = useState(true), chrome = useRef<WindowChromePublisher | undefined>(undefined);
  useLayoutEffect(() => {
    if (!expanded || !activeVideo) return;
    const publisher = createWindowChromePublisher('video');chrome.current = publisher;
    return () => {publisher.dispose();if (chrome.current === publisher) chrome.current = undefined;};
  }, [expanded, activeVideo]);
  useLayoutEffect(() => {chrome.current?.update({controlsVisible, fullscreen});}, [expanded, activeVideo, controlsVisible, fullscreen]);
  useEffect(() => {
    if ((!expanded || !activeVideo) && document.fullscreenElement === host.current) void document.exitFullscreen().catch(binding?.reportError ?? (() => {}));
  }, [expanded, activeVideo, binding]);
  const [notice, setNotice] = useState<{runtime: WebShellPlaybackRuntime; value: DockNoticeData} | null>(null), [now, setNow] = useState(() => performance.now());
  const previous = useRef<{runtime?: WebShellPlaybackRuntime; audio?: string; video?: string}>({});
  useLayoutEffect(() => { setNotice(null); previous.current = {}; }, [runtime]);
  useLayoutEffect(() => {
    const unmount = media.adopt(runtime, video.current!);
    return () => { unmount?.(); cache.clear(); };
  }, [runtime, cache, media]);
  useEffect(() => {
    if (!runtime || !snapshot) { previous.current = {}; return; }
    const parked = snapshot.dock.parkedAudio, resume = snapshot.dock.resumeVideo;
    const audioKey = parked ? fenceKey(parked.fence) : undefined, videoKey = resume ? fenceKey(resume.fence) : undefined;
    const sameRuntime = previous.current.runtime === runtime;
    const changed: Pick<DockNoticeData, 'kind' | 'playback'> | undefined = parked && (!sameRuntime || audioKey !== previous.current.audio) ? {kind: 'paused-audio', playback: parked}
      : resume && (!sameRuntime || videoKey !== previous.current.video) ? {kind: 'stopped-video', playback: resume} : undefined;
    previous.current = {runtime, audio: audioKey, video: videoKey};
    if (changed) {
      const presentation = presentations.find(value => fenceKey(value.fence) === fenceKey(changed.playback.fence));
      if (presentation) { const changedAtMs = performance.now(); setNow(changedAtMs); setNotice({runtime, value: {...changed, presentation, changedAtMs}}); }
    }
  }, [runtime, snapshot, presentations]);
  const currentNotice = retainedPlaybackNotice(notice, runtime, snapshot?.dock, presentations);
  useEffect(() => {
    if (!currentNotice) return;
    const timers = [300, 8300].map(delay => setTimeout(() => setNow(performance.now()), Math.max(0, currentNotice.changedAtMs + delay - performance.now())));
    return () => timers.forEach(clearTimeout);
  }, [currentNotice]);
  useEffect(() => {
    if (!runtime) return;
    let previous = router.state.location.href;
    return router.subscribe('onResolved', () => {
      const href = router.state.location.href, navigated = href !== previous; previous = href;
      const current = runtime.getSnapshot();
      // Expansion and posture never navigate: the underlying route's real DOM/services stay
      // alive. Only a subsequent explicit destination change collapses the local overlay.
      if (navigated && current.dock.local?.expanded && current.fence && router.state.location.pathname !== '/player') runtime.collapse(current.fence);
    });
  }, [runtime]);
  const collapse = () => {
    const live = runtime?.getSnapshot();
    if (!runtime || !snapshot?.fence || !live?.fence || live.disposed || !live.dock.local?.expanded || live.authority !== snapshot.authority || fenceKey(live.fence) !== fenceKey(snapshot.fence)) return;
    runtime.collapse(live.fence);
  };
  const toggleFullscreen = async () => {
    const live = runtime?.getSnapshot();
    if (!runtime || !snapshot?.fence || !live?.fence || live.disposed || !live.dock.local?.expanded || !localVideoSurfaceActive(live) || live.authority !== snapshot.authority || fenceKey(live.fence) !== fenceKey(snapshot.fence)) return;
    if (document.fullscreenElement) await document.exitFullscreen();
    else {
      const target = host.current;
      if (!target?.isConnected) throw new DOMException('Playback surface retired', 'AbortError');
      await target.requestFullscreen();
    }
  };
  const policy = useMemo(() => runtime && binding ? {getSnapshot: runtime.getSnapshot, isCurrent: () => !runtime.getSnapshot().disposed,
    policyAction: runtime.policyAction.bind(runtime), reportError: binding.reportError} : undefined, [runtime, binding]);
  const layout = width < 1024 ? 'phone' : globalThis.matchMedia?.('(pointer: coarse)').matches ? 'tablet' : 'desktop';
  const drawnSurfaces: WatchSurfaces = {...surfaces, available: [...new Set([...(surfaces?.available ?? []), ...(castAvailable ? ['play-on' as const] : [])])], render: surfaces?.render ?? (() => null),
    more: [...(surfaces?.more ?? []), ...(canReport && feedbackTarget ? [{id: 'report-problem', label: defaultI18n.t('web.player.error.report'),
      action: () => {dispatchWatchFeedback(feedbackTarget, {available: feedbackAvailable, open: openFeedback});}}] : [])],
    playOn: cast && castAvailable ? {active: remoteState?.panel === 'play-on', open: anchor => {castAnchor.current = anchor ?? null;cast.controller.openPlayOn();}, close: () => cast.controller.closePanel()} : surfaces?.playOn,
    prompts: <>{surfaces?.prompts}{policy && snapshot && <><WatchSkipPrompt binding={policy} snapshot={snapshot} expanded={expanded} controlsVisible={controlsVisible} layout={layout} />
      <WatchUndoPrompt binding={policy} snapshot={snapshot} expanded={expanded} controlsVisible={controlsVisible} layout={layout} />
      <WatchUpNextPrompt binding={policy} snapshot={snapshot} expanded={expanded} layout={layout} artwork={binding!.artwork} environment={binding!.environment} /></>}</>};
  const remote = remoteState?.remote;
  const visualId = remote?.phase === 'playing' ? remoteState?.cast.receiver?.itemId : remote?.itemId;
  const item = visualId ? cast?.item(visualId) : undefined;
  const source = remote?.source?.fence.kind === 'queue' && remote.source.fence.itemId === visualId ? remote.source : undefined;
  const subject = item ? itemSubject(item) : source?.subject;
  const visuals = binding ? {itemId: subject ? visualId : undefined, identity: subject ? playbackIdentity(subject, 'full', binding.environment) : undefined,
    artwork: subject ? binding.artwork.foreground(item?.posterUrl ?? item?.backdropUrl ?? source?.posterUrl ?? source?.backdropUrl, 256, 16 / 9, {group: 'playback', priority: 'high'}) : undefined,
    skipBackSeconds: binding.skips.back, skipForwardSeconds: binding.skips.forward} : undefined;
  const windowPort = typeof document !== 'undefined' ? {
    ...(document.fullscreenEnabled ? {fullscreen: {active: fullscreen, toggle: toggleFullscreen}} : {}),
    ...(pipAvailable ? {pictureInPicture: {active: pipActive, toggle: togglePip}} : {}),
  } : undefined;
  return <div ref={host} className={styles.dock} data-phone={phone} data-shell-dock data-watch-playback-host>
    {snapshot && dockBinding && <div className={styles.localDock} data-covered={!!remote}><DockHost state={snapshot.dock} presentations={presentations} binding={dockBinding} layout={phone ? 'phone' : 'desktop'} artSlotRef={dockSlot} videoSurface={!!activeVideo} />
      <DockNotice notice={currentNotice} nowMs={now} binding={dockBinding} phone={phone} /></div>}
    <MediaSurface finished={snapshot?.presentation?.finished} expanded={expanded} dockSlot={dockSlot} expandedSlot={expandedSlot} hidden={!activeVideo} nativeRenderActive={snapshot?.nativeVideoSurfaceActive} className={styles.surface}>
      <video ref={video} playsInline preload="none" aria-label={playbackIdentityLabel(presentations, snapshot?.fence && fenceKey(snapshot.fence))} />
    </MediaSurface>
    {binding && snapshot && <WatchScreen binding={binding} snapshot={snapshot} expanded={expanded && !remotePlaying} onCollapse={collapse} surfaceSlotRef={expandedSlot} onControlsVisibleChange={setControlsVisible}
      surfaces={drawnSurfaces} geometry={geometry}
      windowPort={windowPort} />}
    {cast && binding && visuals && <div className={styles.remote}><CastPlayback controller={cast.controller} visuals={visuals} phone={phone} anchorRef={castAnchor} reportError={binding.reportError} /></div>}
  </div>;
}
function playbackIdentityLabel(presentations: readonly DockPresentation[], key: string | undefined) { return presentations.find(value => fenceKey(value.fence) === key)?.identity.title ?? defaultI18n.t('next.dock.nowPlaying'); }
