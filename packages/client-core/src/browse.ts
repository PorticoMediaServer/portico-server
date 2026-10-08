import {unreadableServerResponse} from './server-messages.ts';
/** Browse engine contract. The server owns the vocabulary, the query grammar and
 * every count; this module only parses what it publishes and refuses to send a
 * query the published capabilities cannot execute. No client-side filtering,
 * sorting or facet inference lives here. */
import {validateContentEntry,type ContentEntry} from './library-content.ts';

export type BrowseValueType='string'|'enum'|'number'|'date'|'boolean'|'identity-set';
export type BrowseControlHint='toggle'|'select'|'number-range'|'date-range'|'facet-multi-select'|'text';
export type BrowseComplexity='quick'|'standard'|'advanced';
export type BrowseCost='indexed'|'indexed-join';
export type BrowseDirection='asc'|'desc';
export type BrowseOperator='equals'|'not-equals'|'contains'|'starts-with'|'in'|'not-in'|'less-than'|'at-most'|'greater-than'|'at-least'|'between'|'is-present'|'is-missing'|'contains-any'|'contains-all';
export type BrowseScalar=string|number|boolean;
export type BrowseNode=
 |Readonly<{all:readonly BrowseNode[]}>
 |Readonly<{any:readonly BrowseNode[]}>
 |Readonly<{not:BrowseNode}>
 |Readonly<{field:string;operator:BrowseOperator;value:BrowseScalar|readonly BrowseScalar[]|null}>;
export type BrowseSortSelection=Readonly<{field:string;direction:BrowseDirection}>;
export type BrowseFacetSource=Readonly<{endpoint:string;field:string}>;
export type BrowseFieldCapability=Readonly<{id:string;labelKey:string;type:BrowseValueType;operators:readonly BrowseOperator[];controlHint:BrowseControlHint;complexity:BrowseComplexity;cost:BrowseCost;applicableKinds:readonly string[];allowedValues?:readonly string[];facetSource?:BrowseFacetSource}>;
export type BrowseSortCapability=Readonly<{id:string;labelKey:string;directions:readonly BrowseDirection[];defaultDirection:BrowseDirection;expensive:boolean;applicableKinds:readonly string[]}>;
export type BrowsePivotCapability=Readonly<{id:string;labelKey:string;entityKinds:readonly string[];defaultSort:readonly BrowseSortSelection[];supportedViews:readonly string[];browsable:boolean;aggregate?:string}>;
export type BrowseQuickFilter=Readonly<{id:string;labelKey:string;query:BrowseNode}>;
export type BrowseQueryLimits=Readonly<{maximumDepth:number;maximumClauses:number;maximumBytes:number;maximumSorts:number;defaultLimit:number;maximumLimit:number;cursorTtlSeconds:number}>;
export type BrowseLibraryScope=Readonly<{id:string;name:string;kind:string;defaultView:string;pinned:boolean}>;
export type BrowseCapabilities=Readonly<{library:BrowseLibraryScope;pivots:readonly BrowsePivotCapability[];resolvedPivot:BrowsePivotCapability|null;fields:readonly BrowseFieldCapability[];sorts:readonly BrowseSortCapability[];quickFilters:readonly BrowseQuickFilter[];queryLimits:BrowseQueryLimits}>;
export type BrowsePositionAnchor=Readonly<{key:string;index:number}>;
export type BrowsePageInfo=Readonly<{start:number;total:number;revision:string;hasMore:boolean;nextCursor:string}>;
export type BrowseRange=Readonly<{start:number;revision?:string;anchorId?:string}>;
export type BrowseSeek=Readonly<{prefix:string}>;
export type BrowseApplied=Readonly<{query:BrowseNode|null;sort:readonly BrowseSortSelection[];seek:BrowseSeek|null}>;
export type BrowseResult=Readonly<{pivot:string;applied:BrowseApplied;entries:readonly ContentEntry[];pageInfo:BrowsePageInfo;positionIndex:readonly BrowsePositionAnchor[]}>;
export type BrowseFacetValue=Readonly<{value:string;label:string;count:number}>;
export type BrowseFacetPage=Readonly<{field:string;values:readonly BrowseFacetValue[]}>;
export type BrowseRequestBody=Readonly<{pivot?:string;query?:BrowseNode;sort?:readonly BrowseSortSelection[];limit?:number;cursor?:string;range?:BrowseRange;seek?:BrowseSeek}>;

const hints:readonly string[]=['toggle','select','number-range','date-range','facet-multi-select','text'];
const operators:readonly string[]=['equals','not-equals','contains','starts-with','in','not-in','less-than','at-most','greater-than','at-least','between','is-present','is-missing','contains-any','contains-all'];
const types:readonly string[]=['string','enum','number','date','boolean','identity-set'];
const listOperators:readonly string[]=['in','not-in','between','contains-any','contains-all'];
const presenceOperators:readonly string[]=['is-present','is-missing'];

const obj=(v:unknown):v is Record<string,unknown>=>v!==null&&typeof v==='object'&&!Array.isArray(v);
const str=(v:unknown,max=4096):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>str(v,256)&&v.length>0&&!/[\r\n]/.test(v);
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const finite=(v:unknown):v is number=>typeof v==='number'&&Number.isFinite(v);
function bad(message=unreadableServerResponse):never{throw Object.assign(new Error(message),{code:'invalid_browse'});}
function rows(v:unknown,max:number):unknown[]{if(!Array.isArray(v)||v.length>max)bad();return v;}
function strings(v:unknown,max:number,allowed?:readonly string[]):string[]{return rows(v,max).map(entry=>{if(!id(entry)||(allowed&&!allowed.includes(entry)))bad();return entry;});}
function freeze<T>(v:T):T{if(v&&typeof v==='object'){for(const child of Object.values(v))freeze(child);Object.freeze(v);}return v;}

/** Thrown when a query names something the capabilities do not publish. The path
 * matches the server's own `error.field`, so one message renders in both cases. */
export class BrowseQueryError extends Error{
 readonly field:string;readonly code='invalid_query';
 constructor(field:string,message:string){super(message);this.field=field;this.name='BrowseQueryError';}
}

function sortSelection(v:unknown):BrowseSortSelection{
 if(!obj(v)||!id(v.field)||(v.direction!=='asc'&&v.direction!=='desc'))bad();
 return {field:v.field,direction:v.direction};
}

/** Nodes are re-validated on the way in: a malformed tree is never handed to a
 * caller as if the server had blessed it. */
export function parseBrowseNode(v:unknown,depth=0,path='query'):BrowseNode{
 if(!obj(v)||depth>16)bad();
 if('field' in v){
  if(Object.keys(v).length!==3||!id(v.field)||!str(v.operator,64)||!operators.includes(v.operator))bad();
  const operator=v.operator as BrowseOperator;
  const value=v.value;
  if(presenceOperators.includes(operator)){if(value!==null&&value!==undefined)bad();return {field:v.field,operator,value:null};}
  if(Array.isArray(value)){
   if(!listOperators.includes(operator)||value.length<1||value.length>100||(operator==='between'&&value.length!==2))bad();
   return {field:v.field,operator,value:value.map(entry=>{if(!str(entry,500)&&!finite(entry)&&typeof entry!=='boolean')bad();return entry as BrowseScalar;})};
  }
  if(listOperators.includes(operator))bad();
  if(!str(value,500)&&!finite(value)&&typeof value!=='boolean')bad();
  return {field:v.field,operator,value:value as BrowseScalar};
 }
 const keys=Object.keys(v);
 if(keys.length!==1)bad();
 if(keys[0]==='not')return {not:parseBrowseNode(v.not,depth+1,path+'.not')};
 if(keys[0]==='all'||keys[0]==='any'){
  const children=rows(v[keys[0]],100).map((child,index)=>parseBrowseNode(child,depth+1,`${path}.${keys[0]}[${index}]`));
  if(children.length===0)bad();
  return keys[0]==='all'?{all:children}:{any:children};
 }
 return bad();
}

export function parseBrowseCapabilities(raw:unknown):BrowseCapabilities{
 if(!obj(raw)||!obj(raw.library)||!obj(raw.queryLimits))bad();
 const library=raw.library;
 if(!id(library.id)||!str(library.name,512)||!id(library.kind)||!str(library.defaultView,64))bad();
 const limits=raw.queryLimits;
 for(const key of ['maximumDepth','maximumClauses','maximumBytes','maximumSorts','defaultLimit','maximumLimit','cursorTtlSeconds'])if(!count(limits[key]))bad();
 if((limits.defaultLimit as number)<1||(limits.maximumLimit as number)<(limits.defaultLimit as number))bad();
 const pivot=(v:unknown):BrowsePivotCapability=>{
  if(!obj(v)||!id(v.id)||!id(v.labelKey)||typeof v.browsable!=='boolean')bad();
  return {id:v.id,labelKey:v.labelKey,entityKinds:strings(v.entityKinds,32),defaultSort:rows(v.defaultSort,3).map(sortSelection),supportedViews:strings(v.supportedViews,8),browsable:v.browsable,...(v.aggregate===undefined?{}:{aggregate:id(v.aggregate)?v.aggregate:bad()})};
 };
 const pivots=rows(raw.pivots,32).map(pivot);
 if(new Set(pivots.map(p=>p.id)).size!==pivots.length)bad();
 const fields=rows(raw.fields,128).map(v=>{
  if(!obj(v)||!id(v.id)||!id(v.labelKey)||!str(v.type,32)||!types.includes(v.type)||!str(v.controlHint,32)||!hints.includes(v.controlHint)||!['quick','standard','advanced'].includes(v.complexity as string)||!['indexed','indexed-join'].includes(v.cost as string))bad();
  const facet=v.facetSource===undefined?undefined:(obj(v.facetSource)&&str(v.facetSource.endpoint,256)&&id(v.facetSource.field)?{endpoint:v.facetSource.endpoint,field:v.facetSource.field}:bad());
  return {id:v.id,labelKey:v.labelKey,type:v.type as BrowseValueType,operators:strings(v.operators,32,operators) as BrowseOperator[],controlHint:v.controlHint as BrowseControlHint,complexity:v.complexity as BrowseComplexity,cost:v.cost as BrowseCost,applicableKinds:strings(v.applicableKinds,64),...(v.allowedValues===undefined?{}:{allowedValues:strings(v.allowedValues,256)}),...(facet?{facetSource:facet}:{})};
 });
 if(new Set(fields.map(f=>f.id)).size!==fields.length)bad();
 const sorts=rows(raw.sorts,64).map(v=>{
  if(!obj(v)||!id(v.id)||!id(v.labelKey)||typeof v.expensive!=='boolean'||(v.defaultDirection!=='asc'&&v.defaultDirection!=='desc'))bad();
  const directions=strings(v.directions,2,['asc','desc']) as BrowseDirection[];
  if(!directions.includes(v.defaultDirection))bad();
  return {id:v.id,labelKey:v.labelKey,directions,defaultDirection:v.defaultDirection as BrowseDirection,expensive:v.expensive,applicableKinds:strings(v.applicableKinds,64)};
 });
 const quickFilters=rows(raw.quickFilters,32).map(v=>{
  if(!obj(v)||!id(v.id)||!id(v.labelKey))bad();
  return {id:v.id,labelKey:v.labelKey,query:parseBrowseNode(v.query)};
 });
 const resolved=raw.resolvedPivot===undefined||raw.resolvedPivot===null?null:pivot(raw.resolvedPivot);
 if(resolved&&!pivots.some(p=>p.id===resolved.id))bad();
 return freeze({
  library:{id:library.id,name:library.name,kind:library.kind,defaultView:library.defaultView,pinned:library.pinned===true},
  pivots,resolvedPivot:resolved,fields,sorts,quickFilters,
  queryLimits:{maximumDepth:limits.maximumDepth as number,maximumClauses:limits.maximumClauses as number,maximumBytes:limits.maximumBytes as number,maximumSorts:limits.maximumSorts as number,defaultLimit:limits.defaultLimit as number,maximumLimit:limits.maximumLimit as number,cursorTtlSeconds:limits.cursorTtlSeconds as number},
 });
}

export function parseBrowseResult(raw:unknown):BrowseResult{
 if(!obj(raw)||!id(raw.pivot)||!obj(raw.applied)||!obj(raw.pageInfo))bad();
 const info=raw.pageInfo;
 if(!count(info.start)||!count(info.total)||!id(info.revision)||typeof info.hasMore!=='boolean'||!str(info.nextCursor,4096))bad();
 if(info.start>info.total)bad();
 const entries=rows(raw.entries,200).map(validateContentEntry);
 if(new Set(entries.map(e=>e.id)).size!==entries.length)bad();
  if(info.hasMore!==(info.start+entries.length<info.total))bad();
  const positionIndex=parsePositionIndex(raw.positionIndex,info.total as number);
 const applied=raw.applied;
 return freeze({
  pivot:raw.pivot,
  applied:{
   query:applied.query===undefined||applied.query===null?null:parseBrowseNode(applied.query),
   sort:rows(applied.sort,3).map(sortSelection),
   seek:applied.seek===undefined||applied.seek===null?null:(obj(applied.seek)&&str(applied.seek.prefix,8)&&applied.seek.prefix?{prefix:applied.seek.prefix}:bad()),
  },
  entries,
  pageInfo:{start:info.start,total:info.total,revision:info.revision,hasMore:info.hasMore,nextCursor:info.nextCursor},
  positionIndex,
 });
}

/** Anchors for the position rail, on the first page of every sort (M20): letters for
 * title (either direction), years (decades past 240 anchors) for year, months (years
 * past 240) for added and lastPlayed, whole points for the ratings, half hours in
 * minutes for duration, and an empty key for rows without a value. The field is
 * optional and best-effort: a missing index, or an entry whose shape is unknown, is
 * dropped rather than failing the page. Well-shaped but incoherent anchors (out of
 * order, past the total) still fail, as before. */
function parsePositionIndex(v:unknown,total:number):BrowsePositionAnchor[]{
 if(v===undefined||v===null)return [];
 if(!Array.isArray(v))return [];
 const out:BrowsePositionAnchor[]=[];
 let previous=-1;
 for(const entry of v.slice(0,256)){
  if(!obj(entry)||!str(entry.key,8)||!count(entry.index))continue;
  const index=entry.index as number;
  if(index<=previous||index>=Math.max(total,1))bad();
  previous=index;
  out.push({key:entry.key as string,index});
 }
 return out;
}

/** US English short months for position-rail labels ("Sep 2026"). A static table, not
 * Intl: the rail labels the server's YYYY-MM bucket keys deterministically on every
 * runtime (Hermes included), and US English is the product source locale. */
const shortMonths:readonly string[]=['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];

/** Rail label for one anchor key under the given engine sort (M20). Keys are server
 * buckets, so most display as-is (letters, years like "2019", decades like "1990s");
 * months ("2026-09") read "Sep 2026", whole points ("8") read "8★", half hours in
 * minutes ("90") read "1h 30m", and an empty key reads `noValue` (the caller's
 * catalogue string, e.g. `library.anchorNoValue`). Unknown sorts and unparseable keys
 * pass through untouched, never fatal. */
export function formatBrowseAnchor(key:string,sortField:string,noValue=''):string{
 if(!key)return noValue;
 if(sortField==='communityRating'||sortField==='personalRating')return `${key}★`;
 if(sortField==='duration'){
  const minutes=Number(key);
  if(!Number.isFinite(minutes)||minutes<0)return key;
  const h=Math.floor(minutes/60),m=Math.floor(minutes%60);
  if(h>0)return m?`${h}h ${m}m`:`${h}h`;
  return `${m}m`;
 }
 if(sortField==='added'||sortField==='lastPlayed'){
  const month=/^(\d{4})-(\d{2})$/.exec(key);
  if(month){
   const mi=Number(month[2]);
   if(mi>=1&&mi<=12)return `${shortMonths[mi-1]} ${month[1]}`;
  }
 }
 return key;
}

export function parseBrowseFacets(raw:unknown,expected?:string):BrowseFacetPage{
 if(!obj(raw)||!id(raw.field)||(expected!==undefined&&raw.field!==expected))bad();
 const values=rows(raw.values,200).map(v=>{
  if(!obj(v)||!id(v.value)||!str(v.label,512)||!v.label||!count(v.count)||v.count<1)bad();
  return {value:v.value,label:v.label,count:v.count};
 });
 if(new Set(values.map(v=>v.value)).size!==values.length)bad();
 return freeze({field:raw.field,values});
}

/** Validates one node against the published vocabulary, naming the first failure
 * with the same path the server would return. */
export function validateBrowseQuery(capabilities:BrowseCapabilities,node:BrowseNode,path='query',depth=0):void{
 const limits=capabilities.queryLimits;
 if(depth>limits.maximumDepth)throw new BrowseQueryError(path,`Nest at most ${limits.maximumDepth} levels.`);
 if('field' in node){
  const field=capabilities.fields.find(f=>f.id===node.field);
  if(!field)throw new BrowseQueryError(path+'.field',`This library cannot filter on ${node.field}.`);
  if(!field.operators.includes(node.operator))throw new BrowseQueryError(path+'.operator',`${node.operator} is not available for ${node.field}.`);
  const value=node.value;
  if(presenceOperators.includes(node.operator)){
   if(value!==null)throw new BrowseQueryError(path+'.value','Presence filters take no value.');
   return;
  }
  if(field.type==='boolean'){
   if(typeof value!=='boolean')throw new BrowseQueryError(path+'.value','Choose on or off.');
   return;
  }
  const list=listOperators.includes(node.operator);
  if(list!==Array.isArray(value))throw new BrowseQueryError(path+'.value',list?'This filter takes a list of values.':'This filter takes one value.');
  const candidates=Array.isArray(value)?value:[value];
  if(list&&(candidates.length<1||candidates.length>100||(node.operator==='between'&&candidates.length!==2)))throw new BrowseQueryError(path+'.value','Choose a valid number of values.');
  for(const candidate of candidates){
   if(field.type==='number'){if(!finite(candidate))throw new BrowseQueryError(path+'.value','Enter a number.');continue;}
   if(typeof candidate!=='string'||!candidate.trim()||candidate.length>500)throw new BrowseQueryError(path+'.value','Enter a value.');
   if(field.type==='date'&&!/^\d{4}-\d{2}-\d{2}(T.*)?$/.test(candidate))throw new BrowseQueryError(path+'.value','Use a calendar date.');
   if(field.allowedValues&&!field.allowedValues.includes(candidate))throw new BrowseQueryError(path+'.value','Choose a published value.');
  }
  return;
 }
 if('not' in node){validateBrowseQuery(capabilities,node.not,path+'.not',depth+1);return;}
 const group='all' in node?'all':'any';
 const children=('all' in node?node.all:node.any);
 if(children.length<1||children.length>limits.maximumClauses)throw new BrowseQueryError(`${path}.${group}`,'Add at least one condition.');
 children.forEach((child,index)=>validateBrowseQuery(capabilities,child,`${path}.${group}[${index}]`,depth+1));
}

function countClauses(node:BrowseNode):number{
 if('field' in node)return 1;
 if('not' in node)return countClauses(node.not);
 const children='all' in node?node.all:node.any;
 return children.reduce((total,child)=>total+countClauses(child),0);
}

/** Builds one POST /v1/libraries/{id}/browse body and refuses to produce a
 * request the published capabilities cannot execute. */
export class BrowseQueryBuilder{
 private readonly capabilities:BrowseCapabilities;
 private readonly pivot:BrowsePivotCapability;
 private predicates:BrowseNode[]=[];
 private sorts:BrowseSortSelection[]=[];
 private pageLimit?:number;
 private pageCursor?:string;
 private pageRange?:BrowseRange;
 private pageSeek?:BrowseSeek;
 constructor(capabilities:BrowseCapabilities,pivotId?:string){
  this.capabilities=capabilities;
  const wanted=pivotId??capabilities.resolvedPivot?.id??capabilities.pivots.find(p=>p.browsable)?.id;
  const pivot=capabilities.pivots.find(p=>p.id===wanted);
  if(!pivot)throw new BrowseQueryError('pivot','That view is not available in this library.');
  if(!pivot.browsable)throw new BrowseQueryError('pivot','That view is not queryable.');
  this.pivot=pivot;
 }
 /** Adds one condition to the implicit conjunction. */
 where(field:string,operator:BrowseOperator,value:BrowseScalar|readonly BrowseScalar[]|null=null):this{
  const node:BrowseNode={field,operator,value};
  validateBrowseQuery(this.capabilities,node,`query.all[${this.predicates.length}]`);
  this.predicates.push(node);
  return this;
 }
 /** Adds a pre-built node, such as a published quick filter. */
 add(node:BrowseNode):this{
  validateBrowseQuery(this.capabilities,node,`query.all[${this.predicates.length}]`);
  this.predicates.push(node);
  return this;
 }
 quickFilter(quickFilterId:string):this{
  const filter=this.capabilities.quickFilters.find(f=>f.id===quickFilterId);
  if(!filter)throw new BrowseQueryError('query','That quick filter is not available.');
  return this.add(filter.query);
 }
 sort(field:string,direction?:BrowseDirection):this{
  const capability=this.capabilities.sorts.find(s=>s.id===field);
  if(!capability)throw new BrowseQueryError(`sort[${this.sorts.length}].field`,'That sort is not available here.');
  const chosen=direction??capability.defaultDirection;
  if(!capability.directions.includes(chosen))throw new BrowseQueryError(`sort[${this.sorts.length}].direction`,'That direction is not available.');
  if(this.sorts.some(entry=>entry.field===field))throw new BrowseQueryError(`sort[${this.sorts.length}].field`,'That sort is already applied.');
  if(this.sorts.length>=this.capabilities.queryLimits.maximumSorts)throw new BrowseQueryError('sort',`Use at most ${this.capabilities.queryLimits.maximumSorts} sorts.`);
  this.sorts.push({field,direction:chosen});
  return this;
 }
 limit(value:number):this{
  const limits=this.capabilities.queryLimits;
  if(!Number.isSafeInteger(value)||value<1||value>limits.maximumLimit)throw new BrowseQueryError('limit',`Ask for between 1 and ${limits.maximumLimit} entries.`);
  this.pageLimit=value;
  return this;
 }
 cursor(value:string):this{
  if(!str(value,4096)||!value)throw new BrowseQueryError('cursor','That continuation is not usable.');
  if(this.pageRange)throw new BrowseQueryError('cursor','A range and a cursor cannot be combined.');
  this.pageCursor=value;
  return this;
 }
 range(value:BrowseRange):this{
  if(!Number.isSafeInteger(value.start)||value.start<0)throw new BrowseQueryError('range.start','Start at zero or later.');
  if(this.pageCursor)throw new BrowseQueryError('range','A range and a cursor cannot be combined.');
  this.pageRange={start:value.start,...(value.revision?{revision:value.revision}:{}),...(value.anchorId?{anchorId:value.anchorId}:{})};
  return this;
 }
 /** Seeking is only meaningful under the letter index, which the server
  * publishes only for an ascending title sort. */
 seek(prefix:string):this{
  if(!prefix||prefix.length>8)throw new BrowseQueryError('seek.prefix','Choose a letter.');
  const first=this.sorts[0]??this.pivot.defaultSort[0];
  if(!first||first.field!=='title'||first.direction!=='asc')throw new BrowseQueryError('seek','Sort by title A to Z to jump to a letter.');
  this.pageSeek={prefix};
  return this;
 }
 build():BrowseRequestBody{
  const limits=this.capabilities.queryLimits;
  const query=this.predicates.length===0?undefined:this.predicates.length===1?this.predicates[0]:{all:[...this.predicates]};
  if(query&&countClauses(query)>limits.maximumClauses)throw new BrowseQueryError('query',`Use at most ${limits.maximumClauses} conditions.`);
  const body:BrowseRequestBody={pivot:this.pivot.id,...(query?{query}:{}),...(this.sorts.length?{sort:[...this.sorts]}:{}),...(this.pageLimit?{limit:this.pageLimit}:{}),...(this.pageCursor?{cursor:this.pageCursor}:{}),...(this.pageRange?{range:this.pageRange}:{}),...(this.pageSeek?{seek:this.pageSeek}:{})};
  if(JSON.stringify(body).length>limits.maximumBytes)throw new BrowseQueryError('query','That query is too large.');
  return freeze(body);
 }
}
