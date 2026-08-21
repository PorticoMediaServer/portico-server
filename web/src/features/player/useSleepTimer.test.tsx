/** @vitest-environment jsdom */
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useSleepTimer } from './useSleepTimer';

describe('sleep timer', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('stops playback when a timed deadline expires', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-13T12:00:00Z'));
    const expire = vi.fn();
    const { result } = renderHook(() => useSleepTimer(expire));
    act(() => result.current.setMode(15));
    expect(result.current.label).toBe('15 min');
    act(() => vi.advanceTimersByTime(15 * 60_000));
    expect(expire).toHaveBeenCalledOnce();
    expect(result.current.mode).toBe('off');
  });

  it('can stop at the end of the current track', () => {
    const expire = vi.fn();
    const { result } = renderHook(() => useSleepTimer(expire));
    act(() => result.current.setMode('end'));
    act(() => expect(result.current.expireAtTrackEnd()).toBe(true));
    expect(expire).toHaveBeenCalledOnce();
    expect(result.current.mode).toBe('off');
  });
});
