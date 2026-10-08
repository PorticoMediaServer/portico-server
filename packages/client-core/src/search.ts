import {unreadableServerResponse} from './server-messages.ts';
/** Authorized server search projections. No local matching, grouping, ranking or query persistence.
 * Groups are independent: one group may fail while the response as a whole succeeds, and the
 * client renders that group's own failure rather than discarding the results that arrived. */
import {validateContentEntry,type ContentEntry,type ContentScope,type ContentHeading,type LibraryContentApi} from './library-content.ts';
/** How many results each group shows when every group is searched at once (the server's `SearchPreviewLimit`). */
export const SEARCH_PREVIEW_LIMIT=12;
export type SearchGroup='movies'|'shows'|'episodes'|'artists'|'albums'|'songs'|'books'|'people'|'live-tv';
export type SearchSort='relevance'|'title'|'releaseYear'|'dateAdded';
export type SearchDirection='asc'|'desc';
export type SearchGroupStatus='success'|'error';
export type SearchGroupErrorCode=''|'search_group_timeout'|'search_group_unavailable';
export type SearchQuery=Readonly<{q:string;group?:SearchGroup;groups?:readonly SearchGroup[];sort?:SearchSort;direction?:SearchDirection;libraryIds?:readonly string[];record?:boolean}>;
export type SearchEntry=ContentEntry|Readonly<{id:string;kind:'person'|'channel';title:string;subtitle?:string;posterUrl?:string;count?:number;navigation:Readonly<{view:'person'|'channel';entityId:string}>}>;
export type SearchGroupResult=Readonly<{id:SearchGroup;title:string;entityKind:string;status:SearchGroupStatus;errorCode:SearchGroupErrorCode;items:readonly SearchEntry[];totalCount:number;hasMore:boolean;nextCursor:string}>;
export type SearchGroupCapability=Readonly<{id:SearchGroup;title:string;entityKind:string;available:boolean;reason:string;sorts:readonly SearchSort[]}>;
export type SearchCapabilities=Readonly<{groups:readonly SearchGroupCapability[];sorts:readonly SearchSort[];directions:readonly SearchDirection[];maxLimit:number;groupBudgetMs:number}>;
export type SearchData=Readonly<{scope:Readonly<{serverId:string;libraryId:'';libraryKind:'mixed';view:'search';entityId:'';viewerFence:string}>;revision:Readonly<{catalog:number;viewer:number}>;heading:ContentHeading;query:Readonly<{q:string;sort:SearchSort;direction:SearchDirection;group:SearchGroup|'';groups:readonly SearchGroup[];libraryIds:readonly string[];limit:number;searchMode:'token_prefix';recorded:boolean}>;groups:readonly SearchGroupResult[];capabilities:SearchCapabilities;empty?:ContentHeading}>;
export type SearchSnapshot=Readonly<{generation:number;scope:ContentScope;query:SearchQuery|null;phase:'idle'|'loading'|'ready'|'error'|'refresh-required';data:SearchData|null;groups:readonly SearchGroupResult[];pagination:Readonly<{cursor:string|null;canPrevious:boolean}>;error:Readonly<{code:string;message:string;retryable:boolean}>|null}>;
export type SearchHistoryEntry=Readonly<{q:string;updatedAt:string;uses:number}>;
export type SearchHistoryPage=Readonly<{serverId:string;viewerFence:string;entries:readonly SearchHistoryEntry[];maxEntries:number}>;
const groups:readonly SearchGroup[]=['movies','shows','episodes','artists','albums','songs','books','people','live-tv'];
const sorts:readonly SearchSort[]=['relevance','title','releaseYear','dateAdded'];
const directions:readonly SearchDirection[]=['asc','desc'];
const statuses:readonly SearchGroupStatus[]=['success','error'];
const errorCodes:readonly SearchGroupErrorCode[]=['','search_group_timeout','search_group_unavailable'];
const kinds={movies:'movie',shows:'show',episodes:'episode',artists:'artist',albums:'album',songs:'song',books:'book',people:'person','live-tv':'channel'} as const;
const targets={movies:'item',shows:'show',episodes:'item',artists:'artist',albums:'album',songs:'item',books:'book',people:'person','live-tv':'channel'} as const;
const entityGroups:readonly SearchGroup[]=['people','live-tv'];
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>text(v,256)&&v.length>0;
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
class Failure extends Error{code:string;retryable:boolean;constructor(code:string,message:string,retryable=false){super(message);this.code=code;this.retryable=retryable;}}
function invalid():never{throw new Failure('invalid_search',unreadableServerResponse);}
function array(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)invalid();return v;}
function heading(v:unknown):ContentHeading{if(!object(v)||!id(v.key)||!text(v.fallback,512))invalid();return Object.freeze({key:v.key,fallback:v.fallback});}
function bound(s:ContentScope):ContentScope{if(!object(s)||!id(s.serverId)||!text(s.viewerId,1024)||!s.viewerId)throw new Error('Search requires a bound server and viewer.');return Object.freeze({...s});}
function names(v:unknown,max:number,allowed?:readonly string[]):readonly string[]{return Object.freeze(array(v,max).map(x=>{if(!id(x)||allowed&&!allowed.includes(x))invalid();return x;}));}
/** Why a typed query can't be searched (the server's NormalizeSearch rules, CD-30), so a client can
 * say so instead of sending it: `short` (under 2 code points, or no word of 2+ letters or numbers),
 * `words` (more than 8 words), `long` (over 128 code points). Undefined when it can be searched, or
 * when it is empty (idle). */
export function searchQueryProblem(q:string):'short'|'words'|'long'|undefined{
 const normalized=q.split(/\p{White_Space}+/u).filter(Boolean).join(' ');
 if(normalized==='')return undefined;
 const length=[...normalized].length;
 if(length>128)return 'long';
 if(length<2)return 'short';
 const tokens=normalized.split(/[^\p{L}\p{N}]+/u).filter(Boolean);
 if(tokens.length>8)return 'words';
 if(!tokens.length||!tokens.some(t=>[...t].length>=2))return 'short';
 return undefined;
}
function query(q:string,options:Omit<SearchQuery,'q'>={}):SearchQuery{
 const {group,groups:restrict,sort,direction,libraryIds,record}=options;
 if(typeof q!=='string'||q.length>2048)throw new Error('Invalid search query.');
 if(group!==undefined&&!groups.includes(group))throw new Error('Unknown search group.');
 if(restrict!==undefined&&(group!==undefined||!restrict.length||restrict.length>groups.length||restrict.some(g=>!groups.includes(g))||new Set(restrict).size!==restrict.length))throw new Error('Invalid search group restriction.');
 if(sort!==undefined&&!sorts.includes(sort))throw new Error('Unknown search sort.');
 if(direction!==undefined&&!directions.includes(direction))throw new Error('Unknown search direction.');
 if(libraryIds!==undefined&&(!libraryIds.length||libraryIds.length>64||libraryIds.some(v=>!id(v))))throw new Error('Invalid library restriction.');
 const normalized=q.split(/\p{White_Space}+/u).filter(Boolean).join(' ');
 // CD-30: match server catalog.NormalizeSearch before dispatch. Empty stays idle
 // (the caller clears without requesting); anything else must be 2–128 code
 // points, 1–8 letter/number tokens, with at least one token of 2+ code points.
 if(normalized==='')return Object.freeze({q:normalized,...(group?{group}:{}),...(restrict?{groups:Object.freeze([...restrict])}:{}),...(sort?{sort}:{}),...(direction?{direction}:{}),...(libraryIds?{libraryIds:Object.freeze([...libraryIds])}:{}),...(record?{record:true as const}:{})});
 const problem=searchQueryProblem(normalized);
 if(problem==='long'||!text(normalized,512))throw new Error('Search supports 2 to 128 characters.');
 if(problem)throw new Error('Invalid search query.');
 return Object.freeze({q:normalized,...(group?{group}:{}),...(restrict?{groups:Object.freeze([...restrict])}:{}),...(sort?{sort}:{}),...(direction?{direction}:{}),...(libraryIds?{libraryIds:Object.freeze([...libraryIds])}:{}),...(record?{record:true as const}:{})});
}
/** A person or channel is an entity row, not a media item: it carries no library, playback or progress. */
function entityEntry(v:unknown,group:SearchGroup):SearchEntry{
 if(!object(v)||!id(v.id)||v.kind!==kinds[group]||!text(v.title,2048))invalid();
 if(!object(v.navigation)||v.navigation.view!==targets[group]||v.navigation.entityId!==v.id)invalid();
 if(v.playback!==undefined||v.available!==undefined||v.libraryId!==undefined)invalid();
 if(v.subtitle!==undefined&&!text(v.subtitle,4096))invalid();
 if(v.posterUrl!==undefined&&!text(v.posterUrl,4096))invalid();
 if(v.count!==undefined&&!count(v.count))invalid();
 return Object.freeze({id:v.id,kind:kinds[group] as 'person'|'channel',title:v.title,...(v.subtitle===undefined?{}:{subtitle:v.subtitle as string}),...(v.posterUrl===undefined||v.posterUrl===''?{}:{posterUrl:v.posterUrl as string}),...(v.count===undefined?{}:{count:v.count as number}),navigation:Object.freeze({view:targets[group] as 'person'|'channel',entityId:v.id})});
}
function capabilities(v:unknown):SearchCapabilities{
 if(!object(v)||!count(v.maxLimit)||!count(v.groupBudgetMs)||!v.maxLimit||!v.groupBudgetMs)invalid();
 const published=array(v.groups,groups.length).map(g=>{
  if(!object(g)||!groups.includes(g.id as SearchGroup)||!text(g.title,512)||!id(g.entityKind)||typeof g.available!=='boolean')invalid();
  if(g.reason!==undefined&&!text(g.reason,1024))invalid();
  // An unavailable group must say why; silence would look like an empty result.
  if(!g.available&&!g.reason)invalid();
  return Object.freeze({id:g.id as SearchGroup,title:g.title,entityKind:g.entityKind,available:g.available,reason:(g.reason as string)??'',sorts:names(g.sorts,sorts.length,sorts) as readonly SearchSort[]});
 });
 if(new Set(published.map(g=>g.id)).size!==published.length)invalid();
 return Object.freeze({groups:Object.freeze(published),sorts:names(v.sorts,sorts.length,sorts) as readonly SearchSort[],directions:names(v.directions,2,directions) as readonly SearchDirection[],maxLimit:v.maxLimit,groupBudgetMs:v.groupBudgetMs});
}
function parse(raw:unknown,op:Operation,s:ContentScope,limit:number):SearchData{
 if(!object(raw)||!object(raw.scope)||!object(raw.revision)||!object(raw.query))invalid();const scope=raw.scope,r=raw.revision,q=raw.query;
 if(scope.serverId!==s.serverId||scope.libraryId!==''||scope.libraryKind!=='mixed'||scope.view!=='search'||scope.entityId!==''||!id(scope.viewerFence)||!count(r.catalog)||!count(r.viewer))invalid();
 if(!text(q.q,512)||!q.q||!sorts.includes(q.sort as SearchSort)||!directions.includes(q.direction as SearchDirection)||q.group!==(op.query.group??'')||q.limit!==limit||q.searchMode!=='token_prefix'||typeof q.recorded!=='boolean')invalid();
 if(q.q!==op.query.q)invalid();
 if(op.query.sort&&q.sort!==op.query.sort||op.query.direction&&q.direction!==op.query.direction)invalid();
 if(q.recorded!==(op.query.record===true))invalid();
 const echoedGroups=names(q.groups,groups.length,groups) as readonly SearchGroup[];
 const requested=op.query.group?[op.query.group]:op.query.groups;
 if(requested&&(echoedGroups.length!==requested.length||requested.some(g=>!echoedGroups.includes(g))))invalid();
 const libraryIds=names(q.libraryIds,64);
 const published=capabilities(raw.capabilities);
 const results=array(raw.groups,groups.length).map(value=>{
  if(!object(value)||!groups.includes(value.id as SearchGroup)||!text(value.title,512)||!id(value.entityKind)||!count(value.totalCount)||!text(value.nextCursor)||typeof value.hasMore!=='boolean')invalid();
  if(!statuses.includes(value.status as SearchGroupStatus)||!errorCodes.includes((value.errorCode??'') as SearchGroupErrorCode))invalid();
  const group=value.id as SearchGroup;
  if(value.entityKind!==kinds[group])invalid();
  if(!echoedGroups.includes(group))invalid();
  const items=array(value.items,op.query.group?limit:Math.min(limit,SEARCH_PREVIEW_LIMIT)).map(v=>entityGroups.includes(group)?entityEntry(v,group):validateContentEntry(v));
  if(value.totalCount<items.length||new Set(items.map(e=>e.id)).size!==items.length)invalid();
  // A failed group carries no partial page, and a successful one never carries a failure code.
  if(value.status==='error'&&(items.length||value.totalCount||value.hasMore||value.nextCursor||!value.errorCode))invalid();
  if(value.status==='success'&&value.errorCode)invalid();
  if(value.hasMore!==!!value.nextCursor)invalid();
  for(const entry of items){
   if(entityGroups.includes(group))continue;
   const media=entry as ContentEntry;
   if(!id(media.libraryId)||media.kind!==kinds[group]||media.navigation?.view!==targets[group]||media.navigation.entityId!==media.id)invalid();
   if(media.playback&&(!['movies','episodes','songs'].includes(group)||media.playback.itemId!==media.id||media.available===false))invalid();
  }
  return Object.freeze({id:group,title:value.title,entityKind:value.entityKind,status:value.status as SearchGroupStatus,errorCode:((value.errorCode??'') as SearchGroupErrorCode),items:Object.freeze(items),totalCount:value.totalCount,hasMore:value.hasMore,nextCursor:value.nextCursor});
 });
 if(new Set(results.map(g=>g.id)).size!==results.length)invalid();
 if(results.length!==echoedGroups.length)invalid();
 for(const key of ['navigation','sorts','sections','filters'])if(raw[key]!==undefined)invalid();
 return Object.freeze({scope:Object.freeze({serverId:s.serverId,libraryId:'' as const,libraryKind:'mixed' as const,view:'search' as const,entityId:'' as const,viewerFence:scope.viewerFence}),revision:Object.freeze({catalog:r.catalog,viewer:r.viewer}),heading:heading(raw.heading),query:Object.freeze({q:q.q,sort:q.sort as SearchSort,direction:q.direction as SearchDirection,group:q.group as SearchGroup|'',groups:echoedGroups,libraryIds,limit,searchMode:'token_prefix' as const,recorded:q.recorded}),groups:Object.freeze(results),capabilities:published,...(raw.empty===undefined?{}:{empty:heading(raw.empty)})});
}
/** Recent searches are the viewer's own, bounded and separately erasable. */
export function parseSearchHistory(raw:unknown,scope:ContentScope):SearchHistoryPage{
 if(!object(raw)||raw.serverId!==scope.serverId||!id(raw.viewerFence)||!count(raw.maxEntries)||!raw.maxEntries)invalid();
 const entries=array(raw.entries,raw.maxEntries).map(v=>{
  if(!object(v)||!text(v.q,512)||!v.q||!text(v.updatedAt,128)||!v.updatedAt||!count(v.uses)||!v.uses)invalid();
  return Object.freeze({q:v.q,updatedAt:v.updatedAt,uses:v.uses});
 });
 if(new Set(entries.map(e=>e.q)).size!==entries.length)invalid();
 return Object.freeze({serverId:raw.serverId as string,viewerFence:raw.viewerFence,entries:Object.freeze(entries),maxEntries:raw.maxEntries});
}
export function searchPath(q:SearchQuery,limit:number,cursor?:string|null):string{
 const params=new URLSearchParams({q:q.q,limit:String(limit)});
 if(q.group)params.set('group',q.group);
 if(q.groups)params.set('groups',q.groups.join(','));
 if(q.sort)params.set('sort',q.sort);
 if(q.direction)params.set('direction',q.direction);
 if(q.libraryIds)params.set('libraryIds',q.libraryIds.join(','));
 if(q.record)params.set('record','1');
 if(cursor)params.set('cursor',cursor);
 return '/v1/search?'+params;
}
type Page=Readonly<{query:SearchQuery;cursor:string|null}>;
type Operation=Page&{history:readonly Page[];fence?:SearchData};
export class SearchService{
 private api:LibraryContentApi;private scope:ContentScope;private readonly pageSize:number;private readonly timeoutMs:number;private generation=0;private disposed=false;private controller?:AbortController;private pending?:Promise<void>;private key='';private operation?:Operation;private listeners=new Set<()=>void>();private state:SearchSnapshot;
 constructor(options:{api:LibraryContentApi;scope:ContentScope;pageSize?:number;timeoutMs?:number}){this.api=options.api;this.scope=bound(options.scope);this.pageSize=options.pageSize??40;this.timeoutMs=options.timeoutMs??15000;if(!Number.isInteger(this.pageSize)||this.pageSize<1||this.pageSize>40||!Number.isFinite(this.timeoutMs)||this.timeoutMs<1||this.timeoutMs>60000)throw new Error('Invalid search bounds.');this.state=this.empty();}
 getSnapshot=():SearchSnapshot=>this.state;
 subscribe=(listener:()=>void):(()=>void)=>{this.listeners.add(listener);return()=>this.listeners.delete(listener);};
 private empty():SearchSnapshot{return Object.freeze({generation:this.generation,scope:this.scope,query:null,phase:'idle',data:null,groups:Object.freeze([]),pagination:Object.freeze({cursor:null,canPrevious:false}),error:null});}
 private active(){if(this.disposed)throw new Error('Search service is disposed.');}
 private publish(patch:Partial<SearchSnapshot>){this.state=Object.freeze({...this.state,...patch});for(const listener of this.listeners)listener();}
 select(q:string,options:Omit<SearchQuery,'q'>|SearchGroup={}):Promise<void>{this.active();const checked=query(q,typeof options==='string'?{group:options}:options);if(!checked.q){this.cancel();return Promise.resolve();}return this.load({query:checked,cursor:null,history:[]});}
 next(group:SearchGroup):Promise<void>{this.active();const data=this.state.data,result=data?.groups.find(g=>g.id===group);if(this.state.phase!=='ready'||!result?.nextCursor||!this.operation)return Promise.resolve();return this.load({query:query(data!.query.q,{...this.operation.query,group,groups:undefined}),cursor:result.nextCursor,history:[...this.operation.history,{query:this.operation.query,cursor:this.operation.cursor}].slice(-64),fence:data!});}
 previous():Promise<void>{this.active();const op=this.operation;if(this.state.phase!=='ready'||!op?.history.length||!this.state.data)return Promise.resolve();return this.load({...op.history.at(-1)!,history:op.history.slice(0,-1),fence:this.state.data});}
 refresh():Promise<void>{this.active();const q=this.state.query;return q?this.load({query:q,cursor:null,history:[]}):Promise.resolve();}
 retry():Promise<void>{this.active();if(this.state.phase==='refresh-required')return this.refresh();return this.operation?this.load(this.operation):Promise.resolve();}
 cancel(){this.active();this.stop();this.operation=undefined;this.publish(this.empty());}
 setScope(scope:ContentScope,api:LibraryContentApi){this.active();const checked=bound(scope);this.stop();this.scope=checked;this.api=api;this.operation=undefined;this.publish(this.empty());}
 dispose(){this.stop();this.disposed=true;this.listeners.clear();}
 private stop(){this.generation++;this.controller?.abort();this.pending=undefined;this.key='';}
 private load(op:Operation):Promise<void>{const key=JSON.stringify([op.query,op.cursor]);if(this.pending&&key===this.key)return this.pending;this.stop();this.key=key;this.operation=op;const generation=this.generation,controller=new AbortController();this.controller=controller;const retain=this.state.query?.q===op.query.q&&this.state.query?.group===op.query.group;this.publish({generation,query:op.query,phase:'loading',...(retain?{}:{data:null,groups:Object.freeze([])}),error:null,pagination:Object.freeze({cursor:op.cursor,canPrevious:op.history.length>0})});const task=this.fetch(op,this.api,this.scope,controller,generation);this.pending=task;return task;}
 private async fetch(op:Operation,api:LibraryContentApi,scope:ContentScope,controller:AbortController,generation:number){let timer:ReturnType<typeof setTimeout>|undefined;let abort:()=>void=()=>{};
  try{const raw=await Promise.race([api.request<unknown>(searchPath(op.query,this.pageSize,op.cursor),'GET',undefined,controller.signal),new Promise<never>((_,reject)=>{abort=()=>reject(new Failure('cancelled','Search cancelled.'));controller.signal.addEventListener('abort',abort,{once:true});timer=setTimeout(()=>{reject(new Failure('timeout','Search took too long. Try again.',true));controller.abort();},this.timeoutMs);})]);if(this.disposed||generation!==this.generation)return;const data=parse(raw,op,scope,this.pageSize);
   if(op.fence&&(data.scope.viewerFence!==op.fence.scope.viewerFence||data.revision.catalog!==op.fence.revision.catalog||data.revision.viewer!==op.fence.revision.viewer))throw new Failure('stale_continuation','Search results changed. Refresh to continue.',true);
   for(const group of data.groups)if(group.nextCursor&&(group.nextCursor===op.cursor||op.history.some(p=>p.cursor===group.nextCursor)))throw new Failure('stale_continuation','Search continuation repeated. Refresh to continue.',true);
   this.publish({query:op.query,phase:'ready',data,groups:data.groups,pagination:Object.freeze({cursor:op.cursor,canPrevious:op.history.length>0})});
  }catch(error){if(this.disposed||generation!==this.generation)return;const e=error as {code?:unknown;retryable?:unknown;status?:number};const code=e?.status===401?'unauthorized':e?.status===403?'forbidden':id(e?.code)?e.code:'request_failed';const transient=!['unauthorized','forbidden','invalid_search','stale_continuation','invalid_cursor'].includes(code)&&(error instanceof TypeError||e?.retryable===true||code==='timeout'||!!e?.status&&(e.status>=500||[408,429].includes(e.status)));this.publish({phase:['stale_continuation','invalid_cursor'].includes(code)?'refresh-required':'error',...(transient?{}:{data:null,groups:Object.freeze([])}),error:Object.freeze({code,message:['unauthorized','forbidden'].includes(code)?'Your access changed. Sign in again to search.':['stale_continuation','invalid_cursor'].includes(code)?'The library changed. Refresh these results.':'Search could not connect to this server. Try again.',retryable:transient})});}
  finally{if(timer)clearTimeout(timer);controller.signal.removeEventListener('abort',abort);if(generation===this.generation){this.pending=undefined;this.key='';}}
 }
}
