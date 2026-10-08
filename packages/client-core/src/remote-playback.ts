import {serviceProblem,serviceText} from './presentation/service-text.ts';
import {SOCIAL_PROTOCOL,parseHandoff,parseReceiver,parseReceiverGrant,parseReceiverList,type Handoff,type Receiver,type ReceiverGrant} from './social-playback.ts';
import {readEventStream} from './sse.ts';

/** Moving playback to a Portico television, and being that television.
 *
 * A handoff never guesses: the phone proposes, the television proves it is actually playing,
 * and only then does the phone commit and its own player retire. Anything short of that
 * (declined, timed out, television unreachable) leaves the phone playing exactly where it
 * was. `HandoffController` is the phone's half and `ReceiverHost` the television's. */

export type RemoteApi=Readonly<{request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>}>;
const status=(e:unknown)=>(e as {status?:number}|null)?.status??0;
const wait=(ms:number,signal:AbortSignal)=>new Promise<void>(resolve=>{if(signal.aborted){resolve();return;}const t=setTimeout(done,ms);function done(){clearTimeout(t);signal.removeEventListener('abort',done);resolve();}signal.addEventListener('abort',done,{once:true});});
const us=(seconds:number)=>String(Math.max(0,Math.round(seconds*1e6)));

export type HandoffPhase='idle'|'asking'|'preparing'|'starting'|'done'|'failed';
export type HandoffSnapshot=Readonly<{receivers:readonly Receiver[];directoryPhase:'idle'|'loading'|'ready'|'error';directoryError?:string;phase:HandoffPhase;target?:Receiver;message?:string}>;
export type HandoffSource=Readonly<{playbackId:string;positionSeconds:number}>;

export class HandoffController{
 private api:RemoteApi;private device:{deviceId:string;displayName:string};private requestId:()=>string;private pollMs:number;
 private state:HandoffSnapshot=Object.freeze<HandoffSnapshot>({receivers:[],directoryPhase:'idle',phase:'idle'});
 private listeners=new Set<()=>void>();private run?:AbortController;
 constructor(options:{api:RemoteApi;device:{deviceId:string;displayName:string};requestId:()=>string;pollMs?:number}){this.api=options.api;this.device=options.device;this.requestId=options.requestId;this.pollMs=options.pollMs??1000;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<HandoffSnapshot>){this.state=Object.freeze({...this.state,...patch});this.listeners.forEach(f=>f());}
 dispose(){this.run?.abort();this.listeners.clear();}

 /** Televisions and Cast devices this viewer may send to. A failed refresh keeps the list. */
 async refresh():Promise<void>{
  if(this.state.directoryPhase==='idle')this.publish({directoryPhase:'loading'});
  try{this.publish({receivers:parseReceiverList(await this.api.request<unknown>('/v1/receivers')).filter(r=>r.state==='active'),directoryPhase:'ready',directoryError:undefined});}
  catch(e){this.publish({directoryPhase:this.state.receivers.length?'ready':'error',directoryError:status(e)===404||status(e)===503?'':serviceProblem(e,'devices','playOn.devicesUnavailable','load')});}
 }
 cancel(){this.run?.abort();this.run=undefined;if(this.state.phase!=='done')this.publish({phase:'idle',target:undefined,message:undefined});}

 /** Resolves true once the television has taken over and this device's player should stop. */
 async send(target:Receiver,source:HandoffSource):Promise<boolean>{
  this.run?.abort();const run=this.run=new AbortController(),signal=run.signal;
  const fail=(message:string)=>{if(!signal.aborted)this.publish({phase:'failed',message});return false;};
  let handoff:Handoff|undefined;const device={device:target.displayName};
  try{
   this.publish({phase:'asking',target,message:undefined});
   let grant=parseReceiverGrant(await this.api.request<unknown>(`/v1/receivers/${encodeURIComponent(target.id)}/grants`,'POST',{protocolVersion:SOCIAL_PROTOCOL,controllerDeviceId:this.device.deviceId,controllerDisplayName:this.device.displayName.slice(0,64)},signal));
   // A television that has not met this device asks its own viewer first.
   for(let waited=0;grant.state==='pending'&&waited<60000&&!signal.aborted;waited+=this.pollMs){
    this.publish({message:serviceText('playOn.approveOn',device)});
    await wait(this.pollMs,signal);
    const list=await this.api.request<{grants?:unknown[]}>(`/v1/receivers/${encodeURIComponent(target.id)}/grants`,'GET',undefined,signal);
    const mine=(list.grants??[]).find(g=>(g as {id?:string}|null)?.id===grant.id);
    if(mine)grant=parseReceiverGrant({protocolVersion:SOCIAL_PROTOCOL,serverTime:'',grant:mine});
   }
   if(signal.aborted)return false;
   if(grant.state==='pending')return fail(serviceText('playOn.didNotAnswer',device));
   if(grant.state!=='accepted')return fail(serviceText(grant.state==='declined'?'playOn.declined':'playOn.notAccepting',device));
   this.publish({phase:'preparing',message:undefined});
   handoff=parseHandoff(await this.api.request<unknown>('/v1/handoffs','POST',{protocolVersion:SOCIAL_PROTOCOL,requestId:this.requestId(),receiverId:target.id,grantId:grant.id,sourcePlaybackId:source.playbackId,startPositionUs:us(source.positionSeconds)},signal));
   this.publish({phase:'starting'});
   // The television proves it is playing; until it does, this device stays the player.
   while(handoff.state==='prepared'&&handoff.outcome==='waiting'&&!signal.aborted){
    await wait(this.pollMs,signal);if(signal.aborted)break;
    handoff=parseHandoff(await this.api.request<unknown>('/v1/handoffs/'+encodeURIComponent(handoff.id),'GET',undefined,signal));
   }
   if(signal.aborted){void this.rollback(handoff);return false;}
   if(handoff.state!=='prepared'||handoff.outcome!=='pending')return fail(serviceText(handoff.state==='expired'?'playOn.didNotStartInTime':'playOn.couldNotStart',device));
   handoff=parseHandoff(await this.api.request<unknown>(`/v1/handoffs/${encodeURIComponent(handoff.id)}/commit`,'POST',{protocolVersion:SOCIAL_PROTOCOL,expectedRevision:handoff.revision},signal));
   if(handoff.state!=='committed')return fail(serviceText('playOn.couldNotTakeOver',device));
   this.publish({phase:'done',message:serviceText('playOn.playingOn',device)});
   return true;
  }catch(e){
   if(signal.aborted){if(handoff)void this.rollback(handoff);return false;}
   if(handoff&&handoff.state==='prepared')void this.rollback(handoff);
   return fail(serviceText('playOn.notReachable',device));
  }
 }
 private async rollback(handoff:Handoff){try{await this.api.request<unknown>(`/v1/handoffs/${encodeURIComponent(handoff.id)}/rollback`,'POST',{protocolVersion:SOCIAL_PROTOCOL,expectedRevision:handoff.revision,reason:'controller-cancelled'});}catch{/* it expires by itself */}}
}

/** What a television's player must offer to take a handoff. */
export type ReceiverPlayer=Readonly<{
 load(itemId:string,positionSeconds:number):void;
 state():{itemId:string;playbackId:string;playing:boolean;positionSeconds:number};
}>;
export type ReceiverPhase='off'|'registering'|'waiting'|'starting'|'error';
export type ReceiverSnapshot=Readonly<{phase:ReceiverPhase;receiver?:Receiver;asking?:ReceiverGrant;error?:string}>;
export type ReceiverDevice=Readonly<{deviceId:string;displayName:string;platform:string;keyFingerprint:string}>;

/** PERF-16: the idle inbox delay, `baseMs` ± 20%. `rand` is injected for tests. */
export function receiverPollDelay(baseMs:number,rand:()=>number=Math.random):number{
 const r=Math.min(0.999999,Math.max(0,rand()));
 return Math.round(baseMs*(0.8+0.4*r));
}

/** PERF-16: a `receiver.inbox_changed` hint on `/v1/notifications/events`. The event is a hint,
 * never playback authority: the named inbox is re-read and processed exactly as a poll would.
 * These frames deliberately carry no SSE id, so they never move a notification resume cursor
 * (this host keeps none and sends no `Last-Event-ID`). */
export type ReceiverInboxEventKind='grant'|'handoff'|'resync';
export type ReceiverInboxEvent=Readonly<{kind:ReceiverInboxEventKind;receiverId?:string}>;
/** Opens the authenticated notifications event stream; the app supplies it because only it knows
 * the bearer (same seam as `GroupStreamOpener`). Reuses the shared SSE reader in `sse.ts`. */
export type ReceiverEventOpener=(signal:AbortSignal)=>Promise<Response>;
/** While the event stream is connected, the inbox poll backs off to this safety interval. */
const RECEIVER_SAFETY_MS=60000;
/** Reconnect waits after a dropped stream: 5 s doubling to 20 s, jittered by the poll delay. */
const receiverReconnectDelay=(failures:number,rand:()=>number)=>receiverPollDelay(Math.min(20000,5000*2**Math.min(Math.max(failures-1,0),2)),rand);

const MAX_RECEIVER_ID=200;
/** Unknown kinds and shapes are skipped (a newer server), never applied. */
export function parseReceiverInboxEvent(value:unknown):ReceiverInboxEvent|undefined{
 if(!value||typeof value!=='object'||Array.isArray(value))return undefined;
 const v=value as {kind?:unknown;receiverId?:unknown};
 if(v.kind!=='grant'&&v.kind!=='handoff'&&v.kind!=='resync')return undefined;
 if(v.receiverId!==undefined&&(typeof v.receiverId!=='string'||!v.receiverId||v.receiverId.length>MAX_RECEIVER_ID))return undefined;
 return Object.freeze({kind:v.kind,...(typeof v.receiverId==='string'?{receiverId:v.receiverId}:{})});
}

export class ReceiverHost{
 private api:RemoteApi;private device:ReceiverDevice;private player:ReceiverPlayer;private pollMs:number;private heartbeatMs:number;private rand:()=>number;private events?:ReceiverEventOpener;
 /** True while the event stream is connected: the inbox poll backs off to the safety interval. */
 private streamConnected=false;private streamFailures=0;
 /** True while the player is in the foreground: the inbox poll pauses, the heartbeat doesn't. */
 private playerActive=false;
 private state:ReceiverSnapshot=Object.freeze<ReceiverSnapshot>({phase:'off'});private listeners=new Set<()=>void>();
 private run?:AbortController;private handled=new Set<string>();
  constructor(options:{api:RemoteApi;device:ReceiverDevice;player:ReceiverPlayer;pollMs?:number;heartbeatMs?:number;rand?:()=>number;events?:ReceiverEventOpener}){this.api=options.api;this.device=options.device;this.player=options.player;this.pollMs=options.pollMs??30000;this.heartbeatMs=options.heartbeatMs??45000;this.rand=options.rand??Math.random;this.events=options.events;}
  getSnapshot=()=>this.state;
  subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
  private publish(patch:Partial<ReceiverSnapshot>){this.state=Object.freeze({...this.state,...patch});this.listeners.forEach(f=>f());}
 /** Wakes the poll loop early (a dropped event stream falls back to the 30 s poll at once). */
 private sleeper?:{wake:()=>void};
 private sleep(ms:number,signal:AbortSignal){
  return new Promise<void>(resolve=>{
   if(signal.aborted){resolve();return;}
   const done=()=>{clearTimeout(t);signal.removeEventListener('abort',done);if(this.sleeper?.wake===wake)this.sleeper=undefined;resolve();};
   const wake=()=>done();
   const t=setTimeout(done,ms);
   this.sleeper={wake};
   signal.addEventListener('abort',done,{once:true});
  });
 }
  /** The player is in the foreground (something loaded: playing or paused). While set, the inbox
   * poll pauses; registration and the heartbeat continue so grants arrive on the next idle poll. */
  setPlayerActive(active:boolean){this.playerActive=active;}
 stop(){this.run?.abort();this.run=undefined;this.streamConnected=false;this.publish({phase:'off',asking:undefined});}
 dispose(){this.stop();this.listeners.clear();}

 /** Registers (again, harmlessly) and waits to be used. Failures back off and keep trying: a
  * television that lost its server for a minute should be available again without a visit. */
 start(){
  if(this.run)return;const run=this.run=new AbortController(),signal=run.signal;
  this.streamConnected=false;this.streamFailures=0;
  if(this.events)void this.followEvents(run,signal);
  void(async()=>{
   let failures=0,lastBeat=0;
   while(!signal.aborted){
    try{
     if(!this.state.receiver){
      this.publish({phase:'registering'});
      this.publish({receiver:parseReceiver((await this.api.request<{receiver?:unknown}>('/v1/receivers','POST',{protocolVersion:SOCIAL_PROTOCOL,deviceId:this.device.deviceId,displayName:this.device.displayName.slice(0,64),platform:this.device.platform,keyFingerprint:this.device.keyFingerprint,supportedCommands:['load','play','pause','seek','stop'],grantPolicy:'per-device'},signal)).receiver),phase:'waiting',error:undefined});
      lastBeat=Date.now();
     }
      const id=this.state.receiver!.id;
      if(Date.now()-lastBeat>=this.heartbeatMs){await this.api.request<unknown>(`/v1/receivers/${encodeURIComponent(id)}/heartbeat`,'POST',{protocolVersion:SOCIAL_PROTOCOL,keyFingerprint:this.device.keyFingerprint},signal);lastBeat=Date.now();}
      // PERF-16: no inbox poll while the player is in the foreground. While the event stream is
      // connected the poll backs off to the safety interval; on a dropped stream the 30 s poll
      // (with jitter) covers the gap until it reconnects.
      if(!this.playerActive)await this.refreshInbox(id,signal);
      else if(signal.aborted)return;
      failures=0;if(this.state.phase==='error')this.publish({phase:'waiting',error:undefined});
    }catch(e){
     if(signal.aborted)return;
     // The server forgot this receiver (or never knew it): register again next turn.
     if(status(e)===404)this.publish({receiver:undefined});
     failures++;this.publish({phase:'error',error:serviceProblem(e,'devices','playOn.receiverOffline','load')});
    }
    await this.sleep(failures?Math.min(60000,this.pollMs*2**Math.min(failures,5)):receiverPollDelay(this.streamConnected?RECEIVER_SAFETY_MS:this.pollMs,this.rand),signal);
   }
  })();
 }
 /** Reads one receiver inbox and works it like a poll: a new grant is asked about, a waiting
  * handoff is taken once it is genuinely playing. The event stream calls this too: the hint
  * only decides *when* to read, never what to play. */
 private async refreshInbox(id:string,signal:AbortSignal){
  const inbox=await this.api.request<{grants?:unknown[];handoffs?:unknown[]}>(`/v1/receivers/${encodeURIComponent(id)}/inbox`,'GET',undefined,signal);
  if(signal.aborted)return;
  const asking=(inbox.grants??[]).map(g=>parseReceiverGrant({protocolVersion:SOCIAL_PROTOCOL,serverTime:'',grant:g}))[0];
  if(asking?.id!==this.state.asking?.id)this.publish({asking});
  const handoff=(inbox.handoffs??[]).map(h=>parseHandoff({protocolVersion:SOCIAL_PROTOCOL,serverTime:'',handoff:h})).find(h=>h.outcome==='waiting'&&!this.handled.has(h.id));
  if(handoff){this.handled.add(handoff.id);void this.take(handoff,signal);}
 }
 /** Holds `/v1/notifications/events` open and turns `receiver.inbox_changed` into inbox reads.
  * A dropped stream never empties anything: the poll loop keeps covering until it reconnects. */
 private async followEvents(run:AbortController,signal:AbortSignal){
  while(!signal.aborted&&this.run===run){
   try{
    const response=await this.events!(signal);
    if(signal.aborted||this.run!==run)return;
    this.streamConnected=true;this.streamFailures=0;
    await readEventStream(response,event=>{
     if(signal.aborted||this.run!==run)return;
     if(event.event!=='receiver.inbox_changed')return;
     let body:unknown;try{body=JSON.parse(event.data);}catch{return;}
     const hint=parseReceiverInboxEvent(body);
     if(!hint||this.playerActive)return;
     // `grant`/`handoff` name their receiver; `resync` (and a hint without one) refreshes this
     // host's own inbox. The server sends `resync` on every connection.
     const id=hint.kind==='resync'?this.state.receiver?.id:(hint.receiverId??this.state.receiver?.id);
     if(!id)return;
     // A forgotten receiver re-registers on the next poll turn, as after a failed poll.
     void this.refreshInbox(id,signal).catch(e=>{if(status(e)===404)this.publish({receiver:undefined});});
    },signal);
   }catch{
    if(signal.aborted||this.run!==run)return;
   }
   this.streamConnected=false;
   if(signal.aborted||this.run!==run)return;
   // Fall back to the 30 s poll at once instead of sleeping out the safety interval.
   this.sleeper?.wake();
   this.streamFailures++;
   await wait(receiverReconnectDelay(this.streamFailures,this.rand),signal);
  }
 }
 async decide(grant:ReceiverGrant,decision:'accept'|'decline'):Promise<void>{
  const id=this.state.receiver?.id;if(!id)return;
  this.publish({asking:undefined});
  try{await this.api.request<unknown>(`/v1/receivers/${encodeURIComponent(id)}/grants/${encodeURIComponent(grant.id)}/decision`,'POST',{protocolVersion:SOCIAL_PROTOCOL,decision});}catch{/* it is asked again on the next poll */}
 }
 /** Loads what was sent and reports readiness only once it is genuinely playing. */
 private async take(handoff:Handoff,signal:AbortSignal){
  this.publish({phase:'starting'});
  this.player.load(handoff.itemId,Number(BigInt(handoff.requestedPositionUs)/1000n)/1000);
  const deadline=Date.parse(handoff.expiresAt)||Date.now()+120000;
  while(!signal.aborted&&Date.now()<deadline){
   await wait(500,signal);
   const now=this.player.state();
   if(now.itemId===handoff.itemId&&now.playing&&now.playbackId){
    try{await this.api.request<unknown>(`/v1/handoffs/${encodeURIComponent(handoff.id)}/readiness`,'POST',{protocolVersion:SOCIAL_PROTOCOL,readiness:'playing',receiverPlaybackId:now.playbackId,positionUs:us(now.positionSeconds),expectedRevision:handoff.revision},signal);}catch{/* the phone keeps playing; nothing to undo here */}
    break;
   }
  }
  if(!signal.aborted)this.publish({phase:'waiting'});
 }
}
