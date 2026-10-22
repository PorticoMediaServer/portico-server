import {queueScope, type QueueScope} from './queues.ts';
import {invalidState, record, wireId} from './validation.ts';
export type PlaybackIdentity = Readonly<{scope: QueueScope; run: number}>;
/** A source/entry generation fences pending seeks and renderer callbacks within one playback. */
export type PlaybackFence = Readonly<{playback: PlaybackIdentity; sessionId: string; itemId: string; sourceId: string; entryId: string; showId?: string; seasonId?: string; generation: number}>;
export function playbackIdentity(value: PlaybackIdentity): PlaybackIdentity {
  if (!record(value) || !Number.isSafeInteger(value.run) || value.run < 1) invalidState();
  return Object.freeze({scope: queueScope(value.scope), run: value.run});
}
export function playbackFence(value: PlaybackFence): PlaybackFence {
  if (!record(value) || !(['sessionId','itemId','sourceId','entryId'] as const).every(k => wireId(value[k])) || value.showId!==undefined&&!wireId(value.showId) || value.seasonId!==undefined&&!wireId(value.seasonId) || !Number.isSafeInteger(value.generation) || value.generation < 1) invalidState();
  return Object.freeze({sessionId:value.sessionId,itemId:value.itemId,sourceId:value.sourceId,entryId:value.entryId,generation:value.generation,...(value.showId?{showId:value.showId}:{}),...(value.seasonId?{seasonId:value.seasonId}:{}),playback:playbackIdentity(value.playback)});
}
export const sameScope = (a: QueueScope, b: QueueScope): boolean => Object.keys(a).every(k => a[k as keyof QueueScope] === b[k as keyof QueueScope]);
export const samePlayback = (a: PlaybackIdentity | undefined, b: PlaybackIdentity): boolean => !!a && a.run === b.run && sameScope(a.scope, b.scope);
export const sameFence = (a: PlaybackFence | undefined, b: PlaybackFence): boolean => !!a && samePlayback(a.playback,b.playback) && a.sessionId === b.sessionId && a.itemId === b.itemId && a.sourceId === b.sourceId && a.entryId === b.entryId && a.generation === b.generation;
