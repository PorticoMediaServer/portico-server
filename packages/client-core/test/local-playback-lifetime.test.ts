import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService,type PlaybackSession,type PlaybackOptions} from '../src/index.ts';
import {serviceText} from '../src/presentation/service-text.ts';

function deferred<T>(){let resolve!:(value:T)=>void;const promise=new Promise<T>(r=>{resolve=r;});return {promise,resolve};}
const tick=()=>new Promise<void>(resolve=>setImmediate(resolve));
function local(id='local:entry:1'):PlaybackSession{return {id,generation:1,streamUrl:'app-private-source',mode:'direct',duration:120,resumeSeconds:12};}
function setup(options:PlaybackOptions={}){
 const stopped:string[]=[];let remoteCreates=0;
 const service=new PlaybackService({createPlayback:async()=>{remoteCreates++;throw Error('unexpected remote create');},stopPlayback:async id=>{stopped.push(id);},progressPlayback:async()=>{}},()=> 'request',options);
 return {service,stopped,remoteCreates:()=>remoteCreates};
}

test('a canceled or retired local caller cannot replace accepted playback or call its maker',async()=>{
 const x=setup();try{
  await x.service.playLocal('original',()=>local());const before=x.service.getSnapshot();let made=0;
  const aborted=new AbortController();aborted.abort();
  for(const context of [{signal:aborted.signal},{isCurrent:()=>false},{isCurrent:()=>{throw Error('retired');}}]){
   await assert.rejects(x.service.playLocal('foreign',()=>{made++;return local('foreign');},0,context),{name:'AbortError'});
   assert.equal(x.service.getSnapshot(),before);
  }
  assert.equal(made,0);assert.equal(x.remoteCreates(),0);
 }finally{x.service.leave();}
});

test('aborting journal recovery settles the local command without canceling recovery or a newer source',async()=>{
 const journal=deferred<[]>();let loads=0,made=0;
 const x=setup({deferRecovery:true,journal:{load:()=>{loads++;return journal.promise;},save:async()=>{}}});
 const caller=new AbortController();
 try{
  const pending=x.service.playLocal('old',()=>{made++;return local();},undefined,{signal:caller.signal});
  const rejected=assert.rejects(pending,{name:'AbortError'});caller.abort();await rejected;
  assert.equal(made,0);assert.equal(x.service.getSnapshot().phase,'idle');assert.equal(x.service.getSnapshot().error,undefined);
  const newer=x.service.playLocal('new',()=>local('local:new:2'),33);journal.resolve([]);await newer;
  assert.equal(loads,1);assert.equal(x.service.getSnapshot().session?.id,'local:new:2');
  assert.equal(x.service.getSnapshot().pendingSeek?.positionSeconds,33);assert.equal(x.remoteCreates(),0);
 }finally{journal.resolve([]);x.service.leave();}
});

test('host retirement during recovery refuses the maker; an older command never stops newer Play',async()=>{
 const journal=deferred<[]>();let current=true,made=0;
 const x=setup({deferRecovery:true,journal:{load:()=>journal.promise,save:async()=>{}}});
 try{
  const old=x.service.playLocal('old',()=>{made++;return local();},undefined,{isCurrent:()=>current});
  const rejected=assert.rejects(old,{name:'AbortError'});current=false;
  const next=x.service.playLocal('new',()=>local('local:new:2'));journal.resolve([]);await Promise.all([rejected,next]);
  assert.equal(made,0);assert.equal(x.service.getSnapshot().session?.id,'local:new:2');assert.equal(x.service.getSnapshot().error,undefined);
 }finally{journal.resolve([]);x.service.leave();}
});

test('local factory failures use catalogue text and Retry retains real target, paused intent and caller authority',async()=>{
 const x=setup();let made=0,current=true;
 try{
  await x.service.playLocal('item',()=>{if(++made===1)throw Error('private /path credential');return local('local:item:2');},42,{isCurrent:()=>current});
  assert.equal(x.service.getSnapshot().error,serviceText('playback.message.internal_error'));
  assert.equal(x.service.getSnapshot().session,undefined);
  await x.service.retry();assert.equal(made,2);assert.equal(x.service.getSnapshot().pendingSeek?.positionSeconds,42);
  x.service.pause();x.service.fail(x.service.getSnapshot().intentId,'controlled engine failure');
  current=false;const before=x.service.getSnapshot();await assert.rejects(x.service.retry(),{name:'AbortError'});
  assert.equal(x.service.getSnapshot(),before);assert.equal(made,2);
  current=true;await x.service.retry();assert.equal(x.service.getSnapshot().intent,'paused');assert.equal(x.service.getSnapshot().pendingSeek?.positionSeconds,42);
  assert.equal(x.remoteCreates(),0);
 }finally{x.service.leave();}
});

test('a maker that aborts then throws cannot publish a stale local error',async()=>{
 const x=setup(),caller=new AbortController();
 try{
  await assert.rejects(x.service.playLocal('old',()=>{caller.abort();throw Error('private native exception');},0,{signal:caller.signal}),{name:'AbortError'});
  assert.equal(x.service.getSnapshot().phase,'idle');assert.equal(x.service.getSnapshot().error,undefined);assert.equal(x.service.getSnapshot().session,undefined);
 }finally{x.service.leave();}
});

test('a reentrant local maker cannot adopt or fail over a newer accepted source',async()=>{
 const x=setup();let next:Promise<void>|undefined;
 try{
  await assert.rejects(x.service.playLocal('old',()=>{next=x.service.playLocal('new',()=>local('local:new:2'));throw Error('old maker failed');},undefined,{isCurrent:()=>true}),{name:'AbortError'});
  await next;assert.equal(x.service.getSnapshot().session?.id,'local:new:2');assert.equal(x.service.getSnapshot().error,undefined);
 }finally{x.service.leave();}
});

test('caller cancellation after accepted local admission does not stop that genuine source',async()=>{
 const x=setup(),caller=new AbortController();
 try{
  await x.service.playLocal('item',()=>local(),undefined,{signal:caller.signal,isCurrent:()=>true});
  const before=x.service.getSnapshot();caller.abort();await tick();assert.equal(x.service.getSnapshot(),before);assert.deepEqual(x.stopped,[]);
  x.service.fail(before.intentId,'controlled engine error');await assert.rejects(x.service.retry(),{name:'AbortError'});
 }finally{x.service.leave();}
});

test('local startup timeout uses the canonical service message and retires only its real session',async()=>{
 const x=setup({startupTimeoutMs:5});
 try{
  await x.service.playLocal('item',()=>local());await new Promise(r=>setTimeout(r,15));
  assert.equal(x.service.getSnapshot().phase,'error');assert.equal(x.service.getSnapshot().error,serviceText('playback.message.timeout'));
  assert.equal(x.service.getSnapshot().session,undefined);await tick();assert.deepEqual(x.stopped,['local:entry:1']);
 }finally{x.service.leave();}
});


test('a genuine session returned after synchronous host retirement is cleaned without adoption',async()=>{
 const x=setup();let current=true;
 try{
  await assert.rejects(x.service.playLocal('old',()=>{current=false;return local();},undefined,{isCurrent:()=>current}),{name:'AbortError'});
  await tick();assert.equal(x.service.getSnapshot().phase,'idle');assert.equal(x.service.getSnapshot().error,undefined);
  assert.equal(x.service.getSnapshot().session,undefined);assert.deepEqual(x.stopped,['local:entry:1']);
 }finally{x.service.leave();}
});


test('three-argument local starts retain quiet supersession compatibility',async()=>{
 const journal=deferred<[]>();let oldMaker=0;
 const x=setup({deferRecovery:true,journal:{load:()=>journal.promise,save:async()=>{}}});
 try{
  const old=x.service.playLocal('old',()=>{oldMaker++;return local();});
  const next=x.service.playLocal('new',()=>local('local:new:2'));journal.resolve([]);await Promise.all([old,next]);
  assert.equal(oldMaker,0);assert.equal(x.service.getSnapshot().session?.id,'local:new:2');
 }finally{journal.resolve([]);x.service.leave();}
});
