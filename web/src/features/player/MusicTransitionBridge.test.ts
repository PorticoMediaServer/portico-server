import type { MediaItem, PlaybackResponse, PlaybackSessionQueueResponse } from '@portico/client-core';
import { describe, expect, it, vi } from 'vitest';
import { continuePreloadedAudioUntilPrimaryPlays, nextCandidate } from './MusicTransitionBridge';

describe('music transitions', () => {
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
