import { media, musicMedia } from './fixtures';
import {
  ApiError,
  defaultAccountServerInstallationPreferences,
  defaultProfileDeviceClassPreferences,
  defaultProfileServerPreferences,
  unrestrictedProfilePolicy,
  type AutomaticProfileTrust,
  type LibraryBrowseCapabilities,
  type AdminLibraryChannelListResponse,
  type LibraryChannelAggregate,
  type LibraryChannelConfigurationRequest,
  type LibraryChannelGeneration,
  type LibraryChannelGuide,
  type LibraryChannelsGuide,
  type LibraryChannelListResponse,
  type LibraryChannelRestoreDefaultsRequest,
  type LibraryChannelRestoreDefaultsResponse,
  type LibraryChannelTemplatesResponse,
  type BrowseFacetOption,
  type BrowseFacetSource,
  type MediaImage,
  type PlaybackHandoffRequest,
  type PlaybackPrepareNextRequest,
  type PlaybackProgressEvent,
  type PlaybackResponse,
  type PlaybackRepeatMode,
  type ProductContract,
  type PlaybackSessionQueueRequest,
  type PlaybackSessionQueueReplaceRequest,
  type PlaybackSessionQueueResponse,
  type LyricSearchCandidate,
  type MediaAttachment,
  type SavedView,
  type SavedViewCreateRequest,
  type SearchContract,
  type MediaLyric,
  type MediaTrickplaySet,
  type NotificationAudience,
  type NotificationInvalidation,
  type NotificationPage,
  type NotificationReceiptAction,
  type NotificationReceiptResult,
  type ServerManagedProfileCreateRequest,
  type ServerManagedProfileDirectory,
  type ServerManagedProfileUpdateRequest,
  type ServerOwnerFeedbackPage,
  type ServerOwnerFeedbackRecord,
  type ServerOwnerFeedbackUpdateRequest,
  type ServerOwnerNotificationRecipientDirectory,
  type ServerOwnerNoticeRequest,
  type ServerPatchedPreferenceDocument,
  type ServerProfileAdministrationProofResponse,
  type ServerViewerPreferenceBundle,
  type ViewerFeedbackCapabilities,
  type ViewerFeedbackReceipt,
  type ViewerFeedbackSubmission,
  type DownloadPreparation,
} from '@porticomediaserver/client-core';
import type {
  ActionableDVRRecording,
  ActionableDVRRule,
  ActionableLiveTVChannel,
  ActionableLiveTVProgram,
  ActionableLiveTVSource,
  DVRResult,
  HomeResult,
  HomeRow,
  LibraryBrowseInput,
  LibraryBrowseResult,
  LocalProfileLoginChallenge,
  LocalProfileSelection,
  MediaDownloadOptions,
  MediaDeleteResult,
  MediaItem,
  MediaChildrenPage,
  MediaJob,
  MediaJobOptions,
  MediaJobType,
  MediaMatchCandidate,
  MediaMetadataUpdate,
  PersonDetail,
  PlaybackStartOptions,
  PorticoDataSource,
  SearchPageResult,
  SearchPageInput,
  SearchResult,
  SearchHistoryItem,
  SavedResourceCreateInput,
  SavedResourceDetail,
  SavedResourceItemsInput,
  SavedResourceItemsMutation,
  SavedEditableResourceKind,
  SavedPlaylistEntry,
  SavedResourceKind,
  SavedResourceShare,
  SavedResourceShareRequest,
  SavedResourceSummary,
  SavedResourceUpdateInput,
  SavedShareCandidatePage,
  Viewer,
  ProfileAdministrationProofInput,
  ProfilePINReauthentication,
} from './models';
import { FixtureFilesystemSource, fixtureDirectory, type FilesystemPickerSource } from '../features/filesystem';
import type {
  BrowseExpression,
  BrowseSort,
  LibraryPivotInput,
  LibraryPivotPage,
  LibraryPresentation,
} from '../features/library/libraryTypes';
import { FixtureSettingsDataSource } from '../features/settings/FixtureSettingsDataSource';
import type { SettingsDataSource } from '../features/settings/settingsTypes';
import { FixtureWatchWithFriendsSource, type WatchWithFriendsSource, type WatchWithFriendsViewer } from '../features/watch-with-friends';

const silentPreview = 'data:audio/wav;base64,UklGRkQDAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YSADAACAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgA==';

function playbackMedia(item: MediaItem) {
  return {
    id: item.id, libraryId: item.libraryId, type: item.kind, title: item.title, sortTitle: item.title,
    metadataEtag: `fixture-media-${item.id}-revision-1`, metadataRevision: 1,
    year: item.year || undefined, durationSeconds: 1, summary: item.summary, tagline: item.subtitle,
    genres: item.genre ? [item.genre] : [], tags: [], labels: [], addedAt: '2026-01-01T00:00:00Z',
    images: { poster: item.poster, backdrop: item.backdrop, thumb: item.backdrop },
    state: { watchlisted: item.watchlisted ?? false, favorite: item.favorite ?? false, reaction: item.reaction ?? '', watched: item.watched ?? false, progressSeconds: 0, rating: item.userRating ?? 0 },
    actions: ['play', 'queue.add', 'watchlist.add', 'favorite.add', 'reaction.set', 'rating.set', 'watched.set'] as PlaybackResponse['media']['actions'],
  };
}

function normalizedTitle(title: string) {
  return title.replace(/^(a|an|the)\s+/i, '').toLocaleLowerCase();
}

function sortItems(items: LibraryBrowseResult['items'], input: LibraryBrowseInput) {
  const direction = input.direction === 'ascending' ? 1 : -1;
  return [...items].sort((left, right) => {
    if (input.sort === 'Recently added' || input.sort === 'Year') {
      return (left.year - right.year) * direction;
    }
    if (input.sort === 'Critic rating') {
      return left.rating.localeCompare(right.rating) * direction;
    }
    return normalizedTitle(left.title).localeCompare(normalizedTitle(right.title)) * direction;
  });
}

function fixtureWorkspaceKind(libraryId: string): LibraryBrowseCapabilities['library']['kind'] {
  if (libraryId === 'fixture-movies') return 'movies';
  if (libraryId === 'fixture-music') return 'music';
  return 'tv';
}

function fixtureWorkspacePivots(kind: LibraryBrowseCapabilities['library']['kind']): LibraryBrowseCapabilities['pivots'] {
  const pivot = (
    id: string,
    label: string,
    entityKinds: string[],
    defaultView: string,
    browseSupported: boolean,
    endpointTemplate: string,
    defaultSort: BrowseSort[] = [{ field: 'title', direction: 'asc' }],
  ) => ({
    id, label, entityKinds, defaultView, browseSupported, endpointTemplate, defaultSort,
    supportedViews: browseSupported ? ['grid', 'compact-grid', 'list', 'table'] : [defaultView],
    presentationFields: ['title', 'subtitle', 'year', 'durationSeconds', 'contentRating', 'availability'],
  });
  if (kind === 'music') return [
    pivot('discover', 'Discover', ['album', 'track'], 'shelves', false, '/api/libraries/{libraryId}/discover', [{ field: 'dateAdded', direction: 'desc' }]),
    pivot('artists', 'Artists', ['artist'], 'grid', true, '/api/libraries/{libraryId}/browse'),
    pivot('albums', 'Albums', ['album'], 'grid', true, '/api/libraries/{libraryId}/browse'),
    pivot('tracks', 'Tracks', ['track'], 'list', true, '/api/libraries/{libraryId}/browse', [{ field: 'album', direction: 'asc' }, { field: 'trackNumber', direction: 'asc' }]),
    pivot('playlists', 'Playlists', ['playlist'], 'list', false, '/api/playlists?libraryId={libraryId}'),
    pivot('genres', 'Genres', ['category'], 'facets', false, '/api/libraries/{libraryId}/categories'),
  ];
  if (kind === 'movies') return [
    pivot('discover', 'Discover', ['movie'], 'shelves', false, '/api/libraries/{libraryId}/discover', [{ field: 'dateAdded', direction: 'desc' }]),
    pivot('movies', 'Movies', ['movie'], 'grid', true, '/api/libraries/{libraryId}/browse'),
    pivot('collections', 'Collections', ['collection'], 'grid', false, '/api/collections?libraryId={libraryId}'),
    pivot('categories', 'Categories', ['category'], 'facets', false, '/api/libraries/{libraryId}/categories'),
  ];
  return [
    pivot('discover', 'Discover', ['show'], 'shelves', false, '/api/libraries/{libraryId}/discover', [{ field: 'dateAdded', direction: 'desc' }]),
    pivot('shows', 'Shows', ['show'], 'grid', true, '/api/libraries/{libraryId}/browse'),
    pivot('episodes', 'Episodes', ['episode'], 'list', true, '/api/libraries/{libraryId}/browse', [{ field: 'seasonNumber', direction: 'asc' }, { field: 'episodeNumber', direction: 'asc' }]),
    pivot('collections', 'Collections', ['collection'], 'grid', false, '/api/collections?libraryId={libraryId}'),
    pivot('categories', 'Categories', ['category'], 'facets', false, '/api/libraries/{libraryId}/categories'),
  ];
}

function fixtureBrowseCapabilities(libraryId: string): LibraryBrowseCapabilities {
  const kind = fixtureWorkspaceKind(libraryId);
  const label = kind === 'movies' ? 'Movies' : kind === 'music' ? 'Music' : 'TV Shows';
  return {
    apiVersion: 'v1',
    library: { id: libraryId, name: label, kind },
    pivots: fixtureWorkspacePivots(kind),
    fields: [
      { id: 'entityKind', label: 'Media type', valueType: 'enum', operators: ['equals', 'not-equals', 'in', 'not-in'], allowedValues: ['movie', 'show', 'season', 'episode', 'artist', 'album', 'track'], controlHint: 'select', complexity: 'standard', cost: 'indexed' },
      { id: 'title', label: 'Title', valueType: 'string', operators: ['equals', 'not-equals', 'contains', 'not-contains', 'starts-with'], controlHint: 'text', complexity: 'quick', cost: 'indexed' },
      { id: 'year', label: 'Year', valueType: 'number', operators: ['equals', 'less-than', 'at-most', 'greater-than', 'at-least', 'between'], controlHint: 'number-range', complexity: 'quick', cost: 'indexed' },
      { id: 'decade', label: 'Decade', valueType: 'number', operators: ['equals'], allowedValues: ['2020', '2010', '2000', '1990', '1980'], controlHint: 'select', complexity: 'standard', cost: 'indexed' },
      { id: 'playState', label: 'Playback', valueType: 'enum', operators: ['equals', 'not-equals', 'in', 'not-in'], allowedValues: ['unplayed', 'in-progress', 'played'], controlHint: 'select', complexity: 'quick', cost: 'indexed' },
      { id: 'favorite', label: 'Favorite', valueType: 'boolean', operators: ['is'], controlHint: 'toggle', complexity: 'quick', cost: 'indexed' },
      { id: 'watchlisted', label: 'Watchlisted', valueType: 'boolean', operators: ['is'], controlHint: 'toggle', complexity: 'quick', cost: 'indexed' },
      { id: 'genre', label: 'Genre', valueType: 'identity-set', operators: ['contains', 'not-contains', 'contains-any', 'contains-all'], facetSource: { endpointTemplate: '/api/libraries/{libraryId}/categories', filterField: 'filter', filterPrefix: 'genre:', valueField: 'name', labelField: 'name', countField: 'count' }, controlHint: 'facet-multi-select', complexity: 'standard', cost: 'indexed-join' },
      { id: 'availability', label: 'Availability', valueType: 'enum', operators: ['equals', 'not-equals', 'in', 'not-in'], allowedValues: ['available', 'partial', 'unavailable'], controlHint: 'select', complexity: 'quick', cost: 'indexed' },
    ],
    sorts: [
      { id: 'title', label: 'Title', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false },
      { id: 'dateAdded', label: 'Recently added', defaultDirection: 'desc', directions: ['asc', 'desc'], expensive: false },
      { id: 'year', label: 'Year', defaultDirection: 'desc', directions: ['asc', 'desc'], expensive: false },
      { id: 'criticRating', label: 'Critic rating', defaultDirection: 'desc', directions: ['asc', 'desc'], expensive: false },
      { id: 'seasonNumber', label: 'Season', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false, applicableKinds: ['episode'] },
      { id: 'episodeNumber', label: 'Episode', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false, applicableKinds: ['episode'] },
      { id: 'album', label: 'Album', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false, applicableKinds: ['track'] },
      { id: 'trackNumber', label: 'Track', defaultDirection: 'asc', directions: ['asc', 'desc'], expensive: false, applicableKinds: ['track'] },
    ],
    presentationFields: ['title', 'subtitle', 'year', 'durationSeconds', 'contentRating', 'availability'],
    queryLimits: { defaultLimit: 60, maximumLimit: 200, maximumClauses: 24, maximumDepth: 4, maximumBytes: 65_536, cursorTtlSeconds: 900 },
    actions: ['play', 'watchlist.add', 'favorite.add', 'metadata.edit', 'manageLibrary'],
  };
}

function fixtureFieldValue(item: MediaItem, field: string): unknown {
  if (field === 'entityKind') return item.kind;
  if (field === 'title') return item.title;
  if (field === 'year') return item.year;
  if (field === 'decade') return Math.floor(item.year / 10) * 10;
  if (field === 'favorite') return item.favorite ?? false;
  if (field === 'watchlisted') return item.watchlisted ?? false;
  if (field === 'genre') return item.genre;
  if (field === 'availability') return item.availability ?? 'available';
  if (field === 'playState') return item.progress ? 'in-progress' : item.watched ? 'played' : 'unplayed';
  return undefined;
}

function fixturePredicate(item: MediaItem, expression: BrowseExpression): boolean {
  if ('all' in expression) return expression.all.every((child) => fixturePredicate(item, child));
  if ('any' in expression) return expression.any.some((child) => fixturePredicate(item, child));
  if ('not' in expression) return !fixturePredicate(item, expression.not);
  const actual = fixtureFieldValue(item, expression.field);
  const expected = expression.value;
  const left = String(actual ?? '').toLocaleLowerCase();
  const right = String(expected ?? '').toLocaleLowerCase();
  if (expression.operator === 'is') return Boolean(actual) === Boolean(expected);
  if (expression.operator === 'contains') return left.includes(right);
  if (expression.operator === 'not-contains') return !left.includes(right);
  if (expression.operator === 'starts-with') return left.startsWith(right);
  if (expression.operator === 'not-equals') return left !== right;
  if (expression.operator === 'greater-than') return Number(actual) > Number(expected);
  if (expression.operator === 'at-least') return Number(actual) >= Number(expected);
  if (expression.operator === 'less-than') return Number(actual) < Number(expected);
  if (expression.operator === 'at-most') return Number(actual) <= Number(expected);
  if (expression.operator === 'in' && Array.isArray(expected)) return expected.map(String).some((value) => value.toLocaleLowerCase() === left);
  if (expression.operator === 'not-in' && Array.isArray(expected)) return !expected.map(String).some((value) => value.toLocaleLowerCase() === left);
  return left === right;
}

function fixtureWorkspacePresentation(value: string): LibraryPresentation {
  return value === 'compact' ? 'compact-grid' : ['shelves', 'grid', 'compact-grid', 'list', 'table', 'facets'].includes(value) ? value as LibraryPresentation : 'grid';
}

function fixtureWorkspaceSort(items: MediaItem[], sorts: BrowseSort[]) {
  const valueFor = (item: MediaItem, field: string): string | number => {
    if (field === 'year' || field === 'dateAdded') return item.year;
    if (field === 'criticRating') return item.criticRating ?? 0;
    if (field === 'seasonNumber') return item.seasonNumber ?? 0;
    if (field === 'episodeNumber') return item.episodeNumber ?? 0;
    if (field === 'trackNumber') return Number(item.typedMetadata?.trackNumber ?? 0);
    if (field === 'album') return item.parentTitle ?? item.subtitle;
    return normalizedTitle(item.sortTitle || item.title);
  };
  return [...items].sort((left, right) => {
    for (const sort of sorts) {
      const a = valueFor(left, sort.field);
      const b = valueFor(right, sort.field);
      const result = typeof a === 'number' && typeof b === 'number'
        ? a - b
        : String(a).localeCompare(String(b), undefined, { numeric: true, sensitivity: 'base' });
      if (result !== 0) return sort.direction === 'desc' ? -result : result;
    }
    return left.id.localeCompare(right.id);
  });
}

function fixtureSavedResourceMedia(
  resource: SavedResourceSummary,
  kind: 'collection' | 'playlist',
  libraryKind: LibraryBrowseCapabilities['library']['kind'],
): MediaItem {
  return {
    id: resource.id,
    libraryId: resource.libraryId,
    title: resource.title,
    subtitle: `${resource.itemCount} ${resource.itemCount === 1 ? 'item' : 'items'}`,
    year: 0,
    type: libraryKind === 'movies' ? 'movie' : libraryKind === 'music' || libraryKind === 'audiobooks' ? 'music' : 'show',
    kind,
    poster: '/brand/portico-wordmark-white.svg',
    backdrop: '/brand/portico-wordmark-white.svg',
    rating: '',
    length: '',
    genre: '',
    summary: resource.summary,
    availability: 'available',
    actions: resource.canEdit ? ['play', `${kind}.edit`, `${kind}.delete`] : ['play'],
  };
}

function mergeFixturePreference<T extends Record<string, unknown>>(current: T, changes: Record<string, unknown>): T {
  const next = structuredClone(current) as Record<string, unknown>;
  for (const [key, value] of Object.entries(changes)) {
    if (value && typeof value === 'object' && !Array.isArray(value) && '$clear' in value) delete next[key];
    else if (value && typeof value === 'object' && !Array.isArray(value) && next[key] && typeof next[key] === 'object' && !Array.isArray(next[key])) {
      next[key] = mergeFixturePreference(next[key] as Record<string, unknown>, value as Record<string, unknown>);
    } else next[key] = structuredClone(value);
  }
  return next as T;
}

export class FixturePorticoDataSource implements PorticoDataSource {
  private readonly fixtureSettings = new FixtureSettingsDataSource();
  private readonly fixtureFilesystem: FilesystemPickerSource = (() => {
    const roots = [
      { name: 'Media volume', path: '/media' },
      { name: 'Archive storage', path: '/mnt/archive' },
    ];
    return new FixtureFilesystemSource({
      roots,
      defaultPath: '/media',
      directories: [
        fixtureDirectory('/media', [{ name: 'movies' }, { name: 'music' }, { name: 'tv' }], roots),
        fixtureDirectory('/media/movies', [], roots),
        fixtureDirectory('/media/music', [], roots),
        fixtureDirectory('/media/tv', [], roots),
        fixtureDirectory('/mnt/archive', [{ name: 'Classics' }], roots),
        fixtureDirectory('/mnt/archive/Classics', [], roots),
      ],
    });
  })();
  private fixtureWatchWithFriends?: FixtureWatchWithFriendsSource;

  settingsDataSource(): SettingsDataSource {
    return this.fixtureSettings;
  }

  filesystemSource(): FilesystemPickerSource {
    return this.fixtureFilesystem;
  }

  watchWithFriendsSource(viewer: WatchWithFriendsViewer): WatchWithFriendsSource {
    this.fixtureWatchWithFriends ??= new FixtureWatchWithFriendsSource({
      viewer,
      mediaTitles: Object.fromEntries([...media, ...musicMedia].map((item) => [item.id, item.title])),
    });
    return this.fixtureWatchWithFriends;
  }

  private currentViewer: Viewer;
	private readonly browserAccountViewers = new Map<string, Viewer>();
	private readonly browserAccountLastUsed = new Map<string, string>();
	private automaticSignIn = true;
  private readonly mediaState = new Map<string, Pick<MediaItem, 'watchlisted' | 'watched' | 'favorite' | 'reaction' | 'userRating'>>();
  private readonly mediaMetadata = new Map<string, Partial<MediaItem>>();
  private readonly mediaArtwork = new Map<string, MediaImage[]>();
  private readonly deletedMediaIds = new Set<string>();
  private readonly optimizedJobs = new Map<string, MediaJob>();
  private mediaJobSequence = 0;
  private fixturePlayback?: PlaybackResponse;
  private fixtureQueue?: PlaybackSessionQueueResponse;
  private readonly liveFavorites = new Set(['fixture-channel-1']);
  private readonly deletedDVRRecordings = new Set<string>();
  private readonly deletedDVRRules = new Set<string>();
  private readonly fixtureDVRRuleState = new Map<string, Partial<ActionableDVRRule>>();
  private readonly saved = new Map<SavedResourceKind, Map<string, SavedResourceSummary>>();
  private readonly savedItems = new Map<string, string[]>();
  private readonly savedPlaylistEntries = new Map<string, Array<{ entryId: string; mediaId: string }>>();
  private webPreferences: Record<string, unknown> = {
    pinnedLibraryIds: ['fixture-tv', 'fixture-music', 'fixture-movies'],
  };
  private preferenceBundle?: ServerViewerPreferenceBundle;
  private profiles: ServerManagedProfileDirectory['profiles'] = [];
  private notificationRevision = 1;
  private notifications: NotificationPage['items'] = [];
  private feedback: ServerOwnerFeedbackRecord[] = [];
  private fixtureSearchHistory: SearchHistoryItem[] = [];

  constructor(initialViewer: Viewer = {
    authenticated: true,
    setupRequired: false,
    serverName: 'EhlerFlix Test',
    user: {
      id: 'fixture-owner', displayName: 'Portico Review', email: 'review@portico.local', role: 'owner',
      permissions: { manageServer: true, manageLibraries: true, manageUsers: true, editMetadata: true, deleteMedia: true, playMedia: true },
      preferences: { sidebarOrder: ['library:fixture-tv', 'library:fixture-music', 'library:fixture-movies'] },
    },
	viewerScope: { authority: 'local', accountId: 'fixture-owner', serverId: 'fixture-server', profileId: 'fixture-owner-profile', authorizationRevision: 'fixture-policy-1' },
  }) {
	const initialAccountId = initialViewer.viewerScope?.accountId ?? initialViewer.user?.id ?? 'fixture-owner';
	this.currentViewer = structuredClone({
		...initialViewer,
		viewerScope: initialViewer.viewerScope ?? {
			authority: initialViewer.user?.authProvider === 'portico' ? 'hosted' : 'local',
			accountId: initialAccountId,
			serverId: 'fixture-server',
			profileId: `${initialAccountId}-profile`,
			authorizationRevision: 'fixture-policy-1',
		},
	});
	if (initialViewer.user) {
		this.browserAccountViewers.set(initialViewer.user.id, structuredClone({ ...initialViewer, authenticated: true }));
		this.browserAccountLastUsed.set(initialViewer.user.id, '2026-07-10T17:18:00.000Z');
	}
	const secondaryViewer: Viewer = {
		authenticated: true,
		setupRequired: false,
		serverName: initialViewer.serverName,
		viewerScope: { authority: 'local', accountId: 'fixture-sam', serverId: 'fixture-server', profileId: 'fixture-sam-profile', authorizationRevision: 'fixture-policy-1' },
		user: {
			id: 'fixture-sam',
			displayName: 'Sam Rivera',
			email: 'sam@portico.local',
			role: 'user',
			authOrigin: 'local',
			authProvider: 'local',
			permissions: { playMedia: true, watchWithFriends: true },
			preferences: { sidebarOrder: ['library:fixture-tv', 'library:fixture-movies'] },
		},
	};
	this.browserAccountViewers.set('fixture-sam', secondaryViewer);
	this.browserAccountLastUsed.set('fixture-sam', '2026-07-08T20:45:00.000Z');
    const scope = this.currentViewer.viewerScope!;
    this.profiles = [
      {
        id: scope.profileId,
        name: this.currentViewer.user?.displayName ?? 'Primary',
        isPrimary: true,
        isAccountAdmin: true,
        hasPIN: false,
        pinRevision: 0,
        sortOrder: 0,
        policy: structuredClone(unrestrictedProfilePolicy),
      },
      {
        id: 'fixture-family-profile',
        name: 'Family',
        isPrimary: false,
        isAccountAdmin: false,
        hasPIN: true,
        pinRevision: 1,
        sortOrder: 1,
        policy: structuredClone(unrestrictedProfilePolicy),
      },
    ];
    const now = '2026-07-15T14:00:00.000Z';
    this.notifications = [{
      id: 'fixture-notification-1',
      recipient: { authority: scope.authority, accountId: scope.accountId, serverId: scope.serverId, profileId: scope.profileId, audience: 'profile' },
      kind: 'server.message', severity: 'informational', messageId: 'notification.server-message', iconId: 'status.notification',
      interpolation: {}, content: { title: 'Server update', body: 'Your Portico server will restart tonight after active streams finish.' },
      actions: [{ id: 'open-notifications', kind: 'navigate', target: 'notifications', labelMessageId: 'action.open', parameters: {} }],
      createdAt: now, readAt: null, archivedAt: null,
    }];
    this.mediaMetadata.set('fargo', {
      fileCount: 1,
      missingFileCount: 0,
      streams: [
        { id: 'fargo-video-1', kind: 'video', codec: 'hevc', displayTitle: '4K HEVC HDR', bitrate: 18_700_000, width: 3840, height: 2160, dynamicRange: 'HDR10', bitDepth: 10, profile: 'Main 10' },
        { id: 'fargo-audio-1', kind: 'audio', codec: 'eac3', displayTitle: 'English E-AC-3 5.1', bitrate: 640_000, channels: 6, language: 'en' },
        { id: 'fargo-subtitle-1', kind: 'subtitle', codec: 'webvtt', displayTitle: 'English (SDH)', language: 'en', sourceUrl: '/fixture/fargo-en.vtt', subtitleOffsetMs: 0 },
      ],
      attachments: [
        { id: 'fargo-font-1', streamId: 'fargo-attachment-1', filename: 'Inter-SemiBold.ttf', mimeType: 'font/ttf', codec: 'ttf', sizeBytes: 182_400, url: 'data:font/ttf;base64,' } satisfies MediaAttachment,
      ],
      optimizedVersions: [
        { id: 'fargo-optimized-720p', profile: '720p-medium', profileName: '720p balanced', sizeBytes: 1_480_000_000, available: true, createdAt: '2026-07-03T12:00:00.000Z', updatedAt: '2026-07-03T12:00:00.000Z' },
      ],
    });
    this.mediaMetadata.set('track-kiara', {
      fileCount: 1,
      missingFileCount: 0,
      streams: [
        { id: 'kiara-audio-1', kind: 'audio', codec: 'flac', displayTitle: 'FLAC 24-bit stereo', bitrate: 1_120_000, channels: 2, language: 'und', bitDepth: 24 },
      ],
      lyrics: [
        { id: 'kiara-lyrics-1', source: 'provider', provider: 'lrclib', format: 'lrc', synced: true, language: 'en', createdAt: '2026-07-02T10:00:00.000Z' } satisfies MediaLyric,
      ],
    });
    media.slice(2, 6).forEach((item) => this.mediaState.set(item.id, { watchlisted: true, watched: false }));
    media.slice(4, 8).forEach((item) => this.mediaState.set(item.id, { ...this.mediaState.get(item.id), favorite: true }));
    const stamp = '2026-07-01T12:00:00.000Z';
    const ownerUserId = this.currentViewer.user?.id ?? 'fixture-owner';
    const playlist: SavedResourceSummary = { id: 'fixture-playlist-weekend', kind: 'playlist', ownerUserId, title: 'Weekend queue', summary: 'Films and episodes saved for an unrushed weekend.', itemCount: 6, canEdit: true, visibility: 'private', shares: [{ userId: 'fixture-sam', displayName: 'Sam Rivera', canEdit: false }], createdAt: stamp, updatedAt: stamp };
    const collection: SavedResourceSummary = { id: 'fixture-collection-modern-classics', kind: 'collection', ownerUserId, title: 'Modern classics', summary: 'A personal collection of films worth returning to.', itemCount: 4, canEdit: true, visibility: 'server', shares: [], createdAt: stamp, updatedAt: stamp };
    const view: SavedResourceSummary = { id: 'fixture-view-unwatched-movies', kind: 'view', title: 'Unwatched movies', itemCount: 0, canEdit: true, libraryId: 'fixture-movies', libraryName: 'Movies', pivot: 'movies', isPinned: true, createdAt: stamp, updatedAt: stamp };
    this.saved.set('playlist', new Map([[playlist.id, playlist]]));
    this.saved.set('collection', new Map([[collection.id, collection]]));
    this.saved.set('view', new Map([[view.id, view]]));
    this.savedPlaylistEntries.set(playlist.id, [...media.slice(0, 5).map((item) => item.id), media[0].id].map((mediaId, index) => ({ entryId: `fixture-playlist-entry-${index + 1}`, mediaId })));
    this.savedItems.set(`collection:${collection.id}`, media.slice(4, 8).map((item) => item.id));
  }

  private items() {
    return media
      .filter((item) => !this.deletedMediaIds.has(item.id))
      .map((item, index) => ({ ...item, addedAt: new Date(Date.UTC(2026, 5, 30 - index)).toISOString(), libraryId: item.libraryId ?? (item.type === 'movie' ? 'fixture-movies' : 'fixture-tv'), ...this.mediaMetadata.get(item.id), actions: ['play', 'download', 'watchlist.add', 'watchlist.remove', 'favorite.add', 'favorite.remove', 'watched.set', 'reaction.set', 'rating.set', 'collection.add', 'playlist.add', 'feedback.report-problem', 'feedback.request-higher-quality', 'metadata.edit', 'metadata.refresh', 'media.analyze', 'media.optimize', 'media.delete'], ...this.mediaState.get(item.id), mediaImages: this.fixtureArtwork(item) }));
  }

  private allItems() {
    return [...this.items(), ...musicMedia
      .filter((item) => !this.deletedMediaIds.has(item.id))
      .map((item, index) => ({ ...item, addedAt: new Date(Date.UTC(2026, 4, 28 - index)).toISOString(), libraryId: item.libraryId ?? 'fixture-music', ...this.mediaMetadata.get(item.id), actions: ['play', 'download', 'watchlist.add', 'watchlist.remove', 'favorite.add', 'favorite.remove', 'watched.set', 'reaction.set', 'rating.set', 'collection.add', 'playlist.add', 'feedback.report-problem', 'feedback.request-higher-quality', 'metadata.edit', 'metadata.refresh', 'media.analyze', 'media.optimize', 'media.delete'], ...this.mediaState.get(item.id), mediaImages: this.fixtureArtwork(item) }))];
  }

  private fixtureArtwork(item: Pick<MediaItem, 'id' | 'poster' | 'backdrop'>): MediaImage[] {
    const existing = this.mediaArtwork.get(item.id);
    if (existing) return structuredClone(existing);
    const createdAt = '2026-07-01T12:00:00.000Z';
    const images: MediaImage[] = [
      { id: `${item.id}-poster`, type: 'poster', source: 'provider', provider: 'fixture', width: 1000, height: 1500, sortOrder: 0, preferred: true, createdAt },
      { id: `${item.id}-backdrop`, type: 'backdrop', source: 'provider', provider: 'fixture', width: 1920, height: 1080, sortOrder: 0, preferred: true, createdAt },
    ];
    this.mediaArtwork.set(item.id, images);
    return structuredClone(images);
  }

  private homeRows(): HomeRow[] {
    const items = this.items();
    return [
      { id: 'continue-watching', title: 'Continue Watching', explanation: 'Pick up where you left off', detail: '4 in progress', type: 'poster', kind: 'continue_watching', items: [items[0], items[1], items[10], items[8]], hasMore: false, endpoint: '/api/home/rows/continue-watching', critical: true, cursorCapable: true, defaultVisible: true, priority: 100 },
      { id: 'recently-added', title: 'Recently Added', explanation: this.currentViewer.serverName, detail: this.currentViewer.serverName, type: 'poster', kind: 'recently_added', items: items.slice(2, 9), hasMore: false, endpoint: '/api/home/rows/recently-added', critical: true, cursorCapable: true, defaultVisible: true, priority: 90 },
      { id: 'server-trending', title: 'People on this server are watching', explanation: 'Popular across this server', detail: 'Popular across this server', type: 'poster', kind: 'server_activity', items: [items[6], items[5], items[7], items[11], items[3], items[9]], hasMore: false, endpoint: '/api/home/rows/server-trending', critical: false, cursorCapable: true, defaultVisible: true, privacySensitivity: 'aggregate', policyState: 'enabled', priority: 60 },
    ];
  }

  async authCapabilities(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return {
      setupRequired: this.currentViewer.setupRequired,
      localCredentialsEnabled: true,
      porticoAccountAuthEnabled: true,
      serverFriendlyName: this.currentViewer.serverName,
      publicUserPickerEnabled: false,
      visibleUsers: [],
    };
  }

  async viewer(signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return structuredClone(this.currentViewer);
  }

	async browserAccounts(signal: AbortSignal) {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		const accounts = [...this.browserAccountViewers.values()].flatMap((viewer) => viewer.user ? [{
			id: viewer.user.id,
			displayName: viewer.user.displayName,
			profileImageUrl: viewer.user.profileImageUrl,
			authOrigin: viewer.user.authOrigin ?? 'local' as const,
			authProvider: viewer.user.authProvider === 'portico' ? 'portico' as const : 'local' as const,
			lastUsedAt: this.browserAccountLastUsed.get(viewer.user.id) ?? '2026-07-01T12:00:00.000Z',
		}] : []).sort((left, right) => right.lastUsedAt.localeCompare(left.lastUsedAt));
		return {
			accounts,
			activeAccountId: this.currentViewer.authenticated ? this.currentViewer.user?.id : undefined,
			automaticSignIn: this.automaticSignIn,
			selectionRequired: !this.currentViewer.authenticated && !this.automaticSignIn && accounts.length > 0,
			canAddAccount: true,
		};
	}

	async switchBrowserAccount(accountId: string, signal: AbortSignal): Promise<Viewer> {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		const viewer = this.browserAccountViewers.get(accountId);
		if (!viewer) throw new Error('That account is no longer remembered on this browser.');
		this.currentViewer = structuredClone({ ...viewer, authenticated: true });
		this.browserAccountLastUsed.set(accountId, new Date().toISOString());
		return structuredClone(this.currentViewer);
	}

	async updateBrowserAccountPreferences(automaticSignIn: boolean, signal: AbortSignal) {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		this.automaticSignIn = automaticSignIn;
		return { automaticSignIn };
	}

	async removeBrowserAccount(accountId: string, signal: AbortSignal) {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		if (!this.browserAccountViewers.has(accountId)) throw new Error('That account is no longer remembered on this browser.');
		const activeAccountRemoved = this.currentViewer.authenticated && this.currentViewer.user?.id === accountId;
		this.browserAccountViewers.delete(accountId);
		this.browserAccountLastUsed.delete(accountId);
		if (activeAccountRemoved) this.currentViewer = { ...this.currentViewer, authenticated: false, user: undefined };
		return { ok: true, activeAccountRemoved, vaultRevoked: this.browserAccountViewers.size === 0 };
	}

	async signOutAllBrowserAccounts(signal: AbortSignal): Promise<void> {
		if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
		this.browserAccountViewers.clear();
		this.browserAccountLastUsed.clear();
		this.currentViewer = { ...this.currentViewer, authenticated: false, user: undefined };
	}

  async beginLocalProfileLogin(
    credentials: { login: string; password: string; rememberOnBrowser?: boolean },
    signal: AbortSignal,
  ): Promise<LocalProfileLoginChallenge> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!credentials.login.trim() || !credentials.password) throw new Error('Enter a login and password.');
    return {
      accountAuthenticationToken: 'fixture-local-account-proof',
      expiresAt: new Date(Date.now() + 300_000).toISOString(),
      installationId: 'fixture-web-installation',
      rememberOnBrowser: credentials.rememberOnBrowser !== false,
      directory: {
        authority: 'local',
        accountId: 'fixture-owner',
        serverId: 'fixture-server',
        profilesAllowed: true,
        profiles: [{
          id: 'fixture-owner-profile',
          name: this.currentViewer.user?.displayName ?? 'Owner',
          isPrimary: true,
          isAccountAdmin: true,
          hasPIN: false,
          pinRevision: 0,
          sortOrder: 0,
          policy: structuredClone(unrestrictedProfilePolicy),
        }],
      },
    };
  }

  async verifyLocalProfileSelection(
    challenge: LocalProfileLoginChallenge,
    profileId: string,
    _pin: string | undefined,
    signal: AbortSignal,
  ): Promise<LocalProfileSelection> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (challenge.accountAuthenticationToken !== 'fixture-local-account-proof' || profileId !== 'fixture-owner-profile') {
      throw new Error('The fixture profile selection is invalid.');
    }
    return {
      challenge,
      grant: {
        token: 'fixture-local-selection-grant',
        authority: 'local',
        accountId: challenge.directory.accountId,
        serverId: challenge.directory.serverId,
        profileId,
        pinRevision: 0,
        installationId: challenge.installationId,
        expiresAt: new Date(Date.now() + 120_000).toISOString(),
      },
    };
  }

  async publishLocalProfileSession(selection: LocalProfileSelection, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const {challenge, grant} = selection;
    if (grant.token !== 'fixture-local-selection-grant') throw new Error('The fixture profile grant is invalid.');
    this.currentViewer = {
      ...this.currentViewer,
      authenticated: true,
      viewerScope: {
        authority: 'local',
        accountId: challenge.directory.accountId,
        serverId: challenge.directory.serverId,
        profileId: grant.profileId,
        authorizationRevision: 'fixture-policy-1',
      },
    };
    if (challenge.rememberOnBrowser && this.currentViewer.user) {
      this.browserAccountViewers.set(this.currentViewer.user.id, structuredClone(this.currentViewer));
      this.browserAccountLastUsed.set(this.currentViewer.user.id, new Date().toISOString());
    }
    return structuredClone(this.currentViewer);
  }

  async switchAuthenticatedLocalProfile(profileId: string, pin: string | undefined, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw signal.reason ?? new DOMException('Request aborted', 'AbortError');
    return this.switchLocalProfile({ login: 'fixture', password: 'fixture', profileId, ...(pin ? { pin } : {}) }, signal);
  }

  async login(credentials: { login: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!credentials.login.trim() || !credentials.password) throw new Error('Enter a login and password.');
	const remembered = [...this.browserAccountViewers.values()].find((viewer) => viewer.user && [viewer.user.email, viewer.user.displayName, viewer.user.id].some((value) => value.toLocaleLowerCase().includes(credentials.login.trim().toLocaleLowerCase())));
	if (remembered) this.currentViewer = structuredClone({ ...remembered, authenticated: true });
	else this.currentViewer = { ...this.currentViewer, authenticated: true };
	if (credentials.rememberOnBrowser !== false && this.currentViewer.user) {
		this.browserAccountViewers.set(this.currentViewer.user.id, structuredClone(this.currentViewer));
		this.browserAccountLastUsed.set(this.currentViewer.user.id, new Date().toISOString());
	}
    return structuredClone(this.currentViewer);
  }

  async completeLocalProfileSignIn(input: { profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer> {
    return this.switchLocalProfile({ login: 'fixture', password: 'fixture', ...input }, signal);
  }

  cancelLocalProfileSignIn(): void {}

  async setup(details: { serverName: string; username: string; email: string; displayName: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!details.username.trim() || !details.email.trim() || !details.displayName.trim() || !details.password) throw new Error('Complete every setup field.');
    this.currentViewer = {
      authenticated: true,
      setupRequired: false,
      serverName: details.serverName.trim(),
	  viewerScope: { authority: 'local', accountId: 'fixture-owner', serverId: 'fixture-server', profileId: 'fixture-owner-profile', authorizationRevision: 'fixture-policy-1' },
      user: {
        id: 'fixture-owner', displayName: details.displayName.trim(), email: details.email.trim(), role: 'owner',
        permissions: { manageServer: true, manageLibraries: true, manageUsers: true, editMetadata: true, deleteMedia: true, playMedia: true },
        preferences: { sidebarOrder: [] },
      },
    };
	if (details.rememberOnBrowser !== false && this.currentViewer.user) {
		this.browserAccountViewers.set(this.currentViewer.user.id, structuredClone(this.currentViewer));
		this.browserAccountLastUsed.set(this.currentViewer.user.id, new Date().toISOString());
	}
    return structuredClone(this.currentViewer);
  }

  async startPorticoSetup(serverName: string, signal: AbortSignal): Promise<{ claimUrl: string; expiresAt?: string }> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
	this.currentViewer = { ...this.currentViewer, serverName: serverName.trim() };
    return { claimUrl: 'https://app.getportico.tv/claim/fixture', expiresAt: new Date(Date.now() + 600_000).toISOString() };
  }

  async porticoSetupStatus(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return {
      setupRequired: false,
      remoteAccess: {
        settings: { enabled: true, claimStatus: 'claimed', serverId: 'fixture-server', assignedHostname: 'fixture.direct.getportico.tv' },
        porticoConnected: true,
      },
    } as Awaited<ReturnType<PorticoDataSource['porticoSetupStatus']>>;
  }

  async logout(signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.currentViewer = { ...this.currentViewer, authenticated: false };
  }

  async updateProfile(profile: { displayName: string; email: string }, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!this.currentViewer.user) throw new Error('Sign in to update this profile.');
    this.currentViewer = { ...this.currentViewer, user: { ...this.currentViewer.user, ...profile } };
    return structuredClone(this.currentViewer);
  }

  async viewerPreferences(signal: AbortSignal): Promise<ServerViewerPreferenceBundle> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!this.preferenceBundle) {
      const scope = this.currentViewer.viewerScope!;
      const devicePreferences = defaultProfileDeviceClassPreferences('web') as ServerViewerPreferenceBundle['profileDeviceClass']['values'];
      devicePreferences.navigation.pinnedLibraryIds = Array.isArray(this.webPreferences.pinnedLibraryIds) ? this.webPreferences.pinnedLibraryIds.map(String) : [];
      this.preferenceBundle = {
        identity: { ...scope, deviceClass: 'web', installationId: 'fixture-web-installation' },
        profileServer: { version: 'v1', revision: 1, values: structuredClone(defaultProfileServerPreferences) as ServerViewerPreferenceBundle['profileServer']['values'] },
        profileDeviceClass: { version: 'v1', revision: 1, values: structuredClone(devicePreferences) },
        effectiveProfileDeviceClass: { version: 'v1', revision: 1, values: structuredClone(devicePreferences) },
        accountServerInstallation: { version: 'v1', revision: 1, values: defaultAccountServerInstallationPreferences('web') },
        policy: { downloadsAllowed: true, cellularQualityAllowed: true, feedbackAllowed: true },
        clampedFields: [],
      };
    }
    return structuredClone(this.preferenceBundle!);
  }

  async patchViewerPreference(
    scope: 'profile-server' | 'profile-device-class' | 'account-server-installation',
    expectedRevision: number,
    changes: Record<string, unknown>,
    signal: AbortSignal,
  ): Promise<ServerPatchedPreferenceDocument> {
    const bundle = await this.viewerPreferences(signal);
    const key = scope === 'profile-server' ? 'profileServer' : scope === 'profile-device-class' ? 'profileDeviceClass' : 'accountServerInstallation';
    const current = bundle[key];
    if (current.revision !== expectedRevision) throw new ApiError(409, 'preference_revision_conflict', 'Refresh these preferences before saving again.');
    const updated = { ...current, revision: current.revision + 1, values: mergeFixturePreference(current.values, changes) } as ServerPatchedPreferenceDocument;
    if (key === 'profileServer') bundle.profileServer = updated as typeof bundle.profileServer;
    if (key === 'profileDeviceClass') {
      bundle.profileDeviceClass = updated as typeof bundle.profileDeviceClass;
      bundle.effectiveProfileDeviceClass = structuredClone(bundle.profileDeviceClass);
      this.webPreferences = { ...this.webPreferences, pinnedLibraryIds: [...bundle.profileDeviceClass.values.navigation.pinnedLibraryIds] };
    }
    if (key === 'accountServerInstallation') bundle.accountServerInstallation = updated as typeof bundle.accountServerInstallation;
    this.preferenceBundle = bundle;
    return structuredClone(updated);
  }

  async accountProfiles(signal: AbortSignal): Promise<ServerManagedProfileDirectory> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const scope = this.currentViewer.viewerScope!;
    return { authority: scope.authority, accountId: scope.accountId, serverId: scope.serverId, profilesAllowed: true, profiles: structuredClone(this.profiles), canManage: this.profiles.some((profile) => profile.id === scope.profileId && profile.isPrimary) };
  }

  async createProfileAdministrationProof(input: ProfileAdministrationProofInput, signal: AbortSignal): Promise<ServerProfileAdministrationProofResponse> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!input.password && !input.pin) throw new ApiError(401, 'profile_admin_step_up_required', 'Confirm the primary profile first.');
    return { token: 'fixture-profile-proof', expiresAt: new Date(Date.now() + 300_000).toISOString() };
  }

  async porticoProfileMFAStatus(signal: AbortSignal): Promise<{ enabled: boolean }> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return { enabled: false };
  }

  async requestPorticoProfileRecoveryEmail(signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
  }

  async createAccountProfile(input: ServerManagedProfileCreateRequest, proof: string, signal: AbortSignal) {
    await this.createProfileAdministrationProof({ password: proof }, signal);
    const profile = { id: `fixture-profile-${crypto.randomUUID()}`, name: input.name.trim(), avatar: input.avatar, isPrimary: false, isAccountAdmin: false, hasPIN: Boolean(input.pin), pinRevision: input.pin ? 1 : 0, sortOrder: this.profiles.length, policy: structuredClone(input.policy) };
    this.profiles.push(profile);
    return structuredClone(profile);
  }

  async updateAccountProfile(profileId: string, input: ServerManagedProfileUpdateRequest, proof: string, signal: AbortSignal) {
    await this.createProfileAdministrationProof({ password: proof }, signal);
    const index = this.profiles.findIndex((profile) => profile.id === profileId);
    if (index < 0) throw new ApiError(404, 'profile_not_found', 'This profile is no longer available.');
    this.profiles[index] = { ...this.profiles[index], ...input };
    return structuredClone(this.profiles[index]);
  }

  async deleteAccountProfile(profileId: string, proof: string, signal: AbortSignal) {
    await this.createProfileAdministrationProof({ password: proof }, signal);
    const profile = this.profiles.find((item) => item.id === profileId);
    if (!profile || profile.isPrimary) throw new ApiError(409, 'profile_conflict', 'The primary profile cannot be removed.');
    this.profiles = this.profiles.filter((item) => item.id !== profileId).map((item, index) => ({ ...item, sortOrder: index }));
  }

  async reorderAccountProfiles(profileIds: string[], proof: string, signal: AbortSignal) {
    await this.createProfileAdministrationProof({ password: proof }, signal);
    if (profileIds.length !== this.profiles.length || profileIds.some((id) => !this.profiles.some((profile) => profile.id === id))) throw new Error('Profile order is incomplete.');
    this.profiles = profileIds.map((id, index) => ({ ...this.profiles.find((profile) => profile.id === id)!, sortOrder: index }));
    return this.accountProfiles(signal);
  }

  async setAccountProfilePin(profileId: string, input: { pin: string } & ProfilePINReauthentication, proof: string, signal: AbortSignal) {
    if (!/^\d{4}$/.test(input.pin)) throw new Error('Enter a four-digit PIN.');
    if (!input.password) throw new ApiError(401, 'invalid_credentials', 'Your current account password is incorrect.');
    await this.createProfileAdministrationProof({ password: proof }, signal);
    const profile = this.profiles.find((item) => item.id === profileId);
    if (!profile) throw new Error('This profile is no longer available.');
    profile.hasPIN = true;
    profile.pinRevision += 1;
  }

  async clearAccountProfilePin(profileId: string, input: ProfilePINReauthentication, proof: string, signal: AbortSignal) {
    if (!input.password) throw new ApiError(401, 'invalid_credentials', 'Your current account password is incorrect.');
    await this.createProfileAdministrationProof({ password: proof }, signal);
    const profile = this.profiles.find((item) => item.id === profileId);
    if (!profile) throw new Error('This profile is no longer available.');
    profile.hasPIN = false;
    profile.pinRevision += 1;
  }

  async createAutomaticProfileTrust(signal: AbortSignal): Promise<AutomaticProfileTrust> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const scope = this.currentViewer.viewerScope!;
    const profile = this.profiles.find((item) => item.id === scope.profileId)!;
    return { version: 'v1', purpose: 'automatic-profile-selection', token: 'fixture-trust', ...scope, pinRevision: profile.pinRevision, installationId: 'fixture-web-installation', expiresAt: new Date(Date.now() + 7_776_000_000).toISOString() };
  }

  async revokeAutomaticProfileTrust(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
  }

  async switchLocalProfile(input: { login: string; password: string; profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!input.login.trim() || !input.password) throw new ApiError(401, 'invalid_credentials', 'Enter the account login and password.');
    const profile = this.profiles.find((item) => item.id === input.profileId);
    if (!profile) throw new ApiError(404, 'profile_not_found', 'This profile is no longer available.');
    if (profile.hasPIN && input.pin !== '1234') throw new ApiError(401, 'local_profile_pin_invalid', 'That profile PIN is not correct.');
    this.currentViewer = { ...this.currentViewer, viewerScope: { ...this.currentViewer.viewerScope!, profileId: profile.id }, user: this.currentViewer.user ? { ...this.currentViewer.user, displayName: profile.name } : undefined };
    return structuredClone(this.currentViewer);
  }

  async switchHostedProfile(input: { profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer> {
    return this.switchLocalProfile({ login: 'fixture', password: 'fixture', ...input }, signal);
  }

  async viewerNotifications(audience: NotificationAudience, cursor: string | undefined, signal: AbortSignal): Promise<NotificationPage> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const scope = this.currentViewer.viewerScope!;
    const items = this.notifications.filter((item) => item.recipient.audience === audience && !item.archivedAt);
    return { recipient: audience === 'profile' ? { authority: scope.authority, accountId: scope.accountId, serverId: scope.serverId, profileId: scope.profileId, audience } : { authority: scope.authority, accountId: scope.accountId, serverId: scope.serverId, audience }, items: cursor ? [] : structuredClone(items), unreadCount: items.filter((item) => !item.readAt).length, revision: this.notificationRevision, pageInfo: { hasMore: false, nextCursor: null } };
  }

  async watchViewerNotificationInvalidations(_audience: NotificationAudience, _onInvalidation: (event: NotificationInvalidation) => void, signal: AbortSignal): Promise<void> {
    if (signal.aborted) return;
    await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }));
  }

  async watchApplicationEvents(_onEvent: Parameters<PorticoDataSource['watchApplicationEvents']>[0], _onReset: Parameters<PorticoDataSource['watchApplicationEvents']>[1], signal: AbortSignal): Promise<void> {
    if (signal.aborted) return;
    await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve(), { once: true }));
  }

  async updateViewerNotificationReceipts(audience: NotificationAudience, input: { recipient: NotificationPage['recipient']; notificationIds: string[]; action: NotificationReceiptAction; expectedRevision: number }, signal: AbortSignal): Promise<NotificationReceiptResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (input.expectedRevision !== this.notificationRevision) throw new ApiError(409, 'notification_receipt_conflict', 'Refresh notifications before trying again.');
    const timestamp = new Date().toISOString();
    this.notifications = this.notifications.map((item) => input.notificationIds.includes(item.id) ? { ...item, readAt: input.action === 'mark-read' ? timestamp : input.action === 'mark-unread' ? null : item.readAt, archivedAt: input.action === 'archive' ? timestamp : item.archivedAt } : item);
    this.notificationRevision += 1;
    const page = await this.viewerNotifications(audience, undefined, signal);
    return { recipient: input.recipient, receipts: this.notifications.filter((item) => input.notificationIds.includes(item.id)).map((item) => ({ notificationId: item.id, readAt: item.readAt ?? null, archivedAt: item.archivedAt ?? null })), unreadCount: page.unreadCount, revision: this.notificationRevision };
  }

  async markAllViewerNotificationsRead(audience: NotificationAudience, signal: AbortSignal) {
    const page = await this.viewerNotifications(audience, undefined, signal);
    const timestamp = new Date().toISOString();
    this.notifications = this.notifications.map((item) => item.recipient.audience === audience ? { ...item, readAt: item.readAt ?? timestamp } : item);
    this.notificationRevision += 1;
    return { recipient: page.recipient, unreadCount: 0, revision: this.notificationRevision };
  }

  async viewerFeedbackCapabilities(signal: AbortSignal): Promise<ViewerFeedbackCapabilities> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return { version: 'v1', enabled: true, allowedKinds: ['general', 'playback', 'media', 'quality'], messageMaxLength: 1000, retentionDays: 90 };
  }

  async submitViewerFeedback(input: ViewerFeedbackSubmission, signal: AbortSignal): Promise<ViewerFeedbackReceipt> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const scope = this.currentViewer.viewerScope!;
    const now = new Date().toISOString();
    const reporter = { authority: scope.authority, accountName: this.currentViewer.user?.displayName ?? 'Viewer', accountId: scope.accountId } as unknown as ServerOwnerFeedbackRecord['reporter'];
    const record: ServerOwnerFeedbackRecord = { id: `fixture-feedback-${crypto.randomUUID()}`, reporter, kind: input.kind, category: input.category, status: 'new', message: input.message, diagnostics: { deviceClass: 'web', platform: input.context.platform, appVersion: input.context.appVersion, mediaId: input.context.mediaId, occurredAt: now }, duplicateCount: 0, submittedAt: now, updatedAt: now, revision: 1 };
    this.feedback.unshift(record);
    return { id: record.id, status: 'new', submittedAt: now };
  }

  async ownerViewerFeedback(status: 'new' | 'read' | 'resolved' | 'dismissed' | undefined, cursor: string | undefined, signal: AbortSignal): Promise<ServerOwnerFeedbackPage> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const items = status ? this.feedback.filter((item) => item.status === status) : this.feedback;
    return { items: cursor ? [] : structuredClone(items), pageInfo: { hasMore: false, nextCursor: null }, statusCounts: { new: this.feedback.filter((item) => item.status === 'new').length, read: this.feedback.filter((item) => item.status === 'read').length, resolved: this.feedback.filter((item) => item.status === 'resolved').length, dismissed: this.feedback.filter((item) => item.status === 'dismissed').length } };
  }

  async ownerViewerNotificationRecipients(signal: AbortSignal): Promise<ServerOwnerNotificationRecipientDirectory> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return {
      profiles: this.profiles.map((profile) => ({
        authority: 'local', audience: 'profile', accountId: this.currentViewer.viewerScope!.accountId,
        profileId: profile.id, accountName: this.currentViewer.user?.displayName ?? 'Local account', profileName: profile.name,
      })),
      accountAdmins: [{
        authority: this.currentViewer.viewerScope!.authority,
        audience: 'account-admin',
        accountId: this.currentViewer.viewerScope!.accountId,
        accountName: this.currentViewer.user?.displayName ?? 'Viewer account',
      }],
    };
  }

  async updateOwnerViewerFeedback(feedbackId: string, input: ServerOwnerFeedbackUpdateRequest, signal: AbortSignal): Promise<ServerOwnerFeedbackRecord> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const index = this.feedback.findIndex((item) => item.id === feedbackId);
    if (index < 0) throw new Error('That message is no longer available.');
    const current = this.feedback[index];
    if (current.revision !== input.expectedRevision) throw new ApiError(409, 'feedback_conflict', 'Refresh this message before updating it.');
    const now = new Date().toISOString();
    this.feedback[index] = { ...current, status: input.status ?? current.status, ownerResponse: input.responseMessage ? { message: input.responseMessage, respondedAt: now } : current.ownerResponse, updatedAt: now, revision: current.revision + 1 };
    return structuredClone(this.feedback[index]);
  }

  async createOwnerViewerNotice(input: ServerOwnerNoticeRequest, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const scope = this.currentViewer.viewerScope!;
    this.notifications.unshift({ id: `fixture-notice-${crypto.randomUUID()}`, recipient: input.audience === 'profile' ? { authority: 'local', accountId: scope.accountId, serverId: scope.serverId, profileId: input.profileId, audience: 'profile' } : { authority: scope.authority, accountId: input.accountId, serverId: scope.serverId, audience: 'account-admin' }, kind: 'server.message', severity: input.severity ?? 'informational', messageId: 'notification.server-message', iconId: 'status.notification', interpolation: {}, actions: [], content: { body: input.message }, createdAt: new Date().toISOString(), readAt: null, archivedAt: null });
    this.notificationRevision += 1;
  }

  async searchContract(signal: AbortSignal): Promise<SearchContract> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const groups: SearchContract['groups'] = [
      { id: 'movies', title: 'Movies', entityKind: 'movie', resultKinds: ['movie'], supportsLibraryScope: true, sorts: ['relevance', 'title', 'releaseYear', 'dateAdded'] },
      { id: 'shows', title: 'Shows', entityKind: 'show', resultKinds: ['show', 'season'], supportsLibraryScope: true, sorts: ['relevance', 'title', 'releaseYear', 'dateAdded'] },
      { id: 'episodes', title: 'Episodes', entityKind: 'episode', resultKinds: ['episode'], supportsLibraryScope: true, sorts: ['relevance', 'title', 'releaseYear', 'dateAdded'] },
      { id: 'people', title: 'People', entityKind: 'person', resultKinds: ['person'], supportsLibraryScope: true, sorts: ['relevance', 'title'] },
      { id: 'music', title: 'Music', entityKind: 'track', resultKinds: ['artist', 'album', 'track'], supportsLibraryScope: true, sorts: ['relevance', 'title', 'releaseYear', 'dateAdded'] },
      { id: 'audiobooks', title: 'Audiobooks', entityKind: 'book', resultKinds: ['author', 'audiobook-series', 'book'], supportsLibraryScope: true, sorts: ['relevance', 'title', 'releaseYear', 'dateAdded'] },
      { id: 'live-tv', title: 'Live TV', entityKind: 'live-channel', resultKinds: ['live-channel'], supportsLibraryScope: false, sorts: ['relevance'] },
    ];
    return structuredClone({
      revision: 'fixture-search-v1', endpoint: '/api/search', groupOrder: groups.map((group) => group.id), groups,
      sorts: [
        { id: 'relevance', label: 'Best match', directions: ['desc'], defaultDirection: 'desc', applicableGroups: groups.map((group) => group.id) },
        { id: 'title', label: 'Title', directions: ['asc', 'desc'], defaultDirection: 'asc', applicableGroups: groups.filter((group) => group.id !== 'live-tv').map((group) => group.id) },
        { id: 'releaseYear', label: 'Release year', directions: ['asc', 'desc'], defaultDirection: 'desc', applicableGroups: groups.filter((group) => !['people', 'live-tv'].includes(group.id)).map((group) => group.id) },
        { id: 'dateAdded', label: 'Date added', directions: ['asc', 'desc'], defaultDirection: 'desc', applicableGroups: groups.filter((group) => !['people', 'live-tv'].includes(group.id)).map((group) => group.id) },
      ],
      filters: [
        { id: 'entityKinds', label: 'Result types', valueType: 'enum', multiple: true, allowedValues: [...new Set(groups.flatMap((group) => [group.entityKind, ...group.resultKinds]))] },
        { id: 'libraryIds', label: 'Libraries', valueType: 'identity', multiple: true, source: { endpoint: '/api/libraries', valueField: 'id', labelField: 'name' } },
        { id: 'group', label: 'Result group', valueType: 'enum', multiple: false, allowedValues: groups.map((group) => group.id) },
      ],
      facetMode: 'none',
      limits: { minimumQueryLength: 1, maximumQueryLength: 120, defaultGroupLimit: 6, maximumGroupLimit: 50, quickInitialGroupLimit: 3, quickMaximumGroups: 6, quickMaximumItemsPerGroup: 6, fullDefaultGroupLimit: 50 },
      cursor: { mode: 'independent-group', opaque: true, requiresSingleGroup: true, principalBound: true, scopeFields: ['query', 'group', 'libraryIds', 'sort', 'direction'], ttlSeconds: 900, expiredErrorCode: 'cursor_expired', invalidErrorCode: 'invalid_cursor' },
      resultSemantics: { destinationSource: 'entitySemantics.defaultDestination', hierarchySource: 'entitySemantics.parentKinds+childKinds+childOrder', artworkRoleSource: 'entitySemantics.primaryArtworkRole', kindMappings: [] },
    } satisfies SearchContract);
  }

  async productContract(signal: AbortSignal): Promise<ProductContract> {
    const search = await this.searchContract(signal);
    const presentations: Record<string, { labelMessageId: string; iconId: string; group: string; priority: number; surfaces: string[] }> = {
      play: { labelMessageId: 'action.play', iconId: 'action.play', group: 'playback', priority: 100, surfaces: ['web', 'mobile', 'television'] },
      'play.from-beginning': { labelMessageId: 'action.play-from-beginning', iconId: 'action.restart', group: 'playback', priority: 95, surfaces: ['web', 'mobile', 'television'] },
      'live.play': { labelMessageId: 'action.play', iconId: 'action.play', group: 'playback', priority: 100, surfaces: ['web', 'mobile', 'television'] },
      'dvr.play': { labelMessageId: 'action.play', iconId: 'action.play', group: 'playback', priority: 100, surfaces: ['web', 'mobile', 'television'] },
      download: { labelMessageId: 'action.download', iconId: 'action.download', group: 'playback', priority: 70, surfaces: ['mobile'] },
      'queue.add': { labelMessageId: 'action.add-queue', iconId: 'action.queue', group: 'playback', priority: 60, surfaces: ['web', 'mobile', 'television'] },
      'watchlist.add': { labelMessageId: 'action.add-watchlist', iconId: 'action.watchlist', group: 'saved', priority: 90, surfaces: ['web', 'mobile', 'television'] },
      'watchlist.remove': { labelMessageId: 'action.remove-watchlist', iconId: 'action.watchlist', group: 'saved', priority: 90, surfaces: ['web', 'mobile', 'television'] },
      'favorite.add': { labelMessageId: 'action.add-favorite', iconId: 'action.favorite', group: 'saved', priority: 80, surfaces: ['web', 'mobile', 'television'] },
      'favorite.remove': { labelMessageId: 'action.remove-favorite', iconId: 'action.favorite', group: 'saved', priority: 80, surfaces: ['web', 'mobile', 'television'] },
      'watched.set': { labelMessageId: 'action.mark-watched', iconId: 'action.watched', group: 'state', priority: 70, surfaces: ['web', 'mobile', 'television'] },
      'watched.mark': { labelMessageId: 'action.mark-watched', iconId: 'action.watched', group: 'state', priority: 70, surfaces: ['web', 'mobile', 'television'] },
      'watched.unmark': { labelMessageId: 'action.mark-unwatched', iconId: 'action.watched', group: 'state', priority: 70, surfaces: ['web', 'mobile', 'television'] },
      'reaction.set': { labelMessageId: 'action.react', iconId: 'action.reaction', group: 'state', priority: 50, surfaces: ['web', 'mobile', 'television'] },
      'rating.set': { labelMessageId: 'action.rate', iconId: 'action.rating', group: 'state', priority: 50, surfaces: ['web', 'mobile', 'television'] },
      'collection.add': { labelMessageId: 'action.add-collection', iconId: 'action.collection', group: 'saved', priority: 40, surfaces: ['web', 'mobile', 'television'] },
      'playlist.add': { labelMessageId: 'action.add-playlist', iconId: 'action.playlist', group: 'saved', priority: 40, surfaces: ['web', 'mobile', 'television'] },
      'metadata.edit': { labelMessageId: 'action.edit-metadata', iconId: 'action.metadata', group: 'administration', priority: 50, surfaces: ['web-admin'] },
      'metadata.refresh': { labelMessageId: 'action.refresh-metadata', iconId: 'action.refresh', group: 'administration', priority: 40, surfaces: ['web-admin'] },
      'media.analyze': { labelMessageId: 'action.analyze-media', iconId: 'action.metadata', group: 'administration', priority: 30, surfaces: ['web-admin'] },
      'media.optimize': { labelMessageId: 'action.optimize-media', iconId: 'action.optimize', group: 'administration', priority: 20, surfaces: ['web-admin'] },
      'media.delete': { labelMessageId: 'action.delete-media', iconId: 'action.delete', group: 'administration', priority: 10, surfaces: ['web-admin'] },
      'watch-with-friends.start': { labelMessageId: 'action.watch-with-friends', iconId: 'action.people', group: 'playback', priority: 55, surfaces: ['web', 'mobile', 'television'] },
    };
    const mediaActions = Object.entries(presentations).map(([id, presentation]) => ({
      id, mutating: !['play', 'play.from-beginning', 'live.play', 'download', 'watch-with-friends.start'].includes(id), bulkSupported: !['play', 'play.from-beginning', 'live.play', 'reaction.set', 'rating.set', 'watch-with-friends.start'].includes(id),
      presentation,
      command: { kind: 'client-flow', execution: 'selection', flowId: id, requiredInputs: [], resultHandling: 'flow' },
      confirmation: { required: id === 'media.delete', tone: id === 'media.delete' ? 'destructive' : 'none' }, invalidates: [],
    }));
    return {
      apiVersion: 'v1', actionRevision: 'v1', search,
      libraryKinds: [], entityKinds: [], entitySemantics: [], artworkRoles: [], browseFields: [], browseSorts: [], browseOperators: [], presentationFields: [],
      queryLimits: { maximumDepth: 4, maximumClauses: 24, maximumBytes: 65_536, defaultLimit: 60, maximumLimit: 200, cursorTtlSeconds: 900 },
      mediaActions, serverCapabilities: [],
    } as unknown as ProductContract;
  }

  async uploadClientDiagnostics(_input: Parameters<NonNullable<PorticoDataSource['uploadClientDiagnostics']>>[0], signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
  }

  async home(signal: AbortSignal): Promise<HomeResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return {
      pivots: ['Home'],
      rows: this.homeRows(),
    };
  }

  async webDisplayPreferences(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return { preferences: structuredClone(this.webPreferences), updatedAt: '2026-07-01T12:00:00.000Z' };
  }

  async updateWebDisplayPreferences(preferences: Record<string, unknown>, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.webPreferences = structuredClone(preferences);
    return { preferences: structuredClone(this.webPreferences), updatedAt: new Date().toISOString() };
  }

  async libraryNavigation(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const pinnedLibraryIds = Array.isArray(this.webPreferences.pinnedLibraryIds)
      ? this.webPreferences.pinnedLibraryIds.map(String)
      : [];
    return { pinnedLibraryIds };
  }

  async updateLibraryNavigation(pinnedLibraryIds: string[], signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.webPreferences = { ...this.webPreferences, pinnedLibraryIds: [...pinnedLibraryIds] };
    return { pinnedLibraryIds: [...pinnedLibraryIds] };
  }

  async homeRow(id: string, cursor: string | undefined, signal: AbortSignal, limit = 24): Promise<HomeRow> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const row = this.homeRows().find((candidate) => candidate.id === id);
    if (!row) throw new Error('This Home row is no longer available.');
    const offset = cursor?.startsWith('fixture:') ? Number(cursor.slice('fixture:'.length)) || 0 : 0;
    const items = row.items.slice(offset, offset + limit);
    const nextOffset = offset + items.length;
    const hasMore = nextOffset < row.items.length;
    return { ...row, items, hasMore, nextCursor: hasMore ? `fixture:${nextOffset}` : null };
  }

  async libraries(signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return [
      { id: 'fixture-tv', name: 'TV Shows', kind: 'tv' as const, itemCount: this.items().filter((item) => item.type === 'show').length },
      { id: 'fixture-movies', name: 'Movies', kind: 'movies' as const, itemCount: this.items().filter((item) => item.type === 'movie').length },
      { id: 'fixture-music', name: 'Music', kind: 'music' as const, itemCount: this.allItems().filter((item) => item.type === 'music').length },
    ];
  }

  async libraryBrowseCapabilities(libraryId: string, signal: AbortSignal): Promise<LibraryBrowseCapabilities> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const library = (await this.libraries(signal)).find((candidate) => candidate.id === libraryId);
    if (!library) throw new Error('This library is no longer available.');
    return fixtureBrowseCapabilities(libraryId);
  }

  async libraryFacetOptions(_libraryId: string, facetSource: NonNullable<BrowseFacetSource>, signal: AbortSignal): Promise<BrowseFacetOption[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const field = facetSource.filterPrefix.replace(/:$/, '');
    const values = new Map<string, number>();
    for (const item of this.allItems()) {
      const candidates = field === 'genre' ? item.genre.split(',') : field === 'tag' ? item.tags ?? [] : field === 'label' ? item.labels ?? [] : [];
      for (const candidate of candidates.map((value) => value.trim()).filter(Boolean)) values.set(candidate, (values.get(candidate) ?? 0) + 1);
    }
    return [...values].map(([value, count]) => ({ value, label: value, count })).sort((left, right) => right.count - left.count || left.label.localeCompare(right.label));
  }

  async libraryPivot(input: LibraryPivotInput, signal: AbortSignal): Promise<LibraryPivotPage> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const capabilities = await this.libraryBrowseCapabilities(input.libraryId, signal);
    const pivot = capabilities.pivots.find((candidate) => candidate.id === input.pivot.id);
    if (!pivot) throw new Error(`The ${input.pivot.label} view is no longer available for this library.`);
    const presentation = fixtureWorkspacePresentation(pivot.defaultView);
    const appliedSort = input.request.sort?.length ? input.request.sort : pivot.defaultSort;
    const base = this.allItems().filter((item) => item.libraryId === input.libraryId);

    if (pivot.id === 'discover') {
      const recent = fixtureWorkspaceSort(base, [{ field: 'dateAdded', direction: 'desc' }]);
      const continueItems = base.filter((item) => item.progress != null && item.progress > 0);
      const sections = [
        continueItems.length ? { id: 'continue', title: input.libraryKind === 'music' ? 'Continue listening' : 'Continue watching', items: continueItems } : undefined,
        recent.length ? { id: 'recent', title: 'Recently added', items: recent } : undefined,
      ].filter((section): section is NonNullable<typeof section> => Boolean(section));
      return {
        items: recent,
        sections,
        total: recent.length,
        nextCursor: null,
        hasMore: false,
        applied: { pivot: pivot.id, sort: appliedSort, presentationFields: pivot.presentationFields },
        presentation,
      };
    }

    if (pivot.id === 'collections' || pivot.id === 'playlists') {
      const kind = pivot.id === 'collections' ? 'collection' : 'playlist';
      const items = [...(this.saved.get(kind)?.values() ?? [])]
        .map((resource) => fixtureSavedResourceMedia(resource, kind, capabilities.library.kind));
      return {
        items,
        total: items.length,
        nextCursor: null,
        hasMore: false,
        applied: { pivot: pivot.id, sort: appliedSort, presentationFields: pivot.presentationFields },
        presentation,
      };
    }

    if (pivot.id === 'categories' || pivot.id === 'genres') {
      const groups = new Map<string, MediaItem[]>();
      for (const item of base) {
        for (const value of item.genre.split(',').map((part) => part.trim()).filter(Boolean)) {
          groups.set(value, [...(groups.get(value) ?? []), item]);
        }
      }
      const facets = [...groups.entries()]
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([name, items]) => ({
          id: `fixture-genre-${name.toLocaleLowerCase().replaceAll(/[^a-z0-9]+/g, '-')}`,
          title: name,
          count: items.length,
          artwork: items[0]?.poster,
          query: { field: 'genre', operator: 'contains', value: name } as BrowseExpression,
        }));
      return {
        items: [],
        facets,
        total: facets.length,
        nextCursor: null,
        hasMore: false,
        applied: { pivot: pivot.id, sort: appliedSort, presentationFields: pivot.presentationFields },
        presentation,
      };
    }

    const kinds = new Set(pivot.entityKinds);
    let items: MediaItem[] = base.filter((item) => kinds.size === 0 || kinds.has(item.kind));
    if (input.request.query) items = items.filter((item) => fixturePredicate(item, input.request.query as BrowseExpression));
    items = fixtureWorkspaceSort(items, appliedSort);
    if (input.request.seek?.prefix) {
      const prefix = input.request.seek.prefix.toLocaleUpperCase();
      items = prefix === '#'
        ? items.filter((item) => !/[A-Z]/.test(normalizedTitle(item.sortTitle || item.title).charAt(0).toLocaleUpperCase()))
        : items.filter((item) => normalizedTitle(item.sortTitle || item.title).toLocaleUpperCase() >= prefix);
    }
    const offset = input.request.cursor?.startsWith('fixture-library:')
      ? Number(input.request.cursor.slice('fixture-library:'.length)) || 0
      : 0;
    const limit = Math.min(input.request.limit ?? capabilities.queryLimits.defaultLimit, capabilities.queryLimits.maximumLimit);
    const page = items.slice(offset, offset + limit);
    const nextOffset = offset + page.length;
    const hasMore = nextOffset < items.length;
    return {
      items: page,
      total: items.length,
      nextCursor: hasMore ? `fixture-library:${nextOffset}` : null,
      hasMore,
      applied: { pivot: pivot.id, sort: appliedSort, presentationFields: pivot.presentationFields },
      presentation,
    };
  }

  async createSavedView(input: SavedViewCreateRequest, signal: AbortSignal): Promise<SavedView> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const title = input.title.trim();
    if (!title) throw new Error('Enter a name for this view.');
    const capabilities = await this.libraryBrowseCapabilities(input.libraryId, signal);
    if (!capabilities.pivots.some((pivot) => pivot.id === input.pivot)) throw new Error('This library view is no longer available.');
    const timestamp = new Date().toISOString();
    const id = `fixture-view-${globalThis.crypto.randomUUID()}`;
    const savedView: SavedView = {
      id,
      title,
      libraryId: input.libraryId,
      libraryName: capabilities.library.name,
      ownerUserId: this.currentViewer.user?.id ?? 'fixture-owner',
      pivot: input.pivot,
      presentation: input.presentation ?? { fields: [] },
      query: input.query,
      sort: input.sort ?? [],
      isPinned: input.isPinned ?? false,
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    const resource: SavedResourceSummary = {
      id,
      kind: 'view',
      title,
      itemCount: 0,
      canEdit: true,
      libraryId: input.libraryId,
      libraryName: capabilities.library.name,
      pivot: input.pivot,
      isPinned: input.isPinned ?? false,
      createdAt: timestamp,
      updatedAt: timestamp,
    };
    const resources = this.saved.get('view') ?? new Map<string, SavedResourceSummary>();
    resources.set(id, resource);
    this.saved.set('view', resources);
    return savedView;
  }

  async browseLibrary(input: LibraryBrowseInput, signal: AbortSignal): Promise<LibraryBrowseResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');

    const normalizedPivot = input.pivot.toLocaleLowerCase().replaceAll(' ', '-');
    let items = input.kind === 'music'
      ? normalizedPivot === 'discover'
        ? this.allItems().filter((item) => item.type === 'music')
        : this.allItems().filter((item) => item.type === 'music' && item.kind === normalizedPivot.replace(/s$/, ''))
      : this.items().filter((item) => (input.kind === 'movies' ? item.type === 'movie' : item.type === 'show'));
    if (input.filter === 'In progress') items = items.filter((item) => item.progress != null);
    if (input.filter === 'Unwatched') items = items.filter((item) => item.progress == null);
    if (input.search?.trim()) {
      const query = input.search.trim().toLocaleLowerCase();
      items = items.filter((item) => `${item.title} ${item.subtitle} ${item.genre}`.toLocaleLowerCase().includes(query));
    }
    const sorted = sortItems(items, input);
    return {
      items: sorted,
      total: sorted.length,
      libraryId: input.kind === 'movies' ? 'fixture-movies' : 'fixture-tv',
      hasMore: false,
      nextCursor: null,
      capabilities: {
        apiVersion: 'v1',
        pivots: input.kind === 'music'
          ? [
              { id: 'discover', label: 'Discover', entityKinds: ['album', 'track'], defaultView: 'shelves', supportedViews: ['shelves'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/discover' },
              { id: 'artists', label: 'Artists', entityKinds: ['artist'], defaultView: 'grid', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'albums', label: 'Albums', entityKinds: ['album'], defaultView: 'grid', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'tracks', label: 'Tracks', entityKinds: ['track'], defaultView: 'list', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'playlists', label: 'Playlists', entityKinds: ['playlist'], defaultView: 'list', supportedViews: ['list'], browseSupported: false, endpointTemplate: '/api/playlists?libraryId={libraryId}' },
              { id: 'genres', label: 'Genres', entityKinds: ['category'], defaultView: 'facets', supportedViews: ['facets'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/categories' },
            ]
          : input.kind === 'movies'
          ? [
              { id: 'discover', label: 'Discover', entityKinds: ['movie'], defaultView: 'shelves', supportedViews: ['shelves'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/discover' },
              { id: 'movies', label: 'Movies', entityKinds: ['movie'], defaultView: 'grid', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'collections', label: 'Collections', entityKinds: ['collection'], defaultView: 'grid', supportedViews: ['grid'], browseSupported: false, endpointTemplate: '/api/collections?libraryId={libraryId}' },
              { id: 'categories', label: 'Categories', entityKinds: ['category'], defaultView: 'facets', supportedViews: ['facets'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/categories' },
            ]
          : [
              { id: 'discover', label: 'Discover', entityKinds: ['show'], defaultView: 'shelves', supportedViews: ['shelves'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/discover' },
              { id: 'shows', label: 'Shows', entityKinds: ['show'], defaultView: 'grid', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'episodes', label: 'Episodes', entityKinds: ['episode'], defaultView: 'list', supportedViews: ['grid', 'list'], browseSupported: true, endpointTemplate: '/api/libraries/{libraryId}/browse' },
              { id: 'collections', label: 'Collections', entityKinds: ['collection'], defaultView: 'grid', supportedViews: ['grid'], browseSupported: false, endpointTemplate: '/api/collections?libraryId={libraryId}' },
              { id: 'categories', label: 'Categories', entityKinds: ['category'], defaultView: 'facets', supportedViews: ['facets'], browseSupported: false, endpointTemplate: '/api/libraries/{libraryId}/categories' },
            ],
        sorts: [
          { id: 'title', label: 'Title', defaultDirection: 'asc' },
          { id: 'dateAdded', label: 'Recently added', defaultDirection: 'desc' },
          { id: 'year', label: 'Year', defaultDirection: 'desc' },
          { id: 'criticRating', label: 'Critic rating', defaultDirection: 'desc' },
        ],
        actions: ['play', 'watchlist.add', 'metadata.edit'],
      },
    };
  }

  async search(query: string, signal: AbortSignal, limit = 6): Promise<SearchResult[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const normalized = query.trim().toLocaleLowerCase();
    const results = normalized
      ? this.allItems().filter((item) => `${item.title} ${item.subtitle} ${item.genre}`.toLocaleLowerCase().includes(normalized))
      : this.allItems();
    return results.slice(0, limit);
  }

  async searchPage(input: SearchPageInput, signal: AbortSignal): Promise<SearchPageResult> {
    if (input.recordHistory && input.query.trim()) {
      const query = input.query.trim();
      this.fixtureSearchHistory = [{ query, useCount: 1, lastUsedAt: new Date().toISOString() }, ...this.fixtureSearchHistory.filter((item) => item.query.toLocaleLowerCase() !== query.toLocaleLowerCase())].slice(0, 30);
    }
    const sort = input.sort ?? 'relevance';
    const direction = input.direction ?? (sort === 'title' ? 'asc' : 'desc');
    const matchesKind = (item: MediaItem) => !input.entityKinds?.length || input.entityKinds.includes(String(item.entityKind ?? item.kind));
    const compareKnown = (left: number | string | undefined, right: number | string | undefined, leftId: string, rightId: string) => {
      const leftKnown = left !== undefined && left !== '' && left !== 0;
      const rightKnown = right !== undefined && right !== '' && right !== 0;
      if (leftKnown !== rightKnown) return leftKnown ? -1 : 1;
      if (!leftKnown || !rightKnown) return leftId.localeCompare(rightId);
      const compared = typeof left === 'number' && typeof right === 'number'
        ? left - right
        : String(left).localeCompare(String(right), undefined, { numeric: true, sensitivity: 'base' });
      return (direction === 'asc' ? compared : -compared) || leftId.localeCompare(rightId);
    };
    let items = (await this.search(input.query, signal, 250))
      .filter(matchesKind)
      .filter((item) => !input.libraryIds?.length || (item.libraryId && input.libraryIds.includes(item.libraryId)));
    if (sort === 'title') items = [...items].sort((left, right) => compareKnown(normalizedTitle(left.title), normalizedTitle(right.title), left.id, right.id));
    if (sort === 'releaseYear') items = [...items].sort((left, right) => compareKnown(left.year || undefined, right.year || undefined, left.id, right.id));
    if (sort === 'dateAdded') items = [...items].sort((left, right) => compareKnown(left.addedAt ? Date.parse(left.addedAt) : undefined, right.addedAt ? Date.parse(right.addedAt) : undefined, left.id, right.id));
    const contract = await this.searchContract(signal);
    const groups = contract.groupOrder.flatMap((groupId) => {
      const descriptor = contract.groups.find((group) => group.id === groupId);
      if (!descriptor || (input.group && input.group !== descriptor.id)) return [];
      const resultKinds = new Set(descriptor.resultKinds);
      const groupItems = items.filter((item) => resultKinds.has(String(item.entityKind ?? item.kind)));
      return groupItems.length ? [{
        id: descriptor.id,
        title: descriptor.title,
        entityKind: descriptor.entityKind,
        status: 'success' as const,
        items: groupItems,
        hasMore: false,
      }] : [];
    });
    const offset = input.cursor?.startsWith('fixture:') ? Number(input.cursor.slice('fixture:'.length)) || 0 : 0;
    const limit = input.limit ?? 50;
    return {
      query: input.query,
      sort,
      direction,
      groups: groups.map((group) => {
        const page = group.items.slice(offset, offset + limit);
        const nextOffset = offset + page.length;
        const hasMore = nextOffset < group.items.length;
        return { ...group, items: page, hasMore, nextCursor: hasMore ? `fixture:${nextOffset}` : null };
      }),
    };
  }

  async searchHistory(signal: AbortSignal): Promise<SearchHistoryItem[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return structuredClone(this.fixtureSearchHistory);
  }

  async clearSearchHistory(signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.fixtureSearchHistory = [];
  }

  async person(id: string, signal: AbortSignal, _cursor?: string): Promise<PersonDetail> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const decoded = id.startsWith('person:') ? decodeURIComponent(id.slice('person:'.length)) : id;
    const credits = this.allItems().filter((item) => item.people?.some((person) => person.id === id || person.name.toLocaleLowerCase() === decoded.toLocaleLowerCase()));
    const match = credits.flatMap((item) => item.people ?? []).find((person) => person.id === id || person.name.toLocaleLowerCase() === decoded.toLocaleLowerCase());
    return { id, name: match?.name ?? decoded, imageUrl: match?.imageUrl, credits, hasMore: false, nextCursor: null };
  }

  async media(id: string, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = this.allItems().find((candidate) => candidate.id === id);
    if (!item) throw new Error('This media item is no longer available.');
    const recommendationItems = this.allItems().filter((candidate) => candidate.type === item.type && candidate.id !== id).slice(0, 7);
    return {
      ...item,
      summary: item.type === 'show'
        ? 'Ordinary people are pulled into an escalating chain of consequences where every choice leaves a mark.'
        : 'A carefully catalogued film from this Portico library.',
      studio: item.type === 'show' ? 'FX Productions' : 'Portico Library',
      recommendationRows: [{ id: 'more-like-this', title: 'More like this', type: 'poster', items: recommendationItems, hasMore: false }],
    };
  }

  async mediaChildren(id: string, signal: AbortSignal, cursor?: string, limit = 50): Promise<MediaChildrenPage> {
    const item = await this.media(id, signal);
    const offset = cursor?.startsWith('fixture-children:') ? Number(cursor.slice('fixture-children:'.length)) || 0 : 0;
    const page = (item.children ?? []).slice(offset, offset + limit);
    const nextOffset = offset + page.length;
    const hasMore = nextOffset < (item.children?.length ?? 0);
    return { items: page, hasMore, nextCursor: hasMore ? `fixture-children:${nextOffset}` : null };
  }

  async deleteMedia(id: string, input: { deleteFiles: boolean; confirmation?: string }, signal: AbortSignal): Promise<MediaDeleteResult> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = this.allItems().find((candidate) => candidate.id === id);
    if (!item) throw new Error('This media item is no longer available.');
    if (input.deleteFiles && input.confirmation !== item.title) throw new Error(`Type “${item.title}” exactly to move its source files to trash.`);
    this.deletedMediaIds.add(id);
    this.mediaState.delete(id);
    this.mediaMetadata.delete(id);
    return { ok: true, deletedItems: 1, trashedFiles: input.deleteFiles ? Math.max(1, item.fileCount ?? 1) : 0 };
  }

  async watchlist(signal: AbortSignal): Promise<MediaItem[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return this.items().filter((item) => item.watchlisted);
  }

  async favorites(signal: AbortSignal): Promise<MediaItem[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return this.allItems().filter((item) => item.favorite);
  }

  async setWatchlist(id: string, watchlisted: boolean, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = await this.media(id, signal);
    this.mediaState.set(id, { ...this.mediaState.get(id), watchlisted });
    return { ...item, watchlisted };
  }

  async setFavorite(id: string, favorite: boolean, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = await this.media(id, signal);
    this.mediaState.set(id, { ...this.mediaState.get(id), favorite });
    return { ...item, favorite };
  }

  async setReaction(id: string, reaction: '' | 'like' | 'dislike', signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = await this.media(id, signal);
    this.mediaState.set(id, { ...this.mediaState.get(id), reaction });
    return { ...item, reaction };
  }

  async setRating(id: string, userRating: number, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (!Number.isInteger(userRating) || userRating < 0 || userRating > 10) throw new Error('Rating must be between 0 and 10.');
    const item = await this.media(id, signal);
    this.mediaState.set(id, { ...this.mediaState.get(id), userRating });
    return { ...item, userRating };
  }

  async setWatched(id: string, watched: boolean, signal: AbortSignal): Promise<MediaItem> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const item = await this.media(id, signal);
    this.mediaState.set(id, { ...this.mediaState.get(id), watched });
    return { ...item, watched };
  }

  async updateMediaMetadata(ids: string[], patch: MediaMetadataUpdate, signal: AbortSignal): Promise<MediaItem[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const known = new Set(this.allItems().map((item) => item.id));
    for (const id of ids) {
      if (!known.has(id)) throw new Error('One of the selected items is no longer available.');
      const current = this.mediaMetadata.get(id) ?? {};
      this.mediaMetadata.set(id, {
        ...current, ...patch,
        genre: patch.genres ? patch.genres.join(', ') : current.genre,
        rating: patch.contentRating ?? current.rating,
      });
    }
    return this.allItems().filter((item) => ids.includes(item.id));
  }

  async uploadMediaImage(id: string, type: string, _file: File, _expectedRevision: number, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const imageType = type.trim().toLocaleLowerCase();
    if (!['poster', 'backdrop', 'thumb', 'logo', 'banner', 'disc', 'clearart'].includes(imageType)) throw new Error('Choose a supported artwork type.');
    const current = this.fixtureArtwork(item).map((image) => image.type === imageType ? { ...image, preferred: false } : image);
    current.push({
      id: `fixture-upload-${id}-${Date.now()}`,
      type: imageType,
      source: 'manual',
      provider: 'upload',
      width: imageType === 'poster' ? 1200 : 1920,
      height: imageType === 'poster' ? 1800 : 1080,
      sortOrder: 0,
      preferred: true,
      createdAt: new Date().toISOString(),
    });
    this.mediaArtwork.set(id, current);
  }

  async deleteMediaImage(id: string, imageId: string, _expectedRevision: number, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const current = this.fixtureArtwork(item);
    const image = current.find((candidate) => candidate.id === imageId);
    if (!image || image.source !== 'manual' || image.provider !== 'upload') throw new Error('Only uploaded artwork can be removed.');
    this.mediaArtwork.set(id, current.filter((candidate) => candidate.id !== imageId));
  }

  async setPreferredMediaImage(id: string, imageId: string, _expectedRevision: number, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const current = this.fixtureArtwork(item);
    const selected = current.find((image) => image.id === imageId);
    if (!selected) throw new Error('Artwork was not found.');
    this.mediaArtwork.set(id, current.map((image) => image.type === selected.type ? { ...image, preferred: image.id === imageId } : image));
  }

  async reorderMediaImages(id: string, imageIds: string[], _expectedRevision: number, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const current = this.fixtureArtwork(item);
    const positions = new Map(imageIds.map((imageId, index) => [imageId, index]));
    if (positions.size !== imageIds.length || imageIds.some((imageId) => !current.some((image) => image.id === imageId))) throw new Error('Artwork order contains an unknown or duplicate image.');
    this.mediaArtwork.set(id, current.map((image) => positions.has(image.id) ? { ...image, sortOrder: positions.get(image.id) } : image));
  }

  async uploadSubtitle(id: string, file: File, language: string, label: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const streams = [...(item.streams ?? []), {
      id: `fixture-subtitle-${Date.now()}`,
      kind: 'subtitle' as const,
      codec: file.name.split('.').pop()?.toLocaleLowerCase() || 'webvtt',
      displayTitle: label || `${language.toLocaleUpperCase()} subtitle`,
      language,
      sourceUrl: `/fixture/${encodeURIComponent(file.name)}`,
      subtitleOffsetMs: 0,
    }];
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), streams });
  }

  async updateSubtitle(id: string, streamId: string, offsetMs: number, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    if (!Number.isInteger(offsetMs) || offsetMs < -300000 || offsetMs > 300000) throw new Error('Subtitle offset must be between -300000 and 300000 milliseconds.');
    const selected = item.streams?.find((stream) => stream.id === streamId && stream.kind === 'subtitle');
    if (!selected?.sourceUrl) throw new Error('Only managed subtitles can be timed.');
    const streams = item.streams?.map((stream) => stream.id === streamId ? { ...stream, subtitleOffsetMs: offsetMs } : stream) ?? [];
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), streams });
  }

  async deleteSubtitle(id: string, streamId: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const selected = item.streams?.find((stream) => stream.id === streamId && stream.kind === 'subtitle');
    if (!selected?.sourceUrl) throw new Error('Only managed subtitles can be removed.');
    this.mediaMetadata.set(id, {
      ...this.mediaMetadata.get(id),
      streams: item.streams?.filter((stream) => stream.id !== streamId) ?? [],
    });
  }

  async uploadLyrics(id: string, file: File, language: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    if (item.kind !== 'track') throw new Error('Lyrics can only be added to tracks.');
    const lyric: MediaLyric = {
      id: `fixture-lyric-${Date.now()}`,
      source: 'manual',
      provider: 'upload',
      format: file.name.toLocaleLowerCase().endsWith('.lrc') ? 'lrc' : 'txt',
      synced: file.name.toLocaleLowerCase().endsWith('.lrc'),
      language,
      createdAt: new Date().toISOString(),
    };
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), lyrics: [...(item.lyrics ?? []), lyric] });
  }

  async fetchLyrics(id: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    if (item.kind !== 'track') throw new Error('Lyrics can only be added to tracks.');
    const lyric: MediaLyric = {
      id: `fixture-lrclib-${Date.now()}`,
      source: 'provider',
      provider: 'lrclib',
      format: 'lrc',
      synced: true,
      language: 'en',
      createdAt: new Date().toISOString(),
    };
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), lyrics: [...(item.lyrics ?? []), lyric] });
  }

  async searchLyrics(id: string, query: string, signal: AbortSignal): Promise<LyricSearchCandidate[]> {
    const item = await this.media(id, signal);
    if (item.kind !== 'track') throw new Error('Lyrics search is only available for tracks.');
    const trackName = query.trim() || item.title;
    return [
      { provider: 'lrclib', externalId: `fixture-synced-${id}`, trackName, artistName: item.subtitle.split(' · ')[0], albumName: item.parentTitle, durationSeconds: 230, format: 'lrc', synced: true },
      { provider: 'lrclib', externalId: `fixture-plain-${id}`, trackName, artistName: item.subtitle.split(' · ')[0], albumName: item.parentTitle, durationSeconds: 230, format: 'txt', synced: false },
    ];
  }

  async applyLyrics(id: string, candidate: Pick<LyricSearchCandidate, 'provider' | 'externalId'>, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    if (item.kind !== 'track') throw new Error('Lyrics can only be added to tracks.');
    const synced = candidate.externalId.includes('synced');
    const lyric: MediaLyric = {
      id: `fixture-applied-${Date.now()}`,
      source: 'provider',
      provider: candidate.provider,
      format: synced ? 'lrc' : 'txt',
      synced,
      language: 'en',
      createdAt: new Date().toISOString(),
    };
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), lyrics: [...(item.lyrics ?? []), lyric] });
  }

  async deleteLyrics(id: string, lyricId: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    const lyric = item.lyrics?.find((candidate) => candidate.id === lyricId);
    const deletable = lyric && ((lyric.source === 'manual' && lyric.provider === 'upload') || (lyric.source === 'provider' && lyric.provider === 'lrclib'));
    if (!deletable) throw new Error('Only uploaded or provider lyrics can be removed.');
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), lyrics: item.lyrics?.filter((candidate) => candidate.id !== lyricId) ?? [] });
  }

  async searchMediaMatches(id: string, query: string, signal: AbortSignal): Promise<MediaMatchCandidate[]> {
    const item = await this.media(id, signal);
    const title = query.trim() || item.title;
    return [
      { provider: 'tmdb', externalId: `fixture-${id}`, externalType: item.kind, source: 'TMDB', score: 0.97, accepted: true, title, year: item.year, overview: item.summary, reasons: [{ code: 'title_exact', delta: 0.6 }], createdAt: new Date().toISOString() },
      { provider: 'tvdb', externalId: `fixture-alt-${id}`, externalType: item.kind, source: 'TVDB', score: 0.81, accepted: false, title: `${title} (${item.year})`, year: item.year, overview: 'Alternate catalogue match.', reasons: [{ code: 'title_close', delta: 0.4 }], createdAt: new Date().toISOString() },
    ];
  }

  async applyMediaMatch(id: string, candidate: Pick<MediaMatchCandidate, 'provider' | 'externalId' | 'externalType'>, _expectedRevision: number, signal: AbortSignal): Promise<MediaItem> {
    const item = await this.media(id, signal);
    this.mediaMetadata.set(id, { ...this.mediaMetadata.get(id), summary: `${item.summary ?? ''}`.trim(), tags: [...(item.tags ?? []), candidate.provider.toLocaleUpperCase()] });
    return this.media(id, signal);
  }

  private createFixtureMediaJob(type: MediaJobType, mediaId: string, message: string, metadata?: Record<string, string>): MediaJob {
    const now = new Date().toISOString();
    this.mediaJobSequence += 1;
    return {
      id: `fixture-media-job-${this.mediaJobSequence}`,
      type,
      status: 'queued',
      progress: 0,
      message,
      resourceType: 'media',
      resourceId: mediaId,
      metadata,
      createdAt: now,
      updatedAt: now,
    };
  }

  async queueMediaJob(id: string, type: MediaJobType, options: MediaJobOptions, signal: AbortSignal): Promise<MediaJob> {
    const item = await this.media(id, signal);
    const description = type === 'metadata_refresh'
      ? `Metadata refresh queued for ${item.title}.`
      : type === 'media_analyze' && options.analysisMode === 'probe'
        ? `Media stream analysis queued for ${item.title}.`
        : type === 'media_analyze'
          ? `Media analysis queued for ${item.title}.`
          : `Optimized version queued for ${item.title}.`;
    let metadata: Record<string, string> | undefined;
    if (options.profile) metadata = { profile: options.profile };
    else if (options.analysisMode) metadata = { analysisMode: options.analysisMode };
    const job = this.createFixtureMediaJob(type, id, description, metadata);
    if (type === 'optimize_version' && options.profile) this.optimizedJobs.set(`${id}:${options.profile}`, job);
    return structuredClone(job);
  }

  async mediaDownloadOptions(id: string, signal: AbortSignal): Promise<MediaDownloadOptions> {
    const item = await this.media(id, signal);
    const profiles = [
      { id: '720p-medium', label: '720p balanced', height: 720, videoKbps: 3200, audioKbps: 192 },
      { id: '1080p-high', label: '1080p high quality', height: 1080, videoKbps: 8000, audioKbps: 320, default: true },
    ];
    return {
      canDownload: true,
      defaultProfile: '1080p-high',
      optimizedVersions: item.optimizedVersions ?? [],
      profiles,
      options: [
        {
          id: 'source',
          kind: 'source',
          profile: 'source',
          label: 'Original source',
          description: 'Download the selected source file without conversion.',
          available: true,
          url: silentPreview,
          sizeBytes: 48_000,
          container: item.type === 'music' ? 'wav' : 'mkv',
          sourceKind: 'local',
        },
        ...profiles.map((profile) => {
          const version = item.optimizedVersions?.find((candidate) => candidate.profile === profile.id);
          return {
          id: `optimized-${profile.id}`,
          kind: 'optimized' as const,
          profile: profile.id,
          label: profile.label,
          description: `MP4 H.264/AAC optimized for up to ${profile.height}p playback.`,
          available: Boolean(version),
          requiresOptimizedVersion: !version,
          url: version ? silentPreview : undefined,
          sizeBytes: version?.sizeBytes,
          container: 'mp4',
          videoCodec: 'h264',
          audioCodec: 'aac',
          sourceKind: 'optimized' as const,
          job: this.optimizedJobs.get(`${id}:${profile.id}`),
          };
        }),
      ],
    };
  }

  async createOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<MediaJob> {
    const item = await this.media(id, signal);
    const job = this.createFixtureMediaJob('optimize_version', id, `Optimized ${profile} version queued for ${item.title}.`, { profile });
    this.optimizedJobs.set(`${id}:${profile}`, job);
    return structuredClone(job);
  }

  async deleteOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<void> {
    const item = await this.media(id, signal);
    if (!item.optimizedVersions?.some((version) => version.profile === profile)) throw new Error('That optimized version is not available.');
    this.mediaMetadata.set(id, {
      ...this.mediaMetadata.get(id),
      optimizedVersions: item.optimizedVersions.filter((version) => version.profile !== profile),
    });
    this.optimizedJobs.delete(`${id}:${profile}`);
  }

  async createMediaDownloadURL(id: string, _profile: string, signal: AbortSignal): Promise<string> {
    await this.media(id, signal);
    return silentPreview;
  }

  async downloadPreparations(signal: AbortSignal): Promise<DownloadPreparation[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return [];
  }

  async updateDownloadPreparation(_id: string, _action: 'pause' | 'resume' | 'cancel' | 'retry' | 'remove', signal: AbortSignal): Promise<DownloadPreparation> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    throw new Error('Download preparations are unavailable in the fixture source.');
  }

  async downloadPreparationURL(id: string, signal: AbortSignal): Promise<string> {
    await this.media(id, signal);
    return silentPreview;
  }

  private savedShareCandidatesList() {
    const currentUserId = this.currentViewer.user?.id;
    return [...this.browserAccountViewers.values()]
      .flatMap((viewer) => viewer.authenticated && viewer.user && viewer.user.id !== currentUserId
        ? [{ userId: viewer.user.id, displayName: viewer.user.displayName }]
        : [])
      .filter((candidate, index, candidates) => candidates.findIndex((item) => item.userId === candidate.userId) === index)
      .sort((left, right) => left.displayName.localeCompare(right.displayName));
  }

  private savedSharesFromRequest(requests: SavedResourceShareRequest[] | undefined): SavedResourceShare[] | undefined {
    if (requests === undefined) return undefined;
    const candidates = new Map(this.savedShareCandidatesList().map((candidate) => [candidate.userId, candidate]));
    const seen = new Set<string>();
    return requests.map((request) => {
      if (seen.has(request.userId)) throw new Error('Each person can only be added once.');
      seen.add(request.userId);
      const candidate = candidates.get(request.userId);
      if (!candidate) throw new Error('That person is no longer available to share with.');
      return { ...candidate, canEdit: request.canEdit };
    });
  }

  async savedShareCandidates(query: string, limit: number, signal: AbortSignal): Promise<SavedShareCandidatePage> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const normalizedQuery = query.trim().slice(0, 80).toLocaleLowerCase();
    const boundedLimit = Math.max(1, Math.min(limit, 50));
    const matches = this.savedShareCandidatesList().filter((candidate) => !normalizedQuery || candidate.displayName.toLocaleLowerCase().includes(normalizedQuery));
    return { items: structuredClone(matches.slice(0, boundedLimit)), hasMore: matches.length > boundedLimit };
  }

  async savedResources(kind: SavedResourceKind, signal: AbortSignal): Promise<SavedResourceSummary[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return [...(this.saved.get(kind)?.values() ?? [])].map((resource) => structuredClone(resource));
  }

  async savedResource<K extends SavedResourceKind>(kind: K, id: string, input: SavedResourceItemsInput, signal: AbortSignal): Promise<SavedResourceDetail<K>> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const resource = this.saved.get(kind)?.get(id);
    if (!resource) throw new Error('This saved item no longer exists.');
    const byId = new Map(this.allItems().map((item) => [item.id, item]));
    const offset = input.cursor?.startsWith('fixture-saved:') ? Number(input.cursor.slice('fixture-saved:'.length)) || 0 : 0;
    const limit = Math.max(1, Math.min(input.limit ?? 50, 200));
    if (kind === 'playlist') {
      const allEntries = this.savedPlaylistEntries.get(id) ?? [];
      const page = allEntries.slice(offset, offset + limit);
      const nextOffset = offset + page.length;
      const entries: SavedPlaylistEntry[] = page.flatMap((entry, index) => {
        const item = byId.get(entry.mediaId);
        return item ? [{ entryId: entry.entryId, media: item, position: offset + index }] : [];
      });
      return {
        kind: 'playlist',
        resource: { ...structuredClone(resource), kind: 'playlist', itemCount: allEntries.length },
        entries,
        hasMore: nextOffset < allEntries.length,
        nextCursor: nextOffset < allEntries.length ? `fixture-saved:${nextOffset}` : null,
      } as SavedResourceDetail<K>;
    }
    const itemIds = kind === 'view'
      ? this.allItems().filter((item) => item.type === 'movie' && !item.watched).map((item) => item.id)
      : this.savedItems.get(`${kind}:${id}`) ?? [];
    const page = itemIds.slice(offset, offset + limit);
    const items = page.flatMap((mediaId) => {
      const item = byId.get(mediaId);
      return item ? [item] : [];
    });
    const nextOffset = offset + page.length;
    return {
      kind,
      resource: { ...structuredClone(resource), kind, itemCount: itemIds.length },
      items,
      hasMore: nextOffset < itemIds.length,
      nextCursor: nextOffset < itemIds.length ? `fixture-saved:${nextOffset}` : null,
    } as SavedResourceDetail<K>;
  }

  async createSavedResource(kind: SavedResourceKind, input: SavedResourceCreateInput, signal: AbortSignal): Promise<SavedResourceSummary> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    if (kind === 'view' && (!input.libraryId || !input.pivot)) throw new Error('Choose a library and view.');
    const timestamp = new Date().toISOString();
    const id = `fixture-${kind}-${globalThis.crypto.randomUUID()}`;
    const shares = kind === 'view' ? undefined : this.savedSharesFromRequest(input.shares) ?? [];
    const resource: SavedResourceSummary = {
      id, kind, ownerUserId: kind === 'view' ? undefined : this.currentViewer.user?.id, title: input.title.trim(), summary: input.summary?.trim(), itemCount: input.mediaIds?.length ?? 0,
      canEdit: true, visibility: kind === 'view' ? undefined : input.visibility ?? 'private',
      shares,
      libraryId: input.libraryId, libraryName: this.currentViewer.serverName, pivot: input.pivot,
      isPinned: input.isPinned ?? false, createdAt: timestamp, updatedAt: timestamp,
    };
    if (!resource.title) throw new Error('Enter a name.');
    const resources = this.saved.get(kind) ?? new Map<string, SavedResourceSummary>();
    resources.set(id, resource);
    this.saved.set(kind, resources);
    if (kind === 'playlist') {
      this.savedPlaylistEntries.set(id, (input.mediaIds ?? []).map((mediaId) => ({ entryId: `fixture-playlist-entry-${globalThis.crypto.randomUUID()}`, mediaId })));
    } else if (kind === 'collection') {
      this.savedItems.set(`${kind}:${id}`, [...new Set(input.mediaIds ?? [])]);
    }
    return structuredClone(resource);
  }

  async updateSavedResource(kind: SavedResourceKind, id: string, input: SavedResourceUpdateInput, signal: AbortSignal): Promise<SavedResourceSummary> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const current = this.saved.get(kind)?.get(id);
    if (!current) throw new Error('This saved item no longer exists.');
    const shares = kind === 'view' ? undefined : this.savedSharesFromRequest(input.shares);
    const resource: SavedResourceSummary = {
      ...current, title: input.title.trim(), summary: input.summary?.trim(), visibility: kind === 'view' ? undefined : input.visibility ?? current.visibility,
      shares: shares ?? current.shares,
      libraryId: input.libraryId ?? current.libraryId, pivot: input.pivot ?? current.pivot, isPinned: input.isPinned ?? current.isPinned,
      updatedAt: new Date().toISOString(),
    };
    if (!resource.title) throw new Error('Enter a name.');
    this.saved.get(kind)?.set(id, resource);
    return structuredClone(resource);
  }

  async deleteSavedResource(kind: SavedResourceKind, id: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    this.saved.get(kind)?.delete(id);
    this.savedItems.delete(`${kind}:${id}`);
    this.savedPlaylistEntries.delete(id);
  }

  async mutateSavedResourceItems<K extends SavedEditableResourceKind>(kind: K, id: string, mutation: SavedResourceItemsMutation<K>, signal: AbortSignal): Promise<SavedResourceSummary> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const resource = this.saved.get(kind)?.get(id);
    if (!resource) throw new Error('This saved item no longer exists.');
    if (mutation.expectedUpdatedAt && mutation.expectedUpdatedAt !== resource.updatedAt) throw new Error('revision_conflict: This item changed on another device. Reload it before trying again.');
    let itemCount = 0;
    if (kind === 'playlist') {
      const playlistMutation = mutation as SavedResourceItemsMutation<'playlist'>;
      let entries = [...(this.savedPlaylistEntries.get(id) ?? [])];
      const removals = new Set(playlistMutation.removeEntryIds ?? []);
      entries = entries.filter((entry) => !removals.has(entry.entryId));
      for (const mediaId of playlistMutation.addMediaIds ?? []) entries.push({ entryId: `fixture-playlist-entry-${globalThis.crypto.randomUUID()}`, mediaId });
      if (playlistMutation.orderEntryIds) {
        const byEntryId = new Map(entries.map((entry) => [entry.entryId, entry]));
        if (playlistMutation.orderEntryIds.length !== entries.length || new Set(playlistMutation.orderEntryIds).size !== entries.length || playlistMutation.orderEntryIds.some((entryId) => !byEntryId.has(entryId))) {
          throw new Error('orderEntryIds must contain every current playlist entry exactly once.');
        }
        entries = playlistMutation.orderEntryIds.map((entryId) => byEntryId.get(entryId)!);
      }
      this.savedPlaylistEntries.set(id, entries);
      itemCount = entries.length;
    } else {
      const collectionMutation = mutation as SavedResourceItemsMutation<'collection'>;
      let ids = [...(this.savedItems.get(`collection:${id}`) ?? [])];
      const removals = new Set(collectionMutation.removeMediaIds ?? []);
      ids = ids.filter((mediaId) => !removals.has(mediaId));
      for (const mediaId of collectionMutation.addMediaIds ?? []) if (!ids.includes(mediaId)) ids.push(mediaId);
      this.savedItems.set(`collection:${id}`, ids);
      itemCount = ids.length;
    }
    const updatedAt = new Date(Math.max(Date.now(), Date.parse(resource.updatedAt) + 1)).toISOString();
    const updated = { ...resource, itemCount, updatedAt };
    this.saved.get(kind)?.set(id, updated);
    return structuredClone(updated);
  }

  private playbackFor(item: MediaItem, queueItems = this.allItems().filter((candidate) => candidate.id !== item.id).slice(0, 3), repeatMode: PlaybackRepeatMode = 'off'): PlaybackResponse {
    const sessionId = `fixture-session-${item.id}`;
    const playback = {
      sessionId, nextEventSequence: 1,
      mediaGrant: { token: 'fixture-media-grant', expiresAt: new Date(Date.now() + 600_000).toISOString() },
      continuationCredential: {
        token: 'fixture-playback-continuation',
        expiresAt: new Date(Date.now() + 600_000).toISOString(),
        generation: 1,
        origin: 'http://localhost:32500',
      },
      media: playbackMedia(item), sourceUrl: silentPreview, directPlay: true, streamFormat: 'direct',
      decision: { mode: 'direct_play', reason: 'Fixture preview', reasonCodes: ['exact_tuple'], deliveryProfile: 'video-original', requiresTranscode: false, isProxied: false, isServerCached: false },
      policy: { networkClass: 'local', qualityProfile: 'original', directPlayPolicy: 'prefer', directStreamPolicy: 'allow', transcodePolicy: 'allow', allowHdr: true, deliveryProfile: 'video-original', serverClamps: [] },
      qualities: [{ id: 'original', label: 'Original', description: 'Fixture preview' }],
      resources: [{ id: `${sessionId}-default`, sourceUrl: silentPreview, streamFormat: 'direct', qualityId: 'original', subtitleMode: 'off', default: true }],
      audioStreams: [], subtitleStreams: [], chapters: [],
      queue: queueItems.map(playbackMedia), repeatMode, queueRevision: 1, playbackRevision: 1,
      timeline: { type: 'vod', durationSeconds: 1, canPause: true, canSeek: true }, resumePositionSeconds: 0, generation: 1,
    } as PlaybackResponse;
    this.fixturePlayback = playback;
    this.fixtureQueue = { sessionId, current: playback.media, items: queueItems.map(playbackMedia), history: [], total: queueItems.length, canMutate: true, repeatMode, revision: 1 } as PlaybackSessionQueueResponse;
    return playback;
  }

  async startPlayback(mediaId: string, options: PlaybackStartOptions, signal: AbortSignal) {
    const item = await this.media(mediaId, signal);
    const byId = new Map(this.allItems().map((candidate) => [candidate.id, candidate]));
    const queueItems = (options.queueMediaIds ?? []).flatMap((id) => {
      const candidate = byId.get(id);
      return candidate ? [candidate] : [];
    });
    return this.playbackFor(item, queueItems, options.repeatMode ?? 'off');
  }
  async mediaTrickplay(_id: string, signal: AbortSignal): Promise<MediaTrickplaySet[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    return [];
  }
  async restorePlayback(_signal: AbortSignal, _intent?: import('@porticomediaserver/client-core').PlaybackIntent) { return { active: Boolean(this.fixturePlayback), playback: this.fixturePlayback }; }
  async touchPlayback(_sessionId: string, event: PlaybackProgressEvent, _signal?: AbortSignal, _keepalive?: boolean) {
    return {
      accepted: true,
      duplicate: false,
      stale: false,
      generation: event.generation ?? this.fixturePlayback?.generation ?? 1,
      highestEventSequence: event.eventSequence ?? 1,
      sessionState: event.completed ? 'stopped' as const : event.state ?? 'paused' as const,
    };
  }
  async renewPlaybackMediaGrant(_sessionId: string, _signal: AbortSignal) { return { token: 'fixture-media-grant', expiresAt: new Date(Date.now() + 600_000).toISOString() }; }
  async renegotiatePlayback(sessionId: string, request: import('@porticomediaserver/client-core').PlaybackRenegotiationRequest, signal: AbortSignal) {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const current = this.fixturePlayback;
    if (!current || current.sessionId !== sessionId) throw new Error('No fixture playback session is active.');
    if (request.expectedRevision !== current.playbackRevision) throw new ApiError(409, 'playback_revision_conflict', 'Playback selection changed. Reload it before trying again.');
    const next = {
      ...current,
      playbackRevision: current.playbackRevision + 1,
      selectedQualityId: request.qualityId ?? current.selectedQualityId,
      selectedAudioStreamId: request.audioStreamId ?? current.selectedAudioStreamId,
      selectedSubtitleStreamId: request.subtitleStreamId ?? current.selectedSubtitleStreamId,
      selectedSubtitleMode: request.subtitleMode ?? current.selectedSubtitleMode,
      mediaGrant: { token: `fixture-media-grant-${current.playbackRevision + 1}`, expiresAt: new Date(Date.now() + 600_000).toISOString() },
    } satisfies PlaybackResponse;
    this.fixturePlayback = next;
    return structuredClone(next);
  }
  async stopPlayback(_sessionId: string, _signal?: AbortSignal, _keepalive?: boolean) { this.fixturePlayback = undefined; this.fixtureQueue = undefined; }
  async playbackSessionQueue(_sessionId: string, _signal: AbortSignal) { if (!this.fixtureQueue) throw new Error('No fixture playback session is active.'); return structuredClone(this.fixtureQueue); }
  async updatePlaybackSessionQueue(_sessionId: string, request: PlaybackSessionQueueReplaceRequest, _signal: AbortSignal) {
    if (!this.fixtureQueue) throw new Error('No fixture playback session is active.');
    if (request.expectedRevision !== this.fixtureQueue.revision) throw new ApiError(409, 'queue_revision_conflict', 'The playback queue changed. Reload it before trying again.');
    const byId = new Map(this.allItems().map((item) => [item.id, item]));
    this.fixtureQueue.items = request.mediaIds.flatMap((id) => { const item = byId.get(id); return item ? [playbackMedia(item)] : []; }) as typeof this.fixtureQueue.items;
    this.fixtureQueue.total = this.fixtureQueue.items.length;
    this.fixtureQueue.repeatMode = request.repeatMode;
    this.fixtureQueue.revision += 1;
    if (this.fixturePlayback) this.fixturePlayback = { ...this.fixturePlayback, queue: [...this.fixtureQueue.items], repeatMode: this.fixtureQueue.repeatMode, queueRevision: this.fixtureQueue.revision };
    return structuredClone(this.fixtureQueue);
  }
  async mutatePlaybackSessionQueue(sessionId: string, request: PlaybackSessionQueueRequest, signal: AbortSignal) {
    if (!this.fixtureQueue) throw new Error('No fixture playback session is active.');
    if (request.expectedRevision !== this.fixtureQueue.revision) throw new ApiError(409, 'queue_revision_conflict', 'The playback queue changed. Reload it before trying again.');
    const ids = this.fixtureQueue.items.map((item) => item.id);
    if (request.action === 'remove' && request.index != null) ids.splice(request.index, 1);
    if (request.action === 'reorder' && request.fromIndex != null && request.toIndex != null) {
      const [id] = ids.splice(request.fromIndex, 1);
      if (id) ids.splice(request.toIndex, 0, id);
    }
    if (request.action === 'clear') ids.splice(0, ids.length);
    const requestedIds = request.mediaIds?.length ? request.mediaIds : request.mediaId ? [request.mediaId] : [];
    if (request.action === 'append') ids.push(...requestedIds);
    if (request.action === 'play_next') ids.unshift(...requestedIds);
    return this.updatePlaybackSessionQueue(sessionId, {
      expectedRevision: request.expectedRevision,
      mediaIds: ids,
      repeatMode: request.action === 'set_repeat' ? request.repeatMode ?? this.fixtureQueue.repeatMode : this.fixtureQueue.repeatMode,
    }, signal);
  }
  async prepareNextPlayback(_sessionId: string, _signal: AbortSignal, _request?: PlaybackPrepareNextRequest) {
    const next = this.fixtureQueue?.items[0];
    if (!next) throw new Error('Nothing else is queued.');
    const item = this.allItems().find((candidate) => candidate.id === next.id);
    if (!item) throw new Error('The next fixture item is unavailable.');
    const repeatMode = this.fixtureQueue?.repeatMode ?? 'off';
    const remaining = (this.fixtureQueue?.items ?? []).filter((candidate) => candidate.id !== item.id).flatMap((candidate) => {
      const queued = this.allItems().find((mediaItem) => mediaItem.id === candidate.id);
      return queued ? [queued] : [];
    });
    const playback = this.playbackFor(item, remaining, repeatMode);
    return {
      preparedSessionId: `fixture-prepared-${item.id}`,
      playback,
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      preloadPolicy: 'metadata',
      handoffMode: 'replace',
      queue: this.fixtureQueue?.items ?? [],
      queueRevision: playback.queueRevision,
      playbackRevision: playback.playbackRevision,
    };
  }
  async handoffPlayback(_sessionId: string, request: PlaybackHandoffRequest, _signal: AbortSignal) {
    const id = request.mediaId ?? request.preparedSessionId?.replace('fixture-prepared-', '') ?? this.fixtureQueue?.items[0]?.id;
    const item = this.allItems().find((candidate) => candidate.id === id);
    if (!item) throw new Error('The requested fixture item is unavailable.');
    return this.playbackFor(item, undefined, this.fixtureQueue?.repeatMode ?? 'off');
  }
  playbackResourceUrl(path: string) { return path; }

  private fixtureLiveSource(): ActionableLiveTVSource {
    return { id: 'fixture-live', name: 'Portico Test TV', type: 'm3u', enabled: true, sortOrder: 0, channelCount: 4, programCount: 12, logoCount: 0, hiddenChannelCount: 0, favoriteChannelCount: this.liveFavorites.size, actions: [] };
  }
  private fixtureLiveChannels(): ActionableLiveTVChannel[] {
    return ['News 7', 'Public One', 'Cinema North', 'Coastal Sports'].map((name, index) => ({ id: `fixture-channel-${index + 1}`, sourceId: 'fixture-live', number: String(index + 1), name, groupTitle: index === 3 ? 'Sports' : index === 2 ? 'Movies' : 'Local', enabled: true, favorite: this.liveFavorites.has(`fixture-channel-${index + 1}`), hidden: false, programCount: 3, sortOrder: index, actions: ['live.play', this.liveFavorites.has(`fixture-channel-${index + 1}`) ? 'favorite.remove' : 'favorite.add'] }));
  }
  async liveTVSources(_signal: AbortSignal) { return [this.fixtureLiveSource()]; }
  async liveTVChannels(_sourceId: string, _signal: AbortSignal) { return this.fixtureLiveChannels(); }
  async liveTVGuide(_sourceId: string, input: { from: string; hours: number; query?: string; favoritesOnly?: boolean }, _signal: AbortSignal) {
    const from = new Date(input.from);
    const channels = this.fixtureLiveChannels().filter((channel) => (!input.favoritesOnly || channel.favorite) && (!input.query || channel.name.toLocaleLowerCase().includes(input.query.toLocaleLowerCase())));
    const programs: ActionableLiveTVProgram[] = channels.flatMap((channel) => [0, 1, 2].map((slot) => ({ id: `${channel.id}-program-${slot}`, sourceId: 'fixture-live', channelId: channel.id, title: slot === 0 ? 'Live programming' : slot === 1 ? 'Up next' : 'Later today', category: channel.groupTitle, startAt: new Date(from.getTime() + slot * 60 * 60 * 1000).toISOString(), endAt: new Date(from.getTime() + (slot + 1) * 60 * 60 * 1000).toISOString(), isNew: slot === 0, actions: ['live.play'] })));
    return { source: this.fixtureLiveSource(), channels, programs, channelGroups: [...new Set(channels.map((channel) => channel.groupTitle).filter((value): value is string => Boolean(value)))], from: from.toISOString(), to: new Date(from.getTime() + input.hours * 60 * 60 * 1000).toISOString(), serverTime: from.toISOString(), pageInfo: { nextCursor: null, hasMore: false, total: channels.length }, capabilities: { canPlay: true, canFavoriteChannels: true, canScheduleRecordings: false, canManageRecordingRules: false, canManageSources: false } };
  }
  async updateLiveTVChannel(channelId: string, state: { favorite?: boolean }, _signal: AbortSignal) {
    if (state.favorite) this.liveFavorites.add(channelId); else this.liveFavorites.delete(channelId);
    const channel = this.fixtureLiveChannels().find((item) => item.id === channelId);
    if (!channel) throw new Error('Fixture channel not found.');
    return channel;
  }
  async startLiveTVPlayback(channelId: string, _signal: AbortSignal) {
    const channel = this.fixtureLiveChannels().find((item) => item.id === channelId);
    if (!channel) throw new Error('Fixture channel not found.');
    const item: MediaItem = { id: channel.id, title: channel.name, subtitle: channel.groupTitle ?? 'Live TV', year: 0, type: 'show', kind: 'recording', poster: '/brand/portico-wordmark-white.svg', backdrop: '/brand/portico-wordmark-white.svg', rating: '', length: 'Live', genre: channel.groupTitle ?? '', actions: ['live.play'] };
    const value = this.playbackFor(item, []);
    return {
      ...value,
      isLive: true,
      media: { ...value.media, type: 'live_channel' },
      timeline: { type: 'live' as const, canPause: false, canSeek: false },
    };
  }

  async libraryChannels(_signal: AbortSignal): Promise<LibraryChannelListResponse> {
    return {
      sourceType: 'library-channel',
      items: [
        { id: 'fixture-library-channel', sourceType: 'library-channel', name: 'Saturday Morning', description: 'Cartoons and family adventures from the library.', logoUrl: '/api/library-channels/logos/builtin-saturday-morning', actions: ['live.play'] },
      ],
      pageInfo: { hasMore: false, nextCursor: null },
    };
  }

  async libraryChannelGuide(channelId: string, _input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelGuide> {
    const channel = (await this.libraryChannels(signal)).items.find((item) => item.id === channelId);
    if (!channel) throw new Error('Fixture Library Channel not found.');
    const start = new Date();
    start.setMinutes(0, 0, 0);
    return {
      sourceType: 'library-channel', channel, from: start.toISOString(), to: new Date(start.valueOf() + 10_800_000).toISOString(), serverTime: start.toISOString(), pageInfo: { hasMore: false, nextCursor: null },
      programs: [0, 1, 2].map((offset) => ({ id: `fixture-library-program-${offset}`, channelId, sourceType: 'library-channel' as const, kind: 'media' as const, mediaId: media[offset]?.id, title: media[offset]?.title ?? `Library program ${offset + 1}`, subtitle: media[offset]?.subtitle, summary: media[offset]?.summary, startsAt: new Date(start.valueOf() + offset * 3_600_000).toISOString(), endsAt: new Date(start.valueOf() + (offset + 1) * 3_600_000).toISOString(), artwork: { poster: media[offset]?.poster, backdrop: media[offset]?.backdrop, thumb: media[offset]?.backdrop }, availability: 'available' as const })),
    };
  }

  async libraryChannelsGuide(input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelsGuide> {
    const list = await this.libraryChannels(signal);
    const guides = await Promise.all(list.items.map((channel) => this.libraryChannelGuide(channel.id, input, signal)));
    return {
      sourceType: 'library-channel',
      channels: list.items,
      programs: guides.flatMap((guide) => guide.programs),
      from: guides[0]?.from ?? new Date().toISOString(),
      to: guides[0]?.to ?? new Date(Date.now() + 10_800_000).toISOString(),
      serverTime: guides[0]?.serverTime ?? new Date().toISOString(),
      pageInfo: { hasMore: false, nextCursor: null, total: list.items.length },
    };
  }

  async startLibraryChannelPlayback(channelId: string, signal: AbortSignal) {
    const channel = (await this.libraryChannels(signal)).items.find((item) => item.id === channelId);
    if (!channel) throw new Error('Fixture Library Channel not found.');
    const value = this.playbackFor({ id: channel.id, title: channel.name, subtitle: 'Library Channel', year: 0, type: 'show', kind: 'recording', poster: channel.logoUrl ?? '', backdrop: channel.logoUrl ?? '', rating: '', length: 'Live', genre: 'Library Channel' }, []);
    return { ...value, isLive: true };
  }

  async adminLibraryChannels(_signal: AbortSignal): Promise<AdminLibraryChannelListResponse> {
    const aggregate = this.fixtureLibraryChannelAggregate();
    return { sourceType: 'library-channel', items: [aggregate.channel], pageInfo: { hasMore: false, nextCursor: null, total: 1 } };
  }
  private fixtureLibraryChannelAggregate(): LibraryChannelAggregate {
    const now = new Date().toISOString();
    const channel = { id: 'fixture-library-channel', sourceType: 'library-channel' as const, name: 'Saturday Morning', description: 'Cartoons and family adventures from the library.', enabled: true, sortOrder: 0, timezone: 'America/Halifax', defaultRuleId: 'fixture-rule', qualityProfile: 'auto' as const, logo: { source: 'built_in' as const, ref: 'saturday-morning', url: '/api/library-channels/logos/builtin-saturday-morning', mimeType: 'image/svg+xml' as const, bugEnabled: false, bugOverheadAccepted: false, bugCorner: 'top_right' as const, bugWidthPercent: 9, bugInsetPercent: 2, bugTreatment: 'color' as const }, configRevision: 1, healthState: 'healthy' as const, generatedThrough: new Date(Date.now() + 7 * 86_400_000).toISOString(), createdAt: now, updatedAt: now };
    return { channel, rules: [{ id: 'fixture-rule', name: 'Family animation', enabled: true, sortOrder: 0, query: {}, selectionMode: 'shuffle_bag', episodeMode: 'in_order', exhaustionMode: 'loop', dedupeWindow: 12, maxConsecutive: 4, config: {} }], blocks: [] };
  }
  async libraryChannel(channelId: string, signal: AbortSignal): Promise<LibraryChannelAggregate> {
    if (!(await this.libraryChannels(signal)).items.some((item) => item.id === channelId)) throw new Error('Fixture Library Channel not found.');
    return this.fixtureLibraryChannelAggregate();
  }
  async createLibraryChannel(_input: LibraryChannelConfigurationRequest, _signal: AbortSignal): Promise<LibraryChannelAggregate> { throw new Error('Fixture Library Channel creation is unavailable.'); }
  async updateLibraryChannel(channelId: string, _input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate> { return this.libraryChannel(channelId, signal); }
  async deleteLibraryChannel(_channelId: string, _expectedRevision: number, _signal: AbortSignal): Promise<void> { throw new Error('Fixture Library Channel deletion is unavailable.'); }
  async libraryChannelTemplates(_signal: AbortSignal): Promise<LibraryChannelTemplatesResponse> { return { templates: [], blockPresets: [] }; }
  async restoreLibraryChannelDefaults(_input: LibraryChannelRestoreDefaultsRequest, _signal: AbortSignal): Promise<LibraryChannelRestoreDefaultsResponse> { return { items: [], createdCount: 0, existingCount: 1, skippedCount: 0 }; }
  async regenerateLibraryChannel(channelId: string, _signal: AbortSignal): Promise<LibraryChannelGeneration> { const horizonStart = new Date().toISOString(); const horizonEnd = new Date(Date.now() + 7 * 86_400_000).toISOString(); return { channelId, generationId: `fixture-generation-${channelId}`, configRevision: 1, horizonStart, horizonEnd, generatedThrough: horizonEnd, entryCount: 168, warnings: [] }; }
  async dvr(_signal: AbortSignal): Promise<DVRResult> {
    const now = Date.now();
    const availableRecordings: ActionableDVRRecording[] = [
      { id: 'fixture-recording-1', sourceId: 'fixture-live', channelId: 'fixture-channel-3', title: 'Saturday Cinema', status: 'complete', priority: 50, revision: 1, startsAt: new Date(now - 7_200_000).toISOString(), endsAt: new Date(now - 3_600_000).toISOString(), sizeBytes: 4_294_967_296, createdAt: new Date(now - 7_200_000).toISOString(), updatedAt: new Date(now - 3_600_000).toISOString(), actions: ['dvr.play', 'dvr.delete'] },
      { id: 'fixture-recording-2', sourceId: 'fixture-live', channelId: 'fixture-channel-4', title: 'Coastal Sports Live', status: 'scheduled', priority: 50, revision: 1, startsAt: new Date(now + 3_600_000).toISOString(), endsAt: new Date(now + 10_800_000).toISOString(), sizeBytes: 0, createdAt: new Date(now).toISOString(), updatedAt: new Date(now).toISOString(), actions: ['dvr.cancel', 'dvr.edit'] },
    ];
    const recordings = availableRecordings.filter((recording) => !this.deletedDVRRecordings.has(recording.id));
    const baseRule: ActionableDVRRule = { id: 'fixture-rule-1', profileId: 'fixture-owner-profile', sourceId: 'fixture-live', channelId: 'fixture-channel-4', title: 'Coastal Hockey', matchType: 'series', priority: 50, revision: 1, startPaddingMinutes: 2, endPaddingMinutes: 5, retentionDays: 30, maxRecordingsPerSeries: 10, enabled: true, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', actions: ['dvr.edit', 'dvr.disable', 'dvr.delete'] };
    const rule = { ...baseRule, ...this.fixtureDVRRuleState.get(baseRule.id) };
    rule.actions = ['dvr.edit', rule.enabled ? 'dvr.disable' : 'dvr.enable', 'dvr.delete'];
    return { recordings, rules: this.deletedDVRRules.has(rule.id) ? [] : [rule] };
  }
  async createDVRRecording(_input: { sourceId: string; channelId?: string; programId?: string; title: string; startsAt: string; endsAt: string }, _signal: AbortSignal): Promise<ActionableDVRRecording> { throw new Error('Fixture recording mutations are unavailable.'); }
  async createDVRRule(_input: { sourceId: string; channelId?: string; programId?: string; title: string; matchType: string }, _signal: AbortSignal): Promise<ActionableDVRRule> { throw new Error('Fixture recording rule mutations are unavailable.'); }
  async updateDVRRule(id: string, input: Partial<ActionableDVRRule> & { sourceId: string; title: string }, signal: AbortSignal) { this.fixtureDVRRuleState.set(id, input); const rule = (await this.dvr(signal)).rules.find((item) => item.id === id); if (!rule) throw new Error('Fixture recording rule not found.'); return rule; }
  async deleteDVRRecording(id: string, _signal: AbortSignal) { this.deletedDVRRecordings.add(id); }
  async deleteDVRRule(id: string, _signal: AbortSignal) { this.deletedDVRRules.add(id); }
  async startDVRPlayback(recordingId: string, signal: AbortSignal) {
    const recording = (await this.dvr(signal)).recordings.find((item) => item.id === recordingId);
    if (!recording) throw new Error('Fixture recording not found.');
    return this.playbackFor({ id: recording.id, title: recording.title, subtitle: 'DVR recording', year: 0, type: 'show', kind: 'recording', poster: '/brand/portico-wordmark-white.svg', backdrop: '/brand/portico-wordmark-white.svg', rating: '', length: 'Recorded', genre: 'DVR', actions: ['dvr.play'] }, []);
  }
}
