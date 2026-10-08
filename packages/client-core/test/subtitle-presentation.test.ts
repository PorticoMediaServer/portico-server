import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService} from '../src/index.ts';
import {SubtitleService,validateSubtitlePlan} from '../src/subtitles.ts';
import type {PlaybackApi,PlaybackSession} from '../src/index.ts';
import type {LibraryContentApi} from '../src/library-content.ts';
const session:PlaybackSession={id:'session',generation:1,streamUrl:'/v1/media/original',mode:'direct',duration:100,resumeSeconds:2};
const flush=()=>new Promise<void>(r=>setImmediate(r));
const resource={id:'sub',sourceId:'source',revision:1,scope:'personal',language:'en',title:'ASS',format:'ass',origin:'upload',rights:'Own',offsetUs:'0',canManage:true,enabled:true,renderer:'burn_in',default:false,forced:true} as const;
function plan(revision=1,burned=false):any{return {version:1,sessionId:'session',generation:burned?2:1,sourceId:'source',revision,catalogRevision:1,renderer:burned?'burn_in':'external_text',mode:burned?'track':'off',offAvailable:true,offsetUs:'0',selected:burned?{...resource,pinned:true}:null,resources:[resource],discovered:[],...(burned?{presentation:{sessionId:'session',generation:2,mode:'hls',streamUrl:'/v1/media/new/master.m3u8',positionUs:'2000000'}}:{})};}
const catalog={version:1,itemId:'movie',revision:1,canShare:false,sources:[{id:'source',durationUs:'100000000',inventoryRevision:1,available:true,timingKnown:true}],resources:[resource],discovered:[],provider:{id:'opensubtitles',enabled:false}};
test('subtitle rendition handoff preserves occurrence, pause, pending seek and exact session identity',async()=>{
 let creates=0,stops=0;const api:PlaybackApi={createPlayback:async()=>{creates++;return session;},stopPlayback:async()=>{stops++;},progressPlayback:async()=>{}};
 const player=new PlaybackService(api,()=> 'request');await player.play('movie');const intent=player.getSnapshot().intentId;player.ready(intent);player.pause();player.seek(37);
 const p=plan(2,true).presentation;assert.equal(player.installSubtitlePresentation(p),true);const now=player.getSnapshot();
 assert.equal(creates,1);assert.equal(stops,0);assert.equal(now.intentId,intent);assert.equal(now.session?.id,'session');assert.equal(now.session?.generation,2);assert.equal(now.pendingSeek?.positionSeconds,37);assert.equal(now.intent,'paused');
 assert.equal(player.installSubtitlePresentation(p),true);assert.equal(player.installSubtitlePresentation({...p,generation:1}),false);assert.equal(player.installSubtitlePresentation({...p,sessionId:'other',generation:3}),false);
 player.leave();await flush();
});
test('burn-in wire plans never expose private assets as text documents or origin URLs',()=>{
 const valid=plan(2,true);assert.equal(validateSubtitlePlan(valid).renderer,'burn_in');
 for(const change of [(p:any)=>p.documentUrl='/v1/media/grant/subtitles/sub/1',(p:any)=>p.presentation.streamUrl='https://origin.invalid/private',(p:any)=>p.presentation.generation=3,(p:any)=>p.selected.renderer='external_text']){const p=structuredClone(valid);change(p);assert.throws(()=>validateSubtitlePlan(p));}
});
test('lost burn-in acknowledgement replays the same CAS operation and installs only committed generations',async()=>{
 let current=plan(),failed=false;const writes:any[]=[];const installs:any[]=[];
 const api:LibraryContentApi={request:async(path:string,method='GET',body?:unknown)=>{if(method==='GET')return (path.includes('/playback/')?current:catalog) as any;writes.push(structuredClone(body));current=plan(2,true);if(!failed){failed=true;throw new Error('lost response');}return {...current,appliedRevision:2};}};
 const service=new SubtitleService(api,{itemId:'movie',sessionId:'session',generation:1},()=> 'fixed', {positionUs:()=> '37000000',install:p=>{installs.push(p);return true;}});
 service.start();await flush();await service.choose(resource);assert.equal(service.getSnapshot().canRetry,true);assert.equal(installs.length,0);await service.retry();assert.deepEqual(writes[1],writes[0]);assert.equal(writes[0].positionUs,'37000000');assert.equal(service.getSnapshot().plan?.generation,2);assert(installs.length>=1);service.stop();
});
test('rejected preparation does not suppress the currently burned subtitle video',async()=>{
 const api:LibraryContentApi={request:async(path:string,method='GET')=>{if(method==='GET')return (path.includes('/playback/')?plan(2,true):catalog) as any;throw Object.assign(new Error('preparation failed'),{status:503});}};
 const service=new SubtitleService(api,{itemId:'movie',sessionId:'session',generation:2},()=> 'retry',{positionUs:()=> '2000000',install:()=>true});
 service.start();await flush();await service.choose(null);assert.equal(service.getSnapshot().plan?.renderer,'burn_in');assert.equal(service.getSnapshot().suppressed,false);assert.equal(service.getSnapshot().canRetry,true);service.stop();
});
