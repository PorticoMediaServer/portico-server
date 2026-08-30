import { StatusWarningIcon, ActionConfirmIcon, MediaMovieIcon, StatusLoadingIcon, NavigationSearchIcon, ActionCloseIcon } from '#portico-icons';
import { useEffect, useMemo, useState } from 'react';
import { useMediaDetail, useSearchPage } from '../../data/DataProvider';
import type { MediaItem } from '../../data/models';

function isPlayable(item: MediaItem) {
  return item.actions?.includes('play') === true;
}

function resultDetail(item: MediaItem) {
  return item.subtitle || [item.entityKind.replaceAll('_', ' '), item.year || undefined].filter(Boolean).join(' · ');
}

function ResultArtwork({ item }: { item: MediaItem }) {
  return item.poster ? <img src={item.poster} alt="" /> : <span><MediaMovieIcon /></span>;
}

export function MediaSearchPicker({
  value,
  onChange,
  autoFocus = false,
  label = 'Play together',
  placeholder = 'Search movies, shows, episodes, or music',
  inputLabel = 'Search media for the group',
  compact = false,
  disabled = false,
}: {
  value: string;
  onChange: (mediaId: string) => void;
  autoFocus?: boolean;
  label?: string;
  placeholder?: string;
  inputLabel?: string;
  compact?: boolean;
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState('');
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState(!value);
  const [selectedOverride, setSelectedOverride] = useState<MediaItem>();
  const detail = useMediaDetail(value || undefined);
  const search = useSearchPage({ query, limit: 24 });

  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(draft.trim().length >= 2 ? draft.trim() : ''), 220);
    return () => window.clearTimeout(timer);
  }, [draft]);

  useEffect(() => {
    if (!value) setEditing(true);
  }, [value]);

  const selected = selectedOverride?.id === value ? selectedOverride : detail.status === 'success' ? detail.data : undefined;
  const results = useMemo(() => {
    if (search.status !== 'success') return [];
    const known = new Set<string>();
    return search.data.groups.flatMap((group) => group.items).filter((item) => {
      if (!isPlayable(item) || known.has(item.id)) return false;
      known.add(item.id);
      return true;
    }).slice(0, 8);
  }, [search]);

  const choose = (item: MediaItem) => {
    setSelectedOverride(item);
    onChange(item.id);
    setDraft('');
    setQuery('');
    setEditing(false);
  };

  return <div className={`watch-media-picker${compact ? ' compact' : ''}`}>
    <span className="watch-media-picker-label">{label}</span>
    {value && !editing && <div className="watch-media-selection">
      <span className="watch-media-selection-art">{selected ? <ResultArtwork item={selected} /> : detail.status === 'loading' ? <StatusLoadingIcon className="watch-spin" /> : <MediaMovieIcon />}</span>
      <span>{selected ? <><strong>{selected.title}</strong><small>{resultDetail(selected)}</small></> : <><strong>{detail.status === 'error' ? 'Selected media is unavailable' : 'Loading selection…'}</strong><small>{detail.status === 'error' ? 'Choose another title to continue.' : 'Reading the current media details.'}</small></>}</span>
      <button type="button" disabled={disabled} onClick={() => setEditing(true)}>Change</button>
    </div>}
    {(!value || editing) && <div className="watch-media-search">
      <label><NavigationSearchIcon /><input autoFocus={autoFocus} disabled={disabled} value={draft} onChange={(event) => setDraft(event.target.value)} placeholder={placeholder} aria-label={inputLabel} />{draft && <button type="button" disabled={disabled} aria-label="Clear media search" onClick={() => { setDraft(''); setQuery(''); }}><ActionCloseIcon /></button>}</label>
      {draft.trim().length > 0 && draft.trim().length < 2 && <p className="watch-media-search-hint">Enter at least two characters.</p>}
      {query && <div className="watch-media-results" role="listbox" aria-label="Playable media results">
        {search.status === 'loading' && <div className="watch-media-result-state"><StatusLoadingIcon className="watch-spin" /> Searching your server…</div>}
        {search.status === 'error' && <div className="watch-media-result-state error"><StatusWarningIcon /> Media search is unavailable. Try again.</div>}
        {search.status === 'success' && results.length === 0 && <div className="watch-media-result-state">No playable media found for “{query}”.</div>}
        {results.map((item) => <button type="button" role="option" aria-selected={item.id === value} key={item.id} onClick={() => choose(item)}>
          <span className="watch-media-result-art"><ResultArtwork item={item} /></span>
          <span><strong>{item.title}</strong><small>{resultDetail(item)}</small></span>
          {item.id === value && <ActionConfirmIcon />}
        </button>)}
      </div>}
      {value && <button type="button" className="watch-media-search-cancel" disabled={disabled} onClick={() => { setEditing(false); setDraft(''); setQuery(''); }}>Keep current selection</button>}
    </div>}
  </div>;
}
