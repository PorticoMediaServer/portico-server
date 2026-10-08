/**
 * Builds the in-memory fixture database from the packed catalogue: libraries, items, people,
 * per-profile user state, live TV, recordings, downloads and more. Deterministic on every launch;
 * relative dates are anchored to the moment the app starts so "now" always looks right.
 */
import {images, tints as artTints} from '../fixtures/art.generated';
import {paletteTint} from '../../design/tokens';
import type {
  Album, Artist, Audiobook, Channel, Chapter, Collection, Credit, DeviceSession, Download, DownloadRequest, Episode, Extra, Item, Library,
  MediaVersion, Movie, NotificationItem, Person, Photo, PhotoAlbum, Playlist, Profile, Programme, Recording, RecordingRule, SavedView, Season, Server,
  Show, TogetherGroup, Track, TrackItem, UserState, Invitation, Account,
} from '../types';
import {content, type CMovie, type CPerson} from './content';
import {hash, rng, slug, sortTitle} from './rng';

export const NOW = Date.now();
export const MIN = 60_000;
export const HOUR = 60 * MIN;
export const DAY = 24 * HOUR;

const tintOf = (key: string, palette?: string, fallbackSeed?: string) =>
  artTints[key] ?? (palette && paletteTint[palette]) ?? Object.values(paletteTint)[hash(fallbackSeed ?? key) % 10];
const has = (key: string) => key in images;

export const LIB = {movies: 'lib-movies', shows: 'lib-tv', anime: 'lib-anime', music: 'lib-music', books: 'lib-books', photos: 'lib-photos', home: 'lib-home'} as const;

export type DB = ReturnType<typeof buildDB>;

export function emptyState(): UserState {
  return {watched: false, progress: 0, position: 0, watchlist: false, favorite: false, rating: 0, liked: null, notInterested: false, lastPlayedAt: null};
}

function versionsFor(id: string, year: number, runtimeMin: number, real: boolean): MediaVersion[] {
  const r = rng(id + ':v');
  const secs = runtimeMin * 60;
  if (real) {
    return [{id: id + '-v1', label: '1080p · Restored', resolution: '1080p', hdr: null, container: 'MKV', video: 'H.264 · 1920×1080 · 8 Mb/s', audio: 'FLAC Mono', sizeGB: +(secs * 8 / 8 / 1024).toFixed(1), bitrateMbps: 8}];
  }
  const out: MediaVersion[] = [];
  if (year >= 2012 && r.chance(0.7)) {
    const dv = r.chance(0.5);
    out.push({id: id + '-v4k', label: dv ? '4K · Dolby Vision' : '4K · HDR10', resolution: '4K', hdr: dv ? 'Dolby Vision' : 'HDR10', container: 'MKV', video: `HEVC Main 10 · 3840×2160 · ${r.int(38, 62)} Mb/s`, audio: r.chance(0.6) ? 'TrueHD Atmos 7.1' : 'E-AC3 5.1', sizeGB: +(secs * 50 / 8 / 1024).toFixed(1), bitrateMbps: 50});
  }
  out.push({id: id + '-v1080', label: '1080p', resolution: '1080p', hdr: null, container: r.chance(0.5) ? 'MP4' : 'MKV', video: `H.264 High · 1920×1080 · ${r.int(8, 14)} Mb/s`, audio: 'AAC 5.1', sizeGB: +(secs * 11 / 8 / 1024).toFixed(1), bitrateMbps: 11});
  if (r.chance(0.12)) out.push({id: id + '-vdc', label: 'Director’s Cut · 1080p', resolution: '1080p', hdr: null, container: 'MKV', video: 'H.264 · 1920×1080', audio: 'DTS-HD MA 5.1', sizeGB: +(secs * 1.15 * 12 / 8 / 1024).toFixed(1), bitrateMbps: 12, edition: 'Director’s Cut'});
  return out;
}

const LANGS: Record<string, string> = {English: 'en', French: 'fr', Korean: 'ko', Japanese: 'ja', Spanish: 'es', Danish: 'da', Italian: 'it', Hindi: 'hi', German: 'de', Russian: 'ru', Portuguese: 'pt', Swedish: 'sv', Mandarin: 'zh', Norwegian: 'no', Polish: 'pl', Turkish: 'tr'};

function tracksFor(id: string, language = 'English'): {audio: Track[]; subs: Track[]} {
  const r = rng(id + ':t');
  const code = LANGS[language] ?? 'en';
  const audio: Track[] = [{id: id + '-a1', label: `${language} · 5.1`, language: code, codec: 'E-AC3', channels: '5.1'}];
  if (language !== 'English') audio.push({id: id + '-a2', label: 'English · 5.1 (dub)', language: 'en', codec: 'E-AC3', channels: '5.1'});
  audio.push({id: id + '-a3', label: `${language} · Stereo`, language: code, codec: 'AAC', channels: '2.0'});
  if (r.chance(0.3)) audio.push({id: id + '-a4', label: 'Commentary · Stereo', language: 'en', codec: 'AAC', channels: '2.0'});
  const subs: Track[] = [
    {id: id + '-s1', label: 'English', language: 'en', origin: 'embedded'},
    {id: id + '-s2', label: 'English (SDH)', language: 'en', origin: 'embedded'},
    {id: id + '-s3', label: 'Spanish', language: 'es', origin: r.chance(0.5) ? 'sidecar' : 'embedded'},
    {id: id + '-s4', label: 'French', language: 'fr', origin: 'provider'},
  ];
  if (language !== 'English') subs.unshift({id: id + '-s0', label: 'English (Forced)', language: 'en', forced: true, origin: 'embedded'});
  return {audio, subs};
}

const CHAPTER_NAMES = ['Opening', 'The Arrival', 'First Light', 'An Invitation', 'Crossroads', 'Under Pressure', 'The Turn', 'Night Falls', 'Confrontation', 'Aftermath', 'The Long Road', 'Finale', 'End Credits'];

function chaptersFor(id: string, runtimeMin: number, stillPrefix: string): Chapter[] {
  const n = Math.max(4, Math.min(12, Math.round(runtimeMin / 10)));
  const secs = runtimeMin * 60;
  return Array.from({length: n}, (_, i) => {
    const art = `${stillPrefix}-${i % 10}`;
    return {title: i === n - 1 ? 'End Credits' : i === 0 ? 'Opening' : CHAPTER_NAMES[1 + ((i + hash(id)) % 10)], start: Math.round((secs * i) / n), art: has(art) ? art : undefined};
  });
}

function extrasFor(id: string, title: string, art: string): Extra[] {
  const r = rng(id + ':x');
  const out: Extra[] = [{id: id + '-trailer', title: `${title} · Official Trailer`, kind: 'Trailer', duration: r.int(95, 160), art}];
  if (r.chance(0.7)) out.push({id: id + '-bts', title: 'Making the Film', kind: 'Behind the Scenes', duration: r.int(420, 1400), art});
  if (r.chance(0.5)) out.push({id: id + '-int', title: 'In Conversation with the Director', kind: 'Interview', duration: r.int(300, 900), art});
  if (r.chance(0.35)) out.push({id: id + '-del', title: 'Deleted Scenes', kind: 'Deleted Scene', duration: r.int(180, 600), art});
  return out;
}

export function buildDB() {
  const items = new Map<string, Item>();
  const people = new Map<string, Person>();
  const byLibrary = new Map<string, string[]>();
  const credits = new Map<string, {cast: {itemId: string; character?: string}[]; crew: {itemId: string; job?: string}[]}>();
  const add = (it: Item) => {
    items.set(it.id, it);
    if (!byLibrary.has(it.libraryId)) byLibrary.set(it.libraryId, []);
    if (it.kind !== 'season' && it.kind !== 'episode' && it.kind !== 'track' && it.kind !== 'photo') byLibrary.get(it.libraryId)!.push(it.id);
  };
  const credit = (personId: string, itemId: string, role: {character?: string; job?: string}) => {
    if (!credits.has(personId)) credits.set(personId, {cast: [], crew: []});
    const c = credits.get(personId)!;
    if (role.job) c.crew.push({itemId, job: role.job});
    else c.cast.push({itemId, character: role.character});
  };

  // ---- People
  const addPerson = (p: CPerson) => {
    const key = `person/${p.id}`;
    people.set(p.id, {id: p.id, kind: 'person', name: p.name, art: {headshot: has(key) ? key : undefined}, tint: tintOf(key, undefined, p.id), bio: p.bio ?? '', born: p.born ?? null, died: p.died ?? null, birthplace: p.birthplace, knownFor: p.knownFor ?? 'Acting'});
  };
  content.real.people.forEach(addPerson);
  content.people.forEach(addPerson);
  const pname = (id?: string | null) => (id ? people.get(id)?.name ?? '' : '');

  // ---- Movies
  const addMovie = (m: CMovie, real: boolean, index: number) => {
    const poster = `poster/${m.id}`;
    const backdrop = `backdrop/${m.id}`;
    const cr: Credit[] = [];
    if (m.director) {
      cr.push({personId: m.director, name: pname(m.director), job: 'Director', department: 'Directing'});
      credit(m.director, m.id, {job: 'Director'});
    }
    (m.writers ?? []).forEach(w => {
      cr.push({personId: w, name: pname(w), job: 'Writer', department: 'Writing'});
      credit(w, m.id, {job: 'Writer'});
    });
    (m.cast ?? []).forEach(c => {
      if (!people.has(c.person)) return;
      cr.push({personId: c.person, name: pname(c.person), character: c.character, department: 'Acting'});
      credit(c.person, m.id, {character: c.character});
    });
    const {audio, subs} = tracksFor(m.id, m.language);
    const stillPrefix = `still/${m.id}`;
    const movie: Movie = {
      id: m.id, kind: 'movie', title: m.title, sortTitle: sortTitle(m.title), libraryId: LIB.movies, year: m.year,
      art: {poster, backdrop: has(backdrop) ? backdrop : undefined},
      tint: tintOf(poster, m.palette, m.id), genres: m.genres, synopsis: m.synopsis, addedAt: NOW - (real ? 200 : 3) * DAY - index * 0.9 * DAY,
      tagline: m.tagline, contentRating: m.contentRating, runtime: m.runtime, studio: m.studio, country: m.country, language: m.language, mood: m.mood,
      criticScore: m.criticScore, audienceScore: m.audienceScore, credits: cr, versions: versionsFor(m.id, m.year, m.runtime, real),
      audioTracks: audio, subtitleTracks: subs, chapters: chaptersFor(m.id, m.runtime, has(`${stillPrefix}-0`) ? stillPrefix : `backdrop/${m.id}`),
      extras: extrasFor(m.id, m.title, has(`${stillPrefix}-3`) ? `${stillPrefix}-3` : backdrop), franchise: m.franchise, license: m.license, real,
    };
    add(movie);
  };
  content.real.movies.forEach((m, i) => addMovie(m, true, i));
  // Newer additions first so "Recently added" feels alive.
  [...content.movies].sort((a, b) => b.year - a.year).forEach((m, i) => addMovie(m, false, i));

  // ---- Shows and anime
  for (const [si, s] of content.shows.entries()) {
    const lib = s.kind === 'anime' ? LIB.anime : LIB.shows;
    const poster = `poster/${s.id}`;
    const backdrop = `backdrop/${s.id}`;
    const cr: Credit[] = [];
    (s.creators ?? []).forEach(c => {
      if (!people.has(c)) return;
      cr.push({personId: c, name: pname(c), job: 'Creator', department: 'Writing'});
      credit(c, s.id, {job: 'Creator'});
    });
    (s.cast ?? []).forEach(c => {
      if (!people.has(c.person)) return;
      cr.push({personId: c.person, name: pname(c.person), character: c.character, department: 'Acting'});
      credit(c.person, s.id, {character: c.character});
    });
    const tint = tintOf(poster, s.palette, s.id);
    const seasonIds: string[] = [];
    let epCount = 0;
    const seasons = [...s.seasons].sort((a, b) => (a.number === 0 ? 999 : a.number) - (b.number === 0 ? 999 : b.number));
    for (const season of seasons) {
      const seasonId = `${s.id}-s${season.number}`;
      seasonIds.push(seasonId);
      const episodeIds: string[] = [];
      const yearOf = s.yearStart + Math.max(0, season.number - 1);
      season.episodes.forEach((e, ei) => {
        const epId = `${seasonId}e${ei + 1}`;
        episodeIds.push(epId);
        epCount++;
        const still = `still/${epId}`;
        const {audio, subs} = tracksFor(epId, s.kind === 'anime' ? 'Japanese' : 'English');
        const ep: Episode = {
          id: epId, kind: 'episode', title: e.title, sortTitle: e.title, libraryId: lib, year: yearOf,
          art: {still: has(still) ? still : has(backdrop) ? backdrop : undefined, poster},
          tint, genres: s.genres, synopsis: e.synopsis, addedAt: NOW - (si * 2 + 5) * DAY + ei * HOUR,
          showId: s.id, seasonId, seasonNumber: season.number, number: ei + 1, runtime: e.runtime,
          airDate: new Date(Date.UTC(yearOf, 0, 10) + ei * 7 * DAY).toISOString().slice(0, 10), credits: cr.filter(c => c.department === 'Acting').slice(0, 4),
          versions: versionsFor(epId, yearOf, e.runtime, false).slice(-1), audioTracks: audio, subtitleTracks: subs,
          chapters: chaptersFor(epId, e.runtime, `backdrop/${s.id}`).slice(0, 5), introEnd: season.number === 0 ? undefined : 45 + (hash(epId) % 50), creditsStart: e.runtime * 60 - 50,
        };
        items.set(epId, ep);
      });
      const seasonItem: Season = {id: seasonId, kind: 'season', title: season.number === 0 ? 'Specials' : `Season ${season.number}`, sortTitle: `Season ${season.number}`, libraryId: lib, year: yearOf, art: {poster}, tint, genres: s.genres, addedAt: NOW, showId: s.id, number: season.number, episodeIds};
      items.set(seasonId, seasonItem);
    }
    const show: Show = {
      id: s.id, kind: 'show', title: s.title, sortTitle: sortTitle(s.title), libraryId: lib, year: s.yearStart, art: {poster, backdrop: has(backdrop) ? backdrop : undefined},
      tint, genres: s.genres, synopsis: s.synopsis, addedAt: NOW - (si * 1.7 + 1) * DAY, anime: s.kind === 'anime', contentRating: s.contentRating ?? 'TV-14',
      network: s.network ?? 'Portico Originals', status: (s.status as Show['status']) ?? (s.yearEnd ? 'Ended' : 'Continuing'), yearEnd: s.yearEnd, credits: cr, seasonIds, episodeCount: epCount,
    };
    add(show);
  }

  // ---- Music
  const lyricKeys = new Set(Object.keys(content.lyrics));
  for (const [ai, a] of content.music.entries()) {
    const art = `artist/${a.id}`;
    const tint = tintOf(art, a.palette, a.id);
    const albumIds: string[] = [];
    for (const [bi, al] of a.albums.entries()) {
      const cover = `album/${al.id}`;
      const trackIds: string[] = [];
      let duration = 0;
      al.tracks.forEach((t, ti) => {
        const tid = `${al.id}-t${ti + 1}`;
        trackIds.push(tid);
        duration += t.seconds;
        const tr: TrackItem = {id: tid, kind: 'track', title: t.title, sortTitle: t.title, libraryId: LIB.music, year: al.year, art: {square: cover}, tint: tintOf(cover, a.palette, al.id), genres: a.genres ?? [], addedAt: NOW, albumId: al.id, artistId: a.id, artistName: a.name, albumTitle: al.title, number: ti + 1, disc: t.disc ?? 1, duration: t.seconds, hasLyrics: lyricKeys.has(`${al.id}::${t.title}`)};
        items.set(tid, tr);
      });
      const album: Album = {id: al.id, kind: 'album', title: al.title, sortTitle: sortTitle(al.title), libraryId: LIB.music, year: al.year, art: {square: cover}, tint: tintOf(cover, a.palette, al.id), genres: a.genres ?? [], addedAt: NOW - (ai * 3 + bi) * DAY, artistId: a.id, artistName: a.name, label: al.label ?? 'Independent', albumType: al.type ?? 'Album', trackIds, discs: Math.max(...al.tracks.map(t => t.disc ?? 1)), duration};
      add(album);
      albumIds.push(al.id);
    }
    const artist: Artist = {id: a.id, kind: 'artist', title: a.name, sortTitle: sortTitle(a.name), libraryId: LIB.music, art: {square: has(art) ? art : `album/${a.albums[0]?.id}`}, tint, genres: a.genres ?? [], addedAt: NOW - ai * DAY, bio: a.bio ?? '', origin: a.origin ?? '', formed: a.formed ?? 2000, band: a.kind !== 'solo', albumIds};
    add(artist);
  }

  // ---- Audiobooks
  for (const [i, b] of content.audiobooks.entries()) {
    const cover = `book/${b.id}`;
    let t = 0;
    const chapters = b.chapters.map(c => {
      const ch = {title: c.title, start: t, duration: c.minutes * 60};
      t += c.minutes * 60;
      return ch;
    });
    const authorId = 'author-' + slug(b.author);
    const book: Audiobook = {id: b.id, kind: 'audiobook', title: b.title, sortTitle: sortTitle(b.title), libraryId: LIB.books, year: b.year, art: {poster: cover}, tint: tintOf(cover, b.palette, b.id), genres: b.genres ?? [], synopsis: b.synopsis, addedAt: NOW - i * 2 * DAY, author: b.author, authorId, authorBio: b.authorBio ?? '', narrator: b.narrator ?? 'Unknown narrator', series: b.series ?? null, chapters, duration: t, publicDomain: !!b.publicDomain};
    add(book);
  }

  // ---- Photos
  for (const [i, pa] of content.personal.photoAlbums.entries()) {
    const photoIds: string[] = [];
    const start = new Date(pa.dateStart).getTime();
    for (let n = 0; n < pa.count; n++) {
      const pid = `${pa.id}-${n + 1}`;
      photoIds.push(pid);
      const r = rng(pid);
      const portrait = r.chance(0.22);
      const photo: Photo = {id: pid, kind: 'photo', title: pa.captions?.[n] ?? `${pa.title} ${n + 1}`, sortTitle: pid, libraryId: LIB.photos, art: {thumb: `photothumb/${pid}`, backdrop: `photo/${pid}`}, tint: tintOf(`photo/${pid}`, undefined, pid), genres: [], addedAt: start, albumId: pa.id, caption: pa.captions?.[n] ?? '', taken: new Date(start + n * 47 * MIN).toISOString(), place: pa.place ?? '', width: portrait ? 3024 : 4032, height: portrait ? 4032 : 3024};
      items.set(pid, photo);
    }
    const album: PhotoAlbum = {id: pa.id, kind: 'photoAlbum', title: pa.title, sortTitle: pa.title, libraryId: LIB.photos, year: new Date(pa.dateStart).getFullYear(), art: {thumb: `photothumb/${photoIds[0]}`, backdrop: `photo/${photoIds[0]}`}, tint: tintOf(`photo/${photoIds[0]}`, undefined, pa.id), genres: [], addedAt: NOW - i * 20 * DAY, place: pa.place ?? '', dateStart: pa.dateStart, dateEnd: pa.dateEnd ?? pa.dateStart, photoIds};
    add(album);
  }

  // ---- Home videos
  for (const hv of content.personal.homeVideos) {
    const still = `homevideo/${hv.id}`;
    const {audio, subs} = tracksFor(hv.id);
    const v: Movie = {id: hv.id, kind: 'homevideo', title: hv.title, sortTitle: hv.title, libraryId: LIB.home, year: new Date(hv.date).getFullYear(), art: {backdrop: still, still}, tint: tintOf(still, undefined, hv.id), genres: [], synopsis: hv.description, addedAt: new Date(hv.date).getTime(), contentRating: 'NR', runtime: hv.minutes, credits: [], versions: [{id: hv.id + '-v', label: '4K · HEVC (iPhone)', resolution: '4K', hdr: 'Dolby Vision', container: 'MOV', video: 'HEVC · 3840×2160 · 24 Mb/s', audio: 'AAC Stereo', sizeGB: +(hv.minutes * 60 * 24 / 8 / 1024).toFixed(1), bitrateMbps: 24}], audioTracks: audio.slice(0, 1), subtitleTracks: [], chapters: [], extras: [], place: hv.place, date: hv.date};
    add(v);
  }

  // ---- Libraries
  const libraries: Library[] = [
    {id: LIB.movies, name: 'Movies', kind: 'movies', count: byLibrary.get(LIB.movies)?.length ?? 0, scanning: false, views: ['discover', 'browse', 'collections', 'categories'], sourcePaths: ['/Volumes/Media/Movies', '/Volumes/Archive/Classics']},
    {id: LIB.shows, name: 'TV Shows', kind: 'shows', count: byLibrary.get(LIB.shows)?.length ?? 0, scanning: false, views: ['discover', 'browse', 'collections', 'categories'], sourcePaths: ['/Volumes/Media/TV']},
    {id: LIB.anime, name: 'Anime', kind: 'anime', count: byLibrary.get(LIB.anime)?.length ?? 0, scanning: true, views: ['discover', 'browse', 'categories'], sourcePaths: ['/Volumes/Media/Anime']},
    {id: LIB.music, name: 'Music', kind: 'music', count: byLibrary.get(LIB.music)?.length ?? 0, scanning: false, views: ['discover', 'browse', 'releases', 'songs', 'categories'], sourcePaths: ['/Volumes/Media/Music']},
    {id: LIB.books, name: 'Audiobooks', kind: 'audiobooks', count: byLibrary.get(LIB.books)?.length ?? 0, scanning: false, views: ['discover', 'browse', 'authors', 'series'], sourcePaths: ['/Volumes/Media/Audiobooks']},
    {id: LIB.photos, name: 'Photos', kind: 'photos', count: byLibrary.get(LIB.photos)?.length ?? 0, scanning: false, views: ['browse', 'timeline'], sourcePaths: ['/Users/family/Pictures/Portico']},
    {id: LIB.home, name: 'Home Videos', kind: 'homevideos', count: byLibrary.get(LIB.home)?.length ?? 0, scanning: false, views: ['browse', 'timeline'], sourcePaths: ['/Users/family/Movies/Family']},
  ];

  // ---- Collections (server-authored) from franchises and themes
  const collections: Collection[] = [];
  const franchises = new Map<string, string[]>();
  for (const it of items.values()) if (it.kind === 'movie' && it.franchise) franchises.set(it.franchise, [...(franchises.get(it.franchise) ?? []), it.id]);
  for (const [name, ids] of franchises) {
    ids.sort((a, b) => ((items.get(a) as Movie).year ?? 0) - ((items.get(b) as Movie).year ?? 0));
    collections.push({id: 'col-' + slug(name), kind: 'collection', title: name.endsWith('Collection') || name.endsWith('Movies') ? name : `${name} Collection`, description: `Every ${name} film, in release order.`, itemIds: ids, shared: true, owner: 'server', libraryId: LIB.movies, tint: items.get(ids[0])!.tint});
  }
  const realMovies = content.real.movies.filter(m => !m.license?.startsWith('CC')).map(m => m.id);
  collections.push({id: 'col-public-domain-classics', kind: 'collection', title: 'Public Domain Classics', description: 'Silent comedies, noir and early horror, free for everyone to watch and share.', itemIds: realMovies, shared: true, owner: 'server', libraryId: LIB.movies, tint: '#b99a6b'});
  const movieIds = byLibrary.get(LIB.movies) ?? [];
  const pickBy = (fn: (m: Movie) => boolean, n: number) => movieIds.map(id => items.get(id) as Movie).filter(fn).slice(0, n).map(m => m.id);
  collections.push({id: 'col-midnight-movies', kind: 'collection', title: 'Midnight Movies', description: 'Strange, scary and unforgettable after dark.', itemIds: pickBy(m => m.genres.includes('Horror') || m.mood === 'Eerie', 18), shared: true, owner: 'server', libraryId: LIB.movies, tint: '#6b4bb3'});
  collections.push({id: 'col-award-winners', kind: 'collection', title: 'Critics’ Circle', description: 'The best-reviewed films on this server.', itemIds: movieIds.map(id => items.get(id) as Movie).sort((a, b) => (b.criticScore ?? 0) - (a.criticScore ?? 0)).slice(0, 20).map(m => m.id), shared: true, owner: 'server', libraryId: LIB.movies, tint: '#d8b44a'});
  collections.push({id: 'col-family-night', kind: 'collection', title: 'Family Movie Night', description: 'Something everyone can agree on.', itemIds: pickBy(m => m.genres.includes('Family') || m.genres.includes('Animation'), 18), shared: true, owner: 'server', libraryId: LIB.movies, tint: '#3fae78'});
  collections.push({id: 'col-my-comfort', kind: 'collection', title: 'Comfort Watches', description: 'My rainy-day list.', itemIds: pickBy(m => m.mood === 'Cozy' || m.mood === 'Heartfelt' || m.mood === 'Funny', 12), shared: false, owner: 'me', libraryId: LIB.movies, tint: '#d4668f'});
  const showIds = byLibrary.get(LIB.shows) ?? [];
  collections.push({id: 'col-limited-series', kind: 'collection', title: 'Limited Series', description: 'One story, one season.', itemIds: showIds.filter(id => (items.get(id) as Show).seasonIds.filter(s => !s.endsWith('-s0')).length === 1).slice(0, 12), shared: true, owner: 'server', libraryId: LIB.shows, tint: '#2fa4a0'});

  // ---- Playlists
  const allTracks = [...items.values()].filter(i => i.kind === 'track').map(i => i.id);
  const r = rng('playlists');
  const playlists: Playlist[] = [
    {id: 'pl-late-night-drive', kind: 'playlist', title: 'Late Night Drive', description: 'Low lights, long roads.', itemIds: r.shuffle(allTracks).slice(0, 24), shared: false, media: 'music', updatedAt: NOW - 2 * DAY, tint: '#3f6fe0'},
    {id: 'pl-sunday-morning', kind: 'playlist', title: 'Sunday Morning', description: 'Coffee, windows open.', itemIds: r.shuffle(allTracks).slice(0, 18), shared: true, media: 'music', updatedAt: NOW - 6 * DAY, tint: '#d8b44a'},
    {id: 'pl-focus', kind: 'playlist', title: 'Deep Focus', description: 'Instrumentals for getting things done.', itemIds: r.shuffle(allTracks).slice(0, 30), shared: false, media: 'music', updatedAt: NOW - 12 * DAY, tint: '#2fa4a0'},
    {id: 'pl-halloween-marathon', kind: 'playlist', title: 'Halloween Marathon', description: 'Six films, one very long night.', itemIds: pickBy(m => m.genres.includes('Horror'), 6), shared: true, media: 'video', updatedAt: NOW - 20 * DAY, tint: '#e0763a'},
  ];

  // ---- Profiles
  const limits = {maxRating: null, allowUnrated: true, downloads: true, liveTv: true, dvr: true, together: true, hiddenLabels: []};
  const profiles: Profile[] = [
    {id: 'pr-justin', name: 'Justin', color: '#409fd4', initials: 'J', kids: false, pin: null, main: true, limits},
    {id: 'pr-ava', name: 'Ava', color: '#d4668f', initials: 'A', kids: false, pin: '2468', main: false, limits},
    {id: 'pr-leo', name: 'Leo', color: '#3fae78', initials: 'L', kids: false, pin: null, main: false, limits: {...limits, dvr: false}},
    {id: 'pr-kids', name: 'Kids', color: '#e8bc6a', initials: 'K', kids: true, pin: null, main: false, limits: {maxRating: 'PG', allowUnrated: false, downloads: true, liveTv: false, dvr: false, together: false, hiddenLabels: ['Horror']}},
  ];

  // ---- User state (main profile), seeded from the catalogue
  const state = new Map<string, UserState>();
  const us = (id: string) => {
    if (!state.has(id)) state.set(id, emptyState());
    return state.get(id)!;
  };
  const sr = rng('userstate');
  const movies = movieIds.map(id => items.get(id) as Movie);
  const shows = [...(byLibrary.get(LIB.shows) ?? []), ...(byLibrary.get(LIB.anime) ?? [])].map(id => items.get(id) as Show);
  const history: {itemId: string; at: number}[] = [];
  // Watched history: ~70 movies and several show runs.
  sr.shuffle(movies).slice(0, 70).forEach((m, i) => {
    const s = us(m.id);
    s.watched = true;
    s.lastPlayedAt = NOW - (i * 2.3 + 1) * DAY;
    if (sr.chance(0.4)) s.rating = sr.int(3, 5);
    history.push({itemId: m.id, at: s.lastPlayedAt});
  });
  // Continue watching: movies part-way through.
  const inProgressMovies = sr.shuffle(movies.filter(m => !us(m.id).watched && m.art.backdrop)).slice(0, 5);
  inProgressMovies.forEach((m, i) => {
    const s = us(m.id);
    s.progress = 0.18 + sr.next() * 0.6;
    s.position = Math.round(s.progress * m.runtime * 60);
    s.lastPlayedAt = NOW - (i * 9 + 2) * HOUR;
    history.push({itemId: m.id, at: s.lastPlayedAt});
  });
  // Shows in progress: watched first N episodes, next one partially.
  const inProgressShows = sr.shuffle(shows).slice(0, 6);
  inProgressShows.forEach((sh, i) => {
    const eps = sh.seasonIds.filter(s => !s.endsWith('-s0')).flatMap(s => (items.get(s) as Season).episodeIds);
    const upto = Math.min(eps.length - 1, sr.int(1, Math.max(1, Math.floor(eps.length * 0.7))));
    eps.slice(0, upto).forEach((e, k) => {
      const s = us(e);
      s.watched = true;
      s.lastPlayedAt = NOW - (i * 11 + 3) * HOUR - (upto - k) * DAY;
    });
    const cur = items.get(eps[upto]) as Episode;
    if (i < 4) {
      const s = us(cur.id);
      s.progress = 0.2 + sr.next() * 0.5;
      s.position = Math.round(s.progress * cur.runtime * 60);
      s.lastPlayedAt = NOW - (i * 11 + 1) * HOUR;
      history.push({itemId: cur.id, at: s.lastPlayedAt});
    }
  });
  // Watchlist and favorites.
  sr.shuffle(movies.filter(m => !us(m.id).watched)).slice(0, 22).forEach(m => (us(m.id).watchlist = true));
  sr.shuffle(shows).slice(0, 6).forEach(s => (us(s.id).watchlist = true));
  sr.shuffle(movies.filter(m => us(m.id).watched)).slice(0, 14).forEach(m => (us(m.id).favorite = true));
  shows.slice(0, 3).forEach(s => (us(s.id).favorite = true));
  // Music and books in progress.
  const albums = [...items.values()].filter(i => i.kind === 'album');
  sr.shuffle(albums).slice(0, 8).forEach((a, i) => {
    us(a.id).lastPlayedAt = NOW - (i + 1) * 5 * HOUR;
    history.push({itemId: a.id, at: NOW - (i + 1) * 5 * HOUR});
    if (i < 4) us(a.id).favorite = true;
  });
  const books = [...items.values()].filter(i => i.kind === 'audiobook') as Audiobook[];
  books.slice(0, 3).forEach((b, i) => {
    const s = us(b.id);
    s.progress = 0.15 + i * 0.22;
    s.position = Math.round(s.progress * b.duration);
    s.lastPlayedAt = NOW - (i * 13 + 4) * HOUR;
    history.push({itemId: b.id, at: s.lastPlayedAt});
  });
  history.sort((a, b) => b.at - a.at);

  const savedViews: SavedView[] = [
    {id: 'sv-unwatched-4k', title: 'Unwatched in 4K', libraryId: LIB.movies, filters: {quick: ['unwatched', 'uhd'], fields: {}}, sort: {field: 'added', direction: 'desc'}, shared: false, onHome: true},
    {id: 'sv-90s-thrillers', title: '’90s Thrillers', libraryId: LIB.movies, filters: {quick: [], fields: {genre: ['Thriller'], decade: ['1990s']}}, sort: {field: 'year', direction: 'asc'}, shared: true, onHome: false},
    {id: 'sv-short-films', title: 'Under 90 Minutes', libraryId: LIB.movies, filters: {quick: [], fields: {duration: ['short']}}, sort: {field: 'title', direction: 'asc'}, shared: false, onHome: false},
  ];

  // ---- Servers and account
  const servers: Server[] = [
    {id: 'srv-home', name: 'Ehler Home Theater', host: 'portico.home.arpa:32500', version: '1.4.2', status: 'online', owner: true, role: 'owner', via: 'account', lastSeen: NOW, libraries: libraries.length},
    {id: 'srv-cabin', name: 'Lake Cabin', host: 'cabin.ehler.net:32500', version: '1.4.0', status: 'online', owner: false, role: 'admin', via: 'account', lastSeen: NOW - 20 * MIN, libraries: 3},
    {id: 'srv-maya', name: 'Maya’s Archive', host: '203.0.113.41:32500', version: '1.3.9', status: 'unreachable', owner: false, role: 'member', via: 'account', lastSeen: NOW - 3 * DAY, libraries: 2},
  ];
  const account: Account = {id: 'acct-justin', name: 'Justin Ehler', username: 'justin', email: 'justin@example.com', hosted: true, twoStep: true, recoveryCodesLeft: 8, providers: {apple: true, google: false}, hasPassword: true};
  const sessions: DeviceSession[] = [
    {id: 'dev-1', name: 'Living Room Apple TV', platform: 'tvos', app: 'Portico', signedInAt: NOW - 40 * DAY, lastUsed: NOW - 5 * MIN, current: false, trusted: true, waiting: false, location: 'Home network'},
    {id: 'dev-2', name: 'Justin’s iPhone', platform: 'ios', app: 'Portico', signedInAt: NOW - 90 * DAY, lastUsed: NOW, current: true, trusted: true, waiting: false, location: 'Home network'},
    {id: 'dev-3', name: 'Safari on MacBook Pro', platform: 'web', app: 'Portico on the web', signedInAt: NOW - 12 * DAY, lastUsed: NOW - 2 * HOUR, current: false, trusted: true, waiting: false, location: 'Portland, US'},
    {id: 'dev-4', name: 'Bedroom TV', platform: 'androidtv', app: 'Portico', signedInAt: NOW - 1 * HOUR, lastUsed: NOW - 50 * MIN, current: false, trusted: false, waiting: true, location: 'Home network'},
    {id: 'dev-5', name: 'Ava’s iPad', platform: 'ios', app: 'Portico', signedInAt: NOW - 30 * DAY, lastUsed: NOW - 1 * DAY, current: false, trusted: true, waiting: false, location: 'Seattle, US'},
  ];
  const notifications: NotificationItem[] = [
    {id: 'n1', title: 'New episodes of shows you watch', body: 'Two new episodes were added to TV Shows this morning.', at: NOW - 3 * HOUR, read: false, archived: false, audience: 'profile', kind: 'info'},
    {id: 'n2', title: 'Recording finished', body: 'Your recording from Channel 4.1 is ready to watch in DVR.', at: NOW - 9 * HOUR, read: false, archived: false, audience: 'profile', kind: 'info'},
    {id: 'n3', title: 'New sign-in: Bedroom TV', body: 'If this wasn’t you, sign it out and change your password.', at: NOW - 50 * MIN, read: false, archived: false, audience: 'profile', kind: 'security'},
    {id: 'n4', title: 'Storage is nearly full on Ehler Home Theater', body: 'The Media volume has 6% free. Free up space before adding more media.', at: NOW - 1 * DAY, read: true, archived: false, audience: 'admin', kind: 'urgent'},
    {id: 'n5', title: 'Maya’s Archive has been quiet', body: 'Portico hasn’t heard from this server for three days.', at: NOW - 2 * DAY, read: true, archived: false, audience: 'profile', kind: 'server'},
    {id: 'n6', title: 'Reply to your report', body: '“Thanks, the subtitles for Detour are fixed now.” — Server owner', at: NOW - 4 * DAY, read: true, archived: false, audience: 'profile', kind: 'notice'},
    {id: 'n7', title: 'Someone joined Ehler Home Theater', body: 'An invitation you sent was accepted by Leo.', at: NOW - 9 * DAY, read: true, archived: true, audience: 'admin', kind: 'info'},
    {id: 'n8', title: 'Server maintenance on Saturday', body: 'The server restarts at 3 AM on Saturday for an update. Playback pauses for about five minutes.', at: NOW - 12 * DAY, read: true, archived: false, audience: 'profile', kind: 'notice'},
  ];
  const invitations: Invitation[] = [
    {id: 'inv-1', server: 'Sam’s Film Club', inviter: 'Sam Okafor', libraries: 4, expires: NOW + 5 * DAY, state: 'waiting'},
    {id: 'inv-2', server: 'Lake Cabin', inviter: 'Maya Ehler', libraries: 3, expires: NOW - 30 * DAY, state: 'accepted'},
    {id: 'inv-3', server: 'Old Dorm Server', inviter: 'Chris Wong', libraries: 2, expires: NOW - 60 * DAY, state: 'expired'},
  ];

  // ---- Live TV
  const channels: Channel[] = [];
  const programmes: Programme[] = [];
  const dayStart = new Date(NOW);
  dayStart.setHours(0, 0, 0, 0);
  const guideStart = dayStart.getTime() - DAY;
  const guideEnd = dayStart.getTime() + 2 * DAY;
  for (const c of content.live.channels) {
    const logo = `logo/${c.id}`;
    const library = c.group === 'Library' || c.source === 'Library Channel';
    channels.push({id: c.id, number: c.number, name: c.name, callsign: c.callsign ?? c.name.slice(0, 4).toUpperCase(), group: c.group, source: library ? 'Library Channels' : c.source ?? 'Antenna', logo, tint: tintOf(logo, c.palette, c.id), favorite: hash(c.id) % 5 === 0, hidden: false, library});
    if (!c.programs.length) continue;
    const cr = rng(c.id + ':guide');
    let t = guideStart;
    let idx = cr.int(0, c.programs.length - 1);
    let n = 0;
    while (t < guideEnd) {
      const p = c.programs[idx % c.programs.length];
      const minutes = Math.max(15, p.minutes || 30);
      const pid = `${c.id}-${n}`;
      programmes.push({id: pid, channelId: c.id, title: p.title, episodeTitle: p.episodeTitle ?? null, synopsis: p.synopsis ?? '', start: t, end: t + minutes * MIN, genre: p.genre ?? c.group, rating: p.rating ?? 'TV-PG', series: !!p.series, isNew: cr.chance(0.18)});
      t += minutes * MIN;
      idx++;
      n++;
    }
  }
  const nowProgs = programmes.filter(p => p.start <= NOW && p.end > NOW);
  const future = programmes.filter(p => p.start > NOW + HOUR && p.series).sort((a, b) => a.start - b.start);
  const pastSeries = programmes.filter(p => p.end < NOW - HOUR && p.series).sort((a, b) => b.start - a.start);
  const recordings: Recording[] = [];
  pastSeries.slice(0, 9).forEach((p, i) => recordings.push({id: 'rec-' + p.id, programmeId: p.id, channelId: p.channelId, title: p.title, episodeTitle: p.episodeTitle, start: p.start, end: p.end, state: i === 7 ? 'incomplete' : i === 8 ? 'failed' : 'completed', kept: i === 1, sizeGB: +((p.end - p.start) / HOUR * 3.4).toFixed(1), watched: i > 4, ruleId: i < 4 ? 'rule-1' : undefined, reason: i === 7 ? 'The source changed during recording. What was captured is kept.' : i === 8 ? 'The program ended before recording could start.' : undefined}));
  if (nowProgs[2]) recordings.push({id: 'rec-now', programmeId: nowProgs[2].id, channelId: nowProgs[2].channelId, title: nowProgs[2].title, episodeTitle: nowProgs[2].episodeTitle, start: nowProgs[2].start, end: nowProgs[2].end, state: 'recording', kept: false, sizeGB: 1.2, watched: false});
  future.slice(0, 8).forEach((p, i) => recordings.push({id: 'rec-' + p.id, programmeId: p.id, channelId: p.channelId, title: p.title, episodeTitle: p.episodeTitle, start: p.start, end: p.end, state: i === 2 ? 'conflicted' : 'scheduled', kept: false, sizeGB: 0, watched: false, ruleId: i % 2 ? 'rule-2' : undefined, reason: i === 2 ? 'Other recordings are using every tuner at this time.' : undefined}));
  recordings.push({id: 'rec-cancelled', channelId: channels[0]?.id ?? '', title: pastSeries[12]?.title ?? 'Evening News', start: NOW - 3 * DAY, end: NOW - 3 * DAY + HOUR, state: 'cancelled', kept: false, sizeGB: 0, watched: false, reason: 'Canceled by this profile.'});
  const rules: RecordingRule[] = [
    {id: 'rule-1', title: pastSeries[0]?.title ?? 'Evening News', channelId: pastSeries[0]?.channelId ?? null, newOnly: true, keep: 5, startEarly: 1, endLate: 2, paused: false, priority: 2, scheduledCount: 3},
    {id: 'rule-2', title: future[1]?.title ?? 'Nature Hour', channelId: null, newOnly: false, keep: 'all', startEarly: 0, endLate: 5, paused: false, priority: 1, scheduledCount: 4},
    {id: 'rule-3', title: future[5]?.title ?? 'Match of the Week', channelId: future[5]?.channelId ?? null, newOnly: true, keep: 3, startEarly: 2, endLate: 30, paused: true, priority: 3, scheduledCount: 0},
  ];

  // ---- Downloads
  const dlPool: string[] = [...movies.filter(m => !m.real).slice(3, 7).map(m => m.id), ...inProgressShows.slice(0, 1).flatMap(s => (items.get(s.seasonIds.find(x => !x.endsWith('-s0'))!) as Season).episodeIds.slice(0, 3))];
  const downloads: Download[] = dlPool.map((id, i) => ({id: 'dl-' + i, itemId: id, quality: i % 2 ? '1080p' : 'Original', sizeMB: 900 + (hash(id) % 3000), progress: i === 3 ? 0.46 : i === 4 ? 0.12 : 1, state: i === 3 ? 'downloading' : i === 4 ? 'paused' : 'done', where: 'device'}));
  downloads.push({id: 'dl-s1', itemId: movies[20]?.id ?? '', quality: '720p', sizeMB: 1400, progress: 1, state: 'ready', where: 'server', expires: NOW + 6 * DAY});
  downloads.push({id: 'dl-s2', itemId: movies[21]?.id ?? '', quality: '1080p', sizeMB: 2600, progress: 0.38, state: 'preparing', where: 'server'});
  downloads.push({id: 'dl-s3', itemId: movies[22]?.id ?? '', quality: 'Original', sizeMB: 5200, progress: 0, state: 'queued', where: 'server'});
  downloads.push({id: 'dl-s4', itemId: movies[23]?.id ?? '', quality: '720p', sizeMB: 1100, progress: 1, state: 'expired', where: 'server'});
  const downloadRequests: DownloadRequest[] = inProgressShows[1] ? [{id: 'dr-1', showId: inProgressShows[1].id, policy: 'unwatched', keepNext: 3, ready: 2, total: 5, failed: 0}] : [];

  // ---- Together
  const togetherGroups: TogetherGroup[] = [
    {id: 'tg-1', name: 'Friday Movie Night', code: 'KXWP7M2Q', state: 'lobby', control: 'host', members: [
      {id: 'm1', name: 'Justin', color: '#409fd4', status: 'inStep', host: true, you: true, ready: true},
      {id: 'm2', name: 'Ava', color: '#d4668f', status: 'inStep', host: false, you: false, ready: true},
      {id: 'm3', name: 'Sam', color: '#e8bc6a', status: 'buffering', host: false, you: false, ready: false},
    ], queue: [...new Set(['m-charade', ...movies.filter(m => !m.real).slice(10, 12).map(m => m.id)])], nowPlaying: null, position: 0},
  ];

  return {items, people, byLibrary, credits, libraries, collections, playlists, profiles, state, history, savedViews, servers, account, sessions, notifications, invitations, channels, programmes, recordings, rules, reminders: new Set<string>(nowProgs.length ? [future[3]?.id].filter(Boolean) as string[] : []), downloads, downloadRequests, togetherGroups, recentSearches: ['charade', 'sintel', 'noir'], homeLayout: null as null | {rowId: string; visible: boolean}[], continueHidden: new Set<string>(), lyrics: content.lyrics};
}
