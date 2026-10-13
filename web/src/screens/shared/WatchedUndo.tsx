import {useEffect, useState} from 'react';
import type {ContainerWatched} from './container-watched';
import {useI18n} from '../../app/i18n';
import {Inset, Notice} from '../../ui';

/** How long an Undo offer still has, in milliseconds; none when there is no offer or its time cannot be read. */
export function undoRemaining(state: {undoToken?: string; undoUntil?: string} | null, now: number): number {
  if (!state?.undoToken || !state.undoUntil) return 0;
  const until = Date.parse(state.undoUntil);
  return Number.isFinite(until) ? Math.max(0, until - now) : 0;
}

/**
 * After a show, season, album or book is marked watched or unwatched: a quiet line saying so,
 * with Undo for as long as the server offers it. Undo puts every title back as it was, with its
 * place; it is not the same as marking the other way.
 */
export function WatchedUndo({watched, onUndone}: {watched: ContainerWatched; onUndone?: () => void}) {
  const {t} = useI18n();
  const [, tick] = useState(0);
  const left = undoRemaining(watched.state, Date.now());
  useEffect(() => {
    if (!left) return;
    const timer = setTimeout(() => tick(n => n + 1), left + 50);
    return () => clearTimeout(timer);
  }, [left, watched.state?.undoToken]);
  if (!left || !watched.state) return null;
  return <Inset><Notice tone="info" compact action={{label: t('action.undo'), loading: watched.saving, onClick: () => void watched.undo().then(done => { if (done) onUndone?.(); })}}>{t(watched.state.watched ? 'watched.marked' : 'watched.unmarked')}</Notice></Inset>;
}
