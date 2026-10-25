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
test('an unsafe source is never listed, and malformed lists and changed owner fences fail closed',async()=>{
 for(const row of [source({password:'secret'}),source({root:'https://private/'}),source({origin:'https://user:secret@private.example'}),source({kind:'ftp'}),source({generation:0})]){
  const service=new RemoteSourceService(api(async()=>list([row,source({id:'other'})])),scope,async()=>op);await service.load();assert.deepEqual(service.getSnapshot().sources.map(s=>s.id),['other']);assert(!JSON.stringify(service.getSnapshot()).includes('secret'));service.dispose();
 }
 for(const raw of [{...list(),sources:{}},{...list(),mountSupported:'yes'},{...list(),scope:{...wire,serverId:'other'}}]){
  const service=new RemoteSourceService(api(async()=>raw),scope,async()=>op);await service.load();assert.equal(service.getSnapshot().sources.length,0);assert(service.getSnapshot().error);service.dispose();
 }
 // A state this app does not know is kept as the server named it, unknown range support reads as unknown,
 // an error code it does not know is left off, and a repeated source keeps its first row.
 const newer=new RemoteSourceService(api(async()=>({...list([source({state:'warming_up',rangeSupport:'invented',errorCode:'solar_flare',futureField:1}),source({name:'Again'})]),futureField:true})),scope,async()=>op);
 await newer.load();assert.equal(newer.getSnapshot().error,'');assert.deepEqual(newer.getSnapshot().sources,[{...source({state:'warming_up',rangeSupport:'unknown'})}]);newer.dispose();
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
 for(const patch of [{effective:'yes'},{revision:-1},{libraryId:'other'}]){
  const bad=new RemoteSourceService(api(async()=>({scope:wire,policy:{...policy,...patch}})),scope,async()=>op);
  await assert.rejects(bad.policy('lib'));bad.dispose();
 }
 // A scan tier this app does not know is left off; the policy and its revision are kept.
 const newer=new RemoteSourceService(api(async()=>({scope:wire,policy:{...policy,scanTier:'unbounded'}})),scope,async()=>op);
 assert.deepEqual(await newer.policy('lib'),{libraryId:'lib',enabled:true,revision:3,effective:false});newer.dispose();
});

test('a quiet re-read keeps the sources on screen, and a failed one leaves them as they were',async()=>{
 let state='ready',fail=false;const seen:string[]=[];
 const service=new RemoteSourceService(api(async()=>{if(fail)throw new TypeError('Failed to fetch');return list([source({state})]);}),scope,async()=>op);
 await service.load();service.subscribe(()=>{const x=service.getSnapshot();seen.push((x.loading?'loading':'idle')+':'+(x.sources[0]?.state??'-'));});
 state='unreachable';await service.revalidate();
 assert.equal(seen.every(x=>x.startsWith('idle:')),true,'never loading');assert.equal(service.getSnapshot().sources[0]!.state,'unreachable');
 fail=true;await assert.rejects(service.revalidate());
 assert.equal(service.getSnapshot().sources.length,1);assert.equal(service.getSnapshot().error,'');service.dispose();
});
test('a share changed elsewhere is read again and the command sent once more, silently',async()=>{
 const conflict=()=>Object.assign(new Error('changed'),{code:'storage_command_conflict'});
 // Removal: the share was edited elsewhere (generation 2); it is removed at its new generation.
 let rows=[source({generation:2})];const writes:any[]=[];
 const removing=new RemoteSourceService(api(async(_p,m,b)=>{if(m==='GET')return list(rows);writes.push(b);if((b as any).expectedGeneration!==2)throw conflict();rows=[];return {scope:wire,receipt:{operationId:op,accepted:true,removed:true,source:source({generation:3,state:'removed'})}};}),scope,async()=>op);
 await removing.load();await removing.remove(source() as any);
 assert.deepEqual(writes.map(w=>w.expectedGeneration),[1,2]);assert(removing.getSnapshot().receipt);assert.equal(removing.getSnapshot().error,'');removing.dispose();
 // Removal of a share already removed elsewhere is done, not a failure.
 const gone=new RemoteSourceService(api(async(_p,m)=>{if(m==='GET')return list([]);throw conflict();}),scope,async()=>op);
 await gone.remove(source() as any);assert.equal(gone.getSnapshot().alreadyDone,true);assert.equal(gone.getSnapshot().error,'');gone.dispose();
 // An edit: someone renamed the share, the owner only switched "insecure local"; the rename is kept.
 const sent:any[]=[];
 const editing=new RemoteSourceService(api(async(_p,m,b)=>{if(m==='GET')return list([source({generation:2,name:'Renamed'})]);sent.push(b);if((b as any).expectedGeneration!==2)throw conflict();return receipt();}),scope,async()=>op);
 const base=source() as any;
 await editing.configure('dav',{name:'DAV',root:'',username:'',password:'',keepPassword:true,keepConnection:true,insecureLocal:true,expectedGeneration:1},base);
 assert.equal(sent.length,2);assert.equal(sent[1].name,'Renamed');assert.equal(sent[1].insecureLocal,true);assert.equal(sent[1].expectedGeneration,2);assert(editing.getSnapshot().receipt);editing.dispose();
 // Both renamed it: that is the one case the owner is told about, and nothing is re-sent.
 let tries=0;
 const both=new RemoteSourceService(api(async(_p,m)=>{if(m==='GET')return list([source({generation:2,name:'Theirs'})]);tries++;throw conflict();}),scope,async()=>op);
 await both.configure('dav',{name:'Mine',root:'',username:'',password:'',keepPassword:true,keepConnection:true,insecureLocal:false,expectedGeneration:1},base);
 assert.equal(tries,1);assert.equal(both.getSnapshot().errorCode,'storage_command_conflict');assert.match(both.getSnapshot().error,/changed/);both.dispose();
});
test('an edit is carried onto the share as it is now, field by field',async()=>{
 const {rebaseDavEdit}=await import('../src/remote-sources.ts');
 const base=source() as any,keep={name:'DAV',root:'',username:'',password:'',keepPassword:true,keepConnection:true,insecureLocal:false,expectedGeneration:1};
 // The connection was changed elsewhere and the owner kept theirs as it was: fine, the server keeps the new one.
 assert.equal(rebaseDavEdit(base,source({generation:4,origin:'https://other.example'}) as any,{...keep,name:'New'})?.expectedGeneration,4);
 // Both changed the connection (a new password typed here): conflict.
 assert.equal(rebaseDavEdit(base,source({generation:4,origin:'https://other.example'}) as any,{...keep,keepPassword:false,password:'p'}),undefined);
 // Both made the same change: no conflict.
 assert.equal(rebaseDavEdit(base,source({generation:4,name:'Same'}) as any,{...keep,name:'Same'})?.name,'Same');
});


test('saved native and imported endpoint facts remain visible without a scheme alias',async()=>{
 const rows=[
  source({id:'sftp',kind:'sftp',origin:'sftp://nas.example:22/'}),
  source({id:'smb',kind:'smb',origin:'smb://192.168.1.9:445/'}),
  source({id:'ipv6',kind:'sftp',origin:'sftp://[fd00::9]:2222/'}),
  source({id:'imported',kind:'rclone',origin:'https://storage.example:443/'}),
  source({id:'wrapped',kind:'rclone',origin:'sftp://nas.example:22/'}),
 ];
 const service=new RemoteSourceService(api(async()=>list(rows)),scope,async()=>op);
 await service.load();
 assert.equal(service.getSnapshot().error,'');
 assert.deepEqual(service.getSnapshot().sources.map(row=>[row.id,row.kind,row.origin]),rows.map(row=>[row.id,row.kind,row.origin]));
 service.dispose();
 for(const row of [
  source({kind:'sftp',origin:'https://nas.example:22/'}),
  source({kind:'smb',origin:'sftp://nas.example:445/'}),
  source({origin:'sftp://nas.example:22/'}),
  source({kind:'sftp',origin:'sftp://nas.example/'}),
  source({kind:'smb',origin:'smb://nas.example:0/'}),
  source({kind:'sftp',origin:'sftp://nas.example:65536/'}),
  source({kind:'sftp',origin:'sftp://owner:secret@nas.example:22/'}),
  source({kind:'smb',origin:'smb://nas.example:445/share'}),
  source({kind:'rclone',origin:'https://storage.example:443/?token=secret'}),
  source({kind:'rclone',origin:'https://storage.example:443/#secret'}),
  source({kind:'rclone',origin:'https://storage.example:443/secret'}),
  source({kind:'sftp',origin:'sftp://nas%2eexample:22/'}),
  source({kind:'smb',origin:'file://nas.example:445/'}),
 ]){
  const invalid=new RemoteSourceService(api(async()=>list([row,source({id:'safe'})])),scope,async()=>op);
  await invalid.load();assert.deepEqual(invalid.getSnapshot().sources.map(row=>row.id),['safe']);
  assert(!JSON.stringify(invalid.getSnapshot()).includes('secret'));invalid.dispose();
 }
});

test('setup receipts retain actual range facts across source generations and recovery',async()=>{
 for(const support of ['conditional','unsupported','unknown','sequential']){
  const saved=source({rangeSupport:support,generation:2});
  let lost=true,writes=0;
  const service=new RemoteSourceService(api(async(path,method)=>{
   if(method==='PUT'){writes++;if(lost)throw new Error(input.password);}
   if(path.includes('/source-operations/')||method==='PUT')return {...receipt(),receipt:{operationId:op,accepted:true,removed:false,source:saved}};
   return list([saved]);
  }),scope,async()=>op);
  await service.configure('dav',{...input,expectedGeneration:1});
  assert.equal(service.getSnapshot().ambiguous,true);
  lost=false;await service.recover();
  assert.equal(writes,1,'recovery never repeats a credential-bearing command');
  assert.equal(service.getSnapshot().receipt?.source.rangeSupport,support);
  assert.equal(service.getSnapshot().receipt?.source.generation,2);
  assert.equal(service.getSnapshot().sources[0]?.rangeSupport,support);
  assert(!JSON.stringify(service.getSnapshot()).includes(input.password));service.dispose();
 }
});

test('all saved sources remain listed beyond the former count ceiling',async()=>{
 const rows=Array.from({length:24},(_,i)=>source({id:`source-${i}`,name:`Source ${i}`}));
 const service=new RemoteSourceService(api(async()=>list(rows)),scope,async()=>op);
 await service.load();assert.equal(service.getSnapshot().error,'');
 assert.deepEqual(service.getSnapshot().sources.map(row=>row.id),rows.map(row=>row.id));
 service.dispose();
});


test('source quota and throttle facts survive without a guessed provider wait time',async()=>{
 const rows=[source({id:'quota',state:'degraded',errorCode:'quota_exceeded'}),source({id:'throttled',state:'rate_limited',errorCode:'rate_limited'})];
 const service=new RemoteSourceService(api(async()=>list(rows)),scope,async()=>op);
 await service.load();
 assert.deepEqual(service.getSnapshot().sources.map(row=>[row.id,row.state,row.errorCode]),[
  ['quota','degraded','quota_exceeded'],['throttled','rate_limited','rate_limited'],
 ]);
 for(const row of service.getSnapshot().sources){
  assert(!Object.hasOwn(row,'retryAfterSeconds'));
  assert(!Object.hasOwn(row,'retryAt'));
 }
 service.dispose();
});


test('an unsaved connection test sends credentials once and retains only its safe result',async()=>{
 const calls:{path:string;method:string;body:unknown}[]=[];let operations=0;
 const service=new RemoteSourceService(api(async(path,method,body)=>{calls.push({path,method,body});return {scope:wire,result:{origin:'https://private.example',rangeSupport:'conditional'}};}),scope,async()=>{operations++;return op;});
 const result=await service.testConnection(input);
 assert.deepEqual(result,{origin:'https://private.example',rangeSupport:'conditional'});
 assert.equal(calls.length,1);assert.equal(calls[0]!.path,'/v1/storage/sources/webdav/test');assert.equal(calls[0]!.method,'POST');
 assert.deepEqual(calls[0]!.body,{name:input.name,root:input.root,username:input.username,password:input.password,insecureLocal:false});
 assert.equal(operations,0);assert.equal(service.getSnapshot().operationId,null);assert.equal(service.getSnapshot().receipt,null);assert.deepEqual(service.getSnapshot().sources,[]);
 assert(!JSON.stringify(service.getSnapshot()).includes(input.password));assert(!JSON.stringify(service.getSnapshot()).includes(input.username));service.dispose();
});

test('a failed unsaved test is never retried or exposed as a credential-bearing error',async()=>{
 let calls=0;const service=new RemoteSourceService(api(async()=>{calls++;throw new Error(input.password);}),scope,async()=>op);
 await assert.rejects(service.testConnection(input),(error:unknown)=>error instanceof Error&&!error.message.includes(input.password));
 assert.equal(calls,1);assert.equal(service.getSnapshot().ambiguous,false);assert.equal(service.getSnapshot().operationId,null);assert(!JSON.stringify(service.getSnapshot()).includes(input.password));service.dispose();
});

test('late unsaved test results cannot cross a disposed owner scope',async()=>{
 const pending=deferred();const service=new RemoteSourceService(api(async()=>pending.promise),scope,async()=>op);
 const tested=service.testConnection(input);service.dispose();pending.resolve({scope:wire,result:{origin:'https://private.example',rangeSupport:'conditional'}});
 await assert.rejects(tested,/Scope changed/);assert.equal(service.getSnapshot().sources.length,0);
});

test('read-profile reads the server values and the four drawn presets match its SourceReadPreset',async()=>{
 const {sourceReadPreset,sourceReadPresetName}=await import('../src/remote-sources.ts');
 assert.deepEqual(sourceReadPreset('list'),{scanDepth:'file_list_only',wholeFileReads:false,connections:2,requestsPerSecond:2,bytesPerSecond:0});
 assert.deepEqual(sourceReadPreset('once'),{scanDepth:'basic',wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0});
 assert.deepEqual(sourceReadPreset('everything'),{scanDepth:'complete',wholeFileReads:true,connections:8,requestsPerSecond:10,bytesPerSecond:0});
 assert.deepEqual(sourceReadPreset('ask'),{scanDepth:'basic',wholeFileReads:false,connections:2,requestsPerSecond:2,bytesPerSecond:0});
 assert.equal(sourceReadPresetName(sourceReadPreset('once')),'once');assert.equal(sourceReadPresetName({...sourceReadPreset('once'),connections:5}),undefined);
 const settings={sourceId:'dav',generation:'1',revision:2,saved:true,values:sourceReadPreset('everything')};
 const service=new RemoteSourceService(api(async(path,method)=>{assert.equal(path,'/v1/storage/sources/dav/settings/read-profile');assert.equal(method,'GET');return {scope:wire,settings};}),scope,async()=>op);
 assert.deepEqual(await service.readProfile('dav'),settings);service.dispose();
 for(const raw of [{...settings,values:{...settings.values,connections:0}},{...settings,values:{...settings.values,scanDepth:'custom'}},{...settings,revision:-1},{...settings,saved:'yes'}]){
  const bad=new RemoteSourceService(api(async()=>({scope:wire,settings:raw})),scope,async()=>op);await assert.rejects(bad.readProfile('dav'));bad.dispose();
 }
 const other=new RemoteSourceService(api(async()=>({scope:{...wire,serverId:'other'},settings})),scope,async()=>op);await assert.rejects(other.readProfile('dav'));other.dispose();
});

test('saving a read profile sends the exact revisioned body and returns the receipt settings',async()=>{
 const settings={sourceId:'dav',generation:'7',revision:3,saved:true,values:{scanDepth:'complete' as const,wholeFileReads:true,connections:8,requestsPerSecond:10,bytesPerSecond:0}};
 let body:any;
 const service=new RemoteSourceService(api(async(path,method,raw)=>{if(method==='GET')return list();assert.equal(path,'/v1/storage/sources/dav/settings/read-profile');assert.equal(method,'PUT');body=raw;return {scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source({generation:7}),readSettings:settings}};}),scope,async()=>op);
 await service.load();const value=await service.saveProfile({sourceId:'dav',generation:'7',revision:2,saved:true,values:{scanDepth:'basic',wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0}},settings.values);
 assert.deepEqual(body,{operationId:op,expectedGeneration:'7',expectedRevision:2,values:settings.values});assert.deepEqual(value,settings);assert.equal(service.getSnapshot().receipt?.operationId,op);assert.equal(service.getSnapshot().busy,false);service.dispose();
});

test('read-profile binds the returned source and a refresh failure never becomes an ambiguous write',async()=>{
 const {sourceReadPreset}=await import('../src/remote-sources.ts');
 const foreign={sourceId:'other',generation:'1',revision:1,saved:true,values:sourceReadPreset('once')};
 const read=new RemoteSourceService(api(async()=>({scope:wire,settings:foreign})),scope,async()=>op);
 await assert.rejects(read.readProfile('dav'));read.dispose();
 const before={sourceId:'dav',generation:'1',revision:1,saved:true,values:sourceReadPreset('once')};
 const wrongReceipt=new RemoteSourceService(api(async()=>({scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source({id:'other'}),readSettings:{...foreign,sourceId:'other'}}})),scope,async()=>op);
 await assert.rejects(wrongReceipt.saveProfile(before,sourceReadPreset('once')));wrongReceipt.dispose();
 const wrongSettings=new RemoteSourceService(api(async()=>({scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source(),readSettings:{...foreign,sourceId:'other'}}})),scope,async()=>op);
 await assert.rejects(wrongSettings.saveProfile(before,sourceReadPreset('once')));wrongSettings.dispose();
 let puts=0,gets=0;
 const refresh=new RemoteSourceService(api(async(_path,method)=>{if(method==='PUT'){puts++;return {scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source()}};}gets++;throw new Error('refresh failed');}),scope,async()=>op);
 await assert.rejects(refresh.saveProfile(before,sourceReadPreset('once')),/refreshed values could not be read/);
 assert.equal(puts,1);assert.equal(gets,1);assert.equal(refresh.getSnapshot().ambiguous,false);assert.equal(refresh.getSnapshot().errorCode,'refresh_failed');assert.equal(refresh.getSnapshot().receipt?.operationId,op);refresh.dispose();
});

test('read-profile and save transport failures never retain a hostile message',async()=>{
 const secret='https://private-user:private-password@private.example/media/';
 const read=new RemoteSourceService(api(async()=>{throw Object.assign(new Error(secret),{code:'remote_offline'});}),scope,async()=>op);
 await assert.rejects(read.readProfile('dav'),error=>{const text=String(error);return !text.includes('private-user')&&!text.includes('private-password')&&!text.includes('private.example');});
 assert(!JSON.stringify(read.getSnapshot()).includes('private-password'));read.dispose();
 const save=new RemoteSourceService(api(async(_path,method)=>{if(method==='PUT')throw Object.assign(new Error(secret),{code:'remote_offline'});return list();}),scope,async()=>op);
 const before={sourceId:'dav',generation:'1',revision:1,saved:true,values:{scanDepth:'basic' as const,wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0}};
 await assert.rejects(save.saveProfile(before,before.values),error=>!String(error).includes('private-password'));
 assert(!JSON.stringify(save.getSnapshot()).includes('private-password'));save.dispose();
});

test('retiring the captured scope during a held operation id sends zero mutations',async()=>{
 const held=deferred();let current=true,writes=0;
 const service=new RemoteSourceService(api(async(_path,method)=>{if(method!=='GET')writes++;return list();}),scope,()=>held.promise,()=>current);
 const configure=service.configure(null,input);current=false;held.resolve(op);
 await configure;assert.equal(writes,0);service.dispose();
 current=true;const heldAgain=deferred();
 const profile={sourceId:'dav',generation:'1',revision:1,saved:true,values:{scanDepth:'basic' as const,wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0}};
 const saveService=new RemoteSourceService(api(async(_path,method)=>{if(method!=='GET')writes++;return list();}),scope,()=>heldAgain.promise,()=>current);
 const save=saveService.saveProfile(profile,profile.values);current=false;heldAgain.resolve(op);
 await assert.rejects(save);assert.equal(writes,0);saveService.dispose();
});

test('a confirmed save whose fallback read refuses auth keeps the receipt and reports the refusal',async()=>{
 const before={sourceId:'dav',generation:'1',revision:1,saved:true,values:{scanDepth:'basic' as const,wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0}};
 for(const code of ['unauthorized','forbidden']){
  let puts=0;
  const service=new RemoteSourceService(api(async(_path,method)=>{if(method==='PUT'){puts++;return {scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source()}};}throw Object.assign(new Error('secret https://private-user:private-password@private.example/'),{code});}),scope,async()=>op);
  await assert.rejects(service.saveProfile(before,before.values),error=>(error as {code?:string}).code===code);
  assert.equal(puts,1);assert.equal(service.getSnapshot().ambiguous,false);assert.equal(service.getSnapshot().errorCode,code);assert.equal(service.getSnapshot().receipt?.operationId,op);
  assert(!JSON.stringify(service.getSnapshot()).includes('private-password'));service.dispose();
 }
 const foreign={sourceId:'dav',generation:'9',revision:2,saved:true,values:before.values};
 let puts=0;
 const mismatch=new RemoteSourceService(api(async(_path,method)=>{if(method==='PUT'){puts++;return {scope:wire,receipt:{operationId:op,accepted:true,removed:false,source:source()}};}return {scope:wire,settings:foreign};}),scope,async()=>op);
 await assert.rejects(mismatch.saveProfile(before,before.values),error=>(error as {code?:string}).code==='refresh_failed');
 assert.equal(puts,1);assert.equal(mismatch.getSnapshot().ambiguous,false);assert.equal(mismatch.getSnapshot().receipt?.operationId,op);mismatch.dispose();
});

test('an arbitrary backend code is normalized and never retained',async()=>{
 const hostile='https://private-user:private-password@private.example/media/';
 const service=new RemoteSourceService(api(async()=>{throw Object.assign(new Error('transport'),{code:hostile});}),scope,async()=>op);
 await assert.rejects(service.readProfile('dav'),error=>{const code=(error as {code?:string}).code;return code==='internal'&&!String(error).includes('private-password');});
 const snapshot=JSON.stringify(service.getSnapshot());assert(!snapshot.includes('private-password'));assert(!snapshot.includes('private.example'));service.dispose();
});

test('a dialog attempt guard retiring under a held operation id sends zero mutation',async()=>{
 const held=deferred();let writes=0,open=true;
 const service=new RemoteSourceService(api(async(_path,method)=>{if(method!=='GET')writes++;return list();}),scope,()=>held.promise,()=>true);
 const guard=()=>open;
 const configure=service.configure(null,input,undefined,guard);open=false;held.resolve(op);
 await configure;assert.equal(writes,0);service.dispose();
 open=true;const saveHeld=deferred();let saveWrites=0;
 const profile={sourceId:'dav',generation:'1',revision:1,saved:true,values:{scanDepth:'basic' as const,wholeFileReads:false,connections:4,requestsPerSecond:4,bytesPerSecond:0}};
 const saveService=new RemoteSourceService(api(async(_path,method)=>{if(method!=='GET')saveWrites++;return list();}),scope,()=>saveHeld.promise,()=>true);
 const saveGuard=()=>open;const save=saveService.saveProfile(profile,profile.values,saveGuard);open=false;saveHeld.resolve(op);
 await assert.rejects(save);assert.equal(saveWrites,0);saveService.dispose();
});
