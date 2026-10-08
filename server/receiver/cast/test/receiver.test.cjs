const test=require('node:test'), assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),crypto=require('node:crypto');
const source=fs.readFileSync(require('node:path').join(__dirname,'../receiver.js'),'utf8');
async function harness(fixture={}){
 const calls=[],loads=[],interceptors={},events={},timers=new Map(),storage=new Map(),sent=[],ctxEvents={},senders=[];let timer=0,at=0,state='PAUSED',listener;
 const manager={getCurrentTimeSec:()=>at,getPlayerState:()=>state,load:async x=>{loads.push(x);state=x.autoplay?'PLAYING':'PAUSED'},pause:()=>state='PAUSED',play:()=>state='PLAYING',seek:x=>at=x,stop:()=>state='IDLE',setMessageInterceptor:(k,v)=>interceptors[k]=v,addEventListener:(k,v)=>events[k]=v,getAudioTracksManager:()=>({getTracks:()=>[],setActiveById:()=>{}}),getTextTracksManager:()=>({getTracks:()=>[],setActiveByIds:()=>{}})};
 const castContext={getDeviceCapabilities:()=>fixture.caps||{},canDisplayType:(mime,codecs,width,height,fps)=>{assert(!mime.includes(';'));return !!fixture.supports?.(mime,codecs,width,height,fps)},getPlayerManager:()=>manager,addCustomMessageListener:(ns,fn)=>listener=fn,sendCustomMessage:(ns,id,body)=>sent.push({id,body}),addEventListener:(k,v)=>ctxEvents[k]=v,getSenders:()=>senders,start:()=>{}};
 const nodes={};const document={getElementById:id=>nodes[id]??=( {textContent:'',attrs:{},setAttribute(k,v){this.attrs[k]=v},removeAttribute(k){delete this.attrs[k]},addEventListener:()=>{}} )};
 const cast={framework:{CastReceiverContext:{getInstance:()=>castContext},messages:{MessageType:Object.fromEntries(['LOAD','PLAY','PAUSE','SEEK','STOP'].map(x=>[x,x]))},events:{EventType:Object.fromEntries(['ERROR','BUFFERING','MEDIA_FINISHED','PAUSE','PLAYING','SEEKED'].map(x=>[x,x]))},system:{EventType:{SENDER_DISCONNECTED:'disconnect'},MessageType:{JSON:'json'}}}};
 let failed=false,expired=false,preparing=!!fixture.preparing,generation=1;
 const presentation=(start)=>({generation,mode:fixture.direct?'direct':'stream',url:fixture.direct?'/v1/media/grant':'/v1/media/grant/master.m3u8',container:fixture.direct?'mp4':'fmp4_hls',startPositionMs:start||0,subtitles:[],decision:fixture.transcode?{video:{action:'transcode',reasons:[]}}:{video:{action:'copy',reasons:[]}}});
 const sessionFor=(body,start)=>({id:'ps_1',revision:String(generation),kind:'vod',role:'local',state:'playing',itemId:body?.itemId||'movie',lease:{reportEveryMs:10000},presentation:presentation(start)});
 const reply=(payload,status=200)=>new Response(status===204?null:JSON.stringify(payload),{status});
 const context={document,cast,URL,URLSearchParams,TextDecoder,AbortController,Uint8Array,console,Number,Promise,setTimeout:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearTimeout:id=>timers.delete(id),setInterval:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearInterval:id=>timers.delete(id),fetch:async(url,options)=>{
  const body=options.body?JSON.parse(options.body):undefined;calls.push({url,body,method:options.method,headers:options.headers});
  const path=url.replace(/^https?:\/\/[^/]+/,'');
  if(path.startsWith('/v2/'))throw new Error('removed v2 route '+path);
  if(path.endsWith('/redeem')||path.endsWith('/reconnect'))return reply({session:{accessToken:'bearer-'+(body?.code||'renewed')},deviceToken:'device-'+(body?.code||'renewed'),scope:{accountId:String(body?.code||'').startsWith('OTHER')?'other':'account',profileId:'profile',authority:'hosted'}});
  if(path==='/v1/playback/client-profile')return reply({version:1});
  if(path==='/v1/playback/sessions'&&options.method==='POST'){
   if(fixture.ambiguous&&!failed){failed=true;throw new Error('lost response')}
   if(preparing){preparing=false;return reply({retryAfterMs:500},202);}
   return reply(sessionFor(body,body.startPositionMs),201);
  }
  if(path.endsWith('/timeline')){if(fixture.expired&&!expired){expired=true;return reply({error:{code:'unauthorized'}},401)};return reply({},204);}
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='PATCH'){
   if(options.headers['If-Match']!==String(generation))return reply({error:{code:'precondition_failed',current:sessionFor({},0)}},412);
   generation++;return reply(sessionFor({},body.seek?body.seek.positionMs:0));
  }
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='DELETE')return reply({},204);
  if(path==='/v1/playback/route-failures')return reply({escalates:true});
  throw new Error('unexpected path '+path);
 }};
 context.window={cast,crypto:crypto.webcrypto,location:{origin:'https://cast.getportico.tv'},localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v),removeItem:k=>storage.delete(k)}};
 vm.runInNewContext(source,context);await new Promise(setImmediate);
 return {receiver:context.window.porticoCastReceiver,calls,loads,interceptors,events,castContext,timers,manager,sent,ctxEvents,senders,nodes,storage,message:async(senderId,data)=>{listener({senderId,data});for(let i=0;i<20;i++)await new Promise(setImmediate);},fire:id=>{const t=timers.get(id);timers.delete(id);t.fn();},uiState:()=>nodes.ui?.attrs['data-state']};
}
for(const [name,fixture,want]of[
 ['Chromecast 3',{supports:()=>false},['h264','vp8']],
 ['Ultra',{caps:{is_hdr_supported:true},supports:(_,codec)=>codec.startsWith('hev1')||codec.startsWith('vp09')},['h264','vp8','hevc','vp9']],
 ['Google TV 4K',{caps:{is_hdr_supported:true,is_dv_supported:true},supports:()=>true},['h264','vp8','hevc','vp9','av1']]])test('probes '+name,async()=>{const h=await harness(fixture),p=h.receiver.buildProfile(h.castContext);assert.deepEqual(Array.from(p.video,x=>x.codec),want);assert.equal(p.display.height,name==='Chromecast 3'?1080:2160);assert.equal(p.audioOutput.maxChannels,name==='Google TV 4K'?6:2);});
const settle0=async()=>{for(let i=0;i<30;i++)await new Promise(setImmediate);};
const fireAll=async(h,ms)=>{for(const [id,t] of [...h.timers])if(ms===undefined||t.ms===ms){h.fire(id);await settle0();}};
test('a load starts a Playback v1 session and plays its presentation; a lost start is repeated exactly (SV-012)',async()=>{
 const h=await harness({ambiguous:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',37);
 assert.equal(h.loads.length,1);assert.equal(h.loads[0].currentTime,37);assert.equal(h.loads[0].media.contentUrl,'/v1/media/grant/master.m3u8');
 assert.equal(h.loads[0].media.contentType,'application/x-mpegurl');assert.equal(h.loads[0].media.hlsSegmentFormat,'FMP4');assert.equal(h.loads[0].media.hlsVideoSegmentFormat,'FMP4');
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts.length,2);
 assert.deepEqual(starts[0].body,starts[1].body);assert.equal(starts[0].headers['Idempotency-Key'],starts[1].headers['Idempotency-Key']);
 assert.equal(starts[0].body.itemId,'movie');assert.equal(starts[0].body.startPositionMs,37000);assert.equal(starts[0].headers.Authorization,'Bearer bearer-ABCD');
 assert(h.calls.some(c=>c.url.endsWith('/v1/playback/client-profile')&&c.method==='PUT'));
 assert.equal(h.receiver.state().playbackId,'ps_1');
});
test('a direct file gets its container’s type; a preparing session is asked again with the same key',async()=>{
 const h=await harness({direct:true,preparing:true});await h.receiver.redeem('ABCD');
 const started=h.receiver.startPlayback('movie');await settle0();
 const retry=[...h.timers].find(([,t])=>t.ms===500);assert(retry,'waits retryAfterMs');h.fire(retry[0]);await started;
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts.length,2);assert.equal(starts[0].headers['Idempotency-Key'],starts[1].headers['Idempotency-Key']);
 assert.equal(starts[0].body.startFrom,'resume');assert.equal(h.loads[0].media.contentType,'video/mp4');assert.equal(h.loads[0].media.contentUrl,'/v1/media/grant');
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
 const local=await harness();await local.receiver.redeem('ABCD');await local.receiver.startPlayback('movie',0);
 assert.deepEqual(await local.interceptors.SEEK({currentTime:120}),{currentTime:120});assert(!local.calls.some(c=>c.method==='PATCH'));
 const h=await harness({transcode:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0);
 assert.equal(await h.interceptors.SEEK({currentTime:600}),null);await settle0();
 const patch=h.calls.find(c=>c.method==='PATCH');assert(patch);assert.equal(patch.headers['If-Match'],'1');assert.equal(patch.headers['Content-Type'],'application/merge-patch+json');assert.equal(patch.body.seek.positionMs,600000);
 assert.equal(h.loads.length,2);assert.equal(h.loads[1].currentTime,600);
 await fireAll(h,0);assert.equal(h.calls.filter(c=>c.url.endsWith('/timeline')).pop().body.generation,2);
});
test('loading another title replaces the session in the same request',async()=>{
 const h=await harness();await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0);await h.receiver.startPlayback('other',0);
 const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions'));assert.equal(starts[1].body.replacesSessionId,'ps_1');assert(!h.calls.some(c=>c.method==='DELETE'));
});
test('every request the receiver makes is a current route (no Playback v2)',()=>{
 const paths=[...source.matchAll(/'(\/v\d\/[^'?]*)/g)].map(m=>m[1]);
 assert(paths.length>5);assert(paths.every(x=>x.startsWith('/v1/')),paths.join(' '));
 for(const want of ['/v1/playback/sessions','/v1/playback/client-profile','/v1/cast/reconnect','/v1/cast/redeem','/v1/playback/route-failures'])assert(paths.includes(want),want);
});

const j=x=>JSON.parse(JSON.stringify(x));
const settle=async()=>{for(let i=0;i<30;i++)await new Promise(setImmediate);};
test('probes H.264 1080p60 instead of assuming 30 fps (CAST-08)',async()=>{
 const h=await harness({supports:(_,codec)=>codec==='avc1.64002A'}),p=h.receiver.buildProfile(h.castContext);
 assert.equal(p.video[0].maxFrameRate,60);assert.equal(p.video[0].maxLevel,42);assert.equal(p.display.maxFrameRate,60);
});
test('only the sender that paired can load, and status goes only to it (SEC-14)',async()=>{
 const h=await harness();
 await h.message('stranger',{type:'load',itemId:'movie'});
 assert.equal(h.loads.length,0);assert.deepEqual(j(h.sent.pop()),{id:'stranger',body:{type:'pair-required'}});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await settle();
 assert.deepEqual(j(h.sent.find(m=>m.id==='phone').body),{type:'paired'});assert.equal(h.receiver.state().controller,'phone');
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
