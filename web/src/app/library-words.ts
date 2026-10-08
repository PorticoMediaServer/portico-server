import {captionFor, formatDuration} from '@core/presentation/index.ts';

/**
 * WEB-LIB-03: nouns by library kind — pure words, no React, so unit tests can
 * exercise them under node:test without the session (.tsx) chain. Runtime
 * callers pass `currentI18n().t`; tests pass `defaultI18n.t`.
 */
export type T = (id: string, vars?: Record<string, unknown>) => string;

const sentence = (key: string) => key.replace(/([A-Z])/g, ' $1').toLowerCase().replace(/^./, c => c.toUpperCase());

/** The browse pivot a count belongs to (see `pivotFor` in Library.tsx). */
export function nounPivotFor(pivot: string | undefined, libraryKind?: string): string {
  if (pivot) return pivot;
  if (libraryKind === 'music') return 'artists';
  if (libraryKind === 'audiobook') return 'books';
  if (libraryKind === 'tv' || libraryKind === 'anime') return 'shows';
  if (libraryKind === 'movie') return 'movies';
  return 'titles';
}

/** "10 artists", "80 movies", "12 books" — via the shared `lib.count` catalogue. */
export function browseCountLabel(t: T, count: number, pivot: string | undefined, libraryKind?: string): string {
  return t('lib.count', {count, pivot: nounPivotFor(pivot, libraryKind)});
}

/** Singular kind for "{Kind} library" eyebrows ("Movie library", "Music library"). */
export function libraryKindSingular(kind: string): string {
  switch (kind) {
    case 'movie': return 'Movie';
    case 'tv': return 'TV show';
    case 'anime': return 'Anime';
    case 'music': return 'Music';
    case 'audiobook': return 'Audiobook';
    default: return 'Library';
  }
}

/** Watching words for video; playing and reading words for music and audiobooks. */
export function kindAwareQuickKey(labelKey: string, kind?: string): string {
  const key = labelKey.replace(/^filter\./, '').replace(/^quick\./, '');
  if (kind === 'music') return key === 'unwatched' ? 'unplayed' : key === 'watched' ? 'played' : labelKey;
  if (kind === 'audiobook') return key === 'unwatched' ? 'notStarted' : key === 'watched' ? 'finished' : labelKey;
  return labelKey;
}

/** Consumer label for a server quick filter in this library kind (US English). */
export function quickFilterLabelForKind(t: T, labelKey: string, libraryKind?: string): string {
  const mapped = kindAwareQuickKey(labelKey, libraryKind);
  const key = mapped.replace(/^filter\./, '');
  try {
    const id = `filter.quick.${key}`;
    if ((t as unknown as {has?: (id: string) => boolean}).has?.(id)) return t(id);
  } catch { /* fall through to sentence fallback */ }
  // Catalogue lookup without `has` (tests pass a bare `t`): try and fall back.
  try {
    const rendered = t(`filter.quick.${key}`);
    if (rendered && rendered !== `filter.quick.${key}`) return rendered;
  } catch { /* fall through */ }
  return sentence(key);
}

type QuickFilterLike = {id: string; labelKey: string};

const availabilityRe = /(^|\.)(missing|available|unavailable)$/;

/**
 * Whether a server quick filter belongs in the consumer bar. Missing/Available
 * live in the Filters panel as Availability; music has no Watchlist and no
 * In-progress chip (Unplayed · Played · Favorites); Collections shows none.
 */
export function isQuickFilterVisible(filter: QuickFilterLike, libraryKind?: string, pivot?: string): boolean {
  if (pivot === 'collections') return false;
  if (availabilityRe.test(filter.labelKey) || availabilityRe.test(filter.id)) return false;
  if (libraryKind === 'music') {
    if (/watchlist/i.test(filter.id) || /watchlist/i.test(filter.labelKey)) return false;
    if (filter.id === 'in-progress' || /in.?progress/i.test(filter.labelKey)) return false;
  }
  return true;
}

type CaptionEntry = {kind: string; subtitle?: string; count?: number; duration?: number};
/** Kinds whose caption `kindCaptionFor` owns: when it has nothing to say, the card says nothing
 * rather than the generic "1 item" / "0 items" count (M4 WEB-LIB-03). */
export const kindOwnsCaption = (kind: string): boolean => kind === 'artist' || kind === 'album' || kind === 'book';
/**
 * Kind-aware secondary caption. Artist → "12 songs" (a browse count is member items), album → "2011 · 11 songs"
 * (year when the subtitle carries it, e.g. "Adele · 2011"), book → author and
 * duration. Never "0 items": a zero or missing count contributes nothing.
 */
export function kindCaptionFor(t: T, entry: CaptionEntry): string | undefined {
  const count = entry.count ?? 0;
  if (entry.kind === 'artist') {
    // A browse entry's count is its member items (the server counts browse_entity_membership):
    // for an artist that is songs, not albums.
    return count > 0 ? t('title.songCount', {count}) : undefined;
  }
  if (entry.kind === 'album') {
    const parts: string[] = [];
    const year = entry.subtitle?.split(' · ').find(p => /^(19|20)\d{2}$/.test(p.trim()));
    if (year) parts.push(year.trim());
    if (count > 0) parts.push(t('title.songCount', {count}));
    return parts.length ? parts.join(' · ') : undefined;
  }
  if (entry.kind === 'book') {
    const parts: string[] = [];
    if (entry.subtitle) parts.push(entry.subtitle);
    if (entry.duration && entry.duration > 0) parts.push(formatDuration(entry.duration));
    return parts.length ? parts.join(' · ') : undefined;
  }
  return captionFor(entry as Parameters<typeof captionFor>[0]);
}

/* ── WEB-DETAIL-10: album and artist track lists ───────────────────── */

/** The performer from a song's server subtitle ("Disc 1 · Track 3 · Adele" → "Adele"). */
export function songArtistFromSubtitle(subtitle: string | undefined): string | undefined {
  if (!subtitle) return undefined;
  const parts = subtitle.split(' · ').filter(p => !/^(Disc|Track) \d+$/.test(p));
  return parts.length ? parts[parts.length - 1] : undefined;
}

/** A guest artist on an album track is worth a line; the album artist and "Track 3" aren't. */
export function trackSubtitleForAlbum(trackSubtitle: string | undefined, albumArtist: string | undefined): string | undefined {
  const artist = songArtistFromSubtitle(trackSubtitle);
  if (!artist) return undefined;
  if (albumArtist && artist === albumArtist) return undefined;
  return artist;
}

/** The disc a song subtitle belongs to ("Disc 2 · Track 1 · …" → 2); none when unnumbered. */
export function discNumberFromSubtitle(subtitle: string | undefined): number | undefined {
  if (!subtitle) return undefined;
  const hit = subtitle.split(' · ').find(p => /^Disc \d+$/.test(p));
  return hit ? Number(hit.slice(5)) : undefined;
}

/** Album hero facts: "Album · 2011 · 11 songs · 48 min" (year/duration omitted when unknown). */
export function albumHeroFacts(t: T, opts: {/** The release type the server names (EP, Single, Compilation…); "Album" when it names none. */ type?: string; year?: string; songCount?: number; durationSeconds?: number}): (string | undefined)[] {
  const out: (string | undefined)[] = [opts.type || t('title.kind', {kind: 'album'})];
  if (opts.year) out.push(opts.year);
  if (opts.songCount && opts.songCount > 0) out.push(t('title.songCount', {count: opts.songCount}));
  if (opts.durationSeconds && opts.durationSeconds > 0) out.push(formatDuration(opts.durationSeconds));
  return out;
}

/** Artist hero facts: album and song counts from the server's section totals. */
export function artistHeroFacts(t: T, opts: {albumCount?: number; songCount?: number}): (string | undefined)[] {
  const out: (string | undefined)[] = [];
  if (opts.albumCount && opts.albumCount > 0) out.push(t('title.albumCount', {count: opts.albumCount}));
  if (opts.songCount && opts.songCount > 0) out.push(t('title.songCount', {count: opts.songCount}));
  return out;
}

/* ── WEB-LIB-05: "See all" keeps the row's meaning ─────────────────── */

export type SeeAllSection = {id: string; headingKey: string};
export type SeeAllTarget = {view: 'browse'; sort?: string; direction?: 'asc' | 'desc'; row?: never} | {view: 'discover'; row: string};

/**
 * Maps a Discover rail to its full destination. Recently-added rows sort by
 * date added, newest first; every other row opens as a full-page row view
 * (`?row=<id>`) that pages the row's own cursor.
 */
export function seeAllTarget(section: SeeAllSection): SeeAllTarget {
  if (/recent|added/i.test(section.id + section.headingKey)) {
    return {view: 'browse', sort: 'added', direction: 'desc'};
  }
  return {view: 'discover', row: section.id};
}
