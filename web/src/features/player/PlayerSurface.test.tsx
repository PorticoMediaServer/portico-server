import { ApiError, type PlaybackResponse, type PlaybackSessionQueueResponse } from '@porticomediaserver/client-core';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { ReactNode } from 'react';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import type { PorticoDataSource, Viewer } from '../../data/models';
import { isExplicitMissingHLSManifest, playableSubtitleStreams, PlaybackSessionProvider, PlayerDock, shouldUseNativeHLS, usePlaybackSession, WatchPage } from './PlayerSurface';
import { WebDisplayPreferencesProvider } from '../../preferences/WebDisplayPreferencesProvider';

const hlsFixtures = vi.hoisted(() => ({ instances: [] as Array<{ destroy: ReturnType<typeof vi.fn>; loadSource: ReturnType<typeof vi.fn> }> }));

vi.mock('hls.js', () => {
  class FixtureHls {
    static isSupported = () => true;
    static Events = { SUBTITLE_TRACKS_UPDATED: 'subtitleTracksUpdated', MANIFEST_PARSED: 'manifestParsed', FRAG_LOADED: 'fragLoaded', ERROR: 'error' };
    static ErrorTypes = { NETWORK_ERROR: 'networkError', MEDIA_ERROR: 'mediaError' };
    subtitleDisplay = false;
    subtitleTrack = -1;
    subtitleTracks: Array<{ lang?: string; name?: string }> = [];
    destroy = vi.fn();
    loadSource = vi.fn();
    attachMedia = vi.fn((media: HTMLMediaElement) => queueMicrotask(() => media.dispatchEvent(new Event('loadedmetadata'))));
    on = vi.fn();
    startLoad = vi.fn();
    recoverMediaError = vi.fn();
    constructor() { hlsFixtures.instances.push(this); }
  }
  return { default: FixtureHls };
});

const viewer: Viewer = {
  authenticated: true,
  setupRequired: false,
  serverName: 'Portico Test',
  user: { id: 'viewer', displayName: 'Viewer', email: 'viewer@example.test', role: 'owner' },
};

function coreMedia(id: string, title: string): PlaybackResponse['media'] {
  return {
    id,
    type: 'episode',
    title,
    sortTitle: title,
    metadataEtag: `test-media-${id}-revision-1`,
    metadataRevision: 1,
    addedAt: '2026-01-01T00:00:00Z',
    durationSeconds: 120,
    genres: [],
    tags: [],
    labels: [],
    images: { poster: '/poster.jpg', backdrop: '/backdrop.jpg', thumb: '/thumb.jpg' },
    state: { watchlisted: false, favorite: false, watched: false, progressSeconds: 12, rating: 0 },
    actions: ['play' as const],
  };
}

function playback(overrides: Partial<PlaybackResponse> = {}): PlaybackResponse {
  const value: PlaybackResponse = {
    sessionId: 'session-1',
    nextEventSequence: 1,
    mediaGrant: { token: 'grant', expiresAt: '2099-01-01T00:00:00Z' },
    continuationCredential: { token: 'continuation', expiresAt: '2099-01-01T00:00:00Z', generation: 1, origin: 'http://localhost:32500' },
    media: coreMedia('episode-1', 'The Castle'),
    sourceUrl: '/api/media/episode-1/stream.mp4',
    resources: [
      { id: 'direct-original-en', sourceUrl: '/api/playback-resources/direct-original-en', streamFormat: 'direct', qualityId: 'original', audioStreamId: 'audio-1', subtitleMode: 'off', default: true },
      { id: 'hls-720p-en', sourceUrl: '/api/playback-resources/hls-720p-en?signature=server-owned', streamFormat: 'hls', qualityId: '720p', audioStreamId: 'audio-1', subtitleMode: 'off' },
      { id: 'hls-720p-fr', sourceUrl: '/api/playback-resources/hls-720p-fr?signature=server-owned', streamFormat: 'hls', qualityId: '720p', audioStreamId: 'audio-2', subtitleMode: 'off' },
    ],
    directPlay: true,
    streamFormat: 'direct',
    decision: { mode: 'direct_play', reason: 'Browser compatible', reasonCodes: ['exact_tuple'], requiresTranscode: false, isProxied: true, isServerCached: false },
    policy: { networkClass: 'local', qualityProfile: 'original', directPlayPolicy: 'prefer', directStreamPolicy: 'allow', transcodePolicy: 'allow', allowHdr: true, deliveryProfile: 'video-original', serverClamps: [] },
    qualities: [{ id: 'original', label: 'Original', description: 'Original quality' }],
    audioStreams: [{ id: 'audio-1', kind: 'audio', codec: 'aac', language: 'en', displayTitle: 'English' }],
    selectedAudioStreamId: 'audio-1',
    subtitleStreams: [],
    chapters: [],
    queue: [coreMedia('episode-2', 'Palindrome')],
    repeatMode: 'off',
    queueRevision: 7,
    playbackRevision: 1,
    timeline: { type: 'vod', durationSeconds: 120, canPause: true, canSeek: true },
    resumePositionSeconds: 12,
    generation: 1,
  };
  return { ...value, ...overrides };
}

function queue(overrides: Partial<PlaybackSessionQueueResponse> = {}): PlaybackSessionQueueResponse {
  const value: PlaybackSessionQueueResponse = {
    sessionId: 'session-1',
    current: coreMedia('episode-1', 'The Castle'),
    items: [coreMedia('episode-2', 'Palindrome'), coreMedia('episode-3', 'The Myth of Sisyphus')],
    history: [coreMedia('episode-0', 'Waiting for Dutch')],
    total: 2,
    canMutate: true,
    repeatMode: 'off',
    revision: 7,
  };
  return { ...value, ...overrides };
}

function renderPlayer(source: FixturePorticoDataSource, mediaId = 'episode-1', extra?: ReactNode) {
  return render(<DataProvider source={source} initialViewer={viewer}>
    <WebDisplayPreferencesProvider>
      <MemoryRouter initialEntries={[`/watch/${mediaId}`]}>
        <PlaybackSessionProvider>
          <Routes><Route path="/watch/:id" element={<WatchPage />} /><Route path="*" element={null} /></Routes>
          <PlayerDock />
          {extra}
        </PlaybackSessionProvider>
      </MemoryRouter>
    </WebDisplayPreferencesProvider>
  </DataProvider>);
}

function QueueMutationHarness() {
  const playback = usePlaybackSession();
  return <><button type="button" onClick={() => void playback.appendQueue(['episode-4'])}>Append fixture</button><button type="button" onClick={() => void playback.playNext(['episode-5'])}>Play fixture next</button></>;
}

function PlaybackRecoveryHarness() {
  const playback = usePlaybackSession();
  return <>
    <button type="button" onClick={() => playback.fail('route')}>Fail route</button>
    <button type="button" onClick={() => void playback.start('episode-1', { startSeconds: 0 })}>Play from beginning</button>
  </>;
}

beforeEach(() => {
  hlsFixtures.instances.length = 0;
  localStorage.clear();
  vi.spyOn(HTMLMediaElement.prototype, 'canPlayType').mockReturnValue('probably');
  vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(function load(this: HTMLMediaElement) {
    queueMicrotask(() => this.dispatchEvent(new Event('loadedmetadata')));
  });
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(function play(this: HTMLMediaElement) {
    Object.defineProperty(this, 'paused', { configurable: true, value: false });
    this.dispatchEvent(new Event('play'));
    this.dispatchEvent(new Event('playing'));
    return Promise.resolve();
  });
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(function pause(this: HTMLMediaElement) {
    Object.defineProperty(this, 'paused', { configurable: true, value: true });
    this.dispatchEvent(new Event('pause'));
  });
  Object.defineProperty(HTMLMediaElement.prototype, 'duration', { configurable: true, get: () => 120 });
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.restoreAllMocks();
});

describe('production playback surface', () => {
  it('rebuilds a managed HLS adapter after manual route retry without blindly loading the media element', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Mozilla/5.0 Chrome/147.0.0.0 Safari/537.36');
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({
      streamFormat: 'hls',
      sourceUrl: '/api/playback-resources/hls-original-en/index.m3u8',
      resources: [{ id: 'hls-original-en', sourceUrl: '/api/playback-resources/hls-original-en/index.m3u8', streamFormat: 'hls', qualityId: 'original', audioStreamId: 'audio-1', subtitleMode: 'off', default: true }],
    }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    const renew = vi.spyOn(source as PorticoDataSource, 'renewPlaybackMediaGrant').mockResolvedValue({ token: 'renewed-grant', expiresAt: '2099-01-01T00:00:00Z' });

    renderPlayer(source, 'episode-1', <PlaybackRecoveryHarness />);
    await screen.findByLabelText('Now playing The Castle');
    await waitFor(() => expect(hlsFixtures.instances).toHaveLength(1));
    const loadCalls = vi.mocked(HTMLMediaElement.prototype.load).mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Fail route' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Try again' }));

    await waitFor(() => expect(renew).toHaveBeenCalledWith('session-1', expect.any(AbortSignal)));
    await waitFor(() => expect(hlsFixtures.instances).toHaveLength(2));
    expect(hlsFixtures.instances[0].destroy).toHaveBeenCalledOnce();
    expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(loadCalls);
  });

  it('seeks and resumes an active item when Play from beginning targets the same media', async () => {
    const source = new FixturePorticoDataSource();
    const start = vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source, 'episode-1', <PlaybackRecoveryHarness />);
    const surface = await screen.findByLabelText('Now playing The Castle');
    const media = surface.querySelector('video') as HTMLVideoElement;
    media.currentTime = 71;
    vi.mocked(HTMLMediaElement.prototype.play).mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'Play from beginning' }));

    await waitFor(() => expect(media.currentTime).toBe(0));
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce();
    expect(start).toHaveBeenCalledOnce();
  });

  it('only classifies an explicit pre-play manifest 404 or 410 as a missing source', () => {
    expect(isExplicitMissingHLSManifest(404, 'manifestLoadError', false)).toBe(true);
    expect(isExplicitMissingHLSManifest(410, 'manifestLoadError', false)).toBe(true);
    expect(isExplicitMissingHLSManifest(404, 'fragLoadError', false)).toBe(false);
    expect(isExplicitMissingHLSManifest(503, 'manifestLoadError', false)).toBe(false);
    expect(isExplicitMissingHLSManifest(404, 'manifestLoadError', true)).toBe(false);
  });

  it('uses native HLS only in Safari even when Chromium reports tentative support', () => {
    expect(shouldUseNativeHLS('maybe', 'Mozilla/5.0 Safari/605.1.15')).toBe(true);
    expect(shouldUseNativeHLS('probably', 'Mozilla/5.0 Chrome/141.0.0.0 Safari/537.36')).toBe(false);
    expect(shouldUseNativeHLS('maybe', 'Mozilla/5.0 Edg/141.0.0.0 Safari/537.36')).toBe(false);
    expect(shouldUseNativeHLS('', 'Mozilla/5.0 Safari/605.1.15')).toBe(false);
  });

  it('removes the API off sentinel from playable subtitle choices', () => {
    const sentinel = { id: 'sub_none', kind: 'subtitle', codec: '', language: '', displayTitle: 'None' } as PlaybackResponse['subtitleStreams'][number];
    const english = { id: 'subtitle-en', kind: 'subtitle', codec: 'srt', language: 'en', displayTitle: 'English' } as PlaybackResponse['subtitleStreams'][number];
    expect(playableSubtitleStreams([sentinel, english])).toEqual([english]);
  });

  it('keeps the complete control contract across the full and docked layouts', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source);

    const surface = await screen.findByLabelText('Now playing The Castle');
    expect(surface).toHaveClass('player-full');
    expect(surface.querySelector('.player-copy-art')).toHaveAttribute('src', 'http://localhost:3000/poster.jpg');
    const controlLabels = ['Previous item', 'Rewind 10 seconds', 'Pause', 'Forward 30 seconds', 'Next item', 'Volume', 'Subtitles', 'Playback settings', 'Queue', 'Fullscreen', 'Close player'];
    for (const label of controlLabels) {
      await waitFor(() => expect(surface.querySelector(`[aria-label="${label}"]`)).toBeInTheDocument());
    }

    fireEvent.click(screen.getByLabelText('Collapse player'));
    await waitFor(() => expect(surface).toHaveClass('player-mini'));
    for (const label of controlLabels) {
      expect(surface.querySelector(`[aria-label="${label}"]`)).toBeInTheDocument();
    }
    expect(screen.getByLabelText('Playback position')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Expand player' }));
    await waitFor(() => expect(surface).toHaveClass('player-full'));
  });

  it('keeps one media element alive while collapsing and expanding the player', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source);
    const fullSurface = await screen.findByLabelText('Now playing The Castle');
    const media = fullSurface.querySelector('video');
    expect(media).toBeInTheDocument();
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());
    const playCalls = vi.mocked(HTMLMediaElement.prototype.play).mock.calls.length;

    fireEvent.click(screen.getByRole('button', { name: 'Collapse player' }));
    const dockedSurface = await screen.findByLabelText('Now playing The Castle');
    expect(dockedSurface).toHaveClass('player-mini');
    expect(dockedSurface.querySelector('video')).toBe(media);
    fireEvent.click(within(dockedSurface).getByRole('button', { name: 'Expand player' }));
    const expandedSurface = await screen.findByLabelText('Now playing The Castle');
    expect(expandedSurface).toHaveClass('player-full');
    expect(expandedSurface.querySelector('video')).toBe(media);
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(playCalls);
  });

  it('starts a canonical session and seeks with the real media element', async () => {
    const source = new FixturePorticoDataSource();
    const start = vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    const touch = vi.spyOn(source as PorticoDataSource, 'touchPlayback').mockResolvedValue({ accepted: true, duplicate: false, stale: false, generation: 1, highestEventSequence: 1, sessionState: 'playing' });

    renderPlayer(source);

    expect(await screen.findByLabelText('Now playing The Castle')).toBeInTheDocument();
    expect(start).toHaveBeenCalledWith('episode-1', expect.objectContaining({ intent: expect.objectContaining({ networkClass: 'local', qualityProfile: 'original' }) }), expect.any(AbortSignal));
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('button', { name: 'Forward 30 seconds' }));
    await waitFor(() => expect(touch).toHaveBeenCalledWith('session-1', expect.objectContaining({ positionSeconds: 42 }), expect.any(AbortSignal), false));
  });

  it('preserves the canonical resume marker when the queue refreshes before media metadata', async () => {
    const source = new FixturePorticoDataSource();
    const resumedPlayback = playback({ resumePositionSeconds: 47 });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(resumedPlayback);
    let resolveQueue!: (value: PlaybackSessionQueueResponse) => void;
    const queueResponse = new Promise<PlaybackSessionQueueResponse>((resolve) => { resolveQueue = resolve; });
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockReturnValue(queueResponse);
    vi.mocked(HTMLMediaElement.prototype.load).mockImplementation(() => undefined);

    renderPlayer(source);

    const surface = await screen.findByLabelText('Now playing The Castle');
    const media = surface.querySelector('video') as HTMLVideoElement;
    await waitFor(() => expect(media.src).toContain('/api/playback-resources/direct-original-en'));
    expect(media.currentTime).toBe(0);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();

    // A queue-only response must not replace the source generation while this
    // same session is still waiting for loadedmetadata.
    resolveQueue(queue());
    await waitFor(() => expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(1));
    expect(media.currentTime).toBe(0);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();

    fireEvent.loadedMetadata(media);
    expect(media.currentTime).toBe(47);
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1));
  });

  it('keeps expanded routing separate from browser fullscreen state', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => fullscreenElement });
    Object.defineProperty(document, 'exitFullscreen', { configurable: true, value: vi.fn(async () => {
      fullscreenElement = null;
      document.dispatchEvent(new Event('fullscreenchange'));
    }) });

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    Object.defineProperty(surface, 'requestFullscreen', { configurable: true, value: vi.fn(async () => {
      fullscreenElement = surface;
      document.dispatchEvent(new Event('fullscreenchange'));
    }) });

    fireEvent.click(screen.getByRole('button', { name: 'Fullscreen' }));
    await screen.findByRole('button', { name: 'Exit fullscreen' });
    expect(surface).toHaveClass('player-full');
    fireEvent.click(screen.getByRole('button', { name: 'Exit fullscreen' }));
    await screen.findByRole('button', { name: 'Fullscreen' });
    expect(surface).toHaveClass('player-full');

    fireEvent.click(screen.getByRole('button', { name: 'Collapse player' }));
    await waitFor(() => expect(surface).toHaveClass('player-mini'));
    fireEvent.click(screen.getByRole('button', { name: 'Fullscreen' }));
    await waitFor(() => expect(surface).toHaveClass('player-full'));
    fireEvent.click(screen.getByRole('button', { name: 'Exit fullscreen' }));
    await waitFor(() => expect(surface).toHaveClass('player-mini'));
  });

  it('mutates the real session queue and tears down playback on close', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    vi.spyOn(source as PorticoDataSource, 'touchPlayback').mockResolvedValue({ accepted: true, duplicate: false, stale: false, generation: 1, highestEventSequence: 1, sessionState: 'paused' });
    const mutate = vi.spyOn(source as PorticoDataSource, 'mutatePlaybackSessionQueue').mockResolvedValue(queue());
    const stop = vi.spyOn(source as PorticoDataSource, 'stopPlayback').mockResolvedValue(undefined);

    renderPlayer(source);
    await screen.findByLabelText('Now playing The Castle');
    const queueTrigger = screen.getByLabelText('Queue');
    expect(queueTrigger).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(queueTrigger);
    const queueDialog = await screen.findByRole('dialog', { name: 'Queue' });
    expect(queueTrigger).toHaveAttribute('aria-expanded', 'true');
    expect(within(queueDialog).getByText('Now playing')).toBeInTheDocument();
    expect(within(queueDialog).getByText('The Castle')).toBeInTheDocument();
    fireEvent.click(within(queueDialog).getByRole('button', { name: 'Move Palindrome later' }));
    await waitFor(() => expect(mutate).toHaveBeenCalledWith('session-1', { action: 'reorder', expectedRevision: 7, fromIndex: 0, toIndex: 1 }, expect.any(AbortSignal)));
    fireEvent.click(within(queueDialog).getByRole('button', { name: 'Close queue' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Queue' })).not.toBeInTheDocument());
    expect(queueTrigger).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(screen.getByRole('button', { name: 'Close player' }));
    await waitFor(() => expect(stop).toHaveBeenCalledWith('session-1', expect.any(AbortSignal), true));
    expect(screen.queryByLabelText('Now playing The Castle')).not.toBeInTheDocument();
  });

  it('appends and inserts next through revision-safe queue mutations', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    const mutate = vi.spyOn(source as PorticoDataSource, 'mutatePlaybackSessionQueue')
      .mockResolvedValueOnce(queue({ revision: 8 }))
      .mockResolvedValueOnce(queue({ revision: 9 }));

    renderPlayer(source, 'episode-1', <QueueMutationHarness />);
    await screen.findByLabelText('Now playing The Castle');
    fireEvent.click(screen.getByRole('button', { name: 'Append fixture' }));
    await waitFor(() => expect(mutate).toHaveBeenCalledWith('session-1', { action: 'append', expectedRevision: 7, mediaIds: ['episode-4'] }, expect.any(AbortSignal)));
    fireEvent.click(screen.getByRole('button', { name: 'Play fixture next' }));
    await waitFor(() => expect(mutate).toHaveBeenLastCalledWith('session-1', { action: 'play_next', expectedRevision: 8, mediaIds: ['episode-5'] }, expect.any(AbortSignal)));
  });

  it('keeps repeat mode authoritative and recovers revision conflicts without replaying the mutation', async () => {
    const source = new FixturePorticoDataSource();
    const initialQueue = queue();
    const refreshedQueue = queue({ repeatMode: 'one', revision: 9 });
    const repeatedQueue = queue({ repeatMode: 'off', revision: 10 });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    const readQueue = vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue')
      .mockResolvedValueOnce(initialQueue)
      .mockResolvedValueOnce(refreshedQueue);
    const mutate = vi.spyOn(source as PorticoDataSource, 'mutatePlaybackSessionQueue')
      .mockRejectedValueOnce(new ApiError(409, 'queue_revision_conflict', 'Queue changed.'))
      .mockResolvedValueOnce(repeatedQueue);

    renderPlayer(source);
    await screen.findByLabelText('Now playing The Castle');
    fireEvent.click(screen.getByLabelText('Queue'));
    const queueDialog = await screen.findByRole('dialog', { name: 'Queue' });

    fireEvent.click(within(queueDialog).getByRole('button', { name: 'Repeat off' }));
    await waitFor(() => expect(within(queueDialog).getByRole('button', { name: 'Repeat one' })).toBeInTheDocument());
    expect(within(queueDialog).getByRole('alert')).toHaveTextContent('Queue changed on another device. Review the latest queue and try again.');
    expect(readQueue).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate).toHaveBeenLastCalledWith('session-1', { action: 'set_repeat', expectedRevision: 7, repeatMode: 'all' }, expect.any(AbortSignal));

    fireEvent.click(within(queueDialog).getByRole('button', { name: 'Repeat one' }));
    await waitFor(() => expect(within(queueDialog).getByRole('button', { name: 'Repeat off' })).toBeInTheDocument());
    expect(mutate).toHaveBeenCalledTimes(2);
    expect(mutate).toHaveBeenLastCalledWith('session-1', { action: 'set_repeat', expectedRevision: 9, repeatMode: 'off' }, expect.any(AbortSignal));
  });

  it('prepares Up Next, allows cancellation, and hands off the prepared session on demand', async () => {
    const source = new FixturePorticoDataSource();
    const current = playback();
    const preparedPlayback = playback({ sessionId: 'session-prepared', media: coreMedia('episode-2', 'Palindrome'), queue: [coreMedia('episode-3', 'The Myth of Sisyphus')] });
    const handedOff = playback({ sessionId: 'session-2', media: coreMedia('episode-2', 'Palindrome'), queue: [coreMedia('episode-3', 'The Myth of Sisyphus')] });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    const prepare = vi.spyOn(source as PorticoDataSource, 'prepareNextPlayback').mockResolvedValue({
      preparedSessionId: 'prepared-2',
      playback: preparedPlayback,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      preloadPolicy: 'metadata',
      handoffMode: 'replace',
      queue: preparedPlayback.queue,
      queueRevision: preparedPlayback.queueRevision,
      playbackRevision: preparedPlayback.playbackRevision,
    });
    const handoff = vi.spyOn(source as PorticoDataSource, 'handoffPlayback').mockResolvedValue(handedOff);

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());
    fireEvent.ended(surface.querySelector('video') as HTMLVideoElement);

    const upNext = await screen.findByRole('region', { name: 'Up next' });
    expect(within(upNext).getByText('Palindrome')).toBeInTheDocument();
    expect(within(upNext).getByText(/Playing in \d+ seconds?/)).toBeInTheDocument();
    expect(prepare).toHaveBeenCalledWith('session-1', expect.any(AbortSignal), expect.objectContaining({ intent: expect.any(Object) }));

    fireEvent.click(within(upNext).getByRole('button', { name: 'Cancel' }));
    expect(within(upNext).getByText('Automatic playback cancelled')).toBeInTheDocument();
    expect(handoff).not.toHaveBeenCalled();

    fireEvent.click(within(upNext).getByRole('button', { name: 'Play next' }));
    await waitFor(() => expect(handoff).toHaveBeenCalledWith('session-1', expect.objectContaining({
      preparedSessionId: 'prepared-2',
      intent: expect.any(Object),
    }), expect.any(AbortSignal)));
    expect(await screen.findByLabelText('Now playing Palindrome')).toBeInTheDocument();
  });

  it('shows queue exhaustion and can replay with a fresh canonical handoff', async () => {
    const source = new FixturePorticoDataSource();
    const current = playback({ queue: [] });
    const emptyQueue = { ...queue(), items: [], total: 0 };
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(current);
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(emptyQueue);
    const touch = vi.spyOn(source as PorticoDataSource, 'touchPlayback').mockResolvedValue({ accepted: true, duplicate: false, stale: false, generation: 1, highestEventSequence: 2, sessionState: 'stopped' });
    const handoff = vi.spyOn(source as PorticoDataSource, 'handoffPlayback').mockResolvedValue(playback({ sessionId: 'session-replay', queue: [] }));

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());
    fireEvent.ended(surface.querySelector('video') as HTMLVideoElement);

    const complete = await screen.findByRole('region', { name: 'Playback is complete.' });
    expect(within(complete).getByText("You're all caught up")).toBeInTheDocument();
    await waitFor(() => expect(touch).toHaveBeenCalledWith('session-1', expect.objectContaining({ completed: true, positionSeconds: 120 }), expect.any(AbortSignal), false));
    fireEvent.click(within(complete).getByRole('button', { name: 'Replay' }));
    await waitFor(() => expect(handoff).toHaveBeenCalledWith('session-1', expect.objectContaining({ mediaId: 'episode-1', progressSeconds: 0, queueMediaIds: [] }), expect.any(AbortSignal)));
  });

  it('closes an established playback session with a direct interruption message after an unrecoverable media failure', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    const media = surface.querySelector('video') as HTMLVideoElement;
    await waitFor(() => expect(HTMLMediaElement.prototype.play).toHaveBeenCalled());
    fireEvent.waiting(media);
    expect(await screen.findByText('Buffering')).toBeInTheDocument();

    vi.mocked(HTMLMediaElement.prototype.play).mockImplementation(function stalledPlay(this: HTMLMediaElement) {
      Object.defineProperty(this, 'paused', { configurable: true, value: false });
      this.dispatchEvent(new Event('play'));
      return Promise.resolve();
    });

    Object.defineProperty(media, 'error', { configurable: true, value: { code: 2 } });
    vi.useFakeTimers();
    // The first network failure is reserved for a silent route/grant/source
    // rebase before the normal media-element retry budget is exhausted.
    fireEvent.error(media);
    await vi.advanceTimersByTimeAsync(0);
    for (const delay of [0, 400, 1_200]) {
      fireEvent.error(media);
      await vi.advanceTimersByTimeAsync(delay);
    }
    fireEvent.error(media);
    await vi.advanceTimersByTimeAsync(0);
    vi.useRealTimers();
    expect(await screen.findByText('Playback stopped')).toBeInTheDocument();
    expect(screen.getByText(/several reconnect attempts/i)).toBeInTheDocument();
    expect(screen.queryByLabelText('Now playing The Castle')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss playback message' }));
    expect(screen.queryByText('Playback stopped')).not.toBeInTheDocument();
  });

  it('uses a live state instead of exposing VOD seeking controls', async () => {
    const source = new FixturePorticoDataSource();
    const liveMedia = { ...coreMedia('channel-1', 'News 7'), type: 'live_channel' } as PlaybackResponse['media'];
    const live = playback({ media: liveMedia, isLive: true, queue: [], timeline: { type: 'live', canPause: false, canSeek: false } });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(live);
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue({ ...queue(), current: liveMedia, items: [], history: [], total: 0 });

    renderPlayer(source, 'channel-1');
    const surface = await screen.findByLabelText('Now playing News 7');
    expect(surface).toHaveClass('mode-live');
    const channelLogo = surface.querySelector('.player-copy-logo');
    expect(channelLogo).toHaveAttribute('src', 'http://localhost:3000/thumb.jpg');
    fireEvent.error(channelLogo as HTMLImageElement);
    await waitFor(() => expect(surface.querySelector('.player-copy-art-fallback')).toBeInTheDocument());
    expect(within(surface).getByText('Live')).toBeInTheDocument();
    expect(within(surface).queryByLabelText('Playback position')).not.toBeInTheDocument();
    expect(within(surface).getByRole('button', { name: 'Rewind 10 seconds' })).toBeDisabled();
    expect(within(surface).getByRole('button', { name: 'Forward 30 seconds' })).toBeDisabled();
    expect(within(surface).getByRole('button', { name: 'Next item' })).toBeDisabled();
  });

  it('resolves playback artwork against the selected server and scopes API images to the media grant', async () => {
    const source = new FixturePorticoDataSource();
    const media = {
      ...coreMedia('episode-1', 'The Castle'),
      images: {
        poster: '/api/artwork/episode-1/poster',
        backdrop: '/api/artwork/episode-1/backdrop',
        thumb: '/api/artwork/episode-1/thumb',
      },
    } as PlaybackResponse['media'];
    vi.spyOn(source as PorticoDataSource, 'playbackResourceUrl').mockImplementation((path) => `https://server.example${path}`);
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ media }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue({ current: media }));

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    expect(surface.querySelector('.player-copy-art')).toHaveAttribute(
      'src',
      'https://server.example/api/artwork/episode-1/poster',
    );
    expect(surface.querySelector('video')).toHaveAttribute(
      'poster',
      'https://server.example/api/artwork/episode-1/backdrop',
    );
  });

  it('presents subtitle, lyric, and audio capabilities in their contract locations', async () => {
    const videoSource = new FixturePorticoDataSource();
    const subtitleStream = { id: 'subtitle-en', kind: 'subtitle', codec: 'srt', language: 'en', displayTitle: 'English' } as PlaybackResponse['subtitleStreams'][number];
    vi.spyOn(videoSource as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ subtitleStreams: [subtitleStream] }));
    vi.spyOn(videoSource as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    const videoView = renderPlayer(videoSource);
    await screen.findByLabelText('Now playing The Castle');
    fireEvent.click(screen.getByRole('button', { name: 'Subtitles' }));
    const subtitleDialog = await screen.findByRole('dialog', { name: 'Subtitles' });
    const subtitleChoices = within(subtitleDialog).getByRole('radiogroup', { name: 'Subtitle track' });
    expect(within(subtitleChoices).getByRole('radio', { name: 'Off' })).toHaveAttribute('aria-checked', 'true');
    expect(within(subtitleChoices).getByRole('radio', { name: /English/ })).toHaveAttribute('aria-checked', 'false');
    videoView.unmount();

    const audioSource = new FixturePorticoDataSource();
    const musicMedia = {
      ...coreMedia('track-lyrics', 'Night Drive'),
      type: 'track',
      lyrics: [{ id: 'lyrics-1', source: 'embedded', format: 'lrc', synced: true, text: '[00:00.00]Under city lights\n[00:12.00]The night is ours', createdAt: '2026-01-01T00:00:00Z' }],
    } as PlaybackResponse['media'];
    vi.spyOn(audioSource as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ media: musicMedia, queue: [] }));
    vi.spyOn(audioSource as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue({ ...queue(), current: musicMedia, items: [], history: [], total: 0 });

    renderPlayer(audioSource, 'track-lyrics');
    const audioSurface = await screen.findByLabelText('Now playing Night Drive');
    const audioElement = audioSurface.querySelector('video') as HTMLVideoElement;
    audioElement.currentTime = 13;
    fireEvent.timeUpdate(audioElement);

    fireEvent.click(screen.getByRole('button', { name: 'Lyrics' }));
    const lyricDialog = await screen.findByRole('dialog', { name: 'Lyrics' });
    const synchronizedLyrics = within(lyricDialog).getByRole('region', { name: 'Synchronized lyrics' });
    expect(synchronizedLyrics.querySelector('[aria-current="true"]')).toHaveTextContent('The night is ours');
    fireEvent.wheel(synchronizedLyrics);
    expect(within(lyricDialog).getByRole('button', { name: 'Follow current lyric' })).toBeInTheDocument();
    fireEvent.click(within(lyricDialog).getByRole('button', { name: 'Follow current lyric' }));
    expect(within(lyricDialog).queryByRole('button', { name: 'Follow current lyric' })).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Lyrics' })).not.toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Playback settings' }));
    const settingsDialog = await screen.findByRole('dialog', { name: 'Playback settings' });
    fireEvent.click(within(settingsDialog).getByRole('combobox', { name: 'Audio' }));
    expect(within(settingsDialog).getByRole('option', { name: /English/i })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('button', { name: 'Subtitles' })).not.toBeInTheDocument();
  });

  it('forces HLS when quality or alternate audio requires a server-selected stream', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue('Mozilla/5.0 Safari/605.1.15');
    let selectedPlayback = playback({
      qualities: [
        { id: 'original', label: 'Original', description: 'Original quality' },
        { id: '720p', label: '720p', description: '4 Mbps', requiresTranscode: true },
      ],
      audioStreams: [
        { id: 'audio-1', kind: 'audio', codec: 'aac', language: 'en', displayTitle: 'English' },
        { id: 'audio-2', kind: 'audio', codec: 'aac', language: 'fr', displayTitle: 'French' },
      ],
    });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(selectedPlayback);
    const renegotiate = vi.spyOn(source as PorticoDataSource, 'renegotiatePlayback').mockImplementation(async (_sessionId, request) => {
      selectedPlayback = {
        ...selectedPlayback,
        playbackRevision: selectedPlayback.playbackRevision + 1,
        selectedQualityId: request.qualityId ?? selectedPlayback.selectedQualityId,
        selectedAudioStreamId: request.audioStreamId ?? selectedPlayback.selectedAudioStreamId,
        mediaGrant: { token: `grant-${selectedPlayback.playbackRevision + 1}`, expiresAt: '2099-01-01T00:00:00Z' },
      };
      return selectedPlayback;
    });
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    fireEvent.click(screen.getByRole('button', { name: 'Playback settings' }));
    const settings = await screen.findByRole('dialog', { name: 'Playback settings' });
    fireEvent.click(within(settings).getByRole('combobox', { name: 'Quality' }));
    fireEvent.click(within(settings).getByRole('option', { name: /720p/i }));
    await waitFor(() => expect((surface.querySelector('video') as HTMLVideoElement).src).toBe('http://localhost:3000/api/playback-resources/hls-720p-en?signature=server-owned'));
    fireEvent.click(within(settings).getByRole('combobox', { name: 'Audio' }));
    fireEvent.click(within(settings).getByRole('option', { name: /French/i }));
    await waitFor(() => expect((surface.querySelector('video') as HTMLVideoElement).src).toBe('http://localhost:3000/api/playback-resources/hls-720p-fr?signature=server-owned'));
    expect(renegotiate).toHaveBeenNthCalledWith(1, 'session-1', expect.objectContaining({ qualityId: '720p', expectedRevision: 1 }), expect.any(AbortSignal));
    expect(renegotiate).toHaveBeenNthCalledWith(2, 'session-1', expect.objectContaining({ audioStreamId: 'audio-2', expectedRevision: 2 }), expect.any(AbortSignal));
  });

  it('supports keyboard traversal and restores focus after choosing a player setting', async () => {
    const source = new FixturePorticoDataSource();
    const initialPlayback = playback({
      qualities: [
        { id: 'original', label: 'Original', description: 'Original quality' },
        { id: '720p', label: '720p', description: '4 Mbps', requiresTranscode: true },
        { id: '480p', label: '480p', description: '1.5 Mbps', requiresTranscode: true },
      ],
    });
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(initialPlayback);
    vi.spyOn(source as PorticoDataSource, 'renegotiatePlayback').mockImplementation(async (_sessionId, request) => ({
      ...initialPlayback,
      playbackRevision: initialPlayback.playbackRevision + 1,
      selectedQualityId: request.qualityId,
      mediaGrant: { token: 'grant-quality', expiresAt: '2099-01-01T00:00:00Z' },
    }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    renderPlayer(source);
    await screen.findByLabelText('Now playing The Castle');
    fireEvent.click(screen.getByRole('button', { name: 'Playback settings' }));
    const settings = await screen.findByRole('dialog', { name: 'Playback settings' });
    const quality = within(settings).getByRole('combobox', { name: 'Quality' });

    quality.focus();
    fireEvent.keyDown(quality, { key: 'ArrowDown' });
    const original = within(settings).getByRole('option', { name: /Original/i });
    await waitFor(() => expect(original).toHaveFocus());
    fireEvent.keyDown(original, { key: 'ArrowDown' });
    const medium = within(settings).getByRole('option', { name: /720p/i });
    expect(medium).toHaveFocus();
    fireEvent.keyDown(medium, { key: 'End' });
    const low = within(settings).getByRole('option', { name: /480p/i });
    expect(low).toHaveFocus();
    fireEvent.click(low);
    await waitFor(() => expect(quality).toHaveFocus());
    expect(quality).toHaveTextContent('480p');

    fireEvent.keyDown(quality, { key: 'ArrowUp' });
    await waitFor(() => expect(medium).toHaveFocus());
    fireEvent.keyDown(medium, { key: 'Escape' });
    expect(quality).toHaveFocus();
    expect(quality).toHaveAttribute('aria-expanded', 'false');
  });

  it('applies playback speed, persists volume, and exposes contract chapters and skip prompts', async () => {
    const source = new FixturePorticoDataSource();
    const media = { ...coreMedia('episode-1', 'The Castle'), segments: [{ id: 'intro-1', type: 'intro', startSeconds: 0, endSeconds: 18, automaticSafe: false }] } as PlaybackResponse['media'];
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({
      media,
      chapters: [
        { id: 'chapter-1', title: 'Opening', startSeconds: 0, endSeconds: 30, thumbUrl: '/api/artwork/episode-1/chapter-1.jpg' },
        { id: 'chapter-2', title: 'The plan', startSeconds: 30, endSeconds: 65 },
      ],
    }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue({ current: media }));

    renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    const mediaElement = surface.querySelector('video') as HTMLVideoElement;
    expect(screen.getByRole('button', { name: 'Skip Intro' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss skip intro prompt' }));
    expect(screen.queryByRole('button', { name: 'Skip Intro' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Chapters' }));
    const chapters = await screen.findByRole('dialog', { name: 'Chapters' });
    expect(within(chapters).getByText('Opening')).toBeInTheDocument();
    expect(chapters.querySelector('img')?.getAttribute('src')).toContain('/api/artwork/episode-1/chapter-1.jpg');
    expect(chapters.querySelector('img')?.getAttribute('src')).not.toContain('grant=');
    fireEvent.click(within(chapters).getByText('The plan'));
    expect(mediaElement.currentTime).toBe(30);
    fireEvent.keyDown(window, { key: 'Escape' });

    fireEvent.click(screen.getByRole('button', { name: 'Playback settings' }));
    const settings = await screen.findByRole('dialog', { name: 'Playback settings' });
    fireEvent.click(within(settings).getByRole('combobox', { name: 'Playback speed' }));
    fireEvent.click(within(settings).getByRole('option', { name: '1.5×' }));
    expect(mediaElement.playbackRate).toBe(1.5);
    fireEvent.keyDown(window, { key: 'Escape' });
    fireEvent.click(screen.getByRole('button', { name: 'Volume' }));
    const volume = await screen.findByRole('dialog', { name: 'Volume' });
    fireEvent.change(within(volume).getByLabelText('Volume'), { target: { value: '0.4' } });
    expect(JSON.parse(localStorage.getItem('portico.player.volume.v1') ?? '{}')).toMatchObject({ volume: 0.4, muted: false });
  });

  it('shows real trickplay tiles and keeps diagnostics behind the saved preference', async () => {
    const source = new FixturePorticoDataSource();
    localStorage.setItem('portico.web.installation-preferences.v1', JSON.stringify({ playbackDiagnostics: true }));
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());
    const mediaTrickplay = vi.spyOn(source as PorticoDataSource, 'mediaTrickplay').mockResolvedValue([{
      id: 'trick-1', mediaId: 'episode-1', width: 160, height: 90, tileWidth: 160, tileHeight: 90,
      intervalSeconds: 10, durationSeconds: 120, tileCount: 12, stale: false, createdAt: '2026-01-01T00:00:00Z',
    }]);

    renderPlayer(source);
    const playerSurface = await screen.findByLabelText('Now playing The Castle');
    await waitFor(() => expect(mediaTrickplay).toHaveBeenCalled());
    const timeline = screen.getByLabelText('Playback position');
    vi.spyOn(timeline, 'getBoundingClientRect').mockReturnValue({ left: 0, right: 100, width: 100, top: 0, bottom: 10, height: 10, x: 0, y: 0, toJSON: () => ({}) });
    await waitFor(() => {
      fireEvent.mouseMove(timeline, { clientX: 50 });
      expect(document.querySelector('.trickplay-preview img')).toBeInTheDocument();
    });
    const preview = document.querySelector('.trickplay-preview img') as HTMLImageElement;
    expect(preview.src).toContain('/api/media/episode-1/trickplay/trick-1/tiles/6.jpg');

    fireEvent.click(screen.getByRole('button', { name: 'Playback settings' }));
    const settings = await screen.findByRole('dialog', { name: 'Playback settings' });
    fireEvent.click(within(settings).getByRole('button', { name: 'Show technical stats' }));
    const diagnostics = await screen.findByRole('dialog', { name: 'Playback diagnostics' });
    expect(screen.queryByRole('dialog', { name: 'Playback settings' })).not.toBeInTheDocument();
    expect(playerSurface.closest('[inert]')).not.toBeNull();
    expect(within(diagnostics).getByText('Direct Play')).toBeInTheDocument();
    fireEvent.click(within(diagnostics).getByRole('button', { name: 'Close playback diagnostics' }));
    expect(screen.queryByRole('dialog', { name: 'Playback diagnostics' })).not.toBeInTheDocument();
    await waitFor(() => expect(playerSurface.closest('[inert]')).toBeNull());
  });

  it('defaults discovered browser captions off and renders only the selected cue in Portico', async () => {
    const source = new FixturePorticoDataSource();
    const track = new EventTarget() as TextTrack & { mode: TextTrackMode; activeCues: TextTrackCueList | null };
    Object.assign(track, { kind: 'captions', label: 'English CC', language: 'en', mode: 'showing', activeCues: null });
    const tracks = new EventTarget() as TextTrackList & { 0: TextTrack; length: number };
    Object.assign(tracks, { 0: track, length: 1 });
    vi.spyOn(HTMLMediaElement.prototype, 'textTracks', 'get').mockReturnValue(tracks);
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback());
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    const view = renderPlayer(source);
    await screen.findByLabelText('Now playing The Castle');
    await waitFor(() => expect(track.mode).toBe('disabled'));
    fireEvent.click(screen.getByRole('button', { name: 'Subtitles' }));
    const subtitleDialog = await screen.findByRole('dialog', { name: 'Subtitles' });
    const english = within(subtitleDialog).getByRole('radio', { name: /English CC/ });
    fireEvent.click(english);
    await waitFor(() => expect(track.mode).toBe('hidden'));
    expect(english).toHaveAttribute('aria-checked', 'true');

    Object.assign(track, { activeCues: { 0: { text: 'Breaking news from London' }, length: 1 } as unknown as TextTrackCueList });
    track.dispatchEvent(new Event('cuechange'));
    expect((await screen.findByText('Breaking news from London')).closest('.player-subtitle-layer')).toBeInTheDocument();
    view.unmount();
    expect(track.mode).toBe('disabled');
  });

  it('does not rebuild the video source when selecting an external text subtitle', async () => {
    const source = new FixturePorticoDataSource();
    const subtitle = { id: 'subtitle-en', kind: 'subtitle', codec: 'webvtt', language: 'en', displayTitle: 'English', sourceUrl: '/api/media/episode-1/subtitles/subtitle-en.vtt' } as PlaybackResponse['subtitleStreams'][number];
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ subtitleStreams: [subtitle] }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue(queue());

    const view = renderPlayer(source);
    const surface = await screen.findByLabelText('Now playing The Castle');
    const media = surface.querySelector('video') as HTMLVideoElement;
    await waitFor(() => expect(media.src).toContain('/api/playback-resources/direct-original-en'));
    const originalSource = media.src;
    const loadCalls = vi.mocked(HTMLMediaElement.prototype.load).mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Subtitles' }));
    fireEvent.click(within(await screen.findByRole('dialog', { name: 'Subtitles' })).getByRole('radio', { name: /English/ }));
    expect(media.src).toBe(originalSource);
    expect(HTMLMediaElement.prototype.load).toHaveBeenCalledTimes(loadCalls);
    view.unmount();
  });

  it('falls back to readable plain lyrics when synchronized timing is malformed', async () => {
    const source = new FixturePorticoDataSource();
    const musicMedia = {
      ...coreMedia('track-plain-lyrics', 'Quiet Hours'),
      type: 'track',
      lyrics: [{ id: 'lyrics-broken', source: 'embedded', format: 'lrc', synced: true, text: '[00:99.00]Broken timing\nAll the words remain', createdAt: '2026-01-01T00:00:00Z' }],
    } as PlaybackResponse['media'];
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ media: musicMedia, queue: [] }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue({ ...queue(), current: musicMedia, items: [], history: [], total: 0 });

    renderPlayer(source, 'track-plain-lyrics');
    await screen.findByLabelText('Now playing Quiet Hours');
    fireEvent.click(screen.getByRole('button', { name: 'Lyrics' }));
    const lyricDialog = await screen.findByRole('dialog', { name: 'Lyrics' });
    expect(within(lyricDialog).queryByRole('region', { name: 'Synchronized lyrics' })).not.toBeInTheDocument();
    expect(within(lyricDialog).getByText(/Broken timing/)).toHaveTextContent('All the words remain');
    expect(within(lyricDialog).queryByRole('button', { name: 'Follow current lyric' })).not.toBeInTheDocument();
  });

  it.each([
    ['track', 'track-1', 'Night Drive', 'mode-music'],
    ['audiobook', 'book-1', 'The Long Way Home', 'mode-audiobook'],
  ] as const)('renders %s playback with its audio-first artwork mode', async (type, id, title, className) => {
    const source = new FixturePorticoDataSource();
    const media = { ...coreMedia(id, title), type } as PlaybackResponse['media'];
    vi.spyOn(source as PorticoDataSource, 'startPlayback').mockResolvedValue(playback({ media, queue: [] }));
    vi.spyOn(source as PorticoDataSource, 'playbackSessionQueue').mockResolvedValue({ ...queue(), current: media, items: [], history: [], total: 0 });

    renderPlayer(source, id);
    const surface = await screen.findByLabelText(`Now playing ${title}`);
    expect(surface).toHaveClass(className);
    expect(surface.querySelector('.audio-artwork img')).toHaveAttribute('src', 'http://localhost:3000/poster.jpg');
  });
});
