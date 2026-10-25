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
export type RemoteReceipt=Readonly<{operationId:string;accepted:true;removed:boolean;source:RemoteSource}>;
export type AnalysisPolicy=Readonly<{libraryId:string;enabled:boolean;revision:number;effective?:boolean;scanTier?:string}>;
/** The read-authority part of one source's Reading settings. A missing saved profile is
 * reported as `saved:false` with the server's list/once defaults; it is never invented. */
export type SourceReadValues=Readonly<{scanDepth:'file_list_only'|'basic'|'complete';wholeFileReads:boolean;connections:number;requestsPerSecond:number;bytesPerSecond:number}>;
export type SourceReadSettings=Readonly<{sourceId:string;generation:string;revision:number;saved:boolean;values:SourceReadValues}>;
export type SourceReadPreset='list'|'once'|'everything'|'ask';
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
/** `alreadyDone`: the last command found, on a fresh read, that what it asked for is already so (a share removed elsewhere). */
export type RemoteState=Readonly<{sources:readonly RemoteSource[];loading:boolean;busy:boolean;error:string;errorCode:string;operationId:string|null;ambiguous:boolean;receipt:RemoteReceipt|null;alreadyDone:boolean;mountSupported:boolean}>;
const record=(x:unknown):x is Record<string,unknown>=>!!x&&typeof x==='object'&&!Array.isArray(x);
const text=(x:unknown,n=8192):x is string=>typeof x==='string'&&x.length<=n&&!/[\x00-\x1f\x7f]/.test(x);
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
const messages:Record<string,string>={remote_source_changed:'This source changed. Refresh before submitting a new configuration.',storage_command_conflict:'The source changed or this operation conflicts with a previous receipt.',storage_operation_expired:'The operation expired. Check the device clock and refresh.',remote_credentials_required:'The provider rejected the credentials. The previous configuration remains active.',source_credentials_required:'The provider rejected the credentials. The previous configuration remains active.',remote_invalid_configuration:'Check the endpoint, approved network access and configuration.',remote_range_unsupported:'The server cannot safely range-read this provider representation.',remote_binary_changed:'The approved rclone executable changed. Reapprove it in source configuration.',remote_budget_exceeded:'This operation exceeded the configured safety budget.',remote_rate_limited:'The provider or server is busy. Check the original receipt before retrying.',revision_mismatch:'This source changed. Refresh before saving.',idempotency_key_reused:'This operation was already used. Refresh and try again.',request_in_progress:'Another source change is in progress. Try again in a moment.',refresh_failed:'The profile was saved, but the refreshed values could not be read.',unauthorized:'Owner sign-in is required.',forbidden:'This viewer cannot manage sources.'};
const CONFLICT='storage_command_conflict';
const ERROR_CODES=new Set(['remote_source_changed','storage_command_conflict','storage_operation_expired','remote_credentials_required','remote_invalid_configuration','remote_range_unsupported','remote_binary_changed','remote_budget_exceeded','remote_rate_limited','revision_mismatch','idempotency_key_reused','request_in_progress','refresh_failed','source_credentials_required','unauthorized','forbidden','not_permitted','invalid_request','internal']);
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
 private state:RemoteState={sources:[],loading:false,busy:false,error:'',errorCode:'',operationId:null,ambiguous:false,receipt:null,alreadyDone:false,mountSupported:false};
 private listeners=new Set<()=>void>();private controllers=new Set<AbortController>();private epoch=0;private read=0;private disposed=false;private viewerFence?:string;
 private api:LibraryContentApi;private scope:ContentScope;private operationId:()=>Promise<string>;private readonly isCurrent:()=>boolean;
 constructor(api:LibraryContentApi,scope:ContentScope,operationId:()=>Promise<string>,isCurrent:()=>boolean=()=>true){if(!scope.serverId||!scope.viewerId)throw new Error('Owner scope required.');this.api=api;this.scope=Object.freeze({...scope});this.operationId=operationId;this.isCurrent=isCurrent;}
 getSnapshot=()=>this.state;subscribe=(fn:()=>void)=>{this.listeners.add(fn);return()=>this.listeners.delete(fn);};
 private live(){return !this.disposed&&this.isCurrent();}
 private publish(value:Partial<RemoteState>){if(!this.live())return;this.state=Object.freeze({...this.state,...value});for(const fn of this.listeners)fn();}
 dispose(){this.epoch++;this.disposed=true;for(const c of this.controllers)c.abort();this.controllers.clear();this.listeners.clear();this.state={...this.state,sources:[],receipt:null,operationId:null};}
 private fence(raw:unknown){if(!record(raw)||!record(raw.scope)||raw.scope.serverId!==this.scope.serverId||!text(raw.scope.viewerFence,256)||!raw.scope.viewerFence||this.viewerFence&&this.viewerFence!==raw.scope.viewerFence)invalid();this.viewerFence=raw.scope.viewerFence;}
 private async request(path:string,method='GET',body?:unknown,guard?:()=>boolean):Promise<unknown>{if(!this.live()||(guard&&!guard()))throw new Error('Scope changed.');const c=new AbortController();this.controllers.add(c);let timer:ReturnType<typeof setTimeout>|undefined;try{return await Promise.race([this.api.request(path,method,body,c.signal),new Promise<never>((_,reject)=>{timer=setTimeout(()=>{c.abort();reject(new Error('timeout'));},45000);})]);}finally{if(timer)clearTimeout(timer);this.controllers.delete(c);}}
 /** The sources as the server has them, checked; `undefined` when a later read or another viewer has taken over. */
 private async readSources(epoch:number,sequence:number,guard?:()=>boolean){const raw=await this.request(routePath("get_storage_sources"),'GET',undefined,guard);if(!this.live()||(guard&&!guard())||epoch!==this.epoch||sequence!==this.read)return undefined;this.fence(raw);if(!record(raw)||!Array.isArray(raw.sources)||typeof raw.mountSupported!=='boolean')invalid();const rows=firstOfEach('remote-sources','sources',keepReadable('remote-sources','sources',raw.sources,source),s=>s.id);return {sources:Object.freeze(rows),mountSupported:raw.mountSupported};}
 /**
  * Reads the sources again while they are on screen (a share that became reachable, or stopped
  * being). What is shown stays; a failure leaves it as it was and rejects. Nothing is read while
  * a command is in flight or its outcome is unknown.
  */
 async revalidate(){if(!this.live()||this.state.busy||this.state.ambiguous||this.state.loading)return;const epoch=this.epoch,sequence=++this.read;const next=await this.readSources(epoch,sequence);if(next&&!this.state.busy)this.publish({...next,error:''});}
 async load(){if(!this.live())return;const epoch=this.epoch,sequence=++this.read;this.publish({loading:true,error:''});try{const next=await this.readSources(epoch,sequence);if(!next)return;this.publish(next);}catch(e){if(epoch===this.epoch&&sequence===this.read)this.publish({error:safeError(e),sources:[],receipt:null,mountSupported:false});}finally{if(epoch===this.epoch&&sequence===this.read)this.publish({loading:false});}}
 private parseReceipt(raw:unknown,op:string):RemoteReceipt{this.fence(raw);if(!record(raw)||!record(raw.receipt)||raw.receipt.operationId!==op||raw.receipt.accepted!==true||typeof raw.receipt.removed!=='boolean')invalid();return Object.freeze({operationId:op,accepted:true,removed:raw.receipt.removed,source:source(raw.receipt.source)});}
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
   if(!this.live()||(guard&&!guard())||epoch!==this.epoch)throw new Error('Scope changed.');
   const rawCode=record(error)&&typeof error.code==='string'?normalizedCode(error.code):'';
   const code=['unauthorized','not_permitted','invalid_request','source_credentials_required','internal'].includes(rawCode)?rawCode:'internal';
   const aliases:Record<string,string>={not_permitted:'forbidden',invalid_request:'remote_invalid_configuration',source_credentials_required:'remote_credentials_required'};
   const message=safeError({code:aliases[code]??(code==='internal'?'remote_invalid_configuration':code)});
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
  await this.command(path,id?'PUT':'POST',input,guard);
  if(!id||!base||this.state.errorCode!==CONFLICT)return;
  const fresh=await this.reread(id,guard);if(!fresh)return;
  const rebased=rebaseDavEdit(base,fresh,input);if(!rebased)return this.publish({error:safeError({code:CONFLICT}),errorCode:CONFLICT});
  return this.command(path,'PUT',rebased,guard);}
 /** Removes a share. A share changed elsewhere is still the share the owner meant: it is read again and removed; one already gone is done. */
 async remove(row:RemoteSource,guard?:()=>boolean){if(!this.live()||(guard&&!guard())||row.kind!=='webdav'||this.state.busy||this.state.ambiguous)throw new Error('This source cannot be removed now.');
  const path=encodedRoutePath("delete_storage_sources_webdav_id", {"id": encodeURIComponent(row.id)});
  await this.command(path,'DELETE',{expectedGeneration:row.generation},guard);
  if(this.state.errorCode!==CONFLICT)return;
  const fresh=await this.reread(row.id,guard);if(fresh===null)return this.publish({error:'',errorCode:'',alreadyDone:true});if(!fresh)return;
  return this.command(path,'DELETE',{expectedGeneration:fresh.generation},guard);}
 /** After a conflict: the share as the server has it now, null when it is gone, undefined when it could not be read (the conflict stays as it was). */
 private async reread(id:string,guard?:()=>boolean):Promise<RemoteSource|null|undefined>{const epoch=this.epoch,sequence=++this.read;try{const next=await this.readSources(epoch,sequence,guard);if(!next||(guard&&!guard()))return undefined;this.publish(next);return next.sources.find(s=>s.id===id)??null;}catch{return undefined;}}
 private async command(path:string,method:string,body:unknown,guard?:()=>boolean){const epoch=this.epoch;this.publish({busy:true,error:'',errorCode:'',receipt:null,alreadyDone:false,operationId:null});let submitted=false;try{const op=await this.operationId();if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;if(!/^\d{13}-[0-9a-f-]{36}$/i.test(op))invalid();this.publish({operationId:op});submitted=true;const pending=this.request(path,method,{...(body as object),operationId:op},guard);body=undefined;const raw=await pending;if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;const receipt=this.parseReceipt(raw,op);if(guard&&!guard())return;this.publish({receipt,ambiguous:false});await this.load();}catch(e){if(!this.live()||(guard&&!guard())||epoch!==this.epoch)return;const code=record(e)&&typeof e.code==='string'?normalizedCode(e.code):'';const rejected=['storage_command_conflict','storage_operation_expired','remote_invalid_configuration','remote_credentials_required','unauthorized','forbidden'].includes(code);this.publish({ambiguous:submitted&&!rejected,error:safeError(e),errorCode:code});}finally{body=undefined;if(this.live()&&epoch===this.epoch)this.publish({busy:false});}}
 async recover(){if(!this.live()||this.state.busy||!this.state.operationId)throw new Error('No source operation to recover.');const op=this.state.operationId,epoch=this.epoch;this.publish({busy:true,error:''});try{const raw=await this.request(encodedRoutePath("get_storage_source_operations_id", {"id": encodeURIComponent(op)}));if(!this.live()||epoch!==this.epoch)return;this.publish({receipt:this.parseReceipt(raw,op),ambiguous:false});await this.load();}catch(e){if(epoch===this.epoch)this.publish({ambiguous:true,error:safeError(e)});}finally{if(this.live()&&epoch===this.epoch)this.publish({busy:false});}}
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
 this.publish({busy:true,error:'',errorCode:'',receipt:null,alreadyDone:false,operationId:null});
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
