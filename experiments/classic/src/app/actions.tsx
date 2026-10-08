import React, {createContext, useCallback, useContext, useMemo, useState} from 'react';
import {useClient} from '../data/DataProvider';
import type {Entry} from '../data/types';
import {openEntry, push} from '../nav/nav';
import {usePlayer, usePlayerState} from '../player/PlayerProvider';
import {useToast} from './toast';
import {useSession} from './session';
import {ActionMenu, type MenuItem} from '../ui/Sheet';
import {PlaylistPicker, RatingSheet, DownloadSheet, DeleteMediaSheet} from './dialogs';
import {MetadataEditor} from './MetadataEditor';
import {FeedbackSheet} from './FeedbackSheet';
import {useSelection} from './selection';

type Mode = 'menu' | 'playlist' | 'collection' | 'rate' | 'download' | 'delete';
type Api = {
  more: (e: Entry, origin?: 'continue' | 'recommendation') => void;
  open: (e: Entry) => void;
  play: (e: Entry, opts?: {fromStart?: boolean; shuffle?: boolean}) => Promise<void>;
  editMetadata: (entries: Entry[]) => void;
  deleteMedia: (entries: Entry[]) => void;
  addTo: (kind: 'playlist' | 'collection', entries: Entry[]) => void;
  feedback: (e?: Entry) => void;
};
const Ctx = createContext<Api | null>(null);

/**
 * The long-press / More menu for any title, plus the dialogs it opens
 * (playlist and collection pickers, rating, download, metadata editor,
 * delete and feedback). Offers only what the entry's kind supports.
 */
export function ActionsProvider({children}: {children: React.ReactNode}) {
  const client = useClient();
  const player = usePlayer();
  const pstate = usePlayerState();
  const toast = useToast();
  const {isAdmin, profile} = useSession();
  const selection = useSelection();
  const [target, setTarget] = useState<{entry: Entry; origin?: string}>();
  const [mode, setMode] = useState<Mode>();
  const [multi, setMulti] = useState<Entry[]>([]);
  const [editing, setEditing] = useState<Entry[]>();
  const [feedbackFor, setFeedbackFor] = useState<{entry?: Entry} | undefined>();

  const play = useCallback(async (e: Entry, opts?: {fromStart?: boolean; shuffle?: boolean}) => {
    const shuffle = <T,>(a: T[]) => (opts?.shuffle ? [...a].sort(() => Math.random() - 0.5) : a);
    switch (e.kind) {
      case 'show': {
        const d = await client.catalog.show(e.id);
        const eps = d.seasons.filter(s => s.number > 0).flatMap(s => s.episodes);
        const start = d.nextUp ? eps.findIndex(x => x.id === d.nextUp!.id) : 0;
        player.play(opts?.shuffle ? shuffle(eps) : eps, opts?.shuffle ? 0 : Math.max(0, start), {fromStart: opts?.fromStart});
        return;
      }
      case 'episode': {
        const d = await client.catalog.episode(e.id);
        const show = await client.catalog.show(d.show.id);
        const eps = show.seasons.flatMap(s => s.episodes);
        player.play(eps, Math.max(0, eps.findIndex(x => x.id === e.id)), {fromStart: opts?.fromStart});
        return;
      }
      case 'album': {
        const d = await client.catalog.album(e.id);
        player.play(shuffle(d.tracks.map(t => ({id: t.id, kind: 'track' as const, title: t.title, subtitle: t.artist, image: t.image, shape: 'square' as const, duration: t.duration, parentId: t.albumId}))), 0, {mode: 'music'});
        return;
      }
      case 'artist': {
        const d = await client.catalog.artist(e.id);
        player.play(shuffle(d.popular.map(t => ({id: t.id, kind: 'track' as const, title: t.title, subtitle: t.artist, image: t.image, shape: 'square' as const, duration: t.duration, parentId: t.albumId}))), 0, {mode: 'music'});
        return;
      }
      case 'track': {
        const d = await client.catalog.album(e.parentId!);
        const list = d.tracks.map(t => ({id: t.id, kind: 'track' as const, title: t.title, subtitle: t.artist, image: t.image, shape: 'square' as const, duration: t.duration, parentId: t.albumId}));
        player.play(list, Math.max(0, list.findIndex(t => t.id === e.id)), {mode: 'music'});
        return;
      }
      case 'playlist':
      case 'collection':
      case 'view': {
        const d = e.id.startsWith('col-') ? {items: (await client.libraries.entity(e.libraryId ?? 'lib-movies', 'collection', e.id)).entries} : await client.saved.resource(e.kind, e.id);
        const items = d.items.filter(x => x.playable !== false);
        if (items.length) player.play(shuffle(items), 0);
        return;
      }
      case 'channel':
        player.playLive(e.id, e);
        return;
      case 'program':
        if (e.parentId) player.playLive(e.parentId, {...e, id: e.parentId, kind: 'channel'});
        return;
      default:
        player.play([e], 0, {fromStart: opts?.fromStart});
    }
  }, [client, player]);

  const api = useMemo<Api>(() => ({
    more: (entry, origin) => { setTarget({entry, origin}); setMode('menu'); },
    open: (entry: Entry) => {
      if (entry.kind === 'channel' || entry.kind === 'program') { void play(entry); return; }
      if (entry.kind === 'photo') { push('PhotoAlbum', {id: entry.parentId!}); return; }
      if (entry.kind === 'track') { void play(entry); return; }
      openEntry(entry);
    },
    play,
    editMetadata: entries => setEditing(entries),
    deleteMedia: entries => { setMulti(entries); setMode('delete'); },
    addTo: (kind, entries) => { setMulti(entries); setMode(kind); },
    feedback: entry => setFeedbackFor({entry}),
  }), [play]);

  const e = target?.entry;
  const p = e ? client.personal.get(e.id) : undefined;
  const container = e?.kind === 'show' || e?.kind === 'season' || e?.kind === 'album' || e?.kind === 'artist';
  const playable = !!e && e.playable !== false && e.kind !== 'person' && e.kind !== 'photoAlbum';
  const video = !!e && ['movie', 'episode', 'show', 'homevideo'].includes(e.kind);
  const personalKinds = !!e && ['movie', 'show', 'episode', 'album', 'artist', 'audiobook', 'track', 'homevideo'].includes(e.kind);
  const inQueue = pstate.mode !== 'none';
  const together = client.together.current();
  const items: MenuItem[] = e ? [
    {id: 'play', label: p?.progress ? 'Resume' : e.kind === 'show' ? 'Play next episode' : 'Play', icon: 'play', hidden: !playable},
    {id: 'playFromStart', label: 'Play from beginning', icon: 'refresh', hidden: !playable || !p?.progress},
    {id: 'shuffle', label: 'Shuffle', icon: 'shuffle', hidden: !['album', 'artist', 'playlist', 'collection', 'show'].includes(e.kind)},
    {id: 'playNext', label: 'Play next', icon: 'queue', hidden: !inQueue || !playable || container},
    {id: 'addToQueue', label: 'Add to queue', icon: 'list', hidden: !inQueue || !playable || container},
    {id: 'watchWithGroup', label: 'Watch with group', icon: 'together', hidden: !video || !together || profile?.kids, separatorBefore: true},
    {id: 'addToGroupQueue', label: 'Add to group queue', icon: 'queue', hidden: !video || !together},
    {id: 'watchlist', label: p?.watchlisted ? 'Remove from Watchlist' : 'Add to Watchlist', icon: p?.watchlisted ? 'savedFilled' : 'saved', hidden: !['movie', 'show'].includes(e.kind), separatorBefore: true},
    {id: 'favorite', label: p?.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: p?.favorite ? 'heartFilled' : 'heart', hidden: !personalKinds},
    {id: 'watched', label: e.watched ? (e.kind === 'show' ? 'Mark show as unwatched' : 'Mark as unwatched') : e.kind === 'show' ? 'Mark show as watched' : 'Mark as watched', icon: 'watched', hidden: !video},
    {id: 'removeContinue', label: 'Remove from Continue Watching', icon: 'close', hidden: target?.origin !== 'continue'},
    {id: 'notInterested', label: 'Not interested', icon: 'eyeOff', hidden: !e.recommendation && target?.origin !== 'recommendation'},
    {id: 'like', label: p?.reaction === 'like' ? 'Liked' : 'Like', icon: 'thumbUp', hidden: !video, selected: p?.reaction === 'like'},
    {id: 'dislike', label: p?.reaction === 'dislike' ? 'Disliked' : 'Dislike', icon: 'thumbDown', hidden: !video, selected: p?.reaction === 'dislike'},
    {id: 'rate', label: p?.rating ? `Rated ${p.rating} stars…` : 'Rate…', icon: 'star', hidden: !personalKinds},
    {id: 'playlist', label: 'Add to playlist…', icon: 'playlist', hidden: !playable || e.kind === 'channel', separatorBefore: true},
    {id: 'collection', label: 'Add to collection…', icon: 'collection', hidden: !['movie', 'show'].includes(e.kind)},
    {id: 'download', label: e.kind === 'show' ? 'Download all…' : 'Download…', icon: 'download', hidden: !video || profile?.limits?.downloads === false},
    {id: 'select', label: 'Select…', icon: 'check', hidden: !['movie', 'show', 'episode', 'album', 'audiobook'].includes(e.kind)},
    {id: 'details', label: 'Details', icon: 'info', hidden: ['channel', 'program', 'track', 'photo'].includes(e.kind), separatorBefore: true},
    {id: 'goToShow', label: 'Go to show', icon: 'tv', hidden: e.kind !== 'episode'},
    {id: 'goToArtist', label: 'Go to artist', icon: 'user', hidden: e.kind !== 'album' && e.kind !== 'track'},
    {id: 'editMetadata', label: 'Edit metadata…', icon: 'edit', hidden: !isAdmin || !personalKinds, separatorBefore: true},
    {id: 'refreshMetadata', label: 'Refresh metadata', icon: 'refresh', hidden: !isAdmin || !personalKinds},
    {id: 'delete', label: 'Delete…', icon: 'trash', hidden: !isAdmin || !video, destructive: true},
    {id: 'report', label: 'Report a problem', icon: 'flag', hidden: !playable, separatorBefore: !isAdmin},
  ] : [];

  const onSelect = async (id: string) => {
    if (!e) return;
    const close = () => setMode(undefined);
    switch (id) {
      case 'play': close(); await play(e); return;
      case 'playFromStart': close(); await play(e, {fromStart: true}); return;
      case 'shuffle': close(); await play(e, {shuffle: true}); return;
      case 'playNext': player.playNext(e); close(); toast({message: 'Playing next.', icon: 'queue'}); return;
      case 'addToQueue': player.enqueue(e); close(); toast({message: 'Added to the queue.', icon: 'list'}); return;
      case 'watchWithGroup': close(); await client.together.watch(e.id); push('Together'); return;
      case 'addToGroupQueue': close(); await client.together.queueAdd(e.id); toast({message: 'Added to the group’s queue.', icon: 'queue'}); return;
      case 'watchlist': close(); await client.personal.setWatchlist([e.id], !p?.watchlisted); toast({message: p?.watchlisted ? 'Removed from Watchlist.' : 'Added to Watchlist.', icon: 'saved', action: {label: 'Undo', onPress: () => void client.personal.setWatchlist([e.id], !!p?.watchlisted)}}); return;
      case 'favorite': close(); await client.personal.setFavorite([e.id], !p?.favorite); toast({message: p?.favorite ? 'Removed from Favorites.' : 'Added to Favorites.', icon: 'heart'}); return;
      case 'watched': close(); await client.personal.setWatched([e.id], !e.watched); toast({message: e.watched ? 'Marked as unwatched.' : 'Marked as watched.', icon: 'watched'}); return;
      case 'removeContinue': close(); await client.home.removeFromContinue(e.id); toast({message: `Removed “${e.title}” from Continue Watching.`, action: {label: 'Undo', onPress: () => void client.home.restoreToContinue(e.id)}}); return;
      case 'notInterested': close(); await client.personal.notInterested(e.id, true); toast({message: `We won’t recommend “${e.title}”.`, icon: 'eyeOff', action: {label: 'Undo', onPress: () => void client.personal.notInterested(e.id, false)}}); return;
      case 'like': close(); await client.personal.react(e.id, p?.reaction === 'like' ? undefined : 'like'); return;
      case 'dislike': close(); await client.personal.react(e.id, p?.reaction === 'dislike' ? undefined : 'dislike'); return;
      case 'rate': setMode('rate'); return;
      case 'playlist': setMulti([e]); setMode('playlist'); return;
      case 'collection': setMulti([e]); setMode('collection'); return;
      case 'download': setMode('download'); return;
      case 'select': close(); selection.start(e); return;
      case 'details': close(); openEntry(e); return;
      case 'goToShow': close(); push('Show', {id: e.parentId!}); return;
      case 'goToArtist': close(); { const a = e.kind === 'album' ? e.parentId : client.catalog.entry(e.parentId!)?.parentId; if (a) push('Artist', {id: a}); } return;
      case 'editMetadata': close(); setEditing([e]); return;
      case 'refreshMetadata': close(); await client.catalog.refreshMetadata([e.id]); toast({message: 'Metadata refresh queued.', icon: 'refresh'}); return;
      case 'delete': setMulti([e]); setMode('delete'); return;
      case 'report': close(); setFeedbackFor({entry: e}); return;
    }
  };

  return (
    <Ctx.Provider value={api}>
      {children}
      <ActionMenu visible={mode === 'menu' && !!e} onClose={() => setMode(undefined)} title={e?.title} subtitle={e ? [e.subtitle, e.caption].filter(Boolean).join(' · ') : undefined} image={e?.image} items={items} onSelect={id => void onSelect(id)} />
      <PlaylistPicker visible={mode === 'playlist' || mode === 'collection'} kind={mode === 'collection' ? 'collection' : 'playlist'} entries={multi} onClose={() => setMode(undefined)} />
      {e ? <RatingSheet visible={mode === 'rate'} entry={e} onClose={() => setMode(undefined)} /> : null}
      {e ? <DownloadSheet visible={mode === 'download'} entry={e} onClose={() => setMode(undefined)} /> : null}
      <DeleteMediaSheet visible={mode === 'delete'} entries={multi} onClose={() => setMode(undefined)} />
      <MetadataEditor entries={editing} onClose={() => setEditing(undefined)} />
      <FeedbackSheet visible={!!feedbackFor} entry={feedbackFor?.entry} onClose={() => setFeedbackFor(undefined)} />
    </Ctx.Provider>
  );
}

export const useActions = () => useContext(Ctx)!;
