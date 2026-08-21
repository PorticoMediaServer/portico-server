import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { MediaItem, Viewer } from '../../data/models';
import { PlaybackSessionProvider } from '../player/PlayerSurface';
import { DetailPage } from './DetailPage';
import { musicRecommendationItems, musicRecommendationRows } from './DetailSections';

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
};

function media(overrides: Partial<MediaItem> & Pick<MediaItem, 'id' | 'title'>): MediaItem {
  return {
    subtitle: '',
    year: 0,
    type: 'movie',
    kind: 'movie',
    poster: '/poster.jpg',
    backdrop: '/backdrop.jpg',
    rating: '',
    length: '',
    genre: '',
    availability: 'available',
    actions: [],
    ...overrides,
  };
}

class DetailSource extends FixturePorticoDataSource {
  readonly records: Map<string, MediaItem>;
  failures = new Map<string, number>();

  constructor(records: MediaItem[]) {
    super(viewer);
    this.records = new Map(records.map((item) => [item.id, item]));
  }

  override async media(id: string, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const remaining = this.failures.get(id) ?? 0;
    if (remaining > 0) {
      this.failures.set(id, remaining - 1);
      throw new Error('The server connection was interrupted.');
    }
    const item = this.records.get(id);
    if (!item) throw new Error('This media item is no longer available.');
    return structuredClone(item);
  }
}

function renderDetail(source: FixturePorticoDataSource, id: string, search = '') {
  return render(<DataProvider source={source} initialViewer={viewer}>
    <MemoryRouter initialEntries={[`/media/${id}${search}`]}>
      <PlaybackSessionProvider>
        <Routes><Route path="/media/:id" element={<DetailPage />} /></Routes>
      </PlaybackSessionProvider>
    </MemoryRouter>
  </DataProvider>);
}

describe('media detail workspace', () => {
  it('presents movie provenance, availability, discovery, and capability-gated actions together', async () => {
    const trailer = media({ id: 'trailer', title: 'Official trailer', kind: 'movie', year: 2026 });
    const recommendation = media({ id: 'recommended', title: 'Solaris', year: 1972 });
    const movie = media({
      id: 'arrival',
      title: 'Arrival',
      libraryId: 'movies',
      year: 2016,
      rating: 'PG-13',
      length: '1h 56m',
      genre: 'Science fiction',
      edition: 'Collector’s Edition',
      summary: 'A linguist works to understand visitors whose arrival changes the world.',
      availability: 'partial',
      fileCount: 3,
      missingFileCount: 1,
      actions: ['play', 'watchlist.add', 'favorite.add', 'watched.set', 'collection.add', 'metadata.edit'],
      people: [{ name: 'Amy Adams', role: 'Actor', character: 'Louise Banks', imageUrl: '/amy.jpg', sortOrder: 1 }, { id: 'person_ZGVuaXM', name: 'Denis Villeneuve', role: 'Director', sortOrder: 2 }],
      extras: [{ label: 'Trailers', type: 'trailer', items: [trailer] }],
      recommendationRows: [{ id: 'related', title: 'More like this', type: 'poster', items: [recommendation], hasMore: false }],
      streams: [
        { id: 'video', kind: 'video', codec: 'hevc', displayTitle: '4K HDR', height: 2160, dynamicRange: 'HDR10' },
        { id: 'audio', kind: 'audio', codec: 'eac3', displayTitle: 'English', language: 'English', channels: 6 },
      ],
      mediaFiles: [
        { id: 'arrival-4k', versionLabel: '4K HDR', resolution: '2160p', videoCodec: 'hevc', audioCodec: 'eac3', dynamicRange: 'HDR10', container: 'mkv', available: true, selected: true },
        { id: 'arrival-1080p', versionLabel: '1080p SDR', resolution: '1080p', videoCodec: 'h264', audioCodec: 'aac', container: 'mp4', available: true },
      ],
      optimizedVersions: [{ id: 'mobile', profile: 'mobile-1080p', profileName: 'Mobile 1080p', sizeBytes: 2_147_483_648, available: true, createdAt: '2026-07-01T12:00:00Z', updatedAt: '2026-07-02T12:00:00Z' }],
    });
    const { container } = renderDetail(new DetailSource([movie, trailer, recommendation]), movie.id);

    expect(await screen.findByRole('heading', { name: 'Arrival' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Movies' })).toHaveAttribute('href', '/library/movies');
    expect(screen.getByText('Movie')).toBeInTheDocument();
    expect(screen.getByText(/Collector’s Edition · 2016 · PG-13/)).toBeInTheDocument();
    expect(container.querySelector('.portico-detail-art')).toHaveClass('poster');
    expect(await screen.findByRole('button', { name: 'Play' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add to watchlist' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add to favorites' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mark as watched' })).toBeInTheDocument();
    expect(screen.getByText('2 of 3 catalogued files are available. Portico will use an available version for playback.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Movies' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Cast & Crew' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Amy Adams' })).toHaveAttribute('href', '/search?q=Amy%20Adams');
    expect(screen.getByRole('link', { name: 'Open Amy Adams' }).querySelector('img')).toHaveAttribute('src', '/amy.jpg');
    expect(screen.getByRole('link', { name: 'Open Denis Villeneuve' })).toHaveAttribute('href', '/person/person_ZGVuaXM');
    expect(screen.getByRole('link', { name: 'Open Denis Villeneuve' })).toHaveTextContent('DV');
    expect(screen.getByRole('heading', { name: 'Trailers' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'More like this' })).toBeInTheDocument();
    expect(screen.getByText('Versions & media information')).toBeInTheDocument();
    expect(screen.getByText('Mobile 1080p')).toBeInTheDocument();
    expect(screen.getByText('HEVC · 2160p · HDR10')).toBeInTheDocument();
    const information = container.querySelector('.portico-technical-details');
    const cast = container.querySelector('.portico-people-section');
    const related = screen.getByRole('heading', { name: 'More like this' }).closest('section');
    expect(information).not.toBeNull();
    expect(cast).not.toBeNull();
    expect(related).not.toBeNull();
    expect(information!.compareDocumentPosition(cast!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(cast!.compareDocumentPosition(related!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'More actions for Arrival' }));
    fireEvent.click(screen.getByRole('button', { name: 'Play version' }));
    expect(screen.getByRole('heading', { name: 'Play version' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /4K HDR/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /1080p SDR/ })).toBeInTheDocument();
  });

  it('re-resolves detail actions from mutation-returned server eligibility', async () => {
    const movie = media({ id: 'arrival-actions', title: 'Arrival', actions: ['watchlist.add'] });
    const source = new DetailSource([movie]);
    vi.spyOn(source, 'setWatchlist').mockResolvedValue({ ...movie, watchlisted: true, actions: ['watchlist.remove'] });
    renderDetail(source, movie.id);

    fireEvent.click(await screen.findByRole('button', { name: 'Add to watchlist' }));

    await waitFor(() => expect(screen.getByRole('button', { name: 'Remove from watchlist' })).toBeInTheDocument());
    expect(source.setWatchlist).toHaveBeenCalledWith(movie.id, true, expect.any(AbortSignal));
  });

  it('keeps the show hero while selecting the resume season and rendering complete episode rows', async () => {
    const pilot = media({ id: 'episode-1', title: 'Pilot', type: 'show', kind: 'episode', parentId: 'season-1', grandparentId: 'fargo', seasonNumber: 1, episodeNumber: 1, length: '52m', summary: 'A drifter arrives in Bemidji.', actions: ['play', 'watchlist.add'] });
    const palindrome = media({ id: 'episode-2', title: 'Palindrome', type: 'show', kind: 'episode', parentId: 'season-2', grandparentId: 'fargo', seasonNumber: 2, episodeNumber: 10, length: '48m', summary: 'The case closes in on its final confrontation.', progress: 42, progressSeconds: 270, actions: ['play', 'watched.set'] });
    const firstSeason = media({ id: 'season-1', title: 'Season 1', type: 'show', kind: 'season', seasonNumber: 1, children: [pilot] });
    const secondSeason = media({ id: 'season-2', title: 'Season 2', type: 'show', kind: 'season', seasonNumber: 2, children: [palindrome] });
    const secondSeasonDetail = secondSeason;
    const show = media({
      id: 'fargo',
      title: 'Fargo',
      type: 'show',
      kind: 'show',
      libraryId: 'tv',
      progress: 35,
      actions: ['watchlist.add', 'watched.set'],
      children: [secondSeason, firstSeason],
    });
    renderDetail(new DetailSource([show, firstSeason, secondSeasonDetail, pilot, palindrome]), show.id);

    expect(await screen.findByRole('heading', { name: 'Fargo' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Season' })).toBeInTheDocument();
    expect(screen.getByText('Palindrome')).toBeInTheDocument();
    expect(screen.getByText('The case closes in on its final confrontation.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Resume from 4:30/ })).toBeInTheDocument();
    expect(screen.queryByText(/% watched/)).not.toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Resume Palindrome' })).toHaveAttribute('href', '/watch/episode-2');

    fireEvent.click(screen.getByRole('button', { name: 'Season' }));
    fireEvent.click(screen.getByRole('option', { name: 'Season 1' }));
    expect(await screen.findByText('Pilot')).toBeInTheDocument();
    expect(screen.getByText('A drifter arrives in Bemidji.')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Open Season/ })).not.toBeInTheDocument();
  });

  it('resolves durable season and episode links into the canonical show workspace', async () => {
    const episode = media({ id: 'episode-link', title: 'The Castle', type: 'show', kind: 'episode', parentId: 'season-link', parentTitle: 'Season 2', grandparentId: 'fargo-link', grandparentTitle: 'Fargo', seasonNumber: 2, episodeNumber: 9, actions: ['play'] });
    const season = media({ id: 'season-link', title: 'Season 2', type: 'show', kind: 'season', parentId: 'fargo-link', parentTitle: 'Fargo', seasonNumber: 2, children: [episode] });
    const show = media({ id: 'fargo-link', title: 'Fargo', type: 'show', kind: 'show', children: [season], actions: ['play'] });
    renderDetail(new DetailSource([show, season, episode]), episode.id);

    expect(await screen.findByRole('heading', { name: 'Fargo' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /S2 E9The Castle/ })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('heading', { name: 'The Castle' })).not.toBeInTheDocument();
  });

  it('renders the server-selected show playback target when season children are shallow', async () => {
    const episode = media({ id: 'episode-target', title: 'The Castle', type: 'show', kind: 'episode', parentId: 'season-target', grandparentId: 'fargo-target', seasonNumber: 2, episodeNumber: 9, length: '48m', progressSeconds: 330, actions: ['play'] });
    const shallowSeason = media({ id: 'season-target', title: 'Season 2', type: 'show', kind: 'season', parentId: 'fargo-target', seasonNumber: 2 });
    const seasonDetail = { ...shallowSeason, children: [episode] };
    const show = media({ id: 'fargo-target', title: 'Fargo', type: 'show', kind: 'show', children: [shallowSeason], playbackTarget: episode });
    renderDetail(new DetailSource([show, seasonDetail, episode]), show.id);

    expect(await screen.findByRole('button', { name: 'Resume S2E9' })).toBeInTheDocument();
    expect(await screen.findByText('The Castle')).toBeInTheDocument();
  });

  it('keeps contextual resume copy when the server-projected play action gates eligibility', async () => {
    const episode = media({ id: 'episode-context', title: 'The Return', type: 'show', kind: 'episode', parentId: 'season-context', grandparentId: 'show-context', seasonNumber: 3, episodeNumber: 7, progressSeconds: 91, actions: ['play'] });
    const season = media({ id: 'season-context', title: 'Season 3', type: 'show', kind: 'season', seasonNumber: 3, children: [episode] });
    const show = media({ id: 'show-context', title: 'Contextual Show', type: 'show', kind: 'show', children: [season], playbackTarget: episode });
    renderDetail(new DetailSource([show, season, episode]), show.id);

    expect(await screen.findByRole('button', { name: 'Resume S3E7' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Play' })).not.toBeInTheDocument();
  });

  it('follows episode cursors until a deep-linked episode beyond the initial 200 is loaded and selected', async () => {
    const season = media({ id: 'season-deep', title: 'Season 1', type: 'show', kind: 'season', seasonNumber: 1 });
    const show = media({ id: 'show-deep', title: 'Deep Link Show', type: 'show', kind: 'show', children: [season] });
    const initialEpisodes = Array.from({ length: 3 }, (_, index) => media({
      id: `episode-deep-${index + 1}`,
      title: `Episode ${index + 1}`,
      type: 'show',
      kind: 'episode',
      parentId: season.id,
      grandparentId: show.id,
      seasonNumber: 1,
      episodeNumber: index + 1,
      actions: ['play'],
    }));
    const requested = media({ id: 'episode-deep-201', title: 'The Deep Link', type: 'show', kind: 'episode', parentId: season.id, grandparentId: show.id, seasonNumber: 1, episodeNumber: 201, actions: ['play'] });
    const source = new DetailSource([show, season, ...initialEpisodes, requested]);
    const children = vi.spyOn(source, 'mediaChildren')
      .mockResolvedValueOnce({ items: initialEpisodes, hasMore: true, nextCursor: 'after-200' })
      .mockResolvedValueOnce({ items: [requested], hasMore: false, nextCursor: null });

    renderDetail(source, show.id, `?season=${season.id}&episode=${requested.id}`);

    expect(await screen.findByRole('button', { name: /S1 E201The Deep Link/ }, { timeout: 3_000 })).toHaveAttribute('aria-pressed', 'true');
    expect(children).toHaveBeenNthCalledWith(1, season.id, expect.any(AbortSignal), undefined, 200);
    expect(children).toHaveBeenNthCalledWith(2, season.id, expect.any(AbortSignal), 'after-200', 200);
  });

  it('completes truncated show and season hierarchy through cursor child pages', async () => {
    const pilot = media({ id: 'episode-page-1', title: 'Pilot', type: 'show', kind: 'episode', parentId: 'season-page', grandparentId: 'show-page', seasonNumber: 1, episodeNumber: 1, actions: ['play'] });
    const second = media({ id: 'episode-page-2', title: 'Second Chapter', type: 'show', kind: 'episode', parentId: 'season-page', grandparentId: 'show-page', seasonNumber: 1, episodeNumber: 2, actions: ['play'] });
    const season = media({ id: 'season-page', title: 'Season 1', type: 'show', kind: 'season', seasonNumber: 1, children: [pilot], childrenTruncated: true });
    const show = media({ id: 'show-page', title: 'A Complete Show', type: 'show', kind: 'show', children: [season], childrenTruncated: true });
    const source = new DetailSource([show, season, pilot, second]);
    const children = vi.spyOn(source, 'mediaChildren').mockImplementation(async (id) => id === show.id
      ? { items: [{ ...season, children: undefined }], hasMore: false, nextCursor: null }
      : { items: [pilot, second], hasMore: false, nextCursor: null });

    renderDetail(source, show.id);

    expect(await screen.findByText('Second Chapter')).toBeInTheDocument();
    expect(children).toHaveBeenCalledWith(show.id, expect.any(AbortSignal), undefined, 200);
    expect(children).toHaveBeenCalledWith(season.id, expect.any(AbortSignal), undefined, 200);
  });

  it('loads further truncated album children without discarding the current page', async () => {
    const first = media({ id: 'track-page-1', title: 'First Track', type: 'music', kind: 'track', actions: ['play'] });
    const second = media({ id: 'track-page-2', title: 'Second Track', type: 'music', kind: 'track', actions: ['play'] });
    const third = media({ id: 'track-page-3', title: 'Third Track', type: 'music', kind: 'track', actions: ['play'] });
    const album = media({ id: 'album-page', title: 'Long Album', type: 'music', kind: 'album', children: [first], childrenTruncated: true });
    const source = new DetailSource([album, first, second, third]);
    const children = vi.spyOn(source, 'mediaChildren')
      .mockResolvedValueOnce({ items: [first, second], hasMore: true, nextCursor: 'track-cursor' })
      .mockResolvedValueOnce({ items: [third], hasMore: false, nextCursor: null });

    renderDetail(source, album.id);

    expect(await screen.findByText('Second Track')).toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: 'More tracks' }));
    expect(await screen.findByText('Third Track')).toBeInTheDocument();
    expect(screen.getByText('First Track')).toBeInTheDocument();
    expect(children).toHaveBeenNthCalledWith(2, album.id, expect.any(AbortSignal), 'track-cursor', 200);
  });

  it('keeps loaded hierarchy children visible when a continuation fails', async () => {
    const first = media({ id: 'track-stable-1', title: 'Loaded Track', type: 'music', kind: 'track' });
    const album = media({ id: 'album-stable', title: 'Stable Album', type: 'music', kind: 'album', children: [first], childrenTruncated: true });
    const source = new DetailSource([album, first]);
    vi.spyOn(source, 'mediaChildren')
      .mockResolvedValueOnce({ items: [first], hasMore: true, nextCursor: 'next-page' })
      .mockRejectedValueOnce(new Error('Continuation expired.'));
    renderDetail(source, album.id);

    await screen.findByText('Loaded Track');
    fireEvent.click(await screen.findByRole('button', { name: 'More tracks' }));
    expect(await screen.findByRole('alert')).toHaveTextContent("Portico couldn't complete this request");
    expect(screen.getByText('Loaded Track')).toBeInTheDocument();
  });

  it('selects duplicate season labels by stable season identity', async () => {
    const firstEpisode = media({ id: 'duplicate-episode-1', title: 'First identity', type: 'show', kind: 'episode' });
    const secondEpisode = media({ id: 'duplicate-episode-2', title: 'Second identity', type: 'show', kind: 'episode' });
    const firstSeason = media({ id: 'duplicate-season-1', title: 'Season 1', type: 'show', kind: 'season', seasonNumber: 1, children: [firstEpisode] });
    const secondSeason = media({ id: 'duplicate-season-2', title: 'Season 1', type: 'show', kind: 'season', seasonNumber: 2, children: [secondEpisode] });
    const show = media({ id: 'duplicate-show', title: 'Duplicate seasons', type: 'show', kind: 'show', children: [firstSeason, secondSeason] });
    renderDetail(new DetailSource([show, firstSeason, secondSeason, firstEpisode, secondEpisode]), show.id);

    await screen.findByText('First identity');
    fireEvent.click(screen.getByRole('button', { name: 'Season' }));
    fireEvent.click(screen.getAllByRole('option', { name: 'Season 1' })[1]);
    expect(await screen.findByText('Second identity')).toBeInTheDocument();
    expect(screen.queryByText('First identity')).not.toBeInTheDocument();
  });

  it('uses square artwork only for true music and preserves audiobook lineage as poster media', async () => {
    const track = media({ id: 'track-1', title: 'Kiara', type: 'music', kind: 'track', length: '3:50', actions: ['play'], typedMetadata: { trackNumber: '2', trackArtist: 'Bonobo' } });
    const firstTrack = media({ id: 'track-0', title: 'Prelude', type: 'music', kind: 'track', length: '1:18', actions: ['play'], typedMetadata: { trackNumber: '1', trackArtist: 'Bonobo' } });
    const album = media({ id: 'album', title: 'Black Sands', type: 'music', kind: 'album', parentId: 'artist', parentTitle: 'Bonobo', year: 2010, children: [track, firstTrack] });
    const albumSource = new DetailSource([album, track]);
    const startPlayback = vi.spyOn(albumSource, 'startPlayback').mockRejectedValue(new Error('Playback response omitted in this interaction test.'));
    const albumRender = renderDetail(albumSource, album.id);
    expect(await screen.findByRole('heading', { name: 'Black Sands' })).toBeInTheDocument();
    expect(albumRender.container.querySelector('.portico-detail-art')).toHaveClass('square');
    expect(screen.getByRole('heading', { name: 'Tracks' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Name' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Artist' })).toBeInTheDocument();
    expect(screen.getAllByRole('row')[1]).toHaveTextContent('1PreludeBonobo1:18');
    expect(screen.getAllByRole('row')[2]).toHaveTextContent('2KiaraBonobo3:50');
    const trackPlay = await screen.findByRole('button', { name: 'Play Kiara' });
    expect(trackPlay).not.toHaveAttribute('href');
    fireEvent.click(trackPlay);
    await waitFor(() => expect(startPlayback).toHaveBeenCalledWith('track-1', expect.objectContaining({ queueMediaIds: ['track-0', 'track-1'] }), expect.any(AbortSignal)));
    albumRender.unmount();

    const chapter = media({ id: 'chapter-1', title: 'Chapter 1', type: 'music', kind: 'track', entityKind: 'audiobook-chapter', length: '26m' });
    const audiobook = media({
      id: 'audiobook',
      title: 'Project Hail Mary',
      type: 'music',
      kind: 'book',
      entityKind: 'audiobook',
      libraryId: 'books',
      parentId: 'series',
      parentTitle: 'Hail Mary',
      grandparentId: 'author',
      grandparentTitle: 'Andy Weir',
      length: '16h 10m',
      actions: ['play', 'watchlist.add', 'favorite.add', 'watched.set'],
      typedMetadata: { author: 'Andy Weir', narrator: 'Ray Porter', series: 'Hail Mary', seriesPosition: '1' },
      chapters: [
        { id: 'chapter-opening', title: 'Opening', startSeconds: 0, endSeconds: 320 },
        { id: 'chapter-journey', title: 'The journey begins', startSeconds: 320, endSeconds: 900 },
      ],
    });
    const audiobookRender = renderDetail(new DetailSource([audiobook, chapter]), audiobook.id);
    expect(await screen.findByRole('heading', { name: 'Project Hail Mary' })).toBeInTheDocument();
    expect(audiobookRender.container.querySelector('.portico-detail-art')).toHaveClass('poster');
    expect(screen.getByText('Audiobook')).toBeInTheDocument();
    expect(screen.getByText(/Andy Weir · Narrated by Ray Porter · Hail Mary · Book 1/)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Chapters' })).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Play The journey begins' })).toHaveAttribute('href', '/watch/audiobook');
    expect(screen.getByText('5:20')).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'Play audiobook' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Mark finished' })).toBeInTheDocument();
  });

  it('collapses music recommendation tracks into one album-level card per release', () => {
    const first = media({ id: 'track-a', title: 'First song', type: 'music', kind: 'track', parentId: 'album-a', parentTitle: 'Shared album', grandparentTitle: 'The Artist', poster: '/album.jpg' });
    const second = media({ id: 'track-b', title: 'Second song', type: 'music', kind: 'track', parentId: 'album-a', parentTitle: 'Shared album', grandparentTitle: 'The Artist', poster: '/album.jpg' });
    const other = media({ id: 'track-c', title: 'Other song', type: 'music', kind: 'track', parentId: 'album-b', parentTitle: 'Other album', grandparentTitle: 'The Artist' });

    expect(musicRecommendationItems([first, second, other])).toMatchObject([
      { id: 'album-a', title: 'Shared album', kind: 'album', subtitle: 'The Artist' },
      { id: 'album-b', title: 'Other album', kind: 'album', subtitle: 'The Artist' },
    ]);

    const currentAlbum = media({ id: 'album-a', title: 'Shared album', type: 'music', kind: 'album' });
    expect(musicRecommendationRows([
      { id: 'related', title: 'Related Music', type: 'square', items: [first, second, other], hasMore: false },
      { id: 'artist', title: 'More from The Artist', type: 'square', items: [first, other], hasMore: false },
    ], currentAlbum)).toMatchObject([
      { id: 'related', items: [{ id: 'album-b', title: 'Other album' }] },
      { id: 'artist', items: [] },
    ]);
  });

  it('uses entity-specific hierarchy and action language for saved, live, and recorded media', async () => {
    const child = media({ id: 'child', title: 'Moonlight', year: 2016 });
    const cases: Array<{ item: MediaItem; section?: string; action?: string; kind: string }> = [
      { item: media({ id: 'author', title: 'Ursula K. Le Guin', type: 'music', kind: 'author', children: [media({ id: 'book', title: 'The Dispossessed', type: 'music', kind: 'book', entityKind: 'audiobook' })] }), section: 'Audiobooks', action: undefined, kind: 'Author' },
      { item: media({ id: 'collection', title: 'Modern classics', kind: 'collection', children: [child] }), section: 'Included media', kind: 'Collection' },
      { item: media({ id: 'playlist', title: 'Weekend queue', kind: 'playlist', children: [child], actions: ['play'] }), section: 'Playlist', action: 'Play playlist', kind: 'Playlist' },
      { item: media({ id: 'channel', title: 'Kanal 7', type: 'show', kind: 'category', entityKind: 'live-channel', children: [media({ id: 'program', title: 'Evening News', type: 'show', kind: 'episode' })], actions: ['live.play'] }), section: 'Programming', action: 'Watch live', kind: 'Live channel' },
      { item: media({ id: 'recording', title: 'Saturday Cinema', type: 'show', kind: 'recording', actions: ['dvr.play'], fileCount: 1 }), action: 'Play recording', kind: 'DVR recording' },
    ];

    for (const current of cases) {
      const related = current.item.children ?? [];
      const view = renderDetail(new DetailSource([current.item, ...related, child]), current.item.id);
      expect(await screen.findByRole('heading', { name: current.item.title })).toBeInTheDocument();
      expect(view.container.querySelector('.portico-detail-kind')).toHaveTextContent(current.kind);
      if (current.section) expect(screen.getByRole('heading', { name: current.section })).toBeInTheDocument();
      if (current.action) expect(await screen.findByRole('button', { name: current.action })).toBeInTheDocument();
      view.unmount();
    }
  });

  it('does not infer controls and distinguishes an explicit empty container', async () => {
    const show = media({ id: 'empty-show', title: 'Unscanned Show', type: 'show', kind: 'show', progressSeconds: 90, children: [], actions: [] });
    renderDetail(new DetailSource([show]), show.id);

    expect(await screen.findByRole('heading', { name: 'Unscanned Show' })).toBeInTheDocument();
    expect(screen.getByText('No seasons are available for this show.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^(Play|Resume)/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Watchlist|Favorite|Mark watched/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /More actions/ })).not.toBeInTheDocument();
  });

  it('keeps failure recovery bounded to the detail request', async () => {
    const item = media({ id: 'recover', title: 'Recovered detail' });
    const source = new DetailSource([item]);
    // Exhaust the shared two-retry transient-read budget so the recoverable
    // detail error is exercised before the explicit user retry succeeds.
    source.failures.set(item.id, 3);
    renderDetail(source, item.id);

    expect(await screen.findByRole('alert')).toHaveTextContent('Details unavailable');
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('heading', { name: 'Recovered detail' })).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText('The server connection was interrupted.')).not.toBeInTheDocument());
  });

  it('keeps server administration out of the ordinary media surface', async () => {
    const item = media({
      id: 'maintainable',
      title: 'Maintainable movie',
      actions: ['metadata.refresh', 'media.analyze', 'media.optimize', 'download'],
    });
    renderDetail(new DetailSource([item]), item.id);

    expect(await screen.findByRole('heading', { name: 'Maintainable movie' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'More actions for Maintainable movie' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Download' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refresh metadata' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Analyze media' })).not.toBeInTheDocument();
  });

});
