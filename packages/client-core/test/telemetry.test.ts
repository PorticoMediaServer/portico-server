import assert from 'node:assert/strict';
import test from 'node:test';
import {METRIC_NAMES,parseMetric,parseTelemetryReading,parseTelemetrySample,parseTranscodeCapacity} from '../src/telemetry.ts';
import {parseAttention} from '../src/attention.ts';
import {parsePlaybackHistory} from '../src/playback-history.ts';

const metrics=(overrides:Record<string,unknown>={})=>{
 const out:Record<string,unknown>={};
 for(const name of METRIC_NAMES)out[name]={status:'unavailable',value:0,detail:'Not supported on this platform.'};
 return {...out,...overrides};
};
const sample=(overrides:Record<string,unknown>={})=>({at:1758000000000,metrics:metrics({cpu:{status:'available',value:12.5}}),memoryUsedBytes:4,memoryTotalBytes:8,...overrides});
const reading=(overrides:Record<string,unknown>={})=>{
 const series:Record<string,unknown>={};
 for(const name of METRIC_NAMES)series[name]=[];
 series.cpu=[{t:1757999000000,v:10},{t:1758000000000,v:20}];
 return {window:'10m',series,status:metrics(),observedAt:1758000000000,...overrides};
};

test('a metric carries its own status and an unavailable one cannot smuggle a value',()=>{
 assert.equal(parseMetric({status:'available',value:42}).value,42);
 assert.equal(parseMetric({status:'limited',value:7,detail:'Derived from load average.'}).status,'limited');
 assert.throws(()=>parseMetric({status:'unavailable',value:99}),/Invalid/);
 assert.throws(()=>parseMetric({status:'guessed',value:1}),/Invalid/);
 assert.throws(()=>parseMetric({status:'available',value:'high'}),/Invalid/);
});

test('a sample must publish every metric the server names',()=>{
 assert.equal(parseTelemetrySample(sample()).metrics.cpu.value,12.5);
 const partial=sample();
 delete (partial.metrics as Record<string,unknown>).gpuEncoder;
 assert.throws(()=>parseTelemetrySample(partial),/Invalid/);
 assert.throws(()=>parseTelemetrySample(sample({memoryUsedBytes:9})),/Invalid/);
});

test('a reading is bounded, ordered and never charts points from the future',()=>{
 const parsed=parseTelemetryReading(reading());
 assert.equal(parsed.series.cpu.length,2);
 assert.equal(parsed.series.gpuUsage.length,0);
 assert.throws(()=>parseTelemetryReading(reading({window:'5s'})),/Invalid/);
 const outOfOrder=reading();
 (outOfOrder.series as any).cpu=[{t:2,v:1},{t:1,v:1}];
 assert.throws(()=>parseTelemetryReading(outOfOrder),/Invalid/);
 const future=reading();
 (future.series as any).cpu=[{t:1758000000001,v:1}];
 assert.throws(()=>parseTelemetryReading(future),/Invalid/);
 const huge=reading();
 (huge.series as any).cpu=Array.from({length:1601},(_,i)=>({t:i,v:1}));
 assert.throws(()=>parseTelemetryReading(huge),/Invalid/);
});

const capacity=(overrides:Record<string,unknown>={})=>({
 enabled:true,
 hardware:{configured:'auto',effective:'software',device:'',probes:[{backend:'software',supported:true,detail:'Always available.'}]},
 sessions:{active:null,hardware:null,software:null,background:null,limits:{concurrent:0,hardware:0,software:0,background:0}},
 hdrToneMapping:{status:'disabled',detail:'HDR sources are delivered without tone mapping.'},
 directStreamRemux:true,x264Preset:'veryfast',planningPolicy:'maximum_fidelity',throttleBufferSeconds:60,playedRetentionSeconds:180,
 temporaryDirectory:{path:'/var/portico/hls',ready:true,freeBytes:1024},
 presets:[{id:'1080p',label:'1080p',height:1080,videoKbps:10000,audioKbps:256}],
 dependencies:{ffmpeg:{path:'/usr/bin/ffmpeg',version:'ffmpeg version 7.1',ok:true},ffprobe:{path:'/usr/bin/ffprobe',version:'ffprobe version 7.1',ok:true}},
 warnings:[],observedAt:1758000000000,...overrides});

test('capacity keeps null session counts distinct from zero and rejects bad shapes',()=>{
 const parsed=parseTranscodeCapacity(capacity());
 assert.equal(parsed.sessions.active,null);
 assert.equal(parsed.sessions.limits.concurrent,0);
 assert.equal(parsed.dependencies.ffmpeg.ok,true);
 assert.throws(()=>parseTranscodeCapacity(capacity({hdrToneMapping:{status:'maybe',detail:''}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({sessions:{active:-1,hardware:null,software:null,background:null,limits:{concurrent:0,hardware:0,software:0,background:0}}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({temporaryDirectory:{path:'/x',ready:'yes',freeBytes:null}})),/Invalid/);
});

test('attention items must be actionable and the list stays bounded',()=>{
 const items=parseAttention({items:[{id:'ffmpeg',severity:'critical',title:'ffmpeg is missing',detail:'Conversions depend on it.',action:{kind:'dependency',target:'ffmpeg'}}]});
 assert.equal(items[0].severity,'critical');
 assert.throws(()=>parseAttention({items:[{id:'x',severity:'urgent',title:'t',detail:'d',action:{kind:'k',target:''}}]}),/Invalid/);
 assert.throws(()=>parseAttention({items:[{id:'x',severity:'info',title:'',detail:'d',action:{kind:'k',target:''}}]}),/Invalid/);
 assert.throws(()=>parseAttention({items:Array.from({length:51},()=>({id:'x',severity:'info',title:'t',detail:'d',action:{kind:'k',target:''}}))}),/Invalid/);
});

const entry=(overrides:Record<string,unknown>={})=>({id:'play-1',viewer:'Justin',authority:'local',accountId:'owner',profileId:'primary',
 title:'A film',mediaKind:'movie',libraryName:'Films',itemId:'item-1',startedAt:1758000000000,endedAt:null,durationSeconds:null,
 deliveryMode:'hls',deliveryStrategy:'video_conversion',qualityMode:'automatic',state:'active',...overrides});

test('playback history keeps a running session open rather than zero length',()=>{
 const page=parsePlaybackHistory({items:[entry(),entry({id:'play-2',endedAt:1758000042000,durationSeconds:42,state:'ended'})],nextCursor:'t:1758000000000:play-2'});
 assert.equal(page.items[0].durationSeconds,null);
 assert.equal(page.items[1].durationSeconds,42);
 assert.equal(page.nextCursor,'t:1758000000000:play-2');
 assert.throws(()=>parsePlaybackHistory({items:[entry({endedAt:1,durationSeconds:0})],nextCursor:''}),/Invalid/);
 assert.throws(()=>parsePlaybackHistory({items:[entry({durationSeconds:5})],nextCursor:''}),/Invalid/);
  // B8b: the history cursor is opaque (t:<ms>:<id>); only its type and length are checked.
  assert.equal(parsePlaybackHistory({items:[entry()],nextCursor:'t:1758000000000:play-1'}).nextCursor,'t:1758000000000:play-1');
  assert.throws(()=>parsePlaybackHistory({items:[entry()],nextCursor:'x'.repeat(201)}),/Invalid/);
  assert.throws(()=>parsePlaybackHistory({items:[entry()],nextCursor:12}),/Invalid/);
});
