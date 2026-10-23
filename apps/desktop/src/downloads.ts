import {invoke} from '@tauri-apps/api/core';
import {parseDownloadTitle, type DownloadTitle} from '../../../packages/client-core/src/downloads.ts';

// App-owned transport. UI owners consume the canonical web port after its MAIN landing.
export type DownloadScope = Readonly<{authority:'local'|'hosted';serverId:string;accountId:string;profileId:string;sessionFamilyId:string}>;
export type LocalDownload = Readonly<{id:string;revision:number;preparationId:string;itemId:string;quality:string;sha256:string;bytes:number;bytesDone:number;state:'queued'|'downloading'|'paused'|'failed'|'ready';held:boolean;addedMs:number;receiptExpiresMs:number|null;title:DownloadTitle}>;
export type DownloadRules = Readonly<{removeWatched:boolean;nextEpisode:boolean;unmeteredOnly:boolean;limitBytes:number}>;
export type RulesSnapshot = Readonly<{revision:number;rules:DownloadRules}>;
export type DownloadCursor = Readonly<{addedMs:number;id:string}>;
export type DownloadPage = Readonly<{items:readonly LocalDownload[];nextCursor:DownloadCursor|null}>;
export type Capacity = Readonly<{totalBytes:number;freeBytes:number;usedBytes:number;otherDownloadsBytes:number}>;
export type NativeDownloadsBridge = Readonly<{
  bind(scope:DownloadScope):Promise<string>;
  release(lease:string):Promise<void>;
  command(lease:string,operation:Readonly<Record<string,unknown>>):Promise<unknown>;
}>;
export const nativeDownloadsBridge:NativeDownloadsBridge = Object.freeze({
  bind:scope=>invoke<string>('desktop_downloads_bind',{scope}),
  release:lease=>invoke<void>('desktop_downloads_release',{lease}),
  command:(lease,operation)=>invoke<unknown>('desktop_downloads_command',{lease,operation}),
});
const fixedCodes = new Set(['downloads_unavailable','downloads_conflict','download_not_ready','download_verification_failed','invalid_receipt','storage_full','invalid_command','forbidden_origin','unsupported']);
export function downloadFailure(error:unknown):Error {
  const code=typeof error==='string'?error:error instanceof Error?error.message:'';
  return new Error(fixedCodes.has(code)?code:'downloads_unavailable');
}
function invalid():never {throw new Error('downloads_unavailable');}
function object(value:unknown):Record<string,unknown>{if(!value||typeof value!=='object'||Array.isArray(value))return invalid();return value as Record<string,unknown>;}
function integer(value:unknown):number{if(typeof value!=='number'||!Number.isSafeInteger(value)||value<0)return invalid();return value;}
function text(value:unknown,max=1024):string{if(typeof value!=='string'||value.length>max||/[\x00-\x1f\x7f]/.test(value))return invalid();return value;}
function id(value:unknown):string{const v=text(value,160);if(!/^[A-Za-z0-9_-]+$/.test(v))return invalid();return v;}
function digest(value:unknown):string{const v=text(value,64);if(!/^[a-f0-9]{64}$/.test(v))return invalid();return v;}
function boolean(value:unknown):boolean{if(typeof value!=='boolean')return invalid();return value;}
export function parseLocalDownload(value:unknown):LocalDownload {
  const v=object(value),state=text(v.state);
  if(!['queued','downloading','paused','failed','ready'].includes(state))return invalid();
  const bytes=integer(v.bytes),bytesDone=integer(v.bytesDone);
  if(!bytes||bytesDone>bytes)return invalid();
  return Object.freeze({id:digest(v.id),revision:integer(v.revision),preparationId:id(v.preparationId),itemId:id(v.itemId),quality:id(v.quality),sha256:digest(v.sha256),bytes,bytesDone,state:state as LocalDownload['state'],held:boolean(v.held),addedMs:integer(v.addedMs),receiptExpiresMs:v.receiptExpiresMs===null?null:integer(v.receiptExpiresMs),title:parseDownloadTitle(v.title)});
}
function rules(value:unknown):RulesSnapshot{
  const v=object(value),r=object(v.rules);
  return Object.freeze({revision:integer(v.revision),rules:Object.freeze({removeWatched:boolean(r.removeWatched),nextEpisode:boolean(r.nextEpisode),unmeteredOnly:boolean(r.unmeteredOnly),limitBytes:integer(r.limitBytes)})});
}
function page(value:unknown):DownloadPage{
  const v=object(value);
  if(!Array.isArray(v.items)||v.items.length>200)return invalid();
  const cursor=v.nextCursor===null?null:object(v.nextCursor);
  return Object.freeze({items:Object.freeze(v.items.map(parseLocalDownload)),nextCursor:cursor?Object.freeze({addedMs:integer(cursor.addedMs),id:digest(cursor.id)}):null});
}
function capacity(value:unknown):Capacity{
  const v=object(value),totalBytes=integer(v.totalBytes),freeBytes=integer(v.freeBytes);
  if(freeBytes>totalBytes)return invalid();
  return Object.freeze({totalBytes,freeBytes,usedBytes:integer(v.usedBytes),otherDownloadsBytes:integer(v.otherDownloadsBytes)});
}
function aborted():never{throw new DOMException('Aborted','AbortError');}
function checkScope(scope:DownloadScope):DownloadScope{
  if(!['local','hosted'].includes(scope.authority))return invalid();
  return Object.freeze({authority:scope.authority,serverId:id(scope.serverId),accountId:id(scope.accountId),profileId:id(scope.profileId),sessionFamilyId:id(scope.sessionFamilyId)});
}

/** A captured viewer/family owns one native lease. Late results are never republished. */
export class DesktopDownloadStore {
  readonly scope:DownloadScope;
  private lease='';
  private disposed=false;
  private readonly isCurrent:()=>boolean;
  private readonly bridge:NativeDownloadsBridge;
  private constructor(scope:DownloadScope,isCurrent:()=>boolean,bridge:NativeDownloadsBridge){this.scope=checkScope(scope);this.isCurrent=isCurrent;this.bridge=bridge;}
  static async bind(scope:DownloadScope,isCurrent:()=>boolean,bridge:NativeDownloadsBridge=nativeDownloadsBridge,signal?:AbortSignal):Promise<DesktopDownloadStore>{
    const store=new DesktopDownloadStore(scope,isCurrent,bridge);store.guard(signal);
    try{store.lease=digest(await bridge.bind(store.scope));store.guard(signal);return store;}
    catch(error){store.dispose();if(signal?.aborted||!isCurrent())return aborted();throw downloadFailure(error);}
  }
  private guard(signal?:AbortSignal){if(this.disposed||!this.isCurrent()||signal?.aborted)aborted();}
  async request<T>(operation:Readonly<Record<string,unknown>>,kind:string,parse:(value:unknown)=>T,signal?:AbortSignal):Promise<T>{
    this.guard(signal);
    try{
      const raw=await this.bridge.command(this.lease,operation);this.guard(signal);
      const reply=object(raw);if(reply.kind!==kind)return invalid();
      return parse(reply.value);
    }catch(error){this.guard(signal);throw downloadFailure(error);}
  }
  list(cursor:DownloadCursor|null=null,signal?:AbortSignal){return this.request({action:'list',cursor,limit:100},'page',page,signal);}
  entry(entryId:string,signal?:AbortSignal){return this.request({action:'entry',id:digest(entryId)},'entry',parseLocalDownload,signal);}
  capacity(signal?:AbortSignal){return this.request({action:'capacity'},'capacity',capacity,signal);}
  rules(signal?:AbortSignal){return this.request({action:'rules'},'rules',rules,signal);}
  setRules(expectedRevision:number,value:DownloadRules,signal?:AbortSignal){return this.request({action:'setRules',expectedRevision:integer(expectedRevision),value},'rules',rules,signal);}
  enroll(input:Readonly<{preparationId:string;itemId:string;quality:string;sha256:string;bytes:number;title:DownloadTitle}>,signal?:AbortSignal){
    const {durationSeconds,...title}=input.title;
    return this.request({action:'enroll',...input,title:{...title,durationMs:Math.round(durationSeconds*1000)}},'entry',parseLocalDownload,signal);
  }
  append(entry:Pick<LocalDownload,'id'|'revision'|'bytesDone'>,bytes:Uint8Array,signal?:AbortSignal){
    if(!bytes.byteLength||bytes.byteLength>262144)return Promise.reject(new Error('invalid_command'));
    return this.request({action:'append',id:entry.id,expectedRevision:entry.revision,offset:entry.bytesDone,bytes:Array.from(bytes)},'entry',parseLocalDownload,signal);
  }
  seal(entry:Pick<LocalDownload,'id'|'revision'>,signal?:AbortSignal){return this.request({action:'seal',id:entry.id,expectedRevision:entry.revision},'entry',parseLocalDownload,signal);}
  hold(entry:Pick<LocalDownload,'id'|'revision'>,held:boolean,signal?:AbortSignal){return this.request({action:'hold',id:entry.id,expectedRevision:entry.revision,held},'entry',parseLocalDownload,signal);}
  retry(entry:Pick<LocalDownload,'id'|'revision'>,signal?:AbortSignal){return this.request({action:'retry',id:entry.id,expectedRevision:entry.revision},'entry',parseLocalDownload,signal);}
  remove(entry:Pick<LocalDownload,'id'|'revision'>,signal?:AbortSignal){return this.request({action:'remove',id:entry.id,expectedRevision:entry.revision},'done',()=>undefined,signal);}
  async getArtwork(entryId:string,signal?:AbortSignal):Promise<Blob|null>{
    return this.request({action:'artwork',id:digest(entryId)},'artwork',value=>{
      if(value===null)return null;const v=object(value),mime=text(v.mime,64);
      if(!['image/png','image/jpeg','image/webp'].includes(mime)||!Array.isArray(v.bytes)||v.bytes.length>4194304||!v.bytes.length||!v.bytes.every(b=>typeof b==='number'&&Number.isInteger(b)&&b>=0&&b<=255))return invalid();
      return new Blob([Uint8Array.from(v.bytes)],{type:mime});
    },signal);
  }
  saveArtwork(entry:Pick<LocalDownload,'id'|'revision'>,mime:string,bytes:Uint8Array,signal?:AbortSignal){
    if(bytes.byteLength>4194304)return Promise.reject(new Error('invalid_command'));
    return this.request({action:'saveArtwork',id:entry.id,expectedRevision:entry.revision,mime,bytes:Array.from(bytes)},'entry',parseLocalDownload,signal);
  }
  dispose(){if(this.disposed)return;this.disposed=true;const lease=this.lease;this.lease='';if(lease)void this.bridge.release(lease).catch(()=>{});}
}
