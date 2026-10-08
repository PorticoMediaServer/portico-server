/**
 * Domain model for the Portico client. Shapes follow the real service contracts closely enough
 * that a Portico client-core adapter can fill them (see README › Swapping in real services).
 * Artwork is referenced by an `ArtRef` key; the fixture layer resolves keys to bundled images,
 * a real adapter resolves them to server image URLs.
 */

export type ArtRef = string; // e.g. "poster/m-charade", resolved by data/art.ts

export type LibraryKind = 'movies' | 'shows' | 'anime' | 'music' | 'audiobooks' | 'photos' | 'homevideos';

export type ItemKind =
  | 'movie' | 'show' | 'season' | 'episode'
  | 'artist' | 'album' | 'track'
  | 'audiobook' | 'photoAlbum' | 'photo' | 'homevideo'
  | 'person' | 'collection' | 'playlist' | 'channel' | 'recording';

export type Shape = 'poster' | 'landscape' | 'square' | 'circle';

export type UserState = {
  watched: boolean;
  /** 0..1 progress when partly watched. */
  progress: number;
  /** Seconds position when in progress. */
  position: number;
  watchlist: boolean;
  favorite: boolean;
  /** 0 = unrated, 1..5 stars (halves allowed). */
  rating: number;
  liked: 'like' | 'dislike' | null;
  notInterested: boolean;
  lastPlayedAt: number | null;
};

export type Credit = {personId: string; name: string; character?: string; job?: string; department: 'Acting' | 'Directing' | 'Writing' | 'Production' | 'Sound'};

export type MediaVersion = {id: string; label: string; resolution: '4K' | '1080p' | '720p' | '480p' | 'SD'; hdr: 'Dolby Vision' | 'HDR10' | null; container: string; video: string; audio: string; sizeGB: number; bitrateMbps: number; edition?: string};
export type Track = {id: string; label: string; language: string; codec?: string; channels?: string; forced?: boolean; origin?: 'embedded' | 'sidecar' | 'provider'};
export type Chapter = {title: string; start: number; art?: ArtRef};
export type Extra = {id: string; title: string; kind: 'Trailer' | 'Featurette' | 'Behind the Scenes' | 'Deleted Scene' | 'Interview'; duration: number; art: ArtRef};

export type ItemBase = {
  id: string;
  kind: ItemKind;
  title: string;
  sortTitle: string;
  libraryId: string;
  year?: number;
  art: {poster?: ArtRef; backdrop?: ArtRef; still?: ArtRef; square?: ArtRef; thumb?: ArtRef};
  /** Dominant artwork colour, used to tint the glow when this item has focus. */
  tint: string;
  genres: string[];
  synopsis?: string;
  addedAt: number;
};

export type Movie = ItemBase & {
  kind: 'movie' | 'homevideo';
  tagline?: string;
  contentRating: string;
  runtime: number; // minutes
  studio?: string;
  country?: string;
  language?: string;
  mood?: string;
  criticScore?: number;
  audienceScore?: number;
  credits: Credit[];
  versions: MediaVersion[];
  audioTracks: Track[];
  subtitleTracks: Track[];
  chapters: Chapter[];
  extras: Extra[];
  franchise?: string | null;
  license?: string;
  real?: boolean;
  place?: string;
  date?: string;
};

export type Show = ItemBase & {
  kind: 'show';
  anime: boolean;
  contentRating: string;
  network: string;
  status: 'Continuing' | 'Ended';
  yearEnd?: number | null;
  credits: Credit[];
  seasonIds: string[];
  episodeCount: number;
};

export type Season = ItemBase & {kind: 'season'; showId: string; number: number; episodeIds: string[]};

export type Episode = ItemBase & {
  kind: 'episode';
  showId: string;
  seasonId: string;
  seasonNumber: number;
  number: number;
  runtime: number;
  airDate: string;
  credits: Credit[];
  versions: MediaVersion[];
  audioTracks: Track[];
  subtitleTracks: Track[];
  chapters: Chapter[];
  introEnd?: number;
  creditsStart?: number;
};

export type Artist = ItemBase & {kind: 'artist'; bio: string; origin: string; formed: number; band: boolean; albumIds: string[]};
export type Album = ItemBase & {kind: 'album'; artistId: string; artistName: string; label: string; albumType: string; trackIds: string[]; discs: number; duration: number};
export type TrackItem = ItemBase & {kind: 'track'; albumId: string; artistId: string; artistName: string; albumTitle: string; number: number; disc: number; duration: number; hasLyrics: boolean};
export type Audiobook = ItemBase & {kind: 'audiobook'; author: string; authorId: string; authorBio: string; narrator: string; series?: {name: string; number: number} | null; chapters: {title: string; start: number; duration: number}[]; duration: number; publicDomain: boolean};
export type PhotoAlbum = ItemBase & {kind: 'photoAlbum'; place: string; dateStart: string; dateEnd: string; photoIds: string[]};
export type Photo = ItemBase & {kind: 'photo'; albumId: string; caption: string; taken: string; place: string; width: number; height: number};

export type Person = {id: string; kind: 'person'; name: string; art: {headshot?: ArtRef}; tint: string; bio: string; born?: string | null; died?: string | null; birthplace?: string; knownFor: string};

export type Item = Movie | Show | Season | Episode | Artist | Album | TrackItem | Audiobook | PhotoAlbum | Photo;

export type Library = {
  id: string;
  name: string;
  kind: LibraryKind;
  count: number;
  scanning: boolean;
  views: LibraryView[];
  sourcePaths: string[];
};
export type LibraryView = 'discover' | 'browse' | 'collections' | 'categories' | 'songs' | 'releases' | 'authors' | 'series' | 'albums' | 'timeline';

export type RowKind =
  | 'continue' | 'upNext' | 'recent' | 'recentIn' | 'picks' | 'because' | 'moreLike' | 'starring' | 'directedBy'
  | 'genre' | 'mood' | 'hiddenGems' | 'topRated' | 'rediscover' | 'trending' | 'popular' | 'alsoWatched' | 'newForYou'
  | 'watchlist' | 'favorites' | 'savedView' | 'featured' | 'continueListening' | 'collection' | 'custom';

export type Row = {
  id: string;
  kind: RowKind;
  title: string;
  /** For editorial rows like "Because you watched", the highlighted subject. */
  subject?: string;
  itemIds: string[];
  shape: Shape;
  recommendation?: boolean;
  total?: number;
};

export type HomeLayoutEntry = {rowId: string; visible: boolean; alwaysOn?: boolean};

export type Collection = {id: string; kind: 'collection'; title: string; description: string; itemIds: string[]; shared: boolean; owner: 'server' | 'me'; libraryId?: string; art?: ArtRef; tint: string};
export type Playlist = {id: string; kind: 'playlist'; title: string; description: string; itemIds: string[]; shared: boolean; media: 'music' | 'video' | 'mixed'; updatedAt: number; tint: string};
export type SavedView = {id: string; title: string; libraryId: string; filters: FilterState; sort: SortState; shared: boolean; onHome: boolean};

export type SortState = {field: string; direction: 'asc' | 'desc'};
export type FilterState = {quick: string[]; fields: Record<string, string[]>; yearFrom?: number; yearTo?: number};

export type Profile = {id: string; name: string; color: string; initials: string; kids: boolean; pin: string | null; main: boolean; picture?: ArtRef; limits: ProfileLimits};
export type ProfileLimits = {maxRating: string | null; allowUnrated: boolean; downloads: boolean; liveTv: boolean; dvr: boolean; together: boolean; hiddenLabels: string[]};

export type ServerStatus = 'online' | 'offline' | 'unreachable';
export type Server = {id: string; name: string; host: string; version: string; status: ServerStatus; owner: boolean; role: 'owner' | 'admin' | 'member'; via: 'account' | 'direct'; lastSeen: number; libraries: number};

export type Account = {id: string; name: string; username: string; email: string; hosted: boolean; twoStep: boolean; recoveryCodesLeft: number; providers: {apple: boolean; google: boolean}; hasPassword: boolean};

export type DeviceSession = {id: string; name: string; platform: 'web' | 'ios' | 'tvos' | 'android' | 'androidtv'; app: string; signedInAt: number; lastUsed: number; current: boolean; trusted: boolean; waiting: boolean; location?: string};

export type NotificationItem = {id: string; title: string; body: string; at: number; read: boolean; archived: boolean; audience: 'profile' | 'admin'; kind: 'info' | 'notice' | 'urgent' | 'security' | 'server'};

export type Invitation = {id: string; server: string; inviter: string; libraries: number; expires: number; state: 'waiting' | 'accepted' | 'declined' | 'expired' | 'revoked'};

// ---- Live TV

export type Channel = {id: string; number: string; name: string; callsign: string; group: string; source: string; logo: ArtRef; tint: string; favorite: boolean; hidden: boolean; library: boolean};
export type Programme = {id: string; channelId: string; title: string; episodeTitle?: string | null; synopsis: string; start: number; end: number; genre: string; rating: string; series: boolean; isNew: boolean; art?: ArtRef};
export type RecordingState = 'scheduled' | 'conflicted' | 'recording' | 'completed' | 'incomplete' | 'failed' | 'cancelled';
export type Recording = {id: string; programmeId?: string; channelId: string; title: string; episodeTitle?: string | null; start: number; end: number; state: RecordingState; kept: boolean; sizeGB: number; ruleId?: string; watched: boolean; reason?: string};
export type RecordingRule = {id: string; title: string; channelId: string | null; newOnly: boolean; keep: number | 'all'; startEarly: number; endLate: number; paused: boolean; priority: number; scheduledCount: number};
export type Reminder = {programmeId: string};

// ---- Downloads

export type DownloadState = 'queued' | 'downloading' | 'paused' | 'done' | 'failed' | 'preparing' | 'ready' | 'expired';
export type Download = {id: string; itemId: string; quality: string; sizeMB: number; progress: number; state: DownloadState; where: 'device' | 'server'; expires?: number};
export type DownloadRequest = {id: string; showId: string; policy: 'all' | 'unwatched' | 'next'; keepNext: number; ready: number; total: number; failed: number};
export type DownloadSettings = {wifiOnly: boolean; removeAfterWatching: boolean; nextEpisode: boolean; limitGB: number | null; quality: string};

// ---- Together

export type TogetherMember = {id: string; name: string; color: string; status: 'inStep' | 'buffering' | 'catchingUp' | 'away'; host: boolean; you: boolean; ready: boolean};
export type TogetherGroup = {id: string; name: string; code: string; state: 'lobby' | 'playing' | 'hostReconnecting' | 'ended'; control: 'host' | 'everyone'; members: TogetherMember[]; queue: string[]; nowPlaying: string | null; position: number};

// ---- Search

export type SearchGroup = {kind: string; label: string; ids: string[]; total: number};

// ---- Feedback

export type FeedbackReport = {id: string; from: string; category: string; message: string; itemId?: string; at: number; status: 'new' | 'looking' | 'fixed' | 'wontFix'; reply?: string};
