import {
  defaultProfileDeviceClassPreferences,
  playbackIntentFromPreferences,
  viewerPreferenceLimitsV1,
  type NetworkClass,
  type PlaybackIntent,
  type PlaybackPreferencePolicy,
  type ProfileDeviceClassPreferences,
} from '@porticomediaserver/client-core';

export type WebNetworkClass = NetworkClass;
export type WebPlaybackQuality = 'off' | 'automatic' | 'original' | 'high' | 'standard' | 'data-saver';
export type WebSegmentSkipPreference = 'ask' | 'automatic' | 'off';
export type WebDeliveryPreference = { directPlay: 'allow' | 'prefer' | 'never'; directStream: 'allow' | 'prefer' | 'never'; transcode: 'allow' | 'prefer' | 'require' };

export type WebDisplayPreferences = {
  showBackdrops: boolean;
  reduceMotion: boolean;
  cardSizePercent: number;
  sidebarCollapsed: boolean;
  pinnedLibraryIds: string[];
  homeRowOrder: string[];
  hiddenHomeRows: string[];
  rememberSearchHistory: boolean;
  recentSearches: string[];
  skipBackSeconds: 5 | 10 | 15;
  skipForwardSeconds: 15 | 30 | 45;
  autoplayNext: boolean;
  upNextCountdownSeconds: 5 | 10 | 15 | 0;
  subtitleSize: 'small' | 'medium' | 'large';
  subtitleBackground: 'none' | 'subtle' | 'solid';
  showSyncedLyrics: boolean;
  playbackDiagnostics: boolean;
  introSkip: WebSegmentSkipPreference;
  creditsSkip: WebSegmentSkipPreference;
  passoutProtection: boolean;
  passoutAfterEpisodes: 2 | 3 | 4 | 5;
  deliveryRequest: WebDeliveryPreference;
  playbackQuality: Record<WebNetworkClass, WebPlaybackQuality>;
  defaultPlaybackSpeed: '1' | '1.25' | '1.5';
  audioNormalizationMode: 'off' | 'attenuate';
};

export const defaultWebDisplayPreferences: WebDisplayPreferences = {
  showBackdrops: true,
  reduceMotion: false,
  cardSizePercent: 100,
  sidebarCollapsed: false,
  pinnedLibraryIds: [],
  homeRowOrder: [],
  hiddenHomeRows: [],
  rememberSearchHistory: true,
  recentSearches: [],
  skipBackSeconds: 10,
  skipForwardSeconds: 30,
  autoplayNext: true,
  upNextCountdownSeconds: 10,
  subtitleSize: 'medium',
  subtitleBackground: 'subtle',
  showSyncedLyrics: true,
  playbackDiagnostics: false,
  introSkip: 'ask',
  creditsSkip: 'ask',
  passoutProtection: true,
  passoutAfterEpisodes: 3,
  deliveryRequest: { directPlay: 'prefer', directStream: 'allow', transcode: 'allow' },
  playbackQuality: {
    local: 'original',
    wifi: 'original',
    cellular: 'original',
    unknown: 'original',
  },
  defaultPlaybackSpeed: '1',
  audioNormalizationMode: 'off',
};

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.map((entry) => String(entry).trim()).filter(Boolean))];
}

function choice<T extends string | number>(value: unknown, options: readonly T[], fallback: T): T {
  return options.includes(value as T) ? value as T : fallback;
}

function cardSize(value: unknown): number {
  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(parsed)) return defaultWebDisplayPreferences.cardSizePercent;
  return Math.max(viewerPreferenceLimitsV1.cardSizePercent.minimum, Math.min(viewerPreferenceLimitsV1.cardSizePercent.maximum, Math.round(parsed)));
}

export function normalizeWebDisplayPreferences(value: unknown): WebDisplayPreferences {
  const source = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
  const quality = source.playbackQuality && typeof source.playbackQuality === 'object' && !Array.isArray(source.playbackQuality)
    ? source.playbackQuality as Partial<Record<WebNetworkClass, unknown>>
    : {};
  return {
    showBackdrops: typeof source.showBackdrops === 'boolean' ? source.showBackdrops : defaultWebDisplayPreferences.showBackdrops,
    reduceMotion: typeof source.reduceMotion === 'boolean' ? source.reduceMotion : defaultWebDisplayPreferences.reduceMotion,
    cardSizePercent: cardSize(source.cardSizePercent),
    sidebarCollapsed: typeof source.sidebarCollapsed === 'boolean' ? source.sidebarCollapsed : defaultWebDisplayPreferences.sidebarCollapsed,
    pinnedLibraryIds: stringList(source.pinnedLibraryIds),
    homeRowOrder: stringList(source.homeRowOrder),
    hiddenHomeRows: stringList(source.hiddenHomeRows),
    rememberSearchHistory: typeof source.rememberSearchHistory === 'boolean' ? source.rememberSearchHistory : defaultWebDisplayPreferences.rememberSearchHistory,
    recentSearches: stringList(source.recentSearches).slice(0, viewerPreferenceLimitsV1.searchHistory.maximumItems),
    skipBackSeconds: choice(source.skipBackSeconds, [5, 10, 15] as const, defaultWebDisplayPreferences.skipBackSeconds),
    skipForwardSeconds: choice(source.skipForwardSeconds, [15, 30, 45] as const, defaultWebDisplayPreferences.skipForwardSeconds),
    autoplayNext: typeof source.autoplayNext === 'boolean' ? source.autoplayNext : defaultWebDisplayPreferences.autoplayNext,
    upNextCountdownSeconds: choice(source.upNextCountdownSeconds, [0, 5, 10, 15] as const, defaultWebDisplayPreferences.upNextCountdownSeconds),
    subtitleSize: choice(source.subtitleSize, ['small', 'medium', 'large'] as const, defaultWebDisplayPreferences.subtitleSize),
    subtitleBackground: choice(source.subtitleBackground, ['none', 'subtle', 'solid'] as const, defaultWebDisplayPreferences.subtitleBackground),
    showSyncedLyrics: typeof source.showSyncedLyrics === 'boolean' ? source.showSyncedLyrics : defaultWebDisplayPreferences.showSyncedLyrics,
    playbackDiagnostics: typeof source.playbackDiagnostics === 'boolean' ? source.playbackDiagnostics : defaultWebDisplayPreferences.playbackDiagnostics,
		introSkip: choice(source.introSkip, ['ask', 'automatic', 'off'] as const, defaultWebDisplayPreferences.introSkip),
		creditsSkip: choice(source.creditsSkip, ['ask', 'automatic', 'off'] as const, defaultWebDisplayPreferences.creditsSkip),
    passoutProtection: typeof source.passoutProtection === 'boolean' ? source.passoutProtection : defaultWebDisplayPreferences.passoutProtection,
    passoutAfterEpisodes: choice(source.passoutAfterEpisodes, [2, 3, 4, 5] as const, defaultWebDisplayPreferences.passoutAfterEpisodes),
		deliveryRequest: normalizeDeliveryRequest(source.deliveryRequest),
    playbackQuality: {
      local: normalizeQuality(quality.local, defaultWebDisplayPreferences.playbackQuality.local),
      wifi: normalizeQuality(quality.wifi, defaultWebDisplayPreferences.playbackQuality.wifi),
      cellular: normalizeQuality(quality.cellular, defaultWebDisplayPreferences.playbackQuality.cellular),
      unknown: normalizeQuality(quality.unknown, defaultWebDisplayPreferences.playbackQuality.unknown),
    },
    defaultPlaybackSpeed: choice(source.defaultPlaybackSpeed, ['1', '1.25', '1.5'] as const, defaultWebDisplayPreferences.defaultPlaybackSpeed),
    audioNormalizationMode: source.audioNormalizationMode === 'attenuate' ? 'attenuate' : 'off',
  };
}

function localOrLanHost(hostname: string): boolean {
  const value = hostname.toLocaleLowerCase().replace(/^\[|\]$/g, '');
  if (value === 'localhost' || value === '::1' || value.endsWith('.local')) return true;
  if (/^127\./.test(value) || /^10\./.test(value) || /^192\.168\./.test(value)) return true;
  const private172 = value.match(/^172\.(\d{1,3})\./);
  return Boolean(private172 && Number(private172[1]) >= 16 && Number(private172[1]) <= 31);
}

export function browserNetworkClass(browser: Pick<Window, 'location' | 'navigator'> = window): WebNetworkClass {
  if (localOrLanHost(browser.location.hostname)) return 'local';
  const connection = (browser.navigator as Navigator & { connection?: { type?: string; saveData?: boolean } }).connection;
  if (connection?.type === 'wifi') return 'wifi';
  if (connection?.type === 'cellular' || connection?.saveData) return 'cellular';
  return 'unknown';
}

export function webPlaybackIntent(preferences: WebDisplayPreferences, browser?: Pick<Window, 'location' | 'navigator'>): PlaybackIntent {
  const networkClass = browserNetworkClass(browser ?? window);
	// Local storage contains installation-scoped display state only. Playback compilation itself is
  // exclusively Client Core-owned and consumes the canonical network buckets.
  const canonical = defaultProfileDeviceClassPreferences('web') as ProfileDeviceClassPreferences;
  (canonical.playback as unknown as { deliveryRequest: WebDeliveryPreference }).deliveryRequest = preferences.deliveryRequest;
  (canonical.playback as unknown as { quality: Record<WebNetworkClass, { mode: WebPlaybackQuality }> }).quality = Object.fromEntries(Object.entries(preferences.playbackQuality).map(([network, mode]) => [network, { mode }])) as Record<WebNetworkClass, { mode: WebPlaybackQuality }>;
  const policy = {
    networkAllowed: { local: true, wifi: true, cellular: true, unknown: true },
    deliveryAllowed: { directPlay: true, directStream: true, transcode: true },
    allowedDeliveryRequests: ['automatic', 'prefer-direct-play', 'prefer-direct-stream', 'prefer-transcode'],
    allowHDR: true,
  } as unknown as PlaybackPreferencePolicy;
  return playbackIntentFromPreferences(canonical, networkClass, policy);
}

function normalizeQuality(value: unknown, fallback: WebPlaybackQuality): WebPlaybackQuality {
  return choice(value === 'data_saver' ? 'data-saver' : value === 'disabled' ? 'off' : value, ['off', 'automatic', 'original', 'high', 'standard', 'data-saver'] as const, fallback);
}

function normalizeDeliveryRequest(value: unknown): WebDeliveryPreference {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const source = value as Record<string, unknown>;
    return {
      directPlay: choice(source.directPlay, ['allow', 'prefer', 'never'] as const, 'prefer'),
      directStream: choice(source.directStream, ['allow', 'prefer', 'never'] as const, 'allow'),
      transcode: choice(source.transcode, ['allow', 'prefer', 'require'] as const, 'allow'),
    };
  }
  if (value === 'prefer_direct_stream' || value === 'prefer-direct-stream') return { directPlay: 'allow', directStream: 'prefer', transcode: 'allow' };
  if (value === 'prefer_transcode' || value === 'prefer-transcode') return { directPlay: 'allow', directStream: 'allow', transcode: 'require' };
  return { directPlay: 'prefer', directStream: 'allow', transcode: 'allow' };
}

export function serializeWebDisplayPreferences(value: WebDisplayPreferences): Record<string, unknown> {
  const normalized = normalizeWebDisplayPreferences(value);
  return { ...normalized };
}

export function orderHomeRows<T extends { id: string; priority?: number }>(rows: readonly T[], order: readonly string[]): T[] {
	const continuityRank = new Map([['continue', 0], ['continue_listening', 1]]);
  const rank = new Map(order.map((id, index) => [id, index]));
  return [...rows].sort((left, right) => {
	const leftContinuity = continuityRank.get(left.id);
	const rightContinuity = continuityRank.get(right.id);
	if (leftContinuity !== undefined || rightContinuity !== undefined) {
		if (leftContinuity === undefined) return 1;
		if (rightContinuity === undefined) return -1;
		return leftContinuity - rightContinuity;
	}
    const leftRank = rank.get(left.id);
    const rightRank = rank.get(right.id);
    if (leftRank !== undefined || rightRank !== undefined) {
      if (leftRank === undefined) return 1;
      if (rightRank === undefined) return -1;
      if (leftRank !== rightRank) return leftRank - rightRank;
    }
	// Product Contract priorities are ascending: a smaller number is a more
	// prominent row. Matching that convention prevents a missing/stale client
	// preference document from silently reversing the server-owned Home policy.
	return (left.priority ?? Number.MAX_SAFE_INTEGER) - (right.priority ?? Number.MAX_SAFE_INTEGER);
  });
}

export function completeHomeRowOrder<T extends { id: string; priority?: number }>(rows: readonly T[], order: readonly string[]): string[] {
  return orderHomeRows(rows, order).map((row) => row.id);
}

export function recordRecentSearch(current: readonly string[], query: string, limit = 8): string[] {
  const normalized = query.trim();
  if (!normalized) return [...current];
  const key = normalized.toLocaleLowerCase();
  return [normalized, ...current.filter((entry) => entry.trim().toLocaleLowerCase() !== key)].slice(0, limit);
}
