import { ApiError } from '@porticomediaserver/client-core';
import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  automaticHostedAvailabilityRetry,
  hostedAvailabilityCopy,
  hostedAvailabilityRetryDelay,
  useHostedAvailabilityRetry,
} from './hostedAvailability';

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('Hosted availability retry policy', () => {
  it('uses only retryable, non-ambiguous failures and honors Retry-After', () => {
    expect(automaticHostedAvailabilityRetry(new ApiError(503, 'hosted_unavailable', 'unavailable', undefined, { retryable: true }))).toBe(true);
    expect(automaticHostedAvailabilityRetry(new ApiError(503, 'hosted_unavailable', 'unavailable'))).toBe(true);
    expect(automaticHostedAvailabilityRetry(new ApiError(0, 'transport_ambiguous', 'unknown outcome', undefined, { retryable: false, ambiguous: true }))).toBe(false);
    expect(automaticHostedAvailabilityRetry(new ApiError(400, 'invalid_code', 'invalid', undefined, { retryable: false }))).toBe(false);
    expect(hostedAvailabilityRetryDelay({ retryAfterMs: 8_000 }, 1_000)).toBe(8_000);
    expect(hostedAvailabilityRetryDelay({ retryAt: new Date(13_000).toISOString() }, 1_000)).toBe(12_000);
    expect(hostedAvailabilityRetryDelay({ retryAfterMs: 12_000, retryAt: new Date(4_000).toISOString() }, 1_000)).toBe(5_000);
    expect(hostedAvailabilityRetryDelay(undefined, 1_000)).toBe(5_000);
  });

  it('waits for five visible seconds and cleans up its timer', () => {
    vi.useFakeTimers();
    let visibility: DocumentVisibilityState = 'hidden';
    vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility);
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
    const retry = vi.fn();
    const reason = new ApiError(503, 'hosted_unavailable', 'unavailable', undefined, { retryable: true });
    const hook = renderHook(() => useHostedAvailabilityRetry({ enabled: true, reason, retry }));

    act(() => vi.advanceTimersByTime(20_000));
    expect(retry).not.toHaveBeenCalled();
    act(() => {
      visibility = 'visible';
      document.dispatchEvent(new Event('visibilitychange'));
      vi.advanceTimersByTime(4_999);
    });
    expect(retry).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(retry).toHaveBeenCalledTimes(1);

    hook.unmount();
    act(() => vi.advanceTimersByTime(20_000));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it('resumes immediately when the browser comes online', () => {
    vi.useFakeTimers();
    let online = false;
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    vi.spyOn(navigator, 'onLine', 'get').mockImplementation(() => online);
    const retry = vi.fn();
    const reason = new TypeError('Failed to fetch');
    const { result } = renderHook(() => useHostedAvailabilityRetry({ enabled: true, reason, retry }));

    expect(result.current.copy).toEqual(hostedAvailabilityCopy(true));
    act(() => vi.advanceTimersByTime(20_000));
    expect(retry).not.toHaveBeenCalled();
    act(() => {
      online = true;
      window.dispatchEvent(new Event('online'));
    });
    expect(retry).toHaveBeenCalledTimes(1);
    expect(result.current.copy).toEqual(hostedAvailabilityCopy(false));
  });

  it('does not retry before a longer server Retry-After window', () => {
    vi.useFakeTimers();
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(true);
    const retry = vi.fn();
    const reason = new ApiError(503, 'hosted_unavailable', 'unavailable', undefined, { retryAfterMs: 9_000 });
    renderHook(() => useHostedAvailabilityRetry({ enabled: true, reason, retry }));

    act(() => vi.advanceTimersByTime(8_999));
    expect(retry).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(1));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
