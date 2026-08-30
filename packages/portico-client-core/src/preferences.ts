/**
 * Canonical V1 preference contract shared by every Portico client.
 *
 * The contract separates portable viewing choices from device presentation,
 * profile selection, and installation hardware limits. It contains requests,
 * never authority: server capabilities, membership limits, profile policy,
 * media availability, and operating-system restrictions always win.
 */

import type { PlaybackIntent } from "./types.js";

export const PORTICO_PREFERENCES_VERSION = "v1" as const;

export type PorticoPreferencesVersion = typeof PORTICO_PREFERENCES_VERSION;
export type PorticoDeviceClass = "web" | "mobile" | "television";
export type NetworkClass = "local" | "wifi" | "cellular" | "unknown";
export type SkipSegmentBehavior = "ask" | "automatic" | "off";
export type SubtitlePreferenceSize = "small" | "medium" | "large";
export type SubtitleBackground = "none" | "subtle" | "solid";
export type AudioNormalizationMode = "off" | "attenuate";
export type DeliveryModePreference = "allow" | "prefer" | "never";
export type TranscodePreference = "allow" | "prefer" | "require";
export type DeliveryPreferenceRequest = {
  directPlay: DeliveryModePreference;
  directStream: DeliveryModePreference;
  transcode: TranscodePreference;
};
export type InterfaceDensity = "comfortable" | "compact";
export type MotionPreference = "system" | "reduced" | "full";
export type ProfileSelectionMode = "ask" | "last-used";
export type DefaultLandingSurface = "home" | "library" | "channels" | "saved" | "downloads";
export type RepeatPreference = "none" | "one" | "all";
export type DateFormatPreference = "short" | "medium" | "long";
export type HourCyclePreference = "auto" | "h12" | "h23";
export type QualityRequestMode = "off" | "automatic" | "original" | "high" | "standard" | "data-saver";

export type PreferenceClear = { $clear: true };

export type DeepPartial<T> = T extends readonly unknown[]
  ? T | PreferenceClear
  : T extends object
    ? { [K in keyof T]?: DeepPartial<T[K]> } | PreferenceClear
    : T | PreferenceClear;

export type PreferenceDocument<T> = {
  version: PorticoPreferencesVersion;
  revision: number;
  values: T;
};

export type PreferencePatch<T> = {
  version: PorticoPreferencesVersion;
  expectedRevision: number;
  changes: DeepPartial<T>;
};

export type QualityRequest = {
  mode: QualityRequestMode;
  maxVideoBitrateMbps?: number;
  maxAudioBitrateKbps?: number;
  maxVideoHeight?: number;
  allowHDR?: boolean;
};

export type DownloadQualityRequest = {
  mode: "ask" | Exclude<QualityRequestMode, "off" | "automatic">;
  maxVideoBitrateMbps?: number;
  maxAudioBitrateKbps?: number;
  maxVideoHeight?: number;
};

export type NetworkQualityPreferences = Record<NetworkClass, QualityRequest>;

export type ProfileServerPreferences = {
  localization: {
    locale: string;
    timeZone: string;
    dateFormat: DateFormatPreference;
    hourCycle: HourCyclePreference;
  };
  home: {
    rowOrder: string[];
    hiddenRowIds: string[];
  };
  playback: {
    autoplayNext: boolean;
    upNextCountdownSeconds: 0 | 5 | 10 | 15;
    passoutProtection: boolean;
    passoutAfterEpisodes: 2 | 3 | 4 | 5;
    introSkip: SkipSegmentBehavior;
    creditsSkip: SkipSegmentBehavior;
    startedThresholdPercent: number;
    playedThresholdPercent: number;
    skipBackSeconds: 5 | 10 | 15 | 30;
    skipForwardSeconds: 10 | 15 | 30 | 45;
    defaultSpeed: 0.5 | 0.75 | 1 | 1.25 | 1.5 | 1.75 | 2;
    preferredAudioLanguages: string[];
    preferredSubtitleLanguages: string[];
    subtitlesEnabled: boolean;
    subtitleSize: SubtitlePreferenceSize;
    subtitleBackground: SubtitleBackground;
    showSyncedLyrics: boolean;
  };
  music: {
    shuffleDefault: boolean;
    repeatDefault: RepeatPreference;
    autoplayDefault: boolean;
    audioNormalization: AudioNormalizationMode;
    crossfadeSeconds: number;
    gapless: boolean;
  };
  privacy: {
    pauseWatchHistory: boolean;
    showActivityToMembers: boolean;
    includeInWatchWithFriends: boolean;
  };
  search: {
    rememberHistory: boolean;
    recentQueries: string[];
  };
  downloads: {
    quality: DownloadQualityRequest;
    deleteWatched: boolean;
  };
};

export type ProfileDeviceClassPreferences = {
  deviceClass: PorticoDeviceClass;
  appearance: {
    density: InterfaceDensity;
    cardSizePercent: number;
    showBackdrops: boolean;
  };
  navigation: {
    sidebarCollapsed: boolean;
    pinnedLibraryIds: string[];
    defaultLanding: DefaultLandingSurface;
  };
  playback: {
    deliveryRequest: DeliveryPreferenceRequest;
    quality: NetworkQualityPreferences;
  };
};

export type AccountServerInstallationPreferences = {
  rememberAccount: boolean;
  profileSelection: ProfileSelectionMode;
  lastProfileId?: string;
};

export type InstallationPreferences = {
  accessibility: {
    motion: MotionPreference;
  };
  downloads: {
    wifiOnly: boolean;
    storageLimitBytes?: number;
  };
  diagnostics: {
    playback: boolean;
  };
};

export type PorticoPreferenceBundle = {
  profileServer: PreferenceDocument<ProfileServerPreferences>;
  profileDeviceClass: PreferenceDocument<ProfileDeviceClassPreferences>;
  accountServerInstallation: PreferenceDocument<AccountServerInstallationPreferences>;
  installation: PreferenceDocument<InstallationPreferences>;
};

export type PreferenceScopeIdentity = {
  authority: "hosted" | "local";
  accountId: string;
  serverId: string;
  profileId: string;
  deviceClass: PorticoDeviceClass;
  installationId: string;
};

export type PlaybackPreferencePolicy = {
  networkAllowed: Record<NetworkClass, boolean>;
  deliveryAllowed: {
    directPlay: boolean;
    directStream: boolean;
    transcode: boolean;
  };
  maximumVideoBitrateMbps?: number;
  maximumAudioBitrateKbps?: number;
  maximumVideoHeight?: number;
  allowHDR: boolean;
};

export type EffectiveQualityRequest = QualityRequest & {
  allowed: boolean;
};

export type EffectiveDeliveryPreference = Omit<DeliveryPreferenceRequest, "transcode"> & {
  transcode: TranscodePreference | "never";
};

export type PreferenceCapabilities = {
  downloads: boolean;
  cellularQuality: boolean;
  collapsibleNavigation: boolean;
};

export type HomeRowPreferenceTarget = {
  id: string;
  priority?: number;
  hideable?: boolean;
  reorderable?: boolean;
};

export class PreferenceConflictError extends Error {
  readonly currentRevision: number;

  constructor(currentRevision: number) {
    super("preference document revision conflict");
    this.name = "PreferenceConflictError";
    this.currentRevision = currentRevision;
  }
}

const qualityDefaults: NetworkQualityPreferences = {
  local: { mode: "original", allowHDR: true },
  wifi: { mode: "original", allowHDR: true },
  cellular: { mode: "original", allowHDR: true },
  unknown: { mode: "original", allowHDR: true }
};

const defaultDeliveryRequest: Readonly<DeliveryPreferenceRequest> = deepFreeze({
  directPlay: "prefer",
  directStream: "allow",
  transcode: "allow"
});

function deepFreeze<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    for (const child of Object.values(value as Record<string, unknown>)) deepFreeze(child);
    Object.freeze(value);
  }
  return value;
}

/** Runtime limits mirrored exactly by the published V1 OpenAPI preference schemas. */
export const viewerPreferenceLimitsV1 = deepFreeze({
  startedThresholdPercent: { minimum: 1, maximum: 25 },
  playedThresholdPercent: { minimum: 75, maximum: 100 },
  cardSizePercent: { minimum: 75, maximum: 150 },
  audioBitrateKbps: { minimum: 32, maximum: 4096 },
  videoHeights: [360, 480, 720, 1080, 1440, 2160, 4320] as const,
  searchHistory: { maximumItems: 20, maximumQueryRunes: 160 }
});

export const defaultProfileServerPreferences: ProfileServerPreferences = deepFreeze({
  localization: { locale: "en-US", timeZone: "UTC", dateFormat: "medium", hourCycle: "auto" },
  home: { rowOrder: [], hiddenRowIds: [] },
  playback: {
    autoplayNext: true,
    upNextCountdownSeconds: 10,
    passoutProtection: true,
    passoutAfterEpisodes: 3,
    introSkip: "ask",
    creditsSkip: "ask",
    startedThresholdPercent: 5,
    playedThresholdPercent: 95,
    skipBackSeconds: 10,
    skipForwardSeconds: 30,
    defaultSpeed: 1,
    preferredAudioLanguages: ["original"],
    preferredSubtitleLanguages: [],
    subtitlesEnabled: false,
    subtitleSize: "medium",
    subtitleBackground: "subtle",
    showSyncedLyrics: true
  },
  music: {
    shuffleDefault: false,
    repeatDefault: "none",
    autoplayDefault: true,
    audioNormalization: "off",
    crossfadeSeconds: 0,
    gapless: true
  },
  privacy: { pauseWatchHistory: false, showActivityToMembers: true, includeInWatchWithFriends: true },
  search: { rememberHistory: true, recentQueries: [] },
  downloads: { quality: { mode: "ask" }, deleteWatched: false }
});

export function defaultProfileDeviceClassPreferences(deviceClass: PorticoDeviceClass): ProfileDeviceClassPreferences {
  return {
    deviceClass,
    appearance: { density: "comfortable", cardSizePercent: 100, showBackdrops: true },
    navigation: { sidebarCollapsed: false, pinnedLibraryIds: [], defaultLanding: "home" },
    playback: {
      deliveryRequest: { ...defaultDeliveryRequest },
      quality: structuredCloneQualityPreferences(qualityDefaults)
    }
  };
}

export function defaultAccountServerInstallationPreferences(deviceClass: PorticoDeviceClass): AccountServerInstallationPreferences {
  return { rememberAccount: true, profileSelection: deviceClass === "television" ? "ask" : "last-used" };
}

export const defaultInstallationPreferences: InstallationPreferences = deepFreeze({
  accessibility: { motion: "system" },
  downloads: { wifiOnly: true },
  diagnostics: { playback: false }
});

export function preferenceCapabilitiesForDeviceClass(deviceClass: PorticoDeviceClass): PreferenceCapabilities {
  if (!["web", "mobile", "television"].includes(deviceClass)) throw new TypeError("deviceClass is invalid");
  return {
    downloads: deviceClass !== "television",
    cellularQuality: deviceClass === "mobile",
    collapsibleNavigation: deviceClass !== "mobile"
  };
}

/** Operating-system reduced-motion state always wins over an app request. */
export function resolveMotionPreference(preference: MotionPreference, osReducedMotion: boolean): "reduced" | "full" {
  if (!["system", "reduced", "full"].includes(preference)) throw new TypeError("motion preference is invalid");
  if (osReducedMotion || preference === "reduced") return "reduced";
  return "full";
}

function structuredCloneQualityPreferences(value: NetworkQualityPreferences): NetworkQualityPreferences {
  return {
    local: { ...value.local },
    wifi: { ...value.wifi },
    cellular: { ...value.cellular },
    unknown: { ...value.unknown }
  };
}

function recordValue(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${name} must be an object`);
  return value as Record<string, unknown>;
}

function optionalRecord(value: unknown, name = "preference value"): Record<string, unknown> {
  if (value === undefined) return {};
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${name} must be an object`);
  return value as Record<string, unknown>;
}

function knownKeys(source: Record<string, unknown>, allowed: readonly string[], name: string): void {
  const set = new Set(allowed);
  const unknown = Object.keys(source).find(key => !set.has(key));
  if (unknown) throw new TypeError(`${name} contains unknown field ${unknown}`);
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  if (value === undefined) return fallback;
  if (typeof value !== "boolean") throw new TypeError("preference value must be a boolean");
  return value;
}

function boundedString(value: unknown, fallback: string, maximum = 128): string {
  if (value === undefined) return fallback;
  if (typeof value !== "string") throw new TypeError("preference value must be a string");
  const normalized = value.trim();
  if (!normalized || Array.from(normalized).length > maximum) throw new TypeError("preference string is invalid");
  return normalized;
}

function optionalBoundedString(value: unknown, maximum = 128): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "string" || !value.trim() || Array.from(value.trim()).length > maximum) throw new TypeError("invalid preference identifier");
  return value.trim();
}

function uniqueStrings(value: unknown, limit: number, maximumLength: number): string[] {
  if (value === undefined) return [];
  if (!Array.isArray(value) || value.length > limit) throw new TypeError("preference string list is invalid");
  const result: string[] = [];
  const seen = new Set<string>();
  for (const entry of value) {
    if (typeof entry !== "string") throw new TypeError("preference string list is invalid");
    const normalized = entry.trim();
    if (!normalized || Array.from(normalized).length > maximumLength || seen.has(normalized)) throw new TypeError("preference string list is invalid");
    seen.add(normalized);
    result.push(normalized);
  }
  return result;
}

function choice<T extends string | number>(value: unknown, values: readonly T[], fallback: T): T {
  if (value === undefined) return fallback;
  if (!values.includes(value as T)) throw new TypeError("preference choice is invalid");
  return value as T;
}

function boundedNumber(value: unknown, minimum: number, maximum: number, fallback: number): number {
  if (value === undefined) return fallback;
  if (typeof value !== "number" || !Number.isFinite(value) || value < minimum || value > maximum) throw new TypeError("preference number is invalid");
  return value;
}

function optionalBoundedInteger(value: unknown, minimum: number, maximum: number): number | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < minimum || value > maximum) throw new TypeError("preference integer is invalid");
  return value;
}

function optionalBoundedIntegerWithFallback(value: unknown, minimum: number, maximum: number, fallback?: number): number | undefined {
  return value === undefined ? fallback : optionalBoundedInteger(value, minimum, maximum);
}

function optionalIntegerChoice(value: unknown, choices: readonly number[]): number | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "number" || !Number.isSafeInteger(value) || !choices.includes(value)) throw new TypeError("preference integer is invalid");
  return value;
}

function optionalIntegerChoiceWithFallback(value: unknown, choices: readonly number[], fallback?: number): number | undefined {
  return value === undefined ? fallback : optionalIntegerChoice(value, choices);
}

function minimumDefined(left?: number, right?: number): number | undefined {
  if (left === undefined) return right;
  if (right === undefined) return left;
  return Math.min(left, right);
}

function normalizeQualityRequest(value: unknown, fallback: QualityRequest): QualityRequest {
  const source = optionalRecord(value);
  knownKeys(source, ["mode", "maxVideoBitrateMbps", "maxAudioBitrateKbps", "maxVideoHeight", "allowHDR"], "quality request");
  const mode = choice(source.mode, ["off", "automatic", "original", "high", "standard", "data-saver"] as const, fallback.mode);
  const result: QualityRequest = {
    mode,
    maxVideoBitrateMbps: optionalBoundedIntegerWithFallback(source.maxVideoBitrateMbps, 1, 1000, fallback.maxVideoBitrateMbps),
    maxAudioBitrateKbps: optionalBoundedIntegerWithFallback(source.maxAudioBitrateKbps, viewerPreferenceLimitsV1.audioBitrateKbps.minimum, viewerPreferenceLimitsV1.audioBitrateKbps.maximum, fallback.maxAudioBitrateKbps),
    maxVideoHeight: optionalIntegerChoiceWithFallback(source.maxVideoHeight, viewerPreferenceLimitsV1.videoHeights, fallback.maxVideoHeight),
    allowHDR: booleanValue(source.allowHDR, fallback.allowHDR ?? true)
  };
  if (mode !== "high" && mode !== "standard" && mode !== "data-saver") {
    delete result.maxVideoBitrateMbps;
    delete result.maxAudioBitrateKbps;
    delete result.maxVideoHeight;
  }
  return result;
}

function normalizeDownloadQualityRequest(value: unknown, fallback: DownloadQualityRequest): DownloadQualityRequest {
  const source = optionalRecord(value);
  knownKeys(source, ["mode", "maxVideoBitrateMbps", "maxAudioBitrateKbps", "maxVideoHeight"], "download quality request");
  const mode = choice(source.mode, ["ask", "original", "high", "standard", "data-saver"] as const, fallback.mode);
  const result: DownloadQualityRequest = {
    mode,
    maxVideoBitrateMbps: optionalBoundedInteger(source.maxVideoBitrateMbps, 1, 1000),
    maxAudioBitrateKbps: optionalBoundedInteger(source.maxAudioBitrateKbps, viewerPreferenceLimitsV1.audioBitrateKbps.minimum, viewerPreferenceLimitsV1.audioBitrateKbps.maximum),
    maxVideoHeight: optionalIntegerChoice(source.maxVideoHeight, viewerPreferenceLimitsV1.videoHeights)
  };
  if (mode !== "high" && mode !== "standard" && mode !== "data-saver") {
    delete result.maxVideoBitrateMbps;
    delete result.maxAudioBitrateKbps;
    delete result.maxVideoHeight;
  }
  return result;
}

const segmentBehaviors = ["ask", "automatic", "off"] as const;

function normalizeSegmentBehavior(value: unknown, fallback: SkipSegmentBehavior): SkipSegmentBehavior {
  return choice(value, segmentBehaviors, fallback);
}

function normalizeDeliveryPreferenceRequest(value: unknown, fallback: DeliveryPreferenceRequest): DeliveryPreferenceRequest {
  if (value === undefined) return { ...fallback };
  const source = recordValue(value, "delivery preference");
  knownKeys(source, ["directPlay", "directStream", "transcode"], "delivery preference");
  const result: DeliveryPreferenceRequest = {
    directPlay: choice(source.directPlay, ["allow", "prefer", "never"] as const, fallback.directPlay),
    directStream: choice(source.directStream, ["allow", "prefer", "never"] as const, fallback.directStream),
    transcode: choice(source.transcode, ["allow", "prefer", "require"] as const, fallback.transcode)
  };
  const preferred = [result.directPlay, result.directStream, result.transcode].filter(mode => mode === "prefer").length;
  if (preferred > 1) throw new TypeError("only one delivery mode may be preferred");
  if (result.transcode === "require" && (result.directPlay !== "never" || result.directStream !== "never")) {
    throw new TypeError("required transcoding must disable direct delivery");
  }
  return result;
}

export function normalizeProfileServerPreferences(value: unknown): ProfileServerPreferences {
  const source = optionalRecord(value);
  knownKeys(source, ["localization", "home", "playback", "music", "privacy", "search", "downloads"], "profile-server preferences");
  const localization = optionalRecord(source.localization);
  const home = optionalRecord(source.home);
  const playback = optionalRecord(source.playback);
  const music = optionalRecord(source.music);
  const privacy = optionalRecord(source.privacy);
  const search = optionalRecord(source.search);
  const downloads = optionalRecord(source.downloads);
  knownKeys(localization, ["locale", "timeZone", "dateFormat", "hourCycle"], "localization preferences");
  knownKeys(home, ["rowOrder", "hiddenRowIds"], "home preferences");
  knownKeys(playback, ["autoplayNext", "upNextCountdownSeconds", "passoutProtection", "passoutAfterEpisodes", "introSkip", "creditsSkip", "startedThresholdPercent", "playedThresholdPercent", "skipBackSeconds", "skipForwardSeconds", "defaultSpeed", "preferredAudioLanguages", "preferredSubtitleLanguages", "subtitlesEnabled", "subtitleSize", "subtitleBackground", "showSyncedLyrics"], "playback preferences");
  knownKeys(music, ["shuffleDefault", "repeatDefault", "autoplayDefault", "audioNormalization", "crossfadeSeconds", "gapless"], "music preferences");
  knownKeys(privacy, ["pauseWatchHistory", "showActivityToMembers", "includeInWatchWithFriends"], "privacy preferences");
  knownKeys(search, ["rememberHistory", "recentQueries"], "search preferences");
  knownKeys(downloads, ["quality", "deleteWatched"], "download preferences");
  const defaults = defaultProfileServerPreferences;
  const rememberHistory = booleanValue(search.rememberHistory, defaults.search.rememberHistory);
  const audioLanguages = uniqueStrings(playback.preferredAudioLanguages, 8, 35);
  return {
    localization: {
      locale: boundedString(localization.locale, defaults.localization.locale, 35),
      timeZone: boundedString(localization.timeZone, defaults.localization.timeZone, 64),
      dateFormat: choice(localization.dateFormat, ["short", "medium", "long"] as const, defaults.localization.dateFormat),
      hourCycle: choice(localization.hourCycle, ["auto", "h12", "h23"] as const, defaults.localization.hourCycle)
    },
    home: { rowOrder: uniqueStrings(home.rowOrder, 100, 128), hiddenRowIds: uniqueStrings(home.hiddenRowIds, 100, 128) },
    playback: {
      autoplayNext: booleanValue(playback.autoplayNext, defaults.playback.autoplayNext),
      upNextCountdownSeconds: choice(playback.upNextCountdownSeconds, [0, 5, 10, 15] as const, defaults.playback.upNextCountdownSeconds),
      passoutProtection: booleanValue(playback.passoutProtection, defaults.playback.passoutProtection),
      passoutAfterEpisodes: choice(playback.passoutAfterEpisodes, [2, 3, 4, 5] as const, defaults.playback.passoutAfterEpisodes),
      introSkip: normalizeSegmentBehavior(playback.introSkip, defaults.playback.introSkip),
      creditsSkip: normalizeSegmentBehavior(playback.creditsSkip, defaults.playback.creditsSkip),
      startedThresholdPercent: Math.round(boundedNumber(playback.startedThresholdPercent, viewerPreferenceLimitsV1.startedThresholdPercent.minimum, viewerPreferenceLimitsV1.startedThresholdPercent.maximum, defaults.playback.startedThresholdPercent)),
      playedThresholdPercent: Math.round(boundedNumber(playback.playedThresholdPercent, viewerPreferenceLimitsV1.playedThresholdPercent.minimum, viewerPreferenceLimitsV1.playedThresholdPercent.maximum, defaults.playback.playedThresholdPercent)),
      skipBackSeconds: choice(playback.skipBackSeconds, [5, 10, 15, 30] as const, defaults.playback.skipBackSeconds),
      skipForwardSeconds: choice(playback.skipForwardSeconds, [10, 15, 30, 45] as const, defaults.playback.skipForwardSeconds),
      defaultSpeed: choice(playback.defaultSpeed, [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] as const, defaults.playback.defaultSpeed),
      preferredAudioLanguages: audioLanguages.length ? audioLanguages : [...defaults.playback.preferredAudioLanguages],
      preferredSubtitleLanguages: uniqueStrings(playback.preferredSubtitleLanguages, 8, 35),
      subtitlesEnabled: booleanValue(playback.subtitlesEnabled, defaults.playback.subtitlesEnabled),
      subtitleSize: choice(playback.subtitleSize, ["small", "medium", "large"] as const, defaults.playback.subtitleSize),
      subtitleBackground: choice(playback.subtitleBackground, ["none", "subtle", "solid"] as const, defaults.playback.subtitleBackground),
      showSyncedLyrics: booleanValue(playback.showSyncedLyrics, defaults.playback.showSyncedLyrics)
    },
    music: {
      shuffleDefault: booleanValue(music.shuffleDefault, defaults.music.shuffleDefault),
      repeatDefault: choice(music.repeatDefault, ["none", "one", "all"] as const, defaults.music.repeatDefault),
      autoplayDefault: booleanValue(music.autoplayDefault, defaults.music.autoplayDefault),
      audioNormalization: choice(music.audioNormalization, ["off", "attenuate"] as const, defaults.music.audioNormalization),
      crossfadeSeconds: Math.round(boundedNumber(music.crossfadeSeconds, 0, 12, defaults.music.crossfadeSeconds)),
      gapless: booleanValue(music.gapless, defaults.music.gapless)
    },
    privacy: {
      pauseWatchHistory: booleanValue(privacy.pauseWatchHistory, defaults.privacy.pauseWatchHistory),
      showActivityToMembers: booleanValue(privacy.showActivityToMembers, defaults.privacy.showActivityToMembers),
      includeInWatchWithFriends: booleanValue(privacy.includeInWatchWithFriends, defaults.privacy.includeInWatchWithFriends)
    },
    search: {
      rememberHistory,
      recentQueries: rememberHistory ? uniqueStrings(search.recentQueries, viewerPreferenceLimitsV1.searchHistory.maximumItems, viewerPreferenceLimitsV1.searchHistory.maximumQueryRunes) : []
    },
    downloads: {
      quality: normalizeDownloadQualityRequest(downloads.quality, defaults.downloads.quality),
      deleteWatched: booleanValue(downloads.deleteWatched, defaults.downloads.deleteWatched)
    }
  };
}

export function normalizeProfileDeviceClassPreferences(value: unknown, deviceClass: PorticoDeviceClass): ProfileDeviceClassPreferences {
  const source = optionalRecord(value);
  knownKeys(source, ["deviceClass", "appearance", "navigation", "playback"], "profile device-class preferences");
  if (source.deviceClass !== undefined && source.deviceClass !== deviceClass) throw new TypeError("device class does not match preference scope");
  const appearance = optionalRecord(source.appearance);
  const navigation = optionalRecord(source.navigation);
  const playback = optionalRecord(source.playback);
  const quality = optionalRecord(playback.quality);
  knownKeys(appearance, ["density", "cardSizePercent", "showBackdrops"], "appearance preferences");
  knownKeys(navigation, ["sidebarCollapsed", "pinnedLibraryIds", "defaultLanding"], "navigation preferences");
  knownKeys(playback, ["deliveryRequest", "quality"], "device playback preferences");
  knownKeys(quality, ["local", "wifi", "cellular", "unknown"], "network quality preferences");
  const defaults = defaultProfileDeviceClassPreferences(deviceClass);
  return {
    deviceClass,
    appearance: {
      density: choice(appearance.density, ["comfortable", "compact"] as const, defaults.appearance.density),
      cardSizePercent: Math.round(boundedNumber(appearance.cardSizePercent, viewerPreferenceLimitsV1.cardSizePercent.minimum, viewerPreferenceLimitsV1.cardSizePercent.maximum, defaults.appearance.cardSizePercent)),
      showBackdrops: booleanValue(appearance.showBackdrops, defaults.appearance.showBackdrops)
    },
    navigation: {
      sidebarCollapsed: booleanValue(navigation.sidebarCollapsed, defaults.navigation.sidebarCollapsed),
      pinnedLibraryIds: uniqueStrings(navigation.pinnedLibraryIds, 100, 128),
      defaultLanding: choice(
        navigation.defaultLanding,
        deviceClass === "television" ? ["home", "library", "channels", "saved"] as const : ["home", "library", "channels", "saved", "downloads"] as const,
        defaults.navigation.defaultLanding
      )
    },
    playback: {
      deliveryRequest: normalizeDeliveryPreferenceRequest(playback.deliveryRequest, defaults.playback.deliveryRequest),
      quality: {
        local: normalizeQualityRequest(quality.local, defaults.playback.quality.local),
        wifi: normalizeQualityRequest(quality.wifi, defaults.playback.quality.wifi),
        cellular: normalizeQualityRequest(quality.cellular, defaults.playback.quality.cellular),
        unknown: normalizeQualityRequest(quality.unknown, defaults.playback.quality.unknown)
      }
    }
  };
}

export function normalizeAccountServerInstallationPreferences(value: unknown, deviceClass: PorticoDeviceClass): AccountServerInstallationPreferences {
  const source = optionalRecord(value);
  knownKeys(source, ["rememberAccount", "profileSelection", "lastProfileId"], "account-server installation preferences");
  const defaults = defaultAccountServerInstallationPreferences(deviceClass);
  return {
    rememberAccount: booleanValue(source.rememberAccount, defaults.rememberAccount),
    profileSelection: choice(source.profileSelection, ["ask", "last-used"] as const, defaults.profileSelection),
    lastProfileId: optionalBoundedString(source.lastProfileId)
  };
}

export function normalizeInstallationPreferences(value: unknown): InstallationPreferences {
  const source = optionalRecord(value);
  knownKeys(source, ["accessibility", "downloads", "diagnostics"], "installation preferences");
  const accessibility = optionalRecord(source.accessibility);
  const downloads = optionalRecord(source.downloads);
  const diagnostics = optionalRecord(source.diagnostics);
  knownKeys(accessibility, ["motion"], "installation accessibility preferences");
  knownKeys(downloads, ["wifiOnly", "storageLimitBytes"], "installation download preferences");
  knownKeys(diagnostics, ["playback"], "installation diagnostics preferences");
  return {
    accessibility: { motion: choice(accessibility.motion, ["system", "reduced", "full"] as const, defaultInstallationPreferences.accessibility.motion) },
    downloads: {
      wifiOnly: booleanValue(downloads.wifiOnly, defaultInstallationPreferences.downloads.wifiOnly),
      storageLimitBytes: optionalBoundedInteger(downloads.storageLimitBytes, 256 * 1024 * 1024, 10 * 1024 * 1024 * 1024 * 1024)
    },
    diagnostics: { playback: booleanValue(diagnostics.playback, defaultInstallationPreferences.diagnostics.playback) }
  };
}

export function createPreferenceDocument<T>(values: T, revision = 0): PreferenceDocument<T> {
  if (!Number.isSafeInteger(revision) || revision < 0) throw new TypeError("preference revision must be a non-negative safe integer");
  return { version: PORTICO_PREFERENCES_VERSION, revision, values };
}

export function parsePreferenceDocument<T>(value: unknown, normalize: (input: unknown) => T): PreferenceDocument<T> {
  const source = recordValue(value, "preference document");
  knownKeys(source, ["version", "revision", "values"], "preference document");
  if (!["version", "revision", "values"].every(key => Object.hasOwn(source, key))) throw new TypeError("incomplete preference document");
  if (source.version !== PORTICO_PREFERENCES_VERSION) throw new TypeError("unsupported preference document version");
  if (!Number.isSafeInteger(source.revision) || (source.revision as number) < 0) throw new TypeError("invalid preference document revision");
  recordValue(source.values, "preference document values");
  return { version: PORTICO_PREFERENCES_VERSION, revision: source.revision as number, values: normalize(source.values) };
}

function isPreferenceClear(value: unknown): value is PreferenceClear {
  return Boolean(value && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value as Record<string, unknown>).length === 1
    && (value as Record<string, unknown>).$clear === true);
}

function mergePreferenceValue(current: unknown, changes: unknown): unknown {
  if (isPreferenceClear(changes)) return undefined;
  if (!changes || typeof changes !== "object" || Array.isArray(changes)) return changes;
  const base = current && typeof current === "object" && !Array.isArray(current) ? current as Record<string, unknown> : {};
  const result: Record<string, unknown> = { ...base };
  for (const [key, value] of Object.entries(changes as Record<string, unknown>)) {
    result[key] = mergePreferenceValue(base[key], value);
  }
  return result;
}

export function applyPreferencePatch<T>(document: PreferenceDocument<T>, patch: PreferencePatch<T>, normalize: (input: unknown) => T): PreferenceDocument<T> {
  const current = parsePreferenceDocument(document, normalize);
  const source = recordValue(patch, "preference patch");
  knownKeys(source, ["version", "expectedRevision", "changes"], "preference patch");
  if (!["version", "expectedRevision", "changes"].every(key => Object.hasOwn(source, key))) throw new TypeError("incomplete preference patch");
  if (source.version !== PORTICO_PREFERENCES_VERSION) throw new TypeError("unsupported preference patch version");
  if (!Number.isSafeInteger(source.expectedRevision) || (source.expectedRevision as number) < 0) throw new TypeError("invalid expected preference revision");
  if (source.expectedRevision !== current.revision) throw new PreferenceConflictError(current.revision);
  if (!source.changes || typeof source.changes !== "object" || Array.isArray(source.changes)) throw new TypeError("preference patch changes must be an object");
  return createPreferenceDocument(normalize(mergePreferenceValue(current.values, source.changes)), current.revision + 1);
}

function keyIdentity(value: string, name: string): string {
  const normalized = value.trim();
  if (!normalized || normalized.length > 128) throw new TypeError(`${name} is invalid`);
  return encodeURIComponent(normalized);
}

export function preferenceStorageKeys(identity: PreferenceScopeIdentity): {
  profileServer: string;
  profileDeviceClass: string;
  accountServerInstallation: string;
  installation: string;
} {
  if (!["web", "mobile", "television"].includes(identity.deviceClass)) throw new TypeError("deviceClass is invalid");
  if (identity.authority !== "hosted" && identity.authority !== "local") throw new TypeError("authority is invalid");
  const authority = identity.authority;
  const account = keyIdentity(identity.accountId, "accountId");
  const server = keyIdentity(identity.serverId, "serverId");
  const profile = keyIdentity(identity.profileId, "profileId");
  const installation = keyIdentity(identity.installationId, "installationId");
  return {
    profileServer: `portico:v1:authority:${authority}:account:${account}:server:${server}:profile:${profile}`,
    profileDeviceClass: `portico:v1:authority:${authority}:account:${account}:server:${server}:profile:${profile}:device-class:${identity.deviceClass}`,
    accountServerInstallation: `portico:v1:authority:${authority}:account:${account}:server:${server}:installation:${installation}`,
    installation: `portico:v1:installation:${installation}`
  };
}

export function resolveQualityRequest(preferences: ProfileDeviceClassPreferences, network: NetworkClass, policy: PlaybackPreferencePolicy): EffectiveQualityRequest {
  const request = preferences.playback.quality[network];
  if (!policy.networkAllowed[network] || request.mode === "off") return { mode: "off", allowed: false, allowHDR: false };
  const maxVideoBitrateMbps = minimumDefined(request.maxVideoBitrateMbps, policy.maximumVideoBitrateMbps);
  const maxAudioBitrateKbps = minimumDefined(request.maxAudioBitrateKbps, policy.maximumAudioBitrateKbps);
  const maxVideoHeight = minimumDefined(request.maxVideoHeight, policy.maximumVideoHeight);
  const allowHDR = Boolean(request.allowHDR && policy.allowHDR);
  return {
    ...request,
    mode: request.mode,
    allowed: true,
    allowHDR,
    maxVideoBitrateMbps,
    maxAudioBitrateKbps,
    maxVideoHeight
  };
}

export function resolveDeliveryPreference(preferences: ProfileDeviceClassPreferences, policy: PlaybackPreferencePolicy): EffectiveDeliveryPreference {
  const allowed = policy.deliveryAllowed;
  if (!allowed || typeof allowed !== "object"
    || typeof allowed.directPlay !== "boolean"
    || typeof allowed.directStream !== "boolean"
    || typeof allowed.transcode !== "boolean") throw new TypeError("playback delivery policy is invalid");
  if (!allowed.directPlay && !allowed.directStream && !allowed.transcode) throw new TypeError("playback policy has no allowed delivery mode");
  const request = preferences.playback.deliveryRequest;
  return {
    directPlay: allowed.directPlay ? request.directPlay : "never",
    directStream: allowed.directStream ? request.directStream : "never",
    transcode: allowed.transcode ? request.transcode : "never"
  };
}

/** Produces portable intent for playback start, restore, prepare-next, and handoff. */
export function playbackIntentFromPreferences(
  preferences: ProfileDeviceClassPreferences,
  network: NetworkClass,
  policy: PlaybackPreferencePolicy,
  portablePreferences?: {
    playback: Pick<ProfileServerPreferences["playback"], "preferredAudioLanguages" | "preferredSubtitleLanguages" | "subtitlesEnabled">;
  }
): PlaybackIntent {
  // The canonical preference document intentionally has no remote bucket:
  // unknown is the conservative request and the server independently detects
  // and clamps remote routes.
  const preferenceNetwork: NetworkClass = network;
  const quality = resolveQualityRequest(preferences, preferenceNetwork, policy);
  if (!quality.allowed || quality.mode === "off") {
    throw new TypeError("playback is disabled for this network class");
  }
  const delivery = resolveDeliveryPreference(preferences, policy);
  const deliveryPolicies: Pick<PlaybackIntent, "directPlayPolicy" | "directStreamPolicy" | "transcodePolicy"> = {
    directPlayPolicy: delivery.directPlay,
    directStreamPolicy: delivery.directStream,
    transcodePolicy: delivery.transcode
  };
  const qualityProfile: PlaybackIntent["qualityProfile"] = quality.mode === "data-saver" ? "data_saver"
    : quality.mode;
  const preferredAudioLanguage = (portablePreferences?.playback.preferredAudioLanguages ?? [])
    .map((language) => language.trim())
    .find((language) => language.length > 0 && language.toLowerCase() !== "original");
  const preferredSubtitleLanguage = portablePreferences?.playback.subtitlesEnabled
    ? (portablePreferences.playback.preferredSubtitleLanguages ?? [])
      .map((language) => language.trim())
      .find((language) => language.length > 0)
    : undefined;
  return {
    transportClass: network === "wifi" || network === "cellular" ? network : "unknown",
    qualityProfile,
    ...deliveryPolicies,
    maxVideoBitrateMbps: quality.maxVideoBitrateMbps,
    maxAudioBitrateKbps: quality.maxAudioBitrateKbps,
    maxVideoHeight: quality.maxVideoHeight,
    allowHdr: quality.allowHDR,
    preferredAudioLanguage,
    preferredSubtitleLanguage,
    preferredSubtitleMode: portablePreferences?.playback.subtitlesEnabled ? "text" : "off"
  };
}

export function applyHomeRowPreferences<T extends HomeRowPreferenceTarget>(rows: readonly T[], preferences: ProfileServerPreferences): T[] {
  const hidden = new Set(preferences.home.hiddenRowIds);
  const visible = rows
    .filter(row => row.hideable !== true || !hidden.has(row.id))
    .sort((left, right) => (left.priority ?? 0) - (right.priority ?? 0));
  const rank = new Map(preferences.home.rowOrder.map((id, index) => [id, index]));
  const movable = visible.filter(row => row.reorderable === true).sort((left, right) => {
    const leftRank = rank.get(left.id);
    const rightRank = rank.get(right.id);
    if (leftRank === undefined && rightRank === undefined) return (left.priority ?? 0) - (right.priority ?? 0);
    if (leftRank === undefined) return 1;
    if (rightRank === undefined) return -1;
    if (leftRank !== rightRank) return leftRank - rightRank;
    return (left.priority ?? 0) - (right.priority ?? 0);
  });
  let movableIndex = 0;
  return visible.map(row => row.reorderable === true ? movable[movableIndex++] : row);
}

export function recordRecentPreferenceQuery(current: readonly string[], query: string, rememberHistory: boolean, limit = 8): string[] {
  if (!rememberHistory) return [];
  const normalized = query.trim().slice(0, 256);
  if (!normalized) return [...current];
  const key = normalized.toLocaleLowerCase();
  return [normalized, ...current.filter(entry => entry.trim().toLocaleLowerCase() !== key)].slice(0, Math.max(1, Math.min(12, limit)));
}
