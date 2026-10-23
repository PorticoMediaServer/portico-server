const test=require('node:test'), assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),crypto=require('node:crypto');
const source=fs.readFileSync(require('node:path').join(__dirname,'../receiver.js'),'utf8');
async function harness(fixture={}){
 const calls=[],loads=[],interceptors={},events={},timers=new Map(),storage=new Map(),sent=[],ctxEvents={},senders=[],queueItems=[];let timer=0,at=0,state='PAUSED',listener,urlResolver;
 const manager={setMediaUrlResolver:fn=>urlResolver=fn,getQueueManager:()=>({insertItems:items=>{for(const item of items)queueItems.push({...item,itemId:queueItems.length+1});},getItems:()=>queueItems,getCurrentItem:()=>queueItems[0],removeItems:ids=>{for(let i=queueItems.length-1;i>=0;i--)if(ids.includes(queueItems[i].itemId))queueItems.splice(i,1);}}),getCurrentTimeSec:()=>at,getPlayerState:()=>state,load:async x=>{if(fixture.loadFails)throw new Error('decode_error');loads.push(x);if(x.queueData){queueItems.length=0;for(const item of x.queueData.items)queueItems.push({...item,itemId:queueItems.length+1});}state=x.autoplay?'PLAYING':'PAUSED'},pause:()=>state='PAUSED',play:()=>state='PLAYING',seek:x=>at=x,stop:()=>state='IDLE',setMessageInterceptor:(k,v)=>interceptors[k]=v,addEventListener:(k,v)=>events[k]=v,getAudioTracksManager:()=>({getTracks:()=>[],setActiveById:()=>{}}),getTextTracksManager:()=>({getTracks:()=>[],setActiveByIds:()=>{}})};
 const castContext={getDeviceCapabilities:()=>fixture.caps||{},getDeviceInformation:()=>fixture.info||{},canDisplayType:(mime,codecs,width,height,fps)=>{assert(!mime.includes(';'));return !!fixture.supports?.(mime,codecs,width,height,fps)},getPlayerManager:()=>manager,addCustomMessageListener:(ns,fn)=>listener=fn,sendCustomMessage:(ns,id,body)=>sent.push({id,body}),addEventListener:(k,v)=>ctxEvents[k]=v,getSenders:()=>senders,start:()=>{}};
 const nodes={};const document={getElementById:id=>nodes[id]??=( {textContent:'',attrs:{},setAttribute(k,v){this.attrs[k]=v},removeAttribute(k){delete this.attrs[k]},addEventListener:()=>{}} )};
 const cast={framework:{CastReceiverContext:{getInstance:()=>castContext},messages:{MessageType:Object.fromEntries(['LOAD','PLAY','PAUSE','SEEK','STOP','PRELOAD','QUEUE_INSERT','QUEUE_LOAD','QUEUE_UPDATE','QUEUE_REMOVE','GET_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS'].map(x=>[x,x]))},events:{EventType:Object.fromEntries(['ERROR','BUFFERING','MEDIA_FINISHED','PLAYER_LOAD_COMPLETE','PAUSE','PLAYING','SEEKED'].map(x=>[x,x]))},system:{EventType:{SENDER_DISCONNECTED:'disconnect'},MessageType:{JSON:'json'}}}};
 let failed=false,expired=false,preparing=!!fixture.preparing,generation=1;
 const presentation=(start)=>({generation,mode:fixture.direct?'direct':'stream',url:fixture.direct?'/v1/media/grant':'/v1/media/grant/master.m3u8',container:fixture.container||(fixture.direct?'mp4':'fmp4_hls'),startPositionMs:start||0,subtitles:[],decision:fixture.audioOnly?{audio:{action:'copy',reasons:[]}}:fixture.transcode?{video:{action:'transcode',reasons:[]}}:{video:{action:'copy',reasons:[]}}});
 const sessionFor=(body,start)=>({id:'ps_1',revision:String(generation),kind:'vod',role:'local',state:'playing',itemId:body?.itemId||'movie',...(body?.queue?{queue:body.queue}:fixture.queue?{queue:{queueId:'q-1',entryId:'entry-1'}}:{}),lease:{reportEveryMs:10000},presentation:presentation(start)});
 const reply=(payload,status=200)=>new Response(status===204?null:JSON.stringify(payload),{status});
 const context={document,cast,navigator:fixture.mediaCapabilities?{mediaCapabilities:fixture.mediaCapabilities}:{},URL,URLSearchParams,TextDecoder,AbortController,Uint8Array,console,Number,Promise,setTimeout:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearTimeout:id=>timers.delete(id),setInterval:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearInterval:id=>timers.delete(id),fetch:async(url,options)=>{
  const body=options.body?JSON.parse(options.body):undefined;calls.push({url,body,method:options.method,headers:options.headers});
  const path=url.replace(/^https?:\/\/[^/]+/,'');
  if(path.startsWith('/v2/'))throw new Error('removed v2 route '+path);
  if(path.endsWith('/reconnect')&&fixture.revoked)return reply({error:{code:'cast_device_revoked',message:'That receiver pairing was revoked.',retryable:false}},403);
  if(path.endsWith('/redeem')||path.endsWith('/reconnect'))return reply({device:{id:'cdv_'+(body?.code||'renewed'),generation:'1'},session:{accessToken:'bearer-'+(body?.code||'renewed')},deviceToken:'device-'+(body?.code||'renewed'),scope:{accountId:String(body?.code||'').startsWith('OTHER')?'other':'account',profileId:'profile',authority:'hosted'}});
  if(fixture.queue&&path==='/v1/queues/q-1'&&options.method==='GET')return reply({id:'q-1',revision:'1',next:{available:!fixture.queueEnd,entryId:'entry-2',reason:'ready'}});
  if(fixture.queue&&path==='/v1/queues/q-1:prepare-next')return reply({token:'private-next-token',itemId:'song-2',entryId:'entry-2',expiresAt:'2099-01-01T00:00:00Z',presentation:{...presentation(0),url:fixture.noPrivateURL?'':'/v1/media/private-next'}});
  if(fixture.queue&&path==='/v1/queues/q-1:commit-next'){if(fixture.rejectCommit)return reply({error:{code:'prepared_canceled'}},409);return reply({queue:{id:'q-1',revision:'2'},session:{...sessionFor({},0),id:'ps_2',itemId:'song-2',queue:{queueId:'q-1',entryId:'entry-2'}}});}
  if(fixture.queue&&path==='/v1/queues/q-1:advance')return reply({queue:{id:'q-1',revision:'2'},session:{...sessionFor({},0),id:'ps_2',itemId:'song-2',queue:{queueId:'q-1',entryId:'entry-2'}}});
  if(path==='/v1/me/devices/current/capabilities')return reply({},204);
  if(path==='/v1/playback/sessions'&&options.method==='POST'){
   if(fixture.ambiguous&&!failed){failed=true;throw new Error('lost response')}
   if(preparing){preparing=false;return reply({retryAfterMs:500},202);}
   return reply(sessionFor(body,body.startPositionMs),201);
  }
  if(path.endsWith('/timeline')){if(fixture.timelineDown)throw new Error('network down');if(fixture.revoked)return reply({error:{code:'unauthorized'}},401);if(fixture.expired&&!expired){expired=true;return reply({error:{code:'unauthorized'}},401)};return reply({},204);}
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='PATCH'){
   if(options.headers['If-Match']!==String(generation))return reply({error:{code:'precondition_failed',current:sessionFor({},0)}},412);
   generation++;return reply(sessionFor({},body.seek?body.seek.positionMs:0));
  }
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='DELETE')return reply({},204);
  if(path.startsWith('/v1/cast/devices/')&&options.method==='DELETE')return reply({},204);
  throw new Error('unexpected path '+path);
 }};
 context.window={cast,crypto:crypto.webcrypto,location:{origin:'https://cast.getportico.tv'},localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v),removeItem:k=>storage.delete(k)}};
 vm.runInNewContext(source,context);await new Promise(setImmediate);
 return {resolved:request=>urlResolver(request),receiver:context.window.porticoCastReceiver,calls,loads,interceptors,events,castContext,timers,manager,sent,ctxEvents,senders,nodes,storage,queueItems,message:async(senderId,data)=>{listener({senderId,data});for(let i=0;i<20;i++)await new Promise(setImmediate);},fire:id=>{const t=timers.get(id);timers.delete(id);t.fn();},uiState:()=>nodes.ui?.attrs['data-state']};
}
for(const [name,fixture,want]of[
 ['Chromecast 3',{supports:()=>false},['h264','vp8']],
 ['Ultra',{caps:{is_hdr_supported:true},supports:(_,codec)=>codec.startsWith('hev1')||codec.startsWith('vp09')},['h264','vp8','hevc','vp9']],
 ['Google TV 4K',{caps:{is_hdr_supported:true,is_dv_supported:true},supports:()=>true},['h264','vp8','hevc','vp9','av1']]])test('probes '+name,async()=>{const h=await harness(fixture),p=h.receiver.buildProfile(h.castContext);assert.equal(p.version,2);assert.deepEqual(Array.from(p.engines.native.video,x=>x.codec),want);assert.deepEqual(Array.from(p.engines.native.audioPassthrough,x=>x.codec),name==='Google TV 4K'?['ac3','eac3']:[]);assert(!p.engines.native.audio.some(x=>x.codec==='ac3'));assert.equal(p.engines.native.video.find(v=>v.codec==='hevc')?.maxHeight||p.engines.native.video[0].maxHeight,name==='Chromecast 3'?1080:2160);assert.equal(p.display.height,undefined);assert.equal(p.audioOutput.maxChannels,name==='Google TV 4K'?8:2);});
const settle0=async()=>{for(let i=0;i<30;i++)await new Promise(setImmediate);};
const fireAll=async(h,ms)=>{for(const [id,t] of [...h.timers])if(ms===undefined||t.ms===ms){h.fire(id);await settle0();}};
test('a load starts a Playback v1 session and plays its presentation; a lost start is repeated exactly (SV-012)',async()=>{
 const h=await harness({ambiguous:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',37);
 assert.equal(h.loads.length,1);assert.equal(h.loads[0].currentTime,37);assert.equal(h.resolved(h.loads[0]),'/v1/media/grant/master.m3u8');
 assert.equal(h.loads[0].media.contentType,'application/x-mpegurl');assert.equal(h.loads[0].media.hlsSegmentFormat,'FMP4');assert.equal(h.loads[0].media.hlsVideoSegmentFormat,'FMP4');
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts.length,2);
 assert.deepEqual(starts[0].body,starts[1].body);assert.equal(starts[0].headers['Idempotency-Key'],starts[1].headers['Idempotency-Key']);
 assert.equal(starts[0].body.itemId,'movie');assert.equal(starts[0].body.startPositionMs,37000);assert.equal(starts[0].headers.Authorization,'Bearer bearer-ABCD');
 assert(h.calls.some(c=>c.url.endsWith('/v1/me/devices/current/capabilities')&&c.method==='PUT'&&c.body.version===2));
 assert.equal(h.receiver.state().playbackId,'ps_1');
});
test('a direct file gets its container’s type; a preparing session is asked again with the same key',async()=>{
 const h=await harness({direct:true,preparing:true});await h.receiver.redeem('ABCD');
 const started=h.receiver.startPlayback('movie');await settle0();
 const retry=[...h.timers].find(([,t])=>t.ms===500);assert(retry,'waits retryAfterMs');h.fire(retry[0]);await started;
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts.length,2);assert.equal(starts[0].headers['Idempotency-Key'],starts[1].headers['Idempotency-Key']);
 assert.equal(starts[0].body.startFrom,'resume');assert.equal(h.loads[0].media.contentType,'video/mp4');assert.equal(h.resolved(h.loads[0]),'/v1/media/grant');
});
test('the timeline reports state and renews through the device token; pause is the player’s own',async()=>{
 const h=await harness({expired:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0);
 await fireAll(h,0);
 const timeline=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions/ps_1/timeline'));assert.equal(timeline.length,2);assert.deepEqual(timeline[0].body,timeline[1].body);
 assert.equal(timeline[0].body.seq,1);assert.equal(timeline[0].body.generation,1);assert.equal(timeline[0].body.state,'playing');
 const reconnect=h.calls.find(c=>c.url.endsWith('/v1/cast/reconnect'));assert(reconnect);assert.match(reconnect.body.deviceId,/^cast-/);
 assert(h.timers.size>0&&[...h.timers.values()].some(t=>t.ms===10000),'next report in 10 s while playing');
 const pause=await h.interceptors.PAUSE({});assert(pause);assert(!h.calls.some(c=>c.method==='PATCH'));
 h.manager.pause();h.events.PAUSE();await fireAll(h,0);
 const last=h.calls.filter(c=>c.url.endsWith('/timeline')).pop();assert.equal(last.body.state,'paused');assert.equal(last.body.seq,2);
 assert([...h.timers.values()].some(t=>t.ms===30000),'next report in 30 s while paused');
 await h.interceptors.STOP({});assert.equal(h.receiver.state().playbackId,null);
 const stop=h.calls.find(c=>c.method==='DELETE');assert(stop&&stop.url.endsWith('/v1/playback/sessions/ps_1'));assert.equal(typeof stop.body.positionMs,'number');
});
test('a seek is local for copied streams and goes through the server for transcoded ones',async()=>{
 const local=await harness();await local.message('phone',{type:'pair',code:'ABCD',origin:''});await local.receiver.startPlayback('movie',0);
 assert.deepEqual(await local.interceptors.SEEK({senderId:'phone',currentTime:120}),{senderId:'phone',currentTime:120});assert(!local.calls.some(c=>c.method==='PATCH'));
 const h=await harness({transcode:true});await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.receiver.startPlayback('movie',0);
 assert.equal(await h.interceptors.SEEK({senderId:'phone',currentTime:600}),null);await settle0();
 const patch=h.calls.find(c=>c.method==='PATCH');assert(patch);assert.equal(patch.headers['If-Match'],'1');assert.equal(patch.headers['Content-Type'],'application/merge-patch+json');assert.equal(patch.body.seek.positionMs,600000);
 assert.equal(h.loads.length,2);assert.equal(h.loads[1].currentTime,600);
 await fireAll(h,0);assert.equal(h.calls.filter(c=>c.url.endsWith('/timeline')).pop().body.generation,2);
});
test('loading another title replaces the session in the same request',async()=>{
 const h=await harness();await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0);await h.receiver.startPlayback('other',0);
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts[1].body.replacesSessionId,'ps_1');assert(!h.calls.some(c=>c.method==='DELETE'));
});
test('the sender’s subtitle choice starts the TV’s session with it; a malformed one is left out',async()=>{
 const h=await harness();await h.receiver.redeem('ABCD');
 await h.receiver.startPlayback('movie',0,null,'s3');await h.receiver.startPlayback('other',0,null,'off');await h.receiver.startPlayback('third',0,null,'bad id/../');
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));
 assert.deepEqual(starts[0].body.subtitles,{trackId:'s3'});assert.deepEqual(starts[1].body.subtitles,{trackId:null});assert.equal(starts[2].body.subtitles,undefined);
});
test('every request the receiver makes is a current route (no Playback v2)',()=>{
 const paths=[...source.matchAll(/'(\/v\d\/[^'?]*)/g)].map(m=>m[1]);
 assert(paths.length>5);assert(paths.every(x=>x.startsWith('/v1/')),paths.join(' '));
 for(const want of ['/v1/playback/sessions','/v1/me/devices/current/capabilities','/v1/cast/reconnect','/v1/cast/redeem'])assert(paths.includes(want),want);
 for(const gone of ['/v1/playback/client-profile','/v1/playback/route-failures'])assert(!paths.includes(gone),gone);
});

const j=x=>JSON.parse(JSON.stringify(x));
const settle=async()=>{for(let i=0;i<30;i++)await new Promise(setImmediate);};
test('probes H.264 1080p60 instead of assuming 30 fps (CAST-08)',async()=>{
 const h=await harness({supports:(_,codec)=>codec==='avc1.64002A'}),p=h.receiver.buildProfile(h.castContext);
 assert.equal(p.engines.native.video[0].maxFrameRate,60);assert.equal(p.engines.native.video[0].maxLevel,42);assert.equal(p.display.maxFrameRate,undefined);
});
test('only the sender that paired can load, and status goes only to it (SEC-14)',async()=>{
 const h=await harness();
 await h.message('stranger',{type:'load',itemId:'movie'});
 assert.equal(h.loads.length,0);assert.deepEqual(j(h.sent.pop()),{id:'stranger',body:{type:'pair-required'}});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await settle();
 assert.equal(h.sent.find(m=>m.id==='phone').body.type,'paired');assert.match(h.sent.find(m=>m.id==='phone').body.controlToken,/^[a-f0-9]{48}$/);assert.equal(h.receiver.state().controller,'phone');
 await h.message('phone',{type:'load',itemId:'movie',title:'Jurassic Park',subtitle:'1993',kind:'movie'});await settle();
 assert.equal(h.loads.length,1);assert.equal(h.loads[0].media.metadata.title,'Jurassic Park');assert.equal(h.loads[0].media.metadata.metadataType,1);
 assert.equal(h.uiState(),'playing');
 h.sent.length=0;await h.message('stranger',{type:'status'});await h.message('stranger',{type:'seek',positionSeconds:10});await h.message('stranger',{type:'load',itemId:'other'});
 assert.equal(h.loads.length,1);assert(h.sent.filter(m=>m.id==='stranger').every(m=>m.body.type==='busy'));
 assert(!h.sent.some(m=>m.id==='stranger'&&m.body.type==='status'));
 await h.message('phone',{type:'status'});assert(h.sent.some(m=>m.id==='phone'&&m.body.type==='status'&&m.body.itemId==='movie'));
 assert(!h.calls.some(c=>c.method==='DELETE'));
 assert.equal(await h.interceptors.LOAD({senderId:'stranger',media:{customData:{itemId:'other'}}}),null);
});
test('a second person must be allowed before the TV switches; the old pairing survives a refusal (D-FEAT-10, CAST-04)',async()=>{
 const h=await harness();h.senders.push('phone','guest');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 await h.message('guest',{type:'pair',code:'OTHER1',origin:'',displayName:'Sam’s Pixel'});await settle();
 assert.deepEqual(j(h.sent.find(m=>m.id==='guest').body),{type:'pair-pending'});
 assert.deepEqual(j(h.sent.find(m=>m.id==='phone'&&m.body.type==='takeover-requested').body),{type:'takeover-requested',requester:'Sam’s Pixel'});
 assert.equal(h.nodes.takeover.hidden,false);assert.match(h.nodes['takeover-title'].textContent,/Sam’s Pixel wants to play/);
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.storage.get('portico.cast.deviceToken'),'device-ABCD');
 await h.message('phone',{type:'takeover',allow:false});await settle();
 assert.equal(h.sent.filter(m=>m.id==='guest').pop().body.code,'tv_busy');assert.equal(h.receiver.state().controller,'phone');
 assert.equal(h.storage.get('portico.cast.deviceToken'),'device-ABCD');assert.equal(h.nodes.takeover.hidden,true);
 await h.message('guest',{type:'pair',code:'OTHER2',origin:''});await settle();
 await h.message('phone',{type:'takeover',allow:true});await settle();
 assert.equal(h.receiver.state().controller,'guest');assert.equal(h.storage.get('portico.cast.deviceToken'),'device-OTHER2');
 assert(h.calls.some(c=>c.method==='DELETE'));assert(h.sent.some(m=>m.id==='phone'&&m.body.type==='replaced'));
});
test('the same account switches straight away; an absent controller hands over after the TV says so',async()=>{
 const h=await harness();h.senders.push('phone','tablet','guest');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 await h.message('tablet',{type:'pair',code:'ABCE',origin:''});await settle();
 assert.equal(h.receiver.state().controller,'tablet');
 await h.message('tablet',{type:'load',itemId:'movie'});await settle();
 await h.message('guest',{type:'pair',code:'OTHER3',origin:''});await settle();
 assert.equal(h.receiver.state().pending,'guest');
 h.ctxEvents.disconnect({senderId:'tablet'});
 const handover=[...h.timers].filter(([,t])=>t.ms===10000).pop();assert(handover,'10 s handover timer');
 h.fire(handover[0]);await settle();
 assert.equal(h.receiver.state().controller,'guest');
});
test('a failed title shows an error card on the TV without raw codes (CAST-01)',async()=>{
 const h=await harness();
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',itemId:'not a valid id!'});await settle();
 assert.equal(h.uiState(),'error');assert.doesNotMatch(h.nodes['error-title'].textContent+h.nodes['error-body'].textContent,/_|unavailable_/);
 assert.equal(h.sent.find(m=>m.body.type==='load-failed').body.code,'playback_unavailable');
});
test('the TV keeps playing when the last sender disconnects; Stop and the end of the title still end it',async()=>{
 const h=await harness();h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 assert.equal(h.receiver.state().playbackId,'ps_1');
 // The phone sleeps or leaves Wi-Fi: its Cast connection drops and nobody is connected.
 h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 assert.equal(h.receiver.state().playbackId,'ps_1','a controller going away is not Stop');
 assert.equal(h.manager.getPlayerState(),'PLAYING');assert.equal(h.uiState(),'playing');
 assert(!h.calls.some(c=>c.method==='DELETE'),'the playback session is kept');
 // The receiver goes on reporting progress by itself.
 const before=h.calls.filter(c=>c.url.endsWith('/timeline')).length;
 await fireAll(h,0);await fireAll(h,10000);
 assert(h.calls.filter(c=>c.url.endsWith('/timeline')).length>before,'progress is still reported');
 // The title finishing ends the session, so the idle timeout can close the app.
 h.events.MEDIA_FINISHED();await settle();
 assert.equal(h.receiver.state().playbackId,null);assert.equal(h.uiState(),'idle');
 assert(h.calls.some(c=>c.method==='DELETE'&&c.url.endsWith('/v1/playback/sessions/ps_1')));
});
test('with nobody connected, Stop from the remote or a sender still ends playback',async()=>{
 const h=await harness();h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 assert.equal(h.receiver.state().playbackId,'ps_1');
 await h.interceptors.STOP({});
 assert.equal(h.receiver.state().playbackId,null);assert(h.calls.some(c=>c.method==='DELETE'));
});
test('a title that ends while its last report cannot be delivered still ends on the TV',async()=>{
 const fixture={},h=await harness(fixture);h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 // Nobody is connected to press Stop, and the server cannot be told the title finished.
 fixture.timelineDown=true;h.manager.stop();h.events.MEDIA_FINISHED();await settle();
 assert.equal(h.receiver.state().playbackId,null,'the receiver does not go on holding a finished title');
 assert.equal(h.uiState(),'idle');
 assert(h.calls.some(c=>c.method==='DELETE'&&c.url.endsWith('/v1/playback/sessions/ps_1')),'the session is still closed on the server');
 assert(![...h.timers.values()].some(t=>[1000,5000,10000,30000].includes(t.ms)),'no report is left retrying');
});
test('a receiver whose pairing was revoked stops playing; one that only lost the network keeps going',async()=>{
 const fixture={},h=await harness(fixture);h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await settle();
 h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 // The network drops: nothing is known about the pairing, so the film goes on and the report is retried.
 fixture.timelineDown=true;await fireAll(h,0);await fireAll(h,10000);
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.uiState(),'playing');
 assert([...h.timers.values()].some(t=>t.ms===1000),'retrying soon');
 // The network returns, and the server says this TV's pairing is over.
 fixture.timelineDown=false;fixture.revoked=true;await fireAll(h,1000);
 assert.equal(h.receiver.state().playbackId,null,'a revoked pairing is an answer, not something to retry');
 assert.equal(h.uiState(),'idle');assert.equal(h.manager.getPlayerState(),'IDLE');
 const reports=h.calls.filter(c=>c.url.endsWith('/timeline')).length;
 await fireAll(h);await settle();
 assert.equal(h.calls.filter(c=>c.url.endsWith('/timeline')).length,reports,'and nothing goes on reporting');
});

test('a returning controller can recover the same playing session and an unpaired sender cannot load through CAF',async()=>{
 const h=await harness();h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 const proof=h.sent.find(x=>x.body.type==='paired').body.controlToken;
 await h.message('phone',{type:'load',itemId:'movie'});
 const before=h.loads.length;h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 assert.equal(await h.interceptors.LOAD({senderId:'stranger',media:{customData:{itemId:'other'}}}),null);
 assert.equal(await h.interceptors.PLAY({senderId:'stranger'}),null);
 assert.equal(await h.interceptors.SEEK({senderId:'stranger',currentTime:30}),null);
 h.sent.length=0;await h.message('new-phone-id',{type:'status',controlToken:'0'.repeat(48)});
 assert(!h.sent.some(x=>x.body.type==='status'));
 await h.message('new-phone-id',{type:'status',controlToken:proof});
 assert(h.sent.some(x=>x.id==='new-phone-id'&&x.body.type==='status'&&x.body.itemId==='movie'));
 assert.equal(h.loads.length,before);assert(!h.calls.some(x=>x.method==='DELETE'));
 await h.message('new-phone-id',{type:'pause'});assert.equal(h.manager.getPlayerState(),'PAUSED');
 await h.message('new-phone-id',{type:'play'});assert.equal(h.manager.getPlayerState(),'PLAYING');
 await h.message('new-phone-id',{type:'seek',positionSeconds:70});assert.equal(h.manager.getCurrentTimeSec(),70);
});

test('refusing a takeover revokes the unused pairing and keeps the existing film',async()=>{
 const h=await harness();h.senders.push('phone','guest');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});
 await h.message('guest',{type:'pair',code:'OTHER1',origin:''});await h.message('phone',{type:'takeover',allow:false});
 assert(h.calls.some(x=>x.url.endsWith('/v1/cast/devices/cdv_OTHER1')&&x.method==='DELETE'&&x.headers.Authorization==='Bearer bearer-OTHER1'&&x.headers['If-Match']==='1'));
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.manager.getPlayerState(),'PLAYING');
});

// SDK flags and exact format probes are separate facts; no Atmos from EC-3 alone.
test('HDR/DV/Atmos declarations require positive per-device answers',async()=>{
 const h=await harness({caps:{is_hdr_supported:true,is_dv_supported:true,is_dolby_atmos_supported:true},supports:(_,codec)=>codec.startsWith('hev1')||codec==='ec-3'||codec==='dvhe.05.07'});
 const p=h.receiver.buildProfile(h.castContext),v=p.engines.native.video.find(v=>v.codec==='hevc');
 assert.deepEqual(j(v.dolbyVisionProfiles),[5]);assert.deepEqual(j(p.display.dynamicRanges),['sdr','hdr10','dolby_vision']);
 assert.deepEqual(j(p.engines.native.audioPassthrough.find(a=>a.codec==='eac3').objectAudio),['atmos']);
 assert(!p.engines.native.audio.some(a=>a.codec==='dts'||a.codec==='truehd'));
 const no=await harness({caps:{is_dv_supported:true},supports:(_,codec)=>codec.startsWith('hev1')||codec==='ec-3'}),n=no.receiver.buildProfile(no.castContext);
 assert.deepEqual(j(n.engines.native.video.find(v=>v.codec==='hevc').dolbyVisionProfiles),[]);
 assert.deepEqual(j(n.engines.native.audioPassthrough.find(a=>a.codec==='eac3').objectAudio),[]);
 assert.deepEqual(j(n.display.dynamicRanges),['sdr']);
});
test('music formats and fMP4 transport declarations follow device probes',async()=>{
 const h=await harness({supports:(_,codec)=>codec==='fLaC'}),p=h.receiver.buildProfile(h.castContext);
 assert(!p.engines.native.audio.some(a=>a.codec==='opus'));
 assert(!p.engines.native.transports.find(t=>t.transport==='hls_fmp4').audio.includes('opus'));
 const all=await harness({supports:()=>true}),a=all.receiver.buildProfile(all.castContext);
 assert(a.engines.native.transports.find(t=>t.transport==='hls_fmp4').audio.includes('flac'));
 const speaker=await harness({caps:{display_supported:false},supports:()=>true}),s=speaker.receiver.buildProfile(speaker.castContext);
 assert.equal(s.audioOutput.route,'speaker');assert.deepEqual(j(s.engines.native.audioPassthrough),[]);
});

test('receiver queue preloads privately and only the CAF current-item boundary commits authority',async()=>{
 const h=await harness({queue:true});await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',requestId:'load-1',itemId:'song-1',queue:{queueId:'q-1',entryId:'entry-1'},contentUrl:'https://evil.test/file'});
 assert.equal(h.queueItems.length,2);assert.equal(h.queueItems[1].preloadTime,10);assert.equal(h.loads[0].queueData.startIndex,0);
 assert.deepEqual(h.calls.find(c=>c.url==='/v1/playback/sessions').body.queue,{queueId:'q-1',entryId:'entry-1'});
 assert(h.sent.some(s=>s.body.type==='loaded'&&s.body.requestId==='load-1'));
 const request={media:h.queueItems[1].media};
 assert.equal(await h.interceptors.PRELOAD({...request,senderId:'stranger'}),null);
 const preloaded=await h.interceptors.PRELOAD(request);assert(preloaded);assert.equal(h.resolved(preloaded),'/v1/media/private-next');
 assert.equal(h.receiver.state().playbackId,'ps_1');assert(!h.calls.some(c=>c.url.endsWith(':commit-next')));
 assert.equal(h.calls.filter(c=>c.url==='/v1/playback/sessions').length,1,'preload creates no separate public session');
 const committed=await h.interceptors.LOAD(preloaded);assert(committed);assert.equal(h.receiver.state().playbackId,'ps_2');
 assert.equal(h.calls.find(c=>c.url.endsWith(':commit-next')).body.token,'private-next-token');
 await fireAll(h,0);assert(h.calls.some(c=>c.url==='/v1/playback/sessions/ps_2/timeline'&&c.body.seq===1));
 assert(!JSON.stringify(h.sent).includes('private-next-token'));assert(!JSON.stringify(h.sent).includes('/v1/media/'));
});
test('unavailable private preparation leaves current authority and ordinary advance works',async()=>{
 const h=await harness({queue:true,noPrivateURL:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 assert.equal(await h.interceptors.PRELOAD({media:h.queueItems[1].media}),null);assert.equal(h.receiver.state().playbackId,'ps_1');
 h.events.MEDIA_FINISHED();await settle();assert.equal(h.receiver.state().playbackId,'ps_2');
 assert(h.calls.some(c=>c.url.endsWith(':advance')));assert(!h.calls.some(c=>c.url.endsWith(':commit-next')));
});
test('a stale next nonce and server-canceled preparation never adopt another session',async()=>{
 const h=await harness({queue:true,rejectCommit:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 const preloaded=await h.interceptors.PRELOAD({media:h.queueItems[1].media});
 assert.equal(await h.interceptors.LOAD(preloaded),null);assert.equal(h.receiver.state().playbackId,'ps_1');
 await h.interceptors.STOP({});assert.equal(await h.interceptors.LOAD(preloaded),null);
});

test('decoded multichannel and rate declarations require exact positive output configurations',async()=>{
 const h=await harness({supports:()=>true,mediaCapabilities:{decodingInfo:async q=>({supported:q.audio.channels==='6'&&q.audio.samplerate<=96000})}});
 const p=await h.receiver.probeAudioOutput(h.receiver.buildProfile(h.castContext));
 assert.equal(p.engines.native.audio.find(a=>a.codec==='flac').maxChannels,6);
 assert.equal(p.engines.native.audio.find(a=>a.codec==='flac').maxSampleRate,96000);
 const n=await harness({supports:()=>true,mediaCapabilities:{decodingInfo:async()=>{throw new Error('unavailable')}}});
 const no=await n.receiver.probeAudioOutput(n.receiver.buildProfile(n.castContext));
 assert.equal(no.engines.native.audio.find(a=>a.codec==='flac').maxChannels,2);
 assert.equal(no.engines.native.audio.find(a=>a.codec==='flac').maxSampleRate,undefined);
});

test('standard CAF queue reads and mutations cannot bypass controller authority',async()=>{
 const h=await harness();await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 for(const kind of ['GET_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS']){
  assert.equal(h.interceptors[kind]({senderId:'stranger'}),null);
  assert.deepEqual(h.interceptors[kind]({senderId:'phone'}),{senderId:'phone'});
 }
 for(const kind of ['QUEUE_INSERT','QUEUE_LOAD','QUEUE_UPDATE','QUEUE_REMOVE'])assert.equal(h.interceptors[kind]({senderId:'phone'}),null);
});

test('CAF public media status contains opaque IDs and the private URL map is bounded',async()=>{
 const h=await harness();await h.receiver.redeem('ABCD');
 for(let i=0;i<12;i++)await h.receiver.startPlayback('movie-'+i,0);
 assert(!JSON.stringify(h.loads).includes('/v1/media/'));
 assert.equal(h.resolved(h.loads[0]),'','old private address evicted');
 assert.equal(h.resolved(h.loads.at(-1)),'/v1/media/grant/master.m3u8');
 await h.interceptors.STOP({});assert.equal(h.resolved(h.loads.at(-1)),'');
});

test('audio-only ISO-BMFF demuxer identity has an audio MIME type and unknown containers fail closed',async()=>{
 const h=await harness({direct:true,audioOnly:true,container:'mov'});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0);
 assert.equal(h.loads[0].media.contentType,'audio/mp4');
 const bad=await harness({direct:true,container:'unrecognized'});await bad.receiver.redeem('ABCD');
 await assert.rejects(()=>bad.receiver.startPlayback('movie',0),/unsupported_media_container/);assert.equal(bad.loads.length,0);assert(bad.calls.some(c=>c.method==='DELETE'&&c.url==='/v1/playback/sessions/ps_1'));
});

test('a CAF load rejection ends its owned server session and leaves an honest error card',async()=>{
 const h=await harness({loadFails:true});await h.receiver.redeem('ABCD');await assert.rejects(()=>h.receiver.startPlayback('movie',0),/decode_error/);
 assert.equal(h.receiver.state().playbackId,null);assert.equal(h.uiState(),'error');assert(h.calls.some(c=>c.method==='DELETE'&&c.url==='/v1/playback/sessions/ps_1'));
});

test('device identity comes only from the SDK and decoder limits do not invent display geometry',async()=>{
 const known=await harness({info:{'model-name':'Google TV Streamer'},supports:()=>true}),p=known.receiver.buildProfile(known.castContext);
 assert.equal(p.client.model,'Google TV Streamer');assert.equal(p.display.width,undefined);assert.equal(p.display.height,undefined);
 const unknown=await harness({info:{'model-name':'unknown'},supports:()=>true}),u=unknown.receiver.buildProfile(unknown.castContext);assert.equal(u.client.model,undefined);
});
