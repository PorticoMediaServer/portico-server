import { type PorticoSemanticIconComponent, StatusWarningIcon, NavigationMoveDownIcon, NavigationMoveUpIcon, NavigationLibraryIcon, NavigationDisclosureIcon, MediaMovieIcon, MediaMusicIcon, ActionMoreIcon, ActionPinIcon, ActionUnpinIcon, ActionAddIcon, NavigationChannelsIcon, ActionRefreshIcon, NavigationSearchIcon, DeviceTvIcon, ActionCloseIcon } from '#portico-icons';
import { useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { productProblemText, productText } from '../../components/ProductLanguage';
import { AnchoredOverlay } from '../../components/overlay/OverlayPortal';
import { useAuthSession, useLibraries } from '../../data/DataProvider';
import { canManageLibraries as viewerCanManageLibraries } from '../../data/authority';
import type { LibrarySummary } from '../../data/models';
import { useOptionalWebDisplayPreferences } from '../../preferences/WebDisplayPreferencesProvider';
import './libraries.css';

function libraryPresentation(kind: string): { label: string; icon: PorticoSemanticIconComponent; tone: string } {
  switch (kind) {
    case 'movies':
    case 'movie':
      return { label: 'Movies', icon: MediaMovieIcon, tone: 'movies' };
    case 'music':
      return { label: 'Music', icon: MediaMusicIcon, tone: 'music' };
    case 'audiobook':
    case 'audiobooks':
      return { label: 'Audiobooks', icon: NavigationLibraryIcon, tone: 'audiobooks' };
    case 'recorded-tv':
    case 'recorded_tv':
      return { label: 'Recorded TV', icon: NavigationChannelsIcon, tone: 'recorded-tv' };
    case 'anime':
      return { label: 'Anime', icon: DeviceTvIcon, tone: 'anime' };
    default:
      return { label: 'TV Shows', icon: DeviceTvIcon, tone: 'television' };
  }
}

function libraryDestination(library: LibrarySummary): string {
  return `/library/${encodeURIComponent(library.id)}`;
}

function itemCount(library: LibrarySummary): string {
  return `${library.itemCount.toLocaleString()} ${library.itemCount === 1 ? 'item' : 'items'}`;
}

function LibraryEntry({ library, pinned, firstPinned, lastPinned, busy, onAction }: {
  library: LibrarySummary;
  pinned: boolean;
  firstPinned: boolean;
  lastPinned: boolean;
  busy: boolean;
  onAction: (action: 'pin' | 'unpin' | 'up' | 'down') => void;
}) {
  const presentation = libraryPresentation(library.kind);
  const Icon = presentation.icon;
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return <div className="library-hub-entry">
    <Link className="library-hub-entry-link" to={libraryDestination(library)}>
      <span className={`library-hub-icon ${presentation.tone}`} aria-hidden="true"><Icon /></span>
      <span className="library-hub-entry-copy">
        <span className="library-hub-entry-kicker">{presentation.label}</span>
        <strong>{library.name}</strong>
        <small>{itemCount(library)}</small>
      </span>
      {pinned && <span className="library-hub-pinned"><ActionPinIcon /> {productText('preferences.library-status-pinned')}</span>}
      <NavigationDisclosureIcon className="library-hub-chevron" aria-hidden="true" />
    </Link>
    <button ref={trigger} className="library-hub-more" type="button" disabled={busy} aria-label={`Library options for ${library.name}`} aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((value) => !value)}><ActionMoreIcon /></button>
    {open && <AnchoredOverlay anchorRef={trigger} placement="bottom-end" className="library-hub-menu" role="menu" onDismiss={() => setOpen(false)}>
      <Link role="menuitem" to={libraryDestination(library)}><NavigationDisclosureIcon /> Open library</Link>
      {pinned
        ? <button role="menuitem" onClick={() => { onAction('unpin'); setOpen(false); }}><ActionUnpinIcon /> Unpin from sidebar</button>
        : <button role="menuitem" onClick={() => { onAction('pin'); setOpen(false); }}><ActionPinIcon /> Pin to sidebar</button>}
      {pinned && <button role="menuitem" disabled={firstPinned} onClick={() => { onAction('up'); setOpen(false); }}><NavigationMoveUpIcon /> Move up</button>}
      {pinned && <button role="menuitem" disabled={lastPinned} onClick={() => { onAction('down'); setOpen(false); }}><NavigationMoveDownIcon /> Move down</button>}
    </AnchoredOverlay>}
  </div>;
}

export function LibrariesHubPage() {
  const auth = useAuthSession();
  const display = useOptionalWebDisplayPreferences();
  const [reloadKey, setReloadKey] = useState(0);
  const [query, setQuery] = useState('');
  const libraries = useLibraries(reloadKey);
  const viewer = auth.viewer;
  const canManageLibraries = viewerCanManageLibraries(viewer?.user);
  const storedSidebarOrder = viewer?.user?.preferences?.sidebarOrder ?? [];
  const pinnedLibraryIds = display
    ? display.preferences.pinnedLibraryIds
    : storedSidebarOrder.flatMap((entry) => entry.startsWith('library:') ? [entry.slice('library:'.length)] : []);
  const pinnedIds = useMemo(() => new Set(pinnedLibraryIds), [pinnedLibraryIds]);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const allLibraries = libraries.status === 'success' ? libraries.data : [];
  const visibleLibraries = useMemo(() => allLibraries
    .filter((library) => !normalizedQuery || `${library.name} ${libraryPresentation(library.kind).label}`.toLocaleLowerCase().includes(normalizedQuery))
    .sort((left, right) => {
      const leftRank = pinnedLibraryIds.indexOf(left.id);
      const rightRank = pinnedLibraryIds.indexOf(right.id);
      if (leftRank >= 0 || rightRank >= 0) {
        if (leftRank < 0) return 1;
        if (rightRank < 0) return -1;
        return leftRank - rightRank;
      }
      return left.name.localeCompare(right.name);
    }), [allLibraries, normalizedQuery, pinnedLibraryIds]);
  const totalItems = allLibraries.reduce((total, library) => total + library.itemCount, 0);
  const pinnedCount = allLibraries.filter((library) => pinnedIds.has(library.id)).length;
  const changeLibraryPin = (libraryId: string, action: 'pin' | 'unpin' | 'up' | 'down') => {
    const next = [...pinnedLibraryIds];
    const index = next.indexOf(libraryId);
    if (action === 'pin') {
      if (index < 0) next.push(libraryId);
    } else if (action === 'unpin') {
      if (index >= 0) next.splice(index, 1);
    } else if (index >= 0) {
      const target = action === 'up' ? index - 1 : index + 1;
      if (target < 0 || target >= next.length) return;
      [next[index], next[target]] = [next[target], next[index]];
    }
    void display?.patch({ pinnedLibraryIds: next });
  };

  return <div className="standard-page libraries-hub-page">
    <header className="libraries-hub-header">
      <div>
        <p className="route-context">{viewer?.serverName ?? 'Portico Server'}</p>
        <h1>Libraries</h1>
        <p>Browse every media library available to this account.</p>
      </div>
      {canManageLibraries && <Link className="button secondary" to="/settings/media"><ActionAddIcon /> Manage libraries</Link>}
    </header>

    {libraries.status === 'success' && allLibraries.length > 0 && <div className="library-hub-summary" aria-label="Library summary">
      <span><strong>{allLibraries.length.toLocaleString()}</strong> {allLibraries.length === 1 ? 'library' : 'libraries'}</span>
      <span><strong>{totalItems.toLocaleString()}</strong> {totalItems === 1 ? 'item' : 'items'}</span>
      {pinnedCount > 0 && <span><strong>{pinnedCount.toLocaleString()}</strong> pinned</span>}
    </div>}

    {libraries.status === 'success' && allLibraries.length > 0 && <div className="library-hub-toolbar">
      <label className="library-hub-search">
        <NavigationSearchIcon aria-hidden="true" />
        <input aria-label="Filter libraries" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter libraries" />
        {query && <button type="button" onClick={() => setQuery('')} aria-label="Clear library filter"><ActionCloseIcon /></button>}
      </label>
      <span>{visibleLibraries.length === allLibraries.length ? 'All libraries' : `${visibleLibraries.length} matching`}</span>
    </div>}

    {libraries.status === 'loading' && <div className="library-hub-reservation" aria-busy="true" aria-label="Loading libraries"><div aria-hidden="true" /><div aria-hidden="true" /><div aria-hidden="true" /></div>}

    {libraries.status === 'error' && <div className="library-state error" role="alert"><StatusWarningIcon /><strong>Couldn’t load libraries</strong><p>{productProblemText(libraries.error, 'library.load-failed')}</p><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><ActionRefreshIcon /> {productText('action.retry')}</SecondaryButton></div>}

    {libraries.status === 'success' && allLibraries.length === 0 && (canManageLibraries
      ? <div className="library-state library-hub-first-library"><NavigationLibraryIcon /><strong>Add your first library</strong><p>Choose a media type and one or more folders on this server. Portico keeps the source files in place and queues the first scan.</p><Link className="button primary" to="/settings/media?newLibrary=1"><ActionAddIcon /> Add first library</Link></div>
      : <div className="library-state"><NavigationLibraryIcon /><strong>This server has no available libraries</strong><p>The server owner has not made a media library available to this account.</p></div>)}

    {libraries.status === 'success' && allLibraries.length > 0 && visibleLibraries.length === 0 && <div className="library-state"><NavigationSearchIcon /><strong>No libraries match “{query.trim()}”</strong><p>Try a different library name or media type.</p><SecondaryButton onClick={() => setQuery('')}>Clear filter</SecondaryButton></div>}

    {libraries.status === 'success' && visibleLibraries.length > 0 && <section className="library-hub-directory" aria-label="Available libraries">
      {visibleLibraries.map((library) => {
        const pinnedIndex = pinnedLibraryIds.indexOf(library.id);
        return <LibraryEntry key={library.id} library={library} pinned={pinnedIndex >= 0} firstPinned={pinnedIndex === 0} lastPinned={pinnedIndex === pinnedLibraryIds.length - 1} busy={display?.busy ?? false} onAction={(action) => changeLibraryPin(library.id, action)} />;
      })}
    </section>}
  </div>;
}
