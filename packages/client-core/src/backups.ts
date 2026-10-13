import {routeBody} from "../../contracts/src/routes.ts";
import type {RouteResponse} from "../../contracts/src/routes.ts";
import {routePath} from "../../contracts/src/routes.ts";
import {parseBackupsDocument,parseEnvelope,type BackupsDocument,type RestoreSource,type RestoreStaged} from './administration.ts';

/** Typed clients for the spec §4 backup routes (`Spec — Backups (Plex model).md`).
 * Parsers live in administration.ts; this module holds transport only, no stored credentials.
 * TODO(be backups): switch to generated types once apigen publishes these routes. */
export type BackupTransport={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};

const BACKUPS_PATH=routePath("get_admin_backups");
const RESTORE_PATH=routePath("post_admin_backups_restore");
/** The owner-only fix-permissions operation (spec §6.2), as be/backups publishes it. */
export const STATE_PERMISSIONS_FIX_PATH=routePath("post_admin_state_permissions_fix");

/** getBackups reads the whole backups document: the list, the running backup (if any) and the last restore. */
export async function getBackups(transport:BackupTransport,serverId:string,signal?:AbortSignal):Promise<BackupsDocument>{
 return parseEnvelope(await transport.request<unknown>(BACKUPS_PATH,'GET',undefined,signal),serverId,parseBackupsDocument).result;
}

/** startBackup starts one backup. `operationId` is the caller's: one per logical submission,
 * reused on retry (M19). Progress appears in `running`, read back with getBackups. */
export async function startBackup(transport:BackupTransport,serverId:string,operationId:string,signal?:AbortSignal):Promise<void>{
 await transport.request<unknown>(BACKUPS_PATH,'POST',{operationId},signal);
}

/** deleteBackup removes one listed backup. */
export async function deleteBackup(transport:BackupTransport,serverId:string,backupId:string,signal?:AbortSignal):Promise<void>{
 await transport.request<unknown>(`${BACKUPS_PATH}/${encodeURIComponent(backupId)}`,'DELETE',undefined,signal);
}

/** restoreBackup stages a backup (by list id) or a server path (a backup folder or a `.db` file)
 * and answers `{staged, restartRequired}`. A 4xx refusal carries one of the spec §4 codes
 * (`isRestoreRefusalCode`), which the console maps to catalogue copy. */
export async function restoreBackup(transport:BackupTransport,serverId:string,operationId:string,source:RestoreSource,signal?:AbortSignal):Promise<RestoreStaged>{
 const receipt=parseEnvelope(await transport.request<unknown>(RESTORE_PATH,'POST',{operationId,source},signal),serverId,v=>{
  if(typeof v!=='object'||v===null||typeof (v as {jobId?:unknown}).jobId!=='string')throw new Error('Invalid restore receipt.');
  return (v as {jobId:string}).jobId;
 }).result;
 const started=Date.now();
 for(;;){
  signal?.throwIfAborted();
  let doc:BackupsDocument;
  try{doc=await getBackups(transport,serverId,signal);}catch(e){
   const status=(e as {status?:number})?.status;
   const code=(e as {code?:string})?.code;
   if(authenticationFailure(e)||code==='wrong_server'||code==='invalid_administration_response'||status!==undefined&&status<500&&status!==429)throw e;
   if(Date.now()-started>600_000)throw Object.assign(new Error('The restore is still running.'),{code:'restore_progress_timeout'});
   await delay(500,signal);continue;
  }
  if(doc.lastRestore?.jobId===receipt){
   if(doc.lastRestore.outcome==='rolled_back')throw Object.assign(new Error('The restore was rolled back.'),{code:'restore_integrity_failed'});
   return {staged:true,restartRequired:true,validation:{schemaVersion:doc.lastRestore.schemaVersion??0,integrity:'ok'}};
  }
  if(doc.restore?.jobId===receipt){
   if(doc.restore.errorCode)throw Object.assign(new Error('The restore could not be staged.'),{code:doc.restore.errorCode});
   if(doc.restore.result)return doc.restore.result;
  }
  if(Date.now()-started>600_000)throw Object.assign(new Error('The restore is still running.'),{code:'restore_progress_timeout'});
  await delay(500,signal);
 }
}

/** fixStatePermissions runs the owner-only §6.2 permissions repair. See STATE_PERMISSIONS_FIX_PATH. */
export async function fixStatePermissions(transport:BackupTransport,serverId:string,operationId:string,signal?:AbortSignal):Promise<void>{
 await transport.request<unknown>(STATE_PERMISSIONS_FIX_PATH,'POST',{operationId},signal);
}

export type BackupPollOptions=Readonly<{intervalMs?:number;timeoutMs?:number;signal?:AbortSignal;onProgress?:(doc:BackupsDocument)=>void}>;

function delay(ms:number,signal?:AbortSignal):Promise<void>{
 return new Promise<void>((resolve,reject)=>{
  if(signal?.aborted){reject(Object.assign(new Error('The wait was cancelled.'),{code:'cancelled'}));return;}
  if(ms<=0){resolve();return;}
  const onAbort=()=>{clearTimeout(timer);reject(Object.assign(new Error('The wait was cancelled.'),{code:'cancelled'}));};
  const timer=setTimeout(()=>{signal?.removeEventListener('abort',onAbort);resolve();},ms);
  signal?.addEventListener('abort',onAbort,{once:true});
 });
}

function authenticationFailure(e:unknown):boolean{
 const status=(e as {status?:unknown}|null)?.status;
 if(status===401||status===403)return true;
 const code=(e as {code?:unknown}|null)?.code;
 return code==='unauthorized'||code==='forbidden'||code==='authentication_required'||code==='session_expired'||code==='invalid_token';
}

/** isAuthenticationFailure answers whether a failure ended the sign-in (the app opens
 * sign-in again) rather than reporting a task failure. */
export function isAuthenticationFailure(e:unknown):boolean{
 return authenticationFailure(e);
}

/** waitForBackupCompletion polls getBackups every 2 s until `running` clears, reporting each
 * read through `onProgress`. The caller stops polling by abandoning the promise (abort). */
export async function waitForBackupCompletion(get:()=>Promise<BackupsDocument>,options:BackupPollOptions={}):Promise<BackupsDocument>{
 const intervalMs=options.intervalMs??2000,timeoutMs=options.timeoutMs??600_000;
 const started=Date.now();
 for(;;){
  options.signal?.throwIfAborted();
  const doc=await get();
  options.onProgress?.(doc);
  if(!doc.running)return doc;
  if(Date.now()-started>timeoutMs)throw Object.assign(new Error('The backup is still running.'),{code:'backup_progress_timeout'});
  await delay(intervalMs,options.signal);
 }
}

/** waitForServerAnswer polls until the server answers again after the restore restart, then
 * returns the document the result is read from (`lastRestore`). Connection failures are
 * "not yet"; an authentication failure propagates, so an ended sign-in still lands on the
 * sign-in screen instead of sitting on "Restarting…". */
export async function waitForServerAnswer(get:()=>Promise<BackupsDocument>,options:BackupPollOptions={}):Promise<BackupsDocument>{
 const intervalMs=options.intervalMs??2000,timeoutMs=options.timeoutMs??180_000;
 const started=Date.now();
 for(;;){
  options.signal?.throwIfAborted();
  try{
   return await get();
  }catch(e){
   if(authenticationFailure(e))throw e;
   if(Date.now()-started>timeoutMs)throw Object.assign(new Error('The server has not answered yet.'),{code:'server_restart_timeout'});
   await delay(intervalMs,options.signal);
  }
 }
}

/** Restart is owner-only and works for portable launches as well as services. */
export async function restartServer(transport:BackupTransport,signal?:AbortSignal):Promise<void>{
 await transport.request<RouteResponse<"post_admin_restart">>(routePath("post_admin_restart"),'POST',routeBody("post_admin_restart", {}),signal);
}
