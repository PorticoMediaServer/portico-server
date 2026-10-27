import {lyricSeekPosition} from '@core/lyrics.ts';
import type {LyricRevision, LyricsService, LyricsSnapshot} from '@core/lyrics.ts';
import type {PlayerChaptersService, PlayerChaptersSnapshot, PlayerChaptersTarget, PlayerChapter} from '@core/player-chapters.ts';
import type {QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1PlaybackTarget, V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebQueueListPort, WebQueueListSnapshot} from './queue-list.ts';

/** The caller's own view lifetime, separate from the runtime record. It stays part of
 * every delegate's currentness and signal, not only an initial/final check. */
export type WebListeningLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;

/** The immutable target captured from one admitted playback before any async work. */
export type WebListeningTarget = Readonly<{
  libraryId: string; itemId: string; sessionId: string; sessionGeneration: number; sourceId: string;
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
  /** Resource selection only, as offered by the service; the shown snapshot must still be current. */
  choose(shown: LyricsSnapshot, resource: LyricRevision | null, lifetime: WebListeningLifetime): Promise<void>;
  /** Line seek through the existing guarded runtime seek, never raw media time. */
  seek(shown: LyricsSnapshot, index: number, lifetime: WebListeningLifetime): Promise<void>;
}>;

export type WebListeningQueuePort = Readonly<{
  /** The existing queueList port, bound to this record: no command survives dispose.
   * No continuation/More capability is exposed; Media's captureEditWindow is not a public
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
  /** The exact original record: runtime/stack/source plus the captured target. It may
   * synchronously dispose or replace the source, so it is re-proved after every call. */
  current(): boolean;
  chapters: PlayerChaptersService; chaptersTarget: PlayerChaptersTarget;
  lyrics: LyricsService; lyricsEnabled(): boolean;
  queue?: WebQueueListPort;
  /** Delegates keep the caller lifetime's currentness and signal for the whole command. */
  policy(target: V1PlaybackTarget, revision: string, intent: V1QueuePolicyIntent, lifetime: WebListeningLifetime): Promise<void>;
  save(queueId: string, name: string, lifetime: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void): Promise<QueuePlaylistSave>;
  seek(seconds: number, lifetime: WebListeningLifetime): Promise<void>;
}>;

const stale: () => never = () => {throw new Error('stale_listening_port');};
const unavailable: () => never = () => {throw new Error('listening_command_unavailable');};

const emptyQueue: WebQueueListSnapshot = Object.freeze({current: false, entries: Object.freeze([]), more: null, change: 'idle'});
/** Withheld facts after retirement: no source, target, data or error from the retired record. */
const emptyChapters = (scope: PlayerChaptersSnapshot['scope']): PlayerChaptersSnapshot => Object.freeze({
  generation: 0, scope, target: null, phase: 'idle', data: null, pagination: Object.freeze({cursor: null, canPrevious: false}), error: null});
const emptyLyrics: LyricsSnapshot = Object.freeze({data: null, candidates: Object.freeze([]), warnings: Object.freeze([]), preview: null, busy: false, error: '', generation: 0});

/** One captured listening record. Everything it exposes delegates to the existing
 * PlayerChaptersService, LyricsService, queueList port and V1QueuePlayer methods; this
 * factory adds only target/lifetime guards, never a player, session, queue or engine. */
export function createWebListeningPort(input: WebListeningInput): WebListeningPort {
  const record = new AbortController();
  let disposed = false;
  /** Mutable record state first; the runtime/target predicate last; then the same record
   * state again, because that external predicate may synchronously dispose or replace. */
  const live = () => !disposed && !record.signal.aborted;
  const guard = (): boolean => {
    if (!live()) return false;
    if (input.lifetime.signal.aborted || !input.lifetime.isCurrent()) return false;
    if (!input.current()) return false;
    return live() && !input.lifetime.signal.aborted && input.lifetime.isCurrent();
  };
  const require = () => {if (!guard()) stale();};
  const settled = async <T>(work: Promise<T>): Promise<T> => {
    const value = await work;
    require();
    return value;
  };
  /** Every upstream subscription this record opened, detached by dispose before any abort. */
  const attached = new Set<() => void>();
  /** A notification is published only while this record is still the admitted one. */
  const subscribe = (target: {subscribe(listener: () => void): () => void}) => (listener: () => void) => {
    if (!guard()) return () => {};
    const off = target.subscribe(() => {if (guard()) listener();});
    attached.add(off);
    return () => {attached.delete(off); off();};
  };
  const chapters = Object.freeze({
    getSnapshot: () => live() ? input.chapters.getSnapshot() : emptyChapters(input.chapters.getSnapshot().scope),
    subscribe: subscribe(input.chapters),
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
        || data.scope.generation !== target.sessionGeneration || data.scope.sourceId !== target.sourceId) unavailable();
      const row = data.chapters.find(candidate => candidate.id === chapter.id);
      if (!row || row.index !== chapter.index || row.startSeconds !== chapter.startSeconds || row.action.id !== 'seek'
        || row.action.positionSeconds !== chapter.action.positionSeconds) unavailable();
      await settled(input.seek(row.action.positionSeconds, lifetime));
    },
  });
  const lyrics = Object.freeze({
    getSnapshot: () => live() ? input.lyrics.getSnapshot() : emptyLyrics,
    subscribe: subscribe(input.lyrics),
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
      await settled(input.seek(seconds, lifetime));
    },
  });
  /** The raw port's commands are revoked by this record, not by the host lifetime alone. */
  const list = (() => {
    const port = input.queue;
    if (!port) return undefined;
    const bound: WebQueueListPort = {
      getSnapshot: () => guard() ? port.getSnapshot() : emptyQueue,
      subscribe: listener => {if (!guard()) return () => {}; return port.subscribe(() => {if (guard()) listener();});},
      isCurrent: () => guard() && port.isCurrent(),
      play: async (shown, entryId, lifetime) => {
        if (!guard() || shown !== port.getSnapshot() || !lifetime.isCurrent() || lifetime.signal.aborted) stale();
        await port.play(shown, entryId, lifetime);
        require();
      },
      move: async (shown, entryId, direction, lifetime) => {
        if (!guard() || shown !== port.getSnapshot() || !lifetime.isCurrent() || lifetime.signal.aborted) stale();
        await port.move(shown, entryId, direction, lifetime);
        require();
      },
      remove: async (shown, entryId, lifetime) => {
        if (!guard() || shown !== port.getSnapshot() || !lifetime.isCurrent() || lifetime.signal.aborted) stale();
        await port.remove(shown, entryId, lifetime);
        require();
      },
    };
    return bound;
  })();
  const queue = list ? Object.freeze({
    list,
    policy: async (shown: WebQueueListSnapshot, intent: V1QueuePolicyIntent, lifetime: WebListeningLifetime) => {
      require();
      if (lifetime.signal.aborted || !lifetime.isCurrent() || shown !== list.getSnapshot()) stale();
      if (!shown.queueId || !shown.revision || shown.queueId !== input.target.queueId) unavailable();
      const target = Object.freeze({sessionId: input.target.sessionId, generation: input.target.sessionGeneration, playbackRun: input.target.playbackRun,
        itemId: input.target.itemId, versionId: input.target.versionId, queueId: shown.queueId, entryId: input.target.entryId}) as V1PlaybackTarget;
      await settled(input.policy(target, shown.revision, intent, lifetime));
    },
    save: async (shown: WebQueueListSnapshot, name: string, lifetime: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void) => {
      require();
      if (lifetime.signal.aborted || !lifetime.isCurrent() || shown !== list.getSnapshot()) stale();
      if (!shown.queueId || shown.queueId !== input.target.queueId) unavailable();
      const frozen = name.trim();
      if (!frozen || frozen.length > 256) unavailable();
      // One invocation on the original queue key; the delegate owns its canonical polling.
      const receipt = await input.save(shown.queueId, frozen, lifetime, save => {
        if (!guard() || lifetime.signal.aborted || !lifetime.isCurrent()) return;
        onProgress?.(save);
      });
      require();
      return receipt;
    },
  }) : undefined;
  return Object.freeze({
    fence: input.fence, target: input.target, chapters, lyrics, ...(queue ? {queue} : {}),
    isCurrent: guard,
    dispose: () => {
      if (disposed) return;
      disposed = true; // detach first: no notification or command can dispatch past this point
      for (const off of [...attached]) {attached.delete(off); off();}
      try {input.chapters.dispose();} finally {input.lyrics.dispose();}
      record.abort();
    },
  });
}

/** One line-seek decision, re-exported so a consumer imports the same function. */
export {lyricSeekPosition} from '@core/lyrics.ts';
