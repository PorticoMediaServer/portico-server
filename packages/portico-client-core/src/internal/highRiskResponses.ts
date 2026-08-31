const MAX_TOKEN_LENGTH = 16_384;
const MAX_CURSOR_LENGTH = 4_096;
const MAX_PAGE_ITEMS = 10_000;

export function decodeHighRiskResponse(
  path: string,
  method: string,
  value: unknown,
  api: "server" | "hosted" = "server",
): unknown {
  const pathname = path.split("?", 1)[0] ?? path;
  const verb = method.toUpperCase();

  if (verb === "POST" && isNativeCredentialPath(pathname)) return nativeCredentials(value, api);
  if (verb === "POST" && pathname === "/api/auth/sessions/refresh") return credentialRotation(value);
  if (verb === "POST" && /^\/api\/download-preparations\/[^/]+\/grant$/.test(pathname)) return downloadGrant(value, false);
  if (verb === "POST" && /^\/api\/playback-sessions\/[^/]+\/media-grant$/.test(pathname)) return mediaGrant(value);
  if (/^\/api\/playback-sessions\/[^/]+\/continuation$/.test(pathname)) {
    if (verb === "GET") return continuationState(value);
    if (verb === "POST") return continuationCredential(value);
    if (verb === "DELETE") return playbackTerminalAcknowledgement(value);
  }
  if (verb === "DELETE" && /^\/api\/playback-sessions\/[^/]+$/.test(pathname)) return playbackTerminalAcknowledgement(value);
  if (/^\/api\/playback-sessions\/[^/]+\/command$/.test(pathname) && (verb === "GET" || verb === "POST")) return playbackCommand(value);
  if (isAuthLifecyclePath(pathname, verb)) return authLifecycle(value);
  if (/^\/api\/account\/servers\/[^/]+\/routes$/.test(pathname) && verb === "GET") return routeDocument(value);
  if (pathname.startsWith("/api/watch-with-friends/groups")) return watchWithFriendsResponse(value);
  if (isPlaybackResponsePath(pathname)) return playbackRouteResponse(pathname, value);

  validatePaginationEnvelope(value);
  return value;
}

function isPlaybackResponsePath(pathname: string): boolean {
  return pathname === "/api/playback-sessions" || pathname === "/api/playback/active" ||
    /^\/api\/playback-sessions\/[^/]+\/(?:handoff|renegotiate|prepare-next)$/.test(pathname) ||
    /^\/api\/playback\/receivers\/[^/]+\/(?:handoff|handoffs\/[^/]+\/commit)$/.test(pathname) ||
    /^\/api\/dvr\/recordings\/[^/]+\/(?:play|playback)$/.test(pathname) ||
    pathname === "/api/live-tv/play" ||
    /^\/api\/live-tv\/streams\/[^/]+\/open$/.test(pathname) ||
    /^\/api\/library-channels\/[^/]+\/tune$/.test(pathname);
}

function playbackRouteResponse(pathname: string, value: unknown): unknown {
  if (/^\/api\/library-channels\/[^/]+\/tune$/.test(pathname)) {
    const record = object(value, "library channel tune response");
    playbackResponse(record.playback);
    return value;
  }
  if (pathname === "/api/playback/active") {
    const record = object(value, "playback restore response");
    if (typeof record.active !== "boolean") throw new TypeError("playback restore state is invalid");
    if (record.active) {
      if (!looksLikePlaybackResponse(record.playback)) throw new TypeError("active playback response is missing");
      playbackResponse(record.playback);
    } else if (record.playback !== undefined) {
      throw new TypeError("inactive playback response contains a playback plan");
    }
    return value;
  }
  if (/^\/api\/playback-sessions\/[^/]+\/prepare-next$/.test(pathname)) {
    const record = object(value, "prepared playback response");
    boundedString(record.preparedSessionId, "prepared playback session id", 512);
    boundedString(record.handoffMode, "prepared playback handoff mode", 64);
    boundedString(record.preloadPolicy, "prepared playback preload policy", 64);
    timestamp(record.expiresAt, "prepared playback expiry");
    nonNegativeInteger(record.playbackRevision, "prepared playback revision");
    nonNegativeInteger(record.queueRevision, "prepared queue revision");
    if (!Array.isArray(record.queue) || record.queue.length > 10_000) throw new TypeError("prepared playback queue is invalid");
    preparedPlaybackResponse(record.playback);
    return value;
  }
  return playbackResponse(value);
}

function preparedPlaybackResponse(value: unknown): unknown {
  const record = object(value, "prepared playback descriptor");
  // Preparation is a credential-free preload view owned by the still-active
  // source session. A continuation bearer is minted only when handoff commits.
  // Accept the wire's explicit JSON null (and omission for a credential-free
  // descriptor), but fail closed if this boundary ever leaks a bearer.
  if (record.continuationCredential !== null && record.continuationCredential !== undefined) {
    throw new TypeError("prepared playback descriptor contains a continuation credential");
  }
  return playbackResponse(value, true);
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
  if (record.kind !== "route-document") throw new TypeError("Hosted route document kind is invalid");
  for (const field of ["serverId", "serverName", "serverPublicKey", "serverPublicKeyFingerprint", "signature", "signatureAlgorithm", "audience"] as const) {
    boundedString(record[field], `Hosted route ${field}`, field === "serverPublicKey" || field === "signature" ? 16_384 : 2_048);
  }
  timestamp(record.issuedAt, "Hosted route issue time");
  timestamp(record.expiresAt, "Hosted route expiry");
  if (record.documentVersion !== 1) throw new TypeError("Hosted route document version is invalid");
  if (!Array.isArray(record.routes) || record.routes.length > 64) throw new TypeError("Hosted routes are invalid");
	const canonicalRouteTypes = new Set(["lan", "lan_discovered", "lan_ip_encoded", "public_console_origin", "public_direct", "public_direct_ip_encoded"]);
  for (const candidate of record.routes) {
    const route = object(candidate, "Hosted route");
    if (Object.prototype.hasOwnProperty.call(route, "serverToken")) throw new TypeError("Hosted route contains a protected field");
    boundedString(route.type, "Hosted route type", 64);
		if (!canonicalRouteTypes.has(String(route.type))) throw new TypeError("Hosted route type is invalid");
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

function playbackResponse(value: unknown, credentialFreePrepared = false): unknown {
  const record = object(value, "playback response");
  // These are deliberately structured, short-lived playback grants. They
  // remain validated below; allowing the envelope names here prevents the
  // generic top-level credential guard from confusing a typed playback
  // response with an accidental session/token envelope.
  rejectTopLevelCredentialFields(record, ["continuationCredential"]);
  boundedString(record.sessionId, "playback session id", 512);
  const sourceUrl = playbackUrl(record.sourceUrl, "playback source URL");
  if (typeof record.directPlay !== "boolean") throw new TypeError("playback direct-play state is invalid");
  for (const field of ["generation", "nextEventSequence", "playbackRevision", "queueRevision"] as const) nonNegativeInteger(record[field], `playback ${field}`);
  object(record.decision, "playback decision");
  object(record.media, "playback media");
  mediaGrant(record.mediaGrant);
  if (!credentialFreePrepared) continuationCredential(record.continuationCredential);
  for (const field of ["resources", "audioStreams", "subtitleStreams", "chapters", "queue"] as const) {
    if (!Array.isArray(record[field]) || record[field].length > 10_000) throw new TypeError(`playback ${field} is invalid`);
  }
  const qualityOffers = object(record.qualityOffers, "playback quality offers");
  if (boundedString(qualityOffers.contractId, "playback quality offer contract", 64) !== "PC-PLAYBACK" || boundedString(qualityOffers.schemaVersion, "playback quality offer schema", 64) !== "quality-offers.v1") {
    throw new TypeError("playback quality offer contract is invalid");
  }
  for (const field of ["mediaId", "versionId", "sourceRevision", "offerRevision"] as const) boundedString(qualityOffers[field], `playback quality offer ${field}`, 512);
  if (!Array.isArray(qualityOffers.offers) || qualityOffers.offers.length < 2 || qualityOffers.offers.length > 100) throw new TypeError("playback quality offers are invalid");
  const qualityOfferIDs = new Set<string>();
  let automaticOffers = 0;
  for (const value of qualityOffers.offers) {
    const offer = object(value, "playback quality offer");
    const selectionId = boundedString(offer.selectionId, "playback quality selection id", 512);
    if (qualityOfferIDs.has(selectionId)) throw new TypeError("playback quality selection ids are invalid");
    qualityOfferIDs.add(selectionId);
    boundedString(offer.label, "playback quality offer label", 512);
    const kind = boundedString(offer.kind, "playback quality offer kind", 32, ["automatic", "original", "fixed"]);
    if (kind === "automatic") automaticOffers++;
    const targets = [offer.maxVideoBitrateBps, offer.maxAudioBitrateBps, offer.targetDisplayHeight];
    for (const target of targets) {
      if (target !== undefined && (typeof target !== "number" || !Number.isInteger(target) || target <= 0)) {
        throw new TypeError("playback quality offer target is invalid");
      }
    }
    const concreteTarget = targets.some((target) => target !== undefined);
    if ((kind === "fixed") !== concreteTarget) throw new TypeError("playback quality offer target is invalid");
  }
  if (automaticOffers !== 1) throw new TypeError("playback automatic quality offer is invalid");
  const qualitySelection = object(record.qualitySelection, "playback quality selection");
  const qualityMode = boundedString(qualitySelection.mode, "playback quality selection mode", 32, ["automatic", "explicit"]);
  if (qualityMode === "automatic") {
    if (qualitySelection.selectionId !== undefined || qualitySelection.qualityOfferRevision !== undefined) throw new TypeError("playback automatic quality selection is invalid");
  } else {
    const selectionId = boundedString(qualitySelection.selectionId, "playback selected quality id", 512);
    const revision = boundedString(qualitySelection.qualityOfferRevision, "playback selected quality revision", 512);
    if (!qualityOfferIDs.has(selectionId) || revision !== qualityOffers.offerRevision) throw new TypeError("playback explicit quality selection is stale");
  }
  const resources = record.resources as unknown[];
  if (resources.length !== 1) throw new TypeError("playback resources are invalid");
  const ids = new Set<string>();
  let defaultSource = "";
  let defaults = 0;
  for (const value of resources) {
    const resource = object(value, "playback resource");
    const id = boundedString(resource.id, "playback resource id", 512);
    if (ids.has(id)) throw new TypeError("playback resource ids are invalid");
    ids.add(id);
    const resourceSource = playbackUrl(resource.sourceUrl, "playback resource URL");
    boundedString(resource.streamFormat, "playback resource format", 64);
    if (resource.default === true) {
      defaults++;
      defaultSource = resourceSource;
    } else if (resource.default !== undefined && resource.default !== false) {
      throw new TypeError("playback resource default is invalid");
    }
  }
  if (defaults !== 1 || defaultSource !== sourceUrl) throw new TypeError("playback default resource is invalid");
  const active = object(resources[0], "playback resource");
  for (const [selectionField, resourceField] of [
    ["selectedAudioStreamId", "audioStreamId"],
    ["selectedSubtitleStreamId", "subtitleStreamId"],
    ["selectedSubtitleMode", "subtitleMode"],
  ] as const) {
    if (record[selectionField] !== undefined && record[selectionField] !== active[resourceField]) {
      throw new TypeError(`playback ${selectionField} does not match the active resource`);
    }
  }
  return value;
}

function playbackUrl(value: unknown, name: string): string {
  const candidate = boundedString(value, name, 8_192);
  let parsed: URL;
  try {
    parsed = new URL(candidate, "https://portico.invalid");
  } catch {
    throw new TypeError(`${name} is invalid`);
  }
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.username || parsed.password || parsed.hash) {
    throw new TypeError(`${name} is invalid`);
  }
  for (const key of parsed.searchParams.keys()) {
    if (/(?:authorization|cookie|credential|grant|jwt|password|secret|token)/i.test(key)) throw new TypeError(`${name} is invalid`);
  }
  return candidate;
}

function isNativeCredentialPath(pathname: string): boolean {
  return pathname === "/api/auth/sessions" || pathname === "/api/auth/profile-sessions/native" ||
    pathname === "/api/auth/quick-connect/exchange";
}

function nativeCredentials(value: unknown, api: "server" | "hosted"): unknown {
  const record = object(value, "native session credentials");
  rejectTopLevelCredentialFields(record, ["accessToken", "refreshToken"]);
  boundedString(record.tokenType, "credential token type", 16, "Bearer");
  for (const field of ["accessToken", "refreshToken"] as const) boundedString(record[field], `credential ${field}`, MAX_TOKEN_LENGTH);
  if (api === "server") {
    boundedString(record.authority, "credential authority", 16, ["local", "hosted"]);
    for (const field of ["accountId", "serverId", "profileId", "authorizationRevision"] as const) boundedString(record[field], `credential ${field}`, 512);
  } else {
    boundedString(record.authority, "credential authority", 16, "hosted");
    for (const field of ["accountId", "serverId", "profileId", "authorizationRevision"] as const) {
      if (field in record) throw new TypeError(`Hosted credentials contain server-only ${field}`);
    }
  }
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

function playbackTerminalAcknowledgement(value: unknown): unknown {
  const record = object(value, "playback terminal acknowledgement");
  rejectTopLevelCredentialFields(record);
  const requestId = boundedString(record.requestId, "playback terminal request id", 128);
  if (requestId.length < 8 || !/^[A-Za-z0-9._:-]+$/.test(requestId)) {
    throw new TypeError("playback terminal request id is invalid");
  }
  boundedString(record.sessionId, "playback terminal session id", 512);
  if (record.accepted !== true || typeof record.duplicate !== "boolean") {
    throw new TypeError("playback terminal receipt state is invalid");
  }
  const terminal = object(record.terminal, "playback terminal event");
  boundedString(terminal.disposition, "playback terminal disposition", 16, ["stopped", "completed"]);
  nonNegativeInteger(terminal.generation, "playback terminal generation");
  nonNegativeInteger(terminal.eventSequence, "playback terminal event sequence");
  if (terminal.generation === 0 || terminal.eventSequence === 0) {
    throw new TypeError("playback terminal ordering authority is invalid");
  }
  timestamp(terminal.recordedAt, "playback terminal observation time");
  finiteNumber(terminal.positionSeconds, "playback terminal position", 0);
  finiteNumber(terminal.durationSeconds, "playback terminal duration", 0);
  if (terminal.disposition === "completed" && terminal.durationSeconds === 0) {
    throw new TypeError("completed playback terminal duration is invalid");
  }
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
