import {encodedRoutePath,routeBody} from "../../../contracts/src/routes.ts";
import {routePath} from "../../../contracts/src/routes.ts";
/**
 * The Portico Cast sender, one implementation for every platform. The platform finds a Cast
 * device and opens a session (web: the Chrome Cast SDK picker; Apple: `PorticoCast` over a
 * GCKGenericChannel) and hands this controller a `CastTransport` for it. The controller does the
 * rest: pairing through a server-issued code, loading the title with its display metadata,
 * transport commands, track choice, the takeover prompt and being replaced by another sender.
 */
import type {MessageId} from '../../../i18n/src/index.ts';
import {serviceProblem, serviceText} from '../presentation/service-text.ts';
import {CAST_PAIR_TIMEOUT_MS, CAST_PROTOCOL_VERSION, CAST_TAKEOVER_WINDOW_MS, captureCastChoices,castTransferReceipt,castChoiceFields, castMetadataFields, parseCastEvent, type CastQueueSource,type CastTransferReceipt,type CastChoices, type CastCommand, type CastEvent, type CastMetadata, type CastTrack} from './protocol.ts';

/** One open session with one receiver, over the `urn:x-cast:tv.getportico.cast` namespace. */
export interface CastTransport {
  /** The device's friendly name ("Living Room TV"). */
  readonly deviceName: string;
  /** The Cast SDK's own id for the device, when the platform gives one: two televisions with one name keep separate pairing proofs. */
  readonly deviceId?: string;
  send(message: CastCommand): Promise<void>;
  /** Receiver messages, as the SDK delivers them (JSON string or decoded object). Returns an unsubscribe. */
  onMessage(listener: (raw: unknown) => void): () => void;
  /** The session ended from the device or the SDK side. Returns an unsubscribe. */
  onSessionEnd(listener: () => void): () => void;
  /** Leave the session; `stopReceiver` also closes Portico on the TV. */
  end(stopReceiver: boolean): void;
}

export type CastApi = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

export type CastPhase = 'off' | 'connecting' | 'pairing' | 'waiting' | 'casting' | 'error' | 'replaced';

export type CastSnapshot = Readonly<{
  /** The platform can cast from here (set by the platform once its SDK and the server agree). */
  available: boolean;
  /** Internal accepting receipt: never a receiver-supplied source identity. */
  acceptedSource?:CastQueueSource;
  /** Retain the original source until cancellation or commit is authoritative. */
  transferPending?:boolean;
  phase: CastPhase;
  deviceName?: string;
  itemId?: string;
  title?: string;
  positionSeconds: number;
  durationSeconds: number;
  paused: boolean;
  audioTracks: readonly CastTrack[];
  textTracks: readonly CastTrack[];
  /** A catalogue message in the viewer's language, and its ID. */
  error?: string;
  errorId?: MessageId;
  /** Someone else asked the TV to play; this controller answers (30 s without an answer is no). */
  takeover?: Readonly<{requester: string}>;
}>;

export type CastControllerOptions = Readonly<{
  api: CastApi;
  /** The server origin the TV should redeem its code against. Must be HTTPS: a Cast device loads nothing else. */
  origin: string;
  /** How this sender introduces itself on the TV ("Chrome on Mac", "Sam’s iPhone"). No account details. */
  displayName: string;
  /** Native apps provide installation-owned secure storage; browsers use local storage. */
  controllerStorage?:Readonly<{get(key:string):string|undefined;set(key:string,value:string):void}>;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (timer: unknown) => void;
}>;

class CastFailure extends Error {
  readonly messageId: MessageId;
  constructor(id: MessageId) { super(id); this.messageId = id; }
}

const idle: CastSnapshot = Object.freeze({available: false, phase: 'off', positionSeconds: 0, durationSeconds: 0, paused: false, audioTracks: Object.freeze([]), textTracks: Object.freeze([])});

type CastTransferFlight={id?:string;transport:CastTransport;source:CastQueueSource;itemId:string;position:number;cancellation?:Promise<CastTransferReceipt|undefined>};
export class CastController {
  private transferFlight?:CastTransferFlight;
  private recoveryTransfer?:CastTransferFlight;
  private state: CastSnapshot = idle;
  private listeners = new Set<() => void>();
  private api: CastApi;
  private origin: string;
  private displayName: string;
  private transport?: CastTransport;
  private detach: (() => void)[] = [];
  private startGeneration = 0;
  private requestSequence = 0;
  private loads = new Map<string, {resolve(itemId: string): void; reject(error: Error): void}>();
  private waiting?: {resolve(): void; reject(e: Error): void};
  private adopting = false;
  private adoptTimer?: unknown;
  private settleAdoption?: (ok: boolean) => void;
  private takeoverTimer?: unknown;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (timer: unknown) => void;
  private disposed = false;
  private controllerStorage?:CastControllerOptions['controllerStorage'];
  private controllerKey(){const t=this.transport;return 'portico.cast.controller:'+this.origin+':'+(t?.deviceId?'id:'+t.deviceId:(t?.deviceName??''));}
  private controllerToken(){try{return this.controllerStorage?.get(this.controllerKey());}catch{return undefined;}}

  constructor(options: CastControllerOptions) {
    if (!/^https:\/\//.test(options.origin)) throw new Error('Cast needs an HTTPS server origin.');
    this.controllerStorage=options.controllerStorage;
    if(!this.controllerStorage){try{const storage=globalThis.localStorage;if(storage)this.controllerStorage={get:k=>storage.getItem(k)??undefined,set:(k,v)=>storage.setItem(k,v)};}catch{}}
    this.api = options.api;
    this.origin = new URL(options.origin).origin;
    this.displayName = options.displayName.slice(0, 64);
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
  }

  getSnapshot = (): CastSnapshot => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(patch: Partial<CastSnapshot>) {
    if (this.disposed) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const fn of [...this.listeners]) fn();
  }

  /** The platform's verdict on whether Cast can be offered here. */
  setAvailable(available: boolean) { this.publish({available}); }
  /** The platform's device picker is open ("Choose a device…"). */
  connecting(title?: string) { this.publish({phase: 'connecting', acceptedSource:undefined,title, error: undefined, errorId: undefined, takeover: undefined}); }
  /** The picker closed without a device: not an error. */
  cancelConnecting() { if (this.state.phase === 'connecting') this.publish({phase: 'off'}); }

  /** Pair with the TV on `transport` and start `itemId` there. True once the TV has the title. */
  async start(transport: CastTransport, itemId: string, meta: CastMetadata, startSeconds: number, choices?: CastChoices): Promise<boolean> {
    if(this.recoveryTransfer || this.transferFlight?.id){
      const previous=this.recoveryTransfer ?? this.transferFlight!;
      try{const receipt=await this.cancelTransfer(previous);this.recoveryTransfer=undefined;if(this.transferFlight===previous)this.transferFlight=undefined;
        if(receipt?.state==='committed'&&receipt.itemId===previous.itemId)this.publish({acceptedSource:previous.source});
        this.publish({phase:'off',acceptedSource:undefined,transferPending:false});
      }catch{this.fail('cast.error.unreachable');}
      return false;
    }
    this.release(false);
    const generation = this.startGeneration;
    choices=captureCastChoices(choices);meta=Object.freeze({...meta});
    const flight:CastTransferFlight|undefined=choices?.source?{transport,source:choices.source,itemId,position:Math.max(0,startSeconds)}:undefined;
    this.transferFlight=flight;
    this.transport = transport;
    this.detach.push(transport.onMessage(raw => { const e = parseCastEvent(raw); if (e) this.receive(e); }));
    this.detach.push(transport.onSessionEnd(() => this.sessionEnded()));
    let loading = false;
    this.publish({phase: 'pairing', acceptedSource:undefined,transferPending:!!flight,deviceName: transport.deviceName, title: meta.title, itemId: undefined, error: undefined, errorId: undefined, takeover: undefined, audioTracks: [], textTracks: []});
    try {
      const displayName = transport.deviceName.trim().slice(0, 64);
      const bootstrapBody={protocolVersion:CAST_PROTOCOL_VERSION,...(displayName?{displayName}:{}),...(choices?.source?{source:choices.source}:{})};
      const issued = await this.api.request<{bootstrap?: {id?:unknown;code?: unknown}}>(routePath("social_post_v1_cast_bootstrap"), 'POST', routeBody("social_post_v1_cast_bootstrap", bootstrapBody));
      if(flight){
        const id=issued?.bootstrap?.id;
        if(typeof id!=='string'||!/^[A-Za-z0-9._:-]{1,128}$/.test(id)||!castTransferReceipt(issued,id))throw new CastFailure('cast.error.noCode');
        flight.id=id;choices=Object.freeze({...choices,queue:Object.freeze({...choices!.queue!,transferId:id})});
      }
      if (generation !== this.startGeneration) {if(flight)await this.cancelTransfer(flight);return false;}
      const code = issued?.bootstrap?.code;
      if (typeof code !== 'string' || !code) throw new CastFailure('cast.error.noCode');
      await this.exchange({type: 'pair', code, origin: this.origin, displayName: this.displayName});
      if (generation !== this.startGeneration) return false;
      loading = true;
      const playingItemId = await this.load({type: 'load', itemId, startSeconds: Math.max(0, startSeconds), ...castMetadataFields(meta), ...castChoiceFields(choices)});
      if (generation !== this.startGeneration) return false;
      if(flight?.id){
        const receipt=await this.transferStatus(flight.id);
        if(generation!==this.startGeneration)return false;
        if(receipt?.state!=='committed'||receipt.itemId!==itemId||playingItemId!==itemId)throw new CastFailure('cast.error.loadFailed');
      }
      if(this.transferFlight===flight)this.transferFlight=undefined;
      this.publish({phase: 'casting', acceptedSource:flight?.source,transferPending:false,itemId: playingItemId, positionSeconds: Math.max(0, startSeconds), paused: false});
      this.send({type:'status'});
      return true;
    } catch (e) {
      if (generation !== this.startGeneration) return false;
      if(flight?.id){
        // Delivery/ACK loss is uncertainty, not a refusal. Recover the same
        // committed result without stopping it. Only a real receiver failure
        // or the user's Stop takes the cancellation/authorized Stop path.
        if(!(e instanceof CastFailure && e.messageId==='cast.error.loadFailed')){
          try{
            const receipt=await this.transferStatus(flight.id);
            if(generation!==this.startGeneration)return false;
            if(!receipt)throw new CastFailure('cast.error.noAnswer');
            if(receipt.state==='committed'){
              if(receipt.itemId!==itemId)throw new CastFailure('cast.error.loadFailed');
              this.transferFlight=undefined;this.recoveryTransfer=undefined;
              this.publish({phase:'casting',acceptedSource:flight.source,transferPending:false,itemId:receipt.itemId,paused:false});
              this.send({type:'status'});return true;
            }
          }catch{
            if(generation===this.startGeneration){this.transferFlight=undefined;this.recoveryTransfer=flight;this.release(false);this.fail('cast.error.unreachable');}
            return false;
          }
        }
        try{
          const receipt=await this.cancelTransfer(flight);
          if(generation===this.startGeneration){
            this.transferFlight=undefined;this.recoveryTransfer=undefined;
            this.publish({transferPending:false,...(receipt?.state==='committed'&&receipt.itemId===itemId?{acceptedSource:flight.source}:{})});
          }
        }catch{
          if(generation===this.startGeneration){this.transferFlight=undefined;this.recoveryTransfer=flight;this.fail('cast.error.unreachable');}
        }
      } else if (loading) this.send({type: 'stop'});
      if(generation!==this.startGeneration)return false;
      this.release(false);
      const id: MessageId = e instanceof CastFailure ? e.messageId : 'cast.error.unreachable';
      this.publish({phase: 'error', takeover: undefined, errorId: id, error: e instanceof CastFailure ? serviceText(id) : serviceProblem(e, 'playback', id)});
      return false;
    }
  }

  /**
   * CAST-05: a session the platform resumed on its own (a page reload while the TV plays) is
   * adopted when the TV answers a status request within `waitMs`: the controller shows it as
   * casting and can control and stop it. Otherwise it is left alone (never ended, never paired
   * again). `describe` names the title from its item id. Resolves true once adopted.
   */
  adopt(transport: CastTransport, describe?: (itemId: string) => Promise<string | undefined>, waitMs = 5000): Promise<boolean> {
    if (this.transport || this.disposed || this.state.transferPending || this.recoveryTransfer) return Promise.resolve(false);
    this.transport = transport;
    return new Promise<boolean>(resolve => {
      const give = (ok: boolean) => { if (!this.adopting) return; this.clearTimer(this.adoptTimer); this.adopting = false; this.settleAdoption = undefined; resolve(ok); };
      this.adopting = true;
      this.settleAdoption = give;
      this.detach.push(transport.onMessage(raw => {
        const e = parseCastEvent(raw);
        if (!e) return;
        if (!this.adopting) { this.receive(e); return; }
        if (e.type === 'status' && e.itemId) {
          this.publish({phase: 'casting', deviceName: transport.deviceName, itemId: e.itemId, title: undefined, error: undefined, errorId: undefined, takeover: undefined, positionSeconds: e.positionSeconds, durationSeconds: e.durationSeconds, paused: e.paused, audioTracks: e.audioTracks, textTracks: e.textTracks});
          give(true);
          void describe?.(e.itemId).then(title => { if (title && this.state.phase === 'casting' && this.state.itemId === e.itemId) this.publish({title}); }, () => {});
        } else if (e.type === 'pair-required' || e.type === 'replaced' || e.type === 'busy') { this.release(false, false); give(false); }
      }));
      this.detach.push(transport.onSessionEnd(() => { if (this.adopting) { this.release(false, false); give(false); } else this.sessionEnded(); }));
      this.adoptTimer = this.setTimer(() => { if (this.transport === transport && this.state.phase !== 'casting') this.release(false, false); give(false); }, waitMs);
      const controlToken=this.controllerToken();
      this.send({type:'status',...(controlToken?{controlToken}:{})});
    });
  }

  private exchange(body: CastCommand): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const timer = this.setTimer(() => { this.waiting = undefined; reject(new CastFailure('cast.error.noAnswer')); }, CAST_PAIR_TIMEOUT_MS);
      this.waiting = {resolve: () => { this.clearTimer(timer); resolve(); }, reject: e => { this.clearTimer(timer); reject(e); }};
      this.transport!.send(body).catch(() => { this.waiting?.reject(new CastFailure('cast.error.noAnswer')); this.waiting = undefined; });
    });
  }

  /** Correlate a bounded number of in-flight receiver loads; a transport send is not playback. */
  private transferPath(id:string){return encodedRoutePath('social_get_v1_cast_bootstrap_id',{id:encodeURIComponent(id)});}
  private async transferStatus(id:string):Promise<CastTransferReceipt|undefined>{
    let failure:unknown;
    for(let n=0;n<2;n++)try{return castTransferReceipt(await this.api.request(this.transferPath(id),'GET'),id);}catch(error){failure=error;}
    throw failure;
  }
  private cancelTransfer(flight:CastTransferFlight):Promise<CastTransferReceipt|undefined>{
    if(!flight.id)return Promise.resolve(undefined);
    if(flight.cancellation)return flight.cancellation;
    const id=flight.id;
    flight.cancellation=(async()=>{
      let failure:unknown;
      for(let n=0;n<2;n++)try{
        const receipt=castTransferReceipt(await this.api.request(this.transferPath(id),'DELETE'),id);
        if(!receipt)throw new CastFailure('cast.error.noAnswer');return receipt;
      }catch(error){failure=error;}
      flight.cancellation=undefined;throw failure;
    })();return flight.cancellation;
  }
  /** Replay the same command; resolve uncertain commits with the original authenticated receipt. */
  private load(command: Extract<CastCommand, {type: 'load'}>): Promise<string> {
    if (this.loads.size >= 8 || !this.transport) return Promise.reject(new CastFailure('cast.error.busy'));
    const requestId = 'load-' + (++this.requestSequence),transport=this.transport;
    const queue=command.queue&&typeof command.queue==='object'?command.queue:undefined,transferId=queue?.transferId;
    return new Promise<string>((resolve, reject) => {
      const retries:unknown[]=[];
      const finish = (error?: Error, itemId = command.itemId) => { if (!this.loads.delete(requestId)) return; this.clearTimer(timer);for(const retry of retries)this.clearTimer(retry);if (error) reject(error); else resolve(itemId); };
      const recover=async()=>{try{
        const receipt=typeof transferId==='string'?await this.transferStatus(transferId):undefined;
        if(receipt?.state==='committed'&&receipt.itemId===command.itemId){finish(undefined,receipt.itemId);return;}
        finish(new CastFailure('cast.error.noAnswer'));
      }catch{finish(new CastFailure('cast.error.unreachable'));}};
      const timer = this.setTimer(() => {void recover();}, 150_000);
      this.loads.set(requestId, {resolve: itemId => finish(undefined, itemId), reject: error => finish(error)});
      const send=()=>{if(this.loads.has(requestId)&&this.transport===transport)void transport.send({...command,requestId}).catch(()=>{if(!transferId)finish(new CastFailure('cast.error.unreachable'));});};
      if(transferId)for(const delay of [10_000,30_000])retries.push(this.setTimer(send,delay));
      send();
    });
  }

  private receive(e: CastEvent) {
    switch (e.type) {
      case 'paired': if(e.controlToken){try{this.controllerStorage?.set(this.controllerKey(),e.controlToken);}catch{}} this.waiting?.resolve(); this.waiting = undefined; return;
      // The TV is asking the person watching; keep waiting (the pairing timeout allows for it).
      case 'pair-pending': if (this.state.phase === 'pairing') this.publish({phase: 'waiting'}); return;
      case 'pair-failed': this.waiting?.reject(new CastFailure(e.code === 'tv_busy' ? 'cast.error.tvBusy' : 'cast.error.refused')); this.waiting = undefined; return;
      case 'busy': this.fail('cast.error.busy'); return;
      case 'pair-required': this.fail('cast.error.lostPairing'); return;
      case 'loaded': this.loads.get(e.requestId)?.resolve(e.itemId); return;
      case 'load-failed': if (e.requestId) this.loads.get(e.requestId)?.reject(new CastFailure('cast.error.loadFailed')); else this.fail('cast.error.loadFailed'); return;
      case 'takeover-requested': {
        this.clearTimer(this.takeoverTimer);
        this.publish({takeover: Object.freeze({requester: e.requester || serviceText('cast.someone')})});
        // The TV treats 30 s without an answer as no; drop the prompt with it.
        this.takeoverTimer = this.setTimer(() => this.publish({takeover: undefined}), CAST_TAKEOVER_WINDOW_MS);
        return;
      }
      case 'replaced':
        this.clearTimer(this.takeoverTimer);
        this.release(false);
        this.publish({phase: 'replaced', takeover: undefined});
        return;
      case 'status':
        if (this.state.phase === 'casting') this.publish({...(!e.itemId?{phase:'off' as const}:{}),itemId: e.itemId || undefined, positionSeconds: e.positionSeconds, durationSeconds: e.durationSeconds, paused: e.paused, audioTracks: e.audioTracks, textTracks: e.textTracks});
        return;
    }
  }

  private fail(id: MessageId) { this.publish({phase: 'error', errorId: id, error: serviceText(id)}); }

  private sessionEnded() {
    this.release(false, false);
    if (this.state.phase !== 'error' && this.state.phase !== 'replaced') this.publish({phase: 'off', itemId: undefined, title: undefined, deviceName: undefined, takeover: undefined});
  }

  /** Detach from the transport; `stopReceiver` closes Portico on the TV. */
  private release(stopReceiver: boolean, end = true) {
    this.startGeneration++;
    const flight=this.transferFlight;this.transferFlight=undefined;const released=this.startGeneration;
    if(flight?.id)this.recoveryTransfer=flight;
    if(flight)void this.cancelTransfer(flight).then(receipt=>{
      if(this.disposed||this.transport||this.startGeneration!==released)return;
      if(receipt?.state==='committed'&&receipt.itemId===flight.itemId)this.publish({acceptedSource:flight.source});
      this.recoveryTransfer=undefined;this.publish({phase:'off',acceptedSource:undefined,transferPending:false});
    },()=>{if(!this.disposed&&!this.transport&&this.startGeneration===released){this.recoveryTransfer=flight;this.fail('cast.error.unreachable');}});
    for (const off of this.detach.splice(0)) off();
    this.waiting?.reject(new CastFailure('cast.error.noAnswer'));
    this.waiting = undefined;
    for (const pending of [...this.loads.values()]) pending.reject(new CastFailure('cast.error.noAnswer'));
    const t = this.transport;
    this.transport = undefined;
    if (t && end) t.end(stopReceiver);
  }

  private send(message: CastCommand) { void this.transport?.send(message).catch(() => {}); }

  /** The controller's answer to `takeover-requested`. */
  answerTakeover(allow: boolean) {
    this.clearTimer(this.takeoverTimer);
    this.send({type: 'takeover', allow});
    this.publish({takeover: undefined});
  }
  play() { this.send({type: 'play'}); this.publish({paused: false}); }
  pause() { this.send({type: 'pause'}); this.publish({paused: true}); }
  toggle() { if (this.state.paused) this.play(); else this.pause(); }
  seek(positionSeconds: number) { const p = Math.max(0, positionSeconds); this.send({type: 'seek', positionSeconds: p}); this.publish({positionSeconds: p}); }
  setAudioTrack(trackId: number) { if (Number.isSafeInteger(trackId)) this.send({type: 'audio', trackId}); }
  /** `null` turns subtitles off. */
  setSubtitleTrack(trackId: number | null) { this.send({type: 'subtitles', trackIds: trackId === null ? [] : [trackId]}); }
  stop() {
    this.clearTimer(this.takeoverTimer);
    const flight=this.transferFlight??this.recoveryTransfer;
    if(flight?.id){
      this.transferFlight=undefined;const generation=++this.startGeneration;
      void this.cancelTransfer(flight).then(receipt=>{
        if(this.disposed||generation!==this.startGeneration||this.transport!==undefined&&this.transport!==flight.transport)return;
        if(receipt?.state==='committed'&&receipt.itemId===flight.itemId)this.publish({acceptedSource:flight.source});
        this.recoveryTransfer=undefined;this.release(false);
        this.publish({phase:'off',acceptedSource:undefined,transferPending:false,itemId:undefined,title:undefined,deviceName:undefined,error:undefined,errorId:undefined,takeover:undefined});
      },()=>{if(!this.disposed&&generation===this.startGeneration){this.recoveryTransfer=flight;this.release(false);this.fail('cast.error.unreachable');}});
      return;
    }
    this.send({type: 'stop'});
    this.release(true);
    this.publish({phase: 'off', transferPending:false,itemId: undefined, title: undefined, deviceName: undefined, error: undefined, errorId: undefined, takeover: undefined});
  }
  dismiss() { if(this.state.transferPending){this.stop();return;}this.publish({phase: 'off', error: undefined, errorId: undefined}); }
  dispose() { this.clearTimer(this.takeoverTimer); this.settleAdoption?.(false); this.release(false); this.disposed = true; this.listeners.clear(); }
}
