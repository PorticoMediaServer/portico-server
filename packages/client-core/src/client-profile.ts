/** What this device can decode, display and output — the document a client
 * publishes to `PUT /v1/playback/client-profile` so the server can choose between
 * playing a file as it is, repackaging it and converting it *for this device*.
 *
 * Two sources of truth meet here. A **declared** table says what a platform's
 * player engine is known to take (AVPlayer has never played a Matroska file, and
 * no probe will change that). A **probed** fact was asked of the actual device at
 * run time (this browser decodes HEVC, this television's HDMI sink takes eight
 * channels, this Apple TV is set to a Dolby Vision display mode). A profile starts
 * from the declared table for its family and is narrowed or widened by what the
 * probe finds; every codec entry records which of the two it rests on, so a wrong
 * decision can be traced to its source.
 *
 * Wire version 1. It grows by addition; the server ignores fields it does not
 * know and refuses a different `version`. Codec names are FFmpeg's. Levels use
 * FFmpeg's numbering (H.264 4.1 is 41, HEVC 5.1 is 153). A zero or an empty list
 * means "not stated", which never rejects a route. */

export const clientProfileVersion=1 as const;
export const maxClientProfileBytes=32*1024;

export type ProfileEvidence='declared'|'probed'|'mixed';
export type DynamicRange='sdr'|'hdr10'|'hdr10plus'|'hlg'|'dolby_vision';
export type ProfileTransportKind='direct'|'hls_ts'|'hls_fmp4';

export type ClientIdentity={family:string;platform?:string;platformVersion?:string;os?:string;osVersion?:string;model?:string;app?:string;appVersion?:string;engine?:string;engineVersion?:string};
export type ClientDisplay={width?:number;height?:number;dynamicRanges?:DynamicRange[];maxFrameRate?:number};
export type ClientVideoCodec={codec:string;profiles?:string[];maxLevel?:number;bitDepths?:number[];maxWidth?:number;maxHeight?:number;maxFrameRate?:number;maxBitrateBps?:number;dynamicRanges?:DynamicRange[];dolbyVisionProfiles?:number[];interlaced?:boolean;evidence?:ProfileEvidence};
export type ClientAudioCodec={codec:string;maxChannels?:number;maxSampleRate?:number;objectAudio?:('atmos'|'dtsx')[];passthrough?:boolean;evidence?:ProfileEvidence};
export type ClientTransport={transport:ProfileTransportKind;containers?:string[];video?:string[];audio?:string[]};
export type ClientAudioOutput={route?:string;maxChannels?:number;spatial?:boolean};
export type ClientSubtitles={text?:string[];styled?:string[];bitmap?:string[]};
export type ClientProfile={
 version:typeof clientProfileVersion;client:ClientIdentity;evidence:ProfileEvidence;
 display?:ClientDisplay;video?:ClientVideoCodec[];audio?:ClientAudioCodec[];audioOutput?:ClientAudioOutput;
 transports?:ClientTransport[];subtitles?:ClientSubtitles;maxBitrateBps?:number;embeddedAudioSwitching?:boolean;
 /** What the client's own music engine decodes (spec §3/§18.1 `audioDecode`), for a server that reads
  * it from this document. Grows by addition: servers that don't know it ignore it. */
 audioDecode?:ClientAudioDecode[];
};
export type ClientAudioDecode={codec:string;containers:string[];maxSampleRate?:number;sampleRates?:number[];maxChannels?:number;maxBitDepth?:number;via?:string};

const token=/^[a-z0-9][a-z0-9_.+-]{0,31}$/;
const ranges:readonly string[]=['sdr','hdr10','hdr10plus','hlg','dolby_vision'];
const evidences:readonly string[]=['declared','probed','mixed'];
const text=(v:unknown,max:number)=>v===undefined||typeof v==='string'&&v.length<=max&&!/[\x00-\x1f\x7f]/.test(v);
const int=(v:unknown,max:number)=>v===undefined||Number.isSafeInteger(v)&&Number(v)>=0&&Number(v)<=max;
const num=(v:unknown,max:number)=>v===undefined||typeof v==='number'&&Number.isFinite(v)&&v>=0&&v<=max;
const tokens=(v:unknown,max:number)=>v===undefined||Array.isArray(v)&&v.length<=max&&v.every(x=>typeof x==='string'&&token.test(x));
const rangeList=(v:unknown)=>v===undefined||Array.isArray(v)&&v.length<=8&&v.every(x=>ranges.includes(x as string));

/** Mirrors the server's admission rules, so a profile that passes here is one
 * the server accepts. Returns the first problem found, or null. */
export function clientProfileProblem(p:ClientProfile):string|null{
 if(!p||p.version!==clientProfileVersion)return 'version';
 if(!p.client||typeof p.client.family!=='string'||!token.test(p.client.family))return 'client.family';
 for(const key of ['platform','platformVersion','os','osVersion','model','app','appVersion','engine','engineVersion'] as const)if(!text(p.client[key],64))return 'client.'+key;
 if(!evidences.includes(p.evidence))return 'evidence';
 const d=p.display??{};
 if(!int(d.width,32768)||!int(d.height,32768)||!num(d.maxFrameRate,1000)||!rangeList(d.dynamicRanges))return 'display';
 if((p.video?.length??0)>32||(p.audio?.length??0)>48||(p.transports?.length??0)>16||!int(p.maxBitrateBps,2_000_000_000))return 'bounds';
 for(const v of p.video??[]){
  if(typeof v.codec!=='string'||!token.test(v.codec)||(v.profiles??[]).length>32||!(v.profiles??[]).every(x=>typeof x==='string'&&token.test(x.trim().toLowerCase().replace(/ /g,'_')))||!int(v.maxLevel,1000)||!int(v.maxWidth,32768)||!int(v.maxHeight,32768)||!num(v.maxFrameRate,1000)||!int(v.maxBitrateBps,2_000_000_000)||!rangeList(v.dynamicRanges))return 'video.'+v.codec;
  if((v.bitDepths??[]).length>8||!(v.bitDepths??[]).every(x=>Number.isInteger(x)&&x>=1&&x<=16))return 'video.'+v.codec+'.bitDepths';
  if((v.dolbyVisionProfiles??[]).length>16||!(v.dolbyVisionProfiles??[]).every(x=>Number.isInteger(x)&&x>=1&&x<=31))return 'video.'+v.codec+'.dolbyVisionProfiles';
  if(v.evidence!==undefined&&!evidences.includes(v.evidence))return 'video.'+v.codec+'.evidence';
 }
 for(const a of p.audio??[]){
  if(typeof a.codec!=='string'||!token.test(a.codec)||!int(a.maxChannels,64)||!int(a.maxSampleRate,1<<22)||!tokens(a.objectAudio,4))return 'audio.'+a.codec;
  if(a.evidence!==undefined&&!evidences.includes(a.evidence))return 'audio.'+a.codec+'.evidence';
 }
 if((p.audioDecode?.length??0)>32)return 'audioDecode';
 for(const a of p.audioDecode??[]){
  if(typeof a.codec!=='string'||!token.test(a.codec)||!tokens(a.containers,16)||!int(a.maxSampleRate,1<<22)||!int(a.maxChannels,64)||!int(a.maxBitDepth,64)||(a.sampleRates!==undefined&&!(Array.isArray(a.sampleRates)&&a.sampleRates.length<=32&&a.sampleRates.every(r=>Number.isSafeInteger(r)&&r>0&&r<=1<<22)))||!text(a.via,32))return 'audioDecode.'+a.codec;
 }
 for(const t of p.transports??[]){
  if(!['direct','hls_ts','hls_fmp4'].includes(t.transport)||!tokens(t.containers,32)||!tokens(t.video,32)||!tokens(t.audio,48))return 'transports';
 }
 const o=p.audioOutput??{};
 if(o.route!==undefined&&o.route!==''&&!token.test(o.route)||!int(o.maxChannels,64))return 'audioOutput';
 const s=p.subtitles??{};
 if(!tokens(s.text,16)||!tokens(s.styled,16)||!tokens(s.bitmap,16))return 'subtitles';
 if(JSON.stringify(p).length>maxClientProfileBytes)return 'size';
 return null;
}

const v=(codec:string,more:Partial<ClientVideoCodec>={}):ClientVideoCodec=>({codec,evidence:'declared',...more});
const a=(codec:string,maxChannels:number,more:Partial<ClientAudioCodec>={}):ClientAudioCodec=>({codec,maxChannels,evidence:'declared',...more});
const audioFiles=(audio:string[],containers:string[]):ClientTransport=>({transport:'direct',containers,video:[],audio});
const textSubtitles:ClientSubtitles={text:['srt','vtt'],styled:[],bitmap:[]};

/* ------------------------------------------------------------------------- */
/* Declared tables, one per client family.                                     */
/* ------------------------------------------------------------------------- */

/** A browser playing through a `<video>` element and hls.js. This table is the
 * floor every browser meets; `probeWebProfile` is what finds HEVC, AV1, HDR and
 * surround where they exist. */
export function webDeclaredProfile(identity:Partial<ClientIdentity>={}):ClientProfile{
 return {
  version:1,evidence:'declared',client:{family:'web',engine:'hls.js',app:'portico-web',...identity},
  display:{dynamicRanges:['sdr']},
  video:[v('h264',{bitDepths:[8],maxWidth:4096,maxHeight:2160,maxFrameRate:60,dynamicRanges:['sdr']})],
  audio:[a('aac',2),a('mp3',2)],
  audioOutput:{maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v','mov'],video:['h264'],audio:['aac','mp3']},
   audioFiles(['aac','mp3'],['mp3','m4a','m4b','aac']),
   {transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:['aac','mp3']},
  ],
  subtitles:textSubtitles,
 };
}

/** AVPlayer on iOS, iPadOS and tvOS. AVFoundation plays MP4-family containers and
 * HLS and nothing else: Matroska, AVI and MPEG-TS files are always repackaged.
 * HEVC is universal on supported hardware; AV1, Dolby Vision and the display's
 * HDR modes are per device and come from the native probe. */
export function appleDeclaredProfile(options:{television:boolean;osVersion?:string;model?:string;appVersion?:string}):ClientProfile{
 const mp4Audio=['aac','mp3','ac3','eac3','alac','flac'];
 return {
  version:1,evidence:'declared',
  client:{family:'apple',platform:options.television?'tvos':'ios',os:options.television?'tvos':'ios',osVersion:options.osVersion,model:options.model,app:'portico-apple',appVersion:options.appVersion,engine:'avplayer'},
  display:{dynamicRanges:['sdr']},
  video:[
   v('h264',{bitDepths:[8],maxWidth:4096,maxHeight:2304,maxFrameRate:60,dynamicRanges:['sdr']}),
   v('hevc',{bitDepths:[8,10],maxWidth:4096,maxHeight:2304,maxFrameRate:60,dynamicRanges:['sdr']}),
  ],
  audio:[a('aac',8),a('mp3',2),a('ac3',6),a('eac3',8,{objectAudio:['atmos']}),a('alac',8),a('flac',8),a('pcm',8)],
  audioOutput:{maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v','mov'],video:['h264','hevc'],audio:mp4Audio},
   audioFiles(['aac','mp3','alac','flac','pcm'],['mp3','m4a','m4b','aac','flac','wav','aiff']),
   {transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:['aac','mp3','ac3','eac3']},
   {transport:'hls_fmp4',containers:['fmp4'],video:['h264','hevc'],audio:mp4Audio},
  ],
  subtitles:textSubtitles,
  // AVPlayer selects among a file's audio tracks through its media selection group.
  embeddedAudioSwitching:true,
 };
}

/** Media3 / ExoPlayer on Android phones and tablets. The extractor set is wide
 * (Matroska, MP4, TS, WebM, FLAC, Ogg, WAV), so most files play as they are;
 * which *codecs* decode is per device and is what `MediaCodecList` answers. */
export function androidDeclaredProfile(options:{television?:boolean;family?:'android'|'androidtv'|'firetv';osVersion?:string;model?:string}={}):ClientProfile{
 const family=options.family??(options.television?'androidtv':'android');
 const video=['h264','hevc','vp9','vp8','mpeg4'];
 const audio=['aac','mp3','opus','vorbis','flac','pcm'];
 return {
  version:1,evidence:'declared',client:{family,platform:'android',os:'android',osVersion:options.osVersion,model:options.model,engine:'exoplayer'},
  display:{dynamicRanges:['sdr']},
  video:[v('h264',{bitDepths:[8],maxWidth:1920,maxHeight:1080,maxFrameRate:60,dynamicRanges:['sdr']}),v('hevc',{bitDepths:[8],maxWidth:1920,maxHeight:1080,dynamicRanges:['sdr']}),v('vp9',{bitDepths:[8],maxWidth:1920,maxHeight:1080,dynamicRanges:['sdr']}),v('vp8',{bitDepths:[8]}),v('mpeg4',{bitDepths:[8]})],
  audio:[a('aac',6),a('mp3',2),a('opus',6),a('vorbis',6),a('flac',8),a('pcm',8)],
  audioOutput:{maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v','mov','mkv','webm','ts','m2ts'],video,audio},
   audioFiles(audio,['mp3','m4a','m4b','aac','flac','ogg','opus','wav']),
   {transport:'hls_ts',containers:['mpegts'],video:['h264','hevc'],audio:['aac','mp3','ac3','eac3']},
   {transport:'hls_fmp4',containers:['fmp4'],video:['h264','hevc','vp9','av1'],audio:['aac','mp3','ac3','eac3','flac','opus']},
  ],
  // ExoPlayer draws SubRip, WebVTT and SSA/ASS text itself, and PGS through its bitmap decoder.
  subtitles:{text:['srt','vtt'],styled:['ass','ssa'],bitmap:['pgs']},
  embeddedAudioSwitching:true,
 };
}

/** Amazon Fire TV (Fire OS, Android-based). As Android TV, with Dolby audio
 * passed to the receiver where the HDMI sink reports it. */
export const fireTvDeclaredProfile=(options:{osVersion?:string;model?:string}={})=>androidDeclaredProfile({...options,family:'firetv',television:true});

/** Amazon Vega OS (Fire TV without Android): a React Native app over Amazon's
 * W3C media player, which plays MP4 and HLS/DASH through MSE-like sources. It is
 * planned like a browser with HEVC and Dolby audio until the device is probed. */
export function vegaDeclaredProfile(options:{osVersion?:string;model?:string}={}):ClientProfile{
 const base=webDeclaredProfile({platform:'vega',os:'vega',osVersion:options.osVersion,model:options.model,engine:'w3cmedia',app:'portico-vega'});
 base.client.family='vega';
 base.video!.push(v('hevc',{bitDepths:[8,10],maxWidth:3840,maxHeight:2160,maxFrameRate:60,dynamicRanges:['sdr']}));
 base.audio!.push(a('ac3',6,{passthrough:true}),a('eac3',8,{passthrough:true,objectAudio:['atmos']}));
 base.transports=[
  {transport:'direct',containers:['mp4','m4v','mov'],video:['h264','hevc'],audio:['aac','mp3','ac3','eac3']},
  audioFiles(['aac','mp3'],['mp3','m4a','m4b','aac']),
  {transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:['aac','mp3','ac3','eac3']},
  {transport:'hls_fmp4',containers:['fmp4'],video:['h264','hevc'],audio:['aac','mp3','ac3','eac3']},
 ];
 return base;
}

/** LG webOS: the platform media pipeline behind a `<video>` element. It plays
 * Matroska and TS directly and takes HEVC and Dolby audio on every supported
 * model year; DTS disappeared in 2020–2022 sets and is left to the probe. */
export function webosDeclaredProfile(options:{osVersion?:string;model?:string}={}):ClientProfile{
 const video=['h264','hevc','vp9','mpeg2video','mpeg4'];
 const audio=['aac','mp3','ac3','eac3','pcm','vorbis','opus','flac'];
 return {
  version:1,evidence:'declared',client:{family:'webos',platform:'webos',os:'webos',osVersion:options.osVersion,model:options.model,engine:'webos-media'},
  display:{dynamicRanges:['sdr']},
  video:[v('h264',{bitDepths:[8],maxWidth:3840,maxHeight:2160,maxFrameRate:60,dynamicRanges:['sdr'],interlaced:true}),v('hevc',{bitDepths:[8,10],maxWidth:3840,maxHeight:2160,maxFrameRate:60,dynamicRanges:['sdr']}),v('vp9',{bitDepths:[8,10],maxWidth:3840,maxHeight:2160,dynamicRanges:['sdr']}),v('mpeg2video',{bitDepths:[8],maxWidth:1920,maxHeight:1080,interlaced:true}),v('mpeg4',{bitDepths:[8],maxWidth:1920,maxHeight:1080})],
  audio:[a('aac',6),a('mp3',2),a('ac3',6,{passthrough:true}),a('eac3',8,{passthrough:true,objectAudio:['atmos']}),a('pcm',6),a('vorbis',2),a('opus',2),a('flac',2)],
  audioOutput:{maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v','mov','mkv','ts','m2ts','webm','avi'],video,audio},
   audioFiles(['aac','mp3','flac','vorbis','pcm'],['mp3','m4a','aac','flac','ogg','wav']),
   {transport:'hls_ts',containers:['mpegts'],video:['h264','hevc'],audio:['aac','mp3','ac3','eac3']},
   {transport:'hls_fmp4',containers:['fmp4'],video:['h264','hevc'],audio:['aac','ac3','eac3']},
  ],
  subtitles:textSubtitles,
 };
}

/** Samsung Tizen: AVPlay. Close to webOS; Samsung sets have never decoded DTS
 * since 2018 and never Dolby Vision, and they are the platform that shows HDR10+. */
export function tizenDeclaredProfile(options:{osVersion?:string;model?:string}={}):ClientProfile{
 const p=webosDeclaredProfile(options);
 p.client={family:'tizen',platform:'tizen',os:'tizen',osVersion:options.osVersion,model:options.model,engine:'avplay'};
 return p;
}

/** Roku: the SceneGraph Video node. Every current model decodes H.264 at 1080p60;
 * the 4K models add HEVC 10-bit, VP9 and HDR10/HLG, and the Ultra adds Dolby Vision
 * and AV1. Dolby and DTS go to the HDMI sink as passthrough. The channel widens this
 * from roDeviceInfo at run time (model, codec and HDR answers). */
export function rokuDeclaredProfile(options:{osVersion?:string;model?:string;uhd?:boolean}={}):ClientProfile{
 const uhd=options.uhd!==false;
 const width=uhd?3840:1920,height=uhd?2160:1080;
 const hdr:DynamicRange[]=uhd?['sdr','hdr10','hlg']:['sdr'];
 const video=[v('h264',{bitDepths:[8],maxWidth:width,maxHeight:height,maxLevel:uhd?51:42,maxFrameRate:60,dynamicRanges:['sdr']})];
 if(uhd){
  video.push(v('hevc',{bitDepths:[8,10],maxWidth:width,maxHeight:height,maxLevel:153,maxFrameRate:60,dynamicRanges:hdr}));
  video.push(v('vp9',{bitDepths:[8,10],maxWidth:width,maxHeight:height,maxFrameRate:60,dynamicRanges:hdr}));
 }
 const names=video.map(x=>x.codec);
 const audio=['aac','mp3','ac3','eac3','flac','pcm','alac','opus','vorbis'];
 return {
  version:1,evidence:'declared',client:{family:'roku',platform:'roku',os:'roku',osVersion:options.osVersion,model:options.model,app:'portico-roku',engine:'roku-video'},
  display:{width,height,dynamicRanges:hdr,maxFrameRate:60},
  video,
  audio:[a('aac',6),a('mp3',2),a('ac3',6,{passthrough:true}),a('eac3',8,{passthrough:true,objectAudio:['atmos']}),a('flac',2),a('pcm',2),a('alac',2),a('opus',2),a('vorbis',2)],
  audioOutput:{maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v','mov','mkv','ts','m2ts'],video:names,audio},
   audioFiles(['aac','mp3','flac','pcm','alac','opus','vorbis'],['mp3','m4a','aac','flac','wav','ogg']),
   {transport:'hls_ts',containers:['mpegts'],video:names,audio:['aac','mp3','ac3','eac3']},
   {transport:'hls_fmp4',containers:['fmp4'],video:names,audio:['aac','ac3','eac3']},
  ],
  subtitles:textSubtitles,
 };
}

/** A Google Cast receiver. Models differ enough that the table is per model:
 * the first three generations are 1080p H.264 devices; Ultra and Google TV add
 * HEVC, VP9 and HDR. Audio beyond stereo AAC is passed to the HDMI sink, which
 * only the receiver can see, so the receiver widens this at run time. */
export function castDeclaredProfile(model:'chromecast'|'chromecast_ultra'|'google_tv_4k'|'google_tv_hd'|'nest_hub'|'unknown'):ClientProfile{
 const uhd=model==='chromecast_ultra'||model==='google_tv_4k';
 const hevc=uhd||model==='google_tv_hd';
 const height=uhd?2160:model==='nest_hub'?720:1080,width=uhd?3840:model==='nest_hub'?1280:1920;
 const hdr:DynamicRange[]=uhd?['sdr','hdr10','hlg',...(model==='google_tv_4k'?['hdr10plus','dolby_vision'] as DynamicRange[]:[])]:['sdr'];
 const video=[v('h264',{bitDepths:[8],maxWidth:width,maxHeight:height,maxLevel:uhd?51:41,maxFrameRate:uhd?60:30,dynamicRanges:['sdr']}),v('vp8',{bitDepths:[8],maxWidth:width,maxHeight:height})];
 if(hevc)video.push(v('hevc',{bitDepths:[8,10],maxWidth:width,maxHeight:height,maxLevel:uhd?153:123,maxFrameRate:60,dynamicRanges:hdr,dolbyVisionProfiles:model==='google_tv_4k'?[5,8]:[]}));
 if(hevc||model==='nest_hub')video.push(v('vp9',{bitDepths:uhd?[8,10]:[8],maxWidth:width,maxHeight:height,dynamicRanges:hdr.filter(r=>r!=='dolby_vision'&&r!=='hdr10plus')}));
 if(model==='google_tv_hd'||model==='google_tv_4k')video.push(v('av1',{bitDepths:[8,10],maxWidth:width,maxHeight:height,dynamicRanges:hdr.filter(r=>r!=='dolby_vision')}));
 const names=video.map(x=>x.codec);
 return {
  version:1,evidence:'declared',client:{family:'cast',platform:'cast',model,engine:'caf-shaka',app:'portico-cast'},
  display:{width,height,dynamicRanges:hdr,maxFrameRate:uhd?60:30},
  video,
  audio:[a('aac',2),a('mp3',2),a('opus',2),a('vorbis',2),a('flac',2)],
  audioOutput:{route:model==='nest_hub'?'speaker':'hdmi',maxChannels:2},
  transports:[
   {transport:'direct',containers:['mp4','m4v'],video:names.filter(x=>x!=='vp8'&&x!=='vp9'),audio:['aac','mp3']},
   {transport:'direct',containers:['webm'],video:names.filter(x=>x==='vp8'||x==='vp9'||x==='av1'),audio:['opus','vorbis']},
   audioFiles(['aac','mp3','opus','vorbis','flac'],['mp3','m4a','aac','flac','ogg','wav']),
   {transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:['aac','mp3']},
   {transport:'hls_fmp4',containers:['fmp4'],video:names.filter(x=>x==='h264'||x==='hevc'),audio:['aac','mp3']},
  ],
  subtitles:{text:['vtt','srt'],styled:[],bitmap:[]},
 };
}

/* ------------------------------------------------------------------------- */
/* Browser probe.                                                              */
/* ------------------------------------------------------------------------- */

export type DecodingAnswer={supported:boolean;smooth:boolean;powerEfficient:boolean};
export type VideoDecodingQuestion={contentType:string;width:number;height:number;bitrate:number;framerate:number;transferFunction?:'srgb'|'pq'|'hlg';colorGamut?:'srgb'|'p3'|'rec2020';hdrMetadataType?:'smpteSt2086'|'smpteSt2094-10'|'smpteSt2094-40'};

/** Everything the browser probe asks, as an interface, so it runs under test
 * without a browser and so a webview-based client can answer differently. */
export type WebProbeEnvironment={
 /** `HTMLMediaElement.canPlayType`. */
 canPlayType(type:string):string;
 /** `MediaSource.isTypeSupported` (or ManagedMediaSource); null when there is no MSE at all. */
 mediaSourceSupports:((type:string)=>boolean)|null;
 /** `navigator.mediaCapabilities.decodingInfo` for a media-source video configuration. */
 decodingInfo?:(question:VideoDecodingQuestion)=>Promise<DecodingAnswer>;
 /** `matchMedia(query).matches`. */
 matchMedia?:(query:string)=>boolean;
 /** `screen.width/height` multiplied by `devicePixelRatio`. */
 screen?:{width:number;height:number};
 /** `AudioContext.destination.maxChannelCount`. */
 audioOutputChannels?:number;
 /** `'audioTracks' in HTMLMediaElement.prototype`. */
 embeddedAudioTracks?:boolean;
 /** The engine tone-maps HDR well on a standard display (Safari; Chrome on macOS). COMPAT-05. */
 tonemapsHdr?:boolean;
 identity?:Partial<ClientIdentity>;
};

const mp4=(codecs:string)=>`video/mp4; codecs="${codecs}"`;
const mp4Audio=(codecs:string)=>`audio/mp4; codecs="${codecs}"`;

type VideoProbe={codec:string;contentType:string;tenBit?:string;dolbyVision?:Record<number,string>;hdr?:boolean};
const videoProbes:VideoProbe[]=[
 {codec:'h264',contentType:mp4('avc1.640028'),tenBit:mp4('avc1.6e0028')},
 {codec:'hevc',contentType:mp4('hvc1.1.6.L120.90'),tenBit:mp4('hvc1.2.4.L153.B0'),dolbyVision:{5:mp4('dvh1.05.06'),8:mp4('dvh1.08.06')},hdr:true},
 {codec:'av1',contentType:mp4('av01.0.08M.08'),tenBit:mp4('av01.0.12M.10.0.110.09.16.09.0'),hdr:true},
 {codec:'vp9',contentType:'video/webm; codecs="vp09.00.40.08"',tenBit:'video/webm; codecs="vp09.02.40.10"',hdr:true},
];
const audioProbes:{codec:string;mse:string[];element:string[];channels:number;objectAudio?:('atmos')[]}[]=[
 {codec:'aac',mse:[mp4Audio('mp4a.40.2')],element:[mp4Audio('mp4a.40.2')],channels:6},
 {codec:'mp3',mse:['audio/mpeg',mp4Audio('mp3')],element:['audio/mpeg'],channels:2},
 {codec:'ac3',mse:[mp4Audio('ac-3')],element:[mp4Audio('ac-3')],channels:6},
 {codec:'eac3',mse:[mp4Audio('ec-3')],element:[mp4Audio('ec-3')],channels:8,objectAudio:['atmos']},
 {codec:'flac',mse:[mp4Audio('flac'),mp4Audio('fLaC')],element:['audio/flac'],channels:6},
 {codec:'opus',mse:[mp4Audio('opus'),'audio/webm; codecs="opus"'],element:['audio/ogg; codecs="opus"','audio/webm; codecs="opus"'],channels:6},
 {codec:'vorbis',mse:['audio/webm; codecs="vorbis"'],element:['audio/ogg; codecs="vorbis"'],channels:6},
 {codec:'alac',mse:[mp4Audio('alac')],element:[mp4Audio('alac')],channels:6},
 {codec:'pcm',mse:[],element:['audio/wav; codecs="1"'],channels:2},
];

const plays=(answer:string)=>answer==='probably'||answer==='maybe';

/** Asks the browser what it can actually play. Every question is guarded: a
 * browser that throws, hangs or lacks an API simply contributes nothing, and the
 * result is never narrower than the declared floor for what the floor covers. */
export async function probeWebProfile(env:WebProbeEnvironment):Promise<ClientProfile>{
 const profile=webDeclaredProfile(env.identity);
 const mse=env.mediaSourceSupports;
 const safe=<T>(f:()=>T,fallback:T):T=>{try{return f();}catch{return fallback;}};
 const element=(type:string)=>safe(()=>plays(env.canPlayType(type)),false);
 const source=(type:string)=>mse?safe(()=>mse(type),false):false;
 const ask=async(q:VideoDecodingQuestion):Promise<DecodingAnswer|null>=>{
  if(!env.decodingInfo)return null;
  try{return await Promise.race([env.decodingInfo(q),new Promise<null>(resolve=>setTimeout(()=>resolve(null),1500))]);}catch{return null;}
 };
 const hdrDisplay=safe(()=>env.matchMedia?.('(dynamic-range: high)')??false,false);
 const nativeHls=element('application/vnd.apple.mpegurl');

 const video:ClientVideoCodec[]=[];
 const direct:string[]=[],fragmented:string[]=[],webm:string[]=[];
 for(const probe of videoProbes){
  const inElement=element(probe.contentType),inSource=source(probe.contentType);
  if(!inElement&&!inSource)continue;
  const entry:ClientVideoCodec={codec:probe.codec,bitDepths:[8],dynamicRanges:['sdr'],evidence:'probed',maxFrameRate:60};
  if(probe.tenBit&&(element(probe.tenBit)||source(probe.tenBit)))entry.bitDepths=[8,10];
  // Resolution: ask about 4K and fall back to 1080p. A browser without
  // MediaCapabilities is assumed to stop at 1080p for anything but H.264.
  const uhd=await ask({contentType:probe.tenBit&&entry.bitDepths!.includes(10)?probe.tenBit:probe.contentType,width:3840,height:2160,bitrate:40_000_000,framerate:30});
  if(uhd?.supported&&uhd.smooth){entry.maxWidth=4096;entry.maxHeight=2160;}
  else if(uhd===null&&probe.codec==='h264'){entry.maxWidth=4096;entry.maxHeight=2160;}
  else{entry.maxWidth=1920;entry.maxHeight=1080;}
  if(probe.hdr&&entry.bitDepths!.includes(10)&&probe.tenBit){
   // COMPAT-05: HDR goes untouched only to an HDR display, or to an engine known to tone-map
   // it well on a standard one. Elsewhere "supported and smooth" can still mean a flat, grey
   // picture (Firefox, some Windows GPUs), so the server converts instead.
   const pq=await ask({contentType:probe.tenBit,width:entry.maxWidth!>=3840?3840:1920,height:entry.maxHeight!>=2160?2160:1080,bitrate:25_000_000,framerate:30,transferFunction:'pq',colorGamut:'rec2020',hdrMetadataType:'smpteSt2086'});
   if(pq?.supported&&(hdrDisplay||env.tonemapsHdr===true&&pq.smooth))entry.dynamicRanges=['sdr','hdr10'];
   const hlg=await ask({contentType:probe.tenBit,width:1920,height:1080,bitrate:15_000_000,framerate:30,transferFunction:'hlg',colorGamut:'rec2020'});
   if(hlg?.supported&&entry.dynamicRanges!.includes('hdr10'))entry.dynamicRanges=[...entry.dynamicRanges!,'hlg'];
  }
  if(probe.dolbyVision&&hdrDisplay){
   const profiles=Object.entries(probe.dolbyVision).filter(([,type])=>element(type)||source(type)).map(([n])=>Number(n));
   if(profiles.length){entry.dolbyVisionProfiles=profiles;entry.dynamicRanges=[...new Set([...entry.dynamicRanges!,'dolby_vision' as DynamicRange])];}
  }
  video.push(entry);
  if(probe.codec==='vp9'){if(inElement)webm.push('vp9');}
  else{if(inElement)direct.push(probe.codec);if(inSource||nativeHls&&(probe.codec==='h264'||probe.codec==='hevc'))fragmented.push(probe.codec);}
  if(probe.codec==='av1'&&element('video/webm; codecs="av01.0.08M.08"'))webm.push('av1');
 }
 if(!video.some(x=>x.codec==='h264'))video.unshift(profile.video![0]);

 const outputChannels=Math.max(2,Math.min(8,env.audioOutputChannels??2));
 const audio:ClientAudioCodec[]=[],directAudio:string[]=[],sourceAudio:string[]=[],tsAudio:string[]=[];
 for(const probe of audioProbes){
  const inElement=probe.element.some(element),inSource=probe.mse.some(source);
  if(!inElement&&!inSource&&!(probe.codec==='aac'||probe.codec==='mp3'))continue;
  audio.push({codec:probe.codec,maxChannels:probe.channels,evidence:inElement||inSource?'probed':'declared',...(probe.objectAudio&&(inElement||inSource)?{objectAudio:probe.objectAudio}:{})});
  if(inElement||probe.codec==='aac'||probe.codec==='mp3')directAudio.push(probe.codec);
  if(inSource)sourceAudio.push(probe.codec);
  // hls.js demuxes AAC, MP3 and (where the browser decodes them) AC-3 and E-AC-3 from MPEG-TS.
  if(['aac','mp3'].includes(probe.codec)||['ac3','eac3'].includes(probe.codec)&&(inSource||nativeHls&&inElement))tsAudio.push(probe.codec);
 }

 const mp4DirectAudio=directAudio.filter(x=>['aac','mp3','ac3','eac3','flac','opus','alac'].includes(x));
 const transports:ClientTransport[]=[{transport:'direct',containers:['mp4','m4v','mov'],video:direct.length?direct:['h264'],audio:mp4DirectAudio}];
 if(webm.length)transports.push({transport:'direct',containers:['webm'],video:webm,audio:directAudio.filter(x=>x==='opus'||x==='vorbis')});
 const fileContainers:Record<string,string[]>={mp3:['mp3'],aac:['m4a','m4b','aac'],flac:['flac'],opus:['opus','ogg'],vorbis:['ogg'],pcm:['wav'],alac:['m4a']};
 const containers=[...new Set(directAudio.flatMap(x=>fileContainers[x]??[]))];
 transports.push(audioFiles(directAudio.filter(x=>fileContainers[x]),containers));
 if(mse||nativeHls){
  transports.push({transport:'hls_ts',containers:['mpegts'],video:['h264'],audio:tsAudio});
  const fmp4Audio=nativeHls&&!mse?mp4DirectAudio.filter(x=>x!=='opus'):sourceAudio.filter(x=>x!=='vorbis'&&x!=='pcm');
  transports.push({transport:'hls_fmp4',containers:['fmp4'],video:fragmented.length?fragmented:['h264'],audio:fmp4Audio.length?fmp4Audio:['aac']});
 }
 profile.evidence='probed';
 profile.video=video;
 profile.audio=audio;
 profile.transports=transports;
 profile.audioOutput={maxChannels:outputChannels};
 profile.display={...(env.screen?{width:Math.round(env.screen.width),height:Math.round(env.screen.height)}:{}),dynamicRanges:hdrDisplay?['sdr','hdr10','hlg']:['sdr'],maxFrameRate:60};
 profile.embeddedAudioSwitching=env.embeddedAudioTracks===true;
 const problem=clientProfileProblem(profile);
 // A probe must never produce a document the server refuses: the declared floor
 // is always valid, and is what playback falls back to.
 return problem?webDeclaredProfile(env.identity):profile;
}

/* ------------------------------------------------------------------------- */
/* Native probes.                                                              */
/* ------------------------------------------------------------------------- */

/** What the Apple native module reports. Every field is optional: an older
 * native build, or a simulator, simply reports less. */
export type AppleNativeFacts={
 model?:string;osVersion?:string;
 /** `VTIsHardwareDecodeSupported` per codec. */
 hardwareDecode?:{hevc?:boolean;av1?:boolean;dolbyVision?:boolean};
 /** `AVPlayer.availableHDRModes`, and whether the display is currently eligible. */
 hdrModes?:{hdr10?:boolean;hlg?:boolean;dolbyVision?:boolean};
 eligibleForHDRPlayback?:boolean;
 screen?:{width:number;height:number;maxFrameRate?:number};
 /** `AVAudioSession.maximumOutputNumberOfChannels`, the current route's port type, and spatial audio. */
 audio?:{maxOutputChannels?:number;route?:string;spatial?:boolean};
};

export function appleProfile(options:{television:boolean;osVersion?:string;model?:string;appVersion?:string},facts:AppleNativeFacts|null):ClientProfile{
 const profile=appleDeclaredProfile({...options,model:facts?.model??options.model,osVersion:facts?.osVersion??options.osVersion});
 if(!facts)return profile;
 profile.evidence='mixed';
 const hevc=profile.video!.find(x=>x.codec==='hevc')!;
 if(facts.hardwareDecode?.hevc===false){
  // Software HEVC on an old device stutters at 4K; keep it to 1080p.
  hevc.maxWidth=1920;hevc.maxHeight=1080;hevc.evidence='probed';
 }else if(facts.hardwareDecode?.hevc)hevc.evidence='probed';
 const display:DynamicRange[]=['sdr'];
 if(facts.eligibleForHDRPlayback!==false){
  if(facts.hdrModes?.hdr10)display.push('hdr10');
  if(facts.hdrModes?.hlg)display.push('hlg');
  if(facts.hdrModes?.dolbyVision)display.push('dolby_vision');
 }
 // COMPAT-02: what the device *decodes* is separate from what the screen *shows*. With hardware
 // HEVC Main10, AVPlayer decodes HDR10 and HLG and tone-maps them for an SDR screen itself, so the
 // server must not convert those titles; `display` still says what the screen shows.
 const hardwareHevc=facts.hardwareDecode?.hevc===true;
 hevc.dynamicRanges=hardwareHevc?[...new Set<DynamicRange>(['sdr','hdr10','hlg',...display])]:[...display];
 // Profile 5 is single-layer Dolby Vision; 8.1 and 8.4 carry an HDR10 or HLG base. With the
 // Dolby Vision decoder in hardware AVPlayer handles both, on an SDR screen too.
 if(facts.hardwareDecode?.dolbyVision===true||(display.includes('dolby_vision')&&facts.hardwareDecode?.dolbyVision!==false)){hevc.dolbyVisionProfiles=[5,8];if(!hevc.dynamicRanges.includes('dolby_vision'))hevc.dynamicRanges=[...hevc.dynamicRanges,'dolby_vision'];}
 else hevc.dynamicRanges=hevc.dynamicRanges.filter(x=>x!=='dolby_vision');
 // COMPAT-06: the Apple TV HD (AppleTV5,x) and devices without hardware HEVC decode H.264 up to
 // 1080p only; the declared 4K floor would send them files they stutter on.
 const h264=profile.video!.find(x=>x.codec==='h264');
 if(h264&&(/^AppleTV5,/.test(facts.model??'')||facts.hardwareDecode?.hevc===false)){h264.maxWidth=1920;h264.maxHeight=1080;h264.evidence='probed';}
 if(facts.hardwareDecode?.av1){
  profile.video!.push({codec:'av1',bitDepths:[8,10],maxWidth:4096,maxHeight:2304,maxFrameRate:60,dynamicRanges:display.filter(x=>x!=='dolby_vision'),evidence:'probed'});
  for(const t of profile.transports!)if((t.transport==='direct'&&t.video?.length)||t.transport==='hls_fmp4')t.video=[...t.video!,'av1'];
 }
 profile.display={...(facts.screen?{width:Math.round(facts.screen.width),height:Math.round(facts.screen.height),maxFrameRate:facts.screen.maxFrameRate??60}:{}),dynamicRanges:display};
 const channels=Math.max(2,Math.min(16,facts.audio?.maxOutputChannels??2));
 const route=(facts.audio?.route??'').toLowerCase().replace(/[^a-z0-9_.+-]/g,'').slice(0,32);
 profile.audioOutput={maxChannels:facts.audio?.spatial?Math.max(channels,8):channels,...(route&&token.test(route)?{route}:{}),spatial:facts.audio?.spatial===true};
 return clientProfileProblem(profile)?appleDeclaredProfile(options):profile;
}

/** What an Android native module reports from `MediaCodecList`, `Display` and
 * `AudioManager`. Kept here so the Android clients share one merge with tests. */
export type AndroidNativeFacts={
 model?:string;osVersion?:string;
 decoders?:{codec:string;hardware?:boolean;maxWidth?:number;maxHeight?:number;maxFrameRate?:number;maxBitrateBps?:number;profiles?:string[];maxLevel?:number;tenBit?:boolean;hdr?:DynamicRange[];dolbyVisionProfiles?:number[]}[];
 audioDecoders?:{codec:string;maxChannels?:number}[];
 /** `Display.getHdrCapabilities().getSupportedHdrTypes()`. */
 displayHdr?:DynamicRange[];
 screen?:{width:number;height:number;maxFrameRate?:number};
 /** `AudioManager.getDevices(GET_DEVICES_OUTPUTS)` encodings of the active sink: what the receiver takes as a bitstream. */
 passthrough?:{codec:string;maxChannels?:number;objectAudio?:('atmos'|'dtsx')[]}[];
 audio?:{maxOutputChannels?:number;route?:string};
};

export function androidProfile(options:{family?:'android'|'androidtv'|'firetv';television?:boolean;osVersion?:string;model?:string},facts:AndroidNativeFacts|null):ClientProfile{
 const declared=androidDeclaredProfile({...options,model:facts?.model??options.model,osVersion:facts?.osVersion??options.osVersion});
 if(!facts?.decoders?.length)return declared;
 const profile:ClientProfile={...declared,evidence:'mixed'};
 const display:DynamicRange[]=['sdr',...(facts.displayHdr??[]).filter(x=>x!=='sdr')];
 profile.video=facts.decoders.map(d=>({codec:d.codec,profiles:d.profiles,maxLevel:d.maxLevel,bitDepths:d.tenBit?[8,10]:[8],maxWidth:d.maxWidth,maxHeight:d.maxHeight,maxFrameRate:d.maxFrameRate,maxBitrateBps:d.maxBitrateBps,dynamicRanges:['sdr' as DynamicRange,...(d.hdr??[]).filter(x=>x!=='sdr'&&display.includes(x))],dolbyVisionProfiles:display.includes('dolby_vision')?d.dolbyVisionProfiles:[],evidence:'probed' as const}));
 const decoded=new Map((facts.audioDecoders??[]).map(x=>[x.codec,x]));
 const audio:ClientAudioCodec[]=(declared.audio??[]).filter(x=>!facts.audioDecoders||decoded.has(x.codec)).map(x=>({...x,maxChannels:decoded.get(x.codec)?.maxChannels??x.maxChannels,evidence:decoded.has(x.codec)?'probed' as const:'declared' as const}));
 for(const d of facts.audioDecoders??[])if(!audio.some(x=>x.codec===d.codec))audio.push({codec:d.codec,maxChannels:d.maxChannels??2,evidence:'probed'});
 for(const p of facts.passthrough??[]){
  const existing=audio.find(x=>x.codec===p.codec);
  const entry:ClientAudioCodec={codec:p.codec,maxChannels:p.maxChannels??8,passthrough:true,objectAudio:p.objectAudio,evidence:'probed'};
  if(existing)Object.assign(existing,entry);else audio.push(entry);
 }
 profile.audio=audio;
 const videoNames=profile.video.map(x=>x.codec),audioNames=audio.map(x=>x.codec);
 profile.transports=(declared.transports??[]).map(t=>t.transport==='direct'&&t.video?.length?{...t,video:videoNames,audio:audioNames}:{...t,video:(t.video??[]).filter(x=>videoNames.includes(x)),audio:t.transport==='direct'?(t.audio??[]).filter(x=>audioNames.includes(x)):[...new Set([...(t.audio??[]).filter(x=>audioNames.includes(x))])]});
 profile.display={...(facts.screen?{width:Math.round(facts.screen.width),height:Math.round(facts.screen.height),maxFrameRate:facts.screen.maxFrameRate}:{}),dynamicRanges:display};
 const passthroughChannels=Math.max(0,...(facts.passthrough??[]).map(x=>x.maxChannels??8));
 profile.audioOutput={maxChannels:Math.max(2,Math.min(16,Math.max(facts.audio?.maxOutputChannels??2,passthroughChannels))),...(facts.audio?.route&&token.test(facts.audio.route)?{route:facts.audio.route}:{})};
 return clientProfileProblem(profile)?declared:profile;
}

/* ------------------------------------------------------------------------- */
/* Publishing and reporting.                                                   */
/* ------------------------------------------------------------------------- */

export type ClientProfileReceipt=Readonly<{version:number;revision:string;summary:Readonly<{family:string;evidence:string;revision:string;published:boolean}>}>;
export type ProfileTransport=(path:string,method:'PUT'|'POST',body:unknown)=>Promise<unknown>;

/** Publishes a profile once per distinct document per sign-in. A failure is
 * swallowed by design: a device that could not publish is planned against the
 * server's baseline, which always plays, and publishing is tried again the next
 * time anything calls this. */
export class ClientProfilePublisher{
 private published='';private inflight?:Promise<ClientProfileReceipt|null>;
 private readonly send:ProfileTransport;private readonly scope:()=>string;
 /** `scope` names the sign-in (for example the access token's hash or the
  * server and profile ids): a different scope publishes again. */
 constructor(send:ProfileTransport,scope:()=>string){this.send=send;this.scope=scope;}
 publish(profile:ClientProfile):Promise<ClientProfileReceipt|null>{
  if(clientProfileProblem(profile))return Promise.resolve(null);
  const key=this.scope()+'\n'+JSON.stringify(profile);
  if(key===this.published)return Promise.resolve(null);
  if(this.inflight)return this.inflight;
  const work=this.send('/v1/playback/client-profile','PUT',profile).then(raw=>{
   const r=raw as any;
   if(!r||typeof r.revision!=='string'||!r.summary||typeof r.summary.family!=='string')return null;
   this.published=key;
   return Object.freeze({version:Number(r.version)||1,revision:r.revision,summary:Object.freeze({family:r.summary.family,evidence:String(r.summary.evidence??''),revision:String(r.summary.revision??''),published:r.summary.published===true})});
  }).catch(()=>null).finally(()=>{if(this.inflight===work)this.inflight=undefined;});
  this.inflight=work;return work;
 }
 /** Forget what was published, so the next call sends again (a new sign-in). */
 reset(){this.published='';}
}

/** Engine failure classes a player reports. They are recorded, not interpreted:
 * any of them closes the route that was in use for this file on this device. */
export type RouteFailureCode='decode_error'|'source_not_supported'|'audio_decode_error'|'manifest_incompatible'|'stalled_without_data'|'engine_error';
export type RouteFailureResult=Readonly<{route:string;escalates:boolean}>;

export async function reportRouteFailure(send:ProfileTransport,sessionId:string,code:RouteFailureCode,detail=''):Promise<RouteFailureResult|null>{
 if(!/^[A-Za-z0-9_-]{1,128}$/.test(sessionId))return null;
 try{
  const raw=await send('/v1/playback/route-failures','POST',{sessionId,code,detail:detail.replace(/[\x00-\x1f\x7f]/g,' ').slice(0,500)}) as any;
  if(!raw||typeof raw.escalates!=='boolean')return null;
  return Object.freeze({route:String(raw.route??''),escalates:raw.escalates});
 }catch{return null;}
}
