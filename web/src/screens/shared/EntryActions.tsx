import {cardCaptionOf} from '../../app/content';
import {useNavigate} from '@tanstack/react-router';
import {setPendingTogether, useGroupActions} from '../../app/together';
import {useDownloads} from '../../app/downloads';
import {currentI18n as i18nNow, useI18n} from '../../app/i18n';
import {useDeleteMedia} from '../../app/delete-media';
import React, {useEffect, useMemo, useRef, useState} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {SavedService, type PlaylistCard, type SavedSnapshot} from '@core/saved.ts';
import {PersonalSavedService, type PersonalResource, type PersonalSavedSnapshot} from '@core/personal-saved.ts';
import {queueItemInput} from '@core/queue-controller.ts';
import {useSession} from '../../app/session';
import {repairTargetFor, useMetadataEditor} from '../../app/metadata-editor';
import {useDetail, requestId} from '../../app/detail';
import {useService} from '../../app/content';
import {captionFor, entryActions, type EntryActionId} from '@core/presentation/index.ts';
import {useOpenEntry} from '../../app/open';
import {useMoreHandOff, usePlayer} from '../../player/engine';
import {AnchoredMenu, Button, Input, ListRow, Loading, MenuHeading, MenuRow, MenuSeparator, Notice, Text} from '../../ui';
import {useAppendedPages} from './Sections';
import {addItemsToPlaylist, addItemsToPlaylistViaJob} from './playlist-add';
import {runBulkJobs} from './bulk-job';
import {containerKindFor, useContainerWatched} from './container-watched';
import {errorText} from '../../app/errors';
import {announceRemoval, removeFromContinueWatching, type ContinueRemoval} from '../../app/continue-watching';
import {recommendationViewer, startNotInterested} from '../../app/not-interested';
import type {EntryOrigin} from '../../player/engine';
import {useViewerScope} from '../../app/viewer-scope';

/**
 * The actions menu for one entry, opened from a card's More control, a
 * right-click or a long press. It drops down from the anchor the gesture
 * supplied, like the library picker. Personal actions come from the item's
 * detail projection so the menu never invents a capability the server did
 * not offer; queue actions come from the device queue; playlist membership
 * goes through Saved.
 */
export function EntryActionsSheet() {
  const player = useMoreHandOff();
  const entry = player.moreEntry;
  // The sheet is anchored, so an entry handed over without a rectangle can never
  // open — and because it never opens, `onOpenChange` never fires to clear it.
  // Releasing it here keeps a failed hand-off from wedging the next one.
  useEffect(() => {
    if (entry && !player.moreAnchor) player.clearMore();
  }, [entry, player]);
  return (
    <AnchoredMenu open={!!entry} anchor={player.moreAnchor} onOpenChange={o => !o && player.clearMore()} label={entry ? `Actions for ${entry.title}` : undefined}>
      {entry ? <Body key={entry.id} entry={entry} origin={player.moreOrigin} onDone={player.clearMore} /> : null}
    </AnchoredMenu>
  );
}

function Body({entry, origin, onDone}: {entry: ContentEntry; origin?: EntryOrigin; onDone: () => void}) {
  const {api, session, owner} = useSession();
  const viewer = recommendationViewer(useViewerScope());
  const i18n = useI18n();
  const editor = useMetadataEditor();
  const deleter = useDeleteMedia();
  const downloads = useDownloads();
  const player = usePlayer();
  const open = useOpenEntry();
  const together = useGroupActions();
  const navigate = useNavigate();
  const target = entry.libraryId && entry.id ? {libraryId: entry.libraryId, itemId: entry.id} : null;
  const {snapshot, mutate, refresh: refreshDetail} = useDetail(target);
  const data = snapshot.data;
  const personal = data?.personal;
  const has = (id: string) => data?.actions.some(a => a.id === id && a.enabled) ?? false;
  const loadingDetail = !data && snapshot.phase !== 'error';
  const [mode, setMode] = useState<'actions' | 'playlist' | 'collection'>('actions');
  const [notice, setNotice] = useState<string>();
  // WEB-DL-01: Download… only when the server offers at least one way to download this item.
  const [downloadable, setDownloadable] = useState<boolean>();
  useEffect(() => {
    const itemId = entry.playback?.itemId;
    if (!itemId || !downloads.service) return;
    if (downloads.state.unavailable) { setDownloadable(false); return; }
    const controller = new AbortController();
    downloads.service.options(itemId, controller.signal).then(view => setDownloadable(view.options.some(o => o.available)), () => setDownloadable(false));
    return () => controller.abort();
  }, [entry.playback?.itemId, downloads.service, downloads.state.unavailable]);
  const queue = player.queue;
  const enqueue = (next: boolean) => {
    if (!entry.playback || !queue) return;
    queue.enqueue(queueItemInput(entry.playback.itemId), next).then(() => setNotice(i18n.t(next ? 'entry.playingNext' : 'entry.addedToQueue'))).catch(e => setNotice(errorText(e, 'generic')));
  };
  const refreshMetadata = () => {
    void api.request('/v1/items/' + encodeURIComponent(entry.id) + '/metadata/refresh', 'POST', {}).then(() => setNotice(i18n.t('entry.refreshQueued'))).catch(() => setNotice(i18n.t('entry.refreshFailed')));
  };
  const local = owner && session?.viewer.authority === 'local';
  const repair = repairTargetFor(entry);
  const containerKind = containerKindFor(entry.kind);
  const containerId = containerKind && entry.navigation?.entityId ? entry.navigation.entityId : undefined;
  // A show, season, album, book or artist keeps its own personal state (the watched default,
  // Watchlist, Favorite) on the container route; item kinds keep the detail intent.
  const containerWatched = useContainerWatched(api, containerId ? containerKind : undefined, containerId);
  const container = containerId;
  const containerDownloadKind = containerKind === 'show' || containerKind === 'season' || containerKind === 'album' || containerKind === 'book' ? containerKind : undefined;
  // CON-24: one shared model decides the menu's order, groups, labels and glyphs.
  const groups = useMemo(() => entryActions({
    kind: entry.kind,
    playable: !!entry.playback,
    progressSeconds: entry.progressSeconds,
    queue: !!queue,
    group: together.inRoom && together.canQueue,
    // WEB-MENU-01: until the detail read answers, personal rows stand in (disabled) so the
    // menu doesn't grow under the pointer; a server that doesn't offer them removes them after.
    // On the title's own page these three are the action row (origin 'page'); the menu holds the rest.
    ...(origin === 'page' ? {} : {
    watchlisted: container ? (containerWatched.carries('watchlisted') ? !!containerWatched.state?.watchlisted : undefined) : loadingDetail || has('watchlist') ? !!personal?.watchlisted : undefined,
    favorite: container ? (containerWatched.carries('favorite') ? !!containerWatched.state?.favorite : undefined) : loadingDetail || has('favorite') ? !!personal?.favorite : undefined,
    watched: container ? (containerWatched.carries('watched') ? containerWatched.state?.watched : undefined) : loadingDetail || has('watched') ? (personal ? !!personal.watched : entry.watched === true) : undefined,
    }),
    // X-12: a started, unfinished film or episode can leave Continue Watching (its resume point stays).
    continueWatching: has('watched') && !!personal && !personal.watched && personal.progressSeconds > 0 && !!entry.playback && (entry.kind === 'movie' || entry.kind === 'episode'),
    // P5: a recommendation card can be turned down; it needs nothing from the detail read.
    notInterested: origin === 'recommendation',
    addToPlaylist: has('add_to_playlist') || !!entry.playback,
    addToCollection: loadingDetail ? !!entry.playback : has('add_to_collection'),
    download: downloads.state.unavailable ? undefined : entry.playback && downloadable ? 'media' : container && containerDownloadKind ? 'container' : undefined,
    // No Details for the page already open (an album's own More menu).
    details: !!entry.navigation && !(entry.navigation.entityId && location.pathname.endsWith(`/${entry.navigation.view}/${entry.navigation.entityId}`)),
    editMetadata: !!(local && repair),
    refreshMetadata: !!local,
    delete: !!(owner && entry.playback),
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [entry, origin, queue, together.inRoom, together.canQueue, data, downloadable, downloads.state.unavailable, local, repair, owner, loadingDetail, containerWatched.state, containerWatched.carries]);
  const run = (id: EntryActionId) => {
    switch (id) {
      case 'play': case 'resume': open(entry, 'play'); onDone(); return;
      case 'playFromStart': player.play(entry.playback!.itemId, 0, entry); onDone(); return;
      case 'playNext': enqueue(true); return;
      case 'addToQueue': enqueue(false); return;
      case 'watchWithGroup': void together.service?.watch(entry.playback!.itemId).then(ok => { if (ok) void navigate({to: '/together'}); }); onDone(); return;
      case 'addToGroupQueue': void together.service?.queueAdd([entry.playback!.itemId]).then(ok => setNotice(i18n.t(ok ? 'entry.addedToGroupQueue' : 'entry.groupQueueFailed'))); return;
      case 'watchlist': if (container) void containerWatched.setFlag('watchlisted', !containerWatched.state?.watchlisted); else mutate({action: 'watchlist', value: !personal?.watchlisted}); return;
      case 'favorite': if (container) void containerWatched.setFlag('favorite', !containerWatched.state?.favorite); else mutate({action: 'favorite', value: !personal?.favorite}); return;
      case 'watched': if (container && containerKind) { void containerWatched.setWatched(!(containerWatched.state?.watched ?? false)); } else mutate({action: 'watched', value: !personal?.watched}); return;
      case 'removeFromContinueWatching': {
        if (!personal) return;
        const removal: ContinueRemoval = {itemId: entry.id, title: entry.title, phase: 'pending', undo: async () => {}};
        announceRemoval(removal);
        onDone();
        removeFromContinueWatching(api, entry.id, personal.revision).then(done => { announceRemoval({...removal, phase: 'done', undo: done.undo}); void refreshDetail(); }, () => announceRemoval({...removal, phase: 'failed'}));
        return;
      }
      case 'notInterested': {
        startNotInterested(api, viewer, session?.viewer.serverId ?? '', entry);
        onDone();
        return;
      }
      case 'addToPlaylist': setMode('playlist'); return;
      case 'addToCollection': setMode('collection'); return;
      case 'download':
        if (entry.playback && downloadable) downloads.ask({target: {mediaId: entry.playback.itemId}, title: entry.title});
        else if (container && containerDownloadKind) downloads.ask({target: {containerId: container, containerKind: containerDownloadKind}, title: entry.title});
        onDone(); return;
      case 'details': open(entry, 'open'); onDone(); return;
      case 'editMetadata': if (repair) editor?.open({targets: [repair], titles: [entry.title]}); onDone(); return;
      case 'refreshMetadata': refreshMetadata(); return;
      case 'delete': deleter?.open({itemIds: [entry.playback!.itemId], titles: [entry.title]}); onDone(); return;
    }
  };
  if (mode === 'collection') return <CollectionPicker itemId={entry.id} onBack={() => setMode('actions')} onDone={message => { setNotice(message); setMode('actions'); }} />;
  if (mode === 'playlist') return <PlaylistPicker itemId={entry.playback?.itemId ?? entry.id} itemTitles={[entry.title]} onBack={() => setMode('actions')} onDone={message => { setNotice(message); setMode('actions'); }} />;
  // WEB-TOGETHER-01: outside a group, "Watch Together" starts one with this title queued.
  const startGroup = entry.playback && !together.inRoom && together.service;
  return (
    <>
      <MenuHeading title={entry.title} caption={cardCaptionOf(entry)} />
      {notice ? <div style={{padding: '0 4px 8px'}}><Notice tone="info">{notice}</Notice></div> : null}
      {groups.map((group, g) => (
        <React.Fragment key={group.id}>
          {g > 0 ? <MenuSeparator /> : null}
          {group.actions.map(action => <MenuRow key={action.id} disabled={loadingDetail && ((group.id === 'personal' && action.id !== 'notInterested') || action.id === 'addToCollection')} icon={action.icon} label={i18n.t(action.label)} destructive={action.destructive} trailingIcon={action.id === 'addToPlaylist' || action.id === 'addToCollection' ? 'forward' : undefined} onSelect={() => run(action.id)} />)}
          {group.id === 'play' && startGroup ? <MenuRow icon="people" label={i18n.t('entry.group.together')} onSelect={() => { setPendingTogether({itemId: entry.playback!.itemId, title: entry.title}); void navigate({to: '/together'}); onDone(); }} /> : null}
        </React.Fragment>
      ))}
    </>
  );
}

/** Pick an existing playlist or create one, then add the item. */
export function PlaylistPicker({itemId, itemIds, itemTitles, bulk, onBack, onDone}: {itemId?: string; itemIds?: readonly string[]; /** Titles alongside `itemIds`, so a partial failure can name what missed. */ itemTitles?: readonly string[]; /** Bulk picks add through one playlist-add job per 200 items, not one request per item. */ bulk?: boolean; /** Only when opened from the actions menu; otherwise the picker names itself (WEB-DETAIL-07). */ onBack?: () => void; onDone: (message: string) => void}) {
  const ids = itemIds ?? (itemId ? [itemId] : []);
  const titleOf = (id: string) => {
    const at = ids.indexOf(id);
    return at >= 0 && itemTitles?.[at] ? itemTitles[at]! : id;
  };
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<SavedService, SavedSnapshot>(() => new SavedService({api, scope, requestId}), [api, scope]);
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [failedNames, setFailedNames] = useState<string>();
  const [failedIds, setFailedIds] = useState<readonly string[]>([]);
  const [succeeded, setSucceeded] = useState(0);
  const [target, setTarget] = useState<PlaylistCard>();
  useEffect(() => {
    void service.select({view: 'playlists'}).catch(() => {});
  }, [service]);
  const page = (snapshot.projection?.sections.flatMap(s => s.entries) ?? []).filter((e): e is PlaylistCard => e.kind === 'playlist');
  // PERF-S08: the picker pages through every playlist, not just the first page.
  const playlists = useAppendedPages('playlists', snapshot.pagination.cursor, page, p => p.id);
  const morePlaylists = snapshot.pagination.next[0];
  // Bulk picks go through playlist-add jobs (one per 200 items,
  // appended at the end); single picks keep the per-item mutate. A partial
  // failure stays open naming what missed, with a retry for exactly those items.
  const add = async (playlist: PlaylistCard, only?: readonly string[]) => {
    const batch = only ?? ids;
    setBusy(true);
    setError(undefined);
    setFailedNames(undefined);
    setTarget(playlist);
    try {
      await service.select({view: 'playlist', playlistId: playlist.id});
      const revision = service.getSnapshot().playlist?.revision;
      const useJob = bulk && batch.length > 1;
      const result = useJob && revision
        ? await addItemsToPlaylistViaJob(api, {playlistId: playlist.id, expectedRevision: revision, placement: 'end', ids: batch, operationIds: Array.from({length: Math.max(1, Math.ceil(batch.length / 200))}, () => crypto.randomUUID())})
        : await addItemsToPlaylist(service, playlist.id, batch);
      const total = succeeded + result.ok;
      if (!result.failed.length) {
        onDone(ids.length === 1 ? i18nNow().t('entry.addedTo', {name: playlist.title}) : i18nNow().t('entry.addedCountTo', {count: ids.length, name: playlist.title}));
      } else {
        setSucceeded(total);
        setFailedIds(result.failed);
        setError(i18nNow().t('entry.addedPartial', {ok: total, count: ids.length, failed: result.failed.length}));
        setFailedNames(i18nNow().t('entry.addFailedNames', {names: result.failed.slice(0, 3).map(titleOf).join(', ')}));
      }
    } catch (e) {
      setError(errorText(e, 'playlist', 'save'));
    } finally {
      setBusy(false);
    }
  };
  const create = async () => {
    // Enter reaches this without the button's loading lock, so the guard is here.
    if (busy || !name.trim()) return;
    setBusy(true);
    setError(undefined);
    try {
      await service.select({view: 'playlists'});
      // Synchronous initial itemIds are capped at 200; larger bulk additions
      // create with the first page, then finish through playlist-add jobs.
      const head = bulk ? ids.slice(0, 200) : ids;
      await service.mutate({action: 'create', name: name.trim(), itemIds: head});
      const createdId = service.getSnapshot().lastReceipt?.playlistId;
      const rest = bulk ? ids.slice(200) : [];
      if (rest.length && createdId) {
        await service.select({view: 'playlist', playlistId: createdId});
        const revision = service.getSnapshot().playlist?.revision;
        if (!revision) throw new Error('The playlist changed. Reopen it to add the remaining items.');
        const result = await addItemsToPlaylistViaJob(api, {playlistId: createdId, expectedRevision: revision, placement: 'end', ids: rest, operationIds: Array.from({length: Math.max(1, Math.ceil(rest.length / 200))}, () => crypto.randomUUID())});
        if (result.failed.length) throw new Error(i18nNow().t('entry.addedPartial', {ok: head.length + result.ok, count: ids.length, failed: result.failed.length}));
      }
      onDone(i18nNow().t('entry.created', {name: name.trim()}));
    } catch (e) {
      setError(errorText(e, 'playlist', 'save'));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      {onBack ? <div><Button variant="link" size="sm" icon="back" label={i18nNow().t('action.back')} onClick={onBack} /></div> : <MenuHeading title={i18nNow().t('entry.addToPlaylist').replace(/…$/, '')} />}
      {error ? <Notice tone="error" action={failedIds.length && target && !busy ? {label: i18nNow().t('action.tryAgain'), onClick: () => void add(target, failedIds)} : undefined}>{[error, failedNames].filter(Boolean).join(' ')}</Notice> : null}
      {snapshot.phase === 'loading' && !snapshot.projection ? <Loading label={i18nNow().t('entry.loadingPlaylists')} /> : null}
      <div>
        {playlists.map(p => <ListRow key={p.id} icon="queue" title={p.title} subtitle={p.subtitle} meta={p.count !== undefined ? `${p.count}` : undefined} onClick={() => void add(p)} />)}
        {morePlaylists && snapshot.phase === 'ready' ? <div style={{display: 'flex', justifyContent: 'center', padding: '4px 0'}}><Button variant="ghost" size="sm" label={i18nNow().t('action.showMore')} disabled={busy} onClick={() => void service.next(morePlaylists.sectionId).catch(() => {})} /></div> : null}
        {snapshot.phase === 'ready' && !playlists.length ? <Text as="p" variant="caption" tone="tertiary" style={{padding: '8px 12px'}}>{i18nNow().t('entry.noPlaylists')}</Text> : null}
      </div>
      <div style={{display: 'flex', gap: 8, alignItems: 'flex-end'}}>
        <Input label={i18nNow().t('web.saved.newPlaylist')} placeholder={i18nNow().t('profile.name')} value={name} onChange={e => setName(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); void create(); } }} />
        <Button variant="primary" label={i18nNow().t('web.saved.create')} loading={busy} disabled={!name.trim()} onClick={() => void create()} />
      </div>
    </div>
  );
}

/** Collections use the published edit capability and the service's revision/
 * operation receipts. Failed mutations retain the same operation for retry.
 * In bulk mode, picking a collection submits one collection-add job for every
 * item instead of one request per item. */
export function CollectionPicker({itemId, itemIds, bulk, onBack, onDone}: {itemId?: string; itemIds?: readonly string[]; bulk?: boolean; onBack?: () => void; onDone: (message: string) => void}) {
  const singleId = itemId ?? itemIds?.[0];
  const ids = itemIds ?? (itemId ? [itemId] : []);
  const {api} = useSession();
  const scope = useViewerScope();
  const {service, snapshot} = useService<PersonalSavedService, PersonalSavedSnapshot>(() => new PersonalSavedService(api, scope, requestId), [api, scope]);
  const active = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [selected, setSelected] = useState<PersonalResource>();
  useEffect(() => { void service.select({view: 'collections'}); }, [service]);
  const run = async (work: () => Promise<boolean>, name: string) => {
    if (active.current) return;
    active.current = true; setBusy(true); setError(undefined);
    try {
      if (await work()) {
        const result = service.getSnapshot().result;
        const outcomes = result?.entries;
        if (!result?.deleted && (outcomes?.added.includes(singleId!) || outcomes?.unchanged.includes(singleId!))) onDone(`Added to ${name}.`);
        else setError(outcomes?.failed.some(item => item.itemId === singleId) ? 'This item could not be added to the collection. Check its availability and your access.' : 'The server did not confirm that this item was added. Reopen the collection to check.');
      }
      else setError(errorText(service.getSnapshot().mutationError ?? service.getSnapshot().error ?? {code: 'unexpected'}, 'collection', 'save'));
    } catch (e) { setError(errorText(e, 'collection', 'save')); }
    finally { active.current = false; setBusy(false); }
  };
  /** Bulk mode: one collection-add job for every item, fenced on the picked collection's revision. */
  const addBulk = async (resource: PersonalResource) => {
    if (active.current) return;
    active.current = true; setBusy(true); setError(undefined);
    setSelected(resource);
    try {
      await service.select({view: 'resource', resourceId: resource.id});
      const revision = service.getSnapshot().resource?.revision;
      if (!revision) throw new Error('The collection changed. Reopen it to continue.');
      const outcome = await runBulkJobs(api, 'collection-add', {collectionId: resource.id, expectedRevision: revision}, ids, Array.from({length: Math.max(1, Math.ceil(ids.length / 200))}, () => crypto.randomUUID()));
      if (outcome.failed.length) setError(i18nNow().t('entry.addedPartial', {ok: outcome.ok, count: ids.length, failed: outcome.failed.length}));
      else onDone(ids.length === 1 ? i18nNow().t('entry.addedTo', {name: resource.name}) : i18nNow().t('entry.addedCountTo', {count: ids.length, name: resource.name}));
    } catch (e) { setError(errorText(e, 'collection', 'save')); }
    finally { active.current = false; setBusy(false); }
  };
  const add = (resource: PersonalResource) => (bulk && ids.length > 1 ? addBulk(resource) : run(async () => {
    setSelected(resource);
    await service.select({view: 'resource', resourceId: resource.id});
    return service.mutate({action: 'entries', addItemIds: [singleId!]});
  }, resource.name));
  const available = snapshot.resources.filter(resource => resource.kind === 'collection' && resource.actions.includes('entries'));
  const failure = error ?? (snapshot.error ? errorText(snapshot.error, 'collection', 'load') : snapshot.mutationError ? errorText(snapshot.mutationError, 'collection', 'save') : undefined);
  return <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
    {onBack ? <div><Button variant="link" size="sm" icon="back" label={i18nNow().t('action.back')} disabled={busy} onClick={onBack} /></div> : <MenuHeading title={i18nNow().t('entry.addToCollection').replace(/…$/, '')} />}
    {failure ? <Notice tone="error" action={{label: i18nNow().t('action.tryAgain'), onClick: () => {
      if (busy) return;
      if (snapshot.retryPending && selected) void run(() => service.retry(), selected.name);
      else if (selected) void add(selected);
      else void service.refresh();
    }}}>{failure}</Notice> : null}
    {snapshot.loading || busy ? <Loading label={busy ? i18nNow().t('entry.addingToCollection') : i18nNow().t('entry.loadingCollections')} /> : null}
    {available.map(resource => <Button key={resource.id} variant="ghost" icon="collection" label={resource.name} disabled={busy || snapshot.loading} onClick={() => void add(resource)} />)}
    {!snapshot.loading && !busy && !failure && !available.length ? <Text as="p" variant="caption" tone="tertiary">{i18nNow().t('entry.noCollections')}</Text> : null}
    <div style={{display: 'flex', gap: 8}}>
      {snapshot.history.length ? <Button variant="ghost" label={i18nNow().t('player.previous')} disabled={busy || snapshot.loading} onClick={() => void service.previous()} /> : null}
      {snapshot.nextCursor ? <Button variant="ghost" label={i18nNow().t('player.next')} disabled={busy || snapshot.loading} onClick={() => void service.next()} /> : null}
    </div>
  </div>;
}
