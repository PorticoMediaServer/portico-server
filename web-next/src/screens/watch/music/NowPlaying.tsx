import {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {playbackIdentity} from '@core/presentation/kit-model/index.ts';
import {Artwork} from '../../../kit/artwork';
import {Icon} from '../../../kit/Icon';
import {Slider} from '../../../kit/controls';
import {Menu} from '../../../kit/overlays';
import {router} from '../../../router';
import {ListeningNotice, SignalLine, SignalPath} from '../../../kit/media';
import {dispatchWatch, watchClock, watchSeek, watchSeekBounds, type WatchIntent} from '../model';
import {watchQueueNeighbours} from '../queue-neighbours';
import {watchMusicSignal} from './signal';
import {readWatchMusicAudioFacts} from './audio-facts';
import {fenceKey} from '../../../shell/playback-model';
import {WatchEqualizerPanel, hostAdmit, surfaceAdmit, type WatchHostFacts, type WatchReceipt, type WatchSurfaceReceipt} from './EqualizerPanel';
import type {WatchBindings, WatchScreenProps} from '../types';
import styles from './now-playing.module.css';
import type {MusicExpansion} from './expansion-owner';

const t = defaultI18n.t;
const noSubscribe = () => () => {};
const noEqualizer = () => null;

/** The drawn expanded Now playing host for music (listening-notices frames 2 and 3).
 * It is the same authority-local runtime and the same retained media owner; this view
 * only presents facts, and the actionable DockNotice stays with the dock. */
export function WatchMusicNowPlaying({binding, snapshot, expanded, onCollapse, phone, motion, isCurrent}: Pick<WatchScreenProps, 'binding' | 'snapshot' | 'expanded' | 'onCollapse'> & {phone: boolean; motion?: MusicExpansion; isCurrent?: () => boolean}) {
  const presentation = snapshot.presentation;
  const environment = binding.environment;
  const equalizer = binding.runtime.equalizerPort;
  const settings = useSyncExternalStore(equalizer?.subscribe ?? noSubscribe, equalizer?.getSnapshot ?? noEqualizer, noEqualizer);
  useEffect(() => { if (!equalizer) return; return equalizer.start(); }, [equalizer]);
  const [signalOpen, setSignalOpen] = useState(false);
  const [eqOpen, setEqOpen] = useState(false), [moreOpen, setMoreOpen] = useState(false);
  const signalAnchor = useRef<HTMLButtonElement>(null), moreAnchor = useRef<HTMLButtonElement>(null);
  const signal = watchMusicSignal(snapshot, settings, readWatchMusicAudioFacts(binding.runtime, snapshot));
  const identity = presentation ? playbackIdentity(presentation.subject, 'full', environment) : undefined;
  const artworkSource = presentation ? binding.artwork.foreground(presentation.posterUrl ?? presentation.backdropUrl, 560, 1, {group: 'playback', priority: 'high'}) : undefined;
  const open = expanded && presentation?.kind === 'music' && !!identity && (!isCurrent || isCurrent()) && (!motion || !!motion.scope);
  useLayoutEffect(() => {if (!open) {setSignalOpen(false); setEqOpen(false); setMoreOpen(false);}}, [open]);
  // Immutable rendered facts. A receipt is minted fresh for each layout setup/replay.
  const [hostReceipt, setHostReceipt] = useState<WatchReceipt | null>(null);
  const publishedHost = useRef<WatchReceipt | null>(null);
  const [moreReceipt, setMoreReceipt] = useState<WatchSurfaceReceipt | null>(null), publishedMore = useRef<WatchSurfaceReceipt | null>(null);
  const [eqReceipt, setEqReceipt] = useState<WatchSurfaceReceipt | null>(null), publishedEq = useRef<WatchSurfaceReceipt | null>(null);
  const fenceAt = snapshot?.fence ? fenceKey(snapshot.fence) : '', presentationAt = snapshot?.presentation?.fence ? fenceKey(snapshot.presentation.fence) : '';
  const presentationKind = snapshot?.presentation?.kind, authority = snapshot?.authority, playbackRun = snapshot?.playback.playbackRun;
  const scope = motion?.scope;
  const facts = useMemo<WatchHostFacts | null>(() => !open || !equalizer || presentationKind !== 'music' || !fenceAt || !presentationAt || fenceAt !== presentationAt
    ? null : {runtime: binding.runtime, port: equalizer, authority, fenceAt, presentationAt, run: playbackRun, ...(isCurrent ? {isCurrent} : {}), ...(scope ? {scope} : {})},
    [open, equalizer, presentationKind, fenceAt, presentationAt, binding.runtime, authority, playbackRun, isCurrent, scope]);
  useLayoutEffect(() => {
    if (!facts) {
      const old = publishedHost.current; publishedHost.current = null; if (old) old.live = false;
      setHostReceipt(null); setMoreReceipt(null); setEqReceipt(null); setEqOpen(false); setMoreOpen(false); return;
    }
    const receipt: WatchReceipt = {facts, live: true};
    publishedHost.current = receipt;
    setHostReceipt(receipt); setMoreReceipt(null); setEqReceipt(null); setEqOpen(false); setMoreOpen(false);
    return () => {receipt.live = false; if (publishedHost.current === receipt) publishedHost.current = null;};
  }, [facts]);
  useLayoutEffect(() => {
    if (!moreOpen || !hostReceipt) {const old = publishedMore.current; publishedMore.current = null; if (old) old.live = false; setMoreReceipt(null); return;}
    const receipt: WatchSurfaceReceipt = {host: hostReceipt, live: true};
    publishedMore.current = receipt; setMoreReceipt(receipt);
    return () => {receipt.live = false; if (publishedMore.current === receipt) publishedMore.current = null;};
  }, [moreOpen, hostReceipt]);
  useLayoutEffect(() => {
    if (!eqOpen || !hostReceipt) {const old = publishedEq.current; publishedEq.current = null; if (old) old.live = false; setEqReceipt(null); return;}
    const receipt: WatchSurfaceReceipt = {host: hostReceipt, live: true};
    publishedEq.current = receipt; setEqReceipt(receipt);
    return () => {receipt.live = false; if (publishedEq.current === receipt) publishedEq.current = null;};
  }, [eqOpen, hostReceipt]);
  // A surface retires synchronously before its state changes, so a saved callback cannot act in
  // the same tick as the close/selection that replaced it.
  const retireMore = (receipt: WatchSurfaceReceipt | null) => {
    if (!receipt || publishedMore.current !== receipt) return false;
    receipt.live = false; publishedMore.current = null; setMoreReceipt(null); return true;
  };
  const closeEq = (receipt: WatchSurfaceReceipt | null) => {
    if (!receipt || publishedEq.current !== receipt || !receipt.live) return;
    receipt.live = false; publishedEq.current = null; setEqReceipt(null); setEqOpen(false);
  };
  const present = motion ? motion.present : open;
  if (!present || !presentation || !identity || isCurrent && !isCurrent()) return null;
  const send = (intent: WatchIntent) => { if (!open || motion?.scope && !motion.scope.current()) return;void dispatchWatch(binding.runtime, snapshot, intent).catch(binding.reportError); };
  const bounds = watchSeekBounds(snapshot), at = presentation.positionSeconds, duration = presentation.durationSeconds;
  const can = (kind: string) => presentation.controls.includes(kind as never);
  const canSeek = can('seek') && !!bounds;
  const neighbours = watchQueueNeighbours(snapshot);
  const artist = identity.subtitle.map(part => part.text).join(' · ');
  const from = artist || identity.title;
  const code = snapshot.playback.listeningNotice;
  const notice = code ? {code, sequence: snapshot.sequence} : undefined;
  const seek = (seconds: number) => { const value = watchSeek(snapshot, seconds); if (value !== null) send({kind: 'seek', seconds: value}); };
  return <section ref={motion?.scope ? motion.frame : undefined} className={styles.frame} data-music-motion-owned={!!motion?.scope || undefined} inert={!open || undefined} aria-hidden={!open || undefined} aria-label={t('next.music.nowPlaying')} data-watch-music>
    <header className={styles.top}>
      <button type="button" className={styles.round} aria-label={t('player.minimize')} onClick={onCollapse}><Icon name="i-down" size={22} /></button>
      <span className={styles.from}>{t('next.music.playingFrom', {source: from})}</span>
      <ListeningNotice notice={notice} paused={snapshot.playback.intent === 'paused'}
        stopped={snapshot.disposed || snapshot.playback.phase === 'idle' || snapshot.playback.phase === 'ended'}
        errorShown={snapshot.playback.phase === 'error' || !!snapshot.playback.error || !!snapshot.problem}
        expanded layout={phone ? 'phone' : 'desktop'} translate={t} />
      <span className={styles.spacer} />
      {equalizer ? <button ref={moreAnchor} type="button" className={styles.glass} aria-label={t('player.more')} aria-haspopup="menu" aria-expanded={moreOpen}
        onClick={() => {
        if (moreOpen) {
          if (!surfaceAdmit(moreReceipt, publishedMore, publishedHost, hostReceipt) || !retireMore(moreReceipt)) return;
          setMoreOpen(false); return;
        }
        if (hostAdmit(hostReceipt, publishedHost, facts)) setMoreOpen(true);
      }}><Icon name="i-more" size={18} /></button> : null}
      {equalizer ? <Menu open={moreOpen && open} onOpenChange={value => {
        if (!surfaceAdmit(moreReceipt, publishedMore, publishedHost, hostReceipt)) return;
        if (!value) retireMore(moreReceipt);
        setMoreOpen(value);
      }} title={t('player.more')} anchorRef={moreAnchor} returnFocusRef={moreAnchor} presentation="anchored" width={280}
        items={[{id: 'equalizer', label: t('next.media.eq.title'), description: t(settings?.enabled === true ? 'next.settings.on' : 'next.settings.off'),
          onSelect: () => {if (!surfaceAdmit(moreReceipt, publishedMore, publishedHost, hostReceipt) || !retireMore(moreReceipt)) return; setMoreOpen(false); setEqOpen(true);}}]} /> : null}
      {can('pause') || can('play') ? <button type="button" className={styles.glass} onClick={() => send({kind: presentation.playing ? 'pause' : 'play'})}>
        <Icon name={presentation.playing ? 'i-pause' : 'i-play'} size={18} />{presentation.playing ? t('next.dock.pause') : t('next.dock.play')}</button> : null}
    </header>
    <div className={styles.main}>
      <div ref={motion?.scope ? motion.cover : undefined} className={styles.cover}>{!motion?.scope && <Artwork source={artworkSource} alt={identity.title} />}</div>
      <div className={styles.info}>
        <div className={styles.fade1} data-music-content-group="0"><h1>{identity.title}</h1><p>{artist}</p></div>
        {signal && <div className={styles.signal}><SignalLine model={signal} translate={t} layout={phone ? 'phone' : 'desktop'}
          ref={signalAnchor} aria-haspopup="dialog" aria-expanded={signalOpen} onClick={() => setSignalOpen(true)} />
          <SignalPath model={signal} translate={t} locale={environment.locale} layout={phone ? 'phone' : 'desktop'}
            open={open && signalOpen} onOpenChange={setSignalOpen} anchorRef={signalAnchor} /></div>}
        <div className={`${styles.scrub} ${styles.fade2}`} data-music-content-group="1">
          <span>{watchClock(at)}</span>
          <input type="range" className={styles.track} min={bounds?.start ?? 0} max={bounds?.end ?? Math.max(duration, 1)} step={.1}
            value={Math.min(bounds?.end ?? duration, Math.max(bounds?.start ?? 0, at))} disabled={!canSeek}
            aria-label={t('player.position')} aria-valuetext={t('player.clockOf', {position: watchClock(at), duration: watchClock(duration)})}
            onChange={event => { const value = event.currentTarget.valueAsNumber; if (Number.isFinite(value)) seek(value); }} />
          <span>{watchClock(duration)}</span>
        </div>
        <div className={`${styles.controls} ${styles.fade2}`} data-music-content-group="1">
          <button type="button" className={styles.ctl} aria-label={t('next.dock.previous')} disabled={!can('previous')} onClick={() => send({kind: 'previous'})}><Icon name="i-prev" size={24} /></button>
          <button type="button" className={styles.big} aria-label={presentation.playing ? t('next.dock.pause') : t('next.dock.play')} disabled={!can(presentation.playing ? 'pause' : 'play')}
            onClick={() => send({kind: presentation.playing ? 'pause' : 'play'})}><Icon name={presentation.playing ? 'i-pause' : 'i-play'} size={28} /></button>
          <button type="button" className={styles.ctl} aria-label={t('next.dock.next')} disabled={!can('next')} onClick={() => send({kind: 'next'})}><Icon name="i-next" size={24} /></button>
        </div>
        {can('volume') && <div className={`${styles.volume} ${styles.fade2}`} data-music-content-group="1"><button type="button" className={styles.ctl} aria-label={t(presentation.muted ? 'player.unmute' : 'player.mute')} onClick={() => send({kind: 'mute'})}><Icon name="i-vol" size={20} /></button>
          <Slider min={0} max={1} step={.01} value={presentation.muted ? 0 : presentation.volume} aria-label={t('player.volume')} onValueChange={volume => send({kind: 'volume', volume})} /></div>}
      </div>
    </div>
    <aside className={styles.side} aria-label={eqOpen ? t('next.media.eq.title') : t('next.music.upNext')}>
      {eqOpen && equalizer && facts && hostReceipt && eqReceipt ? <WatchEqualizerPanel receipt={eqReceipt} rendered={eqReceipt} published={publishedEq}
        host={hostReceipt.facts} hostReceipt={hostReceipt} publishedHost={publishedHost}
        runtime={binding.runtime} port={equalizer} {...(isCurrent ? {isCurrent} : {})} {...(scope ? {scope} : {})}
        close={() => closeEq(eqReceipt)}
        openSettings={() => {if (!open) return; void router.navigate({to: '/settings', search: () => ({scope: 'account' as const, section: 'music'})});}} />
        : <>
        <h2>{t('next.music.upNext')}</h2>
        <p className={styles.section}>{t('next.music.nowPlaying')}</p>
        <div className={styles.queueRow}><span className={styles.eq} aria-hidden="true"><i /><i /><i /></span><span><b>{identity.title}</b><small>{artist}</small></span><span className={styles.duration}>{watchClock(duration)}</span></div>
        {neighbours.next && <div className={styles.fade3} data-music-content-group="2"><p className={styles.section}>{t('next.dock.next')}</p>
          <div className={styles.queueRow}><span><b>{neighbours.next}</b></span></div></div>}
        </>}
    </aside>
  </section>;
}
