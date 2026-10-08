import {randomId} from './random-id.ts';
import {readEventStream} from './sse.ts';
import {presentError} from './presentation/errors.ts';
import type {I18n, MessageId} from '../../i18n/src/index.ts';
import {serviceI18n} from './presentation/service-text.ts';
import {
 SOCIAL_PROTOCOL,groupCorrection,groupTargetPositionUs,parseGroupInvite,parseGroupList,parseGroupQueue,parseGroupSnapshot,parseGroupTransportReceipt,
 type Group,type GroupHostAuthority,type GroupInvite,type GroupQueue,type GroupReadiness,type GroupRepeatMode,type GroupTimeline,type GroupTransportCommand,
} from './social-playback.ts';

/** Watch Together for one signed-in viewer.
 *
 * The server owns the group's clock; each member plays its own copy and follows that clock.
 * This service keeps the room current over the event stream (resuming by ordinal, falling back
 * to a re-read when the stream is quiet), sends commands fenced on the revision it last saw, and
 * steers a local player through `GroupFollower`. A dropped stream never empties the room: what
 * was last known stays on screen while it reconnects. */

export type GroupApi=Readonly<{baseUrl:string;request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>}>;
/** Opens the authenticated event stream; the app supplies it because only it knows the bearer. */
export type GroupStreamOpener=(path:string,lastEventId:string,signal:AbortSignal)=>Promise<Response>;
export type GroupRoomPhase='idle'|'joining'|'live'|'reconnecting'|'ended'|'left';
export type GroupSessionSnapshot=Readonly<{
 directory:readonly Group[];directoryPhase:'idle'|'loading'|'ready'|'error';directoryError?:string;
 /** The catalogue ID behind `directoryError` / `error`, for clients that render in another language later. */
 directoryErrorId?:MessageId;errorId?:MessageId;
 phase:GroupRoomPhase;group?:Group;queue?:GroupQueue;invite?:GroupInvite;busy:boolean;error?:string;
 /** A command the server refused because somebody in the group cannot see the title. Only the
  * host may repeat it with `allowUnavailable`, and the group is told when they do. */
 refused?:Readonly<{command:GroupTransportCommand;input:Readonly<{positionUs?:string;entryId?:string;queuePosition?:number}>}>;
 /** Add to `Date.now()` for the server's clock. */
 clockOffsetMs:number;
}>;
type Options=Readonly<{api:GroupApi;stream:GroupStreamOpener;key?:()=>string;now?:()=>number;heartbeatMs?:number;
 /** The viewer's catalogue; messages default to en-US. */
 i18n?:I18n}>;

const HEARTBEAT_MS=10000,SILENCE_MS=40000;
/** BE-MEDIA-14: NTP-style clock. Each request that answers with `serverTime` is a sample: offset =
 * serverTime − the request's midpoint, error at most half its round trip. The clock follows the
 * sample with the least round trip among the last CLOCK_SAMPLES taken within CLOCK_WINDOW_MS, so one
 * slow answer never drags the group clock. A pushed stream heartbeat has no round trip: it is used
 * only while no request sample is in the window. */
const CLOCK_SAMPLES=8,CLOCK_WINDOW_MS=120000;
type ClockSample={offsetMs:number;rttMs:number;at:number};
/** Catalogue-backed service messages: a known code or status names the problem; otherwise the
 * shared presenter, or this action's own message when the failure has no category (X-04). */
const code=(e:unknown)=>(e as {code?:string}|null)?.code??'';
const status=(e:unknown)=>(e as {status?:number}|null)?.status??0;
const path=(id:string,rest='')=>'/v1/groups/'+encodeURIComponent(id)+rest;

export class GroupSessionService{
 private api:GroupApi;private stream:GroupStreamOpener;private key:()=>string;private now:()=>number;private heartbeatMs:number;
 private state:GroupSessionSnapshot=Object.freeze<GroupSessionSnapshot>({directory:[],directoryPhase:'idle',phase:'idle',busy:false,clockOffsetMs:0});
 private listeners=new Set<()=>void>();private disposed=false;private lastCode='';private clock:ClockSample[]=[];
 private room?:{id:string;abort:AbortController;lastEventId:string;beat?:ReturnType<typeof setInterval>;retry?:ReturnType<typeof setTimeout>;watchdog?:ReturnType<typeof setTimeout>;rereading?:Promise<void>;queueReading?:Promise<void>};
 constructor(options:Options){this.api=options.api;this.stream=options.stream;this.key=options.key??(()=>randomId());this.now=options.now??(()=>Date.now());this.heartbeatMs=options.heartbeatMs??HEARTBEAT_MS;this.ownI18n=options.i18n;}
 private ownI18n?:I18n;
 private get i18n():I18n{return this.ownI18n??serviceI18n();}
 /** Use the viewer's catalogue from now on (the region can change after the service starts). */
 setI18n(i18n:I18n){this.ownI18n=i18n;}
 private say(e:unknown,fallback:MessageId):MessageId|{text:string;id?:MessageId}{
  const own=(e as {messageId?:unknown}|null)?.messageId;if(typeof own==='string')return own as MessageId;
  const p=presentError(e,'together',{operation:'action',i18n:this.i18n});
  return p.category==='unknown'?fallback:{text:p.body,id:p.messageId as MessageId};
 }
 private problem(what:MessageId|{text:string;id?:MessageId},field:'error'|'directoryError'='error'){
  const text=typeof what==='string'?this.i18n.t(what):what.text,id=typeof what==='string'?what:what.id;
  return field==='error'?{error:text,errorId:id}:{directoryError:text,directoryErrorId:id};
 }
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<GroupSessionSnapshot>){if(this.disposed)return;this.state=Object.freeze({...this.state,...patch});this.listeners.forEach(f=>f());}
 dispose(){this.close('idle');this.disposed=true;this.listeners.clear();}
 serverNow(){return this.now()+this.state.clockOffsetMs;}

 /** Groups this viewer belongs to. A failed refresh keeps the list it had. */
 async refreshDirectory():Promise<void>{
  if(this.state.directoryPhase==='idle')this.publish({directoryPhase:'loading'});
  try{this.publish({directory:parseGroupList(await this.api.request<unknown>('/v1/groups')).filter(g=>g.state!=='ended'&&g.state!=='failed'),directoryPhase:'ready',directoryError:undefined});}
  catch(e){this.publish({directoryPhase:this.state.directory.length?'ready':'error',...(status(e)===404||status(e)===503?this.problem('together.error.unavailable','directoryError'):this.problem(this.say(e,'together.error.load'),'directoryError'))});}
 }

 async create(input:{name:string;displayName:string;hostAuthority:GroupHostAuthority}):Promise<boolean>{
  // The server binds the group to this signed-in device (B8a): hosting needs no playback authority.
  return this.enter(async()=>parseGroupSnapshot(await this.api.request<unknown>('/v1/groups','POST',{protocolVersion:SOCIAL_PROTOCOL,name:input.name.trim(),displayName:input.displayName.trim(),hostAuthority:input.hostAuthority})),'together.error.create');
 }
 async join(inviteCode:string,displayName:string):Promise<boolean>{
  const tidy=inviteCode.replace(/[\s-]/g,'').toUpperCase();
  return this.enter(async()=>parseGroupSnapshot(await this.api.request<unknown>('/v1/groups/join','POST',{protocolVersion:SOCIAL_PROTOCOL,code:tidy,displayName:displayName.trim()})),'together.error.join');
 }
 /** Re-enters a group this viewer already belongs to. */
 async open(groupId:string):Promise<boolean>{
  if(this.room?.id===groupId&&this.state.group)return true;
  return this.enter(async()=>parseGroupSnapshot(await this.api.request<unknown>(path(groupId))),'together.error.open');
 }
 private async enter(read:()=>Promise<{serverTime:string;group:Group}>,fallback:MessageId):Promise<boolean>{
  this.close('joining');this.publish({busy:true,error:undefined,invite:undefined,queue:undefined});
  try{
   const sent=this.now();const snapshot=await read();if(this.disposed)return false;
   this.adopt(snapshot,{sent,received:this.now()});this.follow(snapshot.group.id);return true;
  }catch(e){this.publish({phase:'idle',...this.problem(this.say(e,fallback))});return false;}
  finally{this.publish({busy:false});}
 }
 /** `timing`: when the request that brought this snapshot was sent and answered (absent for pushed frames). */
 private adopt(snapshot:{serverTime:string;group:Group},timing?:{sent:number;received:number}){
  const at=Date.parse(snapshot.serverTime);
  const ended=snapshot.group.state==='ended'||snapshot.group.state==='failed';
  // Made host by someone else: the group follows no device until this one's heartbeat binds it.
  const promoted=!!this.state.group&&this.state.group.id===snapshot.group.id&&!this.state.group.permissions.isHost&&snapshot.group.permissions.isHost;
  this.publish({group:snapshot.group,queue:snapshot.group.queue??this.state.queue,phase:ended?'ended':this.state.phase==='joining'?'live':this.state.phase,...this.sample(at,timing)});
  if(ended)this.close('ended');
  else if(promoted&&this.room)this.beat(this.room);
 }

 /** Records one clock sample and returns the offset to publish (the least round trip in the window). */
 private sample(at:number,timing?:{sent:number;received:number}):{clockOffsetMs?:number}{
  if(!Number.isFinite(at))return {};
  const now=this.now();
  this.clock=this.clock.filter(x=>now-x.at<=CLOCK_WINDOW_MS);
  if(!timing||!(timing.received>=timing.sent)){
   // A pushed heartbeat: one-way, so its error is unknown; used only when nothing better is at hand.
   return this.clock.length?{}:{clockOffsetMs:at-now};
  }
  this.clock.push({offsetMs:at-(timing.sent+timing.received)/2,rttMs:timing.received-timing.sent,at:now});
  if(this.clock.length>CLOCK_SAMPLES)this.clock.splice(0,this.clock.length-CLOCK_SAMPLES);
  let best=this.clock[0]!;for(const x of this.clock)if(x.rttMs<best.rttMs)best=x;
  return {clockOffsetMs:Math.round(best.offsetMs)};
 }
 private follow(id:string){
  const room={id,abort:new AbortController(),lastEventId:''} as NonNullable<GroupSessionService['room']>;this.room=room;
  room.beat=setInterval(()=>this.beat(room),this.heartbeatMs);
  void this.connect(room,0);
 }
 /** A host's heartbeat also binds the group to the device it comes from. */
 private beat(room:NonNullable<GroupSessionService['room']>){
  if(this.room!==room)return;const sent=this.now();
  void this.api.request<unknown>(path(room.id,'/heartbeat'),'POST').then(raw=>{if(this.room===room)this.adopt(parseGroupSnapshot(raw),{sent,received:this.now()});},()=>{});
 }
 private async connect(room:NonNullable<GroupSessionService['room']>,attempt:number){
  if(this.room!==room||room.abort.signal.aborted)return;
  const alive=()=>{clearTimeout(room.watchdog);room.watchdog=setTimeout(()=>turn.abort(),SILENCE_MS);};
  const turn=new AbortController();const stop=()=>turn.abort();room.abort.signal.addEventListener('abort',stop,{once:true});
  let refused=0;
  try{
   const response=await this.stream(path(room.id,'/events'),room.lastEventId,turn.signal);
   if(!response.ok)refused=response.status;
   alive();
   await readEventStream(response,event=>{
    if(this.room!==room)return;alive();attempt=0;
    if(event.id)room.lastEventId=event.id;
    if(this.state.phase==='reconnecting'||this.state.phase==='joining')this.publish({phase:'live',error:undefined});
    this.receive(room,event.event,event.data);
   },turn.signal);
  }catch(e){refused=refused||status(e);}
  finally{clearTimeout(room.watchdog);room.abort.signal.removeEventListener('abort',stop);}
  if(this.room!==room||room.abort.signal.aborted)return;
  // Refused outright: this viewer is no longer in the group (removed, or it ended while away).
  if(refused===403||refused===404||refused===410){void this.reread(room);if(refused!==403){this.publish({phase:'ended'});this.close('ended');return;}}
  this.publish({phase:'reconnecting'});
  const delay=Math.min(15000,500*2**Math.min(attempt,5))*(0.75+Math.random()/2);
  room.retry=setTimeout(()=>void this.connect(room,attempt+1),delay);
 }
 private receive(room:NonNullable<GroupSessionService['room']>,kind:string,data:string){
  let body:unknown;try{body=JSON.parse(data);}catch{return;}
  try{
   if(kind==='group.snapshot'){this.adopt(parseGroupSnapshot(body));return;}
   if(kind==='heartbeat'){const clock=this.sample(Date.parse((body as {serverTime?:string}).serverTime??''));if(clock.clockOffsetMs!==undefined)this.publish(clock);return;}
   if(kind==='resume-gap'){room.lastEventId='';return;}
   if(kind==='group.ended'){void this.reread(room).finally(()=>{if(this.room===room){this.publish({phase:'ended'});this.close('ended');}});return;}
   // The clock is applied the moment it arrives; everything else in the room is re-read, because
   // members and the queue are projected per viewer and cannot ride in a shared event.
   if(kind==='group.transport'&&this.state.group){const timeline=timelineOf(body,this.state.group.timeline);if(timeline){const was=this.state.group.state,state=(was==='playing'||was==='paused'||was==='ready')&&(timeline.state==='playing'||timeline.state==='paused')?timeline.state:was;this.publish({group:Object.freeze({...this.state.group,timeline,state})});}}
   if(kind==='group.queue')void this.readQueue(room);
   void this.reread(room);
  }catch{/* a frame this client cannot read is skipped; the next re-read corrects the room */}
 }
 private reread(room:NonNullable<GroupSessionService['room']>):Promise<void>{
  return room.rereading??=(async()=>{
   try{const sent=this.now();const snapshot=parseGroupSnapshot(await this.api.request<unknown>(path(room.id),'GET',undefined,room.abort.signal));if(this.room===room)this.adopt(snapshot,{sent,received:this.now()});}
   catch{/* the room keeps what it had */}finally{room.rereading=undefined;}
  })();
 }
 private readQueue(room:NonNullable<GroupSessionService['room']>):Promise<void>{
  return room.queueReading??=(async()=>{
   try{const raw=await this.api.request<{queue?:unknown}>(path(room.id,'/queue'),'GET',undefined,room.abort.signal);if(this.room===room)this.publish({queue:parseGroupQueue((raw as {queue?:unknown}).queue??raw)});}
   catch{}finally{room.queueReading=undefined;}
  })();
 }
 refreshQueue(){return this.room?this.readQueue(this.room):Promise.resolve();}
 private close(phase:GroupRoomPhase){
  const room=this.room;this.room=undefined;
  if(room){room.abort.abort();clearInterval(room.beat);clearTimeout(room.retry);clearTimeout(room.watchdog);}
  if(phase==='idle'||phase==='joining'||phase==='left')this.publish({phase,group:undefined,queue:undefined,invite:undefined});else this.publish({phase});
 }

 private async act<T>(work:(group:Group)=>Promise<T>,fallback:MessageId):Promise<T|undefined>{
  const group=this.state.group,room=this.room;if(!group||!room)return undefined;
  this.publish({busy:true,error:undefined,refused:undefined});this.lastCode='';
  try{return await work(group);}
  catch(e){
   // A stale revision means somebody else moved the group first. Show what it is now and let the
   // person decide again; resending blindly is how a retried Play undoes someone's Pause.
   if(code(e)==='revision_conflict'){await this.reread(room);this.publish({...this.problem('together.error.changed')});}
   else this.publish({...this.problem(this.say(e,fallback))});
   this.lastCode=code(e);return undefined;
  }finally{this.publish({busy:false});}
 }
 async transport(command:GroupTransportCommand,input:{positionUs?:string;entryId?:string;queuePosition?:number;allowUnavailable?:boolean}={}):Promise<boolean>{
  const receipt=await this.act(async group=>parseGroupTransportReceipt(await this.api.request<unknown>(path(group.id,'/transport'),'POST',{protocolVersion:SOCIAL_PROTOCOL,idempotencyKey:this.key(),expectedRevision:group.revision,command,...(input.positionUs!==undefined?{positionUs:input.positionUs}:{}),...(input.entryId?{entryId:input.entryId}:{}),...(input.queuePosition!==undefined?{queuePosition:input.queuePosition}:{}),...(input.allowUnavailable?{allowUnavailable:true}:{})})),'together.error.notReached');
  if(receipt&&this.state.group)this.publish({group:Object.freeze({...this.state.group,timeline:receipt.timeline,revision:receipt.revision})});
  else if(!receipt&&this.lastCode==='media_no_longer_accessible'&&!input.allowUnavailable)this.publish({...this.problem('together.error.memberCantSee'),refused:Object.freeze({command,input:Object.freeze({positionUs:input.positionUs,entryId:input.entryId,queuePosition:input.queuePosition})})});
  if(receipt&&this.room)void this.reread(this.room);
  return !!receipt;
 }
 async settings(input:{shuffleEnabled?:boolean;repeatMode?:GroupRepeatMode}):Promise<boolean>{
  const done=await this.act(async group=>{await this.api.request<unknown>(path(group.id,'/settings'),'POST',{protocolVersion:SOCIAL_PROTOCOL,idempotencyKey:this.key(),expectedRevision:group.revision,...input});return true;},'together.error.setting');
  if(done&&this.room)void this.reread(this.room);return !!done;
 }
 async queueAdd(itemIds:readonly string[],replace=false):Promise<boolean>{return this.queueChange({operation:replace?'replace':'append',itemIds},'together.error.queueAdd');}
 async queueRemove(entryId:string):Promise<boolean>{return this.queueChange({operation:'remove',entryId},'together.error.queueRemove');}
 async queueMove(entryId:string,destinationEntryId:string,placement:'before'|'after'):Promise<boolean>{return this.queueChange({operation:'move',entryId,destinationEntryId,placement},'together.error.queueMove');}
 private async queueChange(body:Record<string,unknown>,fallback:MessageId):Promise<boolean>{
  const room=this.room;
  const done=await this.act(async group=>{
   const send=(revision:string)=>this.api.request<unknown>(path(group.id,'/queue'),'POST',{protocolVersion:SOCIAL_PROTOCOL,idempotencyKey:this.key(),expectedRevision:revision,...body});
   // Adding to a queue commutes with whatever else changed, so one retry on the new revision is safe.
   try{await send(this.state.queue?.revision??group.queueRevision);}catch(e){if(code(e)!=='revision_conflict'||!room)throw e;await this.readQueue(room);await send(this.state.queue?.revision??group.queueRevision);}
   return true;
  },fallback);
  if(done&&room){void this.readQueue(room);void this.reread(room);}
  return !!done;
 }
 /** Puts a title in front of the group: queued, then selected. Selecting never starts it; the
  * group prepares, and whoever may control presses Play once people are ready. */
 async watch(itemId:string):Promise<boolean>{
  const room=this.room;if(!room||!await this.queueAdd([itemId]))return false;
  await this.readQueue(room);await this.reread(room);
  const entry=[...(this.state.queue?.entries??[])].reverse().find(e=>e.itemId===itemId);
  return entry?this.transport('load',{entryId:entry.entryId}):false;
 }
 /** This member's own state: evidence for the group, never a command. Failures are silent. */
 async readiness(readiness:GroupReadiness,positionUs:string):Promise<void>{
  const room=this.room;if(!room)return;
  try{await this.api.request<unknown>(path(room.id,'/readiness'),'POST',{protocolVersion:SOCIAL_PROTOCOL,readiness,positionUs},room.abort.signal);}catch{}
 }
 async invite(input:{expiresInSeconds?:number;maxUses?:number}={}):Promise<boolean>{
  const invite=await this.act(async group=>parseGroupInvite(await this.api.request<unknown>(path(group.id,'/invites'),'POST',{protocolVersion:SOCIAL_PROTOCOL,expiresInSeconds:input.expiresInSeconds??900,maxUses:input.maxUses??8})),'together.error.invite');
  if(invite)this.publish({invite});return !!invite;
 }
 /** Hands the host role to another member, or (while the host is away) claims it for this one. */
 async makeHost(memberId:string):Promise<boolean>{
  const mine=this.state.group?.viewerMemberId===memberId;
  const done=await this.act(async group=>{
   const body={protocolVersion:SOCIAL_PROTOCOL,memberId,expectedRevision:group.revision};
   // Claiming binds this device at once; handing over leaves the incoming host's heartbeat to bind its own.
   const sent=this.now();
   this.adopt(parseGroupSnapshot(await this.api.request<unknown>(path(group.id,'/host-transfer'),'POST',body)),{sent,received:this.now()});
   return true;
  },'together.error.host');
  return !!done;
 }
 async leave():Promise<void>{
  const room=this.room;if(!room)return;
  this.close('left');
  try{await this.api.request<unknown>(path(room.id,'/leave'),'POST');}catch{/* the server ends a silent membership on its own */}
  void this.refreshDirectory();
 }
 async end():Promise<boolean>{
  const done=await this.act(async group=>{await this.api.request<unknown>(path(group.id,'/end'),'POST',{protocolVersion:SOCIAL_PROTOCOL,expectedRevision:group.revision});return true;},'together.error.end');
  if(done){this.publish({phase:'ended'});this.close('ended');void this.refreshDirectory();}
  return !!done;
 }
 /** Says something in the room's own voice, e.g. why a button did nothing. */
 notice(message:string){this.publish({error:message});}
 /** Forgets an ended or left room so the directory shows again. */
 dismiss(){this.close('idle');this.publish({error:undefined});}
}

function timelineOf(body:unknown,previous:GroupTimeline):GroupTimeline|undefined{
 const v=body as Record<string,unknown>|null;if(!v||typeof v!=='object')return undefined;
 const rate=v.rate as {numerator?:unknown;denominator?:unknown}|undefined;
 if(typeof v.anchorPositionUs!=='string'||!/^(0|[1-9][0-9]{0,18})$/.test(v.anchorPositionUs)||typeof v.anchorAt!=='string'||!['idle','playing','paused','stopped'].includes(v.state as string))return undefined;
 const valid=(x:unknown)=>typeof x==='string'&&/^[1-9][0-9]{0,18}$/.test(x);
 return Object.freeze({itemId:typeof v.itemId==='string'?v.itemId:previous.itemId,currentEntryId:typeof v.currentEntryId==='string'?v.currentEntryId:previous.currentEntryId,state:v.state as GroupTimeline['state'],anchorPositionUs:v.anchorPositionUs,anchorAt:v.anchorAt,
  rate:rate&&valid(rate.numerator)&&valid(rate.denominator)?Object.freeze({numerator:rate.numerator as string,denominator:rate.denominator as string}):previous.rate,queuePosition:Number.isSafeInteger(v.queuePosition)?v.queuePosition as number:previous.queuePosition});
}

/** What a follower needs from whichever player the app has. Positions are seconds. */
export type GroupPlayer=Readonly<{
 /** The item loaded now, its position, and whether it is actually moving and able to. */
 state():{itemId:string;positionSeconds:number;playing:boolean;buffering:boolean;ready:boolean};
 load(itemId:string,positionSeconds:number,playing:boolean):void;
 play():void;pause():void;seek(positionSeconds:number):void;
 /** 1 restores normal speed. A player that cannot change speed may ignore it; drift then resolves by seeking. */
 setRate(rate:number):void;
}>;

/** Keeps one local player on the group's clock, and tells the group how this member is doing.
 * It corrects gently (a small speed change) before it corrects visibly (a seek), in the bands
 * the server publishes, and it never commands the group. */
export class GroupFollower{
 private timer?:ReturnType<typeof setInterval>;private correctingUntil=0;private lastReport=0;private lastReadiness='';private loadedFor='';private unsubscribe?:()=>void;
 private service:GroupSessionService;private player:GroupPlayer;private now:()=>number;
 constructor(service:GroupSessionService,player:GroupPlayer,now:()=>number=()=>Date.now()){this.service=service;this.player=player;this.now=now;}
 start(){if(this.timer)return;this.timer=setInterval(()=>this.tick(),1000);this.unsubscribe=this.service.subscribe(()=>this.tick());this.tick();}
 stop(){clearInterval(this.timer);this.timer=undefined;this.unsubscribe?.();this.unsubscribe=undefined;this.player.setRate(1);}
 /** Where the group is now, in seconds, or undefined when nothing is loaded. */
 target():number|undefined{
  const group=this.service.getSnapshot().group;if(!group||!group.timeline.itemId)return undefined;
  return Number(groupTargetPositionUs(group.timeline,Date.parse(group.timeline.anchorAt),this.service.serverNow())/1000n)/1000;
 }
 private tick(){
  const snapshot=this.service.getSnapshot(),group=snapshot.group;
  if(!group||(snapshot.phase!=='live'&&snapshot.phase!=='reconnecting'))return;
  const timeline=group.timeline,target=this.target();
  if(target===undefined||timeline.state==='idle'||timeline.state==='stopped'){if(timeline.state==='stopped'&&this.player.state().playing)this.player.pause();return;}
  const local=this.player.state(),wantPlaying=timeline.state==='playing';
  if(local.itemId!==timeline.itemId){
   // Load once per entry; a player still starting up is not asked again every second.
   const key=timeline.itemId+'/'+timeline.currentEntryId;
   if(this.loadedFor!==key){this.loadedFor=key;this.player.load(timeline.itemId,target,wantPlaying);}
   this.report('buffering',local.positionSeconds);return;
  }
  this.loadedFor=timeline.itemId+'/'+timeline.currentEntryId;
  if(!local.ready){this.report('buffering',local.positionSeconds);return;}
  if(wantPlaying&&!local.playing&&!local.buffering)this.player.play();
  if(!wantPlaying&&local.playing)this.player.pause();
  const driftUs=BigInt(Math.round((target-local.positionSeconds)*1e6)),targetUs=BigInt(Math.round(target*1e6));
  const correction=groupCorrection(group.sync,driftUs,targetUs);
  const at=this.now();
  if(correction.action==='seek'||(!wantPlaying&&correction.action==='rate')){this.player.setRate(1);this.correctingUntil=0;this.player.seek(target);}
  else if(correction.action==='rate'&&wantPlaying){if(!this.correctingUntil){this.correctingUntil=at+(correction.holdMs??4000);this.player.setRate(correction.rate??1);}else if(at>this.correctingUntil){this.player.setRate(1);this.correctingUntil=0;this.player.seek(target);}}
  else if(this.correctingUntil){this.player.setRate(1);this.correctingUntil=0;}
  this.report(local.buffering?'buffering':correction.action==='none'||correction.action==='rate'?'ready':'lagging',local.positionSeconds);
 }
 private report(readiness:GroupReadiness,positionSeconds:number){
  const at=this.now();
  // Often enough that the server never counts this member stale (30 s), and at once on a change.
  if(readiness===this.lastReadiness&&at-this.lastReport<10000)return;
  this.lastReadiness=readiness;this.lastReport=at;
  void this.service.readiness(readiness,String(Math.max(0,Math.round(positionSeconds*1e6))));
 }
}
