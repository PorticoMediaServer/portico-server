import {parseBackupsDocument,parseEnvelope,parseRestoreStaged,type BackupsDocument,type RestoreSource,type RestoreStaged} from './administration.ts';

/** Typed clients for the spec §4 backup routes (`Spec — Backups (Plex model).md`).
 * Parsers live in administration.ts; this module holds transport only, no stored credentials.
 * TODO(be backups): switch to generated types once apigen publishes these routes. */
export type BackupTransport={request<T>(path:string,method?:string,body?:unknown,signal?:AbortSignal):Promise<T>};

const BACKUPS_PATH='/v1/admin/backups';
const RESTORE_PATH='/v1/admin/backups/restore';
/** The owner-only fix-permissions operation (spec §6.2), as be/backups publishes it. */
export const STATE_PERMISSIONS_FIX_PATH='/v1/admin/state-permissions:fix';

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
 return parseEnvelope(await transport.request<unknown>(RESTORE_PATH,'POST',{operationId,source},signal),serverId,parseRestoreStaged).result;
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
