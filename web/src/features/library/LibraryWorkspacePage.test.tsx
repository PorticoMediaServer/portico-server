import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import type { LibraryBrowseCapabilities } from '@porticomediaserver/client-core';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem } from '../../data/models';
import { LibraryWorkspacePage } from './LibraryWorkspacePage';
import type { LibraryPivotPage, LibraryWorkspaceLibrary, LibraryWorkspaceSource } from './libraryTypes';

const library: LibraryWorkspaceLibrary = { id: 'lib-real-movies', name: 'Cinema', kind: 'movies', itemCount: 2 };

const capabilities: LibraryBrowseCapabilities = {
  apiVersion: 'v1',
  library: { id: library.id, name: library.name, kind: library.kind },
  pivots: [
    { id: 'discover', label: 'Discover', entityKinds: ['movie'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/discover', defaultSort: [], defaultView: 'shelves', supportedViews: ['shelves'], presentationFields: [] },
    { id: 'movies', label: 'Movies', entityKinds: ['movie'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse', defaultSort: [{ field: 'title', direction: 'asc' }], defaultView: 'grid', supportedViews: ['grid', 'list', 'table'], presentationFields: ['year', 'availability'] },
    { id: 'collections', label: 'Collections', entityKinds: ['collection'], browseSupported: false, endpointTemplate: '/api/collections?libraryId={libraryId}', defaultSort: [], defaultView: 'grid', supportedViews: ['grid'], presentationFields: [] },
  ],
  fields: [
    { id: 'title', label: 'Title', valueType: 'string', operators: ['contains', 'starts-with'], applicableKinds: ['movie'], controlHint: 'text', complexity: 'quick', cost: 'indexed' },
    { id: 'playState', label: 'Playback', valueType: 'enum', operators: ['equals', 'not-equals'], applicableKinds: ['movie'], controlHint: 'select', complexity: 'quick', cost: 'indexed' },
    { id: 'availability', label: 'Availability', valueType: 'enum', operators: ['equals', 'not-equals'], applicableKinds: ['movie'], controlHint: 'select', complexity: 'quick', cost: 'indexed' },
  ],
  sorts: [
    { id: 'title', label: 'Title', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false, applicableKinds: ['movie'] },
    { id: 'dateAdded', label: 'Recently added', defaultDirection: 'desc', directions: ['desc'], expensive: false, applicableKinds: ['movie'] },
  ],
  presentationFields: ['year', 'availability'],
  actions: ['browse', 'manageLibrary'],
  queryLimits: { defaultLimit: 60, maximumLimit: 200, maximumClauses: 20, maximumDepth: 4, maximumBytes: 65_536, cursorTtlSeconds: 300 },
};

function item(id: string, title: string): MediaItem {
  return {
    id,
    title,
    subtitle: 'Movie',
    year: 2026,
    entityKind: 'movie',
    poster: '/brand/portico-icon.svg',
    backdrop: '/brand/portico-icon.svg',
    rating: '8.0',
    length: '2h',
    genre: 'Drama',
    libraryId: library.id,

    availability: 'available',
    actions: [],
  };
}

function page(items: MediaItem[], hasMore = false): LibraryPivotPage {
  return {
    items,
    total: 2,
    hasMore,
    nextCursor: hasMore ? 'cursor-2' : null,
    applied: { pivot: 'movies', sort: [{ field: 'title', direction: 'asc' }], presentationFields: ['year', 'availability'] },
    presentation: 'grid',
  };
}

function source(overrides: { capabilities?: LibraryBrowseCapabilities; facetOptions?: Array<{ value: string; label: string; count?: number }> } = {}) {
  const first = item('alpha', 'Alpha');
  const second = item('zulu', 'Zulu');
  return {
    libraryBrowseCapabilities: vi.fn(async () => overrides.capabilities ?? capabilities),
    libraryPivot: vi.fn(async (input) => input.request.seek ? page([second]) : input.request.cursor ? page([second]) : page([first], true)),
    libraryFacetOptions: vi.fn(async () => overrides.facetOptions ?? []),
    createSavedView: vi.fn(async (input) => ({
      id: 'view-1',
      ownerUserId: 'user-1',
      title: input.title,
      libraryId: input.libraryId,
      libraryName: library.name,
      pivot: input.pivot,
      query: input.query,
      sort: input.sort ?? [],
      presentation: input.presentation ?? {},
      isPinned: input.isPinned ?? false,
      createdAt: '2026-07-11T00:00:00Z',
      updatedAt: '2026-07-11T00:00:00Z',
    })),
  } satisfies LibraryWorkspaceSource;
}

function renderWorkspace(workspaceSource: LibraryWorkspaceSource, route = '/library/lib-real-movies?pivot=movies') {
  return render(<DataProvider source={new FixturePorticoDataSource()}>
    <MemoryRouter initialEntries={[route]}><LibraryWorkspacePage library={library} source={workspaceSource} /></MemoryRouter>
  </DataProvider>);
}

describe('LibraryWorkspacePage', () => {
  it('reserves the known library header, pivot, toolbar, and result geometry while capabilities load', () => {
    const workspaceSource = source();
    workspaceSource.libraryBrowseCapabilities = vi.fn(() => new Promise(() => {}));
    const { container } = renderWorkspace(workspaceSource);

    expect(screen.getByText('Cinema')).toBeInTheDocument();
    expect(container.querySelector('.library-pivots-reserved')).toBeInTheDocument();
    expect(container.querySelector('.library-workspace-toolbar-reserved')).toBeInTheDocument();
    expect(container.querySelector('.library-results-reservation')).toBeInTheDocument();
  });

  it('reserves square result geometry for music before server capabilities arrive', () => {
    const workspaceSource = source();
    workspaceSource.libraryBrowseCapabilities = vi.fn(() => new Promise(() => {}));
    const musicLibrary: LibraryWorkspaceLibrary = { id: 'lib-real-music', name: 'Music', kind: 'music', itemCount: 2 };
    const { container } = render(<DataProvider source={new FixturePorticoDataSource()}>
      <MemoryRouter initialEntries={['/library/lib-real-music?pivot=albums']}><LibraryWorkspacePage library={musicLibrary} source={workspaceSource} /></MemoryRouter>
    </DataProvider>);

    expect(container.querySelector('.library-workspace-frame')).toHaveClass('is-square');
    expect(container.querySelector('.library-results-reservation')).toHaveClass('is-square');
  });

  it('binds every request to the selected real library and renders only server-declared pivots', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    expect(await screen.findByRole('heading', { name: 'Cinema' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Discover' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Movies' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Collections' })).toBeInTheDocument();
    expect(screen.queryByText('Sources')).not.toBeInTheDocument();
    await waitFor(() => expect(workspaceSource.libraryBrowseCapabilities).toHaveBeenCalledWith(library.id, expect.any(AbortSignal)));
    expect(workspaceSource.libraryPivot).toHaveBeenCalledWith(expect.objectContaining({ libraryId: library.id, libraryKind: library.kind }), expect.any(AbortSignal));
  });

  it('starts Discover shelves immediately after the pivots without reserving browse controls', async () => {
    const workspaceSource = source();
    const { container } = renderWorkspace(workspaceSource, '/library/lib-real-movies?pivot=discover');
    await screen.findByText('Alpha');

    expect(container.querySelector('.library-workspace-toolbar')).not.toBeInTheDocument();
    expect(container.querySelector('.library-results-summary')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Shelves view' })).not.toBeInTheDocument();
    expect(workspaceSource.libraryPivot).toHaveBeenCalledWith(expect.objectContaining({
      request: expect.objectContaining({ limit: 12 }),
    }), expect.any(AbortSignal));
  });

  it('keeps the authoritative library total when the first Discover page contains only 50 of 501 items', async () => {
    const largeLibrary = { ...library, itemCount: 501 };
    const firstPage = Array.from({ length: 50 }, (_, index) => item(`discover-${index}`, `Discover ${index + 1}`));
    const workspaceSource = source();
    vi.mocked(workspaceSource.libraryPivot).mockResolvedValue({
      ...page(firstPage),
      total: 50,
      applied: { pivot: 'discover', sort: [], presentationFields: [] },
      presentation: 'shelves',
    });
    render(<DataProvider source={new FixturePorticoDataSource()}>
      <MemoryRouter initialEntries={['/library/lib-real-movies?pivot=discover']}>
        <LibraryWorkspacePage library={largeLibrary} source={workspaceSource} />
      </MemoryRouter>
    </DataProvider>);

    await screen.findByText('Discover 1');
    expect(screen.getByText('501 results')).toBeInTheDocument();
    expect(screen.queryByText('50 results')).not.toBeInTheDocument();
  });

  it('applies capability-backed filters and ordered sorts to the canonical request', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');
    fireEvent.click(screen.getByRole('button', { name: 'More filters' }));
    const dialog = screen.getByRole('dialog', { name: 'Refine Movies' });
    fireEvent.change(within(dialog).getByLabelText('Value'), { target: { value: 'Fargo' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ query: { all: [{ field: 'title', operator: 'contains', value: 'Fargo' }] } }),
    }), expect.any(AbortSignal)));
  });

  it('uses server allowed values and facet identities instead of internal-token text fields', async () => {
    const facetSource = { endpointTemplate: '/api/libraries/{libraryId}/categories', filterField: 'filter', filterPrefix: 'genre:', valueField: 'name', labelField: 'name', countField: 'count' };
    const workspaceSource = source({
      capabilities: {
        ...capabilities,
        fields: [
          { id: 'playState', label: 'Playback', valueType: 'enum', operators: ['equals'], allowedValues: ['unplayed', 'in-progress', 'played'], applicableKinds: ['movie'], controlHint: 'select', complexity: 'quick', cost: 'indexed' },
          { id: 'genre', label: 'Genre', valueType: 'identity-set', operators: ['contains-any'], facetSource, applicableKinds: ['movie'], controlHint: 'facet-multi-select', complexity: 'standard', cost: 'indexed-join' },
        ],
      },
      facetOptions: [{ value: 'Drama, Mystery', label: 'Drama, Mystery', count: 14 }, { value: 'Comedy', label: 'Comedy', count: 8 }],
    });
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');
    fireEvent.click(screen.getByRole('button', { name: 'More filters' }));
    const dialog = screen.getByRole('dialog', { name: 'Refine Movies' });
    expect(within(dialog).getByRole('button', { name: /ValueSelect a value/ })).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Apply' })).toBeDisabled();
    expect(within(dialog).queryByRole('textbox', { name: 'Value' })).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: /FieldPlayback/ }));
    fireEvent.click(screen.getByRole('option', { name: /Genre/ }));
    const drama = await within(dialog).findByRole('checkbox', { name: /Drama, Mystery/ });
    fireEvent.click(drama);
    expect(within(dialog).getByRole('button', { name: 'Apply' })).toBeEnabled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ query: { all: [{ field: 'genre', operator: 'contains-any', value: ['Drama, Mystery'] }] } }),
    }), expect.any(AbortSignal)));
  });

  it('offers quick filters and toggles the selected sort direction in its menu', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');

    fireEvent.click(screen.getByRole('button', { name: /FilterAll items/ }));
    fireEvent.click(screen.getByRole('option', { name: 'Unwatched' }));
    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ query: { field: 'playState', operator: 'equals', value: 'unplayed' } }),
    }), expect.any(AbortSignal)));

    fireEvent.click(screen.getByRole('button', { name: /SortTitle/ }));
    fireEvent.click(screen.getByRole('option', { name: 'Title' }));
    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ sort: [{ field: 'title', direction: 'desc' }] }),
    }), expect.any(AbortSignal)));
    expect(screen.queryByText('Direction')).not.toBeInTheDocument();
  });

  it('exposes filter and sort choices as a labelled keyboard listbox', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');

    const trigger = screen.getByRole('button', { name: /SortTitle/ });
    fireEvent.click(trigger);
    const listbox = await screen.findByRole('listbox', { name: /SortTitle/ });
    expect(trigger).toHaveAttribute('aria-controls', listbox.id);
    const selected = within(listbox).getByRole('option', { name: 'Title' });
    await waitFor(() => expect(selected).toHaveFocus());
    fireEvent.keyDown(window, { key: 'End' });
    expect(within(listbox).getByRole('option', { name: 'Recently added' })).toHaveFocus();
    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('maps the Missing quick filter to the canonical unavailable status', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');

    fireEvent.click(screen.getByRole('button', { name: /FilterAll items/ }));
    fireEvent.click(screen.getByRole('option', { name: 'Missing' }));

    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ query: { field: 'availability', operator: 'equals', value: 'unavailable' } }),
    }), expect.any(AbortSignal)));
  });

  it('continues with the server cursor and saves the current portable view definition', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');
    fireEvent.click(await screen.findByRole('button', { name: 'More results' }));
    expect(await screen.findByText('Zulu')).toBeInTheDocument();
    expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({ request: expect.objectContaining({ cursor: 'cursor-2' }) }), expect.any(AbortSignal));

    fireEvent.click(screen.getByRole('button', { name: 'Save view' }));
    const dialog = screen.getByRole('dialog', { name: 'Save this view' });
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Name' }), { target: { value: 'Weekend movies' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save view' }));
    await waitFor(() => expect(workspaceSource.createSavedView).toHaveBeenCalledWith(expect.objectContaining({
      title: 'Weekend movies',
      libraryId: library.id,
      pivot: 'movies',
      sort: [{ field: 'title', direction: 'asc' }],
    }), expect.any(AbortSignal)));
  });

  it('reports the loaded count when cursor pages omit an authoritative total', async () => {
    const workspaceSource = source();
    const first = item('alpha', 'Alpha');
    const second = item('zulu', 'Zulu');
    vi.mocked(workspaceSource.libraryPivot).mockImplementation(async (input) => input.request.cursor
      ? { ...page([second]), total: undefined }
      : { ...page([first], true), total: undefined });
    renderWorkspace(workspaceSource);

    await screen.findByText('Alpha');
    expect(screen.getAllByText('1 result loaded').length).toBeGreaterThan(0);
    fireEvent.click(await screen.findByRole('button', { name: 'More results' }));
    await screen.findByText('Zulu');
    expect(screen.getAllByText('2 results loaded').length).toBeGreaterThan(0);
  });

  it('keeps loaded results and offers an inline retry when continuation fails', async () => {
    const workspaceSource = source();
    const second = item('zulu', 'Zulu');
    let attempts = 0;
    vi.mocked(workspaceSource.libraryPivot).mockImplementation(async (input) => {
      if (!input.request.cursor) return page([item('alpha', 'Alpha')], true);
      attempts += 1;
      if (attempts === 1) throw new Error('private transport detail');
      return page([second]);
    });
    renderWorkspace(workspaceSource);

    await screen.findByText('Alpha');
    fireEvent.click(await screen.findByRole('button', { name: 'More results' }));
    const warning = await screen.findByRole('alert');
    expect(within(warning).getByText(/Something interrupted the request before it could finish/i)).toBeInTheDocument();
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    fireEvent.click(within(warning).getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('Zulu')).toBeInTheDocument();
  });

  it('does not let a slow continuation overwrite a newer filtered request', async () => {
    const workspaceSource = source();
    let resolveContinuation!: (value: LibraryPivotPage) => void;
    vi.mocked(workspaceSource.libraryPivot).mockImplementation((input) => {
      if (input.request.cursor) return new Promise((resolve) => { resolveContinuation = resolve; });
      if (input.request.query) return Promise.resolve(page([item('filtered', 'Filtered result')]));
      return Promise.resolve(page([item('alpha', 'Alpha')], true));
    });
    renderWorkspace(workspaceSource);

    await screen.findByText('Alpha');
    fireEvent.click(await screen.findByRole('button', { name: 'More results' }));
    fireEvent.click(screen.getByRole('button', { name: /FilterAll items/ }));
    fireEvent.click(screen.getByRole('option', { name: 'Unwatched' }));
    expect(await screen.findByText('Filtered result')).toBeInTheDocument();
    resolveContinuation(page([item('obsolete', 'Obsolete result')]));
    await Promise.resolve();
    expect(screen.queryByText('Obsolete result')).not.toBeInTheDocument();
  });

  it('uses one server seek request for an alphabet jump instead of walking cursors', async () => {
    const workspaceSource = source();
    renderWorkspace(workspaceSource);
    await screen.findByText('Alpha');
    const callsBeforeSeek = workspaceSource.libraryPivot.mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Jump to Z' }));
    expect(await screen.findByText('Zulu')).toBeInTheDocument();
    await waitFor(() => expect(workspaceSource.libraryPivot).toHaveBeenLastCalledWith(expect.objectContaining({
      request: expect.objectContaining({ seek: { prefix: 'Z' }, cursor: undefined }),
    }), expect.any(AbortSignal)));
    expect(workspaceSource.libraryPivot.mock.calls).toHaveLength(callsBeforeSeek + 1);
  });
});
