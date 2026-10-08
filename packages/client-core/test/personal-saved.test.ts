import test from 'node:test';
import assert from 'node:assert/strict';
import {PersonalSavedService,makeSavedDefinition,savedTitlePrefix,savedCategoryId} from '../src/personal-saved.ts';
const scope={serverId:'server',viewerId:'["local","account","profile"]'};
const media=(id='item')=>({id,libraryId:'library',kind:'movie',title:'Movie',available:true,playback:{itemId:id,startSeconds:0}});
const resource=(patch:any={})=>({serverId:'server',viewerFence:'fence',id:'collection',kind:'collection',name:'Collection',summary:'',visibility:'private',revision:1,role:'owner',entryCount:0,actions:['pin','update','delete','entries','share'],status:'ready',invalidComponents:[],pinned:false,pinRevision:0,shares:[],...patch});
const directory=(resources:any[]=[])=>({serverId:'server',viewerFence:'fence',revision:1,resources,nextCursor:''});
const content=(r=resource(),entries:any[]=[])=>({resource:r,entries,nextCursor:''});
const deferred=()=>{let resolve!:(v:any)=>void;const promise=new Promise<any>(r=>resolve=r);return{promise,resolve};};
const tick=()=>new Promise(r=>setTimeout(r,0));
function setup(request:(path:string,method?:string,body?:any,signal?:AbortSignal)=>Promise<any>){let n=0;return new PersonalSavedService({request},scope,async()=>`00000000-0000-4000-8000-${String(++n).padStart(12,'0')}`);}

test('collection directory and opaque membership use exact server IDs, roles and page positions',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);return p.includes('/content')?{...content(resource(),[{id:'member',hidden:false,media:media()},{id:'hidden',hidden:true}]),nextCursor:p.includes('cursor')?'':'next'}:directory([resource()]);});
 await s.select({view:'collections'});assert.equal(s.getSnapshot().resources[0].id,'collection');
 await s.select({view:'resource',resourceId:'collection'});assert.deepEqual(s.getSnapshot().entries.map(e=>e.id),['member','hidden']);
 await s.next();assert.equal(s.getSnapshot().cursor,'next');assert.deepEqual(s.getSnapshot().history,[null]);await s.previous();assert.equal(s.getSnapshot().cursor,null);assert(paths.some(p=>p.includes('cursor=next')));assert(Object.isFrozen(s.getSnapshot().entries));s.dispose();
});
test('collection network retry preserves exact operation and original CAS revision',async()=>{
 const writes:any[]=[];let revision=1;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({revision}));writes.push(b);if(writes.length===1)throw new Error('lost response');revision=2;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};});
 await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),false);assert(s.getSnapshot().retryPending);assert(await s.retry());assert.deepEqual(writes[0],writes[1]);assert.equal(writes[0].expectedRevision,1);assert.equal(s.getSnapshot().resource?.revision,2);s.dispose();
});
test('CAS conflict refresh requires an explicit rebase with a different operation ID',async()=>{
 const writes:any[]=[];let revision=1;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({revision}));writes.push(b);if(writes.length===1){revision=5;throw Object.assign(new Error('Changed'),{code:'playlist_conflict'});}revision=6;return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision,deleted:false}};});
 await s.select({view:'resource',resourceId:'collection'});await s.mutate({action:'update',name:'Changed'});assert(s.getSnapshot().mutationError?.conflict);assert.equal(writes.length,1);assert.equal(s.getSnapshot().resource?.revision,5);assert(await s.rebase());assert.notEqual(writes[0].operationId,writes[1].operationId);assert.equal(writes[1].expectedRevision,5);s.dispose();
});
test('current deletion status on replay does not resurrect a previously created collection',async()=>{
 const s=setup(async(p,m,b)=>m==='GET'?directory():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'deleted-collection',revision:1,deleted:false},current:{resourceId:'deleted-collection',revision:2,deleted:true}});
 await s.select({view:'collections'});assert(await s.mutate({action:'create',kind:'collection',name:'Collection'}));assert.deepEqual(s.getSnapshot().result,{resourceId:'deleted-collection',deleted:true});s.dispose();
});
test('viewer permissions and opaque member validation fail closed',async()=>{
 let writes=0;const s=setup(async(p,m)=>{if(m!=='GET')writes++;return content(resource({role:'viewer',actions:['pin']}));});await s.select({view:'resource',resourceId:'collection'});await assert.rejects(()=>s.mutate({action:'entries',addItemIds:['item']}),/not currently/);assert.equal(writes,0);s.dispose();
 for(const entries of [[{id:'hidden',hidden:true,media:media()}],[{id:'same',hidden:true},{id:'same',hidden:true}]]){const bad=setup(async()=>content(resource(),entries));await bad.select({view:'resource',resourceId:'collection'});assert.equal(bad.getSnapshot().error?.code,'invalid_saved');assert.equal(bad.getSnapshot().entries.length,0);bad.dispose();}
});
test('late reads and writes cannot publish into another resource or disposed scope',async()=>{
 const late=deferred();let body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return p.includes('/content')?content():directory();body=b;return late.promise;});await s.select({view:'resource',resourceId:'collection'});const change=s.mutate({action:'delete'});await tick();await s.select({view:'views'});await change;late.resolve({serverId:'server',viewerFence:'fence',receipt:{operationId:body.operationId,resourceId:'collection',revision:2,deleted:true}});await tick();assert.equal(s.getSnapshot().route?.view,'views');assert.equal(s.getSnapshot().result,null);s.dispose();
 const read=deferred();const x=setup(async()=>read.promise);const work=x.select({view:'collections'});x.dispose();await work;read.resolve(directory([resource()]));await tick();assert.equal(x.getSnapshot().resources.length,0);
});
test('history-only clear and reset-viewing-activity remain distinct idempotent commands',async()=>{
 const bodies:any[]=[];let revision=0;const s=setup(async(p,m,b)=>{if(m==='GET')return {serverId:'server',viewerFence:'fence',revision,entries:[{id:'occurrence',media:media(),updatedAt:'2026-09-06T12:00:00Z',positionSeconds:30,completed:false}],nextCursor:''};bodies.push(b);return {serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,revision:++revision,action:b.action}};});await s.select({view:'history'});await s.mutate({action:'clear-history'});await s.mutate({action:'reset-viewing-activity'});assert.deepEqual(bodies.map(b=>b.action),['clear-history','reset-viewing-activity']);assert.deepEqual(bodies.map(b=>b.expectedRevision),[0,1]);assert.notEqual(bodies[0].operationId,bodies[1].operationId);s.dispose();
});
test('saved query needs-review does not execute or normalize an unsupported definition',async()=>{
 const s=setup(async()=>content(resource({id:'query',kind:'view',actions:['pin','update','delete'],status:'needs-review',invalidComponents:['pivot'],definition:{pivot:'obsolete'}})));await s.select({view:'resource',resourceId:'query'});assert.equal(s.getSnapshot().resource?.status,'needs-review');assert.equal(s.getSnapshot().browse,null);assert.equal(s.getSnapshot().resource?.definition,undefined);assert.equal(s.getSnapshot().error,null);s.dispose();
 assert.throws(()=>makeSavedDefinition('','prefix'));assert.equal(savedTitlePrefix(makeSavedDefinition('library','Prefix')),'Prefix');assert.equal(savedCategoryId(makeSavedDefinition('library','','grid','title','asc','decade:1990')),'decade:1990');
});
test('pin uses its separate revision and remains available to a resource viewer',async()=>{
 let body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return content(resource({role:'viewer',actions:['pin'],pinRevision:3}));assert.equal(p,'/v1/saved-pins/collection/collection');body=b;return {serverId:'server',viewerFence:'fence',operationId:b.operationId,resourceId:'collection',revision:1,pinRevision:4,pinned:true,deleted:false};});await s.select({view:'resource',resourceId:'collection'});assert(await s.mutate({action:'pin',pinned:true}));assert.equal(body.expectedRevision,3);assert.equal(body.pinned,true);s.dispose();
});
test('scope navigation cancels stalled operation-ID acquisition without issuing a mutation',async()=>{
 const id=deferred();let writes=0;const s=new PersonalSavedService({request:async<T>(_p,m)=>{if(m!=='GET')writes++;return directory() as T;}},scope,()=>id.promise);await s.select({view:'collections'});const pending=s.mutate({action:'create',kind:'collection',name:'Name'});await s.select({view:'views'});assert.equal(await pending,false);id.resolve('00000000-0000-4000-8000-000000000001');await tick();assert.equal(writes,0);assert.equal(s.getSnapshot().pending,false);s.dispose();
});

test('collection receipts preserve added, unchanged and refused item outcomes for UI confirmation',async()=>{
 for(const entries of [
  {added:['item'],removed:[],unchanged:[],failed:[]},
  {added:[],removed:[],unchanged:['item'],failed:[]},
  {added:[],removed:[],unchanged:[],failed:[{itemId:'item',code:'item_unavailable'}]},
 ]){
  const s=setup(async(_p,m,b)=>m==='GET'?content():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision:2,deleted:false,entries}});
  await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),true);
  assert.deepEqual(s.getSnapshot().result?.entries,entries);assert.ok(Object.isFrozen(s.getSnapshot().result?.entries));s.dispose();
 }
});
test('malformed collection outcome cannot become a successful receipt',async()=>{
 const s=setup(async(_p,m,b)=>m==='GET'?content():{serverId:'server',viewerFence:'fence',receipt:{operationId:b.operationId,resourceId:'collection',revision:2,deleted:false,entries:{added:['item'],removed:[],unchanged:[],failed:[{itemId:'item',code:'denied'}]}}});
 await s.select({view:'resource',resourceId:'collection'});assert.equal(await s.mutate({action:'entries',addItemIds:['item']}),false);assert.equal(s.getSnapshot().result,null);s.dispose();
});
