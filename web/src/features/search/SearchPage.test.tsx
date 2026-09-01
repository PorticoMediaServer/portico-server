import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem, PorticoDataSource, SearchGroup, SearchPageInput, SearchPageResult, Viewer } from '../../data/models';
import { WebDisplayPreferencesProvider } from '../../preferences/WebDisplayPreferencesProvider';
import { SearchGroupSurface, SearchPage, searchRequestIdentity } from './SearchPage';

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
  viewerScope: { authority: 'local', accountId: 'viewer', serverId: 'server', profileId: 'profile', authorizationRevision: '1' },
};

function result(id: string, title: string): MediaItem {
  return {
    id, title, subtitle: '', year: 2026, entityKind: 'movie', poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [],
  };
}

async function limitSearchContract(source: FixturePorticoDataSource, groupIDs: string[]) {
  const contract = await source.searchContract(new AbortController().signal);
  const selected = contract.groups.filter((group) => groupIDs.includes(group.id));
  vi.spyOn(source, 'searchContract').mockResolvedValue({
    ...contract,
    groupOrder: selected.map((group) => group.id),
    groups: selected,
    sorts: contract.sorts.map((sort) => ({ ...sort, applicableGroups: sort.applicableGroups.filter((id) => groupIDs.includes(id)) })),
  });
}

function page(input: SearchPageInput, groups: SearchGroup[]): SearchPageResult {
  return { query: input.query, sort: input.sort ?? 'relevance', direction: input.direction ?? 'desc', groups };
}

describe('SearchPage history privacy', () => {
  it('sends recordHistory false when profile search history is disabled', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const preferences = await source.viewerPreferences(new AbortController().signal);
    preferences.profileServer.values.search.rememberHistory = false;
    vi.spyOn(source, 'viewerPreferences').mockResolvedValue(preferences);
    const searchPage = vi.spyOn(source as PorticoDataSource, 'searchPage');

    render(<DataProvider source={source} initialViewer={viewer}>
      <WebDisplayPreferencesProvider>
        <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
      </WebDisplayPreferencesProvider>
    </DataProvider>);

    await waitFor(() => expect(searchPage).toHaveBeenCalledWith(
      expect.objectContaining({ query: 'Fargo', recordHistory: false }),
      expect.any(AbortSignal),
    ));
  });

  it('renders search controls from the server contract', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const contract = await source.searchContract(new AbortController().signal);
    vi.spyOn(source, 'searchContract').mockResolvedValue({
      ...contract,
      groups: contract.groups.map((group) => group.id === 'movies' ? { ...group, title: 'Films' } : group),
      sorts: contract.sorts.map((sort) => sort.id === 'relevance' ? { ...sort, label: 'Server-ranked' } : sort),
    });

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);

    expect(await screen.findByRole('button', { name: 'Sort' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Films' })).toBeInTheDocument();
  });

  it('publishes only result-type choices allowed by the server descriptor', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const contract = await source.searchContract(new AbortController().signal);
    vi.spyOn(source, 'searchContract').mockResolvedValue({
      ...contract,
      filters: contract.filters.map((filter) => filter.id === 'entityKinds' ? { ...filter, allowedValues: ['music'] } : filter),
    });

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Blue']}><SearchPage /></MemoryRouter>
    </DataProvider>);

    expect(await screen.findByRole('button', { name: 'Music' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Movies' })).not.toBeInTheDocument();
  });

  it('keeps contract loading and failure states mutually exclusive with empty results', async () => {
    const loadingSource = new FixturePorticoDataSource(viewer);
    vi.spyOn(loadingSource, 'searchContract').mockImplementation(() => new Promise(() => undefined));
    const loading = render(<DataProvider source={loadingSource} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);
    expect(await screen.findByText('Searching')).toBeInTheDocument();
    expect(loading.container.querySelector('.search-contract-reservation')).toHaveAttribute('aria-busy', 'true');
    expect(loading.container.querySelector('.library-state')).not.toBeInTheDocument();
    expect(screen.queryByText(/No results for/)).not.toBeInTheDocument();
    loading.unmount();

    const failedSource = new FixturePorticoDataSource(viewer);
    vi.spyOn(failedSource, 'searchContract').mockRejectedValue(new Error('Contract unavailable.'));
    render(<DataProvider source={failedSource} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);
    expect(await screen.findByText("Portico couldn't complete this request")).toBeInTheDocument();
    expect(screen.queryByText(/No results for/)).not.toBeInTheDocument();
  });

  it('selects duplicate library names by stable server identity', async () => {
    const source = new FixturePorticoDataSource(viewer);
    vi.spyOn(source, 'libraries').mockResolvedValue([
      { id: 'library-a', name: 'Movies', kind: 'movies', itemCount: 1 },
      { id: 'library-b', name: 'Movies', kind: 'movies', itemCount: 1 },
    ]);
    const search = vi.spyOn(source, 'searchPage');
    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);

    fireEvent.click(await screen.findByRole('button', { name: 'Library' }));
    const duplicateOptions = screen.getAllByRole('option', { name: 'Movies' });
    fireEvent.click(duplicateOptions[1]);
    await waitFor(() => expect(search).toHaveBeenLastCalledWith(
      expect.objectContaining({ libraryIds: ['library-b'] }),
      expect.any(AbortSignal),
    ));
  });

  it('preserves healthy groups and presents a failed sibling through Product Language', async () => {
    const source = new FixturePorticoDataSource(viewer);
    await limitSearchContract(source, ['movies', 'shows']);
    vi.spyOn(source, 'searchPage').mockImplementation(async (input) => page(input, [
      { id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('healthy-movie', 'Healthy movie result')], hasMore: false, nextCursor: null },
      { id: 'shows', title: 'Shows', entityKind: 'show', status: 'error', errorCode: 'search_group_timeout', messageId: 'search.group-timeout', items: [], hasMore: false, nextCursor: null },
    ]));

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);

    expect(await screen.findByText('Healthy movie result')).toBeInTheDocument();
    expect(await screen.findByText("This result group didn't finish in time. The other search results are still available.")).toBeInTheDocument();
    expect(screen.queryByText('No results')).not.toBeInTheDocument();
    expect(screen.getByText('Healthy movie result').closest('.full-search-group')).toHaveAttribute('data-retained-result-count', '1');
  });

  it('loads every server-ordered initial result group in one request', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const contract = await source.searchContract(new AbortController().signal);
    const searchPage = vi.spyOn(source, 'searchPage').mockImplementation(async (input) => page(input, contract.groupOrder.map((groupID) => ({
      id: groupID,
      title: contract.groups.find((group) => group.id === groupID)?.title ?? groupID,
      entityKind: contract.groups.find((group) => group.id === groupID)?.entityKind ?? groupID,
      status: 'success',
      items: [],
      hasMore: false,
      nextCursor: null,
    }))));

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo']}><SearchPage /></MemoryRouter>
    </DataProvider>);

    await waitFor(() => expect(searchPage).toHaveBeenCalledTimes(1));
    expect(searchPage).toHaveBeenCalledWith(expect.objectContaining({ query: 'Fargo' }), expect.any(AbortSignal));
    expect(searchPage.mock.calls[0][0].group).toBeUndefined();
  });

  it('includes every result-shaping field in the stale-result identity', () => {
    const base: SearchPageInput = { query: 'Fargo', entityKinds: ['movie'], libraryIds: ['library-a'], sort: 'title', direction: 'asc' };
    const identity = searchRequestIdentity(base);
    expect(searchRequestIdentity({ ...base, query: 'Arrival' })).not.toBe(identity);
    expect(searchRequestIdentity({ ...base, entityKinds: ['show'] })).not.toBe(identity);
    expect(searchRequestIdentity({ ...base, libraryIds: ['library-b'] })).not.toBe(identity);
    expect(searchRequestIdentity({ ...base, sort: 'releaseYear' })).not.toBe(identity);
    expect(searchRequestIdentity({ ...base, direction: 'desc' })).not.toBe(identity);
    expect(searchRequestIdentity({ ...base, entityKinds: ['episode', 'movie'], libraryIds: ['library-b', 'library-a'] }))
      .toBe(searchRequestIdentity({ ...base, entityKinds: ['movie', 'episode'], libraryIds: ['library-a', 'library-b'] }));
  });

  it('ignores a slow response after the query identity changes', async () => {
    const source = new FixturePorticoDataSource(viewer);
    await limitSearchContract(source, ['movies']);
    let resolveOld!: (value: SearchPageResult) => void;
    vi.spyOn(source, 'searchPage').mockImplementation((input) => input.query === 'Old query'
      ? new Promise((resolve) => { resolveOld = resolve; })
      : Promise.resolve(page(input, [{ id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('new-query', 'New query result')], hasMore: false, nextCursor: null }])));

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Old%20query']}><SearchPage /></MemoryRouter>
    </DataProvider>);
    await waitFor(() => expect(resolveOld).toBeTypeOf('function'));
    const input = screen.getByRole('searchbox', { name: 'Search library' });
    fireEvent.change(input, { target: { value: 'New query' } });
    fireEvent.submit(input.closest('form')!);
    expect(await screen.findByText('New query result')).toBeInTheDocument();
    await act(async () => resolveOld(page({ query: 'Old query' }, [{ id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('old-query', 'Old query result')], hasMore: false, nextCursor: null }])));
    expect(screen.queryByText('Old query result')).not.toBeInTheDocument();
  });

  it('ignores a failed obsolete request after a newer query succeeds', async () => {
    const source = new FixturePorticoDataSource(viewer);
    await limitSearchContract(source, ['movies']);
    let rejectOld!: (reason: Error) => void;
    vi.spyOn(source, 'searchPage').mockImplementation((input) => input.query === 'Old query'
      ? new Promise((_resolve, reject) => { rejectOld = reject; })
      : Promise.resolve(page(input, [{ id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('current-query', 'Current query result')], hasMore: false, nextCursor: null }])));

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Old%20query']}><SearchPage /></MemoryRouter>
    </DataProvider>);
    await waitFor(() => expect(rejectOld).toBeTypeOf('function'));
    const input = screen.getByRole('searchbox', { name: 'Search library' });
    fireEvent.change(input, { target: { value: 'Current query' } });
    fireEvent.submit(input.closest('form')!);
    expect(await screen.findByText('Current query result')).toBeInTheDocument();
    await act(async () => rejectOld(new Error('Obsolete failure')));
    expect(screen.getByText('Current query result')).toBeInTheDocument();
    expect(screen.queryByText("Portico couldn't load this result group. The other search results are still available.")).not.toBeInTheDocument();
  });

  it('does not retain results when a changed library scope fails', async () => {
    const source = new FixturePorticoDataSource(viewer);
    await limitSearchContract(source, ['movies']);
    vi.spyOn(source, 'libraries').mockResolvedValue([
      { id: 'library-a', name: 'Library A', kind: 'movies', itemCount: 1 },
      { id: 'library-b', name: 'Library B', kind: 'movies', itemCount: 1 },
    ]);
    vi.spyOn(source, 'searchPage').mockImplementation((input) => input.libraryIds?.includes('library-b')
      ? Promise.reject(new Error('Scoped search failed'))
      : Promise.resolve(page(input, [{ id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('old-scope', 'Old scope result')], hasMore: false, nextCursor: null }])));

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter initialEntries={['/search?q=Fargo&library=library-a']}><SearchPage /></MemoryRouter>
    </DataProvider>);
    expect(await screen.findByText('Old scope result')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Library' }));
    fireEvent.click(screen.getByRole('option', { name: 'Library B' }));
    expect(await screen.findByText("Portico couldn't load this result group. The other search results are still available.")).toBeInTheDocument();
    expect(screen.queryByText('Old scope result')).not.toBeInTheDocument();
  });

  it('drops a delayed continuation from an obsolete search generation', async () => {
    const source = new FixturePorticoDataSource(viewer);
    let resolveContinuation!: (value: SearchPageResult) => void;
    vi.spyOn(source, 'searchPage').mockImplementation((input) => input.cursor
      ? new Promise((resolve) => { resolveContinuation = resolve; })
      : Promise.resolve({ query: input.query, sort: input.sort ?? 'relevance', direction: input.direction ?? 'desc', groups: [] }));
    const oldGroup: SearchGroup = { id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('old-first', 'Old first')], hasMore: true, nextCursor: 'old-cursor' };
    const newGroup: SearchGroup = { id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [result('new-first', 'New first')], hasMore: false, nextCursor: null };
    const oldRequest: SearchPageInput = { query: 'old', sort: 'relevance', direction: 'desc', limit: 50 };
    const newRequest: SearchPageInput = { query: 'new', sort: 'relevance', direction: 'desc', limit: 50 };
    const shell = (group: SearchGroup, request: SearchPageInput) => <DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><SearchGroupSurface group={group} request={request} view="grid" /></MemoryRouter>
    </DataProvider>;
    const view = render(shell(oldGroup, oldRequest));

    fireEvent.click(screen.getByRole('button', { name: 'More movies' }));
    await waitFor(() => expect(resolveContinuation).toBeTypeOf('function'));
    view.rerender(shell(newGroup, newRequest));
    await act(async () => resolveContinuation({ query: 'old', sort: 'relevance', direction: 'desc', groups: [{ ...oldGroup, items: [result('old-more', 'Old continuation')], hasMore: false, nextCursor: null }] }));

    expect(await screen.findByText('New first')).toBeInTheDocument();
    expect(screen.queryByText('Old continuation')).not.toBeInTheDocument();
  });
});
