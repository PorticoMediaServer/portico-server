import test from 'node:test';import assert from 'node:assert/strict';
import {SearchService,parseSearchHistory,searchQueryProblem,type SearchGroup} from '../src/search.ts';
const scope={serverId:'server',viewerId:JSON.stringify(['hosted','account','profile'])};
const kinds={movies:'movie',shows:'show',episodes:'episode',artists:'artist',albums:'album',songs:'song',books:'book',people:'person','live-tv':'channel'} as const;
const all=Object.keys(kinds) as SearchGroup[];
const mediaGroups=all.filter(g=>g!=='people'&&g!=='live-tv');
const capabilities=(unavailable:SearchGroup[]=['live-tv'])=>({groups:all.map(id=>({id,title:id,entityKind:kinds[id],available:!unavailable.includes(id),...(unavailable.includes(id)?{reason:'Not configured on this server.'}:{}),sorts:['relevance','title']})),sorts:['relevance','title','releaseYear','dateAdded'],directions:['asc','desc'],maxLimit:40,groupBudgetMs:600});
function group(id:SearchGroup,ids=['one'],nextCursor=''):any{
 const entityKind=kinds[id];
 const view=['movies','episodes','songs'].includes(id)?'item':entityKind;
 const items=ids.map(entity=>id==='people'||id==='live-tv'
  ?{id:entity,kind:entityKind,title:'Server '+entity,navigation:{view,entityId:entity}}
  :{id:entity,libraryId:'library',kind:entityKind,title:'Server '+entity,navigation:{view,entityId:entity}});
 return {id,title:id,entityKind,status:'success',errorCode:'',items,totalCount:20,hasMore:!!nextCursor,nextCursor};
}
function failed(id:SearchGroup,errorCode='search_group_timeout'):any{return {id,title:id,entityKind:kinds[id],status:'error',errorCode,items:[],totalCount:0,hasMore:false,nextCursor:''};}
function result(path:string,changes:Record<string,unknown>={}):any{
 const url=new URL(path,'https://server.test'),selected=url.searchParams.get('group') as SearchGroup|null;
 const restrict=url.searchParams.get('groups');
 const echoed=selected?[selected]:restrict?restrict.split(','):all;
 return {scope:{serverId:'server',libraryId:'',libraryKind:'mixed',view:'search',entityId:'',viewerFence:'fence'},revision:{catalog:4,viewer:5},heading:{key:'search.title',fallback:'Search'},
  query:{q:url.searchParams.get('q'),sort:url.searchParams.get('sort')??'relevance',direction:url.searchParams.get('direction')??'desc',group:selected??'',groups:echoed,libraryIds:url.searchParams.get('libraryIds')?.split(',')??[],limit:Number(url.searchParams.get('limit')),searchMode:'token_prefix',recorded:url.searchParams.get('record')==='1'},
  groups:echoed.map(id=>group(id as SearchGroup)),capabilities:capabilities(),...changes};
}
function service(fn:(path:string,signal?:AbortSignal)=>Promise<unknown>,timeoutMs=1000){return new SearchService({scope,timeoutMs,api:{request:<T>(path:string,_m?:string,_b?:unknown,signal?:AbortSignal)=>fn(path,signal) as Promise<T>}});}
function deferred(){let resolve!:(value:unknown)=>void;const promise=new Promise<unknown>(r=>resolve=r);return {promise,resolve};}

test('preserves server group order and publishes per-group capabilities',async()=>{
 const s=service(async path=>result(path,{groups:[group('books'),group('shows'),group('movies')],query:{...result(path).query,groups:['books','shows','movies']}}));
 await s.select('harbor');
 assert.equal(s.getSnapshot().phase,'ready');
 assert.deepEqual(s.getSnapshot().groups.map(g=>g.id),['books','shows','movies']);
 const published=new Map(s.getSnapshot().data!.capabilities.groups.map(g=>[g.id,g.available]));
 assert.equal(published.get('people'),true);
 assert.equal(published.get('live-tv'),false);
 assert.throws(()=>{(s.getSnapshot().groups as any).reverse();});
 s.dispose();
});
test('a failed group is reported beside the groups that succeeded',async()=>{
 const s=service(async path=>{const raw=result(path);raw.groups=raw.groups.map((g:any)=>g.id==='shows'?failed('shows'):g);return raw;});
 await s.select('harbor');
 assert.equal(s.getSnapshot().phase,'ready');
 const shows=s.getSnapshot().groups.find(g=>g.id==='shows')!;
 assert.equal(shows.status,'error');
 assert.equal(shows.errorCode,'search_group_timeout');
 assert.deepEqual(shows.items,[]);
 assert.equal(s.getSnapshot().groups.find(g=>g.id==='movies')!.items.length,1);
 s.dispose();
});
test('a failed group carrying results, or an unavailable group with no reason, fails closed',async()=>{
 const changes=[
  (r:any)=>{r.groups[0]={...failed('movies'),items:[{id:'x',libraryId:'l',kind:'movie',title:'X',navigation:{view:'item',entityId:'x'}}]};},
  (r:any)=>{r.groups[0]={...group('movies'),errorCode:'search_group_timeout'};},
  (r:any)=>{r.groups[0]={...group('movies'),hasMore:true};},
  (r:any)=>{r.capabilities.groups=r.capabilities.groups.map((g:any)=>g.id==='live-tv'?{...g,reason:''}:g);},
  (r:any)=>{r.groups[0]={...group('movies'),errorCode:'made_up'};},
 ];
 for(const change of changes){const s=service(async path=>{const raw=result(path);change(raw);return raw;});await s.select('harbor');assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().data,null);s.dispose();}
});
test('sort, direction, group restriction and library restriction are server requests',async()=>{
 let path='';const s=service(async p=>{path=p;return result(p);});
 await s.select('harbor',{sort:'releaseYear',direction:'asc',groups:['movies','people'],libraryIds:['a','b']});
 const url=new URL(path,'https://x');
 assert.equal(url.searchParams.get('sort'),'releaseYear');
 assert.equal(url.searchParams.get('direction'),'asc');
 assert.equal(url.searchParams.get('groups'),'movies,people');
 assert.equal(url.searchParams.get('libraryIds'),'a,b');
 assert.deepEqual(s.getSnapshot().groups.map(g=>g.id),['movies','people']);
 assert.throws(()=>s.select('harbor',{sort:'popularity' as any}));
 assert.throws(()=>s.select('harbor',{groups:['movies','nope' as any]}));
 assert.throws(()=>s.select('harbor',{group:'movies',groups:['movies']}));
 s.dispose();
});
test('a server that answers a different sort or a group that was not asked for fails closed',async()=>{
 const wrongSort=service(async p=>{const raw=result(p);raw.query.sort='title';return raw;});
 await wrongSort.select('harbor',{sort:'relevance'});
 assert.equal(wrongSort.getSnapshot().phase,'error');
 wrongSort.dispose();
 const extra=service(async p=>{const raw=result(p);raw.groups.push(group('movies'));return raw;});
 await extra.select('harbor',{groups:['people']});
 assert.equal(extra.getSnapshot().phase,'error');
 extra.dispose();
});
test('people and channel rows carry no library, playback or availability',async()=>{
 const s=service(async p=>result(p,{groups:[group('people')],query:{...result(p).query,groups:['people']}}));
 await s.select('harbor');
 const person=s.getSnapshot().groups[0].items[0] as any;
 assert.equal(person.kind,'person');
 assert.equal(person.navigation.view,'person');
 assert.equal(person.libraryId,undefined);
 s.dispose();
 const leaking=service(async p=>{const raw=result(p,{groups:[group('people')],query:{...result(p).query,groups:['people']}});raw.groups[0].items[0].playback={itemId:'one'};return raw;});
 await leaking.select('harbor');
 assert.equal(leaking.getSnapshot().phase,'error');
 leaking.dispose();
});
test('Unicode whitespace normalization and punctuation are encoded without local matching',async()=>{let path='';const s=service(async p=>{path=p;return result(p);});await s.select(' \tCaféone & two  ');assert.equal(new URL(path,'https://x').searchParams.get('q'),'Café one & two');assert.equal(s.getSnapshot().query!.q,'Café one & two');assert.throws(()=>s.select('é'.repeat(129)));s.dispose();});
test('CD-30: search validation matches NormalizeSearch before dispatch',async()=>{
 let calls=0;const s=service(async p=>{calls++;return result(p);});
 // Invalid inputs throw as input feedback without dispatching (no connection loss).
 for(const q of ['a','!!!','a b c d','one two three four five six seven eight nine','𠮷'.repeat(129),'é'.repeat(129)])assert.throws(()=>s.select(q),/Invalid search query|Search supports/);
 assert.equal(calls,0);
 assert.equal(s.getSnapshot().phase,'idle');
 // Astral counting is by code points, not UTF-16 units: 100 astral letters (200 units) is valid.
 await s.select('𠮷'.repeat(100));
 assert.equal(s.getSnapshot().phase,'ready');
 assert.equal([...s.getSnapshot().query!.q].length,100);
 // Valid non-Latin search dispatches.
 await s.select('日本語');
 assert.equal(s.getSnapshot().phase,'ready');
 assert.equal(s.getSnapshot().query!.q,'日本語');
 // Tokens split on non-letter/non-number: CJK + punctuation still valid.
 await s.select('日本語・テスト');
 assert.equal(s.getSnapshot().phase,'ready');
 s.dispose();
});
test('empty query clears results without requesting recommendations or persisting history',async()=>{let calls=0;const s=service(async p=>{calls++;return result(p);});await s.select('title');await s.select('  ');assert.equal(calls,1);assert.equal(s.getSnapshot().phase,'idle');assert.equal(s.getSnapshot().query,null);assert.deepEqual(s.getSnapshot().groups,[]);s.dispose();});
test('history is recorded only when the caller asks for it',async()=>{
 let path='';const s=service(async p=>{path=p;return result(p);});
 await s.select('harbor');
 assert.equal(new URL(path,'https://x').searchParams.has('record'),false);
 assert.equal(s.getSnapshot().data!.query.recorded,false);
 await s.select('harbor two',{record:true});
 assert.equal(new URL(path,'https://x').searchParams.get('record'),'1');
 assert.equal(s.getSnapshot().data!.query.recorded,true);
 s.dispose();
 // A server that claims it recorded a query the client did not ask to record is rejected.
 const lying=service(async p=>{const raw=result(p);raw.query.recorded=true;return raw;});
 await lying.select('harbor');
 assert.equal(lying.getSnapshot().phase,'error');
 lying.dispose();
});
test('search history parses as the viewer own bounded list', ()=>{
 const page=parseSearchHistory({serverId:'server',viewerFence:'fence',maxEntries:50,entries:[{q:'harbor',updatedAt:'2026-09-16T00:00:00Z',uses:3}]},scope);
 assert.equal(page.entries[0].q,'harbor');
 assert.throws(()=>parseSearchHistory({serverId:'other',viewerFence:'fence',maxEntries:50,entries:[]},scope));
 assert.throws(()=>parseSearchHistory({serverId:'server',viewerFence:'fence',maxEntries:50,entries:[{q:'a',updatedAt:'x',uses:1},{q:'a',updatedAt:'y',uses:1}]},scope));
});
test('group continuation and Previous preserve exact query and server page; no accumulation',async()=>{const paths:string[]=[];const s=service(async p=>{paths.push(p);const page=new URL(p,'https://x').searchParams.get('cursor');const raw=result(p);raw.groups=raw.groups.map((g:any)=>g.id==='movies'?group('movies',page?['six']:['one'],page?'':'next-page'):g);return raw;});await s.select('title');await s.next('movies');const movies=s.getSnapshot().groups.find(g=>g.id==='movies')!;assert.equal(movies.items[0].id,'six');assert.equal(movies.items.length,1);assert.equal(s.getSnapshot().pagination.canPrevious,true);await s.previous();assert.equal(new URL(paths.at(-1)!,'https://x').searchParams.has('group'),false);assert.equal(s.getSnapshot().groups.find(g=>g.id==='movies')!.items[0].id,'one');s.dispose();});
test('every published group is a server request, preserving typed navigation',async()=>{const s=service(async p=>result(p));for(const selected of all){await s.select('test',{group:selected});assert.equal(s.getSnapshot().phase,'ready');assert.equal(s.getSnapshot().query?.group,selected);}s.dispose();});
test('rapid query change aborts and ignores transport that completes late',async()=>{const old=deferred();let signal:AbortSignal|undefined,oldPath='';const s=service(async(p,a)=>{if(p.includes('q=old')){signal=a;oldPath=p;return old.promise;}return result(p);});const pending=s.select('old');await s.select('new');await pending;assert(signal?.aborted);old.resolve(result(oldPath));await Promise.resolve();assert.equal(s.getSnapshot().data!.query.q,'new');s.dispose();});
test('same in-flight command is singleflight, cancel clears even ignored abort',async()=>{const d=deferred();let calls=0;const s=service(async()=>{calls++;return d.promise;});const a=s.select('test'),b=s.select('test');assert.equal(a,b);assert.equal(calls,1);s.cancel();await a;assert.equal(s.getSnapshot().phase,'idle');assert.equal(s.getSnapshot().query,null);s.dispose();});
test('scope change clears query and pending data before a new principal binds',async()=>{const d=deferred();const s=service(async()=>d.promise);const pending=s.select('private');s.setScope({...scope,viewerId:'different-profile'},{request:async<T>()=>{throw new Error('unused');}});await pending;d.resolve({});await Promise.resolve();assert.equal(s.getSnapshot().scope.viewerId,'different-profile');assert.equal(s.getSnapshot().query,null);assert.deepEqual(s.getSnapshot().groups,[]);s.dispose();});
test('timeout settles transport ignoring abort and explicit retry can recover',async()=>{let calls=0;const s=service(async p=>{calls++;return calls===1?new Promise(()=>{}):result(p);},5);await s.select('test');assert.equal(s.getSnapshot().error?.code,'timeout');await s.retry();assert.equal(s.getSnapshot().phase,'ready');s.dispose();});
test('denied read clears previously visible data and never offers stale results',async()=>{let denied=false;const s=service(async p=>{if(denied)throw Object.assign(new Error('Permission removed'),{code:'forbidden'});return result(p);});await s.select('test');denied=true;await s.refresh();assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().data,null);assert.deepEqual(s.getSnapshot().groups,[]);s.dispose();});
test('catalog, viewer and permission fences reject changed continuations until explicit refresh',async()=>{for(const change of ['catalog','viewer','fence']){let second=false;const s=service(async p=>{const raw=result(p);raw.groups=raw.groups.map((g:any)=>g.id==='movies'?group('movies',['one'],second?'':'next'):g);if(second){if(change==='fence')raw.scope.viewerFence='new-fence';else raw.revision[change]++;}return raw;});await s.select('test');second=true;await s.next('movies');assert.equal(s.getSnapshot().phase,'refresh-required');assert.equal(s.getSnapshot().data,null);await s.refresh();assert.equal(s.getSnapshot().phase,'ready');s.dispose();}});
test('wrong scope/query, excessive pages, duplicate entries and foreign navigation fail closed',async()=>{const changes=[(r:any)=>r.scope.serverId='wrong',(r:any)=>r.query.q='other',(r:any)=>r.groups[0].items=Array(6).fill(r.groups[0].items[0]),(r:any)=>r.groups[0].items.push(r.groups[0].items[0]),(r:any)=>r.groups[0].items[0].libraryId=undefined,(r:any)=>r.groups[0].items[0].navigation.entityId='wrong',(r:any)=>r.groups[0].items[0].available=false];for(const [i,change]of changes.entries()){const s=service(async p=>{const raw=result(p);change(raw);if(i===6)raw.groups[0].items[0].playback={itemId:'one'};return raw;});await s.select('test');assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().data,null);s.dispose();}});
test('pagination history is bounded to 64 prior pages and repeated cursor requires refresh',async()=>{const s=service(async p=>{const u=new URL(p,'https://x'),index=Number(u.searchParams.get('cursor')??0);const raw=result(p);raw.groups=raw.groups.map((g:any)=>g.id==='movies'?group('movies',[String(index)],String(index+1)):g);return raw;});await s.select('test',{group:'movies'});for(let i=0;i<70;i++)await s.next('movies');for(let i=0;i<64;i++)await s.previous();assert.equal(s.getSnapshot().pagination.canPrevious,false);assert.equal(s.getSnapshot().groups[0].items[0].id,'6');s.dispose();const repeated=service(async p=>{const raw=result(p);raw.groups=raw.groups.map((g:any)=>g.id==='movies'?group('movies',['one'],'same'):g);return raw;});await repeated.select('test');await repeated.next('movies');assert.equal(repeated.getSnapshot().phase,'refresh-required');repeated.dispose();});
test('stale server cursor is explicit refresh-required; retry restarts first page',async()=>{const paths:string[]=[];const s=service(async p=>{paths.push(p);if(p.includes('cursor='))throw Object.assign(new Error('Expired continuation'),{code:'stale_continuation',retryable:true});const raw=result(p);raw.groups=raw.groups.map((g:any)=>g.id==='movies'?group('movies',['one'],'next'):g);return raw;});await s.select('test');await s.next('movies');assert.equal(s.getSnapshot().phase,'refresh-required');await s.retry();assert.equal(s.getSnapshot().phase,'ready');assert(!paths.at(-1)!.includes('cursor='));s.dispose();});
test('same-query transient refresh retains results but a new query never inherits them',async()=>{
 let fail=false;const s=service(async p=>{if(fail)throw new TypeError('Failed to fetch');return result(p);});
 await s.select('harbor');fail=true;const refresh=s.refresh();assert.equal(s.getSnapshot().groups[0].items[0].id,'one');await refresh;
 assert.equal(s.getSnapshot().phase,'error');assert.equal(s.getSnapshot().error?.retryable,true);assert.equal(s.getSnapshot().groups.length,mediaGroups.length+2);
 await s.select('different');assert.equal(s.getSnapshot().groups.length,0);s.dispose();
});

test('searchQueryProblem says why a query would not be sent (CD-30)', () => {
  assert.equal(searchQueryProblem(''), undefined);
  assert.equal(searchQueryProblem('   '), undefined);
  assert.equal(searchQueryProblem('a'), 'short');
  assert.equal(searchQueryProblem('!!!'), 'short');
  assert.equal(searchQueryProblem('a b c'), 'short');
  assert.equal(searchQueryProblem('one two three four five six seven eight nine'), 'words');
  assert.equal(searchQueryProblem('x'.repeat(129)), 'long');
  assert.equal(searchQueryProblem('テスト'), undefined);
  assert.equal(searchQueryProblem('ab'), undefined);
  assert.equal(searchQueryProblem('a lumen'), undefined);
});
