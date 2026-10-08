import React, {useMemo, useState} from 'react';
import {ScrollView, StyleSheet, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {Item, Playlist} from '../../data/types';
import {Artwork, Scrim} from '../../design/Artwork';
import {Button, IconButton} from '../../design/Button';
import {Card} from '../../design/Card';
import {Badge, Chip, ListRow, Segmented, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {ui} from '../../design/overlays';
import {EmptyState, GridSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {Screen} from '../../nav/Screen';
import {back, go, goSection, push, type Routes} from '../../nav/routes';
import {player} from '../../state/player';
import {isManager, useSession} from '../../state/session';
import {openItemMenu} from '../shared/actions';
import {openItem, playItem, subtitleFor, toCard} from '../shared/cards';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

const TABS = [{value: 'watchlist', label: 'Watchlist'}, {value: 'favorites', label: 'Favorites'}, {value: 'playlists', label: 'Playlists'}, {value: 'collections', label: 'Collections'}, {value: 'views', label: 'Saved views'}, {value: 'history', label: 'History'}];

function useGrid(shape: 'poster' | 'landscape' = 'poster') {
  const layout = useLayout();
  const cols = isTV ? (shape === 'poster' ? 7 : 4) : layout.compact ? (shape === 'poster' ? 3 : 1) : shape === 'poster' ? 5 : 3;
  const gap = isTV ? 36 : 12;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * gap) / cols);
  return {cols, gap, w};
}

export function SavedScreen() {
  const route = useRoute<RouteProp<Routes, 'Saved'>>();
  const [tab, setTab] = useState(route.params?.tab ?? 'watchlist');
  React.useEffect(() => {
    if (route.params?.tab) setTab(route.params.tab);
  }, [route.params?.tab]);
  return (
    <Screen heading="Saved" headerRight={!isTV ? <IconButton icon="download" label="Downloads" variant="ghost" onPress={() => goSection('Downloads')} /> : undefined}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8), marginBottom: u(16)}}>
        {TABS.map((t, i) => <Chip key={t.value} label={t.label} active={tab === t.value} onPress={() => setTab(t.value)} hasTVPreferredFocus={i === 0} />)}
      </ScrollView>
      {tab === 'watchlist' || tab === 'favorites' ? <FlagList flag={tab} /> : null}
      {tab === 'playlists' ? <Playlists /> : null}
      {tab === 'collections' ? <MyCollections /> : null}
      {tab === 'views' ? <Views /> : null}
      {tab === 'history' ? <History /> : null}
    </Screen>
  );
}

function FlagList({flag}: {flag: 'watchlist' | 'favorites'}) {
  const s = useServices();
  const session = useSession();
  const [filter, setFilter] = useState<'all' | 'unwatched' | 'inProgress'>('all');
  const [sort, setSort] = useState<'added' | 'title' | 'year'>('added');
  const rows = useQuery('home', () => s.catalog.home());
  const all = useQuery(`flag:${flag}`, async () => {
    const ids: string[] = [];
    for (const lib of await s.catalog.libraries()) {
      const page = await s.catalog.browse({libraryId: lib.id, filters: {quick: [flag], fields: {}}, sort: {field: 'added', direction: 'desc'}, pageSize: 500});
      ids.push(...page.items.map(i => i.id));
    }
    return ids;
  });
  void rows;
  const items = useItems(all.data);
  const {w, gap} = useGrid();
  const list = useMemo(() => {
    let l = (all.data ?? []).map(id => items.lookup(id)).filter(Boolean) as Item[];
    if (filter === 'unwatched') l = l.filter(i => !s.user.state(i.id).watched);
    if (filter === 'inProgress') l = l.filter(i => s.user.state(i.id).progress > 0);
    if (sort === 'title') l = [...l].sort((a, b) => a.sortTitle.localeCompare(b.sortTitle));
    if (sort === 'year') l = [...l].sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
    return l;
  }, [all.data, items, filter, sort, s]);
  const label = flag === 'watchlist' ? 'Watchlist' : 'Favorites';
  return (
    <View>
      <View style={styles.toolbar}>
        <Segmented value={filter} onChange={setFilter} options={[{value: 'all', label: 'All'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}]} />
        <Button icon="sort" size="sm" label={{added: 'Date added', title: 'Title', year: 'Year'}[sort]} onPress={() => ui.menu({title: 'Sort', actions: [{label: 'Date added', checked: sort === 'added', onPress: () => setSort('added')}, {label: 'Title', checked: sort === 'title', onPress: () => setSort('title')}, {label: 'Year', checked: sort === 'year', onPress: () => setSort('year')}]})} />
        <Text variant="caption" tone="tertiary">{list.length} items</Text>
      </View>
      {!all.data ? <GridSkeleton columns={3} width={w} /> : null}
      {all.data && !list.length ? (
        filter !== 'all' ? <EmptyState icon="filter" title={filter === 'unwatched' ? `No unwatched titles in your ${label}` : `Nothing in progress in your ${label}`} action={{label: 'Show all', onPress: () => setFilter('all')}} /> :
        flag === 'watchlist' ? <EmptyState icon="bookmark" title="Your Watchlist is empty" body="Add a title to your Watchlist from its page and it waits for you here." /> : <EmptyState icon="heart" title="No favorites yet" body="Mark anything as a favorite to keep it one click away." />
      ) : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
        {list.map(it => <Card key={it.id} item={toCard(it, s.user.state(it.id), {shape: it.kind === 'album' || it.kind === 'artist' ? 'square' : 'poster', lookup: items.lookup})} width={w} showTitle="always" onPress={() => openItem(it)} onLongPress={() => openItemMenu(s, it, {manager: isManager(session.role)})} />)}
      </View>
    </View>
  );
}

function ListTile({title, subtitle, badge, art, tint, onPress, first}: {title: string; subtitle: string; badge?: string; art?: string; tint: string; onPress: () => void; first?: boolean}) {
  const {w} = useGrid('landscape');
  return (
    <Focusable onPress={onPress} hasTVPreferredFocus={first} scale={1.05} accessibilityLabel={`${title}, ${subtitle}`}>
      {({focused}) => (
        <View style={[styles.tile, {width: w, height: Math.max(u(110), w * 0.42)}, focused && isTV && styles.tileFocus]}>
          <Artwork art={art} tint={tint} style={StyleSheet.absoluteFill} />
          <View style={[StyleSheet.absoluteFill, {backgroundColor: tint, opacity: 0.28}]} />
          <Scrim bottom={0.95} left={0.6} />
          <View style={{padding: u(14)}}>
            {badge ? <Badge label={badge} /> : null}
            <Text variant="display" numberOfLines={1}>{title}</Text>
            <Text variant="caption" tone="secondary" numberOfLines={1}>{subtitle}</Text>
          </View>
        </View>
      )}
    </Focusable>
  );
}

function Playlists() {
  const s = useServices();
  const q = useQuery('playlists', () => s.lists.playlists());
  const firsts = useItems(q.data?.map(p => p.itemIds[0]).filter(Boolean));
  const {gap} = useGrid('landscape');
  const create = () => ui.prompt({title: 'New playlist', fields: [{key: 'title', label: 'Name'}, {key: 'description', label: 'Description (optional)', multiline: true}], submitLabel: 'Create', onSubmit: v => { if (!v.title.trim()) return 'Enter a name.'; s.lists.createPlaylist(v.title.trim(), v.description, false).then(p => ui.toast(`Created ${p.title}.`)); return null; }});
  return (
    <View>
      <Button label="New playlist" icon="plus" size="sm" onPress={create} style={{alignSelf: 'flex-start', marginBottom: u(12)}} />
      {q.data && !q.data.length ? <EmptyState icon="listMusic" title="No playlists yet" body="Make a playlist here, or add any song, album or video to one from its More menu." /> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
        {(q.data ?? []).map((p, i) => {
          const lead = firsts.lookup(p.itemIds[0]);
          return <ListTile key={p.id} first={i === 0} title={p.title} subtitle={`${plural(p.itemIds.length, 'item', 'items')} · ${p.media === 'music' ? 'Music' : 'Video'}`} badge={p.shared ? 'Shared' : undefined} art={lead?.art.square ?? lead?.art.backdrop} tint={p.tint} onPress={() => push('Resource', {kind: 'playlist', id: p.id})} />;
        })}
      </View>
    </View>
  );
}

function MyCollections() {
  const s = useServices();
  const q = useQuery('collections:all', () => s.catalog.collections());
  const firsts = useItems(q.data?.map(p => p.itemIds[0]).filter(Boolean));
  const {gap} = useGrid('landscape');
  const create = () => ui.prompt({title: 'New collection', fields: [{key: 'title', label: 'Name'}, {key: 'description', label: 'Description (optional)', multiline: true}], submitLabel: 'Create', onSubmit: v => { if (!v.title.trim()) return 'Enter a name.'; s.lists.createCollection(v.title.trim(), v.description, false).then(c => ui.toast(`Created ${c.title}.`)); return null; }});
  return (
    <View>
      <Button label="New collection" icon="plus" size="sm" onPress={create} style={{alignSelf: 'flex-start', marginBottom: u(12)}} />
      {q.data && !q.data.length ? <EmptyState icon="layers" title="No collections yet" body="Personal collections group titles across a library the way you like." /> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
        {(q.data ?? []).map((c, i) => {
          const lead = firsts.lookup(c.itemIds[0]);
          return <ListTile key={c.id} first={i === 0} title={c.title} subtitle={`${plural(c.itemIds.length, 'title', 'titles')} · ${c.owner === 'me' ? 'Only you' : 'Shared with everyone'}`} badge={c.owner === 'server' ? 'Shared collection' : undefined} art={lead?.art.backdrop ?? lead?.art.poster} tint={c.tint} onPress={() => push('Resource', {kind: 'collection', id: c.id})} />;
        })}
      </View>
    </View>
  );
}

function Views() {
  const s = useServices();
  const q = useQuery('views', () => s.lists.savedViews());
  if (q.data && !q.data.length) return <EmptyState icon="filter" title="No saved views yet" body="Filter a library the way you like and choose Save view. It’s rerun every time you open it." />;
  return (
    <View>
      {(q.data ?? []).map((v, i) => (
        <ListRow key={v.id} icon="filter" label={v.title} help={`${v.libraryId === 'lib-movies' ? 'Movies' : 'Library'} · ${[...v.filters.quick, ...Object.values(v.filters.fields).flat()].join(', ') || 'Everything'} · sorted by ${v.sort.field}${v.onHome ? ' · On Home' : ''}`} onPress={() => push('Resource', {kind: 'view', id: v.id})} hasTVPreferredFocus={i === 0} />
      ))}
    </View>
  );
}

function History() {
  const s = useServices();
  const session = useSession();
  const [period, setPeriod] = useState('30d');
  const q = useQuery('history:' + period, () => s.user.history(period));
  const items = useItems(q.data?.map(h => h.itemId));
  const day = (t: number) => {
    const d = new Date(t);
    const today = new Date();
    if (d.toDateString() === today.toDateString()) return 'Today';
    const y = new Date(Date.now() - 86400000);
    if (d.toDateString() === y.toDateString()) return 'Yesterday';
    return d.toLocaleDateString('en-US', {weekday: 'long', month: 'short', day: 'numeric'});
  };
  const groups = new Map<string, {itemId: string; at: number}[]>();
  (q.data ?? []).forEach(h => groups.set(day(h.at), [...(groups.get(day(h.at)) ?? []), h]));
  return (
    <View>
      <View style={styles.toolbar}>
        <Segmented value={period} onChange={setPeriod} options={[{value: '24h', label: 'Today'}, {value: '7d', label: 'This week'}, {value: '30d', label: '30 days'}, {value: '90d', label: '90 days'}, {value: 'all', label: 'All time'}]} />
        <Button label="Clear history…" size="sm" variant="danger" onPress={() => ui.confirm({title: 'Clear your viewing history?', body: 'Everything this profile has watched or listened to is removed from History. Watched marks and resume points stay.', confirmLabel: 'Clear history', destructive: true, onConfirm: () => s.user.clearHistory().then(() => ui.toast('History cleared.'))})} />
      </View>
      {q.data && !q.data.length ? <EmptyState icon="history" title="Nothing watched yet" body="What you watch and listen to on this profile appears here." /> : null}
      {[...groups.entries()].map(([label, list]) => (
        <View key={label} style={{marginBottom: u(10)}}>
          <Text variant="eyebrow" tone="tertiary" style={{marginVertical: u(8)}}>{label}</Text>
          {list.map(h => {
            const it = items.lookup(h.itemId);
            if (!it) return null;
            const c = toCard(it, s.user.state(it.id), {lookup: items.lookup});
            return (
              <Focusable key={h.itemId + h.at} onPress={() => openItem(it)} onLongPress={() => openItemMenu(s, it, {manager: isManager(session.role)})} accessibilityLabel={it.title}>
                {({focused}) => (
                  <View style={[styles.histRow, focused && isTV && {backgroundColor: 'rgba(255,255,255,0.12)'}]}>
                    <Artwork art={c.art} title={it.title} tint={it.tint} style={{width: u(44), height: c.shape === 'poster' ? u(66) : u(44), borderRadius: 6}} fallbackVariant="caption" />
                    <View style={{flex: 1, marginLeft: 12}}>
                      <Text variant="bodyStrong" numberOfLines={1}>{it.kind === 'episode' ? c.title : it.title}</Text>
                      <Text variant="caption" tone="tertiary" numberOfLines={1}>{c.subtitle} · {new Date(h.at).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}</Text>
                    </View>
                    {s.user.state(it.id).watched ? <Badge label="Finished" tone="healthy" /> : null}
                  </View>
                )}
              </Focusable>
            );
          })}
        </View>
      ))}
    </View>
  );
}

// ---------------------------------------------------------------- Resource (playlist, collection, saved view)

export function ResourceScreen() {
  const route = useRoute<RouteProp<Routes, 'Resource'>>();
  const s = useServices();
  const session = useSession();
  const {kind, id} = route.params;
  const pl = useQuery(kind === 'playlist' ? 'playlist:' + id : null, () => s.lists.playlist(id));
  const col = useQuery(kind === 'collection' ? 'collections:all' : null, () => s.catalog.collections());
  const views = useQuery(kind === 'view' ? 'views' : null, () => s.lists.savedViews());
  const view = views.data?.find(v => v.id === id);
  const viewItems = useQuery(view ? 'viewitems:' + id + JSON.stringify(view.filters) : null, () => s.catalog.browse({libraryId: view!.libraryId, filters: view!.filters, sort: view!.sort, pageSize: 200}));
  const collection = col.data?.find(c => c.id === id);
  const playlist = pl.data as Playlist | undefined;
  const ids = kind === 'playlist' ? playlist?.itemIds ?? [] : kind === 'collection' ? collection?.itemIds ?? [] : viewItems.data?.items.map(i => i.id) ?? [];
  const items = useItems(ids.length ? ids : undefined);
  const title = playlist?.title ?? collection?.title ?? view?.title ?? '';
  const desc = playlist?.description ?? collection?.description ?? '';
  const shared = playlist?.shared ?? collection?.shared ?? view?.shared ?? false;
  const {w, gap} = useGrid();
  const noun = kind === 'playlist' ? 'Playlist' : kind === 'collection' ? (collection?.owner === 'server' ? 'Shared collection' : 'Collection') : 'Saved view';
  const rename = () => ui.prompt({title: `Rename ${noun.toLowerCase()}`, fields: [{key: 't', label: 'Name', initial: title}], submitLabel: 'Save', onSubmit: v => { if (!v.t.trim()) return 'Enter a name.'; (kind === 'playlist' ? s.lists.renamePlaylist(id, v.t.trim()) : kind === 'collection' ? s.lists.renameCollection(id, v.t.trim()) : s.lists.renameView(id, v.t.trim())).then(() => ui.toast('Renamed.')); return null; }});
  const del = () => ui.confirm({
    title: `Delete ${title}?`,
    body: kind === 'playlist' ? 'The playlist is deleted for everyone it’s shared with. The titles in it stay in your libraries.' : kind === 'collection' ? 'The collection is deleted for everyone who can see it. The titles in it stay in your libraries.' : 'The saved view is deleted. The titles it found stay in your libraries.',
    confirmLabel: `Delete ${noun.toLowerCase()}`, destructive: true,
    onConfirm: () => (kind === 'playlist' ? s.lists.deletePlaylist(id) : kind === 'collection' ? s.lists.deleteCollection(id) : s.lists.deleteView(id)).then(() => { ui.toast('Deleted.'); back(); }),
  });
  const options = () => ui.menu({title: `${noun} options`, actions: [
    {label: 'Rename…', icon: 'edit', onPress: rename},
    ...(kind === 'view' && view ? [{label: view.onHome ? 'Remove from Home' : 'Show on Home', icon: 'home' as const, onPress: () => { view.onHome = !view.onHome; ui.toast(view.onHome ? 'Shown on Home.' : 'Removed from Home.'); }}] : []),
    {label: `Delete ${noun.toLowerCase()}…`, icon: 'trash', destructive: true, onPress: del},
  ]});
  const list = ids.map(x => items.lookup(x)).filter(Boolean) as Item[];
  const music = list.length > 0 && list.every(i => i.kind === 'track');
  return (
    <Screen title={title}>
      <Text variant="eyebrow" tone="accent">{noun} · {ids.length} items</Text>
      <Text variant="hero">{title}</Text>
      {desc ? <Text variant="body" tone="secondary" style={{marginTop: 4}}>{desc}</Text> : null}
      <Text variant="caption" tone="tertiary" style={{marginTop: 4}}>{shared ? 'Visible to everyone on this server' : 'Only this profile can see it'}</Text>
      <View style={{flexDirection: 'row', gap: 10, marginVertical: u(14), flexWrap: 'wrap'}}>
        <Button label="Play" icon="play" variant="primary" hasTVPreferredFocus disabled={!list.length} onPress={() => { if (music) { player.play(list.map(i => i.id), {kind: 'audio'}); go('NowPlaying'); } else if (list[0]) playItem(list[0], {queue: list.filter(i => i.kind === 'movie' || i.kind === 'episode').map(i => i.id)}); }} />
        <Button label="Shuffle" icon="shuffle" disabled={!list.length} onPress={() => { if (music) { player.play(list.map(i => i.id), {kind: 'audio', shuffle: true}); go('NowPlaying'); } else if (list.length) playItem(list[Math.floor(Math.random() * list.length)]); }} />
        {kind === 'playlist' || collection?.owner === 'me' || kind === 'view' ? <Button label="Options" icon="more" onPress={options} /> : null}
      </View>
      {kind === 'view' && view ? <ToggleRow label="Show on Home" help="Adds this view as a row on Home" value={view.onHome} onChange={v => { view.onHome = v; ui.toast(v ? 'Shown on Home.' : 'Removed from Home.'); }} /> : null}
      {!list.length && !items.loading ? <EmptyState icon="layers" title={kind === 'playlist' ? 'This playlist is empty' : kind === 'collection' ? 'Nothing in this collection yet' : 'Nothing matches this view right now'} body={kind === 'playlist' ? 'Open a title’s menu and choose Add to playlist.' : undefined} /> : null}
      {music || kind === 'playlist' ? (
        list.map((it, i) => (
          <ListRow key={it.id + i} label={`${i + 1}. ${it.title}`} help={subtitleFor(it, items.lookup)} chevron={false} compact onPress={() => (music ? (player.play(list.map(x => x.id), {kind: 'audio', index: i}), go('NowPlaying')) : openItem(it))}
            right={kind === 'playlist' ? (
              <View style={{flexDirection: 'row'}}>
                <IconButton icon="arrowUp" label="Move up" size={30} variant="ghost" onPress={() => s.lists.movePlaylistItem(id, i, i - 1)} />
                <IconButton icon="arrowDown" label="Move down" size={30} variant="ghost" onPress={() => s.lists.movePlaylistItem(id, i, i + 1)} />
                <IconButton icon="more" label={`Actions for ${it.title}`} size={30} variant="ghost" onPress={() => ui.menu({title: it.title, actions: [{label: 'Go to item', icon: 'arrowRight', onPress: () => openItem(it)}, {label: 'Remove from playlist', icon: 'x', destructive: true, onPress: () => s.lists.removeFromPlaylist(id, i).then(() => ui.toast('Removed from playlist.'))}]})} />
              </View>
            ) : undefined} />
        ))
      ) : (
        <View style={{flexDirection: 'row', flexWrap: 'wrap', gap}}>
          {list.map(it => <Card key={it.id} item={toCard(it, s.user.state(it.id), {lookup: items.lookup})} width={w} showTitle="always" onPress={() => openItem(it)} onLongPress={() => (kind === 'collection' && collection?.owner === 'me' ? ui.menu({title: it.title, actions: [{label: 'Go to item', icon: 'arrowRight', onPress: () => openItem(it)}, {label: 'Remove from collection', icon: 'x', destructive: true, onPress: () => s.lists.removeFromCollection(id, it.id)}]}) : openItemMenu(s, it, {manager: isManager(session.role)}))} />)}
        </View>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  toolbar: {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: u(10), marginBottom: u(14)},
  tile: {borderRadius: radius.lg, overflow: 'hidden', justifyContent: 'flex-end', backgroundColor: colors.raised},
  tileFocus: {borderWidth: 4, borderColor: colors.white},
  histRow: {flexDirection: 'row', alignItems: 'center', paddingVertical: u(6), paddingHorizontal: u(6), borderRadius: radius.md},
});
