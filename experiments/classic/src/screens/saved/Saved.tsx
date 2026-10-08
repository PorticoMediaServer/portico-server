import React, {useEffect, useState} from 'react';
import {View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {Entry, SavedResource, SavedTab} from '../../data/types';
import {Screen} from '../../ui/Page';
import {MediaCard} from '../../ui/Card';
import {Button} from '../../ui/Button';
import {EmptyState, ListRow, Segmented, Tabs, TextField} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Artwork} from '../../ui/Artwork';
import {Focusable} from '../../ui/Focusable';
import {ChoiceSheet, ConfirmDialog, Sheet} from '../../ui/Sheet';
import {GridSkeleton} from '../../ui/Grid';
import {colors} from '../../theme/tokens';
import {gridFor, useMetrics} from '../../theme/layout';
import {useContentWidth} from '../../ui/useWidth';
import {useCardHandlers} from '../../app/useCards';
import {useToast} from '../../app/toast';
import {push} from '../../nav/nav';

const TABS: {value: SavedTab; label: string}[] = [{value: 'watchlist', label: 'Watchlist'}, {value: 'favorites', label: 'Favorites'}, {value: 'playlists', label: 'Playlists'}, {value: 'collections', label: 'Collections'}, {value: 'views', label: 'Saved views'}, {value: 'history', label: 'History'}];
const EMPTY: Record<SavedTab, [string, string]> = {
  watchlist: ['Your Watchlist is empty', 'Add a title to your Watchlist from its page and it waits for you here.'],
  favorites: ['No favorites yet', 'Mark anything as a favorite to keep it one tap away.'],
  playlists: ['No playlists yet', 'Make a playlist here, or add any song, album or video to one from its More menu.'],
  collections: ['No collections yet', 'Personal collections group titles across a library the way you like.'],
  views: ['No saved views yet', 'Filter a library the way you like and choose Save view. It’s rerun every time you open it.'],
  history: ['Nothing watched yet', 'What you watch and listen to on this profile appears here.'],
};

/** Saved: Watchlist, Favorites, Playlists, Collections, Saved views and History. */
export function SavedScreen() {
  const route = useRoute() as {params?: {tab?: SavedTab}};
  const m = useMetrics();
  const client = useClient();
  const toast = useToast();
  const [tab, setTab] = useState<SavedTab>(route.params?.tab ?? 'watchlist');
  useEffect(() => { if (route.params?.tab) setTab(route.params.tab); }, [route.params?.tab]);
  const [filter, setFilter] = useState<'all' | 'unwatched' | 'inProgress'>('all');
  const [sort, setSort] = useState('added');
  const [period, setPeriod] = useState('all');
  const [sheet, setSheet] = useState<'sort' | 'new' | 'clear'>();
  const [name, setName] = useState('');
  const {busy, run} = useAction();
  const data = useQuery(c => c.saved.list(tab, {filter, sort, period}), [tab, filter, sort, period], ['saved', 'personal']);
  const cards = useCardHandlers();
  const [width, onLayout] = useContentWidth();
  const d = data.data;
  const {itemWidth} = gridFor(width - m.gutter * 2, m.poster.w, m.gap);
  const resW = gridFor(width - m.gutter * 2, m.isTV ? 300 : m.isPhone ? 160 : 210, m.gap).itemWidth;
  const listTab = tab === 'watchlist' || tab === 'favorites';
  const resTab = tab === 'playlists' || tab === 'collections' || tab === 'views';
  const empty = EMPTY[tab];
  return (
    <View style={{flex: 1}} onLayout={onLayout}>
      <Screen contentStyle={{gap: m.space.lg}} refreshing={data.loading} onRefresh={data.refresh}>
        <View style={{paddingHorizontal: m.gutter, gap: m.space.md}}>
          <Text variant="display">Saved</Text>
          <Tabs tabs={TABS} value={tab} onChange={setTab} />
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.sm, alignItems: 'center'}}>
            {listTab ? <Segmented value={filter} onChange={setFilter} options={[{value: 'all', label: 'All'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}]} /> : null}
            {listTab ? <Button size="sm" icon="sort" label={`Sort: ${{added: 'Date added', title: 'Title', year: 'Year', duration: 'Length'}[sort]}`} variant="secondary" onPress={() => setSheet('sort')} /> : null}
            {tab === 'history' ? <Segmented value={period} onChange={setPeriod} options={[{value: '24h', label: 'Today'}, {value: '7d', label: 'This week'}, {value: '30d', label: '30 days'}, {value: 'all', label: 'All time'}]} /> : null}
            {tab === 'history' && d?.history.length ? <Button size="sm" icon="trash" label="Clear history…" variant="ghost" onPress={() => setSheet('clear')} /> : null}
            {tab === 'playlists' || tab === 'collections' ? <Button size="sm" icon="plus" label={tab === 'playlists' ? 'New playlist' : 'New collection'} variant="secondary" onPress={() => setSheet('new')} /> : null}
          </View>
        </View>
        {!d ? <GridSkeleton columns={Math.max(2, Math.floor((width - m.gutter * 2) / (m.poster.w + m.gap)))} itemWidth={itemWidth} shape="poster" /> : null}
        {d && listTab ? (d.entries.length ? (
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap, paddingHorizontal: m.gutter}}>
            {d.entries.map((e, i) => <MediaCard key={e.id} entry={e} shape="poster" width={e.shape === 'poster' ? itemWidth : itemWidth} onPress={cards.onPress} onLongPress={cards.onLongPress} hasTVPreferredFocus={i === 0 && m.isTV} />)}
          </View>
        ) : <EmptyState icon={tab === 'watchlist' ? 'saved' : 'heart'} title={filter === 'all' ? empty[0] : filter === 'unwatched' ? `No unwatched titles in your ${tab === 'watchlist' ? 'Watchlist' : 'Favorites'}` : `Nothing in progress in your ${tab === 'watchlist' ? 'Watchlist' : 'Favorites'}`} body={filter === 'all' ? empty[1] : undefined} action={filter !== 'all' ? {label: 'Show all', onPress: () => setFilter('all')} : undefined} />) : null}
        {d && resTab ? (d.resources.length ? (
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap, paddingHorizontal: m.gutter}}>
            {d.resources.map((r, i) => <ResourceCard key={r.id} r={r} width={resW} first={i === 0} />)}
          </View>
        ) : <EmptyState icon={tab === 'playlists' ? 'playlist' : tab === 'views' ? 'filter' : 'collection'} title={empty[0]} body={empty[1]} />) : null}
        {d && tab === 'history' ? (d.history.length ? (
          <View style={{paddingHorizontal: m.gutter}}>
            {d.history.map((h, i) => {
              const day = new Date(h.watchedAt).toDateString() === new Date().toDateString() ? 'Today' : 'Earlier';
              const prevDay = i ? (new Date(d.history[i - 1]!.watchedAt).toDateString() === new Date().toDateString() ? 'Today' : 'Earlier') : undefined;
              return (
                <View key={h.entry.id}>
                  {day !== prevDay ? <Text variant="label" tone="tertiary" style={{marginTop: i ? 16 : 0, marginBottom: 6}}>{day}</Text> : null}
                  <ListRow left={<Artwork source={h.entry.image} title={h.entry.title} width={h.entry.shape === 'wide' ? (m.isTV ? 128 : 80) : m.isTV ? 64 : 44} height={m.isTV ? 72 : 46} radius={6} />} title={h.entry.kind === 'episode' ? `${h.entry.subtitle} · ${h.entry.title}` : h.entry.title} subtitle={`${h.entry.watched ? 'Watched' : h.entry.progress ? `${Math.round(h.entry.progress * 100)}% watched` : 'Played'} · ${new Date(h.watchedAt).toLocaleString('en-US', {weekday: 'short', hour: 'numeric', minute: '2-digit'})}`} onPress={() => cards.onPress(h.entry)} onLongPress={() => cards.onLongPress(h.entry)} />
                </View>
              );
            })}
          </View>
        ) : <EmptyState icon="history" title={empty[0]} body={empty[1]} />) : null}
      </Screen>
      <ChoiceSheet visible={sheet === 'sort'} onClose={() => setSheet(undefined)} title="Sort" options={[{value: 'added', label: 'Date added'}, {value: 'title', label: 'Title'}, {value: 'year', label: 'Year'}, {value: 'duration', label: 'Length'}]} value={sort} onChange={setSort} />
      <Sheet visible={sheet === 'new'} onClose={() => setSheet(undefined)} title={tab === 'playlists' ? 'New playlist' : 'New collection'} size="sm" footer={<><Button label="Cancel" variant="ghost" onPress={() => setSheet(undefined)} /><Button label="Create" variant="primary" disabled={!name.trim()} loading={busy} onPress={() => void run(async () => { const id = tab === 'playlists' ? await client.saved.createPlaylist(name.trim()) : await client.saved.createCollection(name.trim()); setSheet(undefined); setName(''); toast({message: `Created ${name.trim()}.`}); push('Resource', {kind: tab === 'playlists' ? 'playlist' : 'collection', id}); })} /></>}>
        <TextField label="Name" value={name} onChangeText={setName} autoFocus />
      </Sheet>
      <ConfirmDialog visible={sheet === 'clear'} onClose={() => setSheet(undefined)} title="Clear your viewing history?" body="Everything this profile has watched or listened to is removed from History. Watched marks and resume points stay." confirmLabel="Clear history" destructive busy={busy} onConfirm={() => void run(async () => { await client.saved.clearHistory(); setSheet(undefined); })} />
    </View>
  );
}

/** A playlist, collection or saved view as a collage card. */
function ResourceCard({r, width, first}: {r: SavedResource; width: number; first?: boolean}) {
  const m = useMetrics();
  const imgs = (r.images ?? []).filter(Boolean);
  const half = width / 2;
  return (
    <Focusable onPress={() => push('Resource', {kind: r.kind, id: r.id})} focusScale={m.isTV ? 1.05 : 1} hasTVPreferredFocus={first && m.isTV} style={{width}} accessibilityLabel={`${r.title}, ${r.count} items`}>
      {({focused}) => (
        <View style={{gap: 8}}>
          <View style={{width, height: width, borderRadius: m.radius.card, overflow: 'hidden', flexDirection: 'row', flexWrap: 'wrap', backgroundColor: colors.raised, borderWidth: focused ? 3 : 0, borderColor: colors.focus}}>
            {imgs.length >= 4 ? imgs.slice(0, 4).map((img, i) => <Artwork key={i} source={img} title={r.title} width={half} height={half} radius={0} />) : <Artwork source={imgs[0]} title={r.title} width={width} height={width} radius={0} />}
            {r.kind === 'view' ? <View style={{position: 'absolute', right: 8, bottom: 8, backgroundColor: colors.overlay, borderRadius: 8, padding: 6}}><Text variant="micro" tone="accent">SMART</Text></View> : null}
          </View>
          <Text variant="captionStrong" numberOfLines={1}>{r.title}</Text>
          <Text variant="caption" tone="tertiary" numberOfLines={1}>{r.count} item{r.count === 1 ? '' : 's'}{r.shared ? ' · Shared' : ''}</Text>
        </View>
      )}
    </Focusable>
  );
}

export type {Entry};
