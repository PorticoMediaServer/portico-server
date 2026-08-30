import {
  normalizeViewerScope,
  sameViewerScope,
  type ViewerScope
} from "./viewerScope.js";
import type { ProductContract } from "./types.js";

/**
 * Framework-neutral Portico destination and linking contract.
 *
 * React Navigation, React Router, Roku SceneGraph, and third-party clients may
 * translate this vocabulary into their own route state. This module must never
 * depend on a router implementation or carry transport/runtime resources.
 */
export const PORTICO_NAVIGATION_CONTRACT_REVISION = "v1" as const;

export type PorticoPlatformClass = "handheld" | "television" | "web" | "roku";
export type PorticoPrimaryDestinationId = "home" | "library" | "channels" | "saved" | "downloads";
export type PorticoMediaDetailKind = "live-channel" | "live-program";
export type PorticoPlayerContext = "vod" | "live" | "dvr" | "library-channel" | "watch-with-friends" | "offline";

export type PorticoDestination =
  | {destination: "home"}
  | {destination: "library"; libraryId?: string; pivot?: string}
  | {destination: "channels"; tab?: string}
  | {destination: "saved"; tab?: string}
  | {destination: "downloads"}
  | {destination: "search"; query?: string}
  | {destination: "settings"; section?: string}
  | {destination: "person"; personId: string}
  | {destination: "media-detail"; mediaId: string; seasonId?: string; episodeId?: string; mediaKind?: PorticoMediaDetailKind}
  | {destination: "notifications"}
  | {destination: "watch-with-friends"; groupId?: string}
  | {
      destination: "player";
      mediaId: string;
      context?: PorticoPlayerContext;
      watchWithFriendsGroupId?: string;
      /** Stable identifier only. The download runtime resolves and validates its current local resource. */
      localDownloadId?: string;
    };

export type PorticoPrimaryDestination = Extract<PorticoDestination, {destination: PorticoPrimaryDestinationId}>;

export type PorticoDestinationCapabilities = {
  downloads?: boolean;
  liveTV?: boolean;
  notifications?: boolean;
  watchWithFriends?: boolean;
};

/**
 * Bounded authoritative composite of the Product Contract revisions that can
 * alter actions, copy-backed route meaning, search destinations, or event
 * invalidation. Route-affecting feature booleans have their own capability
 * fence. Length prefixes make the composite unambiguous without hashing.
 */
export function porticoProductContractRevision(
  contract: Pick<ProductContract,
    "apiVersion" |
    "actionRevision" |
    "language" |
    "search" |
    "applicationEvents"
  >,
): string {
  const revisions = [
    contract.apiVersion,
    contract.actionRevision,
    contract.language.revision,
    contract.search.revision,
    contract.applicationEvents.revision,
  ].map(value => boundedText(value, 64));
  if (revisions.some(value => !value)) throw new TypeError("Portico Product Contract revision is invalid");
  const composite = `pc1|${revisions.map(value => `${value!.length}:${value}`).join("|")}`;
  if (composite.length > 128) throw new TypeError("Portico Product Contract revision is too long");
  return composite;
}

/** Stable, non-secret fence for route-affecting platform capabilities. */
export function porticoDestinationCapabilityRevision(
  platform: PorticoPlatformClass,
  capabilities: PorticoDestinationCapabilities,
): string {
  const value = (candidate: boolean | undefined) => candidate === undefined ? "unknown" : candidate ? "yes" : "no";
  return [
    "destination-capabilities-v1",
    platform,
    `downloads=${value(capabilities.downloads)}`,
    `liveTV=${value(capabilities.liveTV)}`,
    `notifications=${value(capabilities.notifications)}`,
    `watchWithFriends=${value(capabilities.watchWithFriends)}`
  ].join(":");
}

export type PorticoNavigationPolicy = {
  history: "root" | "replace" | "push" | "focus-or-replace";
  identity: string;
};

const destinationNames = new Set<PorticoDestination["destination"]>([
  "home", "library", "channels", "saved", "downloads", "search", "settings",
  "person", "media-detail", "notifications", "watch-with-friends", "player"
]);
const mediaKinds = new Set<PorticoMediaDetailKind>(["live-channel", "live-program"]);
const playerContexts = new Set<PorticoPlayerContext>(["vod", "live", "dvr", "library-channel", "watch-with-friends", "offline"]);
const forbiddenParameterNames = /(?:token|credential|password|secret|grant|callback|handle|response|queryclient|sourceurl|localurl|filepath|fileurl)/i;

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function exactKeys(value: Record<string, unknown>, allowed: readonly string[]): boolean {
  const allowedSet = new Set(allowed);
  return Object.keys(value).every(key => allowedSet.has(key) && !forbiddenParameterNames.test(key));
}

function boundedText(value: unknown, maxLength = 256): string | undefined {
  if (typeof value !== "string") return undefined;
  const normalized = value.trim();
  if (!normalized || normalized.length > maxLength || /[\u0000-\u001f\u007f]/.test(normalized)) return undefined;
  return normalized;
}

function optionalText(value: unknown, maxLength = 256): string | undefined | null {
  if (value === undefined) return undefined;
  return boundedText(value, maxLength) ?? null;
}

function optionalEnum<T extends string>(value: unknown, allowed: ReadonlySet<T>): T | undefined | null {
  if (value === undefined) return undefined;
  return typeof value === "string" && allowed.has(value as T) ? value as T : null;
}

/**
 * Strictly validates an untrusted destination. Unknown fields are rejected so
 * credentials, raw URLs, callbacks, or native handles cannot hitchhike inside
 * navigation state.
 */
export function normalizePorticoDestination(value: unknown): PorticoDestination | undefined {
  if (!isPlainRecord(value) || typeof value.destination !== "string" || !destinationNames.has(value.destination as PorticoDestination["destination"])) return undefined;
  switch (value.destination) {
    case "home":
    case "downloads":
    case "notifications":
      return exactKeys(value, ["destination"]) ? {destination: value.destination} : undefined;
    case "library": {
      if (!exactKeys(value, ["destination", "libraryId", "pivot"])) return undefined;
      const libraryId = optionalText(value.libraryId);
      const pivot = optionalText(value.pivot, 80);
      if (libraryId === null || pivot === null) return undefined;
      return {destination: "library", ...(libraryId ? {libraryId} : {}), ...(pivot ? {pivot} : {})};
    }
    case "channels":
    case "saved": {
      if (!exactKeys(value, ["destination", "tab"])) return undefined;
      const tab = optionalText(value.tab, 80);
      if (tab === null) return undefined;
      return {destination: value.destination, ...(tab ? {tab} : {})};
    }
    case "search": {
      if (!exactKeys(value, ["destination", "query"])) return undefined;
      const query = optionalText(value.query, 256);
      if (query === null) return undefined;
      return {destination: "search", ...(query ? {query} : {})};
    }
    case "settings": {
      if (!exactKeys(value, ["destination", "section"])) return undefined;
      const section = optionalText(value.section, 120);
      if (section === null) return undefined;
      return {destination: "settings", ...(section ? {section} : {})};
    }
    case "person": {
      if (!exactKeys(value, ["destination", "personId"])) return undefined;
      const personId = boundedText(value.personId);
      return personId ? {destination: "person", personId} : undefined;
    }
    case "media-detail": {
      if (!exactKeys(value, ["destination", "mediaId", "seasonId", "episodeId", "mediaKind"])) return undefined;
      const mediaId = boundedText(value.mediaId);
      const seasonId = optionalText(value.seasonId);
      const episodeId = optionalText(value.episodeId);
      const mediaKind = optionalEnum(value.mediaKind, mediaKinds);
      if (!mediaId || seasonId === null || episodeId === null || mediaKind === null) return undefined;
      return {destination: "media-detail", mediaId, ...(seasonId ? {seasonId} : {}), ...(episodeId ? {episodeId} : {}), ...(mediaKind ? {mediaKind} : {})};
    }
    case "watch-with-friends": {
      if (!exactKeys(value, ["destination", "groupId"])) return undefined;
      const groupId = optionalText(value.groupId);
      if (groupId === null) return undefined;
      return {destination: "watch-with-friends", ...(groupId ? {groupId} : {})};
    }
    case "player": {
      if (!exactKeys(value, ["destination", "mediaId", "context", "watchWithFriendsGroupId", "localDownloadId"])) return undefined;
      const mediaId = boundedText(value.mediaId);
      const context = optionalEnum(value.context, playerContexts);
      const watchWithFriendsGroupId = optionalText(value.watchWithFriendsGroupId);
      const localDownloadId = optionalText(value.localDownloadId);
      if (!mediaId || context === null || watchWithFriendsGroupId === null || localDownloadId === null) return undefined;
      if (context === "watch-with-friends" && !watchWithFriendsGroupId) return undefined;
      if (context === "offline" && !localDownloadId) return undefined;
      if (watchWithFriendsGroupId && context !== "watch-with-friends") return undefined;
      if (localDownloadId && context !== "offline") return undefined;
      return {destination: "player", mediaId, ...(context ? {context} : {}), ...(watchWithFriendsGroupId ? {watchWithFriendsGroupId} : {}), ...(localDownloadId ? {localDownloadId} : {})};
    }
    default:
      return undefined;
  }
}

export function porticoDestinationIsPrimary(destination: PorticoDestination): destination is PorticoPrimaryDestination {
  return destination.destination === "home" || destination.destination === "library" || destination.destination === "channels" || destination.destination === "saved" || destination.destination === "downloads";
}

/** Semantic identity excludes replace-style state such as pivots, tabs, and submitted search text. */
export function porticoDestinationIdentity(value: PorticoDestination): string {
  const destination = normalizePorticoDestination(value);
  if (!destination) throw new TypeError("Portico destination is invalid");
  switch (destination.destination) {
    case "library": return `library:${destination.libraryId ?? "root"}`;
    case "person": return `person:${destination.personId}`;
    case "media-detail": return `media-detail:${destination.mediaId}`;
    case "watch-with-friends": return `watch-with-friends:${destination.groupId ?? "root"}`;
    case "player": return `player:${destination.mediaId}:${destination.context ?? "vod"}:${destination.watchWithFriendsGroupId ?? ""}:${destination.localDownloadId ?? ""}`;
    default: return destination.destination;
  }
}

export function porticoNavigationPolicy(value: PorticoDestination): PorticoNavigationPolicy {
  const destination = normalizePorticoDestination(value);
  if (!destination) throw new TypeError("Portico destination is invalid");
  const identity = porticoDestinationIdentity(destination);
  if (porticoDestinationIsPrimary(destination)) return {history: "root", identity};
  if (destination.destination === "search" || destination.destination === "settings" || destination.destination === "notifications") {
    return {history: "focus-or-replace", identity};
  }
  return {history: "push", identity};
}

export function porticoDestinationIsAvailable(
  value: PorticoDestination,
  platform: PorticoPlatformClass,
  capabilities: PorticoDestinationCapabilities = {}
): boolean {
  const destination = normalizePorticoDestination(value);
  if (!destination) return false;
  if (destination.destination === "downloads") return platform === "handheld" && capabilities.downloads !== false;
  if (destination.destination === "player" && destination.context === "offline") {
    return platform === "handheld" && capabilities.downloads !== false;
  }
  if (destination.destination === "channels" || (destination.destination === "player" && (destination.context === "live" || destination.context === "dvr" || destination.context === "library-channel"))) {
    return capabilities.liveTV !== false;
  }
  if (destination.destination === "notifications") return capabilities.notifications !== false;
  if (destination.destination === "watch-with-friends" || (destination.destination === "player" && destination.context === "watch-with-friends")) {
    return capabilities.watchWithFriends !== false;
  }
  return true;
}

function appendQuery(path: string, entries: readonly [string, string | undefined][]): string {
  const query = new URLSearchParams();
  for (const [key, value] of entries) if (value) query.set(key, value);
  const encoded = query.toString();
  return encoded ? `${path}?${encoded}` : path;
}

function destinationPath(destination: PorticoDestination): string {
  const segment = (value: string) => encodeURIComponent(value);
  switch (destination.destination) {
    case "home": return "";
    case "library": return appendQuery(destination.libraryId ? `library/${segment(destination.libraryId)}` : "library", [["pivot", destination.pivot]]);
    case "channels": return appendQuery("channels", [["tab", destination.tab]]);
    case "saved": return appendQuery("saved", [["tab", destination.tab]]);
    case "downloads": return "downloads";
    case "search": return appendQuery("search", [["q", destination.query]]);
    case "settings": return destination.section ? `settings/${segment(destination.section)}` : "settings";
    case "person": return `person/${segment(destination.personId)}`;
    case "media-detail": return appendQuery(`media/${segment(destination.mediaId)}`, [["season", destination.seasonId], ["episode", destination.episodeId], ["kind", destination.mediaKind]]);
    case "notifications": return "notifications";
    case "watch-with-friends": return destination.groupId ? `watch-with-friends/${segment(destination.groupId)}` : "watch-with-friends";
    case "player": return appendQuery(`play/${segment(destination.mediaId)}`, [["context", destination.context], ["group", destination.watchWithFriendsGroupId], ["download", destination.localDownloadId]]);
  }
}

/** Serializes to the custom scheme by default, or beneath an HTTPS application base URL. */
export function serializePorticoLink(value: PorticoDestination, baseURL = "portico://"): string {
  const destination = normalizePorticoDestination(value);
  if (!destination) throw new TypeError("Portico destination is invalid");
  const path = destinationPath(destination);
  if (baseURL.toLowerCase() === "portico://") return `portico://${path}`;
  const base = new URL(baseURL);
  if (base.protocol !== "https:" && base.protocol !== "http:") throw new TypeError("Portico link base URL must use HTTP or HTTPS");
  if (destination.destination === "home") {
    base.pathname = "/";
    base.search = "";
    base.hash = "";
    return base.toString();
  }
  const root = base.pathname.replace(/\/+$/, "");
  base.pathname = `${root}/${path.split("?")[0]}`.replace(/\/{2,}/g, "/");
  base.search = path.includes("?") ? path.slice(path.indexOf("?")) : "";
  base.hash = "";
  return base.toString();
}

function decodeSegment(value: string | undefined): string | undefined {
  if (!value) return undefined;
  try { return decodeURIComponent(value); } catch { return undefined; }
}

export type ParsePorticoLinkOptions = {
  /** Optional allowlist for callers that parse ordinary web URLs outside OS-level universal-link validation. */
  allowedWebHosts?: readonly string[];
};

export function parsePorticoLink(value: string, options: ParsePorticoLinkOptions = {}): PorticoDestination | undefined {
  try {
    const url = new URL(value);
    if (url.username || url.password || url.hash) return undefined;
    let segments: string[];
    if (url.protocol === "portico:") {
      segments = [url.hostname, ...url.pathname.split("/")].filter(Boolean);
    } else if (url.protocol === "https:" || url.protocol === "http:") {
      if (options.allowedWebHosts?.length && (url.port || !options.allowedWebHosts.some(host => host.toLowerCase() === url.hostname.toLowerCase()))) return undefined;
      segments = url.pathname.split("/").filter(Boolean);
      // A hosted application may be mounted beneath a base path. The
      // serializer preserves that base, while the grammar begins at the first
      // canonical destination segment.
      const routeIndex = segments.findIndex(segment => {
        const name = segment.toLowerCase();
        return name === "media" || name === "play" || destinationNames.has(name as PorticoDestination["destination"]);
      });
      if (routeIndex > 0) segments = segments.slice(routeIndex);
    } else return undefined;
    const first = segments[0]?.toLowerCase();
    const allowedQueryKeys: Record<string, readonly string[]> = {
      library: ["pivot"], channels: ["tab"], saved: ["tab"], search: ["q"],
      media: ["season", "episode", "kind"], play: ["context", "group", "download"]
    };
    const maximumSegments: Record<string, number> = {
      library: 2, channels: 1, saved: 1, downloads: 1, search: 1,
      settings: 2, person: 2, media: 2, notifications: 1,
      "watch-with-friends": 2, play: 2
    };
    if (first) {
      if (segments.length > (maximumSegments[first] ?? 0)) return undefined;
      const allowed = new Set(allowedQueryKeys[first] ?? []);
      if ([...url.searchParams.keys()].some(key => !allowed.has(key))) return undefined;
    } else if ([...url.searchParams.keys()].length) return undefined;
    const second = decodeSegment(segments[1]);
    let candidate: unknown;
    if (!first) candidate = {destination: "home"};
    else if (first === "library") candidate = {destination: "library", ...(second ? {libraryId: second} : {}), ...(url.searchParams.get("pivot") ? {pivot: url.searchParams.get("pivot")} : {})};
    else if (first === "channels") candidate = {destination: "channels", ...(url.searchParams.get("tab") ? {tab: url.searchParams.get("tab")} : {})};
    else if (first === "saved") candidate = {destination: "saved", ...(url.searchParams.get("tab") ? {tab: url.searchParams.get("tab")} : {})};
    else if (first === "downloads") candidate = {destination: "downloads"};
    else if (first === "search") candidate = {destination: "search", ...(url.searchParams.get("q") ? {query: url.searchParams.get("q")} : {})};
    else if (first === "settings") candidate = {destination: "settings", ...(second ? {section: second} : {})};
    else if (first === "person" && second) candidate = {destination: "person", personId: second};
    else if (first === "media" && second) candidate = {destination: "media-detail", mediaId: second, ...(url.searchParams.get("season") ? {seasonId: url.searchParams.get("season")} : {}), ...(url.searchParams.get("episode") ? {episodeId: url.searchParams.get("episode")} : {}), ...(url.searchParams.get("kind") ? {mediaKind: url.searchParams.get("kind")} : {})};
    else if (first === "notifications") candidate = {destination: "notifications"};
    else if (first === "watch-with-friends") candidate = {destination: "watch-with-friends", ...(second ? {groupId: second} : {})};
    else if (first === "play" && second) candidate = {destination: "player", mediaId: second, ...(url.searchParams.get("context") ? {context: url.searchParams.get("context")} : {}), ...(url.searchParams.get("group") ? {watchWithFriendsGroupId: url.searchParams.get("group")} : {}), ...(url.searchParams.get("download") ? {localDownloadId: url.searchParams.get("download")} : {})};
    return normalizePorticoDestination(candidate);
  } catch {
    return undefined;
  }
}

export type PorticoNavigationRestorationFence = ViewerScope & {
  productContractRevision: string;
  routeContractRevision: typeof PORTICO_NAVIGATION_CONTRACT_REVISION;
  platform: PorticoPlatformClass;
  capabilityRevision: string;
};

export type PorticoNavigationRestoration = {
  version: "v1";
  savedAt: string;
  fence: PorticoNavigationRestorationFence;
  destination: PorticoPrimaryDestination;
};

function normalizedFence(value: unknown): PorticoNavigationRestorationFence | undefined {
  if (!isPlainRecord(value) || !exactKeys(value, ["authority", "accountId", "serverId", "profileId", "authorizationRevision", "productContractRevision", "routeContractRevision", "platform", "capabilityRevision"])) return undefined;
  try {
    const scope = normalizeViewerScope(value as unknown as ViewerScope);
    const productContractRevision = boundedText(value.productContractRevision, 128);
    const capabilityRevision = boundedText(value.capabilityRevision, 128);
    const platform = value.platform;
    if (!productContractRevision || !capabilityRevision || value.routeContractRevision !== PORTICO_NAVIGATION_CONTRACT_REVISION || (platform !== "handheld" && platform !== "television" && platform !== "web" && platform !== "roku")) return undefined;
    return {...scope, productContractRevision, routeContractRevision: PORTICO_NAVIGATION_CONTRACT_REVISION, platform, capabilityRevision};
  } catch { return undefined; }
}

export function porticoNavigationRestorationFence(input: Omit<PorticoNavigationRestorationFence, "routeContractRevision"> & {routeContractRevision?: typeof PORTICO_NAVIGATION_CONTRACT_REVISION}): PorticoNavigationRestorationFence {
  const fence = normalizedFence({...input, routeContractRevision: input.routeContractRevision ?? PORTICO_NAVIGATION_CONTRACT_REVISION});
  if (!fence) throw new TypeError("Portico navigation restoration fence is invalid");
  return fence;
}

function sameFence(left: PorticoNavigationRestorationFence, right: PorticoNavigationRestorationFence): boolean {
  return sameViewerScope(left, right)
    && left.productContractRevision === right.productContractRevision
    && left.routeContractRevision === right.routeContractRevision
    && left.platform === right.platform
    && left.capabilityRevision === right.capabilityRevision;
}

export function createPorticoNavigationRestoration(
  fenceInput: PorticoNavigationRestorationFence,
  destinationInput: PorticoPrimaryDestination,
  now = new Date()
): PorticoNavigationRestoration {
  const fence = normalizedFence(fenceInput);
  const destination = normalizePorticoDestination(destinationInput);
  if (!fence || !destination || !porticoDestinationIsPrimary(destination) || Number.isNaN(now.getTime())) throw new TypeError("Portico navigation restoration input is invalid");
  return {version: "v1", savedAt: now.toISOString(), fence, destination};
}

export type RestorePorticoNavigationOptions = {
  now?: Date;
  maxAgeMs?: number;
  capabilities?: PorticoDestinationCapabilities;
};

/** Returns no destination on any mismatch; restoration never becomes a user-facing error. */
export function restorePorticoNavigation(
  value: unknown,
  expectedFenceInput: PorticoNavigationRestorationFence,
  options: RestorePorticoNavigationOptions = {}
): PorticoPrimaryDestination | undefined {
  if (!isPlainRecord(value) || !exactKeys(value, ["version", "savedAt", "fence", "destination"]) || value.version !== "v1" || typeof value.savedAt !== "string") return undefined;
  const fence = normalizedFence(value.fence);
  const expectedFence = normalizedFence(expectedFenceInput);
  const destination = normalizePorticoDestination(value.destination);
  if (!fence || !expectedFence || !sameFence(fence, expectedFence) || !destination || !porticoDestinationIsPrimary(destination)) return undefined;
  const savedAt = Date.parse(value.savedAt);
  const now = (options.now ?? new Date()).getTime();
  const maxAgeMs = options.maxAgeMs ?? 90 * 24 * 60 * 60 * 1000;
  if (!Number.isFinite(savedAt) || !Number.isFinite(now) || !Number.isFinite(maxAgeMs) || maxAgeMs < 0 || savedAt > now + 5 * 60 * 1000 || now - savedAt > maxAgeMs) return undefined;
  return porticoDestinationIsAvailable(destination, expectedFence.platform, options.capabilities) ? destination : undefined;
}

export type PorticoPendingDestinationIntent = {
  version: "v1";
  destination: PorticoDestination;
  createdAt: string;
  expiresAt: string;
  expectedIdentity?: Pick<ViewerScope, "authority" | "accountId" | "serverId" | "profileId">;
};

export function createPorticoPendingDestinationIntent(
  destinationInput: PorticoDestination,
  options: {now?: Date; ttlMs?: number; expectedIdentity?: PorticoPendingDestinationIntent["expectedIdentity"]} = {}
): PorticoPendingDestinationIntent {
  const destination = normalizePorticoDestination(destinationInput);
  const now = options.now ?? new Date();
  const ttlMs = options.ttlMs ?? 10 * 60 * 1000;
  if (!destination || !Number.isFinite(now.getTime()) || !Number.isFinite(ttlMs) || ttlMs <= 0 || ttlMs > 24 * 60 * 60 * 1000) throw new TypeError("Portico pending destination intent is invalid");
  let expectedIdentity: PorticoPendingDestinationIntent["expectedIdentity"];
  if (options.expectedIdentity) {
    try {
      const scope = normalizeViewerScope({...options.expectedIdentity, authorizationRevision: "pending-intent"});
      expectedIdentity = {authority: scope.authority, accountId: scope.accountId, serverId: scope.serverId, profileId: scope.profileId};
    } catch { throw new TypeError("Portico pending destination identity is invalid"); }
  }
  return {version: "v1", destination, createdAt: now.toISOString(), expiresAt: new Date(now.getTime() + ttlMs).toISOString(), ...(expectedIdentity ? {expectedIdentity} : {})};
}

export function consumePorticoPendingDestinationIntent(
  value: unknown,
  scopeInput: ViewerScope,
  platform: PorticoPlatformClass,
  options: {now?: Date; capabilities?: PorticoDestinationCapabilities; authorize(destination: PorticoDestination): boolean}
): PorticoDestination | undefined {
  if (!isPlainRecord(value) || !exactKeys(value, ["version", "destination", "createdAt", "expiresAt", "expectedIdentity"]) || value.version !== "v1" || typeof value.createdAt !== "string" || typeof value.expiresAt !== "string") return undefined;
  const destination = normalizePorticoDestination(value.destination);
  let scope: ViewerScope;
  try { scope = normalizeViewerScope(scopeInput); } catch { return undefined; }
  const now = (options.now ?? new Date()).getTime();
  const createdAt = Date.parse(value.createdAt);
  const expiresAt = Date.parse(value.expiresAt);
  if (!destination || !Number.isFinite(now) || !Number.isFinite(createdAt) || !Number.isFinite(expiresAt) || createdAt > now + 5 * 60 * 1000 || expiresAt <= now || expiresAt <= createdAt || expiresAt - createdAt > 24 * 60 * 60 * 1000) return undefined;
  if (value.expectedIdentity !== undefined) {
    if (!isPlainRecord(value.expectedIdentity) || !exactKeys(value.expectedIdentity, ["authority", "accountId", "serverId", "profileId"])) return undefined;
    const expected = value.expectedIdentity;
    if (expected.authority !== scope.authority || expected.accountId !== scope.accountId || expected.serverId !== scope.serverId || expected.profileId !== scope.profileId) return undefined;
  }
  if (!porticoDestinationIsAvailable(destination, platform, options.capabilities) || !options.authorize(destination)) return undefined;
  return destination;
}
