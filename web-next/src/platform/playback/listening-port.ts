import {lyricSeekPosition} from '@core/lyrics.ts';
import type {LyricRevision, LyricsService, LyricsSnapshot} from '@core/lyrics.ts';
import type {PlayerChaptersService, PlayerChaptersSnapshot, PlayerChaptersTarget, PlayerChapter} from '@core/player-chapters.ts';
import type {QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1PlaybackTarget, V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackFence} from '../../../../packages/player-core/src/index.ts';
import type {WebQueueListPort, WebQueueListSnapshot} from './queue-list.ts';

/** The caller's own view lifetime. It stays part of every command's currentness and
 * signal from admission through the awaited IO, progress and final return. */
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
  /** The existing queueList port, bound to this record: no command survives disposal.
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

export type WebListeningApi = Readonly<{
  request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>;
  requestLyrics<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>;
}>;

export type WebListeningInput = Readonly<{
  fence: PlaybackFence; lifetime: WebListeningLifetime; target: WebListeningTarget;
  /** The exact original record: runtime/stack/source plus the captured target. It may
   * synchronously dispose or replace the source, so it is re-proved after every call. */
  current(): boolean;
  /** Original bound transport methods, captured once; never looked up again later. */
  request: WebListeningApi['request']; requestLyrics: WebListeningApi['requestLyrics'];
  chaptersTarget: PlayerChaptersTarget;
  makeChapters(api: WebListeningApi): PlayerChaptersService;
  makeLyrics(api: WebListeningApi): LyricsService;
  lyricsEnabled(): boolean;
  /** Runtime/stack/source retirement notifications, so an un-manual dispose still retires. */
  observe(retired: () => void): () => void;
  queue?: WebQueueListPort;
  /** Delegates keep the composed record + host + command currentness and signal. */
  policy(target: V1PlaybackTarget, revision: string, intent: V1QueuePolicyIntent, command: WebListeningLifetime): Promise<void>;
  save(queueId: string, name: string, command: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void): Promise<QueuePlaylistSave>;
  seek(seconds: number, command: WebListeningLifetime): Promise<void>;
}>;

const stale: () => never = () => {throw new Error('stale_listening_port');};
const unavailable: () => never = () => {throw new Error('listening_command_unavailable');};

const emptyQueue: WebQueueListSnapshot = Object.freeze({current: false, entries: Object.freeze([]), more: null, change: 'idle'});
const emptyLyrics: LyricsSnapshot = Object.freeze({data: null, candidates: Object.freeze([]), warnings: Object.freeze([]), preview: null, busy: false, error: '', generation: 0});
const emptyChapters = (scope: PlayerChaptersSnapshot['scope']): PlayerChaptersSnapshot => Object.freeze({
  generation: 0, scope, target: null, phase: 'idle', data: null, pagination: Object.freeze({cursor: null, canPrevious: false}), error: null});

/** One captured listening record. Everything it exposes delegates to the existing
 * PlayerChaptersService, LyricsService, queueList port and V1QueuePlayer methods; this
 * factory adds only target/lifetime guards, never a player, session, queue or engine. */
export function createWebListeningPort(input: WebListeningInput): WebListeningPort {
  const record = new AbortController();
  let dead = false;
  const attached = new Set<() => void>();
  /** Any observed retirement latches dead synchronously; a later true cannot revive it. */
  const retire = () => {
    if (dead) return;
    dead = true;
    for (const off of [...attached]) {attached.delete(off); try {off();} catch {}}
    try {chapters.dispose();} catch {} finally {try {lyrics.dispose();} catch {} finally {record.abort();}}
  };
  /** Mutable record state first, external predicates next, exact internal state after them. */
  const guard = (): boolean => {
    if (dead || record.signal.aborted) return false;
    if (!input.lifetime.signal.aborted && input.lifetime.isCurrent() && input.current()) {
      if (!dead && !record.signal.aborted) return true;
    }
    retire();
    return false;
  };
  const require = () => {if (!guard()) stale();};
  const command = (lifetime: WebListeningLifetime) => {
    const controller = new AbortController();
    const onAbort = () => controller.abort();
    lifetime.signal.addEventListener('abort', onAbort, {once: true});
    return {signal: AbortSignal.any([record.signal, lifetime.signal, controller.signal]),
      isCurrent: () => !controller.signal.aborted && !lifetime.signal.aborted && lifetime.isCurrent() && guard(),
      done: () => {lifetime.signal.removeEventListener('abort', onAbort);}};
  };
  const settled = async <T>(work: Promise<T>, current: () => boolean): Promise<T> => {
    const value = await work;
    if (!current()) stale();
    return value;
  };
  /** Bound original IO: admission before, combined signals, admission after the read. */
  const api: WebListeningApi = Object.freeze({
    request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => {
      if (!guard()) return Promise.reject(new Error('stale_listening_port'));
      return input.request<T>(path, method, body, AbortSignal.any([record.signal, ...(signal ? [signal] : [])]))
        .then(value => {if (!guard()) throw new Error('stale_listening_port'); return value;});
    },
    requestLyrics: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => {
      if (!guard()) return Promise.reject(new Error('stale_listening_port'));
      return input.requestLyrics<T>(path, method, body, AbortSignal.any([record.signal, ...(signal ? [signal] : [])]))
        .then(value => {if (!guard()) throw new Error('stale_listening_port'); return value;});
    },
  });
  const chapters = input.makeChapters(api);
  const lyrics = input.makeLyrics(api);
  /** One stable neutral chapters snapshot per record (useSyncExternalStore identity). */
  const neutralChapters = emptyChapters(chapters.getSnapshot().scope);
  attached.add(input.observe(() => {if (!input.current()) retire();}));
  const onLifetimeAbort = () => retire();
  input.lifetime.signal.addEventListener('abort', onLifetimeAbort, {once: true});
  attached.add(() => input.lifetime.signal.removeEventListener('abort', onLifetimeAbort));
  /** One notification only while this record and its source are still admitted. */
  const subscribe = (target: {subscribe(listener: () => void): () => void}) => (listener: () => void) => {
    if (!guard()) return () => {};
    const off = target.subscribe(() => {if (guard()) listener();});
    attached.add(off);
    return () => {attached.delete(off); off();};
  };
  const chaptersPort = Object.freeze({
    getSnapshot: () => guard() ? chapters.getSnapshot() : neutralChapters,
    subscribe: subscribe(chapters),
    load: async () => {require(); const c = command(input.lifetime); try {await settled(chapters.select(input.chaptersTarget), c.isCurrent);} finally {c.done();}},
    retry: async () => {require(); const c = command(input.lifetime); try {await settled(chapters.retry(), c.isCurrent);} finally {c.done();}},
    refresh: async () => {require(); const c = command(input.lifetime); try {await settled(chapters.refresh(), c.isCurrent);} finally {c.done();}},
    next: async () => {require(); const c = command(input.lifetime); try {await settled(chapters.next(), c.isCurrent);} finally {c.done();}},
    previous: async () => {require(); const c = command(input.lifetime); try {await settled(chapters.previous(), c.isCurrent);} finally {c.done();}},
    seek: async (shown: PlayerChaptersSnapshot, chapter: PlayerChapter, lifetime: WebListeningLifetime) => {
      require();
      if (lifetime.signal.aborted || !lifetime.isCurrent()) stale();
      const snapshot = chapters.getSnapshot();
      if (shown !== snapshot || snapshot.phase !== 'ready' || !snapshot.data) unavailable();
      const data = snapshot.data, target = input.target;
      if (data.scope.libraryId !== target.libraryId || data.scope.itemId !== target.itemId || data.scope.sessionId !== target.sessionId
        || data.scope.generation !== target.sessionGeneration || data.scope.sourceId !== target.sourceId) unavailable();
      const row = data.chapters.find(candidate => candidate.id === chapter.id);
      if (!row || row.index !== chapter.index || row.startSeconds !== chapter.startSeconds || row.action.id !== 'seek'
        || row.action.positionSeconds !== chapter.action.positionSeconds) unavailable();
      const c = command(lifetime);
      try {await settled(input.seek(row.action.positionSeconds, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);} finally {c.done();}
    },
  });
  const lyricsPort = Object.freeze({
    getSnapshot: () => guard() ? lyrics.getSnapshot() : emptyLyrics,
    subscribe: subscribe(lyrics),
    refresh: async () => {
      require();
      if (!input.lyricsEnabled()) unavailable();
      const c = command(input.lifetime);
      try {await settled(lyrics.refresh(), c.isCurrent);} finally {c.done();}
    },
    choose: async (shown: LyricsSnapshot, resource: LyricRevision | null, lifetime: WebListeningLifetime) => {
      require();
      if (!input.lyricsEnabled() || lifetime.signal.aborted || !lifetime.isCurrent()) unavailable();
      if (shown !== lyrics.getSnapshot() || !shown.data) unavailable();
      if (resource !== null && !shown.data.resources.some(candidate => candidate.id === resource.id && candidate.revision === resource.revision)) unavailable();
      const c = command(lifetime);
      // The service owns its own request controller; this command's signal stops this wait and
      // its publication, without cancelling a different drawer's command.
      let stop = () => {};
      try {
        const cancelled = new Promise<never>((_, reject) => {stop = () => reject(new Error('stale_listening_port'));});
        if (c.signal.aborted) stop(); else c.signal.addEventListener('abort', stop, {once: true});
        await settled(Promise.race([lyrics.choose(resource), cancelled]), c.isCurrent);
      } finally {c.signal.removeEventListener?.('abort', stop); c.done();}
    },
    seek: async (shown: LyricsSnapshot, index: number, lifetime: WebListeningLifetime) => {
      require();
      if (!input.lyricsEnabled() || lifetime.signal.aborted || !lifetime.isCurrent()) unavailable();
      if (shown !== lyrics.getSnapshot() || !shown.data || !shown.data.source) unavailable();
      const selected = shown.data.selection.resource, line = selected?.document?.lines[index];
      if (!selected || !line || line.atMs === null) unavailable();
      const seconds = lyricSeekPosition(line, selected.offsetMs, shown.data.source);
      if (seconds === null) unavailable();
      const c = command(lifetime);
      try {await settled(input.seek(seconds, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);} finally {c.done();}
    },
  });
  const list = (() => {
    const port = input.queue;
    if (!port) return undefined;
    const bound: WebQueueListPort = {
      getSnapshot: () => guard() ? port.getSnapshot() : emptyQueue,
      subscribe: subscribe(port),
      isCurrent: () => guard() && port.isCurrent(),
      play: async (shown, entryId, lifetime) => {
        const c = command(lifetime);
        try {
          if (!c.isCurrent() || shown !== port.getSnapshot()) stale();
          await settled(port.play(shown, entryId, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);
        } finally {c.done();}
      },
      move: async (shown, entryId, direction, lifetime) => {
        const c = command(lifetime);
        try {
          if (!c.isCurrent() || shown !== port.getSnapshot()) stale();
          await settled(port.move(shown, entryId, direction, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);
        } finally {c.done();}
      },
      remove: async (shown, entryId, lifetime) => {
        const c = command(lifetime);
        try {
          if (!c.isCurrent() || shown !== port.getSnapshot()) stale();
          await settled(port.remove(shown, entryId, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);
        } finally {c.done();}
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
      const c = command(lifetime);
      try {await settled(input.policy(target, shown.revision, intent, {signal: c.signal, isCurrent: c.isCurrent}), c.isCurrent);} finally {c.done();}
    },
    save: async (shown: WebQueueListSnapshot, name: string, lifetime: WebListeningLifetime, onProgress?: (save: QueuePlaylistSave) => void) => {
      require();
      if (lifetime.signal.aborted || !lifetime.isCurrent() || shown !== list.getSnapshot()) stale();
      if (!shown.queueId || shown.queueId !== input.target.queueId) unavailable();
      const frozen = name.trim();
      if (!frozen || frozen.length > 256) unavailable();
      const c = command(lifetime);
      try {
        const receipt = await input.save(shown.queueId, frozen, {signal: c.signal, isCurrent: c.isCurrent}, save => {
          if (!c.isCurrent()) return;
          onProgress?.(save);
        });
        if (!c.isCurrent()) stale();
        return receipt;
      } finally {c.done();}
    },
  }) : undefined;
  return Object.freeze({
    fence: input.fence, target: input.target, chapters: chaptersPort, lyrics: lyricsPort, ...(queue ? {queue} : {}),
    isCurrent: guard,
    dispose: retire,
  });
}

/** One line-seek decision, re-exported so a consumer imports the same function. */
export {lyricSeekPosition} from '@core/lyrics.ts';
