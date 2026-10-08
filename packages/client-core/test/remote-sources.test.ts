import test from 'node:test';
import assert from 'node:assert/strict';
import {RemoteSourceService,readSourceAvailability,sourceAvailabilityMessage} from '../src/remote-sources.ts';
import type {LibraryContentApi} from '../src/library-content.ts';
const scope={serverId:'server',viewerId:'owner'},wire={serverId:'server',viewerFence:'fence'};
const op='1788580000000-12345678-1234-1234-1234-123456789012';
const input={name:'DAV',root:'https://private.example/media/',username:'private-user',password:'private-password',keepConnection:false,keepPassword:false,insecureLocal:false};
const source=(extra:Record<string,unknown>={})=>({id:'dav',kind:'webdav',name:'DAV',rootPath:'/server/remote/dav',generation:1,state:'ready',rangeSupport:'conditional',insecureLocal:false,credentialPresent:true,origin:'https://private.example',...extra});
const list=(rows=[source()])=>({scope:wire,sources:rows,mountSupported:false});
const receipt=(extra:Record<string,unknown>={})=>({scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source(),...extra}});
function api(fn:(path:string,method:string,body:unknown,signal?:AbortSignal)=>Promise<unknown>):LibraryContentApi{return {request:<T>(p:string,m='GET',b?:unknown,s?:AbortSignal)=>fn(p,m,b,s) as Promise<T>};}
function deferred(){let resolve!:(v:unknown)=>void;return {promise:new Promise<unknown>(r=>{resolve=r;}),resolve:(v:unknown)=>resolve(v)};}
test('private connection body is never retained; lost response recovery reads one receipt only',async()=>{
 const calls:{path:string,method:string,body:unknown}[]=[];let ids=0;
 const service=new RemoteSourceService(api(async(path,method,body)=>{calls.push({path,method,body});if(method==='POST')throw new Error(input.password);return path.includes('/source-operations/')?receipt():list();}),scope,async()=>{ids++;return op;});
 await service.load();await service.configure(null,input);
 assert.equal(service.getSnapshot().ambiguous,true);
 assert(!JSON.stringify(service.getSnapshot()).includes(input.password));assert(!JSON.stringify(service.getSnapshot()).includes(input.username));
 await service.recover();assert.equal(service.getSnapshot().ambiguous,false);assert.equal(service.getSnapshot().receipt?.operationId,op);
 assert.equal(ids,1);assert.equal(calls.filter(c=>c.method==='POST').length,1);assert.equal(calls.find(c=>c.method==='POST')?.body && (calls.find(c=>c.method==='POST')!.body as any).password,input.password);
 assert(calls.some(c=>c.path.endsWith('/source-operations/'+op)&&c.method==='GET'&&c.body===undefined));service.dispose();
});
test('unsafe source responses and changed owner fences fail closed',async()=>{
 for(const row of [source({password:'secret'}),source({root:'https://private/'}),source({origin:'https://user:secret@private.example'}),source({rangeSupport:'invented'})]){
  const service=new RemoteSourceService(api(async()=>list([row])),scope,async()=>op);await service.load();assert.equal(service.getSnapshot().sources.length,0);assert(service.getSnapshot().error);service.dispose();
 }
 let changed=false;
 const service=new RemoteSourceService(api(async()=>changed?{...list(),scope:{...wire,viewerFence:'changed'}}:list()),scope,async()=>op);
 await service.load();assert.equal(service.getSnapshot().sources.length,1);changed=true;await service.load();assert.equal(service.getSnapshot().sources.length,0);service.dispose();
});
test('late list response cannot repopulate disposed owner state or initiate writes',async()=>{
 const d=deferred();let writes=0;
 const service=new RemoteSourceService(api(async(_p,m)=>{if(m!=='GET')writes++;return d.promise;}),scope,async()=>op);
 const pending=service.load();service.dispose();d.resolve(list());await pending;
 assert.equal(service.getSnapshot().sources.length,0);await assert.rejects(service.configure(null,input));await assert.rejects(service.networkPolicy('library'));assert.equal(writes,0);
});
test('explicit configuration rejection retains no secrets and does not become automatic replay',async()=>{
 let writes=0;const service=new RemoteSourceService(api(async(_p,m)=>{if(m!=='GET'){writes++;throw Object.assign(new Error(input.password),{code:'remote_credentials_required'});}return list();}),scope,async()=>op);
 await service.configure(null,input);assert.equal(service.getSnapshot().ambiguous,false);assert.match(service.getSnapshot().error,/credentials/);assert(!JSON.stringify(service.getSnapshot()).includes(input.password));assert.equal(writes,1);service.dispose();
});
test('network and analysis policies submit expected revisions and read scoped metadata',async()=>{
 const calls:any[]=[];let networkRevision=4;
 const service=new RemoteSourceService(api(async(path,method,body)=>{calls.push({path,method,body});if(path.endsWith('/network-roots')){if(method==='POST'){networkRevision++;return {approvals:[]};}return {scope:wire,policy:{libraryId:'lib',revision:networkRevision,origins:['https://private.example']}};}return {scope:wire,policy:{libraryId:'lib',revision:method==='PUT'?3:2,enabled:method==='PUT'}};}),scope,async()=>op);
 const network=await service.networkPolicy('lib');const next=await service.setNetworkPolicy(network,['https://private.example/media/']);assert.equal(next.revision,5);assert.deepEqual(calls.find(c=>c.method==='POST').body,{roots:['https://private.example/media/'],expectedRevision:4});
 const policy=await service.policy('lib');assert.equal((await service.setPolicy(policy,true)).revision,3);assert.deepEqual(calls.find(c=>c.method==='PUT').body,{enabled:true,expectedRevision:2});service.dispose();
});
test('viewer availability enforces item identity, enum and cancellation with safe shared copy',async()=>{
 const c=new AbortController();assert.equal(await readSourceAvailability(api(async()=>({itemId:'item',state:'resolve_on_play'})),'item',c.signal),'resolve_on_play');
 for(const result of [{itemId:'other',state:'ready'},{itemId:'item',state:'https://user:password@private'}])await assert.rejects(readSourceAvailability(api(async()=>result),'item',c.signal));
 c.abort();await assert.rejects(readSourceAvailability(api(async()=>({itemId:'item',state:'ready'})),'item',c.signal));assert.match(sourceAvailabilityMessage('changed'),/refresh/);assert.equal(sourceAvailabilityMessage('ready'),'');
});

test('STRM requested opt-in remains distinct from effective File List Only policy',async()=>{
 const policy={libraryId:'lib',revision:3,enabled:true,effective:false,scanTier:'file_list_only'};
 const service=new RemoteSourceService(api(async()=>({scope:wire,policy})),scope,async()=>op);
 const got=await service.policy('lib');assert.equal(got.enabled,true);assert.equal(got.effective,false);assert.equal(got.scanTier,'file_list_only');service.dispose();
 for(const patch of [{effective:'yes'},{scanTier:'unbounded'}]){
  const bad=new RemoteSourceService(api(async()=>({scope:wire,policy:{...policy,...patch}})),scope,async()=>op);
  await assert.rejects(bad.policy('lib'));bad.dispose();
 }
});
