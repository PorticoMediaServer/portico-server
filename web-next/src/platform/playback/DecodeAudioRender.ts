import type {PlaybackService, PlaybackSnapshot} from '@core/index';
import {audioGainLinear, parseSeekIndex, playableAudio, type AudioRenderV2, type PlayableAudioRender, type SeekIndex} from '@core/playback-v1/audio-render.ts';
import {RangeReader, audioByteCache, type ByteCache} from './audio/bytes';
import {Deck, linkDecks, type PcmSource} from './audio/engine';
import {ConvertedSource} from './audio/converted';
import {TrackStream} from './audio/track';
import {Stretch} from './audio/stretch';
import {BrowserEqualizer, parseEqualizer, type EqualizerSettings} from './audio/equalizer';
import {loadDeviceEqualizer} from './audio/equalizer-store';
import {configureDestination, outputClockLag, type BrowserAudioFacts} from './audio/output';
import {MediaCarrier} from './audio/carrier';
import {listeningDiagnostic, type ListeningEffectsPort} from './listening-effects';

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
  private carrier: MediaCarrier;
  private readonly equalizer: BrowserEqualizer;
  private equalizerRevision = 0;
  private equalizerFailed = false;
  private outputCount = 2;
  private clockBasis: BrowserAudioFacts['clock'] = 'latency';
  /** Seconds between the context's clock and what is heard (the output path's latency), smoothed. */
  private lag = 0;
  private primary?: Playing;
  private candidate?: Candidate;
  private incoming?: Linked;
  private retiring?: Playing;
  private snapshot: PlaybackSnapshot;
  private readonly service: PlaybackService;
  private readonly url: (path: string) => string;
  private readonly element: HTMLVideoElement;
  private readonly sameAlbum: SameAlbum;
  private readonly byteCache: ByteCache;
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
  private processingReport = '';
  private resumeAttempt = 0;
  private readonly listening: ListeningEffectsPort;

  constructor(service: PlaybackService, url: (path: string) => string, element: HTMLVideoElement, sameAlbum: SameAlbum = () => false, byteCache: ByteCache = audioByteCache) {
    this.service = service; this.url = url; this.element = element; this.sameAlbum = sameAlbum; this.byteCache = byteCache;
    this.listening = service;
    this.snapshot = service.getSnapshot();
    // On iPhone and iPad, audio from a bare AudioContext follows the silent switch and stops when
    // the page goes to the background unless the page says it is playing media.
    try { const session = (navigator as Navigator & {audioSession?: {type: string}}).audioSession; if (session && session.type !== 'playback') session.type = 'playback'; } catch { /* not offered here */ }
    this.context = createContext();
    this.outputCount = configureDestination(this.context.destination);
    this.context.addEventListener('statechange', this.contextChanged);
    document.addEventListener('visibilitychange', this.interrupted);
    this.master = this.context.createGain();
    this.equalizer = new BrowserEqualizer(this.context);
    this.equalizer.output.connect(this.master);
    // Through a media element where the browser allows it, so media keys and the lock screen reach music too.
    this.carrier = new MediaCarrier(this.context, this.master, () => { if (!this.disposed && this.service.getSnapshot().intent === 'playing') this.service.pause(); });
    this.outputChanged();
    const equalizerRevision = this.equalizerRevision;
    void loadDeviceEqualizer().then(settings => {
      if (!this.disposed && equalizerRevision === this.equalizerRevision) this.applyEqualizer(settings);
    }).catch(() => { /* Storage/graph failure grants no processing facts. The caller can retry. */ });
    this.context.addEventListener('sinkchange', this.outputChanged);
    navigator.mediaDevices?.addEventListener?.('devicechange', this.outputChanged);
    this.volume();
    element.addEventListener('volumechange', this.volume);
    document.addEventListener('click', this.gesture);
    document.addEventListener('keydown', this.gesture);
    this.timer = setInterval(() => this.tick(), 50);
  }

  /** The system interrupted the context (a call, another app's audio) or suspended it in the
   * background, and does not start it again by itself. While the listener still means it to
   * play, it is started again once; a browser that refuses changes nothing and is not asked twice. */
  private interrupted = () => {
    if (this.disposed || this.starting || this.context.state === 'running' || this.context.state === 'closed') return;
    if (document.visibilityState !== 'visible' || this.service.getSnapshot().intent !== 'playing' || this.service.getSnapshot().seekError) return;
    this.resumeContext();
  };
  /** While the context is not running nothing is heard, so the position stops; once it runs
   * again the schedule is brought up to date. An interruption is also answered as above. */
  private contextChanged = () => {
    if (this.disposed) return;
    if (this.context.state === 'running') this.tick();
    else this.service.audioRenderStatus?.(0, false);
    this.interrupted();
  };
  private volume = () => { this.master.gain.setTargetAtTime(this.element.muted ? 0 : this.element.volume, this.context.currentTime, 0.01); };
  private gesture = () => {
    queueMicrotask(() => {
      if (!this.disposed && this.service.getSnapshot().intent === 'playing') {
        this.askedForGesture = false;
        this.resumeContext();
      }
    });
  };

  private resumeContext(): void {
    const attempt = ++this.resumeAttempt, revision = this.revision;
    const before = this.service.getSnapshot(), key = sessionKey(before.session);
    const refused = () => {
      const current = this.service.getSnapshot();
      if (this.disposed || attempt !== this.resumeAttempt || revision !== this.revision || current.intentId !== before.intentId || sessionKey(current.session) !== key || current.intent !== 'playing') return;
      this.service.pause();
      this.listening.listeningNotice('audio_autoplay_blocked');
    };
    void this.context.resume().then(() => { if (this.context.state !== 'running') refused(); }, refused);
    // AudioContext.resume may remain pending under autoplay policy. An actual media-element
    // NotAllowedError supplies refusal evidence; a running direct path needs no notice.
    this.carrier.play(error => {
      if (error && typeof error === 'object' && 'name' in error && error.name === 'NotAllowedError' && this.context.state !== 'running') refused();
    });
  }

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
    // A start or seek that is still reading the file is alive, however long the link makes it take.
    const alive = () => { if (!this.disposed && !prepared) this.service.engineProgress(this.service.getSnapshot().intentId); };
    const range = new RangeReader({url: this.url(plan.url), key: plan.id, size: plan.bytes, cache: this.byteCache, onBytes: alive, ...(prepared && plan.prefetchBytes ? {limit: plan.prefetchBytes} : {})});
    // A file with no index of its own (MP3, ADTS, Ogg) started part-way reads the server's seek
    // index (once) and the file from the nearest entry; nothing before it is read and nothing
    // is converted.
    const indexUrl = plan.seekIndex?.url;
    const abort = new AbortController();
    let indexRead: Promise<SeekIndex | undefined> | undefined;
    const index = indexUrl ? () => (indexRead ??= fetch(this.url(indexUrl), {signal: abort.signal, credentials: 'omit', cache: 'no-store', redirect: 'error'})
      .then(r => (r.ok ? r.json() : undefined)).then(parseSeekIndex, () => undefined)) : undefined;
    const open = (fromFrame: number): Promise<PcmSource | undefined> => TrackStream.open(plan, range, fromFrame, undefined, {index});
    return {unlock: () => range.unlock(), close: () => { abort.abort(); range.close(); }, prefetchAll: () => range.prefetchAll(), open};
  }

  private gainFor(plan: AudioRenderV2) { const settings = this.snapshot.audioEffects.settings; return audioGainLinear(plan, settings.normalization, 0, settings.neverBoost === true); }

  /** A deck for `plan` from `fromFrame`; at a speed other than 1 the audio is time-stretched so
   * it keeps its pitch (§9.6 rule 3). */
  private async openDeck(plan: PlayableAudioRender, reader: Feed, fromFrame: number, label: string, speed: number): Promise<Deck | undefined> {
    const stream = await reader.open(fromFrame);
    if (!stream) return undefined;
    const frames = plan.durationFrames - fromFrame;
    const source = speed === 1 ? stream : new Stretch(stream, speed, frames);
    return new Deck(this.context, this.equalizer.input, source, {frames: speed === 1 ? frames : (source as Stretch).outputFrames, gain: this.gainFor(plan), label, holdTail: this.snapshot.audioEffects.settings.gapless || this.snapshot.audioEffects.settings.crossfadeSeconds === 0});
  }

  /** A candidate cancelled before its deck opened has no deck yet; its reader still closes. */
  private closePlaying(p?: Playing | Candidate) { if (!p) return; (p.deck as Deck | undefined)?.close(); p.reader.close(); }
  private reset() {
    ++this.revision;
    this.askedForGesture = false;
    for (const p of [this.primary, this.candidate, this.incoming, this.retiring]) this.closePlaying(p);
    this.primary = this.candidate = this.incoming = this.retiring = undefined;
    this.committing = undefined;
    this.starting = false;
    this.reportProcessing();
  }

  apply(s: PlaybackSnapshot) {
    if (this.disposed) return;
    this.snapshot = s;
    if (!s.session || !s.audioEffects.rendering || s.phase === 'error' || s.phase === 'ended' || s.failedSeek) { this.reset(); return; }
    const plan = s.session.audio;
    if (!playableAudio(plan)) { this.service.audioRenderFailed('audio_render_failed'); return; }
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
    this.reportProcessing();
    this.transport(s);
    this.prepared(s);
    this.committed(s);
  }

  private transport(s: PlaybackSnapshot) {
    if (s.intent === 'paused' || s.seekError) { this.carrier.pause(); void this.context.suspend(); return; }
    if ((this.context.state as string === 'suspended' || this.context.state as string === 'interrupted') && !this.starting && !this.askedForGesture) {
      // An autoplay denial needs a gesture: say so rather than failing.
      this.askedForGesture = true;
      this.resumeContext();
    } else this.carrier.play();
  }

  private async start(plan: PlayableAudioRender, key: string, fromFrame: number, intent: number, seekRevision?: number) {
    const revision = this.revision;
    this.starting = true;
    const reader = this.reader(plan, false);
    try {
      const speed = this.snapshot.audioEffects.rate;
      const deck = await this.openDeck(plan, reader, fromFrame, 'primary', speed);
      if (revision !== this.revision || this.disposed) { deck?.close(); reader.close(); return; }
      if (!deck) { reader.close(); this.service.audioRenderFailed('audio_render_failed'); return; }
      this.primary = {deck, plan, key, fromFrame, reader, speed};
      await deck.pump(START_SECONDS);
      if (revision !== this.revision || this.disposed) return;
      if (deck.failure) throw deck.failure;
      deck.start(Math.ceil((this.context.currentTime + LEAD) * this.context.sampleRate));
      this.starting = false;
      this.reportProcessing();
      this.service.ready(intent);
      if (seekRevision !== undefined) this.service.seekApplied(intent, seekRevision, fromFrame / plan.sampleRate, 'paused');
      this.transport(this.service.getSnapshot());
      void deck.pump(AHEAD_SECONDS);
    } catch {
      if (revision === this.revision && !this.disposed) this.service.audioRenderFailed('audio_render_failed');
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
        const previous = this.primary;
        const joins = previous && previous.speed === 1 && speed === 1
          && (s.audioEffects.settings.crossfadeSeconds === 0 || !!prepared.itemId && this.sameAlbum(s.itemId, prepared.itemId));
        if (joins) {
          await previous.deck.pump(AHEAD_SECONDS);
          if (this.candidate !== pending || revision !== this.revision) { deck.close(); return; }
          deck.continueFrom(previous.deck);
        }
        await deck.pump(PREPARE_SECONDS);
        if (this.candidate !== pending) return;
        if (deck.failure) throw deck.failure;
        pending.ready = true;
        this.service.audioPrepared(token);
      } catch {
        if (this.candidate === pending) { this.closePlaying(pending); this.candidate = undefined; listeningDiagnostic('audio_transition_unavailable'); this.listening.audioPreparationFailed(); }
      }
    })();
  }

  private committed(s: PlaybackSnapshot) {
    const committed = s.audioEffects.committed, c = this.candidate, p = this.primary;
    if (!committed || !c || committed.token !== c.token || !c.ready || this.incoming || !p || p.deck.endFrame === undefined) return;
    if (committed.session.audio?.id !== c.plan.id) { this.service.audioRenderFailed('audio_render_failed'); return; }
    // The whole next file may now be fetched; bring it into the cache before its edge.
    c.reader.unlock();
    void c.reader.prefetchAll();
    const sameAlbum = this.sameAlbum(s.itemId, committed.itemId);
    const nowFrame = Math.ceil((this.context.currentTime + LEAD) * this.context.sampleRate);
    const edge = linkDecks(this.context, p.deck, c.deck, {crossfadeSeconds: s.audioEffects.settings.crossfadeSeconds, sameAlbum, earliest: nowFrame});
    if (edge.late) listeningDiagnostic('audio_edge_late');
    this.incoming = {...c, edge: edge.start, fade: edge.fade};
    this.candidate = undefined;
  }

  /** Seconds into the current track, as heard: the sample clock less the output path's latency. */
  private position(): number {
    const p = this.primary;
    if (!p) return this.snapshot.positionSeconds;
    // Output frames at the context rate → track frames at the file's rate, times the speed.
    const played = p.deck.playedFrames(this.heardTime()) * p.plan.sampleRate / this.context.sampleRate * p.speed;
    return (p.fromFrame + played) / p.plan.sampleRate;
  }

  /** The context time of the sample being heard now. */
  private heardTime(): number { return Math.max(0, this.context.currentTime - this.lag); }

  /**
   * How far the speakers are behind the context's clock (A-17.26): the browser's own output
   * timestamp where it gives one, else its stated output and processing latencies. Measured while
   * the context runs (a suspended clock stands still while the page's clock moves on), bounded to
   * half a second, and smoothed so the position never jumps.
   */
  private measureLag() {
    const sample = outputClockLag(this.context, performance.now(), this.carrier.carrying);
    if (!sample) return;
    this.clockBasis = sample.basis;
    this.lag = this.lag === 0 ? sample.seconds : this.lag + (sample.seconds - this.lag) * 0.1;
  }

  private tick() {
    if (this.disposed) return;
    const s = this.service.getSnapshot(), p = this.primary;
    this.reportProcessing();
    if (!p || s.phase === 'error' || s.phase === 'ended') return;
    this.measureLag();
    const frame = Math.floor(this.context.currentTime * this.context.sampleRate);
    // What is heard: the boundary and the end are when the listener hears them, not when they are scheduled.
    const heard = Math.floor(this.heardTime() * this.context.sampleRate);
    // The boundary: the committed track is now the one playing.
    const next = this.incoming;
    if (next && this.context.state === 'running' && heard >= next.edge) {
      this.retiring = p;
      this.primary = {deck: next.deck, plan: next.plan, key: sessionKey(s.audioEffects.committed?.session), fromFrame: 0, reader: next.reader, token: next.token, speed: next.speed};
      this.incoming = undefined; this.committing = undefined; this.seekKey = '';
      this.service.audioBoundary(next.token, this.position());
      return;
    }
    if (this.retiring && (this.retiring.deck.ended || frame >= (this.retiring.deck.endFrame ?? 0) + this.context.sampleRate)) { this.closePlaying(this.retiring); this.retiring = undefined; }
    if (p.deck.failure) { this.service.audioRenderFailed('audio_render_failed'); this.reset(); return; }
    // Decode ahead (bounded), for the current deck and a linked successor.
    void p.deck.pump(AHEAD_SECONDS);
    if (this.incoming && this.incoming.deck.startFrame !== undefined && frame >= this.incoming.edge - AHEAD_SECONDS * this.context.sampleRate) void this.incoming.deck.pump(AHEAD_SECONDS);
    const end = p.deck.endFrame ?? Infinity;
    if (!this.incoming && this.context.currentTime >= p.deck.tailDeadlineSeconds) p.deck.finishTail();
    const playing = s.intent === 'playing' && this.context.state === 'running' && p.deck.startFrame !== undefined && frame >= p.deck.startFrame;
    const starving = playing && !p.deck.complete && p.deck.aheadSeconds < 0.05;
    this.service.audioRenderStatus?.(playing && !starving ? s.audioEffects.rate : 0, starving);
    if (playing && !s.pendingSeek) {
      const position = this.position();
      if (heard >= end && !this.incoming && !this.committing && !s.audioEffects.prepared) this.service.fact(s.intentId, position, 'ended');
      else if (s.intentId !== this.factIntent || Math.abs(position - this.factPosition) >= 0.25) { this.factIntent = s.intentId; this.factPosition = position; this.service.fact(s.intentId, position, 'playing'); }
    }
    // Commit shortly before the edge, once the candidate is buffered (§9.6 rule 5).
    const c = this.candidate;
    if (s.intent === 'playing' && c?.ready && !this.committing && end !== Infinity) {
      const fade = Math.min(s.audioEffects.settings.crossfadeSeconds, 12);
      if ((end - frame) / this.context.sampleRate <= fade + COMMIT_BEFORE) { this.committing = c.token; this.service.audioCommit(c.token); }
    }
  }

  private outputChanged = () => {
    // A route change may alter channel limits, never the app's EQ choice or filter state.
    if (!this.disposed) this.outputCount = this.carrier.refreshOutput();
  };

  /** Apply committed device settings; only a successfully configured graph reports EQ on. */
  applyEqualizer(settings: EqualizerSettings): void {
    const value = parseEqualizer(settings);
    if (this.disposed) throw new Error('equalizer_unavailable');
    ++this.equalizerRevision;
    try { this.equalizer.apply(value); this.equalizerFailed = false; }
    catch (error) {
      this.equalizerFailed = true;
      // Capture the deck this renderer belongs to before any fallback can re-enter and
      // move the service to a successor; the write itself re-checks that binding.
      const deck = this.service.getSnapshot();
      try { this.equalizer.bypass(); }
      catch { this.service.fallbackAudioRender(); }
      // This song continues through the dry path. Saved intent remains on, actual EQ is false.
      listeningDiagnostic('equalizer_filter_unavailable', error);
      if (deck.session?.id) this.service.listeningFallback('eq', {sessionId: deck.session.id, generation: deck.session.presentation.generation, itemId: deck.itemId, intentId: deck.intentId});
    }
    this.reportProcessing();
  }
  getEqualizer(): EqualizerSettings { return this.equalizer.settings; }
  audioFacts(): BrowserAudioFacts | undefined {
    const plan = this.primary?.plan;
    const snapshot = this.service.getSnapshot();
    if (!plan || this.starting || this.primary?.deck.startFrame === undefined || this.primary.deck.failure || this.primary.key !== sessionKey(snapshot.session) || !snapshot.audioEffects.rendering || snapshot.phase === 'error' || snapshot.phase === 'ended') return undefined;
    const levelingGain = this.primary.deck.norm.gain.value;
    return {engine:'web-audio',bitPerfect:false,sourceRate:plan.sampleRate,contextRate:this.context.sampleRate,
      resampledToContext:plan.sampleRate!==this.context.sampleRate,deviceRate:null,browserMayResample:true,
      sourceChannels:plan.channels,outputChannels:Math.min(plan.channels,this.outputCount),eqApplied:!this.equalizerFailed && this.equalizer.settings.enabled,levelingApplied:Number.isFinite(levelingGain) && levelingGain !== 1,
      ...(Number.isFinite(levelingGain) && levelingGain > 0 ? {levelingGainDb:20*Math.log10(levelingGain)} : {}),clock:this.carrier.carrying?'unmeasured-carrier':this.clockBasis};
  }

  private reportProcessing(): void {
    const snapshot = this.service.getSnapshot(), session = snapshot.session;
    if (!session) { this.processingReport = ''; return; }
    const facts = this.audioFacts(), value = {eqApplied: facts?.eqApplied ?? false, levelingApplied: facts?.levelingApplied ?? false};
    const key = `${snapshot.intentId}:${session.id}:${session.generation}:${value.eqApplied}:${value.levelingApplied}`;
    if (key === this.processingReport) return;
    this.processingReport = key;
    this.listening.audioProcessingApplied({intentId: snapshot.intentId, sessionId: session.id, generation: session.generation}, value);
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
    this.context.removeEventListener('statechange', this.contextChanged);
    document.removeEventListener('visibilitychange', this.interrupted);
    this.context.removeEventListener('sinkchange', this.outputChanged);
    navigator.mediaDevices?.removeEventListener?.('devicechange', this.outputChanged);
    this.carrier.dispose();
    this.equalizer.dispose();
    void this.context.close();
  }
}

/** Let the browser choose its real context output rate; it may still resample in the OS path. */
function createContext(): AudioContext {
  return new AudioContext({latencyHint: 'playback'});
}
