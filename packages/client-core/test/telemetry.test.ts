import assert from 'node:assert/strict';
import test from 'node:test';
import {METRIC_NAMES,parseMetric,parseTelemetryReading,parseTelemetrySample,parseTranscodeCapacity} from '../src/telemetry.ts';
import {parseAttention} from '../src/attention.ts';
import {parsePlaybackHistory} from '../src/playback-history.ts';
import {resetSkippedParts,skippedParts} from '../src/tolerant.ts';

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
 // A status this app does not know reads as unavailable, and its value is never drawn.
 assert.deepEqual(parseMetric({status:'guessed',value:1,detail:'Sampled.',unit:'new'}),{status:'unavailable',value:0,detail:'Sampled.'});
 assert.throws(()=>parseMetric({status:'available',value:'high'}),/Invalid/);
 assert.throws(()=>parseMetric({status:'available',value:Infinity}),/Invalid/);
 assert.throws(()=>parseMetric({status:'available',value:1,detail:'x'.repeat(513)}),/Invalid/);
 assert.throws(()=>parseMetric(null),/Invalid/);
});

test('a metric a sample leaves out or garbles reads as unavailable, and the rest of the sample is kept',()=>{
 assert.equal(parseTelemetrySample(sample()).metrics.cpu.value,12.5);
 resetSkippedParts();
 const partial=sample({gpuDevice:'x'.repeat(201),gpuProvider:'vendor',fanSpeed:'new'});
 const published=partial.metrics as Record<string,unknown>;
 delete published.gpuEncoder;
 Object.assign(published,{memory:{status:'available',value:'high'},netIn:{status:'unavailable',value:99},diskRead:{status:'sampled',value:5},thermal:{status:'available',value:60}});
 const parsed=parseTelemetrySample(partial);
 assert.equal(parsed.metrics.cpu.value,12.5);
 for(const name of ['gpuEncoder','memory','netIn','diskRead'] as const)assert.deepEqual([parsed.metrics[name].status,parsed.metrics[name].value],['unavailable',0],name);
 assert.equal('thermal' in parsed.metrics,false);
 assert.equal(parsed.gpuDevice,undefined);assert.equal(parsed.gpuProvider,'vendor');
 assert.deepEqual(skippedParts().map(p=>p.document+':'+p.part+':'+p.count).sort(),['telemetry:gpuDevice:1','telemetry:metric.status:1','telemetry:metric:3']);
 assert.throws(()=>parseTelemetrySample(sample({memoryUsedBytes:9})),/Invalid/);
 assert.throws(()=>parseTelemetrySample(sample({at:-1})),/Invalid/);
 assert.throws(()=>parseTelemetrySample(sample({metrics:[]})),/Invalid/);
});

test('a reading is bounded, ordered and never charts points from the future',()=>{
 const parsed=parseTelemetryReading(reading());
 assert.equal(parsed.series.cpu.length,2);
 assert.equal(parsed.series.gpuUsage.length,0);
 assert.throws(()=>parseTelemetryReading(reading({window:'5s'})),/Invalid/);
 // A point that is out of order, from the future or not a number is left out; the line keeps the rest.
 const ragged=reading({resolution:'new'});
 (ragged.series as any).cpu=[{t:2,v:1,source:'new'},{t:1,v:1},{t:3,v:'high'},{t:1758000000001,v:1},'gap',{t:4,v:2}];
 (ragged.series as any).thermal=[{t:1,v:1}];
 (ragged.status as any).memory={status:'sampled',value:3};
 const kept=parseTelemetryReading(ragged);
 assert.deepEqual(kept.series.cpu,[{t:2,v:1},{t:4,v:2}]);
 assert.equal('thermal' in kept.series,false);
 assert.deepEqual(kept.status.memory,{status:'unavailable',value:0});
 const huge=reading();
 (huge.series as any).cpu=Array.from({length:1601},(_,i)=>({t:i,v:1}));
 assert.throws(()=>parseTelemetryReading(huge),/Invalid/);
 const missing=reading();
 delete (missing.series as any).netOut;
 assert.throws(()=>parseTelemetryReading(missing),/Invalid/);
 assert.throws(()=>parseTelemetryReading(reading({observedAt:'now'})),/Invalid/);
 assert.throws(()=>parseTelemetryReading(reading({series:[]})),/Invalid/);
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
 assert.throws(()=>parseTranscodeCapacity(capacity({sessions:{active:-1,hardware:null,software:null,background:null,limits:{concurrent:0,hardware:0,software:0,background:0}}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({sessions:{active:null,hardware:null,software:null,background:null,limits:{concurrent:'many',hardware:0,software:0,background:0}}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({sessions:{active:null,hardware:null,software:null,background:null}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({temporaryDirectory:{path:'/x',ready:'yes',freeBytes:null}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({enabled:'yes'})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({x264Preset:'x'.repeat(41)})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({hdrToneMapping:{status:'available',detail:'x'.repeat(513)}})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({presets:Array.from({length:21},()=>capacity().presets[0])})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({warnings:Array.from({length:21},()=>'w')})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({warnings:'none'})),/Invalid/);
 assert.throws(()=>parseTranscodeCapacity(capacity({dependencies:Object.fromEntries(Array.from({length:9},(_,i)=>['tool'+i,{path:'/t',version:'1',ok:true}]))})),/Invalid/);
 const crowded=capacity();(crowded.hardware as any).probes=Array.from({length:21},()=>capacity().hardware.probes[0]);
 assert.throws(()=>parseTranscodeCapacity(crowded),/Invalid/);
});

test('capacity leaves out a probe, preset, tool or warning it cannot read and keeps the rest',()=>{
 resetSkippedParts();
 const raw=capacity({
  hdrToneMapping:{status:'maybe',detail:'Depends on the display.',engine:'new'},
  presets:[{id:'1080p',label:'1080p',height:1080,videoKbps:10000,audioKbps:256,codec:'new'},{id:'8k',label:'8K',height:'tall',videoKbps:1,audioKbps:1},'preset'],
  dependencies:{ffmpeg:{path:'/usr/bin/ffmpeg',version:'ffmpeg version 7.1',ok:true},ffprobe:{path:'/usr/bin/ffprobe',version:7,ok:true},vmaf:'present'},
  warnings:['Disk is nearly full.',7,'x'.repeat(513)],queueDepth:4});
 (raw.hardware as any).probes=[{backend:'software',supported:true,detail:'Always available.'},{backend:'vulkan',supported:'partly',detail:''},null];
 const parsed=parseTranscodeCapacity(raw);
 assert.deepEqual(parsed.hardware.probes.map(p=>p.backend),['software']);
 assert.equal(parsed.hardware.effective,'software');
 assert.deepEqual(parsed.hdrToneMapping.status,'unavailable');assert.equal(parsed.hdrToneMapping.detail,'Depends on the display.');
 assert.deepEqual(parsed.presets.map(p=>p.id),['1080p']);
 assert.deepEqual(Object.keys(parsed.dependencies),['ffmpeg']);
 assert.deepEqual(parsed.warnings,['Disk is nearly full.']);
 assert.equal(parsed.sessions.active,null);assert.equal(parsed.throttleBufferSeconds,60);
 assert.deepEqual(skippedParts().map(p=>p.document+':'+p.part+':'+p.count).sort(),['transcode-capacity:dependencies:2','transcode-capacity:hardware.probes:2','transcode-capacity:hdrToneMapping.status:1','transcode-capacity:presets:2','transcode-capacity:warnings:2']);
});

test('attention items must be actionable and the list stays bounded',()=>{
 const items=parseAttention({items:[{id:'ffmpeg',severity:'critical',title:'ffmpeg is missing',detail:'Conversions depend on it.',action:{kind:'dependency',target:'ffmpeg'}}]});
 assert.equal(items[0].severity,'critical');
 // A severity this app does not know reads as the plainest one, and an item with nothing to say is left out on its own.
 assert.equal(parseAttention({items:[{id:'x',severity:'urgent',title:'t',detail:'d',action:{kind:'k',target:''}}]})[0].severity,'info');
 assert.deepEqual(parseAttention({items:[{id:'x',severity:'info',title:'',detail:'d',action:{kind:'k',target:''}},items[0]]}).map(item=>item.id),['ffmpeg']);
 assert.throws(()=>parseAttention({items:Array.from({length:51},()=>({id:'x',severity:'info',title:'t',detail:'d',action:{kind:'k',target:''}}))}),/Invalid/);
});

const entry=(overrides:Record<string,unknown>={})=>({id:'play-1',viewer:'Justin',authority:'local',accountId:'owner',profileId:'primary',
 title:'A film',mediaKind:'movie',libraryName:'Films',itemId:'item-1',startedAt:'2025-09-16T05:20:00Z',endedAt:null,durationMs:null,
 deliveryMode:'hls',deliveryStrategy:'video_conversion',qualityMode:'automatic',state:'active',...overrides});

test('playback history keeps a running session open rather than zero length',()=>{
 const page=parsePlaybackHistory({items:[entry(),entry({id:'play-2',endedAt:'2025-09-16T05:20:42Z',durationMs:42000,state:'ended'})],nextCursor:'t:1758000000000:play-2'});
 assert.equal(page.items[0].durationSeconds,null);
 assert.equal(page.items[1].durationSeconds,42);
 assert.equal(page.nextCursor,'t:1758000000000:play-2');
 assert.deepEqual(parsePlaybackHistory({items:[entry({endedAt:'1970-01-01T00:00:00.001Z',durationMs:0})],nextCursor:''}).items,[]);
 assert.deepEqual(parsePlaybackHistory({items:[entry({durationMs:5000})],nextCursor:''}).items,[]);
  // B8b: the history cursor is opaque (t:<ms>:<id>); only its type and length are checked.
  assert.equal(parsePlaybackHistory({items:[entry()],nextCursor:'t:1758000000000:play-1'}).nextCursor,'t:1758000000000:play-1');
  assert.throws(()=>parsePlaybackHistory({items:[entry()],nextCursor:'x'.repeat(201)}),/Invalid/);
  assert.throws(()=>parsePlaybackHistory({items:[entry()],nextCursor:12}),/Invalid/);
});
