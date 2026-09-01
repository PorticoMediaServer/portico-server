import { ActionConfirmIcon, ActionCloseIcon } from '#portico-icons';
import { productMessage } from '@porticomediaserver/client-core';
import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { IconButton } from '../../components/controls/Buttons';
import { StableImage } from '../../components/media/StableImage';
import { useMediaMutations } from '../../data/DataProvider';
import type { MediaItem, PlaybackStartOptions } from '../../data/models';
import {
  MediaActionMenu,
  MediaArtwork,
  MediaCard,
  MediaListRow,
  MediaRail,
  mediaDetailPath,
} from '../catalog/CatalogSurface';
import { mediaPresentation } from '../catalog/mediaPresentation';
import { MediaMetadataEditor, SavedTargetDialog } from '../media/MediaActionDialogs';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { targetFromMedia } from '../media/contextTarget';
import { playbackOptionsForItems } from '../player/watchNavigation';
import { RETAINED_RESULT_ITEM_BUDGET, retainedResultBudgetState } from './retainedResultBudget';
import type {
  BrowseExpression,
  LibraryPivotCapability,
  LibraryPivotPage,
  LibraryPresentation,
  LibraryWorkspaceLibrary,
} from './libraryTypes';

function LibrarySelectionBar({
  items,
  onClear,
  onChanged,
  onRetain,
  onCompleted,
}: {
  items: MediaItem[];
  onClear: () => void;
  onChanged: () => void;
  onRetain: (ids: string[]) => void;
  onCompleted: (message: string) => void;
}) {
  const mediaActions = useMediaMutations();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [savedTarget, setSavedTarget] = useState<'playlist' | 'collection'>();
  const [metadataOpen, setMetadataOpen] = useState(false);
  const projectedActionKey = items.length
    ? [...new Set(items[0].actions ?? [])].filter((id) => items.every((item) => item.actions?.includes(id))).sort().join('\u001f')
    : '';
  const presentedActions = useMediaActionPresentations(projectedActionKey ? projectedActionKey.split('\u001f') : []);
  const action = (...ids: string[]) => actionPresentation(presentedActions, ...ids);
  const addWatchlist = action('watchlist.add');
  const removeWatchlist = action('watchlist.remove');
  const addFavorite = action('favorite.add');
  const removeFavorite = action('favorite.remove');
  const markWatched = action('watched.mark', 'watched.set');
  const markUnwatched = action('watched.unmark', 'watched.set');
  const addPlaylist = action('playlist.add');
  const addCollection = action('collection.add');
  const editMetadata = action('metadata.edit');
  const canEditMetadata = Boolean(editMetadata) && new Set(items.map((item) => item.entityKind)).size === 1;
  const run = async (actionLabel: string, mutation: (item: MediaItem) => Promise<MediaItem>) => {
    setBusy(true);
    setNotice('');
    const results = await Promise.allSettled(items.map(mutation));
    const failedItems = results.flatMap((result, index) => result.status === 'rejected' ? [items[index]] : []);
    const failures = failedItems.length;
    setBusy(false);
    if (failures) {
      onRetain(failedItems.map((item) => item.id));
      setNotice(productMessage('media.selection-partial', { updated: items.length - failures, failed: failures }).text ?? '');
      if (items.length > failures) onChanged();
      return;
    }
    onCompleted(productMessage('media.selection-updated', { count: items.length, action: actionLabel }).text ?? '');
    onClear();
    onChanged();
  };
  const actionsAvailable = Boolean(addWatchlist || removeWatchlist || addFavorite || removeFavorite || markWatched || markUnwatched || addPlaylist || addCollection || canEditMetadata);
  return <>
    <section className="library-selection-bar" aria-label={productMessage('library.selection-label', { count: items.length }).text ?? ''}>
      <div className="library-selection-summary">
        <span>{productMessage('library.selection-count', { count: items.length }).text}</span>
        <button type="button" onClick={onClear}>{productMessage('action.clear-selection').text}</button>
        {notice && <p aria-live="polite">{notice}</p>}
        <IconButton label={productMessage('action.cancel-selection').text ?? ''} onClick={onClear}><ActionCloseIcon /></IconButton>
      </div>
      {actionsAvailable && <div className="library-selection-actions">
        {addCollection && <button type="button" disabled={busy} onClick={() => setSavedTarget('collection')}><MediaActionIcon action={addCollection} /> {addCollection.label}</button>}
        {addPlaylist && <button type="button" disabled={busy} onClick={() => setSavedTarget('playlist')}><MediaActionIcon action={addPlaylist} /> {addPlaylist.label}</button>}
        {addWatchlist && <button type="button" disabled={busy} onClick={() => void run(addWatchlist.label, (item) => mediaActions.setWatchlist(item.id, true))}><MediaActionIcon action={addWatchlist} /> {addWatchlist.label}</button>}
        {removeWatchlist && <button type="button" disabled={busy} onClick={() => void run(removeWatchlist.label, (item) => mediaActions.setWatchlist(item.id, false))}><MediaActionIcon action={removeWatchlist} /> {removeWatchlist.label}</button>}
        {addFavorite && <button type="button" disabled={busy} onClick={() => void run(addFavorite.label, (item) => mediaActions.setFavorite(item.id, true))}><MediaActionIcon action={addFavorite} /> {addFavorite.label}</button>}
        {removeFavorite && <button type="button" disabled={busy} onClick={() => void run(removeFavorite.label, (item) => mediaActions.setFavorite(item.id, false))}><MediaActionIcon action={removeFavorite} /> {removeFavorite.label}</button>}
        {markWatched && <button type="button" disabled={busy} onClick={() => void run(markWatched.label, (item) => mediaActions.setWatched(item.id, true))}><MediaActionIcon action={markWatched} /> {markWatched.label}</button>}
        {markUnwatched && <button type="button" disabled={busy} onClick={() => void run(markUnwatched.label, (item) => mediaActions.setWatched(item.id, false))}><MediaActionIcon action={markUnwatched} /> {markUnwatched.label}</button>}
        {canEditMetadata && editMetadata && <button type="button" disabled={busy} onClick={() => setMetadataOpen(true)}><MediaActionIcon action={editMetadata} /> {editMetadata.label}</button>}
      </div>}
    </section>
    {savedTarget && <SavedTargetDialog kind={savedTarget} mediaIds={items.map((item) => item.id)} onDismiss={() => setSavedTarget(undefined)} />}
    {metadataOpen && <MediaMetadataEditor mediaIds={items.map((item) => item.id)} initialItems={items} onDismiss={() => setMetadataOpen(false)} onSaved={onChanged} />}
  </>;
}

function LibraryGrid({
  items,
  square,
  compact,
  selected,
  playbackOptions,
  onToggle,
}: {
  items: MediaItem[];
  square: boolean;
  compact: boolean;
  selected: string[];
  playbackOptions?: PlaybackStartOptions;
  onToggle: (id: string) => void;
}) {
  return <div className={`library-media-grid ${compact ? 'compact' : ''} ${square ? 'square' : 'poster'}`} data-retained-result-count={items.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(items.length)}>
    {items.map((item) => <div id={`library-item-${item.id}`} data-library-title={item.sortTitle || item.title} key={item.id}>
      <MediaCard
        item={item}
        shape={square ? 'square' : undefined}
        playbackOptions={playbackOptions}
        selected={selected.includes(item.id)}
        onSelect={item.actions?.length ? () => onToggle(item.id) : undefined}
      />
    </div>)}
  </div>;
}

function LibraryList({ items, selected, playbackOptions, onToggle }: { items: MediaItem[]; selected: string[]; playbackOptions?: PlaybackStartOptions; onToggle: (id: string) => void }) {
  return <div className="library-media-list" data-retained-result-count={items.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(items.length)}>
    {items.map((item) => <div id={`library-item-${item.id}`} data-library-title={item.sortTitle || item.title} key={item.id}>
      <MediaListRow item={item} playbackOptions={playbackOptions} selected={selected.includes(item.id)} onSelect={item.actions?.length ? () => onToggle(item.id) : undefined} />
    </div>)}
  </div>;
}

function LibraryTable({ items, selected, playbackOptions, onToggle }: { items: MediaItem[]; selected: string[]; playbackOptions?: PlaybackStartOptions; onToggle: (id: string) => void }) {
  return <div className="library-table-scroll" tabIndex={0} aria-label={productMessage('library.technical-table-label').text} data-retained-result-count={items.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(items.length)}>
    <div className="library-table" role="table">
      <div className="library-table-head" role="row">
        <span role="columnheader">{productMessage('library.column-select').text}</span><span role="columnheader">{productMessage('library.column-title').text}</span><span role="columnheader">{productMessage('library.column-year').text}</span><span role="columnheader">{productMessage('library.column-duration').text}</span><span role="columnheader">{productMessage('library.column-availability').text}</span><span role="columnheader">{productMessage('library.column-actions').text}</span>
      </div>
      {items.map((item) => {
        const detailPath = mediaDetailPath(item);
        const selectedItem = selected.includes(item.id);
        return <div id={`library-item-${item.id}`} data-library-title={item.sortTitle || item.title} className={`library-table-row ${selectedItem ? 'selected' : ''}`} role="row" key={item.id}>
          <span role="cell">{item.actions?.length ? <button type="button" className={`selection-check table-check ${selectedItem ? 'selected' : ''}`} onClick={() => onToggle(item.id)} aria-label={productMessage(selectedItem ? 'library.deselect-item' : 'library.select-item', { title: item.title }).text} aria-pressed={selectedItem}>{selectedItem && <ActionConfirmIcon />}</button> : null}</span>
          <span role="cell" className="library-table-title"><span className="library-table-art"><MediaArtwork item={item} shape={mediaPresentation(item).artworkShape} /></span>{detailPath ? <Link to={detailPath}><strong>{item.title}</strong><span>{item.parentTitle || item.subtitle}</span></Link> : <span><strong>{item.title}</strong><span>{item.parentTitle || item.subtitle}</span></span>}</span>
          <span role="cell">{item.year || '—'}</span>
          <span role="cell">{item.length || '—'}</span>
          <span role="cell" className={`library-availability ${item.availability ?? 'available'}`}>{formatCapabilityLabel(item.availability ?? 'available')}</span>
          <span role="cell"><MediaActionMenu target={targetFromMedia(item)} playbackOptions={playbackOptions} /></span>
        </div>;
      })}
    </div>
  </div>;
}

function FacetResults({ page, onApply }: { page: LibraryPivotPage; onApply: (query: BrowseExpression, pivotId?: string) => void }) {
  if (!page.facets?.length) return null;
  return <div className="library-facet-grid" data-retained-result-count={page.facets.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(page.facets.length)}>
    {page.facets.map((facet) => <button type="button" key={facet.id} onClick={() => onApply(facet.query, facet.pivotId)}>
      <StableImage src={facet.artwork} alt="" loading="lazy" fallback={<span className="library-facet-mark">{facet.title.slice(0, 2).toLocaleUpperCase()}</span>} />
      <span><strong>{facet.title}</strong><span>{facet.detail || productMessage(facet.count === 1 ? 'media.item-count-single' : 'media.item-count', { count: facet.count }).text}</span></span>
    </button>)}
  </div>;
}

function ResourceResult({ item }: { item: MediaItem }) {
  const actions = useMediaActionPresentations(item.actions ?? []);
  const play = actionPresentation(actions, 'play');
  const resourceKind = item.entityKind === 'playlist' ? 'playlist' : 'collection';
  return <Link to={`/saved/${resourceKind}s/${item.id}`}>
    <span className="library-resource-art"><MediaArtwork item={item} /></span>
    <span><strong>{item.title}</strong><span>{item.summary || item.subtitle}</span></span>
    {play && <MediaActionIcon action={play} />}
  </Link>;
}

function ResourceResults({ items }: { items: MediaItem[] }) {
  return <div className="library-resource-list" data-retained-result-count={items.length} data-retained-result-budget={RETAINED_RESULT_ITEM_BUDGET} data-retained-result-budget-state={retainedResultBudgetState(items.length)}>
    {items.map((item) => <ResourceResult item={item} key={item.id} />)}
  </div>;
}

function formatCapabilityLabel(value: string) {
  const canonical = {
    available: productMessage('media.available').text,
    partial: productMessage('media.partially-available').text,
    unavailable: productMessage('media.unavailable').text,
  }[value];
  if (canonical) return canonical;
  return value.replaceAll('-', ' ').replace(/^./, (letter) => letter.toLocaleUpperCase());
}

export function LibraryResults({
  library,
  pivot,
  page,
  presentation,
  onApplyFacet,
  onChanged,
}: {
  library: LibraryWorkspaceLibrary;
  pivot: LibraryPivotCapability;
  page: LibraryPivotPage;
  presentation: LibraryPresentation;
  onApplyFacet: (query: BrowseExpression, pivotId?: string) => void;
  onChanged: () => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const [operationNotice, setOperationNotice] = useState('');
  const visibleItems = useMemo(() => page.sections?.length ? page.sections.flatMap((section) => section.items) : page.items, [page.items, page.sections]);
  const selectedItems = visibleItems.filter((item) => selected.includes(item.id));
  const pivotShapes = new Set(pivot.entityKinds.map((entityKind) => mediaPresentation({ entityKind }).artworkShape));
  const square = pivotShapes.size === 1 && pivotShapes.has('square');
  const playbackContext = { type: 'library' as const, id: library.id, title: `${library.name} · ${pivot.label}` };
  const playbackOptions = playbackOptionsForItems(visibleItems, playbackContext);
  const toggle = (id: string) => {
    setOperationNotice('');
    setSelected((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id]);
  };
  useEffect(() => setSelected((current) => current.filter((id) => visibleItems.some((item) => item.id === id))), [visibleItems]);

  let content;
  if (presentation === 'shelves' && page.sections?.length) {
    content = <div className="library-discover-sections">{page.sections.map((section) => (
      <MediaRail
        key={section.id}
        title={section.title}
        detail={section.detail}
        items={section.items}
        shape={square ? 'square' : undefined}
        playbackContext={{ type: 'library', id: library.id, title: `${library.name} · ${section.title}` }}
      />
    ))}</div>;
  } else if (presentation === 'facets' && page.facets?.length) {
    content = <FacetResults page={page} onApply={onApplyFacet} />;
  } else if (pivot.entityKinds.every((kind) => kind === 'collection' || kind === 'playlist')) {
    content = <ResourceResults items={page.items} />;
  } else if (presentation === 'list') {
    content = <LibraryList items={page.items} selected={selected} playbackOptions={playbackOptions} onToggle={toggle} />;
  } else if (presentation === 'table') {
    content = <LibraryTable items={page.items} selected={selected} playbackOptions={playbackOptions} onToggle={toggle} />;
  } else {
    content = <LibraryGrid items={page.items} square={square} compact={presentation === 'compact-grid'} selected={selected} playbackOptions={playbackOptions} onToggle={toggle} />;
  }

  return <>
    {content}
    {operationNotice && <div className="library-saved-notice" role="status"><ActionConfirmIcon /><span>{operationNotice}</span><IconButton label={productMessage('action.dismiss-status').text ?? ''} onClick={() => setOperationNotice('')}><ActionCloseIcon /></IconButton></div>}
    {selectedItems.length > 0 && <LibrarySelectionBar items={selectedItems} onClear={() => setSelected([])} onChanged={onChanged} onRetain={setSelected} onCompleted={setOperationNotice} />}
  </>;
}
