import { positiveFullJitterDelay } from "@porticomediaserver/client-core";

const ROUTE_PUBLICATION_RETRY_CAPS_MS = [2_000, 4_000, 8_000, 15_000, 30_000] as const;
export const ROUTE_PUBLICATION_RETRY_HORIZON_MS = 180_000;

/** Positive capped equal jitter prevents hot loops and newly claimed servers polling in lockstep. */
export function routePublicationRetryDelay(attempt: number, cohort: string, serverId: string): number {
  const cap = ROUTE_PUBLICATION_RETRY_CAPS_MS[Math.min(
    Math.max(0, Math.floor(attempt)),
    ROUTE_PUBLICATION_RETRY_CAPS_MS.length - 1,
  )];
  const floor = Math.ceil(cap / 2);
  return floor + positiveFullJitterDelay(cap - floor, `${cohort}:${serverId}`, attempt);
}

/**
 * Fresh claims may need certificate, origin, heartbeat, and signed-route
 * publication to converge. Keep one patient owner for three minutes, while
 * ensuring its final timer never extends the bounded setup window.
 */
export function routePublicationRetryPlan(
  attempt: number,
  elapsedMs: number,
  cohort: string,
  serverId: string,
): { delayMs: number } | undefined {
  const remainingMs = ROUTE_PUBLICATION_RETRY_HORIZON_MS - Math.max(0, Math.floor(elapsedMs));
  if (remainingMs <= 0) return undefined;
  return { delayMs: Math.min(routePublicationRetryDelay(attempt, cohort, serverId), remainingMs) };
}
