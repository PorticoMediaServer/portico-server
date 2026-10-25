import {musicSignal, type MusicSignalInput, type MusicSignalModel} from '@core/music-signal.ts';
import type {EqualizerSettings} from '../../../platform/playback/audio/equalizer';
import type {WebPlaybackRuntimeSnapshot} from '../../../platform/playback';

/** The drawn signal path for this song, from the server's own committed comparison,
 * the renderer's own observation and the device's EQ setting. Nothing is guessed:
 * an unreadable source has no line, and processing is only "applied" when the
 * current session's renderer reports it. Requested settings are not observations. */
export function watchMusicSignal(snapshot: WebPlaybackRuntimeSnapshot, equalizer: EqualizerSettings | null): MusicSignalModel | undefined {
  const session = snapshot.session, info = session?.playbackInfo, observed = session?.observedPlayback?.audio;
  const sourceAudio = info?.source.audio;
  const eqOn = equalizer?.enabled === true;
  const effects = snapshot.playback.audioEffects;
  const fallback = snapshot.playback.musicFallback === 'eq' ? 'eq' as const : undefined;
  if (!session || !info || !sourceAudio?.codec) {
    // No committed comparison: a real engine fallback still has a drawn line, from the
    // engine's own fact alone, and never a guessed format.
    if (!fallback) return undefined;
    return musicSignal({source: {}, output: {platform: 'browser', device: 'browser'}, eqOn, eqApplied: false, normalization: 'off', levelingApplied: false, fallback});
  }
  // The committed comparison names this exact selected track: its numbers and its own measured
  // losslessness. Nothing is inferred from a codec or a channel count, and an absent fact stays
  // absent (a WavPack source cannot prove losslessness).
  const source: MusicSignalInput['source'] = {codec: sourceAudio.codec,
    ...(sourceAudio.bitDepth === undefined ? {} : {bitDepth: sourceAudio.bitDepth}),
    ...(sourceAudio.sampleRate === undefined ? {} : {sampleRate: sourceAudio.sampleRate}),
    ...(sourceAudio.channels === undefined ? {} : {channels: sourceAudio.channels}),
    ...(sourceAudio.layout === undefined ? {} : {layout: sourceAudio.layout}),
    ...(sourceAudio.lossless === undefined ? {} : {lossless: sourceAudio.lossless}),
    ...(sourceAudio.objectAudio === 'atmos' ? {atmos: true} : {})};
  const delivered = info.delivered.audio;
  const server: MusicSignalInput['server'] = {route: info.route,
    ...(delivered ? {delivered: {codec: delivered.codec, bitDepth: delivered.bitDepth, sampleRate: delivered.sampleRate, channels: delivered.channels, layout: delivered.layout, ...(delivered.lossless === undefined ? {} : {lossless: delivered.lossless})}} : {}),
    ...(info.losses.some(loss => loss.feature === 'atmos' || loss.feature === 'dtsx') ? {atmosLost: true} : {}),
    ...(session.presentation.audio && typeof session.presentation.audio.loudnessAppliedDb === 'number' ? {loudnessAppliedDb: session.presentation.audio.loudnessAppliedDb} : {})};
  const resampled = !!(observed?.sampleRate && sourceAudio.sampleRate && observed.sampleRate !== sourceAudio.sampleRate);
  const output: MusicSignalInput['output'] = {platform: 'browser', device: 'browser',
    ...(observed?.sampleRate ? {sampleRate: observed.sampleRate} : {}), ...(observed?.bitDepth ? {bitDepth: observed.bitDepth} : {}),
    ...(observed?.handling === 'passthrough' ? {passthrough: true} : observed?.layout ? {decodedLayout: observed.layout} : {}),
    ...(resampled ? {resampledBy: 'browser' as const} : {})};
  return musicSignal({source, server, output, eqOn, eqApplied: effects.eqApplied,
    normalization: effects.settings.normalization, levelingApplied: effects.levelingApplied, levelingGainDb: effects.levelingGainDb, ...(fallback ? {fallback} : {})});
}
