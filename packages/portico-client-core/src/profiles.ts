/** Canonical account/profile identity models shared by all Portico clients. */

export const PROFILE_POLICY_VERSION = "v1" as const;
export const AUTOMATIC_PROFILE_TRUST_VERSION = "v1" as const;
export const PROFILE_BLOCKED_LABEL_LIMIT = 64;
export const PROFILE_BLOCKED_LABEL_MAX_CODE_POINTS = 128;
export const PROFILE_LIMIT = 8;

export type ProfileAuthority = "hosted" | "local";
export type ProfileFeature = "downloads" | "liveTV" | "dvr" | "watchWithFriends" | "feedback";

/** Exact portable V1 wire contract shared with Portico Cloud and Server. */
export type ProfilePolicy = {
  version: typeof PROFILE_POLICY_VERSION;
  maximumAgeRating: number | null;
  allowUnrated: boolean;
  blockedLabels: string[];
  allowDownloads: boolean;
  allowLiveTV: boolean;
  allowDvr: boolean;
  allowWatchWithFriends: boolean;
  allowFeedback: boolean;
};

export type PorticoProfile = {
  id: string;
  name: string;
  avatar?: {
    kind: "preset" | "custom";
    reference: string;
  };
  isPrimary: boolean;
  isAccountAdmin: boolean;
  hasPIN: boolean;
  pinRevision: number;
  sortOrder: number;
  policy: ProfilePolicy;
};

export type ProfileDirectory = {
  authority: ProfileAuthority;
  accountId: string;
  serverId: string;
  profilesAllowed: boolean;
  profiles: PorticoProfile[];
};

export type ProfileSelectionGrant = {
  token: string;
  authority: ProfileAuthority;
  accountId: string;
  serverId: string;
  profileId: string;
  pinRevision: number;
  installationId?: string;
  expiresAt: string;
};

/**
 * Revocable authority-issued permission to open one remembered profile automatically.
 * This is deliberately not a one-time `ProfileSelectionGrant`: persisting or treating a
 * one-time selection grant as remembered-profile trust would silently weaken the profile boundary.
 */
export type AutomaticProfileTrust = {
  version: typeof AUTOMATIC_PROFILE_TRUST_VERSION;
  purpose: "automatic-profile-selection";
  token: string;
  authority: ProfileAuthority;
  accountId: string;
  serverId: string;
  profileId: string;
  pinRevision: number;
  installationId?: string;
  expiresAt: string;
};

export type ProfileSelectionDecision =
  | { kind: "unavailable" }
  | { kind: "select"; profiles: PorticoProfile[] }
  | { kind: "pin"; profile: PorticoProfile }
  | { kind: "open"; profile: PorticoProfile };

export type EffectiveProfilePolicy = ProfilePolicy;

export const unrestrictedProfilePolicy: Readonly<ProfilePolicy> = deepFreezePolicy({
  version: PROFILE_POLICY_VERSION,
  maximumAgeRating: null,
  allowUnrated: true,
  blockedLabels: [],
  allowDownloads: true,
  allowLiveTV: true,
  allowDvr: true,
  allowWatchWithFriends: true,
  allowFeedback: true
});

function deepFreezePolicy(value: ProfilePolicy): Readonly<ProfilePolicy> {
  Object.freeze(value.blockedLabels);
  return Object.freeze(value);
}

function recordValue(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${name} must be an object`);
  return value as Record<string, unknown>;
}

function exactKeys(source: Record<string, unknown>, expected: readonly string[], name: string): void {
  const keys = Object.keys(source);
  const allowed = new Set(expected);
  if (keys.length !== expected.length || keys.some(key => !allowed.has(key))) throw new TypeError(`${name} has missing or unknown fields`);
}

function exactKeysWithOptional(source: Record<string, unknown>, required: readonly string[], optional: readonly string[], name: string): void {
  const keys = Object.keys(source);
  const allowed = new Set([...required, ...optional]);
  if (required.some(key => !Object.prototype.hasOwnProperty.call(source, key)) || keys.some(key => !allowed.has(key))) {
    throw new TypeError(`${name} has missing or unknown fields`);
  }
}

function codePointLength(value: string): number {
  return Array.from(value).length;
}

function compareCodePoints(left: string, right: string): number {
  const a = Array.from(left.toLowerCase(), character => character.codePointAt(0)!);
  const b = Array.from(right.toLowerCase(), character => character.codePointAt(0)!);
  for (let index = 0; index < Math.min(a.length, b.length); index += 1) {
    if (a[index] !== b[index]) return a[index] - b[index];
  }
  return a.length - b.length;
}

function boundedIdentity(value: unknown, name: string, maximum = 128): string {
  if (typeof value !== "string") throw new TypeError(`${name} must be a string`);
  const normalized = value.trim();
  if (!normalized || codePointLength(normalized) > maximum) throw new TypeError(`${name} is invalid`);
  return normalized;
}

function policyLabels(value: unknown): string[] {
  if (!Array.isArray(value) || value.length > PROFILE_BLOCKED_LABEL_LIMIT) throw new TypeError("invalid blocked labels");
  const labels: string[] = [];
  const seen = new Set<string>();
  for (const entry of value) {
    if (typeof entry !== "string") throw new TypeError("invalid blocked labels");
    const label = entry.trim().normalize("NFC");
    const key = label.toLowerCase();
    if (!label || codePointLength(label) > PROFILE_BLOCKED_LABEL_MAX_CODE_POINTS || seen.has(key)) throw new TypeError("invalid blocked labels");
    seen.add(key);
    labels.push(label);
  }
  return labels.sort(compareCodePoints);
}

function policyAge(value: unknown): number | null {
  if (value === null) return null;
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0 || value > 21) {
    throw new TypeError("maximumAgeRating must be null or an integer from 0 through 21");
  }
  return value;
}

/** Strictly parses the exact portable V1 wire contract. */
export function parseProfilePolicy(value: unknown): ProfilePolicy {
  const source = recordValue(value, "profile policy");
  exactKeys(source, [
    "version", "maximumAgeRating", "allowUnrated", "blockedLabels", "allowDownloads",
    "allowLiveTV", "allowDvr", "allowWatchWithFriends", "allowFeedback"
  ], "profile policy");
  if (source.version !== PROFILE_POLICY_VERSION) throw new TypeError("unsupported profile policy");
  for (const field of ["allowUnrated", "allowDownloads", "allowLiveTV", "allowDvr", "allowWatchWithFriends", "allowFeedback"] as const) {
    if (typeof source[field] !== "boolean") throw new TypeError(`profile policy ${field} must be a boolean`);
  }
  return {
    version: PROFILE_POLICY_VERSION,
    maximumAgeRating: policyAge(source.maximumAgeRating),
    allowUnrated: source.allowUnrated as boolean,
    blockedLabels: policyLabels(source.blockedLabels),
    allowDownloads: source.allowDownloads as boolean,
    allowLiveTV: source.allowLiveTV as boolean,
    allowDvr: source.allowDvr as boolean,
    allowWatchWithFriends: source.allowWatchWithFriends as boolean,
    allowFeedback: source.allowFeedback as boolean
  };
}

function stricterAge(left: number | null, right: number | null): number | null {
  if (left === null) return right;
  if (right === null) return left;
  return Math.min(left, right);
}

/** Membership is the ceiling; profile policy can only subtract from it. */
export function intersectProfilePolicy(membership: ProfilePolicy, profile: ProfilePolicy): EffectiveProfilePolicy {
  const upper = parseProfilePolicy(membership);
  const lower = parseProfilePolicy(profile);
  const combinedLabels = [...upper.blockedLabels, ...lower.blockedLabels].filter((label, index, all) =>
    all.findIndex(candidate => candidate.toLowerCase() === label.toLowerCase()) === index);
  return {
    version: PROFILE_POLICY_VERSION,
    maximumAgeRating: stricterAge(upper.maximumAgeRating, lower.maximumAgeRating),
    allowUnrated: upper.allowUnrated && lower.allowUnrated,
    blockedLabels: policyLabels(combinedLabels),
    allowDownloads: upper.allowDownloads && lower.allowDownloads,
    allowLiveTV: upper.allowLiveTV && lower.allowLiveTV,
    allowDvr: upper.allowDvr && lower.allowDvr,
    allowWatchWithFriends: upper.allowWatchWithFriends && lower.allowWatchWithFriends,
    allowFeedback: upper.allowFeedback && lower.allowFeedback
  };
}

export function profilePolicyAllowsFeature(policy: ProfilePolicy, feature: ProfileFeature): boolean {
  const parsed = parseProfilePolicy(policy);
  switch (feature) {
    case "downloads": return parsed.allowDownloads;
    case "liveTV": return parsed.allowLiveTV;
    case "dvr": return parsed.allowDvr;
    case "watchWithFriends": return parsed.allowWatchWithFriends;
    case "feedback": return parsed.allowFeedback;
  }
}

export function profilePolicyAllowsRating(policy: ProfilePolicy, normalizedAgeRating: number | null, labels: readonly string[] = []): boolean {
  const parsed = parseProfilePolicy(policy);
  if (normalizedAgeRating !== null && (!Number.isInteger(normalizedAgeRating) || normalizedAgeRating < 0 || normalizedAgeRating > 21)) return false;
  const blocked = new Set(parsed.blockedLabels.map(label => label.toLowerCase()));
  for (const raw of labels) {
    if (typeof raw !== "string") return false;
    const label = raw.trim().normalize("NFC");
    if (!label || codePointLength(label) > PROFILE_BLOCKED_LABEL_MAX_CODE_POINTS || blocked.has(label.toLowerCase())) return false;
  }
  if (normalizedAgeRating === null) return parsed.allowUnrated;
  return parsed.maximumAgeRating === null || normalizedAgeRating <= parsed.maximumAgeRating;
}

export function parsePorticoProfile(value: unknown): PorticoProfile {
  const source = recordValue(value, "profile");
  const allowed = new Set(["id", "name", "avatar", "isPrimary", "isAccountAdmin", "hasPIN", "pinRevision", "sortOrder", "policy"]);
  if (Object.keys(source).some(key => !allowed.has(key))) throw new TypeError("profile has unknown fields");
  for (const field of ["isPrimary", "isAccountAdmin", "hasPIN"] as const) {
    if (typeof source[field] !== "boolean") throw new TypeError(`profile ${field} must be a boolean`);
  }
  if (source.isPrimary !== source.isAccountAdmin) throw new TypeError("primary and account-admin profile state must match");
  if (!Number.isSafeInteger(source.pinRevision) || (source.pinRevision as number) < 0) throw new TypeError("profile pinRevision is invalid");
  if (!Number.isSafeInteger(source.sortOrder) || (source.sortOrder as number) < 0) throw new TypeError("profile sortOrder is invalid");
  let avatar: PorticoProfile["avatar"];
  if (source.avatar !== undefined && source.avatar !== null) {
    const raw = recordValue(source.avatar, "profile avatar");
    exactKeys(raw, ["kind", "reference"], "profile avatar");
    if (raw.kind !== "preset" && raw.kind !== "custom") throw new TypeError("profile avatar kind is invalid");
    avatar = { kind: raw.kind, reference: boundedIdentity(raw.reference, "profile avatar reference", 256) };
  }
  return {
    id: boundedIdentity(source.id, "profile id"),
    name: boundedIdentity(source.name, "profile name", 80),
    avatar,
    isPrimary: source.isPrimary as boolean,
    isAccountAdmin: source.isAccountAdmin as boolean,
    hasPIN: source.hasPIN as boolean,
    pinRevision: source.pinRevision as number,
    sortOrder: source.sortOrder as number,
    policy: parseProfilePolicy(source.policy)
  };
}

export function parseProfileDirectory(value: unknown): ProfileDirectory {
  const source = recordValue(value, "profile directory");
  exactKeys(source, ["authority", "accountId", "serverId", "profilesAllowed", "profiles"], "profile directory");
  if (source.authority !== "hosted" && source.authority !== "local") throw new TypeError("profile authority is invalid");
  if (typeof source.profilesAllowed !== "boolean" || !Array.isArray(source.profiles) || source.profiles.length < 1 || source.profiles.length > PROFILE_LIMIT) {
    throw new TypeError("profile directory is invalid");
  }
  const profiles = source.profiles.map(parsePorticoProfile);
  if (new Set(profiles.map(profile => profile.id)).size !== profiles.length || profiles.filter(profile => profile.isPrimary).length !== 1) {
    throw new TypeError("profile directory identities are invalid");
  }
  return {
    authority: source.authority,
    accountId: boundedIdentity(source.accountId, "account id"),
    serverId: boundedIdentity(source.serverId, "server id"),
    profilesAllowed: source.profilesAllowed,
    profiles
  };
}

export function parseProfileSelectionGrant(value: unknown): ProfileSelectionGrant {
  const source = recordValue(value, "profile selection grant");
  exactKeysWithOptional(source, ["token", "authority", "accountId", "serverId", "profileId", "pinRevision", "expiresAt"], ["installationId"], "profile selection grant");
  if (source.authority !== "hosted" && source.authority !== "local") throw new TypeError("profile selection grant authority is invalid");
  if (!Number.isSafeInteger(source.pinRevision) || (source.pinRevision as number) < 0) throw new TypeError("profile selection grant pinRevision is invalid");
  const expiresAt = boundedIdentity(source.expiresAt, "profile selection grant expiry", 64);
  if (!Number.isFinite(Date.parse(expiresAt))) throw new TypeError("profile selection grant expiry is invalid");
  return {
    token: boundedIdentity(source.token, "profile selection grant token", 2048),
    authority: source.authority,
    accountId: boundedIdentity(source.accountId, "profile selection grant account"),
    serverId: boundedIdentity(source.serverId, "profile selection grant server"),
    profileId: boundedIdentity(source.profileId, "profile selection grant profile"),
    pinRevision: source.pinRevision as number,
    ...(source.installationId === undefined ? {} : {installationId: boundedIdentity(source.installationId, "profile selection grant installation")}),
    expiresAt
  };
}

export function parseAutomaticProfileTrust(value: unknown): AutomaticProfileTrust {
  const source = recordValue(value, "automatic profile trust");
  exactKeysWithOptional(source, [
    "version", "purpose", "token", "authority", "accountId", "serverId", "profileId", "pinRevision", "expiresAt"
  ], ["installationId"], "automatic profile trust");
  if (source.version !== AUTOMATIC_PROFILE_TRUST_VERSION || source.purpose !== "automatic-profile-selection") {
    throw new TypeError("automatic profile trust purpose or version is invalid");
  }
  if (source.authority !== "hosted" && source.authority !== "local") throw new TypeError("automatic profile trust authority is invalid");
  if (!Number.isSafeInteger(source.pinRevision) || (source.pinRevision as number) < 0) throw new TypeError("automatic profile trust pinRevision is invalid");
  const expiresAt = boundedIdentity(source.expiresAt, "automatic profile trust expiry", 64);
  if (!Number.isFinite(Date.parse(expiresAt))) throw new TypeError("automatic profile trust expiry is invalid");
  return {
    version: AUTOMATIC_PROFILE_TRUST_VERSION,
    purpose: "automatic-profile-selection",
    token: boundedIdentity(source.token, "automatic profile trust token", 2048),
    authority: source.authority,
    accountId: boundedIdentity(source.accountId, "automatic profile trust account"),
    serverId: boundedIdentity(source.serverId, "automatic profile trust server"),
    profileId: boundedIdentity(source.profileId, "automatic profile trust profile"),
    pinRevision: source.pinRevision as number,
    ...(source.installationId === undefined ? {} : {installationId: boundedIdentity(source.installationId, "automatic profile trust installation")}),
    expiresAt
  };
}

export function automaticProfileTrustFromGrant(
  value: unknown,
  expected: { authority: ProfileAuthority; accountId: string; serverId: string; installationId?: string; now?: Date }
): AutomaticProfileTrust | undefined {
  const grant = parseAutomaticProfileTrust(value);
  const accountId = boundedIdentity(expected.accountId, "account id");
  const serverId = boundedIdentity(expected.serverId, "server id");
  const now = expected.now ?? new Date();
  if (!Number.isFinite(now.getTime())) throw new TypeError("profile selection time is invalid");
  if (grant.authority !== expected.authority
    || grant.accountId !== accountId
    || grant.serverId !== serverId
    || grant.installationId !== expected.installationId
    || Date.parse(grant.expiresAt) <= now.getTime()) return undefined;
  return grant;
}

/** Shared launch policy for web, mobile, television, and future clients. */
export function decideProfileSelection(
  untrustedDirectory: ProfileDirectory,
  preference: { profileSelection: "ask" | "last-used"; lastProfileId?: string },
  trusted?: AutomaticProfileTrust,
  context?: { serverId: string; installationId?: string; now?: Date }
): ProfileSelectionDecision {
  const directory = parseProfileDirectory(untrustedDirectory);
  const profiles = [...directory.profiles]
    .filter(profile => directory.profilesAllowed || profile.isPrimary)
    .sort((left, right) => left.sortOrder - right.sortOrder || compareCodePoints(left.name, right.name));
  if (profiles.length === 0) return { kind: "unavailable" };

  const hasAutomaticTrust = (profile: PorticoProfile): boolean => {
    if (trusted && context) {
      try {
        const verified = automaticProfileTrustFromGrant(trusted, {
          authority: directory.authority,
          accountId: directory.accountId,
          serverId: context.serverId,
          installationId: context.installationId,
          now: context.now
        });
        if (verified
          && verified.accountId === directory.accountId
          && verified.serverId === directory.serverId
          && verified.profileId === profile.id
          && verified.pinRevision === profile.pinRevision) return true;
      } catch {
        // Malformed trust is treated exactly like missing trust.
      }
    }
    return false;
  };

  if (profiles.length === 1) {
    const only = profiles[0];
    if (!only.hasPIN || hasAutomaticTrust(only)) return { kind: "open", profile: only };
    return { kind: "pin", profile: only };
  }
  if (preference.profileSelection === "last-used" && preference.lastProfileId) {
    const last = profiles.find(profile => profile.id === preference.lastProfileId);
    // An unlocked profile can be selected automatically because the final
    // authority-scoped server session still establishes viewer identity. A
    // locked profile additionally requires a revocable server-issued trust;
    // optional installation metadata by itself never bypasses its PIN.
    if (last && (!last.hasPIN || hasAutomaticTrust(last))) return { kind: "open", profile: last };
  }
  return { kind: "select", profiles };
}
