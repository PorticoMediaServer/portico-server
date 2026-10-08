// The contract between the Portico web client and a Portico server.
// The fixture implementation (./fixture) satisfies it in-memory with simulated
// latency; ./http.ts shows the same contract spoken over the REST API.
import type {
  Album,
  Artist,
  Audiobook,
  CastTarget,
  Channel,
  Collection,
  Device,
  Download,
  Episode,
  HistoryEntry,
  HomeFeed,
  HomeVideo,
  LibraryCard,
  LibraryQuery,
  Memory,
  Movie,
  Notification,
  Page,
  Person,
  Photo,
  PhotoAlbum,
  Playlist,
  Profile,
  Program,
  Recording,
  ScheduledRecording,
  SearchResults,
  SeriesRule,
  Shelf,
  Show,
  Track,
} from '../data/types';
import type { LibraryInfo } from '../data/fixtures/index';
import type {
  ActivityEvent,
  LibraryAdmin,
  LogLevel,
  LogLine,
  MetricPoint,
  ServerInfo,
  Session,
  Task,
  UserAdmin,
} from '../data/fixtures/server';

export type { LibraryInfo, ActivityEvent, LibraryAdmin, LogLevel, LogLine, MetricPoint, ServerInfo, Session, Task, UserAdmin };

export interface ServerSummary {
  id: string;
  name: string;
  owner: string;
  version: string;
  address: string;
  local: boolean;
  online: boolean;
  libraries: number;
  latencyMs: number;
}

export interface AuthResult {
  ok: boolean;
  error?: string;
  needsMfa?: boolean;
}

export interface ItemState {
  progress?: { position: number; duration: number };
  watched: boolean;
  inWatchlist: boolean;
  rating?: number;
  downloaded?: Download;
}

export interface MovieDetail {
  movie: Movie;
  state: ItemState;
  related: LibraryCard[];
  collections: (Collection & { items: LibraryCard[] })[];
}

export interface ShowDetail {
  show: Show;
  state: ItemState;
  watched: Record<string, boolean>;
  progress: Record<string, { position: number; duration: number }>;
  nextUp?: { episode: Episode; label: string; position?: number };
  related: LibraryCard[];
}

export interface EpisodeDetail {
  episode: Episode;
  show: Show;
  state: ItemState;
  prev?: Episode;
  next?: Episode;
}

export interface PersonDetail {
  person: Person;
  credits: (LibraryCard & { role: string })[];
}

export interface ArtistDetail {
  artist: Artist;
  albums: Album[];
  topTracks: Track[];
  similar: Artist[];
  appearsOn: Playlist[];
}

export interface MusicHome {
  recent: LibraryCard[];
  shelves: Shelf[];
  playlists: Playlist[];
  artists: Artist[];
  albums: Album[];
}

export interface GuideData {
  start: number;
  hours: number;
  channels: Channel[];
  programs: Program[];
  scheduledProgramIds: string[];
}

export interface DvrData {
  recordings: Recording[];
  scheduled: ScheduledRecording[];
  rules: SeriesRule[];
  diskUsedBytes: number;
  diskLimitBytes: number;
}

export interface PlayableSource {
  itemId: string;
  kind: 'video' | 'audio';
  url: string;
  /** real media duration; the fixture maps longer runtimes onto it */
  mediaDuration?: number;
  duration: number;
  scrub?: { url: string; every: number; cols: number; rows: number; width: number; height: number };
  startAt: number;
}

export interface SecurityState {
  mfa: { totp: boolean; passkeys: { id: string; name: string; created: string; lastUsed: string }[]; recoveryCodesLeft: number };
  password: { lastChanged: string; strength: 'strong' | 'ok' | 'weak' };
  devices: Device[];
  signInHistory: { at: string; device: string; location: string; ip: string; ok: boolean; method: string }[];
}

export interface AdminOverview {
  server: ServerInfo;
  metrics: MetricPoint[];
  sessions: Session[];
  activity: ActivityEvent[];
  health: { ok: number; warn: number; info: number };
  libraries: LibraryInfo[];
  storage: { used: number; size: number };
}

export interface PorticoApi {
  // session
  listServers(): Promise<ServerSummary[]>;
  signIn(email: string, password: string): Promise<AuthResult>;
  verifyMfa(code: string): Promise<AuthResult>;
  signInWithPasskey(): Promise<AuthResult>;
  getProfiles(): Promise<Profile[]>;
  unlockProfile(id: string, pin: string): Promise<boolean>;
  createProfile(p: Partial<Profile> & { name: string }): Promise<Profile>;
  updateProfile(id: string, patch: Partial<Profile>): Promise<Profile>;
  deleteProfile(id: string): Promise<void>;

  // browse
  getLibraries(): Promise<LibraryInfo[]>;
  getHome(): Promise<HomeFeed>;
  getLibraryHome(libraryId: string): Promise<{ hero?: HomeFeed['hero']; shelves: Shelf[] }>;
  getLibraryItems(libraryId: string, q: LibraryQuery): Promise<Page<LibraryCard> & { genres: string[] }>;
  getMovie(id: string): Promise<MovieDetail>;
  getShow(id: string): Promise<ShowDetail>;
  getEpisode(id: string): Promise<EpisodeDetail>;
  getCollection(id: string): Promise<Collection & { items: LibraryCard[] }>;
  getCollections(libraryId: string): Promise<(Collection & { count: number })[]>;
  getPerson(id: string): Promise<PersonDetail>;
  getMusicHome(): Promise<MusicHome>;
  getArtists(): Promise<Artist[]>;
  getAlbums(sort?: 'added' | 'title' | 'artist' | 'year'): Promise<Album[]>;
  getSongs(offset: number, limit: number, sort?: 'title' | 'artist' | 'album' | 'plays'): Promise<Page<Track>>;
  getArtist(id: string): Promise<ArtistDetail>;
  getAlbum(id: string): Promise<{ album: Album; more: Album[] }>;
  getPlaylists(): Promise<Playlist[]>;
  getPlaylist(id: string): Promise<{ playlist: Playlist; tracks: Track[] }>;
  getTracks(ids: string[]): Promise<Track[]>;
  getAudiobooks(): Promise<(Audiobook & { progress?: number })[]>;
  getAudiobook(id: string): Promise<{ book: Audiobook; state: ItemState; more: Audiobook[] }>;
  getPhotos(albumId?: string): Promise<Photo[]>;
  getPhotoAlbums(): Promise<PhotoAlbum[]>;
  getMemories(): Promise<Memory[]>;
  getHomeVideos(): Promise<HomeVideo[]>;
  getHomeVideo(id: string): Promise<{ video: HomeVideo; state: ItemState; more: HomeVideo[] }>;
  search(query: string, kinds?: string[]): Promise<SearchResults>;
  getRecentSearches(): Promise<string[]>;

  // personal
  getWatchlist(): Promise<LibraryCard[]>;
  toggleWatchlist(itemId: string): Promise<boolean>;
  setWatched(itemId: string, watched: boolean): Promise<void>;
  setRating(itemId: string, rating: number): Promise<void>;
  getHistory(): Promise<HistoryEntry[]>;
  clearHistory(): Promise<void>;
  saveProgress(itemId: string, position: number, duration: number): Promise<void>;
  getContinue(): Promise<LibraryCard[]>;
  getDownloads(): Promise<Download[]>;
  startDownload(itemId: string, quality: Download['quality']): Promise<Download>;
  removeDownload(id: string): Promise<void>;
  getNotifications(): Promise<Notification[]>;
  markNotificationsRead(ids?: string[]): Promise<void>;
  getCastTargets(): Promise<CastTarget[]>;
  getSecurity(): Promise<SecurityState>;
  revokeDevice(id: string): Promise<void>;

  // playback
  getPlayable(itemId: string): Promise<PlayableSource>;

  // live TV
  getGuide(): Promise<GuideData>;
  getDvr(): Promise<DvrData>;
  scheduleRecording(programId: string, series: boolean): Promise<ScheduledRecording[]>;
  cancelRecording(id: string): Promise<void>;
  deleteRecording(id: string): Promise<void>;

  // admin console
  getAdminOverview(): Promise<AdminOverview>;
  getMetrics(sinceMs: number): Promise<MetricPoint[]>;
  getSessions(): Promise<Session[]>;
  terminateSession(id: string, message: string): Promise<void>;
  getActivity(): Promise<ActivityEvent[]>;
  getLogs(q: { levels?: LogLevel[]; source?: string; text?: string; offset: number; limit: number }): Promise<Page<LogLine>>;
  getLibrariesAdmin(): Promise<LibraryAdmin[]>;
  scanLibrary(id: string): Promise<void>;
  getUsers(): Promise<UserAdmin[]>;
  updateUser(id: string, patch: Partial<UserAdmin>): Promise<UserAdmin>;
  inviteUser(email: string, libraries: string[]): Promise<UserAdmin>;
  getTasks(): Promise<Task[]>;
  runTask(id: string): Promise<void>;
  getAdminSection<T = unknown>(section: 'transcoding' | 'network' | 'storage' | 'backups' | 'livetv' | 'metadata' | 'diagnostics' | 'settings'): Promise<T>;
  updateAdminSection(section: string, patch: Record<string, unknown>): Promise<void>;
  runBackup(): Promise<void>;
  runDiagnostics(): Promise<void>;
}
