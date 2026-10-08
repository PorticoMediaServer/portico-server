import type {Library, LibraryKind} from '@/data/types';
import type {IconName} from '@/ui/Icon';

export const libraryIcon = (kind: LibraryKind): IconName => ({movie: 'movie', tv: 'tv', anime: 'anime', music: 'music', audiobook: 'audiobook', photo: 'photo', homevideo: 'homeVideo'} as const)[kind];
export const libraryKindLabel = (kind: LibraryKind): string => ({movie: 'Movies', tv: 'TV shows', anime: 'Anime', music: 'Music', audiobook: 'Audiobooks', photo: 'Photos', homevideo: 'Home videos'} as const)[kind];
/** The noun for a library's items, for counts ("368 movies"). */
export const itemNoun = (lib: Pick<Library, 'kind'>, n: number): string => {
  const [one, many] = ({movie: ['movie', 'movies'], tv: ['show', 'shows'], anime: ['series', 'series'], music: ['artist', 'artists'], audiobook: ['audiobook', 'audiobooks'], photo: ['album', 'albums'], homevideo: ['video', 'videos']} as const)[lib.kind];
  return `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
};
