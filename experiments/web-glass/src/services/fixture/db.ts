// In-memory "server database" for the fixture backend, persisted to localStorage
// so that watch state, downloads and settings survive reloads like a real server would.
import type {
  Album,
  Audiobook,
  ContentRating,
  Download,
  Episode,
  HistoryEntry,
  HomeVideo,
  LibraryCard,
  Movie,
  Notification,
  Profile,
  ProgressEntry,
  ScheduledRecording,
  Show,
  ShelfItem,
} from '../../data/types';
import {
  albums,
  artists,
  audiobooks,
  collections,
  episodes,
  homeVideos,
  movies,
  playlists,
  shows,
  tracks,
} from '../../data/fixtures/index';
import {
  downloadsSeed,
  historySeed,
  notificationsSeed,
  profiles as profileSeed,
  progressSeed,
  ratingsSeed,
  watchedSeed,
  watchlistSeed,
} from '../../data/fixtures/household';
import { initialScheduled } from '../../data/fixtures/live';
import { sessionsSeed, tasksSeed, usersSeed, type Session, type Task, type UserAdmin } from '../../data/fixtures/server';

const KEY = 'portico.fixture.v3';

interface UserState {
  progress: Record<string, ProgressEntry>;
  watched: string[];
  watchlist: string[];
  ratings: Record<string, number>;
  history: HistoryEntry[];
  recentSearches: string[];
}

interface Db {
  profiles: Profile[];
  users: Record<string, UserState>;
  downloads: Download[];
  notificationsRead: string[];
  notificationsDismissed: string[];
  scheduled: ScheduledRecording[];
  deletedRecordings: string[];
  sessions: Session[];
  tasks: Task[];
  usersAdmin: UserAdmin[];
  admin: Record<string, Record<string, unknown>>;
  revokedDevices: string[];
}

function seedUser(id: string): UserState {
  return {
    progress: Object.fromEntries((progressSeed[id] ?? []).map((p) => [p.itemId, p])),
    watched: watchedSeed[id] ?? [],
    watchlist: watchlistSeed[id] ?? [],
    ratings: ratingsSeed[id] ?? {},
    history: id === 'maya' ? historySeed() : [],
    recentSearches: id === 'maya' ? ['meridian', 'Cary Grant', 'lo-fi', 'iceland'] : [],
  };
}

function fresh(): Db {
  return {
    profiles: profileSeed.map((p) => ({ ...p })),
    users: Object.fromEntries(profileSeed.map((p) => [p.id, seedUser(p.id)])),
    downloads: downloadsSeed.map((d) => ({ ...d })),
    notificationsRead: notificationsSeed.filter((n) => n.read).map((n) => n.id),
    notificationsDismissed: [],
    scheduled: initialScheduled(),
    deletedRecordings: [],
    sessions: sessionsSeed.map((s) => ({ ...s })),
    tasks: tasksSeed.map((t) => ({ ...t })),
    usersAdmin: usersSeed.map((u) => ({ ...u })),
    admin: {},
    revokedDevices: [],
  };
}

function load(): Db {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<Db>;
      const base = fresh();
      // Time-anchored data (guide, sessions) is always regenerated.
      return { ...base, ...parsed, scheduled: base.scheduled, sessions: base.sessions, tasks: base.tasks };
    }
  } catch {
    /* storage unavailable or corrupt: start fresh */
  }
  return fresh();
}

export const db: Db = load();

let saveTimer: ReturnType<typeof setTimeout> | undefined;
export function persist() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    try {
      const { profiles, users, downloads, notificationsRead, notificationsDismissed, deletedRecordings, usersAdmin, admin, revokedDevices } = db;
      localStorage.setItem(KEY, JSON.stringify({ profiles, users, downloads, notificationsRead, notificationsDismissed, deletedRecordings, usersAdmin, admin, revokedDevices }));
    } catch {
      /* quota or private mode: state stays in memory */
    }
  }, 250);
}

export function resetDb() {
  try {
    localStorage.removeItem(KEY);
  } catch {
    /* ignore */
  }
}

// ---------- current profile ----------
let currentProfileId = 'maya';
export const setCurrentProfile = (id: string) => {
  currentProfileId = id;
  if (!db.users[id]) db.users[id] = seedUser(id);
};
export const profile = (): Profile => db.profiles.find((p) => p.id === currentProfileId) ?? db.profiles[0];
export const user = (): UserState => {
  const p = profile();
  return (db.users[p.id] ??= seedUser(p.id));
};

// ---------- lookups ----------
export const byId = {
  movie: new Map(movies.map((m) => [m.id, m])),
  show: new Map(shows.map((s) => [s.id, s])),
  episode: new Map(episodes.map((e) => [e.id, e])),
  album: new Map(albums.map((a) => [a.id, a])),
  artist: new Map(artists.map((a) => [a.id, a])),
  track: new Map(tracks.map((t) => [t.id, t])),
  playlist: new Map(playlists.map((p) => [p.id, p])),
  audiobook: new Map(audiobooks.map((b) => [b.id, b])),
  homevideo: new Map(homeVideos.map((v) => [v.id, v])),
  collection: new Map(collections.map((c) => [c.id, c])),
};

export function kindOf(id: string): string | undefined {
  for (const [k, m] of Object.entries(byId)) if (m.has(id)) return k;
  return undefined;
}

// ---------- parental rules ----------
const MOVIE_ORDER: ContentRating[] = ['G', 'PG', 'PG-13', 'R', 'NR'];
const TV_ORDER: ContentRating[] = ['TV-Y', 'TV-Y7', 'TV-G', 'TV-PG', 'TV-14', 'TV-MA'];

export function allowedRating(r: ContentRating, p = profile()): boolean {
  if (p.role === 'owner' || p.role === 'admin') return true;
  if (MOVIE_ORDER.includes(r)) {
    const max = MOVIE_ORDER.indexOf(p.maxRating);
    const idx = MOVIE_ORDER.indexOf(r);
    if (r === 'NR') return p.maxRating === 'R' || p.maxRating === 'NR';
    return idx <= max;
  }
  return TV_ORDER.indexOf(r) <= TV_ORDER.indexOf(p.maxTvRating);
}

export function allowedLibrary(libraryId: string | undefined, p = profile()): boolean {
  if (!libraryId) return true;
  return p.libraries === 'all' || p.libraries.includes(libraryId);
}

export function allowedGenres(genres: string[], p = profile()) {
  const blocked = p.restrictions?.blockedGenres ?? [];
  return !genres.some((g) => blocked.includes(g));
}

export const visibleMovies = () => movies.filter((m) => allowedLibrary(m.libraryId) && allowedRating(m.contentRating) && allowedGenres(m.genres));
export const visibleShows = (anime?: boolean) =>
  shows.filter((s) => (anime === undefined || !!s.isAnime === anime) && allowedLibrary(s.libraryId) && allowedRating(s.contentRating) && allowedGenres(s.genres));
export const isKid = () => profile().role === 'kid';

// ---------- state helpers ----------
export const progressOf = (id: string) => {
  const p = user().progress[id];
  return p ? { position: p.position, duration: p.duration } : undefined;
};
export const isWatched = (id: string) => user().watched.includes(id);
export const inWatchlist = (id: string) => user().watchlist.includes(id);
export const downloadFor = (id: string) => db.downloads.find((d) => d.itemId === id && d.status !== 'expired');

export const fmtRuntime = (min: number) => (min >= 60 ? `${Math.floor(min / 60)} hr ${min % 60 ? `${min % 60} min` : ''}`.trim() : `${min} min`);
export const fmtClock = (secs: number) => {
  const s = Math.max(0, Math.round(secs));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const x = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(x).padStart(2, '0')}` : `${m}:${String(x).padStart(2, '0')}`;
};
export const remainingLabel = (pos: number, dur: number) => {
  const left = Math.max(0, Math.round((dur - pos) / 60));
  return left >= 60 ? `${Math.floor(left / 60)} hr ${left % 60} min left` : `${left} min left`;
};

export function showEpisodes(s: Show): Episode[] {
  return s.seasons.flatMap((x) => x.episodes);
}

/** The next episode to watch for a show: resume an in-progress one, else the first unwatched after the last watched. */
export function nextUpFor(s: Show): { episode: Episode; position?: number; label: string } | undefined {
  const eps = showEpisodes(s);
  const inProg = eps
    .map((e) => ({ e, p: user().progress[e.id] }))
    .filter((x) => x.p)
    .sort((a, b) => Date.parse(b.p.updatedAt) - Date.parse(a.p.updatedAt))[0];
  if (inProg) return { episode: inProg.e, position: inProg.p.position, label: `Resume S${inProg.e.season} · E${inProg.e.number}` };
  const lastWatchedIdx = eps.reduce((acc, e, i) => (isWatched(e.id) ? i : acc), -1);
  const next = eps[lastWatchedIdx + 1];
  if (!next) return undefined;
  if (lastWatchedIdx === -1) return { episode: next, label: `Play S${next.season} · E${next.number}` };
  return { episode: next, label: `Next: S${next.season} · E${next.number}` };
}

// ---------- cards ----------
export function movieCard(m: Movie): LibraryCard {
  const p = progressOf(m.id);
  const badge = m.versions[0].resolution === '2160p' ? '4K' : undefined;
  return {
    id: m.id,
    kind: 'movie',
    title: m.title,
    subtitle: String(m.year),
    image: m.art.poster!,
    href: `/movie/${m.id}`,
    year: m.year,
    progress: p ? p.position / p.duration : undefined,
    watched: isWatched(m.id),
    badge,
    palette: m.palette,
    sortKey: m.sortTitle ?? m.title,
  };
}

export function showCard(s: Show): LibraryCard {
  const eps = showEpisodes(s);
  const unwatched = eps.filter((e) => !isWatched(e.id)).length;
  return {
    id: s.id,
    kind: 'show',
    title: s.title,
    subtitle: `${s.seasons.length} season${s.seasons.length > 1 ? 's' : ''}`,
    image: s.art.poster!,
    href: `/show/${s.id}`,
    year: s.year,
    unwatchedCount: unwatched,
    watched: unwatched === 0,
    palette: s.palette,
    sortKey: s.sortTitle ?? s.title,
  };
}

export const episodeLabel = (e: Episode) => `S${e.season} · E${e.number}`;

export function episodeShelfItem(e: Episode, extra?: Partial<ShelfItem>): ShelfItem {
  const p = progressOf(e.id);
  return {
    id: e.id,
    kind: 'episode',
    title: e.showTitle,
    subtitle: `${episodeLabel(e)} · ${e.title}`,
    image: e.art.thumb!,
    href: `/episode/${e.id}`,
    progress: p ? p.position / p.duration : undefined,
    palette: e.palette,
    ...extra,
  };
}

export function albumCard(a: Album): LibraryCard {
  return { id: a.id, kind: 'album', title: a.title, subtitle: a.artistName, image: a.art.square!, href: `/album/${a.id}`, year: a.year, palette: a.palette, sortKey: a.title };
}

export function audiobookCard(b: Audiobook): LibraryCard {
  const p = progressOf(b.id);
  return {
    id: b.id,
    kind: 'audiobook',
    title: b.title,
    subtitle: b.author,
    image: b.art.square!,
    href: `/audiobook/${b.id}`,
    year: b.year,
    progress: p ? p.position / p.duration : undefined,
    watched: isWatched(b.id),
    palette: b.palette,
    sortKey: b.sortTitle ?? b.title,
  };
}

export function homeVideoCard(v: HomeVideo): LibraryCard {
  const p = progressOf(v.id);
  return {
    id: v.id,
    kind: 'homevideo',
    title: v.title,
    subtitle: new Date(v.recordedAt).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' }),
    image: v.art.thumb!,
    href: `/home-video/${v.id}`,
    progress: p ? p.position / p.duration : undefined,
    palette: v.palette,
    sortKey: v.recordedAt,
  };
}

export function cardFor(id: string): LibraryCard | undefined {
  const m = byId.movie.get(id);
  if (m) return movieCard(m);
  const s = byId.show.get(id);
  if (s) return showCard(s);
  const e = byId.episode.get(id);
  if (e) {
    const it = episodeShelfItem(e);
    return { ...it, sortKey: e.title, subtitle: it.subtitle ?? '' };
  }
  const a = byId.album.get(id);
  if (a) return albumCard(a);
  const b = byId.audiobook.get(id);
  if (b) return audiobookCard(b);
  const v = byId.homevideo.get(id);
  if (v) return homeVideoCard(v);
  return undefined;
}

export function notificationsFor(): Notification[] {
  const p = profile();
  const admin = p.role === 'owner' || p.role === 'admin';
  return notificationsSeed
    .filter((n) => (admin || !n.adminOnly) && !db.notificationsDismissed.includes(n.id))
    .filter((n) => (p.role === 'kid' ? n.kind === 'new-episode' && n.href?.includes('little-explorers') : true))
    .map((n) => ({ ...n, read: db.notificationsRead.includes(n.id) }));
}
