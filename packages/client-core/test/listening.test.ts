import test from 'node:test';
import assert from 'node:assert/strict';
import {ListeningService,loadListeningSelection,listeningItemInput,parseListeningJourney,parseListeningPreferences,readListeningItem,readListeningContext,type ListeningRequest} from '../src/listening.ts';
import type {PlaybackService} from '../src/index.ts';
const scope={serverId:'server',authority:'local' as const,accountId:'account',profileId:'profile',controllerId:'controller',controllerEpoch:'epoch',commandLaneId:'lane'};
const prefs={revision:0,musicRate:1,bookRate:1.5,autoplayNext:true,passoutMinutes:0};
const wrapped=(data:unknown,other={})=>({scope:{...scope,...other},data});
const settle=()=>new Promise<void>(resolve=>setImmediate(resolve));
const target={libraryId:'library',kind:'album' as const,id:'album'};
class Engine {
 state:any={itemId:'song',intent:'playing',phase:'ready',positionSeconds:5,session:{id:'session',generation:1},intentId:1};
 policies:any[]=[];native=false;setListeningPolicy=(policy:any)=>{this.policies.push(policy);};isNativeListening=()=>this.native;
 listeners=new Set<()=>void>();rates:number[]=[];pauses=0;
 subscribe=(f:()=>void)=>{this.listeners.add(f);return()=>{this.listeners.delete(f);};};
 getSnapshot=()=>this.state;
 setPlaybackRate=(rate:number)=>{this.rates.push(rate);};
 pause=()=>{this.pauses++;this.update({intent:'paused'});};
 update(patch:unknown){this.state={...this.state,...patch as any};for(const f of this.listeners)f();}
}
function chapterData(){return {scope:{serverId:'server',libraryId:'library',itemId:'song',sessionId:'session',generation:1,sourceId:'source',viewerFence:'fence'},revision:'revision',status:'available',reason:null,duration:60,chapters:[{id:'source:1',index:1,title:'One',startSeconds:0,endSeconds:30,action:{id:'seek',positionSeconds:0}},{id:'source:2',index:2,title:'Two',startSeconds:30,endSeconds:60,action:{id:'seek',positionSeconds:30}}],totalCount:2,nextCursor:''};}
function setup(options:{preferences?:Partial<typeof prefs>;context?:(id:string)=>unknown;request?:ListeningRequest}={}) {
 const engine=new Engine();let now=0,p={...prefs,...options.preferences};const writes:unknown[]=[];
 const request:ListeningRequest=options.request??(async(path,body,method)=>{
  if(path==='/v1/listening/preferences'){if(method==='PUT'){writes.push(body);p={...body as typeof p,revision:p.revision+1};}return wrapped(p);}
  if(path.includes('/chapters'))return chapterData();
  const id=path.split('/')[3];return wrapped(options.context?.(id)??{itemId:id,libraryId:'library',kind:'song',albumId:'album',artistId:'artist'});
 });
 const service=new ListeningService(scope,request,engine as unknown as PlaybackService,()=>now);
 service.connect();return {service,engine,writes,setNow:(v:number)=>{now=v;}};
}
test('preferences strictly validate rates, revision and bounded inactivity choices; scope is never caller-switchable',async()=>{
 assert.deepEqual(parseListeningPreferences(prefs),prefs);
 for(const patch of [{revision:-1},{bookRate:NaN},{musicRate:3},{autoplayNext:'true'},{passoutMinutes:1}])assert.throws(()=>parseListeningPreferences({...prefs,...patch}));
 assert.throws(()=>parseListeningJourney({target:{...target,libraryId:'other'},actions:['play'],resumeUnavailable:false},'library'));
 await assert.rejects(readListeningItem(async()=>wrapped({itemId:'song',libraryId:'library',kind:'song'},{profileId:'other'}),scope,'song'));
 await assert.rejects(readListeningItem(async()=>wrapped({itemId:'song',libraryId:'library',kind:'song',bookId:''}),scope,'song'));
});
test('profile music/book rate is applied to current and replacement engines; saves use CAS revision',async t=>{
 const f=setup({context:id=>({itemId:id,libraryId:'library',kind:id==='song'?'song':'audiobook_file'})});t.after(()=>f.service.dispose());await settle();
 await f.service.setRate(1.25);assert.equal(f.engine.rates.at(-1),1.25);assert.equal((f.writes[0] as any).revision,0);
 f.engine.update({itemId:'part',session:{id:'book-session',generation:1}});await settle();assert.equal(f.engine.rates.at(-1),1.5);
 f.engine.update({session:{id:'book-session',generation:2}});assert.equal(f.engine.rates.at(-1),1.5);
 assert.equal(f.service.getSnapshot().preferences?.musicRate,1.25);
});
test('minute timer pauses the real foreground player once; manual interaction resets stop latch',async t=>{
 const f=setup();t.after(()=>f.service.dispose());await settle();await f.service.setTimer(15);
 f.setNow(899999);f.service.tick();assert.equal(f.engine.pauses,0);
 f.setNow(900000);f.service.tick();f.service.tick();assert.equal(f.engine.pauses,1);assert.equal(f.service.mayAdvance(),false);assert.equal(f.service.getSnapshot().timer,null);
 f.service.interact();assert.equal(f.service.mayAdvance(),true);
});
test('inactivity preference is enforced and deliberate foreground interactions reset its deadline',async t=>{
 const f=setup({preferences:{passoutMinutes:30}});t.after(()=>f.service.dispose());await settle();
 f.setNow(29*60000);f.service.interact();f.setNow(58*60000);f.service.tick();assert.equal(f.engine.pauses,0);
 f.setNow(59*60000);f.service.tick();assert.equal(f.engine.pauses,1);assert.match(f.service.getSnapshot().message!,/without interaction/);
});
test('autoplay false blocks only automatic advancement, not explicit preferences or manual controls',async t=>{
 const f=setup({preferences:{autoplayNext:false}});t.after(()=>f.service.dispose());await settle();assert.equal(f.service.mayAdvance(),false);
 f.service.interact();assert.equal(f.service.mayAdvance(),false);await f.service.savePreferences({autoplayNext:true});assert.equal(f.service.mayAdvance(),true);
});
test('item timer requires an actual ended observation and book timer survives ordered parts',async t=>{
 const f=setup({context:id=>({itemId:id,libraryId:'library',kind:'audiobook_file',bookId:'book',lastBookItemId:'last',bookEndAvailable:true})});t.after(()=>f.service.dispose());await settle();
 await f.service.setTimer('book');f.engine.update({phase:'ended'});assert.equal(f.engine.pauses,0);
 f.engine.update({itemId:'last',phase:'ready',session:{id:'last-session',generation:1}});await settle();assert.equal(f.service.getSnapshot().timer?.choice,'book');
 f.engine.update({phase:'ended'});assert.equal(f.engine.pauses,1);assert.equal(f.service.mayAdvance(),false);
 f.service.interact();f.engine.update({phase:'ready',intent:'playing'});await f.service.setTimer('item');f.engine.update({positionSeconds:10000});assert.equal(f.engine.pauses,1);
 f.engine.update({phase:'ended'});assert.equal(f.engine.pauses,2);
});
test('an unavailable final part never invents an end-of-book timer',async t=>{
 const f=setup({context:id=>({itemId:id,libraryId:'library',kind:'audiobook_file',bookId:'book',lastBookItemId:'missing',bookEndAvailable:false})});t.after(()=>f.service.dispose());await settle();
 await assert.rejects(f.service.setTimer('book'),/final book part is unavailable/);assert.equal(f.service.getSnapshot().timer,null);await f.service.setTimer(30);assert.equal(f.service.getSnapshot().timer?.choice,30);
});
test('chapter timer uses source/session timing and does not stop on unsettled seeks',async t=>{
 const f=setup();t.after(()=>f.service.dispose());await settle();await f.service.setTimer('chapter');assert.equal(f.service.getSnapshot().timer?.endSeconds,30);
 f.engine.update({positionSeconds:35,pendingSeek:{revision:1}});assert.equal(f.engine.pauses,0);
 f.engine.update({pendingSeek:undefined});assert.equal(f.engine.pauses,1);assert.equal(f.service.mayAdvance(),false);
});
test('chapter timers clear on source generation change, while minute timers clear across named libraries',async t=>{
 const f=setup({context:id=>({itemId:id,libraryId:id==='song'?'library':'other',kind:'song'})});t.after(()=>f.service.dispose());await settle();
 await f.service.setTimer('chapter');f.engine.update({session:{id:'session',generation:2}});assert.equal(f.service.getSnapshot().timer,null);
 await f.service.setTimer(30);f.engine.update({itemId:'other-song'});await settle();assert.equal(f.service.getSnapshot().timer,null);assert.equal(f.engine.pauses,0);
});
test('late old-profile reads and disposed clocks cannot publish or pause',async()=>{
 let resolve!:(v:unknown)=>void;const f=setup({request:()=>new Promise(r=>{resolve=r;})});f.service.dispose();resolve(wrapped(prefs));await settle();f.setNow(99*60000);f.service.tick();
 assert.equal(f.service.getSnapshot().phase,'disposed');assert.equal(f.service.getSnapshot().context,null);assert.equal(f.engine.pauses,0);
});
test('save conflict retains actual preference and demands explicit reload; context retry is real',async t=>{
 let failedContext=true;const f=setup({request:async(path,_body,method)=>{if(path==='/v1/listening/preferences'){if(method==='PUT')throw Object.assign(new Error('conflict'),{code:'listening_preferences_conflict'});return wrapped(prefs);}if(failedContext)throw new Error('offline');return wrapped({itemId:'song',libraryId:'library',kind:'song'});}});t.after(()=>f.service.dispose());await settle();
 assert.equal(f.service.getSnapshot().context,null);failedContext=false;f.service.retryContext();await settle();assert.equal(f.service.getSnapshot().context?.itemId,'song');
 await assert.rejects(f.service.savePreferences({autoplayNext:false}),/changed on another device/);assert.equal(f.service.getSnapshot().preferences?.autoplayNext,true);assert.equal(f.service.getSnapshot().phase,'error');await f.service.refreshPreferences();assert.equal(f.service.getSnapshot().phase,'ready');
});
test('full listening action reads every signed page, preserving server order and mix seed',async()=>{
 const items=Array.from({length:205},(_,i)=>listeningItemInput(`song-${i}`,target));let calls=0;
 const result=await loadListeningSelection(async path=>{const q=new URL(path,'https://server.test').searchParams;const at=Number(q.get('cursor')??0);calls++;if(at)assert.equal(q.get('seed'),'server-seed');return wrapped({target,entries:items.slice(at,at+100),totalCount:205,unavailableCount:3,nextCursor:at+100<205?String(at+100):'',seed:'server-seed'});},scope,target,{mix:true});
 assert.equal(calls,3);assert.deepEqual(result.entries,items);assert.equal(result.unavailableCount,3);assert.equal(result.seed,'server-seed');
});
test('selection refuses partial lists, repeated cursors, scope changes and queue overflow without a mutation',async()=>{
 const data={target,entries:[listeningItemInput('song',target)],totalCount:2,unavailableCount:0,nextCursor:'cursor',seed:''};
 await assert.rejects(loadListeningSelection(async()=>wrapped(data),scope,target,{capacity:1}),/room for 1/);
 await assert.rejects(loadListeningSelection(async()=>wrapped(data),scope,target));
 await assert.rejects(loadListeningSelection(async()=>wrapped({...data,nextCursor:''}),scope,target));
 await assert.rejects(loadListeningSelection(async()=>wrapped({...data,totalCount:1,nextCursor:''},{accountId:'other'}),scope,target));
 const abort=new AbortController();abort.abort();let calls=0;await assert.rejects(loadListeningSelection(async()=>{calls++;return wrapped(data);},scope,target,{signal:abort.signal}));assert.equal(calls,0);
});
test('book resume and explicit chapter selection preserve server part order and absolute source start',async()=>{
 const book={libraryId:'library',kind:'book' as const,id:'book'};let path='';const data={target:book,entries:[listeningItemInput('part-2',book),listeningItemInput('part-3',book)],totalCount:2,unavailableCount:0,nextCursor:'',seed:'',resume:{itemId:'part-2',positionSeconds:123.5}};
 const r=await loadListeningSelection(async p=>{path=p;return wrapped(data);},scope,book,{resume:true});assert.match(path,/resume=true/);assert.equal(r.startSeconds,123.5);assert.equal(r.entries[0].itemId,'part-2');
 await loadListeningSelection(async p=>{path=p;return wrapped({...data,resume:undefined});},scope,book,{startItem:'part-2'});assert.match(path,/startItem=part-2/);
});

test('native policy has stable timer/activity revisions; observations cannot extend deadlines',async t=>{
 const f=setup();t.after(()=>f.service.dispose());await settle();
 await f.service.setTimer(15);const armed=f.engine.policies.at(-1);assert.ok(armed.timer.wallDeadline>Date.now());
 f.engine.update({positionSeconds:10});f.service.tick();
 const observed=f.engine.policies.at(-1);assert.equal(observed.timerRevision,armed.timerRevision);assert.equal(observed.timer.wallDeadline,armed.timer.wallDeadline);assert.equal(observed.activityRevision,armed.activityRevision);
 f.service.interact();const interacted=f.engine.policies.at(-1);assert.ok(interacted.activityRevision>armed.activityRevision);assert.equal(interacted.timer.wallDeadline,armed.timer.wallDeadline);
 await f.service.setTimer('off');assert.ok(f.engine.policies.at(-1).timerRevision>armed.timerRevision);assert.equal(f.engine.policies.at(-1).timer,null);
});
test('native owns expiry and rate application; stale timer receipts cannot clear a replacement',async t=>{
 const f=setup();t.after(()=>f.service.dispose());await settle();f.engine.native=true;
 f.engine.update({nativeListening:{timerStopped:false,expiredTimerRevision:-1}});
 await f.service.setTimer(15);const old=f.engine.policies.at(-1).timerRevision;
 await f.service.setTimer(30);const current=f.engine.policies.at(-1).timerRevision;
 const rates=f.engine.rates.length;await f.service.setRate(1.75);assert.equal(f.engine.rates.length,rates,'no duplicate JS rate writer for a native occurrence');
 f.setNow(9999999);f.service.tick();assert.equal(f.engine.pauses,0,'JS must not run the native deadline engine');
 f.engine.update({nativeListening:{timerStopped:true,expiredTimerRevision:old}});assert.equal(f.service.getSnapshot().timer?.choice,30);
 f.engine.update({nativeListening:{timerStopped:true,expiredTimerRevision:current}});assert.equal(f.service.getSnapshot().timer,null);assert.equal(f.service.mayAdvance(),false);assert.equal(f.engine.pauses,0);
 f.engine.update({nativeListening:{timerStopped:false,expiredTimerRevision:current}});assert.equal(f.service.getSnapshot().stopped,false);
});
test('transition admission respects final-item, verified chapter, minute and inactivity edges',async t=>{
 const f=setup({preferences:{passoutMinutes:30}});t.after(()=>f.service.dispose());await settle();
 assert.equal(f.service.mayTransition(10),true);assert.equal(f.service.mayTransition(NaN),false);
 await f.service.setTimer('item');assert.equal(f.service.mayTransition(1),false);
 await f.service.setTimer('chapter');assert.equal(f.service.mayTransition(1),false);
 await f.service.setTimer(15);f.setNow(899000);assert.equal(f.service.mayTransition(2),false);assert.equal(f.service.mayTransition(.1),true);
 await f.service.setTimer('off');f.service.interact();f.setNow(899000+1800000-1000);assert.equal(f.service.mayTransition(2),false);
});
test('end-of-book policy permits intermediate parts but prevents a prebuffered final-part escape',async t=>{
 const f=setup({context:id=>({itemId:id,libraryId:'library',kind:'audiobook_file',bookId:'book',lastBookItemId:'last',bookEndAvailable:true})});t.after(()=>f.service.dispose());await settle();await f.service.setTimer('book');assert.equal(f.service.mayTransition(2),true);
 f.engine.update({itemId:'last',session:{id:'last-session',generation:1}});await settle();assert.equal(f.service.mayTransition(2),false);
});

test('browsing listening entities needs no queue and rejects cross-viewer or cross-item responses',async()=>{
 const bound={serverId:scope.serverId,viewerId:JSON.stringify([scope.authority,scope.accountId,scope.profileId])};
 const data={itemId:'song',libraryId:'library',kind:'song',albumId:'album',artistId:'artist'};
 assert.deepEqual(await readListeningContext(async()=>wrapped(data),bound,'song'),data);
 for(const change of [{serverId:'other'},{accountId:'other'},{profileId:'other'},{authority:'hosted'}])await assert.rejects(readListeningContext(async()=>wrapped(data,change),bound,'song'));
 await assert.rejects(readListeningContext(async()=>wrapped({...data,itemId:'other'}),bound,'song'));
});

test('PERF-24: a video session never asks for listening details; a listening one does, once its session is known',async()=>{
 const engine=new Engine();const paths:string[]=[];
 const request:ListeningRequest=async(path)=>{paths.push(path);if(path==='/v1/listening/preferences')return wrapped(prefs);const id=path.split('/')[3];return wrapped({itemId:id,libraryId:'library',kind:'song',albumId:'album',artistId:'artist'});};
 engine.state={...engine.state,itemId:'movie',session:{id:'s_movie',generation:1}};
 const service=new ListeningService(scope,request,engine as unknown as PlaybackService,()=>0,(itemId,sessionId)=>!(itemId==='movie'&&sessionId==='s_movie'));
 service.connect();await settle();
 assert.equal(paths.filter(p=>p.includes('/items/')).length,0,'no /listening for the movie');
 engine.update({itemId:'song',session:undefined});await settle();
 assert.equal(paths.filter(p=>p.includes('/items/')).length,0,'waits for the song’s session');
 engine.update({session:{id:'s_song',generation:1}});await settle();
 assert.deepEqual(paths.filter(p=>p.includes('/items/')),['/v1/items/song/listening']);
 service.dispose();
});

// P1 (web, 24 Sep): an album stopped after track 1 when the listening preferences hadn't been
// read (or couldn't be). Unknown preferences count as the server default (autoplay on).
test('a song advances before the listening preferences are known', async t => {
 const f=setup({request:async(path)=>{if(path==='/v1/listening/preferences')throw Object.assign(new Error('unavailable'),{status:503});const id=path.split('/')[3];return wrapped({itemId:id,libraryId:'library',kind:'song',albumId:'album',artistId:'artist'});}});
 t.after(()=>f.service.dispose());await settle();await settle();
 assert.equal(f.service.getSnapshot().preferences,null);
 assert.equal(f.service.mayAdvance(),true);
});
