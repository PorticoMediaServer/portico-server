import test from 'node:test';
import assert from 'node:assert/strict';
import {PreparedMediaService,parsePreparedView,parsePreparedChoice,preparedChoice} from '../src/prepared-media.ts';
const sha='a'.repeat(64);
function view():any{return {itemId:'item',canManage:true,configured:true,configurationReason:'',preparedOffersRevision:sha,pollAfterMs:3000,profiles:[{id:'portable-720-v1',name:'Portable',kind:'video',width:1280,height:720,videoKbps:2500,audioKbps:192,description:'Keeps original'}],targets:[{id:'prepared-library',name:'This server'}],sources:[{id:'source',revision:sha,partIndex:0,container:'mp4',height:1080,available:true,reason:''}],jobs:[],versions:[{id:'version',itemId:'item',sourceId:'source',profileId:'portable-720-v1',targetId:'prepared-library',sourceRevision:sha,partIndex:0,editionId:null,digest:sha,size:256,facts:{container:'mp4',videoCodec:'h264',audioCodec:'aac',width:1280,height:720,duration:60,audioTracks:1},state:'published',revision:1,createdMs:1,selectable:true,reason:''}]};}
function fixture(){let raw=view(),error:unknown=null;let serial=0;const posts:{path:string;body:any}[]=[];const api={request:async<T>(path:string,method?:string,body?:unknown):Promise<T>=>{if(method==='POST'){posts.push({path,body:structuredClone(body)});if(error)throw error;return {} as T;}return structuredClone(raw) as T;}};const service=new PreparedMediaService({api,scope:{serverId:'server',viewerId:'viewer'},itemId:'item',requestId:()=>`request-${++serial}`});return {service,posts,setError:(e:unknown)=>{error=e;},setView:(v:any)=>{raw=v;},get keys(){return serial;}};}
test('prepared version offers are bounded, scoped and separate from original identity',()=>{const data=parsePreparedView(view(),'item');assert(Object.isFrozen(data.versions));assert.equal(data.versions[0].editionId,null);assert.deepEqual(preparedChoice(data,data.versions[0]),{versionId:'version',expectedRevision:1,offersRevision:sha});for(const change of [(v:any)=>v.itemId='other',(v:any)=>v.versions[0].itemId='other',(v:any)=>v.versions[0].facts.duration=Infinity,(v:any)=>v.versions[0].facts.audioTracks=17,(v:any)=>v.versions[0].state='deleting',(v:any)=>v.versions.push(v.versions[0]),(v:any)=>v.sources[0].revision='path-to-source']){const v=view();change(v);assert.throws(()=>parsePreparedView(v,'item'));}const offline=view();offline.sources[0]={...offline.sources[0],available:false,revision:'',reason:'source_unavailable'};assert.equal(parsePreparedView(offline,'item').versions[0].selectable,true,'offline original does not disable a valid derivative');});
test('uncertain optimization response retains exact immutable mutation identity',async t=>{const f=fixture();t.after(()=>f.service.dispose());await f.service.refresh();const v=f.service.getSnapshot().data!;f.setError(new Error('response lost'));await f.service.submit(v.sources[0],v.profiles[0],v.targets[0].id);assert.equal(f.service.getSnapshot().uncertain,true);await f.service.submit(v.sources[0],v.profiles[0],v.targets[0].id);assert.equal(f.keys,1);f.setError(null);await f.service.retryPending();assert.equal(f.posts.length,2);assert.deepEqual(f.posts[1],f.posts[0]);assert.equal(f.service.getSnapshot().uncertain,false);});
test('definitive CAS rejection stays visible after successful refresh; next request gets new identity',async t=>{const f=fixture();t.after(()=>f.service.dispose());await f.service.refresh();const v=f.service.getSnapshot().data!;f.setError(Object.assign(new Error(),{status:409}));await f.service.remove(v.versions[0]);assert.equal(f.service.getSnapshot().uncertain,false);assert.match(f.service.getSnapshot().error!,/changed/);f.setError(null);await f.service.remove(v.versions[0]);assert.equal(f.keys,2);assert.notEqual(f.posts[0].body.idempotencyKey,f.posts[1].body.idempotencyKey);});
test('denied current authority clears cached data and cannot be masked by refresh',async t=>{const f=fixture();t.after(()=>f.service.dispose());await f.service.refresh();const v=f.service.getSnapshot().data!;f.setError(Object.assign(new Error(),{status:403}));await f.service.remove(v.versions[0]);assert.equal(f.service.getSnapshot().data,null);assert.match(f.service.getSnapshot().error!,/owner/);});
test('read-only viewers cannot submit or delete, and disposed reads do not publish',async()=>{const f=fixture();const v=view();v.canManage=false;f.setView(v);await f.service.refresh();await f.service.remove(f.service.getSnapshot().data!.versions[0]);assert.equal(f.posts.length,0);f.service.dispose();const prior=f.service.getSnapshot();await f.service.refresh();assert.equal(f.service.getSnapshot(),prior);f.service.connect();await f.service.refresh();assert.equal(f.service.getSnapshot().data?.canManage,false);f.service.dispose();});
test('a read that ignores cancellation cannot restore prepared authority after denial',async()=>{
 let finish!:(v:unknown)=>void;let initial=true;
 const api={request:async<T>(_path:string,method?:string):Promise<T>=>{
  if(method==='POST')throw Object.assign(new Error('denied'),{status:403});
  if(initial){initial=false;return view() as T;}
  return await new Promise<unknown>(resolve=>{finish=resolve;}) as T;
 }};
 const service=new PreparedMediaService({api,scope:{serverId:'server',viewerId:'viewer'},itemId:'item',requestId:()=> 'request'});
 await service.refresh();const version=service.getSnapshot().data!.versions[0];
 const stale=service.refresh();await service.remove(version);
 assert.equal(service.getSnapshot().data,null);
 finish(view());await stale;
 assert.equal(service.getSnapshot().data,null);assert.equal(service.getSnapshot().loading,false);
 assert.match(service.getSnapshot().error!,/owner/);service.dispose();
});

test('prepared facts reject empty media, partial dimensions and unsupported video dimensions',()=>{
 for(const facts of [
  {videoCodec:'',audioCodec:'',audioTracks:0,width:0,height:0},
  {videoCodec:'',audioCodec:'aac',audioTracks:1,width:0,height:720},
  {videoCodec:'h264',audioCodec:'aac',audioTracks:1,width:3840,height:2160},
 ]){const v=view();Object.assign(v.versions[0].facts,facts);assert.throws(()=>parsePreparedView(v,'item'));}
});
