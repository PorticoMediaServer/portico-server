import {test} from 'node:test';
import assert from 'node:assert/strict';
import {DesktopDownloadStore,type NativeDownloadsBridge} from '../src/downloads.ts';
import {DesktopLocalPlaybackBroker} from '../src/local-playback.ts';
import type {LocalPlayerSource} from '../../../packages/client-core/src/playback-v1/local-port.ts';

const scope={authority:'local' as const,serverId:'server',accountId:'account',profileId:'profile',sessionFamilyId:'family'},lease='a'.repeat(64),entryId='b'.repeat(64),localSourceId='c'.repeat(64),preparationId='d'.repeat(64);
const member={id:entryId,revision:4,itemId:'item',localSourceId,durationMs:2000,title:{kind:'song',title:'Actual title',artist:'',parentId:'album',parentTitle:'Album',groupId:'disc',groupTitle:'',groupNumber:1,number:1,year:2026,durationMs:2000,onServer:true}};
const source:LocalPlayerSource={kind:'local',scope,preparationId,localSourceId,entryId,itemId:'item',generation:1,startPositionMs:0,autoplay:true};
async function setup(load:NonNullable<NativeDownloadsBridge['loadLocal']>){
 const retired:number[]=[],released:string[]=[];
 const store=await DesktopDownloadStore.bind(scope,()=>true,{async bind(){return lease},async release(){},async command(_,operation){if(operation.action==='prepareLocalQueue')return {kind:'localQueue',value:{preparationId,scope,selectionRevision:'4',items:[member]}};released.push(String(operation.preparationId));return {kind:'done'};},loadLocal:load,async retireLocal(_,generation){retired.push(generation)}});
 const prepared=await store.prepareLocalQueue({kind:'entry',entryId,expectedRevision:4},new AbortController().signal);
 const broker=new DesktopLocalPlaybackBroker();broker.retain(store,prepared);return {store,prepared,broker,retired,released};
}
const receipt=(generation:number)=>({kind:'local',scope,preparationId,localSourceId,generation,sessionId:`local:${entryId}:1`,playId:'e'.repeat(64),entryId,itemId:'item',artifact:{sha256:'f'.repeat(64),bytes:8}});
test('only the exact native preparation can map an acknowledged private load to public generation',async()=>{
 const h=await setup(async(_,request)=>receipt(request.generation));
 try{
  const ack=await h.broker.load(source,101,{signal:new AbortController().signal,isCurrent:()=>true});
  assert.equal(ack.generation,1);assert.equal(ack.playId,'e'.repeat(64));assert.equal(ack.artifact.bytes,8);assert.doesNotMatch(JSON.stringify(ack),/lease|path|url/);
  await assert.rejects(h.broker.load({...source,scope:{...scope,sessionFamilyId:'other'}},102,{signal:new AbortController().signal,isCurrent:()=>true}),/downloads_conflict/);
  await h.broker.release({...h.prepared});assert.deepEqual(h.released,[]);
  await h.broker.release(h.prepared);assert.deepEqual(h.retired,[101]);assert.deepEqual(h.released,[preparationId]);
 }finally{await h.broker.dispose();h.store.dispose();}
});
test('viewer/operation retirement after native commit retires that exact load before publishing any public receipt',async()=>{
 let complete!:(value:unknown)=>void,current=true;
 const h=await setup(()=>new Promise(resolve=>{complete=resolve}));
 try{
  const pending=h.broker.load(source,107,{signal:new AbortController().signal,isCurrent:()=>current});
  current=false;complete(receipt(107));await assert.rejects(pending,{name:'AbortError'});assert.deepEqual(h.retired,[107]);
 }finally{await h.broker.dispose();h.store.dispose();}
});
