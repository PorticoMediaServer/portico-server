import {test} from 'node:test';
import assert from 'node:assert/strict';
import {InboxService,FeedbackService} from '../src/inbox.ts';

const notification=(over:Record<string,unknown>={})=>({
 id:'n-1',audience:'profile',severity:'info',source:'downloads',category:'download.finished',
 title:'Download ready',body:'A title finished downloading.',arguments:{},actions:[],
 dedupeKey:'download:dl-1',revision:7,createdAt:1,updatedAt:1,expiresAt:99,
 readAt:null,archivedAt:null,read:false,archived:false,cursor:'3',...over});
const counts=(unread:number)=>({unread,read:0,archived:0,total:unread});
const inbox=(items:unknown[],over:Record<string,unknown>={})=>({scope:{serverId:'s',viewerFence:'f'},data:{
 revision:7,audience:'profile',audiences:['profile','account-admin'],state:'unread',counts:counts(items.length),items,nextCursor:'',observedAt:2,retentionDays:180,...over}});
const summary=(unread:number)=>({data:{revision:7,audience:'profile',counts:counts(unread),observedAt:2}});

function server(){
 const calls:{path:string;method:string;body?:any}[]=[];let fail:Error|undefined;let items=[notification(),notification({id:'n-2',cursor:'4'})];
 const api={request:async<T,>(path:string,method='GET',body?:unknown):Promise<T>=>{
  calls.push({path,method,body});if(fail)throw fail;
  if(path.startsWith('/v1/notifications/unread-count'))return summary(path.includes('account-admin')?3:items.length) as T;
  if(path.startsWith('/v1/notifications/inbox/actions'))return {data:{revision:8,audience:'profile',applied:1,receipts:[{action:(body as any).operations[0].action,outcome:'applied'}],counts:counts(1)}} as T;
  if(path.startsWith('/v1/notifications/inbox'))return inbox(items) as T;
  throw new Error('unexpected '+path);
 }};
 return {api,calls,failWith:(e?:Error)=>{fail=e;},set:(next:any[])=>{items=next;}};
}

test('the inbox loads a view, and the badge counts every audience the viewer has',async()=>{
 const s=server(),service=new InboxService(s.api,()=>'op');
 await service.load();await new Promise(r=>setTimeout(r,5));
 const state=service.getSnapshot();
 assert.equal(state.phase,'ready');assert.equal(state.items.length,2);assert.deepEqual(state.audiences,['profile','account-admin']);
 assert.equal(state.unread,5,'profile 2 + account-admin 3');
});

test('a failed refresh keeps what is on screen; only a first load with nothing is an error',async()=>{
 const s=server(),service=new InboxService(s.api,()=>'op');
 s.failWith(new Error('offline'));await service.load();
 assert.equal(service.getSnapshot().phase,'error');
 s.failWith();await service.load();assert.equal(service.getSnapshot().items.length,2);
 s.failWith(new TypeError('Failed to fetch'));await service.load();
 const state=service.getSnapshot();
 assert.equal(state.phase,'ready','the loaded messages stay');assert.equal(state.items.length,2);assert.match(state.error??'',/can’t reach your server/,'an offline refresh says so, from the catalogue');
});

test('marking read is immediate, leaves the unread view, and is put back if the server refuses',async()=>{
 const s=server(),service=new InboxService(s.api,()=>'op-1');await service.load();
 await service.apply('read',['n-1']);
 assert.deepEqual(service.getSnapshot().items.map(n=>n.id),['n-2']);
 const sent=s.calls.find(c=>c.path==='/v1/notifications/inbox/actions')!;
 assert.deepEqual(sent.body,{operationId:'op-1',expectedRevision:0,audience:'profile',operations:[{action:'read',ids:['n-1']}]});
 assert.equal(service.getSnapshot().revision,8);
 s.failWith(new Error('The server is busy.'));await service.apply('archive',['n-2']);
 const state=service.getSnapshot();
 assert.deepEqual(state.items.map(n=>n.id),['n-2'],'the row comes back');assert.equal(state.error,'That change couldn’t be saved.','catalogue text, never the server message');
});

test('a pushed change merges in; an old revision is ignored; a resync reloads',async()=>{
 const s=server(),service=new InboxService(s.api,()=>'op');await service.load();
 service.accept({revision:6,since:5,resync:false,items:[],counts:counts(9)});
 assert.equal(service.getSnapshot().counts.unread,2,'stale');
 service.accept({revision:9,since:7,resync:false,items:[notification({id:'n-3',cursor:'5',revision:9}) as any],counts:counts(3)});
 assert.equal(service.getSnapshot().items.length,3);assert.equal(service.getSnapshot().revision,9);
 s.set([notification({id:'n-9'})]);service.accept({revision:10,since:9,resync:true,items:[],counts:counts(1)});
 await new Promise(r=>setTimeout(r,5));assert.deepEqual(service.getSnapshot().items.map(n=>n.id),['n-9']);
});

const capabilities={revision:'e1.0',canSubmit:true,kinds:[{id:'playback',label:'Playback',categories:[
 {id:'buffering',label:'Buffering',description:'It keeps pausing.',wantsPlaybackSession:true,wantsItem:true},
 {id:'other',label:'Something else',description:'Anything.',wantsPlaybackSession:false,wantsItem:false}]}],
 maxMessageLength:2000,minMessageLength:8,diagnosticsSupported:true,diagnosticsOptional:true,duplicateWindowHours:24,retentionDays:180,
 statuses:['open','in-progress','resolved','closed'],diagnosticsDecisions:['attached','unavailable','declined','not-requested'],
 perProfileHourlyLimit:10,reporterName:'Sam',reporterAuthority:'local'};
const report={id:'r-1',cursor:'1',revision:1,status:'open',kind:'playback',category:'buffering',message:'It keeps pausing every minute.',createdAt:1,updatedAt:1,expiresAt:9,
 reporter:{name:'Sam',authority:'local',role:'member',self:true},diagnostics:{decision:'attached',reference:'bundle-1'},duplicates:0,thread:[]};

test('feedback is prefilled from the player only where the category wants it, and a failed send loses nothing',async()=>{
 const bodies:any[]=[];let fail=true;
 const api={request:async<T,>(path:string,_m='GET',body?:unknown):Promise<T>=>{
  if(path==='/v1/feedback/capabilities')return {data:capabilities} as T;
  bodies.push(body);if(fail)throw new TypeError('Failed to fetch');return {data:{report,duplicate:false,created:true}} as T;}};
 let n=0;const service=new FeedbackService(api,()=>'op-'+(++n));await service.open();
 assert.equal(service.getSnapshot().phase,'ready');
 const draft={kind:'playback',category:'buffering',message:'It keeps pausing every minute.',itemId:'item-1',playbackSessionId:'play-1',attachDiagnostics:true};
  assert.equal(service.problem({...draft,message:'short'}),'Describe it in at least 8 characters.');
  assert.equal(await service.submit(draft),false);
 assert.equal(service.getSnapshot().phase,'ready','back to the form, draft intact');assert.match(service.getSnapshot().error??'',/can’t reach your server/);
 fail=false;assert.equal(await service.submit(draft),true);
 assert.equal(service.getSnapshot().phase,'sent');
 assert.equal(bodies[0].operationId,bodies[1].operationId,'a retry is the same report, never a second one');
 assert.equal(bodies[1].playbackSessionId,'play-1');assert.equal(bodies[1].itemId,'item-1');
 await service.open();await service.submit({...draft,category:'other'});
 assert.equal(bodies[2].playbackSessionId,'','a category that does not want the session does not get it');assert.equal(bodies[2].itemId,'');
});

test('a profile that may not send feedback is told why',async()=>{
  const api={request:async<T,>():Promise<T>=>({data:{...capabilities,canSubmit:false,submitBlockedReason:'Feedback is turned off for this profile.'}}) as T};
  const service=new FeedbackService(api);await service.open();
  assert.equal(service.getSnapshot().phase,'unavailable');assert.match(service.getSnapshot().error??'',/turned off/);
});

test('CD-46: feedback limits count Unicode code points, trimmed as sent',async()=>{
  const api={request:async<T,>():Promise<T>=>({data:capabilities}) as T};
  const service=new FeedbackService(api);await service.open();
  const draft={kind:'playback',category:'buffering',message:'',itemId:'',playbackSessionId:'',attachDiagnostics:false};
  // 2000 CJK or astral characters are accepted; 2001 are rejected.
  assert.equal(service.problem({...draft,message:'日'.repeat(2000)}),'');
  assert.equal(service.problem({...draft,message:'😀'.repeat(2000)}),'');
  assert.match(service.problem({...draft,message:'日'.repeat(2001)}),/under 2,000/);
  assert.match(service.problem({...draft,message:'😀'.repeat(2001)}),/under 2,000/);
  // 2000 astral characters are 4000 UTF-16 units: length alone must not refuse them.
  assert.equal('😀'.repeat(2000).length,4000);
  // Trimming and the minimum still apply to the text actually sent.
  assert.match(service.problem({...draft,message:'   short '}),/at least 8/);
  assert.equal(service.problem({...draft,message:' 12345678 '}),'');
});

test('badge refreshes share held requests instead of multiplying profile and owner reads',async()=>{
 let release!:(raw:unknown)=>void;let calls=0;
 const service=new InboxService({request:<T,>(path:string):Promise<T>=>{calls++;return new Promise(r=>{release=r as (raw:unknown)=>void;});}});
 const first=service.refreshUnread(),second=service.refreshUnread(),third=service.refreshUnread();
 assert.equal(first,second);assert.equal(first,third);assert.equal(calls,1);
 release(summary(4));assert.equal(await first,4);assert.equal(await second,4);assert.equal(calls,1);service.dispose();
});

test('a changed audience during a held badge refresh reruns once and publishes profile plus owner, never a partial total',async()=>{
 let release!:(raw:unknown)=>void;let badgeCalls=0;
 const service=new InboxService({request:<T,>(path:string):Promise<T>=>{
  if(path.includes('/inbox?'))return Promise.resolve(inbox([])) as Promise<T>;
  badgeCalls++;if(badgeCalls===1)return new Promise(r=>{release=r as (raw:unknown)=>void;});
  return Promise.resolve(summary(path.includes('account-admin')?3:2)) as Promise<T>;
 }});
 const badges:number[]=[];service.subscribe(()=>badges.push(service.getSnapshot().unread));
 const first=service.refreshUnread();await service.load();
 release(summary(100));assert.equal(await first,5);assert.equal(badgeCalls,3);
 assert.ok(!badges.includes(100));assert.equal(service.getSnapshot().unread,5);service.dispose();
});

test('badge deadlines release a held transport and disposal cancels the next read while retaining the last good badge',async()=>{
 let expire! :()=>void;let heldSignal:AbortSignal|undefined;let calls=0;
 const service=new InboxService({request:<T,>(_path:string,_method?:string,_body?:unknown,signal?:AbortSignal):Promise<T>=>{
  calls++;if(calls===1)return Promise.resolve(summary(7)) as Promise<T>;
  heldSignal=signal;return new Promise(()=>{});
 }},()=> 'op',{unreadTimeoutMs:100,setTimer:fn=>{expire=fn;return 1;},clearTimer:()=>{}});
 assert.equal(await service.refreshUnread(),7);
 const timed=service.refreshUnread();expire();assert.equal(await timed,7);assert.equal(heldSignal?.aborted,true);
 const disposed=service.refreshUnread();service.dispose();assert.equal(await disposed,7);assert.equal(heldSignal?.aborted,true);
 assert.equal(await service.refreshUnread(),7);assert.equal(calls,3);
});

test('an event arriving during a badge refresh causes one follow-up read rather than publishing a stale count',async()=>{
 let release!:(raw:unknown)=>void;let calls=0;
 const service=new InboxService({request:<T,>():Promise<T>=>{
  if(++calls===1)return new Promise(r=>{release=r as (raw:unknown)=>void;});
  return Promise.resolve(summary(9)) as Promise<T>;
 }});
 const pending=service.refreshUnread();
 service.accept({revision:8,since:7,resync:false,items:[],counts:counts(9)});
 service.accept({revision:9,since:8,resync:false,items:[],counts:counts(9)});
 release(summary(1));assert.equal(await pending,9);assert.equal(calls,2);service.dispose();
});

test('a failed obsolete audience refresh still follows up for the newly discovered owner audience',async()=>{
 let reject!:(e:Error)=>void;let calls=0;
 const service=new InboxService({request:<T,>(path:string):Promise<T>=>{
  if(path.includes('/inbox?'))return Promise.resolve(inbox([])) as Promise<T>;
  if(++calls===1)return new Promise((_r,no)=>{reject=no;});
  return Promise.resolve(summary(path.includes('account-admin')?3:2)) as Promise<T>;
 }});
 const pending=service.refreshUnread();await service.load();reject(new Error('offline'));
 assert.equal(await pending,5);assert.equal(calls,3);service.dispose();
});
