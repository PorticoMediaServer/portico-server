/**
 * The device queue on Playback Protocol v1 (Plan — Client Playback Migration, core step): the
 * same `QueueController` surface as the /v2 `QueuePlayer`, over v1 queues (§8) and sessions (§5).
 * Playback runs through the existing `PlaybackService` and engines (`legacy-bridge.ts`), so an
 * app switches per server and media kind without screen changes.
 *
 * The server queue is the truth: the player holds its header and a window of at most 200
 * entries around the current one (Up Next), never the whole queue. Play and Shuffle are one
 * request when possible (`startPlayback`); a very large selection whose first entry the server
 * chooses later (`startPending`) is awaited. Next, Previous, a chosen entry and completion are
 * `:advance`, which starts the next session in the same request (`replacesSessionId`).
 *
 * `queue.updated` events (the device's `/v1/events` stream) refresh the view and wake a waiting
 * start; a command on an entry still being snapshotted retries (503 queue_building).
 *
 * Rendered audio (spec §18; gapless, crossfade): near the end of a track the player prepares the
 * next entry's audio (`:prepare-next`), the engine buffers its first window, and at the audio
 * boundary the player commits it (`:commit-next`), which moves the queue and starts the next
 * session. Any failure falls back to the ordinary end-and-advance. Only when the server offers
 * `features.queueTransitions` (`transitions`); music uses v1 only behind the per-kind switch.
 *
 * Native listening (plan §9.3 C3; Apple): with `native`, an audio session is handed to the native
 * owner, which reports its timeline, holds its lease and moves the queue (`:advance`, and the §18
 * edges) while JS sleeps. This player then neither reports nor prepares for it; it sends the
 * viewer's queue moves to the owner and adopts the sessions the owner moves to (its snapshots'
 * `transition`), so Up Next follows.
 *
 * Not here yet: live channels (phase 6).
 */
import type {PlaybackService} from '../index.ts';
import {ListeningService, loadListeningSelection, type ListeningTarget} from '../listening.ts';
import type {QueueAdvanceMode, QueuePostPlay} from '../post-play.ts';
import type {PreparedChoice} from '../prepared-media.ts';
import type {CompletionPolicy, HeldCompletion, QueueController, QueuePlaybackView, QueuePlayerState, QueueWorkspaceService} from '../queue-controller.ts';
import type {QueueEntry as LegacyEntry, QueueIntent, QueueItemInput, QueueScope, QueueSnapshot, QueueState} from '../queues.ts';
import {PlaybackApiError, call, idempotencyKey, type V1Http} from './http.ts';
import {toPlaybackSession, type V1PlaybackApi} from './legacy-bridge.ts';
import {QUEUE_WINDOW_MAX, QueueClient, saveQueueAsPlaylist, type CreateResult, type QueueAnchor, type QueuePlaylistSave, type SegmentInput} from './queue.ts';
import {audioTransitionsEnabled} from '../audio-effects.ts';
import type {NativeListeningV1Control, NativeListeningV1Event} from '../playback/native-listening.ts';
import {playableAudio} from './audio-render.ts';
import {consecutiveOnAlbum, type QueueEntry, type QueueHeader, type Selector, type ServerEvent, type Session} from './types.ts';

/** Items per `items` selector segment, and segments per create (spec §8). */
const SEGMENT_ITEMS = 500, MAX_SEGMENTS = 16;
const WINDOW_BEFORE = 20, WINDOW_AFTER = 50;
const MUSIC_CONTAINERS = ['album', 'artist', 'disc'];

export type V1Viewer = Readonly<{serverId: string; authority: 'local' | 'hosted'; accountId: string; profileId: string}>;

export type V1QueuePlayerOptions = Readonly<{
  http: V1Http;
  api: V1PlaybackApi;
  playback: PlaybackService;
  viewer: V1Viewer;
  key?: () => string;
  /** Poll interval while a start waits for its snapshot (tests shorten it). */
  pendingPollMs?: number;
  /** The device's `/v1/events` subscription: `queue.updated` refreshes the view (another
   * device's edit, a building segment growing) and wakes a start waiting for its snapshot. */
  events?: Readonly<{on(typeOrPrefix: string, listener: (event: ServerEvent) => void): () => void}>;
  /** How long a command retries while its entry is still being snapshotted (queue_building). */
  buildingRetryMs?: number;
  /** The server offers queue transitions for rendered audio (spec §18,
   * `features.queueTransitions`): gapless and crossfade edges. */
  transitions?: boolean | (() => boolean);
  /** The viewer's `music.shuffleDefault` and `music.repeatDefault`, applied when a music queue
   * starts with Play (Shuffle is always a shuffle). Undefined: off and off. */
  musicDefaults?: () => Readonly<{shuffle: boolean; repeat: 'off' | 'one' | 'all'}> | undefined;
  /** The native listening owner (Apple): audio sessions are handed to it (plan §9.3 C3). */
  native?: NativeListeningV1Control;
  /** Which item kinds the native owner takes (`song`, `audiobook_file`…). Nothing by default: an
   * app turns a kind on only once the server offers it and the exit tests pass on a real server. */
  nativeFor?: (itemKind: string) => boolean;
}>;

const segments = (ids: readonly string[], shuffle: boolean): SegmentInput[] => {
  const out: SegmentInput[] = [];
  for (let i = 0; i < ids.length && out.length < MAX_SEGMENTS; i += SEGMENT_ITEMS) {
    out.push({source: {items: {ids: ids.slice(i, i + SEGMENT_ITEMS)}}, ...(i === 0 && shuffle ? {order: {mode: 'shuffle'}} : {})});
  }
  return out;
};

const message = (e: unknown, fallback: string): string => {
  if (e instanceof PlaybackApiError) {
    switch (e.code) {
      case 'queue_too_large': return 'This queue is too long. Remove some of it or start a new one.';
      case 'queue_building': return 'This part of the queue is still loading. Try again in a moment.';
      case 'queue_ended': return 'The queue has ended.';
      case 'not_found': return 'This item isn’t available.';
      case 'stream_limit_reached': return 'This account is already playing as many streams as it is allowed.';
      case 'transcode_not_allowed': return 'This title needs converting, and converting is turned off for this account.';
      case 'unsupported_media': return 'This title can’t be played on this device.';
    }
  }
  return fallback;
};

/** Entry kinds that are listening (music and books), in both vocabularies: the catalogue's
 * (song, audiobook_file) and the v1 queue window's (track, audiobook). */
const LISTENING_KINDS: ReadonlySet<string> = new Set(['song', 'track', 'audiobook_file', 'audiobook']);

export class V1QueuePlayer implements QueueController {
  readonly workspace: Readonly<{service: QueueWorkspaceService}>;
  readonly listening: ListeningService;
  private readonly client: QueueClient;
  private readonly api: V1PlaybackApi;
  private readonly playback: PlaybackService;
  private readonly http: V1Http;
  private readonly scope: QueueScope;
  private readonly key: () => string;
  private readonly pendingPollMs: number;
  private header?: QueueHeader;
  private window: readonly QueueEntry[] = [];
  private session?: Session;
  private state: QueuePlayerState = Object.freeze({phase: 'ready', view: null, error: null, completion: null});
  private queueState: QueueState = Object.freeze({queue: null, phase: 'idle', error: null});
  private readonly listeners = new Set<() => void>();
  private readonly queueListeners = new Set<() => void>();
  private completionPolicy: CompletionPolicy = () => 'advance';
  private endedId?: string;
  private generation = 0;
  private disposed = false;
  private detach?: () => void;
  private unsubscribe?: () => void;
  private readonly events?: V1QueuePlayerOptions['events'];
  private unlisten?: () => void;
  private unlistenSessions?: () => void;
  private wakePending?: () => void;
  private refreshTimer?: ReturnType<typeof setTimeout>;
  private readonly buildingRetryMs: number;
  // Rendered-audio edges (§18): one try per track; the prepared token until its boundary.
  /** Read when used: a queue made before the server's capabilities arrived follows them once they do. */
  private readonly transitionsNow: () => boolean;
  private get transitions(): boolean { return this.transitionsNow(); }
  private readonly musicDefaults?: V1QueuePlayerOptions['musicDefaults'];
  private audioAttempt?: string;
  private prepared?: Readonly<{token: string; queueId: string; itemId: string; expiresAtMs: number; durationSeconds: number}>;
  private audioReadyToken?: string;
  private audioCommitting = false;
  // Native listening (C3): the owner, and which handover its snapshots belong to.
  private readonly native?: NativeListeningV1Control;
  private readonly nativeFor: (itemKind: string) => boolean;
  private nativeEpoch = 0;
  private nativeSession?: string;
  private sessionStartedAt = 0;

  constructor(options: V1QueuePlayerOptions) {
    const transitions = options.transitions;
    this.transitionsNow = typeof transitions === 'function' ? transitions : () => transitions === true;
    this.musicDefaults = options.musicDefaults;
    this.native = options.native;
    this.nativeFor = options.nativeFor ?? (() => false);
    this.http = options.http;
    this.api = options.api;
    this.playback = options.playback;
    this.key = options.key ?? (() => idempotencyKey());
    this.client = new QueueClient(options.http, this.key);
    this.pendingPollMs = options.pendingPollMs ?? 1000;
    this.events = options.events;
    this.buildingRetryMs = options.buildingRetryMs ?? 30_000;
    // The listening service speaks plain v1 REST (preferences, selections); it has no controller lane.
    this.scope = Object.freeze({...options.viewer, controllerId: 'v1', controllerEpoch: 'v1', commandLaneId: 'v1'});
    // PERF-24: a video session never asks for listening details.
    this.listening = new ListeningService(this.scope, (path, body, method, signal) => this.request(path, body, method, signal), options.playback, undefined, (itemId, sessionId) => this.session?.id !== sessionId || this.session.itemId !== itemId || this.session.kind === 'audio');
    const self = this;
    this.workspace = Object.freeze({service: {
      getSnapshot: () => self.queueState,
      subscribe: (fn: () => void) => { self.queueListeners.add(fn); return () => { self.queueListeners.delete(fn); }; },
      mutate: (intent: QueueIntent) => self.mutate(intent),
    } satisfies QueueWorkspaceService});
  }

  private request(path: string, body?: unknown, method: 'GET' | 'POST' | 'PUT' = 'GET', signal?: AbortSignal): Promise<unknown> {
    return call(this.http, {method, path, body, signal}).then(r => r.body);
  }

  getSnapshot = () => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };

  private publish(patch: Partial<QueuePlayerState>) {
    if (this.disposed) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const fn of this.listeners) fn();
  }
  private publishQueue(patch: Partial<QueueState>) {
    if (this.disposed) return;
    this.queueState = Object.freeze({...this.queueState, ...patch});
    for (const fn of this.queueListeners) fn();
  }

  connect() {
    this.detach = this.playback.installQueue(this);
    this.listening.connect();
    this.unsubscribe = this.playback.subscribe(() => this.observed());
    this.unlisten = this.events?.on('queue.updated', event => this.queueEvent(event));
    // An administrator's terminate reaches a native owner at once, not at its next report.
    this.unlistenSessions = this.events?.on('session.updated', event => {
      if (!this.nativeOwns() || event.resource?.id !== this.nativeSession) return;
      // The owner's own doing is not news to it: a track it reported finished ("ended"), or one a
      // gapless commit moved past ("completed"). Told about those, it would stop instead of advancing.
      const data = event.data as {state?: unknown; reason?: unknown} | undefined;
      if (data?.state === 'ended' && (data.reason === 'ended' || data.reason === 'completed')) return;
      void this.native!.sessionUpdated?.(this.nativeSession!).catch(() => {});
    });
    return () => this.dispose();
  }

  /** Another device's edit, a building segment growing, or the server choosing a waiting
   * start: refresh the view once (coalesced), and wake a start that waits. */
  private queueEvent(event: ServerEvent) {
    if (this.disposed || !this.header || event.resource?.id !== this.header.id) return;
    this.wakePending?.();
    if (event.revision !== undefined && event.revision === this.header.revision) return;
    if (this.refreshTimer) return;
    this.refreshTimer = setTimeout(() => { this.refreshTimer = undefined; void this.refresh().catch(() => {}); }, 100);
  }

  /** Runs a queue command, retrying while its entry is still being snapshotted (503
   * queue_building), for at most buildingRetryMs. */
  private async whileBuilding<T>(work: () => Promise<T>): Promise<T> {
    const until = Date.now() + this.buildingRetryMs;
    for (;;) {
      try {
        return await work();
      } catch (e) {
        if (!(e instanceof PlaybackApiError && e.code === 'queue_building') || Date.now() >= until || this.disposed) throw e;
        this.publish({phase: 'preparing'});
        await new Promise(r => setTimeout(r, Math.min(Math.max(250, e.retryAfterMs ?? 2000), 5000)));
      }
    }
  }

  // ── The view ────────────────────────────────────────────────────────

  /** Reads the header and the window around the current entry, and rebuilds the view. */
  async refresh(signal?: AbortSignal): Promise<void> {
    const id = this.header?.id;
    if (!id) return;
    const header = await this.client.header(id, signal);
    this.window = await this.client.window(id, WINDOW_BEFORE, WINDOW_AFTER, signal);
    this.accept(header);
  }

  private accept(header: QueueHeader, window?: readonly QueueEntry[]) {
    this.header = header;
    if (window) this.window = window;
    const current = header.current;
    const entries: LegacyEntry[] = this.window.map(e => e.available && e.itemId
      ? Object.freeze({id: e.entryId, removed: false, hidden: false as const, itemId: e.itemId, editionId: null, partId: null, sourceContext: Object.freeze({kind: 'item' as const, id: e.itemId, revision: null, entryId: null})})
      : Object.freeze({id: e.entryId, removed: false, hidden: true as const, ...(e.kind === 'pending' ? {pending: true as const} : {})}));
    const at = current ? this.window.findIndex(e => e.entryId === current.entryId) : -1;
    const following = at >= 0 ? this.window[at + 1] : undefined;
    const reason: 'ready' | 'end' | 'unavailable' = following ? (following.available ? 'ready' : 'unavailable') : header.repeat === 'all' && header.total > 0 ? 'ready' : 'end';
    const next = Object.freeze({entryId: following?.entryId ?? null, available: reason === 'ready', reason});
    const postPlay: QueuePostPlay = Object.freeze({nextEntryId: next.entryId, available: next.available, reason, countdownSeconds: 10, autoplay: true, passoutCheckDue: false, automaticAdvances: 0});
    const queue: QueueSnapshot = Object.freeze({
      id: header.id, scope: this.scope, revision: header.revision, highestSeenSequence: '0', replayWindow: Object.freeze({results: 256, bodies: 32}),
      repeat: header.repeat, shuffled: !!header.shuffle, currentEntryId: current?.entryId ?? null, currentPlaybackId: this.session?.id ?? null, entries: Object.freeze(entries),
    });
    const view: QueuePlaybackView = Object.freeze({
      queue, highestSequence: '0',
      current: this.session ? Object.freeze({id: this.session.id, generation: this.session.presentation.generation, state: this.session.state}) : null,
      next,
      items: Object.freeze(this.window.filter(e => e.itemId).map(e => Object.freeze({entryId: e.entryId, itemId: e.itemId, title: e.title, kind: e.kind, libraryId: ''}))),
      postPlay,
    });
    this.publish({view, error: null});
    this.publishQueue({queue, phase: 'ready', error: null});
  }

  // ── Playing ─────────────────────────────────────────────────────────

  playItem(itemId: string, startSeconds?: number, _prepared?: PreparedChoice, _quality?: string, _audioStream?: number): Promise<void> {
    return this.playIds([itemId], {startSeconds});
  }

  playEntries(entries: readonly QueueItemInput[], options: {shuffle?: boolean; startSeconds?: number} = {}): Promise<void> {
    if (!entries.length) return Promise.reject(new Error('No available items were selected.'));
    return this.playIds(entries.map(e => e.itemId), options);
  }

  async journey(target: ListeningTarget, action: 'play' | 'shuffle' | 'mix' | 'enqueue', resume = false, options: {signal?: AbortSignal; startItem?: string; startSeconds?: number} = {}) {
    this.listening.interact();
    const result = await loadListeningSelection((path, body, method, signal) => this.request(path, body, method, signal), this.scope, target, {mix: action === 'mix', resume, startItem: options.startItem, signal: options.signal});
    if (action === 'enqueue') await this.enqueueMany(result.entries);
    else await this.playEntries(result.entries, {shuffle: action === 'shuffle', startSeconds: options.startSeconds ?? (resume ? result.startSeconds : 0)});
    return result;
  }

  /** Play or Shuffle a container (a show, a season, a collection, an album…) as one queue
   * segment: the server snapshots it, however large, and the first entry plays at once. */
  playSelector(selector: Selector, options: {shuffle?: boolean; anchor?: QueueAnchor; startSeconds?: number} = {}): Promise<void> {
    // A music container played (not shuffled) starts with the viewer's music defaults.
    const defaults = !options.shuffle && 'container' in selector && MUSIC_CONTAINERS.includes(selector.container.kind) ? this.musicDefaults?.() : undefined;
    const shuffle = !!options.shuffle || !!defaults?.shuffle;
    return this.play([{source: selector, ...(shuffle ? {order: {mode: 'shuffle'}} : {}), ...(options.anchor ? {anchor: options.anchor} : {})}], '', {...options, repeat: defaults?.repeat});
  }

  /** Whether the next track follows the current one on its album, so the audio engine joins them
   * gaplessly instead of crossfading (Plexamp's behavior; spec §18.5, Plan §9.6). */
  sameAlbum(currentItemId: string, nextItemId: string): boolean {
    const at = this.window.findIndex(e => e.itemId === currentItemId && e.entryId === this.header?.current?.entryId);
    const current = at >= 0 ? this.window[at] : this.window.find(e => e.itemId === currentItemId);
    const next = at >= 0 && this.window[at + 1]?.itemId === nextItemId ? this.window[at + 1] : this.window.find(e => e.itemId === nextItemId);
    return consecutiveOnAlbum(current, next);
  }

  /** Shows an existing queue (this device's, after a relaunch) without playing it. */
  async open(queueId: string, signal?: AbortSignal): Promise<void> {
    const header = await this.client.header(queueId, signal);
    const window = await this.client.window(queueId, WINDOW_BEFORE, WINDOW_AFTER, signal);
    this.accept(header, window);
  }

  /** Play (or Shuffle): a new queue for this device, playing its first entry (or the shuffle's). */
  private playIds(ids: readonly string[], options: {shuffle?: boolean; startSeconds?: number}) {
    return this.play(segments(ids, !!options.shuffle), ids[0]!, options);
  }

  private async play(input: readonly SegmentInput[], first: string, options: {startSeconds?: number; repeat?: 'off' | 'one' | 'all'}) {
    const generation = ++this.generation;
    this.listening.interact();
    const intent = this.playback.beginQueueIntent(first || undefined);
    this.publish({phase: 'preparing', error: null, completion: null});
    try {
      if (!await this.playback.prepareQueueIntent(intent) || generation !== this.generation) return;
      const at = options.startSeconds;
      // One request when the server may choose the start (resume or the beginning); an explicit
      // position starts the session separately, at that position.
      const created: CreateResult = await this.client.create(input, at === undefined ? {startPlayback: {state: 'playing'}} : {});
      if (generation !== this.generation) return;
      this.accept(created.queue, created.window);
      let header = created.queue;
      if (options.repeat && options.repeat !== 'off') {
        header = (await this.client.update(header.id, header.revision, {repeat: options.repeat})).queue;
        if (generation !== this.generation) return;
        this.accept(header);
      }
      if (QueueClient.startPending(created)) {
        const waiting = this.client.awaitCurrent(header.id, {intervalMs: this.pendingPollMs});
        this.wakePending = waiting.wake;
        try { header = await waiting.done; } finally { this.wakePending = undefined; }
        if (generation !== this.generation) return;
        await this.refresh();
      }
      if (created.session && at === undefined) await this.adopt(created.session, intent);
      else if (header.current) await this.startEntry(header.id, header.current.entryId, intent, at);
      this.publish({phase: 'ready'});
    } catch (e) {
      if (generation !== this.generation) return;
      this.fail(first, e, 'Playback could not start. Try again.');
    }
  }

  /** Starts the session for one entry (a start that wasn't made with the create or advance). */
  private async startEntry(queueId: string, entryId: string, intent: number, startSeconds?: number) {
    const entry = this.window.find(e => e.entryId === entryId);
    const r = await this.whileBuilding(() => this.api.start({queue: {queueId, entryId}}, {state: 'playing', ...(startSeconds === undefined ? {startFrom: 'resume'} : {startPositionMs: Math.round(startSeconds * 1000)}), ...(this.session ? {replacesSessionId: this.session.id} : {})}, this.key(), entry?.durationMs !== undefined ? entry.durationMs / 1000 : undefined));
    // The presentation's timeline is the whole title: the engine starts at the chosen position.
    await this.adoptPlayback(r.session, startSeconds === undefined ? r.playback : {...r.playback, resumeSeconds: startSeconds}, intent);
  }

  /** Adopts a session a queue request started (create with startPlayback, or advance). */
  private async adopt(session: Session, intent: number) {
    const entry = this.window.find(e => e.entryId === session.queue?.entryId);
    const duration = entry?.durationMs !== undefined ? entry.durationMs / 1000 : await this.api.duration(session.itemId ?? '', session.versionId);
    await this.adoptPlayback(session, this.api.adopt(session, duration), intent);
  }

  private async adoptPlayback(session: Session, playback: ReturnType<V1PlaybackApi['adopt']>, intent: number) {
    const previous = this.session;
    this.session = session;
    this.sessionStartedAt = Date.now();
    // Replaced on the server: late reports from the engine for it are dropped, not sent (410).
    if (previous && previous.id !== session.id) this.api.retire(previous.id);
    this.endedId = undefined;
    this.nativeSession = undefined;
    if (this.header) this.accept(this.header);
    const itemKind = this.window.find(e => e.entryId === session.queue?.entryId)?.kind ?? '';
    if (this.native && session.kind === 'audio' && this.nativeFor(itemKind)) { await this.adoptNative(session, playback, intent); return; }
    if (!await this.playback.adoptQueueSession(session.itemId ?? '', playback, intent)) return;
  }

  // ── Native listening (plan §9.3 C3) ─────────────────────────────────

  /** Hands an audio session to the native owner, with the timeline `seq` reached so far. */
  private async adoptNative(session: Session, playback: ReturnType<V1PlaybackApi['adopt']>, intent: number) {
    const native = this.native!, epoch = ++this.nativeEpoch, header = this.header;
    const lastSeq = this.api.reportedSeq(session.id), durationSeconds = playback.duration;
    // INT T4: another playback began while this session's request was in flight. Nobody will own
    // it (the handover would be refused), so it ends now rather than lapsing with its lease.
    if (this.playback.getSnapshot().intentId !== intent) {
      await this.api.stopPlayback(session.id).catch(() => {});
      return;
    }
    // From here on the owner reports this session; JS never does.
    this.api.forget(session.id);
    this.nativeSession = session.id;
    try {
      await this.playback.adoptNativeQueueSession(session.itemId ?? '', intent, native, () => native.adopt({
        session, queueId: header?.id ?? null, queueRevision: header?.revision ?? null, durationSeconds, lastSeq,
        leaseMs: Math.max(0, 120_000 - (Date.now() - this.sessionStartedAt)), transitions: this.transitions,
        intent: session.state === 'paused' ? 'paused' : 'playing',
      }, event => this.nativeEvent(epoch, event)));
    } catch {
      // The native owner refused the handover: the session isn't left without an owner (its lease
      // would run on with nobody reporting). JS plays it, as without a native owner.
      if (this.disposed || epoch !== this.nativeEpoch) return;
      this.nativeSession = undefined;
      ++this.nativeEpoch;
      await this.playback.adoptQueueSession(session.itemId ?? '', this.api.adopt(session, durationSeconds), this.playback.beginQueueIntent(session.itemId));
    }
  }

  /** Whether the native owner has the current session (queue moves then go through it). */
  private nativeOwns(): boolean {
    return !!this.native && !!this.nativeSession && this.nativeSession === this.session?.id && this.playback.hasNativeOwner();
  }

  /** A native snapshot: the owner may have moved the queue on (a completion, Next from the lock
   * screen, a committed gapless edge) while JS slept; its session becomes this player's. */
  private nativeEvent(epoch: number, event: NativeListeningV1Event) {
    if (this.disposed || epoch !== this.nativeEpoch) return;
    const session = event.session, previous = this.session;
    if (previous?.id !== session.id) {
      if (previous) this.api.forget(previous.id);
      this.endedId = undefined;
      this.nativeSession = session.id;
    }
    this.session = session;
    if (event.transition?.queue && event.transition.queue.id === this.header?.id) this.header = event.transition.queue;
    const moved = previous?.id !== session.id || (!!event.queue && !!this.header && event.queue.id === this.header.id && event.queue.revision !== this.header.revision);
    if (event.queue && this.header && event.queue.id === this.header.id && event.queue.revision !== this.header.revision) this.header = Object.freeze({...this.header, revision: event.queue.revision});
    this.playback.applyNativeQueueEvent(event, toPlaybackSession(session, event.durationSeconds));
    if (moved && this.header) {
      this.accept(this.header);
      if (!this.refreshTimer) this.refreshTimer = setTimeout(() => { this.refreshTimer = undefined; void this.refresh().catch(() => {}); }, 100);
    }
  }


  private fail(itemId: string, e: unknown, fallback: string) {
    const text = message(e, fallback);
    this.publish({phase: 'ready', error: text});
    this.playback.queueFailed(itemId, text);
  }

  /** `:advance` with the current revision; the next session starts in the same request. */
  private async advance(reason: 'completion' | 'next' | 'previous' | 'entry', entryId?: string) {
    const header = this.header;
    if (!header) throw new Error('Load the queue first.');
    const generation = ++this.generation;
    const current = this.window.find(e => e.entryId === entryId);
    const intent = this.playback.beginQueueIntent(current?.itemId);
    try {
      const r = await this.whileBuilding(() => this.client.advance(header.id, header.revision, reason, entryId));
      if (generation !== this.generation) return;
      this.header = r.queue;
      this.window = await this.client.window(r.queue.id, WINDOW_BEFORE, WINDOW_AFTER);
      this.accept(r.queue);
      if (r.session) await this.adopt(r.session, intent);
      else if (r.queue.current) await this.startEntry(r.queue.id, r.queue.current.entryId, intent);
    } catch (e) {
      if (generation !== this.generation) return;
      if (e instanceof PlaybackApiError && e.status === 412) { await this.refresh().catch(() => {}); }
      this.fail(current?.itemId ?? '', e, 'The queue couldn’t move on. Try again.');
      throw e;
    }
  }

  async playEntry(queueId: string, entryId: string, expectedRevision: string): Promise<void> {
    this.listening.interact();
    if (queueId !== this.header?.id || expectedRevision !== this.header.revision) throw new Error('Refresh the queue before selecting an item.');
    if (this.nativeOwns()) { await this.native!.advance('entry', entryId); return; }
    await this.advance('entry', entryId);
  }

  async next(queueId: string, expectedRevision: string): Promise<void> {
    this.listening.interact();
    if (queueId !== this.header?.id || expectedRevision !== this.header.revision) throw new Error('Refresh the queue before advancing.');
    if (this.nativeOwns()) { await this.native!.advance('next'); return; }
    await this.advance('next');
  }

  /** Previous restarts the item after three seconds, else moves back. */
  async previous(): Promise<void> {
    this.listening.interact();
    if (this.playback.getSnapshot().positionSeconds > 3) { this.playback.seek(0); return; }
    if (!this.header) throw new Error('Load the queue first.');
    if (this.nativeOwns()) { await this.native!.advance('previous'); return; }
    await this.advance('previous');
  }

  // ── Editing ─────────────────────────────────────────────────────────

  async enqueue(item: QueueItemInput, next = false): Promise<void> {
    await this.enqueueMany([item], next);
  }

  async enqueueMany(entries: readonly QueueItemInput[], next = false): Promise<void> {
    if (!entries.length) throw new Error('No available items were selected.');
    const ids = entries.map(e => e.itemId);
    if (!this.header) {
      const created = await this.client.create(segments(ids, false));
      this.accept(created.queue, created.window);
      return;
    }
    for (let i = 0; i < ids.length; i += SEGMENT_ITEMS) {
      const r = await this.client.addSegment(this.header!.id, this.header!.revision, next ? 'next' : 'end', {source: {items: {ids: ids.slice(i, i + SEGMENT_ITEMS)}}});
      this.header = r.queue;
    }
    await this.refresh();
  }

  /** NEW-37: save the queue as a playlist; a long queue's copy is followed by replaying the key. */
  saveAsPlaylist(queueId: string, name: string, onProgress?: (save: QueuePlaylistSave) => void, signal?: AbortSignal): Promise<QueuePlaylistSave> {
    return saveQueueAsPlaylist(this.client, queueId, name, {onProgress, signal});
  }

  /** The workspace's edits (Up Next): remove, reorder (one moved entry), repeat, shuffle, add. */
  private async mutate(intent: QueueIntent): Promise<void> {
    const h = this.header;
    if (!h) throw new Error('Load the queue first.');
    this.publishQueue({phase: 'sending', error: null});
    try {
      switch (intent.action) {
        case 'remove': this.header = (await this.client.remove(h.id, h.revision, intent.entryId)).queue; break;
        case 'repeat': this.header = (await this.client.update(h.id, h.revision, {repeat: intent.repeat})).queue; break;
        case 'shuffle': this.header = (await this.client.update(h.id, h.revision, {shuffle: {on: intent.shuffled}})).queue; break;
        case 'add': case 'play-next': await this.enqueueMany([intent.entry], intent.action === 'play-next'); return;
        case 'append': case 'insert-next': await this.enqueueMany(intent.entries, intent.action === 'insert-next'); return;
        case 'reorder': {
          const before = this.window.map(e => e.entryId).filter(id => intent.entryIds.includes(id));
          const moved = movedEntry(before, intent.entryIds);
          if (!moved) break;
          const to = moved.after ? {after: moved.after} : {before: moved.before!};
          this.header = (await this.client.move(h.id, h.revision, moved.entryId, to)).queue;
          break;
        }
        default: throw new Error('This change isn’t available for this queue.');
      }
      await this.refresh();
    } catch (e) {
      if (e instanceof PlaybackApiError && e.status === 412) await this.refresh().catch(() => {});
      this.publishQueue({phase: 'ready', error: message(e, 'The queue couldn’t be changed. Try again.')});
      throw e;
    }
  }

  // ── Completion ──────────────────────────────────────────────────────

  private observed() {
    if (this.nativeOwns()) return; // the native owner reports the end and advances
    if (this.maybePrepareAudio()) return;
    const p = this.playback.getSnapshot();
    const session = this.session;
    if (this.disposed || !session || p.channel || p.phase !== 'ended' || p.session?.id !== session.id || this.endedId === session.id) return;
    this.endedId = session.id;
    const intent = p.intentId;
    void (async () => {
      try {
        await this.playback.confirmQueueEnd(intent);
        if (this.disposed || this.playback.getSnapshot().intentId !== intent) return;
        await this.refresh();
        const view = this.state.view;
        const item = view?.items.find(x => x.entryId === view.queue.currentEntryId);
        const kind = item?.kind ?? '';
        // NEW-44: an audio session is music or a book, whatever the window calls the entry (queue
        // windows name songs "track" and book files "audiobook", and the entry may be outside the
        // window). Listening rules (autoplay, sleep timer, "are you still listening") decide whether
        // it goes on, at once; the post-play hold (a countdown the video player shows) never applies.
        const listening = session.kind === 'audio' || LISTENING_KINDS.has(kind);
        if (listening && !this.listening.mayAdvance()) return;
        if (!listening && this.completionPolicy({kind, itemId: item?.itemId ?? p.itemId ?? ''}) === 'hold') {
          const held: HeldCompletion = Object.freeze({sessionId: session.id, intentId: intent, nextEntryId: view!.next.entryId, nextAvailable: view!.next.available, reason: view!.next.reason, postPlay: view!.postPlay});
          this.publish({completion: held});
          return;
        }
        if (view?.next.available) await this.advance('completion');
        else this.playback.queueEnded(intent);
      } catch { /* the engine stays ended; the viewer can choose again */ }
    })();
  }

  setCompletionPolicy(policy: CompletionPolicy) { this.completionPolicy = policy; }

  async continueCompletion(mode: QueueAdvanceMode = 'automatic'): Promise<void> {
    const held = this.state.completion;
    if (!held) return;
    this.publish({completion: null});
    const p = this.playback.getSnapshot();
    if (p.phase !== 'ended' || p.session?.id !== held.sessionId || !held.nextAvailable) return;
    if (mode !== 'automatic') this.listening.interact();
    await this.advance(mode === 'automatic' ? 'completion' : 'next');
  }

  releaseCompletion() { if (this.state.completion) this.publish({completion: null}); }

  // ── Recovery and lifetime ───────────────────────────────────────────

  /** Retry: start the current entry again, where the failed start or seek meant to be (a resume
   * point that never played is kept, not reset to 0:00). */
  async check(_retryUnknown = false): Promise<void> {
    const h = this.header;
    if (!h?.current) return;
    const at = this.playback.recoveryTarget();
    const intent = this.playback.beginQueueIntent(this.window.find(e => e.entryId === h.current!.entryId)?.itemId);
    try { await this.startEntry(h.id, h.current.entryId, intent, at); } catch (e) { this.fail('', e, 'Playback could not start. Try again.'); }
  }

  async cancel(_invalidateSelection = true): Promise<void> { ++this.generation; this.prepared = undefined; this.audioReadyToken = undefined; this.publish({phase: 'ready'}); }

  leave() { ++this.generation; this.prepared = undefined; this.audioReadyToken = undefined; this.publish({completion: null}); }

  dispose() {
    if (this.disposed) return;
    this.leave();
    this.disposed = true;
    ++this.nativeEpoch;
    this.unsubscribe?.();
    this.unlisten?.();
    this.unlistenSessions?.();
    if (this.refreshTimer) clearTimeout(this.refreshTimer);
    this.detach?.();
    this.listening.dispose();
    if (this.session) this.api.forget(this.session.id);
    this.state = Object.freeze({...this.state, phase: 'disposed', view: null});
    this.listeners.clear();
    this.queueListeners.clear();
  }

  // ── Rendered-audio edges (spec §18) ─────────────────────────────────

  /** The server's music preferences are the effects (X-02); nothing is kept per device here. */
  async saveAudioEffects(): Promise<void> {}

  /** Near the end of a playing, rendered track (the /v2 rule: at most min(40, 30×rate) seconds
   * left), prepare the next entry's audio once. */
  private maybePrepareAudio(): boolean {
    const p = this.playback.getSnapshot(), e = p.audioEffects, session = this.session, header = this.header;
    if (!this.transitions || this.disposed || !session || !header || session.kind !== 'audio' || p.session?.id !== session.id || !e.rendering || !audioTransitionsEnabled(e.settings) || p.channel || p.phase !== 'ready' || p.intent !== 'playing' || p.pendingSeek || p.failedSeek || this.audioAttempt === session.id || this.prepared) return false;
    const remaining = p.duration - p.positionSeconds;
    if (remaining <= 0 || remaining > Math.min(40, 30 * e.rate) || !this.listening.mayTransition(remaining / Math.max(.25, e.rate))) return false;
    this.audioAttempt = session.id;
    const generation = this.generation;
    void (async () => {
      try {
        const next = await this.client.prepareNext(header.id, header.revision, {sessionId: session.id, sessionGeneration: session.presentation.generation});
        if (this.disposed || generation !== this.generation || this.session?.id !== session.id) return;
        // Version 2 (§18.1): the engine decodes the next file itself.
        const audio = playableAudio(next.presentation.audio) ? next.presentation.audio : undefined;
        if (!audio) return;
        const expiresAtMs = Date.parse(next.expiresAt);
        const durationSeconds = audio.durationFrames / audio.sampleRate;
        this.prepared = Object.freeze({token: next.token, queueId: header.id, itemId: next.itemId, expiresAtMs, durationSeconds});
        this.playback.prepareAudio({token: next.token, expiresAtMs, audio});
      } catch { /* no edge for this track: it ends and advances as usual */ }
    })();
    return true;
  }

  /** The engine has the next entry's first window. */
  audioPrepared(token: string) { if (this.prepared?.token === token) this.audioReadyToken = token; }

  /** At the audio boundary: commit, which moves the queue and starts the next session (§18.3). */
  async commitAudio(token: string): Promise<void> {
    const prepared = this.prepared, p = this.playback.getSnapshot();
    if (this.audioCommitting || this.disposed || !prepared || prepared.token !== token || this.audioReadyToken !== token || p.audioEffects.prepared?.token !== token) return;
    if (!this.listening.mayTransition(Math.max(0, p.duration - p.positionSeconds) / Math.max(.25, p.audioEffects.rate))) { this.discardAudio('Listening policy changed; the prepared edge was canceled.'); return; }
    if (prepared.expiresAtMs <= Date.now()) { this.discardAudio('Audio preparation expired; this edge will use ordinary playback.'); return; }
    this.audioCommitting = true;
    const generation = this.generation, intent = p.intentId;
    try {
      let r: Awaited<ReturnType<QueueClient['commitNext']>>;
      try { r = await this.client.commitNext(prepared.queueId, token); }
      // A lost answer: the commit is idempotent by token, so asking again gets the same session.
      catch (e) { if (e instanceof PlaybackApiError) throw e; r = await this.client.commitNext(prepared.queueId, token); }
      if (this.disposed || generation !== this.generation) return;
      const old = this.session;
      this.header = r.queue;
      this.session = r.session;
      this.endedId = undefined;
      if (old && old.id !== r.session.id) this.api.retire(old.id); // ended on the server as completed
      const playback = this.api.adopt(r.session, prepared.durationSeconds);
      this.accept(r.queue);
      if (!await this.playback.commitAudioAuthority(token, prepared.itemId, playback, intent)) {
        // The engine moved on meanwhile: play the committed session ordinarily.
        this.prepared = undefined; this.audioReadyToken = undefined;
        await this.adoptPlayback(r.session, playback, this.playback.beginQueueIntent(prepared.itemId));
      }
    } catch (e) {
      if (generation === this.generation && !this.disposed) this.discardAudio(e instanceof PlaybackApiError && e.code === 'prepared_expired' ? 'Audio preparation expired; this edge will use ordinary playback.' : 'The next audio item could not be prepared; continuing with ordinary playback.');
    } finally {
      this.audioCommitting = false;
    }
  }

  /** The engine crossed the boundary: the committed session is now the one playing. */
  async audioBoundary(token: string, position: number): Promise<void> {
    if (!this.playback.adoptAudioBoundary(token, position)) return;
    this.prepared = undefined; this.audioReadyToken = undefined; this.audioAttempt = undefined;
    await this.refresh().catch(() => {});
  }

  /** A preparation this device drops: the track ends and advances as usual (the server's token
   * expires or is canceled by the next command). */
  discardAudio(message: string) {
    this.prepared = undefined; this.audioReadyToken = undefined;
    this.playback.clearPreparedAudio();
    if (this.playback.getSnapshot().phase !== 'error') this.playback.audioEffectsNotice(message);
  }

  invalidateAudioPreparation() {
    if (this.playback.getSnapshot().audioEffects.committed || !this.prepared) return;
    this.prepared = undefined; this.audioReadyToken = undefined;
    this.playback.clearPreparedAudio();
  }

  /** Seeking in a track whose successor is committed isn't offered on v1: the seek applies normally. */
  seekRetiringAudio(): boolean { return false; }

  audioRenderFailed(message: string) {
    const p = this.playback.getSnapshot();
    this.prepared = undefined; this.audioReadyToken = undefined;
    this.playback.fail(p.intentId, message);
  }
}

/**
 * The one entry a drag moved, from the order before and after (Up Next's reorder sends the whole
 * new order): where it now sits, as after its new predecessor, or before the first entry.
 */
export function movedEntry(before: readonly string[], after: readonly string[]): {entryId: string; after?: string; before?: string} | undefined {
  if (before.length !== after.length || before.length < 2) return undefined;
  for (const candidate of before) {
    const rest = before.filter(id => id !== candidate), restAfter = after.filter(id => id !== candidate);
    if (rest.every((id, i) => restAfter[i] === id) && before.indexOf(candidate) !== after.indexOf(candidate)) {
      const at = after.indexOf(candidate);
      return at > 0 ? {entryId: candidate, after: after[at - 1]} : {entryId: candidate, before: after[1]};
    }
  }
  return undefined;
}

/** Whether the server speaks Playback Protocol v1 (`GET /v1/capabilities` → `features.playback_v1`). */
/** Whether the server offers queue transitions for rendered audio (spec §18). */
export function serverSupportsQueueTransitions(capabilities: unknown): boolean {
  const features = capabilities && typeof capabilities === 'object' ? (capabilities as {features?: unknown}).features : undefined;
  return !!features && typeof features === 'object' && (features as Record<string, unknown>).queueTransitions === 'enabled';
}

export function serverSupportsPlaybackV1(capabilities: unknown): boolean {
  const features = capabilities && typeof capabilities === 'object' ? (capabilities as {features?: unknown}).features : undefined;
  return !!features && typeof features === 'object' && (features as Record<string, unknown>).playback_v1 === 'enabled';
}

/** Whether a media kind plays on Playback Protocol v1 when the server supports it: video,
 * audiobooks and music (B7, 23 Sep: music moved once both engines decode on the device; `music:
 * false` keeps it on /v2 for a client that must). Live channels switch on their own capability. */
export function playbackV1ForKind(serverSupportsV1: boolean, kind: string, options: Readonly<{music?: boolean}> = {}): boolean {
  if (!serverSupportsV1) return false;
  if (['movie', 'episode', 'video', 'audiobook', 'audiobook_file', 'book'].includes(kind)) return true;
  return options.music !== false && ['song', 'track', 'album', 'artist', 'disc'].includes(kind);
}

export {QUEUE_WINDOW_MAX};
