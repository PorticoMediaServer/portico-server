import {unreadableServerResponse} from './server-messages.ts';
/**
 * Container personal state (`server/api/jobs.openapi.yaml`): an O(1) watched default for a show, season, album or book,
 * and the title's own Watchlist and Favorite flags (a show can be on the Watchlist; a show, album,
 * book or artist can be a Favorite), so a show's action row matches a movie's.
 *
 * `GET /v1/containers/{kind}/{id}/personal-state` reads the revision the
 * approved PUT contract requires, plus the viewer's visible-member counts.
 * `PUT` writes one profile/container watermark with `{expectedRevision,
 * watched}`; a 409 `revision_mismatch` means reread before authoring another
 * intent. Explicit item watched intent overrides the default; the latest
 * applicable container watermark wins (season wins a timestamp tie); items
 * added after the watermark appear unwatched.
 */

export type ContainerKind='show'|'season'|'album'|'book'|'artist';
/** The saved flags a title carries besides watched. */
export type ContainerFlag='watchlisted'|'favorite';
export type ContainerState=Readonly<{
 revision:number;watched:boolean;watermark:string;watchlisted:boolean;favorite:boolean;
 /** Present on GET: the viewer's visible members only, never library-wide summaries. */
 watchedCount?:number;unwatchedCount?:number;
 /** Present on GET: the library the title lives in, so a link with only its id can open its page. */
 libraryId?:string;
}>;
export interface ContainerStateApi{request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>}

export const containerKinds:readonly ContainerKind[]=Object.freeze(['show','season','album','book','artist'] as const);
/** Which kinds carry a flag: My List (`watchlisted`) is for shows and books; Favorite for shows, albums, books and artists. */
export function containerCarries(kind:string|undefined,flag:ContainerFlag|'watched'):boolean{
 if(flag==='watched')return kind==='show'||kind==='season'||kind==='album'||kind==='book';
 if(flag==='watchlisted')return kind==='show'||kind==='book';
 return kind==='show'||kind==='album'||kind==='book'||kind==='artist';
}
/** Shared error-code correction: a stale revision is HTTP 409 (see `bulk-jobs.ts`). */
export const CONTAINER_REVISION_MISMATCH='revision_mismatch';

const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const id=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&v.length<=256&&!/[\x00-\x1f\x7f]/.test(v);
const count=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;

function invalid():never{throw new Error(unreadableServerResponse);}

function checkKind(kind:string):ContainerKind{
 if(!containerKinds.includes(kind as ContainerKind))throw new Error('Container state covers shows, seasons, albums, books and artists.');
 return kind as ContainerKind;
}

function parse(raw:unknown,withCounts:boolean):ContainerState{
 if(!object(raw))invalid();
 if(!count(raw.revision)||typeof raw.watched!=='boolean'||typeof raw.watermark!=='string'||raw.watermark.length>128)invalid();
 if(Object.keys(raw).some(k=>k!=='revision'&&k!=='watched'&&k!=='watermark'&&k!=='watchlisted'&&k!=='favorite'&&k!=='watchedCount'&&k!=='unwatchedCount'&&k!=='libraryId'))invalid();
 if(raw.libraryId!==undefined&&!id(raw.libraryId))invalid();
 if(raw.watchlisted!==undefined&&typeof raw.watchlisted!=='boolean'||raw.favorite!==undefined&&typeof raw.favorite!=='boolean')invalid();
 const flags={watchlisted:raw.watchlisted===true,favorite:raw.favorite===true};
 if(withCounts){
  if(!count(raw.watchedCount)||!count(raw.unwatchedCount))invalid();
  return Object.freeze({revision:raw.revision as number,watched:raw.watched as boolean,watermark:raw.watermark as string,...flags,watchedCount:raw.watchedCount as number,unwatchedCount:raw.unwatchedCount as number,...(raw.libraryId?{libraryId:raw.libraryId as string}:{})});
 }
 return Object.freeze({
  revision:raw.revision as number,watched:raw.watched as boolean,watermark:raw.watermark as string,...flags,
  ...(raw.watchedCount===undefined?{}:{watchedCount:raw.watchedCount as number}),
  ...(raw.unwatchedCount===undefined?{}:{unwatchedCount:raw.unwatchedCount as number}),
 });
}

/** Reads the revision and the current visible-member detail counts. */
export async function getContainerState(api:ContainerStateApi,kind:ContainerKind,containerId:string,signal?:AbortSignal):Promise<ContainerState>{
 const checked=checkKind(kind);
 if(!id(containerId))throw new Error('A container id is required.');
 return parse(await api.request<unknown>('/v1/containers/'+checked+'/'+encodeURIComponent(containerId)+'/personal-state','GET',undefined,signal),true);
}

/**
 * Writes the container watched default. On a 409 `revision_mismatch`, reread
 * with `getContainerState` before authoring another intent.
 */
export async function setContainerState(api:ContainerStateApi,kind:ContainerKind,containerId:string,input:Readonly<{expectedRevision:number;watched:boolean}>,signal?:AbortSignal):Promise<ContainerState>{
 const checked=checkKind(kind);
 if(!id(containerId))throw new Error('A container id is required.');
 if(!count(input.expectedRevision)||typeof input.watched!=='boolean')throw new Error('Container watched needs the current revision and the watched value.');
 try{
  return parse(await api.request<unknown>(
   '/v1/containers/'+checked+'/'+encodeURIComponent(containerId)+'/personal-state','PUT',
   {expectedRevision:input.expectedRevision,watched:input.watched},signal),false);
 }catch(e){
  const v=e as {status?:unknown;code?:unknown};
  const code=typeof v?.code==='string'&&v.code?v.code:v?.status===409?CONTAINER_REVISION_MISMATCH:'container_state_not_saved';
  throw Object.assign(new Error(e instanceof Error?e.message:'The watched state could not be saved.'),{code});
 }
}

/** Sets the title's Watchlist or Favorite flag. Plain values: the last write wins, so there is no revision to reread. */
export async function setContainerFlag(api:ContainerStateApi,kind:ContainerKind,containerId:string,flag:ContainerFlag,value:boolean,signal?:AbortSignal):Promise<ContainerState>{
 const checked=checkKind(kind);
 if(!id(containerId))throw new Error('A container id is required.');
 if(!containerCarries(checked,flag)||typeof value!=='boolean')throw new Error('This title does not carry that flag.');
 try{
  return parse(await api.request<unknown>('/v1/containers/'+checked+'/'+encodeURIComponent(containerId)+'/personal-state','PUT',{expectedRevision:0,[flag]:value},signal),false);
 }catch(e){
  const v=e as {code?:unknown};
  throw Object.assign(new Error(e instanceof Error?e.message:'The change could not be saved.'),{code:typeof v?.code==='string'&&v.code?v.code:'container_state_not_saved'});
 }
}

/** Detail copy for the visible watched/unwatched counts (no list/grid badges). */
export function containerCountLabel(watchedCount:number,unwatchedCount:number):{watched:number;unwatched:number}{
 if(!count(watchedCount)||!count(unwatchedCount))throw new Error('Invalid container counts.');
 return {watched:watchedCount,unwatched:unwatchedCount};
}
