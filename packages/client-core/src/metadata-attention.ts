import {unreadableServerResponse} from './server-messages.ts';
export type AttentionIdentity=Readonly<{provider:'tmdb'|'tvdb'|'anilist';type:'movie'|'show'|'anime'|'episode';id:string}>;
export type AttentionCandidate=Readonly<{key:string;identity:AttentionIdentity;title:string;year:number;confidence:number;reasons:readonly string[];orders:readonly Readonly<{id:string;name:string}>[]}>;
export type AttentionEntry=Readonly<{entityId:string;kind:'item'|'show';itemKind:string;parentId:string;actions:readonly string[];title:string;revision:number;reason:string;confidence:number;candidates:readonly AttentionCandidate[]}>;
export type AttentionPage=Readonly<{libraryId:string;entries:readonly AttentionEntry[];nextCursor:string}>;
export type AttentionCommand=Readonly<{kind:'item'|'show';entityId:string;expectedRevision:number;action:'accept'|'search'|'id'|'dismiss';candidateKey?:string;provider?:AttentionIdentity['provider'];providerId?:string;query?:string;order?:string}>;
const object=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(v);
const integer=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const score=(v:unknown):v is number=>typeof v==='number'&&Number.isFinite(v)&&v>=0&&v<=1;
function invalid():never{throw new Error(unreadableServerResponse);}
export function parseMetadataAttention(raw:unknown,libraryId:string):AttentionPage {
 if(!object(raw)||raw.libraryId!==libraryId||!Array.isArray(raw.entries)||raw.entries.length>50||!text(raw.nextCursor,128))invalid();
 const entries=raw.entries.map(e=>{
  if(!object(e)||!text(e.entityId,128)||!e.entityId||!['item','show'].includes(e.kind as string)||!text(e.itemKind,64)||!text(e.parentId,128)||!Array.isArray(e.actions)||e.actions.length>5||!e.actions.every(a=>['accept','search','id','dismiss','parent'].includes(a as string))||!text(e.title,2048)||!integer(e.revision)||e.revision<1||!text(e.reason,128)||!score(e.confidence)||!Array.isArray(e.candidates)||e.candidates.length>3)invalid();
  const candidates=e.candidates.map(c=>{
   if(!object(c)||!text(c.key,64)||!/^[0-9a-f]{64}$/.test(c.key)||!object(c.identity)||!['tmdb','tvdb','anilist'].includes(c.identity.provider as string)||!['movie','show','anime','episode'].includes(c.identity.type as string)||!text(c.identity.id,16)||!/^[1-9][0-9]{0,15}$/.test(c.identity.id)||!Number.isSafeInteger(Number(c.identity.id))||(c.identity.provider==='anilist'?(c.identity.type!=='anime'||Number(c.identity.id)>2147483647):c.identity.type==='anime')||!text(c.title,2048)||!integer(c.year)||c.year>9999||!score(c.confidence)||!Array.isArray(c.reasons)||c.reasons.length>16||!c.reasons.every(v=>text(v,256))||!Array.isArray(c.orders)||c.orders.length>64)invalid();
   const orders=c.orders.map(o=>{if(!object(o)||!text(o.id,160)||!text(o.name,2048))invalid();return Object.freeze({id:o.id,name:o.name});});
   return Object.freeze({key:c.key,identity:Object.freeze({...c.identity}) as AttentionIdentity,title:c.title,year:c.year,confidence:c.confidence,reasons:Object.freeze(c.reasons as string[]),orders:Object.freeze(orders)});
  });
  return Object.freeze({entityId:e.entityId,kind:e.kind as 'item'|'show',itemKind:e.itemKind as string,parentId:e.parentId as string,actions:Object.freeze(e.actions as string[]),title:e.title,revision:e.revision,reason:e.reason,confidence:e.confidence,candidates:Object.freeze(candidates)});
 });
 if(new Set(entries.map(e=>e.entityId)).size!==entries.length)invalid();
 return Object.freeze({libraryId,entries:Object.freeze(entries),nextCursor:raw.nextCursor});
}
type Request=<T>(path:string,method?:string,body?:unknown)=>Promise<T>;
export function metadataAttentionService(request:Request,libraryId:string){
 const path=`/v1/libraries/${encodeURIComponent(libraryId)}/metadata/attention`;
 return Object.freeze({
  async list(cursor='',limit=25){return parseMetadataAttention(await request<unknown>(`${path}?cursor=${encodeURIComponent(cursor)}&limit=${limit}`),libraryId);},
  async act(command:AttentionCommand){await request<unknown>(path,'POST',command);},
 });
}
