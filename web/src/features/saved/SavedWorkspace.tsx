import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Bookmark,
  Check,
  ChevronLeft,
  ChevronRight,
  FolderHeart,
  Heart,
  ListMusic,
  Lock,
  Pencil,
  Pin,
  Play,
  Plus,
  RefreshCw,
  Trash2,
  Users,
  X,
} from '#portico-icons';
import { productMessage } from '@porticomediaserver/client-core';
import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { SelectMenu } from '../../components/controls/SelectMenu';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import {
  useAuthSession,
  useFavorites,
  useLibraries,
  useMediaMutations,
  useSavedMutations,
  useSavedResource,
  useSavedResources,
  useWatchlist,
} from '../../data/DataProvider';
import { canManageServer } from '../../data/authority';
import type {
  CollectionMembershipMutation,
  LibraryKind,
  MediaItem,
  PlaylistItemsMutation,
  SavedPlaylistEntry,
  SavedResourceKind,
  SavedResourceSummary,
} from '../../data/models';
import {
  MediaActionMenu,
  MediaArtwork,
  MediaCard,
  mediaDetailPath,
  SectionHeading,
  SelectableMediaGrid,
} from '../catalog/CatalogSurface';
import { mediaPresentation, type MediaPresentationGroup } from '../catalog/mediaPresentation';
import { targetFromMedia } from '../media/contextTarget';
import { usePlaybackSession } from '../player/PlayerSurface';
import { SavedShareEditor, type SavedShareEditorValue } from './SavedShareEditor';
import './saved.css';

const savedTabs = [
  ['watchlist', productMessage('library.quick-watchlist').text ?? ''],
  ['favorites', productMessage('library.quick-favorites').text ?? ''],
  ['playlists', productMessage('destination.playlists').text ?? ''],
  ['collections', productMessage('destination.collections').text ?? ''],
  ['views', productText('saved.views')],
] as const;

const noMediaItems: MediaItem[] = [];

type SavedTab = typeof savedTabs[number][0];

function kindFromTab(tab: SavedTab): SavedResourceKind | undefined {
  if (tab === 'playlists') return 'playlist';
  if (tab === 'collections') return 'collection';
  if (tab === 'views') return 'view';
  return undefined;
}

function kindFromRoute(value: string | undefined): SavedResourceKind | undefined {
  if (value === 'playlists') return 'playlist';
  if (value === 'collections') return 'collection';
  if (value === 'views') return 'view';
  return undefined;
}

function resourcePath(kind: SavedResourceKind, id: string) {
  return `/saved/${kind === 'view' ? 'views' : `${kind}s`}/${encodeURIComponent(id)}`;
}

function kindLabel(kind: SavedResourceKind) {
  return kind === 'view'
    ? 'Saved view'
    : productMessage(kind === 'playlist' ? 'media.kind.playlist' : 'media.kind.collection').text ?? '';
}

function mediaItemCount(count: number) {
  return productMessage(count === 1 ? 'media.item-count-single' : 'media.item-count', { count }).text ?? '';
}

function kindIcon(kind: SavedResourceKind) {
  return kind === 'view' ? Pin : kind === 'playlist' ? ListMusic : FolderHeart;
}

type SavedResourceWithSharing = SavedResourceSummary & {
  ownerUserId?: string;
  shares?: SavedShareEditorValue[];
};

function savedAccessSummary(resource: SavedResourceWithSharing) {
  const shareCount = resource.shares?.length ?? 0;
  if (resource.visibility === 'server') return shareCount > 0 ? `Everyone on this server · ${shareCount} named ${shareCount === 1 ? 'share' : 'shares'}` : 'Everyone on this server';
  if (shareCount > 0) return `Shared with ${shareCount} ${shareCount === 1 ? 'person' : 'people'}`;
  return 'Private';
}

function defaultPivot(kind: LibraryKind | undefined) {
  if (kind === 'music') return 'albums';
  if (kind === 'audiobooks') return 'books';
  if (kind === 'recorded-tv') return 'recordings';
  if (kind === 'tv' || kind === 'anime') return 'shows';
  return 'movies';
}

function viewPivots(kind: LibraryKind | undefined) {
  if (kind === 'music') return ['artists', 'albums', 'tracks', 'playlists', 'genres'];
  if (kind === 'audiobooks') return ['authors', 'series', 'books', 'collections', 'categories'];
  if (kind === 'recorded-tv') return ['recordings', 'shows', 'collections'];
  if (kind === 'tv' || kind === 'anime') return ['shows', 'episodes', 'collections', 'categories'];
  return ['movies', 'collections', 'categories'];
}

function isConflict(reason: unknown) {
  if (!reason || typeof reason !== 'object') return false;
  const candidate = reason as { status?: number; code?: string; problem?: { status?: number; code?: string } };
  return candidate.status === 409
    || candidate.problem?.status === 409
    || candidate.code?.toLocaleLowerCase().includes('conflict')
    || candidate.problem?.code?.toLocaleLowerCase().includes('conflict');
}

function SavedState({ icon: Icon, title, detail, error, onRetry }: { icon: typeof Bookmark; title: string; detail: string; error?: boolean; onRetry?: () => void }) {
  return <div className={`saved-state ${error ? 'error' : ''}`} role={error ? 'alert' : 'status'}>
    <Icon />
    <strong>{title}</strong>
    <p>{detail}</p>
    {onRetry && <SecondaryButton onClick={onRetry}><RefreshCw /> {productMessage('action.retry').text}</SecondaryButton>}
  </div>;
}

function SavedSelectionBar({ count, busy, action, onAction, onClear }: { count: number; busy: boolean; action: string; onAction: () => void; onClear: () => void }) {
  return <section className="portico-saved-selection" aria-label={productMessage('library.selection-label', { count }).text}>
    <div><strong>{count}</strong><span>{productMessage('library.selected-label').text}</span><button type="button" onClick={onClear}>{productMessage('action.clear-selection').text}</button></div>
    <SecondaryButton disabled={busy} onClick={onAction}><Trash2 /> {busy ? 'Removing…' : action}</SecondaryButton>
    <IconButton label={productMessage('action.cancel-selection').text ?? ''} onClick={onClear}><X /></IconButton>
  </section>;
}

function groupedItems(items: MediaItem[]) {
  const order: MediaPresentationGroup[] = ['movies', 'shows', 'episodes', 'music', 'audiobooks', 'live-dvr', 'collections-playlists', 'people', 'other'];
  const titles: Record<MediaPresentationGroup, string> = {
    movies: 'Movies',
    shows: 'Shows',
    episodes: 'Episodes',
    music: 'Music',
    audiobooks: 'Audiobooks',
    'live-dvr': 'Live & DVR',
    'collections-playlists': 'Collections & playlists',
    people: 'People',
    other: productMessage('media.kind.media').text ?? 'Media',
  };
  const grouped = new Map<MediaPresentationGroup, MediaItem[]>();
  for (const item of items) {
    const group = mediaPresentation(item).group;
    grouped.set(group, [...(grouped.get(group) ?? []), item]);
  }
  return order.flatMap((id) => {
    const groupItems = grouped.get(id);
    if (!groupItems?.length) return [];
    const shapes = new Set(groupItems.map((item) => mediaPresentation(item).artworkShape));
    const shape = shapes.size === 1 ? [...shapes][0] : undefined;
    return [{ id, title: titles[id], items: groupItems, shape }];
  });
}

function ControlledMediaGroups({ items, selected, onToggle, onWatchlistChange, onFavoriteChange }: { items: MediaItem[]; selected: string[]; onToggle?: (id: string) => void; onWatchlistChange?: (id: string, saved: boolean) => void; onFavoriteChange?: (id: string, favorite: boolean) => void }) {
  const groups = groupedItems(items);
  return <div className="portico-saved-media-groups">{groups.map((group) => <section key={group.id}>
    {groups.length > 1 && <SectionHeading title={group.title} detail={mediaItemCount(group.items.length)} />}
    <div className={`portico-saved-media-grid ${group.shape ?? 'mixed'}`}>{group.items.map((item) => <MediaCard key={item.id} item={item} shape={mediaPresentation(item).artworkShape === 'square' ? 'square' : undefined} selected={selected.includes(item.id)} onSelect={onToggle ? () => onToggle(item.id) : undefined} onWatchlistChange={onWatchlistChange} onFavoriteChange={onFavoriteChange} />)}</div>
  </section>)}</div>;
}

type PersonalMediaQuery = ReturnType<typeof useWatchlist>;

function PersonalMediaPanel({ mode, query, onReload }: { mode: 'watchlist' | 'favorites'; query: PersonalMediaQuery; onReload: () => void }) {
  const mediaMutations = useMediaMutations();
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const items = query.status === 'success' ? query.data : noMediaItems;
  useEffect(() => setSelected((current) => current.filter((id) => items.some((item) => item.id === id))), [items]);
  const toggle = (id: string) => setSelected((current) => current.includes(id) ? current.filter((candidate) => candidate !== id) : [...current, id]);
  const removeSelected = async () => {
    setBusy(true);
    setNotice('');
    setError('');
    const results = await Promise.allSettled(selected.map((id) => mode === 'watchlist' ? mediaMutations.setWatchlist(id, false) : mediaMutations.setFavorite(id, false)));
    const failures = results.flatMap((result, index) => result.status === 'rejected' ? [selected[index]] : []);
    if (failures.length) {
      setSelected(failures);
      setError(`${selected.length - failures.length} removed; ${failures.length} still selected for retry.`);
    } else {
      setSelected([]);
      setNotice(`${selected.length} ${selected.length === 1 ? 'item' : 'items'} removed.`);
    }
    setBusy(false);
    onReload();
  };

  if (query.status === 'loading') return <SavedState icon={RefreshCw} title={`Loading ${mode}`} detail="Fetching your saved media from this server." />;
  if (query.status === 'error') return <SavedState error icon={AlertTriangle} title={`Couldn’t load ${mode}`} detail={reviewedProductErrorText(query.error, 'problem.request-failed')} onRetry={onReload} />;
  if (!items.length) return <SavedState icon={mode === 'watchlist' ? Bookmark : Heart} title={mode === 'watchlist' ? 'Your watchlist is empty' : 'No favorites yet'} detail={mode === 'watchlist' ? 'Add titles from Home, Search, or any library.' : 'Favorite a movie, show, episode, artist, album, or track to keep it close.'} />;
  return <>
    {(notice || error) && <div className={`portico-saved-notice ${error ? 'error' : ''}`} role={error ? 'alert' : 'status'}>{error || notice}</div>}
    <ControlledMediaGroups items={items} selected={selected} onToggle={toggle} onWatchlistChange={mode === 'watchlist' ? (_, saved) => { if (!saved) onReload(); } : undefined} onFavoriteChange={mode === 'favorites' ? (_, favorite) => { if (!favorite) onReload(); } : undefined} />
    {selected.length > 0 && <SavedSelectionBar count={selected.length} busy={busy} action={`Remove from ${mode === 'watchlist' ? 'watchlist' : 'favorites'}`} onAction={() => void removeSelected()} onClear={() => setSelected([])} />}
  </>;
}

function WatchlistPanel() {
  const [reloadKey, setReloadKey] = useState(0);
  const query = useWatchlist(reloadKey);
  return <PersonalMediaPanel mode="watchlist" query={query} onReload={() => setReloadKey((value) => value + 1)} />;
}

function FavoritesPanel() {
  const [reloadKey, setReloadKey] = useState(0);
  const query = useFavorites(reloadKey);
  return <PersonalMediaPanel mode="favorites" query={query} onReload={() => setReloadKey((value) => value + 1)} />;
}

function SavedResourceList({ kind, reloadKey, onRetry }: { kind: SavedResourceKind; reloadKey: number; onRetry: () => void }) {
  const query = useSavedResources(kind, reloadKey);
  const Icon = kindIcon(kind);
  if (query.status === 'loading') return <SavedState icon={RefreshCw} title={`Loading ${kindLabel(kind).toLocaleLowerCase()}s`} detail="Fetching this section from your server." />;
  if (query.status === 'error') return <SavedState error icon={AlertTriangle} title={`Couldn’t load ${kindLabel(kind).toLocaleLowerCase()}s`} detail={reviewedProductErrorText(query.error, 'problem.request-failed')} onRetry={onRetry} />;
  if (!query.data.length) return <SavedState icon={Icon} title={`No ${kindLabel(kind).toLocaleLowerCase()}s yet`} detail={kind === 'view' ? 'Save a useful library query so it stays available across Portico clients.' : `Create a ${kindLabel(kind).toLocaleLowerCase()} to organize media without changing the library itself.`} />;
  return <div className="portico-saved-resource-list">{query.data.map((resource) => <Link to={resourcePath(kind, resource.id)} key={resource.id}>
    <span className={`portico-saved-resource-icon ${kind}`}><Icon /></span>
    <span className="portico-saved-resource-copy"><strong>{resource.title}</strong><small>{resource.summary || (kind === 'view' ? `${resource.libraryName ?? 'Library'} · ${resource.pivot ?? 'Browse'}` : mediaItemCount(resource.itemCount))}</small></span>
    <span className="portico-saved-resource-access">{kind === 'view' ? (resource.isPinned ? <><Pin /> {productMessage('preferences.library-status-pinned').text}</> : productMessage('navigation.saved').text) : resource.visibility === 'server' ? <><Users /> {productMessage('settings.label.server').text}</> : <><Lock /> Private</>}</span>
    <ChevronRight />
  </Link>)}</div>;
}

function SavedResourceEditor({ kind, resource, onDismiss, onSaved }: { kind: SavedResourceKind; resource?: SavedResourceSummary; onDismiss: () => void; onSaved: (resource: SavedResourceSummary) => void }) {
  const auth = useAuthSession();
  const mutations = useSavedMutations();
  const libraries = useLibraries();
  const sharedResource = resource as SavedResourceWithSharing | undefined;
  const canPublishToServer = canManageServer(auth.viewer?.user);
  const losesServerVisibility = resource?.visibility === 'server' && !canPublishToServer;
  const [title, setTitle] = useState(resource?.title ?? '');
  const [summary, setSummary] = useState(resource?.summary ?? '');
  const [visibility, setVisibility] = useState<'private' | 'server'>(losesServerVisibility ? 'private' : resource?.visibility ?? 'private');
  const [shares, setShares] = useState<SavedShareEditorValue[]>(() => (sharedResource?.shares ?? []).map((share) => ({ userId: share.userId, displayName: share.displayName, canEdit: share.canEdit })));
  const [libraryId, setLibraryId] = useState(resource?.libraryId ?? '');
  const [pivot, setPivot] = useState(resource?.pivot ?? '');
  const [pinned, setPinned] = useState(resource?.isPinned ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const availableLibraries = libraries.status === 'success' ? libraries.data : [];
  const selectedLibrary = availableLibraries.find((library) => library.id === libraryId);
  useEffect(() => {
    if (kind !== 'view' || libraryId || !availableLibraries[0]) return;
    setLibraryId(availableLibraries[0].id);
    setPivot(defaultPivot(availableLibraries[0].kind));
  }, [availableLibraries, kind, libraryId]);
  const submit = async () => {
    if (!title.trim()) {
      setError('Enter a name.');
      return;
    }
    if (kind === 'view' && (!libraryId || !pivot)) {
      setError('Choose a library and view.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const input = {
        title: title.trim(),
        summary: summary.trim() || undefined,
        visibility,
        shares: kind === 'view' ? undefined : shares.map((share) => ({ userId: share.userId, canEdit: share.canEdit })),
        libraryId,
        pivot,
        isPinned: pinned,
      };
      onSaved(resource ? await mutations.update(kind, resource.id, input) : await mutations.create(kind, input));
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'problem.request-failed'));
    } finally {
      setBusy(false);
    }
  };
  const titleId = `portico-saved-${kind}-editor-title`;
  return <ModalOverlay labelledBy={titleId} className={`portico-saved-dialog portico-saved-editor ${kind !== 'view' ? 'has-sharing' : ''}`} onDismiss={onDismiss}>
    <header><div><p>{resource ? 'Edit' : 'Create'}</p><h2 id={titleId}>{resource?.title ?? `New ${kindLabel(kind).toLocaleLowerCase()}`}</h2></div><IconButton label={productMessage('action.close').text ?? ''} onClick={onDismiss}><X /></IconButton></header>
    <div className="portico-saved-editor-fields">
      <label><span>{productMessage('library.save-name').text}</span><input autoFocus value={title} maxLength={160} onChange={(event) => setTitle(event.target.value)} /></label>
      {kind !== 'view' && <label><span>Description <small>Optional</small></span><textarea value={summary} rows={4} onChange={(event) => setSummary(event.target.value)} /></label>}
      {kind !== 'view' && <div className="portico-saved-editor-choice portico-saved-visibility"><span>Server visibility</span>{canPublishToServer
        ? <SelectMenu label="Who can view" value={visibility} options={[{ id: 'private', label: 'Private' }, { id: 'server', label: 'Everyone on this server' }]} onChange={(value) => setVisibility(value === 'server' ? 'server' : 'private')} />
        : <div className="portico-saved-visibility-value"><Lock /> Private</div>}
      <p>{losesServerVisibility
        ? 'Server-wide access requires server management permission. Saving will change this to private.'
        : canPublishToServer ? 'Named access is managed separately below.' : 'Only server managers can share with everyone. You can still add people below.'}</p></div>}
      {kind !== 'view' && <SavedShareEditor shares={shares} visibility={visibility} onChange={setShares} />}
      {kind === 'view' && <>
        {libraries.status === 'loading' && <p className="portico-saved-editor-support">{productMessage('preferences.libraries-loading').title}</p>}
        {libraries.status === 'error' && <p className="portico-saved-dialog-error" role="alert">{reviewedProductErrorText(libraries.error, 'problem.request-failed')}</p>}
        {availableLibraries.length > 0 && <div className="portico-saved-editor-choice"><span>Library</span><SelectMenu label="Library" value={selectedLibrary?.id ?? availableLibraries[0].id} options={availableLibraries.map((library) => ({ id: library.id, label: library.name }))} onChange={(id) => { const library = availableLibraries.find((candidate) => candidate.id === id); if (!library) return; setLibraryId(library.id); setPivot(defaultPivot(library.kind)); }} /></div>}
        {selectedLibrary && <div className="portico-saved-editor-choice"><span>View</span><SelectMenu label="View" value={pivot || defaultPivot(selectedLibrary.kind)} options={viewPivots(selectedLibrary.kind).map((view) => ({ id: view, label: view }))} onChange={setPivot} /></div>}
        <label className="portico-saved-pin"><input type="checkbox" checked={pinned} onChange={(event) => setPinned(event.target.checked)} /><span><strong>Pin this view</strong><small>Keep it available for faster access.</small></span></label>
      </>}
      {error && <p className="portico-saved-dialog-error" role="alert"><AlertTriangle /> {error}</p>}
    </div>
    <footer><SecondaryButton onClick={onDismiss}>{productMessage('action.cancel').text}</SecondaryButton><PrimaryButton disabled={busy || (kind === 'view' && libraries.status !== 'success')} onClick={() => void submit()}>{busy ? productMessage('action.saving').text : resource ? productMessage('action.save-changes').text : `Create ${kindLabel(kind).toLocaleLowerCase()}`}</PrimaryButton></footer>
  </ModalOverlay>;
}

export function SavedPage() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const requested = (params.get('tab') ?? params.get('section')) as SavedTab | null;
  const tab: SavedTab = savedTabs.some(([id]) => id === requested) ? requested as SavedTab : 'watchlist';
  const kind = kindFromTab(tab);
  const [editorOpen, setEditorOpen] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const selectTab = (next: SavedTab) => {
    const updated = new URLSearchParams(params);
    updated.delete('section');
    if (next === 'watchlist') updated.delete('tab'); else updated.set('tab', next);
    setParams(updated);
    setEditorOpen(false);
  };
  return <div className="standard-page portico-saved-page">
    <header className="portico-saved-header"><div><p className="route-context">Your media</p><h1>{productMessage('navigation.saved').text}</h1><p>Watchlist, favorites, playlists, collections, and reusable library views.</p></div>{kind && <PrimaryButton onClick={() => setEditorOpen(true)}><Plus /> New {kindLabel(kind).toLocaleLowerCase()}</PrimaryButton>}</header>
    <nav className="portico-saved-tabs" aria-label="Saved sections">{savedTabs.map(([id, label]) => <button type="button" className={tab === id ? 'active' : ''} aria-current={tab === id ? 'page' : undefined} key={id} onClick={() => selectTab(id)}>{label}</button>)}</nav>
    <div className="portico-saved-content">
      {tab === 'watchlist' && <WatchlistPanel />}
      {tab === 'favorites' && <FavoritesPanel />}
      {kind && <SavedResourceList kind={kind} reloadKey={reloadKey} onRetry={() => setReloadKey((value) => value + 1)} />}
    </div>
    {editorOpen && kind && <SavedResourceEditor kind={kind} onDismiss={() => setEditorOpen(false)} onSaved={(resource) => { setEditorOpen(false); setReloadKey((value) => value + 1); navigate(resourcePath(resource.kind, resource.id)); }} />}
  </div>;
}

function PlaylistItems({ entries, selected, busy, canEdit, canReorder, onToggle, onMove, onRemove }: { entries: SavedPlaylistEntry[]; selected: string[]; busy: boolean; canEdit: boolean; canReorder: boolean; onToggle: (entryId: string) => void; onMove: (index: number, direction: number) => void; onRemove: (entryIds: string[]) => void }) {
  return <ol className="portico-playlist-order">{entries.map((entry, index) => {
    const item = entry.media;
    const selectedEntry = selected.includes(entry.entryId);
    const position = entry.position + 1;
    return <li key={entry.entryId} className={selectedEntry ? 'selected' : ''}>
      {canEdit
        ? <button type="button" className={`selection-check row-check ${selectedEntry ? 'selected' : ''}`} onClick={() => onToggle(entry.entryId)} aria-label={`${productMessage(selectedEntry ? 'library.deselect-item' : 'library.select-item', { title: item.title }).text}, position ${position}`} aria-pressed={selectedEntry}>{selectedEntry && <Check />}</button>
        : <span aria-hidden="true" />}
      <span className="portico-playlist-index">{position}</span>
      <span className={`portico-playlist-art ${mediaPresentation(item).artworkShape}`}><MediaArtwork item={item} shape={mediaPresentation(item).artworkShape === 'square' ? 'square' : 'poster'} /></span>
      <Link to={mediaDetailPath(item) ?? '#'}><strong>{item.title}</strong><small>{item.subtitle || [item.parentTitle, item.year || undefined, item.length || undefined].filter(Boolean).join(' · ')}</small></Link>
      <span className="portico-playlist-duration">{item.length}</span>
      <div className="portico-playlist-actions">
        {canEdit && <>
          <IconButton label={`${productMessage('action.move-queue-earlier', { title: item.title }).text}, position ${position}`} disabled={busy || !canReorder || index === 0} onClick={() => onMove(index, -1)}><ArrowUp /></IconButton>
          <IconButton label={`${productMessage('action.move-queue-later', { title: item.title }).text}, position ${position}`} disabled={busy || !canReorder || index === entries.length - 1} onClick={() => onMove(index, 1)}><ArrowDown /></IconButton>
          <IconButton label={`Remove ${item.title}, position ${position}`} disabled={busy} onClick={() => onRemove([entry.entryId])}><Trash2 /></IconButton>
        </>}
        <MediaActionMenu target={targetFromMedia(item)} />
      </div>
    </li>;
  })}</ol>;
}

function ReadOnlySavedItems({ items }: { items: MediaItem[] }) {
  const groups = groupedItems(items);
  return <div className="portico-saved-media-groups">{groups.map((group) => <section key={group.id}>
    {groups.length > 1 && <SectionHeading title={group.title} detail={mediaItemCount(group.items.length)} />}
    <SelectableMediaGrid items={group.items} shape={group.shape === 'square' ? 'square' : undefined} className="portico-saved-media-grid" />
  </section>)}</div>;
}

function SavedResourceDetail({ kind, id }: { kind: SavedResourceKind; id: string }) {
  const navigate = useNavigate();
  const auth = useAuthSession();
  const playback = usePlaybackSession();
  const mutations = useSavedMutations();
  const [reloadKey, setReloadKey] = useState(0);
  const [cursor, setCursor] = useState<string>();
  const query = useSavedResource(kind, id, { cursor, limit: 50 }, reloadKey);
  const [resource, setResource] = useState<SavedResourceSummary>();
  const [playlistEntries, setPlaylistEntries] = useState<SavedPlaylistEntry[]>([]);
  const [items, setItems] = useState<MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [nextCursor, setNextCursor] = useState<string | null>();
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [conflict, setConflict] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  useEffect(() => {
    setCursor(undefined);
    setResource(undefined);
    setPlaylistEntries([]);
    setItems([]);
    setHasMore(false);
    setNextCursor(undefined);
    setSelected([]);
    setNotice('');
    setError('');
    setConflict(false);
    setEditOpen(false);
    setDeleteOpen(false);
  }, [id, kind]);

  useEffect(() => {
    if (query.status !== 'success' || query.data.resource.id !== id || query.data.kind !== kind) return;
    const page = query.data;
    setResource(page.resource);
    setHasMore(page.hasMore);
    setNextCursor(page.nextCursor);
    if (page.kind === 'playlist') {
      setItems([]);
      setPlaylistEntries((current) => {
        const byEntryId = new Map((cursor ? current : []).map((entry) => [entry.entryId, entry]));
        for (const entry of page.entries) byEntryId.set(entry.entryId, entry);
        return [...byEntryId.values()].sort((left, right) => left.position - right.position);
      });
      return;
    }
    setPlaylistEntries([]);
    setItems((current) => {
      const byMediaId = new Map((cursor ? current : []).map((item) => [item.id, item]));
      for (const item of page.items) byMediaId.set(item.id, item);
      return [...byMediaId.values()];
    });
  }, [cursor, id, kind, query]);

  useEffect(() => {
    const validIds = new Set(kind === 'playlist' ? playlistEntries.map((entry) => entry.entryId) : items.map((item) => item.id));
    setSelected((current) => current.filter((selectedId) => validIds.has(selectedId)));
  }, [items, kind, playlistEntries]);

  const currentResource = resource?.id === id && resource.kind === kind ? resource : undefined;
  if (!currentResource) {
    if (query.status === 'error') return <div className="standard-page"><SavedState error icon={AlertTriangle} title={`Couldn’t load this ${kindLabel(kind).toLocaleLowerCase()}`} detail={reviewedProductErrorText(query.error, 'problem.request-failed')} onRetry={() => setReloadKey((value) => value + 1)} /><Link className="portico-saved-back" to="/saved"><ChevronLeft /> Back to Saved</Link></div>;
    return <div className="standard-page"><SavedState icon={RefreshCw} title={`Loading ${kindLabel(kind).toLocaleLowerCase()}`} detail="Fetching its current contents and permissions." /></div>;
  }

  const sharedResource = currentResource as SavedResourceWithSharing;
  const canManageResource = kind === 'view'
    ? currentResource.canEdit
    : canManageServer(auth.viewer?.user) || sharedResource.ownerUserId === auth.viewer?.user?.id;
  const visibleItems = kind === 'playlist' ? playlistEntries.map((entry) => entry.media) : items;
  const loadedCount = kind === 'playlist' ? playlistEntries.length : items.length;
  const refreshing = query.status === 'loading';
  const canReorder = kind === 'playlist'
    && currentResource.canEdit
    && !busy
    && !refreshing
    && !hasMore
    && playlistEntries.length === currentResource.itemCount;
  const Icon = kindIcon(kind);
  const reloadFromStart = () => {
    setCursor(undefined);
    setReloadKey((value) => value + 1);
  };
  const runMutation = async (perform: () => Promise<SavedResourceSummary>, message: string) => {
    setBusy(true);
    setError('');
    setNotice('');
    setConflict(false);
    try {
      const updated = await perform();
      setResource(updated);
      setSelected([]);
      setNotice(message);
      reloadFromStart();
    } catch (reason) {
      if (isConflict(reason)) {
        setConflict(true);
        setError('This list changed on another device. Reload the latest version before trying again.');
      } else {
        setError(reviewedProductErrorText(reason, 'problem.request-failed'));
      }
    } finally {
      setBusy(false);
    }
  };
  const runPlaylistMutation = (mutation: PlaylistItemsMutation, message: string) => runMutation(
    () => mutations.mutateItems('playlist', currentResource.id, { ...mutation, expectedUpdatedAt: currentResource.updatedAt }),
    message,
  );
  const runCollectionMutation = (mutation: CollectionMembershipMutation, message: string) => runMutation(
    () => mutations.mutateItems('collection', currentResource.id, { ...mutation, expectedUpdatedAt: currentResource.updatedAt }),
    message,
  );
  const remove = (ids: string[]) => {
    const message = `${ids.length} ${ids.length === 1 ? 'item' : 'items'} removed.`;
    if (kind === 'playlist') void runPlaylistMutation({ removeEntryIds: ids }, message);
    if (kind === 'collection') void runCollectionMutation({ removeMediaIds: ids }, message);
  };
  const move = (index: number, direction: number) => {
    if (!canReorder) return;
    const order = playlistEntries.map((entry) => entry.entryId);
    const [moving] = order.splice(index, 1);
    if (!moving) return;
    order.splice(index + direction, 0, moving);
    void runPlaylistMutation({ orderEntryIds: order }, 'Playlist order updated.');
  };
  const playAll = async () => {
    const [first, ...rest] = visibleItems;
    if (!first) return;
    setBusy(true);
    setError('');
    try {
      await playback.start(first.id, { queueMediaIds: rest.map((item) => item.id) });
      navigate(`/watch/${first.id}`);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'playback.start-failed'));
    } finally {
      setBusy(false);
    }
  };
  const toggle = (selectedId: string) => setSelected((current) => current.includes(selectedId) ? current.filter((candidate) => candidate !== selectedId) : [...current, selectedId]);
  return <div className={`standard-page portico-saved-detail ${hasMore || refreshing ? 'has-pagination' : ''}`}>
    <nav className="portico-saved-breadcrumbs"><Link to="/saved">{productMessage('navigation.saved').text}</Link><ChevronRight /><span>{kindLabel(kind)}</span></nav>
    <header className="portico-saved-detail-header">
      <span className={`portico-saved-detail-icon ${kind}`}><Icon /></span>
      <div><p>{kindLabel(kind)}</p><h1>{currentResource.title}</h1><span>{kind === 'view' ? `${currentResource.libraryName ?? 'Library'} · ${currentResource.pivot ?? 'Browse'} · ${currentResource.itemCount} results` : `${mediaItemCount(currentResource.itemCount)} · ${savedAccessSummary(sharedResource)}`}</span>{currentResource.summary && <p>{currentResource.summary}</p>}</div>
      <div className="portico-saved-detail-actions">{kind === 'playlist' && visibleItems.length > 0 && <PrimaryButton disabled={busy || refreshing} onClick={() => void playAll()}><Play /> {hasMore ? 'Play loaded' : 'Play all'}</PrimaryButton>}{canManageResource && <SecondaryButton onClick={() => setEditOpen(true)}><Pencil /> {kind === 'view' ? 'Edit' : 'Edit & share'}</SecondaryButton>}{canManageResource && <IconButton label={`Delete ${currentResource.title}`} onClick={() => setDeleteOpen(true)}><Trash2 /></IconButton>}</div>
    </header>
    {(notice || error) && <div className={`portico-saved-notice ${error ? 'error' : ''} ${conflict ? 'conflict' : ''}`} role={error ? 'alert' : 'status'}><span>{error || notice}</span>{conflict && <button type="button" onClick={() => { setConflict(false); setError(''); reloadFromStart(); }}><RefreshCw /> Reload latest</button>}</div>}
    {query.status === 'error' && <div className="portico-saved-notice error" role="alert"><span>{reviewedProductErrorText(query.error, 'problem.request-failed')}</span><button type="button" onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {productMessage('action.retry').text}</button></div>}
    {!visibleItems.length && !refreshing && <SavedState icon={Icon} title={kind === 'view' ? 'No matches right now' : `This ${kindLabel(kind).toLocaleLowerCase()} is empty`} detail={kind === 'view' ? 'The saved query is valid, but no accessible media currently matches it.' : 'Add media from a library card or its More menu.'} />}
    {selected.length > 0 && kind !== 'view' && <SavedSelectionBar count={selected.length} busy={busy} action={`Remove from ${kindLabel(kind).toLocaleLowerCase()}`} onAction={() => remove(selected)} onClear={() => setSelected([])} />}
    {kind === 'playlist' && playlistEntries.length > 0 && <PlaylistItems entries={playlistEntries} selected={selected} busy={busy || refreshing} canEdit={currentResource.canEdit} canReorder={canReorder} onToggle={toggle} onMove={move} onRemove={remove} />}
    {kind === 'collection' && items.length > 0 && <ControlledMediaGroups items={items} selected={selected} onToggle={currentResource.canEdit ? toggle : undefined} />}
    {kind === 'view' && items.length > 0 && <ReadOnlySavedItems items={items} />}
    {(hasMore || refreshing) && <div className="portico-saved-pagination" role="status">
      <span><strong>{loadedCount}</strong> of {currentResource.itemCount} loaded{kind === 'playlist' && hasMore ? ' · Load all entries to reorder.' : ''}</span>
      {(hasMore || query.status === 'error') && <SecondaryButton disabled={refreshing || !nextCursor} onClick={() => { if (query.status === 'error') setReloadKey((value) => value + 1); else if (nextCursor) setCursor(nextCursor); }}>{refreshing ? productMessage('state.loading-more').title : query.status === 'error' ? productMessage('action.retry').text : `Load ${Math.min(50, Math.max(0, currentResource.itemCount - loadedCount))} more`}</SecondaryButton>}
    </div>}
    {editOpen && canManageResource && <SavedResourceEditor kind={kind} resource={currentResource} onDismiss={() => setEditOpen(false)} onSaved={(updated) => { setEditOpen(false); setResource(updated); setNotice(`${kindLabel(kind)} updated.`); }} />}
    {deleteOpen && canManageResource && <ModalOverlay labelledBy="portico-saved-delete-title" className="portico-saved-dialog portico-saved-delete" onDismiss={() => setDeleteOpen(false)}><header><div><p>Delete {kindLabel(kind).toLocaleLowerCase()}</p><h2 id="portico-saved-delete-title">Delete “{currentResource.title}”?</h2></div><IconButton label={productMessage('action.close').text ?? ''} onClick={() => setDeleteOpen(false)}><X /></IconButton></header><p>This removes the {kindLabel(kind).toLocaleLowerCase()} only. Your media files and library items are not deleted.</p>{error && <p className="portico-saved-dialog-error" role="alert">{error}</p>}<footer><SecondaryButton onClick={() => setDeleteOpen(false)}>{productMessage('action.cancel').text}</SecondaryButton><button type="button" className="button danger" disabled={busy} onClick={() => { setBusy(true); setError(''); void mutations.delete(kind, currentResource.id).then(() => navigate('/saved'), (reason) => { setBusy(false); setError(reviewedProductErrorText(reason, 'problem.request-failed')); }); }}><Trash2 /> {busy ? 'Deleting…' : 'Delete'}</button></footer></ModalOverlay>}
  </div>;
}

export function SavedResourcePage() {
  const { kind: routeKind, id } = useParams();
  const kind = kindFromRoute(routeKind);
  if (!kind || !id) return <div className="standard-page"><SavedState error icon={AlertTriangle} title="This Saved link is invalid" detail="Return to Saved and choose an available item." /><Link className="portico-saved-back" to="/saved"><ChevronLeft /> Back to Saved</Link></div>;
  return <SavedResourceDetail kind={kind} id={id} />;
}
