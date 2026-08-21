import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { PorticoDataSource, Viewer } from '../../data/models';
import { PlaybackSessionProvider } from '../player/PlayerSurface';
import { SavedPage, SavedResourcePage } from './SavedWorkspace';

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
};

function renderSaved(source: FixturePorticoDataSource, entry: string, detail = false) {
  return render(<DataProvider source={source} initialViewer={viewer}>
    <MemoryRouter initialEntries={[entry]}>
      <PlaybackSessionProvider>
        {detail
          ? <Routes><Route path="/saved/:kind/:id" element={<SavedResourcePage />} /></Routes>
          : <SavedPage />}
      </PlaybackSessionProvider>
    </MemoryRouter>
  </DataProvider>);
}

describe('Saved workspace', () => {
  it('discovers share candidates by username without returning the current account', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const result = await source.savedShareCandidates('sam', 20, new AbortController().signal);
    expect(result).toEqual({
      items: [{ userId: 'fixture-sam', displayName: 'Sam Rivera' }],
      hasMore: false,
    });
    expect(result.items.some((candidate) => candidate.userId === viewer.user?.id)).toBe(false);
  });

  it('keeps every Saved resource type addressable from one workspace', async () => {
    renderSaved(new FixturePorticoDataSource(), '/saved?section=collections');
    expect(await screen.findByText('Modern classics')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Playlists' }));
    expect(await screen.findByText('Weekend queue')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Saved views/i }));
    expect(await screen.findByText('Unwatched movies')).toBeInTheDocument();
  });

  it('selects and removes one duplicate playlist entry by its stable entry identity', async () => {
    const source = new FixturePorticoDataSource();
    const mutate = vi.spyOn(source as PorticoDataSource, 'mutateSavedResourceItems');
    renderSaved(source, '/saved/playlists/fixture-playlist-weekend', true);
    await screen.findByRole('heading', { name: 'Weekend queue' });
    const duplicateEntries = screen.getAllByRole('button', { name: /Select Fargo, position/ });
    expect(duplicateEntries).toHaveLength(2);
    fireEvent.click(duplicateEntries[0]);
    fireEvent.click(screen.getByRole('button', { name: 'Remove from playlist' }));
    await waitFor(() => expect(mutate).toHaveBeenCalledWith(
      'playlist',
      'fixture-playlist-weekend',
      expect.objectContaining({ removeEntryIds: ['fixture-playlist-entry-1'], expectedUpdatedAt: expect.any(String) }),
      expect.any(AbortSignal),
    ));
    await waitFor(() => expect(screen.getAllByRole('button', { name: /Select Fargo, position/ })).toHaveLength(1));
  });

  it('sends a complete stable-entry order when a playlist row moves', async () => {
    const source = new FixturePorticoDataSource();
    const mutate = vi.spyOn(source as PorticoDataSource, 'mutateSavedResourceItems');
    renderSaved(source, '/saved/playlists/fixture-playlist-weekend', true);
    await screen.findByRole('heading', { name: 'Weekend queue' });
    fireEvent.click(screen.getByRole('button', { name: 'Move Fargo later, position 1' }));
    await waitFor(() => expect(mutate).toHaveBeenCalledWith(
      'playlist',
      'fixture-playlist-weekend',
      expect.objectContaining({
        orderEntryIds: [
          'fixture-playlist-entry-2',
          'fixture-playlist-entry-1',
          'fixture-playlist-entry-3',
          'fixture-playlist-entry-4',
          'fixture-playlist-entry-5',
          'fixture-playlist-entry-6',
        ],
        expectedUpdatedAt: expect.any(String),
      }),
      expect.any(AbortSignal),
    ));
  });

  it('loads long playlists in pages without collapsing duplicate media entries', async () => {
    const source = new FixturePorticoDataSource();
    const playlist = await source.createSavedResource(
      'playlist',
      { title: 'Long queue', mediaIds: Array.from({ length: 55 }, () => 'fargo') },
      new AbortController().signal,
    );
    const { container } = renderSaved(source, `/saved/playlists/${playlist.id}`, true);
    await screen.findByRole('heading', { name: 'Long queue' });
    await waitFor(() => expect(container.querySelectorAll('.portico-playlist-order .selection-check')).toHaveLength(50));
    fireEvent.click(screen.getByRole('button', { name: 'Load 5 more' }));
    await waitFor(() => expect(container.querySelectorAll('.portico-playlist-order .selection-check')).toHaveLength(55));
  });

  it('lets an owner manage named access and persists the selected permission', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const update = vi.spyOn(source as PorticoDataSource, 'updateSavedResource');
    renderSaved(source, '/saved/playlists/fixture-playlist-weekend', true);
    await screen.findByRole('heading', { name: 'Weekend queue' });
    fireEvent.click(screen.getByRole('button', { name: 'Edit & share' }));
    expect(await screen.findByText('People with access')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Sam Rivera access' }));
    fireEvent.click(await screen.findByRole('option', { name: 'Can edit' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(
      'playlist',
      'fixture-playlist-weekend',
      expect.objectContaining({ shares: [{ userId: 'fixture-sam', canEdit: true }] }),
      expect.any(AbortSignal),
    ));
  });

  it('keeps list collaboration separate from ownership controls', async () => {
    const source = new FixturePorticoDataSource();
    await source.switchBrowserAccount('fixture-sam', new AbortController().signal);
    renderSaved(source, '/saved/playlists/fixture-playlist-weekend', true);
    await screen.findByRole('heading', { name: 'Weekend queue' });
    expect(screen.queryByRole('button', { name: 'Edit & share' })).not.toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: /Select Fargo, position/ }).length).toBeGreaterThan(0);
  });
});
