import test from 'node:test';
import {castProofKey,type CastControllerContext} from '../src/cast/controller-proof.ts';
import assert from 'node:assert/strict';
import {CastController, castChoiceFields, parseCastEvent, type CastCommand, type CastTransport} from '../src/cast/index.ts';

const settle = async () => { for (let i = 0; i < 10; i++) await new Promise(setImmediate); };

/** A fake receiver: records what the sender sends and lets the test answer as the TV would. */
function receiver(deviceName = 'Living Room TV', deviceId?: string) {
  const sent: CastCommand[] = [];
  const onMsg = new Set<(raw: unknown) => void>();
  const onEnd = new Set<() => void>();
  let ended: boolean | undefined;
  let autoAck = true;
  const transport: CastTransport = {
    deviceName,
    ...(deviceId ? {deviceId} : {}),
    send: async m => { sent.push(m); if (m.type === 'load' && autoAck) queueMicrotask(() => { for (const fn of onMsg) fn({type:'loaded',requestId:m.requestId,itemId:m.itemId}); }); },
    onMessage: fn => { onMsg.add(fn); return () => onMsg.delete(fn); },
    onSessionEnd: fn => { onEnd.add(fn); return () => onEnd.delete(fn); },
    end: stop => { ended = stop; },
  };
  return {
    transport, sent,
    set autoAck(value: boolean) { autoAck = value; },
    reply: (m: unknown) => { for (const fn of [...onMsg]) fn(typeof m === 'string' ? m : JSON.stringify(m)); },
    endSession: () => { for (const fn of [...onEnd]) fn(); },
    get ended() { return ended; },
    get listeners() { return onMsg.size; },
  };
}

function harness(code: string | null = 'ABC123') {
  const requests: {path: string; method?: string; body?: unknown}[] = [];
  const timers: {fn: () => void; ms: number; id: number}[] = [];
  let next = 0;
  const controller = new CastController({
    api: {request: async <T,>(path: string, method?: string, body?: unknown) => { requests.push({path, method, body}); return {bootstrap: code ? {code} : {}} as T; }},
    origin: 'https://demo.getportico.tv/some/path',
    displayName: 'Chrome on Mac',
    setTimer: (fn, ms) => { const id = ++next; timers.push({fn, ms, id}); return id; },
    clearTimer: id => { const i = timers.findIndex(t => t.id === id); if (i >= 0) timers.splice(i, 1); },
  });
  return {controller, requests, fire: (ms: number) => { for (const t of timers.filter(x => x.ms === ms)) { timers.splice(timers.indexOf(t), 1); t.fn(); } }};
}

const meta = {title: 'Pilot', seriesTitle: 'The Show', season: 1, episode: 1, kind: 'episode', artworkPath: '/v1/artwork/x'};

test('the subtitle chosen on the sender travels with load: a track id, or off; nothing when unknown', async () => {
  assert.deepEqual(castChoiceFields({subtitleTrackId: 's3'}), {subtitleTrackId: 's3'});
  assert.deepEqual(castChoiceFields({subtitleTrackId: null}), {subtitleTrackId: 'off'});
  assert.deepEqual(castChoiceFields({}), {});
  assert.deepEqual(castChoiceFields(undefined), {});
  assert.deepEqual(castChoiceFields({subtitleTrackId: 'a b'}), {});
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', {title: 'Pilot'}, 0, {subtitleTrackId: 's3'});
  await settle();
  r.reply({type: 'paired'});
  assert.equal(await started, true);
  assert.deepEqual(r.sent[1], {type: 'load', requestId: 'load-1', itemId: 'item-1', startSeconds: 0, title: 'Pilot', subtitleTrackId: 's3'});
});

test('pairs with a server-issued code, then loads the title with its display fields', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 42);
  await settle();
  assert.deepEqual(h.requests[0], {path: '/v1/cast/bootstrap', method: 'POST', body: {protocolVersion: '1.0', displayName: 'Living Room TV'}});
  assert.deepEqual(r.sent[0], {type: 'pair', code: 'ABC123', origin: 'https://demo.getportico.tv', displayName: 'Chrome on Mac'});
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type: 'pair-pending'});
  assert.equal(h.controller.getSnapshot().phase, 'waiting', 'the TV is asking the person watching');
  r.reply({type: 'paired'});
  assert.equal(await started, true);
  assert.deepEqual(r.sent[1], {type: 'load', requestId: 'load-1', itemId: 'item-1', startSeconds: 42, title: 'Pilot', seriesTitle: 'The Show', season: 1, episode: 1, kind: 'episode', artworkPath: '/v1/artwork/x'});
  const s = h.controller.getSnapshot();
  assert.equal(s.phase, 'casting');
  assert.equal(s.deviceName, 'Living Room TV');
  assert.equal(s.positionSeconds, 42);
});

test('status updates position, pause and tracks; commands go to the TV', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'status', itemId: 'item-1', positionSeconds: 60, durationSeconds: 1800, paused: true, audioTracks: [{id: 1, language: 'en', name: 'English'}], textTracks: [{id: 3, language: 'fr', name: 'Français'}, {id: 'bad'}]});
  const s = h.controller.getSnapshot();
  assert.deepEqual([s.positionSeconds, s.durationSeconds, s.paused], [60, 1800, true]);
  assert.deepEqual(s.audioTracks, [{id: 1, language: 'en', name: 'English'}]);
  assert.deepEqual(s.textTracks.map(t => t.id), [3], 'malformed tracks are dropped');
  h.controller.toggle(); h.controller.seek(-5); h.controller.setAudioTrack(1); h.controller.setSubtitleTrack(3); h.controller.setSubtitleTrack(null);
  await settle();
  assert.deepEqual(r.sent.slice(2), [{type: 'status'}, {type: 'play'}, {type: 'seek', positionSeconds: 0}, {type: 'audio', trackId: 1}, {type: 'subtitles', trackIds: [3]}, {type: 'subtitles', trackIds: []}]);
  h.controller.stop(); await settle();
  assert.deepEqual(r.sent.at(-1), {type: 'stop'});
  assert.equal(r.ended, true, 'stop closes Portico on the TV');
  assert.equal(h.controller.getSnapshot().phase, 'off');
  assert.equal(r.listeners, 0);
});

test('a busy TV refuses with catalogue copy, never the receiver’s text', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle();
  r.reply({type: 'pair-failed', code: 'tv_busy', message: 'Sam is watching'});
  assert.equal(await started, false);
  const s = h.controller.getSnapshot();
  assert.equal(s.phase, 'error');
  assert.equal(s.errorId, 'cast.error.tvBusy');
  assert.ok(!/Sam/.test(s.error!));
  assert.equal(r.ended, false, 'a refused takeover leaves the current film playing');
});

test('no answer within the pairing window, and no code from the server, are errors', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); h.fire(35_000);
  assert.equal(await started, false);
  assert.equal(h.controller.getSnapshot().errorId, 'cast.error.noAnswer');
  const noCode = harness(null), r2 = receiver();
  assert.equal(await noCode.controller.start(r2.transport, 'item-1', meta, 0), false);
  assert.equal(noCode.controller.getSnapshot().errorId, 'cast.error.noCode');
  assert.equal(r2.sent.length, 0, 'nothing is sent to the TV without a code');
});

test('takeover: the controller is asked, answers, and the prompt expires after 30 s', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'takeover-requested', requester: '  Sam’s iPhone  '});
  assert.deepEqual(h.controller.getSnapshot().takeover, {requester: 'Sam’s iPhone', controller: undefined, requestId: undefined});
  h.controller.answerTakeover(false); await settle();
  assert.deepEqual(r.sent.at(-1), {type: 'takeover', allow: false});
  assert.equal(h.controller.getSnapshot().takeover, undefined);
  r.reply({type: 'takeover-requested'});
  assert.equal(h.controller.getSnapshot().takeover?.requester, 'Someone');
  h.fire(30_000);
  assert.equal(h.controller.getSnapshot().takeover, undefined);
});

test('replaced by another sender, or the session ending, releases the TV without stopping it', async () => {
  const h = harness(), r = receiver();
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type: 'paired'}); await started;
  r.reply({type: 'replaced'});
  assert.equal(h.controller.getSnapshot().phase, 'replaced');
  assert.equal(r.ended, false, 'the new sender keeps the TV');
  const h2 = harness(), r2 = receiver();
  const s2 = h2.controller.start(r2.transport, 'item-1', meta, 0);
  await settle(); r2.reply({type: 'paired'}); await s2;
  r2.endSession();
  assert.equal(h2.controller.getSnapshot().phase, 'off');
  assert.equal(r2.listeners, 0);
});

test('receiver messages are validated; HTTP origins are refused', () => {
  assert.equal(parseCastEvent('not json'), undefined);
  assert.equal(parseCastEvent({type: 'surprise'}), undefined);
  assert.deepEqual(parseCastEvent({type: 'pair-failed'}), {type: 'pair-failed', code: 'pair_failed'});
  assert.throws(() => new CastController({api: {request: async () => ({}) as never}, origin: 'http://192.168.1.2:32500', displayName: 'x'}));
});

const scope={serverId:'server',authority:'local',accountId:'account',profileId:'profile',senderDeviceId:'device',senderFamilyId:'family'};
const context:CastControllerContext={...scope,origin:'https://server.test',publicKey:'a'.repeat(43)};
const controllerStamp={bindingId:'cct_test',controllerGeneration:2,connectionGeneration:3};
const occurrence={sessionId:'playback-original',generation:7,itemId:'movie'};
const binding={id:'cct_test',bootstrapId:'cbs_test',receiverId:'tv-a',targetId:'cdv_target',targetFamilyId:'target-family',targetGeneration:'1',scope};
function returning(options:{context?:CastControllerContext;legacy?:boolean;validate?:()=>Promise<void>;describe?:()=>Promise<string|undefined>}={}){
 const saved=new Map([[castProofKey(context,'tv-a'),options.legacy?'a'.repeat(48):JSON.stringify({version:2,context,binding,controlToken:'a'.repeat(48)})]]);
 let current=true,first=true;const requests:string[]=[];
 const controller=new CastController({origin:context.origin,displayName:'Phone',controllerContext:options.context??context,isCurrent:()=>current,
 controllerStorage:{get:k=>saved.get(k),set:(k,v)=>{saved.set(k,v);}},describe:options.describe??(async()=> 'Film'),
 api:{request:async<T,>(path:string,_method?:string,body?:unknown)=>{requests.push(path);if(first){first=false;await options.validate?.();}return {binding,attemptId:(body as {attemptId:string}).attemptId,ticket:'b'.repeat(43)} as T;}}});
 return {controller,requests,retire:()=>{current=false;controller.dispose();}};
}
test('Root50: legacy/mismatched original scope and unstable receivers disclose no proof/status and leave the receiver alone',async()=>{
 for(const opts of [{legacy:true},{context:{...context,profileId:'other'}},{context:{...context,senderFamilyId:'new-family'}},{context:{...context,publicKey:'b'.repeat(43)}}]){
  const h=returning(opts),tv=receiver('TV','tv-a');assert.equal(await h.controller.adopt(tv.transport),false);assert.deepEqual(tv.sent,[]);assert.equal(tv.ended,undefined);assert.equal(h.requests.length,0);h.controller.dispose();
 }
 const h=returning(),tv=receiver('TV');assert.equal(await h.controller.adopt(tv.transport),false);assert.deepEqual(tv.sent,[]);h.controller.dispose();
});
test('Root50: fresh sender validation precedes proof disclosure, correlated receiver admission and content read precede publication',async()=>{
 let release!:()=>void,describe!:()=>void;
 const h=returning({validate:()=>new Promise<void>(r=>{release=r;}),describe:()=>new Promise<string>(r=>{describe=()=>r('Film');})}),tv=receiver('TV','tv-a');
 const adopted=h.controller.adopt(tv.transport);assert.equal(h.requests.length,1);assert.deepEqual(tv.sent,[]);release();await settle();
 const command=tv.sent[0];assert.equal(command.type,'adopt');if(command.type!=='adopt')throw new Error('no adoption');
 const status={type:'status',controller:controllerStamp,occurrence,itemId:'movie',positionSeconds:80,durationSeconds:600,paused:false,audioTracks:[],textTracks:[]};
 tv.reply({...status,attemptId:'other-attempt'});assert.equal(h.controller.getSnapshot().phase,'off');
 tv.reply({...status,attemptId:command.attemptId});assert.equal(h.controller.getSnapshot().itemId,undefined);describe();assert.equal(await adopted,true);
 assert.equal(h.controller.getSnapshot().title,'Film');assert.equal(h.controller.getSnapshot().acceptedSource,undefined);assert.equal(tv.sent.some(m=>m.type==='load'||m.type==='pair'),false);h.controller.dispose();
});
test('Root50: validation refusal, caller replacement and late redacted descriptions never stop or populate another caller',async()=>{
 const refused=returning({validate:async()=>{throw new Error('revoked');}}),tv=receiver('TV','tv-a');assert.equal(await refused.controller.adopt(tv.transport),false);assert.deepEqual(tv.sent,[]);assert.equal(tv.ended,undefined);
 let release!:()=>void;const h=returning({validate:()=>new Promise<void>(r=>{release=r;})}),other=receiver('TV','tv-a');const pending=h.controller.adopt(other.transport);h.retire();release();assert.equal(await pending,false);await settle();assert.deepEqual(other.sent,[]);assert.equal(other.ended,undefined);
 const hidden=returning({describe:async()=>undefined}),target=receiver('TV','tv-a'),adopting=hidden.controller.adopt(target.transport);await settle();const command=target.sent[0];if(command.type!=='adopt')throw new Error('no adoption');target.reply({type:'status',controller:controllerStamp,occurrence:{...occurrence,itemId:'hidden'},attemptId:command.attemptId,itemId:'hidden',positionSeconds:9});assert.equal(await adopting,false);assert.equal(hidden.controller.getSnapshot().itemId,undefined);assert.equal(target.ended,undefined);
});

test('load acknowledgement is correlated and a missing/rejected load never claims playback', async () => {
  const h = harness(), r = receiver(); r.autoAck = false;
  const started = h.controller.start(r.transport, 'item-1', meta, 0);
  await settle(); r.reply({type:'paired'}); await settle();
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type:'loaded',requestId:'load-other',itemId:'item-1'});
  assert.equal(h.controller.getSnapshot().phase, 'pairing');
  r.reply({type:'load-failed',requestId:'load-1'});
  assert.equal(await started, false);
  assert.equal(h.controller.getSnapshot().phase, 'error');
  const second=h.controller.start(r.transport, 'item-1', meta, 0);
  await settle();r.reply({type:'paired'});await settle();h.fire(150_000);
  assert.equal(await second,false);
});
test('a queue load carries only bounded server queue and entry identities', async () => {
  assert.deepEqual(castChoiceFields({queue:{queueId:'q-1',entryId:'entry-2'}}),{queue:{queueId:'q-1',entryId:'entry-2'}});
  assert.deepEqual(castChoiceFields({queue:{queueId:'https://evil.test',entryId:'entry-2'}}),{});
  const h=harness(),r=receiver();const started=h.controller.start(r.transport,'song',meta,0,{queue:{queueId:'q-1',entryId:'entry-2'}});
  await settle();r.reply({type:'paired'});assert.equal(await started,true);
  const load=r.sent.find(m=>m.type==='load');assert.deepEqual(load?.queue,{queueId:'q-1',entryId:'entry-2'});
});


test('returning dock uses real occurrence facts; stale commands and Stop receipts cannot cross occurrences',async()=>{
 const h=returning(),tv=receiver('TV','tv-a');const adopted=h.controller.adopt(tv.transport);await settle();
 const command=tv.sent[0];if(command.type!=='adopt')throw new Error('no adoption');
 const status={type:'status',controller:controllerStamp,occurrence,itemId:'movie',positionSeconds:80,durationSeconds:600,paused:false};
 tv.reply({...status,attemptId:command.attemptId});assert.equal(await adopted,true);
 const original=h.controller.getSnapshot().remoteIdentity;assert.ok(original);
 h.controller.stop(original);const stop=tv.sent.at(-1);assert.equal(stop?.type,'stop');if(stop?.type!=='stop')throw new Error('no Stop');
 assert.equal(h.controller.getSnapshot().controlPending,true);assert.equal(tv.ended,undefined);
 tv.reply({...status,itemId:'',occurrence:undefined});assert.equal(h.controller.getSnapshot().remoteStopped,false);
 const successor={...occurrence,sessionId:'playback-successor',generation:8};
 tv.reply({...status,occurrence:successor});await settle();
 tv.reply({type:'stopped',requestId:stop.requestId,target:original,positionSeconds:81,durationSeconds:600});
 assert.equal(h.controller.getSnapshot().remoteIdentity?.occurrence.sessionId,'playback-successor');assert.equal(h.controller.getSnapshot().remoteStopped,false);
 const count=tv.sent.length;h.controller.pause(original);h.controller.seek(40,original);h.controller.stop(original);assert.equal(tv.sent.length,count);
 h.controller.stop(h.controller.getSnapshot().remoteIdentity);const next=tv.sent.at(-1);if(next?.type!=='stop')throw new Error('no successor Stop');
 tv.reply({type:'stopped',requestId:next.requestId,target:next.target,positionSeconds:82,durationSeconds:600});
 assert.equal(h.controller.getSnapshot().remoteStopped,true);assert.equal(h.controller.getSnapshot().reported?.positionSeconds,82);assert.equal(tv.ended,undefined);h.controller.dispose();
});
test('private occurrence parser refuses malformed stamps instead of downgrading to unscoped status',()=>{
 assert.equal(parseCastEvent({type:'status',controller:controllerStamp,occurrence:{...occurrence,generation:0},itemId:'movie'}),undefined);
 assert.equal(parseCastEvent({type:'status',controller:controllerStamp,occurrence,itemId:'other'}),undefined);
 assert.equal(parseCastEvent({type:'status',controller:{...controllerStamp,connectionGeneration:NaN},itemId:'movie'}),undefined);
});


test('scoped loadACK before first status never downgrades controls or Stop to latest receiver authority',async()=>{
 const tv=receiver('TV','tv-a'),saved=new Map<string,string>();
 const controller=new CastController({origin:context.origin,displayName:'Phone',controllerContext:context,describe:async()=> 'Film',
 controllerStorage:{get:k=>saved.get(k),set:(k,v)=>{saved.set(k,v);}},
 api:{request:async<T,>(path:string,_method?:string,body?:unknown)=> (path==='/v1/cast/bootstrap'?{bootstrap:{id:binding.bootstrapId,code:'ABC123'}}:{binding,attemptId:(body as {attemptId:string}).attemptId,ticket:'b'.repeat(43)}) as T}});
 const started=controller.start(tv.transport,'movie',{title:'Film'},0);await settle();
 tv.reply({type:'paired',binding,controlToken:'a'.repeat(48),controller:controllerStamp});assert.equal(await started,true);
 assert.equal(controller.getSnapshot().phase,'casting');assert.equal(controller.getSnapshot().remoteIdentity,undefined);
 const before=tv.sent.length;
 controller.pause();controller.play();controller.seek(20);controller.setAudioTrack(1);controller.setSubtitleTrack(null);
 assert.equal(tv.sent.length,before,'held first status supplies no occurrence-scoped control authority');
 controller.stop();assert.equal(tv.sent.length,before);assert.equal(tv.ended,undefined,'no SDK end can stop a successor receiver');
 tv.reply({type:'status',controller:controllerStamp,occurrence:{sessionId:'successor',generation:9,itemId:'later'},itemId:'later',positionSeconds:3,durationSeconds:60});
 controller.stop();controller.pause();assert.equal(tv.sent.length,before);assert.equal(tv.ended,undefined);assert.equal(controller.getSnapshot().remoteIdentity,undefined);controller.dispose();
});

test('late scoped transport refusal cannot detach a reported successor occurrence',async()=>{
 const h=returning(),tv=receiver('TV','tv-a');let refuse!:(e:Error)=>void;
 const held=new Promise<void>((_resolve,reject)=>{refuse=reject;});
 const transport={...tv.transport,send:(message:CastCommand)=>message.type==='pause'?held:tv.transport.send(message)};
 const adopted=h.controller.adopt(transport);await settle();const command=tv.sent[0];if(command.type!=='adopt')throw new Error('no adoption');
 const status={type:'status',controller:controllerStamp,occurrence,itemId:'movie',positionSeconds:8,durationSeconds:60,paused:false};
 tv.reply({...status,attemptId:command.attemptId});assert.equal(await adopted,true);
 h.controller.pause(h.controller.getSnapshot().remoteIdentity);
 tv.reply({...status,occurrence:{...occurrence,sessionId:'successor',generation:8}});await settle();refuse(new Error('attachment retired'));await settle();
 assert.equal(h.controller.getSnapshot().remoteIdentity?.occurrence.sessionId,'successor');assert.equal(h.controller.getSnapshot().phase,'casting');assert.equal(tv.ended,undefined);h.controller.dispose();
});

test('renewal requires original sender validation and correlated receiver completion before controls resume',async()=>{
 const tv=receiver('TV','tv-a'), saved=new Map<string,string>();
 const next={...binding,targetFamilyId:'target-successor',targetGeneration:'2'};
 const controller=new CastController({origin:context.origin,displayName:'Phone',controllerContext:context,describe:async()=> 'Film',
  controllerStorage:{get:k=>saved.get(k),set:(k,v)=>{saved.set(k,v);}},
  api:{request:async<T,>(path:string,_method?:string,body?:unknown)=>{
   if(path==='/v1/cast/bootstrap')return {bootstrap:{id:binding.bootstrapId,code:'ABC123'}} as T;
   const request=body as {binding:typeof binding;attemptId:string};return {binding:request.binding,attemptId:request.attemptId,ticket:'b'.repeat(43)} as T;
  }}});
 const started=controller.start(tv.transport,'movie',{title:'Film'},0);await settle();
 tv.reply({type:'paired',binding,controlToken:'a'.repeat(48),controller:controllerStamp});assert.equal(await started,true);
 tv.reply({type:'status',controller:controllerStamp,occurrence,itemId:'movie',positionSeconds:2,durationSeconds:20});await settle();
 const renewedStamp={...controllerStamp,controllerGeneration:controllerStamp.controllerGeneration+1};
 tv.reply({type:'renewal-offer',attemptId:'renew-1',binding:next,previousController:controllerStamp,controller:renewedStamp});await settle();
 const ack=tv.sent.find(m=>m.type==='renewal-ack');assert.ok(ack);assert.equal(controller.getSnapshot().controlPending,true);
 const before=tv.sent.length;controller.seek(3);assert.equal(tv.sent.length,before);
 tv.reply({type:'renewal-complete',attemptId:'other',controller:controllerStamp});assert.equal(controller.getSnapshot().controlPending,true);
 tv.reply({type:'renewal-complete',attemptId:'renew-1',controller:renewedStamp});await settle();
 assert.equal(controller.getSnapshot().controlPending,false);
 assert.equal(JSON.parse(saved.get(castProofKey(context,'tv-a'))!).binding.targetFamilyId,'target-successor');
 controller.seek(3);assert.equal(tv.sent.at(-1)?.type,'seek');controller.dispose();
});

test('renewal reentrancy cannot validate or publish into a successor',async()=>{
 for(const retirement of ['pending','storage','complete'] as const){
  const tv=receiver('TV','tv-a'),saved=new Map<string,string>();let validations=0,armed=false,storageWrites=0;
  const next={...binding,targetFamilyId:'target-successor',targetGeneration:'2'},nextStamp={...controllerStamp,controllerGeneration:controllerStamp.controllerGeneration+1};
  let controller:CastController;
  controller=new CastController({origin:context.origin,displayName:'Phone',controllerContext:context,describe:async()=> 'Film',
   controllerStorage:{get:k=>saved.get(k),set:(k,v)=>{storageWrites++;if(armed&&retirement==='storage')controller.dismiss();saved.set(k,v);}},
   api:{request:async<T,>(path:string,_method?:string,body?:unknown)=>{
    if(path==='/v1/cast/bootstrap')return {bootstrap:{id:binding.bootstrapId,code:'ABC123'}} as T;
    validations++;const request=body as {binding:typeof binding;attemptId:string};return {binding:request.binding,attemptId:request.attemptId,ticket:'b'.repeat(43)} as T;
   }}});
  const started=controller.start(tv.transport,'movie',{title:'Film'},0);await settle();tv.reply({type:'paired',binding,controlToken:'a'.repeat(48),controller:controllerStamp});assert.equal(await started,true);
  tv.reply({type:'status',controller:controllerStamp,occurrence,itemId:'movie',positionSeconds:2,durationSeconds:20});await settle();
  const before=validations,baseWrites=storageWrites;armed=true;
  const off=controller.subscribe(()=>{const state=controller.getSnapshot();if(retirement==='pending'&&state.controlPending||retirement==='complete'&&!state.controlPending&&state.remoteIdentity?.controller.controllerGeneration===nextStamp.controllerGeneration)controller.dismiss();});
  tv.reply({type:'renewal-offer',attemptId:'renew-1',binding:next,previousController:controllerStamp,controller:nextStamp});await settle();
  if(retirement==='pending'){assert.equal(validations,before);assert.equal(storageWrites,baseWrites);assert(!tv.sent.some(m=>m.type==='renewal-ack'));}
  else {assert(tv.sent.some(m=>m.type==='renewal-ack'));const beforeComplete=tv.sent.length;tv.reply({type:'renewal-complete',attemptId:'renew-1',controller:nextStamp});await settle();assert.equal(controller.getSnapshot().phase,'off');assert.equal(tv.sent.length,beforeComplete);}
  off();controller.dispose();
 }
});
