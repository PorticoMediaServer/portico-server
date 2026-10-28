import test from 'node:test';
import assert from 'node:assert/strict';
import {FreshnessHub} from '@core/freshness-hub.ts';
import {parseDirectSnapshot} from '@core/profile-management.ts';
import {attachShellFactFeed, shellProofChanged} from '../src/shell/facts.ts';
import {clock, direct, system, harness} from './shell-facts-fixture.ts';

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
