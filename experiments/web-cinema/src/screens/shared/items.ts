import type {Album, Audiobook, Episode, Item, Movie, Show, Shape, TrackItem, UserState, ArtRef} from '../../data/types';

/** Where an item opens. Every destination is a durable URL. */
export function linkFor(item: Pick<Item, 'id' | 'kind'> & Partial<{showId: string; albumId: string; seasonNumber: number}>): {to: string; params: Record<string, string>; search?: Record<string, string>} {
  switch (item.kind) {
    case 'show': return {to: '/show/$showId', params: {showId: item.id}};
    case 'season': return {to: '/show/$showId', params: {showId: item.showId ?? ''}, search: {season: String(item.seasonNumber ?? '')}};
    case 'episode': return {to: '/show/$showId/episode/$episodeId', params: {showId: item.showId ?? '', episodeId: item.id}};
    case 'track': return {to: '/media/$itemId', params: {itemId: item.albumId ?? item.id}};
    default: return {to: '/media/$itemId', params: {itemId: item.id}};
  }
}
export const hrefFor = (item: Parameters<typeof linkFor>[0]) => {
  const l = linkFor(item);
  let path = l.to;
  for (const [k, v] of Object.entries(l.params)) path = path.replace(`$${k}`, encodeURIComponent(v));
  const q = l.search ? '?' + new URLSearchParams(l.search).toString() : '';
  return path + q;
};

/**
 * Portico's artwork shape rules: film, TV, anime and audiobooks (episodes too) are posters in rows
 * and grids; music is square; artists and people are circles; home videos and photos are frames.
 */
export function shapeFor(item: Item): Shape {
  switch (item.kind) {
    case 'album': case 'track': return 'square';
    case 'artist': return 'circle';
    case 'homevideo': case 'photoAlbum': case 'photo': return 'landscape';
    default: return 'poster';
  }
}

export function artFor(item: Item, shape: Shape): ArtRef | undefined {
  const a = item.art;
  if (shape === 'landscape') return a.still ?? a.backdrop ?? a.thumb ?? a.poster;
  if (shape === 'square' || shape === 'circle') return a.square ?? a.poster ?? a.thumb;
  return a.poster ?? a.square ?? a.thumb ?? a.backdrop;
}

export const backdropFor = (item: Item): ArtRef | undefined => item.art.backdrop ?? item.art.still ?? item.art.poster ?? item.art.square;

export function formatRuntime(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = Math.round(minutes % 60);
  return h ? (m ? `${h} hr ${m} min` : `${h} hr`) : `${m} min`;
}
export function formatClock(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}` : `${m}:${String(sec).padStart(2, '0')}`;
}
export function formatRemaining(seconds: number): string {
  const m = Math.max(1, Math.round(seconds / 60));
  return m >= 60 ? `${Math.floor(m / 60)} hr ${m % 60 ? `${m % 60} min ` : ''}left` : `${m} min left`;
}

export const episodeCode = (e: Pick<Episode, 'seasonNumber' | 'number'>) => (e.seasonNumber === 0 ? `Special ${e.number}` : `S${e.seasonNumber} · E${e.number}`);

/** The line under a card's title: what kind of thing it is and its key fact. */
export function subtitleFor(item: Item, showTitle?: string): string {
  switch (item.kind) {
    case 'movie': return [item.year, (item as Movie).runtime ? formatRuntime((item as Movie).runtime) : null].filter(Boolean).join(' · ');
    case 'homevideo': return (item as Movie).date ? new Date((item as Movie).date!).toLocaleDateString('en-US', {month: 'long', day: 'numeric', year: 'numeric'}) : String(item.year ?? '');
    case 'show': {
      const s = item as Show;
      const seasons = s.seasonIds.filter(x => !x.endsWith('-s0')).length;
      return `${seasons} ${seasons === 1 ? 'season' : 'seasons'}`;
    }
    case 'episode': return `${showTitle ? showTitle + ' · ' : ''}${episodeCode(item as Episode)}`;
    case 'album': return `${(item as Album).artistName} · ${item.year ?? ''}`;
    case 'artist': return 'Artist';
    case 'track': return (item as TrackItem).artistName;
    case 'audiobook': return (item as Audiobook).author;
    case 'photoAlbum': return `${item.year} · ${(item as {photoIds: string[]}).photoIds.length} photos`;
    default: return item.year ? String(item.year) : '';
  }
}

/** Facts line for heroes: year · rating · runtime · genres. */
export function factsFor(item: Item): string[] {
  const out: string[] = [];
  if (item.kind === 'movie' || item.kind === 'homevideo') {
    const m = item as Movie;
    if (m.year) out.push(String(m.year));
    if (m.contentRating && m.contentRating !== 'NR') out.push(m.contentRating);
    if (m.runtime) out.push(formatRuntime(m.runtime));
  } else if (item.kind === 'show') {
    const s = item as Show;
    out.push(s.yearEnd ? `${s.year}–${s.yearEnd}` : `${s.year}–`);
    out.push(s.contentRating);
    out.push(subtitleFor(s));
  } else if (item.kind === 'audiobook') {
    const b = item as Audiobook;
    if (b.year) out.push(String(b.year));
    out.push(formatRuntime(b.duration / 60));
  } else if (item.kind === 'album') {
    const a = item as Album;
    if (a.year) out.push(String(a.year));
    out.push(`${a.trackIds.length} songs`);
    out.push(formatRuntime(a.duration / 60));
  }
  return out;
}

export const bestResolution = (item: Item): string | null => {
  if (!('versions' in item)) return null;
  const v = (item as Movie).versions;
  if (v.some(x => x.resolution === '4K')) return '4K';
  if (v.some(x => x.hdr)) return 'HDR';
  if (v.some(x => x.resolution === '1080p')) return 'HD';
  return null;
};

export const isPlayable = (item: Item) => ['movie', 'homevideo', 'episode', 'show', 'season', 'album', 'artist', 'track', 'audiobook', 'photoAlbum'].includes(item.kind);
export const isVideo = (item: Item) => ['movie', 'homevideo', 'episode', 'show', 'season'].includes(item.kind);
export const canWatchlist = (item: Item) => ['movie', 'show', 'audiobook'].includes(item.kind);

export function progressLabel(item: Item, st: UserState): string | null {
  if (st.watched) return 'Watched';
  if (st.progress > 0) {
    const total = 'runtime' in item ? (item as Movie).runtime * 60 : 'duration' in item ? (item as Audiobook).duration : 0;
    return total ? formatRemaining(total - st.position) : null;
  }
  return null;
}

export const plural = (n: number, one: string, many = one + 's') => `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;

export function relativeTime(at: number): string {
  const d = Date.now() - at;
  const m = Math.round(d / 60000);
  if (m < 1) return 'Just now';
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} hr ago`;
  const days = Math.round(h / 24);
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days} days ago`;
  return new Date(at).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: new Date(at).getFullYear() === new Date().getFullYear() ? undefined : 'numeric'});
}
export const timeOfDay = (at: number) => new Date(at).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});

/** A future moment in words: "in 6 days", "tomorrow", "in 3 hr". */
export function untilTime(at: number): string {
  const m = Math.round((at - Date.now()) / 60000);
  if (m <= 0) return 'now';
  if (m < 60) return `in ${m} min`;
  const h = Math.round(m / 60);
  if (h < 24) return `in ${h} hr`;
  const d = Math.round(h / 24);
  return d === 1 ? 'tomorrow' : `in ${d} days`;
}
