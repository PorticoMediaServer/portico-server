import {musicSignal, type MusicSignalInput, type MusicSignalModel} from '@core/music-signal.ts';
import type {EqualizerSettings} from '../../../platform/playback/audio/equalizer';
import type {WebPlaybackRuntimeSnapshot} from '../../../platform/playback';

const LOSSLESS = new Set(['flac', 'alac', 'pcm', 'wavpack', 'truehd', 'dts_hd']);

/** The drawn signal path for this song, from the server's own committed comparison,
 * the renderer's own observation and the device's EQ setting. Nothing is guessed:
 * an unreadable source has no line, and EQ is only "applied" when the engine says so
 * (the web engine does not process the EQ, so it is always reported as not applied). */
export function watchMusicSignal(snapshot: WebPlaybackRuntimeSnapshot, equalizer: EqualizerSettings | null): MusicSignalModel | undefined {
  const session = snapshot.session, info = session?.playbackInfo, observed = session?.observedPlayback?.audio;
  const sourceAudio = info?.source.audio;
  const eqOn = equalizer?.enabled === true;
  const fallback = snapshot.playback.musicFallback === 'eq' ? 'eq' as const : undefined;
  if (!session || !info || !sourceAudio?.codec) {
    // No committed comparison: a real engine fallback still has a drawn line, from the
    // engine's own fact alone, and never a guessed format.
    if (!fallback) return undefined;
    return musicSignal({source: {}, output: {platform: 'browser', device: 'browser'}, eqOn, eqApplied: false, normalization: 'off', levelingApplied: false, fallback});
  }
  const source: MusicSignalInput['source'] = {codec: sourceAudio.codec, bitDepth: sourceAudio.bitDepth, sampleRate: sourceAudio.sampleRate,
    channels: sourceAudio.channels, layout: sourceAudio.layout, lossless: LOSSLESS.has(sourceAudio.codec), atmos: sourceAudio.objectAudio === 'atmos'};
  const delivered = info.delivered.audio;
  const server: MusicSignalInput['server'] = {route: info.route,
    ...(delivered ? {delivered: {codec: delivered.codec, bitDepth: delivered.bitDepth, sampleRate: delivered.sampleRate, channels: delivered.channels, layout: delivered.layout}} : {}),
    ...(info.losses.some(loss => loss.feature === 'atmos' || loss.feature === 'dtsx') ? {atmosLost: true} : {}),
    ...(session.presentation.audioRender && typeof session.presentation.audioRender.loudnessAppliedDb === 'number' ? {loudnessAppliedDb: session.presentation.audioRender.loudnessAppliedDb} : {})};
  const resampled = !!(observed?.sampleRate && sourceAudio.sampleRate && observed.sampleRate !== sourceAudio.sampleRate);
  const output: MusicSignalInput['output'] = {platform: 'browser', device: 'browser',
    ...(observed?.sampleRate ? {sampleRate: observed.sampleRate} : {}), ...(observed?.bitDepth ? {bitDepth: observed.bitDepth} : {}),
    ...(observed?.handling === 'passthrough' ? {passthrough: true} : observed?.handling !== 'passthrough' && observed?.layout ? {decodedLayout: observed.layout} : {}),
    ...(resampled ? {resampledBy: 'browser' as const} : {})};
  return musicSignal({source, server, output, eqOn, eqApplied: false, normalization: 'off', levelingApplied: false, ...(fallback ? {fallback} : {})});
}
