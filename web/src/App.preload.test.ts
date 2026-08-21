import { describe, expect, it, vi } from 'vitest';
import { scheduleProductRoutePreload, shouldPreloadProductRoutes } from './App';

describe('product route preload policy', () => {
  it('respects offline and data-saving connection hints', () => {
    expect(shouldPreloadProductRoutes({ onLine: false })).toBe(false);
    expect(shouldPreloadProductRoutes({ connection: { saveData: true } })).toBe(false);
    expect(shouldPreloadProductRoutes({ connection: { effectiveType: '2g' } })).toBe(false);
    expect(shouldPreloadProductRoutes({ connection: { effectiveType: 'slow-2g' } })).toBe(false);
    expect(shouldPreloadProductRoutes({ connection: { downlink: 1.4 } })).toBe(false);
  });

  it('allows preloading when the browser reports a usable connection', () => {
    expect(shouldPreloadProductRoutes({ onLine: true, connection: { effectiveType: '4g', downlink: 10 } })).toBe(true);
    expect(shouldPreloadProductRoutes({})).toBe(true);
  });

  it('waits for meaningful idle time and cancels the scheduled work', () => {
    let idleCallback: IdleRequestCallback | undefined;
    const requestIdleCallback = vi.fn((callback: IdleRequestCallback) => {
      idleCallback = callback;
      return 7;
    });
    const cancelIdleCallback = vi.fn();
    const originalRequest = Object.getOwnPropertyDescriptor(window, 'requestIdleCallback');
    const originalCancel = Object.getOwnPropertyDescriptor(window, 'cancelIdleCallback');
    Object.defineProperty(window, 'requestIdleCallback', { configurable: true, value: requestIdleCallback });
    Object.defineProperty(window, 'cancelIdleCallback', { configurable: true, value: cancelIdleCallback });
    try {
      const task = vi.fn();
      const cancel = scheduleProductRoutePreload(task);
      expect(task).not.toHaveBeenCalled();
      idleCallback?.({ didTimeout: false, timeRemaining: () => 49 } as IdleDeadline);
      expect(task).not.toHaveBeenCalled();
      idleCallback?.({ didTimeout: false, timeRemaining: () => 50 } as IdleDeadline);
      expect(task).toHaveBeenCalledTimes(1);
      cancel();
      expect(cancelIdleCallback).toHaveBeenCalledWith(7);
    } finally {
      if (originalRequest) Object.defineProperty(window, 'requestIdleCallback', originalRequest);
      else Reflect.deleteProperty(window, 'requestIdleCallback');
      if (originalCancel) Object.defineProperty(window, 'cancelIdleCallback', originalCancel);
      else Reflect.deleteProperty(window, 'cancelIdleCallback');
    }
  });
});
