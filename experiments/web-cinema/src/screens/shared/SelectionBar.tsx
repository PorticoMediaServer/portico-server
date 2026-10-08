import {useState} from 'react';
import {useServices} from '../../data/hooks';
import {selection, useSelection} from '../../app/selection';
import {useSession} from '../../app/session';
import {Menu} from '../../ui/Overlay';
import {Icon} from '../../ui/Icon';
import {Spinner, toast} from '../../ui/Feedback';
import {actions} from './actions';
import {plural} from './items';

/**
 * Multi-select: a floating bar with the count, Select all, and bulk actions. Long jobs show
 * progress; partial results say how many worked.
 */
export function SelectionBar() {
  const services = useServices();
  const session = useSession();
  const sel = useSelection();
  const [busy, setBusy] = useState<string | null>(null);
  if (!sel.active) return null;
  const n = sel.ids.length;
  const run = async (label: string, fn: () => Promise<unknown>, done: string) => {
    setBusy(label);
    await fn();
    await new Promise(r => setTimeout(r, 350));
    setBusy(null);
    toast(done, {icon: 'success'});
  };
  const selectAll = async () => {
    const page = await services.catalog.browse({libraryId: sel.scope ?? 'lib-movies', filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, pageSize: 5000});
    selection.setAll(page.items.map(i => i.id));
  };
  const ids = sel.ids;
  return (
    <div className="selbar" role="region" aria-label="Selection">
      <button type="button" className="selbar-done" onClick={() => selection.end()} aria-label="Done selecting"><Icon name="close" size={18} /></button>
      <span className="selbar-count" aria-live="polite">{n ? `${plural(n, 'item')} selected` : 'Select items'}</span>
      <button type="button" className="link-btn" onClick={() => (n ? selection.clear() : void selectAll())}>{n ? 'Clear' : 'Select all'}</button>
      <span className="selbar-spacer" />
      {busy ? <span className="selbar-busy"><Spinner size={16} />{busy}…</span> : (
        <div className="selbar-actions">
          <button type="button" className="btn btn-sm btn-secondary" disabled={!n} onClick={() => void run('Marking watched', () => services.user.set(ids, {watched: true}), `Marked ${plural(n, 'item')} watched`)}><Icon name="check" size={15} /><span className="btn-label">Watched</span></button>
          <button type="button" className="btn btn-sm btn-secondary" disabled={!n} onClick={() => actions.addToPlaylist(ids)}><Icon name="playlist" size={15} /><span className="btn-label">Playlist</span></button>
          <Menu label="More bulk actions" side="top" trigger={<button type="button" className="btn btn-sm btn-secondary" disabled={!n}><Icon name="more" size={15} /><span className="btn-label">More</span></button>} items={[
            {id: 'unwatched', label: 'Mark unwatched', icon: 'eyeOff', onSelect: () => void run('Marking unwatched', () => services.user.set(ids, {watched: false}), `Marked ${plural(n, 'item')} unwatched`)},
            {id: 'wl', label: 'Add to Watchlist', icon: 'bookmark', separatorBefore: true, onSelect: () => void run('Adding to Watchlist', () => services.user.set(ids, {watchlist: true}), `Added ${plural(n, 'item')} to your Watchlist`)},
            {id: 'wlr', label: 'Remove from Watchlist', icon: 'bookmark', onSelect: () => void run('Removing', () => services.user.set(ids, {watchlist: false}), `Removed ${plural(n, 'item')} from your Watchlist`)},
            {id: 'fav', label: 'Add to Favorites', icon: 'heart', onSelect: () => void run('Adding to Favorites', () => services.user.set(ids, {favorite: true}), `Added ${plural(n, 'item')} to Favorites`)},
            {id: 'favr', label: 'Remove from Favorites', icon: 'heart', onSelect: () => void run('Removing', () => services.user.set(ids, {favorite: false}), `Removed ${plural(n, 'item')} from Favorites`)},
            {id: 'col', label: 'Add to collection…', icon: 'layers', separatorBefore: true, onSelect: () => actions.addToCollection(ids)},
            {id: 'dl', label: 'Download…', icon: 'download', onSelect: () => actions.download(ids)},
            ...(session.manager ? [
              {id: 'edit', label: `Edit metadata for ${plural(n, 'item')}…`, icon: 'edit' as const, separatorBefore: true, onSelect: () => actions.editMetadata(ids)},
              {id: 'refresh', label: 'Refresh metadata', icon: 'refresh' as const, onSelect: () => void run('Refreshing metadata', () => Promise.resolve(), `Refreshed ${plural(n, 'item')}; ${Math.max(0, n - 1)} changed`)},
              {id: 'delete', label: 'Delete…', icon: 'trash' as const, danger: true, onSelect: () => actions.remove(ids)},
            ] : []),
          ]} />
        </div>
      )}
    </div>
  );
}
