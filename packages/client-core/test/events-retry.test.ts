import {test} from 'node:test';
import assert from 'node:assert/strict';
import {EventsClient, type RawStreamEvent} from '../src/playback-v1/events.ts';
import type {V1Response} from '../src/playback-v1/http.ts';

const settle=async()=>{for(let i=0;i<12;i++)await Promise.resolve();};
function timers(){
 const pending=new Map<number,{fn:()=>void;ms:number}>();let id=0;
 return {pending,setTimer:(fn:()=>void,ms:number)=>{pending.set(++id,{fn,ms});return id;},clearTimer:(timer:unknown)=>{pending.delete(timer as number);},fire:()=>{const [id,t]=[...pending][0];pending.delete(id);t.fn();}};
}
const held=()=>new Promise<V1Response>(()=>{});

test('event poll honours a 60-second Retry-After minimum and cancelling its sleep stops requests',async()=>{
 const c=timers();let calls=0;
 const events=new EventsClient({http:{send:async()=>{calls++;return {status:429,headers:{'Retry-After':'60'}};}},random:()=>0,setTimer:c.setTimer,clearTimer:c.clearTimer});
 events.start();await settle();
 assert.equal(calls,1);assert.equal([...c.pending.values()][0].ms,60_000);
 events.stop();await settle();assert.equal(c.pending.size,0);assert.equal(calls,1);
});

test('event retries jitter their backoff deterministically without shortening a server minimum',async()=>{
 for(const [random,delay] of [[0,750],[1,1250]]){
  const c=timers();const events=new EventsClient({http:{send:async()=>{throw new Error('offline');}},random:()=>random,setTimer:c.setTimer,clearTimer:c.clearTimer});
  events.start();await settle();assert.equal([...c.pending.values()][0].ms,delay);events.stop();
 }
});

test('quiet clean SSE closures back off before falling back to long poll, retaining the long-poll wait',async()=>{
 const c=timers();let streams=0,polls=0;
 const events=new EventsClient({http:{send:r=>{polls++;assert.match(r.path,/waitSeconds=55/);return held();}},openStream:async()=>{streams++;},waitSeconds:55,random:()=>0.5,setTimer:c.setTimer,clearTimer:c.clearTimer});
 events.start();await settle();assert.equal(streams,1);assert.equal(polls,0);
 assert.equal([...c.pending.values()][0].ms,1000);
 c.fire();await settle();assert.equal(streams,2);assert.equal([...c.pending.values()][0].ms,2000);
 c.fire();await settle();assert.equal(streams,3);assert.equal(polls,1);assert.equal(events.getStatus().mode,'poll');events.stop();
});

test('stream errors retain Retry-After even when the next request switches to long poll',async()=>{
 const c=timers();let polls=0;
 const events=new EventsClient({http:{send:()=>{polls++;return held();}},openStream:async()=>{throw {status:503,retryAfterMs:60_000};},streamFailuresBeforePoll:1,random:()=>0.5,setTimer:c.setTimer,clearTimer:c.clearTimer});
 events.start();await settle();assert.equal(polls,0);assert.equal([...c.pending.values()][0].ms,60_000);
 c.fire();await settle();assert.equal(polls,1);events.stop();
});

test('a late poll response from a stopped run cannot publish events or cursors into a restarted run',async()=>{
 let resolve!:(r:V1Response)=>void;let calls=0;
 const events=new EventsClient({http:{send:()=>++calls===1?new Promise(r=>{resolve=r;}):held()}});
 let delivered=0;events.on('session.updated',()=>delivered++);
 events.start();await settle();events.stop();events.start();await settle();
 resolve({status:200,headers:{},body:{events:[{id:'old',type:'session.updated'}],nextAfter:'old'}});await settle();
 assert.equal(delivered,0);assert.equal(events.getStatus().cursor,undefined);assert.equal(events.getStatus().connected,false);assert.equal(events.getStatus().mode,'poll');events.stop();
});

test('a retired SSE callback and cleanup cannot overwrite the restarted connection status',async()=>{
 const runs:{event:(raw:RawStreamEvent)=>void;resolve:()=>void}[]=[];
 const events=new EventsClient({http:{send:held},openStream:(_cursor,event)=>new Promise(resolve=>{runs.push({event,resolve});})});
 const emit=(run:number,id:string)=>runs[run].event({data:JSON.stringify({id,type:'session.updated'})});
 const seen:string[]=[];events.on('session.updated',e=>seen.push(e.id));
 events.start();events.stop();events.start();emit(1,'new');emit(0,'old');runs[0].resolve();await settle();
 assert.deepEqual(seen,['new']);assert.equal(events.getStatus().cursor,'new');assert.equal(events.getStatus().connected,true);events.stop();runs[1].resolve();
});

test('a zero-delay stream.closed envelope cannot cause an immediate reconnect loop',async()=>{
 const c=timers();let streams=0;
 const events=new EventsClient({http:{send:held},openStream:async(_cursor,event)=>{streams++;event({data:JSON.stringify({id:'close',type:'stream.closed',data:{reconnectAfterMs:0}})});},setTimer:c.setTimer,clearTimer:c.clearTimer});
 events.start();await settle();assert.equal(streams,1);assert.equal([...c.pending.values()][0].ms,1000);events.stop();
});

test('event retry sleep chunks long Retry-After values without overflowing a timer or retrying early',async()=>{
 const c=timers();let calls=0;
 const events=new EventsClient({http:{send:async()=>{calls++;return {status:429,headers:{'Retry-After':'3000000'}};}},random:()=>0,setTimer:c.setTimer,clearTimer:c.clearTimer});
 events.start();await settle();assert.equal([...c.pending.values()][0].ms,2_147_483_647);
 c.fire();await settle();assert.equal(calls,1);assert.equal([...c.pending.values()][0].ms,3_000_000_000-2_147_483_647);events.stop();await settle();assert.equal(c.pending.size,0);
});
