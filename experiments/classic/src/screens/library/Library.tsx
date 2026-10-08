import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {FlatList, ScrollView, TVFocusGuideView, View} from 'react-native';
import {useRoute} from '@react-navigation/native';
import {useAction, useClient, useQuery} from '../../data/DataProvider';
import type {BrowseQuery, Entry, Filters, Library, LibraryView, Shape, SortField} from '../../data/types';
import {Screen, useChromeInsets, useRegisterNav} from '../../ui/Page';
import {Shelf, ShelfSkeleton} from '../../ui/Shelf';
import {EntryGrid, type GridHandle} from '../../ui/Grid';
import {Button} from '../../ui/Button';
import {Chip, EmptyState, ListRow, Notice, Segmented, Tabs, TextField} from '../../ui/Controls';
import {Text} from '../../ui/Text';
import {Focusable} from '../../ui/Focusable';
import {Artwork} from '../../ui/Artwork';
import {Sheet} from '../../ui/Sheet';
import {colors} from '../../theme/tokens';
import {useMetrics} from '../../theme/layout';
import {useContentWidth} from '../../ui/useWidth';
import {useCardHandlers} from '../../app/useCards';
import {useSession} from '../../app/session';
import {useToast} from '../../app/toast';
import {push, root} from '../../nav/nav';
import {formatDuration, formatTime} from '../../player/PlayerProvider';
import {useActions} from '../../app/actions';
import {CategoryTile} from './CategoryTile';

type ViewDef = {value: LibraryView; label: string; shape: Shape};
function viewsFor(lib?: Library): ViewDef[] {
  switch (lib?.kind) {
    case 'music': return [{value: 'discover', label: 'Discover', shape: 'square'}, {value: 'browse', label: 'Artists', shape: 'circle'}, {value: 'albums', label: 'Albums', shape: 'square'}, {value: 'songs', label: 'Songs', shape: 'square'}, {value: 'categories', label: 'Categories', shape: 'square'}];
    case 'audiobook': return [{value: 'discover', label: 'Discover', shape: 'poster'}, {value: 'browse', label: 'Books', shape: 'poster'}, {value: 'authors', label: 'Authors', shape: 'poster'}, {value: 'series', label: 'Series', shape: 'poster'}, {value: 'categories', label: 'Categories', shape: 'poster'}];
    case 'photo': return [{value: 'discover', label: 'Albums', shape: 'square'}, {value: 'timeline', label: 'Timeline', shape: 'square'}];
    case 'homevideo': return [{value: 'discover', label: 'Discover', shape: 'wide'}, {value: 'browse', label: 'Browse', shape: 'wide'}];
    case 'tv':
    case 'anime': return [{value: 'discover', label: 'Discover', shape: 'poster'}, {value: 'browse', label: 'Shows', shape: 'poster'}, {value: 'collections', label: 'Collections', shape: 'poster'}, {value: 'categories', label: 'Categories', shape: 'poster'}];
    default: return [{value: 'discover', label: 'Discover', shape: 'poster'}, {value: 'browse', label: 'Movies', shape: 'poster'}, {value: 'collections', label: 'Collections', shape: 'poster'}, {value: 'categories', label: 'Categories', shape: 'poster'}];
  }
}
const SORTS: {value: SortField; label: string; kind: 'alpha' | 'date' | 'rating' | 'duration'}[] = [
  {value: 'title', label: 'Title', kind: 'alpha'}, {value: 'added', label: 'Date added', kind: 'date'}, {value: 'year', label: 'Year', kind: 'date'}, {value: 'rating', label: 'Rating', kind: 'rating'},
  {value: 'duration', label: 'Duration', kind: 'duration'}, {value: 'lastPlayed', label: 'Last played', kind: 'date'}, {value: 'plays', label: 'Most played', kind: 'rating'}, {value: 'random', label: 'Random', kind: 'alpha'},
];
const dirLabel = (kind: string, dir: 'asc' | 'desc') => (kind === 'alpha' ? (dir === 'asc' ? 'A–Z' : 'Z–A') : kind === 'date' ? (dir === 'asc' ? 'Oldest first' : 'Newest first') : kind === 'duration' ? (dir === 'asc' ? 'Shortest first' : 'Longest first') : dir === 'asc' ? 'Lowest first' : 'Highest first');
const QUICK: {id: string; label: string; apply: (f: Filters) => Filters; on: (f: Filters) => boolean}[] = [
  {id: 'unwatched', label: 'Unwatched', apply: f => ({...f, playState: f.playState === 'unwatched' ? undefined : 'unwatched'}), on: f => f.playState === 'unwatched'},
  {id: 'inProgress', label: 'In progress', apply: f => ({...f, playState: f.playState === 'inProgress' ? undefined : 'inProgress'}), on: f => f.playState === 'inProgress'},
  {id: 'recent', label: 'Recently added', apply: f => ({...f, quick: f.quick === 'recent' ? undefined : 'recent'}), on: f => f.quick === 'recent'},
  {id: 'favorites', label: 'Favorites', apply: f => ({...f, favorite: !f.favorite || undefined}), on: f => !!f.favorite},
  {id: 'watchlist', label: 'Watchlist', apply: f => ({...f, watchlisted: !f.watchlisted || undefined}), on: f => !!f.watchlisted},
  {id: 'uhd', label: '4K', apply: f => ({...f, resolution: f.resolution?.includes('4K') ? undefined : ['4K']}), on: f => !!f.resolution?.includes('4K')},
];
const PAGE = 60;

/** A library: Discover, Browse (filters, sort, alphabet jump, saved views), Collections and Categories. */
export function LibraryScreen() {
  const {params} = useRoute() as {params: {libraryId: string; view?: LibraryView}};
  const {profile} = useSession();
  const libs = useQuery(c => c.libraries.list(), [profile?.id], ['libraries']);
  const lib = libs.data?.find(l => l.id === params.libraryId);
  const views = viewsFor(lib);
  const [view, setView] = useState<LibraryView>(params.view ?? 'discover');
  useEffect(() => { if (params.view) setView(params.view); }, [params.view]);
  const def = views.find(v => v.value === view) ?? views[0]!;
  const header = <LibraryHeader lib={lib} views={views} view={view} onView={setView} />;
  if (['browse', 'albums', 'authors', 'series', 'timeline'].includes(def.value) || (lib?.kind === 'photo' && def.value === 'discover')) {
    return <BrowseView key={`${params.libraryId}-${def.value}`} lib={lib} libraryId={params.libraryId} view={lib?.kind === 'photo' && def.value === 'discover' ? 'browse' : def.value} shape={def.shape} header={header} />;
  }
  if (def.value === 'songs') return <SongsView libraryId={params.libraryId} header={header} />;
  return (
    <Screen contentStyle={{gap: 0}}>
      {header}
      {def.value === 'discover' ? <DiscoverView libraryId={params.libraryId} /> : null}
      {def.value === 'collections' ? <CollectionsView libraryId={params.libraryId} /> : null}
      {def.value === 'categories' ? <CategoriesView libraryId={params.libraryId} shape={def.shape} /> : null}
    </Screen>
  );
}

function LibraryHeader({lib, views, view, onView}: {lib?: Library; views: ViewDef[]; view: LibraryView; onView: (v: LibraryView) => void}) {
  const m = useMetrics();
  const client = useClient();
  const toast = useToast();
  const {isAdmin} = useSession();
  const kind = lib ? {movie: 'Movies', tv: 'TV', anime: 'Anime', music: 'Music', audiobook: 'Audiobooks', photo: 'Photos', homevideo: 'Home videos'}[lib.kind] : '';
  return (
    <View style={{paddingHorizontal: m.gutter, gap: m.space.md, marginBottom: m.space.lg}}>
      <View style={{flexDirection: 'row', alignItems: 'flex-end', gap: m.space.md}}>
        <View style={{flex: 1}}>
          <Text variant="label" tone="accent">{kind} library · {lib?.count ?? '…'} {lib?.kind === 'music' ? 'artists' : lib?.kind === 'photo' ? 'albums' : lib?.kind === 'audiobook' ? 'books' : lib?.kind === 'homevideo' ? 'videos' : 'titles'}</Text>
          <Text variant="display" numberOfLines={1}>{lib?.name ?? ' '}</Text>
        </View>
        {isAdmin && lib ? (
          <View style={{flexDirection: 'row', gap: m.space.sm}}>
            <Button size="sm" icon="refresh" label={m.isPhone ? undefined : lib.scanning ? `Scanning… ${lib.scanning.found} found` : 'Scan library files'} accessibilityLabel="Scan library files" variant="secondary" loading={!!lib.scanning && m.isPhone} onPress={() => { void client.libraries.scan(lib.id); toast({message: 'Scan started.', icon: 'refresh'}); }} />
            <Button size="sm" icon="settings" label={m.isPhone ? undefined : 'Manage library'} accessibilityLabel="Manage library" variant="ghost" onPress={() => root('Admin', {section: 'libraries', param: lib.id})} />
          </View>
        ) : null}
      </View>
      {lib?.scanning ? <Notice tone="info" icon="refresh" title="Scanning your files…" body={`Titles appear here as Portico finds them. ${lib.scanning.found} found so far.`} /> : null}
      <Tabs tabs={views.map(v => ({value: v.value, label: v.label}))} value={view} onChange={onView} />
    </View>
  );
}

function DiscoverView({libraryId}: {libraryId: string}) {
  const m = useMetrics();
  const rows = useQuery(c => c.libraries.discover(libraryId), [libraryId], ['personal', 'libraries']);
  const cards = useCardHandlers();
  const cont = useCardHandlers('continue');
  if (!rows.data) return <View style={{gap: m.rowGap}}><ShelfSkeleton /><ShelfSkeleton /></View>;
  if (!rows.data.length) return <EmptyState icon="folder" title="Nothing in this library yet" body="When the server finishes scanning this library’s folders, its titles appear here." />;
  return (
    <View style={{gap: m.rowGap}}>
      {rows.data.map(r => {
        const h = r.kind === 'continue' ? cont : cards;
        const seeAll = r.id === 'collections' ? undefined : r.id.startsWith('g-') ? () => push('Entity', {libraryId, kind: 'genre', id: r.title, title: r.title}) : r.kind === 'recent' ? () => root('Library', {libraryId, view: 'browse'}) : undefined;
        return <Shelf key={r.id} title={r.title} entries={r.entries} shape={r.shape} total={r.total} onPress={h.onPress} onLongPress={h.onLongPress} onFocus={h.onFocus} onSeeAll={seeAll} />;
      })}
    </View>
  );
}

function CollectionsView({libraryId}: {libraryId: string}) {
  const m = useMetrics();
  const cols = useQuery(c => c.libraries.collections(libraryId), [libraryId]);
  const cards = useCardHandlers();
  const [width, onLayout] = useContentWidth();
  if (!cols.data) return <ShelfSkeleton />;
  if (!cols.data.length) return <EmptyState icon="collection" title="No collections yet" body="Collections group related titles, like a film series. Owners create them from any title’s menu with Add to collection." />;
  const ncol = m.isTV ? 5 : m.isPhone ? 2 : 4;
  const cw = Math.floor((width - m.gutter * 2 - m.gap * (ncol - 1)) / ncol);
  return (
    <View onLayout={onLayout} style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap, paddingHorizontal: m.gutter}}>
      {cols.data.map(c => <CollectionCard key={c.id} entry={c} width={cw} onPress={cards.onPress} onLongPress={cards.onLongPress} />)}
    </View>
  );
}

/** A collection card: a fanned stack of its first posters over a soft backdrop. */
export function CollectionCard({entry, width, onPress, onLongPress}: {entry: Entry; width: number; onPress: (e: Entry) => void; onLongPress?: (e: Entry) => void}) {
  const m = useMetrics();
  const h = Math.round(width * 0.62);
  const posters = (entry.stack?.length ? entry.stack : [entry.image]).slice(0, 3);
  const ph = Math.round(h * 0.78);
  const pw = Math.round((ph * 2) / 3);
  const step = Math.round(pw * 0.42);
  const total = pw + step * (posters.length - 1);
  const left = Math.round((width - total) / 2);
  return (
    <Focusable onPress={() => onPress(entry)} onLongPress={() => onLongPress?.(entry)} focusScale={m.isTV ? 1.05 : 1} style={{width}} accessibilityLabel={`${entry.title}, collection, ${entry.subtitle ?? ''}`}>
      {({focused}) => (
        <View style={{gap: m.isTV ? 12 : 8}}>
          <View style={{height: h}}>
            <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, borderRadius: m.radius.card, overflow: 'hidden', backgroundColor: colors.raised}}>
              <Artwork source={entry.backdrop ?? entry.image} title={entry.title} width={width} height={h} radius={0} style={{opacity: 0.5}} imageStyle={{transform: [{scale: 1.15}]}} blur={m.isTV ? 18 : 12} />
              <View style={{position: 'absolute', left: 0, right: 0, top: 0, bottom: 0, backgroundColor: 'rgba(7,16,24,0.35)'}} />
            </View>
            {posters.map((p, i) => {
              const k = posters.length - 1 - i; // draw the back posters first
              const scale = 1 - k * 0.08;
              return (
                <View key={k} style={{position: 'absolute', left: left + step * k + (pw * (1 - scale)) / 2, top: (h - ph * scale) / 2, zIndex: 3 - k, shadowColor: '#000', shadowOpacity: 0.5, shadowRadius: 10, shadowOffset: {width: 0, height: 4}}}>
                  <Artwork source={posters[k]} title={entry.title} width={Math.round(pw * scale)} height={Math.round(ph * scale)} radius={m.radius.small} style={k ? {opacity: 0.92} : undefined} />
                </View>
              );
            })}
            {focused ? <View pointerEvents="none" style={{position: 'absolute', left: -5, right: -5, top: -5, bottom: -5, borderRadius: m.radius.card + 5, borderWidth: 3, borderColor: colors.focus, shadowColor: colors.accent, shadowOpacity: 0.9, shadowRadius: 22, shadowOffset: {width: 0, height: 0}}} /> : null}
          </View>
          <View style={{paddingHorizontal: 2}}>
            <Text variant="captionStrong" numberOfLines={1}>{entry.title}</Text>
            <Text variant="caption" tone="tertiary" numberOfLines={1}>{entry.subtitle}</Text>
          </View>
        </View>
      )}
    </Focusable>
  );
}

function CategoriesView({libraryId}: {libraryId: string; shape: Shape}) {
  const m = useMetrics();
  const cats = useQuery(c => c.libraries.categories(libraryId), [libraryId]);
  const [width, onLayout] = useContentWidth();
  if (!cats.data) return <ShelfSkeleton shape="wide" />;
  const groups = (['genre', 'decade', 'studio', 'network'] as const).map(k => ({k, list: cats.data!.filter(c => c.kind === k)})).filter(g => g.list.length);
  const cols = m.isTV ? 4 : m.isPhone ? 2 : 3;
  const tw = Math.floor((width - m.gutter * 2 - m.gap * (cols - 1)) / cols);
  return (
    <View onLayout={onLayout} style={{gap: m.rowGap}}>
      {groups.map(g => (
        <View key={g.k} style={{gap: m.space.md}}>
          <Text variant="heading" style={{paddingHorizontal: m.gutter}}>{{genre: 'Genres', decade: 'Decades', studio: 'Studios', network: 'Networks'}[g.k]}</Text>
          <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.gap, paddingHorizontal: m.gutter}}>
            {g.list.map(c => <CategoryTile key={c.id} category={c} width={tw} onPress={() => push('Entity', {libraryId, kind: c.kind, id: c.title, title: c.title})} />)}
          </View>
        </View>
      ))}
    </View>
  );
}

function BrowseView({lib, libraryId, view, shape, header}: {lib?: Library; libraryId: string; view: LibraryView; shape: Shape; header: React.ReactElement}) {
  const client = useClient();
  const m = useMetrics();
  const [query, setQuery] = useState<BrowseQuery>({sort: 'title', direction: 'asc', filters: {}});
  const [entries, setEntries] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [layout, setLayout] = useState<'grid' | 'list'>('grid');
  const [sheet, setSheet] = useState<'filters' | 'sort' | 'save'>();
  const [width, onLayout] = useContentWidth();
  const grid = useRef<GridHandle>(null);
  const seq = useRef(0);
  const cards = useCardHandlers();
  const load = useCallback(async (offset: number) => {
    const n = ++seq.current;
    setLoading(true);
    const page = await client.libraries.browse(libraryId, query, offset, PAGE, view);
    if (n !== seq.current) return;
    setEntries(prev => (offset === 0 ? page.entries : [...prev, ...page.entries]));
    setTotal(page.total);
    setLoading(false);
  }, [client, libraryId, query, view]);
  useEffect(() => { void load(0); }, [load]);
  useEffect(() => client.subscribe('personal', () => void load(0)), [client, load]);
  const letters = useQuery(c => c.libraries.letters(libraryId, query, view), [libraryId, query, view]);
  const jump = async (letter: string, offset: number) => {
    if (offset >= entries.length) {
      setLoading(true);
      const page = await client.libraries.browse(libraryId, query, entries.length, offset - entries.length + PAGE, view);
      setEntries(prev => [...prev, ...page.entries]);
      setLoading(false);
      setTimeout(() => grid.current?.scrollToIndex(offset), 60);
    } else grid.current?.scrollToIndex(offset);
  };
  const activeCount = Object.entries(query.filters).filter(([, v]) => (Array.isArray(v) ? v.length : v !== undefined)).length;
  const sort = SORTS.find(s => s.value === query.sort)!;
  const showLetters = query.sort === 'title' && !m.isPhone && total > 40 && letters.data && letters.data.length > 3;
  const video = lib?.kind === 'movie' || lib?.kind === 'tv' || lib?.kind === 'anime';
  const noun = lib?.kind === 'music' && view === 'browse' ? 'artists' : lib?.kind === 'photo' ? 'albums' : lib?.kind === 'audiobook' ? 'books' : lib?.kind === 'homevideo' ? 'videos' : 'titles';
  const bar = (
    <View style={{paddingHorizontal: m.gutter, gap: m.space.sm, marginBottom: m.space.lg}}>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: m.space.sm}}>
        <Button size="sm" icon="filter" label={activeCount ? `Filters · ${activeCount}` : 'Filters'} variant={activeCount ? 'primary' : 'secondary'} onPress={() => setSheet('filters')} />
        <Button size="sm" icon="sort" label={`${sort.label} · ${dirLabel(sort.kind, query.direction)}`} variant="secondary" onPress={() => setSheet('sort')} />
        {!['albums', 'timeline'].includes(view) && lib?.kind !== 'photo' ? <Segmented value={layout} onChange={setLayout} options={[{value: 'grid', label: m.isPhone ? '' : 'Grid', icon: 'gridView'}, {value: 'list', label: m.isPhone ? '' : 'List', icon: 'list'}]} /> : null}
        <Button size="sm" icon="saved" label={m.isPhone ? undefined : 'Save view'} accessibilityLabel="Save view" variant="ghost" onPress={() => setSheet('save')} />
        <View style={{flex: 1}} />
        <Text variant="caption" tone="tertiary">{total} {total === 1 ? noun.replace(/s$/, '') : noun}</Text>
      </View>
      {video && !['authors', 'series', 'timeline'].includes(view) ? (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: m.space.xs, paddingVertical: m.isTV ? 8 : 0}}>
          {QUICK.map(q => <Chip key={q.id} label={q.label} selected={q.on(query.filters)} onPress={() => setQuery(prev => ({...prev, filters: q.apply(prev.filters)}))} />)}
          {query.filters.genre?.map(g => <Chip key={g} label={g} selected onClear={() => setQuery(prev => ({...prev, filters: {...prev.filters, genre: prev.filters.genre?.filter(x => x !== g)}}))} onPress={() => setQuery(prev => ({...prev, filters: {...prev.filters, genre: prev.filters.genre?.filter(x => x !== g)}}))} />)}
          {query.filters.decade?.map(g => <Chip key={g} label={g} selected onClear={() => setQuery(prev => ({...prev, filters: {...prev.filters, decade: prev.filters.decade?.filter(x => x !== g)}}))} onPress={() => setQuery(prev => ({...prev, filters: {...prev.filters, decade: prev.filters.decade?.filter(x => x !== g)}}))} />)}
        </ScrollView>
      ) : null}
    </View>
  );
  const empty = activeCount ? <EmptyState icon="filter" title="No titles match" body="Loosen a filter or clear them all." action={{label: 'Clear filters', onPress: () => setQuery(prev => ({...prev, filters: {}}))}} /> : <EmptyState icon="folder" title="Nothing in this library yet" body="When the server finishes scanning this library’s folders, its titles appear here." />;
  return (
    <View style={{flex: 1, flexDirection: 'row'}} onLayout={onLayout}>
      <View style={{flex: 1}}>
        {layout === 'grid' || ['albums', 'timeline'].includes(view) ? (
          <EntryGrid ref={grid} width={width - (showLetters ? (m.isTV ? 92 : 30) : 0)} entries={entries} shape={shape} {...cards} loading={loading && entries.length === 0}
            header={<>{header}{bar}</>} empty={empty} onEndReached={() => !loading && entries.length < total && void load(entries.length)} />
        ) : (
          <ListBrowse entries={entries} header={<>{header}{bar}</>} empty={empty} onEnd={() => !loading && entries.length < total && void load(entries.length)} />
        )}
      </View>
      {showLetters ? <AlphabetRail letters={letters.data!} onJump={jump} /> : null}
      <FilterSheet visible={sheet === 'filters'} onClose={() => setSheet(undefined)} libraryId={libraryId} filters={query.filters} onChange={f => setQuery(prev => ({...prev, filters: f}))} total={total} />
      <SortSheet visible={sheet === 'sort'} onClose={() => setSheet(undefined)} query={query} onChange={(sortField, direction) => setQuery(prev => ({...prev, sort: sortField, direction}))} />
      <SaveViewSheet visible={sheet === 'save'} onClose={() => setSheet(undefined)} libraryId={libraryId} query={query} />
    </View>
  );
}

function ListBrowse({entries, header, empty, onEnd}: {entries: Entry[]; header: React.ReactElement; empty: React.ReactElement; onEnd: () => void}) {
  useRegisterNav();
  const m = useMetrics();
  const chrome = useChromeInsets();
  const cards = useCardHandlers();
  const content = (
    <FlatList data={entries} keyExtractor={e => e.id} ListHeaderComponent={header} ListEmptyComponent={empty} onEndReached={onEnd} onEndReachedThreshold={1.5}
      contentContainerStyle={{paddingTop: chrome.top, paddingBottom: chrome.bottom}}
      renderItem={({item}) => (
        <View style={{paddingHorizontal: m.gutter}}>
          <ListRow left={<Artwork source={item.image} title={item.title} width={item.shape === 'wide' ? (m.isTV ? 128 : 80) : m.isTV ? 64 : 42} height={item.shape === 'poster' ? (m.isTV ? 96 : 63) : m.isTV ? 72 : 45} radius={item.shape === 'circle' ? 40 : 6} />}
            title={item.title} subtitle={[item.subtitle, item.duration ? formatDuration(item.duration) : undefined, item.rating, item.genres?.slice(0, 2).join(', ')].filter(Boolean).join(' · ')}
            value={item.watched ? 'Watched' : item.progress ? `${Math.round(item.progress * 100)}%` : undefined} onPress={() => cards.onPress(item)} onLongPress={() => cards.onLongPress(item)} />
        </View>
      )} />
  );
  return m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView> : content;
}

/** Jump-to-letter rail on the right edge of Browse. */
function AlphabetRail({letters, onJump}: {letters: {letter: string; offset: number}[]; onJump: (letter: string, offset: number) => void}) {
  const m = useMetrics();
  const chrome = useChromeInsets();
  const all = '#ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');
  const top = chrome.top + (m.isTV ? 200 : 160);
  // Every letter fits without scrolling.
  const rowH = Math.max(m.isTV ? 22 : 14, Math.min(m.isTV ? 34 : 19, Math.floor((m.height - top - chrome.bottom) / all.length)));
  return (
    <View style={{width: m.isTV ? 56 : 30, marginRight: m.isTV ? 36 : 0, paddingTop: top, paddingBottom: chrome.bottom, alignItems: 'center'}}>
      <ScrollView showsVerticalScrollIndicator={false} contentContainerStyle={{alignItems: 'center'}}>
        {all.map(l => {
          const hit = letters.find(x => x.letter === l);
          return (
            <Focusable key={l} disabled={!hit} onPress={() => hit && onJump(l, hit.offset)} accessibilityLabel={`Jump to ${l}`}
              style={({focused}) => ({width: m.isTV ? 44 : 24, height: rowH, borderRadius: 8, alignItems: 'center', justifyContent: 'center', backgroundColor: focused ? colors.focus : 'transparent'})}>
              {({focused}) => <Text variant="micro" color={focused ? colors.accentInk : hit ? colors.accentStrong : colors.lineStrong}>{l}</Text>}
            </Focusable>
          );
        })}
      </ScrollView>
    </View>
  );
}

function FilterSheet({visible, onClose, libraryId, filters, onChange, total}: {visible: boolean; onClose: () => void; libraryId: string; filters: Filters; onChange: (f: Filters) => void; total: number}) {
  const m = useMetrics();
  const facets = useQuery(c => c.libraries.facets(libraryId), [libraryId]);
  const [find, setFind] = useState('');
  const toggle = (field: keyof Filters, value: string) => {
    const cur = (filters[field] as string[] | undefined) ?? [];
    onChange({...filters, [field]: cur.includes(value) ? cur.filter(v => v !== value) : [...cur, value]});
  };
  return (
    <Sheet visible={visible} onClose={onClose} title="Filters" subtitle={`Narrow this view · ${total} titles`} size="md" footer={<><Button label="Clear all" variant="ghost" onPress={() => onChange({})} /><Button label="Done" variant="primary" onPress={onClose} /></>}>
      <Text variant="captionStrong" tone="secondary">Play state</Text>
      <Segmented value={filters.playState ?? 'any'} onChange={v => onChange({...filters, playState: v === 'any' ? undefined : (v as Filters['playState'])})} options={[{value: 'any', label: 'Any'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}, {value: 'watched', label: 'Watched'}]} />
      <TextField icon="search" placeholder="Find a genre, decade or studio…" value={find} onChangeText={setFind} />
      {facets.data?.map(f => {
        const values = f.values.filter(v => !find || v.label.toLowerCase().includes(find.toLowerCase())).slice(0, 24);
        if (!values.length) return null;
        return (
          <View key={f.field} style={{gap: m.space.xs}}>
            <Text variant="captionStrong" tone="secondary">{f.label}</Text>
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: m.space.xs}}>
              {values.map(v => <Chip key={v.value} label={v.label} count={v.count || undefined} selected={((filters[f.field] as string[] | undefined) ?? []).includes(v.value)} onPress={() => toggle(f.field, v.value)} />)}
            </View>
          </View>
        );
      })}
    </Sheet>
  );
}

function SortSheet({visible, onClose, query, onChange}: {visible: boolean; onClose: () => void; query: BrowseQuery; onChange: (s: SortField, d: 'asc' | 'desc') => void}) {
  const sort = SORTS.find(s => s.value === query.sort)!;
  return (
    <Sheet visible={visible} onClose={onClose} title="Sort by" size="sm" footer={<Button label="Done" variant="primary" onPress={onClose} />}>
      <Segmented value={query.direction} onChange={d => onChange(query.sort, d)} options={[{value: 'asc', label: dirLabel(sort.kind, 'asc')}, {value: 'desc', label: dirLabel(sort.kind, 'desc')}]} />
      {SORTS.map(s => <ListRow key={s.value} title={s.label} selected={s.value === query.sort} chevron={false} onPress={() => onChange(s.value, s.kind === 'alpha' ? 'asc' : 'desc')} />)}
    </Sheet>
  );
}

function SaveViewSheet({visible, onClose, libraryId, query}: {visible: boolean; onClose: () => void; libraryId: string; query: BrowseQuery}) {
  const client = useClient();
  const toast = useToast();
  const [name, setName] = useState('');
  const {busy, run} = useAction();
  return (
    <Sheet visible={visible} onClose={onClose} title="Save this view" subtitle="The filters and sort you have now, kept under Saved." size="sm"
      footer={<><Button label="Cancel" variant="ghost" onPress={onClose} /><Button label="Save" variant="primary" disabled={!name.trim()} loading={busy} onPress={() => void run(async () => { const id = await client.saved.saveView(libraryId, name.trim(), query); onClose(); setName(''); toast({message: `Saved “${name.trim()}”.`, icon: 'saved', action: {label: 'View', onPress: () => push('Resource', {kind: 'view', id})}}); })} /></>}>
      <TextField label="Name" placeholder="Unwatched 90s action" value={name} onChangeText={setName} help="Saved views run on the server, so they stay current as the library changes." />
    </Sheet>
  );
}

function SongsView({libraryId, header}: {libraryId: string; header: React.ReactElement}) {
  useRegisterNav();
  const m = useMetrics();
  const chrome = useChromeInsets();
  const actions = useActions();
  const client = useClient();
  const [songs, setSongs] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const load = useCallback(async (offset: number) => {
    const page = await client.libraries.browse(libraryId, {sort: 'title', direction: 'asc', filters: {}}, offset, 80, 'songs');
    setSongs(prev => (offset ? [...prev, ...page.entries] : page.entries));
    setTotal(page.total);
  }, [client, libraryId]);
  useEffect(() => { void load(0); }, [load]);
  const list = useMemo(() => songs, [songs]);
  const content = (
    <FlatList data={list} keyExtractor={e => e.id} contentContainerStyle={{paddingTop: chrome.top, paddingBottom: chrome.bottom}}
      ListHeaderComponent={<>{header}<View style={{paddingHorizontal: m.gutter, marginBottom: 8, flexDirection: 'row', gap: 8}}><Button size="sm" icon="shuffle" label="Shuffle all" variant="primary" onPress={() => void actions.play(list[Math.floor(Math.random() * list.length)]!)} /><Text variant="caption" tone="tertiary" style={{alignSelf: 'center'}}>{total} songs</Text></View></>}
      onEndReached={() => songs.length < total && void load(songs.length)}
      renderItem={({item}) => (
        <View style={{paddingHorizontal: m.gutter}}>
          <ListRow left={<Artwork source={item.image} title={item.title} width={m.isTV ? 56 : 40} height={m.isTV ? 56 : 40} radius={4} />} title={item.title} subtitle={item.subtitle} value={formatTime(item.duration ?? 0)} onPress={() => void actions.play(item)} onLongPress={() => actions.more(item)} chevron={false} />
        </View>
      )} />
  );
  return m.isTV ? <TVFocusGuideView autoFocus style={{flex: 1}}>{content}</TVFocusGuideView> : content;
}
