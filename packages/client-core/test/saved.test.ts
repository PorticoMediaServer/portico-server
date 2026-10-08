import test from 'node:test';
import assert from 'node:assert/strict';
import {SavedService} from '../src/saved.ts';
const scope={serverId:'server',viewerId:'["local","account","profile"]'};
const tick=()=>new Promise(r=>setTimeout(r,0));
const deferred=()=>{let resolve!:(v:any)=>void;const promise=new Promise<any>(r=>resolve=r);return{promise,resolve};};
const media=(id='movie')=>({id,libraryId:'library',kind:'movie',title:'Movie',available:true,playback:{itemId:id,startSeconds:0}});
const occurrence=(id='entry')=>({id,kind:'playlist_entry',hidden:false,media:media()});
function savedFilters(count=3){return [{id:'filter',labelKey:'saved.filter',options:[{id:'all',label:'All',count},{id:'unwatched',label:'Unwatched',count},{id:'inProgress',label:'In progress',count}]}];}
function projection(view='playlist',revision=0,entries:any[]=[occurrence()],cursor='',category?:string){
 const listView=view==='watchlist'||view==='favorites';
 const resolved=category??(listView?'all':'');
 return {scope:{serverId:'server',libraryId:'',libraryKind:'mixed',view,entityId:view==='playlist'?'playlist':'',viewerFence:'fence'},revision:{catalog:revision,viewer:0},...(view==='playlist'?{playlistRevision:revision}:{}),...(view==='playlists'?{actions:['create_playlist']}:{}),heading:{key:'saved.title',fallback:'Saved'},navigation:[{id:'watchlist',labelKey:'saved.watchlist',view:'watchlist'},{id:'favorites',labelKey:'saved.favorites',view:'favorites'},{id:'playlists',labelKey:'saved.playlists',view:'playlists'}],query:{sort:view==='playlist'?'position':'title',direction:'asc',category:resolved,q:'',limit:40,searchMode:'none'},sorts:[],filters:listView?savedFilters(Math.max(3,entries.length)):[],sections:[{id:'entries',type:'list',heading:{key:'saved.entries',fallback:'Entries'},entries,totalCount:Math.max(3,entries.length),nextCursor:cursor}]};
}
const resource=(revision=0,entryCount=1)=>({serverId:'server',viewerFence:'fence',id:'playlist',name:'Playlist',summary:'Summary',revision,role:'owner',entryCount,actions:['rename','summary','delete','share','add','remove','reorder'],limits:{maxEntries:1000,maxShares:100,maxReorderEntries:1000},shares:[]});
const receipt=(body:any,revision=1)=>({serverId:'server',viewerFence:'fence',operationId:body.operationId,playlistId:'playlist',revision,deleted:false,entryId:'new-entry'});
function setup(request:(path:string,method?:string,body?:any,signal?:AbortSignal)=>Promise<any>,timeoutMs=200){let i=0;return new SavedService({scope,api:{request},requestId:async()=>`00000000-0000-4000-8000-${String(++i).padStart(12,'0')}`,timeoutMs});}
const isContent=(p:string)=>p.includes('/content');

test('server occurrence order preserves duplicate media and opaque hidden rows',async()=>{
 const entries=[occurrence('one'),{id:'hidden',kind:'playlist_entry',hidden:true},occurrence('two')];
 const s=setup(async p=>isContent(p)?projection('playlist',0,entries):resource(0,3));await s.select({view:'playlist',playlistId:'playlist'});
 assert.equal(s.getSnapshot().phase,'ready');assert.deepEqual(s.getSnapshot().projection!.sections[0].entries,entries);assert(Object.isFrozen(s.getSnapshot().projection!.sections[0].entries));
});
test('hidden metadata leakage and duplicate occurrence IDs are rejected',async()=>{
 for(const entries of [[{...occurrence(),hidden:true}],[{id:'e',kind:'playlist_entry',hidden:true,title:'Secret'}],[occurrence(),occurrence()]]){const s=setup(async p=>isContent(p)?projection('playlist',0,entries):resource());await s.select({view:'playlist',playlistId:'playlist'});assert.equal(s.getSnapshot().error!.code,'invalid_saved');}
});
test('global requests consume server order and declared query without local ranking',async()=>{
 let path='';const s=setup(async p=>{path=p;const d=projection('favorites',0,[media('z'),media('a')]);d.query.sort='added';d.query.direction='desc';return d;});await s.select({view:'favorites',sort:'added',direction:'desc'});
 assert.equal(path,'/v1/content?limit=40&view=favorites&sort=added&direction=desc');assert.deepEqual(s.getSnapshot().projection!.sections[0].entries.map(e=>e.id),['z','a']);
});
test('playlist resource/content torn revision never authorizes reorder',async()=>{
 const s=setup(async p=>isContent(p)?projection():resource(1));await s.select({view:'playlist',playlistId:'playlist'});assert.equal(s.getSnapshot().phase,'refresh-required');assert.equal(s.getSnapshot().playlist,null);assert.throws(()=>s.mutate({action:'reorder',entryIds:['entry']}));
});
test('scope changes abort stale reads even when the adapter ignores abort',async()=>{
 const old=deferred();const s=setup(async()=>old.promise);const first=s.select({view:'watchlist'});s.setScope({...scope,viewerId:'other'},{request:async<T>()=>projection('favorites',0,[]) as T});await s.select({view:'favorites'});old.resolve(projection('watchlist',0,[media()]));await first;assert.equal(s.getSnapshot().projection!.scope.view,'favorites');
});
test('one-page continuations reject stale revision and refresh returns first page',async()=>{
 let revision=0;const requests:string[]=[];const s=setup(async p=>{requests.push(p);return projection('watchlist',revision,[media()], 'next');});await s.select({view:'watchlist'});revision=1;await s.next('entries');assert.equal(s.getSnapshot().phase,'refresh-required');assert.equal(s.getSnapshot().projection!.revision.catalog,0);await s.retry();assert(!requests.at(-1)!.includes('cursor'));assert.equal(s.getSnapshot().projection!.revision.catalog,1);
});
test('failed page retry repeats the requested cursor and successful Back restores prior page',async()=>{
 let fail=true;const s=setup(async p=>{if(p.includes('cursor')&&fail){fail=false;throw new Error('network');}return projection('watchlist',0,[media(p.includes('cursor')?'second':'first')],p.includes('cursor')?'':'next');});await s.select({view:'watchlist'});await s.next('entries');assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'first');await s.retry();assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'second');await s.previous();assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'first');
});
test('deliberate duplicate Adds are separate occurrences; serialized CAS uses refreshed revisions',async()=>{
 let revision=0;const writes:any[]=[];const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection('playlist',revision):resource(revision);writes.push(b);return receipt(b,++revision);});await s.select({view:'playlist',playlistId:'playlist'});const a=s.mutate({action:'add',itemId:'movie'});const b=s.mutate({action:'add',itemId:'movie'});await Promise.all([a,b]);await tick();assert.equal(writes.length,2);assert.notEqual(writes[0].operationId,writes[1].operationId);assert.deepEqual(writes.map(x=>x.expectedRevision),[0,1]);
});
test('lost mutation response retries identical operation/body rather than adding twice',async()=>{
 const writes:any[]=[];const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection():resource();writes.push(b);if(writes.length===1)return new Promise(()=>{});return receipt(b);},10);await s.select({view:'playlist',playlistId:'playlist'});await assert.rejects(s.mutate({action:'add',itemId:'movie'}),/timed out/);assert.deepEqual(s.getSnapshot().pending,[]);assert.equal(s.getSnapshot().mutationError!.code,'timeout');await s.retryMutation();await tick();assert.deepEqual(writes[0],writes[1]);assert.equal(s.getSnapshot().lastReceipt!.entryId,'new-entry');
});
test('CAS conflict refreshes resource and explicit retry uses a new operation and revision',async()=>{
 let revision=0;const writes:any[]=[];const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection('playlist',revision):resource(revision);writes.push(b);if(writes.length===1){revision=4;throw Object.assign(new Error('Changed'),{code:'playlist_conflict'});}return receipt(b,++revision);});await s.select({view:'playlist',playlistId:'playlist'});await assert.rejects(s.mutate({action:'rename',name:'New'}),/Changed/);await tick();assert.equal(s.getSnapshot().playlist!.revision,4);assert.deepEqual(s.getSnapshot().pending,[]);await s.retryMutation();await tick();assert.notEqual(writes[0].operationId,writes[1].operationId);assert.equal(writes[1].expectedRevision,4);
});
test('editor cannot rename/share/delete and partial-page reorder is rejected before transport',async()=>{
 let writes=0;const s=setup(async(p,m)=>{if(m!=='GET')writes++;const r=resource(0,3);r.role='editor';r.actions=['add','remove','reorder'];delete (r as any).shares;return isContent(p)?projection():r;});await s.select({view:'playlist',playlistId:'playlist'});assert.throws(()=>s.mutate({action:'rename',name:'X'}));assert.throws(()=>s.mutate({action:'delete'}));assert.throws(()=>s.mutate({action:'reorder',entryIds:['entry']}));assert.equal(writes,0);
});
test('deletion clears pending actions and cannot resurrect detail on late read',async()=>{
 let deleted=false;const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection():resource();deleted=true;return {...receipt(b),deleted:true};});await s.select({view:'playlist',playlistId:'playlist'});await s.mutate({action:'delete'});await tick();assert(deleted);assert.equal(s.getSnapshot().playlist,null);assert.equal(s.getSnapshot().projection,null);assert(s.getSnapshot().lastReceipt!.deleted);
});
test('candidate page has exact scope, owner rights and bounded entries',async()=>{
 const candidate={id:'candidate',authority:'local',accountId:'account2',profileId:'profile2',displayName:'Profile …profile2'};const s=setup(async p=>p.includes('share-candidates')?{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,candidates:[candidate],nextCursor:''}:isContent(p)?projection():resource());await s.select({view:'playlist',playlistId:'playlist'});await s.loadCandidates();assert.deepEqual(s.getSnapshot().candidates!.candidates,[candidate]);assert.throws(()=>s.mutate({action:'reorder',entryIds:['x','x']}));
});
test('late mutation and queued commands cannot publish across item or profile transition',async()=>{
 const pending=deferred();let body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection():resource();body=b;return pending.promise;});await s.select({view:'playlist',playlistId:'playlist'});const first=s.mutate({action:'add',itemId:'movie'});await tick();const second=s.mutate({action:'add',itemId:'movie'});s.cancel();await Promise.all([first,second]);pending.resolve(receipt(body));await tick();assert.equal(s.getSnapshot().phase,'idle');assert.equal(s.getSnapshot().lastReceipt,null);
});

test('directory create obeys server capability; receipt points to created resource without invented rows',async()=>{
 let available=false;let wrote=false;const s=setup(async(p,m,b)=>{if(m==='GET'){const d=projection('playlists',0,[]);(d as any).actions=available?['create_playlist']:[];return d;}wrote=true;return receipt(b,0);});await s.select({view:'playlists'});assert.throws(()=>s.mutate({action:'create',name:'New'}));available=true;await s.refresh();await s.mutate({action:'create',name:'New',summary:'A summary'});await tick();assert(wrote);assert.equal(s.getSnapshot().lastReceipt!.playlistId,'playlist');assert.equal(s.getSnapshot().projection!.sections[0].entries.length,0);
});

test('confirmed revision cannot be replaced by an older post-mutation read',async()=>{
 const s=setup(async(p,m,b)=>m==='GET'?(isContent(p)?projection():resource()):receipt(b,2));await s.select({view:'playlist',playlistId:'playlist'});await s.mutate({action:'rename',name:'New'});await tick();assert.equal(s.getSnapshot().phase,'refresh-required');assert.equal(s.getSnapshot().lastReceipt!.revision,2);assert.throws(()=>s.mutate({action:'delete'}));
});
test('Unicode names and multiline summaries survive server resource projection',async()=>{
 const r=resource();r.name='🎵'.repeat(200);r.summary='First paragraph.\n\nSecond paragraph.';const s=setup(async p=>isContent(p)?projection():r);await s.select({view:'playlist',playlistId:'playlist'});assert.equal(s.getSnapshot().playlist!.name,r.name);assert.equal(s.getSnapshot().playlist!.summary,r.summary);assert.throws(()=>s.mutate({action:'rename',summary:'x'.repeat(4001)}));
});

test('windowed order read authorizes the occurrences it carries, without ranking',async()=>{
 const ids=Array.from({length:60},(_,i)=>'entry-'+i);let body:any;const s=setup(async(p,m,b)=>{if(m==='PUT'){body=b;return receipt(b,1);}if(p.endsWith('/order')||p.includes('/order?'))return{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:ids,nextCursor:''};return isContent(p)?projection('playlist',0,ids.slice(0,40).map(occurrence),'next'):resource(0,60);});await s.select({view:'playlist',playlistId:'playlist'});
 assert.throws(()=>s.mutate({action:'reorder',entryIds:ids}),/Load the current/);await s.loadOrder();assert.deepEqual(s.getSnapshot().order!.entryIds,ids);assert.equal(s.getSnapshot().order!.nextCursor,'');assert(Object.isFrozen(s.getSnapshot().order!.entryIds));await s.mutate({action:'reorder',entryIds:[...ids].reverse()});assert.equal(body.expectedRevision,0);assert.deepEqual(body.entryIds,[...ids].reverse());assert(!('afterEntryId' in body));
});
test('windowed reorder moves occurrences after an anchor, or prepends on empty anchor',async()=>{
 const ids=['a','b','c','d'];let body:any,rev=0;const s=setup(async(p,m,b)=>{if(m==='PUT'){body=b;return receipt(b,++rev);}if(p.includes('/order'))return{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:rev,entryIds:ids,nextCursor:''};return isContent(p)?projection('playlist',rev,ids.map(occurrence)):resource(rev,4);});await s.select({view:'playlist',playlistId:'playlist'});await s.loadOrder();
 await s.mutate({action:'reorder',entryIds:['d'],afterEntryId:'b'});assert.deepEqual(body.entryIds,['d']);assert.equal(body.afterEntryId,'b');
 await s.mutate({action:'reorder',entryIds:['d'],afterEntryId:''});assert.equal(body.afterEntryId,'');
 assert.throws(()=>s.mutate({action:'reorder',entryIds:['d'],afterEntryId:'d'}));assert.throws(()=>s.mutate({action:'reorder',entryIds:['d'],afterEntryId:'unseen'}));assert.throws(()=>s.mutate({action:'reorder',entryIds:new Array(201).fill('a')}));
});
test('order windows page and append; a revision move restarts from the first page',async()=>{
 const first=Array.from({length:100},(_,i)=>'entry-'+i),second=['entry-100','entry-101'];let revision=0;const s=setup(async p=>{if(p.includes('/order')){if(revision===1)return{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:1,entryIds:first,nextCursor:''};return p.includes('cursor=')?{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:second,nextCursor:''}:{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:first,nextCursor:'cursor-2'};}return isContent(p)?projection('playlist',0,first.slice(0,40).map(occurrence),'next'):resource(0,102);});await s.select({view:'playlist',playlistId:'playlist'});
 await s.loadOrder();assert.equal(s.getSnapshot().order!.entryIds.length,100);assert.equal(s.getSnapshot().order!.nextCursor,'cursor-2');
 await s.loadMoreOrder();assert.equal(s.getSnapshot().order!.entryIds.length,102);assert.equal(s.getSnapshot().order!.nextCursor,'');
 await s.loadMoreOrder();assert.equal(s.getSnapshot().order!.entryIds.length,102);
 revision=1;await s.loadOrder();assert.equal(s.getSnapshot().order,null);assert(s.getSnapshot().orderError);
});
test('order rejects torn revision, wrong fence, over-page windows, duplicate IDs and media payload',async()=>{
 const full=new Array(101).fill('entry');
 for(const patch of [{revision:1},{viewerFence:'other'},{entryIds:full},{entryIds:['entry','entry']},{media:{title:'Private'}}]){const s=setup(async p=>p.includes('/order')?{serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:['entry'],nextCursor:'',...patch}:isContent(p)?projection():resource());await s.select({view:'playlist',playlistId:'playlist'});await s.loadOrder();assert.equal(s.getSnapshot().order,null);assert(s.getSnapshot().orderError);assert.equal(s.getSnapshot().orderLoading,false);}
});
test('late order-window response is fenced after scope change and cancellation',async()=>{
 for(const transition of ['scope','cancel']){const pending=deferred();const s=setup(async p=>p.includes('/order')?pending.promise:isContent(p)?projection():resource());await s.select({view:'playlist',playlistId:'playlist'});const read=s.loadOrder();if(transition==='scope')s.setScope({...scope,viewerId:'other'},{request:async<T>()=>projection('watchlist',0,[]) as T});else s.cancel();await read;pending.resolve({serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:['entry'],nextCursor:''});await tick();assert.equal(s.getSnapshot().order,null);assert.equal(s.getSnapshot().orderLoading,false);}
});
test('viewer cannot fetch order windows and unknown occurrence IDs cannot be moved',async()=>{
 let calls=0;const s=setup(async p=>{calls++;const r=resource();r.role='viewer';r.actions=[];delete(r as any).shares;return isContent(p)?projection():r;});await s.select({view:'playlist',playlistId:'playlist'});await assert.rejects(()=>s.loadOrder(),/editable/);assert.equal(calls,2);
 const editable=setup(async p=>isContent(p)?projection():resource());await editable.select({view:'playlist',playlistId:'playlist'});assert.throws(()=>editable.mutate({action:'reorder',entryIds:['unseen']}),/current playlist order/);
});
test('reorder retains selected revision across delayed operation-ID generation',async()=>{
 const requestId=deferred();let revision=0,body:any;const s=new SavedService({scope,requestId:()=>requestId.promise,api:{request:async<T>(p,m,b)=>{if(m==='PUT'){body=b;return receipt(b,2) as T;}return(isContent(p)?projection('playlist',revision):resource(revision)) as T;}}});await s.select({view:'playlist',playlistId:'playlist'});const mutation=s.mutate({action:'reorder',entryIds:['entry']});revision=1;await s.refresh();requestId.resolve('00000000-0000-4000-8000-000000000099');await mutation;assert.equal(body.expectedRevision,0);
});
test('uncapped playlists read past 1,000 entries through order windows',async()=>{
 const ids=Array.from({length:1050},(_,i)=>'entry-'+i);
 const window=(offset:number)=>({serverId:'server',viewerFence:'fence',playlistId:'playlist',revision:0,entryIds:ids.slice(offset,offset+100),nextCursor:offset+100>=ids.length?'':'c'+(offset+100)});
 const r=resource(0,1050);delete (r.limits as any).maxEntries;r.limits.maxReorderEntries=200;
 const s=setup(async p=>{const at=p.match(/cursor=c(\d+)/);return p.includes('/order')?window(at?Number(at[1]):0):isContent(p)?projection('playlist',0,ids.slice(0,40).map(occurrence),'next'):r;});
 await s.select({view:'playlist',playlistId:'playlist'});await s.loadOrder();
 assert.equal(s.getSnapshot().playlist!.entryCount,1050);assert.equal(s.getSnapshot().playlist!.limits.maxEntries,undefined);
 for(let page=0;page<10;page++)await s.loadMoreOrder();
 assert.equal(s.getSnapshot().order!.entryIds.length,1050);assert.equal(s.getSnapshot().order!.nextCursor,'');
 await s.loadMoreOrder();assert.equal(s.getSnapshot().order!.entryIds.length,1050);
});

test('Saved page2 detail cancellation and scoped restore preserves exact cursor and Back page',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);return projection('watchlist',0,[media(p.includes('cursor=second')?'second':'first')],p.includes('cursor=second')?'':'second');});await s.select({view:'watchlist'});await s.next('entries');const saved=s.getSnapshot();assert.deepEqual(saved.pagination.history,[null]);s.cancel();await s.select(saved.route!,{scope:saved.scope,cursor:saved.pagination.cursor,history:saved.pagination.history});assert(paths.at(-1)!.includes('cursor=second'));assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'second');assert.deepEqual(s.getSnapshot().pagination.history,[null]);await s.previous();assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'first');
});
test('expired restored continuation becomes explicit refresh-required then retries first page',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);if(p.includes('cursor='))throw Object.assign(new Error('Expired'),{code:'invalid_cursor'});return projection('favorites',0,[media()]);});await s.select({view:'favorites'},{scope,cursor:'expired',history:[null]});assert.equal(s.getSnapshot().phase,'refresh-required');assert.equal(s.getSnapshot().projection,null);await s.retry();assert(!paths.at(-1)!.includes('cursor='));assert.equal(s.getSnapshot().phase,'ready');assert.deepEqual(s.getSnapshot().pagination.history,[]);
});
test('restore bounds and foreign viewer/server scope reject before transport',async()=>{
 let calls=0;const s=setup(async()=>{calls++;return projection('watchlist',0,[]);});for(const restore of [{cursor:'x'},{scope:{...scope,viewerId:'other'},cursor:'x'},{scope:{...scope,serverId:'other'},cursor:'x'},{scope,cursor:'x',history:Array(65).fill(null)},{scope,cursor:'x'.repeat(4097)},{scope,cursor:null,history:[null]}])assert.throws(()=>s.select({view:'watchlist'},restore));assert.equal(calls,0);s.setScope({...scope,viewerId:'new'},{request:async<T>()=>projection('watchlist',0,[]) as T});assert.throws(()=>s.select({view:'watchlist'},{scope,cursor:'old'}));
});
test('ignored-abort restored page cannot publish after a newer Saved selection',async()=>{
 const pending=deferred();const s=setup(async p=>p.includes('cursor=old')?pending.promise:projection('favorites',0,[media('new')]));const old=s.select({view:'watchlist'},{scope,cursor:'old',history:[null]});await s.select({view:'favorites'});pending.resolve(projection('watchlist',0,[media('old')]));await old;assert.equal(s.getSnapshot().projection!.scope.view,'favorites');assert.equal(s.getSnapshot().projection!.sections[0].entries[0].id,'new');assert.deepEqual(s.getSnapshot().pagination.history,[]);
});

test('playlist selection resumes an ambiguous addition without duplicating confirmed entries',async()=>{
 const {appendPlaylistSelection}=await import('../src/playlist-selection.ts');
 let revision=0,completed=0,lost=true;const stored=new Map<string,any>(),items:string[]=[],writes:any[]=[];
 const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection('playlist',revision,items.map((_,i)=>occurrence('e'+i))):resource(revision,items.length);
 writes.push(b);if(stored.has(b.operationId))return stored.get(b.operationId);
 items.push(b.itemId);const result=receipt(b,++revision);stored.set(b.operationId,result);
 if(b.itemId==='second'&&lost){lost=false;throw new TypeError('Connection lost');}return result;});
 await s.select({view:'playlist',playlistId:'playlist'});
 await assert.rejects(appendPlaylistSelection(s,['first','second','third'],completed,n=>completed=n));
 assert.equal(completed,1);assert.deepEqual(items,['first','second']);
 await appendPlaylistSelection(s,['first','second','third'],completed,n=>completed=n);
 assert.equal(completed,3);assert.deepEqual(items,['first','second','third']);assert.equal(writes[1].operationId,writes[2].operationId);
 s.dispose();
});
test('playlist selection stops at a changed principal or route',async()=>{
 const {appendPlaylistSelection}=await import('../src/playlist-selection.ts');
 const response=deferred();let writes=0,body:any;const s=setup(async(p,m,b)=>{if(m==='GET')return isContent(p)?projection():resource();writes++;body=b;return response.promise;});
 await s.select({view:'playlist',playlistId:'playlist'});let progress=0;
 const run=appendPlaylistSelection(s,['one','two'],0,n=>progress=n);await tick();s.cancel();response.resolve(receipt(body));await run;assert.equal(progress,0);assert.equal(writes,1);s.dispose();
});


import {readPlaylistSelection} from '../src/playlist-selection.ts';
test('Play all reads every page, preserves repeated media occurrences and skips inaccessible entries',async()=>{
 const paths:string[]=[];const s=setup(async p=>{paths.push(p);return isContent(p)?projection('playlist',0,p.includes('cursor')?[occurrence('third')]:[occurrence('first'),{id:'hidden',kind:'playlist_entry',hidden:true}],p.includes('cursor')?'':'next'):resource(0,3);});
 const result=await readPlaylistSelection(s,'playlist');assert.equal(result.unavailable,1);assert.deepEqual(result.entries.map(e=>e.itemId),['movie','movie']);assert.deepEqual(result.entries.map(e=>e.sourceContext?.entryId),['first','third']);assert(paths.some(p=>p.includes('cursor=next')));s.dispose();
});
test('Play all refuses changed revisions, incomplete counts and repeated pages',async()=>{
 for(const failure of ['revision','count','repeat']){let page=0;const s=setup(async p=>{if(isContent(p)){page=p.includes('cursor')?1:0;return projection('playlist',failure==='revision'?page:0,[occurrence(page&&failure!=='repeat'?'second':'first')],!page&&failure!=='count'?'next':'');}return resource(failure==='revision'?page:0,2);});await assert.rejects(readPlaylistSelection(s,'playlist'));s.dispose();}
});
test('Play all never returns a cancelled or oversized selection',async()=>{
 const abort=new AbortController();abort.abort();const s=setup(async p=>isContent(p)?projection():resource());await assert.rejects(readPlaylistSelection(s,'playlist',abort.signal),/cancelled/);s.dispose();
 const big=setup(async p=>isContent(p)?projection():resource(0,1001));await assert.rejects(readPlaylistSelection(big,'playlist'),{code:'invalid_saved'});big.dispose();
});

test('CD-17: selecting, paging and reloading each filter produces the intended URL/query',async()=>{
 for(const filter of ['all','unwatched','inProgress'] as const){
  const paths:string[]=[];
  const s=setup(async p=>{paths.push(p);const url=new URL(p,'https://x');const cursor=url.searchParams.get('cursor');return projection('watchlist',0,[media(cursor?`second-${filter}`:`first-${filter}`)],cursor?'':`next-${filter}`,filter);});
  await s.select({view:'watchlist',filter});
  assert.equal(s.getSnapshot().phase,'ready');
  const first=new URL(paths[0]!,'https://x');
  assert.equal(first.searchParams.get('view'),'watchlist');
  const sent=first.searchParams.get('filter')??'all';
  assert.equal(sent,filter);
  assert.equal(s.getSnapshot().projection!.query.category,filter);
  assert.equal(s.getSnapshot().projection!.filters[0].options.length,3);
  await s.next('entries');
  assert(paths.at(-1)!.includes(`cursor=next-${filter}`));
  const paged=new URL(paths.at(-1)!,'https://x');
  assert.equal(paged.searchParams.get('filter')??'all',filter);
  assert.equal(s.getSnapshot().projection!.query.category,filter);
  await s.refresh();
  assert(!paths.at(-1)!.includes('cursor='));
  assert.equal(s.getSnapshot().projection!.query.category,filter);
  s.dispose();
 }
 // Late responses cannot overwrite another filter: selecting unwatched then all keeps all.
 {
  const pending=new Map<string,()=>void>();
  const s=setup(async p=>{
   const url=new URL(p,'https://x');const f=url.searchParams.get('filter')??'all';
   if(f==='unwatched')await new Promise<void>(r=>pending.set('unwatched',()=>r()));
   return projection('watchlist',0,[media(f)],'',f as any);
  });
  const first=s.select({view:'watchlist',filter:'unwatched'});
  await tick();
  await s.select({view:'watchlist',filter:'all'});
  assert.equal(s.getSnapshot().projection!.query.category,'all');
  pending.get('unwatched')!();await first;
  assert.equal(s.getSnapshot().projection!.query.category,'all');
  s.dispose();
 }
 // Filter for other views is rejected before transport.
 {
  const s=setup(async()=>projection('playlists',0,[]));
  assert.throws(()=>s.select({view:'playlists',filter:'all' as any}));
  assert.throws(()=>s.select({view:'watchlist',filter:'everything' as any}));
  s.dispose();
 }
 // A mismatched echo fails closed.
 {
  const s=setup(async()=>projection('watchlist',0,[media()],'','unwatched'));
  await s.select({view:'watchlist',filter:'all'});
  assert.equal(s.getSnapshot().phase,'error');
  s.dispose();
 }
});

test('CD-17 response preservation: server filter echo and counts survive decoding',async()=>{
 const s=setup(async()=>projection('favorites',0,[media()],'' ,'unwatched'));
 await s.select({view:'favorites',filter:'unwatched'});
 assert.equal(s.getSnapshot().phase,'ready');
 assert.equal(s.getSnapshot().projection!.query.category,'unwatched');
 assert.equal(s.getSnapshot().projection!.filters[0].id,'filter');
 assert.deepEqual(s.getSnapshot().projection!.filters[0].options.map(o=>o.id),['all','unwatched','inProgress']);
 s.dispose();
});

test('CD-23: watchlist/history entries accept extra, author, book_series and disc',async()=>{
 const kinds: Record<string,{navigation:string}> = {
  extra:'item', author:'author', book_series:'book_series', disc:'disc',
 };
 for(const [kind,view] of Object.entries(kinds)){
  const item={id:`id-${kind}`,libraryId:'library',kind,title:`Title ${kind}`,navigation:{view,entityId:`id-${kind}`}};
  const s=setup(async()=>projection('watchlist',0,[item],'' ,'all'));
  await s.select({view:'watchlist'});
  assert.equal(s.getSnapshot().phase,'ready');
  assert.equal((s.getSnapshot().projection!.sections[0].entries[0] as any).kind,kind);
  s.dispose();
 }
 // Unknown kinds still fail closed.
 {
  const s=setup(async()=>projection('watchlist',0,[{id:'x',libraryId:'library',kind:'podcast',title:'X'}],'' ,'all'));
  await s.select({view:'watchlist'});
  assert.equal(s.getSnapshot().phase,'error');
  s.dispose();
 }
});
