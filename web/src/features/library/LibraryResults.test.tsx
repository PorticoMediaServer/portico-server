import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem } from '../../data/models';
import { LibraryResults } from './LibraryResults';
import type { LibraryPivotCapability, LibraryPivotPage } from './libraryTypes';

function movie(id: string, title: string): MediaItem {
  return {
    id,
    title,
    subtitle: '2026',
    year: 2026,
    type: 'movie',
    kind: 'movie',
    poster: '',
    backdrop: '',
    rating: '',
    length: '2h',
    genre: 'Drama',
    actions: ['watchlist.add'],
  };
}

const pivot: LibraryPivotCapability = {
  id: 'movies',
  label: 'Movies',
  entityKinds: ['movie'],
  browseSupported: true,
  endpointTemplate: '/api/libraries/{libraryId}/browse',
  defaultSort: [{ field: 'title', direction: 'asc' }],
  defaultView: 'grid',
  supportedViews: ['grid'],
  presentationFields: [],
};

function page(items: MediaItem[]): LibraryPivotPage {
  return {
    items,
    total: items.length,
    hasMore: false,
    applied: { pivot: 'movies', sort: [{ field: 'title', direction: 'asc' }], presentationFields: [] },
    presentation: 'grid',
  };
}

describe('library bulk operations', () => {
  it('clears a successful selection and keeps only failed items selected for retry', async () => {
    const source = new FixturePorticoDataSource();
    const items = [movie('one', 'One'), movie('two', 'Two')];
    vi.spyOn(source, 'setWatchlist').mockImplementation(async (id) => {
      if (id === 'two') throw new Error('Two could not be updated.');
      return { ...items[0], id, watchlisted: true };
    });
    const changed = vi.fn();

    render(<DataProvider source={source}><MemoryRouter><LibraryResults
      library={{ id: 'movies', name: 'Movies', kind: 'movies', itemCount: 2 }}
      pivot={pivot}
      page={page(items)}
      presentation="grid"
      onApplyFacet={() => undefined}
      onChanged={changed}
    /></MemoryRouter></DataProvider>);

    fireEvent.click(screen.getByRole('button', { name: 'Select One' }));
    fireEvent.click(screen.getByRole('button', { name: 'Select Two' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Add to watchlist' }));

    await waitFor(() => expect(screen.getByRole('region', { name: '1 selected items' })).toBeInTheDocument());
    expect(screen.getByText('1 updated; 1 still selected for retry.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deselect Two' })).toBePressed();
    expect(screen.getByRole('button', { name: 'Select One' })).not.toBePressed();
    expect(changed).toHaveBeenCalledTimes(1);
  });

  it('uses canonical presentation artwork shapes instead of legacy view types', () => {
    const episode = { ...movie('episode-one', 'Episode One'), type: 'show' as const, kind: 'episode' as const, entityKind: 'episode' };
    render(<DataProvider source={new FixturePorticoDataSource()}><MemoryRouter><LibraryResults
      library={{ id: 'shows', name: 'Shows', kind: 'tv', itemCount: 1 }}
      pivot={{ ...pivot, id: 'episodes', entityKinds: ['episode'], defaultView: 'table', supportedViews: ['table'] }}
      page={page([episode])}
      presentation="table"
      onApplyFacet={() => undefined}
      onChanged={() => undefined}
    /></MemoryRouter></DataProvider>);

    expect(screen.getByRole('img', { name: 'Episode One artwork unavailable' })).toHaveClass('landscape');
    expect(document.querySelector('.library-table-scroll')).toHaveAttribute('data-retained-result-count', '1');
    expect(document.querySelector('.library-table-scroll')).toHaveAttribute('data-retained-result-budget-state', 'within-budget');
  });
});
