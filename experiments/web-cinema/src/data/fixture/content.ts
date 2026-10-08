/** Typed view of the packed catalogue content (scripts/catalog/pack.py → fixtures/content.json). */
import raw from '../fixtures/content.json';

export type CPerson = {id: string; name: string; gender?: string; born?: string | null; died?: string | null; birthplace?: string; bio?: string; knownFor?: string};
export type CCast = {person: string; character?: string};
export type CMovie = {id: string; title: string; year: number; genres: string[]; contentRating: string; runtime: number; tagline?: string; synopsis: string; studio?: string; country?: string; language?: string; director?: string | null; writers?: string[]; cast?: CCast[]; mood?: string; criticScore?: number; audienceScore?: number; franchise?: string | null; palette?: string; license?: string; poster?: string; video?: string};
export type CEpisode = {title: string; synopsis: string; runtime: number};
export type CShow = {id: string; kind: 'show' | 'anime'; title: string; yearStart: number; yearEnd?: number | null; status?: string; network?: string; genres: string[]; contentRating?: string; synopsis: string; creators?: string[]; cast?: CCast[]; palette?: string; seasons: {number: number; episodes: CEpisode[]}[]};
export type CBook = {id: string; title: string; author: string; authorBio?: string; narrator?: string; year?: number; genres?: string[]; synopsis?: string; series?: {name: string; number: number} | null; publicDomain?: boolean; palette?: string; chapters: {title: string; minutes: number}[]};
export type CAlbum = {id: string; title: string; year: number; label?: string; type?: string; tracks: {title: string; seconds: number; disc?: number}[]};
export type CArtist = {id: string; name: string; kind?: string; genres?: string[]; formed?: number; origin?: string; bio?: string; palette?: string; albums: CAlbum[]};
export type CProgram = {title: string; episodeTitle?: string | null; synopsis?: string; minutes: number; genre?: string; rating?: string; series?: boolean};
export type CChannel = {id: string; number: string; name: string; callsign?: string; group: string; source?: string; palette?: string; programs: CProgram[]};
export type CHomeVideo = {id: string; title: string; date: string; minutes: number; place?: string; description?: string};
export type CPhotoAlbum = {id: string; title: string; place?: string; dateStart: string; dateEnd?: string; count: number; captions?: string[]};

type Content = {
  real: {people: CPerson[]; movies: CMovie[]};
  people: CPerson[];
  movies: CMovie[];
  shows: CShow[];
  audiobooks: CBook[];
  music: CArtist[];
  lyrics: Record<string, {t: number; line: string}[]>;
  live: {channels: CChannel[]};
  personal: {homeVideos: CHomeVideo[]; photoAlbums: CPhotoAlbum[]};
};

export const content = raw as unknown as Content;
