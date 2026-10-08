import test from 'node:test';import assert from 'node:assert/strict';import {PlaybackService,type PlaybackApi} from '../src/index.ts';
const tick=()=>new Promise(r=>setImmediate(r));
function setup(escalates:(n:number)=>boolean|null){
 let created=0;const reports:any[]=[];
 const api:PlaybackApi={createPlayback:async()=>({id:'s'+(++created),generation:1,streamUrl:'/media',mode:'direct',duration:120,resumeSeconds:12}),stopPlayback:async()=>{},progressPlayback:async()=>{},
  reportRouteFailure:async(sessionId,code,detail)=>{reports.push({sessionId,code,detail});const e=escalates(reports.length);return e===null?null:{route:'original',escalates:e};}};
 return {service:new PlaybackService(api,()=>String(created),{seekTimeoutMs:1000}),reports,created:()=>created};
}
async function playing(x:ReturnType<typeof setup>,at=40){await x.service.play('movie');const s=x.service.getSnapshot();x.service.ready(s.intentId);x.service.seekApplied(s.intentId,s.pendingSeek!.revision,12,'playing');x.service.fact(x.service.getSnapshot().intentId,at,'playing');}

test('an engine failure asks for the next route from the same position and shows recovery, not an error',async()=>{
 const x=setup(()=>true);await playing(x);
 const before=x.service.getSnapshot();
 x.service.engineFailed(before.intentId,'decode_error','MEDIA_ERR_DECODE','This media could not be played.');
 assert.equal(x.service.getSnapshot().recovering,true);assert.notEqual(x.service.getSnapshot().phase,'error');
 await tick();await tick();
 const after=x.service.getSnapshot();
 assert.deepEqual(x.reports,[{sessionId:'s1',code:'decode_error',detail:'MEDIA_ERR_DECODE'}]);
 assert.equal(x.created(),2);assert.equal(after.session!.id,'s2');assert.notEqual(after.phase,'error');
 assert.equal(after.pendingSeek!.positionSeconds,40,'the replacement starts where the viewer was');
});

test('the last resort failing, or a server that cannot escalate, is a real failure',async()=>{
 const x=setup(()=>false);await playing(x);
 x.service.engineFailed(x.service.getSnapshot().intentId,'decode_error','','This media could not be played.');await tick();
 assert.equal(x.service.getSnapshot().phase,'error');assert.equal(x.service.getSnapshot().error,'This media could not be played.');assert.equal(x.created(),1);
 const y=setup(()=>null);await playing(y);
 y.service.engineFailed(y.service.getSnapshot().intentId,'source_not_supported','','Nope.');await tick();
 assert.equal(y.service.getSnapshot().phase,'error');
});

test('escalation is bounded per title and a stale engine cannot trigger it',async()=>{
 const x=setup(()=>true);await playing(x);
 const stale=x.service.getSnapshot().intentId;
 for(let i=0;i<5;i++){const s=x.service.getSnapshot();if(s.phase==='error')break;x.service.ready(s.intentId);x.service.engineFailed(s.intentId,'decode_error','','Failed.');await tick();await tick();}
 assert.equal(x.reports.length,3);assert.equal(x.service.getSnapshot().phase,'error');
 x.service.engineFailed(stale,'decode_error','','Failed.');assert.equal(x.reports.length,3);
});
