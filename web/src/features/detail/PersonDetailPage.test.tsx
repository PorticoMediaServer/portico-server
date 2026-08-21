import { act, fireEvent, render, screen } from '@testing-library/react';
import { createMemoryRouter, MemoryRouter, Route, RouterProvider, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem } from '../../data/models';
import { PersonDetailPage } from './PersonDetailPage';

describe('person detail', () => {
  it('renders a server-resolved person pivot and accessible credits', async () => {
    class PersonSource extends FixturePorticoDataSource {
      override async person(id: string, signal: AbortSignal) {
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        const credit: MediaItem = { id: 'arrival', title: 'Arrival', subtitle: 'Movie', year: 2016, type: 'movie', kind: 'movie', poster: '/arrival.jpg', backdrop: '', rating: 'PG-13', length: '1h 56m', genre: 'Science fiction', actions: [] };
        return { id, name: 'Amy Adams', imageUrl: '/amy.jpg', knownFor: 'Arrival', credits: [credit], hasMore: false };
      }
    }
    render(<DataProvider source={new PersonSource()}><MemoryRouter initialEntries={['/person/person-amy']}><Routes><Route path="/person/:id" element={<PersonDetailPage />} /></Routes></MemoryRouter></DataProvider>);
    expect(await screen.findByRole('heading', { name: 'Amy Adams' })).toBeInTheDocument();
    expect(screen.getByText('Known for Arrival')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Movies & shows' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Arrival' })).toHaveAttribute('href', '/media/arrival');
  });

  it('does not let an old person continuation overwrite a newer person route', async () => {
    let resolveOldContinuation: ((value: Awaited<ReturnType<FixturePorticoDataSource['person']>>) => void) | undefined;
    const oldContinuation = new Promise<Awaited<ReturnType<FixturePorticoDataSource['person']>>>((resolve) => { resolveOldContinuation = resolve; });
    const credit = (id: string, title: string): MediaItem => ({ id, title, subtitle: 'Movie', year: 2026, type: 'movie', kind: 'movie', poster: '', backdrop: '', rating: '', length: '', genre: '', actions: [] });
    class PersonSource extends FixturePorticoDataSource {
      override async person(id: string, signal: AbortSignal, cursor?: string) {
        if (id === 'person-old' && cursor) return oldContinuation;
        if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
        return id === 'person-new'
          ? { id, name: 'New Person', credits: [credit('new-credit', 'New Credit')], hasMore: false }
          : { id, name: 'Old Person', credits: [credit('old-credit', 'Old Credit')], hasMore: true, nextCursor: 'old-next' };
      }
    }
    const router = createMemoryRouter([{ path: '/person/:id', element: <PersonDetailPage /> }], { initialEntries: ['/person/person-old'] });
    render(<DataProvider source={new PersonSource()}><RouterProvider router={router} /></DataProvider>);

    expect(await screen.findByRole('heading', { name: 'Old Person' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'More credits' }));
    await router.navigate('/person/person-new');
    expect(await screen.findByRole('heading', { name: 'New Person' })).toBeInTheDocument();

    await act(async () => resolveOldContinuation?.({ id: 'person-old', name: 'Old Person', credits: [credit('late-credit', 'Late Old Credit')], hasMore: false }));
    expect(screen.queryByText('Late Old Credit')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'New Person' })).toBeInTheDocument();
  });
});
