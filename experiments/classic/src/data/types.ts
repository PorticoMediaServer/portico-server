import type {ImageSourcePropType} from 'react-native';

/**
 * Portico Classic's domain model. Screens only see these shapes, through the
 * `PorticoClient` service interface (services.ts). The fixture backend builds
 * them from bundled data; a real backend would project Portico's API into them.
 */
export type Img = ImageSourcePropType | undefined;

export type LibraryKind = 'movie' | 'tv' | 'anime' | 'music' | 'audiobook' | 'photo' | 'homevideo';
export type Library = {id: string; name: string; kind: LibraryKind; count: number; scanning?: {found: number}; lastScan?: string; path?: string};

export type ItemKind = 'movie' | 'show' | 'season' | 'episode' | 'artist' | 'album' | 'track' | 'audiobook' | 'chapter' | 'photo' | 'photoAlbum' | 'homevideo' | 'collection' | 'playlist' | 'person' | 'channel' | 'program' | 'recording' | 'view' | 'genre' | 'studio' | 'author' | 'series';
export type Shape = 'poster' | 'wide' | 'square' | 'circle';

/** A card-level projection of anything that can appear in a row, grid or list. */
export type Entry = {
  id: string;
  kind: ItemKind;
  title: string;
  subtitle?: string;
  year?: number;
  image?: Img;
  backdrop?: Img;
  shape: Shape;
  duration?: number;
  progress?: number;
  watched?: boolean;
  unwatched?: number;
  rating?: string;
  genres?: string[];
  overview?: string;
  libraryId?: string;
  parentId?: string;
  playable?: boolean;
  badge?: string;
  /** Plex-style overlay line on wide cards, for example "S2 · E4". */
  caption?: string;
  /** Episodes: the show's poster, used wherever episodes sit in poster rows. */
  poster?: Img;
  /** Collections: posters of the first few titles, for a stacked card. */
  stack?: Img[];
  watchlisted?: boolean;
  favorite?: boolean;
  recommendation?: boolean;
  reason?: string;
};

export type Personal = {
  watched: boolean;
  progress: number;
  watchlisted: boolean;
  favorite: boolean;
  rating?: number;
  reaction?: 'like' | 'dislike';
  lastPlayed?: number;
  notInterested?: boolean;
};

export type Credit = {personId: string; name: string; role: 'cast' | 'crew'; character?: string; job?: string; image?: Img};

export type MediaVersion = {id: string; label: string; resolution: string; video: string; audio: string; container: string; size: string; bitrate: string; hdr?: string; edition?: string};
export type Track = {id: string; label: string; language: string; codec?: string; channels?: string; forced?: boolean; default?: boolean; origin?: 'embedded' | 'sidecar' | 'provider'};
export type Chapter = {title: string; start: number; image?: Img};
export type Marker = {kind: 'intro' | 'recap' | 'credits'; start: number; end: number};

export type MovieDetail = {
  entry: Entry;
  tagline?: string;
  studio?: string;
  audienceRating?: number;
  criticRating?: number;
  directors: Credit[];
  writers: Credit[];
  cast: Credit[];
  versions: MediaVersion[];
  audio: Track[];
  subtitles: Track[];
  chapters: Chapter[];
  extras: Entry[];
  related: Entry[];
  collections: {id: string; name: string}[];
  personal: Personal;
  real: boolean;
  license?: string;
};

export type Season = {id: string; number: number; title: string; year?: number; episodes: EpisodeSummary[]; image?: Img; watchedCount: number};
export type EpisodeSummary = Entry & {season: number; episode: number; airDate?: string};
export type ShowDetail = {
  entry: Entry;
  network?: string;
  endYear?: number;
  audienceRating?: number;
  creators: Credit[];
  cast: Credit[];
  seasons: Season[];
  nextUp?: EpisodeSummary;
  related: Entry[];
  personal: Personal;
  watchedCount: number;
  episodeCount: number;
};
export type EpisodeDetail = {
  entry: EpisodeSummary;
  show: Entry;
  seasonTitle: string;
  directors: Credit[];
  writers: Credit[];
  cast: Credit[];
  versions: MediaVersion[];
  previous?: EpisodeSummary;
  next?: EpisodeSummary;
  personal: Personal;
  markers: Marker[];
};

export type PersonDetail = {id: string; name: string; image?: Img; realPhoto: boolean; born?: string; died?: string; bio?: string; knownFor: Entry[]; credits: {entry: Entry; role: 'cast' | 'crew'; character?: string; job?: string}[]};

export type TrackItem = {id: string; number: number; disc: number; title: string; duration: number; artist: string; artistId: string; album: string; albumId: string; image?: Img; hasLyrics: boolean; lyrics?: {at: number; line: string}[]};
export type ArtistDetail = {entry: Entry; bio: string; genre: string; albums: Entry[]; popular: TrackItem[]; similar: Entry[]; personal: Personal};
export type AlbumDetail = {entry: Entry; artist: Entry; genre: string; label: string; type: string; tracks: TrackItem[]; totalDuration: number; more: Entry[]; personal: Personal};

export type BookChapter = {number: number; title: string; duration: number; start: number};
export type AudiobookDetail = {entry: Entry; author: string; narrator: string; genre: string; publisher: string; series?: {name: string; index: number}; chapters: BookChapter[]; progress: number; personal: Personal; more: Entry[]};

export type PhotoItem = {id: string; title: string; taken: string; camera: string; portrait: boolean; image: Img; thumb: Img; albumId: string};
export type PhotoAlbumDetail = {entry: Entry; photos: PhotoItem[]};

export type PlaylistDetail = {id: string; title: string; kind: 'playlist' | 'collection' | 'view'; description?: string; shared: boolean; owner: string; items: Entry[]; smart?: boolean; totalDuration?: number; image?: Img; editable: boolean};

// ── Home ──────────────────────────────────────────────────────────────
export type HomeRowKind = 'continue' | 'upNext' | 'watchlist' | 'recent' | 'recommended' | 'picks' | 'because' | 'trending' | 'discover' | 'favorites' | 'view' | 'live';
export type HomeRow = {id: string; kind: HomeRowKind; title: string; entries: Entry[]; libraryId?: string; shape: Shape; total: number; required?: boolean; reason?: string};
export type HomeLayoutRow = {id: string; title: string; hidden: boolean; required: boolean; savedView?: boolean};
export type HomeDocument = {featured: Entry[]; rows: HomeRow[]};

// ── Library ───────────────────────────────────────────────────────────
export type LibraryView = 'discover' | 'browse' | 'collections' | 'categories' | 'albums' | 'songs' | 'authors' | 'series' | 'timeline';
export type SortField = 'title' | 'added' | 'year' | 'rating' | 'duration' | 'lastPlayed' | 'random' | 'artist' | 'plays';
export type Filters = {genre?: string[]; decade?: string[]; contentRating?: string[]; playState?: 'unwatched' | 'inProgress' | 'watched'; resolution?: string[]; studio?: string[]; favorite?: boolean; watchlisted?: boolean; quick?: string};
export type BrowseQuery = {sort: SortField; direction: 'asc' | 'desc'; filters: Filters; letter?: string};
export type BrowsePage = {entries: Entry[]; total: number; offset: number};
export type Facet = {field: keyof Filters; label: string; values: {value: string; label: string; count: number}[]};
export type Category = {id: string; kind: 'genre' | 'decade' | 'studio' | 'network' | 'rating' | 'mood'; title: string; count: number; images: Img[]};
export type LetterIndex = {letter: string; offset: number}[];

// ── Search ────────────────────────────────────────────────────────────
export type SearchGroupId = 'movies' | 'shows' | 'episodes' | 'people' | 'artists' | 'albums' | 'songs' | 'books' | 'live';
export type SearchResults = {query: string; groups: {id: SearchGroupId; title: string; entries: Entry[]; total: number}[]};

// ── Saved ─────────────────────────────────────────────────────────────
export type SavedTab = 'watchlist' | 'favorites' | 'playlists' | 'collections' | 'views' | 'history';
export type HistoryEntry = {entry: Entry; watchedAt: number};
export type SavedResource = {id: string; kind: 'playlist' | 'collection' | 'view'; title: string; count: number; image?: Img; images?: Img[]; shared: boolean; description?: string; libraryId?: string; smart?: boolean};

// ── Live TV and DVR ───────────────────────────────────────────────────
export type Channel = {id: string; number: string; name: string; logo?: Img; group: string; source: string; hd: boolean; favorite: boolean; hidden: boolean; library?: boolean; description?: string};
export type Program = {id: string; channelId: string; title: string; episodeTitle?: string; start: number; end: number; category: string; overview?: string; isNew?: boolean; isLive?: boolean; premiere?: boolean; season?: number; episode?: number; rating?: string; image?: Img; seriesId?: string; recording?: 'scheduled' | 'recording' | 'series'; reminder?: boolean};
export type ChannelSource = {id: string; name: string; kind: 'tuner' | 'm3u' | 'library'; channelCount: number};
export type RecordingState = 'scheduled' | 'conflicted' | 'recording' | 'completed' | 'incomplete' | 'failed' | 'cancelled';
export type Recording = {id: string; programId?: string; title: string; episodeTitle?: string; channelId: string; channelName: string; start: number; end: number; state: RecordingState; size?: string; image?: Img; seriesRuleId?: string; kept?: boolean; watched?: boolean; progress?: number; reason?: string};
export type SeriesRule = {id: string; title: string; channel: 'this' | 'any'; channelName?: string; newOnly: boolean; keep: number | 'all'; startEarly: number; endLate: number; paused: boolean; upcoming: number; recorded: number; priority: number; image?: Img};
export type DvrStorage = {usedGB: number; freeGB: number; totalGB: number; hoursLeft: number; retentionDays: number; pendingDeleteGB: number; writable: boolean};

// ── Together ──────────────────────────────────────────────────────────
export type GroupMember = {id: string; name: string; art: string; host: boolean; you: boolean; status: 'inStep' | 'buffering' | 'catchingUp' | 'away'};
export type WatchGroup = {id: string; name: string; code: string; state: 'lobby' | 'playing' | 'paused' | 'ended'; control: 'host-only' | 'anyone'; members: GroupMember[]; nowPlaying?: Entry; position?: number; queue: Entry[]; codeExpires: number};

// ── Downloads ─────────────────────────────────────────────────────────
export type DownloadState = 'queued' | 'preparing' | 'downloading' | 'ready' | 'paused' | 'failed' | 'expired';
export type Download = {id: string; entry: Entry; quality: string; size: string; state: DownloadState; progress: number; expires?: string; watched?: boolean};
export type DownloadRequest = {id: string; show: Entry; policy: 'all' | 'unwatched' | 'next'; keep: number; ready: number; total: number; state: 'preparing' | 'active' | 'failed'};
export type DownloadSettings = {wifiOnly: boolean; removeAfterWatching: boolean; nextEpisode: boolean; limitGB: number | null; quality: string};

// ── Accounts, servers, profiles ───────────────────────────────────────
export type ServerStatus = 'online' | 'offline' | 'unreachable';
export type Server = {id: string; name: string; host: string; status: ServerStatus; owner: string; version: string; lastSeen: string; yours: boolean; libraries: number; direct?: boolean};
export type Role = 'owner' | 'admin' | 'member';
export type Profile = {id: string; name: string; art: string; pin: boolean; kids: boolean; role: Role; main?: boolean; limits?: {rating: string; unrated: boolean; downloads: boolean; liveTv: boolean; dvr: boolean; together: boolean}};
export type Account = {id: string; username: string; displayName: string; email: string; kind: 'hosted' | 'local'; twoStep: boolean; recoveryCodesLeft: number; providers: ('google' | 'apple')[]};
export type Session = {server: Server; profile: Profile; account: Account};
export type Device = {id: string; name: string; platform: 'web' | 'ios' | 'tvos' | 'android' | 'androidtv'; app: string; lastSeen: string; firstSeen: string; current: boolean; trusted: boolean; state: 'approved' | 'waiting' | 'blocked'; location?: string};
export type Invitation = {id: string; server: string; from: string; libraries: number; expires: string; state: 'waiting' | 'accepted' | 'declined' | 'expired'};
export type Notice = {id: string; title: string; body: string; at: number; read: boolean; archived: boolean; audience: 'profile' | 'admin'; kind: 'feedback' | 'download' | 'server' | 'security' | 'recording' | 'broadcast'; importance?: 'info' | 'notice' | 'urgent'};

// ── Preferences ───────────────────────────────────────────────────────
export type PrefValue = boolean | number | string | string[];
export type PrefDef = {key: string; group: string; label: string; help?: string; type: 'toggle' | 'choice' | 'number' | 'languages'; options?: {value: string | number; label: string}[]; min?: number; max?: number; step?: number; unit?: string; scope: 'profile' | 'device'};
