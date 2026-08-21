import type { Job, Library } from '@portico/client-core';
import {
  AlertTriangle,
  Ban,
  ChevronDown,
  ChevronUp,
  CheckCircle2,
  Clock3,
  Film,
  FolderOpen,
  Music2,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  ScanSearch,
  Trash2,
  Tv,
  X,
} from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { ModalOverlay } from '../../components/overlay/OverlayPortal';
import { productProblemText } from '../../components/ProductLanguage';
import {
  FilesystemPickerDialog,
  isAbsoluteFilesystemPath,
  sameFilesystemPath,
  type FilesystemPickerSource,
} from '../filesystem';
import { ChoiceControl, InlineNotice, SettingsGroup } from './SettingsControls';
import { useAbortableMutation } from './settingsHooks';
import type { LibraryMutationInput, LibraryScanMode, LibraryScanOperationsResponse, LibraryScanReviewResponse, SettingsDataSource } from './settingsTypes';

function libraryRoute(library: Library): string {
  return `/library/${encodeURIComponent(library.id)}`;
}

function LibraryIcon({ type }: { type: Library['type'] }) {
  if (type === 'movie') return <Film />;
  if (type === 'music' || type === 'audiobook') return <Music2 />;
  return <Tv />;
}

function scanStateLabel(status?: string) {
  if (status === 'queued') return 'Scan queued';
  if (status === 'running') return 'Scanning';
  if (status === 'complete' || status === 'completed') return 'Scanned';
  if (status === 'failed') return 'Scan failed';
  if (status === 'cancelled') return 'Scan cancelled';
  return status ? `Scan ${status}` : 'Scan requested';
}

type ScanRootState = {
  id?: string;
  path?: string;
  status?: 'healthy' | 'degraded' | 'unavailable' | 'stalled' | string;
  warning?: string;
  error?: string;
  errorClass?: string;
  latencyMillis?: number;
};

type ScanWorkCounts = {
  discovered?: number;
  processed?: number;
  indexed?: number;
  unchanged?: number;
  skipped?: number;
  added?: number;
  updated?: number;
  missing?: number;
  metadataPending?: number;
  metadataCompleted?: number;
  analysisPending?: number;
  analysisCompleted?: number;
};

type OwnerScanSummary = NonNullable<Library['scanSummary']> & {
  mode?: LibraryScanMode;
  phase?: string;
  trigger?: string;
  warnings?: string[];
  roots?: ScanRootState[];
  counts?: ScanWorkCounts;
  lastRunAt?: string;
  nextRunAt?: string;
  startedAt?: string;
  completedAt?: string;
  degradedRootCount?: number;
  confirmedMissingCount?: number;
  ambiguousCount?: number;
  reconciliationReviewRequired?: boolean;
  absenceAuthoritative?: boolean;
  cleanupAllowed?: boolean;
};

const scanModeLabels: Record<LibraryScanMode, string> = {
  targeted: 'Scan changes',
  quick: 'Quick scan',
  reconcile: 'Reconcile library',
  force_full: 'Force full scan',
  remove_missing: 'Remove missing items',
};

function scanSummary(library: Library): OwnerScanSummary | undefined {
  return library.scanSummary as OwnerScanSummary | undefined;
}

function operationScanSummary(library: Library, response?: LibraryScanOperationsResponse): OwnerScanSummary | undefined {
  if (!response) return scanSummary(library);
  const operation = response.operation;
  const run = response.lastRun;
  if (!operation && !run) return scanSummary(library);
  return {
    jobId: operation?.jobId ?? run?.jobId,
    status: (operation?.status ?? run?.status ?? 'completed') as OwnerScanSummary['status'],
    progress: operation?.progress ?? (run ? 100 : 0),
    message: operation?.message,
    createdAt: operation?.createdAt ?? run?.startedAt,
    updatedAt: operation?.updatedAt ?? run?.updatedAt,
    mode: operation?.mode ?? run?.mode,
    phase: operation?.phase.label ?? run?.phase.label,
    trigger: operation?.trigger,
    warnings: run?.warnings.map((warning) => warning.message),
    roots: run?.roots.map((root) => ({ id: root.sourceId, path: root.configuredPath, status: root.status, error: root.errorMessage, errorClass: root.errorClass }))
      ?? response.sources.map((source) => ({ id: source.id, path: source.configuredPath, status: source.health, error: source.errorMessage, errorClass: source.errorClass, latencyMillis: source.latencyMs })),
    counts: run ? {
      processed: run.filesIndexed + run.filesSkipped,
      indexed: run.filesIndexed,
      unchanged: run.filesUnchanged,
      skipped: run.filesSkipped,
      missing: run.missingMarked,
      metadataPending: run.metadataQueued,
      analysisPending: run.analysisQueued,
    } : undefined,
    lastRunAt: response.lastRunAt,
    nextRunAt: response.nextRunAt,
    startedAt: run?.startedAt,
    completedAt: run?.completedAt,
    degradedRootCount: run?.roots.filter((root) => root.status !== 'healthy').length
      ?? response.sources.filter((source) => source.health !== 'healthy').length,
    confirmedMissingCount: run?.missingMarked,
    reconciliationReviewRequired: Boolean(run && (!run.absenceAuthoritative || !run.cleanupAllowed)),
    absenceAuthoritative: run?.absenceAuthoritative,
    cleanupAllowed: run?.cleanupAllowed,
  };
}

function formatScanTime(value?: string) {
  if (!value) return 'Not scheduled';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function formatCount(value?: number) {
  return (value ?? 0).toLocaleString();
}

function phaseLabel(summary?: OwnerScanSummary) {
  const phase = summary?.phase?.trim().replaceAll('_', ' ');
  if (phase) return phase.charAt(0).toUpperCase() + phase.slice(1);
  return scanStateLabel(summary?.status);
}

function rootProblem(root: ScanRootState) {
  return root.warning || root.error || (root.errorClass ? `Storage reported ${root.errorClass}.` : 'This source did not complete a healthy traversal.');
}

function isActiveScan(summary?: OwnerScanSummary) {
  return summary?.status === 'queued' || summary?.status === 'running';
}

function isCompletedScan(summary?: OwnerScanSummary) {
  const status = summary?.status as string | undefined;
  return status === 'complete' || status === 'completed';
}

const scanOperationsConcurrency = 4;

async function loadScanOperations(
  targets: Library[],
  source: SettingsDataSource,
  signal: AbortSignal,
): Promise<Map<string, LibraryScanOperationsResponse>> {
  const uniqueTargets = [...new Map(targets.map((library) => [library.id, library])).values()];
  const responses = new Map<string, LibraryScanOperationsResponse>();
  let cursor = 0;
  const worker = async () => {
    while (!signal.aborted) {
      const library = uniqueTargets[cursor++];
      if (!library) return;
      try {
        responses.set(library.id, await source.libraryScanOperations(library.id, signal));
      } catch {
        if (signal.aborted) return;
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(scanOperationsConcurrency, uniqueTargets.length) }, () => worker()));
  return responses;
}

function uniquePaths(paths: string[]) {
  return paths.reduce<string[]>((result, rawPath) => {
    const path = rawPath.trim();
    if (path && !result.some((candidate) => sameFilesystemPath(candidate, path))) result.push(path);
    return result;
  }, []);
}

function initialLibraryPaths(library?: Library) {
  const paths = uniquePaths(library?.paths?.length ? library.paths : library?.path ? [library.path] : []);
  return paths.length > 0 ? paths : [''];
}

type PickerTarget = { kind: 'add' } | { kind: 'replace'; index: number };

interface LibrarySaveResult {
  library: Library;
  created: boolean;
  scanJob?: Job;
  scanError?: string;
}

interface LibraryEditorProps {
  source: SettingsDataSource;
  library?: Library;
  filesystemSource?: FilesystemPickerSource;
  canBrowseFilesystem: boolean;
  onDismiss: () => void;
  onSaved: (result: LibrarySaveResult) => void;
}

function LibraryEditor({
  source,
  library,
  filesystemSource,
  canBrowseFilesystem,
  onDismiss,
  onSaved,
}: LibraryEditorProps) {
  const [name, setName] = useState(library?.name ?? '');
  const [type, setType] = useState<LibraryMutationInput['type']>(library?.type ?? 'movie');
  const [paths, setPaths] = useState(() => initialLibraryPaths(library));
  const [pickerTarget, setPickerTarget] = useState<PickerTarget | null>(null);
  const [pathNotice, setPathNotice] = useState('');
  const [error, setError] = useState('');
  const [savePhase, setSavePhase] = useState<'idle' | 'saving' | 'scanning'>('idle');
  const nameInput = useRef<HTMLInputElement>(null);
  const mutation = useAbortableMutation();
  const browseAvailable = canBrowseFilesystem && Boolean(filesystemSource);
  const dismiss = mutation.busy ? () => undefined : onDismiss;

  const updatePath = (index: number, value: string) => {
    setPaths((current) => current.map((path, pathIndex) => pathIndex === index ? value : path));
    setPathNotice('');
    setError('');
  };

  const removePath = (index: number) => {
    setPaths((current) => current.length === 1 ? [''] : current.filter((_, pathIndex) => pathIndex !== index));
    setPathNotice('');
    setError('');
  };

  const selectServerPath = (absolutePath: string) => {
    if (!pickerTarget) return;
    const populated = uniquePaths(paths);
    if (pickerTarget.kind === 'add') {
      if (populated.some((candidate) => sameFilesystemPath(candidate, absolutePath))) {
        setPathNotice('That server folder is already included.');
        setPaths(populated.length > 0 ? populated : ['']);
      } else {
        setPathNotice('Server folder added.');
        setPaths([...populated, absolutePath]);
      }
    } else {
      const replaced = paths.map((path, index) => index === pickerTarget.index ? absolutePath : path);
      const result = uniquePaths(replaced);
      setPathNotice(result.length < replaced.filter((path) => path.trim()).length
        ? 'That server folder was already included, so Portico kept one source.'
        : 'Server folder replaced.');
      setPaths(result.length > 0 ? result : ['']);
    }
    setError('');
    setPickerTarget(null);
  };

  const submit = async () => {
    const populated = paths.map((path) => path.trim()).filter(Boolean);
    const values = uniquePaths(populated);
    if (!name.trim()) {
      setError('Enter a library name.');
      return;
    }
    if (values.length === 0) {
      setError('Add at least one media folder.');
      return;
    }
    if (values.some((path) => !isAbsoluteFilesystemPath(path))) {
      setError('Every media folder must be an absolute server path.');
      return;
    }
    if (values.length !== populated.length) setPathNotice('Duplicate folders were combined before saving.');
    setPaths(values);
    const input: LibraryMutationInput = {
      name: name.trim(),
      type,
      path: values[0],
      paths: values,
      settings: library?.settings ?? {},
    };
    setError('');
    setSavePhase('saving');
    try {
      const result = await mutation.run<LibrarySaveResult>(async (signal) => {
        if (library) return { library: await source.updateLibrary(library.id, input, signal), created: false };
        const created = await source.createLibrary(input, signal);
        setSavePhase('scanning');
        try {
          return { library: created, created: true, scanJob: await source.scanLibrary(created.id, signal) };
        } catch (reason) {
          const message = signal.aborted
            ? 'The initial scan was cancelled.'
            : productProblemText(reason, 'settings.action-failed', { actionName: 'queue the initial scan' });
          return { library: created, created: true, scanError: message };
        }
      });
      setSavePhase('idle');
      onSaved(result);
    } catch (reason) {
      setSavePhase('idle');
      setError(productProblemText(reason, 'settings.action-failed', { actionName: 'save this library' }));
    }
  };

  const heading = library ? `Edit ${library.name}` : 'New library';
  const pickerInitialPath = pickerTarget?.kind === 'replace'
    ? paths[pickerTarget.index]?.trim()
    : uniquePaths(paths)[0];

  return <>
    <ModalOverlay labelledBy="portico-library-editor-title" className="portico-settings-dialog portico-library-dialog" initialFocusRef={nameInput} onDismiss={dismiss}>
      <header>
        <div><h2 id="portico-library-editor-title">{heading}</h2><p>Library identity and server media folders</p></div>
        <IconButton label="Close" disabled={mutation.busy} onClick={dismiss}><X /></IconButton>
      </header>
      <div className="portico-settings-dialog-fields">
        <div className="portico-library-identity-fields">
          <label><span>Name</span><input ref={nameInput} aria-label="Library name" value={name} disabled={mutation.busy} onChange={(event) => setName(event.target.value)} /></label>
          <div>
            <span>Library type</span>
            <ChoiceControl
              label="Library type"
              value={type}
              options={[
                { value: 'movie', label: 'Movies' },
                { value: 'show', label: 'TV Shows' },
                { value: 'anime', label: 'Anime' },
                { value: 'music', label: 'Music' },
                { value: 'audiobook', label: 'Audiobooks' },
                { value: 'recorded-tv', label: 'Recorded TV' },
              ]}
              disabled={mutation.busy}
              onChange={(next) => setType(next as LibraryMutationInput['type'])}
            />
          </div>
        </div>
        <fieldset className="portico-library-path-editor">
          <legend>Media folders</legend>
          <div className="portico-library-path-heading">
            <p>Absolute paths on the Portico server host.</p>
          </div>
          <div className="portico-library-path-rows">
            {paths.map((path, index) => <div className="portico-library-path-row" key={`${index}:${paths.length}`}>
              <span aria-hidden="true">{index + 1}</span>
              <input
                aria-label={`Media folder ${index + 1}`}
                value={path}
                disabled={mutation.busy}
                onChange={(event) => updatePath(index, event.target.value)}
                placeholder={type === 'music' ? '/media/music' : type === 'movie' ? '/media/movies' : '/media/tv'}
                spellCheck={false}
                autoComplete="off"
              />
              {browseAvailable && <SecondaryButton disabled={mutation.busy} onClick={() => setPickerTarget({ kind: 'replace', index })}>{path.trim() ? 'Replace' : 'Browse'}</SecondaryButton>}
              <IconButton label={`Remove media folder ${index + 1}`} disabled={mutation.busy || (paths.length === 1 && !path.trim())} onClick={() => removePath(index)}><Trash2 /></IconButton>
            </div>)}
          </div>
          <div className="portico-library-path-footer">
            <SecondaryButton disabled={mutation.busy} onClick={() => setPaths((current) => [...current, ''])}><Plus /> Add path manually</SecondaryButton>
            {browseAvailable && <SecondaryButton disabled={mutation.busy} onClick={() => setPickerTarget({ kind: 'add' })}><FolderOpen /> Browse server</SecondaryButton>}
            {pathNotice && <p role="status">{pathNotice}</p>}
          </div>
        </fieldset>
        {error && <p className="portico-settings-dialog-error" role="alert"><AlertTriangle />{error}</p>}
      </div>
      <footer>
        <SecondaryButton disabled={mutation.busy} onClick={dismiss}>Cancel</SecondaryButton>
        <PrimaryButton disabled={mutation.busy} onClick={() => void submit()}>{savePhase === 'scanning' ? 'Starting scan…' : savePhase === 'saving' ? (library ? 'Saving library…' : 'Creating library…') : library ? 'Save library' : 'Create library'}</PrimaryButton>
      </footer>
    </ModalOverlay>
    {pickerTarget && filesystemSource && <FilesystemPickerDialog
      source={filesystemSource}
      initialPath={pickerInitialPath}
      title={pickerTarget.kind === 'add' ? 'Add server folder' : 'Replace server folder'}
      description="Choose a folder that contains media for this library."
      confirmLabel={pickerTarget.kind === 'add' ? 'Add folder' : 'Replace folder'}
      canCreateDirectory={canBrowseFilesystem}
      onCancel={() => setPickerTarget(null)}
      onSelect={selectServerPath}
    />}
  </>;
}

interface LibraryOperationsProps {
  libraries: Library[];
  source: SettingsDataSource;
  canManage: boolean;
  filesystemSource?: FilesystemPickerSource;
  canBrowseFilesystem?: boolean;
  onChanged: () => void;
}

function RemoveMissingDialog({
  library,
  summary,
  review,
  loading,
  reviewError,
  loadingMore,
  onLoadMore,
  onResolveIdentity,
  busy,
  onDismiss,
  onConfirm,
}: {
  library: Library;
  summary?: OwnerScanSummary;
  review?: LibraryScanReviewResponse;
  loading: boolean;
  reviewError?: string;
  loadingMore: boolean;
  onLoadMore: () => void;
  onResolveIdentity: (reviewId: string, resolution: 'keep_separate' | 'merge_into_candidate', candidateId?: string) => void;
  busy: boolean;
  onDismiss: () => void;
  onConfirm: () => void;
}) {
  const [confirmation, setConfirmation] = useState('');
  const [selectedCandidates, setSelectedCandidates] = useState<Record<string, string>>({});
  const confirmedRunId = review?.confirmationRunId;
  const blocked = !review?.canConfirmRemoval || !confirmedRunId;
  const missing = review?.missingTotal;
  return <ModalOverlay labelledBy="portico-remove-missing-title" className="portico-settings-dialog portico-scan-confirm-dialog" onDismiss={busy ? () => undefined : onDismiss}>
    <header>
      <div><h2 id="portico-remove-missing-title">Remove missing items</h2><p>Review reconciliation evidence for {library.name}</p></div>
      <IconButton label="Close" disabled={busy} onClick={onDismiss}><X /></IconButton>
    </header>
    <div className="portico-settings-dialog-fields portico-scan-reconciliation-review">
      <InlineNotice tone={blocked ? 'error' : 'warn'}>
        <strong>{loading ? 'Loading reconciliation review…' : blocked ? 'Removal is blocked.' : `${missing ?? 'Confirmed'} missing ${missing === 1 ? 'item' : 'items'} will be removed.`}</strong>{' '}
        {loading ? 'Portico is loading the latest bounded reconciliation evidence.' : blocked
          ? 'The server has not authorized removal from the latest reconciliation evidence. Resolve identity questions and run a healthy reconciliation first.'
          : 'Portico will remove database entries only. It will not delete source media files.'}
      </InlineNotice>
      {reviewError && <InlineNotice tone="error"><strong>Review unavailable.</strong> {reviewError}</InlineNotice>}
      {(review?.openIdentityTotal ?? 0) > 0 && <InlineNotice tone="warn"><strong>Identity review required.</strong> {review?.openIdentityTotal} ambiguous {review?.openIdentityTotal === 1 ? 'match needs' : 'matches need'} review before destructive reconciliation.</InlineNotice>}
      <dl>
        <div><dt>Removal evidence</dt><dd>{review?.canConfirmRemoval ? 'Server-authorized' : 'Not authorized'}</dd></div>
        <div><dt>Confirmed missing</dt><dd>{missing ?? (loading ? 'Loading…' : 'Unknown')}</dd></div>
        <div><dt>Degraded roots</dt><dd>{summary?.degradedRootCount ?? summary?.roots?.filter((root) => root.status !== 'healthy').length ?? 0}</dd></div>
        <div><dt>Open identity reviews</dt><dd>{review?.openIdentityTotal ?? 0}</dd></div>
        <div><dt>Evidence run</dt><dd>{confirmedRunId ?? 'Awaiting scan evidence'}</dd></div>
      </dl>
      {(review?.missingItems.length ?? 0) > 0 && <section className="portico-scan-missing-review" aria-label="Missing item evidence">
        <header><strong>Missing-item evidence</strong><small>Showing {review!.missingItems.length} of {review!.missingTotal}{review!.hasMore ? ' — more remain' : ''}</small></header>
        <div>{review!.missingItems.map((item) => <article key={item.fileId}>
          <span><strong>{item.title}</strong><small>{item.path}</small></span>
          <span><time dateTime={item.missingSince}>{formatScanTime(item.missingSince)}</time><small className={item.sourceHealth && item.sourceHealth !== 'healthy' ? 'warning' : ''}>{item.sourceHealth || 'Unknown source health'}</small></span>
        </article>)}</div>
        {review!.hasMore && <footer><SecondaryButton disabled={loadingMore} onClick={onLoadMore}>{loadingMore ? 'Loading…' : 'Load more missing items'}</SecondaryButton></footer>}
      </section>}
      {(review?.identityReviews.length ?? 0) > 0 && <section className="portico-scan-identity-review" aria-label="Open identity review summary">
        <header><strong>Open identity review</strong><small>{review!.openIdentityTotal} total</small></header>
        {review!.identityReviews.slice(0, 5).map((item) => <article key={item.id}>
          <p><strong>{item.candidateLocator || item.subjectId}</strong><span>{item.evidenceKind}: {item.evidenceValue} · subject {item.subjectId}</span></p>
          <fieldset><legend>Candidate identities</legend>{item.candidateIds.map((candidateId) => <label key={candidateId}><input type="radio" name={`identity-${item.id}`} value={candidateId} checked={selectedCandidates[item.id] === candidateId} disabled={busy} onChange={() => setSelectedCandidates((current) => ({ ...current, [item.id]: candidateId }))} /><span><strong>{candidateId}</strong><small>Existing {item.domain} identity eligible for this match</small></span></label>)}</fieldset>
          <footer><SecondaryButton disabled={busy} onClick={() => onResolveIdentity(item.id, 'keep_separate')}>Ignore match · keep separate</SecondaryButton><PrimaryButton disabled={busy || !selectedCandidates[item.id]} onClick={() => onResolveIdentity(item.id, 'merge_into_candidate', selectedCandidates[item.id])}>Merge into selected</PrimaryButton></footer>
        </article>)}
      </section>}
      {!blocked && <label className="portico-scan-confirm-field"><span>Type <strong>REMOVE</strong> to confirm</span><input aria-label="Remove missing confirmation" value={confirmation} disabled={busy} autoComplete="off" onChange={(event) => setConfirmation(event.target.value)} /></label>}
    </div>
    <footer>
      <SecondaryButton disabled={busy} onClick={onDismiss}>Cancel</SecondaryButton>
      <button type="button" className="button danger" disabled={busy || blocked || confirmation !== 'REMOVE'} onClick={onConfirm}>{busy ? 'Removing…' : 'Remove missing items'}</button>
    </footer>
  </ModalOverlay>;
}

export function LibraryOperations({
  libraries,
  source,
  canManage,
  filesystemSource,
  canBrowseFilesystem = false,
  onChanged,
}: LibraryOperationsProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [editor, setEditor] = useState<Library | 'new' | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string>('');
  const [expandedScans, setExpandedScans] = useState<Set<string>>(() => new Set());
  const [removeMissingLibrary, setRemoveMissingLibrary] = useState<Library>();
  const [scanReviews, setScanReviews] = useState<Record<string, LibraryScanReviewResponse>>({});
  const [scanReviewLoading, setScanReviewLoading] = useState(false);
  const [scanReviewLoadingMore, setScanReviewLoadingMore] = useState(false);
  const [scanReviewError, setScanReviewError] = useState('');
  const scanReviewAbort = useRef<AbortController | null>(null);
  const scanOperationsGeneration = useRef(0);
  const scanOperationsRequest = useRef<Promise<Map<string, LibraryScanOperationsResponse> | undefined> | undefined>(undefined);
  const scanOperationsRequestAbort = useRef<AbortController | undefined>(undefined);
  const scanOperationsRef = useRef<Record<string, LibraryScanOperationsResponse>>({});
  const wakeScanPolling = useRef<(targets?: Library[]) => void>(() => undefined);
  const [scanOperations, setScanOperations] = useState<Record<string, LibraryScanOperationsResponse>>({});
  scanOperationsRef.current = scanOperations;
  const [feedback, setFeedback] = useState('');
  const [error, setError] = useState('');
  const [createdState, setCreatedState] = useState<LibrarySaveResult>();
  const mutation = useAbortableMutation();
  const openNewLibrary = useCallback(() => {
    setCreatedState(undefined);
    setFeedback('');
    setError('');
    setEditor('new');
  }, []);

  useEffect(() => {
    if (searchParams.get('newLibrary') !== '1') return;
    const next = new URLSearchParams(searchParams);
    next.delete('newLibrary');
    setSearchParams(next, { replace: true });
    if (canManage) openNewLibrary();
  }, [canManage, openNewLibrary, searchParams, setSearchParams]);

  const displayedLibraries = createdState && !libraries.some((library) => library.id === createdState.library.id)
    ? [...libraries, createdState.library]
    : libraries;

  const refreshScanOperations = useCallback(async (controller: AbortController, targets: Library[] = libraries): Promise<Map<string, LibraryScanOperationsResponse> | undefined> => {
    const signal = controller.signal;
    const pending = scanOperationsRequest.current;
    if (pending && !scanOperationsRequestAbort.current?.signal.aborted) return pending;
    if (pending) return pending.then(() => refreshScanOperations(controller, targets));
    const generation = ++scanOperationsGeneration.current;
    const request = (async () => {
      const responses = await loadScanOperations(targets, source, signal);
      if (signal.aborted || generation !== scanOperationsGeneration.current) return undefined;
      const next = { ...scanOperationsRef.current };
      responses.forEach((response, libraryId) => { next[libraryId] = response; });
      scanOperationsRef.current = next;
      setScanOperations(next);
      return responses;
    })();
    const tracked = request.finally(() => {
      if (scanOperationsRequest.current !== tracked) return;
      scanOperationsRequest.current = undefined;
      if (scanOperationsRequestAbort.current === controller) scanOperationsRequestAbort.current = undefined;
    });
    scanOperationsRequest.current = tracked;
    scanOperationsRequestAbort.current = controller;
    return tracked;
  }, [libraries, source]);

  useEffect(() => {
    const controller = new AbortController();
    let timer: number | undefined;
    let disposed = false;
    const clearTimer = () => {
      if (timer === undefined) return;
      window.clearTimeout(timer);
      timer = undefined;
    };
    const poll = async (targets: Library[]) => {
      if (disposed || targets.length === 0) return;
      const responses = await refreshScanOperations(controller, targets);
      if (disposed || controller.signal.aborted) return;
      const active = libraries.filter((library) => {
        const response = responses?.get(library.id) ?? scanOperationsRef.current[library.id];
        return isActiveScan(operationScanSummary(library, response));
      });
      clearTimer();
      if (active.length > 0) timer = window.setTimeout(() => {
        timer = undefined;
        void poll(active);
      }, 8000);
    };
    const wake = (targets?: Library[]) => {
      clearTimer();
      void poll(targets?.length ? targets : libraries);
    };
    wakeScanPolling.current = wake;
    void poll(libraries);
    return () => {
      disposed = true;
      controller.abort();
      scanOperationsGeneration.current += 1;
      scanOperationsRequestAbort.current?.abort();
      clearTimer();
      if (wakeScanPolling.current === wake) wakeScanPolling.current = () => undefined;
    };
  }, [libraries, refreshScanOperations]);

  const scan = async (library: Library, mode: LibraryScanMode = 'reconcile', confirmedRunId?: string) => {
    setCreatedState(undefined);
    setFeedback('');
    setError('');
    try {
      const job = await mutation.run((signal) => source.scanLibrary(library.id, signal, mode, confirmedRunId));
      const currentSummary = operationScanSummary(library, scanOperations[library.id]);
      setFeedback(isActiveScan(currentSummary) && currentSummary?.jobId === job.id
        ? `${library.name} already has a scan in progress. Portico updated that operation with the ${scanModeLabels[mode].toLocaleLowerCase()} request.`
        : `${scanModeLabels[mode]} queued for ${library.name}.`);
      setExpandedScans((current) => new Set(current).add(library.id));
      const controller = new AbortController();
      await refreshScanOperations(controller, [library]);
      wakeScanPolling.current([library]);
      onChanged();
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `scan ${library.name}` }));
    }
  };
  const retryScan = async (library: Library, runId?: string) => {
    setFeedback('');
    setError('');
    try {
      await mutation.run((signal) => source.retryLibraryScan(library.id, runId, signal));
      setFeedback(`${library.name} scan retry queued.`);
      setExpandedScans((current) => new Set(current).add(library.id));
      const controller = new AbortController();
      await refreshScanOperations(controller, [library]);
      wakeScanPolling.current([library]);
      onChanged();
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `retry the ${library.name} scan` }));
    }
  };
  const openScanReview = (library: Library) => {
    scanReviewAbort.current?.abort();
    setRemoveMissingLibrary(library);
    setScanReviewLoading(true);
    setScanReviewLoadingMore(false);
    setScanReviewError('');
    setScanReviews((current) => {
      const next = { ...current };
      delete next[library.id];
      return next;
    });
    const controller = new AbortController();
    scanReviewAbort.current = controller;
    void source.libraryScanReview(library.id, undefined, controller.signal).then((review) => {
      if (controller.signal.aborted) return;
      setScanReviews((current) => ({ ...current, [library.id]: review }));
    }).catch((reason) => {
      if (controller.signal.aborted) return;
      setScanReviewError(productProblemText(reason, 'settings.action-failed', { actionName: `load reconciliation evidence for ${library.name}` }));
    }).finally(() => { if (!controller.signal.aborted) setScanReviewLoading(false); });
  };

  const loadMoreScanReview = (library: Library) => {
    const current = scanReviews[library.id];
    if (!current?.hasMore || !current.nextCursor || scanReviewLoadingMore) return;
    scanReviewAbort.current?.abort();
    const controller = new AbortController();
    scanReviewAbort.current = controller;
    setScanReviewLoadingMore(true);
    setScanReviewError('');
    void source.libraryScanReview(library.id, current.nextCursor, controller.signal).then((page) => {
      if (controller.signal.aborted) return;
      setScanReviews((reviews) => {
        const previous = reviews[library.id] ?? current;
        if (page.confirmationRunId !== previous.confirmationRunId) {
          return { ...reviews, [library.id]: page };
        }
        const missing = new Map(previous.missingItems.map((item) => [item.fileId, item]));
        page.missingItems.forEach((item) => missing.set(item.fileId, item));
        const identities = new Map(previous.identityReviews.map((item) => [item.id, item]));
        page.identityReviews.forEach((item) => identities.set(item.id, item));
        return { ...reviews, [library.id]: { ...previous, ...page, confirmationRunId: page.confirmationRunId ?? previous.confirmationRunId, missingItems: [...missing.values()], identityReviews: [...identities.values()] } };
      });
    }).catch((reason) => {
      if (controller.signal.aborted) return;
      setScanReviewError(productProblemText(reason, 'settings.action-failed', { actionName: `load more reconciliation evidence for ${library.name}` }));
    }).finally(() => { if (!controller.signal.aborted) setScanReviewLoadingMore(false); });
  };

  const updateStorageClassification = async (library: Library, sourceId: string, classification: 'local' | 'network' | 'fuse' | 'unknown') => {
    setFeedback('');
    setError('');
    try {
      await mutation.run((signal) => source.updateLibraryStorageClassification(library.id, sourceId, classification, signal));
      setFeedback(`${library.name} storage classification saved.`);
      const controller = new AbortController();
      await refreshScanOperations(controller, [library]);
      wakeScanPolling.current([library]);
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `classify storage for ${library.name}` }));
    }
  };

  const resolveIdentityReview = async (library: Library, reviewId: string, resolution: 'keep_separate' | 'merge_into_candidate', candidateId?: string) => {
    setScanReviewError('');
    try {
      await mutation.run((signal) => source.resolveIdentityReconciliationReview(reviewId, resolution, candidateId, signal));
      scanReviewAbort.current?.abort();
      const controller = new AbortController();
      scanReviewAbort.current = controller;
      setScanReviewLoading(true);
      const review = await source.libraryScanReview(library.id, undefined, controller.signal);
      if (controller.signal.aborted) return;
      setScanReviews((current) => ({ ...current, [library.id]: review }));
      setFeedback(resolution === 'keep_separate' ? 'Identity kept separate.' : 'Identity merged into the selected candidate.');
    } catch (reason) {
      setScanReviewError(productProblemText(reason, 'settings.action-failed', { actionName: 'resolve this identity review' }));
    } finally {
      setScanReviewLoading(false);
    }
  };

  useEffect(() => () => scanReviewAbort.current?.abort(), []);
  const cancelScan = async (library: Library) => {
    setFeedback('');
    setError('');
    try {
      await mutation.run((signal) => source.cancelLibraryScan(library.id, signal));
      setFeedback(`${library.name} scan cancellation requested.`);
      const controller = new AbortController();
      await refreshScanOperations(controller, [library]);
      wakeScanPolling.current([library]);
      onChanged();
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `cancel the ${library.name} scan` }));
    }
  };
  const remove = async (library: Library) => {
    setCreatedState(undefined);
    setFeedback('');
    setError('');
    try {
      await mutation.run((signal) => source.deleteLibrary(library.id, signal));
      setConfirmDelete('');
      setFeedback(`${library.name} removed. Source files were not deleted.`);
      onChanged();
    } catch (reason) {
      setError(productProblemText(reason, 'settings.action-failed', { actionName: `remove ${library.name}` }));
    }
  };
  return <SettingsGroup title="Libraries" description="Media roots, scan state, and library ownership." actions={canManage && displayedLibraries.length > 0 ? <PrimaryButton onClick={openNewLibrary}><Plus /> New library</PrimaryButton> : undefined}>
    {(feedback || error) && <InlineNotice tone={error ? 'error' : 'success'}>{error || feedback}</InlineNotice>}
    {createdState && <InlineNotice
      tone={createdState.scanError ? 'warn' : 'success'}
      action={<Link className="button secondary" to={libraryRoute(createdState.library)}>Open library</Link>}
    ><strong>{createdState.library.name} created.</strong> {createdState.scanError
      ? `Portico could not queue its initial scan. ${createdState.scanError}`
      : `Initial scan ${createdState.scanJob?.status ?? 'queued'}.`}</InlineNotice>}
    {displayedLibraries.length === 0
      ? canManage
        ? <section className="portico-first-library">
          <div className="portico-first-library-heading"><FolderOpen /><span><strong>Add your first library</strong><p>Portico indexes media where it already lives on this server.</p></span></div>
          <ol>
            <li><span>1</span><div><strong>Name the library</strong><p>Choose Movies, TV, Music, or another matching media type.</p></div></li>
            <li><span>2</span><div><strong>Add media folders</strong><p>Browse server storage or enter one or more absolute paths.</p></div></li>
            <li><span>3</span><div><strong>Start the first scan</strong><p>Creating the library queues a real scan and reports its server state here.</p></div></li>
          </ol>
          <div className="portico-first-library-action"><PrimaryButton onClick={openNewLibrary}><Plus /> Create first library</PrimaryButton><small>Source files stay in place.</small></div>
        </section>
        : <div className="portico-settings-state"><FolderOpen /><strong>No libraries are shared with this account</strong><p>The server owner can add a library or grant this account access to an existing one.</p></div>
      : <div className="portico-library-list">{displayedLibraries.map((library) => {
        const justCreated = createdState?.library.id === library.id ? createdState : undefined;
        const operations = scanOperations[library.id];
        const summary = operationScanSummary(library, operations);
        const scanLabel = justCreated?.scanError ? 'Scan not started' : scanStateLabel(justCreated?.scanJob?.status ?? summary?.status);
        const expanded = expandedScans.has(library.id);
        const degradedRoots = summary?.roots?.filter((root) => root.status && root.status !== 'healthy') ?? [];
        const warnings = [...(summary?.warnings ?? []), ...degradedRoots.map(rootProblem)];
        const active = isActiveScan(summary);
        const canRetry = operations?.actions.canRetry ?? (summary?.status === 'failed' || summary?.status === 'cancelled');
        const progress = Math.max(0, Math.min(100, summary?.progress ?? 0));
        return <article className={`portico-library-operation ${expanded ? 'expanded' : ''}`} key={library.id}>
        <div className="portico-library-operation-heading">
          <Link to={libraryRoute(library)}>
            <span className="portico-library-icon"><LibraryIcon type={library.type} /></span>
            <span><strong>{library.name}</strong><small>{library.count} {library.count === 1 ? 'item' : 'items'} · {library.paths.length || (library.path ? 1 : 0)} {(library.paths.length || (library.path ? 1 : 0)) === 1 ? 'folder' : 'folders'} · {scanLabel.toLocaleLowerCase()}</small></span>
          </Link>
          <div className="portico-library-operation-actions">
            {justCreated
              ? <span className={`portico-settings-capability ${justCreated.scanError ? 'warning' : 'configured'}`}>{justCreated.scanError ? <AlertTriangle /> : ['complete', 'completed'].includes(justCreated.scanJob?.status ?? '') ? <CheckCircle2 /> : <RefreshCw className="portico-settings-spinner" />} {scanLabel}</span>
              : active ? <span className="portico-settings-capability configured"><RefreshCw className="portico-settings-spinner" /> {phaseLabel(summary)}</span>
                : isCompletedScan(summary) ? <span className="portico-settings-capability configured"><CheckCircle2 /> Complete</span>
                  : summary?.status === 'failed' ? <span className="portico-settings-capability warning"><AlertTriangle /> Failed</span>
                    : summary?.status === 'cancelled' ? <span className="portico-settings-capability"><Ban /> Cancelled</span> : null}
            {active && summary?.jobId && (operations?.actions.canCancel ?? true)
              ? <SecondaryButton disabled={mutation.busy} onClick={() => void cancelScan(library)}><Ban /> Cancel</SecondaryButton>
              : canRetry
                ? <SecondaryButton disabled={mutation.busy} onClick={() => void retryScan(library, operations?.lastRun?.id)}><RotateCcw /> Retry</SecondaryButton>
                : <SecondaryButton disabled={mutation.busy || active} onClick={() => void scan(library, 'quick')}><ScanSearch /> Quick scan</SecondaryButton>}
            <IconButton label={`${expanded ? 'Hide' : 'Show'} scan details for ${library.name}`} onClick={() => setExpandedScans((current) => {
              const next = new Set(current);
              if (next.has(library.id)) next.delete(library.id); else next.add(library.id);
              return next;
            })}>{expanded ? <ChevronUp /> : <ChevronDown />}</IconButton>
          {canManage && <IconButton label={`Edit ${library.name}`} onClick={() => { setCreatedState(undefined); setEditor(library); }}><Pencil /></IconButton>}
          {canManage && (confirmDelete === library.id
            ? <div className="portico-inline-confirm"><span>Remove {library.name} from Portico? Its library records and configuration will be removed; media files remain on disk.</span><button type="button" onClick={() => setConfirmDelete('')}>Cancel</button><button type="button" className="danger" disabled={mutation.busy} onClick={() => void remove(library)}>Remove library</button></div>
            : <IconButton label={`Remove ${library.name}`} onClick={() => setConfirmDelete(library.id)}><Trash2 /></IconButton>)}
          </div>
        </div>
        {expanded && <section className="portico-library-scan-panel" aria-label={`${library.name} scan details`}>
          <div className="portico-library-scan-current">
            <div className="portico-library-scan-phase">
              <span>{summary?.mode ? scanModeLabels[summary.mode] : 'Library scan'}</span>
              <strong>{phaseLabel(summary)}</strong>
              {summary?.message && <p>{summary.message}</p>}
            </div>
            {active && <div className="portico-library-scan-progress">
              <span><span>{progress}%</span><span>{summary?.phase ? phaseLabel(summary) : 'In progress'}</span></span>
              <progress aria-label={`${library.name} scan progress`} max="100" value={progress}>{progress}%</progress>
            </div>}
            <dl className="portico-library-scan-timing">
              <div><dt>Last run</dt><dd><Clock3 /> {formatScanTime(summary?.completedAt ?? summary?.lastRunAt ?? (isCompletedScan(summary) ? summary?.updatedAt : undefined))}</dd></div>
              <div><dt>Next run</dt><dd><Clock3 /> {formatScanTime(summary?.nextRunAt)}</dd></div>
            </dl>
          </div>
          {warnings.length > 0 && <div className="portico-library-scan-warnings" role="status">
            <AlertTriangle />
            <div><strong>{degradedRoots.length > 0 ? `${degradedRoots.length} ${degradedRoots.length === 1 ? 'source needs' : 'sources need'} attention` : 'Scan warning'}</strong>
              {warnings.slice(0, 4).map((warning, index) => <p key={`${warning}:${index}`}>{warning}</p>)}
              {degradedRoots.map((root, index) => <small key={root.id ?? root.path ?? index}>{root.path || `Source ${index + 1}`}{root.status ? ` · ${root.status}` : ''}{root.latencyMillis !== undefined ? ` · ${root.latencyMillis} ms` : ''}</small>)}
            </div>
          </div>}
          {canManage && (operations?.sources.length ?? 0) > 0 && <section className="portico-library-storage-sources" aria-label={`${library.name} storage sources`}>
            <header><strong>Storage behavior</strong><small>Override detection when Portico needs network- or FUSE-aware I/O limits.</small></header>
            {operations!.sources.map((storageSource) => <label key={storageSource.id}>
              <span><strong>{storageSource.configuredPath}</strong><small>{storageSource.health} · {storageSource.classificationSource === 'owner' ? 'Owner-selected' : 'Detected'} as {storageSource.classification}{storageSource.latencyMs >= 0 ? ` · ${storageSource.latencyMs} ms` : ''}</small></span>
              <select
                aria-label={`Storage classification for ${storageSource.configuredPath}`}
                value={storageSource.classification}
                disabled={mutation.busy}
                onChange={(event) => void updateStorageClassification(library, storageSource.id, event.target.value as 'local' | 'network' | 'fuse' | 'unknown')}
              >
                <option value="local">Local disk</option>
                <option value="network">Network drive</option>
                <option value="fuse">FUSE / rclone mount</option>
                <option value="unknown">Unknown / automatic safety</option>
              </select>
            </label>)}
          </section>}
          {summary?.counts && <div className="portico-library-scan-counts" aria-label="Subordinate scan work">
            <div><span>Files</span><strong>{formatCount(summary.counts.processed ?? summary.counts.discovered)}</strong><small>{formatCount(summary.counts.indexed ?? summary.counts.added)} indexed · {formatCount(summary.counts.unchanged ?? summary.counts.updated)} unchanged · {formatCount(summary.counts.skipped)} skipped</small></div>
            <div><span>Metadata</span><strong>{formatCount(summary.counts.metadataPending)} queued</strong><small>Managed beneath this library operation</small></div>
            <div><span>Analysis</span><strong>{formatCount(summary.counts.analysisPending)} queued</strong><small>Managed beneath this library operation</small></div>
            <div><span>Missing</span><strong>{formatCount(summary.confirmedMissingCount ?? summary.counts.missing)}</strong><small>{summary.absenceAuthoritative ? 'Authoritatively confirmed' : 'Awaiting healthy evidence'}</small></div>
          </div>}
          {canManage && <div className="portico-library-scan-controls" aria-label={`${library.name} scan actions`}>
            <span><strong>Start a scan</strong><small>One operation stays visible while Portico manages its metadata and analysis work underneath.</small></span>
            <SecondaryButton disabled={mutation.busy || active || operations?.actions.canTarget === false} onClick={() => void scan(library, 'targeted')}>Scan changes</SecondaryButton>
            <SecondaryButton disabled={mutation.busy || active || operations?.actions.canQuick === false} onClick={() => void scan(library, 'quick')}>Quick</SecondaryButton>
            <SecondaryButton disabled={mutation.busy || active || operations?.actions.canReconcile === false} onClick={() => void scan(library, 'reconcile')}>Reconcile</SecondaryButton>
            <SecondaryButton disabled={mutation.busy || active || operations?.actions.canForceFull === false} onClick={() => void scan(library, 'force_full')}>Force full</SecondaryButton>
            <button type="button" className="button danger subtle" disabled={mutation.busy || active} onClick={() => openScanReview(library)}>Review removal</button>
          </div>}
          {(summary?.reconciliationReviewRequired || (summary?.ambiguousCount ?? 0) > 0) && <InlineNotice tone="warn" action={<button type="button" className="button secondary" onClick={() => openScanReview(library)}>Review reconciliation</button>}><strong>Owner review required.</strong> Portico found ambiguous identity or absence evidence and will not remove items automatically.</InlineNotice>}
        </section>}
      </article>})}</div>}
    {editor && <LibraryEditor
      source={source}
      library={editor === 'new' ? undefined : editor}
      filesystemSource={filesystemSource}
      canBrowseFilesystem={canBrowseFilesystem}
      onDismiss={() => setEditor(null)}
      onSaved={(result) => {
        setEditor(null);
        if (result.created) {
          setCreatedState(result);
          setFeedback('');
        } else {
          setCreatedState(undefined);
          setFeedback('Library saved.');
          onChanged();
        }
      }}
    />}
    {removeMissingLibrary && <RemoveMissingDialog
      library={removeMissingLibrary}
      summary={operationScanSummary(removeMissingLibrary, scanOperations[removeMissingLibrary.id])}
      review={scanReviews[removeMissingLibrary.id]}
      loading={scanReviewLoading}
      reviewError={scanReviewError}
      loadingMore={scanReviewLoadingMore}
      onLoadMore={() => loadMoreScanReview(removeMissingLibrary)}
      onResolveIdentity={(reviewId, resolution, candidateId) => void resolveIdentityReview(removeMissingLibrary, reviewId, resolution, candidateId)}
      busy={mutation.busy}
      onDismiss={() => { scanReviewAbort.current?.abort(); setRemoveMissingLibrary(undefined); }}
      onConfirm={() => {
        const target = removeMissingLibrary;
        void scan(target, 'remove_missing', scanReviews[target.id]?.confirmationRunId).then(() => setRemoveMissingLibrary(undefined));
      }}
    />}
  </SettingsGroup>;
}
