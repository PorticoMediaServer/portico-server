import assert from 'node:assert/strict';
import test from 'node:test';
import {ConsoleClient,parseReport,parseSettings,parsePage,parseEvidenceRecord,type ConsoleAPI,type RuntimeSettings,type Schedule} from '../src/console.ts';
import {consoleUTF8Size,readConsoleJson,ConsoleTransportError} from '../src/console-transport.ts';
const fence='a'.repeat(64);
const envelope=(data:unknown,serverId='server-one',viewerFence=fence)=>({scope:{serverId,viewerFence},data});
const values:RuntimeSettings={name:'My server',transcodingEnabled:true,perAccountCap:null,serverCap:null,diagnosticDays:30,notificationDays:180,jobDays:30,
 hardwareBackend:'auto',hardwareDevice:'',hdrToneMapping:false,hdrToneMappingAlgorithm:'hable',x264Preset:'veryfast',directStreamRemux:true,
 planningPolicy:'maximum_fidelity',throttleBufferSeconds:60,playedRetentionSeconds:180,temporaryDirectory:'',
 maxConcurrentSessions:0,maxHardwareSessions:0,maxSoftwareSessions:0,maxBackgroundSessions:0};
const settings=(revision=1)=>({revision,digest:'b'.repeat(64),requested:values,effective:values,activeRevision:revision,restartFields:[],registryRevision:'p08.0'});
const report={id:'report-one',revision:1,createdAt:100,updatedAt:100,status:'open',category:'playback',message:'Playback stopped.',contextAvailable:true};
const submission={category:'playback' as const,message:report.message,itemId:''};
function client(handler:(path:string,method:string,body:any,signal:AbortSignal)=>Promise<unknown>){
 const api:ConsoleAPI={async request<T>(){throw new Error('unbounded fallback must not be used');},async requestConsole<T>(path,method,body,signal){return await handler(path,method,body,signal) as T;}};
 return new ConsoleClient(api,{serverId:'server-one',viewerId:'local:one:profile'});
}
test('settings preserve explicit unlimited caps and reject invalid limits',()=>{
 assert.equal(parseSettings(settings()).effective.serverCap,null);
 assert.throws(()=>parseSettings({...settings(),effective:{...values,perAccountCap:0}}));
 assert.throws(()=>parseSettings({...settings(),effective:{...values,diagnosticDays:31}}));
 // The retired per-account overrides are never sent: neither the values the
 // client saves nor the parsed documents carry an accountCaps field, so a
 // save cannot carry it back to a server that rejects it.
 assert.equal('accountCaps' in values,false);
 assert.equal('accountCaps' in parseSettings(settings()).requested,false);
 assert.equal('accountCaps' in parseSettings(settings()).effective,false);
});
test('transcoding settings are parsed and values this server would refuse are rejected',()=>{
 const parsed=parseSettings(settings()).effective;
 assert.equal(parsed.hardwareBackend,'auto');
 assert.equal(parsed.x264Preset,'veryfast');
 assert.equal(parsed.maxConcurrentSessions,0);
 for(const bad of [{hardwareBackend:'magic'},{hdrToneMappingAlgorithm:'filmic'},{x264Preset:'placebo'},{planningPolicy:'fastest'},
  {throttleBufferSeconds:2},{playedRetentionSeconds:99999},{maxHardwareSessions:5000},{hdrToneMapping:'yes'},{temporaryDirectory:42}])
  assert.throws(()=>parseSettings({...settings(),effective:{...values,...bad}}),/Invalid/);
 // A document missing the transcoding half is not a settings document.
 const {hardwareBackend:_omitted,...incomplete}=values as Record<string,unknown>;
 assert.throws(()=>parseSettings({...settings(),effective:incomplete}));
});
test('lost responses reuse the identical mutation key and payload',async()=>{
 const calls:any[]=[];const c=client(async(_p,_m,b)=>{calls.push(b);if(calls.length===1)throw new Error('response lost');return envelope(report);});
 await assert.rejects(c.submit(submission));assert.equal(c.hasUnconfirmed(),true);
 await c.submit(submission);assert.deepEqual(calls[0],calls[1]);assert.equal(c.hasUnconfirmed(),false);c.dispose();
});
test('unknown mutations cannot silently become a changed intent; explicit recovery works',async()=>{
 let fail=true;const bodies:any[]=[];const c=client(async(_p,_m,b)=>{bodies.push(b);if(fail)throw new Error('lost');return envelope(report);});
 c.setDraft('feedback',submission);c.setDraft('unrelated',{keep:true});let recovered=0;c.onReconciled(()=>recovered++);
 await assert.rejects(c.submit(submission));await assert.rejects(c.submit({...submission,message:'Different'}),/unconfirmed/);assert.equal(bodies.length,1);
 fail=false;await c.reconcilePending();assert.deepEqual(bodies[0],bodies[1]);assert.equal(recovered,1);assert.equal(c.getDraft('feedback'),undefined);assert.deepEqual(c.getDraft('unrelated'),{keep:true});c.dispose();
});
test('known revision conflicts keep drafts but permit a deliberate revised submission',async()=>{
 let conflict=true;const c=client(async()=>{if(conflict)throw new ConsoleTransportError(409,'console_conflict',4,['values.serverCap']);return envelope(settings(5));});
 c.setDraft('runtime-settings',{revision:1,values});await assert.rejects(c.applySettings(1,values),/revision: 4/);assert.equal(c.hasUnconfirmed(),false);assert.ok(c.getDraft('runtime-settings'));
 conflict=false;assert.equal((await c.applySettings(4,values)).revision,5);c.dispose();
});
test('server and viewer fences revoke the entire selected scope',async()=>{
 let foreign=false;const c=client(async()=>envelope(settings(),foreign?'server-two':'server-one'));
 await c.settings();c.setDraft('private','do not retain');let denied=0;c.onDenied(()=>denied++);foreign=true;
 await assert.rejects(c.settings(),/selected viewer changed/);assert.equal(c.hasDrafts(),false);assert.equal(denied,1);await assert.rejects(c.settings(),/closed/);c.dispose();
});
test('authority failures clear drafts and late responses after disposal never publish',async()=>{
 const c=client(async()=>{throw new ConsoleTransportError(401,'unauthorized');});c.setDraft('private',true);await assert.rejects(c.settings());assert.equal(c.hasDrafts(),false);c.dispose();
 let resolve!:(v:unknown)=>void;const d=client(async()=>new Promise(r=>resolve=r));const request=d.settings();d.dispose();resolve(envelope(settings()));await assert.rejects(request,/ended/);
});
test('viewer-fence rotation is detected independently of server identity',async()=>{
 let changed=false;const c=client(async()=>envelope(settings(),'server-one',changed?'b'.repeat(64):fence));await c.settings();changed=true;await assert.rejects(c.settings(),/selected viewer changed/);c.dispose();
});
test('schedule writes exclude server-owned revision and admission history',async()=>{
 let sent:any;const schedule:Schedule={id:'schedule-one',revision:7,kind:'retention-cleanup',resource:'',enabled:true,timezone:'America/Toronto',startMinute:60,windowMinutes:90,catchUp:true,lastSlot:'2026-09-06',createdAt:20};
 const c=client(async(path,method,body)=>{assert.equal(path,'/v1/admin/console/schedules');assert.equal(method,'PUT');sent=body;return envelope(schedule);});
 await c.saveSchedule(schedule,7);assert.equal(sent.expectedRevision,7);for(const key of ['revision','createdAt','lastSlot'])assert.equal(key in sent.value,false);assert.ok(sent.idempotencyKey);c.dispose();
});
test('receipt-bearing alerts and capture stay on selected-server routes',async()=>{
 const paths:string[]=[];const c=client(async(path,_method,body)=>{paths.push(path);assert.ok(body.idempotencyKey);return envelope({accepted:true});});
 await c.capture('scheduler',500);await c.acknowledge({id:'alert-one',revision:1,code:'job-failed',severity:'warning',status:'open',firstAt:1,lastAt:2,occurrences:1});assert.ok(paths.every(p=>p.startsWith('/v1/admin/console/')));c.dispose();
});
test('UTF-8 bounds do not require native TextEncoder',()=>{for(const value of ['plain','caf\u00e9','\ud83c\udf7f','\ud800','\ud800X','\u4e2d\u6587'])assert.equal(consoleUTF8Size(value),Buffer.byteLength(value));});
function nativeResponse(value:string,status=200,length:string|null=String(Buffer.byteLength(value))):Response{
 const headers=new Headers({'Content-Type':'application/json'});if(length!==null)headers.set('Content-Length',length);
 return {ok:status>=200&&status<300,status,headers,body:null,text:async()=>value} as Response;
}
test('bounded native fetch accepts text without browser stream primitives',async()=>{assert.deepEqual(await readConsoleJson(nativeResponse('{"value":true}'),new AbortController().signal),{value:true});});
test('native fetch accepts a buffered body without a declared length but rejects deceptive and excessive bounds',async()=>{
 // A compressing proxy (chunked gzip) sends no Content-Length; native fetch has already buffered the body.
 const signal=new AbortController().signal;assert.deepEqual(await readConsoleJson(nativeResponse('{"ok":1}',200,null),signal),{ok:1});
 await assert.rejects(readConsoleJson(nativeResponse(' '.repeat(1048577),200,null),signal));
 await assert.rejects(readConsoleJson(nativeResponse('{}',200,'1048577'),signal));await assert.rejects(readConsoleJson(nativeResponse(' '.repeat(1048577),200,'2'),signal));
});
test('browser streaming enforces its real byte bound and abort state',async()=>{
 const signal=new AbortController().signal;assert.deepEqual(await readConsoleJson(new Response('{"ok":1}',{headers:{'Content-Type':'application/json'}}),signal),{ok:1});
 await assert.rejects(readConsoleJson(new Response(' '.repeat(1048577),{headers:{'Content-Type':'application/json'}}),signal));
 const aborted=new AbortController();aborted.abort();await assert.rejects(readConsoleJson(nativeResponse('{}'),aborted.signal));
});
test('server error text is never rendered, but validated field and revision hints survive',async()=>{
 try{await readConsoleJson(nativeResponse(JSON.stringify({error:{code:'console_conflict',message:'secret /private/token',currentRevision:5,fields:['values.name']}}),409),new AbortController().signal);assert.fail('must reject');}
 catch(e){assert.ok(e instanceof ConsoleTransportError);assert.equal(e.currentRevision,5);assert.deepEqual(e.fields,['values.name']);assert.ok(!e.message.includes('/private'));}
});
test('invalid media type, JSON and injected field names fail closed',async()=>{
 const signal=new AbortController().signal;await assert.rejects(readConsoleJson(new Response('<html/>',{headers:{'Content-Type':'text/html'}}),signal));await assert.rejects(readConsoleJson(nativeResponse('not JSON'),signal));
 try{await readConsoleJson(nativeResponse(JSON.stringify({error:{code:'invalid_console_request',fields:['secret/token\n']}}),400),signal);}catch(e){assert.ok(e instanceof ConsoleTransportError);assert.equal(e.fields,undefined);}
});
test('paged projections reject unbounded and malformed cursors',()=>{assert.throws(()=>parsePage({items:Array.from({length:41},()=>1),nextCursor:''},v=>v));assert.throws(()=>parsePage({items:[],nextCursor:'../secret'},v=>v));});

test('recovered support exports remain available for a manual preview without creating another export',async()=>{
 let calls=0;const value={id:'export-one',expiresAt:Date.now()+3600000,manifest:{version:'1',from:1,to:2,included:['snapshot'],excluded:['raw logs'],redaction:'typed projection',maxBytes:1048576}};
 const c=client(async()=>{if(++calls===1)throw new Error('lost export response');return envelope(value);});
 await assert.rejects(c.createExport(1,2,['snapshot']));await c.reconcilePending();assert.equal(calls,2);
 assert.equal(c.takeRecoveredExport()?.id,value.id);assert.equal(c.takeRecoveredExport(),undefined);c.dispose();
});

test('multi-source job pause and resume preserve selected-server revision fencing',async()=>{
 const calls:{path:string;body:any}[]=[];
 const job={id:'operation-one',kind:'library-scan',resource:'library',trigger:'owner',state:'running' as const,phase:'all sources',revision:2,attempt:1,createdAt:1,updatedAt:2,nextAt:3,domainId:'operation-one',settingsRevision:1,lane:'background-media',actions:['pause','cancel'] as ('pause'|'cancel')[]};
 const c=client(async(path,_method,body)=>{calls.push({path,body});const paused=path.endsWith('/pause');return envelope({...job,revision:paused?3:4,state:paused?'paused':'running',actions:paused?['resume','cancel']:['pause','cancel']});});
 const paused=await c.jobCommand(job,'pause');assert.equal(paused.state,'paused');assert.deepEqual(paused.actions,['resume','cancel']);
 const resumed=await c.jobCommand(paused,'resume');assert.equal(resumed.state,'running');
 assert.equal(calls[0].path,'/v1/admin/console/jobs/operation-one/pause');assert.equal(calls[0].body.expectedRevision,2);
 assert.equal(calls[1].path,'/v1/admin/console/jobs/operation-one/resume');assert.equal(calls[1].body.expectedRevision,3);
 assert.notEqual(calls[0].body.idempotencyKey,calls[1].body.idempotencyKey);c.dispose();
});

test('diagnostic records reject invalid dates and unsafe field values before rendering',()=>{
 const value={sequence:1,at:Date.now(),severity:'info',component:'scheduler',code:'job_completed',fields:{count:2,durationMs:50}};
 assert.deepEqual(parseEvidenceRecord(value),value);
 for(const change of [{at:8640000000000001},{severity:{}},{fields:{count:null}},{fields:{count:Infinity}},{fields:[]}])assert.throws(()=>parseEvidenceRecord({...value,...change}));
});

test('CD-38: current-app taxonomy leaves and admin triage events parse; legacy submit stays narrow',()=>{
 const leaf={...report,category:'playback.stall',events:[{sequence:1,at:200,revision:2,status:'in-progress',reply:'Looking into it.',actorClass:'admin'}]};
 const parsed=parseReport(leaf);
 assert.equal(parsed.category,'playback.stall');
 assert.equal(parsed.events?.[0].actorClass,'admin');
 // The legacy four still parse.
 assert.equal(parseReport(report).category,'playback');
 // Malformed identifiers and unknown actor roles still fail.
 for(const change of [{category:'playback stall!'},{category:''},{id:'../secret'},{events:[{sequence:1,at:200,revision:2,status:'in-progress',reply:'x',actorClass:'moderator'}]},{events:[{sequence:1,at:200,revision:2,status:'in-progress',reply:'x',actorClass:''}]}])assert.throws(()=>parseReport({...leaf,...change}));
});
