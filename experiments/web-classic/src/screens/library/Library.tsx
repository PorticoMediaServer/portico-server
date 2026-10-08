import React, {useMemo, useRef, useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {BrowseQuery, Category, Facet, Filters, Library as Lib, LibraryView, Shape, SortField} from '@/data/types';
import {libraryIcon, itemNoun} from '@/app/libraryMeta';
import {usePrefs} from '@/app/prefs';
import {useActions} from '@/app/actions';
import {usePlayer} from '@/player/PlayerProvider';
import {hrefFor} from '@/app/links';
import {formatDuration, formatTime} from '@/app/format';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Chip, Chips, EmptyState, ErrorState, tintOf} from '@/ui/Bits';
import {Dialog, Menu, useToast} from '@/ui/Overlay';
import {Segmented, Select, TextField} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {Shelf, CardGrid} from '@/ui/Shelf';
import {WindowedGrid, WindowedList, type PageSource} from '@/ui/Windowed';
import {LinkTabs} from '@/ui/Tabs';
import {Toolbar} from '@/ui/Page';
import {cx} from '@/ui/cx';
import {usePhone, useTitle} from '@/ui/hooks';
import {PageLoading, useRouteParams, useRouteSearch} from '../common';
import {useDemo} from '@/app/demo';
import s from './Library.module.css';

type ViewDef = {id: LibraryView; label: string};
/** The pivots each kind of library offers. */
export function viewsFor(lib: Lib): ViewDef[] {
  switch (lib.kind) {
    case 'music': return [{id: 'discover', label: 'Discover'}, {id: 'browse', label: 'Artists'}, {id: 'albums', label: 'Albums'}, {id: 'songs', label: 'Songs'}, {id: 'categories', label: 'Genres'}];
    case 'audiobook': return [{id: 'discover', label: 'Discover'}, {id: 'browse', label: 'Books'}, {id: 'authors', label: 'Authors'}, {id: 'series', label: 'Series'}];
    case 'photo': return [{id: 'browse', label: 'Albums'}, {id: 'timeline', label: 'All photos'}];
    case 'homevideo': return [{id: 'discover', label: 'Discover'}, {id: 'browse', label: 'Browse'}];
    default: return [{id: 'discover', label: 'Discover'}, {id: 'browse', label: 'Browse'}, {id: 'collections', label: 'Collections'}, {id: 'categories', label: 'Categories'}];
  }
}
const SORTS: {value: SortField; label: string; dir: 'asc' | 'desc'}[] = [
  {value: 'title', label: 'Title', dir: 'asc'}, {value: 'added', label: 'Date added', dir: 'desc'}, {value: 'year', label: 'Release year', dir: 'desc'},
  {value: 'rating', label: 'Rating', dir: 'desc'}, {value: 'duration', label: 'Length', dir: 'asc'}, {value: 'lastPlayed', label: 'Last played', dir: 'desc'}, {value: 'random', label: 'Shuffle', dir: 'asc'},
];
const QUICK: {id: string; label: string; filters: Partial<Filters>}[] = [
  {id: 'all', label: 'All', filters: {}}, {id: 'unwatched', label: 'Unwatched', filters: {playState: 'unwatched'}}, {id: 'inProgress', label: 'In progress', filters: {playState: 'inProgress'}},
  {id: 'recent', label: 'Recently added', filters: {quick: 'recent'}}, {id: 'favorite', label: 'Favorites', filters: {favorite: true}}, {id: 'watchlisted', label: 'On my Watchlist', filters: {watchlisted: true}},
];
const shapeFor = (lib: Lib, view: LibraryView): Shape => view === 'albums' ? 'square' : lib.kind === 'music' ? 'circle' : lib.kind === 'photo' ? (view === 'timeline' ? 'square' : 'square') : lib.kind === 'homevideo' ? 'wide' : 'poster';

export default function LibraryScreen() {
  const {libraryId} = useRouteParams<{libraryId: string}>();
  const search = useRouteSearch<{view?: string; sort?: string; direction?: string; filters?: string}>();
  const libs = useQuery(c => c.libraries.list(), [], ['libraries']);
  const lib = libs.data?.find(l => l.id === libraryId);
  useTitle(lib?.name);
  if (libs.error) return <ErrorState onRetry={libs.refresh} />;
  if (!libs.data) return <PageLoading />;
  if (!lib) return <EmptyState icon="folder" title="This library isn’t available" body="It may have been removed, or it isn’t shared with this profile." actions={[{label: 'Go to Home', to: '/'}]} />;
  const views = viewsFor(lib);
  const view = (views.find(v => v.id === search.view)?.id ?? views[0]!.id) as LibraryView;
  return (
    <div style={{paddingTop: 28, paddingBottom: 72}}>
      <div className={s.head}>
        <h1 className={s.title}><Icon name={libraryIcon(lib.kind)} size={26} className={s.titleIcon} />{lib.name}</h1>
        <span className={s.count}>{itemNoun(lib, lib.count)}{lib.scanning ? ` · Scanning, ${lib.scanning.found} files found` : ''}</span>
      </div>
      <div className={s.tabs}><LinkTabs label={`${lib.name} views`} current={view} tabs={views.map(v => ({id: v.id, label: v.label, href: `/library/${lib.id}?view=${v.id}`}))} /></div>
      {view === 'discover' ? <Discover lib={lib} />
        : view === 'collections' ? <Collections lib={lib} />
        : view === 'categories' ? <Categories lib={lib} />
        : <Browse lib={lib} view={view} search={search} />}
    </div>
  );
}

function Discover({lib}: {lib: Lib}) {
  const rows = useQuery(c => c.libraries.discover(lib.id), [lib.id], ['personal', 'home']);
  if (rows.error) return <ErrorState onRetry={rows.refresh} />;
  if (rows.data && !rows.data.length) return <EmptyState icon="folder" title={`${lib.name} is empty`} body="Once the server finds media in this library’s folders, it appears here." />;
  return (
    <div className={s.discover}>
      {rows.data ? rows.data.map((r, i) => <Shelf key={r.id} title={r.title} entries={r.entries} shape={r.shape} total={r.total} eager={i < 2} origin={r.kind === 'continue' ? 'continue' : r.kind === 'picks' ? 'recommendation' : undefined} href={r.id.startsWith('g-') ? `/library/${lib.id}/genre/${encodeURIComponent(r.id.slice(2))}` : r.id === 'collections' ? `/library/${lib.id}?view=collections` : undefined} />)
        : [0, 1, 2].map(i => <Shelf key={i} title="" shape={shapeFor(lib, 'browse')} />)}
    </div>
  );
}

function Collections({lib}: {lib: Lib}) {
  const list = useQuery(c => c.libraries.collections(lib.id), [lib.id]);
  if (list.data && !list.data.length) return <EmptyState icon="collection" title="No collections yet" body="Collections group related titles, like a film series. The server makes them from metadata, and owners can add their own." />;
  return <CardGrid entries={list.data} loading={list.loading} shape="poster" />;
}

function Categories({lib}: {lib: Lib}) {
  const cats = useQuery(c => c.libraries.categories(lib.id), [lib.id]);
  if (!cats.data) return <CardGrid loading shape="wide" skeletons={12} />;
  const groups: {kind: Category['kind']; title: string}[] = [{kind: 'genre', title: 'Genres'}, {kind: 'decade', title: 'Decades'}, {kind: 'studio', title: 'Studios'}, {kind: 'network', title: 'Networks'}];
  return (
    <div>
      {groups.map(g => {
        const list = cats.data!.filter(c => c.kind === g.kind);
        if (!list.length) return null;
        return (
          <section key={g.kind} className={s.catGroup}>
            <h2 className={s.catGroupTitle}>{g.title}</h2>
            <div className={s.cats}>{list.map(c => <CategoryTile key={c.id} lib={lib} cat={c} />)}</div>
          </section>
        );
      })}
    </div>
  );
}

function CategoryTile({lib, cat}: {lib: Lib; cat: Category}) {
  return (
    <Link to={`/library/${lib.id}/${cat.kind}/${encodeURIComponent(cat.title)}` as never} className={s.cat} style={{'--tint': tintOf(cat.title)} as React.CSSProperties}>
      <div className={cx(s.catArt, cat.images.filter(Boolean).length < 3 && s.catArtOne)} aria-hidden>{cat.images.filter(Boolean).slice(0, cat.images.filter(Boolean).length < 3 ? 1 : 3).map((src, i) => <img key={i} src={src} alt="" loading="lazy" />)}</div>
      <span><span className={s.catTitle} style={{display: 'block'}}>{cat.title}</span><span className={s.catCount}>{itemNoun(lib, cat.count)}</span></span>
    </Link>
  );
}

function parseFilters(raw?: string): Filters {
  if (!raw) return {};
  try { const v = JSON.parse(raw) as Filters; return v && typeof v === 'object' ? v : {}; } catch { return {}; }
}
const activeCount = (f: Filters) => Object.entries(f).filter(([k, v]) => k !== 'quick' && (Array.isArray(v) ? v.length : v !== undefined && v !== false)).length + (f.quick ? 1 : 0);

function Browse({lib, view, search}: {lib: Lib; view: LibraryView; search: {sort?: string; direction?: string; filters?: string}}) {
  const client = useClient();
  const navigate = useNavigate();
  const toast = useToast();
  const player = usePlayer();
  const phone = usePhone();
  const {prefs, update} = usePrefs();
  const filters = useMemo(() => parseFilters(search.filters), [search.filters]);
  const sort = (SORTS.find(x => x.value === search.sort)?.value ?? 'title') as SortField;
  const direction = (search.direction === 'asc' || search.direction === 'desc' ? search.direction : SORTS.find(x => x.value === sort)!.dir) as 'asc' | 'desc';
  const query: BrowseQuery = useMemo(() => ({sort, direction, filters}), [sort, direction, filters]);
  const key = `${lib.id}|${view}|${JSON.stringify(query)}`;
  const total = useQuery(c => c.libraries.browse(lib.id, query, 0, 1, view), [key], ['personal']);
  const letters = useQuery(c => (sort === 'title' ? c.libraries.letters(lib.id, query, view) : Promise.resolve([])), [key]);
  const [panel, setPanel] = useState(false);
  const [forceList, setForceList] = useState(false);
  useDemo('filters', () => setPanel(true));
  useDemo('list', () => setForceList(true));
  const [saving, setSaving] = useState(false);
  const jump = useRef<((i: number) => void) | null>(null);
  const layout = view === 'songs' || forceList ? 'list' : prefs.libraryLayout[lib.id] ?? 'grid';
  const shape = shapeFor(lib, view);
  const set = (patch: {sort?: string; direction?: string; filters?: Filters}) => {
    const next = {view, sort: patch.sort ?? search.sort, direction: patch.direction ?? search.direction, filters: patch.filters ? (activeCount(patch.filters) ? JSON.stringify(patch.filters) : undefined) : search.filters};
    void navigate({to: `/library/${lib.id}` as never, search: next as never, replace: true});
  };
  const source: PageSource = useMemo(() => ({key, total: total.data?.total ?? 0, fetch: (offset, limit) => client.libraries.browse(lib.id, query, offset, limit, view).then(p => p.entries)}), [key, total.data?.total, client, lib.id, query, view]);
  const quickId = QUICK.find(q => q.id !== 'all' && Object.entries(q.filters).every(([k, v]) => (filters as Record<string, unknown>)[k] === v))?.id ?? (activeCount(filters) ? '' : 'all');
  const video = lib.kind === 'movie' || lib.kind === 'tv' || lib.kind === 'anime' || lib.kind === 'homevideo';
  const n = activeCount(filters);
  return (
    <>
      <Toolbar sticky label="Browse options">
        {video ? <Chips scroll={phone} label="Quick filters">{QUICK.map(q => <Chip key={q.id} pressed={quickId === q.id} onClick={() => set({filters: {...Object.fromEntries(Object.entries(filters).filter(([k]) => !['playState', 'quick', 'favorite', 'watchlisted'].includes(k))), ...q.filters}})}>{q.label}</Chip>)}</Chips> : null}
        <span className={s.barSpacer} />
        {total.data ? <span className={`${s.total} ${n ? '' : s.hidePhone}`}>{countLabel(lib, view, total.data.total)}</span> : null}
        <div className={s.bar}>
          <Select compact className={s.sortSelect} ariaLabel="Sort by" value={sort} onChange={v => set({sort: v, direction: SORTS.find(x => x.value === v)!.dir})} options={SORTS.filter(o => lib.kind !== 'photo' || ['title', 'added', 'year'].includes(o.value)).map(o => ({value: o.value, label: `Sort: ${o.label}`}))} />
          <IconButton variant="secondary" icon="sort" label={direction === 'asc' ? 'Ascending. Switch to descending' : 'Descending. Switch to ascending'} onClick={() => set({direction: direction === 'asc' ? 'desc' : 'asc'})} />
          {lib.kind !== 'photo' ? <Button variant={n ? 'primary' : 'secondary'} icon="filter" label={n ? `Filters · ${n}` : 'Filters'} onClick={() => setPanel(true)} /> : null}
          {view !== 'songs' && view !== 'timeline' && !phone ? <Segmented label="Layout" value={layout} onChange={v => { setForceList(false); update({libraryLayout: {...prefs.libraryLayout, [lib.id]: v}}); }} options={[{value: 'grid', label: '', icon: 'gridView'}, {value: 'list', label: '', icon: 'list'}]} /> : null}
          <Menu label="More" trigger={<IconButton variant="secondary" icon="more" label="More" />} items={[
            {id: 'save', label: 'Save this view…', icon: 'saved', disabled: !n && sort === 'title', onSelect: () => setSaving(true)},
            ...(lib.kind === 'photo' || lib.kind === 'music' ? [] : [{id: 'shuffle', label: 'Shuffle play', icon: 'shuffle' as const, onSelect: async () => { const p = await client.libraries.browse(lib.id, {...query, sort: 'random'}, 0, 50, view); const list = p.entries.filter(e => e.kind === 'movie' || e.kind === 'homevideo'); if (list.length) player.play(list, 0); else toast.show('Shuffle plays movies and videos. Open a show to shuffle its episodes.', {icon: 'info'}); }}]),
            {id: 'clear', label: 'Clear filters', icon: 'close', disabled: !n, onSelect: () => set({filters: {}})},
          ]} />
        </div>
      </Toolbar>
      {n ? <ActiveFilters filters={filters} onChange={f => set({filters: f})} /> : null}
      <div className={s.browse}>
        <div className={s.browseMain}>
          {total.data && total.data.total === 0 ? (
            <EmptyState icon="filter" title="Nothing matches these filters" body="Try removing a filter or two." actions={[{label: 'Clear filters', onClick: () => set({filters: {}})}]} />
          ) : !total.data ? <CardGrid loading shape={shape} /> : layout === 'list' ? <ListView source={source} lib={lib} songs={view === 'songs'} /> : <WindowedGrid source={source} shape={shape} jumpRef={jump} />}
        </div>
        {sort === 'title' && letters.data && letters.data.length > 6 && (total.data?.total ?? 0) > 60 && layout === 'grid' ? <AlphaRail letters={letters.data} onJump={i => jump.current?.(i)} /> : null}
      </div>
      <FilterPanel open={panel} onOpenChange={setPanel} lib={lib} filters={filters} onChange={f => set({filters: f})} />
      <SaveViewDialog open={saving} onOpenChange={setSaving} onSave={async name => { await client.saved.saveView(lib.id, name, query); toast.show(`Saved “${name}”. Find it in Saved › Views.`, {icon: 'saved', action: {label: 'Open', run: () => void navigate({to: '/saved?view=views' as never})}}); }} />
    </>
  );
}

/** "339 movies", "137 albums", "1,445 songs", "200 photos". */
function countLabel(lib: Lib, view: LibraryView, n: number): string {
  const noun = view === 'albums' ? ['album', 'albums'] : view === 'songs' ? ['song', 'songs'] : view === 'timeline' ? ['photo', 'photos'] : view === 'authors' ? ['author', 'authors'] : view === 'series' ? ['series', 'series'] : undefined;
  return noun ? `${n.toLocaleString('en-US')} ${n === 1 ? noun[0] : noun[1]}` : itemNoun(lib, n);
}

function AlphaRail({letters, onJump}: {letters: {letter: string; offset: number}[]; onJump: (i: number) => void}) {
  const all = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];
  return (
    <nav className={s.alpha} aria-label="Jump to letter">
      {all.map(l => { const hit = letters.find(x => x.letter === l); return <button key={l} type="button" className={s.letter} disabled={!hit} onClick={() => hit && onJump(hit.offset)} aria-label={`Jump to ${l === '#' ? 'numbers' : l}`}>{l}</button>; })}
    </nav>
  );
}

function ActiveFilters({filters, onChange}: {filters: Filters; onChange: (f: Filters) => void}) {
  const chips: {label: string; remove: () => void}[] = [];
  (['genre', 'decade', 'contentRating', 'studio', 'resolution'] as const).forEach(k => (filters[k] ?? []).forEach(v => chips.push({label: v, remove: () => onChange({...filters, [k]: filters[k]!.filter(x => x !== v)})})));
  if (filters.playState) chips.push({label: {unwatched: 'Unwatched', inProgress: 'In progress', watched: 'Watched'}[filters.playState], remove: () => onChange({...filters, playState: undefined})});
  if (filters.favorite) chips.push({label: 'Favorites', remove: () => onChange({...filters, favorite: undefined})});
  if (filters.watchlisted) chips.push({label: 'On my Watchlist', remove: () => onChange({...filters, watchlisted: undefined})});
  if (filters.quick === 'recent') chips.push({label: 'Recently added', remove: () => onChange({...filters, quick: undefined})});
  return (
    <div className={s.activeFilters} aria-label="Active filters">
      {chips.map(c => <Chip key={c.label} onRemove={c.remove} title={`Remove ${c.label}`}>{c.label}</Chip>)}
      <Button size="sm" variant="ghost" label="Clear all" onClick={() => onChange({})} />
    </div>
  );
}

function FilterPanel({open, onOpenChange, lib, filters, onChange}: {open: boolean; onOpenChange: (o: boolean) => void; lib: Lib; filters: Filters; onChange: (f: Filters) => void}) {
  const facets = useQuery(c => (open ? c.libraries.facets(lib.id) : Promise.resolve([] as Facet[])), [open, lib.id]);
  const toggle = (field: keyof Filters, value: string) => {
    const cur = (filters[field] as string[] | undefined) ?? [];
    onChange({...filters, [field]: cur.includes(value) ? cur.filter(x => x !== value) : [...cur, value]});
  };
  const video = lib.kind === 'movie' || lib.kind === 'tv' || lib.kind === 'anime';
  return (
    <Dialog open={open} onOpenChange={onOpenChange} side="right" title="Filters" description="Results update as you choose." footer={<><Button variant="ghost" label="Clear all" onClick={() => onChange({})} /><Button variant="primary" label="Done" onClick={() => onOpenChange(false)} /></>}>
      {video ? (
        <div className={s.facet}>
          <div className={s.facetHead}><span className="t-label">Watched</span></div>
          <Segmented label="Watched" value={filters.playState ?? 'any'} onChange={v => onChange({...filters, playState: v === 'any' ? undefined : v})} options={[{value: 'any', label: 'Any'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}, {value: 'watched', label: 'Watched'}]} />
        </div>
      ) : null}
      {facets.data?.map(f => (
        <div key={f.field} className={s.facet}>
          <div className={s.facetHead}><span className="t-label">{f.label}</span>{((filters[f.field] as string[] | undefined) ?? []).length ? <Button size="sm" variant="ghost" label="Clear" onClick={() => onChange({...filters, [f.field]: []})} /> : null}</div>
          <div className={s.facetValues}>
            {f.values.slice(0, 24).map(v => <Chip key={v.value} pressed={((filters[f.field] as string[] | undefined) ?? []).includes(v.value)} onClick={() => toggle(f.field, v.value)}>{v.label}{v.count ? <span className={s.facetCount}>{v.count}</span> : null}</Chip>)}
          </div>
        </div>
      ))}
      <div className={s.facet}>
        <div className={s.facetHead}><span className="t-label">Yours</span></div>
        <div className={s.facetValues}>
          <Chip pressed={!!filters.favorite} onClick={() => onChange({...filters, favorite: filters.favorite ? undefined : true})}>Favorites</Chip>
          {video ? <Chip pressed={!!filters.watchlisted} onClick={() => onChange({...filters, watchlisted: filters.watchlisted ? undefined : true})}>On my Watchlist</Chip> : null}
        </div>
      </div>
    </Dialog>
  );
}

export function SaveViewDialog({open, onOpenChange, onSave}: {open: boolean; onOpenChange: (o: boolean) => void; onSave: (name: string) => Promise<void>}) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Save this view" description="Keeps these filters and this sort as a view you can open from Saved, or pin to Home." width={440} footer={<>
      <Button variant="ghost" label="Cancel" onClick={() => onOpenChange(false)} />
      <Button variant="primary" label="Save view" busy={busy} disabled={!name.trim()} onClick={async () => { setBusy(true); await onSave(name.trim()); setBusy(false); setName(''); onOpenChange(false); }} />
    </>}>
      <TextField label="Name" value={name} onChange={setName} placeholder="Unwatched thrillers" autoFocus />
    </Dialog>
  );
}

function ListView({source, lib, songs}: {source: PageSource; lib: Lib; songs?: boolean}) {
  const actions = useActions();
  if (songs) return <SongList source={source} />;
  const header = (
    <div className={s.listHead} role="presentation">
      <span /><span>Title</span><span>{songs ? 'Time' : 'Year'}</span><span className={s.hideNarrow}>{songs ? '' : 'Rating'}</span><span className={s.hideNarrow}>{songs ? 'Album' : 'Length'}</span><span className={s.hideNarrow}>{songs ? '' : 'Status'}</span><span />
    </div>
  );
  return (
    <WindowedList source={source} rowHeight={60} header={header} renderRow={e => e ? (
      <div className={s.listRow} role="listitem">
        <Artwork src={e.image} title={e.title} shape={e.shape === 'circle' ? 'circle' : e.shape === 'wide' ? 'square' : e.shape} className={s.listThumb} tint={e.tint} />
        <span className={s.listTitle}>
          {hrefFor(e) ? <Link to={hrefFor(e)! as never} onClick={ev => { if (e.kind === 'track') { ev.preventDefault(); void actions.play(e); } }}>{e.title}</Link> : <button type="button" style={{textAlign: 'left', fontWeight: 650}} onClick={() => void actions.play(e)}>{e.title}</button>}
          <span className={s.listSub}>{e.kind === 'track' ? e.subtitle : e.genres?.slice(0, 2).join(', ') || e.subtitle}</span>
        </span>
        <span className={s.listCell}>{songs ? formatDuration(e.duration) : e.year}</span>
        <span className={`${s.listCell} ${s.hideNarrow}`}>{songs ? '' : e.rating}</span>
        <span className={`${s.listCell} ${s.hideNarrow}`}>{songs ? e.caption ?? '' : e.kind === 'show' ? e.caption?.split(' · ')[0] : formatDuration(e.duration)}</span>
        <span className={`${s.listCell} ${s.hideNarrow}`}>{songs ? '' : e.watched ? <span className={s.watchedDot} title="Watched"><Icon name="check" size={12} strokeWidth={3} /></span> : e.progress ? `${Math.round(e.progress * 100)}% watched` : ''}</span>
        <Menu label={`More for ${e.title}`} items={actions.items(e)} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`More for ${e.title}`} />} />
      </div>
    ) : <div className={s.listRow}><div className="skeleton" style={{height: 44, width: 36}} /><div className="skeleton" style={{height: 12, width: '50%'}} /></div>} />
  );
}

/** Songs: number and art, title and artist, album, length, menu. Clicking a song plays from it. */
function SongList({source}: {source: PageSource}) {
  const actions = useActions();
  const header = <div className={`${s.listHead} ${s.songs}`} role="presentation"><span /><span>Title</span><span className={s.hideNarrow}>Album</span><span style={{textAlign: 'right'}}>Time</span><span /></div>;
  return (
    <WindowedList source={source} rowHeight={60} header={header} renderRow={e => e ? (
      <div className={`${s.listRow} ${s.songs}`} role="listitem" onDoubleClick={() => void actions.play(e)}>
        <button type="button" className={s.songArt} onClick={() => void actions.play(e)} aria-label={`Play ${e.title}`}><Artwork src={e.image} title={e.title} shape="square" className={s.listThumb} tint={e.tint} /><span className={s.songPlay}><Icon name="play" size={14} /></span></button>
        <span className={s.listTitle}><button type="button" className={s.songTitle} onClick={() => void actions.play(e)}>{e.title}</button><span className={s.listSub}>{e.subtitle}</span></span>
        <span className={`${s.listCell} ${s.hideNarrow}`}>{e.parentId ? <Link to={`/media/${e.parentId}` as never} className={s.songAlbum}>{e.caption}</Link> : e.caption}</span>
        <span className={`${s.listCell} tnum`} style={{textAlign: 'right'}}>{formatTime(e.duration ?? 0)}</span>
        <Menu label={`More for ${e.title}`} items={actions.items(e)} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`More for ${e.title}`} />} />
      </div>
    ) : <div className={`${s.listRow} ${s.songs}`}><div className="skeleton" style={{height: 36, width: 36}} /><div className="skeleton" style={{height: 12, width: '50%'}} /></div>} />
  );
}
