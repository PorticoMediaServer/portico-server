import test from 'node:test';
import assert from 'node:assert/strict';
import {DVRClient,DVRModel,parseDVRPage,parseRecording,validRecordingOptions,validStoragePolicy,defaultRecordingOptions,dvrMessage,countUnwatched,seriesRuleConfig} from '../src/dvr.ts';
import type {ChannelApi} from '../src/channel-guide.ts';
const hex=(n=64,c='a')=>c.repeat(n);
const time='2026-09-06T12:00:00Z',end='2026-09-06T13:00:00Z';
const recording=()=>({id:hex(),revision:1,occurrence:{sourceId:hex(48),channelId:hex(64,'b'),generation:hex(48,'c'),programmeId:hex(64,'d')},programme:{id:hex(64,'d'),channelId:hex(64,'b'),title:'Owned programme',start:time,end,lineage:'provider-id',seriesId:'series',episodeId:'episode',newEvidence:'unknown'},options:{...defaultRecordingOptions},start:time,end,state:'scheduled',reason:'',keep:false,itemId:'',libraryId:'',coverageStart:'',coverageEnd:'',bytes:0,ruleId:'',conflicts:[]});
const page=()=>({recordings:[recording()],rules:[],nextCursor:'',revision:1,captureAvailable:true,captureUnavailableReason:'',deletionAvailable:true,usage:{bytes:0,pendingDeleteBytes:0,recordings:1}});
const envelope=(result:unknown)=>({protocolVersion:'1.0',serverId:'server',result});
const nonce=async()=>hex(48,'e');
test('DVR public DTOs project known fields and reject foreign identity or invalid intervals',()=>{
 const raw=recording();Object.assign(raw,{path:'/private/secret',providerURL:'secret'});Object.assign(raw.options,{secret:'secret'});Object.assign(raw.programme,{secret:'secret'});
 assert.ok(!JSON.stringify(parseRecording(raw)).includes('secret'));
 raw.occurrence.programmeId=hex(64,'f');assert.throws(()=>parseRecording(raw));
 raw.occurrence.programmeId=raw.programme.id;raw.end=raw.start;assert.throws(()=>parseRecording(raw));
 raw.end=end;raw.programme.start='2026-02-30T12:00:00Z';assert.throws(()=>parseRecording(raw));
 const p=page();p.recordings.push(p.recordings[0]);assert.throws(()=>parseDVRPage(p));
});
test('DVR projections preserve observed incomplete coverage without inventing play authority',()=>{
 const r=recording();r.state='incomplete-playable';r.itemId=hex(64,'1');r.libraryId=hex(64,'2');r.coverageStart=time;r.coverageEnd='2026-09-06T12:30:00Z';r.bytes=188*100;
 const got=parseRecording(r);assert.equal(got.state,'incomplete-playable');assert.equal(got.coverageEnd,r.coverageEnd);assert.equal(got.libraryId,r.libraryId);assert.ok(!('streamURL' in got));
 r.libraryId='';assert.throws(()=>parseRecording(r));
});
test('DVR mutation retries reuse operation id and expected revision, including unknown outcomes',async()=>{
 let calls=0,ids=0;const sent:Array<{path:string;method:string;body:any}>=[];
 const api:ChannelApi={request:async<T>(path,method,body)=>{sent.push({path,method:method??'GET',body});if(++calls===1)throw new Error('unknown outcome');return envelope(recording()) as T;}};
 const client=new DVRClient(api,'server',async()=>{ids++;return nonce()});const r=parseRecording(recording());
 await assert.rejects(client.update(r,r.options));await client.update(r,r.options);
 assert.equal(ids,1);assert.deepEqual(sent[0],sent[1]);assert.equal(sent[0].body.expectedRevision,1);
 await client.keep(r);assert.equal(ids,2);assert.equal(sent[2].body.keep,true);client.dispose();
});
test('DVR stale revision is not automatically rebased or replayed with a fresh receipt',async()=>{
 const sent:any[]=[];const api:ChannelApi={request:async<T>(_p,_m,b)=>{sent.push(b);throw {status:409}}};
 const client=new DVRClient(api,'server',nonce);const r=parseRecording(recording());await assert.rejects(client.cancel(r));
 assert.equal(sent.length,1);assert.equal(sent[0].expectedRevision,r.revision);assert.match(dvrMessage({status:409}),/Refresh/);client.dispose();
});
test('DVR checks selected server, capacity shape, options bounds and exact rule nonce length',async()=>{
 let response:unknown={...envelope(page()),serverId:'foreign'};
 const api:ChannelApi={request:async<T>()=>response as T};const client=new DVRClient(api,'server',nonce);
 await assert.rejects(client.list('upcoming'));response=envelope({recording:recording(),capacity:{known:true,effective:0},overlaps:[],nextOverlap:''});await assert.rejects(client.detail(hex()));
 assert.equal((await client.newRuleID()).length,64);assert.equal(validRecordingOptions({...defaultRecordingOptions,beforeSeconds:21601}),false);
 assert.equal(validRecordingOptions({...defaultRecordingOptions,priority:-1000}),true);assert.equal(validStoragePolicy({revision:1,retentionDays:0,episodeLimit:0,floorBytes:Number.MAX_SAFE_INTEGER+1,capBytes:0}),false);client.dispose();
});
test('DVR late route responses and disposed viewers cannot revive private state',async()=>{
 const pending:Array<(v:unknown)=>void>=[];const api:ChannelApi={request:<T>()=>new Promise<T>(resolve=>pending.push(v=>resolve(v as T)))};
 const model=new DVRModel(new DVRClient(api,'server',nonce));const first=model.load({view:'upcoming'}),second=model.load({view:'history'});
 pending[1](envelope({...page(),revision:2}));await second;pending[0](envelope(page()));await first;assert.equal(model.getSnapshot().page?.revision,2);assert.equal(model.getSnapshot().query.view,'history');
 const third=model.load({view:'recorded'});model.dispose();pending[2](envelope(page()));await third;assert.equal(model.getSnapshot().page,null);assert.equal(model.getSnapshot().detail,null);
});
test('DVR temporary errors retain known same-route state, permission changes clear it',async()=>{
 let error:unknown;const api:ChannelApi={request:async<T>()=>{if(error)throw error;return envelope(page()) as T;}};
 const model=new DVRModel(new DVRClient(api,'server',nonce));await model.load();error=new Error('/private/path/provider-password');await model.load();assert.equal(model.getSnapshot().page?.recordings.length,1);assert.ok(!model.getSnapshot().error.includes('private'));
 error={status:403};await model.load();assert.equal(model.getSnapshot().page,null);model.dispose();
});
test('DVR deadline settles an adapter which ignores AbortSignal',{timeout:1000},async()=>{
 const api:ChannelApi={request:<T>()=>new Promise<T>(()=>{})};const client=new DVRClient(api,'server',nonce,5);await assert.rejects(client.list('upcoming'),/timed out/);client.dispose();
});
test('series rule DTOs do not retain injected provider fields',()=>{
 const p:any=page();p.rules=[{id:hex(),revision:1,reconcileState:'ready',diagnostic:'',updatedAt:time,secret:'secret',config:{name:'Rule',sourceId:hex(48),seriesId:'series',enabled:true,episodes:'new',allowedChannels:[],blockedChannels:[],keywords:[],blockedKeywords:[],options:{...defaultRecordingOptions,secret:'secret'},secret:'secret'}}];
 assert.ok(!JSON.stringify(parseDVRPage(p)).includes('secret'));assert.equal(parseDVRPage(p).rules[0].config.episodes,'new');
});

test('Part 2.2: channel and watched parse behind presence; unwatched counts only false',()=>{
 const withBoth={...recording(),channel:{id:hex(),name:'Channel One',number:'1.1'},watched:false};
 const parsed=parseRecording(withBoth);
 assert.equal(parsed.channel!.name,'Channel One');
 assert.equal(parsed.watched,false);
 assert.equal(parseRecording({...recording(),watched:null}).watched,null);
 assert.equal(parseRecording({...recording(),watched:true}).watched,true);
 // Absent on older servers stays absent.
 assert.equal('channel' in parseRecording(recording()),false);
 assert.equal('watched' in parseRecording(recording()),false);
 // Empty channel name (gone from guide and source) decodes.
 assert.equal(parseRecording({...recording(),channel:{id:hex(),name:'',number:''}}).channel!.name,'');
 assert.throws(()=>parseRecording({...recording(),channel:{id:'bad',name:'X',number:'1'}}));
 assert.throws(()=>parseRecording({...recording(),watched:'yes' as any}));
 // Null counts as neither watched nor unwatched.
 assert.equal(countUnwatched([{watched:false},{watched:false},{watched:true},{watched:null},{}]),2);
 assert.equal(countUnwatched([]),0);
});

test('all DVR read projections drop unknown nested fields and validate overlap intervals',async()=>{
 let response:unknown;const api:ChannelApi={request:async<T>()=>envelope(response) as T};
 const client=new DVRClient(api,'server',nonce),r=parseRecording(recording());
 const cap={known:true,effective:2,planningEstimate:2,mode:'configured',secret:'secret'};
 const overlap={id:hex(64,'f'),title:'Other recording',start:time,end,priority:0,state:'scheduled',secret:'secret'};
 response={recording:recording(),capacity:cap,overlaps:[overlap],nextOverlap:'',secret:'secret'};
 assert.ok(!JSON.stringify(await client.detail(r.id)).includes('secret'));
 response={recording:recording(),capacity:cap,overlaps:[{...overlap,end:time}],nextOverlap:''};await assert.rejects(client.detail(r.id));
 const cfg={name:'Rule',sourceId:hex(48),seriesId:'series',enabled:true,episodes:'all' as const,allowedChannels:[],blockedChannels:[],keywords:[],blockedKeywords:[],options:defaultRecordingOptions};
 response={matches:1,unknownNewEvidence:1,sample:[recording().programme],generation:hex(48),capacity:cap,secret:'secret'};
 assert.ok(!JSON.stringify(await client.preview(cfg)).includes('secret'));
 response={channels:[{id:hex(),name:'Channel',number:'1',secret:'secret'}],generation:hex(48),nextCursor:'',secret:'secret'};
 assert.ok(!JSON.stringify(await client.channels(hex(48))).includes('secret'));
 const policy={revision:1,retentionDays:0,episodeLimit:0,floorBytes:1,capBytes:0,secret:'secret'};
 response={policy,measurement:{freeBytes:1,usedBytes:0,reservedBytes:0,writeHealthy:true,measuredAt:time,secret:'secret'},pendingDeleteBytes:0,forecastBytes:0,forecastHours:24,forecastDescription:'Estimate',captureAvailable:true,warning:'',secret:'secret'};
 assert.ok(!JSON.stringify(await client.storage()).includes('secret'));
 response=policy;assert.ok(!JSON.stringify(await client.saveStorage(policy)).includes('secret'));
 response={recordingId:r.id,revision:1,bytes:0,keep:false,activeReaders:false,result:'Delete?',secret:'secret'};
 assert.ok(!JSON.stringify(await client.deletePreview(r)).includes('secret'));client.dispose();
});

test('overlap denial clears private details and invalidates other responses in this viewer',async()=>{
 const pending:Array<{resolve:(v:unknown)=>void;reject:(e:unknown)=>void}>=[];
 let delayed=false;
 const detail={recording:recording(),capacity:{known:true,effective:1},overlaps:[],nextOverlap:hex(64,'f')};
 const api:ChannelApi={request:<T>()=>delayed?new Promise<T>((resolve,reject)=>pending.push({resolve:v=>resolve(v as T),reject})):Promise.resolve(envelope(detail) as T)};
 const model=new DVRModel(new DVRClient(api,'server',nonce));await model.load({view:'upcoming',recordingId:hex()});delayed=true;
 const more=model.moreOverlaps(),refresh=model.load();pending[0].reject({status:403});await more;
 assert.equal(model.getSnapshot().detail,null);assert.equal(model.getSnapshot().accessDenied,true);
 pending[1].resolve(envelope(detail));await refresh;assert.equal(model.getSnapshot().detail,null);model.dispose();
});

test('editor requests using the shared client invalidate model presentation on denial',async()=>{
 let denied=false;const api:ChannelApi={request:async<T>()=>{if(denied)throw {status:401};return envelope(page()) as T}};
 const model=new DVRModel(new DVRClient(api,'server',nonce));await model.load();denied=true;
 await assert.rejects(model.client.deletePreview(parseRecording(recording())));
 assert.equal(model.getSnapshot().page,null);assert.equal(model.getSnapshot().accessDenied,true);
 denied=false;await model.load();assert.equal(model.getSnapshot().accessDenied,false);assert.equal(model.getSnapshot().page?.recordings.length,1);model.dispose();
});

test('duplicate overlap pagination responses cannot append the same rows twice',async()=>{
 let wait=false;const pending:Array<(v:unknown)=>void>=[];
 const detail={recording:recording(),capacity:{known:true,effective:1},overlaps:[],nextOverlap:hex(64,'f')};
 const api:ChannelApi={request:<T>()=>wait?new Promise<T>(resolve=>pending.push(v=>resolve(v as T))):Promise.resolve(envelope(detail) as T)};
 const model=new DVRModel(new DVRClient(api,'server',nonce));await model.load({view:'upcoming',recordingId:hex()});wait=true;
 const first=model.moreOverlaps(),second=model.moreOverlaps();
 const next=envelope({...detail,nextOverlap:'',overlaps:[{id:hex(64,'e'),title:'Other',start:time,end,priority:0,state:'scheduled'}]});
 pending[1](next);await second;pending[0](next);await first;
 assert.equal(model.getSnapshot().detail?.overlaps.length,1);model.dispose();
});

test('effect replay reactivates the memoized model without reviving old requests or cleanup',async()=>{
 const pending:Array<(v:unknown)=>void>=[];
 const api:ChannelApi={request:<T>()=>new Promise<T>(resolve=>pending.push(v=>resolve(v as T)))};
 const model=new DVRModel(new DVRClient(api,'server',nonce));
 const old=model.activate(),first=model.load();model.deactivate(old);
 assert.equal(model.getSnapshot().page,null);
 const current=model.activate(),second=model.load({view:'recorded'});
 pending[1](envelope({...page(),revision:2}));await second;
 pending[0](envelope(page()));await first;model.deactivate(old);
 assert.equal(model.getSnapshot().page?.revision,2);assert.equal(model.getSnapshot().query.view,'recorded');
 model.deactivate(current);assert.equal(model.getSnapshot().page,null);
});


test('an old action cannot clear a newer activation mutation fence', async()=>{
 const api={request:async()=>({protocolVersion:'1.0',serverId:'server',result:page()})} as ChannelApi;
 const model=new DVRModel(new DVRClient(api,'server',nonce));
 const oldLease=model.activate();
 let finishOld!:()=>void,finishNew!:()=>void;
 const oldAction=model.act(()=>new Promise<void>(resolve=>{finishOld=resolve}));
 model.deactivate(oldLease);model.activate();
 const newAction=model.act(()=>new Promise<void>(resolve=>{finishNew=resolve}));
 finishOld();assert.equal(await oldAction,false);
 assert.equal(model.getSnapshot().mutating,true);
 let ran=false;assert.equal(await model.act(async()=>{ran=true}),false);assert.equal(ran,false);
 finishNew();assert.equal(await newAction,true);assert.equal(model.getSnapshot().mutating,false);
 model.dispose();
});


test('late nonce failure from disposed client lifetime cannot delete a new retry receipt',async()=>{
 let rejectOld!:(e:unknown)=>void,ids=0;
 const api={request:async()=>{throw new Error('unknown transport outcome')}} as ChannelApi;
 const client=new DVRClient(api,'server',()=>++ids===1?new Promise<string>((_,reject)=>{rejectOld=reject}):nonce());
 const r=parseRecording(recording());
 const old=assert.rejects(client.keep(r));client.activate();
 await assert.rejects(client.keep(r));
 rejectOld(new Error('old nonce failed'));await old;
 await assert.rejects(client.keep(r));assert.equal(ids,2);
 client.dispose();
});

test('FEAT-02: rule-editor state maps to RuleConfig (new/all, keep, padding, channel scope)',()=>{
 const draft=()=>({channel:{sourceId:hex(48),id:hex(64,'b'),generation:hex(48,'c')},programme:{id:hex(64,'d'),title:'Show',seriesId:'series'},series:false} as any);
 const {config,anchor}=seriesRuleConfig(draft(),{episodes:'new',keep:5,beforeSeconds:60,afterSeconds:300,anyChannel:false});
 assert.equal(config.episodes,'new');
 assert.equal(config.options.episodeLimit,5);
 assert.equal(config.options.beforeSeconds,60);
 assert.equal(config.options.afterSeconds,300);
 assert.deepEqual(config.allowedChannels,[hex(64,'b')]);
 assert.deepEqual(anchor,{sourceId:hex(48),channelId:hex(64,'b'),generation:hex(48,'c'),programmeId:hex(64,'d')});
 const any=seriesRuleConfig(draft(),{episodes:'all',keep:0,beforeSeconds:0,afterSeconds:0,anyChannel:true});
 assert.equal(any.config.episodes,'all');
 assert.deepEqual(any.config.allowedChannels,[]);
 assert.ok(validRecordingOptions(any.config.options));
 // Out-of-range editor state never reaches the server.
 assert.throws(()=>seriesRuleConfig(draft(),{episodes:'some' as any,keep:0,beforeSeconds:0,afterSeconds:0,anyChannel:true}));
 assert.throws(()=>seriesRuleConfig(draft(),{episodes:'new',keep:-1,beforeSeconds:0,afterSeconds:0,anyChannel:true}));
 assert.throws(()=>seriesRuleConfig(draft(),{episodes:'new',keep:0,beforeSeconds:21601,afterSeconds:0,anyChannel:true}));
 const bad=()=>({channel:{sourceId:'bad',id:hex(64,'b'),generation:hex(48,'c')},programme:{id:hex(64,'d'),title:'Show',seriesId:'series'},series:false} as any);
 assert.throws(()=>seriesRuleConfig(bad(),{episodes:'new',keep:0,beforeSeconds:0,afterSeconds:0,anyChannel:true}));
});

test('FEAT-08: a recording carries the server\'s seriesId and seriesTitle; an unreadable one is left out, never fatal',()=>{
 const named=parseRecording({...recording(),seriesId:'show-1',seriesTitle:'The Show'});
 assert.equal(named.seriesId,'show-1');assert.equal(named.seriesTitle,'The Show');
 const odd=parseRecording({...recording(),seriesId:42,seriesTitle:''});
 assert.equal(odd.seriesId,undefined);assert.equal(odd.seriesTitle,undefined);
});
