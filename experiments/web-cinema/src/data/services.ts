import type {
  Account, Channel, Collection, DeviceSession, Download, DownloadRequest, DownloadSettings, FeedbackReport, FilterState, HomeLayoutEntry, Invitation, Item, Library,
  NotificationItem, Person, Playlist, Profile, ProfileLimits, Programme, Recording, RecordingRule, Row, SavedView, SearchGroup, Server, SortState, TogetherGroup, UserState,
} from './types';
import type {AdminService} from './admin';

/**
 * The client's view of a Portico server and the Portico Account. Screens only ever talk to these
 * interfaces (through `useServices()`), so replacing the fixture backend with real HTTP services
 * is a data-layer change. Methods are async to match real network calls.
 */

export type Page<T> = {items: T[]; total: number; cursor: string | null};

export type BrowseQuery = {libraryId: string; filters: FilterState; sort: SortState; letter?: string; cursor?: string | null; pageSize?: number; kind?: string};

export interface AuthService {
  servers(): Promise<Server[]>;
  account(): Promise<Account | null>;
  signIn(username: string, password: string): Promise<{needsCode: boolean}>;
  signInDirect(address: string, username: string, password: string): Promise<{needsCode: boolean; mustChangePassword: boolean}>;
  verifyCode(code: string): Promise<void>;
  createAccount(name: string, username: string, email: string, password: string): Promise<void>;
  requestPasswordReset(usernameOrEmail: string): Promise<void>;
  resetPassword(password: string, code?: string): Promise<void>;
  verifyEmail(token: string): Promise<'done' | 'expired'>;
  profiles(serverId: string): Promise<Profile[]>;
  unlockProfile(profileId: string, pin: string | null): Promise<boolean>;
  tvLinkCode(): Promise<{code: string; url: string; expiresAt: number}>;
  lookupDevice(code: string): Promise<{name: string; platform: string; location: string} | null>;
  approveDevice(code: string): Promise<void>;
  invitation(token: string): Promise<{server: string; inviter: string; libraries: number} | null>;
  acceptInvitation(token: string, profileId: string): Promise<void>;
  joinDirect(code: string, name: string, username: string, password: string): Promise<void>;
  claim(): Promise<{server: string; address: string; requested: number}>;
  setupServer(input: {name: string; mode: 'account' | 'local'; username: string; password: string}): Promise<void>;
  signOut(): Promise<void>;
}

export interface CatalogService {
  libraries(): Promise<Library[]>;
  item(id: string): Promise<Item | undefined>;
  items(ids: string[]): Promise<Item[]>;
  person(id: string): Promise<Person | undefined>;
  personCredits(id: string): Promise<{cast: {itemId: string; character?: string}[]; crew: {itemId: string; job?: string}[]}>;
  home(): Promise<Row[]>;
  homeLayout(): Promise<HomeLayoutEntry[]>;
  setHomeLayout(layout: HomeLayoutEntry[]): Promise<void>;
  row(rowId: string): Promise<Row | undefined>;
  discover(libraryId: string): Promise<Row[]>;
  browse(q: BrowseQuery): Promise<Page<Item>>;
  letters(q: BrowseQuery): Promise<{letter: string; index: number}[]>;
  facets(libraryId: string): Promise<Record<string, {value: string; count: number}[]>>;
  categories(libraryId: string): Promise<{id: string; title: string; kind: 'genre' | 'studio' | 'network' | 'decade' | 'mood' | 'country'; count: number; art?: string; tint: string}[]>;
  entity(libraryId: string, view: string, entityId: string): Promise<{title: string; subtitle: string; itemIds: string[]; art?: string; tint: string}>;
  collections(libraryId?: string): Promise<Collection[]>;
  related(id: string): Promise<string[]>;
  children(id: string): Promise<string[]>;
  nextUp(showId: string): Promise<string | undefined>;
  search(query: string, kind: string): Promise<SearchGroup[]>;
  recentSearches(): Promise<string[]>;
  addRecentSearch(q: string): Promise<void>;
  clearRecentSearches(): Promise<void>;
  lyrics(trackId: string): Promise<{t: number; line: string}[] | null>;
  /** Playable length in seconds, synchronously from the client's item cache. */
  durationOf(id: string): number;
  scanLibrary(libraryId: string): Promise<void>;
}

export interface UserStateService {
  state(itemId: string): UserState;
  set(itemIds: string[], patch: Partial<UserState>): Promise<void>;
  markContainerWatched(containerId: string, watched: boolean): Promise<number>;
  removeFromContinue(itemId: string): Promise<void>;
  restoreContinue(itemId: string): Promise<void>;
  resetRecommendations(): Promise<void>;
  history(period: string): Promise<{itemId: string; at: number}[]>;
  clearHistory(): Promise<void>;
}

export interface ListsService {
  playlists(): Promise<Playlist[]>;
  playlist(id: string): Promise<Playlist | undefined>;
  createPlaylist(title: string, description: string, shared: boolean, itemIds?: string[]): Promise<Playlist>;
  addToPlaylist(id: string, itemIds: string[]): Promise<void>;
  removeFromPlaylist(id: string, index: number): Promise<void>;
  movePlaylistItem(id: string, from: number, to: number): Promise<void>;
  renamePlaylist(id: string, title: string): Promise<void>;
  deletePlaylist(id: string): Promise<void>;
  createCollection(title: string, description: string, shared: boolean, itemIds?: string[]): Promise<Collection>;
  addToCollection(id: string, itemIds: string[]): Promise<void>;
  removeFromCollection(id: string, itemId: string): Promise<void>;
  renameCollection(id: string, title: string): Promise<void>;
  deleteCollection(id: string): Promise<void>;
  savedViews(): Promise<SavedView[]>;
  saveView(view: Omit<SavedView, 'id'>): Promise<SavedView>;
  renameView(id: string, title: string): Promise<void>;
  deleteView(id: string): Promise<void>;
}

export interface LiveService {
  channels(): Promise<Channel[]>;
  guide(from: number, to: number): Promise<Programme[]>;
  programme(id: string): Promise<Programme | undefined>;
  setChannel(id: string, patch: Partial<Pick<Channel, 'favorite' | 'hidden'>>): Promise<void>;
  recordings(): Promise<Recording[]>;
  rules(): Promise<RecordingRule[]>;
  record(programmeId: string, opts: {series: boolean; newOnly?: boolean; keep?: number | 'all'; startEarly?: number; endLate?: number}): Promise<void>;
  cancelRecording(id: string): Promise<void>;
  deleteRecording(id: string): Promise<void>;
  keepRecording(id: string, keep: boolean): Promise<void>;
  resolveConflict(id: string, choice: 'skip' | 'record'): Promise<void>;
  updateRule(id: string, patch: Partial<RecordingRule>): Promise<void>;
  deleteRule(id: string, cancelUpcoming: boolean): Promise<void>;
  reminders(): Promise<string[]>;
  toggleReminder(programmeId: string): Promise<boolean>;
  storage(): Promise<{usedGB: number; totalGB: number; hoursLeft: number; retentionDays: number; pendingDeleteGB: number}>;
}

export interface DownloadsService {
  list(): Promise<Download[]>;
  requests(): Promise<DownloadRequest[]>;
  settings(): Promise<DownloadSettings>;
  updateSettings(patch: Partial<DownloadSettings>): Promise<void>;
  add(itemIds: string[], quality: string): Promise<number>;
  addShow(showId: string, policy: DownloadRequest['policy'], keepNext: number, quality: string): Promise<void>;
  pause(id: string): Promise<void>;
  resume(id: string): Promise<void>;
  remove(id: string): Promise<void>;
  storage(): Promise<{usedGB: number; freeGB: number; serverSpaceGB: number}>;
}

export interface AccountService {
  profiles(): Promise<Profile[]>;
  saveProfile(p: Profile): Promise<void>;
  addProfile(name: string, color: string, kids: boolean): Promise<Profile>;
  deleteProfile(id: string): Promise<void>;
  moveProfile(id: string, dir: -1 | 1): Promise<void>;
  sessions(): Promise<DeviceSession[]>;
  signOutSession(id: string): Promise<void>;
  signOutOthers(): Promise<void>;
  updateSession(id: string, patch: Partial<DeviceSession>): Promise<void>;
  account(): Promise<Account>;
  updateAccount(patch: Partial<Account>): Promise<void>;
  notifications(): Promise<NotificationItem[]>;
  updateNotification(id: string, patch: Partial<NotificationItem>): Promise<void>;
  markAllRead(): Promise<void>;
  invitations(): Promise<Invitation[]>;
  answerInvitation(id: string, accept: boolean): Promise<void>;
  servers(): Promise<Server[]>;
  leaveServer(id: string): Promise<void>;
}

export interface TogetherService {
  groups(): Promise<TogetherGroup[]>;
  group(id: string): Promise<TogetherGroup | undefined>;
  create(name: string, control: 'host' | 'everyone'): Promise<TogetherGroup>;
  join(code: string): Promise<TogetherGroup | null>;
  update(id: string, patch: Partial<TogetherGroup>): Promise<void>;
  leave(id: string): Promise<void>;
  end(id: string): Promise<void>;
  addToQueue(id: string, itemId: string): Promise<void>;
}

/** The profile's preferences, stored on the server so they follow the profile between devices. */
export type PreferenceValue = string | number | boolean | string[];
export interface PreferencesService {
  get(): Promise<Record<string, PreferenceValue>>;
  set(patch: Record<string, PreferenceValue>): Promise<void>;
}

export interface FeedbackService {
  send(input: {kind: string; category: string; message: string; itemId?: string; diagnostics: boolean}): Promise<'sent' | 'duplicate'>;
  reports(): Promise<FeedbackReport[]>;
  updateReport(id: string, patch: Partial<FeedbackReport>): Promise<void>;
}

export interface Services {
  auth: AuthService;
  catalog: CatalogService;
  user: UserStateService;
  lists: ListsService;
  live: LiveService;
  downloads: DownloadsService;
  account: AccountService;
  together: TogetherService;
  feedback: FeedbackService;
  preferences: PreferencesService;
  admin: AdminService;
  /** Who is viewing: the server applies this profile's limits (rating ceiling, hidden labels) to everything it returns. */
  setViewer(limits: ProfileLimits | null): void;
  /** Subscribe to data changes (any mutation). Real services would push server events here. */
  subscribe(listener: () => void): () => void;
  version(): number;
}
