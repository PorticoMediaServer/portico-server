import React, {useState} from 'react';
import {Link, useNavigate} from '@tanstack/react-router';
import {useClient, useQuery} from '@/data/DataProvider';
import type {Entry, HistoryEntry, SavedResource, SavedTab} from '@/data/types';
import {hrefFor} from '@/app/links';
import {ago, plural} from '@/app/format';
import {useActions} from '@/app/actions';
import {Button, IconButton} from '@/ui/Button';
import {Artwork, Badge, EmptyState, ErrorState} from '@/ui/Bits';
import {ConfirmDialog, Dialog, Menu, useToast} from '@/ui/Overlay';
import {Segmented, Select, TextField} from '@/ui/Form';
import {Icon} from '@/ui/Icon';
import {CardGrid} from '@/ui/Shelf';
import {LinkTabs} from '@/ui/Tabs';
import {PageHeader, Toolbar} from '@/ui/Page';
import {usePhone, useTitle} from '@/ui/hooks';
import {useRouteSearch} from '../common';
import s from './Saved.module.css';

const TABS: {id: SavedTab; label: string}[] = [
  {id: 'watchlist', label: 'Watchlist'}, {id: 'favorites', label: 'Favorites'}, {id: 'playlists', label: 'Playlists'},
  {id: 'collections', label: 'Collections'}, {id: 'views', label: 'Saved views'}, {id: 'history', label: 'History'},
];

/** Everything this profile keeps: Watchlist, Favorites, playlists, personal collections, saved views and history. */
export default function SavedScreen() {
  const search = useRouteSearch<{view?: string; sort?: string; filter?: string; period?: string}>();
  const tab = (TABS.find(t => t.id === search.view)?.id ?? 'watchlist') as SavedTab;
  useTitle(TABS.find(t => t.id === tab)!.label);
  return (
    <div style={{padding: '28px 0 72px'}}>
      <PageHeader title="Saved" subtitle="What you’ve kept for later, your playlists and collections, and what you’ve watched." />
      <div className={s.tabs}><LinkTabs label="Saved" current={tab} tabs={TABS.map(t => ({id: t.id, label: t.label, href: `/saved?view=${t.id}`}))} /></div>
      {tab === 'watchlist' || tab === 'favorites' ? <EntriesTab tab={tab} search={search} />
        : tab === 'history' ? <HistoryTab period={search.period ?? '30d'} />
        : <ResourcesTab tab={tab} />}
    </div>
  );
}

function useSetSearch() {
  const navigate = useNavigate();
  const search = useRouteSearch<Record<string, string | undefined>>();
  return (patch: Record<string, string | undefined>) => void navigate({to: '/saved' as never, search: {...search, ...patch} as never, replace: true});
}

function EntriesTab({tab, search}: {tab: 'watchlist' | 'favorites'; search: {sort?: string; filter?: string}}) {
  const set = useSetSearch();
  const phone = usePhone();
  const filter = (search.filter ?? 'all') as 'all' | 'unwatched' | 'inProgress';
  const sort = search.sort ?? 'added';
  const q = useQuery(c => c.saved.list(tab, {filter, sort}), [tab, filter, sort], ['personal', 'saved']);
  if (q.error) return <ErrorState onRetry={q.refresh} />;
  return (
    <>
      <Toolbar label="Filter and sort">
        <Segmented label="Show" value={filter} onChange={v => set({filter: v === 'all' ? undefined : v})} options={[{value: 'all', label: 'All'}, {value: 'unwatched', label: 'Unwatched'}, {value: 'inProgress', label: 'In progress'}]} />
        <span style={{flex: 1}} />
        {q.data ? <span className="t-caption c-3">{plural(q.data.entries.length, 'title')}</span> : null}
        <Select compact ariaLabel="Sort" value={sort} onChange={v => set({sort: v})} options={[{value: 'added', label: phone ? 'Date added' : 'Sort: Date added'}, {value: 'title', label: phone ? 'Title' : 'Sort: Title'}, {value: 'year', label: phone ? 'Year' : 'Sort: Release year'}, {value: 'duration', label: phone ? 'Length' : 'Sort: Length'}]} />
      </Toolbar>
      {q.data && !q.data.entries.length ? (
        filter !== 'all' ? <EmptyState icon="filter" title="Nothing matches" body="Nothing here matches this filter." actions={[{label: 'Show all', onClick: () => set({filter: undefined})}]} />
          : tab === 'watchlist' ? <EmptyState icon="saved" title="Your Watchlist is empty" body="Add movies and shows you want to watch later with the bookmark button on any title." actions={[{label: 'Browse movies', to: '/library/lib-movies?view=browse'}]} />
          : <EmptyState icon="heart" title="No favorites yet" body="Titles, albums and artists you mark with the heart show up here." />
      ) : <CardGrid entries={q.data?.entries} loading={q.loading} />}
    </>
  );
}

export function Mosaic({images, title, count}: {images?: (string | undefined)[]; title: string; count?: number}) {
  const list = (images ?? []).filter(Boolean) as string[];
  if (!list.length) return <div className={`${s.mosaic} ${s.mosaicOne} ${s.mosaicEmpty}`} aria-hidden><Icon name="playlist" size={36} /></div>;
  if (list.length < 4) return <div className={`${s.mosaic} ${s.mosaicOne}`} aria-hidden><img src={list[0]} alt="" loading="lazy" /></div>;
  return <div className={s.mosaic} aria-label={count !== undefined ? `${title}, ${count} items` : undefined}>{list.slice(0, 4).map((src, i) => <img key={i} src={src} alt="" loading="lazy" />)}</div>;
}

function ResourcesTab({tab}: {tab: 'playlists' | 'collections' | 'views'}) {
  const q = useQuery(c => c.saved.list(tab), [tab], ['saved']);
  const client = useClient();
  const navigate = useNavigate();
  const toast = useToast();
  const [creating, setCreating] = useState(false);
  const kind = tab === 'playlists' ? 'playlist' : tab === 'collections' ? 'collection' : 'view';
  if (q.error) return <ErrorState onRetry={q.refresh} />;
  if (!q.data) return <CardGrid loading shape="square" />;
  const list = q.data.resources;
  if (!list.length && tab === 'views') return <EmptyState icon="filter" title="No saved views" body="In any library’s Browse, set filters and a sort, then choose Save this view. It shows up here and can be pinned to Home." actions={[{label: 'Browse movies', to: '/library/lib-movies?view=browse'}]} />;
  return (
    <>
      <div className={s.resGrid}>
        {tab !== 'views' ? <button type="button" className={s.newTile} onClick={() => setCreating(true)}><Icon name="plus" size={28} />New {kind}</button> : null}
        {list.map(r => <ResourceCard key={r.id} r={r} />)}
      </div>
      {!list.length ? <EmptyState compact icon={tab === 'playlists' ? 'playlist' : 'collection'} title={`No ${tab} yet`} body={tab === 'playlists' ? 'Playlists keep songs and videos in the order you choose.' : 'Collections group titles you pick, like Date Night.'} /> : null}
      <NameDialog open={creating} onOpenChange={setCreating} title={`New ${kind}`} action="Create" onSave={async (name, description) => { const id = await (kind === 'playlist' ? client.saved.createPlaylist(name, [], description || undefined) : client.saved.createCollection(name, [], description || undefined)); toast.show(`Created ${name}`, {icon: 'check'}); void navigate({to: `/saved/${kind}/${id}` as never}); }} />
    </>
  );
}

function ResourceCard({r}: {r: SavedResource}) {
  return (
    <Link to={`/saved/${r.kind}/${r.id}` as never} className={s.res}>
      <div style={{position: 'relative'}}>
        <Mosaic images={r.images} title={r.title} count={r.count} />
        {r.shared ? <Badge tone="accent" className={s.resBadge}>Shared</Badge> : r.smart ? <Badge className={s.resBadge}>Smart</Badge> : null}
      </div>
      <span className={s.resTitle}>{r.title}</span>
      <span className={s.resSub}>{plural(r.count, 'item')}</span>
    </Link>
  );
}

export function NameDialog({open, onOpenChange, title, action, initial = '', initialDescription = '', onSave}: {open: boolean; onOpenChange: (o: boolean) => void; title: string; action: string; initial?: string; initialDescription?: string; onSave: (name: string, description: string) => Promise<void>}) {
  const [name, setName] = useState(initial);
  const [description, setDescription] = useState(initialDescription);
  const [busy, setBusy] = useState(false);
  React.useEffect(() => { if (open) { setName(initial); setDescription(initialDescription); } }, [open, initial, initialDescription]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} width={460} footer={<>
      <Button variant="ghost" label="Cancel" onClick={() => onOpenChange(false)} />
      <Button variant="primary" label={action} busy={busy} disabled={!name.trim()} onClick={async () => { setBusy(true); await onSave(name.trim(), description.trim()); setBusy(false); onOpenChange(false); }} />
    </>}>
      <div style={{display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', gap: 14}}>
        <TextField label="Name" value={name} onChange={setName} autoFocus />
        <TextField label="Description" help="Optional" value={description} onChange={setDescription} multiline />
      </div>
    </Dialog>
  );
}

function HistoryTab({period}: {period: string}) {
  const set = useSetSearch();
  const client = useClient();
  const toast = useToast();
  const actions = useActions();
  const [confirm, setConfirm] = useState(false);
  const q = useQuery(c => c.saved.list('history', {period}), [period], ['personal', 'saved']);
  const days = new Map<string, HistoryEntry[]>();
  for (const h of q.data?.history ?? []) {
    const key = new Date(h.watchedAt).toDateString() === new Date().toDateString() ? 'Today' : new Date(h.watchedAt).toDateString() === new Date(Date.now() - 86400000).toDateString() ? 'Yesterday' : new Date(h.watchedAt).toLocaleDateString('en-US', {weekday: 'long', month: 'long', day: 'numeric'});
    days.set(key, [...(days.get(key) ?? []), h]);
  }
  return (
    <>
      <Toolbar label="History period">
        <Segmented label="Period" value={period} onChange={v => set({period: v})} options={[{value: '24h', label: '24 hours'}, {value: '7d', label: '7 days'}, {value: '30d', label: '30 days'}, {value: '90d', label: '90 days'}, {value: 'all', label: 'All'}]} />
        <span style={{flex: 1}} />
        <Button variant="ghost" icon="trash" label="Clear history" onClick={() => setConfirm(true)} disabled={!q.data?.history.length} />
      </Toolbar>
      {q.error ? <ErrorState onRetry={q.refresh} /> : !q.data ? <CardGrid loading skeletons={6} /> : !q.data.history.length ? <EmptyState icon="history" title="Nothing watched in this period" body="What you watch and listen to shows up here. Pausing watch history in Settings keeps it off this list." /> : (
        [...days.entries()].map(([day, list]) => (
          <section key={day} className={s.day}>
            <h2 className={s.dayTitle}>{day}</h2>
            {list.map(h => <HistoryRow key={h.entry.id + h.watchedAt} h={h} items={actions.items(h.entry)} />)}
          </section>
        ))
      )}
      <ConfirmDialog open={confirm} onOpenChange={setConfirm} title="Clear your watch history?" body="This clears the history list for this profile. Watched marks, Continue Watching and your ratings stay. Recommendations will take a little while to adjust." action="Clear history" tone="danger" onConfirm={async () => { await client.saved.clearHistory(); setConfirm(false); toast.show('History cleared', {icon: 'check'}); }} />
    </>
  );
}

function HistoryRow({h, items}: {h: HistoryEntry; items: ReturnType<ReturnType<typeof useActions>['items']>}) {
  const e: Entry = h.entry;
  const href = hrefFor(e);
  return (
    <div className={s.hist}>
      <Artwork src={e.kind === 'episode' ? e.poster ?? e.image : e.image} title={e.title} shape={e.shape === 'circle' || e.shape === 'square' ? 'square' : 'poster'} tint={e.tint} />
      <div className={s.histTitle} style={{minWidth: 0}}>{href ? <Link to={href as never}>{e.kind === 'episode' ? e.subtitle : e.title}</Link> : e.title}<div className="t-caption c-3 clamp-1">{e.kind === 'episode' ? `${e.caption ?? ''} · ${e.title}` : [e.year, e.kind === 'movie' ? 'Movie' : e.kind === 'audiobook' ? 'Audiobook' : null].filter(Boolean).join(' · ')}</div></div>
      <span className="t-caption c-3">{ago(h.watchedAt)}</span>
      <Menu label={`More for ${e.title}`} items={items} trigger={<IconButton size="sm" variant="ghost" icon="more" label={`More for ${e.title}`} />} />
    </div>
  );
}
