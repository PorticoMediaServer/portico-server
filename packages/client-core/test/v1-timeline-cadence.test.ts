import test from 'node:test';
import assert from 'node:assert/strict';
import {V1PlaybackApi} from '../src/playback-v1/legacy-bridge.ts';

/** PERF-24, spec §6: the timeline is sent every Report-Every-Ms while playing steadily (10 s), and
 * at once on a state change, a new generation, a seek, or the first report. */
function setup() {
  let now = 0;
  const sent: {state: string; positionMs: number}[] = [];
  const timers: {at: number; fn: () => void; id: number}[] = [];
  let ids = 0;
  const http = {send: async (r: {path: string; body?: unknown}) => {
    if (r.path.endsWith('/timeline')) { const b = r.body as {state: string; positionMs: number}; sent.push({state: b.state, positionMs: b.positionMs}); }
    return {status: 204, headers: {'report-every-ms': '10000'}, body: undefined};
  }};
  const api = new V1PlaybackApi(http as never, {now: () => now, timers: {setTimer: (fn, ms) => { const id = ++ids; timers.push({at: now + ms, fn, id}); return id; }, clearTimer: id => { const i = timers.findIndex(t => t.id === id); if (i >= 0) timers.splice(i, 1); }}});
  const advance = async (ms: number) => {
    const end = now + ms;
    for (;;) {
      timers.sort((a, b) => a.at - b.at);
      const next = timers[0];
      if (!next || next.at > end) break;
      timers.shift(); now = next.at; next.fn();
      await new Promise(r => setImmediate(r));
    }
    now = end;
  };
  return {api, sent, advance, at: () => now};
}

test('a minute of steady playback reports every 10 s, not every 3 s', async () => {
  const {api, sent, advance, at} = setup();
  // The service checkpoints every 3 s of position while playing.
  for (let s = 0; s <= 60; s += 3) {
    await api.progressPlayback('ps_1', {generation: 1, sequence: s, positionSeconds: s, state: 'playing'});
    await advance(3000);
  }
  assert.ok(at() >= 63000);
  assert.ok(sent.length >= 6 && sent.length <= 8, `${sent.length} reports in 63 s`);
  assert.ok(sent.every(r => r.state === 'playing'));
  // The timer carries the latest position.
  assert.ok(sent.at(-1)!.positionMs >= 50000);
});

test('pause, a seek and a new generation are sent at once', async () => {
  const {api, sent, advance} = setup();
  await api.progressPlayback('ps_1', {generation: 1, sequence: 1, positionSeconds: 0, state: 'playing'});
  await advance(3000);
  await api.progressPlayback('ps_1', {generation: 1, sequence: 2, positionSeconds: 3, state: 'playing'});
  assert.equal(sent.length, 1, 'steady: carried by the timer');
  await api.progressPlayback('ps_1', {generation: 1, sequence: 3, positionSeconds: 120, state: 'playing'});
  assert.equal(sent.length, 2, 'a seek is reported at once');
  await api.progressPlayback('ps_1', {generation: 1, sequence: 4, positionSeconds: 121, state: 'paused'});
  assert.equal(sent.at(-1)!.state, 'paused', 'a pause is reported at once');
  await api.progressPlayback('ps_1', {generation: 2, sequence: 5, positionSeconds: 121, state: 'playing'});
  assert.equal(sent.length, 4, 'a new generation is reported at once');
  await api.progressPlayback('ps_1', {generation: 2, sequence: 6, positionSeconds: 124, state: 'ended'});
  assert.equal(sent.at(-1)!.state, 'ended');
});

function stalledSetup() {
 let now=0, peak=0, live=0;
 const requests:{body:any;signal?:AbortSignal;resolve:(r:any)=>void;reject:(e:unknown)=>void}[]=[];
 const timers=new Map<number,{fn:()=>void;ms:number}>();let serial=0;
 const api=new V1PlaybackApi({send:r=>new Promise((resolve,reject)=>{
  live++;peak=Math.max(peak,live);
  const done=(fn:(v:any)=>void)=>(v:any)=>{live--;fn(v);};
  const request={body:r.body,signal:r.signal,resolve:done(resolve),reject:done(reject)};
  requests.push(request);r.signal?.addEventListener('abort',()=>request.reject(new Error('aborted')),{once:true});
 })}, {now:()=>now,random:()=>0,timers:{setTimer:(fn,ms)=>{const id=++serial;timers.set(id,{fn:()=>{timers.delete(id);fn();},ms});return id;},clearTimer:id=>{timers.delete(id as number);}}});
 const tick=()=>new Promise(setImmediate);
 const progress=(position:number,state:'playing'|'paused'|'ended'='playing',generation=1)=>api.progressPlayback('s',{generation,sequence:0,positionSeconds:position,state});
 return {api,requests,timers,progress,tick,peak:()=>peak,clock:(n:number)=>{now=n;}};
}

test('a slow timeline holds one request while steady ticks coalesce; urgent facts retain order', async()=>{
 const s=stalledSetup();const first=s.progress(0);
 for(let n=1;n<=20;n++){s.clock(n*250);await s.progress(n/4);}
 assert.equal(s.requests.length,1,'twenty ticks do not add twenty requests');
 const pause=s.progress(5,'paused');const seek=s.progress(120,'paused');const resume=s.progress(120,'playing',2);
 const marker=s.api.markerSkipped('s',{markerId:'m',mode:'manual',positionMs:120000});const end=s.progress(121,'ended',2);
 for(let n=0;n<6;n++){assert.equal(s.requests.length,n+1);s.requests[n].resolve({status:204,headers:{'Report-Every-Ms':'10000'}});await s.tick();}
 await Promise.all([first,pause,seek,resume,marker,end]);
 assert.equal(s.peak(),1);
 assert.deepEqual(s.requests.map(r=>[r.body.state,r.body.positionMs,r.body.generation]),[['playing',0,1],['paused',5000,1],['paused',120000,1],['playing',120000,2],['playing',120000,2],['ended',121000,2]]);
 assert.equal(s.requests[4].body.skipped.markerId,'m');assert.deepEqual(s.requests.map(r=>r.body.seq),[1,2,3,4,5,6]);
 assert.equal(s.timers.size,0,'ended has no keepalive');s.api.forget('s');
});

test('forget aborts the outstanding report and discards queued facts without another request', async()=>{
 const s=stalledSetup();const first=s.progress(0).catch(()=>{});const pause=s.progress(0,'paused');
 s.api.forget('s');await s.tick();await Promise.all([first,pause]);
 assert.equal(s.requests[0].signal?.aborted,true);assert.equal(s.requests.length,1);assert.equal(s.timers.size,0);
});

test('timeline deadline aborts a stalled request, and forgetting stops retry work', async()=>{
 const s=stalledSetup();const first=s.progress(0).catch(()=>{});
 const deadline=[...s.timers.values()].find(t=>t.ms===20000)!;assert.ok(deadline);deadline.fn();await s.tick();
 assert.equal(s.requests[0].signal?.aborted,true);assert.equal(s.requests.length,1);
 s.api.forget('s');await first;assert.equal(s.timers.size,0);
});

test('retry hints retain end and marker facts, ordered ahead of newer reports', async()=>{
 const s=stalledSetup();const first=s.progress(0);
 const marker=s.api.markerSkipped('s',{markerId:'m',mode:'manual',positionMs:0});
 const end=s.progress(60,'ended');const endAgain=s.progress(60,'ended');
 s.requests[0].resolve({status:503,headers:{'Retry-After':'60'},body:{error:{code:'server_busy',retry:'same_request'}}});
 await s.tick();assert.equal(s.requests.length,1);
 const backoff=[...s.timers.values()].find(t=>t.ms===60000)!;assert.ok(backoff,'server-directed minute is not shortened');
 s.clock(60000);backoff.fn();await s.tick();assert.equal(s.requests.length,2);assert.equal(s.requests[1].body.state,'playing');assert.equal(s.requests[1].body.seq,s.requests[0].body.seq,'a lost acknowledgement retries the same fact sequence');
 for(let n=1;n<=3;n++){s.requests[n].resolve({status:204,headers:{}});await s.tick();}
 await Promise.all([first,marker,end,endAgain]);
 assert.equal(s.requests.length,4,'retry, marker and one end; duplicate end joins the acknowledgement');
 assert.equal(s.requests[2].body.skipped.markerId,'m');assert.equal(s.requests[3].body.state,'ended');
 assert.equal(s.peak(),1);assert.equal(s.timers.size,0);s.api.forget('s');
});

test('a pathological burst bounds unacknowledged facts and returns a recoverable refusal', async()=>{
 const s=stalledSetup();const accepted=[s.progress(0)];
 for(let n=0;n<63;n++)accepted.push(s.api.markerSkipped('s',{markerId:'m'+n,mode:'manual',positionMs:n}));
 await assert.rejects(s.api.markerSkipped('s',{markerId:'overflow',mode:'manual',positionMs:64}),e=>(e as {code?:string;retry?:string}).code==='timeline_pending_full'&&(e as {retry?:string}).retry==='same_request');
 assert.equal(s.requests.length,1);
 for(let n=0;n<64;n++){s.requests[n].resolve({status:204,headers:{}});await s.tick();}
 await Promise.all(accepted);assert.equal(s.requests.length,64);assert.equal(s.peak(),1);
 assert.equal(s.requests.at(-1)!.body.skipped.markerId,'m62');s.api.forget('s');
});


test('normal stop drains accepted marker facts before deleting; duplicate cleanup joins', async()=>{
 const s=stalledSetup();const first=s.progress(0);
 const marker=s.api.markerSkipped('s',{markerId:'before-stop',mode:'manual',positionMs:0});
 const stop=s.api.stopPlayback('s'),stopAgain=s.api.stopPlayback('s');
 assert.equal(stop,stopAgain);assert.equal(s.requests.length,1);
 s.requests[0].resolve({status:204,headers:{}});await s.tick();
 assert.equal(s.requests.length,2);assert.equal(s.requests[1].body.skipped.markerId,'before-stop');
 s.requests[1].resolve({status:204,headers:{}});await s.tick();
 assert.equal(s.requests.length,3);assert.equal(s.requests[2].body.positionMs,0);
 s.requests[2].resolve({status:204,headers:{}});await Promise.all([first,marker,stop,stopAgain]);
 assert.equal(s.timers.size,0);assert.equal(s.peak(),1);
});


test('adopting a new presentation of the same session keeps accepted marker facts and seq',async()=>{
 const s=stalledSetup();const first=s.progress(0);
 const marker=s.api.markerSkipped('s',{markerId:'before-change',mode:'manual',positionMs:0});
 s.api.adopt({id:'s',state:'playing',presentation:{generation:2,startPositionMs:60000,mode:'direct',url:'/media'},lease:{reportEveryMs:10000}} as never,120);
 assert.equal(s.requests[0].signal?.aborted,false);
 const changed=s.progress(60,'playing',2);
 for(let n=0;n<3;n++){s.requests[n].resolve({status:204,headers:{}});await s.tick();}
 await Promise.all([first,marker,changed]);
 assert.equal(s.requests[1].body.skipped.markerId,'before-change');assert.equal(s.requests[2].body.generation,2);
 assert.deepEqual(s.requests.map(r=>r.body.seq),[1,2,3]);s.api.forget('s');
});
