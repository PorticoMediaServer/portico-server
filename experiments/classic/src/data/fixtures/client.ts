import type {PorticoClient, Topic} from '../services';
import type * as T from '../types';
import * as C from './catalog';
import * as HH from './household';
import {baseChannels, programsFor, seedRecordings} from './live';
import {prefDefs, defaultPrefs} from './prefs';
import {firstLetter, hash, latency, rng, shuffle, sortKey} from './util';

const HOUR = 3600 * 1000;
const kidsRatings = ['G', 'PG', 'TV-Y', 'TV-Y7', 'TV-PG'];

/**
 * The fixture "server": an in-memory Portico household. Every call resolves
 * after a short simulated latency, and every mutation notifies subscribers.
 */
export function createFixtureClient(): PorticoClient {
  const startedOnce = new Set<string>();
  const listeners = new Map<Topic, Set<() => void>>();
  const emit = (...topics: Topic[]) => topics.forEach(t => listeners.get(t)?.forEach(fn => fn()));

  // ── Session state ───────────────────────────────────────────────────
  let session: T.Session | undefined;
  let serverId = 'srv-lighthouse';
  let connectivity: 'online' | 'reconnecting' | 'offline' = 'online';
  const personalByProfile = new Map<string, Map<string, T.Personal>>();
  const layoutByProfile = new Map<string, T.HomeLayoutRow[]>();
  const continueHidden = new Set<string>();
  const recentSearches: string[] = ['sintel', 'keaton', 'harbor'];
  const notices = HH.seedNotices();
  let devices = [...HH.devices];
  let invitations = [...HH.invitations];
  const prefs = new Map<string, T.PrefValue>(Object.entries(defaultPrefs));
  const libraries: T.Library[] = C.catalog.libraries.map(l => ({...l, count: 0}));

  const profile = () => session?.profile ?? HH.profilesByServer['srv-lighthouse']![0]!;
  const personalMap = () => {
    const p = profile();
    let map = personalByProfile.get(p.id);
    if (!map) {
      map = HH.seedPersonal(p.id, p.kids);
      personalByProfile.set(p.id, map);
    }
    return map;
  };
  const personal = (id: string): T.Personal => personalMap().get(id) ?? {watched: false, progress: 0, watchlisted: false, favorite: false};
  const patch = (id: string, p: Partial<T.Personal>) => personalMap().set(id, {...personal(id), ...p});

  // ── Visibility (kids profile limits) ────────────────────────────────
  const allowedMovie = (m: C.RawMovie) => !profile().kids || m.library === 'lib-family' || kidsRatings.includes(m.rating);
  const allowedShow = (s: C.RawShow) => !profile().kids || kidsRatings.includes(s.rating);
  const visibleMovies = () => C.catalog.movies.filter(allowedMovie);
  const visibleShows = () => C.catalog.shows.filter(allowedShow);
  const visibleLibraries = () => libraries.filter(l => !profile().kids || l.id !== 'lib-anime').map(l => ({...l, count: itemsOf(l.id).length}));

  // ── Entry decoration ────────────────────────────────────────────────
  function decorate(e: T.Entry): T.Entry {
    const p = personal(e.id);
    const out: T.Entry = {...e, watchlisted: p.watchlisted, favorite: p.favorite};
    if (e.kind === 'show') {
      const eps = HH.allEpisodesOf(e.id);
      const watched = eps.filter(x => personal(x.id).watched).length;
      out.unwatched = eps.length - watched;
      out.watched = watched === eps.length;
      if (watched > 0 && watched < eps.length) out.progress = watched / eps.length;
    } else if (e.duration) {
      out.watched = p.watched;
      if (!p.watched && p.progress > 0) out.progress = p.progress / e.duration;
    }
    return out;
  }
  const entry = (id: string) => {
    const e = C.entryFor(id);
    return e ? decorate(e) : undefined;
  };
  const entries = (ids: string[]) => ids.map(entry).filter((e): e is T.Entry => !!e);

  function itemsOf(libraryId: string): T.Entry[] {
    switch (libraryId) {
      case 'lib-movies':
      case 'lib-family':
        return visibleMovies().filter(m => m.library === libraryId).map(C.movieEntry);
      case 'lib-tv':
      case 'lib-anime':
        return visibleShows().filter(s => s.library === libraryId).map(C.showEntry);
      case 'lib-music':
        return C.catalog.artists.map(C.artistEntry);
      case 'lib-audiobooks':
        return C.catalog.books.map(C.bookEntry);
      case 'lib-photos':
        return C.catalog.photoAlbums.map(C.photoAlbumEntry);
      case 'lib-homevideos':
        return C.catalog.homeVideos.map(C.homeVideoEntry);
    }
    return [];
  }
  function viewItems(libraryId: string, view?: T.LibraryView): T.Entry[] {
    if (libraryId === 'lib-music' && view === 'albums') return C.catalog.albums.map(C.albumEntry);
    if (libraryId === 'lib-music' && view === 'songs') return [...C.tracks.values()].map(C.trackEntry);
    if (libraryId === 'lib-audiobooks' && view === 'authors') {
      const authors = [...new Set(C.catalog.books.map(b => b.author))];
      return authors.map(a => ({id: `author:${a}`, kind: 'author' as const, title: a, subtitle: `${C.catalog.books.filter(b => b.author === a).length} books`, image: C.bookArt(C.catalog.books.find(b => b.author === a)!.id), shape: 'poster' as const, libraryId}));
    }
    if (libraryId === 'lib-audiobooks' && view === 'series') {
      const series = [...new Set(C.catalog.books.filter(b => b.series).map(b => b.series!))];
      return series.map(s => ({id: `series:${s}`, kind: 'series' as const, title: s, subtitle: `${C.catalog.books.filter(b => b.series === s).length} books`, image: C.bookArt(C.catalog.books.find(b => b.series === s)!.id), shape: 'poster' as const, libraryId}));
    }
    if (libraryId === 'lib-photos' && view === 'timeline') return [...C.photos.values()].map(({photo, album}) => ({id: photo.id, kind: 'photo' as const, title: photo.title, subtitle: album.title, image: C.photoThumb(photo.id), backdrop: C.photoArt(photo.id), shape: 'square' as const, libraryId, parentId: album.id}));
    return itemsOf(libraryId);
  }

  function applyFilters(list: T.Entry[], q: T.BrowseQuery): T.Entry[] {
    const f = q.filters;
    let out = list.map(decorate);
    if (f.genre?.length) out = out.filter(e => e.genres?.some(g => f.genre!.includes(g)));
    if (f.decade?.length) out = out.filter(e => e.year && f.decade!.includes(`${Math.floor(e.year / 10) * 10}s`));
    if (f.contentRating?.length) out = out.filter(e => e.rating && f.contentRating!.includes(e.rating));
    if (f.playState === 'unwatched') out = out.filter(e => !e.watched && !e.progress);
    if (f.playState === 'inProgress') out = out.filter(e => !!e.progress && !e.watched);
    if (f.playState === 'watched') out = out.filter(e => e.watched);
    if (f.favorite) out = out.filter(e => e.favorite);
    if (f.watchlisted) out = out.filter(e => e.watchlisted);
    if (f.studio?.length) out = out.filter(e => {
      const m = C.movies.get(e.id);
      const s = C.shows.get(e.id);
      return (m && f.studio!.includes(m.studio)) || (s && f.studio!.includes(s.network));
    });
    if (f.resolution?.length) out = out.filter(e => f.resolution!.includes(hash(e.id) % 3 === 0 ? '4K' : hash(e.id) % 3 === 1 ? '1080p' : '720p'));
    if (q.filters.quick === 'recent') out = out.filter(e => (C.addedAt.get(e.id) ?? 0) > Date.now() - 60 * 86400000);
    const dir = q.direction === 'asc' ? 1 : -1;
    const cmp: Record<T.SortField, (a: T.Entry, b: T.Entry) => number> = {
      title: (a, b) => sortKey(a.title).localeCompare(sortKey(b.title)),
      added: (a, b) => (C.addedAt.get(a.id) ?? 0) - (C.addedAt.get(b.id) ?? 0),
      year: (a, b) => (a.year ?? 0) - (b.year ?? 0) || sortKey(a.title).localeCompare(sortKey(b.title)),
      rating: (a, b) => (C.movies.get(a.id)?.audience ?? C.shows.get(a.id)?.audience ?? 0) - (C.movies.get(b.id)?.audience ?? C.shows.get(b.id)?.audience ?? 0),
      duration: (a, b) => (a.duration ?? 0) - (b.duration ?? 0),
      lastPlayed: (a, b) => (personal(a.id).lastPlayed ?? 0) - (personal(b.id).lastPlayed ?? 0),
      random: (a, b) => (hash(a.id + 'r') % 1000) - (hash(b.id + 'r') % 1000),
      artist: (a, b) => (a.subtitle ?? '').localeCompare(b.subtitle ?? ''),
      plays: (a, b) => (hash(a.id + 'p') % 500) - (hash(b.id + 'p') % 500),
    };
    return out.sort((a, b) => cmp[q.sort](a, b) * dir);
  }

  // ── Detail helpers ──────────────────────────────────────────────────
  const credit = (pid: string, role: 'cast' | 'crew', character?: string, job?: string): T.Credit => ({personId: pid, name: C.people.get(pid)?.name ?? 'Unknown', role, character, job, image: C.personArt(pid)});
  function versionsFor(id: string, real: boolean): T.MediaVersion[] {
    const h = hash(id);
    const base: T.MediaVersion = {id: `${id}-v1`, label: h % 3 === 0 ? '4K · Dolby Vision' : '1080p', resolution: h % 3 === 0 ? '4K (2160p)' : '1080p', video: h % 3 === 0 ? 'HEVC Main 10 · Dolby Vision' : 'H.264 High', audio: h % 2 ? 'English · Dolby Digital Plus 5.1' : 'English · AAC Stereo', container: h % 2 ? 'MKV' : 'MP4', size: `${(2 + (h % 90) / 10).toFixed(1)} GB`, bitrate: `${8 + (h % 30)} Mbps`, hdr: h % 3 === 0 ? 'Dolby Vision' : undefined};
    if (real || h % 4) return [base];
    return [base, {id: `${id}-v2`, label: "Director's Cut · 1080p", resolution: '1080p', video: 'H.264 High', audio: 'English · DTS-HD MA 7.1', container: 'MKV', size: '14.2 GB', bitrate: '22 Mbps', edition: "Director's Cut"}];
  }
  const audioTracks = (id: string): T.Track[] => {
    const list: T.Track[] = [{id: 'a1', label: 'English', language: 'en', codec: 'Dolby Digital Plus', channels: '5.1', default: true, origin: 'embedded'}, {id: 'a2', label: 'English · Commentary', language: 'en', codec: 'AAC', channels: 'Stereo', origin: 'embedded'}];
    if (hash(id) % 2) list.push({id: 'a3', label: 'French', language: 'fr', codec: 'AC3', channels: '5.1', origin: 'embedded'}, {id: 'a4', label: 'Spanish', language: 'es', codec: 'AC3', channels: '5.1', origin: 'embedded'});
    return list;
  };
  const subtitleTracks = (id: string): T.Track[] => [
    {id: 's1', label: 'English', language: 'en', codec: 'SRT', origin: 'sidecar'},
    {id: 's2', label: 'English (SDH)', language: 'en', codec: 'PGS', origin: 'embedded'},
    {id: 's3', label: 'English · Forced', language: 'en', codec: 'SRT', forced: true, origin: 'embedded'},
    ...(hash(id) % 2 ? [{id: 's4', label: 'French', language: 'fr', codec: 'SRT', origin: 'provider' as const}, {id: 's5', label: 'Spanish', language: 'es', codec: 'SRT', origin: 'provider' as const}] : []),
  ];
  const chaptersFor = (id: string, duration: number): T.Chapter[] => {
    const n = Math.max(4, Math.min(12, Math.round(duration / 600)));
    const names = ['Opening', 'The Arrival', 'First Night', 'A Discovery', 'Complications', 'The Plan', 'Turning Point', 'Into the Storm', 'Confrontation', 'Aftermath', 'Homeward', 'Finale'];
    return Array.from({length: n}, (_, i) => ({title: i === 0 ? 'Opening' : names[(i + hash(id)) % names.length]!, start: Math.floor((duration / n) * i), image: C.stillFor(`${id}-ch${i}`)}));
  };
  function related(e: T.Entry, pool: T.Entry[], n = 16): T.Entry[] {
    const r = rng(hash(e.id));
    const same = pool.filter(x => x.id !== e.id && x.genres?.some(g => e.genres?.includes(g)));
    return shuffle(r, same).slice(0, n).map(decorate);
  }

  // ── Home ─────────────────────────────────────────────────────────────
  const defaultRowOrder = ['continue', 'upNext', 'recent-movies', 'picks', 'because', 'recent-tv', 'watchlist', 'trending', 'live-now', 'recent-music', 'genre-for-you', 'continue-listening', 'recent-anime', 'top-rated', 'hidden-gems', 'favorites', 'recent-family'];
  function continueWatching(): T.Entry[] {
    const map = personalMap();
    return [...map.entries()]
      .filter(([id, p]) => p.progress > 0 && !p.watched && !continueHidden.has(id) && (C.movies.has(id) || C.episodes.has(id) || C.homeVideos.has(id)))
      .sort((a, b) => (b[1].lastPlayed ?? 0) - (a[1].lastPlayed ?? 0))
      .map(([id]) => {
        const e = entry(id)!;
        if (C.movies.has(id) || C.homeVideos.has(id)) return e;
        return {...e, shape: 'poster' as const, caption: `${e.subtitle} · ${e.caption}`};
      });
  }
  function nextUpFor(showId: string): T.EpisodeSummary | undefined {
    const eps = HH.allEpisodesOf(showId);
    const lastWatched = eps.map(e => personal(e.id)).reduce((idx, p, i) => (p.watched ? i : idx), -1);
    if (lastWatched < 0) return undefined;
    const next = eps[lastWatched + 1];
    return next ? (decorate(C.episodeEntry(C.episodes.get(next.id)!)) as T.EpisodeSummary) : undefined;
  }
  function upNext(): T.Entry[] {
    const out: T.Entry[] = [];
    for (const s of visibleShows()) {
      const n = nextUpFor(s.id);
      if (n && !personal(n.id).progress) out.push({...n, caption: `${s.title} · ${n.caption}`});
    }
    return out.sort((a, b) => hash(a.id) % 7 - hash(b.id) % 7).slice(0, 12);
  }
  const recentIn = (libraryId: string, n = 24): T.Entry[] => {
    if (libraryId === 'lib-tv' || libraryId === 'lib-anime') {
      const eps = [...C.episodes.values()].filter(r => r.show.library === libraryId && allowedShow(r.show) && r.season.number > 0);
      // Newest first, one card per show: a single new episode shows itself;
      // several become the show with a count, as a server groups them.
      const groups = new Map<string, typeof eps>();
      for (const r of eps.sort((a, b) => (C.addedAt.get(b.episode.id) ?? 0) - (C.addedAt.get(a.episode.id) ?? 0))) {
        const g = groups.get(r.show.id) ?? [];
        if (!groups.has(r.show.id)) groups.set(r.show.id, g);
        // A show's latest batch: episodes added within a few days of its newest one.
        const window = (1 + (hash(r.show.id) % 5)) * 86400000;
        if (!g.length || (C.addedAt.get(g[0]!.episode.id) ?? 0) - (C.addedAt.get(r.episode.id) ?? 0) < window) g.push(r);
      }
      return [...groups.values()].slice(0, n).map(g => {
        if (g.length === 1) {
          const e = decorate(C.episodeEntry(g[0]!));
          return {...e, caption: `${g[0]!.show.title} · ${e.caption}`};
        }
        return {...decorate(C.showEntry(g[0]!.show)), subtitle: `${g.length} new episodes`};
      });
    }
    if (libraryId === 'lib-music') return C.catalog.albums.slice().sort((a, b) => (C.addedAt.get(b.id) ?? 0) - (C.addedAt.get(a.id) ?? 0)).slice(0, n).map(a => decorate(C.albumEntry(a)));
    return itemsOf(libraryId).sort((a, b) => (C.addedAt.get(b.id) ?? 0) - (C.addedAt.get(a.id) ?? 0)).slice(0, n).map(decorate);
  };
  function rowById(id: string): T.HomeRow | undefined {
    const p = profile();
    const r = rng(hash(p.id + id));
    const moviesE = visibleMovies().map(C.movieEntry);
    const unwatched = moviesE.filter(e => !personal(e.id).watched);
    const mk = (kind: T.HomeRowKind, title: string, list: T.Entry[], shape: T.Shape, libraryId?: string, extra: Partial<T.HomeRow> = {}): T.HomeRow => ({id, kind, title, entries: list.slice(0, 24), total: list.length, shape, libraryId, ...extra});
    switch (id) {
      case 'continue': return mk('continue', 'Continue Watching', continueWatching(), 'poster', undefined, {required: true});
      case 'upNext': return mk('upNext', 'Up Next', upNext(), 'poster');
      case 'recent-movies': return mk('recent', 'Recently added in Movies', recentIn('lib-movies'), 'poster', 'lib-movies');
      case 'recent-tv': return p.kids ? undefined : mk('recent', 'Recently added in TV Shows', recentIn('lib-tv'), 'poster', 'lib-tv');
      case 'recent-anime': return p.kids ? undefined : mk('recent', 'Recently added in Anime', recentIn('lib-anime'), 'poster', 'lib-anime');
      case 'recent-music': return mk('recent', 'Recently added in Music', recentIn('lib-music'), 'square', 'lib-music');
      case 'recent-family': return mk('recent', 'Recently added in Family', recentIn('lib-family'), 'poster', 'lib-family');
      case 'watchlist': return mk('watchlist', 'Watchlist', [...personalMap().entries()].filter(([, v]) => v.watchlisted).map(([k]) => entry(k)).filter((e): e is T.Entry => !!e && (e.kind === 'movie' || e.kind === 'show')), 'poster');
      case 'favorites': return mk('favorites', 'Favorites', [...personalMap().entries()].filter(([, v]) => v.favorite).map(([k]) => entry(k)).filter((e): e is T.Entry => !!e && (e.kind === 'movie' || e.kind === 'show')), 'poster');
      case 'picks': return mk('picks', 'Picks for you', shuffle(r, unwatched).slice(0, 30).map(e => ({...decorate(e), recommendation: true})).filter(e => !personal(e.id).notInterested), 'poster', undefined, {reason: 'Based on what you watch and rate'});
      case 'because': {
        const seed = [...personalMap().entries()].filter(([k, v]) => v.watched && C.movies.has(k)).sort((a, b) => (b[1].lastPlayed ?? 0) - (a[1].lastPlayed ?? 0))[0];
        if (!seed) return undefined;
        const src = entry(seed[0])!;
        return mk('because', `Because you watched ${src.title}`, related(src, unwatched, 24).map(e => ({...e, recommendation: true})).filter(e => !personal(e.id).notInterested), 'poster');
      }
      case 'trending': return mk('trending', 'Trending now', shuffle(rng(hash('trend')), [...moviesE, ...visibleShows().map(C.showEntry)]).slice(0, 24).map(e => ({...decorate(e), recommendation: true})), 'poster');
      case 'genre-for-you': {
        const g = p.kids ? 'Family' : 'Thriller';
        return mk('picks', `${g} for you`, unwatched.filter(e => e.genres?.includes(g)).slice(0, 24).map(e => ({...decorate(e), recommendation: true})), 'poster');
      }
      case 'top-rated': return mk('discover', 'Top rated you haven’t seen', unwatched.sort((a, b) => (C.movies.get(b.id)!.audience) - (C.movies.get(a.id)!.audience)).slice(0, 24).map(decorate), 'poster');
      case 'hidden-gems': return mk('picks', 'Hidden gems for you', shuffle(r, unwatched.filter(e => (C.movies.get(e.id)?.critic ?? 0) > 85)).slice(0, 24).map(e => ({...decorate(e), recommendation: true})), 'poster');
      case 'continue-listening': return mk('continue', 'Continue Listening', [...personalMap().entries()].filter(([k, v]) => C.books.has(k) && v.progress > 0).map(([k]) => entry(k)!).filter(Boolean), 'poster');
      case 'live-now': {
        const now = Date.now();
        const list = baseChannels().filter(c => !c.hidden).slice(0, 14).map(c => {
          const prog = programsFor(C.channelsRaw.find(x => x.id === c.id)!, now, now + 1)[0];
          return {id: c.id, kind: 'channel' as const, title: prog?.title ?? c.name, subtitle: `${c.number} · ${c.name}`, image: prog?.image, shape: 'wide' as const, playable: true, caption: c.name, progress: prog ? (now - prog.start) / (prog.end - prog.start) : undefined, badge: 'LIVE'};
        });
        return mk('live', 'On now', list, 'wide');
      }
    }
    if (id.startsWith('view:')) {
      const v = savedResources.find(x => x.id === id.slice(5));
      if (!v) return undefined;
      return mk('view', v.title, entries(resourceItems(v.id)), 'poster', v.libraryId);
    }
    return undefined;
  }
  function layout(): T.HomeLayoutRow[] {
    const p = profile();
    let l = layoutByProfile.get(p.id);
    if (!l) {
      l = defaultRowOrder.map(id => ({id, title: rowTitle(id), hidden: ['hidden-gems', 'favorites', 'recent-family', 'top-rated'].includes(id), required: id === 'continue'}));
      layoutByProfile.set(p.id, l);
    }
    return l;
  }
  const rowTitle = (id: string) => ({continue: 'Continue Watching', upNext: 'Up Next', 'recent-movies': 'Recently added in Movies', picks: 'Picks for you', because: 'Because you watched…', 'recent-tv': 'Recently added in TV Shows', watchlist: 'Watchlist', trending: 'Trending now', 'live-now': 'On now', 'recent-music': 'Recently added in Music', 'genre-for-you': 'Thriller for you', 'continue-listening': 'Continue Listening', 'recent-anime': 'Recently added in Anime', 'top-rated': 'Top rated you haven’t seen', 'hidden-gems': 'Hidden gems for you', favorites: 'Favorites', 'recent-family': 'Recently added in Family'} as Record<string, string>)[id] ?? savedResources.find(x => `view:${x.id}` === id)?.title ?? id;

  // ── Saved resources ─────────────────────────────────────────────────
  type Res = T.SavedResource & {items: string[]; owner: string};
  const pickTracks = (n: number, seed: string) => shuffle(rng(hash(seed)), [...C.tracks.keys()]).slice(0, n);
  const savedResources: Res[] = [
    {id: 'pl-road', kind: 'playlist', title: 'Road Trip Mix', count: 0, shared: false, items: pickTracks(24, 'road'), owner: 'Justin', description: 'Windows down.'},
    {id: 'pl-sunday', kind: 'playlist', title: 'Sunday Morning', count: 0, shared: true, items: pickTracks(18, 'sunday'), owner: 'Justin', description: 'Coffee, slow songs.'},
    {id: 'pl-noir', kind: 'playlist', title: 'Noir Night', count: 0, shared: false, items: C.collections.get('col-noir')!.items, owner: 'Justin'},
    {id: 'pl-focus', kind: 'playlist', title: 'Deep Focus', count: 0, shared: false, items: pickTracks(30, 'focus'), owner: 'Justin'},
    {id: 'pl-kids', kind: 'playlist', title: 'Kids Favorites', count: 0, shared: true, items: C.catalog.movies.filter(m => m.library === 'lib-family').slice(0, 10).map(m => m.id), owner: 'Maya'},
    {id: 'pc-date', kind: 'collection', title: 'Date Night', count: 0, shared: false, items: C.catalog.movies.filter(m => m.genres.includes('Romance')).slice(0, 12).map(m => m.id), owner: 'Justin', description: 'Films for two.'},
    {id: 'pc-rainy', kind: 'collection', title: 'Rainy Day Films', count: 0, shared: true, items: shuffle(rng(1), C.catalog.movies).slice(0, 14).map(m => m.id), owner: 'Justin'},
    {id: 'pc-silent', kind: 'collection', title: 'Silent Era Starter Pack', count: 0, shared: true, items: C.catalog.movies.filter(m => m.year < 1930).map(m => m.id), owner: 'Maya'},
    {id: 'sv-unwatched-noir', kind: 'view', title: 'Unwatched Crime', count: 0, shared: false, items: [], owner: 'Justin', libraryId: 'lib-movies', smart: true},
    {id: 'sv-shorts', kind: 'view', title: 'Under 20 Minutes', count: 0, shared: false, items: [], owner: 'Justin', libraryId: 'lib-family', smart: true},
    {id: 'sv-80s', kind: 'view', title: '80s and 90s Thrillers', count: 0, shared: false, items: [], owner: 'Justin', libraryId: 'lib-movies', smart: true},
  ];
  const viewQueries: Record<string, (e: T.Entry) => boolean> = {
    'sv-unwatched-noir': e => !!e.genres?.includes('Crime') && !personal(e.id).watched,
    'sv-shorts': e => (e.duration ?? 9999) <= 20 * 60,
    'sv-80s': e => !!e.year && e.year >= 1980 && e.year < 2000 && !!e.genres?.includes('Thriller'),
  };
  function resourceItems(id: string): string[] {
    const r = savedResources.find(x => x.id === id);
    if (!r) return [];
    if (r.kind === 'view') {
      const q = viewQueries[id] ?? (() => true);
      return itemsOf(r.libraryId ?? 'lib-movies').filter(q).map(e => e.id);
    }
    return r.items;
  }
  const resourceSummary = (r: Res): T.SavedResource => {
    const items = resourceItems(r.id);
    const imgs = items.slice(0, 4).map(id => C.posterFor(id) ?? C.entryFor(id)?.image);
    return {id: r.id, kind: r.kind, title: r.title, count: items.length, image: imgs[0], images: imgs, shared: r.shared, description: r.description, libraryId: r.libraryId, smart: r.smart};
  };

  // ── Live, DVR, Together, Downloads ──────────────────────────────────
  const channels = baseChannels();
  const reminders = new Set<string>();
  const seeded = seedRecordings(Date.now());
  let recordings = seeded.recordings;
  let rules = seeded.rules;
  const scheduledPrograms = new Map<string, 'scheduled' | 'series'>();
  let group: T.WatchGroup | undefined;
  const otherGroups: T.WatchGroup[] = [{id: 'grp-friday', name: 'Friday Movie Night', code: 'HRBR 7QXT', state: 'lobby', control: 'host-only', members: [{id: 'm1', name: 'Maya', art: 'violet', host: true, you: false, status: 'inStep'}, {id: 'm2', name: 'Nora', art: 'mint', host: false, you: false, status: 'away'}], queue: entries(['m-charade-1963', 'm-his-girl-friday-1940']), codeExpires: Date.now() + 15 * 60000}];
  let downloads: T.Download[] = ([
    {id: 'dl-1', entry: entry('m-sintel-2010')!, quality: 'Original', size: '1.2 GB', state: 'ready', progress: 1, expires: 'Oct 26'},
    {id: 'dl-2', entry: entry('m-metropolis-1927')!, quality: '1080p', size: '2.4 GB', state: 'ready', progress: 1, expires: 'Oct 22', watched: true},
    {id: 'dl-3', entry: entry(C.catalog.shows[0]!.seasons[0]!.episodes[0]!.id)!, quality: '720p', size: '640 MB', state: 'downloading', progress: 0.42},
    {id: 'dl-4', entry: entry(C.catalog.shows[0]!.seasons[0]!.episodes[1]!.id)!, quality: '720p', size: '655 MB', state: 'preparing', progress: 0.18},
    {id: 'dl-5', entry: entry('m-big-buck-bunny-2008')!, quality: 'Smaller', size: '210 MB', state: 'paused', progress: 0.6},
    {id: 'dl-6', entry: entry('m-the-general-1926')!, quality: '1080p', size: '1.9 GB', state: 'failed', progress: 0.3},
  ] as T.Download[]).filter(d => d.entry);
  let downloadRequests: T.DownloadRequest[] = [{id: 'rq-1', show: entry(C.catalog.shows[0]!.id)!, policy: 'next', keep: 3, ready: 1, total: 3, state: 'active'}];
  let downloadSettings: T.DownloadSettings = {wifiOnly: true, removeAfterWatching: false, nextEpisode: true, limitGB: 32, quality: '1080p'};
  // Downloads in flight advance while the app runs.
  setInterval(() => {
    let changed = false;
    downloads = downloads.map(d => {
      if (d.state === 'downloading' || d.state === 'preparing') {
        changed = true;
        const p = Math.min(1, d.progress + 0.01);
        return {...d, progress: p, state: p >= 1 ? 'ready' : d.state === 'preparing' && p > 0.35 ? 'downloading' : d.state};
      }
      return d;
    });
    if (changed) emit('downloads');
  }, 3000);

  const client: PorticoClient = {
    subscribe(topic, fn) {
      let set = listeners.get(topic);
      if (!set) listeners.set(topic, (set = new Set()));
      set.add(fn);
      return () => set!.delete(fn);
    },

    session: {
      current: () => session,
      async servers() { await latency(); return HH.servers; },
      async profiles(id) { await latency(); return HH.profilesByServer[id ?? serverId] ?? []; },
      async signInAccount(identity, password) {
        await latency(500);
        if (!identity || password.length < 4) throw new Error('That username or email and password didn’t match a Portico Account. Check them and try again.');
        return {twoStep: HH.account.twoStep};
      },
      async verifyCode(code) {
        await latency(300);
        if (!/^\d{6}$/.test(code.replace(/\s/g, '')) && !/^[a-z0-9]{4}-[a-z0-9]{4}$/i.test(code)) throw new Error('That code didn’t work. Enter the current code from your authenticator app, or a recovery code.');
      },
      async signInDirect(host, username, password) {
        await latency(500);
        if (!host.includes('.') && !host.includes(':')) throw new Error('That isn’t a server address Portico can use. Check it and try again. It usually looks like 192.168.1.20:32500.');
        if (!username || password.length < 4) throw new Error('That username and password didn’t match an account on your server. Check them and try again.');
        return {twoStep: false, mustChangePassword: false};
      },
      async chooseServer(id) {
        await latency(400);
        const s = HH.servers.find(x => x.id === id)!;
        if (s.status === 'online') serverId = id;
        return {status: s.status};
      },
      async chooseProfile(profileId, pin) {
        await latency(250);
        const p = (HH.profilesByServer[serverId] ?? []).find(x => x.id === profileId)!;
        if (p.pin && pin !== HH.PIN) throw new Error('That PIN isn’t right. Try again.');
        session = {server: HH.servers.find(s => s.id === serverId)!, profile: p, account: HH.account};
        emit('session', 'home', 'personal', 'saved', 'libraries', 'notifications');
      },
      async signOut() { await latency(); session = undefined; emit('session'); },
      async createAccount() { await latency(600); },
      async requestPasswordReset() { await latency(500); },
      async resetPassword(pw) { await latency(500); if (pw.length < 8) throw new Error('Use at least 8 characters.'); },
      async verifyEmail(token) { await latency(700); return token === 'expired' ? 'expired' : 'confirmed'; },
      async activationCode() { await latency(); return {code: 'K7QD-2MXF', url: 'getportico.tv/link', expiresIn: 600}; },
      async pollActivation() { await latency(); return 'waiting'; },
      async approveDevice(code) {
        await latency(500);
        if (code.replace(/[^a-z0-9]/gi, '').length < 8) throw new Error('That code wasn’t found. Check it on the TV and try again.');
        return {device: 'Living Room Apple TV', platform: 'Apple TV · tvOS 26', location: 'Home network · Portland, OR'};
      },
      async confirmDevice() { await latency(400); },
      async invitation(code) { await latency(500); return code === 'expired' ? {server: 'Lighthouse', inviter: 'Maya Ehler', libraries: 4, state: 'expired'} : {server: 'Lighthouse', inviter: 'Maya Ehler', libraries: 4, state: 'valid'}; },
      async acceptInvitation() { await latency(600); },
      async declineInvitation() { await latency(400); },
      async claimRequest() { await latency(500); return {server: 'Studio Annex', address: 'https://192.168.1.44:32500', requested: `Today at ${new Date(Date.now() - 4 * 60000).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}`, state: 'waiting'}; },
      async answerClaim() { await latency(700); },
      async setupServer() { await latency(900); },
      async restore() { await latency(900); return 'restored'; },
      connectivity: () => connectivity,
      setConnectivity(s) { connectivity = s; emit('session'); },
    },

    libraries: {
      async list() { await latency(80); return visibleLibraries(); },
      async discover(libraryId) {
        await latency();
        const items = itemsOf(libraryId).map(decorate);
        const r = rng(hash(libraryId + profile().id));
        const rows: T.HomeRow[] = [];
        const shape: T.Shape = libraryId === 'lib-music' ? 'circle' : libraryId === 'lib-photos' ? 'square' : libraryId === 'lib-homevideos' ? 'wide' : 'poster';
        const add = (id: string, title: string, list: T.Entry[], s: T.Shape = shape, kind: T.HomeRowKind = 'discover') => list.length && rows.push({id, kind, title, entries: list.slice(0, 24), total: list.length, shape: s, libraryId});
        if (libraryId === 'lib-music') {
          add('recent-albums', 'Recently added', recentIn('lib-music'), 'square', 'recent');
          add('pick-albums', 'Picks for you', shuffle(r, C.catalog.albums).slice(0, 20).map(a => decorate(C.albumEntry(a))), 'square', 'picks');
          add('artists', 'Artists', shuffle(r, items), 'circle');
          add('played', 'Recently played', shuffle(rng(3), C.catalog.albums).slice(0, 14).map(a => decorate(C.albumEntry(a))), 'square');
          add('playlists', 'Playlists', savedResources.filter(x => x.kind === 'playlist' && x.items.some(i => C.tracks.has(i))).map(x => ({id: x.id, kind: 'playlist' as const, title: x.title, subtitle: `${x.items.length} songs`, image: C.albumArt(C.tracks.get(x.items[0]!)!.album.id), shape: 'square' as const})), 'square');
          const genres = [...new Set(C.catalog.albums.map(a => a.genre))].slice(0, 4);
          genres.forEach(g => add(`g-${g}`, g, C.catalog.albums.filter(a => a.genre === g).map(a => decorate(C.albumEntry(a))), 'square'));
          return rows;
        }
        if (libraryId === 'lib-audiobooks') {
          add('continue', 'Continue Listening', items.filter(e => e.progress), 'poster', 'continue');
          add('recent', 'Recently added', recentIn(libraryId), 'poster', 'recent');
          ['Classics', 'Adventure', 'Mystery', 'Science Fiction', 'Children', 'Horror'].forEach(g => add(`g-${g}`, g, items.filter(e => e.genres?.includes(g))));
          return rows;
        }
        if (libraryId === 'lib-photos') {
          C.catalog.photoAlbums.forEach(a => add(a.id, a.title, a.photos.map(ph => ({id: ph.id, kind: 'photo' as const, title: ph.title, subtitle: a.title, image: C.photoThumb(ph.id), backdrop: C.photoArt(ph.id), shape: 'square' as const, parentId: a.id, libraryId})), 'square'));
          return rows;
        }
        if (libraryId === 'lib-homevideos') {
          add('recent', 'Recently added', items.slice().sort((a, b) => (b.year ?? 0) - (a.year ?? 0)), 'wide', 'recent');
          const byYear = [...new Set(items.map(e => e.year))].sort().reverse();
          byYear.slice(0, 5).forEach(y => add(`y-${y}`, String(y), items.filter(e => e.year === y), 'wide'));
          return rows;
        }
        const isTV = libraryId === 'lib-tv' || libraryId === 'lib-anime';
        add('continue', 'Continue Watching', continueWatching().filter(e => e.libraryId === libraryId), 'poster', 'continue');
        if (isTV) add('upnext', 'Up Next', upNext().filter(e => e.libraryId === libraryId), 'poster', 'upNext');
        add('recent', isTV ? 'Recently added episodes' : 'Recently added', recentIn(libraryId), 'poster', 'recent');
        add('released', 'Recently released', items.slice().sort((a, b) => (b.year ?? 0) - (a.year ?? 0)));
        add('picks', 'Picks for you', shuffle(r, items.filter(e => !e.watched)).map(e => ({...e, recommendation: true})), 'poster', 'picks');
        add('top', 'Top rated', items.slice().sort((a, b) => ((C.movies.get(b.id)?.audience ?? C.shows.get(b.id)?.audience ?? 0) - (C.movies.get(a.id)?.audience ?? C.shows.get(a.id)?.audience ?? 0))));
        const cols = C.catalog.collections.filter(c => c.library === libraryId).map(C.collectionEntry);
        add('collections', 'Collections', cols, 'poster');
        const genres = [...new Set(items.flatMap(e => e.genres ?? []))].sort((a, b) => items.filter(e => e.genres?.includes(b)).length - items.filter(e => e.genres?.includes(a)).length).slice(0, 6);
        genres.forEach(g => add(`g-${g}`, g, shuffle(r, items.filter(e => e.genres?.includes(g)))));
        return rows;
      },
      async browse(libraryId, query, offset, limit, view) {
        await latency(90);
        const all = applyFilters(viewItems(libraryId, view), query);
        return {entries: all.slice(offset, offset + limit), total: all.length, offset};
      },
      async letters(libraryId, query, view) {
        const all = applyFilters(viewItems(libraryId, view), {...query, sort: 'title', direction: 'asc'});
        const out: T.LetterIndex = [];
        all.forEach((e, i) => {
          const l = firstLetter(e.title);
          if (!out.find(x => x.letter === l)) out.push({letter: l, offset: i});
        });
        return out;
      },
      async facets(libraryId) {
        await latency(60);
        const items = itemsOf(libraryId);
        const count = (f: (e: T.Entry) => string[]) => {
          const m = new Map<string, number>();
          items.forEach(e => f(e).forEach(v => m.set(v, (m.get(v) ?? 0) + 1)));
          return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([value, n]) => ({value, label: value, count: n}));
        };
        const facets: T.Facet[] = [
          {field: 'genre', label: 'Genre', values: count(e => e.genres ?? [])},
          {field: 'decade', label: 'Decade', values: count(e => (e.year ? [`${Math.floor(e.year / 10) * 10}s`] : [])).sort((a, b) => b.value.localeCompare(a.value))},
        ];
        if (['lib-movies', 'lib-family', 'lib-tv', 'lib-anime'].includes(libraryId)) {
          facets.push({field: 'contentRating', label: 'Content rating', values: count(e => (e.rating ? [e.rating] : []))});
          facets.push({field: 'studio', label: libraryId.includes('tv') || libraryId.includes('anime') ? 'Network' : 'Studio', values: count(e => [C.movies.get(e.id)?.studio ?? C.shows.get(e.id)?.network ?? ''].filter(Boolean))});
          facets.push({field: 'resolution', label: 'Resolution', values: [{value: '4K', label: '4K', count: 0}, {value: '1080p', label: '1080p', count: 0}, {value: '720p', label: '720p', count: 0}]});
        }
        return facets;
      },
      async collections(libraryId) {
        await latency();
        return C.catalog.collections.filter(c => c.library === libraryId).map(C.collectionEntry);
      },
      async categories(libraryId) {
        await latency();
        const items = itemsOf(libraryId);
        const cats: T.Category[] = [];
        const add = (kind: T.Category['kind'], title: string, list: T.Entry[]) => list.length && cats.push({id: `${kind}:${title}`, kind, title, count: list.length, images: list.slice(0, 3).map(e => e.backdrop ?? e.image)});
        [...new Set(items.flatMap(e => e.genres ?? []))].sort().forEach(g => add('genre', g, items.filter(e => e.genres?.includes(g))));
        [...new Set(items.map(e => e.year && `${Math.floor(e.year / 10) * 10}s`).filter(Boolean) as string[])].sort().reverse().forEach(d => add('decade', d, items.filter(e => e.year && `${Math.floor(e.year / 10) * 10}s` === d)));
        if (libraryId === 'lib-movies' || libraryId === 'lib-family') [...new Set(items.map(e => C.movies.get(e.id)?.studio).filter(Boolean) as string[])].sort().forEach(s => add('studio', s, items.filter(e => C.movies.get(e.id)?.studio === s)));
        if (libraryId === 'lib-tv' || libraryId === 'lib-anime') [...new Set(items.map(e => C.shows.get(e.id)?.network).filter(Boolean) as string[])].sort().forEach(s => add('network', s, items.filter(e => C.shows.get(e.id)?.network === s)));
        return cats;
      },
      async entity(libraryId, kind, id) {
        await latency();
        const items = itemsOf(libraryId).map(decorate);
        const shape: T.Shape = libraryId === 'lib-music' ? 'square' : 'poster';
        if (kind === 'collection') {
          const c = C.collections.get(id)!;
          return {title: c.name, subtitle: `Collection · ${c.items.length} titles`, overview: c.overview, image: C.posterFor(c.items[0]!), backdrop: C.backdropFor(c.items[0]!), entries: entries(c.items), shape: 'poster'};
        }
        if (kind === 'author') return {title: id, subtitle: 'Author', entries: C.catalog.books.filter(b => b.author === id).map(b => decorate(C.bookEntry(b))), shape: 'poster'};
        if (kind === 'series') return {title: id, subtitle: 'Series', entries: C.catalog.books.filter(b => b.series === id).sort((a, b) => (a.seriesIndex ?? 0) - (b.seriesIndex ?? 0)).map(b => decorate(C.bookEntry(b))), shape: 'poster'};
        const match = (e: T.Entry) => kind === 'genre' ? e.genres?.includes(id) : kind === 'decade' ? e.year && `${Math.floor(e.year / 10) * 10}s` === id : kind === 'studio' ? C.movies.get(e.id)?.studio === id : kind === 'network' ? C.shows.get(e.id)?.network === id : kind === 'rating' ? e.rating === id : false;
        const list = items.filter(e => match(e));
        const label = {genre: 'Genre', decade: 'Decade', studio: 'Studio', network: 'Network', rating: 'Rating'}[kind] ?? 'Category';
        return {title: id, subtitle: `${label} · ${list.length} titles`, backdrop: list[0]?.backdrop, entries: list, shape};
      },
      async scan(libraryId) {
        await latency();
        const lib = libraries.find(l => l.id === libraryId)!;
        lib.scanning = {found: 0};
        emit('libraries');
        let n = 0;
        const t = setInterval(() => {
          n += 37;
          lib.scanning = {found: n};
          emit('libraries');
          if (n > 400) {
            clearInterval(t);
            lib.scanning = undefined;
            lib.lastScan = 'Just now';
            emit('libraries');
          }
        }, 900);
      },
    },

    home: {
      async document() {
        await latency();
        const l = layout();
        const rows = l.filter(x => !x.hidden).map(x => rowById(x.id)).filter((r): r is T.HomeRow => !!r && r.entries.length > 0);
        const featured = [...(rowById('picks')?.entries ?? []).slice(0, 3), ...(rowById('recent-movies')?.entries ?? []).slice(0, 2), ...(profile().kids ? [] : visibleShows().slice(2, 3).map(s => decorate(C.showEntry(s))))].filter(e => e.backdrop).slice(0, 6);
        return {featured, rows};
      },
      async row(id) { await latency(); const r = rowById(id); if (!r) throw new Error('This isn’t available anymore.'); return {...r, entries: r.id === 'picks' || r.kind === 'trending' ? r.entries.concat() : r.entries}; },
      async layout() { await latency(); return layout().map(r => ({...r, title: rowTitle(r.id)})); },
      async saveLayout(rows) { await latency(300); layoutByProfile.set(profile().id, rows); emit('home'); },
      async resetLayout() { await latency(300); layoutByProfile.delete(profile().id); emit('home'); },
      async removeFromContinue(id) { await latency(200); continueHidden.add(id); emit('home'); },
      async restoreToContinue(id) { await latency(200); continueHidden.delete(id); emit('home'); },
    },

    catalog: {
      async movie(id) {
        await latency();
        const m = C.movies.get(id);
        if (!m) throw new Error('This isn’t available anymore. It may have been removed, or your access may have changed.');
        const e = decorate(C.movieEntry(m));
        const pool = visibleMovies().map(C.movieEntry);
        const dur = m.runtime * 60;
        const extras: T.Entry[] = m.real && m.studio !== 'Blender Studio' ? [] : [
          {id: `${id}-trailer`, kind: 'movie', title: 'Official Trailer', subtitle: 'Trailer · 2 min', image: C.stillFor(`${id}-x1`), shape: 'wide', duration: 142, playable: true},
          {id: `${id}-bts`, kind: 'movie', title: 'Making Of', subtitle: 'Behind the scenes · 14 min', image: C.stillFor(`${id}-x2`), shape: 'wide', duration: 840, playable: true},
          {id: `${id}-scene`, kind: 'movie', title: 'Deleted Scene', subtitle: 'Scene · 3 min', image: C.stillFor(`${id}-x3`), shape: 'wide', duration: 190, playable: true},
        ];
        return {
          entry: e, tagline: m.tagline ?? undefined, studio: m.studio, audienceRating: m.audience, criticRating: m.critic,
          directors: m.directors.map(d => credit(d, 'crew', undefined, 'Director')), writers: m.writers.map(w => credit(w, 'crew', undefined, 'Writer')),
          cast: m.cast.map(c => credit(c.person, 'cast', c.character)),
          versions: versionsFor(id, m.real), audio: audioTracks(id), subtitles: subtitleTracks(id), chapters: chaptersFor(id, dur), extras,
          related: related(e, pool), collections: (m.collections ?? []).map(cid => ({id: cid, name: C.collections.get(cid)!.name})), personal: personal(id), real: m.real, license: m.license,
        };
      },
      async homeVideo(id) {
        await latency();
        const v = C.homeVideos.get(id)!;
        const e = decorate(C.homeVideoEntry(v));
        return {entry: e, directors: [], writers: [], cast: [], versions: versionsFor(id, true), audio: audioTracks(id).slice(0, 1), subtitles: [], chapters: chaptersFor(id, v.runtime * 60), extras: [], related: C.catalog.homeVideos.filter(x => x.id !== id).slice(0, 10).map(x => decorate(C.homeVideoEntry(x))), collections: [], personal: personal(id), real: false};
      },
      async show(id) {
        await latency();
        const s = C.shows.get(id);
        if (!s) throw new Error('This isn’t available anymore.');
        const e = decorate(C.showEntry(s));
        const seasons: T.Season[] = s.seasons.map(se => {
          const eps = se.episodes.map(ep => decorate(C.episodeEntry(C.episodes.get(ep.id)!)) as T.EpisodeSummary);
          return {id: `${id}-season-${se.number}`, number: se.number, title: se.title, year: se.year, episodes: eps, image: C.posterFor(id), watchedCount: eps.filter(x => x.watched).length};
        });
        const all = seasons.flatMap(x => x.episodes).filter(x => x.season > 0);
        const next = nextUpFor(id) ?? (all.find(x => x.progress) as T.EpisodeSummary | undefined);
        return {entry: e, network: s.network, endYear: s.endYear ?? undefined, audienceRating: s.audience, creators: s.creators.map(c => credit(c, 'crew', undefined, 'Creator')), cast: s.cast.map(c => credit(c.person, 'cast', c.character)), seasons, nextUp: next, related: related(e, visibleShows().map(C.showEntry), 14), personal: personal(id), watchedCount: all.filter(x => x.watched).length, episodeCount: all.length};
      },
      async episode(id) {
        await latency();
        const ref = C.episodes.get(id)!;
        const e = decorate(C.episodeEntry(ref)) as T.EpisodeSummary;
        const list = ref.show.seasons.flatMap(se => se.episodes);
        const i = list.findIndex(x => x.id === id);
        const dur = ref.episode.runtime * 60;
        const summary = (x?: C.RawEpisode) => (x ? (decorate(C.episodeEntry(C.episodes.get(x.id)!)) as T.EpisodeSummary) : undefined);
        const cast = ref.show.cast.slice(0, 5).map(c => credit(c.person, 'cast', c.character));
        return {entry: e, show: decorate(C.showEntry(ref.show)), seasonTitle: ref.season.title, directors: [credit(ref.show.creators[0]!, 'crew', undefined, 'Director')], writers: [credit(ref.show.creators[0]!, 'crew', undefined, 'Writer')], cast, versions: versionsFor(id, false), previous: summary(list[i - 1]), next: summary(list[i + 1]), personal: personal(id), markers: [{kind: 'recap', start: 5, end: 40}, {kind: 'intro', start: 62, end: 118}, {kind: 'credits', start: dur - 70, end: dur}]};
      },
      async person(id) {
        await latency();
        const p = C.people.get(id)!;
        const credits = p.credits.map(c => ({entry: entry(c.item)!, role: c.role, character: c.character, job: c.job})).filter(c => c.entry && (!profile().kids || c.entry.libraryId === 'lib-family' || kidsRatings.includes(c.entry.rating ?? '')));
        const knownFor = credits.map(c => c.entry).filter((e, i, a) => a.findIndex(x => x.id === e.id) === i).sort((a, b) => (C.movies.get(b.id)?.audience ?? 0) - (C.movies.get(a.id)?.audience ?? 0)).slice(0, 8);
        return {id, name: p.name, image: C.personArt(id), realPhoto: C.isRealPhoto(id), born: p.born, died: p.died, bio: p.bio, knownFor, credits: credits.sort((a, b) => (b.entry.year ?? 0) - (a.entry.year ?? 0))};
      },
      async artist(id) {
        await latency();
        const a = C.artists.get(id)!;
        const albumEntries = a.albums.map(al => decorate(C.albumEntry(C.albums.get(al)!))).sort((x, y) => (y.year ?? 0) - (x.year ?? 0));
        const popular = a.albums.flatMap(al => C.albums.get(al)!.tracks.map(t => t.id)).sort((x, y) => hash(x) % 100 - hash(y) % 100).slice(0, 8);
        return {entry: decorate(C.artistEntry(a)), bio: a.bio, genre: a.genre, albums: albumEntries, popular: client.catalog.tracks(popular), similar: C.catalog.artists.filter(x => x.genre === a.genre && x.id !== id).slice(0, 10).map(C.artistEntry), personal: personal(id)};
      },
      async album(id) {
        await latency();
        const a = C.albums.get(id)!;
        const artist = C.artists.get(a.artist)!;
        return {entry: decorate(C.albumEntry(a)), artist: C.artistEntry(artist), genre: a.genre, label: a.label, type: a.type, tracks: client.catalog.tracks(a.tracks.map(t => t.id)), totalDuration: a.tracks.reduce((n, t) => n + t.duration, 0), more: artist.albums.filter(x => x !== id).map(x => decorate(C.albumEntry(C.albums.get(x)!))), personal: personal(id)};
      },
      async audiobook(id) {
        await latency();
        const b = C.books.get(id)!;
        let start = 0;
        const chapters = b.chapters.map(c => { const out = {...c, start}; start += c.duration; return out; });
        return {entry: decorate(C.bookEntry(b)), author: b.author, narrator: b.narrator, genre: b.genre, publisher: b.publisher, series: b.series ? {name: b.series, index: b.seriesIndex ?? 1} : undefined, chapters, progress: personal(id).progress, personal: personal(id), more: C.catalog.books.filter(x => x.author === b.author && x.id !== id).map(x => decorate(C.bookEntry(x)))};
      },
      async photoAlbum(id) {
        await latency();
        const a = C.photoAlbums.get(id)!;
        return {entry: C.photoAlbumEntry(a), photos: a.photos.map(ph => ({id: ph.id, title: ph.title, taken: ph.taken, camera: ph.camera, portrait: ph.portrait, image: C.photoArt(ph.id), thumb: C.photoThumb(ph.id), albumId: id}))};
      },
      entry: id => entry(id),
      tracks: ids => ids.map(tid => {
        const ref = C.tracks.get(tid)!;
        return {id: tid, number: ref.track.number, disc: ref.track.disc, title: ref.track.title, duration: ref.track.duration, artist: ref.artist.name, artistId: ref.artist.id, album: ref.album.title, albumId: ref.album.id, image: C.albumArt(ref.album.id), hasLyrics: !!ref.track.lyrics, lyrics: ref.track.lyrics ?? undefined};
      }),
      async matchCandidates(id, query) {
        await latency(600);
        const e = C.entryFor(id);
        const t = query || e?.title || '';
        const pool = C.catalog.movies.filter(m => sortKey(m.title).includes(sortKey(t).slice(0, 3)));
        const list = [e && {id: `tmdb-${hash(id) % 99999}`, title: e.title, year: e.year ?? 2020, provider: 'TMDB', score: 98, image: e.image}, ...pool.slice(0, 5).map((m, i) => ({id: `tmdb-${hash(m.id) % 99999}`, title: m.title, year: m.year, provider: i % 2 ? 'TMDB' : 'TVDB', score: 80 - i * 9, image: C.posterFor(m.id)}))].filter(Boolean) as {id: string; title: string; year: number; provider: string; score: number; image?: T.Img}[];
        return list.filter((x, i, a) => a.findIndex(y => y.title === x.title && y.year === x.year) === i);
      },
      async metadata(id) {
        await latency();
        const m = C.movies.get(id);
        const s = C.shows.get(id);
        const e = C.entryFor(id);
        return {title: e?.title ?? '', sortTitle: sortKey(e?.title ?? ''), originalTitle: e?.title ?? '', year: String(e?.year ?? ''), contentRating: e?.rating ?? '', studio: m?.studio ?? s?.network ?? '', tagline: m?.tagline ?? '', summary: e?.overview ?? '', genres: (e?.genres ?? []).join(', '), releaseDate: `${e?.year ?? 2020}-06-12`};
      },
      async saveMetadata() { await latency(500); emit('personal'); },
      async fixMatch() { await latency(800); emit('personal'); },
      async refreshMetadata() { await latency(300); },
      async deleteMedia() { await latency(700); },
    },

    personal: {
      get: id => personal(id),
      async setWatched(ids, watched) {
        await latency(120);
        const expand = (id: string) => (C.shows.has(id) ? HH.allEpisodesOf(id).map(e => e.id) : [id]);
        ids.flatMap(expand).forEach(id => patch(id, {watched, progress: 0, lastPlayed: watched ? Date.now() : personal(id).lastPlayed}));
        emit('personal', 'home', 'saved');
      },
      async setWatchlist(ids, on) { await latency(120); ids.forEach(id => patch(id, {watchlisted: on})); emit('personal', 'home', 'saved'); },
      async setFavorite(ids, on) { await latency(120); ids.forEach(id => patch(id, {favorite: on})); emit('personal', 'home', 'saved'); },
      async rate(id, stars) { await latency(120); patch(id, {rating: stars}); emit('personal'); },
      async react(id, reaction) { await latency(120); patch(id, {reaction}); emit('personal'); },
      async notInterested(id, on) { await latency(150); patch(id, {notInterested: on}); emit('personal', 'home'); },
      setProgress(id, seconds) {
        const dur = C.durationOf(id);
        const watched = dur > 0 && seconds / dur > 0.92;
        patch(id, {progress: watched ? 0 : seconds, watched: watched || personal(id).watched, lastPlayed: Date.now()});
        continueHidden.delete(id);
        emit('personal', 'home');
      },
    },

    search: {
      async query(q, groupId) {
        await latency(160);
        const s = q.trim().toLowerCase();
        const has = (t: string) => t.toLowerCase().includes(s);
        const cap = groupId ? 200 : 12;
        const groups: T.SearchResults['groups'] = [];
        const push = (id: T.SearchGroupId, title: string, list: T.Entry[]) => { if (list.length) groups.push({id, title, entries: list.slice(0, cap), total: list.length}); };
        const want = (id: T.SearchGroupId) => !groupId || groupId === id;
        if (want('movies')) push('movies', 'Movies', visibleMovies().filter(m => has(m.title) || m.genres.some(has) || m.cast.some(c => has(C.people.get(c.person)?.name ?? ''))).map(m => decorate(C.movieEntry(m))));
        if (want('shows')) push('shows', 'Shows', visibleShows().filter(x => has(x.title) || has(x.network)).map(x => decorate(C.showEntry(x))));
        if (want('episodes')) push('episodes', 'Episodes', [...C.episodes.values()].filter(r => allowedShow(r.show) && has(r.episode.title)).slice(0, 60).map(r => ({...decorate(C.episodeEntry(r)), caption: `${r.show.title} · S${r.episode.season} · E${r.episode.episode}`})));
        if (want('people')) push('people', 'People', C.catalog.people.filter(p => has(p.name) && p.credits.length).map(p => C.personEntry(p, p.credits[0]?.role === 'crew' ? p.credits[0]?.job : 'Actor')));
        if (want('artists')) push('artists', 'Artists', C.catalog.artists.filter(a => has(a.name) || has(a.genre)).map(C.artistEntry));
        if (want('albums')) push('albums', 'Albums', C.catalog.albums.filter(a => has(a.title)).map(a => decorate(C.albumEntry(a))));
        if (want('songs')) push('songs', 'Songs', [...C.tracks.values()].filter(r => has(r.track.title)).slice(0, 80).map(C.trackEntry));
        if (want('books')) push('books', 'Books', C.catalog.books.filter(b => has(b.title) || has(b.author) || has(b.narrator)).map(b => decorate(C.bookEntry(b))));
        if (want('live')) {
          const now = Date.now();
          const list: T.Entry[] = [];
          for (const ch of C.channelsRaw) {
            for (const p of programsFor(ch, now, now + 12 * HOUR)) if (has(p.title) || has(ch.name)) list.push({id: p.id, kind: 'program', title: p.title, subtitle: `${ch.name} · ${new Date(p.start).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'})}`, image: p.image, shape: 'wide', badge: p.start <= now ? 'LIVE' : undefined, parentId: ch.id, playable: p.start <= now});
            if (list.length > 40) break;
          }
          push('live', 'Live TV', list);
        }
        return {query: q, groups};
      },
      recent: () => (prefs.get('search.rememberHistory') === false ? [] : recentSearches.slice(0, 8)),
      remember(q) {
        const s = q.trim();
        if (!s || prefs.get('search.rememberHistory') === false) return;
        const i = recentSearches.findIndex(x => x.toLowerCase() === s.toLowerCase());
        if (i >= 0) recentSearches.splice(i, 1);
        recentSearches.unshift(s);
      },
      clearRecent() { recentSearches.length = 0; },
    },

    saved: {
      async list(tab, opts) {
        await latency();
        const map = personalMap();
        const filterEntries = (list: T.Entry[]) => opts?.filter === 'unwatched' ? list.filter(e => !e.watched && !e.progress) : opts?.filter === 'inProgress' ? list.filter(e => !!e.progress && !e.watched) : list;
        const sortEntries = (list: T.Entry[]) => {
          const s = opts?.sort ?? 'added';
          if (s === 'title') return list.sort((a, b) => sortKey(a.title).localeCompare(sortKey(b.title)));
          if (s === 'year') return list.sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
          if (s === 'duration') return list.sort((a, b) => (a.duration ?? 0) - (b.duration ?? 0));
          return list;
        };
        if (tab === 'watchlist' || tab === 'favorites') {
          const key = tab === 'watchlist' ? 'watchlisted' : 'favorite';
          const list = [...map.entries()].filter(([, v]) => v[key]).map(([k]) => entry(k)).filter((e): e is T.Entry => !!e);
          return {entries: sortEntries(filterEntries(list)), resources: [], history: []};
        }
        if (tab === 'history') {
          const period = opts?.period ?? 'all';
          const cutoff = period === '24h' ? 1 : period === '7d' ? 7 : period === '30d' ? 30 : period === '90d' ? 90 : 100000;
          const hist = [...map.entries()].filter(([, v]) => v.lastPlayed && v.lastPlayed > Date.now() - cutoff * 86400000).sort((a, b) => (b[1].lastPlayed ?? 0) - (a[1].lastPlayed ?? 0)).map(([k, v]) => ({entry: entry(k)!, watchedAt: v.lastPlayed!})).filter(x => x.entry);
          return {entries: [], resources: [], history: hist};
        }
        const kind = tab === 'playlists' ? 'playlist' : tab === 'collections' ? 'collection' : 'view';
        return {entries: [], resources: savedResources.filter(r => r.kind === kind).map(resourceSummary), history: []};
      },
      async resource(kind, id) {
        await latency();
        const r = savedResources.find(x => x.id === id) ?? (C.collections.has(id) ? {id, kind: 'collection' as const, title: C.collections.get(id)!.name, shared: true, owner: 'Server', items: C.collections.get(id)!.items, count: 0, description: C.collections.get(id)!.overview} : undefined);
        if (!r) throw new Error('This isn’t available anymore.');
        const items = entries(resourceItems(r.id).length ? resourceItems(r.id) : r.items);
        return {id: r.id, title: r.title, kind: r.kind, description: r.description, shared: r.shared, owner: r.owner, items, smart: r.kind === 'view', totalDuration: items.reduce((n, e) => n + (e.duration ?? 0), 0), image: items[0]?.image, editable: r.owner === 'Justin' || profile().name === r.owner};
      },
      async createPlaylist(name, ids = []) { await latency(300); const id = `pl-${Date.now()}`; savedResources.unshift({id, kind: 'playlist', title: name, count: 0, shared: false, items: [...ids], owner: profile().name}); emit('saved'); return id; },
      async createCollection(name, ids = []) { await latency(300); const id = `pc-${Date.now()}`; savedResources.unshift({id, kind: 'collection', title: name, count: 0, shared: false, items: [...ids], owner: profile().name}); emit('saved'); return id; },
      async addToPlaylist(pid, ids) { await latency(200); const r = savedResources.find(x => x.id === pid)!; ids.forEach(i => !r.items.includes(i) && r.items.push(i)); emit('saved'); },
      async addToCollection(cid, ids) { await latency(200); const r = savedResources.find(x => x.id === cid)!; ids.forEach(i => !r.items.includes(i) && r.items.push(i)); emit('saved'); },
      async removeFromResource(rid, itemId) { await latency(150); const r = savedResources.find(x => x.id === rid); if (r) r.items = r.items.filter(i => i !== itemId); emit('saved'); },
      async moveInResource(rid, itemId, delta) { await latency(80); const r = savedResources.find(x => x.id === rid); if (!r) return; const i = r.items.indexOf(itemId); const j = i + delta; if (i < 0 || j < 0 || j >= r.items.length) return; [r.items[i], r.items[j]] = [r.items[j]!, r.items[i]!]; emit('saved'); },
      async renameResource(rid, name) { await latency(200); const r = savedResources.find(x => x.id === rid); if (r) r.title = name; emit('saved', 'home'); },
      async deleteResource(rid) { await latency(250); const i = savedResources.findIndex(x => x.id === rid); if (i >= 0) savedResources.splice(i, 1); emit('saved', 'home'); },
      async setShared(rid, shared) { await latency(150); const r = savedResources.find(x => x.id === rid); if (r) r.shared = shared; emit('saved'); },
      async saveView(libraryId, name) { await latency(300); const id = `sv-${Date.now()}`; savedResources.push({id, kind: 'view', title: name, count: 0, shared: false, items: [], owner: profile().name, libraryId, smart: true}); viewQueries[id] = () => true; emit('saved'); return id; },
      async clearHistory() { await latency(300); personalMap().forEach((v, k) => personalMap().set(k, {...v, lastPlayed: undefined})); emit('saved'); },
      async playlists() { await latency(120); return savedResources.filter(r => r.kind === 'playlist').map(resourceSummary); },
      async collections() { await latency(120); return savedResources.filter(r => r.kind === 'collection').map(resourceSummary); },
    },

    live: {
      async sources() {
        await latency(60);
        return [{id: 'src-hdhr', name: 'HDHomeRun Flex 4K', kind: 'tuner', channelCount: channels.filter(c => c.source === 'src-hdhr').length}, {id: 'src-m3u', name: 'Harbor IPTV', kind: 'm3u', channelCount: channels.filter(c => c.source === 'src-m3u').length}, {id: 'library', name: 'Library Channels', kind: 'library', channelCount: channels.filter(c => c.library).length}];
      },
      async channels(opts) {
        await latency(80);
        return channels.filter(c => (opts?.hidden || !c.hidden) && (!opts?.favorites || c.favorite) && (!opts?.source || opts.source === 'all' || c.source === opts.source) && (!opts?.group || opts.group === 'all' || c.group === opts.group)).filter(c => !profile().kids || ['Kids & Family', 'Library Channels'].includes(c.group) || c.id === 'ch-weather-now');
      },
      async guide(from, to, ids) {
        await latency(120);
        const out: Record<string, T.Program[]> = {};
        for (const ch of C.channelsRaw) {
          if (ids && !ids.includes(ch.id)) continue;
          out[ch.id] = programsFor(ch, from, to).map(p => ({...p, recording: scheduledPrograms.get(p.id) ?? (recordings.some(r => r.channelId === p.channelId && r.start === p.start && (r.state === 'scheduled' || r.state === 'recording')) ? 'scheduled' : undefined), reminder: reminders.has(p.id)}));
        }
        return out;
      },
      async program(id) {
        await latency(60);
        const [chId, start] = id.split('@');
        const ch = C.channelsRaw.find(x => x.id === chId)!;
        const list = programsFor(ch, Number(start) - HOUR, Number(start) + 8 * HOUR);
        const i = list.findIndex(p => p.id === id);
        const program = {...list[i]!, recording: scheduledPrograms.get(id), reminder: reminders.has(id)};
        return {program, channel: channels.find(c => c.id === chId)!, next: list[i + 1]};
      },
      async start(channelId) {
        await latency(500);
        // Fixture: Kestrel Sports finds every tuner busy the first time, then starts on a retry.
        if (channelId === 'ch-kestrel-sports' && !startedOnce.has(channelId)) { startedOnce.add(channelId); return {ok: false, reason: 'tunersBusy'}; }
        return {ok: true};
      },
      nowPlaying(channelId) {
        const ch = C.channelsRaw.find(x => x.id === channelId);
        if (!ch) return undefined;
        const now = Date.now();
        return programsFor(ch, now, now + 1)[0];
      },
      async setFavorite(id, on) { await latency(100); const c = channels.find(x => x.id === id); if (c) c.favorite = on; emit('live'); },
      async setHidden(id, on) { await latency(100); const c = channels.find(x => x.id === id); if (c) c.hidden = on; emit('live'); },
      async remind(id, on) { await latency(100); if (on) reminders.add(id); else reminders.delete(id); emit('live'); },
      reminders: () => [...reminders],
      async record(programId) {
        await latency(250);
        scheduledPrograms.set(programId, 'scheduled');
        const [chId, start] = programId.split('@');
        const ch = channels.find(c => c.id === chId)!;
        const p = programsFor(C.channelsRaw.find(x => x.id === chId)!, Number(start), Number(start) + 1)[0]!;
        recordings = [...recordings, {id: `rec-${Date.now()}`, programId, title: p.title, episodeTitle: p.episodeTitle, channelId: chId!, channelName: ch.name, start: p.start, end: p.end, state: p.start <= Date.now() ? 'recording' : 'scheduled', image: p.image}];
        emit('live', 'dvr');
      },
      async recordSeries(programId, opts) {
        await latency(300);
        scheduledPrograms.set(programId, 'series');
        const [chId, start] = programId.split('@');
        const ch = channels.find(c => c.id === chId)!;
        const p = programsFor(C.channelsRaw.find(x => x.id === chId)!, Number(start), Number(start) + 1)[0]!;
        rules = [{id: `rule-${Date.now()}`, title: p.title, channel: opts.channel, channelName: ch.name, newOnly: opts.newOnly, keep: 'all', startEarly: 0, endLate: 0, paused: false, upcoming: 3, recorded: 0, priority: 1, image: p.image}, ...rules];
        emit('live', 'dvr');
      },
      async cancelRecording(id) {
        await latency(200);
        const r = recordings.find(x => x.id === id || x.programId === id);
        if (r?.programId) scheduledPrograms.delete(r.programId);
        if (!r) scheduledPrograms.delete(id);
        recordings = recordings.map(x => (x === r ? {...x, state: 'cancelled'} : x));
        emit('live', 'dvr');
      },
      async recordings() {
        await latency();
        const now = Date.now();
        return {
          upcoming: recordings.filter(r => r.state === 'scheduled' || r.state === 'recording' || r.state === 'conflicted').sort((a, b) => a.start - b.start),
          recorded: recordings.filter(r => (r.state === 'completed' || r.state === 'incomplete') && r.end < now).sort((a, b) => b.start - a.start),
          conflicts: recordings.filter(r => r.state === 'conflicted'),
          rules,
          storage: {usedGB: 412, freeGB: 1188, totalGB: 1600, hoursLeft: 356, retentionDays: 30, pendingDeleteGB: 6.4, writable: true},
          history: recordings.filter(r => r.state === 'failed' || r.state === 'cancelled' || r.state === 'completed').sort((a, b) => b.start - a.start),
        };
      },
      async updateRule(id, p) { await latency(150); rules = rules.map(r => (r.id === id ? {...r, ...p} : r)); emit('dvr'); },
      async deleteRule(id, cancel) { await latency(200); rules = rules.filter(r => r.id !== id); if (cancel) recordings = recordings.map(r => (r.seriesRuleId === id && r.state === 'scheduled' ? {...r, state: 'cancelled'} : r)); emit('dvr'); },
      async deleteRecording(id) { await latency(200); recordings = recordings.filter(r => r.id !== id); emit('dvr'); },
      async keepRecording(id, keep) { await latency(120); recordings = recordings.map(r => (r.id === id ? {...r, kept: keep} : r)); emit('dvr'); },
      async resolveConflict(id, choice) {
        await latency(200);
        recordings = recordings.map(r => (r.id === id ? {...r, state: choice === 'record' ? 'scheduled' : 'cancelled'} : r.state === 'conflicted' && choice === 'record' ? {...r, state: 'cancelled'} : r));
        emit('dvr');
      },
    },

    together: {
      async groups() { await latency(); return group ? [group, ...otherGroups] : otherGroups; },
      current: () => group,
      async create(name, control) {
        await latency(400);
        group = {id: `grp-${Date.now()}`, name, code: 'LTHS 4KWN', state: 'lobby', control, members: [{id: 'me', name: profile().name, art: profile().art, host: true, you: true, status: 'inStep'}], queue: [], codeExpires: Date.now() + 15 * 60000};
        setTimeout(() => { if (group) { group = {...group, members: [...group.members, {id: 'm1', name: 'Maya', art: 'violet', host: false, you: false, status: 'inStep'}]}; emit('together'); } }, 4000);
        emit('together');
        return group;
      },
      async join(code) {
        await latency(500);
        if (code.replace(/\s/g, '').length !== 8) throw new Error('That code didn’t work.');
        const g = otherGroups[0]!;
        group = {...g, members: [...g.members, {id: 'me', name: profile().name, art: profile().art, host: false, you: true, status: 'inStep'}]};
        emit('together');
        return group;
      },
      async leave() { await latency(200); group = undefined; emit('together'); },
      async end() { await latency(200); group = group ? {...group, state: 'ended'} : undefined; emit('together'); setTimeout(() => { group = undefined; emit('together'); }, 100); },
      async watch(itemId) { await latency(200); if (group) group = {...group, nowPlaying: entry(itemId), state: 'paused', position: 0}; emit('together'); },
      async queueAdd(itemId) { await latency(150); if (group) group = {...group, queue: [...group.queue, entry(itemId)!]}; emit('together'); },
      async queueRemove(itemId) { await latency(150); if (group) group = {...group, queue: group.queue.filter(e => e.id !== itemId)}; emit('together'); },
      async makeHost(memberId) { await latency(150); if (group) group = {...group, members: group.members.map(m => ({...m, host: m.id === memberId}))}; emit('together'); },
      async newCode() { await latency(200); const code = `${Math.random().toString(36).slice(2, 6)} ${Math.random().toString(36).slice(2, 6)}`.toUpperCase(); if (group) group = {...group, code, codeExpires: Date.now() + 15 * 60000}; emit('together'); return code; },
      async setPlaying(playing) { await latency(80); if (group) group = {...group, state: playing ? 'playing' : 'paused'}; emit('together'); },
    },

    downloads: {
      async list() { await latency(); return {items: downloads, requests: downloadRequests, usedGB: 6.9, freeGB: 118.4, serverSpaceGB: 214}; },
      async add(itemId, quality) {
        await latency(300);
        if (downloads.some(d => d.entry.id === itemId)) throw new Error('That’s already in your downloads.');
        const e = entry(itemId)!;
        downloads = [{id: `dl-${Date.now()}`, entry: e, quality, size: quality === 'Original' ? '2.1 GB' : quality === '1080p' ? '1.4 GB' : '640 MB', state: 'preparing', progress: 0.02}, ...downloads];
        emit('downloads');
      },
      async addShow(showId, policy, keep) {
        await latency(300);
        downloadRequests = [{id: `rq-${Date.now()}`, show: entry(showId)!, policy, keep, ready: 0, total: policy === 'all' ? HH.allEpisodesOf(showId).length : keep, state: 'preparing'}, ...downloadRequests];
        emit('downloads');
      },
      async remove(id) { await latency(150); downloads = downloads.filter(d => d.id !== id); downloadRequests = downloadRequests.filter(r => r.id !== id); emit('downloads'); },
      async pause(id, paused) { await latency(100); downloads = downloads.map(d => (d.id === id ? {...d, state: paused ? 'paused' : 'downloading'} : d)); emit('downloads'); },
      async retry(id) { await latency(150); downloads = downloads.map(d => (d.id === id ? {...d, state: 'preparing'} : d)); emit('downloads'); },
      settings: () => downloadSettings,
      async updateSettings(p) { await latency(80); downloadSettings = {...downloadSettings, ...p}; emit('downloads'); },
      has: itemId => downloads.some(d => d.entry.id === itemId),
    },

    account: {
      async devices() { await latency(); return devices; },
      async signOutDevice(id) { await latency(200); devices = devices.filter(d => d.id !== id); emit('account'); },
      async signOutEverywhere() { await latency(400); devices = devices.filter(d => d.current); emit('account'); },
      async trustDevice(id, trusted) { await latency(120); devices = devices.map(d => (d.id === id ? {...d, trusted, state: 'approved'} : d)); emit('account', 'admin'); },
      async renameDevice(id, name) { await latency(120); devices = devices.map(d => (d.id === id ? {...d, name} : d)); emit('account'); },
      async changePassword(current, next) { await latency(400); if (current.length < 4) throw new Error('That password isn’t right.'); if (next.length < 8) throw new Error('Use at least 8 characters.'); },
      async setTwoStep(on, code) {
        await latency(400);
        if (on && code && !/^\d{6}$/.test(code)) throw new Error('That code didn’t work. Check it and try again.');
        HH.account.twoStep = on;
        emit('account');
        return on ? {recoveryCodes: ['k2m9-x7qa', 'p4hd-2wzt', 'r8vn-5kce', 'b3jy-9mtf', 'w6qs-1pld', 'f5xr-8gnb', 'c7tk-4hvw', 'n2ze-6ydq']} : {};
      },
      async recoveryCodes() { await latency(300); HH.account.recoveryCodesLeft = 8; return ['h3kd-8xpq', 'z9wm-2rte', 'q6vb-4nls', 'u1cy-7fgd', 'e8pj-3tkw', 'a4rx-6mzh', 'l2nq-9ysv', 't7gb-5dkc']; },
      async updateProfile(id, p) {
        await latency(200);
        for (const list of Object.values(HH.profilesByServer)) {
          const i = list.findIndex(x => x.id === id);
          if (i >= 0) list[i] = {...list[i]!, ...p, pin: p.pinValue === undefined ? list[i]!.pin : !!p.pinValue};
        }
        if (session?.profile.id === id) session = {...session, profile: HH.profilesByServer[serverId]!.find(x => x.id === id)!};
        emit('account', 'session');
      },
      async addProfile(name, art, kids) {
        await latency(300);
        const list = HH.profilesByServer[serverId]!;
        if (list.length >= 8) throw new Error('An account can have up to 8 profiles.');
        const p: T.Profile = {id: `pr-${Date.now()}`, name, art, pin: false, kids, role: 'member'};
        list.push(p);
        emit('account', 'session');
        return p;
      },
      async deleteProfile(id) { await latency(300); const list = HH.profilesByServer[serverId]!; const i = list.findIndex(p => p.id === id); if (i >= 0) list.splice(i, 1); emit('account', 'session'); },
      async moveProfile(id, delta) { await latency(80); const list = HH.profilesByServer[serverId]!; const i = list.findIndex(p => p.id === id); const j = i + delta; if (i < 0 || j < 1 || j >= list.length) return; [list[i], list[j]] = [list[j]!, list[i]!]; emit('account', 'session'); },
      async invitations() { await latency(); return invitations; },
      async answerInvitation(id, accept) { await latency(300); invitations = invitations.map(i => (i.id === id ? {...i, state: accept ? 'accepted' : 'declined'} : i)); emit('account'); },
      async leaveServer() { await latency(400); emit('account'); },
      async changeEmail(email) { await latency(400); HH.account.email = email; emit('account'); },
      async exportData() { await latency(900); },
      async deleteAccount(confirm) { await latency(500); if (confirm !== HH.account.email) throw new Error('That doesn’t match your account email. Type it exactly as it appears on your account.'); },
      async connectProvider(p, on) { await latency(400); HH.account.providers = on ? [...new Set([...HH.account.providers, p])] : HH.account.providers.filter(x => x !== p); emit('account'); },
    },

    notifications: {
      async list(view) {
        await latency(100);
        const isAdmin = profile().role !== 'member';
        const vis = notices.filter(n => isAdmin || n.audience === 'profile');
        return (view === 'unread' ? vis.filter(n => !n.read && !n.archived) : view === 'archived' ? vis.filter(n => n.archived) : vis.filter(n => !n.archived)).sort((a, b) => b.at - a.at);
      },
      unread: () => notices.filter(n => !n.read && !n.archived && (profile().role !== 'member' || n.audience === 'profile')).length,
      async mark(id, read) { await latency(60); const n = notices.find(x => x.id === id); if (n) n.read = read; emit('notifications'); },
      async archive(id, archived) { await latency(60); const n = notices.find(x => x.id === id); if (n) { n.archived = archived; n.read = true; } emit('notifications'); },
      async markAll() { await latency(100); notices.forEach(n => (n.read = true)); emit('notifications'); },
      async sendFeedback(input) { await latency(700); if (input.message.trim().length < 10) throw new Error('Describe it in at least 10 characters.'); return input.message.includes('again') ? 'duplicate' : 'sent'; },
    },

    prefs: {
      defs: () => prefDefs,
      get: key => prefs.get(key) ?? false,
      async set(key, value) { prefs.set(key, value); emit('prefs'); },
      async resetRecommendations() { await latency(500); personalMap().forEach((v, k) => personalMap().set(k, {...v, notInterested: false, reaction: undefined})); emit('home'); },
    },
  };
  return client;
}
