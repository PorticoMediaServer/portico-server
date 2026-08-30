import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem, Viewer } from '../../data/models';
import { QuickSearchPanel } from './QuickSearchPanel';

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
  viewerScope: { authority: 'local', accountId: 'viewer', serverId: 'server', profileId: 'profile', authorizationRevision: '1' },
};

const movie: MediaItem = {
  id: 'healthy-movie', title: 'Healthy movie result', subtitle: '', year: 2026, entityKind: 'movie', poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [],
};

describe('QuickSearchPanel', () => {
  it('keeps healthy groups visible when a sibling group reports a Product Language failure', async () => {
    const source = new FixturePorticoDataSource(viewer);
    vi.spyOn(source, 'searchPage').mockResolvedValue({
      query: 'Fargo',
      sort: 'relevance',
      direction: 'desc',
      groups: [
        { id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [movie], hasMore: false, nextCursor: null },
        { id: 'shows', title: 'Shows', entityKind: 'show', status: 'error', errorCode: 'search_group_timeout', messageId: 'search.group-timeout', items: [], hasMore: false, nextCursor: null },
      ],
    });

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><QuickSearchPanel query="Fargo" serverName="Portico Test" onSelect={vi.fn()} onViewAll={vi.fn()} /></MemoryRouter>
    </DataProvider>);

    expect(await screen.findByText('Healthy movie result')).toBeInTheDocument();
    expect(screen.getByText("This result group didn't finish in time. The other search results are still available.")).toBeInTheDocument();
    expect(screen.queryByText(/No results/)).not.toBeInTheDocument();
  });

  it('opens all members of a broad Music group with one canonical kind query', async () => {
    const source = new FixturePorticoDataSource(viewer);
    vi.spyOn(source, 'searchPage').mockResolvedValue({
      query: 'Blue',
      sort: 'relevance',
      direction: 'desc',
      groups: [{ id: 'music', title: 'Music', entityKind: 'track', status: 'success', items: [
        { ...movie, id: 'album-blue', title: 'Blue', entityKind: 'album' },
      ], hasMore: false, nextCursor: null }],
    });

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><QuickSearchPanel query="Blue" serverName="Portico Test" onSelect={vi.fn()} onViewAll={vi.fn()} /></MemoryRouter>
    </DataProvider>);

    expect(await screen.findByRole('link', { name: /View all/ })).toHaveAttribute('href', '/search?q=Blue&types=track%2Cartist%2Calbum');
  });

  it('exposes results as ordinary links for the surrounding search dialog', async () => {
    const source = new FixturePorticoDataSource(viewer);
    vi.spyOn(source, 'searchPage').mockResolvedValue({
      query: 'Healthy',
      sort: 'relevance',
      direction: 'desc',
      groups: [{ id: 'movies', title: 'Movies', entityKind: 'movie', status: 'success', items: [movie], hasMore: false, nextCursor: null }],
    });
    const onActiveOptionChange = vi.fn();

    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><QuickSearchPanel query="Healthy" serverName="Portico Test" activeOptionId="unselected" onActiveOptionChange={onActiveOptionChange} onSelect={vi.fn()} onViewAll={vi.fn()} /></MemoryRouter>
    </DataProvider>);

    const result = await screen.findByRole('link', { name: /Healthy movie result/ });
    expect(result).toHaveAttribute('data-search-result');
    expect(result).toHaveAttribute('tabindex', '-1');
    expect(result).not.toHaveAttribute('role', 'option');
    expect(screen.queryByRole('option')).not.toBeInTheDocument();
    fireEvent.mouseEnter(result);
    expect(onActiveOptionChange).toHaveBeenCalledWith(result.id);
  });
});
