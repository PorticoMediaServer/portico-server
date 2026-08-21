import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ARTWORK_FAILURE_TTL_MS,
  artworkFailureCacheVersion,
  artworkFailureExpiresAt,
  clearArtworkFailureCache,
  rememberArtworkFailure,
  subscribeArtworkFailureCache,
} from './artworkFailureCache';

afterEach(() => {
  clearArtworkFailureCache();
  vi.useRealTimers();
});

describe('artwork failure cache', () => {
  it('owns expiry and notifies mounted consumers when a failure expires', () => {
    vi.useFakeTimers();
    const listener = vi.fn();
    const unsubscribe = subscribeArtworkFailureCache(listener);
    const expiresAt = rememberArtworkFailure('/artwork/failed');

    expect(artworkFailureExpiresAt('/artwork/failed')).toBe(expiresAt);
    expect(listener).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(ARTWORK_FAILURE_TTL_MS - 1);
    expect(artworkFailureExpiresAt('/artwork/failed')).toBeGreaterThan(Date.now());
    vi.advanceTimersByTime(1);
    expect(artworkFailureExpiresAt('/artwork/failed')).toBe(0);
    expect(listener).toHaveBeenCalledTimes(2);
    expect(artworkFailureCacheVersion()).toBe(2);

    unsubscribe();
  });

  it('replaces one source timer instead of allowing an older retry to expire it', () => {
    vi.useFakeTimers();
    rememberArtworkFailure('/artwork/failed');
    vi.advanceTimersByTime(10_000);
    const renewed = rememberArtworkFailure('/artwork/failed');

    vi.advanceTimersByTime(20_000);
    expect(artworkFailureExpiresAt('/artwork/failed')).toBe(renewed);
    vi.advanceTimersByTime(10_000);
    expect(artworkFailureExpiresAt('/artwork/failed')).toBe(0);
  });

  it('cancels owned expiry timers when the cache is cleared', () => {
    vi.useFakeTimers();
    rememberArtworkFailure('/artwork/failed');
    expect(vi.getTimerCount()).toBe(1);
    clearArtworkFailureCache();
    expect(vi.getTimerCount()).toBe(0);
    expect(artworkFailureExpiresAt('/artwork/failed')).toBe(0);
  });
});
