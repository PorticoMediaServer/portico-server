import React, {useState} from 'react';
import {Link, useNavigate, useSearch} from '@tanstack/react-router';
import {useDataVersion, useQuery, useServices} from '../../data/hooks';
import type {Item, SavedView} from '../../data/types';
import type {SavedSearch} from '../../app/router';
import {setTint} from '../../app/tint';
import {Page, PageHeader, Tabs} from '../../ui/Layout';
import {Button} from '../../ui/Button';
import {Field, Input, Segmented, Select, Switch, TextArea} from '../../ui/Controls';
import {ConfirmDialog, Dialog, DialogClose} from '../../ui/Overlay';
import {ErrorState, Skeleton, StateView, toast} from '../../ui/Feedback';
import {Icon} from '../../ui/Icon';
import {Art} from '../../ui/Media';
import {CardSkeleton, MediaCard} from '../shared/Cards';
import {plural, relativeTime} from '../shared/items';
import {describeView} from '../library/Filters';

const TABS = [
  {value: 'watchlist', label: 'Watchlist'},
  {value: 'favorites', label: 'Favorites'},
  {value: 'playlists', label: 'Playlists'},
  {value: 'collections', label: 'Collections'},
  {value: 'views', label: 'Saved views'},
  {value: 'history', label: 'History'},
] as const;
type Tab = (typeof TABS)[number]['value'];

/** Everything this profile keeps: watchlist, favorites, playlists, collections, saved views and history. */
export function SavedScreen() {
  const search = useSearch({from: '/app/saved'});
  const navigate = useNavigate();
  const tab = (TABS.some(t => t.value === search.view) ? search.view : 'watchlist') as Tab;
  const set = (patch: Partial<SavedSearch>) => void navigate({to: '/saved', search: s => ({...s, ...patch}), replace: true});
  const [creating, setCreating] = useState<'playlist' | 'collection' | null>(null);
  return (
    <Page title="Saved">
      <PageHeader title="Saved" lede="Your Watchlist, favorites, playlists and history, kept for this profile." actions={
        tab === 'playlists' ? <Button variant="secondary" icon="plus" label="New playlist" onClick={() => setCreating('playlist')} /> : tab === 'collections' ? <Button variant="secondary" icon="plus" label="New collection" onClick={() => setCreating('collection')} /> : null
      } />
      <Tabs label="Saved" value={tab} onChange={v => void navigate({to: '/saved', search: {view: v}})} tabs={TABS} />
      {tab === 'watchlist' || tab === 'favorites' ? <ItemList which={tab} search={search} set={set} /> : null}
      {tab === 'playlists' ? <Lists kind="playlist" onNew={() => setCreating('playlist')} /> : null}
      {tab === 'collections' ? <Lists kind="collection" onNew={() => setCreating('collection')} /> : null}
      {tab === 'views' ? <Views /> : null}
      {tab === 'history' ? <History period={search.period ?? '30d'} set={set} /> : null}
      <CreateDialog kind={creating} onClose={() => setCreating(null)} />
    </Page>
  );
}

function ItemList({which, search, set}: {which: 'watchlist' | 'favorites'; search: SavedSearch; set: (p: Partial<SavedSearch>) => void}) {
  const services = useServices();
  const version = useDataVersion();
  const rows = useQuery(`row.${which}.${version}`, () => services.catalog.row(which === 'watchlist' ? 'watchlist' : 'favorites'));
  const items = useQuery(rows.data ? `saved.${which}.${version}` : null, () => services.catalog.items(rows.data!.itemIds));
  const filter = search.filter ?? 'all';
  const sort = search.sort ?? 'added';
  let list = (items.data ?? []).filter(i => {
    const st = services.user.state(i.id);
    return filter === 'all' || (filter === 'unwatched' ? !st.watched : st.progress > 0 && !st.watched);
  });
  list = [...list].sort((a, b) => {
    const d = search.direction === 'asc' ? 1 : -1;
    if (sort === 'title') return a.sortTitle.localeCompare(b.sortTitle) * -d;
    if (sort === 'year') return ((a.year ?? 0) - (b.year ?? 0)) * d;
    if (sort === 'recent') return ((services.user.state(a.id).lastPlayedAt ?? 0) - (services.user.state(b.id).lastPlayedAt ?? 0)) * d;
    return (a.addedAt - b.addedAt) * d;
  });
  return (
    <>
      <div className="toolbar">
        <Segmented label="Show" value={filter} onChange={v => set({filter: v})} options={[{value: 'all', label: 'All'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}]} />
        <span className="spacer" />
        <Select label="Sort by" value={sort} onChange={v => set({sort: v})} options={[{value: 'added', label: 'Date added'}, {value: 'title', label: 'Title'}, {value: 'year', label: 'Year'}, {value: 'recent', label: 'Recently watched'}]} />
        <button type="button" className="btn btn-secondary btn-md btn-icon" onClick={() => set({direction: search.direction === 'asc' ? 'desc' : 'asc'})} aria-label={search.direction === 'asc' ? 'Ascending' : 'Descending'}><Icon name={search.direction === 'asc' ? 'up' : 'down'} size={16} /></button>
      </div>
      {rows.error ? <ErrorState error={rows.error} onRetry={rows.reload} /> : !items.data ? (
        <div className="card-grid">{Array.from({length: 12}, (_, i) => <CardSkeleton key={i} fill />)}</div>
      ) : !list.length ? (
        filter !== 'all' ? <StateView icon="filter" title="Nothing matches this filter" action={{label: 'Show all', onClick: () => set({filter: 'all'})}} /> :
        which === 'watchlist' ? <StateView icon="bookmark" title="Your Watchlist is empty" body="Add films and shows you want to watch with the + on any title, and they wait here." action={{label: 'Browse Movies', to: '/library/lib-movies', icon: 'film'}} /> :
        <StateView icon="heart" title="No favorites yet" body="Tap the heart on anything you love, from films to albums, and it collects here." />
      ) : (
        <div className="card-grid">{list.map(i => <MediaCard key={i.id} item={i} fill />)}</div>
      )}
    </>
  );
}

function Lists({kind, onNew}: {kind: 'playlist' | 'collection'; onNew: () => void}) {
  const services = useServices();
  const playlists = useQuery(kind === 'playlist' ? 'lists.playlists' : null, () => services.lists.playlists());
  const collections = useQuery(kind === 'collection' ? 'collections.all' : null, () => services.catalog.collections());
  const lists = kind === 'playlist' ? playlists.data : collections.data;
  const firsts = (lists ?? []).map(l => l.itemIds[0]).filter(Boolean) as string[];
  const covers = useQuery(lists ? `covers.${kind}.${firsts.join()}` : null, () => services.catalog.items(firsts));
  const byId = new Map((covers.data ?? []).map(i => [i.id, i]));
  if (!lists) return <div className="tile-grid">{[0, 1, 2, 3].map(i => <Skeleton key={i} h={120} r={16} />)}</div>;
  if (!lists.length) return <StateView icon={kind === 'playlist' ? 'playlist' : 'layers'} title={`No ${kind}s yet`} body={kind === 'playlist' ? 'Playlists hold songs or videos in the order you choose.' : 'Collections gather titles that belong together.'} action={{label: `New ${kind}`, icon: 'plus', onClick: onNew}} />;
  return (
    <div className="list-grid">
      {lists.map(l => {
        const cover = byId.get(l.itemIds[0] ?? '');
        return (
          <Link key={l.id} to="/saved/$kind/$resourceId" params={{kind, resourceId: l.id}} className="list-tile" onMouseEnter={() => setTint(l.tint)} onFocus={() => setTint(l.tint)} style={{'--c': l.tint} as React.CSSProperties}>
            <span className="list-cover">{cover ? <Art src={cover.art.square ?? cover.art.backdrop ?? cover.art.poster} tint={l.tint} /> : null}<span className="list-glyph"><Icon name={kind === 'playlist' ? 'playlist' : 'layers'} size={22} /></span></span>
            <span className="list-text">
              <strong>{l.title}</strong>
              <span>{plural(l.itemIds.length, kind === 'playlist' && 'media' in l && l.media === 'music' ? 'song' : 'item')}{'shared' in l && l.shared ? ' · Shared' : ''}{'owner' in l && l.owner === 'server' ? ' · From the server' : ''}</span>
              {l.description ? <span className="list-desc">{l.description}</span> : null}
            </span>
          </Link>
        );
      })}
    </div>
  );
}

function Views() {
  const services = useServices();
  const views = useQuery('lists.views', () => services.lists.savedViews());
  const libs = useQuery('libraries', () => services.catalog.libraries());
  if (!views.data) return <div className="tile-grid">{[0, 1, 2].map(i => <Skeleton key={i} h={90} r={16} />)}</div>;
  if (!views.data.length) return <StateView icon="save" title="No saved views" body="In any library, set filters and a sort, then choose Save view. It appears here, and can be a row on Home." />;
  return (
    <div className="view-list">
      {views.data.map((v: SavedView) => {
        const lib = libs.data?.find(l => l.id === v.libraryId);
        const d = describeView(v);
        return (
          <Link key={v.id} to="/saved/$kind/$resourceId" params={{kind: 'view', resourceId: v.id}} className="view-row">
            <span className="row-icon"><Icon name="sliders" size={19} /></span>
            <span className="row-text"><span className="row-label">{v.title}</span><span className="row-help">{lib?.name ?? 'Library'} · {d.filters} · {d.sort.toLowerCase()}</span></span>
            {v.onHome ? <span className="badge badge-accent">On Home</span> : null}
            {v.shared ? <span className="badge">Shared</span> : null}
            <Icon name="chevronRight" size={18} className="row-chevron" />
          </Link>
        );
      })}
    </div>
  );
}

function History({period, set}: {period: string; set: (p: Partial<SavedSearch>) => void}) {
  const services = useServices();
  const h = useQuery(`history.${period}`, () => services.user.history(period));
  const items = useQuery(h.data ? `history.items.${period}.${h.data.length}` : null, () => services.catalog.items(h.data!.map(x => x.itemId)));
  const byId = new Map((items.data ?? []).map(i => [i.id, i]));
  const [clearing, setClearing] = useState(false);
  const groups = new Map<string, {item: Item; at: number}[]>();
  (h.data ?? []).forEach(x => {
    const item = byId.get(x.itemId);
    if (!item) return;
    const d = new Date(x.at);
    const today = new Date();
    const label = d.toDateString() === today.toDateString() ? 'Today' : Date.now() - x.at < 2 * 86400000 && d.getDate() === new Date(Date.now() - 86400000).getDate() ? 'Yesterday' : d.toLocaleDateString('en-US', {weekday: 'long', month: 'long', day: 'numeric'});
    groups.set(label, [...(groups.get(label) ?? []), {item, at: x.at}]);
  });
  return (
    <>
      <div className="toolbar">
        <Segmented label="Period" value={period} onChange={v => set({period: v as never})} options={[{value: '24h', label: '24 hours'}, {value: '7d', label: '7 days'}, {value: '30d', label: '30 days'}, {value: '90d', label: '90 days'}, {value: 'all', label: 'All time'}]} />
        <span className="spacer" />
        <Button variant="ghost" icon="trash" label="Clear history…" onClick={() => setClearing(true)} disabled={!h.data?.length} />
      </div>
      {!h.data || !items.data ? <div className="stack">{[0, 1, 2, 3].map(i => <Skeleton key={i} h={64} />)}</div> : !groups.size ? (
        <StateView icon="history" title="Nothing watched in this period" body="What this profile watches and listens to appears here." action={period !== 'all' ? {label: 'Show all time', onClick: () => set({period: 'all'})} : undefined} />
      ) : (
        [...groups.entries()].map(([label, list]) => (
          <section key={label} className="history-day">
            <h2 className="eyebrow">{label}</h2>
            <div className="history-list">
              {list.map(({item, at}) => (
                <Link key={item.id + at} to={(item.kind === 'episode' ? `/show/${item.showId}/episode/${item.id}` : item.kind === 'show' ? `/show/${item.id}` : `/media/${item.id}`) as never} className="history-row">
                  <Art src={item.art.still ?? item.art.square ?? item.art.poster} tint={item.tint} ratio={item.art.still ? '16/9' : item.art.square ? '1/1' : '2/3'} className="history-art" />
                  <span className="row-text"><span className="row-label">{item.title}</span><span className="row-help">{item.kind === 'album' ? 'Album · listened' : item.kind === 'audiobook' ? 'Audiobook · listened' : item.kind === 'episode' ? 'Episode · watched' : 'Film · watched'} {relativeTime(at).toLowerCase()}</span></span>
                  <Icon name="chevronRight" size={18} className="row-chevron" />
                </Link>
              ))}
            </div>
          </section>
        ))
      )}
      <ConfirmDialog open={clearing} onOpenChange={setClearing} danger title="Clear your viewing history?" body="Everything this profile has watched or listened to is removed from History. Watched marks and progress stay, and recommendations start fresh." confirmLabel="Clear history" onConfirm={() => services.user.clearHistory().then(() => toast('History cleared'))} />
    </>
  );
}

function CreateDialog({kind, onClose}: {kind: 'playlist' | 'collection' | null; onClose: () => void}) {
  const services = useServices();
  const navigate = useNavigate();
  const [title, setTitle] = useState('');
  const [desc, setDesc] = useState('');
  const [shared, setShared] = useState(false);
  const [busy, setBusy] = useState(false);
  const create = async () => {
    if (!kind || !title.trim()) return;
    setBusy(true);
    const r = kind === 'playlist' ? await services.lists.createPlaylist(title.trim(), desc.trim(), shared) : await services.lists.createCollection(title.trim(), desc.trim(), shared);
    setBusy(false);
    setTitle(''); setDesc(''); setShared(false);
    onClose();
    toast(`Created “${r.title}”`);
    void navigate({to: '/saved/$kind/$resourceId', params: {kind, resourceId: r.id}});
  };
  return (
    <Dialog open={!!kind} onOpenChange={o => !o && onClose()} title={kind === 'playlist' ? 'New playlist' : 'New collection'} size="sm" footer={<><DialogClose asChild><Button variant="ghost" label="Cancel" /></DialogClose><Button variant="primary" label="Create" disabled={!title.trim()} busy={busy} onClick={() => void create()} /></>}>
      <form onSubmit={e => { e.preventDefault(); void create(); }} className="stack">
        <Field label="Name"><Input value={title} onChange={e => setTitle(e.target.value)} autoFocus maxLength={80} placeholder={kind === 'playlist' ? 'Road trip' : 'Rainy Sundays'} /></Field>
        <Field label="Description" optional><TextArea value={desc} onChange={e => setDesc(e.target.value)} rows={3} maxLength={300} /></Field>
        <label className="row" style={{padding: 0, border: 0, minHeight: 44}}><span className="row-text"><span className="row-label">Share with everyone on this server</span></span><Switch checked={shared} onChange={setShared} label="Share with everyone" /></label>
      </form>
    </Dialog>
  );
}

