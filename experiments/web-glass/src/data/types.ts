// Portico domain model. Everything the UI renders comes through these types,
// so a real backend only has to satisfy the PorticoApi contract in services/api.ts.

export type LibraryKind = 'movies' | 'shows' | 'anime' | 'music' | 'audiobooks' | 'photos' | 'homevideos';

export type ItemKind =
  | 'movie'
  | 'show'
  | 'episode'
  | 'artist'
  | 'album'
  | 'track'
  | 'playlist'
  | 'audiobook'
  | 'photo'
  | 'homevideo'
  | 'channel'
  | 'program'
  | 'recording'
  | 'collection'
  | 'person';

export type ContentRating = 'G' | 'PG' | 'PG-13' | 'R' | 'NR' | 'TV-Y' | 'TV-Y7' | 'TV-G' | 'TV-PG' | 'TV-14' | 'TV-MA';

export type Quality = '4k-dv' | '4k-hdr' | '1080p' | '720p' | 'sd';

export interface Artwork {
  poster?: string;
  backdrop?: string;
  logo?: string;
  thumb?: string;
  square?: string;
}

export interface Palette {
  /** dominant color, used to tint detail pages and now-playing */
  base: string;
  /** a lighter accent drawn from the art */
  accent: string;
  /** true when the art is light enough that overlaid text needs a scrim */
  light?: boolean;
}

export interface Source {
  kind: 'blender' | 'public-domain' | 'generated';
  credit: string;
  license: string;
  url?: string;
}

export interface AudioTrack {
  id: string;
  language: string;
  label: string;
  codec: string;
  channels: string;
  isDefault?: boolean;
  commentary?: boolean;
}

export interface SubtitleTrack {
  id: string;
  language: string;
  label: string;
  format: 'SRT' | 'PGS' | 'ASS' | 'VTT' | 'CC';
  forced?: boolean;
  sdh?: boolean;
  isDefault?: boolean;
}

export interface MediaVersion {
  id: string;
  label: string;
  resolution: '2160p' | '1080p' | '720p' | '480p';
  videoCodec: 'HEVC' | 'H.264' | 'AV1' | 'VP9';
  hdr?: 'Dolby Vision' | 'HDR10' | 'HDR10+' | 'HLG';
  bitrateKbps: number;
  sizeBytes: number;
  container: 'MKV' | 'MP4' | 'M2TS' | 'WEBM';
  path: string;
  audio: AudioTrack[];
  subtitles: SubtitleTrack[];
}

export interface Chapter {
  title: string;
  start: number; // seconds
}

export interface CastCredit {
  personId: string;
  name: string;
  role: string;
}

export interface CrewCredit {
  personId: string;
  name: string;
  job: string;
}

export interface Extra {
  id: string;
  title: string;
  kind: 'Trailer' | 'Featurette' | 'Behind the Scenes' | 'Deleted Scene' | 'Interview';
  runtime: number; // seconds
  thumb: string;
}

interface BaseItem {
  id: string;
  kind: ItemKind;
  title: string;
  sortTitle?: string;
  libraryId?: string;
  art: Artwork;
  palette: Palette;
  addedAt: string; // ISO
}

export interface Movie extends BaseItem {
  kind: 'movie';
  year: number;
  released: string;
  runtime: number; // minutes
  contentRating: ContentRating;
  genres: string[];
  tagline?: string;
  summary: string;
  studio: string;
  country: string;
  originalLanguage: string;
  ratings: { critic?: number; audience?: number };
  cast: CastCredit[];
  crew: CrewCredit[];
  versions: MediaVersion[];
  chapters: Chapter[];
  extras: Extra[];
  collectionIds: string[];
  source: Source;
  streamId: StreamId;
}

export interface Episode extends BaseItem {
  kind: 'episode';
  showId: string;
  showTitle: string;
  season: number;
  number: number;
  summary: string;
  runtime: number; // minutes
  airDate: string;
  contentRating: ContentRating;
  versions: MediaVersion[];
  chapters: Chapter[];
  introEnd?: number; // seconds, for Skip Intro
  creditsStart?: number; // seconds, for Up Next
  streamId: StreamId;
}

export interface Season {
  id: string;
  showId: string;
  number: number;
  title: string;
  year: number;
  summary?: string;
  poster?: string;
  episodes: Episode[];
}

export interface Show extends BaseItem {
  kind: 'show';
  year: number;
  endYear?: number;
  status: 'Continuing' | 'Ended' | 'Limited Series';
  network: string;
  contentRating: ContentRating;
  genres: string[];
  tagline?: string;
  summary: string;
  ratings: { critic?: number; audience?: number };
  cast: CastCredit[];
  crew: CrewCredit[];
  seasons: Season[];
  isAnime?: boolean;
  originalTitle?: string;
  airs?: string; // e.g. "New episodes Saturdays"
  source: Source;
}

export interface Track {
  id: string;
  kind: 'track';
  title: string;
  albumId: string;
  albumTitle: string;
  artistId: string;
  artistName: string;
  disc: number;
  number: number;
  duration: number; // seconds
  explicit?: boolean;
  plays: number;
  loved?: boolean;
  codec: 'FLAC' | 'ALAC' | 'AAC' | 'MP3' | 'Opus';
  bitDepth?: number;
  sampleRate?: number;
  bitrateKbps: number;
  lyrics?: LyricLine[];
  art: Artwork;
  palette: Palette;
  streamId: StreamId;
}

export interface LyricLine {
  t: number; // seconds
  text: string;
}

export interface Album extends BaseItem {
  kind: 'album';
  artistId: string;
  artistName: string;
  year: number;
  genre: string;
  label: string;
  format: string; // "Album", "EP", "Single"
  tracks: Track[];
  review?: string;
  source: Source;
}

export interface Artist extends BaseItem {
  kind: 'artist';
  genres: string[];
  bio: string;
  from: string;
  albumIds: string[];
  monthlyPlays: number;
  similar: string[];
}

export interface Playlist extends BaseItem {
  kind: 'playlist';
  description: string;
  ownerId: string;
  smart?: { rule: string };
  trackIds: string[];
  updatedAt: string;
}

export interface AudiobookChapter {
  title: string;
  start: number; // seconds
  duration: number;
}

export interface Audiobook extends BaseItem {
  kind: 'audiobook';
  author: string;
  narrator: string;
  year: number; // original publication
  published: string; // edition date
  duration: number; // seconds
  genres: string[];
  summary: string;
  series?: { name: string; index: number };
  chapters: AudiobookChapter[];
  publisher: string;
  source: Source;
  streamId: StreamId;
}

export interface Photo {
  id: string;
  kind: 'photo';
  title: string;
  src: string;
  thumb: string;
  width: number;
  height: number;
  takenAt: string;
  albumIds: string[];
  place?: string;
  camera?: string;
  lens?: string;
  exposure?: string; // "1/250s · f/2.8 · ISO 200"
  favorite?: boolean;
  video?: boolean;
  palette: Palette;
}

export interface PhotoAlbum {
  id: string;
  title: string;
  subtitle: string;
  cover: string;
  photoIds: string[];
  kind: 'trip' | 'event' | 'place' | 'smart';
}

export interface Memory {
  id: string;
  title: string;
  subtitle: string;
  cover: string;
  photoIds: string[];
}

export interface HomeVideo extends BaseItem {
  kind: 'homevideo';
  recordedAt: string;
  duration: number; // seconds
  place?: string;
  device: string;
  summary: string;
  people: string[];
  streamId: StreamId;
}

export interface Collection extends BaseItem {
  kind: 'collection';
  summary: string;
  itemIds: string[];
}

export interface Person {
  id: string;
  kind: 'person';
  name: string;
  knownFor: string;
  bio: string;
  born?: string;
  birthplace?: string;
  photo?: string;
}

// ---------- Live TV ----------

export interface Channel {
  id: string;
  number: string;
  name: string;
  callSign: string;
  logoColor: string;
  logoText: string;
  source: 'Antenna' | 'IPTV' | 'HDHomeRun';
  hd: boolean;
  favorite?: boolean;
  category: string;
  streamId: StreamId;
}

export interface Program {
  id: string;
  channelId: string;
  title: string;
  episodeTitle?: string;
  summary: string;
  start: string;
  end: string;
  category: 'Movie' | 'Series' | 'News' | 'Sports' | 'Kids' | 'Documentary' | 'Talk' | 'Food' | 'Music';
  isNew?: boolean;
  isLive?: boolean;
  rating?: ContentRating;
  image?: string;
  seriesId?: string;
}

export interface Recording {
  id: string;
  programId?: string;
  title: string;
  episodeTitle?: string;
  channelName: string;
  recordedAt: string;
  duration: number; // seconds
  sizeBytes: number;
  image: string;
  status: 'completed' | 'recording' | 'failed' | 'partial';
  watched?: boolean;
  progress?: number;
  failureReason?: string;
}

export interface ScheduledRecording {
  id: string;
  programId: string;
  title: string;
  episodeTitle?: string;
  channelId: string;
  channelName: string;
  start: string;
  end: string;
  ruleId?: string;
  conflict?: string;
}

export interface SeriesRule {
  id: string;
  title: string;
  channels: 'any' | string[];
  newOnly: boolean;
  keep: 'all' | number;
  paddingBefore: number; // minutes
  paddingAfter: number;
  priority: number;
  image: string;
  upcoming: number;
}

// ---------- Streams (the fixture media we actually play) ----------

export type StreamId = 'coffee-run' | 'sintel-trailer' | 'ambient-a' | 'ambient-b' | 'ambient-c' | 'ambient-d' | 'spoken';

// ---------- Household, profiles, security ----------

export type Role = 'owner' | 'admin' | 'member' | 'kid' | 'guest';

export interface Profile {
  id: string;
  name: string;
  role: Role;
  avatarColor: string;
  avatarImage?: string;
  initials: string;
  hasPin: boolean;
  maxRating: ContentRating;
  maxTvRating: ContentRating;
  libraries: 'all' | string[];
  email?: string;
  language: string;
  subtitleMode: 'off' | 'auto' | 'always' | 'forced';
  subtitleLanguage: string;
  audioLanguage: string;
  autoplayNext: boolean;
  autoplayPreviews: boolean;
  streamingQuality: 'original' | 'auto' | '1080p' | '720p' | '480p';
  cellularQuality: 'auto' | '720p' | '480p';
  restrictions?: { bedtime?: string; dailyLimitMin?: number; blockedGenres?: string[] };
  createdAt: string;
  lastActive: string;
}

export interface Device {
  id: string;
  name: string;
  platform: string;
  app: string;
  location: string;
  ip: string;
  lastSeen: string;
  current?: boolean;
  profileId: string;
  kind: 'tv' | 'phone' | 'tablet' | 'desktop' | 'browser' | 'speaker' | 'console';
}

export interface CastTarget {
  id: string;
  name: string;
  kind: 'tv' | 'speaker' | 'group';
  protocol: 'Portico' | 'AirPlay' | 'Google Cast' | 'DLNA';
  room: string;
}

export interface Notification {
  id: string;
  kind: 'new-episode' | 'recording' | 'download' | 'share' | 'server' | 'security' | 'request';
  title: string;
  body: string;
  at: string;
  read?: boolean;
  itemId?: string;
  href?: string;
  image?: string;
  adminOnly?: boolean;
}

export interface Download {
  id: string;
  itemId: string;
  itemKind: ItemKind;
  title: string;
  subtitle: string;
  image: string;
  quality: 'Original' | 'High (1080p)' | 'Medium (720p)' | 'Low (480p)';
  sizeBytes: number;
  progress: number; // 0..1
  status: 'queued' | 'downloading' | 'paused' | 'complete' | 'error' | 'expired';
  device: string;
  expiresAt?: string;
  error?: string;
  smart?: boolean;
}

export interface ProgressEntry {
  itemId: string;
  position: number; // seconds
  duration: number; // seconds
  updatedAt: string;
}

export interface HistoryEntry {
  id: string;
  itemId: string;
  itemKind: ItemKind;
  title: string;
  subtitle: string;
  image: string;
  at: string;
  device: string;
  profileId: string;
  completed: boolean;
}

// ---------- Home feed ----------

export interface Shelf {
  id: string;
  title: string;
  subtitle?: string;
  style: 'poster' | 'landscape' | 'square' | 'circle' | 'hero-cards' | 'continue' | 'live' | 'top10';
  items: ShelfItem[];
  href?: string;
}

export interface ShelfItem {
  id: string;
  kind: ItemKind;
  title: string;
  subtitle?: string;
  image: string;
  href: string;
  progress?: number;
  badge?: string;
  palette?: Palette;
  logo?: string;
  meta?: string;
}

export interface HomeFeed {
  hero: HeroItem[];
  shelves: Shelf[];
}

export interface HeroItem {
  id: string;
  kind: 'movie' | 'show';
  title: string;
  logo?: string;
  backdrop: string;
  eyebrow: string;
  summary: string;
  meta: string[];
  badges: string[];
  href: string;
  playHref: string;
  progress?: number;
  resumeLabel?: string;
  palette: Palette;
}

// ---------- Search ----------

export interface SearchHit {
  id: string;
  kind: ItemKind;
  title: string;
  subtitle: string;
  image?: string;
  href: string;
  shape: 'poster' | 'landscape' | 'square' | 'circle';
  score: number;
}

export interface SearchResults {
  query: string;
  top?: SearchHit;
  groups: { kind: string; label: string; hits: SearchHit[] }[];
  total: number;
  tookMs: number;
}

// ---------- Paging ----------

export interface Page<T> {
  items: T[];
  total: number;
  offset: number;
}

export interface LibraryQuery {
  sort?: 'title' | 'added' | 'year' | 'rating' | 'runtime' | 'plays' | 'artist';
  dir?: 'asc' | 'desc';
  genre?: string;
  filter?: 'all' | 'unwatched' | 'in-progress' | 'watched' | '4k' | 'hdr' | 'downloaded';
  decade?: string;
  offset?: number;
  limit?: number;
  view?: string;
}

export interface LibraryCard {
  id: string;
  kind: ItemKind;
  title: string;
  subtitle: string;
  image: string;
  href: string;
  year?: number;
  progress?: number;
  watched?: boolean;
  unwatchedCount?: number;
  badge?: string;
  palette?: Palette;
  sortKey: string;
}
