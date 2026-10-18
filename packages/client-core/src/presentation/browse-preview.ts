import type {ContentEntry} from '../library-content.ts';
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import {cardCaption, continueTitle, episodeCode, isContinueRow} from './card-caption.ts';

/**
 * The passive preview above a television's browsing rows (Home, a library's Discover): it
 * describes the card that has focus, and offers nothing to press (Justin, 18 Oct 2026). Every word
 * comes from the entry the row already carries, so it changes the moment focus moves, with no
 * request per card. Home's hero (`homeHero`) is worded by the same rules.
 */
type Translate = (id: MessageId, values?: MessageValues) => string;

export type BrowsePreview = Readonly<{
  entry: ContentEntry;
  /** The show for an episode; the title itself otherwise. */
  title: string;
  /** Under the title: "S2 E3 · Ozymandias" for an episode, the artist for a song or an album, the author for a book. */
  line?: string;
  /** Year · rating · length · up to two genres; what the card's caption says when there are none. */
  meta?: string;
  /** The synopsis on one line of text, at most `PREVIEW_OVERVIEW_LIMIT` characters. */
  overview?: string;
  /** How far in, 0–1. */
  progress: number;
  /** A Continue card resumes when pressed, and the preview says so: "Resume · 24 min left". Words, not a button. */
  resume?: string;
  /** The wide artwork behind the preview, or the cover beside its words when there is none. */
  art: 'backdrop' | 'cover' | 'none';
  /** `backdrop`: the backdrop; `cover`: the poster or square cover. */
  artUrl?: string;
  coverShape: 'poster' | 'square';
}>;

/** The most of a synopsis a preview holds, in characters (Unicode code points), as the server bounds it. */
export const PREVIEW_OVERVIEW_LIMIT = 1000;

const SQUARE = new Set<string>(['song', 'album', 'artist', 'book', 'audiobook_file', 'chapter', 'disc']);
const parts = (subtitle?: string) => (subtitle ? subtitle.split(' · ').map(p => p.trim()).filter(Boolean) : []);

/** "S2 E3 · Ozymandias" for an episode, the artist (and album) for music, the author for a book. */
export function previewLine(entry: ContentEntry, t: Translate): string | undefined {
  if (entry.kind === 'episode') return [episodeCode(entry, t), entry.title].filter(Boolean).join(' · ') || undefined;
  if (entry.artist?.name) return entry.album?.name ? `${entry.artist.name} · ${entry.album.name}` : entry.artist.name;
  if (entry.author) return entry.author;
  return undefined;
}

/** Year · rating · length · up to two genres. A film's subtitle is its year and an episode's leads with its show: neither is said twice. */
export function previewMeta(entry: ContentEntry, duration: (seconds: number) => string): string | undefined {
  const video = entry.kind === 'movie' || entry.kind === 'episode';
  const year = entry.year ? String(entry.year) : undefined;
  const show = entry.kind === 'episode' ? parts(entry.subtitle)[0] : undefined;
  const meta = [
    ...parts(entry.subtitle).filter(p => p !== year && p !== show && video && entry.kind !== 'episode'),
    year, entry.contentRating, entry.duration ? duration(entry.duration) : undefined, ...(entry.genres ?? []).slice(0, 2),
  ].filter(Boolean).join(' · ');
  return meta || undefined;
}

/** How far in, 0–1, and the stretch left in seconds. */
export function previewProgress(entry: ContentEntry): {progress: number; left: number} {
  const progress = entry.duration && entry.progressSeconds ? Math.min(1, Math.max(0, entry.progressSeconds / entry.duration)) : 0;
  const left = entry.duration && entry.progressSeconds ? Math.max(0, entry.duration - entry.progressSeconds) : 0;
  return {progress, left};
}

/** "Resume · 24 min left", "Resume" (a client may have nothing to say of a short stretch), or "Play" for a next episode not yet started. */
export function resumeWords(entry: ContentEntry, t: Translate, duration: (seconds: number) => string): string {
  const {progress, left} = previewProgress(entry);
  const remaining = left >= 60 ? duration(left) : '';
  return remaining ? t('entry.resumeRemaining', {remaining}) : t(progress > 0 ? 'entry.resume' : 'action.play');
}

/**
 * A synopsis as a preview shows it: whitespace and line breaks run together, at most `limit`
 * code points (never half of a surrogate pair), with an ellipsis where it was cut.
 */
export function previewOverview(text: string | undefined, limit = PREVIEW_OVERVIEW_LIMIT): string | undefined {
  if (!text) return undefined;
  // Bounded before any work over the whole text: no synopsis needs more than four times the limit to be read.
  const head = text.length > limit * 4 ? text.slice(0, limit * 4) : text;
  const flat = head.replace(/\s+/g, ' ').trim();
  if (!flat) return undefined;
  let count = 0;
  let end = 0;
  for (const point of flat) {
    if (count === limit) return `${flat.slice(0, end).trimEnd()}…`;
    count++;
    end += point.length;
  }
  return flat;
}

/** The preview of one entry. `resume`: the card is a Continue card, which resumes when pressed. */
export function browsePreview(entry: ContentEntry, t: Translate, options: Readonly<{duration: (seconds: number) => string; resume?: boolean}>): BrowsePreview {
  const {progress} = previewProgress(entry);
  const backdrop = entry.backdropUrl;
  const cover = entry.posterUrl ?? entry.stillUrl;
  return Object.freeze({
    entry,
    title: continueTitle(entry),
    line: previewLine(entry, t),
    meta: previewMeta(entry, options.duration) ?? cardCaption(entry, t),
    overview: previewOverview(entry.overview),
    progress,
    resume: options.resume && entry.playback ? resumeWords(entry, t, options.duration) : undefined,
    art: backdrop ? 'backdrop' : cover ? 'cover' : 'none',
    artUrl: backdrop ?? cover,
    coverShape: SQUARE.has(entry.kind) ? 'square' : 'poster',
  });
}

export type BrowseRowLike = Readonly<{id: string; entries: readonly ContentEntry[]}>;
export type BrowseStart = Readonly<{rowIndex: number; entryIndex: number}>;

/** A card that can be pressed for what it says: on disk (a title whose files are gone still opens, but is not where browsing starts). */
const usable = (entry: ContentEntry) => entry.available !== false;
/** A Continue card that will resume. */
const resumable = (entry: ContentEntry) => usable(entry) && !!entry.playback;

/**
 * Where browsing starts on a television's Home (Justin, 18 Oct 2026): on the Continue card the
 * server put first (`prefer`, its hero), else the first Continue card that will resume, else the
 * first usable card of the first row that has one, else the first card there is. Undefined when
 * no row has a card. `rows` are the rows as shown (titles the viewer removed already left out).
 */
export function browseStart(rows: readonly BrowseRowLike[], prefer?: Readonly<{rowId: string; entryId: string}>): BrowseStart | undefined {
  if (prefer) {
    const rowIndex = rows.findIndex(r => r.id === prefer.rowId);
    const entryIndex = rowIndex >= 0 ? rows[rowIndex]!.entries.findIndex(e => e.id === prefer.entryId) : -1;
    if (entryIndex >= 0 && isContinueRow(prefer.rowId) && resumable(rows[rowIndex]!.entries[entryIndex]!)) return {rowIndex, entryIndex};
  }
  for (let rowIndex = 0; rowIndex < rows.length; rowIndex++) {
    if (!isContinueRow(rows[rowIndex]!.id)) continue;
    const entryIndex = rows[rowIndex]!.entries.findIndex(resumable);
    if (entryIndex >= 0) return {rowIndex, entryIndex};
  }
  for (let rowIndex = 0; rowIndex < rows.length; rowIndex++) {
    const entryIndex = rows[rowIndex]!.entries.findIndex(usable);
    if (entryIndex >= 0) return {rowIndex, entryIndex};
  }
  const rowIndex = rows.findIndex(r => r.entries.length > 0);
  return rowIndex >= 0 ? {rowIndex, entryIndex: 0} : undefined;
}
