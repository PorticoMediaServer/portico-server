import type {QueuePlaylistSave} from './playback-v1/queue.ts';
import type {AudioEffectsSettings} from './audio-effects.ts';
import type {ListeningService, ListeningTarget} from './listening.ts';
import type {QueueAdvanceMode, QueuePostPlay} from './post-play.ts';
import type {PreparedChoice} from './prepared-media.ts';
import type {QueueIntent, QueueItemInput, QueueSnapshot, QueueState} from './queues.ts';

/** The queue as the player shows it: the snapshot, what is playing, what comes next, and the
 * titles of its visible window. */
export type QueuePlaybackView=Readonly<{queue:QueueSnapshot;highestSequence:string;current:Readonly<{id:string;generation:number;state:string}>|null;next:Readonly<{entryId:string|null;available:boolean;reason:'ready'|'end'|'unavailable'}>;items:readonly Readonly<{entryId:string;itemId:string;title:string;kind:string;libraryId:string}>[];postPlay:QueuePostPlay}>;
/** A completion the player holds for the post-play card instead of advancing. */
export type HeldCompletion=Readonly<{sessionId:string;intentId:number;nextEntryId:string|null;nextAvailable:boolean;reason:'ready'|'end'|'unavailable';postPlay:QueuePostPlay}>;
export type CompletionPolicy=(context:Readonly<{kind:string;itemId:string}>)=>'advance'|'hold';
export type QueuePlayerState=Readonly<{phase:'loading'|'ready'|'preparing'|'recovery-required'|'error'|'disposed';view:QueuePlaybackView|null;error:string|null;completion:HeldCompletion|null}>;
/** The Up Next edits a player accepts (a queue workspace). */
export type QueueWorkspaceService=Readonly<{getSnapshot():QueueState;subscribe(listener:()=>void):()=>void;mutate(intent:QueueIntent):Promise<void>}>;

/**
 * What PlaybackService and the player screens use of the device queue player (`V1QueuePlayer`,
 * Playback Protocol v1 queues and sessions).
 */
export interface QueueController {
  readonly workspace: Readonly<{service: QueueWorkspaceService}>;
  readonly listening: ListeningService;
  getSnapshot(): QueuePlayerState;
  subscribe(fn: () => void): () => void;
  connect(): () => void;
  dispose(): void;
  playItem(itemId: string, startSeconds?: number, prepared?: PreparedChoice, quality?: string, audioStream?: number): Promise<void>;
  playEntries(entries: readonly QueueItemInput[], options?: {shuffle?: boolean; startSeconds?: number}): Promise<void>;
  journey(target: ListeningTarget, action: 'play' | 'shuffle' | 'mix' | 'enqueue', resume?: boolean, options?: {signal?: AbortSignal; startItem?: string; startSeconds?: number}): Promise<unknown>;
  enqueue(item: QueueItemInput, next?: boolean): Promise<void>;
  enqueueMany(entries: readonly QueueItemInput[], next?: boolean): Promise<void>;
  playEntry(queueId: string, entryId: string, expectedRevision: string): Promise<void>;
  /** Saves the queue, in play order, as a new playlist, following a long save to the end (NEW-37). Absent where the queue can't be saved. */
  saveAsPlaylist?(queueId: string, name: string, onProgress?: (save: QueuePlaylistSave) => void, signal?: AbortSignal): Promise<QueuePlaylistSave>;
  next(queueId: string, expectedRevision: string): Promise<void>;
  previous(): Promise<void>;
  /** Whether `next` follows `current` on one album (the engine then joins, not crossfades). */
  sameAlbum?(currentItemId: string, nextItemId: string): boolean;
  check(retryUnknown?: boolean): Promise<void>;
  cancel(invalidateSelection?: boolean): Promise<void>;
  leave(): void;
  setCompletionPolicy(policy: CompletionPolicy): void;
  continueCompletion(mode?: QueueAdvanceMode): Promise<void>;
  releaseCompletion(): void;
  // Prepared audio edges (gapless, crossfade; spec §18).
  saveAudioEffects(settings: AudioEffectsSettings): Promise<void>;
  invalidateAudioPreparation(): void;
  seekRetiringAudio(seconds: number): boolean;
  audioPrepared(token: string): void;
  commitAudio(token: string): Promise<void>;
  audioBoundary(token: string, position: number): Promise<void>;
  discardAudio(message: string): void;
  audioRenderFailed(message: string): void;
}

/** A queue entry for one item as it is (no edition or part chosen). */
export const queueItemInput=(itemId:string):QueueItemInput=>({itemId,editionId:null,partId:null,sourceContext:{kind:'item',id:itemId,revision:null,entryId:null}});
