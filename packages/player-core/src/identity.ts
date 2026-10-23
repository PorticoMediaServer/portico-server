import {queueScope, type QueueScope} from './queues.ts';
import {invalidState, queueOccurrenceId, record, wireId} from './validation.ts';
export type PlaybackIdentity = Readonly<{scope: QueueScope; run: number}>;
/** A real queue occurrence; item and entry IDs never stand for a channel. */
export type QueuePlaybackFence = Readonly<{kind:'queue'; playback: PlaybackIdentity; sessionId: string; itemId: string; sourceId: string; entryId: string; showId?: string; seasonId?: string; generation: number}>;
/** Reference generation belongs to the channel; generation belongs to the attached media. */
export type ChannelPlaybackFence = Readonly<{kind:'channel'; playback:PlaybackIdentity; sessionId:string; channelId:string; channelGeneration:string; generation:number}> & (
  Readonly<{channelKind:'live-source';sourceId:string}> | Readonly<{channelKind:'library-channel';sourceId?:string}>
);
export type PlaybackFence = QueuePlaybackFence | ChannelPlaybackFence;
export function playbackIdentity(value: PlaybackIdentity): PlaybackIdentity {
  if (!record(value) || !Number.isSafeInteger(value.run) || value.run < 1) invalidState();
  return Object.freeze({scope: queueScope(value.scope), run: value.run});
}
export function playbackFence(value:QueuePlaybackFence):QueuePlaybackFence;
export function playbackFence(value:ChannelPlaybackFence):ChannelPlaybackFence;
export function playbackFence(value:PlaybackFence):PlaybackFence;
export function playbackFence(value: PlaybackFence): PlaybackFence {
  if (!record(value) || !wireId(value.sessionId) || !Number.isSafeInteger(value.generation) || value.generation < 1) invalidState();
  const common={playback:playbackIdentity(value.playback),sessionId:value.sessionId,generation:value.generation};
  if(value.kind==='queue') {
    if(!wireId(value.itemId)||!wireId(value.sourceId)||!queueOccurrenceId(value.entryId) || value.showId!==undefined&&!wireId(value.showId) || value.seasonId!==undefined&&!wireId(value.seasonId) || ['channelKind','channelId','channelGeneration'].some(k=>k in value))invalidState();
    return Object.freeze({...common,kind:'queue',itemId:value.itemId,sourceId:value.sourceId,entryId:value.entryId,...(value.showId?{showId:value.showId}:{}),...(value.seasonId?{seasonId:value.seasonId}:{})});
  }
  if(value.kind!=='channel' || !wireId(value.channelId) || !wireId(value.channelGeneration) || ['itemId','entryId','showId','seasonId'].some(k=>k in value))invalidState();
  if(value.channelKind==='live-source') {
    if(!wireId(value.sourceId))invalidState();
    return Object.freeze({...common,kind:'channel',channelKind:value.channelKind,sourceId:value.sourceId,channelId:value.channelId,channelGeneration:value.channelGeneration});
  }
  // The real library reference has no separate source, or explicitly names its channel.
  if(value.channelKind!=='library-channel' || value.sourceId!==undefined && value.sourceId!=='' && value.sourceId!==value.channelId)invalidState();
  return Object.freeze({...common,kind:'channel',channelKind:value.channelKind,channelId:value.channelId,channelGeneration:value.channelGeneration,...(value.sourceId===undefined?{}:{sourceId:value.sourceId})});
}
export const sameScope = (a: QueueScope, b: QueueScope): boolean => Object.keys(a).every(k => a[k as keyof QueueScope] === b[k as keyof QueueScope]);
export const samePlayback = (a: PlaybackIdentity | undefined, b: PlaybackIdentity): boolean => !!a && a.run === b.run && sameScope(a.scope, b.scope);
export const sameFence = (a: PlaybackFence | undefined, b: PlaybackFence): boolean => !!a && a.kind===b.kind && samePlayback(a.playback,b.playback) && a.sessionId === b.sessionId && a.sourceId === b.sourceId && a.generation === b.generation && (
  a.kind==='queue' && b.kind==='queue' && a.itemId===b.itemId && a.entryId===b.entryId ||
  a.kind==='channel' && b.kind==='channel' && a.channelKind===b.channelKind && a.channelId===b.channelId && a.channelGeneration===b.channelGeneration
);
