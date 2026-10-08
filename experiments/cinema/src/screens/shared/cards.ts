import type {CardModel} from '../../design/Card';
import type {Channel, Item, Person, Shape, UserState} from '../../data/types';
import {go, push} from '../../nav/routes';
import {player} from '../../state/player';
import {plural} from '../../design/format';

export const code = (season: number, ep: number) => (season === 0 ? `Special ${ep}` : `S${season} · E${ep}`);

export function fmtRuntime(min: number) {
  if (min < 60) return `${min}m`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  return m ? `${h}h ${m}m` : `${h}h`;
}

export function fmtClock(secs: number) {
  const s = Math.max(0, Math.floor(secs));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(ss).padStart(2, '0')}` : `${m}:${String(ss).padStart(2, '0')}`;
}

export function fmtRemaining(secs: number) {
  const m = Math.round(secs / 60);
  return m >= 60 ? `${Math.floor(m / 60)}h ${m % 60}m left` : `${m}m left`;
}

/** Card shape rule: film, TV, anime, audiobooks (and episodes) are posters; music is square. */
export function shapeFor(item: Item, context: 'row' | 'showDetail' = 'row'): Shape {
  switch (item.kind) {
    case 'album':
    case 'track':
    case 'artist':
    case 'photo':
      return 'square';
    case 'episode':
      return context === 'showDetail' ? 'landscape' : 'poster';
    case 'homevideo':
    case 'photoAlbum':
      return 'landscape';
    default:
      return 'poster';
  }
}

export function subtitleFor(item: Item, lookup?: (id: string) => Item | undefined): string {
  switch (item.kind) {
    case 'movie':
      return [item.year, item.runtime ? fmtRuntime(item.runtime) : null].filter(Boolean).join(' · ');
    case 'show':
      return [item.year, item.anime ? 'Anime' : item.network].filter(Boolean).join(' · ');
    case 'episode': {
      const show = lookup?.(item.showId);
      return [show?.title, code(item.seasonNumber, item.number)].filter(Boolean).join(' · ');
    }
    case 'season':
      return lookup?.(item.showId)?.title ?? '';
    case 'album':
      return `${item.artistName} · ${item.year}`;
    case 'track':
      return `${item.artistName} · ${item.albumTitle}`;
    case 'artist':
      return item.genres[0] ?? 'Artist';
    case 'audiobook':
      return item.author;
    case 'photoAlbum':
      return `${plural(item.photoIds.length, 'photo', 'photos')} · ${item.place}`;
    case 'homevideo':
      return [item.date, item.runtime ? fmtRuntime(item.runtime) : null].filter(Boolean).join(' · ');
    default:
      return String(item.year ?? '');
  }
}

export function toCard(item: Item, st?: UserState, opts: {shape?: Shape; lookup?: (id: string) => Item | undefined; context?: 'row' | 'showDetail'} = {}): CardModel {
  const shape = opts.shape ?? shapeFor(item, opts.context);
  const art =
    shape === 'poster' ? item.art.poster ?? (item.kind === 'episode' ? opts.lookup?.(item.showId)?.art.poster : undefined) :
    shape === 'square' ? item.art.square ?? item.art.thumb ?? item.art.poster :
    item.art.still ?? item.art.backdrop ?? item.art.thumb ?? item.art.poster;
  const isEpisode = item.kind === 'episode';
  return {
    id: item.id,
    title: isEpisode && shape === 'poster' ? opts.lookup?.(item.showId)?.title ?? item.title : item.title,
    subtitle: isEpisode && shape === 'poster' ? `${code(item.seasonNumber, item.number)} · ${item.title}` : subtitleFor(item, opts.lookup),
    art,
    tint: item.tint,
    shape,
    progress: st && st.progress > 0 && !st.watched ? st.progress : undefined,
    watched: st?.watched && item.kind !== 'show' ? true : undefined,
    overline: isEpisode && shape === 'poster' ? code(item.seasonNumber, item.number).replace(' · ', ' ') : undefined,
  };
}

export function personCard(p: Person, role?: string): CardModel {
  return {id: p.id, title: p.name, subtitle: role, art: p.art.headshot, tint: p.tint, shape: 'circle'};
}

export function channelCard(c: Channel, now?: string): CardModel {
  return {id: c.id, title: c.name, subtitle: now ?? c.number, art: c.logo, tint: c.tint, shape: 'landscape'};
}

/** Where selecting an item goes. */
export function openItem(item: Item) {
  switch (item.kind) {
    case 'movie':
    case 'homevideo':
      return push('Movie', {id: item.id});
    case 'show':
      return push('Show', {id: item.id});
    case 'season':
      return push('Show', {id: item.showId, season: item.id});
    case 'episode':
      return push('Episode', {id: item.id});
    case 'artist':
      return push('Artist', {id: item.id});
    case 'album':
      return push('Album', {id: item.id});
    case 'track':
      player.play([item.id], {kind: 'audio'});
      return go('NowPlaying');
    case 'audiobook':
      return push('Audiobook', {id: item.id});
    case 'photoAlbum':
      return push('PhotoAlbum', {id: item.id});
    case 'photo':
      return push('PhotoViewer', {albumId: item.albumId, index: 0});
  }
}

/** Starts playback in the right player for the item. */
export function playItem(item: Item, opts: {from?: number; queue?: string[]} = {}) {
  if (item.kind === 'track' || item.kind === 'album' || item.kind === 'artist') {
    const ids = opts.queue ?? (item.kind === 'album' ? item.trackIds : [item.id]);
    player.play(ids, {kind: 'audio'});
    return go('NowPlaying');
  }
  if (item.kind === 'audiobook') {
    player.play([item.id], {kind: 'book', from: opts.from});
    return go('NowPlaying');
  }
  go('Player', {id: item.id, from: opts.from, queue: opts.queue});
}
