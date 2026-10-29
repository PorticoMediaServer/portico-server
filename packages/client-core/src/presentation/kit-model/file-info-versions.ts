import {fileInfoBytesValue, type FileInfoGroupRow, type FileInfoHeadTrackRow, type FileInfoTrackRow} from '../../file-info-pages.ts';
import type {FileFacts} from './files.ts';
import type {TrackFacts} from './tracks.ts';

/** One version's facts for fileInfoModel, from its group row and whatever windows of its tracks
 * are resident (web: the head file's raw tracks; native: the head's offered tracks). Ordinals
 * come from the server, so a windowed list keeps its numbering; tracks not yet read are simply
 * absent, never guessed from a count. An unrepresentable length is left out, never zero. */
export function fileInfoVersionFacts(group: FileInfoGroupRow, audio: readonly (FileInfoTrackRow | FileInfoHeadTrackRow)[] = [], subtitles: readonly (FileInfoTrackRow | FileInfoHeadTrackRow)[] = []): FileFacts {
  const video = group.video, seconds = group.durationMs !== undefined && group.durationMs > 0 ? group.durationMs / 1000 : undefined, bytes = fileInfoBytesValue(group.size);
  const track = (lane: 'audio' | 'subtitle') => (row: FileInfoTrackRow | FileInfoHeadTrackRow): TrackFacts => ({
    id: 'id' in row ? row.id : `${group.headOccurrence}:${lane}:${row.ordinal}`, ordinal: row.ordinal, codec: row.codec, language: row.language, title: row.title,
    channels: row.channels, channelLayout: row.channelLayout, forced: row.forced, default: row.default, external: row.external,
    commentary: row.commentary, hearingImpaired: row.hearingImpaired,
    objectAudio: lane === 'audio' && (row.objectAudio === 'atmos' || row.objectAudio === 'dtsx') ? row.objectAudio : undefined,
  });
  return {
    id: group.id, ordinal: group.ordinal, available: true, edition: group.edition, container: group.container, sizeBytes: bytes,
    durationSeconds: seconds,
    // A whole-version rate from the version's own totals, as the file rate always was: size over length.
    bitRate: bytes !== undefined && bytes > 0 && seconds ? Math.round(bytes * 8 / seconds) : undefined,
    videoCodec: video?.codec, videoProfile: video?.profile, width: video?.width, height: video?.height, dynamicRange: video?.dynamicRange, hdr10Plus: video?.hdr10Plus,
    frameRate: video?.frameRate, bitDepth: video?.bitDepth,
    audio: audio.map(track('audio')), subtitles: subtitles.map(track('subtitle')),
  };
}
