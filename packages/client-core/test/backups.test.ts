import test from 'node:test';
import assert from 'node:assert/strict';
import {deleteBackup,fixStatePermissions,getBackups,restoreBackup,startBackup,waitForBackupCompletion,waitForServerAnswer,STATE_PERMISSIONS_FIX_PATH,type BackupTransport,type BackupsDocument} from '../src/backups.ts';

const SERVER='srv-1';
const backup={id:'2026-09-24T013000Z',createdAt:'2026-09-24T01:30:00Z',kind:'manual',schemaVersion:41,serverVersion:'1.2.0',bytes:120,path:'/var/lib/portico/backups/2026-09-24T013000Z'};
const envelope=(result:unknown)=>({protocolVersion:'1.0',serverId:SERVER,result});

function transport(respond:(path:string,method:string,body:unknown)=>unknown,calls:{path:string;method:string;body:unknown}[]=[]):BackupTransport{
 return {request:async <T,>(path:string,method='GET',body?:unknown):Promise<T>=>{calls.push({path,method,body});return respond(path,method,body) as T;}};
}

test('the typed clients hit the spec routes with the operation id',async()=>{
 const calls:{path:string;method:string;body:unknown}[]=[];
 const api=transport((path,method)=>{
  if(path==='/v1/admin/backups'&&method==='GET')return envelope({backups:[backup]});
  if(path==='/v1/admin/backups/restore')return envelope({staged:true,restartRequired:true,validation:{schemaVersion:41,integrity:'ok'}});
  return envelope({});
 },calls);
 const doc=await getBackups(api,SERVER);
 assert.equal(doc.backups[0].id,backup.id);
 await startBackup(api,SERVER,'op-1');
 assert.deepEqual(calls[1],{path:'/v1/admin/backups',method:'POST',body:{operationId:'op-1'}});
 await deleteBackup(api,SERVER,'2026-09-24T013000Z');
 assert.deepEqual(calls[2],{path:'/v1/admin/backups/2026-09-24T013000Z',method:'DELETE',body:undefined});
 const staged=await restoreBackup(api,SERVER,'op-2',{backupId:'2026-09-24T013000Z'});
 assert.equal(staged.restartRequired,true);
 assert.deepEqual(calls[3].body,{operationId:'op-2',source:{backupId:'2026-09-24T013000Z'}});
 const byPath=await restoreBackup(api,SERVER,'op-3',{path:'/media/portico.db'});
 assert.deepEqual(calls[4].body,{operationId:'op-3',source:{path:'/media/portico.db'}});
 assert.equal(byPath.validation.integrity,'ok');
 await fixStatePermissions(api,SERVER,'op-4');
 assert.deepEqual(calls[5],{path:STATE_PERMISSIONS_FIX_PATH,method:'POST',body:{operationId:'op-4'}});
});

test('a response from another server is refused',async()=>{
 const api=transport(()=>({protocolVersion:'1.0',serverId:'srv-2',result:{backups:[]}}));
 await assert.rejects(()=>getBackups(api,SERVER),(e:unknown)=>(e as {code?:string}).code==='wrong_server');
});

test('backup completion resolves when running clears and reports each read',async()=>{
 const docs:BackupsDocument[]=[
  {backups:[],running:{jobId:'j',phase:'copying',bytesDone:1,bytesTotal:4}},
  {backups:[],running:{jobId:'j',phase:'copying',bytesDone:3,bytesTotal:4}},
  {backups:[backup]},
 ];
 const seen:BackupsDocument[]=[];
 let n=0;
 const done=await waitForBackupCompletion(()=>Promise.resolve(docs[Math.min(n++,docs.length-1)]),{intervalMs:1,onProgress:d=>{seen.push(d);}});
 assert.equal(done.backups.length,1);
 assert.equal(seen.length,3);
 assert.equal(done.running,undefined);
});

test('backup completion times out instead of polling forever',async()=>{
 const running:BackupsDocument={backups:[],running:{jobId:'j',phase:'copying',bytesDone:1,bytesTotal:4}};
 await assert.rejects(()=>waitForBackupCompletion(()=>Promise.resolve(running),{intervalMs:1,timeoutMs:5}),(e:unknown)=>(e as {code?:string}).code==='backup_progress_timeout');
});

test('the restart wait returns the first answer and its lastRestore',async()=>{
 const restored:BackupsDocument={backups:[backup],lastRestore:{at:'2026-09-24T02:00:00Z',outcome:'restored',reason:''}};
 let n=0;
 const doc=await waitForServerAnswer(()=>{
  n++;
  if(n<3)throw Object.assign(new Error('connect refused'),{code:'request_failed'});
  return Promise.resolve(restored);
 },{intervalMs:1});
 assert.equal(n,3);
 assert.equal(doc.lastRestore?.outcome,'restored');
});

test('the restart wait surfaces an ended sign-in instead of waiting it out',async()=>{
 let n=0;
 await assert.rejects(()=>waitForServerAnswer(()=>{
  n++;
  throw Object.assign(new Error('denied'),{status:401});
 },{intervalMs:1,timeoutMs:50}),(e:unknown)=>(e as {status?:number}).status===401);
 assert.equal(n,1);
});

test('the restart wait times out when the server never answers',async()=>{
 await assert.rejects(()=>waitForServerAnswer(()=>Promise.reject(Object.assign(new Error('down'),{code:'request_failed'})),{intervalMs:1,timeoutMs:5}),(e:unknown)=>(e as {code?:string}).code==='server_restart_timeout');
});
