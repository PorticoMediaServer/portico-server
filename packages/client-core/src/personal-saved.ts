import {unreadableServerResponse} from './server-messages.ts';
/** Online personal resources. The server owns membership, query evaluation and permissions.
 * Existing library navigation owns destinations/Back; this module owns no queue or device store. */
import {validateContentEntry,type ContentEntry} from './library-content.ts';
import {parseBrowseNode,parseBrowseResult,type BrowseNode,type BrowseResult,type BrowseSortSelection} from './browse.ts';
import type {SavedApi,SavedScope,SavedRoute,SavedActor} from './saved.ts';
export type PersonalResourceKind='collection'|'view';
/** A saved view stores the whole browse request: the pivot, the expression tree
 * and the sort. The server validates it against the published capabilities; this
 * parser only refuses what it could never have produced. */
export type SavedDefinition=Readonly<{libraryId:string;pivot:string;query:BrowseNode|null;sort:readonly BrowseSortSelection[];presentation:'grid'|'list'}>;
export type PersonalVisibility='private'|'server';
export type PersonalResource=Readonly<{serverId:string;viewerFence:string;id:string;kind:PersonalResourceKind;name:string;summary:string;visibility:PersonalVisibility;revision:number;role:'owner'|'editor'|'viewer';entryCount:number;actions:readonly string[];definition?:SavedDefinition;status:'ready'|'needs-review';invalidComponents:readonly string[];pinned:boolean;pinRevision:number;shares:readonly Readonly<SavedActor&{role:'viewer'|'editor';displayName:string}>[]}>;
export type PersonalResourceEntry=Readonly<{id:string;hidden:boolean;media?:ContentEntry;updatedAt?:string;positionSeconds?:number;completed?:boolean}>;
export type PersonalSavedIntent=Readonly<
 {action:'create';kind:PersonalResourceKind;name:string;summary?:string;visibility?:PersonalVisibility;definition?:SavedDefinition}|
 {action:'update';name?:string;summary?:string;visibility?:PersonalVisibility;definition?:SavedDefinition}|
 {action:'entries';addItemIds?:readonly string[];removeEntryIds?:readonly string[]}|
 {action:'delete'}|{action:'pin';pinned:boolean}|
 {action:'share';actor:SavedActor;role:'viewer'|'editor'}|{action:'unshare';actor:SavedActor}|
 {action:'clear-history'|'reset-viewing-activity'}>;
export type PersonalSavedError=Readonly<{code:string;message:string;conflict:boolean}>;
export type PersonalSavedSnapshot=Readonly<{route:SavedRoute|null;loading:boolean;error:PersonalSavedError|null;mutationError:PersonalSavedError|null;pending:boolean;retryPending:boolean;resources:readonly PersonalResource[];resource:PersonalResource|null;entries:readonly PersonalResourceEntry[];browse:BrowseResult|null;revision:number;viewerFence:string;cursor:string|null;history:readonly (string|null)[];nextCursor:string;result:Readonly<{resourceId?:string;deleted:boolean;entries?:EntryOutcomes}>|null;candidates:readonly Readonly<SavedActor&{id:string;displayName:string}>[];candidateCursor:string;libraries:readonly Readonly<{id:string;name:string;kind:string}>[]}>;
const obj=(v:unknown):v is Record<string,unknown>=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const str=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>str(v,256)&&v.length>0&&!/[\r\n]/.test(v);
const num=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
function bad():never{throw Object.assign(new Error(unreadableServerResponse),{code:'invalid_saved'});}
function rows(v:unknown,max=100):unknown[]{if(!Array.isArray(v)||v.length>max)bad();return v;}
function distinct<T extends {id:string}>(a:T[]):T[]{if(new Set(a.map(v=>v.id)).size!==a.length)bad();return a;}
function actor(v:unknown):SavedActor{if(!obj(v)||!['local','hosted'].includes(v.authority as string)||!id(v.accountId)||!id(v.profileId))bad();return {authority:v.authority as SavedActor['authority'],accountId:v.accountId,profileId:v.profileId};}
function definition(v:unknown):SavedDefinition{
 if(!obj(v)||!id(v.libraryId)||!id(v.pivot)||!['grid','list'].includes(v.presentation as string))bad();
 const sort=rows(v.sort,3).map(s=>{if(!obj(s)||!id(s.field)||!['asc','desc'].includes(s.direction as string))bad();return {field:s.field,direction:s.direction as 'asc'|'desc'};});
 if(sort.length<1)bad();
 let query:BrowseNode|null=null;
 if(v.query!==undefined&&v.query!==null){try{query=parseBrowseNode(v.query);}catch{bad();}}
 return {libraryId:v.libraryId,pivot:v.pivot,query,sort,presentation:v.presentation as 'grid'|'list'};
}
function resource(v:unknown,scope:SavedScope):PersonalResource{
 if(!obj(v)||v.serverId!==scope.serverId||!id(v.viewerFence)||!id(v.id)||!['collection','view'].includes(v.kind as string)||!str(v.name,400)||!v.name.trim()||!str(v.summary,8000)||!['private','server'].includes(v.visibility as string)||!num(v.revision)||!num(v.entryCount)||!['owner','editor','viewer'].includes(v.role as string)||!['ready','needs-review'].includes(v.status as string)||typeof v.pinned!=='boolean'||!num(v.pinRevision))bad();
 const actions=rows(v.actions,8).map(a=>{if(typeof a!=='string'||!['pin','update','delete','entries','share'].includes(a))bad();return a;});
 if(v.role==='viewer'&&actions.some(a=>a!=='pin')||v.role==='editor'&&actions.some(a=>a!=='pin'&&a!=='entries'))bad();
 const shares=rows(v.shares??[],100).map(s=>{if(!obj(s)||!['viewer','editor'].includes(s.role as string)||!str(s.displayName,512))bad();return {...actor(s),role:s.role as 'viewer'|'editor',displayName:s.displayName};});
 const invalidComponents=rows(v.invalidComponents,32).map(x=>{if(!str(x,256))bad();return x;});
 // Needs-review definitions are deliberately not executed or silently normalized.
 const d=v.kind==='view'&&v.status==='ready'?definition(v.definition):undefined;
 return {serverId:scope.serverId,viewerFence:v.viewerFence,id:v.id,kind:v.kind as PersonalResourceKind,name:v.name,summary:v.summary,visibility:v.visibility as PersonalVisibility,revision:v.revision,role:v.role as PersonalResource['role'],entryCount:v.entryCount,actions,...(d?{definition:d}:{}),status:v.status as PersonalResource['status'],invalidComponents,pinned:v.pinned,pinRevision:v.pinRevision,shares};
}
/** Visibility is a published server policy, not a client-side sharing rule. */
function visibility(v:unknown):PersonalVisibility{if(v!=='private'&&v!=='server')throw new Error('Collection visibility must be private or server.');return v;}
export type EntryOutcomes=Readonly<{added:readonly string[];removed:readonly string[];unchanged:readonly string[];failed:readonly Readonly<{itemId:string;code:string}>[]}>;
/** Membership batches report every item. A row the server refused is an outcome the
 * viewer can see and act on, never a silently dropped change. */
export function parseEntryOutcomes(v:unknown,limit=200):EntryOutcomes{
 if(!obj(v))bad();
 const list=(x:unknown)=>Object.freeze(rows(x,limit).map(entry=>{if(!id(entry))bad();return entry;}));
 const failed=rows(v.failed,limit).map(entry=>{if(!obj(entry)||!id(entry.itemId)||!id(entry.code))bad();return Object.freeze({itemId:entry.itemId,code:entry.code});});
 const outcomes=Object.freeze({added:list(v.added),removed:list(v.removed),unchanged:list(v.unchanged),failed:Object.freeze(failed)});
 const all=[...outcomes.added,...outcomes.removed,...outcomes.unchanged,...outcomes.failed.map(f=>f.itemId)];
 if(new Set(all).size!==all.length)bad();
 return outcomes;
}
export const MAX_PERSONAL_BATCH=200;
/** `notInterested` (recommendation feedback) is the one field a show, album or book id may carry. */
export type PersonalBatchItem=Readonly<{itemId:string;expectedRevision?:number;watched?:boolean;favorite?:boolean;watchlisted?:boolean;notInterested?:boolean}>;
export type PersonalBatchResult=Readonly<{itemId:string;ok:boolean;code:string;message:string;personal:unknown}>;
export type PersonalBatchReceipt=Readonly<{serverId:string;viewerFence:string;operationId:string;results:readonly PersonalBatchResult[];updated:number;failed:number}>;
/** One request per bulk action. The client never fans out a row at a time. */
export function personalBatchBody(operationId:string,items:readonly PersonalBatchItem[]):Readonly<{operationId:string;items:readonly PersonalBatchItem[]}>{
 if(!id(operationId)||!/^[A-Za-z0-9_-]{1,128}$/.test(operationId))throw new Error('A batch needs a single stable operationId.');
 if(!items.length||items.length>MAX_PERSONAL_BATCH)throw new Error('A batch carries 1 to 200 items.');
 const seen=new Set<string>();
 const body=items.map(item=>{
  if(!id(item.itemId)||seen.has(item.itemId))throw new Error('Each batch item must name a distinct itemId.');
  seen.add(item.itemId);
  if(item.watched===undefined&&item.favorite===undefined&&item.watchlisted===undefined&&item.notInterested===undefined)throw new Error('Each batch item must set watched, favorite, watchlisted or notInterested.');
  if(item.expectedRevision!==undefined&&!num(item.expectedRevision))throw new Error('Invalid expectedRevision.');
  return Object.freeze({itemId:item.itemId,...(item.expectedRevision===undefined?{}:{expectedRevision:item.expectedRevision}),...(item.watched===undefined?{}:{watched:item.watched}),...(item.favorite===undefined?{}:{favorite:item.favorite}),...(item.watchlisted===undefined?{}:{watchlisted:item.watchlisted}),...(item.notInterested===undefined?{}:{notInterested:item.notInterested})});
 });
 return Object.freeze({operationId,items:Object.freeze(body)});
}
export function parsePersonalBatchReceipt(v:unknown,serverId:string,requested:readonly string[]):PersonalBatchReceipt{
 if(!obj(v)||v.serverId!==serverId||!id(v.viewerFence)||!id(v.operationId)||!num(v.updated)||!num(v.failed))bad();
 const results=rows(v.results,MAX_PERSONAL_BATCH).map(r=>{
  if(!obj(r)||!id(r.itemId)||typeof r.ok!=='boolean')bad();
  if(r.code!==undefined&&!id(r.code))bad();
  if(r.message!==undefined&&!str(r.message,4096))bad();
  if(r.ok&&r.code)bad();
  if(!r.ok&&!r.code)bad();
  return Object.freeze({itemId:r.itemId,ok:r.ok,code:(r.code as string)??'',message:(r.message as string)??'',personal:r.personal??null});
 });
 // Every item the client sent is answered exactly once, in the order it sent them.
 if(results.length!==requested.length||results.some((r,i)=>r.itemId!==requested[i]))bad();
 if(results.filter(r=>r.ok).length!==v.updated||results.filter(r=>!r.ok).length!==v.failed)bad();
 return Object.freeze({serverId,viewerFence:v.viewerFence,operationId:v.operationId,results:Object.freeze(results),updated:v.updated,failed:v.failed});
}
export const PERSONAL_BATCH_PATH='/v1/items/personal-state:batch';
function info(e:unknown):PersonalSavedError{const x=e as {code?:unknown};const code=id(x?.code)?x.code:'request_failed';return {code,message:e instanceof Error?e.message:'The request failed.',conflict:['playlist_conflict','resource_conflict','personal_state_conflict','stale_continuation','operation_expired'].includes(code)};}
function freeze<T>(v:T):T{if(v&&typeof v==='object'){for(const c of Object.values(v))freeze(c);Object.freeze(v);}return v;}
export function isPersonalSavedRoute(route:SavedRoute|null):boolean{return !!route&&['collections','views','history','resource'].includes(route.view);}
/** The definition names an existing authorized library and canonical browse semantics. */
export function makeSavedDefinition(libraryId:string,titlePrefix='',presentation:'grid'|'list'='grid',sort:string='title',direction:'asc'|'desc'='asc',categoryId='',pivot='movies'):SavedDefinition{
 const predicates:BrowseNode[]=[];
 if(titlePrefix.trim())predicates.push({field:'title',operator:'starts-with',value:titlePrefix});
 if(categoryId){
  const [field,value]=categoryId.split(':');
  if(!field||!value)bad();
  predicates.push(field==='decade'||field==='year'?{field,operator:'equals',value:Number(value)}:{field,operator:'contains',value});
 }
 const query=predicates.length===0?null:predicates.length===1?predicates[0]:{all:predicates};
 return definition({libraryId,pivot,query,sort:[{field:sort,direction}],presentation});
}
/** Reads back the two flat controls the simple saved-view editor writes. A saved
 * view can hold a far richer expression; these only recognise what that editor
 * produced and return '' for anything else, rather than lossily flattening it. */
function topLevel(d?:SavedDefinition):readonly BrowseNode[]{
 if(!d?.query)return [];
 return 'all' in d.query?d.query.all:[d.query];
}
export function savedTitlePrefix(d?:SavedDefinition):string{
 for(const node of topLevel(d))if('field' in node&&node.field==='title'&&node.operator==='starts-with'&&typeof node.value==='string')return node.value;
 return '';
}
export function savedCategoryId(d?:SavedDefinition):string{
 for(const node of topLevel(d)){
  if(!('field' in node)||node.field==='title')continue;
  if((node.operator==='equals'||node.operator==='contains')&&(typeof node.value==='string'||typeof node.value==='number'))return node.field+':'+String(node.value);
 }
 return '';
}
export class PersonalSavedService{
 private state:PersonalSavedSnapshot;private listeners=new Set<()=>void>();private generation=0;private readTicket=0;private candidateTicket=0;private disposed=false;private controllers=new Set<AbortController>();
 private command?:{intent:PersonalSavedIntent;path:string;method:string;body:Record<string,unknown>;fence:string};
 private api:SavedApi;readonly scope:SavedScope;private requestId:()=>Promise<string>;
 constructor(api:SavedApi,scope:SavedScope,requestId:()=>Promise<string>){this.api=api;this.scope=Object.freeze({...scope});this.requestId=requestId;if(!id(scope.serverId)||!str(scope.viewerId,1024)||!scope.viewerId)throw new Error('A complete viewer scope is required.');this.state=this.empty();}
 private empty():PersonalSavedSnapshot{return freeze({route:null,loading:false,error:null,mutationError:null,pending:false,retryPending:false,resources:[],resource:null,entries:[],browse:null,revision:0,viewerFence:'',cursor:null,history:[],nextCursor:'',result:null,candidates:[],candidateCursor:'',libraries:[]});}
 getSnapshot=()=>this.state;
 subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>{this.listeners.delete(fn);};};
 private publish(patch:Partial<PersonalSavedSnapshot>){if(this.disposed)return;this.state=freeze({...this.state,...patch});for(const fn of this.listeners)fn();}
 private alive(g:number){return !this.disposed&&g===this.generation;}
 private async bounded<T>(operation:(signal:AbortSignal)=>Promise<T>):Promise<T>{
  const c=new AbortController();this.controllers.add(c);let timer:ReturnType<typeof setTimeout>|undefined;let abort:(()=>void)|undefined;
  try{return await Promise.race([operation(c.signal),new Promise<never>((_,reject)=>{abort=()=>reject(Object.assign(new Error('The request was cancelled.'),{code:'cancelled'}));c.signal.addEventListener('abort',abort,{once:true});timer=setTimeout(()=>{reject(Object.assign(new Error('The server took too long. Retry the same operation to reconcile it.'),{code:'timeout'}));c.abort();},15000);})]);}
  finally{if(timer)clearTimeout(timer);if(abort)c.signal.removeEventListener('abort',abort);this.controllers.delete(c);}
 }
 private request<T>(path:string,method='GET',body?:unknown):Promise<T>{return this.bounded(signal=>this.api.request<T>(path,method,body,signal));}
 select(route:SavedRoute,position:{cursor?:string|null;history?:readonly (string|null)[]}={}):Promise<void>{
  if(this.disposed)throw new Error('Saved service is disposed.');if(!isPersonalSavedRoute(route)||route.view==='resource'&&!id(route.resourceId))throw new Error('Invalid personal resource destination.');
  if((position.history?.length??0)>64||position.cursor!=null&&!str(position.cursor,4096)||position.history?.some(c=>c!==null&&!str(c,4096)))throw new Error('Invalid Saved position.');
  this.generation++;for(const c of this.controllers)c.abort();this.command=undefined;this.publish({...this.empty(),route:freeze({...route}),cursor:position.cursor??null,history:[...(position.history??[])]});return this.load(position.cursor??null,position.history??[]);
 }
 refresh=()=>this.load(null,[]);
 next=()=>this.state.nextCursor&&this.state.history.length<64?this.load(this.state.nextCursor,[...this.state.history,this.state.cursor]):Promise.resolve();
 previous=()=>this.state.history.length?this.load(this.state.history.at(-1)!,this.state.history.slice(0,-1)):Promise.resolve();
 private async load(cursor:string|null,history:readonly (string|null)[]):Promise<void>{
  const route=this.state.route,g=this.generation,ticket=++this.readTicket;if(!route||this.disposed)return;this.publish({loading:true,error:null});
  try{
   const query=new URLSearchParams({limit:'40',...(cursor?{cursor}:{})});
   const path=route.view==='resource'?'/v1/saved-resources/'+encodeURIComponent(route.resourceId!)+'/content':route.view==='history'?'/v1/personal-history':'/v1/saved-resources';
   if(route.view==='collections'||route.view==='views')query.set('kind',route.view==='collections'?'collection':'view');
   if(route.view==='history'&&'period'in route&&route.period)query.set('period',route.period);
   const raw=await this.request<unknown>(path+'?'+query);if(!this.alive(g)||ticket!==this.readTicket)return;
   if(!obj(raw)||!str(raw.nextCursor,4096))bad();let patch:{-readonly [K in keyof PersonalSavedSnapshot]?:PersonalSavedSnapshot[K]}={resource:null,resources:[],entries:[],browse:null,nextCursor:raw.nextCursor,cursor,history:[...history]};
   if(route.view==='resource'){
    const r=resource(raw.resource,this.scope);if(r.id!==route.resourceId)bad();patch.resource=r;patch.viewerFence=r.viewerFence;patch.revision=r.revision;
    if(r.kind==='view'&&r.status==='ready'){
     // The saved query runs on the server's browse engine; the client re-parses
     // the page it returns rather than re-deriving membership from the query.
     const page=parseBrowseResult(raw.browse);
     if(page.pivot!==r.definition!.pivot)bad();
     patch.browse=page;patch.entries=page.entries.map(media=>({id:media.id,hidden:false,media}));patch.nextCursor=page.pageInfo.nextCursor;
    }else{patch.entries=distinct(rows(raw.entries).map(e=>{if(!obj(e)||!id(e.id)||typeof e.hidden!=='boolean'||e.hidden&&Object.keys(e).some(k=>k!=='id'&&k!=='hidden'))bad();return {id:e.id,hidden:e.hidden,...(!e.hidden?{media:validateContentEntry(e.media)}:{})};}));}
   }else{
    if(raw.serverId!==this.scope.serverId||!id(raw.viewerFence)||!num(raw.revision))bad();patch.viewerFence=raw.viewerFence;patch.revision=raw.revision;
    if(route.view==='history')patch.entries=distinct(rows(raw.entries).map(e=>{if(!obj(e)||!id(e.id)||!str(e.updatedAt,128)||typeof e.positionSeconds!=='number'||!Number.isFinite(e.positionSeconds)||e.positionSeconds<0||typeof e.completed!=='boolean')bad();return {id:e.id,hidden:false,media:validateContentEntry(e.media),updatedAt:e.updatedAt,positionSeconds:e.positionSeconds,completed:e.completed};}));
    else patch.resources=distinct(rows(raw.resources).map(v=>{const r=resource(v,this.scope);if(r.viewerFence!==raw.viewerFence||r.kind!==(route.view==='collections'?'collection':'view'))bad();return r;}));
   }
   if(this.state.viewerFence&&this.state.viewerFence!==patch.viewerFence)throw Object.assign(new Error('Your access changed. Reopen Saved to continue.'),{code:'scope_changed'});
   this.publish({...patch,loading:false});
  }catch(e){if(!this.alive(g)||ticket!==this.readTicket)return;const error=info(e);this.publish({loading:false,error,...(['unauthorized','forbidden','not_found','scope_changed','invalid_saved'].includes(error.code)?{resources:[],resource:null,entries:[],browse:null,viewerFence:''}:{})});}
 }
 async loadLibraries():Promise<void>{const g=this.generation;try{const raw=await this.request<unknown>('/v1/libraries');if(!this.alive(g))return;if(!obj(raw))bad();const libraries=distinct(rows(raw.items,2048).map(v=>{if(!obj(v)||!id(v.id)||!str(v.name,512)||!id(v.kind))bad();return{id:v.id,name:v.name,kind:v.kind};}));this.publish({libraries});}catch(e){if(this.alive(g))this.publish({error:info(e)});}}
 async loadCandidates(cursor=''):Promise<void>{const r=this.state.resource,g=this.generation,ticket=++this.candidateTicket;if(!r?.actions.includes('share'))return;
  try{const raw=await this.request<unknown>('/v1/saved-resources/'+encodeURIComponent(r.id)+'/share-candidates?'+new URLSearchParams({limit:'40',...(cursor?{cursor}:{})}));if(!this.alive(g)||ticket!==this.candidateTicket)return;
   if(!obj(raw)||raw.serverId!==this.scope.serverId||raw.viewerFence!==r.viewerFence||raw.resourceId!==r.id||raw.revision!==r.revision||!str(raw.nextCursor,4096))bad();const candidates=distinct(rows(raw.candidates).map(c=>{if(!obj(c)||!id(c.id)||!str(c.displayName,512))bad();return {...actor(c),id:c.id,displayName:c.displayName};}));this.publish({candidates,candidateCursor:raw.nextCursor});
  }catch(e){if(this.alive(g)&&ticket===this.candidateTicket)this.publish({error:info(e)});}}
 async mutate(intent:PersonalSavedIntent):Promise<boolean>{
  if(this.disposed||this.state.loading||this.state.pending||this.command)throw new Error('Finish or dismiss the outstanding change before starting another.');
  const r=this.state.resource,g=this.generation,route=this.state.route;if(!route||!this.state.viewerFence)throw new Error('Reload Saved before changing it.');
  let path='/v1/saved-resources',method='POST',body:Record<string,unknown>={expectedRevision:r?.revision??0};
  if(intent.action==='create'){if(route.view!=='collections'&&route.view!=='views')throw new Error('Open the matching resource directory.');Object.assign(body,{kind:intent.kind,name:intent.name,summary:intent.summary??'',...(intent.visibility?{visibility:visibility(intent.visibility)}:{}),...(intent.definition?{definition:definition(intent.definition)}:{})});}
  else if(intent.action==='clear-history'||intent.action==='reset-viewing-activity'){if(route.view!=='history')throw new Error('Open viewing history.');path='/v1/personal-history/actions';body={expectedRevision:this.state.revision,action:intent.action};}
  else{
   const action=intent.action==='unshare'?'share':intent.action;if(!r||!r.actions.includes(action))throw new Error('This action is not currently available to your profile.');path+='/'+encodeURIComponent(r.id);
   switch(intent.action){
    case 'update':method='PATCH';Object.assign(body,{...(intent.name!==undefined?{name:intent.name}:{}),...(intent.summary!==undefined?{summary:intent.summary}:{}),...(intent.visibility!==undefined?{visibility:visibility(intent.visibility)}:{}),...(intent.definition?{definition:definition(intent.definition)}:{})});break;
    case 'delete':method='DELETE';break;
    case 'entries':method='PUT';path+='/entries';if((intent.addItemIds?.length??0)>100||(intent.removeEntryIds?.length??0)>100||intent.addItemIds?.some(x=>!id(x))||intent.removeEntryIds?.some(x=>!id(x)))throw new Error('Invalid collection entries.');Object.assign(body,{addItemIds:intent.addItemIds??[],removeEntryIds:intent.removeEntryIds??[]});break;
    case 'share':case 'unshare':path+='/shares';method=intent.action==='share'?'PUT':'DELETE';Object.assign(body,actor(intent.actor));if(intent.action==='share')body.role=intent.role;break;
    case 'pin':path='/v1/saved-pins/'+r.kind+'/'+encodeURIComponent(r.id);method='PUT';body={expectedRevision:r.pinRevision,pinned:intent.pinned};break;
   }
  }
  this.publish({pending:true,mutationError:null,result:null});
  try{const operationId=await this.bounded(()=>this.requestId());if(!this.alive(g))return false;if(!uuid.test(operationId))throw new Error('A UUIDv4 operation ID is required.');body.operationId=operationId;this.command={intent:freeze({...intent}),path,method,body:freeze(body),fence:this.state.viewerFence};return await this.send();}
  catch(e){if(this.alive(g))this.publish({pending:false,mutationError:info(e),retryPending:!!this.command});return false;}
 }
 private async send():Promise<boolean>{const c=this.command,g=this.generation;if(!c||this.disposed)return false;this.publish({pending:true,mutationError:null});
  try{const raw=await this.request<unknown>(c.path,c.method,c.body);if(!this.alive(g))return false;if(!obj(raw)||raw.serverId!==this.scope.serverId||raw.viewerFence!==c.fence)bad();const receipt=c.intent.action==='pin'?raw:raw.receipt;if(!obj(receipt)||receipt.operationId!==c.body.operationId||!num(receipt.revision))bad();const activity=c.intent.action==='clear-history'||c.intent.action==='reset-viewing-activity';if(!activity&&(!id(receipt.resourceId)||typeof receipt.deleted!=='boolean'))bad();
   if(c.intent.action!=='create'&&!activity&&receipt.resourceId!==this.state.resource?.id)bad();
   const deleted=obj(raw.current)&&raw.current.resourceId===receipt.resourceId&&typeof raw.current.deleted==='boolean'?raw.current.deleted:receipt.deleted===true;
   const entries=c.intent.action==='entries'&&receipt.entries!==undefined?parseEntryOutcomes(receipt.entries):undefined;
   this.command=undefined;this.publish({pending:false,retryPending:false,mutationError:null,result:{...(entries?{entries}:{}),...(typeof receipt.resourceId==='string'?{resourceId:receipt.resourceId}:{}),deleted},candidates:[],candidateCursor:''});
   if(!deleted)await this.refresh();return this.alive(g);
  }catch(e){if(!this.alive(g))return false;const error=info(e);this.publish({pending:false,retryPending:true,mutationError:error});if(error.conflict)await this.refresh();return false;}
 }
 /** Network retry is byte-identical. Rebase is a separate, explicit reviewed intent. */
 retry=()=>this.state.pending?Promise.resolve(false):this.send();
 async rebase():Promise<boolean>{const c=this.command;if(!c||this.state.pending||!this.state.mutationError?.conflict)return false;this.command=undefined;return this.mutate(c.intent);}
 dismiss():void{if(this.state.pending)return;this.command=undefined;this.publish({retryPending:false,mutationError:null});}
 dispose():void{this.disposed=true;this.generation++;for(const c of this.controllers)c.abort();this.controllers.clear();this.command=undefined;this.listeners.clear();}
}
