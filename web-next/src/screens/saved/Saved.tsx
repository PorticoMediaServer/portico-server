import {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import type {SavedEntry, SavedProjection, SavedRoute, SavedService} from '@core/saved.ts';
import type {PersonalResource, PersonalResourceEntry, PersonalSavedService} from '@core/personal-saved.ts';
import {serverTextLabel} from '@core/presentation/server-text.ts';
import {shapeFor} from '@core/presentation/card-shape.ts';
import {dependsOn} from '@core/freshness-hub.ts';
import {Button, Select, Tabs, TextField, Switch} from '../../kit/controls';
import {ViewToggle} from '../../kit/browse';
import {Dialog} from '../../kit/overlays';
import {PlaylistMosaic, PosterCard, SquareCard, NewPlaylistCard, type CardSelection} from '../../kit/cards';
import {AmbientArt, type AmbientArtSource, type ArtworkSource} from '../../kit/artwork';
import {WindowedGrid} from '../../kit/layout';
import {Skeleton, StateView} from '../../kit/states';
import type {ShellArtwork} from '../../shell/artwork';
import {useQuietRefresh} from '../../platform/freshness';
import {homeCardModel} from '../home/model';
import {LocalDownloads, ServerDownloads, type SavedDownloadEffects} from './Downloads';
import {sameSavedDownloadsScope, type SavedDownloadsPort} from './downloads-port.ts';
import type {SavedRuntime, SavedScreenRoute} from './runtime.ts';
import {savedSectionWindow} from './window.ts';
import {savedText as s} from './text.ts';
import {SavedHistory, SavedResourceScreen} from './Resource';
import styles from './saved.module.css';

const t = defaultI18n.t;
export type SavedSelection = Readonly<{
  toolbar: ReactNode;
  card(service: SavedService, sectionId: string, entry: SavedEntry): CardSelection | undefined;
  registerWindow(service: SavedService, sectionId: string, first: number, last: number): (() => void) | undefined;
  personalCard(service: PersonalSavedService, entry: PersonalResourceEntry): CardSelection | undefined;
}>;
/** Live presentation from the parent's canonical preference facade, never another
 * persisted view/key map. The parent owns setting effects and route history. */
export type SavedPresentation = Readonly<{
  view: 'grid' | 'list';
  cardWidth: number;
  reducedMotion: boolean;
  backdrop?: AmbientArtSource;
  setView(view: 'grid' | 'list'): void;
}>;
export type SavedEffects = SavedDownloadEffects & Readonly<{
  openEntry(entry: ContentEntry): void;
  playEntry(entry: ContentEntry): Promise<void>;
  openMore(service: SavedService, sectionId: string, entry: SavedEntry, anchor: HTMLElement): void;
  selectRoute(route: SavedScreenRoute): void;
  /** Present only when the playback owner supports this complete server resource. */
  playResource?: (target: Readonly<{kind: 'playlist' | 'collection' | 'view'; id: string; shuffle: boolean}>) => Promise<void>;
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
  selection: SavedSelection;
  effects: SavedEffects;
}>;
const tabLabels = {watchlist: 'saved.tab.watchlist', favorites: 'saved.tab.favorites', playlists: 'saved.tab.playlists', collections: 'saved.tab.collections', views: 'saved.tab.savedViews', history: 'saved.tab.history', downloads: 'downloads.title'} as const;

export function SavedScreen(bindings: SavedBindings) {
  const {runtime, route, presentation, effects} = bindings;
  const routeKey = JSON.stringify(route);
  const [creating, setCreating] = useState<'playlist' | 'collection'>();
  useEffect(() => {
    if (route.view === 'downloads') return;
    const current = ['collections', 'views', 'history', 'resource'].includes(route.view) ? runtime.personal.getSnapshot().route : runtime.saved.getSnapshot().route;
    const same = route.view === 'watchlist' || route.view === 'favorites'
      ? JSON.stringify(runtime.posters.getSnapshot().route) === JSON.stringify({...route, shape: 'posters'}) && JSON.stringify(runtime.squares.getSnapshot().route) === JSON.stringify({...route, shape: 'squares'})
      : JSON.stringify(current) === JSON.stringify(route);
    if (!same) void runtime.select(route).catch(effects.reportError);
  }, [runtime, route, effects.reportError]);
  const refresh = useCallback(async (context?: {signal?: AbortSignal; settle?: () => Promise<void>}) => {
    if (!runtime.operations.current || route.view === 'downloads') return;
    if (['collections', 'views', 'history', 'resource'].includes(route.view)) await runtime.personal.revalidate(context);
    else if (route.view === 'watchlist' || route.view === 'favorites') await Promise.all([runtime.posters.revalidate(context), runtime.squares.revalidate(context)]);
    else await runtime.saved.revalidate(context);
  }, [runtime, route.view]);
  useQuietRefresh('saved', refresh, () => runtime.operations.current, {dependencies: [dependsOn.personal(), dependsOn.progress(), dependsOn.content(), dependsOn.settings('preferences')], everyMs: 0});
  const localSnapshot = useSyncExternalStore(bindings.localDownloads?.subscribe ?? noSubscribe, bindings.localDownloads?.getSnapshot ?? noLocalSnapshot, noLocalSnapshot);
  const localUsable = !!bindings.localDownloads && sameSavedDownloadsScope(bindings.localDownloads.scope, runtime.scope) && !!localSnapshot?.available;
  if (!runtime.operations.current) return null;
  if (route.view === 'playlist' || route.view === 'resource') return <SavedResourceScreen key={routeKey} bindings={bindings} />;
  const active = route.view;
  const tabs = (Object.keys(tabLabels) as (keyof typeof tabLabels)[]).filter(tab => tab !== 'downloads' || bindings.downloadsOffered || localUsable);
  const server = <ServerDownloads service={runtime.downloads} operations={runtime.operations} effects={effects} artwork={bindings.preparationArtwork} online={bindings.online} />;
  return <main className={styles.page}>
    {presentation.backdrop && <div className={styles.wash} aria-hidden="true"><AmbientArt source={presentation.backdrop} treatment="hero" /></div>}
    <header className={styles.header}><h1>{t('saved.title')}</h1></header>
    <Tabs className={styles.tabs} listClassName={styles.tabsList} value={active} aria-label={t('web.saved.sections')} onValueChange={value => { if (value in tabLabels) effects.selectRoute({view: value as keyof typeof tabLabels}); }}
      items={tabs.map(tab => ({value: tab, label: t(tabLabels[tab])}))} />
    {route.view === 'downloads' ? localUsable && bindings.localDownloads
      ? <LocalDownloads port={bindings.localDownloads} operations={runtime.operations} effects={effects} /> : server
      : route.view === 'history' ? <SavedHistory bindings={bindings} />
        : route.view === 'collections' || route.view === 'views' ? <PersonalResources {...bindings} onCreate={() => setCreating('collection')} />
          : <SavedLists {...bindings} onCreate={() => setCreating('playlist')} />}
    <CreateResourceDialog kind={creating} bindings={bindings} close={() => setCreating(undefined)} />
  </main>;
}
const noSubscribe = () => () => {};
const noLocalSnapshot = () => undefined;

function SavedLists(bindings: SavedBindings & {onCreate: () => void}) {
  const {runtime, route, presentation, effects} = bindings;
  const first = route.view === 'watchlist' || route.view === 'favorites' ? runtime.posters : runtime.saved;
  const primary = useSyncExternalStore(first.subscribe, first.getSnapshot, first.getSnapshot), squares = useSyncExternalStore(runtime.squares.subscribe, runtime.squares.getSnapshot, runtime.squares.getSnapshot);
  const split = route.view === 'watchlist' || route.view === 'favorites', projection = primary.projection ?? (split ? squares.projection : null);
  const label = serverTextLabel(defaultI18n, projection?.heading), list = route.view === 'favorites' ? 'favorites' : route.view === 'playlists' ? 'playlists' : 'watchlist';
  const total = split ? primary.phase === 'ready' && squares.phase === 'ready' ? [...(primary.projection?.sections ?? []), ...(squares.projection?.sections ?? [])].reduce((sum, section) => sum + section.totalCount, 0) : undefined : projection?.sections.reduce((sum, section) => sum + section.totalCount, 0);
  const hasContent = !!primary.projection?.sections.some(section => section.entries.length) || split && !!squares.projection?.sections.some(section => section.entries.length);
  if (!projection && primary.phase === 'error') return <StateView title={t('saved.loadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void runtime.select(route); }}} />;
  return <>
    <div className={styles.toolbar}>
      {projection?.filters.map(filter => <Select key={filter.id} aria-label={defaultI18n.has(filter.labelKey) ? t(filter.labelKey) : t('playlists.filter')}
        value={'filter' in route ? route.filter ?? 'all' : 'all'} options={filter.options.map(option => ({value: option.id, label: option.label}))}
        onValueChange={filter => { if (route.view === 'watchlist' || route.view === 'favorites' || route.view === 'playlists') effects.selectRoute({...route, filter: filter as 'all' | 'unwatched' | 'inProgress'}); }} />)}
      {bindings.selection.toolbar}
      <div className={styles.toolbarEnd}>
        {total !== undefined && <span>{t(list === 'playlists' ? 'web.saved.items' : 'lib.count', list === 'playlists' ? {count: total} : {count: total, pivot: 'other'})}</span>}
        {projection?.sorts.length ? <Select aria-label={t('saved.sort')} value={'sort' in route ? route.sort ?? projection.query.sort : projection.query.sort}
          options={projection.sorts.map(sort => ({value: sort.id, label: defaultI18n.has(sort.labelKey) ? t(sort.labelKey) : t('web.saved.sort.other')}))}
          onValueChange={sort => { if (route.view !== 'playlist' && route.view !== 'downloads') effects.selectRoute({...route, sort}); }} /> : null}
        {list !== 'playlists' && <ViewToggle value={presentation.view} onValueChange={presentation.setView} />}
        {list === 'playlists' && projection?.actions?.includes('create_playlist') && <Button icon="i-plus" size="small" onClick={bindings.onCreate}>{t('web.saved.newPlaylist')}</Button>}
      </div>
    </div>
    {!hasContent && primary.phase === 'ready' && (!split || squares.phase === 'ready') ? <StateView title={t(`web.saved.empty.${list}`)} body={t(`web.saved.empty.${list}Body`)} /> : null}
    {primary.projection?.sections.map(section => <SavedSection key={section.id} {...bindings} service={first} section={section} heading={false} />)}
    {split && squares.projection?.sections.map(section => <SavedSection key={section.id} {...bindings} service={runtime.squares} section={section} heading />)}
    {list === 'playlists' && projection?.actions?.includes('create_playlist') && <div className={styles.newTile}><NewPlaylistCard model={{title: t('web.saved.newPlaylist'), caption: []}} onCreate={bindings.onCreate} /></div>}
  </>;
}

export function SavedSection(bindings: SavedBindings & {service: SavedService; section: SavedProjection['sections'][number]; heading: boolean}) {
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
  const onVisibleRange = useCallback((first: number, last: number) => { cleanup.current?.(); cleanup.current = selection.registerWindow(service, section.id, first, last); }, [selection, service, section.id]);
  const first = section.entries[0], media = first && mediaEntry(first), square = first?.kind === 'playlist' || media && shapeFor(media.kind) === 'square';
  const ratio = square ? 1 : 2 / 3, rowHeight = presentation.view === 'list' ? 90 : presentation.cardWidth / ratio + 48;
  const columns = presentation.view === 'list' ? 1 : Math.max(1, Math.floor((width + 16) / (presentation.cardWidth + 16)));
  const height = Math.max(rowHeight, Math.min(window.innerHeight, Math.ceil((section.entries.length + Number(!!section.nextCursor)) / columns) * (rowHeight + 30)));
  return <section className={styles.savedSection} ref={root}>
    {bindings.heading && <h2>{serverTextLabel(defaultI18n, section.heading)}</h2>}
    <WindowedGrid collection={collection} label={serverTextLabel(defaultI18n, section.heading)} height={height} cardWidth={presentation.view === 'list' ? Math.max(1, width) : presentation.cardWidth} rowHeight={rowHeight} rowGap={presentation.view === 'list' ? 0 : 30} gap={16} reducedMotion={presentation.reducedMotion} onVisibleRange={onVisibleRange}
      renderPlaceholder={() => <Skeleton width={presentation.cardWidth} height={presentation.cardWidth / ratio} />}
      renderItem={entry => {
        const cardSelection = selection.card(service, section.id, entry), media = mediaEntry(entry);
        if (entry.kind === 'playlist') {
          const covers = (entry.artworkPaths ?? []).map(path => artwork.foreground(path, presentation.cardWidth, 1)).filter((source): source is ArtworkSource => !!source);
          return <PlaylistMosaic model={{title: entry.title, caption: entry.count ? [{id: 'count', role: 'value', text: t('web.saved.items', {count: entry.count})}] : []}} covers={covers} context="grid" selection={cardSelection} onNavigate={() => effects.selectRoute({view: 'playlist', playlistId: entry.id})} onMore={anchor => effects.openMore(service, section.id, entry, anchor)} />;
        }
        if (!media) return <StateView title={t('card.unavailable')} />;
        const common = {model: homeCardModel(media, false, {t, locale: defaultI18n.region.formatLocale}), artwork: artwork.foreground(media.posterUrl, presentation.cardWidth * devicePixelRatio, ratio),
          selection: cardSelection, context: 'grid' as const, watched: media.watched, onNavigate: () => effects.openEntry(media),
          onPlay: media.playback && media.available !== false && !media.missing ? () => { void bindings.runtime.operations.run(() => effects.playEntry(media)).catch(effects.reportError); } : undefined,
          onMore: (anchor: HTMLButtonElement) => effects.openMore(service, section.id, entry, anchor)};
        if (presentation.view === 'list') return <div className={styles.mediaListRow}><PosterCard {...common} /></div>;
        return square ? <SquareCard {...common} /> : <PosterCard {...common} />;
      }} />
  </section>;
}
function mediaEntry(entry: SavedEntry): ContentEntry | undefined {
  return entry.kind === 'playlist' ? undefined : entry.kind === 'playlist_entry' ? entry.hidden ? undefined : entry.media : entry;
}

function PersonalResources(bindings: SavedBindings & {onCreate: () => void}) {
  const service = bindings.runtime.personal, snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot), kind = bindings.route.view === 'views' ? 'views' : 'collections';
  return <>
    <div className={styles.toolbar}><div className={styles.toolbarEnd}>{kind === 'collections' && <Button size="small" icon="i-plus" onClick={bindings.onCreate}>{t('web.saved.newCollection')}</Button>}</div></div>
    {snapshot.error && !snapshot.resources.length && <StateView title={t('saved.loadFailed')} primary={{id: 'retry', label: t('action.tryAgain'), onPress: () => { void service.retryRead(); }}} />}
    {!snapshot.loading && !snapshot.error && !snapshot.resources.length && <StateView title={t(`web.saved.empty.${kind}`)} body={t(`web.saved.empty.${kind}Body`)} />}
    <div className={styles.resources}>{snapshot.resources.map(resource => <PersonalResourceTile key={resource.id} resource={resource} bindings={bindings} />)}</div>
    {snapshot.nextCursor && <Button className={styles.showMore} size="small" loading={snapshot.loading} onClick={() => { void service.next(); }}>{t('action.showMore')}</Button>}
  </>;
}
function PersonalResourceTile({resource, bindings}: {resource: PersonalResource; bindings: SavedBindings}) {
  const covers = (resource.artworkPaths ?? []).map(path => bindings.artwork.foreground(path, bindings.presentation.cardWidth, 1)).filter((source): source is ArtworkSource => !!source);
  return <PlaylistMosaic covers={covers} model={{title: resource.name, caption: [{id: 'count', role: 'value', text: t('web.saved.items', {count: resource.entryCount})}]}}
    context="grid" onNavigate={() => bindings.effects.selectRoute({view: 'resource', resourceId: resource.id})} />;
}

function CreateResourceDialog({kind, bindings, close}: {kind: 'playlist' | 'collection' | undefined; bindings: SavedBindings; close: () => void}) {
  const [name, setName] = useState(''), [shared, setShared] = useState(false), [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => { setName(''); setShared(false); }, [kind]);
  const create = () => {
    if (!kind || busy || !name.trim() || !bindings.runtime.operations.current) return;
    setBusy(true);
    void bindings.runtime.operations.run(async () => {
      if (kind === 'playlist') await bindings.runtime.saved.mutate({action: 'create', name: name.trim()});
      else if (!await bindings.runtime.personal.mutate({action: 'create', kind: 'collection', name: name.trim(), visibility: shared ? 'server' : 'private'})) throw bindings.runtime.personal.getSnapshot().mutationError;
    }, () => { setBusy(false); close(); }).catch(error => { setBusy(false); bindings.effects.reportError(error); });
  };
  return <Dialog open={!!kind} onOpenChange={open => { if (!open && !busy) close(); }} title={t(kind === 'collection' ? 'web.saved.newCollection' : 'web.saved.newPlaylist')} initialFocusRef={input}
    footer={<><Button size="small" disabled={busy} onClick={close}>{t('action.cancel')}</Button><Button size="small" variant="primary" loading={busy} disabled={!name.trim()} onClick={create}>{t('web.saved.create')}</Button></>}>
    <form onSubmit={event => { event.preventDefault(); create(); }}><TextField ref={input} label={t('lib.saveViewName')} value={name} maxLength={kind === 'collection' ? 400 : 200} disabled={busy} onChange={event => setName(event.currentTarget.value)} />
      {kind === 'collection' && <label className={styles.visibility}><span>{t('web.saved.visibleToAll')}</span><Switch checked={shared} disabled={busy} onCheckedChange={setShared} /></label>}
    </form>
  </Dialog>;
}
