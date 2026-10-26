import {Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type CSSProperties, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {playbackIdentity, type FactPart} from '@core/presentation/kit-model/index.ts';
import {sameFence, type PlaybackFence} from '../../../../packages/player-core/src/index.ts';
import {Icon, type IconName} from '../../kit/Icon';
import {Artwork} from '../../kit/artwork';
import {Button, Slider} from '../../kit/controls';
import {Menu, type MenuEntry} from '../../kit/overlays';
import {KeyboardShortcuts} from '../../kit/states';
import {LossNotice} from '../../kit/media';
import {dispatchWatch, watchClock, watchIsPortrait, watchSeek, watchSeekBounds, watchShortcutTarget, watchStepIntent, type WatchIntent} from './model';
import {settledPlaybackChanged, settledPlaybackKey, type SettledPlayback} from './screen-state';
import type {WatchPanel, WatchRect, WatchScreenProps} from './types';
import styles from './watch.module.css';
import {PlaybackWaiting} from '../../kit/states/PlaybackWaiting';
import {PlaybackFailure} from '../../kit/states/PlaybackFailure';
import {ScrubPreview} from './ScrubPreview';
import {watchPreviewFrame} from './preview';
import {useCoarsePointer} from '../../kit/theme';
import {WatchPictureInput} from './picture';
import {watchQueueNeighbours} from './queue-neighbours';
import {WatchDockPanelOpening, type WatchDockPanel} from './dock-panel-opening';
import {WatchEndCardView} from './EndCard';
import {LiveChrome} from '../live/LiveChrome';
import type {LiveChromePort} from '../live/live-chrome-types';
import {WatchPromptFocus, type PromptFocusHost} from './prompt-focus';

const t = defaultI18n.t;
const subscribeWindow = (changed: () => void) => { window.addEventListener('resize', changed); return () => window.removeEventListener('resize', changed); };
const windowSize = () => `${window.innerWidth}:${window.innerHeight}`;
const serverSize = () => '1440:810';
const rectStyle = (rect: WatchRect | undefined): CSSProperties | undefined => rect ? {position: 'absolute', left: rect.left, top: rect.top, width: rect.width, height: rect.height, right: 'auto', bottom: 'auto'} : undefined;

function Facts({parts}: {parts: readonly FactPart[]}) {
  return parts.map((part, index) => <Fragment key={part.id}>{index > 0 && <span className={styles.gap} aria-hidden="true" />}<span data-fact={part.id}>{part.text}</span></Fragment>);
}
function Control({icon, label, action, disabled, main, skip, large, buttonRef, active}: {
  icon: IconName; label: string; action: (button: HTMLButtonElement) => void; disabled?: boolean; main?: boolean; skip?: number; large?: boolean;
  buttonRef?: RefObject<HTMLButtonElement | null>; active?: boolean;
}) {
  return <button ref={buttonRef} type="button" className={styles.control} data-main={main || undefined} data-large={large || undefined}
    aria-label={label} title={label} disabled={disabled} aria-pressed={active} onClick={event => action(event.currentTarget)}>
    <Icon name={icon} size={large ? 24 : skip !== undefined || main ? 28 : 22} />
    {skip !== undefined && <small aria-hidden="true">{skip}</small>}
  </button>;
}

/** Only drawn controls and geometry targets. The parent's MediaSurface remains its sole video. */
export function WatchScreen({binding, snapshot, expanded, onCollapse, onControlsVisibleChange, publishDockPanels, dockPanelsCurrent, subscribeDockPanels, surfaceSlotRef, surfaces, live, windowPort, geometry}: WatchScreenProps) {
  const size = useSyncExternalStore(subscribeWindow, windowSize, serverSize).split(':').map(Number), width = size[0]!, height = size[1]!;
  const coarsePointer = useCoarsePointer(), touchTablet = coarsePointer && width >= 600;
  const root = useRef<HTMLDivElement>(null), chrome = useRef<HTMLDivElement>(null), body = useRef<HTMLDivElement>(null), panelAnchor = useRef<HTMLButtonElement>(null), moreAnchor = useRef<HTMLButtonElement>(null);
  const promptSeek = useRef<HTMLInputElement | null>(null);
  const bodyScroll = useRef<{identity?: string; top: number; left: number; anchor?: HTMLElement; offset?: number}>({top: 0, left: 0});
  const [panel, setPanel] = useState<WatchPanel | null>(null), [more, setMore] = useState(false), [shortcuts, setShortcuts] = useState(false), [visible, setVisible] = useState(true), [fullWindow, setFullWindow] = useState(false), [liveGuideOpen, setLiveGuideOpen] = useState(false);
  const [controlsExit, setControlsExit] = useState<'idle' | 'tap'>('idle');
  type Scrub = {snapshot: WatchScreenProps['snapshot']; fence: PlaybackFence; seconds: number; pointerId: number};
  const [scrub, setScrub] = useState<Scrub | null>(null), [hoverSeconds, setHoverSeconds] = useState<number | null>(null);
  const scrubDrag = useRef<Scrub | null>(null);
  const [pictureInput] = useState(() => new WatchPictureInput());
  const pointerKind = useRef('mouse');
  const [tapFeedback, setTapFeedback] = useState<{direction: 'left' | 'right'; seconds: number; x: number; y: number; sequence: number} | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const keyboardFocus = useRef(false);
  const controlsVisible = useRef(visible);
  controlsVisible.current = visible;
  const lastLayout = useRef<string | null>(null);
  const current = useRef({binding, snapshot, expanded});
  current.current = {binding, snapshot, expanded};
  const presentation = snapshot.presentation, fence = snapshot.fence;
  const playbackKey = settledPlaybackKey(fence?.playback);
  const settledPlayback = useRef<SettledPlayback | undefined>(undefined);
  const sourceKey = fence ? JSON.stringify(fence) : '';
  const valid = !snapshot.disposed && !!presentation && !!fence && sameFence(fence, presentation.fence)
    && snapshot.authority === binding.runtime.getSnapshot().authority && sameFence(binding.runtime.getSnapshot().fence, fence);
  const liveChrome = live && fence && sameFence(live.fence, fence) && snapshot.authority === binding.runtime.getSnapshot().authority && live.isCurrent() ? live : undefined;
  const shapePortrait = watchIsPortrait(width, height);
  const mode = geometry?.mode ?? (!fullWindow && shapePortrait ? 'portrait' : 'full');
  const portrait = mode === 'portrait' || mode === 'book', permanent = mode === 'tabletop';
  const mobile = width < 1024;
  const bodyVisible = mode !== 'full';
  const externalPanel = surfaces?.playOn?.active === true;
  // Playback info f1: the film keeps playing with no player controls under the panel.
  const infoHidesChrome = panel === 'info' && !mobile;
  const chromeVisible = (visible || permanent || liveGuideOpen) && !infoHidesChrome;
  const open = expanded && valid && presentation.kind === 'video' && snapshot.dock.remote?.phase !== 'playing';
  useLayoutEffect(() => {
    const element = body.current;
    if (!element) return;
    if (bodyScroll.current.identity !== surfaces?.bodyIdentity) bodyScroll.current = {identity: surfaces?.bodyIdentity, top: 0, left: 0};
    if (!bodyVisible || !open) return;
    const saved = bodyScroll.current, anchor = saved.anchor;
    element.scrollTop = anchor?.isConnected && element.contains(anchor) && saved.offset !== undefined
      ? element.scrollTop + anchor.getBoundingClientRect().top - element.getBoundingClientRect().top - saved.offset : saved.top;
    element.scrollLeft = saved.left;
  }, [bodyVisible, mode, open, width, height, geometry?.revision, surfaces?.bodyIdentity]);
  const identity = presentation ? playbackIdentity(presentation.subject, 'full', binding.environment) : undefined;
  const can = (kind: string) => !!presentation?.controls.includes(kind as never);
  const send = useCallback((intent: WatchIntent) => {
    const value = current.current;
    if (!value.expanded) return;
    void dispatchWatch(value.binding.runtime, value.snapshot, intent).catch(value.binding.reportError);
  }, []);
  const effect = (action: () => void | Promise<void>) => {
    const value = current.current, live = value.binding.runtime.getSnapshot();
    if (!value.expanded || live.disposed || live.dock.remote?.phase === 'playing' || !value.snapshot.fence || !sameFence(live.fence, value.snapshot.fence) || live.authority !== value.snapshot.authority) return;
    try { void Promise.resolve(action()).catch(value.binding.reportError); } catch (error) { value.binding.reportError(error); }
  };
  const clearTimer = useCallback(() => { if (timer.current !== undefined) clearTimeout(timer.current); timer.current = undefined; }, []);
  const revealFor = useCallback((milliseconds: number) => {
    clearTimer(); setVisible(true);
    if (!open || permanent || panel || externalPanel || more || shortcuts || liveGuideOpen || scrubDrag.current || !presentation?.playing) return;
    timer.current = setTimeout(() => {
      if (chrome.current?.contains(document.activeElement)) {
        if (keyboardFocus.current) return;
        if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
      }
      setControlsExit('idle'); setVisible(false);
    }, milliseconds);
  }, [clearTimer, open, permanent, panel, externalPanel, more, shortcuts, liveGuideOpen, presentation?.playing]);
  const reveal = useCallback(() => revealFor(coarsePointer ? 4000 : 3000), [revealFor, coarsePointer]);
  useEffect(() => {
    const layout = `${width}:${height}:${geometry?.revision ?? ''}`, changed = lastLayout.current !== null && lastLayout.current !== layout;
    lastLayout.current = layout;
    // Q7 window/posture changes reveal for 3s; ordinary touch activity uses 4s.
    revealFor(changed ? 3000 : coarsePointer ? 4000 : 3000);
    return clearTimer;
  }, [revealFor, clearTimer, coarsePointer, width, height, geometry?.revision, sourceKey]);
  useEffect(() => { onControlsVisibleChange?.(open && chromeVisible); }, [onControlsVisibleChange, open, chromeVisible]);
  useLayoutEffect(() => {
    // A fence replacement blanks playback for a frame; only a settled new runtime or a new
    // playback run starts screen state over (a quality change in the same run keeps it).
    if (!settledPlaybackChanged(settledPlayback.current, binding.runtime, fence?.playback)) return;
    settledPlayback.current = {runtime: binding.runtime, key: playbackKey};
    setPanel(null); setMore(false); setShortcuts(false); setFullWindow(false); setVisible(true); setLiveGuideOpen(false);
  }, [binding.runtime, playbackKey, fence?.playback]);
  useEffect(() => {if (!open) setLiveGuideOpen(false);}, [open]);
  const focusEdge = useRef(false), expandedNow = useRef(expanded); expandedNow.current = expanded;
  useLayoutEffect(() => {
    const wasOpen = focusEdge.current; focusEdge.current = open;
    // Only on the closed-to-open edge: a later autoplay entry change must not take
    // focus, and an open surface above the player keeps it.
    if (!open || wasOpen) return;
    const element = root.current, active = document.activeElement;
    if (!element || (active instanceof Node && element.contains(active))) return;
    if (active instanceof Element && active.closest('[role="dialog"],[role="menu"],[aria-modal="true"]')) return;
    element.focus({preventScroll: true});
  }, [open]);
  useLayoutEffect(() => { pictureInput.cancel(); setTapFeedback(null); scrubDrag.current = null; setScrub(null); setHoverSeconds(null); }, [binding.runtime, snapshot.authority, sourceKey, pictureInput]);
  useEffect(() => () => pictureInput.cancel(), [pictureInput]);
  useLayoutEffect(() => { setFullWindow(false); }, [shapePortrait]);
  // A source change may blink expanded/open for a frame while the new stack starts.
  // Only a settled collapse clears the transient player chrome; the options panel is
  // owned by the playback identity effect above and survives a collapse blink.
  useEffect(() => {
    if (expanded) return;
    const timer = setTimeout(() => {
      if (expandedNow.current) return;
      pictureInput.cancel(); setTapFeedback(null); clearTimer(); setMore(false);
      setShortcuts(false); scrubDrag.current = null; setScrub(null); setHoverSeconds(null);
    }, 250);
    return () => clearTimeout(timer);
  }, [expanded, clearTimer, pictureInput]);
  useEffect(() => { if (panel || more || shortcuts) pictureInput.cancel(); }, [panel, more, shortcuts, pictureInput]);
  const showPanel = (next: WatchPanel, anchor?: HTMLButtonElement) => {
    if (!surfaces?.available.includes(next)) return;
    if (next === 'play-on' && surfaces.playOn) {setMore(false);surfaces.playOn.open(anchor);reveal();return;}
    if (anchor) panelAnchor.current = anchor;
    setMore(false); setPanel(next); reveal();
  };
  const dockOwnerRef = useRef<WatchDockPanelOpening | undefined>(undefined);
  const [dockOwner, setDockOwner] = useState<WatchDockPanelOpening>();
  const [dockOpening, setDockOpening] = useState(0);
  const dockPanels = (surfaces?.available ?? []).filter((panel): panel is WatchDockPanel => panel === 'options' || panel === 'subtitles-audio');
  const dockPanelKey = dockPanels.join('|');
  const dockAdmitted = !!dockPanelsCurrent && dockPanelsCurrent() && !surfaces?.modal && !mobile && valid;
  useLayoutEffect(() => {
    if (!publishDockPanels || !dockPanelsCurrent || !dockAdmitted || !fence || presentation?.kind !== 'video' || !dockPanelKey) return;
    const owner = new WatchDockPanelOpening({binding, authority: snapshot.authority, fence, panels: dockPanels,
      snapshot: binding.runtime.getSnapshot, isCurrent: () => dockOwnerRef.current === owner && dockPanelsCurrent(),
      expand: () => binding.runtime.open(fence), show: next => showPanel(next),
      renew: () => setDockOpening(value => value + 1), reportError: binding.reportError});
    dockOwnerRef.current = owner;
    const off = binding.runtime.subscribe(owner.observe);
    const offSender = subscribeDockPanels?.(owner.observe) ?? (() => {});
    setDockOwner(owner);
    return () => {
      owner.dispose(); off(); offSender();
      if (dockOwnerRef.current === owner) dockOwnerRef.current = undefined;
      setDockOwner(value => value === owner ? undefined : value);
    };
  }, [binding, snapshot.authority, sourceKey, dockAdmitted, presentation?.kind, dockPanelKey, publishDockPanels, dockPanelsCurrent, subscribeDockPanels]);
  const drawnDockOwner = dockOwner && dockOwnerRef.current === dockOwner && dockOwner.owner.binding === binding
    && dockOwner.owner.authority === snapshot.authority && !!fence && sameFence(dockOwner.owner.fence, fence)
    && dockOwner.panels.join('|') === dockPanelKey && dockAdmitted ? dockOwner : undefined;
  useLayoutEffect(() => {
    if (!drawnDockOwner || expanded || !publishDockPanels) return;
    const offer = drawnDockOwner.offer();
    if (!offer) return;
    const unpublish = publishDockPanels(offer);
    return () => {offer.retire(); unpublish();};
  }, [drawnDockOwner, expanded, dockOpening, publishDockPanels]);
  useLayoutEffect(() => {if (expanded) drawnDockOwner?.commitExpanded();}, [drawnDockOwner, expanded]);
  const toggleFullscreen = () => {
    if (touchTablet) return;
    if (windowPort?.fullscreen) effect(windowPort.fullscreen.toggle);
    else if (width < 1024) { setFullWindow(value => !value); reveal(); }
  };
  const pictureTarget = () => ({runtime: binding.runtime, snapshot, reportError: binding.reportError,
    isActive: () => current.current.binding.runtime === binding.runtime && current.current.expanded});
  useEffect(() => {
    if (!open) return;
    const keydown = (event: KeyboardEvent) => {
      keyboardFocus.current = true;
      if (event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey || watchShortcutTarget(event.target) || panel || externalPanel || more || shortcuts) return;
      if ((event.key === ' ' || event.key === 'Enter') && event.target instanceof Element && event.target.closest('button,a')) return;
      const step = liveChrome ? null : watchStepIntent(current.current.binding.runtime, current.current.snapshot, event.key);
      if (step) { event.preventDefault(); send(step); reveal(); return; }
      let intent: WatchIntent | undefined;
      const p = current.current.snapshot.presentation;
      if (!p) return;
      const key = event.key.toLowerCase(), enabled = (kind: string) => p.controls.includes(kind as never);
      if (key === ' ' || key === 'k') { const kind = p.finished ? 'replay' : p.playing ? 'pause' : 'play'; if (enabled(kind)) intent = {kind}; }
      else if (key === 'arrowleft' || key === 'j' || key === 'arrowright' || key === 'l') {
        const back = key === 'arrowleft' || key === 'j', position = watchSeek(current.current.snapshot, p.positionSeconds + (back ? -binding.skips.back : binding.skips.forward));
        if (position !== null) intent = {kind: 'seek', seconds: position};
      } else if (!coarsePointer && (key === 'arrowup' || key === 'arrowdown') && enabled('volume')) intent = {kind: 'volume', volume: Math.min(1, Math.max(0, p.volume + (key === 'arrowup' ? .05 : -.05)))};
      else if (!coarsePointer && key === 'm' && enabled('volume')) intent = {kind: 'mute'};
      else if (event.shiftKey && (key === 'n' || key === 'p') && enabled(key === 'n' ? 'next' : 'previous')) intent = {kind: key === 'n' ? 'next' : 'previous'};
      else if ((/^[0-9]$/.test(key) || key === 'home' || key === 'end') && enabled('seek')) {
        const bounds = watchSeekBounds(current.current.snapshot);
        if (bounds) intent = {kind: 'seek', seconds: key === 'home' ? bounds.start : key === 'end' ? bounds.end : bounds.start + (bounds.end - bounds.start) * Number(key) / 10};
      } else if (key === 'f') { event.preventDefault(); toggleFullscreen(); reveal(); return; }
      else if (key === 'i' && windowPort?.pictureInPicture) { event.preventDefault(); effect(windowPort.pictureInPicture.toggle); reveal(); return; }
      else if (key === 'z' && windowPort?.fit) { event.preventDefault(); effect(windowPort.fit.toggle); reveal(); return; }
      else if (key === '?') { event.preventDefault(); setShortcuts(true); reveal(); return; }
      else if (key === 'escape') { event.preventDefault(); if (fullWindow) setFullWindow(false); else if (windowPort?.fullscreen?.active) effect(windowPort.fullscreen.toggle); else onCollapse(); return; }
      if (intent) { event.preventDefault(); send(intent); reveal(); }
    };
    window.addEventListener('keydown', keydown);
    return () => window.removeEventListener('keydown', keydown);
  });
  const bounds = watchSeekBounds(snapshot), position = presentation?.positionSeconds ?? 0, duration = presentation?.durationSeconds ?? 0;
  const activeScrub = scrub && fence && sameFence(scrub.fence, fence) && scrub.snapshot.authority === snapshot.authority ? scrub : null;
  const drawnPosition = activeScrub?.seconds ?? position;
  const previewSeconds = activeScrub?.seconds ?? hoverSeconds;
  const trickplay = surfaces?.trickplay;
  const preview = open && chromeVisible && !panel && !more && !shortcuts ? watchPreviewFrame(snapshot, trickplay, previewSeconds) : null;
  const endsAt = presentation?.playing && duration > position && presentation.rate > 0
    ? new Intl.DateTimeFormat(binding.environment.locale ?? 'en-US', {hour: 'numeric', minute: '2-digit'}).format(new Date(Date.now() + (duration - position) / presentation.rate * 1000)) : undefined;
  const mainKind = presentation?.finished ? 'replay' : presentation?.playing ? 'pause' : 'play';
  const episode = identity?.subtitle.some(part => part.id === 'episode-label');
  // desktop-player f1: Previous and Next follow the queue and name the title they would play.
  const neighbours = watchQueueNeighbours(snapshot);
  const neighbourLabel = (kind: 'previous' | 'next') => {
    if (episode) return t(kind === 'previous' ? 'next.dock.previousEpisode' : 'next.dock.nextEpisode');
    const named = neighbours[kind];
    return named ? t(kind === 'previous' ? 'next.dock.previousNamed' : 'next.dock.nextNamed', {title: named})
      : t(kind === 'previous' ? 'next.dock.previous' : 'next.dock.next');
  };
  const skip = (delta: number) => { const seconds = watchSeek(snapshot, position + delta); if (seconds !== null) send({kind: 'seek', seconds}); };
  const panelButton = (name: WatchPanel, icon: IconName, label: string, large = false) => surfaces?.available.includes(name) ? <Control icon={icon} label={label} large={large} active={panel === name} action={button => showPanel(name, button)} /> : null;
  const transport = <div className={styles.transport}>
    <Control icon="i-prev" label={neighbourLabel('previous')} disabled={!can('previous')} action={() => send({kind: 'previous'})} />
    <Control icon={portrait ? 'i-rew' : 'i-back'} label={t('next.dock.back', {seconds: binding.skips.back})} skip={binding.skips.back} disabled={!can('seek')} action={() => skip(-binding.skips.back)} />
    <Control main icon={mainKind === 'replay' ? 'i-back' : mainKind === 'pause' ? 'i-pause' : 'i-play'} label={t(mainKind === 'replay' ? 'next.dock.replay' : mainKind === 'pause' ? 'next.dock.pause' : 'next.dock.play')} disabled={!can(mainKind)} action={() => send({kind: mainKind})} />
    <Control icon={portrait ? 'i-ffw' : 'i-fwd'} label={t('next.dock.forward', {seconds: binding.skips.forward})} skip={binding.skips.forward} disabled={!can('seek') || presentation?.finished} action={() => skip(binding.skips.forward)} />
    <Control icon="i-next" label={neighbourLabel('next')} disabled={!can('next')} action={() => send({kind: 'next'})} />
  </div>;
  const quality = surfaces?.quality && fence && sameFence(surfaces.quality.fence, fence) ? surfaces.quality : undefined;
  // desktop-player f4b: the film-end card stands in for the transport once nothing follows.
  const endCard = surfaces?.end && presentation?.finished && snapshot.queueView?.next.available !== true ? surfaces.end : undefined;
  const tracks = surfaces?.tracks && fence && sameFence(surfaces.tracks.fence, fence) ? surfaces.tracks : undefined;
  const namedTrack = (section: 'audio' | 'subtitles', label: string | undefined) => tracks && label !== undefined && surfaces?.available.includes(section) ? <button type="button" className={styles.trackPill} onClick={event => showPanel(section, event.currentTarget)}>
    <Icon name={section === 'subtitles' ? 'i-cc' : 'i-audio'} size={17} />{t(section === 'subtitles' ? 'player.opt.subtitles' : 'player.opt.audio')}<span>{label}</span>
  </button> : null;
  const qualityButton = quality && surfaces?.available.includes('quality') ? <button type="button" className={styles.pill} aria-label={quality.accessibleLabel} title={t('player.opt.quality')} onClick={event => showPanel('quality', event.currentTarget)}>
    {portrait && <Icon name="i-quality" size={15} />}{quality.automatic && <span>{t('next.controls.auto')}</span>}{quality.label}
  </button> : null;
  const menuItems: MenuEntry[] = [
    ...(surfaces?.available.includes('info') ? [{id: 'info', label: t('player.opt.info'), icon: 'i-info' as const, onSelect: () => showPanel('info', moreAnchor.current ?? undefined)}] : []),
    ...(windowPort?.fit ? [{id: 'fit', label: t(windowPort.fit.value === 'cover' ? 'web.player.fit.contain' : 'web.player.fit.fill'), icon: 'i-fith' as const, onSelect: () => effect(windowPort.fit!.toggle)}] : []),
    ...(surfaces?.more ?? []).map(item => ({id: item.id, label: item.label, onSelect: () => effect(item.action)})),
    ...(liveChrome ? [{id: 'live-recordings', label: t('guide.viewRecordings'), icon: 'i-rec' as const, onSelect: liveChrome.openRecordings}] : []),
    {id: 'shortcuts', label: t('player.shortcuts'), icon: 'i-kbd', onSelect: () => setShortcuts(true)},
  ];
  const liveRight = <>
    {!coarsePointer && can('volume') && <span className={styles.volume}><Control icon="i-vol" label={t(presentation?.muted ? 'player.unmute' : 'player.mute')} action={() => send({kind: 'mute'})} />
      <Slider min={0} max={1} step={.01} value={presentation?.muted ? 0 : presentation?.volume} aria-label={t('player.volume')} onValueChange={volume => send({kind: 'volume', volume})} /></span>}
    {panelButton('subtitles-audio', 'i-cc', t('next.dock.subtitlesAudio'))}
    {qualityButton}
    {windowPort?.pictureInPicture && <Control icon="i-pip" label={t('player.pictureInPicture')} active={windowPort.pictureInPicture.active} action={() => effect(windowPort.pictureInPicture!.toggle)} />}
    {!touchTablet && (portrait || !windowPort?.fullscreen || !mobile) && <Control icon="i-full" label={t('next.states.shortcut.fullscreen')} active={fullWindow || windowPort?.fullscreen?.active} action={toggleFullscreen} />}
  </>;
  const stillWatching = surfaces?.modal === 'still-watching';
  const promptSource = useMemo<Omit<PromptFocusHost, 'source' | 'seekPresent' | 'focusAllowed' | 'controlsVisible'> | undefined>(() => {
    if (!fence) return;
    const original = {binding, authority: snapshot.authority, fence, intentId: snapshot.playback.intentId, run: snapshot.playback.playbackRun};
    return {root, seek: promptSeek, current: () => {
      try {
        const live = original.binding.runtime.getSnapshot();
        return !live.disposed && live.authority === original.authority && sameFence(live.fence, original.fence)
          && live.playback.intentId === original.intentId && live.playback.playbackRun === original.run
          && live.presentation?.kind === 'video' && sameFence(live.presentation.fence, original.fence)
          && live.dock.local?.expanded === true && sameFence(live.dock.local.fence, original.fence) && live.dock.remote?.phase !== 'playing';
      } catch {return false;}
    }};
  }, [binding, snapshot.authority, sourceKey, snapshot.playback.intentId, snapshot.playback.playbackRun]);
  const promptHost = useMemo<PromptFocusHost | undefined>(() => promptSource ? {...promptSource, source: promptSource, seekPresent: !endCard && chromeVisible && !!bounds,
    controlsVisible: chromeVisible, focusAllowed: !panel && !externalPanel && !more && !shortcuts && !stillWatching} : undefined,
  [promptSource, !!endCard, !!bounds, chromeVisible, panel, externalPanel, more, shortcuts, stillWatching]);
  return <div ref={root} className={styles.watch} hidden={!open} tabIndex={-1} data-watch-screen data-mode={mode} data-still-watching={stillWatching || undefined} data-touch-expanded={coarsePointer && width >= 1024 || undefined} data-controls-visible={chromeVisible} data-controls-exit={controlsExit} data-panel-open={!!panel} data-scrubbing={!!activeScrub && mobile}
    onPointerDownCapture={() => { keyboardFocus.current = false; }}
    onPointerMove={event => { if (event.pointerType === 'mouse') reveal(); }} onFocusCapture={reveal}>
    <div ref={surfaceSlotRef} className={styles.pictureSlot} style={rectStyle(geometry?.picture)} data-watch-picture-slot data-media-slot="expanded" aria-hidden="true" />
    <div ref={chrome} className={styles.chrome} style={rectStyle(geometry?.controls)} inert={stillWatching || undefined}>
      {open && snapshot.playback.phase === 'error' && <PlaybackFailure key={playbackKey} error={snapshot.playback.error}
        onRetry={fence && binding.runtime.canRetryFailure(fence) ? () => binding.runtime.retryFailure(fence) : undefined}
        onVersions={surfaces?.available.includes('versions') ? () => showPanel('versions') : undefined}
        onReport={surfaces?.more?.find(action => action.id === 'report-problem') ? () => effect(() => surfaces.more!.find(action => action.id === 'report-problem')!.action()) : undefined}
        closeLabel={surfaces?.convertingOwnerAction ? t('web.player.error.turnOnConverting') : undefined}
        onClose={surfaces?.convertingOwnerAction ?? onCollapse} onError={binding.reportError}>{surfaces?.streamLimit}</PlaybackFailure>}
      <PlaybackWaiting key={playbackKey} waiting={open && snapshot.playback.recovering === true && snapshot.playback.intent === 'playing' && snapshot.playback.phase !== 'error' && !snapshot.problem}/>
      <button type="button" className={styles.pictureTap} aria-label={t('player.stageLabel', {title: identity?.title ?? ''})}
        onPointerDown={event => { pointerKind.current = event.pointerType; event.preventDefault(); }}
        onPointerCancel={() => pictureInput.cancel()}
        onClick={event => {
          if (externalPanel) {pictureInput.cancel();surfaces?.playOn?.close();return;}
          if (panel) { pictureInput.cancel(); setPanel(null); return; }
          if (more || shortcuts) return;
          const target = pictureTarget();
          if (event.detail === 0 || pointerKind.current === 'mouse') { pictureInput.mouse(target, event.detail); return; }
          const rect = event.currentTarget.getBoundingClientRect();
          const x = rect.width > 0 ? (event.clientX - rect.left) / rect.width : .5, y = rect.height > 0 ? (event.clientY - rect.top) / rect.height : .5;
          pictureInput.tap(target, {zone: x < 1 / 3 ? 'left' : x > 2 / 3 ? 'right' : 'middle', tablet: touchTablet,
            back: binding.skips.back, forward: binding.skips.forward,
            single: () => {
              if (!controlsVisible.current) { controlsVisible.current = true; reveal(); }
              else if (presentation?.playing && !permanent) { controlsVisible.current = false; clearTimer(); setControlsExit('tap'); setVisible(false); }
            },
            fullscreen: toggleFullscreen,
            feedback: (direction, seconds) => setTapFeedback(value => ({direction, seconds, x, y, sequence: (value?.sequence ?? 0) + 1})),
          });
        }}
        onDoubleClick={() => { if (pointerKind.current === 'mouse') pictureInput.doubleClick(pictureTarget(), toggleFullscreen); }}
        onWheel={event => { if (!coarsePointer && !panel && !more && !shortcuts) pictureInput.wheel(pictureTarget(), event.deltaY); }} />
      {tapFeedback && <div key={tapFeedback.sequence} className={styles.tapFeedback} data-direction={tapFeedback.direction} aria-hidden="true" onAnimationEnd={() => setTapFeedback(null)}>
        <span className={styles.tapDot} style={{left: `${(tapFeedback.x - (tapFeedback.direction === 'right' ? .56 : 0)) / .44 * 100}%`, top: `${tapFeedback.y * 100}%`}} />
        <Icon name={tapFeedback.direction === 'left' ? 'i-back' : 'i-fwd'} size={40} />
        <span>{t('duration.long.seconds', {seconds: tapFeedback.seconds})}</span>
      </div>}
      {liveChrome ? open && <div className={styles.liveOverlay} data-visible={chromeVisible} inert={!chromeVisible || undefined}>
        <LiveChrome port={liveChrome} playing={!!presentation?.playing} onCollapse={onCollapse} onStop={() => send({kind: 'stop'})}
          onPlayPause={() => send({kind: mainKind})} onSkip={skip} onSeek={seconds => {if (can('seek')) send({kind: 'seek', seconds});}}
          onMore={anchor => {moreAnchor.current = anchor;setPanel(null);setMore(value => !value);}}
          right={liveRight} onMiniGuideOpenChange={setLiveGuideOpen} canPlayPause={can(mainKind)} canSeek={can('seek')} reportError={binding.reportError}/>
      </div> : <>
        <div className={styles.shade} />
        {endCard?.backdrop && <><div className={styles.endAmbient} aria-hidden="true"><Artwork source={endCard.backdrop} alt="" width="100%" height="100%" /></div><div className={styles.endVeil} aria-hidden="true" /></>}
        {!(endCard && mobile) && <div className={styles.top} inert={!chromeVisible || undefined}>
          <Control large icon="i-down" label={t('player.minimize')} action={onCollapse} />
          <div className={styles.identity}><h2>{identity?.title}</h2><p>{identity && <Facts parts={identity.subtitle} />}</p></div>
          <div className={styles.together}>{surfaces?.together}</div>
          <div className={styles.tools}>
            {!endCard && panelButton('play-on', 'i-cast', t('player.playOn'), true)}
            {mobile && windowPort?.pictureInPicture && <Control large icon="i-pip" label={t('player.pictureInPicture')} action={() => effect(windowPort.pictureInPicture!.toggle)} active={windowPort.pictureInPicture.active} />}
            {portrait && panelButton('subtitles-audio', 'i-cc', t('next.dock.subtitlesAudio'))}
            {mobile && panelButton('options', 'i-gear', t('next.dock.options'))}
            {!mobile && !endCard && <Control large buttonRef={moreAnchor} icon="i-more" label={t('player.more')} active={more} action={() => { setPanel(null); setMore(value => !value); }} />}
            {!mobile && <Control large icon="i-close" label={t('player.close')} action={() => send({kind: 'stop'})} />}
          </div>
        </div>}
        {endCard ? <WatchEndCardView card={endCard} canReplay={can('replay')} onReplay={() => send({kind: 'replay'})} onClose={() => send({kind: 'stop'})} layout={mobile ? 'phone' : 'desktop'} reportError={binding.reportError}
          artwork={url => binding.artwork.foreground(url, 512, 16 / 9, {group: 'playback', priority: 'high'})} /> : <>
        <div className={styles.center} inert={!chromeVisible || undefined}>{transport}</div>
        <div className={styles.bottom} inert={!chromeVisible || undefined}>
          <div className={styles.seek}>
            <span className={styles.rail}><i style={{width: `${bounds ? Math.min(100, Math.max(0, (drawnPosition - bounds.start) / (bounds.end - bounds.start) * 100)) : 0}%`}} />
              {presentation?.policy?.markers.map(marker => duration > 0 && <span key={marker.id} className={styles.marker} data-kind={marker.kind}
                style={{...(marker.kind === 'credits' ? {right: `${(duration - marker.endSeconds) / duration * 100}%`} : {left: `${marker.startSeconds / duration * 100}%`}), width: `${(marker.endSeconds - marker.startSeconds) / duration * 100}%`}} />)}
              {preview && bounds && previewSeconds !== null && <ScrubPreview frame={preview.frame} source={preview.source} seconds={previewSeconds} phone={mobile}
                percent={(previewSeconds - bounds.start) / (bounds.end - bounds.start) * 100} />}
              <input ref={promptSeek} tabIndex={0} type="range" min={bounds?.start ?? 0} max={bounds?.end ?? 1} step={.1} value={bounds ? Math.min(bounds.end, Math.max(bounds.start, drawnPosition)) : 0} disabled={!bounds}
                aria-label={t('player.position')} aria-valuetext={t('player.clockOf', {position: watchClock(drawnPosition), duration: watchClock(duration)})}
                onPointerDown={event => {
                  if (scrubDrag.current || !bounds || !fence) return;
                  const next = {snapshot, fence, seconds: drawnPosition, pointerId: event.pointerId};
                  scrubDrag.current = next; setScrub(next); clearTimer(); setVisible(true);
                  event.currentTarget.setPointerCapture(event.pointerId);
                }}
                onPointerMove={event => {
                  if (coarsePointer || scrubDrag.current || event.pointerType !== 'mouse' || !bounds || !trickplay) return;
                  const rect = event.currentTarget.getBoundingClientRect();
                  if (rect.width > 0) setHoverSeconds(watchSeek(snapshot, bounds.start + (event.clientX - rect.left) / rect.width * (bounds.end - bounds.start)));
                }}
                onPointerLeave={() => { if (!scrubDrag.current) setHoverSeconds(null); }}
                onPointerUp={event => {
                  const captured = scrubDrag.current;
                  if (!captured || captured.pointerId !== event.pointerId) return;
                  const seconds = watchSeek(captured.snapshot, event.currentTarget.valueAsNumber), value = current.current, live = value.binding.runtime.getSnapshot();
                  scrubDrag.current = null; setScrub(null); setHoverSeconds(null); reveal();
                  if (seconds !== null && value.expanded && live.fence && sameFence(live.fence, captured.fence) && live.authority === captured.snapshot.authority && live.dock.remote?.phase !== 'playing') {
                    void dispatchWatch(value.binding.runtime, captured.snapshot, {kind: 'seek', seconds}).catch(value.binding.reportError);
                  }
                }}
                onPointerCancel={() => { scrubDrag.current = null; setScrub(null); setHoverSeconds(null); reveal(); }}
                onBlur={() => { if (!scrubDrag.current) setHoverSeconds(null); }}
                onChange={event => {
                  const captured = scrubDrag.current, seconds = watchSeek(captured?.snapshot ?? snapshot, event.currentTarget.valueAsNumber);
                  if (seconds === null) return;
                  if (captured) { const next = {...captured, seconds}; scrubDrag.current = next; setScrub(next); }
                  else { setHoverSeconds(seconds); send({kind: 'seek', seconds}); }
                }} />
            </span>
          </div>
          <div className={styles.times}>{portrait
            ? <span className={styles.clock}><b>{watchClock(drawnPosition)}</b>{duration > 0 && <span>{`/ ${watchClock(duration)}`}</span>}</span>
            : <><b>{watchClock(drawnPosition)}</b><span>{!mobile && endsAt && <>{t('guide.ends', {time: endsAt})}<span className={styles.gap} aria-hidden="true" /></>}<b>{duration > 0 ? `−${watchClock(Math.max(0, duration - drawnPosition))}` : ''}</b></span></>}</div>
          <div className={styles.controls}>
            <div className={styles.leading}>{surfaces?.available.includes('episodes')
              ? <Button size="small" variant="glass" icon="i-stack" onClick={event => showPanel('episodes', event.currentTarget)}>{t('next.controls.episodes')}</Button>
              : surfaces?.available.includes('queue') ? <Button size="small" variant="glass" icon="i-queue" onClick={event => showPanel('queue', event.currentTarget)}>{t('player.queue')}</Button> : null}</div>
            {transport}
            <div className={styles.trailing}>
              {!coarsePointer && can('volume') && <span className={styles.volume}><Control icon="i-vol" label={t(presentation?.muted ? 'player.unmute' : 'player.mute')} action={() => send({kind: 'mute'})} />
                <Slider min={0} max={1} step={.01} value={presentation?.muted ? 0 : presentation?.volume} aria-label={t('player.volume')} onValueChange={volume => send({kind: 'volume', volume})} /></span>}
              {!portrait && !mobile && panelButton('subtitles-audio', 'i-cc', t('next.dock.subtitlesAudio'))}
              {!portrait && mobile && <>{namedTrack('subtitles', tracks?.subtitles)}{namedTrack('audio', tracks?.audio)}</>}
              {qualityButton}
              {!mobile && surfaces?.available.includes('speed') && <button type="button" className={styles.pill} aria-label={t('next.dock.speed', {rate: presentation?.rate ?? 1})} onClick={event => showPanel('speed', event.currentTarget)}>{t('next.dock.rate', {rate: presentation?.rate ?? 1})}</button>}
              {!mobile && windowPort?.pictureInPicture && <Control icon="i-pip" label={t('player.pictureInPicture')} active={windowPort.pictureInPicture.active} action={() => effect(windowPort.pictureInPicture!.toggle)} />}
              {!touchTablet && (portrait || !windowPort?.fullscreen || !mobile) && <Control icon="i-full" label={t('next.states.shortcut.fullscreen')} active={fullWindow || windowPort?.fullscreen?.active} action={toggleFullscreen} />}
            </div>
          </div>
        </div>
        </>}
      </>}
    </div>
    {open && surfaces?.loss && <LossNotice notice={surfaces.loss.notice} enabled={surfaces.loss.enabled} layout={mobile ? 'phone' : 'desktop'} />}
    <div className={styles.notices}>{open && surfaces?.notices}</div>
    <div className={styles.prompts}><WatchPromptFocus.Provider value={open ? promptHost : undefined}>{open && surfaces?.prompts}</WatchPromptFocus.Provider></div>
    <div ref={body} className={styles.body} style={rectStyle(geometry?.body)} data-watch-retained-body onScroll={event => {
      if (!bodyVisible || !open) return;
      const element = event.currentTarget, top = element.getBoundingClientRect().top;
      const anchor = Array.from(element.querySelectorAll<HTMLElement>('[data-episode-id],[data-entry-id],[data-item-id],h2,h3')).find(value => value.getBoundingClientRect().bottom > top);
      bodyScroll.current = {identity: surfaces?.bodyIdentity, top: element.scrollTop, left: element.scrollLeft, anchor, offset: anchor ? anchor.getBoundingClientRect().top - top : undefined};
    }}>
      {surfaces?.body}
    </div>
    {open && surfaces?.render({panel, snapshot, available: surfaces.available, reportProblem: surfaces.more?.find(action => action.id === 'report-problem'), layout: width < 1024 ? 'phone' : 'desktop', phoneTop: portrait ? geometry?.picture ? geometry.picture.top + geometry.picture.height : width * 9 / 16 + (window.visualViewport?.offsetTop ?? 0) : 0, close: () => setPanel(null), show: next => showPanel(next), returnFocusRef: panelAnchor})}
    <Menu open={more} onOpenChange={setMore} title={t('player.more')} anchorRef={moreAnchor} returnFocusRef={moreAnchor} presentation="anchored" width={300} items={menuItems} />
    <KeyboardShortcuts open={shortcuts} onOpenChange={setShortcuts} skipBackSeconds={binding.skips.back} skipForwardSeconds={binding.skips.forward} returnFocusRef={moreAnchor} />
  </div>;
}
