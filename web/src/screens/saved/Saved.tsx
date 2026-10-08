import {cardCaptionOf} from '../../app/content';
import {useSelectionRefresh} from '../../app/selection';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useNavigate, useSearch} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {PersonalSavedService, type PersonalSavedSnapshot} from '@core/personal-saved.ts';
import {SavedService, historyPeriods, savedListFilters, type HistoryPeriod, type PlaylistCard, type PlaylistOccurrence, type SavedEntry, type SavedListFilter, type SavedRoute, type SavedSnapshot} from '@core/saved.ts';
import {useService} from '../../app/content';
import {captionFor, iconFor, progressFor, shapeFor} from '@core/presentation/index.ts';
import {useOpenEntry} from '../../app/open';
import {useSession} from '../../app/session';
import {defaultI18n} from '@i18n';
import {Button, Card, Chip, Chips, ConfirmDialog, Dialog, Grid, Input, Inset, ListRow, Menu, Notice, Page, PageHeader, Row, StateView, Surface, Switch, Tabs, Text, TextArea, type IconName} from '../../ui';
import {LoadingGrid} from '../shared/Sections';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';
import {errorText} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';

type SavedTab = 'watchlist' | 'favorites' | 'playlists' | 'collections' | 'views' | 'history';
const t = defaultI18n.t;
const tabIds: readonly SavedTab[] = ['watchlist', 'favorites', 'playlists', 'collections', 'views', 'history'];
const tabs = (): {id: SavedTab; label: string}[] => [{id: 'watchlist', label: t('saved.tab.watchlist')}, {id: 'favorites', label: t('saved.tab.favorites')}, {id: 'playlists', label: t('saved.tab.playlists')}, {id: 'collections', label: t('saved.tab.collections')}, {id: 'views', label: t('saved.tab.savedViews')}, {id: 'history', label: t('saved.tab.history')}];
const filterLabel = (f: string) => f === 'unwatched' ? t('web.saved.unwatched') : f === 'inProgress' ? t('web.saved.inProgress') : t('web.search.all');
const periodLabel = (p: HistoryPeriod) => p === '24h' ? t('web.live.today') : p === '7d' ? t('web.saved.thisWeek') : p === '30d' ? t('web.saved.days', {count: 30}) : p === '90d' ? t('web.saved.days', {count: 90}) : t('web.saved.allTime');
const emptyCopy = (tab: SavedTab): {icon: IconName; title: string; body: string} => {
  switch (tab) {
    case 'watchlist': return {icon: 'bookmark', title: t('web.saved.empty.watchlist'), body: t('web.saved.empty.watchlistBody')};
    case 'favorites': return {icon: 'heart', title: t('web.saved.empty.favorites'), body: t('web.saved.empty.favoritesBody')};
    case 'playlists': return {icon: 'queue', title: t('web.saved.empty.playlists'), body: t('web.saved.empty.playlistsBody')};
    case 'collections': return {icon: 'collection', title: t('web.saved.empty.collections'), body: t('web.saved.empty.collectionsBody')};
    case 'views': return {icon: 'filter', title: t('web.saved.empty.views'), body: t('web.saved.empty.viewsBody')};
    case 'history': return {icon: 'clock', title: t('web.saved.empty.history'), body: t('web.saved.empty.historyBody')};
  }
};
/** WEB-SAVED-05: consumer names for the server's sort keys ("Updated" means last watched). */
function sortLabel(key: string): string {
  switch (key.replace(/^sort\./, '')) {
    case 'title': return t('web.saved.sort.title');
    case 'added': return t('web.saved.sort.added');
    case 'updated': return t('web.saved.sort.recent');
    case 'year': return t('web.saved.sort.year');
    case 'duration': return t('web.saved.sort.duration');
    case 'progress': return t('web.saved.sort.progress');
    default: return t('web.saved.sort.other');
  }
}
/** WEB-SAVED-04: detail pages and card menus announce a Watchlist, Favorite or watched change on
 * this window event, so a mounted Saved page refreshes instead of showing a removed title. */
export const PERSONAL_CHANGE_EVENT = 'portico:personal-change';
function usePersonalChanges(refresh: () => void) {
  useEffect(() => {
    const on = () => refresh();
    window.addEventListener(PERSONAL_CHANGE_EVENT, on);
    return () => window.removeEventListener(PERSONAL_CHANGE_EVENT, on);
  }, [refresh]);
}
const requestId = () => Promise.resolve(crypto.randomUUID());
const isPlaylistCard = (e: SavedEntry): e is PlaylistCard => e.kind === 'playlist';
const isOccurrence = (e: SavedEntry): e is PlaylistOccurrence => e.kind === 'playlist_entry';
const isMedia = (e: SavedEntry): e is ContentEntry => !isPlaylistCard(e) && !isOccurrence(e);

export function useSaved(route: SavedRoute | null) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<SavedService, SavedSnapshot>(() => new SavedService({api, scope, requestId}), [api, scope]);
  const key = route ? JSON.stringify(route) : '';
  const last = useRef<{key: string; service: unknown}>({key: '', service: null});
  useEffect(() => {
    if (!route || (last.current.key === key && last.current.service === service)) return;
    last.current = {key, service};
    try { void service.select(route).catch(() => {}); } catch {}
  }, [service, key, route]);
  return {service, snapshot, refresh: useCallback(() => void service.refresh().catch(() => {}), [service]), retry: useCallback(() => void service.retry().catch(() => {}), [service]), next: useCallback((id: string) => void service.next(id).catch(() => {}), [service]), previous: useCallback(() => void service.previous().catch(() => {}), [service])};
}
function usePersonal(route: SavedRoute | null) {
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<PersonalSavedService, PersonalSavedSnapshot>(() => new PersonalSavedService(api, scope, requestId), [api, scope]);
  const key = route ? JSON.stringify(route) : '';
  const last = useRef<{key: string; service: unknown}>({key: '', service: null});
  useEffect(() => {
    if (!route || (last.current.key === key && last.current.service === service)) return;
    last.current = {key, service};
    try { void service.select(route).catch(() => {}); } catch {}
  }, [service, key, route]);
  return {service, snapshot, refresh: useCallback(() => void service.refresh().catch(() => {}), [service]), next: useCallback(() => void service.next().catch(() => {}), [service]), previous: useCallback(() => void service.previous().catch(() => {}), [service])};
}

/**
 * Saved: Watchlist, Favourites, Playlists, Collections and History for the
 * signed-in profile. The server authors each projection; the screen picks
 * the peer view, a sort where offered, and pages.
 */
export function SavedScreen() {
  const search = useSearch({from: '/app/saved'});
  const navigate = useNavigate();
  const tab = (tabIds.some(id => id === search.view) ? search.view : 'watchlist') as SavedTab;
  const sort = search.sort;
  const direction = search.direction;
  const filter = search.filter;
  const period = search.period;
  const savedRoute = useMemo<SavedRoute | null>(() => (tab === 'history' || tab === 'collections' || tab === 'views' ? null : {view: tab, ...(sort ? {sort, direction} : {}), ...(filter && filter !== 'all' && (tab === 'watchlist' || tab === 'favorites') ? {filter} : {})}), [tab, sort, direction, filter]);
  const personalRoute = useMemo<SavedRoute | null>(() => (tab === 'history' ? {view: 'history', ...(period ? {period} : {})} : tab === 'collections' ? {view: 'collections'} : tab === 'views' ? {view: 'views'} : null), [tab, period]);
  const [creating, setCreating] = useState(false);
  const [creatingPlaylist, setCreatingPlaylist] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [clearBusy, setClearBusy] = useState(false);
  const [clearError, setClearError] = useState('');
  const saved = useSaved(savedRoute);
  const personal = usePersonal(personalRoute);
  const setSearch = (patch: Partial<typeof search>) => void navigate({to: '/saved', search: {...search, ...patch}, replace: true});
  const projection = saved.snapshot.projection;
  const sorts = savedRoute && tab !== 'playlists' ? projection?.sorts ?? [] : [];
  const activeSort = sort ?? projection?.query.sort ?? sorts[0]?.id;
  const activeDirection = direction ?? (projection?.query.direction as 'asc' | 'desc' | undefined);
  const refresh = personalRoute ? personal.refresh : saved.refresh;
  useSelectionRefresh(refresh);
  usePersonalChanges(refresh);
  const listFilter = projection?.filters?.find(f => f.id === 'filter');
  const countFor = (f: SavedListFilter) => listFilter?.options.find(o => o.id === f)?.count;
  const canCreatePlaylist = tab === 'playlists' && !!projection?.actions?.includes('create_playlist');
  const clearHistory = async () => {
    setClearBusy(true); setClearError('');
    try { if (!await personal.service.mutate({action: 'clear-history'})) throw personal.service.getSnapshot().mutationError ?? new Error('not cleared'); setClearing(false); personal.refresh(); } catch (e) { setClearError(errorText(e, 'saved', 'save')); } finally { setClearBusy(false); }
  };
  return (
    <Page>
      <PageHeader title={t('web.saved.title')} actions={<Button variant="ghost" icon="refresh" aria-label={t('action.refresh')} onClick={refresh} />} />
      <Tabs items={tabs()} value={tab} onChange={id => setSearch({view: id, sort: undefined, direction: undefined, filter: undefined, period: undefined})} size="large" label={t('web.saved.sections')} />
      {tab === 'watchlist' || tab === 'favorites' ? (
        <Inset><Row><Text variant="label" tone="tertiary">{t('web.notifications.show')}</Text><Chips>{savedListFilters.map(f => <Chip key={f} label={filterLabel(f)} count={countFor(f)} pressed={(filter ?? 'all') === f} onClick={() => setSearch({filter: f === 'all' ? undefined : f})} />)}</Chips></Row></Inset>
      ) : null}
      {tab === 'history' ? (
        <Inset><Row style={{justifyContent: 'space-between', flexWrap: 'wrap'}}><Row><Text variant="label" tone="tertiary">{t('web.saved.period')}</Text><Chips>{historyPeriods.map(x => <Chip key={x} label={periodLabel(x)} pressed={(period ?? 'all') === x} onClick={() => setSearch({period: x === 'all' ? undefined : x})} />)}</Chips></Row>{personal.snapshot.entries.length ? <Button variant="ghost" size="sm" icon="trash" label={t('web.saved.clearHistory')} onClick={() => { setClearError(''); setClearing(true); }} /> : null}</Row></Inset>
      ) : null}
      {tab === 'collections' ? <Inset><Row><Button variant="secondary" size="sm" icon="plus" label={t('web.saved.newCollection')} onClick={() => setCreating(true)} /></Row></Inset> : null}
      {canCreatePlaylist ? <Inset><Row><Button variant="secondary" size="sm" icon="plus" label={t('web.saved.newPlaylist')} onClick={() => setCreatingPlaylist(true)} /></Row></Inset> : null}
      <NameDialog open={creatingPlaylist} title={t('web.saved.newPlaylist')} action={t('web.saved.create')} context="playlist" onClose={() => setCreatingPlaylist(false)} onSave={async (name, summary) => { await saved.service.mutate({action: 'create', name, summary}); saved.refresh(); }} />
      <ConfirmDialog open={clearing} onOpenChange={o => !o && setClearing(false)} title={t('web.saved.clearHistoryTitle')} body={t('web.saved.clearHistoryBody')} confirmLabel={t('web.saved.clearHistoryAction')} destructive busy={clearBusy} error={clearError || undefined} onConfirm={() => void clearHistory()} />
      <NewCollectionDialog open={creating} onClose={() => setCreating(false)} onCreate={async (name, summary, visibility) => { if (!await personal.service.mutate({action: 'create', kind: 'collection', name, summary, visibility})) throw personal.service.getSnapshot().mutationError ?? new Error('not created'); }} />
      {sorts.length ? (
        <Inset><Row><Text variant="label" tone="tertiary">{t('sort.title')}</Text><Chips>
          <Menu label={t('sort.title')} trigger={<Chip label={sortLabel(sorts.find(x => x.id === activeSort)?.labelKey ?? '')} icon="chevronDown" />} items={sorts.map(x => ({id: x.id, label: sortLabel(x.labelKey), selected: x.id === activeSort}))} onSelect={id => setSearch({sort: id, direction: activeDirection})} />
          <Chip label={activeDirection === 'desc' ? t('sort.descending') : t('sort.ascending')} icon={activeDirection === 'desc' ? 'chevronDown' : 'chevronUp'} onClick={() => setSearch({sort: activeSort, direction: activeDirection === 'desc' ? 'asc' : 'desc'})} />
        </Chips></Row></Inset>
      ) : null}
      {personalRoute ? <PersonalView tab={tab} snapshot={personal.snapshot} onRefresh={personal.refresh} onNext={personal.next} onPrevious={personal.previous} /> : <SavedView tab={tab as Exclude<SavedTab, 'history' | 'collections'>} filter={filter} onShowAll={() => setSearch({filter: undefined})} snapshot={saved.snapshot} onRetry={saved.retry} onRefresh={saved.refresh} onNext={saved.next} onPrevious={saved.previous} />}
    </Page>
  );
}

function SavedView({tab, filter, onShowAll, snapshot, onRetry, onRefresh, onNext, onPrevious}: {tab: Exclude<SavedTab, 'history' | 'collections'>; filter?: string; onShowAll: () => void; snapshot: SavedSnapshot; onRetry: () => void; onRefresh: () => void; onNext: (id: string) => void; onPrevious: () => void}) {
  const navigate = useNavigate();
  const sections = snapshot.projection?.sections ?? [];
  const entries = useMemo(() => sections.flatMap(x => x.entries), [sections]);
  const pageable = sections.find(x => x.nextCursor);
  const copy = emptyCopy(tab);
  // WEB-SAVED-05: a filter that matches nothing says so and offers the way back.
  const filtered = !!filter && filter !== 'all' && (tab === 'watchlist' || tab === 'favorites');
  const list = tab === 'favorites' ? t('saved.tab.favorites') : t('saved.tab.watchlist');
  return (
    <>
      {snapshot.phase === 'error' && !sections.length ? <ErrorState error={snapshot.error} context="saved" retry={onRetry} refresh={onRefresh} /> : snapshot.phase === 'error' ? <Inset><ErrorNotice error={snapshot.error} context="saved" retry={onRetry} refresh={onRefresh} /></Inset> : snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="saved" refresh={onRefresh} /></Inset> : null}
      {snapshot.phase === 'loading' && !sections.length ? <LoadingGrid /> : null}
      {snapshot.phase === 'ready' && !entries.length ? (filtered ? <StateView icon="filter" title={filter === 'unwatched' ? t('web.saved.noneUnwatched', {list}) : t('web.saved.noneInProgress', {list})} action={{label: t('web.saved.showAll'), onClick: onShowAll}} /> : <StateView icon={copy.icon} title={copy.title} body={copy.body} />) : null}
      {tab === 'playlists' ? (
        entries.filter(isPlaylistCard).length ? (
          <Inset><Surface padless>{entries.filter(isPlaylistCard).map(card => <ListRow key={card.id} icon="queue" title={card.title} subtitle={card.subtitle} meta={card.count != null ? t('web.saved.items', {count: card.count}) : undefined} trailingIcon="forward" onClick={() => void navigate({to: '/saved/$kind/$resourceId', params: {kind: 'playlist', resourceId: card.navigation.entityId}, search: {title: card.title}})} />)}</Surface></Inset>
        ) : null
      ) : <MediaGrid entries={entries.filter(isMedia)} />}
      <Pager canPrevious={snapshot.pagination.canPrevious} canNext={!!pageable} busy={snapshot.phase === 'loading'} onPrevious={onPrevious} onNext={() => pageable && onNext(pageable.id)} />
    </>
  );
}

function PersonalView({tab, snapshot, onRefresh, onNext, onPrevious}: {tab: SavedTab; snapshot: PersonalSavedSnapshot; onRefresh: () => void; onNext: () => void; onPrevious: () => void}) {
  const navigate = useNavigate();
  const open = useOpenEntry();
  const loadingFresh = snapshot.loading && !snapshot.entries.length && !snapshot.resources.length;
  const copy = emptyCopy(tab);
  const collections = tab === 'collections' || tab === 'views';
  const empty = !snapshot.loading && !snapshot.error && (collections ? !snapshot.resources.length : !snapshot.entries.length) && snapshot.route !== null;
  return (
    <>
      {snapshot.error && !snapshot.entries.length ? <ErrorState error={snapshot.error} context="saved" retry={onRefresh} refresh={onRefresh} /> : snapshot.error ? <Inset><ErrorNotice error={snapshot.error.conflict ? changedError : snapshot.error} context="saved" retry={onRefresh} refresh={onRefresh} /></Inset> : null}
      {loadingFresh ? <LoadingGrid /> : null}
      {empty ? <StateView icon={copy.icon} title={copy.title} body={copy.body} /> : null}
      {collections ? (
        snapshot.resources.length ? <Inset><Surface padless>{snapshot.resources.map(r => <ListRow key={r.id} icon={r.kind === 'view' ? 'filter' : 'collection'} title={r.name} subtitle={r.summary} meta={t('web.saved.items', {count: r.entryCount})} trailingIcon="forward" onClick={() => void navigate({to: '/saved/$kind/$resourceId', params: {kind: r.kind, resourceId: r.id}, search: {title: r.name}})} />)}</Surface></Inset> : null
      ) : (
        <Grid density="poster">
          {snapshot.entries.filter(e => e.media).map(e => {
            const m = e.media!;
            const when = e.updatedAt ? defaultI18n.relativeTime(Date.parse(e.updatedAt)) : undefined;
            return <Card key={e.id} title={m.title} caption={[e.completed ? t('web.saved.finished') : cardCaptionOf(m), when].filter(Boolean).join(' · ')} path={m.posterUrl ?? m.backdropUrl} shape="poster" fit={shapeFor(m.kind) === 'square' ? 'contain' : undefined} icon={iconFor(m.kind)} progress={e.completed ? 0 : e.positionSeconds && m.duration ? Math.min(1, e.positionSeconds / m.duration) : progressFor(m)} watched={e.completed} onOpen={() => open(m)} onPlay={m.playback ? () => open(m, 'play') : undefined} onMore={anchor => open(m, 'more', undefined, anchor)} />;
          })}
        </Grid>
      )}
      <Pager canPrevious={snapshot.history.length > 0} canNext={!!snapshot.nextCursor} busy={snapshot.loading} onPrevious={onPrevious} onNext={onNext} />
    </>
  );
}

function MediaGrid({entries}: {entries: readonly ContentEntry[]}) {
  const open = useOpenEntry();
  if (!entries.length) return null;
  const shapes = new Set(entries.map(e => shapeFor(e.kind)));
  const density = shapes.size === 1 && shapeFor(entries[0]!.kind) === 'square' ? 'square' : 'poster';
  return <Grid density={density}>{entries.map(e => <Card key={e.id} title={e.title} caption={cardCaptionOf(e)} path={e.posterUrl ?? e.backdropUrl} shape={shapeFor(e.kind)} icon={iconFor(e.kind)} progress={progressFor(e)} watched={e.watched} onOpen={() => open(e)} onPlay={e.playback ? () => open(e, 'play') : undefined} onMore={anchor => open(e, 'more', undefined, anchor)} />)}</Grid>;
}
function Pager({canPrevious, canNext, busy, onPrevious, onNext}: {canPrevious: boolean; canNext: boolean; busy: boolean; onPrevious: () => void; onNext: () => void}) {
  if (!canPrevious && !canNext) return null;
  return <Inset><Row style={{justifyContent: 'center'}}><Button variant="secondary" icon="back" label={t('web.live.previous')} disabled={!canPrevious || busy} onClick={onPrevious} /><Button variant="secondary" iconAfter="forward" label={t('web.search.next')} disabled={!canNext || busy} loading={busy && canNext} onClick={onNext} /></Row></Inset>;
}

/** Collections are private unless the owner publishes them to every member of the server. */
function NewCollectionDialog({open, onClose, onCreate}: {open: boolean; onClose: () => void; onCreate: (name: string, summary: string, visibility: 'private' | 'server') => Promise<void>}) {
  const [name, setName] = useState('');
  const [summary, setSummary] = useState('');
  const [shared, setShared] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const create = async () => {
    // Enter reaches this without the button's loading lock, so the guard is here.
    if (busy || !name.trim()) return;
    setBusy(true); setError(undefined);
    try { await onCreate(name.trim(), summary.trim(), shared ? 'server' : 'private'); setName(''); setSummary(''); setShared(false); onClose(); }
    catch (e) { setError(errorText(e, 'collection', 'save')); }
    finally { setBusy(false); }
  };
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={t('web.saved.newCollection')} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} disabled={busy} /><Button variant="primary" label={t('web.saved.create')} loading={busy} disabled={!name.trim()} onClick={() => void create()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={t('profile.name')} value={name} onChange={e => setName(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && name.trim()) { e.preventDefault(); void create(); } }} autoFocus />
        <TextArea label={t('web.saved.summary')} optional rows={3} value={summary} onChange={e => setSummary(e.target.value)} />
        <Switch label={t('web.saved.visibleToAll')} checked={shared} onCheckedChange={setShared} />
        <Text variant="caption" tone="tertiary">{shared ? t('web.saved.sharedHelp') : t('web.saved.privateHelp')}</Text>
      </div>
    </Dialog>
  );
}

/** A name and optional description, for a new playlist or renaming one. */
export function NameDialog({open, title, action, context, initialName = '', initialSummary = '', onClose, onSave}: {open: boolean; title: string; action: string; context: 'playlist' | 'collection'; initialName?: string; initialSummary?: string; onClose: () => void; onSave: (name: string, summary: string) => Promise<void>}) {
  const [name, setName] = useState(initialName);
  const [summary, setSummary] = useState(initialSummary);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { if (open) { setName(initialName); setSummary(initialSummary); setError(''); } }, [open, initialName, initialSummary]);
  const save = async () => {
    if (busy || !name.trim()) return;
    setBusy(true); setError('');
    try { await onSave(name.trim(), summary.trim()); onClose(); } catch (e) { setError(errorText(e, context, 'save')); } finally { setBusy(false); }
  };
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={title} width={440} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={onClose} disabled={busy} /><Button variant="primary" label={action} loading={busy} disabled={!name.trim()} onClick={() => void save()} /></>}>
      <form onSubmit={e => { e.preventDefault(); void save(); }} style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        <Input label={t('profile.name')} value={name} maxLength={200} autoFocus onChange={e => setName(e.target.value)} />
        <TextArea label={t('web.saved.summary')} optional rows={3} value={summary} onChange={e => setSummary(e.target.value)} />
      </form>
    </Dialog>
  );
}
