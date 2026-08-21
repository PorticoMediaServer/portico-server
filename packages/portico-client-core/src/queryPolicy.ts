import {
  normalizeViewerScope,
  viewerCacheKey,
  type ViewerScope
} from "./viewerScope.js";

export const PORTICO_QUERY_CONTRACT_REVISION = "portico-query-v1";
export const PORTICO_QUERY_STALE_TIME_MS = 30_000;
export const PORTICO_QUERY_RETAIN_TIME_MS = 15 * 60_000;

export type ViewerQueryPrefix = readonly [
  "portico",
  "v1",
  ViewerScope["authority"],
  string,
  string,
  string,
  string
];

/**
 * Human-readable prefix shared by every client cache. The complete verified
 * viewer boundary is deliberately visible in diagnostics and devtools.
 */
export function viewerQueryPrefix(scope: ViewerScope): ViewerQueryPrefix {
  const normalized = normalizeViewerScope(scope);
  return [
    "portico",
    "v1",
    normalized.authority,
    normalized.accountId,
    normalized.serverId,
    normalized.profileId,
    normalized.authorizationRevision
  ];
}

/**
 * Canonical logical resource identity. Transport URLs never participate: a
 * verified route change must not fragment otherwise identical server data.
 */
export function viewerQueryKey(
  scope: ViewerScope,
  resource: string,
  parameters: Record<string, unknown> = {},
  contractRevision = PORTICO_QUERY_CONTRACT_REVISION
): readonly unknown[] {
  const normalized = normalizeViewerScope(scope);
  return [
    ...viewerQueryPrefix(normalized),
    resource,
    viewerCacheKey({
      ...normalized,
      contractRevision,
      resource,
      parameters
    })
  ];
}

export function viewerQueryScopeKey(
  scope: ViewerScope,
  contractRevision = PORTICO_QUERY_CONTRACT_REVISION
): string {
  const normalized = normalizeViewerScope(scope);
  return viewerCacheKey({
    ...normalized,
    contractRevision,
    resource: "viewer-query-runtime"
  });
}

function errorStatus(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const status = (error as { status?: unknown }).status;
  return typeof status === "number" && Number.isFinite(status) ? status : undefined;
}

function terminalAuthorizationFailure(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const code = (error as { code?: unknown }).code;
  return typeof code === "string" && [
    "membership_inactive",
    "server_not_found",
    "server_session_revoked",
    "account_disabled",
    "device_not_allowed"
  ].includes(code);
}

/**
 * Shared retry boundary for idempotent server-state reads. Authentication,
 * authorization, malformed requests, and cancelled work are lifecycle events,
 * not retry loops. Transient transport, timeout, throttling, and 5xx failures
 * receive a small bounded retry budget.
 */
export function shouldRetryPorticoQuery(
  failureCount: number,
  error: unknown,
  maximumRetries = 2
): boolean {
  if (failureCount >= maximumRetries) return false;
  if ((error as { name?: unknown } | null)?.name === "AbortError") return false;
  if (terminalAuthorizationFailure(error)) return false;
  const status = errorStatus(error);
  if (status === undefined || status === 0) return true;
  if (status === 408 || status === 425 || status === 429) return true;
  return status >= 500;
}
