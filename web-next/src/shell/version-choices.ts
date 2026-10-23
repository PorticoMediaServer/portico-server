import type {AudioStream, MediaVersion} from '@core/playback-v1/types.ts';

/** Supplied by the canonical VS-3e names and the current locale, never by a playback plan. */
export type VersionChoiceFormatter = Readonly<{
  pictureName(version: MediaVersion): string | undefined;
  audioFormatName(track: AudioStream): string | undefined;
  formatBytes(bytes: number): string | undefined;
}>;

export type VersionChoiceRow = Readonly<{id: string; heading: string; detail: string}>;

/** Sections 21/23: preserve File info order and describe each file's own default audio. */
export function versionChoiceRows(versions: readonly MediaVersion[], formatter: VersionChoiceFormatter): readonly VersionChoiceRow[] {
  return Object.freeze(versions.map(version => {
    const edition = present(version.edition);
    const picture = present(formatter.pictureName(version));
    const track = version.audio.find(track => track.default) ?? version.audio[0];
    const audio = track ? present(formatter.audioFormatName(track)) : undefined;
    const size = version.sizeBytes === undefined ? undefined : present(formatter.formatBytes(version.sizeBytes));
    const detail = [edition ? picture : undefined, audio, size].filter((part): part is string => part !== undefined).join(', ');
    return Object.freeze({id: version.id, heading: edition ?? picture ?? '', detail});
  }));
}

function present(value: string | undefined): string | undefined {
  return value?.trim() ? value : undefined;
}
