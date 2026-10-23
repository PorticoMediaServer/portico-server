import type {RouteResponse} from "../../contracts/src/routes.ts";
import {routePath} from "../../contracts/src/routes.ts";
import {unreadableServerResponse} from './server-messages.ts';
/** Authorized server search projections. No local matching, grouping, ranking or query persistence.
 * Groups are independent: one group may fail while the response as a whole succeeds, and the
 * client renders that group's own failure rather than discarding the results that arrived. */
import {validateContentEntry,type ContentEntry,type ContentScope,type ContentHeading,type LibraryContentApi} from './library-content.ts';
import {topSearchResult} from './presentation/search-model.ts';
import {firstOfEach,keepReadable,noteSkipped,oneOf,oneOfOr} from './tolerant.ts';
/** How many results each group shows when every group is searched at once (the server's `SearchPreviewLimit`). */
export const SEARCH_PREVIEW_LIMIT=12;
export type SearchGroup='movies'|'shows'|'episodes'|'artists'|'albums'|'songs'|'books'|'collections'|'authors'|'people'|'live-tv'|'programmes';
export type SearchSort='relevance'|'title'|'releaseYear'|'dateAdded'|'channelTitle';
export type SearchDirection='asc'|'desc';
export type SearchGroupStatus='success'|'error';
export type SearchGroupErrorCode=''|'search_group_timeout'|'search_group_unavailable';
export type SearchQuery=Readonly<{q:string;group?:SearchGroup;groups?:readonly SearchGroup[];sort?:SearchSort;direction?:SearchDirection;libraryIds?:readonly string[];record?:boolean}>;
export type SearchChannel=Readonly<{id:string;sourceId:string;generation:string;name:string;number:string;group:string;provenance:'live-source'|'library-channel'}>;
export type SearchProgramme=Readonly<{id:string;title:string;subtitle?:string;start:string;end:string;live:boolean;moreAirings:number;moreAiringsExact:boolean}>;
export type SearchExtras=Readonly<{matchedOn?:Readonly<{field:'credit'|'artist'|'album'|'author'|'narrator'|'series'|'show';text:string}>;roles?:readonly string[];knownFor?:Readonly<{id:string;name:string}>;channel?:SearchChannel;programme?:SearchProgramme}>;
export type SearchEntry=(ContentEntry|Readonly<{id:string;kind:'person'|'channel'|'programme';title:string;subtitle?:string;posterUrl?:string;count?:number;navigation:Readonly<{view:'person'|'channel'|'programme';entityId:string}>}>) & SearchExtras;
export type SearchGroupResult=Readonly<{id:SearchGroup;sourceRevision?:string;windowStart?:string;windowEnd?:string;sort:SearchSort;direction:SearchDirection;labelKey?:string;title:string;entityKind:string;status:SearchGroupStatus;errorCode:SearchGroupErrorCode;items:readonly SearchEntry[];totalCount:number;totalCountExact:boolean;hasMore:boolean;nextCursor:string}>;
export type SearchGroupCapability=Readonly<{id:SearchGroup;labelKey?:string;title:string;entityKind:string;available:boolean;reason:string;sorts:readonly SearchSort[];directions:readonly SearchDirection[]}>;
export type SearchCapabilities=Readonly<{groups:readonly SearchGroupCapability[];sorts:readonly SearchSort[];directions:readonly SearchDirection[];maxLimit:number;groupBudgetMs:number}>;
export type SearchData=Readonly<{top?:Readonly<{group:SearchGroup;entry:SearchEntry;primary?:ContentEntry}>;scope:Readonly<{serverId:string;libraryId:'';libraryKind:'mixed';view:'search';entityId:'';viewerFence:string}>;revision:Readonly<{catalog:number;viewer:number}>;heading:ContentHeading;query:Readonly<{q:string;sort:SearchSort;direction:SearchDirection;group:SearchGroup|'';groups:readonly SearchGroup[];libraryIds:readonly string[];limit:number;searchMode:'normalized_prefix';recorded:boolean}>;groups:readonly SearchGroupResult[];capabilities:SearchCapabilities;empty?:ContentHeading}>;
export type SearchChipCount=Readonly<Pick<SearchGroupResult,"id"|"status"|"errorCode"|"totalCount"|"totalCountExact">>;
export type SearchSnapshot=Readonly<{chipCounts:readonly SearchChipCount[];generation:number;scope:ContentScope;query:SearchQuery|null;phase:'idle'|'loading'|'ready'|'error'|'refresh-required';data:SearchData|null;groups:readonly SearchGroupResult[];pagination:Readonly<{cursor:string|null;canPrevious:boolean}>;error:Readonly<{code:string;message:string;retryable:boolean}>|null}>;
export type SearchHistoryEntry=Readonly<{q:string;updatedAt:string}>;
export type SearchHistoryPage=Readonly<{serverId:string;viewerFence:string;remembered:boolean;entries:readonly SearchHistoryEntry[];maxEntries:number}>;
const groups:readonly SearchGroup[]=['movies','shows','episodes','artists','albums','songs','books','collections','authors','people','live-tv','programmes'];
const sorts:readonly SearchSort[]=['relevance','title','releaseYear','dateAdded','channelTitle'];
const directions:readonly SearchDirection[]=['asc','desc'];
const statuses:readonly SearchGroupStatus[]=['success','error'];
const errorCodes:readonly SearchGroupErrorCode[]=['','search_group_timeout','search_group_unavailable'];
const kinds={movies:'movie',shows:'show',episodes:'episode',artists:'artist',albums:'album',songs:'song',books:'book',collections:'collection',authors:'author',people:'person','live-tv':'channel',programmes:'programme'} as const;
const targets={movies:'item',shows:'show',episodes:'item',artists:'artist',albums:'album',songs:'item',books:'book',collections:'collection',authors:'author',people:'person','live-tv':'channel',programmes:'programme'} as const;
const entityGroups:readonly SearchGroup[]=['people','live-tv','programmes'];
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
/** How many groups, sorts or directions one answer may name. It leaves room for ones a newer server adds, which are left out one by one, and still refuses an unbounded list. */
const MAX_PUBLISHED=64;
/** The names in a published list that this app knows, each once; one it does not know is left out. The list's bound is exact, and a list the server leaves out is empty. */
function known<T extends string>(part:string,v:unknown,max:number,allowed:readonly T[]):readonly T[]{return Object.freeze([...new Set(keepReadable('search',part,array(v??[],max),x=>oneOf(x,allowed)))]);}
/** A heading that cannot be read is blank, so the results are still shown. */
function headingOr(v:unknown,key:string):ContentHeading{try{return heading(v);}catch{if(v!==undefined&&v!==null)noteSkipped('search','heading');return Object.freeze({key,fallback:''});}}
/** Why a typed query can't be searched (the server's NormalizeSearch rules, CD-30), so a client can
 * say so instead of sending it: `short` (no letters or numbers),
 * `words` (more than 8 words), `long` (over 128 code points). Undefined when it can be searched, or
 * when it is empty (idle). */
export function searchQueryProblem(q:string):'short'|'words'|'long'|undefined{
 const normalized=q.split(/\p{White_Space}+/u).filter(Boolean).join(' ');
 if(normalized==='')return undefined;
 const length=[...normalized].length;
 if(length>128)return 'long';
 if(length<1)return 'short';
 const tokens=normalized.replace(/[^\p{L}\p{N}\p{White_Space}]/gu,'').split(/\p{White_Space}+/u).filter(Boolean);
 if(tokens.length>8)return 'words';
 if(!tokens.length)return 'short';
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
 // (the caller clears without requesting); anything else must be 1–128 code
 // points and 1–8 normalized letter/number words.
 if(normalized==='')return Object.freeze({q:normalized,...(group?{group}:{}),...(restrict?{groups:Object.freeze([...restrict])}:{}),...(sort?{sort}:{}),...(direction?{direction}:{}),...(libraryIds?{libraryIds:Object.freeze([...libraryIds])}:{}),...(record?{record:true as const}:{})});
 const problem=searchQueryProblem(normalized);
 if(problem==='long'||!text(normalized,512))throw new Error('Search supports 1 to 128 characters.');
 if(problem)throw new Error('Invalid search query.');
 return Object.freeze({q:normalized,...(group?{group}:{}),...(restrict?{groups:Object.freeze([...restrict])}:{}),...(sort?{sort}:{}),...(direction?{direction}:{}),...(libraryIds?{libraryIds:Object.freeze([...libraryIds])}:{}),...(record?{record:true as const}:{})});
}
export const SEARCH_DRAWN_GROUPS:readonly SearchGroup[]=Object.freeze(['movies','shows','episodes','people','artists','albums','songs','books','live-tv','programmes']);
function extras(v:Record<string,unknown>):SearchExtras {
 const out:{matchedOn?:SearchExtras['matchedOn'];roles?:readonly string[];knownFor?:SearchExtras['knownFor'];channel?:SearchChannel;programme?:SearchProgramme}={};
 if(object(v.matchedOn)&&['credit','artist','album','author','narrator','series','show'].includes(v.matchedOn.field as string)&&text(v.matchedOn.text,2048)&&v.matchedOn.text)out.matchedOn=Object.freeze({field:v.matchedOn.field as NonNullable<SearchExtras['matchedOn']>['field'],text:v.matchedOn.text});
 if(Array.isArray(v.roles)&&v.roles.length<=16)out.roles=Object.freeze(v.roles.filter((x):x is string=>text(x,256)&&!!x));
 if(object(v.knownFor)&&id(v.knownFor.id)&&text(v.knownFor.name,2048)&&v.knownFor.name)out.knownFor=Object.freeze({id:v.knownFor.id,name:v.knownFor.name});
 const ch=v.channel;
 if(object(ch)&&id(ch.id)&&id(ch.sourceId)&&id(ch.generation)&&text(ch.name,2048)&&text(ch.number,256)&&text(ch.group,1024)&&['live-source','library-channel'].includes(ch.provenance as string))out.channel=Object.freeze({id:ch.id,sourceId:ch.sourceId,generation:ch.generation,name:ch.name,number:ch.number,group:ch.group,provenance:ch.provenance as SearchChannel['provenance']});
 const p=v.programme;
 if(object(p)&&id(p.id)&&text(p.title,2048)&&text(p.subtitle??'',4096)&&typeof p.start==='string'&&typeof p.end==='string'&&Number.isFinite(Date.parse(p.start))&&Date.parse(p.end)>Date.parse(p.start)&&typeof p.live==='boolean'&&count(p.moreAirings)&&typeof p.moreAiringsExact==='boolean')out.programme=Object.freeze({id:p.id,title:p.title,...(p.subtitle?{subtitle:p.subtitle as string}:{}),start:p.start,end:p.end,live:p.live,moreAirings:p.moreAirings,moreAiringsExact:p.moreAiringsExact});
 return Object.freeze(out);
}
/** A person or channel is an entity row, not a media item: whatever else the server sends with it, the row carries no library, playback or progress. An optional caption, picture or count that cannot be read is left out. */
function entityEntry(v:unknown,group:SearchGroup):SearchEntry{
 if(!object(v)||!id(v.id)||v.kind!==kinds[group]||!text(v.title??'',2048))invalid();
 if(!object(v.navigation)||v.navigation.view!==targets[group]||v.navigation.entityId!==v.id)invalid();
 const context=extras(v);if(group==='programmes'&&(!context.channel||!context.programme||v.id!==context.channel.provenance+':'+context.channel.sourceId+':'+context.programme.id))invalid();
 return Object.freeze({...context,id:v.id,kind:kinds[group] as 'person'|'channel'|'programme',title:(v.title??'') as string,...(text(v.subtitle,4096)?{subtitle:v.subtitle}:{}),...(text(v.posterUrl,4096)&&v.posterUrl?{posterUrl:v.posterUrl}:{}),...(count(v.count)?{count:v.count}:{}),navigation:Object.freeze({view:targets[group] as 'person'|'channel'|'programme',entityId:v.id})});
}
/** What search offers. Its limits are exact; a group, sort or direction this app does not know is left out of what it offers. */
function capabilities(v:unknown):SearchCapabilities{
 if(!object(v)||!count(v.maxLimit)||!count(v.groupBudgetMs)||!v.maxLimit||!v.groupBudgetMs)invalid();
 const published=firstOfEach('search','capabilities.groups',keepReadable('search','capabilities.groups',array(v.groups,MAX_PUBLISHED),g=>{
  if(!object(g)||!groups.includes(g.id as SearchGroup)||!text(g.title??'',512)||!text(g.reason??'',1024))invalid();
  // A group that does not say it is available is not; one that does not say why reads without a reason.
  return Object.freeze({id:g.id as SearchGroup,title:(g.title??'') as string,...(text(g.labelKey,512)?{labelKey:g.labelKey}:{}),entityKind:kinds[g.id as SearchGroup] as string,available:g.available===true,reason:(g.reason??'') as string,sorts:known('capabilities.group.sorts',g.sorts,MAX_PUBLISHED,sorts),directions:known('capabilities.group.directions',g.directions??['asc','desc'],2,directions)});
 }),g=>g.id);
 return Object.freeze({groups:Object.freeze(published),sorts:known('capabilities.sorts',v.sorts,MAX_PUBLISHED,sorts),directions:known('capabilities.directions',v.directions,MAX_PUBLISHED,directions),maxLimit:v.maxLimit,groupBudgetMs:v.groupBudgetMs});
}
/**
 * One search answer. Its scope, revisions, the query it echoes (text, group, limit, and the sort
 * and direction the client asked for) and its bounds are exact. A group this app does not know,
 * did not ask for, or cannot read is left out alone, and so is a result that cannot be read or is
 * not of its group's kind. A failed group shows only its failure; a status or failure code this
 * app does not know reads as success or as unavailable.
 */
function parse(raw:unknown,op:Operation,s:ContentScope,limit:number):SearchData{
 if(!object(raw)||!object(raw.scope)||!object(raw.revision)||!object(raw.query))invalid();const scope=raw.scope,r=raw.revision,q=raw.query;
 if(scope.serverId!==s.serverId||scope.libraryId!==''||scope.libraryKind!=='mixed'||scope.view!=='search'||scope.entityId!==''||!id(scope.viewerFence)||!count(r.catalog)||!count(r.viewer))invalid();
 if(!text(q.q,512)||!q.q||q.group!==(op.query.group??'')||q.limit!==limit)invalid();
 if(q.q!==op.query.q)invalid();
 // The order the client asked for must be the order answered; an order it left to the server and does not know reads as relevance, best first.
 if(op.query.sort&&q.sort!==op.query.sort||op.query.direction&&q.direction!==op.query.direction)invalid();
 // Recording a search in the viewer's history is the viewer's choice: an answer that says it recorded
 // a search nobody asked it to is refused. One that declined to record (the profile has since turned
 // history off, perhaps on another device) is an ordinary answer.
 if(q.recorded===true&&op.query.record!==true)invalid();
 const sort=oneOfOr('search','query.sort',q.sort,sorts,'relevance'),direction=oneOfOr('search','query.direction',q.direction,directions,'desc');
 const echoedGroups=known('query.groups',q.groups,MAX_PUBLISHED,groups);
 const requested=op.query.group?[op.query.group]:op.query.groups;

 const libraryIds=names(q.libraryIds??[],64);
 if(op.query.libraryIds&&(libraryIds.length!==op.query.libraryIds.length||op.query.libraryIds.some(v=>!libraryIds.includes(v))))invalid();
 const published=capabilities(raw.capabilities);
 if(requested){const admitted=requested.filter(g=>published.groups.some(c=>c.id===g));if(echoedGroups.length!==admitted.length||admitted.some(g=>!echoedGroups.includes(g)))invalid();}
 const results=firstOfEach('search','groups',keepReadable('search','groups',array(raw.groups,MAX_PUBLISHED),value=>{
  if(!object(value)||!groups.includes(value.id as SearchGroup)||!text(value.title??'',512)||!text(value.nextCursor??''))invalid();
  const group=value.id as SearchGroup;
  if(value.entityKind!==kinds[group])invalid();
  if(!echoedGroups.includes(group))invalid();
  const code=value.errorCode===undefined||value.errorCode===null?'':value.errorCode;
  const status:SearchGroupStatus=oneOf(value.status,statuses)??(code?'error':'success');
  // A failed group carries no partial page, and a successful one never carries a failure code.
  if(status==='error')return Object.freeze({id:group,...(id(value.sourceRevision)?{sourceRevision:value.sourceRevision}:{}),...(text(value.windowStart,64)&&Number.isFinite(Date.parse(value.windowStart))?{windowStart:value.windowStart}:{}),...(text(value.windowEnd,64)&&Number.isFinite(Date.parse(value.windowEnd))?{windowEnd:value.windowEnd}:{}),sort:oneOf(value.sort,sorts)??sort,direction:oneOf(value.direction,directions)??direction,title:(value.title??'') as string,...(text(value.labelKey,512)?{labelKey:value.labelKey}:{}),entityKind:value.entityKind as string,status,errorCode:oneOf(code,errorCodes)||'search_group_unavailable' as const,items:Object.freeze([]),totalCount:0,totalCountExact:false,hasMore:false,nextCursor:''});
  const items=firstOfEach('search','items',keepReadable('search','items',array(value.items,op.query.group?limit:Math.min(limit,SEARCH_PREVIEW_LIMIT)),v=>{
   if(entityGroups.includes(group))return entityEntry(v,group);
   const media=validateContentEntry(v);if(!object(v))invalid();const context=extras(v);
   if(!id(media.libraryId)||!libraryIds.includes(media.libraryId)||media.kind!==kinds[group]||media.navigation?.view!==targets[group]||media.navigation.entityId!==media.id)invalid();
   if(media.playback&&(!['movies','episodes','songs'].includes(group)||media.playback.itemId!==media.id||media.available===false))invalid();
   return Object.freeze({...media,...context});
  }),e=>e.id);
  const nextCursor=(value.nextCursor??'') as string;
  // The count is at least the results kept, and more are offered exactly when there is a cursor to ask with.
  return Object.freeze({id:group,...(id(value.sourceRevision)?{sourceRevision:value.sourceRevision}:{}),...(text(value.windowStart,64)&&Number.isFinite(Date.parse(value.windowStart))?{windowStart:value.windowStart}:{}),...(text(value.windowEnd,64)&&Number.isFinite(Date.parse(value.windowEnd))?{windowEnd:value.windowEnd}:{}),sort:oneOf(value.sort,sorts)??sort,direction:oneOf(value.direction,directions)??direction,title:(value.title??'') as string,...(text(value.labelKey,512)?{labelKey:value.labelKey}:{}),entityKind:value.entityKind as string,status,errorCode:'' as const,items:Object.freeze(items),totalCount:Math.max(count(value.totalCount)?value.totalCount:0,items.length),totalCountExact:value.totalCountExact===true&&count(value.totalCount)&&value.totalCount>=items.length,hasMore:!!nextCursor,nextCursor});
 }),g=>g.id);
 // An answer none of whose groups could be read is refused rather than shown as no results.
 if(!results.length&&(raw.groups as unknown[]).length)invalid();
 let empty:ContentHeading|undefined;if(raw.empty!==undefined&&raw.empty!==null){try{empty=heading(raw.empty);}catch{noteSkipped('search','empty');}}
 let top:SearchData['top'];
 if(raw.top!==undefined&&raw.top!==null){
  if(!object(raw.top)||!object(raw.top.entry))invalid();
  const candidate=topSearchResult(q.q,results);
  if(!candidate||raw.top.group!==candidate.group||raw.top.entry.id!==candidate.entry.id||raw.top.entry.kind!==candidate.entry.kind)invalid();
  const e=raw.top.entry;const selected=entityGroups.includes(candidate.group)?entityEntry(e,candidate.group):Object.freeze({...validateContentEntry(e),...extras(e)});
  if(selected.title!==candidate.entry.title)invalid();
  if('libraryId' in candidate.entry&&(!('libraryId'in selected)||selected.libraryId!==candidate.entry.libraryId))invalid();
  let primary:ContentEntry|undefined;
  if(raw.top.primary!==undefined&&raw.top.primary!==null){primary=validateContentEntry(raw.top.primary);if(candidate.group!=='shows'||primary.kind!=='episode'||primary.libraryId!==('libraryId'in selected?selected.libraryId:undefined)||primary.playback?.itemId!==primary.id)invalid();}
  top=Object.freeze({group:candidate.group,entry:selected,...(primary?{primary}:{})});
 }
 return Object.freeze({scope:Object.freeze({serverId:s.serverId,libraryId:'' as const,libraryKind:'mixed' as const,view:'search' as const,entityId:'' as const,viewerFence:scope.viewerFence}),revision:Object.freeze({catalog:r.catalog,viewer:r.viewer}),...(top?{top}:{}),heading:headingOr(raw.heading,'search'),query:Object.freeze({q:q.q,sort,direction,group:q.group as SearchGroup|'',groups:echoedGroups,libraryIds,limit,searchMode:'normalized_prefix' as const,recorded:q.recorded===true}),groups:Object.freeze(results),capabilities:published,...(empty?{empty}:{})});
}
/** Recent searches are the viewer's own, bounded and separately erasable. */
export function parseSearchHistory(raw:unknown,scope:ContentScope):SearchHistoryPage{
 if(!object(raw)||raw.serverId!==scope.serverId||!id(raw.viewerFence)||raw.maxEntries!==20||typeof raw.remembered!=='boolean')invalid();
 // An entry that cannot be read is left out alone; a repeated search keeps its first appearance.
 const entries=firstOfEach('search-history','entries',keepReadable('search-history','entries',array(raw.entries,raw.maxEntries),v=>{
  if(!object(v)||!text(v.q,512)||!v.q||!text(v.updatedAt??'',128))invalid();
  return Object.freeze({q:v.q,updatedAt:(v.updatedAt??'') as string});
 }),e=>e.q);
 return Object.freeze({serverId:raw.serverId as string,viewerFence:raw.viewerFence,remembered:raw.remembered,entries:Object.freeze(entries),maxEntries:raw.maxEntries});
}
/** The profile's recent searches, newest first, as the server keeps them (it keeps none while the profile has history turned off). */
export async function readSearchHistory(api:LibraryContentApi,scope:ContentScope,signal?:AbortSignal):Promise<SearchHistoryPage>{
 return parseSearchHistory(await api.request<RouteResponse<"get_search_history">>(routePath("get_search_history"),'GET',undefined,signal),bound(scope));
}
/** Forgets every search the server kept for this profile. */
export async function clearSearchHistory(api:LibraryContentApi,signal?:AbortSignal):Promise<void>{
 await api.request<RouteResponse<"delete_search_history">>(routePath("delete_search_history"),'DELETE',undefined,signal);
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
 return routePath("search") + "?" + (params);
}
type Page=Readonly<{query:SearchQuery;cursor:string|null}>;
type Operation=Page&{history:readonly Page[];fence?:SearchData};
export class SearchService{
 private api:LibraryContentApi;private scope:ContentScope;private readonly pageSize:number;private readonly timeoutMs:number;private generation=0;private disposed=false;private controller?:AbortController;private pending?:Promise<void>;private key='';private operation?:Operation;private listeners=new Set<()=>void>();private state:SearchSnapshot;private lastAll?:Readonly<{key:string;data:SearchData;counts:readonly SearchChipCount[]}>;
 constructor(options:{api:LibraryContentApi;scope:ContentScope;pageSize?:number;timeoutMs?:number}){this.api=options.api;this.scope=bound(options.scope);this.pageSize=options.pageSize??40;this.timeoutMs=options.timeoutMs??15000;if(!Number.isInteger(this.pageSize)||this.pageSize<1||this.pageSize>40||!Number.isFinite(this.timeoutMs)||this.timeoutMs<1||this.timeoutMs>60000)throw new Error('Invalid search bounds.');this.state=this.empty();}
 getSnapshot=():SearchSnapshot=>this.state;
 subscribe=(listener:()=>void):(()=>void)=>{this.listeners.add(listener);return()=>this.listeners.delete(listener);};
 private empty():SearchSnapshot{return Object.freeze({chipCounts:Object.freeze([]),generation:this.generation,scope:this.scope,query:null,phase:'idle',data:null,groups:Object.freeze([]),pagination:Object.freeze({cursor:null,canPrevious:false}),error:null});}
 private withCounts(data:SearchData):readonly SearchChipCount[]{
  const key=JSON.stringify([data.query.q,[...data.query.libraryIds].sort()]);
  const counts=Object.freeze(data.groups.map(({id,status,errorCode,totalCount,totalCountExact})=>Object.freeze({id,status,errorCode,totalCount,totalCountExact})));
  if(data.query.group===''){this.lastAll=Object.freeze({key,data,counts});return counts;}
  const previous=this.lastAll;
  if(!previous||previous.key!==key||previous.data.scope.viewerFence!==data.scope.viewerFence||previous.data.revision.catalog!==data.revision.catalog||previous.data.revision.viewer!==data.revision.viewer){this.lastAll=undefined;return counts;}
  return Object.freeze([...previous.counts.filter(c=>!counts.some(n=>n.id===c.id)),...counts]);
 }
 private active(){if(this.disposed)throw new Error('Search service is disposed.');}
 private publish(patch:Partial<SearchSnapshot>){this.state=Object.freeze({...this.state,...patch});for(const listener of this.listeners)listener();}
 select(q:string,options:Omit<SearchQuery,'q'>|SearchGroup={}):Promise<void>{this.active();const chosen=typeof options==='string'?{group:options}:options;const checked=query(q,chosen.group||chosen.groups?chosen:{...chosen,groups:SEARCH_DRAWN_GROUPS});if(!checked.q){this.cancel();return Promise.resolve();}return this.load({query:checked,cursor:null,history:[]});}
 next(group:SearchGroup):Promise<void>{this.active();const data=this.state.data,result=data?.groups.find(g=>g.id===group);if(this.state.phase!=='ready'||!result?.nextCursor||!this.operation)return Promise.resolve();return this.load({query:query(data!.query.q,{...this.operation.query,group,groups:undefined}),cursor:result.nextCursor,history:[...this.operation.history,{query:this.operation.query,cursor:this.operation.cursor}].slice(-64),fence:data!});}
 previous():Promise<void>{this.active();const op=this.operation;if(this.state.phase!=='ready'||!op?.history.length||!this.state.data)return Promise.resolve();return this.load({...op.history.at(-1)!,history:op.history.slice(0,-1),fence:this.state.data});}
 refresh():Promise<void>{this.active();const q=this.state.query;return q?this.load({query:q,cursor:null,history:[]}):Promise.resolve();}
 retry():Promise<void>{this.active();if(this.state.phase==='refresh-required')return this.refresh();return this.operation?this.load(this.operation):Promise.resolve();}
 /**
  * Run the search that is showing again without touching `phase`, and show the answer in place.
  * A failed read rejects and changes nothing.
  */
 async revalidate(context?:{signal?:AbortSignal;settle?:()=>Promise<void>}):Promise<void>{
  this.active();const op=this.operation;
  if(!op||this.state.phase!=='ready'||!this.state.data)return;
  const generation=this.generation,controller=new AbortController(),stop=()=>controller.abort(),scope=this.scope;let timer:ReturnType<typeof setTimeout>|undefined;
  context?.signal?.addEventListener('abort',stop,{once:true});
  try{
   const raw=await Promise.race([this.api.request<unknown>(searchPath(op.query,this.pageSize,op.cursor),'GET',undefined,controller.signal),new Promise<never>((_,reject)=>{timer=setTimeout(()=>{reject(new Failure('timeout','Search took too long. Try again.',true));controller.abort();},this.timeoutMs);})]);
   if(this.disposed||generation!==this.generation)return;
   const data=parse(raw,op,scope,this.pageSize);
   await context?.settle?.();
   if(this.disposed||generation!==this.generation||controller.signal.aborted||this.operation!==op||this.state.phase!=='ready')return;
   if(JSON.stringify(data)===JSON.stringify(this.state.data))return;
   this.publish({data,groups:data.groups,chipCounts:this.withCounts(data)});
  }catch(error){
   if(this.disposed||generation!==this.generation||controller.signal.aborted)return;
   const e=error as {code?:unknown;status?:number};
   if(e?.status===401||e?.status===403||['stale_continuation','invalid_cursor','invalid_search'].includes(String(e?.code))){this.lastAll=undefined;this.publish({phase:['stale_continuation','invalid_cursor'].includes(String(e?.code))?'refresh-required':'error',data:null,groups:Object.freeze([]),chipCounts:Object.freeze([])});throw error;}
   throw error;
  }finally{if(timer)clearTimeout(timer);context?.signal?.removeEventListener('abort',stop);}
 }
 cancel(){this.active();this.stop();this.operation=undefined;this.lastAll=undefined;this.publish(this.empty());}
 setScope(scope:ContentScope,api:LibraryContentApi){this.active();const checked=bound(scope);this.stop();this.scope=checked;this.api=api;this.operation=undefined;this.lastAll=undefined;this.publish(this.empty());}
 dispose(){this.stop();this.disposed=true;this.listeners.clear();}
 private stop(){this.generation++;this.controller?.abort();this.pending=undefined;this.key='';}
 private load(op:Operation):Promise<void>{const key=JSON.stringify([op.query,op.cursor]);if(this.pending&&key===this.key)return this.pending;this.stop();this.key=key;this.operation=op;const generation=this.generation,controller=new AbortController();this.controller=controller;const retain=this.state.query?.q===op.query.q&&this.state.query?.group===op.query.group;this.publish({generation,query:op.query,phase:'loading',...(retain?{}:{data:null,groups:Object.freeze([]),chipCounts:this.state.query?.q===op.query.q?this.state.chipCounts:Object.freeze([])}),error:null,pagination:Object.freeze({cursor:op.cursor,canPrevious:op.history.length>0})});const task=this.fetch(op,this.api,this.scope,controller,generation);this.pending=task;return task;}
 private async fetch(op:Operation,api:LibraryContentApi,scope:ContentScope,controller:AbortController,generation:number){let timer:ReturnType<typeof setTimeout>|undefined;let abort:()=>void=()=>{};
  try{const raw=await Promise.race([api.request<unknown>(searchPath(op.query,this.pageSize,op.cursor),'GET',undefined,controller.signal),new Promise<never>((_,reject)=>{abort=()=>reject(new Failure('cancelled','Search cancelled.'));controller.signal.addEventListener('abort',abort,{once:true});timer=setTimeout(()=>{reject(new Failure('timeout','Search took too long. Try again.',true));controller.abort();},this.timeoutMs);})]);if(this.disposed||generation!==this.generation)return;const data=parse(raw,op,scope,this.pageSize);
   // A later page read after the library moved is still a valid page; only another viewer's answer is refused.
   if(op.fence&&(data.scope.viewerFence!==op.fence.scope.viewerFence||data.revision.catalog!==op.fence.revision.catalog||data.revision.viewer!==op.fence.revision.viewer))throw new Failure('stale_continuation','Search results changed. Refresh to continue.',true);
   // A repeated cursor is a broken continuation, never proof of exhaustion.
   const repeats=(c:string)=>c===op.cursor||op.history.some(p=>p.cursor===c);
   if(data.groups.some(g=>g.nextCursor&&repeats(g.nextCursor)))throw new Failure('invalid_cursor','Search returned a repeated cursor. Refresh to continue.');
   this.publish({query:op.query,phase:'ready',data,groups:data.groups,chipCounts:this.withCounts(data),pagination:Object.freeze({cursor:op.cursor,canPrevious:op.history.length>0})});
  }catch(error){if(this.disposed||generation!==this.generation)return;const e=error as {code?:unknown;retryable?:unknown;status?:number};const code=e?.status===401?'unauthorized':e?.status===403?'forbidden':id(e?.code)?e.code:'request_failed';
   if(['unauthorized','forbidden','stale_continuation','invalid_cursor'].includes(code))this.lastAll=undefined;
   const transient=!['unauthorized','forbidden','invalid_search','stale_continuation','invalid_cursor'].includes(code)&&(error instanceof TypeError||e?.retryable===true||code==='timeout'||!!e?.status&&(e.status>=500||[408,429].includes(e.status)));this.publish({phase:['stale_continuation','invalid_cursor'].includes(code)?'refresh-required':'error',...(transient?{}:{data:null,groups:Object.freeze([]),chipCounts:Object.freeze([])}),error:Object.freeze({code,message:['unauthorized','forbidden'].includes(code)?'Your access changed. Sign in again to search.':['stale_continuation','invalid_cursor'].includes(code)?'The library changed. Refresh these results.':'Search could not connect to this server. Try again.',retryable:transient})});}
  finally{if(timer)clearTimeout(timer);controller.signal.removeEventListener('abort',abort);if(generation===this.generation){this.pending=undefined;this.key='';}}
 }
}

export type SearchDiscoveryData=Readonly<{scope:Readonly<{serverId:string;viewerFence:string;view:'search-discovery'}>;revision:Readonly<{catalog:number;viewer:number}>;genres:readonly Readonly<{value:string;label:string;count:number;countIsLowerBound:boolean;backdropUrl?:string;posterUrls:readonly string[]}>[];genresNextCursor:string;onNow:readonly SearchEntry[]}>;
/** One bounded server-composed discovery window. Channel ordering, genre counts
 * and representative images come from the admitted server sources. */
export async function readSearchDiscovery(api:LibraryContentApi,scope:ContentScope,options:Readonly<{cursor?:string;limit?:number;signal?:AbortSignal}>={}):Promise<SearchDiscoveryData>{
 bound(scope);const limit=options.limit??12;if(!Number.isInteger(limit)||limit<1||limit>24)throw new Error('Invalid Search discovery bounds.');
 const params=new URLSearchParams({limit:String(limit)});if(options.cursor)params.set('cursor',options.cursor);
 const raw:unknown=await api.request<RouteResponse<'get_search_discovery'>>(routePath('get_search_discovery')+'?'+params,'GET',undefined,options.signal);
 if(!object(raw)||!object(raw.scope)||raw.scope.serverId!==scope.serverId||raw.scope.view!=='search-discovery'||!id(raw.scope.viewerFence)||!object(raw.revision)||!count(raw.revision.catalog)||!count(raw.revision.viewer)||!text(raw.genresNextCursor))invalid();
 const genres=Object.freeze(array(raw.genres,limit).map(v=>{if(!object(v)||!id(v.value)||!text(v.label,2048)||!count(v.count)||typeof v.countIsLowerBound!=='boolean')invalid();return Object.freeze({value:v.value,label:v.label,count:v.count,countIsLowerBound:v.countIsLowerBound,posterUrls:Object.freeze(array(v.posterUrls,2).map(p=>{if(!text(p,4096)||!p)invalid();return p;})),...(text(v.backdropUrl,4096)&&v.backdropUrl?{backdropUrl:v.backdropUrl}:{})});}));
 const onNow=Object.freeze(array(raw.onNow,4).map(v=>entityEntry(v,'live-tv')));
 return Object.freeze({scope:Object.freeze({serverId:scope.serverId,viewerFence:raw.scope.viewerFence,view:'search-discovery' as const}),revision:Object.freeze({catalog:raw.revision.catalog,viewer:raw.revision.viewer}),genres,genresNextCursor:raw.genresNextCursor,onNow});
}
/** Play all/Shuffle send the whole source definition to the durable queue.
 * The visible cards are never enumerated or converted into an item selector. */
export function searchSongQueueSource(q:SearchQuery):Readonly<{search:Readonly<{q:string;group:'songs';sort?:'relevance'|'title'|'releaseYear'|'dateAdded';direction?:SearchDirection;libraryIds?:readonly string[]}>}>{
 const checked=query(q.q,q);if(!checked.q||checked.group!=='songs'||checked.sort==='channelTitle')throw new Error('A song Search is required.');
 return Object.freeze({search:Object.freeze({q:checked.q,group:'songs' as const,...(checked.sort?{sort:checked.sort}:{}),...(checked.direction?{direction:checked.direction}:{}),...(checked.libraryIds?{libraryIds:checked.libraryIds}:{})})});
}
