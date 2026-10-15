import {useEffect, useState} from 'react';
import {useI18n} from '../../app/i18n';
import {Toast} from '../../ui';

/** An entry just taken out of a playlist or a collection, with the server's undo token and when it lapses. */
export type Removal = Readonly<{token: string; until: string; title: string; from: 'playlist' | 'collection'}>;

/** How long the offer shows: until the server's undo lapses, within a sensible range whatever the clocks say. */
function offerFor(removal: Removal): number {
  const end = Date.parse(removal.until);
  return Number.isFinite(end) ? Math.min(30_000, Math.max(8_000, end - Date.now())) : 8_000;
}

/**
 * After an entry is removed from a playlist or a collection: a toast with Undo for as long as the
 * server offers it. Undo is the server's: the same entry goes back where it was, not a new copy at
 * the end. A refusal (too late, or the list is gone) is said for a few seconds and then goes.
 */
export function RemovalUndoToast({removal, onUndo, onClose}: {removal: Removal | null; onUndo: (removal: Removal) => Promise<boolean>; onClose: () => void}) {
  const {t} = useI18n();
  const [state, setState] = useState<'offer' | 'undoing' | 'failed'>('offer');
  useEffect(() => setState('offer'), [removal]);
  useEffect(() => {
    if (!removal || state === 'undoing') return;
    const timer = setTimeout(onClose, state === 'failed' ? 5000 : offerFor(removal));
    return () => clearTimeout(timer);
  }, [removal, state, onClose]);
  if (!removal) return null;
  if (state === 'failed') return <Toast>{t('saved.removal.undoFailed', {title: removal.title})}</Toast>;
  const undo = () => {
    setState('undoing');
    void onUndo(removal).then(done => (done ? onClose() : setState('failed')), () => setState('failed'));
  };
  return <Toast action={{label: t('action.undo'), loading: state === 'undoing', onClick: undo}}>{t(removal.from === 'playlist' ? 'saved.removal.fromPlaylist' : 'saved.removal.fromCollection', {title: removal.title})}</Toast>;
}
