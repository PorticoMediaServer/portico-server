import {test} from 'node:test';
import assert from 'node:assert/strict';
import {DashboardDataClient,parseDashboardHistory,parseDashboardPeople,DASHBOARD_LOAD_RANGES,DASHBOARD_LOAD_SERIES} from '../src/admin/dashboard.ts';
import {parseDashboardSessionFacts} from '../src/admin/activity-summary/parser.ts';
test('Dashboard analytics use account IDs, page bounds and exact kind totals',()=>{
 const row={accountId:'server-account',username:'person',profiles:['Kids'],plays:2,watchedMs:12000,kinds:[{kind:'movies',plays:2,watchedMs:12000}]};
 assert.equal(parseDashboardPeople({items:[row],nextCursor:''}).items[0].accountId,'server-account');
 assert.throws(()=>parseDashboardPeople({items:[{...row,watchedMs:12345}],nextCursor:''}));
 assert.throws(()=>parseDashboardPeople({items:Array(101).fill(row),nextCursor:''}));
 assert.throws(()=>parseDashboardPeople({items:[row,row],nextCursor:''}));
 assert.equal(parseDashboardPeople({items:[{...row,kinds:[{kind:'audiobooks',plays:1,watchedMs:6000},{kind:'live',plays:1,watchedMs:6000}]}],nextCursor:''}).items[0].kinds.length,2);
 assert.equal(parseDashboardPeople({items:[{...row,kinds:[{kind:'other',plays:2,watchedMs:12000}]}],nextCursor:''}).items[0].kinds[0].kind,'other');
});
test('history preserves actual empty points, partial endpoints and music count units',()=>{
 const raw={period:'7d',from:'2026-10-22T00:00:00Z',until:'2026-10-22T12:00:00Z',timeZone:'UTC',
 buckets:[{start:'2026-10-22T00:00:00Z',end:'2026-10-22T12:00:00Z',partial:true,kinds:[]}],busiestTimes:Array.from({length:7},()=>Array(24).fill(0)),
 quietestStartHour:null,top:[],topKind:'music'};
 const out=parseDashboardHistory(raw);
 assert.equal(out.buckets[0].kinds.length,0);assert.equal(out.quietestStartHour,null);
 assert.throws(()=>parseDashboardHistory({...raw,top:[{itemId:'album',title:'Album',plays:2,people:3,countUnit:'track_plays'}]}));
 assert.deepEqual(DASHBOARD_LOAD_RANGES,['live','12h','24h','7d','30d','90d','1y','all']);
 assert.deepEqual(DASHBOARD_LOAD_SERIES.bandwidth.metrics,['awayMbps','homeMbps']);
});
test('session source projection rejects nested capabilities and never fabricates hardware',()=>{
 const raw={media:{video:{id:'v0',codec:'hevc',profile:'Main 10',height:2160,hdr:'hdr10'}},currentBitrateKbps:0,rateWindowMs:5000,live:false};
 assert.equal((parseDashboardSessionFacts(raw).media as any).videoHardware,undefined);
 assert.throws(()=>parseDashboardSessionFacts({...raw,media:{...raw.media,url:'/private-capability'}}));
 assert.throws(()=>parseDashboardSessionFacts({...raw,rateWindowMs:2000}));
});
test('client requests the same server-account/media/range seam as the history table',async()=>{
 let seen='';
 const client=new DashboardDataClient({request:async(path:string)=>{seen=path;return {items:[],nextCursor:''} as never;}});
 await client.people({period:'30d',accountId:'server-account',media:'music'},'cursor',100);
 const url=new URL(seen,'https://server.invalid');
 assert.equal(url.pathname,'/v1/admin/play-history/people');
 assert.equal(url.searchParams.get('accountId'),'server-account');assert.equal(url.searchParams.get('media'),'music');assert.equal(url.searchParams.get('limit'),'100');
});
test('selected-source facts accept the checked normalized breadth DTO without retaining capabilities',()=>{
 const video={id:'v0',codec:'hevc',profile:'main 10',height:2160,chromaSubsampling:'4:2:0',frameRateNumerator:24000,frameRateDenominator:1001,frameRateMode:'constant',dolbyVisionProfile:8,dolbyVisionCompatibility:1,dolbyVisionRpu:true,dolbyVisionBaseLayer:true,dolbyVisionEnhancementType:'mel',dolbyVisionCmVersion:'4.0',hdr10Plus:false,masterDisplay:'G(13250,34500)',maxCll:'1000,400'};
 const audio={id:'a1',codec:'dts',variant:'dtsx',objectAudio:'dtsx',dtsx:true,channels:8,atmos:false,default:true,commentary:false,visualImpaired:true};
 const raw={media:{video,audio},currentBitrateKbps:0,rateWindowMs:5000,live:false};
 const out=parseDashboardSessionFacts(raw).media as {video:typeof video;audio:typeof audio};
 assert.deepEqual(out.video,video);assert.deepEqual(out.audio,audio);
 const {objectAudio,...ma}=audio;
 const hdma={...ma,variant:'dts_hd_ma',dtsx:false};
 assert.deepEqual((parseDashboardSessionFacts({...raw,media:{video,audio:hdma}}).media as any).audio,hdma);
 assert.throws(()=>parseDashboardSessionFacts({...raw,media:{video:{...video,presentationUrl:'/private-grant'},audio}}));
 assert.throws(()=>parseDashboardSessionFacts({...raw,media:{video,audio:{...audio,url:'/private-grant'}}}));
 assert.throws(()=>parseDashboardSessionFacts({...raw,media:{video,audio:{...audio,visualImpaired:'yes'}}}));
});
