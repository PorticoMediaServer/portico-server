const test=require('node:test'), assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs'),crypto=require('node:crypto');
const source=fs.readFileSync(require('node:path').join(__dirname,'../receiver.js'),'utf8');
async function harness(fixture={}){
 const calls=[],loads=[],metadataUpdates=[],interceptors={},events={},timers=new Map(),storage=new Map(),sent=[],ctxEvents={},senders=[],queueItems=[];let timer=0,at=0,state='PAUSED',listener,urlResolver;
 // The real SDK exposes QUEUE_NEXT/PREV enum values but rejects them as interceptors.
 const supportedInterceptors=new Set(['LOAD','PLAY','PAUSE','SEEK','STOP','PRELOAD','QUEUE_INSERT','QUEUE_LOAD','QUEUE_UPDATE','QUEUE_REMOVE','QUEUE_REORDER','GET_STATUS','MEDIA_STATUS','CLOUD_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS']);
 const manager={setMediaUrlResolver:fn=>urlResolver=fn,getMediaInformation:()=>loads.at(-1)?.media,setMediaInformation:(media,broadcast)=>metadataUpdates.push({media,broadcast}),getQueueManager:()=>({insertItems:items=>{for(const item of items)queueItems.push({...item,itemId:queueItems.length+1});},getItems:()=>queueItems,getCurrentItem:()=>queueItems[0],removeItems:ids=>{for(let i=queueItems.length-1;i>=0;i--)if(ids.includes(queueItems[i].itemId))queueItems.splice(i,1);}}),getCurrentTimeSec:()=>at,getPlayerState:()=>state,load:async x=>{if(fixture.loadFails||(fixture.nextLoadFails&&loads.length))throw new Error('decode_error');if(fixture.localInterceptors&&interceptors.LOAD){x=await interceptors.LOAD(x);if(!x)return;}loads.push(x);document.getElementById('player').ended=false;if(x.queueData){queueItems.length=0;for(const item of x.queueData.items)queueItems.push({...item,itemId:queueItems.length+1});}state=x.autoplay?'PLAYING':'PAUSED'},sendLocalMediaRequest:x=>{assert(Number.isSafeInteger(x.requestId)&&x.requestId>0);const handle=()=>{if(x.type==='PLAY')state='PLAYING';if(x.type==='PAUSE')state='PAUSED';if(x.type==='STOP')state='IDLE';if(x.type==='SEEK')at=x.currentTime;};if(fixture.localInterceptors&&interceptors[x.type])Promise.resolve(interceptors[x.type](x)).then(y=>{if(y)handle();});else handle();},pause:()=>state='PAUSED',play:()=>state='PLAYING',seek:x=>at=x,stop:()=>state='IDLE',setMessageInterceptor:(k,v)=>{assert(supportedInterceptors.has(k),'unsupported CAF interceptor '+k);interceptors[k]=v;},addEventListener:(k,v)=>events[k]=v,getAudioTracksManager:()=>({getTracks:()=>[],setActiveById:()=>{}}),getTextTracksManager:()=>({getTracks:()=>[],setActiveByIds:()=>{}})};
 const castContext={getDeviceCapabilities:()=>fixture.caps||{},getDeviceInformation:()=>fixture.info||{},canDisplayType:(mime,codecs,width,height,fps)=>{assert(!mime.includes(';'));return !!fixture.supports?.(mime,codecs,width,height,fps)},getPlayerManager:()=>manager,addCustomMessageListener:(ns,fn)=>listener=fn,sendCustomMessage:(ns,id,body)=>sent.push({id,body}),addEventListener:(k,v)=>ctxEvents[k]=v,getSenders:()=>senders,getSender:id=>({id,userAgent:fixture.senderUA||'Browser'}),start:()=>{}};
 const nodes={};const document={getElementById:id=>nodes[id]??=( {textContent:'',attrs:{},setAttribute(k,v){this.attrs[k]=v},removeAttribute(k){delete this.attrs[k]},addEventListener:()=>{}} )};
 const cast={framework:{CastReceiverContext:{getInstance:()=>castContext},messages:{MessageType:Object.fromEntries(['LOAD','PLAY','PAUSE','SEEK','STOP','PRELOAD','QUEUE_INSERT','QUEUE_LOAD','QUEUE_UPDATE','QUEUE_REMOVE','QUEUE_REORDER','QUEUE_NEXT','QUEUE_PREV','GET_STATUS','MEDIA_STATUS','CLOUD_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS'].map(x=>[x,x]))},events:{EndedReason:{END_OF_STREAM:'END_OF_STREAM'},EventType:Object.fromEntries(['ERROR','BUFFERING','MEDIA_FINISHED','PLAYER_LOAD_COMPLETE','PLAYER_LOADING','LOADED_DATA','TIME_UPDATE','DURATION_CHANGE','MEDIA_STATUS','PAUSE','PLAYING','SEEKED'].map(x=>[x,x]))},system:{EventType:{SENDER_CONNECTED:'connect',SENDER_DISCONNECTED:'disconnect',SHUTDOWN:'shutdown'},MessageType:{JSON:'json'}}}};
 let failed=false,expired=false,preparing=!!fixture.preparing,generation=1,registeredBinding,controllerExpired=false;
 const presentation=(start,mediaUrl)=>{
  const url=mediaUrl===undefined?(fixture.direct?'/v1/media/grant':'/v1/media/grant/master.m3u8'):mediaUrl;
  return {generation,mode:fixture.direct?'direct':'stream',url,container:fixture.container||(fixture.direct?'mp4':'fmp4_hls'),startPositionMs:start||0,subtitles:[],decision:fixture.audioOnly?{audio:{action:'copy',reasons:[]}}:fixture.transcode?{video:{action:'transcode',reasons:[]}}:{video:{action:'copy',reasons:[]}},...(fixture.audioRender?{audioRender:{version:2,id:'render-1',url,...fixture.audioRender}}:{})};
 };
 const sessionFor=(body,start)=>({id:fixture.nextSessionId||'ps_1',revision:String(generation),kind:'vod',role:'local',state:'playing',itemId:body?.itemId||'movie',...(body?.queue?{queue:body.queue}:fixture.queue?{queue:{queueId:'q-1',entryId:'entry-1'}}:{}),lease:{reportEveryMs:10000},presentation:presentation(start)});
 const reply=(payload,status=200)=>new Response(status===204?null:JSON.stringify(payload),{status});
 const context={document,cast,navigator:fixture.mediaCapabilities?{mediaCapabilities:fixture.mediaCapabilities}:{},URL,URLSearchParams,TextDecoder,AbortController,Uint8Array,console,Number,Promise,setTimeout:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearTimeout:id=>timers.delete(id),setInterval:(fn,ms)=>{timers.set(++timer,{fn,ms});return timer},clearInterval:id=>timers.delete(id),fetch:async(url,options)=>{
  const body=options.body?JSON.parse(options.body):undefined;calls.push({url,body,method:options.method,headers:options.headers});
  const path=url.replace(/^https?:\/\/[^/]+/,'');
  if(path.startsWith('/v2/'))throw new Error('removed v2 route '+path);
  if(path.endsWith('/reconnect')&&fixture.revoked)return reply({error:{code:'cast_device_revoked',message:'That receiver pairing was revoked.',retryable:false}},403);
  if(path.endsWith('/preview'))return reply({controllerBootstrapId:'cbs_'+body.code,expiresAt:'2099-01-01T00:00:00Z',scope:{accountId:String(body.code||'').startsWith('OTHER')?'other':'account',profileId:'profile',authority:'hosted'}});
  if(path.endsWith('/redeem')&&fixture.redeemWait){fixture.redeemCalled?.();await fixture.redeemWait;}
  if(path.endsWith('/redeem')||path.endsWith('/reconnect'))return reply({controllerBootstrapId:'cbs_'+(body?.code||'renewed'),device:{id:'cdv_'+(body?.code||'renewed'),generation:'1'},session:{accessToken:'bearer-'+(body?.code||'renewed')},deviceToken:'device-'+(body?.code||'renewed'),scope:{accountId:String(body?.code||'').startsWith('OTHER')?'other':'account',profileId:'profile',authority:'hosted'}});
  if(path==='/v1/cast/controllers/register'){registeredBinding={id:'cct_current',bootstrapId:body.bootstrapId,receiverId:'sdk-tv',targetId:'cdv_target',targetFamilyId:'target-family',targetGeneration:'1',scope:{serverId:'server',authority:'local',accountId:'account',profileId:'profile',senderDeviceId:'phone',senderFamilyId:'source-family'}};return reply({binding:registeredBinding});}
  if(path==='/v1/cast/controllers/check'){
    if(fixture.firstController401&&!controllerExpired){controllerExpired=true;return reply({error:{code:'unauthorized'}},401);}
    if(fixture.controllerRefuse?.(body))return reply({error:{code:'forbidden'}},403);
    if(fixture.controllerCheck)await fixture.controllerCheck(body);
    if(fixture.renewal && options.headers.Authorization==='Bearer bearer-renewed')return reply({binding:{...registeredBinding,targetFamilyId:'target-successor',targetGeneration:'2'}});
    return reply({binding:{id:body.bindingId}});
  }
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='GET')return reply(sessionFor({},0));
  if(fixture.queue&&path==='/v1/queues/q-1'&&options.method==='GET')return reply({id:'q-1',revision:'1',next:{available:!fixture.queueEnd,entryId:'entry-2',reason:'ready'}});
  if(fixture.queue&&path==='/v1/queues/q-1:advance'){
   if(fixture.rejectAdvance)return reply({error:{code:'queue_changed'}},409);
   return reply({queue:{id:'q-1',revision:'2'},session:{...sessionFor({},0),id:'ps_2',itemId:'song-2',queue:{queueId:'q-1',entryId:'entry-2'}}});
  }
  if(path==='/v1/artwork/fixture'&&fixture.artwork)return new Response('<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>',{headers:{'Content-Type':'image/svg+xml'}});
  if(path.startsWith('/v1/items/')){if(fixture.catalogWait)await fixture.catalogWait;const item=fixture.catalog?.[decodeURIComponent(path.slice('/v1/items/'.length))];return item?reply(item):reply({error:{code:'not_found'}},fixture.catalogStatus||404);}
  if(path==='/v1/me/devices/current/capabilities')return reply({},204);
  if(path==='/v1/playback/sessions'&&options.method==='POST'){
   if(fixture.startWait){fixture.startCalled?.();await fixture.startWait;}
   if(fixture.ambiguous&&!failed){failed=true;throw new Error('lost response')}
   if(fixture.preparingReal){fixture.preparingReal=false;const value=sessionFor(body,body.startPositionMs);return reply({...value,state:'preparing',retryAfterMs:500,lease:{...value.lease,expiresAt:'2099-01-01T00:00:00Z'},presentation:{...value.presentation,engine:'native'}},202);}
   if(fixture.preparingResponses?.length){const next=fixture.preparingResponses.shift();return reply(typeof next==='function'?next(sessionFor(body,body.startPositionMs)):next,202);}
   if(fixture.prepareTerminal)return reply({error:{code:'preparation_failed'}},409);
   if(preparing){preparing=false;return reply({retryAfterMs:500},202);}
   const accepted=reply(sessionFor(body,body.startPositionMs),201);
   if(fixture.retireAfterAccepted){const stream=accepted.body;Object.defineProperty(accepted,'body',{value:{getReader(){const reader=stream.getReader();return {read:()=>reader.read(),releaseLock(){reader.releaseLock();queueMicrotask(()=>queueMicrotask(()=>fixture.retireAfterAccepted()));}}}}});}
   return accepted;
  }
  if(path.endsWith('/timeline')){if(fixture.timelineDown)throw new Error('network down');if(fixture.revoked)return reply({error:{code:'unauthorized'}},401);if(fixture.expired&&!expired){expired=true;return reply({error:{code:'unauthorized'}},401)};return reply({},204);}
  if(path==='/v1/playback/sessions/ps_1'&&options.method==='PATCH'){
   if(options.headers['If-Match']!==String(generation))return reply({error:{code:'precondition_failed',current:sessionFor({},0)}},412);
   generation++;return reply(sessionFor({},body.seek?body.seek.positionMs:0));
  }
  if((path==='/v1/playback/sessions/ps_1'||path==='/v1/playback/sessions/ps_known')&&options.method==='DELETE'){if(fixture.stopWait){fixture.stopCalled?.();await fixture.stopWait;}return reply({},204);}
  if(path.startsWith('/v1/cast/devices/')&&options.method==='DELETE')return reply({},204);
  throw new Error('unexpected path '+path);
 }};
 if(fixture.artwork)context.FileReader=class{readAsDataURL(blob){blob.arrayBuffer().then(data=>{this.result='data:'+blob.type+';base64,'+Buffer.from(data).toString('base64');this.onload();});}};
 if(fixture.clock)context.Date=class extends Date{static now(){return fixture.clock.now}};
 context.window={cast,porticoCastView:fixture.view,porticoCastText:fixture.text,addEventListener:(name,fn)=>{ctxEvents['window:'+name]=fn},crypto:crypto.webcrypto,location:{origin:'https://cast.getportico.tv'},localStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v),removeItem:k=>storage.delete(k)}};
 if(!fixture.text)vm.runInNewContext(fs.readFileSync(require('node:path').join(__dirname,'..','receiver-text.js'),'utf8'),context);
 vm.runInNewContext(source,context);await new Promise(setImmediate);
 return {finish:()=>{document.getElementById('player').ended=true;state='IDLE';events.MEDIA_FINISHED({endedReason:'END_OF_STREAM'});},metadataUpdates,resolved:request=>urlResolver(request),receiver:context.window.porticoCastReceiver,calls,loads,interceptors,events,castContext,timers,manager,sent,ctxEvents,senders,nodes,storage,queueItems,message:async(senderId,data)=>{listener({senderId,data});for(let i=0;i<20;i++)await new Promise(setImmediate);},fire:id=>{const t=timers.get(id);timers.delete(id);t.fn();},uiState:()=>nodes.ui?.attrs['data-state']};
}
for(const [name,fixture,want]of[
 ['Chromecast 3',{supports:()=>false},['h264','vp8']],
 ['Ultra',{caps:{is_hdr_supported:true},supports:(_,codec)=>codec.startsWith('hev1')||codec.startsWith('vp09')},['h264','vp8','hevc','vp9']],
 ['Google TV 4K',{caps:{is_hdr_supported:true,is_dv_supported:true},supports:()=>true},['h264','vp8','hevc','vp9','av1']]])test('probes '+name,async()=>{const h=await harness(fixture),p=h.receiver.buildProfile(h.castContext);assert.equal(p.version,2);assert.deepEqual(Array.from(p.engines.native.video,x=>x.codec),want);assert.deepEqual(Array.from(p.engines.native.audioPassthrough,x=>x.codec),name==='Google TV 4K'?['ac3','eac3']:[]);assert(!p.engines.native.audio.some(x=>x.codec==='ac3'));assert.equal(p.engines.native.video.find(v=>v.codec==='hevc')?.maxHeight||p.engines.native.video[0].maxHeight,name==='Chromecast 3'?1080:2160);assert.equal(p.display.height,undefined);assert.equal(p.audioOutput.maxChannels,0,'decoder answers do not measure the active output sink');});
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
 const requested=h.sent.find(m=>m.id==='phone'&&m.body.type==='takeover-requested').body;assert.deepEqual(j(requested),{type:'takeover-requested',requester:'Sam’s Pixel',controller:{bindingId:'cct_current',controllerGeneration:1,connectionGeneration:1},requestId:'cbs_OTHER1'});
 assert.equal(h.nodes.takeover.hidden,false);assert.match(h.nodes['takeover-title'].textContent,/Sam’s Pixel wants to play/);
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.storage.get('portico.cast.deviceToken'),'device-ABCD');
 await h.message('phone',{type:'takeover',allow:false,controller:requested.controller,requestId:requested.requestId});await settle();
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
 h.finish();await settle();
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
 fixture.timelineDown=true;h.manager.stop();h.finish();await settle();
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
 const accepted=h.sent.find(x=>x.body.type==='paired').body,proof=accepted.controlToken;
 await h.message('phone',{type:'load',itemId:'movie'});
 const before=h.loads.length;h.senders.length=0;h.ctxEvents.disconnect({senderId:'phone'});await settle();
 assert.equal(await h.interceptors.LOAD({senderId:'stranger',media:{customData:{itemId:'other'}}}),null);
 assert.equal(await h.interceptors.PLAY({senderId:'stranger'}),null);
 assert.equal(await h.interceptors.SEEK({senderId:'stranger',currentTime:30}),null);
 h.sent.length=0;await h.message('new-phone-id',{type:'status',controlToken:'0'.repeat(48)});
 assert(!h.sent.some(x=>x.body.type==='status'));
 await h.message('new-phone-id',{type:'adopt',bindingId:accepted.binding.id,controlToken:proof,attemptId:'returning',ticket:'a'.repeat(43)});
 assert(h.sent.some(x=>x.id==='new-phone-id'&&x.body.type==='status'&&x.body.itemId==='movie'));
 assert.equal(h.loads.length,before);assert(!h.calls.some(x=>x.method==='DELETE'));
 await h.message('new-phone-id',{type:'pause'});assert.equal(h.manager.getPlayerState(),'PAUSED');
 await h.message('new-phone-id',{type:'play'});assert.equal(h.manager.getPlayerState(),'PLAYING');
 await h.message('new-phone-id',{type:'seek',positionSeconds:70});assert.equal(h.manager.getCurrentTimeSec(),70);
});

test('refusing a takeover leaves the original controller and target untouched',async()=>{
 const h=await harness();h.senders.push('phone','guest');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});
 await h.message('guest',{type:'pair',code:'OTHER1',origin:''});await h.message('phone',{type:'takeover',allow:false});
 assert(h.calls.some(x=>x.url.endsWith('/v1/cast/preview')&&x.body.code==='OTHER1'));
 assert(!h.calls.some(x=>x.url.endsWith('/v1/cast/redeem')&&x.body.code==='OTHER1'));
 assert(!h.calls.some(x=>x.url.endsWith('/v1/cast/devices/cdv_OTHER1')&&x.method==='DELETE'));
 await h.message('phone',{type:'status'});assert(h.sent.some(x=>x.id==='phone'&&x.body.type==='status'));
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.manager.getPlayerState(),'PLAYING');
});

test('expired, superseded and disconnected offers never redeem a target',async()=>{
 for(const action of ['expire','supersede','disconnect']){
  const h=await harness();h.senders.push('phone','guest','guest2');
  await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});
  await h.message('guest',{type:'pair',code:'OTHER1',origin:''});
  if(action==='expire'){const timer=[...h.timers].find(([,v])=>v.ms===30000);assert(timer);h.fire(timer[0]);await settle0();}
  if(action==='supersede')await h.message('guest2',{type:'pair',code:'OTHER2',origin:''});
  if(action==='disconnect')h.ctxEvents.disconnect({senderId:'guest'});
  assert(!h.calls.some(x=>x.url.endsWith('/v1/cast/redeem')&&x.body.code==='OTHER1'));
  assert(!h.calls.some(x=>x.url.endsWith('/v1/cast/devices/cdv_OTHER1')&&x.method==='DELETE'));
  assert.equal(h.receiver.state().playbackId,'ps_1');
 }
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
 assert.equal(s.audioOutput.route,undefined);assert.deepEqual(j(s.engines.native.audioPassthrough),[]);
});

test('progressive receiver queues only actual current media and never prepares on PRELOAD',async()=>{
 const h=await harness({queue:true,direct:true,audioOnly:true,container:'mp3'});await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',requestId:'load-1',itemId:'song-1',queue:{queueId:'q-1',entryId:'entry-1'},contentUrl:'https://evil.test/file'});
 assert.equal(h.queueItems.length,1);assert.equal(h.queueItems[0].media.contentType,'audio/mpeg');assert.equal(h.queueItems[0].preloadTime,10);
 assert.deepEqual(h.calls.find(c=>c.url==='/v1/playback/sessions').body.queue,{queueId:'q-1',entryId:'entry-1'});
 assert(h.sent.some(s=>s.body.type==='loaded'&&s.body.requestId==='load-1'));
 const before=h.calls.length;
 for(const request of [{media:h.queueItems[0].media},{senderId:'phone',media:h.queueItems[0].media},{senderId:'stranger'}])assert.equal(h.interceptors.PRELOAD(request),null);
 assert.equal(h.calls.length,before);assert.equal(h.receiver.state().playbackId,'ps_1');
 assert(!source.includes(':prepare-next'));assert(!source.includes(':commit-next'));
});
test('real progressive completion adopts only the server ordinary advance result',async()=>{
 const views=[];
 const h=await harness({queue:true,direct:true,audioOnly:true,container:'flac',view:{playing:s=>views.push(s)}});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,{title:'Old song',kind:'song'},null,{queueId:'q-1',entryId:'entry-1'});
 h.events.LOADED_DATA();assert.equal(views.at(-1).meta.title,'Old song');const before=views.length;
 h.finish();await settle();
 h.events.TIME_UPDATE();assert.equal(views.length,before,'clock ticks cannot reuse a previous first-picture state');
 h.events.LOADED_DATA();assert.equal(views.at(-1).meta,null,'new unknown identity is explicit before any clock snapshot');assert.equal(views.at(-1).artworkUrl,undefined);
 assert.equal(h.receiver.state().playbackId,'ps_2');assert.equal(h.loads.length,2);assert.equal(h.loads[1].media.contentType,'audio/flac');assert.equal(h.loads[1].media.metadata,undefined,'the next item never inherits the old identity');
 const advance=h.calls.find(c=>c.url.endsWith(':advance'));assert(advance);assert.equal(advance.headers['If-Match'],'1');assert.deepEqual(advance.body,{reason:'completion'});assert(advance.headers['Idempotency-Key']);
 assert(!h.calls.some(c=>c.url.endsWith(':prepare-next')||c.url.endsWith(':commit-next')));
 assert(!h.calls.some(c=>c.url.endsWith('/timeline')&&c.body.state==='ended'),'the transition transaction owns the end of the old occurrence');
 await fireAll(h,0);assert(h.calls.some(c=>c.url==='/v1/playback/sessions/ps_2/timeline'&&c.body.seq===1));
 assert(!JSON.stringify(h.sent).includes('/v1/media/'));
});
test('progressive completion has no boundary wait and duplicate events cannot advance twice',async()=>{
 const h=await harness({queue:true,direct:true,audioOnly:true,container:'mp3'});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 h.finish();h.finish();await settle();
 assert.equal(h.calls.filter(c=>c.url.endsWith(':advance')).length,1);assert.equal(h.loads.length,2);assert.equal(h.receiver.state().playbackId,'ps_2');
 assert(![...h.timers.values()].some(t=>t.ms===5000),'no progressive boundary timeout');
});
test('end of the actual server queue ends the occurrence without preparing another slot',async()=>{
 const h=await harness({queue:true,queueEnd:true,direct:true,audioOnly:true,container:'mp3'});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 h.finish();await settle();
 assert.equal(h.receiver.state().playbackId,null);assert(h.calls.some(c=>c.url.endsWith('/timeline')&&c.body.state==='ended'));
 assert(h.calls.some(c=>c.url==='/v1/playback/sessions/ps_1'&&c.method==='DELETE'));assert(!h.calls.some(c=>c.url.endsWith(':advance')));
});
test('non-finished events and a rejected ordinary advance cannot fabricate a next session',async()=>{
 const h=await harness({queue:true,rejectAdvance:true,direct:true,audioOnly:true,container:'mp3'});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 h.events.MEDIA_FINISHED({endedReason:'INTERRUPTED'});await settle();assert.equal(h.receiver.state().playbackId,'ps_1');assert(!h.calls.some(c=>c.url.endsWith(':advance')));
 h.finish();await settle();assert.equal(h.receiver.state().playbackId,null);assert.equal(h.loads.length,1);
});

test('a failed next media load ends the newly adopted server occurrence',async()=>{
 const h=await harness({queue:true,nextLoadFails:true,direct:true,audioOnly:true,container:'mp3'});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 h.finish();await settle();
 assert.equal(h.receiver.state().playbackId,null);assert.equal(h.uiState(),'error');
 assert(h.calls.some(c=>c.url==='/v1/playback/sessions/ps_2'&&c.method==='DELETE'),'cleanup follows the adopted server authority');
});

test('next display reads genuine authorized catalogue identity and exact structured artist/album',async()=>{
 const views=[],h=await harness({queue:true,direct:true,audioOnly:true,container:'mp3',catalog:{'song-2':{id:'song-2',title:'New song',kind:'song',song:{artist:'  Actual artist  ',albumTitle:'Actual album'}}},view:{playing:s=>views.push(s)}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,{title:'Old song',kind:'song',subtitle:'Never parse this as artist'},null,{queueId:'q-1',entryId:'entry-1'});
 h.events.LOADED_DATA();h.finish();await settle();h.events.LOADED_DATA();
 assert.equal(views.at(-1).meta.title,'New song');assert.equal(views.at(-1).meta.artist,'Actual artist');assert.equal(views.at(-1).meta.albumTitle,'Actual album');
 const read=h.calls.find(c=>c.url==='/v1/items/song-2');assert.equal(read.method,'GET');assert.equal(read.headers.Authorization,'Bearer bearer-ABCD');
 const update=h.metadataUpdates.at(-1);assert.equal(update.media.metadata.artist,'Actual artist');assert.equal(update.media.metadata.albumName,'Actual album');assert.equal(update.broadcast,false);assert(!JSON.stringify(update).includes('/v1/media/'));
});
test('catalogue loading identity precedes cover and CAF receives that same genuine image',async()=>{
 const methods=[],h=await harness({direct:true,audioOnly:true,container:'mp3',artwork:true,catalog:{song:{id:'song',title:'Read song',kind:'song',posterUrl:'/v1/artwork/fixture',song:{artist:'Read artist',albumTitle:'Read album'}}},view:{loading:()=>methods.push('loading'),artwork:()=>methods.push('artwork')}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song',0,{title:'Before read',kind:'song'});await settle();
 assert.equal(methods.at(-1),'artwork','a later loading publication cannot clear the fetched cover');
 assert(methods.lastIndexOf('loading')<methods.lastIndexOf('artwork'));
 const metadata=h.metadataUpdates.at(-1).media.metadata;assert.equal(metadata.title,'Read song');assert.equal(metadata.artist,'Read artist');assert.match(metadata.images[0].url,/^data:image\/svg\+xml;base64,/);
 assert(h.calls.some(c=>c.url==='/v1/artwork/fixture'&&c.headers.Authorization==='Bearer bearer-ABCD'));
 h.events.LOADED_DATA();assert.equal(h.metadataUpdates.at(-1).media.metadata.images[0].url,metadata.images[0].url);
});

test('catalogue identity without a new poster restores the current occurrence cover after loading',async()=>{
 const methods=[],h=await harness({direct:true,audioOnly:true,container:'mp3',artwork:true,catalog:{song:{id:'song',title:'Read song',kind:'song',song:{artist:'Read artist'}}},view:{loading:()=>methods.push('loading'),artwork:()=>methods.push('artwork')}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song',0,{title:'Sender song',kind:'song',artworkPath:'/v1/artwork/fixture'});await settle();
 assert.deepEqual(methods.slice(-2),['loading','artwork']);
 const original=h.loads[0].media.metadata.images[0].url;assert.match(original,/^data:image\/svg\+xml;base64,/);
 assert.equal(h.metadataUpdates.at(-1).media.metadata.images[0].url,original,'same occurrence retains only its actually resolved cover');
 assert.equal(h.metadataUpdates.at(-1).media.metadata.artist,'Read artist');
});

test('a song without its own cover draws the album cover, and an audiobook draws the book cover with its author',async()=>{
 const h=await harness({direct:true,audioOnly:true,container:'mp3',artwork:true,catalog:{
  song:{id:'song',title:'DNA.',kind:'song',song:{artist:'Kendrick Lamar',albumTitle:'DAMN.',albumId:'album-1'}},
  'album-1':{id:'album-1',title:'DAMN.',kind:'album',posterUrl:'/v1/artwork/fixture'},
  chapter:{id:'chapter',title:'Chapter 15',kind:'chapter',author:'  Andy Weir  ',bookFile:{bookId:'book-1',bookTitle:'Project Hail Mary'}},
  'book-1':{id:'book-1',title:'Project Hail Mary',kind:'book',posterUrl:'/v1/artwork/fixture'},
  other:{id:'other',title:'A song, not an album',kind:'song',posterUrl:'/v1/artwork/fixture'}
 }});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song',0,{title:'DNA.',kind:'song'});await settle();
 assert(h.calls.some(c=>c.url==='/v1/items/album-1'&&c.method==='GET'&&c.headers.Authorization==='Bearer bearer-ABCD'),'the album is read with this TV’s own credential');
 const song=h.metadataUpdates.at(-1).media.metadata;
 assert.equal(song.artist,'Kendrick Lamar');assert.equal(song.albumName,'DAMN.');assert.match(song.images[0].url,/^data:image\/svg\+xml;base64,/,'the album cover becomes the square artwork');
 await h.receiver.startPlayback('chapter',0,{title:'Chapter 15',kind:'chapter'});await settle();
 const book=h.metadataUpdates.at(-1).media.metadata;
 assert.equal(book.metadataType,3,'an audiobook is drawn as audio, not as a film');assert.equal(book.artist,'Andy Weir');assert.equal(book.albumName,'Project Hail Mary');
 assert(h.calls.some(c=>c.url==='/v1/items/book-1'&&c.headers.Authorization==='Bearer bearer-ABCD'));
 assert.match(book.images[0].url,/^data:image\/svg\+xml;base64,/,'the book cover becomes the square artwork');
});
test('a related record of the wrong kind never supplies a cover',async()=>{
 const h=await harness({direct:true,audioOnly:true,container:'mp3',artwork:true,catalog:{
  song:{id:'song',title:'DNA.',kind:'song',song:{artist:'Kendrick Lamar',albumTitle:'DAMN.',albumId:'other'}},
  other:{id:'other',title:'A song, not an album',kind:'song',posterUrl:'/v1/artwork/fixture'}
 }});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song',0,{title:'DNA.',kind:'song'});await settle();
 assert.equal(h.metadataUpdates.at(-1).media.metadata.images.length,0,'a song is not an album cover');
 assert(!h.calls.some(c=>c.url==='/v1/artwork/fixture'));
});
test('catalogue film year uses only its genuine bounded numeric year and omits invalid facts',async()=>{
 for(const year of [2002,undefined,0,-1,10000,2002.5,'2002',null]){
  const views=[],h=await harness({direct:true,container:'mp4',catalog:{movie:{id:'movie',title:'Film',kind:'movie',...(year===undefined?{}:{year})}},view:{playing:s=>views.push(s)}});
  await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0,{title:'Film',kind:'movie',subtitle:'1999'});await settle();h.events.LOADED_DATA();
  assert.equal(views.at(-1).meta.subtitle,year===2002?'2002':'');assert.equal(h.metadataUpdates.at(-1).media.metadata.subtitle,year===2002?'2002':'');
 }
});

test('refused and mismatched catalogue reads keep next identity absent without a notice',async()=>{
 for(const fixture of [{catalogStatus:403},{catalog:{'song-2':{id:'other',title:'Wrong title',kind:'song'}}}]){
  const views=[],notices=[],h=await harness({...fixture,queue:true,direct:true,audioOnly:true,container:'mp3',view:{playing:s=>views.push(s),notice:s=>notices.push(s)}});
  await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,{title:'Old song'},null,{queueId:'q-1',entryId:'entry-1'});h.finish();await settle();h.events.LOADED_DATA();
  assert.equal(views.at(-1).meta,null);assert.equal(h.metadataUpdates.length,0);assert(!notices.some(Boolean));assert.equal(h.receiver.state().playbackId,'ps_2');
 }
});
test('late catalogue responses cannot cross a stopped or replaced occurrence',async()=>{
 let release;const gate=new Promise(resolve=>release=resolve),views=[];
 const h=await harness({queue:true,direct:true,audioOnly:true,container:'mp3',catalogWait:gate,catalog:{'song-2':{id:'song-2',title:'Stale song',kind:'song'}},view:{playing:s=>views.push(s)}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});h.finish();await settle();await h.interceptors.STOP({});
 release();await settle();assert.equal(h.receiver.state().playbackId,null);assert.equal(h.metadataUpdates.length,0);assert(!views.some(s=>s.meta?.title==='Stale song'));
});

test('decoded multichannel and rate declarations require exact positive output configurations',async()=>{
 const h=await harness({supports:()=>true,mediaCapabilities:{decodingInfo:async q=>({supported:q.audio.channels==='6'&&q.audio.samplerate<=96000})}});
 const p=await h.receiver.probeAudioOutput(h.receiver.buildProfile(h.castContext));
 assert.equal(p.engines.native.audio.find(a=>a.codec==='flac').maxChannels,6);
 assert.equal(p.engines.native.audio.find(a=>a.codec==='flac').maxSampleRate,96000);
 const n=await harness({supports:()=>true,mediaCapabilities:{decodingInfo:async()=>{throw new Error('unavailable')}}});
 const no=await n.receiver.probeAudioOutput(n.receiver.buildProfile(n.castContext));
 assert.equal(no.engines.native.audio.find(a=>a.codec==='flac').maxChannels,2);
 assert.equal(no.engines.native.audio.find(a=>a.codec==='flac').maxSampleRate,96000,'a failed exact query retains the declared FLAC ceiling');
});

test('standard CAF queue reads and mutations cannot bypass controller authority',async()=>{
 const h=await harness();await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 for(const kind of ['GET_STATUS','MEDIA_STATUS','CLOUD_STATUS','QUEUE_GET_ITEMS','QUEUE_GET_ITEM_IDS']){
  assert.equal(h.interceptors[kind]({senderId:'stranger'}),null);
  assert.deepEqual(h.interceptors[kind]({senderId:'phone'}),{senderId:'phone'});
 }
 assert.equal(h.interceptors.QUEUE_NEXT,undefined);assert.equal(h.interceptors.QUEUE_PREV,undefined);
 for(const jump of [-1,1])assert.equal(h.interceptors.QUEUE_UPDATE({senderId:'phone',jump}),null);
 for(const kind of ['QUEUE_INSERT','QUEUE_LOAD','QUEUE_UPDATE','QUEUE_REMOVE','QUEUE_REORDER'])assert.equal(h.interceptors[kind]({senderId:'phone'}),null);
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

test('only the matching playable audio render plan describes the delivered audio container',async()=>{
 for(const [container,codec,type] of [['flac','flac','audio/flac'],['ogg','opus','audio/ogg'],['aac','aac','audio/aac'],['mp4','aac','audio/mp4']]){
  const h=await harness({direct:true,audioOnly:true,container:'mov',audioRender:{mode:'converted',container,codec}});
  await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0);
  assert.equal(h.loads[0].media.contentType,type);assert.equal(h.resolved(h.loads[0]),'/v1/media/grant');
 }
 for(const audioRender of [{mode:'converted',container:'flac',url:'/v1/media/different'},{mode:'unavailable',container:'flac'}]){
  const h=await harness({direct:true,audioOnly:true,container:'mov',audioRender});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0);
  assert.equal(h.loads[0].media.contentType,'audio/mp4','an unrelated or unavailable plan cannot relabel this URL');
 }
 const video=await harness({direct:true,container:'mp4',audioRender:{mode:'converted',container:'flac'}});await video.receiver.redeem('ABCD');await video.receiver.startPlayback('movie',0);
 assert.equal(video.loads[0].media.contentType,'video/mp4','an audio plan cannot relabel a video presentation');
 const stream=await harness({audioOnly:true,audioRender:{mode:'converted',container:'flac'}});await stream.receiver.redeem('ABCD');await stream.receiver.startPlayback('song',0);
 assert.equal(stream.loads[0].media.contentType,'application/x-mpegurl');assert.equal(stream.loads[0].media.hlsSegmentFormat,'FMP4');
 const bad=await harness({direct:true,audioOnly:true,audioRender:{mode:'converted',container:'unknown'}});await bad.receiver.redeem('ABCD');
 await assert.rejects(()=>bad.receiver.startPlayback('song',0),/unsupported_media_container/);
});

test('ordinary advance keeps the matching delivered audio render MIME without applying client gain',async()=>{
 const h=await harness({queue:true,direct:true,audioOnly:true,container:'mov',audioRender:{mode:'converted',container:'flac',codec:'flac',loudnessAppliedDb:-12,gain:-12}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('song-1',0,null,null,{queueId:'q-1',entryId:'entry-1'});
 h.finish();await settle();
 assert.equal(h.receiver.state().playbackId,'ps_2');assert.equal(h.loads[1].media.contentType,'audio/flac');assert.equal(h.resolved(h.loads[1]),'/v1/media/grant');
 assert(!('gain' in h.loads[1].media));assert(!('volume' in h.loads[1].media));assert(!JSON.stringify(h.sent).includes('/v1/media/'));
});

test('a CAF load rejection ends its owned server session and leaves an honest error card',async()=>{
 const h=await harness({loadFails:true});await h.receiver.redeem('ABCD');await assert.rejects(()=>h.receiver.startPlayback('movie',0),/decode_error/);
 assert.equal(h.receiver.state().playbackId,null);assert.equal(h.uiState(),'error');assert(h.calls.some(c=>c.method==='DELETE'&&c.url==='/v1/playback/sessions/ps_1'));
});

test('device identity comes only from the SDK and decoder limits do not invent display geometry',async()=>{
 const known=await harness({info:{'model-name':'Google TV Streamer'},supports:()=>true}),p=known.receiver.buildProfile(known.castContext);
 assert.equal(p.client.model,'Google TV Streamer');assert.equal(p.display.width,undefined);assert.equal(p.display.height,undefined);
 assert.equal(p.audioOutput.route,'');
 const unknown=await harness({info:{'model-name':'unknown'},supports:()=>true}),u=unknown.receiver.buildProfile(unknown.castContext);assert.equal(u.client.model,undefined);
});

test('receiver status publishes only actual finite SDK clock readings, including a genuine zero',async()=>{
 const h=await harness();await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});
 h.manager.getDurationSec=()=>180;
 for(const value of [undefined,null,NaN,Infinity,-1,'12']){
  h.manager.getCurrentTimeSec=()=>value;await h.message('phone',{type:'status'});
  const status=h.sent.filter(m=>m.body.type==='status').at(-1).body;
  assert(!Object.hasOwn(status,'positionSeconds'));assert.equal(status.durationSeconds,180);
 }
 h.manager.getCurrentTimeSec=()=>0;await h.message('phone',{type:'status'});
 assert.equal(h.sent.filter(m=>m.body.type==='status').at(-1).body.positionSeconds,0);
 h.manager.getCurrentTimeSec=()=>61.25;h.manager.getDurationSec=()=>NaN;await h.message('phone',{type:'status'});
 const unknownDuration=h.sent.filter(m=>m.body.type==='status').at(-1).body;
 assert.equal(unknownDuration.positionSeconds,61.25);assert(!Object.hasOwn(unknownDuration,'durationSeconds'));
 h.manager.getCurrentTimeSec=()=>{throw new Error('not ready')};h.manager.getDurationSec=()=>{throw new Error('not ready')};await h.message('phone',{type:'status'});
 const notReady=h.sent.filter(m=>m.body.type==='status').at(-1).body;assert(!Object.hasOwn(notReady,'positionSeconds'));assert(!Object.hasOwn(notReady,'durationSeconds'));
});

function viewFixture(){const calls=[];return {calls,view:Object.fromEntries(['idle','loading','artwork','preparing','playing','takeover','notice','error','pairingStatus','browserPair','splash'].map(name=>[name,(...args)=>calls.push({name,args:j(args)})]))};}
test('view playback starts at an actual first-frame/play event and clock ticks do not reveal overlays',async()=>{
 const v=viewFixture(),h=await harness({view:v.view,caps:{dpad_input_supported:true}});
 await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0,{title:'Pilot',seriesTitle:'The Show',season:1,episode:2,kind:'episode'});
 assert(v.calls.some(c=>c.name==='loading'&&c.args[0].title==='Pilot'));assert(!v.calls.some(c=>c.name==='playing'));
 h.manager.getDurationSec=()=>180;h.events.LOADED_DATA();
 let playing=v.calls.filter(c=>c.name==='playing').at(-1).args[0];assert.equal(playing.positionSeconds,0);assert.equal(playing.durationSeconds,180);assert.equal(playing.remoteKeysAvailable,true);assert.equal(playing.revealOverlay,undefined);
 h.manager.seek(42);h.events.TIME_UPDATE();playing=v.calls.filter(c=>c.name==='playing').at(-1).args[0];assert.equal(playing.positionSeconds,42);assert.equal(playing.revealOverlay,undefined);
 h.events.PLAYING();assert.equal(v.calls.filter(c=>c.name==='playing').at(-1).args[0].revealOverlay,true);
 h.events.TIME_UPDATE();assert.equal(v.calls.filter(c=>c.name==='playing').at(-1).args[0].revealOverlay,undefined);
 h.manager.pause();h.events.PAUSE();playing=v.calls.filter(c=>c.name==='playing').at(-1).args[0];assert.equal(playing.state,'paused');assert.equal(playing.revealOverlay,true);
 h.events.BUFFERING({isBuffering:true});assert.equal(v.calls.filter(c=>c.name==='playing').at(-1).args[0].state,'buffering');
 assert(!JSON.stringify(v.calls).includes('bearer-'));assert(!JSON.stringify(v.calls).includes('/v1/media/'));
});
test('a title-less CAF load gets the fallback splash only from actual PLAYER_LOADING',async()=>{
 const v=viewFixture(),h=await harness({view:v.view});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0);
 assert(!v.calls.some(c=>c.name==='splash'||c.name==='playing'||c.name==='loading'));
 h.events.PLAYER_LOADING();assert.equal(v.calls.filter(c=>c.name==='splash').length,1);
 const titled=viewFixture(),known=await harness({view:titled.view});await known.receiver.redeem('ABCD');await known.receiver.startPlayback('movie',0,{title:'Pilot'});
 known.events.PLAYER_LOADING();assert(!titled.calls.some(c=>c.name==='splash'));
 const music=await harness();await music.receiver.redeem('ABCD');await music.receiver.startPlayback('song',0,{title:'Song',kind:'song',subtitle:'Artist · Album'});
 assert.equal(music.loads[0].media.metadata.artist,undefined,'joined display text is not a genuine artist field');
});
test('view preparation elapsed time comes only from a real202 response and stops at readiness/shutdown',async()=>{
 const v=viewFixture(),clock={now:1000},h=await harness({view:v.view,clock,preparing:true});await h.receiver.redeem('ABCD');
 const started=h.receiver.startPlayback('movie',0,{title:'Pilot'});await settle();
 assert.equal(v.calls.filter(c=>c.name==='preparing').at(-1).args[0],0);
 clock.now=9500;const tick=[...h.timers].find(([,t])=>t.ms===1000);h.fire(tick[0]);assert.equal(v.calls.filter(c=>c.name==='preparing').at(-1).args[0],8500);
 const retry=[...h.timers].find(([,t])=>t.ms===500);h.fire(retry[0]);await started;
 assert(![...h.timers.values()].some(t=>t.ms===1000));
 h.ctxEvents.shutdown();assert.equal(h.timers.size,0);
 const regular=viewFixture(),plain=await harness({view:regular.view});await plain.receiver.redeem('ABCD');await plain.receiver.startPlayback('movie',0,{title:'Pilot'});
 assert(!regular.calls.some(c=>c.name==='preparing'));
});
test('takeover view receives actual authority windows and remote observations cannot reset their deadline',async()=>{
 const v=viewFixture(),clock={now:1000},h=await harness({view:v.view,clock});h.senders.push('phone','guest');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});await h.message('guest',{type:'pair',code:'OTHER1',origin:'',displayName:'Sam’s phone'});
 let prompt=v.calls.filter(c=>c.name==='takeover'&&c.args[0]).at(-1).args[0];assert.equal(prompt.timeoutMs,30000);assert.equal(prompt.controllerPresent,true);assert.equal(prompt.remoteKeysAvailable,false);
 clock.now=6000;h.ctxEvents['window:keydown']({key:'ArrowUp'});
 prompt=v.calls.filter(c=>c.name==='takeover'&&c.args[0]).at(-1).args[0];assert.equal(prompt.timeoutMs,25000);assert.equal(prompt.remoteKeysAvailable,true);
 await h.message('phone',{type:'takeover',allow:false});assert.equal(v.calls.filter(c=>c.name==='takeover').at(-1).args[0],null);
});
test('view observes active track changes, omits unknown clocks, and suppresses transient engine notices',async()=>{
 const v=viewFixture(),h=await harness({view:v.view,timelineDown:true});await h.receiver.redeem('ABCD');await h.receiver.startPlayback('movie',0,{title:'Pilot'});
 let active=1;h.manager.getAudioTracksManager=()=>({getTracks:()=>[],getActiveId:()=>active});h.events.PLAYING();
 active=2;h.events.TIME_UPDATE();assert.equal(v.calls.filter(c=>c.name==='playing').at(-1).args[0].revealOverlay,true);
 h.events.TIME_UPDATE();assert.equal(v.calls.filter(c=>c.name==='playing').at(-1).args[0].revealOverlay,undefined);
 h.manager.getCurrentTimeSec=()=>undefined;h.manager.getDurationSec=()=>NaN;h.events.TIME_UPDATE();
 const p=v.calls.filter(c=>c.name==='playing').at(-1).args[0];assert(!Object.hasOwn(p,'positionSeconds'));assert(!Object.hasOwn(p,'durationSeconds'));
 await fireAll(h,0);assert(v.calls.filter(c=>c.name==='notice').every(c=>c.args[0]===''));
 const failed=viewFixture(),bad=await harness({view:failed.view,loadFails:true});await bad.receiver.redeem('ABCD');await assert.rejects(()=>bad.receiver.startPlayback('movie',0),/decode_error/);
 assert.equal(failed.calls.filter(c=>c.name==='error').at(-1).args[0],'decode');
});
test('engine status copy uses the generated catalogue when loaded and never guesses a requester model',async()=>{
 const h=await harness({text:{t:(key)=>'translated:'+key}});
 await h.message('phone',{type:'pair',code:'ABCD',origin:'http://unsafe.example'});
 assert.equal(h.sent.at(-1).body.message,'translated:cast.receiver.pair.failed');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie'});
 await h.message('guest',{type:'pair',code:'OTHER1',origin:''});
 assert.equal(h.nodes['takeover-title'].textContent,'translated:cast.receiver.takeover.title');
 await h.message('phone',{type:'takeover',allow:false});
 assert.equal(h.sent.filter(m=>m.id==='guest').at(-1).body.message,'translated:cast.receiver.pair.busy');
});

test('CAF local media requests retain private object authority through interceptors',async()=>{
 const h=await harness({localInterceptors:true});await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',itemId:'movie',title:'Movie'});assert.equal(h.loads.length,1);
 const own=h.loads[0];assert.equal(await h.interceptors.LOAD({...own,senderId:'stranger'}),null,'published opaque identity cannot authorize an external clone');
 assert.equal(await h.interceptors.PLAY({senderId:'stranger'}),null);assert.equal(await h.interceptors.SEEK({senderId:'stranger',currentTime:5}),null);
 await h.message('phone',{type:'pause'});assert.equal(h.manager.getPlayerState(),'PAUSED');
 await h.message('phone',{type:'play'});assert.equal(h.manager.getPlayerState(),'PLAYING');
 await h.message('phone',{type:'seek',positionSeconds:5});assert.equal(h.manager.getCurrentTimeSec(),5);
 await h.message('phone',{type:'stop'});assert.equal(h.manager.getPlayerState(),'IDLE');assert.equal(h.receiver.state().playbackId,null);
});

// Root50 regressions: held authority/queue boundaries exercise the actual
// receiver closure. These sources are for Release's normal checkpoint.
const held=()=>{let resolve;const promise=new Promise(done=>{resolve=done;});return {promise,resolve};};
test('disconnect during current-controller validation never rearms or dispatches the old sender',async()=>{
 const gate=held(),entered=held();let hold=false;
 const h=await harness({controllerCheck:async()=>{if(hold){entered.resolve();await gate.promise;}}});
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,title:'Film'});
 hold=true;const before=h.calls.length;
 const command=h.message('phone',{type:'load',itemId:'stale',startSeconds:0,title:'Old'});await entered.promise;
 h.ctxEvents.disconnect({senderId:'phone'});gate.resolve();await command;await h.message('observer',{type:'status'});
 assert.equal(h.calls.slice(before).some(x=>x.url.endsWith('/v1/playback/sessions')&&x.body?.itemId==='stale'),false);
 const count=h.sent.length;await h.message('phone',{type:'status'});
 assert.equal(h.sent.slice(count).some(x=>x.body.type==='status'),false);
});
test('a later adoption attempt wins even when the earlier validation resolves last',async()=>{
 const gate=held(),entered=held();
 const h=await harness({controllerCheck:async body=>{if(body.attemptId==='older'){entered.resolve();await gate.promise;}}});
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,title:'Film'});
 const paired=h.sent.find(x=>x.body.type==='paired').body;
 const older=h.message('first-return',{type:'adopt',bindingId:paired.binding.id,controlToken:paired.controlToken,attemptId:'older',ticket:'a'.repeat(43)});await entered.promise;
 await h.message('latest-return',{type:'adopt',bindingId:paired.binding.id,controlToken:paired.controlToken,attemptId:'newer',ticket:'b'.repeat(43)});
 gate.resolve();await older;await h.message('observer',{type:'status'});assert.equal(h.receiver.state().controller,'latest-return');
 assert.equal(h.sent.some(x=>x.id==='first-return'&&x.body.type==='status'),false);
});
test('takeover queued ahead of an old authenticated load cannot lend it the successor credential',async()=>{
 const check=held(),entered=held();let hold=false;
 const fixture={controllerCheck:async()=>{if(hold){entered.resolve();await check.promise;}}};
 const h=await harness(fixture);
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,title:'Film'});
 hold=true;const stale=h.message('phone',{type:'load',itemId:'stale',startSeconds:0,title:'Old'});await entered.promise;
 // This explicit same-viewer pair advances controller/session generations.
 await h.message('successor',{type:'pair',code:'NEW123',origin:'https://server.test'});
 check.resolve();await stale;await h.message('observer',{type:'status'});
 assert.equal(h.calls.some(x=>x.url.endsWith('/v1/playback/sessions')&&x.body?.itemId==='stale'),false);
 assert.equal(h.receiver.state().controller,'successor');
});

test('PAIR retains its SDK connection witness from receipt across delayed redemption',async()=>{
 const gate=held(),entered=held(),fixture={redeemWait:gate.promise,redeemCalled:()=>entered.resolve()};
 const h=await harness(fixture);const pending=h.message('late-phone',{type:'pair',code:'ABC123',origin:'https://server.test'});await entered.promise;
 h.ctxEvents.disconnect({senderId:'late-phone'});gate.resolve();await pending;await h.message('observer',{type:'status'});
 assert.equal(h.sent.some(x=>x.id==='late-phone'&&x.body.type==='paired'),false);
 assert.equal(h.receiver.state().controller,null);
 assert.equal(h.loads.length,0);
});
test('a parsed accepted start retired before publication cleans up only its known occurrence with its original bearer',async()=>{
 const fixture={},h=await harness(fixture);
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',title:'Film'});
 const gate=held(),entered=held();fixture.nextSessionId='ps_known';fixture.startWait=gate.promise;fixture.startCalled=()=>entered.resolve();
 const pending=h.message('phone',{type:'load',itemId:'stale',title:'Old'});await entered.promise;
 await h.receiver.reconnect();gate.resolve();await pending;await h.message('observer',{type:'status'});
 const cleanup=h.calls.filter(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_known'));
 assert.equal(cleanup.length,1);assert.equal(cleanup[0].headers.Authorization,'Bearer bearer-ABC123');
 assert.equal(h.calls.some(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1')),false);
 assert.equal(h.loads.length,1,'stale success never loads or stops a successor deck');
});
test('refused pending adoption cannot invalidate an already accepted controller command',async()=>{
 const gate=held(),entered=held();let hold=false;
 const h=await harness({controllerRefuse:body=>body.attemptId==='invalid',controllerCheck:async body=>{if(hold&&!body.attemptId){entered.resolve();await gate.promise;}}});
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',title:'Film'});
 const paired=h.sent.find(x=>x.body.type==='paired').body;
 hold=true;const pending=h.message('phone',{type:'load',itemId:'allowed',title:'Current request'});await entered.promise;
 await h.message('invalid-return',{type:'adopt',bindingId:paired.binding.id,controlToken:paired.controlToken,attemptId:'invalid',ticket:'x'.repeat(43)});
 gate.resolve();await pending;await h.message('observer',{type:'status'});
 assert.equal(h.receiver.state().controller,'phone');
 assert(h.calls.some(x=>x.url.endsWith('/v1/playback/sessions')&&x.body?.itemId==='allowed'));
});

test('Root51 shared CAF status keeps the active display and strips private application/track/queue payloads',async()=>{
 const h=await harness({direct:true});await h.receiver.redeem('ABC123');
 await h.receiver.startPlayback('movie',0,{title:'Film',kind:'movie'});
 const media=h.loads.at(-1).media;
 const raw={type:'MEDIA_STATUS',mediaSessionId:1,playerState:'PLAYING',currentTime:10,customData:{accountId:'private',ticket:'secret'},queueData:{items:[{media:{contentId:'future'}}]},items:[{itemId:7}],extendedStatus:{media:{contentId:'future'}},
 media:{...media,contentUrl:'/v1/media/private-grant',customData:{profileId:'private',family:'private'},entity:'private-item',metadata:{...media.metadata,accountId:'private',images:[{url:'https://server.test/v1/media/secret-art'}]},
 tracks:[{trackId:1,type:'TEXT',language:'en',name:'English',trackContentId:'https://server.test/private-subtitle',customData:{token:'secret'}}]},activeTrackIds:[1,2]};
 const safe=h.interceptors.MEDIA_STATUS(raw);
 assert.equal(safe.media.metadata.title,'Film');assert.equal(safe.media.contentId,media.contentId);assert.deepEqual(Array.from(safe.activeTrackIds),[1]);
 const serialized=JSON.stringify(safe);for(const privateField of ['private','secret','future','ticket','queueData','trackContentId','customData'])assert(!serialized.includes(privateField));
 assert.equal(h.interceptors.MEDIA_STATUS({...raw,media:{...raw.media,contentId:'portico-'+ '0'.repeat(32)}}),null,'stale other-item status is refused rather than recipient-inferred');
});

test('pair connection retirement during old Stop cannot install origin/credentials or prepare the refused target',async()=>{
 const fixture={},h=await harness(fixture);
 await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});await h.message('phone',{type:'load',itemId:'movie',title:'Film'});
 const gate=held(),entered=held();fixture.stopWait=gate.promise;fixture.stopCalled=()=>entered.resolve();
 await h.message('successor',{type:'pair',code:'OTHER1',origin:'https://second.test'});
 const allowed=h.message('phone',{type:'takeover',allow:true});await entered.promise;
 h.ctxEvents.disconnect({senderId:'successor'});gate.resolve();await allowed;await h.message('observer',{type:'status'});
 assert.equal(h.storage.get('portico.cast.deviceToken'),'device-ABC123');
 assert.equal(h.calls.some(x=>x.url.startsWith('https://second.test')&&x.url.endsWith('/v1/me/devices/current/capabilities')),false);
 assert(h.calls.some(x=>x.url==='https://second.test/v1/cast/devices/cdv_OTHER1'&&x.method==='DELETE'));
 assert.equal(h.sent.some(x=>x.id==='successor'&&x.body.type==='paired'),false);
});
test('a queued never-installed pair is discarded without stopping the accepted controller',async()=>{
 const fixture={},h=await harness(fixture);await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});await h.message('phone',{type:'load',itemId:'movie',title:'Film'});
 const gate=held(),entered=held();fixture.startWait=gate.promise;fixture.startCalled=()=>entered.resolve();
 const load=h.message('phone',{type:'load',itemId:'next',title:'Next'});await entered.promise;
 await h.message('successor',{type:'pair',code:'OTHER1',origin:'https://second.test'});await h.message('phone',{type:'takeover',allow:true});
 h.ctxEvents.disconnect({senderId:'successor'});gate.resolve();await load;await h.message('observer',{type:'status'});
 assert.equal(h.receiver.state().controller,'phone');assert.equal(h.storage.get('portico.cast.deviceToken'),'device-ABC123');
 assert(h.calls.some(x=>x.url==='https://second.test/v1/cast/devices/cdv_OTHER1'&&x.method==='DELETE'));
 assert.equal(h.calls.some(x=>x.method==='DELETE'&&x.url.includes('/v1/playback/sessions/')),false);
});
test('discarding a refused replay cannot revoke an already installed identical target generation',async()=>{
 const fixture={},h=await harness(fixture);await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 const gate=held(),entered=held();fixture.redeemWait=gate.promise;fixture.redeemCalled=()=>entered.resolve();
 const replay=h.message('late-replay',{type:'pair',code:'ABC123',origin:'https://server.test'});await entered.promise;
 h.ctxEvents.disconnect({senderId:'late-replay'});gate.resolve();await replay;await h.message('observer',{type:'status'});
 assert.equal(h.calls.some(x=>x.url==='https://server.test/v1/cast/devices/cdv_ABC123'&&x.method==='DELETE'),false);
});

test('Root51 standard and cloud status fit a complete UTF-8 envelope and keep only genuine small safe artwork',async()=>{
 const h=await harness({direct:true});await h.receiver.redeem('ABC123');await h.receiver.startPlayback('movie',0,{title:'Film',kind:'movie'});
 const media=h.loads.at(-1).media,small='data:image/png;base64,'+Buffer.from('actual fixture image bytes').toString('base64');
 const envelopeSize=(status,type)=>Buffer.byteLength(JSON.stringify({namespace:'urn:x-cast:com.google.cast.media',payloadUtf8:JSON.stringify({type,requestId:Number.MAX_SAFE_INTEGER,status:[status]})}),'utf8');
 for(const type of ['MEDIA_STATUS','CLOUD_STATUS']){
  const raw={type,mediaSessionId:1,currentTime:1,playerState:'PLAYING',customData:{controller:'private'},queueData:{items:[{id:'future'}]},media:{...media,metadata:{title:'曲'.repeat(200),images:[{url:small}]},tracks:[{trackId:1,type:'TEXT',name:'日本語',language:'ja',trackContentId:'/private/subtitle'}]}};
  const safe=h.interceptors[type](raw);assert.equal(safe.media.metadata.images[0].url,small);assert(envelopeSize(safe,type)<=48*1024);assert(!JSON.stringify(safe).includes('private'));assert(!JSON.stringify(safe).includes('future'));
  raw.media.metadata.images=[{url:'data:image/png;base64,'+'a'.repeat(4*1024*1024)}];
  raw.media.tracks=Array.from({length:32},(_,i)=>({trackId:i,type:'TEXT',name:'音'.repeat(100),language:'ja'}));
  const bounded=h.interceptors[type](raw);assert(envelopeSize(bounded,type)<=48*1024);assert.equal(bounded.media.metadata.images,undefined);assert.equal(bounded.media.metadata.title,'曲'.repeat(200));
 }
});

test('private status reports the actual controller/occurrence and targeted Stop cannot become a room Stop',async()=>{
 const h=await harness({direct:true});await h.message('phone',{type:'pair',code:'ABC123',origin:'https://server.test'});
 await h.message('phone',{type:'load',itemId:'movie',title:'Film'});await h.message('phone',{type:'status'});
 const status=h.sent.filter(x=>x.id==='phone'&&x.body.type==='status'&&x.body.occurrence).at(-1).body;
 assert.equal(status.controller.bindingId,'cct_current');assert.equal(status.occurrence.sessionId,'ps_1');assert.equal(status.occurrence.generation,1);assert.equal(status.occurrence.itemId,'movie');
 const target={controller:status.controller,occurrence:status.occurrence},before=h.calls.filter(x=>x.method==='DELETE').length;
 await h.message('observer',{type:'stop',target,requestId:'old-observer'});assert.equal(h.receiver.state().playbackId,'ps_1');
 await h.message('phone',{type:'stop',target:{...target,occurrence:{...target.occurrence,generation:2}},requestId:'wrong-occurrence'});
 assert.equal(h.receiver.state().playbackId,'ps_1');assert.equal(h.calls.filter(x=>x.method==='DELETE').length,before);
 await h.message('phone',{type:'stop',target,requestId:'actual-stop'});await settle0();
 const ack=h.sent.find(x=>x.body.type==='stopped'&&x.body.requestId==='actual-stop').body;
 assert.equal(ack.target.occurrence.sessionId,'ps_1');assert.equal(ack.target.occurrence.generation,1);assert.equal(h.receiver.state().playbackId,null);
});

test('preparation continues past the old receiver deadline with the same start request',async()=>{
 const clock={now:0}, h=await harness({clock,preparingResponses:Array.from({length:13},()=>({retryAfterMs:10000}))});
 await h.receiver.redeem('ABCD');const start=h.receiver.startPlayback('movie');await settle0();
 for(let i=0;i<13;i++){clock.now+=10000;const timer=[...h.timers].find(([,v])=>v.ms===10000);assert(timer);h.fire(timer[0]);await settle0();}
 await start;const starts=h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions')&&c.method==='POST');
 assert.equal(starts.length,14);for(const call of starts){assert.deepEqual(call.body,starts[0].body);assert.equal(call.headers['Idempotency-Key'],starts[0].headers['Idempotency-Key']);}
 assert.equal(h.loads.length,1);
});

test('malformed preparation does not retry',async()=>{
 for(const bad of [{},{retryAfterMs:0},{retryAfterMs:10001},{retryAfterMs:1.5},{retryAfterMs:1,extra:true}]){
  const h=await harness({preparingResponses:[bad]});await h.receiver.redeem('ABCD');
  await assert.rejects(h.receiver.startPlayback('movie'),/invalid_preparation/);
  assert.equal(h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions')&&c.method==='POST').length,1);assert.equal(h.loads.length,0);
 }
});

test('connected original controller resumes only after correlated renewal acknowledgement',async()=>{
 const h=await harness({expired:true,renewal:true});h.senders.push({senderId:'phone'});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 const paired=h.sent.find(x=>x.body.type==='paired').body;
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,requestId:'load-1'});assert.equal(h.loads.length,1);
 await fireAll(h,0);await settle0();
 const offer=h.sent.find(x=>x.body.type==='renewal-offer')?.body;assert(offer);
 await h.message('phone',{type:'status'});assert(h.sent.some(x=>x.body.type==='busy'));
 await h.message('phone',{type:'renewal-ack',attemptId:'wrong',bindingId:offer.binding.id,controlToken:paired.controlToken,ticket:'b'.repeat(43)});
 assert(!h.sent.some(x=>x.body.type==='renewal-complete'));
 await h.message('phone',{type:'renewal-ack',attemptId:offer.attemptId,bindingId:offer.binding.id,controlToken:paired.controlToken,ticket:'b'.repeat(43)});
 assert(h.sent.some(x=>x.body.type==='renewal-complete'));
 await h.message('phone',{type:'seek',positionSeconds:99,controller:paired.controller});assert.equal(h.manager.getCurrentTimeSec(),0,'old controller stamp cannot dispatch');
 await h.message('phone',{type:'seek',positionSeconds:5,controller:offer.controller});await h.message('phone',{type:'play',controller:offer.controller});await h.message('phone',{type:'status',controller:offer.controller});
 assert(h.sent.some(x=>x.body.type==='status'));assert.equal(h.loads.length,1);assert.equal(h.receiver.state().playbackId,'ps_1');
});

test('first controller-check 401 refuses the old command and permits a fresh command after renewal',async()=>{
 const h=await harness({firstController401:true,renewal:true});h.senders.push({senderId:'phone'});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});const paired=h.sent.find(x=>x.body.type==='paired').body;
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,requestId:'stale'});
 const offer=h.sent.find(x=>x.body.type==='renewal-offer')?.body;assert(offer);assert.equal(h.loads.length,0);
 await h.message('phone',{type:'renewal-ack',attemptId:offer.attemptId,bindingId:offer.binding.id,controlToken:paired.controlToken,ticket:'b'.repeat(43)});
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,requestId:'fresh',controller:offer.controller});assert.equal(h.loads.length,1);
});

test('a disconnected controller cannot retry an old preparing start',async()=>{
 const h=await harness({preparingResponses:[{retryAfterMs:500},{retryAfterMs:500}]});h.senders.push({senderId:'phone'});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,requestId:'pending-load'});
 const wait=[...h.timers].find(([,timer])=>timer.ms===500);assert(wait);
 h.ctxEvents.disconnect({senderId:'phone'});h.fire(wait[0]);await settle0();
 assert.equal(h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions')&&c.method==='POST').length,1);assert.equal(h.loads.length,0);
});

test('renewal acknowledgement from a disconnected SDK connection cannot enable control',async()=>{
 const h=await harness({expired:true,renewal:true});h.senders.push({senderId:'phone'});
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});const paired=h.sent.find(x=>x.body.type==='paired').body;
 await h.message('phone',{type:'load',itemId:'movie',startSeconds:0,requestId:'load-1'});await fireAll(h,0);await settle0();
 const offer=h.sent.find(x=>x.body.type==='renewal-offer')?.body;assert(offer);
 h.ctxEvents.disconnect({senderId:'phone'});
 await h.message('phone',{type:'renewal-ack',attemptId:offer.attemptId,bindingId:offer.binding.id,controlToken:paired.controlToken,ticket:'b'.repeat(43)});
 assert(!h.sent.some(x=>x.body.type==='renewal-complete'));assert.equal(h.receiver.state().playbackId,'ps_1');
});

test('server preparation terminal refusal stops the original retry loop',async()=>{
 const h=await harness({preparingResponses:[{retryAfterMs:500}],prepareTerminal:true});await h.receiver.redeem('ABCD');
 const start=h.receiver.startPlayback('movie');await settle0();const wait=[...h.timers].find(([,timer])=>timer.ms===500);assert(wait);h.fire(wait[0]);
 await assert.rejects(start,/preparation_failed/);assert.equal(h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions')&&c.method==='POST').length,2);assert.equal(h.loads.length,0);
});

test('browser preparation does not borrow a replacement receiver credential',async()=>{
 const h=await harness({preparingResponses:[{retryAfterMs:500},{retryAfterMs:500}]});await h.receiver.redeem('ABCD');
 const start=h.receiver.startPlayback('movie');await settle0();const wait=[...h.timers].find(([,timer])=>timer.ms===500);assert(wait);
 await h.receiver.reconnect();h.fire(wait[0]);await assert.rejects(start,/receiver_replaced/);
 assert.equal(h.calls.filter(c=>c.url.endsWith('/v1/playback/sessions')&&c.method==='POST').length,1);assert.equal(h.loads.length,0);
});

test('real preparing resource is cleaned with its original bearer when its controller disconnects',async()=>{
 const h=await harness({preparingReal:true});h.senders.push('phone');await h.message('phone',{type:'pair',code:'ABCD',origin:''});
 await h.message('phone',{type:'load',itemId:'movie',requestId:'preparing'});
 const timer=[...h.timers].find(([,v])=>v.ms===500);assert(timer);h.ctxEvents.disconnect({senderId:'phone'});h.fire(timer[0]);await settle0();
 const cleanup=h.calls.filter(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1'));assert.equal(cleanup.length,1);assert.equal(cleanup[0].headers.Authorization,'Bearer bearer-ABCD');assert.equal(cleanup[0].body.expectedGeneration,1);assert.equal(h.loads.length,0);
});

test('late real 202 after retirement cleans the captured resource; retry-only owns none',async()=>{
 for(const real of [true,false]){
  let release,called;const reached=new Promise(r=>{called=r}),wait=new Promise(r=>{release=r});
  const h=await harness({startWait:wait,startCalled:called,...(real?{preparingReal:true}:{preparingResponses:[{retryAfterMs:500}]})});h.senders.push('phone');
  await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie',requestId:'late'});await reached;
  h.ctxEvents.disconnect({senderId:'phone'});release();await settle0();
  const cleanup=h.calls.filter(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1'));assert.equal(cleanup.length,real?1:0);assert.equal(h.loads.length,0);
 }
});

test('real preparation terminal refusal cleans its original resource',async()=>{
 const h=await harness({preparingReal:true,prepareTerminal:true});await h.receiver.redeem('ABCD');
 const started=h.receiver.startPlayback('movie');await settle0();const timer=[...h.timers].find(([,v])=>v.ms===500);assert(timer);h.fire(timer[0]);
 await assert.rejects(started,/preparation_failed/);assert.equal(h.calls.filter(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1')).length,1);
});

test('real preparation accepts only its same final session and generation',async()=>{
 const h=await harness({preparingReal:true});await h.receiver.redeem('ABCD');
 const start=h.receiver.startPlayback('movie');await settle0();const timer=[...h.timers].find(([,v])=>v.ms===500);assert(timer);h.fire(timer[0]);await start;
 assert.equal(h.loads.length,1);assert(!h.calls.some(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1')));
});

test('malformed or replaced preparation cleans only the original never-installed resource',async()=>{
 for(const second of [{retryAfterMs:1,garbage:true},session=>({...session,id:'other',state:'preparing',retryAfterMs:500,lease:{...session.lease,expiresAt:'2099-01-01T00:00:00Z'},presentation:{...session.presentation,engine:'native'}})]){
  const h=await harness({preparingReal:true,preparingResponses:[second]});await h.receiver.redeem('ABCD');
  const start=h.receiver.startPlayback('movie');await settle0();const timer=[...h.timers].find(([,v])=>v.ms===500);assert(timer);h.fire(timer[0]);await assert.rejects(start,/invalid_preparation|preparation_replaced/);
  const cleanup=h.calls.filter(x=>x.method==='DELETE'&&x.url.includes('/v1/playback/sessions/'));assert.equal(cleanup.length,1);assert(cleanup[0].url.endsWith('/ps_1'));assert.equal(h.loads.length,0);
 }
});

test('late renewal ticket cannot admit a replacement SDK connection',async()=>{
 let release,called;const blocked=new Promise(r=>{release=r}),reached=new Promise(r=>{called=r});
 const h=await harness({expired:true,renewal:true,controllerCheck:async body=>{if(body.attemptId){called();await blocked;}}});h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});const paired=h.sent.find(x=>x.body.type==='paired').body;
 await h.message('phone',{type:'load',itemId:'movie',requestId:'one'});await fireAll(h,0);await settle0();
 const offer=h.sent.find(x=>x.body.type==='renewal-offer')?.body;assert(offer);
 await h.message('phone',{type:'renewal-ack',attemptId:offer.attemptId,bindingId:offer.binding.id,controlToken:paired.controlToken,ticket:'b'.repeat(43)});await reached;
 h.ctxEvents.disconnect({senderId:'phone'});h.ctxEvents.connect({senderId:'phone'});release();await settle0();
 assert(!h.sent.some(x=>x.body.type==='renewal-complete'));assert.equal(h.receiver.state().playbackId,'ps_1');
});

test('real 202 requires the authoritative preparation session domain',async()=>{
 const valid=session=>({...session,state:'preparing',retryAfterMs:500,lease:{expiresAt:'2099-01-01T00:00:00Z',reportEveryMs:10000},presentation:{...session.presentation,engine:'native'}});
 const bad=[
  session=>{const value=valid(session);value.lease.expiresAt='not-a-time';return value},
  session=>{const value=valid(session);value.lease.reportEveryMs=-1;return value},
  session=>{const value=valid(session);value.presentation.startPositionMs=-1;return value},
  session=>({...valid(session),kind:'unknown-kind'}),
  session=>({...valid(session),role:'unknown-role'}),
  session=>{const value=valid(session);value.presentation.mode='unknown-mode';return value},
  session=>{const value=valid(session);value.presentation.engine='unknown-engine';return value},
  session=>{const value=valid(session);value.presentation.decision={};return value},
  session=>{const value=valid(session);value.presentation.decision={video:{action:'copy',reasons:[42]}};return value},
  session=>{const value=valid(session);value.lease.unexpected=true;return value},
  ()=>({retryAfterMs:500,id:'partial-resource'})
 ];
 for(const response of bad){
  const h=await harness({preparingResponses:[response]});await h.receiver.redeem('ABCD');
  await assert.rejects(h.receiver.startPlayback('movie'),/invalid_preparation/);assert.equal(h.loads.length,0);
 }
});

test('accepted start retired after the inner response check cleans only its original occurrence',async()=>{
 let h;const fixture={retireAfterAccepted:()=>h.ctxEvents.disconnect({senderId:'phone'})};h=await harness(fixture);h.senders.push('phone');
 await h.message('phone',{type:'pair',code:'ABCD',origin:''});await h.message('phone',{type:'load',itemId:'movie',requestId:'late-accepted'});await settle0();
 const cleanup=h.calls.filter(x=>x.method==='DELETE'&&x.url.endsWith('/v1/playback/sessions/ps_1'));
 assert.equal(cleanup.length,1);assert.equal(cleanup[0].headers.Authorization,'Bearer bearer-ABCD');assert.equal(cleanup[0].body.expectedGeneration,1);assert.equal(h.loads.length,0);
});
