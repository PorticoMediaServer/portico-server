import { describe, expect, it, vi } from 'vitest';
import { createSeekTransaction, playbackStallReason } from './playerLifecycle';

describe('player lifecycle transactions', () => {
  it('fences a stale seek and resumes only the latest transaction', async () => {
    const media = document.createElement('video');
    Object.defineProperty(media, 'paused', { configurable: true, get: () => false });
    Object.defineProperty(media, 'duration', { configurable: true, value: 120 });
    const play = vi.spyOn(media, 'play').mockResolvedValue(undefined);
    const pause = vi.spyOn(media, 'pause').mockImplementation(() => undefined);
    const transaction = createSeekTransaction(media, 100);

    const first = transaction.seek(30);
    const second = transaction.seek(60);

    await expect(first).resolves.toBe('superseded');
    await expect(second).resolves.toBe('completed');
    expect(media.currentTime).toBe(60);
    expect(pause).toHaveBeenCalledTimes(2);
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('finishes an overrun HLS seek from 56.4 seconds without assigning the exact 60-second endpoint', async () => {
    const media = document.createElement('video');
    Object.defineProperty(media, 'paused', { configurable: true, get: () => true });
    Object.defineProperty(media, 'duration', { configurable: true, value: 60 });
    media.currentTime = 56.4;
    const play = vi.spyOn(media, 'play').mockResolvedValue(undefined);
    vi.spyOn(media, 'pause').mockImplementation(() => undefined);
    const transaction = createSeekTransaction(media, 100);

    await expect(transaction.seek(media.currentTime + 30)).resolves.toBe('completed');
    expect(media.currentTime).toBeCloseTo(59.9, 5);
    expect(media.currentTime).toBeLessThan(media.duration);
    expect(play).toHaveBeenCalledOnce();
  });

  it('does not classify pause, seek, background, or buffered playback as a stall', () => {
    const media = document.createElement('video');
    Object.defineProperty(media, 'paused', { configurable: true, value: true });
    expect(playbackStallReason(media, false, 0)).toBe('paused');
    Object.defineProperty(media, 'paused', { configurable: true, value: false });
    Object.defineProperty(media, 'seeking', { configurable: true, value: true });
    expect(playbackStallReason(media, false, 0)).toBe('seeking');
    Object.defineProperty(media, 'seeking', { configurable: true, value: false });
    expect(playbackStallReason(media, true, 0)).toBe('background');
    Object.defineProperty(media, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_FUTURE_DATA });
    expect(playbackStallReason(media, false, 2)).toBe('buffered');
    Object.defineProperty(media, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_METADATA });
    expect(playbackStallReason(media, false, 0)).toBe('stalled');
  });
});
