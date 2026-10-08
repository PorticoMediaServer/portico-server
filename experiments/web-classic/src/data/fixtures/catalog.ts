import raw from './catalogData.json';
import uniqueStills from './uniqueStills.json';
import * as art from './art.generated';
import type {Entry, Img, LibraryKind} from '../types';
import {hash} from './util';

/** Raw catalogue shapes (scripts/art/catalog.py). */
export type RawCastRef = {person: string; character: string};
export type RawMovie = {id: string; title: string; year: number; runtime: number; rating: string; genres: string[]; directors: string[]; writers: string[]; cast: RawCastRef[]; overview: string; studio: string; audience: number; critic: number; library: string; real: boolean; license?: string; tagline: string | null; collections?: string[]; art: {kind: string}};
export type RawEpisode = {id: string; season: number; episode: number; absolute: number; title: string; runtime: number; airDate: string; overview: string};
export type RawSeason = {number: number; title: string; year: number; episodes: RawEpisode[]};
export type RawShow = {id: string; title: string; kind: 'tv' | 'anime'; year: number; endYear: number | null; rating: string; genres: string[]; network: string; overview: string; cast: RawCastRef[]; creators: string[]; seasons: RawSeason[]; audience: number; library: string};
export type RawPerson = {id: string; name: string; born?: string; died?: string; bio?: string; real: boolean; credits: {item: string; role: 'cast' | 'crew'; character?: string; job?: string}[]};
export type RawTrack = {id: string; number: number; disc: number; title: string; duration: number; lyrics: {at: number; line: string}[] | null};
export type RawAlbum = {id: string; title: string; artist: string; year: number; genre: string; label: string; tracks: RawTrack[]; type: string};
export type RawArtist = {id: string; name: string; genre: string; albums: string[]; bio: string};
export type RawBook = {id: string; title: string; author: string; narrator: string; year: number; genre: string; chapters: {number: number; title: string; duration: number}[]; overview: string; series: string | null; seriesIndex: number | null; publisher: string};
export type RawPhoto = {id: string; title: string; taken: string; camera: string; portrait: boolean};
export type RawPhotoAlbum = {id: string; title: string; photos: RawPhoto[]};
export type RawHomeVideo = {id: string; title: string; year: number; runtime: number; date: string; overview: string};
export type RawChannel = {id: string; number: string; name: string; category: string; group: string; source: string; hd: boolean; library?: boolean; desc?: string};
export type RawCollection = {id: string; name: string; library: string; items: string[]; overview: string};

const data = raw as unknown as {
  libraries: {id: string; name: string; kind: LibraryKind}[];
  movies: RawMovie[];
  shows: RawShow[];
  collections: RawCollection[];
  people: RawPerson[];
  artists: RawArtist[];
  albums: RawAlbum[];
  books: RawBook[];
  photoAlbums: RawPhotoAlbum[];
  homeVideos: RawHomeVideo[];
  channels: RawChannel[];
};

export const catalog = data;
export const movies = new Map(data.movies.map(m => [m.id, m]));
export const shows = new Map(data.shows.map(s => [s.id, s]));
export const people = new Map(data.people.map(p => [p.id, p]));
export const artists = new Map(data.artists.map(a => [a.id, a]));
export const albums = new Map(data.albums.map(a => [a.id, a]));
export const books = new Map(data.books.map(b => [b.id, b]));
export const photoAlbums = new Map(data.photoAlbums.map(a => [a.id, a]));
export const homeVideos = new Map(data.homeVideos.map(v => [v.id, v]));
export const collections = new Map(data.collections.map(c => [c.id, c]));
export const channelsRaw = data.channels;

export type EpisodeRef = {episode: RawEpisode; season: RawSeason; show: RawShow};
export const episodes = new Map<string, EpisodeRef>();
for (const show of data.shows) for (const season of show.seasons) for (const episode of season.episodes) episodes.set(episode.id, {episode, season, show});
export type TrackRef = {track: RawTrack; album: RawAlbum; artist: RawArtist};
export const tracks = new Map<string, TrackRef>();
for (const album of data.albums) {
  const artist = artists.get(album.artist)!;
  for (const track of album.tracks) tracks.set(track.id, {track, album, artist});
}
export const photos = new Map<string, {photo: RawPhoto; album: RawPhotoAlbum}>();
for (const album of data.photoAlbums) for (const photo of album.photos) photos.set(photo.id, {photo, album});

/** Every item gets a stable "date added", newest first spread over two years. */
const NOW = Date.now();
export const addedAt = new Map<string, number>();
const addOrder = [...data.movies.map(m => m.id), ...data.shows.map(s => s.id), ...data.albums.map(a => a.id), ...data.books.map(b => b.id), ...data.homeVideos.map(v => v.id)];
for (const id of addOrder) addedAt.set(id, NOW - (hash(id + 'added') % (730 * 86400)) * 1000);
for (const [id, ref] of episodes) addedAt.set(id, (addedAt.get(ref.show.id) ?? NOW) + ref.episode.absolute * 3600 * 1000 * 20);

// ── Artwork lookup ──────────────────────────────────────────────────────
// Distinct episode stills (scripts/art/dedupe_backdrops.py). A show steps
// through them in episode order, so no picture repeats within a show.
const url = (kind: string, id: string) => `/art/${kind}/${id}.webp`;
const has = (set: Set<string>, kind: string, id: string): Img => (set.has(id) ? url(kind, id) : undefined);
const stillKeys: string[] = (uniqueStills as string[]).filter(k => art.stills.has(k));
export function stillFor(id: string): Img {
  if (art.stills.has(id)) return url('stills', id);
  if (!stillKeys.length) return undefined;
  const ep = episodes.get(id);
  const i = ep ? hash(ep.show.id) + ep.episode.absolute : hash(id);
  return url('stills', stillKeys[i % stillKeys.length]!);
}
export const posterFor = (id: string): Img => has(art.posters, 'posters', id) ?? has(art.books, 'books', id) ?? has(art.albums, 'albums', id);
export const backdropFor = (id: string): Img => has(art.backdrops, 'backdrops', id);
export const albumArt = (id: string): Img => has(art.albums, 'albums', id);
export const artistArt = (id: string): Img => has(art.artists, 'artists', id);
export const bookArt = (id: string): Img => has(art.books, 'books', id);
export const personArt = (id: string): Img => has(art.people, 'people', id);
export const photoArt = (id: string): Img => has(art.photos, 'photos', id);
export const photoThumb = (id: string): Img => has(art.photothumbs, 'photothumbs', id);
export const logoFor = (id: string): Img => has(art.logos, 'logos', id);
/** The dominant colour of a title's artwork, for the glow and for placeholders. */
export const tintFor = (id: string): string | undefined => art.tints[id];
export const isRealPhoto = (id: string) => art.realPhotoPeople.has(id);

export function libraryOf(id: string): string | undefined {
  if (movies.has(id)) return movies.get(id)!.library;
  if (shows.has(id)) return shows.get(id)!.library;
  if (episodes.has(id)) return episodes.get(id)!.show.library;
  if (albums.has(id) || artists.has(id) || tracks.has(id)) return 'lib-music';
  if (books.has(id)) return 'lib-audiobooks';
  if (photoAlbums.has(id) || photos.has(id)) return 'lib-photos';
  if (homeVideos.has(id)) return 'lib-homevideos';
  return undefined;
}

export const years = (s: RawShow) => (s.endYear && s.endYear !== s.year ? `${s.year}–${s.endYear}` : s.endYear === null && s.seasons.length > 1 ? `${s.year}–` : String(s.year));

/** Card projections (without personal state; the client layers that on). */
export function movieEntry(m: RawMovie): Entry {
  return {id: m.id, tint: art.tints[m.id], kind: 'movie', title: m.title, subtitle: String(m.year), year: m.year, image: posterFor(m.id), backdrop: backdropFor(m.id), shape: 'poster', duration: m.runtime * 60, rating: m.rating, genres: m.genres, overview: m.overview, libraryId: m.library, playable: true};
}
export function showEntry(s: RawShow): Entry {
  const eps = s.seasons.reduce((n, se) => n + se.episodes.length, 0);
  return {id: s.id, tint: art.tints[s.id], kind: 'show', title: s.title, subtitle: years(s), year: s.year, image: posterFor(s.id), backdrop: backdropFor(s.id), shape: 'poster', rating: s.rating, genres: s.genres, overview: s.overview, libraryId: s.library, playable: true, badge: undefined, caption: `${s.seasons.filter(x => x.number > 0).length} season${s.seasons.filter(x => x.number > 0).length === 1 ? '' : 's'} · ${eps} episodes`};
}
export function episodeEntry(ref: EpisodeRef): Entry & {season: number; episode: number; airDate: string} {
  const {episode: e, show} = ref;
  const code = e.season === 0 ? `Special ${e.episode}` : `S${e.season} · E${e.episode}`;
  return {id: e.id, tint: art.tints[show.id], kind: 'episode', title: e.title, subtitle: show.title, image: stillFor(e.id), poster: posterFor(show.id), backdrop: backdropFor(show.id), shape: 'wide', duration: e.runtime * 60, overview: e.overview, libraryId: show.library, parentId: show.id, playable: true, caption: code, rating: show.rating, season: e.season, episode: e.episode, airDate: e.airDate, year: Number(e.airDate.slice(0, 4))};
}
export function artistEntry(a: RawArtist): Entry {
  return {id: a.id, kind: 'artist', title: a.name, subtitle: a.genre, image: artistArt(a.id), backdrop: backdropFor(a.id), shape: 'circle', libraryId: 'lib-music', genres: [a.genre], playable: true};
}
export function albumEntry(a: RawAlbum): Entry {
  const artist = artists.get(a.artist)!;
  return {id: a.id, tint: art.tints[a.id], kind: 'album', title: a.title, subtitle: artist.name, year: a.year, image: albumArt(a.id), backdrop: backdropFor(artist.id), shape: 'square', libraryId: 'lib-music', parentId: artist.id, genres: [a.genre], playable: true, duration: a.tracks.reduce((n, t) => n + t.duration, 0), caption: `${a.year} · ${a.type}`};
}
export function trackEntry(ref: TrackRef): Entry {
  return {id: ref.track.id, kind: 'track', title: ref.track.title, subtitle: ref.artist.name, caption: ref.album.title, image: albumArt(ref.album.id), shape: 'square', duration: ref.track.duration, libraryId: 'lib-music', parentId: ref.album.id, playable: true};
}
export function bookEntry(b: RawBook): Entry {
  const total = b.chapters.reduce((n, c) => n + c.duration, 0);
  return {id: b.id, tint: art.tints[b.id], kind: 'audiobook', title: b.title, subtitle: b.author, year: b.year, image: bookArt(b.id), shape: 'poster', duration: total, libraryId: 'lib-audiobooks', genres: [b.genre], overview: b.overview, playable: true, caption: b.series ? `${b.series} · Book ${b.seriesIndex}` : undefined};
}
export function photoAlbumEntry(a: RawPhotoAlbum): Entry {
  const first = a.photos[0];
  return {id: a.id, kind: 'photoAlbum', title: a.title, subtitle: `${a.photos.length} photos`, image: first ? photoThumb(first.id) : undefined, backdrop: first ? photoArt(first.id) : undefined, shape: 'square', libraryId: 'lib-photos', playable: false};
}
export function homeVideoEntry(v: RawHomeVideo): Entry {
  return {id: v.id, kind: 'homevideo', title: v.title, subtitle: new Date(v.date).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'}), year: v.year, image: has(art.stills, 'stills', v.id), backdrop: backdropFor(v.id), shape: 'wide', duration: v.runtime * 60, libraryId: 'lib-homevideos', overview: v.overview, playable: true};
}
export function personEntry(p: RawPerson, role?: string): Entry {
  return {id: p.id, kind: 'person', title: p.name, subtitle: role, image: personArt(p.id), shape: 'circle', playable: false};
}
export function collectionEntry(c: RawCollection): Entry {
  const first = c.items[0];
  return {id: c.id, kind: 'collection', title: c.name, subtitle: `${c.items.length} titles`, image: first ? posterFor(first) : undefined, backdrop: first ? backdropFor(first) : undefined, stack: c.items.slice(0, 3).map(posterFor), shape: 'poster', libraryId: c.library, overview: c.overview, playable: true};
}

export function entryFor(id: string): Entry | undefined {
  if (movies.has(id)) return movieEntry(movies.get(id)!);
  if (shows.has(id)) return showEntry(shows.get(id)!);
  if (episodes.has(id)) return episodeEntry(episodes.get(id)!);
  if (albums.has(id)) return albumEntry(albums.get(id)!);
  if (artists.has(id)) return artistEntry(artists.get(id)!);
  if (tracks.has(id)) return trackEntry(tracks.get(id)!);
  if (books.has(id)) return bookEntry(books.get(id)!);
  if (homeVideos.has(id)) return homeVideoEntry(homeVideos.get(id)!);
  if (photoAlbums.has(id)) return photoAlbumEntry(photoAlbums.get(id)!);
  if (people.has(id)) return personEntry(people.get(id)!);
  if (collections.has(id)) return collectionEntry(collections.get(id)!);
  return undefined;
}

/** Playback length in seconds for anything playable. */
export function durationOf(id: string): number {
  if (movies.has(id)) return movies.get(id)!.runtime * 60;
  if (episodes.has(id)) return episodes.get(id)!.episode.runtime * 60;
  if (tracks.has(id)) return tracks.get(id)!.track.duration;
  if (books.has(id)) return books.get(id)!.chapters.reduce((n, c) => n + c.duration, 0);
  if (homeVideos.has(id)) return homeVideos.get(id)!.runtime * 60;
  return 0;
}
