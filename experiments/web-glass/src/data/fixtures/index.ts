import type { Person } from '../types.ts';
import { movies, collections } from './movies.ts';
import { shows, episodes } from './shows.ts';
import { albums, artists, playlists, tracks } from './music.ts';
import { audiobooks } from './audiobooks.ts';
import { photos, photoAlbums, memories, homeVideos } from './photos.ts';
import { daysAgo, personId } from './util.ts';

export { movies, collections, shows, episodes, albums, artists, playlists, tracks, audiobooks, photos, photoAlbums, memories, homeVideos };

export interface LibraryInfo {
  id: string;
  name: string;
  kind: 'movies' | 'shows' | 'anime' | 'music' | 'audiobooks' | 'photos' | 'homevideos';
  href: string;
  count: number;
  countLabel: string;
  sizeBytes: number;
  paths: string[];
  lastScan: string;
  accent: string;
}

const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0);
const tv = shows.filter((s) => !s.isAnime);
const anime = shows.filter((s) => s.isAnime);
const epCount = (list: typeof shows) => sum(list.map((s) => sum(s.seasons.map((x) => x.episodes.length))));

export const libraries: LibraryInfo[] = [
  { id: 'lib-movies', name: 'Movies', kind: 'movies', href: '/movies', count: movies.length, countLabel: `${movies.length} movies`, sizeBytes: 14.1e12, paths: ['/volume1/media/Movies', '/volume1/media/UHD'], lastScan: daysAgo(0, 9), accent: '#409fd4' },
  { id: 'lib-tv', name: 'TV Shows', kind: 'shows', href: '/tv', count: tv.length, countLabel: `${tv.length} shows · ${epCount(tv)} episodes`, sizeBytes: 12.6e12, paths: ['/volume1/media/TV'], lastScan: daysAgo(0, 9), accent: '#5cb3e2' },
  { id: 'lib-anime', name: 'Anime', kind: 'anime', href: '/anime', count: anime.length, countLabel: `${anime.length} series · ${epCount(anime)} episodes`, sizeBytes: 3.9e12, paths: ['/volume1/media/Anime'], lastScan: daysAgo(0, 9), accent: '#8a7cf0' },
  { id: 'lib-music', name: 'Music', kind: 'music', href: '/music', count: tracks.length, countLabel: `${artists.length} artists · ${albums.length} albums · ${tracks.length} songs`, sizeBytes: 1.2e12, paths: ['/volume1/media/Music'], lastScan: daysAgo(0, 9), accent: '#35c28a' },
  { id: 'lib-audiobooks', name: 'Audiobooks', kind: 'audiobooks', href: '/audiobooks', count: audiobooks.length, countLabel: `${audiobooks.length} books`, sizeBytes: 0.2e12, paths: ['/volume1/media/Audiobooks'], lastScan: daysAgo(0, 9), accent: '#f0a02a' },
  { id: 'lib-photos', name: 'Photos', kind: 'photos', href: '/photos', count: photos.length, countLabel: `${photos.length} photos · ${photoAlbums.length} albums`, sizeBytes: 3.1e12, paths: ['/volume2/photos'], lastScan: daysAgo(0, 9), accent: '#e25aa8' },
  { id: 'lib-homevideos', name: 'Home Videos', kind: 'homevideos', href: '/home-videos', count: homeVideos.length, countLabel: `${homeVideos.length} videos`, sizeBytes: 1.2e12, paths: ['/volume2/home-videos'], lastScan: daysAgo(0, 9), accent: '#c9a86a' },
];

// ---------- People ----------

const BIOS: Record<string, [bio: string, born?: string, place?: string]> = {
  'Anna Lindgren': ['Swedish actor who trained at the Royal Dramatic Theatre in Stockholm before breaking out as Signe Wahl in Meridian. Known for playing brilliant, difficult women with very little patience for fools.', '1986', 'Uppsala, Sweden'],
  'Lucía Ferrer': ['Spanish-American actor and former competitive swimmer. After a decade in Madrid theater, she became an international lead with Deep Field and Tidewater.', '1990', 'Valencia, Spain'],
  'Kwame Mensah': ['British-Ghanaian actor known for grounded, quietly intense performances across film and television, from Meridian to The Ferryman’s Wager.', '1982', 'London, England'],
  'Freya Nilsson': ['Norwegian-Swedish actor who began in children’s television and now anchors some of Scandinavia’s most acclaimed crime dramas.', '1994', 'Tromsø, Norway'],
  'Tomas Ekberg': ['Veteran Swedish character actor and screenwriter, and a long-time collaborator of director Ingrid Solberg.', '1961', 'Kiruna, Sweden'],
  'Ingrid Solberg': ['Norwegian writer-director whose films mix science fiction and family drama. The Long Quiet won the Golden Bear; Deep Field is her first English-language feature.', '1975', 'Bergen, Norway'],
  'Marcus Hale': ['American action lead and former stuntman who does his own climbing, diving and, reportedly, cooking on set.', '1979', 'Boise, Idaho'],
  'Nadia Petrov': ['Bulgarian-American actor and martial artist, star of Ironclad Express and Undercurrent.', '1988', 'Sofia, Bulgaria'],
  'Sofia Marchetti': ['Italian actor equally at home in Venetian period dramas and London thrillers.', '1991', 'Venice, Italy'],
  'Hana Sato': ['Japanese actor and voice performer, known to anime audiences as Ren in Mirror of the Sky and Aoi in Blade of the Ninth Moon.', '1993', 'Sapporo, Japan'],
  'Declan Rourke': ['Irish stage and screen actor with a gift for playing charming men who are hiding something.', '1970', 'Cork, Ireland'],
  'Owen Gallagher': ['American comic actor and former game show host — which made Afterglow Motel feel autobiographical.', '1968', 'Scranton, Pennsylvania'],
  'Iris Kane': ['American comedian, writer and co-creator of The Quiet Office.', '1987', 'Chicago, Illinois'],
  'Callum Reid': ['Scottish actor and narrator whose voice is familiar from a decade of nature documentaries.', '1955', 'Inverness, Scotland'],
  'Grace Whitfield': ['English actor, star and creator of Foxglove Lane.', '1959', 'Bath, England'],
  'Buster Keaton': ['American actor, comedian and filmmaker (1895–1966), famous for his stoic deadpan and dangerous physical stunts. The General is widely considered one of the greatest films ever made.', '1895', 'Piqua, Kansas'],
  'Charlie Chaplin': ['English comic actor and filmmaker (1889–1977) who created the Tramp, the most recognizable character of the silent era.', '1889', 'London, England'],
  'Cary Grant': ['English-American actor (1904–1986), the definitive leading man of Hollywood’s golden age.', '1904', 'Bristol, England'],
  'Audrey Hepburn': ['British actor and humanitarian (1929–1993), star of Roman Holiday, Breakfast at Tiffany’s and Charade.', '1929', 'Brussels, Belgium'],
  'Fritz Lang': ['Austrian-German-American filmmaker (1890–1976), director of Metropolis and M.', '1890', 'Vienna, Austria'],
  'Georges Méliès': ['French illusionist and filmmaker (1861–1938) who pioneered special effects in cinema.', '1861', 'Paris, France'],
  'Colin Levy': ['American director and animator who directed Sintel at the Blender Institute at 22.', '1988', 'United States'],
  'Hjalti Hjálmarsson': ['Icelandic animator and director at Blender Studio, director of Coffee Run and Charge.', undefined, 'Iceland'],
};

const peopleMap = new Map<string, Person>();
const creditOf: Record<string, { itemId: string; role: string }[]> = {};
const addPerson = (name: string, role: string, itemId: string) => {
  const id = personId(name);
  if (!peopleMap.has(id)) {
    const b = BIOS[name];
    peopleMap.set(id, { id, kind: 'person', name, knownFor: '', bio: b?.[0] ?? '', born: b?.[1], birthplace: b?.[2] });
  }
  (creditOf[id] ??= []).push({ itemId, role });
};
for (const m of movies) {
  m.cast.forEach((c) => addPerson(c.name, c.role, m.id));
  m.crew.forEach((c) => addPerson(c.name, c.job, m.id));
}
for (const s of shows) {
  s.cast.forEach((c) => addPerson(c.name, c.role, s.id));
  s.crew.forEach((c) => addPerson(c.name, c.job, s.id));
}
for (const p of peopleMap.values()) {
  const credits = creditOf[p.id];
  const titles = Array.from(new Set(credits.map((c) => [...movies, ...shows].find((x) => x.id === c.itemId)?.title))).filter(Boolean) as string[];
  p.knownFor = titles.slice(0, 3).join(', ');
  const acting = credits.some((c) => !['Director', 'Writer', 'Creator'].includes(c.role));
  if (!p.bio) {
    const dept = acting ? 'Actor' : credits.some((c) => c.role === 'Creator') ? 'Creator' : 'Filmmaker';
    p.bio = `${dept} credited on ${titles.length} ${titles.length === 1 ? 'title' : 'titles'} in this library, including ${titles.slice(0, 2).join(' and ')}.`;
  }
}

export const people: Person[] = [...peopleMap.values()];
export const creditsByPerson = creditOf;
