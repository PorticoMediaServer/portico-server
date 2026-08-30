import { StatusWarningIcon, ActionConfirmIcon, ActionRefreshIcon, NavigationSearchIcon, AccountProfilesIcon, ActionCloseIcon } from '#portico-icons';
import { useEffect, useMemo, useState } from 'react';
import { IconButton } from '../../components/controls/Buttons';
import { productProblemText } from '../../components/ProductLanguage';
import { SelectMenu } from '../../components/controls/SelectMenu';
import { usePorticoDataSource } from '../../data/DataProvider';
import type { SavedResourceShare, SavedShareCandidate, SavedShareCandidatePage } from '../../data/models';

export type SavedShareEditorValue = SavedResourceShare;

type CandidateState =
  | { status: 'loading' }
  | { status: 'success'; data: SavedShareCandidatePage }
  | { status: 'error'; error: Error };

const candidateLimit = 20;
const maximumQueryLength = 80;

function useDebouncedValue(value: string, delay: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}

function useSavedShareCandidates(query: string, reloadKey: number): CandidateState {
  const source = usePorticoDataSource();
  const normalizedQuery = query.trim().slice(0, maximumQueryLength);
  const [state, setState] = useState<CandidateState>({ status: 'loading' });
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: 'loading' });
    source.savedShareCandidates(normalizedQuery, candidateLimit, controller.signal).then(
      (data) => !controller.signal.aborted && setState({ status: 'success', data }),
      (reason: unknown) => {
        if (controller.signal.aborted) return;
        setState({ status: 'error', error: reason instanceof Error ? reason : new Error('People on this server could not be loaded.') });
      },
    );
    return () => controller.abort();
  }, [normalizedQuery, reloadKey, source]);
  return state;
}

export function SavedShareEditor({ shares, visibility, onChange }: {
  shares: SavedShareEditorValue[];
  visibility: 'private' | 'server';
  onChange: (shares: SavedShareEditorValue[]) => void;
}) {
  const [query, setQuery] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const debouncedQuery = useDebouncedValue(query, 280);
  const candidates = useSavedShareCandidates(debouncedQuery, reloadKey);
  const sharedIds = useMemo(() => new Set(shares.map((share) => share.userId)), [shares]);
  const availableCandidates = candidates.status === 'success'
    ? candidates.data.items.filter((candidate) => !sharedIds.has(candidate.userId))
    : [];
  const updateAccess = (userId: string, canEdit: boolean) => onChange(shares.map((share) => share.userId === userId ? { ...share, canEdit } : share));
  const remove = (userId: string) => onChange(shares.filter((share) => share.userId !== userId));
  const add = (candidate: SavedShareCandidate) => {
    if (sharedIds.has(candidate.userId)) return;
    onChange([...shares, { ...candidate, canEdit: false }]);
  };

  return <section className="portico-saved-sharing" aria-labelledby="portico-saved-sharing-title">
    <header>
      <div>
        <strong id="portico-saved-sharing-title">People with access</strong>
        <p>{visibility === 'server'
          ? 'Everyone on this server can view. Named access below can also grant editing.'
          : 'Only you and the people listed below can open this item.'}</p>
      </div>
      <span>{shares.length} {shares.length === 1 ? 'person' : 'people'}</span>
    </header>

    {shares.length > 0
      ? <div className="portico-saved-share-list">{shares.map((share) => <div className="portico-saved-share-row" key={share.userId}>
        <span className="portico-saved-share-avatar" aria-hidden="true">{share.displayName.trim().charAt(0).toLocaleUpperCase() || '?'}</span>
        <strong>{share.displayName}</strong>
        <SelectMenu
          label={`${share.displayName} access`}
          value={share.canEdit ? 'edit' : 'view'}
          options={[{ id: 'view', label: 'Can view' }, { id: 'edit', label: 'Can edit' }]}
          onChange={(value) => updateAccess(share.userId, value === 'edit')}
        />
        <IconButton label={`Remove ${share.displayName}`} onClick={() => remove(share.userId)}><ActionCloseIcon /></IconButton>
      </div>)}</div>
      : <div className="portico-saved-share-empty"><AccountProfilesIcon /><span>No named access</span><p>Add someone below without sharing this with the whole server.</p></div>}

    <div className="portico-saved-share-discovery">
      <label htmlFor="portico-saved-share-search">Add someone</label>
      <div className="portico-saved-share-search"><NavigationSearchIcon /><input
        id="portico-saved-share-search"
        type="search"
        value={query}
        maxLength={maximumQueryLength}
        placeholder="Search by username"
        autoComplete="off"
        onChange={(event) => setQuery(event.target.value.slice(0, maximumQueryLength))}
      />{query && <IconButton label="Clear search" onClick={() => setQuery('')}><ActionCloseIcon /></IconButton>}</div>

      {candidates.status === 'loading' && <div className="portico-saved-share-state" role="status"><ActionRefreshIcon className="state-spinner" /><span>Finding people…</span></div>}
      {candidates.status === 'error' && <div className="portico-saved-share-state error" role="alert"><StatusWarningIcon /><span>{productProblemText(candidates.error)}</span><button type="button" onClick={() => setReloadKey((value) => value + 1)}>Try again</button></div>}
      {candidates.status === 'success' && availableCandidates.length > 0 && <div className="portico-saved-candidate-list">{availableCandidates.map((candidate) => <button type="button" key={candidate.userId} onClick={() => add(candidate)}>
        <span className="portico-saved-share-avatar" aria-hidden="true">{candidate.displayName.trim().charAt(0).toLocaleUpperCase() || '?'}</span>
        <strong>{candidate.displayName}</strong>
        <span><AccountProfilesIcon /> Add</span>
      </button>)}</div>}
      {candidates.status === 'success' && availableCandidates.length === 0 && <div className="portico-saved-share-state empty" role="status"><ActionConfirmIcon /><span>{query.trim() ? 'No other people match this name.' : 'No other active members are available.'}</span></div>}
      {candidates.status === 'success' && candidates.data.hasMore && <div className="portico-saved-share-more"><span>More people match this search. Enter more of the name to narrow the list.</span></div>}
    </div>
  </section>;
}
