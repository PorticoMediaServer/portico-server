import type { Library } from '@porticomediaserver/client-core';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { FixtureFilesystemSource, fixtureDirectory } from '../filesystem';
import { FixtureSettingsDataSource } from './FixtureSettingsDataSource';
import { LibraryOperations } from './LibraryOperations';
import { SettingsPage } from './SettingsPage';
import type { LibraryScanOperationsResponse, SettingsViewer } from './settingsTypes';

const roots = [
  { name: 'Media volume', path: '/srv/media' },
  { name: 'Archive NAS', path: '/mnt/archive' },
];

function filesystemSource(browseFailure?: unknown) {
  return new FixtureFilesystemSource({
    roots,
    defaultPath: '/srv/media',
    browseFailure,
    directories: [
      fixtureDirectory('/srv/media', [{ name: 'Movies' }, { name: 'TV Shows' }], roots),
      fixtureDirectory('/srv/media/Movies', [], roots),
      fixtureDirectory('/srv/media/TV Shows', [], roots),
      fixtureDirectory('/mnt/archive', [{ name: 'Classics' }], roots),
      fixtureDirectory('/mnt/archive/Classics', [], roots),
    ],
  });
}

function library(paths = ['/srv/media/Movies']): Library {
  return {
    id: 'fixture-movies',
    name: 'Movies',
    type: 'movie',
    count: 42,
    path: paths[0] ?? '',
    paths,
    settings: {},
    sortOrder: 0,
  };
}

function renderOperations(options: {
  libraries?: Library[];
  canManage?: boolean;
  canBrowseFilesystem?: boolean;
  filesystem?: FixtureFilesystemSource;
  initialEntry?: string;
  source?: FixtureSettingsDataSource;
} = {}) {
  const source = options.source ?? new FixtureSettingsDataSource();
  const onChanged = vi.fn();
  render(<MemoryRouter initialEntries={[options.initialEntry ?? '/settings/media']}><LibraryOperations
    libraries={options.libraries ?? []}
    source={source}
    canManage={options.canManage ?? true}
    filesystemSource={options.filesystem}
    canBrowseFilesystem={options.canBrowseFilesystem}
    onChanged={onChanged}
  /><LocationProbe /></MemoryRouter>);
  return { source, onChanged };
}

function LocationProbe() {
  const location = useLocation();
  return <output aria-label="Current settings route">{location.pathname}{location.search}</output>;
}

function openNewLibrary() {
  fireEvent.click(screen.getByRole('button', { name: 'Create first library' }));
  return screen.getByRole('dialog', { name: 'New library' });
}

function scanOperations(overrides: Partial<LibraryScanOperationsResponse> = {}): LibraryScanOperationsResponse {
  return {
    libraryId: 'fixture-movies',
    recentRuns: [],
    sources: [],
    actions: { canQuick: true, canTarget: true, canReconcile: true, canForceFull: true, canRemoveMissing: false, canCancel: false, canRetry: false },
    scheduleEnabled: true,
    generatedAt: '2026-08-05T15:30:00Z',
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

describe('library media folder editing', () => {
  it('opens a deep-linked create flow once, clears the query, and focuses the library name', async () => {
    renderOperations({ initialEntry: '/settings/media?newLibrary=1' });
    const editor = await screen.findByRole('dialog', { name: 'New library' });
    const name = within(editor).getByRole('textbox', { name: 'Library name' });
    await waitFor(() => expect(name).toHaveFocus());
    expect(screen.getByLabelText('Current settings route').textContent).toBe('/settings/media');
    fireEvent.click(within(editor).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('dialog', { name: 'New library' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Current settings route').textContent).toBe('/settings/media');
  });

  it('exposes host browsing only to an owner with manageServer permission', async () => {
    const source = new FixtureSettingsDataSource();
    const filesystem = filesystemSource();
    const owner: SettingsViewer = {
      id: 'owner',
      displayName: 'Owner',
      email: 'owner@portico.local',
      role: 'owner',
      serverName: 'Portico',
      permissions: { manageLibraries: true, manageServer: true },
    };
    const view = render(<MemoryRouter initialEntries={['/settings/media']}><Routes><Route path="/settings/:section" element={<SettingsPage source={source} filesystemSource={filesystem} viewer={owner} />} /></Routes></MemoryRouter>);
    await screen.findByRole('heading', { name: 'Media' });
    fireEvent.click((await screen.findAllByRole('button', { name: 'New library' }))[0]);
    expect(screen.getByRole('dialog', { name: 'New library' })).toHaveTextContent('Browse server');

    view.unmount();
    const ordinaryUser = { ...owner, role: 'user' as const };
    const delegatedSource = new FixtureSettingsDataSource();
    const settings = vi.spyOn(delegatedSource, 'settings');
    const summary = vi.spyOn(delegatedSource, 'settingsSummary');
    render(<MemoryRouter initialEntries={['/settings/media']}><Routes><Route path="/settings/:section" element={<SettingsPage source={delegatedSource} filesystemSource={filesystem} viewer={ordinaryUser} />} /></Routes></MemoryRouter>);
    expect(await screen.findByRole('heading', { name: 'Media' })).toBeInTheDocument();
    expect(screen.queryByText('Server settings aren’t available')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New library' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'New library' }));
    expect(screen.getByRole('dialog', { name: 'New library' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Browse server' })).not.toBeInTheDocument();
    expect(settings).not.toHaveBeenCalled();
    expect(summary).not.toHaveBeenCalled();
  });

  it('adds a server-validated folder and ignores a duplicate picker selection', async () => {
    const { source } = renderOperations({ filesystem: filesystemSource(), canBrowseFilesystem: true });
    const create = vi.spyOn(source, 'createLibrary');
    const scan = vi.spyOn(source, 'scanLibrary');
    let editor = openNewLibrary();
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Library name' }), { target: { value: 'Cinema' } });
    fireEvent.click(within(editor).getByRole('button', { name: 'Browse server' }));

    let picker = await screen.findByRole('dialog', { name: 'Add server folder' });
    fireEvent.click(await within(picker).findByRole('button', { name: 'Open folder Movies' }));
    await waitFor(() => expect(within(picker).getByRole('button', { name: 'Add folder' })).toBeEnabled());
    fireEvent.click(within(picker).getByRole('button', { name: 'Add folder' }));

    editor = screen.getByRole('dialog', { name: 'New library' });
    expect(within(editor).getByRole('textbox', { name: 'Media folder 1' })).toHaveValue('/srv/media/Movies');
    fireEvent.click(within(editor).getByRole('button', { name: 'Browse server' }));
    picker = await screen.findByRole('dialog', { name: 'Add server folder' });
    await waitFor(() => expect(within(picker).getByRole('button', { name: 'Add folder' })).toBeEnabled());
    fireEvent.click(within(picker).getByRole('button', { name: 'Add folder' }));

    editor = screen.getByRole('dialog', { name: 'New library' });
    expect(within(editor).getAllByRole('textbox', { name: /Media folder/ })).toHaveLength(1);
    expect(within(editor).getByText('That server folder is already included.')).toBeInTheDocument();
    fireEvent.click(within(editor).getByRole('button', { name: 'Create library' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Cinema',
      path: '/srv/media/Movies',
      paths: ['/srv/media/Movies'],
    }), expect.any(AbortSignal)));
    await waitFor(() => expect(scan).toHaveBeenCalledWith(expect.stringMatching(/^fixture-library-/), expect.any(AbortSignal)));
    expect(await screen.findByText('Cinema created.')).toBeInTheDocument();
    expect(screen.getByText('Scan queued')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open library' })).toHaveAttribute('href', expect.stringMatching(/^\/library\/fixture-library-/));
  });

  it('replaces a configured folder and combines duplicate source paths', async () => {
    const configured = library(['/srv/media/Movies', '/mnt/archive/Classics']);
    const { source } = renderOperations({ libraries: [configured], filesystem: filesystemSource(), canBrowseFilesystem: true });
    const update = vi.spyOn(source, 'updateLibrary');
    fireEvent.click(screen.getByRole('button', { name: 'Edit Movies' }));
    let editor = screen.getByRole('dialog', { name: 'Edit Movies' });
    fireEvent.click(within(editor).getAllByRole('button', { name: 'Replace' })[0]);

    const picker = await screen.findByRole('dialog', { name: 'Replace server folder' });
    const pathInput = within(picker).getByRole('textbox', { name: 'Server path' });
    fireEvent.change(pathInput, { target: { value: '/mnt/archive/Classics' } });
    fireEvent.click(within(picker).getByRole('button', { name: 'Open path' }));
    await waitFor(() => expect(within(picker).getByRole('button', { name: 'Replace folder' })).toBeEnabled());
    fireEvent.click(within(picker).getByRole('button', { name: 'Replace folder' }));

    editor = screen.getByRole('dialog', { name: 'Edit Movies' });
    const pathInputs = within(editor).getAllByRole('textbox', { name: /Media folder/ });
    expect(pathInputs).toHaveLength(1);
    expect(pathInputs[0]).toHaveValue('/mnt/archive/Classics');
    expect(within(editor).getByText(/kept one source/)).toBeInTheDocument();
    fireEvent.click(within(editor).getByRole('button', { name: 'Save library' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith('fixture-movies', expect.objectContaining({
      path: '/mnt/archive/Classics',
      paths: ['/mnt/archive/Classics'],
    }), expect.any(AbortSignal)));
  });

  it('keeps manual path entry available, validates absolutes, and deduplicates before save', async () => {
    const { source } = renderOperations();
    const create = vi.spyOn(source, 'createLibrary');
    const editor = openNewLibrary();
    expect(within(editor).queryByRole('button', { name: 'Browse server' })).not.toBeInTheDocument();
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Library name' }), { target: { value: 'Manual movies' } });
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Media folder 1' }), { target: { value: 'media/movies' } });
    fireEvent.click(within(editor).getByRole('button', { name: 'Create library' }));
    expect(await within(editor).findByRole('alert')).toHaveTextContent('absolute server path');
    expect(create).not.toHaveBeenCalled();

    fireEvent.change(within(editor).getByRole('textbox', { name: 'Media folder 1' }), { target: { value: '/media/movies' } });
    fireEvent.click(within(editor).getByRole('button', { name: 'Add path manually' }));
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Media folder 2' }), { target: { value: '/media/movies/' } });
    fireEvent.click(within(editor).getByRole('button', { name: 'Create library' }));
    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      path: '/media/movies',
      paths: ['/media/movies'],
    }), expect.any(AbortSignal)));
  });

  it('renders the picker failure without turning it into an inert library action', async () => {
    const denied = Object.assign(new Error('Only the server owner can browse host folders.'), { status: 403, code: 'forbidden' });
    renderOperations({ filesystem: filesystemSource(denied), canBrowseFilesystem: true });
    const editor = openNewLibrary();
    fireEvent.click(within(editor).getByRole('button', { name: 'Browse server' }));
    const picker = await screen.findByRole('dialog', { name: 'Add server folder' });
    expect(await within(picker).findByText('Folder access is unavailable')).toBeInTheDocument();
    expect(within(picker).getByText('Only the server owner can browse host folders.')).toBeInTheDocument();
    expect(within(picker).getByRole('button', { name: 'Add folder' })).toBeDisabled();
    fireEvent.click(within(picker).getByRole('button', { name: 'Close folder picker' }));
    expect(screen.getByRole('dialog', { name: 'New library' })).toBeInTheDocument();
  });

  it('keeps a created library visible as success when its initial scan cannot be queued', async () => {
    const { source, onChanged } = renderOperations();
    vi.spyOn(source, 'scanLibrary').mockRejectedValue(new Error('The scanner is temporarily unavailable.'));
    const editor = openNewLibrary();
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Library name' }), { target: { value: 'Documentaries' } });
    fireEvent.change(within(editor).getByRole('textbox', { name: 'Media folder 1' }), { target: { value: '/media/documentaries' } });
    fireEvent.click(within(editor).getByRole('button', { name: 'Create library' }));

    expect(await screen.findByText('Documentaries created.')).toBeInTheDocument();
    expect(screen.getByText(/couldn't queue the initial scan/)).toBeInTheDocument();
    expect(screen.getByText('Scan not started')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open library' })).toBeInTheDocument();
    expect(onChanged).not.toHaveBeenCalled();
  });
});

describe('owner library scan operations', () => {
  it('renders one coherent completed operation with aggregate subordinate work and degraded-root evidence', () => {
    const configured = {
      ...library(['/srv/media/Movies', '/mnt/archive/Classics']),
      scanSummary: {
        jobId: 'scan-movies',
        status: 'completed',
        progress: 100,
        mode: 'reconcile',
        phase: 'complete',
        message: 'Reconciliation completed with a storage warning.',
        completedAt: '2026-08-05T15:20:00Z',
        nextRunAt: '2026-08-06T02:00:00Z',
        degradedRootCount: 1,
        absenceAuthoritative: false,
        cleanupAllowed: false,
        counts: { processed: 2840, added: 12, updated: 8, metadataPending: 41, metadataCompleted: 2799, analysisPending: 17, analysisCompleted: 2823, missing: 6 },
        roots: [{ id: 'archive', path: '/mnt/archive/Classics', status: 'degraded', errorClass: 'ESTALE', warning: 'The archive mount returned a stale file handle.' }],
      },
    } as Library;
    renderOperations({ libraries: [configured] });

    expect(screen.getByText('Complete')).toBeInTheDocument();
    expect(screen.queryByText('Scanning')).not.toBeInTheDocument();
    expect(screen.getAllByRole('article')).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));

    expect(screen.getByRole('region', { name: 'Movies scan details' })).toHaveTextContent('Reconcile library');
    expect(screen.getByRole('region', { name: 'Movies scan details' })).toHaveTextContent('2,840');
    expect(screen.getByRole('region', { name: 'Movies scan details' })).toHaveTextContent('41 queued');
    expect(screen.getByRole('region', { name: 'Movies scan details' })).toHaveTextContent('17 queued');
    expect(screen.getByText('1 source needs attention')).toBeInTheDocument();
    expect(screen.getByText('The archive mount returned a stale file handle.')).toBeInTheDocument();
    expect(screen.getByText('/mnt/archive/Classics · degraded')).toBeInTheDocument();
  });

  it('offers the explicit scan modes, retries failed work, and cancels the single active operation', async () => {
    const running = {
      ...library(),
      scanSummary: { jobId: 'scan-active', status: 'running', progress: 38, mode: 'targeted', phase: 'discovering', message: 'Checking changed folders.' },
    } as Library;
    const { source, onChanged } = renderOperations({ libraries: [running] });
    const cancel = vi.spyOn(source, 'cancelLibraryScan');

    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    expect(screen.getByRole('progressbar', { name: 'Movies scan progress' })).toHaveAttribute('value', '38');
    expect(screen.getAllByText('Discovering')).not.toHaveLength(0);
    const view = screen.getByRole('region', { name: 'Movies scan details' });
    expect(within(view).getByRole('button', { name: 'Scan changes' })).toBeDisabled();
    expect(within(view).getByRole('button', { name: 'Quick' })).toBeDisabled();
    expect(within(view).getByRole('button', { name: 'Reconcile' })).toBeDisabled();
    expect(within(view).getByRole('button', { name: 'Force full' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(cancel).toHaveBeenCalledWith('fixture-movies', expect.any(AbortSignal)));
    expect(onChanged).toHaveBeenCalled();
  });

  it('requires reconciliation review and an explicit destructive confirmation before remove-missing', async () => {
    const configured = {
      ...library(),
      scanSummary: {
        jobId: 'scan-complete', status: 'completed', progress: 100, mode: 'reconcile', phase: 'complete',
        absenceAuthoritative: true, cleanupAllowed: true, confirmedMissingCount: 3, ambiguousCount: 0,
      },
    } as Library;
    const source = new FixtureSettingsDataSource();
    vi.spyOn(source, 'libraryScanOperations').mockResolvedValue(scanOperations({
      actions: { canQuick: true, canTarget: true, canReconcile: true, canForceFull: true, canRemoveMissing: true, canCancel: false, canRetry: false },
      lastRun: {
        id: 'run-fixture-movies', jobId: 'scan-complete', mode: 'reconcile', status: 'healthy', phase: { code: 'complete', label: 'Complete', state: 'complete' },
        filesIndexed: 0, filesUnchanged: 42, filesSkipped: 0, missingMarked: 3, metadataQueued: 0, analysisQueued: 0,
        absenceAuthoritative: true, cleanupAllowed: true, warnings: [], roots: [{ sourceId: 'movies', configuredPath: '/srv/media/Movies', status: 'healthy', directoriesSeen: 8, filesSeen: 42 }],
        startedAt: '2026-08-05T15:00:00Z', completedAt: '2026-08-05T15:20:00Z', updatedAt: '2026-08-05T15:20:00Z',
      },
      lastRunAt: '2026-08-05T15:20:00Z',
    }));
    vi.spyOn(source, 'libraryScanReview').mockResolvedValueOnce({
      libraryId: 'fixture-movies', confirmationRunId: 'run-fixture-movies', canConfirmRemoval: true, missingTotal: 3,
      missingItems: [
        { mediaId: 'movie-1', fileId: 'file-1', title: 'The Example', path: '/srv/media/Movies/The Example.mkv', missingSince: '2026-08-05T14:00:00Z', sourceId: 'movies', sourceHealth: 'healthy' },
        { mediaId: 'movie-2', fileId: 'file-2', title: 'Offline Archive', path: '/mnt/archive/Offline.mkv', missingSince: '2026-08-05T14:05:00Z', sourceId: 'archive', sourceHealth: 'healthy' },
      ],
      identityReviews: [{ id: 'identity-1', domain: 'media', libraryOrSourceId: 'fixture-movies', subjectId: 'movie-3', candidateLocator: 'Example (2026)', evidenceKind: 'fingerprint', evidenceValue: 'ambiguous move', candidateIds: ['candidate-a', 'candidate-b'], status: 'open', createdAt: '2026-08-05T13:00:00Z' }],
      openIdentityTotal: 1, limit: 50, hasMore: true, nextCursor: 'next', generatedAt: '2026-08-05T15:30:00Z',
    }).mockResolvedValueOnce({
      libraryId: 'fixture-movies', confirmationRunId: 'run-fixture-movies', canConfirmRemoval: true, missingTotal: 3,
      missingItems: [
        { mediaId: 'movie-1', fileId: 'file-1', title: 'The Example', path: '/srv/media/Movies/The Example.mkv', missingSince: '2026-08-05T14:00:00Z', sourceId: 'movies', sourceHealth: 'healthy' },
        { mediaId: 'movie-3', fileId: 'file-3', title: 'Third Example', path: '/srv/media/Movies/Third Example.mkv', missingSince: '2026-08-05T14:10:00Z', sourceId: 'movies', sourceHealth: 'healthy' },
      ],
      identityReviews: [], openIdentityTotal: 1, limit: 50, hasMore: false, generatedAt: '2026-08-05T15:31:00Z',
    }).mockResolvedValueOnce({
      libraryId: 'fixture-movies', confirmationRunId: 'run-fixture-movies', canConfirmRemoval: true, missingTotal: 3,
      missingItems: [
        { mediaId: 'movie-1', fileId: 'file-1', title: 'The Example', path: '/srv/media/Movies/The Example.mkv', missingSince: '2026-08-05T14:00:00Z', sourceId: 'movies', sourceHealth: 'healthy' },
        { mediaId: 'movie-2', fileId: 'file-2', title: 'Offline Archive', path: '/mnt/archive/Offline.mkv', missingSince: '2026-08-05T14:05:00Z', sourceId: 'archive', sourceHealth: 'healthy' },
        { mediaId: 'movie-3', fileId: 'file-3', title: 'Third Example', path: '/srv/media/Movies/Third Example.mkv', missingSince: '2026-08-05T14:10:00Z', sourceId: 'movies', sourceHealth: 'healthy' },
      ],
      identityReviews: [], openIdentityTotal: 0, limit: 50, hasMore: false, generatedAt: '2026-08-05T15:32:00Z',
    });
    renderOperations({ libraries: [configured], source });
    const scan = vi.spyOn(source, 'scanLibrary');
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    fireEvent.click(screen.getByRole('button', { name: 'Review removal' }));

    const dialog = screen.getByRole('dialog', { name: 'Remove missing items' });
    await within(dialog).findByText('run-fixture-movies');
    expect(within(dialog).getByText(/3 missing items will be removed/)).toBeInTheDocument();
    expect(within(dialog).getByRole('region', { name: 'Missing item evidence' })).toHaveTextContent('The Example');
    expect(within(dialog).getByRole('region', { name: 'Missing item evidence' })).toHaveTextContent('/srv/media/Movies/The Example.mkv');
    expect(within(dialog).getByRole('region', { name: 'Missing item evidence' })).toHaveTextContent('Showing 2 of 3 — more remain');
    expect(within(dialog).getByRole('region', { name: 'Open identity review summary' })).toHaveTextContent('Example (2026)');
    expect(within(dialog).getByRole('region', { name: 'Open identity review summary' })).toHaveTextContent('fingerprint: ambiguous move · subject movie-3');
    expect(within(dialog).getByRole('region', { name: 'Open identity review summary' })).toHaveTextContent('candidate-a');
    expect(within(dialog).getByRole('region', { name: 'Open identity review summary' })).toHaveTextContent('candidate-b');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Load more missing items' }));
    expect(await within(dialog).findByText('Third Example')).toBeInTheDocument();
    expect(within(dialog).getByRole('region', { name: 'Missing item evidence' }).querySelectorAll('article')).toHaveLength(3);
    expect(source.libraryScanReview).toHaveBeenLastCalledWith('fixture-movies', 'next', expect.any(AbortSignal));
    const resolveIdentity = vi.spyOn(source, 'resolveIdentityReconciliationReview');
    fireEvent.click(within(dialog).getByRole('radio', { name: /candidate-a/ }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Merge into selected' }));
    await waitFor(() => expect(resolveIdentity).toHaveBeenCalledWith('identity-1', 'merge_into_candidate', 'candidate-a', expect.any(AbortSignal)));
    await waitFor(() => expect(within(dialog).queryByRole('region', { name: 'Open identity review summary' })).not.toBeInTheDocument());
    expect(source.libraryScanReview).toHaveBeenLastCalledWith('fixture-movies', undefined, expect.any(AbortSignal));
    const confirm = within(dialog).getByRole('button', { name: 'Remove missing items' });
    expect(confirm).toBeDisabled();
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Remove missing confirmation' }), { target: { value: 'REMOVE' } });
    expect(confirm).toBeEnabled();
    fireEvent.click(confirm);
    await waitFor(() => expect(scan).toHaveBeenCalledWith('fixture-movies', expect.any(AbortSignal), 'remove_missing', 'run-fixture-movies'));
  });

  it('serializes an operation refresh that is requested while an earlier cycle is still in flight', async () => {
    const source = new FixtureSettingsDataSource();
    const first = deferred<LibraryScanOperationsResponse>();
    const second = deferred<LibraryScanOperationsResponse>();
    vi.spyOn(source, 'libraryScanOperations').mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise);
    renderOperations({ libraries: [library()], source });
    await waitFor(() => expect(source.libraryScanOperations).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', { name: 'Quick scan' }));
    await Promise.resolve();
    expect(source.libraryScanOperations).toHaveBeenCalledTimes(1);
    first.resolve(scanOperations({ operation: { jobId: 'first-job', status: 'queued', mode: 'quick', trigger: 'api', progress: 5, phase: { code: 'queued', label: 'Queued', state: 'pending' }, message: 'Waiting for the first cycle.', attemptCount: 0, createdAt: '2026-08-05T15:00:00Z', updatedAt: '2026-08-05T15:01:00Z' } }));
    await waitFor(() => expect(source.libraryScanOperations).toHaveBeenCalledTimes(2));
    second.resolve(scanOperations({ operation: { jobId: 'second-job', status: 'running', mode: 'quick', trigger: 'api', progress: 73, phase: { code: 'analyzing', label: 'Analyzing', state: 'active' }, message: 'Analyzing streams.', attemptCount: 0, createdAt: '2026-08-05T15:02:00Z', updatedAt: '2026-08-05T15:03:00Z' }, actions: { canQuick: false, canTarget: false, canReconcile: false, canForceFull: false, canRemoveMissing: false, canCancel: true, canRetry: false } }));
    expect((await screen.findAllByText('Analyzing')).length).toBeGreaterThan(0);
  });

  it('lets the owner override storage classification and refreshes the operation projection', async () => {
    const source = new FixtureSettingsDataSource();
    const operations = scanOperations({
      sources: [{ id: 'source-archive', configuredPath: '/mnt/archive', classification: 'network', classificationSource: 'detected', health: 'healthy', circuitState: 'closed', latencyMs: 84, consecutiveFailures: 0, updatedAt: '2026-08-05T15:00:00Z' }],
    });
    const load = vi.spyOn(source, 'libraryScanOperations').mockResolvedValue(operations);
    const classify = vi.spyOn(source, 'updateLibraryStorageClassification');
    renderOperations({ libraries: [library()], source });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    const selector = await screen.findByRole('combobox', { name: 'Storage classification for /mnt/archive' });
    expect(selector).toHaveValue('network');
    expect(screen.getByText(/Detected as network/)).toBeInTheDocument();
    fireEvent.change(selector, { target: { value: 'fuse' } });
    await waitFor(() => expect(classify).toHaveBeenCalledWith('fixture-movies', 'source-archive', 'fuse', expect.any(AbortSignal)));
    await waitFor(() => expect(load.mock.calls.length).toBeGreaterThan(1));
    expect(await screen.findByText('Movies storage classification saved.')).toBeInTheDocument();
  });

  it('lets the owner add a WebDAV source without retaining credentials in the rendered settings state', async () => {
    const source = new FixtureSettingsDataSource();
    const load = vi.spyOn(source, 'remoteStorageSources').mockResolvedValue([]);
    const create = vi.spyOn(source, 'createRemoteStorageSource').mockResolvedValue({
      id: 'remote-webdav', libraryId: 'fixture-movies', kind: 'webdav', name: 'Cloud Movies', endpoint: 'https://dav.example.test/media', root: 'Movies',
      analysisMode: 'basic', health: 'unknown', inventoryStatus: 'never', objects: 0, missingObjects: 0, credentialPresent: true, updatedAt: '2026-08-22T12:00:00Z',
    });
    renderOperations({ libraries: [library()], source });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    const remote = await screen.findByRole('region', { name: 'Movies remote storage' });
    await waitFor(() => expect(load).toHaveBeenCalledWith('fixture-movies', expect.any(AbortSignal)));
    fireEvent.click(within(remote).getByRole('button', { name: 'Add source' }));
    const dialog = screen.getByRole('dialog', { name: 'Add remote storage' });
    expect(within(dialog).getByRole('combobox', { name: 'Remote scan depth' })).toHaveValue('basic');
    expect(within(dialog).getByText('Recommended for rclone and WebDAV. Adds technical facts and representative thumbnails with bounded reads.')).toBeInTheDocument();
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Remote source name' }), { target: { value: 'Cloud Movies' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'WebDAV endpoint' }), { target: { value: 'https://dav.example.test/media' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'WebDAV root' }), { target: { value: 'Movies' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'WebDAV username' }), { target: { value: 'owner' } });
    fireEvent.change(within(dialog).getByLabelText('WebDAV password'), { target: { value: 'write-only-secret' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add source' }));

    await waitFor(() => expect(create).toHaveBeenCalledWith('fixture-movies', {
      kind: 'webdav', name: 'Cloud Movies', endpoint: 'https://dav.example.test/media', root: 'Movies', analysisMode: 'basic', username: 'owner', password: 'write-only-secret',
    }, expect.any(AbortSignal)));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Add remote storage' })).not.toBeInTheDocument());
    expect(within(remote).getByText('Cloud Movies')).toBeInTheDocument();
    expect(remote).not.toHaveTextContent('owner');
    expect(remote).not.toHaveTextContent('write-only-secret');
  });

  it('validates rclone input, queues inventory, and requires confirmation before removal', async () => {
    const source = new FixtureSettingsDataSource();
    const remoteSource = {
      id: 'remote-rclone', libraryId: 'fixture-movies', kind: 'rclone' as const, name: 'Archive remote', root: 'Movies',
      analysisMode: 'file_list_only' as const, health: 'healthy' as const, inventoryStatus: 'complete', objects: 12500, missingObjects: 2, credentialPresent: true, updatedAt: '2026-08-22T12:00:00Z',
    };
    vi.spyOn(source, 'remoteStorageSources').mockResolvedValue([remoteSource]);
    const inventory = vi.spyOn(source, 'inventoryRemoteStorageSource');
    const updateMode = vi.spyOn(source, 'updateRemoteStorageSourceAnalysisMode').mockImplementation(async (id, sourceId, analysisMode) => ({ ...remoteSource, id: sourceId, libraryId: id, analysisMode }));
    const remove = vi.spyOn(source, 'deleteRemoteStorageSource');
    renderOperations({ libraries: [library()], source });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    const remote = await screen.findByRole('region', { name: 'Movies remote storage' });
    expect(within(remote).getByText('12,500 · 2 missing')).toBeInTheDocument();
    const depth = within(remote).getByRole('combobox', { name: 'Scan depth for Archive remote' });
    expect(depth).toHaveValue('file_list_only');
    fireEvent.change(depth, { target: { value: 'complete' } });
    await waitFor(() => expect(updateMode).toHaveBeenCalledWith('fixture-movies', 'remote-rclone', 'complete', expect.any(AbortSignal)));
    expect(await within(remote).findByText('Archive remote scan depth updated to complete.')).toBeInTheDocument();
    fireEvent.click(within(remote).getByRole('button', { name: 'Scan' }));
    await waitFor(() => expect(inventory).toHaveBeenCalledWith('fixture-movies', 'remote-rclone', expect.any(AbortSignal)));

    fireEvent.click(within(remote).getByRole('button', { name: 'Remove Archive remote' }));
    expect(within(remote).getByText(/Portico will delete its saved connection, not remote files/)).toBeInTheDocument();
    expect(remove).not.toHaveBeenCalled();
    fireEvent.click(within(remote).getByRole('button', { name: 'Remove source' }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith('fixture-movies', 'remote-rclone', expect.any(AbortSignal)));
    expect(within(remote).queryByText('Archive remote')).not.toBeInTheDocument();
  });

  it('submits an isolated rclone configuration and clears it when creation fails', async () => {
    const source = new FixtureSettingsDataSource();
    vi.spyOn(source, 'remoteStorageSources').mockResolvedValue([]);
    const create = vi.spyOn(source, 'createRemoteStorageSource').mockRejectedValue(new Error('The selected executable did not identify itself as rclone.'));
    renderOperations({ libraries: [library()], source });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    const remote = await screen.findByRole('region', { name: 'Movies remote storage' });
    fireEvent.click(within(remote).getByRole('button', { name: 'Add source' }));
    const dialog = screen.getByRole('dialog', { name: 'Add remote storage' });
    fireEvent.click(within(dialog).getByRole('radio', { name: /rclone/ }));
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Remote source name' }), { target: { value: 'Archive remote' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'rclone binary path' }), { target: { value: '/opt/portico/bin/rclone' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'rclone remote name' }), { target: { value: 'archive' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'rclone root' }), { target: { value: 'Movies' } });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'rclone config' }), { target: { value: '[archive]\ntype = s3\nsecret_access_key = hidden' } });
    fireEvent.change(within(dialog).getByRole('combobox', { name: 'Remote scan depth' }), { target: { value: 'file_list_only' } });
    expect(within(dialog).getByText('Reads no media content during scans. Technical stream data and thumbnails are deferred.')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add source' }));

    await waitFor(() => expect(create).toHaveBeenCalledWith('fixture-movies', {
      kind: 'rclone', name: 'Archive remote', root: 'Movies', analysisMode: 'file_list_only', rcloneBinaryPath: '/opt/portico/bin/rclone', rcloneRemoteName: 'archive', rcloneConfig: '[archive]\ntype = s3\nsecret_access_key = hidden',
    }, expect.any(AbortSignal)));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent("Portico couldn't add this rclone source");
    expect(within(dialog).getByRole('textbox', { name: 'rclone config' })).toHaveValue('');
  });

  it('does not expose managed remote storage controls to non-owner viewers', async () => {
    const source = new FixtureSettingsDataSource();
    const load = vi.spyOn(source, 'remoteStorageSources');
    renderOperations({ libraries: [library()], source, canManage: false });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    expect(screen.queryByRole('region', { name: 'Movies remote storage' })).not.toBeInTheDocument();
    expect(load).not.toHaveBeenCalled();
  });

  it('blocks remove-missing when degraded storage cannot prove absence', async () => {
    const configured = {
      ...library(),
      scanSummary: {
        jobId: 'scan-degraded', status: 'completed', progress: 100, cleanupAllowed: false, absenceAuthoritative: false,
        degradedRootCount: 1, confirmedMissingCount: 19, roots: [{ path: '/srv/media/Movies', status: 'unavailable' }],
      },
    } as Library;
    renderOperations({ libraries: [configured] });
    fireEvent.click(screen.getByRole('button', { name: 'Show scan details for Movies' }));
    fireEvent.click(screen.getByRole('button', { name: 'Review removal' }));
    const dialog = screen.getByRole('dialog', { name: 'Remove missing items' });
    expect(await within(dialog).findByText('Removal is blocked.')).toBeInTheDocument();
    expect(within(dialog).queryByRole('textbox', { name: 'Remove missing confirmation' })).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Remove missing items' })).toBeDisabled();
  });
});
