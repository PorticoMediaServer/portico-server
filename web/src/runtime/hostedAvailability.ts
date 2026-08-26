import { isAmbiguousPorticoError, isRetryablePorticoError, positiveFullJitterDelay } from '@porticomediaserver/client-core';
import { useEffect, useRef, useState } from 'react';

export const HOSTED_AVAILABILITY_RETRY_MS = 5_000;
export const HOSTED_AVAILABILITY_WARNING_DELAY_MS = 8_000;

export function createHostedAvailabilityRetryCohort(): string {
  const values = new Uint32Array(4);
  globalThis.crypto.getRandomValues(values);
  return `page-${Array.from(values, value => value.toString(16).padStart(8, '0')).join('')}`;
}

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

export function hostedAvailabilityRetryDelay(reason: unknown, now = Date.now(), cohort = ''): number {
  const retryAfter = hostedRetryAfterDelay(reason, now);
  if (!cohort.trim())
    return Math.max(HOSTED_AVAILABILITY_RETRY_MS, retryAfter);
  const spread = cohort.trim()
    ? positiveFullJitterDelay(HOSTED_AVAILABILITY_RETRY_MS, cohort.trim())
    : HOSTED_AVAILABILITY_RETRY_MS;
  return retryAfter > 0 ? retryAfter + spread : spread;
}

export function hostedAvailabilityCopy(offline: boolean): HostedAvailabilityCopy {
  if (offline) {
    return {
      title: 'Still waiting for a connection',
      body: 'Portico will continue automatically when this device is back online.',
    };
  }
  return {
    title: 'Still connecting to Portico',
    body: 'This is taking longer than usual. Portico will keep trying automatically.',
  };
}

export function useHostedAvailabilityRetry({
  enabled,
  reason,
  retry,
  cohort,
  failureStartedAt,
}: {
  enabled: boolean;
  reason: unknown;
  retry: () => void;
  cohort?: string;
  failureStartedAt?: number;
}): { automatic: boolean; offline: boolean; showWarning: boolean; copy: HostedAvailabilityCopy } {
  const retryRef = useRef(retry);
  retryRef.current = retry;
  const pageCohortRef = useRef<string | undefined>(undefined);
  if (pageCohortRef.current === undefined) pageCohortRef.current = createHostedAvailabilityRetryCohort();
  const retryCohort = cohort?.trim() || pageCohortRef.current;
  const retryMetadata =
    reason && typeof reason === 'object' ? (reason as RetryMetadata) : undefined;
  const retryAfterMs =
    typeof retryMetadata?.retryAfterMs === 'number' &&
    Number.isFinite(retryMetadata.retryAfterMs)
      ? retryMetadata.retryAfterMs
      : undefined;
  const retryAt =
    typeof retryMetadata?.retryAt === 'string'
      ? retryMetadata.retryAt
      : undefined;
  const [offline, setOffline] = useState(() => typeof navigator !== 'undefined' && navigator.onLine === false);
  const [showWarning, setShowWarning] = useState(false);
  // The runtime provider has already classified the original failure before
  // enabling this hook. `reason` intentionally contains only bounded retry
  // timing metadata, so re-classifying it here would discard the original
  // HTTP/transport evidence and incorrectly turn automatic recovery off.
  const automatic = enabled;

  useEffect(() => {
    if (!enabled) {
      setShowWarning(false);
      return;
    }
    const elapsed = Number.isFinite(failureStartedAt)
      ? Math.max(0, Date.now() - (failureStartedAt as number))
      : 0;
    const remaining = Math.max(0, HOSTED_AVAILABILITY_WARNING_DELAY_MS - elapsed);
    if (remaining === 0) {
      setShowWarning(true);
      return;
    }
    setShowWarning(false);
    const timer = window.setTimeout(
      () => setShowWarning(true),
      remaining,
    );
    return () => window.clearTimeout(timer);
  }, [enabled, failureStartedAt]);

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
    // Keep the Retry-After timestamp separate from the bounded jitter window.
    // A hidden tab waits one fresh bounded interval when it becomes visible;
    // an explicit provider deadline remains a strict lower bound.
    const retryAfterAt =
      scheduledAt +
      hostedRetryAfterDelay({retryAfterMs, retryAt}, scheduledAt);
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
      const now = Date.now();
      // Retry-After is an absolute lower bound. Every initial, online, and
      // visibility-resume attempt then receives its own positive cohort spread
      // so a returning browser fleet cannot collapse onto that boundary.
      const spread = hostedAvailabilityRetryDelay(undefined, now, retryCohort);
      timer = window.setTimeout(run, Math.max(0, retryAfterAt - now) + spread);
    };
    const resumeOnline = () => {
      if (navigator.onLine === false) return;
      schedule();
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
  }, [automatic, retryAfterMs, retryAt, retryCohort]);

  return {
    automatic,
    offline,
    showWarning: enabled && showWarning,
    copy: hostedAvailabilityCopy(offline),
  };
}
