import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  artworkFailureCacheVersion,
  clearArtworkFailureCache,
  hasArtworkFailure,
  rememberArtworkFailure,
  subscribeArtworkFailureCache,
} from './artworkFailureCache';

afterEach(() => {
  clearArtworkFailureCache();
});

describe('artwork failure cache', () => {
  it('retains a failed URL without a timer-driven retry loop', () => {
    vi.useFakeTimers();
    const listener = vi.fn();
    const unsubscribe = subscribeArtworkFailureCache('/artwork/failed', listener);
    rememberArtworkFailure('/artwork/failed');

    expect(hasArtworkFailure('/artwork/failed')).toBe(true);
    expect(listener).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);

    unsubscribe();
    vi.useRealTimers();
  });

  it('does not republish the same failure', () => {
    const listener = vi.fn();
    const unsubscribe = subscribeArtworkFailureCache('/artwork/failed', listener);
    rememberArtworkFailure('/artwork/failed');
    rememberArtworkFailure('/artwork/failed');

    expect(listener).toHaveBeenCalledTimes(1);
    expect(hasArtworkFailure('/artwork/failed')).toBe(true);
    unsubscribe();
  });

  it('clears failures at the explicit viewer artwork fence', () => {
    rememberArtworkFailure('/artwork/failed');
    clearArtworkFailureCache();
    expect(hasArtworkFailure('/artwork/failed')).toBe(false);
    expect(artworkFailureCacheVersion('/artwork/failed')).toMatch(/^\d+:false$/);
  });

  it('notifies only subscribers for the URL whose state changed', () => {
    const matching = vi.fn();
    const unrelated = vi.fn();
    const unsubscribeMatching = subscribeArtworkFailureCache('/artwork/failed', matching);
    const unsubscribeUnrelated = subscribeArtworkFailureCache('/artwork/unrelated', unrelated);

    rememberArtworkFailure('/artwork/failed');

    expect(matching).toHaveBeenCalledTimes(1);
    expect(unrelated).not.toHaveBeenCalled();
    unsubscribeMatching();
    unsubscribeUnrelated();
  });
});
