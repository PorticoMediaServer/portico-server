import { isAmbiguousPorticoError, isRetryablePorticoError, productMessage } from '@porticomediaserver/client-core';
import { useEffect, useRef, useState } from 'react';

export const HOSTED_AVAILABILITY_RETRY_MS = 5_000;

type RetryMetadata = {
  retryAfterMs?: unknown;
  retryAt?: unknown;
};

export type HostedAvailabilityCopy = {
  title: string;
  body: string;
};

export function automaticHostedAvailabilityRetry(reason: unknown): boolean {
  if (reason instanceof DOMException && reason.name === 'TimeoutError') return true;
  if (reason === undefined || reason === null || isAmbiguousPorticoError(reason)) return false;
  const status = typeof reason === 'object' && typeof (reason as { status?: unknown }).status === 'number'
    ? (reason as { status: number }).status
    : undefined;
  const transientHTTP = status === 408 || status === 425 || status === 429 || (status !== undefined && status >= 500);
  return transientHTTP || isRetryablePorticoError(reason);
}

function hostedRetryAfterDelay(reason: unknown, now: number): number {
  const metadata = reason && typeof reason === 'object' ? reason as RetryMetadata : undefined;
  const retryAfterMs = typeof metadata?.retryAfterMs === 'number' && Number.isFinite(metadata.retryAfterMs)
    ? Math.max(0, metadata.retryAfterMs)
    : 0;
  const retryAtMs = typeof metadata?.retryAt === 'string' ? Date.parse(metadata.retryAt) : Number.NaN;
  return Number.isFinite(retryAtMs) ? Math.max(0, retryAtMs - now) : retryAfterMs;
}

export function hostedAvailabilityRetryDelay(reason: unknown, now = Date.now()): number {
  return Math.max(HOSTED_AVAILABILITY_RETRY_MS, hostedRetryAfterDelay(reason, now));
}

export function hostedAvailabilityCopy(offline: boolean): HostedAvailabilityCopy {
  if (offline) {
    return {
      title: productMessage('problem.offline').title ?? "You're offline",
      body: 'Reconnect to the network. Portico will resume automatically when your connection returns.',
    };
  }
  return {
    title: productMessage('problem.cloud-unavailable').title ?? 'Portico Account services unavailable',
    body: 'Portico couldn’t reach account services. It will keep trying automatically.',
  };
}

export function useHostedAvailabilityRetry({
  enabled,
  reason,
  retry,
}: {
  enabled: boolean;
  reason: unknown;
  retry: () => void;
}): { automatic: boolean; offline: boolean; copy: HostedAvailabilityCopy } {
  const retryRef = useRef(retry);
  retryRef.current = retry;
  const [offline, setOffline] = useState(() => typeof navigator !== 'undefined' && navigator.onLine === false);
  // The runtime provider has already classified the original failure before
  // enabling this hook. `reason` intentionally contains only bounded retry
  // timing metadata, so re-classifying it here would discard the original
  // HTTP/transport evidence and incorrectly turn automatic recovery off.
  const automatic = enabled;

  useEffect(() => {
    if (!enabled) return;
    const update = () => setOffline(navigator.onLine === false);
    update();
    window.addEventListener('online', update);
    window.addEventListener('offline', update);
    return () => {
      window.removeEventListener('online', update);
      window.removeEventListener('offline', update);
    };
  }, [enabled]);

  useEffect(() => {
    if (!automatic) return;
    let timer: number | undefined;
    let completed = false;
    const scheduledAt = Date.now();
    const retryAfterAt = scheduledAt + hostedRetryAfterDelay(reason, scheduledAt);
    const clear = () => {
      if (timer !== undefined) window.clearTimeout(timer);
      timer = undefined;
    };
    const run = () => {
      if (completed || document.visibilityState === 'hidden' || navigator.onLine === false) return;
      completed = true;
      clear();
      retryRef.current();
    };
    const schedule = () => {
      clear();
      if (completed || document.visibilityState === 'hidden' || navigator.onLine === false) return;
      timer = window.setTimeout(run, Math.max(HOSTED_AVAILABILITY_RETRY_MS, retryAfterAt - Date.now()));
    };
    const resumeOnline = () => {
      if (navigator.onLine === false) return;
      clear();
      const remaining = retryAfterAt - Date.now();
      if (remaining > 0) timer = window.setTimeout(run, remaining);
      else run();
    };
    const visibilityChanged = () => {
      if (document.visibilityState === 'hidden') clear();
      else schedule();
    };
    schedule();
    window.addEventListener('online', resumeOnline);
    window.addEventListener('offline', clear);
    document.addEventListener('visibilitychange', visibilityChanged);
    return () => {
      clear();
      window.removeEventListener('online', resumeOnline);
      window.removeEventListener('offline', clear);
      document.removeEventListener('visibilitychange', visibilityChanged);
    };
  }, [automatic, reason]);

  return { automatic, offline, copy: hostedAvailabilityCopy(offline) };
}
