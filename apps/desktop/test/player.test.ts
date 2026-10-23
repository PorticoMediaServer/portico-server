import {test} from 'node:test';
import type {PlaybackSnapshot} from '../../../packages/client-core/src/index.ts';
import assert from 'node:assert/strict';
import {DesktopMediaPlayer} from '../src/player.ts';
import {observation, type Bridge, type NativeCommand, type NativeObservation} from '../src/bridge.ts';
import type {VideoLike} from '../../../web-next/src/platform/playback/index.ts';

const source = {url:'/v1/media/opaque',generation:1,mode:'direct' as const,startPositionMs:0,autoplay:true,subtitles:[]};
function harness() {
  const commands: NativeCommand[] = [], handlers = new Map<string,()=>void>();
  let reading: NativeObservation = {generation:0,state:'paused',positionMs:0,rate:1,volume:1,muted:false,droppedFrames:0,nativeSurfaceActive:false,eqApplied:false,levelingApplied:false};
  let answer: (()=>Promise<NativeObservation>) | undefined;
  let commandEffect:((command:NativeCommand)=>Promise<void>)|undefined;
  const bridge: Bridge = {async command(command) {commands.push(command); if (command.source) reading={...reading,generation:command.source.generation};await commandEffect?.(command);},observe:()=>answer?.() ?? Promise.resolve(reading)};
  const video = {volume:1,muted:false,pause(){},load(){},removeAttribute(){},addEventListener(k:string,fn:()=>void){handlers.set(k,fn);},removeEventListener(k:string){handlers.delete(k);}} as unknown as VideoLike;
  const player = new DesktopMediaPlayer(video,{resolveUrl:path=>'https://server.test'+path},bridge,false);
  return {player,commands,handlers,setCommand:(effect:NonNullable<typeof commandEffect>)=>{commandEffect=effect;},setReading:(x:Partial<NativeObservation>)=>{reading={...reading,...x};},setAnswer:(x:()=>Promise<NativeObservation>)=>{answer=x;}};
}
function processingSnapshot(id:string,gainDb:number):Pick<PlaybackSnapshot,'phase'|'session'|'audioEffects'>{
 return {phase:'ready',session:{id,generation:1,streamUrl:source.url,mode:'direct',duration:2,resumeSeconds:0,audio:{version:2,id:'audio',mode:'direct',url:source.url,gain:{trackDb:gainDb,trackPeak:1,source:'ebur128'}}},
  audioEffects:{settings:{gapless:true,crossfadeSeconds:0,normalization:'track'},rendering:false,rate:1,eqApplied:false,levelingApplied:false}};
}
test('real port forwards only projection and translates private native generations',async()=>{
  const h=harness();h.player.load({...source,sessionId:'session-private',itemId:'private-item'});await h.player.tick();
  assert.equal(h.commands[0].source?.url,'https://server.test/v1/media/opaque');
  assert.equal(JSON.stringify(h.commands).includes('session-private'),false);
  assert.equal(h.player.observe().generation,1);
  h.player.play();h.player.setRate(1.5);await h.player.tick();
  assert.equal(h.commands.at(-1)?.rate,1.5);h.player.dispose();assert.equal(h.handlers.size,0);
});
test('late observation cannot report for new source with same protocol generation',async()=>{
  const h=harness();h.player.load(source);await h.player.tick();
  let release!:(x:NativeObservation)=>void;h.setAnswer(()=>new Promise(resolve=>{release=resolve;}));
  const pending=h.player.tick();await Promise.resolve();await Promise.resolve();
  const old=h.commands[0].source!.generation;
  h.player.load({...source,url:'/v1/media/new'});
  release({generation:old,state:'playing',positionMs:999,rate:1,volume:1,muted:false,droppedFrames:0,nativeSurfaceActive:false,eqApplied:false,levelingApplied:false});await pending;
  assert.equal(h.player.observe().positionMs,0);assert.equal(h.player.observe().state,'loading');h.player.dispose();
});
test('native failures report fixed code without exception/session/path strings',async()=>{
  const h=harness();let detail='';h.player.onEvent(e=>{if(e.type==='error')detail=JSON.stringify(e);});
  h.player.load(source);h.setAnswer(async()=>{throw new Error('https://server/v1/media?token=SECRET /home/private');});await h.player.tick();
  assert.match(detail,/engine_unavailable/);assert.doesNotMatch(detail,/SECRET|private|https:/);h.player.dispose();
});
test('native observation validator rejects malformed bytes and projects extra fields away',()=>{
  assert.throws(()=>observation({generation:1,state:'playing',positionMs:NaN}));
  const value=observation({generation:1,state:'playing',positionMs:1,rate:1,volume:1,muted:false,droppedFrames:0,nativeSurfaceActive:false,eqApplied:false,levelingApplied:false,url:'SECRET',path:'/private'});
  assert.doesNotMatch(JSON.stringify(value),/SECRET|private|url|path/);
});

test('native paint flag requires an actual current-generation confirmation and clears on end/unload',async()=>{
 const h=harness();assert.equal(h.player.nativeSurfaceActive,false);h.player.load(source);await h.player.tick();
 assert.equal(h.player.nativeSurfaceActive,false);h.setReading({nativeSurfaceActive:true,state:'playing'});await h.player.tick();
 assert.equal(h.player.nativeSurfaceActive,true);h.setReading({state:'ended'});await h.player.tick();assert.equal(h.player.nativeSurfaceActive,false);h.player.dispose();
});

test('processing uses current native facts and clears across tracks and ended',async()=>{
 const h=harness();assert.equal(h.player.audioProcessing,undefined);h.player.load(source);
 assert.equal(h.player.audioProcessing,undefined);await h.player.tick();
 assert.deepEqual(h.player.audioProcessing,{generation:1,eqApplied:false,levelingApplied:false});
 h.setReading({state:'playing',eqApplied:true});await h.player.tick();
 assert.deepEqual(h.player.audioProcessing,{generation:1,eqApplied:true,levelingApplied:false});
 assert.equal('eqApplied' in h.player.observe(),false);
 h.player.load({...source,url:'/v1/media/next',generation:2});assert.equal(h.player.audioProcessing,undefined);
 h.setReading({state:'playing',eqApplied:false});await h.player.tick();
 assert.deepEqual(h.player.audioProcessing,{generation:2,eqApplied:false,levelingApplied:false});
 h.setReading({state:'ended',eqApplied:true});await h.player.tick();assert.equal(h.player.audioProcessing,undefined);
 h.player.dispose();assert.equal(h.player.audioProcessing,undefined);
});

test('normalization input waits for matching source load and never promotes intent into applied facts',async()=>{
 const h=harness(),first=processingSnapshot('session-private-one',-6);
 h.player.applyAudioEffects(first as PlaybackSnapshot);assert.equal(h.commands.length,0);
 h.player.load(source);await h.player.tick();
 const native=h.commands[0].source!.generation;
 assert.deepEqual(h.commands[1],{action:'processing',gain:10**(-6/20),generation:native});
 assert.equal(h.player.audioProcessing?.levelingApplied,false);
 h.setReading({levelingApplied:true});await h.player.tick();assert.equal(h.player.audioProcessing?.levelingApplied,true);
 const next=processingSnapshot('session-private-two',-12),count=h.commands.length;
 h.player.applyAudioEffects(next as PlaybackSnapshot);await h.player.tick();assert.equal(h.commands.length,count);
 h.player.load(source);await h.player.tick();
 assert.deepEqual(h.commands.at(-1),{action:'processing',gain:10**(-12/20),generation:h.commands.at(-2)!.source!.generation});
 assert.notEqual(h.commands.at(-1)!.generation,native);assert.doesNotMatch(JSON.stringify(h.commands),/session-private|intentId|sessionId/);
 h.player.dispose();
});

test('server-baked normalization/conversion gain stays unity; neverBoost uses canonical gain policy',async()=>{
 const h=harness(),value=processingSnapshot('session-private',6);
 value.audioEffects={...value.audioEffects,settings:{...value.audioEffects.settings,neverBoost:true}};
 h.player.applyAudioEffects(value as PlaybackSnapshot);h.player.load(source);await h.player.tick();
 assert.equal(h.commands.at(-1)?.gain,1);
 value.audioEffects={...value.audioEffects,settings:{...value.audioEffects.settings,neverBoost:false}};
 value.session={...value.session!,audio:{...value.session!.audio!,gain:{trackDb:6,trackPeak:.5,source:'ebur128'}}};
 h.player.applyAudioEffects(value as PlaybackSnapshot);await h.player.tick();assert.equal(h.commands.at(-1)?.gain,10**(6/20));
 value.session={...value.session,audio:{...value.session.audio!,gain:undefined,loudnessAppliedDb:-12,conversionGainDb:-6}};
 h.player.applyAudioEffects(value as PlaybackSnapshot);await h.player.tick();assert.equal(h.commands.at(-1)?.gain,1);
 h.player.dispose();
});

test('frame step uses paused real generation and observation; no seek arithmetic or stale completion',async()=>{
 const h=harness();h.player.load({...source,generation:77});await h.player.tick();
 const native=h.commands[0].source!.generation;
 h.setCommand(async command=>{if(command.action==='frameStep')h.setReading({positionMs:317,state:'paused'});});
 await h.player.stepFrame(1,77);
 assert.deepEqual(h.commands.at(-1),{action:'frameStep',direction:1,generation:native});
 assert.equal(h.player.observe().positionMs,317);assert.equal(h.commands.some(v=>v.action==='seek'),false);
 h.setReading({state:'playing'});await h.player.tick();const count=h.commands.length;
 await assert.rejects(h.player.stepFrame(-1,77),/invalid_command/);assert.equal(h.commands.length,count);
 h.setReading({state:'paused'});await h.player.tick();
 let release!:()=>void;h.setCommand(async command=>{if(command.action==='frameStep')await new Promise<void>(resolve=>{release=resolve});});
 const pending=h.player.stepFrame(-1,77);await new Promise(resolve=>setImmediate(resolve));
 h.player.load({...source,generation:78});release();await assert.rejects(pending,/stale_generation/);
 assert.equal(h.player.observe().generation,78);assert.equal(h.player.observe().positionMs,0);h.player.dispose();
});
