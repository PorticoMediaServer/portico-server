import {useEffect, useMemo, useRef, useState, type ComponentType, type ReactNode} from 'react';
import {defaultI18n, formatMessage} from '@i18n';
import type {RepairFieldSpec} from '@core/metadata-repair';
import type {BulkReceipt} from '@core/metadata-bulk';
import type {DeletePreview, DeleteResult} from '@core/administration';
import {selectionFixtures} from '../../../../packages/i18n/src/catalog/en-US/next/dev/selection-fixtures';
import {defineGalleryFamily} from '../gallery-registry';
import {Artwork, CoverMosaic, createArtworkRuntime} from '../../kit/artwork';
import {PosterCard} from '../../kit/cards';
import {Button} from '../../kit/controls';
import {OwnerTag, ServerMark} from '../../kit/shell';
import {SelectionCircle, SelectionHoldFeedback, SelectionBar, PhoneActionBar, MixedEditExplainer, BulkEditor, PlaylistPicker, CollectionPicker, PickerAddedToast, DeleteMediaDialog, bulkReceiptView, type BulkEditorDraft, type BulkFieldValues, type PickerEntry, type PickerResult, type SelectionActions, type SelectionItemDisplay} from '../../kit/selection';
import {selectionText as text} from '../../kit/selection/copy';
import styles from '../../kit/selection/gallery.module.css';

const fixture = (key: keyof typeof selectionFixtures, values?: Record<string, string | number>) => formatMessage(selectionFixtures[key], values);
const titleKeys = ['movie1', 'movie2', 'movie3', 'movie4'] as const;
const files = ['movies/toy-story.jpg', 'movies/toy-story-2.jpg', 'movies/toy-story-3.jpg', 'movies/toy-story-5.jpg', 'movies/inside-out.jpg', 'movies/moana.jpg', 'movies/the-incredibles.jpg', 'movies/dune.jpg', 'tv/the-last-of-us--bd.jpg', 'albums/after-hours.jpg'] as const;
const names = () => titleKeys.map(key => fixture(`next.selection.fixture.${key}`));
type EvidenceSlots = Readonly<{editorOwnerTag?: ReactNode}>;
declare global { var __porticoSelectionEvidence: EvidenceSlots | undefined; }

function useReferenceArt() {
  const runtime = useMemo(() => createArtworkRuntime({scopeKey: 'dev-selection-private-reference-art', load: async ({url}, signal) => {
    // A real decoded image handle; no opaque fetch is labelled a blob or ambient artwork.
    const image = new Image(); image.src = url;
    const cancel = () => { image.src = ''; };
    signal.addEventListener('abort', cancel, {once: true});
    try { await image.decode(); if (signal.aborted) throw new DOMException('Aborted', 'AbortError'); return {src: url, width: image.naturalWidth, height: image.naturalHeight}; }
    finally { signal.removeEventListener('abort', cancel); }
  }, classify: () => ({retryable: false})}), []);
  useEffect(() => () => runtime.dispose(), [runtime]);
  return useMemo(() => files.map(file => runtime.source({url: `http://100.120.50.24:8787/assets/pool/${file}`, cacheKey: `selection-reference:${file}`})), [runtime]);
}
function Stage({children}: {children: ReactNode}) { return <div className={styles.stage}>{children}</div>; }
function useTargets(): readonly SelectionItemDisplay[] {
  const art = useReferenceArt();
  return names().map((title, index) => ({id: `selection-movie-${index + 1}`, title, artwork: <Artwork source={art[index]} alt="" width="100%" height="100%" />}));
}
function useEditorOwnerTag() {
  const [slot, setSlot] = useState(globalThis.__porticoSelectionEvidence?.editorOwnerTag);
  useEffect(() => { const update = () => setSlot(globalThis.__porticoSelectionEvidence?.editorOwnerTag); window.addEventListener('portico-selection-evidence', update); return () => window.removeEventListener('portico-selection-evidence', update); }, []);
  return slot;
}

function Circles({hold = false, unknown = false}: {hold?: boolean; unknown?: boolean}) {
  const [selected, setSelected] = useState(false), art = useReferenceArt();
  return <Stage><div className={styles.cards}><div className={styles.cardHost}>
    <PosterCard model={{title: names()[0]!, caption: []}} artwork={art[0]} onNavigate={() => setSelected(value => !value)} selection={{active: selected, checked: selected, control: <SelectionCircle accessibilityLabel={text('next.selection.select')} selected={selected} selecting={selected} interactive={false} />, onToggle: () => setSelected(value => !value), onExit: () => setSelected(false)}} />
    {hold && <SelectionHoldFeedback active progress={unknown ? null : .5} className={styles.hold} />}
  </div></div><div className={styles.circles}><SelectionCircle selected={selected} selecting accessibilityLabel={text('next.selection.select')} onSelectedChange={setSelected} /><SelectionCircle selected={false} selecting variant="row" accessibilityLabel={text('next.selection.select')} onSelectedChange={setSelected} /><SelectionCircle selected selecting disabled accessibilityLabel={text('next.selection.select')} /><SelectionCircle interactive={false} selected={selected} selecting accessibilityLabel={text('next.selection.select')} /></div></Stage>;
}

function Bars({variant = 'same'}: {variant?: 'same' | 'mixed' | 'whole' | 'home' | 'episode'}) {
  const [count, setCount] = useState(variant === 'whole' ? 48000 : variant === 'episode' ? 3 : variant === 'mixed' ? 5 : 4), [notice, setNotice] = useState('');
  const record = (action: string) => setNotice(fixture('next.selection.fixture.action', {action, count}));
  const actions: SelectionActions = {onPlay: () => record(text('next.selection.play')), onQueue: where => record(text(where === 'next' ? 'next.selection.playNext' : 'next.selection.playLater')), onPlaylist: () => record(text('next.selection.addToPlaylist')), onMyList: () => record(text('next.selection.myList')), onWatchedChange: variant === 'mixed' ? undefined : watched => record(text(watched ? 'next.selection.markWatched' : 'next.selection.markUnwatched')), onEdit: () => record(text('next.selection.edit')), editMode: variant === 'mixed' ? 'mixed' : 'editable', onCollection: () => record(text('next.selection.addToCollection')), onFavorite: () => record(text('next.selection.favorite')), onRemoveFromMyList: () => record(text('next.selection.removeFromMyList')), onRefresh: () => record(text('next.selection.refresh')), onDelete: () => record(text('next.selection.deleteMenu')), deleteDisabledReason: variant === 'mixed' ? text('next.selection.oneLibrary') : undefined, ownerMark: <ServerMark size={22} />};
  const selection = {count, kind: variant === 'mixed' ? undefined : variant === 'episode' ? 'episode' as const : 'movie' as const};
  return <Stage><SelectionBar selection={selection} actions={actions} context={variant === 'home' ? fixture('next.selection.fixture.recent') : variant === 'episode' ? fixture('next.selection.fixture.season') : undefined} typeCount={variant === 'mixed' ? 4 : 1} selectAll={variant === 'home' ? {kind: 'row'} : variant === 'whole' ? {kind: 'clear'} : {kind: 'view', total: 80}} onSelectAll={() => setCount(variant === 'whole' ? 0 : 80)} onExit={() => setCount(0)} allInView={variant === 'whole' ? {visible: 14} : undefined} sticky={variant !== 'same'} /><PhoneActionBar selection={selection} actions={actions} episodeSelection={variant === 'episode'} />{count === 0 && <Button className={styles.anchor} onClick={() => setCount(4)}>{fixture('next.selection.fixture.open')}</Button>}{notice && <output className={styles.result}>{notice}</output>}</Stage>;
}

function Mixed() {
  const anchor = useRef<HTMLButtonElement | null>(null), [open, setOpen] = useState(true), [kind, setKind] = useState('');
  return <Stage><Button ref={anchor} className={styles.anchor} onClick={() => setOpen(true)}>{text('next.selection.edit')}</Button><MixedEditExplainer open={open} onOpenChange={setOpen} anchorRef={anchor} types={[{kind: 'movie', count: 2}, {kind: 'episode', count: 1}, {kind: 'album', count: 1}, {kind: 'song', count: 1}]} onKeepOnly={type => { setKind(type); setOpen(false); }} /><output className={styles.result}>{kind}</output></Stage>;
}

function schemaField(field: string, label: string, type: RepairFieldSpec['type'] = 'text', bulk = true): RepairFieldSpec {
  return {field, label, type, bulk, group: type === 'integer' ? 'numbers' : type === 'multiline' || field === 'tagline' ? 'text' : 'general',
    ...(type === 'integer' ? {min: field === 'episodeNumber' ? 1 : 0, max: field === 'discNumber' ? 999 : 9999} : {maxLength: type === 'multiline' ? 20000 : type === 'list' ? 128 : type === 'date' ? 10 : 300}),
    ...(type === 'list' ? {max: 64} : {})};
}
function Editor({kind = 'movie', partial = false, error = false}: {kind?: 'movie' | 'episode' | 'song'; partial?: boolean; error?: boolean}) {
  const allTargets = useTargets(), art = useReferenceArt(), targets = kind === 'episode' ? allTargets.slice(0, 3).map((target, index) => ({...target, title: fixture(`next.selection.fixture.episode${index + 1}` as keyof typeof selectionFixtures), artwork: <Artwork source={art[8]} alt="" width="100%" height="100%" zoom={2.4} saturation={.55} brightness={.42} />})) : kind === 'song' ? allTargets.map((target, index) => ({...target, title: fixture(`next.selection.fixture.track${index + 1}` as keyof typeof selectionFixtures), artwork: <Artwork source={art[9]} alt="" width="100%" height="100%" />})) : allTargets;
  const count = targets.length, [open, setOpen] = useState(true), owner = useEditorOwnerTag();
  const label = (key: string) => fixture(`next.selection.fixture.${key}` as keyof typeof selectionFixtures);
  // Exact repair registry shape and bounds; Genres is the preview's separate relationship field.
  const singleItemFields = [
    schemaField('title', defaultI18n.t('web.metadata.field.title'), 'text', false),
    schemaField('sortTitle', defaultI18n.t('web.metadata.field.sortTitle'), 'text', false),
    schemaField('metadataLanguage', label('language')),
    ...(kind === 'movie' ? [schemaField('originalTitle', defaultI18n.t('web.metadata.field.originalTitle'), 'text', false), schemaField('edition', label('edition')), schemaField('tagline', defaultI18n.t('web.metadata.field.tagline'), 'text', false)] : []),
    schemaField('description', defaultI18n.t('builder.description'), 'multiline', false),
    schemaField('year', label('year'), 'integer'),
    ...(kind !== 'song' ? [schemaField('releaseDate', kind === 'episode' ? label('airDate') : defaultI18n.t('web.metadata.field.releaseDate'), 'date', false), schemaField('contentRating', label('rating'))] : []),
    ...(kind === 'movie' ? [schemaField('studio', label('studio')), schemaField('country', label('country'))] : kind === 'episode' ? [schemaField('network', label('network')), schemaField('seasonNumber', defaultI18n.t('web.metadata.field.seasonNumber'), 'integer', false), schemaField('episodeNumber', defaultI18n.t('web.metadata.field.episodeNumber'), 'integer', false)] : [schemaField('trackNumber', defaultI18n.t('web.metadata.field.trackNumber'), 'integer', false), schemaField('discNumber', defaultI18n.t('web.metadata.field.discNumber'), 'integer', false)]),
    schemaField('tags', label('tags'), 'list'), schemaField('labels', label('labels'), 'list'),
  ];
  const schema = [...singleItemFields.filter(field => field.bulk), ...(kind !== 'episode' ? [schemaField('genres', label('genres'), 'list')] : [])];
  const scalar = (value: string): BulkFieldValues => ({state: 'shared', distinctCount: 1, truncated: false, value, values: [{value, count}], entries: []});
  // These are all fixture targets, not bounded examples from a larger preview.
  const list = (members: readonly (readonly string[])[]): BulkFieldValues => {
    const variants = new Map<string, number>(), entries = new Map<string, number>();
    for (const member of members) {
      const value = member.length ? JSON.stringify(member) : '';
      variants.set(value, (variants.get(value) ?? 0) + 1);
      for (const entry of member) entries.set(entry, (entries.get(entry) ?? 0) + 1);
    }
    const values = [...variants].map(([value, count]) => ({value, count}));
    return {state: variants.size === 1 ? 'shared' : 'mixed', distinctCount: variants.size, truncated: false, ...(variants.size === 1 ? {value: values[0].value} : {}), values, entries: [...entries].map(([value, count]) => ({value, count}))};
  };
  const values: Record<string, BulkFieldValues> = {
    ...(kind !== 'song' ? {contentRating: {state: 'mixed', distinctCount: 2, truncated: false, values: [{value: kind === 'episode' ? 'TV-MA' : 'G', count: count - 1}, {value: '', count: 1}], entries: []}} : {}),
    ...(kind === 'movie' ? {studio: scalar(label('pixar')), country: scalar(label('us')), edition: scalar('')} : kind === 'episode' ? {network: scalar(label('hbo'))} : {}),
    year: kind === 'song' ? {state: 'mixed', distinctCount: 2, truncated: false, values: [{value: '2020', count: 3}, {value: '2019', count: 1, titles: targets.slice(2, 3).map(target => target.title)}], entries: []} : kind === 'episode' ? scalar('2023') : {state: 'mixed', distinctCount: 4, truncated: false, values: [{value: '1995', count: 1}, {value: '1999', count: 1}, {value: '2010', count: 1}, {value: '2026', count: 1}], entries: []},
    metadataLanguage: kind === 'episode' ? {state: 'mixed', distinctCount: 2, truncated: false, values: [{value: 'en', count: 2}, {value: 'fr', count: 1, titles: targets.slice(2, 3).map(target => target.title)}], entries: []} : scalar('en'),
    ...(kind !== 'episode' ? {genres: list(kind === 'song' ? [[label('rnb'), label('pop')], [label('rnb'), label('pop')], [label('rnb')], [label('rnb')]] : [[label('animation'), label('family'), label('adventure')], [label('animation'), label('family'), label('adventure'), label('comedy')], [label('animation'), label('family'), label('adventure'), label('comedy')], [label('animation'), label('family'), label('comedy')]])} : {}),
    tags: list(targets.map((_, index) => kind === 'movie' && index < 3 ? [label('pixar')] : [])),
    labels: list(targets.map(() => [])),
  };
  const [draft, setDraft] = useState<BulkEditorDraft>({fields: kind === 'movie' ? {contentRating: 'G'} : kind === 'episode' ? {metadataLanguage: 'en'} : {year: '2020'}, lists: kind === 'movie' ? {tags: {add: [label('playlist')], remove: []}} : {}, lockEdited: true});
  const receipt = (members: readonly SelectionItemDisplay[], fail: boolean, operationId: string): BulkReceipt => {
    const results = members.map((target, index) => ({kind: 'item', id: target.id, ok: !fail || index !== members.length - 1, code: fail && index === members.length - 1 ? 'metadata_conflict' : undefined, revision: 'a'.repeat(64)}));
    return {operationId, updated: results.filter(row => row.ok).length, failed: results.filter(row => !row.ok).length, results};
  };
  const [receipts, setReceipts] = useState<readonly BulkReceipt[]>(partial ? [receipt(targets, true, 'dev-selection-initial')] : []);
  const result = receipts.length ? bulkReceiptView(receipts) : undefined;
  return <Stage><Button className={styles.anchor} onClick={() => setOpen(true)}>{text('next.selection.edit')}</Button><BulkEditor open={open} onOpenChange={setOpen} selection={{kind, count}} schema={schema} singleItemFields={singleItemFields} values={values} draft={draft} onDraftChange={setDraft}
    onSave={() => setReceipts(current => [...current, receipt(targets, false, `dev-selection-save-${current.length + 1}`)])}
    onRetry={failedIds => setReceipts(current => {
      const latest = bulkReceiptView(current), failures = new Set(latest.results.filter(row => !row.ok).map(row => row.id));
      const members = targets.filter(target => failedIds.includes(target.id) && failures.has(target.id));
      return members.length ? [...current, receipt(members, false, `dev-selection-retry-${current.length + 1}`)] : current;
    })}
    targets={targets} resultDescriptions={Object.fromEntries(targets.filter(target => result?.results.some(row => row.id === target.id && row.ok)).map(target => [target.id, label(kind === 'episode' ? 'savedLanguage' : kind === 'song' ? 'savedYear' : 'savedChanges')]))} ownerTag={owner} description={kind === 'episode' ? label('episodeDescription') : kind === 'song' ? label('trackDescription') : undefined} result={result} error={error ? fixture('next.selection.fixture.error') : undefined} /></Stage>;
}

function Picker({mode: initialMode = 'choose', partial = false, collection = false, failed = false}: {mode?: 'choose' | 'new'; partial?: boolean; collection?: boolean; failed?: boolean}) {
  const art = useReferenceArt(), anchor = useRef<HTMLButtonElement | null>(null), [open, setOpen] = useState(true), [mode, setMode] = useState(initialMode), [name, setName] = useState(initialMode === 'new' ? fixture('next.selection.fixture.newName') : ''), [query, setQuery] = useState(''), [toast, setToast] = useState(false);
  const labels = collection ? ['collection', 'collection2', 'collection3'] : ['playlist', 'playlist2', 'playlist3', 'playlist4'];
  const entries: PickerEntry[] = labels.map((label, index) => ({id: `selection-destination-${index}`, name: fixture(`next.selection.fixture.${label}` as keyof typeof selectionFixtures), size: (collection ? [9, 11, 7] : [12, 6, 9, 15])[index], canAdd: true, ...(!collection && index === 0 ? {alreadyIn: 2} : {}), artwork: <CoverMosaic covers={index === 0 ? [art[4], art[5], art[0], art[6]] : art.slice(index, index + 4)} alt="" width="100%" height="100%" />}));
  // Directory intersections can be unknown. These complete DEV write observations are explicit.
  const outcomes: Readonly<Record<string, {inserted: number; alreadyIn: number}>> = collection ? {
    'selection-destination-0': {inserted: 4, alreadyIn: 0}, 'selection-destination-1': {inserted: 4, alreadyIn: 0}, 'selection-destination-2': {inserted: 4, alreadyIn: 0},
  } : {
    'selection-destination-0': {inserted: 2, alreadyIn: 2}, 'selection-destination-1': {inserted: 4, alreadyIn: 0}, 'selection-destination-2': {inserted: 4, alreadyIn: 0}, 'selection-destination-3': {inserted: 4, alreadyIn: 0},
  };
  const initialResult: PickerResult = {destination: {...entries[0], size: entries[0].size + 3}, inserted: 3, processed: 4, total: 4, alreadyIn: 0, failed: 1, unknown: 0, pending: false, complete: true, failures: [{id: 'selection-movie-4', title: fixture('next.selection.fixture.movie4'), reason: fixture('next.selection.fixture.pickerConflict')}]};
  const [result, setResult] = useState<PickerResult | undefined>(partial ? initialResult : undefined);
  const complete = (entry: PickerEntry, outcome: {inserted: number; alreadyIn: number}) => {
    if (!entry.canAdd) return;
    setResult({destination: {...entry, size: entry.size + outcome.inserted}, ...outcome, processed: 4, total: 4, failed: 0, unknown: 0, pending: false, complete: true, failures: []}); setOpen(false); setToast(true);
  };
  const Component = collection ? CollectionPicker : PlaylistPicker;
  return <Stage><Button ref={anchor} className={styles.anchor} onClick={() => setOpen(true)}>{text(collection ? 'next.selection.addToCollection' : 'next.selection.addToPlaylist')}</Button><Component open={open} onOpenChange={setOpen} selection={{kind: 'movie', count: 4}} anchorRef={anchor} entries={entries.filter(entry => entry.name.toLocaleLowerCase().includes(query.toLocaleLowerCase()))} query={query} onQueryChange={setQuery} mode={mode} onModeChange={setMode} name={name} onNameChange={setName}
    onAdd={id => { const entry = entries.find(item => item.id === id), outcome = outcomes[id]; if (entry && outcome) complete(entry, outcome); }}
    onCreateAndAdd={nextName => complete({id: 'selection-created', name: nextName, size: 0, canAdd: true, artwork: entries[0].artwork}, {inserted: 4, alreadyIn: 0})}
    onRetry={missing => {
      if (!result || !missing.length || new Set(missing).size !== missing.length || missing.some(id => !result.failures.some(item => item.id === id))) return;
      const failures = result.failures.filter(item => !missing.includes(item.id));
      setResult({...result, destination: {...result.destination, size: 12 + 3 + missing.length}, inserted: 3 + missing.length, failed: failures.length, failures});
      if (!failures.length) {setOpen(false); setToast(true);}
    }} result={result} error={failed ? fixture('next.selection.fixture.error') : undefined} />{result && <PickerAddedToast open={toast} onOpenChange={setToast} selection={{kind: 'movie', count: 4}} destination={collection ? 'collection' : 'playlist'} result={result} onView={() => setToast(false)} />}</Stage>;
}

type DeleteCase = 'one' | 'bulk' | 'record' | 'show' | 'limit' | 'blocked' | 'checking' | 'result' | 'error';
function Deletion({state = 'one'}: {state?: DeleteCase}) {
  const targets = useTargets(), [open, setOpen] = useState(true), [confirmation, setConfirmation] = useState(state === 'bulk' || state === 'record' ? '4' : ''), [removeFiles, setRemoveFiles] = useState(state !== 'record');
  const many = ['bulk', 'record', 'limit', 'result'].includes(state), count = state === 'limit' ? 240 : many ? 4 : 1;
  const selected = many ? targets : [targets[3]!], bytes = [3435973836, 3865470566, 6335076761, 4402341478];
  const preview: DeletePreview | null = state === 'checking' || state === 'limit' || state === 'error' ? null : {
    revision: 1, confirmation: many ? '4' : state === 'show' ? fixture('next.selection.fixture.otherShow') : targets[3]!.title,
    confirmationKind: many ? 'count' : 'title', totalBytes: many ? bytes.reduce((sum, value) => sum + value, 0) : state === 'show' ? 23192823398 : bytes[3]!, totalFiles: many ? 4 : state === 'show' ? 9 : 1,
    totalBytesText: fixture(many ? 'next.selection.fixture.previewBytes' : state === 'show' ? 'next.selection.fixture.showBytes' : 'next.selection.fixture.oneBytes'), blocked: state === 'blocked' ? ['selection-library'] : [],
    targets: selected.map((target, index) => ({itemId: target.id, title: state === 'show' ? fixture('next.selection.fixture.otherShow') : target.title, kind: state === 'show' ? 'show' : 'movie', libraryId: 'selection-library', libraryName: fixture('next.selection.fixture.movies'), allowMediaDeletion: state !== 'blocked', titles: state === 'show' ? 9 : 1, bytes: many ? bytes[index]! : bytes[3]!,
      files: Array.from({length: state === 'show' ? 9 : 1}, (_, file) => ({assetId: `${target.id}-file-${file}`, path: '', bytes: many ? bytes[index]! : bytes[3]!, present: true, shared: false})),
      dependents: state === 'show' ? [] : [{kind: 'playlist', count: many ? index === 0 ? 2 : 1 : 2, description: fixture('next.selection.fixture.playlistDependent')}, {kind: 'collection', count: many ? index < 2 ? 1 : 0 : 1, description: fixture('next.selection.fixture.collectionDependent')}, {kind: 'progress', count: many ? index < 2 ? 2 : 1 : 3, description: fixture('next.selection.fixture.progressDependent')}],
    })),
  };
  const outcome = (partial = false): DeleteResult => {
    const receipts = selected.map((target, index) => {
      const removed = !partial || index < selected.length - 1, files = state === 'show' ? 9 : 1;
      return {itemId: target.id, title: state === 'show' ? fixture('next.selection.fixture.otherShow') : target.title, removed, filesDeleted: removed && removeFiles ? files : 0, filesKept: removed && removeFiles ? 0 : files, bytes: many ? bytes[index]! : bytes[3]!, code: removed ? '' : 'file_not_removed'};
    });
    return {revision: 2, removed: receipts.filter(row => row.removed).length, failed: receipts.filter(row => !row.removed).length, bytesDeleted: removeFiles ? receipts.filter(row => row.removed).reduce((sum, row) => sum + row.bytes, 0) : 0, filesRetained: !removeFiles, receipts};
  };
  const [result, setResult] = useState<DeleteResult | undefined>(state === 'result' ? outcome(true) : undefined);
  const targetFacts = Object.fromEntries(targets.map((target, index) => [target.id, fixture('next.selection.fixture.deleteFacts', {year: [1995, 1999, 2010, 2026][index]!, size: defaultI18n.bytes(bytes[index]), files: 1})]));
  return <Stage><Button className={styles.anchor} onClick={() => setOpen(true)}>{text('next.selection.deleteMenu')}</Button><DeleteMediaDialog owner open={open} onOpenChange={setOpen} selectionCount={count} preview={preview} confirmation={confirmation} onConfirmationChange={setConfirmation} removeFiles={removeFiles} onRemoveFilesChange={setRemoveFiles} onDelete={() => setResult(outcome())} result={result} error={state === 'error' ? fixture('next.selection.fixture.error') : undefined} ownerTag={<span className={styles.serverSlot}><ServerMark size={22} /><OwnerTag /></span>} artwork={Object.fromEntries(targets.map(target => [target.id, target.artwork]))} targetFacts={targetFacts} /></Stage>;
}

const cases: {id: string; labelKey: 'next.selection.gallery.circle' | 'next.selection.gallery.bar' | 'next.selection.gallery.mixed' | 'next.selection.gallery.editor' | 'next.selection.gallery.picker' | 'next.selection.gallery.delete'; width: number; height: number; render: ComponentType}[] = [
  {id: 'circle-desktop', labelKey: 'next.selection.gallery.circle', width: 520, height: 440, render: Circles},
  {id: 'hold-phone', labelKey: 'next.selection.gallery.circle', width: 390, height: 440, render: () => <Circles hold />},
  {id: 'hold-unknown', labelKey: 'next.selection.gallery.circle', width: 390, height: 440, render: () => <Circles hold unknown />},
  ...(['same', 'mixed', 'whole', 'home', 'episode'] as const).map(variant => ({id: `bar-${variant}`, labelKey: 'next.selection.gallery.bar' as const, width: 1440, height: 500, render: () => <Bars variant={variant} />})),
  {id: 'bar-phone', labelKey: 'next.selection.gallery.bar', width: 390, height: 844, render: Bars},
  {id: 'bar-phone-episodes', labelKey: 'next.selection.gallery.bar', width: 390, height: 844, render: () => <Bars variant="episode" />},
  {id: 'mixed', labelKey: 'next.selection.gallery.mixed', width: 1440, height: 500, render: Mixed},
  {id: 'mixed-phone', labelKey: 'next.selection.gallery.mixed', width: 390, height: 844, render: Mixed},
  ...(['movie', 'episode', 'song'] as const).flatMap(kind => [
    {id: `editor-${kind}`, labelKey: 'next.selection.gallery.editor' as const, width: 1440, height: kind === 'movie' ? 1340 : 1100, render: () => <Editor kind={kind} />},
    {id: `editor-phone-${kind}`, labelKey: 'next.selection.gallery.editor' as const, width: 390, height: 844, render: () => <Editor kind={kind} />},
  ]),
  {id: 'editor-result', labelKey: 'next.selection.gallery.editor', width: 1440, height: 1000, render: () => <Editor partial />},
  {id: 'editor-phone-result', labelKey: 'next.selection.gallery.editor', width: 390, height: 844, render: () => <Editor partial />},
  {id: 'editor-error', labelKey: 'next.selection.gallery.editor', width: 1440, height: 1000, render: () => <Editor error />},
  ...(['choose', 'new', 'partial', 'collection', 'error'] as const).flatMap(mode => [1440, 390].map(width => ({id: `picker-${mode}-${width}`, labelKey: 'next.selection.gallery.picker' as const, width, height: width === 390 ? 844 : 600, render: () => <Picker mode={mode === 'new' ? 'new' : 'choose'} partial={mode === 'partial'} collection={mode === 'collection'} failed={mode === 'error'} />}))),
  ...(['one', 'bulk', 'record', 'show', 'limit', 'blocked', 'checking', 'result', 'error'] as const).flatMap(state => [1440, 390].map(width => ({id: `delete-${state}-${width}`, labelKey: 'next.selection.gallery.delete' as const, width, height: width === 390 ? 844 : 900, render: () => <Deletion state={state} />}))),
];

export default defineGalleryFamily({id: 'selection', labelKey: 'next.selection.gallery', cases});
