import {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import type {SavedEntry, SavedListFilter, SavedProjection, SavedRoute, SavedService} from '@core/saved.ts';
import type {PlaylistListFilter} from '@core/saved.ts';
import type {PersonalResource, PersonalResourceEntry, PersonalSavedService} from '@core/personal-saved.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import {shapeFor} from '@core/presentation/card-shape.ts';
import {dependsOn} from '@core/freshness-hub.ts';
import {Button, Chip, Select, Tabs, Switch} from '../../kit/controls';
import {FiltersButton, SortMenu, ViewToggle} from '../../kit/browse';
import {MenuRow, Popover} from '../../kit/overlays';
import {PlaylistMosaic, PosterCard, SquareCard, NewPlaylistCard, type CardSelection} from '../../kit/cards';
import {AmbientArt, type AmbientArtSource, type ArtworkSource} from '../../kit/artwork';
import {WindowedGrid} from '../../kit/layout';
import {Skeleton, StateView} from '../../kit/states';
import type {ShellArtwork} from '../../shell/artwork';
import {useQuietRefresh} from '../../platform/freshness';
import {homeCardModel} from '../home/model';
import {BrowseEntryCard} from '../browse';
import {LocalDownloads, ServerDownloads, type SavedDownloadEffects} from './Downloads';
import {sameSavedDownloadsScope, type SavedDownloadsPort} from './downloads-port.ts';
import type {SavedRuntime, SavedScreenRoute} from './runtime.ts';
import {savedSectionWindow} from './window.ts';
import {savedText as s} from './text.ts';
import {SavedHistory, SavedResourceScreen} from './Resource';
import {ResourceFields} from './ResourceFields';
import {SavedDialog} from './SavedDialog';
import type {SavedPresentation} from './presentation.ts';
export type {SavedPresentation} from './presentation.ts';
import type {SavedOperations} from './operations.ts';
import {sameSavedRoute, savedViewKey} from './route.ts';
import {useSavedViewOperations} from './use-view-operations.ts';
import styles from './saved.module.css';

const t = defaultI18n.t;
export type SavedSelection = Readonly<{
  toolbar: ReactNode;
  card(service: SavedService, sectionId: string, entry: SavedEntry): CardSelection | undefined;
  registerWindow(service: SavedService, sectionId: string, first: number, last: number): (() => void) | undefined;
  personalCard(service: PersonalSavedService, entry: PersonalResourceEntry): CardSelection | undefined;
}>;
export type SavedEffects = SavedDownloadEffects & Readonly<{
  openEntry(entry: ContentEntry): void;
  playEntry(entry: ContentEntry, signal: AbortSignal): Promise<void>;
  openMore?: (service: SavedService, sectionId: string, entry: SavedEntry, anchor: HTMLElement) => void;
  selectRoute(route: SavedScreenRoute): void;
  /** Present only when the playback owner supports this complete server resource. */
  playResource?: (target: Readonly<{kind: 'playlist' | 'collection' | 'view'; id: string; shuffle: boolean}>, signal: AbortSignal) => Promise<void>;
}>;
export type SavedBindings = Readonly<{
  runtime: SavedRuntime;
  route: SavedScreenRoute;
  online: boolean;
  downloadsOffered: boolean;
  localDownloads?: SavedDownloadsPort;
  artwork: ShellArtwork;
  preparationArtwork: Parameters<typeof ServerDownloads>[0]['artwork'];
  presentation: SavedPresentation;
  selection?: SavedSelection;
  effects: SavedEffects;
}>;
export type SavedViewBindings = SavedBindings & Readonly<{operations: SavedOperations}>;
const tabLabels = {watchlist: 'saved.tab.watchlist', favorites: 'saved.tab.favorites', playlists: 'saved.tab.playlists', collections: 'saved.tab.collections', views: 'saved.tab.savedViews', history: 'saved.tab.history', downloads: 'downloads.title'} as const;

export function SavedScreen(bindings: SavedBindings) {
  const {runtime, route, presentation, effects} = bindings;
  const viewKey = savedViewKey(runtime.scope, route), operations = useSavedViewOperations(runtime.operations, viewKey);
  const viewBindings: SavedViewBindings = {...bindings, operations};
  const [creating, setCreating] = useState<Readonly<{viewKey: string; kind: 'playlist' | 'collection'}>>();
  useEffect(() => {
    if (route.view === 'downloads') return;
    const current = ['collections', 'views', 'history', 'resource'].includes(route.view) ? runtime.personal.getSnapshot().route : runtime.saved.getSnapshot().route;
    const same = route.view === 'watchlist' || route.view === 'favorites'
      ? sameSavedRoute(runtime.posters.getSnapshot().route, {...route, shape: 'posters'}) && sameSavedRoute(runtime.squares.getSnapshot().route, {...route, shape: 'squares'})
      : sameSavedRoute(current, route);
    if (!same) void runtime.select(route).catch(effects.reportError);
  }, [runtime, route, effects.reportError]);
  const refresh = useCallback(async (context?: {signal?: AbortSignal; settle?: () => Promise<void>}) => {
    if (!runtime.operations.current || route.view === 'downloads') return;
    if (['collections', 'views', 'history', 'resource'].includes(route.view)) await Promise.all([runtime.personal.revalidate(context), route.view === 'collections' || route.view === 'views' ? runtime.summary.refresh(context?.signal) : Promise.resolve()]);
    else if (route.view === 'watchlist' || route.view === 'favorites') await Promise.all([runtime.posters.revalidate(context), runtime.squares.revalidate(context)]);
    else await runtime.saved.revalidate(context);
  }, [runtime, route.view]);
  useQuietRefresh('saved', refresh, () => runtime.operations.current, {dependencies: [dependsOn.personal(), dependsOn.progress(), dependsOn.content(), dependsOn.settings('preferences')], everyMs: 0});
  const localSnapshot = useSyncExternalStore(bindings.localDownloads?.subscribe ?? noSubscribe, bindings.localDownloads?.getSnapshot ?? noLocalSnapshot, noLocalSnapshot);
  const localUsable = !!bindings.localDownloads && sameSavedDownloadsScope(bindings.localDownloads.scope, runtime.scope) && !!localSnapshot?.available;
  useEffect(() => {
    if (localUsable && effects.saveReadyPreparation) runtime.installReadyDownloads(effects.saveReadyPreparation, effects.reportError);
    // The captured runtime, rather than this route's effect, owns retirement.
  }, [runtime, localUsable, effects.saveReadyPreparation, effects.reportError]);
  if (!runtime.operations.current) return null;
  if (route.view === 'playlist' || route.view === 'resource') return <SavedResourceScreen key={viewKey} bindings={viewBindings} />;
  const active = route.view;
  const tabs = (Object.keys(tabLabels) as (keyof typeof tabLabels)[]).filter(tab => tab !== 'downloads' || bindings.downloadsOffered || localUsable);
  const server = <ServerDownloads key={viewKey} service={runtime.downloads} operations={operations} effects={effects} artwork={bindings.preparationArtwork} online={bindings.online} />;
  return <main className={styles.page}>
    {presentation.backdrop && <div className={styles.wash} aria-hidden="true"><AmbientArt source={presentation.backdrop} treatment="hero" /></div>}
    <header className={styles.header}><h1>{t('saved.title')}</h1></header>
    <Tabs className={styles.tabs} listClassName={styles.tabsList} value={active} aria-label={t('web.saved.sections')} onValueChange={value => { if (value in tabLabels) effects.selectRoute({view: value as keyof typeof tabLabels}); }}
      items={tabs.map(tab => ({value: tab, label: t(tabLabels[tab])}))} />
    {route.view === 'downloads' ? localUsable && bindings.localDownloads
      ? <LocalDownloads key={viewKey} port={bindings.localDownloads} operations={operations} effects={effects} server={<ServerDownloads service={runtime.downloads} operations={operations} effects={effects} artwork={bindings.preparationArtwork} online={bindings.online} embedded managed={!!effects.saveReadyPreparation} />} /> : server
      : route.view === 'history' ? <SavedHistory key={viewKey} bindings={viewBindings} />
        : route.view === 'collections' || route.view === 'views' ? <PersonalResources key={viewKey} {...viewBindings} onCreate={() => setCreating({viewKey, kind: 'collection'})} />
          : <SavedLists key={viewKey} {...viewBindings} onCreate={() => setCreating({viewKey, kind: 'playlist'})} />}
    <CreateResourceDialog key={viewKey} kind={creating?.viewKey === viewKey ? creating.kind : undefined} bindings={viewBindings} close={() => setCreating(undefined)} />
  </main>;
}
const noSubscribe = () => () => {};
const noLocalSnapshot = () => undefined;

function SavedLists(bindings: SavedViewBindings & {onCreate: () => void}) {
  const {runtime, route, presentation, effects, artwork} = bindings;
  const first = route.view === 'watchlist' || route.view === 'favorites' ? runtime.posters : runtime.saved;
  const primary = useSyncExternalStore(first.subscribe, first.getSnapshot, first.getSnapshot), squares = useSyncExternalStore(runtime.squares.subscribe, runtime.squares.getSnapshot, runtime.squares.getSnapshot);
  const routeMatches = route.view === 'watchlist' || route.view === 'favorites'
    ? sameSavedRoute(primary.route, {...route, shape: 'posters'}) && sameSavedRoute(squares.route, {...route, shape: 'squares'}) : sameSavedRoute(primary.route, route);
  const split = route.view === 'watchlist' || route.view === 'favorites', projection = primary.projection ?? (split ? squares.projection : null);
  // The frame's wash comes from the list's own first titles. The canonical
  // library treatment carries it; no separate artwork request or tint is made.
  const wash = useMemo(() => {
    if (!presentation.backdrops) return [];
    if (presentation.backdrop) return [presentation.backdrop];
    const urls = (projection?.sections ?? []).flatMap(section => section.entries).flatMap(entry => {
      const media = mediaEntry(entry), url = media && (media.backdropUrl ?? media.posterUrl);
      return url ? [url] : [];
    });
    return [...new Set(urls)].slice(0, 3).flatMap(url => { const source = artwork.ambient(url); return source ? [source] : []; });
  }, [presentation.backdrops, presentation.backdrop, projection, artwork]);
  if (!routeMatches) return null;
  const list = route.view === 'favorites' ? 'favorites' : route.view === 'playlists' ? 'playlists' : 'watchlist';
  const total = split ? primary.phase === 'ready' && squares.phase === 'ready' ? [...(primary.projection?.sections ?? []), ...(squares.projection?.sections ?? [])].reduce((sum, section) => sum + section.totalCount, 0) : undefined : projection?.sections.reduce((sum, section) => sum + section.totalCount, 0);
  const hasContent = !!primary.projection?.sections.some(section => section.entries.length) || split && !!squares.projection?.sections.some(section => section.entries.length);
  if (!projection && primary.phase === 'error') return <StateView title={t('saved.loadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void runtime.select(route); }}} />;
  return <>
    {wash.length ? <div className={styles.wash} aria-hidden="true"><AmbientArt sources={wash} treatment="library" height={640} /></div> : null}
    {list === 'playlists'
      ? <div className={styles.toolbar}>
          {projection?.filters.map(filter => <Select key={filter.id} aria-label={defaultI18n.has(filter.labelKey) ? t(filter.labelKey) : t('playlists.filter')}
            value={'filter' in route ? route.filter ?? 'all' : 'all'} options={filter.options.map(option => ({value: option.id, label: option.label}))}
            onValueChange={filter => { if (route.view === 'playlists') effects.selectRoute({...route, filter: filter as PlaylistListFilter}); }} />)}
          {bindings.selection?.toolbar}
          <div className={styles.toolbarEnd}>
            {total !== undefined && <span>{t('web.saved.items', {count: total})}</span>}
            {projection?.sorts.length ? <Select aria-label={t('saved.sort')} value={'sort' in route ? route.sort ?? projection.query.sort : projection.query.sort}
              options={projection.sorts.map(sort => ({value: sort.id, label: savedSortName(sort)}))}
              onValueChange={sort => { if (route.view === 'playlists') effects.selectRoute({...route, sort}); }} /> : null}
            {projection?.actions?.includes('create_playlist') && <Button icon="i-plus" size="small" onClick={bindings.onCreate}>{t('web.saved.newPlaylist')}</Button>}
          </div>
        </div>
      : <SavedListToolbar bindings={bindings} projection={projection} total={total} />}
    {!hasContent && primary.phase === 'ready' && (!split || squares.phase === 'ready') ? <StateView title={t(`web.saved.empty.${list}`)} body={t(`web.saved.empty.${list}Body`)} /> : null}
    {primary.projection?.sections.map(section => <SavedSection key={`posters:${section.id}`} {...bindings} service={first} section={section} />)}
    {split && squares.projection?.sections.map(section => <SavedSection key={`squares:${section.id}`} {...bindings} service={runtime.squares} section={section} heading={{label: s('next.saved.musicAndAudiobooks'), count: section.totalCount}} />)}
    {list === 'playlists' && projection?.actions?.includes('create_playlist') && <div className={styles.newTile}><NewPlaylistCard model={{title: t('web.saved.newPlaylist'), caption: []}} onCreate={bindings.onCreate} /></div>}
  </>;
}

/** The catalogue names the server's saved sorts; an unknown one keeps the fallback. */
const savedSortKeys = {title: 'web.saved.sort.title', added: 'web.saved.sort.added', updated: 'web.saved.sort.recent', year: 'web.saved.sort.year', duration: 'web.saved.sort.duration', progress: 'web.saved.sort.progress'} as const;
function savedSortName(sort: Readonly<{id: string; labelKey: string}>): string {
  const key = savedSortKeys[sort.id as keyof typeof savedSortKeys];
  return key ? t(key) : defaultI18n.has(sort.labelKey) ? t(sort.labelKey) : t('web.saved.sort.other');
}
/** The saved list's own order, as the server published it. */
function savedSortSelection(route: SavedViewBindings['route'], projection: SavedProjection | null) {
  const field = ('sort' in route ? route.sort : undefined) ?? projection?.query.sort ?? 'title';
  const direction = (('direction' in route ? route.direction : undefined) ?? projection?.query.direction) === 'desc' ? 'desc' as const : 'asc' as const;
  const selected = projection?.sorts.find(sort => sort.id === field);
  return {field, direction, flippable: (selected?.directions.length ?? 0) > 1, reshufflable: false};
}

/** My List and Favorites: the library toolbar the frame draws, over the saved
 * list's published sorts and filter. The rows in the frame that the saved
 * contract does not publish (Watched, Favorites as a quick filter) are absent
 * rather than faked; the Filters menu carries the same published options. */
function SavedListToolbar({bindings, projection, total}: {bindings: SavedViewBindings; projection: SavedProjection | null; total: number | undefined}) {
  const {route, effects, presentation} = bindings;
  const listRoute = route.view === 'watchlist' || route.view === 'favorites' ? route : undefined;
  const filter = listRoute?.filter ?? 'all', [open, setOpen] = useState(false);
  const filters = projection?.filters ?? [], quick = filters.flatMap(entry => entry.options.filter(option => option.id !== 'all'));
  const sorts = projection?.sorts ?? [], sort = savedSortSelection(route, projection);
  const sortLabel = (value: Readonly<{id: string; labelKey: string}>) => savedSortName(value);
  return <div className={styles.toolbar}>
    {filters.length ? <>
      <Popover open={open} onOpenChange={setOpen} title={t('next.browse.filters')} accessibilityLabel={t('next.browse.filters')} placement="bottom-start" width={260}
        trigger={<FiltersButton count={filter === 'all' ? 0 : 1} />}>
        {quick.map(option => <MenuRow key={option.id} selected={filter === option.id} onClick={() => { setOpen(false); if (listRoute) effects.selectRoute({...listRoute, filter: filter === option.id ? 'all' : option.id as SavedListFilter}); }}>{option.label}</MenuRow>)}
      </Popover>
      <span className={styles.separator} aria-hidden="true" />
      {quick.map(option => <Chip key={option.id} variant="toolbar" selected={filter === option.id} onClick={() => { if (listRoute) effects.selectRoute({...listRoute, filter: filter === option.id ? 'all' : option.id as SavedListFilter}); }}>{option.label}</Chip>)}
    </> : null}
    {bindings.selection?.toolbar}
    <div className={styles.toolbarEnd}>
      {total !== undefined && <span className={styles.resultCount}>{t('lib.count', {count: total, pivot: 'other'})}</span>}
      {sorts.length ? <SortMenu sorts={sorts.map(value => ({...value, defaultDirection: value.directions[0] ?? 'asc', expensive: false, applicableKinds: []}))} value={sort} label={sortLabel}
        onValueChange={value => { if (listRoute) effects.selectRoute({...listRoute, sort: value.field, direction: value.direction}); }} /> : null}
      <ViewToggle value={presentation.view} onValueChange={presentation.setView} />
    </div>
  </div>;
}

export function SavedSection(bindings: SavedViewBindings & {service: SavedService; section: SavedProjection['sections'][number]; heading?: Readonly<{label: string; count?: number}>; onMore?: (entry: SavedEntry, anchor: HTMLButtonElement) => void}) {
  const {service, section, artwork, effects, presentation, selection} = bindings;
  const collection = useMemo(() => savedSectionWindow(service, section.id), [service, section.id]);
  const cleanup = useRef<(() => void) | undefined>(undefined);
  const root = useRef<HTMLElement>(null), [width, setWidth] = useState(0);
  useEffect(() => {
    const element = root.current; if (!element) return;
    const read = () => setWidth(element.clientWidth);
    const observer = new ResizeObserver(read); observer.observe(element); read();
    return () => observer.disconnect();
  }, []);
  useEffect(() => () => { cleanup.current?.(); cleanup.current = undefined; }, [collection]);
  const onVisibleRange = useCallback((first: number, last: number) => { cleanup.current?.(); cleanup.current = selection?.registerWindow(service, section.id, first, last); }, [selection, service, section.id]);
  const first = section.entries[0], media = first && mediaEntry(first), square = first?.kind === 'playlist' || media && shapeFor(media.kind) === 'square';
  const ratio = square ? 1 : 2 / 3, rowHeight = presentation.view === 'list' ? 90 : presentation.cardWidth / ratio + 48;
  const columns = presentation.view === 'list' ? 1 : Math.max(1, Math.floor((width + 16) / (presentation.cardWidth + 16)));
  const height = Math.max(rowHeight, Math.min(window.innerHeight, Math.ceil(section.entries.length / columns) * (rowHeight + 16)));
  return <section className={styles.savedSection} ref={root}>
    {bindings.heading && <div className={styles.sectionHeading}><h2>{bindings.heading.label}</h2>{bindings.heading.count !== undefined && <span>{defaultI18n.number(bindings.heading.count)}</span>}</div>}
    <WindowedGrid collection={collection} label={bindings.heading?.label ?? serverTextLabel(defaultI18n, section.heading)} height={height} cardWidth={presentation.view === 'list' ? Math.max(1, width) : presentation.cardWidth} rowHeight={rowHeight} rowGap={presentation.view === 'list' ? 0 : 16} gap={16} reducedMotion={presentation.reducedMotion} onVisibleRange={onVisibleRange}
      renderPlaceholder={() => <Skeleton width={presentation.cardWidth} height={presentation.cardWidth / ratio} />}
      renderItem={(entry, context) => {
        const cardSelection = selection?.card(service, section.id, entry), media = mediaEntry(entry);
        if (entry.kind === 'playlist') {
          const covers = (entry.artworkPaths ?? []).map(path => artwork.foreground(path, presentation.cardWidth, 1)).filter((source): source is ArtworkSource => !!source);
          return <PlaylistMosaic model={{title: entry.title, caption: entry.count !== undefined ? [{id: 'count', role: 'value', text: t('web.saved.items', {count: entry.count})}] : []}} covers={covers} context="grid" selection={cardSelection} onNavigate={() => effects.selectRoute({view: 'playlist', playlistId: entry.id})} onMore={effects.openMore ? anchor => effects.openMore!(service, section.id, entry, anchor) : undefined} />;
        }
        if (!media) return <StateView title={t('card.unavailable')} />;
        const common = {model: homeCardModel(media, false, {t, locale: defaultI18n.region.formatLocale}), artwork: artwork.foreground(media.posterUrl, presentation.cardWidth * devicePixelRatio, ratio),
          selection: cardSelection, context: 'grid' as const, watched: media.watched, onNavigate: () => effects.openEntry(media),
          onPlay: media.playback && media.available !== false && !media.missing ? () => { void bindings.operations.run(signal => effects.playEntry(media, signal)).catch(effects.reportError); } : undefined,
          onMore: bindings.onMore ? (anchor: HTMLButtonElement) => bindings.onMore!(entry, anchor)
            : effects.openMore ? (anchor: HTMLButtonElement) => effects.openMore!(service, section.id, entry, anchor) : undefined};
        if (presentation.view === 'list') return <div className={styles.mediaListRow}><PosterCard {...common} /></div>;
        return <BrowseEntryCard entry={media} artwork={artwork} env={{t, locale: defaultI18n.region.formatLocale}} width={presentation.cardWidth} mixed visible order={context.index} group={`saved:${section.id}`} backdrops={presentation.backdrops}
          actions={{onOpen: common.onNavigate, onPlay: common.onPlay, onMore: common.onMore, selection: cardSelection}} />;
      }} />
    {section.nextCursor && <Button className={styles.showMore} size="small" loading={service.getSnapshot().phase === 'loading'} onClick={() => { void service.next(section.id); }}>{t('action.showMore')}</Button>}
  </section>;
}
function mediaEntry(entry: SavedEntry): ContentEntry | undefined {
  return entry.kind === 'playlist' ? undefined : entry.kind === 'playlist_entry' ? entry.hidden ? undefined : entry.media : entry;
}

function PersonalResources(bindings: SavedViewBindings & {onCreate: () => void}) {
  const service = bindings.runtime.personal, snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot), kind = bindings.route.view === 'views' ? 'views' : 'collections';
  const summary = useSyncExternalStore(bindings.runtime.summary.subscribe, bindings.runtime.summary.getSnapshot, bindings.runtime.summary.getSnapshot);
  const count = kind === 'views' ? summary.value?.savedViews : summary.value?.collections;
  if (!sameSavedRoute(snapshot.route, bindings.route)) return null;
  return <>
    <div className={styles.toolbar}>{kind === 'collections' ? <><Button size="small" icon="i-plus" onClick={bindings.onCreate}>{t('web.saved.newCollection')}</Button><div className={styles.toolbarEnd}>{count !== undefined && t('lib.count', {count, pivot: 'collections'})}</div></>
      : <>{count !== undefined && <span className={styles.directoryCount}>{s('next.saved.savedViewsCount', {count})}</span>}<span className={styles.directoryHelp}>{s('next.saved.viewsHelp')}</span></>}</div>
    {snapshot.error && !snapshot.resources.length && <StateView title={t('saved.loadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void service.retryRead(); }}} />}
    {!snapshot.loading && !snapshot.error && !snapshot.resources.length && <StateView title={t(`web.saved.empty.${kind}`)} body={t(`web.saved.empty.${kind}Body`)} />}
    <div className={styles.resources}>{snapshot.resources.map(resource => <PersonalResourceTile key={resource.id} resource={resource} bindings={bindings} />)}</div>
    {snapshot.nextCursor && <Button className={styles.showMore} size="small" loading={snapshot.loading} onClick={() => { void service.next(); }}>{t('action.showMore')}</Button>}
  </>;
}
function PersonalResourceTile({resource, bindings}: {resource: PersonalResource; bindings: SavedViewBindings}) {
  const covers = (resource.artworkPaths ?? []).map(path => bindings.artwork.foreground(path, bindings.presentation.cardWidth, 1)).filter((source): source is ArtworkSource => !!source);
  return <PlaylistMosaic covers={covers} model={{title: resource.name, caption: [{id: 'count', role: 'value', text: t('web.saved.items', {count: resource.entryCount})}]}}
    context="grid" onNavigate={() => bindings.effects.selectRoute({view: 'resource', resourceId: resource.id})} />;
}

function CreateResourceDialog({kind, bindings, close}: {kind: 'playlist' | 'collection' | undefined; bindings: SavedViewBindings; close: () => void}) {
  const [name, setName] = useState(''), [summary, setSummary] = useState(''), [shared, setShared] = useState(false), [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { setName(''); setSummary(''); setShared(false); }, [kind]);
  const create = () => {
    if (!kind || busy || !name.trim() || !bindings.operations.current) return;
    setBusy(true);
    void bindings.operations.run(async () => {
      if (kind === 'playlist') await bindings.runtime.saved.mutate({action: 'create', name: name.trim(), summary});
      else if (!await bindings.runtime.personal.mutate({action: 'create', kind: 'collection', name: name.trim(), summary, visibility: shared ? 'server' : 'private'})) throw bindings.runtime.personal.getSnapshot().mutationError;
    }, () => { setBusy(false); close(); }).catch(error => { setBusy(false); bindings.effects.reportError(error); });
  };
  return <SavedDialog busy={busy} open={!!kind} onOpenChange={open => { if (!open && !busy) close(); }} title={t(kind === 'collection' ? 'web.saved.newCollection' : 'web.saved.newPlaylist')} initialFocusRef={input}
    footer={<><Button size="small" disabled={busy} onClick={close}>{t('action.cancel')}</Button><Button size="small" variant="primary" loading={busy} disabled={!name.trim()} onClick={create}>{t('web.saved.create')}</Button></>}>
    <form onSubmit={event => { event.preventDefault(); create(); }}><ResourceFields nameRef={input} name={name} summary={summary} onNameChange={setName} onSummaryChange={setSummary} nameMaxLength={kind === 'collection' ? 400 : 200} summaryMaxLength={kind === 'collection' ? 8000 : 4000} disabled={busy} />
      {kind === 'collection' && <><label className={styles.visibility}><span>{t('web.saved.visibleToAll')}</span><Switch checked={shared} disabled={busy} onCheckedChange={setShared} /></label><p className={styles.visibilityHelp}>{t(shared ? 'web.saved.sharedHelp' : 'web.saved.privateHelp')}</p></>}
    </form>
  </SavedDialog>;
}
