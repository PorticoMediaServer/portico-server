import {test} from 'node:test';
import assert from 'node:assert/strict';
import {DesktopDownloadStore,downloadFailure,parseLocalDownload,type DownloadScope,type NativeDownloadsBridge} from '../src/downloads.ts';
const scope:DownloadScope={authority:'local',serverId:'server',accountId:'account',profileId:'profile',sessionFamilyId:'family'};
const lease='a'.repeat(64),rowId='b'.repeat(64);
const row={id:rowId,preparationId:'prep',itemId:'item',quality:'original',sha256:'c'.repeat(64),bytes:8,bytesDone:2,state:'downloading',held:false,revision:4,addedMs:1,receiptExpiresMs:null,title:{kind:'song',title:'Actual stored title',artist:'Actual artist',parentId:'album',parentTitle:'Actual album',groupId:'disc',groupTitle:'',groupNumber:1,number:2,year:1977,durationMs:181000,onServer:false}};
function bridge(command:NativeDownloadsBridge['command']):NativeDownloadsBridge{return{async bind(){return lease},async release(){},command};}
test('native records preserve canonical title units without exposing raw fields',()=>{
 const parsed=parseLocalDownload({...row,path:'/private/never',token:'never'});
 assert.equal(parsed.title.durationSeconds,181);assert.equal(parsed.title.onServer,false);
 assert.ok(Object.isFrozen(parsed));assert.ok(!('path' in parsed));assert.ok(!('token' in parsed));
 assert.throws(()=>parseLocalDownload({...row,bytesDone:9}));
});
test('a late native acknowledgement is discarded after captured viewer retirement',async()=>{
 let current=true,finish!:(value:unknown)=>void;const releases:string[]=[];
 const native=bridge(()=>new Promise(resolve=>{finish=resolve}));
 const store=await DesktopDownloadStore.bind(scope,()=>current,{...native,async release(value){releases.push(value)}});
 const request=store.entry(rowId);current=false;finish({kind:'entry',value:row});
 await assert.rejects(request,{name:'AbortError'});store.dispose();await Promise.resolve();assert.deepEqual(releases,[lease]);
});
test('cancelled bind retires the actual native lease and never returns a usable adapter',async()=>{
 const controller=new AbortController();let complete!:(value:string)=>void;const releases:string[]=[];
 const request=DesktopDownloadStore.bind(scope,()=>true,{...bridge(async()=>null),bind:()=>new Promise(resolve=>{complete=resolve}),async release(value){releases.push(value)}},controller.signal);
 controller.abort();complete(lease);await assert.rejects(request,{name:'AbortError'});assert.deepEqual(releases,[lease]);
});
test('bounded writes carry committed revision and offset; oversized payload never invokes native',async()=>{
 const operations:unknown[]=[];const store=await DesktopDownloadStore.bind(scope,()=>true,bridge(async(_,op)=>{operations.push(op);return{kind:'entry',value:{...row,revision:5,bytesDone:4}}}));
 const saved=await store.append(parseLocalDownload(row),new Uint8Array([3,4]));
 assert.equal(saved.revision,5);assert.deepEqual(operations,[{action:'append',id:rowId,expectedRevision:4,offset:2,bytes:[3,4]}]);
 await assert.rejects(store.append(saved,new Uint8Array(262145)),/invalid_command/);assert.equal(operations.length,1);store.dispose();
});
test('artwork is a native byte Blob or null; raw errors never reach UI',async()=>{
 const store=await DesktopDownloadStore.bind(scope,()=>true,bridge(async()=>({kind:'artwork',value:{mime:'image/png',bytes:[137,80,78,71]}})));
 const blob=await store.getArtwork(rowId);assert.equal(blob?.type,'image/png');assert.deepEqual([...new Uint8Array(await blob!.arrayBuffer())],[137,80,78,71]);store.dispose();
 assert.equal(downloadFailure(new Error('/private/path?secret=hidden')).message,'downloads_unavailable');assert.equal(downloadFailure('downloads_conflict').message,'downloads_conflict');
});
