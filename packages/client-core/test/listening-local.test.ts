import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService,type PlaybackSession} from '../src/index.ts';
import {ListeningService,type ListeningRequest} from '../src/listening.ts';
import {V1QueuePlayer} from '../src/playback-v1/v1-queue-player.ts';
import {V1PlaybackApi} from '../src/playback-v1/legacy-bridge.ts';
import {FakePlaybackServer,fakeHttp} from '../src/playback-v1/testing/fake-server.ts';
import type {V1Request} from '../src/playback-v1/http.ts';
const viewer={serverId:'server',authority:'local' as const,accountId:'account',profileId:'profile'};
const scope={...viewer,controllerId:'controller',controllerEpoch:'epoch',commandLaneId:'lane'};
const wrap=(data:unknown)=>({scope,data});
const preferences={revision:1,bookRate:1.5,passoutMinutes:30};
const book={itemId:'book',libraryId:'library',kind:'audiobook_file',bookId:'wholebook',lastBookItemId:'book',bookEndAvailable:true};
const tick=()=>new Promise<void>(resolve=>setImmediate(resolve));
function deferred<T>(){let resolve!:(value:T)=>void;const promise=new Promise<T>(r=>{resolve=r;});return {promise,resolve};}
function session(id:string,mode='direct'):PlaybackSession{return {id,generation:1,streamUrl:'app-private-fixture',mode,duration:120,resumeSeconds:12};}
function engine(){let serial=0;const stops:string[]=[];return {stops,playback:new PlaybackService({createPlayback:async()=>session('server-'+(++serial)),stopPlayback:async id=>{stops.push(id);},progressPlayback:async()=>{}},()=>String(++serial),{})};}
function ready(p:PlaybackService){const s=p.getSnapshot();p.ready(s.intentId);if(s.pendingSeek)p.seekApplied(s.intentId,s.pendingSeek.revision,s.pendingSeek.positionSeconds,s.intent);}

test('the real queue kind hook cannot turn local item or source identities into listening requests',async()=>{
 const server=new FakePlaybackServer(),inner=fakeHttp(server),log:V1Request[]=[];
 const http={send:async(request:V1Request)=>{log.push(request);return inner.send(request);}};
 const api=new V1PlaybackApi(http,{timers:{setTimer:()=>0,clearTimer:()=>{}},key:()=> 'local-kind-key'}),{playback}=engine();
 const queue=new V1QueuePlayer({http,api,playback,viewer,key:()=> 'local-queue-key'}),stop=queue.connect();
 try{
  await tick();const global=log.filter(r=>r.path==='/v1/listening/preferences').length;assert.equal(global,1);
  for(const itemId of ['movie','book','opaque:source/private']){
   await playback.playLocal(itemId,()=>session('local:'+itemId,'local'));ready(playback);await tick();
   queue.listening.retryContext();await tick();
   assert.equal(queue.listening.getSnapshot().context,null);assert.equal(queue.listening.getSnapshot().rate,1);
  }
  assert.deepEqual(log.filter(r=>r.path!=='/v1/listening/preferences'),[],'no per-item/local-source request is permitted');
  assert.equal(log.filter(r=>r.path==='/v1/listening/preferences').length,global,'local replacement does not create a preference connection');
 }finally{stop();playback.leave();api.dispose();}
});

test('same-item server to local replacement clears server-owned rate, timers and passout without changing preferences',async()=>{
 const {playback}=engine();let now=0;const paths:string[]=[];
 const request:ListeningRequest=async path=>{paths.push(path);return wrap(path==='/v1/listening/preferences'?preferences:book);};
 const listening=new ListeningService(scope,request,playback,()=>now,()=>true);listening.connect();
 try{
  await playback.play('book');ready(playback);await tick();await listening.setRate(2);await listening.setTimer(15);
  assert.equal(listening.getSnapshot().context?.itemId,'book');assert.equal(playback.getSnapshot().audioEffects.rate,2);assert.ok(listening.getSnapshot().timer);
  const before=paths.length;await playback.playLocal('book',()=>session('local:book:1','local'));ready(playback);await tick();
  assert.equal(paths.length,before);const state=listening.getSnapshot();
  assert.equal(state.context,null);assert.equal(state.rate,1);assert.equal(state.timer,null);assert.equal(state.stopped,false);assert.equal(state.rewind,undefined);
  assert.deepEqual(state.preferences,preferences);assert.equal(playback.getSnapshot().audioEffects.rate,1);
  now=31*60000;listening.tick();assert.equal(playback.getSnapshot().intent,'playing');assert.equal(listening.mayTransition(1),true);
  await assert.rejects(listening.setTimer('chapter'),/Start a music or audiobook/);assert.equal(paths.length,before);
  listening.interact();await playback.play('book');ready(playback);await tick();
  assert.equal(listening.getSnapshot().context?.itemId,'book');assert.equal(listening.getSnapshot().rate,1.5,'fresh server playback uses its real default, not the old held 2x');
 }finally{listening.dispose();playback.leave();}
});

test('late same-item context and chapter replies cannot restore a retired server policy onto local playback',async()=>{
 const {playback}=engine(),context=deferred<unknown>(),chapter=deferred<unknown>();let reads=0,chapterReads=0;
 const request:ListeningRequest=async path=>{
  if(path==='/v1/listening/preferences')return wrap(preferences);
  if(path.includes('/chapters')){chapterReads++;return chapter.promise;}
  if(++reads===1)return wrap(book);return context.promise;
 };
 const listening=new ListeningService(scope,request,playback,()=>0,()=>true);listening.connect();
 try{
  await playback.play('book');ready(playback);await tick();await listening.setRate(2);
  const timer=listening.setTimer('chapter');await tick();assert.equal(chapterReads,1);assert.equal(listening.getSnapshot().timerLoading,true);
  listening.retryContext();await tick();assert.equal(reads,2);
  await playback.playLocal('book',()=>session('local:book:2','local'));ready(playback);const before=reads;
  context.resolve(wrap(book));chapter.resolve({});await Promise.all([timer,tick()]);
  assert.equal(reads,before);const state=listening.getSnapshot();assert.equal(state.context,null);assert.equal(state.timer,null);assert.equal(state.timerLoading,false);
  assert.equal(state.rate,1);assert.equal(state.error,null);assert.equal(playback.getSnapshot().audioEffects.rate,1);
  assert.equal(playback.getSnapshot().session?.id,'local:book:2');
 }finally{context.resolve(wrap(book));chapter.resolve({});listening.dispose();playback.leave();}
});

test('excluding server listening does not reset a local engine rate the service never applied',async()=>{
 const {playback}=engine(),paths:string[]=[];playback.setPlaybackRate(1.25);
 await playback.playLocal('movie',()=>session('local:movie:1','local'));
 const listening=new ListeningService(scope,async path=>{paths.push(path);return wrap(preferences);},playback);
 try{listening.connect();await tick();assert.deepEqual(paths,['/v1/listening/preferences']);assert.equal(playback.getSnapshot().audioEffects.rate,1.25);assert.equal(listening.getSnapshot().rate,1);}
 finally{listening.dispose();playback.leave();}
});
