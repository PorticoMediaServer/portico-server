import type {PlayerChaptersService, PlayerChaptersSnapshot, PlayerChaptersTarget, PlayerChapter} from '@core/player-chapters.ts';
import {lyricSeekPosition} from '@core/lyrics.ts';
import type {LyricRevision, LyricsService, LyricsSnapshot} from '@core/lyrics.ts';
import type {QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1PlaybackTarget, V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebQueueListPort, WebQueueListSnapshot} from './queue-list.ts';

/** The caller's own view lifetime, separate from the runtime's record. */
export type WebListeningLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;

/** The immutable target captured from one admitted playback before any async work. */
export type WebListeningTarget = Readonly<{
  libraryId: string; itemId: string; sessionId: string; sessionGeneration: number; sourceId?: string;
  versionId: string; playbackRun: number; queueId: string; entryId: string;
}>;

export type WebListeningChaptersPort = Readonly<{
  getSnapshot(): PlayerChaptersSnapshot;
  subscribe(listener: () => void): () => void;
  /** The captured original target only; there is no select(newTarget). */
  load(): Promise<void>;
  retry(): Promise<void>; refresh(): Promise<void>; next(): Promise<void>; previous(): Promise<void>;
  /** The drawn row and the service snapshot that rendered it are the command's proof. */
  seek(shown: PlayerChaptersSnapshot, chapter: PlayerChapter, lifetime: WebListeningLifetime): Promise<void>;
}>;

export type WebListeningLyricsPort = Readonly<{
  getSnapshot(): LyricsSnapshot;
  subscribe(listener: () => void): () => void;
  refresh(): Promise<void>;
  /** Resource selection only, offered by the service; the shown snapshot must still be current. */
  choose(shown: LyricsSnapshot, resource: LyricRevision | null, lifetime: WebListeningLifetime): Promise<void>;
  /** Line seek through the existing guarded runtime seek, never raw media time. */
  seek(shown: LyricsSnapshot, index: number, lifetime: WebListeningLifetime): Promise<void>;
}>;

export type WebListeningQueuePort = Readonly<{
  /** The existing queueList port: immutable snapshot, read and edit of loaded rows only.
   * No continuation/More capability is exposed: Media's captureEditWindow is not a public
   * export, so this facade stays on published methods until that canonical API exists. */
  list: WebQueueListPort;
  /** Reuses V1QueuePlayer.setQueuePolicy; the server's returned policy is the only state. */
  policy(shown: WebQueueListSnapshot, intent: V1QueuePolicyIntent, lifetime: WebListeningLifetime): Promise<void>;
  /** One same-key save of the original queue; progress is only this save's own receipt. */
  save(shown: WebQueueListSnapshot, name: string, lifetime: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void): Promise<QueuePlaylistSave>;
}>;

export type WebListeningPort = Readonly<{
  fence: PlaybackFence; target: WebListeningTarget;
  chapters: WebListeningChaptersPort; lyrics: WebListeningLyricsPort; queue?: WebListeningQueuePort;
  isCurrent(): boolean;
  /** Idempotent: detaches this record before aborting/disposing its own services. */
  dispose(): void;
}>;

export type WebListeningInput = Readonly<{
  fence: PlaybackFence; lifetime: WebListeningLifetime; target: WebListeningTarget;
  /** Runtime record, exact fence and caller lifetime, checked before and after every IO/action. */
  current(): boolean;
  chapters: PlayerChaptersService; chaptersTarget: PlayerChaptersTarget;
  lyrics: LyricsService; lyricsEnabled(): boolean;
  queue?: WebQueueListPort;
  policy(target: V1PlaybackTarget, revision: string, intent: V1QueuePolicyIntent, guard: () => boolean): Promise<void>;
  save(queueId: string, name: string, signal: AbortSignal, guard: () => boolean, onProgress?: (save: QueuePlaylistSave) => void): Promise<QueuePlaylistSave>;
  seek(seconds: number, guard: () => boolean): Promise<void>;
}>;

const stale: () => never = () => {throw new Error('stale_listening_port');};
const unavailable: () => never = () => {throw new Error('listening_command_unavailable');};

/** One captured listening record. Everything it exposes delegates to the existing
 * PlayerChaptersService, LyricsService, queueList port and V1QueuePlayer methods; this
 * factory adds only target/lifetime guards, never a player, session, queue or engine. */
export function createWebListeningPort(input: WebListeningInput): WebListeningPort {
  const record = new AbortController();
  let disposed = false;
  const current = () => !disposed && !record.signal.aborted && input.current() && !input.lifetime.signal.aborted && input.lifetime.isCurrent();
  const require = () => {if (!current()) stale();};
  const settled = async <T>(work: Promise<T>): Promise<T> => {
    const value = await work;
    require();
    return value;
  };
  const chapters = Object.freeze({
    getSnapshot: () => input.chapters.getSnapshot(),
    subscribe: (listener: () => void) => current() ? input.chapters.subscribe(listener) : () => {},
    load: async () => {require(); await settled(input.chapters.select(input.chaptersTarget));},
    retry: async () => {require(); await settled(input.chapters.retry());},
    refresh: async () => {require(); await settled(input.chapters.refresh());},
    next: async () => {require(); await settled(input.chapters.next());},
    previous: async () => {require(); await settled(input.chapters.previous());},
    seek: async (shown: PlayerChaptersSnapshot, chapter: PlayerChapter, lifetime: WebListeningLifetime) => {
      require();
      if (lifetime.signal.aborted || !lifetime.isCurrent()) stale();
      const snapshot = input.chapters.getSnapshot();
      if (shown !== snapshot || snapshot.phase !== 'ready' || !snapshot.data) unavailable();
      const data = snapshot.data, target = input.target;
      if (data.scope.libraryId !== target.libraryId || data.scope.itemId !== target.itemId || data.scope.sessionId !== target.sessionId
        || data.scope.generation !== target.sessionGeneration || (target.sourceId !== undefined && data.scope.sourceId !== target.sourceId)) unavailable();
      const row = data.chapters.find(candidate => candidate.id === chapter.id);
      if (!row || row.index !== chapter.index || row.startSeconds !== chapter.startSeconds || row.action.id !== 'seek'
        || row.action.positionSeconds !== chapter.action.positionSeconds) unavailable();
      await settled(input.seek(row.action.positionSeconds, current));
      if (lifetime.signal.aborted || !lifetime.isCurrent()) stale();
    },
  });
  const lyrics = Object.freeze({
    getSnapshot: () => input.lyrics.getSnapshot(),
    subscribe: (listener: () => void) => current() ? input.lyrics.subscribe(listener) : () => {},
    refresh: async () => {
      require();
      if (!input.lyricsEnabled()) unavailable();
      await settled(input.lyrics.refresh());
    },
    choose: async (shown: LyricsSnapshot, resource: LyricRevision | null, lifetime: WebListeningLifetime) => {
      require();
      if (!input.lyricsEnabled() || lifetime.signal.aborted || !lifetime.isCurrent()) unavailable();
      if (shown !== input.lyrics.getSnapshot() || !shown.data) unavailable();
      if (resource !== null && !shown.data.resources.some(candidate => candidate.id === resource.id && candidate.revision === resource.revision)) unavailable();
      await settled(input.lyrics.choose(resource));
    },
    seek: async (shown: LyricsSnapshot, index: number, lifetime: WebListeningLifetime) => {
      require();
      if (!input.lyricsEnabled() || lifetime.signal.aborted || !lifetime.isCurrent()) unavailable();
      if (shown !== input.lyrics.getSnapshot() || !shown.data || !shown.data.source) unavailable();
      const selected = shown.data.selection.resource, line = selected?.document?.lines[index];
      if (!selected || !line || line.atMs === null) unavailable();
      const seconds = lyricSeekPosition(line, selected.offsetMs, shown.data.source);
      if (seconds === null) unavailable();
      await settled(input.seek(seconds, current));
      if (lifetime.signal.aborted || !lifetime.isCurrent()) stale();
    },
  });
  const queue = input.queue ? Object.freeze({
    list: input.queue,
    policy: async (shown: WebQueueListSnapshot, intent: V1QueuePolicyIntent, lifetime: WebListeningLifetime) => {
      require();
      if (!input.queue || lifetime.signal.aborted || !lifetime.isCurrent()) stale();
      if (shown !== input.queue.getSnapshot() || !shown.queueId || !shown.revision || shown.queueId !== input.target.queueId) unavailable();
      const target = Object.freeze({sessionId: input.target.sessionId, generation: input.target.sessionGeneration, playbackRun: input.target.playbackRun,
        itemId: input.target.itemId, versionId: input.target.versionId, queueId: shown.queueId, entryId: input.target.entryId}) as V1PlaybackTarget;
      await settled(input.policy(target, shown.revision, intent, current));
      if (lifetime.signal.aborted || !lifetime.isCurrent()) stale();
    },
    save: async (shown: WebQueueListSnapshot, name: string, lifetime: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void) => {
      require();
      if (!input.queue || lifetime.signal.aborted || !lifetime.isCurrent()) stale();
      if (shown !== input.queue.getSnapshot() || !shown.queueId || shown.queueId !== input.target.queueId) unavailable();
      const frozen = name.trim();
      if (!frozen || frozen.length > 256) unavailable();
      const signal = AbortSignal.any([record.signal, lifetime.signal]);
      // One invocation, original queue key: the delegate keeps its canonical polling for this save only.
      return input.save(shown.queueId, frozen, signal, current, (save: QueuePlaylistSave) => {
        if (!current() || signal.aborted) return;
        onProgress?.(save);
      });
    },
  }) : undefined;
  return Object.freeze({
    fence: input.fence, target: input.target, chapters, lyrics, ...(queue ? {queue} : {}),
    isCurrent: current,
    dispose: () => {
      if (disposed) return;
      disposed = true;
      record.abort();
      input.chapters.dispose();
      input.lyrics.dispose();
    },
  });
}

/** Active line/seek math, re-exported so one consumer imports the same functions. */
export {lyricSeekPosition} from '@core/lyrics.ts';
