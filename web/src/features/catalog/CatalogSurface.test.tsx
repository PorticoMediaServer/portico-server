import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem, Viewer } from '../../data/models';
import { targetFromMedia } from '../media/contextTarget';
import { mediaDetailPath, MediaActionMenu, MediaCard, MediaListRow, resolveWebMediaDetailViewModel } from './CatalogSurface';

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
};

function media(actions: string[], watched = false): MediaItem {
  return {
    id: 'movie-1',
    title: 'Arrival',
    subtitle: '',
    year: 2016,
    type: 'movie',
    kind: 'movie',
    poster: '',
    backdrop: '',
    rating: '',
    length: '1h 56m',
    genre: 'Science fiction',
    actions,
    watched,
  };
}

function renderMenu(item: MediaItem, activeViewer = viewer, source = new FixturePorticoDataSource(activeViewer)) {
  return render(<DataProvider source={source} initialViewer={activeViewer}>
    <MemoryRouter><MediaActionMenu target={targetFromMedia(item)} /></MemoryRouter>
  </DataProvider>);
}

describe('catalog action contracts', () => {
  it('routes synthetic collection and playlist summaries to their real resource details', () => {
    expect(mediaDetailPath({ ...media([]), id: 'collection/1', kind: 'collection', entityKind: 'collection' })).toBe('/saved/collections/collection%2F1');
    expect(mediaDetailPath({ ...media([]), id: 'playlist/1', kind: 'playlist', entityKind: 'playlist' })).toBe('/saved/playlists/playlist%2F1');
  });

  it('resolves card artwork, destination, and selection accessibility copy through Product Language', () => {
    render(<DataProvider source={new FixturePorticoDataSource(viewer)} initialViewer={viewer}>
      <MemoryRouter><MediaCard item={media([])} onSelect={vi.fn()} /></MemoryRouter>
    </DataProvider>);

    expect(screen.getByRole('img', { name: 'Arrival artwork unavailable' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Arrival' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select Arrival' })).toBeInTheDocument();
  });

  it('consumes Client Core canonical availability for full detail resources', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const contract = await source.productContract(new AbortController().signal);
    const view = resolveWebMediaDetailViewModel(contract, {
      ...media([]),
      missing: false,
      fileCount: 4,
      missingFileCount: 1,
    });
    expect(view.availability).toEqual({ status: 'partial', fileCount: 4, missingFileCount: 1 });
  });

  it('recognizes the directional watched actions emitted by the server', async () => {
    const marked = renderMenu(media(['watched.mark']));
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(await screen.findByRole('button', { name: 'Mark as watched' })).toBeInTheDocument();
    marked.unmount();

    renderMenu(media(['watched.unmark'], true));
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(await screen.findByRole('button', { name: 'Mark as unwatched' })).toBeInTheDocument();
  });

  it('offers restart only when play.from-beginning is server-advertised', async () => {
    const inferred = renderMenu({ ...media(['play'], true), progressSeconds: 240 });
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(screen.queryByRole('button', { name: 'Play from beginning' })).not.toBeInTheDocument();
    inferred.unmount();

    renderMenu({ ...media(['play', 'play.from-beginning'], true), progressSeconds: 240 });
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(await screen.findByRole('button', { name: 'Play from beginning' })).toBeInTheDocument();
  });

  it('offers Watch With Friends only from the server-projected media action', async () => {
    const inferred = renderMenu(media(['play']));
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(screen.queryByRole('button', { name: 'Watch with friends' })).not.toBeInTheDocument();
    inferred.unmount();

    renderMenu(media(['play', 'watch-with-friends.start']));
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(await screen.findByRole('button', { name: 'Watch with friends' })).toBeInTheDocument();
  });

  it('keeps web-admin editors behind both server projection and profile permission', async () => {
    const ordinary = renderMenu(media(['metadata.edit']));
    expect(screen.queryByRole('button', { name: 'More actions for Arrival' })).not.toBeInTheDocument();
    ordinary.unmount();

    const administrator: Viewer = {
      ...viewer,
      user: { ...viewer.user!, permissions: { editMetadata: true } },
    };
    renderMenu(media(['metadata.edit']), administrator);
    fireEvent.click(await screen.findByRole('button', { name: 'More actions for Arrival' }));
    expect(await screen.findByRole('button', { name: 'Edit metadata' })).toBeInTheDocument();
  });

  it('re-resolves card actions from the mutation response instead of retaining stale labels', async () => {
    const source = new FixturePorticoDataSource(viewer);
    vi.spyOn(source, 'setWatchlist').mockResolvedValue({ ...media(['watchlist.remove']), watchlisted: true });
    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><MediaCard item={media(['watchlist.add'])} /></MemoryRouter>
    </DataProvider>);

    fireEvent.click(await screen.findByRole('button', { name: 'Add to watchlist Arrival' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Remove from watchlist Arrival' })).toBeInTheDocument());
    expect(source.setWatchlist).toHaveBeenCalledWith('movie-1', true, expect.any(AbortSignal));
  });

  it('treats the mutation response as authoritative when the server declines an optimistic save', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const onWatchlistChange = vi.fn();
    vi.spyOn(source, 'setWatchlist').mockResolvedValue({ ...media(['watchlist.add']), watchlisted: false });
    render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><MediaCard item={media(['watchlist.add'])} onWatchlistChange={onWatchlistChange} /></MemoryRouter>
    </DataProvider>);

    fireEvent.click(await screen.findByRole('button', { name: 'Add to watchlist Arrival' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Add to watchlist Arrival' })).toBeInTheDocument());
    expect(onWatchlistChange).toHaveBeenCalledWith('movie-1', false);
  });

  it('uses canonical landscape artwork and a truthful details affordance for episodes', () => {
    const episode = { ...media([]), kind: 'episode', entityKind: 'episode', title: 'The Arrival' };
    const { container } = render(<DataProvider source={new FixturePorticoDataSource(viewer)} initialViewer={viewer}>
      <MemoryRouter><MediaListRow item={episode} /></MemoryRouter>
    </DataProvider>);

    expect(container.querySelector('.catalog-list-art > .landscape')).toBeInTheDocument();
    expect(container.querySelector('.list-detail')).toBeInTheDocument();
    expect(container.querySelector('.list-play')).not.toBeInTheDocument();
  });

  it('renders future kinds with Product Contract artwork roles and geometry', async () => {
    const source = new FixturePorticoDataSource(viewer);
    const base = await source.productContract(new AbortController().signal);
    vi.spyOn(source, 'productContract').mockResolvedValue({
      ...base,
      entitySemantics: [{
        id: 'spatial-channel',
        container: false,
        playable: true,
        parentKinds: [],
        childKinds: [],
        childOrder: ['id'],
        defaultDestination: 'detail',
        primaryArtworkRole: 'logo',
      }],
      artworkRoles: [{ id: 'logo', aspectRatio: 16 / 9, fit: 'contain', purpose: 'Channel identity' }],
    });
    const item: MediaItem = {
      ...media([]),
      id: 'future-channel-1',
      title: 'Future Channel',
      entityKind: 'spatial-channel',
      kind: 'spatial-channel',
      poster: '',
      artwork: { logo: '/future-channel-logo.svg' },
    };

    const rendered = render(<DataProvider source={source} initialViewer={viewer}>
      <MemoryRouter><MediaCard item={item} /></MemoryRouter>
    </DataProvider>);

    await waitFor(() => expect(rendered.container.querySelector('img[data-artwork-role="logo"]')).toHaveAttribute('src', '/future-channel-logo.svg'));
    expect(rendered.container.querySelector('.artwork-stage')).toHaveStyle({ aspectRatio: `${16 / 9}` });
    expect(rendered.container.querySelector('img[data-artwork-role="logo"]')).toHaveStyle({ objectFit: 'contain' });
  });
});
