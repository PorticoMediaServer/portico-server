import React, {useEffect, useState, useSyncExternalStore} from 'react';
import {Button, Text} from '../ui';
import {useI18n} from '../app/i18n';
import type {PlayerEngine} from './engine';
import type {SkipMode} from './SkipPrompt';
import {upNextOffer, type ClosingMarker} from './up-next';
import s from './Player.module.css';

/**
 * The next title, offered in a corner while the credits run. "Play now" goes straight to it;
 * "Watch credits" puts the card away for this title. Nothing here plays anything by itself: the
 * countdown and the passout check stay with the card shown when the title has ended.
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
  return {offer: shown, next, dismiss: () => { if (offer) setDismissed(offer.id); }};
}

export function UpNextCredits({engine, offer, next, onDismiss}: {engine: PlayerEngine; offer: {marker?: ClosingMarker}; next: {title: string}; onDismiss: () => void}) {
  const {t} = useI18n();
  const play = () => {
    // Leaving during the credits is a skip of them, recorded like the skip button's.
    if (offer.marker) engine.service.markerSkipped(offer.marker.id, 'manual', engine.state.positionSeconds);
    engine.next();
  };
  return (
    <div className={s.upNext} role="group" aria-label={t('player.upNext.duringCredits')}>
      <Text variant="label" tone="tertiary">{t('player.upNext.duringCredits')}</Text>
      <span className={s.upNextTitle}>{next.title}</span>
      <div className={s.upNextActions}>
        <Button variant="glass" icon="play" label={t('player.upNext.playNow')} onClick={play} />
        <Button variant="ghost" size="sm" label={t(offer.marker ? 'player.upNext.watchCredits' : 'player.upNext.notNow')} onClick={onDismiss} />
      </div>
    </div>
  );
}
