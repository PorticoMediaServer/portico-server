import {routeBody} from "../../contracts/src/routes.ts";
import type {RouteResponse} from "../../contracts/src/routes.ts";
import {routePath, encodedRoutePath} from "../../contracts/src/routes.ts";
/** Owner-only source metadata and operation recovery. Credentials exist only in an
 * explicit in-flight request, never in snapshots, storage, URLs or retry queues. */
import type {ContentScope,LibraryContentApi} from './library-content.ts';
import {firstOfEach,keepReadable,noteSkipped,oneOfOr} from './tolerant.ts';
import {rebaseDraft} from './freshness-actions.ts';
export type RemoteSource=Readonly<{id:string;kind:'rclone'|'webdav'|'sftp'|'smb';name:string;rootPath:string;generation:number;state:string;errorCode?:string;rangeSupport:'unknown'|'conditional'|'unsupported'|'sequential';origin?:string;insecureLocal:boolean;credentialPresent:boolean;mountId?:string}>;
export type DAVConfiguration=Readonly<{name:string;root:string;username:string;password:string;keepPassword:boolean;keepConnection:boolean;insecureLocal:boolean;expectedGeneration?:number}>;
export type DAVConnectionInput=Pick<DAVConfiguration,'name'|'root'|'username'|'password'|'insecureLocal'>;
export type DAVConnectionResult=Readonly<{origin:string;rangeSupport:'unknown'|'conditional'|'unsupported'}>;
/** An imported source is connection material only: a remote name and the
 * configuration that defines it. Portico supplies the program, never the owner. */
export type RcloneSourceDraft=Readonly<{name:string;remote:string;config:string}>;
export type RcloneTestInput=Readonly<{remote:string;config:string}>;
export type RcloneTestResult=Readonly<{version:string}>;
/** The bundled program's own status: what the server ships, never a chosen path. */
export type RcloneProgram=Readonly<{included:boolean;available:boolean;version:string;minimumVersion:string;errorCode:string}>;
/** The durable identity an imported source save returns; the receipt carries no
 * source body, so the list is read again for the row's display facts. */
export type RcloneReceipt=Readonly<{operationId:string;sourceId:string;generation:number;removed:boolean}>;
export type RemoteReceipt=Readonly<{operationId:string;accepted:true;removed:boolean;source:RemoteSource}>;
/** What one command was allowed to do, kept beside its operation id so a later
 * receipt (live or recovered) must still match the original target and posture. */
export type SourceCommandIntent=Readonly<{operationId:string;kind:'source'|'rclone';action:'create'|'edit'|'remove';sourceId:string|null;generation?:number}>;
export type AnalysisPolicy=Readonly<{libraryId:string;enabled:boolean;revision:number;effective?:boolean;scanTier?:string}>;
/** The read-authority part of one source's Reading settings. A missing saved profile is
 * reported as `saved:false` with the server's list/once defaults; it is never invented. */
export type SourceReadValues=Readonly<{scanDepth:'file_list_only'|'basic'|'complete';wholeFileReads:boolean;connections:number;requestsPerSecond:number;bytesPerSecond:number}>;
export type SourceReadSettings=Readonly<{sourceId:string;generation:string;revision:number;saved:boolean;values:SourceReadValues}>;
export type SourceReadPreset='list'|'once'|'everything'|'ask';
/** One folder inside a saved source during browsing: its display name, the
 * absolute path the catalog uses for it, and the path inside the source. */
export type RemoteFolder=Readonly<{name:string;path:string;relative:string;hidden:boolean}>;
export type RemoteFolderPage=Readonly<{sourceId:string;generation:string;path:string;relative:string;entries:readonly RemoteFolder[];nextCursor:string;truncated:boolean}>;
/** One library that uses folders from a source, and how many of its folders do. */
export type SourceLibraryUse=Readonly<{libraryId:string;name:string;kind:string;folders:number}>;
/** A source's present state: the reads it holds now, when it was last checked,
 * and the libraries its folders went into. Nothing here is estimated. */
export type RemoteSourceStatus=Readonly<{sourceId:string;generation:string;state:string;errorCode?:string;rangeSupport:string;lastCheckedAt:string;connectionsInUse:number;playbackReads:number;analysisReads:number;scanIntervalSeconds:number;libraries:readonly SourceLibraryUse[];librariesTruncated:boolean}>;
/** The drawn Advanced page: how a source's folders are checked, the disk its
 * read cache may use, and which analysis stages run for files found through it. */
export type SourceAdvanced=Readonly<{scanIntervalSeconds:number;watchForChanges:boolean;scanOnStart:boolean;fullRecheckDays:number;cacheBytes:number;cacheFloorBytes:number;seekPreviews:boolean;chapterImages:boolean;waveforms:boolean;loudness:boolean;checksums:boolean}>;
export type SourceAdvancedSettings=Readonly<{sourceId:string;generation:string;revision:number;saved:boolean;cacheSupported:boolean;cacheApplied:boolean;cacheError?:string;scanIntervalObserved:number;values:SourceAdvanced}>;
const sourceReadPresets:Readonly<Record<SourceReadPreset,SourceReadValues>>=Object.freeze({
 list:{scanDepth:'file_list_only',wholeFileReads:false,connections:2,requestsPerSecond:2,bytesPerSecond:0},
 once:{scanDepth:'basic',wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0},
 everything:{scanDepth:'complete',wholeFileReads:true,connections:8,requestsPerSecond:10,bytesPerSecond:0},
 ask:{scanDepth:'basic',wholeFileReads:false,connections:2,requestsPerSecond:2,bytesPerSecond:0},
});
/** The four drawn choices, exactly the server's own SourceReadPreset values. */
export function sourceReadPreset(name:SourceReadPreset):SourceReadValues{return Object.freeze({...sourceReadPresets[name]});}
export function sourceReadPresetName(values:SourceReadValues):SourceReadPreset|undefined{
 return (Object.keys(sourceReadPresets) as SourceReadPreset[]).find(name=>sameSourceRead(values,sourceReadPresets[name]));
}
function sameSourceRead(a:SourceReadValues,b:SourceReadValues):boolean{
 return a.scanDepth===b.scanDepth&&a.wholeFileReads===b.wholeFileReads&&a.connections===b.connections&&a.requestsPerSecond===b.requestsPerSecond&&a.bytesPerSecond===b.bytesPerSecond;
}
function sourceReadValues(raw:unknown):SourceReadValues{
 if(!record(raw))invalid();
 const scanDepth=raw.scanDepth, wholeFileReads=raw.wholeFileReads, connections=raw.connections, requestsPerSecond=raw.requestsPerSecond, bytesPerSecond=raw.bytesPerSecond;
 if((scanDepth!=='file_list_only'&&scanDepth!=='basic'&&scanDepth!=='complete')||typeof wholeFileReads!=='boolean'||!integer(connections)||connections<1||connections>32||!integer(requestsPerSecond)||requestsPerSecond<1||requestsPerSecond>50||!integer(bytesPerSecond)||!(bytesPerSecond===0||bytesPerSecond>=1_000_000&&bytesPerSecond<=1_000_000_000&&bytesPerSecond%1_000_000===0))invalid();
 return Object.freeze({scanDepth,wholeFileReads,connections,requestsPerSecond,bytesPerSecond});
}
function sourceReadSettings(raw:unknown):SourceReadSettings{
 if(!record(raw)||!text(raw.sourceId,256)||!raw.sourceId||!text(raw.generation,64)||!raw.generation||!integer(raw.revision)||typeof raw.saved!=='boolean')invalid();
 return Object.freeze({sourceId:raw.sourceId,generation:raw.generation,revision:raw.revision,saved:raw.saved,values:sourceReadValues(raw.values)});
}
function remoteFolder(raw:unknown):RemoteFolder{
 if(!record(raw)||!text(raw.name,255)||!raw.name||!text(raw.path)||!raw.path||!text(raw.relative)||typeof raw.hidden!=='boolean')invalid();
 return Object.freeze({name:raw.name,path:raw.path,relative:raw.relative,hidden:raw.hidden});
}
function remoteFolderPage(raw:unknown,id:string):RemoteFolderPage{
 if(!record(raw)||raw.sourceId!==id||!text(raw.generation,64)||!raw.generation||!text(raw.path)||!text(raw.relative)||!Array.isArray(raw.entries)||!text(raw.nextCursor,255)||typeof raw.truncated!=='boolean')invalid();
 return Object.freeze({sourceId:id,generation:raw.generation,path:raw.path,relative:raw.relative,entries:Object.freeze(raw.entries.map(remoteFolder)),nextCursor:raw.nextCursor,truncated:raw.truncated});
}
function sourceLibraryUse(raw:unknown):SourceLibraryUse{
 if(!record(raw)||!text(raw.libraryId,256)||!raw.libraryId||!text(raw.name,512)||!text(raw.kind,32)||!integer(raw.folders)||raw.folders<1)invalid();
 return Object.freeze({libraryId:raw.libraryId,name:raw.name,kind:raw.kind,folders:raw.folders});
}
function sourceAdvanced(raw:unknown):SourceAdvanced{
 if(!record(raw))invalid();
 const {scanIntervalSeconds,watchForChanges,scanOnStart,fullRecheckDays,cacheBytes,cacheFloorBytes,seekPreviews,chapterImages,waveforms,loudness,checksums}=raw;
 if(!integer(scanIntervalSeconds)||![0,900,3600,21600,86400,604800].includes(scanIntervalSeconds))invalid();
 if(typeof watchForChanges!=='boolean'||typeof scanOnStart!=='boolean')invalid();
 if(!integer(fullRecheckDays)||![0,7,14,30].includes(fullRecheckDays))invalid();
 if(!integer(cacheBytes)||cacheBytes>64*1024*1024*1024||!integer(cacheFloorBytes)||cacheFloorBytes>1024*1024*1024*1024)invalid();
 if(![seekPreviews,chapterImages,waveforms,loudness,checksums].every(value=>typeof value==='boolean'))invalid();
 return Object.freeze({scanIntervalSeconds,watchForChanges,scanOnStart,fullRecheckDays,cacheBytes,cacheFloorBytes,seekPreviews,chapterImages,waveforms,loudness,checksums}) as SourceAdvanced;
}
function sourceAdvancedSettings(raw:unknown,id:string):SourceAdvancedSettings{
 if(!record(raw)||raw.sourceId!==id||!text(raw.generation,64)||!raw.generation||!integer(raw.revision)||typeof raw.saved!=='boolean'||typeof raw.cacheSupported!=='boolean'||typeof raw.cacheApplied!=='boolean'||!integer(raw.scanIntervalObserved))invalid();
 if(raw.cacheError!==undefined&&!['source_changed','mount_unavailable','cache_refused','cache_record_failed'].includes(String(raw.cacheError)))invalid();
 return Object.freeze({sourceId:id,generation:raw.generation,revision:raw.revision,saved:raw.saved,cacheSupported:raw.cacheSupported,cacheApplied:raw.cacheApplied,...(raw.cacheError===undefined?{}:{cacheError:raw.cacheError as string}),scanIntervalObserved:raw.scanIntervalObserved,values:sourceAdvanced(raw.values)});
}
function remoteSourceStatus(raw:unknown,id:string):RemoteSourceStatus{
 if(!record(raw)||raw.sourceId!==id||!text(raw.generation,64)||!raw.generation||!text(raw.state,100)||!text(raw.rangeSupport,32)||!text(raw.lastCheckedAt,64)||!integer(raw.connectionsInUse)||!integer(raw.playbackReads)||!integer(raw.analysisReads)||!integer(raw.scanIntervalSeconds)||!Array.isArray(raw.libraries)||raw.libraries.length>10||typeof raw.librariesTruncated!=='boolean')invalid();
 if(raw.errorCode!==undefined&&!text(raw.errorCode,100))invalid();
 return Object.freeze({sourceId:id,generation:raw.generation,state:raw.state,...(raw.errorCode===undefined?{}:{errorCode:raw.errorCode as string}),rangeSupport:raw.rangeSupport,lastCheckedAt:raw.lastCheckedAt,connectionsInUse:raw.connectionsInUse,playbackReads:raw.playbackReads,analysisReads:raw.analysisReads,scanIntervalSeconds:raw.scanIntervalSeconds,libraries:Object.freeze(raw.libraries.map(sourceLibraryUse)),librariesTruncated:raw.librariesTruncated});
}
/** `alreadyDone`: the last command found, on a fresh read, that what it asked for is already so (a share removed elsewhere).
 * `operationKind` picks the receipt route that command's outcome is recovered from. */
export type RemoteState=Readonly<{sources:readonly RemoteSource[];loading:boolean;busy:boolean;error:string;errorCode:string;operationId:string|null;operationKind:'source'|'rclone'|'';ambiguous:boolean;receipt:RemoteReceipt|null;rclone:RcloneReceipt|null;alreadyDone:boolean;mountSupported:boolean}>;
const record=(x:unknown):x is Record<string,unknown>=>!!x&&typeof x==='object'&&!Array.isArray(x);
const text=(x:unknown,n=8192):x is string=>typeof x==='string'&&x.length<=n&&!/[\x00-\x1f\x7f]/.test(x);
/** Configuration is multi-line INI, so line feeds and tabs are the only control
 * characters it may carry; everything else is refused before it is sent. */
const configText=(x:unknown,n=65536):x is string=>typeof x==='string'&&x.length<=n&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(x);
const integer=(x:unknown):x is number=>typeof x==='number'&&Number.isSafeInteger(x)&&x>=0;
function invalid():never{throw new Error('Invalid remote source response.');}
/** Display-only saved endpoint fact. This never authorizes a connection. Native
 * schemes remain distinct from HTTP, including inside imported rclone sources. */
function sourceOrigin(value:string,kind:RemoteSource['kind']):boolean{
 const match=/^(https?|sftp|smb):\/\/([^/?#@%\\\s]+)\/?$/.exec(value);
 if(!match)return false;
 const scheme=match[1];
 if(kind==='webdav'&&scheme!=='http'&&scheme!=='https'||kind==='sftp'&&scheme!=='sftp'||kind==='smb'&&scheme!=='smb')return false;
 try{
  const url=new URL(value);
  if(!url.hostname||url.username||url.password||url.search||url.hash||url.pathname&&url.pathname!=='/')return false;
  if(scheme==='sftp'||scheme==='smb'){
   if(!/^(?:\[[0-9a-f:.]+\]|[a-z0-9.-]+):[0-9]{1,5}$/.test(match[2])||!url.port||Number(url.port)<1||Number(url.port)>65535)return false;
  }
  return true;
 }catch{return false;}
}
function source(x:unknown):RemoteSource{
 if(!record(x)||!text(x.id,256)||!x.id||!['rclone','webdav','sftp','smb'].includes(String(x.kind))||!text(x.name,512)||!text(x.rootPath)||!integer(x.generation)||x.generation<1||!text(x.state,100)||!x.state||typeof x.insecureLocal!=='boolean'||typeof x.credentialPresent!=='boolean'||x.password!==undefined||x.config!==undefined||x.username!==undefined||x.root!==undefined)invalid();
 if(x.origin!==undefined&&(!text(x.origin)||!sourceOrigin(x.origin,x.kind as RemoteSource['kind'])))invalid();
 // A state this app does not know is kept as the server named it, an error code it does not know is left off,
 // and range support it does not know reads as unknown. Credentials are never part of a read.
 const errorCode=text(x.errorCode,100)&&['source_changed','credentials_required','binary_changed','range_unsupported','invalid_configuration','cursor_expired','budget_exceeded','rate_limited','quota_exceeded','cancelled','offline'].includes(x.errorCode)?x.errorCode:undefined;if(x.errorCode!==undefined&&errorCode===undefined)noteSkipped('remote-sources','source.errorCode');
 if(x.mountId!==undefined&&!text(x.mountId,256))invalid();
 return Object.freeze({id:x.id,kind:x.kind as RemoteSource['kind'],name:x.name,rootPath:x.rootPath,generation:x.generation,state:x.state,rangeSupport:oneOfOr('remote-sources','source.rangeSupport',x.rangeSupport,['unknown','conditional','unsupported','sequential'] as const,'unknown'),insecureLocal:x.insecureLocal,credentialPresent:x.credentialPresent,...(x.origin===undefined?{}:{origin:x.origin as string}),...(errorCode===undefined?{}:{errorCode}),...(x.mountId===undefined?{}:{mountId:x.mountId as string})});
}
const messages:Record<string,string>={remote_source_changed:'This source changed. Refresh before submitting a new configuration.',storage_command_conflict:'The source changed or this operation conflicts with a previous receipt.',storage_operation_expired:'The operation expired. Check the device clock and refresh.',remote_credentials_required:'The provider rejected the credentials. The previous configuration remains active.',source_credentials_required:'The provider rejected the credentials. The previous configuration remains active.',remote_invalid_configuration:'Check the endpoint, approved network access and configuration.',remote_range_unsupported:'The server cannot safely range-read this provider representation.',remote_binary_changed:'The approved rclone executable changed. Reapprove it in source configuration.',remote_budget_exceeded:'This operation exceeded the configured safety budget.',remote_rate_limited:'The provider or server is busy. Check the original receipt before retrying.',revision_mismatch:'This source changed. Refresh before saving.',idempotency_key_reused:'This operation was already used. Refresh and try again.',request_in_progress:'Another source change is in progress. Try again in a moment.',refresh_failed:'The profile was saved, but the refreshed values could not be read.',remote_administration_unavailable:'The rclone program on this server is unavailable. Check the server, then test again.',remote_source_not_listed:'The source was saved, but the list does not agree with the receipt yet. Refresh before continuing.',unauthorized:'Owner sign-in is required.',forbidden:'This viewer cannot manage sources.'};
const CONFLICT='storage_command_conflict';
const ERROR_CODES=new Set(['remote_source_changed','storage_command_conflict','storage_operation_expired','remote_credentials_required','remote_invalid_configuration','remote_range_unsupported','remote_binary_changed','remote_budget_exceeded','remote_rate_limited','revision_mismatch','idempotency_key_reused','request_in_progress','refresh_failed','source_credentials_required','administration_unavailable','remote_administration_unavailable','remote_source_not_listed','storage_command_capacity','storage_allocation_recovery_required','unauthorized','forbidden','not_permitted','invalid_request','internal']);
function normalizedCode(value:unknown):string{return typeof value==='string'&&ERROR_CODES.has(value)?value:'internal';}

/**
 * An edit carried onto the share as it is now, after someone else changed it. What the owner left
 * alone takes the share's new value; what they changed keeps theirs. Undefined when they changed
 * something that was also changed underneath (the one case worth telling them about). The
 * connection is one thing: its address and whether it has a password; credentials are never read.
 */
export function rebaseDavEdit(base:RemoteSource,fresh:RemoteSource,input:DAVConfiguration):DAVConfiguration|undefined{
 const connection=(x:RemoteSource)=>`${x.origin??''}|${x.credentialPresent}`;
 const mine=!input.keepConnection||!input.keepPassword;
 const shape=(x:RemoteSource)=>({name:x.name,insecureLocal:x.insecureLocal,connection:connection(x)});
 const {merged,conflict}=rebaseDraft(shape(base),{name:input.name.trim(),insecureLocal:input.insecureLocal,connection:mine?'edited':connection(base)},shape(fresh));
 if(conflict)return undefined;
 return {...input,name:merged.name,insecureLocal:merged.insecureLocal,expectedGeneration:fresh.generation};
}
function safeError(e:unknown):string{const code=record(e)&&typeof e.code==='string'?normalizedCode(e.code):'';return messages[code]??'The operation outcome is not confirmed. Check the original receipt; credentials will not be resent automatically.';}
export class RemoteSourceService{
 private state:RemoteState={sources:[],loading:false,busy:false,error:'',errorCode:'',operationId:null,operationKind:'',ambiguous:false,receipt:null,rclone:null,alreadyDone:false,mountSupported:false};
 private listeners=new Set<()=>void>();private controllers=new Set<AbortController>();private epoch=0;private read=0;private disposed=false;private viewerFence?:string;
 private intent:SourceCommandIntent|null=null;
 /** An owner refusal retires this viewer's source capability here: no later
  * request, read or command, is sent with it, and nothing already in flight can
  * publish into this viewer's snapshot. A new service reads afresh. */
 private refused=false;private refusedCode='';
 private api:LibraryContentApi;private scope:ContentScope;private operationId:()=>Promise<string>;private readonly isCurrent:()=>boolean;
 constructor(api:LibraryContentApi,scope:ContentScope,operationId:()=>Promise<string>,isCurrent:()=>boolean=()=>true){if(!scope.serverId||!scope.viewerId)throw new Error('Owner scope required.');this.api=api;this.scope=Object.freeze({...scope});this.operationId=operationId;this.isCurrent=isCurrent;}
 getSnapshot=()=>this.state;subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>this.listeners.delete(fn);};
 private live(){return !this.disposed&&this.isCurrent();}
 /** The refusal a failed transport carries, if it is one: an auth code, or a 401/403 status. */
 private refusalMessage(code:string):string{return code==='not_permitted'?messages.forbidden:(messages[code]??messages.unauthorized);}
 private refusalCode(error:unknown):string{
  const raw=record(error)?error:undefined;
  const code=raw&&typeof raw.code==='string'?normalizedCode(raw.code):'';
  if(['unauthorized','forbidden','not_permitted'].includes(code))return code;
  const status=raw?(typeof raw.status==='number'?raw.status:typeof raw.statusCode==='number'?raw.statusCode:NaN):NaN;
  if(status===401)return 'unauthorized';
  if(status===403)return 'forbidden';
  return '';
 }
 /** A refusal retires the capability at once: later reads and commands are
  * refused, in-flight transports are aborted, and the snapshot is emptied before
  * any captured UI handler reports it. Nothing publishes afterwards. */
 private retire(code:string){
  if(this.refused)return;
  this.refused=true;this.refusedCode=code;
  this.intent=null;
  this.epoch++;
  for(const c of this.controllers)c.abort();
  this.controllers.clear();
  this.state=Object.freeze({...this.state,sources:[],loading:false,busy:false,receipt:null,rclone:null,operationId:null,operationKind:'',ambiguous:false,alreadyDone:false,error:this.refusalMessage(code),errorCode:code});
  for(const fn of this.listeners)fn();
 }
 private refuse(error:unknown){const code=this.refusalCode(error);if(code)this.retire(code);}
 private publish(value:Partial<RemoteState>){if(this.refused||!this.live())return;this.state=Object.freeze({...this.state,...value});for(const fn of this.listeners)fn();}
 dispose(){this.epoch++;this.disposed=true;this.intent=null;this.refused=false;this.refusedCode='';for(const c of this.controllers)c.abort();this.controllers.clear();this.listeners.clear();this.state={...this.state,sources:[],receipt:null,rclone:null,operationId:null,operationKind:''};}
 private fence(raw:unknown){if(!record(raw)||!record(raw.scope)||raw.scope.serverId!==this.scope.serverId||!text(raw.scope.viewerFence,256)||!raw.scope.viewerFence||this.viewerFence&&this.viewerFence!==raw.scope.viewerFence)invalid();this.viewerFence=raw.scope.viewerFence;}
 private async request(path:string,method='GET',body?:unknown,guard?:()=>boolean):Promise<unknown>{
  if(this.refused)throw Object.assign(new Error(messages[this.refusedCode]??messages.unauthorized),{code:this.refusedCode||'unauthorized'});
  if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
  const c=new AbortController();this.controllers.add(c);let timer:ReturnType<typeof setTimeout>|undefined;
  try{
   return await Promise.race([this.api.request(path,method,body,c.signal),new Promise<never>((_,reject)=>{timer=setTimeout(()=>{c.abort();reject(new Error('timeout'));},45000);})]);
  }catch(error){
   // One place decides that the owner has been refused, whatever the transport shape.
   const code=this.refusalCode(error);
   if(code){this.retire(code);throw Object.assign(new Error(this.refusalMessage(code)),{code});}
   throw error;
  }finally{if(timer)clearTimeout(timer);this.controllers.delete(c);}
 }
 /** The sources as the server has them, checked; `undefined` when a later read or another viewer has taken over. */
 private async readSources(epoch:number,sequence:number,guard?:()=>boolean){const raw=await this.request(routePath("get_storage_sources"),'GET',undefined,guard);if(!this.live()||(guard&&!guard())||epoch!==this.epoch||sequence!==this.read)return undefined;this.fence(raw);if(!record(raw)||!Array.isArray(raw.sources)||typeof raw.mountSupported!=='boolean')invalid();const rows=firstOfEach('remote-sources','sources',keepReadable('remote-sources','sources',raw.sources,source),s=>s.id);return {sources:Object.freeze(rows),mountSupported:raw.mountSupported};}
 /**
  * Reads the sources again while they are on screen (a share that became reachable, or stopped
  * being). What is shown stays; a failure leaves it as it was and rejects. Nothing is read while
  * a command is in flight or its outcome is unknown.
  */
 async revalidate(){if(!this.live()||this.state.busy||this.state.ambiguous||this.state.loading)return;const epoch=this.epoch,sequence=++this.read;const next=await this.readSources(epoch,sequence);if(next&&!this.state.busy)this.publish({...next,error:''});}
 async load(){if(!this.live())return;const epoch=this.epoch,sequence=++this.read;this.publish({loading:true,error:''});try{const next=await this.readSources(epoch,sequence);if(!next)return;this.publish(next);}catch(e){if(epoch===this.epoch&&sequence===this.read)this.publish({error:safeError(e),sources:[],receipt:null,rclone:null,mountSupported:false});}finally{if(epoch===this.epoch&&sequence===this.read)this.publish({loading:false});}}
 private parseReceipt(raw:unknown,op:string):RemoteReceipt{this.fence(raw);if(!record(raw)||!record(raw.receipt)||raw.receipt.operationId!==op||raw.receipt.accepted!==true||typeof raw.receipt.removed!=='boolean')invalid();return Object.freeze({operationId:op,accepted:true,removed:raw.receipt.removed,source:source(raw.receipt.source)});}
 /** The imported-source receipt holds identity only. `removed` is left out when false. */
 private parseRcloneReceipt(raw:unknown,intent:SourceCommandIntent):RcloneReceipt{this.fence(raw);if(!record(raw)||!record(raw.receipt))invalid();const r=raw.receipt;
  if(r.operationId!==intent.operationId||r.accepted!==true||!text(r.sourceId,256)||!r.sourceId||!integer(r.generation)||r.generation<1||r.removed!==undefined&&typeof r.removed!=='boolean')invalid();
  const removed=r.removed===true;
  // The receipt must answer this command: its own source, its own posture, and
  // a replace that did not move the source's generation backwards.
  if(removed!==(intent.action==='remove'))invalid();
  if(intent.sourceId!==null&&r.sourceId!==intent.sourceId)invalid();
  if(intent.action==='edit'&&intent.generation!==undefined&&r.generation<intent.generation)invalid();
  return Object.freeze({operationId:intent.operationId,sourceId:r.sourceId,generation:r.generation,removed});}
 /** A confirmed rclone receipt is only published after the fresh list agrees with
  * it: the created or replaced row at that generation, or no row after a removal.
  * A list that does not agree is reported as unconfirmed, never assumed. */
 private rcloneListed(intent:SourceCommandIntent,receipt:RcloneReceipt):boolean{
  const row=this.state.sources.find(value=>value.id===receipt.sourceId);
  return intent.action==='remove'?!row:!!row&&row.generation===receipt.generation;
 }
 /** What rclone the server ships, and whether it starts now. The route carries no
  * owner scope, so this read neither sets nor checks the viewer fence. */
 async rcloneProgram():Promise<RcloneProgram>{
  const epoch=this.epoch;
  const raw=await this.request(routePath('get_storage_rclone_bundled'));
  if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');
  if(!record(raw)||typeof raw.included!=='boolean'||typeof raw.available!=='boolean'||!text(raw.version,200)||!text(raw.minimumVersion,200)||!text(raw.errorCode,100))invalid();
  return Object.freeze({included:raw.included,available:raw.available,version:raw.version,minimumVersion:raw.minimumVersion,errorCode:raw.errorCode});
 }
 /** The configuration backend types this server accepts. Facts only: no form,
  * readiness or guided schema is implied by a name in this list. */
 async rcloneProviders():Promise<readonly string[]>{
  const epoch=this.epoch;
  const raw=await this.request(routePath('get_storage_source_rclone_providers'));
  if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');
  this.fence(raw);
  if(!record(raw)||!Array.isArray(raw.providers)||raw.providers.length>64||!raw.providers.every(name=>text(name,100)&&!!name))invalid();
  return Object.freeze([...raw.providers] as string[]);
 }
 /** One explicit in-flight configuration test. It saves no source, receipt or approval. */
 async testRclone(input:RcloneTestInput,guard?:()=>boolean):Promise<RcloneTestResult>{
  if(!this.live()||(guard&&!guard())||this.state.busy||this.state.ambiguous)throw new Error('Finish or recover the current source command first.');
  if(!text(input.remote,512)||!input.remote.trim()||!configText(input.config)||!input.config.trim())throw new Error('Invalid connection fields.');
  const epoch=this.epoch;this.publish({busy:true,error:'',errorCode:''});
  try{
   const raw=await this.request(routePath('test_storage_source_rclone'),'POST',routeBody('test_storage_source_rclone',{remote:input.remote,config:input.config}),guard);
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   this.fence(raw);
   if(!record(raw))invalid();
   const result=raw.result;
   if(!record(result)||!text(result.version,200)||['remote','config','password','token'].some(key=>result[key]!==undefined))invalid();
   return Object.freeze({version:result.version});
  }catch(error){
   // A refusal keeps its normalized code for the captured UI report path.
   const refused=this.refusalCode(error);
   if(refused)throw Object.assign(new Error(this.refusalMessage(refused)),{code:refused});
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   const rawCode=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'';
   const code=['unauthorized','forbidden','not_permitted','invalid_request','source_credentials_required','administration_unavailable','internal'].includes(rawCode)?rawCode:'internal';
   const aliases:Record<string,string>={not_permitted:'forbidden',forbidden:'forbidden',invalid_request:'remote_invalid_configuration',source_credentials_required:'remote_credentials_required',administration_unavailable:'remote_administration_unavailable'};
   const message=safeError({code:aliases[code]??(code==='internal'?'remote_invalid_configuration':code)});
   this.refuse({code});
   this.publish({error:message,errorCode:code});throw Object.assign(new Error(message),{code});
  }finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
 }
 /** Saves an imported source from its configuration, or replaces one at the
  * generation the owner was shown. A conflict is returned for a fresh look and a
  * new explicit save; nothing is replayed or rebased here, and the configuration
  * is not kept after the call. */
 async configureRclone(id:string|null,input:RcloneSourceDraft,base?:RemoteSource,guard?:()=>boolean):Promise<RcloneReceipt|undefined>{
  if(!this.live()||(guard&&!guard())||this.state.busy||this.state.ambiguous)throw new Error('Finish or recover the current source command first.');
  if(id&&(!base||base.kind!=='rclone'||base.id!==id))throw new Error('This source cannot be changed now.');
  if(!text(input.name,100)||!input.name.trim()||!text(input.remote,512)||!input.remote.trim()||!configText(input.config)||!input.config.trim())throw new Error('Invalid connection fields.');
  const body=id
   ? routeBody('update_storage_source_rclone',{name:input.name,remote:input.remote,config:input.config,expectedGeneration:base!.generation})
   : routeBody('create_storage_source_rclone',{name:input.name,remote:input.remote,config:input.config});
  const intent:SourceCommandIntent={operationId:'',kind:'rclone',action:id?'edit':'create',sourceId:id,...(id?{generation:base!.generation}:{})};
  const path=id?encodedRoutePath('update_storage_source_rclone',{id:encodeURIComponent(id)}):routePath('create_storage_source_rclone');
  return await this.command(path,id?'PUT':'POST',body,intent,guard) as RcloneReceipt|undefined;
 }
 /** Removes an imported source at the generation the owner was shown. A conflict
  * is returned for a fresh look; nothing is replayed. */
 async removeRclone(row:RemoteSource,guard?:()=>boolean):Promise<RcloneReceipt|undefined>{
  if(!this.live()||(guard&&!guard())||row.kind!=='rclone'||this.state.busy||this.state.ambiguous)throw new Error('This source cannot be removed now.');
  const path=encodedRoutePath('remove_storage_source_rclone',{id:encodeURIComponent(row.id)});
  const body=routeBody('remove_storage_source_rclone',{expectedGeneration:row.generation});
  const intent:SourceCommandIntent={operationId:'',kind:'rclone',action:'remove',sourceId:row.id};
  return await this.command(path,'DELETE',body,intent,guard) as RcloneReceipt|undefined;
 }
 /** A new-source test uses one explicit in-flight credential request. It saves no source or retry receipt. */
 async testConnection(input:DAVConnectionInput,guard?:()=>boolean):Promise<DAVConnectionResult>{
  if(!this.live()||(guard&&!guard())||this.state.busy||this.state.ambiguous)throw new Error('Finish or recover the current source command first.');
  if(!text(input.name,100)||!input.name.trim()||!text(input.root)||!text(input.username,1024)||!text(input.password)||typeof input.insecureLocal!=='boolean')throw new Error('Invalid connection fields.');
  const epoch=this.epoch;this.publish({busy:true,error:'',errorCode:''});
  try{
   const raw=await this.request(routePath('test_storage_source_webdav'),'POST',routeBody('test_storage_source_webdav',{name:input.name,root:input.root,username:input.username,password:input.password,insecureLocal:input.insecureLocal}),guard);
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   this.fence(raw);
   if(!record(raw)||!record(raw.result))invalid();
   const result=raw.result;
   if(!text(result.origin)||!sourceOrigin(result.origin,'webdav')||!['unknown','conditional','unsupported'].includes(String(result.rangeSupport))||['password','username','config','root'].some(key=>result[key]!==undefined))invalid();
   return Object.freeze({origin:result.origin,rangeSupport:result.rangeSupport as DAVConnectionResult['rangeSupport']});
  }catch(error){
   const refused=this.refusalCode(error);
   if(refused)throw Object.assign(new Error(this.refusalMessage(refused)),{code:refused});
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   const rawCode=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'';
   const code=['unauthorized','forbidden','not_permitted','invalid_request','source_credentials_required','internal'].includes(rawCode)?rawCode:'internal';
   const aliases:Record<string,string>={not_permitted:'forbidden',invalid_request:'remote_invalid_configuration',source_credentials_required:'remote_credentials_required'};
   const message=safeError({code:aliases[code]??(code==='internal'?'remote_invalid_configuration':code)});
   this.refuse({code});
   this.publish({error:message,errorCode:code});throw Object.assign(new Error(message),{code});
  }finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
 }
 /**
  * Adds a share (`id` null) or changes one. `base` is the share as the edit started from it: when
  * someone else changed the share meanwhile, it is read again and the edit, carried onto it field by
  * field, is sent once more without a word; only a field both changed is a conflict.
  */
 async configure(id:string|null,input:DAVConfiguration,base?:RemoteSource,guard?:()=>boolean){if(!this.live()||(guard&&!guard())||this.state.busy||this.state.ambiguous)throw new Error('Finish or recover the current source command first.');if(!text(input.name,100)||!input.name.trim()||!text(input.root)||!text(input.username,1024)||!text(input.password))throw new Error('Invalid connection fields.');
  const path=routePath("post_storage_sources_webdav")+(id?'/'+encodeURIComponent(id):'');
  await this.command(path,id?'PUT':'POST',input,{operationId:'',kind:'source',action:id?'edit':'create',sourceId:id},guard);
  if(!id||!base||this.state.errorCode!==CONFLICT)return;
  const fresh=await this.reread(id,guard);if(!fresh)return;
  const rebased=rebaseDavEdit(base,fresh,input);if(!rebased)return this.publish({error:safeError({code:CONFLICT}),errorCode:CONFLICT});
  return this.command(path,'PUT',rebased,{operationId:'',kind:'source',action:'edit',sourceId:id},guard);}
 /** Removes a share. A share changed elsewhere is still the share the owner meant: it is read again and removed; one already gone is done. */
 async remove(row:RemoteSource,guard?:()=>boolean){if(!this.live()||(guard&&!guard())||row.kind!=='webdav'||this.state.busy||this.state.ambiguous)throw new Error('This source cannot be removed now.');
  const path=encodedRoutePath("delete_storage_sources_webdav_id", {"id": encodeURIComponent(row.id)});
  await this.command(path,'DELETE',{expectedGeneration:row.generation},{operationId:'',kind:'source',action:'remove',sourceId:row.id},guard);
  if(this.state.errorCode!==CONFLICT)return;
  const fresh=await this.reread(row.id,guard);if(fresh===null)return this.publish({error:'',errorCode:'',alreadyDone:true});if(!fresh)return;
  return this.command(path,'DELETE',{expectedGeneration:fresh.generation},{operationId:'',kind:'source',action:'remove',sourceId:row.id},guard);}
 /** After a conflict: the share as the server has it now, null when it is gone, undefined when it could not be read (the conflict stays as it was). */
 private async reread(id:string,guard?:()=>boolean):Promise<RemoteSource|null|undefined>{const epoch=this.epoch,sequence=++this.read;try{const next=await this.readSources(epoch,sequence,guard);if(!next||(guard&&!guard()))return undefined;this.publish(next);return next.sources.find(s=>s.id===id)??null;}catch{return undefined;}}
 private async command(path:string,method:string,body:unknown,intent:SourceCommandIntent,guard?:()=>boolean):Promise<RemoteReceipt|RcloneReceipt|undefined>{
  const epoch=this.epoch;
  this.publish({busy:true,error:'',errorCode:'',receipt:null,rclone:null,alreadyDone:false,operationId:null,operationKind:intent.kind});
  let submitted=false;
  try{
   const op=await this.operationId();
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;
   if(!/^\d{13}-[0-9a-f-]{36}$/i.test(op))invalid();
   const bound=Object.freeze({...intent,operationId:op});
   this.intent=bound;
   this.publish({operationId:op});submitted=true;
   const pending=this.request(path,method,{...(body as object),operationId:op},guard);body=undefined;
   const raw=await pending;
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;
   const receipt=intent.kind==='rclone'?this.parseRcloneReceipt(raw,bound):this.parseReceipt(raw,op);
   if(guard&&!guard())return;
   if(intent.kind==='source')this.publish({receipt:receipt as RemoteReceipt,rclone:null,ambiguous:false});
   await this.load();
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;
   if(intent.kind==='rclone'){
    // The fresh list is read first: a receipt the list cannot account for is not published.
    if(!this.rcloneListed(bound,receipt as RcloneReceipt)){this.publish({error:safeError({code:'remote_source_not_listed'}),errorCode:'remote_source_not_listed',ambiguous:false,rclone:null});return;}
    this.publish({rclone:receipt as RcloneReceipt,receipt:null,ambiguous:false});
   }
   return receipt;
  }catch(e){
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;
   const code=record(e)&&typeof e.code==='string'?normalizedCode(e.code):'';
   const rejected=['storage_command_conflict','storage_operation_expired','storage_command_capacity','storage_allocation_recovery_required','remote_invalid_configuration','remote_credentials_required','source_credentials_required','administration_unavailable','request_in_progress','unauthorized','forbidden','not_permitted','invalid_request'].includes(code);
   this.refuse({code});
   this.publish({ambiguous:submitted&&!rejected,error:safeError(e),errorCode:code});
  }finally{body=undefined;if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
 }
 async recover(){
  const intent=this.intent;
  if(!this.live()||this.state.busy||!this.state.operationId||!intent||intent.operationId!==this.state.operationId)throw new Error('No source operation to recover.');
  const op=intent.operationId,epoch=this.epoch;
  this.publish({busy:true,error:''});
  try{
   const path=intent.kind==='rclone'?encodedRoutePath('get_storage_source_rclone_operation',{id:encodeURIComponent(op)}):encodedRoutePath('get_storage_source_operations_id',{id:encodeURIComponent(op)});
   const raw=await this.request(path);
   if(!this.live()||epoch!==this.epoch)return;
   const receipt=intent.kind==='rclone'?this.parseRcloneReceipt(raw,intent):this.parseReceipt(raw,op);
   if(intent.kind==='source')this.publish({receipt:receipt as RemoteReceipt,rclone:null,ambiguous:false});
   await this.load();
   if(!this.live()||epoch!==this.epoch)return;
   if(intent.kind==='rclone'){
    if(!this.rcloneListed(intent,receipt as RcloneReceipt)){this.publish({error:safeError({code:'remote_source_not_listed'}),errorCode:'remote_source_not_listed',ambiguous:false,rclone:null});return;}
    this.publish({rclone:receipt as RcloneReceipt,receipt:null,ambiguous:false});
   }
  }catch(e){if(epoch===this.epoch)this.publish({ambiguous:true,error:safeError(e)});}
  finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
 }
 async policy(libraryId:string):Promise<AnalysisPolicy>{const epoch=this.epoch,raw=await this.request(encodedRoutePath("get_libraries_id_strm_analysis_policy", {"id": encodeURIComponent(libraryId)}));if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');return this.parsePolicy(raw,libraryId);}
 private parsePolicy(raw:unknown,id:string):AnalysisPolicy{this.fence(raw);if(!record(raw)||!record(raw.policy)||raw.policy.libraryId!==id||typeof raw.policy.enabled!=='boolean'||!integer(raw.policy.revision))invalid();if(raw.policy.effective!==undefined&&typeof raw.policy.effective!=='boolean')invalid();const scanTier=['file_list_only','basic','complete','custom'].includes(String(raw.policy.scanTier))?raw.policy.scanTier as string:undefined;if(raw.policy.scanTier!==undefined&&scanTier===undefined)noteSkipped('remote-sources','policy.scanTier');return Object.freeze({libraryId:id,enabled:raw.policy.enabled,revision:raw.policy.revision,...(raw.policy.effective===undefined?{}:{effective:raw.policy.effective as boolean}),...(scanTier===undefined?{}:{scanTier})});}
 async setPolicy(policy:AnalysisPolicy,enabled:boolean):Promise<AnalysisPolicy>{const epoch=this.epoch,raw=await this.request(encodedRoutePath("put_libraries_id_strm_analysis_policy", {"id": encodeURIComponent(policy.libraryId)}),'PUT',routeBody("put_libraries_id_strm_analysis_policy", {enabled,expectedRevision:policy.revision}));if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');return this.parsePolicy(raw,policy.libraryId);}
 async readProfile(id:string,expectedGeneration?:string,guard?:()=>boolean):Promise<SourceReadSettings>{
 if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
 const epoch=this.epoch;
 try{
  const raw=await this.request(encodedRoutePath("get_storage_source_read_profile",{"id":encodeURIComponent(id)}),'GET',undefined,guard);
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  this.fence(raw);if(!record(raw)||!record(raw.settings))invalid();
  const settings=sourceReadSettings(raw.settings);
  if(settings.sourceId!==id||expectedGeneration!==undefined&&settings.generation!==expectedGeneration)invalid();
  return settings;
 }catch(error){
  const refused=this.refusalCode(error);
  if(refused)throw Object.assign(new Error(this.refusalMessage(refused)),{code:refused});
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  if(error instanceof Error&&error.message==='Invalid remote source response.')throw error;
  const code=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'internal';
  throw Object.assign(new Error(safeError({code})),{code});
 }
}
async saveProfile(profile:SourceReadSettings,values:SourceReadValues,guard?:()=>boolean):Promise<SourceReadSettings>{
 if(!this.live()||(guard&&!guard())||this.state.busy||this.state.ambiguous)throw new Error('Finish or recover the current source command first.');
 const parsed=sourceReadValues(values);
 if(!profile.sourceId||!profile.generation||!Number.isSafeInteger(profile.revision)||profile.revision<0)throw new Error('Invalid source profile.');
 const epoch=this.epoch;
 this.publish({busy:true,error:'',errorCode:'',receipt:null,rclone:null,alreadyDone:false,operationId:null,operationKind:'source'});
 let submitted=false,accepted=false;
 try{
  const op=await this.operationId();if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  if(!/^\d{13}-[0-9a-f-]{36}$/i.test(op))invalid();
  this.publish({operationId:op});submitted=true;
  const raw=await this.request(encodedRoutePath("put_storage_source_read_profile",{"id":encodeURIComponent(profile.sourceId)}),'PUT',routeBody("put_storage_source_read_profile",{operationId:op,expectedGeneration:profile.generation,expectedRevision:profile.revision,values:parsed}),guard);
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  const receipt=this.parseReceipt(raw,op);
  if(receipt.removed||receipt.source.id!==profile.sourceId)invalid();
  const next=record(raw)&&record(raw.receipt)&&raw.receipt.readSettings!==undefined?sourceReadSettings(raw.receipt.readSettings):undefined;
  if(next&&(next.sourceId!==profile.sourceId||next.generation!==receipt.source.generation.toString()))invalid();
  if(guard&&!guard())throw new Error('Scope changed.');
  accepted=true;this.publish({receipt,ambiguous:false});
  if(next)return next;
  try{return await this.readProfile(profile.sourceId, receipt.source.generation.toString(),guard);}
  catch(error){
   const code=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'internal';
   if(['unauthorized','forbidden','not_permitted','remote_credentials_required','source_credentials_required'].includes(code))throw Object.assign(new Error(safeError({code})),{code});
   throw Object.assign(new Error(safeError({code:'refresh_failed'})),{code:'refresh_failed'});
  }
 }catch(error){
  const refused=this.refusalCode(error);
  if(refused)throw Object.assign(new Error(this.refusalMessage(refused)),{code:refused});
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  const code=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'';
  const rejected=['storage_command_conflict','storage_operation_expired','revision_mismatch','idempotency_key_reused','remote_invalid_configuration','remote_credentials_required','unauthorized','forbidden','not_permitted','invalid_request'].includes(code);
  const auth=['unauthorized','forbidden','not_permitted','remote_credentials_required','source_credentials_required'].includes(code);
  this.publish(accepted?{ambiguous:false,error:safeError({code:auth?code:'refresh_failed'}),errorCode:auth?code:'refresh_failed'}:{ambiguous:submitted&&!rejected,error:safeError(error),errorCode:code});
  throw accepted?Object.assign(new Error(safeError({code:auth?code:'refresh_failed'})),{code:auth?code:'refresh_failed'}):Object.assign(new Error(safeError(error)),{code:code||'internal'});
 }finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
}

async networkPolicy(libraryId:string):Promise<NetworkPolicy>{const epoch=this.epoch,raw=await this.request(encodedRoutePath("get_libraries_id_network_roots", {"id": encodeURIComponent(libraryId)}));if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');this.fence(raw);if(!record(raw)||!record(raw.policy)||raw.policy.libraryId!==libraryId||!integer(raw.policy.revision)||!Array.isArray(raw.policy.origins)||raw.policy.origins.length>8||!raw.policy.origins.every(origin=>text(origin,8192)))invalid();return Object.freeze({libraryId,revision:raw.policy.revision,origins:Object.freeze(raw.policy.origins as string[])});}
 async setNetworkPolicy(policy:NetworkPolicy,roots:readonly string[]):Promise<NetworkPolicy>{if(roots.length>8||roots.some(root=>!text(root,8192)))throw new Error('Invalid network roots.');const epoch=this.epoch;await this.request(encodedRoutePath("post_libraries_id_network_roots", {"id": encodeURIComponent(policy.libraryId)}),'POST',routeBody("post_libraries_id_network_roots", {roots,expectedRevision:policy.revision}));if(!this.live()||epoch!==this.epoch)throw new Error('Scope changed.');return this.networkPolicy(policy.libraryId);}
 /** One folder of a saved source, folders only, read when the owner opens it.
  * Nothing is listed in advance, downloaded or cached by this call. */
 async listFolders(id:string,options:Readonly<{path?:string;cursor?:string;showHidden?:boolean;limit?:number}>={},guard?:()=>boolean):Promise<RemoteFolderPage>{
  if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
  if(!text(id,256)||!id)throw new Error('Invalid source.');
  const limit=options.limit??200;
  if(!Number.isInteger(limit)||limit<1||limit>500)throw new Error('Invalid folder query.');
  const query=new URLSearchParams({limit:String(limit)});
  for(const key of ['path','cursor'] as const){const value=options[key];if(value!==undefined){if(!text(value)||!value)throw new Error('Invalid folder path.');query.set(key,value);}}
  if(options.showHidden!==undefined)query.set('showHidden',options.showHidden?'1':'0');
  const epoch=this.epoch;
  const raw=await this.request(encodedRoutePath('get_storage_source_folders',{id:encodeURIComponent(id)})+'?'+query,'GET',undefined,guard);
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  this.fence(raw);
  if(!record(raw)||!record(raw.folders))invalid();
  return remoteFolderPage(raw.folders,id);
 }
 /** The Advanced page as saved; unsaved values default from the source's folders. */
 async readAdvanced(id:string,guard?:()=>boolean):Promise<SourceAdvancedSettings>{
  if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
  if(!text(id,256)||!id)throw new Error('Invalid source.');
  const epoch=this.epoch;
  const raw=await this.request(encodedRoutePath('get_storage_source_advanced',{id:encodeURIComponent(id)}),'GET',undefined,guard);
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  this.fence(raw);
  if(!record(raw)||!record(raw.settings))invalid();
  return sourceAdvancedSettings(raw.settings,id);
 }
 /** One CAS write of the Advanced page; the server applies it to the folders and cache. */
 async saveAdvanced(settings:SourceAdvancedSettings,values:SourceAdvanced,guard?:()=>boolean):Promise<SourceAdvancedSettings>{
  if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
  if(!settings.sourceId||!settings.generation||!Number.isSafeInteger(settings.revision)||settings.revision<0)throw new Error('Invalid source settings.');
  const epoch=this.epoch;
  this.publish({busy:true,error:'',errorCode:''});
  try{
   const raw=await this.request(encodedRoutePath('put_storage_source_advanced',{id:encodeURIComponent(settings.sourceId)}),'PUT',{expectedGeneration:settings.generation,expectedRevision:settings.revision,values:sourceAdvanced(values)},guard);
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   this.fence(raw);
   if(!record(raw)||!record(raw.settings))invalid();
   const next=sourceAdvancedSettings(raw.settings,settings.sourceId);
   if(next.revision<settings.revision||next.generation!==settings.generation)invalid();
   return next;
  }catch(error){
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   const code=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'internal';
   this.publish({error:safeError({code}),errorCode:code});
   throw Object.assign(new Error(safeError({code})),{code:code||'internal'});
  }finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}
 }
 /** What the source is doing now, and where its folders went; one bounded read. */
 async readStatus(id:string,guard?:()=>boolean):Promise<RemoteSourceStatus>{
  if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');
  if(!text(id,256)||!id)throw new Error('Invalid source.');
  const epoch=this.epoch;
  const raw=await this.request(encodedRoutePath('get_storage_source_status',{id:encodeURIComponent(id)}),'GET',undefined,guard);
  if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
  this.fence(raw);
  if(!record(raw)||!record(raw.status))invalid();
  return remoteSourceStatus(raw.status,id);
 }
}
export type SourceAvailability='ready'|'unavailable'|'changed'|'unsupported'|'resolve_on_play';
export async function readSourceAvailability(api:LibraryContentApi,itemId:string,signal:AbortSignal):Promise<SourceAvailability>{
 const raw=await api.request<RouteResponse<"get_items_id_source_availability">>(encodedRoutePath("get_items_id_source_availability", {"id": encodeURIComponent(itemId)}),'GET',undefined,signal);
 if(signal.aborted)throw new Error('Source request cancelled.');
 if(!record(raw)||raw.itemId!==itemId||!['ready','unavailable','changed','unsupported','resolve_on_play'].includes(String(raw.state)))invalid();
 return raw.state as SourceAvailability;
}
export function sourceAvailabilityMessage(state:SourceAvailability):string{return {ready:'',unavailable:'This source is unavailable. Ask the server owner to check its connection.',changed:'The source changed. Ask the server owner to refresh this library before retrying.',unsupported:'This source cannot currently provide safe playback ranges.',resolve_on_play:'This external source will be checked securely by the server when playback starts.'}[state];}

export type NetworkPolicy=Readonly<{libraryId:string;revision:number;origins:readonly string[]}>;
