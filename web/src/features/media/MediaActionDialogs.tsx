import { Check, CircleCheck, FolderHeart, ListMusic, Lock, LockOpen, Plus, RefreshCw, ScanSearch, Search, X } from '#portico-icons';
import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { useMediaDetail, useMediaMutations, useMediaOperations, useSavedMutations, useSavedResources } from '../../data/DataProvider';
import type { MediaItem, MediaMatchCandidate, MediaMetadataUpdate, MediaPerson, SavedResourceKind } from '../../data/models';
import { ArtworkEditor } from './ArtworkEditor';
import { LyricsEditor } from './LyricsEditor';
import { TechnicalMediaEditor } from './TechnicalMediaEditor';
import './media-actions.css';

type ListResourceKind = Exclude<SavedResourceKind, 'view'>;

function resourceLabel(kind: ListResourceKind) {
  return kind === 'playlist' ? 'playlist' : 'collection';
}

const mediaKindLabels: Record<string, string> = {
  movie: 'Movie', show: 'Show', season: 'Season', episode: 'Episode', special: 'Special',
  artist: 'Artist', album: 'Album', track: 'Track', author: 'Author',
  'audiobook-series': 'Audiobook series', book: 'Audiobook', chapter: 'Chapter',
  recording: 'Recording', 'live-channel': 'Live channel', 'live-program': 'Live program',
  person: 'Person', collection: 'Collection', playlist: 'Playlist', category: 'Category',
  extra: 'Extra', unsupported: 'Unsupported media',
};

function mediaKindLabel(kind: string | undefined) {
  return mediaKindLabels[kind ?? ''] ?? 'Unsupported media';
}

const catalogEvidenceKinds = new Set([
  'alternateTitle', 'originalLanguage', 'spokenLanguage', 'status', 'contentRating',
  'studio', 'company', 'network', 'country', 'keyword', 'collection', 'franchise',
  'creator', 'person', 'author', 'narrator', 'series', 'genre', 'tag',
]);

function metadataEvidenceValue(value: unknown) {
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}

export function SavedTargetDialog({ kind, mediaIds, onDismiss }: { kind: ListResourceKind; mediaIds: string[]; onDismiss: () => void }) {
  const [reloadKey, setReloadKey] = useState(0);
  const resources = useSavedResources(kind, reloadKey);
  const mutations = useSavedMutations();
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState('');
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [complete, setComplete] = useState('');
  const label = resourceLabel(kind);
  const headingId = `add-to-${kind}-title`;
  const add = async (id: string, name: string, updatedAt: string) => {
    setBusy(id); setError('');
    try {
      await mutations.mutateItems(kind, id, { addMediaIds: mediaIds, expectedUpdatedAt: updatedAt });
      setComplete(`Added ${mediaIds.length === 1 ? 'this item' : `${mediaIds.length} items`} to ${name}.`);
      setReloadKey((value) => value + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'catalog.action-failed', { actionName: `update this ${label}` }));
    } finally { setBusy(''); }
  };
  const create = async () => {
    if (!title.trim()) { setError(`Enter a ${label} name.`); return; }
    setBusy('new'); setError('');
    try {
      const resource = await mutations.create(kind, { title: title.trim(), visibility: 'private', mediaIds });
      setComplete(`Created ${resource.title} with ${mediaIds.length} ${mediaIds.length === 1 ? 'item' : 'items'}.`);
      setTitle(''); setCreating(false); setReloadKey((value) => value + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'catalog.action-failed', { actionName: `create this ${label}` }));
    } finally { setBusy(''); }
  };
  const Icon = kind === 'playlist' ? ListMusic : FolderHeart;
  return <ModalOverlay labelledBy={headingId} className="saved-target-dialog" onDismiss={onDismiss}>
    <header><div><p>{mediaIds.length === 1 ? 'Save item' : `Save ${mediaIds.length} items`}</p><h2 id={headingId}>Add to {label}</h2></div><IconButton label="Close" onClick={onDismiss}><X /></IconButton></header>
    {complete ? <div className="saved-target-complete" role="status"><CircleCheck /><strong>{complete}</strong><p>You can keep this dialog open to add the same selection somewhere else.</p></div> : null}
    <div className="saved-target-body">
      {resources.status === 'loading' && <div className="saved-target-state" aria-busy="true"><RefreshCw className="state-spinner" /> Loading {label}s</div>}
      {resources.status === 'error' && <div className="saved-target-state error" role="alert">{reviewedProductErrorText(resources.error, 'media.load-failed', { featureName: 'Saved lists' })}</div>}
      {resources.status === 'success' && resources.data.length > 0 && <div className="saved-target-list">{resources.data.map((resource) => <button type="button" disabled={Boolean(busy)} key={resource.id} onClick={() => void add(resource.id, resource.title, resource.updatedAt)}><span><Icon /></span><span><strong>{resource.title}</strong><small>{resource.itemCount} {resource.itemCount === 1 ? 'item' : 'items'} · {resource.visibility === 'server' ? 'Shared' : 'Private'}</small></span>{busy === resource.id ? <RefreshCw className="state-spinner" /> : <Plus />}</button>)}</div>}
      {resources.status === 'success' && !resources.data.length && !creating && <div className="saved-target-state"><Icon /> No {label}s yet</div>}
      {creating && <form className="saved-target-create" onSubmit={(event) => { event.preventDefault(); void create(); }}><label><span>Name</span><input autoFocus value={title} onChange={(event) => setTitle(event.target.value)} maxLength={160} /></label><div><SecondaryButton onClick={() => { setCreating(false); setTitle(''); setError(''); }}>Cancel</SecondaryButton><PrimaryButton type="submit" disabled={busy === 'new'}>{busy === 'new' ? 'Creating…' : `Create ${label}`}</PrimaryButton></div></form>}
      {error && <p className="context-action-error" role="alert">{error}</p>}
    </div>
    <footer>{!creating && <SecondaryButton onClick={() => setCreating(true)}><Plus /> New {label}</SecondaryButton>}<PrimaryButton onClick={onDismiss}>{complete ? 'Done' : 'Close'}</PrimaryButton></footer>
  </ModalOverlay>;
}

function parseList(value: string | undefined) {
  return (value ?? '').split(',').map((entry) => entry.trim()).filter(Boolean);
}

function TagEditor({ action, label, values, onChange }: { action?: ReactNode; label: string; values: string[]; onChange: (values: string[]) => void }) {
  const [draft, setDraft] = useState('');
  const add = () => {
    const value = draft.trim();
    if (value && !values.some((item) => item.toLocaleLowerCase() === value.toLocaleLowerCase())) onChange([...values, value]);
    setDraft('');
  };
  return <div className="tag-editor"><span>{label}{action}</span><div>{values.map((value) => <button type="button" key={value} onClick={() => onChange(values.filter((item) => item !== value))}>{value}<X /></button>)}<input value={draft} onChange={(event) => setDraft(event.target.value)} onBlur={add} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ',') { event.preventDefault(); add(); } }} placeholder={`Add ${label.toLocaleLowerCase()}`} /></div></div>;
}

function PeopleEditor({ people, action, onChange }: { people: MediaPerson[]; action: ReactNode; onChange: (people: MediaPerson[]) => void }) {
  const update = (index: number, patch: Partial<MediaPerson>) => onChange(people.map((person, candidate) => candidate === index ? { ...person, ...patch } : person));
  return <div className="metadata-people-editor">
    <div className="metadata-lock-heading"><span><strong>Cast &amp; Crew</strong><small>People saved here override scanner-provided credits for this item.</small></span>{action}</div>
    <div className="metadata-people-list">{people.map((person, index) => <div key={`${index}:${person.id ?? person.name}`}>
      <label><span>Name</span><input value={person.name} onChange={(event) => update(index, { name: event.target.value })} /></label>
      <label><span>Role</span><input value={person.role} onChange={(event) => update(index, { role: event.target.value })} /></label>
      <label><span>Character or credit</span><input value={person.character ?? ''} onChange={(event) => update(index, { character: event.target.value })} /></label>
      <IconButton label={`Remove ${person.name || 'credit'}`} onClick={() => onChange(people.filter((_, candidate) => candidate !== index))}><X /></IconButton>
    </div>)}</div>
    <SecondaryButton onClick={() => onChange([...people, { name: '', role: 'Actor', character: '' }])}><Plus /> Add person</SecondaryButton>
  </div>;
}

type EditorValues = {
  title: string;
  sortTitle: string;
  originalTitle: string;
  edition: string;
  year: string;
  durationSeconds: string;
  contentRating: string;
  communityRating: string;
  criticRating: string;
  studio: string;
  network: string;
  country: string;
  seasonNumber: string;
  episodeNumber: string;
  indexNumber: string;
  tagline: string;
  summary: string;
};

const emptyValues: EditorValues = { title: '', sortTitle: '', originalTitle: '', edition: '', year: '', durationSeconds: '', contentRating: '', communityRating: '', criticRating: '', studio: '', network: '', country: '', seasonNumber: '', episodeNumber: '', indexNumber: '', tagline: '', summary: '' };

function valuesFromItem(item: MediaItem): EditorValues {
  return {
    title: item.title,
    sortTitle: item.sortTitle ?? item.title.replace(/^(a|an|the)\s+/i, ''),
    originalTitle: item.originalTitle ?? item.title,
    edition: item.edition ?? '',
    year: item.year ? String(item.year) : '',
    durationSeconds: item.durationSeconds != null ? String(item.durationSeconds) : '',
    contentRating: item.contentRating ?? item.rating ?? '',
    communityRating: item.communityRating != null ? String(item.communityRating) : '',
    criticRating: item.criticRating != null ? String(item.criticRating) : '',
    studio: item.studio ?? '',
    network: item.network ?? '',
    country: item.country ?? '',
    seasonNumber: item.seasonNumber != null ? String(item.seasonNumber) : '',
    episodeNumber: item.episodeNumber != null ? String(item.episodeNumber) : '',
    indexNumber: item.indexNumber != null ? String(item.indexNumber) : '',
    tagline: item.tagline ?? '',
    summary: item.summary ?? '',
  };
}

export function MediaMetadataEditor({ mediaIds, initialItems = [], onDismiss, onSaved }: { mediaIds: string[]; initialItems?: MediaItem[]; onDismiss: () => void; onSaved?: () => void }) {
  const single = mediaIds.length === 1;
  const [detailRevision, setDetailRevision] = useState(0);
  const detail = useMediaDetail(mediaIds[0], detailRevision);
  const mutations = useMediaMutations();
  const operations = useMediaOperations();
  const initialized = useRef(false);
  const lastDetail = useRef<MediaItem | undefined>(undefined);
  const [tab, setTab] = useState('General');
  const [values, setValues] = useState<EditorValues>(emptyValues);
  const [genres, setGenres] = useState<string[]>([]);
  const [tags, setTags] = useState<string[]>([]);
  const [labels, setLabels] = useState<string[]>([]);
  const [people, setPeople] = useState<MediaPerson[]>([]);
  const [typedMetadata, setTypedMetadata] = useState<Record<string, string>>({});
  const [lockedFields, setLockedFields] = useState<string[]>([]);
  const [touched, setTouched] = useState<string[]>([]);
  const [locksTouched, setLocksTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [matchQuery, setMatchQuery] = useState('');
  const [matches, setMatches] = useState<MediaMatchCandidate[]>([]);
  const [matching, setMatching] = useState(false);
  if (single && detail.status === 'success') lastDetail.current = detail.data;
  const sourceItems = single ? (detail.status === 'success' ? [detail.data] : lastDetail.current ? [lastDetail.current] : initialItems) : initialItems;
  const first = sourceItems[0];
  useEffect(() => {
    if (initialized.current || !first) return;
    initialized.current = true;
    if (single) {
      setValues(valuesFromItem(first));
      setGenres(parseList(first.genre));
      setTags(first.tags ?? []);
      setLabels(first.labels ?? []);
      setPeople(first.people ?? []);
      setTypedMetadata(first.typedMetadata ?? {});
      setLockedFields(first.lockedFields ?? []);
    }
  }, [first, single]);
  const tabs = single
    ? ['General', 'Artwork', 'Media', ...(first?.kind === 'track' ? ['Lyrics'] : []), 'Tags', 'Cast & Crew', 'Matching']
    : ['General', 'Tags'];
  const mark = (field: keyof EditorValues, value: string) => {
    setValues((current) => ({ ...current, [field]: value }));
    setTouched((current) => current.includes(field) ? current : [...current, field]);
  };
  const markList = (field: 'genres' | 'tags' | 'labels', next: string[]) => {
    if (field === 'genres') setGenres(next);
    if (field === 'tags') setTags(next);
    if (field === 'labels') setLabels(next);
    setTouched((current) => current.includes(field) ? current : [...current, field]);
  };
  const markPeople = (next: MediaPerson[]) => {
    setPeople(next);
    setTouched((current) => current.includes('people') ? current : [...current, 'people']);
  };
  const markTypedMetadata = (key: string, value: string) => {
    setTypedMetadata((current) => ({ ...current, [key]: value }));
    setTouched((current) => current.includes('typedMetadata') ? current : [...current, 'typedMetadata']);
  };
  const toggleLock = (field: string) => {
    setLockedFields((current) => current.includes(field) ? current.filter((value) => value !== field) : [...current, field]);
    setLocksTouched(true);
  };
  const lockControl = (key: string, label: string) => {
    const locked = lockedFields.includes(key);
    const action = `${locked ? 'Unlock' : 'Lock'} ${label}`;
    return <button type="button" aria-label={action} aria-pressed={locked} title={`${action}. Locked values are preserved during metadata refreshes.`} onClick={() => toggleLock(key)}>{locked ? <Lock /> : <LockOpen />}</button>;
  };
  const field = (key: keyof EditorValues, label: string, lockable = single) => <label className={`metadata-field ${touched.includes(key) ? 'touched' : ''}`}><span>{label}{lockable && lockControl(key, label)}</span><input value={values[key]} placeholder={single ? '' : 'Mixed · leave unchanged'} onChange={(event) => mark(key, event.target.value)} /></label>;
  const save = async () => {
    if (!touched.length && !locksTouched) { setError('Change at least one field before saving.'); return; }
    const patch: MediaMetadataUpdate = {};
    for (const key of touched) {
      if (key === 'genres') patch.genres = genres;
      else if (key === 'tags') patch.tags = tags;
      else if (key === 'labels') patch.labels = labels;
      else if (key === 'people') patch.people = people.filter((person) => person.name.trim() && person.role.trim()).map((person, index) => ({ ...person, name: person.name.trim(), role: person.role.trim(), character: person.character?.trim(), sortOrder: index }));
      else if (key === 'typedMetadata') patch.typedMetadata = typedMetadata;
      else if (key === 'year') {
        const value = Number(values.year);
        if (!Number.isInteger(value) || value < 0 || value > 9999) { setError('Enter a valid four-digit year.'); return; }
        patch.year = value;
      } else if (['durationSeconds', 'criticRating', 'seasonNumber', 'episodeNumber', 'indexNumber'].includes(key)) {
        const value = Number(values[key as keyof EditorValues]);
        const maximum = key === 'criticRating' ? 100 : Number.MAX_SAFE_INTEGER;
        if (!Number.isInteger(value) || value < 0 || value > maximum) { setError(`Enter a valid ${key.replace(/([A-Z])/g, ' $1').toLocaleLowerCase()}.`); return; }
        (patch as Record<string, unknown>)[key] = value;
      } else if (key === 'communityRating') {
        const value = Number(values.communityRating);
        if (!Number.isFinite(value) || value < 0 || value > 10) { setError('Enter a community rating between 0 and 10.'); return; }
        patch.communityRating = value;
      } else {
        (patch as Record<string, unknown>)[key] = values[key as keyof EditorValues];
      }
    }
    if (locksTouched) patch.lockedFields = lockedFields;
    setBusy(true); setError('');
    try {
      await mutations.updateMetadata(mediaIds, patch);
      onSaved?.(); onDismiss();
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Metadata' }));
    } finally { setBusy(false); }
  };
  const searchMatches = async () => {
    if (!single) return;
    setMatching(true); setError('');
    try { setMatches(await mutations.searchMatches(mediaIds[0], matchQuery.trim() || first?.title || '')); }
    catch (reason) { setError(reviewedProductErrorText(reason, 'media.search-failed', { featureName: 'Metadata' })); }
    finally { setMatching(false); }
  };
  const applyMatch = async (candidate: MediaMatchCandidate) => {
    setBusy(true); setError('');
    try {
      await mutations.applyMatch(mediaIds[0], candidate);
      onSaved?.(); onDismiss();
    } catch (reason) { setError(reviewedProductErrorText(reason, 'media.update-failed', { featureName: 'Metadata match' })); }
    finally { setBusy(false); }
  };
  const mediaUpdated = () => {
    setDetailRevision((current) => current + 1);
    onSaved?.();
  };
  const uploadArtwork = async (type: string, file: File) => {
    await mutations.uploadArtwork(mediaIds[0], type, file);
    mediaUpdated();
  };
  const deleteArtwork = async (imageId: string) => {
    await mutations.deleteArtwork(mediaIds[0], imageId);
    mediaUpdated();
  };
  const preferArtwork = async (imageId: string) => {
    await mutations.setPreferredArtwork(mediaIds[0], imageId);
    mediaUpdated();
  };
  const reorderArtwork = async (imageIds: string[]) => {
    await mutations.reorderArtwork(mediaIds[0], imageIds);
    mediaUpdated();
  };
  const loadTechnicalOptions = useCallback(
    () => operations.downloadOptions(mediaIds[0]),
    [mediaIds, operations],
  );
  const immediateTab = ['Artwork', 'Media', 'Lyrics'].includes(tab);
  const pendingChanges = touched.length + (locksTouched ? 1 : 0);
  const acceptedIdentities = first?.providerIds ?? [];
  const normalizedEvidence = [
    ...(first?.metadataEvidence?.values ?? []).map((value) => ({
      key: `value:${value.field}:${value.order}`,
      kind: value.field,
      label: value.field.replace(/([A-Z])/g, ' $1'),
      value: metadataEvidenceValue(value.value),
      source: value.provider || value.sourceKind,
    })),
    ...(first?.metadataEvidence?.relationships ?? []).map((relationship) => ({
      key: `relationship:${relationship.type}:${relationship.order}:${relationship.externalId ?? relationship.name}`,
      kind: relationship.type,
      label: relationship.type.replace(/([A-Z])/g, ' $1'),
      value: relationship.name || relationship.externalId || '',
      source: relationship.provider || relationship.sourceKind,
    })),
  ].filter((entry) => catalogEvidenceKinds.has(entry.kind) && entry.value).slice(0, 24);
  const headingId = 'media-metadata-editor-title';
  if (single && detail.status === 'loading' && !first) return <ModalOverlay labelledBy={headingId} className="metadata-editor metadata-loading" onDismiss={onDismiss}><h1 id={headingId}>Edit metadata</h1><div className="library-state" aria-busy="true"><RefreshCw className="state-spinner" /><strong>Loading metadata</strong></div></ModalOverlay>;
  if (single && detail.status === 'error' && !first) return <ModalOverlay labelledBy={headingId} className="metadata-editor metadata-loading" onDismiss={onDismiss}><h1 id={headingId}>Edit metadata</h1><div className="library-state error" role="alert"><strong>Metadata is unavailable</strong><p>{reviewedProductErrorText(detail.error, 'media.load-failed', { featureName: 'Metadata' })}</p><SecondaryButton onClick={onDismiss}>Close</SecondaryButton></div></ModalOverlay>;
  return <ModalOverlay labelledBy={headingId} className="metadata-editor" onDismiss={onDismiss}>
    <header className="metadata-header"><div><p>{single ? `${first?.libraryName || 'Media library'} / ${mediaKindLabel(first?.kind)}` : `${mediaIds.length} selected items`}</p><h1 id={headingId}>Edit metadata</h1>{single && first && <span>{first.title}</span>}</div><IconButton label="Close metadata editor" onClick={onDismiss}><X /></IconButton></header>
    <div className="metadata-body"><nav className="metadata-tabs" aria-label="Metadata sections">{tabs.map((name) => <button type="button" key={name} className={tab === name ? 'active' : ''} onClick={() => setTab(name)}>{name}</button>)}</nav><div className="metadata-panel">
      {tab === 'General' && <><div className="metadata-form-grid">{single && field('title', 'Title')}{single && field('sortTitle', 'Sort title')}{single && field('originalTitle', 'Original title')}{single && field('edition', 'Edition')}{field('year', 'Year')}{single && field('durationSeconds', 'Duration (seconds)')}{field('contentRating', 'Content rating')}{single && field('communityRating', 'Community rating (0–10)')}{single && field('criticRating', 'Critic rating (0–100)')}{field('studio', 'Studio')}{field('network', 'Network')}{field('country', 'Country')}{single && (first?.kind === 'episode' || first?.kind === 'season') && field('seasonNumber', 'Season number')}{single && first?.kind === 'episode' && field('episodeNumber', 'Episode number')}{single && ['track', 'chapter'].includes(first?.kind ?? '') && field('indexNumber', 'Track number')}</div>{single && <label className="metadata-field wide"><span>Tagline{lockControl('tagline', 'Tagline')}</span><input value={values.tagline} onChange={(event) => mark('tagline', event.target.value)} /></label>}<label className="metadata-field wide"><span>Summary{single && lockControl('summary', 'Summary')}</span><textarea value={values.summary} placeholder={single ? '' : 'Mixed · leave unchanged'} onChange={(event) => mark('summary', event.target.value)} /></label>{single && Object.keys(typedMetadata).length > 0 && <div className="metadata-stack"><div className="metadata-lock-heading"><span><strong>Additional metadata</strong><small>Provider-specific fields stored with this item.</small></span>{lockControl('typedMetadata', 'Additional metadata')}</div><div className="metadata-form-grid">{Object.entries(typedMetadata).sort(([left], [right]) => left.localeCompare(right)).map(([key, value]) => <label className="metadata-field" key={key}><span>{key.replace(/([A-Z])/g, ' $1')}{lockControl(`typedMetadata.${key}`, key)}</span><input value={value} onChange={(event) => markTypedMetadata(key, event.target.value)} /></label>)}</div></div>}</>}
      {tab === 'Artwork' && first && <div className="metadata-stack"><div className="metadata-lock-heading"><span><strong>Artwork</strong><small>Lock artwork to keep scanners from replacing the selected images.</small></span>{lockControl('artwork', 'Artwork')}</div><ArtworkEditor images={first.mediaImages ?? []} fallbackUrls={{ poster: first.poster, backdrop: first.backdrop, thumb: first.backdrop }} onUpload={uploadArtwork} onDelete={deleteArtwork} onPreferred={preferArtwork} onReorder={reorderArtwork} /></div>}
      {tab === 'Media' && first && <TechnicalMediaEditor
        item={first}
        attachments={first.attachments}
        loadOptions={loadTechnicalOptions}
        onUploadSubtitle={async (file, language, label) => { await mutations.uploadSubtitle(first.id, file, language, label); mediaUpdated(); }}
        onUpdateSubtitle={async (streamId, offsetMs) => { await mutations.updateSubtitle(first.id, streamId, offsetMs); mediaUpdated(); }}
        onDeleteSubtitle={async (streamId) => { await mutations.deleteSubtitle(first.id, streamId); mediaUpdated(); }}
        onCreateVersion={async (profile) => { await operations.createOptimizedVersion(first.id, profile); mediaUpdated(); }}
        onDeleteVersion={async (profile) => { await operations.deleteOptimizedVersion(first.id, profile); mediaUpdated(); }}
      />}
      {tab === 'Lyrics' && first?.kind === 'track' && <LyricsEditor
        lyrics={first.lyrics ?? []}
        defaultQuery={[first.title, first.subtitle.split(' · ')[0]].filter(Boolean).join(' ')}
        onUpload={async (file, language) => { await mutations.uploadLyrics(first.id, file, language); mediaUpdated(); }}
        onFetch={async () => { await mutations.fetchLyrics(first.id); mediaUpdated(); }}
        onSearch={(query) => mutations.searchLyrics(first.id, query)}
        onApply={async (candidate) => { await mutations.applyLyrics(first.id, candidate); mediaUpdated(); }}
        onDelete={async (lyricId) => { await mutations.deleteLyrics(first.id, lyricId); mediaUpdated(); }}
      />}
      {tab === 'Tags' && <div className="metadata-stack"><TagEditor action={single && lockControl('genres', 'Genres')} label="Genres" values={genres} onChange={(next) => markList('genres', next)} /><TagEditor action={single && lockControl('tags', 'Tags')} label="Tags" values={tags} onChange={(next) => markList('tags', next)} /><TagEditor action={single && lockControl('labels', 'Labels')} label="Labels" values={labels} onChange={(next) => markList('labels', next)} /></div>}
      {tab === 'Cast & Crew' && single && <PeopleEditor people={people} action={lockControl('people', 'Cast & Crew')} onChange={markPeople} />}
      {tab === 'Matching' && <div className="matching-panel">
        {(acceptedIdentities.length > 0 || normalizedEvidence.length > 0) && <section className="metadata-source-summary" aria-label="Current metadata sources">
          <div className="metadata-lock-heading"><span><strong>Current metadata sources</strong><small>Accepted identities and revision {first?.metadataEvidence?.revision ?? '—'} normalized catalog evidence.</small></span></div>
          {acceptedIdentities.length > 0 && <div className="metadata-evidence-list" aria-label="Accepted provider identities">{acceptedIdentities.map((identity) => {
            const provider = identity.provider.toLocaleLowerCase();
            const attribution = provider === 'tvdb' ? { label: 'TheTVDB', href: 'https://thetvdb.com/' } : provider === 'tmdb' ? { label: 'TMDB', href: 'https://www.themoviedb.org/' } : undefined;
            return <span key={`${identity.provider}:${identity.externalType}:${identity.externalId}`}><strong>{attribution ? <a href={attribution.href} target="_blank" rel="noreferrer">{attribution.label}</a> : identity.provider.toLocaleUpperCase()}</strong><small>{identity.externalType} · {identity.externalId}</small></span>;
          })}</div>}
          {normalizedEvidence.length > 0 && <><div className="metadata-lock-heading"><span><strong>Search and filter metadata</strong><small>Normalized fields available to catalog discovery where that field applies.</small></span></div><div className="metadata-evidence-list">{normalizedEvidence.map((entry) => <span key={entry.key}><strong>{entry.label}</strong><small>{entry.value} · {entry.source}</small></span>)}</div></>}
        </section>}
        <form className="match-search" onSubmit={(event) => { event.preventDefault(); void searchMatches(); }}><input value={matchQuery} onChange={(event) => setMatchQuery(event.target.value)} placeholder="Title, year, or provider ID" /><PrimaryButton type="submit" disabled={matching}><Search /> {matching ? 'Searching…' : 'Search'}</PrimaryButton></form>{matches.length > 0 && <div className="match-results">{matches.map((candidate) => <article key={`${candidate.provider}:${candidate.externalId}`}><div className="match-poster">{candidate.posterUrl ? <img src={candidate.posterUrl} alt="" /> : <ScanSearch />}</div><span><strong>{candidate.title || candidate.externalId}</strong><small>{[candidate.year, candidate.source, `${Math.round(Math.min(100, Math.max(0, candidate.score)))}% match`].filter(Boolean).join(' · ')}</small>{candidate.overview && <p>{candidate.overview}</p>}</span><SecondaryButton disabled={busy} onClick={() => void applyMatch(candidate)}>{candidate.accepted && <Check />} Use match</SecondaryButton></article>)}</div>}
      </div>}
      {error && <p className="context-action-error" role="alert">{error}</p>}
    </div></div>
    <footer className="metadata-footer"><span>{immediateTab ? pendingChanges ? `${pendingChanges} metadata ${pendingChanges === 1 ? 'change' : 'changes'} pending · ${tab} saves immediately` : `${tab} changes save immediately` : pendingChanges ? `${pendingChanges} pending ${pendingChanges === 1 ? 'change' : 'changes'}` : 'No changes yet'}</span><SecondaryButton onClick={onDismiss}>{immediateTab && !pendingChanges ? 'Close' : 'Cancel'}</SecondaryButton>{(!immediateTab || pendingChanges > 0) && <PrimaryButton disabled={busy} onClick={() => void save()}>{busy ? 'Saving…' : single ? 'Save changes' : `Apply to ${mediaIds.length} items`}</PrimaryButton>}</footer>
  </ModalOverlay>;
}
