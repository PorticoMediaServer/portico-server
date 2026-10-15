import {cardCaptionOf} from '../../app/content';
import {useBack} from '../../app/back';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import React from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import {SavedService, type SavedRoute, type SavedSnapshot, type PlaylistOccurrence} from '@core/saved.ts';
import {PersonalSavedService, type PersonalSavedSnapshot} from '@core/personal-saved.ts';
import {useService} from '../../app/content';
import {captionFor, formatDuration, iconFor, progressFor, shapeFor} from '@core/presentation/index.ts';
import {mosaicPaths} from '../../app/title-layout';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {sequenceContainer} from '../../app/container-play';
import {usePlayerActions} from '../../player/PlayerContext';
import {useSession} from '../../app/session';
import {AnchoredMenu, Artwork, Button, Card, ConfirmDialog, Grid, Inset, ListRow, Menu, MenuRow, Notice, Page, PageHeader, Skeleton, StateView, Surface, Switch, WindowedGrid, anchorOf, createStaticCollection, type MenuAnchor, type MenuItem} from '../../ui';
import {LoadingGrid, SECTION_WINDOW_AT, useAppendedPages} from '../shared/Sections';
import {ErrorNotice, errorText} from '../../app/errors';
import {NameDialog} from './Saved';
import {useViewerScope} from '../../app/viewer-scope';
import {useFreshDocument} from '../../app/freshness';
import {titlesWithPersonalState} from '@core/freshness-hub.ts';

import {useI18n} from '../../app/i18n';
import type {BrowseNode} from '@core/browse.ts';
import {itemPivot, spanningGroup, useGroupBrowse, useGroupLibraries, type GroupChoice, type GroupSpec} from '../../app/group-browse';
import {useLibrariesContext} from '../../app/libraries';
import {GroupBar, GroupGrid, GroupLibraryChips, GroupRows} from '../shared/GroupBrowse';

const requestId = () => Promise.resolve(crypto.randomUUID());
type Visible = Extract<PlaylistOccurrence, {hidden: false}>;

/** One playlist or personal collection: identity, Play, then its ordered entries, and (WEB-SAVED-02)
 * the management the server offers for it: rename, reorder, remove and delete. */
const resourceDependencies = Object.freeze([...titlesWithPersonalState()]);

export function SavedResourceScreen() {
  const {t} = useI18n();
  const {kind, resourceId} = useParams({from: '/app/saved/$kind/$resourceId'});
  const address = useSearch({from: '/app/saved/$kind/$resourceId'});
  const {title} = address;
  const navigate = useNavigate();
  const {api} = useSession();
  const scope = useViewerScope();
  const player = usePlayerActions();
  const open = useOpenEntry();
  // Cards are links, so they open in a new tab and their address can be copied.
  const hrefOf = useEntryHref();
  const playlist = kind === 'playlist';
  const route = useMemo<SavedRoute>(() => (playlist ? {view: 'playlist', playlistId: resourceId} : {view: 'resource', resourceId}), [playlist, resourceId]);
  const savedBinding = useService<SavedService, SavedSnapshot>(() => new SavedService({api, scope, requestId}), [api, scope]);
  const personalBinding = useService<PersonalSavedService, PersonalSavedSnapshot>(() => new PersonalSavedService(api, scope, requestId), [api, scope]);
  // A playlist, collection or saved view open on screen follows changes made elsewhere, in place.
  useFreshDocument(resourceDependencies, context => savedBinding.service.revalidate(context), () => savedBinding.service.getSnapshot().phase === 'ready');
  useFreshDocument(resourceDependencies, context => personalBinding.service.revalidate(context), () => !!personalBinding.service.getSnapshot().viewerFence);
  const [renaming, setRenaming] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState('');
  const [entryMenu, setEntryMenu] = useState<{id: string; anchor: MenuAnchor} | null>(null);
  const last = useRef('');
  useEffect(() => {
    const key = JSON.stringify(route);
    if (last.current === key) return;
    last.current = key;
    try {
      if (playlist) void savedBinding.service.select(route).catch(() => {});
      else void personalBinding.service.select(route).catch(() => {});
    } catch {}
  }, [route, playlist, savedBinding.service, personalBinding.service]);
  const directory = () => void navigate({to: '/saved', search: {view: playlist ? 'playlists' : 'collections'}});
  const back = useBack(directory);
  const context = playlist ? 'playlist' : 'collection';
  /** Every change goes through the service's intents; a failure is presented, never thrown at the page. */
  const change = async (work: () => Promise<unknown>, after?: () => void) => {
    setBusy(true); setProblem('');
    try { await work(); after?.(); } catch (e) { setProblem(errorText(e, context, 'save')); } finally { setBusy(false); }
  };
  // PERF-S08: pages accumulate as the viewer scrolls (the service holds one
  // page; this keeps every page in order), so entry 1,000 is reachable.
  const savedPage = (savedBinding.snapshot.projection?.sections ?? []).flatMap(x => x.entries);
  const savedAll = useAppendedPages(`playlist:${resourceId}`, savedBinding.snapshot.pagination.cursor, savedPage, e => e.id);
  const personalPage = personalBinding.snapshot.entries;
  const personalAll = useAppendedPages(`resource:${resourceId}`, personalBinding.snapshot.cursor, personalPage, e => e.id);

  // The library's sort and filters on what is listed here. A playlist spans libraries, so with
  // entries from several the viewer picks one; a saved view is one library's already, and its own
  // conditions stay in force under whatever is added. A personal collection has no such list on
  // the server yet and keeps its own order.
  const libraries = useLibrariesContext().items;
  const listBase = useMemo<readonly BrowseNode[]>(() => [{field: 'list', operator: 'equals', value: resourceId}], [resourceId]);
  const held = useGroupLibraries(playlist ? listBase : undefined, itemPivot);
  const listScope = useMemo(() => spanningGroup(held.libraries, address.library, listBase, 'playlist'), [held.libraries, address.library, listBase]);
  const definition = !playlist && personalBinding.snapshot.resource?.kind === 'view' ? personalBinding.snapshot.resource.definition : undefined;
  const viewSpec = useMemo<GroupSpec | undefined>(() => {
    if (!definition) return undefined;
    const library = libraries.find(l => l.id === definition.libraryId);
    return library ? {libraryId: library.id, libraryKind: library.kind, pivot: definition.pivot, base: definition.query ? [definition.query as BrowseNode] : [], opening: definition.sort[0]} : undefined;
  }, [definition, libraries]);
  const choose = useCallback((next: {library?: string} & GroupChoice) => void navigate({to: '/saved/$kind/$resourceId', params: {kind, resourceId}, search: {...(title ? {title} : {}), ...next}, replace: true, resetScroll: false}), [navigate, kind, resourceId, title]);
  const group = useGroupBrowse(playlist ? listScope.spec : viewSpec, {sort: address.sort, direction: address.direction, filters: address.filters}, useCallback((next: GroupChoice) => choose({library: playlist ? listScope.chosen : undefined, ...next}), [choose, playlist, listScope.chosen]));

  if (playlist) {
    const snap = savedBinding.snapshot;
    const service = savedBinding.service;
    const p = snap.playlist;
    const can = (action: string) => !!p?.actions.includes(action);
    const sections = snap.projection?.sections ?? [];
    const entries = savedAll;
    const media = entries.filter((e): e is Visible => e.kind === 'playlist_entry' && !e.hidden);
    const first = media.find(e => e.media.playback);
    // Windowed reads: the next page loads on scroll, and moves send only the
    // moved occurrences with an anchor — never the whole order.
    const pageable = sections.find(s => s.nextCursor);
    const loadingMore = snap.phase === 'loading' && media.length > 0;
    const order = snap.order;
    const orderReady = !!p && !!order && order.revision === p.revision && order.viewerFence === p.viewerFence;
    const ids = entries.map(e => e.id);
    // Ensure the order window covers the occurrences a move needs, then move
    // one occurrence after its anchor ("" prepends to the front).
    const move = (index: number, by: -1 | 1) => {
      const mover = ids[index];
      if (!mover || !p || !can('reorder')) return;
      const anchor = by === -1 ? (index <= 1 ? '' : ids[index - 2]!) : ids[index + 1]!;
      const run = () => service.mutate({action: 'reorder', entryIds: [mover], ...(anchor === undefined ? {} : {afterEntryId: anchor})});
      if (anchor === undefined) return;
      if (orderReady && order.entryIds.includes(mover) && (anchor === '' || order.entryIds.includes(anchor))) { void change(run); return; }
      void change(() => service.loadOrder().then(run));
    };
    const playlistRow = (e: Visible, i: number) => {
      const index = ids.indexOf(e.id);
      const items: MenuItem[] = [{id: 'open', label: t('web.saved.goToItem'), icon: 'info'}];
      if (can('reorder') && index > 0) items.push({id: 'up', label: t('web.profile.moveUp'), icon: 'chevronUp'});
      if (can('reorder') && index < ids.length - 1) items.push({id: 'down', label: t('web.profile.moveDown'), icon: 'chevronDown'});
      if (can('remove')) items.push({id: 'remove', label: t('web.saved.removeFromPlaylist'), icon: 'minus', destructive: true, separatorBefore: true});
      // PERF-S06: a row plays the playlist from here — one v1 container request anchored at the
      // pressed entry, never the loaded page.
      const playFromHere = () => player.playSequence(media.map(x => x.media), media.indexOf(e), e.media.playback?.startSeconds ?? 0, sequenceContainer('playlist', resourceId));
      return <ListRow key={e.id} index={i + 1} title={e.media.title} subtitle={cardCaptionOf(e.media)} meta={formatDuration(e.media.duration)} art={{path: e.media.posterUrl, icon: iconFor(e.media.kind), progress: progressFor(e.media)}} artShape={shapeFor(e.media.kind)} onClick={() => (e.media.playback ? playFromHere() : open(e.media))} actions={<Menu label={t('web.dvr.actionsFor', {title: e.media.title})} trigger={<Button variant="ghost" icon="more" size="sm" aria-label={t('web.dvr.actionsFor', {title: e.media.title})} disabled={busy} />} items={items} onSelect={id => {
        if (id === 'open') open(e.media);
        else if (id === 'up') move(index, -1);
        else if (id === 'down') move(index, 1);
        else if (id === 'remove') void change(() => service.mutate({action: 'remove', entryId: e.id}));
      }} />} />;
    };
    const pageMenu: MenuItem[] = [];
    if (can('rename')) pageMenu.push({id: 'rename', label: t('web.saved.rename'), icon: 'edit'});
    if (can('delete')) pageMenu.push({id: 'delete', label: t('web.saved.deletePlaylist'), icon: 'trash', destructive: true, separatorBefore: pageMenu.length > 0});
    const name = p?.name ?? title ?? t('web.saved.playlist');
    // M26 item 5: playlists have no custom art — the title shows a 2×2 mosaic
    // of the first 4 item posters or covers (nothing when no item has art).
    const mosaic = mosaicPaths({}, media.map(e => e.media));
    return (
      <Page>
        <PageHeader onBack={back} backLabel={t('web.saved.title')} eyebrow={t('web.saved.playlist')} pageKey={p?.id} title={name} titleAdornment={<HeroMosaic paths={mosaic ?? []} />} subtitle={p ? [p.entryCount > 0 ? t('web.saved.items', {count: p.entryCount}) : undefined, p.summary].filter(Boolean).join(' · ') || undefined : undefined} actions={<>
          {/* PERF-S06: Play and Shuffle send one v1 playlist-container request, however large. */}
          {first ? <Button variant="primary" icon="play" label={t('action.play')} onClick={() => player.playSequence(media.map(e => e.media), 0, 0, sequenceContainer('playlist', resourceId))} /> : null}
          {media.filter(e => e.media.playback).length > 1 ? <Button variant="secondary" icon="shuffle" label={t('title.shuffle')} onClick={() => player.playSequence(media.map(e => e.media), 0, 0, sequenceContainer('playlist', resourceId, {shuffle: true}))} /> : null}
          {pageMenu.length ? <Menu label={t('web.saved.playlistOptions')} trigger={<Button variant="ghost" icon="more" aria-label={t('web.saved.playlistOptions')} />} items={pageMenu} onSelect={id => (id === 'rename' ? setRenaming(true) : setDeleting(true))} /> : null}
        </>} />
        {snap.phase === 'error' ? <Inset><ErrorNotice error={snap.error} context="playlist" retry={() => void service.retry().catch(() => {})} /></Inset> : null}
        {problem && !deleting ? <Inset><Notice tone="error" action={{label: t('action.dismiss'), onClick: () => setProblem('')}}>{problem}</Notice></Inset> : null}
        {snap.phase === 'loading' && !entries.length ? <LoadingGrid /> : null}
        {snap.phase === 'ready' && !media.length ? <StateView icon="queue" title={t('web.saved.playlistEmpty')} body={t('web.saved.playlistEmptyBody')} /> : null}
        {media.length && listScope.choose ? <Inset><GroupLibraryChips libraries={held.libraries} value={listScope.chosen} onChange={library => choose(library ? {library} : {})} /></Inset> : null}
        {media.length && listScope.spec ? <Inset><GroupBar group={group} total={p?.entryCount} /></Inset> : null}
        {group.pending ? <LoadingGrid /> : group.active ? (
          // Sorted or filtered, the playlist is a list of titles: its own order, and moving entries within it, belong to Playlist order.
          <GroupRows group={group} context="playlist" label={name} estimateRowHeight={72}
            // The same framed rows as Playlist order; the frame supplies the page's gutter, so the list inside it has none of its own.
            wrap={list => <Inset><Surface padless><div style={{'--page-gutter': '0px'} as React.CSSProperties}>{list}</div></Surface></Inset>}
            renderItem={entry => <ListRow title={entry.title} subtitle={cardCaptionOf(entry)} meta={formatDuration(entry.duration)} art={{path: entry.posterUrl, icon: iconFor(entry.kind), progress: progressFor(entry)}} artShape={shapeFor(entry.kind)} onClick={() => open(entry, entry.playback ? 'play' : 'open')} actions={<Button variant="ghost" icon="more" size="sm" aria-label={t('card.more', {title: entry.title})} onClick={(e: React.MouseEvent<HTMLElement>) => open(entry, 'more', undefined, anchorOf(e.currentTarget))} />} />}
            renderPlaceholder={() => <div style={{height: 72, padding: '8px 16px'}}><Skeleton height={56} /></div>} />
        ) : media.length > SECTION_WINDOW_AT ? (
          <WindowedRows items={media} estimateRowHeight={72} label={name} renderItem={(e, i) => playlistRow(e, i)} />
        ) : media.length ? (
          <Inset><Surface padless>{media.map((e, i) => playlistRow(e, i))}</Surface></Inset>
        ) : null}
        {loadingMore && !group.active ? <Inset><Skeleton height={48} /></Inset> : null}
        {(pageable || loadingMore) && !group.active && !group.pending ? <ScrollSentinel cursor={pageable?.nextCursor ?? ''} disabled={snap.phase !== 'ready'} onMore={() => { const at = savedBinding.snapshot.projection?.sections.find(s => s.nextCursor); if (at) void service.next(at.id).catch(() => {}); }} /> : null}
        <NameDialog open={renaming} title={t('web.saved.renamePlaylist')} action={t('action.save')} context="playlist" initialName={p?.name} initialSummary={p?.summary} onClose={() => setRenaming(false)} onSave={(n, summary) => service.mutate({action: 'rename', name: n, ...(can('summary') ? {summary} : {})})} />
        <ConfirmDialog open={deleting} onOpenChange={o => !o && setDeleting(false)} title={t('web.saved.deleteTitle', {name})} body={t('web.saved.deletePlaylistBody')} confirmLabel={t('web.saved.deletePlaylistAction')} destructive busy={busy} error={problem || undefined} onConfirm={() => void change(() => service.mutate({action: 'delete'}), () => { setDeleting(false); directory(); })} />
      </Page>
    );
  }

  const snap = personalBinding.snapshot;
  const service = personalBinding.service;
  /** Personal changes resolve false (with the reason in the snapshot) instead of throwing. */
  const done = async (work: Promise<boolean>) => { if (!await work) throw service.getSnapshot().mutationError ?? new Error('not saved'); };
  const r = snap.resource;
  const can = (action: string) => !!r?.actions.includes(action);
  const items = personalAll.filter(e => e.media);
  const loadingMore = snap.loading && items.length > 0;
  const name = r?.name ?? title ?? t('web.saved.collection');
  const view = r?.kind === 'view';
  // M26 item 5: collections have no custom art — the title shows a 2×2 mosaic
  // of the first 4 item posters or covers (nothing when no item has art).
  const collectionMosaic = mosaicPaths({}, items.flatMap(e => (e.media ? [e.media] : [])));
  const pageMenu: MenuItem[] = [];
  if (can('update')) pageMenu.push({id: 'rename', label: t('web.saved.rename'), icon: 'edit'});
  if (can('delete')) pageMenu.push({id: 'delete', label: view ? t('web.saved.deleteView') : t('web.saved.deleteCollection'), icon: 'trash', destructive: true, separatorBefore: pageMenu.length > 0});
  return (
    <Page>
      <PageHeader onBack={back} backLabel={t('web.saved.title')} eyebrow={view ? t('web.saved.savedView') : r?.visibility === 'server' ? t('web.saved.sharedCollection') : t('web.saved.collection')} pageKey={r?.id} title={name} titleAdornment={<HeroMosaic paths={collectionMosaic ?? []} />} subtitle={r?.summary} actions={<>
        {r?.kind === 'collection' && r.role === 'owner' && can('update') ? <Switch label={t('web.saved.visibleToAll')} checked={r.visibility === 'server'} disabled={snap.pending} onCheckedChange={v => void service.mutate({action: 'update', visibility: v ? 'server' : 'private'}).catch(() => {})} /> : null}
        {pageMenu.length ? <Menu label={t('web.saved.collectionOptions')} trigger={<Button variant="ghost" icon="more" aria-label={t('web.saved.collectionOptions')} />} items={pageMenu} onSelect={id => (id === 'rename' ? setRenaming(true) : setDeleting(true))} /> : null}
      </>} />
      {snap.error ? <Inset><ErrorNotice error={snap.error} context="collection" retry={() => void service.refresh().catch(() => {})} /></Inset> : null}
      {snap.mutationError && !problem && !renaming && !deleting ? <Inset><ErrorNotice error={snap.mutationError} context="collection" operation="save" /></Inset> : null}
      {problem && !deleting ? <Inset><Notice tone="error" action={{label: t('action.dismiss'), onClick: () => setProblem('')}}>{problem}</Notice></Inset> : null}
      {snap.loading && !items.length ? <LoadingGrid /> : null}
      {!snap.loading && !items.length && !snap.error ? <StateView icon="collection" title={view ? t('web.saved.viewEmpty') : t('web.saved.collectionEmpty')} /> : null}
      {view && viewSpec ? <Inset><GroupBar group={group} total={r?.entryCount} /></Inset> : null}
      {group.active ? <GroupGrid group={group} square={viewSpec?.libraryKind === 'music' || viewSpec?.libraryKind === 'audiobook'} /> : items.length > SECTION_WINDOW_AT ? (
        <WindowedRows items={items} estimateRowHeight={300} columnMin="clamp(120px, 11vw, 168px)" label={name} renderItem={e => <Card key={e.id} title={e.media!.title} caption={cardCaptionOf(e.media!)} path={e.media!.posterUrl} shape={shapeFor(e.media!.kind)} icon={iconFor(e.media!.kind)} progress={progressFor(e.media!)} href={hrefOf(e.media!)} onOpen={() => open(e.media!)} onPlay={e.media!.playback ? () => open(e.media!, 'play') : undefined} onMore={anchor => (can('entries') && !view ? setEntryMenu({id: e.id, anchor}) : open(e.media!, 'more', undefined, anchor))} />} />
      ) : items.length ? <Grid density="poster">{items.map(e => <Card key={e.id} title={e.media!.title} caption={cardCaptionOf(e.media!)} path={e.media!.posterUrl} shape={shapeFor(e.media!.kind)} icon={iconFor(e.media!.kind)} progress={progressFor(e.media!)} href={hrefOf(e.media!)} onOpen={() => open(e.media!)} onPlay={e.media!.playback ? () => open(e.media!, 'play') : undefined} onMore={anchor => (can('entries') && !view ? setEntryMenu({id: e.id, anchor}) : open(e.media!, 'more', undefined, anchor))} />)}</Grid> : null}
      {loadingMore && !group.active ? <Inset><Skeleton height={48} /></Inset> : null}
      {(snap.nextCursor || loadingMore) && !group.active ? <ScrollSentinel cursor={snap.nextCursor} disabled={snap.loading} onMore={() => void service.next().catch(() => {})} /> : null}
      <AnchoredMenu anchor={entryMenu?.anchor} open={!!entryMenu} onOpenChange={o => !o && setEntryMenu(null)} label={t('web.saved.collectionOptions')}>
        <MenuRow icon="info" label={t('web.saved.goToItem')} onSelect={() => { const m = items.find(x => x.id === entryMenu?.id)?.media; setEntryMenu(null); if (m) open(m); }} />
        <MenuRow icon="minus" destructive label={t('web.saved.removeFromCollection')} onSelect={() => { const id = entryMenu!.id; setEntryMenu(null); void change(() => done(service.mutate({action: 'entries', removeEntryIds: [id]}))); }} />
      </AnchoredMenu>
      <NameDialog open={renaming} title={view ? t('web.saved.renameView') : t('web.saved.renameCollection')} action={t('action.save')} context="collection" initialName={r?.name} initialSummary={r?.summary} onClose={() => setRenaming(false)} onSave={(n, summary) => done(service.mutate({action: 'update', name: n, summary}))} />
      <ConfirmDialog open={deleting} onOpenChange={o => !o && setDeleting(false)} title={t('web.saved.deleteTitle', {name})} body={view ? t('web.saved.deleteViewBody') : t('web.saved.deleteCollectionBody')} confirmLabel={view ? t('web.saved.deleteViewAction') : t('web.saved.deleteCollectionAction')} destructive busy={busy} error={problem || undefined} onConfirm={() => void change(() => done(service.mutate({action: 'delete'})), () => { setDeleting(false); directory(); })} />
    </Page>
  );
}

/**
 * M26 item 5: a 2×2 mosaic of the first 4 item posters or covers beside the
 * title (playlists and collections have no custom art). Renders nothing when
 * no item has art, so an artless collection shows no empty tiles.
 */
function HeroMosaic({paths}: {paths: readonly string[]}) {
  if (!paths.length) return null;
  const tiles = [paths[0], paths[1], paths[2], paths[3]];
  return (
    <div aria-hidden style={{display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2, width: 64, flexShrink: 0}}>
      {tiles.map((path, i) => <Artwork key={i} path={path} shape="poster" icon="collection" alt="" />)}
    </div>
  );
}

/**
 * PERF-S08: an already-loaded page list rendered through its visible window
 * only (O(visible), never O(loaded)). Backed by an in-memory static
 * collection; the service's cursor still pages on scroll.
 */
function WindowedRows<T extends {id: string}>({items, estimateRowHeight, columnMin, label, renderItem}: {items: readonly T[]; estimateRowHeight: number; columnMin?: string; label: string; renderItem: (item: T, index: number) => React.ReactNode}) {
  const store = React.useMemo(() => createStaticCollection<T>({keyOf: e => e.id}), []);
  React.useEffect(() => { store.setItems(items); }, [store, items]);
  React.useEffect(() => () => store.collection.dispose(), [store]);
  const placeholder = useCallback(() => <div style={{height: estimateRowHeight, padding: '8px 16px'}}><Skeleton height={estimateRowHeight - 16} /></div>, [estimateRowHeight]);
  return (
    <WindowedGrid
      collection={store.collection}
      estimateRowHeight={estimateRowHeight}
      columnMin={columnMin}
      label={label}
      renderItem={renderItem}
      renderPlaceholder={placeholder}
    />
  );
}

/**
 * PERF-S08: the next cursor page loads as the viewer scrolls — once per page
 * (each cursor fires at most once) and only while idle-ready, so an idle page
 * sends nothing and a scroll step sends one request. Stays mounted across
 * loads; appended content pushes it out of view until the viewer scrolls on.
 */
function ScrollSentinel({cursor, disabled, onMore}: {cursor: string; disabled: boolean; onMore: () => void}) {
  const ref = React.useRef<HTMLDivElement>(null);
  const state = React.useRef({cursor, disabled, onMore});
  state.current = {cursor, disabled, onMore};
  const fired = React.useRef('');
  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver(entries => {
      if (!entries.some(e => e.isIntersecting)) return;
      const s = state.current;
      if (s.disabled || s.cursor === fired.current) return;
      fired.current = s.cursor;
      s.onMore();
    }, {rootMargin: '600px'});
    io.observe(el);
    return () => io.disconnect();
  }, []);
  return <div ref={ref} style={{height: 1}} aria-hidden />;
}
