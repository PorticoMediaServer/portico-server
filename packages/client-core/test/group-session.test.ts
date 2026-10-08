import {test} from 'node:test';
import assert from 'node:assert/strict';
import {GroupSessionService,GroupFollower,type GroupPlayer} from '../src/group-session.ts';
import {SOCIAL_PROTOCOL} from '../src/social-playback.ts';

const T0=Date.parse('2026-09-16T20:00:00Z');
const sync={noCorrectionUnderMs:750,rateCorrectionMinimum:'0.90',rateCorrectionMaximum:'1.10',rateCorrectionMaxMs:4000,seekAtOrOverMs:3000};
const timeline={itemId:'item',currentEntryId:'ent_1',state:'playing',anchorPositionUs:'10000000',anchorAt:'2026-09-16T20:00:00Z',rate:{numerator:'1',denominator:'1'},queuePosition:0};
const host={id:'mem_1',displayName:'Host',role:'host',state:'joined',readiness:'ready',positionUs:'0',reportedAt:'2026-09-16T20:00:00Z',presence:'connected',joinedAt:'2026-09-16T19:59:00Z'};
const guest={...host,id:'mem_2',displayName:'Guest',role:'member'};
const group=(over:Record<string,unknown>={})=>({
 id:'grp_1',name:'Movie night',state:'playing',hostAuthority:'host-only',hostMemberId:'mem_1',revision:'7',playbackRevision:'3',queueRevision:'2',reconnectGeneration:'1',
 lastCommand:'play',lastCommandId:'p',endedReason:'',eventOrdinal:'9',createdAt:'2026-09-16T19:59:00Z',updatedAt:'2026-09-16T20:00:00Z',
 permissions:{isHost:true,canControl:true,canManageQueue:true},authority:{deviceId:'dev_1',playbackId:'pb1',state:'bound'},
 host:{presence:'connected',lastSeenAt:'2026-09-16T20:00:00Z',pauseAt:null,endAt:null},timeline,settings:{shuffleEnabled:false,repeatMode:'none'},sync,
 readiness:{aggregate:'ready',ready:2,buffering:0,lagging:0,stale:0,memberCount:2},members:[host,guest],queue:null,viewerMemberId:'mem_1',...over});
const snap=(over:Record<string,unknown>={},serverTime='2026-09-16T20:00:00Z')=>({protocolVersion:SOCIAL_PROTOCOL,serverTime,group:group(over)});
const frame=(kind:string,data:unknown,id?:string)=>(id?`id: ${id}\n`:'')+`event: ${kind}\ndata: ${JSON.stringify(data)}\n\n`;
const settle=()=>new Promise(r=>setTimeout(r,15));

/** A server whose event stream the test writes to by hand. */
function server(){
 const calls:{path:string;method:string;body?:any}[]=[];const streams:{lastEventId:string;write(text:string):void;close():void}[]=[];
 let current=snap();let refuse:Error|undefined;
 const api={baseUrl:'https://s',request:async<T,>(path:string,method='GET',body?:unknown):Promise<T>=>{
  calls.push({path,method,body});
  if(refuse&&method==='POST'&&path.endsWith('/transport')){const e=refuse;refuse=undefined;throw e;}
  if(path.endsWith('/transport'))return {protocolVersion:SOCIAL_PROTOCOL,groupId:'grp_1',idempotencyKey:(body as any).idempotencyKey,disposition:'accepted',command:(body as any).command,revision:'8',queueRevision:'2',serverTime:'2026-09-16T20:00:01Z',recordedAt:'2026-09-16T20:00:01Z',timeline:{...timeline,state:'paused'},settings:{shuffleEnabled:false,repeatMode:'none'},override:null} as T;
  if(path.endsWith('/queue'))return {protocolVersion:SOCIAL_PROTOCOL,serverTime:'x',groupId:'grp_1',queue:{revision:'3',position:0,entries:[{entryId:'ent_1',position:0,itemId:'item',unavailable:false,addedBy:'mem_1'}],eligibility:null}} as T;
  if(path.endsWith('/invites'))return {protocolVersion:SOCIAL_PROTOCOL,serverTime:'x',invite:{id:'inv_1',groupId:'grp_1',code:'ABCDEFGH',expiresAt:'2026-09-16T20:15:00Z',maxUses:8,uses:0,recipientProfileId:''}} as T;
  if(path==='/v1/groups'&&method==='GET')return {protocolVersion:SOCIAL_PROTOCOL,serverTime:'x',groups:[current.group]} as T;
  return current as T;
 }};
 const stream=async(_path:string,lastEventId:string,signal:AbortSignal)=>{
  let controller!:ReadableStreamDefaultController<Uint8Array>;const encoder=new TextEncoder();
  const body=new ReadableStream<Uint8Array>({start(c){controller=c;}});
  signal.addEventListener('abort',()=>{try{controller.close();}catch{}},{once:true});
  streams.push({lastEventId,write:t=>controller.enqueue(encoder.encode(t)),close:()=>{try{controller.close();}catch{}}});
  return new Response(body,{status:200});
 };
 return {api,stream,calls,streams,set:(next:ReturnType<typeof snap>)=>{current=next;},refuseTransport:(e:Error)=>{refuse=e;}};
}
const make=(s:ReturnType<typeof server>,now=()=>T0)=>{let n=0;return new GroupSessionService({api:s.api,stream:s.stream,key:()=>'key-'+(++n),now,heartbeatMs:3600000});};

test('joining tidies the code, enters the room and follows the stream', async()=>{
 const s=server(),service=make(s);
 assert.equal(await service.join(' abcd-efgh ','Sam'),true);
 assert.equal(s.calls[0].body.code,'ABCDEFGH');
 assert.equal(service.getSnapshot().phase,'live');
 await settle();assert.equal(s.streams.length,1);
 s.set(snap({state:'paused',revision:'8',timeline:{...timeline,state:'paused',anchorPositionUs:'42000000',anchorAt:'2026-09-16T20:00:30Z'}}));
 s.streams[0].write(frame('group.transport',{command:'pause',state:'paused',itemId:'item',currentEntryId:'ent_1',anchorPositionUs:'42000000',anchorAt:'2026-09-16T20:00:30Z',rate:{numerator:'1',denominator:'1'},queuePosition:0,revision:'8'},'10'));
 await settle();
 const g=service.getSnapshot().group!;
 assert.equal(g.timeline.state,'paused');assert.equal(g.timeline.anchorPositionUs,'42000000');
 service.dispose();
});

test('a dropped stream keeps the room and resumes from the last ordinal', async()=>{
 const s=server(),service=make(s);
 await service.open('grp_1');await settle();
 s.streams[0].write(frame('group.members',{revision:'8'},'11'));await settle();
 s.streams[0].close();await settle();
 assert.equal(service.getSnapshot().phase,'reconnecting');
 assert.ok(service.getSnapshot().group,'the room is still on screen');
 await new Promise(r=>setTimeout(r,900));
 assert.equal(s.streams.length,2);assert.equal(s.streams[1].lastEventId,'11');
 // Heartbeats carry no id and must not move the resume point. A pushed heartbeat has no round trip,
 // so it does not override the clock a request measured (BE-MEDIA-14).
 s.streams[1].write(frame('heartbeat',{serverTime:'2026-09-16T20:00:05Z'}));await settle();
 assert.equal(service.getSnapshot().phase,'live');assert.equal(service.getSnapshot().clockOffsetMs,0);
 service.dispose();
});

test('the last frame ends the room without discarding it', async()=>{
 const s=server(),service=make(s);
 await service.open('grp_1');await settle();
 s.set(snap({state:'ended',endedReason:'host-ended',timeline:{...timeline,state:'stopped'}}));
 s.streams[0].write(frame('group.ended',{reason:'host-ended'},'12'));await settle();
 assert.equal(service.getSnapshot().phase,'ended');assert.equal(service.getSnapshot().group?.endedReason,'host-ended');
 service.dispose();
});

test('a command carries the revision it saw; a conflict re-reads and is not resent', async()=>{
 const s=server(),service=make(s);
 await service.open('grp_1');
 s.set(snap({state:'paused',revision:'8',timeline:{...timeline,state:'paused'}}));
 assert.equal(await service.transport('pause'),true);
 const sent=s.calls.find(c=>c.path.endsWith('/transport'))!;
 assert.equal(sent.body.expectedRevision,'7');assert.equal(sent.body.idempotencyKey,'key-1');
 assert.equal(service.getSnapshot().group!.timeline.state,'paused');await settle();
 s.refuseTransport(Object.assign(new Error('The group changed.'),{code:'revision_conflict',status:409}));
 assert.equal(await service.transport('play'),false);
 assert.equal(s.calls.filter(c=>c.path.endsWith('/transport')).length,2,'never resent blindly');
 assert.match(service.getSnapshot().error!,/changed/);
 service.dispose();
});

test('creating posts the group itself and sends no lane: the server binds this device', async()=>{
 const s=server(),service=make(s);
 assert.equal(await service.create({name:' Night ',displayName:'Sam',hostAuthority:'anyone'}),true);
 const created=s.calls.find(c=>c.path==='/v1/groups'&&c.method==='POST')!;
 assert.deepEqual(created.body,{protocolVersion:SOCIAL_PROTOCOL,name:'Night',displayName:'Sam',hostAuthority:'anyone'});
 assert.equal(service.getSnapshot().phase,'live');
 service.dispose();
});

test('being made host heartbeats at once to bind this device; handing over names no lane', async()=>{
 const s=server();
 s.set(snap({permissions:{isHost:false,canControl:false,canManageQueue:false},viewerMemberId:'mem_2'}));
 const service=make(s);
 await service.open('grp_1');await settle();
 s.set(snap({hostMemberId:'mem_2',viewerMemberId:'mem_2',revision:'8',members:[{...host,role:'member'},{...guest,role:'host'}]}));
 s.streams[0].write(frame('group.host',{revision:'8'},'13'));await settle();
 const beats=s.calls.filter(c=>c.path==='/v1/groups/grp_1/heartbeat'&&c.method==='POST');
 assert.equal(beats.length,1);assert.equal(beats[0].body,undefined);
 await service.makeHost('mem_1');
 const handed=s.calls.filter(c=>c.path.endsWith('/host-transfer')).at(-1)!;
 assert.deepEqual(Object.keys(handed.body).sort(),['expectedRevision','memberId','protocolVersion']);
 service.dispose();
});

test('invites and the queue are read through their envelopes', async()=>{
 const s=server(),service=make(s);
 await service.open('grp_1');
 assert.equal(await service.invite(),true);assert.equal(service.getSnapshot().invite?.code,'ABCDEFGH');
 assert.equal(await service.queueAdd(['item']),true);await settle();
 assert.equal(service.getSnapshot().queue?.entries[0].itemId,'item');
 assert.equal(s.calls.find(c=>c.method==='POST'&&c.path.endsWith('/queue'))!.body.expectedRevision,'2');
 service.dispose();
});

function player(start:{itemId:string;positionSeconds:number;playing?:boolean}){
 const log:string[]=[];const state={itemId:start.itemId,positionSeconds:start.positionSeconds,playing:start.playing??true,buffering:false,ready:true};
 const p:GroupPlayer={state:()=>({...state}),load:(id,at,playing)=>{log.push(`load ${id} ${at} ${playing}`);},play:()=>{log.push('play');state.playing=true;},pause:()=>{log.push('pause');state.playing=false;},seek:at=>{log.push('seek '+at);state.positionSeconds=at;},setRate:r=>{log.push('rate '+r);}};
 return {p,log,state};
}

test('the follower loads, nudges, then seeks, in the bands the server publishes', async()=>{
 const s=server();const now=T0+20000;s.set(snap({},'2026-09-16T20:00:20Z'));const service=make(s,()=>now);
 await service.open('grp_1');
 // The group is 20 s past its anchor of 10 s, so the target is 30 s.
 const other=player({itemId:'other',positionSeconds:0});const a=new GroupFollower(service,other.p,()=>now);a.start();a.stop();
 assert.equal(other.log[0],'load item 30 true');
 const near=player({itemId:'item',positionSeconds:29.8});const b=new GroupFollower(service,near.p,()=>now);b.start();b.stop();
 assert.deepEqual(near.log,['rate 1'],'inside the quiet band nothing is touched; stop restores speed');
 const behind=player({itemId:'item',positionSeconds:28.5});const c=new GroupFollower(service,behind.p,()=>now);c.start();c.stop();
 assert.equal(behind.log[0],'rate 1.1');
 const lost=player({itemId:'item',positionSeconds:5});const d=new GroupFollower(service,lost.p,()=>now);d.start();d.stop();
 assert.ok(lost.log.includes('seek 30'));
 const reports=s.calls.filter(c=>c.path.endsWith('/readiness')).map(c=>c.body.readiness);
 assert.deepEqual(reports,['buffering','ready','ready','lagging']);
 service.dispose();
});

test('a paused group holds the follower at the anchor', async()=>{
 const s=server();s.set(snap({state:'paused',timeline:{...timeline,state:'paused'}}));
 const service=make(s,()=>T0+60000);await service.open('grp_1');
 const local=player({itemId:'item',positionSeconds:40});const f=new GroupFollower(service,local.p,()=>T0+60000);f.start();f.stop();
 assert.ok(local.log.includes('pause'));assert.ok(local.log.includes('seek 10'),'no extrapolation while paused');
 service.dispose();
});

test('service messages come from the catalogue with their IDs, never from error.message (X-04)',async()=>{
 const {createI18n,resolveRegion}=await import('../../i18n/src/index.ts');
 const failing={request:async()=>{throw new Error('ECONNRESET socket hang up');}};
 const s=new GroupSessionService({api:failing as never,stream:(()=>{throw new Error('no stream');}) as never,heartbeatMs:3600000});
 assert.equal(await s.join('ABCD','Me'),false);
 const snap=s.getSnapshot();
 assert.equal(snap.errorId,'together.error.join');
 assert.equal(snap.error,'That code didn’t work.');
 assert.ok(!/socket/.test(snap.error!));
 const canada=new GroupSessionService({api:failing as never,stream:(()=>{throw new Error('no');}) as never,heartbeatMs:3600000,i18n:createI18n(resolveRegion({locale:'en-CA'},{locales:['en-CA']}))});
 assert.equal(await canada.create({name:'x',displayName:'y',hostAuthority:'device'} as never),false);
 assert.equal(canada.getSnapshot().errorId,'together.error.create');
});

test('BE-MEDIA-14: with a 300 ms round trip and slow outliers, two members keep the group clock within the 750 ms band', async()=>{
 // True time runs on `real`; the server's clock is real + 5 s. Each member's own clock is off by its
 // own amount, and its requests take up/down legs that the test chooses per call.
 let real=T0;const serverOffset=5000;
 const member=(localSkew:number,legs:[number,number][])=>{
  let call=0;const calls:string[]=[];
  const api={baseUrl:'https://s',request:async<T,>(p:string):Promise<T>=>{
   calls.push(p);const [up,down]=legs[Math.min(call++,legs.length-1)]!;
   real+=up;const serverTime=new Date(real+serverOffset).toISOString();real+=down;
   return snap({},serverTime) as T;
  }};
  const stream=async()=>new Response(new ReadableStream<Uint8Array>({start(){}}),{status:200});
  const service=new GroupSessionService({api,stream,key:()=>'k',now:()=>real+localSkew,heartbeatMs:3600000});
  return {service,calls,beat:()=>(service as any).beat((service as any).room)};
 };
 // Member A: 300 ms round trip, mostly on the way up; then two congested answers (2 s on the way back).
 const a=member(-1000,[[250,50],[100,2000],[100,2500]]);
 // Member B: a quick network.
 const b=member(2000,[[10,10]]);
 await a.service.open('grp_1');await b.service.open('grp_1');
 a.beat();await settle();a.beat();await settle();b.beat();await settle();
 const truth=real+serverOffset;
 const errA=Math.abs(a.service.serverNow()-truth),errB=Math.abs(b.service.serverNow()-truth);
 assert.ok(errA<=150,`member A within half its best round trip (${errA} ms)`);
 assert.ok(errB<=10,`member B within half its round trip (${errB} ms)`);
 assert.ok(Math.abs(a.service.serverNow()-b.service.serverNow())<750,'the two members stay within the 750 ms band');
 // The old rule (serverTime − receive time of the latest answer) would be 2.5 s off for A.
 a.service.dispose();b.service.dispose();
});
