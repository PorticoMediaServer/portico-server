import {CatalogueSelectionEntry} from '../browse/document-selection';
import {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, Fragment, type ReactNode, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import type {PersonalSavedIntent, PersonalResourceEntry} from '@core/personal-saved.ts';
import type {HistoryPeriod, SavedIntent} from '@core/saved.ts';
import {historyPeriods} from '@core/saved.ts';
import {episodeLabel} from '@portico/client-core/kit-model';
import {shapeFor, shapeAspect} from '@core/presentation/card-shape.ts';
import {AmbientArt, Artwork, CoverMosaic, type ArtworkSource} from '../../kit/artwork';
import {Button, Chip, Select, Switch} from '../../kit/controls';
import {ViewToggle} from '../../kit/browse';
import {Dialog, Menu} from '../../kit/overlays';
import {Skeleton, StateView} from '../../kit/states';
import {CircleCard, PosterCard, SquareCard} from '../../kit/cards';
import {WindowedGrid} from '../../kit/layout';
import {useLayoutPresence} from '../../kit/layout/presence';
import {applyRemovalFocus} from '../../kit/layout/focus-context';
import {useReducedMotion} from '../../kit/overlays/shared';
import {HistoryPaint} from './HistoryPaint';
import {homeCardModel, homeCardSubject} from '../home/model';
import {BrowseEntryCard} from '../browse';
import type {SavedViewBindings} from './Saved';
import {SavedSection} from './Saved';
import {sameSavedRoute} from './route.ts';
import {playlistRemovalUndo, type PlaylistRemovalUndo} from './removal.ts';
import {RemovalUndo} from './RemovalUndo';
import {ResourceFields} from './ResourceFields';
import {SavedDialog} from './SavedDialog';
import {personalEntriesWindow} from './window.ts';
import styles from './saved.module.css';

const t = defaultI18n.t;
export function SavedResourceScreen({bindings}: {bindings: SavedViewBindings}) {
  const {runtime, route, effects} = bindings;
  const playlist = useSyncExternalStore(runtime.saved.subscribe, runtime.saved.getSnapshot, runtime.saved.getSnapshot), personal = useSyncExternalStore(runtime.personal.subscribe, runtime.personal.getSnapshot, runtime.personal.getSnapshot);
  const [menu, setMenu] = useState(false), [dialog, setDialog] = useState<'rename' | 'delete'>(), [name, setName] = useState(''), [summary, setSummary] = useState(''), [busy, setBusy] = useState(false);
  const [rowMenu, setRowMenu] = useState<Readonly<{entryId: string; media: ContentEntry}>>(), [undo, setUndo] = useState<PlaylistRemovalUndo>();
  const rowAnchor = useRef<HTMLElement>(null), pendingAction = useRef(false);
  const clearUndo = useCallback(() => setUndo(undefined), []);
  const cancel = useRef<HTMLButtonElement>(null), nameInput = useRef<HTMLInputElement>(null);
  const isPlaylist = route.view === 'playlist', resource = isPlaylist ? playlist.playlist : personal.resource;
  const routeMatches = sameSavedRoute(isPlaylist ? playlist.route : personal.route, route);
  const nameShown = isPlaylist ? playlist.playlist?.name : personal.resource?.name;
  const kind = isPlaylist ? 'playlist' : personal.resource?.kind === 'view' ? 'view' : 'collection';
  const actions = resource?.actions ?? [], pinned = resource?.pinned ?? false;
  const selectedPlaylistEntry = rowMenu && playlist.projection?.sections.flatMap(section => section.entries).find(entry => entry.id === rowMenu.entryId);
  const selectedPersonalEntry = rowMenu && personal.entries.find(entry => entry.id === rowMenu.entryId);
  const menuMedia = isPlaylist ? selectedPlaylistEntry?.kind === 'playlist_entry' && !selectedPlaylistEntry.hidden ? selectedPlaylistEntry.media : undefined
    : selectedPersonalEntry && !selectedPersonalEntry.hidden ? selectedPersonalEntry.media : undefined;
  const perform = (playlistIntent: SavedIntent, personalIntent: PersonalSavedIntent, done?: () => void) => {
    if (pendingAction.current || !bindings.operations.current) return;
    pendingAction.current = true; setBusy(true);
    void bindings.operations.run(async () => {
      if (isPlaylist) await runtime.saved.mutate(playlistIntent);
      else if (!await runtime.personal.mutate(personalIntent)) throw runtime.personal.getSnapshot().mutationError;
    }, () => { pendingAction.current = false; setBusy(false); done?.(); }).catch(error => { pendingAction.current = false; setBusy(false); effects.reportError(error); });
  };
  const failure = isPlaylist ? playlist.error : personal.error;
  if (!routeMatches) return null;
  if (!resource) return failure ? <StateView title={t('saved.listLoadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void runtime.select(route); }}} /> : null;
  const paths = isPlaylist ? playlist.projection?.sections.flatMap(section => section.entries).flatMap(entry => entry.kind === 'playlist_entry' && !entry.hidden ? entry.media.posterUrl ? [entry.media.posterUrl] : [] : []).slice(0, 4) ?? [] : personal.resource?.artworkPaths ?? [];
  const covers = paths.map(path => bindings.artwork.foreground(path, 120, 1)).filter((source): source is ArtworkSource => !!source);
  const renameTitle = t(kind === 'playlist' ? 'web.saved.renamePlaylist' : kind === 'view' ? 'web.saved.renameView' : 'web.saved.renameCollection');
  return <main className={styles.page}>
    <div className={styles.resourceBack}><Button variant="text" icon="i-left" size="small" onClick={() => effects.selectRoute({view: kind === 'playlist' ? 'playlists' : kind === 'view' ? 'views' : 'collections'})}>{t('saved.title')}</Button></div>
    <header className={styles.resourceHeader}><div className={styles.resourceCover}><CoverMosaic covers={covers} shape="square" alt="" /></div><div>
      <span className={styles.resourceKind}>{t(kind === 'playlist' ? 'web.saved.playlist' : kind === 'view' ? 'web.saved.savedView' : resource.role === 'owner' ? 'web.saved.collection' : 'web.saved.sharedCollection')}</span>
      <h1>{nameShown}</h1><p>{kind === 'playlist' ? t('web.saved.items', {count: resource.entryCount}) : t('lib.count', {count: resource.entryCount, pivot: 'other'})}</p>{resource.summary && <p>{resource.summary}</p>}
      {!isPlaylist && personal.resource?.kind === 'collection' && personal.resource.actions.includes('update') && <label className={styles.resourceVisibility}><Switch checked={personal.resource.visibility === 'server'} disabled={busy} onCheckedChange={checked => perform({action: 'pin', pinned}, {action: 'update', visibility: checked ? 'server' : 'private'})} /><span><b>{t('web.saved.visibleToAll')}</b><small>{t('web.saved.sharedHelp')}</small></span></label>}
      <div className={styles.resourceActions}>
        {effects.playResource && <><Button variant="primary" icon="i-play" size="compact" onClick={() => { void bindings.operations.run(signal => effects.playResource!({kind, id: resource.id, shuffle: false}, signal)).catch(effects.reportError); }}>{t('action.play')}</Button>
          <Button icon="i-shuffle" size="compact" onClick={() => { void bindings.operations.run(signal => effects.playResource!({kind, id: resource.id, shuffle: true}, signal)).catch(effects.reportError); }}>{t('title.shuffle')}</Button></>}
        {actions.includes('pin') && <Button icon="i-bookmark" size="compact" disabled={busy} onClick={() => perform({action: 'pin', pinned: !pinned}, {action: 'pin', pinned: !pinned})}>{t(pinned ? 'pins.unpin' : 'pins.pin')}</Button>}
        <Menu open={menu} onOpenChange={setMenu} title={t('player.more')} width={260} trigger={<Button variant="icon" icon="i-more" iconOnly aria-label={t('player.more')} />}
          items={[
            ...(actions.includes('pin') ? [{id: 'pin', label: t(pinned ? 'pins.unpin' : 'pins.pin'), icon: 'i-bookmark' as const, disabled: busy, onSelect: () => perform({action: 'pin', pinned: !pinned}, {action: 'pin', pinned: !pinned})}] : []),
            ...(actions.includes('rename') || actions.includes('update') ? [{id: 'rename', label: t('web.saved.rename'), icon: 'i-edit' as const, disabled: busy, onSelect: () => { setName(nameShown ?? ''); setSummary(resource.summary); setDialog('rename'); }}] : []),
            ...(actions.includes('delete') ? [{id: 'delete-separator', separator: true} as const, {id: 'delete', label: t(kind === 'playlist' ? 'web.saved.deletePlaylist' : kind === 'view' ? 'web.saved.deleteView' : 'web.saved.deleteCollection'), icon: 'i-trash' as const, destructive: true, disabled: busy, onSelect: () => setDialog('delete')}] : []),
          ]} />
      </div>
    </div></header>
    <div className={styles.toolbar}>{bindings.catalogueSelection?.toolbar ?? bindings.selection?.toolbar}<div className={styles.toolbarEnd}><span>{kind === 'playlist' ? t('web.saved.items', {count: resource.entryCount}) : t('lib.count', {count: resource.entryCount, pivot: 'other'})}</span><ViewToggle value={bindings.presentation.view} onValueChange={bindings.presentation.setView} /></div></div>
    {isPlaylist ? playlist.projection?.sections.map(section => <SavedSection key={section.id} {...bindings} service={runtime.saved} section={section} onMore={(entry, anchor) => {
      if (entry.kind !== 'playlist_entry' || entry.hidden || !bindings.operations.current) return;
      rowAnchor.current = anchor; setRowMenu({entryId: entry.id, media: entry.media});
    }} />)
      : <PersonalRows bindings={bindings} entries={personal.entries} onMore={(entry, anchor) => {
        if (!entry.media || entry.hidden || !bindings.operations.current) return;
        rowAnchor.current = anchor; setRowMenu({entryId: entry.id, media: entry.media});
      }} />}
    <Menu open={!!rowMenu && !!menuMedia} onOpenChange={open => { if (!open) setRowMenu(undefined); }} title={menuMedia?.displayTitle ?? menuMedia?.title ?? t('player.more')} anchorRef={rowAnchor} width={290}
      items={rowMenu && menuMedia ? [{id: 'go', label: t('web.saved.goToItem'), icon: 'i-info', onSelect: () => { if (bindings.operations.current) effects.openEntry(menuMedia); }},
        ...((isPlaylist ? actions.includes('remove') : kind === 'collection' && actions.includes('entries')) ? [
          {id: 'remove-separator', separator: true} as const,
          {id: 'remove', label: <span className={styles.removeMenuLabel}><svg width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" d="M6 12h12" /></svg>{t(isPlaylist ? 'web.saved.removeFromPlaylist' : 'web.saved.removeFromCollection')}</span>, destructive: true, disabled: busy, onSelect: () => {
            const removed = {...rowMenu, media: menuMedia};
            perform({action: 'remove', entryId: removed.entryId}, {action: 'entries', removeEntryIds: [removed.entryId]}, () => {
              if (isPlaylist) setUndo(playlistRemovalUndo(runtime.saved.getSnapshot().lastReceipt, {serverId: runtime.scope.serverId, viewerFence: resource.viewerFence, playlistId: resource.id, entryId: removed.entryId}, removed.media.displayTitle ?? removed.media.title));
            });
          }},
        ] : [])] : []} />
    <RemovalUndo value={undo} clear={clearUndo} undo={value => {
      if (Date.now() >= value.until) return;
      perform({action: 'restore', undoToken: value.token}, {action: 'entries', undoToken: value.token}, clearUndo);
    }} />
    <SavedDialog busy={busy} open={!!dialog} onOpenChange={open => { if (!open && !busy) setDialog(undefined); }} title={dialog === 'rename' ? renameTitle : t('web.saved.deleteTitle', {name: nameShown ?? ''})} initialFocusRef={dialog === 'rename' ? nameInput : cancel}
      description={dialog === 'delete' ? t(kind === 'playlist' ? 'web.saved.deletePlaylistBody' : kind === 'view' ? 'web.saved.deleteViewBody' : 'web.saved.deleteCollectionBody') : undefined}
      footer={<><Button ref={cancel} size="small" disabled={busy} onClick={() => setDialog(undefined)}>{t('action.cancel')}</Button><Button variant="primary" className={dialog === 'delete' ? styles.destructive : undefined} size="small" loading={busy} disabled={dialog === 'rename' && !name.trim()} onClick={() => {
        if (dialog === 'rename') perform({action: 'rename', name: name.trim(), ...(actions.includes('summary') ? {summary} : {})}, {action: 'update', name: name.trim(), summary}, () => setDialog(undefined));
        else perform({action: 'delete'}, {action: 'delete'}, () => effects.selectRoute({view: kind === 'playlist' ? 'playlists' : kind === 'view' ? 'views' : 'collections'}));
      }}>{t(dialog === 'rename' ? 'action.save' : kind === 'playlist' ? 'web.saved.deletePlaylistAction' : kind === 'view' ? 'web.saved.deleteViewAction' : 'web.saved.deleteCollectionAction')}</Button></>}>
      {dialog === 'rename' && <ResourceFields nameRef={nameInput} name={name} summary={summary} onNameChange={setName} onSummaryChange={setSummary} nameMaxLength={isPlaylist ? 200 : 400} summaryMaxLength={isPlaylist ? 4000 : 8000} disabled={busy} summaryDisabled={isPlaylist && !actions.includes('summary')} />}
    </SavedDialog>
  </main>;
}

function PersonalRows({bindings, entries, onMore}: {bindings: SavedViewBindings; entries: readonly PersonalResourceEntry[]; onMore: (entry: PersonalResourceEntry, anchor: HTMLButtonElement) => void}) {
  const collection = useMemo(() => personalEntriesWindow(bindings.runtime.personal), [bindings.runtime.personal]);
  const root = useRef<HTMLDivElement>(null), [width, setWidth] = useState(0);
  useEffect(() => {
    const element = root.current; if (!element) return;
    const read = () => setWidth(element.clientWidth);
    const observer = new ResizeObserver(read); observer.observe(element); read();
    return () => observer.disconnect();
  }, []);
  const grid = bindings.presentation.view === 'grid', cardWidth = bindings.presentation.cardWidth, rowHeight = grid ? cardWidth * 1.5 + 48 : 90;
  const columns = grid ? Math.max(1, Math.floor((width + 16) / (cardWidth + 16))) : 1;
  const height = Math.max(rowHeight, Math.min(window.innerHeight, Math.ceil(entries.length / columns) * (rowHeight + (grid ? 30 : 0))));
  return <div className={styles.personalRows}><div ref={root}><WindowedGrid collection={collection} label={bindings.runtime.personal.getSnapshot().resource?.name ?? t('saved.title')}
    height={height} cardWidth={grid ? cardWidth : Math.max(1, width)} rowHeight={rowHeight} gap={16} rowGap={grid ? 30 : 0} reducedMotion={bindings.presentation.reducedMotion}
    renderPlaceholder={() => <Skeleton width={grid ? cardWidth : width} height={grid ? cardWidth * 1.5 : 72} />} renderItem={(entry, context) => {
    const media = entry.media;
    if (!media || entry.hidden) return <div className={styles.hiddenRow}>{t('card.unavailable')}</div>;
    const model = homeCardModel(media, false, {t, locale: defaultI18n.region.formatLocale}), shape = shapeFor(media.kind, grid ? 'grid' : 'list');
    const artwork = bindings.artwork.foreground(media.posterUrl, (grid ? cardWidth : 64) * devicePixelRatio, shapeAspect(shape));
    const selectable = bindings.catalogueSelection?.member(media);
    return <CatalogueSelectionEntry selection={selectable ? bindings.catalogueSelection : undefined} entry={selectable ?? media} position={selectable ? bindings.catalogueSelection!.position(selectable) : context.index}>{documentSelection => {
    const common = {model, context: grid ? 'grid' as const : 'detail' as const, selection: documentSelection ?? bindings.selection?.personalCard(bindings.runtime.personal, entry), watched: media.watched,
      onNavigate: () => bindings.effects.openEntry(media), onMore: (anchor: HTMLButtonElement) => onMore(entry, anchor),
      onPlay: media.playback && media.available !== false && !media.missing ? () => { void bindings.operations.run(signal => bindings.effects.playEntry(media, signal)).catch(bindings.effects.reportError); } : undefined};
    if (grid) return <BrowseEntryCard entry={media} artwork={bindings.artwork} env={{t, locale: defaultI18n.region.formatLocale}} width={cardWidth} mixed visible order={context.index}
      group={`saved:${bindings.runtime.personal.getSnapshot().resource?.id ?? ''}`} backdrops={bindings.presentation.backdrops}
      actions={{onOpen: common.onNavigate, onPlay: common.onPlay, onMore: common.onMore, selection: common.selection}} />;
    return <div className={grid ? undefined : styles.mediaListRow}>{shape === 'circle' ? <CircleCard {...common} covers={artwork ? [artwork] : []} variant="person" />
      : shape === 'square' ? <SquareCard {...common} artwork={artwork} /> : <PosterCard {...common} artwork={artwork} />}</div>;
    }}</CatalogueSelectionEntry>;
  }} /></div>{bindings.runtime.personal.getSnapshot().nextCursor && <Button size="small" className={styles.showMore} loading={bindings.runtime.personal.getSnapshot().loading} onClick={() => { void bindings.runtime.personal.next(); }}>{t('action.showMore')}</Button>}</div>;
}

const periodLabel = (period: HistoryPeriod) => period === '24h' ? t('web.live.today') : period === '7d' ? t('web.saved.thisWeek') : period === '30d' || period === '90d' ? t('web.saved.days', {count: Number.parseInt(period)}) : t('web.saved.allTime');
const weekdayName = new Intl.DateTimeFormat(defaultI18n.region.uiLocale, {weekday: 'long'});
/** The drawn timeline heads each day by its morning: Today, Yesterday, then the day's name. */
function historyDayLabel(date: Date): string {
  const today = new Date(), start = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const day = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const days = Math.round((start.getTime() - day.getTime()) / 86_400_000);
  return days <= 0 ? t('saved.today') : days === 1 ? t('relative.yesterday') : weekdayName.format(day);
}
type HistoryItem = Readonly<{key: string; dayKey: string; day: Date; entry: PersonalResourceEntry; media: ContentEntry; source?: ArtworkSource; bindings: SavedViewBindings}>;
const historyItemKey = (item: HistoryItem) => item.key;
type HistoryRemoval = Readonly<{scope: object; apply(keys: ReadonlySet<string>): void}>;
export function SavedHistory({bindings}: {bindings: SavedViewBindings}) {
  const service = bindings.runtime.personal;
  const scope = useMemo(() => Object.freeze({}), [service, bindings.runtime, bindings.operations, bindings.artwork, bindings.effects]);
  const removal = useRef<HistoryRemoval | undefined>(undefined), clear = useRef<HTMLButtonElement>(null);
  const subscribe = useCallback((listener: () => void) => service.subscribe(() => {
    // The original service has published its accepted rows, but React has not
    // committed inert/removal yet. Focus belongs to the exact old DOM context.
    try {
      const next = service.getSnapshot(), original = removal.current;
      if (original?.scope === scope && bindings.operations.current && sameSavedRoute(next.route, bindings.route))
        original.apply(new Set(next.entries.filter(entry => entry.media && entry.updatedAt && Number.isFinite(Date.parse(entry.updatedAt))).map(entry => entry.id)));
    } catch { /* A focus refusal cannot turn a successful data publication into an error. */ }
    listener();
  }), [service, scope]);
  const snapshot = useSyncExternalStore(subscribe, service.getSnapshot, service.getSnapshot), [confirm, setConfirm] = useState(false), [busy, setBusy] = useState(false), cancel = useRef<HTMLButtonElement>(null);
  const wash = useMemo(() => {
    if (!bindings.presentation.backdrops) return [];
    if (bindings.presentation.backdrop) return [bindings.presentation.backdrop];
    const urls = snapshot.entries.flatMap(entry => { const media = entry.media, url = media && (media.backdropUrl ?? media.posterUrl); return url ? [url] : []; });
    return [...new Set(urls)].slice(0, 3).flatMap(url => { const source = bindings.artwork.ambient(url); return source ? [source] : []; });
  }, [bindings.presentation.backdrops, bindings.presentation.backdrop, bindings.artwork, snapshot.entries]);
  const items = useMemo<readonly HistoryItem[]>(() => snapshot.entries.flatMap(entry => {
    if (!entry.media || !entry.updatedAt) return [];
    const day = new Date(entry.updatedAt);if (!Number.isFinite(day.getTime())) return [];
    const media = entry.media, square = shapeFor(media.kind) === 'square';
    return [{key: entry.id, dayKey: day.toDateString(), day, entry, media, bindings,
      source: bindings.artwork.foreground(square ? media.posterUrl : media.backdropUrl ?? media.posterUrl, 320, square ? 1 : 16 / 9)}];
  }), [snapshot.entries, scope]);
  if (!sameSavedRoute(snapshot.route, bindings.route)) return null;
  const period = bindings.route.view === 'history' ? bindings.route.period ?? '30d' : '30d';
  const empty = snapshot.error && !snapshot.entries.length ? <StateView title={t('saved.historyLoadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void service.retryRead(); }}} />
    : !snapshot.loading && !snapshot.error && !snapshot.entries.length ? <StateView title={t('web.saved.empty.history')} body={t('web.saved.empty.historyBody')} /> : null;
  return <>
    {wash.length ? <div className={styles.wash} aria-hidden="true"><AmbientArt sources={wash} treatment="library" height={640} /></div> : null}
    <div className={styles.toolbar}>
      <div className={styles.periodChips} role="group" aria-label={t('web.saved.period')}>
        {historyPeriods.map(value => <Chip key={value} variant="toolbar" selected={period === value} onClick={() => bindings.effects.selectRoute({view: 'history', period: value})}>{periodLabel(value)}</Chip>)}
      </div>
      <div className={styles.toolbarEnd}><Button ref={clear} variant="text" size="small" onClick={() => setConfirm(true)}>{t('web.saved.clearHistory')}</Button></div>
    </div>
    <HistoryTimeline items={items} scope={scope} reduced={bindings.presentation.reducedMotion} empty={empty} removal={removal} sectionStart={clear} />
    {snapshot.nextCursor && <Button className={styles.showMore} size="small" loading={snapshot.loading} onClick={() => { void service.next(); }}>{t('action.showMore')}</Button>}
    <Dialog open={confirm} onOpenChange={open => { if (!busy) setConfirm(open); }} title={t('web.saved.clearHistoryTitle')} description={t('web.saved.clearHistoryBody')} initialFocusRef={cancel}
      footer={<><Button ref={cancel} size="small" disabled={busy} onClick={() => setConfirm(false)}>{t('action.cancel')}</Button><Button size="small" className={styles.destructive} loading={busy} onClick={() => {
        if (busy || !bindings.operations.current) return;
        setBusy(true); void bindings.operations.run(async () => { if (!await service.mutate({action: 'clear-history'})) throw service.getSnapshot().mutationError; }, () => { setBusy(false); setConfirm(false); }).catch(error => { setBusy(false); bindings.effects.reportError(error); });
      }}>{t('web.saved.clearHistoryAction')}</Button></>}><span /></Dialog>
  </>;
}

/** All accepted live rows stay mounted. Only genuinely visible removed rows may
 * occupy the shared 40-slot outgoing budget across overlapping batches. */
function HistoryTimeline({items, scope, reduced, empty, removal, sectionStart}: {items: readonly HistoryItem[]; scope: object; reduced: boolean; empty: ReactNode; removal: RefObject<HistoryRemoval | undefined>; sectionStart: RefObject<HTMLButtonElement | null>}) {
  const systemReduced = useReducedMotion(), reduce = reduced || systemReduced;
  const presence = useLayoutPresence(items, historyItemKey, reduce, 180, 320, {maxRetained: 40, staggerMs: 40, visibleOnly: true, scope});
  const keyboard = useRef(false), focused = useRef<HTMLElement | null>(null);
  useLayoutEffect(() => {
    const root = presence.root.current;if (!root) return;
    const order = Array.from(root.children).filter((node): node is HTMLElement => node instanceof HTMLElement && node.dataset.historyRow !== undefined);
    const starts = sectionStart.current ? [sectionStart.current] : [];
    const captured: HistoryRemoval = {scope, apply(keys) {
      if (removal.current !== captured || presence.root.current !== root || !root.isConnected || !keyboard.current) return;
      const held = focused.current, active = root.ownerDocument.activeElement;
      if (!held || active !== held && !(active instanceof Node && held.contains(active))) return;
      const outgoing = order.find(node => !keys.has(node.dataset.historyRow!) && node.contains(held));
      if (!outgoing) return;
      const surviving = order.filter(node => keys.has(node.dataset.historyRow!));
      const target = applyRemovalFocus({root, order, surviving, sectionStarts: starts}, outgoing);
      focused.current = target.element;
    }};
    removal.current = captured;
    return () => {if (removal.current === captured) removal.current = undefined;};
  }, [presence.present, scope, removal, sectionStart]);
  const headings = new Set<string>(), liveDays = new Set(items.map(item => item.dayKey));
  return <div ref={presence.root} tabIndex={-1}
    onKeyDownCapture={() => {keyboard.current = true;}} onPointerDownCapture={() => {keyboard.current = false;}}
    onFocusCapture={event => {const target = event.target as HTMLElement;focused.current = target;keyboard.current = target.matches(':focus-visible');}}
    onBlurCapture={event => {const next = event.relatedTarget as Node | null;if (next && !event.currentTarget.contains(next)) focused.current = null;}}
 className={styles.historyRoster} data-reduced-motion={reduce || undefined}>
    {presence.present.map(item => {
      const heading = !headings.has(item.dayKey);headings.add(item.dayKey);
      const leaving = presence.leaving.has(item.key);
      return <Fragment key={item.key}>
        {heading && <div className={styles.historyHeading} data-layout-item={`day:${item.dayKey}`} inert={!liveDays.has(item.dayKey)} aria-hidden={!liveDays.has(item.dayKey) || undefined}><h2>{historyDayLabel(item.day)}</h2></div>}
        <div className={styles.historyItem} data-layout-item={item.key} data-history-row={item.key} data-leaving={leaving || undefined} inert={leaving} aria-hidden={leaving || undefined}>
          <HistoryPaint exit={presence.exits.get(item.key)} reduced={reduce} scope={scope}><HistoryRow bindings={item.bindings} entry={item.entry} media={item.media} source={item.source} paintOnly={leaving} /></HistoryPaint>
        </div>
      </Fragment>;
    })}
    {!presence.present.length && empty && <div className={styles.historyLive}>{empty}</div>}
  </div>;
}
/** The drawn row names what was played: an episode leads with its code and title,
 * a song with its artist and album, a film with its kind and year, a book with its author. */
function historySubtitle(media: ContentEntry): readonly string[] {
  if (media.kind === 'episode' && media.episodeNumber !== undefined) {
    const code = episodeLabel(homeCardSubject(media).episode, 'ordinary', {t, locale: defaultI18n.region.formatLocale})?.text;
    return [...(code ? [code] : []), media.displayTitle ?? media.title];
  }
  if (media.kind === 'song' || media.kind === 'album') return [media.artist?.name, media.album?.name].filter((part): part is string => !!part);
  if (media.kind === 'book' || media.kind === 'audiobook_file') return media.author ? [media.author] : [];
  if (media.kind === 'movie') return [t('next.browse.kind.movie'), ...(media.year === undefined ? [] : [String(media.year)])];
  return media.year === undefined ? [] : [String(media.year)];
}
function HistoryRow({bindings, entry, media, source, paintOnly = false}: {bindings: SavedViewBindings; entry: PersonalResourceEntry; media: ContentEntry; source?: ArtworkSource; paintOnly?: boolean}) {
  // The row names the show for an episode, the title itself otherwise.
  const title = media.kind === 'episode' ? media.show?.name ?? media.displayTitle ?? media.title : media.displayTitle ?? media.title;
  const subtitles = historySubtitle(media), position = entry.positionSeconds ?? 0, duration = media.duration;
  const progress = duration && duration > 0 ? Math.min(1, position / duration) : undefined;
  const left = !entry.completed && duration !== undefined && duration > position ? Math.round((duration - position) / 60) : undefined;
  const remaining = left === undefined ? undefined : left >= 60 ? t(left % 60 ? 'next.kit.duration.hoursMinutes' : 'next.kit.duration.hours', {hours: Math.floor(left / 60), minutes: left % 60}) : t('next.kit.duration.minutes', {minutes: left});
  const square = shapeFor(media.kind) === 'square';
  return <div className={styles.historyRow} data-shape={square ? 'square' : 'wide'}>
    <button className={styles.historyArt} data-shape={square ? 'square' : 'wide'} disabled={paintOnly} onClick={paintOnly ? undefined : () => {if (bindings.operations.current) bindings.effects.openEntry(media);}} aria-label={title}>
      {square ? <><span className={styles.historyBackdrop}><Artwork source={source} alt="" /></span><span className={styles.historyCover}><Artwork source={source} alt="" /></span></> : <Artwork source={source} alt="" />}
    </button>
    <div className={styles.historyCopy}><b>{title}</b>{subtitles.length ? <small>{subtitles.map((part, index) => <span key={`${index}:${part}`}>{part}</span>)}</small> : null}</div>
    <div className={styles.historyState}>
      {entry.completed ? <small>{t(media.kind === 'song' || media.kind === 'album' ? 'next.kit.browse.played' : 'next.cards.finished')}</small>
        : progress !== undefined && remaining ? <><span className={styles.progress} aria-hidden="true"><span style={{width: `${progress * 100}%`}} /></span><small>{t('next.kit.remaining', {time: remaining})}</small></> : null}
    </div>
    <time dateTime={entry.updatedAt}>{entry.updatedAt && defaultI18n.time(entry.updatedAt)}</time>
  </div>;
}
