import React, {useEffect, useMemo, useState} from 'react';
import {Link, useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {useQuery, useServices} from '../../data/hooks';
import type {Audiobook, FilterState, Item, Library, LibraryKind, Shape, SortState} from '../../data/types';
import {useSession} from '../../app/session';
import {selection, useSelection} from '../../app/selection';
import {usePrefs} from '../../app/prefs';
import {setTint} from '../../app/tint';
import {Page} from '../../ui/Layout';
import {Button, IconButton} from '../../ui/Button';
import {Chip, Segmented, Select} from '../../ui/Controls';
import {Badge, ErrorState, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {Menu, Tooltip} from '../../ui/Overlay';
import {cx} from '../../ui/cx';
import {libraryIcon} from '../../shell/Shell';
import {Hero} from '../home/Hero';
import {RowSkeleton} from '../home/Home';
import {Shelf} from '../shared/Shelf';
import {MediaCard} from '../shared/Cards';
import {TrackRow} from '../detail/Media';
import {factsFor, hrefFor, plural, subtitleFor} from '../shared/items';
import {player} from '../../app/player';
import {useBrowse} from './useBrowse';
import {WindowedGrid} from './WindowedGrid';
import {FiltersSheet, QUICK, SaveViewDialog, SORTS, describeField} from './Filters';

const VIEW_LABEL: Record<string, string> = {discover: 'Discover', browse: 'Browse', collections: 'Collections', categories: 'Categories', releases: 'Releases', songs: 'Songs', authors: 'Authors', series: 'Series', albums: 'Albums', timeline: 'Timeline'};
const noun = (k: LibraryKind) => (k === 'music' ? 'artist' : k === 'photos' ? 'album' : k === 'audiobooks' ? 'book' : k === 'homevideos' ? 'video' : k === 'shows' || k === 'anime' ? 'show' : 'film');

export function LibraryScreen() {
  const {libraryId} = useParams({from: '/app/library/$libraryId'});
  const search = useSearch({from: '/app/library/$libraryId'});
  const services = useServices();
  const navigate = useNavigate();
  const session = useSession();
  const libs = useQuery('libraries', () => services.catalog.libraries());
  const lib = libs.data?.find(l => l.id === libraryId);
  useEffect(() => { selection.end(); }, [libraryId]);
  if (libs.error) return <Page><ErrorState error={libs.error} onRetry={libs.reload} what="this library" /></Page>;
  if (!lib) return libs.loading ? <Page><Skeleton w={260} h={48} /><RowSkeleton /></Page> : <Page title="Library"><StateView icon="library" title="This library isn’t on the server any more" body="It may have been removed or renamed by the server owner." action={{label: 'Go home', to: '/', icon: 'home'}} /></Page>;
  const view = search.view && lib.views.includes(search.view as never) ? search.view : lib.views[0]!;
  const menu = [
    {id: 'scan', label: 'Scan library files', icon: 'refresh' as const, onSelect: () => void services.catalog.scanLibrary(lib.id).then(() => toast(`Scanning ${lib.name} for new and changed files`, {icon: 'refresh'}))},
    ...(session.manager ? [{id: 'manage', label: 'Manage library', icon: 'settings' as const, onSelect: () => void navigate({to: '/server/$section', params: {section: 'libraries'}, search: {id: lib.id}})}] : []),
  ];
  return (
    <Page title={lib.name} flush className="library-page">
      {view === 'discover' ? <DiscoverHero lib={lib} /> : null}
      <header className={cx('lib-head', view === 'discover' && 'is-over')}>
        <div className="lib-titles">
          <div className="eyebrow"><Icon name={libraryIcon[lib.kind]} size={13} /> {labelFor(lib.kind)} · {plural(lib.count, noun(lib.kind))}</div>
          <h1 className="display lib-title">{lib.name}</h1>
        </div>
        <div className="lib-actions">
          {session.manager ? <Menu label="Library options" items={menu} trigger={<button type="button" className="btn btn-glass btn-md btn-icon btn-round" aria-label="Library options"><Icon name="more" size={19} /></button>} /> : <IconButton icon="refresh" label="Scan library files" onClick={() => menu[0]!.onSelect()} />}
        </div>
      </header>
      {lib.scanning ? <div className="page-pad"><div className="scan-banner"><span className="rail-scan"><span /></span><span><strong>Scanning {lib.name}.</strong> Found 128 new files so far; they appear here as they’re matched.</span></div></div> : null}
      <nav className="page-pad lib-tabs" aria-label={`${lib.name} views`}>
        <div className="tabs" role="tablist">
          {lib.views.map(v => <Link key={v} to="/library/$libraryId" params={{libraryId}} search={{view: v}} role="tab" aria-selected={v === view} className={cx('tab', v === view && 'is-on')}>{VIEW_LABEL[v] ?? v}</Link>)}
        </div>
      </nav>
      {view === 'discover' ? <Discover lib={lib} /> : null}
      {view === 'browse' || view === 'releases' || view === 'albums' ? <Browse lib={lib} kind={view === 'releases' || view === 'albums' ? 'releases' : undefined} /> : null}
      {view === 'songs' ? <Songs lib={lib} /> : null}
      {view === 'collections' ? <Collections lib={lib} /> : null}
      {view === 'categories' ? <Categories lib={lib} /> : null}
      {view === 'authors' || view === 'series' ? <BookGroups lib={lib} by={view} /> : null}
      {view === 'timeline' ? <Timeline lib={lib} /> : null}
    </Page>
  );
}

const labelFor = (k: LibraryKind) => ({movies: 'Movies', shows: 'TV', anime: 'Anime', music: 'Music', audiobooks: 'Audiobooks', photos: 'Photos', homevideos: 'Home videos'})[k];

function DiscoverHero({lib}: {lib: Library}) {
  const services = useServices();
  const rows = useQuery(`discover.${lib.id}`, () => services.catalog.discover(lib.id));
  const ids = rows.data?.find(r => r.kind === 'picks' || r.kind === 'recent')?.itemIds.slice(0, 16) ?? [];
  const items = useQuery(ids.length ? `discover.hero.${lib.id}` : null, () => services.catalog.items(ids));
  const list = (items.data ?? []).filter(i => i.art.backdrop).slice(0, 5);
  if (!list.length || lib.kind === 'music' || lib.kind === 'photos') return <div className="lib-spacer" />;
  return <div className="lib-hero"><Hero items={list} eyebrow={`New in ${lib.name}`} /></div>;
}

function Discover({lib}: {lib: Library}) {
  const services = useServices();
  const rows = useQuery(`discover.${lib.id}`, () => services.catalog.discover(lib.id));
  const ids = [...new Set((rows.data ?? []).flatMap(r => r.itemIds.slice(0, 18)))];
  const items = useQuery(rows.data ? `discover.items.${lib.id}.${ids.length}` : null, () => services.catalog.items(ids));
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  if (rows.error) return <ErrorState error={rows.error} onRetry={rows.reload} what="Discover" />;
  if (!rows.data || !items.data) return <>{[0, 1, 2].map(i => <RowSkeleton key={i} shape={lib.kind === 'music' ? 'square' : lib.kind === 'photos' || lib.kind === 'homevideos' ? 'landscape' : 'poster'} />)}</>;
  if (!rows.data.length) return <EmptyLibrary lib={lib} />;
  return (
    <div className="lib-discover">
      {rows.data.map(row => {
        const list = row.itemIds.map(id => byId.get(id)).filter((x): x is Item => !!x);
        if (!list.length) return null;
        return (
          <Shelf key={row.id} id={row.id} title={row.title} subject={row.subject} eyebrow={row.recommendation ? 'For you' : undefined}>
            {list.map(i => <MediaCard key={i.id} item={i} context={row.kind === 'continue' ? 'continue' : row.recommendation ? 'recommendation' : 'card'} shape={row.shape === 'circle' ? 'circle' : undefined} />)}
          </Shelf>
        );
      })}
    </div>
  );
}

function EmptyLibrary({lib}: {lib: Library}) {
  const session = useSession();
  const navigate = useNavigate();
  return <StateView icon={libraryIcon[lib.kind]} title={`${lib.name} is empty`} body={session.manager ? `Portico looked in ${lib.sourcePaths.join(' and ')} and didn’t find anything it can play. Add files there, or check the folder in the library’s settings.` : 'Nothing has been added to this library yet.'} action={session.manager ? {label: 'Manage library', icon: 'settings', onClick: () => void navigate({to: '/server/$section', params: {section: 'libraries'}})} : undefined} />;
}

// ---- Browse

export function parseFilters(raw?: string): FilterState {
  try {
    const v = raw ? JSON.parse(raw) : null;
    if (v && typeof v === 'object') return {quick: Array.isArray(v.quick) ? v.quick : [], fields: v.fields && typeof v.fields === 'object' ? v.fields : {}, yearFrom: v.yearFrom, yearTo: v.yearTo};
  } catch { /* ignore a bad link */ }
  return {quick: [], fields: {}};
}

function Browse({lib, kind}: {lib: Library; kind?: string}) {
  const search = useSearch({from: '/app/library/$libraryId'});
  const navigate = useNavigate();
  const {prefs, update} = usePrefs();
  const services = useServices();
  const sel = useSelection();
  const filters = useMemo(() => parseFilters(search.filters), [search.filters]);
  const sort: SortState = {field: search.sort ?? (lib.kind === 'music' || lib.kind === 'audiobooks' ? 'title' : 'added'), direction: search.direction ?? (search.sort === 'title' || (!search.sort && (lib.kind === 'music' || lib.kind === 'audiobooks')) ? 'asc' : 'desc')};
  const layout = search.layout ?? prefs.libraryLayout;
  const query = {libraryId: lib.id, filters, sort, kind};
  const data = useBrowse(query);
  const letters = useQuery(`letters.${JSON.stringify(query)}`, () => services.catalog.letters(query));
  const [sheet, setSheet] = useState(() => new URLSearchParams(location.search).get('demo') === 'filters');
  const [saving, setSaving] = useState(false);
  const [jump, setJump] = useState<number | null>(null);
  const shape: Shape = lib.kind === 'music' ? (kind === 'releases' ? 'square' : 'circle') : lib.kind === 'photos' || lib.kind === 'homevideos' ? 'landscape' : 'poster';
  const set = (patch: {filters?: FilterState; sort?: string; direction?: 'asc' | 'desc'; layout?: 'grid' | 'list'}) => {
    const f = patch.filters ?? filters;
    const empty = !f.quick.length && !Object.values(f.fields).some(v => v.length) && !f.yearFrom && !f.yearTo;
    void navigate({to: '/library/$libraryId', params: {libraryId: lib.id}, search: s => ({...s, filters: empty ? undefined : JSON.stringify(f), sort: patch.sort ?? s.sort, direction: patch.direction ?? s.direction, layout: patch.layout ?? s.layout}), replace: true, resetScroll: false});
    if (patch.layout) update({libraryLayout: patch.layout});
  };
  const toggleQuick = (q: string) => set({filters: {...filters, quick: filters.quick.includes(q) ? filters.quick.filter(x => x !== q) : [...filters.quick, q]}});
  const active = [...filters.quick.map(q => ({id: `q:${q}`, label: QUICK.find(x => x.id === q)?.label ?? q, remove: () => toggleQuick(q)})),
    ...Object.entries(filters.fields).flatMap(([f, vals]) => vals.map(v => ({id: `${f}:${v}`, label: describeField(f, v), remove: () => set({filters: {...filters, fields: {...filters.fields, [f]: vals.filter(x => x !== v)}}})}))),
    ...(filters.yearFrom || filters.yearTo ? [{id: 'year', label: `${filters.yearFrom ?? '…'}–${filters.yearTo ?? '…'}`, remove: () => set({filters: {...filters, yearFrom: undefined, yearTo: undefined}})}] : [])];
  const quick = QUICK.filter(q => q.kinds.includes(lib.kind));
  const sorts = SORTS.filter(s => s.kinds.includes(lib.kind));
  return (
    <div className="page-pad browse">
      <div className="browse-bar">
        <div className="chip-row browse-quick" role="group" aria-label="Quick filters">
          {quick.map(q => <Chip key={q.id} on={filters.quick.includes(q.id)} onClick={() => toggleQuick(q.id)} icon={q.icon}>{q.label}</Chip>)}
        </div>
        <div className="browse-tools">
          <Button variant="secondary" size="sm" icon="sliders" label={`Filters${active.length ? ` · ${active.length}` : ''}`} onClick={() => setSheet(true)} />
          <Select label="Sort by" value={sort.field} onChange={v => set({sort: v})} options={sorts.map(s => ({value: s.id, label: s.label}))} className="sort-select" />
          <Tooltip label={sort.direction === 'asc' ? 'Ascending' : 'Descending'}><button type="button" className="btn btn-secondary btn-sm btn-icon" onClick={() => set({direction: sort.direction === 'asc' ? 'desc' : 'asc'})} aria-label={`Order: ${sort.direction === 'asc' ? 'ascending' : 'descending'}`}><Icon name={sort.direction === 'asc' ? 'up' : 'down'} size={16} /></button></Tooltip>
          <Segmented size="sm" label="Layout" value={layout} onChange={v => set({layout: v})} options={[{value: 'grid', label: <span className="sr-only">Grid</span>, icon: 'grid'}, {value: 'list', label: <span className="sr-only">List</span>, icon: 'list'}]} />
          {!sel.active ? <Button variant="ghost" size="sm" icon="checkAll" label="Select" onClick={() => selection.start(lib.id)} /> : null}
        </div>
      </div>
      <div className="browse-meta">
        <span className="browse-count" aria-live="polite">{data.total === null ? 'Loading…' : plural(data.total, kind === 'releases' ? 'release' : noun(lib.kind))}</span>
        {active.map(a => <Chip key={a.id} on removable onClick={a.remove}>{a.label}</Chip>)}
        {active.length ? <button type="button" className="link-btn" onClick={() => set({filters: {quick: [], fields: {}}})}>Clear all</button> : null}
        <span className="spacer" />
        {active.length ? <Button variant="ghost" size="sm" icon="save" label="Save view" onClick={() => setSaving(true)} /> : null}
      </div>
      <div className={cx('browse-body', letters.data && letters.data.length > 3 && sort.field === 'title' && layout === 'grid' && 'has-alpha')}>
        {data.error ? <ErrorState error={data.error} onRetry={data.reload} what="this library" /> : data.total === 0 ? (
          active.length ? <StateView icon="filter" title="Nothing matches these filters" body="Try removing a filter or two." action={{label: 'Clear all filters', onClick: () => set({filters: {quick: [], fields: {}}})}} /> : <EmptyLibrary lib={lib} />
        ) : layout === 'list' ? (
          <BrowseList items={data.loaded} total={data.total ?? 0} more={() => data.ensure(data.loaded.length, data.loaded.length + 59)} />
        ) : (
          <WindowedGrid total={data.total ?? 18} at={data.at} ensure={data.ensure} shape={shape} jumpTo={jump} onJumped={() => setJump(null)} />
        )}
        {letters.data && letters.data.length > 3 && sort.field === 'title' && layout === 'grid' ? (
          <nav className="alpha" aria-label="Jump to letter">
            {letters.data.map(l => <button key={l.letter} type="button" onClick={() => setJump(l.index)} aria-label={`Jump to ${l.letter === '#' ? 'numbers' : l.letter}`}>{l.letter}</button>)}
          </nav>
        ) : null}
      </div>
      <FiltersSheet open={sheet} onOpenChange={setSheet} lib={lib} filters={filters} onApply={f => set({filters: f})} />
      <SaveViewDialog open={saving} onOpenChange={setSaving} lib={lib} filters={filters} sort={sort} />
    </div>
  );
}

function BrowseList({items, total, more}: {items: Item[]; total: number; more: () => void}) {
  const services = useServices();
  return (
    <div className="browse-list" role="list">
      {items.map(i => {
        const st = services.user.state(i.id);
        return (
          <Link key={i.id} to={hrefFor(i as never) as never} className="blist-row" role="listitem" onMouseEnter={() => setTint(i.tint)}>
            <Art src={i.art.poster ?? i.art.square ?? i.art.thumb ?? i.art.backdrop} tint={i.tint} className="blist-art" ratio={i.art.poster ? '2/3' : i.art.square ? '1/1' : '16/9'} />
            <span className="blist-main"><strong>{i.title}</strong><span>{subtitleFor(i)}</span></span>
            <span className="blist-facts">{factsFor(i).slice(1).join(' · ') || i.genres.slice(0, 2).join(', ')}</span>
            <span className="blist-genres">{i.genres.slice(0, 2).join(', ')}</span>
            <span className="blist-state">{st.watched ? <Badge tone="good" icon="check">Watched</Badge> : st.progress > 0 ? <Badge tone="accent">In progress</Badge> : null}</span>
          </Link>
        );
      })}
      {items.length < total ? <div className="blist-more"><Button variant="secondary" label={`Show more (${(total - items.length).toLocaleString('en-US')} left)`} onClick={more} /></div> : null}
    </div>
  );
}

function Songs({lib}: {lib: Library}) {
  const data = useBrowse({libraryId: lib.id, filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}, kind: 'songs'});
  const tracks = data.loaded.filter(i => i.kind === 'track');
  return (
    <div className="page-pad">
      <div className="browse-meta"><span className="browse-count">{data.total === null ? 'Loading…' : plural(data.total, 'song')}</span><span className="spacer" /><Button variant="secondary" size="sm" icon="shuffle" label="Shuffle all" onClick={() => player.play(tracks.map(t => t.id).sort(() => Math.random() - 0.5), {kind: 'audio', shuffle: true})} /></div>
      <div className="tracklist" role="table" aria-label="Songs">
        {tracks.map((t, i) => <TrackRow key={t.id} track={t as never} index={i + 1} showArt showAlbum onPlay={() => player.play(tracks.map(x => x.id), {kind: 'audio', index: i})} />)}
      </div>
      {data.total !== null && tracks.length < data.total ? <div className="blist-more"><Button variant="secondary" label="Show more" onClick={() => data.ensure(tracks.length, tracks.length + 59)} /></div> : null}
    </div>
  );
}

function Collections({lib}: {lib: Library}) {
  const services = useServices();
  const cols = useQuery(`collections.${lib.id}`, () => services.catalog.collections(lib.id));
  const ids = (cols.data ?? []).flatMap(c => c.itemIds.slice(0, 3));
  const items = useQuery(cols.data ? `collections.items.${lib.id}` : null, () => services.catalog.items(ids));
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  if (!cols.data) return <div className="page-pad tile-grid">{[0, 1, 2, 3].map(i => <Skeleton key={i} h={200} r={16} />)}</div>;
  if (!cols.data.length) return <StateView icon="layers" title="No collections yet" body="Collections group titles that belong together, like a film series. Portico makes some automatically; you can make your own from any title’s More menu." />;
  return (
    <div className="page-pad tile-grid is-collections">
      {cols.data.map(c => {
        const posters = c.itemIds.slice(0, 3).map(id => byId.get(id)).filter((x): x is Item => !!x);
        return (
          <Link key={c.id} to="/library/$libraryId/$view/$entityId" params={{libraryId: lib.id, view: 'collection', entityId: c.id}} className="col-tile" style={{'--c': c.tint} as React.CSSProperties} onMouseEnter={() => setTint(c.tint)} onFocus={() => setTint(c.tint)}>
            <span className="col-stack">{posters.map((p, i) => <Art key={p.id} src={p.art.poster} tint={p.tint} className={`col-p col-p${i}`} />)}</span>
            <span className="col-text"><strong>{c.title}</strong><span>{plural(c.itemIds.length, 'title')}{c.owner === 'me' ? ' · Yours' : ''}{c.shared && c.owner === 'me' ? ' · Shared' : ''}</span></span>
          </Link>
        );
      })}
    </div>
  );
}

function Categories({lib}: {lib: Library}) {
  const services = useServices();
  const cats = useQuery(`categories.${lib.id}`, () => services.catalog.categories(lib.id));
  if (!cats.data) return <div className="page-pad tile-grid">{Array.from({length: 8}, (_, i) => <Skeleton key={i} h={120} r={16} />)}</div>;
  const groups = ['genre', 'mood', 'decade', 'studio', 'network'] as const;
  const titles = {genre: 'Genres', mood: 'Moods', decade: 'Decades', studio: 'Studios', network: 'Networks'};
  return (
    <div className="page-pad lib-cats">
      {groups.map(g => {
        const list = cats.data!.filter(c => c.kind === g);
        if (!list.length) return null;
        return (
          <section key={g} className="section">
            <h2 className="shelf-title" style={{marginBottom: 14}}>{titles[g]}</h2>
            <div className="tile-grid">
              {list.slice(0, g === 'studio' ? 12 : 24).map(c => (
                <Link key={c.id} to="/library/$libraryId/$view/$entityId" params={{libraryId: lib.id, view: c.kind, entityId: c.id}} className="cat-tile" onMouseEnter={() => setTint(c.tint)} onFocus={() => setTint(c.tint)} style={{'--c': c.tint} as React.CSSProperties}>
                  <Art src={c.art} tint={c.tint} />
                  <span className="cat-scrim" />
                  <span className="cat-text"><strong>{c.title}</strong><span>{plural(c.count, 'title')}</span></span>
                </Link>
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function BookGroups({lib, by}: {lib: Library; by: 'authors' | 'series'}) {
  const data = useBrowse({libraryId: lib.id, filters: {quick: [], fields: {}}, sort: {field: 'title', direction: 'asc'}});
  useEffect(() => { data.ensure(0, 200); }, [data.total]);
  const books = data.loaded as Audiobook[];
  const groups = new Map<string, Audiobook[]>();
  books.forEach(b => {
    const k = by === 'authors' ? b.author : b.series?.name;
    if (k) groups.set(k, [...(groups.get(k) ?? []), b]);
  });
  const list = [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
  if (data.total === null) return <div className="page-pad tile-grid">{[0, 1, 2, 3].map(i => <Skeleton key={i} h={96} r={16} />)}</div>;
  if (!list.length) return <StateView icon="bookOpen" title={by === 'series' ? 'No series yet' : 'No authors yet'} />;
  return (
    <div className="page-pad group-grid">
      {list.map(([name, bs]) => (
        <Link key={name} to="/library/$libraryId/$view/$entityId" params={{libraryId: lib.id, view: by === 'authors' ? 'author' : 'series', entityId: `${by === 'authors' ? 'author' : 'series'}:${name}`}} className="group-tile" onMouseEnter={() => setTint(bs[0]!.tint)}>
          <span className="group-covers">{bs.slice(0, 3).map(b => <Art key={b.id} src={b.art.poster} tint={b.tint} ratio="2/3" />)}</span>
          <span className="col-text"><strong>{name}</strong><span>{plural(bs.length, 'book')}{by === 'series' ? ` · by ${bs[0]!.author}` : ''}</span></span>
        </Link>
      ))}
    </div>
  );
}

function Timeline({lib}: {lib: Library}) {
  const data = useBrowse({libraryId: lib.id, filters: {quick: [], fields: {}}, sort: {field: 'added', direction: 'desc'}});
  useEffect(() => { data.ensure(0, 200); }, [data.total]);
  const years = new Map<number, Item[]>();
  data.loaded.forEach(i => years.set(i.year ?? 0, [...(years.get(i.year ?? 0) ?? []), i]));
  if (data.total === null) return <RowSkeleton shape="landscape" />;
  return (
    <div className="lib-timeline">
      {[...years.entries()].sort((a, b) => b[0] - a[0]).map(([y, list]) => (
        <section key={y} className="tl-year">
          <h2 className="display tl-label page-pad">{y || 'Undated'}</h2>
          <div className="page-pad card-grid is-landscape">{list.map(i => <MediaCard key={i.id} item={i} fill />)}</div>
        </section>
      ))}
    </div>
  );
}

