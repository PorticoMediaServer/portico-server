import {useState} from 'react';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Entry} from '@/data/types';
import {Dialog, useToast} from '@/ui/Overlay';
import {Button} from '@/ui/Button';
import {TextField} from '@/ui/Form';
import {Artwork, EmptyState, Skeleton} from '@/ui/Bits';
import {plural} from '@/app/format';
import s from './dialogs.module.css';

/** Add one or more titles to a playlist or personal collection, or start a new one with them. */
export default function PlaylistPicker({kind, entries, onClose}: {kind: 'playlist' | 'collection'; entries: Entry[]; onClose: () => void}) {
  const client = useClient();
  const toast = useToast();
  const [name, setName] = useState('');
  const [busy, setBusy] = useState<string>();
  const list = useQuery(c => (kind === 'playlist' ? c.saved.playlists() : c.saved.collections()), [kind]);
  const noun = kind === 'playlist' ? 'playlist' : 'collection';
  const what = entries.length === 1 ? `“${entries[0]!.title}”` : plural(entries.length, 'item');
  const ids = entries.map(e => e.id);
  const addTo = async (id: string, title: string) => {
    setBusy(id);
    await (kind === 'playlist' ? client.saved.addToPlaylist(id, ids) : client.saved.addToCollection(id, ids));
    toast.show(`Added ${what} to ${title}`, {icon: 'check'});
    onClose();
  };
  const create = async () => {
    if (!name.trim()) return;
    setBusy('new');
    await (kind === 'playlist' ? client.saved.createPlaylist(name.trim(), ids) : client.saved.createCollection(name.trim(), ids));
    toast.show(`Created ${name.trim()} with ${what}`, {icon: 'check'});
    onClose();
  };
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={`Add to ${noun}`} description={`Choose a ${noun} for ${what}.`} width={480}>
      <form className={s.newRow} onSubmit={e => { e.preventDefault(); void create(); }}>
        <TextField label={`New ${noun}`} placeholder={kind === 'playlist' ? 'Road Trip Mix' : 'Date Night'} value={name} onChange={setName} autoFocus />
        <Button type="submit" variant="primary" label="Create" busy={busy === 'new'} disabled={!name.trim()} />
      </form>
      <div className="t-label" style={{margin: '8px 0 6px'}}>Your {noun}s</div>
      {list.loading && !list.data ? <div className={s.stack}>{[0, 1, 2].map(i => <Skeleton key={i} height={48} />)}</div> : list.data?.length ? (
        <div className={s.list}>
          {list.data.map(r => (
            <button key={r.id} type="button" className={s.row} onClick={() => void addTo(r.id, r.title)} disabled={!!busy}>
              <Artwork src={r.image} title={r.title} shape="square" className={s.thumb} />
              <span className={s.rowText}><span className={s.rowTitle}>{r.title}</span><span className={s.rowSub}>{plural(r.count, 'item')}{r.shared ? ' · Shared' : ''}</span></span>
              {busy === r.id ? <span className="t-caption c-3">Adding…</span> : null}
            </button>
          ))}
        </div>
      ) : <EmptyState compact icon={kind === 'playlist' ? 'playlist' : 'collection'} title={`No ${noun}s yet`} body={`Name one above to create it with ${what}.`} />}
    </Dialog>
  );
}
