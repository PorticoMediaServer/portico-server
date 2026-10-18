import type {ContentEntry} from '../library-content.ts';
import type {HomeDocument, HomeRow} from '../home.ts';
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';
import {continueTitle} from './card-caption.ts';
import {previewLine, previewMeta, previewProgress, resumeWords} from './browse-preview.ts';

/**
 * Home's hero (Justin, 2 Oct 2026): what the viewer was last in the middle of. The server names
 * the entry — the newer of the first Continue Watching and the first Continue Listening entry —
 * and this is everything a client writes on it, so the web, iPhone and Apple TV say the same
 * thing. There is no hero when nothing is in progress; Home then opens with its first row.
 */
type Translate = (id: MessageId, values?: MessageValues) => string;

export type HomeHero = Readonly<{
  row: HomeRow;
  entry: ContentEntry;
  /** The show for an episode; the title itself otherwise. */
  title: string;
  /** Under the title: "S2 E3 · Ozymandias" for an episode, the artist for a song or an album, the author for a book. */
  line?: string;
  /** Year · rating · length · up to two genres. */
  meta?: string;
  /** How far in, 0–1; 0 for a next episode that has not been started. */
  progress: number;
  /** The primary button: "Resume · 24 min left", "Resume", or "Play". */
  playLabel: string;
  /** `backdrop` fills the hero with wide artwork; `cover` has only a poster or a square cover to show beside the words. */
  art: 'backdrop' | 'cover';
  /** The cover's shape when `art` is `cover`. */
  coverShape: 'poster' | 'square';
}>;

const SQUARE = new Set<string>(['song', 'album', 'artist', 'book', 'audiobook_file', 'chapter', 'disc']);

/** The hero of a Home document. `hidden` holds entries the viewer has just removed from a row. */
export function homeHero(document: HomeDocument | undefined, t: Translate, options: Readonly<{duration: (seconds: number) => string; hidden?: ReadonlySet<string>}>): HomeHero | undefined {
  const ref = document?.hero;
  if (!document || !ref || options.hidden?.has(ref.entryId)) return undefined;
  const row = document.rows.find(r => r.id === ref.rowId);
  const entry = row?.entries.find(e => e.id === ref.entryId);
  if (!row || !entry) return undefined;
  const {progress} = previewProgress(entry);
  return Object.freeze({
    row, entry,
    title: continueTitle(entry),
    line: previewLine(entry, t),
    meta: previewMeta(entry, options.duration),
    progress,
    playLabel: resumeWords(entry, t, options.duration),
    art: entry.backdropUrl ? 'backdrop' : 'cover',
    coverShape: SQUARE.has(entry.kind) ? 'square' : 'poster',
  });
}
