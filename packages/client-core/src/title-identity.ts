/** Titles as things in their own right: titles whose files are gone from disk,
 * saying two titles are one, and removing a library. Owner only. These routes
 * are on the typed registry, so their answers are plain JSON (no envelope). */

import {firstOfEach,keepReadable,oneOfOr} from './tolerant.ts';

export type TitleIdentityApi=Readonly<{request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>}>;

/** When missing titles are removed: `immediately`, `after_days` (see `days`) or `never`. */
export type MissingRule='immediately'|'after_days'|'never';
export type MissingTitle=Readonly<{id:string;libraryId:string;kind:string;title:string;parentTitle?:string;year?:number;
 /** When the title's last file went (RFC 3339). */missingSince:string;
 /** When the owner's rule will remove it; absent when the rule is `never`. */removedAt?:string}>;
export type MissingTitlesPage=Readonly<{items:readonly MissingTitle[];nextCursor?:string;total:number;rule:MissingRule;days:number}>;
/** What one pass of emptying removed. Call again while `more` is true. */
export type MissingEmptied=Readonly<{titles:number;files:number;more:boolean}>;

const obj=(v:unknown):v is Record<string,unknown>=>typeof v==='object'&&v!==null&&!Array.isArray(v);
const text=(v:unknown,max=1024):v is string=>typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const id=(v:unknown):v is string=>text(v,256)&&v.length>0;
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const rules:readonly MissingRule[]=['immediately','after_days','never'];
function invalid(what:string):never{throw new Error('Unreadable server response: '+what);}

/**
 * The page size, the cursor and the total stay exact: the total is what the owner types to remove
 * them all. A title this app cannot read is left out of the list alone and a repeated one keeps its
 * first row. A rule this app does not know reads as `never`; each row still says when it goes.
 */
export function parseMissingTitlesPage(v:unknown):MissingTitlesPage{
 if(!obj(v)||!Array.isArray(v.items)||v.items.length>200||!count(v.total)||!count(v.days))invalid('missing titles');
 if(v.nextCursor!==undefined&&!text(v.nextCursor,256))invalid('missing titles cursor');
 const items=firstOfEach('missing-titles','items',keepReadable('missing-titles','items',v.items,entry=>{
  if(!obj(entry)||!id(entry.id)||!id(entry.libraryId)||!id(entry.kind)||!text(entry.title)||!text(entry.missingSince,64)||!entry.missingSince)invalid('missing title');
  if(entry.parentTitle!==undefined&&!text(entry.parentTitle)||entry.year!==undefined&&!count(entry.year)||entry.removedAt!==undefined&&!text(entry.removedAt,64))invalid('missing title');
  return Object.freeze({id:entry.id,libraryId:entry.libraryId,kind:entry.kind,title:entry.title,...(entry.parentTitle?{parentTitle:entry.parentTitle as string}:{}),...(entry.year?{year:entry.year as number}:{}),missingSince:entry.missingSince,...(entry.removedAt?{removedAt:entry.removedAt as string}:{})});
 }),entry=>entry.id);
 return Object.freeze({items:Object.freeze(items),...(v.nextCursor?{nextCursor:v.nextCursor as string}:{}),total:v.total,rule:oneOfOr('missing-titles','rule',v.rule,rules,'never'),days:v.days});
}
export function parseMissingEmptied(v:unknown):MissingEmptied{
 if(!obj(v)||!count(v.titles)||!count(v.files)||typeof v.more!=='boolean')invalid('emptied missing titles');
 return Object.freeze({titles:v.titles,files:v.files,more:v.more});
}

/** One page of missing titles, oldest first. `libraryId` omitted lists every library. */
export async function listMissingTitles(api:TitleIdentityApi,input:{libraryId?:string;cursor?:string;limit?:number}={},signal?:AbortSignal):Promise<MissingTitlesPage>{
 const params=new URLSearchParams();
 if(input.libraryId)params.set('libraryId',input.libraryId);
 if(input.cursor)params.set('cursor',input.cursor);
 if(input.limit)params.set('limit',String(input.limit));
 const query=params.toString();
 return parseMissingTitlesPage(await api.request<unknown>('/v1/admin/missing-titles'+(query?'?'+query:''),'GET',undefined,signal));
}
/** Removes missing titles and files now, whatever the owner's rule says. The server works in
 * short passes; this repeats until nothing is left and returns the totals. Watch state is kept. */
export async function emptyMissingTitles(api:TitleIdentityApi,libraryId?:string,signal?:AbortSignal):Promise<MissingEmptied>{
 let titles=0,files=0;
 for(let pass=0;pass<1000;pass++){
  const step=parseMissingEmptied(await api.request<unknown>('/v1/admin/missing-titles/empty','POST',libraryId?{libraryId}:{},signal));
  titles+=step.titles;files+=step.files;
  if(!step.more)return Object.freeze({titles,files,more:false});
 }
 return Object.freeze({titles,files,more:true});
}
/** Says two titles of one library are the same title: `fromId` becomes part of `intoId`, with its
 * files as versions and everyone's state combined. Returns the id that remains. */
export async function mergeTitles(api:TitleIdentityApi,fromId:string,intoId:string):Promise<string>{
 if(!id(fromId)||!id(intoId)||fromId===intoId)invalid('titles to merge');
 const out=await api.request<unknown>('/v1/admin/titles/merge','POST',{fromId,intoId});
 if(!obj(out)||!id(out.itemId))invalid('merged title');
 return out.itemId;
}
/** Removes a library. `confirmation` is the library's name typed back; `expectedRevision` its
 * configuration revision. The library disappears at once; the rest happens in the background.
 * Files on disk are not touched and watch state is kept in case its titles return. */
export async function removeLibrary(api:TitleIdentityApi,libraryId:string,confirmation:string,expectedRevision:number):Promise<void>{
 if(!id(libraryId)||!text(confirmation,200)||!count(expectedRevision))invalid('library to remove');
 const out=await api.request<unknown>('/v1/admin/libraries/'+encodeURIComponent(libraryId),'DELETE',{confirmation,expectedRevision});
 if(!obj(out)||out.removing!==true)invalid('library removal');
}
