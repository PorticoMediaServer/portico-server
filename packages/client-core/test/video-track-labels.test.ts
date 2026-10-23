import test from 'node:test';
import assert from 'node:assert/strict';
import {videoPictureName,videoAudioFormatName,videoTrackName,formatVideoBytes,type VideoTrackWords} from '../src/video-track-labels.ts';
import type {AudioStream,SubtitleStream,VideoStream} from '../src/playback-v1/types.ts';
const words:VideoTrackWords={fourK:'4K',stereo:'Stereo',mono:'Mono',channels:n=>`${n} channels`,withAtmos:format=>`${format} with Atmos`,withDtsx:value=>`${value} (DTS:X)`,besideFile:format=>`${format}, beside the file`,language:tag=>({en:'English',eng:'English',ja:'Japanese'}[tag]??'')};
const audio=(facts:Partial<AudioStream>={}):AudioStream=>({id:'a1',codec:'truehd',atmos:false,default:true,commentary:false,visualImpaired:false,...facts});
const video=(facts:Partial<VideoStream>={}):VideoStream=>({id:'v0',codec:'hevc',...facts});

test('format-only audio preserves explicit objects without inferring them from codecs',()=>{
 assert.equal(videoAudioFormatName(audio({layout:'7.1',atmos:true}),words),'Dolby TrueHD with Atmos, 7.1');
 assert.equal(videoAudioFormatName(audio({layout:'7.1'}),words),'Dolby TrueHD, 7.1');
 assert.equal(videoAudioFormatName(audio({codec:'eac3',layout:'5.1'}),words),'Dolby Digital Plus, 5.1');
 assert.equal(videoAudioFormatName(audio({codec:'eac3',variant:'eac3_joc',layout:'5.1'}),words),'Dolby Digital Plus with Atmos, 5.1');
 assert.equal(videoAudioFormatName(audio({title:'Atmos',language:'en',layout:'7.1'}),words),'Dolby TrueHD, 7.1');
});
test('DTS:X is separate from its observed carrier profile',()=>{
 const facts=audio({codec:'dts',variant:'dtsx',profile:'DTS-HD MA',objectAudio:'dtsx',layout:'7.1'});
 assert.equal(videoAudioFormatName(facts,words),'DTS-HD Master Audio, 7.1 (DTS:X)');
 assert.equal(videoAudioFormatName({...facts,profile:undefined},words),'DTS, 7.1 (DTS:X)');
 assert.equal(videoAudioFormatName(audio({codec:'dts',profile:'DTS-ES Matrix',layout:'5.1'}),words),'DTS, 5.1');
});
test('channel counts alone do not become speaker geometry',()=>{
 assert.equal(videoAudioFormatName(audio({channels:8}),words),'Dolby TrueHD, 8 channels');
 assert.equal(videoAudioFormatName(audio({channels:2}),words),'Dolby TrueHD, 2 channels');
 assert.equal(videoAudioFormatName(audio({channels:2,layout:'stereo'}),words),'Dolby TrueHD, Stereo');
 assert.equal(videoAudioFormatName(audio({channels:6,layout:'5.1(side)'}),words),'Dolby TrueHD, 5.1');
});
test('track identity and technical parts remain separate and do not parse rendition labels',()=>{
 const facts=Object.freeze(audio({title:'Director’s commentary',language:'en',layout:'7.1',atmos:true}));
 const parts=videoTrackName(facts,words);
 assert.equal(parts.first,'Director’s commentary');assert.equal(parts.rest,'Dolby TrueHD with Atmos, 7.1');
 assert.equal(parts.label,'Director’s commentary, Dolby TrueHD with Atmos, 7.1');
 assert.equal(parts.title,facts.title);assert.equal(parts.language,'en');assert.equal(parts.objectAudio,'atmos');assert.equal(parts.layout,'7.1');
 assert.equal(videoTrackName(audio({language:'eng',layout:'7.1'}),words).first,'English');
});
test('subtitle origin is explicit and a missing download provider is not invented',()=>{
 const sub:SubtitleStream={id:'s0',format:'srt',source:'sidecar',language:'en',forced:false,sdh:false,default:false};
 assert.equal(videoTrackName(sub,words).label,'English, SRT, beside the file');
 const downloaded=videoTrackName({...sub,format:'ass',source:'downloaded'},words);
 assert.equal(downloaded.rest,'ASS');assert.equal(downloaded.subtitleOrigin,'downloaded');
 assert.equal(videoTrackName({...sub,format:'hdmv_pgs_subtitle',source:'embedded'},words).rest,'PGS');
});
test('picture naming retains actual scan and HDR facts while unknown stays omitted',()=>{
 assert.equal(videoPictureName(video({height:1080,interlaced:false,hdr:'hdr10'}),words),'1080p, HDR10');
 assert.equal(videoPictureName(video({height:1080,interlaced:true,hdr:'hdr10'}),words),'1080i, HDR10');
 assert.equal(videoPictureName(video({height:1080,hdr:'hdr10'}),words),'HDR10');
 assert.equal(videoPictureName(video({height:1080}),words),'');
 assert.equal(videoPictureName(video({width:3840,height:2160,hdr:'dolbyvision'}),words),'4K, Dolby Vision');
 assert.equal(videoPictureName(video({height:1080,interlaced:false,hdr:'hdr10',hdr10Plus:true}),words),'1080p, HDR10+');
});
test('word ports control localized phrasing and bytes retain the drawn precision',()=>{
 assert.equal(videoAudioFormatName(audio({atmos:true}),{...words,withAtmos:f=>`Atmos: ${f}` }),'Atmos: Dolby TrueHD');
 const bytes=Math.round(58.2*1024**3);
 assert.equal(formatVideoBytes(bytes,'en-US'),'58.2 GB');assert.equal(formatVideoBytes(bytes,'de-DE'),'58,2 GB');
 assert.equal(formatVideoBytes(0,'en-US'),'0 B');
 for(const value of [undefined,NaN,-1,Number.MAX_SAFE_INTEGER+1])assert.equal(formatVideoBytes(value,'en-US'),undefined);
});
