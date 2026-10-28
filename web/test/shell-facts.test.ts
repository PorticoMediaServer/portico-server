import test from 'node:test';
import assert from 'node:assert/strict';
import {GUIDE_PROTOCOL} from '@core/channel-guide.ts';
import {FreshnessHub} from '@core/freshness-hub.ts';
import {parseDirectSnapshot} from '@core/profile-management.ts';
import {createShellFactOwner, attachShellFactFeed, shellAccount, shellProofChanged} from '../src/shell/facts.ts';
import type {ShellFacts} from '../src/shell/bootstrap.ts';
import type {LocalSession} from '@core/index.ts';

function clock() {
  let now = 0, id = 0;
  const timers = new Map<number,{at:number;fn:()=>void}>();
  const flush = async () => {for (let i=0; i<40; i++) await Promise.resolve();};
  return {env:{now:()=>now, random:()=>0, setTimer:(fn:()=>void,ms:number)=>{timers.set(++id,{at:now+ms,fn});return id;}, clearTimer:(id:unknown)=>{timers.delete(id as number);}},
    async advance(ms:number) {const end=now+ms; for (;;) {let due:[number,{at:number;fn:()=>void}]|undefined;
      for (const entry of timers) if (entry[1].at<=end && (!due || entry[1].at<due[1].at)) due=entry;
      if (!due) break; timers.delete(due[0]); now=due[1].at; due[1].fn(); await flush();}
      now=end; await flush();}, pending:()=>timers.size};
}
const direct = () => ({authority:'local', serverId:'server', canManage:true,
  capabilities:{manageAccount:true,managePeople:true,manageServer:true,manageSignIn:true},
  account:{id:'account',username:'viewer',primaryProfileId:'profile',role:'owner',revision:1,disabled:false,allowedLibraries:[]},
  profiles:[{id:'profile',name:'Evening',primary:true,position:0,art:'mint',revision:1,pinRevision:1,pinRequired:false}]});
const system = () => ({id:'server',name:'Study',version:'1',signIn:'local' as const,move:null,setupRequired:false,hostedConfigured:false,hostedAttached:false});
function harness(readonlyAccess=false) {
  const time=clock(), hub=new FreshnessHub(time.env), session={sessionFamilyId:'family',accessToken:'private-test-token',viewer:{serverId:'server',authority:'local',accountId:'account',profileId:'profile',role:'owner'}} as LocalSession;
  let record:unknown=direct(), info=system(), libraries=[{id:'library',name:'Films',kind:'movies'}], guideError:unknown, retired=0, permission=0, manifests=0;
  const calls:string[]=[];
  const settings={effective:{},deviceClass:'web'} as ShellFacts['preferences'];
  const witnesses={system:system(),direct:parseDirectSnapshot(direct())};
  const initial:ShellFacts={serverName:'Study',preferences:settings,identity:{viewer:session.viewer,serverName:'Study',serverVersion:'1',libraries,
    ...shellAccount(witnesses.direct,'profile'),liveAvailable:true,unread:0,showNotificationDot:false}};
  const api={async request(path:string) {calls.push(path); if (path==='/v1/libraries') return {items:libraries}; if (path==='/v1/direct') return record;
    if (path==='/v1/system') return info; if (path.startsWith('/v1/guide/sources?')) {if (guideError) throw guideError;return {protocolVersion:GUIDE_PROTOCOL,serverId:'server',viewerFence:'guide',revision:'1',sources:[]};}
    throw new Error('Unexpected read '+path);}};
  const owner=createShellFactOwner({api:api as never, session:()=>session, hub,initial,witnesses,readonlyAccess,
    isCurrent:()=>retired===0, retire:()=>{retired++;},
    home:{authority:async()=>{manifests++;return 'answered';}} as never,
    preferences:{observePreferences:async()=>({kind:'ok',value:settings})} as never,
    auth:{reconcilePermissions:async()=>{permission++;return {kind:'confirmed-unchanged'};}} as never});
  return {time,hub,owner,calls,initial,witnesses,counts:()=>({retired,permission,manifests}),
    rename:()=>{info={...info,name:'Den'};libraries=[{id:'library',name:'Cinema',kind:'movies'}];const r=direct();r.account.username='updated';r.profiles[0].name='Night';r.account.revision=2;record=r;},
    direct:(value:unknown)=>{record=value;},guideFailure:(error:unknown)=>{guideError=error;},dispose:()=>{owner.dispose();hub.dispose();}};
}

test('five hint groups refresh current display without re-admitting authority; bursts coalesce', async () => {
  const h=harness();h.rename();
  for(let i=0;i<10;i++)for(const group of ['directory','server','members','liveSources','libraryChannels'] as const)h.owner.request(group);
  await h.time.advance(1500);
  assert.equal(h.owner.source.getSnapshot().serverName,'Den');
  assert.equal(h.owner.source.getSnapshot().identity.libraries[0].name,'Cinema');
  assert.equal(h.owner.source.getSnapshot().identity.profileName,'Night');
  assert.equal(h.owner.source.getSnapshot().identity.username,'updated');
  assert.equal(h.counts().retired,0);assert.equal(h.counts().permission,1);assert.equal(h.counts().manifests,1);
  assert.equal(h.calls.filter(path=>path==='/v1/libraries').length,1);assert.equal(h.calls.filter(path=>path==='/v1/direct').length,1);
  assert.equal(h.calls.filter(path=>path.startsWith('/v1/guide/sources?')).length,1);
  assert.equal(h.initial.serverName,'Study');h.dispose();assert.equal(h.time.pending(),0);
});
test('guide failure cannot discard independently confirmed directory or invent a viewer retirement', async () => {
  const h=harness();h.rename();h.guideFailure(Object.assign(new Error('offline'),{retryAfterSeconds:7200}));
  h.owner.request('directory');h.owner.request('server');await h.time.advance(1000);
  assert.equal(h.owner.source.getSnapshot().identity.libraries[0].name,'Cinema');
  assert.equal(h.owner.source.getSnapshot().serverName,'Den');assert.equal(h.owner.source.getSnapshot().identity.liveAvailable,true);
  const reads=h.calls.length;h.hub.setActive(false);h.hub.setActive(true);h.owner.request('server');await h.time.advance(60000);
  assert.equal(h.calls.length,reads);assert.equal(h.counts().retired,0);h.dispose();
});
test('retained hidden media runs independent authority checks while display remains dormant', async () => {
  const h=harness();h.hub.setActive(false);h.owner.setRetained(true);h.owner.all();await h.time.advance(1000);
  assert.equal(h.calls.filter(path=>path==='/v1/libraries').length,0);
  assert.equal(h.calls.filter(path=>path==='/v1/system').length,0);
  assert.equal(h.calls.filter(path=>path==='/v1/direct').length,1);
  assert.deepEqual(h.counts(),{permission:1,manifests:1,retired:0});
  h.hub.setActive(true);await h.time.advance(1500);assert.ok(h.calls.includes('/v1/libraries'));h.dispose();
});
test('strict direct eligibility proof retires before input settles; malformed proof never certifies absence', async () => {
  const h=harness();h.hub.touch();h.direct({...direct(),profiles:[{...direct().profiles[0],id:'replacement'}],account:{...direct().account,primaryProfileId:'replacement'}});
  h.owner.request('members');await h.time.advance(400);assert.equal(h.counts().retired,1);h.dispose();
  const broken=harness();broken.direct({...direct(),profiles:[{...direct().profiles[0],name:undefined}]});broken.owner.request('members');await broken.time.advance(1000);
  assert.equal(broken.counts().retired,0);assert.equal(broken.owner.source.getSnapshot().identity.profileName,'Evening');
  await broken.time.advance(2500);assert.ok(broken.calls.filter(path=>path==='/v1/direct').length>1);broken.dispose();
});
test('readonly admission does not restart a stopped mutating permission owner', async () => {
  const h=harness(true);h.owner.all();await h.time.advance(1500);assert.equal(h.counts().permission,0);assert.equal(h.counts().retired,0);h.dispose();
});
test('system name/version are display but actual scope/security changes replace authority', () => {
  const initial={system:system(),direct:parseDirectSnapshot(direct())};
  assert.equal(shellProofChanged(initial,{system:{...system(),name:'Den',version:'2'}},'profile'),false);
  for(const next of [{...system(),id:'other'},{...system(),setupRequired:true},{...system(),hostedAttached:true}])assert.equal(shellProofChanged(initial,{system:next},'profile'),true);
});
test('first connected relay queues one catch-up without cursor, survives disconnect, and cancels on dispose', async () => {
  const time=clock(),hub=new FreshnessHub(time.env);let connected=false,catchups=0,current=true;const statuses=new Set<()=>void>(),resyncs=new Set<()=>void>();
  const events={getStatus:()=>({connected}),subscribeStatus:(fn:()=>void)=>{statuses.add(fn);return()=>statuses.delete(fn);},onResync:(fn:()=>void)=>{resyncs.add(fn);return()=>resyncs.delete(fn);}};
  const off=attachShellFactFeed(events as never,hub,()=>current,()=>{catchups++;});
  connected=true;for(const fn of statuses)fn();assert.equal(catchups,0);connected=false;await time.advance(0);assert.equal(catchups,1);
  connected=true;for(const fn of statuses)fn();await time.advance(0);assert.equal(catchups,1);
  for(const fn of resyncs)fn();assert.equal(catchups,2);current=false;off();hub.dispose();assert.equal(time.pending(),0);assert.equal(statuses.size+resyncs.size,0);
});
