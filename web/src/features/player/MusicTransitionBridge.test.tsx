import type { MediaItem, PlaybackPreparedResponse, PlaybackResponse, PlaybackSessionQueueResponse } from '@porticomediaserver/client-core';
import { createRef, type ReactNode } from 'react';
import { act, render, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { defaultWebDisplayPreferences } from '../../preferences/webDisplayPreferences';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { continuePreloadedAudioUntilPrimaryPlays, MusicTransitionBridge, musicTransitionOwnerKey, musicTransitionRequest, nextCandidate } from './MusicTransitionBridge';
import { PlaybackPreparationOwner } from './PlaybackPreparationOwner';

describe('music transitions', () => {
  afterEach(() => vi.restoreAllMocks());
  it('retains delayed zero-crossfade preparation, bypasses a pending preload play, and hands off once', async () => {
    vi.spyOn(HTMLMediaElement.prototype, 'canPlayType').mockReturnValue('probably');
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(() => undefined);
    const pendingPreloadPlay = vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(() => new Promise<void>(() => undefined));
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => undefined);
    const media = document.createElement('video');
    Object.defineProperty(media, 'paused', { configurable: true, value: false });
    Object.defineProperty(media, 'duration', { configurable: true, value: 15 });
    Object.defineProperty(media, 'currentTime', { configurable: true, writable: true, value: 14.9 });
    Object.defineProperty(media, 'seeking', { configurable: true, value: false });
    const mediaRef = createRef<HTMLVideoElement>();
    mediaRef.current = media;
    const current = { id: 'track-1', title: 'One' } as MediaItem;
    const candidate = { id: 'track-2', title: 'Two' } as MediaItem;
    const playback = { sessionId: 'session-1', media: current, queue: [candidate], timeline: { durationSeconds: 15 } } as PlaybackResponse;
    const queue = { items: [current, candidate] } as PlaybackSessionQueueResponse;
    const preferences = { shuffleDefault: false, repeatDefault: 'none' as const, autoplayDefault: true, normalizationMode: 'off' as const, crossfadeSeconds: 0, gapless: true };
    let resolve!: (value: PlaybackPreparedResponse) => void;
    const pending = new Promise<PlaybackPreparedResponse>((done) => { resolve = done; });
    const prepare = vi.fn(() => pending);
    const firstHandoff = vi.fn().mockResolvedValue({ ...playback, sessionId: 'session-2', media: candidate });
    const rerenderedHandoff = vi.fn(firstHandoff);
    const source = new FixturePorticoDataSource();
    const wrap = (bridge: ReactNode) => <DataProvider source={source}>{bridge}</DataProvider>;
    const view = render(wrap(<MusicTransitionBridge playback={playback} queue={queue} mediaRef={mediaRef} preferences={preferences} volume={0.7} muted={false} enabled onTransitioning={vi.fn()} handoff={firstHandoff} prepareNext={prepare} />));
    await waitFor(() => expect(prepare).toHaveBeenCalledOnce());
    view.rerender(wrap(<MusicTransitionBridge playback={{ ...playback }} queue={{ ...queue }} mediaRef={mediaRef} preferences={{ ...preferences }} volume={0.2} muted enabled onTransitioning={vi.fn()} handoff={rerenderedHandoff} prepareNext={vi.fn(prepare)} />));
    await act(async () => resolve({ preparedSessionId: 'prepared-2', playback: {
      ...playback,
      sessionId: 'prepared',
      media: candidate,
      mediaGrant: { token: 'grant', expiresAt: '2099-01-01T00:00:00Z' },
      sourceUrl: '/next.flac',
      resources: [{ id: 'direct', sourceUrl: '/next.flac', streamFormat: 'direct', qualityId: 'original', default: true }],
      qualities: [{ id: 'original', label: 'Original', description: 'Original quality' }],
      streamFormat: 'direct',
      directPlay: true,
    }, expiresAt: new Date(Date.now() + 60_000).toISOString() } as PlaybackPreparedResponse));
    await waitFor(() => expect((view.container.querySelector('audio') as HTMLAudioElement).src).toContain('/next.flac'));
    media.dispatchEvent(new Event('timeupdate'));
    await waitFor(() => expect(rerenderedHandoff).toHaveBeenCalledOnce());
    expect(pendingPreloadPlay).not.toHaveBeenCalled();
    expect(firstHandoff).toHaveBeenCalledOnce();
    view.rerender(wrap(<MusicTransitionBridge playback={{ ...playback, sessionId: 'session-changed' }} queue={queue} mediaRef={mediaRef} preferences={preferences} volume={0.2} muted enabled onTransitioning={vi.fn()} handoff={firstHandoff} prepareNext={prepare} />));
    expect(rerenderedHandoff).toHaveBeenCalledOnce();
    view.unmount();
  });
  it('uses the server queue as the canonical next-track order', () => {
    const current = { id: 'track-1' } as MediaItem;
    const fallback = { id: 'track-fallback' } as MediaItem;
    const queued = { id: 'track-2' } as MediaItem;
    const playback = { media: current, queue: [fallback] } as PlaybackResponse;
    const queue = { items: [queued] } as PlaybackSessionQueueResponse;
    expect(nextCandidate(playback, queue)).toBe(queued);
  });

  it('ignores an accidental duplicate of the current item', () => {
    const current = { id: 'track-1' } as MediaItem;
    const queued = { id: 'track-2' } as MediaItem;
    expect(nextCandidate({ media: current, queue: [current, queued] } as PlaybackResponse, undefined)).toBe(queued);
  });

  it('keeps one preparation owner across equivalent queue projections', () => {
    const current = { id: 'track-1' } as MediaItem;
    const queued = { id: 'track-2' } as MediaItem;
    const remaining = { id: 'track-3' } as MediaItem;
    const playback = { sessionId: 'session-1', playbackRevision: 3, media: current, queue: [current, queued, remaining] } as PlaybackResponse;
    const preferences = {
      shuffleDefault: false,
      repeatDefault: 'none' as const,
      autoplayDefault: true,
      normalizationMode: 'off' as const,
      crossfadeSeconds: 0,
      gapless: true,
    };
    const first = musicTransitionOwnerKey(playback, { items: [current, queued, remaining] } as PlaybackSessionQueueResponse, preferences, defaultWebDisplayPreferences);
    const projected = musicTransitionOwnerKey(
      { ...playback, playbackRevision: 99, media: { ...current }, queue: [{ ...queued }, { ...remaining }] } as PlaybackResponse,
      { items: [{ ...queued }, { ...remaining }] } as PlaybackSessionQueueResponse,
      { ...preferences },
      { ...defaultWebDisplayPreferences },
    );
    expect(projected).toBe(first);
    expect(musicTransitionOwnerKey(playback, { items: [current, { id: 'track-4' } as MediaItem, remaining] } as PlaybackSessionQueueResponse, preferences, defaultWebDisplayPreferences)).not.toBe(first);

  });

  it('shares one provider-owned preparation between early bridge and natural-ended fallback', async () => {
    const owner = new PlaybackPreparationOwner();
    const response = { preparedSessionId: 'prepared-2' } as PlaybackPreparedResponse;
    let resolve!: (value: PlaybackPreparedResponse) => void;
    const pending = new Promise<PlaybackPreparedResponse>((done) => { resolve = done; });
    const post = vi.fn(() => pending);
    const key = 'session-1:track-2:request';

    const bridge = owner.prepare(key, post);
    const endedBeforeResolution = owner.prepare(key, post);
    expect(endedBeforeResolution).toBe(bridge);
    expect(post).toHaveBeenCalledOnce();
    resolve(response);
    await expect(bridge).resolves.toBe(response);
    const endedAfterResolution = owner.prepare(key, post);
    expect(endedAfterResolution).toBe(bridge);
    await expect(endedAfterResolution).resolves.toBe(response);
    expect(post).toHaveBeenCalledOnce();
  });

  it('lets natural-ended start first and the early bridge consume the same result', async () => {
    const owner = new PlaybackPreparationOwner();
    const response = { preparedSessionId: 'prepared-2' } as PlaybackPreparedResponse;
    const post = vi.fn().mockResolvedValue(response);
    const ended = owner.prepare('session-1:track-2:request', post);
    const bridge = owner.prepare('session-1:track-2:request', post);
    expect(bridge).toBe(ended);
    await expect(bridge).resolves.toBe(response);
    expect(post).toHaveBeenCalledOnce();
  });

  it('shares a failed preparation once and only explicit retry replaces it', async () => {
    const owner = new PlaybackPreparationOwner();
    const failure = new Error('reviewed preparation failure');
    const post = vi.fn().mockRejectedValueOnce(failure).mockResolvedValue({ preparedSessionId: 'retry-2' } as PlaybackPreparedResponse);
    const first = owner.prepare('session-1:track-2:request', post);
    const shared = owner.prepare('session-1:track-2:request', post);
    expect(shared).toBe(first);
    await expect(first).rejects.toBe(failure);
    await expect(shared).rejects.toBe(failure);
    expect(post).toHaveBeenCalledOnce();
    const retry = owner.prepare('session-1:track-2:request', post, true);
    await expect(retry).resolves.toMatchObject({ preparedSessionId: 'retry-2' });
    expect(post).toHaveBeenCalledTimes(2);
  });

  it('keeps one preparation result across live request re-projection cadence', async () => {
    const owner = new PlaybackPreparationOwner();
    const post = vi.fn().mockResolvedValue({ preparedSessionId: 'prepared' } as PlaybackPreparedResponse);
    const ownerKey = JSON.stringify({ sessionId: 'session-1', candidateId: 'track-2' });
    const first = owner.prepare(ownerKey, post);
    await first;
    for (let render = 0; render < 16; render += 1) {
      const projectedKey = JSON.stringify({ sessionId: 'session-1', candidateId: 'track-2' });
      expect(owner.prepare(projectedKey, post)).toBe(first);
    }
    expect(post).toHaveBeenCalledOnce();

    owner.prepare(JSON.stringify({ sessionId: 'session-1', candidateId: 'track-4' }), post);
    owner.prepare(JSON.stringify({ sessionId: 'session-2', candidateId: 'track-4' }), post);
    expect(post).toHaveBeenCalledTimes(3);
  });

  it('keeps the settled provider result when PlayerSurface remounts the same session', async () => {
    const owner = new PlaybackPreparationOwner();
    const post = vi.fn().mockResolvedValue({ preparedSessionId: 'prepared-2' } as PlaybackPreparedResponse);
    const key = JSON.stringify({ sessionId: 'session-1', candidateId: 'track-2' });
    const firstMount = owner.prepare(key, post);
    await firstMount;

    owner.preserveSession('session-1');
    const remountedSurface = owner.prepare(key, post);
    expect(remountedSurface).toBe(firstMount);
    expect(post).toHaveBeenCalledOnce();

    owner.preserveSession('session-2');
    owner.prepare(JSON.stringify({ sessionId: 'session-2', candidateId: 'track-2' }), post);
    expect(post).toHaveBeenCalledTimes(2);
  });

  it('builds the same exact preparation request for bridge and ended consumers', () => {
    const current = { id: 'track-1' } as MediaItem;
    const candidate = { id: 'track-2' } as MediaItem;
    const remaining = { id: 'track-3' } as MediaItem;
    const playback = { sessionId: 'session-1', media: current, queue: [candidate, remaining] } as PlaybackResponse;
    const queue = { items: [current, candidate, remaining], sourceContext: { kind: 'album', id: 'album-1' } } as unknown as PlaybackSessionQueueResponse;
    const preferences = { shuffleDefault: false, repeatDefault: 'none' as const, autoplayDefault: true, normalizationMode: 'off' as const, crossfadeSeconds: 0, gapless: true };
    expect(musicTransitionRequest(playback, queue, preferences, defaultWebDisplayPreferences)).toEqual({
      mediaId: 'track-2',
      queueMediaIds: ['track-3'],
      crossfadeSeconds: 0,
      preferredHandoff: 'gapless',
      sourceContext: queue.sourceContext,
      commitPreviousEnd: true,
      intent: expect.any(Object),
    });
  });

  it('keeps preloaded audio alive until the primary next source is actually playing', () => {
    const primary = document.createElement('video');
    const preload = document.createElement('audio');
    const primaryPlay = vi.spyOn(primary, 'play').mockResolvedValue(undefined);
    const preloadPause = vi.spyOn(preload, 'pause').mockImplementation(() => undefined);
    const preloadLoad = vi.spyOn(preload, 'load').mockImplementation(() => undefined);
    const settled = vi.fn();
    Object.defineProperty(primary, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_NOTHING });
    Object.defineProperty(primary, 'duration', { configurable: true, value: 180 });
    preload.src = '/next.mp3';
    preload.currentTime = 1.25;

    continuePreloadedAudioUntilPrimaryPlays({
      primary,
      preload,
      volume: 0.8,
      muted: false,
      normalization: undefined,
      normalizationMode: 'off',
      onSettled: settled,
    });

    expect(preloadPause).not.toHaveBeenCalled();
    preload.currentTime = 2.5;
    primary.dispatchEvent(new Event('loadedmetadata'));
    expect(primary.currentTime).toBe(2.5);
    expect(primaryPlay).toHaveBeenCalledOnce();
    expect(preloadPause).not.toHaveBeenCalled();

    primary.dispatchEvent(new Event('playing'));
    expect(preloadPause).toHaveBeenCalledOnce();
    expect(preloadLoad).toHaveBeenCalledOnce();
    expect(preload.hasAttribute('src')).toBe(false);
    expect(settled).toHaveBeenCalledOnce();
  });
});
