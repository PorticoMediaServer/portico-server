import test from 'node:test';import assert from 'node:assert/strict';
import {PlayerOffersService} from '../src/player-offers.ts';
const scope={serverId:'server',viewerId:'local-owner-profile'},q={libraryId:'library',itemId:'item',sessionId:'session'};
function data():any{return {scope:{serverId:'server',libraryId:'library',itemId:'item',viewerFence:'fence'},revision:'rev1',sources:[{id:'source',available:true,container:'mkv',videoCodec:'h264',audioCodec:'aac',width:1920,height:1080,duration:100,factsRevision:1,factsStatus:'known',relation:'unknown',boundary:'whole_source',enabled:false,reason:'source_selection_unavailable',streams:[{index:1,type:'audio',codec:'aac',language:'eng',channels:2,default:true,forced:false,enabled:false,reason:'track_selection_unavailable'}],qualities:[{id:'auto',kind:'automatic',label:'Automatic',enabled:true},{id:'original',kind:'original',label:'Original',enabled:true},{id:'1080p',kind:'fixed',label:'1080p',enabled:true,maxVideoBitrateBps:8000000,maxAudioBitrateBps:192000,targetDisplayHeight:1080},{id:'720p',kind:'fixed',label:'720p',enabled:false,reason:'exceeds_network_policy',maxVideoBitrateBps:4000000,maxAudioBitrateBps:128000,targetDisplayHeight:720}]}],current:{sessionId:'session',generation:1,state:'playing',sourceId:'source',delivery:'direct',audioSelection:'platform_default',subtitleSelection:'platform_default',qualityId:'auto'},controls:['source','audio','subtitles','quality'].map(id=>id==='quality'?({id,enabled:true,reason:''}):({id,enabled:false,reason:'track_selection_unavailable'})),deliveryPolicy:{networkClass:'local',serverLocality:'local',transportClass:'ethernet',preferenceLane:'local',directPlay:'prefer',directStream:'allow',transcode:'allow',qualityMode:'original',maxVideoBitrateBps:0,maxAudioBitrateBps:0,maxVideoHeight:0,allowHDR:true,planningPolicy:'maximum_fidelity',clamps:[]},offersRevision:'a'.repeat(64),transcodingEnabled:true};}
function service(fn:(path:string)=>Promise<unknown>,timeoutMs=1000){return new PlayerOffersService({scope,timeoutMs,api:{request:<T>(p:string,m?:string,b?:unknown)=>{assert.equal(m,'GET');assert.equal(b,undefined);return fn(p) as Promise<T>;}}});}
function deferred(){let resolve!:(v:unknown)=>void;return {promise:new Promise<unknown>(r=>resolve=r),get resolve(){return resolve;}};}
test('bounded server facts preserve unknown audio choice despite stream default flag',async()=>{const s=service(async()=>data());await s.select(q);assert.equal(s.getSnapshot().phase,'ready');assert.equal(s.getSnapshot().data!.current!.audioSelection,'platform_default');assert.equal(s.getSnapshot().data!.sources[0].streams[0].default,true);assert(s.getSnapshot().data!.controls.every(c=>c.id==='quality'?c.enabled:!c.enabled));assert(Object.isFrozen(s.getSnapshot().data!.sources));s.dispose();});
test('a complete offer with more than 32 sources and 128 streams remains readable',async()=>{const d=data();d.sources=Array.from({length:33},(_,i)=>({...d.sources[0],id:`source-${i}`,streams:i===0?Array.from({length:129},(_,n)=>({...d.sources[0].streams[0],index:n})):[]}));d.current.sourceId='source-0';const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'ready');assert.equal(s.getSnapshot().data!.sources.length,33);assert.equal(s.getSnapshot().data!.sources[0].streams.length,129);s.dispose();});
test('same pending query singleflights while newer item wins even when transport ignores abort',async()=>{const old=deferred();let calls=0;const s=service(async p=>{calls++;if(p.includes('/item/'))return old.promise;const d=data();d.scope.itemId='new';return d;});const a=s.select(q);assert.equal(a,s.select(q));const b=s.select({...q,itemId:'new'});await b;old.resolve(data());await a;assert.equal(calls,2);assert.equal(s.getSnapshot().data!.scope.itemId,'new');s.dispose();});
test('cancel and changed principal fence ignored late reads',async()=>{for(const action of ['cancel','scope']){const d=deferred(),s=service(()=>d.promise);const pending=s.select(q);if(action==='cancel')s.cancel();else s.setScope({...scope,viewerId:'other'},{request:async<T>()=>data() as T});d.resolve(data());await pending;assert.equal(s.getSnapshot().phase,'idle');assert.equal(s.getSnapshot().data,null);s.dispose();}});
test('expected offer revision is sent, mismatch refresh-required, explicit refresh drops it',async()=>{const paths:string[]=[];const s=service(async p=>{paths.push(p);return data();});await s.select({...q,revision:'old'});assert.equal(s.getSnapshot().phase,'refresh-required');await s.retry();assert(paths[1].includes('revision=old'));await s.refresh();assert(!paths[2].includes('revision='));assert.equal(s.getSnapshot().phase,'ready');s.dispose();});
test('cross-scope, wrong session/source, oversized/duplicate tracks and unsupported enabled controls fail closed',async()=>{const changes=[(d:any)=>d.scope.serverId='other',(d:any)=>d.scope.libraryId='other',(d:any)=>d.scope.itemId='other',(d:any)=>d.current.sessionId='foreign',(d:any)=>d.current.sourceId='absent',(d:any)=>d.sources=Array(33).fill(d.sources[0]),(d:any)=>d.sources[0].streams.push(d.sources[0].streams[0]),(d:any)=>d.controls[0].enabled=true,(d:any)=>d.controls[3].enabled=false,(d:any)=>d.offersRevision='short',(d:any)=>d.deliveryPolicy.preferenceLane='wifi',(d:any)=>d.deliveryPolicy.clamps=[{field:'maxVideoHeight',source:'server_clamp',requested:720,applied:1080}],(d:any)=>d.current.audioSelection='1',(d:any)=>d.sources[0].duration=Infinity];for(const change of changes){const d=data();change(d);const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().data,null);s.dispose();}});
test('session generation cannot regress and changed viewer fence clears data',async()=>{let d=data();d.current.generation=3;const s=service(async()=>d);await s.select(q);d=data();await s.refresh();assert.equal(s.getSnapshot().phase,'refresh-required');d.scope.viewerFence='different';await s.refresh();assert.equal(s.getSnapshot().error!.code,'permission_changed');assert.equal(s.getSnapshot().data,null);s.dispose();});
test('denied reload clears previous facts, timeout and retry are bounded without command calls',async()=>{let deny=false;const s=service(async()=>{if(deny)throw Object.assign(new Error('private transport content'),{status:403});return data();});await s.select(q);deny=true;await s.refresh();assert.equal(s.getSnapshot().data,null);assert.equal(s.getSnapshot().error!.code,'forbidden');assert(!JSON.stringify(s.getSnapshot()).includes('private transport'));s.dispose();const slow=service(()=>new Promise(()=>{}),5);await slow.select(q);assert.equal(slow.getSnapshot().error!.code,'timeout');slow.dispose();});
test('unbound pre-play read accepts null current while HLS first audio does not invent stream index',async()=>{let d=data();d.current=null;const s=service(async()=>d);await s.select({libraryId:'library',itemId:'item'});assert.equal(s.getSnapshot().phase,'ready');d=data();d.current.delivery='hls';d.current.audioSelection='server_first_audio';d.current.subtitleSelection='disabled';await s.select(q);assert.equal(s.getSnapshot().data!.current!.audioSelection,'server_first_audio');s.dispose();});

test('unknown/stale stream facts cannot masquerade as known and quality offers stay disabled',async()=>{for(const change of [(d:any)=>d.sources[0].factsRevision=0,(d:any)=>d.sources[0].factsStatus='stale',(d:any)=>d.sources[0].qualities=[],(d:any)=>d.sources[0].qualities=[{id:'original',kind:'original',label:'Original',enabled:true}],(d:any)=>d.sources[0].qualities.push({id:'2160p',kind:'fixed',label:'4K',enabled:true,maxVideoBitrateBps:20000000,maxAudioBitrateBps:192000,targetDisplayHeight:2160}),(d:any)=>d.current.qualityId='480p',(d:any)=>d.current.delivery='remote']){const d=data();change(d);const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'error');s.dispose();}const d=data();d.sources[0].factsStatus='stale';d.sources[0].streams=[];const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'ready');s.dispose();});


test('subtitle controls enable only with a validated same-session/source renderer plan',async()=>{
 function planned(){const d=data();d.current.subtitleSelection='off';d.controls[2]={id:'subtitles',enabled:true,reason:''};d.subtitlePlan={version:1,sessionId:'session',generation:1,sourceId:'source',revision:1,catalogRevision:1,renderer:'external_text',mode:'off',offAvailable:true,offsetUs:'0',selected:null,resources:[],discovered:[]};d.subtitlePlanUnavailableReason=null;return d;}
 const good=service(async()=>planned());await good.select(q);assert.equal(good.getSnapshot().phase,'ready');assert.equal(good.getSnapshot().data?.controls[2].enabled,true);good.dispose();
 for(const change of [(d:any)=>d.subtitlePlan=null,(d:any)=>d.subtitlePlan.sessionId='foreign',(d:any)=>d.subtitlePlan.generation=2,(d:any)=>d.subtitlePlan.sourceId='other',(d:any)=>d.current.subtitleSelection='track',(d:any)=>d.subtitlePlan.renderer='invented']){const d=planned();change(d);const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'error');s.dispose();}
});

test('published rungs are parsed with exactly one automatic rung and a fenced revision',async()=>{
 const s=service(async()=>data());await s.select(q);
 const snapshot=s.getSnapshot().data!;
 assert.equal(snapshot.offersRevision.length,64);
 assert.equal(snapshot.deliveryPolicy!.networkClass,'local');
 assert.equal(snapshot.transcodingEnabled,true);
 const rungs=snapshot.sources[0].qualities;
 assert.equal(rungs.filter(r=>r.kind==='automatic').length,1);
 assert.equal(rungs[0].id,'auto');
 // A disabled rung is still published, with the reason the server gave.
 const disabled=rungs.find(r=>r.id==='720p')!;
 assert.equal(disabled.enabled,false);
 assert.equal(disabled.reason,'exceeds_network_policy');
 // Ceilings belong to fixed rungs only.
 assert.equal(rungs.find(r=>r.id==='original')!.targetDisplayHeight,undefined);
 assert.equal(rungs.find(r=>r.id==='1080p')!.targetDisplayHeight,1080);
 assert.equal(snapshot.current!.qualityId,'auto');
 s.dispose();
});

test('a second automatic rung, a rung with contradictory ceilings, or an enabled rung with a reason are refused',async()=>{
 for(const change of [
  (d:any)=>d.sources[0].qualities.push({id:'auto2',kind:'automatic',label:'Automatic',enabled:true}),
  (d:any)=>d.sources[0].qualities[1].targetDisplayHeight=1080,
  (d:any)=>{const r=d.sources[0].qualities[2];delete r.targetDisplayHeight;delete r.maxVideoBitrateBps;delete r.maxAudioBitrateBps;},
  (d:any)=>d.sources[0].qualities[2].reason='exceeds_network_policy',
  (d:any)=>d.sources[0].qualities[0].kind='fixed',
  (d:any)=>d.sources[0].qualities.push({...d.sources[0].qualities[2]}),
 ]){const d=data();change(d);const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'error');s.dispose();}
});

test('audio track labels read like a menu, not a probe dump',async()=>{
 const {audioTrackLabel}=await import('../src/index.ts');
 assert.equal(audioTrackLabel({codec:'eac3',channels:6,language:'en'},1).includes('Dolby Digital Plus 5.1'),true);
 assert.equal(audioTrackLabel({codec:'aac',channels:2,title:'Director commentary'},2).startsWith('Director commentary'),true);
 assert.equal(audioTrackLabel({codec:'dts',language:'und'},3),'Track 3 · DTS');
});
test('retired optional audio payload cannot invalidate quality, source or subtitle offers',async()=>{for(const audioPlan of [{renditions:[]},{renditions:Array(3).fill({})},'invalid']){const d=data();d.current.delivery='hls';d.current.audioSelection='server_first_audio';d.current.subtitleSelection='disabled';d.audioPlan=audioPlan;const s=service(async()=>d);await s.select(q);assert.equal(s.getSnapshot().phase,'ready');assert(s.getSnapshot().data!.sources[0].qualities.length>0);assert.equal('audioPlan' in s.getSnapshot().data!,false);s.dispose();}});
