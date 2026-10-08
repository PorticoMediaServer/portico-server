import React, {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {Artwork, Button, Icon, Text} from '../ui';
import {useDetail} from '../app/detail';
import {useI18n} from '../app/i18n';
import type {PlayerEngine} from './engine';
import s from './Player.module.css';

/**
 * The end-of-item card for video. Three states, all driven by the queue
 * player's held completion: Up Next with a countdown, Still watching? when
 * the server asks for a passout check, and Replay/Back when nothing follows.
 * The countdown length comes from the server's post-play policy when present
 * and falls back to ten seconds.
 */
export function PostPlay({engine, onClose}: {engine: PlayerEngine; onClose: () => void}) {
  const {queue, state, service} = engine;
  const queueState = useSyncExternalStore(queue?.subscribe ?? (() => () => {}), () => queue?.getSnapshot() ?? null);
  const held = queueState?.completion ?? null;
  const next = held?.nextEntryId ? queueState?.view?.items.find(i => i.entryId === held.nextEntryId) : undefined;
  const countdownSeconds = held?.postPlay.countdownSeconds ?? 10;
  const autoplay = held?.postPlay.autoplay ?? true;
  const passoutDue = held?.postPlay.passoutCheckDue ?? false;
  const [remaining, setRemaining] = useState<number>(countdownSeconds);
  const counting = !!held?.nextAvailable && autoplay && !passoutDue;
  // The engine object is rebuilt on every playback fact; the timer must not restart with it.
  const advance = useRef(engine.advance);
  advance.current = engine.advance;
  useEffect(() => {
    setRemaining(countdownSeconds);
    if (!counting) return;
    const started = Date.now();
    const id = setInterval(() => {
      const left = countdownSeconds - Math.floor((Date.now() - started) / 1000);
      setRemaining(Math.max(0, left));
      if (left <= 0) {
        clearInterval(id);
        void advance.current('automatic').catch(() => {});
      }
    }, 250);
    return () => clearInterval(id);
  }, [counting, countdownSeconds, held?.sessionId]);
  if (state.phase !== 'ended') return null;
  const replay = () => {
    queue?.releaseCompletion();
    engine.seekTo(0);
    service.resume();
  };
  if (held?.nextAvailable && next) {
    return (
      <div className={s.postPlay} role="dialog" aria-label="Up next">
        <div className={s.postCard}>
          <Text variant="label" tone="tertiary">{passoutDue ? 'Still watching?' : 'Up next'}</Text>
          <div className={s.postNext}>
            <div className={s.postArt}><Artwork path={undefined} shape={next.kind === 'episode' ? 'landscape' : 'poster'} icon={next.kind === 'episode' ? 'tv' : 'film'} alt="" /></div>
            <div className={s.postCopy}>
              <span className={s.postTitle}>{next.title}</span>
              <span className={s.postSub}>{passoutDue ? 'Playback paused after several episodes. Confirm to keep going.' : counting ? `Playing in ${remaining}s` : 'Ready when you are'}</span>
            </div>
          </div>
          <div className={s.postActions}>
            <Button variant="primary" icon="play" label={passoutDue ? "I'm still watching" : 'Play now'} onClick={() => void engine.advance(passoutDue ? 'still-watching' : 'manual')} />
            {counting ? <Button variant="outline" label="Cancel" onClick={() => queue?.releaseCompletion()} /> : null}
            <Button variant="ghost" icon="refresh" label="Replay" onClick={replay} />
            <Button variant="ghost" label="Back" onClick={onClose} />
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className={s.postPlay} role="dialog" aria-label="Finished">
      <div className={s.postCard}>
        <Text variant="label" tone="tertiary">Finished</Text>
        <span className={s.postTitle}>{engine.identity.title}</span>
        {held && !held.nextAvailable && held.reason === 'unavailable' ? <span className={s.postSub}>The next item in the queue is unavailable.</span> : null}
        <div className={s.postActions}>
          <Button variant="primary" icon="refresh" label="Replay" onClick={replay} />
          <Button variant="outline" label="Back" onClick={onClose} />
        </div>
        {engine.identity.kind === 'movie' && engine.identity.itemId && engine.identity.libraryId ? <FinishedMore itemId={engine.identity.itemId} libraryId={engine.identity.libraryId} title={engine.identity.title} onLeave={onClose} /> : null}
      </div>
    </div>
  );
}

/**
 * WEB-PLAYER-05: after a movie with nothing queued, three related titles (the server's first
 * related row) and a quick star rating. Read from the title's detail projection.
 */
function FinishedMore({itemId, libraryId, title, onLeave}: {itemId: string; libraryId: string; title: string; onLeave: () => void}) {
  const {t} = useI18n();
  const navigate = useNavigate();
  const target = useMemo(() => ({itemId, libraryId}), [itemId, libraryId]);
  const {snapshot, mutate} = useDetail(target);
  const data = snapshot.data;
  const related = data?.related?.rows[0]?.entries.slice(0, 3) ?? [];
  const rating = data?.personal?.rating ?? null;
  const canRate = !!data?.actions.some(a => a.enabled && a.id === 'rating');
  const [hover, setHover] = useState<number | null>(null);
  const shown = hover ?? rating ?? 0;
  const open = (id: string) => { onLeave(); void navigate({to: '/media/$itemId', params: {itemId: id}}); };
  if (!data || (!related.length && !canRate)) return null;
  return (
    <>
      {canRate ? (
        <div className={s.postRate}>
          <Text variant="label" tone="tertiary">{rating != null ? t('player.rated', {stars: t('title.stars', {count: rating})}) : t('player.rateThis')}</Text>
          <div className={s.postStars} onMouseLeave={() => setHover(null)} role="radiogroup" aria-label={t('player.rateThis')}>
            {[1, 2, 3, 4, 5].map(star => (
              <button key={star} type="button" role="radio" aria-checked={rating === star} aria-label={t('title.stars', {count: star})} onMouseEnter={() => setHover(star)} onClick={() => mutate({action: 'rating', value: star})} style={{color: shown >= star ? 'var(--accent)' : 'var(--text-tertiary)'}}>
                <Icon name={shown >= star ? 'starFilled' : 'star'} size={24} />
              </button>
            ))}
          </div>
        </div>
      ) : null}
      {related.length ? (
        <div className={s.postRelated}>
          <Text variant="label" tone="tertiary">{t('player.moreLike', {title})}</Text>
          <div className={s.postRelatedRow}>
            {related.map(e => (
              <button key={e.id} type="button" className={s.postRelatedItem} onClick={() => open(e.id)} aria-label={e.title}>
                <Artwork path={e.posterUrl} shape="poster" icon="film" alt="" />
                <span>{e.title}</span>
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </>
  );
}

/** WEB-PLAYER-04: the seek rows say the viewer's own skip lengths. */
const shortcutRows = (back: number, forward: number): [string, string][] => [['Space or K', 'Play or pause'], ['← or J', `Back ${back} seconds`], ['→ or L', `Forward ${forward} seconds`], ['M', 'Mute'], ['F', 'Fullscreen'], ['Esc', 'Close a menu, leave fullscreen, or close the player'], ['?', 'Show this list']];

export function ShortcutLegend({skipBack = 10, skipForward = 30}: {skipBack?: number; skipForward?: number}) {
  const shortcuts = shortcutRows(skipBack, skipForward);
  return (
    <dl className={s.shortcuts}>
      {shortcuts.map(([keys, what]) => (
        <React.Fragment key={keys}>
          <dt><kbd>{keys}</kbd></dt>
          <dd>{what}</dd>
        </React.Fragment>
      ))}
    </dl>
  );
}
