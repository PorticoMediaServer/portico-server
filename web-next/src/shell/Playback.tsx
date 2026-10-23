import {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {TextEnvironment} from '@core/presentation/kit-model/index.ts';
import type {WebPlaybackRuntimeSnapshot, WebShellPlaybackRuntime} from '../platform/playback';
import {DockHost, DockNotice, MediaSurface, type DockBinding, type DockNoticeData, type DockPresentation} from '../kit/dock';
import {WatchScreen} from '../screens/watch';
import {createWindowChromePublisher, type WindowChromePublisher} from '../platform/window-chrome';
import {router} from '../router';
import type {ShellArtwork} from './artwork';
import {fenceKey, localVideoSurfaceActive, playbackPresentation, PlaybackPresentations, retainedPlaybackNotice} from './playback-model';
import {PlaybackMediaOwner} from './playback-media-owner';
import {useShellWidth} from './viewport';
import styles from './playback.module.css';

export type ShellPlaybackBinding = Readonly<{
  runtime: WebShellPlaybackRuntime; artwork: ShellArtwork; environment: TextEnvironment;
  skips: Readonly<{back: number; forward: number}>; reportError: (error: unknown) => void;
}>;
const noSubscription = () => () => {};
const noSnapshot = () => undefined;
const noPresentations: readonly DockPresentation[] = Object.freeze([]);
const subscribeFullscreen = (changed: () => void) => {document.addEventListener('fullscreenchange', changed);return () => document.removeEventListener('fullscreenchange', changed);};
const fullscreenSnapshot = () => !!document.fullscreenElement;
const noFullscreen = () => false;

/** This parent and its one video stay mounted through routes AND authority handovers. */
export function PlaybackHost({binding}: {binding: ShellPlaybackBinding | undefined}) {
  const runtime = binding?.runtime;
  const snapshot = useSyncExternalStore<WebPlaybackRuntimeSnapshot | undefined>(runtime?.subscribe ?? noSubscription, runtime?.getSnapshot ?? noSnapshot, runtime?.getSnapshot ?? noSnapshot);
  const width = useShellWidth(), phone = width < 1024;
  const video = useRef<HTMLVideoElement>(null), dockSlot = useRef<HTMLSpanElement>(null), expandedSlot = useRef<HTMLDivElement>(null);
  const media = useMemo(() => new PlaybackMediaOwner<HTMLVideoElement>(), []);
  const cache = useMemo(() => new PlaybackPresentations(runtime), [runtime]);
  const presentations = useMemo(() => snapshot && binding ? cache.update(snapshot, playbackPresentation(snapshot, binding.environment, binding.artwork, binding.skips)) : noPresentations, [snapshot, binding, cache]);
  const dockBinding: DockBinding | undefined = useMemo(() => runtime && binding ? {
    getSnapshot: () => runtime.dock.getSnapshot(), onAction: action => { void runtime.onAction(action).catch(binding.reportError); },
  } : undefined, [runtime, binding]);
  const expanded = !!snapshot?.dock.local?.expanded;
  const activeVideo = localVideoSurfaceActive(snapshot);
  const fullscreen = useSyncExternalStore(subscribeFullscreen, fullscreenSnapshot, noFullscreen);
  const [controlsVisible, setControlsVisible] = useState(true), chrome = useRef<WindowChromePublisher | undefined>(undefined);
  useLayoutEffect(() => {
    if (!expanded || !activeVideo) return;
    const publisher = createWindowChromePublisher('video');chrome.current = publisher;
    return () => {publisher.dispose();if (chrome.current === publisher) chrome.current = undefined;};
  }, [expanded, activeVideo]);
  useLayoutEffect(() => {chrome.current?.update({controlsVisible, fullscreen});}, [expanded, activeVideo, controlsVisible, fullscreen]);
  const [notice, setNotice] = useState<{runtime: WebShellPlaybackRuntime; value: DockNoticeData} | null>(null), [now, setNow] = useState(() => performance.now());
  const previous = useRef<{runtime?: WebShellPlaybackRuntime; audio?: string; video?: string}>({});
  const leaving = useRef<string | undefined>(undefined);
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
    if (!binding) { leaving.current = undefined; return; }
    if (expanded && router.state.location.pathname !== '/player') {
      leaving.current = router.state.location.href;
      void router.navigate({to: '/player'}).catch(binding.reportError);
    } else if (!expanded && router.state.location.pathname === '/player' && leaving.current) {
      const href = leaving.current; leaving.current = undefined;
      void router.navigate({href, replace: true}).catch(binding.reportError);
    }
  }, [expanded, binding]);
  useEffect(() => {
    if (!runtime) return;
    return router.subscribe('onResolved', () => {
      const current = runtime.getSnapshot();
      if (current.dock.local?.expanded && current.fence && router.state.location.pathname !== '/player') runtime.collapse(current.fence);
    });
  }, [runtime]);
  const collapse = () => { if (snapshot?.fence) runtime!.collapse(snapshot.fence); };
  const toggleFullscreen = async () => {
    const live = runtime?.getSnapshot();
    if (!runtime || !snapshot?.fence || !live?.fence || live.disposed || live.authority !== snapshot.authority || fenceKey(live.fence) !== fenceKey(snapshot.fence)) return;
    if (document.fullscreenElement) await document.exitFullscreen();
    else await expandedSlot.current?.closest<HTMLElement>('[data-watch-screen]')?.requestFullscreen();
  };
  return <div className={styles.dock} data-phone={phone} data-shell-dock>
    {snapshot && dockBinding && <><DockHost state={snapshot.dock} presentations={presentations} binding={dockBinding} layout={phone ? 'phone' : 'desktop'} artSlotRef={dockSlot} videoSurface={!!activeVideo} />
      <DockNotice notice={currentNotice} nowMs={now} binding={dockBinding} phone={phone} /></>}
    {binding && snapshot && <WatchScreen binding={binding} snapshot={snapshot} expanded={expanded} onCollapse={collapse} surfaceSlotRef={expandedSlot} onControlsVisibleChange={setControlsVisible}
      windowPort={typeof document !== 'undefined' && document.fullscreenEnabled ? {fullscreen: {active: fullscreen, toggle: toggleFullscreen}} : undefined} />}
    <MediaSurface finished={snapshot?.presentation?.finished} expanded={expanded} dockSlot={dockSlot} expandedSlot={expandedSlot} hidden={!activeVideo} className={styles.surface}>
      <video ref={video} playsInline preload="none" aria-label={playbackIdentityLabel(presentations, snapshot?.fence && fenceKey(snapshot.fence))} />
    </MediaSurface>
  </div>;
}
function playbackIdentityLabel(presentations: readonly DockPresentation[], key: string | undefined) { return presentations.find(value => fenceKey(value.fence) === key)?.identity.title ?? defaultI18n.t('next.dock.nowPlaying'); }
