import {useEffect, useState, useSyncExternalStore} from 'react';
import {useI18n} from '../../app/i18n';
import {Toast} from '../../ui';
import {useViewerScope} from '../../app/viewer-scope';
import {PERSONAL_CHANGE} from '../../app/detail';
import {currentWatchedMark, dismissWatchedMark, markRemaining, resetWatchedMark, subscribeWatchedMark, undoWatchedMark} from '../../app/watched-mark';

/**
 * After a show, season, album or book is marked watched or unwatched, from its page or from a
 * card's menu: a toast where the person still is, with Undo for as long as the server offers it
 * (about thirty seconds). Undo is the server's: it puts every title back as it was, with its place;
 * it is not the same as marking the other way. Rendered once, in the shell.
 */
export function WatchedMarkToast() {
  const {t} = useI18n();
  const mark = useSyncExternalStore(subscribeWatchedMark, currentWatchedMark);
  const [, tick] = useState(0);
  const left = markRemaining(mark, Date.now());
  // The offer belongs to the profile that made the mark.
  const scope = useViewerScope();
  useEffect(() => () => resetWatchedMark(), [scope.serverId, scope.viewerId]);
  useEffect(() => {
    if (!mark) return;
    // The offer ends when the server's does; a refusal is said for a few seconds and then goes.
    const timer = setTimeout(() => { if (mark.failed || !markRemaining(mark, Date.now())) dismissWatchedMark(mark); else tick(n => n + 1); }, mark.failed ? 5000 : left + 50);
    return () => clearTimeout(timer);
  }, [mark, left]);
  if (!mark || (!left && !mark.failed)) return null;
  if (mark.failed) return <Toast>{t('watched.undoGone', {title: mark.title})}</Toast>;
  const undo = () => void undoWatchedMark().then(done => { if (done) window.dispatchEvent(new CustomEvent(PERSONAL_CHANGE, {detail: {itemIds: [mark.id], action: 'watched', value: !mark.watched}})); });
  return <Toast action={{label: t('action.undo'), loading: mark.undoing, onClick: undo}}>{t(mark.watched ? 'watched.markedTitle' : 'watched.unmarkedTitle', {title: mark.title})}</Toast>;
}
