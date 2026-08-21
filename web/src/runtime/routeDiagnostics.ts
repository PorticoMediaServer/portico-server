import type { ErrorInfo } from 'react';

export const PORTICO_ROUTE_DIAGNOSTIC_EVENT = 'portico:route-diagnostic';
const storageKey = 'portico.route-diagnostics.v1';
const maximumStoredFailures = 10;
const maximumTrackedQueries = 20;

type QueryDiagnosticStatus = 'loading' | 'success' | 'error';

type QueryDiagnostic = {
  resource: string;
  status: QueryDiagnosticStatus;
  requestId?: string;
  recordedAt: string;
};

export type StoredRouteDiagnostic = {
  version: 1;
  id: string;
  occurredAt: string;
  routeFamily: string;
  fingerprint: string;
  buildId: string;
  dataState: {
    loading: number;
    success: number;
    error: number;
    recentResources: string[];
  };
  correlationIds: string[];
};

export type RouteDiagnosticUpload = {
  device: 'web';
  app: 'Portico Web';
  entries: Array<{
    level: 'error';
    message: 'route-render-failure';
    timestamp: string;
    context: Record<string, string>;
  }>;
};

const recentQueries = new Map<string, QueryDiagnostic>();

function safeOpaqueIdentifier(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  return /^[A-Za-z0-9._:-]{1,128}$/.test(trimmed) ? trimmed : undefined;
}

function correlationId(reason: unknown): string | undefined {
  if (!reason || typeof reason !== 'object') return undefined;
  const candidate = reason as { requestId?: unknown; details?: { requestId?: unknown } };
  return safeOpaqueIdentifier(candidate.requestId) ?? safeOpaqueIdentifier(candidate.details?.requestId);
}

function resourceFamily(queryKey: string): string {
  const resource = queryKey.split(':', 1)[0]?.trim().toLowerCase() || 'query';
  return /^[a-z0-9-]{1,48}$/.test(resource) ? resource : 'query';
}

export function recordRouteDataState(queryKey: string, status: QueryDiagnosticStatus, reason?: unknown): void {
  const resource = resourceFamily(queryKey);
  recentQueries.delete(queryKey);
  recentQueries.set(queryKey, {
    resource,
    status,
    ...(correlationId(reason) ? { requestId: correlationId(reason) } : {}),
    recordedAt: new Date().toISOString(),
  });
  while (recentQueries.size > maximumTrackedQueries) recentQueries.delete(recentQueries.keys().next().value as string);
}

export function routeFamily(routeKey: string): string {
  const pathname = routeKey.split('?')[0] || '/';
  const parts = pathname.split('/').filter(Boolean);
  if (parts.length === 0) return '/';
  if (['library', 'media', 'watch', 'person'].includes(parts[0])) return `/${parts[0]}/:id`;
  if (parts[0] === 'settings') return `/settings/${parts[1] || 'index'}`;
  return `/${parts[0]}`;
}

export function failureFingerprint(value: string): string {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `view-${(hash >>> 0).toString(16).padStart(8, '0')}`;
}

function querySnapshot(): StoredRouteDiagnostic['dataState'] {
  const entries = [...recentQueries.values()];
  return {
    loading: entries.filter((entry) => entry.status === 'loading').length,
    success: entries.filter((entry) => entry.status === 'success').length,
    error: entries.filter((entry) => entry.status === 'error').length,
    recentResources: [...new Set(entries.map((entry) => entry.resource))].slice(-10),
  };
}

function newDiagnosticId(): string {
  try {
    return globalThis.crypto?.randomUUID?.() ?? `view-${Date.now().toString(36)}`;
  } catch {
    return `view-${Date.now().toString(36)}`;
  }
}

export function readStoredRouteDiagnostics(): StoredRouteDiagnostic[] {
  if (typeof window === 'undefined') return [];
  try {
    const parsed = JSON.parse(window.localStorage.getItem(storageKey) ?? '[]');
    return Array.isArray(parsed) ? parsed.slice(-maximumStoredFailures) as StoredRouteDiagnostic[] : [];
  } catch {
    return [];
  }
}

function storeDiagnostic(record: StoredRouteDiagnostic): void {
  if (typeof window === 'undefined') return;
  try {
    const records = [...readStoredRouteDiagnostics(), record].slice(-maximumStoredFailures);
    window.localStorage.setItem(storageKey, JSON.stringify(records));
  } catch {
    // Diagnostics must never cause or replace the product failure being recorded.
  }
}

export function recordRouteRenderFailure(
  error: Error,
  info: ErrorInfo,
  routeKey = '/',
  development = import.meta.env.DEV,
): StoredRouteDiagnostic | undefined {
  if (development) {
    console.error('Portico route render failed.', { error, componentStack: info.componentStack });
    return undefined;
  }

  const family = routeFamily(routeKey);
  const dataState = querySnapshot();
  const correlationIds = [...new Set([...recentQueries.values()].map((entry) => entry.requestId).filter((value): value is string => Boolean(value)))].slice(-5);
  const record: StoredRouteDiagnostic = {
    version: 1,
    id: newDiagnosticId(),
    occurredAt: new Date().toISOString(),
    routeFamily: family,
    fingerprint: failureFingerprint(`${family}\n${error.name}\n${info.componentStack ?? ''}`),
    buildId: import.meta.env.VITE_PORTICO_BUILD_ID || 'unknown',
    dataState,
    correlationIds,
  };
  storeDiagnostic(record);
  console.error('Portico route render failed.', record);

  if (typeof window !== 'undefined') {
    const upload: RouteDiagnosticUpload = {
      device: 'web',
      app: 'Portico Web',
      entries: [{
        level: 'error',
        message: 'route-render-failure',
        timestamp: record.occurredAt,
        context: {
          routeFamily: record.routeFamily,
          fingerprint: record.fingerprint,
          buildId: record.buildId,
          dataState: `loading=${dataState.loading},success=${dataState.success},error=${dataState.error}`,
          recentResources: dataState.recentResources.join(','),
          correlationIds: correlationIds.join(','),
        },
      }],
    };
    window.dispatchEvent(new CustomEvent<RouteDiagnosticUpload>(PORTICO_ROUTE_DIAGNOSTIC_EVENT, { detail: upload }));
  }
  return record;
}
