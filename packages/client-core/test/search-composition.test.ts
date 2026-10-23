import test from 'node:test';
import assert from 'node:assert/strict';
import {SearchService,SEARCH_DRAWN_GROUPS,readSearchDiscovery,searchSongQueueSource,type SearchGroup} from '../src/search.ts';
import {decodeSearchResponse,decodeGetSearchDiscoveryResponse} from '../../contracts/src/decoders.ts';

const scope={serverId:'server',viewerId:'viewer'};
const kinds={movies:'movie',shows:'show',episodes:'episode',people:'person',artists:'artist',albums:'album',songs:'song',books:'book','live-tv':'channel',programmes:'programme'} as const;
const channel={id:'channel',sourceId:'source',generation:'generation',name:'Channel',number:'7',group:'News',provenance:'live-source'};
const programme={id:'airing',title:'Programme',subtitle:'Episode name',start:'2026-10-22T12:00:00Z',end:'2026-10-22T13:00:00Z',live:true,moreAirings:2,moreAiringsExact:true};
const film=(id='movie',title='Harbor')=>({id,kind:'movie',libraryId:'library',title,missing:false,addedAt:null,available:true,playback:{itemId:id},navigation:{view:'item',entityId:id}});
function answer(path:string){
 const u=new URL(path,'https://server');const group=u.searchParams.get('group')??'';
 const ids=(group?[group]:(u.searchParams.get('groups')?.split(',')??SEARCH_DRAWN_GROUPS)) as SearchGroup[];
 return {scope:{serverId:'server',libraryId:'',libraryKind:'mixed',view:'search',entityId:'',viewerFence:'fence'},revision:{catalog:4,viewer:5},heading:{key:'search.title',fallback:'Search'},query:{q:u.searchParams.get('q'),group,groups:ids,sort:u.searchParams.get('sort')??'relevance',direction:u.searchParams.get('direction')??'desc',libraryIds:['library'],limit:40,searchMode:'normalized_prefix',recorded:false},groups:ids.map(id=>({id,title:id,labelKey:'search.chip.'+id,entityKind:kinds[id],status:'success',errorCode:'',sort:id==='programmes'?'channelTitle':'relevance',direction:id==='programmes'?'asc':'desc',items:[] as any[],totalCount:0,totalCountExact:true,nextCursor:'',hasMore:false})),capabilities:{groups:SEARCH_DRAWN_GROUPS.map(id=>({id,title:id,entityKind:kinds[id],labelKey:'search.chip.'+id,available:true,sorts:id==='programmes'?['channelTitle']:['relevance','title'],directions:id==='programmes'?['asc']:['asc','desc']})),sorts:['relevance','title','releaseYear','dateAdded','channelTitle'],directions:['asc','desc'],maxLimit:40,groupBudgetMs:600}};
}
function service(read:(p:string)=>unknown){return new SearchService({scope,api:{request:async<T>(p:string)=>read(p) as T}});}

test('generated decoding and Search whitelist retain structured personal and match facts',async()=>{
 const s=service(p=>{const raw=answer(p);raw.groups.find(g=>g.id==='movies')!.items=[{...film(),progressMs:12500,watched:false,watchlisted:true,matchedOn:{field:'credit',text:'Ada Lovelace'}}];raw.groups.find(g=>g.id==='people')!.items=[{id:'person',kind:'person',title:'Ada Lovelace',missing:false,addedAt:null,roles:['Acting'],knownFor:{id:'movie',name:'Harbor'},navigation:{view:'person',entityId:'person'}}];raw.groups.find(g=>g.id==='episodes')!.items=[{...film('episode','Episode'),kind:'episode',show:{id:'show',name:'Show'},seasonNumber:2,episodeNumber:3,progressMs:5000}];for(const g of raw.groups)g.totalCount=g.items.length;return decodeSearchResponse(raw);});
 await s.select('Ada');assert.equal(s.getSnapshot().phase,'ready');const data=s.getSnapshot().data!;
 const movie=data.groups.find(g=>g.id==='movies')!.items[0] as any;assert.deepEqual(movie.matchedOn,{field:'credit',text:'Ada Lovelace'});assert.equal(movie.watchlisted,true);assert.equal(movie.progressSeconds,12.5);
 const person=data.groups.find(g=>g.id==='people')!.items[0];assert.deepEqual(person.knownFor,{id:'movie',name:'Harbor'});assert.deepEqual(person.roles,['Acting']);assert.equal('libraryId'in person,false);
 const episode=data.groups.find(g=>g.id==='episodes')!.items[0] as any;assert.deepEqual(episode.show,{id:'show',name:'Show'});assert.equal(episode.seasonNumber,2);assert.equal(episode.episodeNumber,3);s.dispose();
});

test('programme/channel identity and actual source order survive generated and headless decoding',async()=>{
 const s=service(p=>{const raw=answer(p);const g=raw.groups.find(g=>g.id==='programmes')!;g.items=[{id:'live-source:source:airing',kind:'programme',title:'Programme',missing:false,addedAt:null,channel,programme,navigation:{view:'programme',entityId:'live-source:source:airing'}}];g.totalCount=1;Object.assign(g,{sourceRevision:'guide-revision',windowStart:programme.start,windowEnd:'2026-11-05T12:00:00Z'});return decodeSearchResponse(raw);});
 await s.select('Programme');const g=s.getSnapshot().data!.groups.find(g=>g.id==='programmes')!;assert.equal(g.sort,'channelTitle');assert.equal(g.direction,'asc');assert.equal(g.sourceRevision,'guide-revision');assert.deepEqual(g.items[0].channel,channel);assert.deepEqual(g.items[0].programme,programme);assert.deepEqual(s.getSnapshot().data!.capabilities.groups.find(g=>g.id==='programmes')!.directions,['asc']);s.dispose();
});

test('same-query chips use last All counts without another All request',async()=>{
 const calls:string[]=[];let revision=4;
 const s=service(p=>{calls.push(p);const raw=answer(p);raw.revision.catalog=revision;for(const g of raw.groups){g.totalCount=g.id==='movies'?7:g.id==='songs'?4:0;}return raw;});
 await s.select('Harbor');await s.select('Harbor',{group:'songs'});
 assert.equal(calls.length,2);assert.equal(new URL(calls[1],'https://server').searchParams.get('group'),'songs');assert.equal(s.getSnapshot().chipCounts.find(g=>g.id==='movies')?.totalCount,7);assert.equal(s.getSnapshot().chipCounts.find(g=>g.id==='songs')?.totalCount,4);
 revision++;await s.refresh();assert.deepEqual(s.getSnapshot().chipCounts.map(g=>g.id),['songs']);
 await s.select('Different',{group:'songs'});assert.equal(s.getSnapshot().chipCounts.some(g=>g.id==='movies'),false);s.setScope({serverId:'server',viewerId:'other'},{request:async<T>()=>undefined as T});assert.deepEqual(s.getSnapshot().chipCounts,[]);s.dispose();
});

test('default Search requests only the drawn groups',async()=>{
 const calls:string[]=[];const s=service(p=>{calls.push(p);return answer(p);});await s.select('Harbor');assert.deepEqual(new URL(calls[0],'https://server').searchParams.get('groups')!.split(','),SEARCH_DRAWN_GROUPS);assert(!SEARCH_DRAWN_GROUPS.includes('collections'));assert(!SEARCH_DRAWN_GROUPS.includes('authors'));s.dispose();
});

test('only the genuine unique returned Top may carry enriched facts and one show action',async()=>{
 const read=(wrong=false)=>service(p=>{const raw:any=answer(p);raw.groups.find((g:any)=>g.id==='shows').items=[{id:'show',kind:'show',title:'Harbor',libraryId:'library',missing:false,addedAt:null,navigation:{view:'show',entityId:'show'}}];raw.top={group:'shows',entry:{...raw.groups.find((g:any)=>g.id==='shows').items[0],id:wrong?'guessed':'show',overview:'Real synopsis',seasonCount:2,watchlisted:true},primary:{...film('episode','Episode'),kind:'episode',show:{id:'show',name:'Harbor'},seasonNumber:1,episodeNumber:1,progressMs:12500}};return raw;});
 const s=read();await s.select('Harbor');assert.equal(s.getSnapshot().data?.top?.entry.id,'show');assert.equal(s.getSnapshot().data?.top?.primary?.id,'episode');assert.equal(s.getSnapshot().data?.top?.primary?.progressSeconds,12.5);s.dispose();const wrong=read(true);await wrong.select('Harbor');assert.equal(wrong.getSnapshot().phase,'error');assert.equal(wrong.getSnapshot().data,null);wrong.dispose();
});

test('empty continuation pages remain pageable until the server omits nextCursor',async()=>{
 const s=service(p=>{const raw=answer(p);const g=raw.groups[0];g.items=[];if(!p.includes('cursor='))g.nextCursor='continue-source';return raw;});await s.select('Rare',{group:'movies'});assert.equal(s.getSnapshot().groups[0].items.length,0);assert.equal(s.getSnapshot().groups[0].hasMore,true);await s.next('movies');assert.equal(s.getSnapshot().groups[0].nextCursor,'');assert.equal(s.getSnapshot().groups[0].hasMore,false);s.dispose();
});

test('Search discovery keeps authoritative genre samples and four-channel bound',async()=>{
 const raw={scope:{serverId:'server',libraryId:'',libraryKind:'mixed',view:'search-discovery',entityId:'',viewerFence:'fence'},revision:{catalog:4,viewer:5},genres:[{value:'Drama',label:'Drama',count:7,countIsLowerBound:false,posterUrls:['/v1/items/movie/art/poster'],backdropUrl:'/v1/items/movie/art/backdrop'}],genresNextCursor:'',onNow:[{id:'channel',kind:'channel',title:'Channel',missing:false,addedAt:null,channel,programme,navigation:{view:'channel',entityId:'channel'}}]};
 const data=await readSearchDiscovery({request:async<T>()=>decodeGetSearchDiscoveryResponse(raw) as T},scope);assert.equal(data.genres[0].count,7);assert.deepEqual(data.genres[0].posterUrls,['/v1/items/movie/art/poster']);assert.equal(data.genres[0].backdropUrl,'/v1/items/movie/art/backdrop');assert.deepEqual(data.onNow[0].channel,channel);
 await assert.rejects(readSearchDiscovery({request:async<T>()=>({...raw,onNow:Array(5).fill(raw.onNow[0])}) as T},scope));
});

test('Play all and Shuffle name the full source without visible cards, cursors or history side effects',()=>{
 assert.deepEqual(searchSongQueueSource({q:'  Harbor  ',group:'songs',sort:'title',direction:'asc',libraryIds:['library'],record:true}),{search:{q:'Harbor',group:'songs',sort:'title',direction:'asc',libraryIds:['library']}});
 assert.throws(()=>searchSongQueueSource({q:'Harbor',group:'movies'}));
});
