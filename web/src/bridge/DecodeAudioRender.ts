import type {PlaybackService, PlaybackSnapshot} from '@core/index';
import {audioGainLinear, playableAudio, type AudioRenderV2, type PlayableAudioRender} from '@core/playback-v1/audio-render.ts';
import {RangeReader, audioByteCache} from './audio/bytes';
import {Deck, linkDecks, type PcmSource} from './audio/engine';
import {ConvertedSource} from './audio/converted';
import {TrackStream} from './audio/track';
import {Stretch} from './audio/stretch';

/**
 * Music decoded on the device (spec §18.1 version 2; Plan §9.6), Plexamp's model: the original file
 * is fetched with Range, decoded here (WebCodecs, or the JavaScript FLAC decoder), trimmed to its
 * exact frames and scheduled on the AudioContext's sample clock. The next track, prepared by the
 * queue (`:prepare-next`), joins gaplessly or crossfades; commit (`:commit-next`) happens shortly
 * before the edge, and the boundary moves the session.
 *
 * Owned by the web adapter: it has no element, queue, lease
 * or progress transport of its own; it reports to the playback service.
 */
const AHEAD_SECONDS = 30; // decoded ahead of the playhead, per deck (§9.6: at most about 60 s)
const PREPARE_SECONDS = 10; // a candidate decodes its first 10 s before it is "prepared"
const START_SECONDS = 1.5; // buffered before the first track starts
const LEAD = 0.08; // seconds between scheduling and the first frame
const COMMIT_BEFORE = 2; // commit at most crossfade + 2 s before the edge


/** Where a deck's audio comes from: the original file by Range (`direct`), or the server's FLAC or
 * Opus stream (`converted`). `unlock()` at commit lifts the prepared presentation's limits. */
type Feed = {unlock(): void; close(): void; prefetchAll(): Promise<void>; open(fromFrame: number): Promise<PcmSource | undefined>};
type Playing = {deck: Deck; plan: PlayableAudioRender; key: string; fromFrame: number; reader: Feed; token?: string; speed: number};
type Candidate = {deck: Deck; plan: PlayableAudioRender; token: string; reader: Feed; ready: boolean; speed: number};
type Linked = Candidate & {edge: number; fade: number};

const sessionKey = (s: PlaybackSnapshot['session']) => (s ? `${s.id}:${s.generation}` : '');

export type SameAlbum = (currentItemId: string | undefined, nextItemId: string) => boolean;

export class DecodeAudioRender {
  private context: AudioContext;
  private master: GainNode;
  private primary?: Playing;
  private candidate?: Candidate;
  private incoming?: Linked;
  private retiring?: Playing;
  private snapshot: PlaybackSnapshot;
  private readonly service: PlaybackService;
  private readonly url: (path: string) => string;
  private readonly element: HTMLVideoElement;
  private readonly sameAlbum: SameAlbum;
  private readonly timer: ReturnType<typeof setInterval>;
  private disposed = false;
  private revision = 0;
  private seekKey = '';
  private lastRate = 1;
  private committing?: string;
  private askedForGesture = false;
  private factIntent = -1;
  private factPosition = -1;
  private starting = false;

  constructor(service: PlaybackService, url: (path: string) => string, element: HTMLVideoElement, sameAlbum: SameAlbum = () => false) {
    this.service = service; this.url = url; this.element = element; this.sameAlbum = sameAlbum;
    this.snapshot = service.getSnapshot();
    const rate = playableAudio(this.snapshot.session?.audio) ? this.snapshot.session!.audio!.sampleRate! : undefined;
    this.context = createContext(rate);
    this.master = this.context.createGain();
    this.master.connect(this.context.destination);
    this.volume();
    element.addEventListener('volumechange', this.volume);
    document.addEventListener('click', this.gesture);
    document.addEventListener('keydown', this.gesture);
    this.timer = setInterval(() => this.tick(), 50);
  }

  private volume = () => { this.master.gain.setTargetAtTime(this.element.muted ? 0 : this.element.volume, this.context.currentTime, 0.01); };
  private gesture = () => {
    queueMicrotask(() => {
      if (!this.disposed && this.service.getSnapshot().intent === 'playing') {
        this.askedForGesture = false;
        void this.context.resume().catch(() => this.service.audioEffectsNotice('Press play to allow audio in this browser.'));
      }
    });
  };

  private reader(plan: PlayableAudioRender, prepared: boolean): Feed {
    if (plan.mode === 'converted') {
      let source: ConvertedSource | undefined, unlocked = !prepared;
      return {
        unlock: () => { unlocked = true; source?.unlock(); },
        close: () => source?.close(),
        prefetchAll: async () => {},
        open: async fromFrame => { source = new ConvertedSource({...plan, url: this.url(plan.url)}, fromFrame, {locked: !unlocked}); return source; },
      };
    }
    const range = new RangeReader({url: this.url(plan.url), key: plan.id, size: plan.bytes, cache: audioByteCache, ...(prepared && plan.prefetchBytes ? {limit: plan.prefetchBytes} : {})});
    return {unlock: () => range.unlock(), close: () => range.close(), prefetchAll: () => range.prefetchAll(), open: fromFrame => TrackStream.open(plan, range, fromFrame)};
  }

  private gainFor(plan: AudioRenderV2) { return audioGainLinear(plan, this.snapshot.audioEffects.settings.normalization); }

  /** A deck for `plan` from `fromFrame`; at a speed other than 1 the audio is time-stretched so
   * it keeps its pitch (§9.6 rule 3). */
  private async openDeck(plan: PlayableAudioRender, reader: Feed, fromFrame: number, label: string, speed: number): Promise<Deck | undefined> {
    const stream = await reader.open(fromFrame);
    if (!stream) return undefined;
    const frames = plan.durationFrames - fromFrame;
    const source = speed === 1 ? stream : new Stretch(stream, speed, frames);
    return new Deck(this.context, this.master, source, {frames: speed === 1 ? frames : (source as Stretch).outputFrames, gain: this.gainFor(plan), label});
  }

  /** A candidate cancelled before its deck opened has no deck yet; its reader still closes. */
  private closePlaying(p?: Playing | Candidate) { if (!p) return; (p.deck as Deck | undefined)?.close(); p.reader.close(); }
  private reset() {
    ++this.revision;
    for (const p of [this.primary, this.candidate, this.incoming, this.retiring]) this.closePlaying(p);
    this.primary = this.candidate = this.incoming = this.retiring = undefined;
    this.committing = undefined;
    this.starting = false;
  }

  apply(s: PlaybackSnapshot) {
    if (this.disposed) return;
    this.snapshot = s;
    if (!s.session || !s.audioEffects.rendering || s.phase === 'error' || s.phase === 'ended' || s.failedSeek) { this.reset(); return; }
    const plan = s.session.audio;
    if (!playableAudio(plan)) { this.service.audioRenderFailed(plan?.reason ?? 'This audio can’t be decoded on this device.'); return; }
    const seek = s.pendingSeek, seekKey = seek ? `${s.intentId}:${seek.revision}` : '';
    const rateChanged = this.lastRate !== s.audioEffects.rate;
    const key = sessionKey(s.session);
    // A boundary promotes the committed deck (same key); anything else that moves the track starts over.
    if (this.primary?.key !== key || (seek && seekKey !== this.seekKey) || rateChanged) {
      const position = seek?.positionSeconds ?? (this.primary?.key === key ? this.position() : s.positionSeconds);
      this.reset();
      this.seekKey = seekKey; this.lastRate = s.audioEffects.rate;
      void this.start(plan, key, Math.max(0, Math.min(plan.durationFrames - 1, Math.round(position * plan.sampleRate))), s.intentId, seek?.revision);
      return;
    }
    for (const p of [this.primary, this.incoming, this.retiring]) if (p) p.deck.setGain(this.gainFor(p.plan));
    this.transport(s);
    this.prepared(s);
    this.committed(s);
  }

  private transport(s: PlaybackSnapshot) {
    if (s.intent === 'paused' || s.seekError) { void this.context.suspend(); return; }
    if (this.context.state === 'suspended' && !this.starting && !this.askedForGesture) {
      // An autoplay denial needs a gesture: say so rather than failing.
      this.askedForGesture = true;
      void this.context.resume().then(() => {
        if (this.context.state !== 'running' && !this.disposed) { this.service.audioEffectsNotice('Press play to allow audio in this browser.'); this.service.pause(); }
      }).catch(() => { if (!this.disposed) { this.service.audioEffectsNotice('Press play to allow audio in this browser.'); this.service.pause(); } });
    }
  }

  private async start(plan: PlayableAudioRender, key: string, fromFrame: number, intent: number, seekRevision?: number) {
    const revision = this.revision;
    this.starting = true;
    const reader = this.reader(plan, false);
    try {
      const speed = this.snapshot.audioEffects.rate;
      const deck = await this.openDeck(plan, reader, fromFrame, 'primary', speed);
      if (revision !== this.revision || this.disposed) { deck?.close(); reader.close(); return; }
      if (!deck) { reader.close(); this.service.audioRenderFailed('This browser can’t decode this audio.'); return; }
      this.primary = {deck, plan, key, fromFrame, reader, speed};
      await deck.pump(START_SECONDS);
      if (revision !== this.revision || this.disposed) return;
      if (deck.failure) throw deck.failure;
      deck.start(Math.ceil((this.context.currentTime + LEAD) * this.context.sampleRate));
      this.starting = false;
      this.service.ready(intent);
      if (seekRevision !== undefined) this.service.seekApplied(intent, seekRevision, fromFrame / plan.sampleRate, 'paused');
      this.transport(this.service.getSnapshot());
      void deck.pump(AHEAD_SECONDS);
    } catch {
      if (revision === this.revision && !this.disposed) this.service.audioRenderFailed('Audio decoding or delivery failed. Retry playback to reconnect.');
    }
  }

  private prepared(s: PlaybackSnapshot) {
    const prepared = s.audioEffects.prepared;
    if (!prepared && !s.audioEffects.committed && this.candidate) { this.closePlaying(this.candidate); this.candidate = undefined; this.committing = undefined; return; }
    if (!prepared?.audio || !playableAudio(prepared.audio) || prepared.token === this.candidate?.token || prepared.token === this.incoming?.token) return;
    this.closePlaying(this.candidate);
    this.committing = undefined;
    const plan = prepared.audio, token = prepared.token, revision = this.revision;
    const reader = this.reader(plan, true);
    const speed = s.audioEffects.rate;
    const pending: Candidate = {deck: undefined as unknown as Deck, plan, token, reader, ready: false, speed};
    this.candidate = pending;
    void (async () => {
      try {
        const deck = await this.openDeck(plan, reader, 0, 'candidate', speed);
        if (!deck) throw new Error('No decoder.');
        if (this.candidate !== pending || revision !== this.revision) { deck.close(); return; }
        pending.deck = deck;
        await deck.pump(PREPARE_SECONDS);
        if (this.candidate !== pending) return;
        if (deck.failure) throw deck.failure;
        pending.ready = true;
        this.service.audioPrepared(token);
      } catch {
        if (this.candidate === pending) { this.closePlaying(pending); this.candidate = undefined; this.service.audioPreparationFailed('The next track could not be buffered. This edge will use ordinary playback.'); }
      }
    })();
  }

  private committed(s: PlaybackSnapshot) {
    const committed = s.audioEffects.committed, c = this.candidate, p = this.primary;
    if (!committed || !c || committed.token !== c.token || !c.ready || this.incoming || !p || p.deck.endFrame === undefined) return;
    if (committed.session.audio?.id !== c.plan.id) { this.service.audioRenderFailed('The prepared audio changed.'); return; }
    // The whole next file may now be fetched; bring it into the cache before its edge.
    c.reader.unlock();
    void c.reader.prefetchAll();
    const sameAlbum = this.sameAlbum(s.itemId, committed.itemId);
    const nowFrame = Math.ceil((this.context.currentTime + LEAD) * this.context.sampleRate);
    const edge = linkDecks(this.context, p.deck, c.deck, {crossfadeSeconds: s.audioEffects.settings.crossfadeSeconds, sameAlbum, earliest: nowFrame});
    if (edge.late) this.service.audioEffectsNotice('The next track arrived after its edge. Playback is continuing.');
    this.incoming = {...c, edge: edge.start, fade: edge.fade};
    this.candidate = undefined;
  }

  /** Seconds into the current track, from the sample clock. */
  private position(): number {
    const p = this.primary;
    if (!p) return this.snapshot.positionSeconds;
    // Output frames at the context rate → track frames at the file's rate, times the speed.
    const played = p.deck.playedFrames() * p.plan.sampleRate / this.context.sampleRate * p.speed;
    return (p.fromFrame + played) / p.plan.sampleRate;
  }

  private tick() {
    if (this.disposed) return;
    const s = this.service.getSnapshot(), p = this.primary;
    if (!p || s.phase === 'error' || s.phase === 'ended') return;
    const frame = Math.floor(this.context.currentTime * this.context.sampleRate);
    // The boundary: the committed track is now the one playing.
    const next = this.incoming;
    if (next && this.context.state === 'running' && frame >= next.edge) {
      this.retiring = p;
      this.primary = {deck: next.deck, plan: next.plan, key: sessionKey(s.audioEffects.committed?.session), fromFrame: 0, reader: next.reader, token: next.token, speed: next.speed};
      this.incoming = undefined; this.committing = undefined; this.seekKey = '';
      this.service.audioBoundary(next.token, this.position());
      return;
    }
    if (this.retiring && (this.retiring.deck.ended || frame >= (this.retiring.deck.endFrame ?? 0) + this.context.sampleRate)) { this.closePlaying(this.retiring); this.retiring = undefined; }
    if (p.deck.failure) { this.service.audioRenderFailed('Audio decoding or delivery failed. Retry playback to reconnect.'); this.reset(); return; }
    // Decode ahead (bounded), for the current deck and a linked successor.
    void p.deck.pump(AHEAD_SECONDS);
    if (this.incoming && this.incoming.deck.startFrame !== undefined && frame >= this.incoming.edge - AHEAD_SECONDS * this.context.sampleRate) void this.incoming.deck.pump(AHEAD_SECONDS);
    const end = p.deck.endFrame ?? Infinity;
    const playing = s.intent === 'playing' && this.context.state === 'running' && p.deck.startFrame !== undefined && frame >= p.deck.startFrame;
    const starving = playing && !p.deck.complete && p.deck.aheadSeconds < 0.05;
    this.service.audioRenderStatus?.(playing && !starving ? s.audioEffects.rate : 0, starving);
    if (playing && !s.pendingSeek) {
      const position = this.position();
      if (frame >= end && !this.incoming && !this.committing && !s.audioEffects.prepared) this.service.fact(s.intentId, position, 'ended');
      else if (s.intentId !== this.factIntent || Math.abs(position - this.factPosition) >= 0.25) { this.factIntent = s.intentId; this.factPosition = position; this.service.fact(s.intentId, position, 'playing'); }
    }
    // Commit shortly before the edge, once the candidate is buffered (§9.6 rule 5).
    const c = this.candidate;
    if (s.intent === 'playing' && c?.ready && !this.committing && end !== Infinity) {
      const fade = Math.min(s.audioEffects.settings.crossfadeSeconds, 12);
      if ((end - frame) / this.context.sampleRate <= fade + COMMIT_BEFORE) { this.committing = c.token; this.service.audioCommit(c.token); }
    }
  }

  transportPosition() { return this.primary ? this.position() : this.snapshot.positionSeconds; }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    clearInterval(this.timer);
    this.reset();
    document.removeEventListener('click', this.gesture);
    document.removeEventListener('keydown', this.gesture);
    this.element.removeEventListener('volumechange', this.volume);
    this.master.disconnect();
    void this.context.close();
  }
}

/** An AudioContext at the music's own rate when the browser allows it: the browser's output path
 * then does the one resampling to the device (§9.6 rule 3), continuously across tracks. */
function createContext(rate?: number): AudioContext {
  if (rate) { try { return new AudioContext({sampleRate: rate, latencyHint: 'playback'}); } catch { /* unsupported rate */ } }
  return new AudioContext({latencyHint: 'playback'});
}
