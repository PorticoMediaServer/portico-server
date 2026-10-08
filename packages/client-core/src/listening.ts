import {unreadableServerResponse} from './server-messages.ts';
/** One listening policy for a proved account/profile/server/device lane.
 * Foreground engines enforce it here; the native owner enforces the same policy
 * when JavaScript is suspended. The queue alone selects accepted occurrences. */
import type {SavedScope} from './saved.ts';
import type {PlaybackService} from './index.ts';
import type {QueueScope,QueueItemInput} from './queues.ts';
import {parseQueueItem} from './queues.ts';
import {PlayerChaptersService} from './player-chapters.ts';

export const listeningRates=[.5,.75,1,1.25,1.5,1.75,2] as const;
export const passoutChoices=[0,30,60,90,120] as const;
export type ListeningPreferences=Readonly<{revision:number;musicRate:number;bookRate:number;autoplayNext:boolean;passoutMinutes:number}>;
export type ListeningTarget=Readonly<{libraryId:string;kind:'library'|'song'|'artist'|'album'|'disc'|'book';id:string}>;
export type ListeningJourney=Readonly<{target:ListeningTarget;actions:readonly ('play'|'shuffle'|'mix'|'enqueue')[];resume?:Readonly<{itemId:string;positionSeconds:number}>;resumeUnavailable:boolean}>;
export type ListeningItem=Readonly<{itemId:string;libraryId:string;kind:string;bookId?:string;lastBookItemId?:string;bookEndAvailable?:boolean;albumId?:string;artistId?:string}>;
export type ListeningRequest=(path:string,body?:unknown,method?:'GET'|'POST'|'PUT',signal?:AbortSignal)=>Promise<unknown>;
export type SleepChoice='off'|'chapter'|'item'|'book'|15|30|45|60|90;
export type ListeningTimer=Readonly<{choice:SleepChoice;label:string;deadline?:number;wallDeadline?:number;libraryId?:string;endSeconds?:number;itemId?:string;bookId?:string;sessionId?:string;generation?:number}>;
export type ListeningSnapshot=Readonly<{phase:'loading'|'ready'|'saving'|'error'|'disposed';preferences:ListeningPreferences|null;context:ListeningItem|null;rate:number;timer:ListeningTimer|null;timerLoading:boolean;stopped:boolean;message:string|null;error:string|null}>;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9_-]{1,128}$/.test(v);
const finite=(v:unknown):v is number=>typeof v==='number'&&Number.isFinite(v)&&v>=0;
const counter=(v:unknown):v is number=>Number.isSafeInteger(v)&&Number(v)>=0;
function bad():never{throw new Error(unreadableServerResponse);}
export function parseListeningPreferences(v:unknown):ListeningPreferences {
 if(!object(v)||!counter(v.revision)||!listeningRates.includes(v.musicRate as never)||!listeningRates.includes(v.bookRate as never)||typeof v.autoplayNext!=='boolean'||!passoutChoices.includes(v.passoutMinutes as never))bad();
 return Object.freeze({revision:v.revision,musicRate:v.musicRate as number,bookRate:v.bookRate as number,autoplayNext:v.autoplayNext,passoutMinutes:v.passoutMinutes as number});
}
export function listeningData(raw:unknown,scope:QueueScope):unknown {
 if(!object(raw)||!object(raw.scope)||(['serverId','authority','accountId','profileId'] as const).some(k=>(raw.scope as Record<string,unknown>)[k]!==scope[k]))bad();
 return raw.data;
}
export function parseListeningJourney(v:unknown,libraryId:string):ListeningJourney {
 if(!object(v)||!object(v.target)||v.target.libraryId!==libraryId||!['library','song','artist','album','disc','book'].includes(String(v.target.kind))||typeof v.target.id!=='string'||!(/^[A-Za-z0-9_-]{1,128}(?::disc:\d{1,4})?$/.test(v.target.id))||!Array.isArray(v.actions)||v.actions.some(a=>!['play','shuffle','mix','enqueue'].includes(String(a)))||typeof v.resumeUnavailable!=='boolean')bad();
 let resume:ListeningJourney['resume'];if(v.resume!==undefined){if(!object(v.resume)||!id(v.resume.itemId)||!finite(v.resume.positionSeconds))bad();resume=Object.freeze({itemId:v.resume.itemId,positionSeconds:v.resume.positionSeconds});}
 return Object.freeze({target:Object.freeze({libraryId,kind:v.target.kind as ListeningTarget['kind'],id:v.target.id}),actions:Object.freeze([...v.actions]) as ListeningJourney['actions'],...(resume?{resume}:{}),resumeUnavailable:v.resumeUnavailable});
}
function parseItem(v:unknown,itemId:string):ListeningItem {
 if(!object(v)||v.itemId!==itemId||!id(v.libraryId)||typeof v.kind!=='string')bad();
 if(v.bookEndAvailable!==undefined&&typeof v.bookEndAvailable!=='boolean')bad();
 for(const k of ['bookId','lastBookItemId','albumId','artistId'])if(v[k]!==undefined&&!id(v[k]))bad();
 return Object.freeze({itemId,libraryId:v.libraryId,kind:v.kind,...(typeof v.bookEndAvailable==='boolean'?{bookEndAvailable:v.bookEndAvailable}:{}),...(v.bookId?{bookId:v.bookId as string}:{}),...(v.lastBookItemId?{lastBookItemId:v.lastBookItemId as string}:{}),...(v.albumId?{albumId:v.albumId as string}:{}),...(v.artistId?{artistId:v.artistId as string}:{})});
}
/** Expands the server's complete selection through signed, revision-bound pages.
 * Refuses an over-capacity selection rather than silently using its first page. */
export async function loadListeningSelection(request:ListeningRequest,scope:QueueScope,target:ListeningTarget,options:{mix?:boolean;resume?:boolean;capacity?:number;startItem?:string;signal?:AbortSignal}={}) {
 const entries:QueueItemInput[]=[],seen=new Set<string>(),cursors=new Set<string>();let cursor='',seed='',total=-1,unavailable=0,startSeconds:number|undefined;
 const capacity=Math.max(0,Math.min(1000,options.capacity??1000));
 do {
  if(options.signal?.aborted)throw new Error('Listening action cancelled.');
  const q=new URLSearchParams({kind:target.kind,entityId:target.id,mode:options.mix?'mix':'ordered',resume:String(options.resume??false),limit:'100'});if(options.startItem)q.set('startItem',options.startItem);if(cursor)q.set('cursor',cursor);if(seed)q.set('seed',seed);
  const d=listeningData(await request(`/v1/libraries/${encodeURIComponent(target.libraryId)}/listening/selection?${q}`,undefined,'GET',options.signal),scope);
  if(!object(d)||!object(d.target)||d.target.libraryId!==target.libraryId||d.target.kind!==target.kind||d.target.id!==target.id||!counter(d.totalCount)||!counter(d.unavailableCount)||!Array.isArray(d.entries)||d.entries.length>100||typeof d.nextCursor!=='string'||d.nextCursor.length>4096||typeof d.seed!=='string'||d.seed.length>128)bad();
  if(total!==-1&&(total!==d.totalCount||seed!==d.seed||unavailable!==d.unavailableCount))bad();
  total=d.totalCount;seed=d.seed;unavailable=d.unavailableCount;
  if(total>capacity)throw new Error(`This selection has ${total.toLocaleString()} available items; this device queue has room for ${capacity.toLocaleString()}. Use the page actions or remove queue entries first (1,000 maximum).`);
  if(d.resume!==undefined){if(!object(d.resume)||!id(d.resume.itemId)||!finite(d.resume.positionSeconds))bad();if(!entries.length)startSeconds=d.resume.positionSeconds;}
  for(const raw of d.entries){const row=parseQueueItem(raw);if(seen.has(row.itemId))bad();seen.add(row.itemId);entries.push(row);}
  cursor=d.nextCursor;if(cursor){if(!d.entries.length||cursors.has(cursor))bad();cursors.add(cursor);}
 }while(cursor);
 if(entries.length!==total)bad();
 if(!entries.length)throw new Error('No available listening items were found. Check the library source or choose another item.');
 return Object.freeze({entries:Object.freeze(entries),seed,unavailableCount:unavailable,startSeconds});
}

export class ListeningService {
 private state:ListeningSnapshot=Object.freeze({phase:'loading',preferences:null,context:null,rate:1,timer:null,timerLoading:false,stopped:false,message:null,error:null});
 private listeners=new Set<()=>void>();private disposed=false;private connected=false;private abort=new AbortController();private unsubscribe?:()=>void;private interval?:ReturnType<typeof setInterval>;
 private contextGeneration=0;private timerGeneration=0;private prefGeneration=0;private itemId='';private lastRateKey='';private lastActivity:number;private chapters?:PlayerChaptersService;private sessionKey='';
 private activityRevision=0;private activityAtMs=Date.now();
 private readonly scope:QueueScope;private readonly request:ListeningRequest;private readonly playback:PlaybackService;private readonly now:()=>number;
 /** PERF-24: whether an item can be a listening item (a song or audiobook file), when the caller
  * knows; video never asks for listening details. */
 private listens?:(itemId:string,sessionId:string)=>boolean;
 constructor(scope:QueueScope,request:ListeningRequest,playback:PlaybackService,now:()=>number=()=>performance.now(),listens?:(itemId:string,sessionId:string)=>boolean){this.scope=scope;this.request=request;this.playback=playback;this.now=now;this.listens=listens;this.lastActivity=this.now();}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<ListeningSnapshot>){if(this.disposed)return;this.state=Object.freeze({...this.state,...patch});this.syncNativePolicy();for(const fn of this.listeners)fn();}
 private syncNativePolicy(){
  const timer=this.state.timer;
  this.playback.setListeningPolicy?.({preferences:this.state.preferences?{musicRate:this.state.preferences.musicRate,bookRate:this.state.preferences.bookRate,autoplayNext:this.state.preferences.autoplayNext,passoutMinutes:this.state.preferences.passoutMinutes}:null,
   activityRevision:this.activityRevision,activityAtMs:this.activityAtMs,timerRevision:this.timerGeneration,
   timer:timer?{choice:timer.choice,label:timer.label,...(timer.wallDeadline===undefined?{}:{wallDeadline:timer.wallDeadline}),...(timer.endSeconds===undefined?{}:{endSeconds:timer.endSeconds}),...(timer.itemId?{itemId:timer.itemId}:{}),...(timer.bookId?{bookId:timer.bookId}:{}),...(timer.sessionId?{sessionId:timer.sessionId,generation:timer.generation}:{}),...(timer.libraryId?{libraryId:timer.libraryId}:{})}:null});
 }
 retryContext(){if(this.disposed)return;this.itemId='';this.publish({error:null});this.observe();}
 connect(){if(this.connected||this.disposed)return;this.connected=true;this.unsubscribe=this.playback.subscribe(()=>this.observe());this.interval=setInterval(()=>this.tick(),1000);void this.refreshPreferences();this.observe();}
 async refreshPreferences(){const g=++this.prefGeneration;this.publish({phase:'loading',error:null});try{const p=parseListeningPreferences(listeningData(await this.request('/v1/listening/preferences',undefined,'GET',this.abort.signal),this.scope));if(this.disposed||g!==this.prefGeneration)return;this.publish({phase:'ready',preferences:p,error:null});this.applyRate();}catch(e){if(!this.disposed&&g===this.prefGeneration)this.publish({phase:'error',error:'Listening preferences could not be loaded. Retry before changing them or enabling automatic progression.'});}}
 async savePreferences(patch:Partial<Omit<ListeningPreferences,'revision'>>){const old=this.state.preferences;if(this.disposed||!old||this.state.phase!=='ready')throw new Error('Load listening preferences before saving.');const next=parseListeningPreferences({...old,...patch});const g=++this.prefGeneration;this.publish({phase:'saving',error:null});
  try{const p=parseListeningPreferences(listeningData(await this.request('/v1/listening/preferences',next,'PUT',this.abort.signal),this.scope));if(this.disposed||g!==this.prefGeneration)return;this.interact();this.publish({phase:'ready',preferences:p,error:null});this.applyRate();}
  catch(e){if(this.disposed||g!==this.prefGeneration)return;const conflict=object(e)&&e.code==='listening_preferences_conflict';this.publish({phase:'error',error:conflict?'These preferences changed on another device. Reload them before saving again.':'The preference save could not be confirmed. Reload to recover its actual result.'});throw new Error(this.state.error!);}
 }
 async setRate(rate:number){const context=this.state.context;if(!context||!['song','audiobook_file'].includes(context.kind))throw new Error('Choose a music or audiobook item first.');await this.savePreferences(context.kind==='song'?{musicRate:rate}:{bookRate:rate});}
 interact(){if(this.disposed)return;this.lastActivity=this.now();this.activityAtMs=Date.now();this.activityRevision++;if(this.state.stopped)this.publish({stopped:false,message:null});else this.syncNativePolicy();}
 private applyRate(){const p=this.state.preferences,c=this.state.context,s=this.playback.getSnapshot();if(!p||!c||!['song','audiobook_file'].includes(c.kind)){this.lastRateKey='';if(this.state.rate!==1)this.publish({rate:1});return;}const rate=c.kind==='audiobook_file'?p.bookRate:p.musicRate;const key=`${s.session?.id??''}:${s.session?.generation??0}:${rate}`;
  if(this.state.rate!==rate)this.publish({rate});if(key!==this.lastRateKey){this.lastRateKey=key;if(s.session&&!s.channel&&!this.playback.isNativeListening?.())this.playback.setPlaybackRate(rate);}
 }
 private observe(){if(this.disposed)return;const p=this.playback.getSnapshot();let item=p.channel?'':p.itemId??'';
  // With a kind hook, an item is decided once its session exists (until then it waits).
  if(item&&this.listens&&item!==this.itemId&&!p.session)item=this.itemId;
  const sessionKey=`${p.session?.id??''}:${p.session?.generation??0}`;
  if(this.sessionKey!==sessionKey){this.sessionKey=sessionKey;const t=this.state.timer;if(t?.sessionId&&(t.sessionId!==p.session?.id||t.generation!==p.session?.generation))this.clearTimer('Chapter timer cleared because the playback source changed.');}
  if(item!==this.itemId){const old=this.state.context;this.itemId=item;const generation=++this.contextGeneration;this.publish({context:null});
   if(!item){this.clearTimer();this.publish({stopped:false,message:null});}
   else if(this.listens&&!this.listens(item,p.session?.id??'')){this.clearTimer();}
   else void this.request(`/v1/items/${encodeURIComponent(item)}/listening`,undefined,'GET',this.abort.signal).then(raw=>{if(this.disposed||generation!==this.contextGeneration)return;const context=parseItem(listeningData(raw,this.scope),item);const t=this.state.timer;
    if(!['song','audiobook_file'].includes(context.kind)||t?.choice==='book'&&t.bookId!==context.bookId||t?.choice==='item'&&t.itemId!==item||old&&old.libraryId!==context.libraryId)this.clearTimer();
    this.publish({context});this.applyRate();this.tick();
   }).catch(()=>{if(!this.disposed&&generation===this.contextGeneration)this.publish({error:'Listening details could not be loaded. Retry listening details; book and chapter timers were not inferred.'});});
  }
  this.applyRate();this.tick();
 }
 clearTimer(message:string|null=null){++this.timerGeneration;this.chapters?.dispose();this.chapters=undefined;this.publish({timer:null,timerLoading:false,message});}
 private installTimer(timer:ListeningTimer){++this.timerGeneration;this.publish({timer,timerLoading:false});}
 async setTimer(choice:SleepChoice){this.interact();this.clearTimer();if(choice==='off')return;
  const p=this.playback.getSnapshot(),c=this.state.context;if(!c||!p.session||p.channel||!['song','audiobook_file'].includes(c.kind))throw new Error('Start a music or audiobook item before setting a timer.');
  if(typeof choice==='number'){if(![15,30,45,60,90].includes(choice))throw new Error('Invalid timer choice.');this.installTimer(Object.freeze({choice,label:`${choice} minutes`,deadline:this.now()+choice*60000,wallDeadline:Date.now()+choice*60000,libraryId:c.libraryId}));return;}
  if(choice==='item'){this.installTimer(Object.freeze({choice,label:'End of this item',itemId:c.itemId,libraryId:c.libraryId}));return;}
  if(choice==='book'){if(!c.bookId||!c.lastBookItemId||c.bookEndAvailable!==true)throw new Error('The final book part is unavailable. Choose a minute, item or verified chapter timer instead.');this.installTimer(Object.freeze({choice,label:'End of this book',bookId:c.bookId,itemId:c.lastBookItemId,libraryId:c.libraryId}));return;}
  if(choice!=='chapter')throw new Error('Invalid timer choice.');
  const generation=this.timerGeneration,position=p.positionSeconds,sessionId=p.session.id,sessionGeneration=p.session.generation;
  const chapters=this.chapters=new PlayerChaptersService({scope:{serverId:this.scope.serverId,viewerId:JSON.stringify([this.scope.authority,this.scope.accountId,this.scope.profileId])},api:{request:<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal)=>this.request(path,body,method as 'GET',signal) as Promise<T>}});
  this.publish({timerLoading:true});
  try{await chapters.select({libraryId:c.libraryId,itemId:c.itemId,sessionId,sessionGeneration});for(let page=0;page<41;page++){
    if(this.disposed||generation!==this.timerGeneration)return;const state=chapters.getSnapshot(),data=state.data;if(state.phase!=='ready'||!data||data.status!=='available')throw new Error(state.error?.message??'Verified chapter timing is unavailable for this source.');
    const chapter=data.chapters.find(ch=>position>=ch.startSeconds&&position<ch.endSeconds);
    if(chapter){const latest=this.playback.getSnapshot();if(latest.session?.id!==sessionId||latest.session.generation!==sessionGeneration||latest.positionSeconds>=chapter.endSeconds)throw new Error('Playback moved to another chapter. Choose the timer again.');this.installTimer(Object.freeze({choice,label:`End of chapter: ${chapter.title}`,itemId:c.itemId,sessionId,generation:sessionGeneration,endSeconds:chapter.endSeconds,libraryId:c.libraryId}));return;}
    if(!data.nextCursor)break;await chapters.next();
   }throw new Error('No verified chapter contains the current position.');
  }catch(e){if(!this.disposed&&generation===this.timerGeneration){this.publish({timerLoading:false,error:e instanceof Error?e.message:'Chapter timer unavailable.'});throw e;}}
  finally{chapters.dispose();if(this.chapters===chapters)this.chapters=undefined;}
 }
 /** Called before natural advancement, and on foreground ticks/observations. */
 tick(){if(this.disposed)return;const p=this.playback.getSnapshot(),c=this.state.context;
  if(p.nativeListening){
   const native=p.nativeListening;
   if(native.timerStopped&&!this.state.stopped){this.publish({stopped:true,message:'The listening timer paused playback.'});}
   if(!native.timerStopped&&this.state.stopped)this.publish({stopped:false,message:null});
   if(this.state.timer&&native.expiredTimerRevision===this.timerGeneration){this.publish({timer:null,timerLoading:false});}
   return;
  }
  if(this.state.stopped)return;if(!c||p.channel||!p.session||!['song','audiobook_file'].includes(c.kind))return;
  const t=this.state.timer;let reason:string|null=null;
  if(t?.deadline!==undefined&&(this.now()>=t.deadline||t.wallDeadline!==undefined&&Date.now()>=t.wallDeadline))reason='Sleep timer ended.';
  if(t?.choice==='chapter'&&t.sessionId===p.session.id&&t.generation===p.session.generation&&!p.pendingSeek&&p.positionSeconds>=t.endSeconds!)reason='The selected chapter ended.';
  if(t&&(t.choice==='item'||t.choice==='book')&&t.itemId===c.itemId&&p.phase==='ended')reason=t.choice==='book'?'The book ended.':'The selected item ended.';
  const minutes=this.state.preferences?.passoutMinutes??0;if(minutes&&p.intent==='playing'&&this.now()-this.lastActivity>=minutes*60000)reason='Playback paused after the selected period without interaction.';
  if(reason){this.clearTimer();this.publish({stopped:true,message:reason});this.playback.pause();}
 }
 /** Prebuffering may span minute timers, but never accept an edge beyond a
  * selected item/chapter/book end or a deadline that precedes that edge. */
 mayTransition(remainingSeconds:number){
  if(!this.mayAdvance())return false;
  const t=this.state.timer,p=this.playback.getSnapshot();
  if(t?.choice==='chapter'||t&&(t.choice==='item'||t.choice==='book')&&t.itemId===p.itemId)return false;
  if(!Number.isFinite(remainingSeconds)||remainingSeconds<0)return false;
  if(t?.deadline!==undefined&&this.now()+remainingSeconds*1000>=t.deadline||t?.wallDeadline!==undefined&&Date.now()+remainingSeconds*1000>=t.wallDeadline)return false;
  const minutes=this.state.preferences?.passoutMinutes??0;
  return !minutes||this.now()+remainingSeconds*1000<this.lastActivity+minutes*60000;
 }
 /** Songs and book parts continue while autoplay is on. Preferences not read yet (or unreadable)
  * count as the server's default, on: an album never stops after track 1 for want of a read. */
 mayAdvance(){this.tick();const c=this.state.context;return !this.disposed&&!this.state.stopped&&(!c||!['song','audiobook_file'].includes(c.kind)?!this.itemId||!!c:(this.state.preferences?.autoplayNext??true));}
 dispose(){if(this.disposed)return;this.disposed=true;this.abort.abort();this.unsubscribe?.();clearInterval(this.interval);this.chapters?.dispose();this.state=Object.freeze({...this.state,phase:'disposed',preferences:null,context:null,timer:null,stopped:false});this.listeners.clear();}
}

export async function readListeningItem(request:ListeningRequest,scope:QueueScope,itemId:string,signal?:AbortSignal){return parseItem(listeningData(await request(`/v1/items/${encodeURIComponent(itemId)}/listening`,undefined,'GET',signal),scope),itemId);}
export function listeningItemInput(itemId:string,target?:ListeningTarget):QueueItemInput {
 const source=target&&['artist','album','book'].includes(target.kind)?{kind:target.kind as 'artist'|'album'|'book',id:target.id}:{kind:'item' as const,id:itemId};
 return parseQueueItem({itemId,editionId:null,partId:null,sourceContext:{...source,revision:null,entryId:null}});
}

/** Read-only entity navigation must remain available before a playback controller connects. */
export async function readListeningContext(request:ListeningRequest,scope:SavedScope,itemId:string,signal?:AbortSignal):Promise<ListeningItem>{
 const raw=await request(`/v1/items/${encodeURIComponent(itemId)}/listening`,undefined,'GET',signal);
 if(!object(raw)||!object(raw.scope)||raw.scope.serverId!==scope.serverId||JSON.stringify([raw.scope.authority,raw.scope.accountId,raw.scope.profileId])!==scope.viewerId)bad();
 return parseItem(raw.data,itemId);
}
