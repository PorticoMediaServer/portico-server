import test from 'node:test';
import assert from 'node:assert/strict';
import {androidDeclaredProfile,androidProfile,appleDeclaredProfile,appleProfile,castDeclaredProfile,clientProfileProblem,ClientProfilePublisher,fireTvDeclaredProfile,probeWebProfile,reportRouteFailure,rokuDeclaredProfile,tizenDeclaredProfile,vegaDeclaredProfile,webDeclaredProfile,webosDeclaredProfile,type WebProbeEnvironment} from '../src/client-profile.ts';

test('every declared table is a document the server accepts',()=>{
 const tables=[webDeclaredProfile(),appleDeclaredProfile({television:true}),appleDeclaredProfile({television:false}),androidDeclaredProfile(),androidDeclaredProfile({television:true}),fireTvDeclaredProfile(),vegaDeclaredProfile(),webosDeclaredProfile(),tizenDeclaredProfile(),rokuDeclaredProfile(),rokuDeclaredProfile({uhd:false}),...(['chromecast','chromecast_ultra','google_tv_4k','google_tv_hd','nest_hub','unknown'] as const).map(castDeclaredProfile)];
 for(const p of tables){
  assert.equal(clientProfileProblem(p),null,p.client.family+' '+(p.client.model??''));
  assert.ok(p.transports!.some(t=>t.transport==='hls_ts'&&t.video!.includes('h264')&&t.audio!.includes('aac')),'every device can take the last-resort conversion');
 }
 assert.equal(appleDeclaredProfile({television:true}).transports!.some(t=>t.containers?.includes('mkv')),false,'AVPlayer has never played Matroska');
 assert.ok(castDeclaredProfile('chromecast').video!.every(v=>v.codec!=='hevc'));
 assert.ok(castDeclaredProfile('google_tv_4k').video!.find(v=>v.codec==='hevc')!.dynamicRanges!.includes('dolby_vision'));
});

test('the validator mirrors the server',()=>{
 const p=webDeclaredProfile();
 assert.equal(clientProfileProblem({...p,version:2 as any}),'version');
 assert.equal(clientProfileProblem({...p,client:{family:'Web Browser'}}),'client.family');
 assert.equal(clientProfileProblem({...p,evidence:'trust me' as any}),'evidence');
 assert.equal(clientProfileProblem({...p,display:{dynamicRanges:['hdr11' as any]}}),'display');
 assert.equal(clientProfileProblem({...p,transports:[{transport:'rtsp' as any}]}),'transports');
});

const chromeLike=(extra:Partial<WebProbeEnvironment>={}):WebProbeEnvironment=>({
 canPlayType:type=>/avc1\.64|mp4a|audio\/mpeg|opus|vorbis|flac|vp09\.00|av01\.0\.08M\.08/.test(type)?'probably':'',
 mediaSourceSupports:type=>/avc1\.64|mp4a|audio\/mpeg|opus|flac|vp09|av01/i.test(type),
 decodingInfo:async q=>({supported:!/hvc1|dvh1/.test(q.contentType)&&q.transferFunction===undefined,smooth:true,powerEfficient:true}),
 matchMedia:()=>false,screen:{width:2560,height:1440},audioOutputChannels:2,embeddedAudioTracks:false,identity:{platform:'chrome'},...extra});

test('a Chromium desktop: no HEVC, AV1 and VP9 found, stereo out',async()=>{
 const p=await probeWebProfile(chromeLike());
 assert.equal(clientProfileProblem(p),null);
 assert.equal(p.evidence,'probed');
 assert.deepEqual(p.video!.map(v=>v.codec).sort(),['av1','h264','vp9']);
 assert.equal(p.video!.find(v=>v.codec==='h264')!.bitDepths!.includes(10),false,'Hi10P is not decoded by browsers');
 assert.equal(p.audioOutput!.maxChannels,2);
 assert.ok(p.transports!.find(t=>t.transport==='direct'&&t.containers!.includes('webm'))!.video!.includes('vp9'));
 assert.equal(p.transports!.some(t=>t.video?.includes('hevc')),false);
 assert.equal(p.embeddedAudioSwitching,false);
});

test('Safari on an HDR Mac: HEVC, HDR10, Dolby Vision, native HLS and surround out',async()=>{
 const p=await probeWebProfile({
  canPlayType:type=>/avc1\.64|hvc1|dvh1|mp4a|ac-3|ec-3|alac|audio\/mpeg|flac|mpegurl/.test(type)?'probably':'',
  mediaSourceSupports:type=>/avc1\.64|hvc1|dvh1|mp4a|ac-3|ec-3|flac/.test(type),
  decodingInfo:async()=>({supported:true,smooth:true,powerEfficient:true}),
  matchMedia:q=>/dynamic-range: high|p3/.test(q),screen:{width:3456,height:2234},audioOutputChannels:6,embeddedAudioTracks:true,identity:{platform:'safari'}});
 assert.equal(clientProfileProblem(p),null);
 const hevc=p.video!.find(v=>v.codec==='hevc')!;
 assert.deepEqual(hevc.bitDepths,[8,10]);
 assert.ok(hevc.dynamicRanges!.includes('hdr10')&&hevc.dynamicRanges!.includes('dolby_vision'));
 assert.deepEqual(hevc.dolbyVisionProfiles,[5,8]);
 assert.equal(p.audioOutput!.maxChannels,6);
 assert.ok(p.transports!.find(t=>t.transport==='hls_fmp4')!.video!.includes('hevc'));
 assert.ok(p.transports!.find(t=>t.transport==='hls_ts')!.audio!.includes('eac3'));
 assert.equal(p.embeddedAudioSwitching,true);
});

test('a probe that throws or hangs leaves the declared floor',async()=>{
 const p=await probeWebProfile({canPlayType:()=>{throw new Error('x');},mediaSourceSupports:()=>{throw new Error('y');},decodingInfo:()=>new Promise(()=>{}),matchMedia:()=>{throw new Error('z');}});
 assert.equal(clientProfileProblem(p),null);
 assert.ok(p.video!.some(v=>v.codec==='h264'));
 assert.ok(p.transports!.some(t=>t.transport==='direct'&&t.video!.includes('h264')));
});

test('Apple facts widen and narrow the declared table',()=>{
 const options={television:true,osVersion:'18.0'};
 const tv=appleProfile(options,{model:'AppleTV14,1',hardwareDecode:{hevc:true,av1:false,dolbyVision:true},hdrModes:{hdr10:true,hlg:true,dolbyVision:true},eligibleForHDRPlayback:true,screen:{width:3840,height:2160,maxFrameRate:60},audio:{maxOutputChannels:8,route:'HDMI',spatial:false}});
 assert.equal(clientProfileProblem(tv),null);
 assert.equal(tv.evidence,'mixed');
 assert.deepEqual(tv.video!.find(v=>v.codec==='hevc')!.dolbyVisionProfiles,[5,8]);
 assert.equal(tv.audioOutput!.maxChannels,8);assert.equal(tv.audioOutput!.route,'hdmi');
 const sdr=appleProfile(options,{hdrModes:{hdr10:true},eligibleForHDRPlayback:false,audio:{maxOutputChannels:2}});
 assert.deepEqual(sdr.video!.find(v=>v.codec==='hevc')!.dynamicRanges,['sdr'],'a display not in an HDR mode is sent SDR');
 const m3=appleProfile({television:false},{hardwareDecode:{hevc:true,av1:true}});
 assert.ok(m3.transports!.find(t=>t.transport==='hls_fmp4')!.video!.includes('av1'));
 assert.equal(appleProfile(options,null).evidence,'declared');
 // COMPAT-02: hardware HEVC decodes HDR10/HLG (and DV with its decoder) for an SDR screen too; the display stays SDR.
 const sdrScreen=appleProfile(options,{hardwareDecode:{hevc:true,dolbyVision:true},hdrModes:{},eligibleForHDRPlayback:false,audio:{maxOutputChannels:2}});
 assert.deepEqual(sdrScreen.video!.find(v=>v.codec==='hevc')!.dynamicRanges,['sdr','hdr10','hlg','dolby_vision']);
 assert.deepEqual(sdrScreen.display!.dynamicRanges,['sdr']);
 // COMPAT-06: the Apple TV HD decodes H.264 up to 1080p.
 const hd=appleProfile(options,{model:'AppleTV5,3',hardwareDecode:{hevc:false}});
 assert.equal(hd.video!.find(v=>v.codec==='h264')!.maxHeight,1080);
 assert.equal(clientProfileProblem(hd),null);
});

test('Android facts replace the decoder table and add passthrough',()=>{
 const p=androidProfile({television:true},{model:'SHIELD',decoders:[{codec:'h264',maxWidth:4096,maxHeight:2160,maxLevel:52},{codec:'hevc',tenBit:true,maxWidth:4096,maxHeight:2160,hdr:['hdr10','dolby_vision'],dolbyVisionProfiles:[5,7,8]},{codec:'vc1',maxWidth:1920,maxHeight:1080}],audioDecoders:[{codec:'aac',maxChannels:8},{codec:'flac',maxChannels:8}],displayHdr:['hdr10','dolby_vision'],passthrough:[{codec:'truehd',maxChannels:8,objectAudio:['atmos']},{codec:'dts',maxChannels:8,objectAudio:['dtsx']},{codec:'eac3',maxChannels:8,objectAudio:['atmos']}],audio:{maxOutputChannels:8,route:'hdmi'}});
 assert.equal(clientProfileProblem(p),null);
 assert.ok(p.transports!.find(t=>t.transport==='direct'&&t.containers!.includes('mkv'))!.audio!.includes('truehd'));
 assert.equal(p.audio!.find(a=>a.codec==='truehd')!.passthrough,true);
 assert.deepEqual(p.video!.find(v=>v.codec==='hevc')!.dolbyVisionProfiles,[5,7,8]);
 assert.equal(androidProfile({},null).evidence,'declared');
});

test('publishing happens once per document per sign-in and never throws',async()=>{
 let calls=0,scope='a',fail=false;
 const publisher=new ClientProfilePublisher(async(path,method,body)=>{calls++;if(fail)throw new Error('offline');assert.equal(path,'/v1/playback/client-profile');assert.equal(method,'PUT');return {version:1,revision:'r1',summary:{family:(body as any).client.family,evidence:'declared',revision:'r1',published:true}};},()=>scope);
 const profile=webDeclaredProfile();
 assert.equal((await publisher.publish(profile))!.revision,'r1');
 assert.equal(await publisher.publish(profile),null);assert.equal(calls,1);
 scope='b';await publisher.publish(profile);assert.equal(calls,2);
 fail=true;publisher.reset();assert.equal(await publisher.publish(profile),null);assert.equal(calls,3);
 fail=false;assert.ok(await publisher.publish(profile),'a failed publication is tried again');
 assert.equal(await publisher.publish({...profile,version:3 as any}),null);assert.equal(calls,4);
});

test('a route failure report is bounded and never throws',async()=>{
 let sent:any;
 const ok=await reportRouteFailure(async(_p,_m,body)=>{sent=body;return {route:'original',escalates:true};},'session_1','decode_error','MEDIA_ERR_DECODE\n'+'x'.repeat(900));
 assert.deepEqual(ok,{route:'original',escalates:true});
 assert.equal(sent.detail.length,500);assert.equal(/[\x00-\x1f]/.test(sent.detail),false);
 assert.equal(await reportRouteFailure(async()=>{throw new Error('offline');},'session_1','decode_error'),null);
 assert.equal(await reportRouteFailure(async()=>({}),'../etc','decode_error'),null);
});

test('COMPAT-05: an SDR display gets HDR10 only from an engine that tone-maps it',async()=>{
 const base={
  canPlayType:(type:string)=>/avc1|hvc1|mp4a/.test(type)?'probably':'',
  mediaSourceSupports:(type:string)=>/avc1|hvc1|mp4a/.test(type),
  decodingInfo:async()=>({supported:true,smooth:true,powerEfficient:true}),
  matchMedia:()=>false,identity:{platform:'web'}};
 const firefox=await probeWebProfile(base);
 assert.deepEqual(firefox.video!.find(v=>v.codec==='hevc')!.dynamicRanges,['sdr']);
 const safari=await probeWebProfile({...base,tonemapsHdr:true});
 assert.ok(safari.video!.find(v=>v.codec==='hevc')!.dynamicRanges!.includes('hdr10'));
 const hdrScreen=await probeWebProfile({...base,matchMedia:(q:string)=>/dynamic-range: high/.test(q)});
 assert.ok(hdrScreen.video!.find(v=>v.codec==='hevc')!.dynamicRanges!.includes('hdr10'));
});
