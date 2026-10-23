import {sameFence} from '../../../packages/player-core/src/index.ts';
import {musicSignal, type MusicSignalModel, type MusicServerSignal} from '@core/music-signal.ts';
import type {WebPlaybackRuntimeSnapshot} from '../platform/playback';
import type {BrowserAudioFacts} from '../platform/playback/audio/output';

/** Source comes only from the actual selected session/version/audio track. Converted render
 * rate/depth describe delivery and never fill absent original-file facts. */
export function browserMusicSignal(snapshot: WebPlaybackRuntimeSnapshot, facts: BrowserAudioFacts | undefined, eqOn: boolean): MusicSignalModel | null {
  const {fence, session, presentation, options, playback} = snapshot;
  if (snapshot.disposed || fence?.kind !== 'queue' || presentation?.kind !== 'music' || !sameFence(presentation.fence, fence)
    || !sameFence(snapshot.dock.local?.fence, fence) || session?.id !== fence.sessionId || session.itemId !== fence.itemId
    || session.presentation.generation !== fence.generation || options?.itemId !== fence.itemId || playback.phase !== 'ready'
    || playback.audioEffects.rendering && !facts) return null;
  const version = options.versions.find(value => value.id === session.versionId);
  const track = session.presentation.audioTrackId ? version?.audio.find(value => value.id === session.presentation.audioTrackId) : undefined;
  if (!track) return null;
  const render = session.presentation.audio;
  const action = session.presentation.decision.audio?.action;
  const route: MusicServerSignal['route'] | undefined = action === 'transcode' ? 'converted' : action === 'copy' ? 'repackaged' : action === 'direct' ? 'original' : undefined;
  return musicSignal({source: {codec: track.codec, sampleRate: track.sampleRate, bitDepth: track.bitDepth, channels: track.channels, layout: track.layout, atmos: track.atmos,
      ...(render?.sourceBitrateBps !== undefined ? {bitrateBps: render.sourceBitrateBps} : {})},
    ...(route ? {server: {route, ...(render && render.mode !== 'unavailable' ? {delivered: {codec: render.codec, sampleRate: render.sampleRate, bitDepth: render.bitDepth, channels: render.channels}, loudnessAppliedDb: render.loudnessAppliedDb} : {})}} : {}),
    output: {platform: 'browser', device: 'browser', ...(facts ? {sampleRate: facts.contextRate, ...(facts.resampledToContext ? {resampledBy: 'browser' as const} : {})} : {})},
    eqOn, eqApplied: facts?.eqApplied ?? false, normalization: playback.audioEffects.settings.normalization,
    levelingApplied: facts?.levelingApplied ?? false, levelingGainDb: facts?.levelingGainDb});
}
