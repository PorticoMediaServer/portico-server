import type {MediaVersion} from '@core/playback-v1/types.ts';
import type {WebPlaybackVersionChoices} from './versions.ts';

/** Bind the MAIN canonical versionChoiceRow(version, words), with its locale environment.
 * That formatter alone owns section23 file-default audio and VS3e two-line wording. */
export type PlaybackVersionRowFormatter = (version: MediaVersion) => Readonly<{primary: string; secondary: string}>;
export type PlaybackVersionRow = Readonly<{id: string; heading: string; detail: string}>;

/** An adapter for the one controlled Watch chooser. It never clones the runtime-issued
 * choices, interprets joined labels, formats a track, or supplies placeholder copy. */
export function playbackVersionRows(choices: WebPlaybackVersionChoices, formatRow: PlaybackVersionRowFormatter): readonly PlaybackVersionRow[] | null {
  if (!choices.isCurrent()) return null;
  const offered = choices.options.versions;
  if (new Set(offered.map(version => version.id)).size !== offered.length) throw new Error('playback_version_rows_invalid');
  const eligible = offered.filter(version => version.id !== choices.failedVersionId);
  if (eligible.length !== choices.versions.length || eligible.some((version, index) => version !== choices.versions[index])) throw new Error('playback_version_rows_invalid');
  const rows: PlaybackVersionRow[] = [];
  for (const version of eligible) {
    const text = formatRow(version);
    if (!choices.isCurrent()) return null;
    if (typeof text.primary !== 'string' || !text.primary.trim() || typeof text.secondary !== 'string') throw new Error('playback_version_rows_invalid');
    rows.push(Object.freeze({id: version.id, heading: text.primary, detail: text.secondary}));
  }
  return Object.freeze(rows);
}
