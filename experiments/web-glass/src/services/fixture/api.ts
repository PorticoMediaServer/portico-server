import type { PorticoApi, AdminOverview, DvrData, GuideData, PlayableSource, SecurityState, ServerSummary } from '../api';
import type {
  Album,
  Download,
  HeroItem,
  HistoryEntry,
  HomeFeed,
  LibraryCard,
  LibraryQuery,
  Movie,
  Profile,
  SearchHit,
  Shelf,
  ShelfItem,
  Show,
  Track,
} from '../../data/types';
import {
  albums,
  artists,
  audiobooks,
  collections,
  creditsByPerson,
  episodes,
  homeVideos,
  libraries,
  memories,
  people,
  photoAlbums,
  photos,
  playlists,
  shows,
  tracks,
} from '../../data/fixtures/index';
import { castTargets, devices } from '../../data/fixtures/household';
import { GUIDE_HOURS, GUIDE_START, channels, programs, recordings, seriesRules } from '../../data/fixtures/live';
import {
  LOG_COUNT,
  activitySeed,
  backupsSeed,
  diagnosticsSeed,
  liveSourcesSeed,
  logAt,
  metadataSeed,
  metricAt,
  metricHistory,
  networkSeed,
  server,
  settingsSeed,
  storageSeed,
  transcoderSeed,
  type LibraryAdmin,
} from '../../data/fixtures/server';
import { NOW, daysAgo, hoursAgo, minutesAgo, rng } from '../../data/fixtures/util';
import { ApiError, simulate } from '../latency';
import {
  albumCard,
  allowedLibrary,
  allowedRating,
  audiobookCard,
  byId,
  cardFor,
  db,
  downloadFor,
  episodeLabel,
  episodeShelfItem,
  fmtRuntime,
  homeVideoCard,
  inWatchlist,
  isKid,
  isWatched,
  kindOf,
  movieCard,
  nextUpFor,
  notificationsFor,
  persist,
  profile,
  progressOf,
  remainingLabel,
  showCard,
  showEpisodes,
  user,
  visibleMovies,
  visibleShows,
} from './db';

const stateOf = (id: string) => ({
  progress: progressOf(id),
  watched: isWatched(id),
  inWatchlist: inWatchlist(id),
  rating: user().ratings[id],
  downloaded: downloadFor(id),
});

const resLabel = (m: Movie) => {
  const v = m.versions[0];
  const res = v.resolution === '2160p' ? '4K' : v.resolution === '1080p' ? 'HD' : 'SD';
  const out = [res];
  if (v.hdr) out.push(v.hdr === 'Dolby Vision' ? 'Dolby Vision' : 'HDR');
  if (v.audio.some((a) => a.label.includes('Atmos'))) out.push('Dolby Atmos');
  else if (v.audio.some((a) => a.channels === '5.1' || a.channels === '7.1')) out.push('5.1');
  return out;
};

function heroFromMovie(m: Movie, eyebrow: string): HeroItem {
  const p = progressOf(m.id);
  return {
    id: m.id,
    kind: 'movie',
    title: m.title,
    logo: m.art.logo,
    backdrop: m.art.backdrop!,
    eyebrow,
    summary: m.summary,
    meta: [String(m.year), m.contentRating, fmtRuntime(m.runtime), m.genres.slice(0, 2).join(' · ')],
    badges: resLabel(m),
    href: `/movie/${m.id}`,
    playHref: `/watch/${m.id}`,
    progress: p ? p.position / p.duration : undefined,
    resumeLabel: p ? remainingLabel(p.position, p.duration) : undefined,
    palette: m.palette,
  };
}

function heroFromShow(s: Show, eyebrow: string): HeroItem {
  const nu = nextUpFor(s);
  const ep = nu?.episode ?? showEpisodes(s)[0];
  const p = progressOf(ep.id);
  return {
    id: s.id,
    kind: 'show',
    title: s.title,
    logo: s.art.logo,
    backdrop: s.art.backdrop!,
    eyebrow,
    summary: s.summary,
    meta: [`${s.year}${s.status === 'Ended' ? `–${s.seasons[s.seasons.length - 1].year}` : ''}`, s.contentRating, `${s.seasons.length} Season${s.seasons.length > 1 ? 's' : ''}`, s.genres.slice(0, 2).join(' · ')],
    badges: s.isAnime ? ['HD', 'Japanese', 'Subtitles'] : ['4K', 'Dolby Vision', '5.1'].slice(0, s.id === 'meridian' ? 3 : 2),
    href: `/show/${s.id}`,
    playHref: `/watch/${ep.id}`,
    progress: p ? p.position / p.duration : undefined,
    resumeLabel: nu?.label,
    palette: s.palette,
  };
}

const posterItem = (c: LibraryCard): ShelfItem => ({ id: c.id, kind: c.kind, title: c.title, subtitle: c.subtitle, image: c.image, href: c.href, progress: c.progress, badge: c.badge, palette: c.palette });

function continueItems(): ShelfItem[] {
  const u = user();
  const entries = Object.values(u.progress).sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt));
  const out: ShelfItem[] = [];
  const seenShows = new Set<string>();
  for (const e of entries) {
    const m = byId.movie.get(e.itemId);
    const ep = byId.episode.get(e.itemId);
    const v = byId.homevideo.get(e.itemId);
    if (m && allowedRating(m.contentRating)) {
      out.push({ id: m.id, kind: 'movie', title: m.title, subtitle: remainingLabel(e.position, e.duration), image: m.art.backdrop!, logo: m.art.logo, href: `/watch/${m.id}`, progress: e.position / e.duration, palette: m.palette, meta: `/movie/${m.id}` });
    } else if (ep && !seenShows.has(ep.showId)) {
      const show = byId.show.get(ep.showId)!;
      if (!allowedRating(show.contentRating) || !allowedLibrary(show.libraryId)) continue;
      seenShows.add(ep.showId);
      out.push({ ...episodeShelfItem(ep), subtitle: `${episodeLabel(ep)} · ${remainingLabel(e.position, e.duration)}`, href: `/watch/${ep.id}`, meta: `/episode/${ep.id}`, logo: show.art.logo });
    } else if (v && allowedLibrary(v.libraryId)) {
      out.push({ id: v.id, kind: 'homevideo', title: v.title, subtitle: remainingLabel(e.position, e.duration), image: v.art.thumb!, href: `/watch/${v.id}`, progress: e.position / e.duration, palette: v.palette, meta: `/home-video/${v.id}` });
    }
  }
  // Up next from shows with recent activity
  for (const s of visibleShows()) {
    if (seenShows.has(s.id)) continue;
    const eps = showEpisodes(s);
    if (!eps.some((e) => isWatched(e.id))) continue;
    const nu = nextUpFor(s);
    if (!nu) continue;
    seenShows.add(s.id);
    const fresh = Date.now() - Date.parse(nu.episode.airDate) < 14 * 86_400_000;
    out.push({ ...episodeShelfItem(nu.episode), subtitle: `${fresh ? 'New · ' : 'Up next · '}${episodeLabel(nu.episode)}`, href: `/watch/${nu.episode.id}`, meta: `/episode/${nu.episode.id}`, badge: fresh ? 'New' : undefined, logo: s.art.logo });
  }
  return out.slice(0, 12);
}

function liveNowItems(): ShelfItem[] {
  const now = Date.now();
  return channels
    .filter((c) => !isKid() || c.category === 'Kids')
    .map((c) => {
      const p = programs.find((x) => x.channelId === c.id && Date.parse(x.start) <= now && Date.parse(x.end) > now);
      if (!p) return null;
      const prog = (now - Date.parse(p.start)) / (Date.parse(p.end) - Date.parse(p.start));
      return {
        id: `live-${c.id}`,
        kind: 'channel' as const,
        title: p.title,
        subtitle: `${c.number} ${c.name}${p.episodeTitle ? ` · ${p.episodeTitle}` : ''}`,
        image: p.image!,
        href: `/watch/${c.id}`,
        progress: prog,
        badge: p.isLive ? 'LIVE' : undefined,
        meta: c.id,
      };
    })
    .filter(Boolean)
    .slice(0, 10) as ShelfItem[];
}

function because(seed: Movie, pool: Movie[]): LibraryCard[] {
  return pool
    .filter((m) => m.id !== seed.id)
    .map((m) => ({ m, s: m.genres.filter((g) => seed.genres.includes(g)).length + (m.collectionIds.some((c) => seed.collectionIds.includes(c)) ? 1 : 0) }))
    .filter((x) => x.s > 0)
    .sort((a, b) => b.s - a.s || (b.m.ratings.critic ?? 0) - (a.m.ratings.critic ?? 0))
    .slice(0, 14)
    .map((x) => movieCard(x.m));
}

const recentMovies = (list: Movie[]) => [...list].sort((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt));

function homeFeed(): HomeFeed {
  const kid = isKid();
  const mv = visibleMovies();
  const sv = visibleShows(false);
  const av = visibleShows(true);
  const hero: HeroItem[] = [];
  const shelves: Shelf[] = [];
  const pick = (id: string) => mv.find((m) => m.id === id);
  const pickS = (id: string) => [...sv, ...av].find((s) => s.id === id);

  if (kid) {
    const ids = ['biscuit-takes-manhattan', 'little-explorers', 'spring', 'the-summer-of-two-dogs', 'big-buck-bunny'];
    for (const id of ids) {
      const m = pick(id);
      const s = pickS(id);
      if (m) hero.push(heroFromMovie(m, m.id === 'biscuit-takes-manhattan' ? 'Keep watching' : 'Just for you'));
      if (s) hero.push(heroFromShow(s, 'New episodes'));
    }
  } else {
    const df = pick('deep-field');
    const mer = pickS('meridian');
    if (df) hero.push(heroFromMovie(df, progressOf('deep-field') ? 'Continue watching' : 'New in 4K Dolby Vision'));
    if (mer) hero.push(heroFromShow(mer, 'New episode Friday'));
    for (const id of ['neon-orchard', 'tidewater']) {
      const m = pick(id);
      if (m) hero.push(heroFromMovie(m, 'Recently added'));
    }
    const k = pickS('kaze-to-kujira');
    if (k) hero.push(heroFromShow(k, 'Simulcast · New every Saturday'));
  }

  const cont = continueItems();
  if (cont.length) shelves.push({ id: 'continue', title: 'Continue Watching', style: 'continue', items: cont });

  const newEps = [...sv, ...av]
    .flatMap((s) => showEpisodes(s))
    .filter((e) => Date.now() - Date.parse(e.addedAt) < 45 * 86_400_000)
    .sort((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt))
    .slice(0, 14);
  if (newEps.length) shelves.push({ id: 'new-episodes', title: 'New Episodes', subtitle: 'Fresh from TV Shows and Anime', style: 'landscape', items: newEps.map((e) => episodeShelfItem(e)), href: '/tv' });

  shelves.push({ id: 'recent-movies', title: 'Recently Added Movies', style: 'poster', items: recentMovies(mv).slice(0, 18).map((m) => posterItem(movieCard(m))), href: '/movies?view=library&sort=added' });

  if (!kid) {
    const live = liveNowItems();
    if (live.length) shelves.push({ id: 'live-now', title: 'On Live TV Now', subtitle: '14 channels · 2 tuners free', style: 'live', items: live, href: '/live' });
  }

  const seed = byId.movie.get('the-long-quiet')!;
  if (!kid && isWatched('the-long-quiet')) shelves.push({ id: 'because', title: 'Because You Watched The Long Quiet', style: 'poster', items: because(seed, mv).map(posterItem) });

  const top = [...mv, ...sv]
    .map((x) => ({ x, score: (x.ratings.audience ?? 0) + (x.kind === 'movie' ? 0 : 3) + (rng(x.id)() * 6) }))
    .sort((a, b) => b.score - a.score)
    .slice(0, 10)
    .map(({ x }) => (x.kind === 'movie' ? posterItem(movieCard(x as Movie)) : posterItem(showCard(x as Show))));
  shelves.push({ id: 'top', title: kid ? 'Most Watched' : 'Top 10 in Your Household', subtitle: 'This week', style: 'top10', items: top });

  if (av.length) shelves.push({ id: 'anime', title: 'Anime', subtitle: 'Simulcasts and favorites', style: 'poster', items: av.map((s) => posterItem(showCard(s))), href: '/anime' });

  if (allowedLibrary('lib-music')) {
    const al = kid ? albums.filter((a) => a.genre === 'Kids' || a.genre === 'Classical') : [...albums].sort((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt)).slice(0, 12);
    shelves.push({ id: 'music', title: kid ? 'Songs for You' : 'Jump Back In', subtitle: kid ? undefined : 'Music', style: 'square', items: al.map((a) => posterItem(albumCard(a))), href: '/music' });
  }

  const books = audiobooks.filter((b) => (kid ? b.genres.includes('Kids') || b.genres.includes('Family') : true));
  const listening = books.filter((b) => progressOf(b.id));
  if (allowedLibrary('lib-audiobooks') && books.length)
    shelves.push({ id: 'books', title: listening.length ? 'Keep Listening' : 'Audiobooks', style: 'square', items: [...listening, ...books.filter((b) => !listening.includes(b))].slice(0, 12).map((b) => posterItem(audiobookCard(b))), href: '/audiobooks' });

  if (!kid && allowedLibrary('lib-photos'))
    shelves.push({ id: 'memories', title: 'Memories', subtitle: 'From Photos', style: 'landscape', items: memories.map((m) => ({ id: m.id, kind: 'photo', title: m.title, subtitle: m.subtitle, image: m.cover, href: `/photos?memory=${m.id}` })) });

  const famIds = collections.filter((c) => (kid ? c.id === 'col-family' || c.id === 'col-blender' : true));
  shelves.push({ id: 'collections', title: 'Collections', style: 'hero-cards', items: famIds.map((c) => ({ id: c.id, kind: 'collection', title: c.title, subtitle: `${c.itemIds.filter((i) => mv.some((m) => m.id === i)).length} films`, image: c.art.backdrop!, href: `/collection/${c.id}`, palette: c.palette })) });

  if (!kid) {
    shelves.push({ id: 'critics', title: 'Critically Acclaimed', subtitle: '90% or higher', style: 'poster', items: mv.filter((m) => (m.ratings.critic ?? 0) >= 90).sort((a, b) => (b.ratings.critic ?? 0) - (a.ratings.critic ?? 0)).slice(0, 16).map((m) => posterItem(movieCard(m))) });
    const shorts = mv.filter((m) => m.runtime < 20);
    shelves.push({ id: 'shorts', title: 'Short on Time', subtitle: 'Under 20 minutes', style: 'landscape', items: shorts.map((m) => ({ id: m.id, kind: 'movie', title: m.title, subtitle: `${m.runtime} min · ${m.year}`, image: m.art.backdrop!, href: `/movie/${m.id}`, palette: m.palette })) });
  }
  return { hero, shelves };
}

function libraryHome(libraryId: string): { hero?: HeroItem[]; shelves: Shelf[] } {
  if (libraryId === 'lib-movies') {
    const mv = visibleMovies();
    const hero = ['neon-orchard', 'glass-harbor', 'the-summit-line', 'midnight-at-the-aurora-palace']
      .map((id) => mv.find((m) => m.id === id))
      .filter(Boolean)
      .map((m) => heroFromMovie(m!, 'Featured'));
    const byGenre = (g: string) => mv.filter((m) => m.genres.includes(g));
    const shelves: Shelf[] = [];
    const cont = continueItems().filter((i) => i.kind === 'movie');
    if (cont.length) shelves.push({ id: 'm-cont', title: 'Continue Watching', style: 'continue', items: cont });
    shelves.push({ id: 'm-recent', title: 'Recently Added', style: 'poster', items: recentMovies(mv).slice(0, 20).map((m) => posterItem(movieCard(m))), href: '/movies?view=library&sort=added' });
    shelves.push({ id: 'm-uhd', title: '4K Dolby Vision', subtitle: 'The best-looking films on Harbor', style: 'landscape', items: mv.filter((m) => m.versions[0].hdr === 'Dolby Vision').map((m) => ({ id: m.id, kind: 'movie', title: m.title, subtitle: `${m.year} · ${m.genres[0]}`, image: m.art.backdrop!, href: `/movie/${m.id}`, logo: m.art.logo, palette: m.palette })) });
    for (const g of ['Science Fiction', 'Thriller', 'Drama', 'Comedy', 'Documentary', 'Family']) {
      const list = byGenre(g);
      if (list.length >= 3) shelves.push({ id: `m-g-${g}`, title: g === 'Science Fiction' ? 'Sci-Fi' : g, style: 'poster', items: list.map((m) => posterItem(movieCard(m))), href: `/movies?view=library&genre=${encodeURIComponent(g)}` });
    }
    shelves.push({ id: 'm-unwatched', title: 'Unwatched', style: 'poster', items: mv.filter((m) => !isWatched(m.id) && !progressOf(m.id)).slice(0, 20).map((m) => posterItem(movieCard(m))), href: '/movies?view=library&filter=unwatched' });
    return { hero, shelves };
  }
  const anime = libraryId === 'lib-anime';
  const sv = visibleShows(anime);
  const hero = (anime ? ['kaze-to-kujira', 'sora-no-kagami', 'tokyo-nightwalkers'] : ['meridian', 'undercurrent', 'saltmarsh', 'dispatch-from-mars'])
    .map((id) => sv.find((s) => s.id === id))
    .filter(Boolean)
    .map((s) => heroFromShow(s!, anime ? 'Simulcast' : 'Featured'));
  const shelves: Shelf[] = [];
  const cont = continueItems().filter((i) => i.kind === 'episode' && sv.some((s) => i.title === s.title));
  if (cont.length) shelves.push({ id: 's-cont', title: 'Continue Watching', style: 'continue', items: cont });
  const eps = sv.flatMap(showEpisodes).sort((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt)).slice(0, 14);
  shelves.push({ id: 's-new', title: anime ? 'New This Season' : 'New Episodes', style: 'landscape', items: eps.map((e) => episodeShelfItem(e)) });
  shelves.push({ id: 's-all', title: anime ? 'All Series' : 'All Shows', style: 'poster', items: sv.map((s) => posterItem(showCard(s))), href: `${anime ? '/anime' : '/tv'}?view=library` });
  const genres = Array.from(new Set(sv.flatMap((s) => s.genres))).filter((g) => g !== 'Anime');
  for (const g of genres.slice(0, 5)) {
    const list = sv.filter((s) => s.genres.includes(g));
    if (list.length >= 2) shelves.push({ id: `s-g-${g}`, title: g, style: 'poster', items: list.map((s) => posterItem(showCard(s))) });
  }
  return { hero, shelves };
}

function libraryItems(libraryId: string, q: LibraryQuery) {
  let cards: LibraryCard[] = [];
  let genres: string[] = [];
  let meta: Record<string, { rating?: number; runtime?: number; added: string; res?: string; hdr?: boolean }> = {};
  if (libraryId === 'lib-movies') {
    let list = visibleMovies();
    genres = Array.from(new Set(list.flatMap((m) => m.genres))).sort();
    if (q.genre) list = list.filter((m) => m.genres.includes(q.genre!));
    if (q.decade) list = list.filter((m) => `${Math.floor(m.year / 10) * 10}s` === q.decade);
    if (q.filter === '4k') list = list.filter((m) => m.versions[0].resolution === '2160p');
    if (q.filter === 'hdr') list = list.filter((m) => !!m.versions[0].hdr);
    meta = Object.fromEntries(list.map((m) => [m.id, { rating: m.ratings.critic, runtime: m.runtime, added: m.addedAt }]));
    cards = list.map(movieCard);
  } else if (libraryId === 'lib-tv' || libraryId === 'lib-anime') {
    let list = visibleShows(libraryId === 'lib-anime');
    genres = Array.from(new Set(list.flatMap((s) => s.genres))).filter((g) => g !== 'Anime').sort();
    if (q.genre) list = list.filter((s) => s.genres.includes(q.genre!));
    meta = Object.fromEntries(list.map((s) => [s.id, { rating: s.ratings.critic, added: s.addedAt }]));
    cards = list.map(showCard);
  } else if (libraryId === 'lib-audiobooks') {
    genres = Array.from(new Set(audiobooks.flatMap((b) => b.genres))).sort();
    let list = audiobooks.filter((b) => !isKid() || b.genres.includes('Kids') || b.genres.includes('Family'));
    if (q.genre) list = list.filter((b) => b.genres.includes(q.genre!));
    meta = Object.fromEntries(list.map((b) => [b.id, { added: b.addedAt, runtime: b.duration / 60 }]));
    cards = list.map(audiobookCard);
  } else if (libraryId === 'lib-homevideos') {
    cards = homeVideos.map(homeVideoCard);
    meta = Object.fromEntries(homeVideos.map((v) => [v.id, { added: v.addedAt }]));
  } else if (libraryId === 'lib-music') {
    cards = albums.map(albumCard);
    genres = Array.from(new Set(albums.map((a) => a.genre))).sort();
    if (q.genre) cards = cards.filter((c) => byId.album.get(c.id)!.genre === q.genre);
    meta = Object.fromEntries(albums.map((a) => [a.id, { added: a.addedAt }]));
  }
  if (q.filter === 'unwatched') cards = cards.filter((c) => !c.watched && !c.progress);
  if (q.filter === 'in-progress') cards = cards.filter((c) => c.progress);
  if (q.filter === 'watched') cards = cards.filter((c) => c.watched);
  if (q.filter === 'downloaded') cards = cards.filter((c) => downloadFor(c.id));
  const dir = q.dir === 'desc' ? -1 : 1;
  const sort = q.sort ?? 'title';
  cards.sort((a, b) => {
    switch (sort) {
      case 'added':
        return (Date.parse(meta[b.id]?.added ?? '0') - Date.parse(meta[a.id]?.added ?? '0')) * (q.dir === 'asc' ? -1 : 1);
      case 'year':
        return ((b.year ?? 0) - (a.year ?? 0)) * (q.dir === 'asc' ? -1 : 1);
      case 'rating':
        return ((meta[b.id]?.rating ?? 0) - (meta[a.id]?.rating ?? 0)) * (q.dir === 'asc' ? -1 : 1);
      case 'runtime':
        return ((meta[a.id]?.runtime ?? 0) - (meta[b.id]?.runtime ?? 0)) * dir;
      default:
        return a.sortKey.localeCompare(b.sortKey) * dir;
    }
  });
  // Review aid: ?scale=250000 multiplies the library so the virtualized grid can be judged at size.
  const scale = scaleParam();
  if (scale > cards.length && cards.length) {
    const base = cards;
    cards = Array.from({ length: scale }, (_, i) => {
      const c = base[i % base.length];
      return i < base.length ? c : { ...c, id: `${c.id}~${i}`, subtitle: `${c.subtitle} · copy ${Math.floor(i / base.length)}` };
    });
  }
  const offset = q.offset ?? 0;
  const limit = q.limit ?? 1_000_000;
  return { items: cards.slice(offset, offset + limit), total: cards.length, offset, genres };
}

function scaleParam(): number {
  try {
    const v = new URLSearchParams(window.location.search).get('scale');
    if (v !== null) sessionStorage.setItem('portico.scale', v);
    return Number(sessionStorage.getItem('portico.scale') ?? 0) || 0;
  } catch {
    return 0;
  }
}

// ---------- search ----------
const norm = (s: string) => s.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[’']/g, '');
function score(q: string, text: string): number {
  const t = norm(text);
  if (!q) return 0;
  if (t === q) return 100;
  if (t.startsWith(q)) return 80 - Math.min(20, t.length - q.length) / 2;
  const words = t.split(/[^a-z0-9]+/);
  if (words.some((w) => w.startsWith(q))) return 60;
  if (t.includes(q)) return 45;
  // subsequence fuzzy match
  let i = 0;
  for (const ch of t) if (ch === q[i]) i++;
  if (i === q.length && q.length >= 3) return 20 - (t.length - q.length) / 10;
  return 0;
}

function doSearch(query: string, kinds?: string[]) {
  const t0 = performance.now();
  const q = norm(query.trim());
  const hits: SearchHit[] = [];
  const add = (h: Omit<SearchHit, 'score'>, s: number) => {
    if (s > 0 && (!kinds || kinds.includes(h.kind))) hits.push({ ...h, score: s });
  };
  for (const m of visibleMovies()) {
    add({ id: m.id, kind: 'movie', title: m.title, subtitle: `Movie · ${m.year} · ${m.genres[0]}`, image: m.art.poster, href: `/movie/${m.id}`, shape: 'poster' }, Math.max(score(q, m.title), score(q, m.genres.join(' ')) * 0.4, score(q, m.studio) * 0.3));
  }
  for (const s of visibleShows()) {
    add({ id: s.id, kind: 'show', title: s.title, subtitle: `${s.isAnime ? 'Anime' : 'TV Show'} · ${s.year} · ${s.network}`, image: s.art.poster, href: `/show/${s.id}`, shape: 'poster' }, Math.max(score(q, s.title) ? score(q, s.title) + 2 : 0, s.originalTitle ? score(q, s.originalTitle) : 0, score(q, s.genres.join(' ')) * 0.4));
  }
  if (q.length >= 3)
    for (const e of episodes) {
      const s = byId.show.get(e.showId)!;
      if (!allowedRating(s.contentRating) || !allowedLibrary(s.libraryId)) continue;
      add({ id: e.id, kind: 'episode', title: e.title, subtitle: `${e.showTitle} · ${episodeLabel(e)}`, image: e.art.thumb, href: `/episode/${e.id}`, shape: 'landscape' }, score(q, e.title) * 0.75);
    }
  if (!isKid())
    for (const p of people) {
      add({ id: p.id, kind: 'person', title: p.name, subtitle: p.knownFor, href: `/person/${p.id}`, shape: 'circle' }, score(q, p.name) * 0.95);
    }
  if (allowedLibrary('lib-music')) {
    for (const a of artists) add({ id: a.id, kind: 'artist', title: a.title, subtitle: `Artist · ${a.genres[0]}`, image: a.art.square, href: `/artist/${a.id}`, shape: 'circle' }, score(q, a.title));
    for (const a of albums) add({ id: a.id, kind: 'album', title: a.title, subtitle: `${a.format} · ${a.artistName} · ${a.year}`, image: a.art.square, href: `/album/${a.id}`, shape: 'square' }, Math.max(score(q, a.title), score(q, a.genre) * 0.4));
    for (const t of tracks) add({ id: t.id, kind: 'track', title: t.title, subtitle: `Song · ${t.artistName}`, image: t.art.square, href: `/album/${t.albumId}?track=${t.id}`, shape: 'square' }, Math.max(score(q, t.title) * 0.85, t.lyrics ? score(q, t.lyrics.map((l) => l.text).join(' ')) * 0.4 : 0));
    for (const p of playlists) add({ id: p.id, kind: 'playlist', title: p.title, subtitle: `Playlist · ${p.trackIds.length} songs`, image: p.art.square, href: `/playlist/${p.id}`, shape: 'square' }, score(q, p.title));
  }
  if (allowedLibrary('lib-audiobooks'))
    for (const b of audiobooks) add({ id: b.id, kind: 'audiobook', title: b.title, subtitle: `Audiobook · ${b.author}`, image: b.art.square, href: `/audiobook/${b.id}`, shape: 'square' }, Math.max(score(q, b.title), score(q, b.author) * 0.9));
  for (const c of collections) add({ id: c.id, kind: 'collection', title: c.title, subtitle: `Collection · ${c.itemIds.length} films`, image: c.art.backdrop, href: `/collection/${c.id}`, shape: 'landscape' }, score(q, c.title) * 0.9);
  if (!isKid()) {
    for (const c of channels) add({ id: c.id, kind: 'channel', title: `${c.number} ${c.name}`, subtitle: `Live TV · ${c.category}`, href: `/watch/${c.id}`, shape: 'landscape' }, Math.max(score(q, c.name), score(q, c.callSign)));
    const seen = new Set<string>();
    for (const p of programs) {
      if (seen.has(p.title) || Date.parse(p.end) < NOW) continue;
      const s = score(q, p.title) * 0.7;
      if (s > 0) {
        seen.add(p.title);
        const ch = channels.find((c) => c.id === p.channelId)!;
        const at = new Date(p.start);
        add({ id: p.id, kind: 'program', title: p.title, subtitle: `On ${ch.name} · ${at.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}`, image: p.image, href: `/live?program=${p.id}`, shape: 'landscape' }, s);
      }
    }
    for (const a of photoAlbums) add({ id: a.id, kind: 'photo', title: a.title, subtitle: `Photos · ${a.subtitle}`, image: a.cover, href: `/photos?album=${a.id}`, shape: 'landscape' }, score(q, a.title));
    for (const v of homeVideos) add({ id: v.id, kind: 'homevideo', title: v.title, subtitle: `Home video · ${v.place}`, image: v.art.thumb, href: `/home-video/${v.id}`, shape: 'landscape' }, Math.max(score(q, v.title), score(q, v.place ?? '') * 0.5));
  }
  // Person → their work
  const person = people.find((p) => norm(p.name) === q || (q.length > 4 && norm(p.name).includes(q)));
  if (person) {
    for (const c of creditsByPerson[person.id] ?? []) {
      const card = cardFor(c.itemId);
      if (card && !hits.some((h) => h.id === card.id)) hits.push({ id: card.id, kind: card.kind, title: card.title, subtitle: `${c.role} · ${card.subtitle}`, image: card.image, href: card.href, shape: 'poster', score: 50 });
    }
  }
  hits.sort((a, b) => b.score - a.score);
  // When there are real matches, drop the loose subsequence ones.
  if (hits.some((h) => h.score >= 45)) for (let i = hits.length - 1; i >= 0; i--) if (hits[i].score < 22) hits.splice(i, 1);
  const order: [string, string][] = [
    ['movie', 'Movies'],
    ['show', 'TV Shows & Anime'],
    ['episode', 'Episodes'],
    ['person', 'Cast & Crew'],
    ['collection', 'Collections'],
    ['artist', 'Artists'],
    ['album', 'Albums'],
    ['track', 'Songs'],
    ['playlist', 'Playlists'],
    ['audiobook', 'Audiobooks'],
    ['channel', 'Live TV Channels'],
    ['program', 'On Live TV'],
    ['photo', 'Photos'],
    ['homevideo', 'Home Videos'],
  ];
  const groups = order
    .map(([kind, label]) => ({ kind, label, hits: hits.filter((h) => h.kind === kind).slice(0, 18) }))
    .filter((g) => g.hits.length)
    .sort((a, b) => b.hits[0].score - a.hits[0].score);
  return { query, top: hits[0], groups, total: hits.length, tookMs: Math.max(1, Math.round(performance.now() - t0)) };
}

// ---------- API ----------

const now = () => new Date().toISOString();
const addHistory = (itemId: string, completed: boolean) => {
  const card = cardFor(itemId);
  if (!card) return;
  const h: HistoryEntry = {
    id: `h-${Date.now()}`,
    itemId,
    itemKind: card.kind,
    title: card.title,
    subtitle: card.subtitle,
    image: card.image,
    at: now(),
    device: 'Safari on MacBook Pro',
    profileId: profile().id,
    completed,
  };
  const u = user();
  u.history = [h, ...u.history.filter((x) => x.itemId !== itemId)].slice(0, 200);
};

const adminSections: Record<string, Record<string, unknown>> = {
  transcoding: transcoderSeed as unknown as Record<string, unknown>,
  network: networkSeed as unknown as Record<string, unknown>,
  storage: storageSeed as unknown as Record<string, unknown>,
  backups: backupsSeed as unknown as Record<string, unknown>,
  livetv: liveSourcesSeed as unknown as Record<string, unknown>,
  metadata: metadataSeed as unknown as Record<string, unknown>,
  diagnostics: diagnosticsSeed as unknown as Record<string, unknown>,
  settings: settingsSeed as unknown as Record<string, unknown>,
};

function librariesAdmin(): LibraryAdmin[] {
  const scanning = db.tasks.find((t) => t.id === 't1')?.status === 'running';
  return libraries.map((l, i) => ({
    id: l.id,
    name: l.name,
    kind: l.kind,
    paths: l.paths.map((p, j) => ({ path: p, items: Math.round(l.count / l.paths.length) + j, sizeBytes: l.sizeBytes / l.paths.length, online: true })),
    agent: { movies: 'The Movie Database', shows: 'TheTVDB + TMDB', anime: 'AniDB + AniList', music: 'MusicBrainz', audiobooks: 'Open Library', photos: 'Embedded EXIF', homevideos: 'Embedded metadata' }[l.kind],
    language: l.kind === 'anime' ? 'Japanese (original) · English' : 'English (US)',
    scanInterval: 'Every 6 hours + file watcher',
    watchFolders: true,
    lastScan: i === 1 ? minutesAgo(9) : hoursAgo(2 + i),
    lastScanDuration: ['41 s', '1 min 12 s', '22 s', '58 s', '6 s', '1 min 40 s', '9 s'][i],
    scan: scanning && l.id === 'lib-tv' ? { progress: 0.64, phase: 'Analyzing media', current: '/volume1/media/TV/Meridian/Season 3/Meridian.S03E05.2160p.mkv', eta: '1 min' } : undefined,
    options: (l.kind === 'music'
        ? { 'Loudness analysis': true, 'Fetch synced lyrics': true, 'Prefer local artwork': false, 'Group compilations': true }
        : l.kind === 'photos'
          ? { 'Detect faces': false, 'Generate memories': true, 'Include RAW files': true }
          : { 'Generate preview thumbnails': true, 'Detect intros': l.kind !== 'movies', 'Detect credits': true, 'Prefer local artwork': false, 'Collections from metadata': l.kind === 'movies' }) as Record<string, boolean | string>,
    issues: [1, 1, 1, 1, 0, 0, 1][i],
  }));
}

const liveMetric = () => metricAt(Date.now());

export const fixtureApi: PorticoApi = {
  listServers: () =>
    simulate('listServers', (): ServerSummary[] => [
      { id: 'srv-harbor', name: 'Harbor', owner: 'Maya Lindqvist', version: server.version, address: '192.168.1.20 · harbor.lindqvist.family', local: true, online: true, libraries: libraries.length, latencyMs: 3 },
      { id: 'srv-cabin', name: 'Cabin', owner: 'Maya Lindqvist', version: '3.1.4', address: 'cabin.lindqvist.family', local: false, online: false, libraries: 2, latencyMs: 0 },
      { id: 'srv-sam', name: 'Sam’s Media', owner: 'Sam Carter (shared with you)', version: '3.2.1', address: 'Relay · Montréal', local: false, online: true, libraries: 3, latencyMs: 41 },
    ]),
  signIn: (email, password) =>
    simulate(
      'signIn',
      () => {
        if (!email.includes('@')) return { ok: false, error: 'Enter the email address for your Portico account.' };
        if (password.length < 6) return { ok: false, error: 'That password is incorrect. Try again or reset it.' };
        return { ok: true, needsMfa: true };
      },
      2,
    ),
  verifyMfa: (code) => simulate('verifyMfa', () => (/^\d{6}$/.test(code) ? { ok: true } : { ok: false, error: 'That code didn’t match. Codes refresh every 30 seconds.' }), 1.5),
  signInWithPasskey: () => simulate('signInWithPasskey', () => ({ ok: true }), 2),
  getProfiles: () => simulate('getProfiles', () => db.profiles),
  unlockProfile: (id, pin) => simulate('unlockProfile', () => (db.profiles.find((p) => p.id === id)?.hasPin ? pin === '1234' || pin === '2580' : true)),
  createProfile: (p) =>
    simulate('createProfile', () => {
      const id = p.name.toLowerCase().replace(/[^a-z0-9]+/g, '-') + '-' + Math.random().toString(36).slice(2, 6);
      const created: Profile = {
        ...db.profiles[2],
        id,
        name: p.name,
        initials: p.name[0]?.toUpperCase() ?? '?',
        role: p.role ?? 'member',
        avatarColor: p.avatarColor ?? '#5cb3e2',
        maxRating: p.maxRating ?? (p.role === 'kid' ? 'G' : 'R'),
        maxTvRating: p.maxTvRating ?? (p.role === 'kid' ? 'TV-Y7' : 'TV-MA'),
        hasPin: false,
        libraries: 'all',
        email: undefined,
        restrictions: p.role === 'kid' ? { bedtime: '20:00' } : undefined,
        createdAt: now(),
        lastActive: now(),
      };
      db.profiles.push(created);
      persist();
      return created;
    }),
  updateProfile: (id, patch) =>
    simulate('updateProfile', () => {
      const p = db.profiles.find((x) => x.id === id);
      if (!p) throw new ApiError('Profile not found', 404);
      Object.assign(p, patch);
      persist();
      return p;
    }),
  deleteProfile: (id) =>
    simulate('deleteProfile', () => {
      db.profiles = db.profiles.filter((p) => p.id !== id || p.role === 'owner');
      persist();
    }),

  getLibraries: () => simulate('getLibraries', () => libraries.filter((l) => allowedLibrary(l.id))),
  getHome: () => simulate('getHome', homeFeed, 1.3),
  getLibraryHome: (id) => simulate('getLibraryHome', () => libraryHome(id)),
  getLibraryItems: (id, q) => simulate('getLibraryItems', () => libraryItems(id, q)),
  getMovie: (id) =>
    simulate('getMovie', () => {
      const m = byId.movie.get(id);
      if (!m) throw new ApiError('This movie isn’t on Harbor anymore.', 404);
      return {
        movie: m,
        state: stateOf(id),
        related: because(m, visibleMovies()).slice(0, 12),
        collections: collections.filter((c) => m.collectionIds.includes(c.id)).map((c) => ({ ...c, items: c.itemIds.map((i) => byId.movie.get(i)!).filter((x) => allowedRating(x.contentRating)).map(movieCard) })),
      };
    }),
  getShow: (id) =>
    simulate('getShow', () => {
      const s = byId.show.get(id);
      if (!s) throw new ApiError('This show isn’t on Harbor anymore.', 404);
      const eps = showEpisodes(s);
      const nu = nextUpFor(s);
      return {
        show: s,
        state: stateOf(id),
        watched: Object.fromEntries(eps.map((e) => [e.id, isWatched(e.id)])),
        progress: Object.fromEntries(eps.filter((e) => progressOf(e.id)).map((e) => [e.id, progressOf(e.id)!])),
        nextUp: nu,
        related: visibleShows(s.isAnime).filter((x) => x.id !== id && x.genres.some((g) => s.genres.includes(g) && g !== 'Anime')).map(showCard).slice(0, 10),
      };
    }),
  getEpisode: (id) =>
    simulate('getEpisode', () => {
      const e = byId.episode.get(id);
      if (!e) throw new ApiError('Episode not found', 404);
      const s = byId.show.get(e.showId)!;
      const eps = showEpisodes(s);
      const i = eps.findIndex((x) => x.id === id);
      return { episode: e, show: s, state: stateOf(id), prev: eps[i - 1], next: eps[i + 1] };
    }),
  getCollection: (id) =>
    simulate('getCollection', () => {
      const c = byId.collection.get(id);
      if (!c) throw new ApiError('Collection not found', 404);
      return { ...c, items: c.itemIds.map((i) => byId.movie.get(i)!).filter((m) => allowedRating(m.contentRating)).map(movieCard) };
    }),
  getCollections: () => simulate('getCollections', () => collections.map((c) => ({ ...c, count: c.itemIds.filter((i) => allowedRating(byId.movie.get(i)!.contentRating)).length })).filter((c) => c.count > 0)),
  getPerson: (id) =>
    simulate('getPerson', () => {
      const p = people.find((x) => x.id === id);
      if (!p) throw new ApiError('Person not found', 404);
      const credits = (creditsByPerson[id] ?? []).map((c) => ({ ...cardFor(c.itemId)!, role: c.role })).filter((c) => c.id);
      return { person: p, credits };
    }),
  getMusicHome: () =>
    simulate('getMusicHome', () => {
      const recent = [...albums].sort((a, b) => Date.parse(b.addedAt) - Date.parse(a.addedAt));
      const loved = tracks.filter((t) => t.loved);
      const shelves: Shelf[] = [
        { id: 'mu-new', title: 'Recently Added', style: 'square', items: recent.slice(0, 12).map((a) => posterItem(albumCard(a))), href: '/music?view=albums' },
        { id: 'mu-pl', title: 'Your Playlists', style: 'square', items: playlists.filter((p) => !p.smart).map((p) => ({ id: p.id, kind: 'playlist', title: p.title, subtitle: `${p.trackIds.length} songs`, image: p.art.square!, href: `/playlist/${p.id}` })) },
        { id: 'mu-artists', title: 'Artists You Play Most', style: 'circle', items: artists.slice(0, 10).map((a) => ({ id: a.id, kind: 'artist', title: a.title, subtitle: a.genres[0], image: a.art.square!, href: `/artist/${a.id}` })), href: '/music?view=artists' },
        { id: 'mu-mixes', title: 'Made for Maya', subtitle: 'Mixes that update every Monday', style: 'square', items: playlists.filter((p) => p.smart).map((p) => ({ id: p.id, kind: 'playlist', title: p.title, subtitle: p.smart!.rule, image: p.art.square!, href: `/playlist/${p.id}` })) },
        { id: 'mu-loved', title: 'Loved Recently', style: 'square', items: Array.from(new Map(loved.map((t) => [t.albumId, t])).values()).slice(0, 10).map((t) => ({ id: t.albumId, kind: 'album', title: t.title, subtitle: t.artistName, image: t.art.square!, href: `/album/${t.albumId}` })) },
        { id: 'mu-classical', title: 'Quiet Hours', subtitle: 'Classical and jazz', style: 'square', items: albums.filter((a) => a.genre === 'Classical' || a.genre === 'Jazz').map((a) => posterItem(albumCard(a))) },
      ];
      return { recent: recent.slice(0, 6).map(albumCard), shelves, playlists, artists, albums: recent };
    }),
  getArtists: () => simulate('getArtists', () => [...artists].sort((a, b) => a.title.localeCompare(b.title))),
  getAlbums: (sort = 'added') =>
    simulate('getAlbums', () =>
      [...albums].sort((a: Album, b: Album) =>
        sort === 'title' ? a.title.localeCompare(b.title) : sort === 'artist' ? a.artistName.localeCompare(b.artistName) || b.year - a.year : sort === 'year' ? b.year - a.year : Date.parse(b.addedAt) - Date.parse(a.addedAt),
      ),
    ),
  getSongs: (offset, limit, sort = 'title') =>
    simulate('getSongs', () => {
      const list = [...tracks].sort((a: Track, b: Track) =>
        sort === 'artist' ? a.artistName.localeCompare(b.artistName) || a.albumTitle.localeCompare(b.albumTitle) || a.number - b.number : sort === 'album' ? a.albumTitle.localeCompare(b.albumTitle) || a.number - b.number : sort === 'plays' ? b.plays - a.plays : a.title.localeCompare(b.title),
      );
      return { items: list.slice(offset, offset + limit), total: list.length, offset };
    }),
  getArtist: (id) =>
    simulate('getArtist', () => {
      const a = byId.artist.get(id);
      if (!a) throw new ApiError('Artist not found', 404);
      const al = a.albumIds.map((x) => byId.album.get(x)!);
      const top = al.flatMap((x) => x.tracks).sort((x, y) => y.plays - x.plays).slice(0, 8);
      return { artist: a, albums: al, topTracks: top, similar: a.similar.map((s) => byId.artist.get(s)!).filter(Boolean), appearsOn: playlists.filter((p) => p.trackIds.some((t) => byId.track.get(t)?.artistId === id)) };
    }),
  getAlbum: (id) =>
    simulate('getAlbum', () => {
      const a = byId.album.get(id);
      if (!a) throw new ApiError('Album not found', 404);
      return { album: a, more: albums.filter((x) => x.artistId === a.artistId && x.id !== id) };
    }),
  getPlaylists: () => simulate('getPlaylists', () => playlists),
  getPlaylist: (id) =>
    simulate('getPlaylist', () => {
      const p = byId.playlist.get(id);
      if (!p) throw new ApiError('Playlist not found', 404);
      return { playlist: p, tracks: p.trackIds.map((t) => byId.track.get(t)!).filter(Boolean) };
    }),
  getTracks: (ids) => simulate('getTracks', () => ids.map((t) => byId.track.get(t)!).filter(Boolean), 0.3),
  getAudiobooks: () => simulate('getAudiobooks', () => audiobooks.map((b) => ({ ...b, progress: progressOf(b.id) ? progressOf(b.id)!.position / b.duration : undefined }))),
  getAudiobook: (id) =>
    simulate('getAudiobook', () => {
      const b = byId.audiobook.get(id);
      if (!b) throw new ApiError('Audiobook not found', 404);
      return { book: b, state: stateOf(id), more: audiobooks.filter((x) => x.id !== id && (x.author === b.author || x.genres.some((g) => b.genres.includes(g)))).slice(0, 8) };
    }),
  getPhotos: (albumId) => simulate('getPhotos', () => (albumId ? photos.filter((p) => photoAlbums.find((a) => a.id === albumId)?.photoIds.includes(p.id)) : photos)),
  getPhotoAlbums: () => simulate('getPhotoAlbums', () => photoAlbums),
  getMemories: () => simulate('getMemories', () => memories),
  getHomeVideos: () => simulate('getHomeVideos', () => [...homeVideos].sort((a, b) => Date.parse(b.recordedAt) - Date.parse(a.recordedAt))),
  getHomeVideo: (id) =>
    simulate('getHomeVideo', () => {
      const v = byId.homevideo.get(id);
      if (!v) throw new ApiError('Video not found', 404);
      return { video: v, state: stateOf(id), more: homeVideos.filter((x) => x.id !== id).slice(0, 10) };
    }),
  search: (query, kinds) =>
    simulate(
      'search',
      () => {
        const res = doSearch(query, kinds);
        const u = user();
        if (query.trim().length > 2) {
          u.recentSearches = [query.trim(), ...u.recentSearches.filter((s) => s.toLowerCase() !== query.trim().toLowerCase())].slice(0, 8);
          persist();
        }
        return res;
      },
      0.35,
    ),
  getRecentSearches: () => simulate('getRecentSearches', () => user().recentSearches, 0.2),

  getWatchlist: () => simulate('getWatchlist', () => user().watchlist.map(cardFor).filter((c): c is LibraryCard => !!c)),
  toggleWatchlist: (id) =>
    simulate('toggleWatchlist', () => {
      const u = user();
      const on = !u.watchlist.includes(id);
      u.watchlist = on ? [id, ...u.watchlist] : u.watchlist.filter((x) => x !== id);
      persist();
      return on;
    }, 0.4),
  setWatched: (id, watched) =>
    simulate('setWatched', () => {
      const u = user();
      const ids = [id];
      const s = byId.show.get(id);
      if (s) ids.push(...showEpisodes(s).map((e) => e.id));
      const season = shows.flatMap((x) => x.seasons).find((x) => x.id === id);
      if (season) ids.push(...season.episodes.map((e) => e.id));
      u.watched = watched ? Array.from(new Set([...u.watched, ...ids])) : u.watched.filter((x) => !ids.includes(x));
      for (const i of ids) delete u.progress[i];
      if (watched) addHistory(id, true);
      persist();
    }, 0.4),
  setRating: (id, rating) =>
    simulate('setRating', () => {
      user().ratings[id] = rating;
      persist();
    }, 0.3),
  getHistory: () => simulate('getHistory', () => user().history),
  clearHistory: () =>
    simulate('clearHistory', () => {
      user().history = [];
      persist();
    }),
  saveProgress: (id, position, duration) =>
    simulate('saveProgress', () => {
      const u = user();
      if (position / duration > 0.94) {
        delete u.progress[id];
        if (!u.watched.includes(id)) u.watched.push(id);
        addHistory(id, true);
      } else if (position > 15) {
        u.progress[id] = { itemId: id, position, duration, updatedAt: now() };
        if (!u.history[0] || u.history[0].itemId !== id) addHistory(id, false);
      }
      persist();
    }, 0.1),
  getContinue: () => simulate('getContinue', () => continueItems().map((i) => ({ ...i, sortKey: i.title, subtitle: i.subtitle ?? '' }))),
  getDownloads: () => simulate('getDownloads', () => db.downloads),
  startDownload: (itemId, quality) =>
    simulate('startDownload', () => {
      const c = cardFor(itemId);
      if (!c) throw new ApiError('Can’t download this item', 400);
      const existing = db.downloads.find((d) => d.itemId === itemId);
      if (existing) {
        Object.assign(existing, { status: 'downloading', progress: existing.status === 'expired' ? 0 : existing.progress, quality, error: undefined });
        persist();
        return existing;
      }
      const kind = kindOf(itemId);
      const bytes = kind === 'movie' ? 5.9e9 : kind === 'episode' ? 2.2e9 : kind === 'album' ? 380e6 : kind === 'audiobook' ? 240e6 : 900e6;
      const d: Download = { id: `dl-${Date.now()}`, itemId, itemKind: c.kind, title: c.title, subtitle: c.subtitle, image: c.image, quality, sizeBytes: quality === 'Original' ? bytes * 2.6 : quality === 'Medium (720p)' ? bytes * 0.45 : quality === 'Low (480p)' ? bytes * 0.22 : bytes, progress: 0, status: 'downloading', device: 'This browser' };
      db.downloads.unshift(d);
      persist();
      return d;
    }, 0.5),
  removeDownload: (id) =>
    simulate('removeDownload', () => {
      db.downloads = db.downloads.filter((d) => d.id !== id);
      persist();
    }, 0.3),
  getNotifications: () => simulate('getNotifications', notificationsFor, 0.6),
  markNotificationsRead: (ids) =>
    simulate('markNotificationsRead', () => {
      const all = ids ?? notificationsFor().map((n) => n.id);
      db.notificationsRead = Array.from(new Set([...db.notificationsRead, ...all]));
      persist();
    }, 0.2),
  getCastTargets: () => simulate('getCastTargets', () => castTargets, 1.2),
  getSecurity: () =>
    simulate('getSecurity', (): SecurityState => ({
      mfa: {
        totp: true,
        passkeys: [
          { id: 'pk1', name: 'iCloud Keychain', created: daysAgo(300), lastUsed: minutesAgo(1) },
          { id: 'pk2', name: 'YubiKey 5C NFC', created: daysAgo(610), lastUsed: daysAgo(40) },
        ],
        recoveryCodesLeft: 8,
      },
      password: { lastChanged: daysAgo(212), strength: 'strong' },
      devices: devices.filter((d) => !db.revokedDevices.includes(d.id)),
      signInHistory: [
        { at: minutesAgo(1), device: 'Safari on MacBook Pro', location: 'Portland, ME', ip: '192.168.1.31', ok: true, method: 'Passkey' },
        { at: hoursAgo(26), device: 'Maya’s iPhone', location: 'Portland, ME', ip: '192.168.1.52', ok: true, method: 'Face ID (passkey)' },
        { at: daysAgo(3, 23), device: 'Unknown browser', location: 'Frankfurt, DE', ip: '185.220.101.7', ok: false, method: 'Password — blocked (rate limit)' },
        { at: daysAgo(6, 9), device: 'Chrome on Windows', location: 'Boston, MA', ip: '73.114.22.9', ok: true, method: 'Password + authenticator' },
      ],
    })),
  revokeDevice: (id) =>
    simulate('revokeDevice', () => {
      db.revokedDevices.push(id);
      persist();
    }),

  getPlayable: (id) =>
    simulate(
      'getPlayable',
      (): PlayableSource => {
        const m = byId.movie.get(id);
        const e = byId.episode.get(id);
        const v = byId.homevideo.get(id);
        const t = byId.track.get(id);
        const b = byId.audiobook.get(id);
        const ch = channels.find((c) => c.id === id);
        const rec = recordings.find((r) => r.id === id);
        const p = progressOf(id);
        const scrubCR = { url: '/media/coffee-run-scrub.jpg', every: 2, cols: 10, rows: 10, width: 240, height: 100 };
        const scrubST = { url: '/media/sintel-trailer-scrub.jpg', every: 2, cols: 10, rows: 3, width: 240, height: 136 };
        const video = (stream: string, duration: number) => {
          const cr = stream === 'coffee-run';
          return { itemId: id, kind: 'video' as const, url: `/media/${stream}.mp4`, mediaDuration: cr ? 184.6 : 52.2, duration, scrub: cr ? scrubCR : scrubST, startAt: p && p.position < duration - 30 ? p.position : 0 };
        };
        if (m) return video(m.streamId, m.id === 'coffee-run' ? 184.6 : m.runtime * 60);
        if (e) return video('coffee-run', e.runtime * 60);
        if (v) return video('coffee-run', v.duration);
        if (rec) return video('coffee-run', rec.duration);
        if (ch) return { ...video(ch.streamId, 3600), startAt: 0 };
        if (t) return { itemId: id, kind: 'audio', url: `/media/music/${t.streamId}.m4a`, mediaDuration: 128.3, duration: t.duration, startAt: 0 };
        if (b) return { itemId: id, kind: 'audio', url: `/media/books/${b.id}.m4a`, duration: b.duration, startAt: p?.position ?? 0 };
        throw new ApiError('This item can’t be played.', 404);
      },
      0.6,
    ),

  getGuide: () =>
    simulate('getGuide', (): GuideData => ({
      start: GUIDE_START,
      hours: GUIDE_HOURS,
      channels: isKid() ? channels.filter((c) => c.category === 'Kids') : channels,
      programs,
      scheduledProgramIds: db.scheduled.map((s) => s.programId),
    })),
  getDvr: () =>
    simulate('getDvr', (): DvrData => {
      const recs = recordings.filter((r) => !db.deletedRecordings.includes(r.id));
      const live = programs.find((p) => p.channelId === 'ch-5' && Date.parse(p.start) <= Date.now() && Date.parse(p.end) > Date.now());
      const nowRec = live
        ? [{ id: 'rec-now', programId: live.id, title: live.title, episodeTitle: live.episodeTitle, channelName: 'PBN', recordedAt: live.start, duration: (Date.parse(live.end) - Date.parse(live.start)) / 1000, sizeBytes: 1.9e9, image: live.image!, status: 'recording' as const }]
        : [];
      return { recordings: [...nowRec, ...recs], scheduled: db.scheduled, rules: seriesRules, diskUsedBytes: 2.2e12, diskLimitBytes: 3e12 };
    }),
  scheduleRecording: (programId, series) =>
    simulate('scheduleRecording', () => {
      const p = programs.find((x) => x.id === programId);
      if (!p) throw new ApiError('Program not found', 404);
      const targets = series ? programs.filter((x) => x.title === p.title && x.channelId === p.channelId && Date.parse(x.start) >= Date.parse(p.start)) : [p];
      for (const t of targets) {
        if (db.scheduled.some((s) => s.programId === t.id)) continue;
        const ch = channels.find((c) => c.id === t.channelId)!;
        db.scheduled.push({ id: `sch-${t.id}`, programId: t.id, title: t.title, episodeTitle: t.episodeTitle, channelId: ch.id, channelName: ch.name, start: t.start, end: t.end, ruleId: series ? 'rule-new' : undefined });
      }
      db.scheduled.sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
      return db.scheduled;
    }, 0.5),
  cancelRecording: (id) =>
    simulate('cancelRecording', () => {
      db.scheduled = db.scheduled.filter((s) => s.id !== id && s.programId !== id);
    }, 0.4),
  deleteRecording: (id) =>
    simulate('deleteRecording', () => {
      db.deletedRecordings.push(id);
      persist();
    }, 0.4),

  getAdminOverview: () =>
    simulate('getAdminOverview', (): AdminOverview => {
      const checks = diagnosticsSeed.checks;
      return {
        server,
        metrics: metricHistory(60, 15),
        sessions: db.sessions,
        activity: activitySeed.slice(0, 8),
        health: { ok: checks.filter((c) => c.status === 'ok').length, warn: checks.filter((c) => c.status === 'warn').length, info: checks.filter((c) => c.status === 'info').length },
        libraries,
        storage: { used: storageSeed.volumes.reduce((a, v) => a + v.used, 0), size: storageSeed.volumes.reduce((a, v) => a + v.size, 0) },
      };
    }),
  getMetrics: () => simulate('getMetrics', () => [liveMetric()], 0.1),
  getSessions: () =>
    simulate('getSessions', () => {
      const dt = (Date.now() - NOW) / 1000;
      return db.sessions.map((s) => (s.state === 'playing' ? { ...s, position: Math.min(s.duration, s.position + dt) } : s));
    }, 0.4),
  terminateSession: (id) =>
    simulate('terminateSession', () => {
      db.sessions = db.sessions.filter((s) => s.id !== id);
    }),
  getActivity: () => simulate('getActivity', () => activitySeed),
  getLogs: ({ levels, source, text, offset, limit }) =>
    simulate(
      'getLogs',
      () => {
        // Newest first. Filtering walks the lazily generated log; fine for the fixture's 36k lines.
        const lv = levels && levels.length ? new Set(levels) : null;
        const tq = text?.toLowerCase();
        const out = [];
        let matched = 0;
        for (let i = LOG_COUNT - 1; i >= 0; i--) {
          const l = logAt(i);
          if (lv && !lv.has(l.level)) continue;
          if (source && l.source !== source) continue;
          if (tq && !l.msg.toLowerCase().includes(tq)) continue;
          if (matched >= offset && out.length < limit) out.push(l);
          matched++;
        }
        return { items: out, total: matched, offset };
      },
      0.5,
    ),
  getLibrariesAdmin: () => simulate('getLibrariesAdmin', librariesAdmin),
  scanLibrary: (id) =>
    simulate('scanLibrary', () => {
      const t = db.tasks.find((x) => x.id === 't1')!;
      t.status = 'running';
      t.progress = 0.02;
      void id;
    }),
  getUsers: () => simulate('getUsers', () => db.usersAdmin),
  updateUser: (id, patch) =>
    simulate('updateUser', () => {
      const u = db.usersAdmin.find((x) => x.id === id);
      if (!u) throw new ApiError('User not found', 404);
      Object.assign(u, patch);
      persist();
      return u;
    }),
  inviteUser: (email, libs) =>
    simulate('inviteUser', () => {
      const u = { ...db.usersAdmin[6], id: `u-${Date.now()}`, name: email.split('@')[0], email, libraries: libs, status: 'invited' as const, joinedAt: now(), avatarColor: '#5cb3e2' };
      db.usersAdmin.push(u);
      persist();
      return u;
    }),
  getTasks: () => simulate('getTasks', () => db.tasks),
  runTask: (id) =>
    simulate('runTask', () => {
      const t = db.tasks.find((x) => x.id === id);
      if (t) {
        t.status = 'running';
        t.progress = 0.01;
        t.lastRun = now();
      }
    }),
  getAdminSection: <T,>(section: string) => simulate(`admin:${section}`, () => ({ ...adminSections[section], ...(db.admin[section] ?? {}) }) as T),
  updateAdminSection: (section, patch) =>
    simulate('updateAdminSection', () => {
      db.admin[section] = { ...(db.admin[section] ?? {}), ...patch };
      persist();
    }, 0.6),
  runBackup: () => simulate('runBackup', () => undefined, 3),
  runDiagnostics: () => simulate('runDiagnostics', () => undefined, 4),
};
