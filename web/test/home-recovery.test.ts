import test from 'node:test';
import assert from 'node:assert/strict';
import {homeRecovery} from '../src/app/home-recovery.ts';

function deferred<T>() { let resolve!:(value:T)=>void,reject!:(error:unknown)=>void;const promise=new Promise<T>((yes,no)=>{resolve=yes;reject=no;});return {resolve,reject,promise}; }
const settle=async()=>{for(let i=0;i<12;i++)await Promise.resolve();};
function environment(random=0.5){
 let now=0,hidden=false,id=0;const timers=new Map<number,{at:number;fn:()=>void}>(),listeners=new Set<()=>void>();
 return {timers,hidden(value:boolean){hidden=value;for(const fn of [...listeners])fn();},async advance(ms:number){const end=now+ms;for(;;){const next=[...timers].sort((a,b)=>a[1].at-b[1].at)[0];if(!next||next[1].at>end)break;now=next[1].at;timers.delete(next[0]);next[1].fn();await settle();}now=end;await settle();},env:{setTimer:(fn:()=>void,ms:number)=>{timers.set(++id,{at:now+ms,fn});return id;},clearTimer:(timer:unknown)=>{timers.delete(timer as number);},hidden:()=>hidden,onVisibility:(fn:()=>void)=>{listeners.add(fn);return()=>{listeners.delete(fn);};},random:()=>random,now:()=>now}};
}
function harness(random=0.5){
 const clock=environment(random),requests:{signal:AbortSignal;result:ReturnType<typeof deferred<string>>}[]=[],values:string[]=[],errors:unknown[]=[];let loading=0;
 const recovery=homeRecovery(signal=>{const result=deferred<string>();requests.push({signal,result});return result.promise;},{loading:()=>{loading++;},success:value=>values.push(value),error:error=>errors.push(error)},clock.env);
 return {clock,requests,values,errors,recovery,loading:()=>loading};
}

test('Home overload retries automatically with positive jitter, shares manual refreshes and respects a 60-second server floor',async()=>{
 const h=harness();const first=h.recovery.refresh();assert.equal(first,h.recovery.refresh());assert.equal(h.requests.length,1);
 h.requests[0].result.reject({status:503,retryAfterSeconds:60});await first;
 assert.equal(h.errors.length,1);await h.recovery.refresh();assert.equal(h.requests.length,1,'manual retry cannot bypass cooldown');
 await h.clock.advance(60_000);assert.equal(h.requests.length,1,'positive jitter follows the server floor');
 await h.clock.advance(15_000);assert.equal(h.requests.length,2);
 h.requests[1].result.resolve('recovered');await settle();assert.deepEqual(h.values,['recovered']);assert.equal(h.clock.timers.size,0);h.recovery.stop();
});

test('hidden Home aborts its current read and resumes once; stale responses cannot overwrite recovery',async()=>{
 const h=harness();void h.recovery.refresh();h.clock.hidden(true);await settle();
 assert.equal(h.requests[0].signal.aborted,true);assert.equal(h.clock.timers.size,0);
 await h.clock.advance(120_000);assert.equal(h.requests.length,1);
 h.clock.hidden(false);await h.clock.advance(1249);assert.equal(h.requests.length,1);
 await h.clock.advance(1);assert.equal(h.requests.length,2);
 h.requests[1].result.resolve('new');await settle();h.requests[0].result.resolve('stale');await settle();assert.deepEqual(h.values,['new']);h.recovery.stop();
});

test('a hidden cooldown is preserved on resume and repeated failures increase backoff',async()=>{
 const h=harness(0);void h.recovery.refresh();h.requests[0].result.reject({status:429,retryAfterSeconds:60});await settle();
 h.clock.hidden(true);await h.clock.advance(10_000);h.clock.hidden(false);await h.clock.advance(49_999);assert.equal(h.requests.length,1);
 await h.clock.advance(1);assert.equal(h.requests.length,2);
 h.requests[1].result.reject(new TypeError('offline'));await settle();await h.clock.advance(1999);assert.equal(h.requests.length,2);await h.clock.advance(1);assert.equal(h.requests.length,3);h.recovery.stop();await settle();
});

test('Home alone has a 15-second deadline even when its transport never settles; a later successful read resets backoff',async()=>{
 const h=harness(0);void h.recovery.refresh();await h.clock.advance(15_000);
 assert.equal(h.requests[0].signal.aborted,true);assert.equal(h.errors.length,1);
 await h.clock.advance(1000);assert.equal(h.requests.length,2);h.requests[1].result.resolve('cached');await settle();
 void h.recovery.refresh();h.requests[2].result.reject({status:503});await settle();
 await h.clock.advance(1000);assert.equal(h.requests.length,4);assert.deepEqual(h.values,['cached'],'a failed refresh does not clear cached rows');h.recovery.stop();await settle();
});

test('identity/reset invalidation and stop fence old reads; permanent API errors do not retry',async()=>{
 const h=harness(0);void h.recovery.refresh();void h.recovery.refresh(true);assert.equal(h.requests[0].signal.aborted,true);
 h.requests[0].result.resolve('old');h.requests[1].result.reject({status:403});await settle();assert.deepEqual(h.values,[]);assert.equal(h.clock.timers.size,0);
 void h.recovery.refresh();h.recovery.stop();h.requests[2].result.resolve('after stop');await settle();h.clock.hidden(true);h.clock.hidden(false);await h.clock.advance(60_000);assert.deepEqual(h.values,[]);assert.equal(h.requests.length,3);
});

test('unusually long server cooldowns use bounded timer chunks rather than overflowing browser timers',async()=>{
 const h=harness(0);void h.recovery.refresh();h.requests[0].result.reject({status:503,retryAfterSeconds:3_000_000});await settle();
 assert.equal([...h.clock.timers.values()][0].at,2_147_483_647);
 await h.clock.advance(2_147_483_647);assert.equal(h.requests.length,1);
 await h.clock.advance(3_000_000_000-2_147_483_647);assert.equal(h.requests.length,2);h.recovery.stop();await settle();
});
