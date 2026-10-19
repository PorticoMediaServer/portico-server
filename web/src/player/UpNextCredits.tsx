import React, {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {Artwork, Button, Text} from '../ui';
import {useI18n} from '../app/i18n';
import {formatClock} from '@core/presentation/index.ts';
import type {PlayerEngine} from './engine';
import type {SkipMode} from './SkipPrompt';
import {upNextCountdown, upNextOffer, type ClosingMarker, type UpNextOffer} from '@core/up-next.ts';
import {followedByScene} from '@core/segment-skip.ts';
import s from './Player.module.css';

type NextTitle = Readonly<{title: string; kind: string; posterUrl?: string; stillUrl?: string}>;

/**
 * The next title, offered in a corner while the credits run, with its artwork and a countdown.
 * "Play now" goes straight to it; "Watch credits" puts the card away for this title. When a scene
 * follows the credits, "Skip credits" goes to that scene instead. The count
 * starts the next title by itself only where `upNextCountdown` says so (the viewer's "Start the next
 * episode during the credits", off by default, with autoplay on and credits the server is sure of);
 * otherwise it says when the end card will start it.
 */
export function useUpNextDuringCredits(engine: PlayerEngine, markers: readonly ClosingMarker[] | undefined, mode: SkipMode, enabled: boolean) {
  const {queue, state} = engine;
  const view = useSyncExternalStore(queue?.subscribe ?? (() => () => {}), () => queue?.getSnapshot()?.view ?? null);
  const [dismissed, setDismissed] = useState<string | null>(null);
  useEffect(() => { setDismissed(null); }, [state.session?.id]);
  const next = view?.next.entryId ? view.items.find(item => item.entryId === view.next.entryId) : undefined;
  const offer = enabled && state.phase === 'ready' && next
    ? upNextOffer({markers, positionSeconds: state.positionSeconds, durationSeconds: state.duration, kind: engine.identity.kind, mode, nextAvailable: !!view?.next.available, passoutDue: !!view?.postPlay.passoutCheckDue})
    : null;
  const shown = offer && dismissed !== offer.id ? offer : null;
  return {offer: shown, next, postPlay: view?.postPlay, dismiss: () => { if (offer) setDismissed(offer.id); }};
}

export function UpNextCredits({engine, offer, next, postPlay, rate, onDismiss}: {engine: PlayerEngine; offer: UpNextOffer; next: NextTitle; postPlay?: Readonly<{autoplay: boolean; countdownSeconds: number; startDuringCredits?: boolean}>; rate: number; onDismiss: () => void}) {
  const {t} = useI18n();
  const {state} = engine;
  // Seconds of this offer spent playing: a pause holds the count where it is.
  const [elapsed, setElapsed] = useState(0);
  const playing = state.intent === 'playing' && !state.pendingSeek;
  useEffect(() => { setElapsed(0); }, [offer.id]);
  useEffect(() => {
    if (!playing) return;
    let last = Date.now();
    const id = setInterval(() => { const now = Date.now(); setElapsed(e => e + (now - last) / 1000); last = now; }, 250);
    return () => clearInterval(id);
  }, [playing, offer.id]);
  const countdown = upNextCountdown({offer, positionSeconds: state.positionSeconds, durationSeconds: state.duration, rate, autoplay: postPlay?.autoplay ?? true, countdownSeconds: postPlay?.countdownSeconds ?? 10, startDuringCredits: postPlay?.startDuringCredits === true, elapsedSeconds: elapsed});
  const moved = useRef('');
  const leave = (automatic: boolean) => {
    if (moved.current === offer.id) return;
    moved.current = offer.id;
    // Leaving during the credits is a skip of them, recorded like the skip button's.
    if (offer.marker) engine.service.markerSkipped(offer.marker.id, automatic ? 'automatic' : 'manual', engine.state.positionSeconds);
    const view = engine.queue?.getSnapshot()?.view;
    if (automatic && view && engine.queue?.advanceFromCredits) void engine.queue.advanceFromCredits(view.queue.id, view.queue.revision).catch(() => {});
    else engine.next();
  };
  const due = countdown?.automatic && countdown.seconds <= 0;
  useEffect(() => { if (due) leave(true); }); // eslint-disable-line react-hooks/exhaustive-deps
  const episode = next.kind === 'episode';
  const scene = !!offer.marker && followedByScene(offer.marker, state.duration);
  const art = next.stillUrl ?? next.posterUrl;
  const total = Math.max(1, postPlay?.countdownSeconds ?? 10);
  return (
    <div className={s.upNext} role="group" aria-label={t('player.upNext.duringCredits')}>
      <div className={s.upNextRow}>
        <div className={next.stillUrl || !art ? s.upNextStill : s.upNextPoster}><Artwork path={art} shape={next.stillUrl || (!art && episode) ? 'landscape' : 'poster'} icon={episode ? 'tv' : 'film'} alt="" /></div>
        <div className={s.upNextCopy}>
          <Text variant="label" tone="tertiary">{t('player.upNext.duringCredits')}</Text>
          <span className={s.upNextTitle}>{next.title}</span>
          {countdown ? <span className={s.upNextWhen} aria-live="off">{countdown.automatic ? t('player.post.playingIn', {seconds: countdown.seconds}) : t('player.upNext.startsIn', {time: formatClock(countdown.seconds)})}</span> : null}
        </div>
      </div>
      {countdown?.automatic ? <div className={s.upNextMeter} aria-hidden><i style={{width: `${Math.min(100, (elapsed / total) * 100)}%`}} /></div> : null}
      <div className={s.upNextActions}>
        <Button variant="glass" icon="play" label={t('player.upNext.playNow')} onClick={() => leave(false)} />
        {scene && offer.marker ? <Button variant="glass" icon="forward" label={t('player.skip', {kind: offer.marker.kind})} onClick={() => { const marker = offer.marker!; engine.service.markerSkipped(marker.id, 'manual', engine.state.positionSeconds); onDismiss(); engine.seekTo(marker.endSeconds); }} /> : null}
        <Button variant="ghost" size="sm" label={t(offer.marker ? 'player.upNext.watchCredits' : 'player.upNext.notNow')} onClick={onDismiss} />
      </div>
    </div>
  );
}
