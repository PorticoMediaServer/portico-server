import type {Entry} from '@/data/types';

/**
 * Where an entry opens. URLs follow the current Portico web app, so links
 * and bookmarks carry over: /media/:id for titles, /show/:id for shows,
 * /show/:id/episode/:id for episodes, /person/:id, /saved/:kind/:id, and
 * /library/:library/:kind/:id for genre, studio, collection and similar pages.
 */
export function hrefFor(e: Entry): string | undefined {
  const lib = e.libraryId ?? '';
  switch (e.kind) {
    case 'movie':
    case 'homevideo':
    case 'album':
    case 'artist':
    case 'audiobook':
    case 'photoAlbum':
      return `/media/${enc(e.id)}`;
    case 'show':
      return `/show/${enc(e.id)}`;
    case 'season':
      return e.parentId ? `/show/${enc(e.parentId)}?season=${enc(e.id)}` : undefined;
    case 'episode':
      return e.parentId ? `/show/${enc(e.parentId)}/episode/${enc(e.id)}` : undefined;
    case 'person':
      return `/person/${enc(e.id)}`;
    case 'collection':
      if (e.id.startsWith('pc-')) return `/saved/collection/${enc(e.id)}`;
      return `/library/${enc(lib || 'lib-movies')}/collection/${enc(e.id)}`;
    case 'playlist':
      return `/saved/playlist/${enc(e.id)}`;
    case 'view':
      return `/saved/view/${enc(e.id)}`;
    case 'photo':
      return e.parentId ? `/media/${enc(e.parentId)}?photo=${enc(e.id)}` : undefined;
    case 'track':
      return e.parentId ? `/media/${enc(e.parentId)}` : undefined;
    case 'author':
      return `/library/lib-audiobooks/author/${enc(e.id.replace(/^author:/, ''))}`;
    case 'series':
      return `/library/lib-audiobooks/series/${enc(e.id.replace(/^series:/, ''))}`;
    case 'genre':
    case 'studio':
      return `/library/${enc(lib)}/${e.kind}/${enc(e.id)}`;
    default:
      return undefined;
  }
}
const enc = encodeURIComponent;

/** Words for an entry's kind, as used in menus and toasts. */
export function kindNoun(e: Pick<Entry, 'kind'>): string {
  return ({movie: 'movie', show: 'show', season: 'season', episode: 'episode', artist: 'artist', album: 'album', track: 'song', audiobook: 'audiobook', photoAlbum: 'album', photo: 'photo', homevideo: 'video', collection: 'collection', playlist: 'playlist', person: 'person', channel: 'channel', program: 'program', recording: 'recording', view: 'saved view'} as Record<string, string>)[e.kind] ?? 'item';
}
