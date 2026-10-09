import {randomId} from './random-id.ts';
/** The viewer's notification inbox and the feedback dialog, as framework-free services shared
 * by the web and Apple clients.
 *
 * Both follow the forgiving-client rule: a failed request never takes what is already on
 * screen away. A refresh that fails keeps the loaded messages and records the error beside
 * them; only a first load with nothing to show is an error state. State changes are optimistic
 * and are put back if the server refuses them. */
import {
 mergeNotifications,parseFeedbackCapabilities,parseFeedbackSubmission,parseNotificationBatchResult,parseNotificationInbox,parseNotificationSummary,
 type FeedbackCapabilities,type FeedbackSubmissionResult,type Notification,type NotificationAudience,type NotificationCounts,type NotificationDelta,
} from './notifications.ts';
import {serviceProblem,serviceText} from './presentation/service-text.ts';
import type {MessageId} from '../../i18n/src/index.ts';

export type InboxApi={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};
export type InboxView='unread'|'all'|'archived';
export type InboxAction='read'|'unread'|'archive'|'unarchive';
export type InboxServiceOptions={unreadTimeoutMs?:number;setTimer?:(fn:()=>void,ms:number)=>unknown;clearTimer?:(timer:unknown)=>void};
export type InboxSnapshot=Readonly<{
 phase:'idle'|'loading'|'ready'|'error';
 audience:NotificationAudience;
 /** Audiences this viewer may read; an owner also has 'account-admin'. */
 audiences:readonly NotificationAudience[];
 view:InboxView;
 items:readonly Notification[];
 counts:NotificationCounts;
 /** Unread across every audience the viewer has, for the badge. */
 unread:number;
 revision:number;
 more:boolean;
 busy:boolean;
 error?:string;
}>;

const noCounts:NotificationCounts=Object.freeze({unread:0,read:0,archived:0,total:0});
const data=(raw:unknown):unknown=>raw&&typeof raw==='object'&&'data' in (raw as Record<string,unknown>)?(raw as Record<string,unknown>).data:raw;
// A transport failure ("Failed to fetch") is not something to show a person; the server's own
// words are.
// X-04: what the viewer reads comes from the catalogue, never from `error.message`.
const message=(e:unknown,fallback:MessageId,context:'notifications'|'feedback'='notifications',operation:'load'|'save'|'action'='load')=>serviceProblem(e,context,fallback,operation);

export class InboxService{
 private api:InboxApi;private operationId:()=>string;
 private state:InboxSnapshot=Object.freeze<InboxSnapshot>({phase:'idle',audience:'profile',audiences:['profile'],view:'unread',items:[],counts:noCounts,unread:0,revision:0,more:false,busy:false});
 private listeners=new Set<()=>void>();private cursor='';private generation=0;private controller?:AbortController;private disposed=false;
 private unreadFlight?:Promise<number>;private unreadController?:AbortController;private unreadDirty=false;private options:InboxServiceOptions;
 constructor(api:InboxApi,operationId:()=>string=()=>randomId(),options:InboxServiceOptions={}){this.api=api;this.operationId=operationId;this.options=options;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<InboxSnapshot>){if(this.disposed)return;this.state=Object.freeze({...this.state,...patch});this.listeners.forEach(f=>f());}
 dispose(){this.disposed=true;this.generation++;this.controller?.abort();this.unreadController?.abort();this.listeners.clear();}

 /** Loads the first page of a view. What is on screen stays until the new page arrives. */
 async load(audience:NotificationAudience=this.state.audience,view:InboxView=this.state.view):Promise<void>{
  const generation=++this.generation;this.controller?.abort();const controller=this.controller=new AbortController();
  const same=audience===this.state.audience&&view===this.state.view;
  this.publish({phase:this.state.items.length&&same?'ready':'loading',audience,view,error:undefined,...(same?{}:{items:[],more:false})});
  try{
   const inbox=parseNotificationInbox(data(await this.api.request<unknown>(`/v1/notifications/inbox?audience=${encodeURIComponent(audience)}&state=${view}&limit=50`,'GET',undefined,controller.signal)));
   if(generation!==this.generation)return;
   this.cursor=inbox.nextCursor;
   const audiences=(inbox.audiences.filter(a=>a==='profile'||a==='account-admin') as NotificationAudience[]);
   this.publish({phase:'ready',items:inbox.items,counts:inbox.counts,revision:inbox.revision,more:!!inbox.nextCursor,audiences:audiences.length?audiences:['profile']});
   void this.refreshUnread();
  }catch(e){
   if(generation!==this.generation||controller.signal.aborted)return;
   this.publish({phase:this.state.items.length?'ready':'error',error:message(e,'notifications.error.load')});
  }
 }
 async more():Promise<void>{
  if(!this.cursor||this.state.busy)return;const generation=this.generation,{audience,view}=this.state;this.publish({busy:true});
  try{
   const inbox=parseNotificationInbox(data(await this.api.request<unknown>(`/v1/notifications/inbox?audience=${encodeURIComponent(audience)}&state=${view}&limit=50&cursor=${encodeURIComponent(this.cursor)}`)));
   if(generation!==this.generation)return;this.cursor=inbox.nextCursor;
   const held=new Set(this.state.items.map(n=>n.id));
   this.publish({items:[...this.state.items,...inbox.items.filter(n=>!held.has(n.id))],counts:inbox.counts,revision:inbox.revision,more:!!inbox.nextCursor,busy:false,error:undefined});
  }catch(e){if(generation===this.generation)this.publish({busy:false,error:message(e,'notifications.error.more')});}
 }
 /** Applies a change pushed by the event stream or long poll. */
 accept(delta:NotificationDelta){
  if(delta.resync){void this.load();return;}
  if(delta.revision<=this.state.revision)return;
  this.publish({items:this.visible(mergeNotifications(this.state.items,delta)),counts:delta.counts,revision:delta.revision});
  this.unreadDirty=true;void this.refreshUnread();
 }
 private visible(items:readonly Notification[]):Notification[]{
  const view=this.state.view;
  return items.filter(n=>view==='archived'?n.archived:view==='unread'?!n.archived&&!n.read:!n.archived);
 }
 /** Optimistic: the row changes at once and is put back if the server refuses. */
 async apply(action:InboxAction,ids:readonly string[]):Promise<void>{
  if(!ids.length)return;const before=this.state,chosen=new Set(ids),now=Date.now();
  const next=before.items.map(n=>!chosen.has(n.id)?n:action==='read'?{...n,read:true,readAt:n.readAt??now}:action==='unread'?{...n,read:false,readAt:null}:action==='archive'?{...n,archived:true,archivedAt:now}:{...n,archived:false,archivedAt:null});
  this.publish({items:this.visible(next),error:undefined});
  await this.send([{action,ids:[...ids]}],before);
 }
 async readAll():Promise<void>{
  const before=this.state,now=Date.now();
  this.publish({items:this.visible(before.items.map(n=>n.read?n:{...n,read:true,readAt:now})),error:undefined});
  await this.send([{action:'read-all'}],before);
 }
 private async send(operations:{action:string;ids?:string[]}[],before:InboxSnapshot){
  const generation=this.generation;
  try{
   const result=parseNotificationBatchResult(data(await this.api.request<unknown>('/v1/notifications/inbox/actions','POST',{operationId:this.operationId(),expectedRevision:0,audience:before.audience,operations})));
   if(generation!==this.generation)return;
   this.publish({counts:result.counts,revision:result.revision});this.unreadDirty=true;void this.refreshUnread();
  }catch(e){
   if(generation!==this.generation)return;
   this.publish({items:before.items,counts:before.counts,error:message(e,'notifications.error.change','notifications','save')});
  }
 }
 /** The badge. Failure leaves the last known number in place. */
 refreshUnread():Promise<number>{
  if(this.disposed)return Promise.resolve(this.state.unread);
  if(this.unreadFlight)return this.unreadFlight;
  this.unreadFlight=this.readUnread().finally(()=>{this.unreadFlight=undefined;});
  return this.unreadFlight;
 }
 private async readUnread():Promise<number>{
  do{
   this.unreadDirty=false;
   const audiences=[...this.state.audiences],revision=this.state.revision;
   const controller=this.unreadController=new AbortController();
   const setTimer=this.options.setTimer??((fn:()=>void,ms:number)=>setTimeout(fn,ms));
   const clearTimer=this.options.clearTimer??((timer:unknown)=>clearTimeout(timer as ReturnType<typeof setTimeout>));
   // This is a small badge read, not the event long poll or a playback request. Bound only
   // this refresh so an unresponsive transport cannot park every subsequent badge update.
   const timer=setTimer(()=>controller.abort(),this.options.unreadTimeoutMs??10_000);
   let abort:()=>void=()=>{};
   const cancelled=new Promise<never>((_,reject)=>{abort=()=>reject(new Error('unread_refresh_aborted'));controller.signal.addEventListener('abort',abort,{once:true});});
   try{
    let total=0;
    for(const audience of audiences){
     const raw=await Promise.race([this.api.request<unknown>('/v1/notifications/unread-count?audience='+encodeURIComponent(audience),'GET',undefined,controller.signal),cancelled]);
     if(controller.signal.aborted||this.disposed)return this.state.unread;
     total+=parseNotificationSummary(data(raw)).counts.unread;
    }
    // An inbox load can discover the owner audience while the profile read is pending.
    // Never publish a partial/obsolete badge; coalesce changes into one fresh round.
    if(audiences.join(',')!==this.state.audiences.join(',')||revision!==this.state.revision)this.unreadDirty=true;
    if(!this.unreadDirty&&total!==this.state.unread)this.publish({unread:total});
   }catch{
    if(controller.signal.aborted||this.disposed)return this.state.unread;
    if(audiences.join(',')!==this.state.audiences.join(',')||revision!==this.state.revision)this.unreadDirty=true;
    if(!this.unreadDirty)return this.state.unread;
   }
   finally{clearTimer(timer);controller.signal.removeEventListener('abort',abort);if(this.unreadController===controller)this.unreadController=undefined;}
  }while(this.unreadDirty&&!this.disposed);
  return this.state.unread;
 }

}

export type FeedbackDraft={kind:string;category:string;message:string;itemId?:string;playbackSessionId?:string;attachDiagnostics:boolean};
export type FeedbackSnapshot=Readonly<{
 phase:'loading'|'ready'|'unavailable'|'sending'|'sent';
 capabilities?:FeedbackCapabilities;
 result?:FeedbackSubmissionResult;
 error?:string;
}>;

/** One feedback dialog's lifetime. The message a person typed is never lost to a failed send:
 * the dialog returns to 'ready' with the error shown and the draft intact (the caller owns it). */
export class FeedbackService{
 private api:InboxApi;private operationId:()=>string;private attempt='';
 private state:FeedbackSnapshot=Object.freeze({phase:'loading'});private listeners=new Set<()=>void>();
 constructor(api:InboxApi,operationId:()=>string=()=>randomId()){this.api=api;this.operationId=operationId;}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(next:FeedbackSnapshot){this.state=Object.freeze(next);this.listeners.forEach(f=>f());}
 async open():Promise<void>{
  this.publish({phase:'loading'});
  try{
   const capabilities=parseFeedbackCapabilities(data(await this.api.request<unknown>('/v1/feedback/capabilities')));
   this.publish(capabilities.canSubmit?{phase:'ready',capabilities}:{phase:'unavailable',capabilities,error:serviceText('feedback.off')});
  }catch(e){this.publish({phase:'unavailable',error:message(e,'feedback.unavailable','feedback')});}
 }
 /** Why a draft cannot be sent yet, or '' when it can. */
 problem(draft:FeedbackDraft):string{
  const c=this.state.capabilities;if(!c)return serviceText('feedback.unavailable');
  const kind=c.kinds.find(k=>k.id===draft.kind);if(!kind)return serviceText('feedback.chooseKind');
  if(!kind.categories.some(x=>x.id===draft.category))return serviceText('feedback.chooseCategory');
  // CD-46: the capability's min/max count Unicode code points (as users count), not UTF-16
  // units or bytes. The trimmed text measured here is exactly what submit() sends.
  const length=Array.from(draft.message.trim()).length;
  if(length<c.minMessageLength)return serviceText('feedback.tooShort',{count:c.minMessageLength});
  if(length>c.maxMessageLength)return serviceText('feedback.tooLong',{count:c.maxMessageLength});
  return '';
 }
 async submit(draft:FeedbackDraft):Promise<boolean>{
  const capabilities=this.state.capabilities;if(!capabilities||this.state.phase==='sending'||this.problem(draft))return false;
  // One operation id per draft: a retry after a lost response is the same report, not a second.
  this.attempt||=this.operationId();
  this.publish({phase:'sending',capabilities});
  try{
   const category=capabilities.kinds.find(k=>k.id===draft.kind)?.categories.find(c=>c.id===draft.category);
   const result=parseFeedbackSubmission(data(await this.api.request<unknown>('/v1/feedback/reports','POST',{
    operationId:this.attempt,kind:draft.kind,category:draft.category,message:draft.message.trim(),
    itemId:category?.wantsItem?draft.itemId??'':'',playbackSessionId:category?.wantsPlaybackSession?draft.playbackSessionId??'':'',
    attachDiagnostics:capabilities.diagnosticsSupported&&draft.attachDiagnostics,
   })));
   this.attempt='';this.publish({phase:'sent',capabilities,result});return true;
  }catch(e){this.publish({phase:'ready',capabilities,error:message(e,'feedback.sendFailed','feedback','save')});return false;}
 }
}
