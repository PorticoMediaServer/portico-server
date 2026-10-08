import {useState} from 'react';
import {useClient} from '@/data/DataProvider';
import {useSelection} from '@/app/selection';
import {useActions} from '@/app/actions';
import {useSession} from '@/app/session';
import {Button, IconButton} from '@/ui/Button';
import {Menu, useToast} from '@/ui/Overlay';
import {plural} from '@/app/format';
import s from './shell-bits.module.css';

/** Bulk actions for the selected titles: watched, Watchlist, Favorites, playlist, collection, edit, refresh, delete. */
export function SelectionBar() {
  const sel = useSelection();
  const client = useClient();
  const actions = useActions();
  const session = useSession();
  const toast = useToast();
  const [busy, setBusy] = useState<string>();
  if (!sel.active) return null;
  const list = [...sel.selected.values()];
  const ids = list.map(e => e.id);
  const n = list.length;
  const run = async (key: string, fn: () => Promise<void>, done: string) => {
    setBusy(key);
    await fn();
    setBusy(undefined);
    toast.show(done, {icon: 'check'});
    sel.clear();
  };
  const what = plural(n, 'item');
  return (
    <div className={s.selbar} role="toolbar" aria-label="Selection">
      <span className={s.selCount} aria-live="polite">{n ? `${n} selected` : 'Select titles'}</span>
      <div className={s.selActions}>
        <Button size="sm" variant="ghost" icon="watched" label="Watched" disabled={!n} busy={busy === 'w'} onClick={() => void run('w', () => client.personal.setWatched(ids, true), `Marked ${what} as watched`)} />
        <Button size="sm" variant="ghost" icon="saved" label="Watchlist" disabled={!n} busy={busy === 'l'} onClick={() => void run('l', () => client.personal.setWatchlist(ids, true), `Added ${what} to your Watchlist`)} />
        <Button size="sm" variant="ghost" icon="playlist" label="Add to…" disabled={!n} onClick={() => actions.dialog({kind: 'playlist', entries: list})} />
        <Menu label="More bulk actions" trigger={<IconButton size="sm" variant="ghost" icon="more" label="More actions" disabled={!n} />} items={[
          {id: 'unwatched', label: 'Mark as unwatched', icon: 'eyeOff', onSelect: () => void run('u', () => client.personal.setWatched(ids, false), `Marked ${what} as unwatched`)},
          {id: 'fav', label: 'Add to Favorites', icon: 'heart', onSelect: () => void run('f', () => client.personal.setFavorite(ids, true), `Added ${what} to Favorites`)},
          {id: 'unlist', label: 'Remove from Watchlist', icon: 'saved', onSelect: () => void run('r', () => client.personal.setWatchlist(ids, false), `Removed ${what} from your Watchlist`)},
          {id: 'collection', label: 'Add to collection…', icon: 'collection', onSelect: () => actions.dialog({kind: 'collection', entries: list})},
          ...(session.isAdmin ? [
            {id: 'edit', label: 'Edit details…', icon: 'edit' as const, separatorBefore: true, onSelect: () => actions.dialog({kind: 'metadata', entries: list})},
            {id: 'refresh', label: 'Refresh details', icon: 'refresh' as const, onSelect: () => void run('m', () => client.catalog.refreshMetadata(ids), `Refreshing details for ${what}`)},
            {id: 'delete', label: 'Delete…', icon: 'trash' as const, danger: true, onSelect: () => actions.dialog({kind: 'delete', entries: list})},
          ] : []),
        ]} />
      </div>
      <Button size="sm" variant="secondary" label="Done" onClick={sel.clear} />
    </div>
  );
}
