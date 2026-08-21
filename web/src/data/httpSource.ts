import {
  browserPlaybackClientProfile,
  browseFacetOptions,
  createPorticoClient,
  productMessage,
  resolveSearchRequest,
  resolveBrowseFacetEndpoint,
  viewerScopeFromAuthMe,
  type ApiSchema,
  type NotificationAudience,
  type NotificationInvalidation,
  type NotificationReceiptAction,
  type NotificationRecipient,
  type AuthCapabilitiesResponse,
  type BrowseFacetOption,
  type BrowseFacetSource,
  type LibraryBrowseCapabilities,
  type LibraryChannelAggregate,
  type AdminLibraryChannelListResponse,
  type LibraryChannelConfigurationRequest,
  type LibraryChannelGeneration,
  type LibraryChannelGuide,
  type LibraryChannelsGuide,
  type LibraryChannelListResponse,
  type LibraryChannelRestoreDefaultsRequest,
  type LibraryChannelRestoreDefaultsResponse,
  type LibraryChannelTemplatesResponse,
  type LyricSearchCandidate,
  type MediaGrant,
  type MediaTrickplaySet,
  type PlaybackHandoffRequest,
  type PlaybackPrepareNextRequest,
  type PlaybackPreparedResponse,
  type PlaybackProgressAcknowledgement,
  type PlaybackProgressInput,
  type PlaybackResponse,
  type PlaybackRestoreResponse,
  type PlaybackSessionQueueRequest,
  type PlaybackSessionQueueReplaceRequest,
  type PlaybackSessionQueueResponse,
  type PorticoClient,
  type HostedServicesClient,
  type RemoteAccessStatus,
  type SavedView,
  type SavedViewCreateRequest,
  type ServerManagedProfileCreateRequest,
  type ServerManagedProfileUpdateRequest,
  type ServerOwnerFeedbackUpdateRequest,
  type ServerOwnerNotificationRecipientDirectory,
  type ServerOwnerNoticeRequest,
  type ViewerFeedbackSubmission,
  type SearchContract,
  type SearchRequestOptions,
  type DownloadPreparation,
} from '@portico/client-core';
import { createBrowserHostedConnectionVault, type HostedConnectionVault } from '../runtime/hostedConnectionVault';
import type {
  BrowseExpression,
  LibraryFacet,
  LibraryPivotInput,
  LibraryPivotPage,
  LibraryPresentation,
} from '../features/library/libraryTypes';
import type {
  LiveTVChannelPage,
  LiveTVChannelPageInput,
  LiveTVGuidePageInput,
  LiveTVGuideWorkspacePage,
  DVRConsumerStatus,
} from '../features/live-tv/liveTypes';
import type {
  BrowsePivot,
  BrowseSortOption,
  ActionableDVRRecording,
  ActionableDVRRule,
  ActionableLiveTVChannel,
  ActionableLiveTVSource,
  DVRResult,
  HomeResult,
  HomeRow,
  LibraryCapabilities,
  LibraryBrowseInput,
  LibraryBrowseResult,
  LibraryNavigationPreferences,
  LibrarySummary,
  LiveTVGuideInput,
  LiveTVGuideResult,
  LocalProfileLoginChallenge,
  LocalProfileSelection,
  MediaItem,
  MediaChildrenPage,
  MediaDeleteResult,
  MediaDownloadOptions,
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
  SavedResourceKind,
  SavedResourceSummary,
  SavedResourceUpdateInput,
  SavedShareCandidatePage,
  Viewer,
  WebDisplayPreference,
  ProfileAdministrationProofInput,
  ProfilePINReauthentication,
} from './models';
import { createPlaybackSessionAdapter } from './playbackAdapter';

const LOCAL_PROFILE_INSTALLATION_KEY = 'portico.web.installation-id.v1';

function abortIfRequested(signal: AbortSignal): void {
  if (signal.aborted) throw signal.reason instanceof Error ? signal.reason : new DOMException('Request aborted', 'AbortError');
}

async function durableBrowserInstallationId(): Promise<string> {
  if (typeof window === 'undefined') throw new Error('Local profile sign-in requires a browser installation identity.');
  const existing = window.localStorage.getItem(LOCAL_PROFILE_INSTALLATION_KEY);
  if (existing?.trim()) return existing;
  const created = globalThis.crypto.randomUUID();
  window.localStorage.setItem(LOCAL_PROFILE_INSTALLATION_KEY, created);
  if (window.localStorage.getItem(LOCAL_PROFILE_INSTALLATION_KEY) !== created) {
    throw new Error('Portico could not verify the browser installation identity.');
  }
  return created;
}

type ApiAuthMe = ApiSchema<'AuthMe'>;
type ApiProfileSelectionResponse = ApiSchema<'ProfileSelectionResponse'>;
type ApiBrowserAccounts = ApiSchema<'BrowserAccountsResponse'>;
type ApiBrowserAccountMutation = ApiSchema<'BrowserAccountMutationResponse'>;
type ApiBrowseCapabilities = ApiSchema<'LibraryBrowseCapabilities'>;
type ApiBrowseRequest = ApiSchema<'BrowseLibraryRequest'>;
type ApiBrowseResponse = ApiSchema<'BrowseLibraryResponse'>;
type ApiLibrary = ApiSchema<'Library'>;
type ApiLibraryList = ApiSchema<'LibraryListResponse'>;
type ApiHomeResponse = ApiSchema<'HomeResponse'>;
type ApiHomeRow = ApiSchema<'HomeRow'>;
type ApiMediaCard = ApiSchema<'MediaCard'>;
type ApiMediaItem = ApiSchema<'MediaItem'>;
type ApiMediaMatchSearchResponse = ApiSchema<'MediaMatchSearchResponse'>;
type ApiProductContract = ApiSchema<'ProductContract'>;
type ApiSearchResponse = ApiSchema<'SearchResponse'>;
type ApiSuggestionsResponse = ApiSchema<'SuggestionsResponse'>;
type ApiLibraryCategoryList = ApiSchema<'LibraryCategoryListResponse'>;
type ApiLibraryCategory = ApiSchema<'LibraryCategory'>;
type ApiLibraryFacetValueList = ApiSchema<'LibraryFacetValueListResponse'>;
type ApiLibraryFacetValue = ApiSchema<'LibraryFacetValue'>;
type ApiDVRSchedule = ApiSchema<'GetDvrScheduleResponse'>;
type ApiCollectionPage = ApiSchema<'CollectionPage'>;
type ApiPlaylistPage = ApiSchema<'PlaylistPage'>;
type ApiSavedPlaylist = ApiSchema<'SavedPlaylist'>;
type ApiPlaylistEntryPage = ApiSchema<'PlaylistEntryPage'>;
type ApiPlaylistItemsBatchResponse = ApiSchema<'PlaylistItemsBatchResponse'>;
type ApiCollection = ApiSchema<'Collection'>;
type ApiCollectionMembershipBatchResponse = ApiSchema<'CollectionMembershipBatchResponse'>;
type ApiSavedShareCandidatePage = ApiSchema<'SavedShareCandidatePage'>;
type ApiSavedView = ApiSchema<'SavedView'>;
type ApiSavedViewPage = ApiSchema<'SavedViewPage'>;
type ApiUser = ApiSchema<'User'>;
type ApiAuthProfileAuthentication = ApiSchema<'ProfileAccountAuthenticationResponse'>;

export class LocalProfileSelectionRequiredError extends Error {
  constructor(readonly challenge: LocalProfileLoginChallenge) {
    super('Choose a viewing profile to continue.');
    this.name = 'LocalProfileSelectionRequiredError';
  }
}

const browserVault = createBrowserHostedConnectionVault();

type ProductContractRequest = {
  generation: number;
  controller: AbortController;
  promise: Promise<ApiProductContract>;
};

type HomeRowSource = Pick<ApiHomeRow, 'id' | 'title' | 'type'> & Partial<Pick<ApiHomeRow,
  | 'cacheTtlSeconds'
  | 'controls'
  | 'critical'
  | 'cursorCapable'
  | 'defaultVisible'
  | 'endpoint'
  | 'explanation'
  | 'hasMore'
  | 'kind'
  | 'libraryId'
  | 'nextCursor'
  | 'policyState'
  | 'priority'
  | 'privacySensitivity'
>> & {
	artworkShape?: HomeRow['artworkShape'];
  items?: ApiMediaItem[];
};

function abortedRequestError(message = 'Request aborted') {
  return new DOMException(message, 'AbortError');
}

function authoritativeRemoteAccessStatus(response: { setupRequired: boolean; remoteAccess: RemoteAccessStatus }): { setupRequired: boolean; remoteAccess: RemoteAccessStatus } {
  // Hosted Services owns server membership. A stale connection flag from a
  // local server must never make an unclaimed server appear account-owned in
  // the Web runtime while membership propagation catches up.
  if (response.remoteAccess.porticoConnected && response.remoteAccess.settings.claimStatus !== 'claimed') {
    return {
      ...response,
      remoteAccess: { ...response.remoteAccess, porticoConnected: false },
    };
  }
  return response;
}

function waitForSharedRequest<T>(request: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortedRequestError());
  return new Promise<T>((resolve, reject) => {
    const abort = () => reject(abortedRequestError());
    signal.addEventListener('abort', abort, { once: true });
    request.then(
      (value) => {
        signal.removeEventListener('abort', abort);
        resolve(value);
      },
      (reason) => {
        signal.removeEventListener('abort', abort);
        reject(reason);
      },
    );
  });
}

function durationLabel(seconds = 0) {
  if (!seconds) return '';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.round((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}m` : `${minutes}m`;
}

type ResourceResolver = (path: string) => string;
const identityResource: ResourceResolver = (path) => path;

function imagePath(
  value: string | undefined,
  fallback = '/brand/portico-wordmark-white.svg',
  resolveResource: ResourceResolver = identityResource,
  dimensions?: { width: number; height: number },
) {
  if (!value) return fallback;
  if (value.startsWith('/api/')) {
    const resolved = resolveResource(value);
    if (!dimensions) return resolved;
    const url = new URL(resolved, 'http://portico.invalid');
    url.searchParams.set('width', String(dimensions.width));
    url.searchParams.set('height', String(dimensions.height));
    return resolved.startsWith('http://') || resolved.startsWith('https://') ? url.toString() : `${url.pathname}${url.search}${url.hash}`;
  }
  if (value.startsWith('/') || value.startsWith('https://') || value.startsWith('http://127.0.0.1') || value.startsWith('http://localhost')) return value;
  return fallback;
}

function viewType(entityKind: string): MediaItem['type'] {
  // `type` predates the canonical entity taxonomy, but remains on the web view
  // model for compatibility. Preserve the server kind at runtime so consumers
  // never mistake episodes, live media, or audiobook entities for shows/music.
  return entityKind as MediaItem['type'];
}

function fieldText(fields: Record<string, unknown> | undefined, key: string) {
  const value = fields?.[key];
  if (typeof value === 'string') return value;
  if (Array.isArray(value)) return value.filter((item): item is string => typeof item === 'string').join(', ');
  return '';
}

function cardToMedia(item: ApiMediaCard, resolveResource: ResourceResolver = identityResource): MediaItem {
  const progressSeconds = item.userState.progressSeconds;
  const progress = item.durationSeconds && progressSeconds > 0 ? Math.min(100, Math.round((progressSeconds / item.durationSeconds) * 100)) : undefined;
  return {
    id: item.id,
    libraryId: item.libraryId,
    libraryName: item.libraryName,
    counts: item.counts,
    entityKind: item.entityKind,
    title: item.title,
    subtitle: item.subtitle ?? '',
    summary: item.summary,
    year: item.year ?? 0,
    type: viewType(item.entityKind),
    kind: item.entityKind as MediaItem['kind'],
    poster: imagePath(item.artwork.poster, undefined, resolveResource, { width: 480, height: 720 }),
    backdrop: imagePath(item.artwork.backdrop, imagePath(item.artwork.thumb, undefined, resolveResource, { width: 1280, height: 720 }), resolveResource, { width: 1280, height: 720 }),
    artwork: Object.fromEntries(Object.entries(item.artwork).flatMap(([role, value]) => {
      const url = imagePath(value, '', resolveResource);
      return url ? [[role, url]] : [];
    })),
    progress,
    progressSeconds,
    rating: fieldText(item.fields, 'contentRating') || productMessage('media.not-rated').text || '',
    length: durationLabel(item.durationSeconds),
    durationSeconds: item.durationSeconds,
    genre: fieldText(item.fields, 'genre'),
    actions: item.actions,
    availability: item.availability.status,
    watchlisted: item.userState.watchlisted,
    favorite: item.userState.favorite,
    watched: item.userState.watched,
  };
}

function detailToMedia(item: ApiMediaItem, includeRelations = true, resolveResource: ResourceResolver = identityResource): MediaItem {
  const entityKind = item.type;
  const progressSeconds = item.state.progressSeconds;
  const progress = item.durationSeconds && progressSeconds > 0 ? Math.min(100, Math.round((progressSeconds / item.durationSeconds) * 100)) : undefined;
  return {
    id: item.id,
    libraryId: item.libraryId,
    libraryName: item.libraryName,
    counts: item.counts,
    entityKind,
    title: item.title,
    subtitle: item.tagline ?? item.parentTitle ?? '',
    year: item.year ?? 0,
    type: viewType(entityKind),
    kind: entityKind as MediaItem['kind'],
    poster: imagePath(item.images.poster, undefined, resolveResource, { width: 480, height: 720 }),
    backdrop: imagePath(item.displayImages?.backdrop ?? item.images.backdrop, imagePath(item.images.thumb, undefined, resolveResource, { width: 1280, height: 720 }), resolveResource, { width: 1280, height: 720 }),
    artwork: Object.fromEntries(Object.entries(item.artwork ?? {}).flatMap(([role, value]) => {
      const url = imagePath(value, '', resolveResource);
      return url ? [[role, url]] : [];
    })),
    progress,
    progressSeconds,
    rating: item.contentRating ?? productMessage('media.not-rated').text ?? '',
    length: durationLabel(item.durationSeconds),
    durationSeconds: item.durationSeconds,
    genre: item.genres.join(', '),
    addedAt: item.addedAt,
    actions: item.actions,
    missing: item.missing,
    watchlisted: item.state.watchlisted,
    favorite: item.state.favorite,
    watched: item.state.watched,
    reaction: item.state.reaction ?? '',
    userRating: item.state.rating,
    summary: item.summary,
    tagline: item.tagline,
    sortTitle: item.sortTitle,
    originalTitle: item.originalTitle,
    edition: item.edition,
    contentRating: item.contentRating,
    communityRating: item.communityRating,
    criticRating: item.criticRating,
    country: item.country,
    studio: item.studio,
    network: item.network,
    tags: item.tags,
    labels: item.labels,
    lockedFields: item.lockedFields,
    providerIds: item.providerIds,
    metadataEvidence: item.metadataEvidence,
    parentId: item.parentId,
    parentTitle: item.parentTitle,
    grandparentId: item.grandparentId,
    grandparentTitle: item.grandparentTitle,
    seasonNumber: item.seasonNumber,
    episodeNumber: item.episodeNumber,
    indexNumber: item.indexNumber,
    fileCount: item.fileCount,
    missingFileCount: item.missingFileCount,
    people: item.people?.map((person) => ({
      ...person,
      imageUrl: imagePath(person.imageUrl, '', resolveResource, { width: 192, height: 192 }) || undefined,
    })),
    extras: includeRelations ? item.extras?.map((relationship) => ({
      label: relationship.label,
      type: relationship.type,
      items: relationship.items.map((extra) => detailToMedia(extra, false, resolveResource)),
    })) : undefined,
    mediaFiles: item.mediaFiles,
    optimizedVersions: item.optimizedVersions?.map((version) => ({
      ...version,
      available: Boolean(version.path || version.streamUrl || version.downloadUrl),
    })),
    streams: item.streams,
    typedMetadata: item.typedMetadata,
    mediaImages: item.mediaImages,
    attachments: item.attachments,
    lyrics: item.lyrics,
    children: item.children?.map((child) => detailToMedia(child, false, resolveResource)),
    childrenTruncated: item.childrenTruncated,
    chapters: item.chapters,
    playbackTarget: includeRelations && item.playbackTarget ? detailToMedia(item.playbackTarget, false, resolveResource) : undefined,
    recommendationRows: includeRelations ? item.recommendationRows?.map((row) => homeRowToView(row, resolveResource)) : undefined,
  };
}

function homeRowToView(row: HomeRowSource, resolveResource: ResourceResolver = identityResource): HomeRow {
  const policy = row as HomeRowSource & { required?: boolean; hideable?: boolean; reorderable?: boolean };
  return {
    id: row.id,
    title: row.title,
    detail: row.explanation,
    type: row.type,
		artworkShape: row.artworkShape,
    items: (row.items ?? []).map((item) => detailToMedia(item, false, resolveResource)),
    hasMore: row.hasMore ?? false,
    nextCursor: row.nextCursor,
    endpoint: row.endpoint,
    explanation: row.explanation,
    controls: row.controls,
    required: policy.required,
    hideable: policy.hideable,
    reorderable: policy.reorderable,
    critical: row.critical,
    cursorCapable: row.cursorCapable,
    defaultVisible: row.defaultVisible,
    cacheTtlSeconds: row.cacheTtlSeconds,
    kind: row.kind,
    libraryId: row.libraryId,
    policyState: row.policyState,
    priority: row.priority,
    privacySensitivity: row.privacySensitivity,
  };
}

function viewerFromAuth(response: ApiAuthMe, resolveResource: ResourceResolver = identityResource): Viewer {
  const viewerScope = response.authenticated ? viewerScopeFromAuthMe(response) : undefined;
  return {
    authenticated: response.authenticated,
    setupRequired: response.setupRequired,
    serverName: response.serverFriendlyName ?? 'Portico Server',
    user: response.user ? userView(response.user, resolveResource) : undefined,
    viewerScope,
  };
}

function userView(user: ApiUser, resolveResource: ResourceResolver = identityResource): NonNullable<Viewer['user']> {
  const musicPlayback = user.preferences.musicPlayback ?? {
    shuffleDefault: false,
    repeatDefault: 'none',
    autoplayDefault: true,
    normalizationMode: 'attenuate',
    crossfadeSeconds: 0,
    gapless: true,
  };
  return {
    id: user.id,
    displayName: user.displayName,
    email: user.email,
    role: user.role,
    profileImageUrl: imagePath(user.profileImageUrl, '', resolveResource, { width: 192, height: 192 }) || undefined,
    authOrigin: user.authOrigin,
    authProvider: user.authProvider,
    hasLocalPassword: user.hasLocalPassword,
    permissions: user.permissions,
    preferences: {
      sidebarOrder: user.preferences.sidebarOrder,
      musicPlayback: {
        shuffleDefault: musicPlayback.shuffleDefault,
        repeatDefault: musicPlayback.repeatDefault === 'one' || musicPlayback.repeatDefault === 'all'
          ? musicPlayback.repeatDefault
          : 'none',
        autoplayDefault: musicPlayback.autoplayDefault,
        normalizationMode: musicPlayback.normalizationMode === 'attenuate' ? 'attenuate' : 'off',
        crossfadeSeconds: Math.max(0, Math.min(12, Math.floor(musicPlayback.crossfadeSeconds))),
        gapless: musicPlayback.gapless,
      },
    },
  };
}

function kindMatches(libraryType: string, requested: LibraryBrowseInput['kind']) {
  const mapping: Record<LibraryBrowseInput['kind'], string> = {
    movies: 'movie',
    tv: 'show',
    anime: 'anime',
    music: 'music',
    audiobooks: 'audiobook',
    'recorded-tv': 'recorded-tv',
  };
  return libraryType === mapping[requested];
}

function canonicalPivot(value: string) {
  return value.trim().toLocaleLowerCase().replaceAll(' ', '-');
}

function canonicalSort(value: string) {
  return ({ Title: 'title', 'Recently added': 'dateAdded', Year: 'year', 'Critic rating': 'criticRating' } as Record<string, string>)[value] ?? value;
}

function queryForFilter(filter: string): ApiBrowseRequest['query'] {
  if (filter === 'In progress') return { field: 'playState', operator: 'equals', value: 'in-progress' };
  if (filter === 'Unwatched') return { field: 'playState', operator: 'equals', value: 'unplayed' };
  return undefined;
}

function mapPivots(capabilities: ApiBrowseCapabilities): BrowsePivot[] {
  return capabilities.pivots.map((pivot) => ({
    id: pivot.id,
    label: pivot.label,
    entityKinds: pivot.entityKinds,
    defaultView: libraryWorkspacePresentation(pivot.defaultView),
    supportedViews: [...new Set(pivot.supportedViews.map(libraryWorkspacePresentation))],
    browseSupported: pivot.browseSupported,
    endpointTemplate: pivot.endpointTemplate,
  }));
}

function normalizeBrowseCapabilities(capabilities: ApiBrowseCapabilities): LibraryBrowseCapabilities {
  const normalizePivot = (pivot: ApiBrowseCapabilities['pivots'][number]) => ({
    ...pivot,
    defaultView: libraryWorkspacePresentation(pivot.defaultView),
    supportedViews: [...new Set(pivot.supportedViews.map(libraryWorkspacePresentation))],
  });
  return {
    ...capabilities,
    pivots: capabilities.pivots.map(normalizePivot),
    resolvedPivot: capabilities.resolvedPivot ? normalizePivot(capabilities.resolvedPivot) : undefined,
  };
}

function mapSorts(capabilities: ApiBrowseCapabilities): BrowseSortOption[] {
  return capabilities.sorts.map((sort) => ({ id: sort.id, label: sort.label, defaultDirection: sort.defaultDirection }));
}

function typeForLibrary(kind: LibraryBrowseInput['kind']): MediaItem['type'] {
  if (kind === 'music' || kind === 'audiobooks') return 'music';
  return kind === 'movies' ? 'movie' : 'show';
}

function summaryToMedia(
  item: { id: string; title: string; summary?: string; itemCount?: number },
  kind: 'collection' | 'playlist',
  libraryKind: LibraryBrowseInput['kind'],
): MediaItem {
  return {
    id: item.id,
    title: item.title,
    subtitle: productMessage(item.itemCount === 1 ? 'media.item-count-single' : 'media.item-count', { count: item.itemCount ?? 0 }).text ?? '',
    year: 0,
    type: typeForLibrary(libraryKind),
    kind,
    poster: '/brand/portico-wordmark-white.svg',
    backdrop: '/brand/portico-wordmark-white.svg',
    rating: '',
    length: '',
    genre: '',
    summary: item.summary,
    availability: 'available',
  };
}

function listResourceSummary(
  item: ApiSavedPlaylist | ApiCollection | ApiPlaylistPage['items'][number] | ApiCollectionPage['items'][number],
  kind: 'playlist' | 'collection',
): SavedResourceSummary {
  return {
    id: item.id,
    kind,
    ownerUserId: item.ownerUserId,
    title: item.title,
    summary: item.summary,
    itemCount: item.itemCount,
    canEdit: item.canEdit,
    visibility: item.visibility,
    shares: 'shares' in item && Array.isArray(item.shares)
      ? item.shares.map((share) => ({ userId: share.userId, displayName: share.displayName, canEdit: share.canEdit }))
      : undefined,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

function savedViewSummary(item: ApiSavedView): SavedResourceSummary {
  return {
    id: item.id,
    kind: 'view',
    title: item.title,
    itemCount: 0,
    canEdit: true,
    libraryId: item.libraryId,
    libraryName: item.libraryName,
    pivot: item.pivot,
    isPinned: item.isPinned,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

function categoryToMedia(
  category: ApiLibraryCategory,
  libraryKind: LibraryBrowseInput['kind'],
): MediaItem {
  return {
    id: category.id,
    title: category.name,
    subtitle: `${category.count} ${category.count === 1 ? 'title' : 'titles'}`,
    year: 0,
    type: typeForLibrary(libraryKind),
    kind: 'category',
    poster: imagePath(category.image),
    backdrop: imagePath(category.image),
    rating: '',
    length: '',
    genre: category.group,
    summary: category.description,
    availability: 'available',
  };
}

function libraryWorkspacePresentation(value: string): LibraryPresentation {
  if (value === 'compact') return 'compact-grid';
  if (value === 'timeline') return 'list';
  if (['shelves', 'grid', 'compact-grid', 'list', 'table', 'facets'].includes(value)) return value as LibraryPresentation;
  return 'grid';
}

function libraryFacetPredicate(filter: string, fallbackField: string): BrowseExpression {
  const separator = filter.indexOf(':');
  const prefix = separator > 0 ? filter.slice(0, separator) : fallbackField;
  const rawValue = separator > 0 ? filter.slice(separator + 1) : filter;
  const field = ({ rating: 'contentRating', label: 'tag' } as Record<string, string>)[prefix] ?? prefix;
  const numeric = field === 'year' || field === 'decade';
  return {
    field,
    operator: field === 'genre' || field === 'tag' ? 'contains' : 'equals',
    value: numeric && Number.isFinite(Number(rawValue)) ? Number(rawValue) : rawValue,
  };
}

function dvrRecordingToMedia(recording: ApiDVRSchedule['items'][number]): MediaItem {
  const starts = new Date(recording.startsAt);
  const ends = new Date(recording.endsAt);
  const minutes = Math.max(0, Math.round((ends.getTime() - starts.getTime()) / 60_000));
  return {
    id: recording.id,
    libraryId: undefined,
    entityKind: 'recording',
    title: recording.title,
    subtitle: `${recording.status} · ${starts.toLocaleString()}`,
    year: Number.isFinite(starts.getTime()) ? starts.getFullYear() : 0,
    type: 'show',
    kind: 'recording',
    poster: '/brand/portico-wordmark-white.svg',
    backdrop: '/brand/portico-wordmark-white.svg',
    rating: '',
    length: minutes ? `${minutes}m` : '',
    genre: 'Recorded TV',
    summary: recording.status === 'incomplete'
      ? 'A playable partial recording was retained after capture stopped early.'
      : recording.failureMessageId
        ? 'Recording failed. The server owner can review tuner and storage health.'
        : `${recording.status === 'running' ? 'Recording now' : 'Scheduled recording'} on this server.`,
    availability: recording.status === 'failed' ? 'unavailable' : 'available',
    actions: recording.actions,
  };
}

function nonBrowsePage(
  input: LibraryPivotInput,
  values: Pick<LibraryPivotPage, 'items' | 'sections' | 'facets' | 'total' | 'nextCursor' | 'hasMore'>,
): LibraryPivotPage {
  return {
    ...values,
    applied: {
      pivot: input.pivot.id,
      sort: input.pivot.defaultSort,
      presentationFields: input.pivot.presentationFields,
    },
    presentation: libraryWorkspacePresentation(input.pivot.defaultView),
  };
}

export class HttpPorticoDataSource implements PorticoDataSource {
  private readonly client: PorticoClient;
  private readonly installationId: () => Promise<string>;
  private readonly playbackClientInstanceId = `web-${globalThis.crypto.randomUUID()}`;
  private readonly playbackAdapter = createPlaybackSessionAdapter();
  private contract?: ApiProductContract;
  private contractRequest?: ProductContractRequest;
  private contractGeneration = 0;
  private observedViewerScope = false;
  private viewerScopeIdentity = '';
  private capabilities = new Map<string, ApiBrowseCapabilities>();
  private serverName = 'Portico Server';
  private activeViewer?: Viewer;
  private hostedProfileRevision = 0;
  private pendingLocalProfileAuthentication?: {
    authenticated: ApiAuthProfileAuthentication;
    rememberOnBrowser: boolean;
  };
	private readonly hostedOptions?: {
		hostedClient: HostedServicesClient;
		connectionVault: HostedConnectionVault;
		switchHostedProfile: (profileId: string, pin?: string) => Promise<Viewer>;
	};
	private readonly bundledConnectionVault: HostedConnectionVault;
	private readonly resolveResource: ResourceResolver = (path) => this.client.resourceUrl(path);
	private readonly mediaCard = (item: ApiMediaCard) => cardToMedia(item, this.resolveResource);
	private readonly mediaDetail = (item: ApiMediaItem, includeRelations = true) => detailToMedia(item, includeRelations, this.resolveResource);
	private readonly homeRowView = (row: HomeRowSource) => homeRowToView(row, this.resolveResource);
	private readonly authViewer = (response: ApiAuthMe) => viewerFromAuth(response, this.resolveResource);
	private readonly accountUser = (user: ApiUser) => userView(user, this.resolveResource);

  constructor(
	client?: PorticoClient,
	installationOrHostedOptions: (() => Promise<string>) | {
		hostedClient: HostedServicesClient;
		connectionVault: HostedConnectionVault;
		switchHostedProfile: (profileId: string, pin?: string) => Promise<Viewer>;
	} = durableBrowserInstallationId,
	bundledConnectionVault: HostedConnectionVault = browserVault,
  ) {
    this.client = client ?? createPorticoClient({
      // Bundled Web uses relative API routes, but media elements need an
      // absolute URL rooted at the server that actually served this app. The
      // platform-neutral client cannot infer a browser origin on its own.
      baseHref: () => typeof window === 'undefined' ? 'http://portico.local' : window.location.href,
      playbackClientProfile: browserPlaybackClientProfile,
    });
	this.installationId = typeof installationOrHostedOptions === 'function'
		? installationOrHostedOptions
		: durableBrowserInstallationId;
	this.hostedOptions = typeof installationOrHostedOptions === 'function'
		? undefined
		: installationOrHostedOptions;
	this.bundledConnectionVault = bundledConnectionVault;
  }

  porticoClient(): PorticoClient {
    return this.client;
  }

  private connectionVault(): HostedConnectionVault {
    return this.hostedOptions?.connectionVault ?? this.bundledConnectionVault;
  }

  private async rememberAuthoritativeProfile(viewer: Viewer, signal: AbortSignal): Promise<void> {
    const identity = viewer.viewerScope;
    if (!identity) return;
    const installationId = await this.connectionVault().installationId();
    try {
      let bundle = await this.client.viewerPreferenceBundle({}, { signal });
      if (bundle.accountServerInstallation.values.lastProfileId !== identity.profileId) {
        const updated = await this.client.recordViewerProfileActivation({
          version: 'v1', expectedRevision: bundle.accountServerInstallation.revision,
        }, { signal });
        bundle = { ...bundle, accountServerInstallation: updated };
      }
      await this.connectionVault().saveProfileLaunchPreference({ ...identity, deviceClass: 'web', installationId }, {
        ...bundle.accountServerInstallation.values,
        lastProfileId: identity.profileId,
      });
    } catch {
      // The profile session is already authoritative. Preference persistence is
      // retryable and must not roll back an identity transition that succeeded.
    }
  }

  /** Clears every cache whose contents can vary by profile or authorization policy. */
  invalidateProductContract(): void {
    this.contractGeneration += 1;
    this.contractRequest?.controller.abort();
    this.contract = undefined;
    this.contractRequest = undefined;
    this.capabilities.clear();
  }

  private recordViewerScope(viewer: Viewer | undefined, force = false): void {
    const scope = viewer?.viewerScope;
    const identity = scope
      ? JSON.stringify([scope.authority, scope.accountId, scope.serverId, scope.profileId, scope.authorizationRevision])
      : 'signed-out';
    if (!force && this.observedViewerScope && identity === this.viewerScopeIdentity) return;
    this.observedViewerScope = true;
    this.viewerScopeIdentity = identity;
    this.invalidateProductContract();
  }

  async authCapabilities(signal: AbortSignal) {
    const response = await this.client.request<AuthCapabilitiesResponse>('/api/auth/capabilities', { signal });
    if (response.serverFriendlyName) this.serverName = response.serverFriendlyName;
    return {
      setupRequired: response.setupRequired,
      localCredentialsEnabled: response.localCredentialsEnabled,
      porticoAccountAuthEnabled: response.porticoAccountAuthEnabled,
      serverFriendlyName: response.serverFriendlyName ?? this.serverName,
      publicUserPickerEnabled: response.publicUserPickerEnabled,
      visibleUsers: response.visibleUsers,
    };
  }

  async viewer(signal: AbortSignal): Promise<Viewer> {
    const response = await this.client.request<ApiAuthMe>('/api/auth/me', { signal });
    const viewer = this.authViewer(response);
    this.recordViewerScope(viewer);
    this.serverName = viewer.serverName;
    this.activeViewer = viewer;
    return viewer;
  }

	async browserAccounts(signal: AbortSignal) {
		const response = await this.client.request<ApiBrowserAccounts>('/api/auth/browser-accounts', { signal });
		return {
			...response,
			accounts: response.accounts.map((account) => ({
				...account,
				profileImageUrl: imagePath(account.profileImageUrl, '', this.resolveResource, { width: 192, height: 192 }) || undefined,
			})),
		};
	}

	async switchBrowserAccount(accountId: string, signal: AbortSignal): Promise<Viewer> {
		const authenticated = await this.client.request<ApiAuthProfileAuthentication>('/api/auth/browser-accounts/switch', {
			method: 'POST',
			body: { accountId },
			signal,
		});
		const installationId = await this.connectionVault().installationId();
		if (signal.aborted) throw signal.reason ?? new DOMException('Request aborted', 'AbortError');
		if (authenticated.directory.authority !== 'local'
			|| authenticated.directory.accountId !== accountId
			|| Date.parse(authenticated.expiresAt) <= Date.now()) {
			throw new Error('The remembered local account proof is invalid or expired.');
		}
		const challenge: LocalProfileLoginChallenge = {
			...authenticated,
			installationId,
			rememberOnBrowser: true,
		};
		const preferenceScope = {
			authority: 'local' as const,
			accountId: authenticated.directory.accountId,
			serverId: authenticated.directory.serverId,
			profileId: authenticated.directory.profiles.find((profile) => profile.isPrimary)?.id ?? authenticated.directory.profiles[0]?.id ?? '',
			deviceClass: 'web' as const,
			installationId,
		};
		const preference = preferenceScope.profileId
			? await this.connectionVault().profileLaunchPreference(preferenceScope)
			: undefined;
		const requestedProfile = preference?.profileSelection === 'last-used' && preference.lastProfileId
			? authenticated.directory.profiles.find((profile) => profile.id === preference.lastProfileId)
			: authenticated.directory.profiles.length === 1 && !authenticated.directory.profiles[0]?.hasPIN
				? authenticated.directory.profiles[0]
				: undefined;
		if (!requestedProfile || requestedProfile.hasPIN) {
			throw new LocalProfileSelectionRequiredError(challenge);
		}
		const selection = await this.verifyLocalProfileSelection(challenge, requestedProfile.id, undefined, signal);
		const viewer = await this.publishLocalProfileSession(selection, signal);
		this.activeViewer = viewer;
		await this.rememberAuthoritativeProfile(viewer, signal);
		this.recordViewerScope(viewer, true);
		this.serverName = viewer.serverName;
		return viewer;
	}

	async updateBrowserAccountPreferences(automaticSignIn: boolean, signal: AbortSignal) {
		return this.client.request<{ automaticSignIn: boolean }>('/api/auth/browser-accounts/preferences', {
			method: 'PATCH',
			body: { automaticSignIn },
			signal,
		});
	}

	async removeBrowserAccount(accountId: string, signal: AbortSignal) {
		const response = await this.client.request<ApiBrowserAccountMutation>('/api/auth/browser-accounts/remove', {
			method: 'POST',
			body: { accountId },
			signal,
		});
		this.invalidateProductContract();
		return response;
	}

	async signOutAllBrowserAccounts(signal: AbortSignal): Promise<void> {
		await this.client.request<ApiBrowserAccountMutation>('/api/auth/browser-accounts/sign-out-all', { method: 'POST', signal });
		this.recordViewerScope(undefined, true);
	}

  async beginLocalProfileLogin(
    credentials: { login: string; password: string; rememberOnBrowser?: boolean },
    signal: AbortSignal,
  ): Promise<LocalProfileLoginChallenge> {
    const installationId = await this.installationId();
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const response = await this.client.authenticateLocalProfileAccount({
      app: 'portico-web',
      deviceName: 'Web browser',
      installationId,
      login: credentials.login,
      password: credentials.password,
      platform: 'web',
      purpose: 'browser',
    }, {signal});
    if (response.directory.authority !== 'local' || Date.parse(response.expiresAt) <= Date.now()) {
      throw new Error('The local profile authentication proof is invalid or expired.');
    }
    return {
      ...response,
      installationId,
      rememberOnBrowser: credentials.rememberOnBrowser !== false,
    };
  }

  async verifyLocalProfileSelection(
    challenge: LocalProfileLoginChallenge,
    profileId: string,
    pin: string | undefined,
    signal: AbortSignal,
  ): Promise<LocalProfileSelection> {
    if (Date.parse(challenge.expiresAt) <= Date.now()) throw new Error('The local profile authentication proof has expired.');
    const expectedProfile = challenge.directory.profiles.find((profile) => profile.id === profileId);
    if (!expectedProfile) throw new Error('That local profile is no longer available.');
    if (expectedProfile.hasPIN && !/^\d{4}$/.test(pin ?? '')) throw new Error('Enter the four-digit profile PIN.');
    const grant = await this.client.selectLocalProfile({
      accountAuthenticationToken: challenge.accountAuthenticationToken,
      profileId,
      ...(pin ? {pin} : {}),
    }, {signal});
    if (grant.authority !== 'local'
      || grant.accountId !== challenge.directory.accountId
      || grant.serverId !== challenge.directory.serverId
      || grant.profileId !== profileId
      || grant.pinRevision !== expectedProfile.pinRevision
      || Date.parse(grant.expiresAt) <= Date.now()) {
      throw new Error('The local profile selection grant does not match the requested profile.');
    }
    return {challenge, grant};
  }

  async publishLocalProfileSession(selection: LocalProfileSelection, signal: AbortSignal): Promise<Viewer> {
    const {challenge, grant} = selection;
    if (Date.parse(grant.expiresAt) <= Date.now()
      || grant.authority !== 'local'
      || grant.accountId !== challenge.directory.accountId
      || grant.serverId !== challenge.directory.serverId) {
      throw new Error('The local profile selection grant is invalid or expired.');
    }
    const response = await this.client.createBrowserProfileSession({
      rememberOnBrowser: challenge.rememberOnBrowser,
      selectionGrant: grant.token,
    }, {signal});
    const viewer = this.authViewer(response);
    const scope = viewer.viewerScope;
    if (!viewer.authenticated || !viewer.user || !scope
      || scope.authority !== 'local'
      || scope.accountId !== grant.accountId
      || scope.serverId !== grant.serverId
      || scope.profileId !== grant.profileId) {
      throw new Error('The server opened a different local profile than the one selected.');
    }
    await this.client.checkCompatibility({ signal });
    this.serverName = viewer.serverName;
    return viewer;
  }

  async switchAuthenticatedLocalProfile(profileId: string, pin: string | undefined, signal: AbortSignal): Promise<Viewer> {
    const current = this.activeViewer?.viewerScope;
    if (!current || current.authority !== 'local') throw new Error('An active Local Auth session is required to switch profiles.');
    const grant = await this.client.request<ApiProfileSelectionResponse>('/api/auth/profile-selections/session', {
      method: 'POST',
      body: { profileId, ...(pin ? { pin } : {}) },
      signal,
    });
    if (grant.authority !== 'local'
      || grant.accountId !== current.accountId
      || grant.serverId !== current.serverId
      || grant.profileId !== profileId
      || Date.parse(grant.expiresAt) <= Date.now()) {
      throw new Error('The local profile selection grant does not match the requested profile.');
    }
    const response = await this.client.createBrowserProfileSession({
      rememberOnBrowser: true,
      selectionGrant: grant.token,
    }, { signal });
    const viewer = this.authViewer(response);
    const scope = viewer.viewerScope;
    if (!viewer.authenticated || !scope
      || scope.authority !== 'local'
      || scope.accountId !== current.accountId
      || scope.serverId !== current.serverId
      || scope.profileId !== profileId) {
      throw new Error('The server opened a different local profile than the one selected.');
    }
    await this.client.checkCompatibility({ signal });
    this.activeViewer = viewer;
    this.serverName = viewer.serverName;
    await this.rememberAuthoritativeProfile(viewer, signal);
    this.recordViewerScope(viewer, true);
    return viewer;
  }

  async login(credentials: { login: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer> {
    const response = await this.client.request<ApiAuthMe>('/api/auth/login', { method: 'POST', body: credentials, signal });
    const viewer = this.authViewer(response);
    if (viewer.authenticated) await this.client.checkCompatibility({ signal });
    this.activeViewer = viewer;
    await this.rememberAuthoritativeProfile(viewer, signal);
    this.recordViewerScope(viewer, true);
    this.serverName = viewer.serverName;
    return viewer;
  }

  async setup(details: { serverName: string; username: string; email: string; displayName: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer> {
    const response = await this.client.request<ApiAuthMe>('/api/auth/setup', {
      method: 'POST',
      body: { ...details, setupMode: 'local_only', localOnlyAcknowledged: true },
      signal,
    });
    const viewer = this.authViewer(response);
    if (viewer.authenticated) await this.client.checkCompatibility({ signal });
    this.recordViewerScope(viewer, true);
    this.serverName = viewer.serverName;
    return viewer;
  }

  async startPorticoSetup(serverName: string, signal: AbortSignal): Promise<{ claimUrl: string; expiresAt?: string }> {
	const response = await this.client.request<{ remoteAccess: RemoteAccessStatus }>('/api/auth/portico-setup/claim/start', { method: 'POST', body: { serverName }, signal });
    const claim = response.remoteAccess.claim;
    if (!claim?.claimUrl) throw new Error('Portico Account setup did not return a claim link.');
    return { claimUrl: claim.claimUrl, expiresAt: claim.expiresAt };
  }

  async porticoSetupStatus(signal: AbortSignal): Promise<{ setupRequired: boolean; remoteAccess: RemoteAccessStatus }> {
    return authoritativeRemoteAccessStatus(await this.client.request<{ setupRequired: boolean; remoteAccess: RemoteAccessStatus }>('/api/auth/portico-setup/status', { signal }));
  }

  async logout(signal: AbortSignal): Promise<void> {
    await this.client.request<{ ok: boolean }>('/api/auth/logout', { method: 'POST', signal });
    this.recordViewerScope(undefined, true);
  }

  async updateProfile(profile: { displayName: string; email: string }, signal: AbortSignal): Promise<Viewer> {
    const user = await this.client.request<ApiUser>('/api/account/profile', { method: 'PATCH', body: profile, signal });
    return { authenticated: true, setupRequired: false, serverName: this.serverName, user: this.accountUser(user) };
  }

  async viewerPreferences(signal: AbortSignal) {
    // Preference identity belongs to the authenticated app session. Asking the
    // browser to repeat an installation identifier here made an otherwise valid
    // session fragile when vault state was restored, rotated, or quarantined.
    return this.client.viewerPreferenceBundle({}, { signal });
  }

  async patchViewerPreference(
    scope: 'profile-server' | 'profile-device-class' | 'account-server-installation',
    expectedRevision: number,
    changes: Record<string, unknown>,
    signal: AbortSignal,
  ) {
    if (scope === 'account-server-installation' && Object.prototype.hasOwnProperty.call(changes, 'lastProfileId')) {
      throw new TypeError('lastProfileId is updated only after authoritative profile activation');
    }
    const document = await this.client.patchViewerPreferenceDocument(scope, {
      // The server derives both values from the active session. This prevents a
      // stale browser vault from selecting (or accidentally creating) another
      // installation's preference document.
    }, { version: 'v1', expectedRevision, changes }, { signal });
    if (scope === 'account-server-installation' && this.activeViewer?.viewerScope) {
      const identity = this.activeViewer.viewerScope;
      const installationId = await this.connectionVault().installationId();
      await this.connectionVault().saveProfileLaunchPreference({
        ...identity,
        deviceClass: 'web',
        installationId,
      }, document.values as { rememberAccount: boolean; profileSelection: 'ask' | 'last-used'; lastProfileId?: string });
    }
    return document;
  }

  async accountProfiles(signal: AbortSignal) {
    const serverDirectory = await this.client.accountProfiles({ signal });
    if (serverDirectory.authority !== 'hosted' || !this.hostedOptions) return serverDirectory;
    const cloud = await this.hostedOptions.hostedClient.profiles({ signal });
    // A Portico Server stores a server-local mirror ID for the account while
    // Hosted Services owns the globally scoped Portico Account ID. Those IDs
    // are intentionally different namespaces. The Hosted session determines
    // which account directory can be returned, and the server independently
    // verifies the signed profile-selection assertion before changing viewer
    // scope, so comparing the two IDs rejects legitimate hosted accounts
    // without adding a security boundary.
    this.hostedProfileRevision = cloud.revision;
    return {
      ...serverDirectory,
      profiles: cloud.profiles,
      canManage: cloud.profiles.some((profile) => profile.id === this.activeViewer?.viewerScope?.profileId && profile.isAccountAdmin),
    };
  }

  async createProfileAdministrationProof(input: ProfileAdministrationProofInput, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      return this.hostedOptions.hostedClient.createProfileAdministrationSession({
        ...(input.pin ? { pin: input.pin } : {}),
        ...(input.password ? { password: input.password } : {}),
        ...(input.replacementPin ? { replacementPin: input.replacementPin } : {}),
        ...(input.mfaCode ? { mfaCode: input.mfaCode } : {}),
        ...(input.recoveryCode ? { recoveryCode: input.recoveryCode } : {}),
        ...(input.emailRecoveryToken ? { emailRecoveryToken: input.emailRecoveryToken } : {}),
      }, { signal });
    }
    return this.client.createProfileAdministrationProof({ pin: input.pin, password: input.password }, { signal });
  }

  async porticoProfileMFAStatus(signal: AbortSignal): Promise<{ enabled: boolean }> {
    if (this.activeViewer?.viewerScope?.authority !== 'hosted' || !this.hostedOptions) throw new Error('Portico Account security is unavailable for Local Auth.');
    abortIfRequested(signal);
    const status = await this.hostedOptions.hostedClient.mfaStatus();
    abortIfRequested(signal);
    return { enabled: status.enabled === true };
  }

  async requestPorticoProfileRecoveryEmail(signal: AbortSignal): Promise<void> {
    if (this.activeViewer?.viewerScope?.authority !== 'hosted' || !this.hostedOptions) throw new Error('Portico Account recovery is unavailable for Local Auth.');
    const email = this.activeViewer.user?.email?.trim();
    if (!email) throw new Error('Your Portico Account email is unavailable.');
    abortIfRequested(signal);
    await this.hostedOptions.hostedClient.requestPasswordReset({ email });
    abortIfRequested(signal);
  }

  async createAccountProfile(input: ServerManagedProfileCreateRequest, proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      if (input.pin) throw new Error('Create the profile first, then add its PIN with the Portico Account password.');
      const result = await this.hostedOptions.hostedClient.createProfile({
        name: input.name,
        avatarKey: input.avatar?.reference ?? '',
        restrictions: input.policy,
      }, { token: proof }, { signal });
      this.hostedProfileRevision = result.revision;
      return result.profile;
    }
    return this.client.createAccountProfile(input, proof, { signal });
  }

  async updateAccountProfile(profileId: string, input: ServerManagedProfileUpdateRequest, proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      const current = await this.hostedOptions.hostedClient.profile(profileId, { signal });
      const result = await this.hostedOptions.hostedClient.updateProfile(profileId, {
        name: input.name ?? current.name,
        avatarKey: input.avatar?.reference ?? current.avatar?.reference ?? '',
        restrictions: input.policy ?? current.policy,
        expectedRevision: this.hostedProfileRevision,
      }, { token: proof }, { signal });
      this.hostedProfileRevision = result.revision;
      return result.profile;
    }
    return this.client.updateAccountProfile(profileId, input, proof, { signal });
  }

  async deleteAccountProfile(profileId: string, proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      const result = await this.hostedOptions.hostedClient.deleteProfile(profileId, { token: proof }, { signal });
      this.hostedProfileRevision = result.revision;
      return;
    }
    return this.client.deleteAccountProfile(profileId, proof, { signal });
  }

  async reorderAccountProfiles(profileIds: string[], proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      const result = await this.hostedOptions.hostedClient.reorderProfiles({ profileIds, expectedRevision: this.hostedProfileRevision }, { token: proof }, { signal });
      this.hostedProfileRevision = result.revision;
      const server = await this.client.accountProfiles({ signal });
      return { ...server, profiles: result.profiles, canManage: true };
    }
    return this.client.reorderAccountProfiles({ profileIds }, proof, { signal });
  }

  async setAccountProfilePin(profileId: string, input: { pin: string } & ProfilePINReauthentication, proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      await this.hostedOptions.hostedClient.setProfilePIN(profileId, input, { token: proof }, { signal });
      return;
    }
    return this.client.setAccountProfilePIN(profileId, input, proof, { signal });
  }

  async clearAccountProfilePin(profileId: string, input: ProfilePINReauthentication, proof: string, signal: AbortSignal) {
    if (this.activeViewer?.viewerScope?.authority === 'hosted' && this.hostedOptions) {
      await this.hostedOptions.hostedClient.clearProfilePIN(profileId, input, { token: proof }, { signal });
      return;
    }
    return this.client.clearAccountProfilePIN(profileId, input, proof, { signal });
  }

  async createAutomaticProfileTrust(signal: AbortSignal) {
    const installationId = await this.connectionVault().installationId();
    const trust = await this.client.createAutomaticProfileTrust({ installationId }, { signal });
    try {
      await this.connectionVault().saveAutomaticProfileTrust(trust);
      return trust;
    } catch (reason) {
      await this.client.revokeAutomaticProfileTrusts({ installationId }, { signal }).catch(() => undefined);
      throw reason;
    }
  }

  async revokeAutomaticProfileTrust(signal: AbortSignal) {
    const current = await this.viewer(signal);
    const installationId = await this.connectionVault().installationId();
    await this.client.revokeAutomaticProfileTrusts({ installationId }, { signal });
    if (current.viewerScope) await this.connectionVault().clearAutomaticProfileTrust(current.viewerScope, current.viewerScope.profileId);
  }

  async switchLocalProfile(input: { login: string; password: string; profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer> {
	const challenge = await this.beginLocalProfileLogin({
		login: input.login,
		password: input.password,
		rememberOnBrowser: true,
	}, signal);
	const selection = await this.verifyLocalProfileSelection(challenge, input.profileId, input.pin, signal);
	const viewer = await this.publishLocalProfileSession(selection, signal);
	this.activeViewer = viewer;
	await this.rememberAuthoritativeProfile(viewer, signal);
	this.recordViewerScope(viewer, true);
	return viewer;
  }

  async switchHostedProfile(input: { profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer> {
    if (!this.hostedOptions) throw new Error('Portico Account profile switching is available only in Hosted Web.');
    if (signal.aborted) throw signal.reason ?? new DOMException('Request aborted', 'AbortError');
    const viewer = await this.hostedOptions.switchHostedProfile(input.profileId, input.pin);
    this.activeViewer = viewer;
    return viewer;
  }

  viewerNotifications(audience: NotificationAudience, cursor: string | undefined, signal: AbortSignal) {
    return this.client.viewerNotifications({ audience, cursor, limit: 30 }, { signal });
  }

  watchViewerNotificationInvalidations(audience: NotificationAudience, onInvalidation: (event: NotificationInvalidation) => void, signal: AbortSignal) {
    return this.client.subscribeViewerNotificationInvalidations({ audience }, {
      transport: 'sse',
      signal,
      publicationFence: { generation: 0, currentGeneration: () => signal.aborted ? 1 : 0 },
      onEvent: onInvalidation,
      onResetRequired: () => onInvalidation({
        version: 'v1',
        kind: 'notifications.invalidated',
        occurredAt: new Date().toISOString(),
      }),
    });
  }

  watchApplicationEvents(onEvent: Parameters<PorticoDataSource['watchApplicationEvents']>[0], onReset: Parameters<PorticoDataSource['watchApplicationEvents']>[1], signal: AbortSignal) {
    return this.client.subscribeAppEvents({
      transport: 'sse',
      signal,
      publicationFence: { generation: 0, currentGeneration: () => signal.aborted ? 1 : 0 },
      onEvent,
      onResetRequired: onReset,
    });
  }

  updateViewerNotificationReceipts(audience: NotificationAudience, input: { recipient: NotificationRecipient; notificationIds: string[]; action: NotificationReceiptAction; expectedRevision: number }, signal: AbortSignal) {
    return this.client.updateViewerNotificationReceipts({
      version: 'v1',
      recipient: input.recipient,
      notificationIds: input.notificationIds,
      action: input.action,
      expectedRevision: input.expectedRevision,
    }, { audience }, { signal });
  }

  async markAllViewerNotificationsRead(audience: NotificationAudience, signal: AbortSignal) {
    return this.client.markAllViewerNotificationsRead({ audience }, { signal });
  }

  viewerFeedbackCapabilities(signal: AbortSignal) {
    return this.client.viewerFeedbackCapabilities({ signal });
  }

  submitViewerFeedback(input: ViewerFeedbackSubmission, signal: AbortSignal) {
    return this.client.submitViewerFeedback(input, { signal });
  }

  ownerViewerFeedback(status: 'new' | 'read' | 'resolved' | 'dismissed' | undefined, cursor: string | undefined, signal: AbortSignal) {
    return this.client.ownerViewerFeedback({ status, cursor, limit: 30 }, { signal });
  }

  ownerViewerNotificationRecipients(signal: AbortSignal): Promise<ServerOwnerNotificationRecipientDirectory> {
    return this.client.ownerViewerNotificationRecipients({ signal });
  }

  updateOwnerViewerFeedback(feedbackId: string, input: ServerOwnerFeedbackUpdateRequest, signal: AbortSignal) {
    return this.client.updateOwnerViewerFeedback(feedbackId, input, { signal });
  }

  async createOwnerViewerNotice(input: ServerOwnerNoticeRequest, signal: AbortSignal) {
    await this.client.createOwnerViewerNotice(input, { signal });
  }

  async home(signal: AbortSignal): Promise<HomeResult> {
    const response = await this.client.request<ApiHomeResponse>('/api/home', { signal });
    return { pivots: response.pivots, rows: response.rows.map(this.homeRowView) };
  }

  async homeRow(id: string, cursor: string | undefined, signal: AbortSignal, limit = 24): Promise<HomeRow> {
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) params.set('cursor', cursor);
    const response = await this.client.request<ApiHomeRow>(`/api/home/rows/${encodeURIComponent(id)}?${params}`, { signal });
    return this.homeRowView(response);
  }

  async webDisplayPreferences(signal: AbortSignal): Promise<WebDisplayPreference> {
    const response = await this.client.displayPreferences('web', 'app', { signal });
    return { preferences: response.preferences, updatedAt: response.updatedAt };
  }

  async updateWebDisplayPreferences(preferences: Record<string, unknown>, signal: AbortSignal): Promise<WebDisplayPreference> {
    const response = await this.client.request<WebDisplayPreference>('/api/display-preferences/web/app', {
      method: 'PATCH',
      body: { preferences },
      signal,
    });
    return response;
  }

  async libraryNavigation(signal: AbortSignal) {
    return this.client.libraryNavigation({ signal });
  }

  async updateLibraryNavigation(pinnedLibraryIds: string[], signal: AbortSignal) {
    return this.client.updateLibraryNavigation({ pinnedLibraryIds }, { signal }) as Promise<LibraryNavigationPreferences>;
  }

  async libraries(signal: AbortSignal): Promise<LibrarySummary[]> {
    const response = await this.client.request<ApiLibraryList>('/api/libraries', { signal });
    return (response.items as ApiLibrary[]).flatMap((library) => {
      const kind = ({
        movie: 'movies',
        show: 'tv',
        anime: 'anime',
        music: 'music',
        audiobook: 'audiobooks',
        'recorded-tv': 'recorded-tv',
      } as const)[library.type];
      return kind ? [{ id: library.id, name: library.name, kind, itemCount: library.count ?? 0 }] : [];
    });
  }

  async productContract(signal: AbortSignal) {
    if (signal.aborted) throw abortedRequestError();
    if (this.contract) return this.contract;
    if (!this.contractRequest) {
      const pending = {} as ProductContractRequest;
      pending.generation = this.contractGeneration;
      // The cache generation owns this signal. A component may stop waiting,
      // but only explicit invalidation may cancel the shared network request.
      pending.controller = new AbortController();
      pending.promise = this.client.request<ApiProductContract>('/api/product-contract', { signal: pending.controller.signal }).then((contract) => {
        if (pending.generation !== this.contractGeneration) {
          throw abortedRequestError('Product Contract request was invalidated');
        }
        this.contract = contract;
        return contract;
      }).finally(() => {
        if (this.contractRequest === pending) this.contractRequest = undefined;
      });
      this.contractRequest = pending;
    }
    return waitForSharedRequest(this.contractRequest.promise, signal);
  }

  async uploadClientDiagnostics(input: Parameters<NonNullable<PorticoDataSource['uploadClientDiagnostics']>>[0], signal: AbortSignal): Promise<void> {
    abortIfRequested(signal);
    await this.client.uploadClientLogs(input);
  }

  async searchContract(signal: AbortSignal): Promise<SearchContract> {
    return (await this.productContract(signal)).search;
  }

  private async browseCapabilities(libraryId: string, signal: AbortSignal) {
    const current = this.capabilities.get(libraryId);
    if (current) return current;
    const generation = this.contractGeneration;
    const capabilities = await this.client.request<ApiBrowseCapabilities>(`/api/libraries/${encodeURIComponent(libraryId)}/browse-capabilities`, { signal });
    if (generation !== this.contractGeneration) throw abortedRequestError('Browse capabilities request was invalidated');
    this.capabilities.set(libraryId, capabilities);
    return capabilities;
  }

  async browseLibrary(input: LibraryBrowseInput, signal: AbortSignal): Promise<LibraryBrowseResult> {
    await this.productContract(signal);
    const libraries = await this.client.request<ApiLibraryList>('/api/libraries', { signal });
    const libraryItems = libraries.items as ApiLibrary[];
    const library = libraryItems.find((candidate) => kindMatches(candidate.type, input.kind));
    if (!library) throw new Error(`No ${input.kind === 'movies' ? 'movie' : input.kind === 'music' ? 'music' : 'TV'} library is available.`);

    const capabilities = await this.browseCapabilities(library.id, signal);
    const requestedPivot = canonicalPivot(input.pivot);
    const pivot = capabilities.pivots.find((candidate) => candidate.id === requestedPivot || canonicalPivot(candidate.label) === requestedPivot)
      ?? capabilities.pivots[0];
    if (!pivot) throw new Error(`This library does not declare any ${input.kind} views.`);

    const mappedCapabilities: LibraryCapabilities = {
      apiVersion: capabilities.apiVersion,
      pivots: mapPivots(capabilities),
      sorts: mapSorts(capabilities),
      actions: capabilities.actions,
    };

    if (!pivot.browseSupported) {
      const endpoint = pivot.endpointTemplate.replaceAll('{libraryId}', encodeURIComponent(library.id));
      if (pivot.id === 'discover') {
        const response = await this.client.request<ApiSuggestionsResponse>(endpoint, { signal });
        const items = response.rows?.flatMap((row) => row.items.map((item) => this.mediaDetail(item, false)))
          ?? response.items.map((suggestion) => this.mediaDetail(suggestion.item, false));
        return { items, total: items.length, libraryId: library.id, hasMore: false, nextCursor: null, capabilities: mappedCapabilities };
      }
      if (pivot.id === 'categories' || pivot.id === 'genres') {
        const response = await this.client.request<ApiLibraryCategoryList>(endpoint, { signal });
        return { items: (response.items as ApiLibraryCategory[]).map((category) => categoryToMedia(category, input.kind)), total: response.total, libraryId: library.id, hasMore: false, nextCursor: null, capabilities: mappedCapabilities };
      }
      if (pivot.id === 'collections') {
        const response = await this.client.request<ApiCollectionPage>(endpoint, { signal });
        return { items: response.items.map((item) => summaryToMedia(item, 'collection', input.kind)), total: response.pageInfo.total ?? response.items.length, libraryId: library.id, hasMore: response.pageInfo.hasMore, nextCursor: response.pageInfo.nextCursor, capabilities: mappedCapabilities };
      }
      if (pivot.id === 'playlists') {
        const response = await this.client.request<ApiPlaylistPage>(endpoint, { signal });
        return { items: response.items.map((item) => summaryToMedia(item, 'playlist', input.kind)), total: response.pageInfo.total ?? response.items.length, libraryId: library.id, hasMore: response.pageInfo.hasMore, nextCursor: response.pageInfo.nextCursor, capabilities: mappedCapabilities };
      }
      const response = await this.client.request<{ items: ApiMediaItem[]; total?: number; pageInfo?: { total?: number; hasMore?: boolean; nextCursor?: string } }>(endpoint, { signal });
      const items = response.items.map((item) => this.mediaDetail(item, false));
      return {
        items,
        total: response.total ?? response.pageInfo?.total ?? items.length,
        libraryId: library.id,
        hasMore: response.pageInfo?.hasMore ?? false,
        nextCursor: response.pageInfo?.nextCursor ?? null,
        capabilities: mappedCapabilities,
      };
    }

    const sortField = canonicalSort(input.sort);
    const supportedSort = capabilities.sorts.find((candidate) => candidate.id === sortField);
    const body: ApiBrowseRequest = {
      pivot: pivot.id,
      limit: 200,
      query: queryForFilter(input.filter),
      sort: [{ field: supportedSort?.id ?? pivot.defaultSort[0]?.field ?? 'title', direction: input.direction === 'ascending' ? 'asc' : 'desc' }],
    };
    const response = await this.client.request<ApiBrowseResponse>(`/api/libraries/${encodeURIComponent(library.id)}/browse`, { method: 'POST', body, signal });
    const items = response.items.map(this.mediaCard);
    return {
      items,
      total: response.pageInfo.total ?? items.length,
      libraryId: library.id,
      nextCursor: response.pageInfo.nextCursor,
      hasMore: response.pageInfo.hasMore,
      capabilities: mappedCapabilities,
    };
  }

  async libraryBrowseCapabilities(libraryId: string, signal: AbortSignal): Promise<LibraryBrowseCapabilities> {
    return normalizeBrowseCapabilities(await this.browseCapabilities(libraryId, signal));
  }

  async libraryFacetOptions(libraryId: string, facetSource: NonNullable<BrowseFacetSource>, signal: AbortSignal): Promise<BrowseFacetOption[]> {
    const endpoint = new URL(resolveBrowseFacetEndpoint(facetSource, libraryId), 'http://portico.local');
    endpoint.searchParams.set(facetSource.filterField, facetSource.filterPrefix);
    endpoint.searchParams.set('limit', '100');
    const response = await this.client.request<{ items: Array<Record<string, unknown>> }>(`${endpoint.pathname}${endpoint.search}`, { signal });
    return browseFacetOptions(facetSource, response.items);
  }

  async libraryPivot(input: LibraryPivotInput, signal: AbortSignal): Promise<LibraryPivotPage> {
    const capabilities = await this.browseCapabilities(input.libraryId, signal);
    const pivot = capabilities.pivots.find((candidate) => candidate.id === input.pivot.id);
    if (!pivot) throw new Error(`The ${input.pivot.label} view is no longer available for this library.`);

    if (pivot.browseSupported) {
      const response = await this.client.browseLibrary(input.libraryId, input.request, { signal });
      return {
        items: response.items.map(this.mediaCard),
        total: response.pageInfo.total,
        nextCursor: response.pageInfo.nextCursor,
        hasMore: response.pageInfo.hasMore,
        applied: response.applied,
        presentation: libraryWorkspacePresentation(pivot.defaultView),
      };
    }

    const endpoint = pivot.endpointTemplate.replaceAll('{libraryId}', encodeURIComponent(input.libraryId));
    if (pivot.id === 'discover') {
      const response = await this.client.libraryDiscover(input.libraryId, { limit: input.request.limit }, { signal });
      const sections = response.rows?.map((row) => ({
        id: row.id,
        title: row.title,
        items: row.items.map((item) => this.mediaDetail(item, false)),
      })) ?? [];
      const items = response.items.map((suggestion) => this.mediaDetail(suggestion.item, false));
      return nonBrowsePage(input, {
        items,
        sections,
        total: response.total,
        nextCursor: null,
        hasMore: false,
      });
    }

    if (pivot.id === 'categories' || pivot.id === 'genres') {
      const response = await this.client.request<ApiLibraryCategoryList>(endpoint, { signal });
      const availableFields = new Set(capabilities.fields.map((field) => field.id));
      const categories = response.items as ApiLibraryCategory[];
      const facets: LibraryFacet[] = categories.flatMap((category) => {
        const query = libraryFacetPredicate(category.filter, category.group);
        if (!('field' in query) || !availableFields.has(query.field)) return [];
        return [{
          id: category.id,
          title: category.name,
          detail: category.description,
          count: category.count,
          artwork: category.image,
          query,
        }];
      });
      return nonBrowsePage(input, {
        items: [],
        facets,
        total: response.total,
        nextCursor: response.nextCursor,
        hasMore: response.hasMore ?? false,
      });
    }

    if (pivot.id === 'authors' || pivot.id === 'series') {
      const search = new URLSearchParams();
      if (input.request.limit) search.set('limit', String(input.request.limit));
      if (input.request.cursor) search.set('cursor', input.request.cursor);
      const response = await this.client.request<ApiLibraryFacetValueList>(`${endpoint}${search.size ? `?${search}` : ''}`, { signal });
      const facetValues = response.items as ApiLibraryFacetValue[];
      return nonBrowsePage(input, {
        items: facetValues.map((facet) => ({
          id: facet.id,
          libraryId: input.libraryId,
          entityKind: facet.entityKind,
          title: facet.name,
          subtitle: '',
          year: 0,
          type: 'music' as const,
          kind: facet.entityKind === 'author' ? 'author' as const : 'series' as const,
          poster: imagePath(facet.image),
          backdrop: imagePath(facet.image),
          rating: '',
          length: '',
          genre: '',
          actions: [],
        })),
        total: response.total,
        nextCursor: response.nextCursor,
        hasMore: response.hasMore ?? false,
      });
    }

    if (pivot.id === 'collections') {
      const response = await this.client.collections({ cursor: input.request.cursor, limit: input.request.limit, libraryId: input.libraryId }, { signal });
      return nonBrowsePage(input, {
        items: response.items.map((item) => summaryToMedia(item, 'collection', input.libraryKind)),
        total: response.pageInfo.total,
        nextCursor: response.pageInfo.nextCursor,
        hasMore: response.pageInfo.hasMore,
      });
    }

    if (pivot.id === 'playlists') {
      const response = await this.client.playlists({ cursor: input.request.cursor, limit: input.request.limit, libraryId: input.libraryId }, { signal });
      return nonBrowsePage(input, {
        items: response.items.map((item) => summaryToMedia(item, 'playlist', input.libraryKind)),
        total: response.pageInfo.total,
        nextCursor: response.pageInfo.nextCursor,
        hasMore: response.pageInfo.hasMore,
      });
    }

    if (pivot.id === 'schedule') {
      const search = new URLSearchParams();
      if (input.request.limit) search.set('limit', String(input.request.limit));
      if (input.request.cursor) search.set('cursor', input.request.cursor);
      const response = await this.client.request<ApiDVRSchedule>(`${endpoint}${search.size ? `?${search}` : ''}`, { signal });
      return nonBrowsePage(input, {
        items: response.items.map(dvrRecordingToMedia),
        total: response.pageInfo.total,
        nextCursor: response.pageInfo.nextCursor,
        hasMore: response.pageInfo.hasMore,
      });
    }

    const response = await this.client.request<{ items: ApiMediaItem[]; total?: number; nextCursor?: string; hasMore?: boolean }>(endpoint, { signal });
    return nonBrowsePage(input, {
      items: response.items.map((item) => this.mediaDetail(item, false)),
      total: response.total,
      nextCursor: response.nextCursor,
      hasMore: response.hasMore ?? false,
    });
  }

  createSavedView(input: SavedViewCreateRequest, signal: AbortSignal): Promise<SavedView> {
    return this.client.createSavedView(input, { signal });
  }

  async search(query: string, signal: AbortSignal, limit = 6): Promise<SearchResult[]> {
    const response = await this.searchPage({ query, limit }, signal);
    return response.groups.flatMap((group) => group.items).slice(0, limit);
  }

  async searchPage(input: SearchPageInput, signal: AbortSignal): Promise<SearchPageResult> {
    const contract = await this.productContract(signal);
    const request = resolveSearchRequest(contract, input.query, {
      group: input.group as SearchRequestOptions['group'],
      entityKinds: input.entityKinds as SearchRequestOptions['entityKinds'],
      libraryIds: input.libraryIds,
      sort: input.sort,
      direction: input.direction,
      cursor: input.cursor,
      limit: input.limit,
      recordHistory: input.recordHistory,
    });
    const response = await this.client.request<ApiSearchResponse>('/api/search', {
      method: 'POST',
      body: request,
      signal,
    });
    return {
      query: response.query,
      sort: response.sort,
      direction: response.direction,
      groups: response.groups.map((group) => ({
        id: group.id,
        title: group.title,
        entityKind: group.entityKind,
        status: group.status,
        errorCode: group.errorCode,
        messageId: group.messageId,
        items: group.items.map((item) => this.mediaDetail(item, false)),
        hasMore: group.hasMore,
        nextCursor: group.nextCursor,
      })),
    };
  }

  async searchHistory(signal: AbortSignal): Promise<SearchHistoryItem[]> {
    const response = await this.client.searchHistory({ signal });
    return response.items;
  }

  async clearSearchHistory(signal: AbortSignal): Promise<void> {
    await this.client.clearSearchHistory(undefined, { signal });
  }

  async person(id: string, signal: AbortSignal, cursor?: string): Promise<PersonDetail> {
    const response = await this.client.person(id, { cursor }, { signal });
    const person = response.person ?? { id, name: productMessage('person.fallback-name').text ?? '', roles: [] };
    return {
      ...person,
      imageUrl: imagePath(person.imageUrl, '', this.resolveResource, { width: 256, height: 256 }) || undefined,
      knownFor: person.roles?.join(' · '),
      credits: response.credits.map(this.mediaCard),
      hasMore: response.pageInfo.hasMore,
      nextCursor: response.pageInfo.nextCursor,
    };
  }

  async media(id: string, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.media(id, { includeRecommendations: true }, { signal });
    return this.mediaDetail({
      ...response,
      attachments: response.attachments?.map((attachment) => ({
        ...attachment,
        url: this.client.mediaAttachmentUrl(response.id, attachment.id),
      })),
    });
  }

  async mediaChildren(id: string, signal: AbortSignal, cursor?: string, limit = 50): Promise<MediaChildrenPage> {
    const response = await this.client.mediaChildren(id, { limit, cursor }, { signal });
    return { items: response.items.map(this.mediaCard), hasMore: response.pageInfo.hasMore, nextCursor: response.pageInfo.nextCursor };
  }

  async deleteMedia(id: string, input: { deleteFiles: boolean; confirmation?: string }, signal: AbortSignal): Promise<MediaDeleteResult> {
    return this.client.request<MediaDeleteResult>(`/api/media/${encodeURIComponent(id)}`, {
      method: 'DELETE',
      body: input,
      signal,
    });
  }

  async watchlist(signal: AbortSignal): Promise<MediaItem[]> {
    const response = await this.client.request<{ items: ApiMediaItem[] }>('/api/watchlist?limit=250&sort=updated&order=desc', { signal });
    return response.items.map((item) => this.mediaDetail(item, false));
  }

  async favorites(signal: AbortSignal): Promise<MediaItem[]> {
    const response = await this.client.request<{ items: ApiMediaItem[] }>('/api/favorites?limit=250&sort=updated&order=desc', { signal });
    return response.items.map((item) => this.mediaDetail(item, false));
  }

  async setWatchlist(id: string, watchlisted: boolean, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/watchlist`, { method: 'POST', body: { watchlisted }, signal });
    return this.mediaDetail(response);
  }

  async setFavorite(id: string, favorite: boolean, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/favorite`, { method: 'POST', body: { favorite }, signal });
    return this.mediaDetail(response);
  }

  async setReaction(id: string, reaction: '' | 'like' | 'dislike', signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/reaction`, { method: 'POST', body: { reaction }, signal });
    return this.mediaDetail(response);
  }

  async setRating(id: string, rating: number, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/rating`, { method: 'POST', body: { rating }, signal });
    return this.mediaDetail(response);
  }

  async setWatched(id: string, watched: boolean, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/watched`, { method: 'POST', body: { watched }, signal });
    return this.mediaDetail(response);
  }

  async updateMediaMetadata(ids: string[], patch: MediaMetadataUpdate, signal: AbortSignal): Promise<MediaItem[]> {
    if (!ids.length) return [];
    if (ids.length === 1) {
      const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(ids[0])}`, { method: 'PATCH', body: patch, signal });
      return [this.mediaDetail(response)];
    }
    const response = await this.client.request<{ items: ApiMediaItem[] }>('/api/media/bulk/metadata', { method: 'POST', body: { mediaIds: ids, patch }, signal });
    return response.items.map((item) => this.mediaDetail(item, false));
  }

  async uploadMediaImage(id: string, type: string, file: File, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const form = new FormData();
    form.set('file', file);
    form.set('type', type);
    await this.client.uploadMediaImage(id, form);
  }

  async deleteMediaImage(id: string, imageId: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.deleteMediaImage(id, imageId);
  }

  async setPreferredMediaImage(id: string, imageId: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.setPreferredMediaImage(id, imageId);
  }

  async reorderMediaImages(id: string, imageIds: string[], signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.reorderMediaImages(id, imageIds);
  }

  async uploadSubtitle(id: string, file: File, language: string, label: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const form = new FormData();
    form.set('file', file);
    form.set('language', language);
    if (label) form.set('label', label);
    await this.client.uploadSubtitle(id, form);
  }

  async updateSubtitle(id: string, streamId: string, offsetMs: number, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.updateSubtitle(id, streamId, { offsetMs });
  }

  async deleteSubtitle(id: string, streamId: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.deleteSubtitle(id, streamId);
  }

  async uploadLyrics(id: string, file: File, language: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const form = new FormData();
    form.set('file', file);
    form.set('language', language);
    await this.client.uploadLyrics(id, form);
  }

  async fetchLyrics(id: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.fetchLyrics(id);
  }

  async searchLyrics(id: string, query: string, signal: AbortSignal): Promise<LyricSearchCandidate[]> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    const response = await this.client.searchLyrics(id, query);
    return response.items;
  }

  async applyLyrics(
    id: string,
    candidate: Pick<LyricSearchCandidate, 'provider' | 'externalId'>,
    signal: AbortSignal,
  ): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.applyLyrics(id, { provider: candidate.provider, externalId: candidate.externalId });
  }

  async deleteLyrics(id: string, lyricId: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.deleteLyrics(id, lyricId);
  }

  async searchMediaMatches(id: string, query: string, signal: AbortSignal): Promise<MediaMatchCandidate[]> {
    const search = new URLSearchParams();
    if (query.trim()) search.set('query', query.trim());
    const response = await this.client.request<ApiMediaMatchSearchResponse>(`/api/media/${encodeURIComponent(id)}/match-candidates?${search}`, { signal });
    return response.items.map((item) => ({
      provider: item.provider, externalId: item.externalId, externalType: item.externalType, source: item.source,
      score: item.score, accepted: item.accepted, title: item.title, year: item.year, overview: item.overview, posterUrl: item.posterUrl,
    }));
  }

  async applyMediaMatch(id: string, candidate: Pick<MediaMatchCandidate, 'provider' | 'externalId' | 'externalType'>, signal: AbortSignal): Promise<MediaItem> {
    const response = await this.client.request<ApiMediaItem>(`/api/media/${encodeURIComponent(id)}/match`, { method: 'POST', body: candidate, signal });
    return this.mediaDetail(response);
  }

  queueMediaJob(id: string, type: MediaJobType, options: MediaJobOptions, signal: AbortSignal): Promise<MediaJob> {
    return this.client.request(`/api/media/${encodeURIComponent(id)}/jobs`, {
      method: 'POST',
      body: { type, ...options },
      signal,
    });
  }

  mediaDownloadOptions(id: string, signal: AbortSignal): Promise<MediaDownloadOptions> {
    return this.client.request(`/api/media/${encodeURIComponent(id)}/download-options`, { signal });
  }

  async downloadPreparations(signal: AbortSignal): Promise<DownloadPreparation[]> {
    const response = await this.client.downloadPreparations({ signal });
    return response.items ?? [];
  }

  updateDownloadPreparation(id: string, action: 'pause' | 'resume' | 'cancel' | 'retry' | 'remove', signal: AbortSignal): Promise<DownloadPreparation> {
    return this.client.updateDownloadPreparation(id, { action }, { signal });
  }

  async downloadPreparationURL(id: string, signal: AbortSignal): Promise<string> {
    const grant = await this.client.createDownloadPreparationGrant(id, { signal });
    return this.client.resourceUrl(grant.downloadUrl);
  }

  createOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<MediaJob> {
    return this.client.request(`/api/media/${encodeURIComponent(id)}/optimized`, {
      method: 'POST',
      body: { profile },
      signal,
    });
  }

  async deleteOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<void> {
    if (signal.aborted) throw new DOMException('Request aborted', 'AbortError');
    await this.client.deleteOptimizedVersion(id, profile);
  }

  async createMediaDownloadURL(id: string, profile: string, signal: AbortSignal): Promise<string> {
    const grant = await this.client.createMediaDownloadGrant(id, { profile: profile.trim() || 'source' }, { signal });
    return this.client.resourceUrl(grant.downloadUrl);
  }

  async savedShareCandidates(query: string, limit: number, signal: AbortSignal): Promise<SavedShareCandidatePage> {
    const response = await this.client.savedShareCandidates({
      query: query.trim() || undefined,
      limit: Math.max(1, Math.min(limit, 50)),
    }, { signal }) as ApiSavedShareCandidatePage;
    return {
      items: response.items.map((candidate) => ({ userId: candidate.userId, displayName: candidate.displayName })),
      hasMore: response.hasMore,
    };
  }

  async savedResources(kind: SavedResourceKind, signal: AbortSignal): Promise<SavedResourceSummary[]> {
    if (kind === 'playlist') {
      const response = await this.client.request<ApiPlaylistPage>('/api/playlists?limit=200', { signal });
      return response.items.map((item) => listResourceSummary(item, kind));
    }
    if (kind === 'collection') {
      const response = await this.client.request<ApiCollectionPage>('/api/collections?limit=200', { signal });
      return response.items.map((item) => listResourceSummary(item, kind));
    }
    const response = await this.client.request<ApiSavedViewPage>('/api/saved-views?limit=200', { signal });
    return response.items.map(savedViewSummary);
  }

  async savedResource<K extends SavedResourceKind>(kind: K, id: string, input: SavedResourceItemsInput, signal: AbortSignal): Promise<SavedResourceDetail<K>> {
    const encodedId = encodeURIComponent(id);
    const pageInput = { cursor: input.cursor, limit: input.limit ?? 50 };
    if (kind === 'view') {
      const [resource, browse] = await Promise.all([
        this.client.request<ApiSavedView>(`/api/saved-views/${encodedId}`, { signal }),
        this.client.browseSavedView(id, pageInput, { signal }),
      ]);
      const items = browse.items.map(this.mediaCard);
      return {
        kind: 'view',
        resource: { ...savedViewSummary(resource), kind: 'view', itemCount: browse.pageInfo.total ?? items.length },
        items,
        hasMore: browse.pageInfo.hasMore,
        nextCursor: browse.pageInfo.nextCursor,
      } as SavedResourceDetail<K>;
    }

    if (kind === 'playlist') {
      const [resource, page] = await Promise.all([
        this.client.playlist(id, { signal }),
        this.client.playlistItems(id, pageInput, { signal }) as Promise<ApiPlaylistEntryPage>,
      ]);
      return {
        kind: 'playlist',
        resource: { ...listResourceSummary(resource, 'playlist'), kind: 'playlist' },
        entries: page.items.map((entry) => ({ entryId: entry.entryId, media: this.mediaCard(entry.media), position: entry.position })),
        hasMore: page.pageInfo.hasMore,
        nextCursor: page.pageInfo.nextCursor,
      } as SavedResourceDetail<K>;
    }

    const [resource, page] = await Promise.all([
      this.client.collection(id, { signal }),
      this.client.collectionItems(id, pageInput, { signal }),
    ]);
    return {
      kind: 'collection',
      resource: { ...listResourceSummary(resource, 'collection'), kind: 'collection' },
      items: page.items.map(this.mediaCard),
      hasMore: page.pageInfo.hasMore,
      nextCursor: page.pageInfo.nextCursor,
    } as SavedResourceDetail<K>;
  }

  async createSavedResource(kind: SavedResourceKind, input: SavedResourceCreateInput, signal: AbortSignal): Promise<SavedResourceSummary> {
    if (kind === 'view') {
      if (!input.libraryId || !input.pivot) throw new Error('Choose a library and view before saving this search.');
      const response = await this.client.request<ApiSavedView>('/api/saved-views', {
        method: 'POST', signal,
        body: { title: input.title, libraryId: input.libraryId, pivot: input.pivot, isPinned: input.isPinned ?? false },
      });
      return savedViewSummary(response);
    }
    const endpoint = kind === 'playlist' ? 'playlists' : 'collections';
    const response = await this.client.request<ApiSavedPlaylist | ApiCollection>(`/api/${endpoint}`, {
      method: 'POST', signal,
      body: { title: input.title, summary: input.summary, visibility: input.visibility ?? 'private', shares: input.shares, mediaIds: input.mediaIds },
    });
    return listResourceSummary(response, kind);
  }

  async updateSavedResource(kind: SavedResourceKind, id: string, input: SavedResourceUpdateInput, signal: AbortSignal): Promise<SavedResourceSummary> {
    const encodedId = encodeURIComponent(id);
    if (kind === 'view') {
      if (!input.libraryId || !input.pivot) throw new Error('A saved view must keep a library and pivot.');
      const response = await this.client.request<ApiSavedView>(`/api/saved-views/${encodedId}`, {
        method: 'PATCH', signal,
        body: { title: input.title, libraryId: input.libraryId, pivot: input.pivot, isPinned: input.isPinned ?? false },
      });
      return savedViewSummary(response);
    }
    const endpoint = kind === 'playlist' ? 'playlists' : 'collections';
    const response = await this.client.request<ApiSavedPlaylist | ApiCollection>(`/api/${endpoint}/${encodedId}`, {
      method: 'PATCH', signal,
      body: { title: input.title, summary: input.summary, visibility: input.visibility, shares: input.shares },
    });
    return listResourceSummary(response, kind);
  }

  async deleteSavedResource(kind: SavedResourceKind, id: string, signal: AbortSignal): Promise<void> {
    const endpoint = kind === 'playlist' ? 'playlists' : kind === 'collection' ? 'collections' : 'saved-views';
    await this.client.request(`/api/${endpoint}/${encodeURIComponent(id)}`, { method: 'DELETE', signal });
  }

  async mutateSavedResourceItems<K extends SavedEditableResourceKind>(kind: K, id: string, mutation: SavedResourceItemsMutation<K>, signal: AbortSignal): Promise<SavedResourceSummary> {
    if (kind === 'playlist') {
      const response = await this.client.mutatePlaylistItems(id, mutation as SavedResourceItemsMutation<'playlist'>, { signal }) as ApiPlaylistItemsBatchResponse;
      return listResourceSummary(response.playlist, kind);
    }
    const response = await this.client.mutateCollectionMemberships(id, mutation as SavedResourceItemsMutation<'collection'>, { signal }) as ApiCollectionMembershipBatchResponse;
    return listResourceSummary(response.collection, kind);
  }

  async startPlayback(mediaId: string, options: PlaybackStartOptions, signal: AbortSignal): Promise<PlaybackResponse> {
    const response = await this.client.request<PlaybackResponse>('/api/playback-sessions', {
      method: 'POST',
      signal,
      body: {
        mediaId,
        clientInstanceId: this.playbackClientInstanceId,
        clientProfile: browserPlaybackClientProfile(),
        ...options,
      },
    });
    return this.playbackAdapter.acceptPlayback(response);
  }

  async mediaTrickplay(id: string, signal: AbortSignal): Promise<MediaTrickplaySet[]> {
    const response = await this.client.mediaTrickplay(id, { signal });
    return response.items;
  }

  async restorePlayback(signal: AbortSignal, intent?: import('@portico/client-core').PlaybackIntent): Promise<PlaybackRestoreResponse> {
    const response = await this.client.request<PlaybackRestoreResponse>('/api/playback/active', {
      method: 'POST',
      signal,
      body: { clientInstanceId: this.playbackClientInstanceId, clientProfile: browserPlaybackClientProfile(), intent },
    });
    if (!response.playback) return response;
    return this.playbackAdapter.acceptRestorePlayback(response);
  }

  touchPlayback(sessionId: string, event: PlaybackProgressInput, signal?: AbortSignal, keepalive = false): Promise<PlaybackProgressAcknowledgement> {
    return this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}`, {
      method: 'PATCH', signal, keepalive, body: this.playbackAdapter.orderProgress(sessionId, event),
    });
  }

  renewPlaybackMediaGrant(sessionId: string, signal: AbortSignal): Promise<MediaGrant> {
    return this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}/media-grant`, { method: 'POST', signal });
  }

  async renegotiatePlayback(sessionId: string, request: import('@portico/client-core').PlaybackRenegotiationRequest, signal: AbortSignal): Promise<PlaybackResponse> {
    return this.playbackAdapter.normalizePlayback(await this.client.renegotiatePlayback(sessionId, request, { signal }));
  }

  async stopPlayback(sessionId: string, signal?: AbortSignal, keepalive = false): Promise<void> {
    this.playbackAdapter.releaseSession(sessionId);
    await this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}`, { method: 'DELETE', signal, keepalive });
  }

  playbackSessionQueue(sessionId: string, signal: AbortSignal): Promise<PlaybackSessionQueueResponse> {
    return this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`, { signal });
  }

  updatePlaybackSessionQueue(sessionId: string, request: PlaybackSessionQueueReplaceRequest, signal: AbortSignal): Promise<PlaybackSessionQueueResponse> {
    return this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`, { method: 'PUT', signal, body: request });
  }

  mutatePlaybackSessionQueue(sessionId: string, request: PlaybackSessionQueueRequest, signal: AbortSignal): Promise<PlaybackSessionQueueResponse> {
    return this.client.request(`/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`, { method: 'PATCH', signal, body: request });
  }

  async prepareNextPlayback(sessionId: string, signal: AbortSignal, request: PlaybackPrepareNextRequest = {}): Promise<PlaybackPreparedResponse> {
    const response = await this.client.request<PlaybackPreparedResponse>(`/api/playback-sessions/${encodeURIComponent(sessionId)}/prepare-next`, {
      method: 'POST',
      signal,
      body: {
        ...request,
        clientProfile: request.clientProfile ?? browserPlaybackClientProfile(),
        commitPreviousEnd: request.commitPreviousEnd ?? true,
      },
    });
    return this.playbackAdapter.acceptPreparedPlayback(response);
  }

  async handoffPlayback(sessionId: string, request: PlaybackHandoffRequest, signal: AbortSignal): Promise<PlaybackResponse> {
    const response = await this.client.request<PlaybackResponse>(`/api/playback-sessions/${encodeURIComponent(sessionId)}/handoff`, {
      method: 'POST', signal, body: { ...request, clientProfile: request.clientProfile ?? browserPlaybackClientProfile() },
    });
    return this.playbackAdapter.acceptPlayback(response);
  }

  playbackResourceUrl(path: string): string {
    return this.client.resourceUrl(path);
  }

  async liveTVSources(signal: AbortSignal): Promise<ActionableLiveTVSource[]> {
    const response = await this.client.request<{ items: ActionableLiveTVSource[] }>('/api/live-tv', { signal });
    return response.items;
  }

  liveTVGuide(sourceId: string, input: LiveTVGuideInput, signal: AbortSignal): Promise<LiveTVGuideResult> {
    const search = new URLSearchParams({ from: input.from, hours: String(input.hours), limit: '200' });
    if (input.query?.trim()) search.set('query', input.query.trim());
    if (input.favoritesOnly) search.set('filter', 'favorites');
    return this.client.request(`/api/live-tv/sources/${encodeURIComponent(sourceId)}/guide?${search}`, { signal });
  }

  async liveTVChannels(sourceId: string, signal: AbortSignal): Promise<ActionableLiveTVChannel[]> {
	const response = await this.liveTVChannelsPage(sourceId, { limit: 250 }, signal);
	return response.items;
  }

  liveTVGuidePage(sourceId: string, input: LiveTVGuidePageInput, signal: AbortSignal): Promise<LiveTVGuideWorkspacePage> {
	const search = new URLSearchParams({ from: input.from, hours: String(input.hours), limit: String(input.limit), count: 'exact' });
	if (input.cursor) search.set('cursor', input.cursor);
	if (input.query?.trim()) search.set('query', input.query.trim());
	if (input.favoritesOnly) search.set('filter', 'favorites');
	if (input.group) search.set('group', input.group);
	if (input.order) search.set('order', input.order);
	return this.client.request(`/api/live-tv/sources/${encodeURIComponent(sourceId)}/guide?${search}`, { signal });
  }

  liveTVChannelsPage(sourceId: string, input: LiveTVChannelPageInput, signal: AbortSignal): Promise<LiveTVChannelPage> {
	const search = new URLSearchParams({ limit: String(input.limit), count: 'exact' });
	if (input.cursor) search.set('cursor', input.cursor);
	if (input.query?.trim()) search.set('query', input.query.trim());
	if (input.favoritesOnly) search.set('favoritesOnly', 'true');
	if (input.group) search.set('group', input.group);
	return this.client.request(`/api/live-tv/sources/${encodeURIComponent(sourceId)}/channels?${search}`, { signal });
  }

  updateLiveTVChannel(channelId: string, state: { favorite?: boolean }, signal: AbortSignal): Promise<ActionableLiveTVChannel> {
    return this.client.request(`/api/live-tv/channels/${encodeURIComponent(channelId)}`, { method: 'PATCH', signal, body: state });
  }

  async startLiveTVPlayback(channelId: string, signal: AbortSignal): Promise<PlaybackResponse> {
    const response = await this.client.request<PlaybackResponse>('/api/live-tv/play', {
      method: 'POST', signal,
      body: { channelId, clientInstanceId: this.playbackClientInstanceId, clientProfile: browserPlaybackClientProfile() },
    });
    return this.playbackAdapter.acceptPlayback(response);
  }

  async startDVRPlayback(recordingId: string, signal: AbortSignal): Promise<PlaybackResponse> {
    const response = await this.client.request<PlaybackResponse>(`/api/dvr/recordings/${encodeURIComponent(recordingId)}/playback`, {
      method: 'POST', signal,
      body: { clientInstanceId: this.playbackClientInstanceId, clientProfile: browserPlaybackClientProfile() },
    });
    return this.playbackAdapter.acceptPlayback(response);
  }

  libraryChannels(signal: AbortSignal): Promise<LibraryChannelListResponse> {
    return this.client.request('/api/library-channels?limit=250', { signal });
  }

  libraryChannelGuide(channelId: string, input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelGuide> {
    const search = new URLSearchParams({ limit: String(input.limit ?? 250) });
    if (input.from) search.set('from', input.from);
    if (input.to) search.set('to', input.to);
    if (input.cursor) search.set('cursor', input.cursor);
    return this.client.request(`/api/library-channels/${encodeURIComponent(channelId)}/guide?${search}`, { signal });
  }

  libraryChannelsGuide(input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelsGuide> {
    const search = new URLSearchParams({ limit: String(input.limit ?? 250) });
    if (input.from) search.set('from', input.from);
    if (input.to) search.set('to', input.to);
    if (input.cursor) search.set('cursor', input.cursor);
    return this.client.request(`/api/library-channels/guide?${search}`, { signal });
  }

  async startLibraryChannelPlayback(channelId: string, signal: AbortSignal): Promise<PlaybackResponse> {
    const response = await this.client.request<{ playback: PlaybackResponse }>(`/api/library-channels/${encodeURIComponent(channelId)}/tune`, {
      method: 'POST', signal,
      body: { clientInstanceId: this.playbackClientInstanceId, clientProfile: browserPlaybackClientProfile() },
    });
    return this.playbackAdapter.acceptPlayback(response.playback);
  }

  adminLibraryChannels(signal: AbortSignal): Promise<AdminLibraryChannelListResponse> {
    return this.client.request('/api/admin/library-channels?limit=250', { signal });
  }

  libraryChannel(channelId: string, signal: AbortSignal): Promise<LibraryChannelAggregate> {
    return this.client.request(`/api/admin/library-channels/${encodeURIComponent(channelId)}`, { signal });
  }

  createLibraryChannel(input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate> {
    return this.client.request('/api/admin/library-channels', { method: 'POST', signal, body: input });
  }

  updateLibraryChannel(channelId: string, input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate> {
    return this.client.request(`/api/admin/library-channels/${encodeURIComponent(channelId)}`, { method: 'PUT', signal, body: input });
  }

  async deleteLibraryChannel(channelId: string, expectedRevision: number, signal: AbortSignal): Promise<void> {
    await this.client.request(`/api/admin/library-channels/${encodeURIComponent(channelId)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}`, { method: 'DELETE', signal });
  }

  libraryChannelTemplates(signal: AbortSignal): Promise<LibraryChannelTemplatesResponse> {
    return this.client.request('/api/admin/library-channels/templates', { signal });
  }

  restoreLibraryChannelDefaults(input: LibraryChannelRestoreDefaultsRequest, signal: AbortSignal): Promise<LibraryChannelRestoreDefaultsResponse> {
    return this.client.request('/api/admin/library-channels/restore-defaults', { method: 'POST', signal, body: input });
  }

  regenerateLibraryChannel(channelId: string, signal: AbortSignal): Promise<LibraryChannelGeneration> {
    return this.client.request(`/api/admin/library-channels/${encodeURIComponent(channelId)}/regenerate`, { method: 'POST', signal });
  }

  async dvr(signal: AbortSignal): Promise<DVRResult> {
    const loadAll = async <T,>(endpoint: string): Promise<T[]> => {
      const items: T[] = [];
      let cursor = '';
      for (let pageNumber = 0; pageNumber < 20; pageNumber += 1) {
        const search = new URLSearchParams({ limit: '250' });
        if (cursor) search.set('cursor', cursor);
        const page = await this.client.request<{ items: T[]; pageInfo: { hasMore: boolean; nextCursor?: string | null } }>(`${endpoint}?${search}`, { signal });
        items.push(...page.items);
        if (!page.pageInfo.hasMore || !page.pageInfo.nextCursor) return items;
        cursor = page.pageInfo.nextCursor;
      }
      throw new Error('The DVR result exceeded the supported pagination window. Narrow the server retention policy before trying again.');
    };
    const [recordings, rules] = await Promise.all([
      loadAll<ActionableDVRRecording>('/api/dvr/recordings'),
      loadAll<ActionableDVRRule>('/api/dvr/rules'),
    ]);
    return { recordings, rules };
  }

  dvrStatus(sourceId: string | undefined, signal: AbortSignal): Promise<DVRConsumerStatus> {
    return this.client.dvrStatus(sourceId, { signal });
  }

  createDVRRecording(input: { sourceId: string; channelId?: string; programId?: string; title: string; startsAt: string; endsAt: string }, signal: AbortSignal): Promise<ActionableDVRRecording> {
    return this.client.request('/api/dvr/recordings', { method: 'POST', signal, body: input });
  }

  createDVRRule(input: { sourceId: string; channelId?: string; programId?: string; title: string; matchType: string; priority?: number }, signal: AbortSignal): Promise<ActionableDVRRule> {
    return this.client.request('/api/dvr/rules', { method: 'POST', signal, body: input });
  }

  updateDVRRule(id: string, input: Partial<ActionableDVRRule> & { sourceId: string; title: string }, signal: AbortSignal): Promise<ActionableDVRRule> {
	const body = {
	  sourceId: input.sourceId, channelId: input.channelId, programId: input.programId, title: input.title,
	  matchType: input.matchType, folder: input.folder, startPaddingMinutes: input.startPaddingMinutes,
	  endPaddingMinutes: input.endPaddingMinutes, retentionDays: input.retentionDays,
	  maxRecordingsPerSeries: input.maxRecordingsPerSeries, requiredKeywords: input.requiredKeywords,
	  blockedKeywords: input.blockedKeywords, allowedChannels: input.allowedChannels,
	  blockedChannels: input.blockedChannels, enabled: input.enabled, priority: input.priority,
	  expectedRevision: input.revision,
	};
	return this.client.request(`/api/dvr/rules/${encodeURIComponent(id)}`, { method: 'PATCH', signal, body });
  }

  async deleteDVRRecording(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request(`/api/dvr/recordings/${encodeURIComponent(id)}`, { method: 'DELETE', signal });
  }

  async deleteDVRRule(id: string, signal: AbortSignal): Promise<void> {
    await this.client.request(`/api/dvr/rules/${encodeURIComponent(id)}`, { method: 'DELETE', signal });
  }
}
