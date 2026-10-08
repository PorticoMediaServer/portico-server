import {randomId} from './random-id.ts';
/** The server side of downloads, as one framework-free service shared by web and Apple: what
 * the server is preparing for this profile, what may be asked for, and the actions on each.
 *
 * Forgiving by design: a failed refresh keeps the list that is on screen. While anything is
 * still being prepared it polls gently; once everything has settled it stops. */
import {serviceProblem} from './presentation/service-text.ts';
import {downloadReasonMessage,parseDownloadBatch,parseDownloadGrant,parseDownloadOptions,parseDownloadPage,parseDownloadPreparation,parseDownloadUsage,type DownloadAction,type DownloadBatch,type DownloadGrant,type DownloadOptionsView,type DownloadPreparation,type DownloadUsage} from './downloads.ts';
import {createDownloadRequest,getDownloadRequest,isDownloadRequestTerminal,type DownloadEpisodes,type DownloadRequest,type DownloadRequestKind} from './download-requests.ts';

export type DownloadsApi={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};
/** Items for one preparation batch. A whole container (show, season, album, book, playlist) is
 * never a batch target: it goes through `requestContainer`, the durable container request. */
export type DownloadTarget=Readonly<{mediaId?:string;mediaIds?:readonly string[];nextAfterMediaId?:string}>;
/** A whole-container request (show, season, album, book or playlist — never TV). */
export type ContainerDownloadTarget=Readonly<{kind:DownloadRequestKind;id:string;title?:string}>;
export type ContainerDownloadPolicy=Readonly<{episodes:DownloadEpisodes;keepNext?:number}>;
export type TrackedDownloadRequest=Readonly<{request:DownloadRequest;title:string;error?:string}>;
export type DownloadsSnapshot=Readonly<{phase:'idle'|'loading'|'ready'|'error';items:readonly DownloadPreparation[];usage?:DownloadUsage;busy:readonly string[];error?:string;/** The failure behind `error`, for `presentError`/`say` (category, code, retry). */failure?:unknown;unavailable?:boolean;/** The next page cursor (`''` when the list is complete). */nextCursor:string;/** A further page is being read. */loadingMore:boolean;/** Whole-container requests started on this device, newest first. */requests:readonly TrackedDownloadRequest[]}>;

const ACTIVE=new Set(['queued','running']);
/** One full-list page (the server allows up to 200; 100 keeps a poll page small). */
const PAGE_LIMIT=100;
// X-04: the words come from the catalogue; `failure` keeps the error for screens that present it themselves.
const say=(e:unknown,fallback:'downloads.error.load'|'downloads.error.action')=>serviceProblem(e,'downloads',fallback,fallback==='downloads.error.load'?'load':'action');

export class DownloadsService{
 private api:DownloadsApi;private operationId:()=>string;private pollMs:number;
 private state:DownloadsSnapshot=Object.freeze<DownloadsSnapshot>({phase:'idle',items:[],busy:[],nextCursor:'',loadingMore:false,requests:[]});
 private listeners=new Set<()=>void>();private timer?:ReturnType<typeof setTimeout>;private generation=0;private disposed=false;private watching=false;
 /** Whole-container requests followed on this device, by request id. */
 private tracked=new Map<string,TrackedDownloadRequest>();
 /** Requests with a follower running. A list refresh doesn't end a follower: only disposal, the
  * request finishing or a 404 does, so a whole-season download keeps its progress current. */
 private following=new Set<string>();
 constructor(api:DownloadsApi,operationId:()=>string=()=>randomId(),pollMs=5000){this.api=api;this.operationId=operationId;this.pollMs=pollMs;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<DownloadsSnapshot>){if(this.disposed)return;this.state=Object.freeze({...this.state,...patch});this.listeners.forEach(f=>f());}
 dispose(){this.disposed=true;this.generation++;clearTimeout(this.timer);this.listeners.clear();}

 /** Starts following the list. Safe to call again; `stop` ends the polling without forgetting. */
 start(){this.watching=true;void this.refresh();void this.refreshRequests();}
 stop(){this.watching=false;clearTimeout(this.timer);}
 private pagePath(cursor:string,state?:string){return '/v1/downloads/preparations?limit='+PAGE_LIMIT+(cursor?'&cursor='+encodeURIComponent(cursor):'')+(state?'&state='+encodeURIComponent(state):'');}
 /** PERF-S12: the UI pages the list — one page per call, newest first — instead of re-reading up
  * to 10 × 100 every 5 s. Every page is reachable; nothing is silently truncated. */
 async refresh():Promise<void>{
  const generation=++this.generation;clearTimeout(this.timer);
  if(this.state.phase==='idle')this.publish({phase:'loading'});
  try{
   const result=parseDownloadPage(await this.api.request<unknown>(this.pagePath('')));
   if(generation!==this.generation)return;
   this.publish({phase:'ready',items:Object.freeze(result.items.filter(p=>p.state!=='cancelled'||p.actions.length>0)),nextCursor:result.nextCursor,loadingMore:false,error:undefined,failure:undefined,unavailable:false});
    void this.api.request<unknown>('/v1/downloads/usage').then(raw=>{try{if(generation===this.generation)this.publish({usage:parseDownloadUsage(raw)});}catch{}},()=>{});
  }catch(e){
   if(generation!==this.generation)return;
   const status=(e as {status?:number}|null)?.status;
   // 404/503 here means this server does not offer downloads at all, which is a fact to show
   // plainly, not an error to retry.
   if(status===404||status===503&&!this.state.items.length)this.publish({phase:'ready',unavailable:true,error:undefined,failure:undefined});
   else this.publish({phase:this.state.items.length?'ready':'error',error:say(e,'downloads.error.load'),failure:e});
  }
  this.schedule(generation);
 }
 /** Reads the next page and appends it (deduplicated by id). A no-op without a cursor. */
 async loadMore():Promise<void>{
  const cursor=this.state.nextCursor;
  if(!cursor||this.state.loadingMore||this.disposed)return;
  const generation=this.generation;
  this.publish({loadingMore:true});
  try{
   const result=parseDownloadPage(await this.api.request<unknown>(this.pagePath(cursor)));
   if(generation!==this.generation||this.disposed){this.publish({loadingMore:false});return;}
   const seen=new Set(this.state.items.map(p=>p.id));
   const items=[...this.state.items,...result.items.filter(p=>(p.state!=='cancelled'||p.actions.length>0)&&!seen.has(p.id))];
   this.publish({items:Object.freeze(items),nextCursor:result.nextCursor,loadingMore:false});
  }catch(e){
   if(generation!==this.generation||this.disposed)return;
   this.publish({loadingMore:false,error:say(e,'downloads.error.load'),failure:e});
  }
 }
 private schedule(generation:number){
  if(this.watching&&generation===this.generation&&!this.disposed&&(this.state.phase==='error'||this.state.items.some(p=>ACTIVE.has(p.state))))this.timer=setTimeout(()=>void this.poll(),this.state.phase==='error'?this.pollMs*3:this.pollMs);
 }
 /** PERF-S12: the 5 s poll reads only active preparations — one page each of `queued` and
  * `running` (the server caps live preparations at 200, so two pages cover them) — never the
  * whole list. Items that left the active set are re-read once by id, so a finished preparation
  * settles without a full reload. Stops when nothing is active. */
 private async poll():Promise<void>{
  const generation=this.generation;clearTimeout(this.timer);
  try{
   const [queued,running]=await Promise.all([
    parseDownloadPage(await this.api.request<unknown>(this.pagePath('', 'queued'))),
    parseDownloadPage(await this.api.request<unknown>(this.pagePath('', 'running'))),
   ]);
   if(generation!==this.generation||this.disposed)return;
   const active=[...queued.items,...running.items];
   const seen=new Set(active.map(p=>p.id));
   const byId=new Map(this.state.items.map(p=>[p.id,p]));
   for(const p of active)byId.set(p.id,p);
   // Anything that was active and is gone from both polls has transitioned: re-read it once.
   for(const p of this.state.items){
    if(!ACTIVE.has(p.state)||seen.has(p.id))continue;
    try{
     const current=parseDownloadPreparation(await this.api.request<unknown>('/v1/downloads/preparations/'+encodeURIComponent(p.id)));
     byId.set(current.id,current);
    }catch(e){
     // Gone server-side (removed elsewhere): drop it rather than polling it forever. Anything
     // else is retried on the next poll (or a manual refresh picks it up).
     if((e as {status?:number}|null)?.status===404)byId.delete(p.id);
    }
   }
   // Page order first, then actives from beyond the loaded pages.
   const paged:DownloadPreparation[]=[];
   for(const p of this.state.items){const current=byId.get(p.id);if(current)paged.push(current);}
   for(const p of active)if(!paged.some(q=>q.id===p.id))paged.push(p);
   this.publish({phase:'ready',items:Object.freeze(paged.filter(p=>p.state!=='cancelled'||p.actions.length>0)),error:undefined,failure:undefined});
  }catch(e){
   if(generation!==this.generation||this.disposed)return;
   this.publish({phase:this.state.items.length?'ready':'error',error:say(e,'downloads.error.load'),failure:e});
  }
  this.schedule(generation);
 }
 /** What may be downloaded for one title, with sizes; the server decides, this shows it. */
 async options(itemId:string,signal?:AbortSignal):Promise<DownloadOptionsView>{
  return parseDownloadOptions(await this.api.request<unknown>('/v1/items/'+encodeURIComponent(itemId)+'/download-options','GET',undefined,signal),itemId);
 }
 /** Asks the server to prepare one title, several, a whole season or album, or the next one.
  * A batch is partial by design: what could not be accepted is reported, the rest proceeds. */
 async request(target:DownloadTarget,quality:string):Promise<DownloadBatch>{
  // Only the item fields are sent, whatever else the caller's object carries (containers are durable requests).
  const body={operationId:this.operationId(),...(target.mediaId?{mediaId:target.mediaId}:{}),...(target.mediaIds?{mediaIds:[...target.mediaIds]}:{}),...(target.nextAfterMediaId?{nextAfterMediaId:target.nextAfterMediaId}:{}),quality};
  const batch=parseDownloadBatch(await this.api.request<unknown>('/v1/downloads/preparations','POST',body));
  void this.refresh();return batch;
 }
 /**
  * Starts one whole-container download (show, season, album, book or playlist)
  * and follows its request until it completes or fails. The caller keeps its
  * own operationId per intent for safe retries. Capture shows as an
  * indeterminate state until `totalKnown` is true; a failed capture reports
  * `selection_changed` without effects.
  */
 async requestContainer(input:Readonly<{target:ContainerDownloadTarget;deviceId:string;quality:string;policy:ContainerDownloadPolicy;operationId?:string}>):Promise<DownloadRequest>{
  if(!input.target||input.target.kind!=='show'&&input.target.kind!=='season'&&input.target.kind!=='album'&&input.target.kind!=='book'&&input.target.kind!=='playlist'||!input.target.id)throw new Error('Whole downloads cover shows, seasons, albums, books and playlists.');
  const title=input.target.title??'';
  const request=await createDownloadRequest(this.api,{operationId:input.operationId??this.operationId(),target:{kind:input.target.kind,id:input.target.id},deviceId:input.deviceId,quality:input.quality,policy:{...input.policy}});
  this.tracked.set(request.requestId,Object.freeze({request,title}));
  this.publish({requests:Object.freeze([...this.tracked.values()])});
  void this.followRequest(request.requestId);
  return request;
 }
 /** Re-reads every followed container request (bounded: at most 20). */
 async refreshRequests():Promise<void>{
  const generation=this.generation;
  const ids=[...this.tracked.keys()].slice(0,20);
  await Promise.all(ids.map(async id=>{
   try{
    const request=await getDownloadRequest(this.api,id);
    if(generation!==this.generation||this.disposed)return;
    const prior=this.tracked.get(id);
    if(prior)this.tracked.set(id,Object.freeze({...prior,request,error:undefined}));
   }catch(e){
    if(generation!==this.generation||this.disposed)return;
    const prior=this.tracked.get(id);
    if(prior&&(e as {status?:number}|null)?.status!==404)this.tracked.set(id,Object.freeze({...prior,error:say(e,'downloads.error.load')}));
    else this.tracked.delete(id);
   }
  }));
  if(generation===this.generation&&!this.disposed)this.publish({requests:Object.freeze([...this.tracked.values()])});
  for(const [id,tracked] of this.tracked)if(!isDownloadRequestTerminal(tracked.request.state))void this.followRequest(id);
 }
 private async followRequest(requestId:string):Promise<void>{
  if(this.following.has(requestId))return;
  this.following.add(requestId);
  try{
   for(;;){
    await new Promise(resolve=>setTimeout(resolve,this.pollMs));
    if(this.disposed)return;
    const prior=this.tracked.get(requestId);
    if(!prior||isDownloadRequestTerminal(prior.request.state)){void this.refresh();return;}
    try{
     const request=await getDownloadRequest(this.api,requestId);
     if(this.disposed)return;
     const current=this.tracked.get(requestId);
     if(!current)return;
     this.tracked.set(requestId,Object.freeze({...current,request,error:undefined}));
     this.publish({requests:Object.freeze([...this.tracked.values()])});
     if(isDownloadRequestTerminal(request.state)){void this.refresh();return;}
    }catch(e){
     if(this.disposed)return;
     if((e as {status?:number}|null)?.status===404){this.tracked.delete(requestId);this.publish({requests:Object.freeze([...this.tracked.values()])});return;}
    }
   }
  }finally{this.following.delete(requestId);}
 }
 async act(preparation:DownloadPreparation,action:DownloadAction):Promise<void>{
  if(this.state.busy.includes(preparation.id))return;
  this.publish({busy:[...this.state.busy,preparation.id],error:undefined,failure:undefined});
  try{
   const raw=await this.api.request<unknown>('/v1/downloads/preparations/'+encodeURIComponent(preparation.id)+'/actions','POST',{operationId:this.operationId(),action,expectedRevision:preparation.revision});
   const next=parseDownloadPreparation(raw);
   this.publish({items:action==='remove'?this.state.items.filter(p=>p.id!==next.id):this.state.items.map(p=>p.id===next.id?next:p)});
  }catch(e){
   // A conflict carries the current row: show it rather than the stale one the action was made on.
   const current=(e as {preparation?:unknown}|null)?.preparation;
   if(current){try{const next=parseDownloadPreparation(current);this.publish({items:this.state.items.map(p=>p.id===next.id?next:p)});}catch{}}
   this.publish({error:say(e,'downloads.error.action'),failure:e});
  }finally{this.publish({busy:this.state.busy.filter(id=>id!==preparation.id)});void this.refresh();}
 }
 /** A short-lived address for the prepared file. Ask for it at the moment of download. */
 async grant(preparation:DownloadPreparation):Promise<DownloadGrant>{
  return parseDownloadGrant(await this.api.request<unknown>('/v1/downloads/preparations/'+encodeURIComponent(preparation.id)+'/grant','POST',{operationId:this.operationId()}));
 }
 reason(preparation:DownloadPreparation):string{return preparation.reason?downloadReasonMessage(preparation.reason):'';}
}
