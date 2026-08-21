export type WebDefaultLanding = "home" | "last-library" | "live";
export type WebSubtitleSize = "small" | "medium" | "large";
export type WebSubtitleBackground = "none" | "subtle" | "solid";
export type WebUpNextCountdown = "5" | "8" | "15" | "off";
export type WebDefaultPlaybackSpeed = "1" | "1.25" | "1.5";

export interface WebDisplayPreferences {
  skipBackSeconds: number;
  skipForwardSeconds: number;
  autoplayNextEpisode: boolean;
  upNextCountdown: WebUpNextCountdown;
  skipIntroPrompts: boolean;
  defaultPlaybackSpeed: WebDefaultPlaybackSpeed;
  subtitleSize: WebSubtitleSize;
  subtitleBackground: WebSubtitleBackground;
  showSyncedLyrics: boolean;
  sidebarCollapsed: boolean;
  showBackdrops: boolean;
  reduceMotion: boolean;
  continueRowsFirst: boolean;
  defaultLanding: WebDefaultLanding;
  pinnedLibraryIds: string[];
  audioNormalizationMode: "off" | "attenuate";
  homeRowOrder: string[];
  hiddenHomeRows: string[];
}

export const defaultWebDisplayPreferences: WebDisplayPreferences = {
  skipBackSeconds: 10,
  skipForwardSeconds: 30,
  autoplayNextEpisode: true,
  upNextCountdown: "8",
  skipIntroPrompts: true,
  defaultPlaybackSpeed: "1",
  subtitleSize: "medium",
  subtitleBackground: "subtle",
  showSyncedLyrics: true,
  sidebarCollapsed: false,
  showBackdrops: true,
  reduceMotion: false,
  continueRowsFirst: false,
  defaultLanding: "home",
  pinnedLibraryIds: [],
  audioNormalizationMode: "off",
  homeRowOrder: [],
  hiddenHomeRows: []
};

export function normalizeWebDisplayPreferences(preferences?: Record<string, unknown>): WebDisplayPreferences {
  return {
    skipBackSeconds: skipBackSeconds(preferences?.skipBackSeconds),
    skipForwardSeconds: skipForwardSeconds(preferences?.skipForwardSeconds),
    autoplayNextEpisode: typeof preferences?.autoplayNextEpisode === "boolean" ? preferences.autoplayNextEpisode : defaultWebDisplayPreferences.autoplayNextEpisode,
    upNextCountdown: upNextCountdown(preferences?.upNextCountdown),
    skipIntroPrompts: typeof preferences?.skipIntroPrompts === "boolean" ? preferences.skipIntroPrompts : defaultWebDisplayPreferences.skipIntroPrompts,
    defaultPlaybackSpeed: defaultPlaybackSpeed(preferences?.defaultPlaybackSpeed),
    subtitleSize: subtitleSize(preferences?.subtitleSize),
    subtitleBackground: subtitleBackground(preferences?.subtitleBackground),
    showSyncedLyrics: typeof preferences?.showSyncedLyrics === "boolean" ? preferences.showSyncedLyrics : defaultWebDisplayPreferences.showSyncedLyrics,
    sidebarCollapsed: typeof preferences?.sidebarCollapsed === "boolean" ? preferences.sidebarCollapsed : defaultWebDisplayPreferences.sidebarCollapsed,
    showBackdrops: typeof preferences?.showBackdrops === "boolean" ? preferences.showBackdrops : defaultWebDisplayPreferences.showBackdrops,
    reduceMotion: typeof preferences?.reduceMotion === "boolean" ? preferences.reduceMotion : defaultWebDisplayPreferences.reduceMotion,
    continueRowsFirst: typeof preferences?.continueRowsFirst === "boolean" ? preferences.continueRowsFirst : defaultWebDisplayPreferences.continueRowsFirst,
    defaultLanding: defaultLanding(preferences?.defaultLanding),
    pinnedLibraryIds: stringArray(preferences?.pinnedLibraryIds),
    audioNormalizationMode: preferences?.audioNormalizationMode === "attenuate" ? "attenuate" : "off",
    homeRowOrder: stringArray(preferences?.homeRowOrder),
    hiddenHomeRows: stringArray(preferences?.hiddenHomeRows)
  };
}

export function serializeWebDisplayPreferences(preferences: WebDisplayPreferences): Record<string, unknown> {
  return {
    skipBackSeconds: skipBackSeconds(preferences.skipBackSeconds),
    skipForwardSeconds: skipForwardSeconds(preferences.skipForwardSeconds),
    autoplayNextEpisode: Boolean(preferences.autoplayNextEpisode),
    upNextCountdown: upNextCountdown(preferences.upNextCountdown),
    skipIntroPrompts: Boolean(preferences.skipIntroPrompts),
    defaultPlaybackSpeed: defaultPlaybackSpeed(preferences.defaultPlaybackSpeed),
    subtitleSize: subtitleSize(preferences.subtitleSize),
    subtitleBackground: subtitleBackground(preferences.subtitleBackground),
    showSyncedLyrics: Boolean(preferences.showSyncedLyrics),
    sidebarCollapsed: Boolean(preferences.sidebarCollapsed),
    showBackdrops: Boolean(preferences.showBackdrops),
    reduceMotion: Boolean(preferences.reduceMotion),
    continueRowsFirst: Boolean(preferences.continueRowsFirst),
    defaultLanding: defaultLanding(preferences.defaultLanding),
    pinnedLibraryIds: stringArray(preferences.pinnedLibraryIds),
    audioNormalizationMode: preferences.audioNormalizationMode === "attenuate" ? "attenuate" : "off",
    homeRowOrder: stringArray(preferences.homeRowOrder),
    hiddenHomeRows: stringArray(preferences.hiddenHomeRows)
  };
}

function boundedSeconds(value: unknown, fallback: number): number {
  const numeric = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(120, Math.max(1, Math.round(numeric)));
}

function skipBackSeconds(value: unknown): number {
  const seconds = boundedSeconds(value, defaultWebDisplayPreferences.skipBackSeconds);
  return seconds === 5 || seconds === 15 ? seconds : 10;
}

function skipForwardSeconds(value: unknown): number {
  const seconds = boundedSeconds(value, defaultWebDisplayPreferences.skipForwardSeconds);
  return seconds === 15 || seconds === 45 ? seconds : 30;
}

function defaultLanding(value: unknown): WebDefaultLanding {
  return value === "last-library" || value === "live" ? value : "home";
}

function subtitleSize(value: unknown): WebSubtitleSize {
  return value === "small" || value === "large" ? value : "medium";
}

function subtitleBackground(value: unknown): WebSubtitleBackground {
  return value === "none" || value === "solid" ? value : "subtle";
}

function upNextCountdown(value: unknown): WebUpNextCountdown {
  const stringValue = String(value);
  return stringValue === "5" || stringValue === "15" || stringValue === "off" ? stringValue : "8";
}

function defaultPlaybackSpeed(value: unknown): WebDefaultPlaybackSpeed {
  const stringValue = String(value);
  return stringValue === "1.25" || stringValue === "1.5" ? stringValue : "1";
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}
