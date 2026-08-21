const MAX_TOKEN_LENGTH = 16_384;
const MAX_CURSOR_LENGTH = 4_096;
const MAX_PAGE_ITEMS = 10_000;

export function decodeHighRiskResponse(path: string, method: string, value: unknown): unknown {
  const pathname = path.split("?", 1)[0] ?? path;
  const verb = method.toUpperCase();

  if (verb === "POST" && isNativeCredentialPath(pathname)) return nativeCredentials(value);
  if (verb === "POST" && pathname === "/api/auth/sessions/refresh") return credentialRotation(value);
  if (verb === "POST" && /^\/api\/download-preparations\/[^/]+\/grant$/.test(pathname)) return downloadGrant(value, true);
  if (verb === "POST" && /^\/api\/media\/[^/]+\/download-grants$/.test(pathname)) return downloadGrant(value, false);
  if (verb === "POST" && /^\/api\/playback-sessions\/[^/]+\/media-grant$/.test(pathname)) return mediaGrant(value);
  if (/^\/api\/playback-sessions\/[^/]+\/continuation$/.test(pathname)) {
    if (verb === "GET") return continuationState(value);
    if (verb === "POST") return continuationCredential(value);
  }
  if (/^\/api\/playback-sessions\/[^/]+\/command$/.test(pathname) && (verb === "GET" || verb === "POST")) return playbackCommand(value);
  if (isAuthLifecyclePath(pathname, verb)) return authLifecycle(value);
  if (/^\/api\/account\/servers\/[^/]+\/routes$/.test(pathname) && verb === "GET") return routeDocument(value);
  if (pathname.startsWith("/api/watch-with-friends/groups")) return watchWithFriendsResponse(value);
  if (isPlaybackResponsePath(pathname) && looksLikePlaybackResponse(value)) return playbackResponse(value);

  validatePaginationEnvelope(value);
  return value;
}

function isPlaybackResponsePath(pathname: string): boolean {
  return pathname === "/api/playback-sessions" || pathname === "/api/playback/active" ||
    /^\/api\/playback-sessions\/[^/]+\/(?:handoff|renegotiate|prepare-next)$/.test(pathname) ||
    /^\/api\/dvr\/recordings\/[^/]+\/play$/.test(pathname) ||
    /^\/api\/live-tv\/streams\/[^/]+\/open$/.test(pathname) ||
    /^\/api\/library-channels\/[^/]+\/tune$/.test(pathname);
}

function isAuthLifecyclePath(pathname: string, verb: string): boolean {
  return (pathname === "/api/auth/me" && verb === "GET") ||
    (["/api/auth/login", "/api/auth/setup", "/api/auth/profile-sessions/browser"].includes(pathname) && verb === "POST");
}

function authLifecycle(value: unknown): unknown {
  const record = object(value, "authentication response");
  rejectTopLevelCredentialFields(record);
  if (typeof record.authenticated !== "boolean") throw new TypeError("authentication state is invalid");
  if (record.setupRequired !== undefined && typeof record.setupRequired !== "boolean") throw new TypeError("authentication setup state is invalid");
  if (record.authenticated && !record.user) throw new TypeError("authenticated viewer identity is missing");
  if (record.user !== undefined) object(record.user, "authenticated user");
  for (const field of ["accountId", "serverId", "profileId", "profileIdentityId", "authorizationRevision"] as const) {
    if (record[field] !== undefined) boundedString(record[field], `authentication ${field}`, 512);
  }
  if (record.authority !== undefined) boundedString(record.authority, "authentication authority", 16, ["local", "hosted"]);
  return value;
}

function routeDocument(value: unknown): unknown {
  const record = object(value, "Hosted route document");
  rejectTopLevelCredentialFields(record);
  for (const field of ["serverId", "serverName", "serverPublicKey", "serverPublicKeyFingerprint", "signature", "signatureAlgorithm", "audience"] as const) {
    boundedString(record[field], `Hosted route ${field}`, field === "serverPublicKey" || field === "signature" ? 16_384 : 2_048);
  }
  timestamp(record.issuedAt, "Hosted route issue time");
  timestamp(record.expiresAt, "Hosted route expiry");
  nonNegativeInteger(record.documentVersion, "Hosted route document version");
  if (!Array.isArray(record.routes) || record.routes.length > 64) throw new TypeError("Hosted routes are invalid");
  for (const candidate of record.routes) {
    const route = object(candidate, "Hosted route");
    if (Object.prototype.hasOwnProperty.call(route, "serverToken")) throw new TypeError("Hosted route contains a protected field");
    boundedString(route.type, "Hosted route type", 64);
    boundedString(route.url, "Hosted route URL", 8_192);
    boundedString(route.quality, "Hosted route quality", 64);
  }
  return value;
}

function watchWithFriendsResponse(value: unknown): unknown {
  const record = object(value, "Watch With Friends response");
  if (Array.isArray(record.items)) {
    if (record.items.length > 1_000) throw new TypeError("Watch With Friends groups are invalid");
    for (const group of record.items) watchWithFriendsGroup(group);
    return value;
  }
  watchWithFriendsGroup(value);
  return value;
}

function watchWithFriendsGroup(value: unknown): void {
  const group = object(value, "Watch With Friends group");
  for (const field of ["id", "mediaId", "name", "ownerProfileId"] as const) boundedString(group[field], `Watch With Friends ${field}`, 512);
  for (const field of ["revision", "reconnectGeneration", "playbackRevision"] as const) nonNegativeInteger(group[field], `Watch With Friends ${field}`);
  finiteNumber(group.positionSeconds, "Watch With Friends position", 0);
  finiteNumber(group.playbackRate, "Watch With Friends playback rate", 0.1);
  boundedString(group.state, "Watch With Friends state", 32, ["paused", "playing", "stopped"]);
  for (const field of ["serverTime", "positionUpdatedAt", "createdAt", "updatedAt"] as const) timestamp(group[field], `Watch With Friends ${field}`);
  if (!Array.isArray(group.members) || group.members.length > 1_000 || !Array.isArray(group.queue) || group.queue.length > 10_000) throw new TypeError("Watch With Friends members or queue are invalid");
  playbackCommand(group.command);
}

function looksLikePlaybackResponse(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  return Object.prototype.hasOwnProperty.call(record, "sessionId") && Object.prototype.hasOwnProperty.call(record, "sourceUrl") && Object.prototype.hasOwnProperty.call(record, "decision");
}

function playbackResponse(value: unknown): unknown {
  const record = object(value, "playback response");
  // These are deliberately structured, short-lived playback grants. They
  // remain validated below; allowing the envelope names here prevents the
  // generic top-level credential guard from confusing a typed playback
  // response with an accidental session/token envelope.
  rejectTopLevelCredentialFields(record, ["continuationCredential"]);
  boundedString(record.sessionId, "playback session id", 512);
  boundedString(record.sourceUrl, "playback source URL", 8_192);
  if (typeof record.directPlay !== "boolean") throw new TypeError("playback direct-play state is invalid");
  for (const field of ["generation", "nextEventSequence", "playbackRevision", "queueRevision"] as const) nonNegativeInteger(record[field], `playback ${field}`);
  object(record.decision, "playback decision");
  object(record.media, "playback media");
  mediaGrant(record.mediaGrant);
  continuationCredential(record.continuationCredential);
  for (const field of ["resources", "audioStreams", "subtitleStreams", "chapters", "qualities", "queue"] as const) {
    if (!Array.isArray(record[field]) || record[field].length > 10_000) throw new TypeError(`playback ${field} is invalid`);
  }
  return value;
}

function isNativeCredentialPath(pathname: string): boolean {
  return pathname === "/api/auth/sessions" || pathname === "/api/auth/profile-sessions/native" ||
    pathname === "/api/auth/quick-connect/exchange" || pathname === "/api/auth/tv-setup/redeem";
}

function nativeCredentials(value: unknown): unknown {
  const record = object(value, "native session credentials");
  rejectTopLevelCredentialFields(record, ["accessToken", "refreshToken"]);
  boundedString(record.tokenType, "credential token type", 16, "Bearer");
  boundedString(record.authority, "credential authority", 16, ["local", "hosted"]);
  for (const field of ["accessToken", "refreshToken"] as const) boundedString(record[field], `credential ${field}`, MAX_TOKEN_LENGTH);
  for (const field of ["accountId", "serverId", "profileId", "authorizationRevision"] as const) boundedString(record[field], `credential ${field}`, 512);
  timestamp(record.accessExpiresAt, "credential access expiry");
  timestamp(record.refreshExpiresAt, "credential refresh expiry");
  object(record.user, "credential user");
  object(record.device, "credential device");
  return value;
}

function credentialRotation(value: unknown): unknown {
  const record = object(value, "credential rotation");
  rejectTopLevelCredentialFields(record, ["accessToken", "refreshToken"]);
  boundedString(record.tokenType, "credential token type", 16, "Bearer");
  for (const field of ["accessToken", "refreshToken"] as const) boundedString(record[field], `credential ${field}`, MAX_TOKEN_LENGTH);
  timestamp(record.accessExpiresAt, "credential access expiry");
  timestamp(record.refreshExpiresAt, "credential refresh expiry");
  return value;
}

function downloadGrant(value: unknown, requiresTransferToken: boolean): unknown {
  const record = object(value, "download grant");
  rejectTopLevelCredentialFields(record, ["grantToken"]);
  boundedString(record.downloadUrl, "download grant URL", 8_192);
  if (requiresTransferToken || record.grantToken !== undefined) boundedString(record.grantToken, "download grant token", MAX_TOKEN_LENGTH);
  boundedString(record.profile, "download grant profile", 256);
  timestamp(record.expiresAt, "download grant expiry");
  return value;
}

function mediaGrant(value: unknown): unknown {
  const record = object(value, "media grant");
  rejectTopLevelCredentialFields(record, ["token"]);
  boundedString(record.token, "media grant token", MAX_TOKEN_LENGTH);
  timestamp(record.expiresAt, "media grant expiry");
  return value;
}

function continuationCredential(value: unknown): unknown {
  const record = object(value, "playback continuation credential");
  rejectTopLevelCredentialFields(record, ["token"]);
  boundedString(record.token, "playback continuation token", MAX_TOKEN_LENGTH);
  boundedString(record.origin, "playback continuation origin", 2_048);
  timestamp(record.expiresAt, "playback continuation expiry");
  nonNegativeInteger(record.generation, "playback continuation generation");
  return value;
}

function continuationState(value: unknown): unknown {
  const record = object(value, "playback continuation state");
  boundedString(record.sessionId, "playback continuation session", 512);
  boundedString(record.state, "playback continuation state", 32, ["playing", "paused", "buffering", "stopped"]);
  for (const field of ["generation", "highestEventSequence", "playbackRevision", "queueRevision"] as const) nonNegativeInteger(record[field], `playback continuation ${field}`);
  finiteNumber(record.positionSeconds, "playback continuation position", 0);
  if (record.mediaGrantExpiresAt !== undefined) timestamp(record.mediaGrantExpiresAt, "playback media grant expiry");
  return value;
}

function playbackCommand(value: unknown): unknown {
  const record = object(value, "playback command");
  boundedString(record.id, "playback command id", 512);
  boundedString(record.action, "playback command action", 32, ["play", "pause", "seek", "stop", "load", "next", "previous"]);
  if (record.issuedAt !== undefined) timestamp(record.issuedAt, "playback command timestamp");
  if (record.positionSeconds !== undefined) finiteNumber(record.positionSeconds, "playback command position", 0);
  if (record.mediaId !== undefined) boundedString(record.mediaId, "playback command mediaId", 512, undefined, true);
  if (record.message !== undefined) boundedString(record.message, "playback command message", 2_000, undefined, true);
  return value;
}

function validatePaginationEnvelope(value: unknown): void {
  if (!value || typeof value !== "object" || Array.isArray(value)) return;
  const record = value as Record<string, unknown>;
  if (Object.prototype.hasOwnProperty.call(record, "items") && (!Array.isArray(record.items) || record.items.length > MAX_PAGE_ITEMS)) throw new TypeError("pagination items are invalid");
  if (Object.prototype.hasOwnProperty.call(record, "nextCursor")) nullableCursor(record.nextCursor);
  if (Object.prototype.hasOwnProperty.call(record, "pageInfo")) {
    const pageInfo = object(record.pageInfo, "pagination page info");
    if (Object.prototype.hasOwnProperty.call(pageInfo, "nextCursor")) nullableCursor(pageInfo.nextCursor);
    if (pageInfo.total !== undefined) nonNegativeInteger(pageInfo.total, "pagination total");
  }
}

function nullableCursor(value: unknown): void {
  if (value !== null && value !== undefined) boundedString(value, "pagination cursor", MAX_CURSOR_LENGTH, undefined, true);
}

function object(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${name} is invalid`);
  return value as Record<string, unknown>;
}

function rejectTopLevelCredentialFields(record: Record<string, unknown>, allowed: readonly string[] = []): void {
  const allowedSet = new Set(allowed);
  const protectedNames = new Set([
    "accesstoken", "authorization", "cookie", "password", "passwordhash", "privatekey",
    "refreshtoken", "secret", "secretkey", "servertoken", "sessioncookie"
  ]);
  for (const key of Object.keys(record)) {
    const normalized = key.replace(/[-_]/g, "").toLowerCase();
    const credentialShaped = protectedNames.has(normalized) ||
      normalized.endsWith("token") || normalized.endsWith("secret") || normalized.endsWith("credential") ||
      normalized.endsWith("cookie") || normalized.endsWith("password") || normalized.endsWith("privatekey") ||
      normalized.endsWith("apikey") || normalized.endsWith("jwt") ||
      /(?:token|secret|credential|password|cookie|privatekey|apikey|jwt)(?:hash|value|material|data)$/u.test(normalized);
    if (credentialShaped && !allowedSet.has(key)) {
      throw new TypeError("The response contains a protected field");
    }
  }
}

function boundedString(value: unknown, name: string, maximum: number, allowed?: string | readonly string[], allowEmpty = false): string {
  if (typeof value !== "string" || value.length > maximum || /[\u0000-\u001f\u007f]/u.test(value) || (!allowEmpty && !value.trim())) throw new TypeError(`${name} is invalid`);
  if (typeof allowed === "string" && value !== allowed) throw new TypeError(`${name} is invalid`);
  if (Array.isArray(allowed) && !allowed.includes(value)) throw new TypeError(`${name} is invalid`);
  return value;
}

function timestamp(value: unknown, name: string): void {
  const candidate = boundedString(value, name, 128);
  if (!Number.isFinite(Date.parse(candidate))) throw new TypeError(`${name} is invalid`);
}

function nonNegativeInteger(value: unknown, name: string): void {
  if (!Number.isSafeInteger(value) || Number(value) < 0) throw new TypeError(`${name} is invalid`);
}

function finiteNumber(value: unknown, name: string, minimum: number): void {
  if (typeof value !== "number" || !Number.isFinite(value) || value < minimum) throw new TypeError(`${name} is invalid`);
}
