import React, {useState} from 'react';
import {Platform, View} from 'react-native';
import type {Services} from '../../data/services';
import type {Item} from '../../data/types';
import {useQuery, useServices} from '../../data/hooks';
import {Button} from '../../design/Button';
import {ListRow, Segmented, TextField, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui, type MenuAction} from '../../design/overlays';
import {Text} from '../../design/Text';
import {colors, isTV, u} from '../../design/tokens';
import {go, push} from '../../nav/routes';
import {player} from '../../state/player';
import {playItem} from './cards';
import {plural} from '../../design/format';

export type Origin = 'recommendation' | 'continue' | 'default';

/**
 * The More menu for any title: the same actions the web's EntryActions offers, grouped the same way
 * (play, lists, Watch Together, manage). Long-press a card or use the ⋯ button on a detail page.
 */
export function openItemMenu(s: Services, item: Item, opts: {origin?: Origin; manager?: boolean; onSelect?: () => void} = {}) {
  const st = s.user.state(item.id);
  const video = item.kind === 'movie' || item.kind === 'episode' || item.kind === 'show' || item.kind === 'season' || item.kind === 'homevideo';
  const music = item.kind === 'album' || item.kind === 'track' || item.kind === 'artist';
  const container = item.kind === 'show' || item.kind === 'season';
  const actions: MenuAction[] = [];
  if (item.kind !== 'show' && item.kind !== 'season' && item.kind !== 'artist' && item.kind !== 'photoAlbum') {
    if (st.progress > 0 && !st.watched) {
      actions.push({label: 'Resume', icon: 'play', onPress: () => playItem(item, {from: st.position})});
      actions.push({label: 'Play from beginning', icon: 'skipBack', onPress: () => playItem(item, {from: 0})});
    } else actions.push({label: 'Play', icon: 'play', onPress: () => playItem(item)});
  }
  if (music || item.kind === 'episode' || item.kind === 'movie') {
    actions.push({label: 'Play next', icon: 'listMusic', onPress: () => { player.enqueue(item.kind === 'album' ? item.trackIds : [item.id], true); ui.toast('Playing next.'); }});
    actions.push({label: 'Add to queue', icon: 'queue', onPress: () => { player.enqueue(item.kind === 'album' ? item.trackIds : [item.id]); ui.toast('Added to the queue.'); }});
  }
  if (video && item.kind !== 'homevideo') {
    actions.push({label: 'Watch with group', icon: 'together', detail: 'Watch Together', onPress: () => go('Together')});
    actions.push({label: 'Add to group queue', icon: 'group', onPress: () => s.together.addToQueue('tg-1', item.id).then(() => ui.toast('Added to the group’s queue.'))});
  }
  if (video || item.kind === 'audiobook') {
    actions.push({label: st.watchlist ? 'Remove from Watchlist' : 'Add to Watchlist', icon: st.watchlist ? 'bookmarkCheck' : 'bookmark', onPress: () => s.user.set([item.id], {watchlist: !st.watchlist}).then(() => ui.toast(st.watchlist ? 'Removed from your Watchlist.' : 'Added to your Watchlist.'))});
  }
  actions.push({label: st.favorite ? 'Remove from Favorites' : 'Add to Favorites', icon: 'heart', onPress: () => s.user.set([item.id], {favorite: !st.favorite}).then(() => ui.toast(st.favorite ? 'Removed from Favorites.' : 'Added to Favorites.'))});
  if (video) {
    if (container) {
      const label = item.kind === 'show' ? 'show' : 'season';
      actions.push({label: `Mark ${label} as watched`, icon: 'checkCircle', onPress: () => s.user.markContainerWatched(item.id, true).then(n => ui.toast(`${plural(n, 'episode', 'episodes')} marked as watched.`))});
      actions.push({label: `Mark ${label} as unwatched`, icon: 'eyeOff', onPress: () => s.user.markContainerWatched(item.id, false).then(n => ui.toast(`${plural(n, 'episode', 'episodes')} marked as unwatched.`))});
    } else {
      actions.push({label: st.watched ? 'Mark as unwatched' : 'Mark as watched', icon: st.watched ? 'eyeOff' : 'checkCircle', onPress: () => s.user.set([item.id], {watched: !st.watched})});
    }
  }
  if (opts.origin === 'continue') {
    actions.push({label: 'Remove from Continue Watching', icon: 'x', onPress: () => s.user.removeFromContinue(item.id).then(() => ui.toast(`Removed “${item.title}” from Continue Watching.`, {action: {label: 'Undo', onPress: () => s.user.restoreContinue(item.id)}}))});
  }
  actions.push({label: st.liked === 'like' ? 'Liked' : 'Like', icon: 'thumbsUp', checked: st.liked === 'like', onPress: () => s.user.set([item.id], {liked: st.liked === 'like' ? null : 'like'})});
  actions.push({label: st.liked === 'dislike' ? 'Disliked' : 'Dislike', icon: 'thumbsDown', checked: st.liked === 'dislike', onPress: () => s.user.set([item.id], {liked: st.liked === 'dislike' ? null : 'dislike'})});
  if (video || item.kind === 'audiobook' || item.kind === 'album') actions.push({label: st.rating ? `Rated ${st.rating} ★ · Change…` : 'Rate…', icon: 'star', onPress: () => openRateSheet(s, item)});
  if (opts.origin === 'recommendation') {
    actions.push({label: 'Not interested', icon: 'ban', detail: 'Stop recommending this title', onPress: () => s.user.set([item.id], {notInterested: true}).then(() => ui.toast(`We won’t recommend “${item.title}”.`, {action: {label: 'Undo', onPress: () => s.user.set([item.id], {notInterested: false})}}))});
  }
  actions.push({label: 'Add to playlist…', icon: 'listMusic', onPress: () => openAddToPlaylist(s, [item.id])});
  if (!music && item.kind !== 'episode') actions.push({label: 'Add to collection…', icon: 'layers', onPress: () => openAddToCollection(s, [item.id])});
  if (!isTV && (video || item.kind === 'audiobook' || item.kind === 'album')) actions.push({label: container ? 'Download all…' : 'Download…', icon: 'download', onPress: () => openDownloadSheet(s, item)});
  if (item.kind === 'episode') actions.push({label: 'Go to show', icon: 'shows', onPress: () => push('Show', {id: item.showId})});
  if (item.kind === 'album' || item.kind === 'track') actions.push({label: 'Go to artist', icon: 'mic', onPress: () => push('Artist', {id: item.artistId})});
  if (opts.onSelect) actions.push({label: 'Select', icon: 'checkCircle', detail: 'Choose several titles for bulk actions', onPress: opts.onSelect});
  if (opts.manager) {
    actions.push({label: 'Edit metadata…', icon: 'edit', detail: 'Fix match, artwork, tags and credits', onPress: () => push('MetadataEditor', {id: item.id})});
    actions.push({label: 'Refresh metadata', icon: 'refresh', onPress: () => ui.toast('Metadata refresh queued.')});
    actions.push({label: 'Delete…', icon: 'trash', destructive: true, onPress: () => confirmDelete(item)});
  }
  actions.push({label: 'Report a problem', icon: 'flag', onPress: () => go('Feedback', {itemId: item.id})});
  ui.menu({title: item.title, subtitle: item.kind === 'episode' ? 'Episode' : undefined, actions});
}

export function confirmDelete(item: Item) {
  ui.confirm({
    title: `Delete “${item.title}”?`,
    body: 'The file moves to Deleted titles on the server and is removed for good after 30 days. Anyone watching it is stopped.',
    confirmLabel: 'Delete',
    destructive: true,
    typeToConfirm: 'DELETE',
    onConfirm: () => ui.toast(`“${item.title}” moved to Deleted titles.`),
  });
}

export function openRateSheet(s: Services, item: Item) {
  ui.sheet({title: `Rate ${item.title}`, render: close => <RateBody s={s} item={item} close={close} />});
}

function RateBody({s, item, close}: {s: Services; item: Item; close: () => void}) {
  const current = s.user.state(item.id).rating;
  return (
    <View>
      <View style={{flexDirection: 'row', justifyContent: 'center', gap: u(10), marginVertical: u(18)}}>
        {[1, 2, 3, 4, 5].map(n => (
          <Focusable key={n} hasTVPreferredFocus={n === (current || 3)} onPress={() => { s.user.set([item.id], {rating: n}); ui.toast(`Rated ${n} ${n === 1 ? 'star' : 'stars'}.`); close(); }} accessibilityLabel={`${n} ${n === 1 ? 'star' : 'stars'}`} scale={1.2}>
            {({focused}) => (
              <View style={{padding: u(6), borderRadius: 99, backgroundColor: focused && isTV ? colors.white : 'transparent'}}>
                <Icon name="star" size={32} color={focused && isTV ? colors.actionInk : n <= current ? colors.amber : colors.textTertiary} fill={n <= current ? true : undefined} />
              </View>
            )}
          </Focusable>
        ))}
      </View>
      {current ? <Button label="Clear rating" variant="ghost" onPress={() => { s.user.set([item.id], {rating: 0}); close(); }} /> : null}
    </View>
  );
}

export function openAddToPlaylist(s: Services, ids: string[]) {
  ui.sheet({title: ids.length > 1 ? `Add ${ids.length} to a playlist` : 'Add to playlist', render: close => <PlaylistPicker s={s} ids={ids} close={close} />});
}

function PlaylistPicker({s, ids, close}: {s: Services; ids: string[]; close: () => void}) {
  const q = useQuery('playlists', () => s.lists.playlists());
  return (
    <View>
      <ListRow icon="plus" label="New playlist" help="Name it, then these titles are added" onPress={() => {
        close();
        ui.prompt({title: 'New playlist', fields: [{key: 'title', label: 'Name', placeholder: 'Saved queue'}, {key: 'description', label: 'Description (optional)'}], submitLabel: 'Create', onSubmit: v => {
          if (!v.title.trim()) return 'Enter a name.';
          s.lists.createPlaylist(v.title.trim(), v.description, false, ids).then(p => ui.toast(`Created ${p.title}.`));
          return null;
        }});
      }} hasTVPreferredFocus />
      {(q.data ?? []).map(p => (
        <ListRow key={p.id} icon={p.media === 'music' ? 'listMusic' : 'queue'} label={p.title} help={`${plural(p.itemIds.length, 'item', 'items')}${p.shared ? ' · Shared' : ''}`} onPress={() => { s.lists.addToPlaylist(p.id, ids).then(() => ui.toast(ids.length > 1 ? `Added ${plural(ids.length, 'item', 'items')} to ${p.title}.` : `Added to ${p.title}.`)); close(); }} />
      ))}
    </View>
  );
}

export function openAddToCollection(s: Services, ids: string[]) {
  ui.sheet({title: ids.length > 1 ? `Add ${ids.length} to a collection` : 'Add to collection', render: close => <CollectionPicker s={s} ids={ids} close={close} />});
}

function CollectionPicker({s, ids, close}: {s: Services; ids: string[]; close: () => void}) {
  const q = useQuery('collections:all', () => s.catalog.collections());
  return (
    <View>
      <ListRow icon="plus" label="New collection" onPress={() => {
        close();
        ui.prompt({title: 'New collection', fields: [{key: 'title', label: 'Name'}, {key: 'description', label: 'Description (optional)'}], submitLabel: 'Create', onSubmit: v => {
          if (!v.title.trim()) return 'Enter a name.';
          s.lists.createCollection(v.title.trim(), v.description, false, ids).then(c => ui.toast(`Created ${c.title}.`));
          return null;
        }});
      }} hasTVPreferredFocus />
      {(q.data ?? []).map(c => (
        <ListRow key={c.id} icon="layers" label={c.title} help={`${plural(c.itemIds.length, 'title', 'titles')}${c.shared ? ' · Shared with everyone' : ' · Only you'}`} onPress={() => { s.lists.addToCollection(c.id, ids).then(() => ui.toast(`Added to ${c.title}.`)); close(); }} />
      ))}
    </View>
  );
}

export function openDownloadSheet(s: Services, item: Item) {
  ui.sheet({title: item.kind === 'show' || item.kind === 'season' ? `Download ${item.title}` : 'Download', subtitle: 'The server prepares the file, then it saves to this device.', render: close => <DownloadBody s={s} item={item} close={close} />});
}

function DownloadBody({s, item, close}: {s: Services; item: Item; close: () => void}) {
  const [quality, setQuality] = useState('1080p');
  const [policy, setPolicy] = useState<'all' | 'unwatched' | 'next'>('unwatched');
  const whole = item.kind === 'show' || item.kind === 'season';
  return (
    <View>
      <Text variant="captionStrong" tone="secondary" style={{marginBottom: u(8)}}>Quality</Text>
      {[
        {id: 'Original', help: 'The file as it is'},
        {id: '1080p', help: 'About 2.1 GB per hour'},
        {id: '720p', help: 'Smaller · about 1.1 GB per hour'},
        {id: '480p', help: 'Smallest · about 600 MB per hour'},
      ].map(q => <ListRow key={q.id} label={q.id} help={q.help} onPress={() => setQuality(q.id)} chevron={false} right={quality === q.id ? <Icon name="check" size={18} color={colors.accentStrong} /> : undefined} />)}
      {whole ? (
        <>
          <Text variant="captionStrong" tone="secondary" style={{marginVertical: u(8)}}>Which episodes</Text>
          <Segmented value={policy} onChange={setPolicy} options={[{value: 'all', label: 'All episodes'}, {value: 'unwatched', label: 'Unwatched only'}, {value: 'next', label: 'Next unwatched'}]} />
          {policy === 'next' ? <Text variant="caption" tone="tertiary" style={{marginTop: u(6)}}>Keep the next 3 episodes</Text> : null}
        </>
      ) : null}
      <Text variant="caption" tone="tertiary" style={{marginTop: u(12)}}>212 GB of download space left on the server.</Text>
      <Button label={whole ? 'Download episodes' : 'Download'} icon="download" variant="primary" style={{marginTop: u(14)}} onPress={() => {
        (whole ? s.downloads.addShow(item.kind === 'season' ? item.showId : item.id, policy, 3, quality) : s.downloads.add([item.id], quality)).then(() => ui.toast('Added to your downloads.', {action: {label: 'View', onPress: () => go('Downloads')}}));
        close();
      }} />
    </View>
  );
}

/** Hook for detail pages: the item's user state that re-renders on changes. */
export function useItemState(id: string) {
  const s = useServices();
  useQuery('state:' + id, async () => s.user.state(id));
  return s.user.state(id);
}

export const platformName = Platform.isTV ? 'TV' : 'device';
export {ToggleRow, TextField};
