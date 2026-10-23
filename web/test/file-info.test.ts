import test from 'node:test';
import assert from 'node:assert/strict';
import {FileInfoReader} from '../src/app/file-info.ts';
import type {DetailApi} from '../../packages/client-core/src/detail.ts';

const scope={serverId:'server',viewerId:'viewer'};
const target={libraryId:'library',itemId:'item'};
const page=(id='first',more='',extra:Record<string,unknown>={})=>({scope:{serverId:'server',libraryId:'library',itemId:'item',viewerFence:'fence'},revision:{catalog:1,viewer:2},kind:'movie',files:[{id,container:'mkv',durationMs:1000,size:10,available:true,audio:[],subtitles:[]}],nextCursor:more,...extra});
const api=(request:(path:string,signal?:AbortSignal)=>Promise<unknown>):DetailApi=>({request:async<T>(path:string,_method?:string,_body?:unknown,signal?:AbortSignal)=>await request(path,signal) as T});

test('file information is lazy, uses the canonical bounded endpoint, and retains every page',async()=>{
 const paths:string[]=[];
 const reader=new FileInfoReader(api(async path=>{paths.push(path);return paths.length===1?page('first','next'):page('second');}),scope,target,'fence');
 assert.equal(paths.length,0);
 await reader.load();await reader.load(true);
 assert.deepEqual(paths,['/v1/items/item/file-info?limit=50','/v1/items/item/file-info?cursor=next&limit=50']);
 assert.deepEqual(reader.getSnapshot().data?.files.map(file=>file.id),['first','second']);
 assert.equal(reader.getSnapshot().data?.manage,undefined);
});

for(const boundary of ['server','library','item','viewer','revision','permissions','duplicate','cursor-loop','member-path'] as const)test(`file pages fail closed across ${boundary} boundary`,async()=>{
 let calls=0;
 const reader=new FileInfoReader(api(async()=>{
  if(++calls===1)return page('first','next');
  const value=page('second');
  if(boundary==='server')value.scope.serverId='other';
  if(boundary==='library')value.scope.libraryId='other';
  if(boundary==='item')value.scope.itemId='other';
  if(boundary==='viewer')value.scope.viewerFence='other';
  if(boundary==='revision')value.revision.catalog++;
  if(boundary==='permissions')return {...value,manage:{delete:true,split:true,merge:true},files:[{...value.files[0],path:'/owner/private.mkv'}]};
  if(boundary==='duplicate')value.files[0]!.id='first';
  if(boundary==='cursor-loop')value.nextCursor='next';
  if(boundary==='member-path')return {...value,files:[{...value.files[0],path:'/owner/private.mkv'}]};
  return value;
 }),scope,target,'fence');
 await reader.load();await reader.load(true);
 assert.equal(reader.getSnapshot().phase,'error');assert.equal(reader.getSnapshot().data,undefined);
});

test('refresh refusal clears owner facts and late aborted/disposed reads cannot publish them',async()=>{
 let finish:((value:unknown)=>void)|undefined, signal:AbortSignal|undefined, calls=0;
 const ownerPage={...page(),manage:{delete:true,split:true,merge:true},files:[{...page().files[0],path:'/owner/private.mkv'}]};
 const reader=new FileInfoReader(api(async(_path,abort)=>{signal=abort;if(++calls===1)return ownerPage;if(calls===2)throw new Error('permission revoked');return new Promise(resolve=>{finish=resolve;});}),scope,target,'fence');
 await reader.load();assert.equal(reader.getSnapshot().data?.files[0]?.path,'/owner/private.mkv');
 await reader.load();assert.equal(reader.getSnapshot().data,undefined);assert.equal(reader.getSnapshot().phase,'error');
 const pending=reader.load();reader.dispose();assert.ok(signal?.aborted);finish!(ownerPage);await pending;
 assert.equal(reader.getSnapshot().data,undefined);assert.equal(reader.getSnapshot().phase,'idle');
});

test('a newer read supersedes an old response even when its transport ignores cancellation',async()=>{
 let finish:((value:unknown)=>void)|undefined,calls=0;
 const reader=new FileInfoReader(api(async()=>++calls===1?new Promise(resolve=>{finish=resolve;}):page('current')),scope,target,'fence');
 const old=reader.load();await reader.load();finish!(page('stale'));await old;
 assert.deepEqual(reader.getSnapshot().data?.files.map(file=>file.id),['current']);
});
