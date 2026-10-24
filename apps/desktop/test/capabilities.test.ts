import {test} from 'node:test';
import assert from 'node:assert/strict';
import {desktopCapabilities} from '../src/capabilities.ts';
test('native decoder inventory never grants browser, hardware, HDR, object audio or passthrough facts',()=>{
 const document=desktopCapabilities({version:'mpv 0.41.0',decoders:['h264','hevc','aac','flac','dca','truehd'],path:'/private'},'linux','0.1.0');
 const engine=document.engines.native;
 assert.deepEqual(engine.video?.map(value=>value.codec),['h264','hevc']);assert.deepEqual(engine.audio?.map(value=>value.codec),['aac','flac']);
 assert.deepEqual(engine.video?.map(value=>value.dynamicRanges),[['sdr'],['sdr']]);assert.deepEqual(engine.audioPassthrough,[]);assert.equal(document.audioOutput,undefined);assert.equal(document.display,undefined);
 assert.deepEqual(engine.transports?.map(value=>value.transport),['direct']);assert.deepEqual(engine.subtitleRendering,[]);assert.equal(engine.toneMapsHdr,false);
 assert.doesNotMatch(JSON.stringify(document),/path|private|maxWidth|maxHeight|sampleRate|atmos|dtsx|dts_hd|dolby_vision|hdr10/);
});
test('native missing, malformed or unreported inventory cannot become a declared fallback',()=>{
 for(const value of [null,{},[],{version:'mpv 0.41.0',decoders:[]},{version:'mpv /private',decoders:['aac']},{version:'mpv 0.41.0',decoders:['../aac'] }])assert.throws(()=>desktopCapabilities(value,'macos','0.1.0'),/engine_unavailable/);
});
test('original WavPack requires the actual decoder; it supplies no losslessness or output/hardware facts',()=>{
 for(const present of [false,true]){
  const profile=desktopCapabilities({version:'mpv 0.41.0',decoders:['flac',...(present?['wavpack']:[])]},'linux','0.1.0');
  const native=profile.engines.native;
  assert.equal(native.audio?.some(a=>a.codec==='wavpack'),present);
  assert.equal(native.transports?.[0]?.containers?.includes('wv'),present);
  assert.equal(native.transports?.[0]?.audio?.includes('wavpack'),present);
  assert.equal(profile.audioOutput,undefined);
  assert.doesNotMatch(JSON.stringify(native.audio?.find(a=>a.codec==='wavpack')??{}),/lossless|bitPerfect|sampleRate|output|passthrough|objectAudio/);
 }
});
