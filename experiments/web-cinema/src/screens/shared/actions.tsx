import {Suspense, lazy, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Services} from '../../data/services';
import type {Item, Show} from '../../data/types';
import {player} from '../../app/player';
import {useSession} from '../../app/session';
import {together} from '../../app/together';
import {ConfirmDialog, Dialog, DialogClose, type MenuItem} from '../../ui/Overlay';
import {Button} from '../../ui/Button';
import {ChoiceCards, Field, Input, Switch} from '../../ui/Controls';
import {Spinner, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {cx} from '../../ui/cx';
import {artFor, canWatchlist, isVideo, plural, shapeFor} from './items';
import {startPlayback} from './playback';

/**
 * Global actions: the dialogs any card, page or the selection bar can open (add to playlist or
 * collection, download, rate, report a problem, edit metadata, delete). One host renders them.
 */
type Request =
  | {kind: 'playlist' | 'collection'; ids: string[]}
  | {kind: 'download'; ids: string[]}
  | {kind: 'rate'; id: string}
  | {kind: 'feedback'; id?: string; from?: 'player'}
  | {kind: 'metadata'; ids: string[]; tab?: string}
  | {kind: 'delete'; ids: string[]};

let current: Request | null = null;
const listeners = new Set<() => void>();
const set = (r: Request | null) => { current = r; listeners.forEach(l => l()); };
export const actions = {
  addToPlaylist: (ids: string[]) => set({kind: 'playlist', ids}),
  addToCollection: (ids: string[]) => set({kind: 'collection', ids}),
  download: (ids: string[]) => set({kind: 'download', ids}),
  rate: (id: string) => set({kind: 'rate', id}),
  feedback: (id?: string, from?: 'player') => set({kind: 'feedback', id, from}),
  editMetadata: (ids: string[], tab?: string) => set({kind: 'metadata', ids, tab}),
  remove: (ids: string[]) => set({kind: 'delete', ids}),
  close: () => set(null),
};

const MetadataEditor = lazy(() => import('./MetadataEditor').then(m => ({default: m.MetadataEditor})));
const FeedbackDialog = lazy(() => import('./FeedbackDialog').then(m => ({default: m.FeedbackDialog})));

export function ActionsHost() {
  const r = useSyncExternalStore(l => { listeners.add(l); return () => listeners.delete(l); }, () => current);
  if (!r) return null;
  const close = (o: boolean) => { if (!o) actions.close(); };
  switch (r.kind) {
    case 'playlist': case 'collection': return <AddToListDialog kind={r.kind} ids={r.ids} onOpenChange={close} />;
    case 'download': return <DownloadDialog ids={r.ids} onOpenChange={close} />;
    case 'rate': return <RateDialog id={r.id} onOpenChange={close} />;
    case 'delete': return <DeleteDialog ids={r.ids} onOpenChange={close} />;
    case 'feedback': return <Suspense fallback={null}><FeedbackDialog open itemId={r.id} fromPlayer={r.from === 'player'} onOpenChange={close} /></Suspense>;
    case 'metadata': return <Suspense fallback={null}><MetadataEditor ids={r.ids} initialTab={r.tab} onOpenChange={close} /></Suspense>;
  }
}

// ---- The entry menu: every action on a title, in one consistent order.

export function entryMenu(item: Item, services: Services, opts: {navigate: (to: string) => void; manager: boolean; context?: 'continue' | 'recommendation' | 'detail' | 'card'; showTitle?: string}): MenuItem[] {
  const st = services.user.state(item.id);
  const out: MenuItem[] = [];
  const video = isVideo(item);
  const music = item.kind === 'album' || item.kind === 'artist' || item.kind === 'track';
  const set = (patch: Parameters<Services['user']['set']>[1], msg: string, undo?: Parameters<Services['user']['set']>[1]) => {
    void services.user.set([item.id], patch).then(() => toast(msg, undo ? {action: {label: 'Undo', onClick: () => void services.user.set([item.id], undo)}} : {}));
  };
  if (opts.context !== 'detail') {
    if (st.progress > 0 && !st.watched) {
      out.push({id: 'resume', label: 'Resume', icon: 'play', onSelect: () => startPlayback(services, item, {resume: true})});
      out.push({id: 'restart', label: 'Play from beginning', icon: 'rewind', onSelect: () => startPlayback(services, item, {resume: false})});
    } else {
      out.push({id: 'play', label: 'Play', icon: 'play', onSelect: () => startPlayback(services, item)});
    }
    out.push({id: 'details', label: 'Go to details', icon: 'info', onSelect: () => opts.navigate(`/${item.kind === 'show' ? 'show' : 'media'}/${item.id}`)});
  }
  if (music || item.kind === 'episode' || item.kind === 'movie') {
    out.push({id: 'next', label: 'Play next', icon: 'next', separatorBefore: opts.context !== 'detail', onSelect: () => { player.enqueue(expand(services, item), true); toast(`“${item.title}” plays next`, {icon: 'queue'}); }});
    out.push({id: 'queue', label: 'Add to queue', icon: 'listPlus', onSelect: () => { player.enqueue(expand(services, item)); toast(`Added “${item.title}” to the queue`, {icon: 'queue'}); }});
  }
  if (video) {
    out.push({id: 'group', label: 'Watch with a group', icon: 'people', onSelect: () => { together.join('tg-1'); void services.together.addToQueue('tg-1', item.id); opts.navigate('/together?group=tg-1'); }});
    out.push({id: 'groupQueue', label: 'Add to group queue', icon: 'queue', onSelect: () => void services.together.addToQueue('tg-1', item.id).then(() => toast(`Added to Friday Movie Night’s queue`, {icon: 'people'}))});
  }
  const lib: MenuItem[] = [];
  if (canWatchlist(item)) lib.push(st.watchlist
    ? {id: 'watchlist', label: 'Remove from Watchlist', icon: 'bookmark', onSelect: () => set({watchlist: false}, `Removed from your Watchlist`, {watchlist: true})}
    : {id: 'watchlist', label: 'Add to Watchlist', icon: 'bookmark', onSelect: () => set({watchlist: true}, `Added to your Watchlist`, {watchlist: false})});
  if (video) lib.push(st.watched
    ? {id: 'watched', label: item.kind === 'show' ? 'Mark show unwatched' : 'Mark unwatched', icon: 'eyeOff', onSelect: () => set({watched: false}, 'Marked unwatched', {watched: true})}
    : {id: 'watched', label: item.kind === 'show' ? 'Mark show watched' : 'Mark watched', icon: 'check', onSelect: () => set({watched: true}, 'Marked watched', {watched: false})});
  lib.push(st.favorite
    ? {id: 'favorite', label: 'Remove from Favorites', icon: 'heart', onSelect: () => set({favorite: false}, 'Removed from Favorites', {favorite: true})}
    : {id: 'favorite', label: 'Add to Favorites', icon: 'heart', onSelect: () => set({favorite: true}, 'Added to Favorites', {favorite: false})});
  if (item.kind !== 'artist') lib.push({id: 'rate', label: st.rating ? `Your rating: ${st.rating} of 5` : 'Rate…', icon: 'star', onSelect: () => actions.rate(item.id)});
  if (opts.context === 'continue') lib.push({id: 'continue', label: 'Remove from Continue Watching', icon: 'close', onSelect: () => void services.user.removeFromContinue(item.id).then(() => toast('Removed from Continue Watching', {action: {label: 'Undo', onClick: () => void services.user.restoreContinue(item.id)}}))});
  if (opts.context === 'recommendation' || opts.context === 'detail') {
    if (item.kind === 'movie' || item.kind === 'show') {
      lib.push({id: 'like', label: st.liked === 'like' ? 'Liked' : 'Like', icon: 'like', checked: st.liked === 'like', onSelect: () => set({liked: st.liked === 'like' ? null : 'like'}, st.liked === 'like' ? 'Rating removed' : 'Thanks. You’ll see more like this.')});
      lib.push({id: 'dislike', label: st.liked === 'dislike' ? 'Disliked' : 'Dislike', icon: 'dislike', checked: st.liked === 'dislike', onSelect: () => set({liked: st.liked === 'dislike' ? null : 'dislike'}, st.liked === 'dislike' ? 'Rating removed' : 'Got it. You’ll see less like this.')});
      lib.push({id: 'notInterested', label: 'Not interested', icon: 'eyeOff', onSelect: () => set({notInterested: true}, `“${item.title}” won’t be recommended`, {notInterested: false})});
    }
  }
  if (lib.length) { lib[0] = {...lib[0]!, separatorBefore: true}; out.push(...lib); }
  const lists: MenuItem[] = [];
  if (item.kind !== 'artist' && item.kind !== 'photoAlbum') lists.push({id: 'playlist', label: 'Add to playlist…', icon: 'playlist', onSelect: () => actions.addToPlaylist([item.id])});
  if (item.kind === 'movie' || item.kind === 'show') lists.push({id: 'collection', label: 'Add to collection…', icon: 'layers', onSelect: () => actions.addToCollection([item.id])});
  if (video || item.kind === 'audiobook' || item.kind === 'album') lists.push({id: 'download', label: item.kind === 'show' ? 'Download all…' : 'Download…', icon: 'download', onSelect: () => actions.download([item.id])});
  if (lists.length) { lists[0] = {...lists[0]!, separatorBefore: true}; out.push(...lists); }
  const admin: MenuItem[] = [];
  if (opts.manager) {
    admin.push({id: 'edit', label: 'Edit metadata…', icon: 'edit', onSelect: () => actions.editMetadata([item.id])});
    admin.push({id: 'fix', label: 'Fix match…', icon: 'wand', onSelect: () => actions.editMetadata([item.id], 'matching')});
    admin.push({id: 'refresh', label: 'Refresh metadata', icon: 'refresh', onSelect: () => toast(`Refreshing metadata for “${item.title}”`, {icon: 'refresh'})});
  }
  admin.push({id: 'report', label: 'Report a problem…', icon: 'flag', onSelect: () => actions.feedback(item.id)});
  if (opts.manager && (video || item.kind === 'album' || item.kind === 'audiobook')) admin.push({id: 'delete', label: 'Delete…', icon: 'trash', danger: true, onSelect: () => actions.remove([item.id])});
  admin[0] = {...admin[0]!, separatorBefore: true};
  out.push(...admin);
  return out;
}

/** Albums and artists enqueue their tracks; everything else enqueues itself. */
function expand(services: Services, item: Item): string[] {
  if (item.kind === 'album') return item.trackIds;
  return [item.id];
}

export function useEntryMenu(item: Item | undefined, context?: 'continue' | 'recommendation' | 'detail' | 'card') {
  const services = useServices();
  const navigate = useNavigate();
  const session = useSession();
  return item ? entryMenu(item, services, {navigate: to => void navigate({to: to as never}), manager: session.manager, context}) : [];
}

// ---- Dialogs

function AddToListDialog({kind, ids, onOpenChange}: {kind: 'playlist' | 'collection'; ids: string[]; onOpenChange: (o: boolean) => void}) {
  const services = useServices();
  const playlists = useQuery(kind === 'playlist' ? 'lists.playlists' : null, () => services.lists.playlists());
  const collections = useQuery(kind === 'collection' ? 'lists.collections' : null, () => services.catalog.collections());
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [shared, setShared] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const lists = (kind === 'playlist' ? playlists.data : collections.data?.filter(c => c.owner === 'me' || c.shared)) ?? [];
  const loading = kind === 'playlist' ? playlists.loading : collections.loading;
  const noun = kind === 'playlist' ? 'playlist' : 'collection';
  const what = ids.length === 1 ? 'it' : plural(ids.length, 'item');
  const add = async (id: string, title: string) => {
    setBusy(id);
    try {
      if (kind === 'playlist') await services.lists.addToPlaylist(id, ids);
      else await services.lists.addToCollection(id, ids);
      toast(ids.length === 1 ? `Added to “${title}”` : `Added ${plural(ids.length, 'item')} to “${title}”`, {icon: kind === 'playlist' ? 'playlist' : 'layers'});
      onOpenChange(false);
    } finally { setBusy(null); }
  };
  const create = async () => {
    if (!name.trim()) return;
    setBusy('new');
    const created = kind === 'playlist' ? await services.lists.createPlaylist(name.trim(), '', shared, ids) : await services.lists.createCollection(name.trim(), '', shared, ids);
    toast(`Created “${created.title}” with ${what}`, {icon: 'success'});
    onOpenChange(false);
  };
  return (
    <Dialog open onOpenChange={onOpenChange} title={kind === 'playlist' ? 'Add to playlist' : 'Add to collection'} description={ids.length > 1 ? `${plural(ids.length, 'item')} selected` : undefined} size="sm">
      {creating ? (
        <form onSubmit={e => { e.preventDefault(); void create(); }} className="stack">
          <Field label={`New ${noun} name`}><Input value={name} onChange={e => setName(e.target.value)} autoFocus maxLength={80} placeholder={kind === 'playlist' ? 'Road trip' : 'Rainy Sundays'} /></Field>
          <label className="row" style={{padding: 0, minHeight: 44, border: 0}}>
            <span className="row-text"><span className="row-label">Share with everyone on this server</span><span className="row-help">Other profiles can see and play it.</span></span>
            <Switch checked={shared} onChange={setShared} label="Share with everyone on this server" />
          </label>
          <div className="hstack" style={{justifyContent: 'flex-end'}}>
            <Button variant="ghost" label="Back" onClick={() => setCreating(false)} />
            <Button variant="primary" label={`Create ${noun}`} disabled={!name.trim()} busy={busy === 'new'} onClick={() => void create()} />
          </div>
        </form>
      ) : (
        <div className="pick-list">
          <button type="button" className="pick-row pick-new" onClick={() => setCreating(true)}>
            <span className="pick-thumb is-new"><Icon name="plus" size={22} /></span>
            <span className="pick-text"><strong>New {noun}</strong><span>Start a {noun} with {what}</span></span>
          </button>
          {loading && !lists.length ? <div className="pick-loading"><Spinner /></div> : null}
          {!loading && !lists.length ? <p className="faint" style={{padding: '12px 4px'}}>You don’t have any {noun}s yet.</p> : null}
          {lists.map(l => (
            <button key={l.id} type="button" className="pick-row" onClick={() => void add(l.id, l.title)} disabled={!!busy}>
              <span className="pick-thumb" style={{background: l.tint}}><Icon name={kind === 'playlist' ? 'playlist' : 'layers'} size={20} /></span>
              <span className="pick-text"><strong>{l.title}</strong><span>{plural(l.itemIds.length, 'item')}{l.shared ? ' · Shared' : ''}</span></span>
              {busy === l.id ? <Spinner size={18} /> : <Icon name="plus" size={18} className="pick-add" />}
            </button>
          ))}
        </div>
      )}
    </Dialog>
  );
}

const QUALITIES = [
  {value: 'Original', title: 'Original', body: 'Best picture, largest files. About 4–12 GB a film.', icon: 'sparkles' as const},
  {value: '1080p', title: 'High · 1080p', body: 'Sharp on any screen. About 2–4 GB a film.', icon: 'monitor' as const},
  {value: '720p', title: 'Standard · 720p', body: 'Good for phones and tablets. About 1–2 GB a film.', icon: 'tablet' as const},
  {value: '480p', title: 'Data saver · 480p', body: 'Smallest files for long trips. Under 1 GB a film.', icon: 'phone' as const},
];

function DownloadDialog({ids, onOpenChange}: {ids: string[]; onOpenChange: (o: boolean) => void}) {
  const services = useServices();
  const navigate = useNavigate();
  const items = useQuery(`items.${ids.join(',')}`, () => services.catalog.items(ids)).data ?? [];
  const settings = useQuery('downloads.settings', () => services.downloads.settings()).data;
  const [quality, setQuality] = useState<string>(settings?.quality ?? '1080p');
  const [policy, setPolicy] = useState<'all' | 'unwatched' | 'next'>('unwatched');
  const [busy, setBusy] = useState(false);
  const show = items.length === 1 && items[0]!.kind === 'show' ? (items[0] as Show) : null;
  const title = show ? `Download ${show.title}` : items.length === 1 ? `Download “${items[0]!.title}”` : `Download ${plural(ids.length, 'item')}`;
  return (
    <Dialog open onOpenChange={onOpenChange} title={title} description="Your server prepares a copy; it downloads to this browser’s storage when ready." size="md" footer={
      <>
        <DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose>
        <Button variant="primary" icon="download" label="Download" busy={busy} onClick={async () => {
          setBusy(true);
          if (show) await services.downloads.addShow(show.id, policy, 3, quality);
          else await services.downloads.add(ids, quality);
          toast(show ? `Downloading ${show.title} (${policy === 'all' ? 'every episode' : policy === 'unwatched' ? 'unwatched episodes' : 'the next 3 episodes'})` : `Downloading ${ids.length === 1 ? `“${items[0]?.title}”` : plural(ids.length, 'item')}`, {icon: 'download', action: {label: 'View', onClick: () => void navigate({to: '/downloads'})}});
          onOpenChange(false);
        }} />
      </>
    }>
      {show ? (
        <div style={{marginBottom: 20}}>
          <div className="field-label" style={{marginBottom: 8}}>Which episodes</div>
          <ChoiceCards label="Which episodes" columns={1} value={policy} onChange={setPolicy} options={[
            {value: 'unwatched', title: 'Unwatched episodes', body: 'New episodes download as they arrive.'},
            {value: 'next', title: 'Keep the next 3 ready', body: 'Watched ones are removed and the next ones fetched.'},
            {value: 'all', title: 'Every episode', body: `All ${show.episodeCount} episodes.`},
          ]} />
        </div>
      ) : null}
      <div className="field-label" style={{marginBottom: 8}}>Quality</div>
      <ChoiceCards label="Quality" columns={2} value={quality} onChange={setQuality} options={QUALITIES} />
    </Dialog>
  );
}

function RateDialog({id, onOpenChange}: {id: string; onOpenChange: (o: boolean) => void}) {
  const services = useServices();
  const item = useQuery(`item.${id}`, () => services.catalog.item(id)).data;
  const st = services.user.state(id);
  const [value, setValue] = useState(st.rating);
  const [hover, setHover] = useState(0);
  const shown = hover || value;
  const words = ['', 'Not for me', 'It was OK', 'Liked it', 'Really liked it', 'Loved it'];
  return (
    <Dialog open onOpenChange={onOpenChange} title={item ? `Rate “${item.title}”` : 'Rate'} description="Ratings are private to your profile and tune your recommendations." size="sm" footer={
      <>
        {st.rating ? <Button variant="ghost" label="Clear rating" onClick={() => void services.user.set([id], {rating: 0}).then(() => { toast('Rating cleared'); onOpenChange(false); })} /> : null}
        <span style={{flex: 1}} />
        <Button variant="primary" label="Save" disabled={!value} onClick={() => void services.user.set([id], {rating: value}).then(() => { toast(`Rated ${value} of 5`, {icon: 'star'}); onOpenChange(false); })} />
      </>
    }>
      <div className="rate">
        {item ? <Art src={artFor(item, shapeFor(item))} tint={item.tint} className="rate-art" ratio={shapeFor(item) === 'poster' ? '2/3' : '1/1'} /> : null}
        <div className="rate-stars" role="radiogroup" aria-label="Rating" onMouseLeave={() => setHover(0)}>
          {[1, 2, 3, 4, 5].map(n => (
            <button key={n} type="button" role="radio" aria-checked={value === n} aria-label={`${n} of 5: ${words[n]}`} className={cx('rate-star', n <= shown && 'is-on')} onMouseEnter={() => setHover(n)} onClick={() => setValue(n)}>
              <Icon name="star" size={34} fill={n <= shown ? 'currentColor' : 'none'} />
            </button>
          ))}
        </div>
        <p className="rate-word" aria-live="polite">{shown ? words[shown] : 'Choose 1 to 5 stars'}</p>
      </div>
    </Dialog>
  );
}

function DeleteDialog({ids, onOpenChange}: {ids: string[]; onOpenChange: (o: boolean) => void}) {
  const services = useServices();
  const navigate = useNavigate();
  const items = useQuery(`items.${ids.join(',')}`, () => services.catalog.items(ids)).data ?? [];
  const [files, setFiles] = useState(true);
  const one = items.length === 1 ? items[0] : undefined;
  return (
    <ConfirmDialog open onOpenChange={onOpenChange} danger title={one ? `Delete “${one.title}”?` : `Delete ${plural(ids.length, 'item')}?`} confirmLabel="Delete" body={
      <>
        <p>{one ? 'It' : 'They'} will be removed from the library for everyone on this server. Watch history and ratings are kept in case {one ? 'it comes' : 'they come'} back.</p>
        <p>Deleted media stays in Server › Maintenance › Deleted titles for 30 days, where you can put it back.</p>
      </>
    } onConfirm={async () => {
      await new Promise(r => setTimeout(r, 400));
      toast(one ? `Deleted “${one.title}”` : `Deleted ${plural(ids.length, 'item')}`, {icon: 'trash', action: {label: 'Undo', onClick: () => toast('Put back in the library')}});
      if (one && location.pathname.includes(one.id)) void navigate({to: '/'});
    }}>
      <div style={{marginTop: 14}}>
        <label className="row" style={{padding: '8px 0', minHeight: 0, border: 0}}>
          <span className="row-text"><span className="row-label">Also delete the files from disk</span><span className="row-help">{one && 'versions' in one ? `${one.versions.length} ${one.versions.length === 1 ? 'file' : 'files'} · ${one.versions.reduce((a, v) => a + v.sizeGB, 0).toFixed(1)} GB` : 'Frees the space on your server'}</span></span>
          <Switch checked={files} onChange={setFiles} label="Also delete the files from disk" />
        </label>
      </div>
    </ConfirmDialog>
  );
}

