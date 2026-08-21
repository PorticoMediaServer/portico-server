import type {
  AppEvent,
  AutomaticProfileTrust,
  MediaGrant,
  LyricSearchCandidate,
  MediaAttachment,
  MediaImage,
  MediaLyric,
  DVRRecording,
  DVRRecordingRule,
  LiveTVChannel,
  LiveTVGuideResponse,
  LiveTVProgram,
  LiveTVSourceSummary,
  LibraryChannelAggregate,
  AdminLibraryChannelListResponse,
  LibraryChannelConfigurationRequest,
  LibraryChannelGeneration,
  LibraryChannelGuide,
  LibraryChannelsGuide,
  LibraryChannelListResponse,
  LibraryChannelRestoreDefaultsRequest,
  LibraryChannelRestoreDefaultsResponse,
  LibraryChannelTemplatesResponse,
  PlaybackHandoffRequest,
  PlaybackIntent,
  PlaybackPrepareNextRequest,
  MediaTrickplaySet,
  PlaybackPreparedResponse,
  PlaybackProgressAcknowledgement,
  PlaybackProgressInput,
  PlaybackRenegotiationRequest,
  PlaybackRepeatMode,
  PlaybackResponse,
  PlaybackRestoreResponse,
  PlaybackSessionQueueRequest,
  PlaybackSessionQueueReplaceRequest,
  PlaybackSessionQueueResponse,
  PlaybackSourceContext,
  ProfileDirectory,
  ProfileSelectionGrant,
  RemoteAccessStatus,
  SearchContract,
  ProductContract,
  Stream,
  ServerManagedProfileCreateRequest,
  ServerManagedProfileDirectory,
  ServerManagedProfileUpdateRequest,
  ServerProfileErasureResponse,
  ServerOwnerFeedbackPage,
  ServerOwnerFeedbackRecord,
  ServerOwnerFeedbackUpdateRequest,
  ServerOwnerNotificationRecipientDirectory,
  ServerOwnerNoticeRequest,
  ServerPatchedPreferenceDocument,
  ServerProfileAdministrationProofResponse,
  ServerViewerPreferenceBundle,
  ViewerFeedbackCapabilities,
  ViewerFeedbackReceipt,
  ViewerFeedbackSubmission,
  NotificationAudience,
  NotificationInvalidation,
  NotificationPage,
  NotificationReadAllResult,
  NotificationRecipient,
  NotificationReceiptAction,
  NotificationReceiptResult,
  ViewerScope,
  DownloadPreparation,
  DownloadPreparationUpdateRequest,
} from '@porticomediaserver/client-core';

export type LibraryKind = 'movies' | 'tv' | 'anime' | 'music' | 'audiobooks' | 'recorded-tv';

export type ProfileAdministrationProofInput = {
  pin?: string;
  password?: string;
  replacementPin?: string;
  mfaCode?: string;
  recoveryCode?: string;
  emailRecoveryToken?: string;
};

export type ProfilePINReauthentication = {
  password: string;
  mfaCode?: string;
  recoveryCode?: string;
};

export type LibrarySummary = {
  id: string;
  name: string;
  kind: LibraryKind;
  itemCount: number;
};

export type LibraryNavigationPreferences = {
  pinnedLibraryIds: string[];
};

export type MediaKind = 'show' | 'movie' | 'season' | 'episode' | 'special' | 'artist' | 'album' | 'track' | 'collection' | 'playlist' | 'category' | 'author' | 'audiobook-series' | 'book' | 'chapter' | 'recording' | 'live-channel' | 'live-program' | 'person' | 'extra' | 'unsupported' | (string & Record<never, never>);

export type MediaProviderIdentity = {
  provider: string;
  externalId: string;
  externalType: string;
  confidence?: number;
  source?: string;
  updatedAt?: string;
};

export type MediaMetadataValueEvidence = {
  field: string;
  order: number;
  locale?: string;
  value: unknown;
  sourceKind: string;
  provider?: string;
  confidence: number;
  decision: string;
};

export type MediaMetadataRelationshipEvidence = {
  type: string;
  name: string;
  targetKind?: string;
  provider?: string;
  externalProvider?: string;
  externalId?: string;
  locale?: string;
  country?: string;
  role?: string;
  order: number;
  sourceKind: string;
  confidence: number;
  decision: string;
};

export type MediaMetadataEvidence = {
  revision: number;
  values: MediaMetadataValueEvidence[];
  relationships: MediaMetadataRelationshipEvidence[];
};

export type MediaHierarchyCounts = {
  seasonCount?: number;
  episodeCount?: number;
  releaseCount?: number;
  trackCount?: number;
  bookCount?: number;
  chapterCount?: number;
  itemCount?: number;
};

export type MediaPerson = {
  id?: string;
  name: string;
  role: string;
  character?: string;
  sortOrder?: number;
  source?: string;
  imageUrl?: string;
  providerIds?: Record<string, string>;
};

export type MediaStream = Stream;

export type MediaChapter = {
  id: string;
  title: string;
  startSeconds: number;
  endSeconds?: number;
  thumbUrl?: string;
};

export type MediaOptimizedVersion = {
  id: string;
  profile: string;
  profileName?: string;
  path?: string;
  sizeBytes: number;
  createdAt: string;
  updatedAt: string;
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
  width?: number;
  height?: number;
  bitrate?: number;
  durationSeconds?: number;
  available: boolean;
};

export type MediaJobType = 'metadata_refresh' | 'media_analyze' | 'optimize_version';

export type MediaAnalysisMode = 'probe' | 'full';

export type MediaJob = {
  id: string;
  type: string;
  status: string;
  progress: number;
  message: string;
  resourceType?: string;
  resourceId?: string;
  metadata?: Record<string, string>;
  attemptCount?: number;
  nextRunAt?: string;
  lastError?: string;
  failureKind?: string;
  createdAt: string;
  updatedAt: string;
};

export type MediaJobOptions = {
  profile?: string;
  analysisMode?: MediaAnalysisMode;
};

export type MediaOptimizationProfile = {
  id: string;
  label: string;
  height: number;
  videoKbps: number;
  audioKbps: number;
  default?: boolean;
};

export type MediaDownloadOption = {
  id: string;
  kind: 'source' | 'optimized';
  profile?: string;
  label: string;
  description?: string;
  available: boolean;
  requiresOptimizedVersion?: boolean;
  url?: string;
  sizeBytes?: number;
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
  sourceKind?: 'local' | 'remote' | 'optimized' | 'unknown' | string;
  job?: MediaJob;
};

export type MediaDownloadOptions = {
  options: MediaDownloadOption[];
  optimizedVersions: MediaOptimizedVersion[];
  profiles: MediaOptimizationProfile[];
  defaultProfile: string;
  canDownload: boolean;
};

export type MediaExtraRelationship = {
  label: string;
  type: 'trailer' | 'featurette' | 'deleted_scene' | 'behind_the_scenes' | 'interview' | 'scene' | 'short' | 'other';
  items: MediaItem[];
};

/** A server-indexed source version that a viewer may explicitly play. */
export type MediaFileVersion = {
  id: string;
  path?: string;
  originalFilename?: string;
  sourceType?: string;
  streamAnalysisStatus?: string;
  source?: string;
  releaseGroup?: string;
  threeD?: boolean;
  versionGroup?: string;
  qualityRank?: number;
  quality?: string;
  container?: string;
  versionLabel?: string;
  resolution?: string;
  videoCodec?: string;
  audioCodec?: string;
  dynamicRange?: string;
  sizeBytes?: number;
  available: boolean;
  missingSince?: string;
  selected?: boolean;
  durationSeconds?: number;
  bitrate?: number;
  width?: number;
  height?: number;
  frameRate?: number;
  aspectRatio?: string;
  videoProfile?: string;
  videoLevel?: number;
  bitDepth?: number;
  pixelFormat?: string;
  colorTransfer?: string;
  colorPrimaries?: string;
  colorSpace?: string;
  chromaLocation?: string;
  audioChannels?: number;
  audioChannelLayout?: string;
  audioSampleRate?: number;
  audioBitrate?: number;
  streams?: MediaStream[];
};

export type MediaItem = {
  id: string;
  title: string;
  subtitle: string;
  year: number;
  type: 'show' | 'movie' | 'music';
  kind: MediaKind;
  poster: string;
  backdrop: string;
  /** Role-keyed artwork preserved from the Product Contract wire model. */
  artwork?: Record<string, string>;
  progress?: number;
  rating: string;
  length: string;
  durationSeconds?: number;
  genre: string;
  addedAt?: string;
  libraryId?: string;
  libraryName?: string;
  counts?: MediaHierarchyCounts;
  entityKind?: string;
  actions?: string[];
  availability?: 'available' | 'partial' | 'unavailable';
  missing?: boolean;
  watchlisted?: boolean;
  favorite?: boolean;
  watched?: boolean;
  reaction?: '' | 'like' | 'dislike';
  userRating?: number;
  progressSeconds?: number;
  summary?: string;
  tagline?: string;
  sortTitle?: string;
  originalTitle?: string;
  edition?: string;
  contentRating?: string;
  communityRating?: number;
  criticRating?: number;
  country?: string;
  studio?: string;
  network?: string;
  tags?: string[];
  labels?: string[];
  lockedFields?: string[];
  providerIds?: MediaProviderIdentity[];
  metadataEvidence?: MediaMetadataEvidence;
  parentId?: string;
  parentTitle?: string;
  grandparentId?: string;
  grandparentTitle?: string;
  seasonNumber?: number;
  episodeNumber?: number;
  indexNumber?: number;
  fileCount?: number;
  missingFileCount?: number;
  people?: MediaPerson[];
  extras?: MediaExtraRelationship[];
  mediaFiles?: MediaFileVersion[];
  optimizedVersions?: MediaOptimizedVersion[];
  streams?: MediaStream[];
  typedMetadata?: Record<string, string>;
  children?: MediaItem[];
  childrenTruncated?: boolean;
  chapters?: MediaChapter[];
  playbackTarget?: MediaItem;
  recommendationRows?: HomeRow[];
  mediaImages?: MediaImage[];
  attachments?: MediaAttachment[];
  lyrics?: MediaLyric[];
};

export type HomeRow = {
  id: string;
  title: string;
  detail?: string;
  type: string;
	artworkShape?: 'square' | 'poster' | 'landscape';
  items: MediaItem[];
  hasMore: boolean;
  nextCursor?: string | null;
  endpoint?: string;
  explanation?: string;
  controls?: Array<'hide' | 'reorder' | 'pin'>;
  required?: boolean;
  hideable?: boolean;
  reorderable?: boolean;
  critical?: boolean;
  cursorCapable?: boolean;
  defaultVisible?: boolean;
  cacheTtlSeconds?: number;
  kind?: string;
  libraryId?: string;
  policyState?: string;
  priority?: number;
  privacySensitivity?: string;
};

export type HomeResult = {
  pivots: string[];
  rows: HomeRow[];
};

export type WebDisplayPreference = {
  preferences: Record<string, unknown>;
  updatedAt?: string;
};

export type SearchGroup = {
  id: string;
  title: string;
  entityKind: string;
  status: 'success' | 'error';
  errorCode?: 'search_group_unavailable' | 'search_group_timeout';
  messageId?: 'search.group-unavailable' | 'search.group-timeout';
  items: SearchResult[];
  hasMore: boolean;
  nextCursor?: string | null;
};

export type SearchPageResult = {
  query: string;
  sort: SearchSort;
  direction: SearchDirection;
  groups: SearchGroup[];
};

export type SearchSort = 'relevance' | 'title' | 'releaseYear' | 'dateAdded';
export type SearchDirection = 'asc' | 'desc';

export type SearchPageInput = {
  query: string;
  cursor?: string;
  group?: string;
  entityKinds?: string[];
  libraryIds?: string[];
  sort?: SearchSort;
  direction?: SearchDirection;
  limit?: number;
  recordHistory?: boolean;
};

export type PersonDetail = {
  id: string;
  name: string;
  imageUrl?: string;
  biography?: string;
  knownFor?: string;
  credits: MediaItem[];
  hasMore: boolean;
  nextCursor?: string | null;
};

export type MediaChildrenPage = {
  items: MediaItem[];
  hasMore: boolean;
  nextCursor?: string | null;
};

export type SearchHistoryItem = {
  query: string;
  useCount?: number;
  lastUsedAt?: string;
};

export type MediaDeleteResult = {
  ok: boolean;
  deletedItems: number;
  trashedFiles: number;
};

export type Viewer = {
  authenticated: boolean;
  setupRequired: boolean;
  serverName: string;
  user?: {
    id: string;
    displayName: string;
    email: string;
    role: 'owner' | 'user';
    profileImageUrl?: string;
    authOrigin?: 'local' | 'portico';
    authProvider?: 'local' | 'portico' | 'api_key';
    hasLocalPassword?: boolean;
    permissions?: Record<string, boolean>;
    preferences?: {
      sidebarOrder: string[];
      musicPlayback?: MusicPlaybackPreferences;
    };
  };
  authCapabilities?: AuthCapabilities;
  /** Present only after the server has returned a complete, verified /api/auth/me principal. */
  viewerScope?: ViewerScope;
};

export type MusicPlaybackPreferences = {
  shuffleDefault: boolean;
  repeatDefault: 'none' | 'one' | 'all';
  autoplayDefault: boolean;
  normalizationMode: 'off' | 'attenuate';
  crossfadeSeconds: number;
  gapless: boolean;
};

export type BrowserAccountSummary = {
	// Stable server-local account ID. This is display identity, not a credential.
	id: string;
	displayName: string;
	profileImageUrl?: string;
	authOrigin: 'local' | 'portico';
	authProvider: 'local' | 'portico';
	lastUsedAt: string;
};

export type BrowserAccountsState = {
	accounts: BrowserAccountSummary[];
	activeAccountId?: string;
	automaticSignIn: boolean;
	selectionRequired: boolean;
	canAddAccount: boolean;
};

export type BrowserAccountRemoval = {
	ok: boolean;
	vaultRevoked?: boolean;
	activeAccountRemoved?: boolean;
};

export type LocalProfileLoginChallenge = {
	accountAuthenticationToken: string;
	directory: ProfileDirectory;
	expiresAt: string;
	installationId: string;
	rememberOnBrowser: boolean;
};

export type LocalProfileSelection = {
	challenge: LocalProfileLoginChallenge;
	grant: ProfileSelectionGrant;
};

// Retained as a transport shape for callers that need to surface a server
// profile directory. The audited browser-session publication flow continues
// to use LocalProfileLoginChallenge and LocalProfileSelection above.
export type LocalProfileLaunchChallenge = {
	authority: 'local';
	accountId: string;
	serverId: string;
	profiles: ServerManagedProfileDirectory['profiles'];
};

export type AuthCapabilities = {
  setupRequired: boolean;
  localCredentialsEnabled: boolean;
  porticoAccountAuthEnabled: boolean;
  serverFriendlyName: string;
  publicUserPickerEnabled: boolean;
  visibleUsers: Array<{ id: string; displayName: string }>;
};

export type BrowsePivot = {
  id: string;
  label: string;
  entityKinds: string[];
  defaultView: string;
  supportedViews: string[];
  browseSupported: boolean;
  endpointTemplate: string;
};

export type BrowseSortOption = {
  id: string;
  label: string;
  defaultDirection: 'asc' | 'desc';
};

export type LibraryCapabilities = {
  apiVersion: 'v1';
  pivots: BrowsePivot[];
  sorts: BrowseSortOption[];
  actions: string[];
};

export type LibraryBrowseInput = {
  kind: LibraryKind;
  pivot: string;
  filter: string;
  sort: string;
  direction: 'ascending' | 'descending';
  search?: string;
};

export type LibraryBrowseResult = {
  items: MediaItem[];
  total: number;
  libraryId?: string;
  nextCursor?: string | null;
  hasMore?: boolean;
  capabilities?: LibraryCapabilities;
};

export type SearchResult = MediaItem;

export type MediaMetadataUpdate = {
  title?: string;
  sortTitle?: string;
  originalTitle?: string;
  edition?: string;
  year?: number;
  durationSeconds?: number;
  summary?: string;
  tagline?: string;
  contentRating?: string;
  communityRating?: number;
  criticRating?: number;
  studio?: string;
  network?: string;
  country?: string;
  genres?: string[];
  tags?: string[];
  labels?: string[];
  people?: MediaPerson[];
  seasonNumber?: number;
  episodeNumber?: number;
  indexNumber?: number;
  typedMetadata?: Record<string, string>;
  lockedFields?: string[];
};

export type MediaMatchCandidate = {
  provider: string;
  externalId: string;
  externalType: string;
  source: string;
  score: number;
  accepted: boolean;
  title?: string;
  year?: number;
  overview?: string;
  posterUrl?: string;
};

export type SavedResourceKind = 'playlist' | 'collection' | 'view';

export type SavedResourceShare = {
  userId: string;
  displayName: string;
  canEdit: boolean;
};

export type SavedResourceShareRequest = Pick<SavedResourceShare, 'userId' | 'canEdit'>;

export type SavedShareCandidate = {
  userId: string;
  displayName: string;
};

export type SavedShareCandidatePage = {
  items: SavedShareCandidate[];
  hasMore: boolean;
};

export type SavedResourceSummary = {
  id: string;
  kind: SavedResourceKind;
  ownerUserId?: string;
  title: string;
  summary?: string;
  itemCount: number;
  canEdit: boolean;
  visibility?: 'private' | 'server';
  shares?: SavedResourceShare[];
  libraryId?: string;
  libraryName?: string;
  pivot?: string;
  isPinned?: boolean;
  createdAt: string;
  updatedAt: string;
};

export type SavedResourceItemsInput = {
  cursor?: string;
  limit?: number;
};

export type SavedPlaylistEntry = {
  entryId: string;
  media: MediaItem;
  position: number;
};

type SavedResourcePageBase<K extends SavedResourceKind> = {
  kind: K;
  resource: SavedResourceSummary & { kind: K };
  hasMore: boolean;
  nextCursor?: string | null;
};

export type SavedResourceDetail<K extends SavedResourceKind = SavedResourceKind> = K extends 'playlist'
  ? SavedResourcePageBase<'playlist'> & { entries: SavedPlaylistEntry[] }
  : SavedResourcePageBase<Exclude<K, 'playlist'>> & { items: MediaItem[] };

export type SavedResourceCreateInput = {
  title: string;
  summary?: string;
  visibility?: 'private' | 'server';
  shares?: SavedResourceShareRequest[];
  mediaIds?: string[];
  libraryId?: string;
  pivot?: string;
  isPinned?: boolean;
};

export type SavedResourceUpdateInput = {
  title: string;
  summary?: string;
  visibility?: 'private' | 'server';
  shares?: SavedResourceShareRequest[];
  libraryId?: string;
  pivot?: string;
  isPinned?: boolean;
};

export type SavedEditableResourceKind = Exclude<SavedResourceKind, 'view'>;

export type PlaylistItemsMutation = {
  addMediaIds?: string[];
  removeEntryIds?: string[];
  orderEntryIds?: string[];
  expectedUpdatedAt?: string;
};

export type CollectionMembershipMutation = {
  addMediaIds?: string[];
  removeMediaIds?: string[];
  expectedUpdatedAt?: string;
};

export type SavedResourceItemsMutation<K extends SavedEditableResourceKind = SavedEditableResourceKind> = K extends 'playlist'
  ? PlaylistItemsMutation
  : CollectionMembershipMutation;

export type ActionableLiveTVSource = LiveTVSourceSummary & { actions?: string[] };
export type ActionableLiveTVChannel = LiveTVChannel & { actions?: string[] };
export type ActionableLiveTVProgram = LiveTVProgram & { actions?: string[] };
export type ActionableDVRRecording = DVRRecording & { actions?: string[] };
export type ActionableDVRRule = DVRRecordingRule & { actions?: string[] };

export type LiveTVGuideInput = {
  from: string;
  hours: number;
  query?: string;
  favoritesOnly?: boolean;
};

export type LiveTVGuideResult = Omit<LiveTVGuideResponse, 'source' | 'channels' | 'programs'> & {
  source: ActionableLiveTVSource;
  channels: ActionableLiveTVChannel[];
  programs: ActionableLiveTVProgram[];
  actions?: string[];
  capabilities: {
    canPlay: boolean;
    canFavoriteChannels: boolean;
    canScheduleRecordings: boolean;
    canManageRecordingRules: boolean;
    canManageSources: boolean;
  };
};

export type DVRResult = {
  recordings: ActionableDVRRecording[];
  rules: ActionableDVRRule[];
};

export type PlaybackStartOptions = {
  /** Opaque server-issued media version identifier. Never a file path. */
  versionId?: string;
  skipPreroll?: boolean;
  burnInSubtitleId?: string;
  audioStreamId?: string;
  startSeconds?: number;
  queueMediaIds?: string[];
  repeatMode?: PlaybackRepeatMode;
  sourceContext?: PlaybackSourceContext;
  intent?: PlaybackIntent;
};

export interface PorticoDataSource {
  authCapabilities(signal: AbortSignal): Promise<AuthCapabilities>;
  viewer(signal: AbortSignal): Promise<Viewer>;
  browserAccounts(signal: AbortSignal): Promise<BrowserAccountsState>;
  switchBrowserAccount(accountId: string, signal: AbortSignal): Promise<Viewer>;
  updateBrowserAccountPreferences(automaticSignIn: boolean, signal: AbortSignal): Promise<{ automaticSignIn: boolean }>;
  removeBrowserAccount(accountId: string, signal: AbortSignal): Promise<BrowserAccountRemoval>;
  signOutAllBrowserAccounts(signal: AbortSignal): Promise<void>;
  beginLocalProfileLogin(credentials: { login: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<LocalProfileLoginChallenge>;
  verifyLocalProfileSelection(challenge: LocalProfileLoginChallenge, profileId: string, pin: string | undefined, signal: AbortSignal): Promise<LocalProfileSelection>;
  publishLocalProfileSession(selection: LocalProfileSelection, signal: AbortSignal): Promise<Viewer>;
  switchAuthenticatedLocalProfile(profileId: string, pin: string | undefined, signal: AbortSignal): Promise<Viewer>;
  login(credentials: { login: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer>;
  setup(details: { serverName: string; username: string; email: string; displayName: string; password: string; rememberOnBrowser?: boolean }, signal: AbortSignal): Promise<Viewer>;
  startPorticoSetup(serverName: string, signal: AbortSignal): Promise<{ claimUrl: string; expiresAt?: string }>;
  porticoSetupStatus(signal: AbortSignal): Promise<{ setupRequired: boolean; remoteAccess: RemoteAccessStatus }>;
  logout(signal: AbortSignal): Promise<void>;
  updateProfile(profile: { displayName: string; email: string }, signal: AbortSignal): Promise<Viewer>;
  viewerPreferences(signal: AbortSignal): Promise<ServerViewerPreferenceBundle>;
  patchViewerPreference(
    scope: 'profile-server' | 'profile-device-class' | 'account-server-installation',
    expectedRevision: number,
    changes: Record<string, unknown>,
    signal: AbortSignal,
  ): Promise<ServerPatchedPreferenceDocument>;
  accountProfiles(signal: AbortSignal): Promise<ServerManagedProfileDirectory>;
  createProfileAdministrationProof(input: ProfileAdministrationProofInput, signal: AbortSignal): Promise<ServerProfileAdministrationProofResponse>;
  porticoProfileMFAStatus(signal: AbortSignal): Promise<{ enabled: boolean }>;
  requestPorticoProfileRecoveryEmail(signal: AbortSignal): Promise<void>;
  createAccountProfile(input: ServerManagedProfileCreateRequest, proof: string, signal: AbortSignal): Promise<ServerManagedProfileDirectory['profiles'][number]>;
  updateAccountProfile(profileId: string, input: ServerManagedProfileUpdateRequest, proof: string, signal: AbortSignal): Promise<ServerManagedProfileDirectory['profiles'][number]>;
  deleteAccountProfile(profileId: string, proof: string, signal: AbortSignal): Promise<ServerProfileErasureResponse | void>;
  reorderAccountProfiles(profileIds: string[], proof: string, signal: AbortSignal): Promise<ServerManagedProfileDirectory>;
  setAccountProfilePin(profileId: string, input: { pin: string } & ProfilePINReauthentication, proof: string, signal: AbortSignal): Promise<void>;
  clearAccountProfilePin(profileId: string, input: ProfilePINReauthentication, proof: string, signal: AbortSignal): Promise<void>;
  createAutomaticProfileTrust(signal: AbortSignal): Promise<AutomaticProfileTrust>;
  revokeAutomaticProfileTrust(signal: AbortSignal): Promise<void>;
  switchLocalProfile(input: { login: string; password: string; profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer>;
  switchHostedProfile(input: { profileId: string; pin?: string }, signal: AbortSignal): Promise<Viewer>;
  viewerNotifications(audience: NotificationAudience, cursor: string | undefined, signal: AbortSignal): Promise<NotificationPage>;
  watchViewerNotificationInvalidations(audience: NotificationAudience, onInvalidation: (event: NotificationInvalidation) => void, signal: AbortSignal): Promise<void>;
  watchApplicationEvents(onEvent: (event: AppEvent) => void, onReset: () => void | Promise<void>, signal: AbortSignal): Promise<void>;
  updateViewerNotificationReceipts(audience: NotificationAudience, input: { recipient: NotificationRecipient; notificationIds: string[]; action: NotificationReceiptAction; expectedRevision: number }, signal: AbortSignal): Promise<NotificationReceiptResult>;
  markAllViewerNotificationsRead(audience: NotificationAudience, signal: AbortSignal): Promise<NotificationReadAllResult>;
  viewerFeedbackCapabilities(signal: AbortSignal): Promise<ViewerFeedbackCapabilities>;
  submitViewerFeedback(input: ViewerFeedbackSubmission, signal: AbortSignal): Promise<ViewerFeedbackReceipt>;
  ownerViewerFeedback(status: 'new' | 'read' | 'resolved' | 'dismissed' | undefined, cursor: string | undefined, signal: AbortSignal): Promise<ServerOwnerFeedbackPage>;
  ownerViewerNotificationRecipients(signal: AbortSignal): Promise<ServerOwnerNotificationRecipientDirectory>;
  updateOwnerViewerFeedback(feedbackId: string, input: ServerOwnerFeedbackUpdateRequest, signal: AbortSignal): Promise<ServerOwnerFeedbackRecord>;
  createOwnerViewerNotice(input: ServerOwnerNoticeRequest, signal: AbortSignal): Promise<void>;
  productContract(signal: AbortSignal): Promise<ProductContract>;
  uploadClientDiagnostics?(input: { device?: string; app?: string; entries: Array<{ level: 'debug' | 'info' | 'warn' | 'error'; message: string; timestamp?: string; context?: Record<string, string> }> }, signal: AbortSignal): Promise<void>;
  searchContract(signal: AbortSignal): Promise<SearchContract>;
  home(signal: AbortSignal): Promise<HomeResult>;
  homeRow(id: string, cursor: string | undefined, signal: AbortSignal, limit?: number): Promise<HomeRow>;
  webDisplayPreferences(signal: AbortSignal): Promise<WebDisplayPreference>;
  updateWebDisplayPreferences(preferences: Record<string, unknown>, signal: AbortSignal): Promise<WebDisplayPreference>;
  libraryNavigation(signal: AbortSignal): Promise<LibraryNavigationPreferences>;
  updateLibraryNavigation(pinnedLibraryIds: string[], signal: AbortSignal): Promise<LibraryNavigationPreferences>;
  libraries(signal: AbortSignal): Promise<LibrarySummary[]>;
  browseLibrary(input: LibraryBrowseInput, signal: AbortSignal): Promise<LibraryBrowseResult>;
  search(query: string, signal: AbortSignal, limit?: number): Promise<SearchResult[]>;
  searchPage(input: SearchPageInput, signal: AbortSignal): Promise<SearchPageResult>;
  searchHistory(signal: AbortSignal): Promise<SearchHistoryItem[]>;
  clearSearchHistory(signal: AbortSignal): Promise<void>;
  person(id: string, signal: AbortSignal, cursor?: string): Promise<PersonDetail>;
  media(id: string, signal: AbortSignal): Promise<MediaItem>;
  mediaChildren(id: string, signal: AbortSignal, cursor?: string, limit?: number): Promise<MediaChildrenPage>;
  mediaTrickplay(id: string, signal: AbortSignal): Promise<MediaTrickplaySet[]>;
  deleteMedia(id: string, input: { deleteFiles: boolean; confirmation?: string }, signal: AbortSignal): Promise<MediaDeleteResult>;
  watchlist(signal: AbortSignal): Promise<MediaItem[]>;
  favorites(signal: AbortSignal): Promise<MediaItem[]>;
  setWatchlist(id: string, watchlisted: boolean, signal: AbortSignal): Promise<MediaItem>;
  setFavorite(id: string, favorite: boolean, signal: AbortSignal): Promise<MediaItem>;
  setReaction(id: string, reaction: '' | 'like' | 'dislike', signal: AbortSignal): Promise<MediaItem>;
  setRating(id: string, rating: number, signal: AbortSignal): Promise<MediaItem>;
  setWatched(id: string, watched: boolean, signal: AbortSignal): Promise<MediaItem>;
  updateMediaMetadata(ids: string[], patch: MediaMetadataUpdate, signal: AbortSignal): Promise<MediaItem[]>;
  uploadMediaImage(id: string, type: string, file: File, signal: AbortSignal): Promise<void>;
  deleteMediaImage(id: string, imageId: string, signal: AbortSignal): Promise<void>;
  setPreferredMediaImage(id: string, imageId: string, signal: AbortSignal): Promise<void>;
  reorderMediaImages(id: string, imageIds: string[], signal: AbortSignal): Promise<void>;
  uploadSubtitle(id: string, file: File, language: string, label: string, signal: AbortSignal): Promise<void>;
  updateSubtitle(id: string, streamId: string, offsetMs: number, signal: AbortSignal): Promise<void>;
  deleteSubtitle(id: string, streamId: string, signal: AbortSignal): Promise<void>;
  uploadLyrics(id: string, file: File, language: string, signal: AbortSignal): Promise<void>;
  fetchLyrics(id: string, signal: AbortSignal): Promise<void>;
  searchLyrics(id: string, query: string, signal: AbortSignal): Promise<LyricSearchCandidate[]>;
  applyLyrics(id: string, candidate: Pick<LyricSearchCandidate, 'provider' | 'externalId'>, signal: AbortSignal): Promise<void>;
  deleteLyrics(id: string, lyricId: string, signal: AbortSignal): Promise<void>;
  searchMediaMatches(id: string, query: string, signal: AbortSignal): Promise<MediaMatchCandidate[]>;
  applyMediaMatch(id: string, candidate: Pick<MediaMatchCandidate, 'provider' | 'externalId' | 'externalType'>, signal: AbortSignal): Promise<MediaItem>;
  queueMediaJob(id: string, type: MediaJobType, options: MediaJobOptions, signal: AbortSignal): Promise<MediaJob>;
  mediaDownloadOptions(id: string, signal: AbortSignal): Promise<MediaDownloadOptions>;
  downloadPreparations(signal: AbortSignal): Promise<DownloadPreparation[]>;
  updateDownloadPreparation(id: string, action: DownloadPreparationUpdateRequest['action'], signal: AbortSignal): Promise<DownloadPreparation>;
  downloadPreparationURL(id: string, signal: AbortSignal): Promise<string>;
  createOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<MediaJob>;
  deleteOptimizedVersion(id: string, profile: string, signal: AbortSignal): Promise<void>;
  createMediaDownloadURL(id: string, profile: string, signal: AbortSignal): Promise<string>;
  savedShareCandidates(query: string, limit: number, signal: AbortSignal): Promise<SavedShareCandidatePage>;
  savedResources(kind: SavedResourceKind, signal: AbortSignal): Promise<SavedResourceSummary[]>;
  savedResource<K extends SavedResourceKind>(kind: K, id: string, input: SavedResourceItemsInput, signal: AbortSignal): Promise<SavedResourceDetail<K>>;
  createSavedResource(kind: SavedResourceKind, input: SavedResourceCreateInput, signal: AbortSignal): Promise<SavedResourceSummary>;
  updateSavedResource(kind: SavedResourceKind, id: string, input: SavedResourceUpdateInput, signal: AbortSignal): Promise<SavedResourceSummary>;
  deleteSavedResource(kind: SavedResourceKind, id: string, signal: AbortSignal): Promise<void>;
  mutateSavedResourceItems<K extends SavedEditableResourceKind>(kind: K, id: string, mutation: SavedResourceItemsMutation<K>, signal: AbortSignal): Promise<SavedResourceSummary>;
  startPlayback(mediaId: string, options: PlaybackStartOptions, signal: AbortSignal): Promise<PlaybackResponse>;
  restorePlayback(signal: AbortSignal, intent?: PlaybackIntent): Promise<PlaybackRestoreResponse>;
  touchPlayback(sessionId: string, event: PlaybackProgressInput, signal?: AbortSignal, keepalive?: boolean): Promise<PlaybackProgressAcknowledgement>;
  renewPlaybackMediaGrant(sessionId: string, signal: AbortSignal): Promise<MediaGrant>;
  renegotiatePlayback(sessionId: string, request: PlaybackRenegotiationRequest, signal: AbortSignal): Promise<PlaybackResponse>;
  stopPlayback(sessionId: string, signal?: AbortSignal, keepalive?: boolean): Promise<void>;
  playbackSessionQueue(sessionId: string, signal: AbortSignal): Promise<PlaybackSessionQueueResponse>;
  updatePlaybackSessionQueue(sessionId: string, request: PlaybackSessionQueueReplaceRequest, signal: AbortSignal): Promise<PlaybackSessionQueueResponse>;
  mutatePlaybackSessionQueue(sessionId: string, request: PlaybackSessionQueueRequest, signal: AbortSignal): Promise<PlaybackSessionQueueResponse>;
  prepareNextPlayback(sessionId: string, signal: AbortSignal, request?: PlaybackPrepareNextRequest): Promise<PlaybackPreparedResponse>;
  handoffPlayback(sessionId: string, request: PlaybackHandoffRequest, signal: AbortSignal): Promise<PlaybackResponse>;
  playbackResourceUrl(path: string): string;
  liveTVSources(signal: AbortSignal): Promise<ActionableLiveTVSource[]>;
  liveTVGuide(sourceId: string, input: LiveTVGuideInput, signal: AbortSignal): Promise<LiveTVGuideResult>;
  liveTVChannels(sourceId: string, signal: AbortSignal): Promise<ActionableLiveTVChannel[]>;
  updateLiveTVChannel(channelId: string, state: { favorite?: boolean }, signal: AbortSignal): Promise<ActionableLiveTVChannel>;
  startLiveTVPlayback(channelId: string, signal: AbortSignal): Promise<PlaybackResponse>;
  startDVRPlayback(recordingId: string, signal: AbortSignal): Promise<PlaybackResponse>;
  libraryChannels(signal: AbortSignal): Promise<LibraryChannelListResponse>;
  libraryChannelGuide(channelId: string, input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelGuide>;
  libraryChannelsGuide(input: { from?: string; to?: string; cursor?: string; limit?: number }, signal: AbortSignal): Promise<LibraryChannelsGuide>;
  startLibraryChannelPlayback(channelId: string, signal: AbortSignal): Promise<PlaybackResponse>;
  adminLibraryChannels(signal: AbortSignal): Promise<AdminLibraryChannelListResponse>;
  libraryChannel(channelId: string, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  createLibraryChannel(input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  updateLibraryChannel(channelId: string, input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  deleteLibraryChannel(channelId: string, expectedRevision: number, signal: AbortSignal): Promise<void>;
  libraryChannelTemplates(signal: AbortSignal): Promise<LibraryChannelTemplatesResponse>;
  restoreLibraryChannelDefaults(input: LibraryChannelRestoreDefaultsRequest, signal: AbortSignal): Promise<LibraryChannelRestoreDefaultsResponse>;
  regenerateLibraryChannel(channelId: string, signal: AbortSignal): Promise<LibraryChannelGeneration>;
  dvr(signal: AbortSignal): Promise<DVRResult>;
  createDVRRecording(input: { sourceId: string; channelId?: string; programId?: string; title: string; startsAt: string; endsAt: string }, signal: AbortSignal): Promise<ActionableDVRRecording>;
  createDVRRule(input: { sourceId: string; channelId?: string; programId?: string; title: string; matchType: string; priority?: number }, signal: AbortSignal): Promise<ActionableDVRRule>;
  updateDVRRule(id: string, input: Partial<ActionableDVRRule> & { sourceId: string; title: string }, signal: AbortSignal): Promise<ActionableDVRRule>;
  deleteDVRRecording(id: string, signal: AbortSignal): Promise<void>;
  deleteDVRRule(id: string, signal: AbortSignal): Promise<void>;
}
