import React, {createContext, lazy, Suspense, useCallback, useContext, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {useClient} from '@/data/DataProvider';
import type {PorticoClient} from '@/data/services';
import type {Entry, TrackItem} from '@/data/types';
import type {MenuItem} from '@/ui/Overlay';
import {useToast} from '@/ui/Overlay';
import {usePlayer} from '@/player/PlayerProvider';
import {hrefFor, kindNoun} from './links';
import {useSession} from './session';
import {useSelection} from './selection';

const PlaylistPicker = lazy(() => import('./dialogs/PlaylistPicker'));
const RateDialog = lazy(() => import('./dialogs/RateDialog'));
const DownloadDialog = lazy(() => import('./dialogs/DownloadDialog'));
const DeleteMediaDialog = lazy(() => import('./dialogs/DeleteMediaDialog'));
const MetadataEditor = lazy(() => import('./dialogs/MetadataEditor'));
const FeedbackDialog = lazy(() => import('./dialogs/FeedbackDialog'));

/** Where a card sits, which changes a few actions (Remove from Continue Watching, Not interested). */
export type Origin = 'continue' | 'recommendation' | 'playlist' | 'queue' | undefined;
export type DialogRequest =
  | {kind: 'playlist' | 'collection'; entries: Entry[]}
  | {kind: 'rate'; entry: Entry}
  | {kind: 'download'; entry: Entry}
  | {kind: 'delete'; entries: Entry[]}
  | {kind: 'metadata'; entries: Entry[]; tab?: string}
  | {kind: 'feedback'; entry?: Entry};

type Api = {
  /** Opens an entry: its page, or playback for things without one (channels, songs). */
  open: (e: Entry) => void;
  play: (e: Entry, opts?: {fromStart?: boolean; shuffle?: boolean}) => Promise<void>;
  items: (e: Entry, origin?: Origin) => MenuItem[];
  dialog: (req: DialogRequest) => void;
  toggleWatched: (e: Entry) => Promise<void>;
  toggleWatchlist: (e: Entry) => Promise<void>;
  toggleFavorite: (e: Entry) => Promise<void>;
};
const Ctx = createContext<Api | null>(null);

export const trackToEntry = (t: TrackItem): Entry => ({id: t.id, kind: 'track', title: t.title, subtitle: t.artist, image: t.image, tint: t.tint, shape: 'square', duration: t.duration, parentId: t.albumId, playable: true, libraryId: 'lib-music'});

/** Resolves what playing an entry means: a show plays its next episode, an album its songs, and so on. */
export async function playableQueue(client: PorticoClient, e: Entry): Promise<{queue: Entry[]; index: number; mode?: 'music' | 'book' | 'video'}> {
  switch (e.kind) {
    case 'show': {
      const d = await client.catalog.show(e.id);
      const eps = d.seasons.filter(s => s.number > 0).flatMap(s => s.episodes);
      const next = d.nextUp ?? eps[0];
      return {queue: eps, index: Math.max(0, eps.findIndex(x => x.id === next?.id))};
    }
    case 'season': {
      const d = await client.catalog.show(e.parentId!);
      const s = d.seasons.find(x => x.id === e.id);
      return {queue: s?.episodes ?? [], index: 0};
    }
    case 'episode': {
      const d = await client.catalog.show(e.parentId!);
      const eps = d.seasons.flatMap(s => s.episodes);
      return {queue: eps, index: Math.max(0, eps.findIndex(x => x.id === e.id))};
    }
    case 'album': {
      const d = await client.catalog.album(e.id);
      return {queue: d.tracks.map(trackToEntry), index: 0, mode: 'music'};
    }
    case 'artist': {
      const d = await client.catalog.artist(e.id);
      const albums = await Promise.all(d.albums.slice(0, 4).map(a => client.catalog.album(a.id)));
      return {queue: albums.flatMap(a => a.tracks).map(trackToEntry), index: 0, mode: 'music'};
    }
    case 'track': return {queue: [e], index: 0, mode: 'music'};
    case 'playlist':
    case 'collection':
    case 'view': {
      const d = e.id.startsWith('col-') ? {items: (await client.libraries.entity(e.libraryId ?? 'lib-movies', 'collection', e.id)).entries} : await client.saved.resource(e.kind, e.id);
      const items = d.items.filter(x => x.kind === 'track' || x.kind === 'movie' || x.kind === 'episode' || x.kind === 'homevideo');
      return {queue: items, index: 0, mode: items[0]?.kind === 'track' ? 'music' : 'video'};
    }
    case 'audiobook': return {queue: [e], index: 0, mode: 'book'};
    default: return {queue: [e], index: 0};
  }
}

export function ActionsProvider({children}: {children: React.ReactNode}) {
  const client = useClient();
  const player = usePlayer();
  const navigate = useNavigate();
  const toast = useToast();
  const session = useSession();
  const selection = useSelection();
  const [req, setReq] = useState<DialogRequest | null>(null);

  const play = useCallback<Api['play']>(async (e, opts) => {
    if (e.kind === 'channel') { player.playLive(e.id, e); return; }
    if (e.kind === 'program') { const [ch] = e.id.split('@'); player.playLive(ch!, {...e, id: ch!, kind: 'channel'}); return; }
    if (e.kind === 'photoAlbum') { void navigate({to: `/media/${encodeURIComponent(e.id)}?slideshow=1` as never}); return; }
    const {queue, index, mode} = await playableQueue(client, e);
    if (!queue.length) { toast.show('There’s nothing here to play yet.', {icon: 'info'}); return; }
    let q = queue;
    if (opts?.shuffle) q = [...queue].sort(() => Math.random() - 0.5);
    player.play(q, opts?.shuffle ? 0 : index, {fromStart: opts?.fromStart, mode});
  }, [client, player, navigate, toast]);

  const open = useCallback<Api['open']>(e => {
    const href = hrefFor(e);
    if (href) void navigate({to: href as never});
    else void play(e);
  }, [navigate, play]);

  const toggleWatched = useCallback(async (e: Entry) => {
    const on = !e.watched;
    await client.personal.setWatched([e.id], on);
    toast.show(on ? `Marked “${e.title}” as watched` : `Marked “${e.title}” as unwatched`, {icon: 'check', action: {label: 'Undo', run: () => void client.personal.setWatched([e.id], !on)}});
  }, [client, toast]);
  const toggleWatchlist = useCallback(async (e: Entry) => {
    const on = !client.personal.get(e.id).watchlisted;
    await client.personal.setWatchlist([e.id], on);
    toast.show(on ? `Added “${e.title}” to your Watchlist` : `Removed “${e.title}” from your Watchlist`, {icon: on ? 'savedFilled' : 'saved', action: {label: 'Undo', run: () => void client.personal.setWatchlist([e.id], !on)}});
  }, [client, toast]);
  const toggleFavorite = useCallback(async (e: Entry) => {
    const on = !client.personal.get(e.id).favorite;
    await client.personal.setFavorite([e.id], on);
    toast.show(on ? `Added “${e.title}” to Favorites` : `Removed “${e.title}” from Favorites`, {icon: on ? 'heartFilled' : 'heart', action: {label: 'Undo', run: () => void client.personal.setFavorite([e.id], !on)}});
  }, [client, toast]);

  const items = useCallback<Api['items']>((e, origin) => {
    const p = client.personal.get(e.id);
    const video = ['movie', 'episode', 'homevideo', 'show', 'season'].includes(e.kind);
    const music = ['album', 'artist', 'track', 'playlist'].includes(e.kind);
    const title = ['movie', 'show', 'episode', 'season', 'homevideo', 'audiobook'].includes(e.kind);
    const list: MenuItem[] = [];
    const add = (item: MenuItem) => list.push(item);
    if (e.playable !== false && e.kind !== 'person') {
      const resume = p.progress > 0 && !p.watched;
      add({id: 'play', label: resume ? 'Resume' : e.kind === 'channel' ? 'Watch now' : 'Play', icon: 'play', onSelect: () => void play(e)});
      if (resume) add({id: 'fromStart', label: 'Play from the beginning', icon: 'refresh', onSelect: () => void play(e, {fromStart: true})});
      if (['show', 'album', 'artist', 'playlist', 'collection', 'season'].includes(e.kind)) add({id: 'shuffle', label: 'Shuffle', icon: 'shuffle', onSelect: () => void play(e, {shuffle: true})});
      if (player.active && e.kind !== 'channel') {
        add({id: 'playNext', label: 'Play next', icon: 'queue', onSelect: () => { player.playNext(e); toast.show(`“${e.title}” plays next`, {icon: 'queue'}); }});
        add({id: 'enqueue', label: 'Add to queue', icon: 'plus', onSelect: () => { player.enqueue(e); toast.show(`Added “${e.title}” to the queue`, {icon: 'queue'}); }});
      }
      if (video && e.kind !== 'season' && session.profile?.limits?.together !== false) add({id: 'together', label: 'Watch with a group', icon: 'together', onSelect: () => {
        if (client.together.current()) { void client.together.watch(e.id).then(() => toast.show(`Playing “${e.title}” for the group`, {icon: 'together'})); }
        else void navigate({to: `/together?item=${encodeURIComponent(e.id)}` as never});
      }});
    }
    if (hrefFor(e) && origin !== undefined) add({id: 'details', label: 'Go to details', icon: 'info', onSelect: () => open(e)});
    if (e.kind === 'episode' && e.parentId) add({id: 'show', label: 'Go to show', icon: 'tv', onSelect: () => void navigate({to: `/show/${encodeURIComponent(e.parentId!)}` as never})});
    if (e.kind === 'track' && e.parentId) add({id: 'album', label: 'Go to album', icon: 'music', onSelect: () => void navigate({to: `/media/${encodeURIComponent(e.parentId!)}` as never})});
    // Personal state
    if (title || e.kind === 'album' || e.kind === 'artist') {
      if (video || e.kind === 'audiobook') add({id: 'watched', label: e.watched ? 'Mark as unwatched' : 'Mark as watched', icon: 'watched', separatorBefore: true, onSelect: () => void toggleWatched(e)});
      if (e.kind === 'movie' || e.kind === 'show') add({id: 'watchlist', label: p.watchlisted ? 'Remove from Watchlist' : 'Add to Watchlist', icon: p.watchlisted ? 'savedFilled' : 'saved', onSelect: () => void toggleWatchlist(e)});
      add({id: 'favorite', label: p.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: p.favorite ? 'heartFilled' : 'heart', separatorBefore: !(video || e.kind === 'audiobook'), onSelect: () => void toggleFavorite(e)});
      add({id: 'rate', label: p.rating ? `Your rating: ${p.rating} of 5` : 'Rate…', icon: 'star', onSelect: () => setReq({kind: 'rate', entry: e})});
    }
    if (origin === 'continue') add({id: 'removeContinue', label: 'Remove from Continue Watching', icon: 'close', onSelect: async () => {
      await client.home.removeFromContinue(e.id);
      toast.show(`Removed “${e.title}” from Continue Watching`, {icon: 'check', action: {label: 'Undo', run: () => void client.home.restoreToContinue(e.id)}});
    }});
    if (origin === 'recommendation' || e.recommendation) add({id: 'notInterested', label: 'Not interested', icon: 'eyeOff', onSelect: async () => {
      await client.personal.notInterested(e.id, true);
      toast.show(`You won’t see “${e.title}” in recommendations`, {icon: 'eyeOff', action: {label: 'Undo', run: () => void client.personal.notInterested(e.id, false)}});
    }});
    if (video || music || e.kind === 'audiobook') {
      add({id: 'playlist', label: 'Add to playlist…', icon: 'playlist', separatorBefore: true, onSelect: () => setReq({kind: 'playlist', entries: [e]})});
      if (e.kind === 'movie' || e.kind === 'show' || e.kind === 'episode') add({id: 'collection', label: 'Add to collection…', icon: 'collection', onSelect: () => setReq({kind: 'collection', entries: [e]})});
    }
    if ((e.kind === 'movie' || e.kind === 'episode' || e.kind === 'show' || e.kind === 'album' || e.kind === 'audiobook') && session.profile?.limits?.downloads !== false) {
      add({id: 'download', label: client.downloads.has(e.id) ? 'Downloaded' : 'Download…', icon: 'download', disabled: client.downloads.has(e.id), onSelect: () => setReq({kind: 'download', entry: e})});
    }
    if (hrefFor(e)) add({id: 'copy', label: 'Copy link', icon: 'link', onSelect: () => { void navigator.clipboard?.writeText(location.origin + hrefFor(e)!).catch(() => {}); toast.show('Link copied', {icon: 'link'}); }});
    if (!['person', 'channel', 'program', 'photo'].includes(e.kind)) add({id: 'select', label: 'Select', icon: 'check', onSelect: () => selection.start(e)});
    if (session.isAdmin && (title || e.kind === 'album' || e.kind === 'artist')) {
      add({id: 'edit', label: 'Edit details…', icon: 'edit', separatorBefore: true, onSelect: () => setReq({kind: 'metadata', entries: [e]})});
      add({id: 'match', label: 'Fix match…', icon: 'tag', onSelect: () => setReq({kind: 'metadata', entries: [e], tab: 'match'})});
      add({id: 'refresh', label: 'Refresh details', icon: 'refresh', onSelect: async () => { await client.catalog.refreshMetadata([e.id]); toast.show(`Refreshing details for “${e.title}”`, {icon: 'refresh'}); }});
      add({id: 'delete', label: `Delete ${kindNoun(e)}…`, icon: 'trash', danger: true, onSelect: () => setReq({kind: 'delete', entries: [e]})});
    }
    if (e.kind !== 'person') add({id: 'report', label: 'Report a problem…', icon: 'flag', separatorBefore: !session.isAdmin, onSelect: () => setReq({kind: 'feedback', entry: e})});
    return list;
  }, [client, player, session, toast, navigate, play, open, selection, toggleWatched, toggleWatchlist, toggleFavorite]);

  const api = useMemo<Api>(() => ({open, play, items, dialog: setReq, toggleWatched, toggleWatchlist, toggleFavorite}), [open, play, items, toggleWatched, toggleWatchlist, toggleFavorite]);
  const close = () => setReq(null);
  return (
    <Ctx.Provider value={api}>
      {children}
      <Suspense fallback={null}>
        {req?.kind === 'playlist' || req?.kind === 'collection' ? <PlaylistPicker kind={req.kind} entries={req.entries} onClose={close} /> : null}
        {req?.kind === 'rate' ? <RateDialog entry={req.entry} onClose={close} /> : null}
        {req?.kind === 'download' ? <DownloadDialog entry={req.entry} onClose={close} /> : null}
        {req?.kind === 'delete' ? <DeleteMediaDialog entries={req.entries} onClose={close} /> : null}
        {req?.kind === 'metadata' ? <MetadataEditor entries={req.entries} initialTab={req.tab} onClose={close} /> : null}
        {req?.kind === 'feedback' ? <FeedbackDialog entry={req.entry} onClose={close} /> : null}
      </Suspense>
    </Ctx.Provider>
  );
}
export const useActions = () => useContext(Ctx)!;
