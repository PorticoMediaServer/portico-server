import {useEffect, useMemo, useState, type ComponentType, type ReactNode} from 'react';
import {defaultI18n, formatMessage, type MessageValues} from '@i18n';
import {parseBrowseCapabilities, parseBrowseResult, type BrowseCapabilities, type BrowseNode, type BrowseSortSelection} from '@core/browse';
import {browseNameId, filterControlOf, rangePresets} from '@core/presentation/browse-toolbar';
import {appliedFilterParts, browseToolbarModel, cardText, titleFactParts} from '@portico/client-core/kit-model';
import {browseFixtures} from '../../../../packages/i18n/src/catalog/en-US/next/dev/browse-fixtures';
import {defineGalleryFamily} from '../gallery-registry';
import {Button, Tabs} from '../../kit/controls';
import {Artwork, createArtworkRuntime, type ArtworkSource} from '../../kit/artwork';
import {Popover} from '../../kit/overlays';
import {BrowseToolbar, FilterBuilder, FilteredEmpty, LibraryHeader, LibraryMenu, LibrarySwitcher, SongRow, SortMenu, StickyBrowseHeader, TrackTable,
  type BrowseLabels, type BrowsePlatform, type FilterFieldChange, type FilterFieldState, type TrackRowData} from '../../kit/browse';
import styles from '../../kit/browse/gallery.module.css';
import movieCapabilities from '../../../../../../design-ui-v1/design/ui-v1/raw/caps_movies.json?raw';
import musicCapabilities from '../../../../../../design-ui-v1/design/ui-v1/raw/caps_music.json?raw';
import songResponse from '../../../../../../design-ui-v1/design/ui-v1/raw/browse_songs.json?raw';
import albumResponse from '../../../../../../design-ui-v1/design/ui-v1/raw/browse_albums.json?raw';

const t = defaultI18n.t;
const fixture = (id: keyof typeof browseFixtures, values?: MessageValues) => formatMessage(browseFixtures[id], values ?? {}, {locale: 'en-US'});
const env = {t, locale: 'en-US', owner: true};
const parsedMovies = parseBrowseCapabilities(JSON.parse(movieCapabilities)), parsedMusic = parseBrowseCapabilities(JSON.parse(musicCapabilities));
const movies: BrowseCapabilities = {...parsedMovies, resolvedPivot: parsedMovies.pivots.find(pivot => pivot.id === 'movies')!};
const music: BrowseCapabilities = {...parsedMusic, resolvedPivot: parsedMusic.pivots.find(pivot => pivot.id === 'songs')!};
const songs = parseBrowseResult(JSON.parse(songResponse)).entries, albums = parseBrowseResult(JSON.parse(albumResponse)).entries;
const name = (value: {id: string; labelKey: string}) => { const key = browseNameId(value.id, value.labelKey); return key ? t(key) : value.labelKey; };
const labels: BrowseLabels = {field: name, sort: name, quick: quick => defaultI18n.has(quick.labelKey) ? t(quick.labelKey) : quick.labelKey};
const genreKeys = ['animation', 'adventure', 'action', 'scifi', 'family', 'comedy', 'drama', 'fantasy', 'romance', 'horror', 'crime', 'documentary', 'thriller'] as const;
const genreCounts = [6, 8, 6, 6, 4, 2, 2, 1, 1, 6, 2, 1, 2];
const genres = genreKeys.map((value, index) => ({value, label: fixture(`next.browse.example.${value}`), count: genreCounts[index]}));
const valueNames = {'unplayed': 'next.kit.browse.unwatched', 'played': 'next.kit.browse.watched', 'in-progress': 'next.kit.browse.inProgress', 'available': 'next.kit.browse.available', 'unavailable': 'next.kit.browse.unavailable'} as const;
const valueName = (value: string) => Object.hasOwn(valueNames, value) ? t(valueNames[value as keyof typeof valueNames]) : value;
const initialPredicates: readonly BrowseNode[] = [{field: 'genre', operator: 'contains-any', value: [genres[0]!.label]}, {field: 'decade', operator: 'in', value: [2010]}];
function Stage({children, outcome}: Readonly<{children: ReactNode; outcome?: ReactNode}>) {
  return <div className={styles.stage}>{children}<output className={styles.outcome} aria-live="polite">{outcome}</output></div>;
}

function useReferenceArt() {
  const [runtime, setRuntime] = useState<ReturnType<typeof createArtworkRuntime> | null>(null);
  useEffect(() => {
    const next = createArtworkRuntime({scopeKey: 'browse-reference-gallery', classify: () => ({retryable: false}), load: async ({url}, signal) => {
      const image = new Image(); image.src = url;
      const abort = () => { image.src = ''; };
      signal.addEventListener('abort', abort, {once: true});
      try { await image.decode(); if (signal.aborted) throw new DOMException('Aborted', 'AbortError'); return {src: url, width: image.naturalWidth, height: image.naturalHeight}; }
      finally { signal.removeEventListener('abort', abort); }
    }});
    setRuntime(next); return () => next.dispose();
  }, []);
  return useMemo(() => {
    const sources = new Map<string, ArtworkSource>();
    for (const album of albums) {
      const slug = album.title === 'Abbey Road (2019 Mix)' ? 'abbey-road-2019-mix' : album.title === 'After Hours' ? 'after-hours' : undefined;
      if (runtime && slug) {
        const url = `http://100.120.50.24:8787/assets/pool/albums/${slug}.jpg`;
        sources.set(album.id, runtime.source({url, cacheKey: url}));
      }
    }
    return sources;
  }, [runtime]);
}

function headerCase(mode: 'desktop' | 'music' | 'phone' | 'switcher' | 'owner' | 'member' | 'read'): ComponentType {
  return function HeaderCase() {
    const [library, setLibrary] = useState(mode === 'music' ? 'music' : mode === 'read' ? 'books' : 'movies');
    const [switcherOpen, setSwitcherOpen] = useState(mode === 'switcher'), [menuOpen, setMenuOpen] = useState(mode === 'owner' || mode === 'member' || mode === 'read');
    const [outcome, setOutcome] = useState(''), [pivot, setPivot] = useState('movies');
    const choices = (['movies', 'tv', 'anime', 'music', 'audiobooks', 'books', 'comics'] as const).map(id => ({id, name: fixture(`next.browse.example.${id}`)}));
    const current = choices.find(choice => choice.id === library)!;
    const phone = !['desktop', 'music'].includes(mode);
    const switcher = <LibrarySwitcher libraries={choices} value={library} onValueChange={setLibrary} compact={phone} open={switcherOpen} onOpenChange={setSwitcherOpen}/>;
    const menuProps = {onSaveView: () => setOutcome(t('next.browse.saveThisView')), onMarkAll: () => setOutcome(t(mode === 'read' ? 'next.browse.markAllRead' : 'next.browse.markAllWatched')),
      onEditPins: () => setOutcome(t('next.browse.editPins')), read: mode === 'read', open: menuOpen, onOpenChange: setMenuOpen,
      viewSummary: `${t('next.browse.title')} ${t('next.browse.asc.alpha')}`, countSummary: fixture('next.browse.example.movieCount', {count: 80})};
    const menu = mode === 'member' ? <LibraryMenu {...menuProps} owner={false}/> : <LibraryMenu {...menuProps} owner onLibrarySettings={() => setOutcome(t('next.browse.librarySettings'))} onScan={() => setOutcome(t('next.browse.scan'))}/>;
    const tabs = <Tabs phone={phone} value={pivot} onValueChange={setPivot} className={phone ? styles.phoneTabs : styles.headerTabs}
      items={(['discover', 'movies', 'collections'] as const).map(value => ({value, label: fixture(`next.browse.example.${value}`)}))}/>;
    return <Stage outcome={outcome}>{phone ? <StickyBrowseHeader name={current.name} switcher={switcher} menu={menu} topInset={0} onLibraries={() => setOutcome(t('next.browse.libraries'))}>{tabs}</StickyBrowseHeader> :
      <LibraryHeader name={current.name} switcher={switcher} menu={menu} counts={mode === 'music' ? [
        {id: 'artists', text: fixture('next.browse.example.musicCounts'), role: 'value'}, {id: 'albums', text: fixture('next.browse.example.albumCounts'), role: 'value'},
        {id: 'songs', text: fixture('next.browse.example.songCount', {count: 40}), role: 'value'},
      ] : undefined}>{tabs}</LibraryHeader>}</Stage>;
  };
}

function toolbarCase(platform: BrowsePlatform, filtered = false, sticky = false, audio = false): ComponentType {
  return function ToolbarCase() {
    const [predicates, setPredicates] = useState<readonly BrowseNode[]>(filtered ? initialPredicates : []), [sort, setSort] = useState<BrowseSortSelection>({field: filtered ? 'year' : 'title', direction: filtered ? 'desc' : 'asc'});
    const [view, setView] = useState<'grid' | 'list'>('grid'), [size, setSize] = useState('medium'), [filtersOpen, setFiltersOpen] = useState(false), [outcome, setOutcome] = useState('');
    const model = browseToolbarModel(audio ? music : movies, audio ? 'music' : 'movie', predicates, sort, {...env, textEntry: platform !== 'tv'});
    const chips = model.applied.flatMap((predicate, index) => 'field' in predicate ? [{id: String(index), parts: appliedFilterParts(predicate, id => name((audio ? music : movies).fields.find(field => field.id === id) ?? {id, labelKey: id}), env)}] : []);
    return <Stage outcome={outcome}><BrowseToolbar model={model} labels={labels} platform={platform} sticky={sticky} topInset={0}
      resultCount={fixture(audio ? 'next.browse.example.songCount' : 'next.browse.example.movieCount', {count: filtered ? 6 : audio ? 40 : 80})}
      onQuickChange={(id, checked) => { const quick = [...model.quick, ...model.filterQuick].find(quick => quick.id === id)!; setPredicates(current => checked ? [...current, quick.query] : current.filter(node => node !== quick.query)); }}
      onFilters={() => setFiltersOpen(value => !value)} filtersOpen={filtersOpen} onSortChange={setSort} onReshuffle={() => setSort(current => ({...current, seed: (current.seed ?? 0) + 1}))}
      view={audio ? undefined : {value: view, onValueChange: setView}} cardSize={audio ? undefined : {value: size, options: (['small', 'medium', 'large'] as const).map(value => ({value, label: t(`next.browse.${value}`)})), onValueChange: setSize}}
      save={{canSave: filtered || view !== 'grid' || size !== 'medium' || sort.field !== 'title', onSave: () => setOutcome(t('next.browse.saveView'))}}
      applied={chips} onRemoveFilter={id => setPredicates(current => current.filter(node => node !== model.applied[Number(id)]))} onClearFilters={() => setPredicates([])}
      onJump={platform === 'tv' ? () => setOutcome(t('next.browse.jump')) : undefined}/>
      {filtersOpen && <FilterSpecimen platform={platform}/>}</Stage>;
  };
}

function SortCase() {
  const [sort, setSort] = useState<BrowseSortSelection>({field: 'year', direction: 'desc'}), [open, setOpen] = useState(true);
  const model = browseToolbarModel(movies, 'movie', initialPredicates, sort, env);
  return <Stage><div className={styles.menuStage}><SortMenu sorts={model.sorts} value={model.sort} label={labels.sort} open={open} onOpenChange={setOpen} onValueChange={setSort} onReshuffle={() => setSort(current => ({...current, seed: (current.seed ?? 0) + 1}))}/></div></Stage>;
}

function makeFields(model: ReturnType<typeof browseToolbarModel>, phone: boolean): Record<string, FilterFieldState> {
  return Object.fromEntries(model.groups.flatMap(group => group.fields.map(field => {
    const kind = filterControlOf(field);
    let state: FilterFieldState;
    if (kind === 'values') state = {kind, query: '', match: 'any', included: field.id === 'genre' ? [phone ? 'animation' : 'animation', ...(phone ? ['family'] : [])] : field.id === 'year' ? ['2010'] : [], excluded: phone && field.id === 'genre' ? ['horror'] : [],
      values: field.id === 'genre' ? genres.slice(0, phone ? 11 : 9) : field.id === 'year' ? [2010, 2000, 1990, 1980].map(year => ({value: String(year), label: t('next.kit.filter.decade', {year: String(year)})})) : (field.allowedValues ?? []).map(value => ({value, label: value})), total: field.id === 'genre' ? 13 : field.id === 'year' ? 4 : field.allowedValues?.length ?? 0, status: 'ready', summary: field.id === 'genre' ? fixture('next.browse.example.animation') : field.id === 'year' ? t('next.kit.filter.decade', {year: '2010'}) : undefined};
    else if (kind === 'toggle') state = {kind, value: undefined};
    else if (kind === 'choice') state = {kind, value: undefined, options: (field.allowedValues ?? []).map(value => ({value, label: valueName(value)}))};
    else if (kind === 'range') state = {kind, value: undefined, options: rangePresets(field, new Date('2026-10-22T12:00:00Z')).map(preset => ({value: preset.id, label: t(preset.label.id, preset.label.values)}))};
    else state = {kind, value: ''};
    return [field.id, state];
  })));
}
function FilterSpecimen({platform, state = 'ready'}: Readonly<{platform: BrowsePlatform; state?: 'ready' | 'loading' | 'error' | 'empty'}>) {
  const model = browseToolbarModel(movies, 'movie', initialPredicates, {field: 'title', direction: 'asc'}, {...env, textEntry: platform !== 'tv'});
  const [fields, setFields] = useState(() => {
    const initial = makeFields(model, platform === 'phone'), genre = initial.genre;
    if (genre?.kind === 'values') initial.genre = {...genre, status: state === 'empty' ? 'ready' : state, values: state === 'empty' ? [] : genre.values,
      query: state === 'empty' ? fixture('next.browse.example.documentary') : '', error: state === 'error' ? fixture('next.browse.example.facetError') : undefined};
    return initial;
  });
  const [openFields, setOpenFields] = useState<readonly string[]>(['genre']), [outcome, setOutcome] = useState('');
  const change = (field: string, change: FilterFieldChange) => setFields(current => {
    const previous = current[field]!;
    if (change.kind === 'clear') return {...current, [field]: {...makeFields(model, false)[field]!, summary: undefined, ...(previous.kind === 'values' ? {included: [], excluded: []} : {})}};
    if (change.kind === 'facet' && previous.kind === 'values') return {...current, [field]: {...previous, included: [...previous.included.filter(value => value !== change.value), ...(change.selection === 'include' ? [change.value] : [])], excluded: [...previous.excluded.filter(value => value !== change.value), ...(change.selection === 'exclude' ? [change.value] : [])]}};
    if (change.kind === 'match' && previous.kind === 'values') return {...current, [field]: {...previous, match: change.value}};
    if (change.kind === 'choice' && (previous.kind === 'choice' || previous.kind === 'range')) return {...current, [field]: {...previous, value: change.value}};
    if (change.kind === 'toggle' && previous.kind === 'toggle') return {...current, [field]: {...previous, value: change.value}};
    if (change.kind === 'text' && previous.kind === 'text') return {...current, [field]: {...previous, value: change.value}};
    return current;
  });
  return <div className={styles.filterCrop}><FilterBuilder model={model} labels={labels} platform={platform} fields={fields} openFields={openFields}
    onFieldOpenChange={(field, open) => setOpenFields(current => open ? [...current, field] : current.filter(value => value !== field))} onFieldChange={change}
    onSearchChange={(field, query) => setFields(current => {
      const previous = current[field];
      return previous?.kind === 'values' ? {...current, [field]: {...previous, query, values: field === 'genre' ? genres.filter(value => value.label.toLocaleLowerCase().includes(query.toLocaleLowerCase())) : previous.values}} : current;
    })}
    onShowAll={field => { setOutcome(field); setFields(current => {const previous = current[field]; return previous?.kind === 'values' ? {...current, [field]: {...previous, values: genres, total: genres.length}} : current;}); }}
    onRetry={field => setFields(current => ({...current, [field]: {...current[field]!, status: 'ready', error: undefined}}))}/><output className={styles.outcome}>{outcome}</output></div>;
}
function filterCase(platform: BrowsePlatform, state: 'ready' | 'loading' | 'error' | 'empty' = 'ready'): ComponentType { return () => <Stage><FilterSpecimen platform={platform} state={state}/></Stage>; }

function EmptyCase() {
  const [cleared, setCleared] = useState(false), [removed, setRemoved] = useState('');
  return <Stage outcome={removed}>{cleared ? <div className={styles.controlStage}><Button onClick={() => setCleared(false)}>{t('next.browse.filters')}</Button></div> :
    <FilteredEmpty description={fixture('next.browse.example.emptyDescription')} onClear={() => setCleared(true)} suggestions={[
      {id: 'year', label: fixture('next.browse.example.withoutYear'), count: 6, onSelect: () => setRemoved('year')},
      {id: 'genre', label: fixture('next.browse.example.withoutGenre'), count: 11, onSelect: () => setRemoved('genre')},
    ].filter(suggestion => suggestion.id !== removed)}/>}</Stage>;
}

function tableCase(variant: 'album' | 'top-songs' | 'songs' | 'chapters' | 'compact' | 'sparse' | 'disc'): ComponentType {
  return function TableCase() {
    const sources = useReferenceArt(), [playing, setPlaying] = useState(variant === 'album' ? songs.find(song => song.title === 'Hardest To Love')?.id : undefined);
    const [favorites, setFavorites] = useState<readonly string[]>(playing ? [playing] : []), [more, setMore] = useState<string | undefined>(), [outcome, setOutcome] = useState('');
    const [range, setRange] = useState({start: 0, end: 14}), [sort, setSort] = useState({field: 'title', direction: 'asc' as 'asc' | 'desc'});
    const rows: readonly TrackRowData[] = useMemo(() => {
      if (variant === 'chapters') return Array.from({length: 11}, (_, index) => {
        const current = index === 5, title = fixture(`next.browse.example.chapter${index + 1}` as keyof typeof browseFixtures);
        const duration = [1663, 1335, 1510, 1598, 2387, 2119, 1649, 1263, 1820, 1499, 1415][index]!;
        const remaining = titleFactParts({durationSeconds: duration * .48}, env).find(part => part.id === 'duration')?.text;
        return {id: `reference-chapter-${index + 1}`, number: index + 1, text: cardText({id: `reference-chapter-${index + 1}`, kind: 'chapter', title}, 'ordinary', env),
          done: index < 5, current, progress: current ? .52 : undefined, duration: {id: 'duration', text: fixture(`next.browse.example.chapterDuration${index + 1}` as keyof typeof browseFixtures), role: 'supplement'}, remaining: remaining ? {id: 'remaining', text: t('next.kit.remaining', {time: remaining}), role: 'supplement'} : undefined};
      });
      const selected = variant === 'album' || variant === 'disc' ? songs.filter(song => song.album?.name === 'After Hours') : variant === 'top-songs' ? songs.filter(song => song.album?.name === 'Abbey Road (2019 Mix)') : songs;
      return selected.map((entry, index) => {
        const album = albums.find(album => album.id === entry.album?.id), source = entry.album && sources.get(entry.album.id);
        return {id: entry.id, number: variant === 'songs' ? index + 1 : entry.trackNumber, text: cardText({id: entry.id, kind: entry.kind, title: entry.title,
          artist: variant === 'sparse' ? undefined : album?.artist?.name, album: variant === 'sparse' ? undefined : entry.album?.name, year: entry.year, durationSeconds: variant === 'sparse' ? undefined : entry.duration}, 'song-row', env),
          plays: entry.plays, addedAt: entry.addedAt, playing: playing === entry.id, favorite: favorites.includes(entry.id),
          artwork: ['top-songs', 'songs', 'compact'].includes(variant) && source ? <Artwork source={source} alt="" className={styles.trackArt}/> : undefined};
      });
    }, [favorites, playing, sources]);
    const actions = {onPlay: (id: string) => {setPlaying(id); setOutcome(id);}, onMore: setMore, onFavorite: (id: string, favorite: boolean) => setFavorites(current => favorite ? [...current, id] : current.filter(value => value !== id))};
    const selected = rows.find(row => row.id === more);
    return <Stage outcome={outcome}>{variant === 'compact' ? rows.slice(0, 4).map(row => <SongRow key={row.id} row={row} onPlay={actions.onPlay} onMore={actions.onMore}/>) :
      <TrackTable rows={variant === 'songs' ? rows.slice(range.start, range.end) : rows} variant={variant === 'sparse' || variant === 'disc' ? 'album' : variant} label={t(variant === 'chapters' ? 'next.browse.chapters' : 'next.browse.songs')}
        {...actions} discNumber={variant === 'disc' ? 2 : undefined} sort={sort} onSort={field => setSort(current => ({field, direction: field === current.field && current.direction === 'asc' ? 'desc' : 'asc'}))}
        height={variant === 'songs' ? 560 : undefined} startIndex={variant === 'songs' ? range.start : undefined} totalCount={rows.length} onWindowChange={variant === 'songs' ? setRange : undefined}/>}
      {selected && <Popover open onOpenChange={open => {if (!open) setMore(undefined);}} title={selected.text.title} trigger={<Button className={styles.outcome}>{selected.text.title}</Button>}>
        <div className={styles.trackMenu}><Button onClick={() => {actions.onPlay(selected.id); setMore(undefined);}}>{t('next.browse.play', {title: selected.text.title})}</Button>
          <Button onClick={() => {actions.onFavorite(selected.id, !selected.favorite); setMore(undefined);}}>{t(selected.favorite ? 'next.browse.unfavorite' : 'next.browse.favorite', {title: selected.text.title})}</Button></div>
      </Popover>}</Stage>;
  };
}

export default defineGalleryFamily({id: 'browse', labelKey: 'next.browse.family', cases: [
  {id: 'library-header', labelKey: 'next.browse.libraryHeader', width: 1440, height: 244, render: headerCase('desktop')},
  {id: 'library-music', labelKey: 'next.browse.libraryHeader', width: 1440, height: 270, render: headerCase('music')},
  {id: 'library-phone', labelKey: 'next.browse.phoneHeader', width: 390, height: 94, render: headerCase('phone')},
  {id: 'library-switcher', labelKey: 'next.browse.switchLibrary', width: 390, height: 450, render: headerCase('switcher')},
  {id: 'library-menu-owner', labelKey: 'next.browse.libraryOptions', width: 390, height: 450, render: headerCase('owner')},
  {id: 'library-menu-member', labelKey: 'next.browse.libraryOptions', width: 390, height: 350, render: headerCase('member')},
  {id: 'library-menu-read', labelKey: 'next.browse.libraryOptions', width: 390, height: 450, render: headerCase('read')},
  {id: 'toolbar-desktop', labelKey: 'next.browse.toolbarCase', width: 1440, height: 80, render: toolbarCase('desktop')},
  {id: 'toolbar-filtered', labelKey: 'next.browse.toolbarCase', width: 1440, height: 120, render: toolbarCase('desktop', true)},
  {id: 'toolbar-sticky', labelKey: 'next.browse.toolbarCase', width: 1440, height: 100, render: toolbarCase('desktop', true, true)},
  {id: 'toolbar-phone', labelKey: 'next.browse.toolbarCase', width: 390, height: 160, render: toolbarCase('phone', true)},
  {id: 'toolbar-tv', labelKey: 'next.browse.toolbarCase', width: 1480, height: 150, render: toolbarCase('tv', true)},
  {id: 'toolbar-music', labelKey: 'next.browse.toolbarCase', width: 1440, height: 120, render: toolbarCase('desktop', false, false, true)},
  {id: 'sort-menu', labelKey: 'next.browse.sortBy', width: 390, height: 780, render: SortCase},
  {id: 'filter-desktop', labelKey: 'next.browse.filtersCase', width: 520, height: 1140, render: filterCase('desktop')},
  {id: 'filter-phone', labelKey: 'next.browse.filtersCase', width: 390, height: 610, render: filterCase('phone')},
  {id: 'filter-tv', labelKey: 'next.browse.filtersCase', width: 670, height: 850, render: filterCase('tv')},
  ...(['loading', 'error', 'empty'] as const).map(state => ({id: `filter-${state}`, labelKey: 'next.browse.filtersCase' as const, width: 520, height: 650, render: filterCase('desktop', state)})),
  {id: 'filtered-empty', labelKey: 'next.browse.emptyCase', width: 1440, height: 650, render: EmptyCase},
  {id: 'tracks-album', labelKey: 'next.browse.tracksCase', width: 880, height: 288, render: tableCase('album')},
  {id: 'tracks-top', labelKey: 'next.browse.tracksCase', width: 880, height: 288, render: tableCase('top-songs')},
  {id: 'tracks-songs', labelKey: 'next.browse.tracksCase', width: 1312, height: 608, render: tableCase('songs')},
  {id: 'tracks-chapters', labelKey: 'next.browse.tracksCase', width: 880, height: 676, render: tableCase('chapters')},
  {id: 'tracks-sparse', labelKey: 'next.browse.tracksCase', width: 880, height: 288, render: tableCase('sparse')},
  {id: 'tracks-disc', labelKey: 'next.browse.tracksCase', width: 880, height: 340, render: tableCase('disc')},
  {id: 'song-row', labelKey: 'next.browse.songCase', width: 625, height: 248, render: tableCase('compact')},
]});
