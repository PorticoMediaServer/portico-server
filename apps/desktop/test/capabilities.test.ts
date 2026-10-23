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
