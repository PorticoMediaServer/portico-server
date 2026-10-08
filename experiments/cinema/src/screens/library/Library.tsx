import React, {useCallback, useEffect, useMemo, useState} from 'react';
import {FlatList, ScrollView, StyleSheet, TVFocusGuideView, View} from 'react-native';
import {useRoute, type RouteProp} from '@react-navigation/native';
import {useQuery, useServices} from '../../data/hooks';
import type {BrowseQuery} from '../../data/services';
import type {FilterState, Item, Library, SortState, TrackItem} from '../../data/types';
import {Artwork, Scrim} from '../../design/Artwork';
import {Button} from '../../design/Button';
import {Card, aspectFor} from '../../design/Card';
import {Chip, ListRow, Segmented, Stepper, ToggleRow} from '../../design/Controls';
import {Focusable} from '../../design/Focusable';
import {Icon} from '../../design/Icon';
import {ui} from '../../design/overlays';
import {Row} from '../../design/Row';
import {Banner, EmptyState, ErrorState, GridSkeleton, RowsSkeleton} from '../../design/States';
import {Text} from '../../design/Text';
import {colors, isTV, radius, u} from '../../design/tokens';
import {useLayout} from '../../design/useLayout';
import {libraryIcon} from '../../nav/menus';
import {Screen, useChromeInsets, Header} from '../../nav/Screen';
import {go, goSection, push, type Routes} from '../../nav/routes';
import {player} from '../../state/player';
import {isManager, useSession} from '../../state/session';
import {openItemMenu} from '../shared/actions';
import {fmtClock, openItem, shapeFor, toCard} from '../shared/cards';
import {SelectionBar} from '../shared/SelectionBar';
import {useItems} from '../shared/useItems';
import {plural} from '../../design/format';

const viewLabel: Record<string, string> = {discover: 'Discover', browse: 'Browse', collections: 'Collections', categories: 'Categories', releases: 'Albums', songs: 'Songs', authors: 'Authors', series: 'Series', timeline: 'Timeline'};
const browseLabel = (l: Library) => (l.kind === 'music' ? 'Artists' : l.kind === 'photos' ? 'Albums' : 'Browse');

export function LibrariesScreen() {
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('libraries', () => s.catalog.libraries());
  const session = useSession();
  const cols = layout.compact ? 1 : isTV ? 3 : 2;
  const w = (layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * 14) / cols;
  if (q.data && !q.data.length) return <NoLibrariesScreen />;
  return (
    <Screen heading="Libraries" eyebrow={session.server?.name}>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 14}}>
        {(q.data ?? []).map((l, i) => <LibraryTile key={l.id} lib={l} width={w} first={i === 0} />)}
      </View>
      {!isTV ? (
        <View style={{marginTop: 20}}>
          <ListRow icon="download" label="Downloads" help="Saved on this device to watch offline" onPress={() => goSection('Downloads')} />
          <ListRow icon="together" label="Watch Together" help="Watch in step with others on this server" onPress={() => goSection('Together')} />
        </View>
      ) : null}
    </Screen>
  );
}

function LibraryTile({lib, width, first}: {lib: Library; width: number; first?: boolean}) {
  const s = useServices();
  const q = useQuery('libtile:' + lib.id, () => s.catalog.browse({libraryId: lib.id, filters: {quick: [], fields: {}}, sort: {field: 'added', direction: 'desc'}, pageSize: 4}));
  const lead = q.data?.items.find(i => i.art.backdrop) ?? q.data?.items[0];
  const art = lead?.art.backdrop ?? lead?.art.square ?? lead?.art.thumb;
  return (
    <Focusable onPress={() => goSection('Library', {libraryId: lib.id})} hasTVPreferredFocus={first} scale={1.05} accessibilityLabel={`${lib.name}, ${plural(lib.count, 'title', 'titles')}`}>
      {({focused}) => (
        <View style={[styles.tile, {width, height: width * 0.42}, focused && isTV && styles.tileFocus]}>
          <Artwork art={art} tint={lead?.tint} style={StyleSheet.absoluteFill} />
          <Scrim bottom={0.95} left={0.6} />
          <View style={styles.tileText}>
            <Icon name={libraryIcon[lib.kind]} size={20} color={colors.accentStrong} />
            <Text variant="display" style={{marginTop: 4}}>{lib.name}</Text>
            <Text variant="caption" tone="secondary">{lib.count} {lib.kind === 'music' ? 'artists' : lib.kind === 'photos' ? 'albums' : 'titles'}{lib.scanning ? ' · Scanning…' : ''}</Text>
          </View>
        </View>
      )}
    </Focusable>
  );
}

export function NoLibrariesScreen() {
  const route = useRoute<RouteProp<Routes, 'NoLibraries'>>();
  const session = useSession();
  const member = route.params?.member || !isManager(session.role);
  return (
    <Screen>
      <EmptyState
        icon="library"
        title={member ? 'Nothing’s been shared with you yet' : 'Add your first library'}
        body={member ? 'Ask the owner of this server to share a library with your profile.' : 'Point Portico at a folder of movies, shows, music or audiobooks. Your files stay where they are.'}
        action={member ? undefined : {label: 'Add library', icon: 'plus', onPress: () => go('Admin', {section: 'libraries'})}}
      />
    </Screen>
  );
}

// ---------------------------------------------------------------- Library

export function LibraryScreen() {
  const route = useRoute<RouteProp<Routes, 'Library'>>();
  const s = useServices();
  const session = useSession();
  const libs = useQuery('libraries', () => s.catalog.libraries());
  const layout = useLayout();
  const lib = libs.data?.find(l => l.id === route.params.libraryId);
  const [view, setView] = useState(route.params.view ?? lib?.views[0] ?? 'discover');
  useEffect(() => {
    if (route.params.view) setView(route.params.view);
    else if (lib && !lib.views.includes(view as never)) setView(lib.views[0]);
  }, [route.params.view, lib, view]);
  if (libs.error) return <Screen><ErrorState onRetry={libs.reload} /></Screen>;
  if (!lib) return <Screen><RowsSkeleton rows={3} /></Screen>;
  const manager = isManager(session.role);
  const switchLib = () => ui.menu({title: 'Switch library', actions: (libs.data ?? []).map(l => ({label: l.name, icon: libraryIcon[l.kind], checked: l.id === lib.id, detail: `${plural(l.count, 'title', 'titles')}`, onPress: () => goSection('Library', {libraryId: l.id})}))});
  const manage = () => ui.menu({title: lib.name, actions: [
    {label: 'Scan library files', icon: 'refresh', onPress: () => s.catalog.scanLibrary(lib.id).then(() => ui.toast('Scan started.'))},
    {label: 'Manage library', icon: 'settings', onPress: () => go('Admin', {section: 'library-settings'})},
  ]});
  const header = (
    <View style={{paddingHorizontal: view === 'discover' ? layout.gutter : 0}}>
      <Focusable onPress={switchLib} accessibilityRole="button" accessibilityLabel={`${lib.name}, switch library`} style={{alignSelf: 'flex-start'}}>
        {({focused}) => (
          <View style={focused && isTV ? {backgroundColor: 'rgba(255,255,255,0.14)', borderRadius: 16, paddingHorizontal: 12, marginHorizontal: -12, borderWidth: 3, borderColor: colors.white} : undefined}>
            <Text variant="eyebrow" tone="accent">{`${lib.kind === 'shows' ? 'TV' : lib.kind === 'homevideos' ? 'Home video' : lib.name.replace(/s$/, '')} library · ${lib.count}`}</Text>
            <View style={{flexDirection: 'row', alignItems: 'center'}}>
              <Text variant="hero">{lib.name}</Text>
              <View style={{marginLeft: 8, marginTop: 8}}><Icon name="chevronDown" size={18} color={colors.textTertiary} /></View>
            </View>
          </View>
        )}
      </Focusable>
      <View style={{flexDirection: 'row', alignItems: 'center', marginTop: u(10), gap: 10, flexWrap: 'wrap'}}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{gap: u(8)}}>
          {lib.views.map(v => <Chip key={v} label={v === 'browse' ? browseLabel(lib) : viewLabel[v] ?? v} active={v === view} hasTVPreferredFocus={v === view} onPress={() => setView(v)} />)}
        </ScrollView>
        {manager ? <Button icon="more" size="sm" accessibilityLabel="Library actions" onPress={manage} /> : null}
      </View>
      {lib.scanning ? <View style={{marginTop: u(12)}}><Banner icon="refresh" tone="info" title="Scanning your files…" body="Titles appear here as Portico finds them. 412 of 655 files checked." /></View> : null}
    </View>
  );
  if (view === 'discover') return <Discover lib={lib} header={header} />;
  return (
    <Screen title={lib.name} scroll={view !== 'browse' && view !== 'songs' && view !== 'timeline' && view !== 'releases'} padded>
      {view === 'browse' || view === 'releases' || view === 'timeline' ? <Browse lib={lib} kind={view === 'releases' ? 'releases' : undefined} header={header} timeline={view === 'timeline'} /> : null}
      {view === 'songs' ? <Songs lib={lib} header={header} /> : null}
      {view === 'collections' ? <>{header}<Collections lib={lib} /></> : null}
      {view === 'categories' ? <>{header}<Categories lib={lib} /></> : null}
      {view === 'authors' || view === 'series' ? <>{header}<Groups lib={lib} kind={view} /></> : null}
    </Screen>
  );
}

function Discover({lib, header}: {lib: Library; header: React.ReactNode}) {
  const s = useServices();
  const session = useSession();
  const chrome = useChromeInsets();
  const rows = useQuery('discover:' + lib.id, () => s.catalog.discover(lib.id));
  const ids = useMemo(() => [...new Set((rows.data ?? []).flatMap(r => r.itemIds))], [rows.data]);
  const items = useItems(rows.data ? ids : undefined, 'discover-items:' + lib.id + ids.length);
  const playlists = useQuery(lib.kind === 'music' ? 'playlists' : null, () => s.lists.playlists());
  const hero = (rows.data ?? [])[0]?.itemIds.map(id => items.lookup(id)).find(i => i?.art.backdrop);
  return (
    <View style={{flex: 1, paddingLeft: chrome.left}}>
      <TVFocusGuideView autoFocus style={{flex: 1}}>
        <ScrollView contentContainerStyle={{paddingTop: isTV ? 56 : 110, paddingBottom: chrome.bottom + 40}}>
          {header}
          <View style={{height: u(18)}} />
          {!rows.data || (items.loading && !items.map.size) ? <RowsSkeleton rows={3} shape={lib.kind === 'music' ? 'square' : 'poster'} /> : null}
          {rows.data && !rows.data.length ? <EmptyState icon={libraryIcon[lib.kind]} title="Nothing in this library yet" body="When the server finishes scanning this library’s folders, its titles appear here." /> : null}
          {(rows.data ?? []).map(row => (
            <Row key={row.id} title={row.title} subject={row.subject} items={row.shape === 'circle' ? items.cards(row.itemIds, {shape: 'circle'}) : items.cards(row.itemIds)}
              onPressItem={c => { const it = items.lookup(c.id); if (it) openItem(it); }}
              onLongPressItem={c => { const it = items.lookup(c.id); if (it) openItemMenu(s, it, {origin: row.recommendation ? 'recommendation' : row.kind === 'continue' ? 'continue' : 'default', manager: isManager(session.role)}); }}
              onSeeAll={() => push('HomeRow', {rowId: row.id})} />
          ))}
          {lib.kind === 'music' && playlists.data?.length ? (
            <Row title="Your playlists" items={playlists.data.map(p => ({id: p.id, title: p.title, subtitle: `${plural(p.itemIds.length, 'song', 'songs')}`, art: undefined, tint: p.tint, shape: 'square' as const}))} onPressItem={c => push('Resource', {kind: 'playlist', id: c.id})} />
          ) : null}
          {hero ? null : null}
        </ScrollView>
      </TVFocusGuideView>
      <View style={[StyleSheet.absoluteFill, {bottom: undefined, left: chrome.left}]} pointerEvents="box-none"><Header /></View>
    </View>
  );
}

// ---------------------------------------------------------------- Browse with filters, sort, alphabet jump and selection

const QUICK = [{id: 'unwatched', label: 'Unwatched'}, {id: 'inProgress', label: 'In progress'}, {id: 'recentlyAdded', label: 'Recently added'}, {id: 'favorites', label: 'Favorites'}, {id: 'watchlist', label: 'Watchlist'}, {id: 'hd', label: 'HD'}, {id: 'uhd', label: '4K'}, {id: 'watched', label: 'Watched'}];
const FIELDS: {id: string; label: string}[] = [{id: 'genre', label: 'Genre'}, {id: 'decade', label: 'Decade'}, {id: 'contentRating', label: 'Content rating'}, {id: 'mood', label: 'Mood'}, {id: 'resolution', label: 'Resolution'}, {id: 'language', label: 'Audio language'}, {id: 'studio', label: 'Studio'}, {id: 'network', label: 'Network'}];
const SORTS = [{id: 'title', label: 'Title'}, {id: 'year', label: 'Year'}, {id: 'added', label: 'Date added'}, {id: 'rating', label: 'Critic rating'}, {id: 'runtime', label: 'Duration'}, {id: 'played', label: 'Last played'}, {id: 'random', label: 'Random'}];
const LETTERS = '#ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

export function Browse({lib, kind, header, timeline}: {lib: Library; kind?: string; header?: React.ReactNode; timeline?: boolean}) {
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const [filters, setFilters] = useState<FilterState>({quick: [], fields: {}});
  const [sort, setSort] = useState<SortState>({field: timeline ? 'year' : 'title', direction: timeline ? 'desc' : 'asc'});
  const [mode, setMode] = useState<'grid' | 'list'>('grid');
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [pages, setPages] = useState(1);
  const q: BrowseQuery = {libraryId: lib.id, filters, sort, kind, pageSize: 60 * pages};
  const key = 'browse:' + JSON.stringify(q);
  const res = useQuery(key, () => s.catalog.browse(q));
  const letters = useQuery('letters:' + JSON.stringify({...q, pageSize: 0}), () => s.catalog.letters({...q, pageSize: 10000}));
  const listRef = React.useRef<FlatList<Item>>(null);
  const filterCount = filters.quick.length + Object.values(filters.fields).reduce((a, v) => a + v.length, 0) + (filters.yearFrom ? 1 : 0);
  const sample = res.data?.items[0];
  const shape = sample ? shapeFor(sample) : lib.kind === 'music' ? 'square' : lib.kind === 'homevideos' || lib.kind === 'photos' ? 'landscape' : 'poster';
  const cardShape = kind === undefined && lib.kind === 'music' ? 'circle' : shape;
  const cols = isTV ? (shape === 'landscape' ? 4 : 7) : layout.compact ? (shape === 'landscape' ? 1 : shape === 'square' ? 2 : 3) : shape === 'landscape' ? 3 : 5;
  const gap = isTV ? 36 : 12;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * gap - (layout.compact ? 16 : 0)) / cols);
  const toggleSel = (id: string) => setSelected(prev => {
    const n = new Set(prev);
    if (n.has(id)) n.delete(id); else n.add(id);
    return n;
  });
  const jump = (letter: string) => {
    const hit = letters.data?.find(l => l.letter === letter);
    if (!hit) return;
    if (hit.index >= (res.data?.items.length ?? 0)) setPages(Math.ceil((hit.index + 1) / 60) + 1);
    setTimeout(() => listRef.current?.scrollToIndex({index: Math.floor(hit.index / (mode === 'grid' ? cols : 1)), animated: true, viewPosition: 0}), 250);
  };
  const toolbar = (
    <View style={styles.toolbar}>
      <Button icon="filter" label={filterCount ? `Filters · ${filterCount}` : 'Filters'} size="sm" active={filterCount > 0} onPress={() => openFilters(s, lib, filters, setFilters)} />
      <Button icon="sort" label={SORTS.find(x => x.id === sort.field)?.label ?? 'Sort'} size="sm" onPress={() => openSort(sort, setSort)} />
      <Segmented value={mode} onChange={setMode} options={[{value: 'grid', label: isTV ? 'Grid' : '', icon: 'grid'}, {value: 'list', label: isTV ? 'List' : '', icon: 'list'}]} />
      <Button icon="checkCircle" label={selecting ? 'Done' : 'Select'} size="sm" active={selecting} onPress={() => { setSelecting(!selecting); setSelected(new Set()); }} />
      <Button icon="save" label="Save view" size="sm" onPress={() => ui.prompt({title: 'Save view', body: 'Saved views rerun on the server every time you open them. Find them in Saved › Saved views.', fields: [{key: 'title', label: 'Name', placeholder: 'Unwatched thrillers'}], submitLabel: 'Save view', onSubmit: v => { if (!v.title.trim()) return 'Enter a name.'; s.lists.saveView({title: v.title.trim(), libraryId: lib.id, filters, sort, shared: false, onHome: false}).then(() => ui.toast('View saved.', {action: {label: 'Open', onPress: () => go('Saved', {tab: 'views'})}})); return null; }})} />
      {res.data ? <Text variant="caption" tone="tertiary" style={{marginLeft: 4}}>{res.data.total} {res.data.total === 1 ? 'title' : 'titles'}</Text> : null}
    </View>
  );
  const chips = filterCount ? (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginBottom: u(12)}}>
      {filters.quick.map(qk => <Chip key={qk} label={QUICK.find(x => x.id === qk)?.label ?? qk} icon="x" active onPress={() => setFilters({...filters, quick: filters.quick.filter(x => x !== qk)})} />)}
      {Object.entries(filters.fields).flatMap(([f, vals]) => vals.map(v => <Chip key={f + v} label={v} icon="x" active onPress={() => setFilters({...filters, fields: {...filters.fields, [f]: vals.filter(x => x !== v)}})} />))}
      <Button label="Clear all" size="sm" variant="ghost" onPress={() => setFilters({quick: [], fields: {}})} />
    </View>
  ) : null;
  const renderItem = ({item}: {item: Item}) => {
    const c = {...toCard(item, s.user.state(item.id), {shape: cardShape === 'circle' ? 'circle' : undefined})};
    const press = () => (selecting ? toggleSel(item.id) : openItem(item));
    const long = () => (selecting ? toggleSel(item.id) : openItemMenu(s, item, {manager: isManager(session.role), onSelect: () => { setSelecting(true); setSelected(new Set([item.id])); }}));
    if (mode === 'list') {
      return (
        <Focusable onPress={press} onLongPress={long} accessibilityLabel={item.title}>
          {({focused}) => (
            <View style={[styles.listRow, focused && isTV && {backgroundColor: 'rgba(255,255,255,0.12)'}]}>
              {selecting ? <View style={[styles.check, selected.has(item.id) && styles.checkOn]}>{selected.has(item.id) ? <Icon name="check" size={12} color={colors.actionInk} /> : null}</View> : null}
              <Artwork art={c.art} title={item.title} tint={item.tint} style={{width: u(46), height: u(46) * aspectFor(c.shape === 'landscape' ? 'landscape' : c.shape === 'poster' ? 'poster' : 'square'), borderRadius: c.shape === 'circle' ? 99 : 6}} fallbackVariant="caption" />
              <View style={{flex: 1, marginLeft: 12}}>
                <Text variant="bodyStrong" numberOfLines={1}>{item.title}</Text>
                <Text variant="caption" tone="tertiary" numberOfLines={1}>{c.subtitle}</Text>
              </View>
              {s.user.state(item.id).watched ? <Icon name="checkCircle" size={16} color={colors.accentStrong} /> : null}
            </View>
          )}
        </Focusable>
      );
    }
    return <Card item={c} width={w} selecting={selecting} selected={selected.has(item.id)} showTitle="always" onPress={press} onLongPress={long} style={{marginBottom: gap, marginRight: gap}} />;
  };
  const items = res.data?.items ?? [];
  return (
    <View style={{flex: 1}}>
      <FlatList
        ref={listRef}
        key={mode + cols}
        data={items}
        numColumns={mode === 'grid' ? cols : 1}
        keyExtractor={i => i.id}
        renderItem={renderItem}
        onScrollToIndexFailed={() => {}}
        ListHeaderComponent={<View style={{marginBottom: u(12)}}>{header}<View style={{height: u(12)}} />{toolbar}{chips}</View>}
        ListEmptyComponent={res.loading ? <GridSkeleton columns={cols} width={w} aspect={aspectFor(shape)} /> : filterCount ? <EmptyState icon="filter" title="No titles match" body="Loosen a filter or clear them all." action={{label: 'Clear filters', onPress: () => setFilters({quick: [], fields: {}})}} /> : <EmptyState icon="film" title="Nothing in this library yet" body="Titles appear here as the server scans this library’s folders." />}
        onEndReached={() => res.data?.cursor && setPages(p => p + 1)}
        onEndReachedThreshold={0.6}
        // On TV the list reaches 24pt into the gutter so a focused card's lift isn't clipped.
        style={isTV ? {marginLeft: -24} : undefined}
        contentContainerStyle={{paddingBottom: 140, paddingRight: layout.compact ? 18 : 0, paddingLeft: isTV ? 24 : 0}}
        showsVerticalScrollIndicator={false}
        initialNumToRender={isTV ? 21 : 18}
        windowSize={7}
      />
      {sort.field === 'title' && !timeline ? (
        <View style={[styles.letters, isTV ? styles.lettersTV : null]} pointerEvents="box-none">
          {LETTERS.map(l => {
            const has = letters.data?.some(x => x.letter === l);
            return (
              <Focusable key={l} onPress={() => jump(l)} disabled={!has} scale={1.3} accessibilityLabel={`Jump to ${l}`}>
                {({focused}) => <Text variant="micro" color={focused && isTV ? colors.white : has ? colors.accentStrong : colors.lineStrong} style={{paddingVertical: isTV ? 2 : 1, paddingHorizontal: 4, textAlign: 'center'}}>{l}</Text>}
              </Focusable>
            );
          })}
        </View>
      ) : null}
      {selecting ? <SelectionBar ids={[...selected]} total={res.data?.total ?? 0} onSelectAll={() => setSelected(new Set(items.map(i => i.id)))} onDone={() => { setSelecting(false); setSelected(new Set()); }} /> : null}
    </View>
  );
}

function openFilters(s: ReturnType<typeof useServices>, lib: Library, current: FilterState, apply: (f: FilterState) => void) {
  ui.sheet({title: 'Filters', subtitle: lib.name, wide: true, render: close => <FiltersBody s={s} lib={lib} current={current} apply={f => { apply(f); close(); }} />});
}

function FiltersBody({s, lib, current, apply}: {s: ReturnType<typeof useServices>; lib: Library; current: FilterState; apply: (f: FilterState) => void}) {
  const facets = useQuery('facets:' + lib.id, () => s.catalog.facets(lib.id));
  const [f, setF] = useState<FilterState>(current);
  const [open, setOpen] = useState<string | null>('genre');
  const toggleQuick = (id: string) => setF({...f, quick: f.quick.includes(id) ? f.quick.filter(x => x !== id) : [...f.quick, id]});
  const toggleField = (field: string, v: string) => {
    const vals = f.fields[field] ?? [];
    setF({...f, fields: {...f.fields, [field]: vals.includes(v) ? vals.filter(x => x !== v) : [...vals, v]}});
  };
  return (
    <View>
      <Text variant="eyebrow" tone="tertiary" style={{marginBottom: 8}}>Quick filters</Text>
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8}}>
        {QUICK.map((qf, i) => <Chip key={qf.id} label={qf.label} active={f.quick.includes(qf.id)} onPress={() => toggleQuick(qf.id)} hasTVPreferredFocus={i === 0} />)}
      </View>
      {FIELDS.filter(fl => (facets.data?.[fl.id] ?? []).length > 1).map(fl => (
        <View key={fl.id} style={{marginTop: u(14)}}>
          <ListRow label={fl.label} value={(f.fields[fl.id] ?? []).join(', ') || 'Any'} onPress={() => setOpen(open === fl.id ? null : fl.id)} chevron compact />
          {open === fl.id ? (
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 8, paddingHorizontal: u(14), marginTop: 6}}>
              {(facets.data?.[fl.id] ?? []).slice(0, 30).map(o => <Chip key={o.value} label={o.value} count={o.count} active={(f.fields[fl.id] ?? []).includes(o.value)} onPress={() => toggleField(fl.id, o.value)} />)}
            </View>
          ) : null}
        </View>
      ))}
      <View style={{marginTop: u(14)}}>
        <Stepper label="From year" value={f.yearFrom ?? 1900} min={1900} max={2026} step={5} onChange={v => setF({...f, yearFrom: v === 1900 ? undefined : v})} />
        <Stepper label="To year" value={f.yearTo ?? 2026} min={1900} max={2026} step={5} onChange={v => setF({...f, yearTo: v === 2026 ? undefined : v})} />
      </View>
      <View style={{flexDirection: 'row', gap: 10, marginTop: u(16), justifyContent: 'flex-end'}}>
        <Button label="Clear all" onPress={() => setF({quick: [], fields: {}})} />
        <Button label="Apply" variant="primary" onPress={() => apply(f)} />
      </View>
    </View>
  );
}

function openSort(sort: SortState, setSort: (s: SortState) => void) {
  ui.menu({title: 'Sort by', actions: [
    ...SORTS.map(o => ({label: o.label, checked: sort.field === o.id, onPress: () => setSort({...sort, field: o.id})})),
    {label: sort.direction === 'asc' ? 'Order: Ascending' : 'Order: Descending', icon: 'sort', onPress: () => setSort({...sort, direction: sort.direction === 'asc' ? 'desc' : 'asc'})},
  ]});
}

function Songs({lib, header}: {lib: Library; header: React.ReactNode}) {
  const s = useServices();
  const res = useQuery('songs:' + lib.id, () => s.catalog.browse({libraryId: lib.id, kind: 'songs', filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, pageSize: 500}));
  const tracks = (res.data?.items ?? []) as TrackItem[];
  return (
    <FlatList
      data={tracks}
      keyExtractor={t => t.id}
      ListHeaderComponent={<View style={{marginBottom: 12}}>{header}<View style={{flexDirection: 'row', gap: 10, marginTop: 14}}><Button label="Play all" icon="play" variant="primary" onPress={() => { player.play(tracks.map(t => t.id), {kind: 'audio'}); go('NowPlaying'); }} /><Button label="Shuffle" icon="shuffle" onPress={() => { player.play(tracks.map(t => t.id), {kind: 'audio', shuffle: true}); go('NowPlaying'); }} /></View></View>}
      renderItem={({item: t, index}) => (
        <ListRow label={t.title} help={`${t.artistName} · ${t.albumTitle}`} value={fmtClock(t.duration)} chevron={false} compact onPress={() => { player.play(tracks.map(x => x.id), {kind: 'audio', index}); go('NowPlaying'); }} />
      )}
      contentContainerStyle={{paddingBottom: 140}}
      initialNumToRender={20}
    />
  );
}

function Collections({lib}: {lib: Library}) {
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('collections:' + lib.id, () => s.catalog.collections(lib.id));
  const items = useItems(q.data?.map(c => c.itemIds[0]).filter(Boolean));
  const cols = isTV ? 3 : layout.compact ? 1 : 2;
  const w = (layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * 14) / cols;
  if (q.data && !q.data.length) return <EmptyState icon="layers" title="No collections yet" body="Collections group related titles, like a film series. Owners create them from any title’s menu with Add to collection." />;
  return (
    <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 14, marginTop: u(16)}}>
      {(q.data ?? []).map((c, i) => {
        const lead = items.lookup(c.itemIds[0]);
        return (
          <Focusable key={c.id} onPress={() => push('Entity', {libraryId: lib.id, view: 'collection', entityId: c.id})} hasTVPreferredFocus={i === 0} scale={1.05} accessibilityLabel={`${c.title}, ${plural(c.itemIds.length, 'title', 'titles')}`}>
            {({focused}) => (
              <View style={[styles.tile, {width: w, height: w * 0.5}, focused && isTV && styles.tileFocus]}>
                <Artwork art={lead?.art.backdrop ?? lead?.art.poster} tint={c.tint} style={StyleSheet.absoluteFill} />
                <Scrim bottom={0.95} left={0.5} />
                <View style={styles.tileText}>
                  <Text variant="eyebrow" tone="accent">{c.owner === 'me' ? 'Your collection' : 'Collection'} · {c.itemIds.length}</Text>
                  <Text variant="display" numberOfLines={1}>{c.title}</Text>
                  <Text variant="caption" tone="secondary" numberOfLines={1}>{c.description}</Text>
                </View>
              </View>
            )}
          </Focusable>
        );
      })}
    </View>
  );
}

function Categories({lib}: {lib: Library}) {
  const s = useServices();
  const layout = useLayout();
  const q = useQuery('categories:' + lib.id, () => s.catalog.categories(lib.id));
  const groups = ['genre', 'mood', 'decade', 'studio', 'network'] as const;
  const cols = isTV ? 5 : layout.compact ? 2 : 4;
  const w = (layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * 12) / cols;
  if (!q.data) return <GridSkeleton columns={cols} width={w} aspect={0.56} />;
  return (
    <View style={{marginTop: u(10)}}>
      {groups.map(g => {
        const list = q.data!.filter(c => c.kind === g);
        if (list.length < 2) return null;
        return (
          <View key={g} style={{marginTop: u(16)}}>
            <Text variant="title" style={{marginBottom: u(10)}}>{{genre: 'Genres', mood: 'Moods', decade: 'Decades', studio: 'Studios', network: 'Networks', country: 'Countries'}[g]}</Text>
            <View style={{flexDirection: 'row', flexWrap: 'wrap', gap: 12}}>
              {list.slice(0, isTV ? 15 : 12).map(c => (
                <Focusable key={c.id} onPress={() => push('Entity', {libraryId: lib.id, view: 'categories', entityId: c.id})} scale={1.06} accessibilityLabel={`${c.title}, ${plural(c.count, 'title', 'titles')}`}>
                  {({focused}) => (
                    <View style={[styles.cat, {width: w, height: w * 0.56}, focused && isTV && styles.tileFocus]}>
                      <Artwork art={c.art} tint={c.tint} style={StyleSheet.absoluteFill} />
                      <View style={[StyleSheet.absoluteFill, {backgroundColor: c.tint, opacity: 0.35}]} />
                      <Scrim bottom={0.85} />
                      <View style={{position: 'absolute', left: u(10), bottom: u(8), right: u(10)}}>
                        <Text variant="heading" numberOfLines={1}>{c.title}</Text>
                        <Text variant="micro" tone="secondary">{c.count} titles</Text>
                      </View>
                    </View>
                  )}
                </Focusable>
              ))}
            </View>
          </View>
        );
      })}
    </View>
  );
}

function Groups({lib, kind}: {lib: Library; kind: 'authors' | 'series'}) {
  const s = useServices();
  const res = useQuery('browse-all:' + lib.id, () => s.catalog.browse({libraryId: lib.id, filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, pageSize: 1000}));
  const books = (res.data?.items ?? []) as import('../../data/types').Audiobook[];
  const groups = new Map<string, typeof books>();
  books.forEach(b => {
    const k = kind === 'authors' ? b.author : b.series?.name;
    if (k) groups.set(k, [...(groups.get(k) ?? []), b]);
  });
  return (
    <View style={{marginTop: u(14)}}>
      {[...groups.entries()].sort((a, b) => a[0].localeCompare(b[0])).map(([name, list], i) => (
        <ListRow key={name} icon={kind === 'authors' ? 'user' : 'layers'} label={name} help={`${list.length} ${list.length === 1 ? 'book' : 'books'} · ${list.map(b => b.title).slice(0, 2).join(', ')}`} hasTVPreferredFocus={i === 0} onPress={() => push('Entity', {libraryId: lib.id, view: kind, entityId: `${kind === 'authors' ? 'author' : 'series'}:${name}`})} />
      ))}
      {!groups.size && res.data ? <EmptyState icon="book" title={kind === 'series' ? 'No series yet' : 'No authors yet'} /> : null}
    </View>
  );
}

// ---------------------------------------------------------------- Entity (genre, studio, collection, author, series…)

export function EntityScreen() {
  const route = useRoute<RouteProp<Routes, 'Entity'>>();
  const s = useServices();
  const session = useSession();
  const layout = useLayout();
  const {libraryId, view, entityId} = route.params;
  const q = useQuery(`entity:${libraryId}:${view}:${entityId}`, () => s.catalog.entity(libraryId, view, entityId));
  const items = useItems(q.data?.itemIds);
  const list = (q.data?.itemIds ?? []).map(id => items.lookup(id)).filter(Boolean) as Item[];
  const shape = list[0] ? shapeFor(list[0]) : 'poster';
  const cols = isTV ? (shape === 'landscape' ? 4 : 7) : layout.compact ? 3 : 5;
  const gap = isTV ? 36 : 12;
  const w = Math.floor((layout.width - layout.railWidth - layout.gutter * 2 - (cols - 1) * gap) / cols);
  const playable = list.filter(i => i.kind === 'movie' || i.kind === 'audiobook' || i.kind === 'album');
  return (
    <Screen title={q.data?.title}>
      {q.data ? (
        <View style={styles.entityHero}>
          <Artwork art={q.data.art} tint={q.data.tint} style={StyleSheet.absoluteFill} />
          <Scrim bottom={0.95} left={0.7} />
          <View style={{padding: u(18)}}>
            <Text variant="eyebrow" tone="accent">{q.data.subtitle}</Text>
            <Text variant="hero" numberOfLines={2}>{q.data.title}</Text>
            <View style={{flexDirection: 'row', gap: 10, marginTop: u(12)}}>
              {playable.length ? <Button label="Play" icon="play" variant="primary" hasTVPreferredFocus onPress={() => { const it = playable[0]; if (it.kind === 'movie') go('Player', {id: it.id, queue: playable.filter(x => x.kind === 'movie').map(x => x.id)}); else openItem(it); }} /> : null}
              {playable.length > 1 ? <Button label="Shuffle" icon="shuffle" onPress={() => { const it = playable[Math.floor(Math.random() * playable.length)]; openItem(it); }} /> : null}
              {!isTV ? <Button label="Download" icon="download" onPress={() => s.downloads.add(list.slice(0, 5).map(i => i.id), '1080p').then(n => ui.toast(`${plural(n, 'title', 'titles')} added to your downloads.`))} /> : null}
            </View>
          </View>
        </View>
      ) : <RowsSkeleton rows={1} />}
      {q.data && !list.length && !items.loading ? <EmptyState icon="compass" title="Nothing here" body="The server has no items for this page yet." /> : null}
      <View style={{flexDirection: 'row', flexWrap: 'wrap', gap, marginTop: u(18)}}>
        {list.map(it => <Card key={it.id} item={toCard(it, s.user.state(it.id))} width={w} showTitle="always" onPress={() => openItem(it)} onLongPress={() => openItemMenu(s, it, {manager: isManager(session.role)})} />)}
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  tile: {borderRadius: radius.lg, overflow: 'hidden', backgroundColor: colors.raised, justifyContent: 'flex-end'},
  tileFocus: {borderWidth: 4, borderColor: colors.white, shadowColor: colors.accentStrong, shadowOpacity: 0.6, shadowRadius: 26},
  tileText: {padding: u(16)},
  toolbar: {flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: u(8), marginBottom: u(12)},
  listRow: {flexDirection: 'row', alignItems: 'center', paddingVertical: u(8), paddingHorizontal: u(6), borderRadius: radius.md},
  check: {width: u(22), height: u(22), borderRadius: u(11), borderWidth: 2, borderColor: colors.lineStrong, marginRight: 10, alignItems: 'center', justifyContent: 'center'},
  checkOn: {backgroundColor: colors.accentStrong, borderColor: colors.accentStrong},
  letters: {position: 'absolute', right: -6, top: 120, bottom: 120, justifyContent: 'center'},
  lettersTV: {right: -40},
  cat: {borderRadius: radius.md, overflow: 'hidden', backgroundColor: colors.raised},
  entityHero: {borderRadius: radius.xl, overflow: 'hidden', minHeight: isTV ? 360 : 220, justifyContent: 'flex-end', backgroundColor: colors.raised},
});

export {ToggleRow};
