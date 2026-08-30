import { sameViewerScope, type ViewerScope } from '@porticomediaserver/client-core';

export const AMBIENT_COOKIE_QUARANTINE_KEY = 'portico-ambient-cookie-quarantine-v1';
export const AMBIENT_COOKIE_RESERVATION_HEAD_KEY = 'portico-ambient-cookie-reservation-head-v1';
export const AMBIENT_COOKIE_RESERVATION_PREFIX = 'portico-ambient-cookie-reservation-v1:';

export type AmbientCookieMutationKind =
  | 'logout'
  | 'profile-session'
  | 'browser-account-switch'
  | 'browser-account-remove'
  | 'browser-account-sign-out-all';

export type AmbientCookieExpectedIdentity = Pick<ViewerScope, 'authority' | 'accountId'>
  & Partial<Pick<ViewerScope, 'serverId' | 'profileId' | 'authorizationRevision'>>;

export type AmbientCookieMutationIntent =
  | { state: 'signed-out' }
  | { state: 'authenticated'; expected: AmbientCookieExpectedIdentity };

export type AmbientCookieQuarantineMarker = {
  version: 1;
  mutationId: string;
  mutationKind: AmbientCookieMutationKind;
  markedAt: string;
  intent: AmbientCookieMutationIntent;
};

export type AmbientCookieRestoreStatus = {
  trustedForRestore: boolean;
  quarantined: boolean;
  marker?: AmbientCookieQuarantineMarker;
};

function storage(): Storage {
  if (typeof window === 'undefined' || !window.localStorage) {
    throw new Error('Durable browser storage is unavailable.');
  }
  return window.localStorage;
}

function nonEmpty(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0;
}

function validExpectedIdentity(value: unknown): value is AmbientCookieExpectedIdentity {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<AmbientCookieExpectedIdentity>;
  return (candidate.authority === 'hosted' || candidate.authority === 'local')
    && nonEmpty(candidate.accountId)
    && (candidate.serverId === undefined || nonEmpty(candidate.serverId))
    && (candidate.profileId === undefined || nonEmpty(candidate.profileId))
    && (candidate.authorizationRevision === undefined || nonEmpty(candidate.authorizationRevision));
}

function parseMarker(raw: string): AmbientCookieQuarantineMarker | undefined {
  const value = JSON.parse(raw) as Partial<AmbientCookieQuarantineMarker> | null;
  if (!value
    || value.version !== 1
    || !nonEmpty(value.mutationId)
    || !['logout', 'profile-session', 'browser-account-switch', 'browser-account-remove', 'browser-account-sign-out-all'].includes(value.mutationKind ?? '')
    || !nonEmpty(value.markedAt)
    || !Number.isFinite(Date.parse(value.markedAt))
    || !value.intent
    || (value.intent.state !== 'signed-out'
      && (value.intent.state !== 'authenticated' || !validExpectedIdentity(value.intent.expected)))) {
    return undefined;
  }
  return value as AmbientCookieQuarantineMarker;
}

function readMarker(target: Storage): AmbientCookieRestoreStatus {
  const raw = target.getItem(AMBIENT_COOKIE_QUARANTINE_KEY);
  let active: AmbientCookieQuarantineMarker | undefined;
  if (raw !== null) {
    active = parseMarker(raw);
    if (!active) return { trustedForRestore: false, quarantined: true };
  }
  const reservation = readLatestReservation(target);
  if (!reservation.trusted) return {trustedForRestore: false, quarantined: true};
  if (reservation.marker) {
    return {
      trustedForRestore: true,
      quarantined: true,
      marker: reservation.marker.mutationId === active?.mutationId ? active : reservation.marker,
    };
  }
  if (active) return { trustedForRestore: true, quarantined: true, marker: active };
  return { trustedForRestore: true, quarantined: false };
}

function reservationKey(mutationId: string): string {
  return `${AMBIENT_COOKIE_RESERVATION_PREFIX}${mutationId}`;
}

function readLatestReservation(target: Storage): {trusted: boolean; marker?: AmbientCookieQuarantineMarker} {
  try {
    const raw = target.getItem(AMBIENT_COOKIE_RESERVATION_HEAD_KEY);
    if (raw === null) return {trusted: true};
    const head = parseMarker(raw);
    if (!head) return {trusted: false};
    const ownedRaw = target.getItem(reservationKey(head.mutationId));
    // A dangling head is the successfully released previous owner. It is not a
    // quarantine; the unique reservation is the durable in-flight authority.
    if (ownedRaw === null) return {trusted: true};
    const owned = parseMarker(ownedRaw);
    if (!owned || JSON.stringify(owned) !== JSON.stringify(head)) return {trusted: false};
    return {trusted: true, marker: owned};
  } catch {
    return {trusted: false};
  }
}

export function ambientCookieRestoreStatus(): AmbientCookieRestoreStatus {
  try {
    return readMarker(storage());
  } catch {
    return { trustedForRestore: false, quarantined: true };
  }
}

/** Read-verifies that this exact mutation still owns the origin-wide marker. */
export function ownsAmbientCookieMutation(marker: AmbientCookieQuarantineMarker): boolean {
  try {
    const target = storage();
    const raw = target.getItem(AMBIENT_COOKIE_QUARANTINE_KEY);
    if (raw === null) return false;
    const current = parseMarker(raw);
    if (!current
      || current.mutationId !== marker.mutationId
      || JSON.stringify(current) !== JSON.stringify(marker)) return false;
    const reservation = readLatestReservation(target);
    return reservation.trusted
      && (!reservation.marker
        || reservation.marker.mutationId === marker.mutationId);
  } catch {
    return false;
  }
}

/** Publishes this context's durable intent before it waits for the shared lock. */
export function reserveAmbientCookieMutation(
  mutationKind: AmbientCookieMutationKind,
  intent: AmbientCookieMutationIntent,
): AmbientCookieQuarantineMarker | undefined {
  if (intent.state === 'authenticated' && !validExpectedIdentity(intent.expected)) return undefined;
  const mutationId = globalThis.crypto?.randomUUID?.();
  if (!mutationId) return undefined;
  const marker: AmbientCookieQuarantineMarker = {
    version: 1,
    mutationId,
    mutationKind,
    markedAt: new Date().toISOString(),
    intent,
  };
  try {
    const target = storage();
    const key = reservationKey(mutationId);
    target.setItem(key, JSON.stringify(marker));
    target.setItem(AMBIENT_COOKIE_RESERVATION_HEAD_KEY, JSON.stringify(marker));
    const latest = readLatestReservation(target);
    return latest.trusted
      && latest.marker?.mutationId === marker.mutationId
      && JSON.stringify(latest.marker) === JSON.stringify(marker)
      ? marker
      : undefined;
  } catch {
    return undefined;
  }
}

/** Claims the active marker only when no newer/foreign reservation exists. */
export function claimAmbientCookieMutation(marker: AmbientCookieQuarantineMarker): boolean {
  try {
    const target = storage();
    const latest = readLatestReservation(target);
    if (!latest.trusted
      || latest.marker?.mutationId !== marker.mutationId
      || JSON.stringify(latest.marker) !== JSON.stringify(marker)) return false;
    return writeAndVerify(marker) && ownsAmbientCookieMutation(marker);
  } catch {
    return false;
  }
}

/** Removes only this context's reservation; an active failure marker remains. */
export function releaseAmbientCookieMutationReservation(marker: AmbientCookieQuarantineMarker): boolean {
  try {
    const target = storage();
    target.removeItem(reservationKey(marker.mutationId));
    return target.getItem(reservationKey(marker.mutationId)) === null;
  } catch {
    return false;
  }
}

function writeAndVerify(marker: AmbientCookieQuarantineMarker): boolean {
  try {
    const target = storage();
    target.setItem(AMBIENT_COOKIE_QUARANTINE_KEY, JSON.stringify(marker));
    const status = readMarker(target);
    return status.trustedForRestore
      && status.quarantined
      && status.marker?.mutationId === marker.mutationId
      && JSON.stringify(status.marker.intent) === JSON.stringify(marker.intent);
  } catch {
    return false;
  }
}

function replaceOwnedMarker(
  owner: AmbientCookieQuarantineMarker,
  replacement: AmbientCookieQuarantineMarker,
): boolean {
  try {
    const target = storage();
    if (!ownsAmbientCookieMutation(owner)) return false;
    target.setItem(AMBIENT_COOKIE_QUARANTINE_KEY, JSON.stringify(replacement));
    return ownsAmbientCookieMutation(replacement);
  } catch {
    return false;
  }
}

/**
 * Synchronously publishes and read-verifies the restart barrier. Callers must
 * do this before the first await and before invoking a cookie-mutating API.
 */
export function beginAmbientCookieMutation(
  mutationKind: AmbientCookieMutationKind,
  intent: AmbientCookieMutationIntent,
): AmbientCookieQuarantineMarker | undefined {
  if (intent.state === 'authenticated' && !validExpectedIdentity(intent.expected)) return undefined;
  const mutationId = globalThis.crypto?.randomUUID?.();
  if (!mutationId) return undefined;
  const marker: AmbientCookieQuarantineMarker = {
    version: 1,
    mutationId,
    mutationKind,
    markedAt: new Date().toISOString(),
    intent,
  };
  return writeAndVerify(marker) ? marker : undefined;
}

/**
 * Narrows a pre-mutation account expectation to the exact authoritative scope
 * returned by the mutating endpoint, before the independent final /me read.
 */
export function bindAmbientCookieMutationToViewer(
  marker: AmbientCookieQuarantineMarker,
  scope: ViewerScope,
): AmbientCookieQuarantineMarker | undefined {
  if (marker.intent.state !== 'authenticated') return undefined;
  const expected = marker.intent.expected;
  if (scope.authority !== expected.authority
    || scope.accountId !== expected.accountId
    || (expected.serverId !== undefined && scope.serverId !== expected.serverId)
    || (expected.profileId !== undefined && scope.profileId !== expected.profileId)
    || (expected.authorizationRevision !== undefined
      && scope.authorizationRevision !== expected.authorizationRevision)) {
    return undefined;
  }
  const exact: AmbientCookieQuarantineMarker = {
    ...marker,
    intent: { state: 'authenticated', expected: scope },
  };
  // An older async response must not narrow over a marker published by a newer
  // cookie mutation in another context.
  return replaceOwnedMarker(marker, exact) ? exact : undefined;
}

/**
 * Clears only the marker owned by this mutation, after a separate /me has
 * proven the exact intended authenticated identity.
 */
export function clearAmbientCookieAfterVerifiedAuthentication(
  marker: AmbientCookieQuarantineMarker,
  scope: ViewerScope,
): boolean {
  if (marker.intent.state !== 'authenticated') return false;
  const expected = marker.intent.expected;
  if (!('serverId' in expected) || !expected.serverId
    || !expected.profileId
    || !expected.authorizationRevision
    || !sameViewerScope(scope, expected as ViewerScope)) return false;
  try {
    const target = storage();
    const current = readMarker(target);
    if (!current.trustedForRestore
      || current.marker?.mutationId !== marker.mutationId
      || current.marker.intent.state !== 'authenticated'
      || JSON.stringify(current.marker.intent.expected) !== JSON.stringify(expected)) return false;
    target.removeItem(AMBIENT_COOKIE_QUARANTINE_KEY);
    target.removeItem(reservationKey(marker.mutationId));
    const status = readMarker(target);
    return status.trustedForRestore && !status.quarantined;
  } catch {
    return false;
  }
}
