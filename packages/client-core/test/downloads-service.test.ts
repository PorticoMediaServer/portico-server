import {test} from 'node:test';
import assert from 'node:assert/strict';
import {DownloadsService} from '../src/downloads-service.ts';

const sha='a'.repeat(64);
const preparation=(over:Record<string,unknown>={}):any=>({
 id:'prep',itemId:'item',libraryId:'lib',profileId:'profile',quality:'1080p',origin:'item',batchId:'batch',state:'ready',reason:'',
 artifact:{kind:'prepared',ref:'version',sha256:sha,bytes:4096,estimated:false,container:'mp4',contentType:'video/mp4',fileName:'prep.mp4'},
 progress:{bytesDone:4096,bytesTotal:4096,bytesTotalEstimated:false,percent:100,etaSeconds:null},
 actions:['remove'],revision:3,createdAt:'2026-09-16T10:00:00Z',updatedAt:'2026-09-16T10:05:00Z',readyAt:'2026-09-16T10:05:00Z',expiresAt:'2026-10-16T10:05:00Z',...over});
const running=()=>preparation({id:'busy',state:'running',artifact:{kind:'',ref:'',sha256:'',bytes:0,estimated:true,container:'',contentType:'',fileName:''},progress:{bytesDone:100,bytesTotal:1000,bytesTotalEstimated:true,percent:10,etaSeconds:60},actions:['pause','cancel'],readyAt:'',expiresAt:''});

function server(items:any[]){
 const calls:{path:string;method:string;body?:any}[]=[];let fail:any;
 const usage={profileId:'profile',profileBytes:0,profileCount:0,serverBytes:0,serverCount:0,distinctArtifactBytes:0,maxPreparedBytes:0,remainingBytes:null,retentionDays:7,profiles:[]};
 return{calls,failWith:(e:any)=>{fail=e;},set:(next:any[])=>{items=next;},api:{request:async<T,>(path:string,method='GET',body?:any):Promise<T>=>{
  calls.push({path,method,body});if(fail)throw fail;
  if(path.startsWith('/v1/downloads/preparations?')){
   const query=new URL(path,'https://server.test').searchParams;
   const state=query.get('state');
   const limit=Number(query.get('limit')??'100');
   const cursor=Number(query.get('cursor')??'0');
   const pool=state?items.filter(p=>p.state===state):items;
   const slice=pool.slice(cursor,cursor+limit);
   return {items:slice,nextCursor:cursor+limit<pool.length?String(cursor+limit):''} as T;
  }
  if(path==='/v1/downloads/usage')return usage as T;
  const single=path.match(/^\/v1\/downloads\/preparations\/([^/]+)$/);
  if(single&&method==='GET'){const row=items.find(p=>p.id===single[1]);if(!row)throw Object.assign(new Error('Not found'),{status:404});return row as T;}
  if(path.endsWith('/actions')){const id=path.split('/')[4];const row=items.find(p=>p.id===id);const next={...row,state:body.action==='pause'?'paused':row.state,actions:['resume','cancel'],revision:row.revision+1};items=items.map(p=>p.id===id?next:p);return next as T;}
  throw new Error('unexpected '+path);}}};
}

test('a failed refresh keeps the list; a server without downloads is a fact, not an error',async()=>{
 const s=server([preparation()]),service=new DownloadsService(s.api,()=>'op',10);
 await service.refresh();assert.equal(service.getSnapshot().items.length,1);
 s.failWith(new TypeError('Failed to fetch'));await service.refresh();
 assert.equal(service.getSnapshot().phase,'ready');assert.equal(service.getSnapshot().items.length,1);assert.ok(service.getSnapshot().error,'a catalogue message');assert.ok(service.getSnapshot().failure,'the error itself is kept for presentError');
 const none=server([]);none.failWith(Object.assign(new Error('nope'),{status:404}));const off=new DownloadsService(none.api,()=>'op',10);
 await off.refresh();assert.equal(off.getSnapshot().unavailable,true);assert.equal(off.getSnapshot().error,undefined);
 service.dispose();off.dispose();
});

test('it polls only while something is still being prepared, and stops when told',async()=>{
 const s=server([running()]),service=new DownloadsService(s.api,()=>'op',15);
 service.start();await new Promise(r=>setTimeout(r,60));
 const polls=s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?')).length;assert.ok(polls>=2,'kept polling while running: '+polls);
 s.set([preparation()]);await new Promise(r=>setTimeout(r,60));
 const settled=s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?')).length;
 await new Promise(r=>setTimeout(r,60));
 assert.equal(s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?')).length,settled,'no polling once everything is ready');
 service.dispose();
});

test('an action carries the revision it was made against, and a conflict shows the current row',async()=>{
 const s=server([running()]),service=new DownloadsService(s.api,()=>'op-7',10000);
 await service.refresh();await service.act(service.getSnapshot().items[0],'pause');
 const sent=s.calls.find(c=>c.path.endsWith('/actions'))!;
 assert.deepEqual(sent.body,{operationId:'op-7',action:'pause',expectedRevision:3});
 assert.equal(service.getSnapshot().items[0].state,'paused');
 await new Promise(r=>setTimeout(r,5));assert.equal(service.getSnapshot().items[0].state,'paused','the follow-up read agrees');
 // A refused action says why, and the list is re-read so it shows what is true now.
 s.set([{...running(),state:'failed',reason:'storage_full',actions:['retry','remove'],revision:9}]);
 const before=s.calls.length;
 s.failWith(Object.assign(new Error('Someone else changed this download.'),{status:409}));
 const acting=service.act(service.getSnapshot().items[0],'resume');s.failWith(undefined);await acting;await new Promise(r=>setTimeout(r,5));
 assert.ok(s.calls.length>before);assert.equal(service.getSnapshot().items[0].state,'failed');assert.equal(service.getSnapshot().items[0].revision,9);
 assert.match(service.reason(service.getSnapshot().items[0]),/./);
 service.dispose();
});

test('PERF-S12: with 1,100 preparations a poll reads only the active pages (<= 2 requests)',async()=>{
 const settled=(i:number)=>preparation({id:'prep-'+i,itemId:'item-'+i,state:'ready',revision:3});
 const runningOne=running();runningOne.id='prep-0';runningOne.itemId='item-0';
 const s=server([runningOne,...Array.from({length:1099},(_,i)=>settled(i+1))]);
 const service=new DownloadsService(s.api,()=>'op',20);
 service.start();
 await new Promise(r=>setTimeout(r,10));
 // The list itself is paged: the first page only, never 10x100 up front.
 assert.equal(service.getSnapshot().items.length,100);
 assert.ok(service.getSnapshot().nextCursor,'more pages remain');
 await new Promise(r=>setTimeout(r,100));
 const pages=()=>s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?'));
 const polls=pages().filter(c=>c.path.includes('state='));
 // Every poll is exactly the two active-state pages; the only unfiltered read is the first page.
 assert.ok(polls.length>=4,'polled while active: '+polls.length);
 assert.ok(polls.length%2===0,'polls come in queued+running pairs: '+polls.length);
 assert.ok(polls.every(c=>c.path==='/v1/downloads/preparations?limit=100&state=queued'||c.path==='/v1/downloads/preparations?limit=100&state=running'));
 assert.equal(pages().length-polls.length,1,'one unfiltered first page, never a full re-read');
 // Finish it server-side: one single read settles it, then polling stops (<= 2 per poll held).
 s.set([settled(0),...Array.from({length:1099},(_,i)=>settled(i+1))]);
 await new Promise(r=>setTimeout(r,80));
 assert.equal(service.getSnapshot().items.find(p=>p.id==='prep-0')?.state,'ready');
 const settledCount=pages().length;
 await new Promise(r=>setTimeout(r,60));
 assert.equal(pages().length,settledCount,'polling stopped once nothing was active');
 service.dispose();
});

test('PERF-S12: every preparation is reachable by paging, and a finished one settles without a reload',async()=>{
 const item=(i:number,state:string)=>preparation({id:'prep-'+i,itemId:'item-'+i,state,revision:3,artifact:state==='ready'?preparation().artifact:running().artifact,progress:state==='ready'?preparation().progress:running().progress,actions:state==='ready'?['remove']:['pause','cancel'],readyAt:state==='ready'?'2026-09-16T10:05:00Z':'',expiresAt:state==='ready'?'2026-10-16T10:05:00Z':''});
 const rows=[item(0,'running'),...Array.from({length:1099},(_,i)=>item(i+1,'ready'))];
 const s=server(rows);
 const service=new DownloadsService(s.api,()=>'op',15);
 await service.refresh();
 for(let page=0;page<10;page++)await service.loadMore();
 assert.equal(service.getSnapshot().items.length,1100,'all 1,100 reachable by paging');
 assert.equal(service.getSnapshot().nextCursor,'','no silent truncation left');
 assert.equal(new Set(service.getSnapshot().items.map(p=>p.id)).size,1100,'no duplicates across pages');
 // Follow the list while one preparation is still running, then finish it server-side: the poll
 // re-reads it by id and stops, with no full reload.
 service.start();
 await new Promise(r=>setTimeout(r,40));
 s.set([item(0,'ready'),...rows.slice(1)]);
 await new Promise(r=>setTimeout(r,80));
 assert.equal(service.getSnapshot().items.find(p=>p.id==='prep-0')?.state,'ready','the finished preparation settled via the poll');
 const pages=s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?'));
 assert.ok(pages.length>0);
 const unfiltered=pages.filter(c=>!c.path.includes('state='));
 assert.equal(unfiltered.length,12,'only the refresh/loadMore reads are unfiltered pages (1 + 10 + start refresh); every poll reads active states');
 const singles=s.calls.filter(c=>/^\/v1\/downloads\/preparations\/[^/]+$/.test(c.path));
 assert.ok(singles.length>=1&&singles.length<=3,'the transition took only single reads: '+singles.length);
 const settled=pages.length;
 await new Promise(r=>setTimeout(r,60));
 assert.equal(s.calls.filter(c=>c.path.startsWith('/v1/downloads/preparations?')).length,settled,'polling stopped once nothing was active');
 service.dispose();
});

test('a whole-container request is tracked from capture to completion',async()=>{
 const view=(over:Record<string,unknown>={})=>({requestId:'req-1',total:0,totalKnown:false,state:'capturing',ready:0,preparing:0,queued:0,failed:0,paused:0,items:[],nextCursor:'',...over});
 let reads=0;
 const api={request:async<T,>(path:string,method='GET',body?:any):Promise<T>=>{
  if(path==='/v1/downloads/requests'&&method==='POST'){
   assert.deepEqual(body,{operationId:'op-1',target:{kind:'season',id:'s-1'},deviceId:'dev-1',quality:'original',policy:{episodes:'next',keepNext:3}});
   return view() as T;
  }
  if(path.startsWith('/v1/downloads/requests/req-1')){
   reads++;
   return (reads<2?view():view({state:'complete',total:2,totalKnown:true,ready:2})) as T;
  }
  if(path.startsWith('/v1/downloads/preparations?'))return {items:[],nextCursor:''} as T;
  if(path==='/v1/downloads/usage')return {profileId:'profile',profileBytes:0,profileCount:0,serverBytes:0,serverCount:0,distinctArtifactBytes:0,maxPreparedBytes:0,remainingBytes:null,retentionDays:7,profiles:[]} as T;
  throw new Error('unexpected '+method+' '+path);}};
 const service=new DownloadsService(api,()=>'op-1',15);
 const created=await service.requestContainer({target:{kind:'season',id:'s-1',title:'Season 1'},deviceId:'dev-1',quality:'original',policy:{episodes:'next',keepNext:3}});
 assert.equal(created.requestId,'req-1');
 assert.equal(service.getSnapshot().requests.length,1);
 assert.equal(service.getSnapshot().requests[0].title,'Season 1');
 assert.equal(service.getSnapshot().requests[0].request.totalKnown,false);
 await new Promise(r=>setTimeout(r,80));
 assert.equal(service.getSnapshot().requests[0].request.state,'complete');
 assert.equal(service.getSnapshot().requests[0].request.ready,2);
 await assert.rejects(service.requestContainer({target:{kind:'episode' as any,id:'e'},deviceId:'dev-1',quality:'original',policy:{episodes:'all'}}));
 service.dispose();
});

test('an ordinary list refresh does not stop following a whole-container request',async()=>{
 const view=(over:Record<string,unknown>={})=>({requestId:'req-1',total:0,totalKnown:false,state:'capturing',ready:0,preparing:0,queued:0,failed:0,paused:0,items:[],nextCursor:'',...over});
 let reads=0;
 const api={request:async<T,>(path:string,method='GET'):Promise<T>=>{
  if(path==='/v1/downloads/requests'&&method==='POST')return view() as T;
  if(path.startsWith('/v1/downloads/requests/req-1')){reads++;return (reads<4?view({state:'preparing',total:2,totalKnown:true}):view({state:'complete',total:2,totalKnown:true,ready:2})) as T;}
  if(path.startsWith('/v1/downloads/preparations?'))return {items:[],nextCursor:''} as T;
  if(path==='/v1/downloads/usage')return {profileId:'profile',profileBytes:0,profileCount:0,serverBytes:0,serverCount:0,distinctArtifactBytes:0,maxPreparedBytes:0,remainingBytes:null,retentionDays:7,profiles:[]} as T;
  throw new Error('unexpected '+method+' '+path);}};
 const service=new DownloadsService(api,()=>'op-1',15);
 await service.requestContainer({target:{kind:'season',id:'s-1',title:'Season 1'},deviceId:'dev-1',quality:'original',policy:{episodes:'all'}});
 // Opening the Downloads screen refreshes the list (a new generation).
 await service.refresh();
 await service.refresh();
 await new Promise(r=>setTimeout(r,150));
 assert.equal(service.getSnapshot().requests[0].request.state,'complete','the request kept being followed to completion');
 service.dispose();
});
